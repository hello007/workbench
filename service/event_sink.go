package service

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// EventSink service 层向前端推送事件的统一出口抽象。
//
// 背景：原 service 层散落直调 runtime.EventsEmit，且 nil-ctx 防护不一致
// （terminal.go 两处 terminal-exit 无防护，ctx 为 nil 时触发 wails runtime
// log.Fatalf；ai_function.go/git.go 各有独立守卫）。收敛为单一出口后：
//   - 桌面模式：注入 Wails 实现（NewWailsEventSink），行为与原直调一致；
//   - 浏览器 serve 模式（后续 PR）：注入 WebSocket hub 实现，service 零改动。
//
// nil 防护集中在本文件：sink 为 nil、实现内部 ctx 为 nil 或非 Wails 上下文
// （无 events 键）时静默返回，等价原 safeEmit 的守卫语义并覆盖全部触点。
type EventSink interface {
	// Emit 推送命名事件，data 为变参载荷（与 Wails EventsEmit 语义一致）。
	Emit(name string, data ...any)
}

// wailsCtxEventsKey 与 Wails 桌面上下文中的 events 能力键一致（wails v2
// pkg/runtime 以字符串 "events" 存入 context）。
// 非 Wails 上下文（context.Background、单测环境）取值为 nil，须跳过发射，
// 否则 runtime.EventsEmit 内部 log.Fatalf。
const wailsCtxEventsKey = "events"

// wailsEventSink EventSink 的 Wails 桌面实现，内部转发 runtime.EventsEmit。
type wailsEventSink struct {
	ctx context.Context
}

// NewWailsEventSink 构造 Wails 桌面模式事件出口。ctx 允许为 nil（单测、
// 启动早期等无 Wails 上下文场景），Emit 时集中防护静默返回。
func NewWailsEventSink(ctx context.Context) EventSink {
	return &wailsEventSink{ctx: ctx}
}

// Emit 集中 nil 防护后转发 runtime.EventsEmit：
//   - 实现指针为 nil（经类型化 nil 指针调用接口）时静默返回；
//   - ctx 为 nil 或非 Wails 上下文（无 events 键）时静默返回。
func (s *wailsEventSink) Emit(name string, data ...any) {
	if s == nil || s.ctx == nil || s.ctx.Value(wailsCtxEventsKey) == nil {
		return
	}
	runtime.EventsEmit(s.ctx, name, data...)
}

// emitEvent 通过 sink 推送事件；sink 为 nil 接口（服务零值构造、构造后未注入）
// 时静默返回。service 层统一经此 helper 发射，调用点无需重复判空。
func emitEvent(sink EventSink, name string, data ...any) {
	if sink == nil {
		return
	}
	sink.Emit(name, data...)
}
