package middleware

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

type RateLimiter struct {
    rdb *redis.Client
}

func NewRateLimiter(rdb *redis.Client) *RateLimiter {
    return &RateLimiter{rdb: rdb}
}

func (rl *RateLimiter) Allow(ctx context.Context, sessionID string) bool {
    key := fmt.Sprintf("ratelimit:%s", sessionID)
    now := time.Now().Unix()
    windowStart := now - 3600 // 1 hour

    pipe := rl.rdb.Pipeline()
    pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
    countCmd := pipe.ZCard(ctx, key)
    pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: now})
    pipe.Expire(ctx, key, time.Hour)
    pipe.Exec(ctx)

    return countCmd.Val() < 20
}
