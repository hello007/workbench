# Research: go-git 消费面 API 盘点与双路线评估

- **Query**: 盘点全部 go-git 消费面 API 清单并做双路线（A 去依赖走 CLI / B 留库升 v6）评估
- **Scope**: internal（源码逐文件盘点）
- **Date**: 2026-10-21
- **依赖版本**: `github.com/go-git/go-git/v5 v5.19.2`（go.mod:10；间接依赖 go-billy v5.9.1 / gcfg v1.5.1-...，go.mod:32-33）
- **Go 版本**: go 1.26.0 / toolchain go1.26.10（go.mod:3-5）

## 关键结论（前置）

| 维度 | 结论 |
|---|---|
| 消费面 | 全仓 import go-git 的正是确认的 6 文件，无遗漏（grep 全仓核对） |
| 运行时消费点 | 6 处，其中 **1 处是死代码**（app_git.go `getCommitFiles`/`getTreeFiles`，无任何调用者） |
| sentinel error 消费 | **零**。6 文件均未引用 go-git 任何错误值/常量语义判定（仅 `plumbing.HashReference` 一个枚举常量），错误处理全是 `err != nil` 黑盒判断 → 去依赖的错误映射成本极低 |
| 测试消费性质 | 3 个测试文件共 6 处消费全部是 **fixture 便利性消费**（PlainOpen+Head 取缓存键），非深度耦合，可随生产签名改造一并消除 |
| 性能决策锚点 | 唯一硬约束是 `HasRemotesBatch`（service/git.go:556）：NF1 = 100 仓库 < 3s（PRD 07-19-repo-filter-dialog），go-git 读 .git/config 是刻意免 fork 决策 |
| 推荐路线 | **混合路线（以 A 为主）**：5 个文件低风险去依赖后 go-git 消费点归零或仅剩 1 处；唯一决策锚是 HasRemotesBatch（详见 §5.1） |

---

## 一、逐文件 API 明细

### 1.1 app_git.go（运行时代码，主包）

import（app_git.go:11-13）：`go-git/v5`、`plumbing`、`object`。

| file:line | API | 用途 | 路线 A 可行性 | 路线 B 适配点 |
|---|---|---|---|---|
| app_git.go:122 | `git.PlainOpen(gitRoot)` | GetGitRemoteURL 打开仓库 | 可消：`util.FindGitRoot` 已保证仓库定位，CLI `git remote get-url origin` + `git rev-parse` 替代 | import 路径改 /v6；签名待对 breaking 清单核验 |
| app_git.go:128 | `repo.Remote("origin")` | 取 origin 远程（无 origin 返空信息降级，:130-136） | 可消：CLI `git config --get remote.origin.url` 失败即同语义降级 | 待核验 |
| app_git.go:140-141 | `remote.Config().URLs[0]` | 取远程 URL | 可消：CLI 输出即 URL | Remote/Config 结构若 v6 有变须核验 |
| app_git.go:145 | `repo.Head()` | 取 HEAD 引用 | 可消：`git rev-parse HEAD` | 待核验 |
| app_git.go:151-152 | `head.Name().Short()` / `head.Name().IsBranch()` | 当前分支名 + detached 判定 | 可消：`git rev-parse --abbrev-ref HEAD`（detached 返 "HEAD"）或 `git symbolic-ref --short -q HEAD`（失败即 detached） | Reference.Name() 拆分是 v6 已知关注面，待对 breaking 清单核验 |
| app_git.go:192-201 | `git.PlainOpen` + `repo.Head()` + `head.Hash().String()` | GetCommitHistory：仅取 HEAD SHA 供缓存键与增量判定 | 可消：CLI `git rev-parse HEAD` | 待核验 |
| app_git.go:249-257 | 同上 | GetRepoStats：同形态 | 可消：同上 | 待核验 |
| app_git.go:523-528 | `*plumbing.Reference` 参数 + `head.Name().IsBranch()` / `head.Name().String()` / `head.Hash().String()` | `commitHistoryCacheKey`：分支用 ref 全名、detached 用 SHA 构造缓存键 | 可消：签名改造为 `(gitRoot, headSHA, branchName string)` 或内部 CLI 探测；这是测试消费的耦合锚点 | plumbing.Reference 形态变化影响签名，待核验 |
| app_git.go:570-603 | `commit.Tree()` / `commit.Parent(0)` / `tree.Patch(parentTree)` / `patch.FilePatches()` / `patchObj.Files()` / `from.Path()` | `getCommitFiles`：逐提交 tree patch 取变更文件 | **死代码**：全仓 grep 无调用者（提交历史采集已 CLI 化，commit_history_cli.go:31/252 注释说明语义对齐旧实现）。直接删除 | 删除后 `object` import 即可移除，v6 核验面减一 |
| app_git.go:606-618 | `tree.Files().ForEach(...)` / `file.Name` | `getTreeFiles`：全树列举（100 上限），仅被死代码 getCommitFiles 内部调用（:580） | **死代码**：同上删除 | 同上 |

