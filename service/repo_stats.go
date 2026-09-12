package service

import (
	"fmt"
	"sort"
	"time"

	"workbench/model"
)

// ===== 仓库提交统计聚合（纯函数，复用 commit_history_cache 全量快照） =====
//
// 统计不依赖 go-git 仓库对象，仅接收 []model.Commit 做内存聚合，便于单测注入任意提交集。
// 时间窗口档位固定（7d/30d/90d/1y/all），聚合粒度随档位自动适配，避免用户选粒度与窗口冲突。
// 热力图独立于 Trend 粒度，始终按日展示最近一年，提供活跃度总览。

// StatsRange 时间窗口档位标识，前端档位切换器传入。
type StatsRange string

const (
	StatsRange7Days  StatsRange = "7d"
	StatsRange30Days StatsRange = "30d"
	StatsRange90Days StatsRange = "90d"
	StatsRange1Year  StatsRange = "1y"
	StatsRangeAll    StatsRange = "all"
)

// StatsGranularity Trend 序列聚合粒度。
type StatsGranularity string

const (
	GranularityDay   StatsGranularity = "day"
	GranularityWeek  StatsGranularity = "week"
	GranularityMonth StatsGranularity = "month"
)

// heatmapDays 热力图固定展示天数（最近一年含当日）。
const heatmapDays = 365

// rangeSpec 时间窗口档位规格：按日历日/日历年回退（AddDate，DST 安全、闰年准确）、Trend 粒度、可读描述。
type rangeSpec struct {
	// sinceDays 按日历日回退的天数（AddDate(0,0,-sinceDays)）；0 表示不走日回退。
	sinceDays int
	// sinceYears 按日历年回退的年数（AddDate(-sinceYears,0,0)），仅 1y 档用，闰年自动取 365/366。
	sinceYears int
	granularity StatsGranularity
	label      string
}

// rangeSpecs 档位规格表。7d/30d 按日粒度可见每日波动；90d/1y 按周粒度避免桶过多；
// all 按月粒度覆盖长跨度仓库。粒度与窗口匹配，杜绝「7 天选月粒度」等无意义组合。
var rangeSpecs = map[StatsRange]rangeSpec{
	StatsRange7Days:  {sinceDays: 7, granularity: GranularityDay, label: "最近 7 天"},
	StatsRange30Days: {sinceDays: 30, granularity: GranularityDay, label: "最近 30 天"},
	StatsRange90Days: {sinceDays: 90, granularity: GranularityWeek, label: "最近 90 天"},
	StatsRange1Year:  {sinceYears: 1, granularity: GranularityWeek, label: "最近 1 年"},
	StatsRangeAll:    {granularity: GranularityMonth, label: "全部历史"},
}

// AggregateRepoStats 基于全量提交快照聚合仓库统计。
//
// commits 为 commit_history_cache 解析的全量快照（按 committer time 倒序），now 为当前
// 时刻（由调用方传入，避免 service 依赖 time.Now 致测试不可控）。rangeKey 控制时间窗口
// 与 Trend 粒度；Heatmap 始终按日展示最近 heatmapDays 天。
//
// 超限仓库（>5000 commit）调用方走 go-git 全量扫传入同结构 commits，聚合逻辑无差异。
func AggregateRepoStats(commits []model.Commit, rangeKey string, now time.Time) model.RepoStats {
	spec, ok := rangeSpecs[StatsRange(rangeKey)]
	if !ok {
		// 未知档位回退 30d，避免空数据；调用方应已校验，此为兜底
		spec = rangeSpecs[StatsRange30Days]
	}

	stats := model.RepoStats{
		DateRange:   spec.label,
		Granularity: string(spec.granularity),
	}

	// 时间窗口下界（AddDate 按日历日/年回退，DST 安全、闰年准确）：all 不限；1y 按日历年；
	// 其余按日历日。untilTs 取 now（窗口上界含 now）。
	var sinceTs int64
	if spec.sinceYears > 0 {
		sinceTs = now.AddDate(-spec.sinceYears, 0, 0).Unix()
	} else if spec.sinceDays > 0 {
		sinceTs = now.AddDate(0, 0, -spec.sinceDays).Unix()
	}

	// Trend + Contributors 在窗口内提交上聚合；Heatmap 独立按日最近一年
	windowed := filterCommitsByTime(commits, sinceTs, now.Unix())

	stats.Trend = aggregateTrend(windowed, spec.granularity)
	stats.Contributors = aggregateContributors(windowed)
	stats.TotalCommits = len(windowed)
	stats.Heatmap = aggregateHeatmap(commits, now, heatmapDays)

	return stats
}

