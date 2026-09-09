package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
)

// newHistorySvc 创建指向临时 dataDir 的历史归档服务
func newHistorySvc(t *testing.T) *AiTaskHistoryService {
	t.Helper()
	return NewAiTaskHistoryService(t.TempDir())
}

// makeEntry 构造测试用历史元数据
func makeEntry(id string, finishedAt time.Time, status string, outputSize int64) *model.AiTaskHistory {
	return &model.AiTaskHistory{
		ID:         id,
		FunctionID: "fn-" + id,
		Name:       "功能-" + id,
		StartedAt:  finishedAt.Add(-time.Minute).UnixMilli(),
		FinishedAt: finishedAt.UnixMilli(),
		Status:     status,
		OutputSize: outputSize,
	}
}

// TestArchive_RenamesOutputFile 验证归档时输出文件 os.Rename 零拷贝移入历史目录，元数据落盘
func TestArchive_RenamesOutputFile(t *testing.T) {
	svc := newHistorySvc(t)
	// 准备运行期输出文件（模拟 data/ai_task_output/<id>.txt）
	outputDir := filepath.Join(svc.dataDir, aiTaskOutputDirName)
	_ = os.MkdirAll(outputDir, 0o755)
	srcPath := filepath.Join(outputDir, "aitask-1.txt")
	wantOutput := "完整输出内容\n多行"
	if err := os.WriteFile(srcPath, []byte(wantOutput), 0o644); err != nil {
		t.Fatalf("写源输出文件失败: %v", err)
	}

	entry := makeEntry("aitask-1", time.Now(), "success", int64(len(wantOutput)))
	archived, err := svc.Archive(entry, srcPath)
	if err != nil {
		t.Fatalf("归档失败: %v", err)
	}
	// 源文件应被移走（os.Rename 后原路径不存在）
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Errorf("归档后源输出文件应被移走: %v", err)
	}
	// 归档目录应有该文件
	dstPath := svc.historyFilePath("aitask-1")
	data, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("归档输出文件应存在: %v", err)
	}
	if string(data) != wantOutput {
		t.Errorf("归档输出内容不符: %q", string(data))
	}
	// 元数据 OutputFile 指向归档相对路径
	if archived.OutputFile == "" {
		t.Error("归档后 OutputFile 不应为空")
	}
	// 元数据已落盘
	list, _ := svc.loadAll()
	if len(list) != 1 || list[0].ID != "aitask-1" {
		t.Errorf("元数据未落盘: %v", list)
	}
}

// TestArchive_NoOutputFile 源输出文件不存在时仍归档元数据（OutputFile 留空）
func TestArchive_NoOutputFile(t *testing.T) {
	svc := newHistorySvc(t)
	entry := makeEntry("aitask-2", time.Now(), "failed", 0)
	archived, err := svc.Archive(entry, "/nonexistent/path.txt")
	if err != nil {
		t.Fatalf("无输出文件归档不应失败: %v", err)
	}
	if archived.OutputFile != "" {
		t.Errorf("无输出文件时 OutputFile 应留空: %s", archived.OutputFile)
	}
}

