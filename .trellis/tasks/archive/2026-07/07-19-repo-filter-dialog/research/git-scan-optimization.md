# Research: Git 仓库递归扫描性能优化（.git 预筛 + mtime 缓存）

- **Query**: 在 `GitService.ScanGitRepos` 基础上优化递归扫描性能，达成"扫描 100 个仓库 < 3 秒"目标
- **Scope**: internal（基于现有 Go 源码分析）+ 方案设计
- **Date**: 2026-07-19
- **关联 PRD**: `.trellis/tasks/07-19-repo-filter-dialog/prd.md`（NF1 性能目标、F12 扫描复用+优化）

---

## 一、现状分析（基于现有代码）

### 1.1 扫描入口与递归逻辑

**`service/git.go:214` `ScanGitRepos`**

```go
func (s *GitService) ScanGitRepos(rootPath string) []string {
	if s.gitCmd.IsGitRepository(rootPath) {  // ① root 本身先 fork 一次 git
		return []string{rootPath}
	}
	var repos []string
	s.scanDir(rootPath, &repos)
	return repos
}
```

**`service/git.go:224` `scanDir`**

```go
func (s *GitService) scanDir(dir string, repos *[]string) {
	entries, err := os.ReadDir(dir)
	// ...
	for _, entry := range entries {
		if !entry.IsDir() { continue }
		fullPath := filepath.Join(dir, entry.Name())
		if entry.Name() == ".git" { continue }     // 仅跳过名为 .git 的直接子项
		if s.gitCmd.IsGitRepository(fullPath) {     // ② 每个子目录 fork 一次 git
			*repos = append(*repos, fullPath)
		} else {
			s.scanDir(fullPath, repos)              // ③ 非仓库则继续递归
		}
	}
}
```

### 1.2 仓库判定实现

**`util/git.go:80` `IsGitRepository`**

```go
func (g *GitCommand) IsGitRepository(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	HideCommandWindow(cmd)
	return cmd.Run() == nil
}
```

**关键问题**：每判定一个目录就 fork 一个 `git.exe` 子进程。Windows 上 `git.exe` 体积大、启动慢，单次 `git rev-parse --git-dir` 冷启动约 **30~80ms**（含进程创建 + git 初始化 + 路径解析）。

### 1.3 性能瓶颈估算

| 场景 | 候选目录数 | fork 次数 | 串行耗时（按 50ms/次） |
|---|---|---|---|
| 100 个仓库，每仓库独立目录 | ~100 | 100 | ~5s |
| 100 个仓库，嵌套在工作目录下 | ~300~1000 | 300~1000 | **15~50s** |
| root 本身是仓库 | 1 | 1 | <0.1s |

**结论**：现状在"工作目录下嵌套 100 个仓库"场景下远超 3 秒目标，瓶颈是子进程开销，必须用 `.git` 存在性预筛替代 fork。

### 1.4 现有可复用资产

| 资产 | 位置 | 复用价值 |
|---|---|---|
| 并行范式（sem + WaitGroup + Mutex） | `service/git.go:499` `BatchPull` | 并行扫描可直接套用 |
| JSON 持久化工具 | `util/json.go` `LoadJSON/SaveJSON` | mtime 缓存读写 |
| 路径规范化 | `filepath.Abs`（`directory.go:53`） | 缓存主键统一 |
| 数据目录约定 | `data/directories.json`（`app.go:40`） | 缓存文件放 `data/repo_scan_cache.json` |
| `ScanAndPullRepos` 调用链 | `app.go:327` | 已调 `ScanGitRepos` + `BatchPull`，优化后自动受益，无需改动 |

**`ScanAndPullRepos` 复用情况**：`app.go:327` 仅做 `ScanGitRepos(dirPath)` → `BatchPull(repos, 5, ctx)`。优化 `ScanGitRepos` 即可让一键更新场景同步受益，**无需新增独立扫描逻辑**。

