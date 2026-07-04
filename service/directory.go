package service

import (
	"fmt"
	"path/filepath"

	"workbench/model"
	"workbench/util"
)

// DirectoryService 工作目录服务
type DirectoryService struct {
	configPath string
}

// NewDirectoryService 创建服务
func NewDirectoryService(configPath string) *DirectoryService {
	return &DirectoryService{configPath: configPath}
}

// Config 配置结构
type Config struct {
	Directories []*model.Directory `json:"directories"`
}

// Load 加载配置
func (s *DirectoryService) Load() ([]*model.Directory, error) {
	if !util.FileExists(s.configPath) {
		return []*model.Directory{}, nil
	}

	var config Config
	err := util.LoadJSON(s.configPath, &config)
	if err != nil {
		return nil, err
	}

	return config.Directories, nil
}

// Save 保存配置
func (s *DirectoryService) Save(directories []*model.Directory) error {
	config := Config{Directories: directories}
	return util.SaveJSON(s.configPath, config)
}

// Create 创建目录
func (s *DirectoryService) Create(name, path string, isDefault bool) (*model.Directory, error) {
	if !util.FileExists(path) {
		return nil, fmt.Errorf("路径不存在: %s", path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	directories, err := s.Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	for _, dir := range directories {
		if dir.Path == absPath {
			return nil, fmt.Errorf("该目录已添加")
		}
	}

	newDir := model.NewDirectory(name, absPath, isDefault)
	// 在持久化前计算并写入 IsGitRepo，避免启动时再同步检测子进程拖慢渲染。
	// absPath 已经过 filepath.Abs 规范化；IsGitRepository 在路径不存在/检测异常时返回 false，不报错。
	newDir.IsGitRepo = util.NewGitCommand().IsGitRepository(absPath)

	if isDefault {
		for _, dir := range directories {
			dir.IsDefault = false
		}
	}

	directories = append(directories, newDir)
	return newDir, s.Save(directories)
}

// Update 更新目录
func (s *DirectoryService) Update(id, name, path string, isDefault bool) (*model.Directory, error) {
	directories, err := s.Load()
	if err != nil {
		return nil, err
	}

	var target *model.Directory
	for _, dir := range directories {
		if dir.ID == id {
			target = dir
			break
		}
	}

	if target == nil {
		return nil, fmt.Errorf("工作目录不存在")
	}

	if path != target.Path && !util.FileExists(path) {
		return nil, fmt.Errorf("路径不存在: %s", path)
	}

	if path != target.Path {
		absPath, _ := filepath.Abs(path)
		target.Path = absPath
	}

	// path 变化时 git 仓库状态可能改变；为简单稳妥起见，每次 Update 都重算并持久化 IsGitRepo，
	// 覆盖"普通目录改成 git 仓库路径"或反之的变更。IsGitRepository 失败按 false 不报错。
	target.IsGitRepo = util.NewGitCommand().IsGitRepository(target.Path)

	target.Name = name

	if isDefault && !target.IsDefault {
		for _, dir := range directories {
			dir.IsDefault = false
		}
		target.IsDefault = true
	}

	return target, s.Save(directories)
}

// Delete 删除目录
func (s *DirectoryService) Delete(id string) error {
	directories, err := s.Load()
	if err != nil {
		return err
	}

	var newDirs []*model.Directory
	found := false
	for _, dir := range directories {
		if dir.ID != id {
			newDirs = append(newDirs, dir)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("工作目录不存在")
	}

	return s.Save(newDirs)
}

// SetDefault 设置默认
func (s *DirectoryService) SetDefault(id string) error {
	directories, err := s.Load()
	if err != nil {
		return err
	}

	found := false
	for _, dir := range directories {
		if dir.ID == id {
			dir.IsDefault = true
			found = true
		} else {
			dir.IsDefault = false
		}
	}

	if !found {
		return fmt.Errorf("工作目录不存在")
	}

	return s.Save(directories)
}

// GetDefault 获取默认目录
func (s *DirectoryService) GetDefault() (*model.Directory, error) {
	directories, err := s.Load()
	if err != nil {
		return nil, err
	}

	for _, dir := range directories {
		if dir.IsDefault {
			return dir, nil
		}
	}

	if len(directories) > 0 {
		return directories[0], nil
	}

	return nil, fmt.Errorf("没有配置工作目录")
}

// Reorder 按给定 id 顺序重排目录
func (s *DirectoryService) Reorder(ids []string) error {
	directories, err := s.Load()
	if err != nil {
		return err
	}

	if len(ids) != len(directories) {
		return fmt.Errorf("排序 id 数量(%d)与实际目录数(%d)不一致", len(ids), len(directories))
	}

	// 构建查找表
	dirMap := make(map[string]*model.Directory, len(directories))
	for _, dir := range directories {
		dirMap[dir.ID] = dir
	}

	// 按新顺序排列，同时校验 id 有效且无重复
	reordered := make([]*model.Directory, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return fmt.Errorf("排序 id 重复: %s", id)
		}
		seen[id] = true
		dir, ok := dirMap[id]
		if !ok {
			return fmt.Errorf("工作目录不存在: %s", id)
		}
		reordered = append(reordered, dir)
	}

	return s.Save(reordered)
}
