# AI 任务可观测性设计 — token / 耗时 / exitCode 展示

**日期**：2026-09-08
**优先级**：P0
**状态**：决策已定，待实施（2026-09-08 抓样确认字段映射）

## 1. 概述

将 `claude -p` 子进程 stream-json 输出中已携带的 `result` 事件计量数据（token 用量、耗时、成本、退出码）提取并在任务 Tab 展示。数据已在子进程 stdout 中，当前被解析器丢弃，本设计为零信息浪费的低成本增强。

## 2. 背景与痛点

执行器 `pumpOutput`（service/ai_function.go:426）逐行读取 stdout 调 `parseStreamLine`（第 398 行），该函数签名仅返回三元组：

```go
func parseStreamLine(line string) (text string, isResult bool, sessionID string)
```

stream-json 的 `result` 终态事件除 `session_id` 外还携带 `usage`（input/output tokens）、`duration_ms`、`cost_usd`、`num_turns` 等，**当前全部丢弃**。

前端 `onDone`（AiFunctionPanel.vue:419）只取 `result.sessionId`，`exitCode`、`error` 未在 Tab 渲染：

| 已有数据 | 当前去向 |
| --- | --- |
| `session_id` | 解析后存 `aiTaskRuntime.sessionID`，前端用于后续段 `--resume` |
| `usage` / token | 丢弃 |
| `duration_ms` | 丢弃 |
| `cost_usd` | 丢弃 |
| `num_turns` | 丢弃 |
| `exitCode` | 存入 `AiTaskRunResult.ExitCode`，前端未展示 |
| `error` | 存入 `AiTaskRunResult.Error`，前端未展示 |

## 3. 需求总结

1. 解析 `result` 事件，提取 token 用量、耗时、成本、轮次
2. 任务完成后面板 Tab 底栏展示计量摘要
3. 失败任务明确展示 exitCode 与错误信息
4. `AiTaskState` 增字段，任务状态恢复（重开应用）后计量不丢

## 4. 设计

### 4.1 result 事件字段抓样

**已抓样确认**（2026-09-08，claude v2.1.158）。抓样命令：

```bash
claude -p "回复一个字：好" --output-format stream-json --verbose 2>&1 | tail -1
```

完整抓样归档于 `docs/plans/samples/result-event-sample.json`。`result` 事件顶层字段：

| 字段 | 类型 | 说明 | 是否采集 |
| --- | --- | --- | --- |
| `type` | string | 固定 `"result"` | 是（终态判定） |
| `subtype` | string | `success` / `error` | 是 |
| `is_error` | bool | 是否出错 | 是 |
| `session_id` | string | claude 会话 id，供后续段 `--resume` | 是（已有） |
| `duration_ms` | int | 总耗时（含本地编排），毫秒 | 是 |
| `duration_api_ms` | int | API 耗时，毫秒 | 否（冗余，`duration_ms` 已够） |
| `ttft_ms` | int | 首 token 耗时，毫秒 | 否（调试用，不展示） |
| `num_turns` | int | 轮次 | 是 |
| `total_cost_usd` | float | 总成本（美元） | 是 |
| `result` | string | 最终输出文本 | 否（已从 assistant 事件流式采集） |
| `stop_reason` | string | 停止原因（`end_turn` 等） | 否（失败态用 `is_error`/`subtype`） |
| `terminal_reason` | string | 终止原因（`completed` 等） | 否 |
| `usage` | object | token 用量，见下 | 是 |
| `modelUsage` | object | 按模型维度拆分的用量与成本 | 否（单模型场景 `usage`+`total_cost_usd` 已够；多模型时再评估） |

`usage` 对象字段：

| 字段 | 类型 | 说明 | 是否采集 |
| --- | --- | --- | --- |
| `input_tokens` | int | 输入 token | 是 |
| `output_tokens` | int | 输出 token | 是 |
| `cache_creation_input_tokens` | int | 缓存创建写入 token | 是 |
| `cache_read_input_tokens` | int | 缓存读取 token | 是 |
| `server_tool_use` | object | 服务端工具调用（`web_search_requests`/`web_fetch_requests`） | 否（后续增强） |
| `cache_creation` | object | 细分缓存（`ephemeral_1h`/`ephemeral_5m`） | 否（`cache_creation_input_tokens` 已够） |
| `service_tier` / `speed` | string | 服务等级 / 速度 | 否 |

**与原预期的差异**（已按抓样修正）：
- 成本字段是 `total_cost_usd`，**不存在 `cost_usd`**（原预期 `cost_usd` 作废）
- 多出 `modelUsage` 按模型维度拆分（`costUSD` 在此），单模型场景用顶层 `total_cost_usd` 即可
- `usage` 多出 `server_tool_use`/`cache_creation`/`service_tier` 等细分，当前不采集
- 多出 `duration_api_ms`/`ttft_ms`/`stop_reason`/`terminal_reason` 过程指标，当前不采集

### 4.2 数据模型扩展

`model/ai_function.go` 新增 `AiTaskUsage` 与计量字段（源字段映射见 4.1 抓样）：

