package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- 幻灯片页级独立标注（SRS §2.2.4，schema v5） ----------
//
// 覆盖面：迁移（v4→v5/新库直建）、外键级联、页标注读写（整体替换/错误分流/
// 存储独立性）、标签继承合并筛选（纯标签浏览与 FTS 混合检索两路径 × 默认/
// OnlyOwn 两模式）、重建按 path+slide_num 重绑保留、增量重解析保留与失配清理、
// 保存无副作用。fixture 全部程序化构造；重解析用注入陈旧 mtime 驱动（禁依赖
// NTFS mtime 时序，docs/spec/test-stability.md）。

// countSlideAnnotRows 统计页级标注表行数（白盒断言用）。
func countSlideAnnotRows(t *testing.T, store *assetsStore, table string) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("统计 %s 行数: %v", table, err)
	}
	return n
}

// mustListSlides 幻灯片全量清单断言辅助（limit 取上限 200 覆盖测试语料）。
func mustListSlides(t *testing.T, svc *AssetLibraryService, libraryID string) []model.AssetSlideEntry {
	t.Helper()
	page, err := svc.ListSlides(libraryID, 200, 0)
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	return page.Items
}

// slideEntryByNum 取指定文件路径与页序的条目（未命中 Fatal）。
func slideEntryByNum(t *testing.T, entries []model.AssetSlideEntry, path string, num int) model.AssetSlideEntry {
	t.Helper()
	for _, e := range entries {
		if e.FilePath == path && e.SlideNum == num {
			return e
		}
	}
	t.Fatalf("未找到条目 %s 第 %d 页", path, num)
	return model.AssetSlideEntry{}
}

// mustUpdateSlideMeta 页级标注保存断言辅助。
func mustUpdateSlideMeta(t *testing.T, svc *AssetLibraryService, libraryID string, fileID int64, slideNum int, meta model.AssetSlideMetaUpdate) {
	t.Helper()
	if err := svc.UpdateSlideMeta(libraryID, fileID, slideNum, meta); err != nil {
		t.Fatalf("UpdateSlideMeta(%d, %d): %v", fileID, slideNum, err)
	}
}

// tagNameSet 标签列表转名字集合（断言用）。
func tagNameSet(tags []model.AssetTag) map[string]bool {
	names := make(map[string]bool, len(tags))
	for _, tag := range tags {
		names[tag.Name] = true
	}
	return names
}

// hitKeySet 检索结果条目集合化（refType+路径+页序，断言用）。
type hitKey struct {
	refType string
	path    string
	num     int
}

func hitKeySet(page model.AssetSearchPage) map[hitKey]bool {
	set := make(map[hitKey]bool, len(page.Items))
	for _, h := range page.Items {
		set[hitKey{refType: h.RefType, path: h.FilePath, num: h.SlideNum}] = true
	}
	return set
}

// ---------- schema v5 迁移 ----------

