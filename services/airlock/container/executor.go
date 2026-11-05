package container

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"go.uber.org/zap"
)

type Executor struct {
	containerID string
	docker      *client.Client
	logger      *zap.Logger
}

const execPIDPrefix = "/tmp/entropy-exec-"

type StreamChunk struct {
	Stdout    string
	Stderr    string
	Completed bool
	ExitCode  int
}

func NewExecutor(containerID string, logger *zap.Logger) (*Executor, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Executor{containerID: containerID, docker: cli, logger: logger}, nil
}

func (e *Executor) RunStreaming(ctx context.Context, command string, out chan<- StreamChunk) error {
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pidFile := execPIDPrefix + strconv.FormatInt(time.Now().UnixNano(), 10) + ".pid"
	// Job control (set -m) gives the command its own process group, so an
	// interrupt or timeout reaches every process it started. It also keeps
	// SIGINT deliverable: without job control, bash starts background jobs
	// with SIGINT ignored, which would make Ctrl+C a no-op. Job control is
	// switched off again right after launch so no "[1]+ Done" notices leak
	// into the output.
	wrapper := `set -m; "$@" & child=$!; set +m; printf '%s' "$child" > "$ENTROPY_PID_FILE"; wait "$child"`
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd: []string{
			"/bin/bash", "-c", wrapper, "entropy-command",
			"/bin/bash", "-c", command,
		},
		Env: []string{"ENTROPY_PID_FILE=" + pidFile},
		Tty: false,
	}

	execID, err := e.docker.ContainerExecCreate(execCtx, e.containerID, execConfig)
	if err != nil {
		return fmt.Errorf("exec create failed: %w", err)
	}

	resp, err := e.docker.ContainerExecAttach(execCtx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return fmt.Errorf("exec attach failed: %w", err)
	}
	defer resp.Close()

	stdoutPipe, stdoutWriter := io.Pipe()
	stderrPipe, stderrWriter := io.Pipe()

	emit := func(chunk StreamChunk) {
		select {
		case out <- chunk:
		case <-execCtx.Done():
		}
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := stdoutPipe.Read(buf)
			if n > 0 {
				emit(StreamChunk{Stdout: string(buf[:n])})
			}
			if readErr != nil {
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := stderrPipe.Read(buf)
			if n > 0 {
				emit(StreamChunk{Stderr: string(buf[:n])})
			}
			if readErr != nil {
				return
			}
		}
	}()

	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := stdcopy.StdCopy(stdoutWriter, stderrWriter, resp.Reader)
		_ = stdoutWriter.Close()
		_ = stderrWriter.Close()
		copyDone <- copyErr
	}()

	timedOut := false
	select {
	case copyErr := <-copyDone:
		if copyErr != nil && copyErr != io.EOF {
			e.logger.Debug("exec copy ended", zap.Error(copyErr))
		}
	case <-execCtx.Done():
		timedOut = true
		resp.Close()
		e.killTimedOutExec(context.Background(), pidFile)
		out <- StreamChunk{
			Stderr:    "command timeout after 30s\n",
			Completed: true,
			ExitCode:  124,
		}
		return fmt.Errorf("command timeout after 30s")
	}

	exitCode := -1
	if !timedOut {
		inspect, err := e.docker.ContainerExecInspect(context.Background(), execID.ID)
		if err == nil {
			exitCode = inspect.ExitCode
		}
		e.removePIDFile(context.Background(), pidFile)
	}
	emit(StreamChunk{Completed: true, ExitCode: exitCode})
	return nil
}

func (e *Executor) killTimedOutExec(ctx context.Context, pidFile string) {
	e.signalPIDFile(ctx, pidFile, "TERM")
	time.Sleep(5 * time.Second)
	e.signalPIDFile(ctx, pidFile, "KILL")
	e.removePIDFile(ctx, pidFile)
}

func (e *Executor) signalPIDFile(ctx context.Context, pidFile, signal string) {
	script := `pid=$(cat "$1" 2>/dev/null) || exit 0; kill -` + signal + ` -- "-$pid" 2>/dev/null || kill -` + signal + ` -- "$pid" 2>/dev/null || true`
	e.runControlExec(ctx, []string{"/bin/bash", "-c", script, "entropy-signal", pidFile})
}

// SignalAll delivers signal to the process group of every running exec in
// the container. It backs terminal Ctrl+C. A program that traps SIGINT and
// keeps running gets SIGTERM one second later, as a second Ctrl+C would.
func (e *Executor) SignalAll(ctx context.Context, signal string) {
	script := `send() {
  for f in ` + execPIDPrefix + `*.pid; do
    [ -f "$f" ] || continue
    pid=$(cat "$f" 2>/dev/null) || continue
    kill -"$1" -- "-$pid" 2>/dev/null || kill -"$1" -- "$pid" 2>/dev/null || true
  done
}
send ` + signal + `
if [ ` + signal + ` = INT ]; then sleep 1; send TERM; fi`
	e.runControlExec(ctx, []string{"/bin/bash", "-c", script})
}

// NormalizeSignal maps SIGINT/INT/int style names to the short form kill
// accepts and rejects anything outside the allowed set.
func NormalizeSignal(sig string) (string, bool) {
	s := strings.ToUpper(strings.TrimSpace(sig))
	s = strings.TrimPrefix(s, "SIG")
	switch s {
	case "INT", "TERM", "KILL", "HUP", "QUIT":
		return s, true
	case "":
		return "INT", true
	}
	return "", false
}

func (e *Executor) removePIDFile(ctx context.Context, pidFile string) {
	e.runControlExec(ctx, []string{"/bin/rm", "-f", pidFile})
}

func (e *Executor) runControlExec(ctx context.Context, cmd []string) {
	control, err := e.docker.ContainerExecCreate(ctx, e.containerID, types.ExecConfig{Cmd: cmd})
	if err != nil {
		e.logger.Warn("timeout control exec create failed", zap.Error(err))
		return
	}
	if err := e.docker.ContainerExecStart(ctx, control.ID, types.ExecStartCheck{}); err != nil {
		e.logger.Warn("timeout control exec start failed", zap.Error(err))
	}
}
