package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"workbench/model"
)

// 多源页合并导出（素材购物栏「合并导出」，基底注入式合并）。
//
// 输入为有序页清单（每项 = 源文件绝对路径 + 源展示页号），输出为单一 .pptx：
// 基底 = 清单第一项所在源文件，先跑集合化 GC（deleteSetForPages）只保留其被勾选
// 页；其余源文件逐个提取保留页闭包（页 → 版式 → 母版 → 主题 → 媒体）并经全局
// 分配器重命名后注入基底包。母版/版式/主题整链注入不合并 theme（等价 PowerPoint
// 「保留源格式」语义）；presentation.xml 为基底文件本体不注入——sldIdLst 按清单
// 顺序重建（基底页沿用原 rId 与数字 id，注入页追加分配），sldMasterIdLst 追加
// 注入母版条目；p:sldSz 恒取基底。
//
// 三重冲突消解（三个独立命名空间分别唯一）：
//  1. part 名：注入 part 经全局分配器消解（原名可用则保留，与基底/他源冲突时按
//     去尾数字递增找空位），part 的 .rels 随 part 同步改名；
//  2. presentation.xml.rels 的 rId：基底现有 Id 最大数字后缀 +1 起递增；
//  3. sldId@id（≥256）与 sldMasterId@id（≥2147483648）：各自取基底现有最大值 +1
//     递增，两个数字命名空间互不相干。
//
// MVP 边界（与 PRD Out of Scope 对齐）：备注页剔除（含基底保留页的备注页）、
// 嵌入字体剥离（presentation.xml 的 embeddedFontLst + fntdata part，字体缺失
// 机器回退系统字体）、尺寸混排不缩放（仅置 SldSzMixed 供前端提示）、跨页超链接
// 指向未勾选页的关系删除（拆分同语义）。
//
// XML 编辑一律「解析定位 + 字节删除/字节替换/字节拼接」（editXMLSpans/
// dropXMLSpans/appendXMLBeforeClosingTag），禁 Encoder 重序列化——presentation.xml
// 除被编辑区间外逐字节保留，注入 part 本体逐字节拷贝（仅其 .rels 按映射重写后
// 整体再生：rels 是纯关系表、字段全集固定为 Id/Type/Target/TargetMode，再生
// 无损且规避属性级字节手术）。重建的列表条目沿用文档既有命名空间前缀写法。
// OPC 关系 Target 相对关系源 part 所在目录解析（pptx-split.md 踩坑先例）。

// 合并导出生成的关系类型（presentation.xml.rels 追加条目用）。
const (
	pptxSlideRelType       = pptxRelsNamespace + "/slide"
	pptxSlideMasterRelType = pptxRelsNamespace + "/slideMaster"
	// 嵌入字体关系类型后缀：presentation.xml.rels 中以该后缀结尾的内部关系指向
	// fntdata part，合并 MVP 整体剥离。
	pptxFontRelTypeSuffix = "/font"
)

