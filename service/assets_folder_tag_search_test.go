package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"workbench/model"
)

// ---------- 文件夹标签级联检索（A9 冒烟反馈） ----------
//
// 文件有效标签 = 文件自身绑定 ∪ 归属文件夹（含全部后代目录，目录边界前缀）
// 的文件夹标签；幻灯片条目经归属文件继承同一有效标签集合。两消费面同源验证：
// 纯标签浏览 SQL 子查询（listTagFilteredFiles/listTagFilteredSlides）与关键词
// 路径 Go 层预取（fileIDsForTagFilter）。

// mustTagIDByName 按名反查标签 ID（文件夹/页级标注测试的取 ID 辅助）。
func mustTagIDByName(t *testing.T, svc *AssetLibraryService, libraryID, name string) int64 {
	t.Helper()
	store := svc.openStoreForTest(t, libraryID)
	var id int64
	if err := store.db.QueryRow("SELECT id FROM tags WHERE name = ?", name).Scan(&id); err != nil {
		t.Fatalf("反查标签 %q: %v", name, err)
	}
	return id
}

// mustHitPaths 检索结果条目路径集合（去重，文件条目与页条目同路径归并）。
func mustHitPaths(t *testing.T, svc *AssetLibraryService, libraryID, keyword string, opts model.AssetSearchOptions) map[string]int {
	t.Helper()
	page, err := svc.SearchAssets(libraryID, keyword, opts)
	if err != nil {
		t.Fatalf("SearchAssets: %v", err)
	}
	paths := make(map[string]int, len(page.Items))
	for _, hit := range page.Items {
		paths[hit.FilePath]++
	}
	return paths
}

