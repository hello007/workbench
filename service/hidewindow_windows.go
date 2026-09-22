//go:build windows

package service

import "syscall"

// hideWindow 返回 SysProcAttr 用于隐藏批处理脚本窗口
func hideWindow() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
