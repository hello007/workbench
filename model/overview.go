package model

// OverviewSnapshot 个人看板「概览」面板的一次聚合快照（只读，零采集层）。
//
// 三个数据域各自带 Error 字段实现分区降级：单域查询失败只让对应卡片降级，
// 不影响其余卡片。失败禁止静默降级为 0（会把「查询失败」伪装成「没有数据」）。
//
// 口径真相源：docs/spec/dashboard-metrics.md 的 M1–M5。
type OverviewSnapshot struct {
	// GeneratedAt 快照生成时间 Unix 毫秒，前端据此展示「统计于 HH:mm」；
	// TTL 判定只在后端，前端不自行计时。
	GeneratedAt int64 `json:"generatedAt"`
	// Range 时间范围回显（today/week）；T1 内部固定同时返回今日与本周块。
	Range string `json:"range"`
	// Repos 仓库状态域：M1 待收尾仓库 + M2 久未推进仓库。
	Repos RepoOverview `json:"repos"`
	// AI AI 任务域：M4 成本 + M5 成功率。
	AI AIOverview `json:"ai"`
	// Todos 待办域：M3 待办积压。
	Todos TodoOverview `json:"todos"`
}

// RepoOverview 仓库状态域聚合（M1/M2）。
//
// Missing 与 IsRepo=false 的仓库不计入 PendingCount / StaleCount，
// 分别落到 MissingCount 单列；LastCommitUnix==0 的仓库不计入 StaleCount，
// 落到 NoCommitCount 单列（空仓库/unborn/查询失败无法计算天数）。
type RepoOverview struct {
	// PinnedTotal pin 仓库总数（含失效仓库与无提交仓库）。
	PinnedTotal int `json:"pinnedTotal"`
	// PendingCount M1：Dirty 或 StashCount>0 的仓库数。
	PendingCount int `json:"pendingCount"`
	// StaleCount M2：LastCommitUnix>0 且距今天数超过阈值的仓库数。
	StaleCount int `json:"staleCount"`
	// StaleDays M2 当前生效阈值（天）。
	StaleDays int `json:"staleDays"`
	// MissingCount 路径已失效或非 Git 仓库的数量（仅展示，不计入 M1/M2）。
	MissingCount int `json:"missingCount"`
	// NoCommitCount LastCommitUnix==0 的仓库数（仅展示，不计入 M2）。
	NoCommitCount int `json:"noCommitCount"`
	// Error 仓库域整体失败原因；正常为空。
	Error string `json:"error,omitempty"`
}

// AIOverview AI 任务域聚合（M4/M5）。
//
// 窗口按 StartedAt 过滤；保留裁剪按 FinishedAt，跨天边界任务可能不计入，
// 属已知微小差异（见 docs/spec/dashboard-metrics.md §3）。
type AIOverview struct {
	// Today 今日（本地时区 00:00 起）成本与 token 聚合（M4）。
	Today AICostBlock `json:"today"`
	// Week 本周（本地时区周一 00:00 起）成本与 token 聚合（M4）。
	Week AICostBlock `json:"week"`
	// SuccessRate 本周成功率 = success ÷ (success+failed+timeout)，canceled 不进分母（M5）。
	SuccessRate float64 `json:"successRate"`
	// HasData 本周是否存在有效任务（success+failed+timeout>0）。false 时前端显示
	// 「无数据」而不是 0%，避免把「没有任务」误读为「全部失败」。
	HasData bool `json:"hasData"`
	// Error AI 域整体失败原因（历史文件读取失败等）；正常为空。
	Error string `json:"error,omitempty"`
}

// AICostBlock 单个时间窗口内的 AI 成本与 token 聚合。
//
// TaskCount 统计窗口内任务数（含 Metrics 为 nil 的异常记录）；
// 成本与 token 仅对 Metrics 非 nil 的记录累加。
type AICostBlock struct {
	CostUSD             float64 `json:"costUsd"`
	TaskCount           int     `json:"taskCount"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
}

// TodoOverview 待办域聚合（M3）。
//
// 只统计未完成条目（CompletedAt==nil）；已完成条目的保留策略不影响本指标。
type TodoOverview struct {
	// PendingCount 未完成待办条数。
	PendingCount int `json:"pendingCount"`
	// OldestDays 最老未完成条目已滞留天数（向下取整，最小 0）。
	OldestDays int `json:"oldestDays"`
	// OldestID 最老未完成条目 ID，供下钻定位；无未完成条目时为空。
	OldestID string `json:"oldestId"`
	// OldestText 最老未完成条目正文快照，供卡片提示；无条目时为空。
	OldestText string `json:"oldestText"`
	// Error 待办域读取失败原因；正常为空。
	Error string `json:"error,omitempty"`
}
