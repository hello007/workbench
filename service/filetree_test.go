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

func TestIsGitRepoDir_WithGitDir(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, ".git"))

	svc := NewFileTreeService()
	if !svc.isGitRepoDir(dir) {
		t.Error("directory with .git subdirectory should be detected as git repo")
	}
}

func TestIsGitRepoDir_WithGitFile_Worktree(t *testing.T) {
	dir := t.TempDir()
	// git worktree: .git is a file (contains "gitdir: ..." reference)
	mustWriteFile(t, filepath.Join(dir, ".git"), []byte("gitdir: /some/where/.git/worktrees/xxx"))

	svc := NewFileTreeService()
	if !svc.isGitRepoDir(dir) {
		t.Error("directory with .git file (worktree) should be detected as git repo")
	}
}

func TestIsGitRepoDir_NoGit(t *testing.T) {
	dir := t.TempDir()

	svc := NewFileTreeService()
	if svc.isGitRepoDir(dir) {
		t.Error("directory without .git should not be detected as git repo")
	}
}

func TestIsGitRepoDir_CacheHit(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, ".git"))

	svc := NewFileTreeService()
	// First call: miss, stores in cache
	if !svc.isGitRepoDir(dir) {
		t.Error("first call should detect git repo")
	}
	// Remove .git to prove second call hits cache
	os.RemoveAll(filepath.Join(dir, ".git"))
	// Second call: should hit cache and return true
	if !svc.isGitRepoDir(dir) {
		t.Error("second call should hit cache and return true")
	}
}

func TestGetChildren_IsGitRepoField(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "git-project"))
	mustMkdir(t, filepath.Join(dir, "git-project", ".git"))
	mustMkdir(t, filepath.Join(dir, "plain-folder"))
	mustWriteFile(t, filepath.Join(dir, "readme.md"), []byte("hello"))

	svc := NewFileTreeService()
	nodes, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	for _, n := range nodes {
		if n.Name == "git-project" && !n.IsGitRepo {
			t.Error("git-project should have IsGitRepo=true")
		}
		if n.Name == "plain-folder" && n.IsGitRepo {
			t.Error("plain-folder should have IsGitRepo=false")
		}
		if n.Name == "readme.md" && n.IsGitRepo {
			t.Error("file should have IsGitRepo=false")
		}
	}
}

func TestGetChildren_HiddenFiles(t *testing.T) {
	dir := t.TempDir()

	mustWriteFile(t, filepath.Join(dir, ".env"), []byte("KEY=VALUE"))
	mustWriteFile(t, filepath.Join(dir, ".gitignore"), []byte("node_modules/"))
	mustWriteFile(t, filepath.Join(dir, "normal.txt"), []byte("hello"))

	svc := NewFileTreeService()
	nodes, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	found := make(map[string]*model.FileTreeNode)
	for _, n := range nodes {
		found[n.Name] = n
	}

	if n, ok := found[".env"]; !ok {
		t.Error(".env file should be visible")
	} else if n.Type != "file" {
		t.Errorf(".env should be type 'file', got %q", n.Type)
	}

	if n, ok := found[".gitignore"]; !ok {
		t.Error(".gitignore file should be visible")
	} else if n.Type != "file" {
		t.Errorf(".gitignore should be type 'file', got %q", n.Type)
	}

	if _, ok := found["normal.txt"]; !ok {
		t.Error("normal.txt should be visible")
	}
}

func TestGetChildren_HiddenDirVsGitDir(t *testing.T) {
	dir := t.TempDir()

	mustMkdir(t, filepath.Join(dir, ".claude"))
	mustMkdir(t, filepath.Join(dir, ".github"))
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
	if !found[".github"] {
		t.Error(".github should be visible")
	}
	if found[".git"] {
		t.Error(".git should be filtered")
	}
	if !found["src"] {
		t.Error("src should be visible")
	}
}

func TestGetChildren_NestedHiddenDir(t *testing.T) {
	dir := t.TempDir()

	// project/.vscode/settings.json 嵌套隐藏目录
	projectDir := filepath.Join(dir, "project")
	mustMkdir(t, projectDir)
	mustMkdir(t, filepath.Join(projectDir, ".vscode"))
	mustWriteFile(t, filepath.Join(projectDir, ".vscode", "settings.json"), []byte("{}"))

	svc := NewFileTreeService()
	tree, err := svc.GetTree(dir, 3)
	if err != nil {
		t.Fatalf("GetTree failed: %v", err)
	}

	if len(tree) == 0 {
		t.Fatal("GetTree returned empty tree")
	}

	projectNode := tree[0]
	if projectNode.Name != "project" {
		t.Fatalf("expected project node, got %q", projectNode.Name)
	}

	if len(projectNode.Children) == 0 {
		t.Fatal("project node should have children")
	}

	vscodeNode := projectNode.Children[0]
	if vscodeNode.Name != ".vscode" {
		t.Errorf("expected .vscode child, got %q", vscodeNode.Name)
	}
	if vscodeNode.Type != "directory" {
		t.Errorf(".vscode should be type 'directory', got %q", vscodeNode.Type)
	}

	if len(vscodeNode.Children) == 0 {
		t.Fatal(".vscode should have children")
	}
	if vscodeNode.Children[0].Name != "settings.json" {
		t.Errorf("expected settings.json, got %q", vscodeNode.Children[0].Name)
	}
}

func TestGetChildren_DotGitFile(t *testing.T) {
	dir := t.TempDir()

	// .git 作为文件（如 git worktree 场景），同样应被过滤
	mustWriteFile(t, filepath.Join(dir, ".git"), []byte("gitdir: /some/path"))
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

	if found[".git"] {
		t.Error(".git file should be filtered just like .git directory")
	}
	if !found["src"] {
		t.Error("src should be visible")
	}
}
