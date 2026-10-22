//go:build windows

// 素材缩略图 COM 实现可注入面单测（Windows 原生，零 COM 真调用——COM 真链路归
// 集成冒烟验证）：页级 Export 瞬态重试（修复 1）+ Discard 重建 owned 退出等待
// （修复 2）+ owned PID 持久化与上会话残留回收判定（D7）。退避/轮询间隔经包级
// var 注入 0 或极小值，禁 sleep 等待竞态（test-stability 契约）；平台耦合断言
// 独立 _windows_test.go 文件（linux-platform 契约）。

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-ole/go-ole"

	"workbench/util"
)

// transientExportErr 按 go-ole 真实形态构造瞬态错误：DISP_E_EXCEPTION + 空
// ExcepInfo（go-ole Invoke 失败分支把 null bstrDescription 经 renderStrings 渲染
// 为字面量 "<nil>"——真机日志「发生意外。 (<nil>)」同源形态）。
func transientExportErr() *ole.OleError {
	return ole.NewErrorWithSubError(dispEException, "<nil>", ole.EXCEPINFO{})
}

// withThumbExportRetryParams 注入页级重试参数（保存/恢复包级 var，包内串行无竞态）。
func withThumbExportRetryParams(t *testing.T, maxAttempts int, delay time.Duration) {
	t.Helper()
	oldMax, oldDelay := thumbExportMaxAttempts, thumbExportRetryDelay
	thumbExportMaxAttempts, thumbExportRetryDelay = maxAttempts, delay
	t.Cleanup(func() { thumbExportMaxAttempts, thumbExportRetryDelay = oldMax, oldDelay })
}

// withThumbOwnedExitWaitParams 注入 owned 退出等待参数与探针（抽象注入点，
// 勿直调全局 officeServerPIDSet 枚举）。
func withThumbOwnedExitWaitParams(t *testing.T, interval, waitCap time.Duration, probe func() map[int]bool) {
	t.Helper()
	oldInterval, oldWait, oldProbe := thumbOwnedExitPollInterval, thumbOwnedExitWait, thumbOwnedExitProbe
	thumbOwnedExitPollInterval, thumbOwnedExitWait, thumbOwnedExitProbe = interval, waitCap, probe
	t.Cleanup(func() {
		thumbOwnedExitPollInterval, thumbOwnedExitWait, thumbOwnedExitProbe = oldInterval, oldWait, oldProbe
	})
}

// TestIsTransientExportError 瞬态判定口径表驱动：真机形态（0x80020009 + 空
// ExcepInfo「<nil>」/空串）与描述含「发生意外」均瞬态；实质业务描述、文件损坏
// 类、非 COM 包装错误、ctx 取消/超时（前置判定）均非瞬态。
func TestIsTransientExportError(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		transient bool
	}{
		{"真机形态：0x80020009+空ExcepInfo字面量nil", ole.NewErrorWithSubError(dispEException, "<nil>", ole.EXCEPINFO{}), true},
		{"0x80020009+空描述", ole.NewErrorWithDescription(dispEException, ""), true},
		{"描述含发生意外", ole.NewErrorWithDescription(dispEException, "发生意外。"), true},
		{"描述含发生意外但其他hr", ole.NewErrorWithDescription(0x8000FFFF, "发生意外。"), true},
		{"0x80020009但实质描述不赌重试", ole.NewErrorWithDescription(dispEException, "页序超出范围"), false},
		{"文件损坏类其他hr", ole.NewErrorWithDescription(0x8000FFFF, "文件已损坏"), false},
		{"非COM包装错误", errors.New("COM 打开失败"), false},
		{"ctx取消前置判定", fmt.Errorf("invoke: %w", context.Canceled), false},
		{"ctx超时前置判定", fmt.Errorf("invoke: %w", context.DeadlineExceeded), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientExportError(tc.err); got != tc.transient {
				t.Fatalf("isTransientExportError = %v, want %v（err=%v）", got, tc.transient, tc.err)
			}
		})
	}
}

