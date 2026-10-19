package service

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"workbench/model"
)

// ExportPptxPageSubset 从 .pptx 中导出指定页集合为独立演示文稿（保留页集合导出，
// 购物栏按页合并导出的第一步能力：单文件内多页子集产出合法 pptx，纯 Go OOXML
// 实现，与 SplitPptxPerPageWithProgress 共用拆分核心与 B 档体积 GC）。
//
// 参数语义：
//   - pageNumbers 为 1 起展示页序（sldIdLst 三跳映射后的页序，非 slide 文件名序），
//     重复项去重、顺序无关——输出页序恒为源展示页序中保留页的相对顺序（购物栏
//     跨文件排序语义由后续合并层负责，本函数不做页内重排）；
//   - outPath 为输出文件绝对路径（调用方权威指定），输出目录不存在时自动创建，
//     禁与源文件同路径（防原地覆盖损坏源文件）。
//
// 保留语义：选定页及其备注页、真实渲染链路（页 → 版式 → 母版 → 主题，多母版
// 自动保留）整体保留，未选定页及其备注页、独占媒体、未用版式/母版链路由 GC
// 剪除（见 deleteSetForPages）；presentation.xml 的 custShowLst / 根级 extLst
// 剥离语义与单页拆分一致；GC 整段降级形态（任一保留页链路不可解析）母版全保留。
//
// 输出为原子写（复用 writeSplitOutput：CreateTemp 同目录 → 写入 → Sync →
// Rename），取消口径与拆分一致（ctx 取消错误 %w 包装 context.Canceled，
// errors.Is 判定可用）；失败不留半态（临时文件 defer 清理）。
func ExportPptxPageSubset(ctx context.Context, srcPath string, pageNumbers []int, outPath string) (model.PptxSubsetResult, error) {
	if err := ctx.Err(); err != nil {
		return model.PptxSubsetResult{}, fmt.Errorf("导出已取消: %w", err)
	}
	if strings.TrimSpace(srcPath) == "" {
		return model.PptxSubsetResult{}, errors.New("请先选择要导出的 PPT 文件")
	}
	// .ppt 老二进制格式（OOXML 时代前）与其他扩展名一并拒绝，与拆分入口引导口径统一
	if !strings.EqualFold(filepath.Ext(srcPath), ".pptx") {
		return model.PptxSubsetResult{}, fmt.Errorf("仅支持 .pptx 格式，当前文件：%s", filepath.Base(srcPath))
	}
	if _, err := os.Stat(srcPath); err != nil {
		return model.PptxSubsetResult{}, fmt.Errorf("无法访问 PPT 文件: %w", err)
	}
	if strings.TrimSpace(outPath) == "" {
		return model.PptxSubsetResult{}, errors.New("请先选择导出保存位置")
	}
	if subsetSamePath(srcPath, outPath) {
		return model.PptxSubsetResult{}, errors.New("导出路径不能与源文件相同")
	}
	pages, err := normalizeSubsetPages(pageNumbers)
	if err != nil {
		return model.PptxSubsetResult{}, err
	}

	zr, err := zip.OpenReader(srcPath)
	if err != nil {
		// 加密 pptx 是 OLE 容器而非 zip，与损坏文件走同一引导
		return model.PptxSubsetResult{}, fmt.Errorf("无法读取 PPT 文件（文件可能已损坏或已加密）: %w", err)
	}
	defer zr.Close()

	// 全量解压进内存：与拆分入口同口径（用户主动单文件操作，入口已有文件级校验）
	pkg, err := readZipPackage(&zr.Reader)
	if err != nil {
		return model.PptxSubsetResult{}, fmt.Errorf("读取 PPT 内部数据失败: %w", err)
	}

	displayPages, err := resolveSlidePages(pkg)
	if err != nil {
		return model.PptxSubsetResult{}, err
	}
	// 页号范围校验须在三跳映射之后（展示页序总数以 sldIdLst 为准，非 slide 文件数）
	for _, n := range pages {
		if n < 1 || n > len(displayPages) {
			return model.PptxSubsetResult{}, fmt.Errorf("页码 %d 超出范围（该演示文稿共 %d 页）", n, len(displayPages))
		}
	}
	notesOf, err := resolveNotesSlides(pkg, displayPages)
	if err != nil {
		return model.PptxSubsetResult{}, err
	}
	gc, err := newSplitGCContext(pkg)
	if err != nil {
		return model.PptxSubsetResult{}, err
	}

	keptPages := make([]pptxPage, 0, len(pages))
	keptRids := make(map[string]bool, len(pages))
	keptPageParts := make([]string, 0, len(pages))
	for _, n := range pages {
		page := displayPages[n-1]
		keptPages = append(keptPages, page)
		keptRids[page.rid] = true
		keptPageParts = append(keptPageParts, page.part)
	}
	gcPlan := gc.deleteSetForPages(pkg, displayPages, notesOf, keptPages)
	plan := splitPagePlan{
		keptRids:                    keptRids,
		deleted:                     gcPlan.deleted,
		keptPageParts:               keptPageParts,
		keptMasterRids:              gcPlan.keptMasterRids,
		keptMasterParts:             gcPlan.keptMasterParts,
		keptMasterDroppedLayoutRids: gcPlan.keptMasterDroppedLayoutRids,
	}

	if err := ctx.Err(); err != nil {
		return model.PptxSubsetResult{}, fmt.Errorf("导出已取消: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return model.PptxSubsetResult{}, fmt.Errorf("创建输出目录失败: %w", err)
	}
	if err := writeSplitOutput(pkg, plan, outPath); err != nil {
		return model.PptxSubsetResult{}, err
	}
	return model.PptxSubsetResult{OutputPath: outPath, Pages: pages, PageCount: len(pages)}, nil
}

// normalizeSubsetPages 页号归一化：去重、过滤非正值、升序。全负/零/空输入返回
// 引导错误（调用方入口语义：须至少选定一页）；越界校验由调用方在三跳映射后
// 执行（此处无法得知总页数）。
func normalizeSubsetPages(pageNumbers []int) ([]int, error) {
	seen := make(map[int]bool, len(pageNumbers))
	pages := make([]int, 0, len(pageNumbers))
	for _, n := range pageNumbers {
		if n < 1 || seen[n] {
			continue
		}
		seen[n] = true
		pages = append(pages, n)
	}
	if len(pages) == 0 {
		return nil, errors.New("请先选择要导出的页")
	}
	sort.Ints(pages)
	return pages, nil
}

// subsetSamePath 导出路径与源文件同路径判定：Windows 文件系统大小写不敏感，
// 按 EqualFold 比较；其余平台按字节比较。
func subsetSamePath(srcPath, outPath string) bool {
	srcClean := filepath.Clean(srcPath)
	outClean := filepath.Clean(outPath)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(srcClean, outClean)
	}
	return srcClean == outClean
}
