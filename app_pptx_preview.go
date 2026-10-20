package main

import (
	"context"
	"errors"

	"workbench/model"
)

// ===== PPTX 预览转换域（文件树 pptx/ppsx 内嵌预览：COM 转 PDF + pdfjs viewer）=====

// ConvertPptxForPreview 把 .pptx/.ppsx 经本机 PowerPoint/WPS COM 转换为 PDF 并
// 返回缓存路径，前端把 pdfPath 当 kind=pdf 走现成 pdfjs viewer iframe 链路
// （/preview-pdf 同源 handler 零改动复用）。
//
// 返回结构（对齐 model.FilePreview 对错字段模式，不走 Wails error reject 通道）：
//   - 成功：PdfPath = 缓存 PDF 绝对路径（缓存命中零 COM 延迟；未命中首次转换
//     1-3s+，前端渲染 loading 态等待）
//   - 失败：Error = 用户可读中文消息，ErrorCode = 机器可读错误码
//     （E_PPTX_PREVIEW_UNAVAILABLE 环境不可用 / E_PPTX_PREVIEW_FAILED 转换失败，
//     常量表 model/app_error.go + 前端 error.js ErrorCode 两处同步），前端降级
//     提示 + 「用默认程序打开」按钮
//
// 签名说明：不带 ctx 参数——本仓 App 绑定方法无 Wails ctx 注入先例（避免
// `wails generate module` 对 ctx 形态的处理不确定性引入绑定漂移），转换超时
// 60s 由 service 层 context.WithTimeout 兜底（ConvertForPreview 内部）。
func (a *App) ConvertPptxForPreview(filePath string) *model.PptxPreviewResult {
	if a.pptxPreviewSvc == nil {
		return &model.PptxPreviewResult{
			Error:     "预览转换服务未初始化",
			ErrorCode: model.ErrCodePptxPreviewFailed,
		}
	}
	pdfPath, err := a.pptxPreviewSvc.ConvertForPreview(context.Background(), filePath)
	result := &model.PptxPreviewResult{PdfPath: pdfPath}
	if err != nil {
		result.PdfPath = ""
		var appErr *model.AppError
		if errors.As(err, &appErr) {
			result.Error = appErr.Message
			result.ErrorCode = appErr.Code
		} else {
			result.Error = err.Error()
			result.ErrorCode = model.ErrCodePptxPreviewFailed
		}
	}
	return result
}
