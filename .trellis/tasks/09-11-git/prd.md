# Git 合并变基与冲突解决

## Goal

为 WorkBench 新增 merge / rebase / 冲突解决能力，补全 Git 客户端核心闭环。当前分支管理（增删改查+切换）已全量，但缺合并变基——分支并行开发后无法在工具内收束分支，是功能断层。

## What I already know

* 后端三层架构：`util/git.go` `GitCommand`（git CLI 薄封装，`Execute`/`ExecuteWithCodes`）→ `service/git.go` `GitService`（业务+校验，如 `HasLocalChanges` 切换前检查）→ `app_git.go` `App`（Wails 绑定，路径校验+委托）
* 变更类操作走 git CLI（非 go-git）；读取类部分走 go-git（`GetCommitHistory`）
* `ExecuteWithCodes` 已支持接受非零退出码（merge 冲突时 git exit 1，可据此判定冲突）
* 前端按域分组件：`GitBranches.vue` / `GitTags.vue` / `GitRemotes.vue`，各有单测
* model 层：`BranchInfo`/`BranchList`/`FileChange`/`Commit` 等已存在
* 路线图需求（152 行）：分支合并、变基操作、冲突解决辅助、合并策略选择

## Assumptions (temporary)

* merge/rebase 走 git CLI（与现有变更操作一致），不引入 go-git 合并 API
* 冲突解决 MVP 不做内置 diff 编辑器，走「冲突文件列表 + 标记已解决 + 提交」流程
* 合并策略 MVP 支持 fast-forward / no-ff / squash 三选一

## Open Questions

* MVP 操作范围：merge / rebase / cherry-pick / abort 各纳入哪些？
* 冲突解决 UX 形态
* 合并策略可选项

## Requirements (evolving)

### MVP 范围（方案 3：最小 + cherry-pick + pull --rebase 整合）

**合并**
* `git merge <branch>` 支持 ff / no-ff / squash 三种策略
* 合并前工作区干净校验（复用 `HasLocalChanges` 口径）
* 冲突检测（`ExecuteWithCodes` 接受 exit 1）

**变基**
* `git rebase <branch>` 基础变基
* rebase 中途多 commit 冲突的 `--continue` / `--abort` / `--skip`

**Cherry-pick**
* `git cherry-pick <sha>` 单 commit 拣选
* 冲突时 `--continue` / `--abort`

**Pull --rebase 整合**
* 现有 `Pull` 增强：可选 rebase 模式（`git pull --rebase`）
* 共用冲突解决入口

**冲突解决（方案 C：文件列表 + 标记解决 + 外部打开）**
* 冲突文件列表（`git diff --name-only --diff-filter=U`）
* 点「打开」调外部编辑器（复用现有 VSCode/Obsidian 打开入口）手动改冲突标记
* 改完回 WorkBench 点「标记已解决」`git add <file>`
* 全部解决后 continue 提交（`git commit` / `git rebase --continue` / `git cherry-pick --continue`）
* abort 逃生口（merge/rebase/cherry-pick 各自 abort）
* 不做内置三方合并编辑器（留三期），不集成外部 mergetool（路线图另列独立任务）

## Decision (ADR-lite)

**Context**: 冲突解决 UX 形态选择。WorkBench 定位为开发者工作台而非全功能 Git GUI，内置 merge editor 投入产出比低。

**Decision**: 方案 C——冲突文件列表 + 外部打开 + 标记已解决 + continue/abort。复用现有外部打开能力，零新依赖。

**Consequences**: 冲突解决流程闭环，体验轻量；用户需依赖外部编辑器解决冲突标记。内置三方编辑器留三期，外部 mergetool 集成另成独立任务。

---

**Context**: Pull rebase 整合签名兼容。现有 `PullRepo(path)` / `Pull(dirPath)` 已有前端调用。

**Decision**: 方案 3——加 `useRebase bool` 参数，默认 false 保持现有行为。统一入口，rebase 是 Pull 模式而非新操作。

**Consequences**: 签名破坏性变更，须按跨层契约同步 wailsjs + 前端调用点（补 `false`）。无方法爆炸，后续 rebase 模式扩展无新增方法。

## Implementation Plan (small PRs)

**PR1 后端基建**
* `util/git.go`：Merge/Rebase/CherryPick/PullRebase 薄封装 + abort/continue/skip + ListConflictFiles + IsXxxInProgress 检测
* `service/git.go`：业务方法 + 前置校验（dirty 工作区、detached HEAD）+ GetConflictState + ResolveConflict
* `Pull(path, useRebase bool)` 签名变更，默认 false
* `model`：`ConflictState` / `MergeMode` 常量
* `app_git.go`：Wails 绑定
* 后端单测（service ≥76%、util ≥40%）

**PR2 前端 `GitMerge.vue` + Pull 增强**
* `frontend/wailsjs/` 同步（App.js / App.d.ts / models.ts）
* 新建 `GitMerge.vue`：操作区（目标分支/SHA + 操作类型 + merge 策略）+ 冲突态面板（文件列表 + 打开/标记解决/continue/abort）
* Pull 入口加 rebase 模式开关
* 前端单测（≥70%）