要点：app_git.go 的 plumbing 活跃消费实质只有「HEAD SHA + 分支名 + detached 判定」三件事（GetGitRemoteURL 的分支信息 + 两处缓存键）；object 消费全部位于死代码。**无任何对象库读取（blob/tree 遍历）留在活跃路径**。

### 1.2 service/git.go（运行时代码）

import（service/git.go:14）：仅 `go-git/v5` 主包。

| file:line | API | 用途 | 路线 A 可行性 | 路线 B 适配点 |
|---|---|---|---|---|
| service/git.go:575 | `git.PlainOpen(path)` | HasRemotesBatch 打开仓库（并发 8） | **决策点**：见 §5.1，去依赖需新增封装 | 待核验 |
| service/git.go:576-577 | `repo.Remotes()` + `len(remotes) > 0` | 判定是否配置远程（读 .git/config，免 fork） | 同上 | Remotes() 语义待核验 |

该文件其余 2200+ 行全部走 `util.GitCommand` CLI 封装（如 GetLocalChanges :600、各冲突操作），go-git 仅此一处。

### 1.3 service/dashboard.go（运行时代码）

import（dashboard.go:12-13）：`go-git/v5` + `plumbing`。

| file:line | API | 用途 | 路线 A 可行性 | 路线 B 适配点 |
|---|---|---|---|---|
| dashboard.go:298 | `git.PlainOpen(gitRoot)` | ComputeBranchSyncInfo 打开仓库 | 可消：`git for-each-ref` 单次 fork 覆盖全部需求 | 待核验 |
| dashboard.go:303 | `repo.Head()` | 取 HEAD；headErr + 分支名非空 = unborn branch 降级（:305-309） | 可消：`git symbolic-ref` 成功 + `git rev-parse HEAD` 失败 = unborn，语义可精确平移 | 待核验 |
| dashboard.go:312 | `head.Hash().String()` | HeadSha | 可消：rev-parse 输出 | 待核验 |
| dashboard.go:349-350 | `plumbing.ReferenceName("refs/heads/"+branch)` / `("refs/remotes/"+upstreamName)` | 构造待匹配 ref 全名 | 可消：for-each-ref 直接传 `refs/heads/<branch> refs/remotes/<upstream>` 限定模式 | ReferenceName 若 v6 改形态须核验 |
| dashboard.go:351-355 | `repo.References()` + `iter.Close()` | 枚举全部引用（迭代器） | 可消：`git for-each-ref --format=%(objectname) %(refname)` 一次 fork，天然只列 refs | v6 迭代器 API 待核验 |
| dashboard.go:356-374 | `iter.ForEach(func(ref *plumbing.Reference))` + `ref.Type()` + `plumbing.HashReference` + `ref.Name()` + `ref.Hash().String()` | 过滤当前本地分支与上游远程引用两类条目的 SHA | 可消：for-each-ref 输出按 refname 前缀分流，`HashReference` 过滤天然不需要（for-each-ref 只输出 hash 型 ref） | `plumbing.HashReference` 是全仓唯一引用的 go-git 枚举常量，v6 待核验 |