// TestList_Filter 验证筛选条件（功能/状态/时间范围）与降序
func TestList_Filter(t *testing.T) {
	svc := newHistorySvc(t)
	base := time.Now()
	// 三条不同功能/状态/时间
	entries := []*model.AiTaskHistory{
		makeEntry("t1", base.Add(-2*time.Hour), "success", 100),
		makeEntry("t2", base.Add(-1*time.Hour), "failed", 200),
		makeEntry("t3", base, "success", 300),
	}
	for _, e := range entries {
		_, _ = svc.Archive(e, "")
	}

	// 全部：降序（最新 t3 在前）
	all, _ := svc.List(nil)
	if len(all) != 3 || all[0].ID != "t3" {
		t.Errorf("降序不符: %v", ids(all))
	}

	// 按功能筛选
	fnFilter := &model.AiTaskHistoryFilter{FunctionID: "fn-t2"}
	fnList, _ := svc.List(fnFilter)
	if len(fnList) != 1 || fnList[0].ID != "t2" {
		t.Errorf("功能筛选不符: %v", ids(fnList))
	}

	// 按状态筛选
	stList, _ := svc.List(&model.AiTaskHistoryFilter{Status: "success"})
	if len(stList) != 2 {
		t.Errorf("状态筛选数量不符: %d（期望 2）", len(stList))
	}

	// 按时间范围：仅 t3（base 之后）
	timeList, _ := svc.List(&model.AiTaskHistoryFilter{From: base.Add(-30 * time.Minute).UnixMilli()})
	if len(timeList) != 1 || timeList[0].ID != "t3" {
		t.Errorf("时间筛选不符: %v", ids(timeList))
	}
}

// ids 辅助：从历史列表提取 id 切片
func ids(list []*model.AiTaskHistory) []string {
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, e.ID)
	}
	return out
}

// TestDelete 删除单条：元数据 + 输出文件同步删
func TestDelete(t *testing.T) {
	svc := newHistorySvc(t)
	// 归档一条带输出文件
	outputDir := filepath.Join(svc.dataDir, aiTaskOutputDirName)
	_ = os.MkdirAll(outputDir, 0o755)
	srcPath := filepath.Join(outputDir, "aitask-del.txt")
	_ = os.WriteFile(srcPath, []byte("to delete"), 0o644)
	entry := makeEntry("aitask-del", time.Now(), "success", 9)
	_, _ = svc.Archive(entry, srcPath)

	dstPath := svc.historyFilePath("aitask-del")
	if _, err := os.Stat(dstPath); err != nil {
		t.Fatalf("归档输出文件应存在: %v", err)
	}
	if !svc.Delete("aitask-del") {
		t.Error("删除应返回 true")
	}
	// 元数据已删
	list, _ := svc.List(nil)
	if len(list) != 0 {
		t.Errorf("删除后元数据应清空: %d", len(list))
	}
	// 输出文件已删
	if _, err := os.Stat(dstPath); !os.IsNotExist(err) {
		t.Errorf("删除后输出文件应不存在: %v", err)
	}
	// 删除不存在的返回 false
	if svc.Delete("nonexistent") {
		t.Error("删除不存在的应返回 false")
	}
}

// TestClear_OlderThan 按天数批量清理：删 N 天前，保留近期
func TestClear_OlderThan(t *testing.T) {
	svc := newHistorySvc(t)
	now := time.Now()
	old := makeEntry("old", now.Add(-40*24*time.Hour), "success", 10)      // 40 天前
	recent := makeEntry("recent", now.Add(-5*24*time.Hour), "success", 20) // 5 天前
	_, _ = svc.Archive(old, "")
	_, _ = svc.Archive(recent, "")

	n, err := svc.Clear(&model.AiTaskHistoryClearCriteria{OlderThanDays: 30})
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if n != 1 {
		t.Errorf("清理条数不符: %d（期望 1）", n)
	}
	list, _ := svc.List(nil)
	if len(list) != 1 || list[0].ID != "recent" {
		t.Errorf("清理后应仅剩 recent: %v", ids(list))
	}
}

// TestClear_KeepRecent 保留最近 N 条，清理其余
func TestClear_KeepRecent(t *testing.T) {
	svc := newHistorySvc(t)
	base := time.Now()
	for i := 0; i < 5; i++ {
		e := makeEntry(string(rune('a'+i)), base.Add(time.Duration(i)*time.Hour), "success", int64(i*10))
		_, _ = svc.Archive(e, "")
	}
	n, _ := svc.Clear(&model.AiTaskHistoryClearCriteria{KeepRecent: 2})
	if n != 3 {
		t.Errorf("清理条数不符: %d（期望 3）", n)
	}
	list, _ := svc.List(nil)
	if len(list) != 2 {
		t.Errorf("保留后应剩 2 条: %d", len(list))
	}
}