// MergePptxPages 把有序页清单合并导出为单一 pptx（合并核心，供服务编排层调用）。
//
// 参数语义：
//   - items 为购物栏顺序的页清单（输出页序与其一致）；(源文件, 页号) 重复项保留
//     首次出现、后续项跳过并在结果中报告；
//   - outPath 为输出文件绝对路径，输出目录不存在时自动创建，禁与任何源文件同
//     路径（防原地覆盖损坏源文件）；
//   - onProgress 在每个源文件处理完成后回调（done 为已就绪页数、total 为清单
//     有效页总数），可为 nil。
//
// 跳过语义：非基底源文件打不开/损坏/页号越界只跳过对应项继续导出（结果
// SkippedItems 报告）；基底文件（清单第一项）打不开为整体失败；跳过后无有效页
// 时报引导错误。取消口径与拆分一致（ctx 取消错误 %w 包装 context.Canceled，
// errors.Is 判定可用）；输出为原子写（writeZipAtomic），失败不留半态。
func MergePptxPages(ctx context.Context, items []model.PptxMergeItem, outPath string, onProgress func(done, total int)) (model.PptxMergeResult, error) {
	if err := ctx.Err(); err != nil {
		return model.PptxMergeResult{}, fmt.Errorf("合并导出已取消: %w", err)
	}
	if len(items) == 0 {
		return model.PptxMergeResult{}, errors.New("请先选择要合并导出的页")
	}
	if strings.TrimSpace(outPath) == "" {
		return model.PptxMergeResult{}, errors.New("请先选择导出保存位置")
	}

	// 清单分组与去重：按规范化源路径分组（同文件不同路径写法命中同组），组间
	// 保持首次出现顺序；(源文件, 页号) 重复项与非法项跳过并报告
	type mergeGroup struct {
		srcPath string // 首次出现时的原始路径写法（结果回显用）
		items   []model.PptxMergeItem
	}
	var groups []*mergeGroup
	groupByKey := make(map[string]*mergeGroup)
	seenPage := make(map[string]bool) // 规范化路径 + 页号
	var skipped []model.PptxMergeSkipped
	var cartOrder []model.PptxMergeItem // 去重后的原始购物栏顺序（输出页序权威）
	for _, item := range items {
		if strings.TrimSpace(item.SrcPath) == "" {
			skipped = append(skipped, model.PptxMergeSkipped{PageNum: item.PageNum, Reason: "源文件路径为空"})
			continue
		}
		if item.PageNum < 1 {
			skipped = append(skipped, model.PptxMergeSkipped{SrcPath: item.SrcPath, PageNum: item.PageNum, Reason: "页码无效（须为 1 起展示页序）"})
			continue
		}
		key := normalizeRepoKey(item.SrcPath)
		pageKey := key + "#" + strconv.Itoa(item.PageNum)
		if seenPage[pageKey] {
			skipped = append(skipped, model.PptxMergeSkipped{SrcPath: item.SrcPath, PageNum: item.PageNum, Reason: "该页已在导出清单中（重复项跳过）"})
			continue
		}
		seenPage[pageKey] = true
		cartOrder = append(cartOrder, item)
		g, ok := groupByKey[key]
		if !ok {
			g = &mergeGroup{srcPath: item.SrcPath}
			groupByKey[key] = g
			groups = append(groups, g)
		}
		g.items = append(g.items, item)
	}
	if len(groups) == 0 {
		return model.PptxMergeResult{}, errors.New("清单中没有可导出的有效页")
	}
	for _, g := range groups {
		if subsetSamePath(g.srcPath, outPath) {
			return model.PptxMergeResult{}, errors.New("导出路径不能与源文件相同")
		}
	}

	// 逐源加载与页号校验：基底（第一组）失败 = 整体失败；其余源失败跳过该组
	type loadedGroup struct {
		src  *mergeSource
		kept []mergeKeptPage // 该组保留页（清单顺序）
	}
	loaded := make([]*loadedGroup, 0, len(groups))
	for gi, g := range groups {
		if err := ctx.Err(); err != nil {
			return model.PptxMergeResult{}, fmt.Errorf("合并导出已取消: %w", err)
		}
		src, err := loadMergeSource(g.srcPath)
		if err != nil {
			if gi == 0 {
				return model.PptxMergeResult{}, err
			}
			for _, item := range g.items {
				skipped = append(skipped, model.PptxMergeSkipped{SrcPath: item.SrcPath, PageNum: item.PageNum, Reason: err.Error()})
			}
			continue
		}
		lg := &loadedGroup{src: src}
		for _, item := range g.items {
			if item.PageNum > len(src.pages) {
				skipped = append(skipped, model.PptxMergeSkipped{SrcPath: item.SrcPath, PageNum: item.PageNum,
					Reason: fmt.Sprintf("页码 %d 超出范围（该演示文稿共 %d 页）", item.PageNum, len(src.pages))})
				continue
			}
			lg.kept = append(lg.kept, mergeKeptPage{page: src.pages[item.PageNum-1], srcPage: item.PageNum})
		}
		loaded = append(loaded, lg)
	}
	totalKept := 0
	for _, lg := range loaded {
		totalKept += len(lg.kept)
	}
	if totalKept == 0 {
		return model.PptxMergeResult{}, errors.New("清单中没有可导出的有效页")
	}
	reportProgress := func(done int) {
		if onProgress != nil {
			onProgress(done, totalKept)
		}
	}

	// 尺寸混排检测：p:sldSz 恒取基底，任一源与基底不一致仅置标志供前端提示
	base := loaded[0]
	sldSzMixed := false
	for _, lg := range loaded[1:] {
		if lg.src.sldSzKey != base.src.sldSzKey {
			sldSzMixed = true
			break
		}
	}

	// ---------- 基底处理：集合化 GC + 备注页/嵌入字体剥离 + 基底 part 编辑 ----------
	baseKeptPages := make([]pptxPage, 0, len(base.kept))
	for _, kp := range base.kept {
		baseKeptPages = append(baseKeptPages, kp.page)
	}
	basePlan, baseEdited, err := buildBaseMergePlan(base.src, baseKeptPages)
	if err != nil {
		return model.PptxMergeResult{}, err
	}
	deleted := basePlan.deleted
	reportProgress(len(baseKeptPages))

	// ---------- 注入处理：逐源闭包提取 + 重命名 + rels 重写 ----------
	allocator := newMergeNameAllocator(base.src.pkg)
	baseCT := parsePptxContentTypes(base.src.pkg)
	nextRid := nextRelIDAllocator(parsePptxRelIDs(base.src.pkg))
	injections := make([]*mergeInjection, 0, len(loaded)-1)
	done := len(baseKeptPages)
	for _, lg := range loaded[1:] {
		if err := ctx.Err(); err != nil {
			return model.PptxMergeResult{}, fmt.Errorf("合并导出已取消: %w", err)
		}
		inj, err := injectMergeSource(lg.src, lg.kept, allocator, nextRid, baseCT)
		if err != nil {
			return model.PptxMergeResult{}, err
		}
		injections = append(injections, inj)
		done += len(lg.kept)
		reportProgress(done)
	}

	// ---------- 汇总：输出页序（购物栏序）+ 各命名空间分配 ----------
	// 页引用登记：(规范化源路径, 页号) → 注入后引用。基底页沿用原 rId。
	refByKey := make(map[string]map[int]mergePageRef)
	registerRef := func(srcPath string, ref mergePageRef) {
		key := normalizeRepoKey(srcPath)
		m := refByKey[key]
		if m == nil {
			m = make(map[int]mergePageRef)
			refByKey[key] = m
		}
		m[ref.srcPage] = ref
	}
	for _, kp := range base.kept {
		registerRef(base.src.path, mergePageRef{rid: kp.page.rid, srcPath: base.src.path, srcPage: kp.srcPage, isBase: true})
	}
	for _, inj := range injections {
		for _, ref := range inj.slideRefs {
			registerRef(ref.srcPath, ref)
		}
	}
	// 按购物栏原始顺序回放页清单（cartOrder = 去重后的清单顺序；失败源/越界页无
	// 登记自然跳过）
	var outputPages []mergePageRef
	for _, item := range cartOrder {
		m := refByKey[normalizeRepoKey(item.SrcPath)]
		if m == nil {
			continue
		}
		if ref, ok := m[item.PageNum]; ok {
			outputPages = append(outputPages, ref)
		}
	}

	// ---------- 基底 presentation.xml 列表条目解析与命名空间分配 ----------
	// 数字 id（sldId@id / sldMasterId@id）与关系 Id（r:id）按属性命名空间精确区分
	// 解析（parsePptxListIDs；结构体标签 xml:"id,attr" 恒命中 r:id，不可用）
	presEntry, ok := base.src.pkg.byName[pptxPresentationPart]
	if !ok {
		return model.PptxMergeResult{}, fmt.Errorf("缺少 %s", pptxPresentationPart)
	}
	baseSldIDs, baseMasterIDs, err := parsePptxListIDs(presEntry.data)
	if err != nil {
		return model.PptxMergeResult{}, fmt.Errorf("解析 %s 失败: %w", pptxPresentationPart, err)
	}
	baseSldIDByRid := make(map[string]int, len(baseSldIDs))
	sldIDTaken := make(map[int]bool, len(baseSldIDs))
	for _, e := range baseSldIDs {
		if e.RID != "" {
			baseSldIDByRid[e.RID] = e.NumID
		}
		if e.NumID > 0 {
			sldIDTaken[e.NumID] = true
		}
	}
	nextSldID := nextIntAllocator(sldIDTaken, 256)
	for i := range outputPages {
		p := &outputPages[i]
		if p.isBase {
			p.id = baseSldIDByRid[p.rid]
			if p.id <= 0 {
				p.id = nextSldID() // 数字 id 缺失/不可解析形态：防御性分配
			}
			continue
		}
		p.id = nextSldID()
	}

	// 注入母版关系（rId 追加分配）与 sldMasterId 数字 id 分配（取基底现有最大值 +1，
	// 与基底条目不冲突）
	masterIDTaken := make(map[int]bool, len(baseMasterIDs))
	for _, e := range baseMasterIDs {
		if e.NumID > 0 {
			masterIDTaken[e.NumID] = true
		}
	}
	nextMasterID := nextIntAllocator(masterIDTaken, 2147483648)
	var injectedMasterNames, injectedMasterRids []string
	var injectedMasterIDs []int
	var masterRelsAppend strings.Builder
	for _, inj := range injections {
		for _, masterNewName := range inj.masters {
			rid := nextRid()
			injectedMasterNames = append(injectedMasterNames, masterNewName)
			injectedMasterRids = append(injectedMasterRids, rid)
			injectedMasterIDs = append(injectedMasterIDs, nextMasterID())
			fmt.Fprintf(&masterRelsAppend, `<Relationship Id="%s" Type="%s" Target="%s"/>`,
				escXMLAttr(rid), pptxSlideMasterRelType, escXMLAttr(relTargetBetween(pptxPresentationPart, masterNewName)))
		}
	}
	var slideRelsAppend strings.Builder
	for _, inj := range injections {
		slideRelsAppend.WriteString(inj.relsAppend)
	}

	// ---------- 基底 presentation.xml 重建 ----------
	// 单遍字节级编辑：剥离 custShowLst/extLst/embeddedFontLst，sldIdLst 与
	// sldMasterIdLst 整体重建为输出顺序（起始标签含命名空间声明逐字节保留），
	// 其余区间（sldSz 等）逐字节不变。
	presData, err := editXMLSpans(presEntry.data, func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr, span []byte) ([]byte, bool) {
		if span == nil {
			// StartElement 探测阶段：仅确认编辑意图，替换字节在 EndElement 阶段
			// 按完整 span 一次性构建（回调同函数双阶段调用）
			switch name.Local {
			case "custShowLst", "extLst", "embeddedFontLst", "sldIdLst", "sldMasterIdLst":
				return nil, depth == 1
			}
			return nil, false
		}
		if depth != 1 {
			return nil, false
		}
		switch name.Local {
		case "custShowLst", "extLst", "embeddedFontLst":
			return nil, true
		case "sldIdLst":
			prefix := xmlListPrefix(span, "sldIdLst")
			var b strings.Builder
			for _, p := range outputPages {
				fmt.Fprintf(&b, `<%s:sldId id="%d" r:id="%s"/>`, prefix, p.id, p.rid)
			}
			return rebuildXMLListSpan(span, "sldIdLst", b.String()), true
		case "sldMasterIdLst":
			prefix := xmlListPrefix(span, "sldMasterIdLst")
			var b strings.Builder
			for _, e := range baseMasterIDs {
				// GC 生效：仅保留母版条目；降级形态全保留（与拆分口径一致）
				if len(basePlan.keptMasterRids) > 0 && !basePlan.keptMasterRids[e.RID] {
					continue
				}
				id := e.NumID
				if id <= 0 {
					id = nextMasterID() // 数字 id 缺失/不可解析形态：防御性分配
				}
				fmt.Fprintf(&b, `<%s:sldMasterId id="%d" r:id="%s"/>`, prefix, id, e.RID)
			}
			for i := range injectedMasterNames {
				fmt.Fprintf(&b, `<%s:sldMasterId id="%d" r:id="%s"/>`, prefix, injectedMasterIDs[i], injectedMasterRids[i])
			}
			return rebuildXMLListSpan(span, "sldMasterIdLst", b.String()), true
		}
		return nil, false
	})
	if err != nil {
		return model.PptxMergeResult{}, fmt.Errorf("编辑 %s 失败: %w", pptxPresentationPart, err)
	}

	// presentation.xml.rels：GC/清理结果上追加注入母版与注入页关系
	presRelsData, ok := baseEdited[pptxPresentationRelsPart]
	if !ok {
		return model.PptxMergeResult{}, fmt.Errorf("缺少 %s", pptxPresentationRelsPart)
	}
	presRelsData, err = appendXMLBeforeClosingTag(presRelsData, "</Relationships>", masterRelsAppend.String()+slideRelsAppend.String())
	if err != nil {
		return model.PptxMergeResult{}, fmt.Errorf("编辑 %s 失败: %w", pptxPresentationRelsPart, err)
	}

	// [Content_Types].xml：追加注入源的缺失扩展名 Default 与注入 part 的 Override
	var ctAppend strings.Builder
	for _, inj := range injections {
		ctAppend.WriteString(inj.ctAppend)
	}
	ctEdited := []byte(nil)
	if ctSource, ok := baseEdited[pptxContentTypesPart]; ok && ctAppend.Len() > 0 {
		ctEdited, err = appendXMLBeforeClosingTag(ctSource, "</Types>", ctAppend.String())
		if err != nil {
			return model.PptxMergeResult{}, fmt.Errorf("编辑 %s 失败: %w", pptxContentTypesPart, err)
		}
	}

	// ---------- 输出组装：基底条目（原序、跳删除集、套编辑）+ 注入条目 ----------
	entries := make([]zipEntry, 0, len(base.src.pkg.entries))
	for i := range base.src.pkg.entries {
		e := base.src.pkg.entries[i]
		if deleted[e.name] {
			continue
		}
		if ed, ok := baseEdited[e.name]; ok {
			e.data = ed
		}
		switch e.name {
		case pptxPresentationPart:
			e.data = presData
		case pptxPresentationRelsPart:
			e.data = presRelsData
		case pptxContentTypesPart:
			if ctEdited != nil {
				e.data = ctEdited
			}
		}
		entries = append(entries, e)
	}
	for _, inj := range injections {
		entries = append(entries, inj.entries...)
	}

	if err := ctx.Err(); err != nil {
		return model.PptxMergeResult{}, fmt.Errorf("合并导出已取消: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return model.PptxMergeResult{}, fmt.Errorf("创建输出目录失败: %w", err)
	}
	if err := writeZipAtomic(entries, outPath); err != nil {
		return model.PptxMergeResult{}, err
	}

	result := model.PptxMergeResult{OutputPath: outPath, SldSzMixed: sldSzMixed, SkippedItems: skipped}
	for i, p := range outputPages {
		result.Pages = append(result.Pages, model.PptxMergedPage{SrcPath: p.srcPath, SrcPageNum: p.srcPage, OutPageNum: i + 1})
	}
	result.PageCount = len(result.Pages)
	return result, nil
}

