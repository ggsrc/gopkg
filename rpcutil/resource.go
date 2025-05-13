package rpcutil

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-co-op/gocron/v2"
	hatchet_cli "github.com/hatchet-dev/hatchet/pkg/v1"
	hatchet_worker "github.com/hatchet-dev/hatchet/pkg/v1/worker"
	"github.com/kelseyhightower/envconfig"
	"github.com/posthog/posthog-go"
	"github.com/redis/go-redis/v9"
	"github.com/stumble/dcache"
	"github.com/stumble/wpgx"
	"github.com/uptrace/uptrace-go/uptrace"

	"github.com/ggsrc/gopkg/database/cache"
	db_wpgx "github.com/ggsrc/gopkg/database/wpgx"
	"github.com/ggsrc/gopkg/env"
	"github.com/ggsrc/gopkg/grpc"
	"github.com/ggsrc/gopkg/health"
	"github.com/ggsrc/gopkg/metric"
	"github.com/ggsrc/gopkg/profiling"
	"github.com/ggsrc/gopkg/zerolog"
	"github.com/ggsrc/gopkg/zerolog/log"
)

var (
	DefaultResourceShutDownTimeout = 40 * time.Second
)

type Resource struct {
	AppName     string
	WPGXPool    *wpgx.Pool
	RedisClient redis.UniversalClient
	DCache      *dcache.DCache

	GrpcServer    *grpc.Server
	CronScheduler gocron.Scheduler

	HealthChecker *health.Server
	Metricer      *metric.Server
	Profiling     *profiling.Server
	HatchetCli    hatchet_cli.HatchetClient
	HatchetWorker hatchet_worker.Worker

	PostHogCli posthog.Client

	CustomResources []CustomResource
}

// Start will hang the main goroutine until a signal is received or an error occurs
func (r *Resource) Start(ctx context.Context) {
	if r.CronScheduler != nil {
		r.CronScheduler.Start()
	}

	grpcErrCh, healthErrCh, metricErrCh, profilingErrCh, hatchetWorkerErrCh :=
		make(chan error, 1),
		make(chan error, 1),
		make(chan error, 1),
		make(chan error, 1),
		make(chan error, 1)

	if r.GrpcServer != nil {
		go func() {
			log.Warn().Msg("GRPC server start")
			grpcErrCh <- r.GrpcServer.Start()
		}()
	}

	if r.HealthChecker != nil {
		go func() {
			log.Warn().Msg("HealthCheck server start")
			healthErrCh <- r.HealthChecker.Start()
		}()
	}

	if r.Metricer != nil {
		go func() {
			log.Warn().Msg("Metric server start")
			metricErrCh <- r.Metricer.Start()
		}()
	}

	if r.Profiling != nil {
		go func() {
			log.Warn().Msg("Profiling server start")
			profilingErrCh <- r.Profiling.Start()
		}()
	}

	var hatchetWorkerCleanUp func() error

	if r.HatchetWorker != nil {
		go func() {
			log.Warn().Msg("HatchetWorker start")
			var err error
			hatchetWorkerCleanUp, err = r.HatchetWorker.Start()
			if err != nil {
				hatchetWorkerErrCh <- err
			}
		}()
	}

	time.Sleep(1 * time.Second)

	for _, res := range r.CustomResources {
		if err := res.Start(ctx); err != nil {
			log.Ctx(ctx).Error().Err(err).Msgf("failed to start custom resource %v", res)
			r.ShutDown(context.Background())
			return
		}
	}

	// Monitor system signal like SIGINT and SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case osSig := <-sig:
		log.Error().Msgf("received signal %s; shutting down", osSig)
		r.ShutDown(context.Background())
	case err := <-healthErrCh:
		log.Error().Err(err).Msg("health server error; shutting down")
		r.ShutDown(context.Background())
	case err := <-metricErrCh:
		log.Error().Err(err).Msg("metricer server error; shutting down")
		r.ShutDown(context.Background())
	case err := <-grpcErrCh:
		log.Error().Err(err).Msg("grpc server error; shutting down")
		r.ShutDown(context.Background())
	case err := <-profilingErrCh:
		log.Error().Err(err).Msg("profiling server error; shutting down")
		r.ShutDown(context.Background())
	case err := <-hatchetWorkerErrCh:
		log.Error().Err(err).Msg("hatchet worker error; shutting down")
		r.ShutDown(context.Background())
	}
	if hatchetWorkerCleanUp != nil {
		_ = hatchetWorkerCleanUp()
	}
}

