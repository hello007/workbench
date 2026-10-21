# go-git 依赖出清契约（2026-10-21）

## 1. Scope / Trigger

- **触发**：go-git v5.19.2 → v6 升级评估任务（security-scan.md §5.4 收尾项）。
- **结论**：v6 正式版未发布（最新 v6.0.0-beta.1，2026-10-04 prerelease，生产不可依赖），改为**整体移除 go-git 依赖**。消费面 5 处活跃点全部改为 CLI / 纯 Go 等价实现，go-git/go-billy/gcfg 依赖链出清。
- **后续约束**：**禁重新引入 go-git**。新增 Git 元数据读取一律走 `util.ResolveHead` / CLI 封装 / 纯 Go config 解析。

## 2. Signatures

| 签名 | 位置 | 用途 |
|---|---|---|
| `ResolveHead(gitRoot string) (HeadInfo, error)` | `util/git.go` | HEAD 引用元数据统一解析入口 |
| `type HeadInfo struct { SHA, RefName, Branch string; Detached, Unborn bool }` | `util/git.go` | SHA=HEAD 提交 SHA（unborn 时空）；RefName=分支 ref 全名 `refs/heads/<branch>`（detached 时 `"HEAD"`）；Branch=短分支名（detached 时空） |
| `commitHistoryCacheKey(gitRoot string, head util.HeadInfo) string` | `app_git.go` | 缓存键构造（原 `*plumbing.Reference` 参数改 string 结构） |
| `resolveGitConfigPath(repoPath string) string` | `service/git.go` | 定位 .git/config（含 .git 文件形态 gitdir 重定向） |
| `repoHasRemote(configPath string) bool` | `service/git.go` | 纯 Go 解析 config 判定是否配置远程 |

## 3. Contracts

### 3.1 ResolveHead 解析链

1. `git symbolic-ref --short -q HEAD`（`ExecuteWithCodes` 接受 exit 1）：成功且非空 = 分支名；exit 1 输出空 = detached。
2. `git rev-parse HEAD` 取 SHA：失败且分支名非空 = **unborn（不报错，`Unborn=true` 返回）**；失败且分支名为空 = 报错（仓库损坏或无 HEAD）。
3. **unborn 不视为错误**，由调用方分流：app 层（GetGitRemoteURL/GetCommitHistory/GetRepoStats）报「无法获取 HEAD 引用」；dashboard 层（ComputeBranchSyncInfo）降级返分支名 + 零值。

### 3.2 HasRemotesBatch 语义对齐口径（对 go-git v5.19.2 实测源码）

| 口径 | go-git v5.19.2 行为 | 本仓解析器对齐点 |
|---|---|---|
| remote 计数 | `Remotes()` 统计 `[remote "name"]` 子段数，**不要求段内配置 url**（`config/config.go unmarshalRemotes`） | 命中任一带子节名的 remote 段头即 `true` |
| 裸 `[remote]` 段 | 非子节，`Subsections` 不含 → 不计 | `header[sp:]` 为空不命中 |
| `.git` 文件形态（worktree/submodule） | `dotGitFileToOSFilesystem` 解析 `gitdir:` 重定向（相对路径以 repoPath 为基准） | `resolveGitConfigPath` 同规则 |
| **commondir** | `PlainOpen` 默认 `EnableDotGitCommonDir=false`，worktree 读 `worktrees/<name>/config`（**不展开 commondir**） | 同样不展开——worktree 无本地 remote 段判 `false`，行为逐字对齐 |
| config include | 本地 config 读取不展开 include | 同样不展开 |
| 段名大小写 | git config 段名大小写不敏感 | `strings.EqualFold` |

### 3.3 ComputeBranchSyncInfo refs 枚举平移

- 原 go-git `References()` 迭代 + `plumbing.HashReference` 过滤 → `git for-each-ref --format=%(objectname) %(refname)` 单次 fork。
- for-each-ref 天然仅输出 hash 型引用且只列 refs/（HEAD 不在输出），`HashReference` 过滤与符号引用跳过不再需要。
- 输出行首空格切分（`%(objectname)` 为固定十六进制无空格），refname 与 `refs/heads/<branch>` / `refs/remotes/<upstream>` 精确相等匹配（原 `ReferenceName` 相等语义平移）。

