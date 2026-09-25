package service

// 本文件为性能测试补全任务（09-25）的长跑稳定性（soak）测试 + 内存泄漏自动检测，**不改生产代码**。
//
// 目的：核心读路径高频循环（缓存命中/手动失效混合），按检查点采样 goroutine/堆内存趋势，
// 并以宽松阈值断言趋势级泄漏（宁松勿紧：CI 偶发失败比漏检危害大）。量化数据沉淀
// docs/spec/perf-baseline.md「长跑稳定性」「泄漏检测」两章节。
//
// 覆盖路径（本文件 4 条 + 仓库根 soak_leak_test.go 的 GetCommitHistory 主包全链路）：
//   - TestSoakFileTreeGetTree          文件树（treeCache 命中 + 手动 InvalidateCache 失效混合）
//   - TestSoakScanGitRepos             仓库扫描（scanCache 命中 + ClearScanCache 失效混合，生产缓存路径）
//   - TestSoakGetLocalChanges          本地变更（每轮 fork git status，fork 路径为轮次预算大头）
//   - TestSoakCommitHistoryCacheGrowth 提交历史缓存层（单键覆盖写 + 双向深拷贝不累积）
//
// 三档轮次（决策 (c)，flag 解析优先级：-soak-rounds > -short 缩减 > 默认档）：
//   - 默认档（CI 裸 go test ./... 即此档）：GetTree/Scan/CacheGrowth 200 轮，GetLocalChanges 40 轮
//   - Short 档（-short）：GetTree/Scan/CacheGrowth 50 轮，GetLocalChanges 10 轮，保 -short 增量 <30s
//   - 深跑档（-soak-rounds=N）：非 fork 路径直接取 N 轮；fork git 路径按默认档比例折算
//     （GetLocalChanges = N/5，默认档 40 = 200/5），见 soakForkRounds 注释
//
// 泄漏断言（决策 (a)）：循环前后各做两次 runtime.GC() 后堆快照，HeapAlloc 增量
// ≤ -leak-heap-mb（本包默认 8MB）且 goroutine 增量 ≤2。双 GC 原因：第一轮 GC 触发
// finalizer 调度，第二轮回收 finalizer 释放的引用链，快照才收敛为当前存活堆。
//
// goleak（决策 (b)）：go.uber.org/goleak v1.3.0（版本钉死，不追 main 分支未发版 API）。
// goroutine 泄漏检测经 TestMain 三档分策（全仓首个 TestMain），判定优先级：
// 深跑档 > -short 跳过 > 默认档（深跑档最高，含 -short 组合，与 soakRounds/soakForkRounds
// 的 flag 优先级一致）：
//   - -soak-rounds>0（深跑档）：goleak.VerifyTestMain 硬门禁，发现泄漏打印栈并非零退出
//   - -short（未传 -soak-rounds）：完全跳过 goleak，保 <30s 约束
//   - 默认档（CI 裸 go test ./... 即此档）：观察模式——m.Run() 后 goleak.Find 结果仅
//     stderr 告警，不改退出码。原因：service 包既有 7 个测试文件启动 goroutine（并发
//     压测的 worker、chat fake 进程输出泵等），包级硬门禁对慢退出/残留栈有误报风险，
//     先观察模式跑绿收集基线，确认干净后再收紧为硬门禁（良性栈用 IgnoreTopFunction
//     全限定名逐个登记，不放宽全包豁免）。
//
// 稳定性约束（docs/spec/test-stability.md 全文有效）：禁 time.Sleep；禁固定端口；
// 缓存失效全部由手动入口（InvalidateCache/ClearScanCache）驱动，不依赖 mtime 时序精度。
//
// flag 说明：本包与主包各自注册同名 -soak-rounds / -leak-heap-mb（heap 阈值默认值不同：
// service 8MB / 主包 16MB）。同名 flag 在各自测试二进制中独立注册解析，同跑无冲突；
// 手动跨包传参须按包分开（go test ./service/ -soak-rounds=2000 与 go test ./ -soak-rounds=2000）。

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/goleak"

	"workbench/model"
	"workbench/util/testutil"
)

