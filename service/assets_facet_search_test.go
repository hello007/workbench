package service

import (
	"errors"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- facet 多维筛选测试（SRS §2.5，文件维度 + 幻灯片条目继承归属文件属性） ----------
//
// 时间/大小 facet 判定值全部经 fixture 注入固定 mtime_ns/size（不依赖真实时钟与
// 文件系统时序，docs/spec/test-stability.md）；星级经 UpdateFileMeta 正常链路写入。

// setAssetFileFacetProps 注入文件的 facet 判定属性（mtime_ns 纳秒 + size 字节）。
// 直改库为测试注入确定值（对齐 TestAssetSearch_FtsSyncOnDeleteAndReparse 的陈旧
// mtime 注入先例），不依赖 fixture 产物真实大小与扫描时刻。
func setAssetFileFacetProps(t *testing.T, svc *AssetLibraryService, libID, relPath string, mtimeNs, size int64) {
	t.Helper()
	store := svc.openStoreForTest(t, libID)
	if _, err := store.db.Exec("UPDATE files SET mtime_ns = ?, size = ? WHERE path = ?", mtimeNs, size, relPath); err != nil {
		t.Fatalf("注入文件 facet 属性 %s: %v", relPath, err)
	}
}

// newFacetFixture 构造三文件固定属性 fixture：A.pptx（未评分/T0/100KB）、
// B.pptx（3 星/T0+10 天/5MB）、C.pptx（5 星/T0+40 天/60MB），每文件 2 页且页
// 正文均含检索词「城商行」（keyword 路径组合用例复用）。
func newFacetFixture(t *testing.T) (*AssetLibraryService, model.AssetLibrary, map[string]int64) {
	t.Helper()
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "facet库", dir)
	t0 := int64(1700000000) * 1e9 // 固定基准纳秒时间戳（与真实时钟无关）
	day := int64(24) * 3600 * 1e9
	specs := []assetsSlideSpec{
		{titlePh: "首页", body: []string{"城商行内容"}},
		{titlePh: "次页", body: []string{"城商行内容"}},
	}
	buildAssetsPptx(t, dir, "A.pptx", specs)
	buildAssetsPptx(t, dir, "B.pptx", specs)
	buildAssetsPptx(t, dir, "C.pptx", specs)
	mustScanIndex(t, svc, lib)
	setAssetFileFacetProps(t, svc, lib.ID, "A.pptx", t0, 100*1024)
	setAssetFileFacetProps(t, svc, lib.ID, "B.pptx", t0+10*day, 5*1024*1024)
	setAssetFileFacetProps(t, svc, lib.ID, "C.pptx", t0+40*day, 60*1024*1024)
	// 星级走正常编辑链路（A 保持未评分 = 0）
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		switch f.Path {
		case "B.pptx":
			if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{Rating: 3}); err != nil {
				t.Fatalf("UpdateFileMeta B: %v", err)
			}
		case "C.pptx":
			if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{Rating: 5}); err != nil {
				t.Fatalf("UpdateFileMeta C: %v", err)
			}
		}
	}
	paths := map[string]int64{
		"A.pptx": t0,
		"B.pptx": t0 + 10*day,
		"C.pptx": t0 + 40*day,
	}
	return svc, lib, paths
}

// facetHitPaths 提取检索结果的文件路径集合（判定辅助）。
func facetHitPaths(page model.AssetSearchPage) map[string]bool {
	paths := make(map[string]bool)
	for _, hit := range page.Items {
		paths[hit.FilePath] = true
	}
	return paths
}

