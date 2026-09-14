# 性能基线（v1.4 PR1）

> 本文档记录 WorkBench v1.4 平台加固前的性能基线数据，供 PR4 性能优化前后对比。
> 严格遵循「先测后优」：本基线只测量不改生产代码，PR4 每项优化须有 before/after 量化支撑。
> 最后更新：2026-09-14 · 来源任务：09-14-v1-4 PR1 子项 3 / PR4 子项 4

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

> **PR4 勘误**：PR1 基线原述「Home 聚合 mermaid/cytoscape/katex」措辞不精确。实测 vite 已将 cytoscape / katex / 各 mermaid 子图拆为独立 chunk 文件，**它们并非字面合并在 Home.js 内**，而是经 `FilePreviewRenderer.vue` 顶部 `import mermaid from 'mermaid'` 静态导入，进入 Home 路由的**静态 import 图**，随首屏一并加载。PR4 的实质收益是把这张静态图改为动态（懒加载），详见第 9 节。

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

## 8. PR4 优化对比

PR4 每项优化按下表填 before（PR1 基线）/ after（优化后复测），量化收益：

| 优化项 | 指标 | before（PR1 基线） | after（PR4 复测） | 变化 | 数据支撑 |
|---|---|---|---|---|---|
| mermaid 懒加载 | Home.js 文件体积 | 2,608.72 kB / gzip 858.87 kB | 2,513.76 kB / gzip 827.33 kB | **−94.96 kB / gzip −31.54 kB** | 维度 4.1 |
| mermaid 懒加载 | 首屏 eager 图表库 JS | ~1.8 MB raw / gzip ~550 kB 随 Home 静态加载 | 0 kB（改为按需懒加载） | **−1.8 MB / gzip ~−550 kB 首屏** | 第 9 节懒加载图 |
| 文件树冷扫描 | ns/op | 2,306,810 (~2.31 ms) | 2,092,055 (~2.09 ms) | −214,755 ns（~−0.21 ms，噪声内，未改 Go） | 维度 3.1 |
| 文件树冷扫描 | allocs/op | 6,358 | 6,358 | 0（未改 Go） | 维度 3.1 |
| 文件树缓存命中 | ns/op（treeCache 不回退校验） | 267,173 (~0.27 ms) | 281,760 (~0.28 ms) | +14,587 ns（~+0.01 ms，噪声内，**treeCache 提速保持**：before 8.6× / after 7.4×，缓存命中仍远快于冷扫描） | 维度 3.1 |
| ScanGitRepos | ns/op | 358,039 (~0.36 ms) | 337,478 (~0.34 ms) | −20,561 ns（~−0.02 ms，噪声内，未改 Go） | 维度 3.1 |
| NewAppServices | ns/op（不动项） | 5,800,774 (~5.80 ms) | _未复测（无 Go 改动）_ | — | 维度 3.2 |

**约束**：每项优化须有本基线 before 数据支撑；优化后重跑对应维度命令复测；禁止无 before/after 量化的盲目优化（PRD PR4 验收硬性要求）。

## 9. PR4 优化结果详情

### 9.1 已完成：mermaid / cytoscape / katex 懒加载（首屏瓶颈，收益最大）

