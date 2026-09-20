//go:build integration

package main

// 提交历史大仓库（>5000 提交，overflow 路径）集成测试。
//
// 用 git fast-import 单子进程构造 5001 条线性提交（逐条 git commit 约 4-8 分钟不可接受，
// fast-import 秒级），驱动 GetCommitHistory 的超限路径：fullScanCommits 探测超限 →
// fetchCommitHistoryFromGit CLI 流式过滤分页（不缓存、凑够 limit 提前终止）。
// 同时覆盖 GetRepoStats 超限采样（Sampled=true）。
//
// 运行：go test -tags=integration -run TestCommitHistoryOverflow ./ （git 须在 PATH）。

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
	"workbench/util/testutil"
)

// overflowCommitCount 构造的提交数：刚好超过 CommitHistoryMaxEntries（5000）1 条，
// 触发 overflow 判定的最小超标场景。
const overflowCommitCount = service.CommitHistoryMaxEntries + 1

// TestCommitHistoryOverflow_PagingAndSampling 大仓库 overflow 路径端到端：
// 全量扫探测超限 → 不缓存 → CLI 流式分页翻页正确 + 采样统计标记 Sampled。
func TestCommitHistoryOverflow_PagingAndSampling(t *testing.T) {
	itRequireGit(t)
	repoPath := buildFastImportRepo(t, overflowCommitCount)
	app := itNewApp()

	// 第 1 页：HEAD 起 20 条（overflow 走流式路径）
	page1, err := app.GetCommitHistory(repoPath, 20, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 20 {
		t.Fatalf("page1 应 20 条: got %d", len(page1))
	}
	// HEAD 为最新提交 Commit 5001，倒序首条即它
	if page1[0].Message != "Commit 5001\n" {
		t.Errorf("page1 首条应为 Commit 5001: got %q", page1[0].Message)
	}
	for i, c := range page1 {
		want := fmt.Sprintf("Commit %d\n", 5001-i)
		if c.Message != want {
			t.Fatalf("page1[%d] 消息应 %q: got %q", i, want, c.Message)
		}
		if len(c.Files) != 1 {
			t.Fatalf("page1[%d] 应含 1 个变更文件: got %v", i, c.Files)
		}
	}

	// 第 2 页 offset=20：应从 Commit 4981 起
	page2, err := app.GetCommitHistory(repoPath, 20, 20, model.CommitFilter{})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 20 || page2[0].Message != "Commit 4981\n" {
		t.Fatalf("page2 应从 Commit 4981 起 20 条: got %d 条, 首条 %q", len(page2), firstMsg(page2))
	}

	// 过滤翻页：keyword 精确匹配单条（流式路径内存过滤生效）
	filtered, err := app.GetCommitHistory(repoPath, 20, 0, model.CommitFilter{Keyword: "Commit 5001"})
	if err != nil {
		t.Fatalf("filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Message != "Commit 5001\n" {
		t.Fatalf("keyword 过滤应命中 1 条 Commit 5001: got %+v", firstMsg(filtered))
	}

	// 大仓库统计：复用已扫前 5000 条采样，Sampled=true
	stats, err := app.GetRepoStats(repoPath, "all")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if !stats.Sampled {
		t.Error("超限仓库统计 Sampled 应为 true")
	}
	if stats.TotalCommits != service.CommitHistoryMaxEntries {
		t.Errorf("采样统计应基于前 %d 条: got %d", service.CommitHistoryMaxEntries, stats.TotalCommits)
	}

}

// TestCommitHistoryOverflow_CacheNotWritten 超限仓库不写缓存（cap 5000 策略不变）：
// 两次请求都走现扫，缓存 Get 始终 miss。
func TestCommitHistoryOverflow_CacheNotWritten(t *testing.T) {
	itRequireGit(t)
	repoPath := buildFastImportRepo(t, overflowCommitCount)
	app := itNewApp()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}

	if _, err := app.GetCommitHistory(repoPath, 5, 0, model.CommitFilter{}); err != nil {
		t.Fatalf("first: %v", err)
	}
	// 期望键经 commitHistoryCacheKey 构造（head 取 repo.Head()），不硬编码键格式，
	// 键格式后续变更时本测试不会产生假阳性
	repo, err := git.PlainOpen(gitRoot)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if _, _, found := app.commitHistoryCache.Get(commitHistoryCacheKey(gitRoot, head)); found {
		t.Error("超限仓库不应写缓存")
	}
}

// firstMsg 取提交切片首条消息（空切片返空串），断言消息辅助。
func firstMsg(commits []model.Commit) string {
	if len(commits) == 0 {
		return ""
	}
	return commits[0].Message
}

// buildFastImportRepo 经 git fast-import 构造含 n 条线性提交的仓库：
// 提交 i（1 起）新增 fileN.txt，消息 "Commit N"，committer date 递增保序。
// 单子进程秒级完成（对比逐条 commit 的分钟级）。
func buildFastImportRepo(t *testing.T, n int) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "big-repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	testutil.RunGit(t, dir, "init")
	testutil.RunGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/master")

	var b strings.Builder
	for i := 1; i <= n; i++ {
		content := fmt.Sprintf("content %d\n", i)
		b.WriteString("commit refs/heads/master\n")
		fmt.Fprintf(&b, "mark :%d\n", i)
		ts := 1700000000 + i
		sig := fmt.Sprintf("Test <test@test.com> %d +0800", ts)
		fmt.Fprintf(&b, "author %s\n", sig)
		fmt.Fprintf(&b, "committer %s\n", sig)
		msg := fmt.Sprintf("Commit %d\n", i)
		fmt.Fprintf(&b, "data %d\n%s", len(msg), msg)
		if i > 1 {
			fmt.Fprintf(&b, "from :%d\n", i-1)
		}
		fmt.Fprintf(&b, "M 100644 inline file%d.txt\ndata %d\n%s\n", i, len(content), content)
	}

	cmd := exec.Command("git", "fast-import", "--quiet")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(b.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git fast-import failed: %v\n%s", err, out)
	}
	return dir
}
