package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
)

// ---------- 测试辅助 ----------

// newTestAssetService 构造注入临时数据目录的素材服务（CloseAll 挂 Cleanup 收口
// 全部库连接，防 sqlite 连接残留干扰 goleak 观察面）。默认注入不可用 fake
// exporter：PR2 起扫描完成会触发缩略图补缺入队，不替换则装 PowerPoint 的开发机
// 会真跑 COM（环境依赖 + 拖慢测试）；需真入队链路的用例显式覆盖 svc.thumbQ。
func newTestAssetService(t *testing.T) *AssetLibraryService {
	t.Helper()
	svc := NewAssetLibraryService(context.Background(), filepath.Join(t.TempDir(), "assets"))
	svc.thumbQ = newThumbQueue(&fakeThumbExporter{available: false}, svc.eventSink, svc.thumbLedger, nil)
	t.Cleanup(svc.CloseAll)
	return svc
}

// mustCreateLibrary 建库断言辅助（参数 testing.TB 供 benchmark 复用 fixture 构造）。
func mustCreateLibrary(tb testing.TB, svc *AssetLibraryService, name, root string) model.AssetLibrary {
	tb.Helper()
	lib, err := svc.CreateLibrary(name, root)
	if err != nil {
		tb.Fatalf("CreateLibrary(%s): %v", name, err)
	}
	return lib
}

// openStoreForTest 直接取库连接（记录篡改等白盒断言与基准语料注入用）；
// 连接由服务缓存持有，测试用例结束经 CloseAll 统一收口。
func (s *AssetLibraryService) openStoreForTest(tb testing.TB, libraryID string) *assetsStore {
	tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	store, err := s.storeForLocked(libraryID)
	if err != nil {
		tb.Fatalf("打开库连接: %v", err)
	}
	return store
}

// blockingAssetsParse 阻塞型解析 fake：首个文件解析即通告并阻塞至 release 关闭，
// 驱动「在途扫描」中间态（并发拒绝 / 取消链路用例）。
type blockingAssetsParse struct {
	started chan struct{}
	release chan struct{}
}

func (f *blockingAssetsParse) parse(srcPath, baseName string) ([]parsedSlide, error) {
	select {
	case <-f.started: // 已关闭：重复调用直接走后续
	default:
		close(f.started)
	}
	<-f.release
	return parsePptxSlides(srcPath, baseName)
}

// deniedForNamesParse 解析 fake：命中指定文件名（不含扩展名）时返回权限分类
// 错误，其余委托真实解析（Windows 无法稳定构造真实无权限文件，经注入覆盖
// denied 状态机分支，见 docs/spec/test-stability.md）。
type deniedForNamesParse struct {
	names map[string]bool
}

func (f *deniedForNamesParse) parse(srcPath, baseName string) ([]parsedSlide, error) {
	if f.names[baseName] {
		return nil, &assetParseError{kind: assetParseDenied,
			err: fmt.Errorf("文件无读取权限: %w", os.ErrPermission)}
	}
	return parsePptxSlides(srcPath, baseName)
}

// ---------- 扫描主链路用例 ----------

