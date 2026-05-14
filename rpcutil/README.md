# rpcutil

rpcutil is a utility package for grpc server.

use environment variable to control the server parameter

### grpc

| environment var        | Description                                          | default value      |
|------------------------|------------------------------------------------------|--------------------|
| GRPC_PORT       | The port number on which the gRPC server is running. | 9090               |
| GRPC_RAVEN_DSN | The DSN for the Sentry client.	                     | "" (empty string)  |
| GRPC_VERBOSE | Enable verbose logging.	                             | false              |


### redis

| environment var        | Description                                          | default value      |
|------------------------|------------------------------------------------------|--------------------|
| REDIS_HOST       |The hostname or IP address of the Redis server.	 | 127.0.0.1               |
| REDIS_PORT | The port number on which the Redis server is running.     | 6379          |
| REDIS_PASSWORD | The password used to authenticate with the Redis server. | "" (empty string) |
| REDIS_IS_FAILOVER | Indicates if failover support is enabled.	             | false             |
| REDIS_IS_ELASTICACHE | Indicates if the Redis instance is an ElastiCache instance. | false |
| REDIS_IS_CLUSTER_MODE | Indicates if Redis is running in cluster mode.	         | false             |
| REDIS_CLUSTER_ADDRS | Addresses of the nodes in the Redis cluster.	         | "" (empty slice)  |
| REDIS_CLUSTER_MAX_REDIRECTS | Maximum number of redirects to follow in cluster mode. | 3                 |
| REDIS_READ_TIMEOUT | The duration to wait before timing out on read operations. | 3s               |
| REDIS_POOL_SIZE | The maximum number of connections in the pool.	         | 50                |


### dcache

| environment var        | Description                                          | default value      |
|------------------------|------------------------------------------------------|--------------------|
| DCACHE_READINTERVAL       | The interval to read the cache data.	 | 500ms               |
| DCACHE_ENABLESTATS | Enable the cache statistics.     | true          |
| DCACHE_ENABLETRACE | Enable the cache tracing. | true |
| DCACHE_INMEMCACHESIZE | The size of the in-memory cache. | 52428800 (50MB) |


### postgres

| environment var        | Description                                          | default value      |
|------------------------|------------------------------------------------------|--------------------|
| POSTGRES_PORT       | Port number for PostgreSQL                           | 5432               |
| POSTGRES_HOST | Host address for PostgreSQL                          | localhost          |
| POSTGRES_USERNAME | Username for PostgreSQL                              | postgres           |
| POSTGRES_PASSWORD | Password for PostgreSQL	                             | my-secret          |
| POSTGRES_DBNAME | Database name for PostgreSQL                         | wpgx_test_db       |
| POSTGRES_MAXCONNS | Maximum number of idle connections                   | 100                |
| POSTGRES_MINCONNS | Minimum number of idle connections	                  | 0                  |
| POSTGRES_MAXCONNLIFETIME | Maximum lifetime of connections	                     | 6h                 |
| POSTGRES_MAXCONNIDLETIME | Maximum idle time of connections                     | 1m                 |
| POSTGRES_ENABLEPROMETHEUS | Enable Prometheus metrics for PostgreSQL connections | true               |
| POSTGRES_ENABLETRACING | Enable tracing for PostgreSQL connections	           | true               |
| POSTGRES_APPNAME | Application name for PostgreSQL	                     | (must be provided) |


### healthcheck:

health check is a http server that expose the health status of the grpc server

| environment var        | Description                     | default value |
|------------------------|---------------------------------|---------------|
| HEALTHCHECK_PORT       | health check expose port        | 8080          |
| HEALTHCHECK_READYCOUNT | Number of successful ready checks required before marking the service as ready | 0             |
| HEALTHCHECK_LIVECOUNT | Number of successful liveness checks required before marking the service as alive | 3             |
| HEALTHCHECK_PROBEINTERVAL | Interval between health check probes | 5s            |
| HEALTHCHECK_PROBETIMEOUT | Timeout for each health check probe	| 5s            |
| HEALTHCHECK_READY | Whether Initial ready checker   | true          |
| HEALTHCHECK_ALIVE | Whether Initial alive checker   | true          |

---

# MultiResource (mega framework, v1)

`MultiResource` is the second-generation API in this package: it runs **multiple sub-apps inside a single process** (a "mega binary") that share Go runtime, sidecar, and SDK init. The old `MustInitResource` / `Resource` API above is preserved unchanged for backward compatibility.

Use MultiResource if you are building a **mega binary** that consolidates several historically-separate services. Use the old `Resource` if you are running a single Go service unchanged.

## Quick start (mega binary `cmd/mega/main.go`)

