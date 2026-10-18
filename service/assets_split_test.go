package service

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"workbench/model"
)

// ---------- 素材拆分链路测试（assets_split.go） ----------
//
// fixture 走 buildAssetsPptx 程序化构造（禁依赖本机 Office）；取消用预取消 ctx
// 与阻塞型注入 fake 核心驱动（禁 sleep 时序，docs/spec/test-stability.md）。

// assertValidZip 断言产物为合法 zip（拆分产物可被标准库打开 = 结构有效的最低门槛）。
func assertValidZip(t *testing.T, path string) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("产物应为合法 zip（%s）: %v", path, err)
	}
	if err := zr.Close(); err != nil {
		t.Fatalf("关闭产物 zip: %v", err)
	}
}

// mustScanLibrary 扫描入库断言辅助（拆分前置：files.hash 增量链路是拆分判定源）。
func mustScanLibrary(t *testing.T, svc *AssetLibraryService, libraryID string) model.AssetScanResult {
	t.Helper()
	result, err := svc.StartScan(context.Background(), libraryID)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	return result
}

// blockingSplitCore 阻塞型拆分核心 fake：记录调用并在首个调用通告后阻塞至
// release 关闭（在途拆分中间态——并发互斥/取消链路用例），放行后按 ctx 状态
// 返回取消错误或成功结果。
type blockingSplitCore struct {
	mu      sync.Mutex
	calls   []string
	started chan struct{}
	release chan struct{}
}

func (f *blockingSplitCore) split(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, srcPath)
	f.mu.Unlock()
	select {
	case <-f.started: // 已关闭：重复调用直接走后续
	default:
		close(f.started)
	}
	<-f.release
	if err := ctx.Err(); err != nil {
		return model.PptxSplitResult{}, fmt.Errorf("拆分已取消: %w", err)
	}
	return model.PptxSplitResult{OutputDir: filepath.Dir(srcPath), Files: []string{srcPath}, PageCount: 1}, nil
}

func (f *blockingSplitCore) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// ---------- schema v4 迁移与 store 方法 ----------

