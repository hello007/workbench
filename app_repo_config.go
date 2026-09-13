package main

import (
	"workbench/model"
)

// ===== 仓库列表配置（工作目录 + 收藏夹）导入导出域 =====

// ExportRepoConfig 导出仓库列表配置（工作目录 + 收藏夹）为 manifest JSON 文本。
// 前端拿到文本后经 SaveFileDialog 选路径 + SaveFile 落盘，后端不耦合用户目录。
func (a *App) ExportRepoConfig() (string, error) {
	return a.repoConfigSvc.Export()
}

// PreviewRepoConfigImport 解析导入文件生成预览，不落盘。
// 全局校验失败（非法 JSON / 不支持的 manifestVersion）返回 AppError（错误码
// 经 ErrorFormatter 序列化为 {code, message}，前端 handleError 分流提示）；
// 逐项非法列入预览 Invalid 由前端展示。冲突项由用户逐项决策后调 ApplyRepoConfigImport。
func (a *App) PreviewRepoConfigImport(jsonText string) (*model.RepoConfigImportPreview, error) {
	return a.repoConfigSvc.PreviewImport(jsonText)
}

// ApplyRepoConfigImport 按用户决策执行导入合并并落盘，返回计数汇总。
// merge 语义保留本机已有项；工作目录覆盖保留本机 ID、另存为新项生成新 ID。
func (a *App) ApplyRepoConfigImport(jsonText string, decisions *model.RepoConfigImportDecisions) (*model.RepoConfigImportResult, error) {
	return a.repoConfigSvc.ApplyImport(jsonText, decisions)
}
