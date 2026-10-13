package service

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------- splitFtsText 切分器单测 ----------

// TestSplitFtsText 切分规则全形态：纯 CJK 逐字切开、拉丁/数字保持词内连续、
// 混合边界插空格、标点/空白原样、空串返回空。CJK 尾随空格与相邻标点/原有
// 空白叠加产生的连续空格对 unicode61 均为 token 分隔符，不影响语义。
func TestSplitFtsText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"空串", "", ""},
		{"纯空白", "  \t\n ", ""},
		{"纯 CJK 连续", "城商行", "城 商 行"},
		{"纯 CJK 长句", "全面服务城商行客户数字化转型需求", "全 面 服 务 城 商 行 客 户 数 字 化 转 型 需 求"},
		{"纯拉丁", "hello world", "hello world"},
		{"纯数字", "Q3 2026", "Q3 2026"},
		{"CJK 拉丁混合", "AB产品", "AB 产 品"},
		{"拉丁 CJK 混合", "Q3汇报", "Q3 汇 报"},
		{"CJK 包围拉丁", "服务AB客群", "服 务 AB 客 群"},
		{"中文标点保持", "城商行，获客。", "城 商 行 ， 获 客 。"},
		{"半角标点保持", "城商行-获客", "城 商 行 - 获 客"},
		{"多空白保留", "城 商 行", "城  商  行"},
		{"CJK 扩展B区间", "\U00020000\U00020001", "\U00020000 \U00020001"},
		{"兼容表意区", "豈更", "豈 更"},
		{"英文引号转义素材", `say "hi"`, `say "hi"`},
	}
	for _, tc := range cases {
		if got := splitFtsText(tc.in); got != tc.want {
			t.Errorf("%s: splitFtsText(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestBuildFtsQuery 查询构造：按空白拆词逐词 phrase、多词 AND、双引号翻倍转义、
// 空查询返回空串（调用方按空结果处理）。
func TestBuildFtsQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"空串", "", ""},
		{"纯空白", "   ", ""},
		{"单词 CJK", "城商行", `"城 商 行"`},
		{"多词 AND", "城商行 获客", `"城 商 行" AND "获 客"`},
		{"多空白拆词", "  城商行   获客  ", `"城 商 行" AND "获 客"`},
		{"拉丁词", "AB产品", `"AB 产 品"`},
		{"双引号转义", `城"商`, `"城 "" 商"`},
	}
	for _, tc := range cases {
		if got := buildFtsQuery(tc.in); got != tc.want {
			t.Errorf("%s: buildFtsQuery(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestSplitFtsText_WriteQuerySameSource 写入/查询同源回归（research 坑 1）：
// 未切分原文按 splitFtsText 写入 FTS 后，buildFtsQuery 对同一关键词构造的
// phrase 必须命中——同源性一旦破坏即静默漏检，此用例锚定最基本闭环。
func TestSplitFtsText_WriteQuerySameSource(t *testing.T) {
	store, err := openAssetsStore(filepath.Join(t.TempDir(), "fts.db"))
	if err != nil {
		t.Fatalf("打开临时索引库: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	text := "全面服务城商行客户数字化转型需求"
	if _, err := store.db.Exec(
		"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, file_name, file_note, slide_title, slide_text, slide_notes) "+
			"VALUES(-1, 'slide', 1, 1, '', '', ?, ?, '')",
		splitFtsText("获客专题"), splitFtsText(text),
	); err != nil {
		t.Fatalf("写入切分文本: %v", err)
	}

	for _, keyword := range []string{"城商行", "获客", "转型"} {
		hits, err := store.searchFts(buildFtsQuery(keyword))
		if err != nil {
			t.Fatalf("检索 %q: %v", keyword, err)
		}
		if len(hits) != 1 {
			t.Errorf("写入查询同源回归：关键词 %q 应命中 1 行，实际 %d", keyword, len(hits))
		}
	}

	// 未出现的词不得误命中（子串匹配的负例）
	hits, err := store.searchFts(buildFtsQuery("信贷"))
	if err != nil {
		t.Fatalf("检索负例: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("未出现的词不应命中，实际 %d 行", len(hits))
	}
}

// TestSnippetFromTexts 命中摘要：取首个包含关键词的候选列原文窗口，两端截断
// 加省略号；全部未命中返回空串；空关键词返回空串。
func TestSnippetFromTexts(t *testing.T) {
	long := strings.Repeat("前", 30) + "城商行获客策略" + strings.Repeat("后", 60)
	got := snippetFromTexts("城商行", []string{"不含关键词", long})
	if !strings.Contains(got, "城商行获客策略") {
		t.Errorf("摘要应包含命中词原文，实际 %q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Errorf("两端截断应加省略号，实际 %q", got)
	}
	// rune 口径窗口：前 20 字上下文 + 关键词 + 后 40 字
	runes := []rune(got)
	if len(runes) > 20+len("城商行获客策略")+40+2 {
		t.Errorf("摘要窗口应受上下文参数约束，实际 %d rune", len(runes))
	}

	if got := snippetFromTexts("城商行", []string{"无", "关 键 词 不 在"}); got != "" {
		t.Errorf("全部未命中应返回空串，实际 %q", got)
	}
	if got := snippetFromTexts("", []string{"任意"}); got != "" {
		t.Errorf("空关键词应返回空串，实际 %q", got)
	}
	// 首个命中列优先
	if got := snippetFromTexts("命中", []string{"命中在首列", "命中在次列"}); got != "命中在首列" {
		t.Errorf("应取首个命中列，实际 %q", got)
	}
	// 短文本不溢出
	if got := buildSnippetWindow("城商行", "城商行"); got != "城商行" {
		t.Errorf("短文本应原样返回不加省略号，实际 %q", got)
	}
}