上下文：该函数调用频率低（前端一次 IPC 聚合），同函数内已有 3-4 次 CLI fork（`branch --show-current` :292、`rev-parse @{u}` :342、`ComputeAheadBehind` 的 rev-parse/rev-list :252/257），再加 1 次 fork 无感知。注释明言「refs 数量极小，成本可忽略」（:276）。

### 1.4 app_git_stats_test.go（仅测试代码）

| file:line | API | 用途 | 路线 A 可行性 |
|---|---|---|---|
| app_git_stats_test.go:164 | `git.PlainOpen(gitRoot)` | `currentHeadInfo` helper：取 fixture 仓库 head 供缓存键构造 | fixture 便利消费。可消：生产 `commitHistoryCacheKey` 签名改造后，改用 CLI（rev-parse + symbolic-ref）或 testutil helper |
| app_git_stats_test.go:168 | `repo.Head()` | 同上 | 同上 |
| app_git_stats_test.go:172 | `commitHistoryCacheKey(gitRoot, head)` | 生产函数直调（传递 `*plumbing.Reference`） | 耦合点是生产签名而非 go-git 本身 |

### 1.5 app_git_cache_test.go（仅测试代码）

4 处同形态消费（PlainOpen + Head + commitHistoryCacheKey），用于注入陈旧/伪造缓存：

| file:line | API | 用途 |
|---|---|---|
| app_git_cache_test.go:64-72 | `git.PlainOpen` + `repo.Head()` | 链断回退测试：构造缓存键注入陈旧缓存 |
| app_git_cache_test.go:114-122 | 同上 | 缓存命中测试：注入伪造缓存 |
| app_git_cache_test.go:216-224 | 同上 | Invalidate 测试 |
| app_git_cache_test.go:263-271 | 同上 | 内存过滤测试 |

fixture 构造本身全部走 `exec.Command("git", ...)`（:23-29）与 `addCommits`（:35-42），go-git 仅用于「读回 head」。**非深度耦合**。

### 1.6 commit_history_integration_test.go（仅测试代码，`//go:build integration`）

| file:line | API | 用途 | 路线 A 可行性 |
|---|---|---|---|
| commit_history_integration_test.go:112-119 | `git.PlainOpen` + `repo.Head()` | 超限不写缓存断言：经 `commitHistoryCacheKey(gitRoot, head)`（:120）构造期望键判 miss | 可消：同上，随生产签名改造消除 |

fixture 构造已 CLI 化（`testutil.RunGit` :142-143 + `git fast-import` :162），go-git 仅读回 head。

---

## 二、util 层封装能力面（路线 A 的复用基础）

`util/git.go`（533 行）现成能力：

| 能力 | API（util/git.go:line） | 对路线 A 的意义 |
|---|---|---|
| 通用执行 | `Execute`（:86）/ `ExecuteWithContext`（:95）/ `ExecuteWithCodes`（:103）/ `ExecuteWithCodesWithContext`（:112） | 全部 CLI 替代点的执行基座；超时/取消/锁错误识别（E_GIT_TIMEOUT/E_GIT_LOCKED）统一收口 |
| 仓库定位 | `FindGitRoot`（:226）/ `IsGitRepositoryFast`（:133）/ `IsGitRepository`（:119） | PlainOpen 的定位职责已由 FindGitRoot 承担（6 个消费点全部先 FindGitRoot 再 PlainOpen） |
| HEAD/分支 | `GetBranch`（:142）/ `BranchShowCurrent`（:526） | 取分支名现成；**缺 HEAD SHA 与 detached 判定组合封装**（rev-parse HEAD / symbolic-ref -q） |
| 远程 | `GetRemote`（:147，`git remote -v` 解析） | 单仓取远程现成但走 fork；批量免 fork 场景见 §5.1 缺口 |
| 引用枚举 | **无** for-each-ref 封装 | ComputeBranchSyncInfo 替代需新增（或裸 Execute 拼参） |
| 冲突态检测 | `IsMergeInProgress` 等（:422-435，文件存在性判定） | 与本任务无关，但示范了「零子进程读 .git 形态」的既有先例 |

