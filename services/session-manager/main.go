package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/entropy/pkg/airlock"
	"github.com/entropy/pkg/db"
	"github.com/entropy/services/session-manager/handlers"
	"github.com/entropy/services/session-manager/metrics"
	smware "github.com/entropy/services/session-manager/middleware"
	"github.com/entropy/services/session-manager/redis"
	"github.com/entropy/services/session-manager/session"
	"github.com/entropy/services/session-manager/storage"
	"github.com/rs/cors"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	rdb := redis.NewClient(os.Getenv("REDIS_URL"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}
	logger.Info("Redis connected")
	metrics.NewExporter(ctx, rdb, logger).Start(ctx)

	store, err := db.Connect(logger)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	storageClient := storage.New(logger)

	airURL := os.Getenv("AIRLOCK_URL")
	if airURL == "" {
		airURL = "http://airlock:8081"
	}
	airClient := airlock.NewClient(airURL)

	manager := session.NewManager(rdb, logger)
	wsHandler := handlers.NewWebSocketHandler(rdb, logger, manager, store)
	manager.SetTerminateHook(func(c context.Context, s session.Session, reason string) {
		wsHandler.NotifyTermination(s.ID, reason)
		_ = airClient.Stop(c, airlock.StopRequest{
			SessionID: s.ID, ContainerID: s.ContainerID, GraceSec: 5,
		})
	})
	manager.StartSweeper(ctx)

	sessionHandler := handlers.NewSessionHandler(rdb, logger, manager, store)
	restoreHandler := handlers.NewRestoreHandler(rdb, logger, store, storageClient, manager)
	kernelHandler := handlers.NewKernelHandler(store, logger, wsHandler)
	labsHandler := handlers.NewLabsHandler(rdb, manager)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler.Handle)
	mux.HandleFunc("/ws/pty", wsHandler.HandlePTY)
	mux.HandleFunc("/api/session/new", sessionHandler.Create)
	mux.HandleFunc("/api/session/restore", restoreHandler.Restore)
	mux.HandleFunc("/api/session/snapshot", restoreHandler.Snapshot)
	mux.HandleFunc("/api/sessions", sessionHandler.List)
	mux.HandleFunc("/api/dashboard", sessionHandler.Dashboard)
	mux.HandleFunc("/api/labs", labsHandler.List)
	mux.HandleFunc("/internal/kernel-events", kernelHandler.Ingest)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/metrics", metrics.PrometheusHandler(rdb))

	handler := cors.New(cors.Options{
		AllowedOrigins: []string{
			"http://localhost", "http://localhost:80", "http://localhost:3000",
			"http://127.0.0.1", "http://127.0.0.1:80", "http://127.0.0.1:3000",
			"http://localhost:5173", "http://127.0.0.1:5173",
			"http://localhost:5175", "http://127.0.0.1:5175",
			"https://entropy.dev",
		},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	}).Handler(smware.AuthMiddleware(mux))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: ":" + port, Handler: handler}
	go func() {
		logger.Info("Session Manager starting", zap.String("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()
	_ = server.Shutdown(context.Background())
}