// ---------- 源文件加载 ----------

// mergeSource 单个合并源文件的内存视图。
type mergeSource struct {
	path     string
	pkg      *pptxPackage
	pages    []pptxPage        // 三跳映射后的展示页序
	notes    map[string]string // 页 part → 备注页 part
	sldSzKey string            // p:sldSz「cxCy」归一比较键（尺寸混排检测）
}

// pptxSldSize p:sldSz 的属性视图。
type pptxSldSize struct {
	Cx string `xml:"cx,attr"`
	Cy string `xml:"cy,attr"`
}

// pptxSldSizeMeta presentation.xml 的 p:sldSz 最小解析视图。
type pptxSldSizeMeta struct {
	SldSz pptxSldSize `xml:"sldSz"`
}

// loadMergeSource 读取并解析单个源文件（扩展名/存在性/zip/三跳映射/备注页/尺寸）。
// 错误消息即用户可读引导（基底失败整体上抛，非基底失败入 SkippedItems.Reason）。
func loadMergeSource(srcPath string) (*mergeSource, error) {
	if !strings.EqualFold(filepath.Ext(srcPath), ".pptx") {
		return nil, fmt.Errorf("仅支持 .pptx 格式，当前文件：%s", filepath.Base(srcPath))
	}
	if _, err := os.Stat(srcPath); err != nil {
		return nil, fmt.Errorf("无法访问 PPT 文件: %w", err)
	}
	zr, err := zip.OpenReader(srcPath)
	if err != nil {
		// 加密 pptx 是 OLE 容器而非 zip，与损坏文件走同一引导
		return nil, fmt.Errorf("无法读取 PPT 文件（文件可能已损坏或已加密）: %w", err)
	}
	defer zr.Close()
	pkg, err := readZipPackage(&zr.Reader)
	if err != nil {
		return nil, fmt.Errorf("读取 PPT 内部数据失败: %w", err)
	}
	pages, err := resolveSlidePages(pkg)
	if err != nil {
		return nil, err
	}
	notes, err := resolveNotesSlides(pkg, pages)
	if err != nil {
		return nil, err
	}
	var sldSz pptxSldSizeMeta
	_ = parsePartXML(pkg, pptxPresentationPart, &sldSz) // 尺寸缺失不阻塞（比较键退化为空）
	return &mergeSource{path: srcPath, pkg: pkg, pages: pages, notes: notes, sldSzKey: sldSz.SldSz.Cx + "x" + sldSz.SldSz.Cy}, nil
}

