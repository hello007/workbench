package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
// 两清单均要求按名唯一命中，多命中不可判定（不猜）；retained 还须目录锚一致。
func TestOwnershipFingerprintForName(t *testing.T) {
	const libDir = "/lib"
	record := &assetSplitRecord{
		outputDir:    libDir,
		fingerprints: []splitArtifactFingerprint{{Page: 1, Name: "a.pptx", Size: 1, SHA256: "fp-a"}},
		retained:     []splitArtifactFingerprint{{Page: 3, Name: "c.pptx", Size: 3, SHA256: "retained-c", Dir: libDir}},
	}
	if fp, ok := ownershipFingerprintForName(record, libDir, "a.pptx"); !ok || fp.SHA256 != "fp-a" {
		t.Errorf("指纹命中优先：fp=%+v ok=%v", fp, ok)
	}
	if fp, ok := ownershipFingerprintForName(record, libDir, "c.pptx"); !ok || fp.SHA256 != "retained-c" {
		t.Errorf("指纹未命中应回落 retained：fp=%+v ok=%v", fp, ok)
	}
	if _, ok := ownershipFingerprintForName(record, libDir, "b.pptx"); ok {
		t.Error("两清单均无条目应为不可判定")
	}
	if _, ok := ownershipFingerprintForName(record, "/lib/other", "c.pptx"); ok {
		t.Error("retained 目录锚不匹配时应为不可判定（同形文件属其他落点）")
	}
	if _, ok := ownershipFingerprintForName(nil, libDir, "a.pptx"); ok {
		t.Error("record 为 nil 应为不可判定")
	}

	multi := &assetSplitRecord{
		fingerprints: []splitArtifactFingerprint{
			{Page: 1, Name: "x.pptx", Size: 1, SHA256: "1"},
			{Page: 2, Name: "x.pptx", Size: 1, SHA256: "2"},
		},
	}
	if _, ok := ownershipFingerprintForName(multi, libDir, "x.pptx"); ok {
		t.Error("指纹同名多命中应为不可判定（不猜）")
	}

	// 指纹多命中 → 视为未命中，回落 retained 唯一命中（契约：先指纹后 retained）
	mixed := &assetSplitRecord{
		fingerprints: multi.fingerprints,
		retained:     []splitArtifactFingerprint{{Page: 3, Name: "x.pptx", Size: 9, SHA256: "r", Dir: libDir}},
	}
	if fp, ok := ownershipFingerprintForName(mixed, libDir, "x.pptx"); !ok || fp.SHA256 != "r" {
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
		// files 表补建：本测试最小库只关心 split_records 列迁移，但升级到当前版本
		// 会依次跑 v12（files.parse_error ADD COLUMN），缺表则迁移失败。最小结构
		// 即可满足 ALTER TABLE 前置条件，不参与本测试断言。
		if _, err := db.Exec("CREATE TABLE IF NOT EXISTS files (id INTEGER PRIMARY KEY AUTOINCREMENT, path TEXT NOT NULL UNIQUE)"); err != nil {
			_ = db.Close()
			t.Fatalf("建 files 最小表: %v", err)
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

// prepareRetainedStaleFixture 构造「旧记录 3 页 → 源减为 2 页重拆、第 3 页清理失败 +
// 删源失败」的夹具：返回源路径与残留页路径，记录 target=replace、pageCount=2、
// retained=[第 3 页]。调用方随后可切 target=source 驱动跨落点消费。
func prepareRetainedStaleFixture(t *testing.T, svc *AssetLibraryService, lib model.AssetLibrary, dir, name string) (src, stalePage string) {
	t.Helper()
	src, _ = resplitFixture(t, svc, lib, dir, name, 3)
	store := svc.openStoreForTest(t, lib.ID)
	// 源改为 2 页并同步库存哈希（驱动页数减少重拆）
	buildAssetsPptx(t, dir, name, []assetsSlideSpec{
		{titlePh: "新一", body: []string{"内容"}},
		{titlePh: "新二", body: []string{"内容"}},
	})
	newHash, err := hashFileSHA256(src)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, name); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("two-bytes")}).split
	svc.removeArtifactFn = func(string) error { return errors.New("占用中") }
	svc.moveToRecycleBinFn = func(string) error { return errors.New("源占用中") }

	outcome, err := resplitReplaceForTest(t, svc, lib, name)
	if err != nil {
		t.Fatalf("删源失败属业务结果（nil error），实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed || outcome.code != model.ErrCodeAssetsSplitSourceDeleteFailed {
		t.Fatalf("删源失败应返回 E_ASSETS_SPLIT_SOURCE_DELETE_FAILED：%+v", outcome)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, name)
	if rec == nil || rec.pageCount != 2 || len(rec.retained) != 1 || rec.retained[0].Page != 3 {
		t.Fatalf("前置：记录应为 2 页且 retained 含第 3 页，实际 %+v", rec)
	}
	stalePage = filepath.Join(dir, splitSourceBaseName(name), rec.retained[0].Name)
	if _, statErr := os.Stat(stalePage); statErr != nil {
		t.Fatalf("前置：残留页应仍在磁盘（%s）：%v", stalePage, statErr)
	}
	return src, stalePage
}

// TestAssetSplitRetained_CrossTargetConsumeAndSwitchBack R1 端到端：replace 残留页
// （retained）在切 target=source（同落点）的成功拆分中被消费——文件删除、索引行
// 清理、retained 清空；再切回 replace 不再被按名永久拒绝。
func TestAssetSplitRetained_CrossTargetConsumeAndSwitchBack(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "跨落点消费库", dir)
	src, stalePage := prepareRetainedStaleFixture(t, svc, lib, dir, "跨落点.pptx")
	if n := countIndexRowsByPath(t, svc, lib.ID, "跨落点/003-跨落点.pptx"); n != 1 {
		t.Fatalf("前置：残留页索引行应存在，实际 %d 行", n)
	}

	// 切 target=source（同落点）：解除删除失败注入，核心改产 2 页源形态
	svc.removeArtifactFn = nil
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("src-bytes")}).split
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "跨落点.pptx", "source")
	if err != nil {
		t.Fatalf("切 target=source 应成功：%v", err)
	}
	if res.Skipped || res.Target != model.AssetSplitTargetSource {
		t.Fatalf("落点切换不得跳过：%+v", res)
	}
	if _, statErr := os.Stat(stalePage); !os.IsNotExist(statErr) {
		t.Errorf("切 target 后残留页应被消费删除，stat err=%v", statErr)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "跨落点/003-跨落点.pptx"); n != 0 {
		t.Errorf("残留页索引行应被清理，实际 %d 行", n)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "跨落点.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetSource {
		t.Fatalf("记录应切到 source：%+v", rec)
	}
	if len(rec.retained) != 0 {
		t.Errorf("消费后 retained 应清空：%+v", rec.retained)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("source 落点保留源文件：%v", statErr)
	}

	// 再切回 replace：旧形态产物（源形态）不应触发按名拒绝
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("final-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	res, err = svc.SplitAssetFile(context.Background(), lib.ID, "跨落点.pptx", "replace")
	if err != nil {
		t.Fatalf("切回 replace 不得被按名拒绝：%v", err)
	}
	if res.Skipped || !res.SourceRemoved || res.PageCount != 2 {
		t.Fatalf("切回 replace 应成功删源：%+v", res)
	}
	rec = splitRecordRowForTest(t, svc, lib.ID, "跨落点.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetReplace || len(rec.retained) != 0 {
		t.Fatalf("最终记录应为 replace 且无 retained：%+v", rec)
	}
	for _, name := range []string{"001-跨落点.pptx", "002-跨落点.pptx"} {
		if got, readErr := os.ReadFile(filepath.Join(dir, "跨落点", name)); readErr != nil || string(got) != "final-bytes" {
			t.Errorf("最终产物应按新内容落盘（%s, err=%v, content=%q）", name, readErr, got)
		}
	}
	assertNoReplaceBackup(t, filepath.Join(dir, "跨落点"))
	if got := recycleCalls(); len(got) != 1 || got[0] != src {
		t.Errorf("切回 replace 成功应把源移入回收站一次，实际 %v", got)
	}
}

