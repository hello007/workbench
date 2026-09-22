package main

import (
	"workbench/model"
)

// ===== 设置域 =====

// GetSettings 获取应用设置
func (a *App) GetSettings() *model.AppSettings {
	settings, err := a.settingsSvc.Load()
	if err != nil {
		settings = &model.AppSettings{}
	}
	// webServe 段默认值兜底（Load 正常路径已补，此处覆盖 settingsSvc 异常分支），
	// 保证前端拿到的配置段始终完整
	settings.EnsureWebServeDefaults()
	// 终端外观三字段兜底（同上，覆盖 settingsSvc 异常分支）
	settings.EnsureTerminalDefaults()
	return settings
}

// SaveSettings 保存应用设置
func (a *App) SaveSettings(settings *model.AppSettings) error {
	return a.settingsSvc.Save(settings)
}
