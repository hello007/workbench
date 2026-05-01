package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScanGitRepos_SingleRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")

	svc := NewGitService()
	repos := svc.ScanGitRepos(dir)

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0] != dir {
		t.Errorf("expected %s, got %s", dir, repos[0])
	}
}

func TestScanGitRepos_NestedRepos(t *testing.T) {
	root := t.TempDir()

	repoA := filepath.Join(root, "project-a")
	repoB := filepath.Join(root, "subdir", "project-b")
	repoC := filepath.Join(root, "subdir", "deep", "project-c")

	for _, repo := range []string{repoA, repoB, repoC} {
		os.MkdirAll(repo, 0755)
		runGit(t, repo, "init")
		runGit(t, repo, "config", "user.email", "test@test.com")
		runGit(t, repo, "config", "user.name", "test")
	}

	svc := NewGitService()
	repos := svc.ScanGitRepos(root)

	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d: %v", len(repos), repos)
	}
}

func TestScanGitRepos_NoRepos(t *testing.T) {
	dir := t.TempDir()

	svc := NewGitService()
	repos := svc.ScanGitRepos(dir)

	if len(repos) != 0 {
		t.Fatalf("expected 0 repos, got %d", len(repos))
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v in %s failed: %v", args, dir, err)
	}
}