**改动**：
- `frontend/src/components/FilePreviewRenderer.vue`：移除顶部静态 `import mermaid from 'mermaid'` 与模块级 `mermaid.initialize(...)`，改为 `ensureMermaid()` 异步懒加载器（模块级缓存 mermaid 实例，首次渲染含 ` ```mermaid ` 代码块的 markdown 时才 `await import('mermaid')`）。`renderMermaid()` 先查 `pre.mermaid` 节点，无则零成本返回（普通 markdown 不触发动态 import）。
- `frontend/vite.config.js`：新增 `optimizeDeps.include: ['mermaid']`，确保 wails dev 启动即预构建 mermaid，规避动态 import 首次触发时 Vite 重新发现依赖并重启 dev server（与 docx/xlsx 历史同源问题，见 FilePreviewRenderer.vue 顶部注释）。

**收益量化**：
- mermaid 经此改为动态 import 后，其整张依赖图（mermaid.core 94.61 kB + cytoscape.esm 434.85 kB + katex 258.68 kB + 各 mermaid 子图 architectureDiagram 148.89 kB / sequenceDiagram 115.74 kB / swimlanes 111.11 kB / gantt / c4 / er / gitGraph … + dagre / rough.esm / cose-bilkent / graphlib / mermaid-parser.core + 共享 mermaid chunk）从「随 Home 首屏静态加载」改为「仅 markdown mermaid 渲染时按需加载」。
- 图表库 chunk 合计 ~1.8 MB raw（gzip ~550 kB）移出首屏 eager 加载。
- Home.js 文件本身仅 −95 kB（mermaid core 抽出为独立懒加载 chunk `mermaid.core`）：因 cytoscape / katex / 各子图在 PR1 基线时已被 vite 拆为独立 chunk 文件，只是经静态 import 随首屏加载，故 Home.js 字面体积下降有限，**实质收益在首屏 eager 传输量**。

**正确性验证**：
- `npm test -- --run`：932 用例全绿（FilePreviewRenderer.spec.js 的 mermaid 用例 `vi.mock('mermaid')` 经动态 import 返回 mock，`mermaid.run` 调用断言通过）。
- `npm run build`：通过，无 MISSING_EXPORT，chunk 分割合理。
- `npm run e2e`：44 用例全绿（vite preview 生产构建，首屏渲染与各流程正常）。
- 前端覆盖率门禁：lines 83.67% / branches 73.07% / functions 76.96% / statements 81%，均 ≥70%。

### 9.2 关于「Home chunk <1MB」目标未达成的说明

PR1 基线设目标「Home chunk 2.5 MB → <1 MB」。实测 mermaid 懒加载后 Home.js 仍 2.5 MB，原因与处置：

- **基线归因偏差**：PR1 基线原以为 cytoscape/katex 在 Home.js 内，实际 vite 已拆为独立 chunk（见维度 4.1 勘误）。mermaid 懒加载把图表库移出首屏 eager 图（首屏 −1.8 MB），但 Home.js 文件本身不含这些图表库代码，故字面体积下降有限。
- **Home.js 剩余体积构成**（经 `grep` 库签名探查 Home.js 内容）：仍含 `xlsx`（SheetJS，~400 KB，`sheet_to_json`/`SheetJS` 命中）、`highlight.js`（12 语言，`registerLanguage` 命中 15 次）、`@codemirror/*`（`EditorView`/`lineNumbers` 命中）、`markdown-it`（`MarkdownIt` 命中）。这些是文本/代码/markdown 预览的**高频路径依赖**，非仅图表场景才用。
- **评估结论（按 PR4「其他 >200KB 大依赖评估」要求）**：
  - `xlsx` / `docx-preview`：>200 KB，**跳过懒加载**。`FilePreviewRenderer.vue` 顶部注释明确记载历史曾用动态 import，因 wails dev 下 Vite 把动态 import 重写为 `.vite/deps` 预构建路径、Wails DevServer 对该路径代理不完整致 fetch 失败，故改回静态。此为**已记录的高风险项**，E2E（vite preview）无法暴露 dev 回归，遵从既有静态 import 决策，不在本轮冒险。
  - `highlight.js` / `@codemirror/*` / `markdown-it`：均为预览区高频路径（首次点击任意文本/代码/markdown 文件即需），懒加载会致首次文件预览闪等，UX 回归风险高；且 highlight.js / markdown-it 接近或低于 200 KB 阈值。**跳过**。
- **结论**：Home <1 MB 需懒加载上述高频预览依赖，风险/收益不划算（违反「禁盲目优化：风险高的跳过」）。PR4 收口于 mermaid/cytoscape/katex 懒加载这一**低风险、首屏收益最大**项，首屏 eager JS 实减 ~1.8 MB。

### 9.3 跳过：文件树冷扫描 allocs 优化（收益 <10%）

- **before**：6,358 allocs/op（2.31 ms）。
- **评估**：6358 分配主要由每节点 `model.NewFileTreeNode`（struct + 字段 string 分配）、`filepath.Join`（string 拼接）、`sort.Slice` 闭包构成。预分配 nodes slice 容量（`make([]*model.FileTreeNode, 0, len(entries))`）仅省 ~10 次切片扩容分配，占比 0.16%，**远低于 10% 阈值**。显著降分配须引入 FileTreeNode 对象池——复杂度高，且 treeCache 命中路径经 deepCopy 持有节点引用，对象池易致**treeCache 8.6× 提速回退**（PR4 硬约束）。
- **结论**：按「收益 <10% 跳过并记录原因」跳过。冷扫描 2.09 ms 已快，热路径缓存命中 0.28 ms 不受影响。

### 9.4 不动项（基线已证非瓶颈）

- **NewAppServices 5.80 ms**：startup 一次，用户感知阈值 ~100 ms 占比小，优化收益不抵风险（不动）。
- **常驻堆 0.6 MB**：已轻量（不动）。
- **ScanGitRepos 0.34 ms / 105 allocs**：.git 预筛已优化到位（不动）。

### 9.5 路线图勾选状态

- 路线图「应用性能 → 启动时间优化」子项「延迟加载非关键模块」：mermaid 懒加载已落实（首屏 eager −1.8 MB），但父项含「优化配置文件读取 / 减少 HTTP 请求」未做，且 GUI 冷启动为手动占位未量化，**父项暂不勾选**。
- 路线图「应用性能 → 内存使用优化」：本轮未涉及对象释放/节点上限/分块读取，**不勾选**。

## 10. 约束与噪声说明

- **不改生产代码**：本基线仅新增 `service/perf_bench_test.go` + `perf_bench_test.go` + `scripts/perf-baseline.sh` + 本文档；唯一非 _test.go 改动是 `util/testutil/testutil.go` 参数类型由 `*testing.T` 宽化为 `testing.TB`（test-only 包，不进生产二进制，向后兼容，使 benchmark 可复用 fixture 构造函数）。PR4 生产改动仅 `FilePreviewRenderer.vue`（mermaid 懒加载）+ `vite.config.js`（optimizeDeps.include）。
- **MemStats 噪声**：进程级 HeapAlloc 受 GC 时序影响，Δ 可能落在噪声内；精确分配看 benchmark B/op
- **NewAppServices benchmark artifact**：每次构造重置全局 logger 致 lumberjack 句柄累积，用 500x 固定迭代控制；生产仅 startup 一次无此问题，ns/op 趋势有效
- **GUI 冷启动占位**：sub-agent 无法自动测，待用户手动补数（见维度 6）
- **跨机器不可比**：基线受机器配置影响，PR4 复测须同机器
- **wails dev 动态 import 风险**：mermaid 懒加载经 `optimizeDeps.include` 缓解 dev 依赖发现问题；生产 build + vite preview（E2E）已验证通过。wails dev 实机仍建议人工首测一次 mermaid markdown 渲染（sub-agent 无法跑 GUI）

## 11. 相关文档

- [test-coverage-gate.md](test-coverage-gate.md) — benchmark 不纳入覆盖率门禁（_test.go 仅测试二进制编译，coverage-check.sh 不跑 -bench）
- [test-stability.md](test-stability.md) — benchmark 稳定性约定（fixture 构造排除耗时、禁 sleep、缓存命中依赖 mtime 未变）
- [app-services-assembly.md](app-services-assembly.md) — NewAppServices 装配契约（纯构造、benchmark 测的就是它）
- [cross-layer-contracts.md](cross-layer-contracts.md) — wailsjs 绑定契约（mermaid 懒加载不涉签名变更，绑定零 diff）