// TestAssetSplitRetained_CrossTargetConsumeFailureKeepsRetained R1：切 target 后再次
// 删除失败 → 新记录继续携带 retained（不静默清空）；再切回 replace 时旧残留页 +
// 旧形态产物经 replaceOutDirOwnedByRecord 认定为本记录所有，正常重拆并最终消费。
func TestAssetSplitRetained_CrossTargetConsumeFailureKeepsRetained(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "跨落点留存库", dir)
	src, stalePage := prepareRetainedStaleFixture(t, svc, lib, dir, "再留存.pptx")

	// 切 target=source，但删除注入仍失败：retained 继续保留
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("src-fail")}).split
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "再留存.pptx", "source")
	if err != nil {
		t.Fatalf("删除失败不阻断拆分，实际：%v", err)
	}
	if res.Skipped {
		t.Fatalf("落点切换不得跳过：%+v", res)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "再留存.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetSource || len(rec.retained) != 1 {
		t.Fatalf("删除失败时 retained 应随新记录保留：%+v", rec)
	}
	if _, statErr := os.Stat(stalePage); statErr != nil {
		t.Fatalf("删除失败时残留页应保留：%v", statErr)
	}

	// 解除失败；切回 replace：残留页 + 源形态产物均由归属凭证放行
	svc.removeArtifactFn = nil
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("final-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	res, err = svc.SplitAssetFile(context.Background(), lib.ID, "再留存.pptx", "replace")
	if err != nil {
		t.Fatalf("切回 replace 不得被按名拒绝：%v", err)
	}
	if res.Skipped || !res.SourceRemoved || res.PageCount != 2 {
		t.Fatalf("切回 replace 应成功删源：%+v", res)
	}
	if _, statErr := os.Stat(stalePage); !os.IsNotExist(statErr) {
		t.Errorf("切回 replace 后残留页应被消费，stat err=%v", statErr)
	}
	rec = splitRecordRowForTest(t, svc, lib.ID, "再留存.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetReplace || len(rec.retained) != 0 {
		t.Fatalf("最终记录应为 replace 且 retained 清空：%+v", rec)
	}
	assertNoReplaceBackup(t, filepath.Join(dir, "再留存"))
	if got := recycleCalls(); len(got) != 1 || got[0] != src {
		t.Errorf("成功应把源移入回收站一次，实际 %v", got)
	}
}

