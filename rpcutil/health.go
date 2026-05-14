// health.go — Criticality / HealthCheckable + readiness / liveness / debug
// HTTP handler。
//
// 详见 mega-project/docs/10-framework-spec.md §八 +
// docs/12-framework-observability.md §三。
//
// 文件所有权：
//   - 类型 stub（Criticality / HealthCheckable + const）→ Wave 0（我）
//   - 行为实现（registerHealthCheck / healthMux / readinessHandler /
//     livenessHandler / debugHandler）→ Wave 1 agent C
package rpcutil

import (
	"context"
	"net/http"
)

// Criticality 表示一个 health check 的关键性等级。
type Criticality int

const (
	// Critical：失败 → 整 mega readiness false（k8s 摘流量）。
	Critical Criticality = iota
	// Optional：失败 → 仅 metric 上报，不影响 readiness。
	Optional
)

// String 用于日志和 debug endpoint。
func (c Criticality) String() string {
	if c == Critical {
		return "critical"
	}
	return "optional"
}

// HealthCheckable 描述 sub-app 的一个 dep 健康检查。
type HealthCheckable struct {
	// Name 是 dep 的人类可读名字，如 "airdrop-db" / "token-redis"。
	Name string
	// Criticality 影响 readiness 聚合行为。
	Criticality Criticality
	// Check 在 readiness handler 调用，必须 ctx-aware（2s timeout）。
	Check func(ctx context.Context) error
}

// registerHealthCheck 把 sub-app 的所有 HealthCheckable 注册到 framework。
// Wave 1 agent C 实现。
func (m *MultiResource) registerHealthCheck(subapp string, checks []HealthCheckable) {
	// stub: Wave 1 agent C
	_ = subapp
	_ = checks
}

// healthMux 返回 /health/* 路由 mux。
// Wave 1 agent C 实现：/health/live、/health/ready、/health/debug。
func (m *MultiResource) healthMux() http.Handler {
	return http.NewServeMux()
}

// readinessHandler 聚合所有 Critical dep 状态。
// 详见 docs/10 §八 + docs/12 §三。Wave 1 agent C 实现：
//   - startedAt == nil → 503 starting
//   - 任一 Critical 失败 → 503 + 失败列表
//   - 都 OK → 200 ready
func (m *MultiResource) readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
}

// livenessHandler 进程存活就 200，永远不检 dep。
func (m *MultiResource) livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// debugHandler 返回所有 dep 的详细状态（JSON）。
// 详见 docs/12 §三。Wave 1 agent C 实现。
func (m *MultiResource) debugHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