// TestAssetsStore_MigrateV5 迁移链：旧 v4 库升级后 slide_tags/slide_meta 建表
// （存量标注天然为空，无 Go 层回填）且存量数据保留、版本钉到 v5；新库直建即 v5。
func TestAssetsStore_MigrateV5(t *testing.T) {
	t.Run("旧库 v4 升级", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "v4.db")
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
		if err != nil {
			t.Fatalf("打开原始库: %v", err)
		}
		for _, v := range []int{1, 2, 3, 4} {
			if _, err := db.Exec(assetsMigrations[v]); err != nil {
				t.Fatalf("手工执行 v%d 迁移: %v", v, err)
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 4"); err != nil {
			t.Fatalf("钉版本: %v", err)
		}
		if _, err := db.Exec(
			"INSERT INTO files(path, status, created_at, updated_at) VALUES('存量.pptx', 'ok', '2026-01-01 00:00:00', '2026-01-01 00:00:00')",
		); err != nil {
			t.Fatalf("插入存量数据: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("关闭原始库: %v", err)
		}

		store, err := openAssetsStore(dbPath)
		if err != nil {
			t.Fatalf("升级打开: %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })

		// 两张新表可查询且为空（存量库无页级标注，空表即正确初态）
		for _, table := range []string{"slide_tags", "slide_meta"} {
			if n := countSlideAnnotRows(t, store, table); n != 0 {
				t.Errorf("升级后 %s 应为空表，实际 %d 行", table, n)
			}
		}
		var files int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&files); err != nil {
			t.Fatalf("读存量数据: %v", err)
		}
		if files != 1 {
			t.Errorf("存量数据应保留，实际 %d 行", files)
		}
		var version int
		if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != assetsSchemaVersion {
			t.Errorf("版本应钉到 v%d，实际 v%d（err=%v）", assetsSchemaVersion, version, err)
		}
	})

	t.Run("新库直建 v5", func(t *testing.T) {
		store, err := openAssetsStore(filepath.Join(t.TempDir(), "fresh.db"))
		if err != nil {
			t.Fatalf("新库打开: %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })
		for _, table := range []string{"slide_tags", "slide_meta"} {
			if n := countSlideAnnotRows(t, store, table); n != 0 {
				t.Errorf("新库应有 %s 表（空），实际查询失败或非空", table)
			}
		}
	})
}

// TestAssetsSchema_SlideAnnotFkCascade 页级标注外键级联：删 files 行时
// slide_tags/slide_meta 随 ON DELETE CASCADE 清除（deleteFilesByPaths/重建清表
// 依赖该语义；clearIndexForRebuild 的显式 DELETE 为双保险非唯一防线）；删 tags
// 行时 slide_tags 绑定随之清除（与 file_tags 同构）。
func TestAssetsSchema_SlideAnnotFkCascade(t *testing.T) {
	store, err := openAssetsStore(filepath.Join(t.TempDir(), "fk.db"))
	if err != nil {
		t.Fatalf("打开新库: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.db.Exec(
		"INSERT INTO files(path, status) VALUES(?, ?)", "级联.pptx", model.AssetFileStatusOK,
	); err != nil {
		t.Fatalf("插文件行: %v", err)
	}
	fileID := fileIDByPath(t, store, "级联.pptx")
	if _, err := store.db.Exec("INSERT INTO tags(name) VALUES('级联标签')"); err != nil {
		t.Fatalf("插标签行: %v", err)
	}
	var tagID int64
	if err := store.db.QueryRow("SELECT id FROM tags WHERE name = '级联标签'").Scan(&tagID); err != nil {
		t.Fatalf("读标签 ID: %v", err)
	}
	if _, err := store.db.Exec(
		"INSERT INTO slide_tags(file_id, slide_num, tag_id) VALUES(?, 1, ?)", fileID, tagID,
	); err != nil {
		t.Fatalf("插页标签: %v", err)
	}
	if _, err := store.db.Exec(
		"INSERT INTO slide_meta(file_id, slide_num, note, rating) VALUES(?, 1, '备注', 3)", fileID,
	); err != nil {
		t.Fatalf("插页元数据: %v", err)
	}

	// 删标签本体：slide_tags 绑定级联清除，slide_meta 不动
	if _, err := store.db.Exec("DELETE FROM tags WHERE id = ?", tagID); err != nil {
		t.Fatalf("删标签行: %v", err)
	}
	if n := countSlideAnnotRows(t, store, "slide_tags"); n != 0 {
		t.Errorf("删标签后 slide_tags 应级联清空，实际 %d 行", n)
	}
	if n := countSlideAnnotRows(t, store, "slide_meta"); n != 1 {
		t.Errorf("删标签不应影响 slide_meta，实际 %d 行", n)
	}

	// 删文件行：slide_tags/slide_meta 全部级联清除
	if _, err := store.db.Exec("DELETE FROM files WHERE id = ?", fileID); err != nil {
		t.Fatalf("删文件行: %v", err)
	}
	if n := countSlideAnnotRows(t, store, "slide_meta"); n != 0 {
		t.Errorf("删文件后 slide_meta 应级联清空，实际 %d 行", n)
	}
}

// ---------- 页标注读写 ----------

// TestUpdateSlideMeta_RoundTrip 页级标注编辑：保存读回（OwnTags/Note/Rating 经
// ListSlides 条目携带）；文件/页不存在错误分流；星级越界与非法页序拒绝；标签
// 整体替换（旧删新留）与不存在的标签自动创建。
func TestUpdateSlideMeta_RoundTrip(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "页标注库", dir)
	buildAssetsPptx(t, dir, "标注样本.pptx", []assetsSlideSpec{
		{titlePh: "首页", body: []string{"城商行内容"}},
		{titlePh: "次页", body: []string{"其他内容"}},
	})
	mustScanIndex(t, svc, lib)
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	fileID := files[0].ID

	// 保存读回：星级/备注/自有标签经条目携带
	mustUpdateSlideMeta(t, svc, lib.ID, fileID, 1, model.AssetSlideMetaUpdate{
		Rating: 4, Note: "重点页备注", TagNames: []string{"获客", "城商行"},
	})
	entry := slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "标注样本.pptx", 1)
	if entry.Rating != 4 || entry.Note != "重点页备注" {
		t.Errorf("页标注读回不符：rating=%d note=%q", entry.Rating, entry.Note)
	}
	if !tagNameSet(entry.OwnTags)["获客"] || !tagNameSet(entry.OwnTags)["城商行"] {
		t.Errorf("页自有标签读回不符：%+v", entry.OwnTags)
	}
	if len(entry.Tags) != 0 {
		t.Errorf("未打文件标签时继承标签应为空，实际：%+v", entry.Tags)
	}
	// 未标注页零值形态（OwnTags 空切片非 nil，对齐 Tags 先例）
	other := slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "标注样本.pptx", 2)
	if other.Rating != 0 || other.Note != "" || other.OwnTags == nil || len(other.OwnTags) != 0 {
		t.Errorf("未标注页应为零值（OwnTags 空切片），实际：%+v", other)
	}

	// 标签整体替换：旧删新留；星级/备注整体替换
	mustUpdateSlideMeta(t, svc, lib.ID, fileID, 1, model.AssetSlideMetaUpdate{
		Rating: 2, Note: "改后备注", TagNames: []string{"城商行", "重点页", "  城商行  "},
	})
	entry = slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "标注样本.pptx", 1)
	if entry.Rating != 2 || entry.Note != "改后备注" {
		t.Errorf("页标注整体替换不符：rating=%d note=%q", entry.Rating, entry.Note)
	}
	names := tagNameSet(entry.OwnTags)
	if len(entry.OwnTags) != 2 || !names["城商行"] || !names["重点页"] {
		t.Errorf("标签应整体替换且去空白去重（旧标签「获客」移除），实际：%+v", entry.OwnTags)
	}

	// 错误分流：文件不存在 / 页不存在 / 星级越界 / 页序非法
	if err := svc.UpdateSlideMeta(lib.ID, 99999, 1, model.AssetSlideMetaUpdate{}); err == nil ||
		!strings.Contains(err.Error(), "素材文件不存在") {
		t.Errorf("文件不存在应报可读错误，实际：%v", err)
	}
	if err := svc.UpdateSlideMeta(lib.ID, fileID, 99, model.AssetSlideMetaUpdate{}); err == nil ||
		!strings.Contains(err.Error(), "幻灯片页不存在") {
		t.Errorf("页不存在应报可读错误，实际：%v", err)
	}
	if err := svc.UpdateSlideMeta(lib.ID, fileID, 1, model.AssetSlideMetaUpdate{Rating: 6}); err == nil ||
		!strings.Contains(err.Error(), "0-5") {
		t.Errorf("星级越界应拒绝，实际：%v", err)
	}
	if err := svc.UpdateSlideMeta(lib.ID, fileID, 0, model.AssetSlideMetaUpdate{}); err == nil ||
		!strings.Contains(err.Error(), "页序") {
		t.Errorf("页序 <1 应拒绝，实际：%v", err)
	}
}

