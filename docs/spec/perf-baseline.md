# 性能基线（v1.4 PR1）

> 本文档记录 WorkBench v1.4 平台加固前的性能基线数据，供 PR4 性能优化前后对比。
> 严格遵循「先测后优」：每项优化须有 before/after 量化支撑（09-20 提交历史优化见第 12 节）。
> 最后更新：2026-09-27 · 来源任务：09-14-v1-4 PR1 子项 3 / PR4 子项 4 / 09-20-commit-history-perf / 09-25-perf-soak-leak-detect / 09-27 冷启动量化（§6）/ 09-27 前端内存维度量化（§15）/ 09-27 前端内存驻留面量化（§16）

## 1. 适用范围

- Go 后端核心路径 benchmark 基线：文件树构建、仓库扫描、App service 装配、提交历史冷扫
- 前端 bundle 体积基线：dist 总量、各 chunk 体积、代码分割候选
- Go 运行时内存占用快照：HeapAlloc / HeapSys / NumGC
- 长跑稳定性测试（soak）：核心读路径高频循环 + 检查点趋势采样（第 13 节）
- 内存泄漏自动检测：heap/goroutine 双 GC 断言 + goleak 三档分策（第 14 节）
- GUI 冷启动耗时：WebView2 初始化 + Go startup + 前端首屏（自动化测量已收口，见第 6 节）
- 前端内存量化：文件树规模曲线（JSHeap/DOM/耗时）+ 大文件预览传输-内存曲线（第 15 节）+ 组件驻留面残留（第 16 节）
- 性能优化前后对比的唯一数据依据

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

### 3.3 主包（提交历史冷扫）

采集命令：`go test -bench=BenchmarkGetCommitHistory_ColdScan -benchmem -benchtime=2s -run=^$ ./`

| Benchmark | 采集层 | 迭代数 | ns/op | B/op | allocs/op | 说明 |
|---|---|---:|---:|---:|---:|---|
| `BenchmarkGetCommitHistory_ColdScan` | go-git（before） | 1 | 12,839,285,200 (~12.84 s) | 53,995,504 (~51.5 MB) | 947,603 | 300 提交冷扫，go-git 逐条树 diff（每提交 Patch ≥2 次对象库读），优化前实现 |
| `BenchmarkGetCommitHistory_ColdScan` | CLI git log（after） | 9 | 300,395,267 (~0.30 s) | 479,859 (~469 KB) | 2,451 | 同一基准，CLI 批量采集（2026-09-20 换层后，见第 12 节） |

**关键结论**：
- 300 提交冷扫从 12.84 s 降至 0.30 s（**~42.7× 提速**），根因（go-git 逐条树 diff 对象库读放大）证实，详见第 12 节。
- fixture 300 提交（testutil 构造，StopTimer/StartTimer 排除构造耗时），每迭代 `ClearByGitRoot` 强制缓存 miss 走全量扫，取单页 limit=20。

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

## 6. 维度 4：GUI 冷启动（2026-09-27 量化收口）

> 测量机同第 2 节（Windows 11 / Ryzen 7 H 255 / 31.27 GB），采集时间 **2026-09-27**，
> 测量对象 `build/bin/workbench.exe`（2026-09-24 构建，含当天前全部生产代码）。本机
> settings.json `gpuDisabled: true`（WebView2 GPU 加速关闭），数据受此影响，跨机器仅看相对趋势。
> 自动化脚本：`scripts/cold-start-bench.ps1`（GUI 段）+ `scripts/cold-start-frontend.mjs`（前端段）。

### 6.1 测量方法（自动化为主 + 人工秒表交叉验证）

1. **GUI 段（`cold-start-bench.ps1`，全自动）**：脚本 `Start-Process` 拉起 exe 记 t0，
   增量解析 `data/logs/app.log` slog JSON 时间戳（`logger initialized` / `web serve listening` /
   `workbench started`），轮询 TCP 36115 就绪（10ms 间隔）与 `MainWindowHandle != 0`
   （WebView2 宿主窗口创建完成代理），稳态后 `Get-Process` 采样工作集；每轮 WM_CLOSE
   优雅退出，3 轮热身 + 5 轮采样取中位数。
2. **前端段（`cold-start-frontend.mjs`，全自动代理口径）**：同一份 embed 前端资产经
   `--serve` 浏览器通道在 Playwright Chromium 中加载（独立 context 冷缓存，浏览器进程
   复用 V8 暖），量 `goto` 到 `.directory-tree-panel` 可见（首屏渲染）与三栏布局挂载
   （可交互代理），并采集请求数/传输体积佐证路线图子项定性。WebView2 与 Chromium 同核，
   渲染端差异仅环境，资产与代码路径一致。
3. **人工秒表交叉验证（可选，WebView2 首帧口径）**：自动化只能测到窗口句柄出现
   （不含首帧绘制）。人工验证步骤：手机慢动作视频（或秒表）拍摄双击 exe 到窗口内容
   首次绘制的间隔，重复 5 次取中位，与「句柄出现 1090.5 ms + 前端首屏 468 ms ≈ 1.56 s」
   代理估算值对照（偏差应 <300 ms）。未做人工验证时，文档按代理口径标注。

### 6.2 基线表（2026-09-27 实测，脚本审核修复后复测批次，5 轮中位数）

