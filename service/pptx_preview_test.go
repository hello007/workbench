package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"workbench/model"
)

// fakePptxConverter PptxPreviewService 单测注入的假转换器（CI 零 PowerPoint
// 依赖）。convertFn 可注入逐次调用的行为（写产物 / 返回错误 / 阻塞模拟在途）。
type fakePptxConverter struct {
	mu        sync.Mutex
	available bool
	calls     int
	paths     [][2]string // 每次调用的 [src, dst]
	convertFn func(n int, src, dst string) error
}

func (f *fakePptxConverter) Available() bool { return f.available }

func (f *fakePptxConverter) ConvertToPdf(_ context.Context, src, dst string) error {
	f.mu.Lock()
	f.calls++
	n := f.calls
	fn := f.convertFn
	f.paths = append(f.paths, [2]string{src, dst})
	f.mu.Unlock()
	if fn != nil {
		return fn(n, src, dst)
	}
	return nil
}

func (f *fakePptxConverter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// writeFakePdf fake 转换产物（非空文件，满足编排层「产物存在且非空」校验）。
// 返回错误而非 t.Fatalf：convertFn 在测试派生的 goroutine 内执行，Fatalf 会
// Goexit 该 goroutine 使在途防抖等待永挂（goleak 抓出的形态）。
func writeFakePdf(_ *testing.T, dst string) error {
	return os.WriteFile(dst, []byte("%PDF-1.4 fake"), 0o644)
}

// newTestPptxPreviewService 构造测试服务（cacheDir = t.TempDir() 子目录）。
func newTestPptxPreviewService(t *testing.T, conv PptxPdfConverter) *PptxPreviewService {
	t.Helper()
	return NewPptxPreviewService(filepath.Join(t.TempDir(), "pptx_preview_pdf"), conv)
}

// mustWritePptx 源文件 fixture + os.Chtimes 显式注入 mtime（test-stability 契约：
// 禁依赖真实 FS mtime 时序，mtime 失效判定路径用确定性注入值驱动）。
func mustWritePptx(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte("fake pptx bytes"), 0o644); err != nil {
		t.Fatalf("写源文件失败: %v", err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("注入 mtime 失败: %v", err)
	}
}

// TestConvertForPreview_CacheHit 二次调用同文件命中缓存，无第二次 COM 调用。
func TestConvertForPreview_CacheHit(t *testing.T) {
	conv := &fakePptxConverter{available: true, convertFn: func(_ int, _, dst string) error {
		return writeFakePdf(t, dst)
	}}
	svc := newTestPptxPreviewService(t, conv)
	src := filepath.Join(t.TempDir(), "a.pptx")
	mustWritePptx(t, src, time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local))

	path1, err := svc.ConvertForPreview(context.Background(), src)
	if err != nil {
		t.Fatalf("首次转换: %v", err)
	}
	path2, err := svc.ConvertForPreview(context.Background(), src)
	if err != nil {
		t.Fatalf("二次调用: %v", err)
	}
	if path1 != path2 {
		t.Errorf("缓存命中应返回同一路径: %s != %s", path1, path2)
	}
	if got := conv.callCount(); got != 1 {
		t.Errorf("缓存命中不应再调转换器: calls=%d, want 1", got)
	}
	if filepath.Ext(path1) != ".pdf" {
		t.Errorf("产物应为 .pdf: %s", path1)
	}
}

// TestConvertForPreview_SourceChanged 源文件 mtime+size 变更后重新转换（新版内容）。
// mtime 注入用 os.Chtimes 显式跨值（禁 sleep 等待 FS tick，test-stability 契约）。
func TestConvertForPreview_SourceChanged(t *testing.T) {
	conv := &fakePptxConverter{available: true, convertFn: func(_ int, _, dst string) error {
		return writeFakePdf(t, dst)
	}}
	svc := newTestPptxPreviewService(t, conv)
	src := filepath.Join(t.TempDir(), "b.pptx")
	base := time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local)
	mustWritePptx(t, src, base)

	path1, err := svc.ConvertForPreview(context.Background(), src)
	if err != nil {
		t.Fatalf("首次转换: %v", err)
	}
	// 显式注入新 mtime + 新 size（不依赖写入触发 FS mtime 变化）
	mustWritePptx(t, src, base.Add(2*time.Hour))
	if err := os.WriteFile(src, []byte("fake pptx bytes v2 bigger"), 0o644); err != nil {
		t.Fatalf("改写源文件失败: %v", err)
	}
	if err := os.Chtimes(src, base.Add(2*time.Hour), base.Add(2*time.Hour)); err != nil {
		t.Fatalf("注入新 mtime 失败: %v", err)
	}

	path2, err := svc.ConvertForPreview(context.Background(), src)
	if err != nil {
		t.Fatalf("源变更后转换: %v", err)
	}
	if path1 == path2 {
		t.Errorf("源变更应产生新缓存文件")
	}
	if got := conv.callCount(); got != 2 {
		t.Errorf("源变更应重新转换: calls=%d, want 2", got)
	}
}

