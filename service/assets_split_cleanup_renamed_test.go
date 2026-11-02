package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
	"workbench/util/testutil"
)

// 重拆清旧页与改名重链（11-01-split-cleanup-renamed、11-02-split-artifact-stem-column）
// 单测：
//   - 源落点：改名重链后记录键迁到新路径、output_dir 冻结旧目录，重拆按
//     `split_records.artifact_stem`（v9 权威）锚定产物组并清理旧主干产物；空值为
//     v9 前旧记录，回落 {目录基名, replace 数字主干变体}；用户自建同名异构 / 同
//     页序文件不动；
//   - replace 落点：改名重链后重拆在成功收尾阶段清理旧产物目录与索引行；纯数字
//     主干（`123` → `123-p`）的旧产物靠记录主干清理（不再必然漏清）；
//   - 同目录改主干（M1，搬迁后 output_dir == 新落点）：旧主干清理、本次新主干保留；
//   - 跨落点切换（M4，cache↔source）同样收敛旧落点产物与索引行；
//   - 同页序多候选（同主干）与记录主干/目录基名均不匹配按安全侧失败保留（不猜；
//     页数覆盖不参与选组），多候选输出 Warn；
//   - 目录不存在/目录内无形态候选属正常路径，静默无操作；非不存在的读错误 Warn；
//   - 失败路径（核心失败 / replace 回滚）不删旧产物（延迟清理契约）。

// writeSplitArtifactAt 写入旧拆分产物并把 mtime 回拨到 splitAt 之前（产物在拆分
// 成功后写入记录，mtime 恒早于 split_at）：清理侧 mtime 守卫据此判定「产物组」。
// splitAt 解析失败时回拨 1 小时（仍早于当前时间，守卫按用户文件处理的风险可控）。
// 期望被保留的用户文件继续用 testutil.WriteFile（mtime = now，晚于 split_at）。
func writeSplitArtifactAt(t *testing.T, path, content, splitAt string) {
	t.Helper()
	testutil.WriteFile(t, path, content)
	mtime := time.Now().Add(-time.Hour)
	if ts, err := time.ParseInLocation("2006-01-02 15:04:05", splitAt, time.Local); err == nil {
		mtime = ts.Add(-time.Minute)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("设置产物 mtime（%s）: %v", path, err)
	}
}

// mustNotExist 断言路径不存在（已被清理）。
func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("应已被清理但文件仍存在：%s", path)
	} else if !os.IsNotExist(err) {
		t.Errorf("检查文件状态失败（%s）: %v", path, err)
	}
}

// mustExist 断言路径存在。
func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("文件应存在：%s，err=%v", path, err)
	}
}

// countIndexRowsByPath 统计指定库内相对路径的 files 行数。
func countIndexRowsByPath(t *testing.T, svc *AssetLibraryService, libraryID, rel string) int {
	t.Helper()
	var n int
	if err := svc.openStoreForTest(t, libraryID).db.QueryRow(
		"SELECT COUNT(*) FROM files WHERE path = ?", rel).Scan(&n); err != nil {
		t.Fatalf("统计索引行（%s）: %v", rel, err)
	}
	return n
}

// insertIndexRowForTest 直接插入一条 files 行（模拟旧产物已被扫描索引）。
func insertIndexRowForTest(t *testing.T, svc *AssetLibraryService, libraryID string, id int64, rel string) {
	t.Helper()
	if _, err := svc.openStoreForTest(t, libraryID).db.Exec(
		"INSERT INTO files(id, path, size, mtime_ns, status, hash, updated_at) VALUES(?, ?, 8, 1, ?, '', ?)",
		id, rel, model.AssetFileStatusOK, "2026-10-30 10:00:00",
	); err != nil {
		t.Fatalf("插入索引行（%s）: %v", rel, err)
	}
}

// TestAssetSplit_SourceResplitCleansRenamedOldArtifacts 源落点改名重链后重拆：
// 旧主干产物与其索引行在旧目录内按页序清理，新主干产物生成，用户自建文件保留。
func TestAssetSplit_SourceResplitCleansRenamedOldArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "改名重拆源落点库", dir)
	buildAssetsPptx(t, dir, "旧名.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	var hash string
	if err := store.db.QueryRow("SELECT id, hash FROM files WHERE path = ?", "旧名.pptx").Scan(&fileID, &hash); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	// 首次拆分现状：记录 + 旧主干产物 + 旧产物索引行
	oldDir := filepath.Join(dir, "旧名")
	oldPage1 := filepath.Join(oldDir, "旧名-001.pptx")
	oldPage2 := filepath.Join(oldDir, "旧名-002.pptx")
	writeSplitArtifactAt(t, oldPage1, "old-1", "2026-10-30 10:00:00")
	writeSplitArtifactAt(t, oldPage2, "old-2", "2026-10-30 10:00:00")
	draft := filepath.Join(oldDir, "旧名-草稿.pptx")
	testutil.WriteFile(t, draft, "draft")
	if err := store.upsertSplitRecord("旧名.pptx", hash, oldDir, 2,
		"2026-10-30 10:00:00", model.AssetSplitTargetSource, "",
		false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	insertIndexRowForTest(t, svc, lib.ID, 9001, "旧名/旧名-001.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9002, "旧名/旧名-002.pptx")

	// 改名 + 移动到另一目录，再重链（record 键迁到新路径，output_dir 冻结旧目录）
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
	const newRel = "新目录/新名.pptx"
	rec := splitRecordRowForTest(t, svc, lib.ID, newRel)
	if rec == nil || !samePathFold(rec.outputDir, oldDir) {
		t.Fatalf("前置：记录键应迁到新路径且 output_dir 冻结旧目录，实际 %+v", rec)
	}

	// 源内容变化 + 库存哈希同步（确定性驱动重拆，不依赖 mtime 时序）
	buildAssetsPptx(t, newDir, "新名.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"新内容"}},
		{titlePh: "新二", body: []string{"新内容"}},
	})
	newHash, err := hashFileSHA256(newAbs)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, newRel); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, newRel, "source")
	if err != nil {
		t.Fatalf("重拆: %v", err)
	}
	if res.Skipped || res.PageCount != 2 || len(res.Files) != 2 {
		t.Fatalf("应完成 2 页重拆：%+v", res)
	}

	// 旧主干产物被清理（按页序反查命中），用户自建同名异构文件保留
	mustNotExist(t, oldPage1)
	mustNotExist(t, oldPage2)
	mustExist(t, draft)
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/旧名-001.pptx"); n != 0 {
		t.Errorf("旧产物索引行应被清理，实际 %d 行", n)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/旧名-002.pptx"); n != 0 {
		t.Errorf("旧产物索引行应被清理，实际 %d 行", n)
	}

	// 新目录按当前主干生成新产物
	newOut := filepath.Join(newDir, "新名")
	for _, name := range []string{"新名-001.pptx", "新名-002.pptx"} {
		p := filepath.Join(newOut, name)
		mustExist(t, p)
		assertValidZip(t, p)
	}
}

