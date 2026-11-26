package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"workbench/model"
	"workbench/util/testutil"
)

// newTestOverviewService 构造指向临时目录的概览聚合服务及其三个数据源。
func newTestOverviewService(t *testing.T) (*OverviewService, *DashboardService, *TempTodoService, *AiFunctionService) {
	t.Helper()
	dir := t.TempDir()
	dashboard := NewDashboardService(filepath.Join(dir, "dashboard_pinned.json"))
	tempTodo := NewTempTodoService(filepath.Join(dir, "temp_todos.json"))
	aiFunc := NewAiFunctionService(context.Background(), filepath.Join(dir, "ai_functions.json"))
	settings := NewSettingsService(filepath.Join(dir, "settings.json"))
	return NewOverviewService(dashboard, tempTodo, aiFunc, settings), dashboard, tempTodo, aiFunc
}

// commitWithDate 在仓库写一个文件并以指定时间提交（author/committer 同步）。
// 文件名带时间戳，避免同一仓库重复提交时无变更。
func commitWithDate(t *testing.T, dir string, when time.Time) {
	t.Helper()
	testutil.WriteFile(t, filepath.Join(dir, "c-"+when.Format("20060102150405")+".txt"), when.String())
	testutil.RunGit(t, dir, "add", ".")
	t.Setenv("GIT_COMMITTER_DATE", when.Format(time.RFC3339))
	testutil.RunGit(t, dir, "commit", "-m", "commit at "+when.String())
}

// TestOverviewSnapshot_Empty 三域均无数据时返回零值与默认阈值，不显示错误。
func TestOverviewSnapshot_Empty(t *testing.T) {
	svc, _, _, _ := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return fixed }

	snap := svc.GetSnapshot("", false)
	if snap.GeneratedAt != fixed.UnixMilli() {
		t.Errorf("GeneratedAt = %d，期望 %d", snap.GeneratedAt, fixed.UnixMilli())
	}
	if snap.Repos.PinnedTotal != 0 || snap.Repos.PendingCount != 0 || snap.Repos.StaleCount != 0 {
		t.Errorf("空仓库域计数不符: %+v", snap.Repos)
	}
	if snap.Repos.StaleDays != model.DefaultDashboardStaleRepoDays {
		t.Errorf("默认阈值 = %d，期望 %d", snap.Repos.StaleDays, model.DefaultDashboardStaleRepoDays)
	}
	if snap.Repos.Error != "" {
		t.Errorf("空数据不应产生仓库域错误: %q", snap.Repos.Error)
	}
	if snap.AI.HasData || snap.AI.SuccessRate != 0 || snap.AI.Error != "" {
		t.Errorf("空 AI 域不符: %+v", snap.AI)
	}
	if snap.Todos.PendingCount != 0 || snap.Todos.Error != "" {
		t.Errorf("空待办域不符: %+v", snap.Todos)
	}
}

// TestOverviewSnapshot_RepoDomain 覆盖 M1/M2 与失效、无提交两类单列口径。
func TestOverviewSnapshot_RepoDomain(t *testing.T) {
	svc, dashboard, _, _ := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return fixed }

	// 待收尾：有提交但工作区脏。
	dirtyRepo := testutil.InitTempRepo(t)
	commitWithDate(t, dirtyRepo, fixed.Add(-1*time.Hour))
	testutil.WriteFile(t, filepath.Join(dirtyRepo, "dirty.txt"), "uncommitted")

	// 久未推进：最近提交在 10 天前（> 默认 7 天）。
	staleRepo := testutil.InitTempRepo(t)
	commitWithDate(t, staleRepo, fixed.Add(-10*24*time.Hour))

	// 无提交：初始化后从未提交（unborn）。
	unbornRepo := testutil.InitTempRepo(t)

	// 失效：pin 后目录被删除。
	missingRepo := filepath.Join(t.TempDir(), "gone")

	for _, path := range []string{dirtyRepo, staleRepo, unbornRepo, missingRepo} {
		if err := dashboard.AddPin(path); err != nil {
			t.Fatalf("AddPin(%s) 失败: %v", path, err)
		}
	}

	snap := svc.GetSnapshot("", true)
	if snap.Repos.PinnedTotal != 4 {
		t.Errorf("PinnedTotal = %d，期望 4", snap.Repos.PinnedTotal)
	}
	if snap.Repos.PendingCount != 1 {
		t.Errorf("PendingCount = %d，期望 1", snap.Repos.PendingCount)
	}
	if snap.Repos.StaleCount != 1 {
		t.Errorf("StaleCount = %d，期望 1", snap.Repos.StaleCount)
	}
	if snap.Repos.MissingCount != 1 {
		t.Errorf("MissingCount = %d，期望 1", snap.Repos.MissingCount)
	}
	if snap.Repos.NoCommitCount != 1 {
		t.Errorf("NoCommitCount = %d，期望 1", snap.Repos.NoCommitCount)
	}
}

