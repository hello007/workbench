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

	os.Mkdir(filepath.Join(dir, "b-folder"), 0755)
	os.WriteFile(filepath.Join(dir, "a-file.txt"), []byte{}, 0644)
	os.Mkdir(filepath.Join(dir, "a-folder"), 0755)
	os.WriteFile(filepath.Join(dir, "z-file.txt"), []byte{}, 0644)
	os.Mkdir(filepath.Join(dir, "Z-Folder"), 0755)

	svc := NewFileTreeService()
	nodes, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	// 所有目录应在文件之前
	firstFileIdx := -1
	for i, n := range nodes {
		if n.Type == "file" {
			firstFileIdx = i
			break
		}
	}
	if firstFileIdx > 0 {
		for i := 0; i < firstFileIdx; i++ {
			if nodes[i].Type != "directory" {
				t.Errorf("found file %q before first file index", nodes[i].Name)
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

	os.Mkdir(filepath.Join(dir, ".hidden"), 0755)
	os.WriteFile(filepath.Join(dir, ".dotfile"), []byte{}, 0644)
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.Mkdir(filepath.Join(dir, "visible"), 0755)

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
