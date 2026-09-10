//go:build windows

package service

import (
	"os"
	"path/filepath"
	"testing"
)

// TestObsidianConfigPath_WithAppData APPDATA 设置时返回 obsidian.json 路径。
func TestObsidianConfigPath_WithAppData(t *testing.T) {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		t.Skip("APPDATA 未设置，跳过")
	}
	got := obsidianConfigPath()
	want := filepath.Join(appdata, "obsidian", "obsidian.json")
	if got != want {
		t.Errorf("obsidianConfigPath: got %s, want %s", got, want)
	}
}

// TestObsidianConfigPath_NoAppData APPDATA 为空时返回空串。
func TestObsidianConfigPath_NoAppData(t *testing.T) {
	t.Setenv("APPDATA", "")
	if got := obsidianConfigPath(); got != "" {
		t.Errorf("APPDATA 空时应返回空串, got %s", got)
	}
}

// TestIsObsidianProtocolRegistered_NoPanic 调用不 panic，返回值依赖系统是否安装 Obsidian。
func TestIsObsidianProtocolRegistered_NoPanic(t *testing.T) {
	_ = isObsidianProtocolRegistered()
}
