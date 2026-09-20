package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"workbench/model"
	"workbench/util"
)

// DashboardService 全局状态看板服务：管理 pin 仓库列表持久化 + 批量计算多仓状态快照。
//
// pin 列表持久化到 data/dashboard_pinned.json（纯路径数组，filepath.Abs 规范化主键），
// 复用 RepoMetaService 持久化范式（loadLocked/saveLocked + Mutate 原子读改写 +
// Load 损坏降级空不阻塞）。与 RepoMeta 职责分离：RepoMeta 存用户元数据（简述/标签），
// PinnedRepos 仅存关注路径列表。
//
// 状态计算：只读操作，不走 tryLockRepo 仓级锁（锁仅约束变更类操作）。
// 批量并发复用 HasRemotesBatch 的 sem+wg 范式（并发 8）。
type DashboardService struct {
	configPath string
	gitCmd     *util.GitCommand
	mu         sync.Mutex
}

// NewDashboardService 创建看板服务实例。
// configPath 为 pin 列表持久化路径（如 data/dashboard_pinned.json）。
func NewDashboardService(configPath string) *DashboardService {
	return &DashboardService{
		configPath: configPath,
		gitCmd:     util.NewGitCommand(),
	}
}

// pinnedReposConfig pin 列表配置文件结构，与 favorites.json / repo_meta.json 风格一致。
type pinnedReposConfig struct {
	Pinned model.PinnedRepos `json:"pinned"`
}

// loadLocked 加载 pin 列表（调用方持 mu 锁）。
// 文件不存在返回空 PinnedRepos（不报错）；文件存在但解析失败返回错误。
func (s *DashboardService) loadLocked() (model.PinnedRepos, error) {
	if !util.FileExists(s.configPath) {
		return model.PinnedRepos{}, nil
	}
	var config pinnedReposConfig
	if err := util.LoadJSON(s.configPath, &config); err != nil {
		return model.PinnedRepos{}, err
	}
	return config.Pinned, nil
}

// saveLocked 持久化 pin 列表（调用方持 mu 锁）。
func (s *DashboardService) saveLocked(pinned model.PinnedRepos) error {
	config := pinnedReposConfig{Pinned: pinned}
	return util.SaveJSON(s.configPath, config)
}

// LoadPinned 加载 pin 仓库路径列表（只读，加锁保证与并发写不冲突）。
// 解析失败降级空列表 + Logger().Warn，不阻塞看板展示。
func (s *DashboardService) LoadPinned() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	pinned, err := s.loadLocked()
	if err != nil {
		Logger().Warn("加载 pin 列表失败，降级空列表", "path", s.configPath, "err", err)
		return []string{}
	}
	if pinned.Paths == nil {
		return []string{}
	}
	return pinned.Paths
}

// Mutate 在互斥锁保护下执行「加载-修改-保存」的原子读改写，串行化所有 pin 增删。
// 加载失败时返回错误且不执行 fn（不落盘，避免覆盖损坏文件造成进一步数据丢失）。
func (s *DashboardService) Mutate(fn func(pinned *model.PinnedRepos) (dirty bool, err error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pinned, err := s.loadLocked()
	if err != nil {
		return fmt.Errorf("加载 pin 列表失败: %w", err)
	}
	dirty, fnErr := fn(&pinned)
	if fnErr != nil {
		return fnErr
	}
	if dirty {
		return s.saveLocked(pinned)
	}
	return nil
}

// AddPin 将仓库路径加入 pin 列表（去重，filepath.Abs 规范化）。已存在则幂等返回。
func (s *DashboardService) AddPin(path string) error {
	normalized := normalizePath(path)
	return s.Mutate(func(pinned *model.PinnedRepos) (bool, error) {
		for _, p := range pinned.Paths {
			if p == normalized {
				return false, nil // 已存在，幂等
			}
		}
		pinned.Paths = append(pinned.Paths, normalized)
		pinned.UpdatedAt = time.Now()
		return true, nil
	})
}

// RemovePin 从 pin 列表移除仓库路径（filepath.Abs 规范化）。不存在则幂等返回。
func (s *DashboardService) RemovePin(path string) error {
	normalized := normalizePath(path)
	return s.Mutate(func(pinned *model.PinnedRepos) (bool, error) {
		for i, p := range pinned.Paths {
			if p == normalized {
				pinned.Paths = append(pinned.Paths[:i], pinned.Paths[i+1:]...)
				pinned.UpdatedAt = time.Now()
				return true, nil
			}
		}
		return false, nil // 不存在，幂等
	})
}

// IsPinned 判断路径是否已在 pin 列表（filepath.Abs 规范化）。
func (s *DashboardService) IsPinned(path string) bool {
	normalized := normalizePath(path)
	for _, p := range s.LoadPinned() {
		if p == normalized {
			return true
		}
	}
	return false
}

// GetStatuses 批量计算全部 pin 仓库的状态快照，并发执行（并发上限 8）。
// 返回顺序与 pin 列表一致；失效路径（目录不存在）标 Missing 不阻塞。
// 批量并发只读，不走 tryLockRepo 仓级锁。
func (s *DashboardService) GetStatuses() []model.RepoStatus {
	paths := s.LoadPinned()
	if len(paths) == 0 {
		return []model.RepoStatus{}
	}

	statuses := make([]model.RepoStatus, len(paths))

	const concurrency = 8
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, p := range paths {
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			statuses[idx] = s.ComputeRepoStatus(path)
		}(i, p)
	}
	wg.Wait()
	return statuses
}

