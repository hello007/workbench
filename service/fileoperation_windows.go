//go:build windows

package service

import (
	"os/exec"

	"workbench/util"
)

// openInFileManager 打开目录或文件：本机可用 tessoa 时优先经 tessoa 打开（目录与
// 文件均传原路径，文件场景由 tessoa 自行定位所在目录）；tessoa 不可用或启动失败时
// 降级 Windows 资源管理器（目录直接打开，文件定位选中 explorer /select,）。
// 降级路径与原实现逐字节一致（Windows 主路径零回归红线）。
func openInFileManager(path string, isDir bool, tessoaPath string) error {
	if exe := resolveTessoaExecutable(tessoaPath); exe != "" {
		if err := exec.Command(exe, path).Start(); err == nil {
			return nil
		}
	}
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
