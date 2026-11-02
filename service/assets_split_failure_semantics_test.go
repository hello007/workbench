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
	"workbench/util/testutil"
)

// ---------- 拆分失败语义与清理可收敛性（11-02-split-failure-semantics） ----------
//
// 三处缺陷的回归面：
//   - source/cache 落点重拆改为「核心成功后才清理超界旧页」，核心失败/取消时上一次
//     成果原样保留（此前核心调用前已删旧页 / 已 RemoveAll 整目录）；
//   - isReplaceArtifactName 大小写不敏感（Windows 仅大小写改名后 replace 重拆不再
//     因误判「非历史产物」永久回滚失败）；
//   - 单页删除失败先重试一次，仍失败告警明确「残留需人工清理，不再自动收敛」。

// TestRemoveSplitArtifactsBeyond 旧页清理：删页序 > keepPage 的产物，以及页序
// ≤ keepPage 但为历史 `%02d` 形态（非当前 `%03d`）的文件；当前形态正常页、用户
// 自建同名异构文件与普通文件不动；keepPage=0 等价全量清理产物形态。
func TestRemoveSplitArtifactsBeyond(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "超界清理库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	names := []string{
		"讲义-001.pptx", "讲义-002.pptx", "讲义-003.pptx",
		"讲义-004.pptx", "讲义-010.pptx",
		// 历史 %02d 形态（页序 1..3 在 keepPage 范围内但非当前 3 位形态）
		"讲义-01.pptx", "讲义-02.pptx", "讲义-03.pptx",
		"讲义-草稿.pptx", "说明.txt",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n), 0644); err != nil {
			t.Fatalf("放置文件（%s）: %v", n, err)
		}
	}
	insertIndexRowForTest(t, svc, lib.ID, 9901, "讲义-004.pptx")

	svc.removeSplitArtifactsBeyond(store, lib, nil, dir, "讲义", 3, "")

	for _, n := range []string{"讲义-001.pptx", "讲义-002.pptx", "讲义-003.pptx", "讲义-草稿.pptx", "说明.txt"} {
		mustExist(t, filepath.Join(dir, n))
	}
	for _, n := range []string{"讲义-004.pptx", "讲义-010.pptx", "讲义-01.pptx", "讲义-02.pptx", "讲义-03.pptx"} {
		mustNotExist(t, filepath.Join(dir, n))
	}
	// 删除的越界旧页联动清理索引行
	if n := countIndexRowsByPath(t, svc, lib.ID, "讲义-004.pptx"); n != 0 {
		t.Errorf("越界旧页索引行应被清理，实际 %d 行", n)
	}

	// keepPage=0：全部产物形态（含页序 1..3 当前形态）清理，用户文件保留
	svc.removeSplitArtifactsBeyond(store, lib, nil, dir, "讲义", 0, "")
	for _, n := range []string{"讲义-001.pptx", "讲义-002.pptx", "讲义-003.pptx"} {
		mustNotExist(t, filepath.Join(dir, n))
	}
	mustExist(t, filepath.Join(dir, "讲义-草稿.pptx"))
	mustExist(t, filepath.Join(dir, "说明.txt"))
}

// TestRemoveSplitArtifactsBeyond_LegacyTwoDigitConverged 历史 %02d 产物与当前
// %03d 产物并存：页序相同（如 01 与 001）时当前形态保留、两位形态删除——此前
// 只放 42/99 等越界页的 fixture 形成假阳性覆盖（旧两位形态在 keepPage 内漏清）。
func TestRemoveSplitArtifactsBeyond_LegacyTwoDigitConverged(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "两位页序收敛库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	for _, n := range []string{"演变-001.pptx", "演变-002.pptx", "演变-003.pptx", "演变-01.pptx", "演变-02.pptx"} {
		testutil.WriteFile(t, filepath.Join(dir, n), n)
	}

	svc.removeSplitArtifactsBeyond(store, lib, nil, dir, "演变", 3, "")

	for _, n := range []string{"演变-001.pptx", "演变-002.pptx", "演变-003.pptx"} {
		mustExist(t, filepath.Join(dir, n))
	}
	for _, n := range []string{"演变-01.pptx", "演变-02.pptx"} {
		mustNotExist(t, filepath.Join(dir, n))
	}
}