// TestOverviewSnapshot_StaleDaysFromSettings 阈值取自设置段，改动后 M2 计数随之变化。
func TestOverviewSnapshot_StaleDaysFromSettings(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	testutil.WriteFile(t, settingsPath, `{"dashboard":{"staleRepoDays":3}}`)

	repo := testutil.InitTempRepo(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	commitWithDate(t, repo, fixed.Add(-5*24*time.Hour))

	dashboard := NewDashboardService(filepath.Join(dir, "pinned.json"))
	if err := dashboard.AddPin(repo); err != nil {
		t.Fatalf("AddPin 失败: %v", err)
	}

	svc := NewOverviewService(dashboard, NewTempTodoService(filepath.Join(dir, "todos.json")),
		NewAiFunctionService(context.Background(), filepath.Join(dir, "ai.json")),
		NewSettingsService(settingsPath))
	svc.now = func() time.Time { return fixed }

	snap := svc.GetSnapshot("", true)
	if snap.Repos.StaleDays != 3 {
		t.Errorf("StaleDays = %d，期望 3", snap.Repos.StaleDays)
	}
	// 5 天前提交：默认 7 天不 stale，设置 3 天后应计入。
	if snap.Repos.StaleCount != 1 {
		t.Errorf("StaleCount = %d，期望 1", snap.Repos.StaleCount)
	}
}

// TestOverviewSnapshot_AIDomain 覆盖 M4 成本/token 窗口与 M5 cancelled 不进分母。
func TestOverviewSnapshot_AIDomain(t *testing.T) {
	svc, _, _, aiFunc := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local) // 周三
	svc.now = func() time.Time { return fixed }

	usage := &model.AiTaskUsage{
		InputTokens:              1000,
		OutputTokens:             500,
		CacheReadInputTokens:     200,
		CacheCreationInputTokens: 100,
	}
	entries := []*model.AiTaskHistory{
		{
			ID: "h1", FunctionID: "f1", Name: "周报", Status: "success",
			StartedAt:  fixed.Add(-1 * time.Hour).UnixMilli(),
			FinishedAt: fixed.UnixMilli(),
			Metrics:    &model.AiTaskMetrics{CostUSD: 0.5, Usage: usage},
		},
		{
			// 用户取消：不进成功率分母，但成本照计。
			ID: "h2", FunctionID: "f1", Name: "周报", Status: "canceled",
			StartedAt:  fixed.Add(-2 * time.Hour).UnixMilli(),
			FinishedAt: fixed.UnixMilli(),
			Metrics:    &model.AiTaskMetrics{CostUSD: 0.1, Usage: usage},
		},
		{
			// 8 天前：必然早于本周一 00:00，不计入本周。
			ID: "h3", FunctionID: "f2", Name: "会议", Status: "failed",
			StartedAt:  fixed.Add(-8 * 24 * time.Hour).UnixMilli(),
			FinishedAt: fixed.UnixMilli(),
			Metrics:    &model.AiTaskMetrics{CostUSD: 9.9, Usage: usage},
		},
	}
	for _, entry := range entries {
		if _, err := aiFunc.historySvc.Archive(entry, ""); err != nil {
			t.Fatalf("Archive(%s) 失败: %v", entry.ID, err)
		}
	}

	snap := svc.GetSnapshot("", true)
	if snap.AI.Error != "" {
		t.Fatalf("AI 域不应报错: %q", snap.AI.Error)
	}
	if !snap.AI.HasData {
		t.Fatal("本周有有效任务，HasData 应为 true")
	}
	// success=1、canceled 不计、failed 在上周之外 → 成功率 100%。
	if snap.AI.SuccessRate != 1 {
		t.Errorf("SuccessRate = %v，期望 1", snap.AI.SuccessRate)
	}
	if snap.AI.Week.TaskCount != 2 {
		t.Errorf("Week.TaskCount = %d，期望 2", snap.AI.Week.TaskCount)
	}
	if diff := snap.AI.Week.CostUSD - 0.6; diff > 0.000001 || diff < -0.000001 {
		t.Errorf("Week.CostUSD = %v，期望 0.6", snap.AI.Week.CostUSD)
	}
	if snap.AI.Today.TaskCount != 2 || snap.AI.Today.InputTokens != 2000 {
		t.Errorf("Today 聚合不符: %+v", snap.AI.Today)
	}
	if snap.AI.Week.CacheReadTokens != 400 || snap.AI.Week.CacheCreationTokens != 200 {
		t.Errorf("Week 缓存 token 不符: %+v", snap.AI.Week)
	}
}