// TestAssetScan_IndexesNewFiles 综合主链路：两 pptx（其一含备注页）扫描入库，
// 文件/幻灯片条目、状态、进度事件序列、终态事件、原始文件字节不变。
func TestAssetScan_IndexesNewFiles(t *testing.T) {
	svc := newTestAssetService(t)
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "主链路库", dir)

	src1 := buildAssetsPptx(t, dir, "演示一.pptx", []assetsSlideSpec{
		{titlePh: "首页标题", body: []string{"首页正文"}, notes: "首页备注"},
		{titlePh: "次页标题", body: []string{"次页正文"}},
	})
	buildAssetsPptx(t, dir, "演示二.pptx", []assetsSlideSpec{
		{titlePh: "另一份", body: []string{"内容"}},
	})
	// 原始文件字节基准（索引模式：扫描不得修改源文件）
	orig1, err := os.ReadFile(src1)
	if err != nil {
		t.Fatalf("读取基准: %v", err)
	}

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	if result.Total != 2 || result.New != 2 || result.Unchanged != 0 || result.Removed != 0 {
		t.Errorf("扫描结果不符：%+v", result)
	}

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("应入库 2 个文件，实际 %d", len(files))
	}
	for _, f := range files {
		if f.Status != model.AssetFileStatusOK {
			t.Errorf("文件 %s 状态 = %q, want ok", f.Path, f.Status)
		}
		if f.Title == "" || strings.HasSuffix(f.Title, ".pptx") {
			t.Errorf("文件展示标题应为源文件名去扩展名，实际 %q", f.Title)
		}
	}

	slidePage, err := svc.ListSlides(lib.ID, 0, 0)
	slides := slidePage.Items
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	if len(slides) != 3 {
		t.Fatalf("应入库 3 页幻灯片，实际 %d", len(slides))
	}
	// 溯源字段：幻灯片带归属文件相对路径，页序按 sldIdLst
	if slides[0].FilePath != "演示一.pptx" || slides[0].SlideNum != 1 || slides[0].Title != "首页标题" {
		t.Errorf("首页溯源不符：%+v", slides[0])
	}
	if slides[0].SpeakerNotes != "首页备注" {
		t.Errorf("首页备注不符：%q", slides[0].SpeakerNotes)
	}

	// 原始文件字节不变
	after, err := os.ReadFile(src1)
	if err != nil {
		t.Fatalf("读取扫描后文件: %v", err)
	}
	if string(after) != string(orig1) {
		t.Error("扫描不得修改原始文件字节")
	}

	// 事件：2 条逐文件 progress + 1 条 done
	if len(sink.calls) != 3 {
		t.Fatalf("应收到 3 条事件（2 progress + 1 done），实际 %d 条：%#v", len(sink.calls), sink.calls)
	}
	for i, want := range []struct{ done, total int }{{1, 2}, {2, 2}} {
		call := sink.calls[i]
		if call.name != "assets-scan:progress" {
			t.Fatalf("第 %d 条事件应为 assets-scan:progress，实际 %q", i, call.name)
		}
		payload, ok := call.data[0].(model.AssetScanProgress)
		if !ok || payload.LibraryID != lib.ID || payload.Done != want.done || payload.Total != want.total {
			t.Errorf("progress 事件载荷不符（期望 done=%d total=%d）：%#v", want.done, want.total, call.data)
		}
	}
	doneCall := sink.calls[2]
	if doneCall.name != "assets-scan:done" {
		t.Fatalf("末条事件应为 assets-scan:done，实际 %q", doneCall.name)
	}
	payload, ok := doneCall.data[0].(map[string]any)
	if !ok || payload["libraryId"] != lib.ID {
		t.Errorf("done 事件载荷不符：%#v", doneCall.data)
	}
}

// TestAssetScan_IncrementalSkipAndReprocess 增量判定：
//   - 未变跳过：首次扫描后的库存记录（扫描时 os.Stat 快照）与磁盘天然一致，
//     重扫不依赖 mtime 变化即确定性命中 unchanged（无时序依赖）；
//   - 变更重处理：注入陈旧记录（mtime 人为回拨 1 小时）驱动失配重解析，
//     对齐 docs/spec/test-stability.md 的「注入陈旧缓存」范式。
func TestAssetScan_IncrementalSkipAndReprocess(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "增量库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{{titlePh: "甲一", body: []string{"内容"}}})
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{{titlePh: "乙一", body: []string{"内容"}}})

	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}

	// 注入陈旧记录：仅「甲」的 mtime 回拨（size 不动），失配必重处理
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec(
		"UPDATE files SET mtime_ns = mtime_ns - 3600000000000 WHERE path = ?", "甲.pptx",
	); err != nil {
		t.Fatalf("注入陈旧记录: %v", err)
	}

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("二次扫描: %v", err)
	}
	if result.Unchanged != 1 || result.Updated != 1 || result.New != 0 {
		t.Errorf("增量判定不符（unchanged=1 updated=1），实际：%+v", result)
	}
	// 重处理后幻灯片仍完整（删旧插新无残留）
	slidePage, err := svc.ListSlides(lib.ID, 0, 0)
	slides := slidePage.Items
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	if len(slides) != 2 {
		t.Errorf("重解析后应仍为 2 页（无残留/丢失），实际 %d", len(slides))
	}
}

// TestAssetScan_ProcessingStatusResidue processing 残留行必重处理（崩溃半态自愈）：
// 注入 status=processing 但 mtime/size 与磁盘一致的记录，扫描不得按未变跳过。
func TestAssetScan_ProcessingStatusResidue(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "残留自愈库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})

	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET status = ?", model.AssetFileStatusProcessing); err != nil {
		t.Fatalf("篡改状态: %v", err)
	}

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("二次扫描: %v", err)
	}
	if result.Updated != 1 || result.Unchanged != 0 {
		t.Errorf("processing 残留应重处理（updated=1 unchanged=0），实际 %+v", result)
	}
}

