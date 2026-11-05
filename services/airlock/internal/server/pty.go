package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// ptyUpgrader serves the Compose-internal PTY bridge. Only the Session
// Manager reaches Airlock, so origin checks happen there.
var ptyUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 16384,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// PTYShell is the interactive shell started for a terminal. The rc file is
// installed by the Session Manager and adds shell integration markers.
var PTYShell = []string{"/bin/bash", "--rcfile", "/root/.entropy/ptyrc", "-i"}

type ptyControl struct {
	Type string `json:"type"`
	Cols uint   `json:"cols"`
	Rows uint   `json:"rows"`
}

func clampSize(v string, def, max uint) uint {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	if uint(n) > max {
		return max
	}
	return uint(n)
}

// handlePTY attaches a WebSocket to an interactive TTY exec in the sandbox.
// Binary frames carry terminal bytes in both directions; text frames carry
// control messages: {"type":"resize","cols":..,"rows":..} from the client and
// {"type":"exit","code":..} from the server when the shell ends.
func (s *Server) handlePTY(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("containerId")
	if containerID == "" {
		http.Error(w, "containerId required", http.StatusBadRequest)
		return
	}
	cols := clampSize(r.URL.Query().Get("cols"), 100, 500)
	rows := clampSize(r.URL.Query().Get("rows"), 30, 200)

	cli := s.orch.Client()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	execResp, err := cli.ContainerExecCreate(ctx, containerID, types.ExecConfig{
		Tty: true, AttachStdin: true, AttachStdout: true, AttachStderr: true,
		Cmd:         PTYShell,
		Env:         []string{"TERM=xterm-256color", "COLORTERM=truecolor", "ENTROPY_PTY=1", "LANG=C.UTF-8"},
		WorkingDir:  "/root",
		ConsoleSize: &[2]uint{rows, cols},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	hijack, err := cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{Tty: true, ConsoleSize: &[2]uint{rows, cols}})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer hijack.Close()

	conn, err := ptyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = cli.ContainerExecResize(ctx, execResp.ID, container.ResizeOptions{Height: rows, Width: cols})

	var writeMu sync.Mutex
	write := func(typ int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(typ, data)
	}

	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		buf := make([]byte, 16384)
		for {
			n, readErr := hijack.Reader.Read(buf)
			if n > 0 {
				if write(websocket.BinaryMessage, buf[:n]) != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		for {
			typ, data, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}
			switch typ {
			case websocket.BinaryMessage:
				if _, err := hijack.Conn.Write(data); err != nil {
					return
				}
			case websocket.TextMessage:
				var ctl ptyControl
				if json.Unmarshal(data, &ctl) == nil && ctl.Type == "resize" && ctl.Cols > 0 && ctl.Rows > 0 {
					_ = cli.ContainerExecResize(ctx, execResp.ID, container.ResizeOptions{
						Height: min(ctl.Rows, 200), Width: min(ctl.Cols, 500),
					})
				}
			}
		}
	}()

	select {
	case <-outputDone:
		code := -1
		if inspect, err := cli.ContainerExecInspect(context.Background(), execResp.ID); err == nil {
			code = inspect.ExitCode
		}
		msg, _ := json.Marshal(map[string]interface{}{"type": "exit", "code": code})
		_ = write(websocket.TextMessage, msg)
		writeMu.Lock()
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shell exited"), time.Now().Add(time.Second))
		writeMu.Unlock()
	case <-inputDone:
		// Client went away: end the shell so it does not linger in the sandbox.
		_, _ = hijack.Conn.Write([]byte("\x03\x04"))
		s.logger.Debug("pty client closed", zap.String("container_id", containerID))
	}
}