// 包级 flag：语义见文件头「flag 说明」。
var (
	soakRoundsFlag = flag.Int("soak-rounds", 0, "soak 深跑档轮次基数（非 fork 路径直接取 N，fork git 路径按默认档比例折算）；0=按档位取默认")
	leakHeapMBFlag = flag.Int("leak-heap-mb", 8, "泄漏断言 heap 增量阈值（MB），宁松勿紧，防 CI 噪声")
)

// 轮次档位常量（决策 (c) 推荐档）。检查点采样数与采样/断言辅助在 util/testutil（soak.go，
// 跨包共用收敛），本文件仅保留依赖包级 flag 的档位折算逻辑。
const (
	soakTreeDefaultRounds    = 200 // GetTree 默认档轮次（纯内存/文件系统路径）
	soakTreeShortRounds      = 50
	soakScanDefaultRounds    = 200 // ScanGitRepos 默认档轮次（.git 预筛纯库路径）
	soakScanShortRounds      = 50
	soakChangesDefaultRounds = 40 // GetLocalChanges 默认档轮次（每轮 fork git status）
	soakChangesShortRounds   = 10
	soakCacheDefaultRounds   = 200 // 提交历史缓存层默认档轮次（纯内存深拷贝）
	soakCacheShortRounds     = 50
)

// TestMain 全仓首个 TestMain（service 包）。goroutine 泄漏检测三档分策，见文件头「goleak」。
func TestMain(m *testing.M) {
	// 手动解析：三档判定须在 m.Run() 之前读取 -short 与 -soak-rounds 的解析值；
	// flag.Parse 之后 m.Run() 检测 flag.Parsed() 为真跳过二次解析（Go 1.13+ 语义），不冲突。
	flag.Parse()

	if *soakRoundsFlag > 0 {
		// 深跑档：显式深跑即有意观察泄漏，goleak 硬门禁（发现泄漏打印栈并非零退出）。
		// 判定优先级最高（含 -short 组合：显式 -soak-rounds 压过 -short，与 soakRounds/
		// soakForkRounds 的 flag 优先级一致），深跑档不因 -short 跳过门禁，守「深跑档 =
		// 硬门禁」语义
		goleak.VerifyTestMain(m)
		return
	}
	if testing.Short() {
		// Short 档：完全跳过 goleak（含观察模式），保 -short 增量 <30s 约束
		os.Exit(m.Run())
	}
	// 默认档（CI）：观察模式，仅告警不改退出码（两步走第一步，见文件头）
	code := m.Run()
	if leak := goleak.Find(); leak != nil {
		fmt.Fprintf(os.Stderr, "goleak 观察模式：检测到疑似 goroutine 泄漏（不影响退出码，仅记录基线）：%v\n", leak)
	}
	os.Exit(code)
}

// soakRounds 当前档位轮次：显式 -soak-rounds=N 优先（非 fork 路径直接取 N），其次
// -short 缩减档，最后默认档。
func soakRounds(defaultRounds, shortRounds int) int {
	if *soakRoundsFlag > 0 {
		return *soakRoundsFlag
	}
	if testing.Short() {
		return shortRounds
	}
	return defaultRounds
}