// TestAssetSplit_SourceResplitTriggeredByMissingPage 旧产物首页缺失触发重拆
// （源落点）：增量判定因产物不存在落空后重拆，剩余旧主干产物（页 2..N，记录
// artifact_stem 锚定即产物组）同样按页序清理；首页缺失本身无文件可删。
func TestAssetSplit_SourceResplitTriggeredByMissingPage(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "缺失首页触发库", dir)
	buildAssetsPptx(t, dir, "缺页旧名.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	var hash string
	if err := store.db.QueryRow("SELECT id, hash FROM files WHERE path = ?", "缺页旧名.pptx").Scan(&fileID, &hash); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	oldDir := filepath.Join(dir, "缺页旧名")
	oldPage2 := filepath.Join(oldDir, "缺页旧名-002.pptx")
	oldPage3 := filepath.Join(oldDir, "缺页旧名-003.pptx")
	writeSplitArtifactAt(t, oldPage2, "old-2", "2026-10-30 10:00:00")
	writeSplitArtifactAt(t, oldPage3, "old-3", "2026-10-30 10:00:00")
	if err := store.upsertSplitRecord("缺页旧名.pptx", hash, oldDir, 3,
		"2026-10-30 10:00:00", model.AssetSplitTargetSource, "",
		false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	insertIndexRowForTest(t, svc, lib.ID, 9302, "缺页旧名/缺页旧名-002.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9303, "缺页旧名/缺页旧名-003.pptx")
	// 首页产物缺失（模拟用户手工删除部分产物）：增量判定落空 → 必须重拆。
	// 剩余页 2..3 仍属记录主干产物组，照常清理；缺失的首页无文件可删，
	// 故其索引行不在本用例断言面（目录内无从定位缺失文件）。

	newDir := filepath.Join(dir, "缺页新目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "缺页新名.pptx")
	if err := os.Rename(filepath.Join(dir, "缺页旧名.pptx"), newAbs); err != nil {
		t.Fatalf("改名移动源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	const newRel = "缺页新目录/缺页新名.pptx"

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, newRel, "source")
	if err != nil {
		t.Fatalf("重拆: %v", err)
	}
	if res.Skipped || res.PageCount != 3 {
		t.Fatalf("首页产物缺失应触发 3 页重拆：%+v", res)
	}
	mustNotExist(t, oldPage2)
	mustNotExist(t, oldPage3)
	for _, rel := range []string{"缺页旧名/缺页旧名-002.pptx", "缺页旧名/缺页旧名-003.pptx"} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 0 {
			t.Errorf("旧产物索引行应被清理（%s），实际 %d 行", rel, n)
		}
	}
	newOut := filepath.Join(newDir, "缺页新名")
	for _, name := range []string{"缺页新名-001.pptx", "缺页新名-002.pptx", "缺页新名-003.pptx"} {
		mustExist(t, filepath.Join(newOut, name))
	}
}

// TestAssetSplit_SourceResplitMissingFirstPageCleaned 2 页源缺首页（旧产物仅剩页 2）
// 触发重拆：记录 artifact_stem 锚定命中即产物组，页 2 照常清理（覆盖不足不再保留）。
func TestAssetSplit_SourceResplitMissingFirstPageCleaned(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "缺首页两页触发库", dir)
	buildAssetsPptx(t, dir, "缺二旧名.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	var hash string
	if err := store.db.QueryRow("SELECT id, hash FROM files WHERE path = ?", "缺二旧名.pptx").Scan(&fileID, &hash); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	oldDir := filepath.Join(dir, "缺二旧名")
	oldPage2 := filepath.Join(oldDir, "缺二旧名-002.pptx")
	writeSplitArtifactAt(t, oldPage2, "old-2", "2026-10-30 10:00:00")
	if err := store.upsertSplitRecord("缺二旧名.pptx", hash, oldDir, 2,
		"2026-10-30 10:00:00", model.AssetSplitTargetSource, "",
		false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	insertIndexRowForTest(t, svc, lib.ID, 9322, "缺二旧名/缺二旧名-002.pptx")
	// 首页产物缺失（模拟用户手工删除）：增量判定落空 → 必须重拆。
	// v9 前「覆盖 ≥2 页才算产物组」的旧规则会保留页 2；记录主干锚定下页 2 属产物组，清理。

	newDir := filepath.Join(dir, "缺二新目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "缺二新名.pptx")
	if err := os.Rename(filepath.Join(dir, "缺二旧名.pptx"), newAbs); err != nil {
		t.Fatalf("改名移动源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	const newRel = "缺二新目录/缺二新名.pptx"

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, newRel, "source")
	if err != nil {
		t.Fatalf("重拆: %v", err)
	}
	if res.Skipped || res.PageCount != 2 {
		t.Fatalf("首页产物缺失应触发 2 页重拆：%+v", res)
	}
	mustNotExist(t, oldPage2)
	if n := countIndexRowsByPath(t, svc, lib.ID, "缺二旧名/缺二旧名-002.pptx"); n != 0 {
		t.Errorf("旧产物索引行应被清理，实际 %d 行", n)
	}
	newOut := filepath.Join(newDir, "缺二新名")
	for _, name := range []string{"缺二新名-001.pptx", "缺二新名-002.pptx"} {
		mustExist(t, filepath.Join(newOut, name))
	}
}

// TestAssetSplit_SourceResplitCoreFailureKeepsRenamedOldArtifacts 源落点核心
// 拆分失败时不得清理旧产物目录：旧文件与索引行原样保留（延迟清理契约——先删
// 后拆会在失败时连带丢掉上一次成果），本次也不产生产物目录。
func TestAssetSplit_SourceResplitCoreFailureKeepsRenamedOldArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "核心失败保留库", dir)
	buildAssetsPptx(t, dir, "失败前旧名.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	var hash string
	if err := store.db.QueryRow("SELECT id, hash FROM files WHERE path = ?", "失败前旧名.pptx").Scan(&fileID, &hash); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	oldDir := filepath.Join(dir, "失败前旧名")
	oldPage1 := filepath.Join(oldDir, "失败前旧名-001.pptx")
	oldPage2 := filepath.Join(oldDir, "失败前旧名-002.pptx")
	testutil.WriteFile(t, oldPage1, "old-1")
	testutil.WriteFile(t, oldPage2, "old-2")
	if err := store.upsertSplitRecord("失败前旧名.pptx", hash, oldDir, 2,
		"2026-10-30 10:00:00", model.AssetSplitTargetSource, "",
		false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	insertIndexRowForTest(t, svc, lib.ID, 9401, "失败前旧名/失败前旧名-001.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9402, "失败前旧名/失败前旧名-002.pptx")

	newDir := filepath.Join(dir, "失败新目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "失败新名.pptx")
	if err := os.Rename(filepath.Join(dir, "失败前旧名.pptx"), newAbs); err != nil {
		t.Fatalf("改名移动源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	const newRel = "失败新目录/失败新名.pptx"

	// 源内容变化 + 库存哈希同步（确定性驱动重拆，不依赖 mtime 时序）
	buildAssetsPptx(t, newDir, "失败新名.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"新内容"}},
		{titlePh: "新二", body: []string{"新内容"}},
	})
	newHash, err := hashFileSHA256(newAbs)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, newRel); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}

	svc.splitCoreFn = func(context.Context, string, func(done, total int)) (model.PptxSplitResult, error) {
		return model.PptxSplitResult{}, errors.New("核心拆分失败（模拟源文件损坏）")
	}

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, newRel, "source"); err == nil {
		t.Fatal("核心拆分失败应返回错误")
	}
	// 旧产物与索引行原样保留（核心失败不触达清理）
	for p, want := range map[string]string{oldPage1: "old-1", oldPage2: "old-2"} {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("旧产物应保留（%s）: %v", p, err)
		}
		if string(got) != want {
			t.Errorf("旧产物内容应原样保留（%s），实际 %q", p, got)
		}
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "失败前旧名/失败前旧名-001.pptx"); n != 1 {
		t.Errorf("旧产物索引行应保留，实际 %d 行", n)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "失败前旧名/失败前旧名-002.pptx"); n != 1 {
		t.Errorf("旧产物索引行应保留，实际 %d 行", n)
	}
	// 本次未产出任何新产物
	mustNotExist(t, filepath.Join(newDir, "失败新名"))
}

// TestAssetSplit_ReplaceResplitCleansRenamedOldArtifacts replace 落点改名重链后
// 重拆：成功收尾阶段清理旧产物目录内的旧页与索引行，新目录产物按新主干生成。
func TestAssetSplit_ReplaceResplitCleansRenamedOldArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "改名重拆替换落点库", dir)
	buildAssetsPptx(t, dir, "旧替换.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	var hash string
	if err := store.db.QueryRow("SELECT id, hash FROM files WHERE path = ?", "旧替换.pptx").Scan(&fileID, &hash); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	oldDir := filepath.Join(dir, "旧替换")
	oldPage1 := filepath.Join(oldDir, replaceArtifactName(1, "旧替换"))
	oldPage2 := filepath.Join(oldDir, replaceArtifactName(2, "旧替换"))
	writeSplitArtifactAt(t, oldPage1, "old-1", "2026-10-30 10:00:00")
	writeSplitArtifactAt(t, oldPage2, "old-2", "2026-10-30 10:00:00")
	draft := filepath.Join(oldDir, "旧替换-草稿.pptx")
	testutil.WriteFile(t, draft, "draft")
	if err := store.upsertSplitRecord("旧替换.pptx", hash, oldDir, 2,
		"2026-10-30 10:00:00", model.AssetSplitTargetReplace, "",
		false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	insertIndexRowForTest(t, svc, lib.ID, 9101, "旧替换/"+filepath.Base(oldPage1))
	insertIndexRowForTest(t, svc, lib.ID, 9102, "旧替换/"+filepath.Base(oldPage2))

	newDir := filepath.Join(dir, "新替换目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "新替换.pptx")
	if err := os.Rename(filepath.Join(dir, "旧替换.pptx"), newAbs); err != nil {
		t.Fatalf("改名移动源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	const newRel = "新替换目录/新替换.pptx"
	rec := splitRecordRowForTest(t, svc, lib.ID, newRel)
	if rec == nil || !samePathFold(rec.outputDir, oldDir) {
		t.Fatalf("前置：记录键应迁到新路径且 output_dir 冻结旧目录，实际 %+v", rec)
	}

	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	svc.parseFn = oneSlideParse()
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, newRel, "replace")
	if err != nil {
		t.Fatalf("重拆（replace）: %v", err)
	}
	if !res.SourceRemoved || res.PageCount != 2 {
		t.Fatalf("应完成替换并删源：%+v", res)
	}

	mustNotExist(t, oldPage1)
	mustNotExist(t, oldPage2)
	mustExist(t, draft) // 用户自建同名异构文件不动
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧替换/"+filepath.Base(oldPage1)); n != 0 {
		t.Errorf("旧产物索引行应被清理，实际 %d 行", n)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧替换/"+filepath.Base(oldPage2)); n != 0 {
		t.Errorf("旧产物索引行应被清理，实际 %d 行", n)
	}
	if got := recycleCalls(); len(got) != 1 || !samePathFold(got[0], newAbs) {
		t.Errorf("应把新路径源文件移入回收站，实际 %v", got)
	}

	newOut := filepath.Join(newDir, "新替换")
	for _, page := range []int{1, 2} {
		mustExist(t, filepath.Join(newOut, replaceArtifactName(page, "新替换")))
	}
	if rec := splitRecordRowForTest(t, svc, lib.ID, newRel); rec == nil || !samePathFold(rec.outputDir, newOut) {
		t.Errorf("重拆后记录落点应更新为新目录，实际 %+v", rec)
	}
}

// TestAssetSplit_ReplaceResplitCleansOtherTargetOldArtifacts 落点切换（上次 source、
// 本次 replace）叠加改名重链：旧落点产物目录同样按页序收敛，不因记录 target 不同
// 而残留（output_dir 恒为该源文件上次产物的独占目录）。
func TestAssetSplit_ReplaceResplitCleansOtherTargetOldArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "落点切换收敛库", dir)
	buildAssetsPptx(t, dir, "切换旧名.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	var hash string
	if err := store.db.QueryRow("SELECT id, hash FROM files WHERE path = ?", "切换旧名.pptx").Scan(&fileID, &hash); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	// 上次以 source 落点拆分的旧产物（target 与本次 replace 不同）
	oldDir := filepath.Join(dir, "切换旧名")
	oldPage1 := filepath.Join(oldDir, "切换旧名-001.pptx")
	oldPage2 := filepath.Join(oldDir, "切换旧名-002.pptx")
	writeSplitArtifactAt(t, oldPage1, "old-1", "2026-10-30 10:00:00")
	writeSplitArtifactAt(t, oldPage2, "old-2", "2026-10-30 10:00:00")
	if err := store.upsertSplitRecord("切换旧名.pptx", hash, oldDir, 2,
		"2026-10-30 10:00:00", model.AssetSplitTargetSource, "",
		false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}

	newDir := filepath.Join(dir, "切换新目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "切换新名.pptx")
	if err := os.Rename(filepath.Join(dir, "切换旧名.pptx"), newAbs); err != nil {
		t.Fatalf("改名移动源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	const newRel = "切换新目录/切换新名.pptx"

	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	svc.parseFn = oneSlideParse()
	move, _ := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, newRel, "replace"); err != nil {
		t.Fatalf("重拆（replace）: %v", err)
	}
	mustNotExist(t, oldPage1)
	mustNotExist(t, oldPage2)
	newOut := filepath.Join(newDir, "切换新名")
	for _, page := range []int{1, 2} {
		mustExist(t, filepath.Join(newOut, replaceArtifactName(page, "切换新名")))
	}
}

// TestAssetSplit_ReplaceRecycleFailureKeepsCleanedOldArtifacts 时序锁定：replace
// 拆分成功但源移入回收站失败，仍返回 E_ASSETS_SPLIT_SOURCE_DELETE_FAILED；新产物
// 与源文件保留，旧产物目录已清理（清理挂在删源之前，不因删源失败回滚）。
func TestAssetSplit_ReplaceRecycleFailureKeepsCleanedOldArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换删源失败清旧库", dir)
	buildAssetsPptx(t, dir, "旧替换名.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	var hash string
	if err := store.db.QueryRow("SELECT id, hash FROM files WHERE path = ?", "旧替换名.pptx").Scan(&fileID, &hash); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	oldDir := filepath.Join(dir, "旧替换名")
	oldPage1 := filepath.Join(oldDir, replaceArtifactName(1, "旧替换名"))
	oldPage2 := filepath.Join(oldDir, replaceArtifactName(2, "旧替换名"))
	writeSplitArtifactAt(t, oldPage1, "old-1", "2026-10-30 10:00:00")
	writeSplitArtifactAt(t, oldPage2, "old-2", "2026-10-30 10:00:00")
	if err := store.upsertSplitRecord("旧替换名.pptx", hash, oldDir, 2,
		"2026-10-30 10:00:00", model.AssetSplitTargetReplace, "",
		false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	insertIndexRowForTest(t, svc, lib.ID, 9251, "旧替换名/"+filepath.Base(oldPage1))
	insertIndexRowForTest(t, svc, lib.ID, 9252, "旧替换名/"+filepath.Base(oldPage2))

	newDir := filepath.Join(dir, "新替换目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "新替换名.pptx")
	if err := os.Rename(filepath.Join(dir, "旧替换名.pptx"), newAbs); err != nil {
		t.Fatalf("改名移动源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	const newRel = "新替换目录/新替换名.pptx"

	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	svc.parseFn = oneSlideParse()
	svc.moveToRecycleBinFn = func(string) error { return errors.New("文件被占用") }

	_, err := svc.SplitAssetFile(context.Background(), lib.ID, newRel, "replace")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsSplitSourceDeleteFailed {
		t.Fatalf("删源失败应返回 E_ASSETS_SPLIT_SOURCE_DELETE_FAILED，实际：%v", err)
	}
	// 旧目录已清理（清理在删源之前，不回随删源失败回滚）
	mustNotExist(t, oldPage1)
	mustNotExist(t, oldPage2)
	for _, rel := range []string{"旧替换名/" + filepath.Base(oldPage1), "旧替换名/" + filepath.Base(oldPage2)} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 0 {
			t.Errorf("旧产物索引行应被清理（%s），实际 %d 行", rel, n)
		}
	}
	// 新产物与源文件保留（可关闭占用程序后重试）
	newOut := filepath.Join(newDir, "新替换名")
	for _, page := range []int{1, 2} {
		mustExist(t, filepath.Join(newOut, replaceArtifactName(page, "新替换名")))
	}
	mustExist(t, newAbs)
}

// TestAssetSplit_ReplaceFailureKeepsRenamedOldArtifacts replace 失败路径（页产物
// rename 失败触发回滚）不得删除旧产物目录内的旧页与其索引行（延迟清理契约）。
func TestAssetSplit_ReplaceFailureKeepsRenamedOldArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "改名重拆失败保留库", dir)
	buildAssetsPptx(t, dir, "失败旧名.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	var hash string
	if err := store.db.QueryRow("SELECT id, hash FROM files WHERE path = ?", "失败旧名.pptx").Scan(&fileID, &hash); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	oldDir := filepath.Join(dir, "失败旧名")
	oldPage1 := filepath.Join(oldDir, replaceArtifactName(1, "失败旧名"))
	oldPage2 := filepath.Join(oldDir, replaceArtifactName(2, "失败旧名"))
	testutil.WriteFile(t, oldPage1, "old-1")
	testutil.WriteFile(t, oldPage2, "old-2")
	if err := store.upsertSplitRecord("失败旧名.pptx", hash, oldDir, 2,
		"2026-10-30 10:00:00", model.AssetSplitTargetReplace, "",
		false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	insertIndexRowForTest(t, svc, lib.ID, 9201, "失败旧名/"+filepath.Base(oldPage1))
	insertIndexRowForTest(t, svc, lib.ID, 9202, "失败旧名/"+filepath.Base(oldPage2))

	newDir := filepath.Join(dir, "失败新目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "失败新名.pptx")
	if err := os.Rename(filepath.Join(dir, "失败旧名.pptx"), newAbs); err != nil {
		t.Fatalf("改名移动源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}

	// 最后一页路径被替换为不存在文件 → rename 失败 → 回滚
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes"), dropFile: true}).split
	svc.parseFn = oneSlideParse()
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "失败新目录/失败新名.pptx", "replace"); err == nil {
		t.Fatal("rename 失败应返回错误")
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("失败路径不得移入回收站，实际 %v", got)
	}
	// 旧产物按原内容保留，索引行完整
	for p, want := range map[string]string{oldPage1: "old-1", oldPage2: "old-2"} {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("旧产物应保留（%s）: %v", p, err)
		}
		if string(got) != want {
			t.Errorf("旧产物内容应原样保留（%s），实际 %q", p, got)
		}
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "失败旧名/"+filepath.Base(oldPage1)); n != 1 {
		t.Errorf("旧产物索引行应保留，实际 %d 行", n)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "失败旧名/"+filepath.Base(oldPage2)); n != 1 {
		t.Errorf("旧产物索引行应保留，实际 %d 行", n)
	}
}

// TestCleanupSplitArtifactsInDirByPage_DifferentStemSamePageKeepsOnlyProduct 同页序
// 存在另一主干的用户文件不影响产物删除：记录主干锚定已确定产物组主干，页 2 的
// `另一份-002` 是用户文件，产物页 1/2 照常清理。
func TestCleanupSplitArtifactsInDirByPage_DifferentStemSamePageKeepsOnlyProduct(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同页异主干库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "多候选")
	page1 := filepath.Join(outDir, "多候选-001.pptx")
	page2 := filepath.Join(outDir, "多候选-002.pptx")
	userFile := filepath.Join(outDir, "另一份-002.pptx")
	draft := filepath.Join(outDir, "多候选-草稿.pptx")
	testutil.WriteFile(t, page1, "p1")
	testutil.WriteFile(t, page2, "p2")
	testutil.WriteFile(t, userFile, "a2")
	testutil.WriteFile(t, draft, "draft")

	record := &assetSplitRecord{
		sourceRelPath: "多候选.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, page1)
	mustNotExist(t, page2)
	mustExist(t, userFile) // 主干不符，不参与产物判定
	mustExist(t, draft)    // 非产物命名形态
	mustExist(t, outDir)   // 用户文件仍在，目录不删
}

// TestCleanupSplitArtifactsInDirByPage_RemovesEmptyDir 旧产物全部清理后目录变空：
// 目录一并删除，不留空壳。
func TestCleanupSplitArtifactsInDirByPage_RemovesEmptyDir(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "空目录收尾库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "空目录")
	page1 := filepath.Join(outDir, "空目录-001.pptx")
	page2 := filepath.Join(outDir, "空目录-002.pptx")
	testutil.WriteFile(t, page1, "p1")
	testutil.WriteFile(t, page2, "p2")

	record := &assetSplitRecord{
		sourceRelPath: "空目录.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, page1)
	mustNotExist(t, page2)
	mustNotExist(t, outDir) // 删空后不留空目录
}

// TestCleanupSplitArtifactsInDirByPage_EmptyRecordNoop 记录缺失/页数为 0 时零操作
// （不读目录、不误删）：改名重链信息不全时按安全侧失败。
func TestCleanupSplitArtifactsInDirByPage_EmptyRecordNoop(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "空记录库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "空记录")
	page1 := filepath.Join(outDir, "空记录-001.pptx")
	testutil.WriteFile(t, page1, "p1")

	svc.cleanupSplitArtifactsInDirByPage(store, lib, nil, outDir, "")
	svc.cleanupSplitArtifactsInDirByPage(store, lib, &assetSplitRecord{outputDir: outDir, pageCount: 0}, outDir, "")
	mustExist(t, page1)
}

// TestCleanupSplitArtifactsInDirByPage_UserFileSamePageKept 页 1 旧产物缺失时，目录内
// 用户自建的同页序文件（笔记-001.pptx）不得被当作旧产物删除：产物组按记录主干
// 锚定，主干不符的用户文件与其索引行原样保留，其余旧页照常清理。
func TestCleanupSplitArtifactsInDirByPage_UserFileSamePageKept(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "用户同页序文件库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧名")
	page2 := filepath.Join(outDir, "旧名-002.pptx")
	page3 := filepath.Join(outDir, "旧名-003.pptx")
	userFile := filepath.Join(outDir, "笔记-001.pptx")
	testutil.WriteFile(t, page2, "p2")
	testutil.WriteFile(t, page3, "p3")
	testutil.WriteFile(t, userFile, "user")
	insertIndexRowForTest(t, svc, lib.ID, 9501, "旧名/旧名-002.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9502, "旧名/旧名-003.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9503, "旧名/笔记-001.pptx")

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     3,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustExist(t, userFile)
	mustExist(t, outDir) // 用户文件仍在，目录不删
	mustNotExist(t, page2)
	mustNotExist(t, page3)
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/笔记-001.pptx"); n != 1 {
		t.Errorf("用户文件索引行应保留，实际 %d 行", n)
	}
	for _, rel := range []string{"旧名/旧名-002.pptx", "旧名/旧名-003.pptx"} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 0 {
			t.Errorf("旧产物索引行应被清理（%s），实际 %d 行", rel, n)
		}
	}
}

// TestCleanupSplitArtifactsInDirByPage_StemNotAnchorKept 目录内仅有形态匹配但主干
// 不等于记录主干（此处为空，回落目录基名）的文件（用户自建）：锚点失配即不可判定，整目录零删除（安全
// 侧失败），文件与目录均保留。
func TestCleanupSplitArtifactsInDirByPage_StemNotAnchorKept(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "证据不足保留库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "证据不足")
	userFile := filepath.Join(outDir, "笔记-001.pptx")
	testutil.WriteFile(t, userFile, "user")

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustExist(t, userFile)
	mustExist(t, outDir)
}

// TestCleanupSplitArtifactsInDirByPage_NoAnchorMatchKept 多个主干均不等于目录基名：
// 无法判定哪组是产物，全部保留。
func TestCleanupSplitArtifactsInDirByPage_NoAnchorMatchKept(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "主干平票库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "平票")
	fileA := filepath.Join(outDir, "甲-001.pptx")
	fileB := filepath.Join(outDir, "乙-002.pptx")
	testutil.WriteFile(t, fileA, "a")
	testutil.WriteFile(t, fileB, "b")

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustExist(t, fileA)
	mustExist(t, fileB)
	mustExist(t, outDir)
}

// TestCleanupSplitArtifactsInDirByPage_SinglePageUserFileKept 单页记录不再有「任意
// 唯一同形候选即产物」豁免：目录基名 `旧名`、用户自建 `笔记-001.pptx` 主干不符，
// 文件、索引行与目录全部保留（审核发现的误删路径 ①）。
func TestCleanupSplitArtifactsInDirByPage_SinglePageUserFileKept(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "单页用户文件库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧名")
	userFile := filepath.Join(outDir, "笔记-001.pptx")
	testutil.WriteFile(t, userFile, "user")
	insertIndexRowForTest(t, svc, lib.ID, 9701, "旧名/笔记-001.pptx")

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     1,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustExist(t, userFile)
	mustExist(t, outDir) // 用户文件仍在，目录不删
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/笔记-001.pptx"); n != 1 {
		t.Errorf("用户文件索引行应保留，实际 %d 行", n)
	}
}

// TestCleanupSplitArtifactsInDirByPage_SinglePageProductRemoved 单页记录的目录基名
// 与产物主干一致（`旧名/旧名-001.pptx`）：记录/目录基名锚定命中，照常删除，目录删空后
// 一并移除。
func TestCleanupSplitArtifactsInDirByPage_SinglePageProductRemoved(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "单页记录库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧名")
	page1 := filepath.Join(outDir, "旧名-001.pptx")
	testutil.WriteFile(t, page1, "p1")
	insertIndexRowForTest(t, svc, lib.ID, 9702, "旧名/旧名-001.pptx")

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     1,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, page1)
	mustNotExist(t, outDir)
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/旧名-001.pptx"); n != 0 {
		t.Errorf("产物索引行应被清理，实际 %d 行", n)
	}
}

// TestCleanupSplitArtifactsInDirByPage_UserStemMajorityKept 用户在同一目录放多份
// 同主干文件反超产物组（审核发现的误删路径 ②）：记录 3 页、产物只剩
// `旧名-001`，用户 `笔记-002`/`笔记-003` 共 2 页。记录主干为空时回落目录基名，只认 `旧名`，
// 产物照常删除、用户文件与其索引行/缩略图保留。
func TestCleanupSplitArtifactsInDirByPage_UserStemMajorityKept(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "用户主干反超库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧名")
	product := filepath.Join(outDir, "旧名-001.pptx")
	user2 := filepath.Join(outDir, "笔记-002.pptx")
	user3 := filepath.Join(outDir, "笔记-003.pptx")
	testutil.WriteFile(t, product, "p1")
	testutil.WriteFile(t, user2, "u2")
	testutil.WriteFile(t, user3, "u3")
	insertIndexRowForTest(t, svc, lib.ID, 9801, "旧名/旧名-001.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9802, "旧名/笔记-002.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9803, "旧名/笔记-003.pptx")
	insertThumbLedgerRow(t, store, 9801, "small", 1, 100, 1)
	insertThumbLedgerRow(t, store, 9802, "small", 1, 100, 2)
	if _, err := store.db.Exec(
		"INSERT INTO slides(file_id, slide_num, title, text, speaker_notes) VALUES(9801, 1, '', '', ''), (9802, 1, '', '', '')"); err != nil {
		t.Fatalf("插入幻灯片行: %v", err)
	}

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     3,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, product)
	mustExist(t, user2)
	mustExist(t, user3)
	mustExist(t, outDir) // 用户文件仍在，目录不删
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/旧名-001.pptx"); n != 0 {
		t.Errorf("产物索引行应被清理，实际 %d 行", n)
	}
	for _, rel := range []string{"旧名/笔记-002.pptx", "旧名/笔记-003.pptx"} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 1 {
			t.Errorf("用户文件索引行应保留（%s），实际 %d 行", rel, n)
		}
	}
	// 级联：被删产物的缩略图记账行一并失效，用户文件的行保留。
	if n := countThumbCacheRows(t, store); n != 1 {
		t.Errorf("清理后应只剩用户文件的 1 条缩略图记账行，实际 %d", n)
	}
	// 级联：被删产物的 slides 行随 files 行级联删除，用户文件的行保留（防僵尸页）。
	for _, tc := range []struct {
		fileID int64
		want   int
	}{{9801, 0}, {9802, 1}} {
		var n int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM slides WHERE file_id = ?", tc.fileID).Scan(&n); err != nil {
			t.Fatalf("统计幻灯片行（%d）: %v", tc.fileID, err)
		}
		if n != tc.want {
			t.Errorf("文件 %d 的幻灯片行应为 %d，实际 %d", tc.fileID, tc.want, n)
		}
	}
}

