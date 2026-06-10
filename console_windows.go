//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func consolePrint(msg string) {
	// 尝试直接用 os.Stdout 输出（适用于管道/重定向场景）
	if _, err := os.Stdout.Write([]byte(msg)); err == nil {
		os.Stdout.Sync()
		return
	}

	// GUI 模式下 os.Stdout 可能无效，尝试 AttachConsole 附加到父进程控制台
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attachConsole := kernel32.NewProc("AttachConsole")
	getStdHandle := kernel32.NewProc("GetStdHandle")

	// ATTACH_PARENT_PROCESS = -1
	if r1, _, _ := attachConsole.Call(^uintptr(0)); r1 != 0 {
		// STD_OUTPUT_HANDLE = -11, ^uintptr(10) = NOT(10) = -11
		if h, _, _ := getStdHandle.Call(^uintptr(10)); h != 0 && h != ^uintptr(0) {
			f := os.NewFile(h, "stdout")
			fmt.Fprint(f, msg)
			f.Sync()
			return
		}
	}

	// 最后回退到 stderr
	fmt.Fprint(os.Stderr, msg)
}
