package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"git-manager/model"
	"git-manager/util"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// GitService Git服务
type GitService struct {
	gitCmd *util.GitCommand
}

// NewGitService 创建服务
func NewGitService() *GitService {
	return &GitService{
		gitCmd: util.NewGitCommand(),
	}
}

// GetInfo 获取仓库信息
func (s *GitService) GetInfo(dirPath string) (*model.GitRepoInfo, error) {
	info := &model.GitRepoInfo{
		Path:   dirPath,
		IsRepo: s.gitCmd.IsGitRepository(dirPath),
	}

	if !info.IsRepo {
		return info, nil
	}

	branch, err := s.gitCmd.GetBranch(dirPath)
	if err == nil {
		info.Branch = strings.TrimSpace(branch)
	}

	remote, remoteURL, err := s.gitCmd.GetRemote(dirPath)
	if err == nil {
		info.Remote = remote
		info.RemoteURL = remoteURL
	}

	return info, nil
}

// GetLog 获取提交历史（需要补充util/git.go的方法）
func (s *GitService) GetLog(dirPath string, page, pageSize int) (*model.PageResult, error) {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return nil, fmt.Errorf("不是Git仓库")
	}

	// 简化实现，这里假设使用固定逻辑
	commits := []model.GitCommit{}

	// 这里需要调用Git命令获取日志
	// 暂时返回空结果
	return model.NewPageResult(commits, 0, page, pageSize), nil
}

// Clone 克隆仓库
func (s *GitService) Clone(url, targetPath string) (string, error) {
	if _, err := os.Stat(targetPath); err == nil {
		return "", fmt.Errorf("目标路径已存在")
	}

	return s.gitCmd.Clone(url, targetPath)
}

// Pull 拉取更新
func (s *GitService) Pull(dirPath string) (string, error) {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return "", fmt.Errorf("不是Git仓库")
	}

	return s.gitCmd.Pull(dirPath)
}

// ExtractRepoName 提取仓库名
func (s *GitService) ExtractRepoName(url string) string {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "repo"
}

// GetBranches 获取仓库的分支列表
func (s *GitService) GetBranches(dirPath string) (*model.BranchList, error) {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return nil, fmt.Errorf("不是Git仓库")
	}

	output, err := s.gitCmd.GetBranchesAll(dirPath)
	if err != nil {
		return nil, fmt.Errorf("获取分支列表失败: %w", err)
	}

	var branches []model.BranchInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		isCurrent := strings.HasPrefix(line, "* ")
		if isCurrent {
			line = strings.TrimSpace(line[2:])
		} else {
			line = strings.TrimSpace(strings.TrimPrefix(line, "  "))
		}

		// 过滤 HEAD -> 引用
		if strings.Contains(line, "HEAD ->") {
			continue
		}

		if strings.HasPrefix(line, "remotes/") {
			name := strings.TrimPrefix(line, "remotes/")
			branches = append(branches, model.BranchInfo{
				Name:      name,
				IsRemote:  true,
				IsCurrent: isCurrent,
			})
		} else {
			branches = append(branches, model.BranchInfo{
				Name:      line,
				IsRemote:  false,
				IsCurrent: isCurrent,
			})
		}
	}

	return &model.BranchList{Branches: branches}, nil
}

// CheckoutBranch 切换分支
func (s *GitService) CheckoutBranch(dirPath string, branchName string, isRemote bool) error {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return fmt.Errorf("不是Git仓库")
	}

	hasChanges, err := s.gitCmd.HasLocalChanges(dirPath)
	if err != nil {
		return fmt.Errorf("检查工作区状态失败: %w", err)
	}
	if hasChanges {
		return fmt.Errorf("当前有未提交的变更，请先提交或暂存后再切换分支")
	}

	if isRemote {
		parts := strings.SplitN(branchName, "/", 2)
		localName := branchName
		if len(parts) == 2 {
			localName = parts[1]
		}
		_, err := s.gitCmd.CheckoutRemote(dirPath, branchName, localName)
		return err
	}

	_, err = s.gitCmd.CheckoutLocal(dirPath, branchName)
	return err
}

// ScanGitRepos 递归扫描目录下所有 Git 仓库
// 如果 rootPath 本身是 git 仓库，直接返回 [rootPath]
// 否则递归遍历子目录，收集所有 git 仓库路径
func (s *GitService) ScanGitRepos(rootPath string) []string {
	if s.gitCmd.IsGitRepository(rootPath) {
		return []string{rootPath}
	}

	var repos []string
	s.scanDir(rootPath, &repos)
	return repos
}

func (s *GitService) scanDir(dir string, repos *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		// 跳过 .git 目录本身
		if entry.Name() == ".git" {
			continue
		}

		if s.gitCmd.IsGitRepository(fullPath) {
			*repos = append(*repos, fullPath)
		} else {
			s.scanDir(fullPath, repos)
		}
	}
}

