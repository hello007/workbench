package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	a.gitSvc = service.NewGitService()
	a.settingsSvc = service.NewSettingsService(settingsPath)
	a.terminalSvc = service.NewTerminalService(ctx)

	favoritesPath := filepath.Join(dataDir, "favorites.json")
	a.searchSvc = service.NewSearchService()
	a.favoritesSvc = service.NewFavoritesService(favoritesPath)
	a.contentSearchSvc = service.NewContentSearchService()

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

// SaveFile 保存文件内容
func (a *App) SaveFile(filePath string, content string) error {
	return a.fileOpSvc.SaveFile(filePath, content)
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
