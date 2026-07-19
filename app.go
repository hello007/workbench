package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"workbench/model"
	"workbench/service"
	"workbench/util"
)

type App struct {
	ctx              context.Context
	directorySvc     *service.DirectoryService
	fileTreeSvc      *service.FileTreeService
	fileOpSvc        *service.FileOperationService
	gitSvc           *service.GitService
	settingsSvc      *service.SettingsService
	terminalSvc      *service.TerminalService
	searchSvc        *service.SearchService
	favoritesSvc     *service.FavoritesService
	contentSearchSvc *service.ContentSearchService
	updateSvc        *service.UpdateService
	repoMetaSvc      *service.RepoMetaService
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dataDir := "data"
	configPath := filepath.Join(dataDir, "directories.json")
	settingsPath := filepath.Join(dataDir, "settings.json")

	a.directorySvc = service.NewDirectoryService(configPath)
	a.fileTreeSvc = service.NewFileTreeService()
	a.fileOpSvc = service.NewFileOperationService()
	// 注入扫描缓存（.git 预筛 + mtime 缓存优化，PRD F12），让 ScanGitRepos 与一键更新同步受益
	a.gitSvc = service.NewGitServiceWithCache(filepath.Join(dataDir, "repo_scan_cache.json"))
	a.settingsSvc = service.NewSettingsService(settingsPath)
	a.terminalSvc = service.NewTerminalService(ctx)

	favoritesPath := filepath.Join(dataDir, "favorites.json")
	a.searchSvc = service.NewSearchService()
	a.favoritesSvc = service.NewFavoritesService(favoritesPath)
	a.contentSearchSvc = service.NewContentSearchService()

	// 仓库筛选器元数据服务（简述/标签持久化，PRD F10）
	a.repoMetaSvc = service.NewRepoMetaService(filepath.Join(dataDir, "repo_meta.json"))

	// 更新服务
	a.updateSvc = service.NewUpdateService()
	a.updateSvc.SetContext(ctx)

	// 检查是否有待应用的更新（上次下载但未重启）
	// 如果有待更新文件，会启动批处理脚本替换 exe 后启动新版本，
	// 当前旧进程需要退出，避免同时运行两个实例
	if hasPending, _ := a.updateSvc.CheckPendingUpdate(); hasPending {
		println("发现待更新文件，正在应用更新并退出...")
		os.Exit(0)
	}

	println("WorkBench started")
}

func (a *App) shutdown(context.Context) {
	if a.terminalSvc != nil {
		a.terminalSvc.CloseAll()
	}
	println("WorkBench shutting down...")
}

// GetAppVersion 获取应用版本号
func (a *App) GetAppVersion() string {
	return version
}

// GetDirectories 获取所有工作目录。
// 启动关键路径：直接返回 Load() 结果，不再同步检测 IsGitRepo（避免 N 次子进程阻塞 UI 渲染）。
// IsGitRepo 取自 directories.json 持久化值（Create/Update 时写入，RefreshDirectoriesGitFlag 启动后异步刷新）。
func (a *App) GetDirectories() []*model.Directory {
	directories, err := a.directorySvc.Load()
	if err != nil {
		println("Error:", err.Error())
		return []*model.Directory{}
	}
	return directories
}

// RefreshDirectoriesGitFlag 重新检测所有工作目录的 IsGitRepo，回写 directories.json，返回刷新后的列表。
// 启动后由前端异步调用，覆盖"目录后来纳管为 git 仓库"等变化。
// 关键：基于最新 Load 合并——只更新 IsGitRepo 字段，保留其他字段最新值，
// 规避"刷新期间用户 AddDirectory，刷新用旧快照 Save 覆盖新目录"的并发竞态。
func (a *App) RefreshDirectoriesGitFlag() []*model.Directory {
	// 1. 基于最新 Load（不使用任何旧快照）
	directories, err := a.directorySvc.Load()
	if err != nil {
		println("Error:", err.Error())
		return []*model.Directory{}
	}
	gitCmd := util.NewGitCommand()
	// 2. 只更新 IsGitRepo 字段（其他字段保留 Load 的最新值）
	for _, d := range directories {
		d.IsGitRepo = gitCmd.IsGitRepository(d.Path)
		if d.IsGitRepo {
			// 同步刷新 HasRemote，供前端灰色图标区分无远程仓库
			_, _, err := gitCmd.GetRemote(d.Path)
			d.HasRemote = err == nil
		}
	}
	// 3. Save 回写（基于最新 Load 的合并结果）
	if err := a.directorySvc.Save(directories); err != nil {
		println("Error:", err.Error())
	}
	return directories
}

