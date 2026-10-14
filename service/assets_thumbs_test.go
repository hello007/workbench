package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"workbench/model"
)

// ---- fake exporter（CI 零 PowerPoint 依赖：队列调度/失败隔离/取消/事件全在 fake 上验证）----

// fakeThumbExporter ThumbExporter 假实现：行为经字段注入，调用被记录供断言。
type fakeThumbExporter struct {
	mu        sync.Mutex
	available bool
	exportFn  func(e *fakeThumbExporter, ctx context.Context, spec thumbFileSpec, thumbsDir string, tiers []thumbTier, preempt func() *thumbJumpRequest) (int, error)
	jumpFn    func(e *fakeThumbExporter, jr *thumbJumpRequest) error

	exportCalls []thumbFileSpec
	jumpCalls   []*thumbJumpRequest
	discards    int
	shutdowns   int
}

func (f *fakeThumbExporter) Available() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available
}

func (f *fakeThumbExporter) ExportPresentation(ctx context.Context, spec thumbFileSpec, thumbsDir string, tiers []thumbTier, preempt func() *thumbJumpRequest) (int, error) {
	f.mu.Lock()
	f.exportCalls = append(f.exportCalls, spec)
	fn := f.exportFn
	f.mu.Unlock()
	if fn == nil {
		return 3, nil
	}
	return fn(f, ctx, spec, thumbsDir, tiers, preempt)
}

func (f *fakeThumbExporter) JumpOnly(jr *thumbJumpRequest) error {
	f.mu.Lock()
	f.jumpCalls = append(f.jumpCalls, jr)
	fn := f.jumpFn
	f.mu.Unlock()
	if fn == nil {
		jr.Result <- nil
		return nil
	}
	return fn(f, jr)
}

func (f *fakeThumbExporter) Discard() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discards++
}

func (f *fakeThumbExporter) Shutdown() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdowns++
}

func (f *fakeThumbExporter) stats() (exports int, jumps int, discards int, shutdowns int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.exportCalls), len(f.jumpCalls), f.discards, f.shutdowns
}

// doneNotifySink 包装 fakeEventSink：assets-thumbs:done 事件到达时向 chan 通知，
// 测试以 channel 同步替代 sleep 轮询（test-stability 契约禁 sleep 等待）。
type doneNotifySink struct {
	inner  *fakeEventSink
	doneCh chan model.AssetThumbsResult
}

func (s *doneNotifySink) Emit(name string, data ...any) {
	s.inner.Emit(name, data...)
	if name == "assets-thumbs:done" {
		if r, ok := data[0].(model.AssetThumbsResult); ok {
			s.doneCh <- r
		}
	}
}

// newThumbTestService 构造注入 fake exporter 的 AssetLibraryService + done 通知 sink。
func newThumbTestService(t *testing.T, fake *fakeThumbExporter) (*AssetLibraryService, *doneNotifySink) {
	t.Helper()
	s := NewAssetLibraryService(nil, filepath.Join(t.TempDir(), "assets"))
	sink := &doneNotifySink{inner: &fakeEventSink{}, doneCh: make(chan model.AssetThumbsResult, 16)}
	s.SetEventSink(sink)
	s.thumbQ = newThumbQueue(fake, s.eventSink)
	return s, sink
}

// waitDone 等指定库的 done 事件（5s 超时守护——防永挂）。
func waitDone(t *testing.T, sink *doneNotifySink) model.AssetThumbsResult {
	t.Helper()
	select {
	case r := <-sink.doneCh:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("5s 内未收到 assets-thumbs:done 事件")
		return model.AssetThumbsResult{}
	}
}

func thumbSpecs(n int) []thumbFileSpec {
	specs := make([]thumbFileSpec, 0, n)
	for i := 1; i <= n; i++ {
		specs = append(specs, thumbFileSpec{
			FileID:  int64(i),
			Path:    fmt.Sprintf("file%d.pptx", i),
			AbsPath: fmt.Sprintf(`C:\素材\file%d.pptx`, i),
		})
	}
	return specs
}

