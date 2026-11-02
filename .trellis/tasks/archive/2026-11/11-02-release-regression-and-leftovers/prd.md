# 交付前回归与遗留收口（父任务）

## Goal

拆分/备份链路累计 20+ 提交后，先做一次完整回归验证，再处理已知遗留：偶发测试与产物指纹。

## 子任务与顺序

| 顺序 | 子任务 | 优先级 | 内容 |
|---|---|---|---|
| 1 | `11-02-release-regression-verify` | P0 | 后端全量 + 覆盖率、前端单测、E2E、`wails build`，只验证不改代码 |
| 2 | `11-02-chat-template-corrupt-flaky` | P1 | 修复 `TestChatService_TemplatesCorruptFallback` 跨秒偶发失败 |
| 3 | `11-02-artifact-page-fingerprint` | P2 | 为拆分产物增加页级指纹，彻底消除同主干同页序用户文件误删（需 brainstorm 选型） |

## 依赖与顺序

* 子任务 1 必须先跑，结果决定后续是否插入修复任务。
* 子任务 2 与 3 相互独立。
* 父任务只跟踪，不写代码。

## 统一约束

* 子任务 1 只验证、不改代码；失败时产出最小复现与修复任务建议。
* 子任务 2/3 走完整 trellis 流程（brainstorm → implement → check → spec → commit）。
* 每个子任务完成后归档；父任务在全部子任务完成后归档。
