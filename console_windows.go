//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func init() {
	if len(os.Args) < 2 || os.Args[1] != "--version" {
		return
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")

	// 检查 stdout 是否已连接（管道/重定向场景）
	// GetFileType: FILE_TYPE_PIPE=3, FILE_TYPE_DISK=1
	getFileType := kernel32.NewProc("GetFileType")
	if ft, _, _ := getFileType.Call(os.Stdout.Fd()); ft == 3 || ft == 1 {
		return // stdout 已是管道或文件，无需处理
	}

	// stdout 未连接，尝试附加到父进程控制台（cmd.exe / PowerShell）
	attachConsole := kernel32.NewProc("AttachConsole")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	if r1, _, _ := attachConsole.Call(^uintptr(0)); r1 != 0 {
		if h, _, _ := getStdHandle.Call(^uintptr(11)); h != 0 && h != ^uintptr(0) {
			os.Stdout = os.NewFile(h, "stdout")
		}
	}
}

func consolePrint(msg string) {
	fmt.Print(msg)
}
