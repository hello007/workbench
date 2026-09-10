package main

import (
	"fmt"

	"workbench/model"
	"workbench/service"
)

// ===== AI 功能（skill 聚合触发）相关 =====

// GetAiFunctions 获取 AI 功能项列表；配置文件不存在时自动写入并返回默认四项
func (a *App) GetAiFunctions() ([]*model.AiFunction, error) {
	return a.aiFuncSvc.LoadAiFunctions()
}

// SaveAiFunctions 保存 AI 功能项列表（配置管理界面增删改后调用）
func (a *App) SaveAiFunctions(funcs []*model.AiFunction) error {
	return a.aiFuncSvc.SaveAiFunctions(funcs)
}

// ExportAiFunctions 导出当前全部功能项为 schema v2 JSON 文本。
// 前端拿到文本后用 Wails runtime.SaveFileDialog 选路径落盘；后端不耦合用户目录。
// env 的 $ENV: 引用与 MCP headers 原样导出，前端导出前提示用户确认共享范围。
func (a *App) ExportAiFunctions() (string, error) {
	return a.aiFuncSvc.ExportAiFunctions()
}

// ImportAiFunctions 解析外部 JSON 配置并生成导入预览，不落盘。
// 复用 migrateFunctions 迁移补全 + validateFunctions 校验，与本机已加载项按 id 比对
// 生成 New/Conflict/Invalid 三类。前端展示预览、用户决策冲突策略后调 SaveAiFunctions 合并落盘。
func (a *App) ImportAiFunctions(jsonText string) (*model.ImportPreview, error) {
	return a.aiFuncSvc.ImportAiFunctions(jsonText)
}

// GetDiscoveredSkills 获取已发现的 skill 列表（带 mtime 缓存）。
// 扫描用户级 ~/.claude/skills + 各工作目录 .claude/skills + 已安装插件 skills，
// 解析 SKILL.md frontmatter 去重后返回，供配置对话框「导入 skill」入口回填 command/cwd/description/name。
func (a *App) GetDiscoveredSkills() []*model.SkillDescriptor {
	if a.skillDiscoverySvc == nil {
		return []*model.SkillDescriptor{}
	}
	return a.skillDiscoverySvc.GetCached()
}

// RefreshDiscoveredSkills 强制重扫已发现 skill 列表（清除缓存），供导入对话框「刷新」按钮调用。
func (a *App) RefreshDiscoveredSkills() []*model.SkillDescriptor {
	if a.skillDiscoverySvc == nil {
		return []*model.SkillDescriptor{}
	}
	return a.skillDiscoverySvc.Refresh()
}

