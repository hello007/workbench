# 终端页面优化：多终端支持与界面美化

## Goal

终端面板当前为单会话：一次只能开一个 shell，切 Shell 类型/目录会重建会话丢失现场。本任务目标：
1. 支持多终端同时使用：VSCode 式 tab 栏，多会话并存、独立输入输出、可切换可关闭。
2. 终端界面美化：tab 栏精设计 + 工具栏精简 + 退出态美化 + 字号/字体/scrollback 用户可配置。
3. 终端整窗全屏：一键切纯终端形态，覆盖整窗（含 ActivityBar/FileTree）。

## Requirements

### R1 多终端会话（核心）

* 底部终端面板内 VSCode 式 tab 栏：每 tab 一个独立 shell 进程（后端 `sessions` map 天然支持，零后端改动）。
* tab 标签显示 shell 类型 + 目录名（尾段），活动 tab 高亮。
* `+` 按钮新建 tab，目录继承文件树当前目录（决策 D2）；Shell 类型用设置页默认值。
* tab 可关闭（× 按钮 + 鼠标中键），关闭非活动 tab 不影响其他会话。
* 会话数量上限 8 个，达上限后 `+` 置灰并提示。
* tab 溢出时横向滚动，不换行不压缩。
* 单终端（仅 1 个 tab）时隐藏 tab 栏，保持现状零视觉噪音（决策 D1）。

### R2 目录跟随变更（行为变化）

* 移除 `watch(terminalDir)` 自动跟随：文件树切换目录不再改变已建终端的 cwd。
* 新建 tab 继承文件树当前目录。

### R3 事件监听治理（技术修正）

* `useTerminal` 改为 `EventsOn` 返回闭包精准注销，移除全局 `EventsOff('terminal-output')`（多实例下会误删他 tab 监听器，spec 已有规则）。
* 隐藏 tab 的 xterm 实例：`v-show` 保 DOM，激活时 refit + focus（xterm.js 官方实践，隐藏期 write 正常缓冲）。

### R4 崩溃恢复快照升级

* `TerminalSnapshot` 升级：tab 数组（`workDir`/`shellType`）+ `activeIndex`；恢复时循环新建会话，超上限截断。
* 旧版单终端快照（`{visible,height,workDir}`）自动降级为单 tab，向后兼容。
* model 字段变更同步 `frontend/wailsjs/` 绑定三处（spec 契约）。

### R5 终端外观设置

* 设置页新增终端外观一节：字号（默认 14）、字体（默认 Cascadia Code 栈）、scrollback 行数（默认 1000）。
* 工具栏字号快捷调节（A-/A+ 按钮），与设置页双向同步。
* 明暗主题切换时 xterm 主题联动（现有机制保留）。

### R6 界面美化

* tab 栏视觉：设计令牌体系内精设计，明暗双主题适配，高度与 36px 工具栏协调。
* 退出态美化：会话退出后 tab 标记退出状态，终端区提示优化（现为裸黄字）。
* 工具栏精简：与 tab 栏整合后去除冗余元素（Shell 下拉职责移至新建行为 + 设置页默认值）。
* `--terminal-bg` 与 xterm 主题 background 严格一致契约不破坏。

### R7 整窗全屏（决策 D5）

* 工具栏最大化/还原按钮切换全屏；全屏时终端容器 `position:fixed; inset:0` 覆盖整窗（含 ActivityBar/FileTree）。
* 不搬 DOM 不重挂载（xterm 会话无损），靠现有 ResizeObserver 触发 refit；ESC 键退出全屏。
* 全屏状态纳入快照（TerminalSnapshot 增 `fullscreen` 字段），重启还原。
* 全屏下 tab 栏/工具栏保留（多 tab 切换在纯终端形态内可用）。

## Acceptance Criteria

* [ ] 可同时打开 ≥2 个终端 tab，各自独立输入输出互不串扰
* [ ] 关闭非活动 tab 不影响活动 tab 会话；会话上限 8 生效
* [ ] 新建 tab 继承文件树当前目录；文件树切换不再触发已建终端 cd（行为变更验证）
* [ ] 单 tab 时 tab 栏隐藏；≥2 tab 时显示
* [ ] 明/暗主题切换下 tab 栏与终端区域视觉一致
* [ ] 切换 tab 后终端尺寸正确（refit），隐藏期间输出不丢失
* [ ] 旧版单终端 session.json 可正常加载（降级单 tab）；新快照含全部 tab + 活动索引
* [ ] 字号/字体/scrollback 设置生效且重启后保持；A-/A+ 与设置页同步
* [ ] 全屏切换后会话保持（不重挂载）、尺寸正确 refit；ESC 退出；重启后还原全屏态
* [ ] 旧全局 `EventsOff` 用法移除，监听器精准注销（代码审查 + 单测）
* [ ] 现有单终端相关测试全部通过 + 新增多会话用例（前端 vitest ≥70%、后端门禁不变）

## Definition of Done

