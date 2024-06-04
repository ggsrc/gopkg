package cache

import (
	"time"

	"github.com/coocood/freecache"
	"github.com/kelseyhightower/envconfig"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/stumble/dcache"
)

type DCacheConfig struct {
	ReadInterval   time.Duration `default:"500ms"`
	EnableStats    bool          `default:"true"`
	EnableTrace    bool          `default:"true"`
	InMemCacheSize int           `default:"52428800"` // base unit in byte: 50 * 1024 * 1024 = 52428800 -> 50MB
}

func NewDCache(appName, dcacheEnvPrefix string, redisConn redis.UniversalClient) (*dcache.DCache, error) {
	c := DCacheConfig{}
	envconfig.MustProcess(dcacheEnvPrefix, &c)
	log.Warn().Msgf("DCache Config: %+v", c)
	return dcache.NewDCache(
		appName,
		redisConn,
		freecache.NewCache(c.InMemCacheSize),
		c.ReadInterval,
		c.EnableStats,
		c.EnableTrace)
}