// RunAiFunction 运行功能项主段：按参数规格组装 prompt 后起 claude 子进程。
// params 为参数值（file/text 为单值 key，form 为字段 key->值），无参数传 nil。
// 返回任务 id；输出经 Wails 事件 ai-task:output / ai-task:done 推送。
func (a *App) RunAiFunction(functionID string, params map[string]string) (string, error) {
	fn, err := a.aiFuncSvc.LoadAiFunctions()
	if err != nil {
		return "", err
	}
	var target *model.AiFunction
	for _, f := range fn {
		if f.ID == functionID {
			target = f
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("AI 功能 %s 不存在", functionID)
	}
	prompt, err := service.BuildStagePrompt(target.Command, target.Params, params)
	if err != nil {
		return "", err
	}
	return a.aiFuncSvc.RunStage(functionID, prompt, "")
}

// RunAiFollowUp 运行后续段（多段编排）：在原任务会话上 --resume 继续发送。
// taskID 为主段任务 id，followUpID 为功能项中定义的后续段 id。
func (a *App) RunAiFollowUp(taskID, followUpID string, params map[string]string) (string, error) {
	state := a.aiFuncSvc.GetAiTaskState(taskID)
	if state == nil {
		return "", fmt.Errorf("任务 %s 不存在", taskID)
	}
	if state.SessionID == "" {
		return "", fmt.Errorf("原任务无会话 id，无法续段（可能未产生任何输出即失败）")
	}
	fn, err := a.aiFuncSvc.LoadAiFunctions()
	if err != nil {
		return "", err
	}
	var target *model.AiFunction
	for _, f := range fn {
		if f.ID == state.FunctionID {
			target = f
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("功能项 %s 不存在", state.FunctionID)
	}
	var followUp *model.AiFollowUp
	for i := range target.FollowUps {
		if target.FollowUps[i].ID == followUpID {
			followUp = &target.FollowUps[i]
			break
		}
	}
	if followUp == nil {
		return "", fmt.Errorf("后续段 %s 不存在", followUpID)
	}
	prompt, err := service.BuildFollowUpPrompt(followUp, params)
	if err != nil {
		return "", err
	}
	return a.aiFuncSvc.RunStage(state.FunctionID, prompt, state.SessionID)
}

// CancelAiTask 取消运行中的任务（杀 claude 及其子进程树）
func (a *App) CancelAiTask(taskID string) bool {
	return a.aiFuncSvc.CancelAiTask(taskID)
}

// GetAiTaskState 查询任务状态（输出内容、会话 id、运行态）
func (a *App) GetAiTaskState(taskID string) *model.AiTaskState {
	return a.aiFuncSvc.GetAiTaskState(taskID)
}

// GetAiConcurrencyStatus 查询全局并发占用（运行中/排队中/上限），供前端标题栏展示「N/M」
func (a *App) GetAiConcurrencyStatus() model.AiConcurrencyStatus {
	return a.aiFuncSvc.GetConcurrencyStatus()
}

// RemoveAiTask 清理已完成/已取消任务的后端 runtime（前端 Tab 关闭时调用）。
// 运行中或排队中的任务不可清理（前端应禁止关闭运行中 Tab）。
func (a *App) RemoveAiTask(taskID string) bool {
	return a.aiFuncSvc.RemoveAiTask(taskID)
}

// GetAiTaskOutput 全量读取任务输出文件（前端 copy/preview/表格视图按需拉取，不依赖已截断的展示文本）。
// 任务仍在 map 读其输出文件；已清理则读归档目录的历史输出文件。
func (a *App) GetAiTaskOutput(taskID string) (string, error) {
	return a.aiFuncSvc.GetAiTaskOutput(taskID)
}

// GetAiTaskHistory 查询历史列表（按筛选条件，空 filter 返回全部，按完成时间降序）。
func (a *App) GetAiTaskHistory(filter model.AiTaskHistoryFilter) []*model.AiTaskHistory {
	list, _ := a.aiFuncSvc.GetAiTaskHistory(&filter)
	if list == nil {
		return []*model.AiTaskHistory{}
	}
	return list
}

// GetAiTaskHistoryOutput 读取单条历史的归档输出文件全文（历史详情查看输出走此路径，懒加载）。
func (a *App) GetAiTaskHistoryOutput(id string) (string, error) {
	return a.aiFuncSvc.GetAiTaskHistoryOutput(id)
}

// DeleteAiTaskHistory 删除单条历史（元数据 + 归档输出文件）。
func (a *App) DeleteAiTaskHistory(id string) bool {
	return a.aiFuncSvc.DeleteAiTaskHistory(id)
}

// ClearAiTaskHistory 按条件批量清理历史（OlderThanDays 按天数清理 / KeepRecent 保留最近 N 条），返回清理条数。
func (a *App) ClearAiTaskHistory(criteria model.AiTaskHistoryClearCriteria) int {
	n, _ := a.aiFuncSvc.ClearAiTaskHistory(&criteria)
	return n
}

// GetAiTaskHistoryStats 历史聚合统计（按筛选范围汇总次数/成本/token 四分项/耗时 + 功能排行）。
// historySvc 未初始化时返回零值统计，前端空态展示。
func (a *App) GetAiTaskHistoryStats(filter model.AiTaskHistoryFilter) *model.AiTaskHistoryStats {
	stats, err := a.aiFuncSvc.GetAiTaskHistoryStats(&filter)
	if err != nil || stats == nil {
		return &model.AiTaskHistoryStats{ByFunction: []model.FunctionStat{}}
	}
	return stats
}

// GetFunctionUsageCounts 各功能项运行次数聚合（functionId → 次数，canceled 不计入）。
// 供「AI 功能」列表按频次排序，避免全量历史元数据过 IPC。
func (a *App) GetFunctionUsageCounts() map[string]int {
	counts, err := a.aiFuncSvc.GetFunctionUsageCounts()
	if err != nil {
		println("Error:", err.Error())
		return map[string]int{}
	}
	return counts
}

// ExportAiTaskHistoryCSV 导出当前筛选范围的历史明细 CSV 文本（UTF-8 BOM 开头）。
// 落盘由前端经 SaveFileDialog 选路径后调 SaveFile 完成，后端不直接写文件。
func (a *App) ExportAiTaskHistoryCSV(filter model.AiTaskHistoryFilter) (string, error) {
	return a.aiFuncSvc.ExportAiTaskHistoryCSV(&filter)
}

// ExportAiTaskHistoryMarkdown 导出当前筛选范围的历史报告 Markdown 文本（统计摘要 + 功能排行 + 明细）。
func (a *App) ExportAiTaskHistoryMarkdown(filter model.AiTaskHistoryFilter) (string, error) {
	return a.aiFuncSvc.ExportAiTaskHistoryMarkdown(&filter)
}
