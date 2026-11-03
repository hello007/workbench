# 研究：AI 对话 × 终端联动落点与口径

> 2026-11-03 盘点。结论：**后端零改动**，前端三处扩展（代码块按钮 / uiStore 通道 / TerminalPanel 消费）。

## 1. 现有终端基建（全部复用，不改）

### 后端（零改动验证过）
- `service/terminal.go` TerminalService：`CreateTerminal(dir, shellType, customPath, cols, rows)`（PTY 会话按 dir 建）、`WriteInput(sessionID, input)`、`ChangeDir`、`Resize`、`CloseTerminal`、`CloseAll`（app 退出清理）；输出经 Wails 事件推送（`startOutputPump`）。
- `app_terminal.go` 绑定：`CreateTerminal / WriteTerminalInput / ChangeTerminalDir / ResizeTerminal / CloseTerminal / GetShellConfigs`。
- PTY 层 `util.PtyProcess`（Windows ConPTY），terminal_test.go / terminal_windows_test.go 已有测试。

### 前端
- `components/TerminalPanel.vue`（893 行）：Home 级全局底部停靠（Home.vue:98-105，`uiStore.terminalVisible` 控制显隐），**多 tab 按 dir 组织**：watch tabs 快照（:254）经 `wails-mock` 的 workDir 持久化重建；`uiStore.terminalDir` 为新建 tab 的默认 dir（:279/:304/:356）；重启走 `inst.restartTerminal`（:399）。
- `composables/useTerminal.js`：xterm 初始化/fit/onData→WriteTerminalInput/事件收流。
- 依赖 `@xterm/xterm ~6.0.0 + addon-fit + addon-web-links` 已在 package.json。

### uiStore（store/ui.js）
- `terminalVisible / terminalHeight / terminalDir / terminalFullscreen / terminalTabsSnapshot` 已有；`toggleTerminal() / toggleTerminalFullscreen()` 已有。**没有**命令送入通道——需新增。

## 2. 会话 cwd 数据源

- `store/aiChat.js`：`selectedChatSession`（GetChatSession 加载的完整会话对象）。
- `model/chat_session.go:45`：`Cwd string json:"cwd"`——会话创建时取目录项路径（store/aiChat.js:196-199 `createChatSession` 用 `dir.path`）。
- 空态：无选中会话（`selectedChatSession.value=null`）或 cwd 空串 → 按钮置灰。

## 3. 代码块渲染与事件委托范式（照抄 chat-question 卡片）

- 渲染：`utils/chatMarkdown.js` `renderChatMarkdown`（markdown-it + hljs）。**fence 规则已被覆写**（:143）：先拦 ```` ```chat-question ```` 渲染卡片，其余走 `defaultFence`。
- 普通代码块 HTML 形态：`<pre class="hljs"><code>…</code></pre>`（highlight 回调 :47/:49）。
- 交互：v-html 内无法绑 Vue 事件，**事件委托**在 AiChatPanel 气泡容器 `onChatMarkdownClick`（:626）——先 `onChatQuestionClick`（closest('.chat-question')）分流，再处理链接拦截。「在终端执行」按钮按同一委托模式：`event.target.closest('[data-cmd-run]')` 命中 → `closest('pre').querySelector('code').textContent` 取原始命令文本（textContent 天然还原转义，**勿用 data 属性存命令**——多行/引号/长度都受限）。
- 按钮注入点：fence 渲染器把 `<pre>` 包一层 `<div class="cmd-run-wrap">` + 头部操作条（含「在终端执行」按钮 `data-cmd-run`）；或后处理 replace。**约束**：`renderMsgMarkdown`（AiChatPanel:643）还有 chat-question-answered 的字符串替换逻辑，注入形态必须稳定可替换；流式渲染中代码块可能未闭合（markdown-it 容错），按钮在流式过程中即可出现，无碍。

## 4. 跨组件命令通道设计（新增，唯一实质新逻辑）

方向：AiChatPanel（发起）→ uiStore（中转）→ TerminalPanel（消费）。

```js
// store/ui.js 新增
const terminalPendingCmd = ref(null)  // { dir: 'D:/xxx', cmd: 'git status', seq: 1 }
```

- AiChatPanel 点击按钮：`uiStore.terminalVisible = true; uiStore.terminalDir = cwd; uiStore.terminalPendingCmd = { dir: cwd, cmd, seq: ++seq }`。
- TerminalPanel watch `terminalPendingCmd.seq`：按 dir 找/建 tab → 激活该 tab → `WriteTerminalInput(tab.id, cmd)`（**直调绑定**，不经 useTerminal 实例——useTerminal 的 onData 是用户键盘路径，注入是程序路径，分离更清晰）→ 清空 pending。
- **不自动回车**：cmd 原样写入（结尾不带 `\r`），用户在终端里按 Enter 执行。
- seq 单调递增防同命令两次点击被 watch 去重丢失；消费后置 null。
- 退出态（R6）：`WriteTerminalInput` 对已死会话报错 → catch 后对该 dir 的 tab 走既有 `restartTerminal`（TerminalPanel:399 已有）再写一次；无该 dir 的 tab 时 `CreateTerminal` 新建即天然无此问题。

## 5. 风险与口径

| 风险 | 处置 |
|---|---|
| 多行命令（AI 输出多行脚本） | textContent 原样含 `\n`；PTY 输入多行=逐行进缓冲（bash 会整体回显待执行），符合直觉；一期不特殊处理，PRD R4 口径「原样写入」 |
| cwd 是文件而非目录（理论上会话 cwd 恒为目录） | TerminalService.CreateTerminal 已有校验，失败走既有错误提示 |
| TerminalPanel 未挂载（v-if）时送入 | terminalVisible=true 先行 → Home v-if 挂载 → watch 时序上 pending 消费在 mounted 后；实测验证，必要时 nextTick/首轮 watch immediate 兜底 |
| 命令注入面 | 无新增：cmd 来自 AI 回复文本，用户可见可改，回车执行权在用户；不落会话消息、不进任何持久化 |

## 6. 测试口径

- `utils/chatMarkdown.spec.js`：扩展 fence 注入断言（按钮存在、code textContent 可取、chat-question 不受影响）。
- `store/ui` 或组件 spec：pending 通道写入/消费/seq 递增。
- TerminalPanel 相关既有测试（composables/__tests__/useTerminal.spec.js）零破坏。
- 后端无改动，无需新增 Go 测试。
