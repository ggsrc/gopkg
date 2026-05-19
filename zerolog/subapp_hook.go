// Package zerolog: subapp ctx hook (mega framework L1).
//
// 在 mega 模式下，多个 sub-app 共享同一进程；日志查询必须能按 sub-app 维度
// 过滤。该文件提供 ctx-based subapp 标记：framework gRPC/HTTP middleware 在
// 请求入口把 sub-app 名字塞进 ctx，下游业务代码通过 zerolog.Ctx(ctx) 拿到
// 带 "subapp=..." 字段的 logger。
//
// 详见 mega-project/docs/12-framework-observability.md §一 Layer 1。
package zerolog

import "context"

// subappCtxKey 是 ctx 中存放 sub-app 名字的 key 类型。
// 用空 struct 作为 key 防止外部代码冲突注入。
type subappCtxKey struct{}

// WithSubApp 把 sub-app 名字塞进 ctx。
//
// 由 framework gRPC/HTTP middleware 自动调用，业务代码不应直接使用。
// 后续 zerolog.Ctx(ctx) 派生的 logger 自动带 "subapp" 字段（见
// gopkg/rpcutil/multi.go 的 SubAppContext / LoggerFor 与 middleware 注入）。
func WithSubApp(ctx context.Context, subapp string) context.Context {
	return context.WithValue(ctx, subappCtxKey{}, subapp)
}

// SubAppFromCtx 从 ctx 取出 sub-app 名字。
// 第二返回值在 ctx 未带 subapp 时为 false。
func SubAppFromCtx(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(subappCtxKey{}).(string)
	return v, ok
}
