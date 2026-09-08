//go:build windows

package util

import (
	"fmt"
	"os/exec"
)

// KillProcessTree 杀掉指定 PID 及其全部子进程。
// claude headless 可能 spawn python（MCP）等子进程，仅杀父进程会留孤儿进程，
// Windows 用 taskkill /T /F 递归终止进程树。
func KillProcessTree(pid int) error {
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprintf("%d", pid))
	HideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("taskkill 失败: %v, output: %s", err, string(out))
	}
	return nil
}
