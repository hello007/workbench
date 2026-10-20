package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
// ledger 复用服务构造的真实例（storeForThumbs 对未注册库返回错误，flush 安全丢弃）。
// Cleanup 走 WaitThumbsDone 全链收口：等 worker 退出（惰性启动的 run goroutine
// 不收口会污染 goleak 观察面）→ exporter.Shutdown → ledger.Shutdown（幂等，测试
// 主体已显式调用时重复执行安全）；不可用环境未启动 worker 时同样安全（close 未
// 消费的 quit 与空 WaitGroup 均为 no-op）。
func newThumbTestService(t *testing.T, fake *fakeThumbExporter) (*AssetLibraryService, *doneNotifySink) {
	t.Helper()
	s := NewAssetLibraryService(nil, filepath.Join(t.TempDir(), "assets"))
	sink := &doneNotifySink{inner: &fakeEventSink{}, doneCh: make(chan model.AssetThumbsResult, 16)}
	s.SetEventSink(sink)
	s.thumbQ = newThumbQueue(fake, s.eventSink, s.thumbLedger, nil)
	t.Cleanup(s.WaitThumbsDone)
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

// TestThumbQueue_JumpAfterBatchIdleParked 批次完成后 worker 停泊于第二个 select
// （阻塞等 batchCh/quit/jumpCh）期间的空闲态跳转仍可达——P0 回归守护：停泊
// select 未含 jumpCh 时空闲期跳转无人接收，JumpToSlide 撞入队超时报 BUSY
// （队列空闲却报忙的假态），且 stale 请求会被后续批量唤醒时误执行。确定性
// 构造：入队小批次（exportFn 直返）→ waitDone 等批次完成（worker 已回停泊
// select）→ 再 JumpToSlide 断言成功且恰执行一次（修复前此用例确定性失败）。
func TestThumbQueue_JumpAfterBatchIdleParked(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s, sink := newThumbTestService(t, fake)

	s.EnqueueAssetThumbs("lib1", thumbSpecs(1), t.TempDir())
	waitDone(t, sink) // 批次完成：worker 已回到停泊 select

	errCh := make(chan error, 1)
	go func() { errCh <- s.JumpToSlide(`C:\素材\a.pptx`, 3) }()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("空闲停泊期跳转应成功: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("5s 内跳转未被执行（停泊 select 未消费 jumpCh——P0 回归）")
	}
	if _, jumps, _, _ := fake.stats(); jumps != 1 {
		t.Fatalf("跳转应恰执行一次, got %d", jumps)
	}

	// worker 存活性守护（spec §6）：停泊期跳转后批次仍能消费（防 handleJump
	// 二次写阻塞等形态——跳转通道集扩张不得破坏主循环完整性）
	s.EnqueueAssetThumbs("lib1", thumbSpecs(1), t.TempDir())
	waitDone(t, sink)
	if exports, _, _, _ := fake.stats(); exports != 2 {
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

// ---------- 设置链路（PRD D2/D1/D4，PR3） ----------

// TestThumbTiersFromSettings 档位构造纯函数表驱动：默认值与包级 thumbTiers 严格
// 一致（320/180、1600/900——缓存子目录与历史产物兼容的基准），注入宽度生效且
// 高按 16:9 自算；thumbsSettings 的 normalize 使未注入 provider / 零值设置与
// 默认档位等价。
func TestThumbTiersFromSettings(t *testing.T) {
	cases := []struct {
		name      string
		cfg       model.AssetThumbsSettings
		wantSmall thumbTier
		wantHD    thumbTier
	}{
		{
			name:      "默认值与包级基准严格一致",
			cfg:       model.DefaultAssetThumbsSettings(),
			wantSmall: thumbTier{Key: "small", Width: 320, Height: 180},
			wantHD:    thumbTier{Key: "hd", Width: 1600, Height: 900},
		},
		{
			name:      "零值设置经 normalize 与默认档位等价",
			cfg:       model.AssetThumbsSettings{},
			wantSmall: thumbTier{Key: "small", Width: 320, Height: 180},
			wantHD:    thumbTier{Key: "hd", Width: 1600, Height: 900},
		},
		{
			name:      "注入宽度生效高按16比9自算",
			cfg:       model.AssetThumbsSettings{SmallWidth: 640, HDWidth: 1920},
			wantSmall: thumbTier{Key: "small", Width: 640, Height: 360},
			wantHD:    thumbTier{Key: "hd", Width: 1920, Height: 1080},
		},
		{
			name:      "clamp 边界收敛后构造（小图下限/高清上限）",
			cfg:       model.AssetThumbsSettings{SmallWidth: 1, HDWidth: 99999},
			wantSmall: thumbTier{Key: "small", Width: 120, Height: 67},
			wantHD:    thumbTier{Key: "hd", Width: 3840, Height: 2160},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			norm := model.NormalizeAssetThumbsSettings(&tc.cfg)
			got := thumbTiersFromSettings(norm)
			if len(got) != 2 {
				t.Fatalf("应恒为双档, got %d 档", len(got))
			}
			if got[0] != tc.wantSmall {
				t.Errorf("small 档不符: got %+v want %+v", got[0], tc.wantSmall)
			}
			if got[1] != tc.wantHD {
				t.Errorf("hd 档不符: got %+v want %+v", got[1], tc.wantHD)
			}
		})
	}
	// 默认一致性对照包级基准（回退路径不漂移的守护）
	got := thumbTiersFromSettings(model.DefaultAssetThumbsSettings())
	for i := range thumbTiers {
		if got[i] != thumbTiers[i] {
			t.Fatalf("默认档位应与包级 thumbTiers 逐字段一致: got %+v want %+v", got, thumbTiers)
		}
	}
}

// newThumbsSettingsTestService 设置链路用例公共构造：Available=true fake + 阻塞
// 型 exportFn（chan 交接防批次完成竞态）+ 可注册的宿主库。返回 svc/lib/store/
// thumbsDir 与放行闭包。
func newThumbsSettingsTestService(t *testing.T) (*AssetLibraryService, *fakeThumbExporter, model.AssetLibrary, *assetsStore, string, func()) {
	t.Helper()
	fake := &fakeThumbExporter{available: true}
	svc := newTestAssetService(t)
	svc.thumbQ = newThumbQueue(fake, svc.eventSink, svc.thumbLedger, nil)
	t.Cleanup(svc.thumbLedger.Shutdown)
	t.Cleanup(func() { svc.WaitThumbsDone() })

	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "设置库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	fileID := insertHostFile(t, store, "甲.pptx")
	if _, err := store.db.Exec(
		"INSERT INTO slides(file_id, slide_num) VALUES(?, 1)", fileID,
	); err != nil {
		t.Fatalf("插幻灯片行: %v", err)
	}

	releaseCh := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
	fake.exportFn = func(e *fakeThumbExporter, _ context.Context, _ thumbFileSpec, _ string, _ []thumbTier, _ func() *thumbJumpRequest) (int, error) {
		<-releaseCh
		return 1, nil
	}
	return svc, fake, lib, store, svc.assetThumbsDir(lib.ID), release
}

// registeredThumbBatch 快照指定库当前注册批次（q.mu 内读取，断言入队面用）。
func registeredThumbBatch(s *AssetLibraryService, libraryID string) *thumbBatchTask {
	s.thumbQ.mu.Lock()
	defer s.thumbQ.mu.Unlock()
	return s.thumbQ.batches[libraryID]
}

// TestThumbsBacklog_AutoDisabledSkipsEnqueue D1 自动生成开关：Auto=false 扫描
// 完成不入队（无导出无事件）；Auto=true 对照入队且 worker 消费（chan 交接
// 确定性等待）。
func TestThumbsBacklog_AutoDisabledSkipsEnqueue(t *testing.T) {
	svc, fake, lib, store, _, release := newThumbsSettingsTestService(t)

	svc.SetThumbsSettingsProvider(func() model.AssetThumbsSettings {
		return model.AssetThumbsSettings{Auto: false} // normalize 后其余四项走默认
	})
	svc.enqueueThumbsBacklog(store, lib)
	if exports, _, _, _ := fake.stats(); exports != 0 {
		t.Fatalf("Auto=false 不应入队导出, got %d 次", exports)
	}
	if registeredThumbBatch(svc, lib.ID) != nil {
		t.Fatal("Auto=false 不应注册批次")
	}

	// Auto=true 对照：入队且 worker 已消费首文件（确定性同步）
	svc.SetThumbsSettingsProvider(func() model.AssetThumbsSettings {
		return model.AssetThumbsSettings{Auto: true}
	})
	svc.enqueueThumbsBacklog(store, lib)
	b := registeredThumbBatch(svc, lib.ID)
	if b == nil {
		t.Fatal("Auto=true 应注册批次")
	}
	if len(b.Files) != 1 || b.Files[0].FileID == 0 {
		t.Fatalf("批次应含 ok 文件甲.pptx: %+v", b.Files)
	}
	release() // 放行 worker 收口（cleanup WaitThumbsDone 防永挂）
}

// TestApplyThumbsSettingsChange_WidthClearsTierAndReenqueues 分辨率变更清档重入队：
// prev/cur 宽度变 → 双档目录删、记账行删、dirty 同清（flush 不复活）、重入队
// 批次含全部 ok 文件（一次一批覆盖双档）。
func TestApplyThumbsSettingsChange_WidthClearsTierAndReenqueues(t *testing.T) {
	svc, _, lib, store, thumbsDir, release := newThumbsSettingsTestService(t)

	fileID := fileIDByPath(t, store, "甲.pptx")
	writeThumbPNG(t, thumbsDir, "small", fileID, 1, "old-small")
	writeThumbPNG(t, thumbsDir, "hd", fileID, 1, "old-hd")
	insertThumbLedgerRow(t, store, fileID, "small", 1, 100, 1)
	insertThumbLedgerRow(t, store, fileID, "hd", 1, 200, 1)
	// dirty 注入：清档须连 dirty 一起清（防旧分辨率记账经 flush 复活）
	svc.thumbLedger.record(lib.ID, fileID, "small", 1, 100)

	prev := model.DefaultAssetThumbsSettings()
	cur := model.DefaultAssetThumbsSettings()
	cur.SmallWidth = 640
	cur.HDWidth = 1920

	svc.applyThumbsSettingsChangeSync(prev, cur)

	for _, tier := range []string{"small", "hd"} {
		if _, err := os.Stat(filepath.Join(thumbsDir, tier)); !os.IsNotExist(err) {
			t.Errorf("%s 档目录应删除, stat err=%v", tier, err)
		}
	}
	if n := countThumbCacheRows(t, store); n != 0 {
		t.Fatalf("清档后记账行应删空, got %d", n)
	}
	svc.thumbLedger.flushLibrary(lib.ID)
	if n := countThumbCacheRows(t, store); n != 0 {
		t.Fatalf("dirty 应随清档清除（flush 不复活）, got %d", n)
	}

	b := registeredThumbBatch(svc, lib.ID)
	if b == nil {
		t.Fatal("清档后应重入队补缺批次")
	}
	if len(b.Files) != 1 {
		t.Fatalf("重入队批次应含全部 ok 文件, got %+v", b.Files)
	}
	if b.Files[0].FileID != fileID || b.Files[0].Path != "甲.pptx" {
		t.Fatalf("重入队批次文件不符: %+v", b.Files[0])
	}
	if wantAbs := filepath.Join(lib.RootPath, "甲.pptx"); b.Files[0].AbsPath != wantAbs {
		t.Errorf("AbsPath 应为 %s, got %s", wantAbs, b.Files[0].AbsPath)
	}
	release() // 放行 worker 收口批次
}

// TestApplyThumbsSettingsChange_AutoDisabledSkipsReenqueue Auto=false 时分辨率
// 变更仍清档（残留旧分辨率产物只会误导）但不重入队（用户关闭了自动生成）。
func TestApplyThumbsSettingsChange_AutoDisabledSkipsReenqueue(t *testing.T) {
	svc, _, lib, store, thumbsDir, release := newThumbsSettingsTestService(t)
	defer release()

	fileID := fileIDByPath(t, store, "甲.pptx")
	writeThumbPNG(t, thumbsDir, "small", fileID, 1, "old-small")
	insertThumbLedgerRow(t, store, fileID, "small", 1, 100, 1)

	prev := model.DefaultAssetThumbsSettings()
	cur := model.DefaultAssetThumbsSettings()
	cur.SmallWidth = 640
	cur.Auto = false

	svc.applyThumbsSettingsChangeSync(prev, cur)

	if _, err := os.Stat(filepath.Join(thumbsDir, "small")); !os.IsNotExist(err) {
		t.Errorf("清档不受 Auto 开关影响（旧分辨率产物应删）, stat err=%v", err)
	}
	if n := countThumbCacheRows(t, store); n != 0 {
		t.Fatalf("清档后记账行应删空, got %d", n)
	}
	if registeredThumbBatch(svc, lib.ID) != nil {
		t.Fatal("Auto=false 不应重入队补缺批次")
	}
}

// TestApplyThumbsSettingsChange_QuotaChangeTriggersEviction 上限变更触发一次
// 惰性淘汰（ledger 配额注入小值——真实设置下限 64MB，字节级测试语料须经注入点）。
func TestApplyThumbsSettingsChange_QuotaChangeTriggersEviction(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "改额库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	thumbsDir := svc.assetThumbsDir(lib.ID)

	// 替换服务 ledger：quotaFn 注入小配额（service 配额注入链路的测试面）
	l := newThumbCacheLedger(
		svc.storeForThumbs,
		func(string) string { return thumbsDir },
		func(string) int64 { return 400 },
	)
	svc.thumbLedger = l
	t.Cleanup(l.Shutdown)

	for i := 1; i <= 5; i++ {
		id := insertHostFile(t, store, fmt.Sprintf("素材%d.pptx", i))
		insertThumbLedgerRow(t, store, id, "small", 1, 100, int64(i))
		writeThumbPNG(t, thumbsDir, "small", id, 1, "x")
	}

	prev := model.DefaultAssetThumbsSettings()
	cur := model.DefaultAssetThumbsSettings()
	cur.SmallQuotaMB = 100 // 仅上限变（宽度不变）→ requota 路径

	svc.applyThumbsSettingsChangeSync(prev, cur)

	if n := countThumbCacheRows(t, store); n != 3 {
		t.Fatalf("上限变更应触发淘汰至水位（余 3 行 300B ≤ 360B）, got %d", n)
	}
}

// TestApplyThumbsSettingsChange_NoChangeNoop prev/cur 无差异时不触发任何清档与
// 淘汰（守卫误伤面：每次保存设置都重清库是不可接受行为）。
func TestApplyThumbsSettingsChange_NoChangeNoop(t *testing.T) {
	svc, _, lib, store, thumbsDir, release := newThumbsSettingsTestService(t)
	defer release()

	fileID := fileIDByPath(t, store, "甲.pptx")
	writeThumbPNG(t, thumbsDir, "small", fileID, 1, "keep")
	insertThumbLedgerRow(t, store, fileID, "small", 1, 100, 1)

	svc.applyThumbsSettingsChangeSync(model.DefaultAssetThumbsSettings(), model.DefaultAssetThumbsSettings())

	if n := countThumbCacheRows(t, store); n != 1 {
		t.Fatalf("无差异不应清记账, got %d 行", n)
	}
	if _, err := os.Stat(thumbPNGPath(thumbsDir, "small", fileID, 1)); err != nil {
		t.Errorf("无差异不应删 PNG: %v", err)
	}
	if registeredThumbBatch(svc, lib.ID) != nil {
		t.Fatal("无差异不应重入队")
	}
}

// TestRetryAssetThumb 单文件重试（D4）四分支：正常路径注册单文件批次
// （FileID/Path/AbsPath 正确，AbsPath = root+rel）、无 COM 返回
// E_ASSETS_THUMBS_UNAVAILABLE、库不存在透传 E_ASSETS_LIB_NOT_FOUND、
// 源文件缺失/参数非法报错。
func TestRetryAssetThumb(t *testing.T) {
	svc, _, lib, _, _, release := newThumbsSettingsTestService(t)

	dir := lib.RootPath
	if err := os.WriteFile(filepath.Join(dir, "甲.pptx"), []byte("fake pptx"), 0644); err != nil {
		t.Fatalf("写源文件: %v", err)
	}

	// 正常路径：单文件批次注册（worker 阻塞在 releaseCh，注册态稳定可断言）
	if err := svc.RetryAssetThumb(lib.ID, 42, "甲.pptx"); err != nil {
		t.Fatalf("重试应成功入队: %v", err)
	}
	b := registeredThumbBatch(svc, lib.ID)
	if b == nil {
		t.Fatal("应注册单文件批次")
	}
	if len(b.Files) != 1 {
		t.Fatalf("应为单文件批次, got %d 文件", len(b.Files))
	}
	f := b.Files[0]
	if f.FileID != 42 || f.Path != "甲.pptx" {
		t.Fatalf("FileID/Path 不符: %+v", f)
	}
	if wantAbs := filepath.Join(lib.RootPath, "甲.pptx"); f.AbsPath != wantAbs {
		t.Fatalf("AbsPath 应为 %s, got %s", wantAbs, f.AbsPath)
	}
	if wantDir := svc.assetThumbsDir(lib.ID); b.ThumbsDir != wantDir {
		t.Fatalf("ThumbsDir 应为 %s, got %s", wantDir, b.ThumbsDir)
	}
	release() // 放行 worker 收口批次

	// 库不存在：透传 E_ASSETS_LIB_NOT_FOUND（可用服务上判，隔离库缺失分支）
	if err := svc.RetryAssetThumb("不存在的库", 1, "甲.pptx"); appErrCode(t, err) != model.ErrCodeAssetsLibNotFound {
		t.Fatalf("库不存在应透传 E_ASSETS_LIB_NOT_FOUND, got %v", err)
	}

	// 无 COM：返回 E_ASSETS_THUMBS_UNAVAILABLE
	offSvc := newTestAssetService(t) // 默认 unavailable fake exporter
	offLib := mustCreateLibrary(t, offSvc, "无COM库", t.TempDir())
	if err := offSvc.RetryAssetThumb(offLib.ID, 1, "x.pptx"); appErrCode(t, err) != model.ErrCodeAssetsThumbsUnavailable {
		t.Fatalf("无 COM 应返回 E_ASSETS_THUMBS_UNAVAILABLE, got %v", err)
	}

	// 源文件缺失：报「源文件不存在或无法访问」包装错
	if err := svc.RetryAssetThumb(lib.ID, 1, "缺失.pptx"); err == nil ||
		!strings.Contains(err.Error(), "源文件不存在或无法访问") {
		t.Fatalf("源文件缺失应报可读错误, got %v", err)
	}

	// 参数非法：fileID<1 / relPath 空
	if err := svc.RetryAssetThumb(lib.ID, 0, "甲.pptx"); err == nil {
		t.Fatal("fileID<1 应报错")
	}
	if err := svc.RetryAssetThumb(lib.ID, 1, ""); err == nil {
		t.Fatal("relPath 空应报错")
	}

	// 路径越界遏制：`..` 逃逸库根与绝对路径均拒绝（防 PowerPoint 渲染库外文件
	// 经 GetAssetThumb 回读内容）；库根内子目录相对路径正常放行
	if err := svc.RetryAssetThumb(lib.ID, 1, "../外部.pptx"); err == nil ||
		!strings.Contains(err.Error(), "越界") {
		t.Fatalf("`..` 逃逸应拒绝, got %v", err)
	}
	if err := svc.RetryAssetThumb(lib.ID, 1, "a/../../外部.pptx"); err == nil ||
		!strings.Contains(err.Error(), "越界") {
		t.Fatalf("嵌套 `..` 逃逸应拒绝, got %v", err)
	}
	if err := svc.RetryAssetThumb(lib.ID, 1, `C:\外部.pptx`); err == nil {
		t.Fatal("绝对路径应拒绝")
	}
}
