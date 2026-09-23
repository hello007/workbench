package service

import (
	"strings"
	"testing"
	"time"

	"workbench/model"
	"workbench/util/testutil"
)

// ===== numstat 解析（纯函数） =====

// sampleNumstatOutput 构造两提交的 numstat 模拟输出：提交 A 两文本文件 + 一二进制，
// 提交 B 单文件（含 rename 路径形态）。
func sampleNumstatOutput() string {
	return "\x00" + strings.Repeat("a", 40) + "\x01Alice\x01alice@x.com\x011700000000\n" +
		"10\t2\tsrc/a.go\n" +
		"5\t0\tsrc/b.go\n" +
		"-\t-\tassets/logo.png\n" +
		"\x00" + strings.Repeat("b", 40) + "\x01Bob\x01bob@x.com\x011700000100\n" +
		"3\t7\told.go => new.go\n"
}

func TestParseCommitNumstatOutput_Basic(t *testing.T) {
	records, err := parseCommitNumstatOutput(sampleNumstatOutput())
	if err != nil {
		t.Fatalf("parseCommitNumstatOutput: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("应解析出 2 条记录: got %d", len(records))
	}
	r0 := records[0]
	if r0.SHA != strings.Repeat("a", 40) {
		t.Errorf("SHA 异常: %q", r0.SHA)
	}
	if r0.Author != "Alice" || r0.Email != "alice@x.com" {
		t.Errorf("作者身份异常: %+v", r0)
	}
	if r0.Timestamp != 1700000000 {
		t.Errorf("Timestamp 异常: got %d", r0.Timestamp)
	}
	// 二进制行不计入：10+5（非 10+5+二进制）
	if r0.Insertions != 15 || r0.Deletions != 2 {
		t.Errorf("行数聚合异常: got ins=%d del=%d, want ins=15 del=2", r0.Insertions, r0.Deletions)
	}
	r1 := records[1]
	// rename 路径 `old => new` 只取数值列，不影响解析
	if r1.Insertions != 3 || r1.Deletions != 7 {
		t.Errorf("rename 行数聚合异常: got ins=%d del=%d, want ins=3 del=7", r1.Insertions, r1.Deletions)
	}
}

func TestParseCommitNumstatOutput_MergeCommitZeroLines(t *testing.T) {
	// merge commit 无文件统计行，自然 0 行不干扰解析
	out := "\x00" + strings.Repeat("c", 40) + "\x01Alice\x01alice@x.com\x011700000200"
	records, err := parseCommitNumstatOutput(out)
	if err != nil {
		t.Fatalf("merge commit 解析不应报错: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("应解析出 1 条记录: got %d", len(records))
	}
	if records[0].Insertions != 0 || records[0].Deletions != 0 {
		t.Errorf("merge commit 应 0 行: got ins=%d del=%d", records[0].Insertions, records[0].Deletions)
	}
}

func TestParseCommitNumstatOutput_EmptyInput(t *testing.T) {
	records, err := parseCommitNumstatOutput("")
	if err != nil {
		t.Fatalf("空输入不应报错: %v", err)
	}
	if records == nil {
		t.Error("空输入应返空切片非 nil")
	}
	if len(records) != 0 {
		t.Errorf("空输入应 0 条记录: got %d", len(records))
	}
}

func TestParseCommitNumstatOutput_MalformedHeader(t *testing.T) {
	cases := []struct {
		name string
		out  string
	}{
		{"SHA 非法（字段错位）", "\x00short-sha\x01Alice\x01a@x.com\x011700000000\n1\t0\ta.go"},
		{"时间戳非数字", "\x00" + strings.Repeat("a", 40) + "\x01Alice\x01a@x.com\x01not-a-ts"},
		{"头部字段不足", "\x00" + strings.Repeat("a", 40) + "\x01Alice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseCommitNumstatOutput(c.out); err == nil {
				t.Errorf("畸形头行应报错，不静默产出错数据")
			}
		})
	}
}

