package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useTempDiffRoot 将 diff 临时根指向本用例专属目录，测试结束恢复，
// 避免读写真实共享 %TEMP%\workbench-diff（误删运行中应用的在用文件）。
func useTempDiffRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := diffTempRootOverride
	diffTempRootOverride = dir
	t.Cleanup(func() { diffTempRootOverride = prev })
	return dir
}

func TestRenderDiffArgsTemplate_Basic(t *testing.T) {
	args, err := RenderDiffArgsTemplate("{left} {right}", `C:\tmp\a.go`, `C:\tmp\b.go`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(args) != 2 || args[0] != `C:\tmp\a.go` || args[1] != `C:\tmp\b.go` {
		t.Errorf("got %v", args)
	}
}

func TestRenderDiffArgsTemplate_WithFlags(t *testing.T) {
	args, err := RenderDiffArgsTemplate("--diff --wait {left} {right}", "/tmp/l", "/tmp/r")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{"--diff", "--wait", "/tmp/l", "/tmp/r"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], want[i])
		}
	}
}

func TestRenderDiffArgsTemplate_PathWithSpaces(t *testing.T) {
	// 占位符替换在分词后进行，含空格路径整体作为单个参数，无需引号
	args, err := RenderDiffArgsTemplate("{left} {right}", `C:\Program Files\left a.go`, "/tmp/r")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if args[0] != `C:\Program Files\left a.go` {
		t.Errorf("含空格路径应整体作为单参数, got %q", args[0])
	}
}

func TestRenderDiffArgsTemplate_Empty(t *testing.T) {
	if _, err := RenderDiffArgsTemplate("  ", "l", "r"); err == nil {
		t.Error("空模板应返回错误")
	}
}

func TestRenderDiffArgsTemplate_MissingPlaceholder(t *testing.T) {
	cases := []string{"{left}", "{right}", "--diff"}
	for _, tpl := range cases {
		if _, err := RenderDiffArgsTemplate(tpl, "l", "r"); err == nil {
			t.Errorf("模板 %q 缺占位符应返回错误", tpl)
		}
	}
}

func TestDiffTempDir_WriteAndRead(t *testing.T) {
	root := useTempDiffRoot(t)
	dir, err := CreateDiffTempDir()
	if err != nil {
		t.Fatalf("CreateDiffTempDir: %v", err)
	}
	if !strings.HasPrefix(dir, root) {
		t.Errorf("临时目录应位于注入根下, got %q", dir)
	}

	leftPath, err := WriteDiffTempFile(dir, "left", "main.go", "left content")
	if err != nil {
		t.Fatalf("WriteDiffTempFile left: %v", err)
	}
	rightPath, err := WriteDiffTempFile(dir, "right", "main.go", "right content")
	if err != nil {
		t.Fatalf("WriteDiffTempFile right: %v", err)
	}

	if filepath.Base(leftPath) != "main.go" || filepath.Base(rightPath) != "main.go" {
		t.Errorf("临时文件应保留原文件名, left=%q right=%q", leftPath, rightPath)
	}
	if filepath.Base(filepath.Dir(leftPath)) != "left" || filepath.Base(filepath.Dir(rightPath)) != "right" {
		t.Errorf("两侧应分别位于 left/right 子目录, left=%q right=%q", leftPath, rightPath)
	}

	data, err := os.ReadFile(leftPath)
	if err != nil || string(data) != "left content" {
		t.Errorf("left 内容不符: %v %q", err, data)
	}
	data, err = os.ReadFile(rightPath)
	if err != nil || string(data) != "right content" {
		t.Errorf("right 内容不符: %v %q", err, data)
	}
}

func TestCreateDiffTempDir_Unique(t *testing.T) {
	useTempDiffRoot(t)
	dirA, err := CreateDiffTempDir()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	dirB, err := CreateDiffTempDir()
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if dirA == dirB {
		t.Errorf("连续创建应得到不同目录, got %q", dirA)
	}
}

func TestWriteDiffTempFile_InvalidInput(t *testing.T) {
	if _, err := WriteDiffTempFile("", "left", "a.go", "x"); err == nil {
		t.Error("空临时目录应返回错误")
	}
	if _, err := WriteDiffTempFile("whatever", "left", "", "x"); err == nil {
		t.Error("空文件名应返回错误")
	}
}

func TestCleanupDiffTempDir(t *testing.T) {
	root := useTempDiffRoot(t)
	if _, err := CreateDiffTempDir(); err != nil {
		t.Fatalf("CreateDiffTempDir: %v", err)
	}
	CleanupDiffTempDir()
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("清理后注入根应不存在, err=%v", err)
	}
}
