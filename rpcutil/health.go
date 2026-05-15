// health.go — Criticality / HealthCheckable + readiness / liveness / debug
// HTTP handler。
//
// 详见 mega-project/docs/10-framework-spec.md §八 +
// docs/12-framework-observability.md §三。
//
// 文件所有权：
//   - 类型 stub（Criticality / HealthCheckable + const）→ Wave 0
//   - 行为实现（registerHealthCheck / healthMux / readinessHandler /
//     livenessHandler / debugHandler）→ Wave 1 agent C
package rpcutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// readinessCheckTimeout is the per-dep ctx timeout used by readiness/debug.
// Aligns with docs/10 §八 readinessHandler code block.
const readinessCheckTimeout = 2 * time.Second

// depUnhealthyCounter tracks individual dep failures observed during a
// readiness/debug evaluation. Labels: subapp / dep_name / criticality.
// Intentionally lives here (not in observability.go) — see the
// "framework metric vars (consolidated)" header note in observability.go
// for the agreed split. Stays per-file to avoid a no-op cross-package diff.
var depUnhealthyCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mega_dep_unhealthy_total",
		Help: "Total number of unhealthy responses observed per dependency.",
	},
	[]string{"subapp", "dep_name", "criticality"},
)

func init() {
	prometheus.MustRegister(depUnhealthyCounter)
}

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
//
// 并发安全：用 m.mu 序列化 append。
func (m *MultiResource) registerHealthCheck(subapp string, checks []HealthCheckable) {
	if len(checks) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.healthChecks == nil {
		m.healthChecks = make([]registeredHealthCheck, 0, len(checks))
	}
	for _, hc := range checks {
		m.healthChecks = append(m.healthChecks, registeredHealthCheck{
			subapp: subapp,
			hc:     hc,
		})
	}
}

// healthMux 返回 /health/* 路由 mux。
func (m *MultiResource) healthMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", m.livenessHandler)
	mux.HandleFunc("/health/ready", m.readinessHandler)
	mux.HandleFunc("/health/debug", m.debugHandler)
	return mux
}

// livenessHandler 进程存活就 200，永远不检 dep。
func (m *MultiResource) livenessHandler(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "alive")
}

// readinessHandler 聚合所有 Critical dep 状态。详见 docs/10 §八。
//   - startedAt 尚未 Store → 503 starting
//   - 任一 Critical 失败 → 503 + 失败列表
//   - 都 OK → 200 ready
//
// Optional 失败仅 inc metric，不影响状态。
//
// Checks fan out in parallel — for a mega with N sub-apps each declaring a
// few HealthCheckables, the total time is max(Check) not Σ(Check) so even
// a hundred deps comfortably fit into the k8s readinessProbe timeout (2s
// in our patch.json). Addresses PR #115 review G3 (gemini).
func (m *MultiResource) readinessHandler(w http.ResponseWriter, r *http.Request) {
	if m.startedAt == nil || m.startedAt.Load() == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, "starting")
		return
	}

	results := m.runHealthChecksParallel(r.Context())

	failed := make([]string, 0)
	for _, res := range results {
		if res.err == nil {
			continue
		}
		depUnhealthyCounter.
			WithLabelValues(res.subapp, res.name, res.criticality.String()).
			Inc()
		if res.criticality == Critical {
			failed = append(failed, fmt.Sprintf("%s.%s: %s", res.subapp, res.name, res.err))
		}
	}

	if len(failed) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, "unhealthy critical deps:")
		for _, f := range failed {
			fmt.Fprintln(w, "  -", f)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ready")
}