// ComputeRepoStatus 计算单个仓库的状态快照（纯查询，不加仓级锁）。
// 失效路径标 Missing + IsRepo=false；非仓库标 IsRepo=false；detached HEAD 标 Detached。
// ahead/behind 用 git rev-list --left-right --count @{u}...HEAD 基于本地远程引用计算，
// 无上游时 HasUpstream=false 且 Ahead/Behind 置 0（不报错降级）。
func (s *DashboardService) ComputeRepoStatus(repoPath string) model.RepoStatus {
	name := filepath.Base(repoPath)
	status := model.RepoStatus{
		Path: repoPath,
		Name: name,
	}

	// 路径失效（目录被删除）标 Missing，不阻塞看板
	if _, err := os.Stat(repoPath); err != nil {
		status.Missing = true
		return status
	}

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		status.Error = fmt.Sprintf("无法定位 Git 仓库根目录: %v", err)
		return status
	}

	status.IsRepo = true

	// 当前分支：git branch --show-current，detached HEAD 时返回空
	branch, err := s.gitCmd.Execute(gitRoot, "branch", "--show-current")
	if err != nil {
		status.Error = fmt.Sprintf("获取分支失败: %v", err)
		return status
	}
	branch = strings.TrimSpace(branch)
	status.Branch = branch

	// detached HEAD：branch --show-current 返回空串
	if branch == "" {
		status.Detached = true
		// 取短 SHA 作展示
		if sha, err := s.gitCmd.Execute(gitRoot, "rev-parse", "--short", "HEAD"); err == nil {
			status.Branch = strings.TrimSpace(sha)
		}
		// detached 无上游可比，ahead/behind 置 0
	} else {
		// 上游检测 + ahead/behind 共享计算（与 ComputeBranchSyncInfo 复用，防两处漂移）
		ahead, behind, hasUpstream, abErr := ComputeAheadBehind(gitRoot)
		if hasUpstream {
			status.HasUpstream = true
			if abErr != nil {
				// 远程引用可能不存在（未 fetch），降级 0/0 + 警告，不阻塞
				status.Error = fmt.Sprintf("计算 ahead/behind 失败: %v", abErr)
			} else {
				status.Ahead = ahead
				status.Behind = behind
			}
		}
		// 无上游时 HasUpstream=false（默认零值），Ahead/Behind 保持 0
	}

	// dirty 检测：git status --porcelain 非空即 dirty（比 GetLocalChanges 轻，不解析文件列表）
	output, err := s.gitCmd.Execute(gitRoot, "status", "--porcelain")
	if err != nil {
		if status.Error == "" {
			status.Error = fmt.Sprintf("检测工作区状态失败: %v", err)
		}
	} else {
		status.Dirty = strings.TrimSpace(output) != ""
	}

	return status
}

