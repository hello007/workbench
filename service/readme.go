package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"workbench/util"
)

// readmeMaxSummaryLen README 摘要最大字符数（按 rune 计），超出截断。
const readmeMaxSummaryLen = 200

// readmeReadLimit README 读取字节上限，避免读取超大文件。摘要只需首段，4KB 足够。
const readmeReadLimit = 4 * 1024

// readmeCandidates README 文件名优先级（大小写不敏感匹配，按数组顺序优先）。
// 顺序：README.md（任意大小写）> README（无扩展名）> README.rst。
var readmeCandidates = []string{"README.md", "README.MD", "readme.md", "README", "README.rst"}

// 预编译 Markdown 标记正则，避免每次调用重新编译。
// 注意：Go regexp 使用 RE2 语法，不支持反向引用，故 ** 与 __ 分开匹配。
var (
	// 图片 ![alt](url) -> 移除
	reMarkdownImage = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	// 链接 [text](url) -> text
	reMarkdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	// 行内代码 `code` -> code
	reMarkdownCode = regexp.MustCompile("`([^`]*)`")
	// 加粗 **text** -> text
	reMarkdownBoldAsterisk = regexp.MustCompile(`\*\*([^*]*)\*\*`)
	// 加粗 __text__ -> text
	reMarkdownBoldUnderscore = regexp.MustCompile(`__([^_]*)__`)
	// 标题前导 #（1-6 级）
	reMarkdownHeader = regexp.MustCompile(`(?m)^#{1,6}\s*`)
	// 引用前导 >
	reMarkdownQuote = regexp.MustCompile(`(?m)^>\s?`)
	// 水平线 --- /*** /___
	reMarkdownHR = regexp.MustCompile(`(?m)^(-{3,}|\*{3,}|_{3,})\s*$`)
	// 无序列表前导 - * +
	reMarkdownUL = regexp.MustCompile(`(?m)^[-*+]\s+`)
	// 有序列表前导 1.
	reMarkdownOL = regexp.MustCompile(`(?m)^\d+\.\s+`)
	// 多余空白折叠
	reMultiSpace = regexp.MustCompile(`[ \t]+`)
)

// ParseReadmeSummary 解析仓库根目录下 README 文件的摘要文本。
// 流程：按优先级定位 README -> 读取字节（上限 4KB）-> DetectTextEncoding 转 UTF-8
// （非 UTF-8 按 GBK 降级）-> 去除 Markdown 标记取首段非空文本 -> 截断 200 字。
// 无 README / 空文件 / 二进制 / 编码不可识别均返回空串（前端显示"暂无 README"）。
func ParseReadmeSummary(repoPath string) string {
	readmePath := findReadme(repoPath)
	if readmePath == "" {
		return ""
	}

	data, err := util.ReadFileSafe(readmePath, readmeReadLimit)
	if err != nil {
		return ""
	}

	// 复用文件预览的编码检测：ok=false（二进制/不可识别）-> 空串
	_, content, ok := util.DetectTextEncoding(data)
	if !ok {
		return ""
	}

	return extractReadmeSummary(content)
}

// findReadme 在仓库根目录下按优先级查找 README 文件，返回首个命中的绝对路径（未命中返回空串）。
// 大小写不敏感匹配（兼容 Windows 不区分大小写文件系统与 Linux 区分大小写场景）。
func findReadme(repoPath string) string {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return ""
	}
	// 构建小写文件名 -> 实际文件名 映射（仅文件，跳过目录）
	lowerMap := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lowerMap[strings.ToLower(e.Name())] = e.Name()
	}
	// 按优先级匹配：多个候选映射到同一小写键时，数组顺序即优先级
	for _, candidate := range readmeCandidates {
		if actual, ok := lowerMap[strings.ToLower(candidate)]; ok {
			return filepath.Join(repoPath, actual)
		}
	}
	return ""
}

// extractReadmeSummary 从 README 文本中提取摘要：去 Markdown 标记 -> 取首段非空文本 -> 截断。
// 首段定义：跳过前导空行后，到首个空行（或全文结尾）之间的内容。
func extractReadmeSummary(content string) string {
	lines := strings.Split(content, "\n")
	var paragraph []string
	started := false
	for _, raw := range lines {
		cleaned := strings.TrimSpace(cleanMarkdownLine(raw))
		if cleaned == "" {
			// 首段已开始 -> 空行结束首段；未开始 -> 跳过前导空行
			if started {
				break
			}
			continue
		}
		started = true
		paragraph = append(paragraph, cleaned)
	}
	if len(paragraph) == 0 {
		return ""
	}
	summary := strings.Join(paragraph, " ")
	return truncateRunes(summary, readmeMaxSummaryLen)
}

// cleanMarkdownLine 去除单行的 Markdown 标记（标题/引用/列表前导、加粗/链接/代码/图片）。
// 返回清理后的纯文本行（未 trim，保留行内空白由调用方处理）。
func cleanMarkdownLine(line string) string {
	s := line
	s = reMarkdownImage.ReplaceAllString(s, "")
	s = reMarkdownLink.ReplaceAllString(s, "$1")
	s = reMarkdownCode.ReplaceAllString(s, "$1")
	s = reMarkdownBoldAsterisk.ReplaceAllString(s, "$1")
	s = reMarkdownBoldUnderscore.ReplaceAllString(s, "$1")
	s = reMarkdownHeader.ReplaceAllString(s, "")
	s = reMarkdownQuote.ReplaceAllString(s, "")
	s = reMarkdownHR.ReplaceAllString(s, "")
	s = reMarkdownUL.ReplaceAllString(s, "")
	s = reMarkdownOL.ReplaceAllString(s, "")
	s = reMultiSpace.ReplaceAllString(s, " ")
	return s
}

// truncateRunes 按 rune 截断字符串到 maxLen，超出则追加省略号。
func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen]) + "..."
}
