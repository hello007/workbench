# Research: Go 测试 goroutine 泄漏检测方案选型（uber-go/goleak 引入 vs 手写对比）

- **Query**: service 层长跑稳定性测试的 goroutine 泄漏检测，goleak 引入与手写 NumGoroutine/栈扫描对比，含依赖成本与误报风险评估
- **Scope**: mixed（内部现状调查 + 外部权威源核实）
- **Date**: 2026-09-25

---

## 摘要

**结论前置：建议引入 `go.uber.org/goleak v1.3.0`，以 `VerifyTestMain` 包级门禁方式挂在 service 层长跑（soak）测试包上，配 `testing.Short()` 短路；手写 `NumGoroutine` 前后对比仅作为用例内辅助观察手段，不作为 CI 断言门禁。**

依赖成本经本仓库临时副本实测：go.mod 仅新增 1 行、go.sum 仅新增 2 条 hash（goleak 自身的 go.mod hash 与 zip hash），无任何新增传递依赖（testify 被模块图剪枝排除），与本项目「依赖少而克制」原则相容。

---

## 1. 背景与约束

| 项 | 内容 |
|---|---|
| 目标 | service 层长跑循环 + 缓存路径的 goroutine 泄漏检测（循环前后 goroutine 数对比） |
| 测试增量 | `-short` 模式总时长 < 30s |
| 阈值原则 | 宁松勿紧；CI 偶发失败比漏检危害大 |
| 覆盖率门禁 | 测试文件不计入覆盖率门禁（不构成约束） |
| 现有依赖 | 无任何 uber 系依赖（go-git/wails/websocket/lumberjack 等） |

### 1.1 本仓库现状（内部调查）

| 事实 | 数据 / 位置 |
|---|---|
| Go 版本 | go 1.26.0 / toolchain go1.26.6（`go.mod` 第 3-5 行） |
| 现有 TestMain | **全仓库 0 个**（`**/*_test.go` grep `func TestMain` 无命中） |
| service 层规模 | 92 个 .go 文件，其中 50 个 `_test.go` |
| service 层常驻 goroutine 源（非测试文件 `go func`/`go xxx`） | `service/ai_function.go`(3)、`service/terminal.go`(2)、`service/update.go`(2)、`service/git.go`(2)、`service/chat_service.go`(1)、`service/commit_history_cache.go`(1)、`service/content_search.go`(1)、`service/dashboard.go`(1)、`service/git_difftool.go`(1)、`service/git_mergetool.go`(1) |
| 现有泄漏检测 | 无（`NumGoroutine`/`goleak`/`runtime.Stack` 全仓库无使用） |
| websocket Hub | 位于主包 `web_serve.go`，不在 service 层（service 层 soak 测试不涉及） |

含义：service 层泄漏检测的真实暴露面是 pty 读泵（terminal）、CLI 流读取（ai_function/chat）、缓存后台（commit_history_cache）、轮询（update），而非 websocket。

---

## 2. uber-go/goleak 调查（全部经权威源核实）

### 2.1 维护状态与版本

| 项 | 结论 | 来源 |
|---|---|---|
| 最新发布版本 | **v1.3.0**（tag 列表首项；其上一个为 v1.2.1） | GitHub API `/repos/uber-go/goleak/tags` |
| 维护活性 | 活跃未死：最近 push 2026-09-15（10 天前），5286 stars，MIT，未归档 | GitHub API `/repos/uber-go/goleak` |
| main 分支新 API | `IgnoreCreatedBy`（按创建者栈帧忽略）已在 main 分支但**未发版**——v1.3.0 中 `grep -c IgnoreCreatedBy = 0`，采用 v1.3.0 时不可引用该 API | v1.3.0 tag `options.go` 实查 |
| Go 版本兼容 | goleak 声明 `go 1.20`；README 明示支持最近两个 Go minor。Go 1.26 在范围内，纯 runtime 栈解析无平台限制 | v1.3.0 go.mod + README |
| 节奏提示 | 发版节奏低（v1.3.0 于 2023-12 发布后 main 持续修但未打 tag），属「低频维护但未弃坑」 | CHANGELOG.md |

### 2.2 传递依赖体积（本仓库实测）

方法：将本仓库源码（排除 .git/node_modules）复制到系统临时目录，`go get go.uber.org/goleak@v1.3.0` + 建 probe 测试 import 后 `go mod tidy` 达稳态，diff go.mod/go.sum（实验目录已清理，未触碰仓库文件）。