// TestOverviewSnapshot_TodoDomain 覆盖 M3 只统计未完成与最老滞留天数。
func TestOverviewSnapshot_TodoDomain(t *testing.T) {
	svc, _, tempTodo, _ := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return fixed }

	if _, err := tempTodo.AddMany([]model.TempTodoDraft{
		{Text: "十天前的待办", CreatedAt: fixed.Add(-10 * 24 * time.Hour).UnixMilli()},
		{Text: "昨天的待办", CreatedAt: fixed.Add(-24 * time.Hour).UnixMilli()},
	}); err != nil {
		t.Fatalf("AddMany 失败: %v", err)
	}
	done, err := tempTodo.Add("已完成的待办", nil)
	if err != nil {
		t.Fatalf("Add 失败: %v", err)
	}
	if err := tempTodo.SetCompleted(done.ID, true); err != nil {
		t.Fatalf("SetCompleted 失败: %v", err)
	}

	snap := svc.GetSnapshot("", true)
	if snap.Todos.Error != "" {
		t.Fatalf("待办域不应报错: %q", snap.Todos.Error)
	}
	if snap.Todos.PendingCount != 2 {
		t.Errorf("PendingCount = %d，期望 2", snap.Todos.PendingCount)
	}
	if snap.Todos.OldestDays != 10 {
		t.Errorf("OldestDays = %d，期望 10", snap.Todos.OldestDays)
	}
	if snap.Todos.OldestText != "十天前的待办" || snap.Todos.OldestID == "" {
		t.Errorf("最老条目不符: %+v", snap.Todos)
	}
}

// TestOverviewSnapshot_TTL 覆盖 5 分钟缓存：未过期不重算、force 强制重算、过期后重算。
func TestOverviewSnapshot_TTL(t *testing.T) {
	svc, _, tempTodo, _ := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return fixed }

	first := svc.GetSnapshot("", false)
	if first.Todos.PendingCount != 0 {
		t.Fatalf("初始待办应为 0，实际 %d", first.Todos.PendingCount)
	}

	if _, err := tempTodo.Add("新增待办", nil); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}

	cached := svc.GetSnapshot("", false)
	if cached.Todos.PendingCount != 0 {
		t.Errorf("TTL 内应复用缓存，实际 PendingCount=%d", cached.Todos.PendingCount)
	}

	forced := svc.GetSnapshot("", true)
	if forced.Todos.PendingCount != 1 {
		t.Errorf("force 应重算，实际 PendingCount=%d", forced.Todos.PendingCount)
	}

	svc.now = func() time.Time { return fixed.Add(6 * time.Minute) }
	expired := svc.GetSnapshot("", false)
	if expired.Todos.PendingCount != 1 {
		t.Errorf("TTL 过期后应重算，实际 PendingCount=%d", expired.Todos.PendingCount)
	}
}

// TestOverviewSnapshot_PartialDegradation 单域依赖缺失时只降级该域，其余域正常。
func TestOverviewSnapshot_PartialDegradation(t *testing.T) {
	svc := &OverviewService{now: time.Now}
	snap := svc.GetSnapshot("", true)

	if snap.Repos.Error == "" {
		t.Error("仓库域依赖缺失应产生错误标记")
	}
	if snap.AI.Error == "" {
		t.Error("AI 域依赖缺失应产生错误标记")
	}
	if snap.Todos.Error == "" {
		t.Error("待办域依赖缺失应产生错误标记")
	}
	// 分区降级的关键：失败不得伪装成 0，错误信息必须可见。
	if snap.Repos.PendingCount != 0 || snap.AI.HasData {
		t.Errorf("依赖缺失时不应产生伪造数据: repos=%+v ai=%+v", snap.Repos, snap.AI)
	}
}

// TestOverviewSnapshot_SettingsUnavailable settings 依赖缺失时 M2 阈值回落默认 7 天。
func TestOverviewSnapshot_SettingsUnavailable(t *testing.T) {
	dir := t.TempDir()
	svc := NewOverviewService(
		NewDashboardService(filepath.Join(dir, "pinned.json")),
		NewTempTodoService(filepath.Join(dir, "todos.json")),
		NewAiFunctionService(context.Background(), filepath.Join(dir, "ai.json")),
		nil,
	)
	svc.now = func() time.Time { return time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local) }

	snap := svc.GetSnapshot("", true)
	if snap.Repos.StaleDays != model.DefaultDashboardStaleRepoDays {
		t.Errorf("StaleDays = %d，期望默认 %d", snap.Repos.StaleDays, model.DefaultDashboardStaleRepoDays)
	}
}

// TestOverviewSnapshot_AINilUsage metrics 非 nil 但 Usage 为 nil：计成本与任务数，token 保持 0。
func TestOverviewSnapshot_AINilUsage(t *testing.T) {
	svc, _, _, aiFunc := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return fixed }

	entry := &model.AiTaskHistory{
		ID: "h1", FunctionID: "f1", Name: "周报", Status: "success",
		StartedAt:  fixed.Add(-time.Hour).UnixMilli(),
		FinishedAt: fixed.UnixMilli(),
		Metrics:    &model.AiTaskMetrics{CostUSD: 0.25},
	}
	if _, err := aiFunc.historySvc.Archive(entry, ""); err != nil {
		t.Fatalf("Archive 失败: %v", err)
	}

	snap := svc.GetSnapshot("", true)
	if snap.AI.Week.CostUSD != 0.25 || snap.AI.Week.TaskCount != 1 {
		t.Errorf("Week 聚合不符: %+v", snap.AI.Week)
	}
	if snap.AI.Week.InputTokens != 0 || snap.AI.Week.OutputTokens != 0 {
		t.Errorf("Usage 为 nil 时 token 应为 0: %+v", snap.AI.Week)
	}
}

