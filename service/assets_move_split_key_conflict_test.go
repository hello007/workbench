package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// 单文件移动（MoveAssetFiles / moveFilePath）的拆分记录键改写收口用例：
// 裸 UPDATE 改为复用 rewriteSplitRecordKeyTx 单点后，非冲突迁移、目标键冲突
// 消解、幂等、旧键不存在零操作、事务回滚五类语义均须与重链/目录迁移同口径。
// 夹具同移动用例：临时目录 + 伪 pptx + parseFn 注入 fake（禁环境外部状态）。

const moveSplitKeyTestAt = "2026-11-01 10:00:00"

// moveSplitKeyTestFileID 按相对路径取 files.id（移动前路径）。
func moveSplitKeyTestFileID(t *testing.T, svc *AssetLibraryService, libraryID, rel string) int64 {
	t.Helper()
	entries, err := svc.ListFiles(libraryID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, e := range entries {
		if e.Path == rel {
			return e.ID
		}
	}
	t.Fatalf("文件记录缺失：%s", rel)
	return 0
}

// TestAssetMoveFiles_SplitRecordNoConflict 无冲突迁移回归：移动后拆分记录按新
// 键可查、旧键不残留、output_dir 与其余字段原样保留（含 v9 产物主干，主干与
// 路径无关，键改写禁触碰）。
func TestAssetMoveFiles_SplitRecordNoConflict(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动拆分键无冲突库", root)
	mustScan(t, svc, lib.ID)
	fileID := moveSplitKeyTestFileID(t, svc, lib.ID, "media/a.pptx")
	if err := svc.openStoreForTest(t, lib.ID).upsertSplitRecord(
		"media/a.pptx", "hash-a", "D:/derived/media/a", 3, moveSplitKeyTestAt, model.AssetSplitTargetCache, "a",
		false, nil, nil); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}

	result, err := svc.MoveAssetFiles(lib.ID, []int64{fileID}, "归档")
	if err != nil {
		t.Fatalf("MoveAssetFiles: %v", err)
	}
	if result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("移动应成功，实际 %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "归档", "a.pptx")); err != nil {
		t.Fatalf("磁盘文件应已移至新目录: %v", err)
	}

	rec := splitRecordRowForTest(t, svc, lib.ID, "归档/a.pptx")
	if rec == nil {
		t.Fatal("移动后拆分记录应按新键可查")
	}
	if rec.outputDir != "D:/derived/media/a" || rec.pageCount != 3 ||
		rec.target != model.AssetSplitTargetCache || rec.sourceRemoved || rec.artifactStem != "a" {
		t.Errorf("拆分记录其余字段（含 v9 产物主干）应原样保留，实际 %+v", rec)
	}
	if splitRecordRowForTest(t, svc, lib.ID, "media/a.pptx") != nil {
		t.Error("旧键拆分记录不得残留")
	}
	if n := countSplitRecordsForTest(t, svc, lib.ID); n != 1 {
		t.Errorf("拆分记录条数应恒为 1，实际 %d", n)
	}
}