## 4. Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| FindGitRoot 失败 | 「无法打开 Git 仓库: %w」（三入口与原 go-git PlainOpen 失败文案一致） |
| `git rev-parse HEAD` 失败且非 unborn | 「无法获取 HEAD 引用: %w」 |
| unborn（GetGitRemoteURL/GetCommitHistory/GetRepoStats） | 「无法获取 HEAD 引用: 仓库尚无任何提交」（原 go-git Head() 失败语义） |
| unborn（ComputeBranchSyncInfo） | 降级返分支名 + 零值 refs，不报错 |
| 无 origin 远程（GetGitRemoteURL） | 返空 `GitRemoteInfo`（URL/Branch 空串、IsDetached=false），不报错 |
| detached HEAD（GetGitRemoteURL） | `Branch="HEAD"`、`IsDetached=true`（原 `head.Name().Short()` 行为） |
| for-each-ref 失败 | 「遍历仓库引用失败: %w」 |
| config 不可读/非仓库路径（HasRemotesBatch） | 判 `false`，不报错（原 PlainOpen 失败同语义） |

## 5. Good/Base/Bad Cases

- **Good**：正常仓库 `ResolveHead` 返 `{SHA, "refs/heads/master", "master", false, false}`；缓存键 `gitRoot + "|refs/heads/master"`。
- **Base**：detached → `{SHA, "HEAD", "", true, false}`，缓存键 `gitRoot + "|HEAD|" + SHA`（跳转隔离不串历史）。
- **Bad**：`git init` 后未 commit → `Unborn=true`、SHA 空串；app 层调用方**必须**判 `Unborn` 报错，禁用空 SHA 构造缓存键。

## 6. Tests Required

- `service/git_scan_test.go TestHasRemotesBatch_WorktreeGitFile`：worktree .git 文件形态判 `false`（commondir 口径锚点）+ 裸 `[remote]` 段不计锚点。
- `service/git_scan_test.go TestHasRemotesBatch`：有远程/无远程/非仓库三态。
- 主包缓存测试（app_git_cache_test.go / app_git_stats_test.go / commit_history_integration_test.go）：`util.ResolveHead` 替代 PlainOpen+Head 构造期望键，键格式变更时测试不假阳性。

## 7. Wrong vs Correct

### Wrong

```go
// 重新引入 go-git 只为一处元数据读取
repo, _ := git.PlainOpen(gitRoot)
head, _ := repo.Head()
// 或：手写 config 解析自行「改进」commondir 展开（与原 go-git 行为漂移，仓库筛选器口径变化）
```

### Correct

```go
head, err := util.ResolveHead(gitRoot)
if err != nil { return fmt.Errorf("无法获取 HEAD 引用: %w", err) }
if head.Unborn { return fmt.Errorf("无法获取 HEAD 引用: 仓库尚无任何提交") }
key := commitHistoryCacheKey(gitRoot, head)
```

## 8. 决策记录（为何不升 v6）

| 判据 | 结论 |
|---|---|
| v6 发布状态 | 仅 v6.0.0-beta.1（prerelease），依赖 beta 不可接受 |
| 消费面性质 | 5 活跃点全是元数据读取（HEAD SHA/分支名/ref 枚举/remote 列表），无对象库/worktree/传输协议消费（Pull/Push/Clone 本就是 CLI） |
| sentinel error | 全仓零消费（仅黑盒 `err != nil`），错误映射成本趋零 |
| 性能锚点 | HasRemotesBatch NF1（100 仓库 < 3s）：纯 Go config 解析与 go-git 同为文件 IO，等价达标 |
| 依赖收益 | go-git/go-billy/gcfg 全链移除，免后续 v6 跟踪 |

研究依据：`.trellis/tasks/10-21-go-git-v6-upgrade/research/`（consumer-api-inventory.md + go-git-v6-migration.md）。
