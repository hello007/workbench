# Research: FindGitRoot 修复方案与 go-git submodule API 能力边界

- **Query**: FindGitRoot 对 submodule 漏判的修复方案，以及 go-git submodule API 能力边界
- **Scope**: 内部（util/git.go、service/git.go、app_git.go、go-git v5.18.0 源码）+ 外部对比
- **Date**: 2026-09-12

---

## 第一部分：FindGitRoot bug 修复

### 1.1 当前 FindGitRoot 实现 + bug 确认

实现位于 `util/git.go:161-177`：

```go
func FindGitRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil { return "", err }
	for {
		gitDir := filepath.Join(abs, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {  // ← 问题点
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("not a git repository: %s", path)
		}
		abs = parent
	}
}
```

**bug 机理**：submodule（与 worktree）的 `.git` 是**文件**，内容形如 `gitdir: /path/to/.git/modules/<name>`。`info.IsDir()` 对该文件返回 `false` → 循环跳过 submodule 根 → 继续向上走到 superproject，superproject 的 `.git` 是目录 → 返回 **superproject 根**。

**后果**：对 submodule 路径调用 `FindGitRoot` 返回的是其父仓库（superproject）根，而非 submodule 根。submodule 操作依赖正确根定位，本任务须修或绕过。

### 1.2 IsGitRepositoryFast 的正确做法（已修复的参照）

实现位于 `util/git.go:97-103`：

```go
func IsGitRepositoryFast(dir string) bool {
	if dir == "" { return false }
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil   // 只判无错，不要求 IsDir
}
```

- 已覆盖 submodule/worktree 的 `.git` 文件场景（注释 `util/git.go:87-96` 自述）
- 有完整测试 `util/git_fast_test.go`，其中 `TestIsGitRepositoryFast_GitFile`（行 23-32）模拟 `gitdir: /some/path/...` 文件断言识别
- 注释明确指出 FindGitRoot 的 `IsDir()` 漏判是「已存在的潜在 bug，本函数避免重蹈覆辙」

### 1.3 修复方案评估

| 方案 | 做法 | 评估 |
|---|---|---|
| **(A)** FindGitRoot 改用 os.Stat 不要求 IsDir | 删除 `&& info.IsDir()`，即 `if err == nil { return abs, nil }` | 最小改动，与 IsGitRepositoryFast 语义一致。推荐。 |
| **(B)** 新增 FindGitRootSubmoduleSafe 不动原函数 | 并行新函数，调用方主动选择 | 现有 ~24 处调用方不会自动受益，须逐处替换；语义割裂；不推荐。 |
| **(C)** FindGitRoot 内部复用 IsGitRepositoryFast | 循环体改为 `if IsGitRepositoryFast(abs) { return abs, nil }` | 逻辑等价 (A)，但复用已测函数、去重判定逻辑。推荐。 |

**推荐 (A) 或 (C)**，二者行为等价。(C) 复用已测函数减少重复；(A) 改动更直观。落地建议取 (C)，理由：IsGitRepositoryFast 已有完整测试覆盖（含 gitdir 文件场景），FindGitRoot 复用它即继承该覆盖语义。

### 1.4 调用方影响分析（全量 caller 清单）

通过 Grep 全量扫描 `FindGitRoot` 调用点（codegraph 工具未在当前 agent 工具集中暴露，改用 Grep 覆盖）：

#### app_git.go（4 处，FindGitRoot + git.PlainOpen）

| 行 | 方法 | 后续用法 |
|---|---|---|
| 92 | `GetGitRemoteURL` | `git.PlainOpen(gitRoot)` → 读 origin remote + HEAD |
| 159 | `GetCommitHistory` | `git.PlainOpen(gitRoot)` → repo.Log 提交历史 |
| 214 | `GetRepoStats` | `git.PlainOpen(gitRoot)` → 统计聚合 |
| 495 | `InvalidateCommitHistoryCache` | `gitRoot` 作缓存键 `ClearByGitRoot` |

**关键事实**：go-git `PlainOpen` 已支持 `.git` gitfile 解析。源码 `repository.go:388-398`：

```go
if fi.IsDir() {
    dot, err = fs.Chroot(GitDirName)        // .git 是目录
    return dot, fs, err
}
dot, err = dotGitFileToOSFilesystem(path, fs)  // .git 是文件 → 解析 gitdir: 指针
```

`dotGitFileToOSFilesystem`（`repository.go:401-423`）读取 `gitdir: ` 前缀并解析绝对/相对路径。

