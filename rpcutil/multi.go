// Package rpcutil: MultiResource — mega framework L1 root object.
//
// 这个文件是 mega 化 framework 的核心：定义 SubApp interface 和 MultiResource
// struct，串起所有 sub-app + listener + DB pool + Redis cache + cron 调度 +
// metric registry + health check + observability。
//
// 与老 Resource API 并存（向后兼容，老 sub-app 不强制改造）。详见
// mega-project/docs/10-framework-spec.md §二、§三。
//
// 文件所有权：
//   - 类型 stub（MultiResource / SubApp / MultiOption / multiConfig / registeredSubApp / registeredHealthCheck）→ Wave 0
//   - 行为实现（NewMultiResource / Register / SubAppContext / LoggerFor /
//     MetricRegistryFor / setMegaSDKEnv / registerSubAppVersionMetric） →
//     Wave 3 agent G（本次提交）
package rpcutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stumble/dcache"
	"github.com/stumble/wpgx"
	"google.golang.org/grpc"

	gopkg_zerolog "github.com/ggsrc/gopkg/zerolog"
)

// errNotImplemented 是占位错误。仅保留以兼容历史 stub 测试；本次实现不再返回。
var errNotImplemented = errors.New("rpcutil: not implemented (Wave 0 stub)")

// subAppNameRe enforces docs/10 §三 sub-app 命名规范：
// 小写字母开头，仅允许小写字母 / 数字 / dash。
var subAppNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// sdkEnvBlacklist 列出不允许 sub-app ConfigMap 通过 envPrefix 暗中覆盖的 SDK
// 全局 env。详见 docs/10 §五。
var sdkEnvBlacklist = []string{
	"OTEL_SERVICE_NAME",
	"OTEL_EXPORTER_OTLP_ENDPOINT",
	"OTEL_EXPORTER_OTLP_PROTOCOL",
	"OTEL_RESOURCE_ATTRIBUTES",
	"OTEL_TRACES_SAMPLER",
	"OTEL_TRACES_SAMPLER_ARG",
}

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

	// subappVersionOnce 保证 registerSubAppVersionMetric 跨多次 Start() 幂等。
	subappVersionOnce sync.Once
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
func MustNewMultiResource(ctx context.Context, opts ...MultiOption) *MultiResource {
	m, err := NewMultiResource(ctx, opts...)
	if err != nil {
		panic(err)
	}
	return m
}

// NewMultiResource 创建 MultiResource 实例。
//
// 初始化所有 map / scheduler / startedAt，读取 MEGA_DISABLE_CRON env，
// 设置 mega SDK env（OTEL_SERVICE_NAME 等）。Redis / DCache 默认 nil；
// 业务 main 后续可通过未来 setter 注入（deps.go 的 CacheFor 已 null-safe）。
func NewMultiResource(ctx context.Context, opts ...MultiOption) (*MultiResource, error) {
	cfg := defaultMultiConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.appName == "" {
		return nil, errors.New("rpcutil: WithMegaAppName required")
	}

	derived := log.Logger.With().Str("app", cfg.appName).Logger()

	m := &MultiResource{
		appName:       cfg.appName,
		rootLogger:    &derived,
		migrationMode: cfg.migrationMode,

		portMap:       map[string]int{},
		grpcServers:   map[int]*grpc.Server{},
		grpcListeners: map[int]any{},
		httpRouters:   map[int]*gin.Engine{},
		httpListeners: map[int]any{},
		httpServers:   map[int]any{},

		dbPools:   map[string]*wpgx.Pool{},
		dbConfigs: map[string]DBConfig{},

		caches: map[string]*dcache.DCache{},

		registries: map[string]*prometheus.Registry{},
		gatherers:  prometheus.Gatherers{prometheus.DefaultGatherer},

		cronJobs: map[string]bool{},

		startedAt: &atomic.Pointer[time.Time]{},
	}

	sched, err := newDefaultScheduler()
	if err != nil {
		return nil, fmt.Errorf("rpcutil: NewMultiResource scheduler: %w", err)
	}
	m.scheduler = sched

	if os.Getenv("MEGA_DISABLE_CRON") == "true" {
		m.cronDisabled = true
		log.Info().Str("app", cfg.appName).Msg("MEGA_DISABLE_CRON=true; cron disabled at mega level")
	}

	m.setMegaSDKEnv()

	return m, nil
}