// TestAssetSearchFacets_FacetsOnlyPath facets-only 路径（无关键词无标签但有
// facet）：三维单条件各命中预期文件集合；返回条目为文件形态（RefType=file、
// 无相关性语义 Score=0 Snippet 空、按路径序）；failed 状态文件不参与筛选。
func TestAssetSearchFacets_FacetsOnlyPath(t *testing.T) {
	svc, lib, paths := newFacetFixture(t)
	day := int64(24) * 3600 * 1e9
	mb := int64(1024 * 1024)

	t.Run("星级单条件", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{3}})
		if err != nil {
			t.Fatalf("星级 facet: %v", err)
		}
		if page.Total != 1 || len(page.Items) != 1 || !facetHitPaths(page)["B.pptx"] {
			t.Errorf("ratings=[3] 应只命中 B.pptx，实际 total=%d items=%+v", page.Total, page.Items)
		}
	})

	t.Run("时间区间单条件", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{
			Flatten: true, MtimeFrom: paths["A.pptx"] - day, MtimeTo: paths["A.pptx"] + day,
		})
		if err != nil {
			t.Fatalf("时间 facet: %v", err)
		}
		if page.Total != 1 || !facetHitPaths(page)["A.pptx"] {
			t.Errorf("A 文件当日区间应只命中 A.pptx，实际 total=%d items=%+v", page.Total, page.Items)
		}
		// 宽区间覆盖三文件
		page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{
			Flatten: true, MtimeFrom: paths["A.pptx"] - day, MtimeTo: paths["C.pptx"] + day,
		})
		if err != nil {
			t.Fatalf("宽区间 facet: %v", err)
		}
		if page.Total != 3 {
			t.Errorf("全域区间应命中 3 文件，实际 total=%d", page.Total)
		}
	})

	t.Run("大小区间单条件", func(t *testing.T) {
		// <1MB 档（sizeFrom=0 = 不设下界）
		page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, SizeTo: mb})
		if err != nil {
			t.Fatalf("大小 facet 小档: %v", err)
		}
		if page.Total != 1 || !facetHitPaths(page)["A.pptx"] {
			t.Errorf("<1MB 档应只命中 A.pptx，实际 total=%d items=%+v", page.Total, page.Items)
		}
		// >50MB 档（sizeTo=0 = 不设上界）
		page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, SizeFrom: 50 * mb})
		if err != nil {
			t.Fatalf("大小 facet 大档: %v", err)
		}
		if page.Total != 1 || !facetHitPaths(page)["C.pptx"] {
			t.Errorf(">50MB 档应只命中 C.pptx，实际 total=%d items=%+v", page.Total, page.Items)
		}
	})

	t.Run("三维组合", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{
			Flatten: true,
			Ratings: []int64{0, 3},   // 未评分 + 3 星（A、B）
			MtimeTo: paths["B.pptx"], // 截至并含 B 的修改时间（A、B）
			SizeTo:  10 * mb,         // ≤10MB（A、B）
		})
		if err != nil {
			t.Fatalf("组合 facet: %v", err)
		}
		got := facetHitPaths(page)
		if page.Total != 2 || !got["A.pptx"] || !got["B.pptx"] || got["C.pptx"] {
			t.Errorf("组合条件应命中 A/B 排除 C，实际 total=%d items=%+v", page.Total, page.Items)
		}
	})

	t.Run("条目形态与排序", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: false, Ratings: []int64{0, 3, 5}})
		if err != nil {
			t.Fatalf("全量 facets-only: %v", err)
		}
		if page.Total != 3 || len(page.Items) != 3 {
			t.Fatalf("应命中全部 3 文件（聚合模式不改变文件条目形态），实际 total=%d", page.Total)
		}
		if page.Items[0].FilePath != "A.pptx" || page.Items[2].FilePath != "C.pptx" {
			t.Errorf("应按路径序返回，实际：%+v", page.Items)
		}
		for _, hit := range page.Items {
			if hit.RefType != model.AssetRefTypeFile {
				t.Errorf("facets-only 应返回文件条目，实际：%+v", hit)
			}
			if hit.Score != 0 || hit.Snippet != "" {
				t.Errorf("facets-only 无相关性语义（score=0 snippet 空），实际 score=%v snippet=%q", hit.Score, hit.Snippet)
			}
			if hit.HitCount != 1 {
				t.Errorf("文件条目 HitCount 应为 1，实际 %d", hit.HitCount)
			}
		}
	})

	t.Run("损坏文件不入筛选结果", func(t *testing.T) {
		dir := t.TempDir()
		lib2 := mustCreateLibrary(t, svc, "facet损坏库", dir)
		buildAssetsPptx(t, dir, "正常.pptx", []assetsSlideSpec{{titlePh: "首页", body: []string{"内容"}}})
		buildOLEEncryptedPptx(t, dir, "加密.pptx")
		mustScanIndex(t, svc, lib2)
		page, err := svc.SearchAssets(lib2.ID, "", model.AssetSearchOptions{Flatten: true, SizeFrom: 1})
		if err != nil {
			t.Fatalf("损坏库 facets-only: %v", err)
		}
		if page.Total != 1 || !facetHitPaths(page)["正常.pptx"] {
			t.Errorf("failed/encrypted 状态文件不应参与 facet 筛选，实际 total=%d items=%+v", page.Total, page.Items)
		}
	})
}

