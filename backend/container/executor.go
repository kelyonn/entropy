package container

import (
    "bytes"
    "context"
    "fmt"

    "github.com/docker/docker/api/types"
    "github.com/docker/docker/client"
    "go.uber.org/zap"
)

type Executor struct {
    containerID string
    docker      *client.Client
    logger      *zap.Logger
}

func NewExecutor(containerID string, logger *zap.Logger) *Executor {
    cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    return &Executor{containerID: containerID, docker: cli, logger: logger}
}

func (e *Executor) Run(ctx context.Context, command string) (string, int, string, error) {
    execConfig := types.ExecConfig{
        AttachStdout: true,
        AttachStderr: true,
        Cmd:          []string{"/bin/bash", "-c", command},
        Tty:          false,
    }

    execID, err := e.docker.ContainerExecCreate(ctx, e.containerID, execConfig)
    if err != nil {
        return "", -1, "", fmt.Errorf("exec create failed: %w", err)
    }

    resp, err := e.docker.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
    if err != nil {
        return "", -1, "", fmt.Errorf("exec attach failed: %w", err)
    }
    defer resp.Close()

    var stdout, stderr bytes.Buffer
    // Read output (Docker multiplexes stdout/stderr)
    buf := make([]byte, 4096)
    for {
        n, err := resp.Reader.Read(buf)
        if n > 0 {
            // First byte is stream type: 1=stdout, 2=stderr
            if buf[0] == 2 {
                stderr.Write(buf[8:n])
            } else {
                stdout.Write(buf[8:n])
            }
        }
        if err != nil {
            break
        }
    }

    // Get exit code
    inspect, err := e.docker.ContainerExecInspect(ctx, execID.ID)
    if err != nil {
        return stdout.String(), -1, stderr.String(), err
    }

    return stdout.String(), inspect.ExitCode, stderr.String(), nil
}