// TestAssetSearch_FolderTagInheritance 文件夹标签级联主链路：标注目录直下与
// 全部后代目录的文件经继承命中（纯标签浏览 SQL 子查询 + 关键词路径 Go 层预取
// 两消费面同源）；未标注目录与无标签文件不命中；文件自身直接绑定与文件夹级联
// 继承并集生效；仅备注无标签的文件夹标注零行为变化（回归面）。
func TestAssetSearch_FolderTagInheritance(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "文件夹标签库", dir)
	for _, sub := range []string{"media", "media/sub", "plain", "mediax"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(sub)), 0755); err != nil {
			t.Fatalf("MkdirAll %s: %v", sub, err)
		}
	}
	specs := []assetsSlideSpec{{titlePh: "首页", body: []string{"城商行专属方案"}}}
	buildAssetsPptx(t, dir, "根直标.pptx", specs)
	buildAssetsPptx(t, dir, "根无标.pptx", specs)
	buildAssetsPptx(t, dir, "media/直下材料.pptx", specs)
	buildAssetsPptx(t, dir, "media/sub/后代材料.pptx", specs)
	buildAssetsPptx(t, dir, "plain/旁目录材料.pptx", specs)
	buildAssetsPptx(t, dir, "mediax/前缀兄弟材料.pptx", specs)
	mustScanIndex(t, svc, lib)

	store := svc.openStoreForTest(t, lib.ID)
	// media 标「重点」（级联源）；plain 仅备注无标签（零行为变化面）；
	// 根直标.pptx 直接绑定「重点」（直绑 ∪ 级联并集面）
	if err := store.updateFolderMeta("media", "", []string{"重点"}, "2026-10-22 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta(media): %v", err)
	}
	if err := store.updateFolderMeta("plain", "仅备注目录", nil, "2026-10-22 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta(plain): %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if f.Path == "根直标.pptx" {
			if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{Tags: []string{"重点"}}); err != nil {
				t.Fatalf("UpdateFileMeta: %v", err)
			}
		}
	}
	tagID := mustTagIDByName(t, svc, lib.ID, "重点")

	// 纯标签浏览路径（SQL 子查询）：直下 + 后代 + 直绑三处命中，根无标/旁目录不命中
	paths := mustHitPaths(t, svc, lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
	if len(paths) != 3 {
		t.Fatalf("纯标签浏览应命中 3 个文件，实际：%v", paths)
	}
	for _, want := range []string{"根直标.pptx", "media/直下材料.pptx", "media/sub/后代材料.pptx"} {
		if paths[want] != 2 { // 文件条目 + 页条目（单页 PPT）
			t.Errorf("文件 %s 应命中文件+页两条目，实际 %d：%v", want, paths[want], paths)
		}
	}
	for _, absent := range []string{"根无标.pptx", "plain/旁目录材料.pptx", "mediax/前缀兄弟材料.pptx"} {
		if paths[absent] != 0 {
			t.Errorf("文件 %s 不应命中，实际 %d：%v", absent, paths[absent], paths)
		}
	}

	// 关键词路径（Go 层预取）：同口径命中
	paths = mustHitPaths(t, svc, lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
	if len(paths) != 3 {
		t.Fatalf("关键词+标签组合应命中 3 个文件，实际：%v", paths)
	}
	for _, want := range []string{"根直标.pptx", "media/直下材料.pptx", "media/sub/后代材料.pptx"} {
		if paths[want] != 1 {
			t.Errorf("关键词路径文件 %s 应命中 1 页，实际 %d：%v", want, paths[want], paths)
		}
	}
	for _, absent := range []string{"根无标.pptx", "plain/旁目录材料.pptx", "mediax/前缀兄弟材料.pptx"} {
		if paths[absent] != 0 {
			t.Errorf("关键词路径文件 %s 不应命中，实际 %d：%v", absent, paths[absent], paths)
		}
	}
}

// TestAssetSearch_FolderTagSlideOnlyOwnInteraction SlideTagOnlyOwn 开关交互不
// 回归：OnlyOwn=true 仅页自身层生效——文件夹级联继承与文件直绑同被裁剪（页条目
// 不命中），页自身标签不受影响；OnlyOwn=false 恢复继承合并语义。两消费面同测。
func TestAssetSearch_FolderTagSlideOnlyOwnInteraction(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "仅页自身库", dir)
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	specs := []assetsSlideSpec{
		{titlePh: "带标页", body: []string{"城商行方案"}},
		{titlePh: "普通页", body: []string{"城商行细节"}},
	}
	buildAssetsPptx(t, dir, "media/开关材料.pptx", specs)
	mustScanIndex(t, svc, lib)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("应恰 1 个文件，实际：%+v", files)
	}
	if err := svc.UpdateSlideMeta(lib.ID, files[0].ID, 1, model.AssetSlideMetaUpdate{TagNames: []string{"次要"}}); err != nil {
		t.Fatalf("UpdateSlideMeta: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.updateFolderMeta("media", "", []string{"重点"}, "2026-10-22 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta: %v", err)
	}
	folderTagID := mustTagIDByName(t, svc, lib.ID, "重点")
	ownTagID := mustTagIDByName(t, svc, lib.ID, "次要")

	// 纯标签浏览：OnlyOwn=true + 文件夹标签 → 页条目不命中（级联继承被裁剪）；
	// 文件条目不受开关影响仍命中
	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{folderTagID}, SlideTagOnlyOwn: true})
	if err != nil {
		t.Fatalf("OnlyOwn 纯标签浏览: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].RefType != model.AssetRefTypeFile {
		t.Fatalf("OnlyOwn=true 文件夹标签应仅剩文件条目（页条目裁剪），实际 total=%d items=%+v", page.Total, page.Items)
	}
	// OnlyOwn=true + 页自身标签 → 第 1 页命中（页自身层不受影响）
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{ownTagID}, SlideTagOnlyOwn: true})
	if err != nil {
		t.Fatalf("OnlyOwn 页自身标签浏览: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].SlideNum != 1 {
		t.Fatalf("OnlyOwn=true 页自身标签应命中第 1 页，实际 total=%d items=%+v", page.Total, page.Items)
	}
	// OnlyOwn=false + 文件夹标签 → 文件条目 + 两页（继承合并语义恢复）
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{folderTagID}})
	if err != nil {
		t.Fatalf("默认纯标签浏览: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("OnlyOwn=false 文件夹标签应命中文件+2 页，实际 total=%d items=%+v", page.Total, page.Items)
	}

	// 关键词路径：OnlyOwn=true + 文件夹标签 → 两页关键词命中均被裁剪
	paths := mustHitPaths(t, svc, lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{folderTagID}, SlideTagOnlyOwn: true})
	if len(paths) != 0 {
		t.Errorf("关键词路径 OnlyOwn=true 文件夹标签应零页命中，实际：%v", paths)
	}
	// OnlyOwn=false → 两页命中
	paths = mustHitPaths(t, svc, lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{folderTagID}})
	if len(paths) != 1 || paths["media/开关材料.pptx"] != 2 {
		t.Errorf("关键词路径 OnlyOwn=false 应命中 2 页，实际：%v", paths)
	}
}

// TestAssetSearch_FolderTagWildcardDirName 通配符目录名准确性：folder_path 为
// 存量数据列无法逐行预转义 LIKE 通配符，继承谓词用 substr 等值比较——含 % 的
// 目录名只匹配字面路径，不误伤同构形目录（LIKE 前缀方案会误判 a%b → axb）。
func TestAssetSearch_FolderTagWildcardDirName(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "通配目录库", dir)
	for _, sub := range []string{"a%b", "axb", "a_b"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatalf("MkdirAll %s: %v", sub, err)
		}
	}
	specs := []assetsSlideSpec{{titlePh: "首页", body: []string{"城商行方案"}}}
	buildAssetsPptx(t, dir, "a%b/百分号目录.pptx", specs)
	buildAssetsPptx(t, dir, "axb/同构目录.pptx", specs)
	buildAssetsPptx(t, dir, "a_b/下划线目录.pptx", specs)
	mustScanIndex(t, svc, lib)

	store := svc.openStoreForTest(t, lib.ID)
	if err := store.updateFolderMeta("a%b", "", []string{"特殊"}, "2026-10-22 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta(a%%b): %v", err)
	}
	tagID := mustTagIDByName(t, svc, lib.ID, "特殊")

	for _, keyword := range []string{"", "城商行"} {
		paths := mustHitPaths(t, svc, lib.ID, keyword, model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
		if len(paths) != 1 || paths["a%b/百分号目录.pptx"] == 0 {
			t.Errorf("keyword=%q 含 %% 目录名应仅命中字面目录下文件，实际：%v", keyword, paths)
		}
	}
}

