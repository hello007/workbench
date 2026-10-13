package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- 检索链路测试（fixture 走真实扫描链路：buildAssetsPptx + StartScan） ----------

// mustScanIndex 扫描断言辅助（检索用例共享前置：建库由 mustCreateLibrary 承担，
// 扫描必须在 fixture 构造之后调用）。
func mustScanIndex(t *testing.T, svc *AssetLibraryService, lib model.AssetLibrary) {
	t.Helper()
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
}

// TestAssetSearch_HitsSlidesByText 中文词命中幻灯片页文本：词条目带类型标记、
// 归属文件溯源字段、页序与原文窗口摘要；未命中关键词返回空页不报错。
func TestAssetSearch_HitsSlidesByText(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "检索库", dir)
	// 文件名不含检索词（文件名命中是独立条目形态，由加权用例覆盖）
	buildAssetsPptx(t, dir, "渠道策略.pptx", []assetsSlideSpec{
		{titlePh: "获客策略总览", body: []string{"全面服务城商行客户数字化转型"}},
		{titlePh: "无关页", body: []string{"其他内容"}},
	})
	mustScanIndex(t, svc, lib)

	// 3 字词命中页文本
	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("SearchAssets: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("应命中 1 条幻灯片条目，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	hit := page.Items[0]
	if hit.RefType != model.AssetRefTypeSlide {
		t.Errorf("条目类型应为 slide，实际 %q", hit.RefType)
	}
	if hit.RefID <= 0 || hit.FileID <= 0 {
		t.Errorf("条目应携带记录 ID（slide=%d file=%d）", hit.RefID, hit.FileID)
	}
	if hit.FilePath != "渠道策略.pptx" {
		t.Errorf("溯源文件路径不符：%q", hit.FilePath)
	}
	if hit.SlideNum != 1 {
		t.Errorf("应命中第 1 页，实际 %d", hit.SlideNum)
	}
	if hit.Title != "获客策略总览" {
		t.Errorf("展示标题应为页标题，实际 %q", hit.Title)
	}
	if hit.HitCount != 1 {
		t.Errorf("平铺条目命中数应为 1，实际 %d", hit.HitCount)
	}
	if hit.Score == 0 {
		t.Error("关键词命中应携带非零 bm25 分值")
	}
	if hit.Snippet == "" || !strings.Contains(hit.Snippet, "城商行") {
		t.Errorf("命中摘要应含关键词原文，实际 %q", hit.Snippet)
	}
	if hit.Tags == nil {
		t.Error("Tags 应为非 nil 空切片（JSON 序列化为 []）")
	}

	// 未命中关键词：空结果不报错
	page, err = svc.SearchAssets(lib.ID, "不存在的词", model.AssetSearchOptions{})
	if err != nil {
		t.Fatalf("未命中检索不应报错: %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("未命中应返回空页，实际 total=%d", page.Total)
	}
	if page.Keyword != "不存在的词" {
		t.Errorf("关键词应原样回显，实际 %q", page.Keyword)
	}
}

// TestAssetSearch_TwoCharKeyword 2 字词命中（research：trigram 方案对 2 字词
// 静默 0 命中，本方案的核心验收形态）。
func TestAssetSearch_TwoCharKeyword(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "二字词库", dir)
	buildAssetsPptx(t, dir, "季度汇报.pptx", []assetsSlideSpec{
		{titlePh: "经营回顾", body: []string{"本季度获客成果显著"}},
	})
	mustScanIndex(t, svc, lib)

	page, err := svc.SearchAssets(lib.ID, "获客", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("SearchAssets: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("2 字词应命中 1 条，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	if page.Items[0].RefType != model.AssetRefTypeSlide || page.Items[0].SlideNum != 1 {
		t.Errorf("应命中第 1 页幻灯片，实际：%+v", page.Items[0])
	}
}

// TestAssetSearch_FileNameWeightOverTitle bm25 列加权排序：文件名命中（4.0）
// 排在幻灯片标题命中（3.0）之前（文件名 > 幻灯片标题 > 文本 > 备注）。
func TestAssetSearch_FileNameWeightOverTitle(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "加权库", dir)
	// 文件名含词，页内容不含 → 产生 file 条目
	buildAssetsPptx(t, dir, "城商行白皮书.pptx", []assetsSlideSpec{
		{titlePh: "概述", body: []string{"面向渠道条线的内容"}},
	})
	// 文件名不含词，页标题含词 → 产生 slide 条目
	buildAssetsPptx(t, dir, "渠道材料.pptx", []assetsSlideSpec{
		{titlePh: "城商行专题", body: []string{"章节内容"}},
	})
	mustScanIndex(t, svc, lib)

	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("SearchAssets: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("应命中 2 条（1 文件 + 1 幻灯片），实际 total=%d items=%d", page.Total, len(page.Items))
	}
	first := page.Items[0]
	if first.RefType != model.AssetRefTypeFile {
		t.Errorf("文件名命中（权重 4.0）应排在最前，实际首条类型 %q：%+v", first.RefType, first)
	}
	if first.FilePath != "城商行白皮书.pptx" || first.Title != "城商行白皮书" {
		t.Errorf("首条应为文件条目（含文件名与标题），实际：%+v", first)
	}
	second := page.Items[1]
	if second.RefType != model.AssetRefTypeSlide || second.SlideNum != 1 {
		t.Errorf("次条应为幻灯片标题命中，实际：%+v", second)
	}
	// file 条目 Score 应严格优于 slide 条目（bm25 负值口径：更小更相关）
	if !(first.Score < second.Score) {
		t.Errorf("文件名命中分值应更优（%v < %v）", first.Score, second.Score)
	}
}

// TestAssetSearch_AggregateAndFlatten 聚合折叠与平铺：同一 PPT 多页命中折叠为
// 聚合条目（HitCount = 命中页数、展示最高分页）；Flatten=true 平铺返回全部；
// 不同 PPT 各自独立聚合条目。
func TestAssetSearch_AggregateAndFlatten(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "聚合库", dir)
	// 页 1 关键词在标题（slide_title 权重 3.0），页 2 仅在正文（slide_text 权重
	// 2.0）——两页命中分数确定性分层，聚合条目必取页 1 为最高分页（平局时
	// bm25 对等文档的排序不保证稳定，fixture 须构造确定的分数差异）
	buildAssetsPptx(t, dir, "多页命中.pptx", []assetsSlideSpec{
		{titlePh: "城商行服务方案", body: []string{"具体举措说明"}},
		{titlePh: "实施路径", body: []string{"城商行落地细节"}},
		{titlePh: "无关页", body: []string{"其他内容"}},
	})
	buildAssetsPptx(t, dir, "另一份.pptx", []assetsSlideSpec{
		{titlePh: "单页", body: []string{"城商行合作案例"}},
	})
	mustScanIndex(t, svc, lib)

	// 聚合模式：两个 PPT → 2 个聚合条目（多页命中折叠 + 单页独立）
	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: false})
	if err != nil {
		t.Fatalf("SearchAssets 聚合模式: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("聚合模式应有 2 个条目，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	agg := page.Items[0]
	if agg.RefType != model.AssetRefTypeSlide || agg.HitCount != 2 {
		t.Errorf("多页 PPT 应折叠为 HitCount=2 的聚合条目，实际：%+v", agg)
	}
	if agg.SlideNum != 1 {
		t.Errorf("聚合条目应展示最高分页（第 1 页），实际 %d", agg.SlideNum)
	}
	if agg.RefID <= 0 {
		t.Error("聚合条目应携带展示页记录 ID")
	}

	// 平铺模式：3 页命中全部返回
	page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("SearchAssets 平铺模式: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("平铺模式应有 3 条命中，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	if page.Flattened != true {
		t.Error("平铺标志应回显 true")
	}
	// 平铺按相关性排序：标题命中页（slide_title 权重 3.0）严格优于正文命中页
	// （2.0），排首位。同权重正文命中页之间的次序受文档长度归一影响不作断言
	// （微型语料全命中时 FTS5 IDF 触及 1e-6 下限，分数差异仅来自权重与长度归一）。
	firstFlat := page.Items[0]
	if firstFlat.FilePath != "多页命中.pptx" || firstFlat.SlideNum != 1 {
		t.Errorf("平铺首位应为标题命中页（多页命中.pptx 第 1 页），实际：%+v", firstFlat)
	}
	for _, hit := range page.Items {
		if hit.HitCount != 1 {
			t.Errorf("平铺条目 HitCount 应为 1，实际 %d", hit.HitCount)
		}
	}
}

// TestAssetSearch_TagFilterCombination 标签筛选与关键词组合：文件条目按直接
// 绑定过滤，幻灯片条目按归属文件标签继承过滤（PRD D4）；换用其他标签则不命中。
func TestAssetSearch_TagFilterCombination(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "标签筛选库", dir)
	buildAssetsPptx(t, dir, "重点材料.pptx", []assetsSlideSpec{
		{titlePh: "首页", body: []string{"城商行专属方案"}},
	})
	buildAssetsPptx(t, dir, "普通材料.pptx", []assetsSlideSpec{
		{titlePh: "首页", body: []string{"城商行通用方案"}},
	})
	mustScanIndex(t, svc, lib)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	taggedPath := "重点材料.pptx"
	var taggedID int64
	for _, f := range files {
		if f.Path == taggedPath {
			taggedID = f.ID
		}
	}
	if taggedID == 0 {
		t.Fatalf("未找到待打标文件，实际：%+v", files)
	}
	if err := svc.UpdateFileMeta(lib.ID, taggedID, model.AssetFileMetaUpdate{
		Tags: []string{"重点"},
	}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}

	// 按标签「重点」筛选关键词命中：仅剩已打标文件及其页
	// （tagIDs 从库中反查：UpdateFileMeta 后重新列表取标签 ID）
	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("二次 ListFiles: %v", err)
	}
	var tagID int64
	for _, f := range files {
		if f.Path == taggedPath && len(f.Tags) > 0 {
			tagID = f.Tags[0].ID
		}
	}
	if tagID == 0 {
		t.Fatal("未取到标签 ID")
	}

	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
	if err != nil {
		t.Fatalf("组合检索: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("标签筛选后应命中 1 条（重点材料的页），实际 total=%d", page.Total)
	}
	hit := page.Items[0]
	if hit.FilePath != taggedPath {
		t.Errorf("筛选后应只剩重点材料的页，实际：%+v", hit)
	}
	if len(hit.Tags) != 1 || hit.Tags[0].Name != "重点" {
		t.Errorf("幻灯片条目应继承文件标签，实际：%+v", hit.Tags)
	}

	// 换无关标签：组合检索为空
	other := tagID + 999
	page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{other}})
	if err != nil {
		t.Fatalf("无关标签检索: %v", err)
	}
	if page.Total != 0 {
		t.Errorf("无关标签筛选应为空结果，实际 total=%d", page.Total)
	}
}

