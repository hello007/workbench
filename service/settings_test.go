package service

import (
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// newSettingsSvc 创建指向临时配置文件的 SettingsService。
func newSettingsSvc(t *testing.T) *SettingsService {
	t.Helper()
	return NewSettingsService(filepath.Join(t.TempDir(), "settings.json"))
}

// TestSettingsLoad_NotExists 配置文件不存在时返回空设置且不报错。
func TestSettingsLoad_NotExists(t *testing.T) {
	svc := newSettingsSvc(t)
	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load 不存在文件不应报错: %v", err)
	}
	if got == nil {
		t.Fatal("Load 不应返回 nil")
	}
	if got.DefaultShell != "" {
		t.Errorf("空设置 DefaultShell 应为空串, got %q", got.DefaultShell)
	}
}

// TestSettingsLoad_Valid 合法配置文件正确反序列化。
func TestSettingsLoad_Valid(t *testing.T) {
	svc := newSettingsSvc(t)
	want := &model.AppSettings{
		DefaultShell:           "powershell",
		ShortcutCommandPalette: "Ctrl+P",
		SearchExcludeDirs:      []string{"node_modules", ".git"},
	}
	if err := svc.Save(want); err != nil {
		t.Fatalf("Save 前置: %v", err)
	}

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DefaultShell != "powershell" {
		t.Errorf("DefaultShell: got %q, want powershell", got.DefaultShell)
	}
	if got.ShortcutCommandPalette != "Ctrl+P" {
		t.Errorf("ShortcutCommandPalette: got %q", got.ShortcutCommandPalette)
	}
	if len(got.SearchExcludeDirs) != 2 {
		t.Errorf("SearchExcludeDirs len: got %d, want 2", len(got.SearchExcludeDirs))
	}
}

// TestSettingsLoad_InvalidJSON 损坏 JSON 时吞错返回空设置（不向上抛）。
func TestSettingsLoad_InvalidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(p, []byte("{not json"), 0o644)
	svc := NewSettingsService(p)

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("损坏 JSON 应吞错返回 nil 错误, got %v", err)
	}
	if got == nil || got.DefaultShell != "" {
		t.Errorf("损坏 JSON 应回退空设置, got %+v", got)
	}
}

// TestSettingsSave_RoundTrip 保存后新建服务重新加载，内容一致。
func TestSettingsSave_RoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	svc1 := NewSettingsService(p)
	want := &model.AppSettings{GitBashPath: "C:\\git\\bin\\bash.exe", WslDistro: "Ubuntu"}
	if err := svc1.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc2 := NewSettingsService(p)
	got, err := svc2.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if got.GitBashPath != "C:\\git\\bin\\bash.exe" || got.WslDistro != "Ubuntu" {
		t.Errorf("round-trip 不一致: %+v", got)
	}
}

// TestSettingsLoad_WebServeDefaults 配置文件不存在时 webServe 段补默认值：
// 默认开启（桌面同开 HTTP）+ 回环默认地址。
func TestSettingsLoad_WebServeDefaults(t *testing.T) {
	svc := newSettingsSvc(t)
	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.WebServe == nil {
		t.Fatal("webServe 段缺失时应补默认值而非 nil")
	}
	if !got.WebServe.Enabled {
		t.Error("webServe 段缺失（老配置升级）应默认开启")
	}
	if got.WebServe.BindAddress != model.DefaultWebServeBindAddress {
		t.Errorf("BindAddress 应为默认回环地址, got %q", got.WebServe.BindAddress)
	}
}

// TestSettingsLoad_WebServeExplicitDisabled 显式关闭（enabled:false）被尊重，
// 不被默认值覆盖；地址为空仅补默认地址。
func TestSettingsLoad_WebServeExplicitDisabled(t *testing.T) {
	svc := newSettingsSvc(t)
	if err := svc.Save(&model.AppSettings{
		WebServe: &model.WebServeSettings{Enabled: false},
	}); err != nil {
		t.Fatalf("Save 前置: %v", err)
	}

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.WebServe == nil {
		t.Fatal("webServe 段不应为 nil")
	}
	if got.WebServe.Enabled {
		t.Error("用户显式关闭应被尊重，不得回退为开启")
	}
	if got.WebServe.BindAddress != model.DefaultWebServeBindAddress {
		t.Errorf("地址为空应补默认回环地址, got %q", got.WebServe.BindAddress)
	}
}