| 指标 | 基线值 | 测量方法 | 说明 |
|---|---|---|---|
| 进程拉起 → logger 初始化 | 884.9 ms | slog 时间戳（`logger initialized`） | OnStartup 进入后 NewAppServices 内首条日志；该段含 Go runtime + Wails 窗口/WebView2 环境初始化（OnStartup 在窗口创建后触发）；冷态首跑批次 1107.9 ms（机器热态 ±15~25%） |
| OnStartup 内部段 | 21.4 ms | slog 时间戳差（`logger initialized` → `workbench started`） | NewAppServices 装配 5.80 ms（维度 1）+ crash flag / 更新检查 / web serve 启动；**Go 侧非瓶颈** |
| 后端服务就绪（TCP 36115） | 956.9 ms | 主事件循环内 TcpClient 单次探测（与日志事件同循环采样） | 浏览器通道可接受请求点 |
| WebView2 宿主窗口创建 | 1090.5 ms | `MainWindowHandle != 0` 轮询代理（上界） | 窗口创建完成下界，不含首帧绘制；实测句柄晚于 startup 日志出现，读数含轮询启动间隙 |
| 前端首屏渲染 | 468 ms | 浏览器通道代理（Chromium 冷缓存，`.directory-tree-panel` 可见） | 页内 first-paint ~36 ms / load ~110 ms，差值为 JS 执行 + Vue 挂载 + GetDirectories RPC |
| 前端首屏可交互 | 488 ms | 浏览器通道代理（三栏布局全部挂载） | 与首屏渲染差 20 ms（布局挂载增量） |
| 进程内存（稳态工作集） | 57.0 MB | `Get-Process WorkingSet64` | 跨批次波动 57–100.7 MB（受 OS 工作集修剪影响大，量级参考）；私有内存 72.2 MB；大头为 WebView2 宿主进程，Go 侧常驻堆 0.6 MB（维度 3） |

**用户感知总冷启动 ≈ 1.56 s**（热态：窗口句柄 1090.5 ms + 前端首屏 468 ms 代理估算；
冷态首跑批次 ≈ 1.85 s；人工秒表口径见 6.1 第 3 步）。

**结论**：耗时大头在 Wails 窗口/WebView2 宿主初始化（~0.9 s，框架层，项目代码可控面小）
与前端 JS 执行/挂载（~0.36 s，懒加载已优化过一轮）；Go 侧 OnStartup 全段仅 21.4 ms，
配置文件读取与资产请求均非瓶颈（详见 9.4）。当前量级属桌面 WebView2 应用正常水平，
无瓶颈级启动问题，启动时间优化按「不动项」收口。

## 7. 测量方法与命令（可复现）

一键采集脚本：`bash scripts/perf-baseline.sh`（封装以下命令）

| 维度 | 命令 | 产物文件 |
|---|---|---|
| 1a service benchmark | `go test -bench=. -benchmem -benchtime=2s ./service/` | `service/perf_bench_test.go` |
| 1b 主包 benchmark | `go test -bench=BenchmarkNewAppServices -benchmem -benchtime=500x -run=^$ ./` | `perf_bench_test.go` |
| 1c 提交历史冷扫 benchmark | `go test -bench=BenchmarkGetCommitHistory_ColdScan -benchmem -benchtime=2s -run=^$ ./` | `commit_history_bench_test.go` |
| 3 MemStats | `go test -run TestPerfMemStats -v ./` | `perf_bench_test.go` |
| 4 GUI 冷启动（GUI 段） | `powershell -ExecutionPolicy Bypass -File scripts/cold-start-bench.ps1 -Runs 5` | `data/logs/app.log`（须先关闭常驻 workbench.exe） |
| 4 前端首屏（浏览器通道代理） | `node scripts/cold-start-frontend.mjs --runs 5` | `build/bin/workbench.exe --serve`（无头，不触碰桌面 36115） |
| 13 长跑稳定性（默认档随 `go test ./...` 自动跑） | `go test ./service/ -run TestSoak -v` 与 `go test ./ -run TestSoak -v` | `service/soak_leak_test.go` / `soak_leak_test.go` |
| 13 长跑稳定性（深跑档） | `go test ./service/ -soak-rounds=2000 -run TestSoak -v` 与 `go test ./ -soak-rounds=2000 -run TestSoak -v` | 同上（flag 须置于包名之后） |
| 14 泄漏断言阈值调整 | 上述命令追加 `-leak-heap-mb=N`（service 缺省 8 / 主包缺省 16） | 同上 |
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
- **配置文件读取（2026-09-27 冷启动量化补证）**：OnStartup 全段仅 21.4 ms（§6.2），settings.json 等本地 <2KB 文件经 OS 缓存读取占比 <5%，非瓶颈（不动）。
- **资产 HTTP 请求（2026-09-27 冷启动量化补证）**：首屏 47 个回环请求 load 仅 ~110 ms，执行+挂载占大头（~360 ms，§6.2），请求数非瓶颈（不动）；后续优化方向若做，应指向 JS 执行/bundle 体积（懒加载已做一轮）。
- **Wails/WebView2 宿主初始化 ~1.1 s（§6.2 大头）**：框架层耗时，项目代码可控面小，属桌面 WebView2 应用正常水平（不动）。
- **大文件分块读取（2026-09-27 前端内存量化补证）**：ReadFileBytes 50MB 上限内峰值驻留 ~64 MB JSHeap（§15.3），膨胀 1.333x 与理论吻合，超限 TooLarge 降级无 OOM 面——50MB 上限即风险挡板，非瓶颈（不动）。

### 9.5 路线图勾选状态

- 路线图「应用性能 → 启动时间优化」：**已勾选（2026-09-27 收口）**。三个子项闭环——「延迟加载非关键模块」mermaid 懒加载已落实（首屏 eager −1.8 MB）；「优化配置文件读取」「减少 HTTP 请求」经冷启动量化（§6.2）证明非瓶颈，按 9.4 先例记不动项；耗时大头 Wails/WebView2 宿主初始化为框架层不可控面，总冷启动 ~1.56 s（热态）属正常水平。
- 路线图「应用性能 → 内存使用优化」：本轮未涉及对象释放/节点上限/分块读取，**不勾选**。（2026-09-27 更新：前端内存量化已收口（§15）——「限制文件树节点数量」立项实锤（100k 节点渲染 49.2 s + 3.14 GB，量化目标见 §15.4），「大文件分块读取」按 §15.3 记不动项；「及时释放不再使用的对象」未测保持不勾选）

## 10. 约束与噪声说明

