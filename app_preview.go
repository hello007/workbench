package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"workbench/model"
)

// ===== 文件预览与对话框域 =====

// PreviewFile 预览文件
func (a *App) PreviewFile(filePath string) *model.FilePreview {
	const maxSize = 1024 * 1024 // 1MB
	preview, err := a.fileOpSvc.PreviewFile(filePath, maxSize)
	if err != nil {
		preview.Error = err.Error()
	}
	return preview
}

// ReadFileBytes 读取文件原始字节（base64），供前端构造 Blob 预览图片/PDF/Office
func (a *App) ReadFileBytes(filePath string) *model.FileBytes {
	const maxSize = 50 * 1024 * 1024 // 50MB（图片/PDF/Office 放宽上限）
	result, err := a.fileOpSvc.ReadFileBytes(filePath, maxSize)
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

// SaveFile 保存文件内容（按 encoding 指定编码写入：gbk 按 GBK，其余按 UTF-8）
func (a *App) SaveFile(filePath string, content string, encoding string) error {
	return a.fileOpSvc.SaveFile(filePath, content, encoding)
}

// SaveFileDialog 弹出原生保存文件对话框，返回用户选择的路径（取消返回空串）。
// 前端导出 CSV/Markdown/JSON 时由前端调此方法选路径，再调 SaveFile 落盘，
// 后端不耦合用户目录。ctx 取自 startup 注入的 a.ctx，filters 为文件类型筛选列表。
func (a *App) SaveFileDialog(defaultFilename string, filters []runtime.FileFilter) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Filters:         filters,
	})
}

// OpenFileDialog 弹出原生打开文件对话框，返回用户选择的路径（取消返回空串）。
// 前端导入配置文件时由前端调此方法选路径，再读文件内容交后端解析。
// ctx 取自 startup 注入的 a.ctx，title 为对话框标题，filters 为文件类型筛选列表。
func (a *App) OpenFileDialog(title string, filters []runtime.FileFilter) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   title,
		Filters: filters,
	})
}