// TestRemoveSplitArtifactsBeyond_CaseFoldBase Windows 仅大小写改名：base 为
// `abc`、目录内是改名前的 `Abc-NNN.pptx`。仅 Windows 识别为产物（sameNameFold
// 折叠），越界页 003 删除、当前形态 001/002 保留；非 Windows 大小写敏感，三个
// 文件均视为用户文件不识别、零删除。
func TestRemoveSplitArtifactsBeyond_CaseFoldBase(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "大小写折叠收敛库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	for _, n := range []string{"Abc-001.pptx", "Abc-002.pptx", "Abc-003.pptx"} {
		testutil.WriteFile(t, filepath.Join(dir, n), n)
	}

	svc.removeSplitArtifactsBeyond(store, lib, nil, dir, "abc", 2, "")

	mustExist(t, filepath.Join(dir, "Abc-001.pptx"))
	mustExist(t, filepath.Join(dir, "Abc-002.pptx"))
	if runtime.GOOS == "windows" {
		mustNotExist(t, filepath.Join(dir, "Abc-003.pptx"))
		return
	}
	mustExist(t, filepath.Join(dir, "Abc-003.pptx")) // 非 Windows：主干不符，不识别
}

// TestRemoveSplitArtifactsBeyond_UserFileAfterSplitAtKept mtime 守卫：候选文件
// mtime 晚于 splitAt + splitArtifactUserFileGrace 视为用户后放入的同名文件保留
// （越界页与历史 %02d 两种形态都受守卫）；早于 splitAt 的产物照常删除。
func TestRemoveSplitArtifactsBeyond_UserFileAfterSplitAtKept(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "mtime 守卫库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	const splitAt = "2026-10-30 10:00:00"
	// 产物：mtime 回拨到 splitAt 之前 → 删除
	writeSplitArtifactAt(t, filepath.Join(dir, "守卫-004.pptx"), "product", splitAt)
	writeSplitArtifactAt(t, filepath.Join(dir, "守卫-01.pptx"), "legacy", splitAt)
	// 用户后放入：mtime = now（远晚于 splitAt + 2s）→ 保留
	testutil.WriteFile(t, filepath.Join(dir, "守卫-005.pptx"), "user")
	testutil.WriteFile(t, filepath.Join(dir, "守卫-02.pptx"), "user-legacy")

	svc.removeSplitArtifactsBeyond(store, lib, nil, dir, "守卫", 3, splitAt)

	mustNotExist(t, filepath.Join(dir, "守卫-004.pptx"))
	mustNotExist(t, filepath.Join(dir, "守卫-01.pptx"))
	mustExist(t, filepath.Join(dir, "守卫-005.pptx"))
	mustExist(t, filepath.Join(dir, "守卫-02.pptx"))
}

// TestSplitArtifactPageSeq 产物名页序解析单点：从每个 '-' 位置尝试以主干前缀
// 切分，主干自身含 '-' 或纯数字（`a-b-001.pptx`、`123-001.pptx`）仍可正确取到
// 页序段；页序段必须为纯数字、扩展名必须是 .pptx、主干必须逐字匹配（大小写折叠
// 由 sameNameFold 承担，见 _CaseFoldBase），否则视为用户文件不识别——`讲义-草稿`、
// `讲义-2026-01` 这类误判会把用户自建内容卷入重拆清理。
func TestSplitArtifactPageSeq(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		name     string
		baseName string
		wantSeq  string
		wantOK   bool
	}{
		{"普通主干三位页序", "讲义-001.pptx", "讲义", "001", true},
		{"历史两位页序", "讲义-01.pptx", "讲义", "01", true},
		{"主干含连字符", "a-b-001.pptx", "a-b", "001", true},
		{"主干取更短前缀时页序段非纯数字", "a-b-001.pptx", "a", "", false},
		{"纯数字主干", "123-001.pptx", "123", "001", true},
		{"主干含连字符与数字", "方案-2026-007.pptx", "方案-2026", "007", true},
		{"页序段混字母（用户草稿）", "讲义-草稿.pptx", "讲义", "", false},
		{"多段连字符的年份用户文件", "讲义-2026-01.pptx", "讲义", "", false},
		{"扩展名不符", "讲义-001.txt", "讲义", "", false},
		{"无连字符分隔", "讲义001.pptx", "讲义", "", false},
		// 空主干是退化形态（源文件名恰为 .pptx）：其产物恒为 `-NNN.pptx`，
		// 与旧实现 HasPrefix("-") 口径一致。
		{"空主干退化形态", "-001.pptx", "", "001", true},
	} {
		gotSeq, gotOK := splitArtifactPageSeq(tc.name, tc.baseName)
		if gotOK != tc.wantOK || gotSeq != tc.wantSeq {
			t.Errorf("%s：splitArtifactPageSeq(%q, %q) = (%q, %v), want (%q, %v)",
				tc.desc, tc.name, tc.baseName, gotSeq, gotOK, tc.wantSeq, tc.wantOK)
		}
	}
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

