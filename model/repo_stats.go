package model

// RepoStats 仓库提交统计聚合结果，由后端 service 层基于 commit_history_cache 全量快照
// 聚合产出，前端统计页据此渲染趋势图/贡献者排名/活跃度热力图。
type RepoStats struct {
	// Trend 按聚合粒度（日/周/月）分桶的提交数量序列，按时间升序，供趋势折线/柱状图。
	Trend []TimeBucket `json:"trend"`
	// Contributors 贡献者提交数排名，按 Count 降序，供贡献者柱状图/榜单。
	Contributors []Contributor `json:"contributors"`
	// Heatmap 最近一年按日的提交数量序列（含提交数为 0 的日期），供类 GitHub 草地热力图。
	Heatmap []DayCount `json:"heatmap"`
	// TotalCommits 当前时间窗口内的提交总数。
	TotalCommits int `json:"totalCommits"`
	// DateRange 当前时间窗口的可读描述（如 "最近 30 天"），供 UI 标题展示。
	DateRange string `json:"dateRange"`
	// Granularity Trend 序列的聚合粒度：day/week/month，供前端图轴格式化。
	Granularity string `json:"granularity"`
	// Sampled 标记统计数据是否基于采样而非全量。超限仓库（>5000 commit）无法全量缓存，
	// 取最近 5000 条提交统计，此时 TotalCommits/Contributors 为采样值，前端应提示用户。
	Sampled bool `json:"sampled"`
}

// TimeBucket 趋势图时间桶：一个聚合周期（日/周/月）的起止标识与该周期内提交数。
type TimeBucket struct {
	// Date 桶标识：日粒度为 YYYY-MM-DD，周粒度为本周一日期，月粒度为 YYYY-MM。
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// Contributor 贡献者提交统计：按作者名称+邮箱归一分组。
type Contributor struct {
	Author string `json:"author"`
	Email  string `json:"email"`
	Count  int    `json:"count"`
}

// DayCount 热力图单日提交数：一个自然日的提交数（0 表示当日无提交）。
type DayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}
