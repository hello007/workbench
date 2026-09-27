# WorkBench 前端内存维度量化收口（perf-baseline 维度空白 + 路线图内存优化两子项数据决策）

## Goal

收口 perf-baseline 前端内存维度空白：现有基线全部是 Go 侧（MemStats §5 / benchmark §3 / soak §13 / 泄漏检测 §14），前端渲染面无任何内存数据。冷启动测量（§6，2026-09-27）已证明进程工作集大头在 WebView2 宿主而非 Go（常驻堆仅 0.6 MB），Go 侧内存优化无收益——真实内存风险在前端渲染：超大目录文件树节点量、大文件预览 base64 全量传输。

路线图「应用性能 → 内存使用优化」父项下「限制文件树节点数量」「大文件分块读取」两子项均无数据支撑，无法决策做不做。本任务按「先测后优」只测量收口：非瓶颈按 §9.4 先例记不动项；确属瓶颈产出带量化目标的优化立项依据（只记录不实施）。

## 用户给定

- 量化曲线一（文件树规模）：fixture 构造 1k / 10k / 100k 文件目录，量 Go 侧 GetTree 耗时/内存（benchmem 扩档）+ 前端渲染后 JSHeapUsedSize / Nodes 数（Playwright CDP），输出「规模-内存-耗时」表
- 量化曲线二（大文件预览）：1 / 4 / 16 / 64 MB 文本与 markdown，量 ReadFileBytes 传输体积（base64 膨胀系数实测）与前端 JSHeap 峰值，定位 TooLarge 阈值是否合理
- 数据决策：非瓶颈 → perf-baseline §9.4 不动项收口 + 路线图子项勾选注明；瓶颈 → 记录量化目标（如「100k 节点 JSHeap >X MB 须虚拟滚动」），不实施优化
- 数据回填 perf-baseline.md 新维度章节 + 路线图「内存使用优化」子项状态更新

## 仓库勘察（已核实）

- `service/fileoperation.go:78` `PreviewFile(filePath, maxSize)` 与 `:139` `ReadFileBytes(filePath, maxSize)` 均有 stat 超限拒绝（TooLarge 分支，:114/:153），无分块/offset 渐进加载能力
- `app_preview.go:12` PreviewFile maxSize = **1MB**（文本读全内容）；`:22` ReadFileBytes maxSize = **50MB**（图片/PDF/Office base64 放宽上限）——50MB 文件经 base64 膨胀 ~1.33x ≈ 66MB 字符串过 Wails 桥 + 前端 decode
- 前端 `FilePreviewRenderer.vue`：base64 → `decodeBase64Utf8`（utils/base64.js，禁裸 atob 契约见 cross-layer-contracts.md）→ 渲染；tooLarge 走降级提示（:793「文件过大，暂不支持内嵌预览」）
- Go 侧已有 `BenchmarkFileTreeGetTree_1000Files`（service/perf_bench_test.go，1000 文件档，fixture 构造可参考）；更大规模无数据
- 测量基建现成：`util/testutil`（RunGit/WriteFile/InitTempRepo，参数 testing.TB，fixture 必须复用）；soak 框架（§13）；`-leak-heap-mb` 阈值机制（§14）
- 前端内存采样：Playwright CDP `Performance.getMetrics`（JSHeapUsedSize / Documents / Nodes）——`scripts/cold-start-frontend.mjs` 已验证 `--serve` 无头 + 独立 context 冷缓存模式可复用（frontend/node_modules 有 playwright）
- `docs/spec/perf-baseline.md` §6 为最近收口先例（测量脚本 + 占位收口 + 路线图联动），本任务同构

## Assumptions (temporary)

- 前端 JSHeap 采样用独立 context 冷缓存（复用冷启动脚本口径），浏览器进程复用 V8 暖——与 §6 前端段口径一致
- Playwright CDP 大文件预览触发方式：浏览器通道（--serve）经 RPC 调 ReadFileBytes，与桌面同链路（前端代码路径一致，§6 已定性代理口径可行性）
- 100k 文件 fixture 构造耗时可能显著（NTFS 海量小文件）——构造放 fixture 阶段计时排除，或降为 50k 档（测量脚本内注明实际档位）
- base64 膨胀系数理论 4/3，实测验证即可不深究 Wails 桥序列化额外开销（量级参考）

## Constraints

- 只新增测量脚本 / benchmark 测试 / 文档，不改生产代码（与 09-27 冷启动任务同约束）
- 测量机本机 Windows 11，数据标注环境（gpuDisabled=true 沿用 §6 标注）
- fixture 大目录构造进 util/testutil 或测试内辅助，禁散落各 _test.go（perf-baseline §7 基准不变量）
- benchmark/_test 不入覆盖率门禁（test-coverage-gate.md）
- 禁 time.Sleep 等待 / 固定端口断言（test-stability.md）；测量脚本端口用独立 --listen（冷启动脚本先例）

## 验收

- perf-baseline.md 新维度章节：文件树「规模-内存-耗时」表 + 大文件「大小-传输-JSHeap」表 + 结论（各子项：不动项或带阈值的量化目标）
- 路线图「内存使用优化」父项下「限制文件树节点数量」「大文件分块读取」两子项有数据支撑的勾选/不动状态
- 测量脚本可重复执行，复测偏差量级有标注（§6 噪声说明同款）
- 工作区提交前过 trellis-check；子 agent 代码审核（冷启动任务先例）
