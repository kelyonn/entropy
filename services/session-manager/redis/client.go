package redis

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

func NewClient(url string) *redis.Client {
	if url == "" {
		panic("REDIS_URL is required (Compose uses redis://redis:6379; host Redis is not published)")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		panic(fmt.Sprintf("invalid REDIS_URL %q: %v", url, err))
	}
	return redis.NewClient(opts)
}
