# Submodule 支持

## Goal

落地路线图「Git 高级操作 → Submodule 支持」最后一块拼图：在 WorkBench 中查看/初始化/更新/状态显示 Git submodule。复用现有 tag/remote/merge 的 service+app+model+前端单组件分层模式，保持一致。

## What I already know

### 现有分层模式（参考 GitTags / GitRemotes）

* **service/git.go**：集中所有 Git 操作（74 symbols）。`ListTags`(`service/git.go:872`)、`CreateTag`、`ListRemotes`、`AddRemote` 等方法均通过 `s.gitCmd.Execute(gitRoot, args...)` 调底层 `util.GitCommand`（os/exec fork git），不直接用 go-git
* **util/git.go**：`GitCommand` 封装 `git` 子进程；`FindGitRoot` 定位仓库根；`IsGitRepositoryFast` 已覆盖 submodule（`.git` 为文件场景，不要求 IsDir）
* **model/commit.go**：数据契约集中（`GitTag`、`GitRemote`、`GitRemoteInfo`、`MergeMode`、`ConflictState` 等具名类型 + JSON tag）
* **前端**：`GitTags.vue` / `GitRemotes.vue` 单组件，`props.repoPath` 驱动，`loadXxx` + `ElMessageBox.confirm` 二次确认 + `handleGitError` 统一错误；挂载在 `ContentPanel.vue` 的 tab 体系
* **并发控制**：变更类操作走 `tryLockRepo(repoPath)`（`sync.Mutex` TryLock，`ErrOperationInProgress`）+ `precheckMutation`（校验工作区干净 + 非 detached HEAD）；只读查询不抢锁
* **跨层契约**：App 方法签名或 model 导出 struct 变更须手动同步 `frontend/wailsjs/go/main/App.js` / `App.d.ts` / `models.ts` 三处（见 `docs/spec/cross-layer-contracts.md`）

### 关键约束（已发现）

* **FindGitRoot 潜在 bug**：`util/git.go` 注释明确指出 `FindGitRoot` 用 `info.IsDir()` 判定，对 worktree/submodule 漏判（submodule 的 `.git` 是文件，内容 `gitdir: /path/...`）。`IsGitRepositoryFast` 已修此问题，但 `FindGitRoot` 未修。submodule 操作依赖正确根定位，本任务须修或绕过
* **service/git.go + util/git.go 现有 submodule 字样均为注释**，无实际实现
* **go-git submodule API**：init/update 能力较弱，`git submodule status` 无直接等价；现有项目全部走 os/exec，go-git 仅用于 commit history Log

### 后端测试范式

* `service/git_tag_test.go`、`service/git_concurrency_test.go` 有完整注入测试范式
* 覆盖率门禁：service ≥76%、前端 ≥70%（exclude wailsjs）

## Assumptions (temporary)

* 沿用 os/exec `git submodule` 子命令（与 tag/remote/merge 一致），不引入 go-git submodule API（go-git 缺 deinit/add、status 测不出 dirty，见研究 2）
* **状态显示双命令融合**：`git submodule status`（SHA/init 态）+ `git status --porcelain=2`（dirty 标记），按 path 关联 —— `git submodule status --porcelain` 不存在，且 `submodule status` 不检测 dirty 工作区（见研究 1）
* 单仓库维度，不跨仓汇总
* FindGitRoot bug 修复纳入本任务（前置依赖，改动极小，见研究 2）

## Research References

* [`research/submodule-ops-and-status.md`](research/submodule-ops-and-status.md) — `git submodule status --porcelain` 不存在且不检测 dirty；状态须双命令融合；删除须显式清 `.git/modules/<name>`；update 默认 detached HEAD
* [`research/findgitroot-fix-and-gogit-submodule.md`](research/findgitroot-fix-and-gogit-submodule.md) — FindGitRoot 复用 IsGitRepositoryFast 修复，~24 caller 安全且修潜在 bug；go-git submodule 缺 deinit/add，坚持 os/exec

## Open Questions

（无 — 待 Step 8 用户最终确认）

## Requirements (evolving)

### MVP 范围（已定：方案 A 完整闭环 6 操作）

