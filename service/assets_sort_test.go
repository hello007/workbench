package service

import (
	"os"
	"testing"

	"workbench/model"
)

// ---------- 检索结果排序器测试（SRS §2.5.3，素材管理第八批 PR1） ----------
//
// 排序键全量经 fixture 注入固定 mtime_ns 与 rating（不依赖真实时钟与文件系统
// 时序，docs/spec/test-stability.md）；分层构造复用 newFacetFixture（A=0 星/T0、
// B=3 星/T0+10 天、C=5 星/T0+40 天，每文件 2 页正文均含「城商行」）。

// sortHitPaths 检索结果条目的归属文件路径序列（排序断言辅助，条目内页序不影响
// 断言——同一文件的条目路径相同，主键分层由文件间顺序证明）。
func sortHitPaths(page model.AssetSearchPage) []string {
	paths := make([]string, 0, len(page.Items))
	for _, hit := range page.Items {
		paths = append(paths, hit.FilePath)
	}
	return paths
}

// assertPathSeq 逐位断言路径序列一致。
func assertPathSeq(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("路径序列长度 = %d, want %d：%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("路径序列第 %d 位 = %q, want %q（全序列 %v）", i, got[i], want[i], got)
		}
	}
}

// TestAssetSearchSort_KeyNormalization 排序入参归一化表断言：SortBy 非法值/空串/
// 显式 relevance 均归一为相关性档（key=""），SortDir 仅显式 "asc" 为升序。
func TestAssetSearchSort_KeyNormalization(t *testing.T) {
	cases := []struct {
		name    string
		sortBy  string
		sortDir string
		wantKey string
		wantAsc bool
	}{
		{name: "空串默认相关性降序", sortBy: "", sortDir: "", wantKey: "", wantAsc: false},
		{name: "显式相关性", sortBy: model.AssetSortByRelevance, sortDir: model.AssetSortDirDesc, wantKey: "", wantAsc: false},
		{name: "mtime默认降序", sortBy: model.AssetSortByMtime, sortDir: "", wantKey: model.AssetSortByMtime, wantAsc: false},
		{name: "mtime升序", sortBy: model.AssetSortByMtime, sortDir: model.AssetSortDirAsc, wantKey: model.AssetSortByMtime, wantAsc: true},
		{name: "rating升序", sortBy: model.AssetSortByRating, sortDir: model.AssetSortDirAsc, wantKey: model.AssetSortByRating, wantAsc: true},
		{name: "rating非法方向按降序", sortBy: model.AssetSortByRating, sortDir: "sideways", wantKey: model.AssetSortByRating, wantAsc: false},
		{name: "非法档位按相关性", sortBy: "bogus", sortDir: model.AssetSortDirAsc, wantKey: "", wantAsc: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, asc := assetSortKey(model.AssetSearchOptions{SortBy: tc.sortBy, SortDir: tc.sortDir})
			if key != tc.wantKey || asc != tc.wantAsc {
				t.Errorf("assetSortKey(%q,%q) = (%q,%v), want (%q,%v)",
					tc.sortBy, tc.sortDir, key, asc, tc.wantKey, tc.wantAsc)
			}
		})
	}
}

