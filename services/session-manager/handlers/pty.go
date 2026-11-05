package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/entropy/pkg/ws"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// ptyBridge is the attached interactive terminal for a session.
type ptyBridge struct {
	mu       sync.Mutex
	upstream *websocket.Conn

	capturing bool
	started   time.Time
	capture   []byte
}

func (b *ptyBridge) inject(data string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_ = b.upstream.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return b.upstream.WriteMessage(websocket.BinaryMessage, []byte(data))
}

const maxCapture = 16384

func (b *ptyBridge) appendCapture(p []byte) {
	if !b.capturing || len(p) == 0 {
		return
	}
	b.capture = append(b.capture, p...)
	if len(b.capture) > maxCapture {
		b.capture = append([]byte(nil), b.capture[len(b.capture)-maxCapture:]...)
	}
}

func (c *conn) attachPTY(b *ptyBridge) {
	c.ptyMu.Lock()
	c.pty = b
	c.ptyMu.Unlock()
}

func (c *conn) detachPTY(b *ptyBridge) {
	c.ptyMu.Lock()
	if c.pty == b {
		c.pty = nil
	}
	c.ptyMu.Unlock()
}

func (c *conn) currentPTY() *ptyBridge {
	c.ptyMu.Lock()
	defer c.ptyMu.Unlock()
	return c.pty
}

// waitForCommand registers a one-shot waiter for the next finished command.
func (c *conn) waitForCommand(display string) chan commandResult {
	ch := make(chan commandResult, 1)
	c.ptyMu.Lock()
	c.cmdWaiter = ch
	c.pendingDisplay = display
	c.ptyMu.Unlock()
	return ch
}

func (c *conn) notifyCommandWaiter(r commandResult) {
	c.ptyMu.Lock()
	ch := c.cmdWaiter
	c.cmdWaiter = nil
	c.ptyMu.Unlock()
	if ch != nil {
		ch <- r
	}
}

func (c *conn) takePendingDisplay(cmd string) string {
	c.ptyMu.Lock()
	defer c.ptyMu.Unlock()
	d := c.pendingDisplay
	c.pendingDisplay = ""
	if d != "" && strings.Contains(cmd, demoScriptPath) {
		return d
	}
	return ""
}

const demoScriptPath = shellStateDir + "/demo.sh"

// runInPTY writes a script into the sandbox and types it into the learner's
// shell, so demos run where the learner is looking. It returns false when no
// interactive terminal is attached.
func (c *conn) runInPTY(ctx context.Context, script, display string) (commandResult, bool) {
	b := c.currentPTY()
	if b == nil {
		return commandResult{}, false
	}
	install := "mkdir -p " + shellStateDir + " && cat > " + demoScriptPath + " <<'ENTROPY_DEMO'\n" + script + "\nENTROPY_DEMO\n"
	if _, code, err := c.execQuiet(ctx, install); err != nil || code != 0 {
		return commandResult{}, false
	}
	done := c.waitForCommand(display)
	// Ctrl+U clears anything the learner had half typed.
	if err := b.inject("\x15bash " + demoScriptPath + "\r"); err != nil {
		return commandResult{}, false
	}
	select {
	case r := <-done:
		return r, true
	case <-ctx.Done():
		return commandResult{}, true
	case <-time.After(90 * time.Second):
		return commandResult{exitCode: 124}, true
	}
}

func (c *conn) onShellEvent(b *ptyBridge, ev shellEvent) {
	switch ev.kind {
	case shellCommandStart:
		b.capturing = true
		b.started = time.Now()
		b.capture = b.capture[:0]
		c.interrupted.Store(false)
	case shellCommandDone:
		output := plainText(b.capture)
		duration := time.Since(b.started)
		if !b.capturing {
			duration = 0
		}
		b.capturing = false
		cmd := strings.TrimSpace(ev.command)
		if cmd == "" {
			return
		}
		display := c.takePendingDisplay(cmd)
		c.accountCommand(cmd)
		c.finishCommand(finishedCommand{
			command: cmd, display: display, exitCode: ev.exitCode, output: output, stdout: output,
			duration: duration, cwd: ev.cwd, learner: display == "", pty: true,
		})
	case shellPrompt:
		b.capturing = false
		if ev.cwd != "" {
			c.send(ws.TypeShell, map[string]interface{}{"cwd": promptPath(ev.cwd)})
		}
	}
}

func airlockPTYURL(containerID string, cols, rows string) string {
	base := os.Getenv("AIRLOCK_URL")
	if base == "" {
		base = "http://airlock:8081"
	}
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/internal/pty"
	q := url.Values{"containerId": {containerID}, "cols": {cols}, "rows": {rows}}
	u.RawQuery = q.Encode()
	return u.String()
}

// HandlePTY bridges the browser terminal to an interactive shell in the
// session's sandbox. The control WebSocket (/ws) must already be connected;
// it owns the container and receives the shell integration events.
func (h *WebSocketHandler) HandlePTY(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	value, ok := h.active.Load(sessionID)
	if !ok || sessionID == "" {
		http.Error(w, "session is not connected", http.StatusConflict)
		return
	}
	c := value.(*activeWebSocket).conn
	if c == nil {
		http.Error(w, "session is not ready", http.StatusConflict)
		return
	}

	upstream, _, err := websocket.DefaultDialer.Dial(
		airlockPTYURL(c.containerID, r.URL.Query().Get("cols"), r.URL.Query().Get("rows")), nil)
	if err != nil {
		h.logger.Warn("pty dial failed", zap.Error(err), zap.String("session_id", sessionID))
		http.Error(w, "interactive terminal unavailable", http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	browser, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer browser.Close()

	bridge := &ptyBridge{upstream: upstream}
	c.attachPTY(bridge)
	defer c.detachPTY(bridge)
	h.rdb.Incr(context.Background(), "metric:pty_sessions")

	var browserMu sync.Mutex
	toBrowser := func(typ int, data []byte) error {
		browserMu.Lock()
		defer browserMu.Unlock()
		_ = browser.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return browser.WriteMessage(typ, data)
	}

	upDone := make(chan struct{})
	go func() {
		defer close(upDone)
		var parser oscParser
		for {
			typ, data, err := upstream.ReadMessage()
			if err != nil {
				return
			}
			if typ == websocket.TextMessage {
				if toBrowser(websocket.TextMessage, data) != nil {
					return
				}
				continue
			}
			var out []byte
			for _, seg := range parser.FeedSegments(data) {
				if seg.event != nil {
					c.onShellEvent(bridge, *seg.event)
					continue
				}
				bridge.appendCapture(seg.data)
				out = append(out, seg.data...)
			}
			if len(out) > 0 && toBrowser(websocket.BinaryMessage, out) != nil {
				return
			}
		}
	}()

	downDone := make(chan struct{})
	go func() {
		defer close(downDone)
		for {
			typ, data, err := browser.ReadMessage()
			if err != nil {
				return
			}
			if typ == websocket.BinaryMessage && strings.ContainsRune(string(data), 0x03) {
				c.interrupted.Store(true)
			}
			if typ == websocket.TextMessage {
				var ctl struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &ctl) != nil || ctl.Type != "resize" {
					continue
				}
			}
			bridge.mu.Lock()
			err = upstream.WriteMessage(typ, data)
			bridge.mu.Unlock()
			if err != nil {
				return
			}
			_ = h.manager.Touch(c.ctx, sessionID)
		}
	}()

	select {
	case <-upDone:
	case <-downDone:
	}
	h.logger.Info("pty closed", zap.String("session_id", sessionID))
}
