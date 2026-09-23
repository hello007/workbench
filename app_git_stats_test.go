package main

import (
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"workbench/model"
	"workbench/util"
)

// TestGetRepoStats_CacheHitReturnsAggregated 缓存命中路径：注入缓存的 App 首次请求触发全量扫
// 并回写缓存，统计基于全量快照，Sampled=false，热力图补满一年。
func TestGetRepoStats_CacheHitReturnsAggregated(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 5)
	app := newAppWithCommitCache()

	stats, err := app.GetRepoStats(repoPath, "7d")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if stats.TotalCommits != 5 {
		t.Errorf("TotalCommits 应为 5: got %d", stats.TotalCommits)
	}
	if stats.Sampled {
		t.Error("缓存命中路径 Sampled 应为 false")
	}
	if stats.Granularity != "day" {
		t.Errorf("7d 粒度应为 day: got %s", stats.Granularity)
	}
	if stats.DateRange != "最近 7 天" {
		t.Errorf("DateRange 应为 最近 7 天: got %s", stats.DateRange)
	}
	if len(stats.Heatmap) != 365 {
		t.Errorf("热力图应 365 天: got %d", len(stats.Heatmap))
	}
	// 单作者 Test，首位应为 Test count=5
	if len(stats.Contributors) != 1 || stats.Contributors[0].Author != "Test" || stats.Contributors[0].Count != 5 {
		t.Errorf("贡献者应为 Test count=5: got %+v", stats.Contributors)
	}
}

// TestGetRepoStats_SecondCallHitsCache 二次请求命中缓存（不重扫），结果与首次一致。
func TestGetRepoStats_SecondCallHitsCache(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 4)
	app := newAppWithCommitCache()

	first, err := app.GetRepoStats(repoPath, "30d")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := app.GetRepoStats(repoPath, "30d")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.TotalCommits != second.TotalCommits {
		t.Errorf("二次请求 TotalCommits 应一致: %d vs %d", first.TotalCommits, second.TotalCommits)
	}
	if first.TotalCommits != 4 {
		t.Errorf("TotalCommits 应为 4: got %d", first.TotalCommits)
	}
}

// TestGetRepoStats_NilCacheFallsBackToFullScan cache 未注入（App{} 未经 startup）走全量扫，
// 仍返聚合结果，Sampled=false（5 条未超限）。
func TestGetRepoStats_NilCacheFallsBackToFullScan(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 3)
	app := NewApp() // 不注入 commitHistoryCache

	stats, err := app.GetRepoStats(repoPath, "all")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if stats.TotalCommits != 3 {
		t.Errorf("TotalCommits 应为 3: got %d", stats.TotalCommits)
	}
	if stats.Sampled {
		t.Error("未超限全量扫 Sampled 应为 false")
	}
	if stats.Granularity != "month" {
		t.Errorf("all 粒度应为 month: got %s", stats.Granularity)
	}
}

// TestGetRepoStats_EmptyPathError 空路径应报错。
func TestGetRepoStats_EmptyPathError(t *testing.T) {
	app := newAppWithCommitCache()
	if _, err := app.GetRepoStats("", "7d"); err == nil {
		t.Error("空路径应报错")
	}
}

// TestGetRepoStats_InvalidRangeError 非法档位应报错。
func TestGetRepoStats_InvalidRangeError(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 2)
	app := newAppWithCommitCache()
	if _, err := app.GetRepoStats(repoPath, "bogus"); err == nil {
		t.Error("非法档位应报错")
	}
}

// TestGetRepoStats_AllRangeGranularity 各档位粒度映射正确。
func TestGetRepoStats_AllRangeGranularity(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 2)
	app := newAppWithCommitCache()

	cases := []struct {
		rangeKey string
		gran     string
	}{
		{"7d", "day"},
		{"30d", "day"},
		{"90d", "week"},
		{"1y", "week"},
		{"all", "month"},
	}
	for _, c := range cases {
		stats, err := app.GetRepoStats(repoPath, c.rangeKey)
		if err != nil {
			t.Errorf("档位 %s 不应报错: %v", c.rangeKey, err)
			continue
		}
		if stats.Granularity != c.gran {
			t.Errorf("档位 %s 粒度应为 %s: got %s", c.rangeKey, c.gran, stats.Granularity)
		}
	}
}

// TestFullScanCommits_NormalRepoNotOverflow 正常仓库（<5000）走 cache==nil 全量扫路径，Sampled=false。
func TestFullScanCommits_NormalRepoNotOverflow(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 4)
	app := newAppWithCommitCache()
	app.commitHistoryCache = nil // 走 fullScanCommits 全量扫路径

	stats, err := app.GetRepoStats(repoPath, "all")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if stats.Sampled {
		t.Error("4 条提交未超限，Sampled 应为 false")
	}
	if stats.TotalCommits != 4 {
		t.Errorf("TotalCommits 应为 4: got %d", stats.TotalCommits)
	}
}

// TestGetRepoStats_NonGitPathError 非 git 路径应报错（验不静默返空统计）。
func TestGetRepoStats_NonGitPathError(t *testing.T) {
	app := newAppWithCommitCache()
	if _, err := app.GetRepoStats(t.TempDir(), "7d"); err == nil {
		t.Error("非 git 路径应报错，不静默返空统计")
	}
}

// ===== 贡献者行数排名（numstat 通道） =====

// currentHeadInfo 取 fixture 仓库的 gitRoot/head/key，供 app 层缓存预填测试。
func currentHeadInfo(t *testing.T, repoPath string) (gitRoot, key, sha string) {
	t.Helper()
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return gitRoot, commitHistoryCacheKey(gitRoot, head), head.Hash().String()
}

