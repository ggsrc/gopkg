// cron.go — CronJob 声明 + Scheduler 抽象 + 默认 gocron v2 wrapper +
// MEGA_DISABLE_CRON 机制 + panic recover + metric 上报。
//
// 详见 mega-project/docs/10-framework-spec.md §七。
//
// 文件所有权：
//   - 类型 stub（CronJob struct + Scheduler interface）→ Wave 0（我）
//   - 行为实现（registerCron / wrapCronFn / 默认 Scheduler impl
//     based on gocron v2 / cronJobs 唯一性校验 / metric 上报）→ Wave 1 agent D
package rpcutil

import (
	"context"
)

// CronJob 描述 sub-app 的一个定时任务。
//
// framework 注册时自动加 "{subapp}." 前缀，包装 panic recover + metric。
type CronJob struct {
	// Name 不带 subapp 前缀的短名，如 "daily-reward-claim"。
	Name string
	// Schedule 是 crontab 表达式（与 gocron v2 CronJob 语法一致）。
	Schedule string
	// Fn 是要执行的函数，必须 ctx-aware（用于 graceful shutdown 取消）。
	Fn func(ctx context.Context) error
}

// Scheduler 是 cron 后端抽象。
//
// 默认实现包装 gocron v2（github.com/go-co-op/gocron/v2）。
// 抽出 interface 是为了单测可 mock。
type Scheduler interface {
	// RegisterCronJob 注册一个定时任务（带预处理后的 fullName，已含 subapp 前缀）。
	RegisterCronJob(name, schedule string, fn func()) error
	// Start 启动调度器。非阻塞。
	Start()
	// Stop graceful 停止，等待 in-flight job 完成或 ctx timeout。
	Stop(ctx context.Context) error
}

// registerCron 把 sub-app 的所有 CronJob 注册到 framework scheduler。
// 自动加 "{subapp}." 前缀，校验 fullName 唯一，包装 wrapCronFn。
// MEGA_DISABLE_CRON=true 时跳过。Wave 1 agent D 实现。
func (m *MultiResource) registerCron(subapp string, jobs []CronJob) error {
	return errNotImplemented
}

// wrapCronFn 把业务 Fn 包装成调度器需要的 func()：
//   - 注入 sub-app ctx（zerolog 带 subapp 字段）
//   - defer recover()，panic 时上报 mega_cron_panic_total
//   - 上报 mega_cron_duration_seconds_bucket 和 mega_cron_error_total
//
// Wave 1 agent D 实现。详见 docs/10 §七 wrapCronFn 代码块。
func (m *MultiResource) wrapCronFn(subapp, name string, fn func(context.Context) error) func() {
	return func() {}
}

// newDefaultScheduler 返回基于 gocron v2 的默认 Scheduler 实现。
// Wave 1 agent D 实现：包装 gocron.NewScheduler(...) + 翻译 schedule string
// 到 gocron 的 CronJob definition。
func newDefaultScheduler() (Scheduler, error) {
	return nil, errNotImplemented
}
