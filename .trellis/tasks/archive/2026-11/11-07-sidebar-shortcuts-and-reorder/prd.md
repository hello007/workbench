# 侧边栏快捷键与顺序自定义

## Goal

为左侧活动栏（ActivityBar）8 个面板提供默认快捷键 Alt+1..Alt+8（按顺序编号），并在设置中支持自定义侧边栏面板顺序。

## What I already know（代码探索结论）

- `frontend/src/components/ActivityBar.vue:443` — `panels` 为硬编码常量数组，8 个面板：directory/ai/ai-chat/toolbox/assets/dashboard/stats/branchcompare；`uiStore.activePanel` 控制激活态，`data-panel-id` 为 E2E 语义锚。
- 活动栏还有三个非 panels 项：设置图标（emit openSettings）、终端图标（emit toggleTerminal）、多设备连接指示（panelVisible 条件显示）——均不在编号范围内。
- **快捷键基础设施齐全**：`frontend/src/store/settings.js` 已有 `parseShortcut/matchShortcut/formatDisplay/isValidShortcut/shortcutFromEvent`，完整支持 Alt 修饰键（Alt+1 直接可用）。
- **全局键盘监听先例**：`frontend/src/views/Home.vue:532` `handleGlobalKeydown`，含焦点判定辅助 `isEditableTarget/isTerminalFocused/isAnyOverlayOpen`（终端聚焦/输入框/对话框打开时不误触）。
- **设置页快捷键 UI 齐备**：`SettingsPanel.vue:277` 快捷键页含「可自定义快捷键列表（录制 UI）+ 固定快捷键 + 冲突检测 + 重置」。
- **设置存储先例**：后端 `model/settings.go` AppSettings 标量字段（shortcutCommandPalette 等 string）+ 标量切片（SearchExcludeDirs []string）直接放 AppSettings，不需要 Ensure 指针段；前端 settingsStore 经 GetSettings/SaveSettings 同步。
- 面板切换本质是 `uiStore.activePanel = id` 赋值，顺序调整仅影响渲染顺序，激活态按 id 保持，天然不受影响。

## Decision (ADR-lite)

### D1：MVP 范围（2026-11-07 用户确认）

**Context**: 基础设施完备，存在三个可选扩展项（tooltip 提示 / 快捷键自定义录制 / 面板隐藏显示）。
**Decision**: 选方案 2「核心 + Tooltip 提示」——Alt+1..8 默认快捷键 + 顺序调整 + 面板 tooltip 显示编号提示（如「工作目录 (Alt+1)」）。
**Consequences**: 快捷键自定义录制与面板隐藏/显示移入 Out of Scope（隐藏面板改动面大，建议单独开任务）。

### D2：顺序调整交互形态（2026-11-07 用户确认）

**Context**: 顺序调整 UI 有「上移/下移按钮」与「拖拽排序」两种方案。
**Decision**: 上移/下移按钮（方案 1），另加「恢复默认顺序」按钮（对齐快捷键页「重置」风格）。
**Consequences**: 实现简单可靠、E2E 稳定；牺牲长距离移动效率（低频操作可接受）。

### 已采纳的默认决策（有争议可推翻）

- Alt+N 跟随顺序：顺序调整后 Alt+1 指向新顺序第一个面板（与用户原话一致）。
- 终端聚焦时不触发（终端程序常用 Alt+N 切 tab，避免劫持；复用 isTerminalFocused 判定先例）。
- 顺序持久化走后端 AppSettings（settings.json），浏览器模式与桌面模式一致。
- 面板数 8 < 9，Alt+1..8 够用；未来面板超 9 个时超出部分不绑定编号（实现按 panels.length 上限循环）。
- 快捷键为固定值（Alt+数字按位置派生），不进快捷键设置页、不提供开关。

## Requirements

- R1 活动栏 8 面板支持 Alt+1..Alt+8 切换，指向当前顺序中第 N 个面板。
- R2 面板 hover tooltip 显示编号提示（如「工作目录 (Alt+1)」）。
- R3 设置中提供面板顺序调整（每行上移/下移按钮 + 恢复默认顺序按钮）。
- R4 顺序持久化到 settings.json（AppSettings 新增 `sidebarOrder []string` 标量切片字段），老配置无字段 = 默认顺序。
- R5 顺序归一化：配置中缺失的面板 id 追加到末尾；配置中的未知 id（面板已移除）过滤掉。
- R6 焦点防护：终端聚焦、输入框聚焦、对话框/消息框/抽屉打开时不触发 Alt+N。
- R7 E2E 功能测试（2026-11-07 用户补充）：新增独立 spec 文件覆盖快捷键切换、顺序调整持久化、tooltip 编号提示主链路。