// TestConvertForPreview_InFlightDebounce 同文件并发请求共享同一次转换
// （第二个调用等待第一个完成），且服务在共享等待后仍可用（防抖注册项清理
// + 无 goroutine/锁残留——assets-thumbs P0「fake 掩蔽缺陷」教训的存活性断言）。
func TestConvertForPreview_InFlightDebounce(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	conv := &fakePptxConverter{available: true, convertFn: func(_ int, _, dst string) error {
		started <- struct{}{}
		<-release
		return writeFakePdf(t, dst)
	}}
	svc := newTestPptxPreviewService(t, conv)
	src := filepath.Join(t.TempDir(), "c.pptx")
	mustWritePptx(t, src, time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local))

	type callResult struct {
		path string
		err  error
	}
	resCh := make(chan callResult, 2)
	go func() {
		p, err := svc.ConvertForPreview(context.Background(), src)
		resCh <- callResult{p, err}
	}()
	<-started // leader 已进入转换
	go func() {
		p, err := svc.ConvertForPreview(context.Background(), src)
		resCh <- callResult{p, err}
	}()
	// follower 已注册（给一点调度机会后放行 leader——chan 交接而非 sleep 判定：
	// 即使 follower 尚未注册，leader 完成后 follower 走缓存命中同路径，断言依旧成立）
	close(release)

	paths := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case r := <-resCh:
			if r.err != nil {
				t.Fatalf("并发调用 %d 失败: %v", i+1, r.err)
			}
			paths[r.path] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("并发调用 %d 超时（防抖等待路径疑似永挂）", i+1)
		}
	}
	if len(paths) != 1 {
		t.Errorf("两次调用应共享同一产物路径: %v", paths)
	}
	if got := conv.callCount(); got != 1 {
		t.Errorf("并发请求应共享一次转换: calls=%d, want 1", got)
	}

	// 存活性断言：防抖结束后服务仍能正常完成新转换（注册项清理 + 无锁残留）
	path3, err := svc.ConvertForPreview(context.Background(), src)
	if err != nil {
		t.Fatalf("防抖后再次调用: %v", err)
	}
	if got := conv.callCount(); got != 1 {
		t.Errorf("防抖后同文件调用应命中缓存: calls=%d, want 1", got)
	}
	if _, ok := paths[path3]; !ok {
		t.Errorf("防抖后命中应返回同一路径: %s", path3)
	}
}

// TestConvertForPreview_RetryOnce 瞬态失败重试 1 次后成功。
func TestConvertForPreview_RetryOnce(t *testing.T) {
	conv := &fakePptxConverter{available: true, convertFn: func(n int, _, dst string) error {
		if n == 1 {
			return errors.New("COM 冷启动瞬态失败")
		}
		return writeFakePdf(t, dst)
	}}
	svc := newTestPptxPreviewService(t, conv)
	src := filepath.Join(t.TempDir(), "d.pptx")
	mustWritePptx(t, src, time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local))

	path, err := svc.ConvertForPreview(context.Background(), src)
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if got := conv.callCount(); got != 2 {
		t.Errorf("瞬态失败应重试 1 次: calls=%d, want 2", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("产物应存在: %v", err)
	}
}

// TestConvertForPreview_Failed 重试后仍失败 → E_PPTX_PREVIEW_FAILED，且无
// 产物文件残留误判。
func TestConvertForPreview_Failed(t *testing.T) {
	conv := &fakePptxConverter{available: true, convertFn: func(_ int, _, _ string) error {
		return errors.New("文件损坏，无法打开")
	}}
	svc := newTestPptxPreviewService(t, conv)
	src := filepath.Join(t.TempDir(), "e.pptx")
	mustWritePptx(t, src, time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local))

	_, err := svc.ConvertForPreview(context.Background(), src)
	if err == nil {
		t.Fatal("转换失败应返回错误")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodePptxPreviewFailed {
		t.Errorf("应返回 E_PPTX_PREVIEW_FAILED: got %v", err)
	}
	if got := conv.callCount(); got != 2 {
		t.Errorf("失败应重试 1 次: calls=%d, want 2", got)
	}
}