| 项 | 实测增量 |
|---|---|
| go.mod | +1 行：`go.uber.org/goleak v1.3.0` |
| go.sum | **+2 条**（`go.uber.org/goleak` 的 go.mod hash + zip hash） |
| testify 链 | **未进入本仓库依赖**。v1.3.0 的 go.mod 虽 require `stretchr/testify v1.8.0`，但 goleak 主代码（leaks.go/options.go/testmain.go 实查 0 处 import）不使用它（仅 goleak 自身 `_test.go` 用），且 goleak 声明 go 1.20 ≥ 1.17 触发模块图剪枝，tidy 后 go.sum 无 testify/go-spew/go-difflib/yaml 任何新条目 |

结论：常见担忧「引入 goleak 拉进 testify 全家桶」在本仓库（Go 1.17+ 主模块）不成立，依赖成本近乎为零。

### 2.3 VerifyTestMain 用法

```go
func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m) // 替代 os.Exit(m.Run())；内部完成 m.Run() 并在退出前检测
}
```

- 语义：包级一次性检测——全部测试跑完后对 goroutine 集合做快照 diff，发现「测试期间新出现且未退出」的 goroutine 则打印泄漏栈并以非零码退出。
- 内置重试退避：发现疑似泄漏时毫秒级退避多次重试，容忍异步 goroutine 慢退出（源码 `opts.retry`）。
- 包级模式的附带收益：定位泄漏用例需二分（README 提供逐用例运行脚本），这是代价；但换来比逐用例 `VerifyNone` 低一个量级的噪声。

### 2.4 内置良性 goroutine 过滤（v1.3.0 实查）

| 过滤类别 | 覆盖的栈 |
|---|---|
| testing 框架 | `testing.RunTests`、`testing.runFuzzing`、`testing.runFuzzTests` |
| runtime | `runtime.goexit` |
| syscall / signal | `syscall`、`os/signal.signal_recv`、`os/signal.loop`、`runtime.ensureSigM` |
| 其他内置分类 | trace、DNS resolver 等栈分类过滤（源码 `isTraceStack`/`isDNSResolverStack` 等） |

这正是手写方案需要自己维护并随 Go 版本漂移跟进的清单；v1.3.0 还修复过「按文件名误匹配函数名」的过滤 bug（CHANGELOG #112）。

### 2.5 公开 API 清单（v1.3.0 实查）

| API | 用途 |
|---|---|
| `VerifyNone(t)` | 单测结束点检测（`defer` 使用） |
| `VerifyTestMain(m)` | 包级检测（推荐形态） |
| `Find(options...)` | 返回泄漏描述而不直接失败（自定断言语义时用） |
| `IgnoreTopFunction(f)` | 忽略栈顶为指定函数的 goroutine（噪声治理主手段） |
| `IgnoreAnyFunction(f)` | 忽略栈中任意帧出现该函数的 goroutine（v1.3.0 新增 #113） |
| `IgnoreCurrent()` | 以创建时刻的现有 goroutine 集合为基线，只关注新增（大型项目增量引入的关键选项） |
| `Cleanup(func(exitCode int))` | 失败退出前注册清理回调 |
| `RunOnFailure()` | 测试失败时也执行泄漏定位输出 |

### 2.6 已知局限

| 局限 | 说明 | 缓解 |
|---|---|---|
| `t.Parallel` 不可辨 | README 明示：并行子测试未结束时其 goroutine 会被误判泄漏 | 本项目 soak 场景无并行子测试需求；如用则改包级 VerifyTestMain |
| 无用例级归属 | 包级模式只报「泄漏了什么栈」，不报「哪个用例泄漏」 | README 的逐用例二分脚本；或临时对该用例 `defer VerifyNone(t)` |
| 无 `-short` 感知 | goleak 不读 testing 标志 | `testing.Short()` 是包级导出函数，可在 TestMain 内直接判断（见 §5 骨架） |
| 只认「结束快照 diff」 | 不支持循环迭代间的过程性计数告警 | 过程性观察另用 `NumGoroutine`（非门禁） |

### 2.7 与 `testing.Short` 的配合

本项目约束「`-short` 模式 <30s」的映射方式：soak 长跑测试本身整体挂 `testing.Short()` 跳过，泄漏检测附着其上自然不执行。goleak 检测本身开销为毫秒级（一次全量栈抓取 + 解析 + 若干次重试），不是时长瓶颈；时长瓶颈是循环本体。TestMain 中短路写法见 §5。

---

## 3. 手写方案调查（业界常见写法）

### 3.1 三档手写实现

| 档位 | 做法 | 优点 | 盲区 |
|---|---|---|---|
| A. 计数对比 | `runtime.NumGoroutine()` 循环前后各取一次，比较差值（可加容差） | 零依赖约 5 行；可做迭代间过程性趋势观察 | ① 数值相同但 goroutine 身份置换的泄漏被掩盖（前一个退出、泄漏的照样驻留则恰好抵消）；② GC/调度波动需宽容差；③ 无栈信息，无法定位 |
| B. 全量栈扫描 + 过滤 | `runtime.Stack(buf, true)` 取全量栈，按 `\n\n` 分割，提取每栈首行函数名，与「良性清单」比对，其余视为泄漏 | 有泄漏栈可定位；能识别身份置换；约 100 行 | 需自行维护良性栈清单（testing/runtime/syscall/signal/DNS……），且随 Go 版本演进而漂移 |
| C. 轻量现成库 | 如 `github.com/fortytw2/leaktest`（B 档的封装，`defer leaktest.Check(t)()` / `CheckTimeout`） | 省去自维护清单 | 仍是第三方依赖（与引入 goleak 的成本同级，但社区规模/维护弱于 goleak） |

