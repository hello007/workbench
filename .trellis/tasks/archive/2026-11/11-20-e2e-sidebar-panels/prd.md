# E2E 侧边栏面板顺序与 SIDEBAR_PANELS 同源守护

## Goal

消除「侧边栏默认面板顺序」在业务侧与 E2E 侧各写一份导致的双份漂移：业务侧增删面板时测试侧无感知，spec 直接变红（f0cb6e5 新增提示词面板后 `DEFAULT_ORDER` 仍是 9 项、`toHaveLength(9)` 失败，直到任务 11-20-e2e-window 顺手修复）。本任务让 E2E 断言与 `SIDEBAR_PANELS` 单一真相源同源，并新增静态守护防止硬编码回流。

## What I already know

- 真相源：`frontend/src/store/settings.js:36` `SIDEBAR_PANELS`（10 项，prompts 末尾；order = 默认顺序）。
- 重复副本：`frontend/e2e/sidebar-shortcuts.spec.js:39` `DEFAULT_ORDER`（硬编码 10 项）+ 第 125 行 `toHaveLength(10)`，另有多处面板 id 字面量（`'ai'`/`'ai-chat'`/`'branchcompare'`/`'todo'`）与文案 `'工作目录 (Alt+1)'`。
- 单测侧已同源：`frontend/src/store/__tests__/settings.spec.js:385`、`SettingsPanel.spec.js`、`Home.spec.js` 均由 `SIDEBAR_PANELS` 派生，无重复。
- E2E 跨 `src/` 导入先例：`frontend/e2e/fixtures.js` 静态 import `../src/test/wails-mock-defaults`（纯数据模块，零运行时依赖）。
- **直接 import `settings.js` 不可行**：其静态 import `vue`、`pinia`、`../../wailsjs/go/main/App`；wailsjs 为生成产物且无扩展名，node ESM 解析实测失败（`Cannot find module .../wailsjs/go/main/App`）。
- 11-20 范式：`scripts/check-e2e-listener-hooks.mjs`（`--selftest` fixture 自证 + 正式扫描）、`ci.yml` 两步（2.6 节）、`package.json` 脚本、`docs/spec/e2e-testing.md §12.1`、AGENTS.md 关键规则行。
- vitest `exclude: ['e2e/**', 'e2e-desktop/**']`，E2E 目录不入单测；覆盖率门禁整体四指标 ≥70%。

## Requirements

1. **单一真相源**：把 `SIDEBAR_PANELS` 抽到零依赖纯模块 `frontend/src/store/sidebarPanels.js`，`settings.js` 重新导出（`export { SIDEBAR_PANELS } from './sidebarPanels'`），store 索引 `export * from './settings'` 不变，全部既有消费面零改动。
2. **E2E 同源**：`frontend/e2e/sidebar-shortcuts.spec.js` 从纯模块导入 `SIDEBAR_PANELS`，顺序、数量、Alt 编号目标、tooltip 文案全部派生，删除硬编码 `DEFAULT_ORDER`。
3. **静态守护**：新增 `scripts/check-e2e-sidebar-panels.mjs`（零依赖零网络），默认扫描 `frontend/e2e/**`，命中「一行出现 ≥3 个已知面板 id 字面量」即判为硬编码列表并报 `file:line`；同时校验收敛点——`sidebar-shortcuts.spec.js` 必须从纯模块导入 `SIDEBAR_PANELS`。
4. **守护自证**：脚本带 `--selftest`，注入违规/合规 fixture 断言判定逻辑有效（parse 正常、硬编码拦截、语义选择器放行、阈值边界、重复/空 id 失败、未导入失败）。
5. **CI 两步**：`.github/workflows/ci.yml` 在 2.6 监听守护之后新增步骤，先 `--selftest` 再正式检查。
6. **本地入口**：`frontend/package.json` 新增 `check:sidebar-panels`。
7. **文档**：`docs/spec/e2e-testing.md` 新增 §12.4；AGENTS.md 关键规则表新增一行（≤250 字符）。

## Acceptance Criteria

- [ ] 人为在 `SIDEBAR_PANELS` 增/删一项且不同步测试侧时，E2E 断言自动跟随（同源），静态检查对「重新硬编码」明确失败并给出 `file:line`
- [ ] `node scripts/check-e2e-sidebar-panels.mjs --selftest` 全用例通过；正式检查在干净仓通过
- [ ] 全量 `npm run e2e` 通过，`sidebar-shortcuts` 连跑 3 次无 flaky
- [ ] `npm test` 全绿，覆盖率不低于 70% 门禁
- [ ] `docs/spec/e2e-testing.md` 与 AGENTS.md 关键规则行已更新且字符档合规

## Technical Approach

### 1. 纯模块抽取

`frontend/src/store/sidebarPanels.js`（零 import）：

```js
export const SIDEBAR_PANELS = [ /* 原 settings.js 的 10 项定义原样迁入 */ ]
export const SIDEBAR_DEFAULT_ORDER = SIDEBAR_PANELS.map((p) => p.id)
```