// debugHandler 返回所有 dep 的详细状态（JSON），SRE 用。详见 docs/12 §三。
// Shares the parallel check path with readinessHandler.
func (m *MultiResource) debugHandler(w http.ResponseWriter, r *http.Request) {
	resp := healthDebugResponse{MegaName: m.appName}
	if m.startedAt != nil {
		resp.StartedAt = m.startedAt.Load()
	}

	results := m.runHealthChecksParallel(r.Context())

	bySubApp := make(map[string][]healthCheckResult)
	for _, res := range results {
		entry := healthCheckResult{
			Name:        res.name,
			Criticality: res.criticality.String(),
			Status:      "ok",
			DurationMs:  res.durationMs,
		}
		if res.err != nil {
			entry.Status = "error"
			entry.Error = res.err.Error()
		}
		bySubApp[res.subapp] = append(bySubApp[res.subapp], entry)
	}

	names := m.orderedSubAppNames(bySubApp)
	resp.SubApps = make([]subAppHealthDetail, 0, len(names))
	for _, name := range names {
		resp.SubApps = append(resp.SubApps, subAppHealthDetail{
			Name:         name,
			HealthChecks: bySubApp[name],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// checkResult is the per-dep outcome from a parallel sweep.
type checkResult struct {
	subapp      string
	name        string
	criticality Criticality
	err         error
	durationMs  int64
}

// runHealthChecksParallel fans out every snapshotted HealthCheckable into a
// goroutine. Each check gets its own timeout. Returns results in the same
// order as the snapshot so callers that need deterministic ordering get it.
func (m *MultiResource) runHealthChecksParallel(parentCtx context.Context) []checkResult {
	checks := m.snapshotHealthChecks()
	if len(checks) == 0 {
		return nil
	}
	results := make([]checkResult, len(checks))
	var wg sync.WaitGroup
	wg.Add(len(checks))
	for i, rhc := range checks {
		i, rhc := i, rhc
		go func() {
			defer wg.Done()
			start := time.Now()
			err := runCheckWithTimeout(parentCtx, rhc.hc.Check, readinessCheckTimeout)
			results[i] = checkResult{
				subapp:      rhc.subapp,
				name:        rhc.hc.Name,
				criticality: rhc.hc.Criticality,
				err:         err,
				durationMs:  time.Since(start).Milliseconds(),
			}
		}()
	}
	wg.Wait()
	return results
}

// snapshotHealthChecks 在锁内返回 m.healthChecks 的浅拷贝。
func (m *MultiResource) snapshotHealthChecks() []registeredHealthCheck {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.healthChecks) == 0 {
		return nil
	}
	out := make([]registeredHealthCheck, len(m.healthChecks))
	copy(out, m.healthChecks)
	return out
}

// orderedSubAppNames 收集所有出现过的 subapp 名字（来自 m.subApps + extra
// map），按字母序排序，保证 debug 输出稳定。
func (m *MultiResource) orderedSubAppNames(extra map[string][]healthCheckResult) []string {
	seen := make(map[string]struct{}, len(extra)+len(m.subApps))
	m.mu.Lock()
	for _, rs := range m.subApps {
		if rs.app == nil {
			continue
		}
		seen[rs.app.Name()] = struct{}{}
	}
	m.mu.Unlock()
	for name := range extra {
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// runCheckWithTimeout 用独立的 derived ctx 跑一次 Check 并保证 timeout 真的生效。
//
// 关键点: 不能直接 `return fn(ctx)` — `context.WithTimeout` 只 cancel ctx,
// 不会中断不 honor ctx 的 Check 函数。一个写得不规范的 sub-app Check (例如
// 调一个 net.Conn.Read 没设 deadline) 会让 readiness handler 卡到 Check 自己
// 返回为止, 超出 k8s readinessProbe timeout (典型 1s) 导致 pod 被错误地标为
// 不健康。
//
// 实现: 在子 goroutine 里跑 fn,主 goroutine select 在 done / ctx.Done。timeout
// 后保证返回 ctx.Err() 给 caller,**子 goroutine 可能继续跑**(leak 一个 goroutine
// 直到 fn 自己返回)—— 这是写得不规范的 Check 的 deliberate trade-off:
// 宁可 leak 一个 goroutine 也不能拖死整个 readiness 探针。Prometheus 上的
// runtime.NumGoroutine 会暴露这类泄漏给 SRE。
//
// Addresses PR #115 Round-9 cross-review: readiness deadline must hold even
// when sub-app Check ignores ctx.
func runCheckWithTimeout(parent context.Context, fn func(context.Context) error, timeout time.Duration) error {
	if fn == nil {
		return nil
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	// Buffered chan size 1 so a slow fn that returns AFTER we've already
	// committed to ctx.Err() doesn't block forever trying to send into a
	// receiverless chan (which would also pin the goroutine).
	done := make(chan error, 1)
	go func() {
		// Defensive: if fn panics, recover and surface the panic as an error
		// so the parent goroutine doesn't block on a never-arrived send.
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("health check panic: %v", r)
			}
		}()
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ---- JSON shape for /health/debug (docs/12 §三) --------------------------

type healthDebugResponse struct {
	MegaName  string               `json:"mega_name"`
	StartedAt *time.Time           `json:"started_at"`
	SubApps   []subAppHealthDetail `json:"subapps"`
}

type subAppHealthDetail struct {
	Name         string              `json:"name"`
	HealthChecks []healthCheckResult `json:"health_checks"`
}

type healthCheckResult struct {
	Name        string `json:"name"`
	Criticality string `json:"criticality"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
}