说明：任务输入中提到的「ggallow」一名未能核实（无对应知名 Go 泄漏检测库），本报告不予采信；如指某具体库请提供链接。

### 3.2 典型误报源与缓冲手段（手写与 goleak 通用）

| 误报源 | 机制 | 缓冲 |
|---|---|---|
| 异步 goroutine 慢退出 | 生产者/worker 收到关闭信号后仍在退出路径上 | 检测前重试退避（goleak 内置；手写需自实现） |
| testing/runtime 自身栈 | Go 测试框架、fuzz、signal loop、trace goroutine | 良性清单过滤（goleak 内置且随版本修） |
| DNS 解析后台 | `net` 包 cgo/解析器 goroutine | goleak 内置 `isDNSResolverStack` |
| 库遗留读泵 | 本项目潜在面：pty 读泵（terminal）、CLI stdout 流（ai_function/chat）、lumberjack（无常驻 goroutine，仅写锁） | 显式 Close/Wait 收口 + `IgnoreTopFunction` 兜底 |

### 3.3 Go 官方血统的 B 档参考实现

`cockroachdb/cockroach` 的 `pkg/util/leaktest/leaktest.go`（文件头 Copyright 2013 The Go Authors，即源自 Go 官方 `x/net/http2` 2013 年 leaktest 原型）：`defer leaktest.AfterTest(t)()` 用法，`allstacks.Get()` 全量栈 → `interestingGoroutines()` 过滤 testing/runtime 栈 → 按 goid 比对 → 轮询重试。`grpc-go` 的 `internal/leakcheck/leakcheck.go` 为同一血统。这条线证明「B 档手写」可行，但其维护主体是有专职基础设施团队的大项目。

---

## 4. 类似项目实践

| 项目 | 路线 | 证据（本次实查） |
|---|---|---|
| prometheus/prometheus | **goleak** | 主 go.mod 含 `go.uber.org/goleak v1.3.0`（API contents 实查） |
| pingcap/tidb | **goleak**（由自研迁移） | 主 go.mod 含 `go.uber.org/goleak v1.3.0`；原自研 `pkg/testleak/leaktest.go` 路径已 404（迁移为推断，依赖存在为实查） |
| kubernetes/kubernetes | goleak（依赖存在） | 主 go.mod 含 goleak 1 处（grep 计数） |
| cockroachdb/cockroach | **自研** `pkg/util/leaktest` | 文件存在（HTTP 200），Go 官方血统栈扫描实现（§3.3） |
| grpc/grpc-go | **自研** `internal/leakcheck` | 文件存在（HTTP 200），同血统 |
| etcd-io/etcd | 未见统一 goleak 门禁 | `tests/go.mod` grep goleak 无命中（证据有限，谨慎采信） |

噪声治理经验（业界通用，goleak 语境）：大型项目治理 goleak 噪声的惯用三件套即 **`IgnoreTopFunction` 精确函数名忽略**、**`IgnoreCurrent` 建立存量基线只管增量**、**包级 `VerifyTestMain` 替代逐用例 `VerifyNone`**；cockroach/grpc 的自研线本质是「把同一套过滤清单养在自己仓库里」。

---

## 5. 选型对比与最终建议

### 5.1 对比表

| 维度 | 引入 goleak v1.3.0 | 手写 A（NumGoroutine 对比） | 手写 B（栈扫描+过滤） |
|---|---|---|---|
| 依赖增量（本仓库实测） | go.mod +1 行、go.sum +2 条，无传递树 | 0 | 0 |
| 代码量 | 0（API 现成） | 约 5-20 行 | 约 100 行 + 良性清单长期维护 |
| 泄漏栈定位 | 有（自动打印泄漏栈 diff） | 无 | 有（自维护） |
| 身份置换泄漏（旧退新进） | 能识别（快照按栈集合 diff） | **不能**（计数恰好抵消） | 能 |
| 良性栈过滤随 Go 漂移 | 官方随版本维护（v1.3.0 修过匹配 bug） | 不涉及 | 自己跟 |
| 异步慢退出缓冲 | 内置重试退避 | 需手写 sleep 容差 | 需手写轮询 |
| CI 偶发失败风险 | 可控（包级一次 + Ignore 系选项 + 宁松起步） | 低（但漏检风险高） | 中（过滤清单不全是主要风险） |
| `-short` 配合 | TestMain 内 `testing.Short()` 短路 | 用例内直接短路 | 同左 |
| 长期负担 | 低（上游养过滤逻辑） | 低（但能力弱） | **高**（清单漂移无人跟） |
| 项目先例 | prometheus/tidb/k8s | 少作为门禁 | cockroach/grpc-go（大厂自养） |

