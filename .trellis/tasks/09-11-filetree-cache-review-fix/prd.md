# filetree-cache 审核修复

## 目标

修复独立代码审核(commit 97ed82a)发现的 4 项问题:#1 内存无界、#2 字段同步无保障、#6 递归死代码、#7 命名错位。

## 背景

filetree-cache 任务已归档(09-10-filetree-cache)。独立 code-review(4 finder × recall)发现 7 项,本次修复其中 4 项。审核报告见本会话上下文。

## 修复项

### #1 TTL 过期不驱逐(中严重性)

**位置**:`service/filetree_cache.go` `get` 方法 TTL 分支。

**问题**:TTL 过期仅 `return nil, false`,不 delete 条目。`nodes` 切片整会话常驻,直到 `refreshAll` 全清。对比 `ScanCacheManager` 范式:TTL 过期触发整体重建。FileTreeCache 只做失效判定未做回收,内存与「历史访问总目录数」成正比而非「当前视图」。

**修复**:`get` 的 TTL 过期分支 `delete(c.entries, path)` 后再返回 miss。单条目驱逐,避免大仓库长会话内存累积。

### #2 deepCopyNode 字段同步无编译期保障(中严重性)

**位置**:`service/filetree_cache.go` `deepCopyNode` + 测试。

**问题**:手写字段拷贝,后续 `FileTreeNode` 新增字段时 `deepCopyNode` 不报编译错误,缓存静默返回缺字段节点,仅缓存命中路径异常,难定位。

**修复**:在 `filetree_cache_test.go` 补一条断言测试——用反射比对 `deepCopyNode` 输出与原节点的全部字段,新增字段未同步则测试失败。用 `reflect.DeepEqual` 或逐字段反射校验。

### #6 deepCopyNode 递归 Children 死代码(低严重性)

**位置**:`service/filetree_cache.go` `deepCopyNode` 递归分支。

**问题**:`GetChildren` 仅构建单层节点(`NewFileTreeNode` 不设 `Children`,`set` 时 `Children` 恒 nil),递归分支永不执行。读者误以为缓存存储多层树。

**修复**:保留递归(防御未来多层缓存,成本零因 Children 恒 nil),补注释明确「当前单层缓存下 Children 恒 nil,递归为防御未来扩展,非当前执行路径」。

### #7 RefreshFileTree 命名错位 + 序列化浪费(低-中严重性)

**位置**:`app_filetree.go` `RefreshFileTree` + `service/filetree.go` `RefreshChildren` + wailsjs 绑定 + 前端 `FileTreePanel.vue`。

**问题**:`RefreshFileTree` 返回 `[]*FileTreeNode`,前端 `refreshNode` `await` 后丢弃返回值,依赖后续 `expand`→`GetFileTree` 命中预热缓存。完整节点列表经 Wails 序列化传输后被 GC,纯浪费。命名暗示「刷新并返回数据」而实际只用清缓存副作用。

**修复**:改名 `InvalidateFileTreeCache(path)`,纯 `clearPath` 不返回、不预热。`expand` 仍 miss→实扫→回写,数据流等价,消除无用序列化。

**跨层契约**:App 方法签名变更(`RefreshFileTree(path) []*FileTreeNode` → `InvalidateFileTreeCache(path)` 无返回),须同步 wailsjs 三处(App.js / App.d.ts;models.ts 无变更因无新模型)。

## 验收标准

- [ ] #1:`get` TTL 过期分支 delete 条目,测试覆盖「过期后条目从 map 移除」。
- [ ] #2:反射断言测试覆盖 `deepCopyNode` 全字段;新增字段未同步时测试失败。
- [ ] #6:`deepCopyNode` 递归分支补注释明确防御语义。
- [ ] #7:`RefreshFileTree` → `InvalidateFileTreeCache`(Go + wailsjs + 前端三处同步);前端 `refreshNode` 不再使用返回值;`expand` 数据流验证。
- [ ] `go test ./...` + `go test -race ./service/...` 绿。
- [ ] service 覆盖率 ≥76%。
- [ ] `cd frontend && npm test` 绿。
- [ ] wailsjs 三处绑定一致(跨层契约)。

## 完成定义

- 后端测试通过 + 覆盖率达标。
- wailsjs 三处绑定同步。
- 前端测试通过。
- 无 git commit(由主线程提交)。

## 技术方案

### service/filetree_cache.go

```go
// get 的 TTL 过期分支(#1)
if time.Since(entry.cachedAt) > fileTreeCacheTTL {
    delete(c.entries, path)  // 驱逐过期条目,避免内存无界增长
    return nil, false
}
```

```go
// deepCopyNode 递归分支补注释(#6)
// 当前 GetChildren 仅构建单层节点(Children 恒为 nil),递归分支不执行;
// 保留递归为防御未来多层缓存扩展,非当前执行路径。
if n.Children != nil {
    cp.Children = make([]*model.FileTreeNode, len(n.Children))
    for i, child := range n.Children {
        cp.Children[i] = deepCopyNode(child)
    }
}
```

### service/filetree.go

```go
// RefreshChildren → InvalidateCache(#7):纯清缓存,不预热
func (s *FileTreeService) InvalidateCache(dirPath string) {
    abs, err := filepath.Abs(dirPath)
    if err != nil {
        abs = dirPath
    }
    s.treeCache.clearPath(abs)
}
```

### app_filetree.go

```go
// RefreshFileTree → InvalidateFileTreeCache(#7):无返回值
func (a *App) InvalidateFileTreeCache(path string) {
    a.fileTreeSvc.InvalidateCache(path)
}
```

### 前端 FileTreePanel.vue

```js
// refreshNode:RefreshFileTree → InvalidateFileTreeCache,不使用返回值
if (refreshPath) {
    try { await InvalidateFileTreeCache(refreshPath) }
    catch (error) { console.error('Error clearing file tree cache:', error) }
}
```

### 测试 #2(反射断言)

用 `reflect` 遍历 `model.FileTreeNode` 全字段,比对 `deepCopyNode(original)` 与 `original` 的字段值相等(指针字段如 Children 需递归比对值非地址)。新增字段未在 `deepCopyNode` 拷贝时,反射断言能捕获(因拷贝值为零值,原值非零值时不等)。

## 暂不涉及(Out of Scope)

- #3(get 锁内深拷贝):桌面低并发,改入成本不抵收益。
- #4(锁外实扫竞态):NTFS 自纠正,SMB 边缘场景。
- #5(RefreshChildren 冗余 Stat):被 #7 消除(改名后不再调 GetChildren)。

## 技术说明

- 原实现:commit 97ed82a(已归档任务 09-10-filetree-cache)。
- 跨层契约:`docs/spec/cross-layer-contracts.md`。
- 测试门禁:`docs/spec/test-coverage-gate.md`(service ≥76%)。