// TestGetRepoStats_LineStatsFilled 行数通道端到端：5 提交各 1 行新增（"content N"），
// Contributors 应填充 Insertions=5 / Deletions=0（与提交数 Count=5 同作者）。
func TestGetRepoStats_LineStatsFilled(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 5)
	app := newAppWithCommitCache()

	stats, err := app.GetRepoStats(repoPath, "all")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if len(stats.Contributors) != 1 {
		t.Fatalf("应 1 位贡献者: got %d", len(stats.Contributors))
	}
	c := stats.Contributors[0]
	if c.Count != 5 {
		t.Errorf("Count 应为 5: got %d", c.Count)
	}
	if c.Insertions != 5 || c.Deletions != 0 {
		t.Errorf("行数应为 ins=5 del=0: got ins=%d del=%d", c.Insertions, c.Deletions)
	}
}

// TestGetRepoStats_LineStatsCacheHitNotRefetched 缓存命中不重扫：预填 numstat 缓存
// （headSHA=当前 HEAD，Insertions=999 假数据，与真实仓库 ins=5 可区分），GetRepoStats
// 返回假数据即证明命中缓存、未执行 numstat 重扫。
func TestGetRepoStats_LineStatsCacheHitNotRefetched(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 3)
	app := newAppWithCommitCache()
	_, key, sha := currentHeadInfo(t, repoPath)
	app.numstatLineCache.Set(key, sha, []model.CommitLineStat{{
		SHA: sha, Author: "Test", Email: "test@test.com",
		Timestamp: 1, Insertions: 999, Deletions: 7,
	}})

	stats, err := app.GetRepoStats(repoPath, "all")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if len(stats.Contributors) != 1 {
		t.Fatalf("应 1 位贡献者: got %d", len(stats.Contributors))
	}
	c := stats.Contributors[0]
	if c.Insertions != 999 || c.Deletions != 7 {
		t.Errorf("应命中预填缓存（不重扫）: got ins=%d del=%d, want ins=999 del=7", c.Insertions, c.Deletions)
	}
}

// TestGetRepoStats_LineStatsStaleSHAReloads headSHA 不同触发全量重拉：预填 stale
// SHA 的假数据应被丢弃，返回真实仓库行数（行数是聚合值无增量路径，二态失效）。
func TestGetRepoStats_LineStatsStaleSHAReloads(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 3)
	app := newAppWithCommitCache()
	_, key, _ := currentHeadInfo(t, repoPath)
	app.numstatLineCache.Set(key, "stale-sha", []model.CommitLineStat{{
		SHA: "stale-sha", Author: "Test", Email: "test@test.com",
		Timestamp: 1, Insertions: 999, Deletions: 7,
	}})

	stats, err := app.GetRepoStats(repoPath, "all")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	c := stats.Contributors[0]
	if c.Insertions != 3 {
		t.Errorf("stale SHA 应重拉真实数据 ins=3: got %d", c.Insertions)
	}
}

// TestGetRepoStats_LineStatsWindowFilter 行数窗口过滤与提交数口径一致：all 档含全部；
// 行数记录的 Timestamp（%at）与提交 Timestamp 同源，切档不重扫（全量落缓存）。
func TestGetRepoStats_LineStatsWindowFilter(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 4)
	app := newAppWithCommitCache()

	stats, err := app.GetRepoStats(repoPath, "all")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if stats.Contributors[0].Insertions != 4 {
		t.Errorf("all 档行数应含全部 4 提交: got %d", stats.Contributors[0].Insertions)
	}
	// 7d 档同样应命中（fixture 提交时间为当前时刻，窗口内）；窗口过滤实现在
	// service.AggregateLineStats 单测覆盖，此处验证编排链路窗口参数传递不丢行数
	stats7d, err := app.GetRepoStats(repoPath, "7d")
	if err != nil {
		t.Fatalf("GetRepoStats 7d: %v", err)
	}
	if stats7d.Contributors[0].Insertions != 4 {
		t.Errorf("7d 档 fixture 提交在窗口内应含全部行数: got %d", stats7d.Contributors[0].Insertions)
	}
}

// TestInvalidateCommitHistoryCache_AlsoClearsLineCache 手动刷新联动：清提交历史缓存
// 时同步清行数缓存（handleRefresh 后两路都全量重扫）。
func TestInvalidateCommitHistoryCache_AlsoClearsLineCache(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 2)
	app := newAppWithCommitCache()
	_, key, sha := currentHeadInfo(t, repoPath)
	app.numstatLineCache.Set(key, sha, sampleLineStatsForInvalidate())

	app.InvalidateCommitHistoryCache(repoPath)

	if _, ok := app.numstatLineCache.Get(key, sha); ok {
		t.Error("手动刷新应同步清除行数缓存")
	}
}

func sampleLineStatsForInvalidate() []model.CommitLineStat {
	return []model.CommitLineStat{{
		SHA: strings.Repeat("a", 40), Author: "Test", Email: "test@test.com", Timestamp: 1,
	}}
}

// TestResolveLineStats_NilCacheDirectFetch numstatLineCache 未注入（测试 App{} 未经
// startup）时直接现拉，对齐 commitHistoryCache==nil 分支模式，结果正常可用。
func TestResolveLineStats_NilCacheDirectFetch(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 2)
	app := NewApp() // numstatLineCache 为 nil
	gitRoot, key, sha := currentHeadInfo(t, repoPath)

	records, err := app.resolveLineStats(gitRoot, key, sha)
	if err != nil {
		t.Fatalf("resolveLineStats: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("nil 缓存应现拉 2 条记录: got %d", len(records))
	}
}