// TestCleanupSplitArtifactsInDirByPage_PartialProductRemoved 2 页记录、目录基名
// `旧名` 内只剩 `旧名-002.pptx`：基名匹配即产物组，覆盖不足也不再保留（与
// 「无基名匹配保留」用例区分）。
func TestCleanupSplitArtifactsInDirByPage_PartialProductRemoved(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "部分产物库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧名")
	page2 := filepath.Join(outDir, "旧名-002.pptx")
	testutil.WriteFile(t, page2, "p2")
	insertIndexRowForTest(t, svc, lib.ID, 9804, "旧名/旧名-002.pptx")

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, page2)
	mustNotExist(t, outDir)
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/旧名-002.pptx"); n != 0 {
		t.Errorf("产物索引行应被清理，实际 %d 行", n)
	}
}

// TestCleanupSplitArtifactsInDirByPage_DirRenamedKeepsAll 目录被改名（`归档` 与产物
// 主干 `旧名` 不符）：锚点失配即不可判定，零删除并 Warn。
func TestCleanupSplitArtifactsInDirByPage_DirRenamedKeepsAll(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "目录改名保留库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "归档")
	page1 := filepath.Join(outDir, "旧名-001.pptx")
	page2 := filepath.Join(outDir, "旧名-002.pptx")
	testutil.WriteFile(t, page1, "p1")
	testutil.WriteFile(t, page2, "p2")

	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustExist(t, page1)
	mustExist(t, page2)
	mustExist(t, outDir)
	if !strings.Contains(logBuf.String(), "基名与产物主干不匹配") {
		t.Errorf("基名失配应 Warn「基名与产物主干不匹配」，实际日志：%s", logBuf.String())
	}
}