// TestAssetSearch_FolderTagExcludedNotRevived 检索可见性口径不变：excluded 文件
// 不因文件夹标签级联命中而复活——关键词路径经 excludedFileIDSet 剔除、纯标签
// 浏览路径经 SQL status='ok' 过滤，两消费面同验。
func TestAssetSearch_FolderTagExcludedNotRevived(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "排除复活库", dir)
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	specs := []assetsSlideSpec{{titlePh: "首页", body: []string{"城商行方案"}}}
	buildAssetsPptx(t, dir, "media/被排除.pptx", specs)
	buildAssetsPptx(t, dir, "media/正常件.pptx", specs)
	mustScanIndex(t, svc, lib)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	var excludedID int64
	for _, f := range files {
		if f.Path == "media/被排除.pptx" {
			excludedID = f.ID
		}
	}
	if excludedID == 0 {
		t.Fatalf("未找到待排除文件，实际：%+v", files)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.markFileExcluded(excludedID, time.Now().Format("2006-01-02 15:04:05")); err != nil {
		t.Fatalf("markFileExcluded: %v", err)
	}
	if err := store.updateFolderMeta("media", "", []string{"重点"}, "2026-10-22 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta: %v", err)
	}
	tagID := mustTagIDByName(t, svc, lib.ID, "重点")

	// 关键词路径：excluded 文件 FTS 行保留但不因文件夹标签命中复活
	paths := mustHitPaths(t, svc, lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
	if len(paths) != 1 || paths["media/正常件.pptx"] != 1 {
		t.Errorf("关键词路径 excluded 文件不应命中，实际：%v", paths)
	}
	// 纯标签浏览路径：status='ok' 过滤同口径
	paths = mustHitPaths(t, svc, lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
	if len(paths) != 1 || paths["media/正常件.pptx"] != 2 {
		t.Errorf("纯标签路径 excluded 文件不应命中（正常件应文件+页两条），实际：%v", paths)
	}
}

// TestAssetSearch_FolderTagNoTagsZeroBehaviorChange 无文件夹标签时零行为变化
// （回归面）：folder_tags 空表或标签无文件夹绑定时，标签筛选结果与既有文件/页
// 绑定口径完全一致——继承谓词的 JOIN/EXISTS 空集不产生任何额外命中。
func TestAssetSearch_FolderTagNoTagsZeroBehaviorChange(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "零变化库", dir)
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	specs := []assetsSlideSpec{{titlePh: "首页", body: []string{"城商行方案"}}}
	buildAssetsPptx(t, dir, "media/已绑件.pptx", specs)
	buildAssetsPptx(t, dir, "media/未绑件.pptx", specs)
	mustScanIndex(t, svc, lib)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if f.Path == "media/已绑件.pptx" {
			if err := svc.UpdateFileMeta(lib.ID, f.ID, model.AssetFileMetaUpdate{Tags: []string{"重点"}}); err != nil {
				t.Fatalf("UpdateFileMeta: %v", err)
			}
		}
	}
	tagID := mustTagIDByName(t, svc, lib.ID, "重点")

	// folder_tags 空表：仅文件直绑命中，media 下未绑文件不吃任何级联
	for _, keyword := range []string{"", "城商行"} {
		paths := mustHitPaths(t, svc, lib.ID, keyword, model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
		if len(paths) != 1 || paths["media/已绑件.pptx"] == 0 {
			t.Errorf("keyword=%q 无文件夹标签时应仅文件直绑命中，实际：%v", keyword, paths)
		}
	}

	// 同库补一条指向其他目录的文件夹标签（tag 无该目录绑定变化不影响本标签筛选：
	// 换无关目录打标后原筛选结果不变）
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.updateFolderMeta("media", "", []string{"归档"}, "2026-10-22 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta: %v", err)
	}
	archiveTagID := mustTagIDByName(t, svc, lib.ID, "归档")
	paths := mustHitPaths(t, svc, lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{archiveTagID}})
	// media 打标「归档」后其下文件经级联命中（级联链路自身成立）；「重点」筛选
	// 结果不受 folder_tags 新增行影响
	if len(paths) != 2 {
		t.Errorf("「归档」级联应命中 media 下 2 个文件，实际：%v", paths)
	}
	paths = mustHitPaths(t, svc, lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
	if len(paths) != 1 || paths["media/已绑件.pptx"] == 0 {
		t.Errorf("新增无关文件夹标签后「重点」筛选应保持仅直绑命中，实际：%v", paths)
	}
}