### 5.2 最终建议

**引入 goleak v1.3.0，包级 `VerifyTestMain` 门禁；手写 NumGoroutine 仅作循环内趋势观察辅助。**

理由按权重：

1. **依赖成本实测近零**（§2.2），「无 uber 系依赖」的现状约束实际只增加 1 个 module、2 条 hash；反而比自养 100 行过滤清单（B 档）更符合依赖克制。
2. **误报风险结构可控**：本项目的 CI 偶发失败顾虑，主要靠三重缓冲——包级 VerifyTestMain（非逐用例）、soak 测试整体随 `-short` 跳过（CI 普通跑不触发）、`IgnoreTopFunction`/`IgnoreCurrent` 留作噪声兜底。泄漏检测只挂在专项 soak 包，不铺全仓，风险面与收益面一致。
3. **能力完整性**：本任务目标「循环前后 goroutine 数对比」在泄漏语义上等价于「结束后快照 diff」，goleak 的 diff 语义天然覆盖且能识破手写 A 的身份置换盲区（§5.1 第 4 行）。
4. **生态佐证**：prometheus/tidb（由自研迁移而来）/k8s 均在 go.mod 持有 goleak；自研路线（cockroach/grpc-go）是大团队养基础设施的选择，不适用于本项目人力规模。

### 5.3 落地要点（供实施参考）

```go
// service 包 soak 测试的 TestMain 骨架（全仓库当前无 TestMain，属首个）
func TestMain(m *testing.M) {
    if testing.Short() { // testing.Short() 为包级函数，TestMain 内可用
        os.Exit(m.Run()) // 短模式：跳过泄漏门禁，保 <30s 约束
    }
    goleak.VerifyTestMain(m) // 长跑模式：退出前快照 diff 检测
}
```

- 版本钉死 `go.uber.org/goleak v1.3.0`；不要追 main 分支的 `IgnoreCreatedBy`（未发版，v1.3.0 无此 API，实查确认）。
- 阈值宁松的落地：首轮先以 `goleak.Find()` 打日志观察（不失败）跑绿 soak，确认基线干净后再换 `VerifyTestMain`；出现已知良性库栈时用 `IgnoreTopFunction`（须全限定函数名）逐个登记，不放宽为全包豁免。
- 本项目预期需登记的候选忽略项（依 §1.1 暴露面预判，非结论）：pty 读泵相关（terminal）、CLI 流读取（ai_function/chat）；websocket Hub 在主包不在 service 层，不涉及。
- 依赖升级后照常跑 `govulncheck ./...`（goleak 无原生依赖面，预期零告警）。

---

## 参考来源

| 来源 | 用途 |
|---|---|
| GitHub API `/repos/uber-go/goleak`（元数据/标签，2026-09-25 实查） | 维护状态、最新版本 v1.3.0 |
| goleak v1.3.0 tag 的 `go.mod` / `options.go` / `CHANGELOG.md` / `README.md`（GitHub contents API，raw 实查） | 依赖声明、API 清单、内置过滤、已知局限 |
| 本仓库临时副本 `go get` + `go mod tidy` 实测（已清理） | go.sum 增量 +2 条、testify 被剪枝 |
| `cockroachdb/cockroach` `pkg/util/leaktest/leaktest.go`、`grpc/grpc-go` `internal/leakcheck/leakcheck.go`（存在性+头部实查） | 自研血统考证 |
| `prometheus/prometheus`、`pingcap/tidb`、`kubernetes/kubernetes` 主 go.mod 实查 | goleak 采用佐证 |

## Caveats / Not Found

- 任务输入中的「ggallow」未能核实为任何已知 Go 泄漏检测库名，未采信；如有具体指涉请提供链接。
- etcd 的结论为「未见统一 goleak 门禁」，证据仅基于 `tests/go.mod` 无命中，未做全仓库代码搜索（GitHub code search 需认证），采信度中等。
- tidb「由自研 testleak 迁移到 goleak」中，「迁移」为推断（原路径 404 + go.mod 现有 goleak），依赖存在本身为实查。
- goleak 重试退避的精确参数（次数/间隔毫秒值）未逐一记录，文档表述为「毫秒级退避多次重试」（源码 `opts.retry` 存在为实查）。
- 未在本仓库真实改动 go.mod 验证（研究 agent 边界），增量为临时目录副本实测，与真实引入路径等价。
