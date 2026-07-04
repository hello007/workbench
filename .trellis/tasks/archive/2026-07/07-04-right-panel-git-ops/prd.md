# 右边栏操作面板支持常用 Git 操作

## Goal

在 WorkBench 右侧操作面板（`ContentPanel.vue`，选中 Git 仓库节点时展示）补全日常 Git 工作流的核心闭环——**差异比较（diff）、选择性提交（commit）、推送（push）**，采用 **IDEA（IntelliJ IDEA）commit 窗口风格**：单区变动文件清单 + 复选框勾选要提交的文件 + 直接 commit / push / 提交并推送。让用户在不离开应用、不打开终端的情况下完成"查看改动 → 勾选文件 → 写提交信息 → 提交 → 推送"的完整流程。

## What I already know（已探明的事实）

### 布局现状（`Home.vue`）

```
[ActivityBar 44px] | [Pane1 20%: DirectoryTree / ToolboxPanel] | [Pane2 30%: FileTreePanel] | [Pane3 50%: ContentPanel]
```

- **"右边栏（操作面板）" = `ContentPanel.vue`**（最右侧 50% 那一栏）。
- 当 `selectedNode.isGitRepo === true` 时，ContentPanel 显示：节点路径信息、Git 操作按钮区（现有【拉取更新】【切换分支】）、`el-tabs` 三页签（仓库信息 GitInfo / 提交历史 CommitHistory / 本地变动 LocalChanges）。

### 后端 Git 能力清单

| 能力 | 后端方法 | 状态 |
|---|---|---|
| 仓库信息 | `GetGitInfo` / `GetGitRemoteURL` | ✅ |
| 提交历史 | `GetCommitHistory` | ✅ |
| 本地变动列表 | `GetLocalChanges`（含 `Staged` 字段） | ✅ |
| 回滚变动 | `DiscardChanges` | ✅ |
| 拉取 | `PullRepo` / `BatchPull` | ✅ |
| 分支切换 | `GetBranches` / `CheckoutBranch` | ✅ |
| 克隆 | `CloneRepo` | ✅ |
| **差异比较 diff** | — | ❌ 缺失 |
| **选择性提交 commit**（带文件列表） | — | ❌ 缺失 |
| **推送 push** | — | ❌ 缺失 |

### 前端 `LocalChanges.vue` 现状

- 单表格展示变动文件（状态标签 + 路径），已支持多选（`type="selection"`）；
- 底部只有【回滚选中】【全部回滚】；
- **无 diff / 提交 / 推送入口**。多选机制可直接复用为"勾选要提交的文件"。

### 关键数据模型（`model/commit.go`）

```go
type FileChange struct {
    Path   string `json:"path"`
    Status string `json:"status"` // M/A/D/R/?
    Staged bool   `json:"staged"`
}
```

## Requirements

### 功能需求

1. **IDEA 风格变动清单**：在"本地变动"tab 展示**单区**变动文件列表，每行前置**复选框**（用于勾选本次要提交的文件）+ 状态标签（M/A/D/R/?）+ 文件路径；表头提供"全选/全不选"。**不区分暂存区/工作区**，沿用 IDEA 风格。
2. **双击 diff**：**双击**文件行 → 弹窗**双栏左右对照**展示差异（左=旧版本、右=新版本）。
   - 已跟踪文件：对比 **HEAD vs 工作区**（`git diff HEAD -- <file>`，一次性显示所有未提交改动）。
   - 未跟踪文件（`?`）：弹窗内展示为新增全文。
   - 二进制、图片、超大文件：弹窗给出友好提示而非展示内容。
3. **选择性提交 commit**：底部提供 commit message 输入框 + 【提交】按钮。点击【提交】时，**仅提交勾选的文件**（后端 `git add -- <selected>` + `git commit -m <msg> -- <selected>`，pathspec 语义，不影响未勾选文件）；未勾选任何文件时【提交】禁用；commit message 为空时禁用并提示。
4. **推送 push**：独立【推送】按钮 + 【提交并推送】快捷按钮；当前分支无上游时弹确认是否 `git push --set-upstream origin <branch>`。
5. **回滚（保留现有能力）**：保留对勾选文件的【回滚】入口（作为次要操作，如下拉菜单或行右键）。
6. **联动刷新**：提交 / 推送后，自动刷新"本地变动"列表与"提交历史"tab 与 GitInfo（复用现有 `commitHistoryRef.handleRefresh` / `gitInfoRef.handleRefresh` 模式）。
7. **反馈**：所有操作有 loading / 成功 / 失败 toast（`ElMessage`），失败信息透传 git stderr。

