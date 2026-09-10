package main

import (
	"errors"

	"workbench/service"
)

// ===== 外部工具打开域 =====

// OpenInExplorer 在资源管理器中打开
func (a *App) OpenInExplorer(path string) bool {
	err := a.fileOpSvc.OpenInExplorer(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// OpenInVSCode 用 VSCode 打开
func (a *App) OpenInVSCode(path string) bool {
	err := a.fileOpSvc.OpenInVSCode(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// OpenInWarp 用 Warp 终端打开
func (a *App) OpenInWarp(path string) bool {
	err := a.fileOpSvc.OpenInWarp(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// OpenInObsidian 用 Obsidian 打开指定路径对应的 vault（文件夹->自身，文件->父目录）。
// 若设置中配置了 Obsidian 程序路径则优先使用，否则走系统协议方案。
// 返回状态码：""=成功、"not-installed"=未检测到 Obsidian、"not-registered"=路径未注册为 vault。
// 其他错误记日志并返回 "not-installed" 兜底，确保前端可处理。
func (a *App) OpenInObsidian(path string) string {
	var obsidianPath string
	if settings, err := a.settingsSvc.Load(); err == nil {
		obsidianPath = settings.ObsidianPath
	}
	err := a.fileOpSvc.OpenInObsidian(path, obsidianPath)
	if err == nil {
		return ""
	}
	if errors.Is(err, service.ErrVaultNotRegistered) {
		return "not-registered"
	}
	if errors.Is(err, service.ErrObsidianNotInstalled) {
		return "not-installed"
	}
	println("Error:", err.Error())
	return "not-installed"
}

// OpenObsidianVaultManager 打开 Obsidian 仓库管理器（obsidian://choose-vault），
// 供用户手动将目录添加为 vault。成功返回 true。
func (a *App) OpenObsidianVaultManager() bool {
	var obsidianPath string
	if settings, err := a.settingsSvc.Load(); err == nil {
		obsidianPath = settings.ObsidianPath
	}
	err := a.fileOpSvc.OpenObsidianVaultManager(obsidianPath)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// CopyObsidianVaultPath 将路径对应的 vault 路径文本复制到系统剪贴板。
// vault 解析：文件夹->自身，文件->父目录。成功返回 true。
// 供「打开仓库管理器」前复制路径，便于用户在 Obsidian 路径栏粘贴。
func (a *App) CopyObsidianVaultPath(path string) bool {
	err := a.fileOpSvc.CopyObsidianVaultPath(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// AutoRegisterAndOpen 自动将路径对应目录注册为 Obsidian vault 并打开。
// 返回状态码：""=成功、"running"=Obsidian 运行中（需关闭后重试）、"not-installed"=未检测到 Obsidian、"failed"=其他失败。
// 其他错误记日志并返回 "failed"，确保前端可分流提示。
func (a *App) AutoRegisterAndOpen(path string) string {
	var obsidianPath string
	if settings, err := a.settingsSvc.Load(); err == nil {
		obsidianPath = settings.ObsidianPath
	}
	err := a.fileOpSvc.AutoRegisterAndOpen(path, obsidianPath)
	if err == nil {
		return ""
	}
	if errors.Is(err, service.ErrObsidianRunning) {
		return "running"
	}
	if errors.Is(err, service.ErrObsidianNotInstalled) {
		return "not-installed"
	}
	println("Error:", err.Error())
	return "failed"
}

// OpenWithDefaultApp 用系统默认程序打开文件
func (a *App) OpenWithDefaultApp(path string) bool {
	err := a.fileOpSvc.OpenWithDefaultApp(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}
