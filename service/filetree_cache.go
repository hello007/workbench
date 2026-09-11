package service

import (
	"sync"
	"time"

	"workbench/model"
)

// fileTreeCacheTTL 文件树缓存 TTL，超过此时间强制重扫，兜底目录 mtime 未变化但深层文件
// 已变更的漏扫风险（深层新增/删除文件不一定更新父目录 mtime）。
const fileTreeCacheTTL = 5 * time.Minute

// FileTreeCache 文件树单层目录节点缓存（纯内存，不落盘）。
//
// 缓存粒度：按目录路径键，存单层 GetChildren 结果（depth=1 隐含）。
// GetTree 递归内部调 GetChildren，自动受益，无需 path|depth 复合键。
//
// 失效策略（三层兜底）：
//   - mtime 差量：被请求目录自身 mtime 变化（条目增删更新父目录 mtime）-> 缓存失效重扫
//   - TTL 5min：过期强制重扫，兜底深层文件变更不更新父目录 mtime 的漏扫
//   - 手动刷新：clearPath/clearAll 绕过缓存，供前端刷新按钮即时生效
//
// 并发安全：mu 互斥锁在 get/set/clearPath/clearAll 整次操作期间持有，串行化 entries map
// 读写，规避并发读写竞态。桌面应用场景用户触发，os.ReadDir <50ms，全局串行可接受
// （同 ScanCacheManager 策略）。
//
// 数据隔离：set 存储节点深拷贝，get 返回节点深拷贝，调用方修改节点（如 buildTree 填充
// Children）不污染缓存。set 与 get 各执行一次深拷贝，保证缓存内部与调用方持有的始终是
// 独立副本。
type FileTreeCache struct {
	mu      sync.Mutex
	entries map[string]fileTreeCacheEntry // key = 规范化 path（filepath.Abs 结果，由调用方传入）
}

// fileTreeCacheEntry 单个目录的节点缓存快照。
type fileTreeCacheEntry struct {
	// modTime 目录上次扫描时的 mtime；若未变则沿用缓存结论。
	modTime time.Time
	// nodes 单层子节点列表（深拷贝隔离，调用方不可直接持有引用）。
	nodes []*model.FileTreeNode
	// cachedAt 落缓存时间，用于 TTL 判定（超过 fileTreeCacheTTL 强制重扫）。
	cachedAt time.Time
}

// NewFileTreeCache 创建文件树缓存。
func NewFileTreeCache() *FileTreeCache {
	return &FileTreeCache{entries: make(map[string]fileTreeCacheEntry)}
}

// get 取缓存。命中（modTime 相等且未 TTL 过期）返回节点深拷贝与 true；
// 未命中或失效（mtime 变化 / TTL 过期）返回 nil, false。
//
// 返回深拷贝而非缓存内部引用，避免调用方修改节点（如 buildTree 填充 Children）污染缓存。
func (c *FileTreeCache) get(path string, curMtime time.Time) ([]*model.FileTreeNode, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[path]
	if !ok {
		return nil, false
	}
	// mtime 变化 -> 目录条目增删 -> 缓存失效
	if !entry.modTime.Equal(curMtime) {
		return nil, false
	}
	// TTL 过期 -> 兜底深层文件变更漏扫
	if time.Since(entry.cachedAt) > fileTreeCacheTTL {
		return nil, false
	}
	return deepCopyNodes(entry.nodes), true
}

// set 回写缓存。存储节点深拷贝，与调用方持有的原始节点解耦，
// 调用方后续修改原始节点不影响缓存。
func (c *FileTreeCache) set(path string, modTime time.Time, nodes []*model.FileTreeNode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[path] = fileTreeCacheEntry{
		modTime:  modTime,
		nodes:    deepCopyNodes(nodes),
		cachedAt: time.Now(),
	}
}

// clearPath 清除指定路径的缓存，供 RefreshFileTree 单点强刷（右键/F5/文件操作后）。
func (c *FileTreeCache) clearPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, path)
}

// clearAll 清除全部缓存，供 ClearAllFileTreeCache 全量刷新（工具栏"刷新"按钮，el-tree 整体重建）。
func (c *FileTreeCache) clearAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]fileTreeCacheEntry)
}

// deepCopyNode 深拷贝单个节点（含 Children 递归），避免调用方修改节点污染缓存。
func deepCopyNode(n *model.FileTreeNode) *model.FileTreeNode {
	if n == nil {
		return nil
	}
	cp := &model.FileTreeNode{
		ID:          n.ID,
		Name:        n.Name,
		Path:        n.Path,
		Type:        n.Type,
		IsGitRepo:   n.IsGitRepo,
		HasRemote:   n.HasRemote,
		HasChildren: n.HasChildren,
		IsLeaf:      n.IsLeaf,
	}
	if n.Children != nil {
		cp.Children = make([]*model.FileTreeNode, len(n.Children))
		for i, child := range n.Children {
			cp.Children[i] = deepCopyNode(child)
		}
	}
	return cp
}

// deepCopyNodes 深拷贝节点列表，避免调用方修改节点污染缓存。
func deepCopyNodes(nodes []*model.FileTreeNode) []*model.FileTreeNode {
	if nodes == nil {
		return nil
	}
	result := make([]*model.FileTreeNode, len(nodes))
	for i, n := range nodes {
		result[i] = deepCopyNode(n)
	}
	return result
}