// TestOverviewSnapshot_StaleThresholdBoundary M2 阈值边界：恰好 7 天不计入，
// 超过 7 天 1 小时计入（按实际时长比较，而非向下取整的天数）。
func TestOverviewSnapshot_StaleThresholdBoundary(t *testing.T) {
	dir := t.TempDir()
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)

	exactRepo := testutil.InitTempRepo(t)
	commitWithDate(t, exactRepo, fixed.Add(-7*24*time.Hour))

	overRepo := testutil.InitTempRepo(t)
	commitWithDate(t, overRepo, fixed.Add(-7*24*time.Hour-time.Hour))

	dashboard := NewDashboardService(filepath.Join(dir, "pinned.json"))
	for _, path := range []string{exactRepo, overRepo} {
		if err := dashboard.AddPin(path); err != nil {
			t.Fatalf("AddPin(%s) 失败: %v", path, err)
		}
	}

	svc := NewOverviewService(dashboard, NewTempTodoService(filepath.Join(dir, "todos.json")),
		NewAiFunctionService(context.Background(), filepath.Join(dir, "ai.json")),
		NewSettingsService(filepath.Join(dir, "settings.json")))
	svc.now = func() time.Time { return fixed }

	snap := svc.GetSnapshot("", true)
	if snap.Repos.StaleCount != 1 {
		t.Errorf("StaleCount = %d，期望 1（恰好 7 天不计、超 7 天 1 小时计入）", snap.Repos.StaleCount)
	}
}

// TestOverviewSnapshot_AIMatchesStats 概览 AI 域与 AiTaskHistoryService.Stats 同口径：
// 同一时间窗的成本/token/任务数与成功率必须逐项一致（T2 聚合同源的核心断言）。
// 含 Metrics==nil 记录，锁定「只计任务数、不计成本与 token」的累加规则。
func TestOverviewSnapshot_AIMatchesStats(t *testing.T) {
	svc, _, _, aiFunc := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local) // 周三
	svc.now = func() time.Time { return fixed }

	usage := &model.AiTaskUsage{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 10, CacheCreationInputTokens: 5}
	entries := []*model.AiTaskHistory{
		{ID: "m1", FunctionID: "f1", Name: "周报", Status: "success", StartedAt: fixed.Add(-4 * time.Hour).UnixMilli(), FinishedAt: fixed.UnixMilli(), Metrics: &model.AiTaskMetrics{CostUSD: 0.2, Usage: usage}},
		{ID: "m2", FunctionID: "f1", Name: "周报", Status: "failed", StartedAt: fixed.Add(-3 * time.Hour).UnixMilli(), FinishedAt: fixed.UnixMilli(), Metrics: &model.AiTaskMetrics{CostUSD: 0.1, Usage: usage}},
		{ID: "m3", FunctionID: "f2", Name: "会议", Status: "timeout", StartedAt: fixed.Add(-2 * time.Hour).UnixMilli(), FinishedAt: fixed.UnixMilli(), Metrics: &model.AiTaskMetrics{CostUSD: 0.05, Usage: usage}},
		{ID: "m4", FunctionID: "f2", Name: "会议", Status: "canceled", StartedAt: fixed.Add(-time.Hour).UnixMilli(), FinishedAt: fixed.UnixMilli(), Metrics: &model.AiTaskMetrics{CostUSD: 0.01, Usage: usage}},
		// 异常 result：计入任务数，不参与成本/token 累加
		{ID: "m5", FunctionID: "f1", Name: "周报", Status: "success", StartedAt: fixed.Add(-30 * time.Minute).UnixMilli(), FinishedAt: fixed.UnixMilli()},
	}
	for _, entry := range entries {
		if _, err := aiFunc.historySvc.Archive(entry, ""); err != nil {
			t.Fatalf("Archive(%s) 失败: %v", entry.ID, err)
		}
	}

	snap := svc.GetSnapshot("", true)
	if snap.AI.Error != "" {
		t.Fatalf("AI 域不应报错: %q", snap.AI.Error)
	}

	weekStats, err := aiFunc.GetAiTaskHistoryStats(&model.AiTaskHistoryFilter{From: startOfWeek(fixed).UnixMilli()})
	if err != nil {
		t.Fatalf("读取周统计失败: %v", err)
	}
	if snap.AI.Week.TaskCount != weekStats.TotalCount {
		t.Errorf("TaskCount 与 Stats 不一致: %d vs %d", snap.AI.Week.TaskCount, weekStats.TotalCount)
	}
	if snap.AI.Week.CostUSD != weekStats.TotalCostUSD {
		t.Errorf("CostUSD 与 Stats 不一致: %v vs %v", snap.AI.Week.CostUSD, weekStats.TotalCostUSD)
	}
	if snap.AI.Week.InputTokens != weekStats.TotalInputTokens ||
		snap.AI.Week.OutputTokens != weekStats.TotalOutputTokens ||
		snap.AI.Week.CacheReadTokens != weekStats.TotalCacheReadTokens ||
		snap.AI.Week.CacheCreationTokens != weekStats.TotalCacheCreationTokens {
		t.Errorf("token 与 Stats 不一致: overview=%+v stats=%+v", snap.AI.Week, weekStats)
	}

	// 四个状态计数逐项与 Stats 对齐（四状态之和 = TotalCount 的不变式）。
	if snap.AI.SuccessCount != weekStats.SuccessCount ||
		snap.AI.FailedCount != weekStats.FailedCount ||
		snap.AI.TimeoutCount != weekStats.TimeoutCount ||
		snap.AI.CanceledCount != weekStats.CanceledCount {
		t.Errorf("四状态计数与 Stats 不一致: overview success=%d failed=%d timeout=%d canceled=%d / stats success=%d failed=%d timeout=%d canceled=%d",
			snap.AI.SuccessCount, snap.AI.FailedCount, snap.AI.TimeoutCount, snap.AI.CanceledCount,
			weekStats.SuccessCount, weekStats.FailedCount, weekStats.TimeoutCount, weekStats.CanceledCount)
	}

	// success=2（m1、m5），分母 = success+failed+timeout = 2+1+1 = 4 → 0.5
	wantDenominator := weekStats.SuccessCount + weekStats.FailedCount + weekStats.TimeoutCount
	wantRate := float64(weekStats.SuccessCount) / float64(wantDenominator)
	if snap.AI.SuccessRate != wantRate {
		t.Errorf("SuccessRate = %v，期望 %v（Stats 同口径）", snap.AI.SuccessRate, wantRate)
	}
	if snap.AI.SuccessRate != 0.5 {
		t.Errorf("SuccessRate = %v，期望 0.5", snap.AI.SuccessRate)
	}
	// 今日窗口包含全部 5 条（均为 fixed 当天），成本不含 m5 的 nil metrics。
	todayStats, _ := aiFunc.GetAiTaskHistoryStats(&model.AiTaskHistoryFilter{From: startOfDay(fixed).UnixMilli()})
	if snap.AI.Today.TaskCount != todayStats.TotalCount || snap.AI.Today.CostUSD != todayStats.TotalCostUSD {
		t.Errorf("今日块与 Stats 不一致: overview=%+v stats=%+v", snap.AI.Today, todayStats)
	}
}