// TestAssetScan_MarksLostOnDiskRemoval 磁盘文件删除后记录标记 lost（SRS §2.6.5
// D1，替代 lost 引入前的删除语义）：记录保留、幻灯片保留（标注语义存活）、
// 统计行丢失数 +1；重复扫描不重复计数（幂等）。
func TestAssetScan_MarksLostOnDiskRemoval(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "丢失标记库", dir)
	src := buildAssetsPptx(t, dir, "待删.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})

	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatalf("删除源文件: %v", err)
	}

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("二次扫描: %v", err)
	}
	if result.Lost != 1 || result.Removed != 0 {
		t.Errorf("应标记 1 条丢失且不删除（lost=1 removed=0），实际 %+v", result)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	slidePage, err := svc.ListSlides(lib.ID, 0, 0)
	slides := slidePage.Items
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	if len(files) != 1 || files[0].Status != model.AssetFileStatusLost {
		t.Fatalf("记录应保留且 status=lost，实际：%+v", files)
	}
	if len(slides) != 1 {
		t.Errorf("幻灯片应随 lost 记录保留（元数据存续），实际 %d 页", len(slides))
	}
	// 统计行丢失数
	stats, err := svc.GetLibraryStats(lib.ID)
	if err != nil {
		t.Fatalf("GetLibraryStats: %v", err)
	}
	if stats.LostCount != 1 {
		t.Errorf("统计丢失数应为 1，实际 %d", stats.LostCount)
	}

	// 重复扫描：lost 行保持 lost 不重复计数（幂等）
	result, err = svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("三次扫描: %v", err)
	}
	if result.Lost != 0 {
		t.Errorf("已 lost 的行不应重复计数，实际 %+v", result)
	}
}

// TestAssetScan_ClassificationContinues 异常分类不中断批量：正常 / 加密（OLE
// 魔数真实样本）/ 损坏（非 zip） / 权限（注入 fake）四形态混合目录，扫描成功
// 返回且各文件状态正确；统计口径同步验证。
func TestAssetScan_ClassificationContinues(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "分类库", dir)
	buildAssetsPptx(t, dir, "正常.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	buildOLEEncryptedPptx(t, dir, "加密.pptx")
	corruptPath := filepath.Join(dir, "损坏.pptx")
	if err := os.WriteFile(corruptPath, []byte("not a zip at all"), 0644); err != nil {
		t.Fatalf("写入损坏样本: %v", err)
	}
	// 权限样本：真实文件存在（内容不限），denied 分支由解析 fake 注入驱动
	// （Windows 无法稳定构造真实无权限文件，docs/spec/test-stability.md）
	deniedPath := filepath.Join(dir, "拒绝.pptx")
	if err := os.WriteFile(deniedPath, []byte("irrelevant"), 0644); err != nil {
		t.Fatalf("写入权限样本: %v", err)
	}
	svc.parseFn = (&deniedForNamesParse{names: map[string]bool{"拒绝": true}}).parse

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("分类异常不得中断扫描: %v", err)
	}
	if result.New != 1 || result.Encrypted != 1 || result.Failed != 1 || result.Denied != 1 {
		t.Errorf("分类计数不符（new=1 encrypted=1 failed=1 denied=1），实际：%+v", result)
	}

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	statusByPath := make(map[string]string, len(files))
	for _, f := range files {
		statusByPath[f.Path] = f.Status
	}
	wantStatus := map[string]string{
		"加密.pptx": model.AssetFileStatusEncrypted,
		"损坏.pptx": model.AssetFileStatusFailed,
		"拒绝.pptx": model.AssetFileStatusDenied,
		"正常.pptx": model.AssetFileStatusOK,
	}
	for p, want := range wantStatus {
		if statusByPath[p] != want {
			t.Errorf("文件 %s 状态 = %q, want %q", p, statusByPath[p], want)
		}
	}

	stats, err := svc.GetLibraryStats(lib.ID)
	if err != nil {
		t.Fatalf("GetLibraryStats: %v", err)
	}
	if stats.FileCount != 4 || stats.SlideCount != 1 || stats.FailedCount != 1 || stats.EncryptedCount != 1 {
		t.Errorf("统计口径不符：%+v", stats)
	}

	// 扫描结束后取消注册表无残留
	if svc.CancelScan(lib.ID) {
		t.Error("扫描结束后注册表应清空（取消应返回 false）")
	}
}

