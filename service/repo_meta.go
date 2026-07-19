package service

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// RepoMetaService 仓库用户元数据服务，持久化到 data/repo_meta.json。
// 所有路径键统一 filepath.Abs 规范化（与 DirectoryService 一致），规避大小写/分隔符歧义。
//
// 并发安全：所有读-改-写操作通过 Mutate 在 mu 互斥锁保护下串行执行，
// 规避 SaveRepoMeta 防抖保存与扫描合并保存交错导致的丢更新
// （如 SaveRepoMeta 的旧快照覆盖扫描刚写入的 ReadmeSummary）。
type RepoMetaService struct {
	configPath string
	mu         sync.Mutex
}

// repoMetaConfig 元数据配置文件结构，与 favorites.json / directories.json 风格一致。
type repoMetaConfig struct {
	Repos map[string]*model.RepoMeta `json:"repos"`
}

// NewRepoMetaService 创建元数据服务实例。
func NewRepoMetaService(configPath string) *RepoMetaService {
	return &RepoMetaService{configPath: configPath}
}

// loadLocked 加载全部元数据，按规范化路径索引（调用方持 mu 锁）。
// 文件不存在返回空 map（不报错）；文件存在但解析失败返回错误。
func (s *RepoMetaService) loadLocked() (map[string]*model.RepoMeta, error) {
	if !util.FileExists(s.configPath) {
		return make(map[string]*model.RepoMeta), nil
	}
	var config repoMetaConfig
	if err := util.LoadJSON(s.configPath, &config); err != nil {
		return nil, err
	}
	if config.Repos == nil {
		return make(map[string]*model.RepoMeta), nil
	}
	return config.Repos, nil
}

// saveLocked 持久化全量元数据（调用方持 mu 锁）。
func (s *RepoMetaService) saveLocked(repos map[string]*model.RepoMeta) error {
	config := repoMetaConfig{Repos: repos}
	return util.SaveJSON(s.configPath, config)
}

// Load 加载全部元数据（只读，加锁以保证与并发写不冲突）。
func (s *RepoMetaService) Load() (map[string]*model.RepoMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// Save 全量持久化元数据（加锁）。供外部读-改-写场景使用；
// 高频或与扫描并发的写请改用 Mutate，避免 Load 与 Save 之间的竞态窗口。
func (s *RepoMetaService) Save(repos map[string]*model.RepoMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(repos)
}

// Mutate 在互斥锁保护下执行「加载-修改-保存」的原子读-改-写，串行化所有写操作。
// fn 接收已加载的 map（可能为空），返回是否需要落盘与可能的错误；fn 内可通过闭包输出结果。
// 加载失败时返回错误且不执行 fn（不落盘，避免覆盖损坏文件造成进一步数据丢失）。
func (s *RepoMetaService) Mutate(fn func(repos map[string]*model.RepoMeta) (dirty bool, err error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	repos, err := s.loadLocked()
	if err != nil {
		return fmt.Errorf("加载元数据失败: %w", err)
	}
	dirty, fnErr := fn(repos)
	if fnErr != nil {
		return fnErr
	}
	if dirty {
		return s.saveLocked(repos)
	}
	return nil
}

// normalizePath 将路径规范化为绝对路径，作为元数据主键。规范化失败时原样返回。
func normalizePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// Upsert 按规范化路径写入/更新元数据，刷新 UpdatedAt。路径经 filepath.Abs 规范化后作为主键。
func (s *RepoMetaService) Upsert(meta *model.RepoMeta) error {
	if meta == nil {
		return fmt.Errorf("元数据不能为空")
	}
	if meta.Path == "" {
		return fmt.Errorf("路径不能为空")
	}
	// 规范化主键，保留其他字段，刷新 UpdatedAt
	normalized := normalizePath(meta.Path)
	meta.Path = normalized
	meta.UpdatedAt = time.Now()
	return s.Mutate(func(repos map[string]*model.RepoMeta) (bool, error) {
		repos[normalized] = meta
		return true, nil
	})
}

// Delete 删除指定路径的元数据（路径内部规范化）。
func (s *RepoMetaService) Delete(path string) error {
	normalized := normalizePath(path)
	return s.Mutate(func(repos map[string]*model.RepoMeta) (bool, error) {
		if _, ok := repos[normalized]; !ok {
			return false, fmt.Errorf("元数据不存在")
		}
		delete(repos, normalized)
		return true, nil
	})
}

// DeleteMissing 清理所有 Missing=true 的失效记录，返回清理数量。
// 供"清理失效记录"手动入口调用（PRD F15：不自动删除，由用户手动触发）。
func (s *RepoMetaService) DeleteMissing() (int, error) {
	removed := 0
	err := s.Mutate(func(repos map[string]*model.RepoMeta) (bool, error) {
		for key, meta := range repos {
			if meta != nil && meta.Missing {
				delete(repos, key)
				removed++
			}
		}
		return removed > 0, nil
	})
	return removed, err
}