// TestSlideAnnotation_StorageIndependence 存储层完全独立：文件标签增删不写
// slide_tags；页自有标签变更不写 file_tags（SRS §2.2.4 三契约之存储层）。
func TestSlideAnnotation_StorageIndependence(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "独立库", dir)
	buildAssetsPptx(t, dir, "独立样本.pptx", []assetsSlideSpec{
		{titlePh: "首页", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)
	store := svc.openStoreForTest(t, lib.ID)
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	fileID := files[0].ID

	// 文件打标签：slide_tags 零写入
	if err := svc.UpdateFileMeta(lib.ID, fileID, model.AssetFileMetaUpdate{Tags: []string{"文件标签"}}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	if n := countSlideAnnotRows(t, store, "slide_tags"); n != 0 {
		t.Errorf("文件打标签不应写 slide_tags，实际 %d 行", n)
	}

	// 页打自有标签：file_tags 不受影响
	mustUpdateSlideMeta(t, svc, lib.ID, fileID, 1, model.AssetSlideMetaUpdate{TagNames: []string{"页标签"}})
	if n := countSlideAnnotRows(t, store, "slide_tags"); n != 1 {
		t.Errorf("页打标签后 slide_tags 应 1 行，实际 %d 行", n)
	}
	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("二次 ListFiles: %v", err)
	}
	if len(files[0].Tags) != 1 || files[0].Tags[0].Name != "文件标签" {
		t.Errorf("页标签变更不应影响文件标签，实际：%+v", files[0].Tags)
	}

	// 文件标签变更：slide_tags 行不变
	if err := svc.UpdateFileMeta(lib.ID, fileID, model.AssetFileMetaUpdate{Tags: []string{"换标签"}}); err != nil {
		t.Fatalf("二次 UpdateFileMeta: %v", err)
	}
	if n := countSlideAnnotRows(t, store, "slide_tags"); n != 1 {
		t.Errorf("文件标签变更不应动 slide_tags，实际 %d 行", n)
	}
}

// ---------- 标签继承合并筛选 ----------

// TestSlideTagFilter_InheritanceMerge 标签继承三契约之筛选层：默认合并【页自身
// 标签 ∪ 归属文件标签】，SlideTagOnlyOwn=true 仅页自身标签；文件条目不受开关
// 影响；纯标签浏览与关键词混合检索两路径都生效。
//
// 语料：甲.pptx（2 页）打文件标签「重点」；乙.pptx（2 页）仅第 1 页打页自有
// 标签「页标」；两文件正文同词保证关键词全页命中。
func TestSlideTagFilter_InheritanceMerge(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "继承筛选库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{
		{titlePh: "甲一", body: []string{"城商行内容"}},
		{titlePh: "甲二", body: []string{"城商行内容"}},
	})
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{
		{titlePh: "乙一", body: []string{"城商行内容"}},
		{titlePh: "乙二", body: []string{"城商行内容"}},
	})
	mustScanIndex(t, svc, lib)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idsByPath := make(map[string]int64, len(files))
	for _, f := range files {
		idsByPath[f.Path] = f.ID
	}
	if err := svc.UpdateFileMeta(lib.ID, idsByPath["甲.pptx"], model.AssetFileMetaUpdate{Tags: []string{"重点"}}); err != nil {
		t.Fatalf("文件打标签: %v", err)
	}
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["乙.pptx"], 1, model.AssetSlideMetaUpdate{TagNames: []string{"页标"}})

	// 反查标签 ID（文件标签「重点」/ 页自有标签「页标」各一）
	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("二次 ListFiles: %v", err)
	}
	var fileTagID int64
	for _, f := range files {
		if f.Path == "甲.pptx" && len(f.Tags) > 0 {
			fileTagID = f.Tags[0].ID
		}
	}
	var slideTagID int64
	for _, e := range mustListSlides(t, svc, lib.ID) {
		if e.FilePath == "乙.pptx" && e.SlideNum == 1 && len(e.OwnTags) > 0 {
			slideTagID = e.OwnTags[0].ID
		}
	}
	if fileTagID == 0 || slideTagID == 0 {
		t.Fatalf("标签 ID 反查失败：fileTagID=%d slideTagID=%d", fileTagID, slideTagID)
	}

	contains := func(set map[hitKey]bool, refType, path string, num int) bool {
		return set[hitKey{refType: refType, path: path, num: num}]
	}

	t.Run("纯标签浏览_默认合并", func(t *testing.T) {
		// 「重点」（文件标签）：甲文件条目 + 甲全部页（继承）；乙不命中
		page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{fileTagID}})
		if err != nil {
			t.Fatalf("纯标签浏览: %v", err)
		}
		set := hitKeySet(page)
		if page.Total != 3 || !contains(set, model.AssetRefTypeFile, "甲.pptx", 0) ||
			!contains(set, model.AssetRefTypeSlide, "甲.pptx", 1) || !contains(set, model.AssetRefTypeSlide, "甲.pptx", 2) {
			t.Errorf("文件标签应命中甲条目与其全部页（默认合并），实际 total=%d set=%v", page.Total, set)
		}
		if contains(set, model.AssetRefTypeSlide, "乙.pptx", 1) {
			t.Error("乙未关联「重点」，不应命中")
		}
	})
	t.Run("纯标签浏览_OnlyOwn", func(t *testing.T) {
		// 「重点」+ OnlyOwn：甲页无自有标签，页条目全不出；文件条目不受开关影响
		page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{
			Flatten: true, TagIDs: []int64{fileTagID}, SlideTagOnlyOwn: true,
		})
		if err != nil {
			t.Fatalf("OnlyOwn 浏览: %v", err)
		}
		set := hitKeySet(page)
		if page.Total != 1 || !contains(set, model.AssetRefTypeFile, "甲.pptx", 0) {
			t.Errorf("OnlyOwn 下文件条目应保留、页条目应消失，实际 total=%d set=%v", page.Total, set)
		}
	})
	t.Run("纯标签浏览_页自有标签", func(t *testing.T) {
		// 「页标」：仅乙第 1 页命中（自身绑定），乙文件条目与乙其余页不命中；
		// 开关两档同语义（该页本就只有自身标签）
		for _, onlyOwn := range []bool{false, true} {
			page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{
				Flatten: true, TagIDs: []int64{slideTagID}, SlideTagOnlyOwn: onlyOwn,
			})
			if err != nil {
				t.Fatalf("页标浏览（onlyOwn=%v）: %v", onlyOwn, err)
			}
			set := hitKeySet(page)
			if page.Total != 1 || !contains(set, model.AssetRefTypeSlide, "乙.pptx", 1) {
				t.Errorf("页自有标签应仅命中乙第 1 页（onlyOwn=%v），实际 total=%d set=%v", onlyOwn, page.Total, set)
			}
		}
	})
	t.Run("混合检索_默认合并", func(t *testing.T) {
		// 关键词全页命中 + 文件标签「重点」：只剩甲的 2 页
		page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{fileTagID}})
		if err != nil {
			t.Fatalf("混合检索: %v", err)
		}
		set := hitKeySet(page)
		if page.Total != 2 || !contains(set, model.AssetRefTypeSlide, "甲.pptx", 1) ||
			!contains(set, model.AssetRefTypeSlide, "甲.pptx", 2) {
			t.Errorf("混合检索默认合并应命中甲 2 页，实际 total=%d set=%v", page.Total, set)
		}
	})
	t.Run("混合检索_OnlyOwn", func(t *testing.T) {
		// 「重点」+ OnlyOwn：甲页全过滤（关键词命中不受开关影响的前提是被筛选
		// 语义放行——此处标签筛选语义本身过滤全部页，属预期）；「页标」+ OnlyOwn：
		// 仅乙第 1 页
		page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
			Flatten: true, TagIDs: []int64{fileTagID}, SlideTagOnlyOwn: true,
		})
		if err != nil {
			t.Fatalf("混合检索 OnlyOwn: %v", err)
		}
		if page.Total != 0 {
			t.Errorf("OnlyOwn 下无页自有标签命中应空结果，实际 total=%d", page.Total)
		}
		page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
			Flatten: true, TagIDs: []int64{slideTagID}, SlideTagOnlyOwn: true,
		})
		if err != nil {
			t.Fatalf("混合检索页标 OnlyOwn: %v", err)
		}
		set := hitKeySet(page)
		if page.Total != 1 || !contains(set, model.AssetRefTypeSlide, "乙.pptx", 1) {
			t.Errorf("混合检索 OnlyOwn 应仅命中乙第 1 页，实际 total=%d set=%v", page.Total, set)
		}
		if hit := page.Items[0]; hit.Note != "" || hit.Rating != 0 {
			t.Errorf("乙第 1 页仅打标签未写备注，条目备注/星级应为零值：%+v", hit)
		}
	})
	t.Run("检索条目携带页自有标注", func(t *testing.T) {
		// 给乙第 1 页补备注/星级后，检索幻灯片条目应携带（详情面板数据源）；
		// 文件条目页自有标注字段恒零值（文件自有标注经 ListAssetFiles 携带）
		mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["乙.pptx"], 1, model.AssetSlideMetaUpdate{
			Rating: 5, Note: "页备注", TagNames: []string{"页标"},
		})
		page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
		if err != nil {
			t.Fatalf("全量检索: %v", err)
		}
		var slideHit *model.AssetSearchHit
		for i := range page.Items {
			if page.Items[i].RefType == model.AssetRefTypeSlide &&
				page.Items[i].FilePath == "乙.pptx" && page.Items[i].SlideNum == 1 {
				slideHit = &page.Items[i]
			}
		}
		if slideHit == nil {
			t.Fatalf("检索结果缺目标条目（乙第 1 页）")
		}
		if slideHit.Rating != 5 || slideHit.Note != "页备注" || len(slideHit.OwnTags) != 1 || slideHit.OwnTags[0].Name != "页标" {
			t.Errorf("幻灯片条目应携带页自有标注，实际：%+v", slideHit)
		}
		// Tags 字段维持继承语义不变：乙未打文件标签 → 继承为空（页自有标签不混入）
		if len(slideHit.Tags) != 0 {
			t.Errorf("幻灯片条目 Tags 应维持继承语义（乙未打文件标签应为空），实际：%+v", slideHit.Tags)
		}

		// 文件条目（关键词命中文件名）：页自有标注字段恒零值
		pageFile, err := svc.SearchAssets(lib.ID, "甲", model.AssetSearchOptions{Flatten: true})
		if err != nil {
			t.Fatalf("文件名检索: %v", err)
		}
		var fileHit *model.AssetSearchHit
		for i := range pageFile.Items {
			if pageFile.Items[i].RefType == model.AssetRefTypeFile && pageFile.Items[i].FilePath == "甲.pptx" {
				fileHit = &pageFile.Items[i]
			}
		}
		if fileHit == nil {
			t.Fatalf("文件名检索缺甲文件条目，实际：%+v", pageFile.Items)
		}
		if len(fileHit.OwnTags) != 0 || fileHit.Note != "" || fileHit.Rating != 0 {
			t.Errorf("文件条目页自有标注字段应恒零值：%+v", fileHit)
		}
		// 甲打了文件标签「重点」：Tags 继承语义在文件条目上即自身标签
		if len(fileHit.Tags) != 1 || fileHit.Tags[0].Name != "重点" {
			t.Errorf("文件条目 Tags 应为自身文件标签，实际：%+v", fileHit.Tags)
		}
	})
}