// TestRetryTransientSlideExport_TransientThenSuccess 修复 1 主路径：前两次瞬态
// 第三次成功 → 重试至成功且返回 nil（导出继续）。
func TestRetryTransientSlideExport_TransientThenSuccess(t *testing.T) {
	withThumbExportRetryParams(t, 3, 0)
	calls := 0
	err := retryTransientSlideExport(context.Background(), 1, "small", func() error {
		calls++
		if calls < 3 {
			return transientExportErr()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("瞬态消除后应成功: %v", err)
	}
	if calls != 3 {
		t.Fatalf("应尝试 3 次后成功, got %d", calls)
	}
}

// TestRetryTransientSlideExport_AlwaysTransientExhausts 恒瞬态：3 次尽返回最后
// 一次错误，错误信息追加「已重试 2 次」定位标记，attempt 恰执行 3 次。
func TestRetryTransientSlideExport_AlwaysTransientExhausts(t *testing.T) {
	withThumbExportRetryParams(t, 3, 0)
	calls := 0
	err := retryTransientSlideExport(context.Background(), 2, "hd", func() error {
		calls++
		return transientExportErr()
	})
	if err == nil {
		t.Fatal("恒瞬态应返回错误")
	}
	if calls != 3 {
		t.Fatalf("应尝试 3 次, got %d", calls)
	}
	if !strings.Contains(err.Error(), "已重试 2 次") {
		t.Fatalf("错误应含「已重试」定位标记: %v", err)
	}
	if !strings.Contains(err.Error(), "第 2 页导出 hd 档失败") {
		t.Fatalf("错误应保留既有定位格式: %v", err)
	}
}

// TestRetryTransientSlideExport_CtxCanceledZeroRetry ctx 已取消：零重试立即返回
// （取消/超时错误绝不重试——判定前置）。
func TestRetryTransientSlideExport_CtxCanceledZeroRetry(t *testing.T) {
	withThumbExportRetryParams(t, 3, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := retryTransientSlideExport(ctx, 1, "small", func() error {
		calls++
		return transientExportErr()
	})
	if err == nil {
		t.Fatal("应返回错误")
	}
	if calls != 1 {
		t.Fatalf("ctx 取消后零重试, got %d 次尝试", calls)
	}
	if strings.Contains(err.Error(), "已重试") {
		t.Fatalf("未发生重试不应带「已重试」标记: %v", err)
	}
}

// TestRetryTransientSlideExport_CancelDuringBackoff 退避等待中取消：立即从退避
// 返回不再重试。大退避值下 select 必走 ctx.Done 分支，chan 交接替代 sleep
// （首次 attempt 通知后取消，不依赖时序竞态）。
func TestRetryTransientSlideExport_CancelDuringBackoff(t *testing.T) {
	withThumbExportRetryParams(t, 3, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstAttempt := make(chan struct{}, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- retryTransientSlideExport(ctx, 1, "small", func() error {
			firstAttempt <- struct{}{} // 退避基准 1h：第二次 attempt 不会发生，恰一次发送
			return transientExportErr()
		})
	}()
	<-firstAttempt // 首次失败已发生（即将/已进入退避 select）
	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("取消后应返回错误")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("退避中取消未及时返回（select 未消费 ctx.Done）")
	}
}

// TestRetryTransientSlideExport_NonTransientNoRetry 非瞬态（文件损坏类 OleError、
// 实质业务描述、普通 error）：零重试立即返回，单次失败错误格式与既有逐字一致。
func TestRetryTransientSlideExport_NonTransientNoRetry(t *testing.T) {
	withThumbExportRetryParams(t, 3, 0)
	cases := []struct {
		name string
		err  error
	}{
		{"文件损坏类OleError", ole.NewErrorWithDescription(0x8000FFFF, "文件已损坏")},
		{"DISP_E_EXCEPTION但实质描述", ole.NewErrorWithDescription(dispEException, "页序超出范围")},
		{"普通error", errors.New("COM 打开失败")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := retryTransientSlideExport(context.Background(), 1, "small", func() error {
				calls++
				return tc.err
			})
			if err == nil {
				t.Fatal("应返回错误")
			}
			if calls != 1 {
				t.Fatalf("非瞬态零重试, got %d 次尝试", calls)
			}
			if !strings.Contains(err.Error(), "第 1 页导出 small 档失败") {
				t.Fatalf("错误格式应与既有逐字一致: %v", err)
			}
			if strings.Contains(err.Error(), "已重试") {
				t.Fatalf("零重试不应带「已重试」标记: %v", err)
			}
		})
	}
}

// TestWaitForOwnedProcessesExit_ReturnsWhenPIDGone 修复 2：owned PID 两轮探针后
// 消失 → 等待立即结束（重建放行），探针恰消费 3 轮（前两轮存活、第三轮消失）。
func TestWaitForOwnedProcessesExit_ReturnsWhenPIDGone(t *testing.T) {
	calls := 0
	withThumbOwnedExitWaitParams(t, time.Millisecond, 50*time.Millisecond, func() map[int]bool {
		calls++
		if calls < 3 {
			return map[int]bool{4242: true}
		}
		return map[int]bool{}
	})
	waitForOwnedProcessesExit(map[int]bool{4242: true})
	if calls != 3 {
		t.Fatalf("应轮询至 PID 消失（第 3 轮）, got %d 轮", calls)
	}
}

// TestWaitForOwnedProcessesExit_CapsAndProceedsOnTimeout 恒存活：等待上限到达后
// 返回继续重建（CreateInstance 0x8001010A 重试链兜底），不永挂。
func TestWaitForOwnedProcessesExit_CapsAndProceedsOnTimeout(t *testing.T) {
	withThumbOwnedExitWaitParams(t, time.Millisecond, 20*time.Millisecond, func() map[int]bool {
		return map[int]bool{4242: true} // 恒存活
	})
	done := make(chan struct{})
	go func() {
		waitForOwnedProcessesExit(map[int]bool{4242: true})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("等待上限后应返回继续重建（超时未退由 CreateInstance 重试链兜底）")
	}
}

// TestWaitForOwnedProcessesExit_EmptyOwnedSkipsProbe 复用用户实例路径（owned 空）：
// 不等待不探针——禁杀语义下 Quit 本就未调用。
func TestWaitForOwnedProcessesExit_EmptyOwnedSkipsProbe(t *testing.T) {
	probed := false
	withThumbOwnedExitWaitParams(t, time.Millisecond, 50*time.Millisecond, func() map[int]bool {
		probed = true
		return nil
	})
	waitForOwnedProcessesExit(nil)
	if probed {
		t.Fatal("owned 集为空不应触发探针（复用用户实例路径禁等待）")
	}
}

// TestWaitForOwnedProcessesExit_TimeoutReturnsStillAliveSubset 垂死实例困局修复
// 的返回值契约：超时未退时返回仍存活的 PID 子集——调用方并回 w.ownedPIDs 恢复
// 归属自证（不并回则 Quit 失败的残留 owned 实例被误归「复用用户实例」禁杀）；
// 已退出成员从返回集剔除。
func TestWaitForOwnedProcessesExit_TimeoutReturnsStillAliveSubset(t *testing.T) {
	calls := 0
	withThumbOwnedExitWaitParams(t, time.Millisecond, 20*time.Millisecond, func() map[int]bool {
		calls++
		if calls == 1 {
			return map[int]bool{4242: true, 108: true}
		}
		return map[int]bool{4242: true} // 108 退出、4242 恒存活
	})
	stillAlive := waitForOwnedProcessesExit(map[int]bool{4242: true, 108: true})
	if len(stillAlive) != 1 || !stillAlive[4242] {
		t.Fatalf("超时应返回仍存活的子集 {4242}, got %v", stillAlive)
	}
}

// TestWaitForOwnedProcessesExit_AllExitedReturnsEmpty 返回值契约另一面：全部退出
// 返回空集（调用方零并回，不污染 diff 归属判定）。
func TestWaitForOwnedProcessesExit_AllExitedReturnsEmpty(t *testing.T) {
	withThumbOwnedExitWaitParams(t, time.Millisecond, 50*time.Millisecond, func() map[int]bool {
		return map[int]bool{}
	})
	stillAlive := waitForOwnedProcessesExit(map[int]bool{4242: true})
	if len(stillAlive) != 0 {
		t.Fatalf("全部退出应返回空集, got %v", stillAlive)
	}
}

// ---- owned PID 持久化与残留回收判定（D7）----

// TestOwnedPIDsFileRoundTrip 持久化 round-trip：persistOwnedMirror 落盘 →
// loadOwnedPIDsFile 读回一致；空镜像发布（releaseApp 语义）落盘 {"pids":[]}，
// 读回空集合——本会话已知自身无 owned 实例，旧残留集据此失效。
func TestOwnedPIDsFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), comOwnedPIDsFileName)
	e := &goOleThumbExporter{ownedPersistPath: path}

	e.ownedMu.Lock()
	e.ownedMirror = map[int]bool{4242: true, 108: true}
	e.ownedMu.Unlock()
	e.persistOwnedMirror()

	got := loadOwnedPIDsFile(path)
	if len(got) != 2 || !got[4242] || !got[108] {
		t.Fatalf("round-trip 读回不一致: %v", got)
	}

	e.ownedMu.Lock()
	e.ownedMirror = nil
	e.ownedMu.Unlock()
	e.persistOwnedMirror()
	if got := loadOwnedPIDsFile(path); len(got) != 0 {
		t.Fatalf("空镜像应读回空集合: %v", got)
	}
}

