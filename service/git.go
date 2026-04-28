package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git-manager/model"
	"git-manager/util"
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
