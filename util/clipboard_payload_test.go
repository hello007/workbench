package util

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildClipboardFilesPayload_Copy 复制动作：首行 copy + file:// URI 列表。
func TestBuildClipboardFilesPayload_Copy(t *testing.T) {
	got := buildClipboardFilesPayload([]string{"/tmp/a.txt", "/tmp/b dir/c.txt"}, false)
	lines := strings.Split(got, "\n")
	if lines[0] != "copy" {
		t.Errorf("首行应为 copy, 实际=%q", lines[0])
	}
	if len(lines) != 3 {
		t.Fatalf("期望 3 行（动作 + 2 个 URI）, 实际=%d", len(lines))
	}
	if lines[1] != "file:///tmp/a.txt" {
		t.Errorf("URI 应为 file:///tmp/a.txt, 实际=%q", lines[1])
	}
	// 空格须百分号编码，URI 中不得出现原始空格
	if lines[2] != "file:///tmp/b%20dir/c.txt" {
		t.Errorf("空格应编码为 %%20, 实际=%q", lines[2])
	}
}

// TestBuildClipboardFilesPayload_Cut 剪切动作：首行为 cut。
func TestBuildClipboardFilesPayload_Cut(t *testing.T) {
	got := buildClipboardFilesPayload([]string{"/tmp/a.txt"}, true)
	if !strings.HasPrefix(got, "cut\n") {
		t.Errorf("剪切载荷应以 cut 行开头, 实际=%q", got)
	}
}

// TestBuildClipboardFilesPayload_Unicode 中文路径须安全编码且可无损还原（round-trip）。
// 期望路径经 FromSlash 归一（fileURIToPath 返回平台原生分隔符路径）。
func TestBuildClipboardFilesPayload_Unicode(t *testing.T) {
	path := "/tmp/项目/笔记.md"
	got := buildClipboardFilesPayload([]string{path}, false)
	if strings.Contains(got, "项目") || strings.Contains(got, "笔记") {
		t.Errorf("中文应被百分号编码, 实际=%q", got)
	}
	paths, isCut, ok := parseClipboardFilesPayload(got)
	if !ok || isCut || len(paths) != 1 || paths[0] != filepath.FromSlash(path) {
		t.Errorf("round-trip 应无损还原: paths=%v, isCut=%v, ok=%v", paths, isCut, ok)
	}
}

// TestParseClipboardFilesPayload_Copy 正常复制载荷解析。
func TestParseClipboardFilesPayload_Copy(t *testing.T) {
	paths, isCut, ok := parseClipboardFilesPayload("copy\nfile:///tmp/a.txt\nfile:///tmp/b%20.txt")
	if !ok {
		t.Fatal("有效载荷应解析成功")
	}
	if isCut {
		t.Error("copy 动作 isCut 应为 false")
	}
	if len(paths) != 2 || paths[0] != filepath.FromSlash("/tmp/a.txt") || paths[1] != filepath.FromSlash("/tmp/b .txt") {
		t.Errorf("路径解析不符: %v", paths)
	}
}

// TestParseClipboardFilesPayload_Cut 剪切载荷解析（cut 动作语义）。
func TestParseClipboardFilesPayload_Cut(t *testing.T) {
	paths, isCut, ok := parseClipboardFilesPayload("cut\nfile:///tmp/a.txt")
	if !ok {
		t.Fatal("有效载荷应解析成功")
	}
	if !isCut {
		t.Error("cut 动作 isCut 应为 true")
	}
	if len(paths) != 1 || paths[0] != filepath.FromSlash("/tmp/a.txt") {
		t.Errorf("路径解析不符: %v", paths)
	}
}

// TestParseClipboardFilesPayload_Invalid 无动作行/空内容/无文件行均按「无文件」处理。
func TestParseClipboardFilesPayload_Invalid(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"空内容", ""},
		{"仅空白", "  \n "},
		{"无动作行", "file:///tmp/a.txt"},
		{"未知动作", "move\nfile:///tmp/a.txt"},
		{"动作行后无文件", "copy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			paths, isCut, ok := parseClipboardFilesPayload(c.in)
			if ok || paths != nil || isCut {
				t.Errorf("无效载荷应返回 (nil, false, false), 实际=(%v, %v, %v)", paths, isCut, ok)
			}
		})
	}
}

// TestParseClipboardFilesPayload_PlainPathFallback 兼容未加 scheme 的纯 slash 路径行。
func TestParseClipboardFilesPayload_PlainPathFallback(t *testing.T) {
	paths, isCut, ok := parseClipboardFilesPayload("copy\n/tmp/plain.txt")
	if !ok {
		t.Fatal("纯路径载荷应解析成功")
	}
	if isCut {
		t.Error("copy 动作 isCut 应为 false")
	}
	if len(paths) != 1 || paths[0] != filepath.FromSlash("/tmp/plain.txt") {
		t.Errorf("路径解析不符: %v", paths)
	}
}

// TestParseClipboardFilesPayload_RemoteHostURI 远程主机 URI 无法映射本地路径，跳过该行。
func TestParseClipboardFilesPayload_RemoteHostURI(t *testing.T) {
	paths, _, ok := parseClipboardFilesPayload("copy\nfile://other-host/tmp/a.txt")
	if ok {
		t.Errorf("仅含远程主机 URI 的载荷应按无文件处理, 实际=%v", paths)
	}
}

// TestPathToFileURI_FileURIToPath_RoundTrip Windows 风格反斜杠路径同样可 round-trip。
func TestPathToFileURI_FileURIToPath_RoundTrip(t *testing.T) {
	path := filepath.Join(string(filepath.Separator), "tmp", "a b", "文.md")
	uri := pathToFileURI(path)
	if strings.Contains(uri, " ") {
		t.Errorf("URI 不应含原始空格: %q", uri)
	}
	got := fileURIToPath(uri)
	if got != path {
		t.Errorf("round-trip 应还原原路径: 期望=%q, 实际=%q", path, got)
	}
}

// TestFileURIToPath_LocalhostHost file://localhost/ 形式等价 file:///。
func TestFileURIToPath_LocalhostHost(t *testing.T) {
	if got := fileURIToPath("file://localhost/tmp/a.txt"); got != filepath.FromSlash("/tmp/a.txt") {
		t.Errorf("localhost 主机应解析为本地路径, 实际=%q", got)
	}
}