---

## 二、.git 预筛方案

### 2.1 核心思路

用 `os.Stat(<dir>/.git)` 的存在性作为快速预筛，仅当 `.git` 存在时才（可选）调 `git rev-parse` 兜底校验。`os.Stat` 是单次系统调用，开销约 **0.05~0.2ms**，比 fork git 子进程快 **200~1000 倍**。

### 2.2 准确性分析（关键边界情况）

| 仓库类型 | `.git` 形态 | `os.Stat` 结果 | `info.IsDir()` | 纯存在性判定 | 备注 |
|---|---|---|---|---|---|
| 标准仓库（`git init`/`clone`） | 目录 | 存在 | true | ✅ 识别 | 主流场景 |
| worktree（`git worktree add`） | **文件**，内容 `gitdir: /path/...` | 存在 | **false** | ✅ 识别（仅判存在） | ⚠️ 若用 `IsDir()` 会漏判 |
| submodule | 文件，指向父仓 `.git/modules/<name>` | 存在 | false | ✅ 识别（仅判存在） | 同上 |
| bare repo（`git init --bare`） | **无 `.git`**，根目录即 git dir | 不存在 | - | ❌ 漏判 | 桌面工作目录几乎不出现 |
| 损坏/残留 `.git`（手动建空目录、init 中断） | 目录（内容不全） | 存在 | true | ⚠️ 误判 | 概率极低，后续 `BatchPull` 会报错暴露 |

**关键发现**：现有 `util/git.go:142` `FindGitRoot` 使用 `info.IsDir()` 判定，**对 worktree/submodule 会漏判**（这是一个已存在的潜在 bug，但本任务的预筛方案应避免重蹈覆辙）。

**预筛实现要点**：判定条件应为 `os.Stat(dir+"/.git")` **无错**（不要求 `IsDir()`），才能覆盖 worktree 和 submodule。

### 2.3 性能提升估算

| 方案 | 100 仓库（~500 候选目录） | 改动量 |
|---|---|---|
| 现状（全 fork git） | ~25s | - |
| 预筛 os.Stat + 疑似才 fork git 兜底 | 100 fork × 50ms + 400 stat × 0.1ms ≈ **5s** | 中 |
| 纯 os.Stat 判定（免 fork） | 500 stat × 0.1ms ≈ **0.05s** | 小 |

---

## 三、能否完全用 .git 存在性判定（免 git 子进程）

### 3.1 结论：**可以，且推荐**

**理由**：
1. **覆盖面足够**：标准仓库、worktree、submodule 全覆盖，这三类占桌面应用场景 99.9%+。
2. **bare repo 漏判可接受**：用户工作目录下几乎不会出现裸仓库（裸仓库用于服务器托管，无工作区，无法在文件树中浏览文件）。
3. **损坏 .git 误判风险低且可暴露**：即便误判为仓库，后续 `BatchPull` / `GetInfo` 调用真实 git 命令时会立即报错，不会造成静默数据错误。
4. **扫描用途决定**：`ScanGitRepos` 的职责是"收集疑似仓库路径列表"供后续操作，不是"权威验证仓库完整性"。验证可在实际操作时按需进行。

### 3.2 与 `git rev-parse` 的语义差异

| 维度 | `os.Stat(.git)` | `git rev-parse --git-dir` |
|---|---|---|
| 检测内容 | `.git` 条目存在 | git 能正确解析仓库元数据 |
| 损坏仓库 | 仍判定为仓库 | 判定为非仓库（报错） |
| 权限问题 | 仅看条目存在 | 实际尝试访问，可能因权限失败 |
| 开销 | ~0.1ms | ~50ms |

对"扫描收集路径"场景，`os.Stat` 的宽松判定**反而更合适**——宁可多收一个后续可暴露的损坏仓库，也不愿为每个目录付出 500 倍开销。

---

## 四、mtime 缓存差量方案