// TestCleanupSplitArtifactsInDirByPage_SkipsSubdirNamedLikeArtifact 目录内存在与产物
// 同名形态的子目录（`旧名-001.pptx/`）：IsRegular 过滤跳过（不动目录），其余常规
// 产物照常清理。
func TestCleanupSplitArtifactsInDirByPage_SkipsSubdirNamedLikeArtifact(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同名子目录库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧名")
	subdir := filepath.Join(outDir, "旧名-001.pptx")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("建同名子目录: %v", err)
	}
	page2 := filepath.Join(outDir, "旧名-002.pptx")
	testutil.WriteFile(t, page2, "p2")

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustExist(t, subdir) // 子目录不是常规文件，不得删除
	mustNotExist(t, page2)
	mustExist(t, outDir) // 子目录仍在，目录不删
}

// TestCleanupSplitArtifactsInDirByPage_ReplaceKeepsUserFile replace 形态下用户自建
// `<页序>-<主干>.pptx`（`001-笔记.pptx`）与产物同形态，只能靠主干区分：产物组
// 主干 `旧名` 等于目录基名，照常清理；用户主干 `笔记` 不符，文件与其索引行保留。
func TestCleanupSplitArtifactsInDirByPage_ReplaceKeepsUserFile(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换形态用户文件库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧名")
	page1 := filepath.Join(outDir, replaceArtifactName(1, "旧名"))
	page2 := filepath.Join(outDir, replaceArtifactName(2, "旧名"))
	userFile := filepath.Join(outDir, "001-笔记.pptx")
	testutil.WriteFile(t, page1, "p1")
	testutil.WriteFile(t, page2, "p2")
	testutil.WriteFile(t, userFile, "user")
	insertIndexRowForTest(t, svc, lib.ID, 9601, "旧名/"+filepath.Base(userFile))

	record := &assetSplitRecord{
		sourceRelPath: "新名替换.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetReplace,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, page1)
	mustNotExist(t, page2)
	mustExist(t, userFile)
	mustExist(t, outDir)
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/"+filepath.Base(userFile)); n != 1 {
		t.Errorf("用户文件索引行应保留，实际 %d 行", n)
	}
}

