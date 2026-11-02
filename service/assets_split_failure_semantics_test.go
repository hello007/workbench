package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- 拆分失败语义与清理可收敛性（11-02-split-failure-semantics） ----------
//
// 三处缺陷的回归面：
//   - source/cache 落点重拆改为「核心成功后才清理超界旧页」，核心失败/取消时上一次
//     成果原样保留（此前核心调用前已删旧页 / 已 RemoveAll 整目录）；
//   - isReplaceArtifactName 大小写不敏感（Windows 仅大小写改名后 replace 重拆不再
//     因误判「非历史产物」永久回滚失败）；
//   - 单页删除失败先重试一次，仍失败告警明确「残留需人工清理，不再自动收敛」。

// TestRemoveSplitArtifactsBeyond 超界旧页清理：仅删 `<主干>-<纯数字>.pptx` 且页序
// > keepPage 的文件，用户自建同名异构文件与普通文件不动；keepPage=0 等价全量清理。
func TestRemoveSplitArtifactsBeyond(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"讲义-001.pptx", "讲义-002.pptx", "讲义-003.pptx",
		"讲义-004.pptx", "讲义-010.pptx",
		"讲义-草稿.pptx", "说明.txt",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n), 0644); err != nil {
			t.Fatalf("放置文件（%s）: %v", n, err)
		}
	}

	removeSplitArtifactsBeyond(dir, "讲义", 3)

	for _, n := range []string{"讲义-001.pptx", "讲义-002.pptx", "讲义-003.pptx", "讲义-草稿.pptx", "说明.txt"} {
		mustExist(t, filepath.Join(dir, n))
	}
	for _, n := range []string{"讲义-004.pptx", "讲义-010.pptx"} {
		mustNotExist(t, filepath.Join(dir, n))
	}

	// keepPage=0：全部产物形态（含页序 1..3）清理
	removeSplitArtifactsBeyond(dir, "讲义", 0)
	for _, n := range []string{"讲义-001.pptx", "讲义-002.pptx", "讲义-003.pptx"} {
		mustNotExist(t, filepath.Join(dir, n))
	}
	mustExist(t, filepath.Join(dir, "讲义-草稿.pptx"))
	mustExist(t, filepath.Join(dir, "说明.txt"))
}

// TestAssetSplit_CacheCoreFailureKeepsOldArtifacts cache 落点重拆核心失败：上一次
// 产物原样保留（不再前置 RemoveAll 整目录），记录不变。
func TestAssetSplit_CacheCoreFailureKeepsOldArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "缓存失败保留库", dir)
	src := buildAssetsPptx(t, dir, "保底.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "保底.pptx", "cache"); err != nil {
		t.Fatalf("首次拆分: %v", err)
	}
	outDir := filepath.Join(svc.assetsDir, lib.ID, "derived", "保底")
	old1 := filepath.Join(outDir, "保底-001.pptx")
	old2 := filepath.Join(outDir, "保底-002.pptx")
	mustExist(t, old1)
	mustExist(t, old2)

	// 源变为 3 页 + 库存哈希同步（确定性驱动重拆，不依赖 mtime 时序）
	buildAssetsPptx(t, dir, "保底.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"内容"}},
		{titlePh: "新二", body: []string{"内容"}},
		{titlePh: "新三", body: []string{"内容"}},
	})
	newHash, err := hashFileSHA256(src)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, "保底.pptx"); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}

	svc.splitCoreFn = func(context.Context, string, func(done, total int)) (model.PptxSplitResult, error) {
		return model.PptxSplitResult{}, errors.New("核心拆分失败（模拟源文件损坏）")
	}
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "保底.pptx", "cache"); err == nil {
		t.Fatal("核心拆分失败应返回错误")
	}
	// 旧产物完整保留且仍为合法产物
	for _, p := range []string{old1, old2} {
		mustExist(t, p)
		assertValidZip(t, p)
	}
	// 记录不变（仍为 2 页，未被失败重拆覆盖）
	view, err := svc.GetAssetSplitRecord(lib.ID, "保底.pptx")
	if err != nil || !view.Exists || view.PageCount != 2 {
		t.Errorf("核心失败不得改写拆分记录：%+v (err=%v)", view, err)
	}
}

