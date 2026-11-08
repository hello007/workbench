# 切换分支支持选择 git 标签

## Goal

「工作目录」Git 面板的「切换分支」对话框当前仅支持分支（本地/远程），需要支持选择 git 标签进行切换。后端 `GetTags`/`ListTags` 已存在可复用，缺口在：切换入口对话框不展示标签、后端无 tag 检出语义（checkout tag 会进入 detached HEAD，与分支切换语义不同，需要产品决策与状态展示配套）。

## What I already know

### 现有链路（勘察结论）

**前端**（`frontend/src/components/ContentPanel.vue`）：
- 按钮：第 54 行「切换分支」；对话框：第 300-347 行（`el-dialog` + `el-select filterable`）
- 分组展示：`el-option-group` 本地分支（`localBranches` computed）/ 远程分支（`remoteBranches` computed，空则隐藏）
- `showBranchDialog()`（:667）→ `GetBranches(path)` → `branchList`；`currentBranchName` 取 `isCurrent` 项
- `doCheckout()`（:689）→ 按 `branch.isRemote` 传第三参 `CheckoutBranch(path, name, isRemote)`；成功后刷新 GitInfo + CommitHistory
- 仅 ContentPanel.vue 一处消费 `CheckoutBranch`（grep 确认）

**后端**：
- `CheckoutBranch(path, branchName, isRemote)`（app_git.go:686 → service/git.go:371）：tryLockRepo → 未提交变更检查（有变更拒绝并提示先提交/暂存）→ isRemote 时拆 `remote/branch` 判断本地同名分支存在性，分流 `CheckoutLocal` / `CheckoutRemote(-b)`
- `util.CheckoutLocal(dir, ref)` = `git checkout <ref>` —— **对 tag 同样生效**（检出为 detached HEAD），util 层无需新增命令方法（`Checkout` 同实现，:667）
- `GetTags(path)`（app_git.go:740 → service/git.go:1189 `ListTags`）已存在：`git for-each-ref --sort=-version:refname` 返回 `[]model.GitTag`（Name/Type/Sha/ShortSha/Message/Tagger/Date），标签面板已在消费

**detached HEAD 现状行为缺口**：
- `GetBranch`（`branch --show-current`）返回空 → GitInfo「当前分支」显示空
- `GetBranches` 过滤 `(HEAD detached` 行 → `isCurrent` 全 false → 对话框「当前分支：」为空，全部条目可点（无禁用）

**约束**：
- App 方法签名与 `model/` 导出 struct 是前后端契约真相源，变更须手动同步 `frontend/wailsjs/`（App.js / App.d.ts / models.ts 三处）；具名 string 类型不生成 models.ts 别名，须手动补（cross-layer-contracts 规则）
- 向后兼容默认：绑定方法签名变更须谨慎（AGENTS.md §7）
- 覆盖率门禁：service ≥76%、前端 ≥70%
- git 调用 UTF-8 `-c` 前缀规则适用于新增解析中文 tag 的调用（现有 `ListTags` 经 `gitCmd.Execute`，`GitCommand` 不默认带前缀；本任务若新增 for-each-ref 调用须评估）

## Assumptions (temporary)

- tag 列表直接全量加载（与标签面板一致，仓库 tag 量级可控），不做分页/懒加载
- tag 切换沿用「未提交变更拒绝切换」约束（与分支切换一致）

## Open Questions

（无——Q1/Q2 已确认，Q3 技术形态随 D3 自决，见最终确认）

## Decision (ADR-lite)

### D1：tag 切换语义 = 直接检出 detached HEAD（Q1，用户已确认 2026-11-08）

**Context**：`git checkout <tag>` 与分支切换语义不同，直接检出进入 detached HEAD，须在「检出形态 / 建分支 / 用户选择」间决策。
**Decision**：方案 A——执行 `git checkout <tag>` 直接检出，进入 detached HEAD；成功提示明确告知分离头状态。与 SourceTree/IDEA 等主流 GUI 默认一致，零副作用。
**Consequences**：用户在此状态提交改动不被分支引用（提示文案兜底）；配套需求——前端须能显示「当前处于标签 vX（分离头）」而非空白（D2 解决）；切回分支复用现有对话框，无额外入口。

### D2：detached 状态展示 = BranchList 新增 DetachedRef 字段（Q2，用户已确认 2026-11-08）

**Context**：detached 时「当前分支：」显示空白、全部条目可点（无禁用），须定信息量档位。
**Decision**：方案 1——`model.BranchList` 新增 `DetachedRef string`（向后兼容字段，`detachedRef,omitempty`）；`GetBranches` 解析 `branch -a` 输出的 `(HEAD detached at/from X)` 行时顺路捕获（现有过滤分支处，不新增 git 调用）。前端：① 对话框头部显示「当前：标签 vX（分离头）」② 该 tag 条目置灰禁用（与分支 isCurrent 禁用一致）。
**Consequences**：wailsjs models.ts 须同步 BranchList 字段；「仓库信息」面板 Branch 空白维持现状（Out of Scope）。

### D3：契约形态 = 新增 CheckoutTag 独立方法 + 无歧义检出命令（技术自决）

