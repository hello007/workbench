package service

// 本文件为 v1.4 PR1 性能基线（子项 3）的 benchmark 集合，**不改生产代码**。
//
// 目的：采集 FileTreeService.GetTree / GitService.ScanGitRepos 的真实耗时与内存分配基线，
// 写入 docs/spec/perf-baseline.md，供 PR4 性能优化前后对比。禁止盲目优化——本文件只测量。
//
// 复用约束：fixture 构造复用 util/testutil（RunGit / WriteFile），不在 benchmark 内重复
// 造 git init/config 与批量写文件的轮子。testutil 函数参数为 testing.TB 接口，
// *testing.B 自动满足，可直接传入。
//
// 运行：见 scripts/perf-baseline.sh
//   go test -bench=. -benchmem -benchtime=2s ./service/
//
// benchmark 不纳入覆盖率门禁（_test.go 仅测试二进制编译，coverage-check.sh 跑默认 go test
// 不含 -bench，本文件对覆盖率无影响）。详见 docs/spec/test-coverage-gate.md。

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"workbench/util/testutil"
)

// fileTreeBenchFileCount 单层目录文件数；10 子目录 × 100 文件 = 1000 文件，
// 模拟大型仓库（1000+ 文件）文件树加载场景（PRD 性能基线维度）。
const (
	fileTreeBenchDirs  = 10
	fileTreeBenchFiles = 100
)

// buildFileTreeFixture 在 dir 下构造 10 子目录 × 100 文件 = 1000 文件的目录结构，
// 供 GetTree 基线测量。fixture 构造耗时由调用方用 b.StopTimer/b.StartTimer 排除。
func buildFileTreeFixture(b *testing.B, dir string) {
	b.Helper()
	for d := 0; d < fileTreeBenchDirs; d++ {
		sub := filepath.Join(dir, fmt.Sprintf("dir%d", d))
		for f := 0; f < fileTreeBenchFiles; f++ {
			testutil.WriteFile(b, filepath.Join(sub, fmt.Sprintf("file%d.txt", f)), "x")
		}
	}
}

// BenchmarkFileTreeGetTree_1000Files 测 1000+ 文件目录树冷扫描（缓存未命中）耗时与内存分配。
//
// 每次迭代前 ClearAllCache 清空 treeCache，强制 GetTree 递归 os.ReadDir + 排序 + 节点构造
// （冷路径）。ClearAllCache 仅重建空 map（纳秒级），相对 GetTree 毫秒级耗时可忽略。
//
// 注意：fixture 无 git 仓库（纯 .txt），isGitRepoDir / hasRemote 不触发，gitRepoCache 与
// gitRemoteCache 保持空，测量纯净聚焦文件树扫描本身。
func BenchmarkFileTreeGetTree_1000Files(b *testing.B) {
	dir := b.TempDir()
	b.StopTimer()
	buildFileTreeFixture(b, dir)
	svc := NewFileTreeService()
	b.StartTimer()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		svc.ClearAllCache() // 强制冷扫描，排除缓存命中路径
		nodes, err := svc.GetTree(dir, 3)
		if err != nil {
			b.Fatalf("GetTree failed: %v", err)
		}
		if len(nodes) != fileTreeBenchDirs {
			b.Fatalf("expected %d top-level nodes, got %d", fileTreeBenchDirs, len(nodes))
		}
	}
}

// BenchmarkFileTreeGetTree_Cached 测 GetTree 命中 treeCache（mtime 未变 + TTL 内）的耗时。
//
// 预热：首次 GetTree 实扫并回写 treeCache（b.ResetTimer 排除预热耗时）。
// 测量：后续 GetTree 每层 GetChildren 命中缓存，走 deepCopyNodes + map 查表路径，
// 无 os.ReadDir / 排序开销。对比冷扫描可量化缓存收益（PR4 优化对比依据）。
func BenchmarkFileTreeGetTree_Cached(b *testing.B) {
	dir := b.TempDir()
	b.StopTimer()
	buildFileTreeFixture(b, dir)
	svc := NewFileTreeService()
	// 预热缓存：首次实扫回写 treeCache
	if _, err := svc.GetTree(dir, 3); err != nil {
		b.Fatalf("warmup GetTree failed: %v", err)
	}
	b.StartTimer()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		nodes, err := svc.GetTree(dir, 3) // 命中 treeCache
		if err != nil {
			b.Fatalf("GetTree failed: %v", err)
		}
		if len(nodes) != fileTreeBenchDirs {
			b.Fatalf("expected %d top-level nodes, got %d", fileTreeBenchDirs, len(nodes))
		}
	}
}

// scanGitReposBenchCount 扫描基线的仓库数量，模拟工作目录下多仓库场景。
const scanGitReposBenchCount = 10

// BenchmarkScanGitRepos_10Repos 测扫描 10 个嵌套 git 仓库的耗时与内存分配。
//
// fixture：root 下 2 个 group × 5 个 repo = 10 个嵌套 git 仓库，用 testutil.RunGit
// 复用 git init + config 身份配置（不重复造轮子）。构造耗时由 b.StopTimer/b.StartTimer 排除。
//
// 用 NewGitService（无 cache）测纯扫描路径（scanDir 递归 + .git 预筛），为 PR4 优化对比基线。
// 生产 app_services.go 用 NewGitServiceWithCache（带 mtime 缓存），二次扫描近乎瞬时——
// 缓存收益由 PR4 据本基线对比量化，本基线聚焦冷扫描成本。
func BenchmarkScanGitRepos_10Repos(b *testing.B) {
	root := b.TempDir()
	b.StopTimer()
	for i := 0; i < scanGitReposBenchCount; i++ {
		repo := filepath.Join(root, fmt.Sprintf("group%d", i/5), fmt.Sprintf("repo%d", i))
		if err := os.MkdirAll(repo, 0o755); err != nil {
			b.Fatalf("mkdir %s: %v", repo, err)
		}
		testutil.RunGit(b, repo, "init")
		testutil.RunGit(b, repo, "config", "user.email", "test@test.com")
		testutil.RunGit(b, repo, "config", "user.name", "test")
	}
	svc := NewGitService()
	b.StartTimer()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		repos := svc.ScanGitRepos(root)
		if len(repos) != scanGitReposBenchCount {
			b.Fatalf("expected %d repos, got %d: %v", scanGitReposBenchCount, len(repos), repos)
		}
	}
}
