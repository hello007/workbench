package model

import (
	"encoding/json"
	"reflect"
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

// TestAppSettings_EnsureWebServeDefaults_TLSFields TLS 字段零值语义（内置 TLS
// 三期 PR2）：Ensure 不补 TLS 默认值（TLS 是显式决策项，false = 纯 HTTP 向后
// 兼容老配置），已配置值原样保留；JSON 往返与老配置（缺 tls 字段）反序列化
// 均得零值。
func TestAppSettings_EnsureWebServeDefaults_TLSFields(t *testing.T) {
	// 段缺失补默认后 TLS 三字段应为零值（不默认开启 TLS）
	s := &AppSettings{}
	s.EnsureWebServeDefaults()
	if s.WebServe.TLS {
		t.Error("TLS 默认应为 false（纯 HTTP 向后兼容）")
	}
	if s.WebServe.TLSCertPath != "" || s.WebServe.TLSKeyPath != "" {
		t.Errorf("证书路径默认应为空（双空 = 自签引导）, got %q / %q", s.WebServe.TLSCertPath, s.WebServe.TLSKeyPath)
	}

	// 已配置 TLS 值原样保留（Ensure 不覆盖显式取值）
	s2 := &AppSettings{WebServe: &WebServeSettings{
		Enabled:     true,
		BindAddress: "127.0.0.1:36115",
		TLS:         true,
		TLSCertPath: "a.pem",
		TLSKeyPath:  "b.pem",
	}}
	s2.EnsureWebServeDefaults()
	if !s2.WebServe.TLS || s2.WebServe.TLSCertPath != "a.pem" || s2.WebServe.TLSKeyPath != "b.pem" {
		t.Errorf("已配置 TLS 值应原样保留: %+v", s2.WebServe)
	}

	// JSON 往返：TLS 字段随 webServe 段序列化
	data, err := json.Marshal(s2)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var decoded AppSettings
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if decoded.WebServe == nil || !decoded.WebServe.TLS || decoded.WebServe.TLSCertPath != "a.pem" {
		t.Errorf("TLS 字段应随 JSON 往返保留: %+v", decoded.WebServe)
	}

	// 老配置（JSON 缺 tls 字段）反序列化 → 零值 = 纯 HTTP，行为不变
	var legacy AppSettings
	if err := json.Unmarshal([]byte(`{"webServe":{"enabled":true,"bindAddress":"127.0.0.1:36115"}}`), &legacy); err != nil {
		t.Fatalf("老配置反序列化失败: %v", err)
	}
	if legacy.WebServe.TLS || legacy.WebServe.TLSCertPath != "" || legacy.WebServe.TLSKeyPath != "" {
		t.Errorf("老配置缺 tls 字段应得零值: %+v", legacy.WebServe)
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

// TestAppSettings_EnsureIPBanDefaults 覆盖 IP 封禁参数段默认值与越界收敛（连接
// 管控六期；七期扩白名单归一）：段缺失补四项全默认 + 白名单空切片、零值回默认、
// 越界 clamp、界内保留、封顶不低于基础时长的组合收敛、白名单非法条目剔除。
// 结构含切片字段后不可用 == 比较，统一以 reflect.DeepEqual 断言。
func TestAppSettings_EnsureIPBanDefaults(t *testing.T) {
	t.Run("段缺失补四项全默认", func(t *testing.T) {
		s := &AppSettings{}
		s.EnsureIPBanDefaults()
		want := DefaultIPBanSettings()
		if s.IPBan == nil || !reflect.DeepEqual(*s.IPBan, want) {
			t.Fatalf("段缺失应补默认值, got %+v", s.IPBan)
		}
		if s.IPBan.Whitelist == nil || len(s.IPBan.Whitelist) != 0 {
			t.Errorf("白名单默认应为非 nil 空切片, got %#v", s.IPBan.Whitelist)
		}
	})

	t.Run("零值字段回默认", func(t *testing.T) {
		s := &AppSettings{IPBan: &IPBanSettings{}}
		s.EnsureIPBanDefaults()
		if !reflect.DeepEqual(*s.IPBan, DefaultIPBanSettings()) {
			t.Errorf("零值字段应回默认, got %+v", s.IPBan)
		}
	})

	cases := []struct {
		name string
		in   IPBanSettings
		want IPBanSettings
	}{
		{name: "越界收敛", in: IPBanSettings{Threshold: 1, WindowMinutes: 500, DurationMinutes: 999, MaxBanDurationMinutes: 1},
			want: IPBanSettings{Threshold: MinIPBanThreshold, WindowMinutes: MaxIPBanWindowMinutes, DurationMinutes: MaxIPBanDurationMinutes, MaxBanDurationMinutes: MaxIPBanDurationMinutes, Whitelist: []string{}}},
		{name: "界内取值原样保留", in: IPBanSettings{Threshold: 20, WindowMinutes: 30, DurationMinutes: 10, MaxBanDurationMinutes: 120},
			want: IPBanSettings{Threshold: 20, WindowMinutes: 30, DurationMinutes: 10, MaxBanDurationMinutes: 120, Whitelist: []string{}}},
		{name: "封顶低于基础时长抬到基础时长", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 60, MaxBanDurationMinutes: 5},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 60, MaxBanDurationMinutes: 60, Whitelist: []string{}}},
		{name: "白名单合法条目保留非法剔除", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
			Whitelist: []string{"192.168.1.0/24", " 裸IP不是合法条目 ", "10.0.0.5", "2001:db8::/32", "", "  "}},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
				Whitelist: []string{"192.168.1.0/24", "10.0.0.5", "2001:db8::/32"}}},
		{name: "白名单裸IP保留原字符串", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
			Whitelist: []string{"203.0.113.9", "2001:db8::1"}},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
				Whitelist: []string{"203.0.113.9", "2001:db8::1"}}},
		{name: "白名单nil收敛空切片", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, Whitelist: []string{}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &AppSettings{IPBan: &c.in}
			s.EnsureIPBanDefaults()
			if !reflect.DeepEqual(*s.IPBan, c.want) {
				t.Errorf("clamp 结果不符, got %+v, want %+v", *s.IPBan, c.want)
			}
		})
	}
}

// TestNormalizeIPBanSettings 归一化入口契约：nil → 默认值；非 nil → 拷贝
// normalize 后返回且不改写入参（对齐 NormalizeAssetThumbsSettings 先例）。
func TestNormalizeIPBanSettings(t *testing.T) {
	if got := NormalizeIPBanSettings(nil); !reflect.DeepEqual(got, DefaultIPBanSettings()) {
		t.Fatalf("nil 应返回默认值, got %+v", got)
	}
	in := &IPBanSettings{Threshold: 1, WindowMinutes: 0, DurationMinutes: 60, MaxBanDurationMinutes: 5,
		Whitelist: []string{"192.168.1.0/24", "非法条目"}}
	got := NormalizeIPBanSettings(in)
	if got.Threshold != MinIPBanThreshold || got.WindowMinutes != DefaultIPBanWindowMinutes ||
		got.DurationMinutes != MaxIPBanDurationMinutes || got.MaxBanDurationMinutes != MaxIPBanDurationMinutes {
		t.Fatalf("归一化结果不符, got %+v", got)
	}
	if len(got.Whitelist) != 1 || got.Whitelist[0] != "192.168.1.0/24" {
		t.Errorf("白名单应剔除非法条目, got %#v", got.Whitelist)
	}
	if in.Threshold != 1 || in.WindowMinutes != 0 || len(in.Whitelist) != 2 {
		t.Fatalf("入参不应被原地修改, got %+v", in)
	}
}
