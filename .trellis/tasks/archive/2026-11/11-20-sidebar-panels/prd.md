# 崩溃恢复面板白名单与 SIDEBAR_PANELS 同源

## Goal

消除崩溃恢复面板白名单的二次登记漂移面：`useSessionState.js` 的 `VALID_PANELS` 现硬编码前 7 个面板 id，缺 `branchcompare` / `todo` / `prompts`，导致这三类面板在崩溃恢复后被判非法并回退 `directory`（用户无提示落到错误面板）。改为从 `frontend/src/store/sidebarPanels.js` 的 `SIDEBAR_PANELS` 派生，使新增面板只需维护单一真相源。

## What I already know

* `frontend/src/composables/useSessionState.js:24` 定义 `VALID_PANELS = new Set(['directory', 'ai', 'ai-chat', 'toolbox', 'assets', 'dashboard', 'stats'])`，`applySessionState` 第 73 行用其判定 `state.activePanel`，非法则忽略（保持 store 默认 `directory`）。
* 同源真相源已存在：`frontend/src/store/sidebarPanels.js` 的 `SIDEBAR_PANELS`（10 项，含 branchcompare/todo/prompts），模块头注释明确要求**零依赖**（E2E 与 `scripts/check-e2e-sidebar-panels.mjs` 直接 import，不得间接引入 vue / pinia / wailsjs 生成产物）。
* `frontend/src/store/settings.js:36` 已有一份等价派生 `const SIDEBAR_PANEL_IDS = new Set(SIDEBAR_PANELS.map(p => p.id))`，用于 `normalizeSidebarOrder` 剔除未知 id。
* 面板集合恒为 `SIDEBAR_PANELS` 全集：设置区只允许调整顺序（`sidebarOrder`），无删除面板能力；因此白名单与用户排序无关，集合语义恒定。
* 现有单测 `frontend/src/composables/__tests__/useSessionState.spec.js` 已有负用例「非法 activePanel 被忽略（保持默认 directory）」及 ai-chat / dashboard / assets 正用例，但无 branchcompare / todo / prompts 用例。

## Requirements

* `useSessionState.js` 的合法面板集合改为从 `SIDEBAR_PANELS` 派生，禁止再硬编码 id 字面量清单。
* 引入方式不得破坏 `sidebarPanels.js` 的零依赖约束（node 侧可直接 import）。
* 判定行为不变：`state.activePanel` 非空且属合法集合才写回；非法值/空值保持 `directory` 默认；`touched` 返回值语义不变。
* 单测覆盖：新面板（branchcompare/todo/prompts）可恢复；未知 id（不在 SIDEBAR_PANELS 内）仍回退 directory；负用例断言原样保留。
* 用「遍历 SIDEBAR_PANELS 全量 id」的参数化正例，证明新增面板无需改测试/白名单，即解耦成立。

## Acceptance Criteria

* [x] `activePanel` 为 `branchcompare` / `todo` / `prompts` 时，`applySessionState` 恢复后 `uiStore.activePanel` 保持该值且 `touched === true`
* [x] 未知面板 id（如 `invalid-panel`、非 SIDEBAR_PANELS 的自定义 id）仍回退 `directory`，`touched === false`，原负用例断言不变
* [x] 正例参数化遍历 `SIDEBAR_PANELS` 全量 id（新增面板自动被覆盖，无需改测试）
* [x] `useSessionState.js` 内不再出现面板 id 字面量清单
* [x] `npm test` 全绿，前端覆盖率四指标仍 ≥70%（实测 87.17 / 81.04 / 85.38 / 89.82，2935 用例通过）
* [x] 结论：**是，已沉淀**——`docs/spec/cross-layer-contracts.md` 会话快照 §3 Contracts 新增白名单派生契约与历史缺口说明；AGENTS「关键规则」侧边栏面板行内补钩子「崩溃恢复白名单（`useSessionState`）自 `SIDEBAR_PANELS` 派生禁硬编码」（行 302 字符，钩子密集行 ≤350 合规）

## Definition of Done

* 单测新增/调整并通过；`npm test` 全绿
* 无降级检查、无放宽既有断言
* 如采纳沉淀，AGENTS 关键规则行同步更新（含字符数上限校验）

## Technical Approach

* `useSessionState.js` 导入零依赖的 `SIDEBAR_PANELS`，本地派生 `VALID_PANELS = new Set(SIDEBAR_PANELS.map(p => p.id))`，替换现有硬编码字面量清单。
* 判定逻辑（第 73 行）保持不变：非空且属集合才写回，非法值/空值保持默认 `directory`。
* 单测策略：
  * 参数化遍历 `SIDEBAR_PANELS` 全量 id 断言可恢复（新增面板自动覆盖，证明解耦）；
  * 单独显式覆盖 `branchcompare` / `todo` / `prompts` 三个本次修复目标（回归锚点）；
  * 显式负例：`invalid-panel` 与一个不在 `SIDEBAR_PANELS` 内的自定义 id 均回退 `directory` 且 `touched === false`。

## Decision (ADR-lite)

**Context**：面板白名单存在两处派生——`useSessionState.js` 硬编码（已漂移，缺 branchcompare/todo/prompts），`settings.js:36` 从 `SIDEBAR_PANELS` 派生。两者本可同源却各写一份。
**Decision**：在 `useSessionState.js` 本地从 `SIDEBAR_PANELS` 派生集合（改动面最小、彻底消除硬编码漂移）；不在 `sidebarPanels.js` 新增导出、不改 `settings.js`。`settings.js` 既有派生同为 `SIDEBAR_PANELS` 派生，不会漂移，避免扩大改动面。
**Consequences**：两份派生并存（均自同一真相源，无漂移风险）；若未来出现「面板可被用户禁用/隐藏」需求，需再收敛为 `sidebarPanels.js` 单点导出。当前无此需求，遵循 YAGNI。

## Out of Scope

* 应用设置持久化段（`AppSettings`）字段变更
* Home 面板渲染逻辑（v-show 编排）
* E2E 断言口径与 `check-e2e-sidebar-panels.mjs` 逻辑
* 其他 composable / store 的逻辑重构

## Technical Notes

* 相关文件：
  * `frontend/src/composables/useSessionState.js`（改动点）
  * `frontend/src/store/sidebarPanels.js`（真相源，零依赖）
  * `frontend/src/store/settings.js:36`（已有等价派生）
  * `frontend/src/composables/__tests__/useSessionState.spec.js`（单测）
* 约束参考：AGENTS「关键规则」新增侧边栏面板四处同改一行（SIDEBAR_PANELS + panelIconMap + Home v-show + SettingsPanel 快捷键）；本任务完成后 useSessionState 不再需要登记。
* 覆盖率门禁：前端 ≥70% 硬失败（exclude wailsjs），见 `docs/spec/test-coverage-gate.md`。
