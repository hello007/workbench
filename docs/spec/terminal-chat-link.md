# AI 对话 × 终端联动契约

> 任务来源：11-03-embedded-terminal。AI 对话代码块「在终端执行」联动全局底部终端；后端零改动，契约全在前端三处（chatMarkdown 渲染 / uiStore 通道 / TerminalPanel 消费）。

## 1. 组件链路与职责

```
AiChatPanel（发起）→ uiStore.terminalPendingCmd（内存信箱）→ TerminalPanel（消费）→ WriteTerminalInput 绑定
```

| 环节 | 职责 | 关键约束 |
|---|---|---|
| `utils/chatMarkdown.js` fence 渲染 | 非 chat-question 围栏外包 `.cmd-run-wrap` + 操作条按钮（`data-cmd-run` + `CMD_RUN_BTN_TITLE*` 常量） | HTML 形态稳定（renderMsgMarkdown 字符串替换依赖锚点）；按钮静态字符串零插值（XSS 面） |
| `AiChatPanel.vue` 事件委托 | `onChatMarkdownClick` 分流顺序：chat-question → cmd-run → 链接拦截 | 命令取 `.cmd-run-wrap` 内 `pre code` 的 **textContent**（还原转义/多行），**勿存 data 属性**；剥除围栏结尾 `\n`（否则=自动回车，违背「不自动回车」） |
| `store/ui.js` 信箱 | `terminalPendingCmd = { dir, cmd, seq }` + `pushTerminalPendingCmd(dir, cmd)` | seq 单调递增（store 内计数，同命令两次点击不被 watch 依赖去重吞掉）；消费即置 null，不持久化 |
| `TerminalPanel.vue` 消费 | watch `seq`（immediate）→ 按 dir 找/建 tab → 激活 → 直调 `WriteTerminalInput(tabId, cmd)`（**不含 `\r\n`**） | 直调绑定不经 useTerminal.onData（键盘路径与程序注入路径分离）；try/finally 保证信箱不残留 |

## 2. 关键口径

- **不自动回车**：写入 PTY 的命令结尾必须无换行；多行命令中间 `\n` 原样保留（逐行进缓冲，用户在终端逐段确认）。
- **cwd 来源**：`selectedChatSession.cwd`（会话创建时取目录项路径）；无 cwd → 按钮置灰 + title 提示（`renderMsgMarkdown` 无状态感知，经 `replaceAll` 注入 `is-disabled` 与替换 title 两处锚点，形态唯一性由 chatMarkdown.spec 整串断言锁定）。
- **时序三态**（TerminalPanel 消费侧全覆盖）：
  1. 面板未就绪（tabs 空 / settings 未就绪）：让路不清空，首个可见 watch 建 tab 后补偿消费
  2. 无该 dir tab：createTab 新建 + 惰性 init；达 tab 上限清空信箱不阻塞
  3. 写入时会话已死：WriteTerminalInput reject → 既有 restartTerminal 重启后重试一次 → 仍失败静默交退出提示条
- **与 AI 对话隔离**：命令不写 chatMessages、不进任何持久化；AI 对话进程与终端进程互不影响（事件前缀 `chat-task:*` / 终端事件独立）。

## 3. 反模式（勿回退）

- 把命令文本放进 `data-cmd` 属性（多行/引号/长度受限 + 属性注入面）。
- 消费端经 useTerminal 实例 write（混淆用户输入回显与程序注入路径）。
- 修改 fence 渲染形态时不同步 `renderMsgMarkdown` 的 `replaceAll` 锚点与 chatMarkdown.spec 整串断言。
