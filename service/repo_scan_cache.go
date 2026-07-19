package service

import (
	"path/filepath"
	"sync"
	"time"

	"workbench/util"
)

// scanCacheTTL 扫描缓存 TTL，超过此时间强制全量重扫，兜底 mtime 漏扫深层新增的风险。
const scanCacheTTL = 5 * time.Minute

// RepoScanCache 单个工作目录的扫描缓存。按规范化 rootPath 组织，落盘到 data/repo_scan_cache.json。
type RepoScanCache struct {
	// RootPath 规范化后的工作目录绝对路径（filepath.Abs）。
	RootPath string `json:"rootPath"`
	// ScannedAt 整次扫描完成时间，用于 TTL 判定（超过 scanCacheTTL 强制全扫）。
	ScannedAt time.Time `json:"scannedAt"`
	// Entries 目录扫描快照，key 为规范化子目录绝对路径。
	Entries map[string]CacheEntry `json:"entries"`
}

// CacheEntry 单个目录的扫描快照：记录 mtime 与该子树下的仓库列表，用于下次扫描差量判定。
type CacheEntry struct {
	// ModTime 该目录上次扫描时的 mtime；若未变则沿用缓存结论。
	ModTime time.Time `json:"modTime"`
	// IsRepo 该目录是否为 Git 仓库。
	IsRepo bool `json:"isRepo"`
	// SubtreeRepos 该目录子树下扫描到的所有仓库路径（含自身，若 IsRepo）。
	// 缓存命中时直接复用，避免递归重复扫描。
	SubtreeRepos []string `json:"subtreeRepos,omitempty"`
}

// ScanCacheManager 扫描缓存管理器，进程内单例。
//
// 并发安全：mu 互斥锁在 scanGitReposCached 的整次「扫描 + 落盘」期间持有，
// 串行化所有缓存读写，规避 Entries map 并发读写竞态（并发扫描同一 rootPath、
// 或跨 rootPath 扫描与落盘序列化交错均可触发 map 并发读写 panic）。
// 桌面应用场景扫描由用户触发且 .git 预筛后耗时极短（<0.5s），全局串行可接受。
// 缓存落盘失败时静默降级（仅内存），不阻塞扫描返回。
type ScanCacheManager struct {
	mu        sync.Mutex
	caches    map[string]*RepoScanCache // key = 规范化 rootPath
	cachePath string                    // data/repo_scan_cache.json
}

// NewScanCacheManager 创建缓存管理器并从磁盘加载已有缓存。加载失败时返回空内存缓存（降级）。
// 构造阶段无并发，无需持锁。
func NewScanCacheManager(cachePath string) *ScanCacheManager {
	m := &ScanCacheManager{
		caches:    make(map[string]*RepoScanCache),
		cachePath: cachePath,
	}
	m.load()
	return m
}

// load 从磁盘加载所有工作目录的缓存。文件不存在或解析失败时静默降级为空缓存。
func (m *ScanCacheManager) load() {
	var file map[string]*RepoScanCache
	if err := util.LoadJSON(m.cachePath, &file); err != nil || file == nil {
		return
	}
	m.caches = file
}

// getCacheLocked 取某 rootPath 的缓存（不存在则新建空缓存）。调用方须持 m.mu。
func (m *ScanCacheManager) getCacheLocked(rootPath string) *RepoScanCache {
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		abs = rootPath
	}
	if c, ok := m.caches[abs]; ok {
		return c
	}
	c := &RepoScanCache{RootPath: abs, Entries: make(map[string]CacheEntry)}
	m.caches[abs] = c
	return c
}

// saveLocked 将内存缓存快照落盘（调用方持 m.mu）。
// 持锁期间无并发修改，快照可直接共享 RepoScanCache 指针，无需深拷贝。
// 落盘失败仅静默降级：下次扫描仍可基于内存缓存工作，符合"不阻塞扫描"约束。
func (m *ScanCacheManager) saveLocked() {
	snapshot := make(map[string]*RepoScanCache, len(m.caches))
	for k, v := range m.caches {
		snapshot[k] = v
	}
	_ = util.SaveJSON(m.cachePath, snapshot)
}

// clear 清除指定 rootPath 的缓存，供手动刷新按钮（PRD F9）绕过缓存强制全扫。
func (m *ScanCacheManager) clear(rootPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		abs = rootPath
	}
	delete(m.caches, abs)
}

// ttlExpired 判断缓存是否超过 TTL，需强制全扫。
func (c *RepoScanCache) ttlExpired() bool {
	if c.ScannedAt.IsZero() {
		return true
	}
	return time.Since(c.ScannedAt) > scanCacheTTL
}
