# PRD：AI 对话 × 内嵌终端联动（一期）

## Goal

打通「AI 对话 ↔ 终端」闭环：AI 回复中的命令一键送入全局底部终端执行（类 Warp/VS Code 的对话+终端同屏）。**复用现有终端基建**（后端 TerminalService + 前端 TerminalPanel），一期不做对话页内嵌新分栏。

## 现状盘点（研究结论，显著缩小范围）

WorkBench 已有完整终端能力：
- 后端 `service/terminal.go` TerminalService：PTY 会话按目录建实例、WriteInput/ChangeDir/Resize/CloseTerminal/CloseAll，输出经 Wails 事件推送
- 前端 `components/TerminalPanel.vue`（Home 级全局底部停靠，ActivityBar 开关，多 tab 按目录，状态持久化）+ `composables/useTerminal.js` + `@xterm/xterm` 依赖齐备
- `uiStore.terminalDir` 已支持指定终端工作目录

缺口仅两处：终端 cwd 不跟随对话会话；AI 代码块无送入终端入口。

## Requirements

- **R1 代码块「在终端执行」**：AI 对话 assistant 回复的代码块提供「在终端执行」入口（hover 操作区按钮，与既有复制按钮同区）。
- **R2 点击行为**：① 打开全局底部终端（`uiStore.terminalVisible=true`）；② 终端定位到当前会话 cwd 对应 tab（无则创建，机制同 `terminalDir`）；③ 命令文本写入该 tab 的 PTY 输入行——**不带换行、不自动回车**，用户确认后执行。
- **R3 cwd 来源与兜底**：cwd 取 `selectedChatSession.cwd`；无选中会话/会话无 cwd 时按钮置灰 + tooltip 说明「当前会话无工作目录」。
- **R4 命令传递通道**：AiChatPanel 与 TerminalPanel 跨组件送入命令，经 uiStore 单向状态（如 pending 命令队列）或既有事件机制，研究阶段定案；多语言命令（含中文/引号）按原样写入，不做转义加工。
- **R5 隔离性**：送入命令不出现在会话消息记录；AI 对话进程与终端进程互不影响。
- **R6 退出态友好**：目标终端进程已退出时，送入动作触发该 tab 重启（复用既有 restart 逻辑）或提示（研究定案，取改造成本低者）。

## 已定决策

- **零后端改动为默认目标**：TerminalService 绑定（CreateTerminal/WriteTerminalInput/ResizeTerminal/CloseTerminal）一期原样复用；仅当前端送入通道确无绑定时才考虑最小新增。
- 跨组件状态经 uiStore（终端可见性/目录已是 uiStore 现状，命令队列沿用同域）。
- 代码块操作区在 AiChatPanel 消息渲染处扩展，样式遵循 frontend-visual-conventions。

## 非目标（二期候选）

- 对话页右侧内嵌终端分栏（Warp 式左右布局，需重构 TerminalPanel 单例）
- 终端选区「问 AI」、终端输出作为对话上下文
- 终端命令历史注入提示词、shell 可配置

## 验收标准

1. R1–R6 满足；点击「在终端执行」后终端打开、目录正确、命令出现在输入行且未执行
2. 无 cwd 会话按钮置灰有提示；命令不入会话消息
3. 前端 vitest 全量通过、新增逻辑有单测；`wails build` 成功
4. 若涉及 App 签名变更，wailsjs 三处同步（cross-layer-contracts spec）