// TestSplitRetained_SurviveKeyRewriteFolderMigrateAndBackup retained 随记录行迁移：
// 文件级重链与目录级迁移只改键/落点、不重写凭证；备份快照（VACUUM INTO）整行携带，
// 重开备份库可读。
func TestSplitRetained_SurviveKeyRewriteFolderMigrateAndBackup(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "保留凭证迁移库", root)
	store := svc.openStoreForTest(t, lib.ID)

	retained := []splitArtifactFingerprint{
		{Page: 3, Name: "003-迁移.pptx", Size: 9, SHA256: "ee"},
	}
	if err := store.upsertSplitRecord("旧目录/迁移.pptx", "h", filepath.Join(root, "旧目录", "迁移"), 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetReplace, "迁移", false, nil, retained); err != nil {
		t.Fatalf("写入带 retained 记录: %v", err)
	}
	if err := store.upsertSplitRecord("待迁/文件.pptx", "h2", filepath.Join(root, "待迁", "文件"), 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetReplace, "文件", false, nil, retained); err != nil {
		t.Fatalf("写入迁移记录: %v", err)
	}

	// 文件级重链：源键改写，retained 原样
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
	if !reflect.DeepEqual(rec.retained, retained) {
		t.Errorf("重链不应重写 retained：got %+v want %+v", rec.retained, retained)
	}

	// 目录级迁移：键与落点同事务改写，retained 不变
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
	if !reflect.DeepEqual(rec.retained, retained) {
		t.Errorf("目录迁移不应重写 retained：got %+v want %+v", rec.retained, retained)
	}

	// 备份快照：retained 随整行落盘，重开备份库可读
	backupPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := store.vacuumInto(backupPath); err != nil {
		t.Fatalf("VACUUM INTO 备份: %v", err)
	}
	backupStore, err := openAssetsStore(backupPath)
	if err != nil {
		t.Fatalf("打开备份库: %v", err)
	}
	t.Cleanup(func() { _ = backupStore.Close() })
	rec, err = backupStore.getSplitRecord("归档/文件.pptx")
	if err != nil || rec == nil {
		t.Fatalf("备份库读记录: rec=%+v err=%v", rec, err)
	}
	if !reflect.DeepEqual(rec.retained, retained) {
		t.Errorf("备份快照应携带 retained：got %+v want %+v", rec.retained, retained)
	}
}

