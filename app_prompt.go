package main

import "workbench/model"

// ListPrompts 列出全部提示词（按创建时间降序）。
func (a *App) ListPrompts() ([]model.Prompt, error) {
	return a.promptSvc.List()
}

// AddPrompt 新建一条提示词（标题与正文均必填）。
func (a *App) AddPrompt(title, content string) (model.Prompt, error) {
	return a.promptSvc.Add(title, content)
}

// UpdatePrompt 修改提示词的标题与正文。
func (a *App) UpdatePrompt(id, title, content string) error {
	return a.promptSvc.Update(id, title, content)
}

// DeletePrompt 删除一条提示词。
func (a *App) DeletePrompt(id string) error {
	return a.promptSvc.Delete(id)
}
