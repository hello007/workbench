package main

import (
	"path/filepath"
	"testing"

	"workbench/util/testutil"
)

// TestCheckFileExists 行归属入口的存在性预判契约：
// 文件/目录存在返回 true，不存在与空路径返回 false（不存在时前端降级转文件历史）。
func TestCheckFileExists(t *testing.T) {
	app := &App{}
	tmp := t.TempDir()

	existingFile := filepath.Join(tmp, "main.go")
	testutil.WriteFile(t, existingFile, "package main\n")
	existingDir := filepath.Join(tmp, "sub")
	testutil.WriteFile(t, filepath.Join(existingDir, "placeholder.txt"), "x")

	if !app.CheckFileExists(existingFile) {
		t.Errorf("存在的文件应返回 true: %s", existingFile)
	}
	if !app.CheckFileExists(existingDir) {
		t.Errorf("存在的目录应返回 true: %s", existingDir)
	}
	if app.CheckFileExists(filepath.Join(tmp, "missing.txt")) {
		t.Errorf("不存在的路径应返回 false")
	}
	if app.CheckFileExists("") {
		t.Errorf("空路径应返回 false")
	}
	if app.CheckFileExists("   ") {
		t.Errorf("空白路径应返回 false")
	}
}
