# Research: Go service 层 soak/长稳测试与内存泄漏断言业界模式

- **Query**: Go 长跑（soak/stability）测试结构、heap 泄漏断言、采样趋势输出、fork git 子进程循环注意事项、本项目档位/阈值建议
- **Scope**: mixed（内部仓库勘察 + 业界模式整理）
- **Date**: 2026-09-25
- **说明**: 本会话无 web 搜索工具可用，外部部分基于长期稳定的官方文档语义（pkg.go.dev）与知名开源项目公开惯例整理，引用为 canonical 稳定路径，未做实时链接验证。

---

## 1. soak 测试常见结构：轮次驱动 vs 时间盒驱动

### 1.1 两种驱动模式对比

| 维度 | 轮次驱动（`for i := 0; i < N; i++`） | 时间盒驱动（`for time.Now().Before(deadline)`） |
|---|---|---|
| 工作负载确定性 | **固定**：每轮操作数恒定，堆分配量可复现 | **不定**：机器快慢决定实际轮数，堆增量逐次漂移 |
| 泄漏断言适配 | **好**：固定工作 = 固定分配，Δ heap 可比 | 差：Δ heap 与实际执行轮数强耦合，断言噪声大 |
| 墙钟时长 | 随机器速度浮动（慢机 CI 可能超时） | **固定**：可精确控制长跑时长 |
| 真实性 | 偏合成负载 | 更接近「持续运行 N 小时」的 soak 语义 |
| CI 友好度 | 好（轮次数即预算，可静态估算） | 中（需预判单轮耗时才能估总量） |
| 典型用途 | **泄漏检测、回归断言** | 纯 soak 冒烟、压测类长跑 |

**业界结论**：泄漏断言类测试一律轮次驱动（确定性是断言可比性的前提）；时间盒驱动仅用于「跑满固定时长看挂不挂」的冒烟型 soak，且通常不断言内存增量只断言不出 panic。两者可并存为两个测试函数，共享循环体。

**对本项目的含义**：PRD 要求「泄漏断言 + 趋势量化」，全部采用**轮次驱动**。禁 `time.Sleep` 约束下时间盒模式本就需要 sleep 轮转，天然排除。

### 1.2 `-short` 与自定义 flag 分档惯例

业界通行三分法：

| 档位 | 触发方式 | 典型用途 | 业界先例 |
|---|---|---|---|
| 跳过档 | `testing.Short()` 为 true 时 `t.Skip` | 单元测试套件剔除集成/长跑（moby、k8s 惯例） | `go test -short` 剔除依赖 daemon 的集成测试 |
| 缩减档 | `testing.Short()` 为 true 时**减轮次而非跳过** | 保留冒烟覆盖，控制增量时长 | 各类带内存断言的库测试 |
| 深跑档 | 自定义 flag（如 `-soak-rounds=N`） | 本机手动/夜间任务深跑 | etcd 集成测试自定义 flag；性能回归流水线 |

**注意**：本仓库 CI（`.github/workflows/ci.yml:110`）跑的是裸 `go test ./...`，**不带 `-short`**。即 soak 测试默认（无 flag）轮次必须直接满足「增量 <30s」验收标准，`-short` 只能再往下缩，不能当作默认保护。这决定档位写法：

```go
func soakRounds(defaultRounds, shortRounds int) int {
    if soakRoundsFlag > 0 {   // 深跑档：显式 flag 优先
        return soakRoundsFlag
    }
    if testing.Short() {      // 缩减档
        return shortRounds
    }
    return defaultRounds      // 默认档（CI 即此档，须 <30s 增量）
}
```

### 1.3 flag 注册与 TestMain 用法

- 自定义 flag 必须在**包级变量**（init 期）注册，写在测试函数体内为时已晚（flag 已解析）：

```go
// soak_test.go（包级）
var soakRoundsFlag = flag.Int("soak-rounds", 0, "soak 深跑轮次；0=按档位取默认（默认/short 自动分档）")
var leakHeapMBFlag = flag.Int("leak-heap-mb", 8, "泄漏断言 heap 增量阈值（MB），宁松勿紧")
```