```go
package main

import (
    "context"
    "os/signal"
    "syscall"

    "github.com/ggsrc/gopkg/rpcutil"
    "github.com/rs/zerolog/log"

    "github.com/NFTGalaxy/galxe-airdrop/pkg/subapp"
    tokensubapp   "github.com/NFTGalaxy/galxe-token/pkg/subapp"
    stakingsubapp "github.com/NFTGalaxy/galxe-staking/pkg/subapp"
)

// Set via -ldflags -X main.airdropVersion=$SHA at build time.
var airdropVersion, tokenVersion, stakingVersion string

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    res := rpcutil.MustNewMultiResource(ctx,
        rpcutil.WithMegaAppName("airdrop-mega"),
    )

    // Optional: record per-sub-app build versions for the mega_subapp_version metric.
    res.RecordSubAppVersion("airdrop", airdropVersion)
    res.RecordSubAppVersion("token",   tokenVersion)
    res.RecordSubAppVersion("staking", stakingVersion)

    // Register DB pools (one per sub-app, distinct envconfig prefixes).
    must(res.RegisterDB("airdrop", rpcutil.DBConfig{EnvPrefix: "AIRDROP_POSTGRES", MaxConns: 15}))
    must(res.RegisterDB("token",   rpcutil.DBConfig{EnvPrefix: "TOKEN_POSTGRES",   MaxConns: 10}))
    must(res.RegisterDB("staking", rpcutil.DBConfig{EnvPrefix: "STAKING_POSTGRES", MaxConns: 15}))

    // Hard PG connection budget gate.
    must(res.ValidateBudget(20 /* HPA maxReplicas */, rpcutil.BudgetConfig{
        InstanceMaxConnections: 500,
        SuperuserReserved:      3,
        SafetyFactor:           0.7,
        AdminPoolReserved:      5,
    }))

    // Register sub-apps with explicit per-listener ports.
    must(res.Register(
        subapp.New(),        rpcutil.OnPorts(rpcutil.PortMap{"grpc": 9090, "http-rest": 3000, "custome-web": 8989}),
        tokensubapp.New(),   rpcutil.OnPorts(rpcutil.PortMap{"grpc": 9091, "http-rest": 3001}),
        stakingsubapp.New(), rpcutil.OnPorts(rpcutil.PortMap{"grpc": 9092, "http-rest": 3002}),
    ))

    // Start blocks until ctx is cancelled (SIGTERM), then invokes graceful Stop.
    if err := res.Start(ctx); err != nil {
        log.Fatal().Err(err).Msg("mega start/stop")
    }
}

func must(err error) {
    if err != nil { log.Fatal().Err(err).Msg("init") }
}
```

## SubApp interface

Each sub-app library implements `SubApp`. Example skeleton:

```go
type airdropSubApp struct {
    deps *rpcutil.MultiResource
    svc  *service.Airdrop
}

func New() rpcutil.SubApp { return &airdropSubApp{} }

func (a *airdropSubApp) Name() string { return "airdrop" }

func (a *airdropSubApp) Listeners() []rpcutil.ListenerSpec {
    return []rpcutil.ListenerSpec{
        {Name: "grpc",        Protocol: "grpc", PreferredPort: 9090},
        {Name: "http-rest",   Protocol: "http", PreferredPort: 3000},
        {Name: "custome-web", Protocol: "http", PreferredPort: 8989},
    }
}

func (a *airdropSubApp) Init(deps *rpcutil.MultiResource) error {
    a.deps = deps
    pool, err := deps.DBFor("airdrop")
    if err != nil { return err }
    cache := deps.CacheFor("airdrop")  // keys auto-prefixed with "airdrop:"
    a.svc = service.New(pool, cache)
    return nil
}

func (a *airdropSubApp) RegisterRoutes(reg rpcutil.RouteRegistry) error {
    airdroppb.RegisterAirdropServer(reg.GRPC("grpc"), a.svc)
    a.svc.RegisterHTTP(reg.HTTP("http-rest"))
    return nil
}

func (a *airdropSubApp) HealthChecks() []rpcutil.HealthCheckable {
    pool, _ := a.deps.DBFor("airdrop")
    return []rpcutil.HealthCheckable{
        {Name: "airdrop-db", Criticality: rpcutil.Critical,
            Check: func(ctx context.Context) error { return pool.Ping(ctx) }},
    }
}

func (a *airdropSubApp) Cron() []rpcutil.CronJob {
    return []rpcutil.CronJob{
        {Name: "daily-reward", Schedule: "0 0 * * *",
            Fn: func(ctx context.Context) error { return a.svc.DailyReward(ctx) }},
    }
}

func (a *airdropSubApp) Close(ctx context.Context) error {
    return a.svc.Close(ctx)
}
```

