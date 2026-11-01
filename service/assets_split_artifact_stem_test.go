package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// 拆分记录持久化产物主干（11-02-split-artifact-stem-column）用例：
//   - blocker：replace 纯数字主干（`123` → 产物主干 `123-p`）改名重链后重拆，
//     旧产物与索引行清理干净（旧实现按目录基名反推必然漏清）；
//   - M1：搬迁后 output_dir 已指新落点、记录主干仍是旧名，同目录重拆清理旧主干
//     并以 excludeStem 保护本次新主干产物；
//   - M4：cache↔source 跨落点切换收敛旧落点产物与索引行（去掉 outputToSourceDir
//     门控）；
//   - v9 前旧记录（artifact_stem='') 回落 {目录基名, replace 数字主干变体} 仍能清理；
//   - 同主干同页序多候选不可判定时保留全部并 Warn。

// TestAssetSplit_ReplaceNumericStemCleanedAfterRename replace 纯数字主干（`123` →
// 产物主干 `123-p`）改名重链后重拆：旧目录 `001-123-p.pptx` / `002-123-p.pptx`
// 与索引行必须清理干净。回归防护：旧实现按目录基名 `123` 反推产物组，与 `-p`
// 后缀产物主干恒不匹配，旧产物与其索引行永不清理（必然漏清）。
func TestAssetSplit_ReplaceNumericStemCleanedAfterRename(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "数字主干改名重拆库", dir)
	buildAssetsPptx(t, dir, "123.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "123.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}

	// 旧落点：目录名 = 源主干 123；产物主干 = replaceArtifactBase("123") = "123-p"
	oldDir := filepath.Join(dir, "123")
	oldPage1 := filepath.Join(oldDir, replaceArtifactName(1, "123"))
	oldPage2 := filepath.Join(oldDir, replaceArtifactName(2, "123"))
	if filepath.Base(oldPage1) != "001-123-p.pptx" {
		t.Fatalf("前置：数字主干产物应为 001-123-p.pptx，实际 %s", filepath.Base(oldPage1))
	}
	testutil.WriteFile(t, oldPage1, "old-1")
	testutil.WriteFile(t, oldPage2, "old-2")
	// 记录持久化实际产物主干 "123-p"（不是目录基名 123）
	if err := store.upsertSplitRecord("123.pptx", "stale-hash", oldDir, 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetReplace, "123-p", false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	insertIndexRowForTest(t, svc, lib.ID, 9901, "123/"+filepath.Base(oldPage1))
	insertIndexRowForTest(t, svc, lib.ID, 9902, "123/"+filepath.Base(oldPage2))

	// 改名 + 移动到另一目录并重链：记录键迁到新路径，output_dir 冻结旧目录
	newDir := filepath.Join(dir, "新目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "新名.pptx")
	if err := os.Rename(filepath.Join(dir, "123.pptx"), newAbs); err != nil {
		t.Fatalf("改名移动源文件: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	const newRel = "新目录/新名.pptx"
	rec := splitRecordRowForTest(t, svc, lib.ID, newRel)
	if rec == nil || !samePathFold(rec.outputDir, oldDir) || rec.artifactStem != "123-p" {
		t.Fatalf("前置：记录键应迁到新路径、output_dir 冻结旧目录且主干保留，实际 %+v", rec)
	}

	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	svc.parseFn = oneSlideParse()
	move, _ := fakeRecycleBin(t)
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
	for _, rel := range []string{"123/" + filepath.Base(oldPage1), "123/" + filepath.Base(oldPage2)} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 0 {
			t.Errorf("旧数字主干产物索引行应被清理（%s），实际 %d 行", rel, n)
		}
	}
	newOut := filepath.Join(newDir, "新名")
	for _, page := range []int{1, 2} {
		mustExist(t, filepath.Join(newOut, replaceArtifactName(page, "新名")))
	}
}

// TestAssetSplit_SourceResplitSameDirCleansOldStemKeepsNew M1：搬迁后 output_dir
// 已指新落点、记录主干仍是旧名，同一目录内重拆时清理旧主干产物，并以 excludeStem
// 排除本次新主干候选，防误删刚生成的新产物。
func TestAssetSplit_SourceResplitSameDirCleansOldStemKeepsNew(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同目录改主干库", dir)
	buildAssetsPptx(t, dir, "新名.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	outDir := filepath.Join(dir, "新名")
	oldPage1 := filepath.Join(outDir, "旧名-001.pptx")
	oldPage2 := filepath.Join(outDir, "旧名-002.pptx")
	testutil.WriteFile(t, oldPage1, "old-1")
	testutil.WriteFile(t, oldPage2, "old-2")
	// 记录落点已指新目录（搬迁后态），主干仍是旧名；哈希置陈旧驱动重拆
	if err := store.upsertSplitRecord("新名.pptx", "stale-hash", outDir, 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "旧名", false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "新名.pptx", "source")
	if err != nil {
		t.Fatalf("重拆: %v", err)
	}
	if res.Skipped || res.PageCount != 2 {
		t.Fatalf("应完成 2 页重拆：%+v", res)
	}
	mustNotExist(t, oldPage1)
	mustNotExist(t, oldPage2)
	for _, page := range []int{1, 2} {
		p := filepath.Join(outDir, fmt.Sprintf("新名-%03d.pptx", page))
		mustExist(t, p)
		assertValidZip(t, p)
	}
}

// TestAssetSplit_TargetSwitchCleansOldSourceArtifacts M4：上次 source 落点（产物在
// 源目录）、本次 cache 落点拆同一源 —— 旧源目录产物与索引行必须收敛（旧实现按
// outputToSourceDir 门控，cache 分支不清理旧落点）。
func TestAssetSplit_TargetSwitchCleansOldSourceArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "跨落点切换库", dir)
	buildAssetsPptx(t, dir, "跨落点.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	oldDir := filepath.Join(dir, "跨落点")
	oldPage1 := filepath.Join(oldDir, "跨落点-001.pptx")
	oldPage2 := filepath.Join(oldDir, "跨落点-002.pptx")
	testutil.WriteFile(t, oldPage1, "old-1")
	testutil.WriteFile(t, oldPage2, "old-2")
	if err := store.upsertSplitRecord("跨落点.pptx", "stale-hash", oldDir, 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "跨落点", false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	insertIndexRowForTest(t, svc, lib.ID, 9911, "跨落点/跨落点-001.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9912, "跨落点/跨落点-002.pptx")

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "跨落点.pptx", "cache")
	if err != nil {
		t.Fatalf("重拆（cache）: %v", err)
	}
	if res.Skipped || res.PageCount != 2 {
		t.Fatalf("应完成 2 页 cache 重拆：%+v", res)
	}
	mustNotExist(t, oldPage1)
	mustNotExist(t, oldPage2)
	for _, rel := range []string{"跨落点/跨落点-001.pptx", "跨落点/跨落点-002.pptx"} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 0 {
			t.Errorf("旧 source 落点产物索引行应被清理（%s），实际 %d 行", rel, n)
		}
	}
}

// TestCleanupSplitArtifactsInDirByPage_ReplaceNumericLegacyFallback 旧记录
// artifact_stem 为空串时回落 {目录基名, replace 数字主干变体}：目录 `123`、产物
// `001-123-p.pptx` 仍可定位并清理（v9 前记录兼容）。
func TestCleanupSplitArtifactsInDirByPage_ReplaceNumericLegacyFallback(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "数字主干旧记录库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "123")
	page1 := filepath.Join(outDir, "001-123-p.pptx")
	page2 := filepath.Join(outDir, "002-123-p.pptx")
	testutil.WriteFile(t, page1, "p1")
	testutil.WriteFile(t, page2, "p2")

	record := &assetSplitRecord{
		sourceRelPath: "旧记录.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetReplace,
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, page1)
	mustNotExist(t, page2)
	mustNotExist(t, outDir)
}

// TestCleanupSplitArtifactsInDirByPage_RecordStemAbsentSilent 记录主干权威模式下
// 目录内已无该主干组（上次重拆已清理 / M1 同目录重拆时目录内只剩本次新主干产物）
// 属正常路径：静默返回零删除，不产生「主干不匹配」告警噪音。
func TestCleanupSplitArtifactsInDirByPage_RecordStemAbsentSilent(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "记录主干缺失静默库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "新名")
	keep := filepath.Join(outDir, "新名-001.pptx")
	testutil.WriteFile(t, keep, "new")

	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     2,
		target:        model.AssetSplitTargetSource,
		artifactStem:  "旧名",
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "新名")

	mustExist(t, keep)
	if strings.Contains(logBuf.String(), "level=WARN") {
		t.Errorf("记录主干组不存在属正常路径，不应告警，实际日志：%s", logBuf.String())
	}
}

// TestCleanupSplitArtifactsInDirByPage_AmbiguousSamePageWarns 同主干同页序多候选
// （`旧名-001.pptx` 与 `旧名-1.pptx`）不可判定：两文件均保留并 Warn，不再静默。
func TestCleanupSplitArtifactsInDirByPage_AmbiguousSamePageWarns(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同页多候选告警库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧名")
	a := filepath.Join(outDir, "旧名-001.pptx")
	b := filepath.Join(outDir, "旧名-1.pptx")
	testutil.WriteFile(t, a, "a")
	testutil.WriteFile(t, b, "b")

	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     1,
		target:        model.AssetSplitTargetSource,
		artifactStem:  "旧名",
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustExist(t, a)
	mustExist(t, b)
	if !strings.Contains(logBuf.String(), "旧产物同页序多候选") {
		t.Errorf("同主干同页序多候选应 Warn「旧产物同页序多候选」，实际日志：%s", logBuf.String())
	}
}