- **`TestMain` 不是自定义 flag 的必要条件**：`testing.M.Run` 在 flag 未解析时会自动调 `flag.Parse()`（Go 1.13+），包级注册的 flag 无 TestMain 也能用，`go test -soak-rounds=1000 ./service/` 直接生效。
- `TestMain` 仅在需要**前置/后置钩子**时才引入（goleak `VerifyTestMain`、全局 fixture、进程级 setup/teardown）：

```go
func TestMain(m *testing.M) {
    // 前置：全局 fixture（如需要）
    code := m.Run()
    // 后置：goroutine 泄漏检查等
    os.Exit(code)
}
```

- 每个测试包只能有一个 `TestMain`；本仓库当前**无任何 TestMain**（全仓检索仅归档计划文档出现），新增属首次，需在研究结论中给出选型建议（见 §5.3 goleak 对照）。
- flag 传参位置：`go test` 的自定义 flag 须置于包名之后（`go test ./service/ -soak-rounds=N`），置于 `go test` 与包名之间会报 `flag provided but not defined`。

---

## 2. heap 泄漏断言模式

### 2.1 双 GC + ReadMemStats 标准写法

```go
func forceGCAndSnapshot() runtime.MemStats {
    runtime.GC()  // 第一轮：标记清除 + 触发 finalizer
    runtime.GC()  // 第二轮：回收第一轮 finalizer 释放的对象、清除断代晋升残留
    var ms runtime.MemStats
    runtime.ReadMemStats(&ms)
    return ms
}

func TestSoakNoLeak(t *testing.T) {
    // 基线前先跑一次预热循环，把惰性初始化（map 扩容、sync.Pool、缓存首建）排除在断言外
    warmupRounds(t)
    before := forceGCAndSnapshot()

    runSoakRounds(t, rounds)   // 被测循环

    after := forceGCAndSnapshot()
    deltaHeap := int64(after.HeapAlloc) - int64(before.HeapAlloc)
    deltaGoro  := int(after.NumGoroutine) - int(before.NumGoroutine)
    // 断言 + 采样表输出（见 §3）
}
```

**为什么要两次 `runtime.GC()`**：
1. 第一轮 GC 可能触发 finalizer（仅调度执行，未回收 finalizer 保护的对象本身）；
2. 第二轮才能回收第一轮 finalizer 释放的引用链；
3. 带缓存的对象池（`sync.Pool`）等也需要额外一轮才稳定。这是 Go 社区泄漏断言的通行写法（stdlib 部分 `net/http` 测试、grpc-go leakcheck 均采用多轮 GC 思路）。

**预热（warmup）不可省**：首次调用会做 map 分配、惰性构造（本项目 `FileTreeCache.entries`、`ScanCacheManager` 首建都在首次调用时发生），不预热则基线偏低、Δ 天然为正。预热轮数 = 正式轮次的 10% 左右即可。

### 2.2 MemStats 字段选型

| 字段 | 语义 | 断言适配 | 说明 |
|---|---|---|---|
| `HeapAlloc` | 当前存活堆对象字节数（GC 后即存活堆） | **断言首选** | 双 GC 后读取即「真实存活」，泄漏直接体现于此 |
| `HeapSys` | 运行时向 OS 索取的堆总量 | **禁作断言** | 只增不减（scavenger 异步还页），受分配峰值影响大，仅趋势记录 |
| `HeapObjects` | 存活对象**个数** | 辅助信号 | 对「海量小对象泄漏」（如每轮泄漏一个 struct）比字节数更早灵敏，可作第二断言 |
| `HeapInuse` | 正在使用的 span 字节 | 不作断言 | 含碎片，介于 Alloc 与 Sys 之间 |
| `HeapReleased` | 已还给 OS 的字节数 | 仅诊断 | 平台相关（Windows 还页节奏与 Linux MADV 不同），不作跨平台断言 |
| `NumGC` | 累计 GC 次数 | 仅记录 | 反映 GC 压力，无泄漏语义 |
| `NumGoroutine` | 当前 goroutine 数 | **goroutine 泄漏断言首选** | 对比 Δ 而非绝对值（testing 框架自身常驻若干 goroutine） |

本仓库 `docs/spec/perf-baseline.md:118-122` 已有同口径结论：HeapAlloc 受 GC 时序影响、HeapSys 更稳、精确分配看 `-benchmem` B/op。**注意语境差异**：perf-baseline 讲的是「不做双 GC 的进程级快照」，彼时 HeapSys 更稳；本任务做了双 GC + 预热，HeapAlloc 已收敛为存活堆，**反而是断言首选**，两者不矛盾。

