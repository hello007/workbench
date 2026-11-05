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
	gitRepoCache   sync.Map       // path -> bool 缓存（是否 git 仓库）
	gitRemoteCache sync.Map       // path -> bool 缓存（git 仓库是否配置远程）
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

// GetRootNodesForPaths 为聚合视图构造根节点列表（虚拟工作区：收藏目录平铺浏览）。
//
// 对每个入参 path：
//   - 空白串（含纯空白）-> 跳过，不产占位节点
//   - 入参先经 filepath.Clean 归一：尾分隔符（D:\repo\ 与 D:\repo）落到同一节点 key
//   - Stat 成功且为目录 -> 正常节点（复用 isGitRepoDir/hasRemote 缓存填 git 徽标）
//   - Stat 成功但非目录 -> 跳过（收藏可含文件，文件类收藏不进聚合树）
//   - Stat 失败（被删/移动/无权限）-> Missing 占位节点，前端标灰提示并可右键移除收藏；
//     静默丢弃会让用户误以为收藏丢失，显式占位才能形成失效兜底闭环
//
// 顺序恒与入参一致（收藏列表顺序），不排序不去重；一次批量调用同时收口存在性判定
// 与 git 徽标，避免前端逐目录 IPC（前端无轻量存在性检查绑定，借道 GetFileTree 全扫
// 或逐个 GetGitInfo 均劣）。
func (s *FileTreeService) GetRootNodesForPaths(paths []string) []*model.FileTreeNode {
	nodes := make([]*model.FileTreeNode, 0, len(paths))
	for _, raw := range paths {
		// 空白路径（含纯空白串）跳过：Stat 失败会产出不可读的 Missing 占位，
		// 与「收藏目录已失效」的兜底语义混淆
		if strings.TrimSpace(raw) == "" {
			continue
		}
		// 路径归一到干净写法：D:\repo\ 与 D:\repo 必须落到同一节点 key
		// （前端 el-tree node-key=path，差异写法会重复注册 nodesMap、重复缓存）
		// 后续 Stat、filepath.Base、git 徽标缓存键与返回节点的 Path/ID 全部使用清洗后的路径
		p := filepath.Clean(raw)
		name := filepath.Base(p)
		// 卷根（C:\、/）的 Base 结果是分隔符本身，回退 VolumeName 给出可读展示名；
		// 非 Windows 卷或以分隔符为名的路径再回退清洗后的路径串，保证不产出空名
		if name == "." || name == string(os.PathSeparator) || name == "/" || name == "\\" {
			if vol := filepath.VolumeName(p); vol != "" {
				name = vol
			} else {
				name = p
			}
		}
		info, err := os.Stat(p)
		if err != nil {
			node := model.NewFileTreeNode(name, p, "directory")
			node.Missing = true
			// 失效目录磁盘上已无子节点：显式置叶子，避免节点自相矛盾
			// （directory 类型默认 HasChildren=true/IsLeaf=false，会诱导消费面展开一个不存在的目录）
			node.HasChildren = false
			node.IsLeaf = true
			nodes = append(nodes, node)
			continue
		}
		if !info.IsDir() {
			continue
		}
		node := model.NewFileTreeNode(name, p, "directory")
		node.IsGitRepo = s.isGitRepoDir(p)
		if node.IsGitRepo {
			node.HasRemote = s.hasRemote(p)
		}
		nodes = append(nodes, node)
	}
	return nodes
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
	// git 徽标缓存联动失效：目录本身 + 直接子目录。
	// 手动刷新父目录时子目录徽标同样要更新（git init / git remote add 后立即可见）；
	// 孙级由各自的 InvalidateCache 驱动，不在此级联（避免刷新父目录清空整棵子树缓存）。
	s.invalidateGitBadgeCache(abs)
	// GetChildren 以调用方传入的路径拼子节点键，相对路径场景下缓存键未必是 abs，两种写法都清
	if dirPath != abs {
		s.invalidateGitBadgeCache(dirPath)
	}
}

// invalidateGitBadgeCache 清除指定路径及其直接子目录的 git 徽标缓存（是否仓库 / 是否有远程）。
// 不引入 TTL：徽标只随手动刷新失效，避免刷新后仍显示陈旧结论。
func (s *FileTreeService) invalidateGitBadgeCache(dirPath string) {
	// 前缀拼接前归一：带尾分隔符的入参会拼出双分隔符，导致直接子目录匹配落空
	dirPath = filepath.Clean(dirPath)
	s.gitRepoCache.Delete(dirPath)
	s.gitRemoteCache.Delete(dirPath)

	prefix := dirPath + string(os.PathSeparator)
	deleteDirectChildren := func(cache *sync.Map) {
		cache.Range(func(key, _ any) bool {
			k, ok := key.(string)
			if !ok || !strings.HasPrefix(k, prefix) {
				return true
			}
			// 仅直接子目录：前缀之后的剩余部分不含分隔符
			if !strings.Contains(k[len(prefix):], string(os.PathSeparator)) {
				cache.Delete(k)
			}
			return true
		})
	}
	deleteDirectChildren(&s.gitRepoCache)
	deleteDirectChildren(&s.gitRemoteCache)
}

// ClearAllCache 清除全部文件树缓存，供工具栏"刷新"按钮（el-tree 整体重建）前置调用。
func (s *FileTreeService) ClearAllCache() {
	s.treeCache.clearAll()
	s.gitRepoCache.Clear()
	s.gitRemoteCache.Clear()
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