// ---------- 重建/重解析保留链 ----------

// TestRebuild_PreservesSlideAnnotations 重建保留链：页级标注（标签/业务备注/
// 星级）按 path+slide_num 重绑保留；页消失（重写后页数减少）的标注静默丢弃；
// 残留行零残留（级联 + 显式清理后重绑为唯一来源）。
func TestRebuild_PreservesSlideAnnotations(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重建标注库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{
		{titlePh: "甲一", body: []string{"内容"}},
		{titlePh: "甲二", body: []string{"内容"}},
	})
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{
		{titlePh: "乙一", body: []string{"内容"}},
		{titlePh: "乙二", body: []string{"内容"}},
		{titlePh: "乙三", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idsByPath := make(map[string]int64, len(files))
	for _, f := range files {
		idsByPath[f.Path] = f.ID
	}
	if err := svc.UpdateFileMeta(lib.ID, idsByPath["甲.pptx"], model.AssetFileMetaUpdate{Tags: []string{"文标"}}); err != nil {
		t.Fatalf("文件打标签: %v", err)
	}
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["甲.pptx"], 1, model.AssetSlideMetaUpdate{
		Rating: 4, Note: "甲一备注", TagNames: []string{"页标A"},
	})
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["乙.pptx"], 2, model.AssetSlideMetaUpdate{
		Rating: 2, Note: "乙二备注", TagNames: []string{"页标B"},
	})
	// 乙第 3 页标注：重写后页消失，应静默丢弃
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["乙.pptx"], 3, model.AssetSlideMetaUpdate{
		Rating: 5, Note: "将丢弃", TagNames: []string{"页标C"},
	})

	// 重写乙.pptx 为 2 页（重建全量重扫以磁盘为准，无 mtime 时序依赖）
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{
		{titlePh: "乙一", body: []string{"内容"}},
		{titlePh: "乙二", body: []string{"内容"}},
	})
	if _, err := svc.RebuildScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}

	entries := mustListSlides(t, svc, lib.ID)
	kept := slideEntryByNum(t, entries, "甲.pptx", 1)
	if kept.Rating != 4 || kept.Note != "甲一备注" || len(kept.OwnTags) != 1 || kept.OwnTags[0].Name != "页标A" {
		t.Errorf("甲第 1 页标注应按 path+slide_num 重绑保留，实际：%+v", kept)
	}
	kept = slideEntryByNum(t, entries, "乙.pptx", 2)
	if kept.Rating != 2 || kept.Note != "乙二备注" || len(kept.OwnTags) != 1 || kept.OwnTags[0].Name != "页标B" {
		t.Errorf("乙第 2 页标注应重绑保留，实际：%+v", kept)
	}
	dropped := slideEntryByNum(t, entries, "乙.pptx", 1)
	if dropped.Rating != 0 || dropped.Note != "" || len(dropped.OwnTags) != 0 {
		t.Errorf("未标注页应保持零值，实际：%+v", dropped)
	}
	for _, e := range entries {
		if e.FilePath == "乙.pptx" && e.SlideNum == 3 {
			t.Fatal("乙第 3 页应随重写消失，不应存在条目")
		}
	}

	// 文件标签照常保留；页级标注残留行零残留（丢弃页的行不得以幽灵形态存活）
	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("重建后 ListFiles: %v", err)
	}
	for _, f := range files {
		if f.Path == "甲.pptx" && (len(f.Tags) != 1 || f.Tags[0].Name != "文标") {
			t.Errorf("重建后文件标签应保留，实际：%+v", f.Tags)
		}
	}
	store := svc.openStoreForTest(t, lib.ID)
	if n := countSlideAnnotRows(t, store, "slide_tags"); n != 2 {
		t.Errorf("重建后 slide_tags 应恰 2 行（页标A+页标B），实际 %d 行", n)
	}
	if n := countSlideAnnotRows(t, store, "slide_meta"); n != 2 {
		t.Errorf("重建后 slide_meta 应恰 2 行，实际 %d 行", n)
	}
}

