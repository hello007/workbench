package service

import (
	"testing"

	"workbench/model"
)

// bcCommit 构造配对测试用的提交（hash 兼作唯一标识，date 秒）。
func bcCommit(hash string, date int64, subject, patchID string) model.BranchCompareCommit {
	return model.BranchCompareCommit{
		Hash:      hash,
		ShortHash: hash[:8],
		Author:    "tester",
		Email:     "test@test.local",
		Date:      date,
		Subject:   subject,
		PatchID:   patchID,
	}
}

// TestPairCommits_HashExact hash 精确匹配：同 hash 提交配对，MatchedBy=hash。
func TestPairCommits_HashExact(t *testing.T) {
	rows := pairCommits(
		[]model.BranchCompareCommit{bcCommit("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100, "feat: a", "p1")},
		[]model.BranchCompareCommit{bcCommit("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100, "feat: a", "p1")},
	)
	if len(rows) != 1 {
		t.Fatalf("应为 1 行, got %d", len(rows))
	}
	r := rows[0]
	if r.Side != model.RowSideMatched || r.MatchedBy != model.RowMatchedByHash {
		t.Errorf("应 hash 配对, got side=%s matchedBy=%s", r.Side, r.MatchedBy)
	}
	if r.A == nil || r.B == nil || r.A.Hash != r.B.Hash {
		t.Errorf("配对行双侧提交应同 hash, got %+v", r)
	}
	if r.Key != r.A.Hash+".."+r.B.Hash {
		t.Errorf("配对行 Key 应为 hash 组合, got %q", r.Key)
	}
}

// TestPairCommits_PatchIDCherryPick patch-id 匹配：hash 不同但 patch-id 相同（真 cherry-pick）配对，
// MatchedBy=patch-id。
func TestPairCommits_PatchIDCherryPick(t *testing.T) {
	rows := pairCommits(
		[]model.BranchCompareCommit{bcCommit("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100, "feat: x", "pid-1")},
		[]model.BranchCompareCommit{bcCommit("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 200, "feat: x", "pid-1")},
	)
	if len(rows) != 1 || rows[0].MatchedBy != model.RowMatchedByPatchID {
		t.Fatalf("应 patch-id 配对, got %+v", rows)
	}
}

// TestPairCommits_SubjectMatchWithSuffix 归一化 subject 匹配：剥离 cherry-pick 尾注 + 大小写不敏感。
func TestPairCommits_SubjectMatchWithSuffix(t *testing.T) {
	rows := pairCommits(
		[]model.BranchCompareCommit{bcCommit("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100, "Feat: Add Login (cherry picked from commit deadbeefdeadbeefdeadbeefdeadbeefdeadbeef)", "p-a")},
		[]model.BranchCompareCommit{bcCommit("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 200, "feat: add login", "p-b")},
	)
	if len(rows) != 1 || rows[0].MatchedBy != model.RowMatchedBySubject {
		t.Fatalf("应 subject 配对（尾注剥离 + 大小写不敏感）, got %+v", rows)
	}
}

// TestPairCommits_TierPrecedenceHashWins 三级优先级：A 提交同时与 B1 hash 相同、与 B2 patch-id
// 相同时，hash 级先命中 B1（每级独立扫描找第一个未使用匹配）。
func TestPairCommits_TierPrecedenceHashWins(t *testing.T) {
	a := bcCommit("cccccccccccccccccccccccccccccccccccccccc", 100, "feat: same", "pid-shared")
	b1 := bcCommit("cccccccccccccccccccccccccccccccccccccccc", 100, "feat: same", "pid-other")
	b2 := bcCommit("dddddddddddddddddddddddddddddddddddddddd", 200, "feat: same", "pid-shared")
	rows := pairCommits([]model.BranchCompareCommit{a}, []model.BranchCompareCommit{b1, b2})
	if len(rows) != 2 {
		t.Fatalf("应为 2 行（1 配对 + 1 b-only）, got %d", len(rows))
	}
	var matched *model.BranchCompareRow
	for i := range rows {
		if rows[i].Side == model.RowSideMatched {
			matched = &rows[i]
		}
	}
	if matched == nil {
		t.Fatalf("应存在配对行, got %+v", rows)
	}
	if matched.MatchedBy != model.RowMatchedByHash {
		t.Errorf("hash 级应先于 patch-id 级命中, got matchedBy=%s", matched.MatchedBy)
	}
	if matched.B.Hash != b1.Hash {
		t.Errorf("hash 配对应命中 B1, got %s", matched.B.Hash)
	}
	for _, r := range rows {
		if r.Side == model.RowSideBOnly && r.B.Hash != b2.Hash {
			t.Errorf("剩余 B2 应为 b-only, got %+v", r)
		}
	}
}

// TestPairCommits_OneToOneNonReuse 一配一无复用：两个 A 提交 patch-id 相同、B 侧只有一个
// 该 patch-id 提交时，仅一个配对，另一个 a-only。
func TestPairCommits_OneToOneNonReuse(t *testing.T) {
	aList := []model.BranchCompareCommit{
		bcCommit("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100, "feat: dup", "pid-dup"),
		bcCommit("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 200, "feat: dup", "pid-dup"),
	}
	bList := []model.BranchCompareCommit{
		bcCommit("dddddddddddddddddddddddddddddddddddddddd", 300, "feat: dup", "pid-dup"),
	}
	rows := pairCommits(aList, bList)
	matched, aOnly := 0, 0
	for _, r := range rows {
		switch r.Side {
		case model.RowSideMatched:
			matched++
		case model.RowSideAOnly:
			aOnly++
		}
	}
	if matched != 1 || aOnly != 1 {
		t.Errorf("应恰 1 配对 + 1 a-only, got matched=%d aOnly=%d", matched, aOnly)
	}
}

// TestPairCommits_LeftoversAndOrdering 独有提交与排序：配对行取 max(a.Date,b.Date) 降序，
// a-only/b-only 剩余齐全，MatchedBy=none。
func TestPairCommits_LeftoversAndOrdering(t *testing.T) {
	aList := []model.BranchCompareCommit{
		bcCommit("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100, "a1", "pid-pair"),
		bcCommit("cccccccccccccccccccccccccccccccccccccccc", 900, "a2", "pa2"), // a-only
	}
	bList := []model.BranchCompareCommit{
		bcCommit("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 500, "b1", "pid-pair"),
	}
	// a1 与 b1 patch-id 相同 → 配对（max date = 500）；a2 a-only（900）；b1 已用无 b-only
	rows := pairCommits(aList, bList)
	if len(rows) != 2 {
		t.Fatalf("应为 2 行, got %d", len(rows))
	}
	if rows[0].Key != "cccccccccccccccccccccccccccccccccccccccc" {
		t.Errorf("a2（max date 900）应排首, got key=%s", rows[0].Key)
	}
	if rows[0].Side != model.RowSideAOnly || rows[0].MatchedBy != model.RowMatchedByNone {
		t.Errorf("a2 应为 a-only/none, got %+v", rows[0])
	}
	if rows[1].Side != model.RowSideMatched || rows[1].MatchedBy != model.RowMatchedByPatchID {
		t.Errorf("a1/b1 应 patch-id 配对, got %+v", rows[1])
	}
}

// TestPairCommits_EmptyInputs 空输入：任一侧为空返空切片不 panic。
func TestPairCommits_EmptyInputs(t *testing.T) {
	if rows := pairCommits(nil, nil); len(rows) != 0 {
		t.Errorf("双侧空应 0 行, got %d", len(rows))
	}
	if rows := pairCommits([]model.BranchCompareCommit{bcCommit("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1, "x", "")}, nil); len(rows) != 1 || rows[0].Side != model.RowSideAOnly {
		t.Errorf("B 空应全 a-only, got %+v", rows)
	}
}

// TestNormalizeSubject 归一化表驱动：尾注剥离 / 无尾注原样 / 大小写 / 纯空白。
func TestNormalizeSubject(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"feat: add (cherry picked from commit abc123)", "feat: add"},
		{"(cherry picked from commit abc123) feat: add", "feat: add"},
		{"  Feat: ADD  ", "feat: add"},
		{"feat: no suffix", "feat: no suffix"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeSubject(c.in); got != c.want {
			t.Errorf("normalizeSubject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestParseCommitRecords 记录解析表驱动：正常记录 / subject 含 | / 正文 \x01 注入折叠 /
// 多父 / 畸形行跳过 / 首前导 \x00 空块 / 末记录缺尾分隔容忍。
func TestParseCommitRecords(t *testing.T) {
	sep, fld := "\x00", "\x01"
	out := sep + "1111111111111111111111111111111111111111" + fld + "11111111" + fld + "Alice" + fld + "a@x.com" + fld + "100" + fld + "feat: with | pipe" + fld + "body line" + fld + "p1p1p1p1" + "\n" +
		sep + "2222222222222222222222222222222222222222" + fld + "22222222" + fld + "Bob" + fld + "b@x.com" + fld + "200" + fld + "feat: inject" + fld + "body\x01with-injected-sep" + fld + "p1p1p1p1 p2p2p2p2" + "\n" +
		sep + "bad-record" + "\n" +
		sep + "3333333333333333333333333333333333333333" + fld + "33333333" + fld + "Cara" + fld + "c@x.com" + fld + "300" + fld + "root" + fld + "" + fld // 末记录缺尾分隔
	commits := parseCommitRecords(out, "branch-a")
	if len(commits) != 3 {
		t.Fatalf("应解析 3 条（畸形行跳过）, got %d: %+v", len(commits), commits)
	}
	c1 := commits[0]
	if c1.Subject != "feat: with | pipe" {
		t.Errorf("subject 含 | 应原样保留, got %q", c1.Subject)
	}
	if c1.Body != "body line" || c1.BranchName != "branch-a" {
		t.Errorf("body/branchName 解析错误, got %+v", c1)
	}
	if len(c1.ParentHashes) != 1 || c1.ParentHashes[0] != "p1p1p1p1" {
		t.Errorf("单父解析错误, got %v", c1.ParentHashes)
	}
	c2 := commits[1]
	if c2.Body != "body\x01with-injected-sep" {
		t.Errorf("正文注入 \\x01 应折叠回 body, got %q", c2.Body)
	}
	if len(c2.ParentHashes) != 2 || c2.ParentHashes[0] != "p1p1p1p1" || c2.ParentHashes[1] != "p2p2p2p2" {
		t.Errorf("多父解析错误, got %v", c2.ParentHashes)
	}
	c3 := commits[2]
	if c3.Subject != "root" || c3.Body != "" || c3.ParentHashes != nil {
		t.Errorf("root 记录（空 body/parents）解析错误, got %+v", c3)
	}
	if c3.Date != 300 || c3.Hash != "3333333333333333333333333333333333333333" {
		t.Errorf("末记录缺尾分隔应容忍, got %+v", c3)
	}
	if got := parseCommitRecords("", "x"); len(got) != 0 {
		t.Errorf("空输入应 0 条, got %d", len(got))
	}
	if got := parseCommitRecords("junk-without-separator", "x"); len(got) != 0 {
		t.Errorf("无 \\x00 的杂散输出应 0 条, got %d", len(got))
	}
}

// TestParseNameStatus name-status 解析：单路径 A/D/M/T 与双路径 R/C（含分数剥离）、畸形行跳过。
func TestParseNameStatus(t *testing.T) {
	out := "A\tnew.txt\nD\tgone.txt\nM\tmod.txt\nT\tlink.txt\nR100\told.txt\tnew-name.txt\nC75\tsrc.txt\tdst.txt\n\nbogus-line"
	m := parseNameStatus(out)
	if len(m) != 6 {
		t.Fatalf("应 6 条（空行与畸形行跳过）, got %d: %+v", len(m), m)
	}
	if e, ok := m["new.txt"]; !ok || e.Status != "A" {
		t.Errorf("new.txt 应 A, got %+v", e)
	}
	if e, ok := m["gone.txt"]; !ok || e.Status != "D" {
		t.Errorf("gone.txt 应 D, got %+v", e)
	}
	if e, ok := m["mod.txt"]; !ok || e.Status != "M" || e.OldPath != "" {
		t.Errorf("mod.txt 应 M 无旧路径, got %+v", e)
	}
	if e, ok := m["link.txt"]; !ok || e.Status != "T" {
		t.Errorf("link.txt 应 T, got %+v", e)
	}
	if e, ok := m["new-name.txt"]; !ok || e.Status != "R" || e.OldPath != "old.txt" {
		t.Errorf("rename 双路径解析错误, got %+v", e)
	}
	if e, ok := m["dst.txt"]; !ok || e.Status != "C" || e.OldPath != "src.txt" {
		t.Errorf("copy 双路径解析错误, got %+v", e)
	}
	if _, ok := m["old.txt"]; ok {
		t.Error("rename 旧路径不应作为 map 键（键为新路径，旧路径由 OldPath 承载）")
	}
}

// TestParseNumstat numstat 解析：普通计数 / "-" 二进制 / rename 花括号与直排两种形态。
func TestParseNumstat(t *testing.T) {
	out := "3\t1\ta.txt\n-\t-\tbin.dat\n2\t0\tsrc/{old.go => new.go}\n1\t1\told-name.txt => new-name.txt"
	m := parseNumstat(out)
	if e := m["a.txt"]; e.Additions != 3 || e.Deletions != 1 || e.Binary {
		t.Errorf("a.txt 计数错误, got %+v", e)
	}
	if e := m["bin.dat"]; !e.Binary || e.Additions != 0 || e.Deletions != 0 {
		t.Errorf("bin.dat 应二进制, got %+v", e)
	}
	if e := m["src/old.go"]; e.Additions != 2 || e.Binary {
		t.Errorf("rename 旧路径应登记, got %+v", e)
	}
	if e := m["src/new.go"]; e.Deletions != 0 || e.Binary {
		t.Errorf("rename 新路径应登记, got %+v", e)
	}
	if _, ok := m["old-name.txt"]; !ok {
		t.Error("直排 rename 旧路径应登记")
	}
	if _, ok := m["new-name.txt"]; !ok {
		t.Error("直排 rename 新路径应登记")
	}
}

// TestExpandNumstatRenamePath rename 路径展开：普通 / 花括号公共前缀 / 直排。
func TestExpandNumstatRenamePath(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a.txt", []string{"a.txt"}},
		{"src/{old.go => new.go}", []string{"src/old.go", "src/new.go"}},
		{"old.txt => new.txt", []string{"old.txt", "new.txt"}},
	}
	for _, c := range cases {
		got := expandNumstatRenamePath(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("expand(%q) = %v, want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("expand(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// TestComputeStats 统计聚合：各侧计数 + 疑似拣选数 + 共同历史透传。
func TestComputeStats(t *testing.T) {
	rows := []model.BranchCompareRow{
		{Side: model.RowSideMatched, MatchedBy: model.RowMatchedByHash},
		{Side: model.RowSideMatched, MatchedBy: model.RowMatchedByPatchID},
		{Side: model.RowSideMatched, MatchedBy: model.RowMatchedBySubject},
		{Side: model.RowSideAOnly},
		{Side: model.RowSideAOnly},
		{Side: model.RowSideBOnly},
	}
	stats := computeStats(rows, 42)
	if stats.AOnly != 2 || stats.BOnly != 1 || stats.Matched != 3 || stats.PatchMatched != 1 || stats.CommonBelow != 42 {
		t.Errorf("统计错误, got %+v", stats)
	}
}

// TestParsePatchIDOutput patch-id 输出解析：正常行 / merge 提交无输出的空行 / 畸形行跳过。
func TestParsePatchIDOutput(t *testing.T) {
	out := "pid-1 1111111111111111111111111111111111111111\n\nnot-two-fields\npid-2 2222222222222222222222222222222222222222\n"
	m := parsePatchIDOutput(out)
	if len(m) != 2 {
		t.Fatalf("应 2 条, got %d", len(m))
	}
	if m["1111111111111111111111111111111111111111"] != "pid-1" {
		t.Errorf("hash→patchId 映射错误, got %v", m)
	}
}
