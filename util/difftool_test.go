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

// TestCreateDiffTempDir_MissingRootRecreated 根目录被清理（启动期 CleanupDiffTempDir
// 删除整个根）后创建应自动重建父目录，而非报「找不到文件」。
func TestCreateDiffTempDir_MissingRootRecreated(t *testing.T) {
	root := useTempDiffRoot(t)
	CleanupDiffTempDir() // 确保根目录不存在
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("前置条件失败: 根目录应不存在, err=%v", err)
	}

	dir, err := CreateDiffTempDir()
	if err != nil {
		t.Fatalf("根目录缺失时 CreateDiffTempDir 应自动重建: %v", err)
	}
	if !strings.HasPrefix(dir, root) {
		t.Errorf("临时目录应位于重建的根下, got %q", dir)
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

// TestRenderMergeArgsTemplate 三向合并模板渲染：四占位符替换、含空格路径整体传参、
// 占位符缺失/空模板报错。
func TestRenderMergeArgsTemplate(t *testing.T) {
	// 四占位符全替换（含工具选项与输出参数形式）
	args, err := RenderMergeArgsTemplate("-e -o {merged} {base} {local} {remote}", "C:/b f.txt", "C:/l f.txt", "C:/r f.txt", "C:/m f.txt")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := []string{"-e", "-o", "C:/m f.txt", "C:/b f.txt", "C:/l f.txt", "C:/r f.txt"}
	if len(args) != len(want) {
		t.Fatalf("args 数不符: %v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}

	// 空模板
	if _, err := RenderMergeArgsTemplate("", "b", "l", "r", "m"); err == nil {
		t.Error("空模板应报错")
	}
	// 缺任一占位符均报错；含全部四占位符（重复出现亦可）不报错
	for _, tpl := range []string{"{base} {local} {remote}", "{local} {remote} {merged}", "{base} {base} {base}"} {
		if _, err := RenderMergeArgsTemplate(tpl, "b", "l", "r", "m"); err == nil {
			t.Errorf("模板 %q 缺占位符应报错", tpl)
		}
	}
	if _, err := RenderMergeArgsTemplate("{base} {local} {remote} {merged} {merged}", "b", "l", "r", "m"); err != nil {
		t.Errorf("四占位符齐备应渲染成功: %v", err)
	}
}