// TestResolveExistingDirCase 落点目录大小写归一：磁盘实际条目名唯一命中时返回
// 实际形态；目录不存在返回原值；多命中（大小写敏感目录标志开启，仅非 Windows
// 可构造）不可判定返回原值。
func TestResolveExistingDirCase(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "abc")
	if err := os.MkdirAll(actual, 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	if got := resolveExistingDirCase(filepath.Join(root, "Abc")); !samePathFold(got, actual) || got != actual {
		t.Errorf("应归一为磁盘实际名 %q，实际 %q", actual, got)
	}
	if got := resolveExistingDirCase(actual); got != actual {
		t.Errorf("大小写已一致时应原样返回，实际 %q", got)
	}
	missing := filepath.Join(root, "不存在")
	if got := resolveExistingDirCase(missing); got != missing {
		t.Errorf("目录不存在时应原样返回，实际 %q", got)
	}
	if runtime.GOOS != "windows" {
		// 大小写敏感文件系统上 abc 与 Abc 可并存，折叠命中 >1 视为不可判定
		if err := os.MkdirAll(filepath.Join(root, "Abc"), 0755); err != nil {
			t.Fatalf("建大小写变体目录: %v", err)
		}
		want := filepath.Join(root, "ABc")
		if got := resolveExistingDirCase(want); got != want {
			t.Errorf("折叠多命中时应原样返回 %q，实际 %q", want, got)
		}
	}
}

// TestAssetSplit_SourceCoreFailureKeepsPageDecreaseArtifacts 源目录落点同目录重拆
// 核心失败：上一次的**全部**旧页原样保留。修复前 source 落点在核心调用前就
// removeSplitArtifacts 删掉产物命名形态的文件，核心失败后旧成果无法恢复。
func TestAssetSplit_SourceCoreFailureKeepsPageDecreaseArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "源失败保留库", dir)
	src := buildAssetsPptx(t, dir, "原稿.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "原稿.pptx", "source"); err != nil {
		t.Fatalf("首次拆分: %v", err)
	}
	outDir := filepath.Join(dir, "原稿")

	// 源改为 2 页（页数减少形态）+ 库存哈希同步，确定性驱动重拆
	buildAssetsPptx(t, dir, "原稿.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"内容"}},
		{titlePh: "新二", body: []string{"内容"}},
	})
	newHash, err := hashFileSHA256(src)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, "原稿.pptx"); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}

	svc.splitCoreFn = func(context.Context, string, func(done, total int)) (model.PptxSplitResult, error) {
		return model.PptxSplitResult{}, errors.New("核心拆分失败（模拟源文件损坏）")
	}
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "原稿.pptx", "source"); err == nil {
		t.Fatal("核心拆分失败应返回错误")
	}
	// 页数减少前产出的三页全部保留且仍为合法产物（核心失败不触达超界清理）
	for _, n := range []string{"原稿-001.pptx", "原稿-002.pptx", "原稿-003.pptx"} {
		p := filepath.Join(outDir, n)
		mustExist(t, p)
		assertValidZip(t, p)
	}
}

// TestAssetSplit_ResplitPageIncreaseKeepsAll 页数增加重拆：旧页全部覆盖保留，新页
// 一并产出，超界清理不误删（keepPage = 新页数）。
func TestAssetSplit_ResplitPageIncreaseKeepsAll(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "页数增加库", dir)
	src := buildAssetsPptx(t, dir, "扩展.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "扩展.pptx", "source"); err != nil {
		t.Fatalf("首次拆分: %v", err)
	}
	outDir := filepath.Join(dir, "扩展")

	// 源 2 页 → 3 页 + 库存哈希同步
	buildAssetsPptx(t, dir, "扩展.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"内容"}},
		{titlePh: "新二", body: []string{"内容"}},
		{titlePh: "新三", body: []string{"内容"}},
	})
	newHash, err := hashFileSHA256(src)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, "扩展.pptx"); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "扩展.pptx", "source")
	if err != nil {
		t.Fatalf("页数增加重拆: %v", err)
	}
	if res.Skipped || res.PageCount != 3 || len(res.Files) != 3 {
		t.Fatalf("页数增加应产出 3 页：%+v", res)
	}
	for _, page := range []string{"扩展-001.pptx", "扩展-002.pptx", "扩展-003.pptx"} {
		p := filepath.Join(outDir, page)
		mustExist(t, p)
		assertValidZip(t, p)
	}
}

