# e2e-terminal-fullscreen-esc-baseline-fix

## Goal

修复 E2E 既有失败 `frontend/e2e/terminal-tabs.spec.js:153`（终端全屏与全局 ESC 过滤用例）第 175 行断言 `expect(addBase).toBe(0)` 实际收到 1 的问题，恢复该用例与 `npm run e2e` 全量绿。

## What I already know（已验证事实）

* 复现确认：本地跑该用例失败于 175 行，`Expected: 0, Received: 1`，与用户描述一致。
* 全仓 window 级 `keydown` 注册点仅 2 处（grep 确认）：
  * `frontend/src/components/TerminalPanel.vue:653` — 全屏 ESC 监听，仅 isFullscreen 时挂载（用例预期计数对象）。
  * `frontend/src/components/FolderTree.vue:377` — onMounted 常驻注册（ESC 关节点右键菜单），**基线 +1 的来源**。
* `Home.vue:92`：`<AssetsPanel v-show="uiStore.activePanel === 'assets'" />` — AssetsPanel（含 FolderTree）在 `/` 页面加载即常驻挂载，`v-show` 只控显隐，`onMounted` 立即执行。
* 临时诊断 spec 抓 stack 证据：页面加载期 window keydown 注册恰 1 次，stack 指向 Home chunk（AssetsPanel→FolderTree 的引入链）。
* 时间线（git log 闭环）：
  * `013032e`（2026-10-13）：AssetsPanel 以 v-show 常驻挂载进 Home.vue。
  * `bf5b2f7`（2026-10-21）：用例写入 `expect(addBase).toBe(0)`，当时恰好成立。
  * `ddd0807`（2026-11-06）：FolderTree 引入 window keydown（ESC 关右键菜单）→ 基线变 1 → 用例自此失败。早于 0205ec6，与用户「0205ec6 之前就失败」吻合。

## 根因定性

**不是组件缺陷，也不是 hook 误计数**，而是第三种形态：**新增合法注册点导致绝对基线断言过时**。

* FolderTree 的 window keydown 是正当功能（ESC 关右键菜单），与同 onMounted 注册的 click/contextmenu/scroll 三监听对称，常驻 no-op 代价极低。
* 用例 hook 本身无多算（stack 证据：真实注册 1 次）。
* 用例内部自相矛盾：注释写「计数以 openTerminal 挂载后的值为基线取增量」，后续全部断言（184/194/200/217/224 行）均用 addBase/removeBase 相对增量；`toBe(0)` 是与注释矛盾的残留绝对断言。

## Requirements

* 该用例恢复绿：基线断言改为与注释一致的相对计数语义。
* 用例注释更新：记录 FolderTree 也在 window 注册 keydown 的事实与「基线取相对增量、不依赖绝对值」的设计依据，防止后人再困惑或再次引入绝对断言。
* FolderTree 右键菜单的 4 个 window 监听（click/contextmenu 捕获/scroll 捕获/keydown）改为**菜单打开期才注册、关闭即注销**（方案 B）；卸载期兜底注销防泄漏。
* FolderTree.spec.js 补监听挂载/卸载生命周期单测（打开前零注册、打开后注册、各关闭路径注销、卸载防泄漏）。

## Decision (ADR-lite)

**Context**: 绝对基线断言 `toBe(0)` 因 ddd0807 引入 FolderTree 合法 window keydown 而失效；修复方向需在「改用例」与「改组件」间取舍。
**Decision**: 用户确认 A+B 都做——A（用例相对基线 + 注释更新）根治脆弱断言；B（FolderTree 监听动态挂载）让非菜单期 window 零监听、消除对用例基线的实际污染源。
**Consequences**: A 保证未来新增合法 window keydown 注册点不再破坏用例；B 增加菜单生命周期复杂度，需配单测守护注册/注销配对与卸载防泄漏；assets 系列 E2E 需回归验证菜单行为不变。

## Technical Approach

**方案 A：改用例（推荐）** — 删除 `expect(addBase).toBe(0)`（或改为记录式），后续增量断言天然兼容；同步更新 hook 注释。

* 优点：最小改动、零行为变更、零回归风险；符合用例注释既有设计意图（相对计数）；对未来新增合法 window keydown 注册点天然免疫。
* 缺点：FolderTree 常驻监听维持现状（本就是低代价 no-op 设计，非缺陷）。

**方案 B：改组件** — FolderTree 4 个 window 监听改为菜单打开注册/关闭注销动态挂载。

* 优点：非菜单期 window 零监听。
* 缺点：生命周期复杂化，破坏现有 4 监听对称设计；需回归 FolderTree.spec.js 与 assets 系列 E2E；且不能根治——未来任何组件合法注册 window keydown 仍会破坏绝对基线断言，脆弱性根源在用例。

## Acceptance Criteria

* [x] `npx playwright test e2e/terminal-tabs.spec.js` 全绿（4/4，含原失败 153 行用例）。
* [x] `npm run e2e` 全量 0 失败（138/138）。
* [x] 用例注释更新：记录两个合法 window keydown 注册点与相对基线依据。
* [x] FolderTree 4 个 window 监听动态挂载：非菜单期 window 零监听，卸载期兜底注销。
* [x] FolderTree.spec.js 新增监听生命周期单测（5 条）且全绿。
* [x] `npm test`（前端单测全量）不回归（2671/2671，119 文件）。
* [ ] 根因结论沉淀（docs/spec 或用例注释内）——用例注释已承载；e2e-testing.md 沉淀待 finish-work 阶段评估。

## Definition of Done

* 上述验收全过；若涉及终端相关单测回归（方案 A 不改组件，无需）。
* 归档前确认 README.md 是否需更新（本任务预期不需要）。

## Out of Scope

* FolderTree 右键菜单监听生命周期重构（除非用户选方案 B）。
* 其他 E2E 用例改动（全量跑绿即可，不顺手改无关用例）。

## Technical Notes

* E2E 运行链路：`npm run e2e` → vite build + vite preview（固定端口 4173）→ Chromium 访问；Wails 后端 mock 注入（fixtures.js）。
* 相关规范：`docs/spec/terminal-multi-session.md`（全屏 ESC 过滤契约）、`docs/spec/e2e-testing.md`（E2E 约定）。
