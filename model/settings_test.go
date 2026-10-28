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
	t.Run("段缺失补参数全默认", func(t *testing.T) {
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
			want: IPBanSettings{Threshold: MinIPBanThreshold, WindowMinutes: MaxIPBanWindowMinutes, DurationMinutes: MaxIPBanDurationMinutes, MaxBanDurationMinutes: MaxIPBanDurationMinutes, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: DefaultIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "界内取值原样保留", in: IPBanSettings{Threshold: 20, WindowMinutes: 30, DurationMinutes: 10, MaxBanDurationMinutes: 120},
			want: IPBanSettings{Threshold: 20, WindowMinutes: 30, DurationMinutes: 10, MaxBanDurationMinutes: 120, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: DefaultIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "封顶低于基础时长抬到基础时长", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 60, MaxBanDurationMinutes: 5},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 60, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: DefaultIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "白名单合法条目保留非法剔除", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
			Whitelist: []string{"192.168.1.0/24", " 裸IP不是合法条目 ", "10.0.0.5", "2001:db8::/32", "", "  "}},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: DefaultIPBanTrendWindowDays,
				Whitelist: []string{"192.168.1.0/24", "10.0.0.5", "2001:db8::/32"}}},
		{name: "白名单裸IP保留原字符串", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
			Whitelist: []string{"203.0.113.9", "2001:db8::1"}},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: DefaultIPBanTrendWindowDays,
				Whitelist: []string{"203.0.113.9", "2001:db8::1"}}},
		{name: "白名单nil收敛空切片", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: DefaultIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "聚合建议阈值低于下限收敛", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: 1},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: MinIPBanAggregateSuggestThreshold, TrendWindowDays: DefaultIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "聚合建议阈值越上限收敛", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: 99},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: MaxIPBanAggregateSuggestThreshold, TrendWindowDays: DefaultIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "聚合建议阈值界内保留", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: 25},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: 25, TrendWindowDays: DefaultIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "趋势窗口天数零值回默认", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: DefaultIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "趋势窗口天数低于下限收敛", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, TrendWindowDays: -3},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: MinIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "趋势窗口天数越上限收敛", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, TrendWindowDays: 99},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: MaxIPBanTrendWindowDays, Whitelist: []string{}}},
		{name: "趋势窗口天数界内保留", in: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, TrendWindowDays: 14},
			want: IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: DefaultIPBanAggregateSuggestThreshold, TrendWindowDays: 14, Whitelist: []string{}}},
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
// normalize 后返回且不改写入参（对齐 NormalizeAssetThumbsSettings 先例）；
// 聚合建议阈值（九期）零值回默认与越界 clamp 同口径覆盖。
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
	// 聚合建议阈值零值回默认（九期）
	if got.AggregateSuggestThreshold != DefaultIPBanAggregateSuggestThreshold {
		t.Errorf("聚合建议阈值零值应回默认, got %d", got.AggregateSuggestThreshold)
	}
	// 趋势窗口天数零值回默认（十期）
	if got.TrendWindowDays != DefaultIPBanTrendWindowDays {
		t.Errorf("趋势窗口天数零值应回默认, got %d", got.TrendWindowDays)
	}
	if len(got.Whitelist) != 1 || got.Whitelist[0] != "192.168.1.0/24" {
		t.Errorf("白名单应剔除非法条目, got %#v", got.Whitelist)
	}
	if in.Threshold != 1 || in.WindowMinutes != 0 || len(in.Whitelist) != 2 {
		t.Fatalf("入参不应被原地修改, got %+v", in)
	}
	// 越界 clamp（拷贝返回不改写入参同口径）
	up := NormalizeIPBanSettings(&IPBanSettings{Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60, AggregateSuggestThreshold: 99})
	if up.AggregateSuggestThreshold != MaxIPBanAggregateSuggestThreshold {
		t.Errorf("聚合建议阈值越上限应收敛, got %d", up.AggregateSuggestThreshold)
	}
}

