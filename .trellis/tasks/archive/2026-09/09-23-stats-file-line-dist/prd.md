# 统计：按文件/目录行数分布

## Goal

统计页新增按文件/目录的行数分布视图，直接延伸 09-23-contributor-line-ranking 建立的 numstat 通道（贡献者行数排名任务的 Out of Scope 明确留白项）。

## Requirements

* **brainstorm 决策（已定，桥接未获应答按推荐项推进，均可低成本调整）**：
  * UI 形态 → **横向条形图**（对齐贡献者排名先例，复用 tooltip +新增/−删除 细分模式）
  * 聚合粒度 → **目录一级上卷 + Top N 文件双维度切换**（复用贡献者卡片维度切换器先例，一次覆盖模块分布与热点明细两类诉求）
  * rename 口径 → **归一 new 路径**（`old => new` 取 new；`{a => b}/x.go` 归一为 `b/x.go`；与「文件现在在哪」直觉一致）
* numstat 解析保留文件路径列（现 `CommitLineStat` 只取数值列，需扩展字段并评估缓存条目兼容/失效）
  * 缓存兼容评估结论：CommitLineStatsCache 为纯内存缓存不落盘，进程重启即重建，无跨版本兼容负担；但 `CommitLineStat` 新增切片字段后 `deepCopyLineStats` 浅拷贝须同步升级为深拷贝
* 后端按路径聚合接口（目录上卷：取路径前缀分级归并；二进制排除口径沿用）
* 前端统计页新增图表，时间档位与贡献者行数口径一致

## Acceptance Criteria

* [x] 后端路径聚合 + 单测（rename `old => new` 路径口径、二进制排除、目录上卷正确性）——commit_path_stats_test.go 6 组测试：归一 10 形态表驱动、Files 收集（rename/二进制/汇总一致性）、深拷贝隔离、目录桶、上卷聚合、文件 Top N、窗口过滤
* [x] 缓存结构变更兼容策略明确——纯内存缓存进程重启即重建无跨版本负担；`deepCopyLineStats` 已升级逐条复制 Files（测试锚定拷贝修改不污染缓存）
* [x] 前端图表 + 维度交互 + 组件测试——RepoStatsChart 第四卡「行数分布」+ 目录/文件切换器（stub 升级为值序列循环驱动两组独立切换）；builder 单测 6 例 + 组件测试 8 例，前端全量 1387 绿
* [x] `wails generate module` 同步、npm run build 通过——走 RepoStats 扩展字段方案零新增 App 方法，models.ts 手动补 PathLineStat 类与 RepoStats 两字段，build 通过（无 MISSING_EXPORT）
* [x] docs/功能说明.md、docs/路线图.md 同步

## Out of Scope

* 文件 hotspot 演变分析（历史趋势）；代码年龄分布；贡献者×文件交叉矩阵

## Technical Notes

* 基础：service/commit_line_stats.go 通道现成；rename 行路径列为 `old => new` 形式需口径定义（算 old、new 还是排除）