- **不改生产代码**：本基线仅新增 `service/perf_bench_test.go` + `perf_bench_test.go` + `scripts/perf-baseline.sh` + 本文档；唯一非 _test.go 改动是 `util/testutil/testutil.go` 参数类型由 `*testing.T` 宽化为 `testing.TB`（test-only 包，不进生产二进制，向后兼容，使 benchmark 可复用 fixture 构造函数）。PR4 生产改动仅 `FilePreviewRenderer.vue`（mermaid 懒加载）+ `vite.config.js`（optimizeDeps.include）。
- **MemStats 噪声**：进程级 HeapAlloc 受 GC 时序影响，Δ 可能落在噪声内；精确分配看 benchmark B/op
- **NewAppServices benchmark artifact**：每次构造重置全局 logger 致 lumberjack 句柄累积，用 500x 固定迭代控制；生产仅 startup 一次无此问题，ns/op 趋势有效
- **GUI 冷启动测量噪声（2026-09-27）**：复跑中位偏差 ±15%（OS 文件缓存/Defender/机器热态），脚本内置 3 轮热身缓解；复跑需先关闭常驻 workbench.exe（脚本预检拒绝端口占用与日志串场）；本机 `gpuDisabled: true` 影响首帧耗时；前端段为 Chromium 代理口径，与 WebView2 渲染端存在环境差异（资产与代码路径一致）
- **跨机器不可比**：基线受机器配置影响，PR4 复测须同机器
- **wails dev 动态 import 风险**：mermaid 懒加载经 `optimizeDeps.include` 缓解 dev 依赖发现问题；生产 build + vite preview（E2E）已验证通过。wails dev 实机仍建议人工首测一次 mermaid markdown 渲染（sub-agent 无法跑 GUI）

## 11. 相关文档

- [test-coverage-gate.md](test-coverage-gate.md) — benchmark 不纳入覆盖率门禁（_test.go 仅测试二进制编译，coverage-check.sh 不跑 -bench）
- [test-stability.md](test-stability.md) — benchmark 稳定性约定（fixture 构造排除耗时、禁 sleep、缓存命中依赖 mtime 未变）
- [app-services-assembly.md](app-services-assembly.md) — NewAppServices 装配契约（纯构造、benchmark 测的就是它）
- [cross-layer-contracts.md](cross-layer-contracts.md) — wailsjs 绑定契约（mermaid 懒加载不涉签名变更，绑定零 diff）
- `scripts/cold-start-bench.ps1` / `scripts/cold-start-frontend.mjs` — GUI 冷启动与前端首屏自动化测量脚本（§6，Windows 本机专用，不进 CI）

## 12. 提交历史采集层优化（09-20 任务）

### 12.1 before/after 对比

根因：采集层经 go-git 逐提交做树 diff（`getCommitFiles` 的 `Patch`，每条 ≥2 次对象库读），
Windows 上读放大 1-2 个数量级。优化将采集层换为 CLI `git log` 批量/流式子进程（workbench
既有依赖，Push/Pull/Fetch 已用，零新增）。

采集命令：`go test -bench=BenchmarkGetCommitHistory_ColdScan -benchmem -benchtime=2s -run=^$ ./`
（300 提交 fixture，每迭代 `ClearByGitRoot` 强制冷扫，GetCommitHistory 取单页 limit=20，
同机同环境先后两次采集）

| 指标 | before（go-git 逐条树 diff） | after（CLI git log 批量） | 变化 |
|---|---:|---:|---:|
| ns/op | 12,839,285,200 (~12.84 s) | 300,395,267 (~0.30 s) | **−12.54 s（~42.7× 提速）** |
| B/op | 53,995,504 (~51.5 MB) | 479,859 (~469 KB) | **~112× 降分配量** |
| allocs/op | 947,603 | 2,451 | **~386× 降分配次数** |
| 迭代数（2s 预算） | 1 | 9 | — |

**关键结论**：
- **根因证实**：go-git 逐条树 diff ~42 ms/条（300 条 12.84 s，2s 预算仅容 1 次操作）；
  CLI 单子进程批量采集 0.30 s。满足验收「提升 ≥5×」且达 1-2 数量级预期。
- **after 的 0.30 s 构成**：约 2/3 为 Windows git 进程创建开销（沙盒对照：裸 `git log`
  300 提交 5 次共 1.008 s ≈ 200 ms/次，user 0.015s / sys 0.200s，系统调用占绝对大头），
  已是单进程采集的下限量级。冷扫仅首启 / TTL 过期 / 手动刷新触发，日常翻页与过滤走内存
  缓存命中路径，不触采集。
- **overflow 流式**：>5000 提交仓库全量扫换 `-n 5001` 探测超限；翻页路径 CLI 流式读
  stdout（\x00 分隔逐记录解析），凑够 limit 关闭管道提前终止不读全量，增量 prepend 命中
  缓存 SHA 交集即停。翻页/过滤/不缓存语义由 git fast-import 构造的 5001 提交集成测试锚定
  （单子进程秒级构造，规避逐条 commit 分钟级 fixture 成本）。

### 12.2 改动与语义零漂移锚点

- **采集层**（`commit_history_cli.go` 新增 + `app_git.go` 换实现）：`fullScanCommits` /
  `incrementalCommits` / `fetchCommitHistoryFromGit` 签名由 `*git.Repository` 改
  `gitRoot string`（git.PlainOpen/Head 保留供缓存键与空仓判定）；`getCommitFiles` /
  `getTreeFiles` 按 PRD 保留（历史主路径不再逐条调用）。
- **缓存层零改动**：CommitHistoryCache key/TTL/SHA 链增量判定/失效入口（ClearPath /
  ClearByGitRoot / ClearAll）/ cap 5000 与 overflow 不缓存策略全保留。
- **过滤零下推**：Author/Keyword/Since/Until/FilePath 内存过滤，不下推 CLI flags
  （pathspec 子串与 `--author` 正则语义均与内存子串匹配不等价）；缓存路径 `filterCommits`
  与流式路径共用 `commitFilterPredicate` 单一实现防漂移。