// soakForkRounds fork git 子进程路径的轮次折算：显式 -soak-rounds=N 时按 N 的既定分数
// 取整折算（默认档比例锚定：GetLocalChanges 默认 40 = 基数 200 的 1/5，即 N/5；主包
// GetCommitHistory 默认 30 ≈ 200/6.67，即 N*3/20），避免深跑档 fork 轮次失控超时；
// 折算不足 1 时钳 1（整数除法向下取整，小 N 会折出 0，写法对齐 util/testutil
// SoakCheckpointInterval 的保底风格），保证深跑档 fork 路径至少执行一轮而非零轮空跑；
// 未传 flag 时退回 soakRounds 的档位值。
func soakForkRounds(defaultRounds, shortRounds, ratioNum, ratioDen int) int {
	if *soakRoundsFlag > 0 {
		// 小 N 折算可能为 0（如 N≤5 时 N/5、N≤6 时 N*3/20），钳 1 保证至少执行一轮
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

// TestSoakFileTreeGetTree 文件树读路径长跑：循环 GetTree(dir, 3)，每 10 轮手动
// InvalidateCache 强制下一轮根目录重拉（失效混合：其余轮次命中 treeCache）。
// 失效由手动入口驱动而非写文件等 mtime 变化（不依赖 NTFS mtime 时序精度，无 sleep）。
//
// 结束后断言 treeCache 条目有界：GetTree(dir,3) 理论上界 = 根 + 10 个子目录 = 11 键，
// InvalidateCache 只清根键、子目录键稳定不累积；上限放宽到 32（宁松勿紧）。
func TestSoakFileTreeGetTree(t *testing.T) {
	// fixture：10 子目录 × 100 文件 = 1000 文件。testutil.WriteFile 循环构造，不复用
	// perf_bench_test.go 的 buildFileTreeFixture（其参数为 *testing.B，签名不匹配）。
	dir := t.TempDir()
	for d := 0; d < 10; d++ {
		sub := filepath.Join(dir, fmt.Sprintf("dir%d", d))
		for f := 0; f < 100; f++ {
			testutil.WriteFile(t, filepath.Join(sub, fmt.Sprintf("file%d.txt", f)), "x")
		}
	}

	svc := NewFileTreeService()
	// 预热：首次实扫回写 treeCache 并填 gitRepoCache（纯 os.Stat 不 fork git），
	// 把惰性初始化（map 首建、缓存首写）排除在泄漏断言基线之外
	if _, err := svc.GetTree(dir, 3); err != nil {
		t.Fatalf("预热 GetTree: %v", err)
	}

	before, beforeGoroutines := testutil.SoakLeakBaseline()
	rounds := soakRounds(soakTreeDefaultRounds, soakTreeShortRounds)
	interval := testutil.SoakCheckpointInterval(rounds)
	var samples []testutil.SoakSample

	for i := 1; i <= rounds; i++ {
		// 命中/失效混合：每 10 轮手动 InvalidateCache 清根目录键，下一轮 GetChildren miss 实扫
		if i%10 == 0 {
			svc.InvalidateCache(dir)
		}
		nodes, err := svc.GetTree(dir, 3)
		if err != nil {
			t.Fatalf("第 %d 轮 GetTree: %v", i, err)
		}
		// 抽查断言：每 50 轮一次 + 末轮兜底（每轮断言太频），保证功能在长跑中不漂移
		if i%50 == 0 || i == rounds {
			if len(nodes) != 10 {
				t.Fatalf("第 %d 轮 GetTree 顶层节点数 %d，应为 10", i, len(nodes))
			}
		}
		if i%interval == 0 {
			samples = append(samples, testutil.SoakSampleAt(i))
		}
	}
	testutil.SoakLogSamples(t, "GetTree", samples)

	// treeCache 有界断言：条目数不随轮次增长（单次 GetTree 键集合固定）
	svc.treeCache.mu.Lock()
	entries := len(svc.treeCache.entries)
	svc.treeCache.mu.Unlock()
	if entries > 32 {
		t.Errorf("treeCache 条目数 %d 超上限 32：疑似缓存无界增长", entries)
	}
	t.Logf("[GetTree] treeCache 条目数 %d（上界 32，理论 11 键）", entries)

	testutil.SoakAssertNoLeak(t, "GetTree", before, beforeGoroutines, *leakHeapMBFlag)
}

// TestSoakScanGitRepos 仓库扫描读路径长跑：生产缓存路径（NewGitServiceWithCache，同
// app_services.go 装配），循环 ScanGitRepos(root)，每 10 轮 ClearScanCache 强制全扫
// （手动失效入口驱动，不依赖 mtime 时序）。每轮扫描经 saveLocked 落盘缓存文件，
// 覆盖「扫描 + 序列化落盘」完整生产链路的稳定性。
func TestSoakScanGitRepos(t *testing.T) {
	// fixture：root 下 2 group × 5 repo = 10 嵌套 git 仓库（testutil.RunGit init+config，
	// 同 BenchmarkScanGitRepos_10Repos 写法，不重复造 git fixture 轮子）
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		repo := filepath.Join(root, fmt.Sprintf("group%d", i/5), fmt.Sprintf("repo%d", i))
		if err := os.MkdirAll(repo, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", repo, err)
		}
		testutil.RunGit(t, repo, "init")
		testutil.RunGit(t, repo, "config", "user.email", "test@test.com")
		testutil.RunGit(t, repo, "config", "user.name", "test")
	}

	// 生产缓存路径：缓存文件落独立临时目录（与 fixture 隔离）
	svc := NewGitServiceWithCache(filepath.Join(t.TempDir(), "scan_cache.json"))
	// 预热：首扫回写扫描缓存，排除惰性初始化
	if repos := svc.ScanGitRepos(root); len(repos) != 10 {
		t.Fatalf("预热扫描仓库数 %d，应为 10", len(repos))
	}

	before, beforeGoroutines := testutil.SoakLeakBaseline()
	rounds := soakRounds(soakScanDefaultRounds, soakScanShortRounds)
	interval := testutil.SoakCheckpointInterval(rounds)
	var samples []testutil.SoakSample

	for i := 1; i <= rounds; i++ {
		// 命中/失效混合：每 10 轮 ClearScanCache 清该工作目录缓存，本轮即全扫 + 重新落盘
		if i%10 == 0 {
			svc.ClearScanCache(root)
		}
		repos := svc.ScanGitRepos(root)
		if i%10 == 0 || i == rounds {
			if len(repos) != 10 {
				t.Fatalf("第 %d 轮扫描仓库数 %d，应为 10", i, len(repos))
			}
		}
		if i%interval == 0 {
			samples = append(samples, testutil.SoakSampleAt(i))
		}
	}
	testutil.SoakLogSamples(t, "ScanGitRepos", samples)

	testutil.SoakAssertNoLeak(t, "ScanGitRepos", before, beforeGoroutines, *leakHeapMBFlag)
}

// TestSoakGetLocalChanges 本地变更读路径长跑（fork git 路径）：每轮 GetLocalChanges
// 经 exec git status 子进程采集（生产 util.GitCommand.Execute 30s 超时兜底），
// 验证子进程 Wait 闭环下无句柄/僵尸进程累积。fixture 留一处未提交修改保证每轮
// 结果非空可抽查。fork 单轮约 20-60ms（Windows git 进程启动量级），是轮次预算大头，
// 默认档仅 40 轮控时长。
func TestSoakGetLocalChanges(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	// 基线提交
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "base\n")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "base")
	// 留一处未提交变更：每轮 git status 应稳定报告，保证长跑中结果语义不漂移
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "modified\n")

	svc := NewGitService()
	// 预热（排除 gitCmd 等惰性初始化）
	if _, err := svc.GetLocalChanges(repo); err != nil {
		t.Fatalf("预热 GetLocalChanges: %v", err)
	}

	before, beforeGoroutines := testutil.SoakLeakBaseline()
	// fork 路径轮次折算：默认 40 = 基数 200 的 1/5，深跑档 -soak-rounds=N 时取 N/5
	rounds := soakForkRounds(soakChangesDefaultRounds, soakChangesShortRounds, 1, 5)
	interval := testutil.SoakCheckpointInterval(rounds)
	var samples []testutil.SoakSample

	for i := 1; i <= rounds; i++ {
		changes, err := svc.GetLocalChanges(repo)
		if err != nil {
			t.Fatalf("第 %d 轮 GetLocalChanges: %v", i, err)
		}
		if i%10 == 0 || i == rounds {
			if len(changes) < 1 {
				t.Fatalf("第 %d 轮本地变更数 %d，应 ≥1（a.txt 未提交修改）", i, len(changes))
			}
		}
		if i%interval == 0 {
			samples = append(samples, testutil.SoakSampleAt(i))
		}
	}
	testutil.SoakLogSamples(t, "GetLocalChanges", samples)

	testutil.SoakAssertNoLeak(t, "GetLocalChanges", before, beforeGoroutines, *leakHeapMBFlag)
}