// TestCleanupSplitArtifactsInDirByPage_ReadErrorWarns 非「不存在」的读错误必须
// Warn 后跳过，不得静默失败。用含 NUL 的非法路径驱动（Windows/Linux 均返回
// EINVAL 而非 ENOENT）。
func TestCleanupSplitArtifactsInDirByPage_ReadErrorWarns(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "读错误告警库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	badPath := filepath.Join(dir, "旧名\x00非法")

	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     badPath,
		pageCount:     2,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, badPath, "")

	if !strings.Contains(logBuf.String(), "读取旧拆分产物目录失败") {
		t.Errorf("非不存在读错误应 Warn「读取旧拆分产物目录失败」，实际日志：%s", logBuf.String())
	}
}

// TestCleanupSplitArtifactsInDirByPage_MissingDirNoop 旧产物目录不存在（首次重拆
// 尚无产物、或用户已自行清理）：属正常路径，静默无操作——不 panic、不重建目录、
// 不误报「读取失败」或「基名失配」告警。
func TestCleanupSplitArtifactsInDirByPage_MissingDirNoop(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "目录不存在库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "不存在")

	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     3,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, outDir)
	if log := logBuf.String(); strings.Contains(log, "读取旧拆分产物目录失败") ||
		strings.Contains(log, "基名与产物主干不匹配") {
		t.Errorf("目录不存在属正常路径，不应告警，实际日志：%s", log)
	}
}