- **字段语义对齐 go-git**（字节级）：
  - Message 取 `%B`（raw message 含 subject+body+尾部换行，与 `commitObj.Message` 一致；
    消息内 `\x01` 由 `parseCommitLogChunk` 终止符反向定位消化（「最后一个后随 `\n\n`
    的 `\x01`」+ 空提交尾部 `\x01\n` 兜底锚点，头部 40 位 hex SHA/parents/ts/ai 强校验
    快速失败）；git 原生拒绝消息含 NUL，`\x00` 记录分隔安全）；
  - Timestamp 取 `%at`（对齐 `Author.When.Unix()`）；DateTime 取 `%ai` 首 19 字节
    （对齐 `Author.When.Format("2006-01-02 15:04:05")` 作者时区墙钟，非本地时区）；
  - merge 提交 `--diff-merges=first-parent`（对齐 `Parent(0)` diff，须 git ≥2.31）；
  - root 提交（parents 空）文件列表截断 100（对齐 `getTreeFiles` 上限）；
  - `-c core.quotePath=false` 输出原始非 ASCII 路径（缺省转义会破坏 FilePath 子串过滤）；
  - 排序：git log 缺省按 committer date 弹出，线性链退化为链序，与 go-git
    LogOrderCommitterTime 语义一致（时间乱序 fixture 测试锚定）。
- **行为锚定**：既有 GetCommitHistory 全部过滤组合 / 缓存命中 / 增量 prepend / SHA 链断 /
  日期区间测试未改一字全绿（语义零漂移硬验证）；新增解析单测（换行 / 中文 / `\x01` 消息、
  root 截断、畸形输入报错）与行为测试（Message 字节级、committer 序、merge、非本地时区
  DateTime、流式提前终止）。

### 12.3 前端与绑定

前端与 `frontend/wailsjs/` 零改动（GetCommitHistory / GetRepoStats / Invalidate 签名不变，
绑定零 diff）。

### 12.4 语义决策与已知上限记录（09-20 check 审核修复）

- **Since/Until 统一按 author 时间**：CLI 采集层的 Timestamp 取 `%at`（author unix 秒），
  Since/Until 内存过滤统一按 author 时间命中——与缓存路径既有语义一致。原 go-git uncached
  路径按 committer 时间过滤，换层后两路径统一为 author 时间（方向正确，非漂移）。
  决策记录：author≠committer 提交（`GIT_AUTHOR_DATE` / `GIT_COMMITTER_DATE` 环境变量分离
  构造）单测锚定日期过滤按 author 时间命中（`TestGetCommitHistory_DateFilterAuthorTimeBasis`）。
- **流式采集 30s 超时已知上限**：overflow 流式路径（`streamCommitLog`）沿用
  `commitLogTimeout = 30s`（与 `util.GitCommand` 缺省超时对齐）。数十万提交量级的超限仓库
  + 无匹配过滤条件时，流式须读完全部历史才能确认无匹配，可能触发 30s 超时报错。属已知
  上限，维持现状（超出桌面仓库常见量级；凑够 limit 的常规翻页不受影响）。
- **采集失败可观测**：全量扫/增量流式的 git 失败（启动失败、非零退出，错误含 args 与
  stderr 摘要）落 `slog.Warn` 并向上 `%w` 包裹（「无法获取提交历史: ...」），不再静默吞错。

## 13. 长跑稳定性测试（soak，09-25 任务）

**结论前置**：四条核心读路径 + 提交历史缓存层深跑（GetTree/Scan 2000 轮、GetLocalChanges 400 次 fork、GetCommitHistory 300 轮）全部无趋势级泄漏——heap 前后 20% 均值变化在 ±1.3 MB 内（锯齿震荡无单调增长），goroutine 全程稳定 2-3，treeCache 条目恒 11（理论上界），缓存条目单键覆盖写恒 1，400 次 fork git status 无句柄累积。

### 13.1 覆盖路径与三档轮次

测试文件：`service/soak_leak_test.go`（4 条 + 全仓首个 TestMain）+ 主包 `soak_leak_test.go`（GetCommitHistory App 全链路）；采样/断言辅助收敛 `util/testutil/soak.go`（跨包共用）。轮次驱动（非时间盒）：工作负载确定，heap 增量可比。flag 解析优先级：`-soak-rounds` > `-short` 缩减 > 默认档。

| 路径 | 默认档（CI 裸 `go test ./...` 即此档） | Short 档 | 深跑档（`-soak-rounds=N`） |
|---|---|---|---|
| TestSoakFileTreeGetTree（treeCache 命中 + 每 10 轮手动 InvalidateCache 失效） | 200 轮 | 50 | N（2000 实测） |
| TestSoakScanGitRepos（生产 scanCache 路径 + ClearScanCache 失效） | 200 轮 | 50 | N |
| TestSoakGetLocalChanges（每轮 fork git status） | 40 轮 | 10 | N/5 |
| TestSoakCommitHistoryCacheGrowth（单键覆盖写 + 双向深拷贝） | 200 轮 | 50 | N |
| TestSoakGetCommitHistory（主包，命中 + 每 10 轮 InvalidateCommitHistoryCache 失效全扫） | 30 轮 | 8 | N×3/20 |

### 13.2 实测数据（Windows 11 测量环境同第 2 节，2026-09-25）

| 测试 | 默认档耗时 | 深跑档耗时（轮次） | 泄漏断言（heap 增量/阈值，goroutine 增量） |
|---|---:|---:|---|
| TestSoakFileTreeGetTree | 0.57s / 200 轮 | 1.23s（2000） | −1 MB / 8 MB，+0 |
| TestSoakScanGitRepos | 1.66s / 200 轮 | 5.88s（2000） | −1 MB / 8 MB，+0 |
| TestSoakGetLocalChanges | 1.99s / 40 轮 | 14.66s（400） | +0 MB / 8 MB，+0 |
| TestSoakCommitHistoryCacheGrowth | 0.01s / 200 轮 | 0.04s（2000） | −1 MB / 8 MB，+0 |
| TestSoakGetCommitHistory（主包） | 1.32s / 30 轮 | 3.04s（300） | +0 MB / 16 MB，+0 |

