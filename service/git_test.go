package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"workbench/model"
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

func TestBatchPull_SuccessAndFail(t *testing.T) {
	dir := t.TempDir()

	// 创建一个真实的 git 仓库（无远程，pull 会失败）
	repoPath := filepath.Join(dir, "repo")
	os.MkdirAll(repoPath, 0755)
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.email", "test@test.com")
	runGit(t, repoPath, "config", "user.name", "test")

	// 创建一个非 git 目录（会失败）
	nonRepo := filepath.Join(dir, "not-a-repo")
	os.MkdirAll(nonRepo, 0755)

	svc := NewGitService()
	results := svc.BatchPull([]string{repoPath, nonRepo}, 2, context.Background())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// 按路径查找结果（goroutine 执行顺序不确定）
	var repoResult, nonRepoResult *model.PullResult
	for i := range results {
		if results[i].Path == repoPath {
			repoResult = &results[i]
		}
		if results[i].Path == nonRepo {
			nonRepoResult = &results[i]
		}
	}

	if repoResult == nil {
		t.Fatal("expected result for repoPath")
	}
	if repoResult.Path != repoPath {
		t.Errorf("expected path %s, got %s", repoPath, repoResult.Path)
	}

	if nonRepoResult == nil {
		t.Fatal("expected result for nonRepo")
	}
	if nonRepoResult.Success {
		t.Error("expected non-repo to fail")
	}
	if nonRepoResult.Error == "" {
		t.Error("expected error message for non-repo")
	}
}
