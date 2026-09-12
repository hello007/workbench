# 提交历史缓存与增量更新

## Goal

落地路线图 `docs/路线图.md:91-94`「Git 操作优化 → 提交历史缓存」三项推荐项：**本地缓存提交历史 / 增量更新机制 / 缓存过期策略**。

现状：`GetCommitHistory`（`app_git.go:139`）每次请求（含翻页、防抖过滤）都重走 go-git `repo.Log` 迭代 + `getCommitFiles`（`app_git.go:239`）逐提交算 tree patch。大仓 + 高频翻页 + 300ms 防抖过滤场景下，`getCommitFiles` 的 tree patch 计算重复开销显著。本次引入纯内存缓存复用 `filetree_cache` 既定模式（mtime+TTL+手动刷新+Mutex+深拷贝），过滤与分页下沉至缓存内内存执行，增量更新覆盖 commit/pull 后 HEAD 前移场景。

## What I already know

### 现有后端基础（app_git.go:139 GetCommitHistory）
- 签名：`func (a *App) GetCommitHistory(path string, limit, offset int, filter model.CommitFilter) ([]model.Commit, error)`
- 引擎：go-git v5.18.0 `repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})`，Since/Until/FilePath 原生下推，Author/Keyword 迭代内手动子串匹配
- 分页：过滤后偏移——跳过不匹配 → 跳过 offset 个匹配 → 收集 limit 个；无总数返回，前端靠 `newCommits.length === pageSize` 判 hasMore
- `getCommitFiles`（app_git.go:239）：逐提交 `currentTree.Patch(parentTree)` 算变更文件，root commit 走 `getTreeFiles`（上限 100）；**此为重复计算主要开销点**
- 返回 `[]model.Commit`（SHA/ShortSHA/Message/Author/Email/Timestamp/DateTime/Files）

### 现有前端基础（frontend/src/components/CommitHistory.vue）
- `loadCommits`（:269）调 `GetCommitHistory(props.repoPath, pageSize, offset, buildFilter())`，PAGE_SIZE=20
- `handleRefresh`（:305）reset 重载；`applyFilter`（:255）300ms 防抖 reset
- 过滤条件 author/dateRange/filePath + searchKeyword（keyword），`buildFilter`（:235）组装 CommitFilter
- watch repoPath（:377）切仓重载

### 既定缓存模式（service/filetree_cache.go，本任务复用同套）
- 纯内存不落盘（桌面单用户场景，路线图:86 已决策纯内存收益满足）
- 三层失效：mtime 差量 + TTL 5min（`fileTreeCacheTTL`，:12）+ 手动刷新（clearPath/clearAll）
- 并发安全：`sync.Mutex` 在 get/set/clear 整次操作持有，串行化 map 读写
- 数据隔离：set 存深拷贝、get 返深拷贝，调用方修改不污染缓存
- 挂载：cache 挂 `FileTreeService`（service/filetree.go:19 treeCache 字段），App 方法薄包装调 service（app_filetree.go:31 InvalidateFileTreeCache → fileTreeSvc.InvalidateCache）

### 跨层契约（docs/spec/cross-layer-contracts.md）
- App 方法签名变更 → 同步 `frontend/wailsjs/go/main/App.js` + `App.d.ts`（整目录 gitignore，`wails generate module` 重生成）
- model struct 字段变更 → 同步 `frontend/wailsjs/go/models.ts`
- 本任务新增 App 桥接方法（InvalidateCommitHistoryCache 等）须同步绑定；无新 model struct（CommitFilter 已有），models.ts 无新增

### 架构现状关键点
- `GetCommitHistory` 是**例外**：App 方法直调 go-git，不经 `gitSvc`（其余 Commit/Push/Merge/GetDiff 等均走 `a.gitSvc`）
- App struct（app.go:13）持各 `*service.XxxService`，startup（app.go:34）注入

## Requirements

