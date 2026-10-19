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
	"strings"
	"time"

	"workbench/model"
)

// OOXML 关系命名空间（.rels 与 r:id 属性的 xmlns:r 声明值）。解码器把已声明
// 前缀解析为完整 URI，属性匹配按 URI 进行，不依赖生产方使用的前缀写法。
const pptxRelsNamespace = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

// 包内固定路径的 part 名。OPC 包内路径一律使用正斜杠分隔，与运行平台无关，
// 涉及路径拼接时用 path（URL 风格）而非 filepath。
const (
	pptxContentTypesPart     = "[Content_Types].xml"
	pptxPresentationPart     = "ppt/presentation.xml"
	pptxPresentationRelsPart = "ppt/_rels/presentation.xml.rels"
	// 备注页关系类型后缀：slide 的 rels 中以该后缀结尾的关系指向其备注页，
	// 删除 slide 时据此联动定位所属 notesSlide part。
	pptxNotesSlideRelTypeSuffix = "/notesSlide"
	// 保留页真实链路（页 → 版式 → 母版 → 主题）逐跳定位用关系类型后缀：
	// B 档 GC 据此把保留页渲染不需要的母版/版式/主题及其媒体从输出包剪除。
	pptxSlideLayoutRelTypeSuffix = "/slideLayout"
	pptxSlideMasterRelTypeSuffix = "/slideMaster"
)

// SplitPptxPerPage 将 .pptx 按页拆分为单页演示文稿（无进度回调形态）。
// 拆分语义见 SplitPptxPerPageWithProgress。
func SplitPptxPerPage(ctx context.Context, srcPath string) (model.PptxSplitResult, error) {
	return SplitPptxPerPageWithProgress(ctx, srcPath, nil)
}

// SplitPptxPerPageWithProgress 将 .pptx 按页拆分为单页演示文稿（纯 Go OOXML 实现，
// 标准库 archive/zip + encoding/xml，零第三方依赖，Windows 与 Linux 行为一致）。
//
// 拆分原理：.pptx 是标准 OOXML zip 包，「复制整个包 + 最小编辑集」即得单页
// 副本，保留 part 逐字节原样拷贝，不存在任何格式转换环节，保真度最高。最小
// 编辑集：
//  1. presentation.xml 的 sldIdLst 只留当前页条目——页序以 sldIdLst 为准，
//     与 slide 文件名无关（slide3.xml 可能是第 1 页），必须经
//     sldIdLst → rId → presentation.xml.rels → part 名 三跳映射定位页；
//  2. presentation.xml.rels 删除指向已删 part 的 Relationship；
//  3. [Content_Types].xml 删除已删 part 的 Override 声明
//     （Override 指向不存在的 part 会触发 PowerPoint 修复弹窗）；
//  4. 从包中删除其余页的 slide part 及其 rels、所属备注页及其 rels
//     （notesSlide 关系双向指回 slide，留孤儿 part 同样触发修复）；
//  5. presentation.xml 剥离 custShowLst 与根级 extLst（节信息 p14:sectionLst
//     挂在其中，按数字 sldId 引用被删页，悬空触发修复）；
//  6. 保留页 rels 中指向已删页的内部跳转关系（跨页超链接/动作按钮）一并清理；
//  7. B 档体积 GC：以保留页真实渲染链路（页 → 版式 → 母版 → 主题）为根做
//     rels 图可达性闭包，闭包外的 part（其余页独占媒体、未用版式/母版/主题、
//     孤儿媒体）从输出包剪除。剪除面配套三处联动编辑：presentation.xml 的
//     sldMasterIdLst 剪枝至保留页母版、保留母版 rels 移除指向被剪版式的
//     Relationship、保留母版 XML 的 sldLayoutIdLst 剪除被剪版式条目（母版本体
//     逐字节拷贝，条目经 r:id 引用版式关系——rels 删除后条目残留即悬空引用，
//     PowerPoint 打开触发修复弹窗）。链路不可解析（rels 缺失/形态异常）时整段
//     GC 静默降级为旧行为（仅删页与备注页），保真优先于体积。
//
// 输出规则：源文件同级以源文件名命名的文件夹，命名 `<原文件名>-<三位页序号>.pptx`
// （固定 3 位，≥100 页字典序与页序一致）；文件夹已存在时同名单文件覆盖重生成。
// 每页输出为原子写（CreateTemp 同目录 → 写入 → Sync → Rename），复刻
// util.SaveJSON 序列——二进制 zip 不适用 SaveJSON 本体（JSON 专用）。
//
// 取消口径：ctx 贯通到每页输出前检查，取消错误以 %w 包装 context.Canceled，
// errors.Is 判定可用（服务编排层统一映射为 ErrOperationCancelled 中性提示）；
// 取消前已写盘的完整单页文件保留在输出文件夹内，不回滚。
//
// 进度口径：onProgress 在每页写盘成功后回调（done 为已完成页数、total 为总页数，
// done 从 1 计），可为 nil（无进度消费方形态）；进度事件由服务编排层经 EventSink
// 转发，核心不感知出口。
func SplitPptxPerPageWithProgress(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
	if err := ctx.Err(); err != nil {
		return model.PptxSplitResult{}, fmt.Errorf("拆分已取消: %w", err)
	}
	if strings.TrimSpace(srcPath) == "" {
		return model.PptxSplitResult{}, errors.New("请先选择要拆分的 PPT 文件")
	}
	// .ppt 老二进制格式（OOXML 时代前）与其他扩展名一并拒绝，引导口径统一
	if !strings.EqualFold(filepath.Ext(srcPath), ".pptx") {
		return model.PptxSplitResult{}, fmt.Errorf("仅支持 .pptx 格式，当前文件：%s", filepath.Base(srcPath))
	}
	if _, err := os.Stat(srcPath); err != nil {
		return model.PptxSplitResult{}, fmt.Errorf("无法访问 PPT 文件: %w", err)
	}

	zr, err := zip.OpenReader(srcPath)
	if err != nil {
		// 加密 pptx 是 OLE 容器而非 zip，与损坏文件走同一引导
		return model.PptxSplitResult{}, fmt.Errorf("无法读取 PPT 文件（文件可能已损坏或已加密）: %w", err)
	}
	defer zr.Close()

	// 全量解压进内存：典型演示文稿体积在几十 MB 量级，可接受；换取单次解压
	// 与「保留 part 逐字节拷贝」的简单实现（闭包外媒体由 B 档 GC 剪除，见下方
	// splitGCContext 注释块）
	pkg, err := readZipPackage(&zr.Reader)
	if err != nil {
		return model.PptxSplitResult{}, fmt.Errorf("读取 PPT 内部数据失败: %w", err)
	}

	pages, err := resolveSlidePages(pkg)
	if err != nil {
		return model.PptxSplitResult{}, err
	}
	notesOf, err := resolveNotesSlides(pkg, pages)
	if err != nil {
		return model.PptxSplitResult{}, err
	}
	gc, err := newSplitGCContext(pkg)
	if err != nil {
		return model.PptxSplitResult{}, err
	}

	baseName := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	outDir := filepath.Join(filepath.Dir(srcPath), baseName)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return model.PptxSplitResult{}, fmt.Errorf("创建输出文件夹失败: %w", err)
	}

	result := model.PptxSplitResult{OutputDir: outDir, Files: make([]string, 0, len(pages))}
	for i, page := range pages {
		if err := ctx.Err(); err != nil {
			return model.PptxSplitResult{}, fmt.Errorf("拆分已取消: %w", err)
		}
		// 单页拆分 = 保留页集合恰一页的集合化形态（deleteSetForPages 泛化入口，
		// 与多页子集导出共用同一删除集计算与联动编辑语义）
		gcPlan := gc.deleteSetForPages(pkg, pages, notesOf, []pptxPage{page})
		outPath := filepath.Join(outDir, fmt.Sprintf("%s-%03d.pptx", baseName, i+1))
		plan := splitPagePlan{
			keptRids:                    map[string]bool{page.rid: true},
			deleted:                     gcPlan.deleted,
			keptPageParts:               []string{page.part},
			keptMasterRids:              gcPlan.keptMasterRids,
			keptMasterParts:             gcPlan.keptMasterParts,
			keptMasterDroppedLayoutRids: gcPlan.keptMasterDroppedLayoutRids,
		}
		if err := writeSplitOutput(pkg, plan, outPath); err != nil {
			return model.PptxSplitResult{}, err
		}
		result.Files = append(result.Files, outPath)
		if onProgress != nil {
			onProgress(len(result.Files), len(pages))
		}
	}
	result.PageCount = len(result.Files)
	return result, nil
}