**PR3 集成 + 文档**
* 端到端验证：真实 merge 冲突 / rebase 多 commit 冲突 / cherry-pick 冲突 / pull --rebase 冲突
* README.md / 功能说明.md 更新
* 路线图 152 行标记完成

## Acceptance Criteria (evolving)

* [ ] merge 三策略可选（ff/no-ff/squash），fast-forward 场景按策略正确执行
* [ ] rebase 执行成功，冲突时进入冲突态
* [ ] rebase 中途多 commit 冲突可 --continue / --abort / --skip
* [ ] cherry-pick 单 commit 成功，冲突可 continue/abort
* [ ] pull --rebase 模式可选（useRebase=true），冲突走统一入口
* [ ] useRebase=false 时 Pull 行为与现有完全一致（兼容）
* [ ] 冲突态返回冲突文件列表，标记解决后可 continue
* [ ] merge/rebase/cherry-pick 均支持 abort
* [ ] 工作区不干净时拒绝 merge/rebase/cherry-pick
* [ ] detached HEAD 下禁止 merge/rebase
* [ ] 操作后分支列表/提交历史可刷新
* [ ] 后端单测达门禁基线
* [ ] 前端单测达门禁基线

## Definition of Done (team quality bar)

* 后端单测覆盖（service ≥76% 基线，util ≥40%）
* 前端单测覆盖（≥70% 硬失败）
* `frontend/wailsjs/` 绑定同步（App.js / App.d.ts / models.ts 三处，见 cross-layer-contracts.md）
* README.md / 功能说明.md 评估更新
* 路线图标记完成

## Out of Scope (explicit)

* 待 brainstorm 收敛

## Technical Approach

### 后端（三层，遵循现有架构）

**util/git.go `GitCommand` 新增薄封装**
* `Merge(workDir, branch, mode string)` — mode: ff/no-ff/squash
* `Rebase(workDir, branch string)`
* `CherryPick(workDir, sha string)`
* `PullRebase(workDir string)` — `git pull --rebase`
* `MergeAbort` / `RebaseAbort` / `RebaseContinue` / `RebaseSkip` / `CherryPickAbort` / `CherryPickContinue`
* `ListConflictFiles(workDir)` — `git diff --name-only --diff-filter=U`
* `IsMergeInProgress(workDir)` — 检 `.git/MERGE_HEAD`
* `IsRebaseInProgress(workDir)` — 检 `.git/rebase-merge/` 或 `.git/rebase-apply/`
* `IsCherryPickInProgress(workDir)` — 检 `.git/CHERRY_PICK_HEAD`
* 冲突类操作走 `ExecuteWithCodes` 接受 exit 1

**service/git.go `GitService` 新增业务方法**
* `Merge(path, branch, mode)` / `Rebase(path, branch)` / `CherryPick(path, sha)`
* `Pull(path, useRebase bool)` — 现有 Pull 增 rebase 模式参数，默认 false 保持现有行为
* `GetConflictState(path)` — 返回 `{Type, Files}`，Type: merge/rebase/cherry-pick/none
* `ResolveConflict(path, file)` — `git add`
* `ContinueMerge` / `ContinueRebase` / `ContinueCherryPick`
* `AbortMerge` / `AbortRebase` / `AbortCherryPick`
* 前置校验：工作区干净（`HasLocalChanges`）、非 detached HEAD

**app_git.go `App` 新增 Wails 绑定**
* 路径校验 + 委托 service，签名风格对齐现有 `CheckoutBranch`/`CreateBranch`

**model 层新增**
* `MergeMode` 常量（ff/no-ff/squash）
* `ConflictState` struct（Type + Files）

### 前端

**新建 `GitMerge.vue`** — 与 `GitBranches.vue`/`GitTags.vue` 平级
* 操作区：选目标分支/SHA + 操作类型（merge/rebase/cherry-pick）+ 策略（merge）
* 冲突态：展示冲突文件列表，每文件「打开」「标记已解决」按钮，底部 continue/abort
* 拉取增强：Pull 入口加 rebase 模式开关

### 跨层同步
* `frontend/wailsjs/go/main/` App.js / App.d.ts 同步新绑定
* `frontend/wailsjs/go/models.ts` 同步 `ConflictState`/`MergeMode`

## Technical Notes

* `util/git.go:56` `ExecuteWithCodes` 可处理 merge 冲突的 exit 1
* `service/git.go` 现有 `HasLocalChanges` 检查模式可复用（merge/rebase 前置工作区干净校验）
* `CheckoutBranch` 的 detached HEAD / dirty 工作区校验口径可参照
* 跨层契约：[docs/spec/cross-layer-contracts.md](../../docs/spec/cross-layer-contracts.md)
* 测试门禁：[docs/spec/test-coverage-gate.md](../../docs/spec/test-coverage-gate.md)（service ≥76%、util ≥40%、前端 ≥70%）
* Pull 现有签名 `PullRepo(path) (string, error)`，增 rebase 模式须评估兼容（加参数或新增 `PullRebaseRepo`）

## Out of Scope (explicit)

* 内置三方合并编辑器（留三期）
* 外部 mergetool 集成 / Beyond Compare / WinMerge（路线图 199 行另列独立任务）
* rebase -i 交互式变基
* merge 策略 ours/theirs（仅 ff/no-ff/squash）
* 全局冲突态横幅（留三期）
