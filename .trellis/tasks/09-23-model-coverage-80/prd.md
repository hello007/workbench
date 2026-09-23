# model 覆盖率补测至 80% 门禁

## Goal

model 包覆盖率实测 76.5% < 80% 门禁，缺口来自 `model/settings.go` 的 `EnsureWebServeDefaults`/`EnsureTerminalDefaults`（PR5 web serve 与终端外观任务引入时未补测）。补表驱动单测回绿。

## Requirements

* 补 `EnsureWebServeDefaults` 单测：默认值填充、已设合法值不覆盖、空串/零值字段补默认
* 补 `EnsureTerminalDefaults` 单测：同上口径
* 表驱动风格对齐 model 包现有测试（`model/*_test.go` 先例）

## Acceptance Criteria

* [ ] `go test ./model/ -cover` ≥ 80%
* [ ] `scripts/coverage-check.sh` 全门禁 PASS（model ≥80%）
* [ ] `go test ./...` 全绿
* [ ] 无凑覆盖率的无断言测试（断言具体行为）

## Out of Scope

* 其他包覆盖率调整；settings 功能行为变更

## Technical Notes

* 债来源：2026-09-23 contributor-line-ranking 任务 check 阶段定性「既有失败、非本次引入」，当时按范围约定未处理，本任务专门清偿
