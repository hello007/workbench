package service

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"workbench/model"
)

// 素材元数据导出链路（素材管理第九批 PR1，SRS §2.6.4 / 附录A）：CSV（UTF-8 BOM +
// RFC 4180 全字段引号保护）与 Markdown+YAML frontmatter 卡片（Obsidian 互通），
// 打通外部知识库消费面。
//
// 纯只读边界（硬约束）：只发 SELECT 与 os.Stat/文件写出，禁碰索引写链路（files/
// slides/FTS/thumb_cache/split_records 均不写）；写面仅用户指定的导出目标路径
//（非 data/ 域，不受 util.SaveJSON 原子写约束）。
//
// 行粒度（D3）：每页一行（slide_num ≥ 1，note/rating 取页级 slide_meta，tags =
// 页自有 ∪ 归属文件合并去重）+ 文件行 slide_num=0（文件级 note/rating/tags）；
// 输出按文件相对路径 + 页序稳定排序。
//
// 字段（SRS 11 列固定序）：ocr_text / summary 为保留列恒空（OCR 与 AI 摘要未实现，
// §2.7 前瞻占位）。
//
// 路径口径（D5）：source_file_path 按入参口径（relative 基准 root_path 正斜杠 /
// absolute）；thumbnail_path 恒绝对（small 档缓存实际位置，键 <fileID>-<3位页>.png
// 与 thumbPNGPath 单点同口径，探测不存在导空）；single_pptx_path 跟随 source 口径
//（定位以 split_records.output_dir 为权威，与 OpenAssetSplitPage 同链，产物存在性
// 探测缺失导空；库外 derived 缓存产物无库相对语义，恒绝对）。
//
// 全局锁解耦（P1 审核收口，勿回退整程单会话形态）：导出全程不长时间持有 service
// 级全局 s.mu（withStore 互斥域，全部库共享一把，长持锁会阻塞所有库的检索/统计/
// 扫描/拆分）。三段式：① 第一遍短锁会话物化导出行键全集——范围内全部
// (fileID, path, slideNum) 行键按 path + slide_num 稳定序一次取齐（全库单条 LEFT
// JOIN；选中/检索范围按 fileID 分批 IN，同一会话内批量查询非逐行查询）；② 行键
// 分批（批大小复用 assetsQueryIDBatch），每批新开短 withStore 会话做纯 DB 水合，
// 批间释放锁；③ 批内水合产物在锁外写汇——MD 卡文件写出与缩略图/拆分产物 os.Stat
// 存在性探测均不占锁。禁改批间 OFFSET 续读：批间释放锁后并发写插入会使 OFFSET
// 分页漂移，行键物化即为此而设。
//
// 快照口径（语义留痕）：行键物化后到水合会话之间新增的文件/页不进本次导出；已
// 物化行键在水合前被删（增量重扫/清理丢失记录）按水合 miss 跳过计数（日志
// skippedRows）——导出是调用时刻的快照面，不提供跨时长一致性事务。
//
// 大库流式写（PR1 验收项）：行键分批水合 + 逐行写 CSV 行与 MD 卡（双格式单遍历
// 同投），行载荷按批驻留（批大小 500 行键）不整体拼接全量字符串，1 万文件级内存
// 平稳。
//
// 中止语义（P3 体验收口定案）：ExportAssetMetadata 一次性语义——每次调用全新
// writer，无跨调用重入；失败与用户中止统一走 w.abort() 删半成品 CSV 收口（MD
// 已写卡片保留，同名覆盖写幂等，重导即收敛）；再次导出 = 新调用。中止通道对齐
// 既有扫描取消形态（service 内部派生可取消 ctx + exportCancels 注册表 + 独立
// CancelExport 命中，App 绑定无 ctx 参数透传）：ctx 检查点在行键分批环顶部，
// 取消后下一批开始前收口。物化阶段（第一遍短锁 + 检索重放）与单批水合内部不可
// 中断——均有界（行键全集与批大小封顶），批间粒度与「锁外写汇」三段式正交。
//
// 单连接池自锁纪律：物化与逐文件水合的每条查询收齐结果并显式 Close 后才发下一条
//（见 assets_db.go libraryStats 注释）。

// assetExportCSVHeader CSV 表头（SRS §2.6.4 / 附录A 11 列固定序）。
var assetExportCSVHeader = []string{
	"source_file_path", "slide_num", "slide_title", "slide_text", "speaker_notes",
	"ocr_text", "thumbnail_path", "tags", "summary", "note", "single_pptx_path",
}

// assetExportRow 导出中间行（行粒度 D3 的内存形态）：文件行 slide_num=0（text/
// speakerNotes/ownTags 恒空，note/rating 取 files 列），页行 slide_num ≥ 1。
// thumbnail_path / single_pptx_path 不驻留行内——写出口径化时按 pathMode 现算
// （存在性探测），避免为未选格式做无效探测。
type assetExportRow struct {
	fileID       int64
	relPath      string // 库内相对路径（正斜杠，索引存储原样）
	slideNum     int    // 0 = 文件行
	title        string
	text         string
	speakerNotes string
	fileTags     []string // 文件标签名（查询序：按标签名升序）
	ownTags      []string // 页自有标签名（页行；按标签名升序）
	note         string
	rating       int
	// 拆分记录快照（单文件首次需要页行时惰性查询一次）：hasSplitRecord=false =
	// 无记录（single_pptx_path 恒空）；splitPageCount 为记录页数（页序越界导空）
	hasSplitRecord bool
	splitOutputDir string
	splitPageCount int
}

