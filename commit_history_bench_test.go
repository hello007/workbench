package main

// 本文件为提交历史采集层性能 benchmark（09-20 提交历史加载优化），遵循 perf-baseline
// 「先测后优」硬约束：采集层换 CLI git log 前后共用同一基准量化对比，数据写入
// docs/spec/perf-baseline.md「维度：提交历史冷扫」小节。
//
// 冷扫语义：ClearByGitRoot 强制缓存 miss，走 resolveCommitHistory 全量扫路径
// （生产首启 / TTL 过期 / 手动刷新同路径），测 GetCommitHistory 全链
// （采集 + 转 model + 缓存回写），取单页 limit=20（5000 上限内）。
//
// fixture 复用 util/testutil（参数 testing.TB，*testing.B 可直接调用），构造耗时段
// 用 b.StopTimer/b.StartTimer 排除；缓存命中 benchmark 依赖 mtime 未变的约束此处不适用
// （每迭代显式清缓存，不依赖 mtime 时序，无 flaky 风险，见 docs/spec/test-stability.md）。
//
// 运行：go test -bench=BenchmarkGetCommitHistory_ColdScan -benchmem -benchtime=2s -run=^$ ./

import (
	"fmt"
	"path/filepath"
	"testing"

	"workbench/model"
	"workbench/util"
	"workbench/util/testutil"
)

// benchCommitCount benchmark fixture 提交数。300 条覆盖桌面仓库常见量级且构造耗时可控
// （3n 次 git 子进程，全部落在 StopTimer 区间）；上限 5000 场景由单元/集成测试覆盖语义。
const benchCommitCount = 300

// setupBenchRepo 构造含 n 条提交的临时仓库：每提交新增一个 fileN.txt（消息 "Commit N"，
// 与 app_git_cache_test.go setupRepoWithCommits 同构，保证跨 benchmark 可比性）。
func setupBenchRepo(b *testing.B, n int) string {
	b.Helper()
	dir := testutil.InitTempRepo(b)
	testutil.SetupMasterBranch(b, dir)
	for i := 1; i <= n; i++ {
		testutil.WriteFile(b, filepath.Join(dir, fmt.Sprintf("file%d.txt", i)), fmt.Sprintf("content %d\n", i))
		testutil.RunGit(b, dir, "add", ".")
		testutil.RunGit(b, dir, "commit", "-q", "-m", fmt.Sprintf("Commit %d", i))
	}
	return dir
}

// BenchmarkGetCommitHistory_ColdScan 测提交历史冷扫全链耗时与内存分配。
// 每迭代 ClearByGitRoot 强制冷扫（排除清缓存耗时），GetCommitHistory 走全量采集 + 回写缓存
// + 内存分页取 20 条。
func BenchmarkGetCommitHistory_ColdScan(b *testing.B) {
	repoPath := setupBenchRepo(b, benchCommitCount)
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		b.Fatalf("FindGitRoot: %v", err)
	}
	app := newAppWithCommitCache()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		app.commitHistoryCache.ClearByGitRoot(gitRoot)
		b.StartTimer()

		commits, err := app.GetCommitHistory(repoPath, 20, 0, model.CommitFilter{})
		if err != nil {
			b.Fatalf("GetCommitHistory: %v", err)
		}
		if len(commits) != 20 {
			b.Fatalf("应返回 limit=20 条: got %d", len(commits))
		}
		if commits[0].Message != fmt.Sprintf("Commit %d\n", benchCommitCount) {
			b.Fatalf("首条应为最新提交: got %q", commits[0].Message)
		}
	}
}
