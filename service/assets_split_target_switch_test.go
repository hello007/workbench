package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"workbench/model"
	"workbench/util/testutil"
)

// 落点切换生效与清理 mtime 守卫（11-02-split-target-switch-and-mtime-guard）用例：
//   - 增量跳过判定纳入 target 一致性（不比较 output_dir，改名重链仍跳过）：
//     source→cache / cache→source 源未变时必须真正换落点重拆（旧实现只比哈希 +
//     首页产物存在，直接 Skipped，旧落点产物永不收敛，结果还回显与实际不符的
//     Target）；
//   - 同目录跨落点（replace→source）清理旧形态产物与索引行；
//   - 清理 mtime 守卫：候选晚于记录 split_at + 容差视为用户后放入的同名文件保留；
//   - excludeStem 精确比较 + 仅大小写差异不进入 M1 清理（新产物不被误删）；
//   - 三种落点成功用例断言 artifact_stem 写入值（变异为空或缺 -p 会变红）。

// TestAssetSplit_TargetSwitchSourceToCacheReclones source→cache、源未变：不得
// 命中「已拆分且源未变化」跳过，必须重拆并把旧 source 落点产物与索引行收敛，
// 新记录 target/落点/产物主干与实际一致。
func TestAssetSplit_TargetSwitchSourceToCacheReclones(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "源到缓存切换库", dir)
	buildAssetsPptx(t, dir, "切换源.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	first, err := svc.SplitAssetFile(context.Background(), lib.ID, "切换源.pptx", "source")
	if err != nil {
		t.Fatalf("首次 source 拆分: %v", err)
	}
	if first.Skipped || first.Target != model.AssetSplitTargetSource {
		t.Fatalf("首次拆分应成功且落点 source：%+v", first)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "切换源.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetSource || rec.artifactStem != "切换源" {
		t.Fatalf("source 落点记录应持久化 target/主干：%+v", rec)
	}
	oldDir := filepath.Join(dir, "切换源")
	oldPage1 := filepath.Join(oldDir, "切换源-001.pptx")
	oldPage2 := filepath.Join(oldDir, "切换源-002.pptx")
	mustExist(t, oldPage1)
	mustExist(t, oldPage2)
	// 旧落点产物被扫描索引（source 落点产物实际不进索引，此处仅验证清理联动面）
	insertIndexRowForTest(t, svc, lib.ID, 9951, "切换源/切换源-001.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9952, "切换源/切换源-002.pptx")

	// 源内容未变、仅切换落点：必须重拆（旧实现按哈希 + 首页产物存在直接跳过）
	second, err := svc.SplitAssetFile(context.Background(), lib.ID, "切换源.pptx", "cache")
	if err != nil {
		t.Fatalf("切换 cache 重拆: %v", err)
	}
	if second.Skipped || second.Target != model.AssetSplitTargetCache || second.PageCount != 2 {
		t.Fatalf("落点切换不得跳过，应完成 2 页 cache 重拆：%+v", second)
	}
	mustNotExist(t, oldPage1)
	mustNotExist(t, oldPage2)
	for _, rel := range []string{"切换源/切换源-001.pptx", "切换源/切换源-002.pptx"} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 0 {
			t.Errorf("旧 source 落点产物索引行应被清理（%s），实际 %d 行", rel, n)
		}
	}
	wantOut := filepath.Join(svc.assetsDir, lib.ID, "derived", "切换源")
	if !samePathFold(second.OutputDir, wantOut) {
		t.Errorf("重拆结果 OutputDir 应与实际 cache 落点一致：want %q, got %q", wantOut, second.OutputDir)
	}
	rec = splitRecordRowForTest(t, svc, lib.ID, "切换源.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetCache ||
		rec.artifactStem != "切换源" || !samePathFold(rec.outputDir, wantOut) {
		t.Errorf("重拆后记录应为 cache 落点、主干保留且落点目录更新：%+v", rec)
	}
}

// TestAssetSplit_TargetSwitchCacheToSource cache→source 反向：源未变同样不得跳过，
// 旧 derived 落点产物收敛，新 source 落点产物生成。
func TestAssetSplit_TargetSwitchCacheToSource(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "缓存到源切换库", dir)
	buildAssetsPptx(t, dir, "切换缓存.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	first, err := svc.SplitAssetFile(context.Background(), lib.ID, "切换缓存.pptx", "cache")
	if err != nil {
		t.Fatalf("首次 cache 拆分: %v", err)
	}
	if first.Skipped || first.Target != model.AssetSplitTargetCache {
		t.Fatalf("首次拆分应成功且落点 cache：%+v", first)
	}
	oldOut := filepath.Join(svc.assetsDir, lib.ID, "derived", "切换缓存")
	mustExist(t, filepath.Join(oldOut, "切换缓存-001.pptx"))
	mustExist(t, filepath.Join(oldOut, "切换缓存-002.pptx"))

	second, err := svc.SplitAssetFile(context.Background(), lib.ID, "切换缓存.pptx", "source")
	if err != nil {
		t.Fatalf("切换 source 重拆: %v", err)
	}
	if second.Skipped || second.Target != model.AssetSplitTargetSource || second.PageCount != 2 {
		t.Fatalf("落点切换不得跳过，应完成 2 页 source 重拆：%+v", second)
	}
	mustNotExist(t, filepath.Join(oldOut, "切换缓存-001.pptx"))
	mustNotExist(t, filepath.Join(oldOut, "切换缓存-002.pptx"))
	wantOut := filepath.Join(dir, "切换缓存")
	if !samePathFold(second.OutputDir, wantOut) {
		t.Errorf("重拆结果 OutputDir 应与实际 source 落点一致：want %q, got %q", wantOut, second.OutputDir)
	}
	for _, page := range []int{1, 2} {
		mustExist(t, filepath.Join(wantOut, fmt.Sprintf("切换缓存-%03d.pptx", page)))
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "切换缓存.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetSource ||
		rec.artifactStem != "切换缓存" || !samePathFold(rec.outputDir, wantOut) {
		t.Errorf("重拆后记录应为 source 落点、主干保留且落点目录更新：%+v", rec)
	}
}

// TestAssetSplit_SameDirReplaceToSourceCleansOldForm 同目录跨落点（replace→source，
// 主干同名）：旧 `<页序>-<主干>.pptx` 与索引行清理，新 `<主干>-<页序>.pptx` 保留。
func TestAssetSplit_SameDirReplaceToSourceCleansOldForm(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同目录跨落点库", dir)
	buildAssetsPptx(t, dir, "方案.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var hash string
	if err := store.db.QueryRow("SELECT hash FROM files WHERE path = ?", "方案.pptx").Scan(&hash); err != nil {
		t.Fatalf("查源哈希: %v", err)
	}
	const splitAt = "2026-11-02 10:00:00"
	outDir := filepath.Join(dir, "方案")
	oldPage1 := filepath.Join(outDir, replaceArtifactName(1, "方案"))
	oldPage2 := filepath.Join(outDir, replaceArtifactName(2, "方案"))
	writeSplitArtifactAt(t, oldPage1, "old-1", splitAt)
	writeSplitArtifactAt(t, oldPage2, "old-2", splitAt)
	// 上次 replace 记录的产物已进索引
	insertIndexRowForTest(t, svc, lib.ID, 9961, "方案/"+filepath.Base(oldPage1))
	insertIndexRowForTest(t, svc, lib.ID, 9962, "方案/"+filepath.Base(oldPage2))
	if err := store.upsertSplitRecord("方案.pptx", hash, outDir, 2,
		splitAt, model.AssetSplitTargetReplace, "方案", false, nil); err != nil {
		t.Fatalf("写入上次 replace 记录: %v", err)
	}

	// 源未变、同目录、仅落点切换：旧实现会按哈希 + 首页产物存在直接跳过
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "source")
	if err != nil {
		t.Fatalf("同目录切换 source 重拆: %v", err)
	}
	if res.Skipped || res.Target != model.AssetSplitTargetSource || res.PageCount != 2 {
		t.Fatalf("同目录跨落点不得跳过，应完成 2 页 source 重拆：%+v", res)
	}
	mustNotExist(t, oldPage1)
	mustNotExist(t, oldPage2)
	for _, rel := range []string{"方案/" + filepath.Base(oldPage1), "方案/" + filepath.Base(oldPage2)} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 0 {
			t.Errorf("旧 replace 形态产物索引行应被清理（%s），实际 %d 行", rel, n)
		}
	}
	for _, page := range []int{1, 2} {
		mustNotExist(t, filepath.Join(outDir, replaceArtifactName(page, "方案"))) // replace 形态已清
		mustExist(t, filepath.Join(outDir, fmt.Sprintf("方案-%03d.pptx", page)))
	}
	if !samePathFold(res.OutputDir, outDir) {
		t.Errorf("重拆结果 OutputDir 应与实际 source 落点一致：want %q, got %q", outDir, res.OutputDir)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "方案.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetSource || rec.artifactStem != "方案" {
		t.Errorf("重拆后记录应为 source 落点且主干保留：%+v", rec)
	}
}

// TestCleanupSplitArtifactsInDirByPage_UserFileNewerMtimeKept mtime 守卫：产物
// （mtime 早于 split_at）照常清理；用户后放入的同主干同页序文件（mtime 晚于
// split_at + 容差，如 `旧名-003.pptx`）保留，索引行不动。
func TestCleanupSplitArtifactsInDirByPage_UserFileNewerMtimeKept(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "mtime 守卫库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	const splitAt = "2026-10-30 10:00:00"
	outDir := filepath.Join(dir, "旧名")
	page1 := filepath.Join(outDir, "旧名-001.pptx")
	page2 := filepath.Join(outDir, "旧名-002.pptx")
	userPage3 := filepath.Join(outDir, "旧名-003.pptx")
	writeSplitArtifactAt(t, page1, "p1", splitAt)
	writeSplitArtifactAt(t, page2, "p2", splitAt)
	testutil.WriteFile(t, userPage3, "user") // mtime = now，晚于 split_at
	insertIndexRowForTest(t, svc, lib.ID, 9971, "旧名/旧名-001.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9972, "旧名/旧名-002.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9973, "旧名/旧名-003.pptx")

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     3,
		splitAt:       splitAt,
		target:        model.AssetSplitTargetSource,
		artifactStem:  "旧名",
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, page1)
	mustNotExist(t, page2)
	mustExist(t, userPage3)
	for _, tc := range []struct {
		rel  string
		want int
	}{
		{"旧名/旧名-001.pptx", 0},
		{"旧名/旧名-002.pptx", 0},
		{"旧名/旧名-003.pptx", 1},
	} {
		if n := countIndexRowsByPath(t, svc, lib.ID, tc.rel); n != tc.want {
			t.Errorf("索引行数不符（%s）：want %d, got %d", tc.rel, tc.want, n)
		}
	}
	mustExist(t, outDir) // 用户文件仍在，目录不删
}