// zipEntry 源包内单个 part 的解压内容。modified/method 原样透传到输出 zip 头
// （时间戳与压缩方式保留，内容字节不受重打包影响）。
type zipEntry struct {
	name     string
	method   uint16
	modified time.Time
	data     []byte
}

// pptxPackage 源包内存视图：entries 保持原 zip 条目顺序（输出按此顺序重写），
// byName 供按路径定位 part。
type pptxPackage struct {
	entries []zipEntry
	byName  map[string]*zipEntry
}

// readZipPackage 读取 zip 全部条目解压进内存（不设字节上限，拆分链路既有语义
// 逐字不变——拆分入口已有文件级校验且为用户主动单文件操作）。
func readZipPackage(r *zip.Reader) (*pptxPackage, error) {
	return readZipPackageWithLimits(r, 0, 0)
}

// readZipPackageWithLimits 带字节上限的 zip 全量解压：maxEntryBytes 限单条目
// 解压体积，maxTotalBytes 限全部条目解压累计体积；两者 ≤0 表示不设限（透传为
// readZipPackage 的无限语义）。超限立即返回错误（不再继续解压剩余条目），调用
// 方按自身语义分类降级（素材索引侧按「文件损坏」处理）。
//
// 上限用途：素材目录扫描批量解析外来 pptx，畸形包（zip 炸弹：压缩后极小、解压
// 后巨大）无上限解压会拖垮进程内存；真实演示文稿解压量级为数十 MB（媒体为主），
// 上限给足裕量仅拦截异常形态。
func readZipPackageWithLimits(r *zip.Reader, maxEntryBytes, maxTotalBytes int64) (*pptxPackage, error) {
	pkg := &pptxPackage{
		entries: make([]zipEntry, 0, len(r.File)),
		byName:  make(map[string]*zipEntry, len(r.File)),
	}
	var total int64
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("打开 %s 失败: %w", f.Name, err)
		}
		var data []byte
		var readErr error
		if maxEntryBytes > 0 {
			// 多读 1 字节探测超限（LimitReader 读满即截断，无法区分「恰好等于上限」）
			data, readErr = io.ReadAll(io.LimitReader(rc, maxEntryBytes+1))
		} else {
			data, readErr = io.ReadAll(rc)
		}
		closeErr := rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取 %s 失败: %w", f.Name, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("关闭 %s 失败: %w", f.Name, closeErr)
		}
		if maxEntryBytes > 0 && int64(len(data)) > maxEntryBytes {
			return nil, fmt.Errorf("条目 %s 解压后超过单条目上限（%d 字节）", f.Name, maxEntryBytes)
		}
		total += int64(len(data))
		if maxTotalBytes > 0 && total > maxTotalBytes {
			return nil, fmt.Errorf("全部条目解压累计超过上限（%d 字节）", maxTotalBytes)
		}
		pkg.entries = append(pkg.entries, zipEntry{name: f.Name, method: f.Method, modified: f.Modified, data: data})
		pkg.byName[f.Name] = &pkg.entries[len(pkg.entries)-1]
	}
	return pkg, nil
}