**影响结论**：修复后 `PlainOpen(submoduleRoot)` 会正确打开 submodule 自身仓库（解析 gitdir 文件），而非 superproject。这不仅是安全的，反而**修复了一处潜在错误数据 bug**——当前对 submodule 路径请求提交历史/统计/远程信息会返回 superproject 的数据。`InvalidateCommitHistoryCache` 的缓存键也随之正确切换到 submodule 根，与 `GetCommitHistory` 的键（`commitHistoryCacheKey` 用 `gitRoot+head`）保持一致。

#### service/git.go（约 20 处，FindGitRoot + s.gitCmd.Execute）

| 行 | 方法 | 行 | 方法 |
|---|---|---|---|
| 451 | GetLocalChanges | 984 | GetTag |
| 512 | StageFiles | 1000 | ListRemotes |
| 591 | UnstageFiles | 1046 | AddRemote |
| 618 | UnstageFiles | 1068 | RemoveRemote |
| 648 | HasUpstream | 1087 | RenameRemote |
| 666 | GetDiff | 1121 | SetRemoteURL |
| 718 | GetFileHistory | 1146 | SwitchBranch |
| 751 | GetFileHistory（SHA） | 1168 | CheckoutRemote |
| 873 | ListTags | 1197 | RenameBranch |
| 934 | CreateTag | 1221 | StageFiles（批量） |
| 962 | DeleteTag | 1244 | UnstageFiles（批量） |

**影响结论**：修复后 `s.gitCmd.Execute(gitRoot, args...)` 在 submodule 根执行 git CLI，上下文正确切换到 submodule。安全且更正确。

#### precheckMutation（service/git.go:1265-1288）

```go
if !s.gitCmd.IsGitRepository(repoPath) { ... }   // fork git rev-parse，已正确处理 submodule
gitRoot, err := util.FindGitRoot(repoPath)        // 修复后与上一行判定一致
```

`IsGitRepository`（`util/git.go:80-85`）走 `git rev-parse --git-dir`，git CLI 自身解析 gitdir 文件，已正确处理 submodule。修复后 FindGitRoot 与之一致，消除两者对 submodule 路径判定不一致的隐患。

#### 测试文件（不受影响）

- `app_git_cache_test.go`（4 处）：真实 `git init` 仓库，`.git` 为目录，修复不改变行为
- `util/git_test.go`（3 处 `TestFindGitRoot_RealRepo/Subdir/NonRepo`）：同样 `.git` 为目录，不受影响；**目前无 submodule 场景测试，修复时须补**

### 1.5 是否纳入本 submodule 任务

- PRD Open Question #3 明确待决；Acceptance Criteria 已含「FindGitRoot 对 submodule 路径不再漏判」
- **建议：纳入本任务**。理由：
  1. submodule 操作依赖正确根定位，修复是 submodule 功能可用的前置
  2. 改动极小（删一个 `IsDir` 谓词，或复用 `IsGitRepositoryFast`）+ 补一个 gitdir 文件场景测试
  3. AC 已列入
  4. 不纳入则 submodule init/update/status 在 submodule 路径下仍定位到 superproject 根，功能不可用
- **备注**：修复收益面比 submodule 更广（同修 worktree 路径下 commit 历史/统计的错误数据 bug），但内聚于「.git 根定位正确性」，适合随 submodule 任务一并落地。若团队要求独立 bugfix commit，可在本任务内拆为独立 commit，不必拆任务。

---

## 第二部分：go-git submodule API 能力边界

### 2.1 go-git submodule 能力（源码：`submodule.go`、`worktree.go:867`、`options.go:354`）

go-git 的 submodule API 位于根包 `git`（非 `worktree/` 子目录），`submodule.go` 文件。

| API | 位置 | 说明 |
|---|---|---|
| `Worktree.Submodules() (Submodules, error)` | `worktree.go:867` | 读 `.gitmodules` + `.git/config`，返回 submodule 列表 |
| `Submodule.Config() *config.Submodule` | `submodule.go:33` | name/path/url/branch 配置 |
| `Submodule.Init() error` | `submodule.go:39` | 写入 `.git/config`；重复返 `ErrSubmoduleAlreadyInitialized` |
| `Submodule.Update(o *SubmoduleUpdateOptions) error` | `submodule.go:167` | 条件 init + fetch + checkout 到 index 期望 hash |
| `Submodule.Status() (*SubmoduleStatus, error)` | `submodule.go:57` | 返回 `SubmoduleStatus{Path, Current, Expected, Branch}` |
| `Submodule.Repository() (*Repository, error)` | `submodule.go:102` | 返回 submodule 自身 `*Repository`（须已 init） |
| `Submodules.Init()/Update(o)/Status()` | `submodule.go:288/299/319` | 批量遍历列表 |
| `SubmodulesStatus.String()` | `submodule.go:348` | 近似 `git submodule status` 文本 |

