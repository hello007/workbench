package main

import (
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"workbench/model"
	"workbench/util"
)

// ===== Git 操作域 =====

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

// ===== 标签管理域 =====

// GetTags 获取仓库标签列表
func (a *App) GetTags(path string) ([]model.GitTag, error) {
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.ListTags(path)
}

// CreateTag 创建标签（message 为空创建轻量标签，非空创建注释标签），仅钉 HEAD
func (a *App) CreateTag(path, name, message string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.CreateTag(path, name, message)
}

// DeleteTag 删除本地标签
func (a *App) DeleteTag(path, name string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.DeleteTag(path, name)
}

// PushTag 推送单个标签到远程 origin，返回 git stdout
func (a *App) PushTag(path, name string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.PushTag(path, name)
}

// ===== 远程仓库管理域 =====

// GetRemotes 获取远程仓库列表（名称 + URL）
func (a *App) GetRemotes(path string) ([]model.GitRemote, error) {
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.ListRemotes(path)
}

// AddRemote 新增远程仓库
func (a *App) AddRemote(path, name, url string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.AddRemote(path, name, url)
}

// RemoveRemote 删除远程仓库
func (a *App) RemoveRemote(path, name string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.RemoveRemote(path, name)
}

// FetchRepo 拉取远程更新（remote 为空时对所有远程执行，prune 控制是否清理远端已删分支）
func (a *App) FetchRepo(path, remote string, prune bool) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.Fetch(path, remote, prune)
}

// SetBranchUpstream 为指定分支设置上游跟踪分支
func (a *App) SetBranchUpstream(path, branch, remote string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.SetBranchUpstream(path, branch, remote)
}
