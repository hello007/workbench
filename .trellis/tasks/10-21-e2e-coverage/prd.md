# E2E 覆盖缺口盘点与高价值链路补齐

> 父任务：10-21-hardening-batch（顺序 3，在 tilde-upgrade 归档后启动——用例断言基于升级后版本行为写入，避免返工）

## Goal

web 通道 E2E 现 72 用例（六关键流程 + assets 域扩展），desktop 通道 3 用例。路线图留白「全量 E2E 覆盖留作后续按需增强」。vitest 已 2022 用例但 mock 面屏蔽 EP 运行时行为，跨组件编排链路（多面板联动、事件广播、快捷键冲突、z-index 层叠）是 E2E 独有暴露面。本任务盘点缺口并补齐高价值链路。

## Requirements

1. **盘点**：按功能域（工具箱 7 面板 / 素材管理 / 终端多 tab / AI 对话 / 导航中心）对照 E2E 现有用例，输出覆盖缺口矩阵（research 持久化），标注风险档（近期改动频繁域 / 跨组件事件链 / mock 盲区优先）
2. **高价值链路补齐**：按缺口矩阵选 Top N（建议 ≤15 用例/批，批次 brainstorm 定），守 e2e-testing.md 全部契约：
   - fixtures.js import `{test,expect}`（禁直接 import `@playwright/test`）
   - 禁 `waitForTimeout`，靠 expect 自动重试
   - mock 返回值改 `src/test/wails-mock-defaults.js` 单一数据源
   - 新增 E2E 通道目录须同步 `frontend/vitest.config.js` 的 `test.exclude`（e2e-desktop/** 先例）
   - ElMessageBox 递归/两段链断言按 confirm-dialog.md §4 范式（`.first()`/`.last()` 或 `:visible`+filter）
3. **文档同步**：测试策略.md 与 e2e-testing.md 按实际沉淀更新

## Acceptance Criteria

- [ ] 覆盖缺口矩阵落 research（五功能域全覆盖）
- [ ] 新增用例本地 + CI 全绿
- [ ] vitest 全量零回归（exclude 变更不误伤）
- [ ] 用例遵守 e2e-testing.md 全部契约（check 阶段逐条核对）
- [ ] 文档按实际沉淀更新

## Definition of Done

- AC 全勾；CI 全 job 绿；文档更新

## Open Questions（brainstorm 待定）

- 本批 Top N 用例清单（缺口矩阵出来后定）
- desktop 通道是否纳入补齐范围（现 3 用例，非 Windows 自动 skip）

## Out of Scope

- 视频教程、多语言（路线图暂缓项）
- 后端集成测试扩展（`//go:build integration` 面已覆盖六关键流程）

## Technical Notes

- 哨兵 5 spec 清单（ElMessageBox 消费）见 frontend-dep-upgrade.md §3.3
- EP 升级排查方法论（confirm-dialog.md §1）是 mock 盲区分析的现成框架