`SubmoduleUpdateOptions`（`options.go:354-369`）字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `Init` | `bool` | Update 时若未 init 则先 init |
| `NoFetch` | `bool` | 不从远端 fetch |
| `RecurseSubmodules` | `SubmoduleRescursivity` | 递归深度计数（非布尔） |
| `Auth` | `transport.AuthMethod` | 认证 |
| `Depth` | `int` | fetch 深度 |

`SubmoduleStatus.IsClean()`（`submodule.go:366`）：`return s.Current == s.Expected`。

### 2.2 go-git 做不到 / 不可靠（vs os/exec `git submodule`）

| 能力 | os/exec `git submodule` | go-git | 说明 |
|---|---|---|---|
| `deinit`（注销） | ✅ `git submodule deinit` | ❌ 无方法 | 缺失 |
| `add`（新增） | ✅ `git submodule add <url> <path>` | ❌ 无方法 | 缺失 |
| status 三态（未初始化/已初始化/dirty） | ✅ `git submodule status` + 工作区 dirty 检测 | ⚠️ 浅层 | go-git `IsClean()` 仅比 Current HEAD vs Expected index hash，**检测不到 submodule 工作区内未提交改动** |
| `--merge`/`--rebase` 更新模式 | ✅ | ❌ | Update 固定 fetch+checkout 到期望 hash（detached） |
| `--recursive` | ✅ 布尔 flag | ⚠️ 深度计数 | `RecurseSubmodules` 递减，`NoRecurseSubmodules` 哨兵禁用；语义非布尔 |
| status 输出字节级等价 | ✅ | ❌ | `String()` 注释自称等价，但省略 `git describe` 输出，仅用短 SHA（`submodule.go:390`） |
| 认证 | 系统 git 凭证 | 须传 `Auth` | 引入 go-git 须单独处理认证 |

**核心差距**：go-git status 无法识别「submodule 工作区 dirty」（未提交改动），而 PRD AC 要求「未初始化/已初始化/dirty 三态」。go-git 只能区分「未初始化 / HEAD 不匹配（+）」，无法识别 working-tree-dirty。

### 2.3 结论：WorkBench 应坚持 os/exec

**建议：沿用 os/exec `git submodule` 子命令**（与 tag/remote/merge 一致），不引入 go-git submodule API。理由：

1. **一致性**：`service/git.go` 所有变更/列表操作走 `s.gitCmd.Execute(gitRoot, args...)`（os/exec），go-git 仅用于只读 `Log`（`app_git.go`）+ 批量远程检测 `HasRemotesBatch`（`service/git.go:416`）。submodule 操作是变更密集型，os/exec 契合项目惯例
2. **能力缺失**：go-git 缺 `deinit`/`add`，MVP+ 扩展时受限
3. **AC 不可达**：go-git status 满足不了「dirty 三态」AC，须解析 `git submodule status` 输出
4. **解析范式一致**：`git submodule status --porcelain` 输出与现有 `for-each-ref`（tag，`service/git.go:879`）、`remote -v`（remote）的 porcelain 解析范式一致
5. **认证零成本**：os/exec 复用系统 git 凭证，go-git 须单独传 `Auth`

**唯一可考虑 go-git 的窄场景**：不 fork 读 `.gitmodules` 列出 submodule 配置——但一次 `git submodule status` 即覆盖且信息更全，收益边际，不推荐。

---

## Caveats / Not Found

- go-git 源码版本 v5.18.0（`go.mod` 锁定），结论基于该版本源码
- FindGitRoot 修复后建议补 `util/git_test.go`：`TestFindGitRoot_GitFile`（模拟 submodule `.git` 文件，断言返回该目录而非向上走到父目录）；可参考 `util/git_fast_test.go:23-32` 与 `service/git_scan_test.go:11-34` 的 gitdir 文件构造范式
- 修复对 worktree 路径同样生效（worktree 的 `.git` 也是 gitdir 文件），影响面比 submodule 略广——属正向收益，非风险
- codegraph MCP 工具未在当前 agent 工具集中暴露，caller 清单通过 Grep 全量扫描 `*.go` 得出，已覆盖 app_git.go / service/git.go / 测试文件全部调用点
