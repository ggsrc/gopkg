// listener.go — ListenerSpec / PortMap / RouteRegistry + 端口分配 + 多 listener
// gRPC/HTTP server 工厂。
//
// 详见 mega-project/docs/10-framework-spec.md §四。
//
// 文件所有权：
//   - 类型 stub（ListenerSpec / PortMap / RouteRegistry / RegisterOption / registerConfig）→ Wave 0（我）
//   - 行为实现（OnPorts / WithRequirePreferredPort / processSubApp /
//     subappForListener / GrpcServerOn / HttpRouterOn）→ Wave 1 agent B
package rpcutil

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// ListenerSpec 描述 sub-app 需要的一个 listener。
type ListenerSpec struct {
	// Name 在 sub-app 内唯一的标识，如 "grpc" / "http-rest" / "custome-web"。
	Name string
	// Protocol 是 "grpc" 或 "http"。
	Protocol string
	// PreferredPort 是 sub-app 历史端口偏好。
	// mega main 通常会显式覆盖（OnPorts(PortMap{...})）；
	// 仅在 mega main 未指定时使用，且 framework 启动期会打印 warn。
	PreferredPort int
}

// PortMap 是 mega main 给某 sub-app 的端口分配：listener name → 实际端口。
type PortMap map[string]int

// RouteRegistry 让 sub-app 在 RegisterRoutes 时按 listener name 拿 server / router。
// framework 实现会查 listener → port → grpc.Server / gin.Engine 映射。
type RouteRegistry interface {
	GRPC(listenerName string) *grpc.Server
	HTTP(listenerName string) *gin.Engine
}

// RegisterOption 是 Register 接受的 sub-app 选项（如 OnPorts）。
type RegisterOption func(*registerConfig)

type registerConfig struct {
	portMap          PortMap
	requirePreferred bool
}

// OnPorts 是 Register 的 option，指定本 sub-app 各 listener 的实际端口。
func OnPorts(pm PortMap) RegisterOption {
	return func(c *registerConfig) { c.portMap = pm }
}

// WithRequirePreferredPort 强制 sub-app 的 PreferredPort 不可被覆盖。
// 罕见场景：sub-app 历史协议绑死端口。
func WithRequirePreferredPort() RegisterOption {
	return func(c *registerConfig) { c.requirePreferred = true }
}

