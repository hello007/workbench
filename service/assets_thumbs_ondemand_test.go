package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"workbench/model"
	"workbench/util/testutil"
)

// ---------- 纯函数：档位过滤与页白名单 ----------

// TestBatchBacklogTiers 批量补缺只取 small 档（hd 退出批量补缺，PRD R1）。
func TestBatchBacklogTiers(t *testing.T) {
	got := batchBacklogTiers(thumbTiers)
	if len(got) != 1 || got[0].Key != "small" {
		t.Fatalf("批量补缺档位应只含 small: got %+v", got)
	}
	if batchBacklogTiers([]thumbTier{{Key: "hd", Width: 1600, Height: 900}}) != nil {
		t.Fatal("不含 small 的档位集合应返回 nil（调用方跳过补缺）")
	}
}

// TestPageRequested nil 表示全书恒导；白名单只命中指定页。
func TestPageRequested(t *testing.T) {
	if !pageRequested(nil, 7) {
		t.Fatal("pages 为 nil 应导出全部页")
	}
	if !pageRequested([]int{3}, 3) {
		t.Fatal("白名单命中页应导出")
	}
	if pageRequested([]int{3}, 4) {
		t.Fatal("白名单未命中页不应导出")
	}
}

// ---------- RequestAssetThumb 按需链路 ----------

// newOnDemandTestService 按需链路夹具：Available=true fake + 可控 exportFn +
// done 事件通知 sink + 真实库根下的源文件。
func newOnDemandTestService(t *testing.T) (*AssetLibraryService, *fakeThumbExporter, doneNotifySink, model.AssetLibrary, int64) {
	t.Helper()
	fake := &fakeThumbExporter{available: true}
	svc := newTestAssetService(t)
	sink := doneNotifySink{inner: &fakeEventSink{}, doneCh: make(chan model.AssetThumbsResult, 4)}
	svc.SetEventSink(&sink)
	svc.thumbQ = newThumbQueue(fake, svc.eventSink, svc.thumbLedger, nil)
	t.Cleanup(svc.thumbLedger.Shutdown)
	t.Cleanup(svc.WaitThumbsDone)

	root := t.TempDir()
	testutil.WriteFile(t, filepath.Join(root, "甲.pptx"), "src")
	lib := mustCreateLibrary(t, svc, "按需库", root)
	store := svc.openStoreForTest(t, lib.ID)
	fileID := insertHostFile(t, store, "甲.pptx")
	return svc, fake, sink, lib, fileID
}

// waitDone 等待一次 assets-thumbs:done（chan 同步替代 sleep）。
func waitOnDemandDone(t *testing.T, sink doneNotifySink) model.AssetThumbsResult {
	t.Helper()
	select {
	case res := <-sink.doneCh:
		return res
	case <-time.After(5 * time.Second):
		t.Fatal("等待 assets-thumbs:done 超时")
		return model.AssetThumbsResult{}
	}
}

// TestRequestAssetThumb_EnqueuesSinglePageTier 按需请求只导单页单档：导出参数
// 档位为 hd、页白名单为目标页；不发 progress 事件；done 载荷 OnDemand=true。
func TestRequestAssetThumb_EnqueuesSinglePageTier(t *testing.T) {
	svc, fake, sink, lib, fileID := newOnDemandTestService(t)

	if err := svc.RequestAssetThumb(lib.ID, fileID, "甲.pptx", 2, 1); err != nil {
		t.Fatalf("RequestAssetThumb: %v", err)
	}
	res := waitOnDemandDone(t, sink)
	if !res.OnDemand {
		t.Fatal("按需批次 done 载荷应带 OnDemand=true")
	}
	if len(fake.exportArgs) != 1 {
		t.Fatalf("应导出 1 次, got %d", len(fake.exportArgs))
	}
	args := fake.exportArgs[0]
	if len(args.Tiers) != 1 || args.Tiers[0].Key != "hd" {
		t.Fatalf("按需导出档位应为 hd: %+v", args.Tiers)
	}
	if len(args.Pages) != 1 || args.Pages[0] != 2 {
		t.Fatalf("按需导出页白名单应为 [2]: %+v", args.Pages)
	}
	for _, call := range sink.inner.calls {
		if call.name == "assets-thumbs:progress" {
			t.Fatal("按需批次不应发 progress 事件（前端批量进度条不闪）")
		}
	}
}

// TestRequestAssetThumb_IdempotentWhenPNGExists 目标 PNG 已存在直接返回，不入队。
func TestRequestAssetThumb_IdempotentWhenPNGExists(t *testing.T) {
	svc, fake, _, lib, fileID := newOnDemandTestService(t)
	writeThumbPNG(t, svc.assetThumbsDir(lib.ID), "hd", fileID, 2, "exists")

	if err := svc.RequestAssetThumb(lib.ID, fileID, "甲.pptx", 2, 1); err != nil {
		t.Fatalf("RequestAssetThumb: %v", err)
	}
	if exports, _, _, _ := fake.stats(); exports != 0 {
		t.Fatalf("PNG 已存在不应入队导出, got %d 次", exports)
	}
}

