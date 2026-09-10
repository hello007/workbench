package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createSearchTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	dirs := []string{"src", "src/components", "src/utils", "docs"}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(root, d), 0755)
	}

	files := []string{
		"src/main.go",
		"src/components/Button.vue",
		"src/components/Modal.vue",
		"src/utils/helper.js",
		"docs/README.md",
	}
	for _, f := range files {
		os.WriteFile(filepath.Join(root, f), []byte("test"), 0644)
	}
	return root
}

func TestSearchFiles_ByName(t *testing.T) {
	root := createSearchTestDir(t)
	svc := NewSearchService()

	results, err := svc.Search(root, "Button", 20)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Expected at least one result")
	}
	if results[0].Name != "Button.vue" {
		t.Errorf("Expected Button.vue, got %s", results[0].Name)
	}
}

func TestSearchFiles_Fuzzy(t *testing.T) {
	root := createSearchTestDir(t)
	svc := NewSearchService()

	results, err := svc.Search(root, "hlp", 20)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Expected fuzzy match for helper.js")
	}
}

func TestSearchFiles_MaxResults(t *testing.T) {
	root := createSearchTestDir(t)
	svc := NewSearchService()

	results, err := svc.Search(root, "", 2)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("Expected max 2 results, got %d", len(results))
	}
}

func TestSearchFiles_EmptyDir(t *testing.T) {
	root := t.TempDir()
	svc := NewSearchService()

	results, err := svc.Search(root, "anything", 20)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty dir, got %d", len(results))
	}
}

func TestSearchFiles_SkipsGitDir(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".git", "objects"), 0755)
	os.WriteFile(filepath.Join(root, ".git", "config"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(root, "main.go"), []byte("test"), 0644)

	svc := NewSearchService()
	results, err := svc.Search(root, "config", 20)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.Path, ".git") {
			t.Errorf("Search results should not include .git contents, got: %s", r.Path)
		}
	}
}

func TestSearchFiles_SkipsNodeModules(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "node_modules", "lodash"), 0755)
	os.WriteFile(filepath.Join(root, "node_modules", "lodash", "index.js"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(root, "src", "index.js"), []byte("test"), 0644)

	svc := NewSearchService()
	results, err := svc.Search(root, "index", 20)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.Path, "node_modules") {
			t.Errorf("Search results should not include node_modules, got: %s", r.Path)
		}
	}
}

// TestMatchScore_Branches 覆盖 matchScore 全部分支：空模式/完全匹配/前缀/包含/其余。
func TestMatchScore_Branches(t *testing.T) {
	cases := []struct {
		text, pattern string
		want          int
	}{
		{"anything", "", 0},          // 空模式
		{"button", "button", 100},    // 完全匹配
		{"button.vue", "button", 80}, // 前缀
		{"mybutton", "button", 60},   // 包含
		{"xyz", "button", 40},        // 其余
	}
	for _, c := range cases {
		if got := matchScore(c.text, c.pattern); got != c.want {
			t.Errorf("matchScore(%q, %q): got %d, want %d", c.text, c.pattern, got, c.want)
		}
	}
}

// TestFuzzyMatch_EmptyPattern 空模式恒匹配。
func TestFuzzyMatch_EmptyPattern(t *testing.T) {
	if !fuzzyMatch("anything", "") {
		t.Error("空模式应匹配")
	}
}

// TestFuzzyMatch_NoMatch 字符不构成子序列返回 false。
func TestFuzzyMatch_NoMatch(t *testing.T) {
	if fuzzyMatch("abc", "xyz") {
		t.Error("无子序列应不匹配")
	}
}

// TestSearchFiles_SortByScore 结果按分数降序（完全匹配在前）。
func TestSearchFiles_SortByScore(t *testing.T) {
	root := t.TempDir()
	// app 完全匹配(100)、application 前缀(80)、myapp 包含(60)
	for _, f := range []string{"app", "application", "myapp"} {
		os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644)
	}
	svc := NewSearchService()
	results, err := svc.Search(root, "app", 20)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("结果数: got %d, want 3", len(results))
	}
	if results[0].Name != "app" {
		t.Errorf("首位应为完全匹配 app, got %s", results[0].Name)
	}
}