// mergeRegisterOptions 合并多个 RegisterOption。
func mergeRegisterOptions(opts ...RegisterOption) registerConfig {
	var c registerConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// processSubApp 在 Register 期间处理一个 sub-app：
//   - 校验 Listeners vs OnPorts 一致（缺失 listener 报错）
//   - 校验端口在 mega 内唯一
//   - 创建 gRPC server / gin engine（per port）
//   - 把 portMap 存进 m.portMap
//
// 详见 docs/10 §四 实现细节代码块。
func (m *MultiResource) processSubApp(app SubApp, opts ...RegisterOption) error {
	cfg := mergeRegisterOptions(opts...)

	specs := app.Listeners()
	portMap := cfg.portMap
	if portMap == nil {
		portMap = PortMap{}
		for _, spec := range specs {
			portMap[spec.Name] = spec.PreferredPort
		}
		log.Warn().
			Str("subapp", app.Name()).
			Interface("ports", portMap).
			Msg("no explicit OnPorts; using PreferredPort fallback")
	}

	for _, spec := range specs {
		port, ok := portMap[spec.Name]
		if !ok {
			return fmt.Errorf("subapp=%s listener=%s: no port in OnPorts",
				app.Name(), spec.Name)
		}

		if port != spec.PreferredPort {
			if cfg.requirePreferred {
				return fmt.Errorf(
					"subapp=%s listener=%s: port %d != required preferred %d",
					app.Name(), spec.Name, port, spec.PreferredPort)
			}
			log.Warn().
				Str("subapp", app.Name()).Str("listener", spec.Name).
				Int("preferred", spec.PreferredPort).Int("actual", port).
				Msg("port differs from preferred (override accepted)")
		}

		// Atomic "check port free + reserve subapp/listener -> port" under one
		// lock — fixes F5 TOCTOU where two concurrent Register calls could both
		// pass the port-taken check and stomp the same port. Server creation
		// (GrpcServerOn / HttpRouterOn) re-acquires m.mu separately; the
		// reservation persists across that lock-drop so the subapp lookup inside
		// the interceptor chain still sees the right value.
		m.mu.Lock()
		if existingSubapp, existingListener := lookupListenerLocked(m.portMap, port); existingSubapp != "" {
			m.mu.Unlock()
			return fmt.Errorf(
				"port %d already taken by subapp=%s listener=%s",
				port, existingSubapp, existingListener)
		}
		if m.portMap == nil {
			m.portMap = map[string]int{}
		}
		m.portMap[app.Name()+"."+spec.Name] = port
		m.mu.Unlock()

		switch spec.Protocol {
		case "grpc":
			m.GrpcServerOn(port)
		case "http":
			m.HttpRouterOn(port)
		default:
			// Best-effort rollback of the just-inserted mapping; impact is
			// limited (Register fails fast → mega aborts) but keeps state tidy.
			m.mu.Lock()
			delete(m.portMap, app.Name()+"."+spec.Name)
			m.mu.Unlock()
			return fmt.Errorf("subapp=%s listener=%s: unknown protocol %q (want grpc|http)",
				app.Name(), spec.Name, spec.Protocol)
		}
	}
	return nil
}

// subappForListener 给定 port 查所属 sub-app 名字（反向 portMap）。
// 用于 interceptor 注入 subapp label。
func (m *MultiResource) subappForListener(port int) string {
	subapp, _ := m.lookupListenerByPort(port)
	return subapp
}

// lookupListenerByPort scans m.portMap for an entry whose value matches port.
// Returns (subapp, listenerName) or ("", "") if not found.
// Holds m.mu while iterating.
func (m *MultiResource) lookupListenerByPort(port int) (string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return lookupListenerLocked(m.portMap, port)
}

// lookupListenerLocked is the lock-free variant used by callers that already
// hold m.mu. Returns (subapp, listenerName) or ("", "") if not found.
func lookupListenerLocked(portMap map[string]int, port int) (string, string) {
	for key, p := range portMap {
		if p != port {
			continue
		}
		if idx := strings.Index(key, "."); idx >= 0 {
			return key[:idx], key[idx+1:]
		}
		return key, ""
	}
	return "", ""
}

// GrpcServerOn 返回 port 对应的 *grpc.Server，按需创建。
// 详见 docs/12 §一 注入 chain。
//
// Defensive defaults (caller-supplied opts win on conflict — placed AFTER the
// defaults in the slice):
//   - MaxRecvMsgSize 8 MiB (grpc-go default is 4 MiB; bounds payload size).
//     Sub-apps doing batch upload / image / indexer payloads MUST override
//     with grpc.MaxRecvMsgSize(N) — see README §gRPC defaults.
//   - MaxConcurrentStreams 1024 (cap goroutine fan-out from one client).
//   - Keepalive enforcement: reject clients pinging more often than 30s.
//   - Keepalive params: MaxConnectionIdle 5min (close unused conns) + ping
//     probe Time/Timeout (detect half-dead conns).
//
// Intentionally NOT applied (would force long-lived streaming clients to
// reconnect on a fixed cadence, surprising sub-apps doing SSE / server-
// streaming RPC / pub-sub fan-out):
//   - MaxConnectionAge (no default; opt-in only)
//
// Addresses PR #115 Round-7 S-001 + Round-8 cross-review #2 (drop opinionated
// MaxConnectionAge).
func (m *MultiResource) GrpcServerOn(port int, opts ...grpc.ServerOption) *grpc.Server {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.grpcServers == nil {
		m.grpcServers = map[int]*grpc.Server{}
	}
	if s, ok := m.grpcServers[port]; ok {
		return s
	}
	// Round-8 cross-review #6: reject post-Start creation. routeRegistry.GRPC
	// (called inside Phase 2 RegisterRoutes) only ever LOOKS UP servers
	// already-created via Register's processSubApp path, so it should hit
	// the `if s, ok := m.grpcServers[port]; ok` branch above. A late call
	// (e.g. from a misbehaving sub-app Init that tries to register a brand-
	// new port) would create a server whose listener Phase 1 has already
	// missed — return nil so the caller fails fast.
	if m.shutdownStarted.Load() || m.phase1BindStarted.Load() {
		log.Warn().Int("port", port).Msg("GrpcServerOn called after Phase 1 began; refusing to create new server")
		return nil
	}
	m.ensureGrpcPrometheus()
	subapp := lookupSubappLocked(m.portMap, port)
	defaults := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(grpcDefaultMaxRecvMsgSize),
		grpc.MaxConcurrentStreams(grpcDefaultMaxConcurrentStreams),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second,
			PermitWithoutStream: false,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			// MaxConnectionAge intentionally not set — see godoc.
			Time:    1 * time.Minute,
			Timeout: 20 * time.Second,
		}),
		grpc.ChainUnaryInterceptor(
			m.subappLoggerUnaryInterceptor(subapp),
			m.panicRecoverUnaryInterceptor(subapp),
			m.metricUnaryInterceptor(subapp),
			grpc_prometheus.UnaryServerInterceptor,
		),
		grpc.ChainStreamInterceptor(
			m.subappLoggerStreamInterceptor(subapp),
			m.panicRecoverStreamInterceptor(subapp),
			m.metricStreamInterceptor(subapp),
			grpc_prometheus.StreamServerInterceptor,
		),
	}
	s := grpc.NewServer(append(defaults, opts...)...)
	m.grpcServers[port] = s
	return s
}

const (
	grpcDefaultMaxRecvMsgSize       = 8 << 20 // 8 MiB
	grpcDefaultMaxConcurrentStreams = 1024
)

// HttpRouterOn 返回 port 对应的 *gin.Engine，按需创建。
// 详见 docs/12 §一 中间件链。
//
// Round-8 cross-review #6: refuses to create a new engine once Phase 1 has
// begun (would leak a router with no listener). Returns nil in that case;
// caller should pre-check m.startedAt or treat nil as configuration error.
func (m *MultiResource) HttpRouterOn(port int) *gin.Engine {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.httpRouters == nil {
		m.httpRouters = map[int]*gin.Engine{}
	}
	if e, ok := m.httpRouters[port]; ok {
		return e
	}
	if m.shutdownStarted.Load() || m.phase1BindStarted.Load() {
		log.Warn().Int("port", port).Msg("HttpRouterOn called after Phase 1 began; refusing to create new engine")
		return nil
	}
	// Inline lookup to avoid re-entering m.mu (m.subappForListener locks);
	// safe because we already hold m.mu. May be "" if processSubApp hasn't
	// recorded the listener yet — middleware handles "" gracefully.
	subapp := lookupSubappLocked(m.portMap, port)
	e := gin.New()
	e.Use(m.subappLoggerHttpMiddleware(subapp))
	e.Use(m.panicRecoverHttpMiddleware(subapp))
	e.Use(m.metricHttpMiddleware(subapp))
	m.httpRouters[port] = e
	return e
}
