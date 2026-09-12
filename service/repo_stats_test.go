package service

import (
	"fmt"
	"testing"
	"time"

	"workbench/model"
)

// statsNow 测试基准时刻：2026-09-12 12:00 本地。固定避免依赖 time.Now。
var statsNow = time.Date(2026, 9, 12, 12, 0, 0, 0, time.Local)

// statsCommitSeq 测试提交序号，保证 SHA 唯一（统计聚合不依赖 SHA，但避免未来去重逻辑误判）。
var statsCommitSeq int

// makeCommit 构造测试用提交：作者 + 距基准时刻 daysAgo 天前提交。
func makeCommit(author, email string, daysAgo int) model.Commit {
	statsCommitSeq++
	t := statsNow.AddDate(0, 0, -daysAgo)
	return model.Commit{
		SHA:       fmt.Sprintf("%s-%s-%d-%d", author, email, daysAgo, statsCommitSeq),
		Author:    author,
		Email:     email,
		Timestamp: t.Unix(),
		DateTime:  t.Format("2006-01-02 15:04:05"),
		Message:   "test commit",
	}
}

// statsCommits 测试数据集：Alice 今天3 + 昨天1，Bob 昨天1 + 30天前1。共5条在7d窗口内。
func statsCommits() []model.Commit {
	return []model.Commit{
		makeCommit("Alice", "a@x.com", 0),
		makeCommit("Alice", "a@x.com", 0),
		makeCommit("Alice", "a@x.com", 0),
		makeCommit("Alice", "a@x.com", 1),
		makeCommit("Bob", "b@x.com", 1),
		makeCommit("Bob", "b@x.com", 30),
	}
}

func TestAggregateRepoStats_TrendDayBuckets(t *testing.T) {
	stats := AggregateRepoStats(statsCommits(), string(StatsRange7Days), statsNow)

	if stats.Granularity != string(GranularityDay) {
		t.Errorf("7d 粒度应为 day: got %s", stats.Granularity)
	}
	// 7d 窗口含今天(3)+昨天(2)，30天前的 Bob 被过滤
	if stats.TotalCommits != 5 {
		t.Errorf("7d 窗口提交数应为 5: got %d", stats.TotalCommits)
	}
	if len(stats.Trend) != 2 {
		t.Fatalf("7d 趋势桶数应为 2(今天+昨天): got %d", len(stats.Trend))
	}
	// 升序：昨天在前，今天在后
	yesterday := statsNow.AddDate(0, 0, -1).Format("2006-01-02")
	today := statsNow.Format("2006-01-02")
	if stats.Trend[0].Date != yesterday || stats.Trend[0].Count != 2 {
		t.Errorf("首桶应为昨天 count=2: got %s %d", stats.Trend[0].Date, stats.Trend[0].Count)
	}
	if stats.Trend[1].Date != today || stats.Trend[1].Count != 3 {
		t.Errorf("次桶应为今天 count=3: got %s %d", stats.Trend[1].Date, stats.Trend[1].Count)
	}
}

func TestAggregateRepoStats_TrendWeekBuckets(t *testing.T) {
	stats := AggregateRepoStats(statsCommits(), string(StatsRange90Days), statsNow)
	if stats.Granularity != string(GranularityWeek) {
		t.Errorf("90d 粒度应为 week: got %s", stats.Granularity)
	}
	// 90d 窗口含全部6条（30天前在内），按周分桶：今天+昨天同周，30天前另一周
	if stats.TotalCommits != 6 {
		t.Errorf("90d 窗口提交数应为 6: got %d", stats.TotalCommits)
	}
	// 周桶日期应为各自周一
	for _, b := range stats.Trend {
		day, _ := time.ParseInLocation("2006-01-02", b.Date, time.Local)
		if int(day.Weekday()) != 1 {
			t.Errorf("周桶日期应为周一: got %s weekday=%d", b.Date, day.Weekday())
		}
	}
}

func TestAggregateRepoStats_TrendMonthBucketsAllRange(t *testing.T) {
	stats := AggregateRepoStats(statsCommits(), string(StatsRangeAll), statsNow)
	if stats.Granularity != string(GranularityMonth) {
		t.Errorf("all 粒度应为 month: got %s", stats.Granularity)
	}
	if stats.TotalCommits != 6 {
		t.Errorf("all 窗口提交数应为 6(不限): got %d", stats.TotalCommits)
	}
	// 9月(今天+昨天5条) + 8月(30天前1条)
	if len(stats.Trend) != 2 {
		t.Fatalf("all 月桶数应为 2(8月+9月): got %d", len(stats.Trend))
	}
	if stats.Trend[0].Date != "2026-08" || stats.Trend[0].Count != 1 {
		t.Errorf("首月桶应为 2026-08 count=1: got %s %d", stats.Trend[0].Date, stats.Trend[0].Count)
	}
	if stats.Trend[1].Date != "2026-09" || stats.Trend[1].Count != 5 {
		t.Errorf("次月桶应为 2026-09 count=5: got %s %d", stats.Trend[1].Date, stats.Trend[1].Count)
	}
}

