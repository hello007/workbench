package model

import (
	"encoding/json"
	"testing"
)

func TestAppSettings_DefaultValues(t *testing.T) {
	s := &AppSettings{}
	if s.GpuDisabled != false {
		t.Error("GpuDisabled 默认应为 false")
	}
	if s.DefaultShell != "" {
		t.Error("DefaultShell 默认应为空")
	}
	if s.GitBashPath != "" {
		t.Error("GitBashPath 默认应为空")
	}
	if s.WslDistro != "" {
		t.Error("WslDistro 默认应为空")
	}
	if s.ThemeMode != "" {
		t.Error("ThemeMode 默认应为空（空值视为 system）")
	}
}

func TestAppSettings_JSONSerialization(t *testing.T) {
	s := &AppSettings{
		GpuDisabled:  true,
		DefaultShell: "gitbash",
		GitBashPath:  "D:\\custom\\bash.exe",
		WslDistro:    "Ubuntu",
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded AppSettings
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if decoded.DefaultShell != "gitbash" {
		t.Errorf("DefaultShell: 期望 gitbash, 实际=%s", decoded.DefaultShell)
	}
	if decoded.GitBashPath != "D:\\custom\\bash.exe" {
		t.Errorf("GitBashPath: 期望 D:\\custom\\bash.exe, 实际=%s", decoded.GitBashPath)
	}
	if decoded.WslDistro != "Ubuntu" {
		t.Errorf("WslDistro: 期望 Ubuntu, 实际=%s", decoded.WslDistro)
	}
	if !decoded.GpuDisabled {
		t.Error("GpuDisabled 应为 true")
	}
}

func TestAppSettings_JSONDeserialization_Partial(t *testing.T) {
	// 测试只有部分字段的 JSON（向后兼容）
	jsonStr := `{"gpuDisabled":true}`
	var s AppSettings
	if err := json.Unmarshal([]byte(jsonStr), &s); err != nil {
		t.Fatalf("部分字段反序列化失败: %v", err)
	}
	if !s.GpuDisabled {
		t.Error("GpuDisabled 应为 true")
	}
	if s.DefaultShell != "" {
		t.Error("缺失字段应为零值")
	}
	if s.ThemeMode != "" {
		t.Error("ThemeMode 缺失时应为零值")
	}
}

func TestAppSettings_ThemeModeRoundTrip(t *testing.T) {
	// themeMode 字段需完整往返：序列化 → 反序列化保留原值
	// 该字段是前端主题持久化落盘的唯一存储位，丢失即重启后主题回退
	s := &AppSettings{ThemeMode: "dark"}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var decoded AppSettings
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if decoded.ThemeMode != "dark" {
		t.Errorf("ThemeMode 往返丢失: 期望 dark, 实际=%s", decoded.ThemeMode)
	}
}

// TestAppSettings_EnsureWebServeDefaults 覆盖 webServe 段默认值补全三分支：
// 段缺失补默认开启 + 回环地址、地址为空仅补地址、已配置完整不覆盖。
func TestAppSettings_EnsureWebServeDefaults(t *testing.T) {
	cases := []struct {
		name         string
		in           *WebServeSettings
		wantEnabled  bool
		wantBindAddr string
	}{
		{
			name:         "段缺失补默认开启与回环地址",
			in:           nil,
			wantEnabled:  true,
			wantBindAddr: DefaultWebServeBindAddress,
		},
		{
			name:         "段存在地址为空仅补地址enabled保留",
			in:           &WebServeSettings{Enabled: false},
			wantEnabled:  false,
			wantBindAddr: DefaultWebServeBindAddress,
		},
		{
			name:         "已配置完整值不覆盖",
			in:           &WebServeSettings{Enabled: false, BindAddress: "0.0.0.0:8080"},
			wantEnabled:  false,
			wantBindAddr: "0.0.0.0:8080",
		},
		{
			name:         "段存在显式开启地址为空补默认地址",
			in:           &WebServeSettings{Enabled: true},
			wantEnabled:  true,
			wantBindAddr: DefaultWebServeBindAddress,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &AppSettings{WebServe: c.in}
			s.EnsureWebServeDefaults()
			if s.WebServe == nil {
				t.Fatal("EnsureWebServeDefaults 后 WebServe 段不应为 nil")
			}
			if s.WebServe.Enabled != c.wantEnabled {
				t.Errorf("Enabled: 期望 %v, 实际=%v", c.wantEnabled, s.WebServe.Enabled)
			}
			if s.WebServe.BindAddress != c.wantBindAddr {
				t.Errorf("BindAddress: 期望 %s, 实际=%s", c.wantBindAddr, s.WebServe.BindAddress)
			}
		})
	}
}

// TestAppSettings_EnsureTerminalDefaults 覆盖终端外观默认值与越界收敛：
// 零值回默认、低于下限收敛下限、高于上限收敛上限、合法值不动。
func TestAppSettings_EnsureTerminalDefaults(t *testing.T) {
	cases := []struct {
		name           string
		inFontSize     int
		inScrollback   int
		wantFontSize   int
		wantScrollback int
	}{
		{name: "双零值回默认", inFontSize: 0, inScrollback: 0, wantFontSize: DefaultTerminalFontSize, wantScrollback: DefaultTerminalScrollback},
		{name: "字号低于下限收敛10", inFontSize: 5, inScrollback: DefaultTerminalScrollback, wantFontSize: MinTerminalFontSize, wantScrollback: DefaultTerminalScrollback},
		{name: "字号高于上限收敛24", inFontSize: 100, inScrollback: DefaultTerminalScrollback, wantFontSize: MaxTerminalFontSize, wantScrollback: DefaultTerminalScrollback},
		{name: "回滚低于下限收敛1000", inFontSize: DefaultTerminalFontSize, inScrollback: 100, wantFontSize: DefaultTerminalFontSize, wantScrollback: MinTerminalScrollback},
		{name: "回滚高于上限收敛10000", inFontSize: DefaultTerminalFontSize, inScrollback: 99999, wantFontSize: DefaultTerminalFontSize, wantScrollback: MaxTerminalScrollback},
		{name: "合法值不动", inFontSize: 16, inScrollback: 5000, wantFontSize: 16, wantScrollback: 5000},
		{name: "边界值恰好等于上下限不动", inFontSize: MinTerminalFontSize, inScrollback: MaxTerminalScrollback, wantFontSize: MinTerminalFontSize, wantScrollback: MaxTerminalScrollback},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &AppSettings{TerminalFontSize: c.inFontSize, TerminalScrollback: c.inScrollback}
			s.EnsureTerminalDefaults()
			if s.TerminalFontSize != c.wantFontSize {
				t.Errorf("TerminalFontSize: 期望 %d, 实际=%d", c.wantFontSize, s.TerminalFontSize)
			}
			if s.TerminalScrollback != c.wantScrollback {
				t.Errorf("TerminalScrollback: 期望 %d, 实际=%d", c.wantScrollback, s.TerminalScrollback)
			}
		})
	}
}
