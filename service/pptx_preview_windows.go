//go:build windows

// PPTX→PDF 转换 Windows COM 实现（go-ole，late binding）。
//
// 转换任务排进 assets 缩略图同一 goOleThumbExporter STA worker（单队列串行，
// 复用 ensureApp / ownedPIDs / 超时杀进程 / 0x8001010A 重试基建——契约见
// docs/spec/assets_thumbs_com.md，禁止新建第二个 COM STA 队列）。
//
// 技术依据：.trellis/tasks/10-20-pptx-com-pdf-pdfjs-viewer/research/com-saveas-pdf.md：
//   - 首选 Presentation.SaveAs(pdfAbs, ppSaveAsPDF=32)：ReadOnly 无窗口打开后
//     调用合法（ReadOnly 只阻止写回源文件，不阻止导出）；静默覆盖已存在目标；
//     保存语义副作用（FullName 重定向 + Saved 置 true）对「自会话打开 → Close」
//     短链路无影响；
//   - WPS 对 32 值无权威佐证（24 值已真机实证、32 待冒烟）——SaveAs 失败回退
//     ExportAsFixedFormat(pdfAbs, ppFixedFormatTypePDF=2)（发布语义，官方明文
//     「等价于 UI 的 Save As PDF or XPS」；late binding 15 参封送复杂，仅传
//     Path+FixedFormatType 两参，尾部可选参数由 COM 缺省）；
//   - 复用用户已开实例的 Presentation 禁 SaveAs（保存语义会把用户会话的
//     FullName 重定向到 PDF，用户后续 Ctrl+S 将写坏其文档）——直接走
//     ExportAsFixedFormat（发布语义，不动文件关联）；
//   - 源文件零写回由 ReadOnly 本身保证（Save 报错/拒绝），Close 无需 Save 处理
//     （SaveAs 后 Saved 置 true、Close 无保存提示；PDF 已落盘）。
//
// 渲染预期行为（research §3，均注释登记不做处理）：隐藏页默认不导出（SaveAs
// 无参可控）；嵌入视频渲染为海报帧/首帧占位；动画渲染为结束后的终态画面；
// 超链接默认保留为 PDF 内可点击链接。

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// ppSaveAsPDF / ppFixedFormatTypePDF COM 层字面量（注释语义——VBA 常量不可引用，
// 取值来源 research/com-saveas-pdf.md §1.1/§1.2 官方枚举表）。
const (
	ppSaveAsPDF          = 32
	ppFixedFormatTypePDF = 2
)

// ConvertToPdf PptxPdfConverter 接口实现（主侧）：提交转换任务到 STA worker
// 并等待结果。转换属用户可感知交互（文件树点开 pptx 预览首开延迟），走
// submitUrgent 严格优先通道——批量缩略图导图在途时插队，先于 FIFO 存量缩略图
// 任务被消费（单 worker 契约不变，正在执行的导图任务不可打断）。ctx 触发
// 超时/取消时经 waitFor 杀自启实例让阻塞 Invoke 返回。
func (e *goOleThumbExporter) ConvertToPdf(ctx context.Context, srcPath, dstPath string) error {
	resCh := make(chan error, 1)
	task := func(w *comWorkerState) {
		resCh <- w.convertToPdfSession(ctx, srcPath, dstPath)
	}
	if err := e.submitUrgent(ctx, task); err != nil {
		return err
	}
	err := e.waitFor(ctx, resCh)
	if isComClassError(err) {
		e.markUnavailable()
	}
	if err != nil {
		// 失败隔离对齐缩略图队列契约「单文件失败（含超时强杀）→ Discard 置脏，
		// 下任务重建会话」：转换任务无队列层承担该职责，主侧自行置脏——超时
		// 杀进程后 w.app 代理指向已死实例，不置脏则后续任务（缩略图批次/跳转/
		// 转换）持续撞死代理逐个失败；置脏后 ensureApp 重建自愈。复用用户实例
		// 场景 ownedPIDs 空，重建链 releaseApp 不会 Quit 用户文档（归属判定
		// 保护不变）。
		e.Discard()
	}
	return err
}