### 非功能需求

- 后端复用 `util.GitCommand.Execute`（30s 超时）；diff 大文件注意体积与超时。
- 推送凭证**透传系统 git 配置**（https credential helper / SSH agent），应用内不托管凭证。

## Acceptance Criteria

- [ ] 选中 Git 仓库节点 → "本地变动"tab 显示单区变动清单，每行带复选框，表头可全选。
- [ ] 双击变动文件 → 弹窗以双栏左右对照展示 diff（已跟踪文件 vs HEAD，未跟踪文件展示为新增全文）。
- [ ] 二进制 / 图片 / 超大文件双击后弹窗给出友好提示，不展示乱码。
- [ ] 勾选若干文件 + 输入 message → 【提交】可用；提交成功后**仅这些文件**进入新提交，未勾选文件保持未提交；列表与"提交历史"tab 自动刷新。
- [ ] 未勾选文件或 message 为空时【提交】禁用并有视觉提示。
- [ ] 【推送】与【提交并推送】可用；无上游分支时提示是否 set-upstream。
- [ ] push 失败（网络 / 需先 pull / 凭证）时，错误信息清晰可读。
- [ ] 【回滚】对勾选文件仍可用，且有二次确认。
- [ ] `go test ./...` 与 `cd frontend && npm test` 全绿；新增后端方法有单测。

## Definition of Done（团队质量基线）

- 后端新增方法有单元测试（参考 `service/git_test.go`）。
- 前端组件变更通过 `vitest`（参考 `__tests__/ContentPanel.spec.js`）。
- `go test ./...` 与 `npm test` 全绿。
- 行为变更后确认是否需要更新 `README.md` 与 `docs/功能说明.md`。
- 危险操作（回滚）保留二次确认；提交 / 推送为常规操作，无需二次确认（除 set-upstream）。

## Technical Approach

### 后端（`util/git.go` → `service/git.go` → `app.go`）

新增方法（统一通过 `util.GitCommand.Execute(gitRoot, args...)` 执行，复用 `util.FindGitRoot`）：

| 方法 | 实现 | 说明 |
|---|---|---|
| `Commit(repoPath, message string, files []string)` | `git add -- <files>` → `git commit -m <msg> -- <files>` | pathspec 提交，仅提交勾选文件；`files` 为空返回错误；未跟踪文件先 add 再 commit |
| `CommitAll(repoPath, message string)` | `git add -A` → `git commit -m <msg>` | 可选：提交全部（表头全选时复用 `Commit` 即可，未必需要单独方法） |
| `Push(repoPath string, setUpstream bool) (string, error)` | `git push [--set-upstream origin <branch>]` | 返回 stdout 用于结果展示 |
| `GetDiff(repoPath, file string) (string, error)` | 已跟踪：`git diff HEAD -- <file>`；未跟踪：`git diff --no-index /dev/null <file>`（或直接构造全文为新增） | 返回 unified diff 文本 |
| `GetCurrentBranch(repoPath)` | 复用已有 `gitCmd.GetBranch` | 用于 set-upstream 时拼 `origin/<branch>` |
| `HasUpstream(repoPath) (bool, error)` | `git rev-parse --abbrev-ref @{u}` 成功即有上游 | push 前判断 |

> 说明：取消原方案中的 `StageFiles / UnstageFiles / StageAll / UnstageAll`——IDEA 风格不暴露 git index 的暂存语义，提交时直接按勾选文件 pathspec 提交。

`app.go` 暴露上述方法供前端 `wailsjs` 绑定调用。

### 前端

- 重构 `LocalChanges.vue`：
  - 单区变动清单，复用现有 `el-table` 的 `type="selection"`（已是复选框多选）作为"勾选要提交的文件"；
  - 行**双击**打开 diff 弹窗（`@row-dblclick`）；
  - 底部操作区：commit message 输入框（`el-input type="textarea"`）+ 【提交】【提交并推送】【推送】；【回滚】移到次要位置（如"更多"下拉）。
  - 状态徽标保留（M/A/D/R/?）。
- 新增 diff 弹窗组件（`el-dialog`）：双栏左右对照渲染 unified diff——前端解析 unified 文本为左右行数组，左栏旧版本、右栏新版本，删除行红、新增行绿。
- 操作成功后调用 `localChangesRef.loadChanges()` + `commitHistoryRef.handleRefresh()` + `gitInfoRef.handleRefresh()`。