// TestAssetSearch_EmptyKeywordAndTagOnly 空查询语义：关键词与标签均为空返回
// 空页不报错；关键词为空且给定标签退化为纯标签浏览（文件 + 幻灯片条目，
// 幻灯片按 PPT 聚合折叠）。
func TestAssetSearch_EmptyKeywordAndTagOnly(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "空词库", dir)
	buildAssetsPptx(t, dir, "标签浏览.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if err := svc.UpdateFileMeta(lib.ID, files[0].ID, model.AssetFileMetaUpdate{Tags: []string{"素材"}}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	files, _ = svc.ListFiles(lib.ID)
	tagID := files[0].Tags[0].ID

	// 双空：空结果不报错
	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{})
	if err != nil {
		t.Fatalf("双空检索不应报错: %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("双空应返回空页，实际 total=%d", page.Total)
	}
	// 纯空白关键词同语义
	if page, err = svc.SearchAssets(lib.ID, "   ", model.AssetSearchOptions{}); err != nil || page.Total != 0 {
		t.Errorf("纯空白关键词应空结果不报错，实际 page=%+v err=%v", page, err)
	}

	// 纯标签浏览（聚合模式）：1 文件条目 + 1 聚合条目（2 页折叠）
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: false, TagIDs: []int64{tagID}})
	if err != nil {
		t.Fatalf("纯标签浏览: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("纯标签浏览应有 2 条（文件 + 聚合页），实际 total=%d items=%d", page.Total, len(page.Items))
	}
	if page.Items[0].RefType != model.AssetRefTypeFile {
		t.Errorf("同路径下文件条目应排最前，实际首条：%+v", page.Items[0])
	}
	agg := page.Items[1]
	if agg.RefType != model.AssetRefTypeSlide || agg.HitCount != 2 {
		t.Errorf("纯标签浏览幻灯片应折叠为 HitCount=2，实际：%+v", agg)
	}
	if agg.Score != 0 || agg.Snippet != "" {
		t.Errorf("纯标签浏览无相关性语义（score=0 snippet 空），实际 score=%v snippet=%q", agg.Score, agg.Snippet)
	}

	// 平铺模式：2 页全部返回
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{Flatten: true, TagIDs: []int64{tagID}})
	if err != nil {
		t.Fatalf("纯标签平铺: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("纯标签平铺应有 3 条（文件 + 2 页），实际 total=%d", page.Total)
	}
}