// ---- 公共 API ----

// Register 注册一组 sub-app。可多次调用，但所有 Register 必须在 Start 之前完成。
//
// 接收交替的 SubApp 和 ...RegisterOption 参数：
//
//	res.Register(
//	    airdrop.New(), OnPorts(PortMap{"grpc": 9090, "http-rest": 3000}),
//	    token.New(),   OnPorts(PortMap{"grpc": 9091, "http-rest": 3001}),
//	)
//
// 校验顺序：
//  1. arg 非空
//  2. 每个 SubApp.Name() 满足命名规范且全 mega 内唯一
//  3. processSubApp 分配端口 + 创建 gRPC server / gin engine
//  4. 追加 registeredSubApp 到 m.subApps（envPrefix 全大写下划线）
//
// 任一 sub-app 失败立即返回 error；已注册 sub-app 保留（端口已绑无法回滚）。
func (m *MultiResource) Register(args ...interface{}) error {
	if len(args) == 0 {
		return errors.New("rpcutil: Register requires at least one SubApp")
	}

	i := 0
	for i < len(args) {
		app, ok := args[i].(SubApp)
		if !ok {
			return fmt.Errorf("rpcutil: Register: arg %d expected SubApp, got %T", i, args[i])
		}

		// 收集 opts，直到下一个 SubApp 或 args 终结。
		j := i + 1
		var opts []RegisterOption
		for j < len(args) {
			if _, isApp := args[j].(SubApp); isApp {
				break
			}
			opt, isOpt := args[j].(RegisterOption)
			if !isOpt {
				return fmt.Errorf(
					"rpcutil: Register: arg %d expected RegisterOption or SubApp, got %T",
					j, args[j])
			}
			opts = append(opts, opt)
			j++
		}

		if err := m.registerOne(app, opts...); err != nil {
			return err
		}

		i = j
	}
	return nil
}

// registerOne 处理 Register 拆分后的单个 (SubApp, opts) 组。
func (m *MultiResource) registerOne(app SubApp, opts ...RegisterOption) error {
	name := app.Name()
	if !subAppNameRe.MatchString(name) {
		return fmt.Errorf("rpcutil: Register: invalid SubApp name %q (want %s)",
			name, subAppNameRe.String())
	}

	m.mu.Lock()
	for _, rs := range m.subApps {
		if rs.app != nil && rs.app.Name() == name {
			m.mu.Unlock()
			return fmt.Errorf("rpcutil: Register: duplicate SubApp name %q", name)
		}
	}
	m.mu.Unlock()

	if err := m.processSubApp(app, opts...); err != nil {
		return fmt.Errorf("rpcutil: Register %q: %w", name, err)
	}

	cfg := mergeRegisterOptions(opts...)
	envPrefix := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))

	m.mu.Lock()
	m.subApps = append(m.subApps, registeredSubApp{
		app:       app,
		portMap:   cfg.portMap,
		envPrefix: envPrefix,
	})
	m.mu.Unlock()
	return nil
}

// SubAppContext 返回带 subapp logger 的 base ctx。
// 详见 docs/12 §一 Layer 1。
func (m *MultiResource) SubAppContext(subapp string) context.Context {
	ctx := gopkg_zerolog.WithSubApp(context.Background(), subapp)
	base := m.rootLogger
	if base == nil {
		base = &log.Logger
	}
	builder := base.With().Str("subapp", subapp)
	if m.migrationMode {
		builder = builder.Str("legacy_app", "galxe-"+subapp)
	}
	l := builder.Logger()
	return l.WithContext(ctx)
}