* **查看 submodule 列表**：`ListSubmodules(repoPath)` 双命令融合 —— `git submodule status`（前导码/SHA/path/describe）+ `git status --porcelain=2`（dirty 标记），按 path 关联
* **状态显示**：前端表格列 path/shortSha/describe/状态标签/操作；`el-tag` 四色 —— 灰=未初始化、绿=干净、橙=dirty 或 SHA 不同步、红=冲突；detached HEAD 警告标记
* **初始化**：`InitSubmodules(repoPath, recursive)` → `git submodule update --init [--recursive]`，抢锁
* **更新**：`UpdateSubmodules(repoPath, mode, recursive, path?)` → mode ∈ checkout(默认)/merge/rebase/remote，`git submodule update [--merge|--rebase|--remote] [--recursive]`，抢锁；支持单 submodule（传 path）；默认 checkout+recursive 钉 index SHA 不漂移
* **添加**：`AddSubmodule(repoPath, url, path, branch)` → `git submodule add [-b branch] <url> <path>`，抢锁 + `precheckMutation`（工作区干净）
* **删除**：`RemoveSubmodule(repoPath, path)` → 三步 `deinit -f` + `git rm -f` + 显式 `rm -rf .git/modules/<name>`（git 不自动清，遗漏致历史错乱），抢锁
* **detached HEAD 检测与切换**：列表对 detached submodule 显示警告标记（橙 tag "detached"，检测走 `git -C <path> branch --show-current` 为空）+ 行内"切换跟踪分支"操作（读 `.gitmodules` 的 branch 配置，`git -C <path> checkout <branch>`），避免用户在 detached 上开发丢提交
* **FindGitRoot 修复**：复用 `IsGitRepositoryFast` 判定（仅判 `.git` 存在不要求 IsDir），修 submodule/worktree 漏判，~24 caller 安全
* 复用现有分层模式 + tryLockRepo 并发控制 + 跨层契约同步

## Acceptance Criteria (evolving)

* [ ] submodule 列表正确展示四态（未初始化/干净/dirty/SHA 不同步/冲突），dirty 来自 `status --porcelain=2` 而非 `submodule status`
* [ ] detached HEAD submodule 显示警告标记 + "切换跟踪分支"操作可执行
* [ ] init/update 支持 recursive 开关 + 4 模式（checkout/merge/rebase/remote）
* [ ] add 创建 `.gitmodules` + 工作区检出 + `.git` 文件指针
* [ ] remove 三步清理（deinit + git rm + `.git/modules/<name>`），无残留
* [ ] 所有变更操作走 tryLockRepo，add/remove 加 precheckMutation
* [ ] FindGitRoot 对 submodule 路径不再漏判（复用 IsGitRepositoryFast）
* [ ] 后端 service 覆盖率 ≥76%，前端 ≥70%
* [ ] 跨层契约同步（wailsjs App.js/App.d.ts/models.ts 三处）

## Definition of Done (team quality bar)

* Tests added/updated（service 单测 + 前端组件测）
* Lint / typecheck / CI green
* README.md / 功能说明.md 补 submodule 功能
* 跨层契约同步

## Out of Scope (explicit)

* **跨仓 submodule 汇总**：MVP 单仓维度，跨仓留后续
* **嵌套 submodule 树形呈现**：MVP 用 `--recursive` 操作，列表仅列顶层；树形缩进留后续
* **submodule 内文件级 diff 编辑器**：MVP 不做内置三方合并编辑器（与 merge 冲突策略一致）
* **`git submodule foreach` 批量自定义命令**：留后续
* **submodule 同步 `git submodule sync`**（URL 变更同步）：留后续
* **`update --remote` 后自动 superproject commit 指针**：MVP 仅提示用户须手动 commit，不自动提交
* **file:// 协议处理**：git 环境配置（`protocol.file.allow`），错误原样透传

## Decision (ADR-lite)

### 状态查询：双命令融合（非 `--porcelain`）

**Context**：PRD 初版假设 `git submodule status --porcelain`，实测不存在；且 `submodule status` 不检测 dirty 工作区。

**Decision**：`ListSubmodules` = `git submodule status`（前导码/SHA/path/describe/init 态）+ `git status --porcelain=2`（dirty 标记），按 path 关联产出 `GitSubmodule` 结构。

**Consequences**：两次 git 子进程调用（只读不抢锁，与 ListTags 一致）；解析逻辑稍复杂但数据完整；dirty 检测可靠。

### 底层实现：os/exec（非 go-git）

**Context**：go-git submodule API 缺 deinit/add，`Status.IsClean()` 测不出工作区 dirty。

**Decision**：沿用 os/exec `git submodule` 子命令，与 tag/remote/merge 一致。

**Consequences**：依赖系统 git（桌面工具已有约束）；无 go-git API 限制；测试用临时仓库实测命令输出（参考 git_tag_test.go 范式）。

### FindGitRoot 修复：纳入本任务

