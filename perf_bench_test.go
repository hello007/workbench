package main

// 本文件为 v1.4 PR1 性能基线（子项 3）的主包 benchmark 与 MemStats 快照测试，**不改生产代码**。
//
// 目的：采集 NewAppServices 集中装配耗时 + Go 运行时内存占用基线，写入
// docs/spec/perf-baseline.md，供 PR4 性能优化前后对比。禁止盲目优化——本文件只测量。
//
// 复用约束：1000 文件 fixture 复用 util/testutil.WriteFile，不重复造轮子。
// testutil 函数参数为 testing.TB 接口，*testing.T / *testing.B 均满足。
//
// 运行：见 scripts/perf-baseline.sh
//   go test -bench=BenchmarkNewAppServices -benchmem -benchtime=500x -run=^$ ./
//   go test -run TestPerfMemStats -v ./
//
// 主包不设覆盖率门禁（app_*.go 为转发 service 的薄包装），本文件对覆盖率无影响。
// 详见 docs/spec/test-coverage-gate.md。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"workbench/util/testutil"
)

// perfDataDir 创建供 NewAppServices 落盘日志/配置的手动临时目录。
//
// 不用 t.TempDir：NewAppServices 内 util.InitLogger 创建的 lumberjack rotator 会持有
// data/logs/app.log 文件句柄（生产仅 startup 调用一次无此问题；测试/benchmark 多次调用
// 或单次调用后句柄未释放），t.TempDir 的 RemoveAll cleanup 删该文件失败会把测试标记为 FAIL。
// 改用 os.MkdirTemp + best-effort Cleanup（os.RemoveAll 返回值忽略），句柄占用时残留目录
// 留在系统 TEMP，由 OS 最终清理，不阻断测试。
func perfDataDir(tb testing.TB) string {
	tb.Helper()
	dir, err := os.MkdirTemp("", "perf-data-")
	if err != nil {
		tb.Fatalf("mkdtemp: %v", err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// BenchmarkNewAppServices 测 NewAppServices 集中装配 15 个 service/cache（14 service + 1 cache）的纯构造耗时与内存分配。
//
// isDev=false 走纯文件日志（贴近生产构建）。NewAppServices 为纯构造（注释明确：不执行
// startup 副作用——不启 goroutine、不调 os.Exit），可安全 benchmark。
//
// 测量artifact：每次构造 util.InitLogger 创建新 lumberjack rotator 并 slog.SetDefault 覆盖
// 全局 logger，旧 rotator 的 app.log 句柄不主动 Close（累积）。生产仅调用一次无此问题；
// benchmark 用固定 -benchtime=500x 控制句柄累积量（500 个句柄 Windows 可承受），ns/op 稳定。
// 该 artifact 仅影响 benchmark 进程，PR4 对比取 ns/op 趋势即可。
func BenchmarkNewAppServices(b *testing.B) {
	ctx := context.Background()
	dataDir := perfDataDir(b)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s := NewAppServices(ctx, dataDir, false)
		if s == nil {
			b.Fatal("NewAppServices returned nil")
		}
	}
}

// TestPerfMemStats 采集 Go 运行时内存快照，记录 NewAppServices 构造后 + 1000 文件树加载后
// 的 HeapAlloc / HeapSys / NumGC，写入测试日志供 docs/spec/perf-baseline.md 引用。
//
// 三个采样点：
//  1. 基线：runtime.GC 后（含测试框架常驻堆，绝对值偏高，重点看 HeapSys 稳定值与 B/op 对照）
//  2. NewAppServices 构造后：量 15 个 service/cache struct + logger rotator 的常驻内存
//  3. 1000 文件树加载后：量 FileTreeNode 对象图的常驻内存
//
// 运行：go test -run TestPerfMemStats -v ./
//
// 噪声说明：测试进程含 Go 运行时与 testing 框架常驻堆，HeapAlloc 绝对值与 GC 时序相关，
// 构造/文件树增量可能落在 GC 噪声内（甚至为负）；HeapSys（运行时堆系统总量）稳定，更具
// 参考性。精确的每次操作分配量看 benchmark -benchmem 的 B/op（见 service benchmark 结果）。
func TestPerfMemStats(t *testing.T) {
	ctx := context.Background()
	dataDir := perfDataDir(t)

	// 采样点 1：基线
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	t.Logf("采样点1 基线（GC 后）: HeapAlloc=%d KB HeapSys=%d KB NumGC=%d",
		base.HeapAlloc/1024, base.HeapSys/1024, base.NumGC)

	// 采样点 2：NewAppServices 构造后
	s := NewAppServices(ctx, dataDir, false)
	if s == nil {
		t.Fatal("NewAppServices returned nil")
	}
	runtime.GC()
	var afterConstruct runtime.MemStats
	runtime.ReadMemStats(&afterConstruct)
	t.Logf("采样点2 NewAppServices 构造后: HeapAlloc=%d KB HeapSys=%d KB NumGC=%d",
		afterConstruct.HeapAlloc/1024, afterConstruct.HeapSys/1024, afterConstruct.NumGC)

	// 采样点 3：1000 文件树加载后
	dir := t.TempDir()
	for d := 0; d < 10; d++ {
		sub := filepath.Join(dir, fmt.Sprintf("dir%d", d))
		for f := 0; f < 100; f++ {
			testutil.WriteFile(t, filepath.Join(sub, fmt.Sprintf("file%d.txt", f)), "x")
		}
	}
	if _, err := s.fileTreeSvc.GetTree(dir, 3); err != nil {
		t.Fatalf("GetTree failed: %v", err)
	}
	runtime.GC()
	var afterTree runtime.MemStats
	runtime.ReadMemStats(&afterTree)
	t.Logf("采样点3 1000 文件树加载后: HeapAlloc=%d KB HeapSys=%d KB NumGC=%d",
		afterTree.HeapAlloc/1024, afterTree.HeapSys/1024, afterTree.NumGC)

	t.Logf("Δ 构造增量: HeapAlloc %+d KB（参考；受 GC 噪声影响，精确分配见 B/op）",
		int64(afterConstruct.HeapAlloc-base.HeapAlloc)/1024)
	t.Logf("Δ 文件树增量: HeapAlloc %+d KB（参考；受 GC 噪声影响，精确分配见 B/op）",
		int64(afterTree.HeapAlloc-afterConstruct.HeapAlloc)/1024)
}
