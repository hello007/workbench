# 素材拆分 replace 索引规模基线实测

## 目标

按 [perf-baseline.md §19](../../../docs/spec/perf-baseline.md) 登记的复测口径，实测「素材拆分 replace 落点」导致的索引规模放大，产出可回填文档的量化数据，替换 §19 中「未实测前不写死数值」的占位表述。

## 需求

- 用程序化索引语料（批量 SQL 直写主表 + FTS 双写，`splitFtsText` 同源切分）构造两档形态：
  - **before**：53 文件 / 1208 页（用户真实素材库规模，来源 `10-30-hd` 任务基线）；
  - **after**：1208 文件 / 1208 页（全量 replace 后每页一个单页 pptx）。
- 测量 §19 三项指标 + 统计耗时：`files` 行数、`ListFiles`（前端 `ListAssetFiles`）全量拉取、`SearchAssets` 典型词（正文词「城商行」/ 标题词「获客」）、`GetLibraryStats`。
- 取数口径：单次 3 轮中位，重复 5 次取中位（首轮进程冷启离群值弃用）。
- 产物：永久基准测试 + 采集脚本登记 + 文档回填。

## 验收标准

- [x] 新增 `service/assets_replace_scale_bench_test.go`：`TestAssetReplaceScale_Report`（报告）+ `BenchmarkAssetReplaceScale`（回归基准）
- [x] `-short` 跳过报告测试；`go test ./service/ -short` 全绿
- [x] before/after 两档三项指标实测数据，3 轮中位 × 5 次取中位
- [x] 数据回填 `docs/spec/perf-baseline.md` §19.1，删除「未实测前不写死数值」占位
- [x] `scripts/perf-baseline.sh` 增加维度 6 采集命令
- [x] `docs/spec/assets-management.md` §15.2 引用实测数值

## 实测结果（2026-10-31，Windows 11）

|形态|files 行|slides 行|FTS 行|ListFiles|检索（正文词）|检索（标题词）|GetLibraryStats|
|---|---|---|---|---|---|---|---|
|before（53 文件 / 1208 页）|53|1208|1261|108 µs|526 µs|650 µs|37 µs|
|after（1208 文件 / 1208 页）|1208|1208|2416|1.72 ms|541 µs|693 µs|0.30 ms|
|放大倍数|22.8×|1.0×|1.9×|16.0×|1.03×|1.07×|8.1×|

结论：全量替换后 DB 读取层仍为亚毫秒至毫秒级；关键词检索耗时基本持平（命中页数两档相同），`ListFiles` 与统计随 files 行线性增长。数字以 docs/spec/perf-baseline.md §19.1 为准（该表初版为同进程串测，后测档被放慢约 2 倍，已改为每档独立进程复测）。

## 决策（ADR-lite）

**背景**：§19 要求实测但不得把示例页数当基线；本机无 1208 页真实样本（现库最大 116 页），真实端到端需 PowerPoint COM 且不可确定性复现。

**决策**：采用程序化索引语料直接构造两档形态，只测索引读取链路（§19 登记口径本就是索引规模面）；不引入 COM 与真实 pptx 解析。测量代码长期保留为回归基准。

**后果**：数据不含真实拆分/解析耗时（本就不在 §19 范围）；语料为合成分布，命中率与真实库不完全一致，但两档同注入规则、命中页数相同，可比性成立。跨进程波动约 ±15%，Windows 逐次计时被时钟粒度量化为 1 ms 台阶，已改为 10 ms 窗口批量平均。

## 不在范围内

- 不优化 replace 链路或索引读取性能（本任务只测量）
- 不做真实 pptx + PowerPoint COM 端到端拆分计时
- 不引入额外裁剪或虚拟条目规避规模放大（§19 明确禁止）
- 不改生产代码行为

## 技术备注

- 相关方法：`AssetLibraryService.ListFiles`（assets_library.go:385）、`GetLibraryStats`（:357）、`SearchAssets`（assets_search.go:54）；底层 `listFileEntries` / `libraryStats`（assets_db.go:769/786）。
- 语料构造与命中注入复用 `assets_search_bench_test.go` 同款规则（`benchRandomText`、`rand.NewSource(42)`），库走真实建库迁移链路。
- 复现命令见 perf-baseline §19.1 与 `scripts/perf-baseline.sh` 维度 6。
