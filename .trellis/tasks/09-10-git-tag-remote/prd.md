# Git 标签与远程仓库管理

## Goal

补全 Git 高级操作闭环：在现有 Git 集成面板基础上新增**标签管理**（列出 / 创建轻量+注释标签 / 删除 / 推送远程）与**远程仓库管理**（增删 remote / 查看 remote 信息 / fetch / 设跟踪分支）。复用现有 `GitInfo.vue` + `CommitHistory.vue` 交互范式，前后端契约同步 `wailsjs` 绑定。

## What I already know

- **后端 GitService**（`service/git.go`）已实现：`GetInfo/GetLog/Clone/Pull/HasRemote/GetBranches/CheckoutBranch/Commit/Push/HasUpstream/GetDiff/DiscardChanges/GetLocalChanges/BatchPull`。
- **exec 范式**：`s.gitCmd.Execute(gitRoot, args...)` → `(string, error)`，`util.FindGitRoot` 定位根，`strings.TrimSpace/Split` 解析输出，错误 `fmt.Errorf("…: %w", err)`。
- **桥接范式**（`app_git.go`）：`a.gitSvc.Xxx` 委派；`GetGitRemoteURL`/`GetCommitHistory` 直接用 `git.PlainOpen` + go-git API。两套并存。返回 `(*model.X, error)` / `(string, error)` / `error`，`path==""` 前置校验。
- **现有 model**：`GitRemoteInfo{remoteUrl, branch, isDetached}`、`BranchInfo{name, isRemote, isCurrent}`、`Commit{sha, shortSha, message, …}`。
- **前端范式**：`GitInfo.vue` = el-card + el-descriptions + Refresh circle + loading + ElMessage + `gitCache`。import `from '../../wailsjs/go/main/App'`。props `repoPath`。
- **wailsjs 现状**：`App.d.ts`/`App.js`/`models.ts` 三处手工同步。Git 绑定 16 个（CheckoutBranch…ScanAndPullRepos）。
- **跨层契约**：改 `app_git.go` App 方法签名或 `model/` 导出 struct 字段 → 须手动同步 `frontend/wailsjs/` 的 `App.js` / `App.d.ts` / `models.ts` 三处（见 `docs/spec/cross-layer-contracts.md`）。

## Decision (ADR-lite)

**Context**: 4 个 Preference 影响 MVP 边界与前端文件结构。

**Decision**（用户确认按推荐）:

- Q1 UI 形态 = A：独立双面板 `GitTags.vue` + `GitRemotes.vue`，复用 GitInfo card 范式，Home 按需挂载。
- Q2 创建粒度 = A：仅 HEAD，不支持指定起始 commit。
- Q3 推送粒度 = A：单个 `git push origin <tag>` + 面板级「推送全部标签」`git push --tags`。
- Q4 设跟踪 = A：对当前分支，用户选目标 remote，`git branch --set-upstream-to=<remote>/<branch>`。

**Consequences**: 前端两独立组件，状态隔离清晰；未来扩展指定 commit / 任意分支设上游需加输入项，model 无需改。

## Requirements

### 标签管理
- 列出仓库所有 tag（名称 / 类型 lightweight|annotated / sha / message / date）
- 创建标签：轻量（无 message）+ 注释（有 message，`-a -m`），仅钉 HEAD
- 删除标签（本地 `git tag -d`）
- 推送标签到远程：单个 `git push origin <tag>` + 面板级「推送全部标签」`git push --tags`

### 远程仓库管理
- 列出 remote（名称 + URL，取首个 URL）
- 新增 remote（`git remote add`）
- 删除 remote（`git remote remove`）
- fetch（`git fetch [<remote>]`，可选 `--prune`）
- 设跟踪分支（当前分支 + 选 remote，`git branch --set-upstream-to=<remote>/<branch>`）

### 跨层契约
- 新增 9 个后端桥接方法 → 同步 `wailsjs` `App.js`/`App.d.ts`/`models.ts`

## Acceptance Criteria

- [ ] 列出标签，区分轻量/注释，展示 sha/message/date
- [ ] 创建轻量 + 注释标签均可成功，重名报错
- [ ] 删除标签后列表刷新
- [ ] 推送单个 tag 成功，远程已删 tag 给明确错误
- [ ] 「推送全部标签」成功，stdout 展示
- [ ] 列出 remote 名称+URL
- [ ] 新增/删除 remote 后列表刷新，重名/不存在给明确错误
- [ ] fetch 成功返回 stdout，网络失败给明确错误
- [ ] 设跟踪分支后 `HasUpstream` 返回 true
- [ ] wailsjs 三处绑定同步，前端调用无类型错误
- [ ] 后端单测覆盖 ListTags/CreateTag/DeleteTag/PushTag/ListRemotes/AddRemote/RemoveRemote/Fetch/SetBranchUpstream
- [ ] 前端测试覆盖面板渲染 + 操作交互

## Definition of Done

- 后端 `go test ./...` 绿，model/service 覆盖率达基线
- 前端 `npm test` 绿，覆盖率 ≥70%
- wailsjs 三处绑定同步
- `README.md` / `docs/功能说明.md` 确认是否更新

## Out of Scope

- 标签签名（GPG / `-s`）
- 推送标签触发 CI/release（release 走 `release` skill）
- remote 重命名（`git remote rename`）
- fetch 多 remote 批量
- 远程 tag 批量删除
- 指定 commit 创建 tag（MVP 仅 HEAD，见 Decision Q2）
- 对非当前分支设上游（见 Decision Q4）

## Technical Notes

- **后端文件**：`service/git.go`（GitService 新 9 方法）、`model/`（新 `GitTag`、`GitRemote` struct）、`app_git.go`（新 9 桥接）。
- **exec 复用**：tag/remote/fetch/upstream 均走 `s.gitCmd.Execute` + `util.FindGitRoot`，与 `Push`/`HasUpstream` 同范式。
- **前端文件**：新建 `frontend/src/components/GitTags.vue` + `GitRemotes.vue`，复用 `GitInfo.vue` card 范式，Home 按需挂载。
- **新方法清单**（GitService → app_git.go 桥接同名首字母大写）：
  - `ListTags/CreateTag/DeleteTag/PushTag`（标签）
  - `ListRemotes/AddRemote/RemoveRemote/Fetch/SetBranchUpstream`（远程）
- **跨层契约**：`docs/spec/cross-layer-contracts.md`。
- **测试基线**：model/service ≥ 基线，前端 ≥70%（见 `docs/spec/test-coverage-gate.md`）。