### 4.1 设计

**缓存结构**：

```go
// RepoScanCache 扫描结果缓存，按工作目录根组织
type RepoScanCache struct {
	RootPath  string                  `json:"rootPath"`  // 规范化后的工作目录绝对路径
	ScannedAt time.Time               `json:"scannedAt"` // 整次扫描完成时间
	Entries   map[string]CacheEntry   `json:"entries"`   // key = 规范化子目录绝对路径
}

// CacheEntry 单个目录的扫描快照
type CacheEntry struct {
	ModTime time.Time `json:"modTime"` // 该目录的 mtime（os.ReadDir info.ModTime()）
	IsRepo  bool      `json:"isRepo"`  // 是否为 git 仓库
}
```

**缓存文件**：`data/repo_scan_cache.json`（与 `directories.json` 同目录，复用 `util.LoadJSON/SaveJSON`）。

### 4.2 失效策略（逐目录 mtime 比对）

递归扫描时对每个目录执行：

```
entry, hit := cache[dir]
currentMtime := readDirModTime(dir)   // os.Stat(dir).ModTime()

if hit && entry.ModTime.Equal(currentMtime) {
	// mtime 未变 -> 沿用缓存
	if entry.IsRepo {
		repos = append(repos, dir)   // 是仓库，直接收录
	}
	// 无论是否仓库，都不再递归该子树（mtime 未变意味着直接子条目无增删）
	continue
}

// mtime 变化或无缓存 -> 实际扫描该目录
isRepo := os.Stat(dir+"/.git") == nil
if isRepo {
	repos = append(repos, dir)
	// 仓库目录不再递归（与现状 scanDir 语义一致）
} else {
	scanDir(dir, repos)  // 继续递归
}
// 更新缓存
cache[dir] = CacheEntry{ModTime: currentMtime, IsRepo: isRepo}
```

### 4.3 mtime 可靠性边界（重要 Caveat）

**可靠的情况**：在目录下**新增/删除直接子条目**（文件或子目录），父目录 mtime 会更新（Windows NTFS、Linux ext4、macOS APFS 均如此）。

**不可靠的情况**：
1. **深层变更不传导**：`a/b/c` 下新增仓库，`a/b/c` 的 mtime 变，但 `a/b`、`a` 的 mtime **可能不变**。
   - 缓解：本方案是**逐目录比对**（递归进入 `a/b` 后再比对 `a/b/c`），只要 `a/b/c` 自身 mtime 变了就能识别。但若 `a/b` 被缓存判定为"mtime 未变且非仓库 -> 跳过整棵子树"，则会漏扫 `a/b/c` 的新增仓库。
   - **关键**：必须确保"非仓库 + mtime 未变"时**仍递归进入子目录**比对，而非直接跳过整棵子树。即缓存只跳过"该目录自身的 IsRepo 判定"，**不跳过子目录的遍历**（除非能确认整棵子树无变化）。
2. **mtime 精度**：某些文件系统 mtime 精度为秒，1 秒内多次变更可能不可见。桌面场景罕见。
3. **外部工具修改**：用户在 WorkBench 之外用资源管理器增删目录，mtime 仍会更新，方案有效。

**推荐的安全策略**：
- **逐目录比对 mtime，但始终递归遍历子目录**（不因缓存跳过子树遍历），仅用缓存跳过"该目录是否仓库"的 os.Stat 调用——但这几乎省不了多少开销（os.Stat 本就极快）。
- **更激进的策略**：非仓库目录 mtime 未变时跳过整棵子树，配合**手动刷新按钮强制全扫**（PRD F9 已要求）+ **TTL 兜底**（如缓存超过 5 分钟强制重扫）。这是真正能带来"二次打开瞬时"体验的策略。