func (r *Resource) ShutDown(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, DefaultResourceShutDownTimeout)
	defer cancel()

	// shutdown services concurrently and wait for all to finish, e.g. grpc server, cronjob, etc.
	var wg sync.WaitGroup

	if r.GrpcServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.GrpcServer.Shutdown(ctx); err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("failed to shutdown grpc server")
			}
		}()
	}

	if r.CronScheduler != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.CronScheduler.Shutdown(); err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("failed to shutdown cronjob")
			}
		}()
	}

	wg.Wait()
	for _, res := range r.CustomResources {
		if err := res.Close(ctx); err != nil {
			log.Ctx(ctx).Error().Err(err).Msgf("failed to close custom resource %v", res)
		}
	}
	// close db connection pool
	if r.WPGXPool != nil {
		r.WPGXPool.Close()
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
	if r.PostHogCli != nil {
		if err := r.PostHogCli.Close(); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to close posthog connection")
		}
	}
	// close health check
	if r.HealthChecker != nil {
		r.HealthChecker.Stop()
	}

	if err := uptrace.Shutdown(ctx); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to shutdown uptrace")
	}
}

func (r *Resource) OK(ctx context.Context) error {
	var checker []health.Checkable
	if r.DCache != nil {
		checker = append(checker, r.DCache.Ping)
	}
	if r.WPGXPool != nil {
		checker = append(checker, r.WPGXPool.Ping)
	}
	return health.GoCheck(
		ctx,
		checker...,
	)
}

func (r *Resource) RegisterCustomResource(ctx context.Context, resourceList ...CustomResource) {
	r.CustomResources = append(r.CustomResources, resourceList...)
}

func (r *Resource) RegisterHealthCheckable(ctx context.Context, checkable ...health.HealthCheckable) {
	if r.HealthChecker == nil {
		log.Ctx(ctx).Error().Msg("health checker not initialized")
		return
	}
	checkFn := make([]health.Checkable, 0, len(checkable))
	for _, c := range checkable {
		checkFn = append(checkFn, c.OK)
	}
	r.HealthChecker.AddHooks(checkFn...)
}

var (
	resource *Resource
	initRes  sync.Once
)

func NewResource(ctx context.Context, o RpcInitHelperOptions) (*Resource, error) {
	if o.AppName == "" {
		o.AppName = env.ServiceName()
	}
	zerolog.InitLogger(o.Debug)
	myResource := &Resource{
		AppName: o.AppName,
	}

	// init db
	if o.InitWpgx {
		opts := []db_wpgx.ConfigOption{}
		if o.WPGXBeforeAcquire != nil {
			opts = append(opts, db_wpgx.WithBeforeAcquire(o.WPGXBeforeAcquire))
		}
		db, err := db_wpgx.InitDB(ctx, DefaultInitDBTimeout, opts...)
		if err != nil {
			return nil, err
		}
		myResource.WPGXPool = db
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
		myResource.GrpcServer = grpc.NewServer(o.AppName, o.GrpcServerConf, o.GrpcServerOpt...)
	}
	// init cronjob
	if o.InitCronJob {
		scheduler, err := gocron.NewScheduler(o.CronJobOpt...)
		if err != nil {
			return nil, err
		}
		myResource.CronScheduler = scheduler
	}
	// init health check
	if o.InitHealthCheck {
		allCheck := []health.HealthCheckable{myResource}
		allCheck = append(allCheck, o.Checkable...)
		myResource.HealthChecker = health.InitHealthCheck(allCheck...)
	}
	// init metric
	if o.InitMetric {
		myResource.Metricer = metric.New(nil)
	}
	// init profiling
	if o.InitProfiling {
		myResource.Profiling = profiling.InitProfiler(o.ProfilingConf)
	}
	if o.InitHatchetCli {
		hatchetCli, err := hatchet_cli.NewHatchetClient(o.HatchetCliOpts...)
		if err != nil {
			return nil, err
		}
		myResource.HatchetCli = hatchetCli
		worker, err := hatchetCli.Worker(o.HatchetWorkerOpts)
		if err != nil {
			return nil, err
		}
		myResource.HatchetWorker = worker
	}
	if o.InitPosthogCli {
		conf := o.PosthogConf
		if conf == nil {
			conf = &posthog.Config{}
			envconfig.MustProcess("posthog", conf)
		}
		if o.PosthogApiKey == "" {
			o.PosthogApiKey = os.Getenv("POSTHOG_APIKEY")
		}
		posthogCli, err := posthog.NewWithConfig(o.PosthogApiKey, *conf)
		if err != nil {
			return nil, err
		}
		myResource.PostHogCli = posthogCli
	}
	if o.CustomResourceOps != nil {
		myResource.CustomResources = o.CustomResourceOps
	}
	return myResource, nil
}
