package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// ---------- 排除规则匹配语义（SRS §2.7.3，定稿见 research/exclude-rules-semantics.md） ----------

// TestAssetExcludeRules_NormalizeExt 后缀三形态归一：tmp / .tmp / *.tmp 等价
// （带点小写 .tmp）；空白与归一后为空的输入丢弃。
func TestAssetExcludeRules_NormalizeExt(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"tmp", ".tmp"},
		{".tmp", ".tmp"},
		{"*.tmp", ".tmp"},
		{"*.TMP", ".tmp"},
		{"  .Tmp  ", ".tmp"},
		{"", ""},
		{"   ", ""},
		{".", ""},  // 剥前导点后为空：丢弃
		{"*.", ""}, // 剥 *. 前缀后为空：丢弃
	}
	for _, tc := range cases {
		if got := normalizeAssetExcludeExt(tc.in); got != tc.want {
			t.Errorf("normalizeAssetExcludeExt(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// 三形态归一为同一存储值
	seen := map[string]bool{}
	for _, in := range []string{"tmp", ".tmp", "*.tmp"} {
		seen[normalizeAssetExcludeExt(in)] = true
	}
	if len(seen) != 1 {
		t.Errorf("三形态应归一为同一存储值，实际：%v", seen)
	}
}

// TestAssetExcludeRules_NormalizeRejectsInvalid 保存校验（第一道防线）：坏 glob
// 语法与含路径分隔符的目录模式拒绝（单段语义下分隔符模式永不命中，拒绝防误配
// 静默失效）；空白项丢弃；后缀归一落位。
func TestAssetExcludeRules_NormalizeRejectsInvalid(t *testing.T) {
	if _, _, err := normalizeAssetExcludeRules([]string{"[a"}, nil); err == nil {
		t.Error("坏 glob 模式应被拒绝")
	}
	if _, _, err := normalizeAssetExcludeRules([]string{"a/b"}, nil); err == nil {
		t.Error("含「/」的目录模式应被拒绝（按文件夹名匹配，不含路径分隔符）")
	}
	if _, _, err := normalizeAssetExcludeRules([]string{`a\b`}, nil); err == nil {
		t.Error(`含「\」的目录模式应被拒绝`)
	}
	dirs, exts, err := normalizeAssetExcludeRules(
		[]string{"  ", "*草稿*", "DRAFT"},
		[]string{"tmp", ".MP4", "*.pptx"},
	)
	if err != nil {
		t.Fatalf("合法规则不应报错: %v", err)
	}
	if len(dirs) != 2 || dirs[0] != "*草稿*" || dirs[1] != "DRAFT" {
		t.Errorf("目录规则应 TrimSpace 丢弃空白项并保留原值（大小写归一在匹配侧），实际：%v", dirs)
	}
	if len(exts) != 3 || exts[0] != ".tmp" || exts[1] != ".mp4" || exts[2] != ".pptx" {
		t.Errorf("后缀规则应归一带点小写，实际：%v", exts)
	}
}

// TestAssetExcludeRules_NormalizeDedup 归一后保序去重（冒烟 S2.1b 缺陷修复）：
// 后缀三形态等价输入只落一条；目录模式 TrimSpace 后同串只落一条；重复项剔除后
// 首次出现位置保留（保序）。去重发生在校验之后（坏模式拒绝语义不受影响）；
// 目录模式大小写不归一（匹配侧才 ToLower），DRAFT 与 draft 是两条不同规则。
func TestAssetExcludeRules_NormalizeDedup(t *testing.T) {
	// 三形态混合输入回显单条 .tmp
	_, exts, err := normalizeAssetExcludeRules(nil, []string{"tmp", ".tmp", "*.TMP"})
	if err != nil {
		t.Fatalf("合法规则不应报错: %v", err)
	}
	if len(exts) != 1 || exts[0] != ".tmp" {
		t.Errorf("三形态等价输入应去重为单条 .tmp，实际：%v", exts)
	}

	// 重复目录模式回显单条；TrimSpace 后同串视为重复
	dirs, _, err := normalizeAssetExcludeRules([]string{"archive", "archive", "  archive  "}, nil)
	if err != nil {
		t.Fatalf("合法规则不应报错: %v", err)
	}
	if len(dirs) != 1 || dirs[0] != "archive" {
		t.Errorf("重复目录模式应去重为单条，实际：%v", dirs)
	}

	// 保序：首次出现位置保留，重复项剔除不改变其余项相对顺序
	dirs, exts, err = normalizeAssetExcludeRules(
		[]string{"草稿*", "DRAFT", "草稿*", "正式", "DRAFT"},
		[]string{".mp4", "tmp", ".mp4", "*.TMP", "doc"},
	)
	if err != nil {
		t.Fatalf("合法规则不应报错: %v", err)
	}
	wantDirs := []string{"草稿*", "DRAFT", "正式"}
	if len(dirs) != len(wantDirs) {
		t.Fatalf("目录去重后应剩 %d 条，实际 %d：%v", len(wantDirs), len(dirs), dirs)
	}
	for i, want := range wantDirs {
		if dirs[i] != want {
			t.Errorf("目录去重应保序 dirs[%d] = %q, want %q（全部：%v）", i, dirs[i], want, dirs)
		}
	}
	wantExts := []string{".mp4", ".tmp", ".doc"}
	if len(exts) != len(wantExts) {
		t.Fatalf("后缀去重后应剩 %d 条，实际 %d：%v", len(wantExts), len(exts), exts)
	}
	for i, want := range wantExts {
		if exts[i] != want {
			t.Errorf("后缀去重应保序 exts[%d] = %q, want %q（全部：%v）", i, exts[i], want, exts)
		}
	}

	// 大小写不归一边界：目录模式 DRAFT 与 draft 不去重（归一仅 TrimSpace，
	// 大小写归一在匹配侧；误去重会丢规则改变语义）
	dirs, _, err = normalizeAssetExcludeRules([]string{"DRAFT", "draft"}, nil)
	if err != nil {
		t.Fatalf("合法规则不应报错: %v", err)
	}
	if len(dirs) != 2 {
		t.Errorf("目录模式大小写不同不去重，实际：%v", dirs)
	}
}

// TestAssetExcludeMatcher_DirGlob 目录段匹配：大小写不敏感（双侧归一小写）、
// 单段通配 * ? [...]、中文按 rune 匹配；坏模式编译期剔除后其余规则仍生效
// （手改 libraries.json 绕过保存校验的防御，剔错不炸扫描）。
func TestAssetExcludeMatcher_DirGlob(t *testing.T) {
	m := newAssetExcludeMatcher([]string{"*草稿*", "DRAFT", "v?", "[a-c]档"}, nil)
	if !m.active() {
		t.Fatal("非空规则应 active")
	}
	cases := []struct {
		dir  string
		want bool
	}{
		{"营销草稿2024", true}, // *草稿* 前后通配
		{"草稿", true},
		{"draft", true}, // 大小写不敏感（模式 DRAFT）
		{"DRAFT", true}, // 段名大写同样命中
		{"v1", true},    // ? 单字符
		{"v10", false},  // ? 恰一字符
		{"b档", true},    // 字符类范围
		{"d档", false},   // 范围外
		{"素材包", false},  // 未命中
	}
	for _, tc := range cases {
		if got := m.matchDir(tc.dir); got != tc.want {
			t.Errorf("matchDir(%q) = %v, want %v", tc.dir, got, tc.want)
		}
	}
	// 坏模式剔除（不 panic）且不影响其余规则
	bad := newAssetExcludeMatcher([]string{"[a", "draft"}, nil)
	if bad.matchDir("anything[") {
		t.Error("坏模式应被剔除不命中")
	}
	if !bad.matchDir("draft") {
		t.Error("坏模式剔除后其余规则应仍生效")
	}
	// 空规则恒不命中
	empty := newAssetExcludeMatcher(nil, nil)
	if empty.active() || empty.matchDir("草稿") || empty.matchPath("草稿/a.pptx") {
		t.Error("空规则应恒不命中")
	}
	var nilMatcher *assetExcludeMatcher
	if nilMatcher.active() || nilMatcher.matchDir("草稿") {
		t.Error("nil matcher 应安全不命中")
	}
}

// TestAssetExcludeMatcher_MatchPath 存量索引路径判定（「规则 × 存量索引 path」
// 独立链路）：任意单层目录段命中即命中；文件基名不参与目录规则；后缀命中。
func TestAssetExcludeMatcher_MatchPath(t *testing.T) {
	m := newAssetExcludeMatcher([]string{"*草稿*", "archive"}, []string{".tmp"})
	cases := []struct {
		rel  string
		want bool
	}{
		{"营销草稿2024/演示.pptx", true},    // 首层目录段命中
		{"年份/营销草稿2024/演示.pptx", true}, // 深层目录段命中
		{"archive/old.pptx", true},    // 精确段名
		{"草稿.pptx", false},            // 文件基名不参与目录规则
		{"正常目录/草稿汇报.pptx", false},     // 基名含「草稿」不算目录命中
		{"说明.tmp", true},              // 后缀命中
		{"子目录/数据.tmp", true},          // 深层后缀命中
		{"正常目录/演示.pptx", false},       // 全不命中
	}
	for _, tc := range cases {
		if got := m.matchPath(tc.rel); got != tc.want {
			t.Errorf("matchPath(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

// ---------- 收集层过滤（collectPptxEntries：磁盘扫描结果不含命中文件） ----------

// TestAssetScan_RulesExcludeCollection 收集层：目录段命中 SkipDir 剪除整棵子树
// （Total 不含命中文件）；后缀命中不入清单；库根目录自身豁免目录匹配（根名含
// 「草稿」不剪空整库）。
func TestAssetScan_RulesExcludeCollection(t *testing.T) {
	svc := newTestAssetService(t)
	base := t.TempDir()
	// 库根目录名本身含「草稿」：root 豁免匹配，根下文件仍入库
	root := filepath.Join(base, "素材草稿库根")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("建库根: %v", err)
	}
	buildAssetsPptx(t, root, "根下.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	// 命中规则的子目录（嵌套两层）与其内文件
	nested := filepath.Join(root, "年份", "营销草稿2024")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("建排除目录: %v", err)
	}
	buildAssetsPptx(t, nested, "草稿件.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	// 未命中子目录正常入库
	if err := os.MkdirAll(filepath.Join(root, "正式"), 0755); err != nil {
		t.Fatalf("建正常目录: %v", err)
	}
	buildAssetsPptx(t, filepath.Join(root, "正式"), "正式件.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})

	lib := mustCreateLibrary(t, svc, root, root)
	if _, err := svc.UpdateLibraryRules(lib.ID, []string{"*草稿*"}, nil); err != nil {
		t.Fatalf("UpdateLibraryRules: %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	paths := make(map[string]bool, len(files))
	for _, f := range files {
		paths[f.Path] = true
	}
	if !paths["根下.pptx"] || !paths["正式/正式件.pptx"] {
		t.Errorf("未命中文件应入库（root 豁免），实际：%v", paths)
	}
	if paths["年份/营销草稿2024/草稿件.pptx"] {
		t.Error("目录规则命中子树应整体剪除不入库")
	}

	// 后缀规则：忽略 pptx 后磁盘收集为空（Total=0）
	if _, err := svc.UpdateLibraryRules(lib.ID, nil, []string{"pptx"}); err != nil {
		t.Fatalf("UpdateLibraryRules(pptx): %v", err)
	}
	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("忽略 pptx 后缀应收集为空（total=0），实际：%+v", result)
	}
}

// ---------- excluded 状态机（存量命中 / 复活 / 与 lost 互斥） ----------

// newExcludedFixture 两文件库：草稿目录下「草稿件.pptx」（将被规则命中）与根下
// 「保留件.pptx」，均含检索词「城商行」页文本；草稿件带文件标签与星级（标注
// 保留断言基准）。首次扫描（无规则）均 ok。
func newExcludedFixture(t *testing.T) (*AssetLibraryService, model.AssetLibrary) {
	t.Helper()
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "排除库", dir)
	specs := []assetsSlideSpec{{titlePh: "获客策略", body: []string{"城商行专属内容"}}}
	if err := os.MkdirAll(filepath.Join(dir, "营销草稿2024"), 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	buildAssetsPptx(t, filepath.Join(dir, "营销草稿2024"), "草稿件.pptx", specs)
	buildAssetsPptx(t, dir, "保留件.pptx", specs)
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	// 草稿件打标签 + 星级（excluded 转换后保留断言基准）
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if f.Path == "营销草稿2024/草稿件.pptx" {
			if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{
				Rating: 4, Note: "待归档", Tags: []string{"城商行"},
			}); err != nil {
				t.Fatalf("UpdateFileMeta: %v", err)
			}
		}
	}
	return svc, lib
}

// assetStatusMapByPath 库内全部文件状态映射（断言辅助；与 assets_relink_test.go
// 的单文件白盒读 fileStatusByPath 分立——本形态一次取全量映射，多文件状态断言复用）。
func assetStatusMapByPath(t *testing.T, svc *AssetLibraryService, libraryID string) map[string]string {
	t.Helper()
	files, err := svc.ListFiles(libraryID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	out := make(map[string]string, len(files))
	for _, f := range files {
		out[f.Path] = f.Status
	}
	return out
}

// TestAssetScan_ExcludedLifecycle excluded 全生命周期：存量命中标 excluded
// （标注/缩略图记账/FTS 行保留）→ 三入口不可见 → 规则改宽复活转 ok 且检索恢复。
// 规则切换经 UpdateLibraryRules（保存即自动重扫链路一并覆盖）。
func TestAssetScan_ExcludedLifecycle(t *testing.T) {
	svc, lib := newExcludedFixture(t)
	excludedRel := "营销草稿2024/草稿件.pptx"
	store := svc.openStoreForTest(t, lib.ID)
	ftsBefore, err := store.countFtsRows()
	if err != nil {
		t.Fatalf("countFtsRows: %v", err)
	}

	// 1. 设规则 → 自动重扫 → 存量命中转 excluded
	result, err := svc.UpdateLibraryRules(lib.ID, []string{"*草稿*"}, nil)
	if err != nil {
		t.Fatalf("UpdateLibraryRules: %v", err)
	}
	if result.Excluded != 1 {
		t.Errorf("应新标记 1 条 excluded，实际：%+v", result)
	}
	status := assetStatusMapByPath(t, svc, lib.ID)
	if status[excludedRel] != model.AssetFileStatusExcluded {
		t.Errorf("规则命中存量应转 excluded，实际：%v", status)
	}
	if status["保留件.pptx"] != model.AssetFileStatusOK {
		t.Errorf("未命中文件应保持 ok，实际：%v", status)
	}

	// 2. 无损保留：标注/评分/备注/slides/FTS 行全保留
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if f.Path != excludedRel {
			continue
		}
		if f.Rating != 4 || f.Note != "待归档" || len(f.Tags) != 1 || f.Tags[0].Name != "城商行" {
			t.Errorf("excluded 文件标注应保留，实际：%+v", f)
		}
	}
	slidePage, err := svc.ListSlides(lib.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	if len(slidePage.Items) != 2 || slidePage.Total != 2 {
		t.Errorf("excluded 文件幻灯片应保留（2 页），实际：%d 页 total=%d", len(slidePage.Items), slidePage.Total)
	}
	ftsAfter, err := store.countFtsRows()
	if err != nil {
		t.Fatalf("countFtsRows: %v", err)
	}
	if ftsAfter != ftsBefore {
		t.Errorf("excluded 转换不应动 FTS 行（%d → %d）", ftsBefore, ftsAfter)
	}

	// 3. 库统计含 excluded（FileCount 含各状态 + 专项计数）
	stats, err := svc.GetLibraryStats(lib.ID)
	if err != nil {
		t.Fatalf("GetLibraryStats: %v", err)
	}
	if stats.FileCount != 2 || stats.ExcludedCount != 1 {
		t.Errorf("统计应含 excluded（file=2 excluded=1），实际：%+v", stats)
	}

	// 4. 三入口不可见：关键词检索 / 标签浏览 / facet-only
	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("关键词检索: %v", err)
	}
	for _, hit := range page.Items {
		if hit.FilePath == excludedRel {
			t.Error("关键词检索不应返回 excluded 文件命中")
		}
	}
	if page.Total != 1 {
		t.Errorf("excluded 过滤后关键词命中应收敛为 1（保留件），实际 total=%d", page.Total)
	}
	// 标签浏览：两文件同标签，excluded 一个后仅返回另一个
	tags, err := svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("应有 1 个标签，实际：%+v", tags)
	}
	tagPage, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tags[0].ID}})
	if err != nil {
		t.Fatalf("标签浏览: %v", err)
	}
	if tagPage.Total != 0 || len(tagPage.Items) != 0 {
		t.Errorf("标签浏览不应返回 excluded 文件（其标签命中但 status 过滤），实际 total=%d", tagPage.Total)
	}
	// facet-only：excluded 文件星级 4，星级筛选不返回
	facetPage, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{4}})
	if err != nil {
		t.Fatalf("facet 检索: %v", err)
	}
	if facetPage.Total != 0 || len(facetPage.Items) != 0 {
		t.Errorf("facet-only 不应返回 excluded 文件，实际 total=%d", facetPage.Total)
	}

	// 5. 重复扫描幂等：已 excluded 行不重复计数
	result, err = svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("重复扫描: %v", err)
	}
	if result.Excluded != 0 {
		t.Errorf("已 excluded 行不应重复计数，实际：%+v", result)
	}

	// 6. 规则改宽 → 复活转 ok（unchanged 直复活，无需重解析）
	result, err = svc.UpdateLibraryRules(lib.ID, nil, nil)
	if err != nil {
		t.Fatalf("改宽规则: %v", err)
	}
	if result.Revived != 1 {
		t.Errorf("excluded 复活应计入 revived=1，实际：%+v", result)
	}
	status = assetStatusMapByPath(t, svc, lib.ID)
	if status[excludedRel] != model.AssetFileStatusOK {
		t.Errorf("规则改宽后应复活 ok，实际：%v", status)
	}
	// 检索恢复可见
	page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("复活后检索: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("复活后关键词命中应恢复为 2，实际 total=%d", page.Total)
	}
}

