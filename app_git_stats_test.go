package main

import (
	"testing"
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