### 2.3 阈值定法：绝对 MB vs 相对基线百分比

| 定法 | 写法 | 优点 | 缺点 |
|---|---|---|---|
| 绝对 MB（推荐） | `deltaHeap < 8 MB` | 直观、跨机器稳定（fixture 固定时分配量恒定） | 需随 fixture 规模调整 |
| 相对百分比 | `deltaHeap < before.HeapAlloc * 0.5` | 随 fixture 自适应 | **基线极小时失真**（基线 1MB 时 50% 仅 512KB，噪声即可击穿）；PRD 明确「宁松勿紧」，百分比在小基线下反而过紧 |

**业界倾向**：固定 fixture 的长跑断言用绝对阈值 + flag 可配置（防 CI 机器差异噪声）；相对阈值仅在 fixture 规模浮动时使用。本任务 fixture 固定（testutil 构造的小仓 + 1000 文件树），**绝对 MB 阈值 + flag 覆盖**为最优。

### 2.4 已知噪声源清单

| 噪声源 | 机理 | 规避手段 |
|---|---|---|
| GC 不还内存给 OS | scavenge 异步，`HeapSys` 只涨不跌 | 断言只用 `HeapAlloc`（§2.2） |
| testing 框架常驻堆 | 测试二进制自身、`-race` 放大数倍 | 阈值按非 race 校准；`-race` 下放宽或跳过内存断言（goroutine 断言保留） |
| 并行测试干扰 | `t.Parallel` 的其他测试同时分配堆、起 goroutine | **泄漏断言测试禁 `t.Parallel()`**；如需绝对干净，单独跑 `-run TestSoakNoLeak` |
| 基线前惰性初始化 | map 首建、sync.Pool 首用、缓存首写 | 预热轮次排除（§2.1） |
| 后台 goroutine 滞后分配 | 快照后仍有 goroutine 在写堆（logger、janitor） | 本轮四条读路径无后台 goroutine，可忽略；若扩到 TerminalService/EventSink 需先静默再快照 |
| mtime/TTL 采样撞车 | `FileTreeCache` TTL 5min 内不会触发，循环内不存在 | 无需处理（TTL 判定是纯时间比较，不分配） |
| finalizer 残留 | 单轮 GC 不回收 finalizer 对象 | 双 GC（§2.1） |
| NumGoroutine 绝对值噪声 | testing 框架、信号处理常驻 goroutine 随测试顺序浮动 | 断言 Δ 且允许小常数（≤2），不断言绝对值 |

---

## 3. 采样与趋势输出

### 3.1 采样间隔：按轮，不按时间

- **按轮采样**（每 `rounds/10` 轮记录一次）：零等待、确定性、与禁 `time.Sleep` 约束完全兼容——采样动作本身就是循环体间隙的一次 `ReadMemStats`，无任何 sleep。
- **按时间采样**必须 sleep 轮转，被本仓库 test-stability 约束排除（NTFS mtime 粗分辨率等偶发挂问题同源）。
- 采样点数控制在 **10~20 个**：够画趋势、不刷爆 `t.Logf`（`go test -v` 可见；CI 日志建议同时输出首尾对比与结论行，非 `-v` 时失败信息也能看到）。

### 3.2 趋势结论输出到测试日志（t.Logf 表格）

```go
// samples 每个检查点一行；输出 markdown 表 + 趋势判定
t.Logf("| 轮次 | NumGoroutine | HeapAlloc KB | HeapSys KB | NumGC |")
t.Logf("|---|---|---|---|---|")
for _, s := range samples {
    t.Logf("| %d | %d | %d | %d | %d |", s.round, s.goroutines, s.heapAlloc/1024, s.heapSys/1024, s.numGC)
}
// 趋势判定：后 20% 检查点 HeapAlloc 均值 vs 前 20% 均值
first := avgHeap(samples[:len(samples)/5])
last  := avgHeap(samples[len(samples)*4/5:])
t.Logf("趋势: 前20%%均值=%.0fKB 后20%%均值=%.0fKB 增长=%.0fKB (%.1f KB/千轮)", first, last, last-first, float64(last-first)*1000/float64(rounds))
```

