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

// safeEmit 安全发射 Wails 事件，非 Wails 上下文时静默跳过
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
