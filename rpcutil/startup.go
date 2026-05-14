// startup.go — mega framework L1 Start (4-phase) + Stop (6-phase) +
// route registry adapter for SubApp.RegisterRoutes.
//
// 详见 mega-project/docs/11-framework-startup-shutdown.md §一 (Start)、§二
// (Stop)、§五 (signal handler 示例).
//
// 文件所有权：Wave 4 agent H. 本文件只读取 MultiResource 字段，不扩字段。
//
// Cardinal property: Phase 1 → 4 in Start; Phase 1 → 6 in Stop. NEVER reverse.
package rpcutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// 关停相关的统一超时常量（详见 docs/11 §二）：
//   - GracefulStopTimeout: Phase 2 每个 grpc/http server graceful 等待上限.
//   - ShutdownPhaseTimeout: Phase 3/4/5 子阶段单步 ctx timeout.
//   - HealthServerShutdownTimeout: Phase 6 health + metric server.
const (
	GracefulStopTimeout         = 30 * time.Second
	ShutdownPhaseTimeout        = 30 * time.Second
	HealthServerShutdownTimeout = 5 * time.Second
)

// healthListenPort / metricListenPort 在 spec 中固定（docs/11 §一 Phase 4）。
// 暴露为 var 是为了让单测覆写到 ephemeral 端口，避免与 host 上既有进程冲突。
var (
	healthListenPort = 8080
	metricListenPort = 4014
)

// ---- Start (4 phases) ------------------------------------------------------

// Start 同步启动整个 mega：
//
//  1. 绑定所有 grpc / http listener（fail fast）。
//  2. 对每个 sub-app 依次 Init → RegisterRoutes → 注册 HealthCheck。
//  3. 注册并启动 cron scheduler。
//  4. 启动 grpc.Serve / http.Serve / health endpoint / metric endpoint goroutines；
//     设置 startedAt 让 /health/ready 起来。
//
// 然后阻塞等 ctx.Done（典型来源：main 里 signal handler 收到 SIGTERM 后 cancel），
// 阻塞解除后调用 Stop(context.Background()) 触发 6 阶段反向关停。
//
// 任一 Phase 失败立即返回 wrapped error；本函数不主动调 Stop，
// 调用方（main）若 Start 失败可自行判断是否补救（通常 os.Exit(1)）。
func (m *MultiResource) Start(ctx context.Context) error {
	if err := m.phase1BindListeners(ctx); err != nil {
		return fmt.Errorf("phase1 bind listeners: %w", err)
	}
	if err := m.phase2InitSubApps(ctx); err != nil {
		return fmt.Errorf("phase2 init sub-apps: %w", err)
	}
	if err := m.phase3RegisterCron(ctx); err != nil {
		return fmt.Errorf("phase3 register cron: %w", err)
	}
	if err := m.phase4OpenHealth(ctx); err != nil {
		return fmt.Errorf("phase4 open health: %w", err)
	}

	// Idempotent version metric registration (sync.Once inside multi.go).
	m.registerSubAppVersionMetric()

	now := time.Now()
	m.startedAt.Store(&now)
	log.Info().
		Str("app", m.appName).
		Int("subapps", len(m.subApps)).
		Int("listeners", len(m.portMap)).
		Msg("mega started")

	// 阻塞直到 ctx 被取消（main 的 signal handler 在 SIGTERM 时 cancel）。
	<-ctx.Done()
	return m.Stop(context.Background())
}

// phase1BindListeners 按 m.grpcServers / m.httpRouters 中已注册的端口顺次
// net.Listen。任一失败立即返回；已绑定的 listener 仍存活，由 Stop 阶段或
// 进程退出自动释放（GC 关闭 fd）。
func (m *MultiResource) phase1BindListeners(ctx context.Context) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	for port := range m.grpcServers {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return fmt.Errorf("listen :%d: %w", port, err)
		}
		m.grpcListeners[port] = lis
	}
	for port := range m.httpRouters {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return fmt.Errorf("listen :%d: %w", port, err)
		}
		m.httpListeners[port] = lis
	}
	return nil
}

// phase2InitSubApps 对每个 sub-app 调 Init → RegisterRoutes →
// registerHealthCheck。失败立即返回，剩余 sub-app 不再处理。
func (m *MultiResource) phase2InitSubApps(ctx context.Context) error {
	_ = ctx
	apps := m.snapshotSubApps()
	for _, rs := range apps {
		if err := rs.app.Init(m); err != nil {
			return fmt.Errorf("subapp=%s Init: %w", rs.app.Name(), err)
		}
		reg := newRouteRegistry(m, rs.app.Name(), rs.portMap)
		if err := rs.app.RegisterRoutes(reg); err != nil {
			return fmt.Errorf("subapp=%s RegisterRoutes: %w", rs.app.Name(), err)
		}
		m.registerHealthCheck(rs.app.Name(), rs.app.HealthChecks())
	}
	return nil
}

