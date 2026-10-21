# E2E 覆盖缺口盘点与高价值链路补齐

> 父任务：10-21-hardening-batch（顺序 3，最后一批）

## Goal

补齐 E2E 独有暴露面的高价值缺口（跨组件事件链、近期重构不变式、mock 盲区），本批聚焦 Top 5 链路展开 ≈ 10-14 用例。

## 盘点结论（research/coverage-matrix.md，2026-10-21）

- 现状 17 spec / 72 用例；mock 表 176 方法有缺省、66 个绑定方法无缺省（终端 5 方法全在其中）
- 近 5 周改动频次 Top3：AssetsPanel 18、SettingsPanel 12、FileTreePanel 10
- 零覆盖域：工具箱 7 面板（Panel 化刚重构）、终端多 tab、购物栏事件链、设置保存链

## 本批范围（Top 5 链路）

| # | 链路 | 核心断言面 | mock 前置 |
|---|---|---|---|
| 1 | 命令面板搜索超时批量重试编排 | searchVersion 版本守卫丢弃、信号量 3 限流、banner 进度三态、worker 版本失效提前 return | mock 缺省现成（ContentSearchDirs pending resolver 形态） |
| 2 | 终端多 tab 生命周期 | 关闭非活动 tab（getter 投影路径）、快照归一化、冷启动判据 | 先补 CreateTerminal 系 5 方法 mock 缺省 |
| 3 | 工具箱 Panel 化主链 | 全窗口面板切换互斥（三栏隐藏）、ProcessManagerPanel 查杀链（确认交互 + 行级防重） | mock 形状已定型 |
| 4 | 素材购物栏收集-排序-合并导出 | assets-cart:* 三事件跨组件广播、stale 条目 SkippedItems 报告、溯源跳转 | 缺省补齐（MergePptxCart 系） |
| 5 | 设置面板保存整体提交链 | 失败基底 null 禁保存不变式、保存后热更生效（终端字号等） | GetSettings/SaveSettings 缺省核对 |

## Requirements

1. **mock 前置补课**：终端 5 方法 + 购物栏/设置缺省方法补入 wails-mock-defaults.js（单一数据源）
2. **新 spec 落地**：每链路独立 spec 文件（toolbox-panel.spec.js / terminal-tabs.spec.js / command-palette-retry.spec.js / assets-cart.spec.js / settings-save.spec.js，命名对齐既有 kebab-case），用例展开 2-3 条/链路
3. **契约遵守**：fixtures.js import、禁 waitForTimeout（chan 守护/expect 自动重试）、ElMessageBox 范式（confirm-dialog.md §4）、pending resolver 形态（参考 logging-and-errors.md 批量重试 mock 形态）
4. **vitest.config.js exclude 核对**：新 spec 均在 e2e/ 目录（既有排除面覆盖），确认无新通道目录则零改动
5. **文档同步**：测试策略.md E2E 覆盖清单更新；e2e-testing.md 按实际沉淀（若有新范式——如 pending resolver 通用化）

## Acceptance Criteria

- [ ] 5 个新 spec 落地，10-14 用例全绿（本地 + 口径同 CI）
- [ ] vitest 全量零回归（mock-defaults 变更不破坏既有 spec）
- [ ] E2E 全量（含新用例）零回归
- [ ] 测试策略.md 更新；e2e-testing.md 按需沉淀
- [ ] research/coverage-matrix.md 候选清单标注本批落地/留后续状态

## Definition of Done

- AC 全勾；CI 全 job 绿（push 后）

## Decision (ADR-lite)

**Context**：缺口矩阵 Top 20 候选，PRD 约束 ≤15 用例/批；Top 5 链路覆盖全部高风险档（重构不变式 + 事件链 + 高频改动面）且 mock 复杂度已知（仅终端域需前置补课）。
**Decision**：本批 = Top 5 链路展开；矩阵其余候选（素材 facet/评分/终端 z-index 层叠/看板等）留后续批次，候选清单标注状态。
**Consequences**：单批体量可控（约 5 spec）；素材域与工具箱剩余面仍有缺口（矩阵已留痕，按需启动后续批）。

## Out of Scope

- 矩阵候选中非 Top 5 链路（留后续批）
- desktop 通道扩容（现 3 用例，非 Windows 自动 skip，另批评估）
- 后端集成测试扩展

## Research References

- [research/coverage-matrix.md](research/coverage-matrix.md) — 五域缺口矩阵 + mock 可行性 + Top 20 候选

## Technical Notes

- 终端 5 方法 mock 缺省形态参考既有 TerminalPanel spec（若有 jsdom 面单测）与 wails-mock-defaults.js 结构
- 命令面板重试编排的 pending resolver mock 形态在 logging-and-errors.md 批量重试节有现成记录（前 3 次 pending 第 4+ 次 resolve）
- 购物栏事件链断言参考 ai-task:* 共听注销范式（cross-layer-contracts.md EventsOn 闭包规则）