**权衡**：方案 B（推荐）采用"激进跳过子树 + 手动刷新兜底"，因为：
- PRD F9 明确提供"手动刷新按钮"，用户可强制重扫
- mtime 漏扫深层新增的概率低（新增目录通常更新父 mtime）
- 漏扫的仓库会在下次 TTL 过期或手动刷新时补上，不丢数据

### 4.4 缓存生命周期

- **加载**：`GitService` 初始化时或首次扫描某 rootPath 时从 `data/repo_scan_cache.json` 加载到内存 `map[string]*RepoScanCache`（key 为 rootPath）
- **更新**：扫描完成后回写内存 + 异步落盘（避免阻塞返回）
- **失效**：手动刷新按钮触发 `forceRescan=true`，绕过缓存全量扫描并覆盖缓存
- **TTL**：`ScannedAt` 超过 5 分钟（可配置）时强制全扫

---

## 五、并行扫描分析

### 5.1 收益评估

| 子任务 | 单次开销 | 并行收益 |
|---|---|---|
| `os.Stat(.git)` | ~0.1ms | 极低（IO 轻量，OS 缓存命中率高，并行反而有调度开销） |
| `git rev-parse`（若保留兜底） | ~50ms | **高**（CPU/IO bound，并行可线性加速） |
| `os.ReadDir` | ~0.5ms | 中等 |

**结论**：
- **纯 os.Stat 方案下，并行收益微小**（1000 目录 × 0.1ms = 0.1s，串行已达标）
- **若保留 git rev-parse 兜底校验，并行有意义**（100 fork × 50ms / 8 并发 ≈ 0.6s）

### 5.2 实现要点（如需并行）

复用 `BatchPull`（`service/git.go:499`）的成熟范式：

```go
sem := make(chan struct{}, concurrency)  // 信号量限流，建议 8~16
var wg sync.WaitGroup
var mu sync.Mutex
var repos []string

for _, subDir := range candidateDirs {
	wg.Add(1)
	go func(d string) {
		defer wg.Done()
		sem <- struct{}{}
		defer func() { <-sem }()
		if isGitRepoFast(d) {
			mu.Lock()
			repos = append(repos, d)
			mu.Unlock()
		}
	}(subDir)
}
wg.Wait()
```

**注意**：
- 并发度不宜过高，`git.exe` 子进程并发过多会争抢 CPU/磁盘，建议 8~16
- 递归扫描的并行需先收集"待判定目录列表"再并行判定，或用 worker pool 模式递归投递任务（复杂度上升）

### 5.3 推荐：**不并行**

纯 os.Stat 方案下串行已远超目标（<0.5s），并行徒增复杂度（递归 + 共享状态 + 调试困难），**不推荐**。仅在保留 git 兜底校验时才考虑并行。

---

## 六、三方案对比与推荐

| 维度 | A: 纯 .git 预筛 | B: 预筛 + mtime 缓存 | C: 预筛 + 缓存 + 并行 |
|---|---|---|---|
| **改动量** | 小（改 `scanDir` + 新增 `IsGitRepositoryFast`） | 中（A + 缓存读写层 + mtime 比对） | 大（B + errgroup/worker pool） |
| **首次扫描（100 仓库）** | <0.5s ✅ | <0.5s ✅ | <0.3s ✅ |
| **二次扫描（缓存命中）** | 同首次（<0.5s） | **<50ms** ✅✅ | <50ms |
| **达标 NF1（<3s）** | ✅ 达标 | ✅ 达标 | ✅ 达标 |
| **复杂度** | 低 | 中 | 高 |
| **准确性风险** | bare repo 漏判（可忽略）；损坏 .git 误判（罕见） | 同 A + mtime 漏扫深层新增（手动刷新/TTL 兜底） | 同 B + 并发状态管理风险 |
| **契合 PRD F12** | 部分（仅预筛） | ✅ 完全契合（预筛 + mtime 缓存） | 完全契合但过度 |
| **契合 PRD F9（手动刷新）** | 无缓存无需刷新 | ✅ 手动刷新绕过缓存 | ✅ |

