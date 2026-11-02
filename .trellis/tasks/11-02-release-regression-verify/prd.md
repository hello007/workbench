# 交付前完整回归验证（P0，只验证不改代码）

## Goal

本轮累计 20+ 提交涉及拆分主链路、清理判定、重链探测与备份还原，单测虽全绿，但未跑 E2E 与真实构建。需在交付前确认整体可构建、可运行、E2E 通过。

## Requirements（只验证，禁改代码）

1. `go test ./...` 与 `bash scripts/coverage-check.sh`。
2. `cd frontend && npm test`。
3. `cd frontend && npm run e2e`（首次先 `npm run e2e:install`）。
4. `wails build`，确认产物生成。
5. 汇总：每项结果、失败命令原文末 20 行、耗时。
6. 真机冒烟建议清单（不强制执行）：拆分 source/replace/cache 三落点、改名重链后重拆、落点切换、备份/还原（含版本过新与空库拒绝）、删除库。

## Acceptance Criteria

* [ ] 五项命令均执行并记录实际结果（失败也如实记录）。
* [ ] 失败项给出最小复现与「是否与本轮改动相关」的判断。
* [ ] 若需修复，产出建议的子任务拆分，不在本任务内改代码。

## Out of Scope

* 不修改任何代码、配置、测试或文档，仅产出验证报告。
