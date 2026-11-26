package service

import (
	"fmt"
	"sync"
	"time"

	"workbench/model"
)

// overviewSnapshotTTL 概览快照缓存有效期：5 分钟内的切走再切回不重复计算。
//
// 仓库域每个 pin 仓库要跑多条 git 子进程，是快照的主要成本；AI 历史与待办
// 为内存/JSON 读取。TTL 判定只在后端，前端不自行计时。
const overviewSnapshotTTL = 5 * time.Minute

// OverviewService 个人看板「概览」面板的只读聚合服务。
//
// 聚合三个既有数据源，零新增采集层、零 LLM 调用：
//   - M1/M2 ← DashboardService.GetStatuses（pin 仓库状态快照）
//   - M3    ← TempTodoService.List（未完成待办）
//   - M4/M5 ← AiFunctionService.GetAiTaskHistoryStats（与 AI 历史面板同源聚合）
//
// 分区降级：三个域各自把失败写进对应 Error 字段，不阻断其余域，也绝不把
// 查询失败静默降级为 0。指针依赖在 NewAppServices 中注入，构造纯 new 无副作用。
type OverviewService struct {
	dashboard *DashboardService
	tempTodo  *TempTodoService
	aiFunc    *AiFunctionService
	settings  *SettingsService

	// now 时钟注入点，测试固定「今日/本周」窗口边界。
	now func() time.Time

	mu     sync.Mutex
	cached *model.OverviewSnapshot
	// cachedRanges 保存一次 build 算出的全部五档 AI 统计；命中缓存时按入参档位
	// 回填 Selected，切档不触发任何域重算（仓库域 git 子进程是主要成本）。
	cachedRanges map[string]model.AIRangeStats
	cachedAt     time.Time
}

// NewOverviewService 创建概览聚合服务。
// 依赖必须在 AppServices 中先于本服务构造（dashboard/aiFunc/tempTodo/settings）。
func NewOverviewService(
	dashboard *DashboardService,
	tempTodo *TempTodoService,
	aiFunc *AiFunctionService,
	settings *SettingsService,
) *OverviewService {
	return &OverviewService{
		dashboard: dashboard,
		tempTodo:  tempTodo,
		aiFunc:    aiFunc,
		settings:  settings,
		now:       time.Now,
	}
}

// GetSnapshot 返回概览快照。force=false 且缓存未过期时直接复用；
// force=true 跳过 TTL 强制重算（用户点刷新）。
//
// rangeKey 为所选时间档位（today/week/7d/30d/90d，空值/非法值归一为 week）。
// 缓存按「一次快照 + 五档 AI 统计」存放：命中时浅拷贝快照并按入参档位回填
// Range / AI.RangeKey / AI.Selected，不重算任何域——切档因此只换返回块。
// AI 域失败态不参与缓存命中：失败快照每次请求重算，切档即可恢复；成功态仍走 5 分钟 TTL。
func (s *OverviewService) GetSnapshot(rangeKey string, force bool) *model.OverviewSnapshot {
	now := s.now()
	key := normalizeOverviewRange(rangeKey)

	s.mu.Lock()
	// 失败快照（AI.Error 非空）不复用：否则 TTL 内切档一直拿零值 Selected，
	// 数据恢复后还得手动点重试（force=true）才能走出降级态。
	if !force && s.cached != nil && s.cached.AI.Error == "" && now.Sub(s.cachedAt) < overviewSnapshotTTL {
		snapshot := *s.cached
		// 快照的 AIOverview 为值类型，浅拷贝后改写 RangeKey/Selected 不影响缓存本体。
		snapshot.Range = key
		snapshot.AI.RangeKey = key
		snapshot.AI.Selected = s.cachedRanges[key]
		s.mu.Unlock()
		return &snapshot
	}
	s.mu.Unlock()

	// 锁外计算：仓库状态可能耗时数秒，持锁会阻塞并发读；单用户场景下并发触发
	// 由前端 requestSeq 去抖，这里允许极小概率的重复计算，写回时以最后一次为准。
	snapshot, ranges := s.build(now, key)

	s.mu.Lock()
	s.cached = snapshot
	s.cachedRanges = ranges
	s.cachedAt = now
	s.mu.Unlock()

	return snapshot
}

// build 组装三域快照。各域独立降级，单域失败不影响其余域。
// 返回五档 AI 统计供缓存保存（切档复用）。
func (s *OverviewService) build(now time.Time, rangeKey string) (*model.OverviewSnapshot, map[string]model.AIRangeStats) {
	ai, ranges := s.buildAI(now)
	ai.RangeKey = rangeKey
	ai.Selected = ranges[rangeKey]
	return &model.OverviewSnapshot{
		GeneratedAt: now.UnixMilli(),
		Range:       rangeKey,
		Repos:       s.buildRepos(now),
		AI:          ai,
		Todos:       s.buildTodos(now),
	}, ranges
}