// TestAssetScan_CancelPreCancelled 预取消 ctx：立即返回 E_ASSETS_CANCELLED
// AppError 并发 error 事件；defer 注销后注册表无残留。
func TestAssetScan_CancelPreCancelled(t *testing.T) {
	svc := newTestAssetService(t)
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "预取消库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.StartScan(ctx, lib.ID)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
		t.Fatalf("取消应映射为 E_ASSETS_CANCELLED AppError，实际：%v", err)
	}
	if len(sink.calls) != 1 || sink.calls[0].name != "assets-scan:error" {
		t.Fatalf("应收到 1 条 assets-scan:error，实际：%#v", sink.calls)
	}
	if payload, ok := sink.calls[0].data[0].(map[string]any); !ok || payload["message"] != "扫描已取消" {
		t.Errorf("error 事件载荷应为用户可读消息，实际：%#v", sink.calls[0].data)
	}
	if svc.CancelScan(lib.ID) {
		t.Error("扫描返回后注册表应无残留")
	}
}

// TestAssetScan_ConcurrentRejectedAndCancel 同库并发互斥与在途取消：
// 阻塞型解析 fake 保持首扫描在途 → 第二次扫描同库被拒（E_ASSETS_SCAN_IN_PROGRESS）
// → CancelScan 命中在途 → 首扫描在下一文件检查点返回取消错误（两文件保证取消后
// 仍有检查点可命中）。
func TestAssetScan_ConcurrentRejectedAndCancel(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "并发库", dir)
	buildAssetsPptx(t, dir, "阻塞一.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	buildAssetsPptx(t, dir, "阻塞二.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})

	blocker := &blockingAssetsParse{started: make(chan struct{}), release: make(chan struct{})}
	svc.parseFn = blocker.parse

	done := make(chan error, 1)
	go func() {
		_, err := svc.StartScan(context.Background(), lib.ID)
		done <- err
	}()
	<-blocker.started

	// 在途期间同库二次扫描被拒
	_, err := svc.StartScan(context.Background(), lib.ID)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsScanInProgress {
		t.Fatalf("同库并发扫描应被拒（E_ASSETS_SCAN_IN_PROGRESS），实际：%v", err)
	}
	// 在途期间删除库同样被拒（复用扫描锁互斥）
	if err := svc.DeleteLibrary(lib.ID); !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsScanInProgress {
		t.Fatalf("扫描中删库应被拒，实际：%v", err)
	}

	if !svc.CancelScan(lib.ID) {
		t.Fatal("在途扫描应可取消")
	}
	// 取消信号经 ctx 在下一文件检查点生效：闭掉解析阻塞让在途文件返回
	close(blocker.release)
	select {
	case err := <-done:
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
			t.Fatalf("取消错误应映射为 E_ASSETS_CANCELLED，实际：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消后扫描未返回")
	}
	if svc.CancelScan(lib.ID) {
		t.Error("扫描结束后注册表应清空")
	}
}

// TestAssetScan_DerivedDirAndExtensionFilter 清单过滤：PPT 拆分派生副本
// （<同名文件夹>/<同名>-<页序>.pptx 布局，对齐 pptx_split.go 输出约定）整类
// 排除（含 ≥100 页的三位页序形态）；同名异构非派生命名不误伤；仅 .pptx 后缀
// 入库（大小写不敏感）；子目录递归。
func TestAssetScan_DerivedDirAndExtensionFilter(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "过滤库", dir)
	// 源文件 + 其拆分产物布局（源文件/源文件-01.pptx 等，应排除）
	buildAssetsPptx(t, dir, "源文件.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	derivedDir := filepath.Join(dir, "源文件")
	if err := os.MkdirAll(derivedDir, 0755); err != nil {
		t.Fatalf("建拆分输出目录: %v", err)
	}
	buildAssetsPptx(t, derivedDir, "源文件-01.pptx", []assetsSlideSpec{{titlePh: "派生一", body: []string{"正文"}}})
	buildAssetsPptx(t, derivedDir, "源文件-100.pptx", []assetsSlideSpec{{titlePh: "派生百", body: []string{"正文"}}})
	// 同构命名但目录名与文件名主干不同（非拆分布局）：不得误伤
	pkgDir := filepath.Join(dir, "素材包")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("建素材包目录: %v", err)
	}
	buildAssetsPptx(t, pkgDir, "介绍-01.pptx", []assetsSlideSpec{{titlePh: "介绍", body: []string{"正文"}}})
	// 根目录直置的「名字-01.pptx」（父目录为根，无同名文件夹）：不得误伤
	buildAssetsPptx(t, dir, "直置-01.pptx", []assetsSlideSpec{{titlePh: "直置", body: []string{"正文"}}})
	if err := os.WriteFile(filepath.Join(dir, "说明.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatalf("写非 pptx: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "子目录"), 0755); err != nil {
		t.Fatalf("建子目录: %v", err)
	}
	buildAssetsPptx(t, filepath.Join(dir, "子目录"), "大写.PPTX", []assetsSlideSpec{{titlePh: "大写", body: []string{"正文"}}})

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	if result.Total != 4 || result.New != 4 {
		t.Errorf("拆分副本应排除且大小写不敏感递归（total=4 new=4），实际：%+v", result)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	paths := make(map[string]bool)
	for _, f := range files {
		paths[f.Path] = true
	}
	for _, want := range []string{"源文件.pptx", "子目录/大写.PPTX", "素材包/介绍-01.pptx", "直置-01.pptx"} {
		if !paths[want] {
			t.Errorf("应索引 %s，实际：%v", want, paths)
		}
	}
	for _, banned := range []string{"源文件/源文件-01.pptx", "源文件/源文件-100.pptx"} {
		if paths[banned] {
			t.Errorf("拆分派生副本 %s 不应入库", banned)
		}
	}
}