func TestAggregateRepoStats_ContributorsRanking(t *testing.T) {
	stats := AggregateRepoStats(statsCommits(), string(StatsRange30Days), statsNow)
	// 30d 窗口含 Alice 4 + Bob 1（30天前的 Bob 刚好在边界，since=now-30d，ts=now-30d 当日）
	// Alice 排首
	if len(stats.Contributors) < 2 {
		t.Fatalf("贡献者应至少 2 个: got %d", len(stats.Contributors))
	}
	if stats.Contributors[0].Author != "Alice" || stats.Contributors[0].Count != 4 {
		t.Errorf("首位应为 Alice count=4: got %+v", stats.Contributors[0])
	}
}

func TestAggregateRepoStats_ContributorsSameNameDifferentEmail(t *testing.T) {
	// 同名不同邮箱视为不同贡献者
	commits := []model.Commit{
		makeCommit("Dev", "a@x.com", 0),
		makeCommit("Dev", "b@x.com", 0),
	}
	stats := AggregateRepoStats(commits, string(StatsRange7Days), statsNow)
	if len(stats.Contributors) != 2 {
		t.Errorf("同名不同邮箱应视为 2 个贡献者: got %d", len(stats.Contributors))
	}
}

func TestAggregateRepoStats_HeatmapFullYear(t *testing.T) {
	stats := AggregateRepoStats(statsCommits(), string(StatsRange7Days), statsNow)
	if len(stats.Heatmap) != heatmapDays {
		t.Fatalf("热力图长度应为 %d: got %d", heatmapDays, len(stats.Heatmap))
	}
	// 末尾为今天，count=3
	last := stats.Heatmap[len(stats.Heatmap)-1]
	today := statsNow.Format("2006-01-02")
	if last.Date != today {
		t.Errorf("热力图末尾应为今天 %s: got %s", today, last.Date)
	}
	if last.Count != 3 {
		t.Errorf("今天热力 count 应为 3: got %d", last.Count)
	}
	// 首尾日期差 heatmapDays-1 天
	first, _ := time.ParseInLocation("2006-01-02", stats.Heatmap[0].Date, time.Local)
	lastT, _ := time.ParseInLocation("2006-01-02", last.Date, time.Local)
	if int(lastT.Sub(first).Hours()/24) != heatmapDays-1 {
		t.Errorf("热力图首尾应跨 %d 天: got %d", heatmapDays-1, int(lastT.Sub(first).Hours()/24))
	}
}

func TestAggregateRepoStats_HeatmapZeroDaysFilled(t *testing.T) {
	// 仅1条提交，热力图应补364个零日
	commits := []model.Commit{makeCommit("Solo", "s@x.com", 0)}
	stats := AggregateRepoStats(commits, string(StatsRange7Days), statsNow)
	zeroCount := 0
	for _, d := range stats.Heatmap {
		if d.Count == 0 {
			zeroCount++
		}
	}
	if zeroCount != heatmapDays-1 {
		t.Errorf("零日应为 %d: got %d", heatmapDays-1, zeroCount)
	}
}

func TestAggregateRepoStats_WindowFiltersOutOldCommits(t *testing.T) {
	// 7d 窗口应排除 30 天前的提交
	stats := AggregateRepoStats(statsCommits(), string(StatsRange7Days), statsNow)
	for _, c := range stats.Contributors {
		if c.Author == "Bob" && c.Count > 1 {
			t.Errorf("7d 窗口 Bob 应仅1条(30天前被过滤): got %d", c.Count)
		}
	}
}

func TestAggregateRepoStats_UnknownRangeFallsBackTo30d(t *testing.T) {
	stats := AggregateRepoStats(statsCommits(), "bogus", statsNow)
	if stats.DateRange != rangeSpecs[StatsRange30Days].label {
		t.Errorf("未知档位应回退 30d 描述: got %s", stats.DateRange)
	}
	if stats.Granularity != string(GranularityDay) {
		t.Errorf("回退 30d 粒度应为 day: got %s", stats.Granularity)
	}
}

func TestAggregateRepoStats_EmptyCommits(t *testing.T) {
	stats := AggregateRepoStats(nil, string(StatsRange7Days), statsNow)
	if stats.TotalCommits != 0 {
		t.Errorf("空提交集 TotalCommits 应为 0: got %d", stats.TotalCommits)
	}
	if len(stats.Trend) != 0 {
		t.Errorf("空提交集 Trend 应为空: got %d", len(stats.Trend))
	}
	if len(stats.Contributors) != 0 {
		t.Errorf("空提交集 Contributors 应为空: got %d", len(stats.Contributors))
	}
	// 热力图仍补全 365 个零日
	if len(stats.Heatmap) != heatmapDays {
		t.Errorf("空提交热力图仍应 %d 天: got %d", heatmapDays, len(stats.Heatmap))
	}
}