// TestAppSettings_EnsureAuditDefaults 覆盖审计留存段默认值与越界收敛（连接管控
// 十一期）：段缺失补两项全默认、零值回默认、越界 clamp、界内保留。
func TestAppSettings_EnsureAuditDefaults(t *testing.T) {
	t.Run("段缺失补两项全默认", func(t *testing.T) {
		s := &AppSettings{}
		s.EnsureAuditDefaults()
		want := DefaultAuditSettings()
		if s.Audit == nil || *s.Audit != want {
			t.Fatalf("段缺失应补默认值, got %+v", s.Audit)
		}
	})

	t.Run("零值字段回默认", func(t *testing.T) {
		s := &AppSettings{Audit: &AuditSettings{}}
		s.EnsureAuditDefaults()
		if !reflect.DeepEqual(*s.Audit, DefaultAuditSettings()) {
			t.Errorf("零值字段应回默认, got %+v", s.Audit)
		}
	})

	cases := []struct {
		name string
		in   AuditSettings
		want AuditSettings
	}{
		{name: "越界收敛", in: AuditSettings{MaxBytesMB: 0, Depth: 99},
			want: AuditSettings{MaxBytesMB: DefaultAuditMaxBytesMB, Depth: MaxAuditDepth}},
		{name: "低于下限收敛", in: AuditSettings{MaxBytesMB: -3, Depth: 1},
			want: AuditSettings{MaxBytesMB: MinAuditMaxBytesMB, Depth: MinAuditDepth}},
		{name: "越上限收敛", in: AuditSettings{MaxBytesMB: 999, Depth: 10},
			want: AuditSettings{MaxBytesMB: MaxAuditMaxBytesMB, Depth: MaxAuditDepth}},
		{name: "界内取值原样保留", in: AuditSettings{MaxBytesMB: 8, Depth: 4},
			want: AuditSettings{MaxBytesMB: 8, Depth: 4}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &AppSettings{Audit: &c.in}
			s.EnsureAuditDefaults()
			if !reflect.DeepEqual(*s.Audit, c.want) {
				t.Errorf("clamp 结果不符, got %+v, want %+v", *s.Audit, c.want)
			}
		})
	}
}

// TestNormalizeAuditSettings 归一化入口契约：nil → 默认值；非 nil → 拷贝
// normalize 后返回且不改写入参（对齐 NormalizeIPBanSettings 先例）。
func TestNormalizeAuditSettings(t *testing.T) {
	if got := NormalizeAuditSettings(nil); got != DefaultAuditSettings() {
		t.Fatalf("nil 应返回默认值, got %+v", got)
	}
	in := &AuditSettings{MaxBytesMB: 99, Depth: 1}
	got := NormalizeAuditSettings(in)
	if got.MaxBytesMB != MaxAuditMaxBytesMB || got.Depth != MinAuditDepth {
		t.Fatalf("越界值应 clamp, got %+v", got)
	}
	// 入参不被修改（共享缓存指针防竞态纪律）
	if in.MaxBytesMB != 99 || in.Depth != 1 {
		t.Fatalf("入参不应被原地修改, got %+v", in)
	}
}

