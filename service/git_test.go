package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// 创建一个真实的 git 仓库（无远程，会被跳过）
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

func TestHasRemote(t *testing.T) {
	// 无远程仓库
	repo := initTempRepo(t)
	svc := NewGitService()
	if svc.HasRemote(repo) {
		t.Error("expected HasRemote=false for repo without remote")
	}

	// 配置远程后应返回 true（不要求远程可达，仅检测配置存在）
	runGit(t, repo, "remote", "add", "origin", "https://example.com/repo.git")
	if !svc.HasRemote(repo) {
		t.Error("expected HasRemote=true after adding remote")
	}
}

func TestBatchPull_SkipsNoRemote(t *testing.T) {
	repo := initTempRepo(t) // 无远程配置
	svc := NewGitService()
	results := svc.BatchPull([]string{repo}, 1, context.Background())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.Skipped {
		t.Error("expected Skipped=true for repo without remote")
	}
	if r.Success {
		t.Error("expected Success=false for skipped repo")
	}
	if r.Error != "" {
		t.Errorf("expected no error for skipped repo, got: %s", r.Error)
	}
}

// initTempRepo 初始化一个临时 git 仓库并配置身份，返回仓库根目录。
func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	return dir
}

// writeFile 写入文件内容（自动创建父目录）。
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func TestCommit_EmptyFilesReturnsError(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	err := svc.Commit(repo, "msg", nil)
	if err == nil {
		t.Fatal("expected error for empty files")
	}
	if !strings.Contains(err.Error(), "未选择") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCommit_EmptyMessageReturnsError(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	err := svc.Commit(repo, "  ", []string{"a.txt"})
	if err == nil {
		t.Fatal("expected error for empty message")
	}
	if !strings.Contains(err.Error(), "提交信息") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCommit_TrackedFile(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 初始提交建立 HEAD
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 修改 a.txt 并提交
	writeFile(t, filepath.Join(repo, "a.txt"), "modified")
	if err := svc.Commit(repo, "change a", []string{"a.txt"}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	output, err := exec.Command("git", "-C", repo, "log", "--oneline").Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if !strings.Contains(string(output), "change a") {
		t.Errorf("commit message not found in log: %s", output)
	}
}

func TestCommit_UntrackedFile(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 先建一个初始提交，避免首次提交特殊语义
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 新增未跟踪文件 b.txt
	writeFile(t, filepath.Join(repo, "b.txt"), "new file")
	if err := svc.Commit(repo, "add b", []string{"b.txt"}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	output, err := exec.Command("git", "-C", repo, "log", "--oneline").Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if !strings.Contains(string(output), "add b") {
		t.Errorf("commit message not found in log: %s", output)
	}

	// 提交后 b.txt 应已不在变动列表
	changes, err := svc.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges failed: %v", err)
	}
	for _, c := range changes {
		if c.Path == "b.txt" {
			t.Errorf("b.txt should be committed, still in changes: %+v", c)
		}
	}
}

func TestCommit_Pathspec_OnlySelectedFiles(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 建立初始提交
	writeFile(t, filepath.Join(repo, "a.txt"), "init a")
	writeFile(t, filepath.Join(repo, "b.txt"), "init b")
	runGit(t, repo, "add", "a.txt", "b.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 同时修改 a.txt 和 b.txt，但只提交 a.txt
	writeFile(t, filepath.Join(repo, "a.txt"), "changed a")
	writeFile(t, filepath.Join(repo, "b.txt"), "changed b")
	if err := svc.Commit(repo, "only a", []string{"a.txt"}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// b.txt 应仍在变动列表中（未提交），a.txt 不在
	changes, err := svc.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges failed: %v", err)
	}
	hasB, hasA := false, false
	for _, c := range changes {
		if c.Path == "b.txt" {
			hasB = true
		}
		if c.Path == "a.txt" {
			hasA = true
		}
	}
	if !hasB {
		t.Error("b.txt should remain uncommitted")
	}
	if hasA {
		t.Error("a.txt should be committed, not in changes")
	}
}

func TestCommit_ChinesePath(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 初始提交
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 子目录下的中文路径文件
	writeFile(t, filepath.Join(repo, "中文目录", "文件.txt"), "中文内容")
	if err := svc.Commit(repo, "中文提交", []string{filepath.ToSlash(filepath.Join("中文目录", "文件.txt"))}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	output, err := exec.Command("git", "-C", repo, "log", "--oneline").Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if !strings.Contains(string(output), "中文提交") {
		t.Errorf("commit message not found in log: %s", output)
	}
}

func TestGetDiff_TrackedFile(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	writeFile(t, filepath.Join(repo, "a.txt"), "line1\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	writeFile(t, filepath.Join(repo, "a.txt"), "line1\nline2\n")

	diff, err := svc.GetDiff(repo, "a.txt")
	if err != nil {
		t.Fatalf("GetDiff failed: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(diff, "+line2") {
		t.Errorf("expected diff to contain added line, got:\n%s", diff)
	}
}

func TestGetDiff_UntrackedFile(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 初始提交（确保工作区有 HEAD）
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 未跟踪文件
	writeFile(t, filepath.Join(repo, "b.txt"), "new\ncontent\n")
	diff, err := svc.GetDiff(repo, "b.txt")
	if err != nil {
		t.Fatalf("GetDiff failed: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff for untracked file")
	}
	if !strings.Contains(diff, "+new") || !strings.Contains(diff, "+content") {
		t.Errorf("expected diff to contain file content as added lines, got:\n%s", diff)
	}
}

func TestHasUpstream_NoRemote(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	has, err := svc.HasUpstream(repo)
	if err != nil {
		t.Fatalf("HasUpstream failed: %v", err)
	}
	if has {
		t.Error("expected false for repo without remote/upstream")
	}
}

func TestPush_NoUpstream(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 无远程配置的仓库直接 push 应返回错误（可接受）
	_, err := svc.Push(repo, false)
	if err == nil {
		t.Fatal("expected error when pushing without remote/upstream")
	}
}
