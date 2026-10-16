package service

import (
	"errors"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- 批量评分（素材管理第八批，SRS §2.6 P0） ----------
//
// 覆盖面：文件条目/页条目各写正确表（files.rating / slide_meta.rating）、页业务
// 备注保留、0 = 清除、部分失败隔离（不存在 fileID/slideNum 记 failed 明细继续）、
// rating 越界/空目标/库不存在参数拒绝、评分后星级 facet 命中联动。
// fixture 走真实扫描链路（buildAssetsPptx + StartScan），无 sleep/mtime 依赖。

// slideMetaRatingByNum 按 ListSlides 回读页级星级与备注（条目 Rating/Note 快照）。
func slideMetaRatingByNum(t *testing.T, svc *AssetLibraryService, libraryID string) map[int]model.AssetSlideEntry {
	t.Helper()
	slides := mustListSlides(t, svc, libraryID)
	byNum := make(map[int]model.AssetSlideEntry, len(slides))
	for _, s := range slides {
		byNum[s.SlideNum] = s
	}
	return byNum
}

// mustSetSlideMeta 页级标注预置辅助（整体替换语义，初始态设置用）。
func mustSetSlideMeta(t *testing.T, svc *AssetLibraryService, libraryID string, fileID int64, slideNum int, meta model.AssetSlideMetaUpdate) {
	t.Helper()
	if err := svc.UpdateSlideMeta(libraryID, fileID, slideNum, meta); err != nil {
		t.Fatalf("UpdateSlideMeta(%d, %d): %v", fileID, slideNum, err)
	}
}

// TestAssetBatchRating_MixedTargets 混合目标：文件条目写 files.rating、页条目写
// slide_meta.rating（页业务备注保留），明细带后端拼装定位 Label，文件标签互不影响。
func TestAssetBatchRating_MixedTargets(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "批量评分库", dir)
	buildAssetsPptx(t, dir, "方案甲.pptx", []assetsSlideSpec{
		{titlePh: "首页标题", body: []string{"首页正文"}},
		{titlePh: "次页标题", body: []string{"次页正文"}},
	})
	buildAssetsPptx(t, dir, "方案乙.pptx", []assetsSlideSpec{
		{titlePh: "乙页标题", body: []string{"乙页正文"}},
	})
	mustScanIndex(t, svc, lib)

	fileA := assetFileIDByPath(t, svc, lib.ID, "方案甲.pptx")
	fileB := assetFileIDByPath(t, svc, lib.ID, "方案乙.pptx")
	// 预置页业务备注与页级标签：批量评分只改 rating，备注必须保留（纯标注层边界）
	mustSetSlideMeta(t, svc, lib.ID, fileA, 2, model.AssetSlideMetaUpdate{Note: "获客方案核心页", Rating: 1})

	result, err := svc.BatchSetRating(lib.ID, []model.AssetRatingTarget{
		{FileID: fileA},              // 文件条目
		{FileID: fileA, SlideNum: 2}, // 页条目（有备注）
		{FileID: fileB, SlideNum: 1}, // 页条目（无既有 slide_meta 行）
	}, 5)
	if err != nil {
		t.Fatalf("BatchSetRating: %v", err)
	}
	if result.Total != 3 || result.Succeeded != 3 || result.Failed != 0 {
		t.Fatalf("汇总不符：%+v", result)
	}
	if result.Items[0].Label != "方案甲.pptx" {
		t.Errorf("文件条目 Label 应为相对路径，实际 %q", result.Items[0].Label)
	}
	if result.Items[1].Label != "方案甲.pptx · 第 2 页" {
		t.Errorf("页条目 Label 应为「路径 · 第 N 页」，实际 %q", result.Items[1].Label)
	}

	// 文件条目落 files.rating
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		want := 5
		if f.Path == "方案乙.pptx" {
			want = 0
		}
		if f.Rating != want {
			t.Errorf("文件 %s rating 应为 %d，实际 %d", f.Path, want, f.Rating)
		}
	}

	// 页条目落 slide_meta.rating，业务备注保留；无既有行的页不被误写
	byNum := slideMetaRatingByNum(t, svc, lib.ID)
	s2 := byNum[2]
	if s2.Rating != 5 || s2.Note != "获客方案核心页" {
		t.Errorf("页 2 应 rating=5 且备注保留，实际 rating=%d note=%q", s2.Rating, s2.Note)
	}
	if byNum[1].Rating != 0 {
		t.Errorf("方案甲未评分页应保持 rating=0，实际 %d", byNum[1].Rating)
	}
	for _, s := range byNum {
		if s.FileID == fileB && s.Rating != 0 {
			t.Errorf("方案乙页不应被写入，实际 rating=%d", s.Rating)
		}
	}
}

