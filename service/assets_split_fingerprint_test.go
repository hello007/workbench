package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// 拆分产物页级指纹（schema v10，11-02-artifact-page-fingerprint）用例：
//   - 指纹命中删除（含索引行联动）、不符/缺页保留、路径穿越拒绝；
//   - 三处清理路径（cleanupSplitArtifactsInDirByPage / removeSplitArtifactsBeyond /
//     cleanupStaleReplaceArtifacts）分别走指纹 allow-list；
//   - 指纹缺失（空串 / JSON 损坏 / 条目数与 page_count 不符）整体回落 mtime 守卫；
//   - 记录读写往返、重链键改写与目录迁移不重写指纹、备份快照携带指纹；
//   - OOXML 产物字节确定性（同一 fixture 连续拆两次逐页 SHA-256 相等）。

// splitFingerprintOfTest 按磁盘现状构造单页指纹（产品文件已落盘后用）。
func splitFingerprintOfTest(t *testing.T, path string, page int) splitArtifactFingerprint {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取指纹文件状态（%s）: %v", path, err)
	}
	sum, err := hashFileSHA256(path)
	if err != nil {
		t.Fatalf("计算指纹文件哈希（%s）: %v", path, err)
	}
	return splitArtifactFingerprint{
		Page: page, Name: filepath.Base(path), Size: info.Size(), SHA256: sum,
	}
}

// TestSplitFingerprints_RoundTrip 记录读写往返：非空指纹按页序读回、空清单读回为
// 空、损坏 JSON 读回为空且不返回错误（查询宽容语义）。
func TestSplitFingerprints_RoundTrip(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹往返库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	fps := []splitArtifactFingerprint{
		{Page: 1, Name: "讲义-001.pptx", Size: 11, SHA256: "aaaa"},
		{Page: 2, Name: "讲义-002.pptx", Size: 22, SHA256: "bbbb"},
	}
	if err := store.upsertSplitRecord("讲义.pptx", "h1", filepath.Join(dir, "讲义"), 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "讲义", false, fps, nil); err != nil {
		t.Fatalf("写入带指纹记录: %v", err)
	}
	rec, err := store.getSplitRecord("讲义.pptx")
	if err != nil || rec == nil {
		t.Fatalf("读记录: rec=%+v err=%v", rec, err)
	}
	if !reflect.DeepEqual(rec.fingerprints, fps) {
		t.Errorf("指纹往返不一致：got %+v want %+v", rec.fingerprints, fps)
	}

	// 覆盖为空指纹
	if err := store.upsertSplitRecord("讲义.pptx", "h2", filepath.Join(dir, "讲义"), 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "讲义", false, nil, nil); err != nil {
		t.Fatalf("覆盖空指纹: %v", err)
	}
	rec, err = store.getSplitRecord("讲义.pptx")
	if err != nil || rec == nil || len(rec.fingerprints) != 0 {
		t.Errorf("空指纹应读回空：rec=%+v err=%v", rec, err)
	}

	// 手工写坏 JSON：按无指纹处理，不返回错误（清理侧回落 mtime）
	if _, err := store.db.Exec(
		"UPDATE split_records SET artifact_fingerprints = ? WHERE source_rel_path = ?",
		"{not-json", "讲义.pptx",
	); err != nil {
		t.Fatalf("写坏指纹 JSON: %v", err)
	}
	rec, err = store.getSplitRecord("讲义.pptx")
	if err != nil {
		t.Fatalf("坏 JSON 不应返回错误: %v", err)
	}
	if rec == nil || len(rec.fingerprints) != 0 {
		t.Errorf("坏 JSON 应按无指纹处理：rec=%+v", rec)
	}
}

