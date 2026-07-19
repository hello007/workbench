package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// writeReadme 在仓库目录下写入 README 文件（自动创建父目录）。
func writeReadme(t *testing.T, repoPath, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestParseReadmeSummary_PlainMarkdown 标准 Markdown README：去标记取首段，截断 200 字。
// 首段需 >= 30 字以避免触发智能续取（优化 4a：短首段会续取下一段），从而验证"长首段不续取"。
func TestParseReadmeSummary_PlainMarkdown(t *testing.T) {
	repo := t.TempDir()
	// 首段为含加粗的说明文字（>30 字不触发续取），第二段不应出现
	writeReadme(t, repo, "README.md", "这是**首段**说明文字，介绍项目用途，首段需足够长以避免触发智能续取逻辑。\n\n第二段不应出现。")

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

// TestParseReadmeSummary_SmartContinue_TitleHeader 标题型 README（如 asgf）：
// 首段仅 H1 标题（< 30 字）-> 智能续取描述段，遇下一个 H1 标题停止。
// 预期摘要包含标题与描述，不含后续 H1 标题段。
func TestParseReadmeSummary_SmartContinue_TitleHeader(t *testing.T) {
	repo := t.TempDir()
	writeReadme(t, repo, "README.md", "# asgf\n\nAgree Service Governance Framework\n\n# 开发规范\n\n开发规范内容不应出现。")

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "asgf") {
		t.Errorf("expected title 'asgf' in summary, got: %q", got)
	}
	if !strings.Contains(got, "Agree Service Governance Framework") {
		t.Errorf("expected continued description in summary, got: %q", got)
	}
	if strings.Contains(got, "开发规范") {
		t.Errorf("H1 header '开发规范' should stop continuation, got: %q", got)
	}
}

// TestParseReadmeSummary_SmartContinue_StopOnH2 续取遇 H2 标题（##）停止，
// H1/H2 均视为正文边界。
func TestParseReadmeSummary_SmartContinue_StopOnH2(t *testing.T) {
	repo := t.TempDir()
	writeReadme(t, repo, "README.md", "# 短\n\n续取段一内容。\n\n## 二级标题\n\n二级标题下内容不应出现。")

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "续取段一内容") {
		t.Errorf("expected continued paragraph, got: %q", got)
	}
	if strings.Contains(got, "二级标题") {
		t.Errorf("H2 header should stop continuation, got: %q", got)
	}
}

// TestParseReadmeSummary_SmartContinue_NotStopOnH3 续取遇 H3（###）不停止，
// 3-6 级标题视为正文继续累计。
func TestParseReadmeSummary_SmartContinue_NotStopOnH3(t *testing.T) {
	repo := t.TempDir()
	writeReadme(t, repo, "README.md", "# 短\n\n续取段一。\n\n### 三级标题不算边界\n\n继续内容二。")

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "三级标题不算边界") {
		t.Errorf("H3 should NOT stop continuation, got: %q", got)
	}
	if !strings.Contains(got, "继续内容二") {
		t.Errorf("paragraph after H3 should be included, got: %q", got)
	}
}

// TestParseReadmeSummary_SmartContinue_LongFirstNoContinue 首段 >= 30 字时不触发续取。
func TestParseReadmeSummary_SmartContinue_LongFirstNoContinue(t *testing.T) {
	repo := t.TempDir()
	// 首段 33 字（>= 30），不续取
	writeReadme(t, repo, "README.md", "这是一段足够长的首段描述文字用于验证不触发智能续取逻辑的测试用例。\n\n第二段不应出现。")

	got := ParseReadmeSummary(repo)
	if strings.Contains(got, "第二段") {
		t.Errorf("long first paragraph should not trigger continuation, got: %q", got)
	}
}