// TestRebindAfterRebuild_PartialRescan 重绑机制单测（rebuildLibrary 重绑 defer 的
// 核心语义，失败路径部分重绑优于全丢的基础）：构造「已清索引 + 仅重扫 a.pptx」
// 状态后直调两重绑函数（签名不变，接备份 map，与 rebuildLibrary defer 内同参
// 调用）——已重扫文件（a.pptx）的文件标签与页级标注按 path(+页序) 恢复，
// 未重扫文件（b.pptx）的备份条目因无 files/slides 行可匹配自然丢弃。端到端
// defer 接线见 TestRebuildScan_MidScanCancelRebindsScannedFiles。
func TestRebindAfterRebuild_PartialRescan(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "部分重绑库", dir)
	buildAssetsPptx(t, dir, "a.pptx", []assetsSlideSpec{
		{titlePh: "A1", body: []string{"内容"}},
		{titlePh: "A2", body: []string{"内容"}},
	})
	buildAssetsPptx(t, dir, "b.pptx", []assetsSlideSpec{
		{titlePh: "B1", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)
	store := svc.openStoreForTest(t, lib.ID)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idsByPath := make(map[string]int64, len(files))
	for _, f := range files {
		idsByPath[f.Path] = f.ID
	}
	if err := svc.UpdateFileMeta(lib.ID, idsByPath["a.pptx"], model.AssetFileMetaUpdate{Tags: []string{"文标"}}); err != nil {
		t.Fatalf("文件打标签: %v", err)
	}
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["a.pptx"], 1, model.AssetSlideMetaUpdate{
		Rating: 4, Note: "A1 备注", TagNames: []string{"页标A"},
	})
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["b.pptx"], 1, model.AssetSlideMetaUpdate{
		Rating: 2, Note: "B1 备注", TagNames: []string{"页标B"},
	})

	// 备份 → 清索引（与 rebuildLibrary 前半段一致的重构状态）
	bindings, err := store.fileTagBindingsByPath()
	if err != nil {
		t.Fatalf("备份文件标签绑定: %v", err)
	}
	annots, err := store.slideAnnotationsByPath()
	if err != nil {
		t.Fatalf("备份页级标注: %v", err)
	}
	if len(annots) != 2 {
		t.Fatalf("页标注备份应含两文件条目，实际 %d 个", len(annots))
	}
	if err := store.clearIndexForRebuild(); err != nil {
		t.Fatalf("清空索引: %v", err)
	}

	// 部分重扫：仅重灌 a.pptx（两页），b.pptx 保持缺席（模拟中途失败时已扫文件）
	const now = "2026-01-01 00:00:00"
	partialFileID, err := store.insertProcessingFile("a.pptx", "a", 128, 64, now)
	if err != nil {
		t.Fatalf("插占位记录: %v", err)
	}
	if err := store.replaceFileSlides(partialFileID, 128, 64, "hash-a", now, []parsedSlide{
		{Num: 1, Title: "A1", Text: "A1 内容"},
		{Num: 2, Title: "A2", Text: "A2 内容"},
	}); err != nil {
		t.Fatalf("重灌 a.pptx: %v", err)
	}

	fileRebound, fileHashRebound, err := store.rebindFileTagsAfterRebuild(bindings)
	if err != nil {
		t.Fatalf("重绑文件标签: %v", err)
	}
	if fileRebound != 1 || fileHashRebound != 0 {
		t.Errorf("文件标签应仅恢复已重扫的 a.pptx（b 无行可匹配丢弃，无哈希兜底命中），实际 path=%d hash=%d", fileRebound, fileHashRebound)
	}
	slideRebound, slideHashRebound, err := store.rebindSlideAnnotationsAfterRebuild(annots)
	if err != nil {
		t.Fatalf("重绑页级标注: %v", err)
	}
	if slideRebound != 1 || slideHashRebound != 0 {
		t.Errorf("页级标注应仅恢复 a.pptx 第 1 页（b 标注随文件缺席丢弃），实际 path=%d hash=%d", slideRebound, slideHashRebound)
	}

	// 已重扫文件：标注与文件标签恢复
	kept := slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "a.pptx", 1)
	if kept.Rating != 4 || kept.Note != "A1 备注" || len(kept.OwnTags) != 1 || kept.OwnTags[0].Name != "页标A" {
		t.Errorf("已重扫页标注应恢复，实际：%+v", kept)
	}
	restFiles, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("重绑后 ListFiles: %v", err)
	}
	if len(restFiles) != 1 || restFiles[0].Path != "a.pptx" {
		t.Fatalf("索引应仅剩已重扫的 a.pptx，实际：%+v", restFiles)
	}
	if len(restFiles[0].Tags) != 1 || restFiles[0].Tags[0].Name != "文标" {
		t.Errorf("已重扫文件标签应恢复，实际：%+v", restFiles[0].Tags)
	}
	// 未重扫文件：标注零残留（丢弃即彻底丢弃，无幽灵行）
	if n := countSlideAnnotRows(t, store, "slide_tags"); n != 1 {
		t.Errorf("slide_tags 应恰 1 行（未扫文件标注零残留），实际 %d 行", n)
	}
	if n := countSlideAnnotRows(t, store, "slide_meta"); n != 1 {
		t.Errorf("slide_meta 应恰 1 行（未扫文件标注零残留），实际 %d 行", n)
	}
}