// TestAssetSearchSort_KeywordMtime 关键词路径 mtime 排序：平铺降序/升序按归属
// 文件 mtime_ns 分层；聚合模式组首继承文件维度（聚合条目按文件 mtime 排序、
// HitCount 折叠不丢）；分页切片基于排序后全量序（各页拼接 = 全量序，跨页稳定）。
func TestAssetSearchSort_KeywordMtime(t *testing.T) {
	svc, lib, _ := newFacetFixture(t)

	t.Run("平铺降序", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
			Flatten: true, SortBy: model.AssetSortByMtime,
		})
		if err != nil {
			t.Fatalf("mtime 降序检索: %v", err)
		}
		if page.Total != 6 {
			t.Fatalf("应命中 6 页，实际 total=%d", page.Total)
		}
		// mtime：C（T0+40 天）> B（T0+10 天）> A（T0），同文件 2 页相邻
		assertPathSeq(t, sortHitPaths(page),
			"C.pptx", "C.pptx", "B.pptx", "B.pptx", "A.pptx", "A.pptx")
	})

	t.Run("平铺升序", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
			Flatten: true, SortBy: model.AssetSortByMtime, SortDir: model.AssetSortDirAsc,
		})
		if err != nil {
			t.Fatalf("mtime 升序检索: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page),
			"A.pptx", "A.pptx", "B.pptx", "B.pptx", "C.pptx", "C.pptx")
	})

	t.Run("聚合模式组首继承文件维度", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
			Flatten: false, SortBy: model.AssetSortByMtime,
		})
		if err != nil {
			t.Fatalf("mtime 聚合检索: %v", err)
		}
		if page.Total != 3 || len(page.Items) != 3 {
			t.Fatalf("聚合后应 3 个条目，实际 total=%d items=%d", page.Total, len(page.Items))
		}
		assertPathSeq(t, sortHitPaths(page), "C.pptx", "B.pptx", "A.pptx")
		for _, hit := range page.Items {
			if hit.HitCount != 2 {
				t.Errorf("聚合条目 %s HitCount 应为 2，实际 %d", hit.FilePath, hit.HitCount)
			}
			if hit.RefType != model.AssetRefTypeSlide {
				t.Errorf("聚合条目应为幻灯片形态，实际：%+v", hit)
			}
		}
	})

	t.Run("跨分页稳定", func(t *testing.T) {
		full, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
			Flatten: true, SortBy: model.AssetSortByMtime, Limit: 200,
		})
		if err != nil {
			t.Fatalf("全量检索: %v", err)
		}
		var joined []string
		for offset := 0; offset < 6; offset += 2 {
			page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
				Flatten: true, SortBy: model.AssetSortByMtime, Limit: 2, Offset: offset,
			})
			if err != nil {
				t.Fatalf("分页检索 offset=%d: %v", offset, err)
			}
			joined = append(joined, sortHitPaths(page)...)
		}
		assertPathSeq(t, joined, sortHitPaths(full)...)
	})
}