// TestCleanupRetainedSplitArtifacts_States 消费单点状态直测：条目按自身目录锚
// 消费（record.outputDir 已改也不影响）；
// 内容全等删除并清索引行；内容不符/文件不存在/名字非法丢弃条目；删除失败保留。
func TestCleanupRetainedSplitArtifacts_States(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "保留消费单点库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "单点")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	matchPath := filepath.Join(outDir, "003-单点.pptx")
	if err := os.WriteFile(matchPath, []byte("keep-bytes"), 0644); err != nil {
		t.Fatalf("写残留页: %v", err)
	}
	matchFP := splitFingerprintOfTest(t, matchPath, 3)
	matchFP.Dir = outDir
	mismatchPath := filepath.Join(outDir, "004-单点.pptx")
	if err := os.WriteFile(mismatchPath, []byte("userbytes"), 0644); err != nil {
		t.Fatalf("写用户文件: %v", err)
	}
	mismatchFP := splitFingerprintOfTest(t, mismatchPath, 4)
	mismatchFP.SHA256 = strings.Repeat("0", 64) // 等长不同哈希
	mismatchFP.Dir = outDir
	insertIndexRowForTest(t, svc, lib.ID, 9903, "单点/003-单点.pptx")

	record := &assetSplitRecord{
		sourceRelPath: "单点.pptx", outputDir: outDir, pageCount: 2,
		target: model.AssetSplitTargetReplace,
		retained: []splitArtifactFingerprint{
			matchFP, mismatchFP,
			{Page: 5, Name: "../逃逸.pptx", Size: 1, SHA256: "x", Dir: outDir},
			{Page: 6, Name: "006-库外.pptx", Size: 1, SHA256: "x", Dir: filepath.Join(dir, "..", "库外")},
		},
	}
	// 目录锚优先于 record.outputDir：记录落点改到别处后，条目仍按自身目录消费。
	other := &assetSplitRecord{outputDir: filepath.Join(dir, "别处"), retained: record.retained}

	// 删除失败：保留该条目（只保留内容全等且删除失败的）
	svc.removeArtifactFn = func(string) error { return errors.New("占用中") }
	got := svc.cleanupRetainedSplitArtifacts(store, lib, other, "")
	if len(got) != 1 || got[0].Name != matchFP.Name || !samePathFold(got[0].Dir, outDir) {
		t.Errorf("删除失败应仅保留内容全等条目且目录锚不变：%+v", got)
	}
	mustExist(t, matchPath)

	// 解除失败：删除成功、条目丢弃并清索引行；内容不符与非法名/非法目录丢弃
	svc.removeArtifactFn = nil
	got = svc.cleanupRetainedSplitArtifacts(store, lib, other, "")
	if len(got) != 0 {
		t.Errorf("全部消费后不应保留条目：%+v", got)
	}
	mustNotExist(t, matchPath)
	mustExist(t, mismatchPath) // 内容不符的文件不动
	if n := countIndexRowsByPath(t, svc, lib.ID, "单点/003-单点.pptx"); n != 0 {
		t.Errorf("消费删除后索引行应清理，实际 %d 行", n)
	}
}

// TestCleanupRetainedSplitArtifacts_LegacyEntryWithoutDir 旧数据兼容：v11 早期
// retained 条目无目录锚（Dir 为空），消费时回落 record.output_dir，行为与改造前
// 一致（同目录内容全等则删除）。
func TestCleanupRetainedSplitArtifacts_LegacyEntryWithoutDir(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "旧格式 retained 库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧格式")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	stale := filepath.Join(outDir, "旧格式-003.pptx")
	if err := os.WriteFile(stale, []byte("legacy-bytes"), 0644); err != nil {
		t.Fatalf("写残留页: %v", err)
	}
	fp := splitFingerprintOfTest(t, stale, 3) // Dir 留空 = v11 旧格式
	record := &assetSplitRecord{
		outputDir: outDir, pageCount: 2, target: model.AssetSplitTargetReplace,
		retained: []splitArtifactFingerprint{fp},
	}
	if got := svc.cleanupRetainedSplitArtifacts(store, lib, record, ""); len(got) != 0 {
		t.Errorf("旧格式条目应按 output_dir 消费清空：%+v", got)
	}
	mustNotExist(t, stale)
}

// TestCleanupRetainedSplitArtifacts_LstatNonENOENTRetainsEntry Lstat 的「非不存在」
// 错误（权限/句柄占用/文件名超长等瞬态）不得丢弃条目——丢弃等于永久失去归属，
// 残留文件下次重拆会被当作用户同名文件。用超过 NAME_MAX 的纯 basename 稳定触发
// ENAMETOOLONG（非 ENOENT），跨平台可复现且无需特殊权限。
func TestCleanupRetainedSplitArtifacts_LstatNonENOENTRetainsEntry(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "Lstat 瞬态错误库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "瞬态")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	longName := strings.Repeat("a", 300) + ".pptx" // 超过常见 NAME_MAX(255)
	record := &assetSplitRecord{
		outputDir: outDir, pageCount: 1, target: model.AssetSplitTargetReplace,
		retained: []splitArtifactFingerprint{
			{Page: 1, Name: longName, Size: 1, SHA256: "x", Dir: outDir},
		},
	}
	got := svc.cleanupRetainedSplitArtifacts(store, lib, record, "")
	if len(got) != 1 || got[0].Name != longName || !samePathFold(got[0].Dir, outDir) {
		t.Fatalf("Lstat 非 ENOENT 错误应保留条目（含目录锚）：%+v", got)
	}
}