// ComputeAheadBehind 计算当前分支相对上游跟踪分支的 ahead/behind（含上游探测）。
// 导出共享：ComputeRepoStatus 与 ComputeBranchSyncInfo 复用同一计算，防两处漂移。
// 语义：git rev-list --left-right --count @{u}...HEAD，基于本地已有远程引用
// （上次 fetch/clone 快照），不主动 fetch。
// 返回约定：
//   - hasUpstream=false：分支无跟踪上游，ahead/behind 置 0，err=nil（降级非错误）
//   - hasUpstream=true 且 err!=nil：有上游但 rev-list 失败（远程引用未 fetch 等），
//     ahead/behind 置 0，由调用方决定降级方式
func ComputeAheadBehind(gitRoot string) (ahead, behind int, hasUpstream bool, err error) {
	gitCmd := util.NewGitCommand()
	// 上游检测：git rev-parse --abbrev-ref @{u}，无上游时 git 非零退出
	if _, err := gitCmd.Execute(gitRoot, "rev-parse", "--abbrev-ref", "@{u}"); err != nil {
		return 0, 0, false, nil
	}
	// @{u}...HEAD 三点表示法：左右各自独有的提交数
	// 输出格式："behind\tahead"（左=上游独有=behind，右=HEAD独有=ahead）
	output, err := gitCmd.Execute(gitRoot, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	if err != nil {
		return 0, 0, true, err
	}
	output = strings.TrimSpace(output)
	parts := strings.Split(output, "\t")
	if len(parts) != 2 {
		return 0, 0, true, fmt.Errorf("rev-list 输出格式异常: %q", output)
	}
	behind, berr := strconv.Atoi(strings.TrimSpace(parts[0]))
	ahead, aerr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if berr != nil || aerr != nil {
		return 0, 0, true, fmt.Errorf("rev-list 数值解析失败: %q", output)
	}
	return ahead, behind, true, nil
}

// ComputeBranchSyncInfo 计算单仓库当前分支与上游的同步状态摘要 + refs 位置映射（纯查询）。
// 供 App.GetBranchSyncInfo 薄委托暴露：一次调用聚合摘要与 refs，避免前端多次 IPC。
// refs 不入 CommitHistoryCache（随 push/fetch 变化），每次调用现算（refs 数量极小，成本可忽略）。
//
// ahead/behind 语义与 ComputeRepoStatus 一致（共享 ComputeAheadBehind）。边界降级：
//   - 空仓库（unborn branch）：返回分支名 + 零值计数 + 空 refs，不报错
//   - detached HEAD：Detached=true，Branch=短 SHA，refs 仅含 HEAD 条目
//   - 无上游：HasUpstream=false，Ahead/Behind 置 0，refs 无 remote 条目
func ComputeBranchSyncInfo(repoPath string) (*model.BranchSyncInfo, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	gitCmd := util.NewGitCommand()

	info := &model.BranchSyncInfo{Refs: []model.CommitRef{}}

	// 当前分支：git branch --show-current，detached HEAD 时返回空（与 ComputeRepoStatus 同语义）
	branch, err := gitCmd.Execute(gitRoot, "branch", "--show-current")
	if err != nil {
		return nil, fmt.Errorf("获取分支失败: %w", err)
	}
	branch = strings.TrimSpace(branch)

	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("无法打开 Git 仓库: %w", err)
	}

	head, headErr := repo.Head()
	if headErr != nil {
		if branch != "" {
			// unborn branch（空仓库有分支名但无 commit）：降级分支名 + 零值，不报错
			info.Branch = branch
			return info, nil
		}
		return nil, fmt.Errorf("无法获取 HEAD 引用: %w", headErr)
	}
	info.HeadSha = head.Hash().String()

	// detached HEAD：branch --show-current 返回空串；Branch 展示短 SHA（对齐 ComputeRepoStatus），
	// refs 仅含 HEAD 条目（detached 时 @{u} 无法解析上游，无 local/remote 条目）
	if branch == "" {
		info.Detached = true
		info.Branch = info.HeadSha[:8]
		info.Refs = append(info.Refs, model.CommitRef{
			Sha:  info.HeadSha,
			Kind: model.CommitRefKindHead,
			Name: "HEAD",
		})
		return info, nil
	}
	info.Branch = branch

	// ahead/behind + 上游检测（与 ComputeRepoStatus 共享同一计算）
	ahead, behind, hasUpstream, abErr := ComputeAheadBehind(gitRoot)
	info.HasUpstream = hasUpstream
	if abErr != nil {
		// 有上游但 rev-list 失败（远程引用不存在未 fetch），降级 0/0 + 警告日志，不阻塞
		Logger().Warn("计算 ahead/behind 失败，降级 0/0", "path", repoPath, "err", abErr)
	} else {
		info.Ahead = ahead
		info.Behind = behind
	}

	// 实际上游名：git rev-parse --abbrev-ref @{u}（MVP 上游即 origin/<branch>，以实际解析为准）
	upstreamName := ""
	if hasUpstream {
		if out, uerr := gitCmd.Execute(gitRoot, "rev-parse", "--abbrev-ref", "@{u}"); uerr == nil {
			upstreamName = strings.TrimSpace(out)
		}
	}

	// refs 遍历：过滤当前本地分支与上游远程引用两类条目。非 detached 时 HEAD 与当前分支
	// 同 commit，位置由 local 条目承载（detached 的 HEAD 条目已在上方单独构造）
	localRefName := plumbing.ReferenceName("refs/heads/" + branch)
	remoteRefName := plumbing.ReferenceName("refs/remotes/" + upstreamName)
	iter, err := repo.References()
	if err != nil {
		return nil, fmt.Errorf("遍历仓库引用失败: %w", err)
	}
	defer iter.Close()
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.HashReference {
			return nil
		}
		switch ref.Name() {
		case localRefName:
			info.Refs = append(info.Refs, model.CommitRef{
				Sha:  ref.Hash().String(),
				Kind: model.CommitRefKindLocal,
				Name: branch,
			})
		case remoteRefName:
			// upstreamName 为空时 remoteRefName 不可能匹配任何引用，此分支仅在有效上游时命中
			info.Refs = append(info.Refs, model.CommitRef{
				Sha:  ref.Hash().String(),
				Kind: model.CommitRefKindRemote,
				Name: upstreamName,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("遍历仓库引用失败: %w", err)
	}

	return info, nil
}