// TestRebuildScan_MidScanCancelRebindsScannedFiles 重建中途失败的判别性用例
// （defer 接线端到端）：解析 fake 在首个文件（rel 字典序 a < b）解析内取消扫描
// 父 ctx——备份与清库无 ctx 检查点先完成，a 落库后 b 的文件间检查点命中取消，
// scanLibrary 确定性中途返回错误（无 sleep 无时序依赖）。断言失败路径重绑已由
// defer 执行：已重扫的 a 恢复文件标签与页级标注；未重扫的 b 不在索引、标注
// 丢弃。修复前重绑仅挂成功路径，本用例下 a 的标注将永久丢失。
func TestRebuildScan_MidScanCancelRebindsScannedFiles(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重建中断库", dir)
	buildAssetsPptx(t, dir, "a.pptx", []assetsSlideSpec{
		{titlePh: "A1", body: []string{"内容"}},
		{titlePh: "A2", body: []string{"内容"}},
	})
	buildAssetsPptx(t, dir, "b.pptx", []assetsSlideSpec{
		{titlePh: "B1", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idsByPath := make(map[string]int64, len(files))
	for _, f := range files {
		idsByPath[f.Path] = f.ID
	}
	if err := svc.UpdateFileMeta(lib.ID, idsByPath["a.pptx"], model.AssetFileMetaUpdate{Tags: []string{"文标"}}); err != nil {
		t.Fatalf("文件打标签: %v", err)
	}
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["a.pptx"], 1, model.AssetSlideMetaUpdate{
		Rating: 4, Note: "A1 备注", TagNames: []string{"页标A"},
	})
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["b.pptx"], 1, model.AssetSlideMetaUpdate{
		Rating: 2, Note: "B1 备注", TagNames: []string{"页标B"},
	})

	// 确定性中途取消构造：父 ctx 在 a 的解析内取消，向 scanCtx 同步传播
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.parseFn = func(srcPath, baseName string) ([]parsedSlide, error) {
		if baseName == "a" {
			cancel()
		}
		return parsePptxSlides(srcPath, baseName)
	}
	_, err = svc.RebuildScan(ctx, lib.ID)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
		t.Fatalf("重建应经中途取消失败并映射 E_ASSETS_CANCELLED，实际：%v", err)
	}

	// 失败路径 defer 重绑已执行：已重扫的 a 恢复，未重扫的 b 缺席且标注丢弃
	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("重建失败后 ListFiles: %v", err)
	}
	if len(files) != 1 || files[0].Path != "a.pptx" {
		t.Fatalf("失败路径下索引应仅剩已重扫的 a.pptx，实际：%+v", files)
	}
	if len(files[0].Tags) != 1 || files[0].Tags[0].Name != "文标" {
		t.Errorf("已重扫文件标签应在失败路径经 defer 重绑恢复，实际：%+v", files[0].Tags)
	}
	kept := slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "a.pptx", 1)
	if kept.Rating != 4 || kept.Note != "A1 备注" || len(kept.OwnTags) != 1 || kept.OwnTags[0].Name != "页标A" {
		t.Errorf("已重扫页标注应在失败路径经 defer 重绑恢复，实际：%+v", kept)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if n := countSlideAnnotRows(t, store, "slide_tags"); n != 1 {
		t.Errorf("失败路径重绑后 slide_tags 应恰 1 行（b 标注随文件未重扫丢弃），实际 %d 行", n)
	}
	if n := countSlideAnnotRows(t, store, "slide_meta"); n != 1 {
		t.Errorf("失败路径重绑后 slide_meta 应恰 1 行，实际 %d 行", n)
	}
}