// TestMergeSplitFingerprintsByName_DirAware 合并按（目录, 名称）去重：同名异目录
// 条目描述不同落点的物理文件，必须各自保留；同目录同名只保留 base。
func TestMergeSplitFingerprintsByName_DirAware(t *testing.T) {
	base := []splitArtifactFingerprint{
		{Page: 1, Name: "same.pptx", Dir: "/a"},
		{Page: 2, Name: "only-base.pptx", Dir: "/a"},
	}
	extra := []splitArtifactFingerprint{
		{Page: 1, Name: "same.pptx", Dir: "/b"},
		{Page: 2, Name: "only-base.pptx", Dir: "/a"},
	}
	got := mergeSplitFingerprintsByName(base, extra)
	if len(got) != 3 {
		t.Fatalf("同名异目录应保留两条、同目录同名去重，实际 %+v", got)
	}
	for _, fp := range got {
		if fp.Name == "same.pptx" && fp.Dir != "/a" && fp.Dir != "/b" {
			t.Errorf("同名条目目录异常：%+v", fp)
		}
	}
	if got := mergeSplitFingerprintsByName(nil, nil); got != nil {
		t.Errorf("空输入应返回 nil：%+v", got)
	}
}

// TestSplitArtifactDirWithinLibrary 目录锚越界遏制：库内路径放行；相对路径、
// `..` 逃逸、库外绝对路径、指向库外的符号链接（可创建时）拒绝。
func TestSplitArtifactDirWithinLibrary(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "x")
	if err := os.MkdirAll(inside, 0755); err != nil {
		t.Fatalf("建库内目录: %v", err)
	}
	if !splitArtifactDirWithinLibrary(root, inside) {
		t.Error("库内目录应放行")
	}
	if !splitArtifactDirWithinLibrary(root, root) {
		t.Error("库根本身应放行")
	}
	for _, dir := range []string{"relative/x", filepath.Join(root, "..", "out"), filepath.Join(inside, "..", "..")} {
		if splitArtifactDirWithinLibrary(root, dir) {
			t.Errorf("越界目录应拒绝：%s", dir)
		}
	}
	outside := filepath.Dir(root)
	link := filepath.Join(root, "escape-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Logf("跳过符号链接用例（当前环境无法创建：%v）", err)
	} else {
		defer os.Remove(link)
		if splitArtifactDirWithinLibrary(root, link) {
			t.Error("指向库外的符号链接应拒绝")
		}
	}
	// 目录不存在：返回 true 交后续 Lstat 自愈（不误删）
	if !splitArtifactDirWithinLibrary(root, filepath.Join(root, "missing")) {
		t.Error("不存在的库内目录应放行（由消费侧按文件不存在自愈）")
	}
}