// pptxSldIdEntry presentation.xml 中 <p:sldId> 的最小解析视图：
// ID 为页数字标识（p:id），RID 为关系表键（r:id）。
//
// Warning：ID 的标签 `xml:"id,attr"` 无命名空间限定，对任意命名空间的同名属性
// 都命中且后出现者覆盖——sldId 同时带无前缀 id 与 r:id 两个属性，该字段解析值
// 会被 r:id 污染（实证 2026-10-19：`id="256" r:id="rId2"` → ID="rId2"），不可
// 作为数字 id 消费；数字 id 须用 parsePptxListIDs（service/pptx_merge.go，按
// 属性命名空间精确解析）。本结构当前仅消费 RID，ID 字段留作解析视图完整性。
type pptxSldIdEntry struct {
	ID  string `xml:"id,attr"`
	RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

// pptxSldIdList <p:sldIdLst> 的最小解析视图。
type pptxSldIdList struct {
	SldIds []pptxSldIdEntry `xml:"sldId"`
}

// pptxSldMasterIdEntry <p:sldMasterId> 的最小解析视图（母版条目，结构同页条目）。
// ID 字段同样受 `xml:"id,attr"` 命名空间不分缺陷影响（见 pptxSldIdEntry 注释），
// 不可作为数字 id 消费。
type pptxSldMasterIdEntry struct {
	ID  string `xml:"id,attr"`
	RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

// pptxSldMasterIdList <p:sldMasterIdLst> 的最小解析视图。
type pptxSldMasterIdList struct {
	SldMasterIds []pptxSldMasterIdEntry `xml:"sldMasterId"`
}

// pptxPresentationMeta presentation.xml 的最小解析视图：只取 sldIdLst 与
// sldMasterIdLst，其余元素由 Unmarshal 忽略（presentation.xml 本体走字节级编辑，
// 不经此结构重写）。
type pptxPresentationMeta struct {
	SldIdLst       pptxSldIdList       `xml:"sldIdLst"`
	SldMasterIdLst pptxSldMasterIdList `xml:"sldMasterIdLst"`
}

// pptxRelationship .rels 文件中单个 <Relationship> 的最小解析视图。
type pptxRelationship struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
}

// pptxRelationships .rels 文件的最小解析视图。
type pptxRelationships struct {
	Relationships []pptxRelationship `xml:"Relationship"`
}

// pptxPage 单页拆分定位结果：rid 供 sldIdLst 编辑，part 供删除集与保留页 rels 编辑。
type pptxPage struct {
	rid  string
	part string
}

// resolveSlidePages 页序三跳映射：sldIdLst 顺序 → rId → presentation.xml.rels
// Target → part 名。返回顺序即展示页序（非 slide 文件名字典序）。
func resolveSlidePages(pkg *pptxPackage) ([]pptxPage, error) {
	var meta pptxPresentationMeta
	if err := parsePartXML(pkg, pptxPresentationPart, &meta); err != nil {
		return nil, fmt.Errorf("不是有效的 PowerPoint 演示文稿（%v）", err)
	}
	var rels pptxRelationships
	if err := parsePartXML(pkg, pptxPresentationRelsPart, &rels); err != nil {
		return nil, fmt.Errorf("不是有效的 PowerPoint 演示文稿（%v）", err)
	}
	relTarget := make(map[string]string, len(rels.Relationships))
	for _, rel := range rels.Relationships {
		relTarget[rel.ID] = rel.Target
	}
	pages := make([]pptxPage, 0, len(meta.SldIdLst.SldIds))
	for _, sldId := range meta.SldIdLst.SldIds {
		target, ok := relTarget[sldId.RID]
		if !ok {
			return nil, fmt.Errorf("演示文稿关系表缺少 %s 对应的页关系，文件可能已损坏", sldId.RID)
		}
		part := resolveRelTarget(pptxPresentationPart, target)
		if _, ok := pkg.byName[part]; !ok {
			return nil, fmt.Errorf("演示文稿页 %s 在包内缺失，文件可能已损坏", part)
		}
		pages = append(pages, pptxPage{rid: sldId.RID, part: part})
	}
	if len(pages) == 0 {
		return nil, errors.New("演示文稿不包含任何幻灯片，无法拆分")
	}
	return pages, nil
}

// resolveNotesSlides 逐页读取 slide rels 定位所属备注页 part：
// Type 以 /notesSlide 结尾的关系即该页备注。删除页的备注页随页一并删除，
// 留孤儿 notesSlide（其关系指回已删 slide）会触发 PowerPoint 修复弹窗。
// slide 无 rels（无备注页形态）时跳过，不视为损坏。
func resolveNotesSlides(pkg *pptxPackage, pages []pptxPage) (map[string]string, error) {
	notesOf := make(map[string]string, len(pages))
	for _, page := range pages {
		relsPath := relsPartOf(page.part)
		entry, ok := pkg.byName[relsPath]
		if !ok {
			continue
		}
		var rels pptxRelationships
		if err := xml.Unmarshal(entry.data, &rels); err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", relsPath, err)
		}
		for _, rel := range rels.Relationships {
			if strings.HasSuffix(rel.Type, pptxNotesSlideRelTypeSuffix) {
				notesOf[page.part] = resolveRelTarget(page.part, rel.Target)
				break
			}
		}
	}
	return notesOf, nil
}