### diff 渲染策略（实现时定）

后端返回 unified diff 文本，前端解析为 `{ left: Line[], right: Line[] }` 配对渲染双栏。首版优先**后端返回 unified 文本 + 前端轻量解析**，复杂度低；如对齐困难再考虑后端用 Go diff 库返回结构化 JSON。

### commit pathspec 安全性

`git commit -m <msg> -- <selected files>` 的 pathspec 语义确保**只提交勾选文件**，即使 git index 中存在其他已 `git add` 的文件也不会被纳入（IDEA 标准行为）。未跟踪文件需先 `git add` 才能 commit，因此后端对勾选文件统一 `git add -- <files>` 后再 `git commit -- <files>`。

## Decision (ADR-lite)

**Context**：右侧操作面板已有变动列表（仅回滚）和仓库信息/提交历史，但缺日常工作流核心（提交/推送/diff）。需选定交互范式。

**Decision**：
1. MVP 范围 = diff + **选择性 commit**（pathspec） + **push**（stash/amend/rebase/暂存区语义延后）。
2. 文件清单采用 **IDEA commit 窗口风格**：单区 + 复选框勾选要提交的文件，**不引入 VSCode 式暂存区双区**。
3. diff 触发为**双击文件 → 弹窗双栏左右对照**（已跟踪 vs HEAD，未跟踪显示全文新增）。
4. 提交/推送交互：输入框 + 【提交】【推送】【提交并推送】；无上游提示 set-upstream。
5. push 凭证**透传系统 git**，不在应用内托管。

**Consequences**：
- 优点：交互贴近 IDEA 用户习惯；复用现有表格多选机制，前端改动比双区方案更小；绕过 git index 暂存语义，对未跟踪文件友好。
- 缺点：与 git 原生"暂存区"心智模型不同，纯命令行 git 用户可能需要适应；diff 统一对比 HEAD（不像 VSCode 区分暂存/未暂存），但日常使用更直观。
- 后续演进：如需暂存区语义（部分暂存 / 行级暂存）、stash、amend、冲突解决，可在此架构上扩展。

## Out of Scope（明确排除）

- 暂存区双区 / 行级暂存（hunk staging）、stash、amend、rebase、merge、冲突解决。
- 应用内远程凭证管理（默认透传系统 git 配置）。
- 多仓库批量提交 / 推送。
- diff 语法高亮深度优化（首版红绿纯文本）。
- commit message 的 Conventional Commits 模板强制（仅 placeholder 提示）。

## Technical Notes

- 关键文件：
  - 后端：`service/git.go`、`util/git.go`、`app.go`、`model/commit.go`
  - 前端：`frontend/src/components/ContentPanel.vue`、`LocalChanges.vue`、`GitInfo.vue`、`CommitHistory.vue`
- `util.GitCommand.Execute(workDir, args...)` 是统一命令通道（30s 超时）；`util.FindGitRoot` 已存在。
- 前端通过 `wailsjs/go/main/App.js` 调后端，新增 `app.go` 方法后需重新生成 wails 绑定（`wails dev` / `wails build` 自动）。
- 中文路径、重命名文件、已删除文件在 `git add` / `git diff` / `git commit --` 时的兼容性需在单测覆盖。
- 现有 `el-table` 已具备 `type="selection"` + `@selection-change`，可直接复用为提交勾选。

## Implementation Plan（小 PR 拆分）

- **PR1 — 后端 Git 提交/推送/diff 能力 + 单测**
  `util/git.go` + `service/git.go` 新增 `Commit / Push / GetDiff / HasUpstream`（取消 Stage/Unstage）；`app.go` 暴露；`service/git_test.go` 补单测（含中文路径、未跟踪文件、pathspec 提交）。
- **PR2 — 前端 IDEA 风格清单 + 双击 diff 弹窗**
  重构 `LocalChanges.vue`（复选框清单 + 双击 diff + commit 输入框 + 三按钮）；新增 diff 弹窗（双栏左右对照）；wails 绑定；前端 `vitest` 用例。
- **PR3 — 联动刷新 + set-upstream 提示 + 回滚下沉 + 文档**
  提交/推送后联动刷新；无上游提示；【回滚】移入次要菜单；更新 `README.md` / `docs/功能说明.md`。
