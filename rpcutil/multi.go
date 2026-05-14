// Package rpcutil: MultiResource — mega framework L1 root object.
//
// 这个文件是 mega 化 framework 的核心：定义 SubApp interface 和 MultiResource
// struct，串起所有 sub-app + listener + DB pool + Redis cache + cron 调度 +
// metric registry + health check + observability。
//
// 与老 Resource API 并存（向后兼容，老 sub-app 不强制改造）。详见
// mega-project/docs/10-framework-spec.md §二、§三。
//
// 本文件目前是 **API freeze 占位版本**：类型定义齐全（其它文件依赖），
// 方法体多为空 / 返回 errNotImplemented。Wave 3 agent 会替换占位，
// 实现 Register / SubAppContext / LoggerFor / MetricRegistryFor /
// GrpcServerOn / HttpRouterOn / setMegaSDKEnv 等行为。
//
// 文件所有权：
//   - 类型 stub（MultiResource / SubApp / MultiOption / multiConfig / registeredSubApp / registeredHealthCheck）→ Wave 0（我）
//   - 行为实现 → Wave 3 agent G
package rpcutil

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stumble/dcache"
	"github.com/stumble/wpgx"
	"google.golang.org/grpc"
)

// errNotImplemented 是 Wave 0 stub 的占位错误。Wave 1-4 agent 替换。
var errNotImplemented = errors.New("rpcutil: not implemented (Wave 0 stub)")

// SubApp 是 mega 内一个子应用必须实现的契约。
//
// 生命周期：
//  1. mega main 调用 res.Register(subapp, OnPorts(...)) → framework 内部校验
//  2. framework 按 Listeners() 分配端口（Phase 1 net.Listen）
//  3. Phase 2 调 Init(deps) → RegisterRoutes(reg)
//  4. Phase 3 注册 HealthChecks + Cron
//  5. 收到 SIGTERM：framework 反向调 Close(ctx)
//
// 详细行为见 mega-project/docs/10-framework-spec.md §二。
type SubApp interface {
	Name() string
	Listeners() []ListenerSpec
	Init(deps *MultiResource) error
	RegisterRoutes(reg RouteRegistry) error
	HealthChecks() []HealthCheckable
	Cron() []CronJob
	Close(ctx context.Context) error
}

// MultiResource 是 mega 模式的 framework 根对象。
//
// **字段所有权**：本 struct 的字段定义在本文件，所有跨文件方法（在
// listener.go / cron.go / health.go / deps.go / observability.go /
// startup.go 中定义）通过 *MultiResource receiver 读写这些字段。
//
// Wave 1-4 agent 可在自家文件添加方法（不要扩字段；扩字段需要回到本文件）。
type MultiResource struct {
	appName       string
	rootLogger    *zerolog.Logger
	migrationMode bool

	subApps []registeredSubApp
	portMap map[string]int // "subapp.listener" → port

	grpcServers   map[int]*grpc.Server
	grpcListeners map[int]any // net.Listener
	httpRouters   map[int]*gin.Engine
	httpListeners map[int]any
	httpServers   map[int]any // *http.Server

	dbPools   map[string]*wpgx.Pool
	dbConfigs map[string]DBConfig

	redis  redis.UniversalClient
	dcache *dcache.DCache
	caches map[string]*dcache.DCache // subapp → namespaced wrapper

	registries map[string]*prometheus.Registry
	gatherers  prometheus.Gatherers

	scheduler    Scheduler
	cronDisabled bool
	cronJobs     map[string]bool

	healthChecks []registeredHealthCheck
	healthServer any // *http.Server
	metricServer any // *http.Server

	startedAt  *atomic.Pointer[time.Time]
	closeFuncs []func(context.Context) error

	mu sync.Mutex // 保护并发 Register 和内部 map 写入
}

// registeredSubApp 是 framework 内部存放每个已注册 sub-app 的记录。
type registeredSubApp struct {
	app       SubApp
	portMap   PortMap // mega main 传入的端口分配
	envPrefix string  // 派生自 Name()，全大写下划线
}

// registeredHealthCheck 把每个 sub-app 的 HealthCheckable 关联到 subapp 名字。
type registeredHealthCheck struct {
	subapp string
	hc     HealthCheckable
}

// ---- 工厂 ----

// MultiOption 是 NewMultiResource 的选项。
type MultiOption func(*multiConfig)

type multiConfig struct {
	appName       string
	migrationMode bool
}

func defaultMultiConfig() multiConfig {
	return multiConfig{migrationMode: true}
}

// WithMegaAppName 设置 mega 整体的 appName（如 "airdrop-mega"）。
// 命名不用 WithAppName 是为了避免与老 Init.go 的同名 RpcInitHelperOption 冲突。
func WithMegaAppName(name string) MultiOption {
	return func(c *multiConfig) { c.appName = name }
}

// WithMigrationMode 控制是否在每条日志输出 legacy_app 字段。
// 默认 true（迁移期 1-2 月用，让老 dashboard 改 legacy_app=galxe-X 即可工作）；
// 稳定后业务 main 改 WithMigrationMode(false)。
func WithMigrationMode(enabled bool) MultiOption {
	return func(c *multiConfig) { c.migrationMode = enabled }
}

// MustNewMultiResource 是 NewMultiResource 的 panic 版本。
// Wave 3 agent 实现。
func MustNewMultiResource(ctx context.Context, opts ...MultiOption) *MultiResource {
	m, err := NewMultiResource(ctx, opts...)
	if err != nil {
		panic(err)
	}
	return m
}

// NewMultiResource 创建 MultiResource 实例。
// Wave 3 agent 实现：初始化所有 map / scheduler / startedAt / redis client / etc.
// 包括读取 MEGA_DISABLE_CRON env、设置 mega SDK env（OTEL_SERVICE_NAME 等）。
func NewMultiResource(ctx context.Context, opts ...MultiOption) (*MultiResource, error) {
	return nil, errNotImplemented
}

// ---- 公共 API（Wave 3 agent 实现，其它 wave 可看签名） ----

// Register 注册一组 sub-app。可多次调用，但所有 Register 必须在 Start 之前完成。
//
// 接收交替的 SubApp 和 ...RegisterOption 参数：
//
//	res.Register(
//	    airdrop.New(), OnPorts(PortMap{"grpc": 9090, "http-rest": 3000}),
//	    token.New(),   OnPorts(PortMap{"grpc": 9091, "http-rest": 3001}),
//	)
//
// Wave 3 agent 实现：校验 Listeners vs OnPorts 一致 / 端口唯一 / Cron 唯一 /
// DB pool 名唯一，然后 m.subApps 追加 registeredSubApp。
func (m *MultiResource) Register(args ...interface{}) error {
	return errNotImplemented
}

// SubAppContext 返回带 subapp logger 的 base ctx。
// 详见 docs/12 §一 Layer 1。Wave 3 agent 实现。
func (m *MultiResource) SubAppContext(subapp string) context.Context {
	return context.Background()
}

// LoggerFor 直接返回 sub-app logger（不通过 ctx）。
// Wave 3 agent 实现。
func (m *MultiResource) LoggerFor(subapp string) *zerolog.Logger {
	return m.rootLogger
}

// MetricRegistryFor 返回 sub-app 独立 Prometheus registry。
// 详见 docs/12 §二。Wave 3 agent 实现。
func (m *MultiResource) MetricRegistryFor(subapp string) prometheus.Registerer {
	return prometheus.DefaultRegisterer
}