// AddDirectory 添加工作目录。
// IsGitRepo 由 service.Create 在持久化时计算并写入 directories.json，此处直接返回。
func (a *App) AddDirectory(name, path string, isDefault bool) *model.Directory {
	dir, err := a.directorySvc.Create(name, path, isDefault)
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	return dir
}

// UpdateDirectory 更新工作目录。
// IsGitRepo 由 service.Update 在持久化时重算并写入 directories.json，此处直接返回。
func (a *App) UpdateDirectory(id, name, path string, isDefault bool) *model.Directory {
	dir, err := a.directorySvc.Update(id, name, path, isDefault)
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	return dir
}

// DeleteDirectory 删除工作目录
func (a *App) DeleteDirectory(id string) bool {
	err := a.directorySvc.Delete(id)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// SetDefaultDirectory 设置默认目录
func (a *App) SetDefaultDirectory(id string) bool {
	err := a.directorySvc.SetDefault(id)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// GetDefaultDirectory 获取默认目录。
// 读方法不触发检测，直接返回 Load 的持久化结果。
func (a *App) GetDefaultDirectory() *model.Directory {
	dir, err := a.directorySvc.GetDefault()
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	return dir
}

// ReorderDirectories 重排工作目录顺序
func (a *App) ReorderDirectories(ids []string) bool {
	err := a.directorySvc.Reorder(ids)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// GetFileTree 获取文件树
func (a *App) GetFileTree(path string) []*model.FileTreeNode {
	nodes, err := a.fileTreeSvc.GetChildren(path)
	if err != nil {
		println("Error:", err.Error())
		return []*model.FileTreeNode{}
	}
	return nodes
}

// GetFileTreeRecursive 获取完整树
func (a *App) GetFileTreeRecursive(path string, maxDepth int) []*model.FileTreeNode {
	nodes, err := a.fileTreeSvc.GetTree(path, maxDepth)
	if err != nil {
		println("Error:", err.Error())
		return []*model.FileTreeNode{}
	}
	return nodes
}

// GetGitInfo 获取Git信息
func (a *App) GetGitInfo(path string) *model.GitRepoInfo {
	info, err := a.fileTreeSvc.GetGitInfo(path)
	if err != nil {
		println("Error:", err.Error())
		return &model.GitRepoInfo{
			Path:   path,
			IsRepo: false,
		}
	}
	return info
}

// CreateDirectory 创建文件夹
func (a *App) CreateDirectory(parentPath, name string) bool {
	err := a.fileOpSvc.CreateDirectory(parentPath, name)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// CreateFile 创建文件
func (a *App) CreateFile(parentPath, name, content string) bool {
	err := a.fileOpSvc.CreateFile(parentPath, name, content)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// RenameFile 重命名
func (a *App) RenameFile(oldPath, newName string) bool {
	err := a.fileOpSvc.Rename(oldPath, newName)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// DeleteFile 删除
func (a *App) DeleteFile(path string) bool {
	err := a.fileOpSvc.Delete(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// PreviewFile 预览文件
func (a *App) PreviewFile(filePath string) *model.FilePreview {
	const maxSize = 1024 * 1024 // 1MB
	preview, err := a.fileOpSvc.PreviewFile(filePath, maxSize)
	if err != nil {
		preview.Error = err.Error()
	}
	return preview
}

// ReadFileBytes 读取文件原始字节（base64），供前端构造 Blob 预览图片/PDF/Office
func (a *App) ReadFileBytes(filePath string) *model.FileBytes {
	const maxSize = 50 * 1024 * 1024 // 50MB（图片/PDF/Office 放宽上限）
	result, err := a.fileOpSvc.ReadFileBytes(filePath, maxSize)
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

// SaveFile 保存文件内容（按 encoding 指定编码写入：gbk 按 GBK，其余按 UTF-8）
func (a *App) SaveFile(filePath string, content string, encoding string) error {
	return a.fileOpSvc.SaveFile(filePath, content, encoding)
}

// GetGitLog 获取提交历史
func (a *App) GetGitLog(dirPath string, page, pageSize int) *model.PageResult {
	result, err := a.gitSvc.GetLog(dirPath, page, pageSize)
	if err != nil {
		println("Error:", err.Error())
		return model.NewPageResult([]model.GitCommit{}, 0, page, pageSize)
	}
	return result
}

// CloneRepo 克隆仓库
func (a *App) CloneRepo(url, targetPath string) string {
	repoName := a.gitSvc.ExtractRepoName(url)
	fullPath := filepath.Join(targetPath, repoName)

	info, _ := a.gitSvc.GetInfo(fullPath)
	if info.IsRepo {
		return "错误: Git仓库已存在"
	}

	_, err := a.gitSvc.Clone(url, fullPath)
	if err != nil {
		return "错误: " + err.Error()
	}

	return "克隆成功"
}

// PullRepo 拉取更新
func (a *App) PullRepo(dirPath string) string {
	if !util.NewGitCommand().IsGitRepository(dirPath) {
		return "错误: 不是Git仓库"
	}
	if !a.gitSvc.HasRemote(dirPath) {
		return "该仓库未配置远程，无需拉取"
	}
	output, err := a.gitSvc.Pull(dirPath)
	if err != nil {
		return "错误: " + err.Error()
	}
	return output
}

// ScanAndPullRepos 扫描并批量拉取 Git 仓库
func (a *App) ScanAndPullRepos(dirPath string) (*model.PullSummary, error) {
	repos := a.gitSvc.ScanGitRepos(dirPath)
	if len(repos) == 0 {
		return nil, fmt.Errorf("未找到任何 Git 仓库")
	}

	summary := &model.PullSummary{Total: len(repos)}

	go func() {
		a.gitSvc.BatchPull(repos, 5, a.ctx)
	}()

	return summary, nil
}

// ExtractRepoName 提取仓库名
func (a *App) ExtractRepoName(url string) string {
	return a.gitSvc.ExtractRepoName(url)
}

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

// GetGitRemoteURL 获取 Git 仓库的远程地址和当前分支信息
func (a *App) GetGitRemoteURL(path string) (*model.GitRemoteInfo, error) {
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}

	gitRoot, err := util.FindGitRoot(path)
	if err != nil {
		return nil, fmt.Errorf("无法打开 Git 仓库: %w", err)
	}

	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("无法打开 Git 仓库: %w", err)
	}

	// Get remote configuration
	remote, err := repo.Remote("origin")
	if err != nil {
		// No origin remote, return empty info
		return &model.GitRemoteInfo{
			RemoteURL:  "",
			Branch:     "",
			IsDetached: false,
		}, nil
	}

	// Get remote URL
	remoteURL := ""
	if len(remote.Config().URLs) > 0 {
		remoteURL = remote.Config().URLs[0]
	}

	// Get current HEAD reference
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("无法获取 HEAD 引用: %w", err)
	}

	// Check if detached HEAD
	branchName := head.Name().Short()
	isDetached := !head.Name().IsBranch()

	return &model.GitRemoteInfo{
		RemoteURL:  remoteURL,
		Branch:     branchName,
		IsDetached: isDetached,
	}, nil
}

// GetCommitHistory 获取 Git 仓库的提交历史
func (a *App) GetCommitHistory(path string, limit int, offset int) ([]model.Commit, error) {
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	gitRoot, err := util.FindGitRoot(path)
	if err != nil {
		return nil, fmt.Errorf("无法打开 Git 仓库: %w", err)
	}

	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("无法打开 Git 仓库: %w", err)
	}

	// 获取提交日志迭代器
	commitIter, err := repo.Log(&git.LogOptions{
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("无法获取提交历史: %w", err)
	}
	defer commitIter.Close()

	// 跳过 offset 个提交
	for i := 0; i < offset; i++ {
		_, err := commitIter.Next()
		if err != nil {
			break
		}
	}

	// 收集指定数量的提交
	commits := make([]model.Commit, 0, limit)
	for i := 0; i < limit; i++ {
		commitObj, err := commitIter.Next()
		if err != nil {
			break
		}

		commit := model.Commit{
			SHA:       commitObj.Hash.String(),
			ShortSHA:  commitObj.Hash.String()[:8],
			Message:   commitObj.Message,
			Author:    commitObj.Author.Name,
			Email:     commitObj.Author.Email,
			Timestamp: commitObj.Author.When.Unix(),
			DateTime:  commitObj.Author.When.Format("2006-01-02 15:04:05"),
		}

		files := getCommitFiles(repo, commitObj)
		commit.Files = files

		commits = append(commits, commit)
	}

	return commits, nil
}

// getCommitFiles 获取提交中变更的文件列表
func getCommitFiles(repo *git.Repository, commit *object.Commit) []string {
	var files []string

	currentTree, err := commit.Tree()
	if err != nil {
		return files
	}

	parentCommit, err := commit.Parent(0)
	if err != nil {
		return getTreeFiles(currentTree)
	}

	parentTree, err := parentCommit.Tree()
	if err != nil {
		return files
	}

	patch, err := currentTree.Patch(parentTree)
	if err != nil {
		return files
	}

	for _, patchObj := range patch.FilePatches() {
		from, to := patchObj.Files()
		if from != nil {
			files = append(files, from.Path())
		} else if to != nil {
			files = append(files, to.Path())
		}
	}

	return files
}

// getTreeFiles 获取树中的文件路径（最多返回100个）
func getTreeFiles(tree *object.Tree) []string {
	files := make([]string, 0, 100)
	count := 0
	tree.Files().ForEach(func(file *object.File) error {
		if count >= 100 {
			return fmt.Errorf("limit reached")
		}
		files = append(files, file.Name)
		count++
		return nil
	})
	return files
}

// OpenInExplorer 在资源管理器中打开
func (a *App) OpenInExplorer(path string) bool {
	err := a.fileOpSvc.OpenInExplorer(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// OpenInVSCode 用 VSCode 打开
func (a *App) OpenInVSCode(path string) bool {
	err := a.fileOpSvc.OpenInVSCode(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// OpenInWarp 用 Warp 终端打开
func (a *App) OpenInWarp(path string) bool {
	err := a.fileOpSvc.OpenInWarp(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// OpenInObsidian 用 Obsidian 打开指定路径对应的 vault（文件夹->自身，文件->父目录）。
// 若设置中配置了 Obsidian 程序路径则优先使用，否则走系统协议方案。
// 返回状态码：""=成功、"not-installed"=未检测到 Obsidian、"not-registered"=路径未注册为 vault。
// 其他错误记日志并返回 "not-installed" 兜底，确保前端可处理。
func (a *App) OpenInObsidian(path string) string {
	var obsidianPath string
	if settings, err := a.settingsSvc.Load(); err == nil {
		obsidianPath = settings.ObsidianPath
	}
	err := a.fileOpSvc.OpenInObsidian(path, obsidianPath)
	if err == nil {
		return ""
	}
	if errors.Is(err, service.ErrVaultNotRegistered) {
		return "not-registered"
	}
	if errors.Is(err, service.ErrObsidianNotInstalled) {
		return "not-installed"
	}
	println("Error:", err.Error())
	return "not-installed"
}

// OpenObsidianVaultManager 打开 Obsidian 仓库管理器（obsidian://choose-vault），
// 供用户手动将目录添加为 vault。成功返回 true。
func (a *App) OpenObsidianVaultManager() bool {
	var obsidianPath string
	if settings, err := a.settingsSvc.Load(); err == nil {
		obsidianPath = settings.ObsidianPath
	}
	err := a.fileOpSvc.OpenObsidianVaultManager(obsidianPath)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// CopyObsidianVaultPath 将路径对应的 vault 路径文本复制到系统剪贴板。
// vault 解析：文件夹->自身，文件->父目录。成功返回 true。
// 供「打开仓库管理器」前复制路径，便于用户在 Obsidian 路径栏粘贴。
func (a *App) CopyObsidianVaultPath(path string) bool {
	err := a.fileOpSvc.CopyObsidianVaultPath(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// AutoRegisterAndOpen 自动将路径对应目录注册为 Obsidian vault 并打开。
// 返回状态码：""=成功、"running"=Obsidian 运行中（需关闭后重试）、"not-installed"=未检测到 Obsidian、"failed"=其他失败。
// 其他错误记日志并返回 "failed"，确保前端可分流提示。
func (a *App) AutoRegisterAndOpen(path string) string {
	var obsidianPath string
	if settings, err := a.settingsSvc.Load(); err == nil {
		obsidianPath = settings.ObsidianPath
	}
	err := a.fileOpSvc.AutoRegisterAndOpen(path, obsidianPath)
	if err == nil {
		return ""
	}
	if errors.Is(err, service.ErrObsidianRunning) {
		return "running"
	}
	if errors.Is(err, service.ErrObsidianNotInstalled) {
		return "not-installed"
	}
	println("Error:", err.Error())
	return "failed"
}

// OpenWithDefaultApp 用系统默认程序打开文件
func (a *App) OpenWithDefaultApp(path string) bool {
	err := a.fileOpSvc.OpenWithDefaultApp(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// CopyItem 复制文件或文件夹
func (a *App) CopyItem(sourcePath, targetDir string) string {
	result, err := a.fileOpSvc.CopyItem(sourcePath, targetDir)
	if err != nil {
		return "错误: " + err.Error()
	}
	return result
}

// MoveItem 移动文件或文件夹
func (a *App) MoveItem(sourcePath, targetDir string) string {
	result, err := a.fileOpSvc.MoveItem(sourcePath, targetDir)
	if err != nil {
		return "错误: " + err.Error()
	}
	return result
}

// CopyTo 将文件或文件夹拷贝到指定目标目录
func (a *App) CopyTo(sourcePath, targetPath string, copyWholeDir bool) string {
	result, err := a.fileOpSvc.CopyTo(sourcePath, targetPath, copyWholeDir)
	if err != nil {
		println("Error:", err.Error())
		return "错误: " + err.Error()
	}
	return result
}

// CopyToSystemClipboard 写入系统剪贴板（复制模式）
func (a *App) CopyToSystemClipboard(path string) string {
	err := a.fileOpSvc.CopyToSystemClipboard([]string{path})
	if err != nil {
		println("Error:", err.Error())
		return "错误: " + err.Error()
	}
	return ""
}

// CutToSystemClipboard 写入系统剪贴板（剪切模式）
func (a *App) CutToSystemClipboard(path string) string {
	err := a.fileOpSvc.CutToSystemClipboard([]string{path})
	if err != nil {
		println("Error:", err.Error())
		return "错误: " + err.Error()
	}
	return ""
}

// ReadFromSystemClipboard 读取系统剪贴板文件列表
func (a *App) ReadFromSystemClipboard() string {
	paths, isCut, err := a.fileOpSvc.ReadFromSystemClipboard()
	if err != nil {
		println("Error:", err.Error())
		return ""
	}
	if len(paths) == 0 {
		return ""
	}
	data, _ := json.Marshal(map[string]interface{}{
		"paths": paths,
		"isCut": isCut,
	})
	return string(data)
}

// GetLocalChanges 获取仓库本地变动文件列表
func (a *App) GetLocalChanges(path string) ([]model.FileChange, error) {
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.GetLocalChanges(path)
}

// DiscardChanges 回滚本地变动，filePaths 为空则回滚全部
func (a *App) DiscardChanges(path string, filePaths []string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.DiscardChanges(path, filePaths)
}

// CommitFiles 选择性提交（pathspec 语义）：仅提交 filePaths 中的文件，不影响 index 中其他已暂存文件。
func (a *App) CommitFiles(path, message string, filePaths []string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.Commit(path, message, filePaths)
}

// PushRepo 推送当前分支到远程。setUpstream=true 时执行 git push --set-upstream origin <branch>。
// 返回 git stdout 用于结果展示。
func (a *App) PushRepo(path string, setUpstream bool) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.Push(path, setUpstream)
}

// GetFileDiff 获取单个文件的 unified diff 文本（已跟踪 vs HEAD，未跟踪显示为新增全文）。
func (a *App) GetFileDiff(path, file string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	if file == "" {
		return "", fmt.Errorf("文件路径不能为空")
	}
	return a.gitSvc.GetDiff(path, file)
}

// HasUpstream 判断当前分支是否配置了上游跟踪分支。
func (a *App) HasUpstream(path string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.HasUpstream(path)
}

// GetBranches 获取仓库分支列表
func (a *App) GetBranches(path string) (*model.BranchList, error) {
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.GetBranches(path)
}

// CheckoutBranch 切换分支
func (a *App) CheckoutBranch(path string, branchName string, isRemote bool) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.CheckoutBranch(path, branchName, isRemote)
}

// GetSettings 获取应用设置
func (a *App) GetSettings() *model.AppSettings {
	settings, err := a.settingsSvc.Load()
	if err != nil {
		return &model.AppSettings{}
	}
	return settings
}

// SaveSettings 保存应用设置
func (a *App) SaveSettings(settings *model.AppSettings) error {
	return a.settingsSvc.Save(settings)
}

// ===== 终端相关 =====

// CreateTerminal 创建终端会话
func (a *App) CreateTerminal(dir, shellType string, cols, rows uint16) (string, error) {
	var customPath string
	settings, err := a.settingsSvc.Load()
	if err == nil {
		switch shellType {
		case "gitbash":
			customPath = settings.GitBashPath
		}
	}
	return a.terminalSvc.CreateTerminal(dir, shellType, customPath, cols, rows)
}

// WriteTerminalInput 向终端写入用户输入
func (a *App) WriteTerminalInput(sessionID, input string) error {
	return a.terminalSvc.WriteInput(sessionID, input)
}

// ChangeTerminalDir 切换终端工作目录
func (a *App) ChangeTerminalDir(sessionID, dir string) error {
	return a.terminalSvc.ChangeDir(sessionID, dir)
}

// ResizeTerminal 调整终端窗口大小
func (a *App) ResizeTerminal(sessionID string, cols, rows uint16) error {
	return a.terminalSvc.Resize(sessionID, cols, rows)
}

// CloseTerminal 关闭终端会话
func (a *App) CloseTerminal(sessionID string) error {
	return a.terminalSvc.CloseTerminal(sessionID)
}

// GetShellConfigs 获取可用的 Shell 配置列表
func (a *App) GetShellConfigs() []model.ShellConfig {
	return model.GetShellConfigs()
}

// ===== 搜索相关 =====

// SearchFiles 搜索文件
func (a *App) SearchFiles(rootDir, query string, maxResults int) []*model.SearchResult {
	results, err := a.searchSvc.Search(rootDir, query, maxResults)
	if err != nil {
		println("SearchFiles error:", err.Error())
		return []*model.SearchResult{}
	}
	return results
}

// ContentSearch 内容搜索
// keyword: 搜索关键词
// fileExt: 文件类型过滤（如 ".java"，为空则不过滤）
// subDir: 子目录路径（相对于工作目录，为空则搜索整个目录）
// searchAll: 是否搜索所有工作目录（true 搜索全部，false 仅当前目录）
func (a *App) ContentSearch(keyword, fileExt, subDir string, searchAll bool) ([]*model.ContentSearchGroup, error) {
	if keyword == "" {
		return nil, nil
	}

	// 加载设置获取排除配置
	settings, _ := a.settingsSvc.Load()

	// 确定搜索目录列表
	var dirs []string
	var repoNames []string

	if searchAll {
		directories, err := a.directorySvc.Load()
		if err != nil {
			return nil, err
		}
		for _, d := range directories {
			dirs = append(dirs, d.Path)
			repoNames = append(repoNames, d.Name)
		}
	} else {
		// 仅当前选中的工作目录
		directories, _ := a.directorySvc.Load()
		for _, d := range directories {
			if d.IsDefault {
				dirs = append(dirs, d.Path)
				repoNames = append(repoNames, d.Name)
				break
			}
		}
		// 如果没有默认目录，用第一个
		if len(dirs) == 0 && len(directories) > 0 {
			dirs = append(dirs, directories[0].Path)
			repoNames = append(repoNames, directories[0].Name)
		}
	}

	if len(dirs) == 0 {
		return nil, nil
	}

	// 全局搜索超时 60s，单目录 10s
	timeout := 10
	if searchAll {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	return a.contentSearchSvc.ContentSearch(
		ctx, dirs, repoNames,
		keyword, fileExt, subDir,
		settings.SearchExcludeDirs, settings.SearchExcludeFiles,
		20,
	)
}

// ===== 收藏相关 =====

// GetFavorites 获取所有收藏
func (a *App) GetFavorites() []*model.Favorite {
	favorites, err := a.favoritesSvc.Load()
	if err != nil {
		println("GetFavorites error:", err.Error())
		return []*model.Favorite{}
	}
	return favorites
}

// AddFavorite 添加收藏
func (a *App) AddFavorite(path, alias, group string) string {
	err := a.favoritesSvc.Add(path, alias, group)
	if err != nil {
		return err.Error()
	}
	return ""
}

// RemoveFavorite 移除收藏
func (a *App) RemoveFavorite(path string) string {
	err := a.favoritesSvc.Remove(path)
	if err != nil {
		return err.Error()
	}
	return ""
}

// UpdateFavoriteAlias 更新收藏别名
func (a *App) UpdateFavoriteAlias(path, alias string) string {
	err := a.favoritesSvc.UpdateAlias(path, alias)
	if err != nil {
		return err.Error()
	}
	return ""
}

// UpdateFavoriteGroup 更新收藏分组
func (a *App) UpdateFavoriteGroup(path, group string) string {
	err := a.favoritesSvc.UpdateGroup(path, group)
	if err != nil {
		return err.Error()
	}
	return ""
}

// ===== 更新相关 =====

// CheckForUpdate 检查是否有新版本
func (a *App) CheckForUpdate() (*model.UpdateInfo, error) {
	return a.updateSvc.CheckForUpdate(version)
}

// DownloadUpdate 下载新版本
func (a *App) DownloadUpdate(downloadURL string) error {
	return a.updateSvc.DownloadUpdate(downloadURL)
}

// CancelDownload 取消下载
func (a *App) CancelDownload() {
	a.updateSvc.CancelDownload()
}

// ApplyUpdate 执行更新替换并退出应用
func (a *App) ApplyUpdate() error {
	err := a.updateSvc.ApplyUpdate()
	if err != nil {
		return err
	}
	// 退出当前应用
	os.Exit(0)
	return nil
}
