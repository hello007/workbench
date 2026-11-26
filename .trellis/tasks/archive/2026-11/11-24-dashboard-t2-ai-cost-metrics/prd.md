# T2 AI 成本口径统一与排行视图

- 父任务：`11-24-personal-dashboard`，执行顺序第 3 项（序号 2）
- 阶段 / 优先级：阶段 1 P0 纯聚合 / P0
- 前置：T1 面板骨架；与 T3 可并行
- 状态：planning（范围已于 2026-11-25 经 T1 头脑风暴收缩，2026-11-26 完成 T2 设计确认）

## 目标

在 T1 已交付「成本 / 成功率」卡片的基础上，消除 AI 指标的两套实现，补齐可解释性：
让概览与 AI 历史面板的数值由**同一段后端聚合代码**产出，用户能自己验算成功率，
并看清「哪个功能项最花钱、还用不用得下去」。

## 现状核对（2026-11-26 只读确认）

| 事实 | 位置 |
|---|---|
| `Stats().ByFunction.Count` 含 canceled，`UsageCounts()` 排除 canceled，仓内两套次数口径 | `service/ai_task_history.go` |
| T1 的 `buildAI` 用 `GetAiTaskHistory(nil)` 手写循环计数，未走 `Stats()` | `service/overview.go:139` |
| 历史抽屉已有功能排行表（次数 / 成本，按次数降序） | `AiTaskHistoryPanel.vue:80` |
| M5 下钻只带时间窗，不带状态；`Filter.Status` 仅支持单值 | `AiFunctionPanel.vue:258`、`model/ai_task_history.go` |
| 归档状态为闭集 `{success, failed, canceled, timeout}` | `service/ai_function.go:2099` |
| `AiTaskHistoryStats` / `FunctionStat` / `AiTaskHistoryFilter` 已入 wailsjs 契约 | `frontend/wailsjs/go/models.ts` |

## 已确认决策（ADR-lite）

- **D1 排行落点**：增强 `AiTaskHistoryPanel` 现有功能排行表（排序切换 次数/成本、
  成本占比、token、取消数），不在概览卡片内嵌 Top3、不新增第 6 模块。
  概览 M4 下钻已带「本周」窗口直接进入该表，链路不新建。
- **D2 次数口径**：`FunctionStat.Count` 保持含全部运行（不改既有数字与 Markdown 导出），
  新增 `FunctionStat.CanceledCount`，前端显示「次数（取消 n）」。
- **D3 聚合同源**：`OverviewService.buildAI` 改为调用 `GetAiTaskHistoryStats` 两次
  （今日 `From=今日 00:00`、本周 `From=周一 00:00`），删除手写状态计数循环；
  成功率分母 = `SuccessCount + FailedCount + TimeoutCount`（与卡片文案「success ÷
  （success+failed+timeout）」同式，不依赖「状态闭集」假设；四状态之和 = `TotalCount`
  仅作当前实现的验证点，不作分母推导前提）。
- **D4 契约补全**：`AiTaskHistoryStats` 新增 `CanceledCount` / `FailedCount` /
  `TimeoutCount`（四个状态计数齐全）；`AiTaskHistoryFilter` 新增 `Statuses []string`
  （非空优先，`Status` 保留向后兼容），M5 下钻预选 `failed + timeout`。

**取舍理由**：T2 的定位是口径统一与可解释，不是新增信息面。改 `Count` 语义会静默
改变历史面板与导出报告；新增计数与多状态筛选只增不改，向后兼容。

## 需求

### R1 统计契约补全（后端）

- `model.AiTaskHistoryStats` 新增：`CanceledCount`、`FailedCount`、`TimeoutCount`。
- `model.FunctionStat` 新增：`CanceledCount`（按功能项的取消数）。
- `model.AiTaskHistoryFilter` 新增：`Statuses []string`；`List` 过滤规则：
  `Statuses` 非空时按集合匹配，忽略 `Status`；为空时沿用单值 `Status`。
- `Stats()` 在单次遍历内同时累加四个状态计数与功能项取消数。

### R2 概览改走统一聚合（后端）

- `service/overview.go` 的 `buildAI` 改为两次 `GetAiTaskHistoryStats` 调用，
  删除 `accumulateAICost` 手写累加与状态 switch。
- `AIOverview.SuccessRate = SuccessCount / (SuccessCount + FailedCount + TimeoutCount)`，
  `HasData = SuccessCount + FailedCount + TimeoutCount > 0`；今日/本周成本与 token 直接取
  `Stats` 字段。
- 分区降级保持不变：任一 `Stats` 调用失败写入 `AIOverview.Error`，不伪装为 0。

### R3 功能排行增强（前端）

- `AiTaskHistoryPanel` 排行表新增：成本占比（占当前筛选总成本）、token、取消数；
  排序切换（次数 / 成本 / token），默认次数降序。
- 表头与列宽沿用 `el-table`，不做横向滚动；成本占比保留一位小数。
- 概览 M5 卡片提示补充「成功 n / 失败 n / 超时 n」（来自 `Stats` 四计数），
  下钻预选 `Statuses=['failed','timeout']`。

### R4 契约同步

