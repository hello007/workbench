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
//   - emitCurrent：发射唯一入口——持读锁期间完成取值与投递，与切换互斥；
//   - eventSink：仅测试/诊断读，业务 emit 调用点禁止经此两步发射（切换窗口
//     内事件会迟到旧出口）。
//
// 四个持 sink 服务（terminal/update/ai_function/git）嵌入本类型，方法提升
// 直达，emit 调用点统一走 s.emitCurrent(...)。桌面模式不调用 SetEventSink，
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

// emitCurrent 在持有读锁期间完成「取当前出口并投递」，供 service 层发射事件。
//
// 相比先 eventSink() 释放锁再 Emit 的两步形态，本方法把取值与投递收进同一
// RLock 域，与 SetEventSink 写锁互斥：任一条事件的投递期间出口不可被切换，
// 消除切换窗口内单条事件投递到旧出口的顺序缺口（serve 模式重启切换 hub 时，
// 浏览器可能收到错序事件）。Emit 在锁内执行要求下游实现不得回调本持有者的
// SetEventSink/eventSink（RWMutex 写锁等待下递归取读锁会死锁）——现有三个
// 实现（wailsEventSink/WSHub/multicastSink）均只触自身内部锁，满足约束。
func (h *sinkHolder) emitCurrent(name string, data ...any) {
	h.sinkMu.RLock()
	defer h.sinkMu.RUnlock()
	emitEvent(h.sink, name, data...)
}
