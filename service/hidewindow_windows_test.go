//go:build windows

package service

import "testing"

// TestHideWindow 返回隐藏窗口的 SysProcAttr（Windows 专属行为：
// HideWindow + CREATE_NO_WINDOW，非 Windows 平台 hideWindow 恒为 nil 无此断言）。
func TestHideWindow(t *testing.T) {
	attr := hideWindow()
	if attr == nil {
		t.Fatal("hideWindow 应返回非 nil")
	}
	if !attr.HideWindow {
		t.Error("HideWindow 应为 true")
	}
	if attr.CreationFlags != 0x08000000 {
		t.Errorf("CreationFlags 期望 0x08000000, got %#x", attr.CreationFlags)
	}
}
