package rpcutil

import (
	"context"
	"github.com/ggsrc/gopkg/database/cache"
	db_wpgx "github.com/ggsrc/gopkg/database/wpgx"
	"github.com/ggsrc/gopkg/env"
	"github.com/ggsrc/gopkg/grpc"
	"github.com/ggsrc/gopkg/zerolog"
	"github.com/ggsrc/gopkg/zerolog/log"

	"github.com/redis/go-redis/v9"
	"github.com/stumble/dcache"
	"github.com/stumble/wpgx"
	"github.com/uptrace/uptrace-go/uptrace"
	"sync"
	"time"
)

var (
	DefaultResourceShutDownTimeout = 40 * time.Second
)

type Resource struct {
	Pool        *wpgx.Pool
	RedisClient redis.UniversalClient
	DCache      *dcache.DCache

	grpcServer *grpc.Server
}

func (r *Resource) ShutDown(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, DefaultResourceShutDownTimeout)
	defer cancel()

	// shutdown services concurrently and wait for all to finish, e.g. grpc server, cronjob, etc.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// shutdown grpc server
		if r.grpcServer != nil {
			if err := r.grpcServer.Shutdown(ctx); err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("failed to shutdown grpc server")
			}
		}
	}()

	wg.Wait()
	// close db connection pool
	if r.Pool != nil {
		r.Pool.Close()
	}
	// close dcache connection
	if r.DCache != nil {
		r.DCache.Close()
	}
	// close redis connection
	if r.RedisClient != nil {
		if err := r.RedisClient.Close(); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to close redis connection")
		}
	}
	if err := uptrace.Shutdown(ctx); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to shutdown uptrace")
	}
}

var (
	resource *Resource
	InitRes  sync.Once
)

func NewResource(ctx context.Context, o RpcInitHelperOptions) (*Resource, error) {
	if o.AppName == "" {
		o.AppName = env.ServiceName()
	}
	zerolog.InitLogger(o.Debug)
	myResource := &Resource{}
	// init db
	if o.InitWpgx {
		db, err := db_wpgx.InitDB(ctx, DefaultInitDBTimeout)
		if err != nil {
			return nil, err
		}
		myResource.Pool = db
	}
	// init cache
	if o.InitCache {
		myRedis, myCache, err := cache.InitCache(o.AppName)
		if err != nil {
			return nil, err
		}
		myResource.DCache = myCache
		myResource.RedisClient = myRedis
	}
	// init grpc
	if o.InitGrpcServer {
		myResource.grpcServer = grpc.NewServer(o.AppName, o.GrpcServerConf, o.GrpcServerOpt...)
	}
	return myResource, nil
}
