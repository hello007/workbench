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

	// 读取方：输出泵 goroutine 语义，经 emitCurrent 持读锁发射（与生产调用
	// 路径一致：取值与投递在同一 RLock 域内）
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
					svc.emitCurrent("terminal-output", "session-1", "data")
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	switchWG.Wait()
	emitWG.Wait()
}

// probeSink 可观测 Emit 生命周期的假出口：每条事件 Emit 期间标记 inEmit，
// 供断言「投递期间出口不可被切换」（emitCurrent 持读锁投递语义）。
type probeSink struct {
	mu       sync.Mutex
	names    []string
	emitting bool
}

func (p *probeSink) Emit(name string, data ...any) {
	p.mu.Lock()
	p.emitting = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.emitting = false
		p.mu.Unlock()
	}()
	if name == "slow" {
		// 拉长本条事件的投递时长，制造与 SetEventSink 的交错窗口
		time.Sleep(50 * time.Millisecond)
	}
	p.mu.Lock()
	p.names = append(p.names, name)
	p.mu.Unlock()
}

// TestSinkHolder_EmitCurrentUnderLockNoStaleDelivery emitCurrent 持读锁投递：
// 旧出口某条事件的 Emit 尚未返回时，SetEventSink 切换被锁阻塞、新出口不生效，
// 因此新出口首条事件的投递必然晚于旧出口在途 Emit 结束——消除旧实现（取值与
// 投递分两步、中间释放锁）切换窗口内事件迟到旧出口、新出口先于旧出口收到
// 事件的顺序缺口。
func TestSinkHolder_EmitCurrentUnderLockNoStaleDelivery(t *testing.T) {
	h := &sinkHolder{}
	slow := &probeSink{}
	fast := &probeSink{}
	h.SetEventSink(slow)

	emitDone := make(chan struct{})
	go func() {
		defer close(emitDone)
		h.emitCurrent("slow", "payload") // 投递期间 slow.Emit 睡眠 50ms
	}()

	// 等待 slow 真正进入 Emit（inEmit=true）后再切换，保证交错窗口成立
	deadline := time.Now().Add(5 * time.Second)
	for {
		slow.mu.Lock()
		emitting := slow.emitting
		slow.mu.Unlock()
		if emitting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("等待 slow 进入 Emit 超时")
		}
		time.Sleep(1 * time.Millisecond)
	}

	// 切换方：阻塞至 slow 的 Emit 返回（读锁释放）后才生效并完成新投递
	h.SetEventSink(fast)
	h.emitCurrent("fast-first", "payload")

	// fast 收到事件时 slow 的在途 Emit 必须已结束（顺序缺口消除的断言点）
	slow.mu.Lock()
	emitting := slow.emitting
	slowNames := append([]string(nil), slow.names...)
	slow.mu.Unlock()
	if emitting {
		t.Error("切换后新出口投递时旧出口的在途 Emit 不应仍在进行（持锁投递被违反）")
	}
	if len(slowNames) != 1 || slowNames[0] != "slow" {
		t.Errorf("旧出口应恰好收到切换前的 1 条事件: %v", slowNames)
	}
	fast.mu.Lock()
	fastNames := append([]string(nil), fast.names...)
	fast.mu.Unlock()
	if len(fastNames) != 1 || fastNames[0] != "fast-first" {
		t.Errorf("新出口应恰好收到切换后的 1 条事件: %v", fastNames)
	}

	<-emitDone
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
