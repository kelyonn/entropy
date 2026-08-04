package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/entropy/services/airlock/internal/server"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	srv := server.New(rdb, logger)
	logger.Info("Airlock starting", zap.String("port", port))
	log.Fatal(http.ListenAndServe(":"+port, srv.Routes()))
}

func redisAddr() string {
	if u := os.Getenv("REDIS_URL"); u != "" {
		opts, err := redis.ParseURL(u)
		if err != nil {
			log.Fatalf("invalid REDIS_URL %q: %v", u, err)
		}
		return opts.Addr
	}
	// Default matches Compose service name (Airlock is Compose-only by default).
	return "redis:6379"
}