// TestAssetsStore_MigrateV4 迁移链：旧 v3 库升级后 split_records 建表且存量数据
// 保留、版本钉到 v4；新库直建即 v4。
func TestAssetsStore_MigrateV4(t *testing.T) {
	t.Run("旧库 v3 升级", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "v3.db")
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
		if err != nil {
			t.Fatalf("打开原始库: %v", err)
		}
		for _, v := range []int{1, 2, 3} {
			if _, err := db.Exec(assetsMigrations[v]); err != nil {
				t.Fatalf("手工执行 v%d 迁移: %v", v, err)
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 3"); err != nil {
			t.Fatalf("钉版本: %v", err)
		}
		if _, err := db.Exec(
			"INSERT INTO files(path, status, created_at, updated_at) VALUES('存量.pptx', 'ok', '2026-01-01 00:00:00', '2026-01-01 00:00:00')",
		); err != nil {
			t.Fatalf("插入存量数据: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("关闭原始库: %v", err)
		}

		store, err := openAssetsStore(dbPath)
		if err != nil {
			t.Fatalf("升级打开: %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })

		var n int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM split_records").Scan(&n); err != nil {
			t.Fatalf("split_records 表应已建: %v", err)
		}
		var files int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&files); err != nil {
			t.Fatalf("读存量数据: %v", err)
		}
		if files != 1 {
			t.Errorf("存量数据应保留，实际 %d 行", files)
		}
		var version int
		if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != assetsSchemaVersion {
			t.Errorf("版本应钉到 v%d，实际 v%d（err=%v）", assetsSchemaVersion, version, err)
		}
	})

	t.Run("新库直建 v4", func(t *testing.T) {
		store, err := openAssetsStore(filepath.Join(t.TempDir(), "fresh.db"))
		if err != nil {
			t.Fatalf("新库打开: %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })
		var n int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM split_records").Scan(&n); err != nil {
			t.Fatalf("新库应有 split_records 表: %v", err)
		}
	})
}

// TestAssetsStore_SplitRecordRoundTrip 拆分记录 store 方法：upsert 插入 → 读取 →
// 覆盖写 → 读取 → 删除 → 未命中（nil, nil）。
func TestAssetsStore_SplitRecordRoundTrip(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "记录库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)

	rec, err := store.getSplitRecord("a.pptx")
	if err != nil || rec != nil {
		t.Fatalf("未拆分应返回 (nil, nil)，实际 (%v, %v)", rec, err)
	}
	if err := store.upsertSplitRecord("a.pptx", "hash1", "dir1", 3, "2026-01-01 00:00:00"); err != nil {
		t.Fatalf("upsert 插入: %v", err)
	}
	rec, err = store.getSplitRecord("a.pptx")
	if err != nil || rec == nil {
		t.Fatalf("插入后应可读回: %v", err)
	}
	if rec.sourceRelPath != "a.pptx" || rec.sourceHash != "hash1" || rec.outputDir != "dir1" ||
		rec.pageCount != 3 || rec.splitAt != "2026-01-01 00:00:00" {
		t.Errorf("记录字段不符：%+v", rec)
	}
	// 覆盖写（重拆语义）
	if err := store.upsertSplitRecord("a.pptx", "hash2", "dir2", 5, "2026-01-02 00:00:00"); err != nil {
		t.Fatalf("upsert 覆盖: %v", err)
	}
	rec, _ = store.getSplitRecord("a.pptx")
	if rec == nil || rec.sourceHash != "hash2" || rec.outputDir != "dir2" || rec.pageCount != 5 {
		t.Errorf("覆盖写应生效：%+v", rec)
	}
	if err := store.deleteSplitRecord("a.pptx"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rec, err = store.getSplitRecord("a.pptx")
	if err != nil || rec != nil {
		t.Errorf("删除后应未命中，实际 (%v, %v)", rec, err)
	}
}

// ---------- 单文件拆分 ----------

// TestAssetSplit_SingleFileDerivedLayout 单文件拆分主链路：derived 镜像源树布局
// 断言（根目录文件 = SRS 字面 derived/{源名}/；子目录文件多一层镜像目录）、产物
// 合法 zip、源文件字节不变、拆分记录可查、progress/done 事件序列。
func TestAssetSplit_SingleFileDerivedLayout(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "拆分库", dir)

	srcRoot := buildAssetsPptx(t, dir, "材料.pptx", []assetsSlideSpec{
		{titlePh: "首页", body: []string{"内容"}},
		{titlePh: "次页", body: []string{"内容"}},
	})
	subDir := filepath.Join(dir, "子目录")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("建子目录: %v", err)
	}
	buildAssetsPptx(t, subDir, "汇报.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	orig, err := os.ReadFile(srcRoot)
	if err != nil {
		t.Fatalf("读源基准: %v", err)
	}
	mustScanLibrary(t, svc, lib.ID)
	// 事件出口在扫描后注入：只观察拆分事件（扫描 progress/done 不混入计数）
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "材料.pptx", false)
	if err != nil {
		t.Fatalf("SplitAssetFile: %v", err)
	}
	wantDir := filepath.Join(svc.assetsDir, lib.ID, "derived", "材料")
	if res.OutputDir != wantDir {
		t.Errorf("根目录文件落点应为 derived/{源名}/：%q, want %q", res.OutputDir, wantDir)
	}
	if res.PageCount != 2 || res.Skipped || len(res.Files) != 2 {
		t.Fatalf("拆分结果不符：%+v", res)
	}
	for i, want := range []string{
		filepath.Join(wantDir, "材料-001.pptx"),
		filepath.Join(wantDir, "材料-002.pptx"),
	} {
		if res.Files[i] != want {
			t.Errorf("产物命名应为 {源名}-%03d.pptx：%q, want %q", i+1, res.Files[i], want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Errorf("产物应落盘: %v", err)
		}
		assertValidZip(t, want)
	}
	// 子目录文件：镜像源树多一层目录，同名 pptx 撞目录结构性消除
	res2, err := svc.SplitAssetFile(context.Background(), lib.ID, "子目录/汇报.pptx", false)
	if err != nil {
		t.Fatalf("子目录文件拆分: %v", err)
	}
	wantDir2 := filepath.Join(svc.assetsDir, lib.ID, "derived", "子目录", "汇报")
	if res2.OutputDir != wantDir2 || res2.PageCount != 3 {
		t.Errorf("子目录文件落点应为 derived/子目录/汇报/（3 页）：%+v", res2)
	}
	for _, f := range res2.Files {
		assertValidZip(t, f)
	}

	// 源文件字节不变（索引模式红线）
	after, err := os.ReadFile(srcRoot)
	if err != nil {
		t.Fatalf("读拆分后源文件: %v", err)
	}
	if string(after) != string(orig) {
		t.Error("拆分不得修改源文件字节")
	}

	// 拆分记录可查（跳转方式 2 判定面）
	view, err := svc.GetAssetSplitRecord(lib.ID, "材料.pptx")
	if err != nil {
		t.Fatalf("GetAssetSplitRecord: %v", err)
	}
	if !view.Exists || view.PageCount != 2 || view.OutputDir != wantDir || view.SplitAt == "" || !view.UpToDate {
		t.Errorf("拆分记录视图不符：%+v", view)
	}
	// 未拆分文件：Exists=false 零值（relPath 相对形态清洗后按「不存在.pptx」查询）
	empty, err := svc.GetAssetSplitRecord(lib.ID, "子目录/../不存在.pptx")
	if err != nil {
		t.Fatalf("未拆分查询不应报错: %v", err)
	}
	if empty.Exists || empty.UpToDate {
		t.Errorf("未拆分文件应 Exists=false：%+v", empty)
	}

	// 事件序列（两次拆分共 9 条）：材料（2 页）= 2 页级 + 1 收尾 + done；汇报
	//（3 页）= 3 页级 + 1 收尾 + done。页级事件 done=已完成文件数（当前文件进行中），
	// 收尾事件 done=1（完成数口径）。
	type wantProgress struct {
		file      string
		done      int
		pageDone  int
		pageTotal int
	}
	wantSequence := []struct {
		name      string
		progress  wantProgress
		hasResult bool // done 事件
	}{
		{"assets-split:progress", wantProgress{"材料.pptx", 0, 1, 2}, false},
		{"assets-split:progress", wantProgress{"材料.pptx", 0, 2, 2}, false},
		{"assets-split:progress", wantProgress{"材料.pptx", 1, 2, 2}, false},
		{"assets-split:done", wantProgress{}, true},
		{"assets-split:progress", wantProgress{"子目录/汇报.pptx", 0, 1, 3}, false},
		{"assets-split:progress", wantProgress{"子目录/汇报.pptx", 0, 2, 3}, false},
		{"assets-split:progress", wantProgress{"子目录/汇报.pptx", 0, 3, 3}, false},
		{"assets-split:progress", wantProgress{"子目录/汇报.pptx", 1, 3, 3}, false},
		{"assets-split:done", wantProgress{}, true},
	}
	if len(sink.calls) != len(wantSequence) {
		t.Fatalf("应收到 %d 条事件，实际 %d 条：%#v", len(wantSequence), len(sink.calls), sink.calls)
	}
	for i, want := range wantSequence {
		call := sink.calls[i]
		if call.name != want.name {
			t.Fatalf("第 %d 条事件应为 %q，实际 %q", i, want.name, call.name)
		}
		if want.hasResult {
			payload, ok := call.data[0].(map[string]any)
			if !ok || payload["libraryId"] != lib.ID {
				t.Fatalf("第 %d 条 done 事件载荷不符：%#v", i, call.data)
			}
			batch, ok := payload["result"].(model.AssetSplitBatchResult)
			if !ok || batch.Total != 1 || batch.Split != 1 || batch.Failed != 0 {
				t.Errorf("第 %d 条 done 事件 result 应为单文件批量形状（total=1 split=1）：%#v", i, payload["result"])
			}
			continue
		}
		payload, ok := call.data[0].(model.AssetSplitProgress)
		if !ok || payload.LibraryID != lib.ID || payload.Done != want.progress.done ||
			payload.Total != 1 || payload.CurrentFile != want.progress.file ||
			payload.PageDone != want.progress.pageDone || payload.PageTotal != want.progress.pageTotal {
			t.Errorf("第 %d 条 progress 事件载荷不符（期望 %+v）：%#v", i, want.progress, call.data)
		}
	}
}

// TestAssetSplit_SourceDirLayout 源目录落点（outputToSourceDir=true）：产物落
// <源所在目录>/<源名>/（工具箱既有布局），源文件字节不变。
func TestAssetSplit_SourceDirLayout(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "源目录落点库", dir)
	src := buildAssetsPptx(t, dir, "演讲.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	// 源文件字节基准（拆分不得修改源文件）
	orig, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("读源基准: %v", err)
	}
	mustScanLibrary(t, svc, lib.ID)

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "演讲.pptx", true)
	if err != nil {
		t.Fatalf("SplitAssetFile: %v", err)
	}
	wantDir := filepath.Join(dir, "演讲")
	if res.OutputDir != wantDir || res.PageCount != 2 {
		t.Fatalf("源目录落点不符：%+v", res)
	}
	for _, f := range res.Files {
		assertValidZip(t, f)
	}
	// 源文件字节不变（索引模式红线）
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("读拆分后源文件: %v", err)
	}
	if string(after) != string(orig) {
		t.Error("拆分不得修改源文件字节")
	}
	// 落点在库根内：产物布局为拆分派生形态，扫描排除判定必须命中（防重复索引）
	if !isPptxSplitDerivative("演讲/演讲-001.pptx") {
		t.Error("源目录落点产物应被 isPptxSplitDerivative 排除")
	}
}

