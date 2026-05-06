package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewClient parses REDIS_URL and returns a configured go-redis client.
// URL format: redis://:password@host:port/db
func NewClient(redisURL string) *redis.Client {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		panic(fmt.Sprintf("redis.NewClient: invalid URL %q: %v", redisURL, err))
	}

	opts.PoolSize = 20
	opts.MinIdleConns = 5

	rdb := redis.NewClient(opts)

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		panic(fmt.Sprintf("redis.NewClient: ping failed: %v", err))
	}

	return rdb
}
