// service/content_search_test.go
package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"workbench/model"
)

func TestSearchWithGo_BasicMatch(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "hello world\nfoo bar\nhello golang\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"hello",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].LineNum != 1 {
		t.Errorf("expected line 1, got %d", results[0].LineNum)
	}
	if results[1].LineNum != 3 {
		t.Errorf("expected line 3, got %d", results[1].LineNum)
	}
}

func TestSearchWithGo_FileExtFilter(t *testing.T) {
	tmpDir := t.TempDir()
	javaFile := filepath.Join(tmpDir, "App.java")
	xmlFile := filepath.Join(tmpDir, "config.xml")
	os.WriteFile(javaFile, []byte("public class App {}\n"), 0644)
	os.WriteFile(xmlFile, []byte("<config>App</config>\n"), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"App",
		".java",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result (only .java), got %d", len(results))
	}
}

func TestSearchWithGo_ExcludeDirs(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "node_modules")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "package.js"), []byte("var config = {};\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "main.js"), []byte("var config = {};\n"), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"config",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result (node_modules excluded), got %d", len(results))
	}
}

func TestSearchWithGo_SkipBinary(t *testing.T) {
	tmpDir := t.TempDir()
	binFile := filepath.Join(tmpDir, "data.bin")
	os.WriteFile(binFile, []byte("hello\x00world\n"), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"hello",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 0 {
		t.Fatalf("expected 0 results (binary skipped), got %d", len(results))
	}
}

func TestSearchWithGo_MaxResults(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := ""
	for i := 0; i < 10; i++ {
		content += "keyword match line\n"
	}
	os.WriteFile(testFile, []byte(content), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"keyword",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		3,
	)

	if len(results) > 3 {
		t.Fatalf("expected at most 3 results, got %d", len(results))
	}
}

func TestSearchWithGo_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("Hello World\nHELLO WORLD\nhello world\n"), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"hello",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 3 {
		t.Fatalf("expected 3 results (case insensitive), got %d", len(results))
	}
}

func TestIsExcludedDir(t *testing.T) {
	if !isExcludedDir("node_modules", defaultExcludeDirs) {
		t.Error("node_modules should be excluded")
	}
	if !isExcludedDir(".git", defaultExcludeDirs) {
		t.Error(".git should be excluded")
	}
	if isExcludedDir("src", defaultExcludeDirs) {
		t.Error("src should not be excluded")
	}
}

func TestIsExcludedFile(t *testing.T) {
	if !isExcludedFile("app.log", defaultExcludeFiles) {
		t.Error("app.log should be excluded")
	}
	if !isExcludedFile("App.class", defaultExcludeFiles) {
		t.Error("App.class should be excluded")
	}
	if isExcludedFile("App.java", defaultExcludeFiles) {
		t.Error("App.java should not be excluded")
	}
}

// TestSearchFileContent_LargeFile 超 10MB 文件跳过不搜索。
func TestSearchFileContent_LargeFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.txt")
	big := bytes.Repeat([]byte("a"), 10*1024*1024+1)
	if err := os.WriteFile(p, big, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := searchFileContent(p, "a", int64(len(big)))
	if len(got) != 0 {
		t.Errorf("大文件应跳过, got %d results", len(got))
	}
}