// TestCleanupSplitArtifactsInDirByPage_Fingerprint 指纹分支：
//   - 指纹四条件全等的产物删除并联动清理索引行；
//   - 同名但内容不同的用户文件（mtime 早于 split_at）保留；
//   - 不在指纹中的同形文件保留；
//   - 指纹命中非常规文件（目录）跳过；
//   - 指纹名含路径分隔符（路径穿越）拒绝且不删目录外文件。
func TestCleanupSplitArtifactsInDirByPage_Fingerprint(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹清理库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	outDir := filepath.Join(dir, "方案")
	product := filepath.Join(outDir, "方案-001.pptx")
	userSameName := filepath.Join(outDir, "方案-002.pptx")
	foreign := filepath.Join(outDir, "笔记-001.pptx")
	nonRegular := filepath.Join(outDir, "方案-003.pptx")
	writeSplitArtifactAt(t, product, "product-1", "2026-11-02 10:00:00")
	writeSplitArtifactAt(t, userSameName, "user-copy", "2026-11-02 10:00:00")
	testutil.WriteFile(t, foreign, "foreign")
	if err := os.MkdirAll(nonRegular, 0755); err != nil {
		t.Fatalf("创建同名目录: %v", err)
	}
	escapeTarget := filepath.Join(dir, "逃逸.pptx")
	testutil.WriteFile(t, escapeTarget, "escape")

	// page2 的指纹描述的是「旧产物内容」，与磁盘上的用户文件不同 → 保留
	fp1 := splitFingerprintOfTest(t, product, 1)
	fp2 := splitArtifactFingerprint{Page: 2, Name: "方案-002.pptx", Size: 9, SHA256: "0123456789abcdef"}
	fp3 := splitArtifactFingerprint{Page: 3, Name: "方案-003.pptx", Size: 1, SHA256: "ee"}
	fp4 := splitArtifactFingerprint{Page: 4, Name: "../逃逸.pptx", Size: 6, SHA256: "deadbeef"}
	record := &assetSplitRecord{
		sourceRelPath: "方案.pptx",
		outputDir:     outDir,
		pageCount:     4,
		splitAt:       "2026-11-02 10:00:00",
		target:        model.AssetSplitTargetSource,
		artifactStem:  "方案",
		fingerprints:  []splitArtifactFingerprint{fp1, fp2, fp3, fp4},
	}
	insertIndexRowForTest(t, svc, lib.ID, 9801, "方案/方案-001.pptx")

	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, product)
	mustExist(t, userSameName)
	mustExist(t, foreign)
	mustExist(t, nonRegular) // 目录（非常规文件）保留
	mustExist(t, escapeTarget)
	if n := countIndexRowsByPath(t, svc, lib.ID, "方案/方案-001.pptx"); n != 0 {
		t.Errorf("指纹命中产物索引行应被清理，实际 %d 行", n)
	}
}

// TestCleanupSplitArtifactsInDirByPage_FingerprintMissingPageKept 指纹可用但某页
// 无条目（页集合缺页）：该页保留，不回落 mtime 守卫（避免把用户文件按旧启发式删掉）。
func TestCleanupSplitArtifactsInDirByPage_FingerprintMissingPageKept(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "缺页指纹库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	outDir := filepath.Join(dir, "缺页")
	page1 := filepath.Join(outDir, "缺页-001.pptx")
	page2 := filepath.Join(outDir, "缺页-002.pptx")
	writeSplitArtifactAt(t, page1, "product-1", "2026-11-02 10:00:00")
	// page2 mtime 早于 split_at：若回落 mtime 守卫会被删除，必须因缺页指纹而保留
	writeSplitArtifactAt(t, page2, "user", "2026-11-02 10:00:00")

	fp1 := splitFingerprintOfTest(t, page1, 1)
	record := &assetSplitRecord{
		sourceRelPath: "缺页.pptx", outputDir: outDir, pageCount: 2,
		splitAt: "2026-11-02 10:00:00", target: model.AssetSplitTargetSource,
		artifactStem: "缺页",
		// len == pageCount（可用），但 page 2 无条目（页集合为 {1, 5}）
		fingerprints: []splitArtifactFingerprint{fp1, {Page: 5, Name: "缺页-005.pptx"}},
	}

	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, page1)
	mustExist(t, page2)
}

