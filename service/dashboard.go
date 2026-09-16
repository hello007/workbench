package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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
		// 上游检测：git rev-parse --abbrev-ref @{u}，无上游时 git 非零退出
		if _, err := s.gitCmd.Execute(gitRoot, "rev-parse", "--abbrev-ref", "@{u}"); err == nil {
			status.HasUpstream = true
			ahead, behind, err := s.computeAheadBehind(gitRoot)
			if err != nil {
				// 远程引用可能不存在（未 fetch），降级 0/0 + 警告，不阻塞
				status.Error = fmt.Sprintf("计算 ahead/behind 失败: %v", err)
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

// computeAheadBehind 用 git rev-list --left-right --count @{u}...HEAD 计算 ahead/behind。
// 左值=behind（远程领先本地），右值=ahead（本地领先远程）。
// 基于本地已有远程引用（上次 fetch/clone 快照），不主动 fetch。
func (s *DashboardService) computeAheadBehind(gitRoot string) (ahead, behind int, err error) {
	// @{u}...HEAD 三点表示法：左右各自独有的提交数
	// 输出格式："behind\tahead"（左=上游独有=behind，右=HEAD独有=ahead）
	output, err := s.gitCmd.Execute(gitRoot, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	if err != nil {
		return 0, 0, err
	}
	output = strings.TrimSpace(output)
	parts := strings.Split(output, "\t")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("rev-list 输出格式异常: %q", output)
	}
	behind, berr := strconv.Atoi(strings.TrimSpace(parts[0]))
	ahead, aerr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if berr != nil || aerr != nil {
		return 0, 0, fmt.Errorf("rev-list 数值解析失败: %q", output)
	}
	return ahead, behind, nil
}