// TestResolveExistingDirCase 落点目录大小写归一：仅 Windows 下磁盘实际条目名
// 唯一折叠命中时返回实际形态；Linux/macOS 大小写敏感，大小写变体是不同目录，
// 必须原样返回不读父目录。目录不存在恒原样返回。
func TestResolveExistingDirCase(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "abc")
	if err := os.MkdirAll(actual, 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	variant := filepath.Join(root, "Abc")
	if runtime.GOOS == "windows" {
		if got := resolveExistingDirCase(variant); got != actual {
			t.Errorf("Windows 应归一为磁盘实际名 %q，实际 %q", actual, got)
		}
	} else if got := resolveExistingDirCase(variant); got != variant {
		t.Errorf("非 Windows 应原样返回 %q，实际 %q", variant, got)
	}
	if got := resolveExistingDirCase(actual); got != actual {
		t.Errorf("大小写已一致时应原样返回，实际 %q", got)
	}
	missing := filepath.Join(root, "不存在")
	if got := resolveExistingDirCase(missing); got != missing {
		t.Errorf("目录不存在时应原样返回，实际 %q", got)
	}
}

// TestResolveExistingDirCase_NonWindowsReturnsAsIs 非 Windows（大小写敏感）：
// resolveExistingDirCase 直接返回原值，不把大小写变体目录折叠成磁盘实际名。
func TestResolveExistingDirCase_NonWindowsReturnsAsIs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("仅非 Windows 大小写敏感文件系统覆盖")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "abc"), 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	want := filepath.Join(root, "Abc")
	if got := resolveExistingDirCase(want); got != want {
		t.Errorf("非 Windows 应原样返回 %q，实际 %q", want, got)
	}
}

// TestSameNameFold 文件名/目录名等价判定三态：精确相等恒真；不同名称恒假；仅
// 大小写差异仅在 Windows 下折叠为真（Linux/macOS 是两个不同文件）。
func TestSameNameFold(t *testing.T) {
	if !sameNameFold("abc.pptx", "abc.pptx") {
		t.Error("精确相等应为真")
	}
	if sameNameFold("abc.pptx", "other.pptx") {
		t.Error("不同名称应为假")
	}
	wantFold := runtime.GOOS == "windows"
	if got := sameNameFold("001-Abc.pptx", "001-abc.pptx"); got != wantFold {
		t.Errorf("大小写变体 sameNameFold = %v, want %v（GOOS=%s）", got, wantFold, runtime.GOOS)
	}
}