## Acceptance Criteria

- [ ] Alt+1..Alt+8 能切换到当前顺序中对应位置的面板
- [ ] 顺序调整后活动栏立即重排、tooltip 编号同步更新、activePanel 保持不变
- [ ] 顺序保存后重启保留（settings.json 往返）
- [ ] 老配置（无 sidebarOrder 字段）加载为默认顺序，行为不变
- [ ] 终端聚焦/输入框聚焦/对话框打开时不触发面板切换
- [ ] 单测：顺序归一化（补缺失/滤未知）、Alt+N 匹配与边界（N 超出面板数不触发）、SettingsPanel 上移/下移/恢复默认
- [ ] E2E：Alt+N 切换对应面板（活动栏 is-active 断言）；设置区上移后 Alt+1 指向新第一个面板；恢复默认顺序生效；tooltip 含编号提示；SaveSettings 调用断言（顺序持久化链路）

## Definition of Done

- 前端单测更新（vitest ≥70% 门禁通过）
- E2E 语义锚兼容（data-panel-id 不变，现有用例不破坏）
- 后端 settings.json 字段向后兼容（向后兼容纪律：新增字段不改旧字段）
- model/ struct 变更同步 wailsjs 绑定（models.ts 等，详见关键规则 cross-layer-contracts）
- README.md 若有用户可见功能变化需确认更新

## Technical Approach

1. **后端**：`model/settings.go` AppSettings 加 `SidebarOrder []string`（json: `sidebarOrder`），标量切片对齐 SearchExcludeDirs 先例，无指针段、不进 Ensure/PrivateCopy。
2. **前端 store**：settingsStore 加 `sidebarOrder` ref + load/save（对齐 shortcutXxx 模式）；导出 `orderedPanels()` 归一化纯函数（默认顺序常量 + 补缺失 + 滤未知）。
3. **ActivityBar**：`panels` 常量改为基于归一化顺序的 computed；tooltip content 拼接 `(Alt+N)`。
4. **快捷键**：Home.vue `handleGlobalKeydown` 加 Alt+N 分支（`e.key` 为 1..8 数字 → 排序后面板第 N 个；放在现有焦点判定链中，复用 isEditableTarget/isTerminalFocused/isAnyOverlayOpen）。
5. **设置 UI**：SettingsPanel 快捷键页（或相邻设置区）加「侧边栏顺序」区：列表 + 上移/下移/恢复默认，改动经现有 SaveSettings 保存。
6. **wailsjs 同步**：models.ts 中 AppSettings 类型加 sidebarOrder 字段。

## Implementation Plan

单任务一次实现（改动收敛、无跨服务依赖）：

1. 后端字段 + wailsjs 绑定同步
2. store（sidebarOrder + 归一化纯函数 + 单测）
3. ActivityBar 顺序渲染 + tooltip + 单测
4. Home.vue 快捷键分支 + 单测（含焦点防护）
5. SettingsPanel 顺序调整 UI + 单测
6. E2E 新增 `sidebar-shortcuts.spec.js`（Alt+N 切换 / 顺序调整后快捷键跟随 / 恢复默认 / tooltip 编号 / SaveSettings 持久化断言）+ `wails-mock-defaults.js` 补 sidebarOrder 默认值；现有用例回归不破坏
7. README 确认更新

E2E 约束（关键规则 e2e-testing.md）：`test/expect` 从 `fixtures.js` import；定位走 `activityPanel(page, panelId)` 语义锚（禁 nth）；mock 单一数据源 `wails-mock-defaults.js`；禁 `waitForTimeout`。

## Out of Scope (explicit)

- 快捷键自定义录制（8 个键位进快捷键设置页）
- 面板隐藏/显示（隐藏不占编号）——改动面涉及聚合视图与 E2E，建议单独开任务
- 终端图标 / 设置图标 / 多设备连接指示参与编号（它们不在 panels 数组，保持现状）

## Technical Notes

- 探索文件：ActivityBar.vue、Home.vue（handleGlobalKeydown）、store/settings.js（matchShortcut 等）、SettingsPanel.vue（快捷键页）、model/settings.go（AppSettings/EnsureAllDefaults/PrivateCopy）
- 前端规则文件：docs/agent-resources/conventions/coding-vue-style.md、project-boundaries.md
- 关键规则关联：model 字段变更须同步 wailsjs（cross-layer-contracts.md）；单测禁 mousedown+click 双绑等（frontend-visual-conventions.md）