时长预算：默认档（CI 档）service 4.4s + 主包 1.5s ≈ **6s 增量**；`-short` 档 ≈ 4s；深跑档 service 22s + 主包 3.2s ≈ 25s。

### 13.3 趋势结论（深跑档检查点前/后 20% 均值）

| 路径 | 前 20% 均值 HeapAlloc | 后 20% 均值 | 变化 | 结论 |
|---|---:|---:|---:|---|
| GetTree（2000 轮） | 2,528 KB | 2,536 KB | +8 KB（4 KB/千轮） | 无增长；treeCache 条目恒 11（根 + 10 子目录理论上界，断言上限 32） |
| ScanGitRepos（2000 轮） | 2,134 KB | 1,902 KB | −232 KB | 锯齿回落，无单调增长 |
| GetLocalChanges（400 次 fork） | 1,984 KB | 2,107 KB | +124 KB | 无句柄/僵尸进程累积（子进程 Wait 闭环有效） |
| CommitHistoryCacheGrowth（2000 轮） | 2,354 KB | 1,066 KB | −1,289 KB | 单键覆盖写条目恒 1，每轮 50 条双向深拷贝均回收 |
| GetCommitHistory（300 轮） | 2,468 KB | 1,625 KB | −842 KB | 缓存命中深拷贝分页 + 周期性全量重扫均无残留 |

趋势采样仅输出（`t.Logf` 检查点表）不参与断言：GOGC 动态调整与碎片化可能造出伪线性段，阈值断言只抓绝对增量（第 14 节口径）。

### 13.4 稳定性约定

- 缓存失效全部由手动入口驱动（`InvalidateCache` / `ClearScanCache` / `InvalidateCommitHistoryCache`），不依赖 NTFS mtime 时序精度；禁 `time.Sleep`；不涉网络端口（[test-stability.md](test-stability.md) 全文有效）
- 每条路径循环前预热一次（排除惰性初始化：缓存 map 首建、gitCmd 构造等），泄漏基线从预热后起算
- 主包 `NewAppServices` 仅构造一次（多次构造重置全局 logger 致 lumberjack 句柄累积的 artifact，见 3.2 节，本测试天然规避）

## 14. 内存泄漏自动检测（09-25 任务）

### 14.1 断言口径（宁松勿紧：抓趋势级灾难泄漏，不抓噪声级波动）

| 断言 | 口径 | 阈值 | 入 CI |
|---|---|---|---|
| heap 泄漏 | 循环前后各两次 `runtime.GC()` 后 HeapAlloc 增量（第一轮 GC 触发 finalizer 调度，第二轮回收其释放的引用链，快照才收敛为存活堆） | service 8 MB / 主包 16 MB，`-leak-heap-mb` 可调 | 是（默认档随 `go test ./...`） |
| goroutine 泄漏（计数） | 循环前后 `runtime.NumGoroutine()` 增量 | ≤2 | 是 |
| goroutine 泄漏（栈集合） | goleak 按栈快照 diff（能识破计数对比的「旧退新进」身份置换盲区） | 零容忍（深跑档硬门禁） | 否（两步走，见 14.3） |

heap 阈值取宽（服务层常驻堆仅 ~2 MB 量级，8/16 MB 阈值意味着允许翻 4-8 倍才算泄漏）；CI 偶发失败比漏检危害大，偶发噪声靠宽松阈值兜底，出现误报先调阈值/登记良性栈，不撤门禁。

### 14.2 goleak 三档分策（决策：引入 go.uber.org/goleak v1.3.0，版本钉死）

依赖成本实测：go.mod +1 行 / go.sum +2 条，零传递依赖（testify 被模块图剪枝）；引入后 `govulncheck ./...` 影响代码漏洞 0。选型依据：手写计数有身份置换盲区；goleak 栈集合 diff + 自动打印泄漏栈 + `IgnoreTopFunction` 官方随版本维护；prometheus/tidb/k8s 先例。

service 包 TestMain（全仓首个）三档：

| 档位 | goleak 行为 | 理由 |
|---|---|---|
| `-short` | 完全跳过 | 保 `-short` 增量 <30s |
| `-soak-rounds>0`（深跑档） | `goleak.VerifyTestMain` 硬门禁（泄漏打印栈并非零退出） | 显式深跑即有意观察泄漏 |
| 默认档（CI） | 观察模式：`goleak.Find` 结果仅 stderr 告警，不改退出码 | 见 14.3 两步走 |

主包不挂 goleak TestMain（websocket Hub 等常驻 goroutine 误报面大），主包 goroutine 门禁即 Δ≤2 计数断言。

### 14.3 两步走：观察模式先行，基线干净后收紧硬门禁

service 包既有 7 个测试文件启动 goroutine（并发压测 worker、chat fake 进程输出泵、terminal 读泵等），包级硬门禁对慢退出/残留栈有误报风险，故 CI 默认档先观察：

- **观察期发现**：全量 service 套件曾捕获一次非确定性残留——`(*TerminalService).watchProcess`（`service/terminal.go` CreateTerminal 起，time.Sleep 轮询驻留），复跑未复现。此为 `IgnoreTopFunction`（全限定名）候选，登记与否待观察期积累：确定性复现才登记，非确定性残留优先修测试清理逻辑。
- **收紧条件**：连续数周 CI 观察模式无新告警后，将默认档 `goleak.Find` 分支替换为 `VerifyTestMain`（单行改动），良性栈逐个 `IgnoreTopFunction` 登记，不放宽全包豁免。

### 14.4 覆盖范围说明

| 有状态路径 | 处置 |
|---|---|
| treeCache 增长 | 已覆盖：条目有界断言（≤32，理论 11）随 TestSoakFileTreeGetTree |
| commitHistoryCache 增长 | 已覆盖：TestSoakCommitHistoryCacheGrowth（单键覆盖写 + 深拷贝隔离抽查）+ 主包全链路 |
| 事件订阅 | 核实无泄漏面：`sinkHolder` 为单出口持有器（`SetEventSink` 换引用，非累积订阅者注册表），不构成泄漏路径，不专项测试 |
| 终端会话 | 不进自动长跑：须 fork 真实 pty/conpty 进程（conpty 为 Windows-only 依赖、CI 为 Linux runner、进程 fork 成本高且会污染计数断言）；goroutine 驻留风险由 goleak 观察模式兜底（14.3 已捕获 watchProcess 残留即证明该路径有暴露面） |

