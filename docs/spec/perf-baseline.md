# 性能基线（v1.4 PR1）

> 本文档记录 WorkBench v1.4 平台加固前的性能基线数据，供 PR4 性能优化前后对比。
> 严格遵循「先测后优」：本基线只测量不改生产代码，PR4 每项优化须有 before/after 量化支撑。
> 最后更新：2026-09-14 · 来源任务：09-14-v1-4 PR1 子项 3

## 1. 适用范围

- Go 后端核心路径 benchmark 基线：文件树构建、仓库扫描、App service 装配
- 前端 bundle 体积基线：dist 总量、各 chunk 体积、代码分割候选
- Go 运行时内存占用快照：HeapAlloc / HeapSys / NumGC
- GUI 冷启动耗时：WebView2 初始化 + Go startup + 前端首屏（手动测量，留占位）
- PR4 性能优化前后对比的唯一数据依据

## 2. 测量环境

| 项 | 值 |
|---|---|
| 操作系统 | Windows 11 Pro 10.0.26200 |
| CPU | AMD Ryzen 7 H 255 w/ Radeon 780M Graphics（16 逻辑核） |
| 内存 | 31.27 GB |
| Go 工具链 | go1.26.2（go.mod 声明 go 1.24.0 最低兼容线，GOTOOLCHAIN=auto） |
| Wails | v2.12.0 |
| 采集时间 | 2026-09-14 |

> 基线数据受测量机器配置影响，PR4 对比须在同一机器复测。跨机器对比仅看相对趋势。

## 3. 维度 1：Go benchmark 基线

采集命令：`go test -bench=. -benchmem -benchtime=2s ./service/`（NewAppServices 见 3.2）

### 3.1 service 包（FileTree / ScanGitRepos）

| Benchmark | 迭代数 | ns/op | B/op | allocs/op | 说明 |
|---|---:|---:|---:|---:|---|
| `BenchmarkFileTreeGetTree_1000Files` | 1012 | 2,306,810 (~2.31 ms) | 753,147 (~735 KB) | 6,358 | 1000 文件冷扫描（每 iter ClearAllCache 强制 miss） |
| `BenchmarkFileTreeGetTree_Cached` | 10000 | 267,173 (~0.27 ms) | 130,608 (~127 KB) | 1,076 | 命中 treeCache（mtime 未变 + TTL 内），deepCopy 路径 |
| `BenchmarkScanGitRepos_10Repos` | 6544 | 358,039 (~0.36 ms) | 10,842 (~10.6 KB) | 105 | 10 嵌套仓库纯扫描（NewGitService 无 cache） |

**关键结论**：
- 文件树冷扫描 2.31 ms / 735 KB / 6358 次分配；缓存命中降至 0.27 ms（**~8.6× 提速**）/ 127 KB（~5.8× 降分配）。treeCache 收益显著，PR4 须保证不回退。
- 冷扫描 6358 allocs 偏高：每节点多次分配（NewFileTreeNode + sort.Slice 闭包 + filepath.Join 等），PR4 可考察对象池 / 预分配 slices 减分配数。
- ScanGitRepos 10 仓库 0.36 ms / 105 allocs，.git 预筛（os.Stat 不 fork git）已优化到位，瓶颈不在此。

### 3.2 主包（NewAppServices 装配）

采集命令：`go test -bench=BenchmarkNewAppServices -benchmem -benchtime=500x -run=^$ ./`

| Benchmark | 迭代数 | ns/op | B/op | allocs/op | 说明 |
|---|---:|---:|---:|---:|---|
| `BenchmarkNewAppServices` | 500 | 5,800,774 (~5.80 ms) | 140,708 (~137 KB) | 1,958 | 15 service/cache 纯构造（isDev=false） |

**关键结论**：
- NewAppServices 装配 5.80 ms / 137 KB / 1958 allocs。单次 startup 成本可接受（用户感知阈值 ~100 ms，5.8 ms 占比小）。
- 5.8 ms 中 logger 初始化（InitLogger 建 lumberjack + JSONHandler + 落盘首条日志）占可观比例，PR4 若优化启动可考察 logger 懒初始化。
- benchmark artifact：每次构造重新 InitLogger 覆盖全局 logger，lumberjack 句柄累积（生产仅 startup 一次无此问题）；用 500x 固定迭代控制句柄量，ns/op 稳定。

## 4. 维度 2：前端 bundle 体积

采集命令：`cd frontend && npm run build`（= `vite build`，wailsjs 已存在）

