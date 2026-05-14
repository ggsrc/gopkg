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

	"github.com/gin-gonic/gin"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
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

		if existingSubapp, existingListener := m.lookupListenerByPort(port); existingSubapp != "" {
			return fmt.Errorf(
				"port %d already taken by subapp=%s listener=%s",
				port, existingSubapp, existingListener)
		}

		switch spec.Protocol {
		case "grpc":
			m.GrpcServerOn(port)
		case "http":
			m.HttpRouterOn(port)
		default:
			return fmt.Errorf("subapp=%s listener=%s: unknown protocol %q (want grpc|http)",
				app.Name(), spec.Name, spec.Protocol)
		}

		m.mu.Lock()
		if m.portMap == nil {
			m.portMap = map[string]int{}
		}
		m.portMap[app.Name()+"."+spec.Name] = port
		m.mu.Unlock()
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
	for key, p := range m.portMap {
		if p != port {
			continue
		}
		// key format: "subapp.listener" (listener name may itself contain dots
		// in pathological cases — split on the first dot only).
		if idx := strings.Index(key, "."); idx >= 0 {
			return key[:idx], key[idx+1:]
		}
		return key, ""
	}
	return "", ""
}

// GrpcServerOn 返回 port 对应的 *grpc.Server，按需创建。
// 详见 docs/12 §一 注入 chain。
func (m *MultiResource) GrpcServerOn(port int, opts ...grpc.ServerOption) *grpc.Server {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.grpcServers == nil {
		m.grpcServers = map[int]*grpc.Server{}
	}
	if s, ok := m.grpcServers[port]; ok {
		return s
	}
	m.ensureGrpcPrometheus()
	// Inline lookup to avoid re-entering m.mu (m.subappForListener locks);
	// safe because we already hold m.mu. May be "" if processSubApp hasn't
	// recorded the listener yet — interceptors handle "" gracefully.
	subapp := lookupSubappLocked(m.portMap, port)
	chained := append([]grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			m.subappLoggerUnaryInterceptor(subapp),
			m.panicRecoverUnaryInterceptor(subapp),
			grpc_prometheus.UnaryServerInterceptor,
		),
		grpc.ChainStreamInterceptor(
			m.subappLoggerStreamInterceptor(subapp),
			m.panicRecoverStreamInterceptor(subapp),
			grpc_prometheus.StreamServerInterceptor,
		),
	}, opts...)
	s := grpc.NewServer(chained...)
	m.grpcServers[port] = s
	return s
}

// HttpRouterOn 返回 port 对应的 *gin.Engine，按需创建。
// 详见 docs/12 §一 中间件链。
func (m *MultiResource) HttpRouterOn(port int) *gin.Engine {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.httpRouters == nil {
		m.httpRouters = map[int]*gin.Engine{}
	}
	if e, ok := m.httpRouters[port]; ok {
		return e
	}
	// Inline lookup to avoid re-entering m.mu (m.subappForListener locks);
	// safe because we already hold m.mu. May be "" if processSubApp hasn't
	// recorded the listener yet — middleware handles "" gracefully.
	subapp := lookupSubappLocked(m.portMap, port)
	e := gin.New()
	e.Use(m.subappLoggerHttpMiddleware(subapp))
	e.Use(m.panicRecoverHttpMiddleware(subapp))
	m.httpRouters[port] = e
	return e
}
