package service

import "sync"

// sinkHolder EventSink 并发安全持有器。
//
// 背景：serve 模式（main.runServe）在 service 构造完成后经 SetEventSink 把
// 事件出口从 wails sink 切换为 WebSocket hub，而 terminal 输出泵、下载进度等
// 长期运行 goroutine 在切换窗口内持续读 sink 字段——裸字段赋值存在数据竞态。
// 读写锁集中封装：
//   - SetEventSink：运行期切换出口（对齐 UpdateService/GitService SetContext
//     的 setter 注入模式），serve 模式注入 WS hub 实现；
//   - eventSink：包内读取当前出口，emit 调用点经此取值。
//
// 四个持 sink 服务（terminal/update/ai_function/git）嵌入本类型，方法提升
// 直达，emit 调用点统一改为 s.eventSink()。桌面模式不调用 SetEventSink，
// 构造注入的 wails sink 行为不变（零回归）。
type sinkHolder struct {
	sinkMu sync.RWMutex
	sink   EventSink
}

// SetEventSink 运行期切换事件出口。sink 为 nil 时静默忽略——清空出口与
// 未注入语义等价（emitEvent 统一静默跳过），忽略可防误调用覆盖已生效出口。
func (h *sinkHolder) SetEventSink(sink EventSink) {
	if sink == nil {
		return
	}
	h.sinkMu.Lock()
	defer h.sinkMu.Unlock()
	h.sink = sink
}

// eventSink 读取当前事件出口；未注入（零值构造）时返回 nil，由 emitEvent
// 统一 nil 防护静默跳过。
func (h *sinkHolder) eventSink() EventSink {
	h.sinkMu.RLock()
	defer h.sinkMu.RUnlock()
	return h.sink
}
