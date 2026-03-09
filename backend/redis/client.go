package redis

import (
    "github.com/redis/go-redis/v9"
)

func NewClient(url string) *redis.Client {
    opts, err := redis.ParseURL(url)
    if err != nil {
        // fallback for local dev
        opts = &redis.Options{
            Addr:     "localhost:6379",
            Password: "",
            DB:       0,
        }
    }
    return redis.NewClient(opts)
}