// TestAssetSearch_Pagination 分页边界：total 稳定不随页变化；offset 越界返回
// 空页；limit 归一化（≤0 取默认，超上限截断）。
func TestAssetSearch_Pagination(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "分页库", dir)
	// 3 个文件各 2 页命中（聚合模式 = 3 条目；平铺 = 6 条目）
	for _, name := range []string{"甲.pptx", "乙.pptx", "丙.pptx"} {
		buildAssetsPptx(t, dir, name, []assetsSlideSpec{
			{titlePh: "首页", body: []string{"城商行内容"}},
			{titlePh: "次页", body: []string{"城商行内容"}},
		})
	}
	mustScanIndex(t, svc, lib)

	// 平铺分页
	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, Limit: 4, Offset: 0})
	if err != nil {
		t.Fatalf("首页: %v", err)
	}
	if page.Total != 6 || len(page.Items) != 4 {
		t.Fatalf("首页应 total=6 items=4，实际 total=%d items=%d", page.Total, len(page.Items))
	}
	page2, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, Limit: 4, Offset: 4})
	if err != nil {
		t.Fatalf("次页: %v", err)
	}
	if page2.Total != 6 || len(page2.Items) != 2 {
		t.Fatalf("次页应 total=6 items=2，实际 total=%d items=%d", page2.Total, len(page2.Items))
	}
	// 翻页无重叠（按 RefID 判重）
	seen := make(map[int64]bool)
	for _, hit := range append(append([]model.AssetSearchHit{}, page.Items...), page2.Items...) {
		if seen[hit.RefID] {
			t.Errorf("翻页条目重叠（RefID=%d）", hit.RefID)
		}
		seen[hit.RefID] = true
	}
	// offset 越界：空页 total 保持
	page3, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, Limit: 4, Offset: 100})
	if err != nil {
		t.Fatalf("越界页: %v", err)
	}
	if page3.Total != 6 || len(page3.Items) != 0 {
		t.Errorf("越界页应 total=6 items=0，实际 total=%d items=%d", page3.Total, len(page3.Items))
	}
	// 聚合模式分页：3 个聚合条目
	pageAgg, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: false, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("聚合首页: %v", err)
	}
	if pageAgg.Total != 3 || len(pageAgg.Items) != 2 {
		t.Errorf("聚合分页应 total=3 items=2，实际 total=%d items=%d", pageAgg.Total, len(pageAgg.Items))
	}
	// limit 归一化：≤0 取默认（不报错），超上限截断
	if page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true, Limit: 0}); err != nil {
		t.Fatalf("limit=0: %v", err)
	}
	if len(page.Items) != 6 {
		t.Errorf("limit=0 应回退默认值返回全部 6 条，实际 %d", len(page.Items))
	}
}