// ---------- B 档体积 GC（保留页真实链路可达性闭包） ----------
//
// 背景：最小编辑集只删页与备注页 part，其余页独占媒体与未用母版/版式/主题
// 全量随包携带，图片密集的演示文稿单页输出可达源包体积（实测 10MB 级源拆出
// 9MB/页）。GC 以保留页真实渲染链路（页 → 版式 → 母版 → 主题）为根做 rels 图
// 可达性闭包：闭包外的 part 从输出包剪除，保留链 part 逐字节不变（保真红线）。
// 链路不可解析时整段降级为旧行为（仅删页与备注页），体积优化不构成正确性依赖。

// pptxRelsEdge 关系图边：包内目标 part 与关系类型（目标存在性在建图时过滤）。
type pptxRelsEdge struct {
	target  string
	relType string
}

// splitGCContext 单文件拆分的 GC 共享上下文：关系图与母版/版式清单一次构建
// 全页复用。
type splitGCContext struct {
	graph   map[string][]pptxRelsEdge // 关系源 part → 边集（包根 .rels 以空串为源）
	masters map[string]string         // 母版 part → sldMasterIdLst 对应 rId
	layouts map[string]bool           // 全部版式 part（母版 rels 收集，含未用版式）
}

// newSplitGCContext 构建 GC 共享上下文。sldMasterIdLst 为空或 rId 悬空时返回
// 空母版表——deleteSetForPages 对空表整段降级，不视为文件损坏。
func newSplitGCContext(pkg *pptxPackage) (*splitGCContext, error) {
	var meta pptxPresentationMeta
	if err := parsePartXML(pkg, pptxPresentationPart, &meta); err != nil {
		return nil, fmt.Errorf("不是有效的 PowerPoint 演示文稿（%v）", err)
	}
	var rels pptxRelationships
	if err := parsePartXML(pkg, pptxPresentationRelsPart, &rels); err != nil {
		return nil, fmt.Errorf("不是有效的 PowerPoint 演示文稿（%v）", err)
	}
	ridToTarget := make(map[string]string, len(rels.Relationships))
	for _, rel := range rels.Relationships {
		if rel.Target == "" || strings.EqualFold(rel.TargetMode, "External") {
			continue
		}
		ridToTarget[rel.ID] = resolveRelTarget(pptxPresentationPart, rel.Target)
	}
	gc := &splitGCContext{
		graph:   buildPptxRelsGraph(pkg),
		masters: make(map[string]string),
		layouts: make(map[string]bool),
	}
	for _, m := range meta.SldMasterIdLst.SldMasterIds {
		if part, ok := ridToTarget[m.RID]; ok && pkg.byName[part] != nil {
			gc.masters[part] = m.RID
		}
	}
	// 版式全集从各母版 rels 收集（OPC 规范上版式只被母版与页引用，页引用的
	// 版式必属某母版；孤置版式无需收集——闭包扫描自然剪除）
	for master := range gc.masters {
		for _, e := range gc.graph[master] {
			if strings.HasSuffix(e.relType, pptxSlideLayoutRelTypeSuffix) {
				gc.layouts[e.target] = true
			}
		}
	}
	return gc, nil
}

// buildPptxRelsGraph 全包关系图：解析每个 .rels，仅保留目标存在于包内的内部
// 关系。个别 rels 解析失败按「该源无边」处理（下游闭包把关联 part 剪除），
// 不阻塞拆分——编辑面悬空由 danglingRelsDropPredicate 兜底。
func buildPptxRelsGraph(pkg *pptxPackage) map[string][]pptxRelsEdge {
	graph := make(map[string][]pptxRelsEdge)
	for i := range pkg.entries {
		name := pkg.entries[i].name
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		var rels pptxRelationships
		if err := xml.Unmarshal(pkg.entries[i].data, &rels); err != nil {
			continue
		}
		source := relsSourcePart(name)
		for _, rel := range rels.Relationships {
			if rel.Target == "" || strings.EqualFold(rel.TargetMode, "External") {
				continue
			}
			target := resolveRelTarget(source, rel.Target)
			if pkg.byName[target] == nil {
				continue
			}
			graph[source] = append(graph[source], pptxRelsEdge{target: target, relType: rel.Type})
		}
	}
	return graph
}

// splitGCPlan GC 删除集计算产物：删除集 + sldMasterIdLst 剪枝保留集合与母版
// 联动编辑定位。keptMasterRids/keptMasterParts 为空 = GC 降级（调用方跳过全部
// 母版联动编辑）。
type splitGCPlan struct {
	deleted         map[string]bool // 输出删除集（页/备注页/被剪母版版式主题/闭包孤儿）
	keptMasterRids  map[string]bool // sldMasterIdLst 剪枝保留集合（母版 rId）
	keptMasterParts map[string]bool // 保留母版 part 集合（母版 rels 悬空清理编辑面）
	// keptMasterDroppedLayoutRids 保留母版 part → 被剪版式的 rId 集合（母版 XML
	// sldLayoutIdLst 剪枝谓词入参）；缺失键 = 该母版无被剪版式或 rels 不可解析，
	// 母版 XML 零编辑面
	keptMasterDroppedLayoutRids map[string]map[string]bool
}

