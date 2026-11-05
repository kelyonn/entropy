package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgai "github.com/entropy/pkg/ai"
	"github.com/entropy/pkg/airlock"
	"github.com/entropy/pkg/db"
	pkgebpf "github.com/entropy/pkg/ebpf"
	"github.com/entropy/pkg/ws"
	"github.com/entropy/services/session-manager/labs"
	"github.com/entropy/services/session-manager/session"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin(allowedOrigins()),
}

type WSEnvelope struct {
	Type            string          `json:"type"`
	SessionID       string          `json:"sessionId"`
	Timestamp       string          `json:"timestamp,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Command         string          `json:"command,omitempty"`
	SessionIDLegacy string          `json:"session_id,omitempty"`
}

type WebSocketHandler struct {
	rdb      *redis.Client
	logger   *zap.Logger
	manager  *session.Manager
	store    *db.Store
	airlock  *airlock.Client
	aiClient *pkgai.Client
	progress *labs.Progress
	active   sync.Map
}

// failureContext is the most recent failed command, kept so the learner can
// ask for an explanation or a follow-up at any time.
type failureContext struct {
	command  string
	output   string
	exitCode int
}

type queuedCommand struct {
	command string
	// display is what the terminal shows instead of the raw script, used for
	// lab demos whose scripts are long.
	display string
	done    chan commandResult
}

type commandResult struct {
	command  string
	stdout   string
	stderr   string
	exitCode int
}

type activeWebSocket struct {
	terminate func(reason string)
	kernel    chan pkgebpf.KernelEvent
	conn      *conn
}

func (h *WebSocketHandler) NotifyKernelEvent(event pkgebpf.KernelEvent) {
	if value, ok := h.active.Load(event.SessionID); ok {
		select {
		case value.(*activeWebSocket).kernel <- event:
		default:
			h.logger.Warn("kernel event channel full", zap.String("session_id", event.SessionID))
		}
	}
}

func NewWebSocketHandler(rdb *redis.Client, logger *zap.Logger, manager *session.Manager, store *db.Store) *WebSocketHandler {
	airURL := os.Getenv("AIRLOCK_URL")
	if airURL == "" {
		airURL = "http://airlock:8081"
	}
	aiURL := os.Getenv("AI_EXPLAINER_URL")
	if aiURL == "" {
		aiURL = "http://ai-explainer:8082"
	}
	return &WebSocketHandler{
		rdb:      rdb,
		logger:   logger,
		manager:  manager,
		store:    store,
		airlock:  airlock.NewClient(airURL),
		aiClient: pkgai.NewClient(aiURL),
		progress: labs.NewProgress(rdb),
	}
}

func (h *WebSocketHandler) NotifyTermination(sessionID, reason string) {
	if value, ok := h.active.Load(sessionID); ok {
		value.(*activeWebSocket).terminate(reason)
	}
}

// conn holds the state of one learner connection.
type conn struct {
	h           *WebSocketHandler
	ws          *websocket.Conn
	sessionID   string
	containerID string
	owner       string
	ctx         context.Context
	startedAt   time.Time

	writeMu sync.Mutex

	failMu      sync.Mutex
	lastFailure failureContext

	// interrupted is set when the learner presses Ctrl+C during a command so
	// the resulting signal exit is not treated as a crash.
	interrupted atomic.Bool

	commandsRun   atomic.Int64
	crashes       atomic.Int64
	labsCompleted atomic.Int64

	labMu sync.Mutex
	lab   *labRun

	// trail is the learner's recent commands, shared with the AI so it can
	// point at what they actually did.
	trailMu sync.Mutex
	trail   []trailEntry

	// explained remembers command+exit pairs already auto-explained so a
	// repeated mistake does not spend the AI budget twice.
	explainedMu sync.Mutex
	explained   map[string]bool

	// ptyMu guards the attached interactive terminal and demo waiters.
	ptyMu          sync.Mutex
	pty            *ptyBridge
	cmdWaiter      chan commandResult
	pendingDisplay string

	replayMu sync.Mutex

	// aiMu serializes explanation streams so tokens never interleave.
	aiMu sync.Mutex

	cmdQueue chan queuedCommand
}

func (c *conn) send(typ string, payload interface{}) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = writeEnv(c.ws, typ, c.sessionID, payload)
}

func (c *conn) notice(text string) {
	c.send(ws.TypeOutput, map[string]interface{}{"stdout": "\r\n" + text + "\r\n", "completed": false, "notice": true})
}

// execQuiet runs a script in the sandbox without streaming it to the
// terminal. It backs lab setup, objective checks, and the cwd probe.
func (c *conn) execQuiet(ctx context.Context, script string) (string, int, error) {
	var out strings.Builder
	exitCode := -1
	err := c.h.airlock.ExecStream(ctx, airlock.ExecRequest{
		SessionID: c.sessionID, ContainerID: c.containerID, Command: script,
	}, func(ch airlock.ExecChunk) error {
		out.WriteString(ch.Stdout)
		out.WriteString(ch.Stderr)
		if ch.Completed {
			exitCode = ch.ExitCode
		}
		return nil
	})
	return out.String(), exitCode, err
}

func (c *conn) currentCwd() string {
	out, code, err := c.execQuiet(c.ctx, cwdProbe)
	if err != nil || code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

func (c *conn) sendStats() {
	stats, err := c.h.airlock.Stats(c.ctx, c.containerID)
	if err != nil {
		return
	}
	c.h.rdb.Set(c.ctx, "metric:container_cpu", stats.CPU, 0)
	c.h.rdb.Set(c.ctx, "metric:container_memory", stats.Memory, 0)
	payload := map[string]interface{}{"cpu": stats.CPU, "memory": stats.Memory, "pids": stats.Pids}
	if stats.MemoryBytes > 0 {
		payload["memoryBytes"] = stats.MemoryBytes
		payload["memoryLimit"] = stats.MemoryLimit
	}
	if c.stackActive() {
		if t, ok := c.readTraffic(); ok {
			payload["traffic"] = t
		}
	}
	c.send(ws.TypeVisualization, payload)
}

func (c *conn) aiContext() string {
	c.labMu.Lock()
	var b strings.Builder
	if c.lab != nil {
		b.WriteString(c.lab.lab.AIContext())
		if alerts := c.lab.alerts; len(alerts) > 0 {
			b.WriteString("\nThings the learner fixed and then undid:")
			for _, a := range alerts {
				b.WriteString("\n- " + a.Message)
			}
		}
		if len(c.lab.lab.Objectives) > 0 {
			b.WriteString("\nObjective status:")
			for _, o := range c.lab.lab.Objectives {
				mark := "not met"
				if c.lab.status[o.ID] {
					mark = "met"
				}
				b.WriteString("\n- " + o.Text + ": " + mark)
			}
		}
	}
	c.labMu.Unlock()
	if trail := c.recentTrail(6); trail != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Learner's recent commands, oldest first. Point out a specific mistake if one of these caused the problem:\n")
		b.WriteString(trail)
	}
	return b.String()
}

type trailEntry struct {
	command  string
	exitCode int
	output   string
}

func (c *conn) recordTrail(command string, exitCode int, output string) {
	c.trailMu.Lock()
	defer c.trailMu.Unlock()
	c.trail = append(c.trail, trailEntry{command: clip(command, 200), exitCode: exitCode, output: clip(tail(strings.TrimSpace(output), 160), 160)})
	if len(c.trail) > 12 {
		c.trail = c.trail[len(c.trail)-12:]
	}
}

func (c *conn) recentTrail(n int) string {
	c.trailMu.Lock()
	defer c.trailMu.Unlock()
	start := len(c.trail) - n
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	for _, e := range c.trail[start:] {
		fmt.Fprintf(&b, "$ %s  (exit %d)\n", e.command, e.exitCode)
		if e.output != "" {
			b.WriteString("  " + strings.ReplaceAll(e.output, "\n", "\n  ") + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// firstAutoExplain reports whether this command+exit pair has not been
// auto-explained yet in this session.
func (c *conn) firstAutoExplain(command string, exitCode int) bool {
	key := fmt.Sprintf("%d|%s", exitCode, strings.TrimSpace(command))
	c.explainedMu.Lock()
	defer c.explainedMu.Unlock()
	if c.explained == nil {
		c.explained = map[string]bool{}
	}
	if c.explained[key] {
		return false
	}
	c.explained[key] = true
	return true
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// streamAI streams an explanation and restores the visualization to
// afterStatus once the stream ends.
func (c *conn) streamAI(command, output string, exitCode int, simpler bool, question, context, afterStatus string, auto bool) {
	h := c.h
	c.aiMu.Lock()
	defer c.aiMu.Unlock()
	c.send(ws.TypeVisualization, map[string]interface{}{"status": "analyzing"})
	c.send(ws.TypeAIStream, map[string]interface{}{
		"token": "", "done": false, "reset": true,
		"subject": map[string]interface{}{"command": command, "exitCode": exitCode, "question": question},
	})
	req := pkgai.ExplainRequest{
		SessionID: c.sessionID, Command: command, Stderr: output,
		ExitCode: exitCode, Simpler: simpler, Question: question, Context: context, Auto: auto,
	}
	var explanation strings.Builder
	started := time.Now()
	var firstToken sync.Once
	err := h.aiClient.ExplainStream(c.ctx, req, func(tok pkgai.ExplainToken) error {
		if tok.Token != "" {
			firstToken.Do(func() {
				h.rdb.Set(c.ctx, "metric:ai_latency_ms", time.Since(started).Milliseconds(), 0)
			})
			h.rdb.Incr(c.ctx, "metric:ai_token_count")
		}
		explanation.WriteString(tok.Token)
		c.send(ws.TypeAIStream, map[string]interface{}{"token": tok.Token, "done": tok.Done, "fallback": tok.Fallback})
		return nil
	})
	if err != nil {
		h.logger.Warn("ai stream failed", zap.Error(err))
		c.send(ws.TypeAIStream, map[string]interface{}{
			"token": "The explanation service is unavailable right now. Read the output above, then try again shortly.",
			"done":  false, "fallback": true,
		})
		c.send(ws.TypeAIStream, map[string]interface{}{"token": "", "done": true, "fallback": true})
	}
	if text := explanation.String(); text != "" {
		provider := os.Getenv("LLM_PROVIDER")
		if provider == "" {
			provider = "ollama"
		}
		model := os.Getenv("LLM_MODEL")
		if model == "" {
			model = os.Getenv("OLLAMA_MODEL")
		}
		if model == "" {
			model = "default"
		}
		_ = h.store.InsertAIExplanation(c.ctx, c.sessionID, text, provider+":"+model, len(strings.Fields(text)))
	}
	h.rdb.Incr(c.ctx, "metric:ai_explanations")
	c.send(ws.TypeVisualization, map[string]interface{}{"status": afterStatus})
}

func (c *conn) rememberFailure(command, output string, exitCode int) {
	c.failMu.Lock()
	c.lastFailure = failureContext{command: command, output: output, exitCode: exitCode}
	c.failMu.Unlock()
}

// handleCrash reacts to a crash-class failure: signal deaths, timeouts,
// kernel events. These are explained automatically.
func (c *conn) handleCrash(command, output string, exitCode int, event string) {
	c.crashes.Add(1)
	c.rememberFailure(command, output, exitCode)
	c.h.rdb.Incr(c.ctx, "metric:total_crashes")
	viz := map[string]interface{}{"status": "crashed"}
	if event != "" {
		viz["event"] = event
	}
	c.send(ws.TypeVisualization, viz)
	c.send(ws.TypeCrash, map[string]interface{}{
		"data": crashSummary(output, exitCode), "exitCode": exitCode, "command": command, "event": event,
	})
	go c.streamAI(command, output, exitCode, false, "", c.aiContext(), "crashed", true)
}

func (c *conn) persistSummary(reason string, deliver bool) {
	summary := ws.SessionSummaryPayload{
		CommandsRun:        int(c.commandsRun.Load()),
		CrashesEncountered: int(c.crashes.Load()),
		DurationSeconds:    int(time.Since(c.startedAt).Seconds()),
		Reason:             reason,
		LabsCompleted:      int(c.labsCompleted.Load()),
	}
	raw, _ := json.Marshal(summary)
	bg := context.Background()
	_ = c.h.rdb.Set(bg, "summary:"+c.sessionID, raw, 30*24*time.Hour).Err()
	c.h.rdb.LPush(bg, "metric:session_summary_events", raw)
	c.h.rdb.Incr(bg, "metric:session_summaries")
	_ = c.h.store.FinalizeSession(bg, c.sessionID, summary.CommandsRun, summary.CrashesEncountered)
	if deliver {
		c.send(ws.TypeSessionSummary, summary)
	}
}

// runCommand executes one queued command and reports the result.
func (c *conn) runCommand(q queuedCommand) commandResult {
	h := c.h
	cmd := q.command
	c.accountCommand(cmd)

	display := cmd
	if q.display != "" {
		display = q.display
	}

	if hint := interactiveHint(cmd); hint != "" {
		c.send(ws.TypeOutput, map[string]interface{}{"stderr": hint + "\n", "completed": false, "notice": true})
		c.send(ws.TypeOutput, map[string]interface{}{
			"exitCode": 0, "completed": true, "command": display, "durationMs": 0, "cwd": promptPath(c.currentCwd()),
		})
		return commandResult{command: cmd}
	}

	c.interrupted.Store(false)
	started := time.Now()
	var stdout, stderr strings.Builder
	exitCode := 0
	err := h.airlock.ExecStream(c.ctx, airlock.ExecRequest{
		SessionID: c.sessionID, ContainerID: c.containerID, Command: wrapUserCommand(cmd),
	}, func(ch airlock.ExecChunk) error {
		stdout.WriteString(ch.Stdout)
		stderr.WriteString(ch.Stderr)
		if !ch.Completed {
			c.send(ws.TypeOutput, map[string]interface{}{
				"stdout": ch.Stdout, "stderr": ch.Stderr, "completed": false,
			})
		} else {
			exitCode = ch.ExitCode
		}
		return nil
	})
	if err != nil {
		h.logger.Warn("exec failed", zap.Error(err), zap.String("session_id", c.sessionID))
		c.send(ws.TypeOutput, map[string]interface{}{
			"stderr": "sandbox unavailable: the command could not be started\n", "completed": false,
		})
		if exitCode == 0 {
			exitCode = 126
		}
	}
	output := stderr.String()
	if strings.TrimSpace(output) == "" {
		output = stdout.String()
	}
	c.finishCommand(finishedCommand{
		command: cmd, display: display, exitCode: exitCode, output: output,
		stdout: stdout.String(), stderr: stderr.String(), duration: time.Since(started),
		cwd: c.currentCwd(), learner: q.display == "",
	})
	return commandResult{command: cmd, stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func (h *WebSocketHandler) Handle(w http.ResponseWriter, r *http.Request) {
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer wsConn.Close()

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	ctx := context.Background()

	var existingContainerID, userID string
	if s, err := h.manager.Get(ctx, sessionID); err == nil {
		if s.Status == session.StatusTerminating || s.Status == session.StatusTerminated {
			var summary ws.SessionSummaryPayload
			if raw, getErr := h.rdb.Get(ctx, "summary:"+sessionID).Bytes(); getErr == nil {
				_ = json.Unmarshal(raw, &summary)
			}
			if summary.Reason == "" {
				summary.Reason = "session ended"
			}
			_ = writeEnv(wsConn, ws.TypeSessionSummary, sessionID, summary)
			return
		}
		existingContainerID = s.ContainerID
		userID = s.UserID
	}
	if userID == "" {
		userID = r.Header.Get("X-User-Id")
	}
	h.manager.CancelDisconnect(sessionID)

	containerID, err := h.airlock.Spawn(ctx, sessionID, existingContainerID, "")
	if err != nil {
		h.rdb.Incr(ctx, "metric:session_creation_failures")
		h.logger.Error("Container spawn failed", zap.Error(err))
		_ = writeEnv(wsConn, "error", sessionID, map[string]string{"message": "failed to spawn container"})
		return
	}
	_ = h.manager.Activate(ctx, sessionID, containerID)
	_ = h.store.UpsertSession(ctx, sessionID, "", containerID, string(session.StatusActive))

	c := &conn{
		h: h, ws: wsConn, sessionID: sessionID, containerID: containerID,
		owner: labs.OwnerKey(userID, sessionID), ctx: ctx, startedAt: time.Now(),
		cmdQueue: make(chan queuedCommand, 10),
	}

	var summaryOnce sync.Once
	var serverTerminated atomic.Bool
	persistSummary := func(reason string, deliver bool) {
		summaryOnce.Do(func() { c.persistSummary(reason, deliver) })
	}

	disconnectScheduled := false
	scheduleDisconnect := func() {
		if disconnectScheduled || serverTerminated.Load() {
			return
		}
		disconnectScheduled = true
		h.manager.ScheduleDisconnect(sessionID, 5*time.Second, func() {
			h.logger.Info("terminating disconnected session", zap.String("session_id", sessionID))
			persistSummary("connection lost", false)
			_, _ = h.manager.MarkTerminating(context.Background(), sessionID)
			_ = h.manager.MarkTerminated(context.Background(), sessionID)
			_ = h.airlock.Stop(context.Background(), airlock.StopRequest{
				SessionID: sessionID, ContainerID: containerID, GraceSec: 5,
			})
		})
	}

	kernelEvents := make(chan pkgebpf.KernelEvent, 20)
	kernelDone := make(chan struct{})
	defer close(kernelDone)
	active := &activeWebSocket{kernel: kernelEvents, conn: c, terminate: func(reason string) {
		serverTerminated.Store(true)
		persistSummary(reason, true)
		c.writeMu.Lock()
		_ = wsConn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason),
			time.Now().Add(time.Second))
		c.writeMu.Unlock()
	}}
	h.active.Store(sessionID, active)
	defer h.active.Delete(sessionID)
	defer scheduleDisconnect()

	_ = wsConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	wsConn.SetPongHandler(func(string) error {
		return wsConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	})
	pingTicker := time.NewTicker(2 * time.Second)
	defer pingTicker.Stop()
	pingDone := make(chan struct{})
	defer close(pingDone)
	go func() {
		for {
			select {
			case <-pingTicker.C:
				c.writeMu.Lock()
				err := wsConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))
				c.writeMu.Unlock()
				if err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case event := <-kernelEvents:
				exitCode := event.ExitCode
				if exitCode == 0 {
					switch event.EventType {
					case pkgebpf.EventOOMKill:
						exitCode = 137
					case pkgebpf.EventKernelPanic:
						exitCode = 99
					case pkgebpf.EventSegfault:
						exitCode = 139
					}
				}
				message := event.Message
				if message == "" {
					message = fmt.Sprintf("kernel event %s detected for pid %d", event.EventType, event.PID)
				}
				c.handleCrash("[kernel event: "+event.EventType+"]", message, exitCode, event.EventType)
			case <-kernelDone:
				return
			}
		}
	}()

	if _, code, err := c.execQuiet(ctx, installShellRC); err != nil || code != 0 {
		h.logger.Warn("shell rc install failed; cd and export will not persist",
			zap.String("session_id", sessionID), zap.Int("exit", code), zap.Error(err))
	}
	c.send(ws.TypeVisualization, map[string]interface{}{"status": "healthy"})
	c.send(ws.TypeShell, map[string]interface{}{
		"cwd": promptPath(c.currentCwd()), "startedAt": c.startedAt.UTC().Format(time.RFC3339),
	})

	if replay, _ := h.rdb.LRange(ctx, fmt.Sprintf("history:%s", sessionID), 0, 49).Result(); len(replay) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "\n[restored] %d prior commands\n", len(replay))
		for index := len(replay) - 1; index >= 0; index-- {
			fmt.Fprintf(&b, "  %s\n", replay[index])
		}
		c.send(ws.TypeOutput, map[string]interface{}{"stdout": b.String(), "notice": true})
		c.send(ws.TypeShell, map[string]interface{}{"history": reverse(replay)})
	}

	statsTicker := time.NewTicker(2 * time.Second)
	defer statsTicker.Stop()
	statsDone := make(chan struct{})
	defer close(statsDone)
	go func() {
		c.sendStats()
		tick := 0
		for {
			select {
			case <-statsTicker.C:
				c.sendStats()
				// Re-check objectives every other tick so time-based goals
				// (SLO windows) complete and regressions surface without the
				// learner typing another command.
				tick++
				if tick%2 == 0 {
					c.afterCommand()
				}
			case <-statsDone:
				return
			}
		}
	}()

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for queued := range c.cmdQueue {
			result := c.runCommand(queued)
			if queued.done != nil {
				queued.done <- result
			}
		}
	}()

	for {
		_, rawMsg, err := wsConn.ReadMessage()
		if err != nil {
			h.logger.Info("WebSocket closed", zap.String("session", sessionID))
			scheduleDisconnect()
			break
		}

		var env WSEnvelope
		if err := json.Unmarshal(rawMsg, &env); err != nil {
			continue
		}

		switch env.Type {
		case "ping", ws.TypeHeartbeat:
			c.send(ws.TypePong, map[string]string{"ok": "1"})
		case ws.TypeCommand:
			cmd := env.Command
			if len(env.Payload) > 0 {
				var p struct {
					Command string `json:"command"`
				}
				_ = json.Unmarshal(env.Payload, &p)
				if p.Command != "" {
					cmd = p.Command
				}
			}
			if strings.TrimSpace(cmd) == "" {
				continue
			}
			select {
			case c.cmdQueue <- queuedCommand{command: cmd}:
			default:
				c.send(ws.TypeError, map[string]string{"message": "command queue full, try again"})
			}
		case ws.TypeInterrupt:
			c.interrupted.Store(true)
			go func() {
				if err := h.airlock.Kill(ctx, airlock.KillRequest{ContainerID: containerID, Signal: "SIGINT"}); err != nil {
					h.logger.Warn("interrupt failed", zap.Error(err))
				}
			}()
		case ws.TypeAIExplain, "ai_question":
			var p struct {
				Question string `json:"question"`
				Simpler  bool   `json:"simpler"`
			}
			_ = json.Unmarshal(env.Payload, &p)
			c.failMu.Lock()
			lf := c.lastFailure
			c.failMu.Unlock()
			q := strings.TrimSpace(p.Question)
			if p.Simpler && q == "" {
				q = "Explain more simply"
			}
			if lf.command == "" && q == "" {
				c.send(ws.TypeError, map[string]string{"message": "nothing has failed yet. Ask a question instead."})
				continue
			}
			command := lf.command
			if command == "" {
				command = "(no failing command)"
			}
			go c.streamAI(command, lf.output, lf.exitCode, p.Simpler, q, c.aiContext(), "healthy", false)
		case ws.TypeLab, ws.TypeScenario:
			var p struct {
				ID     string `json:"id"`
				Action string `json:"action"`
			}
			_ = json.Unmarshal(env.Payload, &p)
			c.handleLabAction(p.Action, p.ID)
		}
	}

	c.abandonLab("connection closed", false)
	close(c.cmdQueue)
	<-workerDone
}

type exitClass int

const (
	exitOK exitClass = iota
	exitFailure
	exitCrash
)

// classifyExit separates ordinary failures (a missing file, a bad flag) from
// crash-class exits (killed by a signal, timed out, init failure). Only
// crashes are explained automatically; ordinary failures can be explained on
// request so the panel does not interrupt normal exploration.
func classifyExit(code int) exitClass {
	switch {
	case code == 0:
		return exitOK
	case code == 130:
		// SIGINT is how a person stops a command, not a failure to explain.
		return exitFailure
	case code == 99 || code == 124 || code >= 128:
		return exitCrash
	default:
		return exitFailure
	}
}

func inferEvent(exitCode int, output string) string {
	switch exitCode {
	case 137:
		return "oom_kill"
	case 139:
		return "segfault"
	case 99:
		return "kernel_panic"
	case 124:
		return "timeout"
	}
	return ""
}

func crashSummary(output string, exitCode int) string {
	switch exitCode {
	case 124:
		return "Command exceeded the 30 second limit and was stopped."
	case 130:
		return "Command interrupted (SIGINT)."
	case 137:
		return "Process killed by SIGKILL (exit 137). Often the OOM killer."
	case 139:
		return "Segmentation fault (SIGSEGV, exit 139)."
	case 99:
		return "Container init failure."
	}
	if exitCode > 128 {
		return fmt.Sprintf("Process terminated by signal %d (exit %d).", exitCode-128, exitCode)
	}
	if line := lastLine(output); line != "" {
		return line
	}
	return fmt.Sprintf("Exit %d", exitCode)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func reverse(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[len(in)-1-i] = v
	}
	return out
}

func writeEnv(conn *websocket.Conn, typ, sessionID string, payload interface{}) error {
	msg, err := ws.MarshalClientMessage(typ, sessionID, payload)
	if err != nil {
		return err
	}
	return conn.WriteJSON(json.RawMessage(msg))
}