// TestAssetSearch_FtsSyncOnDeleteAndReparse FTS 与主表同步（禁脏数据）：删除
// 文件后 FTS 行同步清零；变更重解析（页数变化）后 FTS 行数等于新页数 + 文件行。
// 重处理驱动用注入陈旧 mtime 实现，不依赖文件系统时序（docs/spec/test-stability.md）。
func TestAssetSearch_FtsSyncOnDeleteAndReparse(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同步库", dir)
	src := buildAssetsPptx(t, dir, "同步样本.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"城商行内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)
	store := svc.openStoreForTest(t, lib.ID)
	assertFtsRows(t, store, 1+2, "首次扫描后（1 文件行 + 2 页行）")

	// 变更重解析：覆盖写同路径 fixture（页数 2 → 3）+ 注入陈旧 mtime 强制重处理
	buildAssetsPptx(t, dir, "同步样本.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"城商行内容"}},
		{titlePh: "二", body: []string{"城商行内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	if _, err := store.db.Exec("UPDATE files SET mtime_ns = mtime_ns - 3600000000000"); err != nil {
		t.Fatalf("注入陈旧 mtime: %v", err)
	}
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("重扫: %v", err)
	}
	assertFtsRows(t, store, 1+3, "重解析后（1 文件行 + 3 页行，旧页行无残留）")
	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("重解析后检索: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("重解析后「城商行」应命中 2 页，实际 %d", page.Total)
	}

	// 删除文件：FTS 行同步清零
	if err := os.Remove(src); err != nil {
		t.Fatalf("删除源文件: %v", err)
	}
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("删除后重扫: %v", err)
	}
	assertFtsRows(t, store, 0, "删除文件后（全部清零）")
}

