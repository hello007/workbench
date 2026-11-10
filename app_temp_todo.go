package main

import "workbench/model"

// ===== 临时待办 =====
//
// 委托 TempTodoService：剪贴板快速记录 + 完成归档。
// 跨层契约见 docs/spec/cross-layer-contracts.md：model 新增导出字段须同步
// frontend/wailsjs/ 三处（App.js / App.d.ts / models.ts）。

// ListTempTodos 返回全部待办条目（待办在前按创建时间降序，已完成在后按完成
// 时间降序）。读取时顺带执行保留策略清理，无需前端单独触发。
func (a *App) ListTempTodos() ([]model.TempTodo, error) {
	return a.tempTodoSvc.List()
}

// AddTempTodo 手动新建一条待办。tags 为自由文本标签，空切片表示无标签。
func (a *App) AddTempTodo(text string, tags []string) (model.TempTodo, error) {
	return a.tempTodoSvc.Add(text, tags)
}

// AddTempTodoFromClipboard 读取系统剪贴板文本并落一条待办（快速记录快捷键入口）。
// 剪贴板为空 / 不含文本 / 读取失败分别返回不同错误码，前端据此分流提示。
func (a *App) AddTempTodoFromClipboard() (model.TempTodo, error) {
	return a.tempTodoSvc.AddFromClipboard()
}

// AddTempTodos 批量新建待办（Markdown 导入），一次落盘并返回实际新增条数。
// 空正文条目由 service 跳过；CreatedAt<=0 表示取导入时刻。
func (a *App) AddTempTodos(items []model.TempTodoDraft) (int, error) {
	return a.tempTodoSvc.AddMany(items)
}

// UpdateTempTodo 修改待办正文与标签（不改变创建时间与完成态）。
func (a *App) UpdateTempTodo(id, text string, tags []string) error {
	return a.tempTodoSvc.Update(id, text, tags)
}

// SetTempTodoCompleted 完成 / 取消完成；completed=false 时回到待办区。
func (a *App) SetTempTodoCompleted(id string, completed bool) error {
	return a.tempTodoSvc.SetCompleted(id, completed)
}

// DeleteTempTodo 物理删除一条待办（待办区与已完成区共用）。
func (a *App) DeleteTempTodo(id string) error {
	return a.tempTodoSvc.Delete(id)
}

// ExportTempTodosMarkdown 把全部条目追加导出到 dir 下当天的 YYYY-MM-DD.md，
// 返回写入文件的绝对路径（dir 由前端经 OpenDirectoryDialog 选定）。
// 浏览器通道不提供该能力（main.go RPCHiddenMethods 屏蔽）：允许浏览器侧指定
// 任意路径写文件会突破「本地文件只经桌面出口」的边界。
func (a *App) ExportTempTodosMarkdown(dir string) (string, error) {
	return a.tempTodoSvc.ExportMarkdown(dir)
}