func TestAggregateRepoStats_SampledDefaultFalse(t *testing.T) {
	// AggregateRepoStats 不设 Sampled，由 App 层定；默认应为 false
	stats := AggregateRepoStats(statsCommits(), string(StatsRange7Days), statsNow)
	if stats.Sampled {
		t.Error("纯聚合 Sampled 应默认 false")
	}
}

func TestAggregateRepoStats_DateRangeAndGranularity(t *testing.T) {
	cases := []struct {
		rangeKey    string
		wantLabel   string
		wantGran    string
	}{
		{string(StatsRange7Days), "最近 7 天", string(GranularityDay)},
		{string(StatsRange30Days), "最近 30 天", string(GranularityDay)},
		{string(StatsRange90Days), "最近 90 天", string(GranularityWeek)},
		{string(StatsRange1Year), "最近 1 年", string(GranularityWeek)},
		{string(StatsRangeAll), "全部历史", string(GranularityMonth)},
	}
	for _, c := range cases {
		stats := AggregateRepoStats(statsCommits(), c.rangeKey, statsNow)
		if stats.DateRange != c.wantLabel || stats.Granularity != c.wantGran {
			t.Errorf("档位 %s: want %s/%s, got %s/%s", c.rangeKey, c.wantLabel, c.wantGran, stats.DateRange, stats.Granularity)
		}
	}
}

func TestTimeBucketKey(t *testing.T) {
	// 2026-09-12 是周六；周一应为 2026-09-07
	sat := time.Date(2026, 9, 12, 3, 0, 0, 0, time.Local)
	if got := timeBucketKey(sat, GranularityWeek); got != "2026-09-07" {
		t.Errorf("周六的周桶应为本周一 2026-09-07: got %s", got)
	}
	// 周一当天
	mon := time.Date(2026, 9, 7, 3, 0, 0, 0, time.Local)
	if got := timeBucketKey(mon, GranularityWeek); got != "2026-09-07" {
		t.Errorf("周一的周桶应为自身 2026-09-07: got %s", got)
	}
	// 周日（Sunday=0）应归到上周一
	sun := time.Date(2026, 9, 13, 3, 0, 0, 0, time.Local)
	if got := timeBucketKey(sun, GranularityWeek); got != "2026-09-07" {
		t.Errorf("周日的周桶应为上周一 2026-09-07: got %s", got)
	}
	// 日粒度
	if got := timeBucketKey(sat, GranularityDay); got != "2026-09-12" {
		t.Errorf("日桶应为 2026-09-12: got %s", got)
	}
	// 月粒度
	if got := timeBucketKey(sat, GranularityMonth); got != "2026-09" {
		t.Errorf("月桶应为 2026-09: got %s", got)
	}
}

func TestValidateStatsRange(t *testing.T) {
	valid := []string{"7d", "30d", "90d", "1y", "all"}
	for _, r := range valid {
		if err := ValidateStatsRange(r); err != nil {
			t.Errorf("合法档位 %s 不应报错: %v", r, err)
		}
	}
	if err := ValidateStatsRange("bogus"); err == nil {
		t.Error("非法档位应报错")
	}
}

func TestFilterCommitsByTime(t *testing.T) {
	commits := statsCommits()
	// 窗口 [now-7d, now]，应含5条（排除30天前）
	got := filterCommitsByTime(commits, statsNow.Add(-7*24*time.Hour).Unix(), statsNow.Unix())
	if len(got) != 5 {
		t.Errorf("7d 窗口应含5条: got %d", len(got))
	}
	// since=0 不限下界，应含全部6条
	got = filterCommitsByTime(commits, 0, statsNow.Unix())
	if len(got) != 6 {
		t.Errorf("不限下界应含6条: got %d", len(got))
	}
}

func TestFilterCommitsByTime_UntilBoundary(t *testing.T) {
	// 未来提交（daysAgo=-1）应被 untilTs 过滤掉
	commits := []model.Commit{
		makeCommit("Future", "f@x.com", -1),
		makeCommit("Past", "p@x.com", 1),
	}
	got := filterCommitsByTime(commits, 0, statsNow.Unix())
	if len(got) != 1 || got[0].Author != "Past" {
		t.Errorf("untilTs 应过滤未来提交仅留 Past: got %+v", got)
	}
}

func TestTimeBucketKey_DefaultFallback(t *testing.T) {
	// 未知粒度回退 day
	tm := time.Date(2026, 9, 12, 0, 0, 0, 0, time.Local)
	if got := timeBucketKey(tm, StatsGranularity("bogus")); got != "2026-09-12" {
		t.Errorf("未知粒度应回退 day 桶 2026-09-12: got %s", got)
	}
}