// TestOverviewSnapshot_RangeEcho 五档 key 原样回显，空值/未知值回落默认 week；
// 缓存命中时按本次入参回填 Range 与 AI.RangeKey（切档不回显旧值）。
func TestOverviewSnapshot_RangeEcho(t *testing.T) {
	svc, _, _, _ := newTestOverviewService(t)
	svc.now = func() time.Time { return time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local) }

	for _, key := range []string{"today", "week", "7d", "30d", "90d"} {
		snap := svc.GetSnapshot(key, true)
		if snap.Range != key {
			t.Errorf("%q 应原样回显，实际 %q", key, snap.Range)
		}
		if snap.AI.RangeKey != key {
			t.Errorf("%q 应回填 AI.RangeKey，实际 %q", key, snap.AI.RangeKey)
		}
	}

	// 空值/未知值统一回落默认档 week。
	if got := svc.GetSnapshot("", true); got.Range != "week" || got.AI.RangeKey != "week" {
		t.Errorf("空 range 应归一为 week，实际 range=%q rangeKey=%q", got.Range, got.AI.RangeKey)
	}
	if got := svc.GetSnapshot("365d", true); got.Range != "week" || got.AI.RangeKey != "week" {
		t.Errorf("未知 range 应归一为 week，实际 range=%q rangeKey=%q", got.Range, got.AI.RangeKey)
	}

	// force=false 命中缓存，Range 与 RangeKey 仍按本次入参回填。
	if got := svc.GetSnapshot("30d", false); got.Range != "30d" || got.AI.RangeKey != "30d" {
		t.Errorf("缓存命中应按入参回填，实际 range=%q rangeKey=%q", got.Range, got.AI.RangeKey)
	}
}