// TestIsSplitArtifactAfterSplitAt mtime 守卫判定口径：晚于 split_at + 容差判用户
// 文件（true）；容差内/早于判产物；split_at 解析失败守卫不生效（false）；Stat
// 失败按不可判定保留（true）。
func TestIsSplitArtifactAfterSplitAt(t *testing.T) {
	dir := t.TempDir()
	const splitAt = "2026-10-30 10:00:00"
	base := time.Date(2026, 10, 30, 10, 0, 0, 0, time.Local)
	writeAt := func(name string, mtime time.Time) string {
		t.Helper()
		p := filepath.Join(dir, name)
		testutil.WriteFile(t, p, name)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatalf("设置 mtime（%s）: %v", name, err)
		}
		return p
	}
	oldPath := writeAt("旧产物.pptx", base.Add(-time.Minute))
	gracePath := writeAt("容差内.pptx", base.Add(time.Second))
	newPath := writeAt("用户新文件.pptx", base.Add(splitArtifactUserFileGrace+time.Second))

	for _, tc := range []struct {
		name    string
		path    string
		splitAt string
		want    bool
	}{
		{"产物早于 split_at", oldPath, splitAt, false},
		{"容差内仍属产物", gracePath, splitAt, false},
		{"晚于 split_at+容差为用户文件", newPath, splitAt, true},
		{"Stat 失败按不可判定保留", filepath.Join(dir, "不存在.pptx"), splitAt, true},
		{"split_at 解析失败守卫不生效", oldPath, "非法时间", false},
	} {
		if got := isSplitArtifactAfterSplitAt(tc.path, tc.splitAt); got != tc.want {
			t.Errorf("%s：want %v, got %v", tc.name, tc.want, got)
		}
	}
}