// deleteSetForPages 计算保留页集合输出的删除集：页/备注页基础删除集（旧行为）+
// B 档 GC 剪除集。保留面为页集合语义（单页拆分即恰一页的特例，多页子集导出/
// 购物栏合并导出的前置能力），与单页口径的三点差异：
//  1. 保留母版/版式为集合——全部保留页链路（页 → 版式 → 母版）上的母版与版式
//     保留（rels 图可达性闭包自动多母版化），其余母版/版式进删除集；
//  2. 任一保留页链路不可解析（页 rels 缺版式跳 / 版式 rels 缺母版跳 / 保留母版
//     不在 sldMasterIdLst）时整段 GC 降级——单页版为该页降级，集合版保守放大为
//     整段降级（GC 是体积优化非正确性依赖，保真优先于体积）；
//  3. 保留母版被剪版式的 rId 集合按母版分组返回（多保留母版各自的 sldLayoutIdLst
//     剪枝面互不相干）。
//
// 其余语义与单页口径一致：闭包屏障逻辑不变、presentation 级非页引用保留、
// keptMasterDroppedLayoutRids 计算口径不变（rels 缺失/不可解析返回缺键）。
// keptPages 须非空（调用方入口校验，空集合语义未定义）。
func (g *splitGCContext) deleteSetForPages(pkg *pptxPackage, pages []pptxPage, notesOf map[string]string, keptPages []pptxPage) splitGCPlan {
	// 基础删除集：未保留页 part + 其 rels + 其备注页 + 备注页 rels；保留页自身
	// 三件套与所属备注页不进删除集（备注页随保留页原样带走）
	keptByPart := make(map[string]bool, len(keptPages))
	for _, kept := range keptPages {
		keptByPart[kept.part] = true
	}
	plan := splitGCPlan{deleted: make(map[string]bool, len(pages)*4)}
	for _, other := range pages {
		if keptByPart[other.part] {
			continue
		}
		plan.deleted[other.part] = true
		plan.deleted[relsPartOf(other.part)] = true
		if notes, ok := notesOf[other.part]; ok {
			plan.deleted[notes] = true
			plan.deleted[relsPartOf(notes)] = true
		}
	}
	if len(g.masters) == 0 || len(keptPages) == 0 {
		return plan
	}
	// 全部保留页链路解析：任一页断链即整段降级（保守口径，见函数注释差异第 2 条）
	keptLayouts := make(map[string]bool)
	keptMasters := make(map[string]bool)
	for _, kept := range keptPages {
		layout, master := g.resolveKeptChain(kept.part)
		if layout == "" || master == "" {
			return splitGCPlan{deleted: plan.deleted}
		}
		if _, ok := g.masters[master]; !ok {
			return splitGCPlan{deleted: plan.deleted}
		}
		keptLayouts[layout] = true
		keptMasters[master] = true
	}
	plan.keptMasterRids = make(map[string]bool, len(keptMasters))
	plan.keptMasterParts = make(map[string]bool, len(keptMasters))
	plan.keptMasterDroppedLayoutRids = make(map[string]map[string]bool, len(keptMasters))
	for master := range keptMasters {
		plan.keptMasterRids[g.masters[master]] = true
		plan.keptMasterParts[master] = true
	}
	// 未保留的母版与未用版式进删除集（主题不显式剪：保留母版的主题经图可达保留，
	// 被剪母版的主题因屏障不可达由闭包扫描剪除）
	for master := range g.masters {
		if !keptMasters[master] {
			plan.deleted[master] = true
			plan.deleted[relsPartOf(master)] = true
		}
	}
	for layout := range g.layouts {
		if !keptLayouts[layout] {
			plan.deleted[layout] = true
			plan.deleted[relsPartOf(layout)] = true
		}
	}
	// 可达性闭包：包根 .rels（docProps）、presentation 级引用（presProps/
	// viewProps/tableStyles/notesMaster 等）、全部保留页 + 备注 + 版式 + 母版为
	// 根，deleted 作屏障（被剪母版/版式的下挂资源不可达）。闭包外 part（未保留
	// 页独占媒体、孤儿媒体、被剪母版主题等）进删除集。
	keep := make(map[string]bool, len(pkg.entries))
	roots := []string{"", pptxContentTypesPart, pptxPresentationPart}
	for _, kept := range keptPages {
		roots = append(roots, kept.part)
		if notes, ok := notesOf[kept.part]; ok {
			roots = append(roots, notes)
		}
	}
	for layout := range keptLayouts {
		roots = append(roots, layout)
	}
	for master := range keptMasters {
		roots = append(roots, master)
	}
	for _, e := range g.graph[pptxPresentationPart] {
		if !plan.deleted[e.target] {
			roots = append(roots, e.target)
		}
	}
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		part := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if keep[part] {
			continue
		}
		keep[part] = true
		if rp := relsPartOf(part); pkg.byName[rp] != nil {
			keep[rp] = true
		}
		for _, e := range g.graph[part] {
			if !plan.deleted[e.target] && !keep[e.target] {
				queue = append(queue, e.target)
			}
		}
	}
	for i := range pkg.entries {
		if !keep[pkg.entries[i].name] {
			plan.deleted[pkg.entries[i].name] = true
		}
	}
	// 保留母版被剪版式的 rId 集合（母版 XML sldLayoutIdLst 剪枝联动，按保留母版
	// 分组；此时删除集已含闭包剪除面，口径为最终删除集）
	for master := range keptMasters {
		if rids := g.keptMasterDroppedLayoutRids(pkg, master, plan.deleted); len(rids) > 0 {
			plan.keptMasterDroppedLayoutRids[master] = rids
		}
	}
	return plan
}

