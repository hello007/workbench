package main

// 本文件为性能测试补全任务（09-25）的长跑稳定性（soak）测试 + 内存泄漏自动检测（主包部分），
// **不改生产代码**。GetCommitHistory 为 App 方法（依赖 a.commitHistoryCache），仅主包可测全链路；
// service 层对应缓存层的有界性由 service/soak_leak_test.go 的 TestSoakCommitHistoryCacheGrowth 覆盖。
//
// 循环形态：高频 GetCommitHistory（缓存命中路径深拷贝分页）+ 每 10 轮手动
// InvalidateCommitHistoryCache（App 单仓失效入口，app_git.go）强制全量重扫 fork git log，
// 覆盖「命中 / 手动失效 / 全量重扫回写」三形态的稳定性。
//
// 三档轮次（决策 (c)，flag 解析优先级：-soak-rounds > -short 缩减 > 默认档）：
// 默认档 30 轮（每轮至多一次 git log fork，~50-100ms/次为预算大头）；Short 档 8 轮；
// 深跑档 -soak-rounds=N 按 N*3/20 取整折算（默认档 30 ≈ 基数 200/6.67），见 soakForkRounds。
//
// 泄漏断言（决策 (a)）：循环前后各做两次 runtime.GC() 后堆快照，HeapAlloc 增量
// ≤ -leak-heap-mb（主包默认 16MB，宽于 service 包：CommitHistoryCache 深拷贝瞬时
// 分配压力大，口径更宽松）且 goroutine 增量 ≤2。主包不挂 goleak TestMain（决策 (b)：
// websocket Hub 等常驻 goroutine 误报面大），goroutine 门禁即此 Δ≤2 断言。
//
// 测量 artifact：NewAppServices 每次构造重置全局 logger 致 lumberjack 句柄累积
// （perf_bench_test.go 注释记载）——本测试仅构造一次，无累积问题。
//
// flag 说明：本包与 service 包各自注册同名 -soak-rounds / -leak-heap-mb（heap 阈值默认
// 值不同：主包 16MB / service 8MB）。同名 flag 在各自测试二进制中独立注册解析，同跑无
// 冲突；手动跨包传参须按包分开（go test ./ -soak-rounds=2000 与 go test ./service/ -soak-rounds=2000）。
//
// 稳定性约束（docs/spec/test-stability.md 全文有效）：禁 time.Sleep；禁固定端口（本测试
// 不涉网络）；缓存失效由手动入口驱动，不依赖 mtime 时序精度。

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// 包级 flag：语义见文件头「flag 说明」。
var (
	soakRoundsFlag = flag.Int("soak-rounds", 0, "soak 深跑档轮次基数（GetCommitHistory 按 N*3/20 取整折算）；0=按档位取默认")
	leakHeapMBFlag = flag.Int("leak-heap-mb", 16, "泄漏断言 heap 增量阈值（MB），宁松勿紧，防 CI 噪声")
)

// 轮次档位常量（决策 (c) 推荐档）。检查点采样数与采样/断言辅助在 util/testutil（soak.go，
// 跨包共用收敛），本文件仅保留依赖包级 flag 的 fork 轮次折算逻辑。
const (
	soakCommitDefaultRounds = 30 // GetCommitHistory 默认档轮次（fork git log 路径）
	soakCommitShortRounds   = 8
)

// soakForkRounds fork git 子进程路径的轮次折算：显式 -soak-rounds=N 时按 N*3/20 取整
// （默认档锚定：GetCommitHistory 默认 30 ≈ 基数 200/6.67），避免深跑档 fork 轮次失控
// 超时；折算不足 1 时钳 1（整数除法向下取整，小 N 会折出 0，写法对齐 util/testutil
// SoakCheckpointInterval 的保底风格），保证深跑档 fork 路径至少执行一轮而非零轮空跑；
// 未传 flag 时退回档位值（-short 缩减档 / 默认档）。
func soakForkRounds(defaultRounds, shortRounds, ratioNum, ratioDen int) int {
	if *soakRoundsFlag > 0 {
		// 小 N 折算可能为 0（如 N≤6 时 N*3/20），钳 1 保证至少执行一轮
		if rounds := *soakRoundsFlag * ratioNum / ratioDen; rounds > 0 {
			return rounds
		}
		return 1
	}
	if testing.Short() {
		return shortRounds
	}
	return defaultRounds
}

// TestSoakGetCommitHistory 提交历史全链路长跑（fork git 路径）：App 级装配
// （NewAppServices，复用 perfDataDir 规避 lumberjack 句柄致 t.TempDir 清理失败）+
// GetCommitHistory 高频循环，每 10 轮 InvalidateCommitHistoryCache 强制下轮全量重扫
// （App 单仓手动失效入口，不依赖 mtime 时序）。覆盖缓存命中深拷贝分页、缓存条目
// 覆盖写回收、git log 子进程 Wait 闭环三面的长跑稳定性。
func TestSoakGetCommitHistory(t *testing.T) {
	ctx := context.Background()
	dataDir := perfDataDir(t) // perf_bench_test.go：手动临时目录，规避 lumberjack 句柄残留
	app := &App{AppServices: NewAppServices(ctx, dataDir, false)}

	// fixture：master 分支 5 提交（testutil 复用，不重复造 git fixture 轮子）
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	for i := 1; i <= 5; i++ {
		testutil.WriteFile(t, filepath.Join(repo, fmt.Sprintf("file%d.txt", i)), fmt.Sprintf("content %d\n", i))
		testutil.RunGit(t, repo, "add", ".")
		testutil.RunGit(t, repo, "commit", "-m", fmt.Sprintf("Commit %d", i))
	}

	// 预热：首次全量扫 fork git log 回写 commitHistoryCache，排除惰性初始化
	if commits, err := app.GetCommitHistory(repo, 20, 0, model.CommitFilter{}); err != nil || len(commits) != 5 {
		t.Fatalf("预热 GetCommitHistory: err=%v commits=%d", err, len(commits))
	}

	before, beforeGoroutines := testutil.SoakLeakBaseline()
	// fork 路径轮次折算：默认 30 ≈ 基数 200/6.67，深跑档 -soak-rounds=N 时取 N*3/20
	rounds := soakForkRounds(soakCommitDefaultRounds, soakCommitShortRounds, 3, 20)
	interval := testutil.SoakCheckpointInterval(rounds)
	var samples []testutil.SoakSample

	for i := 1; i <= rounds; i++ {
		// 命中/失效混合：每 10 轮走 App 单仓失效入口清提交历史缓存与行数统计缓存
		if i%10 == 0 {
			app.InvalidateCommitHistoryCache(repo)
		}
		commits, err := app.GetCommitHistory(repo, 20, 0, model.CommitFilter{})
		if err != nil {
			t.Fatalf("第 %d 轮 GetCommitHistory: %v", i, err)
		}
		// 抽查断言：每 10 轮 + 末轮（fork 轮次少，抽查密度相应高）
		if i%10 == 0 || i == rounds {
			if len(commits) != 5 {
				t.Fatalf("第 %d 轮提交数 %d，应为 5", i, len(commits))
			}
		}
		if i%interval == 0 {
			samples = append(samples, testutil.SoakSampleAt(i))
		}
	}
	testutil.SoakLogSamples(t, "GetCommitHistory", samples)

	testutil.SoakAssertNoLeak(t, "GetCommitHistory", before, beforeGoroutines, *leakHeapMBFlag)
}