// TestRetainedAnchorsRewrittenTx_ExplicitAndLegacy 迁移改锚：显式锚匹配 oldDir
// 时改写为 newDir；旧格式空锚（动态回落 output_dir）保持原样。
func TestRetainedAnchorsRewrittenTx_ExplicitAndLegacy(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "改锚单点库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	oldDir := filepath.Join(dir, "旧")
	newDir := filepath.Join(dir, "新")
	if err := store.upsertSplitRecord("改锚.pptx", "h", oldDir, 2,
		"2026-11-03 10:00:00", model.AssetSplitTargetReplace, "改锚", false, nil,
		[]splitArtifactFingerprint{
			{Page: 3, Name: "a-003.pptx", Size: 1, SHA256: "a", Dir: oldDir},
			{Page: 4, Name: "b-004.pptx", Size: 1, SHA256: "b"},
		}); err != nil {
		t.Fatalf("写记录: %v", err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()
	raw, changed, err := retainedAnchorsRewrittenTx(tx, "改锚.pptx", oldDir, newDir)
	if err != nil || !changed {
		t.Fatalf("应发生改锚：changed=%v err=%v", changed, err)
	}
	fps := parseSplitFingerprintsJSON(raw, "改锚.pptx")
	if len(fps) != 2 || fps[0].Dir != newDir || fps[1].Dir != "" {
		t.Errorf("显式锚应改新目录、空锚保持空：%+v", fps)
	}
}

// TestReplaceArtifactMismatchMessage 备份让位归属不符文案两态映射（R3）：换回成功
// 不含备份路径；换回失败（备份保留）文案含备份绝对路径。
func TestReplaceArtifactMismatchMessage(t *testing.T) {
	reverted := replaceArtifactMismatchMessage(2, "002-方案.pptx", "")
	if !strings.Contains(reverted, "第 2 页目标文件「002-方案.pptx」") ||
		!strings.Contains(reverted, "内容与拆分记录不一致") ||
		strings.Contains(reverted, "备份已保留") {
		t.Errorf("换回成功文案不符：%q", reverted)
	}
	bak := filepath.Join("D:", "库", "方案", "002-方案.pptx.replace-bak")
	kept := replaceArtifactMismatchMessage(2, "002-方案.pptx", bak)
	if !strings.Contains(kept, "备份已保留在「"+bak+"」") {
		t.Errorf("备份保留文案应含绝对路径：%q", kept)
	}
	if !strings.Contains(kept, "请人工确认后移走或还原") {
		t.Errorf("备份保留文案应提示人工处置：%q", kept)
	}
}

// TestAssetSplitRetained_RelaxedOutDirRejectsUnownedCoreName 落点归属放宽（R1）后
// 核心中间态守卫仍须生效：目录内既有凭证覆盖的历史产物（放宽依据），又有用户自建
// 的 `<主干>-009.pptx`（≥2 位页序，核心将同名覆盖写）时，必须早于核心拒绝，用户
// 文件与源文件零改动。
func TestAssetSplitRetained_RelaxedOutDirRejectsUnownedCoreName(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "放宽守卫库", dir)
	src := buildAssetsPptx(t, dir, "放宽.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	svc.parseFn = oneSlideParse()
	// 先做 source 落点拆分：目录内留下凭证覆盖的 <主干>-NNN.pptx，构成放宽依据
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("src-bytes")}).split
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "放宽.pptx", "source"); err != nil {
		t.Fatalf("source 落点拆分: %v", err)
	}
	outDir := filepath.Join(dir, "放宽")
	userPath := filepath.Join(outDir, "放宽-009.pptx")
	if err := os.WriteFile(userPath, []byte("user-bytes"), 0644); err != nil {
		t.Fatalf("写用户核心同名文件: %v", err)
	}

	core := &fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}
	svc.splitCoreFn = core.split
	outcome, err := resplitReplaceForTest(t, svc, lib, "放宽.pptx")
	if err != nil {
		t.Fatalf("守卫命中属业务失败结果（nil error），实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed || !strings.Contains(outcome.message, "放宽-009.pptx") {
		t.Fatalf("应早于核心拒绝用户核心同名文件：%+v", outcome)
	}
	if got, readErr := os.ReadFile(userPath); readErr != nil || string(got) != "user-bytes" {
		t.Errorf("用户文件不得被覆盖（err=%v content=%q）", readErr, got)
	}
	if core.calls != 0 {
		t.Errorf("守卫命中时核心不应调用，实际 %d 次", core.calls)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("拒绝路径应保留源文件：%v", statErr)
	}
}

// TestAssetSplitRetained_SourceTargetNewProductNotConsumedAsRetained R1 反向保护：
// 切 target 消费 retained 前必须先剔除与本次新产物同名的条目——旧凭证若描述的是
// 源形态产物（`<主干>-NNN.pptx`）且新核心产出字节恰好全等（拆分字节确定性），
// 直接消费会误删刚生成的新页。
func TestAssetSplitRetained_SourceTargetNewProductNotConsumedAsRetained(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "回切保护库", dir)
	name := "回切.pptx"
	src := buildAssetsPptx(t, dir, name, []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("读取源文件: %v", err)
	}
	mustScanLibrary(t, svc, lib.ID)
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("old-bytes")}).split
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, name, "source"); err != nil {
		t.Fatalf("source 落点拆分: %v", err)
	}

	// 源缩为 2 页后切 replace：第 3 页源形态产物清理失败 + 删源失败 → retained 携带
	// `<主干>-003.pptx`（内容 old-bytes）。
	buildAssetsPptx(t, dir, name, []assetsSlideSpec{
		{titlePh: "新一", body: []string{"内容"}},
		{titlePh: "新二", body: []string{"内容"}},
	})
	newHash, hashErr := hashFileSHA256(src)
	if hashErr != nil {
		t.Fatalf("算新哈希: %v", hashErr)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, name); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("two-bytes")}).split
	svc.removeArtifactFn = func(string) error { return errors.New("占用中") }
	svc.moveToRecycleBinFn = func(string) error { return errors.New("源占用中") }
	outcome, err := resplitReplaceForTest(t, svc, lib, name)
	if err != nil {
		t.Fatalf("删源失败属业务结果（nil error），实际：%v", err)
	}
	if outcome.code != model.ErrCodeAssetsSplitSourceDeleteFailed {
		t.Fatalf("前置：应返回删源失败码：%+v", outcome)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, name)
	if rec == nil || len(rec.retained) != 1 || rec.retained[0].Name != "回切-003.pptx" {
		t.Fatalf("前置：retained 应携带源形态残留页，实际 %+v", rec)
	}

	// 恢复原 3 页源并切回 source：新第 3 页与旧凭证内容全等，不得被当残留消费。
	if err := os.WriteFile(src, original, 0644); err != nil {
		t.Fatalf("还原源文件: %v", err)
	}
	mustScanLibrary(t, svc, lib.ID)
	svc.removeArtifactFn = nil
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("old-bytes")}).split
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, name, "source")
	if err != nil {
		t.Fatalf("切回 source 应成功：%v", err)
	}
	if res.Skipped || res.PageCount != 3 {
		t.Fatalf("切回 source 不得跳过：%+v", res)
	}
	page3 := filepath.Join(dir, "回切", "回切-003.pptx")
	if got, readErr := os.ReadFile(page3); readErr != nil || string(got) != "old-bytes" {
		t.Errorf("新产物第 3 页不得被 retained 消费删除（err=%v content=%q）", readErr, got)
	}
	rec = splitRecordRowForTest(t, svc, lib.ID, name)
	if rec == nil || rec.target != model.AssetSplitTargetSource {
		t.Fatalf("记录应切到 source：%+v", rec)
	}
	if len(rec.retained) != 0 {
		t.Errorf("同名条目已产出为新页，不应继续携带：%+v", rec.retained)
	}
	if len(rec.fingerprints) != 3 {
		t.Errorf("本次 3 页指纹应完整（删新页会使哈希失败降级）：%+v", rec.fingerprints)
	}
}