**封装缺口清单**（路线 A 需新增，均小）：
1. `ResolveHead(gitRoot) (sha, branchName string, detached, unborn bool, err error)` 型 helper——覆盖 GetGitRemoteURL / GetCommitHistory / GetRepoStats / ComputeBranchSyncInfo 四处消费 + 测试 6 处（收敛一处，替代全部 PlainOpen+Head）。
2. for-each-ref 封装（或 ComputeBranchSyncInfo 内裸 Execute 拼 `for-each-ref --format=...`）。
3. HasRemotesBatch 的免 fork 批量远程检测（见 §5.1 三方案）。

---

## 三、路线 A 汇总（全量去依赖）

### 3.1 需改写的消费点清单

| # | 消费点 | 改写方式 | 工作量 | 风险 |
|---|---|---|---|---|
| 1 | app_git.go `getCommitFiles`/`getTreeFiles`（object 消费全部） | **直接删除死代码** | 小 | 无（无调用者，编译期可证） |
| 2 | app_git.go `GetGitRemoteURL` | CLI：`git config --get remote.origin.url` + ResolveHead helper；保持「无 origin 返空信息」降级语义（:130-136）与 detached 展示语义 | 小 | 错误文案需保持「无法打开 Git 仓库」「无法获取 HEAD 引用」（前端文案面） |
| 3 | app_git.go `GetCommitHistory` + `GetRepoStats` + `commitHistoryCacheKey` | ResolveHead helper 替换 PlainOpen+Head；缓存键签名改 string 参数（headSHA/branchName/detached），内部拼接逻辑不变（分支 `gitRoot + "\|" + ref 全名`、detached `gitRoot + "\|HEAD\|" + sha`，:523-528） | 小-中 | 空仓库语义：现状 PlainOpen 成功 + Head 失败报「无法获取 HEAD 引用」（:197-199/:253-255），CLI 侧 rev-parse HEAD 失败即同语义，须保文案 |
| 4 | service/dashboard.go `ComputeBranchSyncInfo` | `git for-each-ref` 单 fork 替代 References 迭代；unborn/detached 降级分支（:305-324）按 symbolic-ref/rev-parse 组合精确平移 | 小-中 | ref 匹配逻辑（localRefName/remoteRefName 精确相等，:360-374）平移为输出行比对，语义等价 |
| 5 | service/git.go `HasRemotesBatch` | 三方案决策（§5.1） | 中 | NF1 性能约束（§5.1） |
| 6 | 测试 6 处（stats/cache/integration） | 随 #3 签名改造，改用 ResolveHead 型 helper 或 testutil 新增 `HeadInfo`（对齐 testutil.InitTempRepo 既有范式，禁各测试重复定义） | 小 | 无 |

### 3.2 收益与风险总评

- **收益**：go.mod 移除 go-git v5.19.2 及间接依赖链（go-billy v5.9.1、gcfg 等，go.mod:32-33 仅是直接间接项，下游还有更深层传递依赖）；彻底免除 v6 迁移与后续版本跟踪；消费面 6 文件 import 全部消失。
- **语义风险**：低。全部活跃消费是「HEAD SHA / 分支名 / ref 枚举 / remote 列表」四类 CLI 成熟能力；无对象库读取、无 worktree 操作、无传输协议（fetch/push 走库的能力本仓从未使用，Pull/Push/Clone 已是 CLI）。
- **错误映射差异**：go-git 错误（如 `ErrRepositoryNotExists`）从未被程序化识别（无 errors.Is 消费），文案级差异可控。
- **总体工作量**：**小-中**（估 0.5-1 天 + 回归测试；HasRemotesBatch 若选方案 B2 需额外边界测试）。

---

## 四、路线 B 汇总（留库升 v6）

### 4.1 受影响消费点

全部 6 文件、上文明细表逐行（活跃消费 5 处 + 测试 6 处；死代码无论何路线都应删除，删除后 object 包核验面归零）。

### 4.2 待对 breaking 清单核验项（外部 breaking 清单由另一研究线产出，此处仅标注核验锚点）

