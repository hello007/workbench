package main

import (
	"workbench/model"
)

// ===== 设置域 =====

// GetSettings 获取应用设置
func (a *App) GetSettings() *model.AppSettings {
	settings, err := a.settingsSvc.Load()
	if err != nil {
		return &model.AppSettings{}
	}
	return settings
}

// SaveSettings 保存应用设置
func (a *App) SaveSettings(settings *model.AppSettings) error {
	return a.settingsSvc.Save(settings)
}
