package handlers

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/google/uuid"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
)

type SessionHandler struct {
    rdb    *redis.Client
    logger *zap.Logger
}

func NewSessionHandler(rdb *redis.Client, logger *zap.Logger) *SessionHandler {
    return &SessionHandler{rdb: rdb, logger: logger}
}

type SessionData struct {
    UserID      string    `json:"user_id"`
    ContainerID string    `json:"container_id"`
    CreatedAt   time.Time `json:"created_at"`
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
    ctx := context.Background()
    sessionID := uuid.New().String()

    session := SessionData{
        UserID:    uuid.New().String(),
        CreatedAt: time.Now(),
    }

    data, _ := json.Marshal(session)
    h.rdb.Set(ctx, "session:"+sessionID, data, 5*time.Minute)
    h.rdb.Incr(ctx, "metric:total_sessions")

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "session_id": sessionID,
        "ws_url":     "wss://api.entropy.dev/ws?session_id=" + sessionID,
    })
}

func (h *SessionHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
    ctx := context.Background()

    keys := []string{
        "metric:total_sessions",
        "metric:total_commands",
        "metric:total_crashes",
        "metric:ai_explanations",
    }

    result := map[string]string{}
    for _, key := range keys {
        val, _ := h.rdb.Get(ctx, key).Result()
        result[key] = val
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}