### 6.1 推荐：**方案 B（预筛 + mtime 缓存）**

**理由**：
1. **完全契合 PRD F12**："复用 `ScanGitRepos`，加 `.git` 预筛 + mtime 缓存"——方案 B 一一对应。
2. **NF1 达标余量大**：首次 <0.5s，二次 <50ms，远低于 3s 目标。
3. **体验最佳**：用户二次打开弹窗近乎瞬时（PRD F9"打开弹窗时自动扫描"不会让用户等待）。
4. **复杂度可控**：缓存层逻辑清晰，复用现有 `util.LoadJSON/SaveJSON`，无需引入并发复杂度。
5. **方案 C 过度**：纯 os.Stat 下并行收益微小（<0.2s 差距），不值得引入递归并发的复杂度与风险。

**何时升级到 C**：若未来保留 `git rev-parse` 兜底校验（例如要求"扫描时排除损坏仓库"），则并行有意义，可平滑升级。

---

## 七、推荐方案（B）代码骨架

### 7.1 新增：`util/git.go` 快速预筛

```go
// IsGitRepositoryFast 基于 .git 条目存在性快速判定目录是否为 Git 仓库。
// 不要求 .git 是目录（覆盖 worktree/submodule 的 .git 文件场景）。
// 桌面应用场景下 bare repo（无 .git）会漏判，可接受。
// 开销：单次 os.Stat，约 0.1ms，比 fork git rev-parse 快 200~1000 倍。
func IsGitRepositoryFast(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}
```

### 7.2 新增：`service/repo_scan_cache.go` 缓存层

```go
package service

import (
	"path/filepath"
	"sync"
	"time"

	"workbench/util"
)

// RepoScanCache 单个工作目录的扫描缓存
type RepoScanCache struct {
	RootPath  string                `json:"rootPath"`
	ScannedAt time.Time             `json:"scannedAt"`
	Entries   map[string]CacheEntry `json:"entries"`
}

// CacheEntry 目录扫描快照
type CacheEntry struct {
	ModTime time.Time `json:"modTime"`
	IsRepo  bool      `json:"isRepo"`
}

// ScanCacheManager 缓存管理器，进程内单例
type ScanCacheManager struct {
	mu        sync.RWMutex
	caches    map[string]*RepoScanCache // key = 规范化 rootPath
	cachePath string                    // data/repo_scan_cache.json
	ttl       time.Duration             // 缓存 TTL，超时强制全扫
}

func NewScanCacheManager(cachePath string) *ScanCacheManager {
	m := &ScanCacheManager{
		caches:    make(map[string]*RepoScanCache),
		cachePath: cachePath,
		ttl:       5 * time.Minute,
	}
	m.load() // 启动时加载
	return m
}

// load 从磁盘加载所有工作目录的缓存
func (m *ScanCacheManager) load() {
	var file map[string]*RepoScanCache
	if err := util.LoadJSON(m.cachePath, &file); err == nil && file != nil {
		m.caches = file
	}
}

// save 异步落盘
func (m *ScanCacheManager) save() {
	m.mu.RLock()
	snapshot := make(map[string]*RepoScanCache, len(m.caches))
	for k, v := range m.caches {
		snapshot[k] = v
	}
	m.mu.RUnlock()
	_ = util.SaveJSON(m.cachePath, snapshot) // 实际可包一层 goroutine
}

// getCache 取某 rootPath 的缓存（不存在则新建空缓存）
func (m *ScanCacheManager) getCache(rootPath string) *RepoScanCache {
	m.mu.Lock()
	defer m.mu.Unlock()
	abs, _ := filepath.Abs(rootPath)
	if c, ok := m.caches[abs]; ok {
		return c
	}
	c := &RepoScanCache{RootPath: abs, Entries: make(map[string]CacheEntry)}
	m.caches[abs] = c
	return c
}
```