| 核验锚点 | 涉及消费点 |
|---|---|
| `git.PlainOpen` 签名与行为 | 6 处运行时 + 测试 5 处 |
| `Repository.Head()` / `Remote()` / `Remotes()` / `References()` 迭代器 | app_git.go / service/git.go / dashboard.go |
| `plumbing.Reference`（`Name()`/`Hash()`/`Type()`/`IsBranch()`）与 `ReferenceName` 形态 | app_git.go:523-528、dashboard.go:349-374 |
| `plumbing.HashReference` 枚举常量 | dashboard.go:357（全仓唯一常量引用） |
| module path `go-git/v5` → `go-git/v6`（机械替换，必然项） | 6 文件 import + go.mod |

### 4.3 工作量

- 机械部分（import 路径 + go.mod）：小。
- API 适配：取决于 breaking 清单，涉及 plumbing.Reference 形态的消费点（缓存键签名、refs 枚举）是重点核验对象。
- 回归：桌面全量 go test + E2E 提交历史/看板/仓库筛选器链路。
- **总体工作量**：**小-中**；风险在 breaking 清单未定，且 v6 后仍长期承担库的间接依赖链与后续升级。

---

## 五、专项评估

### 5.1 service/git.go:553 remote 读取的性能决策（NF1）

**决策原文**：HasRemotesBatch 用 go-git（读 .git/config）实现，「不 fork git 子进程，规避逐个 git remote -v 的子进程开销」（service/git.go:553-554），服务仓库筛选器列表，NF1 = 「扫描 100 个仓库 < 3 秒」（PRD 07-19-repo-filter-dialog prd.md:124）。调用方 app_repometa.go:113-114 明确注释「go-git 并发，不 fork 子进程，保证 NF1」。

**性能账**（同任务研究文档 git-scan-optimization.md 的实测口径：fork ≈ 50ms、os.Stat ≈ 0.1ms，Windows 桌面估算）：

| 替代方案 | 100 仓库耗时估算 | NF1 判定 | 说明 |
|---|---|---|---|
| 现状 go-git（并发 8） | ~0.1-0.5s（纯文件 IO + INI 解析，无 fork） | 余量大 | 基准 |
| A1：逐仓 fork `git remote`（并发 8） | 100 × 50ms / 8 ≈ 0.6-1s | 勉强达标但余量小 | Windows fork 开销波动风险；并发提到 16 可压至 ~0.3s，但 fork 风暴与台机负载不可控 |
| A2：纯 Go 解析 .git/config（零子进程） | 与 go-git 同量级 | 余量大 | 需自行处理：`.git` 为文件时解析 `gitdir:` 重定向定位真实 config（worktree/submodule，FindGitRoot 已有同形态先例）+ remote 段判定语义 |
| B：留 go-git 仅此一处 | 现状 | 余量大 | 承担整条依赖链只为一个批量判定 |

**性能影响结论**：NF1 不构成「必须留库」的硬阻塞——A2 方案性能与 go-git 等价（都是读 config 文件），且仓库筛选器列表的仓库路径来自 `ScanGitRepos`（.git 存在性预筛），`.git` 形态处理逻辑与 `IsGitRepositoryFast`/`FindGitRoot` 同源可复用。真正代价是**维护一个 config 解析的边界面**（go-git 用 gcfg 帮你做了 INI 解析与 remote 段结构化；手写需测试 remote 段无 url、多 remote、config 含 includes 等边界的语义一致性——go-git `repo.Remotes()` 对无 url remote 段的计数行为待核验后对齐）。

**替代方案建议排序**（供主 agent 决策，非本 agent 职责内的实现推荐）：A2 > A1 > B；若不愿引入 config 解析边界面，则「仅此一处留库升 v6」的混合路线是最低风险解。

### 5.2 commit_history 测试消费性质确认

结论：**fixture 便利性消费，非深度耦合**。证据：
- 采集层已 CLI 化（commit_history_cli.go，`streamCommitLog` + `git log --name-only`），测试文件对 go-git 的唯一用法是 `PlainOpen + Head()` 读回 fixture 仓库的 HEAD 以构造缓存键（commitHistoryCacheKey 直调）。
- fixture 构造本身全部 CLI（app_git_cache_test.go:23-29 逐条 `git init/commit`；integration_test.go:142 `testutil.RunGit` + :162 `git fast-import`）。
- 替代路径现成：`testutil.InitTempRepo` 既有范式 + CLI `rev-parse HEAD` / `symbolic-ref --short -q HEAD`，或随生产 `commitHistoryCacheKey` 签名改造（string 化）后测试 go-git import 自然消失。
- 无任何 go-git 对象构造、worktree 操作、object 遍历式断言。

