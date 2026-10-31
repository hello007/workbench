//go:build windows

package util

import "testing"

// TestRecycleFlags 守护回收站删除标志组合：允许撤销 + 永久删除警告，
// 且不得出现「不确认」类标志（否则回收站不可用时静默永久删除源文件）。
func TestRecycleFlags(t *testing.T) {
	flags := recycleFlags()

	if flags&fofAllowUndo == 0 {
		t.Fatal("缺少 FOF_ALLOWUNDO：优先移入回收站")
	}
	if flags&fofWantNukeWarning == 0 {
		t.Fatal("缺少 FOF_WANTNUKEWARNING：永久删除前须警告")
	}
	if flags&0x0010 != 0 { // FOF_NOCONFIRMATION
		t.Fatal("禁止 FOF_NOCONFIRMATION：会静默永久删除")
	}
	if flags&0x0004 != 0 { // FOF_SILENT
		t.Fatal("禁止 FOF_SILENT：失败/退化不得静默")
	}
}
