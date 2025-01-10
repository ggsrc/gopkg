package wpgx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/stumble/wpgx"
)

type ConfigOption func(c *wpgx.Config)

func WithBeforeAcquire(f func(context.Context, *pgx.Conn) bool) ConfigOption {
	return func(c *wpgx.Config) {
		c.BeforeAcquire = f
	}
}