// TestSplitArtifactCleanupPlan 清理门控决策单点：异目录与同目录跨 target 均清理
// （excludeStem 空）；同目录同 target 改主干（M1）以本次新主干排除；Windows 下
// 仅大小写差异折叠视为同一主干不进入清理（新产物覆盖同名物理文件），非 Windows
// 大小写敏感按不同主干清理旧组；普通重拆与 nil 记录不清理。
func TestSplitArtifactCleanupPlan(t *testing.T) {
	const outDir = `C:\lib\方案`
	cases := []struct {
		name     string
		record   *assetSplitRecord
		target   string
		stem     string
		wantOK   bool
		wantDir  string
		wantExcl string
	}{
		{
			name:   "nil 记录不清理",
			record: nil,
		},
		{
			name:     "异目录清理旧落点",
			record:   &assetSplitRecord{outputDir: `C:\lib\旧`, target: model.AssetSplitTargetCache, artifactStem: "旧名"},
			target:   model.AssetSplitTargetCache,
			stem:     "旧名",
			wantOK:   true,
			wantDir:  `C:\lib\旧`,
			wantExcl: "",
		},
		{
			name:     "同目录跨 target 清理旧形态",
			record:   &assetSplitRecord{outputDir: outDir, target: model.AssetSplitTargetReplace, artifactStem: "方案"},
			target:   model.AssetSplitTargetSource,
			stem:     "方案",
			wantOK:   true,
			wantDir:  outDir,
			wantExcl: "",
		},
		{
			name:     "同目录同 target 改主干以新主干排除",
			record:   &assetSplitRecord{outputDir: outDir, target: model.AssetSplitTargetSource, artifactStem: "旧名"},
			target:   model.AssetSplitTargetSource,
			stem:     "方案",
			wantOK:   true,
			wantDir:  outDir,
			wantExcl: "方案",
		},
		{
			name:   "同目录同 target 同主干不清理",
			record: &assetSplitRecord{outputDir: outDir, target: model.AssetSplitTargetCache, artifactStem: "方案"},
			target: model.AssetSplitTargetCache,
			stem:   "方案",
		},
		{
			name:   "同目录同 target 记录主干为空不清理",
			record: &assetSplitRecord{outputDir: outDir, target: model.AssetSplitTargetCache},
			target: model.AssetSplitTargetCache,
			stem:   "方案",
		},
	}
	for _, tc := range cases {
		gotDir, gotExcl, gotOK := splitArtifactCleanupPlan(tc.record, tc.target, tc.stem, outDir)
		if gotOK != tc.wantOK || gotDir != tc.wantDir || gotExcl != tc.wantExcl {
			t.Errorf("%s：want (%q, %q, %v), got (%q, %q, %v)",
				tc.name, tc.wantDir, tc.wantExcl, tc.wantOK, gotDir, gotExcl, gotOK)
		}
	}
	// 仅大小写差异（Abc vs ABC）：折叠守卫按平台分流
	rec := &assetSplitRecord{outputDir: outDir, target: model.AssetSplitTargetSource, artifactStem: "Abc"}
	gotDir, gotExcl, gotOK := splitArtifactCleanupPlan(rec, model.AssetSplitTargetSource, "ABC", outDir)
	if runtime.GOOS == "windows" {
		// Windows：折叠相等视为同一主干，不清理（本次产物覆盖同名物理文件）
		if gotOK {
			t.Errorf("Windows 下仅大小写差异应视为同一主干不清理，got (%q, %q, %v)", gotDir, gotExcl, gotOK)
		}
	} else {
		// Linux/macOS：大小写敏感，Abc 与 ABC 是不同主干，清理旧组并排除新主干
		if !gotOK || gotDir != outDir || gotExcl != "ABC" {
			t.Errorf("非 Windows 下大小写变体应按改主干清理，got (%q, %q, %v)", gotDir, gotExcl, gotOK)
		}
	}
}

