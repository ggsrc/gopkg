package wpgx

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/stumble/wpgx"
)

var (
	ptMap   = map[string]*pgtype.Type{}
	timeout = 5 * time.Second // default timeout

	ts []string
)

type TypeLoaderParam struct {
	Timeout time.Duration `default:"5s"`
	Types   []string
}

func InitTypeLoader(p *TypeLoaderParam) {
	if p == nil {
		cfg := TypeLoaderParam{}
		envconfig.MustProcess("WPGX_TYPE_LOADER", &cfg)
		log.Info().Msgf("type loader config: %+v", cfg)
		p = &cfg
	}
	if p.Timeout > 0 {
		timeout = p.Timeout
	}
	dedup := map[string]struct{}{}
	for _, t := range p.Types {
		dedup[t] = struct{}{}
	}
	ts = make([]string, 0, len(dedup))
	for t := range dedup {
		ts = append(ts, t)
	}
}

func LoadTypes(ctx context.Context, conn *pgx.Conn) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for _, t := range ts {
		// reuse type if already registered in conn
		if _, ok := conn.TypeMap().TypeForName(t); ok {
			log.Debug().Msgf("type %s already loaded", t)
			continue
		}
		if pt, ok := ptMap[t]; ok {
			// reuse type if already loaded in ptMap
			conn.TypeMap().RegisterType(pt)
			log.Debug().Msgf("type %s already loaded", t)
		} else {
			var err error
			// load type from database
			if pt, err = conn.LoadType(ctx, t); err != nil {
				log.Err(err).Msgf("failed to load type %s", t)
				return true
			}
			log.Info().Msgf("loaded type %s", t)
			conn.TypeMap().RegisterType(pt)
			ptMap[t] = pt
		}
	}
	return true
}

func LoadTypesBeforeAcquire(c *wpgx.Config) {
	c.BeforeAcquire = LoadTypes
}
