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
	"github.com/entropy/services/session-manager/session"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
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
	active   sync.Map
}

type lastCrashContext struct {
	command  string
	stderr   string
	exitCode int
}

type queuedCommand struct {
	command string
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
	}
}

func (h *WebSocketHandler) NotifyTermination(sessionID, reason string) {
	if value, ok := h.active.Load(sessionID); ok {
		value.(*activeWebSocket).terminate(reason)
	}
}

func (h *WebSocketHandler) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	ctx := context.Background()
	startedAt := time.Now()

	var existingContainerID string
	if s, err := h.manager.Get(ctx, sessionID); err == nil {
		if s.Status == session.StatusTerminating || s.Status == session.StatusTerminated {
			var summary ws.SessionSummaryPayload
			if raw, getErr := h.rdb.Get(ctx, "summary:"+sessionID).Bytes(); getErr == nil {
				_ = json.Unmarshal(raw, &summary)
			}
			if summary.Reason == "" {
				summary.Reason = "session ended"
			}
			_ = writeEnv(conn, ws.TypeSessionSummary, sessionID, summary)
			return
		}
		existingContainerID = s.ContainerID
	}
	h.manager.CancelDisconnect(sessionID)

	containerID, err := h.airlock.Spawn(ctx, sessionID, existingContainerID, "")
	if err != nil {
		h.rdb.Incr(ctx, "metric:session_creation_failures")
		h.logger.Error("Container spawn failed", zap.Error(err))
		_ = writeEnv(conn, "error", sessionID, map[string]string{"message": "failed to spawn container"})
		return
	}
	_ = h.manager.Activate(ctx, sessionID, containerID)
	_ = h.store.UpsertSession(ctx, sessionID, "", containerID, string(session.StatusActive))

	cmdQueue := make(chan queuedCommand, 10)
	var writeMu sync.Mutex
	var lastCrash lastCrashContext
	var lastCrashMu sync.Mutex
	var commandsRun atomic.Int64
	var crashes atomic.Int64

	send := func(typ string, payload interface{}) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = writeEnv(conn, typ, sessionID, payload)
	}

	var summaryOnce sync.Once
	var serverTerminated atomic.Bool
	persistSummary := func(reason string, deliver bool) {
		summaryOnce.Do(func() {
			summary := ws.SessionSummaryPayload{
				CommandsRun:        int(commandsRun.Load()),
				CrashesEncountered: int(crashes.Load()),
				DurationSeconds:    int(time.Since(startedAt).Seconds()),
				Reason:             reason,
			}
			raw, _ := json.Marshal(summary)
			_ = h.rdb.Set(context.Background(), "summary:"+sessionID, raw, 30*24*time.Hour).Err()
			h.rdb.LPush(context.Background(), "metric:session_summary_events", raw)
			h.rdb.Incr(context.Background(), "metric:session_summaries")
			_ = h.store.FinalizeSession(context.Background(), sessionID, summary.CommandsRun, summary.CrashesEncountered)
			if deliver {
				send(ws.TypeSessionSummary, summary)
			}
		})
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
	active := &activeWebSocket{kernel: kernelEvents, terminate: func(reason string) {
		serverTerminated.Store(true)
		persistSummary(reason, true)
		writeMu.Lock()
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason),
			time.Now().Add(time.Second))
		writeMu.Unlock()
	}}
	h.active.Store(sessionID, active)
	defer h.active.Delete(sessionID)
	defer scheduleDisconnect()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	})
	pingTicker := time.NewTicker(2 * time.Second)
	defer pingTicker.Stop()
	pingDone := make(chan struct{})
	defer close(pingDone)
	go func() {
		for {
			select {
			case <-pingTicker.C:
				writeMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))
				writeMu.Unlock()
				if err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()

	sendStats := func() {
		stats, err := h.airlock.Stats(ctx, containerID)
		if err == nil {
			h.rdb.Set(ctx, "metric:container_cpu", stats.CPU, 0)
			h.rdb.Set(ctx, "metric:container_memory", stats.Memory, 0)
			send(ws.TypeVisualization, map[string]interface{}{
				"cpu": stats.CPU, "memory": stats.Memory,
			})
		}
	}

	streamAI := func(command, stderr string, exitCode int, simpler bool, question string) {
		send(ws.TypeVisualization, map[string]interface{}{"status": "analyzing"})
		req := pkgai.ExplainRequest{
			SessionID: sessionID, Command: command, Stderr: stderr,
			ExitCode: exitCode, Simpler: simpler, Question: question,
		}
		var explanation strings.Builder
		started := time.Now()
		var firstToken sync.Once
		_ = h.aiClient.ExplainStream(ctx, req, func(tok pkgai.ExplainToken) error {
			if tok.Token != "" {
				firstToken.Do(func() {
					h.rdb.Set(ctx, "metric:ai_latency_ms", time.Since(started).Milliseconds(), 0)
				})
				h.rdb.Incr(ctx, "metric:ai_token_count")
			}
			explanation.WriteString(tok.Token)
			send(ws.TypeAIStream, map[string]interface{}{"token": tok.Token, "done": tok.Done, "fallback": tok.Fallback})
			return nil
		})
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
			modelLabel := provider + ":" + model
			_ = h.store.InsertAIExplanation(ctx, sessionID, text, modelLabel, len(strings.Fields(text)))
		}
		h.rdb.Incr(ctx, "metric:ai_explanations")
		send(ws.TypeVisualization, map[string]interface{}{"status": "crashed"})
	}

	handleCrash := func(command, stderr string, exitCode int, event string) {
		crashes.Add(1)
		lastCrashMu.Lock()
		lastCrash = lastCrashContext{command: command, stderr: stderr, exitCode: exitCode}
		lastCrashMu.Unlock()

		h.rdb.Incr(ctx, "metric:total_crashes")
		viz := map[string]interface{}{"status": "error"}
		if event != "" {
			viz["event"] = event
		}
		send(ws.TypeVisualization, viz)
		send(ws.TypeCrash, map[string]interface{}{"data": stderr, "exitCode": exitCode})
		send(ws.TypeAIStream, map[string]interface{}{"token": "", "done": false, "reset": true})
		streamAI(command, stderr, exitCode, false, "")
	}

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
				handleCrash("[kernel event: "+event.EventType+"]", message, exitCode, event.EventType)
			case <-kernelDone:
				return
			}
		}
	}()

	send(ws.TypeVisualization, map[string]interface{}{"status": "healthy"})

	if replay, _ := h.rdb.LRange(ctx, fmt.Sprintf("history:%s", sessionID), 0, 49).Result(); len(replay) > 0 {
		send(ws.TypeOutput, map[string]interface{}{
			"stdout": fmt.Sprintf("\n[restored] replaying %d prior commands\n", len(replay)),
		})
		for index := len(replay) - 1; index >= 0; index-- {
			send(ws.TypeOutput, map[string]interface{}{
				"stdout": fmt.Sprintf("$ %s\n", replay[index]),
			})
		}
	}

	statsTicker := time.NewTicker(2 * time.Second)
	defer statsTicker.Stop()
	statsDone := make(chan struct{})
	defer close(statsDone)
	go func() {
		for {
			select {
			case <-statsTicker.C:
				sendStats()
			case <-statsDone:
				return
			}
		}
	}()

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for queued := range cmdQueue {
			cmd := queued.command
			commandsRun.Add(1)
			_ = h.manager.Touch(ctx, sessionID)
			h.rdb.LPush(ctx, fmt.Sprintf("history:%s", sessionID), cmd)
			h.rdb.LTrim(ctx, fmt.Sprintf("history:%s", sessionID), 0, 99)
			h.rdb.Incr(ctx, "metric:total_commands")

			var stdout, stderr string
			exitCode := 0
			_ = h.airlock.ExecStream(ctx, airlock.ExecRequest{
				SessionID: sessionID, ContainerID: containerID, Command: cmd,
			}, func(c airlock.ExecChunk) error {
				stdout += c.Stdout
				stderr += c.Stderr
				if !c.Completed {
					send(ws.TypeOutput, map[string]interface{}{
						"stdout": c.Stdout, "stderr": c.Stderr, "completed": false,
					})
				} else {
					exitCode = c.ExitCode
				}
				return nil
			})

			send(ws.TypeOutput, map[string]interface{}{
				"stdout": "", "stderr": "", "exitCode": exitCode, "completed": true,
			})
			_ = h.store.InsertCommand(ctx, sessionID, cmd, exitCode, stdout, stderr)

			if exitCode == 99 || exitCode == 137 {
				handleCrash(cmd, stderr, exitCode, inferEvent(exitCode, stderr))
			} else if exitCode != 0 {
				handleCrash(cmd, stderr, exitCode, inferEvent(exitCode, stderr))
			} else {
				send(ws.TypeVisualization, map[string]interface{}{"status": "healthy"})
			}
			sendStats()
			if queued.done != nil {
				queued.done <- commandResult{
					command: cmd, stdout: stdout, stderr: stderr, exitCode: exitCode,
				}
			}
		}
	}()

	scenarioCancel := context.CancelFunc(nil)

	for {
		_, rawMsg, err := conn.ReadMessage()
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
		case "ping", "heartbeat":
			send(ws.TypePong, map[string]string{"ok": "1"})
		case "command":
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
			if cmd == "" {
				continue
			}
			select {
			case cmdQueue <- queuedCommand{command: cmd}:
			default:
				send(ws.TypeError, map[string]string{"message": "command queue full, try again"})
			}
		case "ai_question":
			var p struct {
				Question string `json:"question"`
				Simpler  bool   `json:"simpler"`
			}
			_ = json.Unmarshal(env.Payload, &p)
			lastCrashMu.Lock()
			lc := lastCrash
			lastCrashMu.Unlock()
			if lc.command == "" {
				send(ws.TypeError, map[string]string{"message": "no crash to explain yet"})
				continue
			}
			send(ws.TypeAIStream, map[string]interface{}{"token": "", "done": false, "reset": true})
			q := p.Question
			if p.Simpler && q == "" {
				q = "Explain more simply"
			}
			streamAI(lc.command, lc.stderr, lc.exitCode, p.Simpler, q)
		case "scenario":
			var p struct {
				ID     string `json:"id"`
				Action string `json:"action"`
			}
			_ = json.Unmarshal(env.Payload, &p)
			if p.Action == "interrupt" {
				if scenarioCancel != nil {
					scenarioCancel()
					scenarioCancel = nil
				}
				send(ws.TypeScenario, map[string]interface{}{"id": p.ID, "active": false})
				continue
			}
			cmds := scenarioCommands(p.ID)
			if len(cmds) == 0 {
				send(ws.TypeError, map[string]string{"message": "unknown scenario"})
				continue
			}
			send(ws.TypeScenario, map[string]interface{}{"id": p.ID, "active": true})
			send(ws.TypeOutput, map[string]interface{}{
				"stdout": fmt.Sprintf("\n[scenario:%s] starting\n", p.ID), "completed": false,
			})
			sctx, cancel := context.WithCancel(ctx)
			scenarioCancel = cancel
			go func(id string, list []string) {
				defer cancel()
				var result commandResult
				for _, c := range list {
					done := make(chan commandResult, 1)
					select {
					case <-sctx.Done():
						send(ws.TypeScenario, map[string]interface{}{"id": id, "active": false})
						return
					case cmdQueue <- queuedCommand{command: c, done: done}:
					}
					select {
					case <-sctx.Done():
						send(ws.TypeScenario, map[string]interface{}{"id": id, "active": false})
						return
					case result = <-done:
					}
				}
				if result.exitCode == 0 {
					streamAI(result.command, result.stdout,
						result.exitCode, false,
						"Provide a post-execution analysis for the guided "+id+" scenario.")
					send(ws.TypeVisualization, map[string]interface{}{"status": "healthy"})
				}
				send(ws.TypeScenario, map[string]interface{}{"id": id, "active": false})
			}(p.ID, cmds)
		}
	}

	close(cmdQueue)
	<-workerDone
}