// TestAppSettings_EnsureAllDefaults 覆盖 Ensure 串联单点：零值 AppSettings 经
// EnsureAllDefaults 后全部设置段补全默认值（指针段非 nil + 终端外观标量字段
// 回默认），与逐段调用各 EnsureXxx 等效——Service 层 Load miss / Save 与
// App.GetSettings 降级分支三处消费点共用本链路，新增设置段在此处断言面加行。
func TestAppSettings_EnsureAllDefaults(t *testing.T) {
	s := &AppSettings{}
	s.EnsureAllDefaults()

	if s.WebServe == nil || !s.WebServe.Enabled || s.WebServe.BindAddress != DefaultWebServeBindAddress {
		t.Errorf("webServe 段应补默认开启 + 回环地址, got %+v", s.WebServe)
	}
	if s.TerminalFontSize != DefaultTerminalFontSize || s.TerminalScrollback != DefaultTerminalScrollback {
		t.Errorf("终端外观字段应补默认值, got 字号=%d 回滚=%d", s.TerminalFontSize, s.TerminalScrollback)
	}
	wantAssets := DefaultAssetThumbsSettings()
	if s.Assets == nil || *s.Assets != wantAssets {
		t.Errorf("素材缩略图段应补五项全默认, got %+v", s.Assets)
	}
	if s.IPBan == nil {
		t.Fatal("IP 封禁段应补全默认, got nil")
	}
	if !reflect.DeepEqual(*s.IPBan, DefaultIPBanSettings()) {
		t.Errorf("IP 封禁段应补参数全默认, got %+v", *s.IPBan)
	}
	if s.IPBan.Whitelist == nil || len(s.IPBan.Whitelist) != 0 {
		t.Errorf("白名单默认应为非 nil 空切片, got %#v", s.IPBan.Whitelist)
	}
	wantAudit := DefaultAuditSettings()
	if s.Audit == nil || *s.Audit != wantAudit {
		t.Errorf("审计留存段应补两项全默认, got %+v", s.Audit)
	}
}

// TestAppSettings_PrivateCopy 覆盖指针段私有拷贝单点：返回值四指针段与原对象
// 段指针不同址且段内容相等、改拷贝段不影响原对象；nil 段保持 nil（补段是
// EnsureAllDefaults 职责，拷贝不凭空造段）。
func TestAppSettings_PrivateCopy(t *testing.T) {
	orig := &AppSettings{
		TerminalFontSize: 16,
		WebServe:         &WebServeSettings{Enabled: true, BindAddress: "127.0.0.1:36115", TLS: true, TLSCertPath: "a.pem", TLSKeyPath: "b.pem"},
		Assets:           &AssetThumbsSettings{Auto: true, SmallWidth: 480, HDWidth: 2560, SmallQuotaMB: 2048, HDQuotaMB: 4096},
		IPBan:            &IPBanSettings{Threshold: 20, Whitelist: []string{"192.168.1.0/24"}},
		Audit:            &AuditSettings{MaxBytesMB: 8, Depth: 4},
	}
	copied := orig.PrivateCopy()

	// 四指针段与原对象不同址（浅拷贝形态下段指针值相等，此处必须不等）
	if copied.WebServe == orig.WebServe || copied.Assets == orig.Assets ||
		copied.IPBan == orig.IPBan || copied.Audit == orig.Audit {
		t.Fatal("四指针段应与原对象段指针不同址（私有拷贝）")
	}
	// 段内容与原值相等
	if !reflect.DeepEqual(copied.WebServe, orig.WebServe) ||
		!reflect.DeepEqual(copied.Assets, orig.Assets) ||
		!reflect.DeepEqual(copied.IPBan, orig.IPBan) ||
		!reflect.DeepEqual(copied.Audit, orig.Audit) {
		t.Fatal("拷贝段内容应与原值相等")
	}
	// 非指针标量字段随值拷贝携带
	if copied.TerminalFontSize != orig.TerminalFontSize {
		t.Errorf("标量字段应随值拷贝携带, got %d want %d", copied.TerminalFontSize, orig.TerminalFontSize)
	}

	// 改拷贝段不影响原对象（写入隔离）
	copied.WebServe.BindAddress = "0.0.0.0:9999"
	copied.Assets.SmallWidth = 999
	copied.IPBan.Threshold = 1
	copied.Audit.Depth = 2
	if orig.WebServe.BindAddress != "127.0.0.1:36115" ||
		orig.Assets.SmallWidth != 480 ||
		orig.IPBan.Threshold != 20 ||
		orig.Audit.Depth != 4 {
		t.Fatalf("改拷贝段不应影响原对象, got %+v", orig)
	}

	// nil 段保持 nil（不凭空补段）
	empty := (&AppSettings{}).PrivateCopy()
	if empty.WebServe != nil || empty.Assets != nil || empty.IPBan != nil || empty.Audit != nil {
		t.Errorf("nil 段应保持 nil, got %+v", empty)
	}
}
