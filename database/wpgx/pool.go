package wpgx

import (
	"context"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/stumble/wpgx"
)

func NewWPGXPool(ctx context.Context, envPrefix string) (*wpgx.Pool, error) {
	c := wpgx.Config{}
	envconfig.MustProcess(envPrefix, &c)
	log.Ctx(ctx).Warn().Msgf("WPGX Config: %+v", &c)
	pool, err := wpgx.NewPool(ctx, &c)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		return nil, err
	}
	return pool, nil
}