// TestRemoveSplitArtifactsBeyond_Fingerprint 指纹分支：页序 > keepPage 且指纹
// 命中的旧页删除并联动索引行；页序 ≤ keepPage 的当前形态产物由核心覆盖写保留；
// 不在指纹中的用户同形文件（含越界页序）一律保留。
func TestRemoveSplitArtifactsBeyond_Fingerprint(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "越界指纹库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	outDir := filepath.Join(dir, "轨迹")
	page1 := filepath.Join(outDir, "轨迹-001.pptx")
	page2 := filepath.Join(outDir, "轨迹-002.pptx")
	page3 := filepath.Join(outDir, "轨迹-003.pptx")
	userOutOfRange := filepath.Join(outDir, "轨迹-099.pptx")
	writeSplitArtifactAt(t, page1, "p1", "2026-11-02 10:00:00")
	writeSplitArtifactAt(t, page2, "p2", "2026-11-02 10:00:00")
	writeSplitArtifactAt(t, page3, "p3", "2026-11-02 10:00:00")
	writeSplitArtifactAt(t, userOutOfRange, "user", "2026-11-02 10:00:00")

	record := &assetSplitRecord{
		sourceRelPath: "轨迹.pptx", outputDir: outDir, pageCount: 3,
		splitAt: "2026-11-02 10:00:00", target: model.AssetSplitTargetCache,
		artifactStem: "轨迹",
		fingerprints: []splitArtifactFingerprint{
			splitFingerprintOfTest(t, page1, 1),
			splitFingerprintOfTest(t, page2, 2),
			splitFingerprintOfTest(t, page3, 3),
		},
	}
	insertIndexRowForTest(t, svc, lib.ID, 9802, "轨迹/轨迹-003.pptx")

	svc.removeSplitArtifactsBeyond(store, lib, record, outDir, "轨迹", 2, record.splitAt)

	mustExist(t, page1)
	mustExist(t, page2)
	mustNotExist(t, page3)
	mustExist(t, userOutOfRange)
	if n := countIndexRowsByPath(t, svc, lib.ID, "轨迹/轨迹-003.pptx"); n != 0 {
		t.Errorf("指纹删除的旧页索引行应被清理，实际 %d 行", n)
	}
}

// TestCleanupStaleReplaceArtifacts_Fingerprint replace 收尾：指纹命中的陈旧页
// 删除并联动索引行；不在指纹中的同形用户文件保留；本次新页（命中 newNames）保留。
func TestCleanupStaleReplaceArtifacts_Fingerprint(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换陈旧指纹库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	outDir := filepath.Join(dir, "替换")
	page1 := filepath.Join(outDir, "001-替换.pptx") // 本次新页（内容已覆盖）
	stalePage := filepath.Join(outDir, "002-替换.pptx")
	userFile := filepath.Join(outDir, "003-替换.pptx")
	writeSplitArtifactAt(t, page1, "new-1", "2026-11-02 10:00:00")
	writeSplitArtifactAt(t, stalePage, "old-2", "2026-11-02 10:00:00")
	writeSplitArtifactAt(t, userFile, "user-3", "2026-11-02 10:00:00")

	record := &assetSplitRecord{
		sourceRelPath: "替换.pptx", outputDir: outDir, pageCount: 2,
		splitAt: "2026-11-02 10:00:00", target: model.AssetSplitTargetReplace,
		artifactStem: "替换",
		fingerprints: []splitArtifactFingerprint{
			{Page: 1, Name: "001-替换.pptx", Size: 5, SHA256: "stale-page1-not-on-disk"},
			splitFingerprintOfTest(t, stalePage, 2),
		},
	}
	oldArtifacts := map[string]struct{}{
		"001-替换.pptx": {}, "002-替换.pptx": {}, "003-替换.pptx": {},
	}
	insertIndexRowForTest(t, svc, lib.ID, 9803, "替换/002-替换.pptx")

	svc.cleanupStaleReplaceArtifacts(store, lib, record, outDir, oldArtifacts,
		[]string{page1})

	mustExist(t, page1)
	mustNotExist(t, stalePage)
	mustExist(t, userFile)
	if n := countIndexRowsByPath(t, svc, lib.ID, "替换/002-替换.pptx"); n != 0 {
		t.Errorf("指纹删除的陈旧页索引行应被清理，实际 %d 行", n)
	}
}

