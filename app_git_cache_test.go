package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"workbench/model"
	"workbench/service"
	"workbench/util"
)

// setupRepoWithCommits 构造含 n 个提交的临时 git 仓库，返回仓库路径。
// 每个提交包含一个 fileN.txt，提交消息 "Commit N"。
func setupRepoWithCommits(t *testing.T, n int) string {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), "test-repo")
	os.MkdirAll(repoPath, 0755)
	exec.Command("git", "init", repoPath).Run()
	exec.Command("git", "-C", repoPath, "config", "user.name", "Test").Run()
	exec.Command("git", "-C", repoPath, "config", "user.email", "test@test.com").Run()
	for i := 1; i <= n; i++ {
		os.WriteFile(filepath.Join(repoPath, fmt.Sprintf("file%d.txt", i)), []byte(fmt.Sprintf("content %d", i)), 0644)
		exec.Command("git", "-C", repoPath, "add", ".").Run()
		exec.Command("git", "-C", repoPath, "commit", "-m", fmt.Sprintf("Commit %d", i)).Run()
	}
	return repoPath
}

// addCommits 向已有仓库追加 count 个提交（fileN.txt，消息 "Commit N"，N 从 start 起）。
func addCommits(t *testing.T, repoPath string, start, count int) {
	t.Helper()
	for i := start; i < start+count; i++ {
		os.WriteFile(filepath.Join(repoPath, fmt.Sprintf("file%d.txt", i)), []byte(fmt.Sprintf("content %d", i)), 0644)
		exec.Command("git", "-C", repoPath, "add", ".").Run()
		exec.Command("git", "-C", repoPath, "commit", "-m", fmt.Sprintf("Commit %d", i)).Run()
	}
}

// newAppWithCommitCache 构造注入提交历史缓存的 App（模拟 startup 注入，供缓存路径测试）。
func newAppWithCommitCache() *App {
	app := NewApp()
	app.commitHistoryCache = service.NewCommitHistoryCache()
	return app
}

// TestGetCommitHistory_ShaChainBreak_FallsBackToFullScan 注入 headSHA 不匹配且缓存 SHA
// 全不存在的陈旧缓存（模拟 rebase/amend 改写历史致 SHA 链断裂）：incrementalCommits 迭代
// 到根无交集返 nil，resolveCommitHistory 回退全量扫，返真实数据并回写新缓存。
func TestGetCommitHistory_ShaChainBreak_FallsBackToFullScan(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 3)
	app := newAppWithCommitCache()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	key := commitHistoryCacheKey(gitRoot, head)

	// 注入陈旧缓存：headSHA 不匹配（强制走增量），缓存 SHA 全不存在（强制链断）
	staleCommits := []model.Commit{{
		SHA: "nonexistent-sha-1", Message: "stale1\n", Files: []string{},
	}}
	app.commitHistoryCache.Set(key, "different-head-sha", staleCommits)

	// 增量无交集→链断→回退全量扫返真实 3 commit
	got, err := app.GetCommitHistory(repoPath, 10, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("GetCommitHistory: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("链断回退全量应返真实 3 commit: got %d", len(got))
	}
	for _, c := range got {
		if c.SHA == "nonexistent-sha-1" {
			t.Error("链断后不应返回陈旧缓存的 nonexistent SHA")
		}
	}

	// 验证回写新缓存：二次请求 headSHA 匹配应命中（不再重扫）
	got2, err := app.GetCommitHistory(repoPath, 10, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(got2) != 3 {
		t.Errorf("二次应命中回写缓存: got %d", len(got2))
	}
}

// TestGetCommitHistory_CacheHit_UsesInjectedCache 注入 headSHA 匹配的伪造缓存，
// 验证 GetCommitHistory 命中缓存返伪造数据（不重扫真实仓库），证明缓存路径生效。
func TestGetCommitHistory_CacheHit_UsesInjectedCache(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 5)
	app := newAppWithCommitCache()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	key := commitHistoryCacheKey(gitRoot, head)

	// 注入伪造缓存：headSHA 正确（命中），数据为单条 fake
	fakeCommits := []model.Commit{{
		SHA:      "fake-sha",
		ShortSHA: "fake-sha",
		Message:  "fake commit\n",
		Author:   "faker",
		Files:    []string{"fake.go"},
	}}
	app.commitHistoryCache.Set(key, head.Hash().String(), fakeCommits)

	// GetCommitHistory 应命中缓存返 fake（不重扫真实 5 commit）
	got, err := app.GetCommitHistory(repoPath, 5, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("GetCommitHistory: %v", err)
	}
	if len(got) != 1 || got[0].Message != "fake commit\n" {
		t.Errorf("应命中注入缓存返 fake: got %+v", got)
	}
}