// TestAssetMoveFiles_SplitRecordKeyConflict_RemovedExistingWins 目标键已被
// source_removed=1 记录占用：移动不因主键冲突回滚，记录收敛为一条且保留既有
// 留痕行，files.path 为新路径、磁盘文件已移动。
func TestAssetMoveFiles_SplitRecordKeyConflict_RemovedExistingWins(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动拆分键冲突留痕库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	fileID := moveSplitKeyTestFileID(t, svc, lib.ID, "media/a.pptx")
	if err := store.upsertSplitRecord(
		"media/a.pptx", "hash-a", "旧产物目录", 3, moveSplitKeyTestAt, model.AssetSplitTargetCache, "",
		false, nil, nil); err != nil {
		t.Fatalf("写入待迁移记录: %v", err)
	}
	// 目标键既有留痕记录（files 表无该路径，不触发磁盘重名与占用预检）
	if err := store.upsertSplitRecord(
		"归档/a.pptx", "hash-exist", "既有产物目录", 2, moveSplitKeyTestAt, model.AssetSplitTargetReplace, "",
		true, nil, nil); err != nil {
		t.Fatalf("写入目标键记录: %v", err)
	}

	result, err := svc.MoveAssetFiles(lib.ID, []int64{fileID}, "归档")
	if err != nil {
		t.Fatalf("目标键冲突不得回滚移动事务，实际 err：%v", err)
	}
	if result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("移动应成功，实际 %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "归档", "a.pptx")); err != nil {
		t.Fatalf("磁盘文件应已移至新目录: %v", err)
	}
	var path string
	if err := store.db.QueryRow("SELECT path FROM files WHERE id = ?", fileID).Scan(&path); err != nil {
		t.Fatalf("读文件路径: %v", err)
	}
	if path != "归档/a.pptx" {
		t.Errorf("files.path 应为新路径，实际 %q", path)
	}

	if n := countSplitRecordsForTest(t, svc, lib.ID); n != 1 {
		t.Fatalf("冲突后应只剩 1 条记录，实际 %d", n)
	}
	survivor := splitRecordRowForTest(t, svc, lib.ID, "归档/a.pptx")
	if survivor == nil {
		t.Fatal("新键应命中胜出记录")
	}
	if !survivor.sourceRemoved || survivor.outputDir != "既有产物目录" {
		t.Errorf("应保留既有留痕记录（source_removed=1，落点原样），实际 %+v", survivor)
	}
	if splitRecordRowForTest(t, svc, lib.ID, "media/a.pptx") != nil {
		t.Error("待迁移记录应已被删除，旧键不得残留")
	}
}

// TestAssetMoveFiles_SplitRecordKeyConflict_BothSourceExists 目标键已被
// source_removed=0 记录占用、待迁移记录同为 0：按既有口径保留既有行、删除待
// 迁移行，移动照常成功。
func TestAssetMoveFiles_SplitRecordKeyConflict_BothSourceExists(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动拆分键双源在库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	fileID := moveSplitKeyTestFileID(t, svc, lib.ID, "media/a.pptx")
	if err := store.upsertSplitRecord(
		"media/a.pptx", "hash-a", "旧产物目录", 3, moveSplitKeyTestAt, model.AssetSplitTargetCache, "",
		false, nil, nil); err != nil {
		t.Fatalf("写入待迁移记录: %v", err)
	}
	if err := store.upsertSplitRecord(
		"归档/a.pptx", "hash-exist", "既有产物目录", 2, moveSplitKeyTestAt, model.AssetSplitTargetCache, "",
		false, nil, nil); err != nil {
		t.Fatalf("写入目标键记录: %v", err)
	}

	result, err := svc.MoveAssetFiles(lib.ID, []int64{fileID}, "归档")
	if err != nil {
		t.Fatalf("MoveAssetFiles: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("移动应成功，实际 %+v", result)
	}
	if n := countSplitRecordsForTest(t, svc, lib.ID); n != 1 {
		t.Fatalf("冲突后应只剩 1 条记录，实际 %d", n)
	}
	survivor := splitRecordRowForTest(t, svc, lib.ID, "归档/a.pptx")
	if survivor == nil {
		t.Fatal("新键应命中胜出记录")
	}
	if survivor.sourceRemoved || survivor.outputDir != "既有产物目录" || survivor.pageCount != 2 {
		t.Errorf("应保留既有记录（source_removed=0，落点原样），实际 %+v", survivor)
	}
	if splitRecordRowForTest(t, svc, lib.ID, "media/a.pptx") != nil {
		t.Error("待迁移记录应已被删除，旧键不得残留")
	}
}

// TestAssetMoveFilePath_SplitRecordIdempotent 同一 (old→new) 重复调用零变更、
// 不报错：第二次改写时旧键已不存在，单点按「键不存在零操作」返回。
func TestAssetMoveFilePath_SplitRecordIdempotent(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动拆分键幂等库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	fileID := moveSplitKeyTestFileID(t, svc, lib.ID, "media/a.pptx")
	if err := store.upsertSplitRecord(
		"media/a.pptx", "hash-a", "D:/derived/media/a", 3, moveSplitKeyTestAt, model.AssetSplitTargetCache, "",
		false, nil, nil); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}

	r := relPathRename{fileID: fileID, oldRel: "media/a.pptx", newRel: "归档/a.pptx"}
	if err := store.moveFilePath(r, moveSplitKeyTestAt); err != nil {
		t.Fatalf("首次 moveFilePath: %v", err)
	}
	first := splitRecordRowForTest(t, svc, lib.ID, "归档/a.pptx")
	if first == nil {
		t.Fatal("首次移动后新键应命中拆分记录")
	}
	// 重复执行同一 (old→new)：files.path 写同值、拆分键旧键已不存在 → 零变更不报错
	if err := store.moveFilePath(r, moveSplitKeyTestAt); err != nil {
		t.Fatalf("重复 moveFilePath 应幂等不报错，实际：%v", err)
	}
	second := splitRecordRowForTest(t, svc, lib.ID, "归档/a.pptx")
	if !splitRecordEqualForTest(second, first) {
		t.Errorf("重复执行应零变更，实际 %+v → %+v", first, second)
	}
	if n := countSplitRecordsForTest(t, svc, lib.ID); n != 1 {
		t.Errorf("拆分记录条数应恒为 1，实际 %d", n)
	}
}