// TestAssetSearchSort_KeywordRating 关键词路径 rating 排序：排序键为 files.rating
// 文件级口径（幻灯片条目继承归属文件星级）。将 A 提到 4 星后 rating 档分层
// C(5) > A(4) > B(3) 与 mtime 档分层 C > B > A 相互区分，排除误用 mtime 排序的
// 假阳性；文件名命中产生的文件条目同样按 files.rating/文件 mtime 排序。
func TestAssetSearchSort_KeywordRating(t *testing.T) {
	svc, lib, _ := newFacetFixture(t)
	// A 提到 4 星（原 0），rating 分层变为 C(5) > A(4) > B(3)
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if f.Path == "A.pptx" {
			if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{Rating: 4}); err != nil {
				t.Fatalf("UpdateFileMeta A: %v", err)
			}
		}
	}

	t.Run("平铺降序继承文件星级", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
			Flatten: true, SortBy: model.AssetSortByRating,
		})
		if err != nil {
			t.Fatalf("rating 降序检索: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page),
			"C.pptx", "C.pptx", "A.pptx", "A.pptx", "B.pptx", "B.pptx")
	})

	t.Run("平铺升序", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
			Flatten: true, SortBy: model.AssetSortByRating, SortDir: model.AssetSortDirAsc,
		})
		if err != nil {
			t.Fatalf("rating 升序检索: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page),
			"B.pptx", "B.pptx", "A.pptx", "A.pptx", "C.pptx", "C.pptx")
	})

	t.Run("聚合模式按文件星级", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
			Flatten: false, SortBy: model.AssetSortByRating,
		})
		if err != nil {
			t.Fatalf("rating 聚合检索: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page), "C.pptx", "A.pptx", "B.pptx")
	})

	// 文件条目（文件名命中）排序：三文件 rating 与 mtime 分层互异，排除误用
	// 对方排序键的假阳性（页正文不含检索词，仅文件名命中 → 纯文件条目）
	t.Run("文件条目rating与mtime分档", func(t *testing.T) {
		dir := t.TempDir()
		lib2 := mustCreateLibrary(t, svc, "排序文件条目库", dir)
		specs := []assetsSlideSpec{{titlePh: "首页", body: []string{"内容"}}}
		buildAssetsPptx(t, dir, "城商行A材料.pptx", specs)
		buildAssetsPptx(t, dir, "城商行B材料.pptx", specs)
		buildAssetsPptx(t, dir, "城商行C材料.pptx", specs)
		mustScanIndex(t, svc, lib2)
		t0 := int64(1700000000) * 1e9
		day := int64(24) * 3600 * 1e9
		// A：rating 1 / mtime 居中；B：rating 4 / mtime 最旧；C：rating 3 / mtime 最新
		setAssetFileFacetProps(t, svc, lib2.ID, "城商行A材料.pptx", t0+10*day, 1024)
		setAssetFileFacetProps(t, svc, lib2.ID, "城商行B材料.pptx", t0, 1024)
		setAssetFileFacetProps(t, svc, lib2.ID, "城商行C材料.pptx", t0+40*day, 1024)
		lib2Files, err := svc.ListFiles(lib2.ID)
		if err != nil {
			t.Fatalf("ListFiles: %v", err)
		}
		ratingByPath := map[string]int{"城商行A材料.pptx": 1, "城商行B材料.pptx": 4, "城商行C材料.pptx": 3}
		for _, f := range lib2Files {
			if err := svc.UpdateFileMeta(lib2.ID, f.ID, model.AssetFileMetaUpdate{Rating: ratingByPath[f.Path]}); err != nil {
				t.Fatalf("UpdateFileMeta %s: %v", f.Path, err)
			}
		}

		// rating 降序：B(4) > C(3) > A(1)；mtime 降序：C > A > B；路径序：A < B < C
		// 三序互异，档位互斥可证
		page, err := svc.SearchAssets(lib2.ID, "城商行", model.AssetSearchOptions{
			Flatten: true, SortBy: model.AssetSortByRating,
		})
		if err != nil {
			t.Fatalf("文件条目 rating 检索: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page), "城商行B材料.pptx", "城商行C材料.pptx", "城商行A材料.pptx")

		page, err = svc.SearchAssets(lib2.ID, "城商行", model.AssetSearchOptions{
			Flatten: true, SortBy: model.AssetSortByMtime,
		})
		if err != nil {
			t.Fatalf("文件条目 mtime 检索: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page), "城商行C材料.pptx", "城商行A材料.pptx", "城商行B材料.pptx")
	})
}

// TestAssetSearchSort_TagsOnly 纯标签浏览路径排序：相关性档保持路径序现状
// （文件本体先于其页）；mtime 档按归属文件 mtime 分层重排（降序/升序），非法
// SortDir 按降序处理。
func TestAssetSearchSort_TagsOnly(t *testing.T) {
	svc, lib, _ := newFacetFixture(t)
	// 三文件打同一标签 → 纯标签浏览 3 文件条目 + 6 页条目
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{Tags: []string{"全域"}}); err != nil {
			t.Fatalf("UpdateFileMeta %s: %v", f.Path, err)
		}
	}
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
	baseOpts := model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}}

	t.Run("相关性档路径序现状不动", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "", baseOpts)
		if err != nil {
			t.Fatalf("纯标签基线: %v", err)
		}
		if page.Total != 9 {
			t.Fatalf("应命中 9 条，实际 total=%d", page.Total)
		}
		assertPathSeq(t, sortHitPaths(page),
			"A.pptx", "A.pptx", "A.pptx", "B.pptx", "B.pptx", "B.pptx", "C.pptx", "C.pptx", "C.pptx")
		if page.Items[0].RefType != model.AssetRefTypeFile {
			t.Errorf("同文件组首应为文件条目（文件本体先于其页），实际：%+v", page.Items[0])
		}
	})

	t.Run("mtime降序与非法方向", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{
			Flatten: true, TagIDs: []int64{tagID}, SortBy: model.AssetSortByMtime,
		})
		if err != nil {
			t.Fatalf("mtime 降序: %v", err)
		}
		want := []string{
			"C.pptx", "C.pptx", "C.pptx",
			"B.pptx", "B.pptx", "B.pptx",
			"A.pptx", "A.pptx", "A.pptx",
		}
		assertPathSeq(t, sortHitPaths(page), want...)
		// 同文件条目主键同值：稳定序保留路径序的文件本体先于其页分组语义
		if page.Items[0].RefType != model.AssetRefTypeFile {
			t.Errorf("mtime 档同文件组首应为文件条目，实际：%+v", page.Items[0])
		}
		// 非法 SortDir 按降序处理：结果与显式降序一致
		page2, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{
			Flatten: true, TagIDs: []int64{tagID}, SortBy: model.AssetSortByMtime, SortDir: "sideways",
		})
		if err != nil {
			t.Fatalf("非法方向: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page2), sortHitPaths(page)...)
	})

	t.Run("mtime升序", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{
			Flatten: true, TagIDs: []int64{tagID}, SortBy: model.AssetSortByMtime, SortDir: model.AssetSortDirAsc,
		})
		if err != nil {
			t.Fatalf("mtime 升序: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page),
			"A.pptx", "A.pptx", "A.pptx", "B.pptx", "B.pptx", "B.pptx", "C.pptx", "C.pptx", "C.pptx")
	})
}

