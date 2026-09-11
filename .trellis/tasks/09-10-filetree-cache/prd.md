# 文件树缓存机制

## 目标

在 `FileTreeService` 引入缓存层,复用 `RepoScanCache` 的 mtime 差量 + TTL + 手动刷新范式,提升大型仓库文件树二次展开/切换的加载体感,消除重复 `os.ReadDir` 与 `git remote` 子进程开销。

## 已知事实(Auto-Context 探得)

### 后端 service/filetree.go

- `FileTreeService` 现有字段:`gitRepoCache`、`gitRemoteCache`(均 `sync.Map`,path→bool),**无树结构缓存**。
- `GetChildren(dirPath)`:`os.ReadDir` 单层懒加载,逐目录 `isGitRepoDir`(`os.Stat .git`)+ `hasRemote`(`git remote` 子进程,有缓存)。目录/文件排序后返回 `[]*model.FileTreeNode`。
- `GetTree(dirPath, maxDepth)`:递归 `buildTree`,内部调 `GetChildren`。
- `GetGitInfo`:独立路径,不涉及树缓存。

### 后端 app_filetree.go

- `GetFileTree(path)` → `fileTreeSvc.GetChildren(path)`,err 返回空切片。
- `GetFileTreeRecursive(path, maxDepth)` → `fileTreeSvc.GetTree(path, maxDepth)`,err 返回空切片。
- 签名:`func (a *App) GetFileTree(path string) []*model.FileTreeNode`。

### 参考范式 service/repo_scan_cache.go + git.go:282

- `ScanCacheManager`:进程内单例,`mu sync.Mutex` 串行化「扫描+落盘」,落盘 `data/repo_scan_cache.json`,加载/落盘失败静默降级。
- `RepoScanCache`:`RootPath` + `ScannedAt`(TTL 判定)+ `Entries map[string]CacheEntry`。
- `CacheEntry`:`ModTime` + `IsRepo` + `SubtreeRepos`。
- `scanGitReposCached`:TTL 过期清 entries 强制全扫;`scanDirCached` 命中且 mtime 未变→复用,未命中/mtime 变→实扫+回写。
- `ClearScanCache(rootPath)`:手动刷新按钮绕过缓存。
- `scanCacheTTL = 5 * time.Minute`。

### 前端调用方

- `FileTreePanel.vue`(主路径):el-tree `lazy` + `:load="loadTreeNode"`,每次展开节点调 `GetFileTree(path)` 单层懒加载。
- `AiFunctionRunner.vue` / `AiParamDialog.vue`:浏览弹窗调 `GetFileTree`。
- `GetFileTreeRecursive` 在 `frontend/src` 下**无实际调用方**(wailsjs 死绑定)。
- 前端已有刷新机制:
  - `refreshAll()`:工具栏"刷新"按钮,`refreshCounter++` 改 `treeKey` 强制 el-tree 整体重建。
  - `refreshNode(nodePath)`:右键"刷新"/F5/文件操作后,el-tree `loadData` 重建子节点。
  - 两者均重新触发 `loadTreeNode` → `GetFileTree(path)`;**若后端缓存未失效,前端刷新拿不到新数据**。

### 跨层契约

- 修改 `app.go`/`app_filetree.go` App 方法签名或 `model.FileTreeNode` 字段,须同步 `frontend/wailsjs/`(App.js / App.d.ts / models.ts 三处)——见 `docs/spec/cross-layer-contracts.md`。
- 测试覆盖率门禁:service ≥76% 基线——见 `docs/spec/test-coverage-gate.md`。

## 收敛 MVP(用户已定方向)

- **缓存粒度**:按目录路径 + depth 键,TTL 5min(同 git scan)。
- **失效策略**:手动刷新兜底 + mtime 变化差量检测。
- **签名兼容**:`GetChildren`/`GetTree` 返回类型不变,不破坏现有调用方。

## 已定决策

1. **缓存存储:纯内存**。进程内 `sync.Mutex` + `map`,不落盘。首次展开全量,5min TTL 内命中。不受「写 data JSON 前须关 workbench.exe」约束拖累。`task.json` description 的「持久化」属早期构想,MVP 用纯内存——文件树 `os.ReadDir` 本身廉价,跨会话命中收益不抵落盘成本+约束。
2. **缓存范围:仅 `GetChildren`**。单层懒加载,FileTreePanel 主路径,`path` 为键(depth=1 隐含)。`GetTree` 递归内部调 `GetChildren`,自动受益,无需 `path|depth` 复合键。
3. **手动刷新打通:`RefreshFileTree(path)` + `ClearAllFileTreeCache()` 双 API**。
   - `RefreshFileTree(path)`:清单 path 缓存 + 立即调 `GetChildren` 返回最新数据。覆盖 `refreshNode`(右键/F5/文件操作后)。
   - `ClearAllFileTreeCache()`:清全部缓存。覆盖 `refreshAll`(工具栏"刷新",el-tree 整体重建,逐节点重拉)。
   - 前端 `refreshNode` 改调 `RefreshFileTree`,`refreshAll` 先调 `ClearAllFileTreeCache` 再走原重建流程。

## 失效策略(三层兜底)