// TestThumbQueue_BatchSerialAndEvents 批次逐文件串行执行 + progress/done 事件序列
// （progress 计数递增、done 汇总 Generated）。
func TestThumbQueue_BatchSerialAndEvents(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s, sink := newThumbTestService(t, fake)

	s.EnqueueAssetThumbs("lib1", thumbSpecs(3), t.TempDir())
	result := waitDone(t, sink)

	if result.Total != 3 || result.Generated != 3 || result.Failed != 0 || result.Canceled {
		t.Fatalf("done 载荷不符: %+v", result)
	}
	exports, _, _, shutdowns := fake.stats()
	if exports != 3 {
		t.Fatalf("应串行导出 3 个文件, got %d", exports)
	}
	if shutdowns != 0 {
		t.Fatalf("批次完成不应触发 Shutdown, got %d", shutdowns)
	}

	// progress 事件序列：done 1→3 递增，载荷带 libraryId
	sink.inner.mu.Lock()
	defer sink.inner.mu.Unlock()
	progressDone := []int{}
	for _, c := range sink.inner.calls {
		if c.name != "assets-thumbs:progress" {
			continue
		}
		p, ok := c.data[0].(model.AssetThumbsProgress)
		if !ok {
			t.Fatalf("progress 载荷类型不符: %T", c.data[0])
		}
		if p.LibraryID != "lib1" {
			t.Errorf("progress 载荷应带 libraryId=lib1, got %q", p.LibraryID)
		}
		progressDone = append(progressDone, p.Done)
	}
	if fmt.Sprint(progressDone) != "[1 2 3]" {
		t.Fatalf("progress 序列应为 1→3 递增, got %v", progressDone)
	}
}

// TestThumbQueue_FailIsolation 单文件失败不中断批次：失败文件 Discard（会话重建）
// + 计入 Failed，后续文件继续（SRS「重启实例继续后续任务」）。
func TestThumbQueue_FailIsolation(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s, sink := newThumbTestService(t, fake)
	fake.exportFn = func(e *fakeThumbExporter, _ context.Context, spec thumbFileSpec, _ string, _ []thumbTier, _ func() *thumbJumpRequest) (int, error) {
		if spec.FileID == 2 { // 第 2 个文件失败（损坏/占用模拟）
			return 0, errors.New("COM 打开失败")
		}
		return 3, nil
	}

	s.EnqueueAssetThumbs("lib1", thumbSpecs(3), t.TempDir())
	result := waitDone(t, sink)

	if result.Generated != 2 || result.Failed != 1 {
		t.Fatalf("失败隔离汇总不符: %+v", result)
	}
	exports, _, discards, _ := fake.stats()
	if exports != 3 {
		t.Fatalf("失败后应继续第 3 个文件, got %d 次导出", exports)
	}
	if discards != 1 {
		t.Fatalf("失败文件应触发一次 Discard, got %d", discards)
	}
}

// TestThumbQueue_JumpDuringBatch 批量在途时跳转不丢失且结果正确回传。
// 执行路径二选一（皆合规）：页边界 preempt 摘走（D3 快路径）或批次结束后
// worker 回主循环消费（导出快于提交调度时的兜底路径）——jumpCh 无缓冲保证
// 提交必被消费，测试断言用户可感知语义：JumpToSlide 返回 nil、跳转恰执行一次。
func TestThumbQueue_JumpDuringBatch(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s, sink := newThumbTestService(t, fake)

	// 批次在途：exportFn 逐页等待放行（chan 交接 = 确定性同步），页边界调 preempt
	release := make(chan struct{})
	fake.exportFn = func(e *fakeThumbExporter, _ context.Context, spec thumbFileSpec, _ string, _ []thumbTier, preempt func() *thumbJumpRequest) (int, error) {
		for page := 1; page <= 10; page++ {
			<-release
			runtime.Gosched() // 给 JumpToSlide goroutine 提交机会（页边界会合点）
			if jr := preempt(); jr != nil {
				e.mu.Lock() // preempt 路径不经 JumpOnly，手动记账保 stats 口径一致
				e.jumpCalls = append(e.jumpCalls, jr)
				e.mu.Unlock()
				jr.Result <- nil
				return 10, nil
			}
		}
		return 10, nil // 页全部放行完（跳转走批次后主循环兜底路径）
	}

	s.EnqueueAssetThumbs("lib1", thumbSpecs(1), t.TempDir())
	release <- struct{}{} // 放行第 1 页（阻塞至 exportFn 收取 = 批次已在途）

	errCh := make(chan error, 1)
	go func() { errCh <- s.JumpToSlide(`C:\素材\other.pptx`, 5) }()

	// 放行剩余页直到跳转结果返回（preempt 提前 return 后 release 无人收，走 errCh 分支）
	for i := 0; i < 9; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("批量在途跳转应成功: %v", err)
			}
			waitDone(t, sink)
			if _, jumps, _, _ := fake.stats(); jumps != 1 {
				t.Fatalf("跳转应恰执行一次, got %d", jumps)
			}
			return
		default:
		}
		select {
		case release <- struct{}{}:
		case err := <-errCh:
			if err != nil {
				t.Fatalf("批量在途跳转应成功: %v", err)
			}
			waitDone(t, sink)
			if _, jumps, _, _ := fake.stats(); jumps != 1 {
				t.Fatalf("跳转应恰执行一次, got %d", jumps)
			}
			return
		}
	}

	// 页全放完仍未返回：跳转走兜底路径（批次结束后主循环消费），等结果 + done
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("批量在途跳转应成功（兜底路径）: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("5s 内跳转未被执行（jumpCh 消费链路失效）")
	}
	waitDone(t, sink)
	if _, jumps, _, _ := fake.stats(); jumps != 1 {
		t.Fatalf("跳转应恰执行一次, got %d", jumps)
	}
}

