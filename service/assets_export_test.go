package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- 元数据导出（素材管理第九批 PR1，SRS §2.6.4 / 附录A） ----------
//
// 覆盖面：CSV 转义矩阵（中文/逗号/引号/换行/反斜杠路径）与 BOM 言、YAML 反斜杠
// 转 /、页行/文件行粒度与字段层级（note/rating/tags 来源）、三种范围行集（选中/
// 检索重放全集/全库）、覆盖拒绝与 overwrite 放行、流式分批（超单批 500 文件）与
// 检索重放分页（超单页 200 命中）、参数拒绝、缩略图与拆分产物路径探测。
// 数据构造两路：受控内容直接 SQL 插入（转义矩阵不经 pptx 解析链）；检索范围
// 用真实扫描链路（buildAssetsPptx + mustScanIndex，FTS 双写齐全）。无 sleep/
// mtime 时序依赖。

// insertExportFile 直接插入 ok 状态文件行（导出面受控数据构造，返回 fileID）。
// 不经 pptx 解析链，FTS 行不写（导出非检索路径不消费 FTS）。
func insertExportFile(t *testing.T, store *assetsStore, relPath, title, note string, rating int) int64 {
	t.Helper()
	res, err := store.db.Exec(
		"INSERT INTO files(path, size, mtime_ns, status, title, note, rating, created_at, updated_at) "+
			"VALUES(?, 1024, 0, 'ok', ?, ?, ?, '2026-10-16 10:00:00', '2026-10-16 10:00:00')",
		relPath, title, note, rating)
	if err != nil {
		t.Fatalf("插入导出 fixture 文件（%s）: %v", relPath, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("读取 fileID: %v", err)
	}
	return id
}

// insertExportSlide 直接插入页行（受控内容：标题/文本/备注含特殊字符）。
func insertExportSlide(t *testing.T, store *assetsStore, fileID int64, num int, title, text, notes string) {
	t.Helper()
	if _, err := store.db.Exec(
		"INSERT INTO slides(file_id, slide_num, title, text, speaker_notes) VALUES(?, ?, ?, ?, ?)",
		fileID, num, title, text, notes); err != nil {
		t.Fatalf("插入导出 fixture 页（%d）: %v", num, err)
	}
}

// mustExport 执行导出并断言成功。
func mustExport(t *testing.T, svc *AssetLibraryService, libraryID string, opts model.AssetExportOptions) model.AssetExportResult {
	t.Helper()
	result, err := svc.ExportAssetMetadata(context.Background(), libraryID, opts)
	if err != nil {
		t.Fatalf("ExportAssetMetadata: %v", err)
	}
	return result
}

// readExportCSV 读回 CSV 文件并解析为行集（跳过 BOM 与表头，返回数据行）。
func readExportCSV(t *testing.T, path string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 CSV: %v", err)
	}
	if len(raw) < 3 || raw[0] != 0xEF || raw[1] != 0xBB || raw[2] != 0xBF {
		t.Fatalf("CSV 须以 UTF-8 BOM 开头，实际前 3 字节：%v", raw[:minInt(3, len(raw))])
	}
	reader := csv.NewReader(bytes.NewReader(raw[3:]))
	reader.FieldsPerRecord = len(assetExportCSVHeader)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("解析 CSV（转义非法）: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("CSV 为空")
	}
	if got, want := strings.Join(records[0], ","), strings.Join(assetExportCSVHeader, ","); got != want {
		t.Fatalf("表头不符：got=%q want=%q", got, want)
	}
	return records[1:]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// csvRowsByKey 数据行索引（key = 「slide_num:源文件基名」→ 行；第一列为口径化
// source 路径，absolute 口径含目录前缀，取基名定位与口径解耦）。
func csvRowsByKey(t *testing.T, rows [][]string) map[string][]string {
	t.Helper()
	out := make(map[string][]string, len(rows))
	for _, r := range rows {
		key := r[1] + ":" + filepath.Base(r[0])
		if _, dup := out[key]; dup {
			t.Fatalf("行键重复（排序/去重破坏）: %s", key)
		}
		out[key] = r
	}
	return out
}

// TestAssetExport_CSVEscapeMatrixAndBOM 转义矩阵：中文/逗号/双引号/换行经
// encoding/csv 全字段引号保护后可无损读回；反斜杠路径（absolute 口径）原样
// 出现在字段值中（CSV 引号包裹天然转义）；ocr_text/summary 空列存在。
func TestAssetExport_CSVEscapeMatrixAndBOM(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出转义库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	fileID := insertExportFile(t, store, "方案/城商行.pptx",
		`文,件"标题"`, "备注,含\"引号\"\n与换行", 4)
	insertExportSlide(t, store, fileID, 1,
		`页"一"，甲`, "文本含,逗号、\"引号\"\n与换行符\\反斜杠", `备注，乙"丙"`)

	csvPath := filepath.Join(t.TempDir(), "导出,矩阵.csv")
	mustExport(t, svc, lib.ID, model.AssetExportOptions{
		CSVPath:  csvPath,
		Formats:  []string{model.AssetExportFormatCSV},
		PathMode: model.AssetExportPathModeAbsolute,
	})
	rows := readExportCSV(t, csvPath)
	if len(rows) != 2 {
		t.Fatalf("应导出文件行+页行共 2 行，实际 %d 行", len(rows))
	}
	byKey := csvRowsByKey(t, rows)
	fileRow := byKey["0:城商行.pptx"]
	slideRow := byKey["1:城商行.pptx"]
	if fileRow == nil || slideRow == nil {
		t.Fatalf("行集缺失：%v", rows)
	}
	// absolute 口径：source_file_path = rootPath 拼接（Windows 反斜杠原样保留，
	// 引号字段保护下无损）
	if want := filepath.Join(lib.RootPath, "方案", "城商行.pptx"); fileRow[0] != want {
		t.Errorf("文件行 source_file_path 应为绝对路径 %q，实际 %q", want, fileRow[0])
	}
	if !strings.Contains(fileRow[0], string(os.PathSeparator)) {
		t.Errorf("absolute 口径应含本地路径分隔符（反斜杠场景），实际 %q", fileRow[0])
	}
	// 字段值无损读回（引号/逗号/换行转义往返一致）
	if fileRow[2] != `文,件"标题"` {
		t.Errorf("文件标题转义往返不符：%q", fileRow[2])
	}
	if fileRow[9] != "备注,含\"引号\"\n与换行" {
		t.Errorf("文件备注转义往返不符：%q", fileRow[9])
	}
	if slideRow[3] != "文本含,逗号、\"引号\"\n与换行符\\反斜杠" {
		t.Errorf("页文本转义往返不符：%q", slideRow[3])
	}
	if slideRow[4] != `备注，乙"丙"` {
		t.Errorf("演讲者备注转义往返不符：%q", slideRow[4])
	}
	// 保留列恒空（ocr_text=5 / summary=8）
	if fileRow[5] != "" || fileRow[8] != "" || slideRow[5] != "" || slideRow[8] != "" {
		t.Errorf("ocr_text/summary 保留列应恒空：%v / %v", fileRow, slideRow)
	}
	// 11 列固定序
	if len(slideRow) != 11 {
		t.Errorf("应 11 列，实际 %d", len(slideRow))
	}
}

// TestAssetExport_MarkdownFrontmatterAndFilenames MD 卡：frontmatter 路径反斜杠
// 统一转 /（absolute 口径 Windows 路径无反斜杠残留）、tags 继承合并排序、rating
// 页级、卡名 `<3位序号>-<标题清洗>.md`（非法字符清洗）、正文段落。
func TestAssetExport_MarkdownFrontmatterAndFilenames(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出MD库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	fileID := insertExportFile(t, store, "方案/方案甲.pptx", "方案甲标题", "文件级备注", 3)
	insertExportSlide(t, store, fileID, 1, "首页标题", "首页正文", "")
	insertExportSlide(t, store, fileID, 2, `页: *二/b?`, "次页正文", "演讲者备注甲")
	if err := svc.UpdateFileMeta(lib.ID, fileID, model.AssetFileMetaUpdate{
		Rating: 3, Note: "文件级备注", Tags: []string{"城商行", "方案"},
	}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	if err := svc.UpdateSlideMeta(lib.ID, fileID, 2, model.AssetSlideMetaUpdate{
		Note: "页级备注", Rating: 5, TagNames: []string{"获客"},
	}); err != nil {
		t.Fatalf("UpdateSlideMeta: %v", err)
	}

	mdDir := filepath.Join(t.TempDir(), "cards", "nested")
	mustExport(t, svc, lib.ID, model.AssetExportOptions{
		MDDir:   mdDir,
		Formats: []string{model.AssetExportFormatMD},
	})
	entries, err := os.ReadDir(mdDir)
	if err != nil {
		t.Fatalf("读取导出目录: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("应 3 张卡（文件行+2 页行），实际 %d", len(entries))
	}
	// 卡名清洗：非法字符 ： * / ? → '-'，序号 3 位
	wantNames := []string{"001-方案甲标题.md", "002-首页标题.md", "003-页- -二-b-.md"}
	for i, name := range wantNames {
		if entries[i].Name() != name {
			t.Errorf("第 %d 张卡名应为 %q，实际 %q", i+1, name, entries[i].Name())
		}
	}

	// 页 2 卡：frontmatter 合并标签排序 + 页级 rating/note + 演讲者备注段
	raw, err := os.ReadFile(filepath.Join(mdDir, wantNames[2]))
	if err != nil {
		t.Fatalf("读取页卡: %v", err)
	}
	content := string(raw)
	for _, want := range []string{
		`source_file_path: "方案/方案甲.pptx"`,
		"slide_num: 2",
		`slide_title: "页: *二/b?"`,
		`tags: ["城商行", "方案", "获客"]`,
		"rating: 5",
		"# 页: *二/b?",
		"次页正文",
		"## 演讲者备注",
		"演讲者备注甲",
		"## 备注",
		"页级备注",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("页卡缺 %q，实际：\n%s", want, content)
		}
	}
	// 文件行卡：slide_num=0、文件级备注、无正文文本段
	raw, err = os.ReadFile(filepath.Join(mdDir, wantNames[0]))
	if err != nil {
		t.Fatalf("读取文件卡: %v", err)
	}
	content = string(raw)
	if !strings.Contains(content, "slide_num: 0") || !strings.Contains(content, "文件级备注") {
		t.Errorf("文件卡 slide_num/备注不符：\n%s", content)
	}
	if strings.Contains(content, "首页正文") {
		t.Errorf("文件卡不应携带页文本：\n%s", content)
	}

	// absolute 口径：YAML 路径反斜杠全部转 /（frontmatter 行无反斜杠残留）
	mdDirAbs := filepath.Join(t.TempDir(), "abs-cards")
	mustExport(t, svc, lib.ID, model.AssetExportOptions{
		MDDir:    mdDirAbs,
		Formats:  []string{model.AssetExportFormatMD},
		PathMode: model.AssetExportPathModeAbsolute,
	})
	raw, err = os.ReadFile(filepath.Join(mdDirAbs, "001-方案甲标题.md"))
	if err != nil {
		t.Fatalf("读取绝对口径文件卡: %v", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "source_file_path:") {
			if strings.Contains(line, "\\") {
				t.Errorf("YAML 路径应统一为 / 口径（无反斜杠残留），实际 %q", line)
			}
			want := `source_file_path: "` + filepath.ToSlash(filepath.Join(lib.RootPath, "方案", "方案甲.pptx")) + `"`
			if strings.TrimRight(line, "\r") != want {
				t.Errorf("YAML 绝对路径不符：\n got=%q\nwant=%q", strings.TrimRight(line, "\r"), want)
			}
		}
	}
}

// TestAssetExport_RowGranularityAndFieldHierarchy 行粒度与字段层级：文件行
// slide_num=0 取文件级 note/rating/tags；页行取页级 slide_meta（无行则空/0）、
// tags = 页自有 ∪ 归属文件合并去重排序；排序 path + slide_num 稳定。
func TestAssetExport_RowGranularityAndFieldHierarchy(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出粒度库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	// 方案甲（2 页）：文件级 4 星/文件备注/文件标签；页 2 页级 5 星/页备注/页自有标签
	fileA := insertExportFile(t, store, "方案/方案甲.pptx", "方案甲标题", "文件备注甲", 4)
	insertExportSlide(t, store, fileA, 1, "甲一", "甲一正文", "甲一演讲")
	insertExportSlide(t, store, fileA, 2, "甲二", "甲二正文", "")
	if err := svc.UpdateFileMeta(lib.ID, fileA, model.AssetFileMetaUpdate{
		Rating: 4, Note: "文件备注甲", Tags: []string{"城商行", "方案"},
	}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	if err := svc.UpdateSlideMeta(lib.ID, fileA, 2, model.AssetSlideMetaUpdate{
		Note: "页备注甲二", Rating: 5, TagNames: []string{"获客", "方案"},
	}); err != nil {
		t.Fatalf("UpdateSlideMeta: %v", err)
	}
	// 方案乙（1 页）：无任何标注；页 1 无 slide_meta 行
	fileB := insertExportFile(t, store, "获客/方案乙.pptx", "方案乙标题", "", 0)
	insertExportSlide(t, store, fileB, 1, "乙一", "乙一正文", "乙一演讲")

	csvPath := filepath.Join(t.TempDir(), "out.csv")
	mustExport(t, svc, lib.ID, model.AssetExportOptions{
		CSVPath: csvPath, Formats: []string{model.AssetExportFormatCSV},
	})
	rows := readExportCSV(t, csvPath)
	if len(rows) != 5 {
		t.Fatalf("全库应 5 行（2 文件 + 3 页），实际 %d", len(rows))
	}
	// 排序：方案/方案甲.pptx（文件行0→页1→页2）→ 获客/方案乙.pptx（文件行0→页1）
	wantOrder := []string{
		"0:方案/方案甲.pptx", "1:方案/方案甲.pptx", "2:方案/方案甲.pptx",
		"0:获客/方案乙.pptx", "1:获客/方案乙.pptx",
	}
	for i, key := range wantOrder {
		if got := rows[i][1] + ":" + rows[i][0]; got != key {
			t.Errorf("第 %d 行应为 %s，实际 %s", i+1, key, got)
		}
	}
	byKey := csvRowsByKey(t, rows)

	fileRow := byKey["0:方案甲.pptx"]
	if fileRow[2] != "方案甲标题" || fileRow[9] != "文件备注甲" || fileRow[10] != "" {
		t.Errorf("文件行标题/备注/拆分路径不符：%v", fileRow)
	}
	if fileRow[7] != "城商行;方案" {
		t.Errorf("文件行 tags 应为文件标签：%q", fileRow[7])
	}
	// 页 2：tags = 页自有 ∪ 文件 合并去重排序（获客/方案/城商行 → 城商行;方案;获客）；
	// note 取页级 slide_meta（rating 不在 CSV 11 列内，仅在 MD frontmatter，另测）
	slide2 := byKey["2:方案甲.pptx"]
	if slide2[7] != "城商行;方案;获客" {
		t.Errorf("页行 tags 应为继承合并去重排序：%q", slide2[7])
	}
	if slide2[9] != "页备注甲二" {
		t.Errorf("页行应取页级备注：%q", slide2[9])
	}
	// 页 1：无 slide_meta 行 → note 空；文本/演讲者备注来自 slides
	slide1 := byKey["1:方案甲.pptx"]
	if slide1[9] != "" || slide1[3] != "甲一正文" || slide1[4] != "甲一演讲" {
		t.Errorf("无标注页行兜底不符：%v", slide1)
	}
	// 文件行无文本/演讲者备注语义（恒空），thumbnail/single_pptx 恒空（未生成/未拆分）
	if fileRow[3] != "" || fileRow[4] != "" || fileRow[6] != "" {
		t.Errorf("文件行 text/speakerNotes/thumbnail 应为空：%v", fileRow)
	}
}

// TestAssetExport_ThreeScopes 三范围行集（D2）：选中（文件/页/混合，优先级高于
// 检索）、检索命中后端重放全集（关键词命中文件行+页行）、全库兜底；排序档单独
// 存在不构成检索语境（全库不缩窄）。
func TestAssetExport_ThreeScopes(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出范围库", dir)
	buildAssetsPptx(t, dir, "城商行方案.pptx", []assetsSlideSpec{
		{titlePh: "城商行方案标题", body: []string{"城商行方案正文"}},
		{titlePh: "城商行附录", body: []string{"附录内容"}},
	})
	buildAssetsPptx(t, dir, "获客活动.pptx", []assetsSlideSpec{
		{titlePh: "获客活动标题", body: []string{"获客活动正文"}},
	})
	mustScanIndex(t, svc, lib)
	fileA := assetFileIDByPath(t, svc, lib.ID, "城商行方案.pptx")
	fileB := assetFileIDByPath(t, svc, lib.ID, "获客活动.pptx")

	count := func(opts model.AssetExportOptions) int {
		t.Helper()
		opts.CSVPath = filepath.Join(t.TempDir(), "scope.csv")
		opts.Formats = []string{model.AssetExportFormatCSV}
		return mustExport(t, svc, lib.ID, opts).Rows
	}
	keys := func(opts model.AssetExportOptions) []string {
		t.Helper()
		opts.CSVPath = filepath.Join(t.TempDir(), "keys.csv")
		opts.Formats = []string{model.AssetExportFormatCSV}
		mustExport(t, svc, lib.ID, opts)
		rows := readExportCSV(t, opts.CSVPath)
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r[1]+":"+r[0])
		}
		return out
	}

	// 全库：2 文件 + 3 页 = 5 行
	if got := count(model.AssetExportOptions{}); got != 5 {
		t.Errorf("全库应 5 行，实际 %d", got)
	}
	// 排序档单独存在不构成检索语境（全库不缩窄）
	if got := count(model.AssetExportOptions{Search: model.AssetSearchOptions{SortBy: model.AssetSortByMtime}}); got != 5 {
		t.Errorf("仅排序档应仍全库 5 行，实际 %d", got)
	}
	// 选中范围 = 选中条目集与行集一一对应（与 collectSelectionTargets 一致，预览
	// N 精确）：文件条目出文件行、页条目出页行，不做「文件展开全部页」——展开会
	// 使预览数失真且与命中范围语义不对称
	if got := keys(model.AssetExportOptions{FileIDs: []int64{fileA}}); len(got) != 1 || got[0] != "0:城商行方案.pptx" {
		t.Errorf("选中文件应仅 1 行文件行，实际 %v", got)
	}
	// 选中页：仅该页行
	if got := keys(model.AssetExportOptions{SlideTargets: []model.AssetSlideTarget{{FileID: fileB, SlideNum: 1}}}); len(got) != 1 || got[0] != "1:获客活动.pptx" {
		t.Errorf("选中页应仅 1 行页行，实际 %v", got)
	}
	// 混合选中：文件行集 + 页行集并集
	got := keys(model.AssetExportOptions{
		FileIDs:      []int64{fileA},
		SlideTargets: []model.AssetSlideTarget{{FileID: fileB, SlideNum: 1}},
	})
	if len(got) != 2 {
		t.Errorf("混合选中应 2 行（文件行+页行），实际 %v", got)
	}
	// 陈旧 ID 静默忽略（库内实存行自然过滤）
	if got := count(model.AssetExportOptions{FileIDs: []int64{99999}}); got != 0 {
		t.Errorf("陈旧选中应 0 行，实际 %d", got)
	}
	// 检索命中重放全集：关键词「获客」命中 获客活动 文件行 + 其页行（城商行不命中）
	got = keys(model.AssetExportOptions{Keyword: "获客"})
	if len(got) != 2 || got[0] != "0:获客活动.pptx" || got[1] != "1:获客活动.pptx" {
		t.Errorf("检索范围应命中文件行+页行 2 行，实际 %v", got)
	}
	// 选中优先于检索：选中与关键词同时给走选中
	if got := count(model.AssetExportOptions{FileIDs: []int64{fileB}, Keyword: "城商行"}); got != 1 {
		t.Errorf("选中应优先于检索（获客文件行 1 行），实际 %d", got)
	}
	// 标签检索语境（纯标签路径重放）：给 fileA 打标签后按标签导出
	if err := svc.UpdateFileMeta(lib.ID, fileA, model.AssetFileMetaUpdate{Tags: []string{"重点客户"}}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	tags, err := svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	var tagID int64
	for _, tg := range tags {
		if tg.Name == "重点客户" {
			tagID = tg.ID
		}
	}
	if tagID == 0 {
		t.Fatalf("标签未创建")
	}
	got = keys(model.AssetExportOptions{Search: model.AssetSearchOptions{TagIDs: []int64{tagID}}})
	if len(got) != 3 || got[0] != "0:城商行方案.pptx" {
		t.Errorf("标签检索范围应命中城商行 3 行，实际 %v", got)
	}
}

// TestAssetExport_OverwritePolicy 覆盖策略：CSV 已存在 / md 目录非空拒绝（确认型
// AppError 码），overwrite=true 放行；md 覆盖保留目录内其他内容（同名卡覆盖写、
// 不清理陌生文件）；md 目录不存在直接创建不拒。
func TestAssetExport_OverwritePolicy(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出覆盖库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	fileID := insertExportFile(t, store, "方案甲.pptx", "方案甲标题", "", 0)
	insertExportSlide(t, store, fileID, 1, "甲一", "甲一正文", "")

	csvPath := filepath.Join(t.TempDir(), "out.csv")
	opts := model.AssetExportOptions{CSVPath: csvPath, Formats: []string{model.AssetExportFormatCSV}}
	mustExport(t, svc, lib.ID, opts)
	// 二次不带 overwrite：确认型拒绝
	_, err := svc.ExportAssetMetadata(context.Background(), lib.ID, opts)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsExportTargetExists {
		t.Fatalf("目标已存在应返回确认型 AppError（E_ASSETS_EXPORT_TARGET_EXISTS），实际 %v", err)
	}
	if !strings.Contains(err.Error(), csvPath) {
		t.Errorf("拒绝消息应含目标路径：%v", err)
	}
	// overwrite=true 放行且内容完整
	result := mustExport(t, svc, lib.ID, model.AssetExportOptions{
		CSVPath: csvPath, Formats: []string{model.AssetExportFormatCSV}, Overwrite: true,
	})
	if result.Rows != 2 {
		t.Errorf("覆盖导出应 2 行，实际 %d", result.Rows)
	}

	// md 目录非空：拒绝 → overwrite 放行且陌生文件保留
	mdDir := filepath.Join(t.TempDir(), "cards")
	stranger := filepath.Join(mdDir, "用户自建.md")
	if err := os.MkdirAll(mdDir, 0o755); err != nil {
		t.Fatalf("预建目录: %v", err)
	}
	if err := os.WriteFile(stranger, []byte("keep"), 0o644); err != nil {
		t.Fatalf("预置陌生文件: %v", err)
	}
	mdOpts := model.AssetExportOptions{MDDir: mdDir, Formats: []string{model.AssetExportFormatMD}}
	_, err = svc.ExportAssetMetadata(context.Background(), lib.ID, mdOpts)
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsExportTargetExists {
		t.Fatalf("md 目录非空应确认型拒绝，实际 %v", err)
	}
	mustExport(t, svc, lib.ID, model.AssetExportOptions{
		MDDir: mdDir, Formats: []string{model.AssetExportFormatMD}, Overwrite: true,
	})
	if _, err := os.Stat(stranger); err != nil {
		t.Errorf("覆盖导出不应清理目录内陌生文件: %v", err)
	}
	// md 目录不存在：直接创建导出（不触发确认）
	mustExport(t, svc, lib.ID, model.AssetExportOptions{
		MDDir: filepath.Join(t.TempDir(), "fresh", "cards"), Formats: []string{model.AssetExportFormatMD},
	})
}

// TestAssetExport_StreamBatchingBeyondOneBatch 流式分批：超单批（500）文件数
// 分页读出无遗漏无重复（LIMIT/OFFSET 循环终止正确），行集合不整体驻留的批处理
// 口径下全库导出行数完整。
func TestAssetExport_StreamBatchingBeyondOneBatch(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出流式库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	const totalFiles = 502 // > assetsQueryIDBatch(500)：至少两批
	for i := 1; i <= totalFiles; i++ {
		rel := fmt.Sprintf("批/方案%03d.pptx", i)
		fileID := insertExportFile(t, store, rel, "标题"+strconv.Itoa(i), "", 0)
		insertExportSlide(t, store, fileID, 1, "页"+strconv.Itoa(i), "正文", "")
	}

	csvPath := filepath.Join(t.TempDir(), "batch.csv")
	result := mustExport(t, svc, lib.ID, model.AssetExportOptions{
		CSVPath: csvPath, Formats: []string{model.AssetExportFormatCSV},
	})
	if want := totalFiles * 2; result.Rows != want {
		t.Fatalf("流式分批导出应 %d 行，实际 %d", want, result.Rows)
	}
	rows := readExportCSV(t, csvPath)
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		key := r[1] + ":" + r[0]
		if seen[key] {
			t.Fatalf("分批导出出现重复行：%s", key)
		}
		seen[key] = true
	}
	if len(seen) != totalFiles*2 {
		t.Fatalf("去重后应 %d 行，实际 %d", totalFiles*2, len(seen))
	}
}

// TestAssetExport_HydrateMissSkipsStaleKeys 水合 miss 跳过（P1 物化快照口径守护）：
// 行键物化后、水合会话前的删除窗口（增量重扫改写 / 清理丢失记录）对应行按 miss
// 跳过并计数——files 行删除 → 该文件全部行键（文件行+页行）跳过；slides 行删除
// → 对应页行键跳过；行序仍按物化序稳定。直测两阶段接缝（materialize → 删行 →
// hydrate，中间删行即模拟跨锁会话窗口），服务层完整流经路径由其余导出用例覆盖。
func TestAssetExport_HydrateMissSkipsStaleKeys(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出miss库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	fileA := insertExportFile(t, store, "方案/方案甲.pptx", "方案甲标题", "", 0)
	insertExportSlide(t, store, fileA, 1, "甲一", "甲一正文", "")
	insertExportSlide(t, store, fileA, 2, "甲二", "甲二正文", "")
	fileB := insertExportFile(t, store, "获客/方案乙.pptx", "方案乙标题", "", 0)
	insertExportSlide(t, store, fileB, 1, "乙一", "乙一正文", "")

	keys, err := materializeAssetExportRowKeys(store, nil, nil)
	if err != nil {
		t.Fatalf("物化行键: %v", err)
	}
	if len(keys) != 5 {
		t.Fatalf("物化应 5 行键（2 文件行 + 3 页行），实际 %d", len(keys))
	}

	// 模拟物化后的删除窗口：fileA 整文件删除（3 行键全跳过）+ fileB 页 1 删除
	if _, err := store.db.Exec("DELETE FROM files WHERE id = ?", fileA); err != nil {
		t.Fatalf("删除文件行: %v", err)
	}
	if _, err := store.db.Exec("DELETE FROM slides WHERE file_id = ? AND slide_num = 1", fileB); err != nil {
		t.Fatalf("删除页行: %v", err)
	}

	rows, miss, err := hydrateAssetExportBatchRows(store, keys)
	if err != nil {
		t.Fatalf("水合: %v", err)
	}
	if miss != 4 {
		t.Errorf("水合 miss 应 4（fileA 3 行键 + fileB 页 1），实际 %d", miss)
	}
	if len(rows) != 1 {
		t.Fatalf("应仅剩 fileB 文件行 1 行，实际 %d 行", len(rows))
	}
	if rows[0].relPath != "获客/方案乙.pptx" || rows[0].slideNum != 0 {
		t.Errorf("剩余行应为 fileB 文件行（path=获客/方案乙.pptx, slideNum=0），实际 %+v", rows[0])
	}
}

// TestAssetExport_ThumbAndSplitProbe 路径探测（D5）：thumbnail_path 恒绝对（small
// 档缓存键 <fileID>-<3位页>.png，缺档导空）；single_pptx_path 以 split_records
// 为权威、产物存在才导出，库外 derived 落点恒绝对、库内落点（源目录拆分）relative
// 口径转库相对正斜杠；页序超出已拆分页数导空。
func TestAssetExport_ThumbAndSplitProbe(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出探测库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	rel := "方案甲.pptx"
	fileID := insertExportFile(t, store, rel, "方案甲标题", "", 0)
	insertExportSlide(t, store, fileID, 1, "甲一", "甲一正文", "")
	insertExportSlide(t, store, fileID, 2, "甲二", "甲二正文", "")

	// 缩略图：仅页 1 small 档 PNG 存在
	thumbsDir := svc.assetThumbsDir(lib.ID)
	if err := os.MkdirAll(filepath.Join(thumbsDir, "small"), 0o755); err != nil {
		t.Fatalf("预建缩略图目录: %v", err)
	}
	thumb1 := thumbPNGPath(thumbsDir, "small", fileID, 1)
	if err := os.WriteFile(thumb1, []byte("png"), 0o644); err != nil {
		t.Fatalf("预置缩略图: %v", err)
	}

	// 拆分记录：已拆 2 页，产物仅页 1 存在；derived 落点在库外
	derivedOut := filepath.Join(t.TempDir(), "derived-out")
	if err := os.MkdirAll(derivedOut, 0o755); err != nil {
		t.Fatalf("预建拆分产物目录: %v", err)
	}
	product1 := filepath.Join(derivedOut, "方案甲-001.pptx")
	if err := os.WriteFile(product1, []byte("pptx"), 0o644); err != nil {
		t.Fatalf("预置拆分产物: %v", err)
	}
	if err := store.upsertSplitRecord(rel, "hash-x", derivedOut, 2, "2026-10-16 10:00:00"); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}

	csvPath := filepath.Join(t.TempDir(), "probe.csv")
	mustExport(t, svc, lib.ID, model.AssetExportOptions{
		CSVPath: csvPath, Formats: []string{model.AssetExportFormatCSV},
	})
	rows := csvRowsByKey(t, readExportCSV(t, csvPath))

	s1 := rows["1:"+rel]
	if s1[6] != thumb1 {
		t.Errorf("页 1 thumbnail_path 应为 small 档绝对路径 %q，实际 %q", thumb1, s1[6])
	}
	if s1[10] != product1 {
		t.Errorf("页 1 single_pptx_path 应为库外产物绝对路径 %q，实际 %q", product1, s1[10])
	}
	s2 := rows["2:"+rel]
	if s2[6] != "" || s2[10] != "" {
		t.Errorf("缺缩略图/缺产物的页 2 两列应导空：%v", s2)
	}
	fileRow := rows["0:"+rel]
	if fileRow[6] != "" || fileRow[10] != "" {
		t.Errorf("文件行 thumbnail/single_pptx 应恒空：%v", fileRow)
	}

	// 库内落点（源目录拆分形态）：relative 口径转库相对正斜杠；页序越界导空
	inRootOut := filepath.Join(lib.RootPath, "拆分目录")
	if err := os.MkdirAll(inRootOut, 0o755); err != nil {
		t.Fatalf("预建库内产物目录: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inRootOut, "方案甲-001.pptx"), []byte("p"), 0o644); err != nil {
		t.Fatalf("预置库内产物: %v", err)
	}
	if err := store.upsertSplitRecord(rel, "hash-y", inRootOut, 1, "2026-10-16 10:00:00"); err != nil {
		t.Fatalf("覆盖拆分记录: %v", err)
	}
	csvPath2 := filepath.Join(t.TempDir(), "probe-rel.csv")
	mustExport(t, svc, lib.ID, model.AssetExportOptions{
		CSVPath: csvPath2, Formats: []string{model.AssetExportFormatCSV},
	})
	rows2 := csvRowsByKey(t, readExportCSV(t, csvPath2))
	s1rel := rows2["1:"+rel]
	if want := "拆分目录/方案甲-001.pptx"; s1rel[10] != want {
		t.Errorf("库内产物 relative 口径应为 %q，实际 %q", want, s1rel[10])
	}
	s2rel := rows2["2:"+rel]
	if s2rel[10] != "" {
		t.Errorf("页序超出已拆分页数应导空：%v", s2rel[10])
	}
}

// TestAssetExport_SearchReplayPagingBeyondMaxLimit 检索重放分页边界：命中条目超
// 单页上限（assetSearchMaxLimit=200）时分页循环取满 Total 无遗漏无重复——流式
// 分批测试覆盖文件批读（assetsQueryIDBatch=500/批），本用例专覆盖检索语境重放
// 分页（200/页）。数据直接 SQL 插入 + FTS 行写入（与 insertFtsSlideRowTx 同构：
// rowid = -slideID，切分同源 splitFtsText），不经 pptx 解析链。
func TestAssetExport_SearchReplayPagingBeyondMaxLimit(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出重放分页库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	const fileCount = 3
	const slidesPerFile = 80 // 3×80=240 页命中 > 200：重放至少两页
	for i := 1; i <= fileCount; i++ {
		rel := fmt.Sprintf("批/重放%03d.pptx", i)
		fileID := insertExportFile(t, store, rel, "重放标题"+strconv.Itoa(i), "", 0)
		for n := 1; n <= slidesPerFile; n++ {
			title := "重放页" + strconv.Itoa(n)
			res, err := store.db.Exec(
				"INSERT INTO slides(file_id, slide_num, title, text, speaker_notes) VALUES(?, ?, ?, ?, '')",
				fileID, n, title, "重放正文")
			if err != nil {
				t.Fatalf("插入重放页（%s-%d）: %v", rel, n, err)
			}
			slideID, err := res.LastInsertId()
			if err != nil {
				t.Fatalf("读取 slideID: %v", err)
			}
			// FTS 幻灯片行（生产写路径 insertFtsSlideRowTx 同构，切分同源）
			if _, err := store.db.Exec(
				"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, file_name, file_note, slide_title, slide_text, slide_notes) "+
					"VALUES(?, 'slide', ?, ?, '', '', ?, ?, '')",
				-slideID, slideID, fileID, splitFtsText(title), splitFtsText("重放正文")); err != nil {
				t.Fatalf("写入 FTS 行（%s-%d）: %v", rel, n, err)
			}
		}
	}

	csvPath := filepath.Join(t.TempDir(), "replay.csv")
	result := mustExport(t, svc, lib.ID, model.AssetExportOptions{
		CSVPath: csvPath, Formats: []string{model.AssetExportFormatCSV}, Keyword: "重放正文",
	})
	want := fileCount * slidesPerFile
	if result.Rows != want {
		t.Fatalf("检索重放应导出 %d 页行（跨页取满 Total），实际 %d", want, result.Rows)
	}
	rows := readExportCSV(t, csvPath)
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		key := r[1] + ":" + r[0]
		if seen[key] {
			t.Fatalf("重放分页出现重复行：%s", key)
		}
		seen[key] = true
		// 重放范围仅页命中（文件标题不含关键词）：不应出现文件行
		if r[1] == "0" {
			t.Errorf("重放不应含文件行：%s", key)
		}
	}
	if len(seen) != want {
		t.Fatalf("去重后应 %d 行，实际 %d", want, len(seen))
	}
}