// TestAssetSearchFacets_AllEmptyReturnsEmptyPage 全空不变式：无关键词、无标签且
// 无任何 facet 条件仍返回空结果页（facet 批次语义重定义不得破坏原空查询行为）。
func TestAssetSearchFacets_AllEmptyReturnsEmptyPage(t *testing.T) {
	svc, lib, _ := newFacetFixture(t)
	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("全空检索不应报错: %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("全空应返回空页，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	// 空切片 Ratings 与 nil 同义（不构成 facet 条件）
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{}})
	if err != nil {
		t.Fatalf("空切片 ratings 检索不应报错: %v", err)
	}
	if page.Total != 0 {
		t.Errorf("Ratings 空切片应视为未启用 facet，实际 total=%d", page.Total)
	}
}

// TestAssetSearchFacets_RatingZeroSemantics 星级 0 = 未评分语义（D1）：ratings=[0]
// 只中未评分文件；ratings=[0,5] 同时中未评分与 5 星；页级评分（slide_meta.rating）
// 纯展示不参与筛选——未评分文件下已评分的页不因页星级命中。
func TestAssetSearchFacets_RatingZeroSemantics(t *testing.T) {
	svc, lib, _ := newFacetFixture(t)

	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{0}})
	if err != nil {
		t.Fatalf("ratings=[0]: %v", err)
	}
	if page.Total != 1 || !facetHitPaths(page)["A.pptx"] {
		t.Errorf("ratings=[0] 应只中未评分的 A.pptx，实际 total=%d items=%+v", page.Total, page.Items)
	}

	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{0, 5}})
	if err != nil {
		t.Fatalf("ratings=[0,5]: %v", err)
	}
	got := facetHitPaths(page)
	if page.Total != 2 || !got["A.pptx"] || !got["C.pptx"] {
		t.Errorf("ratings=[0,5] 应中 A/C，实际 total=%d items=%+v", page.Total, page.Items)
	}

	// 页级评分不参与：给 A.pptx 的页打页级星级 5，ratings=[5] 仍不命中 A.pptx
	//（文件级口径，页级评分纯展示）
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	var fileAID int64
	for _, f := range files {
		if f.Path == "A.pptx" {
			fileAID = f.ID
		}
	}
	if fileAID == 0 {
		t.Fatal("未找到 A.pptx 文件 ID")
	}
	if err := svc.UpdateSlideMeta(lib.ID, fileAID, 1, model.AssetSlideMetaUpdate{Rating: 5}); err != nil {
		t.Fatalf("UpdateSlideMeta: %v", err)
	}
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{5}})
	if err != nil {
		t.Fatalf("页级评分核对检索: %v", err)
	}
	if facetHitPaths(page)["A.pptx"] {
		t.Error("页级评分不应参与星级 facet（文件级口径）")
	}
}