// TestPersistOwnedMirror_EmptyPathSkips 持久化路径空（非 Windows 桩/单测构造
// 形态）：不落盘任何文件。
func TestPersistOwnedMirror_EmptyPathSkips(t *testing.T) {
	dir := t.TempDir()
	e := &goOleThumbExporter{ownedPersistPath: ""}
	e.ownedMu.Lock()
	e.ownedMirror = map[int]bool{4242: true}
	e.ownedMu.Unlock()
	e.persistOwnedMirror()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读临时目录失败: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("空持久化路径不应产生文件: %v", entries)
	}
}

// TestLoadOwnedPIDsFile_StaleMtimeFiltered mtime 超 24h：视同无残留（os.Chtimes
// 注入陈旧时间驱动——test-stability 契约禁 mtime 写入时序依赖，注入时间戳合规）。
func TestLoadOwnedPIDsFile_StaleMtimeFiltered(t *testing.T) {
	path := filepath.Join(t.TempDir(), comOwnedPIDsFileName)
	if err := util.SaveJSON(path, ownedPIDsFilePayload{PIDs: []int{4242}}); err != nil {
		t.Fatalf("写持久化文件失败: %v", err)
	}
	stale := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(path, stale, stale); err != nil {
		t.Fatalf("注入陈旧 mtime 失败: %v", err)
	}
	if got := loadOwnedPIDsFile(path); len(got) != 0 {
		t.Fatalf("mtime 超 24h 应过滤为空集合: %v", got)
	}
}

