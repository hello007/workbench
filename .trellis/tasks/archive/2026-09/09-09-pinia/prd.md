# 前端引入 Pinia 状态管理

## Goal

WorkBench 前端（Vue3 + Element Plus）当前无统一状态管理层，核心状态散落于 `Home.vue` 本地 ref（~20 个，781 行上帝组件）+ `composables/` 模块级单例 ref（useFavorites / useShortcuts）+ localStorage 工具函数（useRecentAccess / useTreeState）。状态通过 props + emit 透传 4 层子组件，跨组件命令走 ref 链。引入 Pinia 建立按域拆分的 setup store，收敛全局态、消除 prop 透传、建立可维护的状态管理基线。

> **背景修正**：任务书原文称"provide/inject 散布各组件"，实际 `grep provide\(|inject\(` 零命中。真实痛点为 Home.vue 上帝组件 + prop 透传 + 单例 composable 模式不统一。范围已按真实现状重定。

## What I already know

### 任务书原文（来源：WorkBench技术债优化提示词.md 任务1）

- 背景：24 组件 + 2 视图，无 store 目录，状态管理用 provide/inject 散布各组件（grep 确认零 defineStore 零 pinia 依赖）
- 目标：引入 Pinia，建 `frontend/src/store/`，按域拆分 directory / gitRepo / fileTree / aiTask / settings
- 关键全局态：当前工作目录、默认目录、仓库筛选列表、收藏夹
- 约束：不改 Wails 绑定；分批迁移每批可测；保留快捷键/右键菜单行为；验收 npm test 全过 + 渲染行为不变

### 代码实际现状（已核实，与任务书背景有出入）

- **provide/inject 零命中**：`grep provide\(|inject\(` on `frontend/src/` 无结果。
- **真实状态分布**：
  - `Home.vue`（781 行）持有核心状态本地 ref：`directories` / `selectedDirectoryId` / `selectedNode` / `latestCommit` / `appVersion` / `activePanel` / `lastInteractedTree` / `clipboard`(reactive) / `terminalVisible/Height/Dir` / `settingsVisible` / `updateDialogVisible` / `updateInfo` / `commandPaletteVisible` / `contentSearchInit` / `repoFilterVisible` / `repoFilterInitialDirId`
  - 通过 props + emit（v-model）下发 4 层子组件：DirectoryTree / FileTreePanel / ContentPanel / ActivityBar / ToolboxPanel / AiFunctionPanel / SettingsPanel / TerminalPanel / CommandPalette / RepoFilterDialog
  - 跨组件操作走 `ref` 链：`fileTreePanelRef.value?.refreshNode()` / `contentPanelRef.value?.previewFile()` 等（Home 充当中介调度）
- **composables 现状**（5 个，已逐一核实）：
  - `useFavorites.js`：模块级 `const favorites = ref([])` → **全局单例**（适合迁入 store）
  - `useShortcuts.js`：模块级 4 个 shortcut ref + `loadShortcuts/saveShortcuts/checkConflict` → **全局单例**（适合迁入 settings store）
  - `useCommandPalette.js`：工厂函数，每次调用产生 `visible/input/fileResults/...` 新实例 → **组件内状态封装**（不迁）
  - `useTerminal.js`：工厂函数，`term/sessionID/currentDir/...` 每实例独立 → **组件内状态封装**（不迁）
  - `useRecentAccess.js` / `useTreeState.js`：纯 localStorage 工具函数，无共享 ref → **保留**（供 store 调用）
- **main.js**：仅注册 ElementPlus + router，无状态层
- **package.json**：无 pinia 依赖，vitest 4.1.5 已配 `test:coverage` 脚本但无阈值

## Decisions (ADR-lite)

**Context**：前端无状态层，Home.vue 上帝组件 + prop 透传 + composable 单例模式混杂，任务书背景描述与实际不符。

**Decision**（7 项已决）：
1. 范围按真实现状重定（用户选 1）：迁 Home.vue 本地 ref + prop 透传 + composable 单例 → Pinia。
2. store 域按状态内聚重划 5 store（用户选 B）：directory / workspace / ui / settings / favorites。
3. 迁移批次风险递增 6 批（用户选 A）：骨架 → favorites → settings → ui → directory → workspace。
4. prop 兼容策略：子组件直读 store 删 prop（用户选 A），彻底治本。
5. ref 链保留（用户选 A）：store 只管响应式数据，命令式跨组件调用仍走 template ref + defineExpose。职责：store=数据，ref=命令。
6. store 写法用 setup store（用户选 A）：`defineStore('id', () => {...})`，语法等同现有 composable。
7. 测试用 createTestingPinia（用户选 A）：store 自动 mock，新增 store 单测。

**Consequences**：
- 收益：Home.vue 减负、状态来源单一、子组件解耦 prop、测试模式统一。
- 成本：每批需改子组件 + 适配 spec（mock 策略从 props 改 createTestingPinia），改动面随批次递增。
- 风险：directory/workspace 批次涉核心流程与时序（onRepoLocate 的 treeKey/treeReadyPromise 时序不能破坏）。

## Requirements

### 通用
- 安装 pinia + `@pinia/testing`，`main.js` 注册 createPinia
- 建 `frontend/src/store/` 目录，setup store 写法，每个 store 配 `acceptHMRUpdate` 保 HMR
- 不改 Wails 绑定（App.js / App.d.ts / models.ts）
- 保留快捷键、右键菜单行为不变
- 不一次性全量重构，分批迁移，每批独立提交 + `npm test` 验证