// TestFingerprintsUnusableFallbackToMtime 指纹不可用（空 / 条目数与 page_count
// 不符）整体回落 mtime 守卫：早于 split_at 的产物删除、晚于的保留。
func TestFingerprintsUnusableFallbackToMtime(t *testing.T) {
	const splitAt = "2026-11-02 10:00:00"
	cases := []struct {
		name         string
		fingerprints []splitArtifactFingerprint
	}{
		{name: "空指纹"},
		{name: "条目数不符", fingerprints: []splitArtifactFingerprint{{Page: 1, Name: "形态-001.pptx", Size: 1, SHA256: "aa"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestAssetService(t)
			dir := t.TempDir()
			lib := mustCreateLibrary(t, svc, "回落库", dir)
			store := svc.openStoreForTest(t, lib.ID)

			outDir := filepath.Join(dir, "形态")
			oldProduct := filepath.Join(outDir, "形态-001.pptx")
			lateUser := filepath.Join(outDir, "形态-002.pptx")
			writeSplitArtifactAt(t, oldProduct, "product", splitAt)
			testutil.WriteFile(t, lateUser, "user") // mtime = now，晚于 splitAt

			record := &assetSplitRecord{
				sourceRelPath: "形态.pptx", outputDir: outDir, pageCount: 2,
				splitAt: splitAt, target: model.AssetSplitTargetSource,
				artifactStem: "形态", fingerprints: tc.fingerprints,
			}
			if fingerprintsUsable(record) {
				t.Fatalf("前置：该用例指纹应不可用")
			}
			svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

			mustNotExist(t, oldProduct)
			mustExist(t, lateUser)
		})
	}
}

// TestFingerprintForName_CaseFoldByPlatform 指纹名与候选文件名的等价判定走
// sameNameFold：Windows 折叠（同一物理文件），Linux/macOS 大小写敏感不折叠。
func TestFingerprintForName_CaseFoldByPlatform(t *testing.T) {
	record := &assetSplitRecord{
		fingerprints: []splitArtifactFingerprint{
			{Page: 1, Name: "Abc-001.pptx", Size: 1, SHA256: "aa"},
		},
	}
	_, ok := fingerprintForName(record, "abc-001.pptx")
	if runtime.GOOS == "windows" {
		if !ok {
			t.Error("Windows 应折叠大小写命中")
		}
		return
	}
	if ok {
		t.Error("非 Windows 大小写敏感，不应命中")
	}
}

// TestSplitFingerprintNameValid 指纹名合法判定：纯 basename 通过；空、`.`/`..`、
// 两种目录分隔符与含目录形态一律拒绝（防脏数据路径穿越）。
func TestSplitFingerprintNameValid(t *testing.T) {
	valid := []string{"讲义-001.pptx", "001-讲义.pptx", "2026.pptx"}
	for _, name := range valid {
		if !splitFingerprintNameValid(name) {
			t.Errorf("应判为合法：%q", name)
		}
	}
	invalid := []string{"", ".", "..", "../逃逸.pptx", `..\逃逸.pptx`, "子目录/产物.pptx", `子目录\产物.pptx`}
	for _, name := range invalid {
		if splitFingerprintNameValid(name) {
			t.Errorf("应判为非法：%q", name)
		}
	}
}

// TestSplitFingerprints_SurviveKeyRewriteAndFolderMigrate 重链源键改写与目录迁移
// 只改键/落点，不重写指纹（产物文件名不变）——迁移后指纹仍可用于清理判定。
func TestSplitFingerprints_SurviveKeyRewriteAndFolderMigrate(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹迁移库", root)
	store := svc.openStoreForTest(t, lib.ID)

	fps := []splitArtifactFingerprint{
		{Page: 1, Name: "迁移-001.pptx", Size: 7, SHA256: "cc"},
	}
	if err := store.upsertSplitRecord("旧目录/迁移.pptx", "h", filepath.Join(root, "旧目录", "迁移"), 1,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "迁移", false, fps, nil); err != nil {
		t.Fatalf("写入记录: %v", err)
	}
	// 目录级迁移独立记录：源键与落点都位于被迁移前缀下
	if err := store.upsertSplitRecord("待迁/文件.pptx", "h2", filepath.Join(root, "待迁", "文件"), 1,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "文件", false, fps, nil); err != nil {
		t.Fatalf("写入迁移记录: %v", err)
	}

	// 文件级重链：源键改写，落点保持原值（产物未随源移动）
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("开始事务: %v", err)
	}
	if _, err := store.rewriteSplitRecordKeyTx(tx, "旧目录/迁移.pptx", "新目录/迁移.pptx", ""); err != nil {
		_ = tx.Rollback()
		t.Fatalf("重链键改写: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("提交重链事务: %v", err)
	}
	rec, err := store.getSplitRecord("新目录/迁移.pptx")
	if err != nil || rec == nil {
		t.Fatalf("重链后读记录: rec=%+v err=%v", rec, err)
	}
	if !reflect.DeepEqual(rec.fingerprints, fps) {
		t.Errorf("重链不应重写指纹：got %+v want %+v", rec.fingerprints, fps)
	}

	// 目录级迁移：旧目录前缀 → 新前缀，记录键与落点同事务改写，指纹不变
	tx, err = store.db.Begin()
	if err != nil {
		t.Fatalf("开始迁移事务: %v", err)
	}
	if _, err := store.migrateSplitRecordsTx(tx, splitRecordMigration{
		libraryRoot: root, oldPrefix: "待迁", newPrefix: "归档",
	}); err != nil {
		_ = tx.Rollback()
		t.Fatalf("目录迁移: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("提交迁移事务: %v", err)
	}
	rec, err = store.getSplitRecord("归档/文件.pptx")
	if err != nil || rec == nil {
		t.Fatalf("目录迁移后读记录: rec=%+v err=%v", rec, err)
	}
	if !reflect.DeepEqual(rec.fingerprints, fps) {
		t.Errorf("目录迁移不应重写指纹：got %+v want %+v", rec.fingerprints, fps)
	}
	if !samePathFold(rec.outputDir, filepath.Join(root, "归档", "文件")) {
		t.Errorf("source 落点应随目录迁移重算，实际 %q", rec.outputDir)
	}
}

// TestSplitFingerprints_SurviveBackupSnapshot 备份为整库 VACUUM INTO 快照：指纹
// 随 split_records 行落盘，重开备份库直接可读（无需额外同步）。
func TestSplitFingerprints_SurviveBackupSnapshot(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹备份库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	fps := []splitArtifactFingerprint{
		{Page: 1, Name: "备份-001.pptx", Size: 3, SHA256: "dd"},
	}
	if err := store.upsertSplitRecord("备份.pptx", "h", filepath.Join(dir, "备份"), 1,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "备份", false, fps, nil); err != nil {
		t.Fatalf("写入记录: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := store.vacuumInto(backupPath); err != nil {
		t.Fatalf("VACUUM INTO 备份: %v", err)
	}
	backupStore, err := openAssetsStore(backupPath)
	if err != nil {
		t.Fatalf("打开备份库: %v", err)
	}
	t.Cleanup(func() { _ = backupStore.Close() })
	rec, err := backupStore.getSplitRecord("备份.pptx")
	if err != nil || rec == nil {
		t.Fatalf("备份库读记录: rec=%+v err=%v", rec, err)
	}
	if !reflect.DeepEqual(rec.fingerprints, fps) {
		t.Errorf("备份快照应携带指纹：got %+v want %+v", rec.fingerprints, fps)
	}
}

// TestAssetsStore_MigrateV10 v10 迁移：v9 库（无 artifact_fingerprints 列）升级
// 补列并钉版本；半迁移（列已存在 + user_version=9）可重开不撞 duplicate column。
func TestAssetsStore_MigrateV10(t *testing.T) {
	const v9SplitRecordsDDL = `
CREATE TABLE split_records (
	source_rel_path TEXT    PRIMARY KEY,
	source_hash     TEXT    NOT NULL,
	output_dir      TEXT    NOT NULL,
	page_count      INTEGER NOT NULL,
	split_at        TEXT    NOT NULL,
	target          TEXT    NOT NULL DEFAULT 'cache',
	source_removed  INTEGER NOT NULL DEFAULT 0,
	artifact_stem   TEXT    NOT NULL DEFAULT ''
);`
	newV9DB := func(t *testing.T, withFingerprintColumn bool) string {
		t.Helper()
		dbPath := filepath.Join(t.TempDir(), "v9.db")
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
		if err != nil {
			t.Fatalf("打开裸库: %v", err)
		}
		if _, err := db.Exec(v9SplitRecordsDDL); err != nil {
			_ = db.Close()
			t.Fatalf("建 v9 split_records: %v", err)
		}
		if withFingerprintColumn {
			if _, err := db.Exec("ALTER TABLE split_records ADD COLUMN artifact_fingerprints TEXT NOT NULL DEFAULT ''"); err != nil {
				_ = db.Close()
				t.Fatalf("预建指纹列: %v", err)
			}
		}
		// files 表补建：本测试最小库只关心 split_records 列迁移，但升级到当前版本
		// 会依次跑 v12（files.parse_error ADD COLUMN），缺表则迁移失败。最小结构
		// 即可满足 ALTER TABLE 前置条件，不参与本测试断言。
		if _, err := db.Exec("CREATE TABLE IF NOT EXISTS files (id INTEGER PRIMARY KEY AUTOINCREMENT, path TEXT NOT NULL UNIQUE)"); err != nil {
			_ = db.Close()
			t.Fatalf("建 files 最小表: %v", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 9"); err != nil {
			_ = db.Close()
			t.Fatalf("钉版本 9: %v", err)
		}
		// 存入一条 v9 期记录：升级后须原样保留，且新列必须为空（禁自动回填——现有
		// 目录文件可能已被用户替换，回填会把用户文件认证为产物、后续清理即误删）。
		if _, err := db.Exec(
			"INSERT INTO split_records(source_rel_path, source_hash, output_dir, page_count, split_at, target, source_removed, artifact_stem) " +
				"VALUES('存量.pptx', 'h-old', '旧落点', 2, '2026-11-02 10:00:00', 'source', 1, '存量')",
		); err != nil {
			_ = db.Close()
			t.Fatalf("插入存量 v9 记录: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("关闭裸库: %v", err)
		}
		return dbPath
	}

	for _, caseName := range []string{"v9 升级补列", "半迁移列已存在可重开"} {
		t.Run(caseName, func(t *testing.T) {
			dbPath := newV9DB(t, caseName == "半迁移列已存在可重开")
			store, err := openAssetsStore(dbPath)
			if err != nil {
				t.Fatalf("升级打开: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			var n int
			if err := store.db.QueryRow(
				"SELECT COUNT(*) FROM pragma_table_info('split_records') WHERE name = 'artifact_fingerprints'",
			).Scan(&n); err != nil {
				t.Fatalf("查列信息: %v", err)
			}
			if n != 1 {
				t.Errorf("v10 迁移后应有 artifact_fingerprints 列，实际 %d", n)
			}
			var version int
			if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != assetsSchemaVersion {
				t.Errorf("版本应钉到 v%d，实际 v%d（err=%v）", assetsSchemaVersion, version, err)
			}
			// 存量行原样保留，且指纹列为空——迁移只补列，禁自动回填（回填会把
			// 目录内现存文件认证为产物，后续清理即误删用户数据）。
			rec, recErr := store.getSplitRecord("存量.pptx")
			if recErr != nil || rec == nil {
				t.Fatalf("迁移后存量记录应可读：rec=%+v err=%v", rec, recErr)
			}
			if rec.artifactStem != "存量" || !rec.sourceRemoved || rec.pageCount != 2 || rec.outputDir != "旧落点" {
				t.Errorf("迁移不应改动存量字段：%+v", rec)
			}
			if len(rec.fingerprints) != 0 {
				t.Errorf("存量行迁移后不得自动回填指纹，实际 %+v", rec.fingerprints)
			}
		})
	}
}

// TestAssetSplit_RealSplitFingerprintsProtectUserFiles 端到端验收：真实拆分写入
// 指纹；源改名重链后重拆，旧产物目录中指纹命中的旧页删除，被用户替换内容的同
// 名文件与用户自建异构文件（任意 mtime）保留。
func TestAssetSplit_RealSplitFingerprintsProtectUserFiles(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "真实拆分指纹库", dir)
	buildAssetsPptx(t, dir, "旧名.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "旧名.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "旧名.pptx", "source"); err != nil {
		t.Fatalf("首次拆分: %v", err)
	}
	oldOut := filepath.Join(dir, "旧名")
	rec := splitRecordRowForTest(t, svc, lib.ID, "旧名.pptx")
	if rec == nil || len(rec.fingerprints) != 2 {
		t.Fatalf("首次拆分应写入 2 条页级指纹，实际 %+v", rec)
	}
	for _, fp := range rec.fingerprints {
		if !fingerprintMatch(filepath.Join(oldOut, fp.Name), fp) {
			t.Errorf("指纹应与首次拆分产物字节一致：%+v", fp)
		}
	}

	// 源改名移动到新目录并重链：记录键迁走，output_dir 冻结旧目录
	newDir := filepath.Join(dir, "新目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "新名.pptx")
	if err := os.Rename(filepath.Join(dir, "旧名.pptx"), newAbs); err != nil {
		t.Fatalf("改名移动源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}

	// 用户把旧第 1 页产物替换为自建内容（mtime 回拨到 split_at 之前，旧启发式
	// 会误删），并放入自建异构文件
	userPage1 := filepath.Join(oldOut, "旧名-001.pptx")
	writeSplitArtifactAt(t, userPage1, "user-content", rec.splitAt)
	draft := filepath.Join(oldOut, "旧名-草稿.pptx")
	testutil.WriteFile(t, draft, "draft")

	// 源内容变化（2 页，内容不同）并同步库存哈希，驱动真实重拆
	buildAssetsPptx(t, newDir, "新名.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"新内容"}},
		{titlePh: "新二", body: []string{"新内容"}},
	})
	newHash, err := hashFileSHA256(newAbs)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, "新目录/新名.pptx"); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "新目录/新名.pptx", "source")
	if err != nil {
		t.Fatalf("重拆: %v", err)
	}
	if res.Skipped || res.PageCount != 2 {
		t.Fatalf("应完成 2 页重拆：%+v", res)
	}

	mustExist(t, userPage1)
	mustExist(t, draft)
	mustNotExist(t, filepath.Join(oldOut, "旧名-002.pptx"))
	newOut := filepath.Join(newDir, "新名")
	for _, page := range []int{1, 2} {
		mustExist(t, filepath.Join(newOut, fmt.Sprintf("新名-%03d.pptx", page)))
	}
}

// TestSplitPptxPerPage_ByteDeterminism OOXML 产物字节确定性证据：同一 fixture
// 连续拆分两次，逐页 SHA-256 相等（writeZipAtomic 只保留源 zip 条目的 Modified/
// Method 与顺序，不写当前时间；指纹基线的长期有效性依赖该性质）。
func TestSplitPptxPerPage_ByteDeterminism(t *testing.T) {
	dir := t.TempDir()
	src := buildAssetsPptx(t, dir, "确定性.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容一"}},
		{titlePh: "二", body: []string{"内容二"}},
		{titlePh: "三", body: []string{"内容三"}},
	})

	hashPages := func() map[string]string {
		t.Helper()
		result, err := SplitPptxPerPage(context.Background(), src)
		if err != nil {
			t.Fatalf("拆分: %v", err)
		}
		if result.PageCount == 0 || len(result.Files) == 0 {
			t.Fatalf("拆分应产出页文件：%+v", result)
		}
		out := make(map[string]string, len(result.Files))
		for _, p := range result.Files {
			sum, err := hashFileSHA256(p)
			if err != nil {
				t.Fatalf("计算产物哈希（%s）: %v", p, err)
			}
			out[filepath.Base(p)] = sum
		}
		return out
	}

	first := hashPages()
	second := hashPages()
	if !reflect.DeepEqual(first, second) {
		t.Errorf("同一源连续拆分两次逐页 SHA-256 应相等：first=%v second=%v", first, second)
	}
}
