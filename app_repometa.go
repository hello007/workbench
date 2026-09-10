package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"workbench/model"
	"workbench/service"
	"workbench/util"
)

// ===== 仓库筛选器相关 =====

// GetRepoFilterList 获取仓库筛选器列表（PRD F1~F19）。
// 流程：按 dirId 查 Directory.Path -> ScanGitRepos 扫描（.git 预筛 + mtime 缓存）
// -> 批量检测远程 -> 加载 RepoMeta 合并（简述/标签/失效标记）-> 解析 README 摘要（缓存到 ReadmeSummary）。
// 路径不在扫描结果中的元数据标记 Missing=true（灰显），不自动删除。
// 采用 mtime 缓存，二次打开近乎瞬时；手动刷新请用 RefreshRepoFilterList。
func (a *App) GetRepoFilterList(dirId string) []*model.RepoFilterItem {
	dir := a.findDirectoryById(dirId)
	if dir == nil {
		return []*model.RepoFilterItem{}
	}
	return a.buildRepoFilterList(dir, false)
}

// RefreshRepoFilterList 手动刷新仓库筛选器列表（PRD F9）。
// 清除该工作目录的扫描缓存后强制全量重扫，并重新解析所有 README 摘要。
func (a *App) RefreshRepoFilterList(dirId string) []*model.RepoFilterItem {
	dir := a.findDirectoryById(dirId)
	if dir == nil {
		return []*model.RepoFilterItem{}
	}
	return a.buildRepoFilterList(dir, true)
}

