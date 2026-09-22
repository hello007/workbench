package model

import (
	"runtime"
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

// TestGetShellConfigs_Count 各平台 Shell 配置均为 4 项（Windows 与 Linux 列表等宽）。
func TestGetShellConfigs_Count(t *testing.T) {
	configs := GetShellConfigs()
	if len(configs) != 4 {
		t.Errorf("期望 4 种 Shell 配置, 实际=%d", len(configs))
	}
}

// TestGetShellConfigs_PlatformTypes 各平台 Shell 类型集合与默认顺序（首项为默认 Shell）。
func TestGetShellConfigs_PlatformTypes(t *testing.T) {
	configs := GetShellConfigs()
	var wantOrder []string
	switch runtime.GOOS {
	case "windows":
		wantOrder = []string{"powershell", "cmd", "gitbash", "wsl"}
	case "linux":
		wantOrder = []string{"bash", "zsh", "fish", "sh"}
	default:
		t.Skipf("平台 %s 无既定 Shell 列表断言", runtime.GOOS)
	}
	if len(configs) != len(wantOrder) {
		t.Fatalf("期望 %d 项配置, 实际=%d", len(wantOrder), len(configs))
	}
	for i, want := range wantOrder {
		if configs[i].Type != want {
			t.Errorf("第 %d 项应为 %s, 实际=%s", i+1, want, configs[i].Type)
		}
	}
}

// TestGetShellConfigs_EntriesUnique 跨平台通用不变式：Type 唯一且 Executable 非空。
func TestGetShellConfigs_EntriesUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, c := range GetShellConfigs() {
		if c.Type == "" {
			t.Error("Shell 类型不应为空")
		}
		if seen[c.Type] {
			t.Errorf("Shell 类型重复: %s", c.Type)
		}
		seen[c.Type] = true
		if c.Executable == "" {
			t.Errorf("%s: Executable 不应为空", c.Type)
		}
	}
}

// TestResolveShellConfig_KnownType 解析当前平台列表中真实存在的类型：
// 返回同类型配置，customPath 为空时用列表默认路径。
func TestResolveShellConfig_KnownType(t *testing.T) {
	configs := GetShellConfigs()
	first := configs[0]

	config := ResolveShellConfig(first.Type, "")
	if config.Type != first.Type {
		t.Errorf("期望 Type=%s, 实际=%s", first.Type, config.Type)
	}
	if config.Executable != first.Executable {
		t.Errorf("空自定义路径应用列表默认路径, 期望=%s, 实际=%s", first.Executable, config.Executable)
	}
}

// TestResolveShellConfig_CustomPathOverride 解析当前平台列表中的类型并覆盖自定义路径。
func TestResolveShellConfig_CustomPathOverride(t *testing.T) {
	first := GetShellConfigs()[0]
	const custom = "/custom/path/override"
	config := ResolveShellConfig(first.Type, custom)
	if config.Executable != custom {
		t.Errorf("自定义路径应覆盖默认路径, 期望=%s, 实际=%s", custom, config.Executable)
	}
}

// TestResolveShellConfig_UnknownTypeFallsBackToDefault 未知类型回退到平台默认 Shell。
func TestResolveShellConfig_UnknownTypeFallsBackToDefault(t *testing.T) {
	config := ResolveShellConfig("no_such_shell_type", "")
	wantType := "bash"
	if runtime.GOOS == "windows" {
		wantType = "powershell"
	}
	if config.Type != wantType {
		t.Errorf("未知类型应回退到平台默认 %s, 实际=%s", wantType, config.Type)
	}
}
