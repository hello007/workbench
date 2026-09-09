# AI 任务历史统计与导出报告

## Goal

在已落地的 `data/ai_task_history.json` 历史归档数据上增加聚合统计与导出能力，兑现数据价值：
按筛选范围汇总运行次数/总 token/总成本/总耗时，按功能项统计使用频次与成本排行，
一键导出 CSV（明细）与 Markdown（统计摘要 + 排行）报告，支撑月度复盘。

## What I already know

- `AiTaskHistoryService.List(filter)`（service/ai_task_history.go:157）已实现按 FunctionID/Status/From/To 筛选，按 FinishedAt 降序返回，量级上限 2000 条（enforceRetention），内存聚合无性能压力
- `AiTaskHistory` 结构（model/ai_task_history.go:7）含 FunctionID/Name/StartedAt/FinishedAt/Status/Metrics/OutputFile/OutputSize
- `AiTaskMetrics`（model/ai_function.go:100）：`Usage *AiTaskUsage`（4 字段：InputTokens/OutputTokens/CacheCreationInputTokens/CacheReadInputTokens）+ DurationMs + CostUSD + NumTurns
- `AiTaskHistoryFilter`（model/ai_task_history.go:33）：FunctionID/Status/From/To（unix 毫秒闭区间）
- 现有 App 方法（app.go:1367-1390）：GetAiTaskHistory / GetAiTaskHistoryOutput / DeleteAiTaskHistory / ClearAiTaskHistory，命名风格「动词+AiTaskHistory+名词」
- wailsjs 绑定现状：App.d.ts 已有 4 个 AiTaskHistory 方法签名，models.ts 已有 AiTaskHistory/AiTaskMetrics 等 class
- 前端 AiTaskHistoryPanel.vue：筛选区（功能/时间范围/状态/查询/清理）+ el-table 列表 + 详情抽屉，已监听 ai-task:archived 事件刷新
- metrics 为 nil 的记录（异常 result）需跳过计量累加但仍计入 count（plan 4.2 明确）
- 项目尚无 SaveFileDialog 代码先例，配套 ai-config-import-export 也计划用同模式（共享新模式）

## Assumptions (temporary)

- 聚合复用 List(filter) 结果在内存计算，不新增存储或采集
- 导出文本经 App 方法返回前端，前端用 Wails runtime.SaveFileDialog 落盘（不耦合用户目录）
- 导出范围为当前筛选范围（与列表一致，所见即所导）

## Decisions (已确认，用户「都按推荐」)

1. **时间维度趋势本批不做**——只做汇总数字卡 + 功能排行 + 导出。趋势图等真实复盘需求出现再加。→ Out of Scope
2. **总 token 保留 4 个分项 Total，不合并丢失口径**——plan 4.1 原只列 Input/Output/CacheRead 三个 Total，漏了 CacheCreationInputTokens。`AiTaskUsage` 实有 4 字段，Anthropic 计费 cache_creation 也是实际消耗，应单独累加保留。`AiTaskHistoryStats` 补 `TotalCacheCreationTokens` 字段。
3. **导出落盘用 Wails runtime.SaveFileDialog**——前端拿到后端返回的文本后调 SaveFileDialog 让用户选路径落盘，后端不直接写文件、不耦合用户目录。与配套 ai-config-import-export 共享模式。
4. **导出范围为当前筛选范围**——与列表一致，所见即所导。

## Requirements (evolving)

- 历史面板顶部加统计区：按当前筛选范围汇总运行次数、成功数、总成本、总 token、总耗时
- 按功能项维度统计：每个功能项运行次数与总成本，展示为排行
- 导出报告：当前筛选范围导出 CSV（明细）与 Markdown（统计摘要 + 功能排行 + 明细）
- 后端新增 AiTaskHistoryStats / FunctionStat 聚合结构，service 层 Stats/ExportCSV/ExportMarkdown 方法
- app.go 新增 GetAiTaskHistoryStats / ExportAiTaskHistoryCSV / ExportAiTaskHistoryMarkdown 三个 App 方法
- wailsjs 绑定三处同步（App.js / App.d.ts / models.ts）

## Acceptance Criteria (evolving)

- [ ] 统计区 4 个数字卡随筛选查询一并刷新（次数/总成本/总token/总耗时）
- [ ] 功能项排行展示每个功能的运行次数与总成本
- [ ] metrics 为 nil 的记录计入 count 但不计入计量累加
- [ ] 导出 CSV 含列：时间/功能/状态/耗时/成本/入token/出token/输出大小
- [ ] 导出 Markdown 含统计摘要表 + 功能排行表 + 明细表
- [ ] 导出经 SaveFileDialog 让用户选路径落盘
- [ ] 后端单元测试覆盖：聚合计算、metrics nil 跳过、导出格式
- [ ] 前端测试覆盖：统计展示、导出按钮调用
- [ ] wailsjs 绑定三处同步

## Definition of Done

- 后端 go test ./... 通过
- 前端 cd frontend && npm test 通过
- wailsjs 绑定三处同步（App.js / App.d.ts / models.ts）
- README.md 按需更新（功能说明.md 历史面板章节补充统计与导出）
- 跨层契约遵守（app.go 方法签名变更同步 wailsjs）

## Out of Scope (explicit)

- 时间维度趋势图（按天/周/月分桶）——待确认，倾向本批不做
- 使用频次智能排序（配套任务 ai-usage-frequency-sort，独立实施）
- 后端直接写固定目录落盘

## Technical Notes

- 来源 plan：docs/plans/2026-09-09-ai-history-stats-export.md
- 关联：[[2026-09-08-ai-optimization-overview]] 第 8 节降级项提回、[[2026-09-08-ai-run-history-design]] 历史归档已实施、[[2026-09-08-ai-task-observability-design]] metrics 已采集
- 配套：[[2026-09-09-ai-usage-frequency-sort]] 同样复用历史聚合、[[2026-09-09-ai-config-import-export]] 同样用 SaveFileDialog 模式
- 跨层契约：docs/spec/cross-layer-contracts.md（App 方法签名变更须同步 wailsjs 三处）
- CodeGraph 索引：AiTaskHistoryService.List / AiTaskHistory / AiTaskMetrics / AiTaskHistoryFilter 已确认
