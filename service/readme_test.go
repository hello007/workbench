package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeReadme 在仓库目录下写入 README 文件（自动创建父目录）。
func writeReadme(t *testing.T, repoPath, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestParseReadmeSummary_PlainMarkdown 标准 Markdown README：去标记取首段，截断 200 字。
func TestParseReadmeSummary_PlainMarkdown(t *testing.T) {
	repo := t.TempDir()
	// 首段为含加粗的说明文字，第二段不应出现
	writeReadme(t, repo, "README.md", "这是**首段**说明文字，介绍项目用途。\n\n第二段不应出现。")

	got := ParseReadmeSummary(repo)
	if got == "" {
		t.Fatal("expected non-empty summary")
	}
	if !strings.Contains(got, "首段") {
		t.Errorf("expected summary to contain bold-stripped text '首段', got: %q", got)
	}
	if !strings.Contains(got, "说明文字") {
		t.Errorf("expected summary to contain description text, got: %q", got)
	}
	if strings.Contains(got, "第二段") {
		t.Errorf("second paragraph should not appear, got: %q", got)
	}
	if strings.Contains(got, "**") {
		t.Errorf("bold markers should be stripped, got: %q", got)
	}
}

// TestParseReadmeSummary_LinkStripped 链接 [text](url) 应保留 text，移除 url。
func TestParseReadmeSummary_LinkStripped(t *testing.T) {
	repo := t.TempDir()
	writeReadme(t, repo, "README.md", "see [文档](https://example.com/doc) for details")

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "文档") {
		t.Errorf("expected link text '文档' preserved, got: %q", got)
	}
	if strings.Contains(got, "https://example.com") {
		t.Errorf("link url should be stripped, got: %q", got)
	}
}

// TestParseReadmeSummary_NoReadme 无 README 文件应返回空串。
func TestParseReadmeSummary_NoReadme(t *testing.T) {
	repo := t.TempDir()
	got := ParseReadmeSummary(repo)
	if got != "" {
		t.Errorf("expected empty summary when no README, got: %q", got)
	}
}

// TestParseReadmeSummary_EmptyFile 空 README 文件应返回空串。
func TestParseReadmeSummary_EmptyFile(t *testing.T) {
	repo := t.TempDir()
	writeReadme(t, repo, "README.md", "")
	got := ParseReadmeSummary(repo)
	if got != "" {
		t.Errorf("expected empty summary for empty README, got: %q", got)
	}
}

// TestParseReadmeSummary_PriorityMdOverReadme README.md 优先于无扩展名 README。
func TestParseReadmeSummary_PriorityMdOverReadme(t *testing.T) {
	repo := t.TempDir()
	writeReadme(t, repo, "README", "无扩展名内容")
	writeReadme(t, repo, "README.md", "Markdown内容")

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "Markdown内容") {
		t.Errorf("expected README.md to take priority, got: %q", got)
	}
	if strings.Contains(got, "无扩展名内容") {
		t.Errorf("README (no ext) should not be used when README.md exists, got: %q", got)
	}
}

// TestParseReadmeSummary_CaseInsensitive 大小写不敏感匹配（readme.md / README.MD 等）。
func TestParseReadmeSummary_CaseInsensitive(t *testing.T) {
	repo := t.TempDir()
	writeReadme(t, repo, "readme.md", "小写文件名内容")

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "小写文件名内容") {
		t.Errorf("expected case-insensitive match for readme.md, got: %q", got)
	}
}

// TestParseReadmeSummary_GBKEncoding 非 UTF-8 的 GBK README 应降级解码为 UTF-8 文本。
func TestParseReadmeSummary_GBKEncoding(t *testing.T) {
	repo := t.TempDir()
	// "中文README" 的 GBK 编码字节
	// 中=D6D0 文=CEC4 R=52 E=45 A=41 D=44 M=4D E=45
	gbkBytes := []byte{0xD6, 0xD0, 0xCE, 0xC4, 0x52, 0x45, 0x41, 0x44, 0x4D, 0x45}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), gbkBytes, 0644); err != nil {
		t.Fatalf("write GBK readme: %v", err)
	}

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "中文README") {
		t.Errorf("expected GBK README decoded to '中文README', got: %q", got)
	}
}

// TestParseReadmeSummary_BinaryFile 二进制 README（含 NUL）应返回空串。
func TestParseReadmeSummary_BinaryFile(t *testing.T) {
	repo := t.TempDir()
	// 含 NUL 字节 -> DetectTextEncoding 判为二进制
	bin := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x00, 0x77, 0x6F, 0x72, 0x6C, 0x64}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), bin, 0644); err != nil {
		t.Fatalf("write binary readme: %v", err)
	}
	got := ParseReadmeSummary(repo)
	if got != "" {
		t.Errorf("expected empty summary for binary README, got: %q", got)
	}
}

// TestParseReadmeSummary_Truncate200 超长首段应截断到 200 rune 并追加省略号。
func TestParseReadmeSummary_Truncate200(t *testing.T) {
	repo := t.TempDir()
	// 构造超长单行（300 个中文字符）
	longRunes := make([]rune, 300)
	for i := range longRunes {
		longRunes[i] = '字'
	}
	writeReadme(t, repo, "README.md", string(longRunes))

	got := ParseReadmeSummary(repo)
	// 200 字 + 3 字省略号（"..."）
	runeCount := 0
	for range got {
		runeCount++
	}
	if runeCount != 200+3 {
		t.Errorf("expected 200+3 runes after truncate, got %d", runeCount)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected truncated summary to end with '...', got: %q", got)
	}
}

// TestParseReadmeSummary_ImageStripped 图片语法 ![alt](url) 应被移除。
func TestParseReadmeSummary_ImageStripped(t *testing.T) {
	repo := t.TempDir()
	writeReadme(t, repo, "README.md", "![logo](https://example.com/logo.png)\n\n这是说明文字")

	got := ParseReadmeSummary(repo)
	if strings.Contains(got, "logo.png") {
		t.Errorf("image url should be stripped, got: %q", got)
	}
	if !strings.Contains(got, "说明文字") {
		t.Errorf("expected description text preserved, got: %q", got)
	}
}

// TestParseReadmeSummary_LeadingBlankLines 前导空行应被跳过，首段为首个非空段。
func TestParseReadmeSummary_LeadingBlankLines(t *testing.T) {
	repo := t.TempDir()
	writeReadme(t, repo, "README.md", "\n\n\n# 标题\n\n实际首段内容")

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "标题") {
		t.Errorf("expected first non-empty paragraph (header), got: %q", got)
	}
	if !strings.Contains(got, "实际首段内容") {
		// 标题行后无空行 -> 标题与首段同段；有空行则标题为独立首段
		t.Logf("summary: %q", got)
	}
}