// TestReparse_SlideAnnotationRetention 增量重解析语义：页数不变标注原样保留；
// 页数减少失配标注清理（保留页原样）。重解析由注入陈旧 mtime 驱动（禁依赖
// NTFS mtime 时序）。
func TestReparse_SlideAnnotationRetention(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重解析标注库", dir)
	buildAssetsPptx(t, dir, "丙.pptx", []assetsSlideSpec{
		{titlePh: "丙一", body: []string{"内容"}},
		{titlePh: "丙二", body: []string{"内容"}},
		{titlePh: "丙三", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)
	store := svc.openStoreForTest(t, lib.ID)
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	fileID := files[0].ID
	mustUpdateSlideMeta(t, svc, lib.ID, fileID, 1, model.AssetSlideMetaUpdate{
		Rating: 3, Note: "丙一备注", TagNames: []string{"保留页标"},
	})
	mustUpdateSlideMeta(t, svc, lib.ID, fileID, 3, model.AssetSlideMetaUpdate{
		Note: "丙三备注",
	})

	// 页数不变重解析：标注原样保留（slides.id 全变但标注锚定 file_id+页码）
	if _, err := store.db.Exec(
		"UPDATE files SET mtime_ns = mtime_ns - 3600000000000 WHERE id = ?", fileID,
	); err != nil {
		t.Fatalf("注入陈旧 mtime: %v", err)
	}
	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("重扫: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("应触发重解析，实际：%+v", result)
	}
	kept := slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "丙.pptx", 1)
	if kept.Rating != 3 || kept.Note != "丙一备注" || len(kept.OwnTags) != 1 || kept.OwnTags[0].Name != "保留页标" {
		t.Errorf("页数不变重解析后标注应保留，实际：%+v", kept)
	}
	if e := slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "丙.pptx", 3); e.Note != "丙三备注" {
		t.Errorf("第 3 页标注应保留，实际：%+v", e)
	}

	// 页数减少重解析：失配页（第 3 页）标注清理，保留页原样
	buildAssetsPptx(t, dir, "丙.pptx", []assetsSlideSpec{
		{titlePh: "丙一", body: []string{"内容"}},
		{titlePh: "丙二", body: []string{"内容"}},
	})
	if _, err := store.db.Exec(
		"UPDATE files SET mtime_ns = mtime_ns - 3600000000000 WHERE id = ?", fileID,
	); err != nil {
		t.Fatalf("二次注入陈旧 mtime: %v", err)
	}
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("二次重扫: %v", err)
	}
	kept = slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "丙.pptx", 1)
	if kept.Rating != 3 || kept.Note != "丙一备注" || len(kept.OwnTags) != 1 {
		t.Errorf("页数减少后保留页标注应原样，实际：%+v", kept)
	}
	if n := countSlideAnnotRows(t, store, "slide_meta"); n != 1 {
		t.Errorf("失配页标注应清理（slide_meta 恰 1 行），实际 %d 行", n)
	}
	if n := countSlideAnnotRows(t, store, "slide_tags"); n != 1 {
		t.Errorf("失配页标签应清理（slide_tags 恰 1 行），实际 %d 行", n)
	}
}