// TestOverviewRangeStart 五档窗口起点：today=当天 00:00、week=本周一 00:00、
// 近 N 天 = 今天 00:00 往前 N−1 天（含今天共 N 个自然日）。
// 固定周三验证基础档，另用周日与跨月/跨年锚点钉住边界换算。
func TestOverviewRangeStart(t *testing.T) {
	loc := time.Local
	cases := []struct {
		name string
		now  time.Time
		key  string
		want time.Time
	}{
		{
			// 2026-11-25 是周三
			name: "today-周三",
			now:  time.Date(2026, 11, 25, 12, 0, 0, 0, loc),
			key:  "today",
			want: time.Date(2026, 11, 25, 0, 0, 0, 0, loc),
		},
		{
			name: "week-周三",
			now:  time.Date(2026, 11, 25, 12, 0, 0, 0, loc),
			key:  "week",
			want: time.Date(2026, 11, 23, 0, 0, 0, 0, loc),
		},
		{
			// 周日是一周最后一天：本周一为 6 天前，不能算到次日。
			name: "week-周日边界",
			now:  time.Date(2026, 11, 29, 9, 0, 0, 0, loc),
			key:  "week",
			want: time.Date(2026, 11, 23, 0, 0, 0, 0, loc),
		},
		{
			name: "近7天-跨月",
			now:  time.Date(2026, 12, 2, 10, 0, 0, 0, loc),
			key:  "7d",
			want: time.Date(2026, 11, 26, 0, 0, 0, 0, loc),
		},
		{
			name: "近30天-跨年",
			now:  time.Date(2027, 1, 5, 10, 0, 0, 0, loc),
			key:  "30d",
			want: time.Date(2026, 12, 7, 0, 0, 0, 0, loc),
		},
		{
			name: "近90天-跨月",
			now:  time.Date(2026, 11, 25, 10, 0, 0, 0, loc),
			key:  "90d",
			want: time.Date(2026, 8, 28, 0, 0, 0, 0, loc),
		},
		{
			// 未知档位回落 week（与 normalizeOverviewRange 同口径）。
			name: "未知档位回落-week",
			now:  time.Date(2026, 11, 25, 12, 0, 0, 0, loc),
			key:  "365d",
			want: time.Date(2026, 11, 23, 0, 0, 0, 0, loc),
		},
	}

	for _, tc := range cases {
		if got := overviewRangeStart(tc.key, tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: overviewRangeStart = %v，期望 %v", tc.name, got, tc.want)
		}
	}
}

// TestOverviewSnapshot_SelectedMatchesStats 任意档位 Selected 与直接调用
// GetAiTaskHistoryStats(From=该档起点) 逐项一致；week 档与顶层本周字段相等。
func TestOverviewSnapshot_SelectedMatchesStats(t *testing.T) {
	svc, _, _, aiFunc := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local) // 周三
	svc.now = func() time.Time { return fixed }

	usage := &model.AiTaskUsage{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 10, CacheCreationInputTokens: 5}
	entries := []*model.AiTaskHistory{
		// 当天：success / failed / canceled
		{ID: "r1", FunctionID: "f1", Name: "周报", Status: "success", StartedAt: fixed.Add(-4 * time.Hour).UnixMilli(), FinishedAt: fixed.UnixMilli(), Metrics: &model.AiTaskMetrics{CostUSD: 0.2, Usage: usage}},
		{ID: "r2", FunctionID: "f1", Name: "周报", Status: "failed", StartedAt: fixed.Add(-3 * time.Hour).UnixMilli(), FinishedAt: fixed.UnixMilli(), Metrics: &model.AiTaskMetrics{CostUSD: 0.1, Usage: usage}},
		{ID: "r3", FunctionID: "f1", Name: "周报", Status: "canceled", StartedAt: fixed.Add(-2 * time.Hour).UnixMilli(), FinishedAt: fixed.UnixMilli(), Metrics: &model.AiTaskMetrics{CostUSD: 0.01, Usage: usage}},
		// 10 天前：仅落入 30d/90d 档
		{ID: "r4", FunctionID: "f2", Name: "会议", Status: "timeout", StartedAt: fixed.AddDate(0, 0, -10).UnixMilli(), FinishedAt: fixed.UnixMilli(), Metrics: &model.AiTaskMetrics{CostUSD: 1.0, Usage: usage}},
		// 60 天前：仅落入 90d 档
		{ID: "r5", FunctionID: "f3", Name: "归档", Status: "success", StartedAt: fixed.AddDate(0, 0, -60).UnixMilli(), FinishedAt: fixed.UnixMilli(), Metrics: &model.AiTaskMetrics{CostUSD: 2.0, Usage: usage}},
	}
	for _, entry := range entries {
		if _, err := aiFunc.historySvc.Archive(entry, ""); err != nil {
			t.Fatalf("Archive(%s) 失败: %v", entry.ID, err)
		}
	}

	for _, key := range overviewRangeKeys {
		snap := svc.GetSnapshot(key, true)
		start := overviewRangeStart(key, fixed).UnixMilli()
		stats, err := aiFunc.GetAiTaskHistoryStats(&model.AiTaskHistoryFilter{From: start})
		if err != nil {
			t.Fatalf("读取 %q 统计失败: %v", key, err)
		}
		want := aiRangeStatsFromStats(stats)
		if snap.AI.Selected != want {
			t.Errorf("%q 档 Selected 与 Stats 不一致: selected=%+v want=%+v", key, snap.AI.Selected, want)
		}
	}

	// week 档 Selected 与顶层本周语义字段逐项相等（纯增量契约的硬约束）。
	weekSnap := svc.GetSnapshot("week", true)
	if weekSnap.AI.Selected.Cost != weekSnap.AI.Week ||
		weekSnap.AI.Selected.SuccessRate != weekSnap.AI.SuccessRate ||
		weekSnap.AI.Selected.HasData != weekSnap.AI.HasData ||
		weekSnap.AI.Selected.SuccessCount != weekSnap.AI.SuccessCount ||
		weekSnap.AI.Selected.FailedCount != weekSnap.AI.FailedCount ||
		weekSnap.AI.Selected.TimeoutCount != weekSnap.AI.TimeoutCount ||
		weekSnap.AI.Selected.CanceledCount != weekSnap.AI.CanceledCount {
		t.Errorf("week 档 Selected 与顶层字段不等: selected=%+v ai=%+v", weekSnap.AI.Selected, weekSnap.AI)
	}
}

