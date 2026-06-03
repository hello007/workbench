package service

import (
	"os"
	"path/filepath"
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