// convertToPdfSession 单文件转换会话（worker STA 线程内执行）：绝对路径校验 →
// 先删旧产物 → ReadOnly 无窗口打开（复用检查）→ SaveAs PDF（失败回退
// ExportAsFixedFormat；复用用户实例直接走回退法）→ 校验产物存在且非空。
// Close/Release 收口沿用 exportSession 先例（defer LIFO：Close 先于 Release）。
func (w *comWorkerState) convertToPdfSession(ctx context.Context, srcPath, dstPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// SaveAs/Open 的路径参数由 COM 服务器进程按自身 cwd 解析（历史 P0：相对
	// 路径 = 确定性「发生意外」），凡交给 COM 的调用一律先 Abs 收口。
	src, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("源路径解析失败: %w", err)
	}
	dst, err := filepath.Abs(dstPath)
	if err != nil {
		return fmt.Errorf("目标路径解析失败: %w", err)
	}
	// 先删旧产物：调用方校验「产物存在且非空」只对本次转换结果负责（失败重试
	// 场景下旧残留文件会掩盖失败）
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理旧 PDF 产物失败: %w", err)
	}
	// 目标父目录预建（转换编排层已 MkdirAll，此处幂等兜底）
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("创建 PDF 缓存目录失败: %w", err)
	}

	app, err := w.ensureApp()
	if err != nil {
		return err
	}
	// 导出场景与缩略图同参：ReadOnly(msoTrue) + Untitled(msoFalse) +
	// WithWindow(msoFalse)（无窗口，SaveAs/ExportAsFixedFormat 均不依赖窗口视图）
	pres, reused, err := w.findOrOpen(app, src, msoTrue, msoFalse)
	if err != nil {
		return err
	}
	defer pres.Release()
	if !reused {
		// SaveAs 的保存语义副作用（FullName 重定向）对本会话自有 Presentation
		// 无影响；ReadOnly 打开从根上保证源文件零写回，Close 无需 Save 处理
		// （SaveAs 后 Saved 置 true，Close 无保存提示，PDF 已落盘）。
		defer oleutil.CallMethod(pres, "Close")
	}

	if reused {
		// 复用用户已开实例的 Presentation：禁 SaveAs（保存语义会把用户会话的
		// 文件关联重定向到 PDF，用户后续 Ctrl+S 将写坏其文档）——直接走
		// ExportAsFixedFormat 发布语义导出。
		return w.exportPdfViaFixedFormat(pres, dst)
	}
	// 首选 SaveAs(ppSaveAsPDF=32)：静默覆盖已存在目标；WPS 对 32 值无权威佐证
	// （research §2.2，待真机冒烟），失败回退 ExportAsFixedFormat。
	if _, err := oleutil.CallMethod(pres, "SaveAs", dst, ppSaveAsPDF); err == nil {
		return verifyPdfProduced(dst)
	} else {
		Logger().Warn("SaveAs PDF 失败，回退 ExportAsFixedFormat", "path", src,
			"hr", fmt.Sprintf("0x%08X", oleErrorHresult(err)), "err", err)
	}
	return w.exportPdfViaFixedFormat(pres, dst)
}

// exportPdfViaFixedFormat 回退转换法：Presentation.ExportAsFixedFormat(Path,
// FixedFormatType=ppFixedFormatTypePDF=2)。官方语义「等价于 UI 的 Save As PDF
// or XPS」发布命令，不动 Presentation 文件关联（复用用户实例场景专用 + SaveAs
// 回退共用）。尾部 13 个可选参数（Intent/PrintHiddenSlides/PrintRange 等）经
// late binding 缺省（PrintHiddenSlides 默认 msoFalse = 隐藏页不导出）。
func (w *comWorkerState) exportPdfViaFixedFormat(pres *ole.IDispatch, dst string) error {
	if _, err := oleutil.CallMethod(pres, "ExportAsFixedFormat", dst, ppFixedFormatTypePDF); err != nil {
		return fmt.Errorf("ExportAsFixedFormat 转换 PDF 失败 (hr=0x%08X): %w", oleErrorHresult(err), err)
	}
	return verifyPdfProduced(dst)
}

// verifyPdfProduced 转换产物校验（存在且非空）。
func verifyPdfProduced(dst string) error {
	info, err := os.Stat(dst)
	if err != nil {
		return fmt.Errorf("PDF 产物未生成: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("PDF 产物为空文件")
	}
	return nil
}
