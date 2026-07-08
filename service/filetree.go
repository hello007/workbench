package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"workbench/model"
	"workbench/util"
)

// FileTreeService 文件树服务
type FileTreeService struct {
	gitCmd         *util.GitCommand
	gitRepoCache   sync.Map // path -> bool 缓存（是否 git 仓库）
	gitRemoteCache sync.Map // path -> bool 缓存（git 仓库是否配置远程）
}

// NewFileTreeService 创建服务
func NewFileTreeService() *FileTreeService {
	return &FileTreeService{
		gitCmd: util.NewGitCommand(),
	}
}

// isGitRepoDir 使用 os.Stat 快速检查目录是否是 Git 仓库（带缓存）
func (s *FileTreeService) isGitRepoDir(dir string) bool {
	if v, ok := s.gitRepoCache.Load(dir); ok {
		return v.(bool)
	}
	info, err := os.Stat(filepath.Join(dir, ".git"))
	isRepo := err == nil
	_ = info
	s.gitRepoCache.Store(dir, isRepo)
	return isRepo
}

// hasRemote 检测 git 仓库是否配置了远程仓库（带缓存，避免重复 git remote 子进程）。
// 仅对 git 仓库调用；缓存命中直接返回，未命中执行 git remote -v 检测后缓存。
func (s *FileTreeService) hasRemote(dir string) bool {
	if v, ok := s.gitRemoteCache.Load(dir); ok {
		return v.(bool)
	}
	_, _, err := s.gitCmd.GetRemote(dir)
	has := err == nil
	s.gitRemoteCache.Store(dir, has)
	return has
}

// GetChildren 获取子节点
func (s *FileTreeService) GetChildren(dirPath string) ([]*model.FileTreeNode, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var nodes []*model.FileTreeNode

	for _, entry := range entries {
		name := entry.Name()

		if name == ".git" {
			continue
		}

		fullPath := filepath.Join(dirPath, name)
		var fileType string
		if entry.IsDir() {
			fileType = "directory"
		} else {
			fileType = "file"
		}

		node := model.NewFileTreeNode(name, fullPath, fileType)

		if entry.IsDir() {
			node.IsGitRepo = s.isGitRepoDir(fullPath)
			if node.IsGitRepo {
				node.HasRemote = s.hasRemote(fullPath)
			}
		}

		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type == "directory"
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	return nodes, nil
}

// GetTree 递归获取完整树
func (s *FileTreeService) GetTree(dirPath string, maxDepth int) ([]*model.FileTreeNode, error) {
	return s.buildTree(dirPath, 0, maxDepth)
}

// buildTree 递归构建树
func (s *FileTreeService) buildTree(dirPath string, currentDepth, maxDepth int) ([]*model.FileTreeNode, error) {
	if currentDepth >= maxDepth {
		return nil, nil
	}

	nodes, err := s.GetChildren(dirPath)
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if node.Type == "directory" {
			children, err := s.buildTree(node.Path, currentDepth+1, maxDepth)
			if err != nil {
				continue
			}
			node.Children = children
		}
	}

	return nodes, nil
}

// GetGitInfo 获取Git信息
func (s *FileTreeService) GetGitInfo(dirPath string) (*model.GitRepoInfo, error) {
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