// LoggerFor 直接返回 sub-app logger（不通过 ctx）。
func (m *MultiResource) LoggerFor(subapp string) *zerolog.Logger {
	base := m.rootLogger
	if base == nil {
		base = &log.Logger
	}
	builder := base.With().Str("subapp", subapp)
	if m.migrationMode {
		builder = builder.Str("legacy_app", "galxe-"+subapp)
	}
	l := builder.Logger()
	return &l
}

// MetricRegistryFor 返回 sub-app 独立 Prometheus registry。
// 同名重复调用返回同一 registry（幂等）。注册时把 registry 同时挂到
// m.gatherers，启动期 /metrics 通过 prometheus.Gatherers.Gather() 聚合。
// 详见 docs/12 §二。
func (m *MultiResource) MetricRegistryFor(subapp string) prometheus.Registerer {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.registries == nil {
		m.registries = map[string]*prometheus.Registry{}
	}
	if reg, ok := m.registries[subapp]; ok {
		return reg
	}
	reg := prometheus.NewRegistry()
	m.registries[subapp] = reg
	m.gatherers = append(m.gatherers, reg)
	return reg
}

// subAppNames 返回 Register 顺序的 sub-app 名字列表。
// health.go 的 orderedSubAppNames 自带排序逻辑，此处保持注册顺序不动。
func (m *MultiResource) subAppNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.subApps))
	for _, rs := range m.subApps {
		if rs.app != nil {
			names = append(names, rs.app.Name())
		}
	}
	return names
}

// subAppByPort 反查端口对应的 sub-app 名字。Wave 4 startup.go 在 shutdown
// abort metric 上做 subapp label 时使用。
func (m *MultiResource) subAppByPort(port int) string {
	return m.subappForListener(port)
}

// setMegaSDKEnv 在 NewMultiResource 时统一 mega 层 SDK env：
//  1. 强制设置 OTEL_SERVICE_NAME = "galxe-<appName>"，覆盖 ConfigMap 残余值
//  2. 扫描已注册 sub-app 的 envPrefix，若发现 sub-app ConfigMap 泄漏的 SDK
//     env（如 STAKING_OTEL_SERVICE_NAME），输出 warn 但不 fail
//
// NewMultiResource 时 m.subApps 为空，循环 no-op；Register 之后业务也可
// 再次调用以做二次审计。
func (m *MultiResource) setMegaSDKEnv() {
	_ = os.Setenv("OTEL_SERVICE_NAME", "galxe-"+m.appName)
	names := m.subAppNames()
	for _, key := range sdkEnvBlacklist {
		for _, subapp := range names {
			prefix := strings.ToUpper(strings.ReplaceAll(subapp, "-", "_"))
			prefixed := prefix + "_" + key
			if v, ok := os.LookupEnv(prefixed); ok {
				log.Warn().
					Str("env", prefixed).
					Str("value", v).
					Str("subapp", subapp).
					Msg("sub-app ConfigMap leaked SDK env; ignored (use mega-shared-config)")
			}
		}
	}
}

// registerSubAppVersionMetric 注册 mega_subapp_version gauge（docs/12 §四）。
// 当前 v1 用 "unknown"（Wave 4 startup 后续会从 ldflags 注入真实版本）。
// sync.Once 保证多次调用幂等，避免 prometheus.MustRegister 重复 panic。
func (m *MultiResource) registerSubAppVersionMetric() {
	m.subappVersionOnce.Do(func() {
		g := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "mega_subapp_version",
				Help: "Sub-app version (Go module pseudo-version or commit SHA) loaded in this mega",
			},
			[]string{"subapp", "version"},
		)
		m.mu.Lock()
		apps := make([]registeredSubApp, len(m.subApps))
		copy(apps, m.subApps)
		m.mu.Unlock()
		for _, rs := range apps {
			if rs.app == nil {
				continue
			}
			g.WithLabelValues(rs.app.Name(), "unknown").Set(1)
		}
		prometheus.MustRegister(g)
	})
}
