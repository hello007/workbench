//go:build windows

package util

import (
	"os/exec"
	"syscall"
)

func HideCommandWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