**分级处理（宁松勿紧的落地方式）**：
1. **断言阈值**（fail）：只抓「绝对增量超宽阈值」（如 8 MB），抓灾难级泄漏；
2. **趋势输出**（仅 Logf）：每千轮增长量、前后 20% 对比，异常趋势肉眼可见但不 fail；
3. 数据沉淀：手动深跑（`-soak-rounds` 大值）时把表格抄入 `docs/spec/perf-baseline.md` 新章节，作为量化依据（PRD 验收项）。

不建议首轮就把「线性增长」做成 fail 条件——GOGC 动态调整、碎片化都可能造出伪线性段，先跑几轮人工确认斜率真实后再收紧。

---

## 4. 子进程型路径（fork git）在循环测试中的注意事项

### 4.1 本项目四条路径的 fork 面核实（内部勘察结论）

| 路径 | 是否 fork git 子进程 | 实现位置 | 超时机制 |
|---|---|---|---|
| `FileTreeService.GetTree` | 否（纯 `os.ReadDir` + `os.Stat` 判 `.git`） | `service/filetree.go`、`service/filetree_cache.go` | 不适用 |
| `GitService.ScanGitRepos` | 否（递归目录扫描；remote 判定走 go-git `PlainOpen` 库调用） | `service/git.go:300`、`git.go:449` | 不适用 |
| `GitService.GetLocalChanges` | **是**（`git status --porcelain -z`） | `service/git.go:474` → `util.GitCommand.Execute` | `exec.CommandContext` 30s（`util/git.go:22`），`cmd.Run()` 内部含 `Wait` |
| `App.GetCommitHistory` | **是**（`git log` 流式） | 主包 `commit_history_cli.go:300-324` `streamCommitLog` | `exec.CommandContext` 30s（`commitLogTimeout`，line 58），`finish()` 先 `cancel()` 再 `Wait()` |

即：**fork 风险集中在 GetLocalChanges 与 GetCommitHistory 两条路径**；GetTree/ScanGitRepos 是纯内存/文件系统 + go-git 库路径，循环里无进程开销，轮次可以放大一个数量级。

### 4.2 循环测试中的子进程要点

| 事项 | 说明 | 本项目现状 |
|---|---|---|
| **句柄泄漏（Windows）** | `os/exec` 文档明确：`cmd.Wait` 未被调用则进程句柄泄漏（Windows 表现为句柄数持续增长）；`Output()`/`CombinedOutput()`/`Run()` 内部均已调 Wait | 生产路径已闭环（`cmd.Run`、流式 `finish()`）；测试侧 `testutil.RunGit` 用 `CombinedOutput`（内部 Wait），fixture 构造安全 |
| **僵尸进程（Unix）** | 子进程退出后未 Wait 成僵尸；Linux CI 下若泄漏会撑爆进程表 | 同上，只要走生产执行器即闭环；不要在测试里自造 `exec.Command().Start()` 不 Wait 的路径 |
| **`exec.CommandContext` 超时语义** | ctx 到期只 kill 直接子进程（git 基本不再 fork 子进程，hooks 除外），够用；Go 1.20+ 可配 `cmd.Cancel`/`cmd.WaitDelay` 处理管道滞留 | 生产 30s 超时直接复用即可；长跑循环正常路径每轮耗时远低于超时，超时分支本就是被测容错逻辑的一部分 |
| **管道死锁** | 子进程写满 stdout 管道而父进程不读 → 子进程阻塞 | `GetLocalChanges` 走 `cmd.Run`（内存 buffer，无死锁面）；`streamCommitLog` 逐 chunk 读管道，安全。测试侧只要不绕开这两个入口就无风险 |
| **每轮 fork 量级控制** | Windows 进程创建 + git 启动约 20~60ms/次（小仓 `git status` 实测量级），是循环预算大头；**串行循环天然限流**（上一轮 Wait 返回才进下一轮），无并发 fork 爆量风险 | 轮次预算见 §5；不要在循环里对同一仓库开并发 fork（并发路径已有 `git_concurrency_test.go` 单测覆盖，不属 soak 范畴） |
| **控制台窗口闪烁（Windows）** | 每次 fork 若不带 CREATE_NO_WINDOW 会闪黑框 | 生产执行器已统一 `util.HideCommandWindow`，循环测试自动继承 |
| **TempDir 清理与文件锁** | `t.TempDir` 在测试结束时删除目录，若 git 进程未退完 Windows 下删目录报错（文件被占用） | 只要每轮走 Wait 闭环即无残留进程；这是「不要自造 Start 不 Wait」的另一个理由 |
| **循环内的 fork 计数** | 每轮 fork 次数恒定 → 总 fork 数 = 轮次 × 每轮次数，确定性指标 | 采样表建议附「总 fork 数」列或写进结论行，perf-baseline 数据引用时口径完整 |