// TestEnforceRetention_MaxCount 归档时超条数上限自动清理最旧
func TestEnforceRetention_MaxCount(t *testing.T) {
	// 临时降低上限便于测试：直接构造超量列表调 enforceRetentionLocked
	svc := newHistorySvc(t)
	base := time.Now()
	list := make([]*model.AiTaskHistory, 0, aiTaskHistoryMaxCount+5)
	for i := 0; i < aiTaskHistoryMaxCount+5; i++ {
		list = append(list, makeEntry(string(rune('a'+i%26))+string(rune('a'+i/26)), base.Add(time.Duration(i)*time.Minute), "success", int64(i)))
	}
	kept := svc.enforceRetentionLocked(list)
	if len(kept) != aiTaskHistoryMaxCount {
		t.Errorf("保留后条数不符: %d（期望 %d）", len(kept), aiTaskHistoryMaxCount)
	}
}

// TestCleanStaleOutputFiles 定时清理：删 mtime 早于 TTL 的运行期输出文件
func TestCleanStaleOutputFiles(t *testing.T) {
	svc := newHistorySvc(t)
	outputDir := filepath.Join(svc.dataDir, aiTaskOutputDirName)
	_ = os.MkdirAll(outputDir, 0o755)
	// 旧文件（mtime 2 天前）
	oldPath := filepath.Join(outputDir, "old.txt")
	_ = os.WriteFile(oldPath, []byte("old"), 0o644)
	oldTime := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(oldPath, oldTime, oldTime)
	// 新文件（mtime 1 小时前）
	newPath := filepath.Join(outputDir, "new.txt")
	_ = os.WriteFile(newPath, []byte("new"), 0o644)
	newTime := time.Now().Add(-1 * time.Hour)
	_ = os.Chtimes(newPath, newTime, newTime)

	removed := svc.CleanStaleOutputFiles(24 * time.Hour)
	if removed != 1 {
		t.Errorf("清理条数不符: %d（期望 1）", removed)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("旧文件应被清理: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("新文件应保留: %v", err)
	}
}

// TestGetOutput 读取归档输出全文
func TestGetOutput(t *testing.T) {
	svc := newHistorySvc(t)
	outputDir := filepath.Join(svc.dataDir, aiTaskOutputDirName)
	_ = os.MkdirAll(outputDir, 0o755)
	srcPath := filepath.Join(outputDir, "aitask-go.txt")
	want := "归档输出全文\n多行内容"
	_ = os.WriteFile(srcPath, []byte(want), 0o644)
	_, _ = svc.Archive(makeEntry("aitask-go", time.Now(), "success", int64(len(want))), srcPath)

	got, err := svc.GetOutput("aitask-go")
	if err != nil {
		t.Fatalf("读取归档输出失败: %v", err)
	}
	if got != want {
		t.Errorf("归档输出内容不符: %q", got)
	}
	// 不存在的 id 报错
	if _, err := svc.GetOutput("nonexistent"); err == nil {
		t.Error("不存在的 id 应报错")
	}
}

// TestBuildPromptPreview prompt 预览截断
func TestBuildPromptPreview(t *testing.T) {
	short := "短 prompt"
	if got := buildPromptPreview(short, 200); got != short {
		t.Errorf("短 prompt 不应截断: %s", got)
	}
	long := string([]rune("每字算一符")[:3]) + string(make([]rune, 300))
	got := buildPromptPreview(long, 10)
	if len([]rune(got)) > 11 {
		t.Errorf("长 prompt 应截断到约 10 字: %d", len([]rune(got)))
	}
}

// makeEntryWithMetrics 构造带计量的历史元数据（Stats/Export 测试用）
func makeEntryWithMetrics(id, functionID, name string, finishedAt time.Time, status string, usage *model.AiTaskUsage, costUSD float64, durationMs int64) *model.AiTaskHistory {
	e := makeEntry(id, finishedAt, status, 100)
	e.FunctionID = functionID
	e.Name = name
	if usage != nil || costUSD > 0 || durationMs > 0 {
		e.Metrics = &model.AiTaskMetrics{Usage: usage, CostUSD: costUSD, DurationMs: durationMs}
	}
	return e
}

// TestStats_Aggregation 多记录多功能项聚合：总数/成功数/token 四分项/成本/耗时 + 按次数降序
func TestStats_Aggregation(t *testing.T) {
	svc := newHistorySvc(t)
	base := time.Now()
	entries := []*model.AiTaskHistory{
		// 周报：2 次（1 成功 1 失败），带计量
		makeEntryWithMetrics("s1", "fn-report", "生成周报", base.Add(-3*time.Hour), "success",
			&model.AiTaskUsage{InputTokens: 1000, OutputTokens: 500, CacheReadInputTokens: 200, CacheCreationInputTokens: 100}, 0.012, 60000),
		makeEntryWithMetrics("s2", "fn-report", "生成周报", base.Add(-2*time.Hour), "failed",
			&model.AiTaskUsage{InputTokens: 200, OutputTokens: 100, CacheReadInputTokens: 0, CacheCreationInputTokens: 50}, 0.003, 10000),
		// 会议预约：1 次成功
		makeEntryWithMetrics("s3", "fn-meeting", "预约腾讯会议", base.Add(-1*time.Hour), "success",
			&model.AiTaskUsage{InputTokens: 300, OutputTokens: 80, CacheReadInputTokens: 20, CacheCreationInputTokens: 10}, 0.005, 30000),
	}
	for _, e := range entries {
		_, _ = svc.Archive(e, "")
	}

	stats, err := svc.Stats(nil)
	if err != nil {
		t.Fatalf("Stats 失败: %v", err)
	}
	if stats.TotalCount != 3 || stats.SuccessCount != 2 {
		t.Errorf("计数不符: total=%d success=%d（期望 3/2）", stats.TotalCount, stats.SuccessCount)
	}
	if stats.TotalInputTokens != 1500 || stats.TotalOutputTokens != 680 {
		t.Errorf("入/出 token 不符: %d/%d（期望 1500/680）", stats.TotalInputTokens, stats.TotalOutputTokens)
	}
	if stats.TotalCacheReadTokens != 220 || stats.TotalCacheCreationTokens != 160 {
		t.Errorf("缓存 token 不符: read=%d creation=%d（期望 220/160）", stats.TotalCacheReadTokens, stats.TotalCacheCreationTokens)
	}
	if stats.TotalCostUSD < 0.0199 || stats.TotalCostUSD > 0.0201 {
		t.Errorf("总成本不符: %f（期望约 0.020）", stats.TotalCostUSD)
	}
	if stats.TotalDurationMs != 100000 {
		t.Errorf("总耗时不符: %d（期望 100000）", stats.TotalDurationMs)
	}
	// 功能排行按次数降序：fn-report(2) 在前
	if len(stats.ByFunction) != 2 {
		t.Fatalf("功能聚合数不符: %d（期望 2）", len(stats.ByFunction))
	}
	if stats.ByFunction[0].FunctionID != "fn-report" || stats.ByFunction[0].Count != 2 {
		t.Errorf("排行首位不符: %+v", stats.ByFunction[0])
	}
	if stats.ByFunction[0].TotalTokens != 1800 {
		t.Errorf("fn-report 入+出 token 不符: %d（期望 1800）", stats.ByFunction[0].TotalTokens)
	}
}

// TestStats_NilMetrics metrics 为 nil 的记录计入 count 但不累加计量
func TestStats_NilMetrics(t *testing.T) {
	svc := newHistorySvc(t)
	now := time.Now()
	_, _ = svc.Archive(makeEntry("m1", now, "failed", 0), "") // metrics 为 nil
	_, _ = svc.Archive(makeEntryWithMetrics("m2", "fn-a", "功能A", now.Add(-time.Hour), "success",
		&model.AiTaskUsage{InputTokens: 10, OutputTokens: 5}, 0.001, 1000), "")

	stats, _ := svc.Stats(nil)
	if stats.TotalCount != 2 {
		t.Errorf("nil metrics 应计入 TotalCount: %d（期望 2）", stats.TotalCount)
	}
	if stats.TotalInputTokens != 10 || stats.TotalCostUSD > 0.0011 || stats.TotalDurationMs != 1000 {
		t.Errorf("nil metrics 不应累加计量: %+v", stats)
	}
}

// TestStats_Filter Stats 尊重筛选条件（功能/状态/时间范围）
func TestStats_Filter(t *testing.T) {
	svc := newHistorySvc(t)
	base := time.Now()
	_, _ = svc.Archive(makeEntryWithMetrics("f1", "fn-a", "功能A", base.Add(-2*time.Hour), "success",
		&model.AiTaskUsage{InputTokens: 100, OutputTokens: 50}, 0.01, 5000), "")
	_, _ = svc.Archive(makeEntryWithMetrics("f2", "fn-b", "功能B", base, "failed",
		&model.AiTaskUsage{InputTokens: 200, OutputTokens: 0}, 0.02, 0), "")

	// 功能筛选
	fnStats, _ := svc.Stats(&model.AiTaskHistoryFilter{FunctionID: "fn-a"})
	if fnStats.TotalCount != 1 || fnStats.TotalInputTokens != 100 {
		t.Errorf("功能筛选聚合不符: %+v", fnStats)
	}
	// 状态筛选
	stStats, _ := svc.Stats(&model.AiTaskHistoryFilter{Status: "success"})
	if stStats.TotalCount != 1 || stStats.SuccessCount != 1 {
		t.Errorf("状态筛选聚合不符: %+v", stStats)
	}
	// 时间范围：仅 f2（base 时刻）
	timeStats, _ := svc.Stats(&model.AiTaskHistoryFilter{From: base.Add(-time.Hour).UnixMilli()})
	if timeStats.TotalCount != 1 {
		t.Errorf("时间筛选聚合不符: %+v", timeStats)
	}
}

// TestUsageCounts 频次聚合：success/failed/timeout 计入，canceled 不计入，
// FunctionID 为空的记录跳过；同功能多条累加
func TestUsageCounts(t *testing.T) {
	svc := newHistorySvc(t)
	base := time.Now()
	_, _ = svc.Archive(makeEntry("u1", base.Add(-3*time.Hour), "success", 0), "")
	_, _ = svc.Archive(makeEntry("u2", base.Add(-2*time.Hour), "success", 0), "")
	_, _ = svc.Archive(makeEntry("u3", base.Add(-time.Hour), "failed", 0), "")
	_, _ = svc.Archive(makeEntry("u4", base.Add(-time.Hour), "timeout", 0), "")
	// canceled 不计入
	_, _ = svc.Archive(makeEntry("u5", base.Add(-time.Hour), "canceled", 0), "")
	// FunctionID 为空跳过
	empty := makeEntry("u6", base, "success", 0)
	empty.FunctionID = ""
	_, _ = svc.Archive(empty, "")

	counts, err := svc.UsageCounts()
	if err != nil {
		t.Fatalf("UsageCounts 失败: %v", err)
	}
	// 逐项断言：每功能各 1 次（success/failed/timeout 各计入），canceled 与空 FunctionID 不产生键
	for _, id := range []string{"fn-u1", "fn-u2", "fn-u3", "fn-u4"} {
		if counts[id] != 1 {
			t.Errorf("%s 应计 1 次, got %d", id, counts[id])
		}
	}
	if _, ok := counts["fn-u5"]; ok {
		t.Errorf("canceled 不应计入频次: %v", counts)
	}
	if len(counts) != 4 {
		t.Errorf("应恰好 4 个功能项计数, got %d: %v", len(counts), counts)
	}
}

// TestUsageCounts_MultiSameFunction 同功能多条记录累加
func TestUsageCounts_MultiSameFunction(t *testing.T) {
	svc := newHistorySvc(t)
	base := time.Now()
	for i := 0; i < 3; i++ {
		e := makeEntry("m1", base.Add(-time.Duration(i)*time.Minute), "success", 0)
		e.FunctionID = "fn-same"
		_, _ = svc.Archive(e, "")
	}
	counts, err := svc.UsageCounts()
	if err != nil {
		t.Fatalf("UsageCounts 失败: %v", err)
	}
	if counts["fn-same"] != 3 {
		t.Errorf("同功能应累加为 3 次, got %d", counts["fn-same"])
	}
}

// TestExportCSV BOM 头、表头、字段转义（含逗号的功能名）
func TestExportCSV(t *testing.T) {
	svc := newHistorySvc(t)
	now := time.Now()
	_, _ = svc.Archive(makeEntryWithMetrics("c1", "fn-x", "导出,测试", now, "success",
		&model.AiTaskUsage{InputTokens: 10, OutputTokens: 5}, 0.01, 2000), "")

	csv, err := svc.ExportCSV(nil)
	if err != nil {
		t.Fatalf("ExportCSV 失败: %v", err)
	}
	// UTF-8 BOM 开头（U+FEFF）
	if !strings.HasPrefix(csv, string(rune(0xFEFF))) {
		t.Error("CSV 应以 UTF-8 BOM 开头")
	}
	// 表头
	if !strings.Contains(csv, "时间,功能,状态,耗时ms,成本USD,入token,出token,输出大小") {
		t.Errorf("CSV 表头不符: %s", csv)
	}
	// 含逗号的功能名被引号包裹
	if !strings.Contains(csv, `"导出,测试"`) {
		t.Errorf("含逗号功能名应转义: %s", csv)
	}
	// 数据列
	if !strings.Contains(csv, ",success,2000,") {
		t.Errorf("耗时/状态列不符: %s", csv)
	}
}

// TestExportMarkdown 含统计摘要表与功能排行表关键行
func TestExportMarkdown(t *testing.T) {
	svc := newHistorySvc(t)
	now := time.Now()
	_, _ = svc.Archive(makeEntryWithMetrics("d1", "fn-md", "周报生成", now, "success",
		&model.AiTaskUsage{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 10, CacheCreationInputTokens: 5}, 0.02, 30000), "")
	_, _ = svc.Archive(makeEntry("d2", now.Add(-time.Minute), "failed", 0), "")

	md, err := svc.ExportMarkdown(nil)
	if err != nil {
		t.Fatalf("ExportMarkdown 失败: %v", err)
	}
	// 标题与三个表
	for _, want := range []string{"# AI 任务历史报告", "## 统计摘要", "## 功能排行", "## 明细", "周报生成"} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown 缺少关键内容 %q:\n%s", want, md)
		}
	}
	// 摘要含次数与成功数
	if !strings.Contains(md, "| 运行次数 | 2（成功 1） |") {
		t.Errorf("摘要运行次数不符:\n%s", md)
	}
}

// TestCsvEscape CSV 字段转义：逗号/引号包裹、内部引号翻倍
func TestCsvEscape(t *testing.T) {
	if got := csvEscape("普通"); got != "普通" {
		t.Errorf("普通字段不应转义: %s", got)
	}
	if got := csvEscape("a,b"); got != `"a,b"` {
		t.Errorf("含逗号应引号包裹: %s", got)
	}
	if got := csvEscape(`说"好"`); got != `"说""好"""` {
		t.Errorf("含引号应翻倍: %s", got)
	}
}
