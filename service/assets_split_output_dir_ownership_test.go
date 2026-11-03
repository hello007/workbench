package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"workbench/model"
)

// 拆分落点目录归属校验（11-02-split-output-dir-ownership）用例：
//   - A 拆分后改名重链为 B（记录 output_dir 冻结 A 目录），新建 A.pptx 再拆分
//     必须被拒（E_ASSETS_SPLIT_OUTPUT_CONFLICT），B 的产物与索引行零改动；
//   - 同一源重复拆分不受影响（记录键相同，归属查询排除）；
//   - splitArtifactPath 页序反查命中但主干不属于记录候选时不采纳。

// filesRowForTest 白盒读取索引行关键字段（幂等断言用）。
func filesRowForTest(t *testing.T, svc *AssetLibraryService, libraryID, rel string) (id, size, mtimeNs int64, status, hash string) {
	t.Helper()
	err := svc.openStoreForTest(t, libraryID).db.QueryRow(
		"SELECT id, size, mtime_ns, status, hash FROM files WHERE path = ?", rel,
	).Scan(&id, &size, &mtimeNs, &status, &hash)
	if err != nil {
		t.Fatalf("读取索引行（%s）: %v", rel, err)
	}
	return id, size, mtimeNs, status, hash
}

// TestAssetSplit_OutputDirOwnershipConflict 可达数据损坏场景：A.pptx 拆分（source
// 落点 → <root>/A）后改名为 B.pptx 并重链（记录键迁到 B、output_dir 仍冻结 A
// 目录、产物主干仍为 A），随后放入新的 A.pptx 拆分——新 A 的落点恰是同一 A
// 目录，旧实现会直接清空并覆盖 B 的产物。修复后必须在删除动作之前拒绝，且
// 不触碰 B 的产物文件与索引行。
func TestAssetSplit_OutputDirOwnershipConflict(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "落点归属冲突库", root)
	buildAssetsPptx(t, root, "A.pptx", []assetsSlideSpec{
		{titlePh: "原一", body: []string{"原首拆内容"}},
		{titlePh: "原二", body: []string{"原末页内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "A.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "A.pptx", model.AssetSplitTargetSource); err != nil {
		t.Fatalf("首次拆分 A: %v", err)
	}

	outDir := filepath.Join(root, "A")
	page1 := filepath.Join(outDir, "A-001.pptx")
	page2 := filepath.Join(outDir, "A-002.pptx")
	mustExist(t, page1)
	mustExist(t, page2)
	beforePage1, err := os.ReadFile(page1)
	if err != nil {
		t.Fatalf("读原产物: %v", err)
	}

	// 改名 A → B 并重链：记录键迁移、output_dir 冻结 A 目录
	bAbs := filepath.Join(root, "B.pptx")
	if err := os.Rename(filepath.Join(root, "A.pptx"), bAbs); err != nil {
		t.Fatalf("改名源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, bAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "B.pptx")
	if rec == nil || !samePathFold(rec.outputDir, outDir) || rec.artifactStem != "A" {
		t.Fatalf("前置：记录键应为 B 且 output_dir/主干冻结为 A，实际 %+v", rec)
	}
	bID, bSize, bMtime, bStatus, bHash := filesRowForTest(t, svc, lib.ID, "B.pptx")

	// 新的 A.pptx（内容刻意不同，若发生覆盖可被字节比较识别）
	buildAssetsPptx(t, root, "A.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"这是新的同名源文件"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	_, err = svc.SplitAssetFile(context.Background(), lib.ID, "A.pptx", model.AssetSplitTargetSource)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsSplitOutputConflict {
		t.Fatalf("落点被另一源文件占用应返回 %s，实际：%v", model.ErrCodeAssetsSplitOutputConflict, err)
	}
	if !strings.Contains(appErr.Message, "B.pptx") || !strings.Contains(appErr.Message, "占用") {
		t.Errorf("错误文案应指明占用源文件与处理方式，实际：%s", appErr.Message)
	}

	// B 的产物零改动（未被删除、未被覆盖）
	afterPage1, err := os.ReadFile(page1)
	if err != nil {
		t.Fatalf("冲突后原产物应保留: %v", err)
	}
	if !bytes.Equal(beforePage1, afterPage1) {
		t.Errorf("冲突后 A-001.pptx 内容被改动（说明发生了删除/覆盖）")
	}
	mustExist(t, page2)

	// B 的索引行零改动
	id, size, mtimeNs, status, hash := filesRowForTest(t, svc, lib.ID, "B.pptx")
	if id != bID || size != bSize || mtimeNs != bMtime || status != bStatus || hash != bHash {
		t.Errorf("冲突后 B.pptx 索引行被改动：before(id=%d size=%d mtime=%d status=%s hash=%s) after(id=%d size=%d mtime=%d status=%s hash=%s)",
			bID, bSize, bMtime, bStatus, bHash, id, size, mtimeNs, status, hash)
	}
	// B 的拆分记录仍在
	rec = splitRecordRowForTest(t, svc, lib.ID, "B.pptx")
	if rec == nil || !samePathFold(rec.outputDir, outDir) {
		t.Errorf("冲突后 B 的拆分记录应原样保留，实际 %+v", rec)
	}
	// 新 A 自身未产生拆分记录
	if aRec := splitRecordRowForTest(t, svc, lib.ID, "A.pptx"); aRec != nil {
		t.Errorf("拒绝拆分后不应写入 A 的拆分记录，实际 %+v", aRec)
	}
}

// TestAssetSplit_OutputDirOwnershipSameSourceAllowed 同一源重复拆分不被归属校验
// 误拒：删除首页产物驱动重拆（增量判定首页存在性落空），记录键与当前源相同，
// 归属查询以 source_rel_path <> rel 排除，重拆正常完成并重建全部页。
func TestAssetSplit_OutputDirOwnershipSameSourceAllowed(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同源重拆放行库", root)
	buildAssetsPptx(t, root, "同源.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	first, err := svc.SplitAssetFile(context.Background(), lib.ID, "同源.pptx", model.AssetSplitTargetSource)
	if err != nil {
		t.Fatalf("首次拆分: %v", err)
	}
	if first.Skipped || first.PageCount != 2 {
		t.Fatalf("首次拆分应产出 2 页：%+v", first)
	}

	outDir := filepath.Join(root, "同源")
	page1 := filepath.Join(outDir, "同源-001.pptx")
	// 删除首页产物驱动重拆（增量判定要求首页产物存在）
	if err := os.Remove(page1); err != nil {
		t.Fatalf("删除首页产物驱动重拆: %v", err)
	}

	second, err := svc.SplitAssetFile(context.Background(), lib.ID, "同源.pptx", model.AssetSplitTargetSource)
	if err != nil {
		t.Fatalf("同源重拆不应被归属校验拒绝: %v", err)
	}
	if second.Skipped || second.PageCount != 2 {
		t.Fatalf("同源重拆应完成 2 页：%+v", second)
	}
	for _, page := range []int{1, 2} {
		p := filepath.Join(outDir, fmt.Sprintf("同源-%03d.pptx", page))
		mustExist(t, p)
		assertValidZip(t, p)
	}
}

// TestAssetSplitReplace_OutputDirOwnershipConflict replace 落点归属复核（P2）：目录
// 被另一源文件的 replace 记录占用（典型为改名重链后记录 output_dir 冻结旧目录，
// 且该目录内产物已缺失）时，同目录新源的就地替换必须被拒绝——旧实现仅凭
// ownsOutDir（本记录拥有目录）与 firstReplaceArtifactName（同形文件）判断，二者
// 都不成立即放行，会覆盖/删除另一记录的产物。要求：拒绝且目录、源文件、索引行与
// 记录集合零改动，核心拆分不被调用。
func TestAssetSplitReplace_OutputDirOwnershipConflict(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "replace 落点归属冲突库", root)

	// B.pptx 的 replace 记录指向 A 目录（改名重链后落点冻结的形态），目录内仅有
	// 用户自建内容（产物已缺失——正是旧检查无法发现归属的形态）。
	buildAssetsPptx(t, root, "B.pptx", []assetsSlideSpec{{titlePh: "一", body: []string{"B 内容"}}})
	mustScanLibrary(t, svc, lib.ID)
	outDir := filepath.Join(root, "A")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("建占位目录: %v", err)
	}
	keep := filepath.Join(outDir, "keep.txt")
	if err := os.WriteFile(keep, []byte("B-product"), 0644); err != nil {
		t.Fatalf("写占位文件: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("B.pptx", "hash-b", outDir, 1,
		"2026-11-02 10:00:00", model.AssetSplitTargetReplace, "B", true, nil, nil); err != nil {
		t.Fatalf("写入 B 的 replace 记录: %v", err)
	}

	// 新的同名源 A.pptx（无自身记录），其 replace 落点恰为同一 A 目录
	buildAssetsPptx(t, root, "A.pptx", []assetsSlideSpec{{titlePh: "新", body: []string{"新内容"}}})
	mustScanLibrary(t, svc, lib.ID)
	aID, aSize, aMtime, aStatus, aHash := filesRowForTest(t, svc, lib.ID, "A.pptx")

	coreCalled := false
	svc.splitCoreFn = func(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
		coreCalled = true
		return model.PptxSplitResult{}, errors.New("冲突时不应进入拆分核心")
	}
	svc.moveToRecycleBinFn = func(path string) error {
		t.Errorf("冲突时不应移动源文件: %s", path)
		return nil
	}

	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "A.pptx", model.AssetSplitTargetReplace)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsSplitOutputConflict {
		t.Fatalf("落点被另一源文件占用应返回 %s，实际：%v", model.ErrCodeAssetsSplitOutputConflict, err)
	}
	if !strings.Contains(appErr.Message, "B.pptx") || !strings.Contains(appErr.Message, "占用") {
		t.Errorf("错误文案应指明占用源文件与处理方式，实际：%s", appErr.Message)
	}
	if coreCalled {
		t.Error("归属复核必须先于拆分核心（任何写入/覆盖动作之前）")
	}

	// 目录零改动：占位文件内容不变、无新增条目
	if data, readErr := os.ReadFile(keep); readErr != nil || string(data) != "B-product" {
		t.Errorf("占位文件应原样保留，实际 data=%q err=%v", data, readErr)
	}
	entries, readErr := os.ReadDir(outDir)
	if readErr != nil || len(entries) != 1 {
		t.Errorf("目录不应新增/删除任何文件，实际 %d 项（err=%v）", len(entries), readErr)
	}

	// 源文件与索引行零改动，A 未写记录，B 记录原样保留
	if _, statErr := os.Stat(filepath.Join(root, "A.pptx")); statErr != nil {
		t.Errorf("源文件应原样保留: %v", statErr)
	}
	id, size, mtimeNs, status, hash := filesRowForTest(t, svc, lib.ID, "A.pptx")
	if id != aID || size != aSize || mtimeNs != aMtime || status != aStatus || hash != aHash {
		t.Errorf("冲突后 A.pptx 索引行被改动：before(id=%d size=%d mtime=%d status=%s hash=%s) after(id=%d size=%d mtime=%d status=%s hash=%s)",
			aID, aSize, aMtime, aStatus, aHash, id, size, mtimeNs, status, hash)
	}
	if aRec := splitRecordRowForTest(t, svc, lib.ID, "A.pptx"); aRec != nil {
		t.Errorf("拒绝后不应写入 A 的拆分记录，实际 %+v", aRec)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "B.pptx")
	if rec == nil || !samePathFold(rec.outputDir, outDir) || rec.pageCount != 1 ||
		rec.target != model.AssetSplitTargetReplace || rec.artifactStem != "B" {
		t.Errorf("冲突后 B 的拆分记录应原样保留，实际 %+v", rec)
	}
}

// TestSplitArtifactPath_StemMismatchNotAdopted 页序反查命中但主干不属于记录候选
// （目录内只有另一源文件的 `笔记-001.pptx`）：不采纳反查结果，返回直拼路径且
// 判定不存在——增量判定不会把他人的首页产物误当成本记录产物而跳过拆分。
func TestSplitArtifactPath_StemMismatchNotAdopted(t *testing.T) {
	outDir := t.TempDir()
	writeSplitArtifactForTest(t, filepath.Join(outDir, "笔记-001.pptx"))

	record := &assetSplitRecord{
		sourceRelPath: "旧名.pptx",
		outputDir:     outDir,
		pageCount:     1,
		target:        model.AssetSplitTargetSource,
		artifactStem:  "旧名",
	}
	got, ok := splitArtifactPath(record, "旧名.pptx", 1)
	if ok {
		t.Fatalf("主干不符的反查候选不应被采纳，实际命中 %q", got)
	}
	want := filepath.Join(outDir, "旧名-001.pptx")
	if !samePathFold(got, want) {
		t.Errorf("未采纳时应返回直拼路径 %q，实际 %q", want, got)
	}
}

// TestSplitRecordByOutputDir 归属查询单点语义：空表返回 (nil,nil)、命中另一源
// 文件、排除当前源键、落点不同不命中；同时守护 SELECT 列序与 Scan 对齐
// （source_rel_path/page_count/artifact_stem 等字段取错会在此暴露）。
func TestSplitRecordByOutputDir(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "归属查询库", root)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(root, "A")

	got, err := store.splitRecordByOutputDir(outDir, "A.pptx")
	if err != nil || got != nil {
		t.Fatalf("空表应返回 (nil,nil)，实际 (%+v,%v)", got, err)
	}

	if err := store.upsertSplitRecord("B.pptx", "hash-b", outDir, 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "A", false, nil, nil); err != nil {
		t.Fatalf("写入记录: %v", err)
	}
	got, err = store.splitRecordByOutputDir(outDir, "A.pptx")
	if err != nil || got == nil {
		t.Fatalf("应命中他源记录，实际 (%+v,%v)", got, err)
	}
	if got.sourceRelPath != "B.pptx" || got.sourceHash != "hash-b" || got.pageCount != 2 ||
		got.target != model.AssetSplitTargetSource || got.artifactStem != "A" || got.sourceRemoved {
		t.Errorf("命中记录字段与写入不符（列序/Scan 对齐？）：%+v", got)
	}

	got, err = store.splitRecordByOutputDir(outDir, "B.pptx")
	if err != nil || got != nil {
		t.Fatalf("同源记录应被排除，实际 (%+v,%v)", got, err)
	}
	got, err = store.splitRecordByOutputDir(filepath.Join(root, "C"), "A.pptx")
	if err != nil || got != nil {
		t.Fatalf("落点不同不应命中，实际 (%+v,%v)", got, err)
	}
}

// TestSplitRecordByOutputDir_SameSourceCaseFold 同源大小写折叠排除分支：SQL
// `source_rel_path <> ?` 大小写敏感，Windows 上 `A.pptx` / `a.pptx` 是同一物理
// 文件，Go 侧 samePathFold 必须把它排除（否则正常重拆被误判为「他源占用」）；
// Linux/macOS 大小写敏感，两键是不同源，应正常命中。
func TestSplitRecordByOutputDir_SameSourceCaseFold(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同源折叠排除库", root)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(root, "A")

	for _, rec := range []struct {
		rel  string
		hash string
	}{{"A.pptx", "hash-A"}, {"a.pptx", "hash-a"}} {
		if err := store.upsertSplitRecord(rec.rel, rec.hash, outDir, 1,
			"2026-11-02 10:00:00", model.AssetSplitTargetSource, "A", false, nil, nil); err != nil {
			t.Fatalf("写入记录 %s: %v", rec.rel, err)
		}
	}

	got, err := store.splitRecordByOutputDir(outDir, "A.pptx")
	if err != nil {
		t.Fatalf("归属查询: %v", err)
	}
	if runtime.GOOS == "windows" {
		if got != nil {
			t.Errorf("Windows 上仅大小写不同的同源记录应被折叠排除，实际命中 %+v", got)
		}
		return
	}
	if got == nil || got.sourceRelPath != "a.pptx" {
		t.Errorf("非 Windows 大小写不同视为不同源，应命中 a.pptx，实际 %+v", got)
	}
}

// TestAssetSplitLibrary_ItemCodeOutputDirConflict 批量入口：落点归属冲突按单文件
// 失败隔离（批量不整体失败），明细 Item.Code 必须透出 E_ASSETS_SPLIT_OUTPUT_CONFLICT，
// 供前端分类与提示；冲突文件零产出、源保留。
func TestAssetSplitLibrary_ItemCodeOutputDirConflict(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "批量落点冲突库", root)
	buildAssetsPptx(t, root, "A.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	// B 的拆分记录冻结在 A 目录（改名重链后落点不随源移动的形态）。
	outDir := filepath.Join(root, "A")
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("B.pptx", "hash-b", outDir, 1,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "A", false, nil, nil); err != nil {
		t.Fatalf("写入 B 的拆分记录: %v", err)
	}

	batch, err := svc.SplitAssetLibrary(context.Background(), lib.ID, model.AssetSplitTargetSource)
	if err != nil {
		t.Fatalf("单文件归属冲突不应使批量整体失败: %v", err)
	}
	if batch.Total != 1 || batch.Failed != 1 || batch.Split != 0 || batch.Skipped != 0 {
		t.Fatalf("批量汇总不符（total=1 failed=1）：%+v", batch)
	}
	if len(batch.Items) != 1 {
		t.Fatalf("明细应仅含 1 项，实际 %d 项：%+v", len(batch.Items), batch.Items)
	}
	item := batch.Items[0]
	if item.RelPath != "A.pptx" || item.Status != model.AssetSplitStatusFailed ||
		item.Code != model.ErrCodeAssetsSplitOutputConflict {
		t.Errorf("冲突项应为 failed + %s，实际 %+v", model.ErrCodeAssetsSplitOutputConflict, item)
	}
	mustNotExist(t, filepath.Join(outDir, "A-001.pptx"))
	mustExist(t, filepath.Join(root, "A.pptx"))
}

// TestAssetExport_SinglePptxPathUsesRecordStem 产物目录随源改名搬迁后目录基名与
// 记录主干不一致（目录 `新名`、产物 `旧名-001.pptx`）：导出 single_pptx_path 的
// 页序反查归属校验必须用记录持久化的 artifact_stem，而非目录基名启发式，否则该
// 场景导出恒空（打开单页走完整记录不受影响，两消费面不应分叉）。
func TestAssetExport_SinglePptxPathUsesRecordStem(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "新名")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("建产物目录: %v", err)
	}
	writeSplitArtifactForTest(t, filepath.Join(outDir, "旧名-001.pptx"))

	row := &assetExportRow{
		relPath: "新名.pptx", slideNum: 1, hasSplitRecord: true,
		splitOutputDir: outDir, splitPageCount: 1,
		splitTarget: model.AssetSplitTargetSource, splitArtifactStem: "旧名",
	}
	got := row.singlePptxPath(&assetExportCtx{lib: model.AssetLibrary{RootPath: t.TempDir()},
		pathMode: model.AssetExportPathModeAbsolute})
	want := filepath.Join(outDir, "旧名-001.pptx")
	if !samePathFold(got, want) {
		t.Errorf("导出 single_pptx_path 应为 %q，实际 %q", want, got)
	}
}
