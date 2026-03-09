package handlers

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/entropy/backend/ai"
    "github.com/entropy/backend/container"
    "github.com/entropy/backend/middleware"
    "github.com/google/uuid"
    "github.com/gorilla/websocket"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        return true // restrict in production
    },
}

type WSMessage struct {
    Type      string `json:"type"`       // "command" | "ping"
    Command   string `json:"command"`
    SessionID string `json:"session_id"`
}

type WSResponse struct {
    Type    string `json:"type"`    // "output" | "crash" | "ai" | "restore_ok"
    Data    string `json:"data"`
    IsError bool   `json:"is_error"`
}

type WebSocketHandler struct {
    rdb    *redis.Client
    logger *zap.Logger
}

func NewWebSocketHandler(rdb *redis.Client, logger *zap.Logger) *WebSocketHandler {
    return &WebSocketHandler{rdb: rdb, logger: logger}
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

    // Subscribe to Redis Pub/Sub for multi-instance sync
    pubsub := h.rdb.Subscribe(ctx, fmt.Sprintf("session:%s", sessionID))
    defer pubsub.Close()

    // Forward Redis Pub/Sub messages to WebSocket
    go func() {
        ch := pubsub.Channel()
        for msg := range ch {
            conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
        }
    }()

    // Spawn container for this session
    orch := container.NewOrchestrator(h.rdb, h.logger)
    containerID, err := orch.SpawnContainer(ctx, sessionID)
    if err != nil {
        h.logger.Error("Container spawn failed", zap.Error(err))
        return
    }
    defer orch.DeleteContainer(ctx, sessionID, containerID)

    executor := container.NewExecutor(containerID, h.logger)
    rateLimiter := middleware.NewRateLimiter(h.rdb)
    aiClient := ai.NewBedrockClient(h.rdb, h.logger)

    // Main message loop
    for {
        _, rawMsg, err := conn.ReadMessage()
        if err != nil {
            h.logger.Info("WebSocket closed", zap.String("session", sessionID))
            break
        }

        var msg WSMessage
        if err := json.Unmarshal(rawMsg, &msg); err != nil {
            continue
        }

        if msg.Type == "ping" {
            conn.WriteJSON(WSResponse{Type: "pong"})
            continue
        }

        // Store command in Redis history
        h.rdb.LPush(ctx, fmt.Sprintf("history:%s", sessionID), msg.Command)
        h.rdb.LTrim(ctx, fmt.Sprintf("history:%s", sessionID), 0, 99)
        h.rdb.Incr(ctx, "metric:total_commands")

        // Execute command
        output, exitCode, stderr, err := executor.Run(ctx, msg.Command)

        // Stream stdout to frontend
        conn.WriteJSON(WSResponse{
            Type: "output",
            Data: output,
        })

        // Crash detected
        if exitCode != 0 || err != nil {
            h.rdb.Incr(ctx, "metric:total_crashes")

            conn.WriteJSON(WSResponse{
                Type:    "crash",
                Data:    stderr,
                IsError: true,
            })

            // Rate limit AI calls
            if !rateLimiter.Allow(ctx, sessionID) {
                conn.WriteJSON(WSResponse{
                    Type: "ai",
                    Data: "Rate limit reached. Try again in 1 hour.",
                })
                continue
            }

            // Stream AI explanation
            conn.WriteJSON(WSResponse{
                Type: "ai",
                Data: "🤖 Analyzing crash...",
            })

            explanation, err := aiClient.ExplainCrash(ctx, msg.Command, stderr, exitCode)
            if err != nil {
                h.logger.Error("Bedrock call failed", zap.Error(err))
                continue
            }

            conn.WriteJSON(WSResponse{
                Type: "ai",
                Data: explanation,
            })

            h.rdb.Incr(ctx, "metric:ai_explanations")
        }

        // Refresh session TTL on every command
        h.rdb.Expire(ctx, fmt.Sprintf("session:%s", sessionID), 5*time.Minute)
    }
}
