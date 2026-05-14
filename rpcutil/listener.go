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
	"github.com/gin-gonic/gin"
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
// Wave 1 agent B 实现。
func OnPorts(pm PortMap) RegisterOption {
	return func(c *registerConfig) { c.portMap = pm }
}

// WithRequirePreferredPort 强制 sub-app 的 PreferredPort 不可被覆盖。
// 罕见场景：sub-app 历史协议绑死端口。
// Wave 1 agent B 实现。
func WithRequirePreferredPort() RegisterOption {
	return func(c *registerConfig) { c.requirePreferred = true }
}

// mergeRegisterOptions 合并多个 RegisterOption。Wave 1 helper。
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
// Wave 1 agent B 实现。详见 docs/10 §四 实现细节代码块。
func (m *MultiResource) processSubApp(app SubApp, opts ...RegisterOption) error {
	return errNotImplemented
}

// subappForListener 给定 port 查所属 sub-app 名字（反向 portMap）。
// 用于 interceptor 注入 subapp label。Wave 1 agent B 实现。
func (m *MultiResource) subappForListener(port int) string {
	return ""
}

// GrpcServerOn 返回 port 对应的 *grpc.Server，按需创建并挂上所有 interceptor。
// 详见 docs/12 §一 注入 chain。Wave 1 agent B 实现（interceptor 函数本身在
// observability.go 由 Wave 2 agent E 实现；Wave 1 这里直接引用其签名）。
func (m *MultiResource) GrpcServerOn(port int, opts ...grpc.ServerOption) *grpc.Server {
	return nil
}

// HttpRouterOn 返回 port 对应的 *gin.Engine，按需创建并挂上所有 middleware。
// 详见 docs/12 §一 中间件链。Wave 1 agent B 实现。
func (m *MultiResource) HttpRouterOn(port int) *gin.Engine {
	return nil
}
