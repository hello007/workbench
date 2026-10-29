//go:build !windows

package service

import (
	"os/exec"
	"path/filepath"

	"workbench/util"
)

// openInFileManager 打开目录或文件：本机可用 tessoa 时优先经 tessoa 打开（目录与
// 文件均传原路径，文件场景由 tessoa 自行定位所在目录）；tessoa 不可用或启动失败时
// 降级平台文件管理器。Linux 无「定位选中文件」的通用等价物（对应 Windows 的
// explorer /select,），降级路径文件场景打开其所在父目录。
func openInFileManager(path string, isDir bool, tessoaPath string) error {
	if exe := resolveTessoaExecutable(tessoaPath); exe != "" {
		if err := exec.Command(exe, path).Start(); err == nil {
			return nil
		}
	}
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