// TestAssetExport_IncludesExcludedRows excluded 行照导（素材管理第十批 PR1，SRS
// §2.7.3 可见性分叉）：排除规则命中的已入库文件检索三入口不可见，但导出行键无
// status 过滤（对齐 lost 现状）——全库导出须含 excluded 文件行与其页行。
func TestAssetExport_IncludesExcludedRows(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出排除库", dir)
	buildAssetsPptx(t, dir, "保留件.pptx", []assetsSlideSpec{
		{titlePh: "保留标题", body: []string{"保留正文"}},
	})
	if err := os.MkdirAll(filepath.Join(dir, "营销草稿2024"), 0o755); err != nil {
		t.Fatalf("建排除目录: %v", err)
	}
	buildAssetsPptx(t, filepath.Join(dir, "营销草稿2024"), "草稿件.pptx", []assetsSlideSpec{
		{titlePh: "草稿标题", body: []string{"草稿正文"}},
	})
	mustScanIndex(t, svc, lib)

	// 规则命中存量转 excluded（UpdateLibraryRules 保存即自动重扫）
	if _, err := svc.UpdateLibraryRules(lib.ID, []string{"*草稿*"}, nil); err != nil {
		t.Fatalf("UpdateLibraryRules: %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	excludedSeen := false
	for _, f := range files {
		if f.Path == filepath.ToSlash(filepath.Join("营销草稿2024", "草稿件.pptx")) {
			excludedSeen = f.Status == model.AssetFileStatusExcluded
		}
	}
	if !excludedSeen {
		t.Fatalf("前置失败：草稿件.pptx 应处于 excluded 态")
	}

	csvPath := filepath.Join(t.TempDir(), "out.csv")
	mustExport(t, svc, lib.ID, model.AssetExportOptions{
		CSVPath: csvPath, Formats: []string{model.AssetExportFormatCSV},
	})
	rows := readExportCSV(t, csvPath)
	paths := make(map[string]bool, len(rows))
	for _, r := range rows {
		paths[r[0]] = true
	}
	excludedRel := filepath.ToSlash(filepath.Join("营销草稿2024", "草稿件.pptx"))
	if !paths[excludedRel] {
		t.Errorf("excluded 文件行应照常导出（行键无 status 过滤），实际行集：%v", paths)
	}
	if !paths["保留件.pptx"] {
		t.Errorf("正常文件行应存在，实际行集：%v", paths)
	}
}

// TestAssetExport_ParamsRejected 参数校验：空格式/未知格式/目标空缺/口径非法/
// 库不存在分别拒绝（库不存在走 E_ASSETS_LIB_NOT_FOUND 语义）。
func TestAssetExport_ParamsRejected(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出参数库", dir)
	target := filepath.Join(t.TempDir(), "x.csv")

	if _, err := svc.ExportAssetMetadata(context.Background(), lib.ID, model.AssetExportOptions{CSVPath: target}); err == nil || !strings.Contains(err.Error(), "至少选择一种") {
		t.Errorf("空格式应拒绝，实际 err=%v", err)
	}
	if _, err := svc.ExportAssetMetadata(context.Background(), lib.ID, model.AssetExportOptions{
		Formats: []string{"xlsx"}, CSVPath: target,
	}); err == nil || !strings.Contains(err.Error(), "导出格式非法") {
		t.Errorf("未知格式应拒绝，实际 err=%v", err)
	}
	if _, err := svc.ExportAssetMetadata(context.Background(), lib.ID, model.AssetExportOptions{
		Formats: []string{model.AssetExportFormatCSV},
	}); err == nil || !strings.Contains(err.Error(), "CSV 导出目标") {
		t.Errorf("CSV 目标空缺应拒绝，实际 err=%v", err)
	}
	if _, err := svc.ExportAssetMetadata(context.Background(), lib.ID, model.AssetExportOptions{
		Formats: []string{model.AssetExportFormatMD},
	}); err == nil || !strings.Contains(err.Error(), "Markdown 导出目标") {
		t.Errorf("MD 目标空缺应拒绝，实际 err=%v", err)
	}
	if _, err := svc.ExportAssetMetadata(context.Background(), lib.ID, model.AssetExportOptions{
		Formats: []string{model.AssetExportFormatCSV}, CSVPath: target, PathMode: "unc",
	}); err == nil || !strings.Contains(err.Error(), "路径口径非法") {
		t.Errorf("口径非法应拒绝，实际 err=%v", err)
	}
	if _, err := svc.ExportAssetMetadata(context.Background(), "不存在的库", model.AssetExportOptions{
		Formats: []string{model.AssetExportFormatCSV}, CSVPath: target,
	}); !errors.Is(err, errAssetLibraryNotFound) {
		t.Errorf("库不存在应返回 E_ASSETS_LIB_NOT_FOUND 语义，实际 err=%v", err)
	}
}

// TestAssetExport_CancelledContextAborts 中止链路（预取消 ctx 注入驱动，对齐扫描
// 取消测试先例，禁 sleep 时序）：ctx 检查点在行键分批环顶部（文件头「中止语义」
// 段），首批开始前命中——半成品 CSV（已写 BOM+表头）被 w.abort() 删除，取消错误
// 统一映射 E_ASSETS_CANCELLED（导出文案 sentinel）；导出结束注册表已注销，
// CancelExport 幂等返回 false。
func TestAssetExport_CancelledContextAborts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出取消库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	fileID := insertExportFile(t, store, "方案乙.pptx", "方案乙标题", "", 0)
	insertExportSlide(t, store, fileID, 1, "乙一", "乙一正文", "")

	csvPath := filepath.Join(t.TempDir(), "cancelled.csv")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 预取消：首批检查点必命中，无时序依赖
	_, err := svc.ExportAssetMetadata(ctx, lib.ID, model.AssetExportOptions{
		CSVPath: csvPath, Formats: []string{model.AssetExportFormatCSV},
	})
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
		t.Fatalf("取消导出应返回 E_ASSETS_CANCELLED，实际 %v", err)
	}
	if appErr.Message != "导出已取消" {
		t.Errorf("取消消息应为导出措辞，实际 %q", appErr.Message)
	}
	if _, statErr := os.Stat(csvPath); !os.IsNotExist(statErr) {
		t.Errorf("中止后半成品 CSV 应被 abort 删除，stat err=%v", statErr)
	}
	if svc.CancelExport(lib.ID) {
		t.Error("导出结束后注册表应已注销，CancelExport 应幂等返回 false")
	}
}

// TestCancelExport_HitsRegisteredCancel 注册表命中语义（对齐 CancelScan / 拆分
// 取消注册表三件套同构模式）：无在途导出返回 false；注册在途条目后命中返回 true
// 且 CancelFunc 生效（ctx.Done 关闭）；注销幂等（条目已被 CancelExport 删除后
// unregister 不 panic 不误删）。
func TestCancelExport_HitsRegisteredCancel(t *testing.T) {
	svc := newTestAssetService(t)
	if svc.CancelExport("lib-x") {
		t.Fatal("无在途导出应返回 false")
	}
	ctx, cancel := context.WithCancel(context.Background())
	entry := svc.registerExportCancel("lib-x", cancel)
	if !svc.CancelExport("lib-x") {
		t.Fatal("注册后应命中返回 true")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("CancelExport 应触发登记的 CancelFunc")
	}
	svc.unregisterExportCancel("lib-x", entry)
}
