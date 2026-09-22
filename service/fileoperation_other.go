//go:build !windows

package service

import (
	"os/exec"
	"path/filepath"

	"workbench/util"
)

// openInFileManager 用平台文件管理器打开目录。
// Linux 无「定位选中文件」的通用等价物（对应 Windows 的 explorer /select,），
// 文件场景降级为打开其所在父目录。
func openInFileManager(path string, isDir bool) error {
	target := path
	if !isDir {
		target = filepath.Dir(path)
	}
	return openWithSystemHandler(target)
}

// openWithSystemHandler 用系统默认关联程序打开文件路径或 URI（xdg-open）。
// xdg-open 未安装时返回启动错误，由调用方提示。
func openWithSystemHandler(target string) error {
	cmd := exec.Command("xdg-open", target)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}