// mergeKeptPage 已解析的清单保留页：页对象 + 源展示页号（结果回显用）。
type mergeKeptPage struct {
	page    pptxPage
	srcPage int
}

// ---------- 基底编辑计划 ----------

// buildBaseMergePlan 计算基底包的合并编辑：集合化 GC（保留基底被勾选页集合，
// 与 PR1 子集导出共用删除集计算）+ 合并特有删除面扩充（备注页剔除、嵌入字体
// 剥离）+ 基底 part 的字节级编辑。返回 GC 计划（sldMasterIdLst 剪枝与母版联动
// 编辑定位）与「part 名 → 编辑后字节」映射；未列出的保留 part 逐字节拷贝。
// presentation.xml 本体（sldIdLst/sldMasterIdLst 重建）由调用方单独处理。
func buildBaseMergePlan(src *mergeSource, kept []pptxPage) (splitGCPlan, map[string][]byte, error) {
	gc, err := newSplitGCContext(src.pkg)
	if err != nil {
		return splitGCPlan{}, nil, err
	}
	// notesOf 传 nil：基底备注页剔除由下方删除面扩充完成（GC 闭包先按拆分口径
	// 计算——保留页备注页经其 rels 可达会被闭包含入——再统一并入删除集，保留页
	// rels 的备注关系由下方悬空清理一并移除）
	plan := gc.deleteSetForPages(src.pkg, src.pages, nil, kept)
	deleted := plan.deleted
	// 备注页剔除（合并 MVP 边界）：保留页的备注页随输出剥离
	for _, page := range kept {
		if notes, ok := src.notes[page.part]; ok {
			deleted[notes] = true
			deleted[relsPartOf(notes)] = true
		}
	}
	// 嵌入字体剥离：presentation.xml.rels 中内部字体关系指向的 fntdata part
	if entry, ok := src.pkg.byName[pptxPresentationRelsPart]; ok {
		var rels pptxRelationships
		if err := xml.Unmarshal(entry.data, &rels); err == nil {
			for _, rel := range rels.Relationships {
				if rel.Target == "" || strings.EqualFold(rel.TargetMode, "External") {
					continue
				}
				if strings.HasSuffix(rel.Type, pptxFontRelTypeSuffix) {
					target := resolveRelTarget(pptxPresentationPart, rel.Target)
					deleted[target] = true
					deleted[relsPartOf(target)] = true
				}
			}
		}
	}

	edited := make(map[string][]byte, 6)
	edit := func(name string, predicate func(int, []string, xml.Name, []xml.Attr) bool) error {
		entry, ok := src.pkg.byName[name]
		if !ok {
			return nil // part 缺失（无该文件形态）时无编辑面，其余 part 原样拷贝
		}
		data, err := dropXMLSpans(entry.data, predicate)
		if err != nil {
			return fmt.Errorf("编辑 %s 失败: %w", name, err)
		}
		edited[name] = data
		return nil
	}
	// presentation.xml.rels：Target 落在删除集的关系移除（未勾选页/备注页/被剪
	// 母版/嵌入字体）；注入关系由调用方在该结果上追加
	if err := edit(pptxPresentationRelsPart, danglingRelsDropPredicate(pptxPresentationPart, deleted)); err != nil {
		return splitGCPlan{}, nil, err
	}
	// 保留页 rels：指向未勾选页/备注页的跳转关系移除（拆分同语义）
	for _, page := range kept {
		if err := edit(relsPartOf(page.part), danglingRelsDropPredicate(page.part, deleted)); err != nil {
			return splitGCPlan{}, nil, err
		}
	}
	// 保留母版 rels 与 XML sldLayoutIdLst 剪枝（GC 生效时；降级形态零编辑面）
	for masterPart := range plan.keptMasterParts {
		if err := edit(relsPartOf(masterPart), danglingRelsDropPredicate(masterPart, deleted)); err != nil {
			return splitGCPlan{}, nil, err
		}
	}
	for masterPart, dropRids := range plan.keptMasterDroppedLayoutRids {
		if len(dropRids) == 0 {
			continue
		}
		if err := edit(masterPart, masterLayoutIdDropPredicate(dropRids)); err != nil {
			return splitGCPlan{}, nil, err
		}
	}
	// [Content_Types].xml：已删 part 的 Override 移除（注入追加由调用方处理）
	if err := edit(pptxContentTypesPart, overrideDropPredicate(deleted)); err != nil {
		return splitGCPlan{}, nil, err
	}
	return plan, edited, nil
}