// TestGetCommitHistory_CacheHit_ReturnsDeepCopy 命中缓存返回深拷贝，
// 调用方修改返回切片不污染缓存内部数据。
func TestGetCommitHistory_CacheHit_ReturnsDeepCopy(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 3)
	app := newAppWithCommitCache()

	// 首次请求 set 真实缓存
	got1, err := app.GetCommitHistory(repoPath, 3, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if len(got1) != 3 {
		t.Fatalf("first: got %d want 3", len(got1))
	}

	// 篡改返回切片
	got1[0].Message = "mutated"
	got1[0].Files[0] = "mutated-file"

	// 二次请求命中缓存，应返未受污染数据
	got2, err := app.GetCommitHistory(repoPath, 3, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if got2[0].Message == "mutated" {
		t.Error("缓存被返回切片污染：Message")
	}
	if len(got2[0].Files) > 0 && got2[0].Files[0] == "mutated-file" {
		t.Error("缓存被返回切片污染：Files")
	}
}

// TestGetCommitHistory_IncrementalPrepend 首次 set 缓存后新增提交（HEAD 前移），
// 二次请求走增量 prepend：从新 HEAD 迭代收集新提交直到与缓存 SHA 交集，prepend 后返全量。
func TestGetCommitHistory_IncrementalPrepend(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 3)
	app := newAppWithCommitCache()

	// 首次请求 set 缓存（3 commit 全量）
	if _, err := app.GetCommitHistory(repoPath, 10, 0, model.CommitFilter{}); err != nil {
		t.Fatalf("first: %v", err)
	}

	// 新增 2 commit（HEAD 前移）
	addCommits(t, repoPath, 4, 2)

	// 二次请求：headSHA 不同走增量 prepend，应返 5 commit（Commit 5..1 倒序）
	got, err := app.GetCommitHistory(repoPath, 10, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("增量 prepend 后应 5 commit: got %d", len(got))
	}
	if !strings.HasPrefix(got[0].Message, "Commit 5") {
		t.Errorf("首条应 Commit 5: got %q", got[0].Message)
	}
	if !strings.HasPrefix(got[4].Message, "Commit 1") {
		t.Errorf("末条应 Commit 1: got %q", got[4].Message)
	}
}

// TestGetCommitHistory_InvalidateCache 注入陈旧缓存（headSHA 匹配会命中），
// InvalidateCommitHistoryCache 后应清缓存，下次请求全量重扫返真实数据。
func TestGetCommitHistory_InvalidateCache(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 3)
	app := newAppWithCommitCache()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	key := commitHistoryCacheKey(gitRoot, head)

	// 注入陈旧缓存：headSHA 正确（直接请求会命中），数据为 fake
	app.commitHistoryCache.Set(key, head.Hash().String(), []model.Commit{{
		SHA: "fake", Message: "fake\n", Files: []string{},
	}})

	// 前置验证：直接请求命中陈旧缓存
	cached, err := app.GetCommitHistory(repoPath, 10, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("前置验证: %v", err)
	}
	if len(cached) != 1 || cached[0].Message != "fake\n" {
		t.Fatalf("前置验证应命中陈旧缓存: %+v", cached)
	}

	// Invalidate 清缓存
	app.InvalidateCommitHistoryCache(repoPath)

	// 重扫返真实 3 commit
	fresh, err := app.GetCommitHistory(repoPath, 10, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("Invalidate 后: %v", err)
	}
	if len(fresh) != 3 || fresh[0].Message == "fake\n" {
		t.Errorf("Invalidate 后应重扫返真实 3 commit: %+v", fresh)
	}
}

// TestGetCommitHistory_CacheHit_MemoryFilter 命中缓存后过滤分页在缓存全量上内存执行，
// 翻页与过滤不再触 go-git Log 迭代。注入伪造全量缓存验证内存过滤 + offset 翻页正确。
func TestGetCommitHistory_CacheHit_MemoryFilter(t *testing.T) {
	repoPath := setupRepoWithCommits(t, 1) // 真实仓库仅用于打开 + 取 HEAD
	app := newAppWithCommitCache()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	key := commitHistoryCacheKey(gitRoot, head)

	// 注入 4 条伪造缓存：alice 2 条 + bob 2 条，按时间倒序
	injected := []model.Commit{
		{SHA: "s4", Message: "alice fix\n", Author: "alice", Email: "a@x.com", Timestamp: 400, Files: []string{"src/a.go"}},
		{SHA: "s3", Message: "bob add\n", Author: "bob", Email: "b@x.com", Timestamp: 300, Files: []string{"doc/b.md"}},
		{SHA: "s2", Message: "alice init\n", Author: "alice", Email: "a@x.com", Timestamp: 200, Files: []string{"src/c.go"}},
		{SHA: "s1", Message: "bob setup\n", Author: "bob", Email: "b@x.com", Timestamp: 100, Files: []string{"doc/d.md"}},
	}
	app.commitHistoryCache.Set(key, head.Hash().String(), injected)

	// 内存过滤 author=alice + filePath=src：命中 s4（alice src/a.go）与 s2（alice src/c.go）
	got, err := app.GetCommitHistory(repoPath, 10, 0, model.CommitFilter{Author: "alice", FilePath: "src"})
	if err != nil {
		t.Fatalf("GetCommitHistory: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("alice+src 应 2 条: got %d", len(got))
	}
	if got[0].SHA != "s4" {
		t.Errorf("首条应 s4: got %s", got[0].SHA)
	}

	// 内存过滤 + offset 翻页：author=bob 共 2 条，offset=1 取第 2 条
	got2, err := app.GetCommitHistory(repoPath, 10, 1, model.CommitFilter{Author: "bob"})
	if err != nil {
		t.Fatalf("GetCommitHistory offset: %v", err)
	}
	if len(got2) != 1 || got2[0].SHA != "s1" {
		t.Errorf("bob offset=1 应 s1: got %+v", got2)
	}
}