// seedReplaceRecordWithCoreFormRetained 构造「记录 target=replace、落点 A、retained
// 携带 **source 形态**残留凭证 `<stem>-003.pptx`、源仍在」的落库状态：A 内已有上次
// replace 的两页产物（`001-<stem>` / `002-<stem>`，含 files 索引行）与第 3 页 source
// 形态残留（`<stem>-003`，内容 old-bytes）。
//
// 记录以 source_removed=0 落库，对应真实链路删除第 3 页旧产物失败（凭证写入 retained）
// 且删源失败（补写 source_removed=0 记录）的双失败态；成功路径不会写 source_removed=0
// 的 replace 记录，故该状态只可能来自这两类失败叠加。真实链路分别由
// cleanupStaleReplaceArtifacts 写 retained、删源失败分支补写记录，二者独立发生，
// 测试直接落库即等价（不再精确复现两次失败时序）。
//
// 直接落库而非驱动两次失败尝试：删除失败注入只覆盖 cleanupStaleReplaceArtifacts，
// 随后 cleanupSplitArtifactsInDirByPage 走生产删除函数，测试无法在不依赖外部状态
// （真实句柄占用）的前提下稳定模拟两次连续失败（见 docs/spec/test-stability.md）。
// 返回源路径、残留页路径与落点目录。
func seedReplaceRecordWithCoreFormRetained(t *testing.T, svc *AssetLibraryService, lib model.AssetLibrary, dir, name string) (src, stalePage, outDir string) {
	t.Helper()
	src = buildAssetsPptx(t, dir, name, []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	stem := splitSourceBaseName(name)
	outDir = filepath.Join(dir, stem)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("建落点目录: %v", err)
	}
	// 上次 replace 的两页产物（replace 形态），构成记录指纹，并按真实链路写入索引行
	fps := make([]splitArtifactFingerprint, 0, 2)
	for i := 1; i <= 2; i++ {
		p := filepath.Join(outDir, replaceArtifactName(i, stem))
		if err := os.WriteFile(p, []byte("two-bytes"), 0644); err != nil {
			t.Fatalf("写旧产物页 %d: %v", i, err)
		}
		fps = append(fps, splitFingerprintOfTest(t, p, i))
		insertIndexRowForTest(t, svc, lib.ID, int64(9700+i), stem+"/"+filepath.Base(p))
	}
	// source 形态残留页（核心中间态命名），凭证写入 retained
	stalePage = filepath.Join(outDir, fmt.Sprintf("%s-003.pptx", stem))
	if err := os.WriteFile(stalePage, []byte("old-bytes"), 0644); err != nil {
		t.Fatalf("写 source 形态残留页: %v", err)
	}
	retained := splitFingerprintOfTest(t, stalePage, 3)
	retained.Dir = outDir
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord(name, "", outDir, 2,
		"2026-11-03 10:00:00", model.AssetSplitTargetReplace, replaceArtifactBase(stem),
		false, fps, []splitArtifactFingerprint{retained}); err != nil {
		t.Fatalf("写入带 retained 的 replace 记录: %v", err)
	}
	return src, stalePage, outDir
}

