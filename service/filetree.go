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
//   - Stat 失败（被删/移动/无权限/网络盘不可达）-> Missing 占位节点，前端标灰提示并可
//     右键移除收藏；静默丢弃会让用户误以为收藏丢失，显式占位才能形成失效兜底闭环。
//     原始错误只记录 Warn 日志（前端不区分原因），排障依赖该日志
//
// 顺序恒与入参一致（收藏列表顺序），不排序不去重；一次批量调用同时收口存在性判定
// 与 git 徽标，避免前端逐目录 IPC（前端无轻量存在性检查绑定，借道 GetFileTree 全扫
// 或逐个 GetGitInfo 均劣）。
//
// 并发化（收藏上限 100，串行 os.Stat + git remote 子进程在首帧总时长可观）：
//  1. 预处理阶段串行：空白跳过、filepath.Clean 归一、展示名计算（纯 CPU，无 IO）；
//  2. 阶段一：仅对有效项并发 os.Stat（信号量 4 限流，避免网络盘/大目录抖动放大并发压力）；
//  3. 阶段二：仅对「Stat 成功且是目录」的项并发探测 git 徽标（信号量 4）；isGitRepoDir 为
//     纯 Stat，hasRemote 会 fork git 子进程，二者共用同一限流避免子进程风暴；
//  4. 收尾串行按入参下标组装输出，顺序不依赖并发完成次序。
//
// 内存安全：两个阶段的 goroutine 均只写各自入参下标的预分配切片，互不重叠，
// 故不需要互斥锁，也不使用共享可变切片 append。
//
// 刻意不做整体超时：Wails 绑定签名没有 ctx，且网络盘的 os.Stat 是不可中断的阻塞
// 系统调用；强行超时只能靠独立 goroutine 丢弃结果，存在 goroutine 泄漏风险。
// 本批仅通过并发化降低总时长，不承诺最坏耗时有上界。
func (s *FileTreeService) GetRootNodesForPaths(paths []string) []*model.FileTreeNode {
	// ---- 预处理：串行归一（纯 CPU） ----
	type pathEntry struct {
		path string
		name string
	}
	entries := make([]pathEntry, len(paths))
	valid := make([]bool, len(paths))
	for i, raw := range paths {
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
		entries[i] = pathEntry{path: p, name: name}
		valid[i] = true
	}

	// 并发限流：可缓冲信号量（容量即并发上限），goroutine 进出槽位成对
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup

	// ---- 阶段一：并发存在性判定 ----
	type statResult struct {
		exists bool
		isDir  bool
		err    error
	}
	stats := make([]statResult, len(paths))
	for i := range entries {
		if !valid[i] {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			info, err := os.Stat(entries[idx].path)
			if err != nil {
				// 临界区内只记失败，不在持槽期间做日志 IO（文件 IO 会挤占并发槽位，
				// 并发上限 4 时直接延长其他路径的等待）；Warn 在 wg.Wait 之后补写
				stats[idx].err = err
				return
			}
			stats[idx].exists = true
			stats[idx].isDir = info.IsDir()
		}(i)
	}
	wg.Wait()

	// 日志在信号量临界区外逐条补写：占位节点对前端是同一形态（不区分不存在/无权限/
	// 网络盘不可达），原始错误是失效占位的唯一排障依据。刻意不做同轮去重/汇总——
	// 每条失效路径都要带原始 error（100 条失效即 100 行 Warn，属已知取舍，
	// 规模由收藏上限 100 兜底）。
	for i := range entries {
		if stats[i].err != nil {
			Logger().Warn("aggregated root stat failed", "path", entries[i].path, "err", stats[i].err)
		}
	}

	// ---- 阶段二：仅对有效目录并发探测 git 徽标 ----
	isRepo := make([]bool, len(paths))
	hasRemote := make([]bool, len(paths))
	for i := range entries {
		if !valid[i] || !stats[i].exists || !stats[i].isDir {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			isRepo[idx] = s.isGitRepoDir(entries[idx].path)
			if isRepo[idx] {
				hasRemote[idx] = s.hasRemote(entries[idx].path)
			}
		}(i)
	}
	wg.Wait()

	// ---- 收尾：按入参下标组装，保持入参顺序 ----
	nodes := make([]*model.FileTreeNode, 0, len(paths))
	for i := range entries {
		if !valid[i] {
			continue
		}
		if !stats[i].exists {
			node := model.NewFileTreeNode(entries[i].name, entries[i].path, "directory")
			node.Missing = true
			// 失效目录磁盘上已无子节点：显式置叶子，避免节点自相矛盾
			// （directory 类型默认 HasChildren=true/IsLeaf=false，会诱导消费面展开一个不存在的目录）
			node.HasChildren = false
			node.IsLeaf = true
			nodes = append(nodes, node)
			continue
		}
		if !stats[i].isDir {
			continue
		}
		node := model.NewFileTreeNode(entries[i].name, entries[i].path, "directory")
		node.IsGitRepo = isRepo[i]
		if node.IsGitRepo {
			node.HasRemote = hasRemote[i]
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

	// 拼接前去掉尾分隔符：卷根（C:\）经 Clean 后仍带尾分隔符，直接拼接会得到
	// 「C:\\」使 HasPrefix 对「C:\Windows」恒为 false，卷根下一级子目录徽标永不失效。
	// Unix 根「/」TrimRight 后为空串，补回单个「/」，前缀语义保持一致。
	prefix := strings.TrimRight(dirPath, string(os.PathSeparator)) + string(os.PathSeparator)
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