| 指标 | 值 |
|---|---|
| dist 总体积 | 22 MB（含字体/图片等非 JS 资源） |
| dist/assets（JS+CSS chunks） | 8.0 MB |
| JS 文件数 | 97 |
| 构建耗时 | 17.97 s |
| 构建告警 | chunks > 500 kB，建议 code-splitting |

### 4.1 Top 10 chunk（按体积降序）

| Chunk | 体积 | gzip | 性质 / 优化候选 |
|---|---:|---:|---|
| `Home-DowoqzmA.js` | 2.5 MB | 858 KB | **最大瓶颈**：Home 路由，疑似聚合 mermaid/cytoscape/katex 等图表库 |
| `index-B3PjTPh6.js` | 1.2 MB | 365 KB | 主入口 chunk |
| `chunk-FOHPRMQF-DzxwRta7.js` | 647 KB | 143 KB | 共享 chunk |
| `cytoscape.esm-Yq6u8L66.js` | 425 KB | 138 KB | 图谱库（依赖按需加载） |
| `katex-ZlcWpGUi.js` | 253 KB | 77 KB | 数学公式渲染 |
| `chunk-DU6HZSFF-CfU6704K.js` | 231 KB | 38 KB | 共享 chunk |
| `architectureDiagram-*.js` | 146 KB | 41 KB | 架构图（mermaid 子图） |
| `sequenceDiagram-*.js` | 114 KB | 31 KB | 时序图 |
| `swimlanes-*.js` | 111 KB | 38 KB | 泳道图 |
| `chunk-4HAMMTFA-*.js` | 103 KB | — | 共享 chunk |

**关键结论**：
- **Home 路由 2.5 MB 是首要优化目标**：单 chunk gzip 858 KB，首屏加载瓶颈。图表库（cytoscape 425 KB / katex 253 KB / mermaid 子图合集）应懒加载——用户进入 Home 不一定立即用图表。
- vite 已对部分库做 chunk 拆分（cytoscape / katex / 各 mermaid 图独立 chunk），但 Home 仍聚合 2.5 MB，说明 Home 组件本身或其静态 import 把图表库拉入。
- PR4 优化方向（须据基线量化）：Home 路由懒加载（`() => import('views/Home.vue')`）+ 图表组件按需 dynamic import（仅在用户触发图表功能时加载 cytoscape/katex/mermaid）。
- 主入口 index 1.2 MB（gzip 365 KB）次之，可考察路由级 code-splitting 分摊。

## 5. 维度 3：Go 运行时内存 MemStats

采集命令：`go test -run TestPerfMemStats -v ./`（定义于 `perf_bench_test.go`）

| 采样点 | HeapAlloc | HeapSys | NumGC | Δ HeapAlloc |
|---|---:|---:|---:|---:|
| 1. 基线（GC 后） | 607 KB | 7,872 KB | 1 | — |
| 2. NewAppServices 构造后 | 646 KB | 7,904 KB | 2 | +38 KB |
| 3. 1000 文件树加载后 | 646 KB | 12,032 KB | 3 | +0 KB（HeapSys +4,128 KB） |

**关键结论**：
- HeapAlloc 常驻堆仅 ~0.6 MB：app 数据结构轻量（service struct + 1000 文件树节点对象图常驻仅百 KB 级）。
- NewAppServices 构造后 HeapAlloc 仅 +38 KB：15 service/cache struct 本身常驻内存极小，符合「薄包装 + 纯构造」设计。
- 1000 文件树加载后 HeapAlloc 表观 +0 KB 但 **HeapSys 跳增 +4 MB**：运行时为一次性分配（GetTree 构造 1010 节点 + deepCopy）扩张堆，GC 后回收无引用部分，常驻 HeapAlloc 回落。HeapSys 增长是分配压力的真实信号。
- NumGC 增长缓慢（3 次 GC 覆盖构造 + 树加载）：稳态 GC 压力低。

**噪声说明**：测试进程含 Go 运行时 + testing 框架常驻堆，HeapAlloc 绝对值与 GC 时序相关，Δ 可落在噪声内（甚至 0）。HeapSys（运行时堆系统总量）更稳定。**精确的每次操作分配量以 benchmark `-benchmem` 的 B/op 为准**（见维度 1），MemStats 仅反映进程级常驻足迹。

## 6. 维度 4：GUI 冷启动（手动测量，占位）

> sub-agent 无法自动测 GUI 冷启动（须 `wails build` 产物 + 人眼/秒表观测 WebView2 到首屏）。
> 以下为测量方法与占位表，待用户手动补数。

### 6.1 测量方法

