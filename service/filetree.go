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
	treeCache      *FileTreeCache // 单层目录节点缓存（纯内存，mtime + TTL + 手动刷新）
}

// NewFileTreeService 创建服务
func NewFileTreeService() *FileTreeService {
	return &FileTreeService{
		gitCmd:    util.NewGitCommand(),
		treeCache: NewFileTreeCache(),
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
//
// 缓存策略（复用 RepoScanCache 的 mtime + TTL + 手动刷新范式）：
//   - 规范化 path（filepath.Abs）作为缓存键
//   - os.Stat 取目录 mtime；Stat 失败（目录不存在等）跳过缓存直接实扫
//   - 缓存命中且 mtime 未变且未 TTL 过期 -> 返回缓存节点深拷贝，无 os.ReadDir / git remote 开销
//   - 未命中或失效 -> 实扫 + 排序 + git 信息 -> 回写缓存 -> 返回
//
// GetTree 递归内部调本方法，自动受益于缓存，无需 path|depth 复合键。
func (s *FileTreeService) GetChildren(dirPath string) ([]*model.FileTreeNode, error) {
	abs, err := filepath.Abs(dirPath)
	if err != nil {
		abs = dirPath
	}

	// 取目录 mtime 用于缓存键判定；Stat 失败（目录不存在等）跳过缓存直接实扫
	info, statErr := os.Stat(dirPath)
	if statErr == nil {
		if nodes, ok := s.treeCache.get(abs, info.ModTime()); ok {
			return nodes, nil
		}
	}

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

	// 回写缓存：仅当成功取到 mtime 时（Stat 失败说明目录不可访问，不缓存）
	if statErr == nil {
		s.treeCache.set(abs, info.ModTime(), nodes)
	}

	return nodes, nil
}

// InvalidateCache 清除指定路径的缓存（纯失效，不重扫不返回数据）。
// 供手动刷新（右键/F5/文件操作后）绕过缓存，后续 GetChildren miss 后实扫回写最新数据。
// 数据流等价于原 RefreshChildren，但消除无用序列化：前端 refreshNode 调本方法清缓存后，
// 经 el-tree expand 触发 loadTreeNode -> GetFileTree 命中实扫结果，无需经 App 方法返回节点列表。
func (s *FileTreeService) InvalidateCache(dirPath string) {
	abs, err := filepath.Abs(dirPath)
	if err != nil {
		abs = dirPath
	}
	s.treeCache.clearPath(abs)
}

// ClearAllCache 清除全部文件树缓存，供工具栏"刷新"按钮（el-tree 整体重建）前置调用。
func (s *FileTreeService) ClearAllCache() {
	s.treeCache.clearAll()
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