// ---------- 注入层 ----------

// mergePageRef 输出 presentation.xml sldIdLst 单条目的数据来源。
type mergePageRef struct {
	rid     string // 输出 presentation.xml.rels 中的关系 Id（基底页 = 原 rId）
	id      int    // sldId 数字 id（基底页 = 原 id；注入页 = 分配）
	srcPath string // 源文件路径（首次出现写法，结果回显）
	srcPage int    // 源展示页号
	isBase  bool
}

// mergeInjection 单个注入源的产物。
type mergeInjection struct {
	slideRefs  []mergePageRef // 注入页引用（清单顺序，rid 已分配、数字 id 由调用方统一分配）
	masters    []string       // 注入母版新 part 名（首现顺序）
	relsAppend string         // presentation.xml.rels 追加的注入页 Relationship 条目
	// ctAppend [Content_Types].xml 追加行：Override 与缺失扩展名 Default 按条目
	// 顺序交错产出（OPC CT_Types 为无序 choice，元素顺序合法）
	ctAppend string
	entries  []zipEntry // 注入条目（新名；part 本体逐字节拷贝、rels 重写后再生）
}

// injectMergeSource 提取非基底源文件的保留页闭包并注入基底包：
//  1. 闭包 = 全部保留页出发的 rels 图可达集（未勾选页与全部备注页为屏障——指向
//     未勾选页的跨页跳转关系随屏障删除，备注页按 MVP 边界剔除；保留母版的全部
//     版式经母版 rels 可达一并注入，避免对注入母版本体做 sldLayoutIdLst 手术）；
//  2. part 名经全局分配器消解冲突（.rels 随 part 同步改名）；
//  3. 注入 part 的 rels 逐关系重写 Target（命中映射按新 part 名换算相对路径；
//     未命中 = 屏障目标或悬空，关系删除）；
//  4. 内容类型照搬源包声明：Override 优先，否则按扩展名补 Default（基底缺失该
//     扩展名时）。
func injectMergeSource(src *mergeSource, kept []mergeKeptPage, allocator *mergeNameAllocator, nextRid func() string, baseCT *pptxContentTypesMeta) (*mergeInjection, error) {
	graph := buildPptxRelsGraph(src.pkg)
	// 屏障：未勾选页（+rels）与全部备注页（+rels）
	keptByPart := make(map[string]bool, len(kept))
	for _, kp := range kept {
		keptByPart[kp.page.part] = true
	}
	barrier := make(map[string]bool, len(src.pages)*2+len(src.notes)*2)
	for _, page := range src.pages {
		if !keptByPart[page.part] {
			barrier[page.part] = true
			barrier[relsPartOf(page.part)] = true
		}
	}
	for _, notes := range src.notes {
		barrier[notes] = true
		barrier[relsPartOf(notes)] = true
	}
	// 可达性闭包（BFS）：根 = 保留页；rels 随所属 part 保留
	keep := make(map[string]bool, len(src.pkg.entries))
	queue := make([]string, 0, len(kept))
	for _, kp := range kept {
		queue = append(queue, kp.page.part)
	}
	for len(queue) > 0 {
		part := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if keep[part] {
			continue
		}
		keep[part] = true
		if rp := relsPartOf(part); src.pkg.byName[rp] != nil {
			keep[rp] = true
		}
		for _, e := range graph[part] {
			if !barrier[e.target] && !keep[e.target] {
				queue = append(queue, e.target)
			}
		}
	}

	// part 名映射（按源 zip 条目顺序分配，输出顺序稳定；.rels 随 part 另行改名）
	mapping := make(map[string]string, len(src.pkg.entries))
	for i := range src.pkg.entries {
		name := src.pkg.entries[i].name
		if !keep[name] || strings.HasSuffix(name, ".rels") {
			continue
		}
		mapping[name] = allocator.allocate(name)
	}

	// 注入 rels 重写：Target 按映射换算；屏障目标/悬空的关系删除
	rewrittenRels := make(map[string][]byte, len(mapping))
	for origPart, newPart := range mapping {
		relsName := relsPartOf(origPart)
		entry, ok := src.pkg.byName[relsName]
		if !ok {
			continue
		}
		var rels pptxRelationships
		if err := xml.Unmarshal(entry.data, &rels); err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", relsName, err)
		}
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n")
		b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
		for _, rel := range rels.Relationships {
			if rel.Target == "" || strings.EqualFold(rel.TargetMode, "External") {
				// 外部超链接（URL）与包内部件无关，原样保留
				if rel.TargetMode != "" {
					fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s" Target="%s" TargetMode="%s"/>`,
						escXMLAttr(rel.ID), escXMLAttr(rel.Type), escXMLAttr(rel.Target), escXMLAttr(rel.TargetMode))
				} else {
					fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s" Target="%s"/>`,
						escXMLAttr(rel.ID), escXMLAttr(rel.Type), escXMLAttr(rel.Target))
				}
				continue
			}
			newTarget, ok := mapping[resolveRelTarget(origPart, rel.Target)]
			if !ok {
				continue // 屏障目标（未勾选页/备注页）或悬空：关系删除（拆分同语义）
			}
			fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s" Target="%s"/>`,
				escXMLAttr(rel.ID), escXMLAttr(rel.Type), escXMLAttr(relTargetBetween(newPart, newTarget)))
		}
		b.WriteString(`</Relationships>`)
		rewrittenRels[relsName] = []byte(b.String())
	}

	// 内容类型：Override 优先，否则按扩展名补 Default（基底缺失该扩展名时）
	srcCT := parsePptxContentTypes(src.pkg)
	var ctAppend strings.Builder
	addedDefaults := make(map[string]bool)
	for i := range src.pkg.entries {
		orig := src.pkg.entries[i].name
		newPart, ok := mapping[orig]
		if !ok {
			continue
		}
		if ct, ok := srcCT.overrides[orig]; ok && ct != "" {
			fmt.Fprintf(&ctAppend, `<Override PartName="/%s" ContentType="%s"/>`, escXMLAttr(newPart), escXMLAttr(ct))
			continue
		}
		// 扩展名小写归一：同扩展名大小写异形的多个 part 只补一条 Default，且
		// 发出的 Extension 归一小写（与 OPC Default 匹配惯例及 baseCT 查表口径一致）
		ext := strings.ToLower(strings.TrimPrefix(path.Ext(orig), "."))
		if ext == "" || addedDefaults[ext] || baseCT.defaults[ext] != "" {
			continue
		}
		if ct, ok := srcCT.defaults[ext]; ok && ct != "" {
			addedDefaults[ext] = true
			fmt.Fprintf(&ctAppend, `<Default Extension="%s" ContentType="%s"/>`, escXMLAttr(ext), escXMLAttr(ct))
		}
	}

	// 注入母版定位：保留页 → 版式 → 母版（闭包内必可达），首现顺序
	mastersSeen := make(map[string]bool)
	var masters []string
	for _, kp := range kept {
		layout := firstRelTargetInGraph(graph, kp.page.part, pptxSlideLayoutRelTypeSuffix)
		master := firstRelTargetInGraph(graph, layout, pptxSlideMasterRelTypeSuffix)
		if master == "" {
			continue
		}
		newMaster, ok := mapping[master]
		if !ok || mastersSeen[master] {
			continue
		}
		mastersSeen[master] = true
		masters = append(masters, newMaster)
	}

	// 注入页引用与 presentation.xml.rels 追加条目（rId 由调用方分配器分配）
	var slideRefs []mergePageRef
	var relsAppend strings.Builder
	for _, kp := range kept {
		newPart := mapping[kp.page.part]
		rid := nextRid()
		slideRefs = append(slideRefs, mergePageRef{rid: rid, srcPath: src.path, srcPage: kp.srcPage})
		fmt.Fprintf(&relsAppend, `<Relationship Id="%s" Type="%s" Target="%s"/>`,
			escXMLAttr(rid), pptxSlideRelType, escXMLAttr(relTargetBetween(pptxPresentationPart, newPart)))
	}

	// 注入条目组装：part 本体在前（源顺序），重写后 rels 在后（新名随 part）
	entries := make([]zipEntry, 0, len(src.pkg.entries))
	for i := range src.pkg.entries {
		e := &src.pkg.entries[i]
		if strings.HasSuffix(e.name, ".rels") {
			continue
		}
		newName, ok := mapping[e.name]
		if !ok {
			continue
		}
		entries = append(entries, zipEntry{name: newName, method: e.method, modified: e.modified, data: e.data})
	}
	for i := range src.pkg.entries {
		e := &src.pkg.entries[i]
		if !strings.HasSuffix(e.name, ".rels") {
			continue
		}
		newSource, ok := mapping[relsSourcePart(e.name)]
		if !ok {
			continue
		}
		data, ok := rewrittenRels[e.name]
		if !ok {
			continue
		}
		entries = append(entries, zipEntry{name: relsPartOf(newSource), method: e.method, modified: e.modified, data: data})
	}

	return &mergeInjection{slideRefs: slideRefs, masters: masters, relsAppend: relsAppend.String(), ctAppend: ctAppend.String(), entries: entries}, nil
}

