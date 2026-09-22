//go:build !windows

package service

import "syscall"

// hideWindow 非 Windows 平台无批处理脚本窗口概念，返回 nil（exec.Cmd 默认行为）。
// Linux 更新走 shell 脚本（PR2 实现），启动方式由终端/桌面环境决定。
func hideWindow() *syscall.SysProcAttr {
	return nil
}