## Lifecycle (Start / Stop)

`Start(ctx)` is strict 4-phase. Each phase fails-fast; partial init never leaves a half-open pod.

```
Phase 1: net.Listen on every grpc + http port
Phase 2: per sub-app Init(deps) → RegisterRoutes(reg) → register HealthChecks
Phase 3: register cron (skipped when MEGA_DISABLE_CRON=true) → scheduler.Start()
Phase 4: spawn Serve goroutines + open /health and /metrics → readiness flips 503→200
```

`Stop(ctx)` is strict 6-phase reverse. Each phase has its own timeout (30s for graceful, 5s for health server). Aborts increment `mega_shutdown_aborted_total{subapp,phase}`.

```
Phase 1: clear startedAt → /health/ready returns 503 (k8s drains endpoint)
Phase 2: parallel GracefulStop on grpc + http servers (30s each, force on timeout)
Phase 3: scheduler.Stop (30s)
Phase 4: sub-app Close() in REVERSE register order (30s each)
Phase 5: framework closeFuncs (DB pool Close hooks)
Phase 6: health + metric server Shutdown (5s)
```

`Start` blocks on `<-ctx.Done()` and chains `Stop` automatically. In mega main use `signal.NotifyContext(SIGINT, SIGTERM)`.

## Configuration knobs

| Knob | Source | Default | Purpose |
|---|---|---|---|
| `WithMegaAppName(name)` | option | required | Used as `OTEL_SERVICE_NAME=galxe-<name>` and root logger field |
| `WithMigrationMode(bool)` | option | `true` | Adds `legacy_app=galxe-<subapp>` to every log line (1–2 month override of old `app=...` dashboard queries) |
| `MEGA_DISABLE_CRON=true` | env | unset | Skip cron registration; useful during Day 0–4 of cutover |
| `MEGA_MAX_REPLICAS` | env (caller injects) | n/a | Pass to `ValidateBudget(maxReplicas, ...)` to size PG connection budget |
| `OTEL_SERVICE_NAME` etc | env | overridden | Framework hard-sets `OTEL_SERVICE_NAME`; **do not** put these in per-sub-app ConfigMap (CI lint will reject) |

## Metrics emitted

| Name | Type | Labels | Source |
|---|---|---|---|
| `mega_grpc_request_total` | counter | subapp, method, code | unary + stream interceptors |
| `mega_http_request_total` | counter | subapp, path, method, status | gin middleware |
| `mega_panic_total` | counter | subapp, site, method | panic recover (unary/stream/http) |
| `mega_goroutine_fail_total` | counter | subapp, name | `MultiResource.Go` retries |
| `mega_goroutine_panic_total` | counter | subapp, name | `MultiResource.Go` panic recover |
| `mega_cron_duration_seconds` | histogram | subapp, job | wrapped cron Fn |
| `mega_cron_error_total` | counter | subapp, job | cron Fn returns non-nil err |
| `mega_cron_panic_total` | counter | subapp, job | cron Fn panic |
| `mega_dep_unhealthy_total` | counter | subapp, dep_name, criticality | readinessHandler |
| `mega_shutdown_aborted_total` | counter | subapp, phase | Stop phase timeout |
| `mega_subapp_version` | gauge | subapp, version | build-time ldflags (or "unknown") |

Plus all `grpc_*` metrics from `grpc_prometheus` (server-side request/latency).

## Per sub-app envconfig prefix

Each `RegisterDB(name, DBConfig{EnvPrefix: "FOO"})` reads `FOO_*` env to populate `wpgx.Config`. Required keys (validated at startup if listed in `DBConfig.RequiredEnv`):

```
FOO_USERNAME / FOO_PASSWORD / FOO_HOST / FOO_PORT / FOO_DBNAME / FOO_APPNAME
```

`FOO_APPNAME` falls back to the registered name if unset.

## Backward compatibility

The old `MustInitResource` / `Resource` API in `Init.go` / `resource.go` / `custom_resource.go` is untouched. Existing single-binary Go services keep working without changes. New mega binaries should use `MultiResource`; sub-apps that want to be importable into a mega should be refactored to expose a `SubApp` implementation.

## Design references

The full design plan and rollout runbook live alongside this codebase in
`mega-project/docs/`:

- `10-framework-spec.md` — framework API spec (types + behaviors)
- `11-framework-startup-shutdown.md` — Start / Stop phase details
- `12-framework-observability.md` — logger / metric / health endpoint
- `20-subapp-libraryize-sop.md` — how to library-ize an existing sub-app
- `30-mega-repo-bootstrap.md` — how to lay out a mega binary repo

