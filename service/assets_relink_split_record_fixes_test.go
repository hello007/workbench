package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// 重链拆分记录口径修复（11-01-relink-split-record-fixes）单测：
//   - 改名重链后产物定位（打开单页 / 导出 / 增量判定 / replace 产物识别）仍有效；
//   - 目录级迁移不重算文件级重链冻结的 output_dir，常规落点重算行为不变；
//   - 清理丢失记录同事务删除拆分记录；
//   - force 重链（内容已变）删除失效拆分记录。

// assetSplitResultReader 便于断言：读取拆分结果中的 skip 态。
func mustSplitSkipped(t *testing.T, res model.AssetSplitResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("SplitAssetFile: %v", err)
	}
	if !res.Skipped {
		t.Fatalf("改名重链后增量判定应命中「已最新」跳过，实际 %+v", res)
	}
}

// TestRelocateAssetFile_RenameKeepsSplitArtifactReachable 改名 + 移动重链后：
// 记录键迁移、产物仍按旧主干定位——打开单页、导出 single_pptx_path、增量判定
// 三条消费面全部有效（修复前产品名按新主干拼接，三者同时失效）。
func TestRelocateAssetFile_RenameKeepsSplitArtifactReachable(t *testing.T) {
	svc := newTestAssetService(t)
	var opened []string
	svc.openSplitPageFn = func(p string) error {
		opened = append(opened, p)
		return nil
	}
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "改名重链库", dir)
	buildAssetsPptx(t, dir, "旧名.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	var hash string
	if err := store.db.QueryRow("SELECT id, hash FROM files WHERE path = ?", "旧名.pptx").Scan(&fileID, &hash); err != nil {
		t.Fatalf("查文件记录: %v", err)
	}
	outDir := filepath.Join(dir, "旧名")
	if err := store.upsertSplitRecord("旧名.pptx", hash, outDir, 2, "2026-10-30 10:00:00", model.AssetSplitTargetSource, "", false); err != nil {
		t.Fatalf("写入记录: %v", err)
	}
	writeSplitArtifactForTest(t, filepath.Join(outDir, "旧名-001.pptx"))
	writeSplitArtifactForTest(t, filepath.Join(outDir, "旧名-002.pptx"))

	newDirAbs := filepath.Join(dir, "新目录")
	if err := os.MkdirAll(newDirAbs, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	if err := os.Rename(filepath.Join(dir, "旧名.pptx"), filepath.Join(newDirAbs, "新名.pptx")); err != nil {
		t.Fatalf("改名移动: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, filepath.Join(newDirAbs, "新名.pptx"), false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	const newRel = "新目录/新名.pptx"

	view, err := svc.GetAssetSplitRecord(lib.ID, newRel)
	if err != nil || !view.Exists {
		t.Fatalf("改名重链后记录应可查: err=%v view=%+v", err, view)
	}
	if view.OutputDir != outDir {
		t.Errorf("output_dir 应保持产物实际目录 %q，实际 %q", outDir, view.OutputDir)
	}

	wantArtifact := filepath.Join(outDir, "旧名-001.pptx")
	if err := svc.OpenAssetSplitPage(lib.ID, newRel, 1); err != nil {
		t.Fatalf("改名重链后打开拆分单页应成功: %v", err)
	}
	if len(opened) != 1 || !samePathFold(opened[0], wantArtifact) {
		t.Errorf("应打开旧主干产物 %q，实际 %v", wantArtifact, opened)
	}

	row := &assetExportRow{
		relPath: newRel, slideNum: 1, hasSplitRecord: true,
		splitOutputDir: outDir, splitPageCount: 2, splitTarget: model.AssetSplitTargetSource,
	}
	got := row.singlePptxPath(&assetExportCtx{lib: lib, pathMode: model.AssetExportPathModeAbsolute})
	if !samePathFold(got, wantArtifact) {
		t.Errorf("导出 single_pptx_path 应为 %q，实际 %q", wantArtifact, got)
	}

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, newRel, model.AssetSplitTargetSource)
	mustSplitSkipped(t, res, err)
}

// TestRelocateAssetFile_RenameReplaceArtifactStillFiltered replace 记录改名重链后，
// 旧主干的单页产物仍被批量拆分剔除（不拆出「页的页」）。
func TestRelocateAssetFile_RenameReplaceArtifactStillFiltered(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "旧名")
	outputs := []replaceSplitOutput{{outputDir: outDir, sourceRel: "新目录/新名.pptx"}}
	renamed := filepath.Join(outDir, replaceArtifactName(2, "旧名"))
	writeSplitArtifactForTest(t, renamed)

	if !isReplaceArtifactOutput(renamed, outputs) {
		t.Errorf("改名重链后旧主干产物 %q 应仍被识别为 replace 产物", filepath.Base(renamed))
	}
	// 用户自建同名异构文件（<主干>-<数字>.pptx）不误剔除
	userFile := filepath.Join(outDir, "草稿-1.pptx")
	if isReplaceArtifactOutput(userFile, outputs) {
		t.Errorf("用户自建文件 %q 不应被判为拆分产物", filepath.Base(userFile))
	}
	// 目录外的同形文件不命中
	outside := filepath.Join(t.TempDir(), replaceArtifactName(2, "旧名"))
	if isReplaceArtifactOutput(outside, outputs) {
		t.Error("产物目录之外的路径不应被判为拆分产物")
	}

	sources := []assetSplitSource{{rel: "旧名/旧名-001.pptx"}}
	got := filterReplaceArtifactSources(model.AssetLibrary{RootPath: t.TempDir()}, sources, outputs)
	// 根目录不同时绝对路径不落在 outDir 下，仅验证单点判定路径；filterReplaceArtifactSources
	// 的目录匹配语义由既有用例守护。
	if len(got) != 1 {
		t.Errorf("目录不匹配时不应剔除，实际剩 %d 条", len(got))
	}
}

// TestMigrateSplitRecords_KeepsFrozenOutDirAfterRelink 重链把源迁到 B 后，再移动
// 目录 B：记录键随目录迁移，但 output_dir 冻结在产物实际所在的 A——不被重算到
// 新源位置（否则「打开拆分单页」再次指向不存在目录）。
func TestMigrateSplitRecords_KeepsFrozenOutDirAfterRelink(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "冻结落点库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	const now = "2026-10-30 10:00:00"
	frozenOut := filepath.Join(dir, "旧名")
	// 前置：源已在 B（重链后的现状），产物目录仍在 A
	if _, err := store.db.Exec(
		"INSERT INTO files(id, path, size, mtime_ns, status, hash, updated_at) VALUES(1, ?, 1, 1, ?, '', ?)",
		"新目录/新名.pptx", model.AssetFileStatusOK, now,
	); err != nil {
		t.Fatalf("插入文件记录: %v", err)
	}
	if err := store.upsertSplitRecord("新目录/新名.pptx", "h", frozenOut, 2, now, model.AssetSplitTargetSource, "", false); err != nil {
		t.Fatalf("写入记录: %v", err)
	}

	renames := []assetPathRename{{fileID: 1, oldPath: "新目录/新名.pptx", newRel: "目标目录/新名.pptx"}}
	if err := store.replaceFilePaths(renames, now, "新目录", "目标目录",
		splitRecordMigration{libraryRoot: dir, oldPrefix: "新目录", newPrefix: "目标目录"}); err != nil {
		t.Fatalf("replaceFilePaths: %v", err)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "目标目录/新名.pptx")
	if rec == nil {
		t.Fatal("记录键应随目录迁移")
	}
	if rec.outputDir != frozenOut {
		t.Errorf("冻结落点不得被重算，期望 %q，实际 %q", frozenOut, rec.outputDir)
	}
}

// TestMigrateSplitRecords_RecalcWhenOutDirUnderPrefix 常规落点回归：产物目录位于
// 源目录下时，目录迁移仍按「库根 + 新源目录 + 新源主干」重算（修复不得误伤既有口径）。
func TestMigrateSplitRecords_RecalcWhenOutDirUnderPrefix(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "常规落点库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	const now = "2026-10-30 10:00:00"
	oldOut := filepath.Join(dir, "常规", "方案")
	if _, err := store.db.Exec(
		"INSERT INTO files(id, path, size, mtime_ns, status, hash, updated_at) VALUES(1, ?, 1, 1, ?, '', ?)",
		"常规/方案.pptx", model.AssetFileStatusOK, now,
	); err != nil {
		t.Fatalf("插入文件记录: %v", err)
	}
	if err := store.upsertSplitRecord("常规/方案.pptx", "h", oldOut, 2, now, model.AssetSplitTargetSource, "", false); err != nil {
		t.Fatalf("写入记录: %v", err)
	}

	renames := []assetPathRename{{fileID: 1, oldPath: "常规/方案.pptx", newRel: "归档/方案.pptx"}}
	if err := store.replaceFilePaths(renames, now, "常规", "归档",
		splitRecordMigration{libraryRoot: dir, oldPrefix: "常规", newPrefix: "归档"}); err != nil {
		t.Fatalf("replaceFilePaths: %v", err)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "归档/方案.pptx")
	if rec == nil {
		t.Fatal("记录键应迁移")
	}
	wantOut := filepath.Join(dir, "归档", "方案")
	if rec.outputDir != wantOut {
		t.Errorf("常规落点应重算为 %q，实际 %q", wantOut, rec.outputDir)
	}
}

// TestClearLostAssetFiles_DropsSplitRecords 清理丢失记录：files 行与其拆分记录
// 同事务删除（孤儿记录不得残留在被清理文件的路径上）。
func TestClearLostAssetFiles_DropsSplitRecords(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "清理联动库", dir)
	src := buildAssetsPptx(t, dir, "待清.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("待清.pptx", "h", filepath.Join(dir, "待清"), 2,
		"2026-10-30 10:00:00", model.AssetSplitTargetCache, "",
		false); err != nil {
		t.Fatalf("写入记录: %v", err)
	}
	// 模拟源消失后的 lost 态
	if err := os.Remove(src); err != nil {
		t.Fatalf("删除源文件: %v", err)
	}
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("二次扫描: %v", err)
	}
	if splitRecordRowForTest(t, svc, lib.ID, "待清.pptx") == nil {
		t.Fatal("前置：丢失文件的拆分记录应存在")
	}
	if _, err := svc.ClearLostAssetFiles(lib.ID); err != nil {
		t.Fatalf("ClearLostAssetFiles: %v", err)
	}
	if rec := splitRecordRowForTest(t, svc, lib.ID, "待清.pptx"); rec != nil {
		t.Errorf("清理丢失记录后拆分记录应同事务删除，仍剩 %+v", rec)
	}
}

