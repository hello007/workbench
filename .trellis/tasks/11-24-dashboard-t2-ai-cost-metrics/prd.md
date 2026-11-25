# T2 将 AI 成本与成功率上浮到概览主视野

- 父任务：`11-24-personal-dashboard`，执行顺序第 3 项（序号 2）
- 阶段 / 优先级：阶段 1 P0 纯聚合 / P0
- 前置：T1 面板骨架；与 T3 可并行
- 状态：planning（仅登记，未 start）

## 目标

把已有但不在主视野的 AI 使用数据变成可改变行为的指标：哪个 skill 值得继续用、成本是否失控。

## 复用点

`service/ai_task_history.go` 的 `Stats()` / `UsageCounts()`；`app_ai.go` 的 `GetAiTaskHistoryStats` / `GetFunctionUsageCounts`；`AiTaskHistoryPanel.vue` 已有 4 张数字卡 + 功能排行先例。指标：任务数、成功率、成本 USD、token 四分项、耗时、功能 Top5。可视化：数字卡 + 横向排行条 + 堆叠趋势，复用现有 ECharts 基建。

## 验收标准（可测试）

概览页可见成本与成功率；数值与 `AiTaskHistoryPanel.vue` 一致；统计逻辑未重复实现（复用现有 service）。

## 不做的事

不新增 AI 调用，不做模型效果评测，不展示伪精确置信度。