// TestAssetSplit_SameNameDiffDirsNoCollision 同库不同目录同名 pptx（PRD 验收：
// 镜像源树结构性消除撞目录）：同名文件分处 a/ 与 b/ 子目录，产物各落 derived/a/
// 同名/ 与 derived/b/同名/，页数互不串扰；拆分记录按 relPath 主键区分。
func TestAssetSplit_SameNameDiffDirsNoCollision(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同名库", dir)
	for _, sub := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatalf("建子目录 %s: %v", sub, err)
		}
	}
	buildAssetsPptx(t, filepath.Join(dir, "a"), "同名.pptx", []assetsSlideSpec{
		{titlePh: "甲", body: []string{"内容"}},
	})
	buildAssetsPptx(t, filepath.Join(dir, "b"), "同名.pptx", []assetsSlideSpec{
		{titlePh: "乙", body: []string{"内容"}},
		{titlePh: "丙", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	resA, err := svc.SplitAssetFile(context.Background(), lib.ID, "a/同名.pptx", false)
	if err != nil {
		t.Fatalf("拆分 a/同名.pptx: %v", err)
	}
	resB, err := svc.SplitAssetFile(context.Background(), lib.ID, "b/同名.pptx", false)
	if err != nil {
		t.Fatalf("拆分 b/同名.pptx: %v", err)
	}
	if resA.OutputDir == resB.OutputDir {
		t.Fatalf("同名文件产物目录应镜像源树隔离，实际同为 %q", resA.OutputDir)
	}
	if resA.PageCount != 1 || resB.PageCount != 2 {
		t.Errorf("同名文件页数应互不串扰：a=%d b=%d", resA.PageCount, resB.PageCount)
	}
	for _, f := range append(append([]string{}, resA.Files...), resB.Files...) {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("产物应落盘（%s）: %v", f, err)
		}
		assertValidZip(t, f)
	}
	viewA, err := svc.GetAssetSplitRecord(lib.ID, "a/同名.pptx")
	if err != nil || !viewA.Exists || viewA.PageCount != 1 {
		t.Errorf("a/同名.pptx 记录应为 1 页：%+v (err=%v)", viewA, err)
	}
	viewB, err := svc.GetAssetSplitRecord(lib.ID, "b/同名.pptx")
	if err != nil || !viewB.Exists || viewB.PageCount != 2 {
		t.Errorf("b/同名.pptx 记录应为 2 页（记录按 relPath 主键区分）：%+v (err=%v)", viewB, err)
	}
}

// ---------- 增量拆分 ----------

// TestAssetSplit_IncrementalSkip 已拆 + 哈希同 → 秒回「已最新」跳过不重拆
// （注入失败型核心：增量命中时核心不得被调用——确定性断言而非产物比对）。
func TestAssetSplit_IncrementalSkip(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "增量库", dir)
	buildAssetsPptx(t, dir, "已拆.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	first, err := svc.SplitAssetFile(context.Background(), lib.ID, "已拆.pptx", false)
	if err != nil {
		t.Fatalf("首次拆分: %v", err)
	}
	if first.Skipped {
		t.Fatal("首次拆分不应跳过")
	}

	// 注入失败型核心：增量命中不得重拆（重拆即测试失败）
	svc.splitCoreFn = func(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
		t.Error("增量命中不应调用拆分核心")
		return model.PptxSplitResult{}, errors.New("不应重拆")
	}
	second, err := svc.SplitAssetFile(context.Background(), lib.ID, "已拆.pptx", false)
	if err != nil {
		t.Fatalf("二次拆分: %v", err)
	}
	if !second.Skipped || second.SkipReason == "" {
		t.Errorf("二次拆分应命中增量跳过（Skipped=true 且带原因）：%+v", second)
	}
	if second.OutputDir != first.OutputDir || second.PageCount != first.PageCount {
		t.Errorf("跳过结果应回显已拆记录：%+v", second)
	}
}

// TestAssetSplit_HashChangedResplitsCleansOld 哈希变 → 重拆且旧产物目录清空：
// 页数减少场景无残留 + 手工放置的旧命名产物（%02d/越界页）一并收敛。哈希变更
// 经「源文件重写 + 库存 hash 同步更新」驱动（对齐重扫后的状态，不依赖 mtime
// 时序，docs/spec/test-stability.md）。
func TestAssetSplit_HashChangedResplitsCleansOld(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重拆库", dir)
	src := buildAssetsPptx(t, dir, "演变.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "演变.pptx", false); err != nil {
		t.Fatalf("首次拆分: %v", err)
	}
	// 手工放置旧命名/越界产物（模拟 %02d 时代残留与越界页）
	outDir := filepath.Join(svc.assetsDir, lib.ID, "derived", "演变")
	for _, junk := range []string{"演变-042.pptx", "演变-99.pptx"} {
		if err := os.WriteFile(filepath.Join(outDir, junk), []byte("junk"), 0644); err != nil {
			t.Fatalf("放置残留产物: %v", err)
		}
	}

	// 源文件变为 2 页 + 库存 hash 同步更新（确定性驱动哈希失配）
	buildAssetsPptx(t, dir, "演变.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"内容"}},
		{titlePh: "新二", body: []string{"内容"}},
	})
	newHash, err := hashFileSHA256(src)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, "演变.pptx"); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "演变.pptx", false)
	if err != nil {
		t.Fatalf("重拆: %v", err)
	}
	if res.Skipped {
		t.Fatalf("哈希变应重拆而非跳过：%+v", res)
	}
	if res.PageCount != 2 || len(res.Files) != 2 {
		t.Fatalf("重拆应产出 2 页：%+v", res)
	}
	// 旧产物（3 页产物 + 手工残留）全部收敛，目录内恰为 2 个新产物
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("读产物目录: %v", err)
	}
	got := make(map[string]bool, len(entries))
	for _, e := range entries {
		got[e.Name()] = true
	}
	want := map[string]bool{"演变-001.pptx": true, "演变-002.pptx": true}
	for name := range got {
		if !want[name] {
			t.Errorf("重拆后应无残留产物：%q", name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("重拆产物缺失：%q", name)
		}
	}
	// 记录更新为新页数且 UpToDate
	view, err := svc.GetAssetSplitRecord(lib.ID, "演变.pptx")
	if err != nil || !view.Exists || view.PageCount != 2 || !view.UpToDate {
		t.Errorf("重拆后记录应为 2 页且 UpToDate：%+v (err=%v)", view, err)
	}
}

