package service

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"workbench/model"
)

// ---------- 检索 FilePath 单文件子页窗口过滤（聚合展开懒加载，2026-10-22） ----------
//
// 消费形态：前端聚合展开「按文件取逐页命中」——此前取全局命中前 200 再客户端过滤，
// 多文件命中时目标文件子页被挤出窗口且无提示；后端汇聚层按 opts.FilePath 过滤后
// limit/offset/total 即为单文件子页窗口口径。三条检索路径共用同一过滤单点
// （filterEntriesByFilePath），本文件对三路径行为一致性与空值零行为做锚定。

// filepathFilterFixture 构造两文件多页命中的标准场景：文件 A（3 页全含关键词）、
// 文件 B（2 页全含关键词），文件名不含关键词（无文件条目干扰）。返回库与两文件
// 的相对路径（slash 口径，与 AssetSearchHit.FilePath 同源）。
func filepathFilterFixture(t *testing.T) (svc *AssetLibraryService, lib model.AssetLibrary, pathA, pathB string) {
	t.Helper()
	svc = newTestAssetService(t)
	dir := t.TempDir()
	lib = mustCreateLibrary(t, svc, "文件过滤库", dir)
	if err := os.MkdirAll(filepath.Join(dir, "方案A"), 0755); err != nil {
		t.Fatalf("MkdirAll 方案A: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "方案B"), 0755); err != nil {
		t.Fatalf("MkdirAll 方案B: %v", err)
	}
	aSpecs := make([]assetsSlideSpec, 0, 3)
	for i := 0; i < 3; i++ {
		aSpecs = append(aSpecs, assetsSlideSpec{titlePh: "A 章节页", body: []string{"城商行获客举措"}})
	}
	bSpecs := make([]assetsSlideSpec, 0, 2)
	for i := 0; i < 2; i++ {
		bSpecs = append(bSpecs, assetsSlideSpec{titlePh: "B 章节页", body: []string{"城商行获客案例"}})
	}
	buildAssetsPptx(t, dir, "方案A/多页命中.pptx", aSpecs)
	buildAssetsPptx(t, dir, "方案B/另一份.pptx", bSpecs)
	mustScanIndex(t, svc, lib)
	return svc, lib, "方案A/多页命中.pptx", "方案B/另一份.pptx"
}

// TestAssetSearch_FilePathFilter_KeywordPath 关键词路径：FilePath 收敛为单文件
// 逐页命中（文件条目被滤除）；空值零行为变化（与不传字段逐字段一致）；路径未命中
// 返回空页不报错。
func TestAssetSearch_FilePathFilter_KeywordPath(t *testing.T) {
	svc, lib, pathA, pathB := filepathFilterFixture(t)

	// 基线：无 FilePath 时全局命中 5 页（3+2），平铺全返回
	base, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("基线检索: %v", err)
	}
	if base.Total != 5 || len(base.Items) != 5 {
		t.Fatalf("无 FilePath 应命中 5 页，实际 total=%d items=%d", base.Total, len(base.Items))
	}

	// FilePath=A：仅 A 的 3 页，全部为幻灯片条目且路径精确匹配
	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, FilePath: pathA})
	if err != nil {
		t.Fatalf("FilePath 检索: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("FilePath=A 应命中 3 页，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	for _, hit := range page.Items {
		if hit.RefType != model.AssetRefTypeSlide {
			t.Errorf("文件条目应被滤除，实际含类型 %q 条目：%+v", hit.RefType, hit)
		}
		if hit.FilePath != pathA {
			t.Errorf("命中页应归属 %q，实际 %q", pathA, hit.FilePath)
		}
	}

	// 空值零行为：显式空串与不传字段结果逐字段一致（回归面，既有调用零变化）
	empty, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, FilePath: ""})
	if err != nil {
		t.Fatalf("空 FilePath 检索: %v", err)
	}
	if empty.Total != base.Total || !reflect.DeepEqual(empty.Items, base.Items) {
		t.Errorf("空 FilePath 应与不传字段逐字段一致（total %d vs %d）", empty.Total, base.Total)
	}

	// 路径未命中（已删除/不存在）：空结果页不报错
	missing, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, FilePath: "方案A/已删除.pptx"})
	if err != nil {
		t.Fatalf("路径未命中检索不应报错: %v", err)
	}
	if missing.Total != 0 || len(missing.Items) != 0 {
		t.Errorf("路径未命中应返回空页，实际 total=%d", missing.Total)
	}
	_ = pathB
}