// TestAssetSearchFacets_KeywordCombination keyword 路径 facet 组合：文件条目按
// 自身属性判定；幻灯片条目按归属文件属性继承判定（归属文件排除则其页一并出局，
// 聚合条目同理）；分页 Total 按 facet 过滤后口径计。
func TestAssetSearchFacets_KeywordCombination(t *testing.T) {
	svc, lib, paths := newFacetFixture(t)
	day := int64(24) * 3600 * 1e9

	// 全部文件页正文含「城商行」：平铺 6 条（3 文件 × 2 页）
	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("基线检索: %v", err)
	}
	if page.Total != 6 {
		t.Fatalf("基线平铺应命中 6 页，实际 %d", page.Total)
	}

	// facet 限定 A.pptx 时间当日：仅剩 A 的 2 页（幻灯片继承归属文件属性）
	page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
		Flatten: true, MtimeFrom: paths["A.pptx"] - day, MtimeTo: paths["A.pptx"] + day,
	})
	if err != nil {
		t.Fatalf("keyword+时间 facet: %v", err)
	}
	got := facetHitPaths(page)
	if page.Total != 2 || len(got) != 1 || !got["A.pptx"] {
		t.Errorf("facet 后应只剩 A.pptx 的 2 页，实际 total=%d items=%+v", page.Total, page.Items)
	}

	// 聚合模式：facet 排除后聚合条目同步出局（C 5 星不在 ratings=[3]）
	page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: false, Ratings: []int64{3}})
	if err != nil {
		t.Fatalf("keyword+星级 facet 聚合: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("聚合模式 facet 后应只剩 1 个聚合条目，实际 total=%d items=%+v", page.Total, page.Items)
	}
	if page.Items[0].FilePath != "B.pptx" || page.Items[0].HitCount != 2 {
		t.Errorf("聚合条目应为 B.pptx（2 页折叠），实际：%+v", page.Items[0])
	}

	// 文件条目（文件名命中）：给 A.pptx 加一个文件名含词的新库文件，facet 排除后
	// 文件条目同步出局
	dir := t.TempDir()
	lib2 := mustCreateLibrary(t, svc, "facet文件条目库", dir)
	buildAssetsPptx(t, dir, "城商行材料A.pptx", []assetsSlideSpec{{titlePh: "首页", body: []string{"内容"}}})
	buildAssetsPptx(t, dir, "城商行材料B.pptx", []assetsSlideSpec{{titlePh: "首页", body: []string{"内容"}}})
	mustScanIndex(t, svc, lib2)
	setAssetFileFacetProps(t, svc, lib2.ID, "城商行材料A.pptx", 1000, 1024)
	setAssetFileFacetProps(t, svc, lib2.ID, "城商行材料B.pptx", 1000, 999*1024*1024)
	page, err = svc.SearchAssets(lib2.ID, "城商行", model.AssetSearchOptions{Flatten: true, SizeTo: 1024 * 1024})
	if err != nil {
		t.Fatalf("文件条目 facet 检索: %v", err)
	}
	got = facetHitPaths(page)
	if page.Total != 1 || !got["城商行材料A.pptx"] {
		t.Errorf("size facet 后应只剩文件名命中的 A 文件条目，实际 total=%d items=%+v", page.Total, page.Items)
	}
}

// TestAssetSearchFacets_TagsOnlyCombination tagsOnly 路径 facet 组合：标签浏览
// 结果之上叠加 facet 条件（文件条目按自身属性、幻灯片条目经 JOIN files 按归属
// 文件属性判定），继承语义与关键词路径同源。
func TestAssetSearchFacets_TagsOnlyCombination(t *testing.T) {
	svc, lib, paths := newFacetFixture(t)
	day := int64(24) * 3600 * 1e9

	// 三文件全打同一标签 → 纯标签浏览基线（平铺 = 3 文件条目 + 6 页条目）
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{Tags: []string{"全域"}}); err != nil {
			t.Fatalf("UpdateFileMeta %s: %v", f.Path, err)
		}
	}
	// 重新列表取标签 ID（UpdateFileMeta 前快照不带标签）
	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("二次 ListFiles: %v", err)
	}
	var tagID int64
	for _, f := range files {
		if len(f.Tags) > 0 {
			tagID = f.Tags[0].ID
			break
		}
	}
	if tagID == 0 {
		t.Fatal("未取到标签 ID")
	}
	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
	if err != nil {
		t.Fatalf("纯标签浏览基线: %v", err)
	}
	if page.Total != 9 {
		t.Fatalf("基线应命中 3 文件 + 6 页 = 9 条，实际 total=%d", page.Total)
	}

	// facet 限定 A 当日：剩 1 文件条目 + 2 页条目
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{
		Flatten: true, TagIDs: []int64{tagID},
		MtimeFrom: paths["A.pptx"] - day, MtimeTo: paths["A.pptx"] + day,
	})
	if err != nil {
		t.Fatalf("tagsOnly+facet: %v", err)
	}
	got := facetHitPaths(page)
	if page.Total != 3 || len(got) != 1 || !got["A.pptx"] {
		t.Errorf("标签+facet 后应只剩 A.pptx（1 文件 + 2 页），实际 total=%d items=%+v", page.Total, page.Items)
	}
}