// TestAssetBatchRating_ClearAndFacetHit 0 = 清除评分（files.rating 与
// slide_meta.rating 置 0，备注仍保留）；评分后星级 facet 命中联动（检索路径实时
// 读 rating，无独立失效链路）。
func TestAssetBatchRating_ClearAndFacetHit(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "清除评分库", dir)
	buildAssetsPptx(t, dir, "方案甲.pptx", []assetsSlideSpec{
		{titlePh: "首页标题", body: []string{"首页正文"}},
	})
	mustScanIndex(t, svc, lib)

	fileA := assetFileIDByPath(t, svc, lib.ID, "方案甲.pptx")
	mustSetSlideMeta(t, svc, lib.ID, fileA, 1, model.AssetSlideMetaUpdate{Note: "重点页", Rating: 3})

	// 先评 5 星，确认 facet 星级命中联动
	if _, err := svc.BatchSetRating(lib.ID, []model.AssetRatingTarget{{FileID: fileA}, {FileID: fileA, SlideNum: 1}}, 5); err != nil {
		t.Fatalf("BatchSetRating(5): %v", err)
	}
	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Ratings: []int64{5}})
	if err != nil {
		t.Fatalf("SearchAssets(5星): %v", err)
	}
	if page.Total != 1 {
		t.Errorf("5 星 facet 应命中文件条目，实际 total=%d", page.Total)
	}

	// 0 = 清除：files.rating 与 slide_meta.rating 归 0，页备注保留
	result, err := svc.BatchSetRating(lib.ID, []model.AssetRatingTarget{{FileID: fileA}, {FileID: fileA, SlideNum: 1}}, 0)
	if err != nil {
		t.Fatalf("BatchSetRating(0): %v", err)
	}
	if result.Succeeded != 2 {
		t.Fatalf("清除应全部成功：%+v", result)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if files[0].Rating != 0 {
		t.Errorf("清除后文件 rating 应为 0，实际 %d", files[0].Rating)
	}
	byNum := slideMetaRatingByNum(t, svc, lib.ID)
	if byNum[1].Rating != 0 || byNum[1].Note != "重点页" {
		t.Errorf("清除后页应 rating=0 且备注保留，实际 rating=%d note=%q", byNum[1].Rating, byNum[1].Note)
	}
	// facet 命中联动：清除后 5 星零命中、0（未评分）命中
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Ratings: []int64{5}})
	if err != nil {
		t.Fatalf("SearchAssets(5星) 清除后: %v", err)
	}
	if page.Total != 0 {
		t.Errorf("清除后 5 星 facet 应零命中，实际 total=%d", page.Total)
	}
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Ratings: []int64{0}})
	if err != nil {
		t.Fatalf("SearchAssets(0星): %v", err)
	}
	if page.Total != 1 {
		t.Errorf("清除后未评分档应命中文件条目，实际 total=%d", page.Total)
	}
}

// TestAssetBatchRating_PartialFailure 部分失败隔离：不存在 fileID 与不存在页记
// failed 明细继续，成功条目已写入不回滚；失败明细携带 (fileId, slideNum) 供前端
// slideNum||0 反查契约。
func TestAssetBatchRating_PartialFailure(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "评分隔离库", dir)
	buildAssetsPptx(t, dir, "方案甲.pptx", []assetsSlideSpec{
		{titlePh: "首页标题", body: []string{"首页正文"}},
	})
	mustScanIndex(t, svc, lib)
	fileA := assetFileIDByPath(t, svc, lib.ID, "方案甲.pptx")

	result, err := svc.BatchSetRating(lib.ID, []model.AssetRatingTarget{
		{FileID: 99999},              // 不存在文件
		{FileID: fileA},              // 成功
		{FileID: fileA, SlideNum: 9}, // 不存在页
	}, 4)
	if err != nil {
		t.Fatalf("部分失败不应整体报错: %v", err)
	}
	if result.Total != 3 || result.Succeeded != 1 || result.Failed != 2 {
		t.Fatalf("汇总不符：%+v", result)
	}
	if result.Items[0].Status != model.AssetBatchTagStatusFailed ||
		!strings.Contains(result.Items[0].Message, "文件不存在") {
		t.Errorf("首条应为「文件不存在」failed 明细：%+v", result.Items[0])
	}
	if result.Items[2].Status != model.AssetBatchTagStatusFailed ||
		!strings.Contains(result.Items[2].Message, "页不存在") || result.Items[2].SlideNum != 9 {
		t.Errorf("第 3 条应为「页不存在」failed 明细：%+v", result.Items[2])
	}
	if result.Items[1].Status != model.AssetBatchTagStatusOK {
		t.Errorf("次条应成功：%+v", result.Items[1])
	}
	// 成功条目已写入（不回滚）
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if files[0].Rating != 4 {
		t.Errorf("成功条目评分应已写入，实际 %d", files[0].Rating)
	}
}

// TestAssetBatchRating_ParamsRejected 参数校验：rating 越界（6 / -1）、空目标、
// 库不存在分别拒绝（库不存在走 E_ASSETS_LIB_NOT_FOUND 语义）。
func TestAssetBatchRating_ParamsRejected(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "评分参数库", dir)
	targets := []model.AssetRatingTarget{{FileID: 1}}

	if _, err := svc.BatchSetRating(lib.ID, targets, 6); err == nil || !strings.Contains(err.Error(), "0-5") {
		t.Errorf("rating 越上界应拒绝，实际 err=%v", err)
	}
	if _, err := svc.BatchSetRating(lib.ID, targets, -1); err == nil || !strings.Contains(err.Error(), "0-5") {
		t.Errorf("rating 越下界应拒绝，实际 err=%v", err)
	}
	if _, err := svc.BatchSetRating(lib.ID, nil, 3); err == nil || !strings.Contains(err.Error(), "请先选择") {
		t.Errorf("空目标应拒绝，实际 err=%v", err)
	}
	if _, err := svc.BatchSetRating("不存在的库", targets, 3); !errors.Is(err, errAssetLibraryNotFound) {
		t.Errorf("库不存在应返回 E_ASSETS_LIB_NOT_FOUND 语义，实际 err=%v", err)
	}
}
