# CHANGELOG 1.9.0 补录 + CLAUDE.md 文档版本提升

## Goal

v1.9.0 已发版（tag v1.9.0，提交 cac805e），CHANGELOG.md 缺 [1.9.0] 条目、CLAUDE.md 文档版本停在 v2.16，按项目惯例补录对齐。

## Requirements

* CHANGELOG.md 补 `[1.9.0] - 2026-09-28` 条目：v1.8.0..v1.9.0 提交按 Added/Fixed/Changed 分类（沿用 1.8.0 条目格式），feat/test(perf)/fix 分流归并
* CLAUDE.md 底部「文档版本」v2.16 → v2.17，「最后更新」改 2026-09-28
* 零代码改动

## Acceptance Criteria

* [ ] CHANGELOG [1.9.0] 条目覆盖 v1.8.0..v1.9.0 全部用户可见变更（文件树大目录三连/三向合并/行数排名/诊断导出/性能基线/E2E 通道等）
* [ ] 分类与 1.8.0 条目格式一致（Added/Fixed/Changed）
* [ ] CLAUDE.md 版本行同步

## Out of Scope

* 路线图勾选（本轮无对应项）
* 已发版内容回改

## Technical Notes

* 范围 `git log v1.8.0..v1.9.0 --oneline`（50 提交），feat 6 个 / test(perf|e2e) 5 个 / fix 4 个，其余 chore/docs/test 归并或不列