// mergeNameAllocator 注入 part 名全局分配器：以基底包全部条目名为已占用基线，
// 跨注入源共享同一实例保证互不冲突。原名可用则原样保留（跨包同名仅在与基底或
// 他源碰撞时发生），冲突时按「去尾数字递增」找空位（slide1.xml → slideN.xml），
// 并同时保证 part 名与其派生 .rels 名均不冲突。
type mergeNameAllocator struct {
	used map[string]bool
}

func newMergeNameAllocator(base *pptxPackage) *mergeNameAllocator {
	used := make(map[string]bool, len(base.entries)*2)
	for i := range base.entries {
		used[base.entries[i].name] = true
	}
	return &mergeNameAllocator{used: used}
}

func (a *mergeNameAllocator) allocate(orig string) string {
	take := func(name string) {
		a.used[name] = true
		a.used[relsPartOf(name)] = true
	}
	if !a.used[orig] && !a.used[relsPartOf(orig)] {
		take(orig)
		return orig
	}
	dir, base := path.Split(orig)
	ext := path.Ext(base)
	stem := base[:len(base)-len(ext)]
	i := len(stem)
	for i > 0 && stem[i-1] >= '0' && stem[i-1] <= '9' {
		i--
	}
	prefix := stem[:i]
	n := 0
	if i < len(stem) {
		n, _ = strconv.Atoi(stem[i:])
	}
	for {
		n++
		candidate := dir + prefix + strconv.Itoa(n) + ext
		if !a.used[candidate] && !a.used[relsPartOf(candidate)] {
			take(candidate)
			return candidate
		}
	}
}

// ---------- 命名空间分配器与查表 ----------

// parsePptxRelIDs 基底 presentation.xml.rels 全部关系 Id 集合（rId 分配冲突面）。
func parsePptxRelIDs(pkg *pptxPackage) map[string]bool {
	ids := make(map[string]bool)
	entry, ok := pkg.byName[pptxPresentationRelsPart]
	if !ok {
		return ids
	}
	var rels pptxRelationships
	if err := xml.Unmarshal(entry.data, &rels); err != nil {
		return ids
	}
	for _, rel := range rels.Relationships {
		ids[rel.ID] = true
	}
	return ids
}

// relIDNumberRe rId 形态（rId + 纯数字）识别，分配起点取现有最大数字后缀。
var relIDNumberRe = regexp.MustCompile(`^rId(\d+)$`)

