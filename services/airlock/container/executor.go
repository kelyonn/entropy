package container

import (
	"context"
	"fmt"
	"io"
	"strconv"
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

	pidFile := "/tmp/entropy-exec-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".pid"
	wrapper := `"$@" & child=$!; printf '%s' "$child" > "$ENTROPY_PID_FILE"; wait "$child"`
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
	script := `pid=$(cat "$1" 2>/dev/null) || exit 0; kill -` + signal + ` -- "$pid" 2>/dev/null || true`
	e.runControlExec(ctx, []string{"/bin/sh", "-c", script, "entropy-signal", pidFile})
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
