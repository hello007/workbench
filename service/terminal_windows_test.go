//go:build windows

package service

import "testing"

// 以下断言依赖 Windows 平台的 Shell 类型（powershell/cmd/gitbash）与
// Windows 路径分隔符语义（filepath.Clean 的 / → \ 归一），
// Linux 侧对应断言见 terminal_test.go 的跨平台用例。

// TestResolveShellConfig_Default Windows 默认 Shell 为 PowerShell。
func TestResolveShellConfig_Default(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("powershell", "")
	if config.Type != "powershell" {
		t.Errorf("期望 shellType=powershell, 实际=%s", config.Type)
	}
	if config.Executable != "powershell.exe" {
		t.Errorf("期望 executable=powershell.exe, 实际=%s", config.Executable)
	}
}

// TestResolveShellConfig_CustomPath gitbash 类型解析 + 自定义路径覆盖。
func TestResolveShellConfig_CustomPath(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("gitbash", "D:\\custom\\bash.exe")
	if config.Executable != "D:\\custom\\bash.exe" {
		t.Errorf("期望自定义路径, 实际=%s", config.Executable)
	}
}

// TestResolveShellConfig_UnknownType 未知类型回退到 PowerShell（Windows 默认）。
func TestResolveShellConfig_UnknownType(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("unknown", "")
	if config.Type != "powershell" {
		t.Errorf("未知类型应回退到 powershell, 实际=%s", config.Type)
	}
}

// TestBuildCdCommand_PathNormalization cmd 分支依赖 Windows 分隔符语义：
// filepath.Clean 在 Windows 上会将 / 转为 \（Linux 上 \ 为合法文件名字符不参与归一）。
func TestBuildCdCommand_PathNormalization(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand("C:/Users/test", "cmd")
	expected := `cd /d "C:\Users\test"` + "\r"
	if cmd != expected {
		t.Errorf("路径应被规范化, 期望=%q, 实际=%q", expected, cmd)
	}
}
