# Git 分支与暂存增强

## Goal

补全 Git 域两项 P1 缺口:**分支增删改**(创建/删除/重命名,查看+切换已实现)与**单文件暂存**(Stage/Unstage,数据层 `Staged` 字段已就绪、前端未消费)。两者同触达 `service/git.go` + `app_git.go` + wailsjs 三处 + Git 面板,合并一任务避免重复动同批文件。复用 GitTags/GitRemotes 已建立的 card 范式与跨层同步流程。

## What I already know

### 后端 service/git.go

- `GetBranches`(132):`git branch -a` 解析,返回 `BranchInfo{Name,IsRemote,IsCurrent}`,过滤 `HEAD ->` / detached。
- `CheckoutBranch`(183):切换前 `HasLocalChanges` 检查,脏工作区即拒绝("请先提交或暂存后再切换分支")。远程分支含本地化逻辑(`CheckoutRemote`)。
- `getLocalBranchNames`(116):本地分支名列表,可复用于重命名/删除前校验。
- `GetLocalChanges`(394):`git status --porcelain -z --untracked-files=all`,**已返回 `Staged` 字段**(seg[0] != ' ' && != '?')。
- `Commit`(515):`git add -- <files>` + `git commit -m <msg> -- <files>`,pathspec 选择性提交已就绪。
- `DiscardChanges`(449):区分已跟踪/未跟踪,分别 `checkout HEAD --` / `clean -fd`。
- exec 范式:`s.gitCmd.Execute(gitRoot, args...)` → `(string, error)`,`util.FindGitRoot` 定位根,错误 `fmt.Errorf("…: %w", err)`。
- GitTags/GitRemotes 范式(722-922):`ListTags/CreateTag/DeleteTag/PushTag` + `ListRemotes/AddRemote/RemoveRemote/Fetch/SetBranchUpstream`,均走 `s.gitCmd.Execute`。

### app_git.go 桥接

- 24 个 Git 方法,`a.gitSvc.Xxx` 委派,返回 `(*model.X, error)` / `(string, error)` / `error`,`path==""` 前置校验。
- 新增方法需同步 `frontend/wailsjs/` 的 `App.js` / `App.d.ts` / `models.ts` 三处(见 [cross-layer-contracts.md](../../../docs/spec/cross-layer-contracts.md))。

### 前端

- `LocalChanges.vue`:el-card + el-table(单区),`type=selection` 勾选 + 状态 tag + 路径列,footer 含 commit 输入 + 提交/提交并推送/推送/更多(回滚)。**未分已暂存/未暂存双栏,`Staged` 字段未消费**。
- `ContentPanel.vue:593` `showBranchDialog` / `doCheckout`:分支下拉弹窗,`GetBranches` → `branchList` → `CheckoutBranch(path, name, isRemote)`。仅查看+切换。
- GitInfo.vue / GitTags.vue / GitRemotes.vue:el-card + el-descriptions + Refresh,Home 按需挂载。

### model

- `FileChange{Path, Status, Staged}`(model/commit.go:23)
- `BranchInfo{Name, IsRemote, IsCurrent}`(model/commit.go:30)、`BranchList{Branches}`(:37)

### 测试门禁

- model/server ≥80% + service ≥76% 基线 + util ≥40% + 前端 ≥70% 硬失败(见 [test-coverage-gate.md](../../../docs/spec/test-coverage-gate.md))。

## Decision (ADR-lite)

**Context**: 9 项决策影响分支管理 UI、暂存区交互、提交语义与命令选择。

**Decision**(逐项用户确认):

| 序 | 决策点 | 选择 | 说明 |
|---|---|---|---|
| 1 | 分支管理 UI 形态 | B | 新建 GitBranches.vue 独立面板,对齐 GitTags/GitRemotes 范式 |
| 2 | 切换入口归属 | B | 保留 ContentPanel 原弹窗切换,面板仅管增删改,职责隔离 |
| 3 | 删除安全策略 | B | `git branch -d` 失败后二次确认走 `-D` 强删,不直接暴露强删 |
| 4 | 暂存区双栏形态 | B | 单表分组(未暂存上/已暂存下)+ 行级 +/- 按钮,不拆双栏表 |
| 5 | 暂存与提交语义 | A | 提交仍走勾选集合(`Commit` 不改),暂存区解耦为工作区整理辅助 |
| 6 | 暂存操作粒度 | A | 行级 + 批量,批量「暂存选中/取消暂存选中」并入现有「更多」下拉 |
| 7 | 分支创建起点 | A | 仅从当前 HEAD,不支持指定 commit/tag |
| 8 | 重命名范围 | B | 任意本地分支含当前,仅本地不触远程 |
| 9 | 取消暂存命令 | A | 统一 `git restore --staged`,git 2.25+ 已通配已跟踪/未跟踪 |

**Consequences**:
- 优点:UI 范式一致(GitBranches 对齐 GitTags/GitRemotes)、提交链路零改动(`Commit` 不动)、暂存语义清晰(勾选=提交集合,暂存=工作区整理)
- 风险:两入口并存(切换弹窗 + 管理面板)有轻微心智割裂,后续可合并;`git restore --staged` 依赖 git 2.25+(2020 发布,普及率高,风险极低)
- 演进:未来可加指定起点建分支、部分暂存 `git add -p`、git stash,当前 Out of Scope

## Requirements

### 分支管理(GitBranches.vue 独立面板)