### 14.5 运行方式速查

```bash
# 默认档（CI 即此档，自动随 go test ./... 跑）
go test ./service/ -run TestSoak -v
go test ./ -run TestSoak -v

# 深跑档（本机手动；flag 须置于包名之后）
go test ./service/ -soak-rounds=2000 -run TestSoak -v
go test ./ -soak-rounds=2000 -run TestSoak -v

# 调整 heap 阈值
go test ./service/ -run TestSoak -leak-heap-mb=16 -v
```

注意：`-soak-rounds`/`-leak-heap-mb` 为 service 与主包各自注册的同名 flag（独立测试二进制无冲突），但 `go test ./... -soak-rounds=N` 会因未注册该 flag 的包报错，跨包深跑须按包分开传参。

## 15. 维度 5：前端内存量化（09-27 前端内存维度任务）

> 背景：第 6 节冷启动收口证明进程工作集大头在 WebView2 宿主而非 Go（常驻堆仅 0.6 MB），
> Go 侧内存优化无收益，真实内存风险在前端渲染面。本节量化两条前端内存曲线，为路线图
> 「应用性能 → 内存使用优化」下「限制文件树节点数量」「大文件分块读取」两子项提供数据决策。

### 15.1 测量方法

| 项 | 说明 |
|---|---|
| 曲线一脚本 | `node scripts/frontend-memory-tree.mjs [--runs 3] [--scales 1000,10000,100000]` |
| 曲线一脚链路 | 选中工作目录 → `loadTreeNode` → `App.GetFileTree`（`GetChildren` 单层）→ el-tree 全量渲染（无虚拟滚动，`lazy` 懒加载展开即全量） |
| 曲线一采样 | 独立 context 冷缓存，控制目录（空树）归零基线 → 点击 scale 目录 → `.el-tree-node` 计数达标判渲染完成；CDP `Performance.getMetrics`（JSHeapUsedSize，基线与渲染后采样前均经 `HeapProfiler.collectGarbage` 强制 GC，消除跨档残留——实测无 GC 时 10k 档驻留读数虚高 255 MB（581→326））+ 页内 `querySelectorAll('*')`（DOM 计数，CDP Nodes 指标 headless 动态插入不刷新已弃用） |
| 曲线二脚本 | `node scripts/frontend-memory-preview.mjs [--runs 3]` |
| 曲线二链路 | 浏览器通道 RPC 直调 `App.ReadFileBytes`（单参，50MB 上限硬编码 `app_preview.go`）→ 页面堆 base64 字符串驻留 → `atob` → `Uint8Array`（与 `FilePreviewRenderer` `base64ToUint8Array` 同实现） |
| 曲线二采样 | CDP JSHeapUsedSize 双驻留点（RPC 返回后 / decode 后，采样前强制 GC）；`performance.memory` 同执行流内读数不刷新已弃用；decode 中间串（atob）无引用 GC 即回收，实测 decode 驻留增量 ≈0，峰值在 RPC base64 驻留点 |
| 隔离 | 两脚本 serve 进程 cwd 指向临时目录，`data/`（工作目录配置/settings/web_token）与 fixture 全部落临时目录，用户真实配置零污染 |
| 口径 | 每轮独立 context = 冷缓存，浏览器进程复用 = V8 暖（§6 前端段同口径）；fixture 内容伪随机防 V8 字符串去重扭曲读数 |

**PRD 假设修正**：任务 PRD 假设大文件用「文本与 markdown」测——实测链路核实文本/markdown
走 `PreviewFile` 1MB 通道（`ContentPanel.vue` `needsBytes` 仅 image/office 触发
ReadFileBytes，PDF 走 iframe + Range 流式无上限），故曲线二 fixture 用 `.png`（image kind），
口径对齐真实用户路径。

### 15.2 曲线一：文件树「规模-内存-耗时」（2026-09-27 实测，3 轮中位）

| 单层节点量 | Go `GetChildren`（benchmem） | 前端渲染耗时 | DOM 节点增量 | JSHeap 增量 | JSHeap 驻留 |
|---|---|---|---|---|---|
| 1k | 1.31 ms / 534 KB / 5,036 allocs | ~346 ms | +7,994 | ~32 MB | ~44 MB |
| 10k | 35.2 ms / 8.0 MB / 60,055 allocs | ~4.36 s | +79,998 | ~311 MB | ~326 MB |
| 100k | 306 ms / 83 MB / 600,076 allocs | **~49.2 s** | +799,989 | **~3.14 GB** | **~3.15 GB** |

（GC 口径重测后增量与驻留收敛，上表为最终口径数据；无强制 GC 的首批测量 10k 档驻留
581 MB 系跨档残留，已废弃。）

Go 侧 benchmark：`go test -bench=BenchmarkFileTreeGetChildren_Scale -benchmem -benchtime=2s -run='^$' ./service/`
（fixture 单目录 N 文件，构造耗时排除计时，与前端脚本 fixture 同构可对照）。

**结论：瓶颈定性在前端渲染，不在 Go。** Go 侧 100k 冷扫仅 306 ms（线性扩展），前端渲染
100k 节点 49.2 s + 3.14 GB JSHeap（每 DOM 节点约 8 元素 × 4 KB 级开销），已属交互卡死级：
- **1k 节点**：~346 ms 无感，属正常（不动）
- **10k 节点**：~4.4 s + 311 MB，交互明显劣化（大数据目录/日志目录/扁平 node_modules 可触发）
- **100k 节点**：49 s + 3.14 GB，灾难级（可致 WebView2 OOM 崩溃）

### 15.3 曲线二：大文件「大小-传输-JSHeap」（2026-09-27 实测，3 轮中位）

