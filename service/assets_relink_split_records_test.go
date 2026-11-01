package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// 文件级重链同步迁移 split_records（11-01-relink-split-records）单测：
//   - 三类 target（cache/source/replace）记录的源键迁移，output_dir 保持原绝对路径；
//   - 重链后 GetAssetSplitRecord(新路径) 命中、OpenAssetSplitPage 可打开单页产物；
//   - 新键冲突沿用目录级迁移口径（保留 source_removed=1 的一条并 Warn）；
//   - 重复重链幂等、旧键不存在零操作；
//   - 事务回滚：split_records 写失败时 files.path 与记录均不变。

// splitRecordRowForTest 白盒读单条拆分记录（nil = 键不存在）。
func splitRecordRowForTest(t *testing.T, svc *AssetLibraryService, libraryID, rel string) *assetSplitRecord {
	t.Helper()
	rec, err := svc.openStoreForTest(t, libraryID).getSplitRecord(rel)
	if err != nil {
		t.Fatalf("读拆分记录（%s）: %v", rel, err)
	}
	return rec
}

// countSplitRecordsForTest 白盒统计拆分记录行数。
func countSplitRecordsForTest(t *testing.T, svc *AssetLibraryService, libraryID string) int {
	t.Helper()
	var n int
	if err := svc.openStoreForTest(t, libraryID).db.QueryRow("SELECT COUNT(*) FROM split_records").Scan(&n); err != nil {
		t.Fatalf("统计拆分记录: %v", err)
	}
	return n
}

// writeSplitArtifactForTest 在产物目录写一个占位 pptx（OpenAssetSplitPage 的
// 存在性校验用，内容无关）。
func writeSplitArtifactForTest(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("建产物目录: %v", err)
	}
	if err := os.WriteFile(path, []byte("pptx"), 0644); err != nil {
		t.Fatalf("写产物文件: %v", err)
	}
}