// TestLoadOwnedPIDsFile_CorruptedJSONDegradesEmpty 损坏 JSON：降级空集合不致命
// （读失败/无文件同口径）；LoadJSON 产出的合法文件读回一致。
func TestLoadOwnedPIDsFile_CorruptedJSONDegradesEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, comOwnedPIDsFileName)
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("写损坏文件失败: %v", err)
	}
	if got := loadOwnedPIDsFile(path); len(got) != 0 {
		t.Fatalf("损坏 JSON 应降级空集合: %v", got)
	}
	if got := loadOwnedPIDsFile(filepath.Join(dir, "absent.json")); len(got) != 0 {
		t.Fatalf("无文件应返回空集合: %v", got)
	}
	if got := loadOwnedPIDsFile(""); len(got) != 0 {
		t.Fatalf("空路径（未启用持久化）应返回空集合: %v", got)
	}
}

// TestClassifyReusedInstance 残留判定三态：交集命中（自有残留，可回收）/
// 双非空无交集（真用户实例，禁杀兜底）/ 残留集空（无盘上证据）。
func TestClassifyReusedInstance(t *testing.T) {
	cases := []struct {
		name         string
		aliveAfter   map[int]bool
		staleOwned   map[int]bool
		wantResidual map[int]bool
	}{
		{"交集命中", map[int]bool{101: true, 4242: true}, map[int]bool{4242: true}, map[int]bool{4242: true}},
		{"双非空无交集（真用户实例）", map[int]bool{101: true}, map[int]bool{4242: true}, map[int]bool{}},
		{"残留集空（无盘上证据）", map[int]bool{101: true}, nil, map[int]bool{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyReusedInstance(tc.aliveAfter, tc.staleOwned)
			if len(got) != len(tc.wantResidual) {
				t.Fatalf("residual = %v, want %v", got, tc.wantResidual)
			}
			for pid := range tc.wantResidual {
				if !got[pid] {
					t.Fatalf("residual 缺 %d: %v", pid, got)
				}
			}
		})
	}
}

