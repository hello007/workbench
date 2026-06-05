// service/content_search_test.go
package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