// TestOverviewSnapshot_RangeSwitchUsesCache 缓存有效期内切档不重算任何域：
// 时钟前移 1 秒（仍 < TTL）后切档，GeneratedAt 保持首次 build 的值即为命中缓存；
// 仓库/待办域数值不随档位变化。
func TestOverviewSnapshot_RangeSwitchUsesCache(t *testing.T) {
	svc, dashboard, tempTodo, aiFunc := newTestOverviewService(t)
	base := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return base }

	if _, err := tempTodo.Add("未完成待办", nil); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}
	entry := &model.AiTaskHistory{
		ID: "c1", FunctionID: "f1", Name: "周报", Status: "success",
		StartedAt:  base.Add(-time.Hour).UnixMilli(),
		FinishedAt: base.UnixMilli(),
		Metrics:    &model.AiTaskMetrics{CostUSD: 0.3},
	}
	if _, err := aiFunc.historySvc.Archive(entry, ""); err != nil {
		t.Fatalf("Archive 失败: %v", err)
	}

	first := svc.GetSnapshot("today", false)
	if first.GeneratedAt != base.UnixMilli() {
		t.Fatalf("首次 GeneratedAt = %d，期望 %d", first.GeneratedAt, base.UnixMilli())
	}

	// 状态变更探针：OverviewService 依赖均为具体类型指针，无接口注入缝，无法注入计数 stub；
	// 改以「首次 build 后变更数据源，若切档重算必然读到新数据」等价断言缓存未重算。
	if err := dashboard.AddPin(filepath.Join(t.TempDir(), "probe-pin")); err != nil {
		t.Fatalf("AddPin 失败: %v", err)
	}
	if _, err := tempTodo.Add("首次 build 后新增待办", nil); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}
	// 探针有效性自检：数据源确实已变化，否则「切档值不变」可能假绿。
	if got := len(dashboard.GetStatuses()); got != first.Repos.PinnedTotal+1 {
		t.Fatalf("状态变更探针未生效: GetStatuses=%d，期望 %d", got, first.Repos.PinnedTotal+1)
	}

	// 时钟前移 1 秒仍命中 TTL；若重算 GeneratedAt 会随之变化。
	svc.now = func() time.Time { return base.Add(time.Second) }
	switched := svc.GetSnapshot("90d", false)

	if switched.GeneratedAt != first.GeneratedAt {
		t.Errorf("切档应命中缓存不重算，GeneratedAt 变化 %d -> %d", first.GeneratedAt, switched.GeneratedAt)
	}
	if switched.Range != "90d" || switched.AI.RangeKey != "90d" {
		t.Errorf("切档回显不符: range=%q rangeKey=%q", switched.Range, switched.AI.RangeKey)
	}
	// 仓库/待办域是快照/全量语义，不随 AI 档位变化。
	if switched.Repos != first.Repos || switched.Todos != first.Todos {
		t.Errorf("仓库/待办域不应随档位变化: repos %+v -> %+v todos %+v -> %+v",
			first.Repos, switched.Repos, first.Todos, switched.Todos)
	}
	// 状态变更探针：命中缓存时两个域仍保持首次快照值；若切档触发重算，
	// PinnedTotal 会从 0 变 1、PendingCount 会从 1 变 2。
	if switched.Repos.PinnedTotal != first.Repos.PinnedTotal ||
		switched.Todos.PendingCount != first.Todos.PendingCount {
		t.Errorf("切档命中缓存不应重算仓库/待办域: PinnedTotal %d -> %d, PendingCount %d -> %d",
			first.Repos.PinnedTotal, switched.Repos.PinnedTotal,
			first.Todos.PendingCount, switched.Todos.PendingCount)
	}
	if switched.AI.Selected.Cost.CostUSD != 0.3 {
		t.Errorf("90d 档成本 = %v，期望 0.3", switched.AI.Selected.Cost.CostUSD)
	}
}