### store 域定义（5 store）
- `directory` — directories 列表 + selectedDirectoryId + currentDirPath computed + loadDirectories/refreshGitFlags/onDirectorySelect/onRepoLocate 逻辑（内部调 useTreeState 工具）
- `workspace` — selectedNode + latestCommit + clipboard(reactive) + lastInteractedTree（跨三栏共享态）
- `ui` — activePanel + 终端3(terminalVisible/Height/Dir) + 弹窗 visible×6(settings/updateDialog/commandPalette/repoFilter + contentSearchInit + repoFilterInitialDirId) + appVersion
- `settings` — 快捷键（useShortcuts 模块级 ref 迁入）+ 设置面板配置
- `favorites` — useFavorites 模块级 ref 迁入

### 迁移批次（6 批，每批独立提交）
1. 骨架：装 pinia + main.js 注册 + 建 `store/` 空目录 + index 导出
2. favorites：机械替换 useFavorites 单例为 store，验证模式跑通
3. settings：useShortcuts 单例迁入，验证快捷键链路（loadShortcuts/saveShortcuts/checkConflict/matchShortcut）
4. ui：6 弹窗 visible + 终端3 + activePanel + appVersion 剥离 Home.vue
5. directory：核心数据流，directories/selectedDirectoryId/currentDirPath + load/refresh/select，子组件改直读 store 删 prop
6. workspace：selectedNode/clipboard/latestCommit/lastInteractedTree + 三栏 ref 链交互，最后啃

### store 初始化时序设计（批次1 骨架阶段定）
- store action 触发点明确：loadDirectories 等 onMounted 触发的初始化，迁 store 后由 Home.vue `onMounted` 调 store action（不在 store 构造期触发，避免首屏空渲染与副作用时序问题）
- 子组件直读 store 首屏：store 初始值为空数组/null，子组件渲染空态，action 触发后响应式更新（与现有 Home.vue 行为一致：directories 初始 `ref([])`）

### HMR 配置
- 每个 store 文件末尾配 `if (import.meta.hot) { import.meta.hot.accept(acceptHMRUpdate(useXxxStore, import.meta.hot)) }`

## Acceptance Criteria

- [ ] `npm test` 全过（现有 spec 适配 createTestingPinia + 新增 store 单测）
- [ ] `vite build` 通过
- [ ] 组件渲染行为不变（手动验证：目录切换/默认选中/git 标记刷新/文件树展开恢复/命令面板/仓库筛选定位/设置面板/快捷键/复制粘贴/终端）
- [ ] Wails 绑定零变更（App.js / App.d.ts / models.ts diff 为空）
- [ ] `frontend/src/store/` 目录建立，5 store + index，pinia 在 main.js 注册
- [ ] 每个 store 配 acceptHMRUpdate
- [ ] Home.vue 不再透传已迁移 store 对应的 prop
- [ ] onRepoLocate 时序（treeKey/treeReadyPromise）不被破坏

## Definition of Done (team quality bar)

- Tests added/updated（现有 spec 全过，新增 store 单测覆盖 action/getter）
- Lint / build green（`vite build` 通过）
- Docs 更新（README.md / docs/功能说明.md 反映 store 架构，按 CLAUDE.md 要求确认是否更新 README）
- 分批可独立测试，每批一个提交

## Out of Scope (explicit)

- 不改后端 Go / Wails 绑定
- 不改组件模板结构与交互行为（保留快捷键/右键菜单行为）
- 不迁 useCommandPalette / useTerminal（组件内实例态，非全局）
- 不迁 useRecentAccess / useTreeState（localStorage 工具，保留供 store 调用）
- 不迁 AiFunctionPanel 内部状态（本任务 store 域不含 aiTask，目录结构不预留空壳）
- 不一次性全量重构（分批）
- 不引入事件总线替代 ref 链（ref 链保留）

## Technical Notes

- 前端入口：`frontend/src/main.js`
- 状态根：`frontend/src/views/Home.vue`（781 行，~20 ref）
- 现有 composable 单例：`frontend/src/composables/useFavorites.js` / `useShortcuts.js`
- 跨组件 ref 链方法（保留，不走 store）：FileTreePanel.refreshNode/previewFile/locateNode/restoreTreeState/showCreateAt/showRenameAt/showCopyToDialog/saveCurrentState；ContentPanel.clearPreview/startBatchPull/previewFile；DirectoryTree.closeMenu/triggerRenameCurrent/triggerDeleteCurrent
- onRepoLocate 时序参考：`research/cross-workdir-locate.md`（已存在，迁移时不得破坏）
- 测试：`frontend/src/**/__tests__/*.spec.js`（Vitest + @vue/test-utils 2.4.9 + createTestingPinia）
- 约束来源：CLAUDE.md「修改 app.go App 方法签名或 model/ 导出 struct 字段时须同步 wailsjs 绑定」——本任务不动绑定，仅前端内部
- 现有 10 个组件 spec：ActivityBar/AiFunctionConfigDialog/AiFunctionPanel/AiTaskHistoryPanel/CommandPalette/ContentPanel/DirectoryTree/FileTreePanel/GitInfo/RepoFilterDialog/ToolboxPanel + Home.spec.js