// nextRelIDAllocator 关系 Id 分配器：起点 = 现有 Id 最大数字后缀 +1，生成 "rIdN"
// 形态并跳过既有 Id（含非数字形态 Id 的占位冲突）。
func nextRelIDAllocator(used map[string]bool) func() string {
	n := 0
	for id := range used {
		if m := relIDNumberRe.FindStringSubmatch(id); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil && v > n {
				n = v
			}
		}
	}
	return func() string {
		for {
			n++
			id := fmt.Sprintf("rId%d", n)
			if !used[id] {
				used[id] = true
				return id
			}
		}
	}
}

// pptxListIDEntry sldIdLst / sldMasterIdLst 条目的数字 id 与关系 Id 对。
type pptxListIDEntry struct {
	NumID int    // 数字 id（无命名空间前缀的 id 属性；缺失/不可解析 = 0）
	RID   string // 关系 Id（relationships 命名空间的 id 属性）
}

// parsePptxListIDs 解析 presentation.xml 中 sldIdLst 与 sldMasterIdLst 全部条目的
// 数字 id 与关系 Id。实现说明：encoding/xml 的结构体标签 `xml:"id,attr"` 对任意
// 命名空间的同名属性都命中且后出现者覆盖前者——sldId 条目同时带无前缀 id 与
// r:id 两个属性，标签解析恒取 r:id 值（实证结论，既有 pptxSldIdEntry.ID 字段
// 即存在该潜在缺陷但从未被消费），数字 id 与关系 Id 的区分必须按属性命名空间
// 精确解析；depth 口径与 dropXMLSpans 一致（根元素 0），节信息等深层同局部名
// 元素由 depth==2 限制天然隔离。
func parsePptxListIDs(data []byte) (sldIDs, masterIDs []pptxListIDEntry, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	list := "" // 当前所在列表元素局部名
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return sldIDs, masterIDs, nil
		}
		if err != nil {
			return nil, nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case depth == 1 && t.Name.Local == "sldIdLst":
				list = t.Name.Local
			case depth == 1 && t.Name.Local == "sldMasterIdLst":
				list = t.Name.Local
			case depth == 2 && list == "sldIdLst" && t.Name.Local == "sldId":
				sldIDs = append(sldIDs, listIDEntryFromAttrs(t.Attr))
			case depth == 2 && list == "sldMasterIdLst" && t.Name.Local == "sldMasterId":
				masterIDs = append(masterIDs, listIDEntryFromAttrs(t.Attr))
			}
			depth++
		case xml.EndElement:
			depth--
			if depth <= 1 {
				list = ""
			}
		}
	}
}

// listIDEntryFromAttrs 从条目属性中提取数字 id（Space 为空）与关系 Id
// （Space 为 relationships 命名空间）。
func listIDEntryFromAttrs(attrs []xml.Attr) pptxListIDEntry {
	var e pptxListIDEntry
	for _, a := range attrs {
		switch {
		case a.Name.Space == "" && a.Name.Local == "id":
			if v, err := strconv.Atoi(a.Value); err == nil {
				e.NumID = v
			}
		case a.Name.Space == pptxRelsNamespace && a.Name.Local == "id":
			e.RID = a.Value
		}
	}
	return e
}

// nextIntAllocator 数字 id 分配器（sldId@id 与 sldMasterId@id 两个独立命名空间）：
// 起点 = max(现有最大值 + 1, 下限)，跳过既有取值。
func nextIntAllocator(taken map[int]bool, min int) func() int {
	n := min - 1
	for v := range taken {
		if v > n {
			n = v
		}
	}
	return func() int {
		for {
			n++
			if !taken[n] {
				taken[n] = true
				return n
			}
		}
	}
}

// pptxContentTypesMeta [Content_Types].xml 的最小解析视图：Override 按 part 名、
// Default 按扩展名（小写归一）建表，供注入内容类型照搬与缺失扩展名补齐判定。
type pptxContentTypesMeta struct {
	overrides map[string]string // PartName（无前导 /）→ ContentType
	defaults  map[string]string // 扩展名（小写）→ ContentType
}

func parsePptxContentTypes(pkg *pptxPackage) *pptxContentTypesMeta {
	meta := &pptxContentTypesMeta{overrides: map[string]string{}, defaults: map[string]string{}}
	entry, ok := pkg.byName[pptxContentTypesPart]
	if !ok {
		return meta
	}
	var parsed struct {
		Defaults []struct {
			Extension   string `xml:"Extension,attr"`
			ContentType string `xml:"ContentType,attr"`
		} `xml:"Default"`
		Overrides []struct {
			PartName    string `xml:"PartName,attr"`
			ContentType string `xml:"ContentType,attr"`
		} `xml:"Override"`
	}
	if err := xml.Unmarshal(entry.data, &parsed); err != nil {
		return meta
	}
	for _, d := range parsed.Defaults {
		meta.defaults[strings.ToLower(d.Extension)] = d.ContentType
	}
	for _, o := range parsed.Overrides {
		meta.overrides[strings.TrimPrefix(o.PartName, "/")] = o.ContentType
	}
	return meta
}

// ---------- 字节级 XML 编辑原语 ----------

// editXMLSpans 以「解析定位 + 字节替换」编辑 XML：edit 回调对每个元素调用一次，
// 返回第二值为 false 表示不编辑（原样保留）；为 true 时该元素整块子树替换为
// 第一值字节（可为空 = 删除）。其余字节逐字节保留（前缀写法、属性顺序、空白、
// 注释、CDATA 全保留），编辑进行中不再评估回调（外层子树整体覆盖，无嵌套区间）。
// 编辑命中时的 attrs/ancestors 已做深拷贝，回调延迟执行不受解码器缓冲复用影响。
func editXMLSpans(data []byte, edit func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr, span []byte) ([]byte, bool)) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	type spanEdit struct {
		start, end int
		repl       []byte
	}
	var edits []spanEdit
	var ancestors []string
	editDepth := -1 // 正在编辑的元素层级；-1 = 未处于编辑中
	editStart := 0
	var editAttrs []xml.Attr
	var editName xml.Name
	for {
		offStart := dec.InputOffset()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		offEnd := dec.InputOffset()
		switch t := tok.(type) {
		case xml.StartElement:
			if editDepth < 0 {
				// 探测阶段以 nil span 调用回调确认编辑意图；替换字节在 EndElement
				// 阶段按完整 span 一次性构建
				if _, ok := edit(len(ancestors), ancestors, t.Name, t.Attr, nil); ok {
					editDepth = len(ancestors)
					editStart = int(offStart)
					editName = t.Name
					editAttrs = append([]xml.Attr(nil), t.Attr...)
				}
			}
			ancestors = append(ancestors, t.Name.Local)
		case xml.EndElement:
			ancestors = ancestors[:len(ancestors)-1]
			if editDepth == len(ancestors) {
				span := data[editStart:int(offEnd)]
				repl, _ := edit(len(ancestors), ancestors, editName, editAttrs, span)
				edits = append(edits, spanEdit{start: editStart, end: int(offEnd), repl: repl})
				editDepth = -1
				editAttrs = nil
			}
		}
	}
	if len(edits) == 0 {
		return data, nil
	}
	out := make([]byte, 0, len(data))
	prev := 0
	for _, sp := range edits {
		out = append(out, data[prev:sp.start]...)
		out = append(out, sp.repl...)
		prev = sp.end
	}
	return append(out, data[prev:]...), nil
}