- **mtime 差量**:被请求目录自身 mtime 变化(条目增删更新父目录 mtime)→ 缓存失效重扫。
- **TTL 5min**:过期强制重扫,兜底深层文件变更不更新父目录 mtime 的漏扫。
- **手动刷新**:`RefreshFileTree`/`ClearAllFileTreeCache` 绕过缓存,前端刷新按钮即时生效。

## 验收标准

- [ ] 二次展开同目录命中缓存,无 `os.ReadDir` / `git remote` 子进程调用。
- [ ] 目录 mtime 变化时缓存自动失效重扫。
- [ ] 前端"刷新"按钮(`refreshAll`)调 `ClearAllFileTreeCache` 后拿到最新数据。
- [ ] 右键刷新/F5/文件操作后(`refreshNode`)调 `RefreshFileTree` 拿到最新数据。
- [ ] TTL 5min 过期强制重扫。
- [ ] `GetFileTree`/`GetFileTreeRecursive` 返回类型与字段不变,前端无感知。
- [ ] 新增 `RefreshFileTree`/`ClearAllFileTreeCache` App 方法,wailsjs 三处绑定同步。
- [ ] 并发安全:并发展开同目录不触发 map 读写竞态。
- [ ] service 层测试覆盖率 ≥76%。

## 技术方案

### 缓存结构(纯内存)

`FileTreeService` 新增字段:

```go
type FileTreeService struct {
    gitCmd         *util.GitCommand
    gitRepoCache   sync.Map
    gitRemoteCache sync.Map
    treeCache      *FileTreeCache // 新增:单层目录节点缓存
}
```

`FileTreeCache`(新建 `service/filetree_cache.go`):

```go
type FileTreeCache struct {
    mu      sync.Mutex
    entries map[string]fileTreeCacheEntry // key = 规范化 path
}

type fileTreeCacheEntry struct {
    modTime time.Time
    nodes   []*model.FileTreeNode
    cachedAt time.Time
}
```

### GetChildren 改造

1. 规范化 path(`filepath.Abs`)。
2. `os.Stat(path)` 取 mtime;err → 直接实扫(目录不存在)。
3. 缓存命中且 `entry.modTime.Equal(curMtime)` 且未 TTL 过期 → 返回缓存节点深拷贝。
4. 未命中/失效 → 实扫(`os.ReadDir` + 排序 + git 信息)→ 回写缓存 → 返回。
5. 深拷贝:避免调用方修改缓存节点污染(`Children` 等)。

### 新增 App 方法(app_filetree.go)

- `RefreshFileTree(path string) []*model.FileTreeNode`:清 path 缓存 + 调 `GetChildren` 返回。
- `ClearAllFileTreeCache()`:清全部树缓存。

### 前端改造(FileTreePanel.vue)

- `refreshNode`:`GetFileTree(path)` → `RefreshFileTree(path)`。
- `refreshAll`:前置调 `ClearAllFileTreeCache()`,再 `refreshCounter++`。

### 并发安全

`FileTreeCache.mu` 在「读缓存 + 实扫 + 回写」期间持有,串行化同 path 并发。桌面应用场景用户触发,耗时极短(`os.ReadDir` <50ms),全局串行可接受(同 `ScanCacheManager` 策略)。

## 决策(ADR-lite)

**Context**: 文件树二次展开重复 `os.ReadDir` + `git remote` 子进程,大型仓库加载体感差。需缓存层。`task.json` 早期构想含持久化+fsnotify+LRU,需收敛 MVP。

**Decision**:
- 纯内存缓存(不落盘),复用 `RepoScanCache` 的 mtime+TTL+手动刷新范式。
- 仅缓存 `GetChildren` 单层,`GetTree` 自动受益。
- 双 API 打通前端刷新:`RefreshFileTree`(单点强刷)+ `ClearAllFileTreeCache`(全清)。

**Consequences**:
- 优点:实现简单,无 IO,无数据写入约束,签名兼容。
- 风险:跨会话不命中(首次展开仍全量),深层文件变更靠 TTL 兜底(5min 内可能短暂陈旧)。
- 演进:未来可加 fsnotify 实时失效或 LRU 上限,当前 Out of Scope。

## 完成定义

- 新增缓存层单元测试(命中/失效/TTL/并发)。
- 前端刷新路径打通后回归测试通过。
- `go test ./...` + `cd frontend && npm test` 绿。
- 若改签名或字段,同步 wailsjs 三处绑定。
- README/docs 按需更新。

## 技术说明

- 参考文件:`service/repo_scan_cache.go`、`service/git.go:282 scanGitReposCached`、`service/filetree.go`、`app_filetree.go`、`model/models.go:46 FileTreeNode`。
- 跨层契约:`docs/spec/cross-layer-contracts.md`。
- 测试门禁:`docs/spec/test-coverage-gate.md`。
- 数据写入约束:写 `data/*.json` 前须关闭 `workbench.exe`(见 memory)。

## 暂不涉及(Out of Scope)

- 监听 git 事件 / fsnotify 实时失效(MVP 用 TTL + 手动刷新兜底)。
- 缓存大小限制 / LRU 淘汰(MVP 不设上限,桌面单用户场景)。
- `GetGitInfo` 缓存(独立路径,不涉及树结构)。
