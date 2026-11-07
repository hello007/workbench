# 多仓库管理与代码追溯体验增强（父任务）

## Goal

容器任务：串接三个子任务，按序交付「多仓库巡检 → 工作现场暂存 → 单文件追溯」三条体验链路。来源：2026-11-07 功能增强调研（302 个 App 方法功能面盘点 + 同类工具对照）。

## 子任务与执行顺序

| 顺序 | 子任务 | 能力 | 调研评估 |
|---|---|---|---|
| 1 | 11-07-dashboard-worktree-status | 看板补全工作树状态列（dirty 文件数/最后提交时间/stash 数） | S，成本最低，补齐多仓库巡检高频动作 |
| 2 | 11-07-stash-management | Stash 管理（list/push/pop/apply/drop + 描述命名） | S-M，Git 功能补全，UI 可复制 GitBranches 模式 |
| 3 | 11-07-file-level-history | 文件级提交历史（git log --follow + CommitHistory 参数化复用） | S-M，与既有 CommitHistory 形成追溯闭环 |

## Requirements（容器级约束）

- 各子任务独立走完整 trellis 流程（brainstorm → jsonl → start → implement → check → finish-work），父任务不承载实现
- 子任务间无硬依赖，但建议按上表顺序实施（1 的看板列含 stash 数，与 2 的数据链路有弱耦合，先做 1 时 stash 数列可留空/后续接入）
- 共同约束：遵循 AGENTS.md 关键规则（EventSink 事件出口、cross-layer-contracts 签名同步、测试覆盖率门禁）

## Out of Scope

- 调研清单中的其他候选（外部变更自动刷新、blame、终端分屏等）未立项，另行评估
