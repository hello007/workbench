package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"git-manager/model"
	"git-manager/service"
	"git-manager/util"
)

type App struct {
	ctx            context.Context
	directorySvc   *service.DirectoryService
	fileTreeSvc    *service.FileTreeService
	fileOpSvc      *service.FileOperationService
	gitSvc         *service.GitService
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dataDir := "data"
	configPath := filepath.Join(dataDir, "directories.json")

	a.directorySvc = service.NewDirectoryService(configPath)
	a.fileTreeSvc = service.NewFileTreeService()
	a.fileOpSvc = service.NewFileOperationService()
	a.gitSvc = service.NewGitService()

	println("Git Manager started")
}

func (a *App) shutdown(context.Context) {
	println("Git Manager shutting down...")
}

// GetAppVersion 获取应用版本号
func (a *App) GetAppVersion() string {
	return version
}

// GetDirectories 获取所有工作目录
func (a *App) GetDirectories() []*model.Directory {
	directories, err := a.directorySvc.Load()
	if err != nil {
		println("Error:", err.Error())
		return []*model.Directory{}
	}
	return directories
}

// AddDirectory 添加工作目录
func (a *App) AddDirectory(name, path string, isDefault bool) *model.Directory {
	dir, err := a.directorySvc.Create(name, path, isDefault)
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	return dir
}

// UpdateDirectory 更新工作目录
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

// GetDefaultDirectory 获取默认目录
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