### 7.3 改造：`service/git.go` `ScanGitRepos`

```go
// GitService 新增字段
type GitService struct {
	gitCmd  *util.GitCommand
	scanCache *ScanCacheManager // 可为 nil（兼容旧调用方）
}

// ScanGitRepos 递归扫描目录下所有 Git 仓库（.git 预筛 + mtime 缓存优化版）
func (s *GitService) ScanGitRepos(rootPath string) []string {
	// root 本身先判定（保持原语义）
	if util.IsGitRepositoryFast(rootPath) {
		return []string{rootPath}
	}

	var cache *RepoScanCache
	if s.scanCache != nil {
		cache = s.scanCache.getCache(rootPath)
		// TTL 过期则清空缓存强制全扫
		if !cache.ScannedAt.IsZero() && time.Since(cache.ScannedAt) > s.scanCache.ttl {
			cache.Entries = make(map[string]CacheEntry)
		}
	}

	var repos []string
	s.scanDirCached(rootPath, &repos, cache)

	if cache != nil {
		cache.ScannedAt = time.Now()
		s.scanCache.save() // 异步落盘
	}
	return repos
}

// scanDirCached 带 mtime 缓存的递归扫描
func (s *GitService) scanDirCached(dir string, repos *[]string, cache *RepoScanCache) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// 获取当前目录 mtime（ReadDir 不直接给，需 os.Stat）
	info, err := os.Stat(dir)
	if err != nil {
		return
	}
	curMtime := info.ModTime()

	// 缓存命中：mtime 未变 -> 沿用缓存，跳过本目录判定
	if cache != nil {
		if entry, hit := cache.Entries[dir]; hit && entry.ModTime.Equal(curMtime) {
			if entry.IsRepo {
				*repos = append(*repos, dir)
			}
			// 注意：此处选择"mtime 未变则跳过整棵子树"
			// 漏扫深层新增的风险由 TTL（5min）+ 手动刷新按钮（PRD F9）兜底
			return
		}
	}

	// 缓存未命中或 mtime 变化：实际判定本目录是否仓库
	// 注意：本函数入参 dir 已是"被遍历到的子目录"，其 .git 判定应在调用处做
	// 此处统一处理：先判定 dir 本身是否仓库
	isRepo := util.IsGitRepositoryFast(dir)
	if cache != nil {
		cache.Entries[dir] = CacheEntry{ModTime: curMtime, IsRepo: isRepo}
	}
	if isRepo {
		*repos = append(*repos, dir)
		return // 仓库目录不再递归
	}

	// 非仓库：递归子目录
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".git" {
			continue
		}
		s.scanDirCached(filepath.Join(dir, entry.Name()), repos, cache)
	}
}
```

> **说明**：上述骨架为示意，实际实现时需注意 `scanDirCached` 对"dir 本身是否仓库"的判定与"遍历子目录"的职责划分，避免与原 `scanDir` 语义偏离。建议保留原 `scanDir` 不动，新增 `scanDirCached` 作为优化路径，由 `ScanGitRepos` 根据 `scanCache` 是否启用选择路径，保证向后兼容（`ScanAndPullRepos` 等旧调用方在未注入缓存时走原逻辑）。

### 7.4 改造：`app.go` 注入缓存管理器

```go
// App 初始化时（app.go startup）
func (a *App) startup(ctx context.Context) {
	// ... 现有逻辑 ...
	dataDir := filepath.Join(".", "data")
	a.gitSvc = service.NewGitServiceWithCache(
		filepath.Join(dataDir, "repo_scan_cache.json"),
	)
}

// 手动刷新入口（PRD F9），forceRescan=true 绕过缓存
func (a *App) RefreshRepoScan(dirPath string) ([]string, error) {
	// 可在 ScanGitRepos 增加 forceRescan 参数，或临时清空对应 rootPath 的缓存
}
```

---

