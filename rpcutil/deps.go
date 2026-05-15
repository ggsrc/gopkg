// deps.go — DBConfig / BudgetConfig + RegisterDB / DBFor / CacheFor +
// ValidateBudget.
//
// 详见 mega-project/docs/10-framework-spec.md §五、§六。
//
// 文件所有权:
//   - 类型 stub (DBConfig / BudgetConfig) → Wave 0
//   - 行为实现 (RegisterDB / DBFor / CacheFor / ValidateBudget +
//     subappCache prefix wrapper + envconfig prefix glue) → Wave 2 agent F
package rpcutil

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/stumble/dcache"
	"github.com/stumble/wpgx"
)

// DBConfig 是 sub-app DB pool 配置。
//
// 与老 gopkg/database/wpgx 区别: EnvPrefix 让每个 sub-app 有独立 env namespace
// (如 "STAKING_POSTGRES")。详见 docs/10 §五 envconfig prefix 表格。
type DBConfig struct {
	// EnvPrefix 是 envconfig.Process 的第一参数, 如 "STAKING_POSTGRES"。
	EnvPrefix string
	// MaxConns 推荐 10–15。受 ValidateBudget 总预算约束。
	MaxConns int
	// MinConns 推荐 2。
	MinConns int
	// MaxConnLifetime 推荐 30min。
	MaxConnLifetime time.Duration
	// RequiredEnv 启动期 ValidateEnv 校验必含的 (不带 EnvPrefix) 短名。
	RequiredEnv []string
}

// BudgetConfig 是 ValidateBudget 的输入: PG 实例容量 + 安全系数。
type BudgetConfig struct {
	InstanceMaxConnections int
	SuperuserReserved      int
	SafetyFactor           float64
	AdminPoolReserved      int
}

