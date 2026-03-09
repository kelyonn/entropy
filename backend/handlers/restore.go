package handlers

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os/exec"

    "github.com/entropy/backend/container"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
)

type RestoreHandler struct {
    rdb    *redis.Client
    logger *zap.Logger
}

func NewRestoreHandler(rdb *redis.Client, logger *zap.Logger) *RestoreHandler {
    return &RestoreHandler{rdb: rdb, logger: logger}
}

func (h *RestoreHandler) Restore(w http.ResponseWriter, r *http.Request) {
    ctx := context.Background()
    sessionID := r.URL.Query().Get("session_id")
    if sessionID == "" {
        http.Error(w, "session_id required", http.StatusBadRequest)
        return
    }

    // Step 1: Fetch deleted container metadata from Redis
    key := fmt.Sprintf("container:deleted:%s", sessionID)
    metadata, err := h.rdb.HGetAll(ctx, key).Result()
    if err != nil || len(metadata) == 0 {
        http.Error(w, "session not found or expired", http.StatusNotFound)
        return
    }

    h.logger.Info("Restoring session", zap.String("session_id", sessionID))

    // Step 2: Spawn fresh container
    orch := container.NewOrchestrator(h.rdb, h.logger)
    newContainerID, err := orch.SpawnContainer(ctx, sessionID)
    if err != nil {
        http.Error(w, "failed to spawn container", http.StatusInternalServerError)
        return
    }

    // Step 3: Restore file tree
    if fileTreeJSON, ok := metadata["file_tree"]; ok {
        var fileTree map[string][]byte
        if err := json.Unmarshal([]byte(fileTreeJSON), &fileTree); err == nil {
            for path, content := range fileTree {
                cmd := exec.Command("docker", "cp", "-",
                    fmt.Sprintf("%s:%s", newContainerID, path))
                cmd.Stdin = bytes.NewReader(content)
                cmd.Run()
            }
        }
    }

    // Step 4: Restore command history to Redis
    if historyJSON, ok := metadata["command_history"]; ok {
        var history []string
        if err := json.Unmarshal([]byte(historyJSON), &history); err == nil {
            histKey := fmt.Sprintf("history:%s", sessionID)
            for _, cmd := range history {
                h.rdb.LPush(ctx, histKey, cmd)
            }
        }
    }

    // Step 5: Return new WebSocket URL
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "status":       "restored",
        "session_id":   sessionID,
        "container_id": newContainerID,
        "ws_url":       "wss://api.entropy.dev/ws?session_id=" + sessionID,
    })
}