| 文件档位 | base64 传输体积 | 膨胀系数 | 峰值 JSHeap 增量 | TooLarge 分支 |
|---|---|---|---|---|
| 1 MB | 1.3 MB | 1.333x | +1.3 MB | - |
| 4 MB | 5.3 MB | 1.333x | +5.1 MB | - |
| 16 MB | 21.3 MB | 1.333x | +21.1 MB | - |
| 48 MB（上限内边界） | 64 MB | 1.333x | +63.8 MB | - |
| 64 MB（超限） | 不传输 | - | 0 | **TooLarge=true，后端拒绝读，无 base64 返回** |

**结论：非瓶颈，50MB 上限合理（不动项）。** base64 膨胀实测 1.333x（理论 4/3 精确吻合）；
上限内最大驻留 ~64 MB JSHeap（48 MB 文件），量级可控；decode 中间串 GC 即回收（decode
驻留增量实测 ≈0），超限走 TooLarge 降级提示，无 OOM 风险面。image 真实路径后续 dataURL
拼接 + `<img>` 位图由合成器持有不在 JSHeap（定性不计）。

### 15.4 路线图勾选状态（「应用性能 → 内存使用优化」）

- **限制文件树节点数量：已实施（2026-09-27 截断优化落地，before/after 见 §15.6）**。
  立项量化锚点：单层展开 ≥10k 节点渲染 ~4.4 s、JSHeap +311 MB，交互明显劣化；
  100k 节点 49.2 s + 3.14 GB 灾难级。实施路线为单层 2k 截断 + 哨兵提示节点
  （el-tree-v2 迁移经评估否决：其无懒加载支持，expand-hack 属非官方路径且
  refreshNode 依赖的 store.nodesMap/loadData 全须重写，改动面 3-5 倍）。
- **大文件分块读取：非瓶颈不动（§15.3）**。50MB 上限内峰值驻留 <100 MB，上限本身
  即风险挡板；分块读取复杂度（Range 协议、渐进渲染、断点状态）收益不抵。
- **及时释放不再使用的对象：本轮未测，保持不勾选**。

### 15.5 复测与噪声说明

- 复测偏差量级：曲线一 1k 档渲染 334-380 ms（±7%）、JSHeap 增量 32.1 MB 稳定；100k 档
  渲染 47.5-51.5 s（±4%）、JSHeap 3.14 GB（±0.1%）——结论量级（100k 灾难级）远超噪声带。
- 曲线二膨胀系数三轮稳定 1.333x；48 MB 档峰值驻留 63.8 MB（±1%）。
- 口径备注：首批测量（无强制 GC）与 GC 口径复测的差异集中在「驻留」列（跨档残留，
  10k 档 581 MB→326 MB），增量列两批差异为 GC 时机噪声（10k 档 211 MB→311 MB，
  首批渲染后未 GC 部分中间对象已回收）；结论不受影响。
- 测量环境与 §6 差异：两脚本 serve cwd 指向临时 data/，settings 走默认值
  （gpuDisabled=false），非沿用 §6 测量机真实配置的 `gpuDisabled: true`；CDP 采样为
  Chromium headless 口径，WebView2 渲染端存在环境差异（JSHeap 量级趋势可参考，绝对值
  受 V8 版本/堆配置影响）。
- 测量机同第 2 节（Windows 11 / Ryzen 7 H 255 / 31.27 GB）。

### 15.6 大目录展开截断优化 before/after（09-27 实施收口，§15.2 同口径 3 轮中位）

**方案**：前端单层节点截断——`loadTreeNode` resolve 前经 `utils/treeTruncate.js`
`truncateTreeNodes` 截至前 2000 项（`FILE_TREE_NODE_LIMIT` 硬编码常量，settings 化
经评估收益不抵跨层契约成本），末尾追加哨兵提示节点（`truncation-hint` 类型，
弱化展示、点击/右键均过滤，文案引导 Ctrl+P 文件搜索定位其余文件）。渲染量与本层
目录规模解耦，OOM 结构性防死。

**before/after 对比**（同脚本 `frontend-memory-tree.mjs`，GC 口径，3 轮中位）：

| 单层节点量 | 渲染耗时（before → after） | JSHeap 增量（before → after） | JSHeap 驻留（before → after） | DOM 节点增量（after） |
|---|---|---|---|---|
| 1k | ~346 ms → ~287 ms | ~32 MB → ~32.1 MB | ~44 MB → ~43.9 MB | +7,998 |
| 10k | ~4.36 s → **~509 ms（-88%）** | ~311 MB → **~63.7 MB（-80%）** | ~326 MB → **~75.4 MB（-77%）** | +16,008 |
| 100k | ~49.2 s → **~647 ms（-99%）** | **~3.14 GB → ~63.6 MB（-98%）** | **~3.15 GB → ~75.3 MB（-98%）** | +16,008 |

**达标判定（目标：10k/100k 展开首屏 <1 s / JSHeap 增量 <100 MB / 驻留 <150 MB）**：
全档达标。10k 与 100k 档 after 收敛至同量级（~0.5-0.65 s / ~64 MB）——渲染量与
规模解耦的直接证据；1k 档不触发截断（<2000），before/after 同量级（287 ms 属
机器噪声带 ±7% 内）为无回归旁证。

**行为变化**：超 2000 项的层仅显示前 2000 项 + 哨兵行；文件定位走既有
CommandPalette（Ctrl+P）文件名搜索（SearchFiles 模糊匹配），目标位于截断层
2000 项之外时树中无节点可定位，locateNode 显式 ElMessage 提示（替代静默失败）。
SearchFiles 跳过 node_modules 的既有语义使「扁平 node_modules 内找包文件」经
Ctrl+P 不可达——该盲区已由树内按名筛选补齐（09-27-tree-dir-name-filter 任务）：
工具栏筛选框输入后该层切换为命中项覆盖视图（GetFileTree 全量拉取走 Go 侧缓存 +
前端 `utils/treeFilter.js` 按名过滤，不受截断限制），截断哨兵点击聚焦筛选框，
清除/ESC/切换目录/refreshNode 退出筛选态。覆盖层与 el-tree 的互斥用 v-show 保持
树挂载（v-if 卸载会丢子树展开态，且重挂载后 locateNode 首段 getNode 必 miss、
误导提示「未能定位该文件」）；筛选作用域取筛选前最后点击的目录节点（treeKey
变化时随筛选态一并清空，防跨目录作用域泄漏）。