* 后端/前端测试按覆盖率门禁达标（model/server ≥80%，service ≥76%，util ≥40%，前端 ≥70% 硬失败）
* E2E 用例从 `frontend/e2e/fixtures.js` import，mock 默认值入 `wails-mock-defaults.js` 单一数据源
* model 导出字段变更已同步 `frontend/wailsjs/` 三处（App.js / App.d.ts / models.ts）
* README.md / docs/功能说明.md 更新多终端与外观设置说明

## Decision (ADR-lite)

### D1 多终端交互形态：仅 Tab 栏

**Context**: 需在 tab 栏 / 分屏 / tab+分屏三种形态中择一，底部终端面板常态高度约 200px。
**Decision**: 采用 VSCode 式 tab 栏。单终端时隐藏 tab 栏保持零视觉噪音；分屏留作未来扩展（tab 架构天然兼容）。
**Consequences**: 改动集中在前端 TerminalPanel + useTerminal + ui store，后端 map 会话零改动；分屏不在本期范围。

### D2 目录跟随：新建继承，已建不跟随

**Context**: 现状为文件树切目录终端自动 cd，多 tab 后"跟谁"有歧义；主流（VSCode splitCwd=inherited / Windows Terminal）均为创建时决定。
**Decision**: 新建 tab 继承文件树当前目录；已建 tab 目录独立，文件树切换不再触发已有会话 cd。
**Consequences**: 行为变化——老用户失去自动 cd，换取多 tab 各自现场稳定。

### D3 崩溃恢复快照：还原全部 tab

**Context**: 现有 TerminalSnapshot 仅单终端三字段；恢复机制本为"重新 CreateTerminal"，多 tab 还原成本 = 循环新建。
**Decision**: 快照升级为 tab 数组 + `activeIndex`，恢复时循环新建（超上限截断）；旧版快照降级为单 tab。
**Consequences**: TerminalSnapshot 结构变更须同步 `frontend/wailsjs/` 绑定三处；SessionStateVersion 升版并做旧版降级。

### D4 美化范围：视觉打磨 + 终端外观设置

**Context**: useTerminal fontSize/scrollback 硬编码是现成痛点；全面翻新（光标样式/主题色可选）性价比低。
**Decision**: 方案 2——视觉打磨 + 设置页新增字号/字体/scrollback，工具栏 A-/A+ 快捷调节。
**Consequences**: settings 结构扩展（后端 Settings model + 前端设置页 + wailsjs 同步）；光标样式等可选项明确 out of scope。

### D5 全屏形态：整窗真全屏覆盖层

**Context**: 现有 AI 工具箱"全屏"为 activePanel 切换占上半主区（ActivityBar 保留）；用户诉求"全终端形态"。
**Decision**: 方案 1——终端容器 `position:fixed; inset:0` 整窗覆盖层，含 ActivityBar/FileTree 全部盖住；不搬 DOM（xterm 会话无损），现有 ResizeObserver 触发 refit；ESC + 工具栏按钮退出；快照增 `fullscreen` 字段。
**Consequences**: 不复用 activePanel 机制（少处理底部条与主区双终端入口合并问题）；fixed 层 z-index 层级需与弹窗体系（el-dialog 等）协调，弹窗类组件在全屏态下置顶规则待实现时验证。

## Out of Scope

* 分屏（split pane）布局
* tab 拖拽重排、tab 重命名
* 光标样式/光标闪烁/终端主题色自定义
* 终端会话跨面板复用或浏览器通道多终端（浏览器侧行为保持现状）
* Shell 类型按 tab 快速切换（保留设置页默认值 + tab 关闭重开路径）

## Research References

* [`research/terminal-multi-session-ux.md`](research/terminal-multi-session-ux.md) — 四工具多终端形态对比（tab 为主形态、单终端隐藏 tab 栏）、目录跟随主流做法（创建时决定）、xterm.js 多实例要点（隐藏容器 fit 静默失败、激活 refit、dispose 泄漏教训）及 A/B/C 方案概要。

## Technical Notes

* 已核查文件：service/terminal.go（sessions map + sid 事件）、model/terminal.go、model/session.go（TerminalSnapshot）、frontend/src/composables/useTerminal.js、frontend/src/components/TerminalPanel.vue、frontend/src/store/ui.js
* 后端 `CreateTerminal(dir, shellType, customPath, cols, rows)` 每调用新建会话，多 tab 直接循环调用即可
* xterm.js 多实例：`v-show` 保 DOM + 激活 refit + focus；隐藏期 `write()` 正常缓冲（官方 issue #3029/#664，VSCode setVisible 同款实践）
* 相关 spec：docs/spec/cross-layer-contracts.md（wailsjs 三处同步、EventsOn 闭包注销）、e2e-testing.md、frontend-visual-conventions.md、design-tokens.md
* ui store `terminalDir` 单值语义需重新定义（新建 tab 的继承源），tab 列表状态归属待实现时定（建议新 `useTerminalTabs` 或 ui store 扩展）