// TestAssetSearchSort_FacetsOnly facets-only 路径排序：rating 档与 mtime 档分层
// 互异（排除误用对方排序键），相关性档路径序由既有 facet 测试锚定不重复。
func TestAssetSearchSort_FacetsOnly(t *testing.T) {
	svc, lib, _ := newFacetFixture(t)
	// A 提到 4 星：rating 分层 C(5) > A(4) > B(3)，与 mtime 分层 C > B > A 互异
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if f.Path == "A.pptx" {
			if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{Rating: 4}); err != nil {
				t.Fatalf("UpdateFileMeta A: %v", err)
			}
		}
	}
	allFiles := model.AssetSearchOptions{Flatten: true, Ratings: []int64{0, 3, 4, 5}}

	t.Run("rating降序升序", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "", mergeSortOptions(allFiles, model.AssetSortByRating, ""))
		if err != nil {
			t.Fatalf("rating 降序: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page), "C.pptx", "A.pptx", "B.pptx")

		page, err = svc.SearchAssets(lib.ID, "", mergeSortOptions(allFiles, model.AssetSortByRating, model.AssetSortDirAsc))
		if err != nil {
			t.Fatalf("rating 升序: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page), "B.pptx", "A.pptx", "C.pptx")
	})

	t.Run("mtime降序", func(t *testing.T) {
		page, err := svc.SearchAssets(lib.ID, "", mergeSortOptions(allFiles, model.AssetSortByMtime, ""))
		if err != nil {
			t.Fatalf("mtime 降序: %v", err)
		}
		assertPathSeq(t, sortHitPaths(page), "C.pptx", "B.pptx", "A.pptx")
	})
}

// mergeSortOptions 测试辅助：facet 条件之上叠加排序档位。
func mergeSortOptions(base model.AssetSearchOptions, sortBy, sortDir string) model.AssetSearchOptions {
	base.SortBy = sortBy
	base.SortDir = sortDir
	return base
}

// TestAssetSearchSort_KeywordInvalidSortByEqualsRelevance 关键词路径非法档位按
// 相关性处理：SortBy 非法值的结果序与不带排序参数的默认序逐位一致（FTS bm25 序
// 现状不动）。
func TestAssetSearchSort_KeywordInvalidSortByEqualsRelevance(t *testing.T) {
	svc, lib, _ := newFacetFixture(t)
	def, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, Limit: 200})
	if err != nil {
		t.Fatalf("默认序检索: %v", err)
	}
	bogus, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
		Flatten: true, Limit: 200, SortBy: "bogus",
	})
	if err != nil {
		t.Fatalf("非法档位检索: %v", err)
	}
	if bogus.Total != def.Total {
		t.Fatalf("非法档位 total = %d, want %d", bogus.Total, def.Total)
	}
	assertPathSeq(t, sortHitPaths(bogus), sortHitPaths(def)...)
}

