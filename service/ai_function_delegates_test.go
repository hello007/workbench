package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
)

// newAiSvcForTest 构造指向临时 data 目录的 AiFunctionService。
// ctx 传 nil：测试环境无 Wails 上下文，emit 检查 ctx==nil 后直接返回，避免触发 runtime.EventsEmit fatal。
func newAiSvcForTest(t *testing.T) *AiFunctionService {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "ai_functions.json")
	return NewAiFunctionService(nil, configPath)
}

// archiveOne 归档一条历史（含输出文件），返回归档后的元数据。
func archiveOne(t *testing.T, svc *AiFunctionService, id string, status string) *model.AiTaskHistory {
	t.Helper()
	outputDir := filepath.Join(svc.dataDir(), aiTaskOutputDirName)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("mkdir output dir: %v", err)
	}
	srcPath := filepath.Join(outputDir, id+".txt")
	wantOutput := "任务输出内容\n多行"
	if err := os.WriteFile(srcPath, []byte(wantOutput), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}
	entry := makeEntry(id, time.Now(), status, int64(len(wantOutput)))
	archived, err := svc.historySvc.Archive(entry, srcPath)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	return archived
}

// TestAiFunctionDelegates_NilHistorySvc historySvc 未初始化时各委托返回空值不 panic。
func TestAiFunctionDelegates_NilHistorySvc(t *testing.T) {
	svc := &AiFunctionService{} // historySvc=nil

	list, err := svc.GetAiTaskHistory(nil)
	if err != nil || len(list) != 0 {
		t.Errorf("nil history GetAiTaskHistory: got %v, %v", list, err)
	}

	if _, err := svc.GetAiTaskHistoryOutput("x"); err == nil {
		t.Error("nil history GetAiTaskHistoryOutput 应返回错误")
	}

	if svc.DeleteAiTaskHistory("x") {
		t.Error("nil history DeleteAiTaskHistory 应返回 false")
	}

	if n, _ := svc.ClearAiTaskHistory(nil); n != 0 {
		t.Errorf("nil history ClearAiTaskHistory 应返回 0, got %d", n)
	}

	stats, _ := svc.GetAiTaskHistoryStats(nil)
	if stats == nil || stats.TotalCount != 0 {
		t.Errorf("nil history Stats 应返回空统计, got %+v", stats)
	}

	counts, _ := svc.GetFunctionUsageCounts()
	if len(counts) != 0 {
		t.Errorf("nil history UsageCounts 应返回空, got %v", counts)
	}

	if _, err := svc.ExportAiTaskHistoryCSV(nil); err == nil {
		t.Error("nil history ExportCSV 应返回错误")
	}
	if _, err := svc.ExportAiTaskHistoryMarkdown(nil); err == nil {
		t.Error("nil history ExportMarkdown 应返回错误")
	}
}

