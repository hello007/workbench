package service

import (
	"strings"
	"sync"
	"time"

	"workbench/model"
)

// commitHistoryCacheTTL 提交历史缓存 TTL，超过此时间强制全量重扫。
// 兜底 SHA 链断裂检测遗漏（如外部工具改写历史后未触发增量路径）与 HEAD ref 未变但
// refs 内容已被外部改写的极端场景。同 fileTreeCacheTTL 取 5min。
const commitHistoryCacheTTL = 5 * time.Minute

// CommitHistoryMaxEntries 单仓缓存条数上限。超限不写缓存，走原 go-git 路径，防爆内存。
// 桌面单用户场景 5000 条覆盖绝大多数仓库；超大仓（如内核级）不缓存，性能退回原行为。
// 导出供 app_git.go 全量扫上限判定与 cache.Set 内超限拒绝共用。
const CommitHistoryMaxEntries = 5000

// CommitHistoryCache 提交历史全量快照缓存（纯内存，不落盘）。
//
// 缓存粒度：按 仓库根 + HEAD ref 复合键，存全量 []model.Commit（含 Files 字段，
// 避免 getCommitFiles 逐提交 tree patch 重复计算）。HEAD ref 隔离分支切换与
// detached HEAD，不串历史。
//
// 失效与更新策略（四层）：
//   - HEAD SHA 相同 + TTL 内 -> 命中，返缓存全量深拷贝
//   - HEAD SHA 不同（commit/pull 后 HEAD 前移）-> 调用方走增量 prepend 路径：
//     从新 HEAD 迭代收集新提交直到与缓存已有 SHA 交集，prepend 后 Set 回写
//   - SHA 链断裂（rebase/amend 改写历史，迭代超阈值无交集）-> 调用方回退全量重扫
//   - TTL 过期 -> 驱逐条目，调用方全量重扫；手动 ClearPath/ClearAll 绕过缓存
//
// 缓存层只管存取与 TTL/上限判定，不依赖 go-git；增量 prepend 的交集检测与
// 全量扫描由调用方（app_git.go GetCommitHistory）编排，结果回写 Set。
//
// 并发安全：mu 互斥锁在 Get/Set/ClearPath/ClearByGitRoot/ClearAll 整次操作期间持有，
// 串行化 entries map 读写，规避并发读写竞态。同 FileTreeCache / ScanCacheManager 策略。
//
// 数据隔离：Set 存储深拷贝、Get 返回深拷贝，调用方修改返回切片（如过滤、分页切片）
// 不污染缓存。model.Commit 含 Files []string 切片，深拷贝须复制切片底层数组，
// 不能仅复制 struct（切片共享底层数组会致过滤修改污染缓存）。
type CommitHistoryCache struct {
	mu      sync.Mutex
	entries map[string]commitHistoryCacheEntry // key = gitRoot + "|" + headRef，由调用方构造传入
}

// commitHistoryCacheEntry 单个仓库 + HEAD ref 的提交历史缓存快照。
type commitHistoryCacheEntry struct {
	// headSHA 落缓存时的 HEAD SHA，增量判定基准：调用方取当前 HEAD SHA 与此比对，
	// 相同则命中，不同则走增量 prepend（HEAD 前移）或全量回退（SHA 链断）。
	headSHA string
	// commits 全量提交快照（深拷贝隔离，调用方不可直接持有引用）。按 committer time 倒序，
	// 与 go-git LogOrderCommitterTime 一致。
	commits []model.Commit
	// cachedAt 落缓存时间，用于 TTL 判定（超过 commitHistoryCacheTTL 强制重扫）。
	cachedAt time.Time
}

// NewCommitHistoryCache 创建提交历史缓存。
func NewCommitHistoryCache() *CommitHistoryCache {
	return &CommitHistoryCache{entries: make(map[string]commitHistoryCacheEntry)}
}

// Get 取缓存。TTL 内返条目深拷贝与缓存 headSHA；TTL 过期驱逐条目返 found=false；
// 无条目返 found=false。
//
// 返回 headSHA 供调用方判定命中/增量/全量：found=true 且 headSHA==当前 HEAD -> 命中；
// found=true 且 headSHA!=当前 HEAD -> 增量 prepend；found=false -> 全量重扫。
// 返回深拷贝而非缓存内部引用，避免调用方过滤/分页修改污染缓存。
func (c *CommitHistoryCache) Get(key string) (commits []model.Commit, headSHA string, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, "", false
	}
	// TTL 过期 -> 兜底 SHA 链断裂检测遗漏与 refs 外部改写；驱逐条目避免内存无界增长
	if time.Since(entry.cachedAt) > commitHistoryCacheTTL {
		delete(c.entries, key)
		return nil, "", false
	}
	return deepCopyCommits(entry.commits), entry.headSHA, true
}

// Set 回写缓存。存储深拷贝与调用方持有的原始切片解耦，调用方后续修改不影响缓存。
// len(commits) 超 CommitHistoryMaxEntries 时不写缓存，避免超大仓占用内存，
// 调用方仍返回数据（走原 go-git 路径不缓存）。
func (c *CommitHistoryCache) Set(key, headSHA string, commits []model.Commit) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(commits) > CommitHistoryMaxEntries {
		return
	}
	c.entries[key] = commitHistoryCacheEntry{
		headSHA:  headSHA,
		commits:  deepCopyCommits(commits),
		cachedAt: time.Now(),
	}
}

// ClearPath 清除指定 key 的缓存，供 InvalidateCommitHistoryCache 单点强刷
// （前端 handleRefresh 前置调用，绕过缓存确保下次请求全量重扫）。
func (c *CommitHistoryCache) ClearPath(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// ClearByGitRoot 清除指定仓库根路径下的全部缓存条目（覆盖该仓库所有分支 ref 与
// detached HEAD 键）。供 InvalidateCommitHistoryCache 按仓库清单点强刷，无需调用方
// 知晓当前 ref——同仓库分支切换遗留的旧 ref 键一并清除。
func (c *CommitHistoryCache) ClearByGitRoot(gitRoot string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := gitRoot + "|"
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
}

// ClearAll 清除全部缓存，供 ClearAllCommitHistoryCache 全量刷新
// （工具栏全刷按钮，预留入口）。
func (c *CommitHistoryCache) ClearAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]commitHistoryCacheEntry)
}

// deepCopyCommits 深拷贝提交切片。struct 值字段（SHA/Message 等）随切片复制传递，
// Files []string 切片须显式 make+copy 复制底层数组，否则调用方修改 Files 元素
// 或对返回切片做 append 截断会污染缓存内部底层数组。
func deepCopyCommits(in []model.Commit) []model.Commit {
	if in == nil {
		return nil
	}
	out := make([]model.Commit, len(in))
	for i, c := range in {
		out[i] = c
		if c.Files != nil {
			out[i].Files = make([]string, len(c.Files))
			copy(out[i].Files, c.Files)
		}
	}
	return out
}
