package wpgx

import (
	"context"
	"fmt"
	"time"

	"github.com/stumble/wpgx"
)

func InitDB(ctx context.Context, timeout time.Duration, configOpts ...ConfigOption) (*wpgx.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	pool, err := NewWPGXPool(ctx, "postgres", configOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create db connection pool: %w", err)
	}
	return pool, nil
}