// TestSearchFileContent_LongLineTruncated 超长匹配行截断为 300 字符 + "..."。
func TestSearchFileContent_LongLineTruncated(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	longLine := strings.Repeat("a", 400) + "keyword"
	if err := os.WriteFile(p, []byte(longLine), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := searchFileContent(p, "keyword", int64(len(longLine)))
	if len(got) != 1 {
		t.Fatalf("应 1 条结果, got %d", len(got))
	}
	if !strings.HasSuffix(got[0].LineText, "...") {
		t.Errorf("长行应截断以 ... 结尾, got len=%d", len(got[0].LineText))
	}
}

// TestSearchFileContent_NoMatch 无匹配返回空切片。
func TestSearchFileContent_NoMatch(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(p, []byte("hello world\n"), 0o644)
	got := searchFileContent(p, "nonexistent", 12)
	if len(got) != 0 {
		t.Errorf("无匹配应返回空, got %d", len(got))
	}
}

// TestSearchFileContent_NotExists 文件不存在返回空（ReadFile 失败）。
func TestSearchFileContent_NotExists(t *testing.T) {
	got := searchFileContent(filepath.Join(t.TempDir(), "missing.txt"), "x", 100)
	if len(got) != 0 {
		t.Errorf("不存在文件应返回空, got %d", len(got))
	}
}

func TestContentSearch_MultipleDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("findme in dir1\n"), 0644)
	os.WriteFile(filepath.Join(dir2, "b.txt"), []byte("findme in dir2\n"), 0644)

	svc := NewContentSearchService()
	// 强制使用 Go 原生搜索，避免依赖 ripgrep
	svc.rgAvailable = false
	groups, err := svc.ContentSearch(
		context.Background(),
		[]string{dir1, dir2},
		[]string{"repo1", "repo2"},
		"findme",
		"", "",
		nil, nil,
		20,
	)

	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	for _, g := range groups {
		if g.TimedOut {
			t.Errorf("无超时态正常组不应携带 TimedOut 标志, group %s", g.RepoName)
		}
	}
}

// cancelledCtx 返回已取消的 ctx（无 sleep、无慢命令）。
func cancelledCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx, cancel
}

// writeExitOneRgStub 写恒 exit 1 的 rg stub 脚本，模拟 rg 无匹配退出码。
// 不依赖环境真实 rg（CI 无 rg 安装步骤，真实 rg 用例曾恒 Skip 致
// 「ctx 未超时 exit 1 → nil,nil」核心约束无守护）。
//
// Windows 分支首行 @echo ok 供探针校验 stub 真被执行：TEMP 路径含空格时
// cmd.exe 包装层会在空格处截断路径报 not recognized（退出码恰为 1 与
// 无匹配同值），stub 未被执行时用例会假绿——调用方须先跑探针显式甄别。
func writeExitOneRgStub(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(t.TempDir(), "rg.bat")
		if err := os.WriteFile(path, []byte("@echo ok\r\n@exit 1\r\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := filepath.Join(t.TempDir(), "rg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// probeRgStub 校验 stub 真被执行且退出码透传（宁红勿假绿）：执行 stub
// 断言 stdout 含标记行且 exit 1。路径含空格等环境下 stub 未被执行时
// 显式失败，防守护静默失效。
func probeRgStub(t *testing.T, stubPath string) {
	t.Helper()
	probe := exec.Command(stubPath)
	// exit 1 时 Output 返回 ExitError，退出码经 ProcessState 读取，err 无需消费
	out, _ := probe.Output()
	exitCode := 0
	if probe.ProcessState != nil {
		exitCode = probe.ProcessState.ExitCode()
	}
	if !strings.Contains(string(out), "ok") || exitCode != 1 {
		t.Fatalf("rg stub 未按预期执行（stdout=%q exit=%d），TEMP 路径含空格等环境会使 .bat 截断失效", string(out), exitCode)
	}
}

// TestSearchWithRipgrep_TimeoutReturnsSearchTimeout rg 超时被杀须返回
// E_SEARCH_TIMEOUT 引导级错误而非 nil,nil 静默「无匹配」。预取消 ctx 下
// cmd.Run 必返非 nil err：Go 1.19+ Cmd.start 在 os.StartProcess 前有显式
// ctx.Err() 检查，Start 前直接返回 context.Canceled（进程不创建）；
// rg 缺失时 LookPath 失败 ErrNotFound 先于 ctx 检查返回——两路径均命中
// 生产代码 ctx.Err() 判定，rg 二进制是否真实存在不影响覆盖。
func TestSearchWithRipgrep_TimeoutReturnsSearchTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("findme\n"), 0644)

	svc := &ContentSearchService{rgAvailable: true, rgPath: "rg"}
	ctx, cancel := cancelledCtx()
	defer cancel()

	items, err := svc.searchWithRipgrep(ctx, tmpDir, "findme", "", defaultExcludeDirs, defaultExcludeFiles, 20)
	if items != nil {
		t.Fatalf("超时应无结果, got %d items", len(items))
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("超时应返回 AppError, got %v", err)
	}
	if appErr.Code != model.ErrCodeSearchTimeout {
		t.Errorf("错误码应为 %s, got %s", model.ErrCodeSearchTimeout, appErr.Code)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("根因应穿透为 context.Canceled, got %v", err)
	}
}

// TestSearchWithRipgrep_NoMatchUnchanged ctx 未超时时 exit 1 无匹配语义
// 逐字不变：仍返回 nil, nil。用恒 exit 1 的 rg stub 模拟无匹配退出码，
// CI（无 rg）也能守护该核心约束。
func TestSearchWithRipgrep_NoMatchUnchanged(t *testing.T) {
	stubPath := writeExitOneRgStub(t)
	probeRgStub(t, stubPath)
	svc := &ContentSearchService{rgAvailable: true, rgPath: stubPath}
	items, err := svc.searchWithRipgrep(
		context.Background(),
		t.TempDir(),
		"keyword-that-never-exists",
		"", defaultExcludeDirs, defaultExcludeFiles, 20,
	)
	if err != nil {
		t.Fatalf("exit 1 无匹配不应报错, got %v", err)
	}
	if items != nil {
		t.Fatalf("无匹配应返回 nil, got %d items", len(items))
	}
}

// TestContentSearch_RgFailureFallsBackToGo rg 非超时真失败（二进制不存在）
// 且 ctx 未超时时，ContentSearch 层保持回退 searchWithGo 既有语义，结果
// 正常返回——d8b8055 控制流重排（单行 if 改嵌套两分支）的回退分支守护。
func TestContentSearch_RgFailureFallsBackToGo(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("findme in dir\n"), 0644)

	svc := &ContentSearchService{rgAvailable: true, rgPath: "rg-not-exists-anywhere"}
	groups, err := svc.ContentSearch(
		context.Background(),
		[]string{tmpDir},
		[]string{"repo1"},
		"findme", "", "",
		nil, nil, 20,
	)
	if err != nil {
		t.Fatalf("rg 失败应回退 Go 路径正常返回, got %v", err)
	}
	if len(groups) != 1 || len(groups[0].Items) != 1 {
		t.Fatalf("回退后应返回搜索结果, got %d groups", len(groups))
	}
	if got := groups[0].Items[0].FilePath; got != "a.txt" {
		t.Errorf("回退结果 FilePath 应归一化为相对路径 a.txt, got %q", got)
	}
	if got := groups[0].Items[0].RepoName; got != "repo1" {
		t.Errorf("回退结果 RepoName 应补全为 repo1, got %q", got)
	}
}

