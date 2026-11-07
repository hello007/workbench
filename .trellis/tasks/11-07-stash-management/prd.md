# Stash 管理

## Goal

当前无 stash 入口（已核对 2026-11-07：302 个 App 方法中无 Stash 相关），暂存/恢复工作现场须切终端；多仓库并行开发时 stash 易遗忘。提供 stash 列表与管理能力：list/push/pop/apply/drop，支持描述命名。

## What I already know

- UI 组件模式可复制 GitBranches.vue（列表 + 操作菜单 + 刷新链路）
- git 操作走 util 层 `GitCommand` 链路；取消语义遵循 `ExecuteWithContext` ctx 变体与 `E_GIT_CANCELLED`（Abort 刻意无 ctx 变体为先例）
- git 调用编码：涉及 commit 信息（stash 描述可含中文）的调用须自带 UTF-8 `-c` 前缀与 `%x00`/`%x01` 分隔符规范（branch-compare.md）
- 后端新增 service/App 方法遵循 app-services-assembly（AppServices 字段 + 构造行）与跨层契约（wailsjs 三处手动同步）
- 弱耦合：DashboardView 看板计划补 stash 数列（11-07-dashboard-worktree-status），本任务提供数据口径后接入

## Open Questions（brainstorm 待讨论）

- 入口位置：GitInfo 签页内新 tab？LocalChanges 顶部按钮？独立签页
- stash push 语义：默认全部 tracked 变更还是支持 untracked（-u）/部分文件（-p 交互式不可行，可选路径级 stash push -- <path>）
- 冲突场景：pop/apply 冲突时的 UI 反馈与 stash 保留语义（pop 冲突不 drop 的 git 默认行为如何呈现）
- 看板 stash 数列接入是否纳入本任务范围

## Acceptance Criteria (evolving)

- [ ] stash 列表展示（描述/分支/时间）+ pop/apply/drop 操作
- [ ] 中文描述往返无乱码（单测用中文数据）
- [ ] 空列表/冲突/取消等边角有明确反馈
- [ ] 后端单测 + 前端组件测试全绿

## Technical Notes

- 参照既有 Git 功能组件的分层：App 方法 → service → wailsjs 同步 → Vue 组件