// keptMasterDroppedLayoutRids 保留母版 rels 中指向被剪版式的 rId 集合：母版 XML
// 本体逐字节拷贝，其 sldLayoutIdLst 内 sldLayoutId 条目经 r:id 引用版式关系——
// 母版 rels 侧关系已删而 XML 条目残留即悬空引用（PowerPoint 打开触发修复弹窗），
// 调用方据本集合对母版 XML 做同款 sldMasterIdLst 剪枝式编辑。解析保留母版原始
// rels（编辑前字节）建 rid→target 映射，target 落删除集的 rid 收集；rels 缺失/
// 不可解析返回 nil（零编辑面，与 GC 降级同口径——保真优先）。
func (g *splitGCContext) keptMasterDroppedLayoutRids(pkg *pptxPackage, keptMaster string, deleted map[string]bool) map[string]bool {
	entry, ok := pkg.byName[relsPartOf(keptMaster)]
	if !ok {
		return nil
	}
	var rels pptxRelationships
	if err := xml.Unmarshal(entry.data, &rels); err != nil {
		return nil
	}
	var rids map[string]bool
	for _, rel := range rels.Relationships {
		if rel.Target == "" || strings.EqualFold(rel.TargetMode, "External") {
			continue
		}
		if deleted[resolveRelTarget(keptMaster, rel.Target)] {
			if rids == nil {
				rids = make(map[string]bool)
			}
			rids[rel.ID] = true
		}
	}
	return rids
}

// resolveKeptChain 保留页真实渲染链路定位：页 rels 的版式关系 → 版式 rels 的
// 母版关系。任一跳缺失返回空（调用方降级）。
func (g *splitGCContext) resolveKeptChain(pagePart string) (layout, master string) {
	layout = g.firstRelTarget(pagePart, pptxSlideLayoutRelTypeSuffix)
	if layout == "" {
		return "", ""
	}
	return layout, g.firstRelTarget(layout, pptxSlideMasterRelTypeSuffix)
}

// firstRelTarget 关系类型以后缀结尾的首个包内目标，未命中返回空串。
func (g *splitGCContext) firstRelTarget(part, typeSuffix string) string {
	for _, e := range g.graph[part] {
		if strings.HasSuffix(e.relType, typeSuffix) {
			return e.target
		}
	}
	return ""
}

// relsSourcePart .rels 文件路径反推其关系源 part（OPC：<dir>/_rels/<名>.rels 的
// 源为 <dir>/<名>；包根 _rels/.rels 的源为空串即包根本身）。
func relsSourcePart(relsName string) string {
	dir, base := path.Split(relsName)
	sourceDir := strings.TrimSuffix(dir, "_rels/")
	name := strings.TrimSuffix(base, ".rels")
	return sourceDir + name
}

// parsePartXML 按路径取包内 part 并解析进 v。
func parsePartXML(pkg *pptxPackage, name string, v any) error {
	entry, ok := pkg.byName[name]
	if !ok {
		return fmt.Errorf("缺少 %s", name)
	}
	if err := xml.Unmarshal(entry.data, v); err != nil {
		return fmt.Errorf("解析 %s 失败: %w", name, err)
	}
	return nil
}

// splitPagePlan 单次输出的编辑计划：删除集 + GC 联动编辑定位。保留面为页集合
// 语义（单页拆分与多页子集导出共用，单页即单元素集合）。
type splitPagePlan struct {
	keptRids       map[string]bool // presentation.xml sldIdLst 保留页 rId 集合
	deleted        map[string]bool // 输出删除集（页/备注页/被剪母版版式主题/闭包孤儿）
	keptPageParts  []string        // 保留页 part 集合（页 rels 悬空跳转清理编辑面）
	keptMasterRids map[string]bool // sldMasterIdLst 剪枝保留集合；空 = GC 降级不剪
	// keptMasterParts 保留母版 part 集合（母版 rels 悬挂清理编辑面；空 = GC 降级）
	keptMasterParts map[string]bool
	// keptMasterDroppedLayoutRids 保留母版 part → 被剪版式的 rId 集合（母版 XML
	// sldLayoutIdLst 剪枝谓词入参）；缺失键 = 该母版无被剪版式/rels 不可解析，
	// 母版 XML 零编辑面
	keptMasterDroppedLayoutRids map[string]map[string]bool
}