// TestAssetSplit_SourceDirResplitKeepsForeignFiles 源目录落点重拆清旧页的破坏面
// 收口：落点目录与用户自建内容共享，重拆仅删产物命名形态（<源名主干>-<纯数字>.
// pptx）文件——用户自建文件（说明.txt）与同名非产物文件（<源名>-草稿.pptx，
// 页序段非数字）必须保留；旧页产物（页数减少的越界页）照常收敛。derived 落点
// 为应用自属目录不受影响（保持整目录清空，TestAssetSplit_HashChangedResplits-
// CleansOld 已覆盖）。
func TestAssetSplit_SourceDirResplitKeepsForeignFiles(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "源目录重拆库", dir)
	src := buildAssetsPptx(t, dir, "讲义.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "讲义.pptx", true); err != nil {
		t.Fatalf("首次拆分: %v", err)
	}
	outDir := filepath.Join(dir, "讲义")
	// 用户自建内容：普通文件 + 文件名主干相同但页序段非数字的同名异构 pptx
	note := filepath.Join(outDir, "说明.txt")
	draft := filepath.Join(outDir, "讲义-草稿.pptx")
	if err := os.WriteFile(note, []byte("用户笔记，拆分不得删除"), 0644); err != nil {
		t.Fatalf("放置用户文件: %v", err)
	}
	if err := os.WriteFile(draft, []byte("draft"), 0644); err != nil {
		t.Fatalf("放置草稿文件: %v", err)
	}

	// 源文件 3 页 → 2 页 + 库存 hash 同步更新（确定性驱动重拆，不依赖 mtime 时序）
	buildAssetsPptx(t, dir, "讲义.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"内容"}},
		{titlePh: "新二", body: []string{"内容"}},
	})
	newHash, err := hashFileSHA256(src)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, "讲义.pptx"); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "讲义.pptx", true)
	if err != nil {
		t.Fatalf("源目录落点重拆: %v", err)
	}
	if res.Skipped || res.PageCount != 2 {
		t.Fatalf("重拆应产出 2 页：%+v", res)
	}
	// 旧页产物收敛（页数减少的 003 不残留），新产物在位
	if _, err := os.Stat(filepath.Join(outDir, "讲义-003.pptx")); !os.IsNotExist(err) {
		t.Errorf("页数减少后旧页产物应收敛，err=%v", err)
	}
	for _, page := range []string{"讲义-001.pptx", "讲义-002.pptx"} {
		if _, err := os.Stat(filepath.Join(outDir, page)); err != nil {
			t.Errorf("新产物 %s 应存在: %v", page, err)
		}
	}
	// 用户自建内容原样保留（P1 修复面：逐文件删除不误伤）
	gotNote, err := os.ReadFile(note)
	if err != nil || string(gotNote) != "用户笔记，拆分不得删除" {
		t.Errorf("用户自建文件应原样保留（err=%v, content=%q）", err, gotNote)
	}
	if got, err := os.ReadFile(draft); err != nil || string(got) != "draft" {
		t.Errorf("同名非产物文件（页序段非数字）应原样保留（err=%v, content=%q）", err, got)
	}
}

