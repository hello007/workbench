# AI 任务历史统计与导出报告

**日期**：2026-09-09
**优先级**：P1（总览第 8 节降级理由「P1-2 先做展示」已不成立——第 3 批历史归档展示已就位，数据在手边被丢弃）
**状态**：待评审

## 1. 概述

`data/ai_task_history.json` 已落地任务运行历史归档（第 3 批），`AiTaskHistory` 含
token 用量、成本、耗时、功能 id 等完整计量字段，前端 `AiTaskHistoryPanel.vue`
仅做列表展示。本设计在已有数据上增加聚合统计与导出，兑现数据价值：按周/月统计
token 消耗与成本、按功能项统计使用频次、一键导出 CSV/Markdown 报告。

## 2. 现状与痛点

`AiTaskHistoryService.List`（service/ai_task_history.go:157）按筛选条件返回记录列表，
前端 `AiTaskHistoryPanel.vue` 第 58-98 行用 `el-table` 平铺渲染，仅展示单条耗时/成本/输出大小。

| 痛点 | 说明 |
| --- | --- |
| 无聚合 | 看不到「本月花了多少 token/钱」「周报功能本月用了几次」 |
| 无导出 | 月度复盘只能手抄，无法生成报告 |
| 数据闲置 | token/成本/耗时全量存了，但只做单条回显，聚合价值未兑现 |

> **关键**：聚合所需字段（functionId / StartedAt / FinishedAt / Metrics.usage 四 token 字段 /
> Metrics.costUsd / Metrics.durationMs）全部已在 `AiTaskHistory`，无需新增存储或采集。

## 3. 需求总结

1. 历史面板加统计区：按筛选范围汇总运行次数、总 token（入+出+缓存读）、总成本、总耗时
2. 按功能项维度统计：每个功能项的运行次数与总成本，展示为排行/分布
3. 时间维度统计：按天/周/月分桶的 token 与成本趋势（简单柱状或表格）
4. 导出报告：当前筛选范围的历史导出为 CSV（明细）与 Markdown（统计摘要+排行）

## 4. 设计要点

### 4.1 聚合模型

`model/ai_task_history.go` 新增聚合结果结构（后端算好返回，前端只渲染）：

```go
// AiTaskHistoryStats 历史聚合统计（按筛选范围汇总）
type AiTaskHistoryStats struct {
    TotalCount   int     `json:"totalCount"`   // 记录总数
    SuccessCount int     `json:"successCount"` // 成功数
    TotalCostUSD float64 `json:"totalCostUsd"` // 总成本
    TotalInputTokens  int `json:"totalInputTokens"`
    TotalOutputTokens int `json:"totalOutputTokens"`
    TotalCacheReadTokens int `json:"totalCacheReadTokens"`
    TotalDurationMs int64 `json:"totalDurationMs"` // 总耗时（仅成功任务有计量）
    ByFunction []FunctionStat `json:"byFunction"` // 按功能项聚合
}

// FunctionStat 单个功能项的统计
type FunctionStat struct {
    FunctionID   string  `json:"functionId"`
    FunctionName string  `json:"functionName"` // 取最近一条的 name 快照（功能改名取最新）
    Count        int     `json:"count"`
    TotalCostUSD float64 `json:"totalCostUsd"`
    TotalTokens  int     `json:"totalTokens"` // 入+出
}
```

### 4.2 聚合方法

`AiTaskHistoryService` 新增 `Stats(filter *model.AiTaskHistoryFilter) (*model.AiTaskHistoryStats, error)`：
复用 `List(filter)` 结果在内存聚合（量级 2000 条上限，无性能压力）。
metrics 为 nil 的记录（异常 result）跳过计量累加，但仍计入 count。

### 4.3 导出方法

`AiTaskHistoryService` 新增导出（复用 filter 查询后格式化）：
- `ExportCSV(filter) (string, error)`：返回 CSV 文本，列含 时间/功能/状态/耗时/成本/入token/出token/输出大小
- `ExportMarkdown(filter) (string, error)`：返回 Markdown，含统计摘要表 + 功能排行表 + 明细表

导出文本经 App 方法返回前端，前端用文件保存对话框落盘（复用既有 SaveFile 机制或
Wails runtime.SaveFileDialog）。避免后端直接写文件路径耦合用户目录。

### 4.4 前端面板改造

`AiTaskHistoryPanel.vue` 顶部筛选区下方加统计卡片区（4 个数字卡：次数/总成本/总token/总耗时），
功能区右侧加「导出 CSV」「导出 Markdown」按钮。统计随筛选条件查询时一并拉取（Stats 与 List 并行请求）。

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `model/ai_task_history.go` | 新增 | `AiTaskHistoryStats` / `FunctionStat` 结构 |
| `service/ai_task_history.go` | 修改 | 新增 `Stats` / `ExportCSV` / `ExportMarkdown` 方法 |
| `service/ai_task_history_test.go` | 修改 | 聚合计算、metrics 为 nil 跳过、导出格式测试 |
| `app.go` | 修改 | 新增 `GetAiTaskHistoryStats` / `ExportAiTaskHistoryCSV` / `ExportAiTaskHistoryMarkdown` 三个 App 方法 |
| `frontend/wailsjs/go/main/App.js` + `App.d.ts` | 同步 | 三个新方法签名同步 |
| `frontend/wailsjs/go/models.ts` | 同步 | `AiTaskHistoryStats` / `FunctionStat` class |
| `frontend/src/components/AiTaskHistoryPanel.vue` | 修改 | 统计卡片区 + 导出按钮 |
| `frontend/src/components/__tests__/AiTaskHistoryPanel.spec.js` | 修改 | 统计展示、导出调用测试 |

## 6. 待确认点

1. 时间维度趋势（4.1 的按天/周/月分桶）是否本批做，还是先只做汇总+功能排行？
   建议先做汇总+功能排行（收益明确、改动小），趋势图等真实复盘需求出现再加
2. 导出落盘方式：用 Wails `runtime.SaveFileDialog`（用户选目录）还是直接写固定目录？
   建议用 SaveFileDialog（用户可控，符合桌面应用习惯）
3. 导出范围：当前筛选范围导出 vs 全部历史导出？建议当前筛选范围（与列表一致，所见即所导）

## 7. 关联

- 来源：[[2026-09-08-ai-optimization-overview]] 第 8 节降级项（「历史导出报告 P1-2 先做展示，
  月度统计导出属后续增强」——展示已就位，可提回）
- 依赖：[[2026-09-08-ai-run-history-design]]（历史归档已实施，本设计复用其数据）、
  [[2026-09-08-ai-task-observability-design]]（metrics 字段已采集）
- 配套：使用频次智能排序（[[2026-09-09-ai-usage-frequency-sort]]）同样复用历史数据聚合
