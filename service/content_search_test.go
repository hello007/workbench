// service/content_search_test.go
package service

import (
	"bytes"
	"context"
	"errors"
	"os"
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
func writeExitOneRgStub(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(t.TempDir(), "rg.bat")
		if err := os.WriteFile(path, []byte("@exit 1\r\n"), 0o644); err != nil {
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

// TestSearchWithRipgrep_TimeoutReturnsSearchTimeout rg 超时被杀须返回
// E_SEARCH_TIMEOUT 引导级错误而非 nil,nil 静默「无匹配」。预取消 ctx 下
// cmd.Run 必返非 nil err：rg 存在时进程创建后被 Kill，Wait 阶段返 ctx
// 错误；rg 缺失时 Start 前返 exec.ErrNotFound——两路径均命中 ctx.Err()
// 判定，rg 二进制是否真实存在不影响覆盖。
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
	svc := &ContentSearchService{rgAvailable: true, rgPath: writeExitOneRgStub(t)}
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