// buildRepos 计算 M1（待收尾仓库）与 M2（久未推进仓库）。
func (s *OverviewService) buildRepos(now time.Time) model.RepoOverview {
	staleDays := s.staleRepoDays()
	out := model.RepoOverview{StaleDays: staleDays}
	if s.dashboard == nil {
		out.Error = "看板服务未就绪"
		return out
	}

	statuses := s.dashboard.GetStatuses()
	out.PinnedTotal = len(statuses)
	staleAfter := time.Duration(staleDays) * 24 * time.Hour
	for _, status := range statuses {
		// 失效路径与非 Git 仓库无法计算工作树与提交时间，只单列不计数（spec §3）。
		if status.Missing || !status.IsRepo {
			out.MissingCount++
			continue
		}
		if status.Dirty || status.StashCount > 0 {
			out.PendingCount++
		}
		if status.LastCommitUnix <= 0 {
			// 空仓库（unborn）或提交时间查询失败，无法计算距今天数。
			out.NoCommitCount++
			continue
		}
		// 按实际时长比较而非向下取整的天数：提交于 7 天前 1 小时应计入
		// 「超过 7 天未提交」，取整会把它算作 7 天而漏计（与卡片提示文案一致）。
		// 负时长来自本机时钟回拨或提交时间在未来，自然不满足 > 阈值。
		if now.Sub(time.Unix(status.LastCommitUnix, 0)) > staleAfter {
			out.StaleCount++
		}
	}
	return out
}

// buildAI 计算 M4/M5 的全部五档统计（一次 build 算齐，缓存后切档零重算）。
//
// 与 AI 历史面板同源：五档各调用一次 AiTaskHistoryService.Stats，不再手写状态
// 计数与成本累加，概览与历史抽屉的数值由同一段聚合代码产出。
// 窗口按 StartedAt；canceled 不进成功率分母（spec §2 M5）。
//
// 五档共用同一 now（一次 build 内不重复取时钟），避免跨秒漂移导致档位边界不一致。
// 顶层 Today/Week 与本语义字段恒取 week 档，保持 T2 契约不变。
func (s *OverviewService) buildAI(now time.Time) (model.AIOverview, map[string]model.AIRangeStats) {
	out := model.AIOverview{}
	ranges := make(map[string]model.AIRangeStats, len(overviewRangeKeys))
	if s.aiFunc == nil {
		out.Error = "AI 服务未就绪"
		return out, ranges
	}

	for _, key := range overviewRangeKeys {
		start := overviewRangeStart(key, now).UnixMilli()
		stats, err := s.aiFunc.GetAiTaskHistoryStats(&model.AiTaskHistoryFilter{From: start})
		if err != nil || stats == nil {
			out.Error = fmt.Sprintf("读取 AI 统计失败(%s): %v", key, err)
			return out, ranges
		}
		ranges[key] = aiRangeStatsFromStats(stats)
	}

	week := ranges[overviewRangeWeek]
	out.Week = week.Cost
	out.Today = ranges[overviewRangeToday].Cost
	out.SuccessRate = week.SuccessRate
	out.HasData = week.HasData
	out.SuccessCount = week.SuccessCount
	out.FailedCount = week.FailedCount
	out.TimeoutCount = week.TimeoutCount
	out.CanceledCount = week.CanceledCount
	return out, ranges
}

// aiRangeStatsFromStats 把单档位统计映射为 AIRangeStats。
//
// 分母显式取 success+failed+timeout 三态之和，与卡片文案「success ÷
// （success+failed+timeout）」同式，不依赖「归档状态为闭集」这一隐含假设：
// 历史文件若出现未知状态，只会计入 TotalCount 而不会污染分母。
// 四状态计数之和 = TotalCount 仍作为 Stats 实现的验证点，但不作为分母推导前提。
func aiRangeStatsFromStats(stats *model.AiTaskHistoryStats) model.AIRangeStats {
	out := model.AIRangeStats{
		Cost:          aiCostBlockFromStats(stats),
		SuccessCount:  stats.SuccessCount,
		FailedCount:   stats.FailedCount,
		TimeoutCount:  stats.TimeoutCount,
		CanceledCount: stats.CanceledCount,
	}
	denominator := stats.SuccessCount + stats.FailedCount + stats.TimeoutCount
	out.HasData = denominator > 0
	if denominator > 0 {
		out.SuccessRate = float64(stats.SuccessCount) / float64(denominator)
	}
	return out
}