`settings.js`：删除本地定义，改为 `import { SIDEBAR_PANELS, SIDEBAR_DEFAULT_ORDER } from './sidebarPanels'` + `export { SIDEBAR_PANELS } from './sidebarPanels'`（保留公共导出，消费者不变）。行为零变化。

### 2. E2E spec 同源

```js
import { SIDEBAR_PANELS } from '../src/store/sidebarPanels'
const DEFAULT_ORDER = SIDEBAR_PANELS.map((p) => p.id)
const panelAt = (altN) => SIDEBAR_PANELS[altN - 1]   // Alt+编号 → 面板（1-based）
```

- Alt+2/3/8/9 目标改 `panelAt(n).id`；`toHaveLength(10)` 改 `SIDEBAR_PANELS.length`；上移用例的 `'ai'`/`'directory'` 改 `panelAt(2).id`/`panelAt(1).id`；tooltip 文案改 `${panelAt(1).label} (Alt+1)`。
- 新增前置断言：`expect(SIDEBAR_PANELS.length, 'Alt+1..9 固定映射要求至少 9 个面板').toBeGreaterThanOrEqual(9)`。
- 新增渲染顺序断言：`[data-panel-id]` 数量 = `SIDEBAR_PANELS.length` 且 id 序列 `toEqual(DEFAULT_ORDER)`（业务侧排序漂移的显式失败点）。

### 3. 静态检查脚本契约

```
node scripts/check-e2e-sidebar-panels.mjs [targetDir] [--source <file>] [--selftest]
```

- `--source` 默认 `frontend/src/store/sidebarPanels.js`：解析 `id: '<x>'` 列表，校验非空、无重复（失败即退出 1）。
- 扫描 target（默认 `frontend/e2e`，递归 `.js/.jsx/.ts/.tsx/.mjs/.cjs/.mts/.cts`）：单行出现 **≥3 个**已知 id 字符串字面量 → 违规（阈值 3 使 `activityPanel(page, 'directory')` 等单 id 语义定位放行）。
- 收敛点校验：若 target 下存在 `sidebar-shortcuts.spec.js`，必须含 `from '...store/sidebarPanels'` 导入，否则违规。
- 退出码 0/1；违规输出 `file:line` + 修复指引；与 `check-e2e-listener-hooks.mjs` 同构（顶层 CLI，不导出，避免 import 副作用）。

### 4. selftest 用例

| 用例 | 期望 |
|---|---|
| 3 个已知 id 的数组字面量（嵌套目录 / .ts） | 失败，命中 `file:line` |
| 仅 1 个 id 的语义选择器调用 | 通过 |
| 从纯模块导入并派生 | 通过 |
| 一行 2 个 id（阈值边界） | 通过 |
| source 空数组 / 重复 id | 失败 |
| 指定 spec 未导入纯模块 | 失败 |
| 指定 spec 已导入 | 通过 |

### 5. 落地清单

- `frontend/src/store/sidebarPanels.js`（新）、`frontend/src/store/settings.js`（改导出）
- `frontend/e2e/sidebar-shortcuts.spec.js`（同源化）
- `scripts/check-e2e-sidebar-panels.mjs`（新，含 --selftest）
- `frontend/package.json`（`check:sidebar-panels`）
- `.github/workflows/ci.yml`（2.7 步骤）
- `docs/spec/e2e-testing.md` §12.4、AGENTS.md 关键规则行

## Decision (ADR-lite)

**Context**：业务侧与 E2E 侧各持一份面板顺序，增删面板时 E2E 静默变红；单纯补一份副本无法根治，单纯静态检查仍需人工同步。
**Decision**：采用「纯模块单一真相源 + 静态守护防回流」组合（选项 A+C）。抽取零依赖模块解决 import 不可行问题，E2E 直接派生；静态脚本拦截未来再次硬编码，并复用 11-20 已确立的 `--selftest + CI 两步` 范式。
**Consequences**：新增一个源文件与一个检查脚本（约 10 行装配成本）；面板增删时 E2E 自动跟随、零手工同步；若未来有人绕过同源直接写字面量，CI 明确失败并定位。

## Out of Scope

- 后端 Go 代码、`frontend/wailsjs/`、既有 mock 数据形状
- 侧边栏面板本身的增删、图标映射、交互改动
- 组件单测新增（现有 settings.spec 已覆盖纯模块导出与归一化逻辑）

## Technical Notes

- 约束：E2E 从 `frontend/e2e/fixtures.js` import `{ test, expect }`，禁裸 `@playwright/test`、禁 `waitForTimeout`。
- 检查脚本零依赖零网络，须 `--selftest`；新增检查工具已在本文档说明并待确认（`process-build-quality.md` 约束）。
- 规范：`docs/spec/e2e-testing.md §5.2/§12`、`docs/agent-resources/conventions/testing-strategy.md`。
- 风险：Playwright 对 `src/` 纯模块的静态导入已由 fixtures.js 先例证明可行；本地验证以 `npm run e2e -- sidebar-shortcuts` 为准。