// TestRelocateAssetFile_ForceDropsSplitRecord force 重链（内容已变）删除失效记录；
// 哈希匹配（含改名）保留记录——两条分支互不串味。
func TestRelocateAssetFile_ForceDropsSplitRecord(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "force 删记录库", dir)
	buildAssetsPptx(t, dir, "原.pptx", []assetsSlideSpec{{titlePh: "原标题", body: []string{"原正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "原.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查文件记录: %v", err)
	}
	if err := store.upsertSplitRecord("原.pptx", "h", filepath.Join(dir, "原"), 2,
		"2026-10-30 10:00:00", model.AssetSplitTargetCache, "",
		false); err != nil {
		t.Fatalf("写入记录: %v", err)
	}
	// 未入库的不同内容文件（新目标）
	otherAbs := buildAssetsPptx(t, dir, "他内容.pptx", []assetsSlideSpec{{titlePh: "他标题", body: []string{"他正文"}}})

	// force=false：内容不一致 → 确认型拒绝，记录保留
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, otherAbs, false); err == nil {
		t.Fatal("哈希不一致且未 force 时应返回确认型拒绝")
	}
	if tot := countSplitRecordsForTest(t, svc, lib.ID); tot != 1 {
		t.Fatalf("确认型拒绝不得改动记录，实际 %d 条", tot)
	}

	// force=true：内容已变 → 删失效记录
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, otherAbs, true); err != nil {
		t.Fatalf("force 重链: %v", err)
	}
	if rec := splitRecordRowForTest(t, svc, lib.ID, "他内容.pptx"); rec != nil {
		t.Errorf("force 重链后旧拆分记录应删除，仍剩 %+v", rec)
	}
	if rec := splitRecordRowForTest(t, svc, lib.ID, "原.pptx"); rec != nil {
		t.Errorf("旧键记录不得残留，仍剩 %+v", rec)
	}
}