// TestAssetSplitRetained_CacheCrossDirConsumesOldDirResidual R2 端到端：replace
// 记录 output_dir=A 携带 retained（source 形态 `<stem>-003.pptx`），切 target=cache
// 后新落点 B 的第 3 页与凭证**同名**。旧实现按 basename 过滤 retained，会把 A 中
// 的真实残留误判为「本次新页同名」而丢掉，残留成永久孤儿；限定同目录后（A ≠ B）
// 不再过滤，A 残留被消费、B 中新页保留。
//
// 变异验证：去掉调用侧 samePathFold 条件（恢复无条件 recordWithoutRetainedNames），
// 本条「A 残留被删除 / retained 清空」断言变红。
func TestAssetSplitRetained_CacheCrossDirConsumesOldDirResidual(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "异目录消费库", dir)
	src, stalePage, oldOutDir := seedReplaceRecordWithCoreFormRetained(t, svc, lib, dir, "异地.pptx")

	// 解除删除失败，切 target=cache（新落点 B 在 derived 目录，与 A 不同）
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("cache-bytes")}).split
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "异地.pptx", "cache")
	if err != nil {
		t.Fatalf("切 target=cache 应成功：%v", err)
	}
	if res.Skipped || res.Target != model.AssetSplitTargetCache {
		t.Fatalf("落点切换不得跳过：%+v", res)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "异地.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetCache {
		t.Fatalf("记录应切到 cache：%+v", rec)
	}
	if samePathFold(rec.outputDir, oldOutDir) {
		t.Fatalf("前置：cache 新落点应与旧 replace 落点不同（%s vs %s）", rec.outputDir, oldOutDir)
	}
	// A 中残留页被消费删除；retained 不再携带
	if _, statErr := os.Stat(stalePage); !os.IsNotExist(statErr) {
		t.Errorf("异目录切换后旧落点残留应被消费删除（%s），stat err=%v", stalePage, statErr)
	}
	if len(rec.retained) != 0 {
		t.Errorf("消费后 retained 应清空：%+v", rec.retained)
	}
	// B 中与新残留同名的第 3 页必须保留（不得被 retained 消费误删）
	cachePage := filepath.Join(rec.outputDir, "异地-003.pptx")
	if got, readErr := os.ReadFile(cachePage); readErr != nil || string(got) != "cache-bytes" {
		t.Errorf("新落点第 3 页应保留本次产物内容（err=%v content=%q）", readErr, got)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("cache 落点应保留源文件：%v", statErr)
	}
}

// TestAssetSplitRetained_OwnsOutDirCoreNameRetryAfterUnlock R3：source→replace 同
// 目录切换后，旧 source 残留以 `<stem>-NNN.pptx` 形式留在 retained；记录已是
// target=replace 且落点相同（ownsOutDir 为真），旧守卫 firstReplaceCoreArtifactName
// 无条件拒绝核心同名文件 → 占用解除后重试永久失败。守卫改走
// firstUnownedReplaceCoreArtifactName 后，凭 retained 归属放行，重试成功并消费残留。
//
// 变异验证：把 ownsOutDir 分支守卫改回无条件 firstReplaceCoreArtifactName（或任何
// 不认凭证的判定），本条「重试成功」断言变红（被拒 + 残留仍在）。
func TestAssetSplitRetained_OwnsOutDirCoreNameRetryAfterUnlock(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "owns 守卫重试库", dir)
	src, stalePage, _ := seedReplaceRecordWithCoreFormRetained(t, svc, lib, dir, "守卫.pptx")
	rec := splitRecordRowForTest(t, svc, lib.ID, "守卫.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetReplace || len(rec.retained) != 1 {
		t.Fatalf("前置：记录应为 target=replace 且携带 retained：%+v", rec)
	}

	// 占用解除后重试 replace
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("retry-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "守卫.pptx", "replace")
	if err != nil {
		t.Fatalf("占用解除后重试不得被核心同名守卫永久拒绝：%v", err)
	}
	if res.Skipped || !res.SourceRemoved || res.PageCount != 2 {
		t.Fatalf("重试应成功删源：%+v", res)
	}
	if _, statErr := os.Stat(stalePage); !os.IsNotExist(statErr) {
		t.Errorf("retained 覆盖的核心形态残留应被消费删除（%s），stat err=%v", stalePage, statErr)
	}
	rec = splitRecordRowForTest(t, svc, lib.ID, "守卫.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetReplace || len(rec.retained) != 0 {
		t.Fatalf("重试成功后记录应为 replace 且 retained 清空：%+v", rec)
	}
	outDir := filepath.Join(dir, "守卫")
	for _, name := range []string{"001-守卫.pptx", "002-守卫.pptx"} {
		if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "retry-bytes" {
			t.Errorf("新产物应按新内容占位（%s, err=%v, content=%q）", name, readErr, got)
		}
	}
	assertNoReplaceBackup(t, outDir)
	if got := recycleCalls(); len(got) != 1 || got[0] != src {
		t.Errorf("成功应把源移入回收站一次，实际 %v", got)
	}
}