// TestAssetSavedSearch_SortFieldsRoundTripAndLegacy 保存检索条件排序字段：新增
// SortBy/SortDir 持久化读回一致；旧快照（无排序字段的 JSON）反序列化为零值 =
// 相关性排序（向后兼容，无版本号）。
func TestAssetSavedSearch_SortFieldsRoundTripAndLegacy(t *testing.T) {
	svc, path := newSavedSearchTestService(t)

	saved, err := svc.Save(model.AssetSavedSearch{
		Name:      "排序条件",
		LibraryID: "lib-a",
		Keyword:   "城商行",
		SortBy:    model.AssetSortByMtime,
		SortDir:   model.AssetSortDirAsc,
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.SortBy != model.AssetSortByMtime || saved.SortDir != model.AssetSortDirAsc {
		t.Errorf("保存返回应带排序字段，实际 sortBy=%q sortDir=%q", saved.SortBy, saved.SortDir)
	}

	// 新实例读回：排序字段落盘不丢
	reborn := NewAssetSavedSearchService(path)
	list, err := reborn.List()
	if err != nil {
		t.Fatalf("新实例 List: %v", err)
	}
	if len(list) != 1 || list[0].SortBy != model.AssetSortByMtime || list[0].SortDir != model.AssetSortDirAsc {
		t.Errorf("读回应保留排序字段，实际：%+v", list)
	}

	// 旧快照兼容：无 sortBy/sortDir 字段的 JSON 反序列化为零值（相关性排序），不报错
	legacyPath := path + ".legacy"
	legacyJSON := `{"searches":[{"id":1,"name":"旧快照","libraryId":"lib-a","keyword":"城商行","createdAt":"2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(legacyPath, []byte(legacyJSON), 0644); err != nil {
		t.Fatalf("写入旧快照: %v", err)
	}
	legacySvc := NewAssetSavedSearchService(legacyPath)
	legacyList, err := legacySvc.List()
	if err != nil {
		t.Fatalf("旧快照 List 不应报错: %v", err)
	}
	if len(legacyList) != 1 || legacyList[0].SortBy != "" || legacyList[0].SortDir != "" {
		t.Errorf("旧快照排序字段应为零值（相关性），实际：%+v", legacyList)
	}
}

// TestAssetSearchSort_KeywordMtime_MissingFileProps 关键词排序档的并发删除窗口：
// 排序属性批量查询前文件行被删（props 缺键）时，该文件全部条目剔除而非以零值
// mtime/rating 参与排序（零值会把已删文件排到序端形成用户可见异常序）；剔除后
// total 口径收敛为存活条目。相关性档不走该查询，行为不变（由既有用例锚定）。
func TestAssetSearchSort_KeywordMtime_MissingFileProps(t *testing.T) {
	svc, lib, _ := newFacetFixture(t)
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	var cID int64
	for _, f := range files {
		if f.Path == "C.pptx" {
			cID = f.ID
		}
	}
	if cID == 0 {
		t.Fatal("fixture 应含 C.pptx")
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("DELETE FROM files WHERE id = ?", cID); err != nil {
		t.Fatalf("模拟并发删除 C.pptx: %v", err)
	}

	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
		Flatten: true, SortBy: model.AssetSortByMtime,
	})
	if err != nil {
		t.Fatalf("mtime 降序检索: %v", err)
	}
	// C（40 天）消失后仅剩 A（T0）/B（T0+10 天）各 2 页，降序 B 先于 A
	assertPathSeq(t, sortHitPaths(page), "B.pptx", "B.pptx", "A.pptx", "A.pptx")
	if page.Total != 4 {
		t.Errorf("剔除已删文件后 total 应为 4，实际 %d", page.Total)
	}

	page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
		Flatten: true, SortBy: model.AssetSortByMtime, SortDir: model.AssetSortDirAsc,
	})
	if err != nil {
		t.Fatalf("mtime 升序检索: %v", err)
	}
	assertPathSeq(t, sortHitPaths(page), "A.pptx", "A.pptx", "B.pptx", "B.pptx")
}
