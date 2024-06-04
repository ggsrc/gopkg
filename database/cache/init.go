package cache

import (
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/stumble/dcache"
)

func InitCache(appName string) (redis.UniversalClient, *dcache.DCache, error) {
	// create redis connection
	redisConn := NewRedisClient("redis")
	// create dcache instance
	dCache, err := NewDCache(appName, "dcache", redisConn)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dcache: %w", err)
	}
	return redisConn, dCache, nil
}