// TestAssetMoveFiles_SplitRecordTxRollback 事务回滚：split_records 写失败时
// files.path 与拆分记录均保持原值，移动条目以 failed 明细返回（磁盘 rename
// 已先行完成属既有语义：失败后由后续扫描按实际磁盘收敛）。
func TestAssetMoveFiles_SplitRecordTxRollback(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动拆分键回滚库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	fileID := moveSplitKeyTestFileID(t, svc, lib.ID, "media/a.pptx")
	if err := store.upsertSplitRecord(
		"media/a.pptx", "hash-a", "D:/derived/media/a", 3, moveSplitKeyTestAt, model.AssetSplitTargetCache, "",
		false, nil, nil); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}
	// 触发器注入写失败：split_records 任一 UPDATE 中止事务
	if _, err := store.db.Exec(
		`CREATE TRIGGER block_split_move BEFORE UPDATE ON split_records BEGIN SELECT RAISE(ABORT, 'blocked'); END;`,
	); err != nil {
		t.Fatalf("创建触发器: %v", err)
	}

	result, err := svc.MoveAssetFiles(lib.ID, []int64{fileID}, "归档")
	if err != nil {
		t.Fatalf("MoveAssetFiles 应以明细隔离失败而非整体报错: %v", err)
	}
	if result.Succeeded != 0 || result.Failed != 1 {
		t.Fatalf("拆分记录写失败应计入 failed 明细，实际 %+v", result)
	}
	if len(result.Items) != 1 || !strings.Contains(result.Items[0].Message, "更新索引路径失败") {
		t.Errorf("失败明细应说明索引更新失败，实际 %+v", result.Items)
	}

	var path string
	if err := store.db.QueryRow("SELECT path FROM files WHERE id = ?", fileID).Scan(&path); err != nil {
		t.Fatalf("读文件路径: %v", err)
	}
	if path != "media/a.pptx" {
		t.Errorf("事务回滚后 files.path 应保持原值，实际 %q", path)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "media/a.pptx")
	if rec == nil || rec.outputDir != "D:/derived/media/a" {
		t.Errorf("事务回滚后拆分记录应保持原键与原落点，实际 %+v", rec)
	}
	if splitRecordRowForTest(t, svc, lib.ID, "归档/a.pptx") != nil {
		t.Error("事务回滚后不得出现新键记录")
	}
}

// TestAssetMoveFiles_SplitRecordMissingKeyNoop 旧键不存在：移动成功且不产生
// 拆分记录（从未拆分的文件走同一事务落库零操作）。
func TestAssetMoveFiles_SplitRecordMissingKeyNoop(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动拆分键无记录库", root)
	mustScan(t, svc, lib.ID)
	fileID := moveSplitKeyTestFileID(t, svc, lib.ID, "media/a.pptx")

	result, err := svc.MoveAssetFiles(lib.ID, []int64{fileID}, "归档")
	if err != nil {
		t.Fatalf("MoveAssetFiles: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("移动应成功，实际 %+v", result)
	}
	if n := countSplitRecordsForTest(t, svc, lib.ID); n != 0 {
		t.Errorf("无拆分记录的文件移动不得新增记录，实际 %d", n)
	}
	var path string
	if err := svc.openStoreForTest(t, lib.ID).db.QueryRow(
		"SELECT path FROM files WHERE id = ?", fileID,
	).Scan(&path); err != nil {
		t.Fatalf("读文件路径: %v", err)
	}
	if path != "归档/a.pptx" {
		t.Errorf("files.path 应为新路径，实际 %q", path)
	}
}
