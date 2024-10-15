package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	mask "github.com/showa-93/go-mask"

	"github.com/ggsrc/gopkg/goodns"
)

type Config struct {
	Host                string        `default:"127.0.0.1"`
	Port                int           `default:"6379"`
	Password            string        `default:"" mask:"fixed"`
	IsFailover          bool          `default:"false"`
	IsElastiCache       bool          `default:"false"`
	IsClusterMode       bool          `default:"false"`
	ClusterAddrs        []string      `default:""`
	ClusterMaxRedirects int           `default:"3"`
	ReadTimeout         time.Duration `default:"3s"`
	PoolSize            int           `default:"50"`
}

func NewRedisClient(envPrefix string) redis.UniversalClient {
	masker := mask.NewMasker()
	masker.RegisterMaskStringFunc(mask.MaskTypeFilled, masker.MaskFilledString)
	masker.RegisterMaskStringFunc(mask.MaskTypeFixed, masker.MaskFixedString)

	c := Config{}
	envconfig.MustProcess(envPrefix, &c)
	conf, _ := masker.Mask(c)
	log.Warn().Msgf("Redis Config: %+v", conf)

	var redisClient redis.UniversalClient
	if c.IsClusterMode {
		redisClient = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        c.ClusterAddrs,
			MaxRedirects: c.ClusterMaxRedirects,
			ReadTimeout:  c.ReadTimeout,
			PoolSize:     c.PoolSize,
			Password:     c.Password,
		})
	} else if !c.IsFailover {
		option := &redis.Options{
			Addr:        fmt.Sprintf("%s:%d", c.Host, c.Port),
			ReadTimeout: c.ReadTimeout,
			PoolSize:    c.PoolSize,
			Password:    c.Password,
		}
		if c.IsElastiCache {
			// Elasticache cert cannot be applied to cname record we use
			option.TLSConfig = &tls.Config{
				// nolint: gosec
				InsecureSkipVerify: true,
			}
		}
		redisClient = redis.NewClient(option)
	} else {
		dnsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ips, err := goodns.LookupA(dnsCtx, c.Host, true)
		if err != nil {
			panic("failed to exec goodns.LookupA: " + err.Error())
		}
		for _, ip := range ips {
			log.Info().Msgf("%s Has A: %s\n", c.Host, ip.String())
		}

		addrs := make([]string, len(ips))
		for i, ip := range ips {
			addrs[i] = fmt.Sprintf("%s:%d", ip.String(), c.Port)
		}
		redisClient = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    "master",
			SentinelAddrs: addrs,
			Password:      c.Password,
			PoolSize:      c.PoolSize,
			ReadTimeout:   c.ReadTimeout,
		})
		//redisClient = redis.NewClusterClient(&redis.ClusterOptions{
		//	Addrs:         addrs,
		//	Password:      c.Password,
		//	PoolSize:      c.PoolSize,
		//	ReadTimeout:   c.ReadTimeout,
		//	ReadOnly:      true,
		//	RouteRandomly: true,
		//})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	if err := redisotel.InstrumentTracing(redisClient); err != nil {
		log.Fatal().Err(err).Msgf("failed to InstrumentTracing to redis; config: %v", conf)
	}
	if err := redisotel.InstrumentMetrics(redisClient); err != nil {
		log.Fatal().Err(err).Msgf("failed to InstrumentMetrics to redis; config: %v", conf)
	}
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Msgf("failed to connect to redis; config: %v", conf)
	}
	return redisClient
}