// TestContentSearch_TimeoutReturnsSearchTimeout Go 原生回退路径对称收口：
// 已超时 ctx 下 searchWithGo 中途 SkipAll 的部分结果不可信，目录计超时；
// 全部目录超时且结果为空时返回 E_SEARCH_TIMEOUT 而非 nil,nil。
func TestContentSearch_TimeoutReturnsSearchTimeout(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("findme in dir1\n"), 0644)
	os.WriteFile(filepath.Join(dir2, "b.txt"), []byte("findme in dir2\n"), 0644)

	svc := &ContentSearchService{rgAvailable: false}
	ctx, cancel := cancelledCtx()
	defer cancel()

	groups, err := svc.ContentSearch(
		ctx,
		[]string{dir1, dir2},
		[]string{"repo1", "repo2"},
		"findme",
		"", "",
		nil, nil,
		20,
	)

	if len(groups) != 0 {
		t.Fatalf("超时不应返回结果, got %d groups", len(groups))
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("全部目录超时应返回 AppError, got %v", err)
	}
	if appErr.Code != model.ErrCodeSearchTimeout {
		t.Errorf("错误码应为 %s, got %s", model.ErrCodeSearchTimeout, appErr.Code)
	}
}

// TestMergeTimeoutGroups_NoTimeoutPassthrough 无超时（timedOutGroups 空）时
// 结果组原样透传，不新增标志组——正常路径零干扰。
func TestMergeTimeoutGroups_NoTimeoutPassthrough(t *testing.T) {
	normal := []*model.ContentSearchGroup{
		{RepoName: "repo1", RepoPath: "/r1", Items: []*model.ContentSearchResult{{FilePath: "a.txt"}}},
	}
	got, err := mergeTimeoutGroups(normal, nil)
	if err != nil {
		t.Fatalf("无超时应无错误, got %v", err)
	}
	if len(got) != 1 || got[0].RepoName != "repo1" {
		t.Fatalf("结果组应原样透传, got %d groups", len(got))
	}
	if got[0].TimedOut {
		t.Error("透传组不应携带 TimedOut 标志")
	}
}