func TestParseCommitNumstatOutput_MalformedFileLine(t *testing.T) {
	// 文件行数值列非数字：格式异常报错，不静默丢行
	out := "\x00" + strings.Repeat("a", 40) + "\x01Alice\x01a@x.com\x011700000000\n" +
		"abc\txyz\ta.go"
	if _, err := parseCommitNumstatOutput(out); err == nil {
		t.Error("文件行数值列非数字应报错")
	}
	// 文件行仅一段（无 tab 分隔）：格式异常报错
	out2 := "\x00" + strings.Repeat("a", 40) + "\x01Alice\x01a@x.com\x011700000000\n" +
		"just-a-path"
	if _, err := parseCommitNumstatOutput(out2); err == nil {
		t.Error("文件行无 tab 分隔应报错")
	}
}

// ===== FetchCommitLineStats（真实临时仓库） =====

func TestFetchCommitLineStats_RealRepo(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)

	// 提交 1：alice 3 行
	testutil.WriteFile(t, repo+"/a.txt", "l1\nl2\nl3\n")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "c1", "--author", "alice <alice@x.com>")

	// 提交 2：bob 2 行新增 + 1 行删除（改写 a.txt 前两行）
	testutil.WriteFile(t, repo+"/a.txt", "n1\nn2\nl3\n")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "c2", "--author", "bob <bob@x.com>")

	// 提交 3：二进制文件（含 NUL 字节，numstat 输出 - -）
	testutil.WriteFile(t, repo+"/logo.bin", "\x00\x01\x02binary")
	testutil.RunGit(t, repo, "add", "logo.bin")
	testutil.RunGit(t, repo, "commit", "-m", "c3 binary", "--author", "bob <bob@x.com>")

	records, err := FetchCommitLineStats(repo)
	if err != nil {
		t.Fatalf("FetchCommitLineStats: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("应解析出 3 条记录: got %d", len(records))
	}

	// 按作者聚合校验：alice ins=3；bob c2(2 ins/2 del) + c3(二进制 0 行)
	agg := AggregateLineStats(records, 0, 0)
	if len(agg) != 2 {
		t.Fatalf("应聚合出 2 位贡献者: got %d", len(agg))
	}
	byAuthor := map[string]model.Contributor{}
	for _, c := range agg {
		byAuthor[c.Author] = c
	}
	alice := byAuthor["alice"]
	if alice.Insertions != 3 || alice.Deletions != 0 {
		t.Errorf("alice 行数异常: got ins=%d del=%d, want ins=3 del=0", alice.Insertions, alice.Deletions)
	}
	bob := byAuthor["bob"]
	if bob.Insertions != 2 || bob.Deletions != 2 {
		t.Errorf("bob 行数异常（二进制行应不计）: got ins=%d del=%d, want ins=2 del=2", bob.Insertions, bob.Deletions)
	}
}

func TestFetchCommitLineStats_MergeCommitZeroLines(t *testing.T) {
	repo := testutil.SetupFFRepo(t) // master 基线 + feature 1 提交，已切回 master

	// no-ff 合并产生 merge commit（缺省 --numstat 无文件统计行）
	testutil.RunGit(t, repo, "merge", "--no-ff", "feature", "-m", "merge feature")

	records, err := FetchCommitLineStats(repo)
	if err != nil {
		t.Fatalf("FetchCommitLineStats: %v", err)
	}
	var mergeStat *model.CommitLineStat
	for i := range records {
		if records[i].Author == "test" && records[i].Insertions == 0 && records[i].Deletions == 0 {
			mergeStat = &records[i]
		}
	}
	if mergeStat == nil {
		t.Errorf("应存在 merge commit 的 0 行记录: %+v", records)
	}
	// 非 merge 提交行数不受 merge 干扰：base 提交 1 行 + feature 提交 1 行 = 2
	testIns := 0
	for _, r := range records {
		if r.Author == "test" {
			testIns += r.Insertions
		}
	}
	if testIns != 2 {
		t.Errorf("非 merge 提交行数应为 2（merge 0 行不干扰）: got %d", testIns)
	}
}

// TestFetchCommitLineStats_EmptyRepo 空仓库（无任何提交）git log 以非零退出码失败，
// 本函数向上透传错误。行为锚点：编排层 GetRepoStats 中 repo.Head() 对空仓库先行报错
// 「无法获取 HEAD 引用」，本错误路径通常不可达；即使触达（如 HEAD 获取与 numstat
// 拉取间历史被清空），app 层降级为 0 行（slog.Warn 留痕）不阻塞提交数统计。
func TestFetchCommitLineStats_EmptyRepo(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	if _, err := FetchCommitLineStats(repo); err == nil {
		t.Error("空仓库 git log 应报错（非零退出码向上透传，由编排层降级）")
	}
}