// TestCollectSplitArtifactsByPage_ReadError 目录不可读时 os.ReadDir 错误上抛
// （调用方据错误静默返回，与「有候选但无法判定」的告警路径区分）。
func TestCollectSplitArtifactsByPage_ReadError(t *testing.T) {
	_, _, err := collectSplitArtifactsByPage(
		filepath.Join(t.TempDir(), "缺失目录"), model.AssetSplitTargetSource, 3)
	if err == nil {
		t.Fatal("目录不存在应返回错误")
	}
}

// TestCleanupSplitArtifactsInDirByPage_ExistingEmptyDirRemoved 目录存在但为空（用户
// 已手工清理全部旧产物）：无候选可清，静默返回并顺手收敛空目录，不留空壳。
func TestCleanupSplitArtifactsInDirByPage_ExistingEmptyDirRemoved(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "空目录无候选库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "空壳")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("建空目录: %v", err)
	}

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     3,
		target:        model.AssetSplitTargetSource,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, outDir)
}

// TestDominantSplitArtifactStem_ExactPreferredOverCaseVariant 大小写敏感文件系统上
// `Old`（产物组）与 `old`（用户组）并存：精确匹配恒定优先，不随 map 遍历顺序随机
// 选中用户组（否则同一目录状态可误删用户文件）。用户组页数覆盖更多也不得反超。
func TestDominantSplitArtifactStem_ExactPreferredOverCaseVariant(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Old")
	byStem := map[string]map[int]bool{
		"Old": {1: true},
		"old": {1: true, 2: true},
	}
	for i := 0; i < 32; i++ { // 多轮覆盖 map 随机遍历序
		stem, ok := dominantSplitArtifactStem(byStem, []string{filepath.Base(dir)})
		if !ok || stem != "Old" {
			t.Fatalf("精确匹配应优先返回 Old，实际 (%q, %v)", stem, ok)
		}
	}
}