// TestSettingsLoad_WebServeMissingSegmentOldConfig 老配置文件（无 webServe 段）
// 升级加载后补默认开启段；其余字段不受影响。
func TestSettingsLoad_WebServeMissingSegmentOldConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	// 手写不含 webServe 段的旧版本配置
	if err := os.WriteFile(p, []byte(`{"defaultShell":"cmd","themeMode":"dark"}`), 0o644); err != nil {
		t.Fatalf("准备旧配置: %v", err)
	}

	got, err := NewSettingsService(p).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DefaultShell != "cmd" || got.ThemeMode != "dark" {
		t.Errorf("既有字段应原样保留: %+v", got)
	}
	if got.WebServe == nil || !got.WebServe.Enabled {
		t.Error("无 webServe 段的老配置应默认开启浏览器访问")
	}
}

// TestSettingsLoad_InvalidJSON_WebServeDefaults 配置损坏降级时 webServe 段同样
// 补默认值（不阻塞启动，桌面同开语义保持）。
func TestSettingsLoad_InvalidJSON_WebServeDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("准备损坏配置: %v", err)
	}

	got, err := NewSettingsService(p).Load()
	if err != nil {
		t.Fatalf("损坏 JSON 应吞错, got %v", err)
	}
	if got.WebServe == nil || !got.WebServe.Enabled ||
		got.WebServe.BindAddress != model.DefaultWebServeBindAddress {
		t.Errorf("损坏降级应补 webServe 默认值, got %+v", got.WebServe)
	}
}

// TestSettingsSave_RoundTrip_WebServe webServe 段持久化 round-trip：
// 显式关闭 + 自定义地址写盘后重读一致。
func TestSettingsSave_RoundTrip_WebServe(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	svc1 := NewSettingsService(p)
	if err := svc1.Save(&model.AppSettings{
		WebServe: &model.WebServeSettings{Enabled: false, BindAddress: "192.168.1.5:36115"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := NewSettingsService(p).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.WebServe == nil || got.WebServe.Enabled ||
		got.WebServe.BindAddress != "192.168.1.5:36115" {
		t.Errorf("webServe 段 round-trip 不一致: %+v", got.WebServe)
	}
}

// TestSettingsLoad_TerminalDefaults 老配置（无终端外观字段）升级加载后
// 字号/回滚行数补默认值，字体保留空串语义（默认 Cascadia Code 栈）。
func TestSettingsLoad_TerminalDefaults(t *testing.T) {
	svc := newSettingsSvc(t)
	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TerminalFontSize != model.DefaultTerminalFontSize {
		t.Errorf("TerminalFontSize 缺省应补 %d, got %d", model.DefaultTerminalFontSize, got.TerminalFontSize)
	}
	if got.TerminalScrollback != model.DefaultTerminalScrollback {
		t.Errorf("TerminalScrollback 缺省应补 %d, got %d", model.DefaultTerminalScrollback, got.TerminalScrollback)
	}
	if got.TerminalFontFamily != "" {
		t.Errorf("TerminalFontFamily 缺省应保留空串, got %q", got.TerminalFontFamily)
	}
}

// TestSettingsLoad_TerminalClamp 手改配置文件写入越界值时 clamp 收敛到
// 合法区间（字号 10-24、回滚 1000-10000），界内用户取值原样保留。
func TestSettingsLoad_TerminalClamp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(`{"terminalFontSize":99,"terminalScrollback":1}`), 0o644); err != nil {
		t.Fatalf("准备越界配置: %v", err)
	}

	got, err := NewSettingsService(p).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TerminalFontSize != model.MaxTerminalFontSize {
		t.Errorf("字号越界应收敛到上限 %d, got %d", model.MaxTerminalFontSize, got.TerminalFontSize)
	}
	if got.TerminalScrollback != model.MinTerminalScrollback {
		t.Errorf("回滚越界应收敛到下限 %d, got %d", model.MinTerminalScrollback, got.TerminalScrollback)
	}

	// 界内取值原样保留
	if err := os.WriteFile(p, []byte(`{"terminalFontSize":18,"terminalScrollback":5000}`), 0o644); err != nil {
		t.Fatalf("准备界内配置: %v", err)
	}
	got, err = NewSettingsService(p).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TerminalFontSize != 18 || got.TerminalScrollback != 5000 {
		t.Errorf("界内取值应原样保留, got fontSize=%d scrollback=%d", got.TerminalFontSize, got.TerminalScrollback)
	}
}