// assetExportCtx 导出写出口径上下文（rootPath 拼接与缩略图目录定位基准）。
type assetExportCtx struct {
	lib       model.AssetLibrary
	thumbsDir string // 缩略图缓存根目录（data/assets/<lib-id>/thumbs）
	pathMode  string // AssetExportPathMode* 取值（入口已归一）
}

// mergedTags 行标签合并口径（D3 继承合并单点在后端）：页行 = 页自有 ∪ 归属文件
// 去重（同名标签两级绑定只出现一次），按名称升序稳定输出；文件行 = 文件标签。
func (r *assetExportRow) mergedTags() []string {
	if len(r.ownTags) == 0 {
		return r.fileTags
	}
	seen := make(map[string]bool, len(r.fileTags)+len(r.ownTags))
	out := make([]string, 0, len(r.fileTags)+len(r.ownTags))
	for _, t := range r.fileTags {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range r.ownTags {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// csvFields CSV 行字段（11 列固定序，转义由 encoding/csv 全字段引号保护承担）。
func (r *assetExportRow) csvFields(ectx *assetExportCtx) []string {
	return []string{
		assetExportSourcePath(ectx.lib.RootPath, r.relPath, ectx.pathMode),
		strconv.Itoa(r.slideNum),
		r.title,
		r.text,
		r.speakerNotes,
		"", // ocr_text 保留列恒空（OCR 未实现，SRS §2.7）
		r.thumbnailPath(ectx),
		strings.Join(r.mergedTags(), ";"),
		"", // summary 保留列恒空（AI 摘要未实现，SRS §2.7）
		r.note,
		r.singlePptxPath(ectx),
	}
}

// thumbnailPath 缩略图缓存路径（D5 恒绝对）：small 档实际位置，键 <fileID>-<3位页>
// .png（thumbPNGPath 单点同口径）；仅页行携带，未生成/已淘汰导空（探测存在性，
// 不回退 hd 档——导出语义是「当前缓存有什么」）。
func (r *assetExportRow) thumbnailPath(ectx *assetExportCtx) string {
	if r.slideNum < 1 {
		return ""
	}
	p := thumbPNGPath(ectx.thumbsDir, "small", r.fileID, r.slideNum)
	if !fileExists(p) {
		return ""
	}
	return p
}

// singlePptxPath 拆分单页产物路径（D5 跟随 source 口径 + 存在性探测）：定位以
// split_records.output_dir 为权威（与 OpenAssetSplitPage 同链，覆盖 derived 缓存
// 与源目录两种落点），页序超出已拆分页数或产物文件不存在均导空。库内产物（源目录
// 拆分落点）按 pathMode 相对化（relative = 相对 root_path 正斜杠）；库外 derived
// 缓存产物无库相对语义，恒绝对。
func (r *assetExportRow) singlePptxPath(ectx *assetExportCtx) string {
	if r.slideNum < 1 || !r.hasSplitRecord || r.slideNum > r.splitPageCount {
		return ""
	}
	abs := filepath.Join(r.splitOutputDir,
		fmt.Sprintf("%s-%03d.pptx", splitSourceBaseName(r.relPath), r.slideNum))
	if !fileExists(abs) {
		return ""
	}
	if ectx.pathMode == model.AssetExportPathModeRelative && assetExportUnderRoot(ectx.lib.RootPath, abs) {
		rel, err := filepath.Rel(ectx.lib.RootPath, abs)
		if err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return abs
}

// assetExportUnderRoot 绝对路径是否位于素材根目录内（single_pptx_path 口径化辅助，
// 判定口径对齐 resolveLibraryRelPath 的越界遏制：Rel 失败或 `..` 前缀均视为库外）。
func assetExportUnderRoot(rootPath, absPath string) bool {
	rel, err := filepath.Rel(rootPath, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// assetExportSourcePath source_file_path 口径化（D5）：relative = 索引存储原样
// （库内相对路径正斜杠）；absolute = rootPath 拼接的本地绝对路径。
func assetExportSourcePath(rootPath, relPath, pathMode string) string {
	if pathMode == model.AssetExportPathModeAbsolute {
		return filepath.Join(rootPath, filepath.FromSlash(relPath))
	}
	return relPath
}

// yamlAssetPath YAML 路径口径（SRS §2.6.4）：Windows 反斜杠统一转 /，防 YAML
// 双引号标量内反斜杠转义报错。
func yamlAssetPath(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// yamlQuote YAML 双引号标量包裹：反斜杠/双引号/换行转义（frontmatter 只放单行
// 标量——路径/标题/标签名；slide_text 等多行内容走正文段落不经此处）。
func yamlQuote(s string) string {
	replaced := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
	).Replace(s)
	return "\"" + replaced + "\""
}

// assetExportMDTitleMaxRunes 卡片标题清洗后的 rune 上限（防超长标题把文件名推过
// 路径长度上限；Windows MAX_PATH 260 下深目录叠加余量）。
const assetExportMDTitleMaxRunes = 60

// assetExportMDTitle 卡片标题清洗（D4）：Windows 文件名非法字符（\/:*?"<>|）与
// 控制符替换为 '-'，去首尾空白与尾随点空格（Windows 命名规则），内部空白折叠，
// rune 上限截断；清洗后为空回退「无标题」（与解析空标题兜底同文案）。
func assetExportMDTitle(title string) string {
	var b strings.Builder
	for _, r := range title {
		if r < 0x20 || strings.ContainsRune(`\/:*?"<>|`, r) {
			b.WriteRune('-')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimRight(strings.TrimSpace(b.String()), ". ")
	out = strings.Join(strings.Fields(out), " ")
	runes := []rune(out)
	if len(runes) > assetExportMDTitleMaxRunes {
		out = string(runes[:assetExportMDTitleMaxRunes])
	}
	if strings.TrimRight(out, ". ") == "" {
		return "无标题"
	}
	return out
}

// buildAssetExportMarkdown 单行 Markdown 卡片内容（附录A 示例形态）：YAML
// frontmatter（source_file_path / slide_num / slide_title / tags / rating）+
// 正文（# 标题 → 幻灯片文本 → 演讲者备注 → 业务备注；空段省略）。文件行
// slide_num=0、无文本段，备注取文件级 note。
func buildAssetExportMarkdown(row *assetExportRow, sourcePath string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "source_file_path: %s\n", yamlQuote(yamlAssetPath(sourcePath)))
	fmt.Fprintf(&b, "slide_num: %d\n", row.slideNum)
	fmt.Fprintf(&b, "slide_title: %s\n", yamlQuote(row.title))
	tags := row.mergedTags()
	tagItems := make([]string, 0, len(tags))
	for _, t := range tags {
		tagItems = append(tagItems, yamlQuote(t))
	}
	fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(tagItems, ", "))
	if row.rating > 0 {
		fmt.Fprintf(&b, "rating: %d\n", row.rating)
	}
	b.WriteString("---\n\n")
	b.WriteString("# " + row.title + "\n")
	if row.text != "" {
		b.WriteString("\n" + row.text + "\n")
	}
	if row.speakerNotes != "" {
		b.WriteString("\n## 演讲者备注\n\n" + row.speakerNotes + "\n")
	}
	if row.note != "" {
		b.WriteString("\n## 备注\n\n" + row.note + "\n")
	}
	return b.String()
}

// assetExportWriter 双格式写出汇（csv/md 可同选）：行经 writeRow 同投已启用汇，
// 计数各自累计。CSV 打开即写 UTF-8 BOM + 表头（UseCRLF 行尾，Excel 直开中文
// 不乱码）；MD 目录 MkdirAll 预建。中途失败经 abort 收口（关闭并删除半成品
// CSV，防截断文件被误认为有效导出；已写出的 MD 卡保留——同名覆盖写幂等，
// 重导即收敛）。
type assetExportWriter struct {
	ectx    *assetExportCtx
	csvPath string // 导出目标 CSV 路径（abort 删半成品定位）
	csvFile *os.File
	csvBuf  *bufio.Writer
	csv     *csv.Writer
	mdDir   string
	hasCSV  bool
	hasMD   bool
	mdSeq   int // 卡片全局流水号（1 起，含文件行——每行一卡）
	rows    int // 已写数据行数（文件行 + 页行）
	mdFiles int // 已写 MD 卡片数
}

// openAssetExportWriter 建汇（写文件前调用，入参已在入口完成目标空缺校验）。
func openAssetExportWriter(ectx *assetExportCtx, opts model.AssetExportOptions, formats []string) (*assetExportWriter, error) {
	w := &assetExportWriter{ectx: ectx, csvPath: opts.CSVPath}
	for _, f := range formats {
		switch f {
		case model.AssetExportFormatCSV:
			// TOCTOU 收口（P2a）：入口的 os.Stat 预检只是 UX 提前返回，预检通过到
			// 建文件之间存在目标被并发创建的窗口——非覆盖用 O_EXCL 创建（目标已
			// 存在打开失败，兜底映射同一确认型错误码，单点构造复用），覆盖用
			// O_TRUNC 直写；打开语义是权威判定。md 目录形态预检保留（writeRow
			// 卡名带全局序号，文件级冲突面在目录预检已覆盖）
			openFlags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			if !opts.Overwrite {
				openFlags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
			}
			file, err := os.OpenFile(opts.CSVPath, openFlags, 0o644)
			if err != nil {
				if !opts.Overwrite && fileExists(opts.CSVPath) {
					return nil, assetExportTargetExistsError(opts.CSVPath)
				}
				_ = w.abort()
				return nil, fmt.Errorf("创建 CSV 导出文件失败: %w", err)
			}
			w.csvFile = file
			w.csvBuf = bufio.NewWriter(file)
			// UTF-8 BOM 前置（Excel 按 UTF-8 识别中文，缺 BOM 按 ANSI 解码乱码）
			if _, err := w.csvBuf.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
				_ = w.abort()
				return nil, fmt.Errorf("写 CSV BOM 失败: %w", err)
			}
			w.csv = csv.NewWriter(w.csvBuf)
			w.csv.UseCRLF = true
			if err := w.csv.Write(assetExportCSVHeader); err != nil {
				_ = w.abort()
				return nil, fmt.Errorf("写 CSV 表头失败: %w", err)
			}
			w.hasCSV = true
		case model.AssetExportFormatMD:
			if err := os.MkdirAll(opts.MDDir, 0o755); err != nil {
				_ = w.abort()
				return nil, fmt.Errorf("创建 Markdown 导出目录失败: %w", err)
			}
			w.mdDir = opts.MDDir
			w.hasMD = true
		}
	}
	return w, nil
}

// abort 中途失败收口：关闭未收口的 CSV 文件并删除半成品（MD 已写卡片保留）。
func (w *assetExportWriter) abort() error {
	if w.csvFile == nil {
		return nil
	}
	_ = w.csvBuf.Flush()
	_ = w.csvFile.Close()
	w.csvFile = nil
	return os.Remove(w.csvPath)
}

// finish 正常收口：flush CSV（csv.Writer → bufio → file 逐层）并关闭。
func (w *assetExportWriter) finish() error {
	if !w.hasCSV {
		return nil
	}
	w.csv.Flush()
	if err := w.csv.Error(); err != nil {
		return fmt.Errorf("CSV 写入失败: %w", err)
	}
	if err := w.csvBuf.Flush(); err != nil {
		return fmt.Errorf("CSV 写入失败: %w", err)
	}
	if err := w.csvFile.Close(); err != nil {
		return fmt.Errorf("关闭 CSV 导出文件失败: %w", err)
	}
	w.csvFile = nil
	return nil
}

// writeRow 单行同投已启用汇（CSV 一行 / MD 一卡，卡名 `<3位序号>-<标题清洗>.md`）。
func (w *assetExportWriter) writeRow(row *assetExportRow) error {
	w.rows++
	if w.hasCSV {
		if err := w.csv.Write(row.csvFields(w.ectx)); err != nil {
			return fmt.Errorf("写 CSV 行失败（%s）: %w", row.relPath, err)
		}
	}
	if w.hasMD {
		w.mdSeq++
		name := fmt.Sprintf("%03d-%s.md", w.mdSeq, assetExportMDTitle(row.title))
		sourcePath := assetExportSourcePath(w.ectx.lib.RootPath, row.relPath, w.ectx.pathMode)
		if err := os.WriteFile(filepath.Join(w.mdDir, name), []byte(buildAssetExportMarkdown(row, sourcePath)), 0o644); err != nil {
			return fmt.Errorf("写 Markdown 卡片失败（%s）: %w", name, err)
		}
		w.mdFiles++
	}
	return nil
}

// assetExportSearchActive 检索语境激活判定（D2）：关键词/标签/facet 任一非空；
// 排序档单独存在不构成检索语境（全库导出不因排序参数缩窄）。
func assetExportSearchActive(keyword string, opts model.AssetSearchOptions) bool {
	return keyword != "" || len(opts.TagIDs) > 0 || newAssetsFacetFilter(opts) != nil
}

// resolveAssetExportScope 导出范围解析（D2 三态，优先级 选中 > 检索命中 > 全库）。
// 返回文件行目标集与页行目标集；双 nil = 全库不设界（与标签筛选 nil 放行同款约定）。
//
// 检索语境后端重放（PR1 验收项：导全集而非前端已加载页）：复用 searchAssetsInStore
// 全部既有路径（FTS 关键词 / 纯标签 / facets-only + facet 过滤 + 排序），恒平铺
// （Flatten=true——聚合语义与导出行粒度无关）并分页循环取满 Total。
func resolveAssetExportScope(store *assetsStore, keyword string, opts model.AssetExportOptions) (map[int64]bool, map[slideAnnotKey]bool, error) {
	if len(opts.FileIDs) > 0 || len(opts.SlideTargets) > 0 {
		fileScope := make(map[int64]bool, len(opts.FileIDs))
		for _, id := range opts.FileIDs {
			fileScope[id] = true
		}
		slideScope := make(map[slideAnnotKey]bool, len(opts.SlideTargets))
		for _, t := range opts.SlideTargets {
			if t.SlideNum >= 1 {
				slideScope[slideAnnotKey{fileID: t.FileID, slideNum: int64(t.SlideNum)}] = true
			}
		}
		return fileScope, slideScope, nil
	}
	if !assetExportSearchActive(keyword, opts.Search) {
		return nil, nil, nil
	}
	replay := opts.Search
	replay.Flatten = true
	replay.Limit = assetSearchMaxLimit
	replay.Offset = 0
	fileScope := make(map[int64]bool)
	slideScope := make(map[slideAnnotKey]bool)
	collected := 0
	for {
		page, err := searchAssetsInStore(store, keyword, replay)
		if err != nil {
			return nil, nil, err
		}
		for _, hit := range page.Items {
			if hit.RefType == model.AssetRefTypeFile {
				fileScope[hit.FileID] = true
				continue
			}
			slideScope[slideAnnotKey{fileID: hit.FileID, slideNum: int64(hit.SlideNum)}] = true
		}
		collected += len(page.Items)
		if len(page.Items) == 0 || collected >= page.Total {
			break
		}
		replay.Offset = collected
	}
	return fileScope, slideScope, nil
}

// assetExportSlideRow 页清单水合行。
type assetExportSlideRow struct {
	num   int
	title string
	text  string
	notes string
}

// assetExportFileData 单文件水合数据（行键驱动产出用）：exists=false = 物化后
// files 行已删除（水合 miss，该文件全部行键跳过，见 materializeAssetExportRowKeys
// 快照口径注释）；页内容按页序入 map（行键只含页号，页行水合 miss = 重解析后
// 该页消失）。
type assetExportFileData struct {
	exists   bool
	path     string
	title    string
	note     string
	rating   int
	slides   map[int]assetExportSlideRow
	fileTags []string
	ownTags  map[int64][]string
	meta     map[int64]slideMetaAnnot
	split    *assetSplitRecord // 页行需要时惰性水合（split_records 定位 single_pptx_path）
}

// hydrateAssetExportFile 单文件水合（≤6 条顺序查询，单连接池纪律：每条收齐并
// Close 后才发下一条）。标签按名称升序；needSplit=false 跳过拆分记录查询（批内
// 无页行目标时不做无效查询）。
func (s *assetsStore) hydrateAssetExportFile(fileID int64, needSplit bool) (*assetExportFileData, error) {
	data := &assetExportFileData{
		slides:  make(map[int]assetExportSlideRow),
		ownTags: make(map[int64][]string),
		meta:    make(map[int64]slideMetaAnnot),
	}

	// files 行存在性与文件级字段（QueryRow+Scan 单行口径；ErrNoRows = 水合 miss：
	// 物化后该文件被删除/清理，不上抛，由调用方按 exists 跳过该文件全部行键）
	err := s.db.QueryRow("SELECT path, title, note, rating FROM files WHERE id = ?", fileID).
		Scan(&data.path, &data.title, &data.note, &data.rating)
	if errors.Is(err, sql.ErrNoRows) {
		return data, nil
	}
	if err != nil {
		return nil, err
	}
	data.exists = true

	if needSplit {
		rec, err := s.getSplitRecord(data.path)
		if err != nil {
			return nil, err
		}
		data.split = rec
	}

	rows, err := s.db.Query(
		"SELECT slide_num, title, text, speaker_notes FROM slides WHERE file_id = ? ORDER BY slide_num", fileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sl assetExportSlideRow
		if err := rows.Scan(&sl.num, &sl.title, &sl.text, &sl.notes); err != nil {
			_ = rows.Close()
			return nil, err
		}
		data.slides[sl.num] = sl
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = s.db.Query(
		"SELECT t.name FROM file_tags ft JOIN tags t ON t.id = ft.tag_id WHERE ft.file_id = ? ORDER BY t.name", fileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		data.fileTags = append(data.fileTags, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = s.db.Query(
		"SELECT st.slide_num, t.name FROM slide_tags st JOIN tags t ON t.id = st.tag_id "+
			"WHERE st.file_id = ? ORDER BY st.slide_num, t.name", fileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var num int64
		var name string
		if err := rows.Scan(&num, &name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		data.ownTags[num] = append(data.ownTags[num], name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = s.db.Query(
		"SELECT slide_num, note, rating FROM slide_meta WHERE file_id = ? ORDER BY slide_num", fileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var num int64
		var m slideMetaAnnot
		if err := rows.Scan(&num, &m.note, &m.rating); err != nil {
			_ = rows.Close()
			return nil, err
		}
		data.meta[num] = m
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return data, nil
}

// assetExportRowKey 物化行键（第一遍短锁会话产物，文件头「全局锁解耦」段）：范围
// 内一个导出行的定位键，slideNum 0 = 文件行、≥1 = 页行；path 为稳定排序键。
type assetExportRowKey struct {
	fileID   int64
	path     string
	slideNum int
}

// materializeAssetExportRowKeys 物化导出行键全集（第一遍短锁会话内执行）：范围内
// 全部 (fileID, path, slideNum) 行键按 path + slide_num 稳定序一次取齐，后续分批
// 水合按行键驱动，数据库游标不跨锁会话存续。
//
// 语义留痕（物化快照口径，脱离本函数可懂）：行键物化后到水合会话之间新增的
// 文件/页不进本次导出；已物化行键在水合前被删（增量重扫/清理丢失记录）由
// hydrateAssetExportBatchRows 按水合 miss 跳过并计数——导出是调用时刻的快照面，
// 不提供跨时长一致性事务。
func materializeAssetExportRowKeys(store *assetsStore, fileScope map[int64]bool, slideScope map[slideAnnotKey]bool) ([]assetExportRowKey, error) {
	if fileScope == nil && slideScope == nil {
		// 全库：文件行（slide_num=0）与页行 UNION ALL 一次取齐——LEFT JOIN 单条
		// 只为无 slides 行的文件产出 COALESCE=0 行、有页文件不产文件行（文件行会
		// 整体缺失），文件行必须独立一路；ORDER BY path + 行号保证「文件行先于
		// 其全部页行」（复合 SELECT 按行列名排序，SQLite 合法）
		rows, err := store.db.Query(
			"SELECT id, path, 0 AS row_num FROM files " +
				"UNION ALL " +
				"SELECT s.file_id, f.path, s.slide_num FROM slides s JOIN files f ON f.id = s.file_id " +
				"ORDER BY path, row_num")
		if err != nil {
			return nil, err
		}
		keys := make([]assetExportRowKey, 0, 64)
		for rows.Next() {
			var k assetExportRowKey
			var num int64
			if err := rows.Scan(&k.fileID, &k.path, &num); err != nil {
				_ = rows.Close()
				return nil, err
			}
			k.slideNum = int(num)
			keys = append(keys, k)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		return keys, nil
	}

	// 选中/检索范围：文件行按 fileScope 的 fileID 分批 IN 取齐；页行按涉及文件
	// 分批 IN 取全部页后内存按 (fileID, slideNum) 对过滤（范围判定走内存 map，
	// 避免逐对 OR 长链与 row-value IN 方言耦合）；同一短锁会话内的批量查询，
	// 非逐行查询。
	var keys []assetExportRowKey
	appendFileRowKeys := func(ids []int64) error {
		for start := 0; start < len(ids); start += assetsQueryIDBatch {
			end := start + assetsQueryIDBatch
			if end > len(ids) {
				end = len(ids)
			}
			placeholders := strings.TrimSuffix(strings.Repeat("?, ", end-start), ", ")
			args := make([]any, 0, end-start)
			for _, id := range ids[start:end] {
				args = append(args, id)
			}
			rows, err := store.db.Query(
				"SELECT id, path FROM files WHERE id IN ("+placeholders+")", args...)
			if err != nil {
				return err
			}
			for rows.Next() {
				var k assetExportRowKey
				if err := rows.Scan(&k.fileID, &k.path); err != nil {
					_ = rows.Close()
					return err
				}
				keys = append(keys, k)
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
		}
		return nil
	}
	scopeIDs := func(scope map[int64]bool) []int64 {
		ids := make([]int64, 0, len(scope))
		for id := range scope {
			ids = append(ids, id)
		}
		return ids
	}
	if fileScope != nil {
		if err := appendFileRowKeys(scopeIDs(fileScope)); err != nil {
			return nil, err
		}
	}
	if slideScope != nil {
		// 页行涉及文件集合（去重后分批 IN）
		slideFileIDs := make(map[int64]bool, len(slideScope))
		for key := range slideScope {
			slideFileIDs[key.fileID] = true
		}
		ids := scopeIDs(slideFileIDs)
		for start := 0; start < len(ids); start += assetsQueryIDBatch {
			end := start + assetsQueryIDBatch
			if end > len(ids) {
				end = len(ids)
			}
			placeholders := strings.TrimSuffix(strings.Repeat("?, ", end-start), ", ")
			args := make([]any, 0, end-start)
			for _, id := range ids[start:end] {
				args = append(args, id)
			}
			rows, err := store.db.Query(
				"SELECT s.file_id, f.path, s.slide_num FROM slides s JOIN files f ON f.id = s.file_id "+
					"WHERE s.file_id IN ("+placeholders+") ORDER BY f.path, s.slide_num", args...)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var k assetExportRowKey
				var num int64
				if err := rows.Scan(&k.fileID, &k.path, &num); err != nil {
					_ = rows.Close()
					return nil, err
				}
				k.slideNum = int(num)
				if !slideScope[slideAnnotKey{fileID: k.fileID, slideNum: num}] {
					continue // 同文件未选中的页（页行范围过滤）
				}
				keys = append(keys, k)
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if err := rows.Close(); err != nil {
				return nil, err
			}
		}
	}
	// 两路行键（文件行 + 页行）合并后内存排序：与全库 LEFT JOIN 的 SQL 排序同
	// 口径（path 字节序 + slide_num 升序，文件行 0 先于其全部页行）
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		return keys[i].slideNum < keys[j].slideNum
	})
	return keys, nil
}

// hydrateAssetExportBatchRows 单批行键水合（一个短锁会话内执行，纯 DB 读：不写
// 索引、不做文件 IO 与存在性探测——写汇与 os.Stat 由调用方在锁外执行）。批内同
// 文件多行键共享一次水合；返回行键序导出行载荷与水合 miss 计数（物化后被删的
// files/slides 行键跳过，快照口径见 materializeAssetExportRowKeys 注释）。
//
// 批边界可切开同一文件的行键（行键全局有序后同文件相邻、跨批可能截断），跨批
// 文件在相邻两批各水合一次——仅重复查询开销，无正确性影响。
func hydrateAssetExportBatchRows(store *assetsStore, keys []assetExportRowKey) ([]*assetExportRow, int, error) {
	// 批内页行需求按文件聚合（文件行无拆分语义，仅批内存在页行键才查拆分记录）
	needSplit := make(map[int64]bool, len(keys)/2+1)
	for _, k := range keys {
		if k.slideNum >= 1 {
			needSplit[k.fileID] = true
		}
	}

	hydrated := make(map[int64]*assetExportFileData, len(needSplit)+1)
	misses := 0
	rowsOut := make([]*assetExportRow, 0, len(keys))
	for _, k := range keys {
		hd, ok := hydrated[k.fileID]
		if !ok {
			var err error
			hd, err = store.hydrateAssetExportFile(k.fileID, needSplit[k.fileID])
			if err != nil {
				return nil, misses, fmt.Errorf("读取导出数据失败（%s）: %w", k.path, err)
			}
			hydrated[k.fileID] = hd
		}
		if !hd.exists {
			misses++ // files 行已消失：该文件全部行键跳过
			continue
		}
		if k.slideNum == 0 {
			rowsOut = append(rowsOut, &assetExportRow{
				fileID:   k.fileID,
				relPath:  hd.path,
				slideNum: 0, // 文件行
				title:    hd.title,
				fileTags: hd.fileTags,
				note:     hd.note,
				rating:   hd.rating,
			})
			continue
		}
		sl, ok := hd.slides[k.slideNum]
		if !ok {
			misses++ // 页行已消失（重解析减页/清理）：该行键跳过
			continue
		}
		// 行序 = (相对路径, slide_num) 升序由行键序保证：文件行先于其全部页行
		//（与检索「文件本体先于其页」分组语义同口径）
		row := &assetExportRow{
			fileID:       k.fileID,
			relPath:      hd.path,
			slideNum:     sl.num,
			title:        sl.title,
			text:         sl.text,
			speakerNotes: sl.notes,
			fileTags:     hd.fileTags,
			ownTags:      hd.ownTags[int64(sl.num)],
		}
		if m, ok := hd.meta[int64(sl.num)]; ok {
			row.note = m.note
			row.rating = m.rating
		}
		if hd.split != nil {
			row.hasSplitRecord = true
			row.splitOutputDir = hd.split.outputDir
			row.splitPageCount = hd.split.pageCount
		}
		rowsOut = append(rowsOut, row)
	}
	return rowsOut, misses, nil
}

// errAssetExportTargetExists 导出目标已存在的确认型错误基座（消息由构造点按
// 具体目标拼装，错误码供前端识别后弹覆盖确认框）。
func assetExportTargetExistsError(target string) error {
	return model.NewAppError(model.ErrCodeAssetsExportTargetExists,
		fmt.Sprintf("导出目标已存在：%s。确认覆盖将重写同名内容", target))
}

// errAssetExportCancelled 素材元数据导出被用户取消（复用 E_ASSETS_CANCELLED 错误
// 码——前端 warning 分流按 code；消息按导出措辞，与扫描 sentinel ErrAssetsCancelled
// 的「扫描已取消」区分，对齐 errAssetSplitCancelled 先例）。
var errAssetExportCancelled = model.NewAppError(model.ErrCodeAssetsCancelled, "导出已取消")

// mapAssetExportCancelError 将 ctx 取消错误（%w 包装 context.Canceled 透传）映射为
// 导出文案的 errAssetExportCancelled，其余错误原样透传（单点实现防漂移）。
func mapAssetExportCancelError(err error) error {
	if errors.Is(err, context.Canceled) {
		return errAssetExportCancelled
	}
	return err
}

// normalizeAssetExportFormats 导出格式归一与校验：去重保序，空列表 / 未知取值拒绝
// （参数拒绝走普通 error 形态，对齐 BatchSetRating 先例）。
func normalizeAssetExportFormats(formats []string) ([]string, error) {
	seen := make(map[string]bool, len(formats))
	out := make([]string, 0, len(formats))
	for _, f := range formats {
		f = strings.TrimSpace(strings.ToLower(f))
		if f == "" {
			continue
		}
		if f != model.AssetExportFormatCSV && f != model.AssetExportFormatMD {
			return nil, fmt.Errorf("导出格式非法（%q，须为 csv / md）", f)
		}
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("请至少选择一种导出格式（CSV / Markdown）")
	}
	return out, nil
}

// ExportAssetMetadata 导出素材元数据（SRS §2.6.4，工具行「导出」入口）：范围三态
// （D2 选中 > 检索命中 > 全库）、行粒度（D3 文件行 slide_num=0 + 页行）、CSV
// （UTF-8 BOM）/ Markdown+YAML（D4 每行一卡）可同选、路径口径（D5）、覆盖确认
// （目标已存在返回 E_ASSETS_EXPORT_TARGET_EXISTS，overwrite=true 重调放行）。
// 纯只读：索引零写入；大库流式写（分批水合 + 逐行写，无全量内存拼接）；全局锁
// 解耦（文件头「全局锁解耦」段：物化行键 + 分批短会话水合 + 锁外写汇）。
//
// 可取消（文件头「中止语义」段）：内部派生可取消 ctx 并登记 exportCancels 注册表
// （键 = 规范化 libraryID），CancelExport 从另一调用线命中；前端「提交中关闭导出
// 弹窗 = 放弃本次导出」经 CancelAssetExport 绑定触发。取消错误经
// mapAssetExportCancelError 统一映射 E_ASSETS_CANCELLED（warning 中性提示码）。
// defer LIFO：unregisterExportCancel 先行收口（docs/spec/logging-and-errors.md
// 取消链路契约）。
func (s *AssetLibraryService) ExportAssetMetadata(ctx context.Context, libraryID string, opts model.AssetExportOptions) (model.AssetExportResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetExportResult{}, err
	}

	exportCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	entry := s.registerExportCancel(libraryID, cancel)
	defer s.unregisterExportCancel(libraryID, entry)
	// 在途计数（对齐 splitWG 形态）：还原链经 CancelExport + WaitExportsDone 排空
	// 在途导出后才换库文件，防半批行键失配产出混批 CSV（defer LIFO：Done 先于
	// unregisterExportCancel 执行，Wait 归零时注册表必已清空）
	s.exportWG.Add(1)
	defer s.exportWG.Done()
	formats, err := normalizeAssetExportFormats(opts.Formats)
	if err != nil {
		return model.AssetExportResult{}, err
	}
	pathMode := opts.PathMode
	if pathMode == "" {
		pathMode = model.AssetExportPathModeRelative
	}
	if pathMode != model.AssetExportPathModeRelative && pathMode != model.AssetExportPathModeAbsolute {
		return model.AssetExportResult{}, fmt.Errorf("路径口径非法（%q，须为 relative / absolute）", opts.PathMode)
	}
	for _, f := range formats {
		switch f {
		case model.AssetExportFormatCSV:
			if strings.TrimSpace(opts.CSVPath) == "" {
				return model.AssetExportResult{}, errors.New("CSV 导出目标文件路径不能为空")
			}
		case model.AssetExportFormatMD:
			if strings.TrimSpace(opts.MDDir) == "" {
				return model.AssetExportResult{}, errors.New("Markdown 导出目标目录不能为空")
			}
		}
	}

	// 覆盖预检（写任何字节前完成，确认型错误不落半成品）
	if !opts.Overwrite {
		for _, f := range formats {
			switch f {
			case model.AssetExportFormatCSV:
				if _, err := os.Stat(opts.CSVPath); err == nil {
					return model.AssetExportResult{}, assetExportTargetExistsError(opts.CSVPath)
				}
			case model.AssetExportFormatMD:
				if entries, err := os.ReadDir(opts.MDDir); err == nil && len(entries) > 0 {
					return model.AssetExportResult{}, assetExportTargetExistsError(opts.MDDir)
				}
			}
		}
	}

	keyword := strings.TrimSpace(opts.Keyword)
	ectx := &assetExportCtx{lib: lib, thumbsDir: s.assetThumbsDir(libraryID), pathMode: pathMode}

	// 第一遍短锁会话（范围解析 + 行键物化，纯 DB 读即开即关）：会话返回即释放
	// s.mu，万文件级导出不长时间持有 service 级全局锁
	var keys []assetExportRowKey
	err = s.withStore(libraryID, func(store *assetsStore) error {
		fileScope, slideScope, scopeErr := resolveAssetExportScope(store, keyword, opts)
		if scopeErr != nil {
			return scopeErr
		}
		keys, scopeErr = materializeAssetExportRowKeys(store, fileScope, slideScope)
		return scopeErr
	})
	if err != nil {
		return model.AssetExportResult{}, err
	}

	w, err := openAssetExportWriter(ectx, opts, formats)
	if err != nil {
		return model.AssetExportResult{}, err
	}

	// 行键分批水合（批大小复用 assetsQueryIDBatch），批间释放 s.mu：每批新开短
	// 会话只做 DB 水合，批内产物在锁外写汇——MD 卡文件写出与缩略图/拆分产物
	// 存在性探测（os.Stat）均不占锁。禁改批间 OFFSET 续读：批间释放锁后并发写
	// 插入会使 OFFSET 分页漂移，行键物化即为此而设（勿回退）
	skippedRows := 0
	for start := 0; start < len(keys); start += assetsQueryIDBatch {
		// 中止检查点（批间粒度，文件头「中止语义」段）：取消后下一批开始前收口——
		// w.abort() 删半成品 CSV（MD 已写卡保留，重导收敛），取消错误统一映射
		// E_ASSETS_CANCELLED
		if exportCtx.Err() != nil {
			_ = w.abort()
			return model.AssetExportResult{}, mapAssetExportCancelError(fmt.Errorf("导出已中止: %w", exportCtx.Err()))
		}
		end := start + assetsQueryIDBatch
		if end > len(keys) {
			end = len(keys)
		}
		var rows []*assetExportRow
		var miss int
		err = s.withStore(libraryID, func(store *assetsStore) error {
			var hydrateErr error
			rows, miss, hydrateErr = hydrateAssetExportBatchRows(store, keys[start:end])
			return hydrateErr
		})
		if err != nil {
			_ = w.abort()
			return model.AssetExportResult{}, err
		}
		for _, row := range rows {
			if writeErr := w.writeRow(row); writeErr != nil {
				_ = w.abort()
				return model.AssetExportResult{}, writeErr
			}
		}
		skippedRows += miss
	}

	if err := w.finish(); err != nil {
		_ = w.abort()
		return model.AssetExportResult{}, err
	}
	result := model.AssetExportResult{
		LibraryID: libraryID,
		Rows:      w.rows,
		MDFiles:   w.mdFiles,
	}
	for _, f := range formats {
		switch f {
		case model.AssetExportFormatCSV:
			result.CSVPath = opts.CSVPath
		case model.AssetExportFormatMD:
			result.MDDir = opts.MDDir
		}
	}
	Logger().Info("assets export done", "libraryId", libraryID, "rows", result.Rows,
		"mdFiles", result.MDFiles, "skippedRows", skippedRows,
		"csv", result.CSVPath, "mdDir", result.MDDir)
	return result, nil
}