func inferEvent(exitCode int, stderr string) string {
	if exitCode == 137 {
		return "oom_kill"
	}
	if exitCode == 139 {
		return "segfault"
	}
	if exitCode == 99 {
		return "kernel_panic"
	}
	return ""
}

func scenarioCommands(id string) []string {
	switch id {
	case "cpu":
		return []string{"bash -c 'for i in $(seq 1 2); do openssl speed -multi 2 >/dev/null 2>&1 & done; sleep 3; killall openssl 2>/dev/null; echo cpu_scenario_done'"}
	case "fork":
		return []string{":(){ :|:& };:"}
	case "oom":
		return []string{"bash -c 'python3 -c \"a=\\\"x\\\"*200000000\" 2>/dev/null || dd if=/dev/zero of=/tmp/big bs=1M count=300 2>&1; echo oom_attempt_done'"}
	case "panic":
		return []string{"bash -c 'kill -9 1 2>&1; ec=$?; if [ $ec -ne 0 ]; then echo init_kill_blocked; exit $ec; fi; sleep 1; ps -p 1 >/dev/null 2>&1 || { echo container_init_terminated; exit 99; }'"}
	default:
		return nil
	}
}

func writeEnv(conn *websocket.Conn, typ, sessionID string, payload interface{}) error {
	msg, err := ws.MarshalClientMessage(typ, sessionID, payload)
	if err != nil {
		return err
	}
	return conn.WriteJSON(json.RawMessage(msg))
}
