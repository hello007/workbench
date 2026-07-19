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

// readmeReadLimit README 摘要读取字节上限，避免读取超大文件。摘要只需首段，4KB 足够。
const readmeReadLimit = 4 * 1024

// readmeFullReadLimit 完整 README 读取字节上限（1MB），供二级弹窗渲染完整内容。
// 超出按 ReadFileSafe 失败处理（返回空串），避免超大文件拖垮前端。
const readmeFullReadLimit = 1024 * 1024

// readmeSummaryMinContinueRunes 首段 rune 数低于该阈值时触发智能续取下一段。
// 标题型 README（如 "# asgf"）首段仅标题，需续取描述段才有意义。
const readmeSummaryMinContinueRunes = 30

// readmeSummaryTargetRunes 续取累计 rune 数达到该阈值后停止，平衡摘要信息量与简洁性。
const readmeSummaryTargetRunes = 100

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
// （非 UTF-8 按 GBK 降级）-> 去除 Markdown 标记取首段非空文本 -> 智能续取 -> 截断 200 字。
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

// ReadFullReadme 读取仓库根目录下 README 文件的完整文本（不截断摘要，供二级弹窗渲染）。
// 流程：路径校验（须存在且为目录）-> findReadme 定位 -> util.ReadFileSafe 读取（上限 1MB）
// -> DetectTextEncoding 转 UTF-8。无 README / 二进制 / 编码不可识别 / 路径非目录 均返回空串。
func ReadFullReadme(repoPath string) string {
	// 路径校验：须存在且为目录，防越界读取任意文件
	info, err := os.Stat(repoPath)
	if err != nil || !info.IsDir() {
		return ""
	}
	readmePath := findReadme(repoPath)
	if readmePath == "" {
		return ""
	}
	data, err := util.ReadFileSafe(readmePath, readmeFullReadLimit)
	if err != nil {
		return ""
	}
	_, content, ok := util.DetectTextEncoding(data)
	if !ok {
		return ""
	}
	return content
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

// extractReadmeSummary 从 README 文本中提取摘要：去 Markdown 标记 -> 取首段非空文本
// -> 智能续取 -> 截断。段落定义：跳过前导空行后，到首个空行（或全文结尾）之间的内容。
//
// 智能续取（优化 4a）：标题型 README（如 asgf：`# asgf` + 空行 + 描述）首段仅标题，
// 字符数 < 30 时跳过空行续取下一段，累计直到 >= 100 字或遇 H1/H2 标题停止。
// 3-6 级标题（### 等）不视为边界，按正文继续累计。续取结果仍受 readmeMaxSummaryLen 截断。
func extractReadmeSummary(content string) string {
	lines := strings.Split(content, "\n")
	// 收集段落：每个段落是连续的非空行（cleaned 后拼接），同时记录段落首行原始文本
	// 供续取阶段判定 H1/H2 标题（须在 cleanMarkdownLine 之前判定，否则 # 已被剥离）。
	type paragraph struct {
		text     string // cleaned + 空格拼接的段落文本
		firstRaw string // 段落首行原始文本（用于标题判定）
	}
	var paragraphs []paragraph
	var cur []string
	var curFirstRaw string
	started := false
	for _, raw := range lines {
		cleaned := strings.TrimSpace(cleanMarkdownLine(raw))
		if cleaned == "" {
			// 首段已开始 -> 空行结束当前段落；未开始 -> 跳过前导空行
			if started && len(cur) > 0 {
				paragraphs = append(paragraphs, paragraph{
					text:     strings.Join(cur, " "),
					firstRaw: curFirstRaw,
				})
				cur = nil
				curFirstRaw = ""
			}
			started = false
			continue
		}
		if !started {
			started = true
			curFirstRaw = raw
		}
		cur = append(cur, cleaned)
	}
	if len(cur) > 0 {
		paragraphs = append(paragraphs, paragraph{
			text:     strings.Join(cur, " "),
			firstRaw: curFirstRaw,
		})
	}
	if len(paragraphs) == 0 {
		return ""
	}

	// 首段必取
	summary := paragraphs[0].text
	// 首段 rune 数 < 阈值 -> 智能续取后续段落，累计到目标字数或遇 H1/H2 标题停止
	if utf8.RuneCountInString(summary) < readmeSummaryMinContinueRunes {
		for i := 1; i < len(paragraphs); i++ {
			p := paragraphs[i]
			// 续取段落首行若为 H1/H2 标题 -> 视为正文边界，停止续取
			if isH1H2Header(p.firstRaw) {
				break
			}
			if summary != "" {
				summary += " "
			}
			summary += p.text
			if utf8.RuneCountInString(summary) >= readmeSummaryTargetRunes {
				break
			}
		}
	}
	return truncateRunes(summary, readmeMaxSummaryLen)
}

// isH1H2Header 判定原始行是否为 1-2 级 Markdown 标题（^#{1,2}\s）。
// 仅在续取阶段用于判定段落首行（cleanMarkdownLine 之前），3-6 级标题（### 等）
// 不视为边界（视为正文继续累计）。前导空白（空格/制表符）允许。
func isH1H2Header(raw string) bool {
	s := strings.TrimLeft(raw, " \t")
	if !strings.HasPrefix(s, "#") {
		return false
	}
	// 统计前导 # 数量
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n > 2 {
		return false
	}
	// # 后须紧跟空白或行尾；"#" 单独成行也视为标题边界
	if n == len(s) {
		return true
	}
	return s[n] == ' ' || s[n] == '\t'
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
