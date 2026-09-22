//go:build windows

package service

import (
	"os/exec"

	"workbench/util"
)

// openInFileManager 在 Windows 资源管理器中打开目录，或定位选中文件（explorer /select,）。
// 与原 OpenInExplorer 内联实现逐字节一致（Windows 主路径零回归红线）。
func openInFileManager(path string, isDir bool) error {
	if isDir {
		return exec.Command("explorer", path).Start()
	}
	return exec.Command("explorer", "/select,"+path).Start()
}

// openWithSystemHandler 用系统默认关联程序打开文件路径或 URI
// （cmd /c start "" <target>，start 首个引号参数为窗口标题占位）。
func openWithSystemHandler(target string) error {
	cmd := exec.Command("cmd", "/c", "start", "", target)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}