// TestAssetScan_ReviveRebuildsFtsAfterMetaEditDuringExcluded 复活同事务重建 FTS
// 文件行（素材审核 2026-10-18 补口）：excluded 期间经文件底册编辑备注（ListFiles
// 无 status 门禁，非 ok 行可编辑）→ updateFileMeta 的 syncFtsFileRowTx 对非 ok 行
// 先删后不插，FTS 文件行缺失 → 规则改宽 mtime+size 未变走 unchanged 直复活——复活
// 须重建 FTS 行，否则此后每轮扫描 unchanged 跳过永不重解析，FTS 行永久缺失，
// 关键词检索永久漏该文件。
func TestAssetScan_ReviveRebuildsFtsAfterMetaEditDuringExcluded(t *testing.T) {
	svc, lib := newExcludedFixture(t)
	excludedRel := "营销草稿2024/草稿件.pptx"

	// 1. 设规则 → 存量命中转 excluded
	if _, err := svc.UpdateLibraryRules(lib.ID, []string{"*草稿*"}, nil); err != nil {
		t.Fatalf("UpdateLibraryRules: %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	var excludedID int64
	for _, f := range files {
		if f.Path == excludedRel {
			excludedID = f.ID
		}
	}
	if excludedID == 0 {
		t.Fatalf("未找到 excluded 文件 %s，实际：%+v", excludedRel, files)
	}

	// 2. excluded 期间编辑备注：FTS 文件行被清（非 ok 行只删不插）
	if err := svc.UpdateFileMeta(lib.ID, excludedID, model.AssetFileMetaUpdate{
		Rating: 4, Note: "排除期修订独家批注",
	}); err != nil {
		t.Fatalf("excluded 期间改备注: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	var ftsRows int
	if err := store.db.QueryRow(
		"SELECT COUNT(*) FROM search_fts WHERE rowid = ?", excludedID,
	).Scan(&ftsRows); err != nil {
		t.Fatalf("查 FTS 文件行: %v", err)
	}
	if ftsRows != 0 {
		t.Fatalf("非 ok 行编辑备注后 FTS 文件行应被清除，实际 %d 行", ftsRows)
	}

	// 3. 规则改宽 → unchanged 直复活（不重解析）
	result, err := svc.UpdateLibraryRules(lib.ID, nil, nil)
	if err != nil {
		t.Fatalf("改宽规则: %v", err)
	}
	if result.Revived != 1 {
		t.Errorf("应直复活 1 条，实际：%+v", result)
	}

	// 4. 复活须重建 FTS 文件行：按 excluded 期间新编辑的备注关键词命中
	// （漏重建则 FTS 行永久缺失，该关键词恒 0 命中）
	page, err := svc.SearchAssets(lib.ID, "独家批注", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("复活后检索: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].FilePath != excludedRel {
		t.Errorf("复活后应按 excluded 期间编辑的备注关键词命中文件，实际 total=%d items=%+v",
			page.Total, page.Items)
	}
}

// TestAssetScan_ExcludedMarkingRespectsCancellation excluded 标记循环的取消检查点
// （素材审核 2026-10-18 补口）：预取消 ctx 下循环首行即返回取消错误、记录不被
// 标记（漏检查点时整个存量逐行 UPDATE 循环跑完取消才生效，且本构造下会静默
// 返回成功）。直调 scanLibrary 白盒驱动（经 StartScan 无法在标记循环前注入取消
// 且带存量命中行的形态）。
func TestAssetScan_ExcludedMarkingRespectsCancellation(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "取消标记库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)
	store := svc.openStoreForTest(t, lib.ID)

	// 规则命中全部存量（后缀规则），磁盘文件同时被收集层过滤（entries 空：
	// 排除标记循环是取消检查点唯一可生效的前置环节）
	libWithRules := lib
	libWithRules.ExcludeExts = []string{"pptx"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.scanLibrary(ctx, store, libWithRules)
	if err == nil {
		t.Fatal("预取消 ctx 的 excluded 标记循环应返回取消错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("取消错误应包装 context.Canceled 供 errors.Is 穿透，实际：%v", err)
	}
	// 取消先行生效：记录未被标记 excluded（保持扫描前状态）
	records, err := store.listFileRecords()
	if err != nil {
		t.Fatalf("listFileRecords: %v", err)
	}
	if len(records) != 1 || records[0].status != model.AssetFileStatusOK {
		t.Errorf("取消后记录不应被标记 excluded，实际：%+v", records)
	}
}

// TestAssetScan_ExcludedVsLostInteraction excluded 与 lost 互斥（SRS §2.7.3）：
// excluded 且规则仍命中的文件消失保持 excluded 不转 lost（规则优先）；规则改宽
// 后消失的文件才可见 lost。
func TestAssetScan_ExcludedVsLostInteraction(t *testing.T) {
	svc, lib := newExcludedFixture(t)
	excludedRel := "营销草稿2024/草稿件.pptx"

	if _, err := svc.UpdateLibraryRules(lib.ID, []string{"*草稿*"}, nil); err != nil {
		t.Fatalf("设规则: %v", err)
	}
	// 文件消失（规则仍命中）
	if err := os.Remove(filepath.Join(lib.RootPath, "营销草稿2024", "草稿件.pptx")); err != nil {
		t.Fatalf("删除文件: %v", err)
	}
	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("扫描: %v", err)
	}
	if result.Lost != 0 {
		t.Errorf("规则命中文件的消失不应标 lost，实际：%+v", result)
	}
	status := assetStatusMapByPath(t, svc, lib.ID)
	if status[excludedRel] != model.AssetFileStatusExcluded {
		t.Errorf("excluded + 规则仍命中 + 文件消失应保持 excluded，实际：%v", status)
	}

	// 规则改宽：消失文件可见 lost（UpdateLibraryRules 的自动重扫即完成标记）
	result, err = svc.UpdateLibraryRules(lib.ID, nil, nil)
	if err != nil {
		t.Fatalf("改宽规则: %v", err)
	}
	if result.Lost != 1 {
		t.Errorf("规则改宽后消失文件应标 lost=1，实际：%+v", result)
	}
	status = assetStatusMapByPath(t, svc, lib.ID)
	if status[excludedRel] != model.AssetFileStatusLost {
		t.Errorf("规则改宽后消失文件应转 lost，实际：%v", status)
	}
	// 重复扫描幂等：已 lost 行不重复计数
	result, err = svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("重复扫描: %v", err)
	}
	if result.Lost != 0 {
		t.Errorf("已 lost 的行不应重复计数，实际：%+v", result)
	}
}

// TestAssetScan_ExcludedExtRuleOnExisting 后缀规则对存量索引的命中：已入库
// pptx 在配置忽略后缀后转 excluded（存量链路按后缀判定，与收集层两链并存）。
func TestAssetScan_ExcludedExtRuleOnExisting(t *testing.T) {
	svc, lib := newExcludedFixture(t)
	result, err := svc.UpdateLibraryRules(lib.ID, nil, []string{"*.pptx"})
	if err != nil {
		t.Fatalf("UpdateLibraryRules(*.pptx): %v", err)
	}
	if result.Excluded != 2 {
		t.Errorf("忽略 pptx 后缀应排除全部存量 2 条，实际：%+v", result)
	}
	status := assetStatusMapByPath(t, svc, lib.ID)
	if status["保留件.pptx"] != model.AssetFileStatusExcluded || status["营销草稿2024/草稿件.pptx"] != model.AssetFileStatusExcluded {
		t.Errorf("全部存量应转 excluded，实际：%v", status)
	}
}

// ---------- 规则保存 API ----------

// TestAssetLibrary_UpdateRulesValidationAndPersist UpdateLibraryRules 校验与
// 持久化：坏 glob / 含分隔符拒绝且不落盘；归一后规则写入 libraries.json；
// 库不存在报 E_ASSETS_LIB_NOT_FOUND；保存触发自动重扫（返回扫描结果）。
func TestAssetLibrary_UpdateRulesValidationAndPersist(t *testing.T) {
	svc, lib := newExcludedFixture(t)

	// 非法规则拒绝且不落盘
	_, err := svc.UpdateLibraryRules(lib.ID, []string{"a/b"}, nil)
	if err == nil {
		t.Fatal("含分隔符规则应被拒绝")
	}
	_, err = svc.UpdateLibraryRules(lib.ID, []string{"[a"}, nil)
	if err == nil {
		t.Fatal("坏 glob 规则应被拒绝")
	}
	libs, err := svc.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(libs[0].ExcludeDirs) != 0 {
		t.Errorf("被拒绝的规则不应落盘，实际：%v", libs[0].ExcludeDirs)
	}

	// 合法规则归一落盘（逐项归一带点小写 + 保序去重，tmp/.TMP 两形态等价只落
	// 一条 .tmp）+ 自动重扫（以新规则执行：草稿目录剪除 total=1，存量命中标
	// excluded=1）
	result, err := svc.UpdateLibraryRules(lib.ID, []string{"*草稿*"}, []string{"tmp", ".TMP"})
	if err != nil {
		t.Fatalf("UpdateLibraryRules: %v", err)
	}
	if result.Total != 1 || result.Excluded != 1 {
		t.Errorf("保存应触发自动重扫（新规则生效 total=1 excluded=1），实际：%+v", result)
	}
	libs, err = svc.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(libs[0].ExcludeDirs) != 1 || libs[0].ExcludeDirs[0] != "*草稿*" {
		t.Errorf("目录规则应落盘，实际：%v", libs[0].ExcludeDirs)
	}
	if len(libs[0].ExcludeExts) != 1 || libs[0].ExcludeExts[0] != ".tmp" {
		t.Errorf("后缀规则应归一去重落盘单条 .tmp，实际：%v", libs[0].ExcludeExts)
	}
	if libs[0].UpdatedAt == "" {
		t.Error("规则保存应刷新 UpdatedAt")
	}

	// 库不存在报 E_ASSETS_LIB_NOT_FOUND
	_, err = svc.UpdateLibraryRules("不存在的库", nil, nil)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsLibNotFound {
		t.Errorf("库不存在应报 E_ASSETS_LIB_NOT_FOUND，实际：%v", err)
	}
}