// TestParseReadmeSummary_SmartContinue_TargetRunes 续取累计到 100 字后停止，
// 使累计达 100 的段落保留，其后的段落不再续取。
func TestParseReadmeSummary_SmartContinue_TargetRunes(t *testing.T) {
	repo := t.TempDir()
	// 首段短（"短" 1 字 < 30），后续每段 50 字：
	//   短(1) + 空格 + p1(50) = 52; + 空格 + p2(50) = 103 >= 100 停止 -> p3 不出现
	short := "# 短\n\n"
	p1 := "段一" + strings.Repeat("内", 48) // 50 字符
	p2 := "段二" + strings.Repeat("内", 48)
	p3 := "段三" + strings.Repeat("内", 48)
	writeReadme(t, repo, "README.md", short+p1+"\n\n"+p2+"\n\n"+p3)

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "段一") {
		t.Errorf("expected p1 in summary, got: %q", got)
	}
	if !strings.Contains(got, "段二") {
		t.Errorf("expected p2 in summary (cumulative reached target), got: %q", got)
	}
	if strings.Contains(got, "段三") {
		t.Errorf("p3 should not appear (continuation stopped after reaching target runes), got: %q", got)
	}
}

// TestParseReadmeSummary_SmartContinue_GBKMultiPara GBK 编码的多段落 README：
// 编码降级为 UTF-8 后智能续取仍生效。
func TestParseReadmeSummary_SmartContinue_GBKMultiPara(t *testing.T) {
	repo := t.TempDir()
	// 构造 UTF-8 原文："# 短\n\n描述段内容续取。\n\n# 不应出现"
	// 转 GBK 字节后写入，验证降级解码 + 智能续取
	utf8Text := "# 短\n\n描述段内容续取。\n\n# 不应出现"
	encoder := simplifiedchinese.GBK.NewEncoder()
	gbkBytes, err := encoder.Bytes([]byte(utf8Text))
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), gbkBytes, 0644); err != nil {
		t.Fatalf("write GBK readme: %v", err)
	}

	got := ParseReadmeSummary(repo)
	if !strings.Contains(got, "短") {
		t.Errorf("expected title '短' decoded from GBK, got: %q", got)
	}
	if !strings.Contains(got, "描述段内容续取") {
		t.Errorf("expected continued description decoded from GBK, got: %q", got)
	}
	if strings.Contains(got, "不应出现") {
		t.Errorf("H1 header should stop continuation after GBK decode, got: %q", got)
	}
}

// TestReadFullReadme_FullContent ReadFullReadme 返回完整 README 文本（不截断摘要）。
func TestReadFullReadme_FullContent(t *testing.T) {
	repo := t.TempDir()
	full := "# 项目名\n\n第一段描述。\n\n第二段描述。\n\n```go\nfmt.Println(\"hi\")\n```"
	writeReadme(t, repo, "README.md", full)

	got := ReadFullReadme(repo)
	if got != full {
		t.Errorf("expected full README content, got: %q", got)
	}
}

// TestReadFullReadme_NotDir 路径非目录（指向文件）应返回空串，防越界。
func TestReadFullReadme_NotDir(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "notadir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	got := ReadFullReadme(filePath)
	if got != "" {
		t.Errorf("expected empty for non-dir path, got: %q", got)
	}
}

// TestReadFullReadme_NoReadme 无 README 文件应返回空串。
func TestReadFullReadme_NoReadme(t *testing.T) {
	repo := t.TempDir()
	got := ReadFullReadme(repo)
	if got != "" {
		t.Errorf("expected empty when no README, got: %q", got)
	}
}

// TestReadFullReadme_GBK GBK 编码 README 应降级解码为完整 UTF-8 文本。
func TestReadFullReadme_GBK(t *testing.T) {
	repo := t.TempDir()
	utf8Text := "# 中文项目\n\n完整描述内容。"
	encoder := simplifiedchinese.GBK.NewEncoder()
	gbkBytes, err := encoder.Bytes([]byte(utf8Text))
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), gbkBytes, 0644); err != nil {
		t.Fatalf("write GBK readme: %v", err)
	}
	got := ReadFullReadme(repo)
	if got != utf8Text {
		t.Errorf("expected full GBK-decoded content, got: %q", got)
	}
}
