package service

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// FTS5 全文检索切分层（素材管理）。
//
// 选型依据（research/sqlite-fts5-chinese.md 10 万行实测）：unicode61 分词器对
// 连续中文切不出词级 token，必须配合应用层 CJK 按字切分 + phrase 查询，语义与
// LIKE 子串匹配精确等价，且 2 字词（如「获客」）可命中（trigram 方案的最小
// 3 字符硬伤被排除）。
//
// 三条硬约束（research 坑清单）：
//  1. 写入与查询必须经同一 splitFtsText 切分（切分不同源 = 静默漏检）；
//  2. phrase 查询依赖 token 位置信息，FTS 表必须保持 detail=full（见 schema）；
//  3. 命中摘要不用 FTS snippet()（输出含切分空格需逆处理），改从原文截窗口。

// isCJKIdeograph CJK 表意文字判定（切分区间的唯一来源，写入与查询共用）。
// 区间取 research 实测口径：U+3400–9FFF（扩展 A + 基本区）、U+F900–FAFF
// （兼容表意文字）、U+20000–2FA1F（扩展 B/C/D + 兼容补充）。拉丁字母、数字、
// 标点、空白不属表意文字，不触发切分（ASCII 词保持整体，大小写折叠交给
// unicode61）。
func isCJKIdeograph(r rune) bool {
	switch {
	case r >= 0x3400 && r <= 0x9FFF:
		return true
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0x20000 && r <= 0x2FA1F:
		return true
	}
	return false
}

// splitFtsText FTS 索引/查询同源切分：连续 CJK 字符逐字切开（字间插单空格），
// 拉丁/数字连续串保持词内连续，CJK 与拉丁/数字混合边界插空格；原有空白原样
// 保留（对 unicode61 均为 token 分隔符）。
//
// 示例：「全面服务城商行客户」→「全 面 服 务 城 商 行 客 户」；
// 「AB产品」→「AB 产 品」；「Q3汇报」→「Q3 汇 报」。
//
// 写入侧（FTS 双写）与查询侧（buildFtsQuery）必须调用本函数，禁止任何一方
// 自行实现切分逻辑。
func splitFtsText(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	lastSpace := true // 视初值为空白：首个 CJK 字符不产生前导空格
	for _, r := range s {
		if isCJKIdeograph(r) {
			if !lastSpace {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			b.WriteByte(' ')
			lastSpace = true
			continue
		}
		b.WriteRune(r)
		lastSpace = r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}
	return strings.TrimSpace(b.String())
}

// buildFtsQuery 用户关键词 → FTS5 MATCH 表达式：按空白拆词，每词经 splitFtsText
// 切分后以双引号 phrase 包裹（短语语法，保证按字 token 序列相邻匹配），多词以
// AND 连接（全部词命中才算命中）。phrase 内双引号按 FTS5 字符串语法翻倍转义。
// 无有效查询词（空串/纯空白/全标点）返回空串，调用方按空结果处理不报错。
func buildFtsQuery(keyword string) string {
	words := strings.Fields(keyword)
	if len(words) == 0 {
		return ""
	}
	parts := make([]string, 0, len(words))
	for _, w := range words {
		tokens := splitFtsText(w)
		if !hasTokenizableRune(tokens) {
			continue
		}
		parts = append(parts, `"`+strings.ReplaceAll(tokens, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " AND ")
}

// hasTokenizableRune 判定切分后的词是否含可索引 token 字符（字母/数字/CJK 表意
// 文字）。unicode61 分词器只把字母/数字类 rune 产出为 token，纯标点/符号词切分
// 后为「零 token phrase」——实测（2026-10-14，modernc.org/sqlite）MATCH 不报错
// 而是恒 0 行（审核报告标 syntax error 为 unverified，实测不成立），故此跳过属
// 防御性收口而非修错：让纯标点词走「无有效查询词 → 空结果」路径，语义更明确，
// 且不依赖「零 token phrase 恒空行」的分词器实现细节。保留实测守护测试
// （TestBuildFtsQuery 纯标点词用例 + TestSearchFts_ZeroTokenPhraseIsSafe）。
func hasTokenizableRune(s string) bool {
	for _, r := range s {
		if isCJKIdeograph(r) || unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// 命中摘要窗口参数（rune 口径）：命中词前保留 20 字、后保留 40 字上下文，
// 超出截断并加省略号（约 60 字窗口足够识别命中语境，与画廊卡片高度匹配）。
const (
	assetsSnippetContextBefore = 20
	assetsSnippetContextAfter  = 40
)

// snippetFromTexts 命中摘要：依次在各候选原文（按展示优先级排列）中定位关键词
// 首次出现位置，取首个命中列的原文窗口；全部未命中返回空串。
//
// 多词关键词（按空白拆词，与 buildFtsQuery 同一拆词口径）：FTS 匹配语义为逐词
// AND（各词在列内独立命中，不要求原文连续出现），原文通常不含「词1 词2」连续
// 字面形态，整体定位会恒空——整体未命中时逐词定位，取首个命中词的窗口。
//
// 实现说明（弃用 FTS snippet()）：snippet() 输出的是切分后文本（CJK 字间带
// 空格，research 坑 4），逆处理合并不可靠；改为用用户关键词原文在页/文件原文
// 中定位首次出现位置截取窗口，输出与原文逐字一致，展示无需后处理。
func snippetFromTexts(keyword string, texts []string) string {
	if keyword == "" {
		return ""
	}
	words := strings.Fields(keyword)
	for _, text := range texts {
		if strings.Contains(text, keyword) {
			return buildSnippetWindow(text, keyword)
		}
		// 整体字面未命中的多词场景：逐词定位取首个命中词窗口（单词场景 Fields
		// 后即原词，上方 Contains 已判定不命中，无需重复）
		if len(words) > 1 {
			for _, w := range words {
				if strings.Contains(text, w) {
					return buildSnippetWindow(text, w)
				}
			}
		}
	}
	return ""
}

// buildSnippetWindow 单列原文窗口截取：以关键词首次出现位置为锚，前后各取
// 上下文 rune（rune 口径截取防多字节字符截半），越界收口，两端截断加省略号。
// 关键词未被原文包含时（切分边界差异的罕见形态）回退取原文首部窗口。
func buildSnippetWindow(text, keyword string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)

	// 定位关键词首次出现的 rune 偏移（strings.Index 返回字节索引，须换算）
	start := 0
	if byteIdx := strings.Index(text, keyword); byteIdx > 0 {
		start = utf8.RuneCountInString(text[:byteIdx])
	} else if byteIdx < 0 {
		start = 0
	}
	if start > assetsSnippetContextBefore {
		start -= assetsSnippetContextBefore
	} else {
		start = 0
	}

	end := start + utf8.RuneCountInString(keyword) + assetsSnippetContextAfter
	if end > len(runes) {
		end = len(runes)
	}

	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	b.WriteString(string(runes[start:end]))
	if end < len(runes) {
		b.WriteString("…")
	}
	return b.String()
}
