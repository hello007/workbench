# 体验优化与浏览器能力批次（父任务）

> 编排任务：统辖三个子任务，**执行顺序 = 子任务 1 → 2 → 3**（先轻后重：素材/预览遗留收口 → 浏览器连接健壮性 → Agent 交互式多轮会话）。
> 状态：仅建任务占位，**等用户指令再逐个实施**（实施时对子任务走完整 brainstorm → jsonl → start 流程）。

## Goal

v1.10.x 发版后的功能批次：先把 journal 遗留三件清掉（低风险热身），再补浏览器通道健壮性，最后落地浏览器二期最高优先级的 Agent 交互式多轮会话。

## 子任务（执行顺序）

| 顺序 | 任务 | 量级 | 说明 |
|---|---|---|---|
| 1 | [10-22-legacy-trio-cleanup](../10-22-legacy-trio-cleanup/prd.md) | 小 | 素材聚合展开截断/PPT 预览队列优先级/clipboard vet 清理 |
| 2 | [10-22-ws-reconnect-resume](../10-22-ws-reconnect-resume/prd.md) | 中 | WS 断线重连事件序号增量续传 |
| 3 | [10-22-agent-interactive-session](../10-22-agent-interactive-session/prd.md) | 大 | Agent 运行中插话/权限审批/多会话并行 |

## 决策记录

* OCR（SRS §2.2.4/§2.4）用户 2026-10-22 决策**暂缓**：未考虑好是否加入，本批次不含；重启前需用户明确表态。

## Out of Scope

* 单页 OCR 及其相关 schema/队列改动