// TestMergeTimeoutGroups_AllTimeoutReturnsError 全部目录超时且结果为空时
// 返回 E_SEARCH_TIMEOUT 引导级错误（报错路径语义与收口前逐字一致）。
func TestMergeTimeoutGroups_AllTimeoutReturnsError(t *testing.T) {
	timedOut := []*model.ContentSearchGroup{
		{RepoName: "repo1", RepoPath: "/r1", TimedOut: true},
		{RepoName: "repo2", RepoPath: "/r2", TimedOut: true},
	}
	got, err := mergeTimeoutGroups(nil, timedOut)
	if got != nil {
		t.Fatalf("全部超时应无结果, got %d groups", len(got))
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("全部超时应返回 AppError, got %v", err)
	}
	if appErr.Code != model.ErrCodeSearchTimeout {
		t.Errorf("错误码应为 %s, got %s", model.ErrCodeSearchTimeout, appErr.Code)
	}
}

// TestMergeTimeoutGroups_NormalDirsZeroMatchMixedTimeout 混合形态守护：部分
// 目录正常完成但零命中不产组 + 其余超时 → 正常结果组为空，仍报 E_SEARCH_TIMEOUT
//（判定条件与收口前逐字一致，不因「非全部目录超时」误返空结果）。
func TestMergeTimeoutGroups_NormalDirsZeroMatchMixedTimeout(t *testing.T) {
	timedOut := []*model.ContentSearchGroup{
		{RepoName: "repo1", RepoPath: "/r1", TimedOut: true},
	}
	got, err := mergeTimeoutGroups(nil, timedOut)
	if got != nil {
		t.Fatalf("超时且无正常结果组应无结果, got %d groups", len(got))
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("应返回 AppError, got %v", err)
	}
	if appErr.Code != model.ErrCodeSearchTimeout {
		t.Errorf("错误码应为 %s, got %s", model.ErrCodeSearchTimeout, appErr.Code)
	}
}

// TestMergeTimeoutGroups_PartialTimeoutMergesFlaggedGroups 部分超时（正常组
// 与标志组并存）时标志组尾部合并进结果组：正常组在前、标志组在后且 TimedOut
// 为 true、Items 恒空——前端据此渲染「结果可能不完整」warning 行。
func TestMergeTimeoutGroups_PartialTimeoutMergesFlaggedGroups(t *testing.T) {
	normal := []*model.ContentSearchGroup{
		{RepoName: "repo1", RepoPath: "/r1", Items: []*model.ContentSearchResult{{FilePath: "a.txt"}}},
	}
	timedOut := []*model.ContentSearchGroup{
		{RepoName: "repo2", RepoPath: "/r2", TimedOut: true},
	}
	got, err := mergeTimeoutGroups(normal, timedOut)
	if err != nil {
		t.Fatalf("部分超时应返回结果, got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应合并为 2 组, got %d", len(got))
	}
	if got[0].RepoName != "repo1" || got[0].TimedOut {
		t.Errorf("正常组应在前且无标志, got %+v", got[0])
	}
	if got[1].RepoName != "repo2" || !got[1].TimedOut {
		t.Errorf("标志组应在后且 TimedOut 为 true, got %+v", got[1])
	}
	if len(got[1].Items) != 0 {
		t.Errorf("标志组 Items 应恒空, got %d items", len(got[1].Items))
	}
}
