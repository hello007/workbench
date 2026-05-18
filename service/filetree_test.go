package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"git-manager/model"
)

func TestGetChildren_DirectoriesFirst(t *testing.T) {
	dir := t.TempDir()

	mustMkdir(t, filepath.Join(dir, "b-folder"))
	mustWriteFile(t, filepath.Join(dir, "a-file.txt"), []byte{})
	mustMkdir(t, filepath.Join(dir, "a-folder"))
	mustWriteFile(t, filepath.Join(dir, "z-file.txt"), []byte{})
	mustMkdir(t, filepath.Join(dir, "Z-Folder"))

	svc := NewFileTreeService()
	nodes, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	// 所有目录应在文件之前
	for i, n := range nodes {
		if n.Type == "file" {
			for j := 0; j < i; j++ {
				if nodes[j].Type != "directory" {
					t.Errorf("found file %q before directory %q", nodes[j].Name, n.Name)
				}
			}
			break
		}
	}
	// 文件之后不应出现目录
	firstFileIdx := -1
	for i, n := range nodes {
		if n.Type == "file" {
			firstFileIdx = i
			break
		}
	}
	if firstFileIdx >= 0 {
		for i := firstFileIdx + 1; i < len(nodes); i++ {
			if nodes[i].Type == "directory" {
				t.Errorf("found directory %q after file at index %d", nodes[i].Name, firstFileIdx)
			}
		}
	}

	// 目录内部按名称排序（大小写不敏感）
	dirs := filterByType(nodes, "directory")
	if !sort.SliceIsSorted(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	}) {
		t.Errorf("directories not sorted: %v", namesOf(dirs))
	}

	// 文件内部按名称排序（大小写不敏感）
	files := filterByType(nodes, "file")
	if !sort.SliceIsSorted(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	}) {
		t.Errorf("files not sorted: %v", namesOf(files))
	}
}

func TestGetChildren_OnlyGitSkipped(t *testing.T) {
	dir := t.TempDir()

	mustMkdir(t, filepath.Join(dir, ".hidden"))
	mustWriteFile(t, filepath.Join(dir, ".dotfile"), []byte{})
	mustMkdir(t, filepath.Join(dir, ".git"))
	mustMkdir(t, filepath.Join(dir, "visible"))

	svc := NewFileTreeService()
	nodes, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	found := make(map[string]bool)
	for _, n := range nodes {
		found[n.Name] = true
	}

	if !found[".hidden"] {
		t.Error(".hidden folder should be visible")
	}
	if !found[".dotfile"] {
		t.Error(".dotfile should be visible")
	}
	if found[".git"] {
		t.Error(".git should be skipped")
	}
}

func filterByType(nodes []*model.FileTreeNode, typ string) []*model.FileTreeNode {
	var result []*model.FileTreeNode
	for _, n := range nodes {
		if n.Type == typ {
			result = append(result, n)
		}
	}
	return result
}

func namesOf(nodes []*model.FileTreeNode) []string {
	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name
	}
	return names
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatalf("failed to create dir %q: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write file %q: %v", path, err)
	}
}

func TestGetChildren_HiddenDirectories(t *testing.T) {
	dir := t.TempDir()

	mustMkdir(t, filepath.Join(dir, ".claude"))
	mustMkdir(t, filepath.Join(dir, ".vscode"))
	mustMkdir(t, filepath.Join(dir, ".git"))
	mustMkdir(t, filepath.Join(dir, "src"))

	svc := NewFileTreeService()
	nodes, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	found := make(map[string]bool)
	for _, n := range nodes {
		found[n.Name] = true
	}

	if !found[".claude"] {
		t.Error(".claude should be visible")
	}
	if !found[".vscode"] {
		t.Error(".vscode should be visible")
	}
	if found[".git"] {
		t.Error(".git should be skipped")
	}
	if !found["src"] {
		t.Error("src should be visible")
	}
}

func TestGetChildren_IsLeafField(t *testing.T) {
	dir := t.TempDir()

	mustMkdir(t, filepath.Join(dir, "folder"))
	mustWriteFile(t, filepath.Join(dir, "file.txt"), []byte("hello"))

	svc := NewFileTreeService()
	nodes, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	for _, n := range nodes {
		if n.Type == "file" && !n.IsLeaf {
			t.Errorf("file %q: IsLeaf should be true", n.Name)
		}
		if n.Type == "directory" && n.IsLeaf {
			t.Errorf("directory %q: IsLeaf should be false", n.Name)
		}
	}
}

func TestGetChildren_HasChildrenField(t *testing.T) {
	dir := t.TempDir()

	mustMkdir(t, filepath.Join(dir, "folder"))
	mustWriteFile(t, filepath.Join(dir, "file.txt"), []byte("hello"))

	svc := NewFileTreeService()
	nodes, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	for _, n := range nodes {
		if n.Type == "directory" && !n.HasChildren {
			t.Errorf("directory %q: HasChildren should be true", n.Name)
		}
		if n.Type == "file" && n.HasChildren {
			t.Errorf("file %q: HasChildren should be false", n.Name)
		}
	}
}

func TestGetChildren_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	emptyDir := filepath.Join(dir, "empty")
	mustMkdir(t, emptyDir)

	svc := NewFileTreeService()
	nodes, err := svc.GetChildren(emptyDir)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	if len(nodes) != 0 {
		t.Errorf("empty directory: got %d nodes, want 0", len(nodes))
	}
}

func TestGetChildren_NonExistentPath(t *testing.T) {
	dir := t.TempDir()
	nonExistent := filepath.Join(dir, "does_not_exist")

	svc := NewFileTreeService()
	_, err := svc.GetChildren(nonExistent)
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}
