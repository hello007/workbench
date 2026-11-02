package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"workbench/model"
)

// ---------- schema v11：retained_fingerprints（清理失败保留归属） ----------
//
// 背景：replace 同目录旧页收尾删除失败时，文件与索引行按安全侧保留，但记录随即
// 被本次新页数覆盖——残留页脱离 artifact_fingerprints 的 allow-list，后续增页重拆
// 会把它当「用户同名文件」永久拒绝。v11 独立 retained_fingerprints 列承载「清理
// 失败但仍属本记录历史产物」的凭证，由后续重拆的备份让位 / 清理消费，消费成功后
// 不再携带（自愈）。指纹列契约（条目数 = page_count）保持不变。

// TestSplitRetained_RoundTrip 记录读写往返携带 retained：非空按页序读回、覆盖为空
// 即清空（自愈），splitRecordByOutputDir 反查同样携带。
func TestSplitRetained_RoundTrip(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "保留凭证往返库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	fps := []splitArtifactFingerprint{
		{Page: 1, Name: "讲义-001.pptx", Size: 11, SHA256: "aaaa"},
		{Page: 2, Name: "讲义-002.pptx", Size: 22, SHA256: "bbbb"},
	}
	retained := []splitArtifactFingerprint{
		{Page: 3, Name: "讲义-003.pptx", Size: 33, SHA256: "cccc"},
	}
	outDir := filepath.Join(dir, "讲义")
	if err := store.upsertSplitRecord("讲义.pptx", "h1", outDir, 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetReplace, "讲义", false, fps, retained); err != nil {
		t.Fatalf("写入带 retained 记录: %v", err)
	}
	rec, err := store.getSplitRecord("讲义.pptx")
	if err != nil || rec == nil {
		t.Fatalf("读记录: rec=%+v err=%v", rec, err)
	}
	if !reflect.DeepEqual(rec.fingerprints, fps) {
		t.Errorf("指纹往返不一致：got %+v want %+v", rec.fingerprints, fps)
	}
	if !reflect.DeepEqual(rec.retained, retained) {
		t.Errorf("retained 往返不一致：got %+v want %+v", rec.retained, retained)
	}

	// 反查路径同样携带 retained（splitRecordByOutputDir select/scan 同步）
	other, err := store.splitRecordByOutputDir(outDir, "其他.pptx")
	if err != nil || other == nil {
		t.Fatalf("按落点反查记录: rec=%+v err=%v", other, err)
	}
	if !reflect.DeepEqual(other.retained, retained) {
		t.Errorf("反查记录应携带 retained：got %+v want %+v", other.retained, retained)
	}

	// 覆盖为空 = 自愈清空
	if err := store.upsertSplitRecord("讲义.pptx", "h2", outDir, 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetReplace, "讲义", true, fps, nil); err != nil {
		t.Fatalf("覆盖清空 retained: %v", err)
	}
	rec, err = store.getSplitRecord("讲义.pptx")
	if err != nil || rec == nil || len(rec.retained) != 0 {
		t.Errorf("retained 应被清空：rec=%+v err=%v", rec, err)
	}
}

// TestOwnershipFingerprintForName 归属查询单点：先查本次指纹、未命中再查 retained；
// 两清单均要求按名唯一命中，多命中不可判定（不猜）。
func TestOwnershipFingerprintForName(t *testing.T) {
	record := &assetSplitRecord{
		fingerprints: []splitArtifactFingerprint{{Page: 1, Name: "a.pptx", Size: 1, SHA256: "fp-a"}},
		retained:     []splitArtifactFingerprint{{Page: 3, Name: "c.pptx", Size: 3, SHA256: "retained-c"}},
	}
	if fp, ok := ownershipFingerprintForName(record, "a.pptx"); !ok || fp.SHA256 != "fp-a" {
		t.Errorf("指纹命中优先：fp=%+v ok=%v", fp, ok)
	}
	if fp, ok := ownershipFingerprintForName(record, "c.pptx"); !ok || fp.SHA256 != "retained-c" {
		t.Errorf("指纹未命中应回落 retained：fp=%+v ok=%v", fp, ok)
	}
	if _, ok := ownershipFingerprintForName(record, "b.pptx"); ok {
		t.Error("两清单均无条目应为不可判定")
	}
	if _, ok := ownershipFingerprintForName(nil, "a.pptx"); ok {
		t.Error("record 为 nil 应为不可判定")
	}

	multi := &assetSplitRecord{
		fingerprints: []splitArtifactFingerprint{
			{Page: 1, Name: "x.pptx", Size: 1, SHA256: "1"},
			{Page: 2, Name: "x.pptx", Size: 1, SHA256: "2"},
		},
	}
	if _, ok := ownershipFingerprintForName(multi, "x.pptx"); ok {
		t.Error("指纹同名多命中应为不可判定（不猜）")
	}

	// 指纹多命中 → 视为未命中，回落 retained 唯一命中（契约：先指纹后 retained）
	mixed := &assetSplitRecord{
		fingerprints: multi.fingerprints,
		retained:     []splitArtifactFingerprint{{Page: 3, Name: "x.pptx", Size: 9, SHA256: "r"}},
	}
	if fp, ok := ownershipFingerprintForName(mixed, "x.pptx"); !ok || fp.SHA256 != "r" {
		t.Errorf("指纹多命中应回落 retained 唯一命中：fp=%+v ok=%v", fp, ok)
	}
}