// TestIsPptxSplitDerivative 派生判定单测：标准拆分布局命中；根目录直置
// 「名字-01.pptx」与「.」主干等异构命名不误伤。回归锚点：根目录文件的
// parentBase="."，文件名主干以「.」开头时（如 .-01.pptx）曾因前缀恰与「.」
// 匹配被误判为派生而漏索引（审核发现，2026-10-14 修复）。
func TestIsPptxSplitDerivative(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		{"源文件/源文件-01.pptx", true},     // 标准拆分产物（两位页序）
		{"源文件/源文件-100.pptx", true},    // 三位页序（≥100 页）
		{"子目录/源文件/源文件-01.pptx", true}, // 多级目录下的标准拆分布局
		{"源文件.pptx", false},           // 根目录源文件
		{"直置-01.pptx", false},         // 根目录直置「名字-01.pptx」
		{".-01.pptx", false},          // 回归：根目录「.」主干误判
		{"素材包/介绍-01.pptx", false},     // 目录名与文件主干不同
		{"源文件/源文件-x1.pptx", false},    // 页序段含非数字
		{"源文件/源文件-.pptx", false},      // 页序段为空
		{"说明.txt", false},             // 非 .pptx 后缀
	}
	for _, tc := range cases {
		if got := isPptxSplitDerivative(tc.rel); got != tc.want {
			t.Errorf("isPptxSplitDerivative(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

// ---------- 库管理用例 ----------

// TestAssetLibrary_CreateValidation 建库校验：名称非空、目录存在且为目录；
// 建库即预建索引库文件（磁盘/驱动问题建库期暴露）。
func TestAssetLibrary_CreateValidation(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()

	if _, err := svc.CreateLibrary("  ", dir); err == nil {
		t.Error("空名称应被拒绝")
	}
	if _, err := svc.CreateLibrary("缺失目录", filepath.Join(dir, "不存在")); err == nil {
		t.Error("不存在目录应被拒绝")
	}
	filePath := filepath.Join(dir, "普通文件")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("写文件: %v", err)
	}
	if _, err := svc.CreateLibrary("文件非目录", filePath); err == nil {
		t.Error("文件路径应被拒绝（必须是目录）")
	}

	lib, err := svc.CreateLibrary("有效库", dir)
	if err != nil {
		t.Fatalf("有效建库: %v", err)
	}
	if lib.ID == "" || lib.RootPath == "" {
		t.Errorf("建库返回信息不完整：%+v", lib)
	}
	// 注册表落盘（util.SaveJSON 原子写）
	if _, err := os.Stat(filepath.Join(svc.assetsDir, "libraries.json")); err != nil {
		t.Errorf("注册表文件应已落盘: %v", err)
	}
	// 索引库文件预建
	if _, err := os.Stat(svc.libraryDBPath(lib.ID)); err != nil {
		t.Errorf("索引库文件应已预建: %v", err)
	}
}

// TestAssetLibrary_CreateRollsBackOrphanDB 注册表写失败时回删预建索引库文件
// （防孤儿 <id>.db 残留：库里无此库登记，删库流程不会清理它）。注入方式：注册表
// 路径被同名目录占位，util.SaveJSON 必失败。
func TestAssetLibrary_CreateRollsBackOrphanDB(t *testing.T) {
	assetsDir := filepath.Join(t.TempDir(), "assets")
	// libraries.json 占位为目录：SaveJSON 的 temp 创建/rename 必失败
	if err := os.MkdirAll(filepath.Join(assetsDir, "libraries.json"), 0755); err != nil {
		t.Fatalf("占位注册表路径: %v", err)
	}
	svc := NewAssetLibraryService(context.Background(), assetsDir)
	t.Cleanup(svc.CloseAll)

	if _, err := svc.CreateLibrary("孤儿库", t.TempDir()); err == nil {
		t.Fatal("注册表写失败应使建库失败")
	}
	leftovers, err := filepath.Glob(filepath.Join(assetsDir, "*.db*"))
	if err != nil {
		t.Fatalf("扫描残留文件: %v", err)
	}
	if len(leftovers) > 0 {
		t.Errorf("注册表写失败后应回删孤儿索引库文件（含 WAL/SHM），残留：%v", leftovers)
	}
	// 连接缓存同步清理：回滚路径必须关连接并 delete(s.open, lib.ID)
	svc.mu.Lock()
	cached := len(svc.open)
	svc.mu.Unlock()
	if cached != 0 {
		t.Errorf("回滚后连接缓存应清空，实际残留 %d 条", cached)
	}
}

// TestAssetLibrary_WaitScansDone 关停收口：WaitScansDone 触发在途扫描取消并等待
// 其退出——扫描阻塞期间不返回（无超时强断语义），取消信号放行后即返回。
func TestAssetLibrary_WaitScansDone(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "收口库", dir)
	buildAssetsPptx(t, dir, "阻塞一.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	buildAssetsPptx(t, dir, "阻塞二.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})

	blocker := &blockingAssetsParse{started: make(chan struct{}), release: make(chan struct{})}
	svc.parseFn = blocker.parse

	scanDone := make(chan error, 1)
	go func() {
		_, err := svc.StartScan(context.Background(), lib.ID)
		scanDone <- err
	}()
	<-blocker.started

	drained := make(chan struct{})
	go func() {
		svc.WaitScansDone()
		close(drained)
	}()

	// 解析阻塞在 channel 上（ctx 取消检查点不可达）：WaitScansDone 必须仍等待
	//（负向断言需时间窗，阻塞为确定性 channel 等待，无时序脆弱性）
	select {
	case <-drained:
		t.Fatal("在途扫描未退出时 WaitScansDone 不应返回")
	case <-time.After(100 * time.Millisecond):
	}

	// 放行阻塞：扫描在下一文件检查点感知取消并退出，WaitScansDone 随之返回
	close(blocker.release)
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("取消后 WaitScansDone 未返回")
	}
	select {
	case err := <-scanDone:
		var appErr *model.AppError
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
			t.Fatalf("在途扫描应被 WaitScansDone 取消（E_ASSETS_CANCELLED），实际：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("在途扫描未按取消收尾")
	}

	// 无在途扫描时幂等立即返回
	done := make(chan struct{})
	go func() {
		svc.WaitScansDone()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("无在途扫描时 WaitScansDone 应立即返回")
	}
}

