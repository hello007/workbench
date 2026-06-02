package service

import (
	"testing"
)

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

func TestResolveShellConfig_CustomPath(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("gitbash", "D:\\custom\\bash.exe")
	if config.Executable != "D:\\custom\\bash.exe" {
		t.Errorf("期望自定义路径, 实际=%s", config.Executable)
	}
}

func TestResolveShellConfig_UnknownType(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("unknown", "")
	if config.Type != "powershell" {
		t.Errorf("未知类型应回退到 powershell, 实际=%s", config.Type)
	}
}

func TestBuildCdCommand_Windows(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\workspace\test`)
	expected := `cd /d "D:\workspace\test"` + "\n"
	if cmd != expected {
		t.Errorf("期望 cd 命令=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\my project\test folder`)
	expected := `cd /d "D:\my project\test folder"` + "\n"
	if cmd != expected {
		t.Errorf("期望 cd 命令(含空格)=%q, 实际=%q", expected, cmd)
	}
}