// ---------- 库级批量 ----------

// TestAssetSplit_BatchIsolation 批量异常隔离：ok 快照 3 文件其一在扫描后损坏 →
// Failed=1 不中断其余拆分，无 error 事件（单文件失败走结果统计）；二次批量全跳过
// 路径；空库批量零事件形态。
func TestAssetSplit_BatchIsolation(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "批量库", dir)

	buildAssetsPptx(t, dir, "正常一.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	buildAssetsPptx(t, dir, "正常二.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
	})
	corruptPath := filepath.Join(dir, "坏损.pptx")
	buildAssetsPptx(t, dir, "坏损.pptx", []assetsSlideSpec{{titlePh: "一", body: []string{"内容"}}})
	mustScanLibrary(t, svc, lib.ID)
	// 事件出口在扫描后注入：只观察拆分事件
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)
	// 扫描后损坏（ok 快照圈定后文件变坏——拆分期 zip 打开失败的隔离形态）
	if err := os.WriteFile(corruptPath, []byte("not a zip at all"), 0644); err != nil {
		t.Fatalf("损坏样本: %v", err)
	}

	batch, err := svc.SplitAssetLibrary(context.Background(), lib.ID, false)
	if err != nil {
		t.Fatalf("批量拆分不应整体失败: %v", err)
	}
	if batch.Total != 3 || batch.Split != 2 || batch.Skipped != 0 || batch.Failed != 1 {
		t.Fatalf("批量汇总不符（total=3 split=2 failed=1）：%+v", batch)
	}
	byPath := make(map[string]model.AssetSplitFileOutcome, len(batch.Items))
	for _, item := range batch.Items {
		byPath[item.RelPath] = item
	}
	if item := byPath["坏损.pptx"]; item.Status != model.AssetSplitStatusFailed || item.Message == "" {
		t.Errorf("坏损文件应为 failed 且带原因：%+v", item)
	}
	for _, name := range []string{"正常一.pptx", "正常二.pptx"} {
		if item := byPath[name]; item.Status != model.AssetSplitStatusSplit {
			t.Errorf("文件 %s 应拆分成功：%+v", name, item)
		}
	}
	for _, call := range sink.calls {
		if call.name == "assets-split:error" {
			t.Errorf("单文件失败不应发 error 事件（走结果统计），实际：%#v", call)
		}
	}

	// 二次批量：已拆 2 文件增量跳过，坏损仍失败
	batch2, err := svc.SplitAssetLibrary(context.Background(), lib.ID, false)
	if err != nil {
		t.Fatalf("二次批量: %v", err)
	}
	if batch2.Split != 0 || batch2.Skipped != 2 || batch2.Failed != 1 {
		t.Errorf("二次批量应为全跳过路径（skipped=2 failed=1）：%+v", batch2)
	}

	// 空库批量：零文件零失败，仅 done 终态
	emptyLib := mustCreateLibrary(t, svc, "空批量库", t.TempDir())
	emptyBatch, err := svc.SplitAssetLibrary(context.Background(), emptyLib.ID, false)
	if err != nil {
		t.Fatalf("空库批量: %v", err)
	}
	if emptyBatch.Total != 0 {
		t.Errorf("空库批量 total 应为 0：%+v", emptyBatch)
	}
}

