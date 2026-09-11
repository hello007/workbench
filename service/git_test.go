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

// TestExtractRepoName 覆盖 .git 后缀剥离与路径末段提取。
func TestExtractRepoName(t *testing.T) {
	svc := NewGitService()
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/user/repo.git", "repo"},
		{"https://github.com/user/repo", "repo"},
		{"git@github.com:user/repo.git", "repo"},
		{"ssh://git@gitlab.com/group/sub/project.git", "project"},
		{"repo", "repo"},
		{"", ""},
	}
	for _, c := range cases {
		if got := svc.ExtractRepoName(c.url); got != c.want {
			t.Errorf("ExtractRepoName(%q): got %q, want %q", c.url, got, c.want)
		}
	}
}

// TestHasRemote_NonRepo 非仓库目录无远程返回 false。
func TestHasRemote_NonRepo(t *testing.T) {
	svc := NewGitService()
	if svc.HasRemote(t.TempDir()) {
		t.Error("非仓库目录应无远程")
	}
}

// TestGetInfo_NonRepo 非仓库目录返回 IsRepo=false 且不报错。
func TestGetInfo_NonRepo(t *testing.T) {
	svc := NewGitService()
	info, err := svc.GetInfo(t.TempDir())
	if err != nil {
		t.Fatalf("GetInfo non-repo: %v", err)
	}
	if info.IsRepo {
		t.Error("非仓库目录 IsRepo 应为 false")
	}
}

// TestPull_NonRepo 非仓库目录 pull 返回错误。
func TestPull_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.Pull(t.TempDir(), false); err == nil {
		t.Error("非仓库 pull 应返回错误")
	}
}

// TestClone_TargetExists 目标路径已存在时返回错误（不真克隆）。
func TestClone_TargetExists(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "exists")
	os.MkdirAll(target, 0o755)

	svc := NewGitService()
	_, err := svc.Clone("https://example.com/x.git", target)
	if err == nil {
		t.Error("目标已存在应返回错误")
	}
}

// TestGetBranches_NonRepo 非仓库目录返回错误。
func TestGetBranches_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.GetBranches(t.TempDir()); err == nil {
		t.Error("非仓库 getbranches 应返回错误")
	}
}

// TestCheckoutBranch_NonRepo 非仓库目录返回错误。
func TestCheckoutBranch_NonRepo(t *testing.T) {
	svc := NewGitService()
	if err := svc.CheckoutBranch(t.TempDir(), "main", false); err == nil {
		t.Error("非仓库 checkout 应返回错误")
	}
}

// TestDiscardChanges_NonRepo 非 git 目录无法定位仓库根，返回错误。
func TestDiscardChanges_NonRepo(t *testing.T) {
	svc := NewGitService()
	if err := svc.DiscardChanges(t.TempDir(), nil); err == nil {
		t.Error("非仓库 discard 应返回错误")
	}
}

// TestGetInfo_RealRepo 真实仓库（无远程）IsRepo=true 且不报错。
func TestGetInfo_RealRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@t.com")
	runGit(t, dir, "config", "user.name", "t")

	svc := NewGitService()
	info, err := svc.GetInfo(dir)
	if err != nil {
		t.Fatalf("GetInfo real repo: %v", err)
	}
	if !info.IsRepo {
		t.Error("真实仓库 IsRepo 应为 true")
	}
}