// GetLocalChanges 获取本地变动文件列表
func (s *GitService) GetLocalChanges(dirPath string) ([]model.FileChange, error) {
	gitRoot, err := util.FindGitRoot(dirPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	// 使用 -z 以 NUL 分隔输出，避免路径引号和八进制转义问题
	output, err := s.gitCmd.Execute(gitRoot, "status", "--porcelain", "-z")
	if err != nil {
		return nil, fmt.Errorf("获取本地变动失败: %w", err)
	}

	if output == "" {
		return []model.FileChange{}, nil
	}

	segments := strings.Split(output, "\x00")
	changes := make([]model.FileChange, 0, len(segments))

	for i := 0; i < len(segments); i++ {
		seg := segments[i]
		if seg == "" || len(seg) < 4 || seg[2] != ' ' {
			continue
		}

		staged := seg[0] != ' ' && seg[0] != '?'
		statusRaw := seg[:2]
		filePath := seg[3:]

		// 重命名/复制时，下一个 segment 是目标路径
		if (statusRaw[0] == 'R' || statusRaw[0] == 'C') && i+1 < len(segments) && segments[i+1] != "" {
			filePath = segments[i+1]
			i++
		}

		// 取工作区状态码
		status := strings.TrimSpace(statusRaw)
		if len(status) == 2 {
			status = string(status[1])
		}

		changes = append(changes, model.FileChange{
			Path:   filePath,
			Status: status,
			Staged: staged,
		})
	}

	return changes, nil
}

// DiscardChanges 回滚本地变动
func (s *GitService) DiscardChanges(dirPath string, filePaths []string) error {
	gitRoot, err := util.FindGitRoot(dirPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	if len(filePaths) == 0 {
		// 回滚全部：从 HEAD 恢复已跟踪文件，再清理未跟踪文件
		if _, err := s.gitCmd.Execute(gitRoot, "checkout", "HEAD", "--", "."); err != nil {
			return fmt.Errorf("回滚失败: %w", err)
		}
		if _, err := s.gitCmd.Execute(gitRoot, "clean", "-fd"); err != nil {
			return fmt.Errorf("清理未跟踪文件失败: %w", err)
		}
		return nil
	}

	// 查询文件状态，区分已跟踪和未跟踪
	changes, err := s.GetLocalChanges(dirPath)
	if err != nil {
		return fmt.Errorf("获取文件状态失败: %w", err)
	}

	untrackedSet := make(map[string]bool)
	for _, c := range changes {
		if c.Status == "?" {
			untrackedSet[c.Path] = true
		}
	}

	var tracked, untracked []string
	for _, p := range filePaths {
		if untrackedSet[p] {
			untracked = append(untracked, p)
		} else {
			tracked = append(tracked, p)
		}
	}

	if len(tracked) > 0 {
		// 从 HEAD 恢复已跟踪文件（同时更新索引和工作区）
		args := append([]string{"checkout", "HEAD", "--"}, tracked...)
		if _, err := s.gitCmd.Execute(gitRoot, args...); err != nil {
			return fmt.Errorf("回滚失败: %w", err)
		}
	}

	if len(untracked) > 0 {
		args := append([]string{"clean", "-fd", "--"}, untracked...)
		if _, err := s.gitCmd.Execute(gitRoot, args...); err != nil {
			return fmt.Errorf("清理未跟踪文件失败: %w", err)
		}
	}

	return nil
}
func safeEmit(ctx context.Context, event string, data ...interface{}) {
	if ctx == nil || ctx.Value("events") == nil {
		return
	}
	runtime.EventsEmit(ctx, event, data...)
}

// BatchPull 并行拉取多个 Git 仓库
func (s *GitService) BatchPull(repos []string, concurrency int, ctx context.Context) []model.PullResult {
	if concurrency <= 0 {
		concurrency = 5
	}

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		results      []model.PullResult
		sem          = make(chan struct{}, concurrency)
		successCount int
		failCount    int
	)

	for _, repo := range repos {
		wg.Add(1)
		go func(repoPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			name := filepath.Base(repoPath)
			result := model.PullResult{
				Path: repoPath,
				Name: name,
			}

			if !s.gitCmd.IsGitRepository(repoPath) {
				result.Success = false
				result.Error = "不是 Git 仓库"
			} else {
				gitCmd := util.NewGitCommandWithTimeout(5 * time.Minute)
				output, err := gitCmd.Pull(repoPath)
				if err != nil {
					result.Success = false
					result.Error = err.Error()
				} else {
					result.Success = true
					result.Output = strings.TrimSpace(output)
				}
			}

			mu.Lock()
			results = append(results, result)
			if result.Success {
				successCount++
			} else {
				failCount++
			}
			mu.Unlock()

			safeEmit(ctx, "pull-progress", result)
		}(repo)
	}

	wg.Wait()

	safeEmit(ctx, "pull-complete", map[string]int{
		"success": successCount,
		"failed":  failCount,
	})

	return results
}
