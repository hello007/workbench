//go:build !windows

// PPTX→PDF 转换非 Windows 桩（linux-platform 契约：编译期字段独有走
// _windows/_other 双文件分叉）。PowerPoint/WPS COM 仅存在于 Windows，
// 非 Windows 平台预览转换整体降级（Available 恒 false → 编排层返回
// E_PPTX_PREVIEW_UNAVAILABLE，前端保留降级提示 + 用默认程序打开）。

package service

import "context"

// ConvertToPdf PptxPdfConverter 接口桩实现：非 Windows 无 COM 能力恒降级。
func (unavailableThumbExporter) ConvertToPdf(_ context.Context, _, _ string) error {
	return errThumbsUnavailable
}