// ---- urgent 通道严格优先（PPT 预览转换插队，2026-10-22）----

// TestGoOleThumbExporter_UrgentJumpsQueuedNormalTask 优先级语义：普通任务 N0
// 执行中（body 阻塞 = 批量缩略图导图在途形态）+ 普通任务 N1 提交方挂起于
// taskCh（FIFO 存量：无缓冲通道 worker 忙时 submit 必然挂起，无第二接收方），
// urgent 提交后在 N0 放行后的首个消费决策点先于 N1 被消费。确定性构造：guReady
// 无缓冲会合——main 收到会合即知 urgent 发送方下一语句进入通道发送，先于
// close(hold) 的 worker 决策点到达挂起态（对齐 TestThumbQueue_JumpDuringBatch
// 的 runtime.Gosched 会合余量先例）；消费序经 order 通道记录断言（chan 交接
// 同步点，禁 sleep 等待）；收尾断言 worker 存活（urgent 消费后普通通道仍正常
// 消费——spec §6 存活性守护）。
func TestGoOleThumbExporter_UrgentJumpsQueuedNormalTask(t *testing.T) {
	e := newThumbExporter("").(*goOleThumbExporter)
	defer e.Shutdown()

	ctx := context.Background()
	order := make(chan string, 8)

	// 阶段 1：N0 占住 worker（body 阻塞在 hold 上，submit 返回 = 交接完成，
	// worker 正在执行 N0）
	hold := make(chan struct{})
	if err := e.submit(ctx, func(*comWorkerState) {
		order <- "normal#0"
		<-hold
	}); err != nil {
		t.Fatalf("提交 N0 失败: %v", err)
	}
	if got := <-order; got != "normal#0" {
		t.Fatalf("N0 应最先被消费, got %q", got)
	}

	// 阶段 2：N1 提交方挂起于 taskCh（worker 忙，交接只能等 N0 放行——FIFO 存量）
	normalDone := make(chan error, 1)
	go func() {
		normalDone <- e.submit(ctx, func(*comWorkerState) { order <- "normal#1" })
	}()

	// 阶段 3：urgent 提交——guReady 无缓冲会合保证发送方在放行前已到达发送点
	guReady := make(chan struct{})
	urgentDone := make(chan error, 1)
	go func() {
		guReady <- struct{}{}
		urgentDone <- e.submitUrgent(ctx, func(*comWorkerState) { order <- "urgent#0" })
	}()
	<-guReady
	runtime.Gosched() // 会合余量：给 urgent 发送方进入通道发送的调度机会

	// 阶段 4：放行 N0 → 决策点上 urgent 发送方已挂起 → 严格优先先于 N1 消费
	close(hold)
	select {
	case got := <-order:
		if got != "urgent#0" {
			t.Fatalf("urgent 应先于挂起的普通任务被消费, got %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("5s 内 urgent 任务未被消费（urgentCh 未纳入消费决策）")
	}
	select {
	case got := <-order:
		if got != "normal#1" {
			t.Fatalf("挂起的普通任务应在 urgent 之后被消费, got %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("5s 内挂起的普通任务未被消费（消费决策丢失提交方）")
	}
	// 提交侧返回序 = 消费序（单 worker 串行 + 无缓冲通道交接即消费）双检
	if err := <-urgentDone; err != nil {
		t.Fatalf("urgent 提交应成功: %v", err)
	}
	if err := <-normalDone; err != nil {
		t.Fatalf("普通提交应成功: %v", err)
	}

	// 阶段 5：worker 存活性守护——urgent 消费后普通通道仍正常消费
	if err := e.submit(ctx, func(*comWorkerState) { order <- "normal#2" }); err != nil {
		t.Fatalf("提交存活性任务失败: %v", err)
	}
	select {
	case got := <-order:
		if got != "normal#2" {
			t.Fatalf("存活性任务消费序不符, got %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("5s 内 worker 未继续消费普通任务（urgent 消费后主循环存活性破坏）")
	}
}
