# AI 执行链路增强（第 1 批：可观测性 + 并发上限 + 输出截断）

## Goal

合并实施第 1 批三项 P0 优化，集中改造 `service/ai_function.go` 执行链路与 `AiFunctionPanel.vue` 前端，避免分批改动同一文件返工。三项共享 `aiTaskRuntime` 结构与 `pumpOutput` 流程，须协调字段新增与生命周期。

- **P0-2 可观测性**：提取 `result` 事件的 token/耗时/成本/exitCode，Tab 底栏展示计量
- **P0-3 并发上限**：全局信号量限并发（默认 3），超限排队 + 状态可见，超时起算后移
- **P0-4(3.1) 前端截断**：`onOutput` 末尾窗口截断（256KB），治前端渲染卡顿

## What I already know

- 3 份 design 文档已完备且决策定稿（见 Technical Notes 链接）
- result 事件字段已抓样确认（`total_cost_usd` 非 `cost_usd`），归档 `docs/plans/samples/result-event-sample.json`
- 代码探查已完成：`RunStage`（service/ai_function.go:89）/ `pumpOutput`（:426）/ `parseStreamLine`（:398，三元组）/ `CancelAiTask`（:142）/ `GetAiTaskState`（:157）/ `onOutput`（AiFunctionPanel.vue:409）/ `onDone`（:413）/ `<pre>` 渲染（:151）
- `AiFunctionService` struct（:25）仅 `mu` + `tasks`，无信号量、无清理入口
- 跨层契约 spec：改 `app.go` 绑定或 `model/` 导出字段须手动同步 `frontend/wailsjs/`（App.js / App.d.ts / models.ts 三处）

## Requirements

### P0-2 可观测性

- `parseStreamLine` 增返回 `*model.AiTaskMetrics`（四元组）
- `model/ai_function.go` 新增 `AiTaskUsage` / `AiTaskMetrics`；`AiTaskRunResult` / `AiTaskState` 增 `Metrics`
- `pumpOutput` 在 `isResult` 分支持锁写 `task.metrics`，并入 `AiTaskRunResult`
- `GetAiTaskState` 返回 metrics
- 前端 `onDone` 取 `metrics`，Tab 底栏渲染：耗时 / token 入出 / 缓存读（非零）/ 成本 / 轮次（>1）
- 失败任务底栏展示 exitCode + 错误分类（取消/超时/异常）

### P0-3 并发上限

- `AiFunctionService` 增 `concurrencySem chan struct{}`（缓冲 3）
- `RunStage` 拆排队/执行两段：先注册排队态 task（`queued=true`）入 map + emit `ai-task:queued`；`select` 获槽位后 `WithTimeout`（超时起算后移）；`pumpOutput` defer 释放信号量
- 排队取消两态：`CancelAiTask` 区分 `queued`（删 map + emit done）与运行中（杀进程树）
- `model/ai_function.go`：`AiTaskState` 增 `Queued`；新增 `AiConcurrencyStatus`
- `app.go` 新增 `GetAiConcurrencyStatus()` / `RemoveAiTask(taskID)` 绑定
- 前端：`task` 增 `queued`；`onQueued`/`onStarted` 事件；`statusText`/`statusTagType` 排队态；标题栏并发占用展示；Tab 关闭调 `RemoveAiTask`
- 运行中 Tab 禁止关闭（按钮置灰或提示）

### P0-4(3.1) 前端截断

- `onOutput` 超 256KB 只保留末尾窗口，顶部提示省略 KB 数
- `task` 增 `truncated` 标记
- `onDone` 存 `t.fullOutput = result.output`（3.1 阶段临时方案，3.3 落地后改 `GetAiTaskOutput`）
- `copyOutput` / `previewOutput` / `meetingTable` 改取 `t.fullOutput` 全量

## Acceptance Criteria

### P0-2

- [ ] result 事件计量字段正确解析（按抓样 `total_cost_usd` 等）
- [ ] Tab 底栏展示耗时/token/成本，缓存读与轮次非默认值才展示
- [ ] 失败任务展示 exitCode + 错误分类
- [ ] 重开应用后 `GetAiTask` 恢复任务仍展示计量

### P0-3

- [ ] 并发运行数不超过 3，超限进入排队态，Tab 显示「排队中」
- [ ] 排队任务前序完成后自动启动，转「运行中」
- [ ] 排队任务可取消，让出排队位
- [ ] 超时从获取槽位后起算，排队等待不计入
- [ ] 进程退出释放槽位，标题栏「N/M」实时更新
- [ ] Tab 关闭调 `RemoveAiTask`，map 不无限增长

### P0-4(3.1)

- [ ] 输出超 256KB 只保留末尾窗口，顶部提示省略量
- [ ] 截断后 `<pre>` 渲染流畅，流式无持续掉帧
- [ ] copy/preview/meetingTable 取 `t.fullOutput` 全量不受截断影响

## Definition of Done

- 后端测试：`go test ./...` 绿（parseStreamLine 四元组、metrics 写入、并发上限排队、排队取消、槽位释放）
- 前端测试：`cd frontend && npm test` 绿（计量条渲染、排队态、截断触发、完成动作取全量）
- `frontend/wailsjs/` 同步 `GetAiConcurrencyStatus` / `RemoveAiTask` 绑定（App.js / App.d.ts / models.ts）
- 跨层契约 `docs/spec/cross-layer-contracts.md` 的同步规则已遵循
- 行为变更后 README.md / 功能说明.md 评估是否更新