---

## 5. 针对本项目的具体建议

### 5.1 档位与轮次建议（默认档预算 <30s 为硬约束）

单轮成本参照 perf-baseline 实测与 git 进程启动量级估算：

| 路径 | 单轮量级 | Short 档轮次 | 默认档轮次（CI/本地 `go test ./...`） | Long 档轮次（`-soak-rounds`） |
|---|---|---|---|---|
| `GetTree`（命中+失效混合） | 命中 ~0.3ms / 失效 ~2.3ms（1000 文件 fixture） | 50 | 200 | 2000 |
| `ScanGitRepos`（10 仓 fixture） | ~0.4ms（纯库路径无 fork） | 50 | 200 | 2000 |
| `GetLocalChanges`（**fork**） | ~30-60ms/次（Windows git 启动） | 10 | 40 | 400 |
| `GetCommitHistory`（**fork**） | ~50-100ms/次（git log + 解析） | 10 | 30 | 300 |
| **默认档总增量估算** | | **<1s** | **约 6-10s**（留余量满足 <30s） | 约 60-120s（手动深跑） |

- Long 档用**轮次乘数语义**而非 deadline：`-soak-rounds=2000` 即四个路径各按上表 Long 档比例放大；不建议再加 `-soak-duration` 时间盒变体（§1.1 已论证确定性优先，两个入口徒增维护面）。
- 失效形态混合比例建议 **命中：失效 = 4:1**（每 5 轮 1 次 Invalidate/Clear 后强制重拉），贴近真实「用户高频翻树 + 偶发手动刷新」负载，且保证缓存 map 的 set/delete 两路径都被循环覆盖。
- `treeCache` 陈旧缓存注入（mtime 驱动失效）按 test-stability 方案 C 在 service 包内测试文件完成，与 PRD 既定一致，不依赖 sleep/TTL。

### 5.2 采样与泄漏断言阈值建议（宁松勿紧的具体数字）

| 参数 | 建议值 | 说明 |
|---|---|---|
| 采样间隔 | 每 `rounds/10` 轮一个检查点（每路径 10 个点） | 按轮采样，无 sleep（§3.1） |
| HeapAlloc 增量阈值（service 包） | **默认 8 MB**，flag `-leak-heap-mb` 可调 | fixture 小（百 KB 级常驻），8MB 已是噪声数十倍，抓灾难级泄漏 |
| HeapAlloc 增量阈值（主包） | **默认 16 MB**，同 flag | `CommitHistoryCache` 上限 5000 条 Commit 深拷贝瞬时压力大，口径更宽 |
| goroutine 增量阈值 | **≤2**（绝对差，双 GC 快照间对比） | 允许 testing 框架小常数浮动；>2 视为泄漏 |
| HeapObjects | 记录不断言（首轮） | 数据齐后再评估是否加第二断言 |
| HeapSys / NumGC | 仅采样表记录，不断言 | §2.2 |
| 趋势判定 | 仅 `t.Logf` 输出「前/后 20% 均值 + KB/千轮」，不 fail | §3.2 分级；斜率数据稳定后再考虑收紧 |
| `-race` 下 | 跳过 heap 数值断言（goroutine 断言保留） | race 检测器使内存放大数倍，数值断言无意义 |
| 并行 | 泄漏断言测试禁 `t.Parallel()` | §2.4 |
| 预热 | 正式轮次的 10%，排除惰性初始化 | §2.1 |

### 5.3 goleak 选型对照（PRD 开放问题 b）