// TestAssetSplit_CaseOnlyRenameKeepsNewArtifact 仅大小写改名（Windows 大小写不
// 敏感）后重拆：本次新产物不被清理删除。M1 门控把折叠相等视为同一主干、不进入
// 清理分支；excludeStem 精确比较也不再让整组候选被折叠误跳过。
func TestAssetSplit_CaseOnlyRenameKeepsNewArtifact(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 大小写不敏感文件系统覆盖")
	}
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "大小写改名库", dir)
	src := buildAssetsPptx(t, dir, "Abc.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "Abc.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查源文件记录: %v", err)
	}
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "Abc.pptx", "source"); err != nil {
		t.Fatalf("首次 source 拆分: %v", err)
	}

	// 仅大小写改名并重链：记录键迁到 ABC.pptx，output_dir 与原目录折叠等价保持不变
	newAbs := filepath.Join(dir, "ABC.pptx")
	if err := os.Rename(src, newAbs); err != nil {
		t.Fatalf("大小写改名: %v", err)
	}
	if _, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false); err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	const newRel = "ABC.pptx"
	// 源内容变化 + 库存哈希同步（确定性驱动重拆，不依赖 mtime 时序）
	buildAssetsPptx(t, dir, "ABC.pptx", []assetsSlideSpec{
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
		t.Fatalf("大小写改名后重拆: %v", err)
	}
	if res.Skipped || res.PageCount != 2 || len(res.Files) != 2 {
		t.Fatalf("应完成 2 页重拆：%+v", res)
	}
	for _, p := range res.Files {
		mustExist(t, p)
		assertValidZip(t, p)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, newRel)
	if rec == nil || rec.artifactStem != "ABC" || rec.target != model.AssetSplitTargetSource {
		t.Errorf("重拆后记录主干应为本次新主干 ABC：%+v", rec)
	}
}