// ===== CommitLineStatsCache =====

func sampleLineRecords(n int) []model.CommitLineStat {
	out := make([]model.CommitLineStat, n)
	for i := range out {
		out[i] = model.CommitLineStat{
			SHA:       strings.Repeat("a", 40),
			Author:    "author",
			Email:     "a@x.com",
			Timestamp: int64(i),
		}
	}
	return out
}

func TestCommitLineStatsCache_HitAndMiss(t *testing.T) {
	cache := NewCommitLineStatsCache()

	if _, ok := cache.Get("k", "h1"); ok {
		t.Error("未写入前不应命中")
	}

	cache.Set("k", "h1", sampleLineRecords(2))
	got, ok := cache.Get("k", "h1")
	if !ok {
		t.Error("同 headSHA 应命中")
	}
	if len(got) != 2 {
		t.Errorf("命中数据异常: got %d 条", len(got))
	}
}

// TestCommitLineStatsCache_DifferentHeadSHAMiss 二态失效关键路径：headSHA 不同直接
// miss（行数是聚合值无增量 prepend 路径，与提交历史缓存「SHA 不同返 found=true 供
// 增量」的语义不同）。
func TestCommitLineStatsCache_DifferentHeadSHAMiss(t *testing.T) {
	cache := NewCommitLineStatsCache()
	cache.Set("k", "h-old", sampleLineRecords(2))

	if _, ok := cache.Get("k", "h-new"); ok {
		t.Error("headSHA 不同应 miss（无增量路径）")
	}
	// miss 后 Set 覆盖，新 headSHA 命中
	cache.Set("k", "h-new", sampleLineRecords(3))
	got, ok := cache.Get("k", "h-new")
	if !ok || len(got) != 3 {
		t.Errorf("Set 覆盖后新 headSHA 应命中 3 条: ok=%v len=%d", ok, len(got))
	}
}

// TestCommitLineStatsCache_TTLExpiry 注入陈旧 cachedAt 驱动 TTL 过期分支，不依赖
// 真实时间 sleep，确定性 PASS（规避 flaky，同 CommitHistoryCache 范式）。
func TestCommitLineStatsCache_TTLExpiry(t *testing.T) {
	cache := NewCommitLineStatsCache()
	cache.Set("k", "h1", sampleLineRecords(2))

	if _, ok := cache.Get("k", "h1"); !ok {
		t.Error("TTL 内应命中")
	}

	cache.mu.Lock()
	e := cache.entries["k"]
	e.cachedAt = time.Now().Add(-commitLineStatsCacheTTL - time.Second)
	cache.entries["k"] = e
	cache.mu.Unlock()

	if _, ok := cache.Get("k", "h1"); ok {
		t.Error("TTL 过期应 miss")
	}
	cache.mu.Lock()
	_, stillExists := cache.entries["k"]
	cache.mu.Unlock()
	if stillExists {
		t.Error("TTL 过期后条目应被驱逐")
	}
}

func TestCommitLineStatsCache_ClearByGitRoot(t *testing.T) {
	cache := NewCommitLineStatsCache()
	cache.Set("/repoA|refs/heads/master", "h-a1", sampleLineRecords(1))
	cache.Set("/repoA|HEAD|abc", "h-a2", sampleLineRecords(1))
	cache.Set("/repoB|refs/heads/master", "h-b1", sampleLineRecords(1))

	cache.ClearByGitRoot("/repoA")

	if _, ok := cache.Get("/repoA|refs/heads/master", "h-a1"); ok {
		t.Error("clearByGitRoot 后仓库 A master 应清除")
	}
	if _, ok := cache.Get("/repoA|HEAD|abc", "h-a2"); ok {
		t.Error("clearByGitRoot 后仓库 A detached 应清除")
	}
	if _, ok := cache.Get("/repoB|refs/heads/master", "h-b1"); !ok {
		t.Error("clearByGitRoot(/repoA) 不应影响仓库 B")
	}
}

func TestCommitLineStatsCache_ClearAll(t *testing.T) {
	cache := NewCommitLineStatsCache()
	cache.Set("k1", "h1", sampleLineRecords(1))
	cache.Set("k2", "h2", sampleLineRecords(1))

	cache.ClearAll()

	if _, ok := cache.Get("k1", "h1"); ok {
		t.Error("clearAll 后 k1 应未命中")
	}
	if _, ok := cache.Get("k2", "h2"); ok {
		t.Error("clearAll 后 k2 应未命中")
	}
}

