package util

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsPreviewable_SupportedExt 白名单内扩展名返回 true（含大小写）。
func TestIsPreviewable_SupportedExt(t *testing.T) {
	for _, name := range []string{"a.txt", "b.MD", "c.json", "d.vue", "e.go", "f.gitignore"} {
		if !IsPreviewable(name) {
			t.Errorf("IsPreviewable(%q) 期望 true", name)
		}
	}
}

// TestIsPreviewable_UnsupportedExt 白名单外扩展名返回 false。
func TestIsPreviewable_UnsupportedExt(t *testing.T) {
	for _, name := range []string{"a.pdf", "b.exe", "c.zip", "noext"} {
		if IsPreviewable(name) {
			t.Errorf("IsPreviewable(%q) 期望 false", name)
		}
	}
}

// TestFormatFileSize_Units 各档单位格式化正确。
func TestFormatFileSize_Units(t *testing.T) {
	cases := []struct {
		size int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, c := range cases {
		if got := FormatFileSize(c.size); got != c.want {
			t.Errorf("FormatFileSize(%d): got %q, want %q", c.size, got, c.want)
		}
	}
}

// TestReadFileSafe_Success 正常读取返回内容。
func TestReadFileSafe_Success(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(p, []byte("hello"), 0o644)
	data, err := ReadFileSafe(p, 1024)
	if err != nil {
		t.Fatalf("ReadFileSafe: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("内容不符: %q", data)
	}
}

// TestReadFileSafe_TooLarge 超限返回错误。
func TestReadFileSafe_TooLarge(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.txt")
	os.WriteFile(p, []byte("1234567890"), 0o644)
	_, err := ReadFileSafe(p, 5)
	if err == nil {
		t.Error("超限应返回错误")
	}
}

// TestReadFileSafe_NotExists 文件不存在返回错误。
func TestReadFileSafe_NotExists(t *testing.T) {
	_, err := ReadFileSafe(filepath.Join(t.TempDir(), "missing.txt"), 1024)
	if err == nil {
		t.Error("不存在文件应返回错误")
	}
}

// TestCreateDirectory_New 创建新目录（含嵌套）。
func TestCreateDirectory_New(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "b", "c")
	if err := CreateDirectory(p); err != nil {
		t.Fatalf("CreateDirectory: %v", err)
	}
	if !FileExists(p) {
		t.Error("目录应已创建")
	}
}

// TestCreateFile_WithContent 带内容创建文件并自动建父目录。
func TestCreateFile_WithContent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "f.txt")
	if err := CreateFile(p, "内容"); err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "内容" {
		t.Errorf("内容不符: %q", data)
	}
}

// TestCreateFile_EmptyContent 空内容创建空文件。
func TestCreateFile_EmptyContent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.txt")
	if err := CreateFile(p, ""); err != nil {
		t.Fatalf("CreateFile empty: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil || info.Size() != 0 {
		t.Errorf("空文件应为 0 字节, got %v", info)
	}
}

// TestRenamePath_Success 重命名文件成功。
func TestRenamePath_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.txt")
	dst := filepath.Join(dir, "new.txt")
	os.WriteFile(src, []byte("x"), 0o644)

	if err := RenamePath(src, dst); err != nil {
		t.Fatalf("RenamePath: %v", err)
	}
	if FileExists(src) || !FileExists(dst) {
		t.Error("重命名后源应消失、目标应存在")
	}
}

// TestRenamePath_SrcNotExists 源不存在返回错误。
func TestRenamePath_SrcNotExists(t *testing.T) {
	if err := RenamePath(filepath.Join(t.TempDir(), "no.txt"), filepath.Join(t.TempDir(), "x.txt")); err == nil {
		t.Error("源不存在应返回错误")
	}
}

// TestRemovePath_FileAndDir 文件与目录均可删除。
func TestRemovePath_FileAndDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	os.WriteFile(file, []byte("x"), 0o644)
	if err := RemovePath(file); err != nil {
		t.Fatalf("RemovePath file: %v", err)
	}
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0o755)
	if err := RemovePath(subDir); err != nil {
		t.Fatalf("RemovePath dir: %v", err)
	}
}

// TestRemovePath_NotExists 不存在路径不报错。
func TestRemovePath_NotExists(t *testing.T) {
	if err := RemovePath(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Errorf("不存在路径应不报错, got %v", err)
	}
}

// TestCopyFile_Success 复制后内容与权限一致且目标可写。
func TestCopyFile_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("复制内容"), 0o644)

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "复制内容" {
		t.Errorf("复制内容不符: %q", data)
	}
	// 目标文件应可写（chmod 置 0222）
	info, _ := os.Stat(dst)
	if info.Mode().Perm()&0o200 == 0 {
		t.Error("目标文件应可写")
	}
}

// TestCopyFile_SrcNotExists 源不存在返回错误。
func TestCopyFile_SrcNotExists(t *testing.T) {
	if err := CopyFile(filepath.Join(t.TempDir(), "no.txt"), filepath.Join(t.TempDir(), "x.txt")); err == nil {
		t.Error("源不存在应返回错误")
	}
}

// TestCopyFile_OverwriteExisting 目标已存在时覆盖。
func TestCopyFile_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("新内容"), 0o644)
	os.WriteFile(dst, []byte("旧内容"), 0o644)

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile overwrite: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "新内容" {
		t.Errorf("覆盖后内容应为新内容, got %q", data)
	}
}

// TestCopyDir_Recursive 递归复制目录树，结构与内容一致。
func TestCopyDir_Recursive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0o644)

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	if !FileExists(filepath.Join(dst, "a.txt")) || !FileExists(filepath.Join(dst, "sub", "b.txt")) {
		t.Error("递归复制后子项应存在")
	}
}

// TestCopyDir_SrcNotExists 源目录不存在返回错误。
func TestCopyDir_SrcNotExists(t *testing.T) {
	if err := CopyDir(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "dst")); err == nil {
		t.Error("源目录不存在应返回错误")
	}
}