// TestThumbQueue_JumpDirectWhenIdle 无批量在途时跳转直执行（worker 主循环优先摘取）；
// 跳转后追加批次仍能执行——守护 handleJump 不二次写 jr.Result（Result 缓冲 1
// 单读者，双写会永久阻塞 worker，后续批次永不执行，done 事件超时暴露）。
func TestThumbQueue_JumpDirectWhenIdle(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s, sink := newThumbTestService(t, fake)

	err := s.JumpToSlide(`C:\素材\a.pptx`, 2)
	if err != nil {
		t.Fatalf("空闲跳转应成功: %v", err)
	}
	if _, jumps, _, _ := fake.stats(); jumps != 1 {
		t.Fatalf("应执行 1 次跳转, got %d", jumps)
	}

	// 跳转后 worker 必须回到主循环继续消费批次（回归守护：handleJump 双写阻塞）
	s.EnqueueAssetThumbs("lib1", thumbSpecs(1), t.TempDir())
	waitDone(t, sink)
	if exports, _, _, _ := fake.stats(); exports != 1 {
		t.Fatalf("跳转后批次应正常执行, got %d 次导出", exports)
	}
}

// TestThumbQueue_JumpErrorIsolated 跳转失败不拖垮队列：错误回传调用方
// （jr.Result 由 JumpOnly 契约写回恰一次），worker 存活并继续消费后续批次。
func TestThumbQueue_JumpErrorIsolated(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s, sink := newThumbTestService(t, fake)

	fake.jumpFn = func(e *fakeThumbExporter, jr *thumbJumpRequest) error {
		err := errors.New("COM 打开失败")
		jr.Result <- err // 契约：成功/失败均恰一次
		return err
	}
	if err := s.JumpToSlide(`C:\素材\a.pptx`, 2); err == nil {
		t.Fatal("跳转失败应回传错误")
	}
	s.EnqueueAssetThumbs("lib1", thumbSpecs(1), t.TempDir())
	waitDone(t, sink)
	if exports, _, _, _ := fake.stats(); exports != 1 {
		t.Fatalf("跳转失败后批次应正常执行, got %d 次导出", exports)
	}
}

// TestThumbQueue_CancelBatch 取消在途批次：中断剩余文件 + done.Canceled=true。
func TestThumbQueue_CancelBatch(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s, sink := newThumbTestService(t, fake)

	fileStarted := make(chan string, 8)
	fake.exportFn = func(e *fakeThumbExporter, ctx context.Context, spec thumbFileSpec, _ string, _ []thumbTier, _ func() *thumbJumpRequest) (int, error) {
		fileStarted <- spec.Path
		<-ctx.Done() // 模拟长导出，直到批次取消传播
		return 0, ctx.Err()
	}

	s.EnqueueAssetThumbs("lib1", thumbSpecs(5), t.TempDir())
	<-fileStarted // 第 1 个文件已开始
	if !s.CancelAssetThumbs("lib1") {
		t.Fatal("取消应命中在途批次")
	}
	result := waitDone(t, sink)
	if !result.Canceled {
		t.Fatalf("done 载荷应标记 Canceled: %+v", result)
	}
	exports, _, _, _ := fake.stats()
	if exports != 1 {
		t.Fatalf("取消后不应继续后续文件, got %d 次导出", exports)
	}
	// 重复取消幂等返回 false
	if s.CancelAssetThumbs("lib1") {
		t.Fatal("批次已清理，重复取消应返回 false")
	}
}

// TestThumbQueue_UnavailableSilent 无 PowerPoint 环境整体降级：入队静默跳过
// （无事件）、跳转返回 errThumbsUnavailable（D1/D4）。
func TestThumbQueue_UnavailableSilent(t *testing.T) {
	fake := &fakeThumbExporter{available: false}
	s, sink := newThumbTestService(t, fake)

	s.EnqueueAssetThumbs("lib1", thumbSpecs(3), t.TempDir())
	err := s.JumpToSlide(`C:\素材\a.pptx`, 1)
	if got := appErrCode(t, err); got != model.ErrCodeAssetsThumbsUnavailable {
		t.Fatalf("跳转应返回 E_ASSETS_THUMBS_UNAVAILABLE, got %v", err)
	}
	sink.inner.mu.Lock()
	n := len(sink.inner.calls)
	sink.inner.mu.Unlock()
	if n != 0 {
		t.Fatalf("不可用环境不应发出任何事件, got %d", n)
	}
	if exports, _, _, _ := fake.stats(); exports != 0 {
		t.Fatalf("不可用环境不应触发导出, got %d", exports)
	}
}