## 八、Caveats / 注意事项

1. **bare repo 漏判**：纯 `.git` 存在性判定无法识别 bare repo（无 `.git` 条目）。桌面应用工作目录场景可忽略；若未来需支持，可加 `git rev-parse --is-bare-repository` 兜底，但会重新引入子进程开销，不推荐。

2. **`FindGitRoot` 已有 bug**：`util/git.go:142` `FindGitRoot` 用 `info.IsDir()` 判定，对 worktree/submodule 漏判。本任务优化 `ScanGitRepos` 时应避免此问题（用 `os.Stat` 无错即判定）。建议后续单独修复 `FindGitRoot`，但**不在本任务范围内**（本任务仅优化扫描）。

3. **mtime 漏扫深层新增**：若用户在 `a/b/c` 深层新增仓库，而 `a/b` 的 mtime 未变（某些 OS/FS 场景），激进跳过子树策略会漏扫。缓解：TTL 5 分钟强制全扫 + 手动刷新按钮（PRD F9）。若需更强一致性，可改为"始终递归遍历，仅用缓存跳过 IsRepo 的 os.Stat"——但这样缓存收益大幅下降（os.Stat 本就极快）。

4. **缓存文件并发写**：多窗口/多操作同时触发扫描时可能并发写 `repo_scan_cache.json`。`ScanCacheManager` 用 `sync.RWMutex` 保护内存，落盘时加读锁快照；建议落盘放在单独 goroutine + channel 串行化，避免并发写冲突。

5. **缓存主键规范化**：缓存 `Entries` 的 key 必须用 `filepath.Abs` 规范化（与 PRD F13、`DirectoryService` 一致），规避大小写/分隔符歧义。Windows 上 `C:\foo` 与 `c:\foo` 需统一。

6. **`ScanGitRepos` 返回 `[]string` 无 error**：现状签名不返回错误，缓存落盘失败应静默降级（仅日志），不影响扫描结果返回。`os.ReadDir`/`os.Stat` 失败也仅跳过该目录，不中断整体扫描。

7. **未读 `service/git_test.go` 全文**：现有测试可能依赖 `IsGitRepository` 的 fork 行为，新增 `IsGitRepositoryFast` 后需补充测试。改造 `ScanGitRepos` 时注意保持旧测试兼容（建议保留无缓存的旧路径作为 fallback）。

8. **并行方案（C）暂不推荐**：纯 os.Stat 下串行已达标，并行徒增复杂度。仅在保留 `git rev-parse` 兜底校验时才考虑，届时可复用 `BatchPull` 的 `sem + WaitGroup` 范式。

9. **未进行实际性能压测**：本研究的开销数值（git fork ~50ms、os.Stat ~0.1ms）基于 Windows 桌面环境经验估算，建议实现后用 `time.Now()` 包裹 `ScanGitRepos` 实测，验证是否达标 NF1（<3s）。

---

## 九、Related Specs / 相关文件

| 文件 | 说明 |
|---|---|
| `service/git.go:214` | `ScanGitRepos` + `scanDir`（待优化） |
| `service/git.go:499` | `BatchPull`（并行范式参考，方案 C 可复用） |
| `util/git.go:80` | `IsGitRepository`（fork git 子进程，瓶颈所在） |
| `util/git.go:142` | `FindGitRoot`（已有 worktree 漏判 bug，参考） |
| `util/json.go` | `LoadJSON/SaveJSON`（缓存持久化复用） |
| `app.go:327` | `ScanAndPullRepos`（调用 `ScanGitRepos`，优化后自动受益） |
| `app.go:40` | `data/directories.json`（缓存文件路径约定参考） |
| `service/directory.go:53` | `filepath.Abs` 规范化范式（缓存主键复用） |
| `.trellis/tasks/07-19-repo-filter-dialog/prd.md` | NF1 性能目标、F9 手动刷新、F12 扫描复用+优化 |