- 本地内存缓存提交历史（含 Files 字段，避免 getCommitFiles 重复计算）
- 增量更新：commit/pull 后 HEAD 前移时，仅追加新提交段（prepend），不全量重扫
- 缓存过期策略：TTL + 手动刷新 + SHA 链断裂回退全量重扫
- 过滤与分页下沉缓存内内存执行（Author/Keyword/FilePath/Since/Until 全内存，不再每页触 go-git）
- 缓存键按仓库 + HEAD ref 隔离（分支切换/detached 不串数据）
- 并发安全 + 深拷贝隔离（同 filetree 模式）
- 大仓防爆内存：单仓缓存条数上限，超限走原 go-git 路径不缓存
- 无过滤条件时行为与原 GetCommitHistory 完全一致（向后兼容）
- 跨层绑定同步（新增失效桥接方法 App.js / App.d.ts）

## Acceptance Criteria

- [ ] 首次加载后，翻页/过滤复用缓存不再触 go-git Log 迭代（getCommitFiles 不重算）
- [ ] commit/pull 后 HEAD 前移：增量 prepend 新提交段，旧段沿用，不全量重扫
- [ ] rebase/amend 改写历史致 SHA 链断裂：检测到新 HEAD 不在缓存链 → 回退全量重扫
- [ ] 分支切换/detached HEAD：按 ref 隔离，不串历史
- [ ] TTL 过期或手动刷新后缓存失效，下次请求重扫
- [ ] 过滤（author/keyword/since/until/filePath）在缓存全量上内存执行，翻页仅遍历过滤后结果集
- [ ] 单仓缓存超上限阈值走原 go-git 路径，不缓存不报错
- [ ] 无过滤时与原 GetCommitHistory 行为一致（向后兼容）
- [ ] 并发 get/set/clear 无竞态（go test -race）
- [ ] 跨层绑定三处同步，`npm run build` 绿
- [ ] 后端单测覆盖：hit/miss/TTL/增量 prepend/SHA 链断回退/上限/并发/深拷贝/各过滤维度
- [ ] 前端单测覆盖 handleRefresh 调失效桥接 + 缓存命中场景渲染

## Definition of Done

- 后端测试覆盖率达标（service ≥76% 基线，新增 commit_history_cache.go 纳入）
- 前端测试覆盖率 ≥70%
- `npm run build` + `go test ./...` + `cd frontend && npm test` 三绿
- `go test -race ./service/...` 无竞态
- README / 路线图勾选更新（路线图:91-94 三项）
- 跨层契约无回归

## Technical Approach

### 方案 A：App 持有纯内存缓存 + 增量 prepend + 过滤内存化（已选定）

**缓存挂载层**：App struct 加 `commitHistoryCache *service.CommitHistoryCache` 字段，`GetCommitHistory` 仍为 App 方法，内部调 cache。不把 GetCommitHistory 下沉 GitService（避免大范围重构）。失效靠 TTL + 增量 + 手动刷新桥接，与 filetree 一致。

**新增 service/commit_history_cache.go**（仿 filetree_cache.go）：
```go
const commitHistoryCacheTTL = 5 * time.Minute
const commitHistoryMaxEntries = 5000 // 单仓缓存上限，超限走原路径不缓存

type CommitHistoryCache struct {
    mu      sync.Mutex
    entries map[string]commitHistoryCacheEntry // key = gitRoot + "|" + headRef
}

type commitHistoryCacheEntry struct {
    headSHA  string        // 落缓存时 HEAD SHA，增量判定基准
    commits  []model.Commit // 全量提交快照（含 Files），深拷贝隔离
    cachedAt time.Time
}
```

**缓存键**：`gitRoot + "|" + headRef`，`headRef = head.Name().String()`（`refs/heads/<branch>` 或 detached 的 `HEAD`+sha）。分支切换/detached → 不同键。

**增量更新逻辑**（cache.get 内）：
1. 取当前 HEAD SHA + ref
2. miss → 全量扫（go-git Log 迭代收集，带上限），写 cache，返 misses
3. hit 且 TTL 内：
   - 当前 HEAD SHA == 缓存 headSHA → 命中，返缓存深拷贝
   - 不同 → 从新 HEAD 迭代 go-git Log，逐提交收集直到遇到缓存 commits 中已有 SHA（交集），把新段 prepend 到缓存；迭代超阈值（如 500）仍无交集 → 视为 SHA 链断裂（rebase/amend 改写），回退全量重扫
4. TTL 过期 → 驱逐条目，miss 走全量

