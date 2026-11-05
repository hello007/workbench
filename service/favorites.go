package service

import (
	"fmt"
	"path/filepath"
	"time"

	"workbench/model"
	"workbench/util"
)

// normalizeFavoritePath 收藏路径归一：filepath.Clean 去掉尾分隔符与冗余分隔符，
// 使 D:\repo\ 与 D:\repo 视为同一路径（判重、删除、更新别名/分组同口径）。
// 空串保持空串（Clean("") 返回 "." 会误指向进程工作目录）。
func normalizeFavoritePath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

// FavoritesService 收藏夹服务
type FavoritesService struct {
	configPath string
}

// FavoritesConfig 收藏夹配置文件结构
type FavoritesConfig struct {
	Favorites []*model.Favorite `json:"favorites"`
}

// NewFavoritesService 创建收藏夹服务实例
func NewFavoritesService(configPath string) *FavoritesService {
	return &FavoritesService{configPath: configPath}
}

// Load 加载所有收藏
//
// 返回前对每条路径做内存归一（旧数据可能带尾分隔符），不强制落盘迁移：
// 归一结果在后续任一写操作（Add/Remove/UpdateAlias/UpdateGroup）落盘时自然持久化。
func (s *FavoritesService) Load() ([]*model.Favorite, error) {
	if !util.FileExists(s.configPath) {
		return []*model.Favorite{}, nil
	}
	var config FavoritesConfig
	err := util.LoadJSON(s.configPath, &config)
	if err != nil {
		return nil, err
	}
	for _, f := range config.Favorites {
		if f != nil {
			f.Path = normalizeFavoritePath(f.Path)
		}
	}
	return config.Favorites, nil
}

// save 持久化收藏列表
func (s *FavoritesService) save(favorites []*model.Favorite) error {
	config := FavoritesConfig{Favorites: favorites}
	return util.SaveJSON(s.configPath, config)
}

// Add 添加收藏（重复路径或超出上限时返回错误）
func (s *FavoritesService) Add(path, alias, group string) error {
	path = normalizeFavoritePath(path)

	favorites, err := s.Load()
	if err != nil {
		return err
	}

	if len(favorites) >= 100 {
		return fmt.Errorf("收藏夹已满（最多 100 条）")
	}

	for _, f := range favorites {
		if f.Path == path {
			return fmt.Errorf("该路径已收藏")
		}
	}

	if group == "" {
		group = "默认"
	}

	favorites = append(favorites, &model.Favorite{
		Path:      path,
		Alias:     alias,
		Group:     group,
		CreatedAt: time.Now().UnixMilli(),
	})
	return s.save(favorites)
}

// Remove 删除指定路径的收藏
func (s *FavoritesService) Remove(path string) error {
	path = normalizeFavoritePath(path)

	favorites, err := s.Load()
	if err != nil {
		return err
	}

	var filtered []*model.Favorite
	for _, f := range favorites {
		if f.Path != path {
			filtered = append(filtered, f)
		}
	}
	return s.save(filtered)
}

// UpdateAlias 更新收藏别名
func (s *FavoritesService) UpdateAlias(path, alias string) error {
	path = normalizeFavoritePath(path)

	favorites, err := s.Load()
	if err != nil {
		return err
	}

	for _, f := range favorites {
		if f.Path == path {
			f.Alias = alias
			return s.save(favorites)
		}
	}
	return fmt.Errorf("收藏不存在")
}

// UpdateGroup 更新收藏分组
func (s *FavoritesService) UpdateGroup(path, group string) error {
	path = normalizeFavoritePath(path)

	favorites, err := s.Load()
	if err != nil {
		return err
	}

	for _, f := range favorites {
		if f.Path == path {
			f.Group = group
			return s.save(favorites)
		}
	}
	return fmt.Errorf("收藏不存在")
}