// TestCommitLineStatsCache_DeepCopy 纯值 struct 切片复制即深拷贝：Get 返回切片的
// 元素修改与 append、Set 后原切片修改，均不污染缓存内部数据。
func TestCommitLineStatsCache_DeepCopy(t *testing.T) {
	cache := NewCommitLineStatsCache()
	original := sampleLineRecords(2)
	original[0].Insertions = 100
	cache.Set("k", "h1", original)

	// Set 后修改原切片不影响缓存
	original[0].Insertions = 999
	original[0].Author = "mutated"

	got, ok := cache.Get("k", "h1")
	if !ok {
		t.Fatal("应命中")
	}
	got[0].Insertions = 888
	got[0].Author = "mutated-by-get"
	got = append(got, model.CommitLineStat{Author: "extra"})

	got2, _ := cache.Get("k", "h1")
	if got2[0].Insertions != 100 || got2[0].Author != "author" {
		t.Errorf("缓存被污染: got %+v", got2[0])
	}
	if len(got2) != 2 {
		t.Errorf("缓存长度被 append 污染: got %d want 2", len(got2))
	}
}

func TestCommitLineStatsCache_MaxEntriesRejected(t *testing.T) {
	cache := NewCommitLineStatsCache()
	cache.Set("k", "h1", sampleLineRecords(CommitHistoryMaxEntries+1))
	if _, ok := cache.Get("k", "h1"); ok {
		t.Error("超上限条数不应写入缓存")
	}
}

// ===== AggregateLineStats / MergeContributorLineStats / StatsRangeWindow =====

func TestAggregateLineStats_GroupSumAndSort(t *testing.T) {
	records := []model.CommitLineStat{
		{Author: "alice", Email: "a@x.com", Timestamp: 100, Insertions: 10, Deletions: 5},
		{Author: "bob", Email: "b@x.com", Timestamp: 110, Insertions: 40, Deletions: 0},
		{Author: "alice", Email: "a@x.com", Timestamp: 120, Insertions: 20, Deletions: 15},
		// 同名不同邮箱视为不同贡献者
		{Author: "alice", Email: "alice@other.com", Timestamp: 130, Insertions: 1, Deletions: 1},
	}
	agg := AggregateLineStats(records, 0, 0)
	if len(agg) != 3 {
		t.Fatalf("应聚合出 3 位贡献者: got %d", len(agg))
	}
	// 排序：alice@x.com 总变更 50（ins=30+del=20）> bob 40 > alice@other 2
	if agg[0].Author != "alice" || agg[0].Email != "a@x.com" {
		t.Errorf("首位应为 alice@x.com（总变更 50）: got %+v", agg[0])
	}
	if agg[0].Insertions != 30 || agg[0].Deletions != 20 {
		t.Errorf("alice@x.com 行数聚合异常: got ins=%d del=%d, want ins=30 del=20", agg[0].Insertions, agg[0].Deletions)
	}
	if agg[1].Author != "bob" {
		t.Errorf("第二位应为 bob（总变更 40）: got %+v", agg[1])
	}
	if agg[2].Author != "alice" || agg[2].Email != "alice@other.com" {
		t.Errorf("第三位应为 alice@other.com（总变更 2）: got %+v", agg[2])
	}
	// Count 由提交历史通道聚合，此处恒 0
	if agg[0].Count != 0 {
		t.Errorf("行数通道 Count 应为 0: got %d", agg[0].Count)
	}
}

func TestAggregateLineStats_TieBreakByAuthorName(t *testing.T) {
	// 总变更相同（10）时按作者名升序
	records := []model.CommitLineStat{
		{Author: "carol", Email: "c@x.com", Timestamp: 100, Insertions: 10, Deletions: 0},
		{Author: "bob", Email: "b@x.com", Timestamp: 110, Insertions: 5, Deletions: 5},
		{Author: "alice", Email: "a@x.com", Timestamp: 120, Insertions: 2, Deletions: 8},
	}
	agg := AggregateLineStats(records, 0, 0)
	if agg[0].Author != "alice" || agg[1].Author != "bob" || agg[2].Author != "carol" {
		t.Errorf("同值应按作者名升序: got %s, %s, %s", agg[0].Author, agg[1].Author, agg[2].Author)
	}
}

