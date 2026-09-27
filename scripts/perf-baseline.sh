#!/usr/bin/env bash
# WorkBench 性能基线采集脚本（v1.4 PR1 子项 3）
#
# 采集 4 维度性能数据，写入控制台日志，供 docs/spec/perf-baseline.md 记录与 PR4 优化对比。
# 本脚本只测量不改生产代码；benchmark 与 MemStats 测试定义于：
#   service/perf_bench_test.go  - BenchmarkFileTreeGetTree_1000Files / _Cached / BenchmarkScanGitRepos_10Repos
#   perf_bench_test.go          - BenchmarkNewAppServices / TestPerfMemStats
#
# 运行：bash scripts/perf-baseline.sh
# 依赖：git 在 PATH（benchmark fixture 构造 git 仓库）；Go 工具链。
# 注意：主包 BenchmarkNewAppServices 每次迭代重新初始化全局 logger（lumberjack 句柄累积
# artifact），2s 迭代次数有限可接受；如需更精确可改 -benchtime=500x。

set -u

cd "$(dirname "$0")/.."

echo "=== WorkBench 性能基线采集 ==="
echo "时间: $(date)"
echo "Go 版本: $(go version)"
echo

echo "--- 维度 1a: service benchmark（FileTree / ScanGitRepos）---"
go test -bench=. -benchmem -benchtime=2s ./service/
echo

echo "--- 维度 1b: 主包 benchmark（NewAppServices）---"
# 主包 benchmark 句柄 artifact，用 500x 固定迭代控制句柄累积，ns/op 仍稳定
go test -bench=BenchmarkNewAppServices -benchmem -benchtime=500x ./
echo

echo "--- 维度 1c: 主包 benchmark（提交历史冷扫）---"
# fixture 构造 300 提交仓库（每次 benchmark 调用重建，构造耗时段 StopTimer 排除）
go test -bench=BenchmarkGetCommitHistory_ColdScan -benchmem -benchtime=2s -run=^$ ./
echo

echo "--- 维度 3: Go 运行时内存 MemStats 快照 ---"
go test -run TestPerfMemStats -v ./
echo

echo "--- 维度 2: 前端 bundle 体积（需另行手动执行，见 docs/spec/perf-baseline.md）---"
echo "  cd frontend && npm run build     # 或 npx vite build（wailsjs 缺失时）"
echo "  du -sh frontend/dist && ls -lh frontend/dist/assets"
echo

echo "--- 维度 4: GUI 冷启动（需 wails build 产物，Windows 本机，见 docs/spec/perf-baseline.md §6）---"
echo "  powershell -ExecutionPolicy Bypass -File scripts/cold-start-bench.ps1 -Runs 5      # GUI 冷启动（需先关闭常驻 workbench.exe）"
echo "  node scripts/cold-start-frontend.mjs --runs 5                                      # 前端首屏（浏览器通道代理，--serve 无头）"
echo

echo "=== 采集结束 ==="
echo "将上述数据填入 docs/spec/perf-baseline.md 对应基线表。"