// TestDominantSplitArtifactStem_AmbiguousFoldNoExact 无精确命中且折叠命中多于一个：
// 不可判定（调用方零删除 + Warn），禁按遍历顺序取首个。
func TestDominantSplitArtifactStem_AmbiguousFoldNoExact(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "OLD")
	byStem := map[string]map[int]bool{
		"Old": {1: true},
		"old": {1: true},
	}
	for i := 0; i < 32; i++ {
		if stem, ok := dominantSplitArtifactStem(byStem, []string{filepath.Base(dir)}); ok {
			t.Fatalf("折叠命中多于一个且无精确匹配应不可判定，实际返回 %q", stem)
		}
	}
}

// TestDominantSplitArtifactStem_SingleFoldHit 唯一折叠命中（Windows 大小写不敏感
// 语义：目录名大小写变体与产物主干视为同一组）仍判为产物组。
func TestDominantSplitArtifactStem_SingleFoldHit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "old")
	byStem := map[string]map[int]bool{"Old": {1: true}}
	stem, ok := dominantSplitArtifactStem(byStem, []string{filepath.Base(dir)})
	if !ok || stem != "Old" {
		t.Fatalf("唯一折叠命中应返回 Old，实际 (%q, %v)", stem, ok)
	}
}

// TestIsBenignDirRemoveErr 目录删除的可静默错误口径：成功/不存在/非空（Linux
// ENOTEMPTY、Windows ERROR_DIR_NOT_EMPTY=145）静默；其他错误（权限等）必须告警。
func TestIsBenignDirRemoveErr(t *testing.T) {
	if !isBenignDirRemoveErr(nil) {
		t.Error("nil 应视为可静默（删除成功）")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "非空目录")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	testutil.WriteFile(t, filepath.Join(sub, "x.txt"), "x")
	rmErr := os.Remove(sub)
	if rmErr == nil {
		t.Fatal("非空目录删除应返回错误")
	}
	if !isBenignDirRemoveErr(rmErr) {
		t.Errorf("非空目录删除错误应视为可静默，实际 %v", rmErr)
	}
	if isBenignDirRemoveErr(errors.New("permission denied")) {
		t.Error("普通错误不应视为可静默")
	}
}