// Cache 是 framework 暴露给 sub-app 的 KV/cache 抽象 (subset of dcache.DCache)。
//
// 选用 interface 而非 struct embed 的理由: embed 会同时暴露 *dcache.DCache
// 的所有非 prefix 方法 → caller 可绕过 prefix 直接调底层。interface 物理屏蔽
// 这种 bypass, 把 "所有 key 必须带 subapp prefix" 变成编译期 contract。
//
// 方法签名严格按 dcache@v0.3.0 DCache 同名方法拷贝, 保证 zero-cost 转发。
type Cache interface {
	Get(ctx context.Context, key string, target any, expire time.Duration, read dcache.ReadFunc, noCache bool, noStore bool) error
	GetWithTtl(ctx context.Context, key string, target any, read dcache.ReadWithTtlFunc, noCache bool, noStore bool) error
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
	Invalidate(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

// subappCache 给 Cache 调用的 key 自动加 "{subapp}:" 前缀, 保证多 sub-app
// 共享 Redis 时彼此 namespace 隔离。
//
// underlying == nil 时所有方法返回 error (典型场景: Wave 3 未初始化 dcache)。
type subappCache struct {
	underlying *dcache.DCache
	prefix     string // 例: "airdrop:"
}

func (s *subappCache) key(k string) string { return s.prefix + k }

func (s *subappCache) errIfNil() error {
	if s == nil || s.underlying == nil {
		return fmt.Errorf("rpcutil: cache not initialized (subapp prefix %q)", s.prefix)
	}
	return nil
}

func (s *subappCache) Get(ctx context.Context, key string, target any, expire time.Duration, read dcache.ReadFunc, noCache bool, noStore bool) error {
	if err := s.errIfNil(); err != nil {
		return err
	}
	return s.underlying.Get(ctx, s.key(key), target, expire, read, noCache, noStore)
}

func (s *subappCache) GetWithTtl(ctx context.Context, key string, target any, read dcache.ReadWithTtlFunc, noCache bool, noStore bool) error {
	if err := s.errIfNil(); err != nil {
		return err
	}
	return s.underlying.GetWithTtl(ctx, s.key(key), target, read, noCache, noStore)
}

func (s *subappCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	if err := s.errIfNil(); err != nil {
		return err
	}
	return s.underlying.Set(ctx, s.key(key), val, ttl)
}

func (s *subappCache) Invalidate(ctx context.Context, key string) error {
	if err := s.errIfNil(); err != nil {
		return err
	}
	return s.underlying.Invalidate(ctx, s.key(key))
}

func (s *subappCache) Ping(ctx context.Context) error {
	if err := s.errIfNil(); err != nil {
		return err
	}
	return s.underlying.Ping(ctx)
}

// newPoolFunc 是 wpgx.NewPool 的可注入 indirection。测试用 t.Cleanup 替换。
// 默认指向真实 wpgx.NewPool, 生产路径零开销。
var newPoolFunc = func(ctx context.Context, cfg *wpgx.Config) (*wpgx.Pool, error) {
	return wpgx.NewPool(ctx, cfg)
}

// envconfigMu serializes the os.Setenv → envconfig.Process → os.Unsetenv
// sequence inside processWpgxEnvWithAppNameFallback so concurrent RegisterDB
// calls don't race on the global env. PR #115 review G7 fix (gemini).
var envconfigMu sync.Mutex

// processWpgxEnvWithAppNameFallback runs envconfig.Process(prefix, &wpgx.Config)
// while temporarily injecting {prefix}_APPNAME=fallback if it's not already
// set. The env mutation is serialized + reverted, so the global state
// observed outside this function never changes (no leak, no race).
func processWpgxEnvWithAppNameFallback(prefix, fallback string) (wpgx.Config, error) {
	appNameKey := prefix + "_APPNAME"

	envconfigMu.Lock()
	defer envconfigMu.Unlock()

	original, hadOriginal := os.LookupEnv(appNameKey)
	if !hadOriginal {
		_ = os.Setenv(appNameKey, fallback)
		defer func() { _ = os.Unsetenv(appNameKey) }()
	} else {
		// keep original; revert is a no-op (lookup still hadOriginal).
		_ = original
	}

	var out wpgx.Config
	if err := envconfig.Process(prefix, &out); err != nil {
		return wpgx.Config{}, err
	}
	return out, nil
}

// RegisterDB 注册一个 sub-app 的 DB pool。
//
// 步骤:
//  1. name 唯一性校验
//  2. envconfig.Process(EnvPrefix) 填充 wpgx.Config
//  3. RequiredEnv 显式存在性校验 (envconfig 不会 fail 在 default value 上)
//  4. 用户层 override (MaxConns/MinConns/MaxConnLifetime)
//  5. newPoolFunc 建池 + 注册 close func
func (m *MultiResource) RegisterDB(name string, cfg DBConfig) error {
	if name == "" {
		return fmt.Errorf("rpcutil: RegisterDB requires non-empty name")
	}
	if cfg.EnvPrefix == "" {
		return fmt.Errorf("rpcutil: RegisterDB %q requires non-empty EnvPrefix", name)
	}

	m.mu.Lock()
	if m.dbPools == nil {
		m.dbPools = map[string]*wpgx.Pool{}
	}
	if m.dbConfigs == nil {
		m.dbConfigs = map[string]DBConfig{}
	}
	if _, exists := m.dbConfigs[name]; exists {
		m.mu.Unlock()
		return fmt.Errorf("rpcutil: db pool %q already registered", name)
	}
	m.mu.Unlock()

	// 校验 RequiredEnv 必须实际出现在 environment 里 (区别于 envconfig 默认值)。
	if missing := missingRequiredEnv(cfg.EnvPrefix, cfg.RequiredEnv); len(missing) > 0 {
		return fmt.Errorf("rpcutil: db pool %q missing required env: %s",
			name, strings.Join(missing, ", "))
	}

	// wpgx.Config.AppName has envconfig tag `required:"true"`, so we can't
	// satisfy it by pre-filling the struct — envconfig.Process always re-reads
	// env. Original code's os.Setenv-based fallback was racy across
	// concurrent RegisterDB calls (PR #115 review G7, gemini).
	//
	// Fix: serialize the env mutation under envconfigMu, restore the env
	// var to its pre-call state immediately. Env stays globally mutated for
	// the duration of envconfig.Process only, and concurrent RegisterDB calls
	// observe deterministic, atomic windows instead of races.
	wpgxCfg, err := processWpgxEnvWithAppNameFallback(cfg.EnvPrefix, name)
	if err != nil {
		return fmt.Errorf("rpcutil: db pool %q envconfig.Process(%q) failed: %w",
			name, cfg.EnvPrefix, err)
	}

	// 用户层 override 优先于 env (env 已经填好默认值)。
	if cfg.MaxConns > 0 {
		wpgxCfg.MaxConns = int32(cfg.MaxConns)
	}
	if cfg.MinConns > 0 {
		wpgxCfg.MinConns = int32(cfg.MinConns)
	}
	if cfg.MaxConnLifetime > 0 {
		wpgxCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}

	pool, err := newPoolFunc(context.Background(), &wpgxCfg)
	if err != nil {
		return fmt.Errorf("rpcutil: db pool %q NewPool failed: %w", name, err)
	}

	m.mu.Lock()
	// 双重检查 (RegisterDB 间并发竞争同名)。
	if _, exists := m.dbConfigs[name]; exists {
		m.mu.Unlock()
		if pool != nil {
			pool.Close()
		}
		return fmt.Errorf("rpcutil: db pool %q already registered", name)
	}
	m.dbPools[name] = pool
	m.dbConfigs[name] = cfg
	m.closeFuncs = append(m.closeFuncs, func(ctx context.Context) error {
		if pool != nil {
			pool.Close()
		}
		return nil
	})
	m.mu.Unlock()
	return nil
}

// missingRequiredEnv 返回 EnvPrefix + "_" + key 没有出现在 environment 里的 key 集合。
func missingRequiredEnv(prefix string, required []string) []string {
	var missing []string
	for _, k := range required {
		full := prefix + "_" + k
		if _, ok := os.LookupEnv(full); !ok {
			missing = append(missing, full)
		}
	}
	return missing
}

// DBFor 返回已注册的 sub-app pool。未注册 → 明确 error (不返回 nil pool)。
func (m *MultiResource) DBFor(name string) (*wpgx.Pool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pool, ok := m.dbPools[name]
	if !ok {
		return nil, fmt.Errorf("rpcutil: db pool not registered: %q", name)
	}
	return pool, nil
}

// CacheFor 返回带 "{subapp}:" prefix 的 Cache。
//
// 即使 m.dcache == nil (Wave 3 未初始化), 也返回非 nil wrapper; 调用方法时
// 才返回 error。这样 sub-app 在 Init 阶段可无脑 cache := deps.CacheFor("x")
// 不必判 nil。
func (m *MultiResource) CacheFor(subapp string) Cache {
	return &subappCache{
		underlying: m.dcache,
		prefix:     subapp + ":",
	}
}

// ValidateBudget 启动期硬校验 PG 总连接预算 (docs/10 §六 公式):
//
//	Σ(MaxConns) × maxReplicas + AdminPoolReserved
//	  ≤ (InstanceMaxConnections - SuperuserReserved) × SafetyFactor
func (m *MultiResource) ValidateBudget(maxReplicas int, cfg BudgetConfig) error {
	if maxReplicas <= 0 {
		return fmt.Errorf("rpcutil: ValidateBudget maxReplicas must be > 0, got %d", maxReplicas)
	}
	if cfg.SafetyFactor <= 0 {
		return fmt.Errorf("rpcutil: ValidateBudget SafetyFactor must be > 0, got %f", cfg.SafetyFactor)
	}

	m.mu.Lock()
	var subappTotal int
	names := make([]string, 0, len(m.dbConfigs))
	for n, dbCfg := range m.dbConfigs {
		subappTotal += dbCfg.MaxConns
		names = append(names, n)
	}
	m.mu.Unlock()
	sort.Strings(names)

	headroom := cfg.InstanceMaxConnections - cfg.SuperuserReserved
	budget := float64(headroom) * cfg.SafetyFactor
	used := float64(subappTotal*maxReplicas) + float64(cfg.AdminPoolReserved)

	if used > budget {
		return fmt.Errorf(
			"rpcutil: PG connection budget exceeded: needed %.0f, available %.0f "+
				"(subapp_max_conns_sum=%d × max_replicas=%d + admin_reserved=%d > "+
				"(instance_max=%d - superuser_reserved=%d) × safety_factor=%.2f); pools=%v",
			used, budget,
			subappTotal, maxReplicas, cfg.AdminPoolReserved,
			cfg.InstanceMaxConnections, cfg.SuperuserReserved, cfg.SafetyFactor,
			names,
		)
	}

	log.Info().
		Float64("used_conns", used).
		Float64("budget", budget).
		Int("subapp_max_conns_sum", subappTotal).
		Int("max_replicas", maxReplicas).
		Int("admin_reserved", cfg.AdminPoolReserved).
		Int("instance_max", cfg.InstanceMaxConnections).
		Int("superuser_reserved", cfg.SuperuserReserved).
		Float64("safety_factor", cfg.SafetyFactor).
		Strs("pools", names).
		Msg("rpcutil: PG connection budget OK")
	return nil
}