// TestAssetLibrary_ListSlidesPagination 幻灯片清单分页（对齐 SearchAssets 分页
// 契约）：limit/offset 窗口、Total 恒为总数不随页变化、默认值/钳制（limit ≤0
// 取 50、>200 钳 200、offset<0 按 0）、库不存在报 E_ASSETS_LIB_NOT_FOUND。
func TestAssetLibrary_ListSlidesPagination(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "分页库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{
		{titlePh: "甲一", body: []string{"内容"}},
		{titlePh: "甲二", body: []string{"内容"}},
	})
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{
		{titlePh: "乙一", body: []string{"内容"}},
		{titlePh: "乙二", body: []string{"内容"}},
	})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("StartScan: %v", err)
	}

	// 分页窗口：limit=3 offset 0/3 → 3+1，Total 恒 4
	page1, err := svc.ListSlides(lib.ID, 3, 0)
	if err != nil {
		t.Fatalf("首页: %v", err)
	}
	if len(page1.Items) != 3 || page1.Total != 4 {
		t.Errorf("首页应为 3 条且 Total=4，实际：%d 条 Total=%d", len(page1.Items), page1.Total)
	}
	page2, err := svc.ListSlides(lib.ID, 3, 3)
	if err != nil {
		t.Fatalf("次页: %v", err)
	}
	if len(page2.Items) != 1 || page2.Total != 4 {
		t.Errorf("次页应为 1 条且 Total=4，实际：%d 条 Total=%d", len(page2.Items), page2.Total)
	}
	// 分页窗口无缝衔接：首页 + 次页拼接与全量清单（同序键）逐位一致
	allPage, err := svc.ListSlides(lib.ID, 50, 0)
	if err != nil {
		t.Fatalf("全量页: %v", err)
	}
	if len(allPage.Items) != 4 {
		t.Fatalf("全量应为 4 条，实际 %d", len(allPage.Items))
	}
	seam := append(append([]model.AssetSlideEntry{}, page1.Items...), page2.Items...)
	for i, want := range allPage.Items {
		if seam[i].ID != want.ID {
			t.Errorf("分页拼接第 %d 位应与全量一致，实际 %+v vs %+v", i, seam[i], want)
		}
	}
	// 越界 offset：空页、Total 不变
	page3, err := svc.ListSlides(lib.ID, 3, 4)
	if err != nil {
		t.Fatalf("越界页: %v", err)
	}
	if len(page3.Items) != 0 || page3.Total != 4 {
		t.Errorf("越界页应为空且 Total=4，实际：%d 条 Total=%d", len(page3.Items), page3.Total)
	}

	// 默认值与钳制：limit=0 → 默认 50（4 条全回）；limit=-1/offset=-5 → 同默认钳制
	for _, tc := range []struct{ limit, offset int }{{0, 0}, {-1, -5}} {
		page, err := svc.ListSlides(lib.ID, tc.limit, tc.offset)
		if err != nil {
			t.Fatalf("默认值用例 (%d,%d): %v", tc.limit, tc.offset, err)
		}
		if len(page.Items) != 4 || page.Total != 4 {
			t.Errorf("默认值用例 (%d,%d) 应取全部 4 条，实际：%d 条", tc.limit, tc.offset, len(page.Items))
		}
	}

	// 库不存在
	_, err = svc.ListSlides("不存在的库", 10, 0)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsLibNotFound {
		t.Errorf("库不存在应报 E_ASSETS_LIB_NOT_FOUND，实际：%v", err)
	}
}