// TestIsReplaceArtifactName_CaseFold 前缀/后缀判定大小写不敏感：仅大小写差异的
// 历史产物（001-Abc.pptx vs 主干 abc）与核心中间态（Abc-001.pptx）都须识别。
func TestIsReplaceArtifactName_CaseFold(t *testing.T) {
	cases := []struct {
		name string
		base string
		want bool
	}{
		{"001-abc.pptx", "abc", true},
		{"001-Abc.pptx", "abc", true},
		{"001-abc.pptx", "Abc", true},
		{"001-ABC.pptx", "abc", true},
		{"Abc-001.pptx", "abc", true},
		{"abc-001.pptx", "Abc", true},
		{"001-abc.PPTX", "abc", true},
		{"001-123-p.pptx", "123", true},
		{"123-001.pptx", "123", true},
		{"001-abc-extra.pptx", "abc", false},
		{"abc-草稿.pptx", "abc", false},
		{"说明.txt", "abc", false},
		{"001-abc.pptx", "other", false},
		// 多字节主干：切片按字节长度取，须与 rune 边界对齐不截半
		{"001-讲义.pptx", "讲义", true},
		{"讲义-001.pptx", "讲义", true},
		{"讲义-001.PPTX", "讲义", true},
		{"001-讲义-extra.pptx", "讲义", false},
		{"讲义-草稿.pptx", "讲义", false},
		{"讲义-001.pptx", "讲义A", false},
		{"讲义A-001.pptx", "讲义", false},
	}
	for _, tc := range cases {
		if got := isReplaceArtifactName(tc.name, tc.base); got != tc.want {
			t.Errorf("isReplaceArtifactName(%q, %q) = %v, want %v", tc.name, tc.base, got, tc.want)
		}
	}
}

// TestSnapshotReplaceArtifactNames_CaseFold 目录快照随之大小写折叠：主干 Abc 时
// 磁盘上的 001-abc.pptx / Abc-001.pptx 都计入历史产物，用户文件不计入。
func TestSnapshotReplaceArtifactNames_CaseFold(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"001-abc.pptx", "Abc-002.pptx", "002-Abc.pptx", "说明.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n), 0644); err != nil {
			t.Fatalf("放置文件（%s）: %v", n, err)
		}
	}
	got := snapshotReplaceArtifactNames(dir, "Abc")
	for _, n := range []string{"001-abc.pptx", "Abc-002.pptx", "002-Abc.pptx"} {
		if _, ok := got[n]; !ok {
			t.Errorf("快照应折叠命中 %q，实际 %v", n, got)
		}
	}
	if _, ok := got["说明.txt"]; ok {
		t.Errorf("用户文件不应计入历史产物：%v", got)
	}
	if _, hit := foldNameInSet(got, "001-ABC.PPTX"); !hit {
		t.Error("foldNameInSet 应大小写不敏感命中")
	}
}

