package model

// AiTaskHistory 历史记录元数据（完整输出单独存文件，按需懒加载）。
// 任务完成（成功/失败/取消/超时）后由 service 层归档，复用第 1 批的 AiTaskMetrics
// 展示耗时/token/成本/轮次。输出文件经 os.Rename 零拷贝从 data/ai_task_output/
// 移入 data/ai_task_history/<id>.txt，元数据只存路径与大小，无大对象过 IPC。
type AiTaskHistory struct {
	ID         string         `json:"id"`         // 任务 id（与 aiTaskRuntime.id 一致）
	FunctionID string         `json:"functionId"` // 功能项 id
	Name       string         `json:"name"`       // 功能名快照（功能项改名后历史仍展示原名）
	Prompt     string         `json:"prompt"`     // prompt 预览（截断，供列表/详情展示）
	StartedAt  int64          `json:"startedAt"`  // unix 毫秒，任务开始时间
	FinishedAt int64          `json:"finishedAt"` // unix 毫秒，任务结束时间
	Status     string         `json:"status"`     // success / failed / canceled / timeout
	ExitCode   int            `json:"exitCode"`
	Error      string         `json:"error"`             // 失败/取消/超时原因
	SessionID  string         `json:"sessionId"`         // claude 会话 id，供事后 --resume（本批仅存储，不接入入口）
	Metrics    *AiTaskMetrics `json:"metrics,omitempty"` // 计量摘要（复用 AiTaskMetrics）
	OutputFile string         `json:"outputFile"`        // 输出文件相对路径（data/ai_task_history/<id>.txt）
	OutputSize int64          `json:"outputSize"`        // 输出字节数（列表展示用，详情按需读文件）
}

// MeetingTable 预解析的 markdown 表格，供表格视图直接渲染，避免前端从截断后的
// 输出文本重新解析导致表格丢失。解析规则与前端 parseMarkdownTable 一致：
// 取最后一个连续 |...| 行块，跳过 --- 分隔行后首行为表头、其余为数据行。
type MeetingTable struct {
	Headers []string            `json:"headers"` // 表头列表（保留原列序）
	Rows    []map[string]string `json:"rows"`    // 数据行，按表头名映射为 { 列名: 值 }
}

// AiTaskHistoryFilter 历史列表查询筛选条件。所有字段为空时返回全部历史。
// 时间范围用 unix 毫秒闭区间（StartedAt 落在 [From, To] 内）。
type AiTaskHistoryFilter struct {
	FunctionID string `json:"functionId,omitempty"` // 功能项 id 精确匹配，空表示不限
	Status     string `json:"status,omitempty"`     // 状态精确匹配，空表示不限
	From       int64  `json:"from,omitempty"`       // 起始时间（unix 毫秒），0 表示不限
	To         int64  `json:"to,omitempty"`         // 结束时间（unix 毫秒），0 表示不限
}

// AiTaskHistoryClearCriteria 批量清理条件，满足任一条件的记录被清理。
// OlderThan 与 KeepRecent 互斥：OlderThan 按天数清理（清理 N 天前），
// KeepRecent 保留最近 N 条、清理其余。
type AiTaskHistoryClearCriteria struct {
	OlderThanDays int    `json:"olderThanDays,omitempty"` // 清理 N 天前的记录（按 FinishedAt）
	KeepRecent    int    `json:"keepRecent,omitempty"`    // 仅保留最近 N 条，清理其余
	FunctionID    string `json:"functionId,omitempty"`    // 限定功能项，空表示不限
}
