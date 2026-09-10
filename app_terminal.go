package main

import (
	"workbench/model"
)

// ===== 终端相关 =====

// CreateTerminal 创建终端会话
func (a *App) CreateTerminal(dir, shellType string, cols, rows uint16) (string, error) {
	var customPath string
	settings, err := a.settingsSvc.Load()
	if err == nil {
		switch shellType {
		case "gitbash":
			customPath = settings.GitBashPath
		}
	}
	return a.terminalSvc.CreateTerminal(dir, shellType, customPath, cols, rows)
}

// WriteTerminalInput 向终端写入用户输入
func (a *App) WriteTerminalInput(sessionID, input string) error {
	return a.terminalSvc.WriteInput(sessionID, input)
}

// ChangeTerminalDir 切换终端工作目录
func (a *App) ChangeTerminalDir(sessionID, dir string) error {
	return a.terminalSvc.ChangeDir(sessionID, dir)
}

// ResizeTerminal 调整终端窗口大小
func (a *App) ResizeTerminal(sessionID string, cols, rows uint16) error {
	return a.terminalSvc.Resize(sessionID, cols, rows)
}

// CloseTerminal 关闭终端会话
func (a *App) CloseTerminal(sessionID string) error {
	return a.terminalSvc.CloseTerminal(sessionID)
}

// GetShellConfigs 获取可用的 Shell 配置列表
func (a *App) GetShellConfigs() []model.ShellConfig {
	return model.GetShellConfigs()
}
