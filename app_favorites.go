package main

import (
	"workbench/model"
)

// ===== 收藏相关 =====

// GetFavorites 获取所有收藏
func (a *App) GetFavorites() []*model.Favorite {
	favorites, err := a.favoritesSvc.Load()
	if err != nil {
		println("GetFavorites error:", err.Error())
		return []*model.Favorite{}
	}
	return favorites
}

// AddFavorite 添加收藏
func (a *App) AddFavorite(path, alias, group string) string {
	err := a.favoritesSvc.Add(path, alias, group)
	if err != nil {
		return err.Error()
	}
	return ""
}

// RemoveFavorite 移除收藏
func (a *App) RemoveFavorite(path string) string {
	err := a.favoritesSvc.Remove(path)
	if err != nil {
		return err.Error()
	}
	return ""
}

// UpdateFavoriteAlias 更新收藏别名
func (a *App) UpdateFavoriteAlias(path, alias string) string {
	err := a.favoritesSvc.UpdateAlias(path, alias)
	if err != nil {
		return err.Error()
	}
	return ""
}

// UpdateFavoriteGroup 更新收藏分组
func (a *App) UpdateFavoriteGroup(path, group string) string {
	err := a.favoritesSvc.UpdateGroup(path, group)
	if err != nil {
		return err.Error()
	}
	return ""
}
