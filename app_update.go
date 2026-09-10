package main

import (
	"os"

	"workbench/model"
)

// ===== 更新相关 =====

// CheckForUpdate 检查是否有新版本
func (a *App) CheckForUpdate() (*model.UpdateInfo, error) {
	return a.updateSvc.CheckForUpdate(version)
}

// DownloadUpdate 下载新版本
func (a *App) DownloadUpdate(downloadURL string) error {
	return a.updateSvc.DownloadUpdate(downloadURL)
}

// CancelDownload 取消下载
func (a *App) CancelDownload() {
	a.updateSvc.CancelDownload()
}

// ApplyUpdate 执行更新替换并退出应用
func (a *App) ApplyUpdate() error {
	err := a.updateSvc.ApplyUpdate()
	if err != nil {
		return err
	}
	// 退出当前应用
	os.Exit(0)
	return nil
}