// TestFoldNameInSet 集合成员判定（表驱动）：精确命中恒真（返回原键）；不存在
// 名称恒假；仅大小写差异仅在 Windows 下折叠命中，且返回集合中的实际键（供调用
// 方按磁盘真实名定位）；Linux/macOS 大小写敏感不得命中。
func TestFoldNameInSet(t *testing.T) {
	set := map[string]struct{}{"001-abc.pptx": {}, "其它.txt": {}}
	foldOK := runtime.GOOS == "windows"
	foldKey := ""
	if foldOK {
		foldKey = "001-abc.pptx"
	}
	cases := []struct {
		name    string
		query   string
		wantOK  bool
		wantKey string
	}{
		{"精确命中返回原键", "001-abc.pptx", true, "001-abc.pptx"},
		{"不存在名称不命中", "missing.pptx", false, ""},
		{"大小写变体按平台折叠", "001-ABC.PPTX", foldOK, foldKey},
	}
	for _, tc := range cases {
		got, ok := foldNameInSet(set, tc.query)
		if ok != tc.wantOK || got != tc.wantKey {
			t.Errorf("%s：foldNameInSet(%q) = (%q, %v), want (%q, %v)（GOOS=%s）",
				tc.name, tc.query, got, ok, tc.wantKey, tc.wantOK, runtime.GOOS)
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

// TestIsReplaceArtifactName_PlatformFold 产物命名形态判定：精确形态全平台命中；
// 仅大小写差异的变体（001-ABC.pptx vs 主干 abc、扩展名 .PPTX 等）仅 Windows
// 折叠命中，Linux/macOS 是不同文件必须为假（否则误删用户文件）。
func TestIsReplaceArtifactName_PlatformFold(t *testing.T) {
	exact := []struct{ name, base string }{
		{"001-abc.pptx", "abc"},
		{"abc-001.pptx", "abc"},
		{"001-123-p.pptx", "123"},
		{"123-001.pptx", "123"},
		// 多字节主干：切片按字节长度取，须与 rune 边界对齐不截半
		{"001-讲义.pptx", "讲义"},
		{"讲义-001.pptx", "讲义"},
	}
	for _, tc := range exact {
		if !isReplaceArtifactName(tc.name, tc.base) {
			t.Errorf("精确形态 isReplaceArtifactName(%q, %q) 应命中", tc.name, tc.base)
		}
	}
	wantFold := runtime.GOOS == "windows"
	fold := []struct{ name, base string }{
		{"001-ABC.pptx", "abc"},
		{"001-Abc.pptx", "abc"},
		{"001-abc.pptx", "Abc"},
		{"Abc-001.pptx", "abc"},
		{"abc-001.pptx", "Abc"},
		{"001-abc.PPTX", "abc"},
		{"讲义-001.PPTX", "讲义"},
	}
	for _, tc := range fold {
		if got := isReplaceArtifactName(tc.name, tc.base); got != wantFold {
			t.Errorf("折叠形态 isReplaceArtifactName(%q, %q) = %v, want %v（GOOS=%s）",
				tc.name, tc.base, got, wantFold, runtime.GOOS)
		}
	}
	negatives := []struct{ name, base string }{
		{"001-abc-extra.pptx", "abc"},
		{"abc-草稿.pptx", "abc"},
		{"说明.txt", "abc"},
		{"001-abc.pptx", "other"},
		{"001-讲义-extra.pptx", "讲义"},
		{"讲义-草稿.pptx", "讲义"},
		{"讲义-001.pptx", "讲义A"},
		{"讲义A-001.pptx", "讲义"},
	}
	for _, tc := range negatives {
		if isReplaceArtifactName(tc.name, tc.base) {
			t.Errorf("非产物形态 isReplaceArtifactName(%q, %q) 不应命中", tc.name, tc.base)
		}
	}
}

// TestSnapshotReplaceArtifactNames_CaseFold 目录快照：精确主干恒计入；仅大小写
// 差异的旧产物仅 Windows 计入（折叠），Linux/macOS 是用户文件不计入；用户文件
// 始终不计入。
func TestSnapshotReplaceArtifactNames_CaseFold(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"001-abc.pptx", "Abc-002.pptx", "002-Abc.pptx", "说明.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n), 0644); err != nil {
			t.Fatalf("放置文件（%s）: %v", n, err)
		}
	}
	got := snapshotReplaceArtifactNames(dir, "Abc")
	// 精确命中（主干 Abc）恒计入
	for _, n := range []string{"Abc-002.pptx", "002-Abc.pptx"} {
		if _, ok := got[n]; !ok {
			t.Errorf("快照应精确命中 %q，实际 %v", n, got)
		}
	}
	// 仅大小写差异（主干 abc vs Abc）：Windows 折叠计入，其余平台不计入
	_, foldHit := got["001-abc.pptx"]
	if want := runtime.GOOS == "windows"; foldHit != want {
		t.Errorf("大小写变体快照命中 = %v, want %v（GOOS=%s）", foldHit, want, runtime.GOOS)
	}
	if _, ok := got["说明.txt"]; ok {
		t.Errorf("用户文件不应计入历史产物：%v", got)
	}
	if _, hit := foldNameInSet(got, "001-ABC.PPTX"); hit != (runtime.GOOS == "windows") {
		t.Errorf("foldNameInSet 大小写变体命中 = %v（GOOS=%s）", hit, runtime.GOOS)
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
		splitAt, model.AssetSplitTargetReplace, "abc", false, nil, nil); err != nil {
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

// TestRemoveSplitArtifactFile 旧产物删除单点：文件存在时删除成功，已不存在
// （并发清理/用户已删）按成功处理——三处旧页清理链路（removeSplitArtifactsBeyond /
// cleanupSplitArtifactsInDirByPage / cleanupStaleReplaceArtifacts）共用该重试语义。
func TestRemoveSplitArtifactFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "演变-001.pptx")
	testutil.WriteFile(t, p, "x")
	if err := removeSplitArtifactFile(p); err != nil {
		t.Fatalf("删除既有产物: %v", err)
	}
	mustNotExist(t, p)
	if err := removeSplitArtifactFile(p); err != nil {
		t.Errorf("已不存在的产物应按成功处理，实际：%v", err)
	}
}

// 注：删除失败重试分支难以在不依赖平台权限语义的前提下稳定构造（chmod 在 Windows
// 无效），由 removeSplitArtifactFile 的实现单点覆盖：首次失败 50ms 后重试一次，
// 仍失败返回错误、调用方告警明确「残留需人工清理，本次重拆不再自动收敛」。
// 该告警文案的语义由 docs/spec/pptx-split.md §9.5 登记。
