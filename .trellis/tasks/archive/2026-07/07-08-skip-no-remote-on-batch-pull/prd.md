# 一键更新跳过无远程仓库并加灰色图标区分

## Goal

一键更新（批量拉取）时，自动跳过未配置远程仓库的本地测试仓库，避免报错干扰；同时在三处仓库展示位置（拉取结果表格、文件树节点、工作目录列表）加灰色 git 图标（`git-gray.png`），让用户一眼识别哪些 git 仓库未配置远程。

## What I already know（已探明事实）

### 一键更新完整链路

- **入口（三处）**：工作目录右键"更新仓库"（`DirectoryTree.vue:90`）、文件树节点右键"更新仓库"（`FileTreePanel.vue:247`）、ContentPanel 顶部按钮（`ContentPanel.vue:731`）
- **前端**：`Home.vue:295 onBatchPull` -> `ScanAndPullRepos(data.path)`
- **后端 `app.go:316 ScanAndPullRepos`**：`ScanGitRepos` 扫描所有 git 子仓库 -> goroutine 异步 `BatchPull(repos, 5, ctx)` -> 立即返回 `PullSummary{Total}`
- **`service/git.go:489 BatchPull`**：并发 5，每个仓库 `gitCmd.Pull`，失败标记 `Success=false / Error`
- **事件推送**：Wails `pull-progress`（单条结果）/ `pull-complete`（汇总 success/failed）
- **前端展示**：`ContentPanel.vue:343` `pullResults` 表格，状态列成功绿 `SuccessFilled` / 失败红 `CircleCloseFilled`

### 无远程报错根因

`BatchPull` 对无远程仓库执行 `git pull` 必然失败 -> 计入 `failCount` 红叉显示。即用户所见"报错"。

### 远程检测能力已具备

- **`util/git.go:92 GetRemote`**：执行 `git remote -v`，无远程返回 `"no remote configured"` 错误，与"没有配置远程仓库"完全吻合
- `service/git.go:419 HasUpstream`：检测上游跟踪（本次不用作跳过判定）

### 现有图标与模型

- `DirectoryTree.vue:34` 工作目录项 `dir.isGitRepo` 显示 `git.png`
- `FileTreePanel.vue:55` 文件树节点 `data.isGitRepo` 显示 `git.png`
- `service/filetree.go:28 isGitRepoDir`：os.Stat 快速检测 + sync.Map 缓存
- `service/directory.go:48 Create` / `:84 Update` / `app.go:98 RefreshDirectoriesGitFlag`：填充 `IsGitRepo`
- 图标目录已有 `git-gray.png`（未跟踪，待用）
- `model/models.go`：`PullResult{Path,Name,Success,Output,Error}`、`PullSummary{Total}`、`Directory{IsGitRepo}`、`FileTreeNode{IsGitRepo}`

### 性能约束

`app.go:83` 注释：启动关键路径不同步检测 `IsGitRepo`，由 `RefreshDirectoriesGitFlag` 异步刷新。远程检测须遵循同模式。

## Requirements

- 批量拉取（`BatchPull`）跳过无任何 remote 的仓库，不报错、不计入失败，单独标记 `Skipped`
- **检测口径**：仅"无任何 remote"（`git remote -v` 为空，`GetRemote` 返回错误）；有 remote 无 upstream 的不跳过（失败照常报错以提示修复）
- `PullResult` 新增 `Skipped` 字段，前端表格三态（成功绿对勾 / 跳过灰图标+"已跳过" / 失败红叉），汇总新增"已跳过"计数
- 单仓库 `PullRepo` 遇无远程返回友好提示（"该仓库未配置远程，无需拉取"），不报错
- 三处图标区分（`git.png` 有远程 / `git-gray.png` 无远程）：
  - 拉取结果表格（`ContentPanel.vue:343`）
  - 文件树节点（`FileTreePanel.vue:55`）
  - 工作目录列表（`DirectoryTree.vue:34`）

## Acceptance Criteria

- [ ] 一键更新含无远程仓库时，不再出现该类仓库的失败报错
- [ ] 无远程仓库被标记 `Skipped`，结果表格显示灰图标+"已跳过"
- [ ] 汇总显示"成功/跳过/失败"三类计数
- [ ] 单仓库拉取无远程时返回友好提示，不报错
- [ ] 文件树 git 仓库节点按有无远程显示 `git.png` / `git-gray.png`
- [ ] 工作目录列表 git 仓库项按有无远程显示 `git.png` / `git-gray.png`
- [ ] 既有批量拉取、单仓库拉取、文件树、工作目录列表无回归
- [ ] `go test ./...` / `npm test` 通过
- [ ] 不引入启动性能回归（远程检测遵循异步刷新/缓存模式）