// 需要特别说明的匹配形态：自闭合元素（<a:sldIdLst/>）的 StartElement 跨度覆盖
// 整个标签、解码器合成的 EndElement 零宽度，故「命中起点 → 结束终点」对自闭合
// 与容器形态均精确（与 dropXMLSpans 同一机制）。

var (
	xmlSldIdLstPrefixRe       = regexp.MustCompile(`^<([A-Za-z_][\w.\-]*):sldIdLst[\s/>]`)
	xmlSldMasterIdLstPrefixRe = regexp.MustCompile(`^<([A-Za-z_][\w.\-]*):sldMasterIdLst[\s/>]`)
)

// xmlListPrefix 从 sldIdLst / sldMasterIdLst 元素原始起始标签提取命名空间前缀
// （重建条目沿用文档既有前缀写法，避免引入未声明前缀）；无前缀形态返回空串。
func xmlListPrefix(span []byte, localName string) string {
	re := xmlSldIdLstPrefixRe
	if localName == "sldMasterIdLst" {
		re = xmlSldMasterIdLstPrefixRe
	}
	if m := re.FindSubmatch(span); m != nil {
		return string(m[1])
	}
	return ""
}

// indexGtOutsideQuotes 起始标签内属性值引号之外的 '>' 下标（未命中返回 -1）。
func indexGtOutsideQuotes(b []byte) int {
	inQuote := false
	for i, c := range b {
		switch {
		case c == '"':
			inQuote = !inQuote
		case c == '>' && !inQuote:
			return i
		}
	}
	return -1
}

// rebuildXMLListSpan 整体重建列表元素的内部条目：起始标签（含其命名空间声明）
// 与结束标签逐字节保留，仅替换内层；自闭合形态展开为完整容器。entriesXML 为
// 调用方按文档既有前缀生成的条目串。防御口径：标签结构定位失败时原样返回
// （不产出损坏 XML）。
func rebuildXMLListSpan(span []byte, localName, entriesXML string) []byte {
	prefix := xmlListPrefix(span, localName)
	openTag := localName
	if prefix != "" {
		openTag = prefix + ":" + localName
	}
	if bytes.HasSuffix(span, []byte("/>")) {
		// 自闭合：保留起始标签（剥尾部 "/>"），展开为容器形态
		start := span[:len(span)-2]
		out := make([]byte, 0, len(start)+1+len(entriesXML)+len(openTag)+3)
		out = append(out, start...)
		out = append(out, '>')
		out = append(out, entriesXML...)
		out = append(out, ("<" + openTag + ">")...)
		return out
	}
	gt := indexGtOutsideQuotes(span)
	if gt < 0 {
		return span
	}
	lt := bytes.LastIndex(span, []byte("</"))
	if lt < 0 || lt < gt {
		return span
	}
	out := make([]byte, 0, gt+1+len(entriesXML)+len(span)-lt)
	out = append(out, span[:gt+1]...)
	out = append(out, entriesXML...)
	out = append(out, span[lt:]...)
	return out
}

// appendXMLBeforeClosingTag 在指定闭合标签前拼接 XML 片段（字节级追加，文件其余
// 部分逐字节保留）；闭合标签不存在时报错（关系表/内容类型表损坏形态）。
func appendXMLBeforeClosingTag(data []byte, closingTag, fragment string) ([]byte, error) {
	idx := bytes.LastIndex(data, []byte(closingTag))
	if idx < 0 {
		return nil, fmt.Errorf("缺少 %s 闭合标签，文件可能已损坏", closingTag)
	}
	out := make([]byte, 0, len(data)+len(fragment))
	out = append(out, data[:idx]...)
	out = append(out, fragment...)
	out = append(out, data[idx:]...)
	return out, nil
}

// relTargetBetween 换算注入后两个 part 之间的关系 Target：重命名保持原目录，同
// 目录直接取目标基名，跨目录按相对路径（清洗 ../），与 OPC「Target 相对关系源
// part 所在目录」语义一致；换算失败防御回退包根绝对路径形态。
func relTargetBetween(fromPart, toPart string) string {
	if rel := slashRelPath(path.Dir(fromPart), toPart); rel != "." && rel != "" {
		return rel
	}
	return "/" + toPart
}

// slashRelPath URL 风格 slash 路径的相对路径计算（标准库 path 包无 Rel，
// filepath.Rel 依赖平台分隔符不可用于包内路径，故自实现）。
func slashRelPath(fromDir, to string) string {
	splitParts := func(p string) []string {
		p = strings.Trim(p, "/")
		if p == "" || p == "." {
			return nil
		}
		return strings.Split(p, "/")
	}
	fromParts := splitParts(fromDir)
	toParts := splitParts(to)
	i := 0
	for i < len(fromParts) && i < len(toParts) && fromParts[i] == toParts[i] {
		i++
	}
	var b strings.Builder
	for j := i; j < len(fromParts); j++ {
		b.WriteString("../")
	}
	b.WriteString(strings.Join(toParts[i:], "/"))
	return b.String()
}

// firstRelTargetInGraph 关系图中首个类型后缀匹配的包内目标（注入母版定位用）。
func firstRelTargetInGraph(graph map[string][]pptxRelsEdge, part, typeSuffix string) string {
	for _, e := range graph[part] {
		if strings.HasSuffix(e.relType, typeSuffix) {
			return e.target
		}
	}
	return ""
}

// escXMLAttr XML 属性值转义（再生 rels 与生成条目的属性写入面）。
func escXMLAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
