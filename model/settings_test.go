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

// TestAppSettings_EnsureAssetsThumbsDefaults 覆盖素材缩略图段默认值与越界收敛：
// 段缺失补五项全默认、零值回默认、越界 clamp、界内保留、Auto 显式关闭被尊重。
func TestAppSettings_EnsureAssetsThumbsDefaults(t *testing.T) {
	t.Run("段缺失补五项全默认", func(t *testing.T) {
		s := &AppSettings{}
		s.EnsureAssetsThumbsDefaults()
		want := DefaultAssetThumbsSettings()
		if s.Assets == nil || *s.Assets != want {
			t.Fatalf("段缺失应补默认值, got %+v", s.Assets)
		}
	})

	t.Run("零值字段回默认且Auto显式关闭被尊重", func(t *testing.T) {
		s := &AppSettings{Assets: &AssetThumbsSettings{Auto: false}}
		s.EnsureAssetsThumbsDefaults()
		if s.Assets.Auto {
			t.Error("Auto 显式 false 应被尊重（用户主动关闭），不得回退 true")
		}
		if s.Assets.SmallWidth != DefaultAssetThumbSmallWidth ||
			s.Assets.HDWidth != DefaultAssetThumbHDWidth ||
			s.Assets.SmallQuotaMB != DefaultAssetThumbSmallQuotaMB ||
			s.Assets.HDQuotaMB != DefaultAssetThumbHDQuotaMB {
			t.Errorf("零值字段应回默认, got %+v", s.Assets)
		}
	})

	cases := []struct {
		name             string
		smallWidth       int
		hdWidth          int
		smallQuotaMB     int
		hdQuotaMB        int
		wantSmallWidth   int
		wantHDWidth      int
		wantSmallQuotaMB int
		wantHDQuotaMB    int
	}{
		{name: "宽度越界收敛", smallWidth: 50, hdWidth: 9999, smallQuotaMB: 1024, hdQuotaMB: 1024,
			wantSmallWidth: MinAssetThumbSmallWidth, wantHDWidth: MaxAssetThumbHDWidth,
			wantSmallQuotaMB: 1024, wantHDQuotaMB: 1024},
		{name: "配额越界收敛", smallWidth: 320, hdWidth: 1600, smallQuotaMB: 1, hdQuotaMB: 99999,
			wantSmallWidth: 320, wantHDWidth: 1600,
			wantSmallQuotaMB: MinAssetThumbQuotaMB, wantHDQuotaMB: MaxAssetThumbQuotaMB},
		{name: "界内取值原样保留", smallWidth: 480, hdWidth: 2560, smallQuotaMB: 2048, hdQuotaMB: 4096,
			wantSmallWidth: 480, wantHDWidth: 2560, wantSmallQuotaMB: 2048, wantHDQuotaMB: 4096},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &AppSettings{Assets: &AssetThumbsSettings{
				SmallWidth: c.smallWidth, HDWidth: c.hdWidth,
				SmallQuotaMB: c.smallQuotaMB, HDQuotaMB: c.hdQuotaMB,
			}}
			s.EnsureAssetsThumbsDefaults()
			if s.Assets.SmallWidth != c.wantSmallWidth || s.Assets.HDWidth != c.wantHDWidth ||
				s.Assets.SmallQuotaMB != c.wantSmallQuotaMB || s.Assets.HDQuotaMB != c.wantHDQuotaMB {
				t.Errorf("clamp 结果不符, got %+v", s.Assets)
			}
		})
	}
}

// TestNormalizeAssetThumbsSettings 归一化入口契约：nil → 默认值；非 nil → 拷贝
// normalize 后返回且不改写入参（调用方可能传入共享的设置缓存指针）。
func TestNormalizeAssetThumbsSettings(t *testing.T) {
	if got := NormalizeAssetThumbsSettings(nil); got != DefaultAssetThumbsSettings() {
		t.Fatalf("nil 应返回默认值, got %+v", got)
	}
	in := &AssetThumbsSettings{SmallWidth: 99, HDWidth: 0, SmallQuotaMB: 10, HDQuotaMB: 5}
	got := NormalizeAssetThumbsSettings(in)
	if got.SmallWidth != MinAssetThumbSmallWidth || got.HDWidth != DefaultAssetThumbHDWidth ||
		got.SmallQuotaMB != MinAssetThumbQuotaMB || got.HDQuotaMB != MinAssetThumbQuotaMB {
		t.Fatalf("越界值应 clamp, got %+v", got)
	}
	// 入参不被修改（共享缓存指针防竞态纪律）
	if in.SmallWidth != 99 || in.HDWidth != 0 || in.SmallQuotaMB != 10 || in.HDQuotaMB != 5 {
		t.Fatalf("入参不应被原地修改, got %+v", in)
	}
}
