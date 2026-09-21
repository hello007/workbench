package service

import (
	"sync"
	"testing"
	"time"

	"workbench/util/testutil"
)

// TestSetEventSink_SwitchOnAllServices 四个持 sink 服务（terminal/update/
// ai_function/git）经 SetEventSink 运行期切换出口——serve 模式注入 WS hub
// 时走同一入口；nil 入参忽略不清空已生效出口。
func TestSetEventSink_SwitchOnAllServices(t *testing.T) {
	fake := &fakeEventSink{}

	terminal := NewTerminalService(nil)
	terminal.SetEventSink(fake)
	if terminal.eventSink() != fake {
		t.Error("TerminalService 切换后应返回新出口")
	}

	update := NewUpdateService()
	update.SetEventSink(fake)
	if update.eventSink() != fake {
		t.Error("UpdateService 切换后应返回新出口")
	}

	ai := NewAiFunctionService(nil, "unused.json")
	ai.SetEventSink(fake)
	if ai.eventSink() != fake {
		t.Error("AiFunctionService 切换后应返回新出口")
	}

	git := NewGitService()
	git.SetEventSink(fake)
	if git.eventSink() != fake {
		t.Error("GitService 切换后应返回新出口")
	}

	// nil 入参忽略：不得覆盖已生效出口
	terminal.SetEventSink(nil)
	if terminal.eventSink() != fake {
		t.Error("SetEventSink(nil) 应忽略不清空")
	}
}

// TestSetEventSink_SwitchRedirectsEmit 切换后事件走新出口、不再走旧出口：
// 对应 serve 模式「事件从 wails sink 切到 WS hub」的指向断言（fake 模拟 hub）。
func TestSetEventSink_SwitchRedirectsEmit(t *testing.T) {
	svc := NewGitService()
	old := &fakeEventSink{}
	current := &fakeEventSink{}
	svc.SetEventSink(old)
	svc.SetEventSink(current)

	emitEvent(svc.eventSink(), "pull-complete", map[string]int{"success": 1})

	if len(old.calls) != 0 {
		t.Errorf("切换后不应再走旧出口, got %d 条", len(old.calls))
	}
	if len(current.calls) != 1 || current.calls[0].name != "pull-complete" {
		t.Errorf("切换后事件应走新出口: %#v", current.calls)
	}
}

// TestSetEventSink_ConcurrentSwitchAndEmit 切换竞态（-race 抽查）：模拟
// terminal 输出泵 goroutine 持续读出口与 serve 启动期 SetEventSink 切换
// 并发交错，读写锁保护下无数据竞态。时间量仅用于拉开交错窗口，不做时序断言。
func TestSetEventSink_ConcurrentSwitchAndEmit(t *testing.T) {
	svc := NewTerminalService(nil)

	fakes := []*fakeEventSink{{}, {}, {}}
	stop := make(chan struct{})

	// 切换方：serve 启动期反复注入不同出口
	var switchWG sync.WaitGroup
	switchWG.Add(1)
	go func() {
		defer switchWG.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				svc.SetEventSink(fakes[i%len(fakes)])
			}
		}
	}()

	// 读取方：输出泵 goroutine 语义，持续经 eventSink() 取当前出口发射
	var emitWG sync.WaitGroup
	for g := 0; g < 3; g++ {
		emitWG.Add(1)
		go func() {
			defer emitWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
					emitEvent(svc.eventSink(), "terminal-output", "session-1", "data")
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	switchWG.Wait()
	emitWG.Wait()
}

// TestBatchPull_EmitsViaSwitchedSink serve 模式端到端（service 层）：构造期
// 出口为 wails sink，SetEventSink 切换为 hub 形态出口后，真实 BatchPull 的
// pull-progress/pull-complete 事件全部走新出口（对应 runServe 装配行）。
func TestBatchPull_EmitsViaSwitchedSink(t *testing.T) {
	repo := testutil.InitTempRepo(t) // 无远程配置，走跳过路径（仍推 progress+complete）
	svc := NewGitService()
	hubFake := &fakeEventSink{}
	svc.SetEventSink(hubFake) // runServe: gitSvc.SetEventSink(hub) 同构

	results := svc.BatchPull([]string{repo}, 1)
	if len(results) != 1 {
		t.Fatalf("应返回 1 个结果, got %d", len(results))
	}

	var progress, complete int
	for _, c := range hubFake.calls {
		switch c.name {
		case "pull-progress":
			progress++
		case "pull-complete":
			complete++
		default:
			t.Errorf("不应推送其他事件: %q", c.name)
		}
	}
	if progress != 1 || complete != 1 {
		t.Errorf("切换后事件应全部走 hub 出口: progress=%d complete=%d", progress, complete)
	}
}
