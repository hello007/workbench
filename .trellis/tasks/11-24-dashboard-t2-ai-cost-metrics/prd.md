# T2 AI 成本口径统一与排行视图

- 父任务：`11-24-personal-dashboard`，执行顺序第 3 项（序号 2）
- 阶段 / 优先级：阶段 1 P0 纯聚合 / P0
- 前置：T1 面板骨架；与 T3 可并行
- 状态：planning（范围已于 2026-11-25 经 T1 头脑风暴收缩，未 start）

## 目标

在 T1 已交付「成本 / 成功率」卡片的基础上，补齐 AI 指标的口径一致性与可解释性：
哪个功能项值得继续用、成本是否失控，并能与 AI 历史面板逐项核对。

## 范围收缩说明（2026-11-25）

T1 已按 `docs/spec/dashboard-metrics.md` §2 完整实现 M4/M5 卡片（今日/本周成本 +
token 四项、本周成功率）及下钻。为避免验收重叠，T2 范围收缩为以下三项：

1. **`CanceledCount` 口径统一**：按 spec §5 在 `model.AiTaskHistoryStats` 增加
   `CanceledCount`，后端统一计算成功率，移除 T1 的 `List` 内存计数临时实现；
   契约变更须同步 `frontend/wailsjs/` 三处。
2. **功能排行视图**：在概览面板的 M4 下钻侧或 AI 历史抽屉内提供按功能项排行
   （成本 / 次数），复用 `Stats().ByFunction`。
3. **一致性核对**：概览卡片数值与 `AiTaskHistoryPanel.vue` 的同口径数值一致，
   差异须有明确解释（窗口定义、保留裁剪按 `FinishedAt` 等）。

## 复用点

`service/ai_task_history.go` 的 `Stats()` / `UsageCounts()`；`app_ai.go` 的
`GetAiTaskHistoryStats` / `GetFunctionUsageCounts`；`AiTaskHistoryPanel.vue` 已有的
数字卡 + 功能排行先例；ECharts 基建 `RepoStatsChart.vue`。

## 验收标准（可测试）

- [ ] `AiTaskHistoryStats.CanceledCount` 落地并经 `wailsjs` 三处同步。
- [ ] 概览 M5 成功率改为读取后端 `CanceledCount`，不再前端全量拉取计数。
- [ ] 功能排行可见且与 `Stats().ByFunction` 一致。
- [ ] 概览与 AI 历史面板同口径数值一致，或差异有登记的解释。
- [ ] 未新增 AI 调用；覆盖率门禁不降。

## 不做的事

不新增 AI 调用，不做模型效果评测，不展示伪精确置信度，不重建 T1 的卡片骨架。

## 已知边界

- AI 历史保留 90 天 / 2000 条，统计范围须显式标注。
- 保留裁剪按 `FinishedAt`、查询窗口按 `StartedAt`，跨天边界任务可能不计入，
  属已知微小差异，不要求消除。