| 维度 | 手写 `NumGoroutine` Δ 对比 | uber-go/goleak |
|---|---|---|
| 依赖成本 | 零依赖 | 引入 go.uber.org/goleak 单包（依赖极轻，无重传递依赖）；改动 go.mod 需重跑 govulncheck（CLAUDE.md 安全扫描规则） |
| 接入成本 | 零（断言逻辑在 soak 测试内） | 需每包引入 `TestMain`（本仓库首次出现 TestMain）；或每测试 `defer goleak.VerifyNone(t)` |
| 检测能力 | 数量级趋势（泄漏 goroutine 个数），**无法定位泄漏点** | 栈 diff 精确定位泄漏 goroutine 的创建栈；`IgnoreTopFunction` 排除已知后台 goroutine |
| 误报风险 | 低（Δ + 小常数容差） | 中：对后台常驻 goroutine 的服务（TerminalService pty、EventSink）需逐个 ignore，维护面随服务增加 |
| 与本轮目标匹配度 | **高**——PRD 目标是「趋势级明显泄漏，宁松勿紧」 | 超出目标（精确定位属调试工具属性）；四条读路径无后台 goroutine 时收益有限 |
| 建议 | **本轮采用手写方案** | 列为可选后续：若手写 Δ 检出过泄漏需要定位、或扩展到终端/事件订阅路径时再引入 |

### 5.4 落地形态小结（供 PRD Requirements 回填参考）

- 文件：`service/soak_test.go`（GetTree/ScanGitRepos/GetLocalChanges + treeCache 陈旧注入）+ 主包 `soak_test.go`（GetCommitHistory 全链路，含缓存命中/增量/ClearAll 失效三形态）。
- flag：`-soak-rounds`（深跑轮次，默认 0 走档位）、`-leak-heap-mb`（阈值），包级注册即可，无需 TestMain（goleak 不引入的前提下）。
- 断言：双 GC HeapAlloc Δ < 阈值 + goroutine Δ ≤ 2；其余全为 Logf 趋势输出。
- 数据沉淀：Long 档手动跑一次，采样表抄入 `docs/spec/perf-baseline.md` 新增「长跑稳定性」「泄漏检测」两节。

---

## 外部参考

> 本会话无 web 搜索工具，以下为长期稳定的 canonical 引用，语义以官方文档为准。

- [testing 包文档](https://pkg.go.dev/testing) — `Short`、`TestMain`、`M.Run` 自动 `flag.Parse` 语义、`-short`/自定义 testflag 传参位置
- [runtime#MemStats](https://pkg.go.dev/runtime#MemStats) — HeapAlloc/HeapSys/HeapObjects/HeapReleased 字段精确语义、scavenge 异步还页说明
- [os/exec 包文档](https://pkg.go.dev/os/exec) — `Wait` 未调用的句柄泄漏警告、`CommandContext` 超时 kill 语义、Go 1.20+ `Cancel`/`WaitDelay`
- [uber-go/goleak](https://github.com/uber-go/goleak) — `VerifyTestMain`/`VerifyNone`/`IgnoreTopFunction` 用法
- grpc-go `test/leakcheck` — TestMain 级 goroutine 栈 diff 的成熟先例
- `docs/spec/perf-baseline.md` §5/§10 — 本仓库 MemStats 噪声既有结论（HeapAlloc GC 时序敏感、HeapSys 稳、精确分配看 B/op）
- `docs/spec/test-stability.md` — 禁 sleep/固定端口/mtime 注入陈旧缓存既有约束

## Caveats / Not Found

- 本会话无 web 工具，未能实时核对 goleak 最新版本的 go.mod 传递依赖明细（表述为「依赖极轻」而非断言零依赖；采纳引入方案前建议本地 `go mod tidy` 后跑 govulncheck 确认）。
- 单轮 fork 耗时（20-60ms）为 Windows git 进程启动的通用量级估计，未在本机实测；首轮 soak 跑完后以采样表实测值修正 Long 档轮次预算。
- `GetTree` 内 `isGitRepoDir`/`hasRemote` 的 go-git 调用面（`gitRepoCache`/`gitRemoteCache`）为纯库操作、不 fork 子进程，基于 perf_bench_test.go 注释与 git.go 源码核实；如后续 ScanGitRepos 改为 CLI 实现则 §4 结论需重估。
