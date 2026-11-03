# E2E CI 失败修复（上游 merge UI 变更未同步测试）

## Goal

修复 CI `test` job 的 E2E 失败：上游 merge `31b9de7` 引入「分支对比」面板与冲突解决弹窗等 UI 变更，但 E2E 仍用硬编码活动栏索引与旧冲突面板断言。目标：E2E 全绿、CI test job 恢复，并消除同类脆性选择器。

## What I already know

- CI `test` 失败步骤为「E2E 测试」；上游 merge 提交 `31b9de7` 的 CI 同样失败，非本仓库文档提交引入。
- 本地复现（Windows，先 `wails generate module`）：15 failed / 111 passed。
- 主因 1：`frontend/src/components/ActivityBar.vue` panels 由 7 项增至 8 项（新增 `branchcompare`「分支对比」，位于 index 7），settings/terminal 顺移到 8/9；E2E 多处使用 `.activity-bar-item` 硬编码 `nth(7)`（settings）、`nth(8)`（terminal）。
- 主因 2：`GitMerge.vue` 上游改动新增冲突解决弹窗（`ConflictResolverDialog.vue`）与额外 `GetConflictState` 调用；`merge.spec.js` 仍断言内联「冲突未解决」面板且 `GetConflictState` 恰好 2 次（实际 4 次）。
- 主因 3：`assets-filter-sort.spec.js` 1 例超时，需按新筛选 UI 核对断言。
- 相关规则：`docs/spec/e2e-testing.md`（fixture、mock 单一数据源、禁 waitForTimeout）、`docs/spec/test-stability.md`。

## Requirements

- R1 活动栏选择器语义化：给 `ActivityBar.vue` 各入口加稳定 `data-*` 属性（如 `data-panel-id` / `data-action="settings|terminal|devices"`），E2E 全部改为语义定位，**禁再使用 `.activity-bar-item` 硬编码 nth**（含负数 nth）。
- R2 更新失效用例：
  - 活动栏索引导致的 12 例（audit-history、settings-save、settings-token、terminal-tabs 等）改语义选择器后应恢复；不得通过调整数字索引敷衍。
  - `merge.spec.js`：按新冲突解决弹窗 UI 更新断言与 `GetConflictState` 调用次数（以组件实际行为为准；若发现产品行为回归则记录并修复产品）。
  - `assets-filter-sort.spec.js`：定位超时原因（新筛选面板/参数换算），按实际契约更新断言。
- R3 全量本地 E2E 通过：`cd frontend && npx playwright test`（0 failed）；必要时用 `--repeat-each=2` 验证稳定性。
- R4 不改产品行为，除非确认是回归（需在报告中举证）。
- R5 文档：在 `docs/spec/e2e-testing.md` 增补「活动栏语义选择器」约定；若更新了组件选择器约定，同步。

## Acceptance Criteria

- [ ] 全仓 E2E 无 `.activity-bar-item` 硬编码 nth（`grep` 为空；`nth(-2)` 等负数同禁）。
- [ ] `cd frontend && npx playwright test` 全绿（0 failed）。
- [ ] 关键失败 spec（assets-split、audit-history、settings-save、settings-token、terminal-tabs、merge、assets-filter-sort）单独复跑通过。
- [ ] 无 `waitForTimeout` 新增；mock 仍走 `wails-mock-defaults.js` 单一数据源。
- [ ] 测试文件改动说明中区分「测试适配」与「产品修复」。
- [ ] `npm test -- --run` 前端单测仍全绿（选择器/组件改动可能影响组件测试）。

## Definition of Done

- E2E 本地全绿；组件单测全绿。
- 文档补充语义选择器约定。
- 提交、归档；推送后 CI test job 转绿（推送由用户确认）。

## Decision (ADR-lite)

**Context**：新增导航项导致索引型选择器集体失效；继续用 nth 只会在下次 UI 扩展时复发。

**Decision**：一次性改为数据属性语义定位；冲突弹窗按新 UI 更新断言；不调整数字索引。

**Consequences**：测试改动面较大（~12 个 spec 的入口函数），但后续新增面板不再破坏 E2E；组件需保持 `data-*` 属性稳定。

## Out of Scope

- 重构全部 E2E 选择器（仅活动栏与本次失败相关）。
- 修改 CI 配置或重试策略。
- 处理与本失败无关的 flaky 风险。

## Technical Notes

- 复现前置：`wails generate module`（`frontend/wailsjs` 不入库）。
- 本地命令：`cd frontend && npx playwright test --reporter=line`。
- 关键文件：`frontend/src/components/ActivityBar.vue`、`frontend/e2e/*.spec.js`、`frontend/src/components/GitMerge.vue`、`frontend/src/components/ConflictResolverDialog.vue`、`frontend/src/components/AssetsPanel.vue`。
- 上游 merge：`31b9de7`（含分支对比面板、冲突弹窗、AiChat 等）。