// TestSoakCommitHistoryCacheGrowth 提交历史缓存层有界性与深拷贝不累积长跑。
// 直接操作导出缓存 API（NewCommitHistoryCache/Set/Get），复刻主包 GetCommitHistory
// 缓存命中路径的分配压力：每轮 Set 存 50 条提交深拷贝 + Get 返深拷贝，验证
// 「单键覆盖写条目有界 + 双向深拷贝垃圾可回收 + 返回值修改不污染缓存」。
// 纯内存无 fork，轮次与 GetTree 基准一致；map 键单一本身有界，重点是深拷贝不累积。
func TestSoakCommitHistoryCacheGrowth(t *testing.T) {
	cache := NewCommitHistoryCache()
	const cacheKey = "soak-repo|refs/heads/master"

	// 预构造 50 条提交快照（含 Files 切片，触发 deepCopyCommits 的切片底层数组复制）
	commits := make([]model.Commit, 50)
	for i := range commits {
		commits[i] = model.Commit{
			SHA:       fmt.Sprintf("%040d", i),
			ShortSHA:  fmt.Sprintf("%08d", i),
			Message:   fmt.Sprintf("Commit %d\n", i),
			Author:    "test",
			Email:     "test@test.com",
			Timestamp: 1700000000 + int64(i),
			DateTime:  "2026-01-01 00:00:00",
			Files:     []string{fmt.Sprintf("src/file%d.go", i), "go.mod"},
		}
	}

	// 预热：首轮 Set+Get 排除缓存 map 首建
	cache.Set(cacheKey, "headsha", commits)
	if got, sha, found := cache.Get(cacheKey); !found || sha != "headsha" || len(got) != 50 {
		t.Fatalf("预热缓存 Set/Get 异常: found=%v sha=%q len=%d", found, sha, len(got))
	}

	before, beforeGoroutines := testutil.SoakLeakBaseline()
	rounds := soakRounds(soakCacheDefaultRounds, soakCacheShortRounds)
	interval := testutil.SoakCheckpointInterval(rounds)
	var samples []testutil.SoakSample

	for i := 1; i <= rounds; i++ {
		// 同键覆盖写：entries 恒 1 条，旧值深拷贝变为垃圾可回收
		cache.Set(cacheKey, "headsha", commits)
		got, sha, found := cache.Get(cacheKey)
		if !found || sha != "headsha" || len(got) != 50 {
			t.Fatalf("第 %d 轮缓存 Set/Get 异常: found=%v sha=%q len=%d", i, found, sha, len(got))
		}
		// 抽查（每 50 轮 + 末轮）：深拷贝隔离——修改返回值不得污染缓存内部
		if i%50 == 0 || i == rounds {
			got[0].Files[0] = "polluted"
			again, _, _ := cache.Get(cacheKey)
			if again[0].Files[0] == "polluted" {
				t.Fatalf("第 %d 轮缓存被外部修改污染：Get 深拷贝隔离失效", i)
			}
		}
		if i%interval == 0 {
			samples = append(samples, testutil.SoakSampleAt(i))
		}
	}
	testutil.SoakLogSamples(t, "CommitHistoryCacheGrowth", samples)
	t.Logf("[CommitHistoryCacheGrowth] 缓存条目恒 1（单键覆盖写），每轮 50 条双向深拷贝均回收")

	testutil.SoakAssertNoLeak(t, "CommitHistoryCacheGrowth", before, beforeGoroutines, *leakHeapMBFlag)
}