## 16. 维度 6：前端内存驻留面量化（09-27 驻留任务，收口路线图「及时释放不再使用的对象」）

> 背景：§15 收口渲染面（峰值增量）后，路线图「应用性能 → 内存使用优化」仅剩
> 「及时释放不再使用的对象」未测待定。本节量化「操作结束后不再使用的数据是否
> 归还」——驻留残留（与 §15 峰值互补），按数据分流实施/不动收口。

### 16.1 测量方法

| 项 | 说明 |
|---|---|
| 脚本 | `node scripts/frontend-memory-retention.mjs [--runs 3] [--commits 100] [--diff-lines 4000]` |
| 口径 | 同 §15.1：每轮独立 context 冷缓存、采样前 `HeapProfiler.collectGarbage` 强制 GC、CDP `JSHeapUsedSize`、3 轮中位；「操作前基线 → 操作后 → 释放动作后」三点驻留差 |
| fixture | 隔离工作区内 git 双仓：repoA（100 空提交 + 8000 行大文本改 4000 行工作区 diff）+ repoB（1 提交切换归零目标）；git 全命令显式 `-c core.autocrlf=false / user.name / user.email` 不依赖环境全局配置 |
| 面一链路 | 选仓库 A → 提交历史 tab（PAGE_SIZE=20 点「加载更多」补足 100 条 `.commit-card`）→ 切回 B（watch 重置）→ 残留 = 切走后 - 加载后 |
| 面二链路 | 选仓库 A → 本地变动 tab → dblclick 行打开 FileDiffDialog（单文件双栏 8000 `.diff-line`）→ 关闭 → 残留 = 关闭后 − **dblclick 前基线**（重选仓库后 CommitHistory 重载 20 条 + 本地变动表格的驻留在基线采样前已发生，不混入残留口径） |
| 未脚本实测面 | AiFunctionPanel tasks / aiChat 消息 / ContentPanel 预览依赖 claude CLI 真实任务驱动或属使用中数据，走代码审计定性（16.3 表） |

### 16.2 实测驻留数据（2026-09-27，GC 口径 3 轮中位，修正基线采样后终版口径）

| 面 | 操作后驻留 | 释放动作后残留 | 结论 |
|---|---|---|---|
| CommitHistory 100 条历史 | +15.2 MB | 切走后 **-2.5 MB**（回落） | watch 重置生效，无驻留 |
| FileDiffDialog 4000 行 diff | +15.9 MB | 关闭后 **+2.8 MB → 实施后 +0.3 MB** | 行对象无清理驻留，已实施关闭释放 |

实施 before/after（FileDiffDialog 关闭释放：行数据清空挂 el-dialog `@closed`
（destroy-on-close 渐隐期间 slot 未销毁，清空挂 watch 会致动画期间闪空态）；
`loadSeq++` + loading 复位挂关闭即时（防渐隐期间在途响应写回））：关闭残留
2.8 → 0.3 MB（≈0，噪声带内），行对象释放达标。
> 口径备注：首批测量曾以「切走仓库后」作面二基线，CommitHistory 重载与本地变动
> 表格驻留混入读数（6.4/4.1 MB 假残留），审核修正为 dblclick 前采样后重测，
> 本表为终版口径。

### 16.3 全驻留面分流表（2 面实测 + 4 面审计定性）

| 面 | 增长操作 | 定性 | 分流 |
|---|---|---|---|
| CommitHistory `commits` | loadMore 只增不减（千条级） | 单仓库内属**使用中数据**（用户在浏览）；切仓库 watch 重置实测回落 -2.5 MB | **不动**（既有机制健全，16.2 实测证据） |
| FileDiffDialog `left/right/fileGroups` | 打开大 diff | 关闭后**不再使用仍驻留**（watch 仅 visible=true 分支加载，无清理路径） | **已实施**：行数据清空挂 `@closed` + 关闭即时在途响应丢弃（16.2） |
| AiFunctionPanel `tasks` | AI 任务每轮 push 无上限 | 单条 256KB 截断 output 已限量级；关 Tab `RemoveAiTask` 释放点已有；列表属使用中数据 | **不动**（审计定性） |
| aiChat `chatMessages` / `answeredChatQuestions` | 会话消息累积 | 当前会话消息使用中（切会话替换）；answered 单调增长但量小（问答对文本） | **不动**（审计定性） |
| ContentPanel `filePreview.base64` / xlsxSheets | 大文件预览 | 当前份使用中；切换即替换（§15.3 曲线二同口径：48 MB 档峰值 64 MB 属使用中峰值非驻留） | **不动**（使用中数据） |
| EventsOn 注销完整性 | 组件挂/卸 | 3 处 `EventsOff` 全局清偏差（AiTaskHistoryPanel / ContentPanel pull 事件 / UpdateDialog），均唯一监听者：当前零误伤、无泄漏 | **不动**（规范债记录：多监听者场景出现时须按 cross-layer-contracts 改闭包注销） |

### 16.4 路线图勾选状态（「应用性能 → 内存使用优化」收口）

- **及时释放不再使用的对象：已测收口（2026-09-27，本节）**。唯一「不再使用仍驻留」
  实例（FileDiffDialog 关闭残留）已实施修复（2.8 → 0.3 MB，行对象归零）；
  其余各面为使用中数据或既有释放机制健全（审计 + 实测双证据）。
- 三子项全收口：限制文件树节点数量（§15.6 截断实施）+ 大文件分块读取（§15.3
  非瓶颈不动）+ 及时释放对象（本节）→ 父项「内存使用优化」勾选。