// writeSplitOutput 生成输出文件：编辑在内存中完成后按原条目顺序重写
// zip，原子写序列与 util.SaveJSON 一致（CreateTemp 同目录 → 写入 → Sync →
// Rename；成功 rename 后 defer 删除为 no-op，失败路径清理临时文件残留）。
// plan.keptMasterRids 非空时 presentation.xml 的 sldMasterIdLst 同步剪枝（B 档
// GC 联动），空则不剪（GC 降级形态，母版条目原样保留）。
func writeSplitOutput(pkg *pptxPackage, plan splitPagePlan, outPath string) error {
	deleted := plan.deleted
	edited := make(map[string][]byte, 5)
	edit := func(name string, predicate func(int, []string, xml.Name, []xml.Attr) bool) error {
		entry, ok := pkg.byName[name]
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
	// presentation.xml：sldIdLst 只留保留页 + sldMasterIdLst 剪枝（GC 生效时）
	// + 剥离 custShowLst / 根级 extLst
	if err := edit(pptxPresentationPart, presentationDropPredicate(plan.keptRids, plan.keptMasterRids)); err != nil {
		return err
	}
	// presentation.xml.rels 与保留页 rels：Target 落在删除集的关系一律移除。
	// 谓词入参是关系源 part（Target 相对源 part 所在目录解析），而非 rels 文件
	// 本身——presentation.xml.rels 位于 ppt/_rels/，其 Target 却相对 ppt/ 解析。
	if err := edit(pptxPresentationRelsPart, danglingRelsDropPredicate(pptxPresentationPart, deleted)); err != nil {
		return err
	}
	for _, pagePart := range plan.keptPageParts {
		if err := edit(relsPartOf(pagePart), danglingRelsDropPredicate(pagePart, deleted)); err != nil {
			return err
		}
	}
	// 保留母版 rels：指向被剪版式的 Relationship 移除（GC 生效时才有删除面；
	// 母版 rels 缺失时 edit 内部跳过）。Target 相对母版所在目录解析。
	for masterPart := range plan.keptMasterParts {
		if err := edit(relsPartOf(masterPart), danglingRelsDropPredicate(masterPart, deleted)); err != nil {
			return err
		}
	}
	// 保留母版 XML：sldLayoutIdLst 剪除指向被剪版式的条目（母版本体字节拷贝，
	// 条目残留 = 悬空 r:id 引用触发 PowerPoint 修复弹窗）。集合空时跳过编辑
	// （降级形态/无未用版式零编辑面，字节不变）。与母版 rels 关系删除、
	// Content_Types Override 清理三处联动齐。
	for masterPart, dropRids := range plan.keptMasterDroppedLayoutRids {
		if len(dropRids) == 0 {
			continue
		}
		if err := edit(masterPart, masterLayoutIdDropPredicate(dropRids)); err != nil {
			return err
		}
	}
	// [Content_Types].xml：已删 part 的 Override 声明移除
	if err := edit(pptxContentTypesPart, overrideDropPredicate(deleted)); err != nil {
		return err
	}

	entries := make([]zipEntry, 0, len(pkg.entries))
	for i := range pkg.entries {
		entry := pkg.entries[i]
		if deleted[entry.name] {
			continue
		}
		if ed, ok := edited[entry.name]; ok {
			entry.data = ed
		}
		entries = append(entries, entry)
	}
	return writeZipAtomic(entries, outPath)
}

// writeZipAtomic 按 entries 顺序重写 zip 并原子落盘（CreateTemp 同目录 → 写入 →
// Sync → Rename，与 util.SaveJSON 序列一致；成功 rename 后 defer 删除为 no-op，
// 失败路径清理临时文件残留）。单页拆分与多源合并共用同一落盘序列。
func writeZipAtomic(entries []zipEntry, outPath string) error {
	dir := filepath.Dir(outPath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(outPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // 成功 rename 后目标名已不存在，删除为 no-op

	zw := zip.NewWriter(tmp)
	for i := range entries {
		entry := &entries[i]
		header := &zip.FileHeader{Name: entry.name, Method: entry.method, Modified: entry.modified}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			_ = tmp.Close()
			return fmt.Errorf("写入 %s 失败: %w", entry.name, err)
		}
		if _, err := writer.Write(entry.data); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("写入 %s 失败: %w", entry.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("生成 zip 数据失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("落盘 %s 失败: %w", outPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("写入输出文件失败: %w", err)
	}
	return nil
}

// presentationDropPredicate presentation.xml 的删除谓词：根级 custShowLst
// （按页 rId 引用的自定义放映）、根级 extLst（p14:sectionLst 节信息挂在其中，
// 按数字 sldId 引用页，引用被删页即悬空），sldIdLst 中非保留页的 sldId 条目，
// 以及 sldMasterIdLst 中非保留母版的条目（keptMasterRids 非空时；空 = GC 降级
// 不剪）。depth 口径：根元素为 0，其直接子元素为 1。
//
// 层级双保险说明：节信息内部的 p14:sldId 与真实页条目局部名相同（父同为
// sldIdLst），但位于 depth>=6 的 extLst 子树内——depth==2 限制 + 删除 extLst
// 时整子树跳过（删除中不再评估谓词）双重隔离，节内条目不会被误判。
func presentationDropPredicate(keptRids, keptMasterRids map[string]bool) func(int, []string, xml.Name, []xml.Attr) bool {
	return func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr) bool {
		if depth == 2 && len(ancestors) > 0 {
			switch ancestors[len(ancestors)-1] {
			case "sldIdLst":
				if name.Local == "sldId" {
					rid := relIDAttr(attrs)
					return rid != "" && !keptRids[rid]
				}
			case "sldMasterIdLst":
				if name.Local == "sldMasterId" {
					rid := relIDAttr(attrs)
					return len(keptMasterRids) > 0 && rid != "" && !keptMasterRids[rid]
				}
			}
		}
		switch {
		case depth == 1 && (name.Local == "custShowLst" || name.Local == "extLst"):
			return true
		}
		return false
	}
}

// masterLayoutIdDropPredicate 保留母版 XML 的删除谓词：sldLayoutIdLst 内指向被剪
// 版式（rels rId ∈ drop 集）的 sldLayoutId 条目。depth==2 + 父元素限定与
// presentationDropPredicate 同款双保险（母版子树其他位置的同名局部名元素不误伤，
// 删除进行中不再评估谓词）。
func masterLayoutIdDropPredicate(dropRids map[string]bool) func(int, []string, xml.Name, []xml.Attr) bool {
	return func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr) bool {
		if depth != 2 || len(ancestors) == 0 || ancestors[len(ancestors)-1] != "sldLayoutIdLst" {
			return false
		}
		if name.Local != "sldLayoutId" {
			return false
		}
		rid := relIDAttr(attrs)
		return rid != "" && dropRids[rid]
	}
}

// danglingRelsDropPredicate .rels 文件的删除谓词：Target 解析后落在删除集内的
// Relationship 一律移除（不允许关系指向不存在的 part）。同一谓词覆盖两类引用面：
// presentation.xml.rels 中被删页的 slide 关系，与保留页 rels 中指向已删页的
// 内部跳转关系（跨页超链接/动作按钮，Type 以 /slide 结尾）。
// sourcePart 为该 rels 对应的关系源 part（如 ppt/presentation.xml）——OPC 关系
// Target 相对源 part 所在目录解析，与 rels 文件自身所在的 _rels/ 目录无关。
func danglingRelsDropPredicate(sourcePart string, deleted map[string]bool) func(int, []string, xml.Name, []xml.Attr) bool {
	return func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr) bool {
		if name.Local != "Relationship" {
			return false
		}
		if strings.EqualFold(attrValue(attrs, "", "TargetMode"), "External") {
			return false // 外部超链接（URL）与包内部件无关，原样保留
		}
		target := attrValue(attrs, "", "Target")
		if target == "" {
			return false
		}
		return deleted[resolveRelTarget(sourcePart, target)]
	}
}

// overrideDropPredicate [Content_Types].xml 的删除谓词：Override 声明的 part
// 已进删除集时移除该声明。
func overrideDropPredicate(deleted map[string]bool) func(int, []string, xml.Name, []xml.Attr) bool {
	return func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr) bool {
		if name.Local != "Override" {
			return false
		}
		partName := strings.TrimPrefix(attrValue(attrs, "", "PartName"), "/")
		return partName != "" && deleted[partName]
	}
}