// TestAssetSearchFacets_FacetWithPagination facets-only 分页契约：Total 恒为
// 过滤后总数，翻页以 offset 递增驱动（对齐既有分页契约）。
func TestAssetSearchFacets_FacetWithPagination(t *testing.T) {
	svc, lib, _ := newFacetFixture(t)
	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{0, 3, 5}, Limit: 2})
	if err != nil {
		t.Fatalf("首页: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 2 {
		t.Fatalf("首页应 total=3 items=2，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	page2, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, Ratings: []int64{0, 3, 5}, Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("次页: %v", err)
	}
	if page2.Total != 3 || len(page2.Items) != 1 {
		t.Errorf("次页应 total=3 items=1，实际 total=%d items=%d", page2.Total, len(page2.Items))
	}
}

// TestAssetsFacetFilter_WhereClause whereClause 纯函数构造断言：rating 三分支
// （仅 0 / 混合 / 仅非零）与区间条件占位符形态，SQL 片段构造单点的确定性锚定。
func TestAssetsFacetFilter_WhereClause(t *testing.T) {
	cases := []struct {
		name     string
		f        *assetsFacetFilter
		wantClue []string // 片段必须包含的子串
		wantArgs int      // 参数个数
	}{
		{
			name:     "仅未评分",
			f:        &assetsFacetFilter{ratings: []int64{0}},
			wantClue: []string{"rating = 0"},
			wantArgs: 0,
		},
		{
			name:     "未评分与非零混合",
			f:        &assetsFacetFilter{ratings: []int64{0, 3, 5}},
			wantClue: []string{"rating = 0 OR f.rating IN (?, ?)"},
			wantArgs: 2,
		},
		{
			name:     "仅非零",
			f:        &assetsFacetFilter{ratings: []int64{3}},
			wantClue: []string{"rating IN (?)"},
			wantArgs: 1,
		},
		{
			name:     "时间与大小区间",
			f:        &assetsFacetFilter{mtimeFrom: 1, mtimeTo: 2, sizeFrom: 3, sizeTo: 4},
			wantClue: []string{"mtime_ns >= ?", "mtime_ns <= ?", "size >= ?", "size <= ?"},
			wantArgs: 4,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clause, args := tc.f.whereClause("f.")
			for _, clue := range tc.wantClue {
				if !strings.Contains(clause, clue) {
					t.Errorf("片段应含 %q，实际 %q", clue, clause)
				}
			}
			if len(args) != tc.wantArgs {
				t.Errorf("参数个数 = %d, want %d（片段 %q）", len(args), tc.wantArgs, clause)
			}
		})
	}
}

// TestAssetSearchFacets_NotFoundLibrary facets-only 路径同样校验库存在性
// （E_ASSETS_LIB_NOT_FOUND 分流不受空查询语义重定义影响）。
func TestAssetSearchFacets_NotFoundLibrary(t *testing.T) {
	svc := newTestAssetService(t)
	_, err := svc.SearchAssets("不存在的库", "", model.AssetSearchOptions{Ratings: []int64{3}})
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsLibNotFound {
		t.Fatalf("应报 E_ASSETS_LIB_NOT_FOUND，实际：%v", err)
	}
}