## Technical Approach

三项共享 `aiTaskRuntime`，字段新增合并为一次性扩展：

```go
type aiTaskRuntime struct {
    // 现有
    id, functionID, prompt, sessionID string
    cmd *exec.Cmd
    ctx context.Context
    cancel context.CancelFunc
    running, canceled bool
    startedAt time.Time
    timeoutMin int
    output strings.Builder
    errText string
    // P0-2 新增
    metrics *model.AiTaskMetrics
    // P0-3 新增
    queued bool
    queuedAt time.Time
}
```

**实施顺序**（同任务内小步）：

1. model 层字段扩展（`AiTaskUsage`/`AiTaskMetrics`/`AiTaskState.Queued`/`AiConcurrencyStatus`）+ wailsjs 同步
2. `parseStreamLine` 四元组改造 + metrics 写入（P0-2 后端）
3. 信号量 + 排队/执行两段 + 槽位释放（P0-3 后端）
4. `CancelAiTask` 两态 + `GetAiConcurrencyStatus` / `RemoveAiTask` 绑定（P0-3 后端收尾）
5. 前端 `onOutput` 截断 + `t.fullOutput`（P0-4 3.1）
6. 前端计量条 + 排队态 + 并发展示 + Tab 关闭清理（P0-2/P0-3 前端合并）
7. 测试补全

**关键冲突点**：`RunStage` 同时被 P0-2（metrics 写入依赖 result 事件，在 pumpOutput）与 P0-3（排队/执行拆分，超时 ctx 起算后移）改动。P0-3 改 RunStage 的 `ctx` 创建位置（获槽位后），P0-2 改 pumpOutput 的 result 分支——两处不重叠，但 `task` 对象构造须一次包含 `queued` 与 `metrics` 字段。

## Decision (ADR-lite)

**Context**：3 项 P0 优化同处执行链路，分批做会二次改动 `service/ai_function.go` 与 `AiFunctionPanel.vue`。

**Decision**：合并为单任务，按 model → 后端解析 → 后端并发 → 后端收尾 → 前端截断 → 前端展示 → 测试 顺序小步推进。所有决策点已按 design 第 7 节定稿（并发 3 / 策略 C / 超时后移 / map 清理同批 / 运行中 Tab 禁关 / 256KB 截断 / 表格后端预解析留 3.3）。

**Consequences**：单任务改动面大（7 文件），但避免返工；review 时按小步提交可分阶段验证。`RemoveAiTask` 与 P0-4(3.3) 共用，3.3 落地时不再改此入口。

## Out of Scope

- P0-4(3.2) 事件时间窗口合并——视 3.1 落地后实测决定
- P0-4(3.3) 流式写文件——随 P1-2 历史归档第 3 批
- 表格视图后端预解析 `TableExtracted`——随 3.3
- 并发上限入配置文件——随 P2-3 schema v2
- 失败任务重试入口（缺口 5）——独立小项

## Technical Notes

### Design 文档（决策定稿）

- [`docs/plans/2026-09-08-ai-task-observability-design.md`](../../../docs/plans/2026-09-08-ai-task-observability-design.md) — P0-2，第 4.1 节抓样字段表，第 7 节决策
- [`docs/plans/2026-09-08-ai-concurrency-control-design.md`](../../../docs/plans/2026-09-08-ai-concurrency-control-design.md) — P0-3，第 4 节设计，第 7 节决策
- [`docs/plans/2026-09-08-ai-output-memory-design.md`](../../../docs/plans/2026-09-08-ai-output-memory-design.md) — P0-4，本批仅做 3.1（第 4.1 节），3.2/3.3 出 scope
- [`docs/plans/2026-09-08-ai-optimization-overview.md`](../../../docs/plans/2026-09-08-ai-optimization-overview.md) — 总览，第 5 节实施顺序

### 抓样归档

- [`docs/plans/samples/result-event-sample.json`](../../../docs/plans/samples/result-event-sample.json) — result 事件完整字段，P0-2 解析依据

### 跨层契约 spec

- [`docs/spec/cross-layer-contracts.md`](../../../docs/spec/cross-layer-contracts.md) — 改 app.go 绑定 / model 导出字段须同步 wailsjs 三处

### 探查依据（代码位置）

- `service/ai_function.go`：`RunStage`(:89) / `parseStreamLine`(:398) / `pumpOutput`(:426) / `CancelAiTask`(:142) / `GetAiTaskState`(:157) / `AiFunctionService` struct(:25) / `aiTaskRuntime`(:33)
- `model/ai_function.go`：`AiTaskRunResult`(:70) / `AiTaskState`(:80) / `AiMcpServer`(:28)
- `app.go`：`RunAiFunction`(:1253) / `RunAiFollowUp`(:1277) / `CancelAiTask`(:1317) / `GetAiTaskState`(:1322)
- `frontend/src/components/AiFunctionPanel.vue`：`onOutput`(:409) / `onDone`(:413) / `doRunMain`(:230) / `<pre>` 渲染(:151) / `meetingTable`(:280) / 事件注册(:515)