- 列出本地分支(名称 + 是否当前),仅展示本地分支(操作目标);远程分支切换仍走原弹窗,不在本面板展示
- 创建新分支:输入分支名,从当前 HEAD 创建(`git branch <name>`),重名报错透传
- 删除分支:`git branch -d <name>`,失败(含 "not fully merged")弹二次确认走 `-D` 强删;当前分支禁用删除按钮
- 重命名分支:输入新名,`git branch -m [old] <new>`(当前分支省略 old),仅本地不触远程
- 操作后列表刷新

### 单文件暂存(LocalChanges.vue 改造)

- 暂存单文件 / 批量暂存选中(`git add -- <files>`)
- 取消暂存单文件 / 批量(`git restore --staged -- <files>`,统一命令通配已跟踪/未跟踪)
- 单表分组展示:未暂存组在上、已暂存组在下,消费 `Staged` 字段
- 行级 +/- 按钮(暂存/取消暂存),批量「暂存选中/取消暂存选中」并入「更多」下拉
- 暂存/取消暂存后面板即时刷新

## Acceptance Criteria

- [ ] 列出本地分支,标记当前分支
- [ ] 创建分支后列表刷新,重名报错明确
- [ ] 删除分支:`-d` 成功即删;未合并弹二次确认,确认后 `-D` 强删;当前分支删除按钮禁用
- [ ] 重命名分支:当前分支与非当前均支持,重命名后列表刷新;新名重名报错明确
- [ ] 暂存单文件 + 批量选中,面板即时刷新,文件落入已暂存组
- [ ] 取消暂存单文件 + 批量,已跟踪回未暂存组、未跟踪回未跟踪状态
- [ ] 单表分组正确:未暂存上、已暂存下,`Staged` 字段驱动
- [ ] 行级 +/- 按钮与「更多」下拉批量操作均可用
- [ ] 提交链路未改:勾选 + 提交信息 → `CommitFiles` 行为不变
- [ ] wailsjs 三处绑定(App.js / App.d.ts / models.ts)同步 5 个新方法
- [ ] 后端单测覆盖 CreateBranch / DeleteBranch(含 force 路径)/ RenameBranch / StageFiles / UnstageFiles
- [ ] 前端测试覆盖 GitBranches 面板渲染 + 操作交互、LocalChanges 暂存交互

## Definition of Done

- 后端 `go test ./...` 绿,service 覆盖率 ≥76% 基线
- 前端 `npm test` 绿,≥70%(exclude wailsjs)
- wailsjs 三处绑定同步
- `README.md` / `docs/功能说明.md` 按需更新(分支管理 + 暂存操作)
- 路线图分支管理项补勾选

## Out of Scope

- 切换分支(走 ContentPanel 原弹窗,本面板不含)
- 删除远程分支 / 重命名推远程(`git push origin :old new`)
- 从指定 commit/tag 建分支(仅 HEAD)
- 强制删除一键入口(必须 `-d` 失败二次确认)
- 部分暂存 `git add -p` / `git stash`
- 已暂存区 diff 查看(staged diff,现 `GetDiff` 对比 HEAD)
- 分支名前端合法性预校验(git 自身报错透传)

## Technical Approach

### 后端新方法(service/git.go + app_git.go 桥接)

| GitService 方法 | App 桥接 | git 命令 | 说明 |
|---|---|---|---|
| `CreateBranch(repoPath, name string) error` | `CreateBranch(path, name) error` | `git branch <name>` | 从 HEAD 创建 |
| `DeleteBranch(repoPath, name string, force bool) error` | `DeleteBranch(path, name, force) error` | `git branch -d` / `-D` | force 走强删 |
| `RenameBranch(repoPath, oldName, newName string) error` | `RenameBranch(path, oldName, newName) error` | `git branch -m [old] <new>` | oldName 空 = 当前分支 |
| `StageFiles(repoPath, files []string) error` | `StageFiles(path, files) error` | `git add -- <files>` | 暂存 |
| `UnstageFiles(repoPath, files []string) error` | `UnstageFiles(path, files) error` | `git restore --staged -- <files>` | 取消暂存 |

- exec 复用 `s.gitCmd.Execute(gitRoot, args...)` + `util.FindGitRoot`,与 GitTags/GitRemotes 同范式
- `path==""` 前置校验,错误 `fmt.Errorf("…: %w", err)`
- 无新 model struct,复用 `BranchInfo` / `FileChange`

### 前端

- **新建 `GitBranches.vue`**:el-card + 分支列表(本地)+ 顶部「新建分支」按钮 + 行级「删除/重命名」操作,复用 GitTags.vue card 范式,Home 按需挂载(对齐 GitTags/GitRemotes 挂载点)
- **改造 `LocalChanges.vue`**:
  - 单表分组:`changes` 按 `Staged` 排序(未暂存组上、已暂存组下),可加分隔行或组标签
  - 行级 +/- 按钮(未暂存行显「+暂存」,已暂存行显「-取消暂存」)
  - 「更多」下拉增 `stageSelected` / `unstageSelected` 命令(复用现有 `onMoreCommand` + `selectedChanges`)
  - footer commit 区不动(`Commit` 语义不变)

### 跨层契约

- 5 个新 App 方法 → 同步 `frontend/wailsjs/` 的 `App.js` / `App.d.ts` / `models.ts` 三处
- 无新 model struct → `models.ts` 无新类型(若 `force bool` 参数仅签名同步,无新导出字段)

## Technical Notes

- 参考文件:`service/git.go`(132/183/394/449/515/722-922)、`app_git.go`、`model/commit.go`、`frontend/src/components/LocalChanges.vue`、`ContentPanel.vue:593`、`GitTags.vue`/`GitRemotes.vue` 范式
- 跨层契约:`docs/spec/cross-layer-contracts.md`
- 测试门禁:`docs/spec/test-coverage-gate.md`