// assertFtsRows FTS 行数断言（同步完整性白盒断言）。
func assertFtsRows(t *testing.T, store *assetsStore, want int, scene string) {
	t.Helper()
	got, err := store.countFtsRows()
	if err != nil {
		t.Fatalf("%s：统计 FTS 行数: %v", scene, err)
	}
	if got != want {
		t.Errorf("%s：FTS 行数 = %d, want %d", scene, got, want)
	}
}

// TestAssetSearch_V1MigrationBackfill v1→v2 迁移回填：手工构造 v1 基线库
// （主表 + 存量数据 + user_version=1），openAssetsStore 触发 v2 迁移后断言
// 存量数据全部入 FTS 且可检索（ok 文件入文件行，failed 文件不入）。
func TestAssetSearch_V1MigrationBackfill(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v1-legacy.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("打开 v1 库: %v", err)
	}
	if _, err := db.Exec(assetsSchemaSQL); err != nil {
		t.Fatalf("建 v1 基线表: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatalf("写 v1 版本: %v", err)
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("执行 %s: %v", q, err)
		}
	}
	mustExec("INSERT INTO files(path, size, mtime_ns, status, title, note) VALUES(?, 1, 1, ?, ?, ?)",
		"存量材料.pptx", model.AssetFileStatusOK, "存量材料", "迁移前的用户备注")
	mustExec("INSERT INTO files(path, size, mtime_ns, status, title) VALUES(?, 1, 1, ?, ?)",
		"损坏文件.pptx", model.AssetFileStatusFailed, "损坏文件")
	mustExec("INSERT INTO slides(file_id, slide_num, title, text, speaker_notes) VALUES(1, 1, ?, ?, ?)",
		"迁移页", "存量页文本包含城商行关键词", "")
	if err := db.Close(); err != nil {
		t.Fatalf("关闭 v1 库: %v", err)
	}

	store, err := openAssetsStore(dbPath) // 触发 v1→v2 迁移 + 回填
	if err != nil {
		t.Fatalf("v1 库升级打开: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("读版本: %v", err)
	}
	if version != assetsSchemaVersion {
		t.Errorf("迁移后版本 = %d, want %d", version, assetsSchemaVersion)
	}
	// 回填口径：1 文件行（仅 ok）+ 1 页行 = 2（failed 文件不入检索索引）
	assertFtsRows(t, store, 2, "v2 回填后")
	// 存量数据可检索（写入查询同源：回填文本经 splitFtsText）
	hits, err := store.searchFts(buildFtsQuery("城商行"))
	if err != nil {
		t.Fatalf("存量检索: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("存量页应可检索命中 1 行，实际 %d", len(hits))
	}
}

// TestAssetSearch_RebuildIdempotent 手动全量重建：清理后重扫幂等（files/slides/
// FTS 记录数一致），文件标签按路径映射重绑保留，重建后检索可用；扫描锁与取消
// 注册表无残留。
func TestAssetSearch_RebuildIdempotent(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重建库", dir)
	buildAssetsPptx(t, dir, "重建样本.pptx", []assetsSlideSpec{
		{titlePh: "首页", body: []string{"城商行内容"}},
		{titlePh: "次页", body: []string{"城商行内容"}},
	})
	mustScanIndex(t, svc, lib)
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if err := svc.UpdateFileMeta(lib.ID, files[0].ID, model.AssetFileMetaUpdate{
		Rating: 3, Note: "重建保留备注", Tags: []string{"重点"},
	}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}

	snapshot := func() (filesN, slidesN, ftsN int) {
		t.Helper()
		stats, err := svc.GetLibraryStats(lib.ID)
		if err != nil {
			t.Fatalf("GetLibraryStats: %v", err)
		}
		store := svc.openStoreForTest(t, lib.ID)
		fts, err := store.countFtsRows()
		if err != nil {
			t.Fatalf("countFtsRows: %v", err)
		}
		return stats.FileCount, stats.SlideCount, fts
	}
	beforeFiles, beforeSlides, beforeFts := snapshot()

	// 重建（模拟索引脏态自愈：重建前注入一条 FTS 悬挂行，重建后必须消失）
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec(
		"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, slide_title) VALUES(99999, 'file', 99999, 99999, ?)",
		splitFtsText("脏数据"),
	); err != nil {
		t.Fatalf("注入脏数据: %v", err)
	}

	result, err := svc.RebuildScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}
	if result.Total != 1 || result.New != 1 {
		t.Errorf("重建应全量新增（total=1 new=1），实际：%+v", result)
	}
	afterFiles, afterSlides, afterFts := snapshot()
	if afterFiles != beforeFiles || afterSlides != beforeSlides || afterFts != beforeFts {
		t.Errorf("重建后记录数应一致：files %d→%d slides %d→%d fts %d→%d",
			beforeFiles, afterFiles, beforeSlides, afterSlides, beforeFts, afterFts)
	}

	// 标签按路径重绑保留；星级/备注属重建语义不保留（见 RebuildScan 契约注释）
	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("重建后 ListFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("重建后应 1 个文件，实际 %d", len(files))
	}
	if len(files[0].Tags) != 1 || files[0].Tags[0].Name != "重点" {
		t.Errorf("重建后文件标签应按路径重绑保留，实际：%+v", files[0].Tags)
	}

	// 再次重建：幂等
	if _, err := svc.RebuildScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("二次 RebuildScan: %v", err)
	}
	againFiles, againSlides, againFts := snapshot()
	if againFiles != beforeFiles || againSlides != beforeSlides || againFts != beforeFts {
		t.Errorf("二次重建应幂等：files=%d slides=%d fts=%d", againFiles, againSlides, againFts)
	}

	// 重建后检索可用 + 脏数据已清
	page, err := svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{Flatten: true})
	if err != nil {
		t.Fatalf("重建后检索: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("重建后「城商行」应命中 2 页，实际 %d", page.Total)
	}
	hits, err := store.searchFts(buildFtsQuery("脏数据"))
	if err != nil {
		t.Fatalf("脏数据检索: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("重建后脏数据应清除，实际命中 %d 行", len(hits))
	}
	// 注册表无残留
	if svc.CancelScan(lib.ID) {
		t.Error("重建结束后取消注册表应清空")
	}
}

// TestAssetSearch_NotFoundLibrary 检索不存在的库：沿用 E_ASSETS_LIB_NOT_FOUND
// 错误分流（与列表/扫描同口径）。
func TestAssetSearch_NotFoundLibrary(t *testing.T) {
	svc := newTestAssetService(t)
	_, err := svc.SearchAssets("不存在的库", "词", model.AssetSearchOptions{})
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsLibNotFound {
		t.Fatalf("不存在的库应报 E_ASSETS_LIB_NOT_FOUND，实际：%v", err)
	}
}