1. 生产构建：`wails build`（生成 `build/bin/workbench.exe`）
2. 启动计时：双击 exe 起秒表（或用日志时间戳——`slog.Info("workbench started")` 时间戳减进程启动时间）
3. 量三段：
   - 进程启动 → WebView2 初始化完成（窗口出现白屏）
   - WebView2 完成 → 前端首屏渲染（工作目录列表可见）
   - 前端首屏 → 首次可交互（树可点击）
4. 进程内存：启动稳态后任务管理器看 `workbench.exe` 内存（工作集）

### 6.2 基线占位表（待手动填写）

| 指标 | 基线值 | 测量方法 |
|---|---|---|
| WebView2 初始化耗时 | _待填_ | 秒表 / 日志时间戳 |
| Go startup 各阶段耗时 | _待填_ | slog 时间戳（NewAppServices 5.80 ms 见维度 1） |
| 前端首屏可交互耗时 | _待填_ | 秒表（窗口出现到树可点） |
| 进程内存（工作集） | _待填_ | 任务管理器 |

## 7. 测量方法与命令（可复现）

一键采集脚本：`bash scripts/perf-baseline.sh`（封装以下命令）

| 维度 | 命令 | 产物文件 |
|---|---|---|
| 1a service benchmark | `go test -bench=. -benchmem -benchtime=2s ./service/` | `service/perf_bench_test.go` |
| 1b 主包 benchmark | `go test -bench=BenchmarkNewAppServices -benchmem -benchtime=500x -run=^$ ./` | `perf_bench_test.go` |
| 3 MemStats | `go test -run TestPerfMemStats -v ./` | `perf_bench_test.go` |
| 2 前端 bundle | `cd frontend && npm run build` 后 `du -sh dist dist/assets` | `frontend/dist/` |

**fixture 复用**：benchmark 用 `util/testutil`（RunGit / WriteFile）构造 fixture，不重复造轮子。testutil 函数参数为 `testing.TB` 接口，benchmark（`*testing.B`）与测试（`*testing.T`）共用。

**基准不变量**：
- benchmark 文件为 `_test.go`，仅测试二进制编译，**不纳入覆盖率门禁**（`coverage-check.sh` 跑默认 `go test` 不含 `-bench`，见 `test-coverage-gate.md`）
- benchmark 稳定性遵循 `test-stability.md`：fixture 构造用 `b.StopTimer/b.StartTimer` 排除，禁 `time.Sleep`；缓存命中 benchmark 依赖 mtime 未变（fixture 构造后不再改文件）

## 8. PR4 优化对比模板

PR4 每项优化须按下表填 before（本基线）/ after（优化后复测），量化收益：

| 优化项 | 指标 | before（本基线） | after | 变化 | 数据支撑 |
|---|---|---|---|---|---|
| _示例：Home 路由懒加载_ | Home chunk 体积 | 2.5 MB / gzip 858 KB | _待测_ | _待算_ | 维度 4.1 |
| _示例：文件树对象池_ | 冷扫描 allocs/op | 6,358 | _待测_ | _待算_ | 维度 3.1 |
| _优化项 2_ | | | | | |
| _优化项 3_ | | | | | |

**约束**：每项优化须有本基线 before 数据支撑；优化后重跑对应维度命令复测；禁止无 before/after 量化的盲目优化（PRD PR4 验收硬性要求）。

## 9. 约束与噪声说明

- **不改生产代码**：本基线仅新增 `service/perf_bench_test.go` + `perf_bench_test.go` + `scripts/perf-baseline.sh` + 本文档；唯一非 _test.go 改动是 `util/testutil/testutil.go` 参数类型由 `*testing.T` 宽化为 `testing.TB`（test-only 包，不进生产二进制，向后兼容，使 benchmark 可复用 fixture 构造函数）
- **MemStats 噪声**：进程级 HeapAlloc 受 GC 时序影响，Δ 可能落在噪声内；精确分配看 benchmark B/op
- **NewAppServices benchmark artifact**：每次构造重置全局 logger 致 lumberjack 句柄累积，用 500x 固定迭代控制；生产仅 startup 一次无此问题，ns/op 趋势有效
- **GUI 冷启动占位**：sub-agent 无法自动测，待用户手动补数（见维度 6）
- **跨机器不可比**：基线受机器配置影响，PR4 复测须同机器

## 10. 相关文档

- [test-coverage-gate.md](test-coverage-gate.md) — benchmark 不纳入覆盖率门禁（_test.go 仅测试二进制编译，coverage-check.sh 不跑 -bench）
- [test-stability.md](test-stability.md) — benchmark 稳定性约定（fixture 构造排除耗时、禁 sleep、缓存命中依赖 mtime 未变）
- [app-services-assembly.md](app-services-assembly.md) — NewAppServices 装配契约（纯构造、benchmark 测的就是它）
