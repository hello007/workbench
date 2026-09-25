// soak.go 提供 soak 长跑稳定性测试与内存泄漏断言的跨包共用辅助（service 与主包
// soak_leak_test.go 共用，与 testutil.go 的 git fixture 辅助同属「跨包共用测试辅助入
// util/testutil」收敛规则，禁各 _test.go 重复定义同义实现）。
//
// 仅 _test.go 调用，不进生产二进制。参数类型 testing.TB（*testing.T/*testing.B 均满足）。
//
// 设计要点（09-25 性能测试补全任务决策 (a)，宁松勿紧）：
//   - 泄漏断言口径：循环前后各做两次 runtime.GC() 后的 HeapAlloc 增量（第一轮 GC 触发
//     finalizer 调度执行，第二轮回收 finalizer 释放的引用链，快照才收敛为当前存活堆）
//   - goroutine 数增量。阈值由调用方传入（service 8MB / 主包 16MB，-leak-heap-mb 可调），
//     只抓趋势级灾难泄漏，偶发噪声靠宽松阈值兜底。
//   - 趋势采样：按轮次取固定检查点（默认 10 个）输出 NumGoroutine/HeapAlloc/HeapSys/NumGC
//     趋势表，仅供 perf-baseline.md 引用，不参与断言（GOGC 动态调整与碎片化可能造出伪
//     线性段，阈值断言只抓绝对增量）。按轮采样无 sleep，符合 test-stability.md 约定。
package testutil

import (
	"runtime"
	"testing"
)

// SoakCheckpoints 每测试检查点采样数（每 rounds/SoakCheckpoints 轮一个检查点）。
const SoakCheckpoints = 10

// GcHeapSnapshot 双 runtime.GC() 后堆快照：双 GC 原因见包注释；双 GC 后 HeapAlloc 即
// 当前存活堆，是泄漏断言的首选口径（HeapSys 只增不减、受分配峰值影响，仅趋势记录不作断言）。
func GcHeapSnapshot() runtime.MemStats {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms
}

// SoakSample 单检查点采样行（轮次 + 四项运行时指标）。
type SoakSample struct {
	round       int
	goroutines  int
	heapAllocKB uint64
	heapSysKB   uint64
	numGC       uint32
}

// Round 返回采样对应的轮次（供测试自定义趋势输出）。
func (s SoakSample) Round() int { return s.round }

// SoakSampleAt 取当前检查点采样（ReadMemStats 短暂 STW，10 次/测试可忽略）。
func SoakSampleAt(round int) SoakSample {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return SoakSample{
		round:       round,
		goroutines:  runtime.NumGoroutine(),
		heapAllocKB: ms.HeapAlloc / 1024,
		heapSysKB:   ms.HeapSys / 1024,
		numGC:       ms.NumGC,
	}
}

// SoakLogSamples 输出检查点趋势表与前/后 20% 均值趋势行（t.Logf，供 perf-baseline.md 引用）。
// 趋势仅输出不参与断言。轮次少于 5 个检查点时无趋势意义，仅输出表格。
func SoakLogSamples(t testing.TB, label string, samples []SoakSample) {
	t.Helper()
	t.Logf("[%s] 检查点趋势表：", label)
	t.Logf("| 轮次 | NumGoroutine | HeapAlloc KB | HeapSys KB | NumGC |")
	t.Logf("|---|---|---|---|---|")
	for _, s := range samples {
		t.Logf("| %d | %d | %d | %d | %d |", s.round, s.goroutines, s.heapAllocKB, s.heapSysKB, s.numGC)
	}
	if len(samples) < 5 {
		return
	}
	head := samples[:len(samples)/5]
	tail := samples[len(samples)*4/5:]
	headAvg, tailAvg := soakAvgHeapKB(head), soakAvgHeapKB(tail)
	lastRound := samples[len(samples)-1].round
	t.Logf("[%s] 趋势：前 20%% 均值 %.0f KB，后 20%% 均值 %.0f KB，增长 %.0f KB（%.2f KB/千轮）",
		label, headAvg, tailAvg, tailAvg-headAvg, (tailAvg-headAvg)*1000/float64(lastRound))
}

func soakAvgHeapKB(samples []SoakSample) float64 {
	var sum uint64
	for _, s := range samples {
		sum += s.heapAllocKB
	}
	return float64(sum) / float64(len(samples))
}

// SoakLeakBaseline 循环前基线：双 GC 堆快照 + goroutine 数（GC 后取，排除退出中 goroutine 抖动）。
func SoakLeakBaseline() (runtime.MemStats, int) {
	return GcHeapSnapshot(), runtime.NumGoroutine()
}

// SoakAssertNoLeak 循环后泄漏断言（宁松勿紧）：双 GC 后 HeapAlloc 增量 ≤ thresholdMB
// 且 goroutine 增量 ≤2，只抓趋势级灾难泄漏，偶发噪声靠宽松阈值兜底。
// thresholdMB 由调用方传入本包 -leak-heap-mb flag 解析值（service 8 / 主包 16 缺省）。
// 注：-race 下检测器使堆占用放大数倍，heap 数值断言会失真（goroutine 断言不受影响）；
// CI 不跑 -race，手动 -race 复核 goroutine 泄漏时忽略 heap 断言。
func SoakAssertNoLeak(t testing.TB, label string, before runtime.MemStats, beforeGoroutines, thresholdMB int) {
	t.Helper()
	after := GcHeapSnapshot()
	deltaHeapMB := (int64(after.HeapAlloc) - int64(before.HeapAlloc)) >> 20
	if deltaHeapMB > int64(thresholdMB) {
		t.Errorf("[%s] HeapAlloc 增量 %d MB 超阈值 %d MB：疑似内存泄漏", label, deltaHeapMB, thresholdMB)
	}
	deltaGoroutines := runtime.NumGoroutine() - beforeGoroutines
	if deltaGoroutines > 2 {
		t.Errorf("[%s] goroutine 增量 %d 超阈值 2：疑似 goroutine 泄漏", label, deltaGoroutines)
	}
	t.Logf("[%s] 泄漏断言：HeapAlloc 增量 %+d MB（阈值 %d MB），goroutine 增量 %+d（阈值 ≤2）",
		label, deltaHeapMB, thresholdMB, deltaGoroutines)
}

// SoakCheckpointInterval 检查点采样间隔：rounds/SoakCheckpoints 向上保底 1（短轮次逐轮采样）。
func SoakCheckpointInterval(rounds int) int {
	if interval := rounds / SoakCheckpoints; interval > 0 {
		return interval
	}
	return 1
}