// TestRequestAssetThumb_DedupInFlight 同页请求在途时去重：并发第二次请求不再
// 入队（只管单次导出）。
func TestRequestAssetThumb_DedupInFlight(t *testing.T) {
	svc, fake, sink, lib, fileID := newOnDemandTestService(t)
	started := make(chan struct{})
	release := make(chan struct{})
	fake.exportFn = func(_ *fakeThumbExporter, _ context.Context, _ thumbFileSpec, _ string, _ []thumbTier, _ []int, _ func() *thumbJumpRequest) (int, error) {
		close(started)
		<-release
		return 2, nil
	}

	if err := svc.RequestAssetThumb(lib.ID, fileID, "甲.pptx", 2, 1); err != nil {
		t.Fatalf("首次请求: %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("首次按需导出未开始")
	}
	if err := svc.RequestAssetThumb(lib.ID, fileID, "甲.pptx", 2, 1); err != nil {
		t.Fatalf("重复请求应静默去重: %v", err)
	}
	close(release)
	waitOnDemandDone(t, sink)

	if exports, _, _, _ := fake.stats(); exports != 1 {
		t.Fatalf("同页在途应去重为单次导出, got %d 次", exports)
	}
}

// TestRequestAssetThumb_InvalidParams 参数与越界校验（错值不浪费 COM 打开）。
func TestRequestAssetThumb_InvalidParams(t *testing.T) {
	svc, fake, _, lib, fileID := newOnDemandTestService(t)

	cases := []struct {
		name      string
		fileID    int64
		relPath   string
		slideNum  int
		tierIndex int
	}{
		{"档位越界", fileID, "甲.pptx", 1, 2},
		{"档位负数", fileID, "甲.pptx", 1, -1},
		{"页序非法", fileID, "甲.pptx", 0, 0},
		{"文件 ID 非法", 0, "甲.pptx", 1, 0},
		{"路径为空", fileID, "", 1, 0},
		{"路径越界", fileID, "../outside.pptx", 1, 0},
		{"源文件不存在", fileID, "不存在.pptx", 1, 0},
	}
	for _, c := range cases {
		if err := svc.RequestAssetThumb(lib.ID, c.fileID, c.relPath, c.slideNum, c.tierIndex); err == nil {
			t.Fatalf("%s: 应返回错误", c.name)
		}
	}
	if exports, _, _, _ := fake.stats(); exports != 0 {
		t.Fatalf("非法参数不应触发导出, got %d 次", exports)
	}
}

// TestRequestAssetThumb_Unavailable 无 PowerPoint 返回 errThumbsUnavailable。
func TestRequestAssetThumb_Unavailable(t *testing.T) {
	svc, fake, _, lib, fileID := newOnDemandTestService(t)
	fake.available = false
	err := svc.RequestAssetThumb(lib.ID, fileID, "甲.pptx", 1, 1)
	if err != errThumbsUnavailable {
		t.Fatalf("无 PowerPoint 应返回 errThumbsUnavailable, got %v", err)
	}
}

// TestEnqueueAssetThumbs_DefaultTiersOnlySmall 批量批次默认只导 small 档：hd 已
// 退出批量链路（详情侧按需），批量再导 hd 会白耗 COM 时间并挤占 hd 配额。
func TestEnqueueAssetThumbs_DefaultTiersOnlySmall(t *testing.T) {
	svc, fake, sink, lib, fileID := newOnDemandTestService(t)
	svc.EnqueueAssetThumbs(lib.ID, []thumbFileSpec{{
		FileID: fileID, Path: "甲.pptx", AbsPath: filepath.Join(lib.RootPath, "甲.pptx"),
	}}, svc.assetThumbsDir(lib.ID))
	res := waitOnDemandDone(t, sink)
	if res.OnDemand {
		t.Fatal("批量批次 done 不应带 OnDemand 标记")
	}
	if len(fake.exportArgs) != 1 {
		t.Fatalf("应导出 1 次, got %d", len(fake.exportArgs))
	}
	args := fake.exportArgs[0]
	if len(args.Tiers) != 1 || args.Tiers[0].Key != "small" {
		t.Fatalf("批量批次档位应只含 small: %+v", args.Tiers)
	}
	if args.Pages != nil {
		t.Fatalf("批量批次页白名单应为 nil（全书）, got %+v", args.Pages)
	}
}

// ---------- 记账分流：页白名单不触发页数缩减对账 ----------

// TestRecordExportedPages_WhitelistSkipsPrune 白名单批次只对目标页记账，且不做
// pruneBeyond——按「本次导出页数」裁剪会把第 1 页之后的既有产物误删。
func TestRecordExportedPages_WhitelistSkipsPrune(t *testing.T) {
	svc, _, _, lib, fileID := newOnDemandTestService(t)
	dir := svc.assetThumbsDir(lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	for page := 1; page <= 3; page++ {
		writeThumbPNG(t, dir, "hd", fileID, page, "png")
		// pruneBeyond 以记账行为越界页依据（thumbPagesBeyond 查 thumb_cache），
		// 无记账行则无从裁剪——夹具须同时建记账。
		insertThumbLedgerRow(t, store, fileID, "hd", page, 50, int64(page))
	}
	tier := []thumbTier{{Key: "hd", Width: 1600, Height: 900}}
	spec := thumbFileSpec{FileID: fileID, Path: "甲.pptx"}

	// 白名单单页：越界页 3 的产物必须保留
	whitelist := &thumbBatchTask{LibraryID: lib.ID, Files: []thumbFileSpec{spec}, ThumbsDir: dir, Tiers: tier, Pages: []int{2}}
	svc.thumbQ.recordExportedPages(whitelist, spec, tier, 2)
	if _, err := os.Stat(thumbPNGPath(dir, "hd", fileID, 3)); err != nil {
		t.Fatalf("白名单批次不应删除未导出页产物: %v", err)
	}

	// 全页批次对照：页数缩减对账删除越界页
	full := &thumbBatchTask{LibraryID: lib.ID, Files: []thumbFileSpec{spec}, ThumbsDir: dir, Tiers: tier}
	svc.thumbQ.recordExportedPages(full, spec, tier, 2)
	if _, err := os.Stat(thumbPNGPath(dir, "hd", fileID, 3)); !os.IsNotExist(err) {
		t.Fatalf("全页批次应按页数清理越界页产物, stat err=%v", err)
	}
}

// TestRequestAssetThumb_RejectsFileIDMismatch fileID 与路径不一致拒绝（浏览器通道
// 可达：不校验则可把 A 文件页面写进 B 的缓存键）。
func TestRequestAssetThumb_RejectsFileIDMismatch(t *testing.T) {
	svc, fake, _, lib, fileID := newOnDemandTestService(t)
	if err := svc.RequestAssetThumb(lib.ID, fileID+1000, "甲.pptx", 1, 1); err == nil {
		t.Fatal("fileID 与路径不一致应报错")
	}
	if exports, _, _, _ := fake.stats(); exports != 0 {
		t.Fatalf("校验失败不应触发导出, got %d 次", exports)
	}
}

// TestRequestAssetThumb_CanceledByLibrary 按需任务纳入库级取消面：Clear/删库链路
// 取消后任务以 done.Canceled 终态收口（不会在档目录 RemoveAll 后继续写旧分辨率图）。
func TestRequestAssetThumb_CanceledByLibrary(t *testing.T) {
	svc, fake, sink, lib, fileID := newOnDemandTestService(t)
	started := make(chan struct{})
	release := make(chan struct{})
	fake.exportFn = func(_ *fakeThumbExporter, ctx context.Context, _ thumbFileSpec, _ string, _ []thumbTier, _ []int, _ func() *thumbJumpRequest) (int, error) {
		close(started)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-release:
			return 1, nil
		}
	}
	if err := svc.RequestAssetThumb(lib.ID, fileID, "甲.pptx", 1, 1); err != nil {
		t.Fatalf("RequestAssetThumb: %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("按需导出未开始")
	}
	if !svc.CancelAssetThumbs(lib.ID) {
		t.Fatal("按需任务应可被库级取消")
	}
	res := waitOnDemandDone(t, sink)
	if !res.Canceled {
		t.Fatalf("取消后 done 应收口为 Canceled, got %+v", res)
	}
	close(release)
}

// TestRecordPassiveKeepsLRU 被动补记账不刷新 LRU 新鲜度（每轮扫描批量 touch 会
// 抹平淘汰区分度）。
func TestRecordPassiveKeepsLRU(t *testing.T) {
	svc, store, lib, fileID, _ := setupInvalidationFixture(t)
	// 先写入一个「很久以前」的访问时间
	if _, err := store.db.Exec(
		"UPDATE thumb_cache SET last_access_ns = 12345 WHERE file_id = ? AND tier = 'small' AND page = 1", fileID,
	); err != nil {
		t.Fatalf("注入旧访问时间: %v", err)
	}
	svc.thumbLedger.recordPassive(lib.ID, fileID, "small", 1, 99)
	svc.thumbLedger.flushLibrary(lib.ID)

	var ns int64
	if err := store.db.QueryRow(
		"SELECT last_access_ns FROM thumb_cache WHERE file_id = ? AND tier = 'small' AND page = 1", fileID,
	).Scan(&ns); err != nil {
		t.Fatalf("查访问时间: %v", err)
	}
	if ns != 12345 {
		t.Fatalf("passive 记账不应推高 last_access_ns, got %d", ns)
	}
	var bytes int64
	if err := store.db.QueryRow(
		"SELECT bytes FROM thumb_cache WHERE file_id = ? AND tier = 'small' AND page = 1", fileID,
	).Scan(&bytes); err != nil {
		t.Fatalf("查字节: %v", err)
	}
	if bytes != 99 {
		t.Fatalf("bytes 应被更新为 99, got %d", bytes)
	}
}