// phase3RegisterCron 注册所有 sub-app cron job 并启动 scheduler。
// cronDisabled=true 时 registerCron 内部跳过实际注册，scheduler.Start 也跳过。
func (m *MultiResource) phase3RegisterCron(ctx context.Context) error {
	_ = ctx
	apps := m.snapshotSubApps()
	for _, rs := range apps {
		if err := m.registerCron(rs.app.Name(), rs.app.Cron()); err != nil {
			return err
		}
	}
	if !m.cronDisabled && m.scheduler != nil {
		m.scheduler.Start()
	}
	return nil
}

// phase4OpenHealth 启动 grpc / http / health / metric 四类 server goroutine。
// 创建 *http.Server 时把它写回 m.httpServers / m.healthServer / m.metricServer，
// Stop 阶段读取它们做 graceful Shutdown。
func (m *MultiResource) phase4OpenHealth(ctx context.Context) error {
	_ = ctx
	type grpcPair struct {
		srv *grpc.Server
		lis net.Listener
	}
	type httpPair struct {
		eng *gin.Engine
		lis net.Listener
		srv *http.Server
	}

	m.mu.Lock()
	grpcPairs := make(map[int]grpcPair, len(m.grpcServers))
	for port, srv := range m.grpcServers {
		lis, ok := m.grpcListeners[port].(net.Listener)
		if !ok {
			m.mu.Unlock()
			return fmt.Errorf("phase4: no listener bound for grpc port %d", port)
		}
		grpcPairs[port] = grpcPair{srv: srv, lis: lis}
	}

	httpPairs := make(map[int]httpPair, len(m.httpRouters))
	for port, eng := range m.httpRouters {
		lis, ok := m.httpListeners[port].(net.Listener)
		if !ok {
			m.mu.Unlock()
			return fmt.Errorf("phase4: no listener bound for http port %d", port)
		}
		srv := &http.Server{Handler: eng}
		m.httpServers[port] = srv
		httpPairs[port] = httpPair{eng: eng, lis: lis, srv: srv}
	}
	m.mu.Unlock()

	for port, pair := range grpcPairs {
		port, pair := port, pair
		go func() {
			if err := pair.srv.Serve(pair.lis); err != nil && err != grpc.ErrServerStopped {
				log.Error().Err(err).Int("port", port).Msg("grpc Serve exited")
			}
		}()
	}
	for port, pair := range httpPairs {
		port, pair := port, pair
		go func() {
			if err := pair.srv.Serve(pair.lis); err != nil && err != http.ErrServerClosed {
				log.Error().Err(err).Int("port", port).Msg("http Serve exited")
			}
		}()
	}

	// /health/{live,ready,debug}
	healthSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", healthListenPort),
		Handler: m.healthMux(),
	}
	m.mu.Lock()
	m.healthServer = healthSrv
	m.mu.Unlock()
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Int("port", healthListenPort).Msg("health Serve exited")
		}
	}()

	// /metrics (聚合所有 sub-app registry + DefaultGatherer)
	metricSrv := &http.Server{
		Addr: fmt.Sprintf(":%d", metricListenPort),
		Handler: promhttp.HandlerFor(m.gatherers, promhttp.HandlerOpts{
			ErrorHandling: promhttp.ContinueOnError,
		}),
	}
	m.mu.Lock()
	m.metricServer = metricSrv
	m.mu.Unlock()
	go func() {
		if err := metricSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Int("port", metricListenPort).Msg("metric Serve exited")
		}
	}()

	return nil
}

// ---- Stop (6 phases, reverse) ---------------------------------------------

// Stop 反向 6 阶段关停：
//
//  1. health off（clear startedAt → /health/ready 立刻 503 starting，
//     让 k8s 在剩余阶段开始前先把流量摘走）。
//  2. 并行 GracefulStop grpc / http server（每个 30s timeout，超时强 Stop/Close
//     并 inc shutdownAbortCounter）。
//  3. Stop cron scheduler（30s ctx）。
//  4. SubApp.Close 反向顺序调用（30s ctx 每个）。
//  5. closeFuncs（DB pool close 等）按 append 顺序调用（典型场景间无依赖）。
//  6. health + metric server Shutdown（5s ctx 各）。
//
// 任一子阶段错误仅 log + metric，不中断后续阶段（最大努力关停）。
func (m *MultiResource) Stop(ctx context.Context) error {
	_ = ctx
	log.Info().Str("app", m.appName).Msg("mega stopping")

	// Phase 1: health off
	if m.startedAt != nil {
		m.startedAt.Store(nil)
	}

	// Phase 2: grpc/http graceful stop in parallel
	m.stopServers()

	// Phase 3: stop cron
	if !m.cronDisabled && m.scheduler != nil {
		sCtx, cancel := context.WithTimeout(context.Background(), ShutdownPhaseTimeout)
		if err := m.scheduler.Stop(sCtx); err != nil {
			log.Warn().Err(err).Msg("scheduler stop error")
		}
		cancel()
	}

	// Phase 4: sub-app Close in reverse order
	apps := m.snapshotSubApps()
	for i := len(apps) - 1; i >= 0; i-- {
		rs := apps[i]
		cCtx, cancel := context.WithTimeout(context.Background(), ShutdownPhaseTimeout)
		if err := rs.app.Close(cCtx); err != nil {
			log.Warn().Err(err).Str("subapp", rs.app.Name()).Msg("Close error")
			shutdownAbortCounter.WithLabelValues(rs.app.Name(), "close").Inc()
		}
		cancel()
	}

	// Phase 5: shared resource close funcs (e.g. DB pools)
	m.mu.Lock()
	closes := make([]func(context.Context) error, len(m.closeFuncs))
	copy(closes, m.closeFuncs)
	m.mu.Unlock()
	for _, fn := range closes {
		fCtx, cancel := context.WithTimeout(context.Background(), ShutdownPhaseTimeout)
		_ = fn(fCtx)
		cancel()
	}

	// Phase 6: health + metric server (last so SRE can scrape until end)
	m.mu.Lock()
	hs, _ := m.healthServer.(*http.Server)
	ms, _ := m.metricServer.(*http.Server)
	m.mu.Unlock()
	if hs != nil {
		hCtx, cancel := context.WithTimeout(context.Background(), HealthServerShutdownTimeout)
		_ = hs.Shutdown(hCtx)
		cancel()
	}
	if ms != nil {
		mCtx, cancel := context.WithTimeout(context.Background(), HealthServerShutdownTimeout)
		_ = ms.Shutdown(mCtx)
		cancel()
	}

	log.Info().Msg("mega stopped")
	return nil
}