// TestAssetLibrary_ListDelete 生命周期：多库列举、删除清文件与注册表、重复删除报
// 不存在（E_ASSETS_LIB_NOT_FOUND）；删除后新建同目录库可用（id 不复用）。
func TestAssetLibrary_ListDelete(t *testing.T) {
	svc := newTestAssetService(t)
	dirA := t.TempDir()
	dirB := t.TempDir()
	libA := mustCreateLibrary(t, svc, "库A", dirA)
	libB := mustCreateLibrary(t, svc, "库B", dirB)

	libs, err := svc.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(libs) != 2 {
		t.Fatalf("应列举 2 个库，实际 %d", len(libs))
	}

	if err := svc.DeleteLibrary(libA.ID); err != nil {
		t.Fatalf("DeleteLibrary: %v", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(svc.libraryDBPath(libA.ID) + suffix); !os.IsNotExist(err) {
			t.Errorf("库 db 文件 %q 应已删除", suffix)
		}
	}
	libs, err = svc.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(libs) != 1 || libs[0].ID != libB.ID {
		t.Errorf("删除后应仅剩库B，实际：%+v", libs)
	}

	err = svc.DeleteLibrary(libA.ID)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsLibNotFound {
		t.Fatalf("重复删除应报 E_ASSETS_LIB_NOT_FOUND，实际：%v", err)
	}
}

// TestAssetLibrary_RegistryPersistence 注册表持久化：新服务实例同数据目录可
// 加载既有库（跨进程重启语义）。
func TestAssetLibrary_RegistryPersistence(t *testing.T) {
	assetsDir := filepath.Join(t.TempDir(), "assets")
	svc1 := NewAssetLibraryService(context.Background(), assetsDir)
	t.Cleanup(svc1.CloseAll)
	lib := mustCreateLibrary(t, svc1, "持久化库", t.TempDir())

	svc2 := NewAssetLibraryService(context.Background(), assetsDir)
	t.Cleanup(svc2.CloseAll)
	libs, err := svc2.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(libs) != 1 || libs[0].ID != lib.ID || libs[0].Name != "持久化库" {
		t.Errorf("新实例应加载注册表既有库，实际：%+v", libs)
	}
}

// TestAssetLibrary_UpdateFileMetaAndTags 元数据编辑与标签继承：
// 标签自动创建（默认色非空、确定性）、整体替换、星级/备注回显、幻灯片继承
// 文件标签、星级越界拒绝、文件不存在报错。
func TestAssetLibrary_UpdateFileMetaAndTags(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "元数据库", dir)
	buildAssetsPptx(t, dir, "材料.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"正文"}},
		{titlePh: "二", body: []string{"正文"}},
	})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	fileID := files[0].ID

	// 首次编辑：标签「城商行」「重点」自动创建
	if err := svc.UpdateFileMeta(lib.ID, fileID, model.AssetFileMetaUpdate{
		Rating: 4, Note: "季度主打材料", Tags: []string{"城商行", "重点", "重点"},
	}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	updated, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	f := updated[0]
	if f.Rating != 4 || f.Note != "季度主打材料" {
		t.Errorf("星级/备注未回显：%+v", f)
	}
	if len(f.Tags) != 2 {
		t.Fatalf("去重后应 2 个标签，实际：%+v", f.Tags)
	}
	// 标签按名排序 + 默认色分配
	if f.Tags[0].Name != "城商行" || f.Tags[1].Name != "重点" {
		t.Errorf("标签应按名排序：%+v", f.Tags)
	}
	for _, tag := range f.Tags {
		if tag.Color == "" || len(tag.Color) != 7 || tag.Color[0] != '#' {
			t.Errorf("自动创建标签应带默认色（#RRGGBB），实际 %q", tag.Color)
		}
	}

	// 幻灯片继承文件标签
	slidePage, err := svc.ListSlides(lib.ID, 0, 0)
	slides := slidePage.Items
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	for _, s := range slides {
		if len(s.Tags) != 2 {
			t.Errorf("幻灯片应继承归属文件 2 个标签（页 %d），实际：%+v", s.SlideNum, s.Tags)
		}
	}

	// 二次编辑：标签整体替换为 1 个
	if err := svc.UpdateFileMeta(lib.ID, fileID, model.AssetFileMetaUpdate{
		Rating: 0, Note: "", Tags: []string{"重点"},
	}); err != nil {
		t.Fatalf("二次 UpdateFileMeta: %v", err)
	}
	updated, _ = svc.ListFiles(lib.ID)
	if len(updated[0].Tags) != 1 || updated[0].Tags[0].Name != "重点" {
		t.Errorf("标签应整体替换为 [重点]，实际：%+v", updated[0].Tags)
	}
	if updated[0].Rating != 0 || updated[0].Note != "" {
		t.Errorf("整体替换语义：星级/备注应清零，实际：%+v", updated[0])
	}

	// 星级越界
	if err := svc.UpdateFileMeta(lib.ID, fileID, model.AssetFileMetaUpdate{Rating: 6}); err == nil {
		t.Error("星级 6 应被拒绝")
	}
	// 文件不存在
	err = svc.UpdateFileMeta(lib.ID, fileID+999, model.AssetFileMetaUpdate{Rating: 1})
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Errorf("文件不存在应报可读错误，实际：%v", err)
	}
}

