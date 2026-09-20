# 优化提交历史加载慢（go-git 采集层换 CLI git log 批量）

## Goal

修复提交历史加载耗时过长：根因为采集层每条提交经 go-git 做树 diff（上万次对象库读），冷启动全量扫与大仓库（>5000 提交）翻页均慢。将采集层换为 CLI `git log` 批量子进程（快 1-2 个数量级），缓存/增量/过滤/分页语义零变化。

## Requirements

1. **采集层替换**（app_git.go）：
   * `fullScanCommits`（≤5000 快照路径）改用 CLI `git log -n <cap> --format=... --name-only` 一次子进程取全量；
   * `fetchCommitHistoryFromGit`（overflow 路径）改 CLI 流式扫描 + Go 侧内存过滤分页，凑够 limit 即停（stdout pipe 提前关闭，不读全量）；
   * `incrementalCommits`（增量 prepend）改 CLI 流式从新 HEAD 迭代到 SHA 交集停；
   * 输出解析用 `%x00` 记录分隔 + `%x01` 字段分隔，鲁棒处理消息换行/特殊字符；
   * `getCommitFiles` 的 go-git 树 diff 在历史主路径不再逐条调用（CLI `--name-only` 批量给出；`getCommitFiles` 保留供其他调用方）。
2. **语义零漂移**：Since/Until/Author/Keyword/FilePath 过滤仍走现有内存过滤（`filterCommits` / 流式同逻辑），**不下推 CLI 过滤 flags**（pathspec 子串与 `--author` 正则语义均与现内存子串匹配不等价）；返回字段与顺序（CommitterTime 序）不变。
3. **缓存层不动**：CommitHistoryCache 的 key/TTL/SHA 链增量判定/失效入口（InvalidateCommitHistoryCache）全保留；cap 5000 与 overflow 不缓存策略不变。
4. **先测后优（perf-baseline 硬约束）**：新增冷扫 benchmark（真实 git fixture，量化 go-git before）→ 改后复测 CLI after，数据写入 `docs/spec/perf-baseline.md`（新增提交历史维度）。
5. 测试全绿：现有 GetCommitHistory 单测/过滤组合测试基于真实 git fixture，行为不变应全过；`go test -tags=integration ./...` 绿。

## Acceptance Criteria

* [ ] 冷扫 benchmark before/after 量化数据落 perf-baseline.md，提升 ≥5×（预期 1-2 数量级）。
* [ ] 现有提交历史单测/集成测试全绿（行为兼容验证）。
* [ ] 过滤语义回归：Keyword/Author/Since/Until/FilePath/组合过滤结果与改前一致（现有测试覆盖）。
* [ ] 大仓库（>5000，可用 fixture 构造或集成测试模拟）翻页正常，流式提前终止生效（不读全量）。
* [ ] 消息含换行/`%x01`/中文的提交解析正确（解析单测）。
* [ ] `go test ./...`、`go test -tags=integration ./...`、前端 `npm test` 全绿；service 覆盖率 ≥76%。

## Definition of Done

* perf-baseline.md 补提交历史维度 before/after 表。
* 前端零改动（绑定签名不变）。

## Decision (ADR-lite)

**Context**: 加载慢根因 = go-git 逐条树 diff（对象库读放大），go-git 纯 Go 对象库在 Windows packfile 上慢。
**Decision**: 采集层换 CLI git log 批量/流式（用户确认方案 1）；过滤不下推 CLI flags 保语义零漂移；缓存策略与 cap 不动。
**Consequences**: 新增 git CLI 子进程依赖（workbench 既有依赖，Push/Pull/Fetch 已用，零新增）；超大仓库 overflow 流式不缓存维持现状；CLI 输出解析需鲁棒单测。

## Out of Scope

* cap 5000 上限调整；overflow 仓库缓存化；前端懒加载 Files（方案 3 被否）；CommitHistoryCache 结构改动。

## Technical Notes

* 热路径实证：app_git.go:523 `getCommitFiles` go-git `Patch` 每条 ≥2 树对象读；app_git.go:335 `fullScanCommits` × 5000 上限；app_git.go:433 overflow 每次 go-git Log 全迭代。
* CLI 参考：`git log -n 5000 --date-order --format=%x00%H%x01%h%x01%an%x01%ae%x01%at%x01%s%x01%b%x01 --name-only`（root commit 无 parent 文件列表走全树，`--name-only` 对 root 列全文件，与现 `getTreeFiles` 上限 100 语义差异需处理——root commit 文件列表语义见 app_git.go:558）。
* CommitterTime 序：CLI 默认 reverse time 即 committer date 序，与 `git.LogOrderCommitterTime` 对齐（验证单测覆盖时间乱序提交场景）。
* benchmark 对齐 perf-baseline.md 第 7 节规范（util/testutil fixture、b.StopTimer/StartTimer）。