func TestAggregateLineStats_WindowFilter(t *testing.T) {
	records := []model.CommitLineStat{
		{Author: "old", Email: "o@x.com", Timestamp: 100, Insertions: 100, Deletions: 0},
		{Author: "edge-start", Email: "e@x.com", Timestamp: 200, Insertions: 1, Deletions: 0},
		{Author: "mid", Email: "m@x.com", Timestamp: 300, Insertions: 2, Deletions: 0},
		{Author: "edge-end", Email: "e@x.com", Timestamp: 400, Insertions: 4, Deletions: 0},
		{Author: "future", Email: "f@x.com", Timestamp: 500, Insertions: 8, Deletions: 0},
	}
	// 窗口 [200, 400] 含端点
	agg := AggregateLineStats(records, 200, 400)
	if len(agg) != 3 {
		t.Fatalf("窗口外贡献者应被过滤: got %d 位", len(agg))
	}
	authors := map[string]bool{}
	for _, c := range agg {
		authors[c.Author] = true
	}
	if authors["old"] || authors["future"] {
		t.Error("窗口外记录不应计入")
	}
	if !authors["edge-start"] || !authors["edge-end"] {
		t.Error("窗口端点记录应计入（含端点语义）")
	}
}

func TestAggregateLineStats_EmptyInput(t *testing.T) {
	agg := AggregateLineStats(nil, 0, 0)
	if agg == nil || len(agg) != 0 {
		t.Errorf("空输入应返空切片: got %+v", agg)
	}
}

func TestMergeContributorLineStats(t *testing.T) {
	contributors := []model.Contributor{
		{Author: "alice", Email: "a@x.com", Count: 10},
		{Author: "bob", Email: "b@x.com", Count: 3},
	}
	lineStats := []model.Contributor{
		{Author: "alice", Email: "a@x.com", Insertions: 30, Deletions: 20},
		// bob 无行数记录（防御场景）保持 0；charlie 不在 contributors 中被忽略
		{Author: "charlie", Email: "c@x.com", Insertions: 999, Deletions: 999},
	}
	merged := MergeContributorLineStats(contributors, lineStats)
	if len(merged) != 2 {
		t.Fatalf("合并不应增删贡献者: got %d", len(merged))
	}
	if merged[0].Insertions != 30 || merged[0].Deletions != 20 {
		t.Errorf("alice 行数应被填充: got %+v", merged[0])
	}
	if merged[0].Count != 10 {
		t.Errorf("提交数不应被合并破坏: got %d", merged[0].Count)
	}
	if merged[1].Insertions != 0 || merged[1].Deletions != 0 {
		t.Errorf("无行数记录的贡献者应保持 0: got %+v", merged[1])
	}
}

func TestStatsRangeWindow(t *testing.T) {
	// 固定基准时刻：2026-09-23 12:00:00 本地
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.Local)

	cases := []struct {
		rangeKey string
		wantYear int
		wantMon  time.Month
		wantDay  int
	}{
		{"7d", 2026, 9, 16},
		{"30d", 2026, 8, 24},
		{"90d", 2026, 6, 25},
		{"1y", 2025, 9, 23},
	}
	for _, c := range cases {
		sinceTs, untilTs := StatsRangeWindow(c.rangeKey, now)
		got := time.Unix(sinceTs, 0)
		if got.Year() != c.wantYear || got.Month() != c.wantMon || got.Day() != c.wantDay {
			t.Errorf("档位 %s 下界异常: got %s, want %d-%02d-%02d", c.rangeKey, got, c.wantYear, c.wantMon, c.wantDay)
		}
		if untilTs != now.Unix() {
			t.Errorf("档位 %s 上界应为 now", c.rangeKey)
		}
	}
	// all 档不限下界
	sinceTs, _ := StatsRangeWindow("all", now)
	if sinceTs != 0 {
		t.Errorf("all 档 sinceTs 应为 0: got %d", sinceTs)
	}
	// 未知档位回退 30d（与 AggregateRepoStats 兜底一致）
	sinceUnknown, _ := StatsRangeWindow("bogus", now)
	since30d, _ := StatsRangeWindow("30d", now)
	if sinceUnknown != since30d {
		t.Errorf("未知档位应回退 30d: got %d want %d", sinceUnknown, since30d)
	}
}