// dropXMLSpans 删除 XML 中匹配谓词的元素子树，其余字节逐字节保留。
//
// 编辑策略为「解析定位 + 字节删除」而非 token 重新序列化：encoding/xml 的
// Encoder 会对命名空间声明、自闭合形态做规范化重写，而 OOXML 对未识别内容
// 敏感（PowerPoint 兼容面），字节保留把改写面压到零——输出除被删区间外与
// 输入逐字节一致（前缀写法、属性顺序、空白、注释、CDATA 全保留）。
//
// 区间定位依赖 Decoder.InputOffset：token 跨度 = [本次调用前, 本次调用后) 的
// 偏移；自闭合元素（<a/>）的 StartElement 跨度覆盖整个标签，解码器合成的
// EndElement 零宽度，故「命中 StartElement 起点 → 匹配 EndElement 终点」对
// 自闭合与容器形态均精确。
//
// match 参数：depth 为元素层级（根元素 0），ancestors 为祖先元素局部名链
// （不含当前元素），name/attrs 为当前元素。谓词命中即整块删除该元素子树；
// 删除进行中不再评估谓词（外层子树整体覆盖，不产生嵌套区间）。
func dropXMLSpans(data []byte, match func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr) bool) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	type span struct{ start, end int }
	var spans []span
	var ancestors []string
	dropDepth := -1 // 正在删除的元素层级；-1 = 未处于删除中
	dropStart := 0
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
			if dropDepth < 0 && match(len(ancestors), ancestors, t.Name, t.Attr) {
				dropDepth = len(ancestors)
				dropStart = int(offStart)
			}
			ancestors = append(ancestors, t.Name.Local)
		case xml.EndElement:
			ancestors = ancestors[:len(ancestors)-1]
			// 弹栈后回到被删元素自身层级：该 EndElement 即其结束标签
			if dropDepth == len(ancestors) {
				spans = append(spans, span{start: dropStart, end: int(offEnd)})
				dropDepth = -1
			}
		}
		// CharData / Comment / ProcInst / Directive：被删子树内随子树删除，其余原样保留
	}
	if len(spans) == 0 {
		return data, nil
	}
	out := make([]byte, 0, len(data))
	prev := 0
	for _, sp := range spans {
		out = append(out, data[prev:sp.start]...)
		prev = sp.end
	}
	return append(out, data[prev:]...), nil
}

// resolveRelTarget 解析关系目标为包内 part 名：Target 相对关系源 part 所在
// 目录，以 / 开头为包根绝对路径；path.Join 顺带清洗 ..（如 slide rels 的
// ../media/image1.png）。空 Target 由调用方先行拦截。
func resolveRelTarget(sourcePart, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}
	return path.Join(path.Dir(sourcePart), target)
}

// relsPartOf part 的关系文件路径（OPC 约定：<dir>/_rels/<文件名>.rels）。
func relsPartOf(part string) string {
	dir, base := path.Split(part)
	return dir + "_rels/" + base + ".rels"
}

// attrValue 取首个命名空间与局部名都匹配的属性值，未命中返回空串。
// 命名空间按解码器解析后的完整 URI 匹配（与生产方使用的前缀写法无关）。
func attrValue(attrs []xml.Attr, space, local string) string {
	for _, a := range attrs {
		if a.Name.Local == local && a.Name.Space == space {
			return a.Value
		}
	}
	return ""
}

// relIDAttr 取关系命名空间的 id 属性（r:id），页条目靠它关联 presentation.xml.rels。
// 注意不能用「任意命名空间的 id 属性」兜底：sldId 同时带无前缀的 p:id 与 r:id
// 两个局部名相同的属性，宽松匹配会拿到错误的那个。
func relIDAttr(attrs []xml.Attr) string {
	return attrValue(attrs, pptxRelsNamespace, "id")
}