// TestConvertForPreview_Unavailable 环境不可用（非 Windows / 无 Office）返回
// E_PPTX_PREVIEW_UNAVAILABLE，且不调转换器。
func TestConvertForPreview_Unavailable(t *testing.T) {
	conv := &fakePptxConverter{available: false}
	svc := newTestPptxPreviewService(t, conv)
	src := filepath.Join(t.TempDir(), "f.pptx")
	mustWritePptx(t, src, time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local))

	_, err := svc.ConvertForPreview(context.Background(), src)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodePptxPreviewUnavailable {
		t.Errorf("应返回 E_PPTX_PREVIEW_UNAVAILABLE: got %v", err)
	}
	if got := conv.callCount(); got != 0 {
		t.Errorf("不可用不应调转换器: calls=%d, want 0", got)
	}
}

// TestConvertForPreview_SourceMissing 源文件不存在返回失败错误。
func TestConvertForPreview_SourceMissing(t *testing.T) {
	conv := &fakePptxConverter{available: true}
	svc := newTestPptxPreviewService(t, conv)
	_, err := svc.ConvertForPreview(context.Background(), filepath.Join(t.TempDir(), "missing.pptx"))
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodePptxPreviewFailed {
		t.Errorf("源缺失应返回 E_PPTX_PREVIEW_FAILED: got %v", err)
	}
}

// TestConvertForPreview_IndexPersists 索引持久化：重建服务实例后命中缓存
// （跨会话缓存生效，不重复转换）。
func TestConvertForPreview_IndexPersists(t *testing.T) {
	dir := t.TempDir()
	conv := &fakePptxConverter{available: true, convertFn: func(_ int, _, dst string) error {
		return writeFakePdf(t, dst)
	}}
	cacheDir := filepath.Join(dir, "pptx_preview_pdf")
	svc1 := NewPptxPreviewService(cacheDir, conv)
	src := filepath.Join(dir, "g.pptx")
	mustWritePptx(t, src, time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local))

	path1, err := svc1.ConvertForPreview(context.Background(), src)
	if err != nil {
		t.Fatalf("首次转换: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "index.json")); err != nil {
		t.Fatalf("索引文件应落盘: %v", err)
	}

	svc2 := NewPptxPreviewService(cacheDir, conv)
	path2, err := svc2.ConvertForPreview(context.Background(), src)
	if err != nil {
		t.Fatalf("重建实例后调用: %v", err)
	}
	if path1 != path2 {
		t.Errorf("跨实例应命中同一缓存: %s != %s", path1, path2)
	}
	if got := conv.callCount(); got != 1 {
		t.Errorf("跨实例命中不应再转换: calls=%d, want 1", got)
	}
}

// TestConvertForPreview_IndexCorruptDegrades 索引损坏降级空索引（重新转换 +
// 重建索引文件），不阻塞调用链路。
func TestConvertForPreview_IndexCorruptDegrades(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "pptx_preview_pdf")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("建缓存目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "index.json"), []byte("{broken json"), 0o644); err != nil {
		t.Fatalf("写损坏索引失败: %v", err)
	}
	conv := &fakePptxConverter{available: true, convertFn: func(_ int, _, dst string) error {
		return writeFakePdf(t, dst)
	}}
	svc := NewPptxPreviewService(cacheDir, conv)
	src := filepath.Join(dir, "h.pptx")
	mustWritePptx(t, src, time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local))

	if _, err := svc.ConvertForPreview(context.Background(), src); err != nil {
		t.Fatalf("损坏索引应降级重转换: %v", err)
	}
	// 降级后索引随首次成功转换重建落盘，二次调用命中缓存不再转换
	if _, err := svc.ConvertForPreview(context.Background(), src); err != nil {
		t.Fatalf("重建后二次调用: %v", err)
	}
	if got := conv.callCount(); got != 1 {
		t.Errorf("损坏降级后应重转换一次且二次命中: calls=%d, want 1", got)
	}
	// 重建后的索引应可被新实例复用（降级自愈落盘）
	svc2 := NewPptxPreviewService(cacheDir, conv)
	if _, err := svc2.ConvertForPreview(context.Background(), src); err != nil {
		t.Fatalf("新实例复用重建索引: %v", err)
	}
	if got := conv.callCount(); got != 1 {
		t.Errorf("新实例应命中重建索引: calls=%d, want 1", got)
	}
}