// filterCommitsByTime 过滤时间窗口内的提交。sinceTs=0 表示不限下界。返回新切片，不修改入参。
func filterCommitsByTime(commits []model.Commit, sinceTs, untilTs int64) []model.Commit {
	out := make([]model.Commit, 0, len(commits))
	for _, c := range commits {
		if sinceTs > 0 && c.Timestamp < sinceTs {
			continue
		}
		if untilTs > 0 && c.Timestamp > untilTs {
			continue
		}
		out = append(out, c)
	}
	return out
}

// aggregateTrend 按粒度分桶计数。桶按时间升序返回，空窗口（桶存在但无提交）不在序列中——
// 趋势图省略空桶避免密集零值噪声；前端 X 轴按实际桶日期渲染。
func aggregateTrend(commits []model.Commit, g StatsGranularity) []model.TimeBucket {
	buckets := make(map[string]int)
	for _, c := range commits {
		buckets[timeBucketKey(time.Unix(c.Timestamp, 0), g)]++
	}

	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys) // YYYY-MM-DD / YYYY-MM 字典序即时间序

	out := make([]model.TimeBucket, 0, len(keys))
	for _, k := range keys {
		out = append(out, model.TimeBucket{Date: k, Count: buckets[k]})
	}
	return out
}

// timeBucketKey 计算提交时刻所属粒度桶的标识键。
//   - day:   YYYY-MM-DD
//   - week:  本周一的 YYYY-MM-DD（ISO 周一为周首日，统一周归属）
//   - month: YYYY-MM
func timeBucketKey(t time.Time, g StatsGranularity) string {
	switch g {
	case GranularityDay:
		return t.Format("2006-01-02")
	case GranularityWeek:
		// 周一为 0 偏移：Sunday=0 → +6 mod 7 得周一偏移
		mondayOffset := (int(t.Weekday()) + 6) % 7
		return t.AddDate(0, 0, -mondayOffset).Format("2006-01-02")
	case GranularityMonth:
		return t.Format("2006-01")
	default:
		return t.Format("2006-01-02")
	}
}

// aggregateContributors 按作者名称+邮箱归一分组计数，按 Count 降序排列。
// 同名不同邮箱或同邮箱不同名视为不同贡献者，与 git log 作者身份一致。
func aggregateContributors(commits []model.Commit) []model.Contributor {
	type key struct {
		author, email string
	}
	counts := make(map[key]int)
	for _, c := range commits {
		counts[key{c.Author, c.Email}]++
	}

	out := make([]model.Contributor, 0, len(counts))
	for k, c := range counts {
		out = append(out, model.Contributor{Author: k.author, Email: k.email, Count: c})
	}
	// 降序；同 Count 按作者名升序稳定排列
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Author < out[j].Author
	})
	return out
}

// aggregateHeatmap 按日聚合最近 days 天的提交数（含 0 提交日，热力图需连续日期序列）。
// 从结束日往前逐日生成桶，保证序列按日期升序且无缺日。
// commits 按 committer time 倒序，遍历到 Timestamp < 窗口下界即 break——早于 days 天的
// 提交日计数写入 map 后从不被读取，提前终止避免多年仓库白扫。
func aggregateHeatmap(commits []model.Commit, now time.Time, days int) []model.DayCount {
	// 窗口下界：now 往前 days-1 天的当日 00:00（与输出序列首日对齐）
	heatmapSince := truncateToLocalDay(now).AddDate(0, 0, -(days - 1)).Unix()

	// 提交集按日计数
	counts := make(map[string]int)
	for _, c := range commits {
		if c.Timestamp < heatmapSince {
			break // 倒序，后续皆更旧，提前终止
		}
		counts[time.Unix(c.Timestamp, 0).Format("2006-01-02")]++
	}

	// 生成 [now-(days-1), now] 连续日期序列，按日步进
	out := make([]model.DayCount, 0, days)
	end := truncateToLocalDay(now)
	for i := days - 1; i >= 0; i-- {
		day := end.AddDate(0, 0, -i)
		key := day.Format("2006-01-02")
		out = append(out, model.DayCount{Date: key, Count: counts[key]})
	}
	return out
}

// truncateToLocalDay 截断到当日 00:00:00 本地时刻，作为热力图日期序列的基准。
func truncateToLocalDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// ValidateStatsRange 校验档位是否合法，非法返错误。
func ValidateStatsRange(rangeKey string) error {
	if _, ok := rangeSpecs[StatsRange(rangeKey)]; !ok {
		return fmt.Errorf("未知统计时间档位: %s", rangeKey)
	}
	return nil
}