// TestAssetSplitReplace_CaseOnlyRenameResplits Windows 上仅大小写改名后 replace
// 重拆：上次产物 001-abc.pptx 与本次目标 001-Abc.pptx 是同一物理文件，须识别为
// 历史产物（备份让位）而非「非历史产物已存在」拒绝；页数减少的越界旧页收敛。
// 非 Windows 跳过（大小写敏感文件系统无此语义）。
func TestAssetSplitReplace_CaseOnlyRenameResplits(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 大小写不敏感文件系统覆盖")
	}
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换大小写库", dir)
	buildAssetsPptx(t, dir, "Abc.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	var hash string
	if err := store.db.QueryRow("SELECT hash FROM files WHERE path = ?", "Abc.pptx").Scan(&hash); err != nil {
		t.Fatalf("查源哈希: %v", err)
	}

	// 模拟「首次 replace 拆分（源主干 abc）后仅大小写改名并重链」的落库状态：记录键
	// 已迁到 Abc.pptx，落点目录与产物主干冻结为旧大小写 abc。
	outDir := filepath.Join(dir, "abc")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("建产物目录: %v", err)
	}
	const splitAt = "2026-11-02 10:00:00"
	old1 := filepath.Join(outDir, "001-abc.pptx")
	old2 := filepath.Join(outDir, "002-abc.pptx")
	old3 := filepath.Join(outDir, "003-abc.pptx")
	for p, c := range map[string]string{old1: "old-1", old2: "old-2", old3: "old-3"} {
		writeSplitArtifactAt(t, p, c, splitAt)
	}
	// 上次产物已进索引（目录与文件名均为旧大小写形态）
	for i, n := range []string{"001-abc.pptx", "002-abc.pptx", "003-abc.pptx"} {
		insertIndexRowForTest(t, svc, lib.ID, int64(9801+i), "abc/"+n)
	}
	if err := store.upsertSplitRecord("Abc.pptx", hash, outDir, 3,
		splitAt, model.AssetSplitTargetReplace, "abc", false); err != nil {
		t.Fatalf("写入上次 replace 记录: %v", err)
	}

	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "Abc.pptx", "replace")
	if err != nil {
		t.Fatalf("大小写折叠后 replace 重拆应成功: %v", err)
	}
	if res.Skipped || res.PageCount != 2 || !res.SourceRemoved {
		t.Fatalf("应完成 2 页替换并删源：%+v", res)
	}
	for _, page := range []string{"001-Abc.pptx", "002-Abc.pptx"} {
		p := filepath.Join(outDir, page)
		mustExist(t, p)
		if got, readErr := os.ReadFile(p); readErr != nil || string(got) != "new-bytes" {
			t.Errorf("新产物内容应为本次拆分结果（%s），实际 %q err=%v", p, got, readErr)
		}
	}
	// 新页已进索引且路径沿用磁盘实际目录大小写（abc，非当前源名派生的 Abc）：
	// 每页恰一条，旧大小写形态的行（含越界页）全部清除，不留同物理文件双行
	for _, rel := range []string{"abc/001-Abc.pptx", "abc/002-Abc.pptx"} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 1 {
			t.Errorf("新产物索引行应为 1 行（%s），实际 %d 行", rel, n)
		}
	}
	for _, rel := range []string{"abc/001-abc.pptx", "abc/002-abc.pptx", "abc/003-abc.pptx"} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 0 {
			t.Errorf("旧大小写形态索引行应清除（%s），实际 %d 行", rel, n)
		}
	}
	// 再扫一轮：索引路径与磁盘实际名一致，不得产生幽灵 lost 行
	mustScanLibrary(t, svc, lib.ID)
	var lost int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files WHERE status = 'lost'").Scan(&lost); err != nil {
		t.Fatalf("统计 lost 行: %v", err)
	}
	if lost != 0 {
		t.Errorf("重拆后不应出现幽灵 lost 行，实际 %d 行", lost)
	}
	// 页数 3→2：越界旧页收敛（不同名，可直接断言不存在）
	mustNotExist(t, old3)
	// 无备份残留（备份让位后成功路径删除）
	entries, readErr := os.ReadDir(outDir)
	if readErr != nil {
		t.Fatalf("读产物目录: %v", readErr)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), replaceArtifactBackupSuffix) {
			t.Errorf("成功路径不应残留备份文件：%q", e.Name())
		}
	}
	if got := recycleCalls(); len(got) != 1 {
		t.Errorf("回收站应被调用一次，实际 %v", got)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "Abc.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetReplace || rec.artifactStem != "Abc" {
		t.Errorf("重拆后记录主干应为本次新主干 Abc：%+v", rec)
	}
}

// 注：删除失败重试分支难以在不依赖平台权限语义的前提下稳定构造（chmod 在 Windows
// 无效），由 removeSplitArtifactFile 的实现单点覆盖：首次失败 50ms 后重试一次，
// 仍失败返回错误、调用方告警明确「残留需人工清理，本次重拆不再自动收敛」。
// 该告警文案的语义由 docs/spec/pptx-split.md §9.5 登记。
