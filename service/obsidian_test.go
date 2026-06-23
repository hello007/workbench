package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveObsidianVault_Directory 文件夹→自身
func TestResolveObsidianVault_Directory(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveObsidianVault(dir)
	if err != nil {
		t.Fatalf("resolveObsidianVault(dir) 出错: %v", err)
	}
	if got != dir {
		t.Errorf("文件夹应返回自身: 期望 %q, 实际 %q", dir, got)
	}
}

// TestResolveObsidianVault_File 文件→父目录
func TestResolveObsidianVault_File(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	got, err := resolveObsidianVault(file)
	if err != nil {
		t.Fatalf("resolveObsidianVault(file) 出错: %v", err)
	}
	if got != dir {
		t.Errorf("文件应返回父目录: 期望 %q, 实际 %q", dir, got)
	}
}

// TestResolveObsidianVault_NotFound 不存在路径→错误
func TestResolveObsidianVault_NotFound(t *testing.T) {
	_, err := resolveObsidianVault(filepath.Join(os.TempDir(), "definitely-not-exist-12345"))
	if err == nil {
		t.Fatal("不存在路径应返回错误")
	}
}

// TestEncodeObsidianPath 空格/中文/反斜杠编码
func TestEncodeObsidianPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // 关键断言片段
	}{
		{"反斜杠转正斜杠后编码", `C:\Users\test`, "C%3A%2FUsers%2Ftest"},
		{"空格编码为%20", `C:\my notes`, "my%20notes"},
		{"中文安全编码", `C:\Users\张三\笔记`, "%E5%BC%A0%E4%B8%89"},
		{"不含加号", `C:\a b\c`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := encodeObsidianPath(c.in)
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Errorf("encodeObsidianPath(%q) = %q, 应包含 %q", c.in, got, c.want)
			}
			// 编码结果不应出现原始空格或反斜杠
			if strings.ContainsAny(got, " \\") {
				t.Errorf("编码结果 %q 不应含空格或反斜杠", got)
			}
			// 空格应编码为 %20 而非 +
			if strings.Contains(got, "+") {
				t.Errorf("空格应编码为 %%20 而非 +: %q", got)
			}
		})
	}
}

// TestOpenInObsidian_NotFoundPath 路径不存在→友好错误（不启动外部进程）
func TestOpenInObsidian_NotFoundPath(t *testing.T) {
	svc := NewFileOperationService()
	err := svc.OpenInObsidian(filepath.Join(os.TempDir(), "definitely-not-exist-67890"), "")
	if err == nil {
		t.Fatal("不存在路径应返回错误")
	}
}