// TestRelocateAssetFile_MigratesSplitRecords 三类 target 记录的源键迁移：
// 缓存落点（哈希匹配）、源落点（无哈希走重解析）、替换落点（哈希匹配）——
// 新键可查、旧键消失、output_dir 保持原绝对路径、target/页数不变，且
// OpenAssetSplitPage 按迁移后的键仍能定位产物。
func TestRelocateAssetFile_MigratesSplitRecords(t *testing.T) {
	svc := newTestAssetService(t)
	var opened []string
	svc.openSplitPageFn = func(p string) error {
		opened = append(opened, p)
		return nil
	}
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重链拆分记录库", dir)
	for _, name := range []string{"缓存.pptx", "源落点.pptx", "替换落点.pptx"} {
		buildAssetsPptx(t, dir, name, []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	}
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}

	store := svc.openStoreForTest(t, lib.ID)
	const splitAt = "2026-10-30 10:00:00"
	cacheOut := filepath.Join(svc.assetDerivedDir(lib.ID), "缓存")
	sourceOut := filepath.Join(dir, "源落点")
	replaceOut := filepath.Join(dir, "替换落点")
	if err := store.upsertSplitRecord("缓存.pptx", "h-cache", cacheOut, 2, splitAt, model.AssetSplitTargetCache, "", false); err != nil {
		t.Fatalf("写入缓存记录: %v", err)
	}
	if err := store.upsertSplitRecord("源落点.pptx", "h-source", sourceOut, 2, splitAt, model.AssetSplitTargetSource, "", false); err != nil {
		t.Fatalf("写入源落点记录: %v", err)
	}
	if err := store.upsertSplitRecord("替换落点.pptx", "h-replace", replaceOut, 2, splitAt, model.AssetSplitTargetReplace, "", false); err != nil {
		t.Fatalf("写入替换落点记录: %v", err)
	}
	// 三类产物（迁移后 output_dir 不变，打开单页仍应命中）
	writeSplitArtifactForTest(t, filepath.Join(cacheOut, "缓存-001.pptx"))
	writeSplitArtifactForTest(t, filepath.Join(sourceOut, "源落点-001.pptx"))
	writeSplitArtifactForTest(t, filepath.Join(replaceOut, replaceArtifactName(1, "替换落点")))

	// 无哈希记录走重解析分支（哈希校验跳过 → relocateFileProcessing）
	if _, err := store.db.Exec("UPDATE files SET hash = '' WHERE path = ?", "源落点.pptx"); err != nil {
		t.Fatalf("清空哈希: %v", err)
	}
	fileIDs := map[string]int64{}
	rows, err := store.db.Query("SELECT id, path FROM files")
	if err != nil {
		t.Fatalf("读文件记录: %v", err)
	}
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			_ = rows.Close()
			t.Fatalf("扫描行: %v", err)
		}
		fileIDs[p] = id
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("关闭行: %v", err)
	}

	newDirAbs := filepath.Join(dir, "新目录")
	if err := os.MkdirAll(newDirAbs, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	cases := []struct {
		name      string
		target    string
		outDir    string
		artifact  string
		wantParse bool
	}{
		{"缓存.pptx", model.AssetSplitTargetCache, cacheOut, filepath.Join(cacheOut, "缓存-001.pptx"), false},
		{"源落点.pptx", model.AssetSplitTargetSource, sourceOut, filepath.Join(sourceOut, "源落点-001.pptx"), true},
		{"替换落点.pptx", model.AssetSplitTargetReplace, replaceOut, filepath.Join(replaceOut, replaceArtifactName(1, "替换落点")), false},
	}
	for _, tc := range cases {
		srcAbs := filepath.Join(dir, tc.name)
		newAbs := filepath.Join(newDirAbs, tc.name)
		if err := os.Rename(srcAbs, newAbs); err != nil {
			t.Fatalf("移动 %s: %v", tc.name, err)
		}
		// 重链前旧键在位
		if splitRecordRowForTest(t, svc, lib.ID, tc.name) == nil {
			t.Fatalf("%s 前置记录缺失", tc.name)
		}
		result, err := svc.RelocateAssetFile(lib.ID, fileIDs[tc.name], newAbs, false)
		if err != nil {
			t.Fatalf("RelocateAssetFile(%s): %v", tc.name, err)
		}
		if result.Reparsed != tc.wantParse {
			t.Errorf("%s 重解析标志应为 %v，实际 %+v", tc.name, tc.wantParse, result)
		}

		newRel := "新目录/" + tc.name
		rec := splitRecordRowForTest(t, svc, lib.ID, newRel)
		if rec == nil {
			t.Fatalf("%s 重链后新键应命中拆分记录", tc.name)
		}
		if rec.outputDir != tc.outDir {
			t.Errorf("%s output_dir 应保持原绝对路径 %q，实际 %q", tc.name, tc.outDir, rec.outputDir)
		}
		if rec.target != tc.target {
			t.Errorf("%s target 应保持 %q，实际 %q", tc.name, tc.target, rec.target)
		}
		if rec.pageCount != 2 || rec.splitAt != splitAt {
			t.Errorf("%s 记录其余字段应原样保留，实际 %+v", tc.name, rec)
		}
		if splitRecordRowForTest(t, svc, lib.ID, tc.name) != nil {
			t.Errorf("%s 旧键应消失（不得残留旧记录）", tc.name)
		}

		view, err := svc.GetAssetSplitRecord(lib.ID, newRel)
		if err != nil {
			t.Fatalf("GetAssetSplitRecord(%s): %v", tc.name, err)
		}
		if !view.Exists || view.OutputDir != tc.outDir || view.Target != tc.target {
			t.Errorf("%s 新路径查询应命中且落点一致，实际 %+v", tc.name, view)
		}
		opened = opened[:0]
		if err := svc.OpenAssetSplitPage(lib.ID, newRel, 1); err != nil {
			t.Fatalf("%s 重链后打开拆分单页应成功: %v", tc.name, err)
		}
		if len(opened) != 1 || !samePathFold(opened[0], tc.artifact) {
			t.Errorf("%s 打开的产物路径应为 %q，实际 %v", tc.name, tc.artifact, opened)
		}
	}
	if n := countSplitRecordsForTest(t, svc, lib.ID); n != 3 {
		t.Errorf("拆分记录条数应恒为 3（无滞留旧记录），实际 %d", n)
	}
}

