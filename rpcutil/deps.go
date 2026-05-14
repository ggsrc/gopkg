// deps.go — DBConfig / BudgetConfig + RegisterDB / DBFor / CacheFor +
// ValidateBudget。
//
// 详见 mega-project/docs/10-framework-spec.md §五、§六。
//
// 文件所有权：
//   - 类型 stub（DBConfig / BudgetConfig）→ Wave 0（我）
//   - 行为实现（RegisterDB / DBFor / CacheFor / ValidateBudget +
//     subappCache prefix wrapper + envconfig prefix glue）→ Wave 2 agent F
package rpcutil

import (
	"time"

	"github.com/stumble/dcache"
	"github.com/stumble/wpgx"
)

// DBConfig 是 sub-app DB pool 配置。
//
// 与老 gopkg/database/wpgx 区别：EnvPrefix 让每个 sub-app 有独立 env namespace
// （如 "STAKING_POSTGRES"）。详见 docs/10 §五 envconfig prefix 表格。
type DBConfig struct {
	// EnvPrefix 是 envconfig.Process 的第一参数，如 "STAKING_POSTGRES"。
	EnvPrefix string
	// MaxConns 推荐 10–15。受 ValidateBudget 总预算约束。
	MaxConns int
	// MinConns 推荐 2。
	MinConns int
	// MaxConnLifetime 推荐 30min。
	MaxConnLifetime time.Duration
	// RequiredEnv 启动期 ValidateEnv 校验必含的（不带 EnvPrefix）短名。
	RequiredEnv []string
}

// BudgetConfig 是 ValidateBudget 的输入：PG 实例容量 + 安全系数。
type BudgetConfig struct {
	InstanceMaxConnections int
	SuperuserReserved      int
	SafetyFactor           float64
	AdminPoolReserved      int
}

// RegisterDB 注册一个 sub-app 的 DB pool。Wave 2 agent F 实现。
func (m *MultiResource) RegisterDB(name string, cfg DBConfig) error {
	return errNotImplemented
}

// DBFor 返回已注册的 sub-app pool。Wave 2 agent F 实现。
func (m *MultiResource) DBFor(name string) (*wpgx.Pool, error) {
	return nil, errNotImplemented
}

// CacheFor 返回带 "{subapp}:" 前缀的 dcache wrapper。Wave 2 agent F 实现。
func (m *MultiResource) CacheFor(subapp string) *dcache.DCache {
	return m.dcache
}

// ValidateBudget 启动期硬校验 PG 总连接预算。Wave 2 agent F 实现。
func (m *MultiResource) ValidateBudget(maxReplicas int, cfg BudgetConfig) error {
	return errNotImplemented
}