// TestAssetSearch_FilePathFilter_AggregateBefore 聚合模式：过滤发生在聚合之前，
// 单文件子页折叠为一个聚合条目且 HitCount = 该文件全部命中页数（未被全局窗口
// 挤出——本修复的核心语义）。
func TestAssetSearch_FilePathFilter_AggregateBefore(t *testing.T) {
	svc, lib, pathA, _ := filepathFilterFixture(t)

	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: false, FilePath: pathA})
	if err != nil {
		t.Fatalf("聚合模式 FilePath 检索: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("FilePath=A 聚合模式应为 1 个聚合条目，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	agg := page.Items[0]
	if agg.RefType != model.AssetRefTypeSlide || agg.HitCount != 3 {
		t.Errorf("聚合条目应折叠 A 全部 3 页命中，实际：%+v", agg)
	}
}

// TestAssetSearch_FilePathFilter_Pagination 过滤后分页：limit/offset 作用于过滤后
// 集合，Total 恒为过滤后总数（翻页以 offset 递增驱动，页内条目均为目标文件子页）。
func TestAssetSearch_FilePathFilter_Pagination(t *testing.T) {
	svc, lib, pathA, _ := filepathFilterFixture(t)

	page1, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, FilePath: pathA, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("第一页: %v", err)
	}
	if page1.Total != 3 || len(page1.Items) != 2 {
		t.Fatalf("第一页应 2 条（Total=3），实际 total=%d items=%d", page1.Total, len(page1.Items))
	}
	page2, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, FilePath: pathA, Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("第二页: %v", err)
	}
	if page2.Total != 3 || len(page2.Items) != 1 {
		t.Fatalf("第二页应 1 条（Total=3），实际 total=%d items=%d", page2.Total, len(page2.Items))
	}
	// 两页无重叠（续拉追加语义的前提）
	seen := make(map[int64]bool)
	for _, hit := range page1.Items {
		seen[hit.RefID] = true
	}
	for _, hit := range page2.Items {
		if seen[hit.RefID] {
			t.Errorf("分页条目不应跨页重叠：refID=%d 同时出现在两页", hit.RefID)
		}
	}
}

// TestAssetSearch_FilePathFilter_TagsOnlyPath 纯标签浏览路径一致性：FilePath 与
// 标签筛选组合后仅剩目标文件页条目（文件条目被滤除）；无 FilePath 时该路径返回
// 文件条目 + 页条目的完整形态（对照基线）。
func TestAssetSearch_FilePathFilter_TagsOnlyPath(t *testing.T) {
	svc, lib, pathA, pathB := filepathFilterFixture(t)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{Tags: []string{"重点"}}); err != nil {
			t.Fatalf("UpdateFileMeta %s: %v", f.Path, err)
		}
	}
	store := svc.openStoreForTest(t, lib.ID)
	var tagID int64
	if err := store.db.QueryRow("SELECT id FROM tags WHERE name = ?", "重点").Scan(&tagID); err != nil {
		t.Fatalf("反查标签: %v", err)
	}

	// 对照基线：2 文件条目 + 5 页条目
	base, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
	if err != nil {
		t.Fatalf("无 FilePath 标签浏览: %v", err)
	}
	if base.Total != 7 {
		t.Fatalf("无 FilePath 应命中 2 文件 + 5 页 = 7 条，实际 total=%d", base.Total)
	}

	// FilePath=A：仅 3 页条目（B 的条目与两文件条目均被滤除）
	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}, FilePath: pathA})
	if err != nil {
		t.Fatalf("FilePath 标签浏览: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("FilePath=A 标签浏览应命中 3 页，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	for _, hit := range page.Items {
		if hit.RefType != model.AssetRefTypeSlide || hit.FilePath != pathA {
			t.Errorf("应仅剩 A 的页条目，实际：%+v", hit)
		}
	}
	_ = pathB
}

// TestAssetSearch_FilePathFilter_FacetsOnlyPath facets-only 路径口径收口：该路径
// 仅产出文件条目，FilePath 过滤谓词（仅保留幻灯片条目）下返回空结果页——聚合展开
// 从不携带 FilePath+facets-only 组合（无聚合条目即无展开面），此断言锚定的是三
// 路径同一谓词的一致性而非生产消费形态。
func TestAssetSearch_FilePathFilter_FacetsOnlyPath(t *testing.T) {
	svc, lib, pathA, _ := filepathFilterFixture(t)

	// 对照基线：星级 facet（两文件均未评分）→ 2 个文件条目
	base, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{0}})
	if err != nil {
		t.Fatalf("无 FilePath facets-only: %v", err)
	}
	if base.Total != 2 {
		t.Fatalf("无 FilePath 应命中 2 个文件条目，实际 total=%d", base.Total)
	}

	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{0}, FilePath: pathA})
	if err != nil {
		t.Fatalf("FilePath facets-only: %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("facets-only 路径 FilePath 过滤语义下应返回空页（仅文件条目被滤除），实际 total=%d", page.Total)
	}
}
