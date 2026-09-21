package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"workbench/model"
)

// fakeEventSink 记录 Emit 调用的假事件出口，用于断言事件名与载荷透传。
// Emit 加锁：SetEventSink 切换竞态测试中多个 goroutine 并发写同一实例。
type fakeEventSink struct {
	mu    sync.Mutex
	calls []fakeEmitCall
}

type fakeEmitCall struct {
	name string
	data []any
}

func (f *fakeEventSink) Emit(name string, data ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeEmitCall{name: name, data: data})
}

// TestEmitEvent_Passthrough emitEvent 将事件名与变参载荷原样透传给 sink。
func TestEmitEvent_Passthrough(t *testing.T) {
	fake := &fakeEventSink{}
	emitEvent(fake, "ai-task:done", "payload1", 2)

	if len(fake.calls) != 1 {
		t.Fatalf("应记录 1 次调用, got %d", len(fake.calls))
	}
	if fake.calls[0].name != "ai-task:done" {
		t.Errorf("事件名不符: got %q", fake.calls[0].name)
	}
	if len(fake.calls[0].data) != 2 || fake.calls[0].data[0] != "payload1" || fake.calls[0].data[1] != 2 {
		t.Errorf("载荷透传不符: got %#v", fake.calls[0].data)
	}
}

// TestEmitEvent_NilSink_NoPanic sink 为 nil 接口（服务零值构造）时静默返回不 panic。
func TestEmitEvent_NilSink_NoPanic(t *testing.T) {
	emitEvent(nil, "event", "data")
}

// TestWailsEventSink_NilCtx_NoPanic nil ctx 静默跳过，修复原 terminal.go 两处
// 直调 EventsEmit 的 nil-ctx fatal 隐患。
func TestWailsEventSink_NilCtx_NoPanic(t *testing.T) {
	sink := NewWailsEventSink(nil)
	sink.Emit("terminal-exit", "session-1")
}

// TestWailsEventSink_NonWailsCtx_NoPanic 非 Wails 上下文（无 events 键）静默跳过，
// 等价原 safeEmit 守卫语义。
func TestWailsEventSink_NonWailsCtx_NoPanic(t *testing.T) {
	sink := NewWailsEventSink(context.Background())
	sink.Emit("terminal-output", "session-1", "data")
}

// TestWailsEventSink_TypedNilReceiver_NoPanic 类型化 nil 指针调用 Emit 不 panic。
func TestWailsEventSink_TypedNilReceiver_NoPanic(t *testing.T) {
	var sink *wailsEventSink
	sink.Emit("event")
}

// TestTerminalService_ConstructorInjectsSink 构造时注入事件出口，nil ctx 不报错。
func TestTerminalService_ConstructorInjectsSink(t *testing.T) {
	svc := NewTerminalService(nil)
	if svc.eventSink() == nil {
		t.Fatal("NewTerminalService 应注入事件出口")
	}
	// nil ctx 下 Emit 静默跳过，不得 panic/fatal
	svc.eventSink().Emit("terminal-exit", "session-1")
}

// TestTerminalService_Emit_ThroughSink 换注入 fake sink 后事件经出口透传。
func TestTerminalService_Emit_ThroughSink(t *testing.T) {
	svc := NewTerminalService(nil)
	fake := &fakeEventSink{}
	svc.SetEventSink(fake)

	emitEvent(svc.eventSink(), "terminal-output", "session-1", "output-text")

	if len(fake.calls) != 1 {
		t.Fatalf("应记录 1 次调用, got %d", len(fake.calls))
	}
	if fake.calls[0].name != "terminal-output" {
		t.Errorf("事件名不符: got %q", fake.calls[0].name)
	}
	if len(fake.calls[0].data) != 2 || fake.calls[0].data[0] != "session-1" {
		t.Errorf("载荷透传不符: got %#v", fake.calls[0].data)
	}
}

// TestAiFunctionService_Emit_ThroughSink AiFunctionService.emit 经出口透传；
// sink 未注入（零值构造）时静默不 panic。
func TestAiFunctionService_Emit_ThroughSink(t *testing.T) {
	zero := &AiFunctionService{}   // sink=nil
	zero.emit("ai-task:done", "x") // 不应 panic

	svc := NewAiFunctionService(nil, "unused.json")
	fake := &fakeEventSink{}
	svc.SetEventSink(fake)

	svc.emit("ai-task:queued", 1, "two")

	if len(fake.calls) != 1 {
		t.Fatalf("应记录 1 次调用, got %d", len(fake.calls))
	}
	if fake.calls[0].name != "ai-task:queued" {
		t.Errorf("事件名不符: got %q", fake.calls[0].name)
	}
}

// TestGitService_Emit_ThroughSink GitService 批量拉取事件经出口透传；
// sink 未注入（零值构造）时静默不 panic——替代原 safeEmit 收敛前的守卫测试。
func TestGitService_Emit_ThroughSink(t *testing.T) {
	zero := &GitService{}                                            // sink 未注入
	emitEvent(zero.eventSink(), "pull-progress", model.PullResult{}) // 不应 panic

	svc := NewGitService()
	fake := &fakeEventSink{}
	svc.SetEventSink(fake)

	emitEvent(svc.eventSink(), "pull-complete", map[string]int{"success": 1})

	if len(fake.calls) != 1 {
		t.Fatalf("应记录 1 次调用, got %d", len(fake.calls))
	}
	if fake.calls[0].name != "pull-complete" {
		t.Errorf("事件名不符: got %q", fake.calls[0].name)
	}
}

// TestDownloadUpdate_EmitsProgressViaSink 下载完成后经 sink 推送完成事件
// （httptest 本地回环模拟下载源，不经真实网络）。
func TestDownloadUpdate_EmitsProgressViaSink(t *testing.T) {
	cleanupUpdateDir(t)
	defer cleanupUpdateDir(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake exe content"))
	}))
	defer srv.Close()

	svc := NewUpdateService()
	fake := &fakeEventSink{}
	svc.SetEventSink(fake)

	if err := svc.DownloadUpdate(srv.URL); err != nil {
		t.Fatalf("DownloadUpdate: %v", err)
	}

	completed := false
	for _, c := range fake.calls {
		if c.name != "update:download-progress" {
			t.Errorf("不应推送其他事件: %q", c.name)
			continue
		}
		if p, ok := c.data[0].(model.DownloadProgress); ok && p.Completed {
			completed = true
			if p.Percent != 100 {
				t.Errorf("完成事件 Percent 应为 100, got %v", p.Percent)
			}
		}
	}
	if !completed {
		t.Error("完成事件未经 sink 推送")
	}
}