// TestDiscardChanges_RealRepo_All 真实仓库回滚全部改动（已跟踪文件恢复 + 未跟踪清理）。
func TestDiscardChanges_RealRepo_All(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@t.com")
	runGit(t, dir, "config", "user.name", "t")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("原"), 0o644)
	runGit(t, dir, "add", "f.txt")
	runGit(t, dir, "commit", "-m", "init")

	// 修改已跟踪文件 + 新增未跟踪文件
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("改"), 0o644)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("新"), 0o644)

	svc := NewGitService()
	if err := svc.DiscardChanges(dir, nil); err != nil {
		t.Fatalf("DiscardChanges all: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "f.txt"))
	if string(data) != "原" {
		t.Errorf("已跟踪文件应恢复原内容, got %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Error("未跟踪文件应被清理")
	}
}

// TestCommit_NoFiles 未选文件返回错误。
func TestCommit_NoFiles(t *testing.T) {
	svc := NewGitService()
	if err := svc.Commit(t.TempDir(), "msg", nil); err == nil {
		t.Error("未选文件应返回错误")
	}
}

// TestCommit_EmptyMessage 提交信息为空返回错误。
func TestCommit_EmptyMessage(t *testing.T) {
	svc := NewGitService()
	if err := svc.Commit(t.TempDir(), "  ", []string{"f.txt"}); err == nil {
		t.Error("空提交信息应返回错误")
	}
}

// TestCommit_RealRepo 真实仓库选择性提交文件。
func TestCommit_RealRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@t.com")
	runGit(t, dir, "config", "user.name", "t")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644)

	svc := NewGitService()
	if err := svc.Commit(dir, "首次提交", []string{"f.txt"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	// 提交后工作区应无改动
	has, _ := svc.gitCmd.HasLocalChanges(dir)
	if has {
		t.Error("提交后工作区应干净")
	}
}

// TestGetLocalChanges_RealRepo 真实仓库文件状态解析。
func TestGetLocalChanges_RealRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@t.com")
	runGit(t, dir, "config", "user.name", "t")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)

	svc := NewGitService()
	changes, err := svc.GetLocalChanges(dir)
	if err != nil {
		t.Fatalf("GetLocalChanges: %v", err)
	}
	if len(changes) == 0 {
		t.Error("应检测到改动文件")
	}
}

// TestGetDiff_NonRepo 非仓库目录返回错误（不 panic）。
func TestGetDiff_NonRepo(t *testing.T) {
	svc := NewGitService()
	_, err := svc.GetDiff(t.TempDir(), "f.txt")
	if err == nil {
		t.Error("非仓库 GetDiff 应返回错误")
	}
}

// TestPush_NonRepo 非仓库目录 push 返回错误。
func TestPush_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.Push(t.TempDir(), false); err == nil {
		t.Error("非仓库 push 应返回错误")
	}
}

// TestPush_RealRepo_NoRemote 真实仓库无远程时 push 失败。
func TestPush_RealRepo_NoRemote(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@t.com")
	runGit(t, dir, "config", "user.name", "t")
	svc := NewGitService()
	if _, err := svc.Push(dir, false); err == nil {
		t.Error("无远程仓库 push 应失败")
	}
}

// TestHasUpstream_NonRepo 非仓库目录返回错误。
func TestHasUpstream_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.HasUpstream(t.TempDir()); err == nil {
		t.Error("非仓库 HasUpstream 应返回错误")
	}
}

// TestHasUpstream_RealRepo_NoUpstream 真实仓库无上游返回 false。
func TestHasUpstream_RealRepo_NoUpstream(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@t.com")
	runGit(t, dir, "config", "user.name", "t")
	svc := NewGitService()
	has, err := svc.HasUpstream(dir)
	if err != nil {
		t.Fatalf("HasUpstream: %v", err)
	}
	if has {
		t.Error("无远程仓库应无上游")
	}
}

// TestSafeEmit_NoPanic nil ctx 或无 events 的 ctx 不 panic、不调用 EventsEmit。
func TestSafeEmit_NoPanic(t *testing.T) {
	safeEmit(nil, "event", "data")
	safeEmit(context.Background(), "event", "data")
}

// TestSafeEmit_NonGitDir_ScanGitReposCached 未注入缓存的 ScanGitRepos 走纯 .git 预筛路径。
func TestScanGitRepos_CachedWithCache(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "r1")
	os.MkdirAll(repo, 0o755)
	runGit(t, repo, "init")

	svc := NewGitServiceWithCache(filepath.Join(t.TempDir(), "scan_cache.json"))
	repos := svc.ScanGitRepos(root)
	if len(repos) != 1 {
		t.Errorf("缓存路径扫描应找到 1 个仓库, got %d", len(repos))
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

// headSHA 取仓库 HEAD 的完整 SHA，供 commit diff 测试定位提交用。
func headSHA(t *testing.T, dir string) string {
	t.Helper()
	output, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in %s failed: %v", dir, err)
	}
	return strings.TrimSpace(string(output))
}