**Context**：改 `CheckoutBranch(path, name, isRemote)` 签名为 kind 枚举属破坏性变更（wailsjs 三处 + 消费方全改），违反向后兼容默认；且裸 `git checkout <tag>` 在分支与 tag 同名时优先检出分支（git 原生歧义）。
**Decision**：
- 新增 `App.CheckoutTag(path, tagName string) error` → `GitService.CheckoutTag`：tryLockRepo → IsGitRepository → HasLocalChanges 未提交变更拒绝（同分支切换）→ 检出。变更类不注册取消（同 Commit 先例，与 CheckoutBranch 一致）。
- util 新增 `GitCommand.CheckoutTag(dir, tag)` = `git checkout --detach refs/tags/<tag>`：`--detach` 显式声明分离头意图（省去 git 的「分离头警告」噪音判定），`refs/tags/` 全路径消除同名分支歧义。
- 前端对话框 option value 用 `kind:name` 前缀编码（`branch:` / `remote:` / `tag:`），解决同名冲突时选中项语义不可辨的问题；doCheckout 解析前缀分流到 CheckoutBranch / CheckoutTag。
**Consequences**：wailsjs App.js / App.d.ts 新增绑定 + models.ts BranchList 加字段（无新增具名 string 类型，不涉及别名规则）。

## Requirements

- 「切换分支」对话框新增「标签」分组（tagList 空则隐藏，与远程分支分组一致），复用 filterable 搜索
- 打开对话框并行加载 `GetBranches` + `GetTags`：分支失败维持现状报错；标签失败仅 ElMessage.warning 降级（分支功能不受影响）
- 选中标签执行 `git checkout --detach refs/tags/<tag>` 直接检出（detached HEAD），成功提示告知分离头状态
- tag 切换沿用「未提交变更拒绝切换」约束，与分支切换一致
- detached 状态展示：对话框头部「当前：标签 vX（分离头）」+ 当前 tag 条目禁用
- 现有分支切换行为（本地/远程/同名本地分支建分支）回归不变

## Acceptance Criteria

- [ ] 对话框展示「本地分支 / 远程分支 / 标签」三组，标签可搜索过滤；无标签仓库不显示标签分组
- [ ] 选中标签点击切换后 `git status` 处于 detached HEAD 且 `git describe --tags` 命中该 tag（service 单测断言）
- [ ] detached 时 `GetBranches` 返回 `DetachedRef` = 该引用名（`at` 形态）；非 detached 时为空（回归）
- [ ] 对话框头部正确显示「当前：标签 vX（分离头）」，且该 tag 条目置灰不可重复点击
- [ ] 工作区有未提交变更时切换标签被拒绝，提示与分支切换一致
- [ ] 分支与标签同名的仓库：选标签走 CheckoutTag（检出 tag 而非同名分支）
- [ ] 现有分支切换行为回归不变（本地/远程/isCurrent 禁用/成功后刷新 GitInfo+CommitHistory）
- [ ] wailsjs 三处同步（App.js 新增 CheckoutTag、App.d.ts 签名、models.ts BranchList.detachedRef），`npm run build` 通过
- [ ] service 单测 + 前端组件测试覆盖新增路径

## Definition of Done (team quality bar)

- `go vet ./...` / `go test ./...` 通过；前端 `npm test` 通过
- 覆盖率门禁满足（service ≥76%，前端 ≥70%）
- 契约变更同步 wailsjs 三处
- README.md 是否需要更新已确认（功能面向用户操作，预计 Git 面板相关章节需补一句）

## Out of Scope (explicit)

- 「仓库信息」面板（GetInfo）的 detached 状态展示（维持 Branch 空白现状，切回分支即恢复）
- 子模块面板（GitSubmodules）的标签切换
- 远端标签 fetch/同步（沿用 git 原生：pull 后本地可见）
- 基于标签创建新分支的入口（方案 B/C，未来如需再立任务）

## Technical Notes

- 勘察文件：ContentPanel.vue（按钮 :54、对话框 :300-347、showBranchDialog :667、doCheckout :689）、app_git.go（CheckoutBranch :686、GetTags :740）、service/git.go（GetBranches :320、CheckoutBranch :371、ListTags :1189）、util/git.go（CheckoutLocal :410、Checkout :667）、model/commit.go（GitTag :114、BranchList :67）
- `CheckoutBranch` 前端唯一消费方：ContentPanel.vue
- detached 行两形态：`* (HEAD detached at v1.0.0)`（checkout tag/SHA）与 `* (HEAD detached from <sha>)`（rebase 中间态等）——解析按 `at`/`from` 后取名字
- `ListTags` 按 `--sort=-version:refname` 版本语义序降序，对话框标签分组直接沿用该序
- UTF-8 `-c` 前缀规则评估：`ListTags`（既有）与 `branch -a`（既有）不经手中文 tag 名解析新增面；`CheckoutTag` 不解析输出，无需新增前缀
- 测试基建：service 侧用 `util/testutil`（InitTempRepo/RunGit）；前端 ContentPanel 组件测试按现有 spec 基建（无则按「组件测试覆盖新增路径」评估最小验证面，如抽 doCheckout 分流逻辑为可测纯函数）
