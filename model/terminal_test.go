package model

import (
	"sync"
	"testing"
)

func TestTerminalSession_SetRunning_IsRunning(t *testing.T) {
	s := &TerminalSession{}
	if s.IsRunning() {
		t.Error("新会话应该不是运行状态")
	}
	s.SetRunning(true)
	if !s.IsRunning() {
		t.Error("SetRunning(true) 后应该是运行状态")
	}
	s.SetRunning(false)
	if s.IsRunning() {
		t.Error("SetRunning(false) 后应该不是运行状态")
	}
}

func TestTerminalSession_ConcurrentAccess(t *testing.T) {
	s := &TerminalSession{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(val bool) {
			defer wg.Done()
			s.SetRunning(val)
			_ = s.IsRunning()
		}(i%2 == 0)
	}
	wg.Wait()
}

func TestGetShellConfigs_Count(t *testing.T) {
	configs := GetShellConfigs()
	if len(configs) != 4 {
		t.Errorf("期望 4 种 Shell 配置, 实际=%d", len(configs))
	}
}

func TestGetShellConfigs_Fields(t *testing.T) {
	configs := GetShellConfigs()
	expected := map[string]ShellConfig{
		"powershell": {Type: "powershell", Executable: "powershell.exe", DisplayName: "PowerShell"},
		"cmd":        {Type: "cmd", Executable: "cmd.exe", DisplayName: "CMD"},
		"gitbash":    {Type: "gitbash", Executable: `C:\Program Files\Git\bin\bash.exe`, DisplayName: "Git Bash"},
		"wsl":        {Type: "wsl", Executable: "wsl.exe", DisplayName: "WSL"},
	}
	for _, c := range configs {
		exp, ok := expected[c.Type]
		if !ok {
			t.Errorf("未预期的 Shell 类型: %s", c.Type)
			continue
		}
		if c.Executable != exp.Executable {
			t.Errorf("%s: 期望 Executable=%s, 实际=%s", c.Type, exp.Executable, c.Executable)
		}
		if c.DisplayName != exp.DisplayName {
			t.Errorf("%s: 期望 DisplayName=%s, 实际=%s", c.Type, exp.DisplayName, c.DisplayName)
		}
	}
}

func TestGetShellConfigs_GitBashHasArgs(t *testing.T) {
	configs := GetShellConfigs()
	for _, c := range configs {
		if c.Type == "gitbash" {
			if len(c.Args) == 0 {
				t.Error("Git Bash 配置应包含 Args")
			}
			return
		}
	}
	t.Error("未找到 gitbash 配置")
}

func TestResolveShellConfig_PowerShell(t *testing.T) {
	config := ResolveShellConfig("powershell", "")
	if config.Type != "powershell" {
		t.Errorf("期望 powershell, 实际=%s", config.Type)
	}
	if config.Executable != "powershell.exe" {
		t.Errorf("期望 powershell.exe, 实际=%s", config.Executable)
	}
}

func TestResolveShellConfig_CMD(t *testing.T) {
	config := ResolveShellConfig("cmd", "")
	if config.Type != "cmd" {
		t.Errorf("期望 cmd, 实际=%s", config.Type)
	}
}

func TestResolveShellConfig_GitBash(t *testing.T) {
	config := ResolveShellConfig("gitbash", "")
	if config.Type != "gitbash" {
		t.Errorf("期望 gitbash, 实际=%s", config.Type)
	}
}

func TestResolveShellConfig_WSL(t *testing.T) {
	config := ResolveShellConfig("wsl", "")
	if config.Type != "wsl" {
		t.Errorf("期望 wsl, 实际=%s", config.Type)
	}
}

func TestResolveShellConfig_CustomPathOverride(t *testing.T) {
	config := ResolveShellConfig("gitbash", "D:\\custom\\bash.exe")
	if config.Executable != "D:\\custom\\bash.exe" {
		t.Errorf("自定义路径应覆盖默认路径, 实际=%s", config.Executable)
	}
}

func TestResolveShellConfig_EmptyCustomPath(t *testing.T) {
	config := ResolveShellConfig("gitbash", "")
	if config.Executable != `C:\Program Files\Git\bin\bash.exe` {
		t.Errorf("空自定义路径应使用默认路径, 实际=%s", config.Executable)
	}
}

func TestResolveShellConfig_UnknownType(t *testing.T) {
	config := ResolveShellConfig("unknown_shell", "")
	if config.Type != "powershell" {
		t.Errorf("未知类型应回退到 powershell, 实际=%s", config.Type)
	}
}