**Context**：`FindGitRoot` IsDir 判定对 submodule（`.git` 为文件）漏判，submodule 操作前置依赖。

**Decision**：循环体复用 `IsGitRepositoryFast(abs)`（已测去重），不新增并行函数；~24 caller 安全，且修 commit 历史/统计返回 superproject 数据的潜在 bug。

**Consequences**：改动极小；同修 worktree 路径；独立 commit 便于回溯。

## Technical Approach

### 后端（Go）

* **util/git.go**：`FindGitRoot` 循环体改用 `IsGitRepositoryFast(abs)`（删 `&& info.IsDir()` 或复用函数）；新增 `SubmoduleStatus`/`SubmoduleInit`/`SubmoduleUpdate`/`SubmoduleAdd`/`SubmoduleDeinit` 等 `GitCommand` 方法（os/exec 封装，参考现有 `Tag`/`Remote` 方法）
* **service/git.go**：新增 `ListSubmodules`（双命令融合解析）+ `InitSubmodules`/`UpdateSubmodules`/`AddSubmodule`/`RemoveSubmodule`/`CheckoutSubmoduleBranch`（detached 切换）；变更类走 `tryLockRepo` + `precheckMutation`（add/remove）
* **model/commit.go**：新增 `GitSubmodule` struct + `SubmoduleUpdateMode` 具名类型（checkout/merge/rebase/remote）
* **app_git.go**：新增 `GetSubmodules`/`InitSubmodules`/`UpdateSubmodules`/`AddSubmodule`/`RemoveSubmodule`/`CheckoutSubmoduleBranch` 绑定方法（参考 `GetRemotes`:744 范式）

### 前端（Vue3）

* **frontend/src/components/GitSubmodules.vue**：单组件（参考 `GitRemotes.vue`）—— `props.repoPath` 驱动 + `loadSubmodules` + 表格（path/shortSha/describe/状态标签/操作）+ 全量 Init/Update 按钮 + 单行 Init/Update/Remove + Add 弹窗 + detached 切换分支；`el-tag` 四色状态；`ElMessageBox.confirm` 二次确认 + `handleGitError`
* **ContentPanel.vue**：合并/变基 tab 后新增 `<el-tab-pane label="子模块" name="submodules" lazy>` + import

### 跨层契约

* 新增 App 方法 + `GitSubmodule`/`SubmoduleUpdateMode` model 须同步 `frontend/wailsjs/go/main/App.js` / `App.d.ts` / `models.ts` 三处（见 `docs/spec/cross-layer-contracts.md`）；`SubmoduleUpdateMode` 为 `type X string` 具名类型，wails generate 不自动生成 models.ts 别名须手补

### 实现计划（小 PR）

* **PR1**：FindGitRoot 修复（独立 commit）+ `util/git.go` submodule GitCommand 方法 + `model/commit.go` 数据契约 + service 层 `ListSubmodules`/`InitSubmodules`/`UpdateSubmodules` + service 单测（覆盖率 ≥76%）
* **PR2**：service 层 `AddSubmodule`/`RemoveSubmodule`/`CheckoutSubmoduleBranch` + `app_git.go` 全部绑定 + 跨层契约同步 + service 单测补全
* **PR3**：前端 `GitSubmodules.vue` 组件 + `ContentPanel.vue` tab 挂载 + 组件测（覆盖率 ≥70%）+ README/功能说明.md 更新 + 边界场景（空仓库无 submodule/未初始化/detached/嵌套 recursive）

## Technical Notes

* 参考实现：`ListTags`(`service/git.go:872`) / `ListRemotes`(`service/git.go:999`) / `GitTags.vue` / `GitRemotes.vue`
* App 绑定范式：`GetRemotes`(`app_git.go:744`)
* 并发：`tryLockRepo`(`service/git.go:65`) + `precheckMutation`(`service/git.go:1265`)
* 跨层契约：`docs/spec/cross-layer-contracts.md`（`type X string` 具名类型须手补 models.ts 别名）
* 已知 bug：`FindGitRoot`(`util/git.go:161`) IsDir 判定漏 submodule（注释自述）；`IsGitRepositoryFast`(`util/git.go:97`) 已修
* 前端挂载：`ContentPanel.vue` 合并/变基 tab（`:82`）后新增子模块 tab
* 测试范式：`service/git_tag_test.go`、`service/git_concurrency_test.go`（临时仓库实测命令输出）
* 临时实测仓库：`C:/Users/liuyang/AppData/Local/Temp/submod-demo`（研究用，可 `rm -rf` 清理）
