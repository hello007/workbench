//go:build !windows

package util

import "os/exec"

func HideCommandWindow(cmd *exec.Cmd) {}