// TestPptxPreviewLRUEviction 超限按 lastAccess 淘汰最旧（删文件 + 删索引项），
// 新鲜条目保留。LRU 上限为包级常量，测试用小文件多条目 + 逐条注入 lastAccess
// 驱动（不改常量——改包级常量影响并行测试，直接构造 N 个略大于上限的记账值）。
func TestPptxPreviewLRUEviction(t *testing.T) {
	conv := &fakePptxConverter{available: true, convertFn: func(_ int, _, dst string) error {
		return writeFakePdf(t, dst)
	}}
	svc := newTestPptxPreviewService(t, conv)
	srcDir := t.TempDir()

	// 直接构造记账条目驱动淘汰分支（不依赖真实转换顺序控制 lastAccess）：
	// 两个陈旧条目各占上限一半以上（合计必超限），另有一个小体积新鲜条目保留
	svc.mu.Lock()
	svc.loaded = true
	base := time.Now().UnixNano()
	pdfSize := int64(pptxPreviewCacheLimitBytes) // 每个陈旧条目独占上限，两个合计必超限且逐个淘汰后仍超限
	staleKeys := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		src := filepath.Join(srcDir, "old"+string(rune('a'+i))+".pptx")
		mustWritePptx(t, src, time.Unix(0, base))
		key := pptxPreviewCacheKey(src, 16, base)
		svc.entries[key] = pptxPreviewEntry{
			SrcPath: src, Size: 16, MtimeNs: base,
			LastAccess: base + int64(i), // 陈旧（远小于新鲜条目的 lastAccess）
			PdfName:    key + ".pdf", PdfSize: pdfSize,
		}
		staleKeys = append(staleKeys, key)
	}
	// 保留一个新鲜条目（lastAccess 明显更晚 + PdfSize 小）
	freshSrc := filepath.Join(srcDir, "fresh.pptx")
	mustWritePptx(t, freshSrc, time.Unix(0, base))
	freshKey := pptxPreviewCacheKey(freshSrc, 16, base)
	svc.entries[freshKey] = pptxPreviewEntry{SrcPath: freshSrc, Size: 16, MtimeNs: base, LastAccess: base + 1_000_000, PdfName: freshKey + ".pdf", PdfSize: 10}
	svc.mu.Unlock()

	svc.mu.Lock()
	svc.evictLocked()
	svc.mu.Unlock()

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.entries) != 1 {
		t.Errorf("淘汰后应仅剩新鲜条目: got %d", len(svc.entries))
	}
	if _, ok := svc.entries[freshKey]; !ok {
		t.Errorf("新鲜条目不应被淘汰")
	}
	for _, key := range staleKeys {
		if _, ok := svc.entries[key]; ok {
			t.Errorf("陈旧条目应被淘汰: %s", key)
		}
	}
}

// TestPptxPreviewLRUEviction_RemovesFiles 淘汰时产物文件同步删除。
func TestPptxPreviewLRUEviction_RemovesFiles(t *testing.T) {
	conv := &fakePptxConverter{available: true, convertFn: func(_ int, _, dst string) error {
		return writeFakePdf(t, dst)
	}}
	svc := newTestPptxPreviewService(t, conv)
	src := filepath.Join(t.TempDir(), "i.pptx")
	mustWritePptx(t, src, time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local))

	path, err := svc.ConvertForPreview(context.Background(), src)
	if err != nil {
		t.Fatalf("转换: %v", err)
	}
	// 手动把全部条目 PdfSize 抬到超限（单一旧条目总超限即淘汰自身；遍历取
	// 实际 key，避免测试侧重复推导键字段引入不一致）
	svc.mu.Lock()
	for k, e := range svc.entries {
		e.PdfSize = pptxPreviewCacheLimitBytes + 1
		svc.entries[k] = e
	}
	svc.evictLocked()
	svc.mu.Unlock()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("淘汰条目的 PDF 文件应删除: err=%v", err)
	}
}

// TestPptxPreviewCacheKey_KeyFields 缓存键覆盖 size 与 mtime 字段（任一变化
// 产生新键）。
func TestPptxPreviewCacheKey_KeyFields(t *testing.T) {
	p := `D:\demo\a.pptx`
	k1 := pptxPreviewCacheKey(p, 100, 1_000)
	k2 := pptxPreviewCacheKey(p, 200, 1_000)
	k3 := pptxPreviewCacheKey(p, 100, 2_000)
	if k1 == k2 || k1 == k3 || k2 == k3 {
		t.Errorf("size/mtime 变化应产生新键: %s %s %s", k1, k2, k3)
	}
	if pptxPreviewCacheKey(p, 100, 1_000) != k1 {
		t.Errorf("同字段应产生同键")
	}
}

// TestUnavailablePptxConverter 兜底桩：Available 恒 false / ConvertToPdf 返回
// E_PPTX_PREVIEW_UNAVAILABLE（exporter 未实现转换能力时的降级出口）。
func TestUnavailablePptxConverter(t *testing.T) {
	c := unavailablePptxConverter{}
	if c.Available() {
		t.Error("兜底桩应恒不可用")
	}
	err := c.ConvertToPdf(context.Background(), "a.pptx", "b.pdf")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodePptxPreviewUnavailable {
		t.Errorf("兜底桩应返回 E_PPTX_PREVIEW_UNAVAILABLE: got %v", err)
	}
}
