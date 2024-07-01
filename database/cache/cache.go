package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	minSleep = 50 * time.Millisecond
)

var (
	// ErrTimeout is get from cache timeout error
	ErrTimeout = errors.New("timeout")
)

// PassThroughFunc is the actual call to underlying data source
type PassThroughFunc = func() (interface{}, error)

// PassThroughExpireFunc is the actual call to underlying data source while
// returning a duration as expire timer
type PassThroughExpireFunc = func() (interface{}, time.Duration, error)

// Cache defines interface to cache
type Cache interface {
	// Get returns value of f while caching in redis
	// Inputs:
	// queryKey	 - key used in cache
	// target	 - receive the cached value, must be pointer
	// expire 	 - expiration of cache key
	// f		 - actual call that hits underlying data source
	// noCache 	 - whether force read from data source
	Get(ctx context.Context, queryKey string, target interface{}, expire time.Duration, f PassThroughFunc) error

	// GetWithExpire returns value of f while caching in redis
	// Inputs:
	// queryKey	 - key used in cache
	// target	 - receive the cached value, must be pointer
	// f		 - actual call that hits underlying data source, sets expire duration
	// noCache 	 - whether force read from data source
	GetWithExpire(ctx context.Context, queryKey string, target interface{}, f PassThroughExpireFunc) error

	// Set explicitly set a cache key to a val
	// Inputs:
	// key	  - key to set
	// val	  - val to set
	// ttl    - ttl of key
	Set(ctx context.Context, key string, val interface{}, ttl time.Duration) error

	// Invalidate explicitly invalidates a cache key
	// Inputs:
	// key    - key to invalidate
	Invalidate(ctx context.Context, keys ...string) error
}

// Client captures redis connection
type Client struct {
	primaryConn            redis.UniversalClient
	readThroughPerKeyLimit time.Duration
	maxWaitingTime         time.Duration
}

// NewCache creates a new redis cache
func NewCache(
	primaryClient redis.UniversalClient,
	readThroughPerKeyLimit, maxWaitingTime time.Duration,
) (Cache, error) {
	c := &Client{
		primaryConn:            primaryClient,
		readThroughPerKeyLimit: readThroughPerKeyLimit,
		maxWaitingTime:         maxWaitingTime,
	}
	return c, nil
}

// getNoCache read through using f and populate cache if no error
func (c *Client) getNoCache(ctx context.Context, queryKey string, f PassThroughExpireFunc, v interface{}) error {
	dbres, expire, err := f()
	if err != nil {
		// clear lock key, make other go routine to get lock and set value
		e := c.invalidKey(ctx, lock(queryKey))
		if e != nil {
			log.Err(e).Str("key", queryKey).Str("funcErr", err.Error()).Msg("failed to invalidate cache")
		}
		return err
	}

	bs, e := marshal(dbres)
	if e != nil {
		return e
	}
	e = c.primaryConn.Set(ctx, store(queryKey), bs, expire).Err()
	if e != nil {
		log.Err(e).Str("key", queryKey).Msg("failed to set cache")
		// It doesn't need clear lock key, if it fails to set cache,
		// lock key maybe not set successfully
	}
	e = unmarshal(bs, v)
	if e != nil {
		return e
	}
	return nil
}

func store(key string) string {
	return fmt.Sprintf("#%s#", key)
}

func lock(key string) string {
	return fmt.Sprintf("#%s#_lock", key)
}

// Get implements Cache interface
func (c *Client) Get(ctx context.Context, queryKey string, target interface{}, expire time.Duration, f PassThroughFunc) error {
	var fn PassThroughExpireFunc = func() (interface{}, time.Duration, error) {
		res, err := f()
		return res, expire, err
	}
	return c.GetWithExpire(ctx, queryKey, target, fn)
}

// GetWithExpire gets cache result, and uses pass through func to set expire time
func (c *Client) GetWithExpire(ctx context.Context, queryKey string, target interface{}, f PassThroughExpireFunc) error {
	var waitCtx context.Context
	var waitCtxCancelFunc context.CancelFunc
retry:
	res, e := c.primaryConn.Get(ctx, store(queryKey)).Result()
	// not found
	if e != nil {
		if e != redis.Nil {
			log.Err(e).Str("key", queryKey).Msg("failed to get from cache")
			return c.getNoCache(ctx, queryKey, f, target)
		}

		// Empty cache, obtain lock first to query db
		// If timeout or not cacheable error, another thread will obtain lock after ratelimit
		updated, err := c.primaryConn.SetNX(ctx, lock(queryKey), "", c.readThroughPerKeyLimit).Result()
		if err != nil {
			log.Err(err).Str("key", queryKey).Msg("failed to set cache lock")
			return c.getNoCache(ctx, queryKey, f, target)
		}
		if updated {
			return c.getNoCache(ctx, queryKey, f, target)
		}
		// Did not obtain lock, sleep and retry to wait for update
		if waitCtx == nil {
			waitCtx, waitCtxCancelFunc = context.WithTimeout(ctx, c.maxWaitingTime)
			defer waitCtxCancelFunc()
		}

		select {
		case <-ctx.Done():
			return ErrTimeout
		case <-time.After(minSleep):
			goto retry
		case <-waitCtx.Done():
			// Response timeout
			return ErrTimeout
		}
	}
	e = unmarshal([]byte(res), target)
	if e != nil {
		return e
	}
	return nil
}

// Invalidate implements Cache interface
func (c *Client) Invalidate(ctx context.Context, keys ...string) error {
	var err error
	for _, key := range keys {
		err = c.invalidKey(ctx, lock(key), store(key))
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) invalidKey(ctx context.Context, keys ...string) error {
	_, err := c.primaryConn.Del(ctx, keys...).Result()
	if err != nil {
		return err
	}
	return nil
}

// Set implements Cache interface
func (c *Client) Set(ctx context.Context, key string, val interface{}, ttl time.Duration) error {
	bs, e := marshal(val)
	if e != nil {
		return e
	}
	e = c.primaryConn.Set(ctx, store(key), bs, ttl).Err()
	if e != nil {
		return e
	}
	return nil
}

// marshal copy from https://github.com/go-redis/cache/blob/v8/cache.go#L331
// remove compression
func marshal(value interface{}) ([]byte, error) {
	switch value := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return value, nil
	case string:
		return []byte(value), nil
	}

	b, err := msgpack.Marshal(value)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// unmarshal copy from https://github.com/go-redis/cache/blob/v8/cache.go#L369
func unmarshal(b []byte, value interface{}) error {
	if len(b) == 0 {
		return nil
	}

	switch value := value.(type) {
	case nil:
		return nil
	case *[]byte:
		clone := make([]byte, len(b))
		copy(clone, b)
		*value = clone
		return nil
	case *string:
		*value = string(b)
		return nil
	}

	return msgpack.Unmarshal(b, value)
}
