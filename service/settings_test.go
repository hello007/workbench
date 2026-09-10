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
