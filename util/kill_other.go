//go:build !windows

package util

import (
	"fmt"
	"syscall"
)

// KillProcessTree 杀掉指定 PID 及其全部子进程（非 Windows 平台）。
// 通过 SIGKILL 向进程组发信号；若 setpgid 失败（进程已是别的组员）则直接杀进程。
func KillProcessTree(pid int) error {
	// SIGKILL=-9：先尝试杀整个进程组（子进程同组时一并终止）
	if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill %d 失败: %w", pid, err)
	}
	return nil
}