// TestAssetsStore_MigrateVersionTooNew 库文件版本高于当前程序时拒绝打开
// （防旧程序写坏新版本数据）。
func TestAssetsStore_MigrateVersionTooNew(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "future.db")
	store, err := openAssetsStore(dbPath)
	if err != nil {
		t.Fatalf("首次打开: %v", err)
	}
	if _, err := store.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", assetsSchemaVersion+10)); err != nil {
		t.Fatalf("篡改版本: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("关闭: %v", err)
	}
	if _, err := openAssetsStore(dbPath); err == nil {
		t.Fatal("版本过新的库文件应拒绝打开")
	} else if !strings.Contains(err.Error(), "版本过新") {
		t.Errorf("错误提示应含「版本过新」，实际：%v", err)
	}
}

// TestAssetScan_EmptyRootDir 空目录扫描：零文件零事件终态 done，不出错。
func TestAssetScan_EmptyRootDir(t *testing.T) {
	svc := newTestAssetService(t)
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)
	lib := mustCreateLibrary(t, svc, "空库", t.TempDir())

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("空目录扫描: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("空目录应 total=0，实际：%+v", result)
	}
	if len(sink.calls) != 1 || sink.calls[0].name != "assets-scan:done" {
		t.Errorf("空目录应仅终态 done 事件，实际：%#v", sink.calls)
	}
}
