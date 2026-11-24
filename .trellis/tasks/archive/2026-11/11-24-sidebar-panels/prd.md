# 新增侧边栏面板图标映射与 SIDEBAR_PANELS 对齐守护

## Goal

消除侧边栏面板图标映射的静默漂移面：`frontend/src/components/ActivityBar.vue` 的 `panelIconMap` 是手写 `id → 图标组件` 映射，新增面板只改 `SIDEBAR_PANELS` 而漏加映射时，`<component :is="undefined">` 渲染为空图标且无任何报错。该问题与刚修复的 `useSessionState` 崩溃恢复白名单漂移（`VALID_PANELS` 硬编码缺项）同型。本任务新增零依赖静态守护脚本，遍历 `SIDEBAR_PANELS` 全量 id 校验 `panelIconMap` 命中，并反向拦截多余/失效键；沿用 `check-e2e-sidebar-panels.mjs` 已确立的 `--selftest + CI 两步`范式，新增面板时守护自动覆盖、零手工同步。

## What I already know

- 真相源：`frontend/src/store/sidebarPanels.js` 的 `SIDEBAR_PANELS`（10 项，零依赖纯模块，E2E 与静态脚本可直接 import/解析）。
- 漂移面：`frontend/src/components/ActivityBar.vue:448` `const panelIconMap = { ... }`，键为面板 id（`'ai-chat'` 带引号，其余裸标识符），值为图标组件标识符；`panels` computed 用 `icon: panelIconMap[p.id]` 映射，无兜底默认图标。
- 同型范式：`scripts/check-e2e-sidebar-panels.mjs`（零依赖、`--selftest` fixture 自证、正式检查两份入口）；`.github/workflows/ci.yml` 2.7 步骤先跑 `--selftest` 再跑正式检查；`frontend/package.json` 有 `check:sidebar-panels` 本地入口。
- 规范约束：`docs/agent-resources/conventions/process-build-quality.md` 规定「新增检查工具须经用户授权」，并维护 CI 真实门禁清单表；本任务由用户明确要求新增守护脚本，授权成立，须同步登记门禁清单。
- 面板注册清单：`docs/spec/prompt-library.md §5` 列「新增一级面板须同步」六处（含 `panelIconMap`），当前仅靠人工遵守。
- 规则行现状：`AGENTS.md` 侧边栏面板关键规则行 302 字符（钩子密集档上限 350），已含「四处同改 + 崩溃恢复白名单派生 + Alt 编号 + 提示词占位符」四组钩子。

## Requirements

1. **静态守护脚本**：新增 `scripts/check-sidebar-panel-icons.mjs`（零依赖、零网络）：
   - 解析 `SIDEBAR_PANELS` 全量 id（复用 `{ id: '<x>' }` 解析口径，校验非空、无重复）；
   - 解析 `ActivityBar.vue` 中 `const panelIconMap = { ... }` 的键集（支持引号键与裸标识符键，忽略注释行）；
   - **缺失校验**：任一 `SIDEBAR_PANELS` id 不在 `panelIconMap` 中即失败，逐项报缺失 id；
   - **反向校验**：`panelIconMap` 存在 `SIDEBAR_PANELS` 未登记的键即失败（防删面板后残留死映射）；
   - 解析不到任何键或真相源异常时失败（fail-closed，避免检查器失效后静默放行）；
   - 退出码 0/1，违规输出 `file:line`/缺失清单 + 修复指引。
2. **`--selftest` 自证**：注入临时 fixture，覆盖「完全对齐通过 / 缺键失败 / 多键失败 / 引号键解析 / 空映射失败 / 真相源空或重复失败 / 注释行不误判」用例，断言判定逻辑有效。
3. **CI 两步**：`.github/workflows/ci.yml` 在 2.7 侧边栏顺序守护之后新增步骤，先 `--selftest` 再正式检查。
4. **本地入口**：`frontend/package.json` 新增 `check:sidebar-panel-icons`。
5. **文档与门禁登记**：`docs/agent-resources/conventions/process-build-quality.md` §1 门禁表新增一行；`docs/spec/prompt-library.md §5` 注明 `panelIconMap` 漏加由该脚本 CI 拦截。
6. **AGENTS 规则行结论**：更新侧边栏面板关键规则行，追加「`panelIconMap` 漏加/多登记由 `check-sidebar-panel-icons.mjs` CI 拦截」短钩子；改后整行字符数须 ≤350（钩子密集档），细节下沉 `docs/spec/`，不突破上限。
7. **不改动渲染行为**：`ActivityBar.vue` 的面板渲染、图标映射内容、交互均不改；本任务只新增守护与文档。

## Acceptance Criteria