// TestAssetsStore_MigrateV11 v11 迁移：v10 库（无 retained_fingerprints 列）升级
// 补列并钉版本，存量行原样保留且新列默认空串（禁自动回填）；半迁移（列已存在 +
// user_version=10）可重开不撞 duplicate column。
func TestAssetsStore_MigrateV11(t *testing.T) {
	const v10SplitRecordsDDL = `
CREATE TABLE split_records (
	source_rel_path TEXT    PRIMARY KEY,
	source_hash     TEXT    NOT NULL,
	output_dir      TEXT    NOT NULL,
	page_count      INTEGER NOT NULL,
	split_at        TEXT    NOT NULL,
	target          TEXT    NOT NULL DEFAULT 'cache',
	source_removed  INTEGER NOT NULL DEFAULT 0,
	artifact_stem   TEXT    NOT NULL DEFAULT '',
	artifact_fingerprints TEXT NOT NULL DEFAULT ''
);`
	const seedFingerprints = `[{"page":1,"name":"存量-001.pptx","size":1,"sha256":"aa"}]`

	newV10DB := func(t *testing.T, withRetainedColumn bool) string {
		t.Helper()
		dbPath := filepath.Join(t.TempDir(), "v10.db")
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
		if err != nil {
			t.Fatalf("打开裸库: %v", err)
		}
		if _, err := db.Exec(v10SplitRecordsDDL); err != nil {
			_ = db.Close()
			t.Fatalf("建 v10 split_records: %v", err)
		}
		if withRetainedColumn {
			if _, err := db.Exec("ALTER TABLE split_records ADD COLUMN retained_fingerprints TEXT NOT NULL DEFAULT ''"); err != nil {
				_ = db.Close()
				t.Fatalf("预建 retained 列: %v", err)
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 10"); err != nil {
			_ = db.Close()
			t.Fatalf("钉版本 10: %v", err)
		}
		if _, err := db.Exec(
			"INSERT INTO split_records(source_rel_path, source_hash, output_dir, page_count, split_at, target, source_removed, artifact_stem, artifact_fingerprints) "+
				"VALUES('存量.pptx', 'h-old', '旧落点', 1, '2026-11-02 10:00:00', 'replace', 0, '存量', ?)",
			seedFingerprints,
		); err != nil {
			_ = db.Close()
			t.Fatalf("插入存量 v10 记录: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("关闭裸库: %v", err)
		}
		return dbPath
	}

	for _, caseName := range []string{"v10 升级补列", "半迁移列已存在可重开"} {
		t.Run(caseName, func(t *testing.T) {
			dbPath := newV10DB(t, caseName == "半迁移列已存在可重开")
			store, err := openAssetsStore(dbPath)
			if err != nil {
				t.Fatalf("升级打开: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			var n int
			if err := store.db.QueryRow(
				"SELECT COUNT(*) FROM pragma_table_info('split_records') WHERE name = 'retained_fingerprints'",
			).Scan(&n); err != nil {
				t.Fatalf("查列信息: %v", err)
			}
			if n != 1 {
				t.Errorf("v11 迁移后应有 retained_fingerprints 列，实际 %d", n)
			}
			var version int
			if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != assetsSchemaVersion {
				t.Errorf("版本应钉到 v%d，实际 v%d（err=%v）", assetsSchemaVersion, version, err)
			}
			// 存量行原样保留，指纹不动，retained 默认空（禁自动回填）
			rec, recErr := store.getSplitRecord("存量.pptx")
			if recErr != nil || rec == nil {
				t.Fatalf("迁移后存量记录应可读：rec=%+v err=%v", rec, recErr)
			}
			if rec.artifactStem != "存量" || rec.pageCount != 1 || rec.outputDir != "旧落点" || rec.target != model.AssetSplitTargetReplace {
				t.Errorf("迁移不应改动存量字段：%+v", rec)
			}
			if len(rec.fingerprints) != 1 || rec.fingerprints[0].Name != "存量-001.pptx" {
				t.Errorf("迁移不应改动指纹：%+v", rec.fingerprints)
			}
			if len(rec.retained) != 0 {
				t.Errorf("存量行迁移后不得自动回填 retained，实际 %+v", rec.retained)
			}
		})
	}
}

// TestAssetSplitReplace_CleanupFailureRetainsOwnershipAndResplitSucceeds 核心回归
// （R2）：旧页清理删除失败 → 该页归属凭证写入 retained_fingerprints；随后页数恢复
// 重拆时备份让位按 retained 认定归属并成功，新记录不再携带该条目（自愈清空）。
func TestAssetSplitReplace_CleanupFailureRetainsOwnershipAndResplitSucceeds(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "清理失败留存库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "留存.pptx", 3)
	store := svc.openStoreForTest(t, lib.ID)

	// 源改为 2 页并同步库存哈希（驱动页数减少重拆，删除第 3 页旧产物）
	buildAssetsPptx(t, dir, "留存.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"内容"}},
		{titlePh: "新二", body: []string{"内容"}},
	})
	newHash, err := hashFileSHA256(src)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, "留存.pptx"); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}

	// 删除注入失败 + 源占用（保留源，走 8b 删源失败补写记录分支）
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	svc.removeArtifactFn = func(string) error { return errors.New("占用中") }
	svc.moveToRecycleBinFn = func(string) error { return errors.New("源占用中") }

	outcome, err := resplitReplaceForTest(t, svc, lib, "留存.pptx")
	if err != nil {
		t.Fatalf("删源失败属业务结果（nil error），实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed || outcome.code != model.ErrCodeAssetsSplitSourceDeleteFailed {
		t.Fatalf("删源失败应返回 E_ASSETS_SPLIT_SOURCE_DELETE_FAILED：%+v", outcome)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "留存.pptx")
	if rec == nil || rec.pageCount != 2 || len(rec.fingerprints) != 2 {
		t.Fatalf("重拆后记录应覆盖为 2 页指纹：%+v", rec)
	}
	if len(rec.retained) != 1 || rec.retained[0].Page != 3 || rec.retained[0].Name != "003-留存.pptx" {
		t.Fatalf("删除失败页归属应写入 retained：%+v", rec.retained)
	}
	// 删除失败页文件与索引行保留
	stalePage := filepath.Join(dir, "留存", "003-留存.pptx")
	if got, readErr := os.ReadFile(stalePage); readErr != nil || string(got) != "old-bytes" {
		t.Fatalf("删除失败页应原样保留（err=%v, content=%q）", readErr, got)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "留存/003-留存.pptx"); n != 1 {
		t.Fatalf("删除失败页索引行应保留，实际 %d 行", n)
	}

	// 解除注入失败；源恢复为 3 页（内容变化）并同步哈希 → 页数恢复重拆
	svc.removeArtifactFn = nil
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	buildAssetsPptx(t, dir, "留存.pptx", []assetsSlideSpec{
		{titlePh: "三一", body: []string{"内容"}},
		{titlePh: "三二", body: []string{"内容"}},
		{titlePh: "三三", body: []string{"内容"}},
	})
	newHash, err = hashFileSHA256(src)
	if err != nil {
		t.Fatalf("算三页哈希: %v", err)
	}
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, "留存.pptx"); err != nil {
		t.Fatalf("更新三页库存哈希: %v", err)
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("third-bytes")}).split

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "留存.pptx", "replace")
	if err != nil {
		t.Fatalf("retained 归属命中应允许重拆成功：%v", err)
	}
	if res.Skipped || !res.SourceRemoved || res.PageCount != 3 {
		t.Fatalf("页数恢复重拆应成功删源：%+v", res)
	}
	rec = splitRecordRowForTest(t, svc, lib.ID, "留存.pptx")
	if rec == nil || len(rec.fingerprints) != 3 {
		t.Fatalf("重拆成功应写入 3 页指纹：%+v", rec)
	}
	if len(rec.retained) != 0 {
		t.Errorf("消费 retained 后新记录不应再携带（自愈）：%+v", rec.retained)
	}
	if got, readErr := os.ReadFile(stalePage); readErr != nil || string(got) != "third-bytes" {
		t.Errorf("第 3 页应按新内容占位（err=%v, content=%q）", readErr, got)
	}
	assertNoReplaceBackup(t, filepath.Join(dir, "留存"))
	if got := recycleCalls(); len(got) != 1 || got[0] != src {
		t.Errorf("重拆成功应把源移入回收站一次，实际 %v", got)
	}
}
