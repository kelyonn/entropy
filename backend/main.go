package main

import (
    "context"
    "log"
    "net/http"
    "os"

    "github.com/entropy/backend/handlers"
    "github.com/entropy/backend/redis"
    "github.com/rs/cors"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    // Init Redis
    rdb := redis.NewClient(os.Getenv("REDIS_URL"))
    ctx := context.Background()
    if err := rdb.Ping(ctx).Err(); err != nil {
        log.Fatalf("Redis connection failed: %v", err)
    }
    logger.Info("Redis connected")

    // Init handlers
    wsHandler := handlers.NewWebSocketHandler(rdb, logger)
    sessionHandler := handlers.NewSessionHandler(rdb, logger)
    restoreHandler := handlers.NewRestoreHandler(rdb, logger)

    // Routes
    mux := http.NewServeMux()
    mux.HandleFunc("/ws", wsHandler.Handle)
    mux.HandleFunc("/api/session/new", sessionHandler.Create)
    mux.HandleFunc("/api/session/restore", restoreHandler.Restore)
    mux.HandleFunc("/api/dashboard", sessionHandler.Dashboard)
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("ok"))
    })

    // CORS for React frontend
    handler := cors.New(cors.Options{
        AllowedOrigins: []string{
            "http://localhost",
            "http://localhost:80",
            "http://127.0.0.1",
            "http://127.0.0.1:80",
            "http://localhost:5173",
            "http://127.0.0.1:5173",
            "https://entropy.dev",
        },
        AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
        AllowCredentials: true,
    }).Handler(mux)

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    logger.Info("Entropy backend starting", zap.String("port", port))
    log.Fatal(http.ListenAndServe(":"+port, handler))
}