- [x] 人为从 `panelIconMap` 删除任一键（如 `todo`）时，`node scripts/check-sidebar-panel-icons.mjs` 退出 1 并明确报出缺失 id
- [x] 人为在 `panelIconMap` 增加未登记键时，脚本退出 1 并报出多余键
- [x] `SIDEBAR_PANELS` 全量 id 均有映射时脚本退出 0 并输出核对数量
- [x] `node scripts/check-sidebar-panel-icons.mjs --selftest` 全部用例通过
- [x] `npm test` 全绿（前端覆盖率四指标仍 ≥70% 硬门禁）
- [x] CI 新增步骤形态与 2.7 一致（`--selftest` + 正式检查两步），`npm run check:sidebar-panel-icons` 本地可用
- [x] `process-build-quality.md` 门禁表、`prompt-library.md §5`、`AGENTS.md` 规则行已更新且规则行 ≤350 字符
- [x] `ActivityBar.vue` 无渲染行为变更（diff 仅可能为零改动）

## Definition of Done

- 守护脚本含 `--selftest` 且自证通过；正式检查在干净仓通过
- `npm test` 全绿，未放宽任何既有断言、未降级检查
- 新检查工具已登记 CI 门禁清单并更新本地脚本入口
- 结论（是否更新 AGENTS 行）明确落地，字符档合规

## Technical Approach

### 1. 脚本契约

```text
node scripts/check-sidebar-panel-icons.mjs
node scripts/check-sidebar-panel-icons.mjs --panel-source <sidebarPanels.js> --icon-source <ActivityBar.vue>
node scripts/check-sidebar-panel-icons.mjs --selftest
```

- 默认源：面板 `frontend/src/store/sidebarPanels.js`；图标 `frontend/src/components/ActivityBar.vue`。
- 面板 id 解析：`/\{\s*id:\s*['"]([^'"]+)['"]/g`（与 `check-e2e-sidebar-panels.mjs` 同口径），校验非空、无重复。
- 图标键解析：定位 `const panelIconMap = {` 至匹配的顶层 `}`，剥离行注释后按 `(?:^|[\n,{])\s*(?:'([^']+)'|"([^"]+)"|([A-Za-z_$][\w$]*))\s*:` 全量提取键；解析结果为空即失败。
- 判定：`missing = panelIds - iconKeys`、`extra = iconKeys - panelIds`，任一非空即违规；全部对齐退出 0。
- 形态对齐 `check-e2e-sidebar-panels.mjs`：顶层 CLI、临时目录 fixture、不导出（避免 import 副作用）。

### 2. selftest 用例

| 用例 | 期望 |
|---|---|
| 面板 id 与图标键完全一致 | 通过 |
| 缺一个图标键 | 失败，报缺失 id |
| 多一个未登记图标键 | 失败，报多余键 |
| 引号键（`'ai-chat'`）与裸标识符键混排 | 通过 |
| 图标映射为空对象 | 失败（解析失效 fail-closed） |
| 面板真相源空列表 / 重复 id | 失败 |
| 注释行含疑似 `id:` / `键:` 文本 | 不误判，通过 |

### 3. 落地清单

- `scripts/check-sidebar-panel-icons.mjs`（新，含 `--selftest`）
- `frontend/package.json`（`check:sidebar-panel-icons`）
- `.github/workflows/ci.yml`（2.8 步骤）
- `docs/agent-resources/conventions/process-build-quality.md` §1 门禁表
- `docs/spec/prompt-library.md §5`
- `AGENTS.md` 侧边栏面板关键规则行

## Decision (ADR-lite)

**Context**：`panelIconMap` 与 `SIDEBAR_PANELS` 是两个必须同步的文件，漏加时渲染空白且无报错；单测渲染断言在「未来有人加默认兜底图标」后会失去鉴别力，且需挂载整组件（wailsjs/pinia/EP 全桩）成本较高。
**Decision**：采用静态守护脚本（集合双向比对）而非仅单测。脚本直接比较两个源的键集，既覆盖「漏加」也覆盖「删面板后残留」，且不受渲染兜底实现影响；沿用仓库既有 `check-*.mjs + --selftest + CI 两步`文化，零运行时依赖。
**Consequences**：新增一个检查脚本与 CI 步骤（约 10 行装配成本）；`panelIconMap` 若被重构为其他形态（如迁入 `sidebarPanels.js`），脚本会 fail-closed 报「解析不到键」，迫使同步调整而非静默失效；面板增删时守护自动覆盖，无需手工改脚本。

## Out of Scope

- `ActivityBar.vue` 渲染逻辑、图标选型、交互与样式改动
- 后端 Go 代码、`frontend/wailsjs/` 生成产物
- `Home.vue` v-show 挂载与 `SettingsPanel.vue` 快捷键清单的同类守护（本次仅覆盖 `panelIconMap`）
- 新增面板流程本身的自动化（如代码生成器）

## Technical Notes

- 约束：脚本零依赖零网络；`frontend/wailsjs/` 不入库，脚本不得 import 任何 Vue/TS 源码，只读文本。
- 现有范式参考：`scripts/check-e2e-sidebar-panels.mjs`、`.github/workflows/ci.yml` 2.6/2.7、`docs/spec/e2e-testing.md §12.4`。
- 规范：`docs/agent-resources/conventions/process-build-quality.md`（新检查工具授权与门禁登记）、`docs/agent-resources/conventions/testing-strategy.md`（前端测试分层）。
- 规则行字符档：`AGENTS.md` §4「两档上限」——普通行 ≤250、钩子密集行 ≤350；当前侧边栏行 302 字符，属钩子密集档。
