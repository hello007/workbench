package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

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

// PullRepo 拉取更新。useRebase=true 走 git pull --rebase（变基模式），false 走普通 pull。
func (a *App) PullRepo(dirPath string, useRebase bool) string {
	if !util.NewGitCommand().IsGitRepository(dirPath) {
		return "错误: 不是Git仓库"
	}
	if !a.gitSvc.HasRemote(dirPath) {
		return "该仓库未配置远程，无需拉取"
	}
	output, err := a.gitSvc.Pull(dirPath, useRebase)
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

// GetCommitHistory 获取 Git 仓库的提交历史，支持服务端过滤与分页。
// filter 各字段组合语义为 AND：Since/Until/FilePath 下推 go-git LogOptions 原生过滤，
// Author/Keyword 因 go-git v5.18.0 LogOptions 无 Author 字段，在迭代内手动子串匹配（大小写不敏感）。
// offset 为过滤后偏移：先跳过不匹配提交，再跳过 offset 个匹配提交，最后收集 limit 个。
// filter 全空时与原分页行为完全一致（跳过 offset + 收集 limit）。
func (a *App) GetCommitHistory(path string, limit, offset int, filter model.CommitFilter) ([]model.Commit, error) {
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

	// 构造日志迭代选项：Since/Until/FilePath 原生下推，Author/Keyword 迭代内手动匹配
	logOpts := &git.LogOptions{Order: git.LogOrderCommitterTime}
	if t, err := parseDateStart(filter.Since); err == nil {
		logOpts.Since = &t
	}
	if t, err := parseDateEnd(filter.Until); err == nil {
		logOpts.Until = &t
	}
	if filter.FilePath != "" {
		fp := strings.ToLower(filter.FilePath)
		logOpts.PathFilter = func(p string) bool {
			return strings.Contains(strings.ToLower(p), fp)
		}
	}
	authorQ := strings.ToLower(filter.Author)
	keywordQ := strings.ToLower(filter.Keyword)

	commitIter, err := repo.Log(logOpts)
	if err != nil {
		return nil, fmt.Errorf("无法获取提交历史: %w", err)
	}
	defer commitIter.Close()

	// 过滤后分页：跳过不匹配提交 → 跳过 offset 个匹配提交 → 收集 limit 个
	commits := make([]model.Commit, 0, limit)
	skipped := 0
	for len(commits) < limit {
		commitObj, err := commitIter.Next()
		if err != nil {
			break
		}

		// Author/Keyword 手动过滤（Since/Until/FilePath 已由迭代器过滤）
		if authorQ != "" {
			hay := strings.ToLower(commitObj.Author.Name + " " + commitObj.Author.Email)
			if !strings.Contains(hay, authorQ) {
				continue
			}
		}
		if keywordQ != "" && !strings.Contains(strings.ToLower(commitObj.Message), keywordQ) {
			continue
		}

		if skipped < offset {
			skipped++
			continue
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
		commit.Files = getCommitFiles(repo, commitObj)
		commits = append(commits, commit)
	}

	return commits, nil
}

// parseDateStart 解析 YYYY-MM-DD 为当天 00:00:00 本地时刻，空串或格式错返回错误。
func parseDateStart(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
}

// parseDateEnd 解析 YYYY-MM-DD 为当天 23:59:59 本地时刻，空串或格式错返回错误。
func parseDateEnd(s string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
	if err != nil {
		return t, err
	}
	return t.Add(24*time.Hour - time.Second), nil
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

// GetCommitFileDiff 获取指定提交中单个文件相对其父提交的 unified diff 文本。
// root commit（无 parent）对比空树呈现为全增。file 为空时返回整提交 diff。
func (a *App) GetCommitFileDiff(path, sha, file string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	if sha == "" {
		return "", fmt.Errorf("提交 SHA 不能为空")
	}
	return a.gitSvc.GetCommitFileDiff(path, sha, file)
}

// GetRangeDiff 获取两个提交之间的 unified diff 文本（全文件，base 到 head 方向）。
func (a *App) GetRangeDiff(path, baseSHA, headSHA string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	if baseSHA == "" || headSHA == "" {
		return "", fmt.Errorf("提交 SHA 不能为空")
	}
	return a.gitSvc.GetRangeDiff(path, baseSHA, headSHA)
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

// ===== 分支管理域（增删改） =====

// CreateBranch 从当前 HEAD 创建新分支
func (a *App) CreateBranch(path, name string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.CreateBranch(path, name)
}

// DeleteBranch 删除本地分支，force=true 走强删（-D）
func (a *App) DeleteBranch(path, name string, force bool) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.DeleteBranch(path, name, force)
}

// RenameBranch 重命名本地分支（仅本地，不触远程）
func (a *App) RenameBranch(path, oldName, newName string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.RenameBranch(path, oldName, newName)
}

// ===== 暂存区管理域 =====

// StageFiles 暂存文件（git add -- <files>）
func (a *App) StageFiles(path string, files []string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.StageFiles(path, files)
}

// UnstageFiles 取消暂存文件（git restore --staged -- <files>）
func (a *App) UnstageFiles(path string, files []string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.UnstageFiles(path, files)
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

// ===== 合并 / 变基 / 拣选 / 冲突解决域 =====

// Merge 合并 branch 到当前分支，mode 取 ff/no-ff/squash。返回 git stdout。
func (a *App) Merge(path, branch string, mode model.MergeMode) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.Merge(path, branch, mode)
}

// Rebase 将当前分支变基到 branch 之上。
func (a *App) Rebase(path, branch string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.Rebase(path, branch)
}

// CherryPick 将 sha 拣选到当前分支。
func (a *App) CherryPick(path, sha string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.CherryPick(path, sha)
}

// GetConflictState 返回当前冲突态快照（操作类型 + 冲突文件列表）。
func (a *App) GetConflictState(path string) (*model.ConflictState, error) {
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.GetConflictState(path)
}

// ResolveConflict 标记单个冲突文件已解决（git add）。
func (a *App) ResolveConflict(path, file string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.ResolveConflict(path, file)
}

// ContinueMerge 合并冲突解决后提交合并。
func (a *App) ContinueMerge(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.ContinueMerge(path)
}

// ContinueRebase 变基冲突解决后继续。
func (a *App) ContinueRebase(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.ContinueRebase(path)
}

// ContinueCherryPick 拣选冲突解决后继续。
func (a *App) ContinueCherryPick(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.ContinueCherryPick(path)
}

// AbortMerge 中止合并。
func (a *App) AbortMerge(path string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.AbortMerge(path)
}

// AbortRebase 中止变基。
func (a *App) AbortRebase(path string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.AbortRebase(path)
}

// AbortCherryPick 中止拣选。
func (a *App) AbortCherryPick(path string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.AbortCherryPick(path)
}

// SkipRebase 跳过当前冲突提交继续变基。
func (a *App) SkipRebase(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.SkipRebase(path)
}