// findDirectoryById 按 id 查找工作目录，未找到返回 nil。
func (a *App) findDirectoryById(id string) *model.Directory {
	directories, err := a.directorySvc.Load()
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	for _, d := range directories {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// normalizeRepoPathForApp 规范化路径为绝对路径，作为元数据主键（与 service.RepoMetaService 一致）。
// 规范化失败时原样返回。
func normalizeRepoPathForApp(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// isPathUnder 判断 child 路径是否位于 parent 目录下（含相等）。
// 规范化：filepath.Abs + ToSlash + ToLower，兼容 Windows 大小写与分隔符差异。
// 用于将全局存储的 RepoMeta（按 path 主键）限定到当前工作目录范围内，
// 避免其他工作目录的已编辑仓库以失效状态混入当前列表。
func isPathUnder(child, parent string) bool {
	c, err := filepath.Abs(child)
	if err != nil {
		c = child
	}
	p, err := filepath.Abs(parent)
	if err != nil {
		p = parent
	}
	cs := strings.ToLower(filepath.ToSlash(c))
	ps := strings.ToLower(filepath.ToSlash(p))
	if cs == ps {
		return true
	}
	return strings.HasPrefix(cs, ps+"/")
}

// buildRepoFilterList 构建仓库筛选器列表的核心逻辑。
// forceRescan=true 时清除扫描缓存并重新解析所有 README（手动刷新）；false 时走 mtime 缓存。
//
// 并发安全：扫描与 README 解析在锁外执行（耗时 IO），合并+落盘通过 repoMetaSvc.Mutate
// 在互斥锁保护下原子完成，规避与 SaveRepoMeta 防抖保存的读-改-写竞态
// （旧实现 Load+修改+Save 非原子，并发 SaveRepoMeta 的旧快照会覆盖扫描刚写入的 ReadmeSummary）。
// 持久化策略：仅当元数据实际变化（新增仓库/README 首次或刷新解析/失效状态变更）时落盘，
// 常规重复打开（缓存命中、无失效变化）不写盘。
// 落盘时保留从磁盘加载的最新 Summary/Tags，仅更新扫描归属字段（ReadmeSummary/Missing/LastScanAt）。
func (a *App) buildRepoFilterList(dir *model.Directory, forceRescan bool) []*model.RepoFilterItem {
	if dir == nil || dir.Path == "" {
		return []*model.RepoFilterItem{}
	}

	// 规范化根路径为绝对路径，保证扫描结果与元数据主键一致（PRD F13）
	rootPath, err := filepath.Abs(dir.Path)
	if err != nil {
		rootPath = dir.Path
	}

	if forceRescan {
		a.gitSvc.ClearScanCache(rootPath)
	}

	// 1. 扫描仓库（.git 预筛 + mtime 缓存）—— 锁外
	repoPaths := a.gitSvc.ScanGitRepos(rootPath)

	// 2. 批量检测远程配置（go-git 并发，不 fork 子进程，保证 NF1）—— 锁外
	hasRemoteMap := a.gitSvc.HasRemotesBatch(repoPaths)

	// 3. 只读快照元数据，判定哪些仓库需要（重新）解析 README —— 锁外
	//    SaveRepoMeta 仅改 Summary/Tags/UpdatedAt，不改 LastScanAt/ReadmeSummary/Missing，
	//    故该快照的 readmeNeeded 判定（依赖 LastScanAt/meta 是否存在）在后续 Mutate 重新加载时仍成立。
	metaSnapshot, err := a.repoMetaSvc.Load()
	if err != nil {
		println("Error:", err.Error())
		metaSnapshot = make(map[string]*model.RepoMeta)
	}
	readmeNeeded := make(map[string]bool, len(repoPaths))
	for _, repoPath := range repoPaths {
		normalized := normalizeRepoPathForApp(repoPath)
		meta := metaSnapshot[normalized]
		if meta == nil || meta.LastScanAt.IsZero() || forceRescan {
			readmeNeeded[repoPath] = true
		}
	}

	// 4. 解析需要的 README 摘要（磁盘读）—— 锁外
	readmeResults := make(map[string]string, len(readmeNeeded))
	for repoPath := range readmeNeeded {
		readmeResults[repoPath] = service.ParseReadmeSummary(repoPath)
	}

	// 5. 在互斥锁保护下合并扫描结果与元数据，构建列表并按需落盘
	items := make([]*model.RepoFilterItem, 0, len(repoPaths)+len(metaSnapshot))
	now := time.Now()
	mutateErr := a.repoMetaSvc.Mutate(func(metaMap map[string]*model.RepoMeta) (bool, error) {
		scannedSet := make(map[string]bool, len(repoPaths))
		dirty := forceRescan // 强制刷新始终落盘（更新 LastScanAt 缓存标记）

		// 合并扫描结果与元数据
		for _, repoPath := range repoPaths {
			normalized := normalizeRepoPathForApp(repoPath)
			scannedSet[normalized] = true
			meta := metaMap[normalized]

			var readmeSummary string
			if meta != nil {
				readmeSummary = meta.ReadmeSummary
			}
			if readmeNeeded[repoPath] {
				// 重新解析：更新 ReadmeSummary 与 LastScanAt 缓存标记，需落盘持久化
				readmeSummary = readmeResults[repoPath]
				dirty = true
			}

			// 新建或更新元数据（仅扫描归属字段，保留用户 Summary/Tags）
			if meta == nil {
				meta = &model.RepoMeta{Path: normalized}
				metaMap[normalized] = meta
				dirty = true
			}
			if readmeNeeded[repoPath] {
				meta.ReadmeSummary = readmeSummary
				meta.LastScanAt = now
			}
			if meta.Missing {
				meta.Missing = false // 路径重新出现，清除失效标记
				dirty = true
			}

			items = append(items, &model.RepoFilterItem{
				Name:          filepath.Base(repoPath),
				Path:          repoPath,
				Summary:       meta.Summary,
				Tags:          meta.Tags,
				ReadmeSummary: readmeSummary,
				Missing:       false,
				HasRemote:     hasRemoteMap[repoPath],
				IsGitRepo:     true,
			})
		}

		// 处理失效仓库（在元数据中但不在扫描结果中）
		for normalized, meta := range metaMap {
			if meta == nil || scannedSet[normalized] {
				continue
			}
			// 仅处理当前工作目录范围内的失效记录，避免其他工作目录的已编辑仓库以失效状态出现。
			// repo_meta.json 按 path 全局存储（path 全局唯一且仓库可能被嵌套工作目录包含），
			// 故查询时按 rootPath 范围过滤，实现工作目录隔离。
			if !isPathUnder(meta.Path, rootPath) {
				continue
			}
			if !meta.Missing {
				meta.Missing = true
				dirty = true
			}
			items = append(items, &model.RepoFilterItem{
				Name:          filepath.Base(meta.Path),
				Path:          meta.Path,
				Summary:       meta.Summary,
				Tags:          meta.Tags,
				ReadmeSummary: meta.ReadmeSummary,
				Missing:       true,
				HasRemote:     false, // 路径失效无法检测远程
				IsGitRepo:     true,
			})
		}

		return dirty, nil
	})
	if mutateErr != nil {
		println("Error:", mutateErr.Error())
	}

	return items
}

// SaveRepoMeta 保存仓库元数据（用户自定义简述与标签，PRD F10/F16）。
// 路径内部 filepath.Abs 规范化后作主键；由前端防抖（简述 800ms）/即时（标签增删）调用。
// 通过 Mutate 原子读-改-写，保留 LastScanAt/ReadmeSummary/Missing，仅更新用户字段，
// 并规避与扫描合并保存的竞态。
func (a *App) SaveRepoMeta(path string, summary string, tags []string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	normalized := normalizeRepoPathForApp(path)
	return a.repoMetaSvc.Mutate(func(metaMap map[string]*model.RepoMeta) (bool, error) {
		meta := metaMap[normalized]
		if meta == nil {
			meta = &model.RepoMeta{Path: normalized}
			metaMap[normalized] = meta
		}
		meta.Summary = summary
		meta.Tags = tags
		meta.UpdatedAt = time.Now()
		return true, nil
	})
}

// CleanMissingRepoMeta 清理失效仓库元数据（PRD F15），返回清理数量。
// 失效判定：元数据记录的路径当前已不存在（os.Stat 失败），不依赖扫描结果。
// 通过 Mutate 原子读-改-写，规避与扫描/保存的竞态。
func (a *App) CleanMissingRepoMeta() (int, error) {
	removed := 0
	err := a.repoMetaSvc.Mutate(func(metaMap map[string]*model.RepoMeta) (bool, error) {
		for key, meta := range metaMap {
			if meta == nil {
				continue
			}
			if !util.FileExists(meta.Path) {
				delete(metaMap, key)
				removed++
			}
		}
		return removed > 0, nil
	})
	return removed, err
}

// GetRepoReadme 返回指定仓库根目录下 README 的完整文本（不截断，供二级弹窗渲染）。
// 复用 service.ReadFullReadme：路径校验（须存在且为目录）+ findReadme 定位
// + ReadFileSafe 读取（上限 1MB）+ DetectTextEncoding 转 UTF-8。
// 无 README / 二进制 / 编码不可识别 / 路径非目录 均返回空串（前端据此禁用"查看完整 README"）。
func (a *App) GetRepoReadme(repoPath string) string {
	return service.ReadFullReadme(repoPath)
}
