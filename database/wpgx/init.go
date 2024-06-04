package wpgx

import (
	"context"
	"fmt"
	"github.com/stumble/wpgx"
	"time"
)

func InitDB(ctx context.Context, timeout time.Duration) (*wpgx.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	pool, err := NewWPGXPool(ctx, "postgres")
	if err != nil {
		return nil, fmt.Errorf("failed to create db connection pool: %w", err)
	}
	return pool, nil
}