// TestRelocateAssetFile_SplitRecordKeyConflict 新键已存在时的冲突消解沿用
// migrateSplitRecordsTx 口径：保留 source_removed=1 的一条并删除落败行。
//   - 既有行为留痕（removed=1）、待迁移行源仍在（removed=0）→ 既有胜出；
//   - 待迁移行为留痕（removed=1）、既有行源仍在（removed=0）→ 留痕胜出。
func TestRelocateAssetFile_SplitRecordKeyConflict(t *testing.T) {
	cases := []struct {
		name          string
		oldRemoved    bool
		existRemoved  bool
		wantOutDir    string
		wantSurvivors int
	}{
		{"既有留痕胜出", false, true, "既有产物目录", 1},
		{"旧行留痕胜出", true, false, "旧产物目录", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestAssetService(t)
			dir := t.TempDir()
			lib := mustCreateLibrary(t, svc, "冲突库", dir)
			const name = "冲突.pptx"
			buildAssetsPptx(t, dir, name, []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
			if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
				t.Fatalf("首次扫描: %v", err)
			}
			store := svc.openStoreForTest(t, lib.ID)
			var fileID int64
			if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", name).Scan(&fileID); err != nil {
				t.Fatalf("查文件记录: %v", err)
			}
			const splitAt = "2026-10-30 10:00:00"
			oldOut := "旧产物目录"
			newRel := "新目录/" + name
			existOut := "既有产物目录"
			if err := store.upsertSplitRecord(name, "h-old", oldOut, 2, splitAt, model.AssetSplitTargetCache, "", tc.oldRemoved); err != nil {
				t.Fatalf("写入旧记录: %v", err)
			}
			// 目标键记录只存在于 split_records（files 表无该路径，重链不被占用预检拦下）
			if err := store.upsertSplitRecord(newRel, "h-exist", existOut, 3, splitAt, model.AssetSplitTargetCache, "", tc.existRemoved); err != nil {
				t.Fatalf("写入目标键记录: %v", err)
			}

			newDirAbs := filepath.Join(dir, "新目录")
			if err := os.MkdirAll(newDirAbs, 0755); err != nil {
				t.Fatalf("建新目录: %v", err)
			}
			if err := os.Rename(filepath.Join(dir, name), filepath.Join(newDirAbs, name)); err != nil {
				t.Fatalf("移动文件: %v", err)
			}
			if _, err := svc.RelocateAssetFile(lib.ID, fileID, filepath.Join(newDirAbs, name), false); err != nil {
				t.Fatalf("RelocateAssetFile: %v", err)
			}

			if n := countSplitRecordsForTest(t, svc, lib.ID); n != tc.wantSurvivors {
				t.Fatalf("冲突后应只剩 %d 条记录，实际 %d", tc.wantSurvivors, n)
			}
			survivor := splitRecordRowForTest(t, svc, lib.ID, newRel)
			if survivor == nil {
				t.Fatal("新键应命中胜出记录")
			}
			if survivor.sourceRemoved != (tc.oldRemoved || tc.existRemoved) {
				t.Errorf("胜出记录 source_removed 应为 true，实际 %+v", survivor)
			}
			if survivor.outputDir != tc.wantOutDir {
				t.Errorf("胜出记录 output_dir 应为 %q，实际 %q", tc.wantOutDir, survivor.outputDir)
			}
			if splitRecordRowForTest(t, svc, lib.ID, name) != nil {
				t.Error("旧键不得残留")
			}
		})
	}
}