// TestAssetSplit_CancelPreCancelled 预取消 ctx：批量与单文件均立即返回
// E_ASSETS_CANCELLED 并发 error 事件；defer 注销后注册表无残留。
func TestAssetSplit_CancelPreCancelled(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "预取消拆分库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanLibrary(t, svc, lib.ID)
	// 事件出口在扫描后注入：只观察拆分事件
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.SplitAssetLibrary(ctx, lib.ID, false)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
		t.Fatalf("批量取消应映射为 E_ASSETS_CANCELLED，实际：%v", err)
	}
	_, err = svc.SplitAssetFile(ctx, lib.ID, "内容.pptx", false)
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
		t.Fatalf("单文件取消应映射为 E_ASSETS_CANCELLED，实际：%v", err)
	}
	// 2 条 error 事件（批量 + 单文件各一条），message 为用户可读取消文案
	if len(sink.calls) != 2 {
		t.Fatalf("应收到 2 条 error 事件，实际：%#v", sink.calls)
	}
	for i, call := range sink.calls {
		if call.name != "assets-split:error" {
			t.Fatalf("第 %d 条事件应为 assets-split:error，实际 %q", i, call.name)
		}
		payload, ok := call.data[0].(map[string]any)
		if !ok || payload["libraryId"] != lib.ID || payload["message"] != "拆分已取消" {
			t.Errorf("error 事件载荷应为用户可读消息，实际：%#v", call.data)
		}
	}
	if svc.CancelAssetSplit(lib.ID) {
		t.Error("拆分返回后注册表应无残留")
	}
}

