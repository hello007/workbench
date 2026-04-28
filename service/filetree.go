package service

import (
	"os"
	"path/filepath"
	"strings"

	"git-manager/model"
	"git-manager/util"
)

// FileTreeService 文件树服务
type FileTreeService struct {
	gitCmd *util.GitCommand
}

// NewFileTreeService 创建服务
func NewFileTreeService() *FileTreeService {
	return &FileTreeService{
		gitCmd: util.NewGitCommand(),
	}
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

		if name == ".git" || strings.HasPrefix(name, ".") {
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
			node.IsGitRepo = s.gitCmd.IsGitRepository(fullPath)
		}

		nodes = append(nodes, node)
	}

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