- `frontend/wailsjs/go/models.ts` 手同步 `AiTaskHistoryStats`、`FunctionStat`、
  `AiTaskHistoryFilter`；若涉及 `App.d.ts` 签名无变化则不改。
- `frontend/wailsjs/go/main/App.js` / `App.d.ts` 仅在方法签名变化时调整（本次预期不变）。

## 验收标准（可测试）

- [ ] `AiTaskHistoryStats` 四个状态计数之和 = `TotalCount`（单测覆盖四状态 + `Metrics==nil`）。
- [ ] `FunctionStat.CanceledCount` 与同筛选下 `Statuses=['canceled']` 的 `Stats` 结果一致。
- [ ] `Filter.Statuses` 多值匹配生效，`Status` 单值仍兼容，两者同时存在时 `Statuses` 优先。
- [ ] 概览 M5 成功率与历史面板同时间窗 `Stats` 的成功率逐位一致（service 单测对比）。
- [ ] 概览 `HasData=false` 时前端仍显示「无数据」，不显示 0%。
- [ ] 排行表可按次数/成本/token 排序，成本占比之和 ≈ 100%（允许舍入误差）。
- [ ] M5 下钻进入历史抽屉后列表仅含 failed/timeout，且状态筛选可见。
- [ ] 未新增 AI 调用；后端 service 覆盖率 ≥76%、前端 ≥70% 不下降。

## Definition of Done

- `go vet ./...`、`go test ./...` 通过；前端 `npm test`、`npm run build` 通过。
- 新增/修改单测：`service/ai_task_history_test.go`、`service/overview_test.go`、
  `AiTaskHistoryPanel.spec.js`、`OverviewView.spec.js`。
- `docs/spec/dashboard-metrics.md` 回写：§5 实现状态、§7 中 T2 约束、D4 契约变化。
- 无 `go-git` 引入；`data/` 无新增直写；无新错误码。

## 技术方案

| 层 | 文件 | 改动 |
|---|---|---|
| model | `model/ai_task_history.go` | 三处 struct 加字段（`Statuses` 带 `omitempty`） |
| service | `service/ai_task_history.go` | `List` 多状态匹配；`Stats` 四状态计数 + 功能项取消数 |
| service | `service/overview.go` | `buildAI` 改两次 `GetAiTaskHistoryStats`；删除 `accumulateAICost` |
| app | `app_ai.go` | 无签名变化（预期不改） |
| 前端 | `AiTaskHistoryPanel.vue` | 排行列扩展 + 排序切换 + 初始 `Statuses` 消费 |
| 前端 | `AiSuccessCard.vue` | 成功/失败/超时三元提示 |
| 前端 | `OverviewView.vue` / `AiFunctionPanel.vue` | 下钻 payload 增 `statuses` 并透传 |
| 契约 | `frontend/wailsjs/go/models.ts` | 三处 struct 手同步 |

聚合口径保持不变：窗口按 `StartedAt`；保留裁剪按 `FinishedAt`；`Metrics==nil` 只计
任务数不计成本/token；排行 `Count` 含全部状态。

## 不做的事

- 不新增 AI 调用、不做模型效果评测、不展示伪精确置信度。
- 不重建 T1 卡片骨架、不新增第 6 个主看板模块、不在卡片内嵌排行。
- 不改 `FunctionStat.Count` 语义、不改 Markdown/CSV 导出格式。
- 不做跨 IPC 的统计缓存或增量聚合。

## 已知边界

- AI 历史保留 90 天 / 2000 条，统计范围须显式标注（卡片已有，排行表头需补）。
- 保留裁剪按 `FinishedAt`、窗口按 `StartedAt`，跨天边界任务可能不计入，不要求消除。
- `Statuses` 为前端传入的字符串集合，非法值不报错、自然匹配不到记录（与 `Status` 一致）。
- **行为变更（有意）**：历史抽屉每次打开统一复位筛选（日期范围、单值状态、多状态），
  不再保留上次手动设置的范围；与 `destroy-on-close` 的「每次打开重新加载」语义对齐，
  同时消除下钻残留。用户若需要固定范围，须在当次会话内重设。
- 历史面板「共 N 条」仍为当前筛选结果数，与统计卡片的 `TotalCount` 同筛选下应相等；
  若不等说明筛选未同步，属缺陷而非边界。

## 风险

| 风险 | 缓解 |
|---|---|
| `Statuses` 契约扩展遗漏 wailsjs 同步 → 前端读 `undefined` | 三处 models.ts 手改 + 前端防御 `|| []`；`npm run build` 校验 |
| 概览两次 `Stats` 读取放大 I/O | 单次 loadAll ≤2000 条 + 5 分钟 TTL；实测记录在 PR |
| 排行表列变多导致抽屉横向拥挤 | 成本占比与 token 列宽固定，`show-overflow-tooltip` |

## 实施计划（小步）

- PR1：后端契约 + `Stats`/`List` 改造 + 单测（不动前端）。
- PR2：`OverviewService.buildAI` 切换 + 概览单测一致性断言。
- PR3：前端排行增强、M5 下钻状态筛选、卡片三元提示 + 前端单测。
- PR4：`docs/spec/dashboard-metrics.md` 回写 + 覆盖率核对。