// TestAssetSplit_InFlightMutexAndCancel 在途拆分互斥与取消：阻塞型核心 fake 保持
// 批量在途 → 单文件拆分同库被拒（互斥）→ CancelAssetSplit 命中 → 批量在检查点
// 返回取消错误，剩余文件不再处理，注册表无残留。
func TestAssetSplit_InFlightMutexAndCancel(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "在途拆分库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanLibrary(t, svc, lib.ID)
	// 事件出口在扫描后注入：只观察拆分事件
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)

	blocker := &blockingSplitCore{started: make(chan struct{}), release: make(chan struct{})}
	svc.splitCoreFn = blocker.split

	batchDone := make(chan error, 1)
	go func() {
		_, err := svc.SplitAssetLibrary(context.Background(), lib.ID, false)
		batchDone <- err
	}()
	<-blocker.started

	// 在途期间同库单文件拆分被拒（拆分互斥，独立于扫描锁）
	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "甲.pptx", false)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsScanInProgress {
		t.Fatalf("同库并发拆分应被拒，实际：%v", err)
	}
	// 在途期间删库同样被拒（DeleteLibrary 复用拆分锁互斥）
	if err := svc.DeleteLibrary(lib.ID, false); !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsScanInProgress {
		t.Fatalf("拆分中删库应被拒，实际：%v", err)
	}

	if !svc.CancelAssetSplit(lib.ID) {
		t.Fatal("在途拆分应可取消")
	}
	close(blocker.release)
	select {
	case err := <-batchDone:
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
			t.Fatalf("取消错误应映射为 E_ASSETS_CANCELLED，实际：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消后批量未返回")
	}
	if got := blocker.callCount(); got != 1 {
		t.Errorf("取消后剩余文件不应再拆分，核心实际被调 %d 次", got)
	}
	if svc.CancelAssetSplit(lib.ID) {
		t.Error("批量返回后注册表应无残留")
	}
	// 取消错误应发 error 事件（整体失败口径）
	found := false
	for _, call := range sink.calls {
		if call.name == "assets-split:error" {
			found = true
		}
	}
	if !found {
		t.Error("取消应发 assets-split:error 事件")
	}
}

// TestAssetLibrary_ShutdownCancelsInFlightSplit 关停收口链（CancelAllSplits →
// WaitSplitsDone）：在途拆分（阻塞型核心 fake）被取消且 WaitSplitsDone 等待拆分
// 退出后才返回——返回顺序以共享序录断言（batch 先于 wait；无拆分计数时 Wait 会
// 先返回，顺序翻转即暴露）；拆分返回 E_ASSETS_CANCELLED、注册表无残留；无在途
// 拆分时 WaitSplitsDone 立即返回。chan 交接驱动，禁 sleep 时序。
func TestAssetLibrary_ShutdownCancelsInFlightSplit(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "关停拆分库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanLibrary(t, svc, lib.ID)

	blocker := &blockingSplitCore{started: make(chan struct{}), release: make(chan struct{})}
	svc.splitCoreFn = blocker.split

	var orderMu sync.Mutex
	var order []string
	record := func(stage string) {
		orderMu.Lock()
		order = append(order, stage)
		orderMu.Unlock()
	}

	batchDone := make(chan error, 1)
	go func() {
		_, err := svc.SplitAssetLibrary(context.Background(), lib.ID, false)
		record("batch")
		batchDone <- err
	}()
	<-blocker.started

	waitDone := make(chan struct{})
	go func() {
		svc.CancelAllSplits()
		svc.WaitSplitsDone()
		record("wait")
		close(waitDone)
	}()

	// 放行阻塞：核心在 ctx 取消后返回取消错误 → 批量中止退出 → splitWG 归零 →
	// WaitSplitsDone 返回。返回顺序必须 batch 先于 wait（等待真实 join 在途拆分）
	close(blocker.release)
	var appErr *model.AppError
	select {
	case err := <-batchDone:
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
			t.Fatalf("关停取消应映射为 E_ASSETS_CANCELLED，实际：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("关停取消后批量未返回")
	}
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("CancelAllSplits 后 WaitSplitsDone 未返回")
	}
	orderMu.Lock()
	gotOrder := append([]string(nil), order...)
	orderMu.Unlock()
	if len(gotOrder) != 2 || gotOrder[0] != "batch" || gotOrder[1] != "wait" {
		t.Errorf("WaitSplitsDone 应等待拆分退出后返回（顺序 [batch wait]），实际 %v", gotOrder)
	}
	if svc.CancelAssetSplit(lib.ID) {
		t.Error("关停收口后注册表应无残留")
	}

	// 无在途拆分：WaitSplitsDone 立即返回（计数归零不阻塞）
	immediate := make(chan struct{})
	go func() {
		svc.WaitSplitsDone()
		close(immediate)
	}()
	select {
	case <-immediate:
	case <-time.After(2 * time.Second):
		t.Fatal("无在途拆分时 WaitSplitsDone 未立即返回")
	}
}

// ---------- 越界遏制与打开方式 2 ----------