// TestRelocateAssetFile_SplitRecordIdempotent 重复重链幂等 + 无记录零操作：
// 第二次重链（目标即当前路径）不改写记录；从未拆分的文件重链不报错。
func TestRelocateAssetFile_SplitRecordIdempotent(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "幂等库", dir)
	buildAssetsPptx(t, dir, "幂等.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	buildAssetsPptx(t, dir, "无记录.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	fileIDs := map[string]int64{}
	rows, err := store.db.Query("SELECT id, path FROM files")
	if err != nil {
		t.Fatalf("读文件记录: %v", err)
	}
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			_ = rows.Close()
			t.Fatalf("扫描行: %v", err)
		}
		fileIDs[p] = id
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("关闭行: %v", err)
	}
	const splitAt = "2026-10-30 10:00:00"
	if err := store.upsertSplitRecord("幂等.pptx", "h-idem", "D:/derived/幂等", 2, splitAt, model.AssetSplitTargetCache, "", false); err != nil {
		t.Fatalf("写入记录: %v", err)
	}

	newDirAbs := filepath.Join(dir, "新目录")
	if err := os.MkdirAll(newDirAbs, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDirAbs, "幂等.pptx")
	if err := os.Rename(filepath.Join(dir, "幂等.pptx"), newAbs); err != nil {
		t.Fatalf("移动文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileIDs["幂等.pptx"], newAbs, false); err != nil {
		t.Fatalf("首次重链: %v", err)
	}
	first := splitRecordRowForTest(t, svc, lib.ID, "新目录/幂等.pptx")
	if first == nil {
		t.Fatal("首次重链后新键应命中")
	}
	// 重复重链：目标即当前路径，记录零变更
	if _, err := svc.RelocateAssetFile(lib.ID, fileIDs["幂等.pptx"], newAbs, false); err != nil {
		t.Fatalf("重复重链: %v", err)
	}
	second := splitRecordRowForTest(t, svc, lib.ID, "新目录/幂等.pptx")
	if second == nil || *second != *first {
		t.Errorf("重复重链应幂等零变更，实际 %+v → %+v", first, second)
	}
	if n := countSplitRecordsForTest(t, svc, lib.ID); n != 1 {
		t.Errorf("记录条数应恒为 1，实际 %d", n)
	}

	// 无拆分记录的文件：重链成功且不产生记录
	newAbs2 := filepath.Join(newDirAbs, "无记录.pptx")
	if err := os.Rename(filepath.Join(dir, "无记录.pptx"), newAbs2); err != nil {
		t.Fatalf("移动无记录文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileIDs["无记录.pptx"], newAbs2, false); err != nil {
		t.Fatalf("无记录重链不应报错: %v", err)
	}
	if n := countSplitRecordsForTest(t, svc, lib.ID); n != 1 {
		t.Errorf("无记录文件重链不得新增记录，实际 %d", n)
	}
}

// TestRelocateAssetFile_SplitRecordTxRollback 事务回滚：split_records 写失败时
// files.path 与拆分记录均保持原值（同事务保证「路径已改、记录仍指旧键」半态不落地）。
func TestRelocateAssetFile_SplitRecordTxRollback(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "回滚库", dir)
	buildAssetsPptx(t, dir, "回滚.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "回滚.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查文件记录: %v", err)
	}
	const splitAt = "2026-10-30 10:00:00"
	if err := store.upsertSplitRecord("回滚.pptx", "h-rollback", "D:/derived/回滚", 2, splitAt, model.AssetSplitTargetCache, "", false); err != nil {
		t.Fatalf("写入记录: %v", err)
	}
	// 触发器注入写失败：split_records 任一 UPDATE 中止事务
	if _, err := store.db.Exec(
		`CREATE TRIGGER block_split_migrate BEFORE UPDATE ON split_records BEGIN SELECT RAISE(ABORT, 'blocked'); END;`,
	); err != nil {
		t.Fatalf("创建触发器: %v", err)
	}

	newDirAbs := filepath.Join(dir, "新目录")
	if err := os.MkdirAll(newDirAbs, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDirAbs, "回滚.pptx")
	if err := os.Rename(filepath.Join(dir, "回滚.pptx"), newAbs); err != nil {
		t.Fatalf("移动文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err == nil {
		t.Fatal("拆分记录写失败时重链应报错")
	}

	var path string
	if err := store.db.QueryRow("SELECT path FROM files WHERE id = ?", fileID).Scan(&path); err != nil {
		t.Fatalf("读文件路径: %v", err)
	}
	if path != "回滚.pptx" {
		t.Errorf("事务回滚后 files.path 应保持原值，实际 %q", path)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "回滚.pptx")
	if rec == nil || rec.outputDir != "D:/derived/回滚" {
		t.Errorf("事务回滚后拆分记录应保持原键与原落点，实际 %+v", rec)
	}
	if splitRecordRowForTest(t, svc, lib.ID, "新目录/回滚.pptx") != nil {
		t.Error("事务回滚后不得出现新键记录")
	}
}
