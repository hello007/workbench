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

	mu       sync.Mutex
	cached   *model.OverviewSnapshot
	cachedAt time.Time
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
// rangeKey 仅作回显与 T3 预留：T1 无论传什么值都同时返回今日与本周两个窗口块，
// 保证 T3 只加切换控件、不再改契约。
func (s *OverviewService) GetSnapshot(rangeKey string, force bool) *model.OverviewSnapshot {
	now := s.now()

	s.mu.Lock()
	if !force && s.cached != nil && now.Sub(s.cachedAt) < overviewSnapshotTTL {
		snapshot := *s.cached
		// 缓存不含 range 维度（T1 恒同时返回今日与本周块），命中时按本次入参
		// 回显 range，避免 T3 引入切换控件后回显值与请求值不一致。
		snapshot.Range = normalizeOverviewRange(rangeKey)
		s.mu.Unlock()
		return &snapshot
	}
	s.mu.Unlock()

	// 锁外计算：仓库状态可能耗时数秒，持锁会阻塞并发读；单用户场景下并发触发
	// 由前端 requestSeq 去抖，这里允许极小概率的重复计算，写回时以最后一次为准。
	snapshot := s.build(now, normalizeOverviewRange(rangeKey))

	s.mu.Lock()
	s.cached = snapshot
	s.cachedAt = now
	s.mu.Unlock()

	return snapshot
}

// build 组装三域快照。各域独立降级，单域失败不影响其余域。
func (s *OverviewService) build(now time.Time, rangeKey string) *model.OverviewSnapshot {
	return &model.OverviewSnapshot{
		GeneratedAt: now.UnixMilli(),
		Range:       rangeKey,
		Repos:       s.buildRepos(now),
		AI:          s.buildAI(now),
		Todos:       s.buildTodos(now),
	}
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

// buildAI 计算 M4（今日/本周成本与 token）与 M5（本周成功率）。
//
// 与 AI 历史面板同源：两次调用 AiTaskHistoryService.Stats（今日、本周各一次），
// 不再手写状态计数与成本累加，概览与历史抽屉的数值由同一段聚合代码产出。
// 窗口按 StartedAt；canceled 不进成功率分母（spec §2 M5）。
func (s *OverviewService) buildAI(now time.Time) model.AIOverview {
	out := model.AIOverview{}
	if s.aiFunc == nil {
		out.Error = "AI 服务未就绪"
		return out
	}

	todayStart := startOfDay(now).UnixMilli()
	weekStart := startOfWeek(now).UnixMilli()

	weekStats, err := s.aiFunc.GetAiTaskHistoryStats(&model.AiTaskHistoryFilter{From: weekStart})
	if err != nil || weekStats == nil {
		out.Error = fmt.Sprintf("读取 AI 周统计失败: %v", err)
		return out
	}
	todayStats, err := s.aiFunc.GetAiTaskHistoryStats(&model.AiTaskHistoryFilter{From: todayStart})
	if err != nil || todayStats == nil {
		out.Error = fmt.Sprintf("读取 AI 今日统计失败: %v", err)
		return out
	}

	out.Week = aiCostBlockFromStats(weekStats)
	out.Today = aiCostBlockFromStats(todayStats)
	out.SuccessCount = weekStats.SuccessCount
	out.FailedCount = weekStats.FailedCount
	out.TimeoutCount = weekStats.TimeoutCount
	out.CanceledCount = weekStats.CanceledCount

	// 分母显式取 success+failed+timeout 三态之和，与卡片文案「success ÷
	// （success+failed+timeout）」同式，不依赖「归档状态为闭集」这一隐含假设：
	// 历史文件若出现未知状态，只会计入 TotalCount 而不会污染分母。
	// 四状态计数之和 = TotalCount 仍作为 Stats 实现的验证点，但不作为分母推导前提。
	denominator := weekStats.SuccessCount + weekStats.FailedCount + weekStats.TimeoutCount
	out.HasData = denominator > 0
	if denominator > 0 {
		out.SuccessRate = float64(weekStats.SuccessCount) / float64(denominator)
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

// normalizeOverviewRange 归一化时间范围回显值，未知值按 today 处理。
func normalizeOverviewRange(v string) string {
	if v == "week" {
		return "week"
	}
	return "today"
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