// TestAssetSplit_RelPathEscapeRejected relPath 越界遏制：`..` 逃逸与绝对路径在
// 三个入口（拆分/查记录/打开单页）一致拒绝（浏览器通道可达，对齐 RetryAssetThumb）。
func TestAssetSplit_RelPathEscapeRejected(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "越界库", dir)
	buildAssetsPptx(t, dir, "材料.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanLibrary(t, svc, lib.ID)

	ctx := context.Background()
	if _, err := svc.SplitAssetFile(ctx, lib.ID, "../外部.pptx", false); err == nil {
		t.Error("`..` 逃逸拆分应被拒绝")
	}
	if _, err := svc.SplitAssetFile(ctx, lib.ID, filepath.Join(dir, "材料.pptx"), false); err == nil {
		t.Error("绝对路径拆分应被拒绝")
	}
	if _, err := svc.SplitAssetFile(ctx, lib.ID, "", false); err == nil {
		t.Error("空相对路径应被拒绝")
	}
	if _, err := svc.GetAssetSplitRecord(lib.ID, "../x.pptx"); err == nil {
		t.Error("`..` 逃逸查记录应被拒绝")
	}
	if err := svc.OpenAssetSplitPage(lib.ID, "../x.pptx", 1); err == nil {
		t.Error("`..` 逃逸打开单页应被拒绝")
	}
}

// TestAssetSplit_OpenPage 打开拆分单页（方式 2）：无记录报「尚未拆分」、页序
// 越界报错、正常路径打开注入 fake 收到产物首页绝对路径、产物被清后报存在性错误。
func TestAssetSplit_OpenPage(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "打开单页库", dir)
	buildAssetsPptx(t, dir, "双页.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	buildAssetsPptx(t, dir, "未拆.pptx", []assetsSlideSpec{{titlePh: "一", body: []string{"内容"}}})
	mustScanLibrary(t, svc, lib.ID)

	// 注入 fake 打开动作（隔离真实系统调用）
	var opened []string
	svc.openSplitPageFn = func(p string) error {
		opened = append(opened, p)
		return nil
	}

	// 无记录：尚未拆分
	if err := svc.OpenAssetSplitPage(lib.ID, "未拆.pptx", 1); err == nil || !strings.Contains(err.Error(), "尚未拆分") {
		t.Errorf("无记录应报「尚未拆分」，实际：%v", err)
	}

	// 正常路径：先拆分再打开
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "双页.pptx", false)
	if err != nil {
		t.Fatalf("拆分: %v", err)
	}
	// 页序非法/越界（有记录后越界校验才可达）
	if err := svc.OpenAssetSplitPage(lib.ID, "双页.pptx", 0); err == nil {
		t.Error("页序 0 应被拒绝")
	}
	if err := svc.OpenAssetSplitPage(lib.ID, "双页.pptx", 3); err == nil || !strings.Contains(err.Error(), "超出已拆分页数") {
		t.Errorf("页序越界应报「超出已拆分页数」，实际：%v", err)
	}
	if err := svc.OpenAssetSplitPage(lib.ID, "双页.pptx", 1); err != nil {
		t.Fatalf("打开单页: %v", err)
	}
	wantPath := filepath.Join(res.OutputDir, "双页-001.pptx")
	if len(opened) != 1 || opened[0] != wantPath {
		t.Errorf("打开动作应收产物首页绝对路径：%v, want %q", opened, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("产物首页应存在: %v", err)
	}

	// 产物被手动清理：存在性校验兜住（对齐「PNG 存在唯一权威」先例的反向面）
	if err := os.Remove(wantPath); err != nil {
		t.Fatalf("清理产物: %v", err)
	}
	if err := svc.OpenAssetSplitPage(lib.ID, "双页.pptx", 1); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Errorf("产物缺失应报存在性错误，实际：%v", err)
	}
}

// ---------- 库删除联动 ----------

// TestAssetLibrary_DeleteLibraryRemovesDerivedDir 删库联动：derived 拆分产物目录
// 随库删除清理（与 thumbs 目录同口径）。
func TestAssetLibrary_DeleteLibraryRemovesDerivedDir(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "删库联动库", dir)
	buildAssetsPptx(t, dir, "材料.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "材料.pptx", false); err != nil {
		t.Fatalf("拆分: %v", err)
	}
	derivedDir := filepath.Join(svc.assetsDir, lib.ID, "derived")
	if _, err := os.Stat(derivedDir); err != nil {
		t.Fatalf("拆分产物目录应存在: %v", err)
	}
	if err := svc.DeleteLibrary(lib.ID, false); err != nil {
		t.Fatalf("DeleteLibrary: %v", err)
	}
	if _, err := os.Stat(derivedDir); !os.IsNotExist(err) {
		t.Errorf("删库后 derived 目录应已删除，err=%v", err)
	}
}
