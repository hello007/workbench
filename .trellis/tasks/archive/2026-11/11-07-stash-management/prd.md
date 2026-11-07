# Stash 管理

## Goal

当前无 stash 入口（已核对 2026-11-07：302 个 App 方法中无 Stash 相关），暂存/恢复工作现场须切终端；多仓库并行开发时 stash 易遗忘。提供 stash 列表与管理能力：list/push/pop/apply/drop，支持描述命名。

## Requirements

- 入口：ContentPanel Git 签页新增「Stash」tab（`name="stash"`，lazy），新建 `GitStash.vue` 复制 GitBranches 列表+操作模式（2026-11-07 用户确认）
- 列表展示：描述（含自定义命名）/ 基点分支 / 时间；空列表显示空态
- stash push：可输入自定义描述（`-m`），默认暂存全部 tracked 变更，界面提供「含未跟踪文件」勾选追加 `-u`（2026-11-07 用户确认）
- pop/apply/drop：破坏性操作（pop/drop）走 ElMessageBox.confirm；pop 冲突时 stash 保留（git 默认语义如实转译）
- 冲突呈现：操作返回冲突错误时展示 git 输出（超 200 字符弹 Dialog，对齐 LocalChanges 推送范式）+ 提示「冲突未解决，stash 条目已保留」+ 列出冲突文件；冲突处理依赖既有 GitMerge tab 链路（红点角标自动感知），不重复造轮子（2026-11-07 用户确认）
- 取消：Push/Pop/Apply/Drop 走 `ExecuteWithContext` ctx 变体 + `registerCancel` 链路，取消返回 `E_GIT_CANCELLED`
- 后端契约变更同步 wailsjs 三处（App.js / App.d.ts / models.ts）

## Acceptance Criteria

- [ ] stash 列表展示（描述/分支/时间）+ push（描述命名 + untracked 勾选）/pop/apply/drop 操作可用
- [ ] 中文描述往返无乱码（单测用中文数据）
- [ ] pop 冲突：stash 条目保留 + 冲突文件清单反馈明确
- [ ] pop/drop 有 confirm 确认；空列表/取消等边角有明确反馈
- [ ] 后端单测 + 前端组件测试全绿，覆盖率门禁通过

## Definition of Done

- service 层单测：列表解析（中文/多条目）、push/pop/apply/drop 语义、取消链路
- 前端组件测试：GitStash.vue 渲染 + 操作 emit/confirm 流
- `go vet ./...` / `go test ./...` / `npm run build` / `npm test` 全绿
- 完成后确认是否更新 README.md；若有踩坑沉淀按两级分流写入 docs/spec/

## Technical Approach

分层链路：App 方法（`app_git.go`）→ `service/git_stash.go`（对齐 git_merge.go/git_difftool.go 领域拆分模式）→ wailsjs 三处手动同步 → `GitStash.vue` + ContentPanel tab 挂载。

- `app_services.go` 两处装配（AppServices 字段 + 构造行）
- 列表解析：`git stash list` 须自带 UTF-8 `-c` 前缀 + `%x00`/`%x01` 分隔符（branch-compare.md 契约）；reflog 语法 `%gd`（stash@{n}）/ `%gs`（subject 内含分支信息）/ 时间字段
- model 契约：`model.StashEntry`（Index/Description/Branch/Timestamp 等），具名类型若作绑定返回值须手动补 models.ts 别名
- 取消：ctx 变体 + `registerCancel`/`unregisterCancel` LIFO + `mapCancelError`

## Decision (ADR-lite)

**Context**: stash 是 Git 域新功能，入口位置、push 语义、冲突呈现存在多种做法。
**Decision**: Git 签页第 9 个 tab（GitStash.vue 复制 GitBranches 模式）；push = 全部 tracked + 「含未跟踪文件」勾选，pathspec 不做；冲突 = git 输出展示 + stash 保留提示，冲突解决依赖既有 GitMerge 链路。
**Consequences**: tab 数增至 9（lazy 加载不影响首屏）；部分文件暂存后续按需立项；看板 stash 数已由子任务 1 交付且 dashboard 即查即算，stash 操作后下次看板刷新自然更新，无缓存联动需求。

## Out of Scope

- 部分文件 pathspec 暂存（`git stash push -- <path>`）
- stash clear（清空全部，危险操作，逐条 drop 可替代）
- stash 内容 diff 查看（`git stash show`）
- 看板 stash 数列接入（子任务 1 已交付，无需额外工作）

## What I already know（代码探查，2026-11-07）

- **入口结构**：Git 功能集中在 `ContentPanel.vue` 的 `el-tabs`（现有 8 个 tab：仓库信息/提交历史/本地变动/分支/标签/远程仓库/合并变基/子模块），每个领域一个 tab，组件模式 `GitXxx.vue` + el-card + 列表/操作行（GitBranches 为最佳模板）
- **看板 stash 数列已接通**：子任务 1 已交付——`DashboardView.vue` Stash 列（`row.stashCount`）+ `service/dashboard.go` `git stash list` 非空行计数（只读、失败降级置 0）
- **后端落点**：service 层新文件 `service/git_stash.go`；App 方法加 `app_git.go`；`app_services.go` 两处装配
- **列表解析**：stash 描述可含中文，须 UTF-8 `-c` 前缀 + `%x00`/`%x01` 分隔符（branch-compare.md 契约）
- **取消链路**：`ExecuteWithContext` ctx 变体；Abort 无 ctx 先例
- **确认对话框规范**：pop/drop 属破坏性操作，须走 ElMessageBox.confirm（confirm-dialog.md 形态规范）

## Technical Notes

- 参照既有 Git 功能组件的分层：App 方法 → service → wailsjs 同步 → Vue 组件
- 弱耦合：DashboardView 看板 stash 数列已由 11-07-dashboard-worktree-status 交付，本任务零接入工作
- 相关规范：cross-layer-contracts（wailsjs 同步/具名类型别名）、branch-compare.md（UTF-8 编码契约）、logging-and-errors.md（取消链路/错误码）、confirm-dialog.md（确认框形态）、frontend-visual-conventions（Git 卡片样式）