// TestOverviewSnapshot_AIFailureDegradesNotZero AI 域读取失败时只标记 Error，
// 不得把失败降级为 0 成本 / 0%（Selected 零值仅在 Error 非空时出现）。
func TestOverviewSnapshot_AIFailureDegradesNotZero(t *testing.T) {
	svc, _, _, _ := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return fixed }
	// 依赖缺失：最直接的可控失败路径（与 nil aiFunc 一致的分区降级语义）。
	svc.aiFunc = nil

	snap := svc.GetSnapshot("7d", true)
	if snap.AI.Error == "" {
		t.Fatal("AI 域依赖缺失应产生错误标记")
	}
	if snap.AI.Selected.HasData || snap.AI.HasData {
		t.Errorf("失败不得伪装成有数据: selected=%+v", snap.AI.Selected)
	}
}

// TestOverviewSnapshot_AIFailureNotCached AI 域失败快照不参与缓存命中：
// 损坏历史元数据制造真实失败，恢复数据后仅用 force=false 再请求即可自愈，
// 无需手动点「重试」（force=true），且 Selected 回到真实数据而非零值。
func TestOverviewSnapshot_AIFailureNotCached(t *testing.T) {
	svc, _, _, aiFunc := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return fixed }
	historyPath := aiFunc.historySvc.historyMetaPath()

	// 制造真实失败：损坏的历史元数据 JSON 让五档 Stats 读取报错。
	testutil.WriteFile(t, historyPath, "{broken json")

	failed := svc.GetSnapshot("", false)
	if failed.AI.Error == "" {
		t.Fatal("损坏历史文件应产生 AI 域错误")
	}
	if failed.AI.Selected.HasData {
		t.Errorf("失败态 Selected 不应伪装成有数据: %+v", failed.AI.Selected)
	}

	// 恢复数据：写回合法空数组后归档一条有效任务（归档内部会读到空数组）。
	testutil.WriteFile(t, historyPath, "[]")
	entry := &model.AiTaskHistory{
		ID: "recover-1", FunctionID: "f1", Name: "周报", Status: "success",
		StartedAt:  fixed.Add(-time.Hour).UnixMilli(),
		FinishedAt: fixed.UnixMilli(),
		Metrics:    &model.AiTaskMetrics{CostUSD: 0.42},
	}
	if _, err := aiFunc.historySvc.Archive(entry, ""); err != nil {
		t.Fatalf("Archive 失败: %v", err)
	}

	// 失败态下再次 force=false 请求不得复用失败缓存，必须重算并恢复真实数据。
	healed := svc.GetSnapshot("7d", false)
	if healed.AI.Error != "" {
		t.Errorf("数据恢复后 force=false 应重算，AI.Error = %q", healed.AI.Error)
	}
	if !healed.AI.Selected.HasData || healed.AI.Selected.Cost.TaskCount != 1 ||
		healed.AI.Selected.Cost.CostUSD != 0.42 {
		t.Errorf("恢复后 Selected 应为真实数据: %+v", healed.AI.Selected)
	}
	if healed.Range != "7d" || healed.AI.RangeKey != "7d" {
		t.Errorf("切档回显不符: range=%q rangeKey=%q", healed.Range, healed.AI.RangeKey)
	}
}

// TestOverviewSnapshot_AIFailureKeepsOtherDomains 分区降级的真实读取失败路径：
// 损坏历史元数据使五档 Stats 循环报错时只标记 AI.Error，仓库/待办域照常聚合，
// Selected 保持零值交由前端降级显示，不伪装成 0 成本 / 0%。
func TestOverviewSnapshot_AIFailureKeepsOtherDomains(t *testing.T) {
	svc, dashboard, tempTodo, aiFunc := newTestOverviewService(t)
	fixed := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return fixed }

	// 仓库/待办域先备好真实数据，用于验证 AI 失败不连累其余两域。
	repo := testutil.InitTempRepo(t)
	commitWithDate(t, repo, fixed.Add(-time.Hour))
	if err := dashboard.AddPin(repo); err != nil {
		t.Fatalf("AddPin 失败: %v", err)
	}
	if _, err := tempTodo.Add("未完成待办", nil); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}

	testutil.WriteFile(t, aiFunc.historySvc.historyMetaPath(), "{broken json")

	snap := svc.GetSnapshot("90d", true)
	if snap.AI.Error == "" {
		t.Fatal("损坏历史文件应产生 AI 域错误")
	}
	if snap.Repos.Error != "" || snap.Todos.Error != "" {
		t.Errorf("AI 域失败不应影响仓库/待办域: repos=%q todos=%q", snap.Repos.Error, snap.Todos.Error)
	}
	if snap.Repos.PinnedTotal != 1 || snap.Todos.PendingCount != 1 {
		t.Errorf("仓库/待办域应照常聚合: PinnedTotal=%d PendingCount=%d",
			snap.Repos.PinnedTotal, snap.Todos.PendingCount)
	}
	if snap.AI.Selected.HasData || snap.AI.Selected.Cost.TaskCount != 0 ||
		snap.AI.Selected.SuccessRate != 0 {
		t.Errorf("失败态 Selected 应保持零值: %+v", snap.AI.Selected)
	}
}