// stopServers 在 Phase 2 并行 GracefulStop grpc + http server。
// 超时 → 强制 Stop/Close 并 inc shutdownAbortCounter(subapp, "grpc"|"http").
func (m *MultiResource) stopServers() {
	m.mu.Lock()
	grpcSnapshot := make(map[int]*grpc.Server, len(m.grpcServers))
	for p, s := range m.grpcServers {
		grpcSnapshot[p] = s
	}
	httpSnapshot := make(map[int]*http.Server, len(m.httpServers))
	for p, srv := range m.httpServers {
		if s, ok := srv.(*http.Server); ok {
			httpSnapshot[p] = s
		}
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for port, srv := range grpcSnapshot {
		port, srv := port, srv
		wg.Add(1)
		go func() {
			defer wg.Done()
			done := make(chan struct{})
			go func() {
				srv.GracefulStop()
				close(done)
			}()
			select {
			case <-done:
				log.Info().Int("port", port).Msg("grpc stopped gracefully")
			case <-time.After(GracefulStopTimeout):
				log.Warn().Int("port", port).Msg("grpc graceful stop timeout; forcing")
				srv.Stop()
				shutdownAbortCounter.WithLabelValues(m.subAppByPort(port), "grpc").Inc()
			}
		}()
	}
	for port, srv := range httpSnapshot {
		port, srv := port, srv
		wg.Add(1)
		go func() {
			defer wg.Done()
			sCtx, cancel := context.WithTimeout(context.Background(), GracefulStopTimeout)
			defer cancel()
			if err := srv.Shutdown(sCtx); err != nil {
				log.Warn().Err(err).Int("port", port).Msg("http shutdown error")
				shutdownAbortCounter.WithLabelValues(m.subAppByPort(port), "http").Inc()
				_ = srv.Close()
			}
		}()
	}
	wg.Wait()
}

// snapshotSubApps 在锁内浅拷贝 m.subApps，供 phase2 / phase3 / Stop 无锁遍历。
func (m *MultiResource) snapshotSubApps() []registeredSubApp {
	m.mu.Lock()
	defer m.mu.Unlock()
	apps := make([]registeredSubApp, len(m.subApps))
	copy(apps, m.subApps)
	return apps
}

// ---- routeRegistry (RouteRegistry impl) -----------------------------------

// routeRegistry 在 phase2 给 sub-app.RegisterRoutes 用：按 listener 名字
// 查 portMap → port → 调用 m.GrpcServerOn / HttpRouterOn 返回已存在的 server /
// engine（GrpcServerOn / HttpRouterOn 自带幂等）。
type routeRegistry struct {
	m       *MultiResource
	subapp  string
	portMap PortMap
}

func newRouteRegistry(m *MultiResource, subapp string, pm PortMap) RouteRegistry {
	return &routeRegistry{m: m, subapp: subapp, portMap: pm}
}

func (r *routeRegistry) GRPC(listenerName string) *grpc.Server {
	port, ok := r.portMap[listenerName]
	if !ok {
		log.Warn().
			Str("subapp", r.subapp).
			Str("listener", listenerName).
			Msg("RouteRegistry.GRPC: unknown listener name")
		return nil
	}
	return r.m.GrpcServerOn(port)
}

func (r *routeRegistry) HTTP(listenerName string) *gin.Engine {
	port, ok := r.portMap[listenerName]
	if !ok {
		log.Warn().
			Str("subapp", r.subapp).
			Str("listener", listenerName).
			Msg("RouteRegistry.HTTP: unknown listener name")
		return nil
	}
	return r.m.HttpRouterOn(port)
}