func TestGetCommitFileDiff_NormalCommit(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 首次提交（root）
	writeFile(t, filepath.Join(repo, "a.txt"), "line1\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 第二次提交：修改 a.txt
	writeFile(t, filepath.Join(repo, "a.txt"), "line1\nline2\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "add line2")

	sha := headSHA(t, repo)
	diff, err := svc.GetCommitFileDiff(repo, sha, "a.txt")
	if err != nil {
		t.Fatalf("GetCommitFileDiff failed: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff for commit file change")
	}
	if !strings.Contains(diff, "+line2") {
		t.Errorf("expected diff to contain +line2, got:\n%s", diff)
	}
}

func TestGetCommitFileDiff_RootCommit(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 仅一条 root commit
	writeFile(t, filepath.Join(repo, "a.txt"), "first\ncontent\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "root")

	sha := headSHA(t, repo)
	diff, err := svc.GetCommitFileDiff(repo, sha, "a.txt")
	if err != nil {
		t.Fatalf("GetCommitFileDiff on root commit failed: %v", err)
	}
	if diff == "" {
		t.Fatal("root commit diff should be non-empty (all-added)")
	}
	// root commit 无 parent，文件应呈现为全增
	if !strings.Contains(diff, "+first") || !strings.Contains(diff, "+content") {
		t.Errorf("root commit diff should show all lines as added, got:\n%s", diff)
	}
	// 不应出现删除行（无 parent 无旧版本）
	if strings.Contains(diff, "-first") {
		t.Errorf("root commit diff should not contain deleted lines, got:\n%s", diff)
	}
}

func TestGetCommitFileDiff_BinaryFile(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 首次提交一个文本文件建立非 root 环境
	writeFile(t, filepath.Join(repo, "a.txt"), "init\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 二进制文件（含 NUL 字节）
	binPath := filepath.Join(repo, "bin.dat")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 0xFF}, 0644); err != nil {
		t.Fatalf("write binary file failed: %v", err)
	}
	runGit(t, repo, "add", "bin.dat")
	runGit(t, repo, "commit", "-m", "add binary")

	sha := headSHA(t, repo)
	diff, err := svc.GetCommitFileDiff(repo, sha, "bin.dat")
	if err != nil {
		t.Fatalf("GetCommitFileDiff on binary file failed: %v", err)
	}
	// git diff 对二进制文件输出 "Binary files ... differ"
	if !strings.Contains(diff, "Binary files") {
		t.Errorf("expected binary file hint in diff, got:\n%s", diff)
	}
}

func TestGetCommitFileDiff_NoChangeReturnsEmpty(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	writeFile(t, filepath.Join(repo, "a.txt"), "line1\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 第二次提交改的是 b.txt，对 a.txt 取 diff 应为空
	writeFile(t, filepath.Join(repo, "b.txt"), "new\n")
	runGit(t, repo, "add", "b.txt")
	runGit(t, repo, "commit", "-m", "add b")

	sha := headSHA(t, repo)
	diff, err := svc.GetCommitFileDiff(repo, sha, "a.txt")
	if err != nil {
		t.Fatalf("GetCommitFileDiff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for unchanged file, got:\n%s", diff)
	}
}

func TestGetCommitFileDiff_EmptySHAReturnsError(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	_, err := svc.GetCommitFileDiff(repo, "", "a.txt")
	if err == nil {
		t.Error("empty SHA should return error")
	}
}

func TestGetRangeDiff_TwoCommits(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// commit1: a.txt 初始
	writeFile(t, filepath.Join(repo, "a.txt"), "v1\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "c1")
	sha1 := headSHA(t, repo)

	// commit2: 改 a.txt + 加 b.txt
	writeFile(t, filepath.Join(repo, "a.txt"), "v1\nv2\n")
	writeFile(t, filepath.Join(repo, "b.txt"), "new\n")
	runGit(t, repo, "add", "a.txt", "b.txt")
	runGit(t, repo, "commit", "-m", "c2")
	sha2 := headSHA(t, repo)

	diff, err := svc.GetRangeDiff(repo, sha1, sha2)
	if err != nil {
		t.Fatalf("GetRangeDiff failed: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty range diff")
	}
	// range diff 应覆盖两个文件的变更
	if !strings.Contains(diff, "+v2") {
		t.Errorf("range diff should contain a.txt change +v2, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+new") {
		t.Errorf("range diff should contain b.txt add +new, got:\n%s", diff)
	}
}

func TestGetRangeDiff_SameSHAEmpty(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	writeFile(t, filepath.Join(repo, "a.txt"), "v1\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "c1")
	sha := headSHA(t, repo)

	diff, err := svc.GetRangeDiff(repo, sha, sha)
	if err != nil {
		t.Fatalf("GetRangeDiff same SHA failed: %v", err)
	}
	if diff != "" {
		t.Errorf("same SHA range diff should be empty, got:\n%s", diff)
	}
}

func TestGetRangeDiff_EmptySHAReturnsError(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	_, err := svc.GetRangeDiff(repo, "", "abc")
	if err == nil {
		t.Error("empty base SHA should return error")
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

// TestGetLocalChanges_UntrackedDirExpanded 验证未跟踪目录被展开为内部每个文件单独成条
// （对应 --untracked-files=all），而非默认 --untracked-files=normal 的单行 ?? dir/
func TestGetLocalChanges_UntrackedDirExpanded(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 建立初始提交（确立 HEAD，未跟踪目录内文件均为真正新增）
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 在未跟踪目录下放多个文件
	writeFile(t, filepath.Join(repo, "newdir", "f1.txt"), "one")
	writeFile(t, filepath.Join(repo, "newdir", "f2.txt"), "two")
	writeFile(t, filepath.Join(repo, "newdir", "sub", "f3.txt"), "three")

	changes, err := svc.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges failed: %v", err)
	}

	want := map[string]bool{
		"newdir/f1.txt":     false,
		"newdir/f2.txt":     false,
		"newdir/sub/f3.txt": false,
	}
	for _, c := range changes {
		// 未跟踪文件状态码为 ?
		if _, ok := want[c.Path]; ok {
			if c.Status != "?" {
				t.Errorf("path %s: expected status '?', got %q", c.Path, c.Status)
			}
			if c.Staged {
				t.Errorf("path %s: expected Staged=false for untracked", c.Path)
			}
			want[c.Path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("expected untracked file %q in changes, not found (dir was collapsed?)", path)
		}
	}

	// 目录本身不应作为独立条目出现（折叠形态 newdir/ 不应存在）
	for _, c := range changes {
		if c.Path == "newdir/" || c.Path == "newdir" {
			t.Errorf("untracked dir should be expanded, got collapsed entry: %q", c.Path)
		}
	}
}

// TestGetLocalChanges_ChineseUntrackedPath 验证 -z 下中文路径原样保留且被 -uall 展开
func TestGetLocalChanges_ChineseUntrackedPath(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	writeFile(t, filepath.Join(repo, "中文目录", "文件.txt"), "中文内容")

	changes, err := svc.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges failed: %v", err)
	}

	found := false
	for _, c := range changes {
		if c.Path == filepath.ToSlash(filepath.Join("中文目录", "文件.txt")) {
			found = true
			if c.Status != "?" {
				t.Errorf("expected status '?', got %q", c.Status)
			}
		}
	}
	if !found {
		t.Errorf("chinese untracked path not found in changes: %+v", changes)
	}
}

// TestGetLocalChanges_RenameStillParses 验证 -uall 下重命名(R)两段式路径解析。
//
// git status -z 重命名条目格式为 "R  new.txt\x00old.txt\x00"（目标在前、源在后）：
// 目标路径已在 seg[3:]，下一段为源路径，解析器仅跳过、不取作 Path。
// 故 staged 记录的 Path 应为目标路径 new.txt，源路径 old.txt 不应出现。
func TestGetLocalChanges_RenameStillParses(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()

	// 初始提交一个文件
	writeFile(t, filepath.Join(repo, "old.txt"), "content\n")
	runGit(t, repo, "add", "old.txt")
	runGit(t, repo, "commit", "-m", "init")

	// git mv 制造重命名（已暂存，状态 R）
	runGit(t, repo, "mv", "old.txt", "new.txt")

	changes, err := svc.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges failed: %v", err)
	}

	// 重命名应只产出一条记录（两段式折叠为一条），且 Staged=true
	renameCount := 0
	hasNew := false
	for _, c := range changes {
		if c.Staged {
			renameCount++
			if c.Path == "new.txt" {
				hasNew = true
			}
		}
		// 源路径 old.txt 不应作为任何条目的 Path（既非独立条目，也非 rename 条目的 Path）
		if c.Path == "old.txt" {
			t.Errorf("source path old.txt should not appear as a change entry: %+v", c)
		}
	}
	if renameCount != 1 {
		t.Errorf("expected exactly 1 staged rename entry, got %d: %+v", renameCount, changes)
	}
	if !hasNew {
		t.Errorf("staged rename entry should use target path new.txt, got changes: %+v", changes)
	}
}

// TestCreateBranch_EmptyName 空分支名返回错误。
func TestCreateBranch_EmptyName(t *testing.T) {
	svc := NewGitService()
	if err := svc.CreateBranch(t.TempDir(), "  "); err == nil {
		t.Error("空分支名应返回错误")
	}
}

// TestCreateBranch_NonRepo 非 git 目录无法定位仓库根，返回错误。
func TestCreateBranch_NonRepo(t *testing.T) {
	svc := NewGitService()
	if err := svc.CreateBranch(t.TempDir(), "feature"); err == nil {
		t.Error("非仓库 createbranch 应返回错误")
	}
}

// TestCreateBranch_RealRepo 真实仓库从 HEAD 创建分支，列表应包含新分支。
func TestCreateBranch_RealRepo(t *testing.T) {
	repo := initTempRepo(t)
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	svc := NewGitService()
	if err := svc.CreateBranch(repo, "feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	branches, err := svc.GetBranches(repo)
	if err != nil {
		t.Fatalf("GetBranches: %v", err)
	}
	found := false
	for _, b := range branches.Branches {
		if b.Name == "feature" {
			found = true
		}
	}
	if !found {
		t.Error("创建的 feature 分支应在分支列表中")
	}
}

// TestDeleteBranch_EmptyName 空分支名返回错误。
func TestDeleteBranch_EmptyName(t *testing.T) {
	svc := NewGitService()
	if err := svc.DeleteBranch(t.TempDir(), "", false); err == nil {
		t.Error("空分支名应返回错误")
	}
}

// TestDeleteBranch_ForceDeletesUnmerged 验证 force=true 走 -D 强删路径：
// 未合并分支用 -d（force=false）删除失败，用 -D（force=true）删除成功。
func TestDeleteBranch_ForceDeletesUnmerged(t *testing.T) {
	repo := initTempRepo(t)
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 创建并切换到 feature 分支，新增未合并提交后切回原分支
	runGit(t, repo, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(repo, "b.txt"), "feature-only")
	runGit(t, repo, "add", "b.txt")
	runGit(t, repo, "commit", "-m", "feature commit")
	runGit(t, repo, "checkout", "-")

	svc := NewGitService()
	// force=false 走 -d：未合并应失败
	if err := svc.DeleteBranch(repo, "feature", false); err == nil {
		t.Fatal("未合并分支用 -d 删除应失败")
	}
	// force=true 走 -D：应强删成功
	if err := svc.DeleteBranch(repo, "feature", true); err != nil {
		t.Fatalf("force=true 强删应成功: %v", err)
	}
}

// TestRenameBranch_EmptyName 原分支名或新分支名空均返回错误。
func TestRenameBranch_EmptyName(t *testing.T) {
	svc := NewGitService()
	if err := svc.RenameBranch(t.TempDir(), "", "new"); err == nil {
		t.Error("空原分支名应返回错误")
	}
	if err := svc.RenameBranch(t.TempDir(), "old", ""); err == nil {
		t.Error("空新分支名应返回错误")
	}
}

// TestRenameBranch_RealRepo 真实仓库重命名分支，旧名消失、新名出现。
func TestRenameBranch_RealRepo(t *testing.T) {
	repo := initTempRepo(t)
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")
	runGit(t, repo, "branch", "old-name")

	svc := NewGitService()
	if err := svc.RenameBranch(repo, "old-name", "new-name"); err != nil {
		t.Fatalf("RenameBranch: %v", err)
	}
	branches, err := svc.GetBranches(repo)
	if err != nil {
		t.Fatalf("GetBranches: %v", err)
	}
	hasOld, hasNew := false, false
	for _, b := range branches.Branches {
		if b.Name == "old-name" {
			hasOld = true
		}
		if b.Name == "new-name" {
			hasNew = true
		}
	}
	if hasOld {
		t.Error("旧分支名应不存在")
	}
	if !hasNew {
		t.Error("新分支名应存在")
	}
}

// TestStageFiles_EmptyFiles 空文件列表返回错误。
func TestStageFiles_EmptyFiles(t *testing.T) {
	svc := NewGitService()
	if err := svc.StageFiles(t.TempDir(), nil); err == nil {
		t.Error("空文件列表应返回错误")
	}
}

// TestStageFiles_RealRepo 真实仓库暂存已修改文件，Staged 应转为 true。
func TestStageFiles_RealRepo(t *testing.T) {
	repo := initTempRepo(t)
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")
	writeFile(t, filepath.Join(repo, "a.txt"), "modified")

	svc := NewGitService()
	if err := svc.StageFiles(repo, []string{"a.txt"}); err != nil {
		t.Fatalf("StageFiles: %v", err)
	}
	changes, err := svc.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges: %v", err)
	}
	found := false
	for _, c := range changes {
		if c.Path == "a.txt" {
			found = true
			if !c.Staged {
				t.Error("暂存后 a.txt 应 Staged=true")
			}
		}
	}
	if !found {
		t.Error("a.txt 应在变动列表中")
	}
}

// TestUnstageFiles_EmptyFiles 空文件列表返回错误。
func TestUnstageFiles_EmptyFiles(t *testing.T) {
	svc := NewGitService()
	if err := svc.UnstageFiles(t.TempDir(), nil); err == nil {
		t.Error("空文件列表应返回错误")
	}
}

// TestUnstageFiles_RealRepo 真实仓库取消暂存已暂存文件，Staged 应转为 false 且仍在变动列表。
func TestUnstageFiles_RealRepo(t *testing.T) {
	repo := initTempRepo(t)
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")
	writeFile(t, filepath.Join(repo, "a.txt"), "modified")
	runGit(t, repo, "add", "a.txt")

	svc := NewGitService()
	if err := svc.UnstageFiles(repo, []string{"a.txt"}); err != nil {
		t.Fatalf("UnstageFiles: %v", err)
	}
	changes, err := svc.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges: %v", err)
	}
	found := false
	for _, c := range changes {
		if c.Path == "a.txt" {
			found = true
			if c.Staged {
				t.Error("取消暂存后 a.txt 应 Staged=false")
			}
		}
	}
	if !found {
		t.Error("a.txt 应仍在变动列表中（仅取消暂存，未丢弃改动）")
	}
}