// TestThumbQueue_OverwriteBatch 同库重复入队：旧批次作废（Cancel），仅新批次执行。
func TestThumbQueue_OverwriteBatch(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s, sink := newThumbTestService(t, fake)

	block := make(chan struct{})
	oldBatchStarted := make(chan struct{})
	fake.exportFn = func(e *fakeThumbExporter, ctx context.Context, spec thumbFileSpec, _ string, _ []thumbTier, _ func() *thumbJumpRequest) (int, error) {
		if spec.FileID == 1 { // 旧批次首文件：挂起直到被新批次覆盖取消
			close(oldBatchStarted)
			<-ctx.Done()
			return 0, ctx.Err()
		}
		<-block
		return 3, nil
	}

	s.EnqueueAssetThumbs("lib1", thumbSpecs(1), t.TempDir())
	<-oldBatchStarted // 旧批次已进入首文件（chan 交接替代 sleep 等待）
	// 新批次 FileID 刻意错开（10/11）：若复用 1/2，新批次首文件会命中同一挂起分支
	s.EnqueueAssetThumbs("lib1", []thumbFileSpec{
		{FileID: 10, Path: "new1.pptx", AbsPath: `C:\素材\new1.pptx`},
		{FileID: 11, Path: "new2.pptx", AbsPath: `C:\素材\new2.pptx`},
	}, t.TempDir())

	result := waitDone(t, sink)
	if !result.Canceled {
		t.Fatalf("被覆盖的旧批次应以 Canceled 终态收口: %+v", result)
	}
	close(block)
	waitDone(t, sink)                                   // 新批次完成
	if exports, _, _, _ := fake.stats(); exports != 3 { // 旧 1 + 新 2
		t.Fatalf("导出次数不符（旧 1 失败 + 新 2 成功）, got %d", exports)
	}
}

// TestThumbQueue_WaitThumbsDone 关停收口：取消注册批次（在途/待消费均覆盖，
// 不依赖批次是否已被 worker 取走——断言在全部交错下成立，禁 sleep 等竞态）→
// worker 退出 → exporter Shutdown 恰一次；closed 后拒绝新任务（不 panic、不执行）。
func TestThumbQueue_WaitThumbsDone(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s, _ := newThumbTestService(t, fake)

	fake.exportFn = func(e *fakeThumbExporter, ctx context.Context, spec thumbFileSpec, _ string, _ []thumbTier, _ func() *thumbJumpRequest) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	s.EnqueueAssetThumbs("lib1", thumbSpecs(2), t.TempDir())

	done := make(chan struct{})
	go func() { s.WaitThumbsDone(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitThumbsDone 5s 未收口（worker 退出链路阻塞）")
	}
	if _, _, _, shutdowns := fake.stats(); shutdowns != 1 {
		t.Fatalf("Shutdown 应恰一次, got %d", shutdowns)
	}
	// closed 后入队/跳转静默拒绝
	s.EnqueueAssetThumbs("lib1", thumbSpecs(1), t.TempDir())
	if err := s.JumpToSlide(`C:\素材\a.pptx`, 1); appErrCode(t, err) != model.ErrCodeAssetsThumbsUnavailable {
		t.Fatalf("关停后跳转应拒绝, got %v", err)
	}
}

// TestThumbPNGPath 缓存命名契约单点：<thumbsDir>/<tier>/<fileID>-<3位页>.png
// （slides.id 禁作缓存键——files.id 稳定，research/lru-thumbnail-cache.md）。
func TestThumbPNGPath(t *testing.T) {
	got := thumbPNGPath(filepath.Join("data", "assets", "lib1", "thumbs"), "small", 42, 7)
	want := filepath.Join("data", "assets", "lib1", "thumbs", "small", "42-007.png")
	if got != want {
		t.Fatalf("缓存命名契约不符:\n got %s\nwant %s", got, want)
	}
}

// TestAssetLibraryService_ThumbQueueDefaultConstruction 生产构造默认装配队列
// （平台分叉 exporter；此处仅验证字段非 nil 与不 panic——COM 行为归集成验证）。
func TestAssetLibraryService_ThumbQueueDefaultConstruction(t *testing.T) {
	s := NewAssetLibraryService(nil, t.TempDir())
	if s.thumbQ == nil {
		t.Fatal("构造应初始化缩略图队列")
	}
	if s.thumbQ.exporter == nil {
		t.Fatal("队列应注入平台 exporter")
	}
}