```go
// AiTaskUsage 单次执行的 token 用量（来自 result 事件 usage 字段）
type AiTaskUsage struct {
    InputTokens              int `json:"inputTokens"`              // usage.input_tokens
    OutputTokens             int `json:"outputTokens"`             // usage.output_tokens
    CacheCreationInputTokens int `json:"cacheCreationInputTokens"` // usage.cache_creation_input_tokens
    CacheReadInputTokens     int `json:"cacheReadInputTokens"`     // usage.cache_read_input_tokens
}

// AiTaskMetrics 任务计量摘要（来自 result 事件 + 进程退出状态）
type AiTaskMetrics struct {
    Usage      *AiTaskUsage `json:"usage,omitempty"`
    DurationMs int64        `json:"durationMs"`  // duration_ms
    CostUSD    float64      `json:"costUsd"`     // total_cost_usd（顶层，非 cost_usd）
    NumTurns   int          `json:"numTurns"`    // num_turns
}
```

`AiTaskRunResult` 增 `Metrics AiTaskMetrics`；`AiTaskState` 同步增 `Metrics`，供 `GetAiTask` 恢复展示。

### 4.3 解析器扩展

`parseStreamLine` 增返回计量数据：

```go
func parseStreamLine(line string) (text string, isResult bool, sessionID string, metrics *model.AiTaskMetrics)
```

`pumpOutput` 在 `isResult` 分支将 `metrics` 写入 `aiTaskRuntime.metrics`（持锁，与 sessionID 同区），最终并入 `AiTaskRunResult`。

> 兼容：旧版无计量的输出（result 事件缺字段）`metrics` 为 nil，前端判空跳过展示。

### 4.4 前端展示

任务 Tab 底部新增计量条，`onDone` 写入 `task.metrics`，模板渲染：

```
耗时 12s · 入 1.2k / 出 860 · 缓存读 4.3k · $0.03 · 1 轮
```

| 展示项 | 取值 | 缺省 |
| --- | --- | --- |
| 耗时 | `metrics.durationMs` 转 `Xs` 或 `Xm Ys` | 无 metrics 不显示 |
| token 入/出 | `usage.inputTokens` / `outputTokens`，k 进制 | 不显示 |
| 缓存读 | `cacheReadInputTokens`，仅非零显示 | 隐藏 |
| 成本 | `costUsd`，`$0.000` 三位小数 | 不显示 |
| 轮次 | `numTurns`，仅 >1 显示 | 隐藏 |

失败任务底栏额外展示（已有数据，仅渲染）：

```
✗ 失败 · exit 130 · 已取消 / 执行超时（上限 10 分钟）/ <错误信息>
```

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `model/ai_function.go` | 新增 | `AiTaskUsage`、`AiTaskMetrics`；`AiTaskRunResult`/`AiTaskState` 增 `Metrics` 字段 |
| `service/ai_function.go` | 修改 | `parseStreamLine` 增返回 metrics；`pumpOutput` 写入；`aiTaskRuntime` 增 metrics 字段；`GetAiTaskState` 返回 metrics |
| `service/ai_function_test.go` | 修改 | `parseStreamLine` result 事件解析测试；pumpOutput metrics 写入测试 |
| `frontend/src/components/AiFunctionPanel.vue` | 修改 | `onDone` 取 `metrics`；Tab 底栏计量条渲染；失败态 exitCode/error 展示 |
| `frontend/src/components/__tests__/AiFunctionPanel.spec.js` | 修改 | 计量条渲染、缺省隐藏、失败态展示测试 |

## 6. 验收标准

1. `result` 事件计量字段正确解析，Tab 底栏展示耗时/token/成本
2. 缓存读、轮次非默认值时才展示，避免信息冗余
3. 失败任务明确展示 exitCode 与错误分类（取消/超时/异常）
4. 任务进行中无 metrics，底栏不渲染占位
5. 重开应用后 `GetAiTask` 恢复任务仍展示计量

## 7. 决策结论

2026-09-08 抓样确认（claude v2.1.158），以下决策已定稿：

| 决策项 | 结论 | 依据 |
| --- | --- | --- |
| result 事件字段映射 | 见 4.1 节抓样表，归档于 `docs/plans/samples/result-event-sample.json` | 实际抓样，非文档推测 |
| 成本字段 | `total_cost_usd`（顶层），非 `cost_usd` | 抓样确认 `cost_usd` 不存在 |
| 采集范围 | 顶层 `duration_ms`/`num_turns`/`total_cost_usd`/`is_error`/`subtype` + `usage` 四个 token 字段；不采 `modelUsage`/`server_tool_use`/`cache_creation`/`ttft_ms`/`stop_reason` 等细分 | 单模型场景顶层字段已够；细分留后续增强 |
| 成本汇总 | 本设计仅展示单次，累计属 [[2026-09-08-ai-run-history-design]] 范畴 | 历史归档后做累计统计 |

## 8. 关联

- 来源：[[2026-09-08-ai-skill-menu-design]] 后续优化建议
- 配套：[[2026-09-08-ai-run-history-design]]（历史归档后可做累计统计）