// TestAiFunctionDelegates_WithHistory 归档后委托函数返回真实数据。
func TestAiFunctionDelegates_WithHistory(t *testing.T) {
	svc := newAiSvcForTest(t)
	archiveOne(t, svc, "t1", "success")
	archiveOne(t, svc, "t2", "failed")

	// GetAiTaskHistory 返回 2 条
	list, err := svc.GetAiTaskHistory(nil)
	if err != nil {
		t.Fatalf("GetAiTaskHistory: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("list len: got %d, want 2", len(list))
	}

	// GetAiTaskHistoryOutput 读取归档输出
	out, err := svc.GetAiTaskHistoryOutput("t1")
	if err != nil {
		t.Fatalf("GetAiTaskHistoryOutput: %v", err)
	}
	if out == "" {
		t.Error("输出不应为空")
	}

	// GetAiTaskHistoryStats 汇总
	stats, err := svc.GetAiTaskHistoryStats(nil)
	if err != nil {
		t.Fatalf("GetAiTaskHistoryStats: %v", err)
	}
	if stats.TotalCount != 2 {
		t.Errorf("stats TotalCount: got %d, want 2", stats.TotalCount)
	}

	// GetFunctionUsageCounts 各功能次数
	counts, err := svc.GetFunctionUsageCounts()
	if err != nil {
		t.Fatalf("GetFunctionUsageCounts: %v", err)
	}
	if counts["fn-t1"] != 1 || counts["fn-t2"] != 1 {
		t.Errorf("usage counts: got %v", counts)
	}

	// ExportAiTaskHistoryCSV 非空
	csv, err := svc.ExportAiTaskHistoryCSV(nil)
	if err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	if csv == "" {
		t.Error("CSV 不应为空")
	}

	// ExportAiTaskHistoryMarkdown 非空
	md, err := svc.ExportAiTaskHistoryMarkdown(nil)
	if err != nil {
		t.Fatalf("ExportMarkdown: %v", err)
	}
	if md == "" {
		t.Error("Markdown 不应为空")
	}
}

// TestAiFunctionDelegates_DeleteAndClear 删除/清理后列表缩减。
func TestAiFunctionDelegates_DeleteAndClear(t *testing.T) {
	svc := newAiSvcForTest(t)
	archiveOne(t, svc, "t1", "success")
	archiveOne(t, svc, "t2", "success")
	archiveOne(t, svc, "t3", "success")

	// 删除 t1，剩 2 条
	if !svc.DeleteAiTaskHistory("t1") {
		t.Error("DeleteAiTaskHistory 应返回 true")
	}
	list, _ := svc.GetAiTaskHistory(nil)
	if len(list) != 2 {
		t.Errorf("删除后应剩 2 条, got %d", len(list))
	}

	// KeepRecent=1：剩 2 条保留 1 条清理 1 条
	n, err := svc.ClearAiTaskHistory(&model.AiTaskHistoryClearCriteria{KeepRecent: 1})
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if n != 1 {
		t.Errorf("Clear 应清理 1 条, got %d", n)
	}
	list, _ = svc.GetAiTaskHistory(nil)
	if len(list) != 1 {
		t.Errorf("清理后应剩 1 条, got %d", len(list))
	}
}

// TestAiFunctionDelegates_FilterByFunction 按功能项筛选。
func TestAiFunctionDelegates_FilterByFunction(t *testing.T) {
	svc := newAiSvcForTest(t)
	archiveOne(t, svc, "t1", "success")
	archiveOne(t, svc, "t2", "success")

	list, _ := svc.GetAiTaskHistory(&model.AiTaskHistoryFilter{FunctionID: "fn-t1"})
	if len(list) != 1 || list[0].ID != "t1" {
		t.Errorf("按功能筛选应只返回 t1, got %v", list)
	}
}

// TestEnsureOutputDir_CreatesDir 调用后输出目录应被创建。
func TestEnsureOutputDir_CreatesDir(t *testing.T) {
	svc := newAiSvcForTest(t)
	dir, err := svc.ensureOutputDir()
	if err != nil {
		t.Fatalf("ensureOutputDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Errorf("输出目录应已创建: %s", dir)
	}
}

// TestBackupConfig_WritesFile 备份原配置内容到 .bak 时间戳文件。
func TestBackupConfig_WritesFile(t *testing.T) {
	svc := newAiSvcForTest(t)
	raw := []byte("原配置内容")
	svc.backupConfig(raw)

	matches, err := filepath.Glob(svc.configPath + ".bak.*")
	if err != nil || len(matches) == 0 {
		t.Fatalf("应生成备份文件, matches=%v err=%v", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("读备份文件: %v", err)
	}
	if string(data) != "原配置内容" {
		t.Errorf("备份内容不符: %q", data)
	}
}

// TestLoadFunction_Found 加载默认 seed 后 loadFunction 能定位到功能项。
func TestLoadFunction_Found(t *testing.T) {
	svc := newAiSvcForTest(t)
	defaults, err := svc.LoadAiFunctions()
	if err != nil || len(defaults) == 0 {
		t.Fatalf("LoadAiFunctions 默认 seed: err=%v len=%d", err, len(defaults))
	}
	fn, err := svc.loadFunction(defaults[0].ID)
	if err != nil {
		t.Fatalf("loadFunction: %v", err)
	}
	if fn.ID != defaults[0].ID {
		t.Errorf("loadFunction 返回 ID: got %s, want %s", fn.ID, defaults[0].ID)
	}
}

// TestLoadFunction_NotFound 不存在 id 返回错误。
func TestLoadFunction_NotFound(t *testing.T) {
	svc := newAiSvcForTest(t)
	// 先加载默认 seed（确保配置文件存在且有数据）
	if _, err := svc.LoadAiFunctions(); err != nil {
		t.Fatalf("LoadAiFunctions: %v", err)
	}
	if _, err := svc.loadFunction("nonexistent-id-xyz"); err == nil {
		t.Error("不存在 id 应返回错误")
	}
}

// TestArchiveTask_ArchivesToHistory archiveTask 将 runtime 元数据归档到历史服务。
func TestArchiveTask_ArchivesToHistory(t *testing.T) {
	svc := newAiSvcForTest(t)
	task := &aiTaskRuntime{
		id:         "arch-1",
		functionID: "fn-arch",
		prompt:     "测试 prompt",
		startedAt:  time.Now(),
		finishedAt: time.Now(),
	}
	svc.archiveTask(task, model.AiTaskRunResult{ExitCode: 0}, "success")

	list, _ := svc.GetAiTaskHistory(nil)
	if len(list) != 1 {
		t.Fatalf("应归档 1 条, got %d", len(list))
	}
	if list[0].ID != "arch-1" {
		t.Errorf("归档 ID: got %s, want arch-1", list[0].ID)
	}
	if list[0].Status != "success" {
		t.Errorf("Status: got %s, want success", list[0].Status)
	}
}

// TestArchiveTask_NilHistorySvc historySvc 为 nil 时直接返回不归档、不 panic。
func TestArchiveTask_NilHistorySvc(t *testing.T) {
	svc := &AiFunctionService{} // historySvc=nil
	task := &aiTaskRuntime{id: "x", functionID: "fn", startedAt: time.Now(), finishedAt: time.Now()}
	svc.archiveTask(task, model.AiTaskRunResult{}, "success")
}

// TestCancelAiTask_NotExists 不存在任务返回 false。
func TestCancelAiTask_NotExists(t *testing.T) {
	svc := newAiSvcForTest(t)
	if svc.CancelAiTask("nonexistent-id") {
		t.Error("不存在任务应返回 false")
	}
}

// TestGetAiTaskState_NotExists 不存在任务返回 nil。
func TestGetAiTaskState_NotExists(t *testing.T) {
	svc := newAiSvcForTest(t)
	if got := svc.GetAiTaskState("nonexistent-id"); got != nil {
		t.Error("不存在任务应返回 nil")
	}
}

// TestCloseAll_Empty 空 tasks 时 CloseAll 不 panic。
func TestCloseAll_Empty(t *testing.T) {
	svc := newAiSvcForTest(t)
	svc.CloseAll()
}

// TestOutputSnapshot_NilOutputFile 无输出文件时 preview 为空、relFile 为空。
func TestOutputSnapshot_NilOutputFile(t *testing.T) {
	svc := newAiSvcForTest(t)
	task := &aiTaskRuntime{id: "x", functionID: "fn", startedAt: time.Now(), finishedAt: time.Now()}
	preview, _, relFile := svc.outputSnapshot(task)
	if preview != "" {
		t.Errorf("nil outputFile preview 应为空, got %s", preview)
	}
	if relFile != "" {
		t.Errorf("outputPath 为空时 relFile 应为空, got %s", relFile)
	}
}

// TestRelativeOutputFile_EmptyPath outputPath 为空返回空串。
func TestRelativeOutputFile_EmptyPath(t *testing.T) {
	svc := newAiSvcForTest(t)
	task := &aiTaskRuntime{outputPath: ""}
	if got := svc.relativeOutputFile(task); got != "" {
		t.Errorf("空 outputPath 应返回空串, got %s", got)
	}
}

// TestRelativeOutputFile_NormalPath 正常路径返回相对 data 目录的路径。
func TestRelativeOutputFile_NormalPath(t *testing.T) {
	svc := newAiSvcForTest(t)
	// outputPath 在 data 目录下
	outPath := filepath.Join(svc.dataDir(), aiTaskOutputDirName, "t1.txt")
	task := &aiTaskRuntime{outputPath: outPath}
	got := svc.relativeOutputFile(task)
	want := filepath.Join(aiTaskOutputDirName, "t1.txt")
	if got != want {
		t.Errorf("relativeOutputFile: got %s, want %s", got, want)
	}
}

// TestExportAiFunctions 导出默认 seed 为非空 JSON 文本。
func TestExportAiFunctions(t *testing.T) {
	svc := newAiSvcForTest(t)
	data, err := svc.ExportAiFunctions()
	if err != nil {
		t.Fatalf("ExportAiFunctions: %v", err)
	}
	if data == "" {
		t.Error("导出不应为空")
	}
}

// TestImportAiFunctions_Valid 导入有效配置返回预览（默认 seed 的功能项 ID 已存在，归入 Conflict）。
func TestImportAiFunctions_Valid(t *testing.T) {
	svc := newAiSvcForTest(t)
	data, err := svc.ExportAiFunctions()
	if err != nil {
		t.Fatalf("ExportAiFunctions: %v", err)
	}
	preview, err := svc.ImportAiFunctions(data)
	if err != nil {
		t.Fatalf("ImportAiFunctions: %v", err)
	}
	if preview == nil {
		t.Fatal("preview 不应为 nil")
	}
	// 默认 seed 功能项已存在，应归入 Conflict
	if len(preview.Conflict) == 0 && len(preview.New) == 0 {
		t.Error("Conflict 或 New 应至少有一项非空")
	}
}

// TestImportAiFunctions_EmptyJSON 空内容走自愈回种，不报错。
func TestImportAiFunctions_EmptyJSON(t *testing.T) {
	svc := newAiSvcForTest(t)
	_, err := svc.ImportAiFunctions("")
	if err != nil {
		t.Errorf("空内容不应报错, got %v", err)
	}
}

// TestValidateFunctions 覆盖 nil 项、关键字段空、有效项各分支。
func TestValidateFunctions(t *testing.T) {
	funcs := []*model.AiFunction{
		nil,
		{ID: "", Name: "x", Command: "/c", Cwd: "/d"},   // ID 空
		{ID: "f1", Name: "", Command: "/c", Cwd: "/d"},  // Name 空
		{ID: "f2", Name: "n", Command: "/c", Cwd: "/d"}, // 有效
	}
	valid, invalidIDs := validateFunctions(funcs)
	if len(valid) != 1 || valid[0].ID != "f2" {
		t.Errorf("valid 应只含 f2, got %v", valid)
	}
	if len(invalidIDs) != 3 {
		t.Errorf("invalidIDs 数: got %d, want 3", len(invalidIDs))
	}
}

// TestBuildPromptPreview_OverLimit 超限 prompt 被截断（结果短于原串）。
func TestBuildPromptPreview_OverLimit(t *testing.T) {
	long := strings.Repeat("a", 300)
	got := buildPromptPreview(long, 100)
	if len(got) >= len(long) {
		t.Errorf("超限应截断, got len=%d", len(got))
	}
}

// TestRunStage_FunctionNotFound 功能项不存在时返回错误（不启动子进程）。
func TestRunStage_FunctionNotFound(t *testing.T) {
	svc := newAiSvcForTest(t)
	if _, err := svc.RunStage("nonexistent-fn-xyz", "prompt", ""); err == nil {
		t.Error("不存在功能应返回错误")
	}
}

// TestRunStage_EmptyPrompt prompt 为空时返回错误。
func TestRunStage_EmptyPrompt(t *testing.T) {
	svc := newAiSvcForTest(t)
	defaults, err := svc.LoadAiFunctions()
	if err != nil || len(defaults) == 0 {
		t.Fatalf("LoadAiFunctions: err=%v len=%d", err, len(defaults))
	}
	if _, err := svc.RunStage(defaults[0].ID, "   ", ""); err == nil {
		t.Error("空 prompt 应返回错误")
	}
}

// TestJoinParamLines 多行参数整理为单行。
func TestJoinParamLines_SingleValue(t *testing.T) {
	got := joinParamLines("单行值")
	if got != "单行值" {
		t.Errorf("单值应原样: got %s", got)
	}
}

// TestJoinParamLines_MultiValues 多行值用双引号包裹以空格连接。
func TestJoinParamLines_MultiValues(t *testing.T) {
	got := joinParamLines("path1\npath2\npath3")
	if !strings.Contains(got, "path1") || !strings.Contains(got, "path3") {
		t.Errorf("多值应包含各路径: got %s", got)
	}
}
