//go:build windows

package util

import (
	"testing"
)

func TestQuoteArg_PlainArg(t *testing.T) {
	result := quoteArg("powershell.exe")
	if result != "powershell.exe" {
		t.Errorf("无空格参数不应加引号, 实际=%s", result)
	}
}

func TestQuoteArg_ArgWithSpaces(t *testing.T) {
	result := quoteArg(`C:\Program Files\Git\bin\bash.exe`)
	if result != `"C:\Program Files\Git\bin\bash.exe"` {
		t.Errorf("含空格参数应加引号, 实际=%s", result)
	}
}

func TestQuoteArg_EmptyArg(t *testing.T) {
	result := quoteArg("")
	if result != `""` {
		t.Errorf("空参数应加引号, 实际=%s", result)
	}
}

func TestQuoteArg_ArgWithQuotes(t *testing.T) {
	result := quoteArg(`say "hello"`)
	if result != `"say \"hello\""` {
		t.Errorf("含双引号参数应转义, 实际=%s", result)
	}
}

func TestBuildCommandLine_NoArgs(t *testing.T) {
	result := buildCommandLine("cmd.exe", nil)
	if result != "cmd.exe" {
		t.Errorf("无参数: 期望 cmd.exe, 实际=%s", result)
	}
}

func TestBuildCommandLine_WithArgs(t *testing.T) {
	result := buildCommandLine("bash.exe", []string{"-i", "-l"})
	if result != `bash.exe -i -l` {
		t.Errorf("带参数: 期望 bash.exe -i -l, 实际=%s", result)
	}
}

func TestBuildCommandLine_PathWithSpaces(t *testing.T) {
	result := buildCommandLine(`C:\Program Files\Git\bin\bash.exe`, []string{"-i"})
	if result != `"C:\Program Files\Git\bin\bash.exe" -i` {
		t.Errorf("路径含空格+参数: 实际=%s", result)
	}
}

func TestNewPtyProcess_InvalidCommand(t *testing.T) {
	_, err := NewPtyProcess("nonexistent_command_xyz.exe", nil, "", 80, 24)
	if err == nil {
		t.Error("无效命令应返回错误")
	}
}

func TestPtyProcess_Close_Nil(t *testing.T) {
	p := &PtyProcess{}
	err := p.Close()
	if err != nil {
		t.Errorf("关闭 nil ConPTY 不应返回错误, 实际=%v", err)
	}
}

func TestPtyProcess_IsProcessRunning_ZeroPid(t *testing.T) {
	p := &PtyProcess{pid: 0}
	if p.IsProcessRunning() {
		t.Error("pid=0 不应判定为运行中")
	}
}

func TestPtyProcess_GetCmd_AlwaysNil(t *testing.T) {
	p := &PtyProcess{}
	if p.GetCmd() != nil {
		t.Error("Windows 实现的 GetCmd 应始终返回 nil")
	}
}