// aiCostBlockFromStats 把单窗口统计映射为卡片成本块。
// Metrics 为 nil 的记录在 Stats 内只计 TotalCount、不计成本与 token，与 T1 口径一致。
func aiCostBlockFromStats(stats *model.AiTaskHistoryStats) model.AICostBlock {
	return model.AICostBlock{
		CostUSD:             stats.TotalCostUSD,
		TaskCount:           stats.TotalCount,
		InputTokens:         stats.TotalInputTokens,
		OutputTokens:        stats.TotalOutputTokens,
		CacheReadTokens:     stats.TotalCacheReadTokens,
		CacheCreationTokens: stats.TotalCacheCreationTokens,
	}
}

// buildTodos 计算 M3（未完成待办条数与最老滞留天数）。
func (s *OverviewService) buildTodos(now time.Time) model.TodoOverview {
	out := model.TodoOverview{}
	if s.tempTodo == nil {
		out.Error = "待办服务未就绪"
		return out
	}

	list, err := s.tempTodo.List()
	if err != nil {
		out.Error = fmt.Sprintf("读取临时待办失败: %v", err)
		return out
	}

	var oldest *model.TempTodo
	for i := range list {
		todo := &list[i]
		if todo.CompletedAt != nil {
			continue
		}
		out.PendingCount++
		if oldest == nil || todo.CreatedAt < oldest.CreatedAt {
			oldest = todo
		}
	}
	if oldest == nil {
		return out
	}

	out.OldestID = oldest.ID
	out.OldestText = oldest.Text
	days := int(now.Sub(time.UnixMilli(oldest.CreatedAt)).Hours() / 24)
	if days > 0 {
		out.OldestDays = days
	}
	return out
}

// staleRepoDays 读取 M2 阈值；设置不可用或越界时回落默认 7 天并记 Warn（不阻断快照）。
func (s *OverviewService) staleRepoDays() int {
	if s.settings == nil {
		return model.DefaultDashboardStaleRepoDays
	}
	cfg, err := s.settings.Load()
	if err != nil {
		Logger().Warn("load settings for overview failed", "err", err)
		return model.DefaultDashboardStaleRepoDays
	}
	if cfg == nil || cfg.Dashboard == nil || cfg.Dashboard.StaleRepoDays <= 0 {
		return model.DefaultDashboardStaleRepoDays
	}
	return cfg.Dashboard.StaleRepoDays
}

// 时间档位 key（固定五档，见 docs/spec/dashboard-metrics.md §4）。
const (
	overviewRangeToday = "today"
	overviewRangeWeek  = "week"
	overviewRange7d    = "7d"
	overviewRange30d   = "30d"
	overviewRange90d   = "90d"
)

// overviewRangeKeys 五档 key 的固定顺序（与前端控件顺序一致）。
// 不设 365 天档：AI 历史保留上限即 90 天，更长档位恒等于 90 天的结果，属伪档位。
var overviewRangeKeys = []string{
	overviewRangeToday,
	overviewRangeWeek,
	overviewRange7d,
	overviewRange30d,
	overviewRange90d,
}

// normalizeOverviewRange 归一化时间档位，空值/未知值回落默认档 week。
func normalizeOverviewRange(v string) string {
	for _, key := range overviewRangeKeys {
		if v == key {
			return v
		}
	}
	return overviewRangeWeek
}

// overviewRangeStart 返回档位窗口起点（本地时区），是唯一的窗口起点换算点。
//
// 「近 N 天」按自然日边界：今天 00:00 往前 N−1 天，含今天共 N 个自然日，
// 与「今日/本周」同族（spec §4）。buildAI 与单测都经本函数，禁止在别处重算日期。
func overviewRangeStart(rangeKey string, now time.Time) time.Time {
	day := startOfDay(now)
	switch rangeKey {
	case overviewRangeToday:
		return day
	case overviewRange7d:
		return day.AddDate(0, 0, -6)
	case overviewRange30d:
		return day.AddDate(0, 0, -29)
	case overviewRange90d:
		return day.AddDate(0, 0, -89)
	default: // week 及任何未知档位
		return startOfWeek(now)
	}
}

// startOfDay 返回本地时区当天 00:00。
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// startOfWeek 返回本地时区本周一 00:00（周一为一周起点）。
func startOfWeek(t time.Time) time.Time {
	day := startOfDay(t)
	// time.Weekday 周日为 0，换算成「距本周一的天数」。
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}