// TestUpdateSlideMeta_NoSideEffect 保存页标注无解析链副作用：files 行
// （status/hash/mtime_ns/updated_at）不变、FTS 行数不变、缩略图记账不变
// （纯标注层约束——不触发重解析与缩略图失效）。
func TestUpdateSlideMeta_NoSideEffect(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "无副作用库", dir)
	buildAssetsPptx(t, dir, "丁.pptx", []assetsSlideSpec{
		{titlePh: "丁一", body: []string{"城商行内容"}},
	})
	mustScanIndex(t, svc, lib)
	store := svc.openStoreForTest(t, lib.ID)
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	fileID := files[0].ID

	var status, hash, updatedAt string
	var mtimeNs int64
	if err := store.db.QueryRow(
		"SELECT status, hash, mtime_ns, updated_at FROM files WHERE id = ?", fileID,
	).Scan(&status, &hash, &mtimeNs, &updatedAt); err != nil {
		t.Fatalf("读文件行: %v", err)
	}
	ftsBefore, err := store.countFtsRows()
	if err != nil {
		t.Fatalf("countFtsRows: %v", err)
	}
	thumbsBefore := countThumbCacheRows(t, store)

	mustUpdateSlideMeta(t, svc, lib.ID, fileID, 1, model.AssetSlideMetaUpdate{
		Rating: 5, Note: "新备注", TagNames: []string{"新页标"},
	})

	var status2, hash2, updatedAt2 string
	var mtimeNs2 int64
	if err := store.db.QueryRow(
		"SELECT status, hash, mtime_ns, updated_at FROM files WHERE id = ?", fileID,
	).Scan(&status2, &hash2, &mtimeNs2, &updatedAt2); err != nil {
		t.Fatalf("二次读文件行: %v", err)
	}
	if status2 != status || hash2 != hash || mtimeNs2 != mtimeNs || updatedAt2 != updatedAt {
		t.Errorf("保存页标注不应触碰 files 行：status %q→%q hash %q→%q mtime %d→%d updated %q→%q",
			status, status2, hash, hash2, mtimeNs, mtimeNs2, updatedAt, updatedAt2)
	}
	ftsAfter, err := store.countFtsRows()
	if err != nil {
		t.Fatalf("二次 countFtsRows: %v", err)
	}
	if ftsAfter != ftsBefore {
		t.Errorf("保存页标注不应动 FTS（页业务备注不入检索），%d→%d", ftsBefore, ftsAfter)
	}
	if n := countThumbCacheRows(t, store); n != thumbsBefore {
		t.Errorf("保存页标注不应动缩略图记账，%d→%d", thumbsBefore, n)
	}
}
