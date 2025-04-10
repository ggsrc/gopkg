package wpgx

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
)

const defaultTimeout = 5 * time.Second

type TypeLoaderParam struct {
	Timeout time.Duration `default:"5s"`
	Types   []string
}

var defaultLoader *Loader

func InitTypeLoader(p *TypeLoaderParam) {
	if p == nil {
		cfg := TypeLoaderParam{}
		envconfig.MustProcess("WPGX_TYPE_LOADER", &cfg)
		log.Info().Msgf("type loader config: %+v", cfg)
		p = &cfg
	}
	defaultLoader = NewLoader(p)
}

func LoadTypes(ctx context.Context, conn *pgx.Conn) bool {
	if defaultLoader == nil {
		// no type loader, skip
		log.Error().Msg("type loader not initialized")
		return true
	}
	return defaultLoader.LoadTypes(ctx, conn)
}

func Reset() {
	if defaultLoader == nil {
		// no type loader, skip
		log.Error().Msg("type loader not initialized")
		return
	}
	defaultLoader.Reset()
}

func NewLoader(p *TypeLoaderParam) *Loader {
	l := Loader{
		ptMap:   map[string]*pgtype.Type{},
		timeout: defaultTimeout,
	}
	if p.Timeout > 0 {
		l.timeout = p.Timeout
	}
	l.ts = make([]string, 0, len(p.Types))
	dedup := map[string]struct{}{}
	for _, t := range p.Types {
		if _, ok := dedup[t]; !ok {
			dedup[t] = struct{}{}
			l.ts = append(l.ts, t)
		}
	}
	return &l
}

type Loader struct {
	ptMap   map[string]*pgtype.Type
	timeout time.Duration
	ts      []string
}

func (l *Loader) LoadTypes(ctx context.Context, conn *pgx.Conn) bool {
	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	for _, t := range l.ts {
		// reuse type if already registered in conn
		if _, ok := conn.TypeMap().TypeForName(t); ok {
			continue
		}
		if pt, ok := l.ptMap[t]; ok {
			// reuse type if already loaded in ptMap
			conn.TypeMap().RegisterType(pt)
		} else {
			var err error
			// load type from database
			if pt, err = conn.LoadType(ctx, t); err != nil {
				log.Err(err).Msgf("failed to load type %s", t)
				return true
			}
			log.Info().Msgf("loaded type %s", t)
			conn.TypeMap().RegisterType(pt)
			l.ptMap[t] = pt
		}
	}
	return true
}

func (l *Loader) Reset() {
	l.ptMap = map[string]*pgtype.Type{}
	l.ts = []string{}
	l.timeout = defaultTimeout
}
