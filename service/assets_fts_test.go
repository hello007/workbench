package service

import (
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
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
// 空查询返回空串（调用方按空结果处理）；纯标点/符号词（切分后不含可索引 token
// 字符）防御性跳过，全跳过返回空串。
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
		// 纯标点/符号词：unicode61 不产出 token（零 token phrase），跳过不走 MATCH
		{"纯半角标点", "!!!", ""},
		{"纯中文标点", "……", ""},
		{"纯符号", "@#$", ""},
		{"标点词与有效词混合", "城商行 !!!", `"城 商 行"`},
		{"全部词纯标点", "!!! …… @#$", ""},
	}
	for _, tc := range cases {
		if got := buildFtsQuery(tc.in); got != tc.want {
			t.Errorf("%s: buildFtsQuery(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestSearchFts_ZeroTokenPhraseIsSafe 零 token phrase 行为守护（2026-10-14 实测
// 结论）：unicode61 分词器对纯标点词 phrase 的 MATCH 不报 syntax error 而是恒
// 0 行（审核报告标「报错」为 unverified，实测不成立）。buildFtsQuery 已防御性
// 跳过纯标点词，本用例锚定修复后的全链路行为：纯标点关键词检索无错误、空结果，
// 同库有效词检索不受影响。
func TestSearchFts_ZeroTokenPhraseIsSafe(t *testing.T) {
	store, err := openAssetsStore(filepath.Join(t.TempDir(), "fts.db"))
	if err != nil {
		t.Fatalf("打开临时索引库: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.db.Exec(
		"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, file_name, file_note, slide_title, slide_text, slide_notes) "+
			"VALUES(-1, 'slide', 1, 1, '', '', ?, ?, '')",
		splitFtsText("获客专题"), splitFtsText("全面服务城商行客户"),
	); err != nil {
		t.Fatalf("写入切分文本: %v", err)
	}

	// 纯标点关键词：全链路（buildFtsQuery → searchByKeyword 编排入口）不报错、
	// 空结果。走编排入口而非直调 searchFts：空表达式禁喂 MATCH（FTS 契约），
	// 编排层按空结果短路。
	for _, keyword := range []string{"!!!", "!!! ……", "@#$"} {
		page, err := searchAssetsInStore(store, keyword, model.AssetSearchOptions{Limit: 10})
		if err != nil {
			t.Errorf("纯标点关键词 %q 检索不应报错: %v", keyword, err)
		}
		if len(page.Items) != 0 {
			t.Errorf("纯标点关键词 %q 应空结果，实际 %d 行", keyword, len(page.Items))
		}
	}

	// 同库有效词检索不受防御性跳过影响：混合关键词构造的表达式（仅含有效词
	// phrase）在 FTS 层正常命中（FTS 层断言，编排层 join 断言见检索集成用例）
	hits, err := store.searchFts(buildFtsQuery("城商行 !!!"))
	if err != nil {
		t.Fatalf("混合关键词检索: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("混合关键词应命中 1 行，实际 %d", len(hits))
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

// TestSnippetFromTexts_MultiWord 多词关键词摘要（FTS 逐词 AND 语义）：原文不含
// 多词连续字面形态时整体定位恒空，逐词定位取首个命中词窗口；列优先级仍高于
// 词序（先按列找，列内再按词序）。
func TestSnippetFromTexts_MultiWord(t *testing.T) {
	// 原文无连续字面「城商行 获客」：逐词命中「城商行」
	got := snippetFromTexts("城商行 获客", []string{"城商行获客策略解读"})
	if !strings.Contains(got, "城商行") {
		t.Errorf("多词整体未命中应逐词取「城商行」窗口，实际 %q", got)
	}
	// 首词未命中取次词
	if got := snippetFromTexts("城商行 获客", []string{"获客成本分析"}); !strings.Contains(got, "获客") {
		t.Errorf("首词未命中应取次词「获客」窗口，实际 %q", got)
	}
	// 列优先级高于词序：次列含首词、首列含次词时取首列（展示优先级口径）
	if got := snippetFromTexts("城商行 获客", []string{"获客成本分析", "城商行专题"}); !strings.Contains(got, "获客") {
		t.Errorf("应取首个命中列（首列「获客」），实际 %q", got)
	}
	// 整体字面命中仍优先（单窗口覆盖多词连续出现的原文形态）
	if got := snippetFromTexts("城商行 获客", []string{"城商行 获客双引擎"}); !strings.Contains(got, "城商行 获客") {
		t.Errorf("整体字面命中应优先整词窗口，实际 %q", got)
	}
	// 全部词均未命中返回空串
	if got := snippetFromTexts("城商行 获客", []string{"毫不相干"}); got != "" {
		t.Errorf("全部词未命中应返回空串，实际 %q", got)
	}
}