**过滤内存化**（GetCommitHistory 改造）：
- cache.get 返全量 `[]model.Commit` 后，Author/Keyword/FilePath/Since/Until 全部内存过滤（Since/Until 转 timestamp 数值比较，不再下推 go-git LogOptions）
- 过滤后偏移分页（跳过 offset 个匹配 → 收集 limit 个），逻辑同现状但数据源换缓存
- 无缓存（超上限/miss 未写）→ 走原 go-git Log 路径（含 Since/Until/FilePath 下推 + Author/Keyword 迭代），向后兼容

**失效桥接**（新增 App 方法，仿 app_filetree.go:31）：
- `InvalidateCommitHistoryCache(path string)` — 清单仓
- `ClearAllCommitHistoryCache()` — 清全部（预留，前端工具栏全刷）
- 前端 `handleRefresh` 前置调 `InvalidateCommitHistoryCache(props.repoPath)` 再 loadCommits

**并发与隔离**：sync.Mutex 整次操作持有；set 存深拷贝、get 返深拷贝（`model.Commit` 含 `Files []string` 切片，深拷贝须复制切片）。

### 跨层同步清单
- `service/commit_history_cache.go` 新建（+ `_test.go`）
- `app.go` App struct 加 `commitHistoryCache` 字段，startup 注入 `NewCommitHistoryCache()`
- `app_git.go` GetCommitHistory 改造：先 cache.get → 命中内存过滤分页 / 未命中走原 go-git 路径后 cache.set
- `app_git.go` 加 InvalidateCommitHistoryCache / ClearAllCommitHistoryCache 桥接
- `wails generate module` 重生成 App.js / App.d.ts（无新 model struct，models.ts 无新增）
- 前端 CommitHistory.vue handleRefresh 前置调失效桥接

## Decision (ADR-lite)

**Context**：需补齐提交历史缓存，覆盖翻页/过滤重复 go-git 迭代 + getCommitFiles 重算开销。GetCommitHistory 当前是 App 方法直调 go-git 的例外（其余 Git 操作走 gitSvc）。

**Decision**：方案 A——App struct 持有 `CommitHistoryCache`（纯内存），不把 GetCommitHistory 下沉 GitService。增量更新靠 HEAD SHA 对比（相同=命中 / 不同=prepend 新段 / SHA 链断=全量回退）。过滤与分页下沉缓存内内存执行。失效靠 TTL 5min + 手动刷新桥接 + 增量，不主动在 Commit/Push 后失效（增量已覆盖 commit 后 HEAD 前移）。

**Consequences**：
- 优点：复用 filetree 既定模式（模式一致、风险低）、范围小不重构 service、增量覆盖最常见 commit/pull 场景、过滤翻页零 go-git 调用、Files 字段缓存免重算
- 代价：单仓内存占用（上限 5000 条兜底）、Commit/Push 后不主动失效靠增量+TTL（amend/rebase 靠 SHA 链断检测回退全量，正确但有延迟至下次请求）
- 风险：增量 SHA 链断裂判定阈值（500）需平衡——过小误判全量、过大遍历成本；MVP 取 500，后续按实际调

## Out of Scope

- 持久化缓存到本地文件（路线图:86 已决策纯内存，同 filetree）
- Commit/Push/Merge 后主动失效（靠增量 + TTL + 手动刷新，后续可加）
- 提交总数返回（保持 hasMore 页满判定，同上一任务 Out of Scope）
- LRU 淘汰（单仓上限直接走原路径，不设 LRU）
- 跨仓库缓存共享 / 预加载
- 缓存命中率统计埋点

## Technical Notes

- 引擎现状：GetCommitHistory 用 go-git（本任务沿用）；service/git.go 中 Commit/Push/GetDiff 等用 git CLI——两种引擎并存
- filetree_cache.go 是直接参照模板（结构、TTL、Mutex、深拷贝、注释风格均复用）
- 增量判定基准：缓存 headSHA vs 当前 HEAD SHA；HEAD 前移=prepend，SHA 链断=全量
- 深拷贝注意：`model.Commit.Files` 是 `[]string`，深拷贝须 `make + copy`，不能仅复制 struct（切片共享底层数组）
- 路线图:91-94 三项对应：本地缓存（entries map）/ 增量更新（HEAD SHA prepend）/ 过期策略（TTL + 手动 + SHA 链断回退）
- 上一任务（09-12-commit-history-server-search-and-filter）prd Decision 显式标注"后续可加缓存"作为 follow-up，本任务即该 follow-up
