package container

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "time"

    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/client"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
)

type Orchestrator struct {
    docker *client.Client
    rdb    *redis.Client
    logger *zap.Logger
}

func NewOrchestrator(rdb *redis.Client, logger *zap.Logger) *Orchestrator {
    cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    return &Orchestrator{docker: cli, rdb: rdb, logger: logger}
}

func (o *Orchestrator) SpawnContainer(ctx context.Context, sessionID string) (string, error) {
    resp, err := o.docker.ContainerCreate(ctx,
        &container.Config{
            Image:      "ubuntu:22.04",
            Cmd:        []string{"/bin/bash"},
            Tty:        true,
            OpenStdin:  true,
            WorkingDir: "/sandbox",
            Env: []string{
                "ENTROPY_SESSION=" + sessionID,
            },
        },
        &container.HostConfig{
            Resources: container.Resources{
                Memory:   256 * 1024 * 1024, // 256MB
                NanoCPUs: 500000000,          // 0.5 CPU
            },
            NetworkMode: "bridge",
            AutoRemove:  false,
        },
        nil, nil, "entropy-"+sessionID[:8],
    )
    if err != nil {
        return "", err
    }

    if err := o.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
        return "", err
    }

    o.logger.Info("Container spawned",
        zap.String("container_id", resp.ID),
        zap.String("session_id", sessionID))

    return resp.ID, nil
}

func (o *Orchestrator) DeleteContainer(ctx context.Context, sessionID, containerID string) {
    // Save metadata BEFORE deletion
    o.saveMetadata(ctx, sessionID, containerID)

    // Enqueue async snapshot job
    job, _ := json.Marshal(map[string]string{
        "session_id":   sessionID,
        "container_id": containerID,
        "timestamp":    time.Now().Format(time.RFC3339),
    })
    o.rdb.LPush(ctx, "jobs:snapshots", job)

    // Remove container
    o.docker.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
    o.logger.Info("Container removed", zap.String("container_id", containerID))
}

func (o *Orchestrator) saveMetadata(ctx context.Context, sessionID, containerID string) {
    key := fmt.Sprintf("container:deleted:%s", sessionID)

    // Capture file tree from /sandbox
    fileTree := o.captureFileTree(ctx, containerID)
    fileTreeJSON, _ := json.Marshal(fileTree)

    // Fetch command history from Redis
    history, _ := o.rdb.LRange(ctx,
        fmt.Sprintf("history:%s", sessionID), 0, 99).Result()
    historyJSON, _ := json.Marshal(history)

    o.rdb.HSet(ctx, key,
        "container_id", containerID,
        "file_tree", string(fileTreeJSON),
        "command_history", string(historyJSON),
        "deleted_at", time.Now().Format(time.RFC3339),
    )

    // Keep metadata for 30 days
    o.rdb.Expire(ctx, key, 30*24*time.Hour)

    o.logger.Info("Container metadata saved to Redis",
        zap.String("session_id", sessionID))
}

func (o *Orchestrator) captureFileTree(ctx context.Context, containerID string) map[string]string {
    fileTree := map[string]string{}

    reader, _, err := o.docker.CopyFromContainer(ctx, containerID, "/sandbox")
    if err != nil {
        return fileTree
    }
    defer reader.Close()

    content, err := io.ReadAll(reader)
    if err != nil {
        return fileTree
    }

    fileTree["/sandbox"] = string(content)
    return fileTree
}