### 5.3 app_git.go 的 plumbing/object 消费实质

- **plumbing 活跃消费 = 三件事**：HEAD SHA（缓存键/增量判定）、分支 ref 全名（缓存键隔离分支切换）、detached 判定（缓存键用 SHA 隔离 + GetGitRemoteURL 的 IsDetached 字段）。全部分布在 GetGitRemoteURL / GetCommitHistory / GetRepoStats / commitHistoryCacheKey 四个函数，均为「元数据读取」而非「对象库读取」。
- **object 消费 = 全部死代码**：getCommitFiles（app_git.go:570）与 getTreeFiles（:606）在全仓（含测试）无调用者；其语义已由 commit_history_cli.go 用 CLI 平移（该文件 :31/:61/:177/:252 注释逐条对齐「Parent(0) diff」「100 上限」语义）。删除即可移除 object import，这也是任务问句「plumbing/object 消费具体做什么」的答案：**object 已无活跃消费；plumbing 活跃消费仅为引用元数据**。

---

## 六、路线对比与推荐结论

| 维度 | 路线 A（全量去依赖） | 路线 B（留库升 v6） | 混合路线 |
|---|---|---|---|
| 改写面 | 5 活跃点（1 处为删死代码）+ 测试 6 处 | 6 文件 import + API 适配 | A 的 4 点 + B 仅 HasRemotesBatch |
| 依赖收益 | go-git 全链移除 | 保留全链，版本升级 | 保留最小面 |
| 性能风险 | 仅 HasRemotesBatch（NF1）有决策点 | 无 | 同 A |
| 语义风险 | 低（CLI 等价物成熟；unborn/detached/无 origin 三类降级语义可精确平移，须保错误文案） | 取决于 breaking 清单（待另一研究线） | 最低 |
| 后续维护 | 不再跟踪 go-git 版本 | 持续跟踪 v6.x | 持续跟踪（面最小） |
| 工作量 | 小-中 | 小-中（breaking 未定） | 小 |

**推荐结论**：**混合路线（以 A 为主）更优**。判据：
1. 活跃消费 5 处中 4 处是「低频单仓 + CLI 等价物成熟 + 错误处理黑盒（无 sentinel error 消费）」，去依赖是低风险机械改造，且共享一个 ResolveHead 型 helper 即可收敛。
2. object 死代码删除无争议，先行独立可做。
3. 唯一决策锚是 HasRemotesBatch（NF1）：若接受手写 .git/config 解析（性能等价、需补边界测试）则可全量去依赖、彻底免除 v6 升级；若不愿承担该边界面，则保留此一处升 v6，其余 4 点照 A 消除——此时 go-git 消费面从 6 文件缩至 1 文件 1 函数，breaking 核验面缩至 `PlainOpen + Remotes()` 两个 API。
4. 若主任务动机本就是「升级 v6」，建议先等外部 breaking 清单产出后再对 §4.2 锚点核验；在该清单缺位期间，路线 A 的第 1-4、6 点改造与 breaking 清单无关，可先行。

## Caveats / Not Found

- go-git v6 的具体 breaking 变更未在本报告展开（任务约定由另一研究线产出）；§4.2 仅列核验锚点。
- go-git v5 `repo.Remotes()` 对「config 存在 remote 段但无 url」时的计数行为未实测（影响 A2 方案语义对齐），标注待核验。
- fork ≈ 50ms 为 07-19 研究文档的 Windows 经验估算（其文末自述未经实际压测），NF1 达标估算沿用了该口径；若需精确决策建议实现后实测。
- 本仓从未消费 go-git 的传输能力（fetch/push/clone 全是 CLI），此为路线 A 语义风险低的判断依据之一。