## Definition of Done

- 测试新增/更新（`BatchPull` 跳过逻辑、`PullRepo` 友好提示单测）
- `go test ./...` / `npm test` 通过，构建绿
- 后端模型变更后 `wailsjs/go/models.ts` 重新生成
- 涉及行为变化则更新 `README.md` / `docs`（项目 CLAUDE.md 要求）
- 不引入启动性能回归

## Technical Approach

### 后端

- **模型扩展**（`model/models.go`）：
  - `PullResult` 加 `Skipped bool json:"skipped"`
  - `PullSummary` 加 `Skipped int json:"skipped"`（`pull-complete` 汇总）
  - `Directory` 加 `HasRemote bool json:"hasRemote"`
  - `FileTreeNode` 加 `HasRemote bool json:"hasRemote"`
- **`service/git.go`**：
  - 新增 `HasRemote(dirPath) bool`（封装 `GetRemote` 错误判断）
  - `BatchPull`：先 `HasRemote` 检测，无远程则 `result.Skipped=true` 跳过 pull；汇总 `skippedCount`；`pull-complete` 事件加 `skipped`
- **`app.go`**：
  - `PullRepo`：无远程返回友好提示
  - `RefreshDirectoriesGitFlag`：异步刷新时对 `IsGitRepo=true` 的目录检测 `HasRemote`
- **`service/directory.go`**：`Create`/`Update` 计算 `HasRemote`（仅 `IsGitRepo=true` 时）
- **`service/filetree.go`**：`GetChildren`/`buildTree` 对 `IsGitRepo=true` 节点检测 `HasRemote`（复用 `isGitRepoDir` 的 sync.Map 缓存模式）

### 前端

- **`ContentPanel.vue`**：表格状态列三态；`pullSummary` 加 `skipped`；汇总文案"成功/跳过/失败"
- **`FileTreePanel.vue`**：git 节点图标按 `data.hasRemote` 切换 `git.png`/`git-gray.png`，`title` 区分
- **`DirectoryTree.vue`**：工作目录 git 图标按 `dir.hasRemote` 切换，`title` 区分

### 性能

- 文件树远程检测复用 `isGitRepoDir` 的 sync.Map 缓存模式，仅对 git 节点检测
- 工作目录远程检测走 `RefreshDirectoriesGitFlag` 异步刷新，不阻塞启动

## Decision (ADR-lite)

- **Context**：无远程仓库在一键更新时报错干扰用户；用户希望跳过并视觉区分
- **Decision**：三处全做灰图标 + `BatchPull` 跳过无远程 + 新增 `Skipped` 状态 + 单仓库友好提示；检测口径仅"无任何 remote"
- **Consequences**：覆盖全面、语义清晰；代价是模型扩展（4 处加字段）+ 文件树远程检测需缓存控制；有 remote 无 upstream 的仓库仍报错（保留修复提示）

## Out of Scope

- "有 remote 但无 upstream" 的跳过与提示（保留报错以提示修复配置）
- "一键配置远程"入口
- Push 流程的无远程处理（可后续对齐）
- 文件树非 git 节点的远程标注

## Open Questions

（已全部确认）

1. **[已定]** 灰色图标采用组合方案（多处标注）。
2. **[已定]** 三处全做：拉取结果表格 + 文件树节点 + 工作目录列表，一次性交付（不分阶段）。
3. **[已定]** 新增 skipped 状态：`PullResult` 加 `Skipped` 字段，前端表格三态，汇总加"已跳过"计数。
4. **[已定]** 检测口径：仅"无任何 remote"（`git remote -v` 为空），用 `GetRemote` 错误判定。有 remote 无 upstream 的不跳过。
5. **[已定]** 单仓库 `PullRepo` 遇无远程返回友好提示，与批量行为一致。

## Technical Notes

- **后端关键文件**：`app.go:316 ScanAndPullRepos`、`service/git.go:489 BatchPull`、`service/git.go:207 ScanGitRepos`、`util/git.go:92 GetRemote`、`service/directory.go:48 Create`、`service/filetree.go:40 GetChildren`、`app.go:98 RefreshDirectoriesGitFlag`
- **前端关键文件**：`DirectoryTree.vue:34`、`FileTreePanel.vue:55`、`ContentPanel.vue:343`、`Home.vue:295 onBatchPull`
- **模型**：`model/models.go` `PullResult`/`PullSummary`/`Directory`/`FileTreeNode`
- **图标**：`frontend/src/assets/icons/git-gray.png`（已存在）、`git.png`（在用）
- **性能约束**：`app.go:83` 启动异步刷新模式、`service/filetree.go:28 isGitRepoDir` 缓存模式
