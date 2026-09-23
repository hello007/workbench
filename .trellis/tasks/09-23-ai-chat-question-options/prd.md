# AI 对话选择题交互（方案 A + B）

## Goal

AI 对话（ChatService）中模型需要用户做单选/多选决策时，当前 `--print` 无头模式下 AskUserQuestion 工具被 CLI 自动拒绝式应答（不渲染 UI、不等待），模型基于「用户没答」继续输出，回复混乱。本任务落地组合方案：

- **方案 A（兜底）**：禁用 AskUserQuestion 工具 + prompt 声明非交互环境，让模型稳定以文字/结构化块提问。
- **方案 B（体验）**：约定结构化提问块格式，前端渲染为单选/多选 chips 组件，用户点选后拼答案文本经现有 `--resume` 链路作为下一轮用户消息回传。

## What I already know

- 根因：`claude -p` 无头模式 AskUserQuestion 立即自动应答不等待（GitHub issue 2026-02 实证）；真原生交互需 `--input-format stream-json` 双向流 control protocol（方案 C，已否决——投入产出比低）。
- `buildChatArgs`（service/chat_service.go:684）现拼 `-p <prompt> --output-format stream-json --verbose` + 可选 `--resume`/`--permission-mode`/`--model`；无工具禁用参数。
- `claude --disallowedTools`（别名 `--disallowed-tools`）已验证存在。
- 一问一进程架构：每轮 `RunChat` spawn 进程，`--resume <claudeSessionID>` 续上下文；`ChatSession.ClaudeSessionID` 空表示首轮。
- 前端渲染链路：`frontend/src/utils/chatMarkdown.js`（markdown-it 单例，html:false 防 XSS，highlight.js 按需注册）+ `AiChatPanel.vue` `renderChatMarkdown(msg.content)` v-html；已有事件委托先例 `onChatMarkdownClick`（拦截 `<a>` 点击）。
- 消息落盘：`model.ChatMessage{Role, Content(markdown), Timestamp, TaskID}`，原文即 Content；历史重载走 `GetChatSession`。
- 流式：`chat-task:output` 事件推增量文本；`ChatTaskState.Reply`/`ChatTaskRunResult.Reply` 为累积全文。

## Requirements

### 方案 A（后端，2 处）

- A1 `buildChatArgs` 恒定追加 `--disallowedTools AskUserQuestion`（所有 chat 轮次禁用该工具，杜绝自动拒绝噪音）。
- A2 首轮（`ClaudeSessionID` 为空）prompt 前缀追加非交互环境声明：选择题须用约定 `chat-question` 代码块输出、块后停止，等用户点选/文字回复。续轮 resume 上下文已含声明不重复注入（省 token）。

### 方案 B（以前端为主）

- B1 约定 fence 标记 ` ```chat-question `，内容为 JSON：`{questions:[{question, options:[{label,description?}], multiSelect?}]}`（对齐 AskUserQuestion schema 语义，支持一次多问题）。
- B2 `chatMarkdown.js` 拦截该 fence 渲染为交互组件（问题 + 选项 chips，单选/多选按 multiSelect）。
- B3 点选提交后前端拼结构化答案文本（含问题与所选 label）调 `RunChat`，经 `--resume` 回传模型。
- B4 历史消息原文含 fence，重载后同样渲染 chips；已答消息 chips 置灰禁用（本地 state，不落盘）。
- B5 流式期间 fence 未闭合时按普通代码块显示原始 JSON（markdown-it 容错已支持未闭合围栏），done 后渲染为 chips。
- B6 模型不守约定（无块、块 JSON 非法）时降级：非法块按普通代码块显示，文字提问走普通对话——不阻塞、不报错。
- B7（用户明确约束）chips 内联于消息流（非弹窗/非模态），不抢焦点、不阻塞任何导航：切其他会话、切工作目录树、切面板均随时可用。
- B8 已答 state 存 aiChat store 内存（key：chatSessionId + 消息标识），切换会话往返后置灰状态保留；不落盘（重启恢复为未答可再点，可接受）。
- B9 提交时机守卫：该会话存在 running/queued 任务时（ChatService 同会话串行保护 E_CHAT_IN_PROGRESS），提交按钮禁用并在任务结束后恢复。

## Acceptance Criteria

- [x] `buildChatArgs` 产物含 `--disallowedTools AskUserQuestion`（单测更新）。
- [x] 首轮 prompt 带声明、续轮不带（单测）。
- [x] 含合法 `chat-question` 块的 markdown 渲染为卡片组件（vitest 8 例）。
- [x] 非法 JSON 块 / 未闭合流式块按普通代码块显示（vitest）。
- [x] 选项点选（单选 radio / 多选 toggle）后每题至少一项启用提交；提交后置灰（vitest 覆盖渲染层；交互为事件委托 DOM 逻辑，E2E 未覆盖——可选项跳过）。
- [x] 提交拼出的答案文本格式稳定（`用户通过选项卡片回答：\n1. <问题>：<label>、<label>`）。
- [x] 历史会话重载后卡片正常渲染；已答标记切会话往返保留（store 内存态，重启恢复未答——B8 决策）。

### 实施偏差记录

- 原 AC「单选 chip 点击即提交」调整为统一提交按钮（每题至少一项后启用）：多问题卡片下点击即提交会打断后续问题作答，统一按钮语义更稳且实现更简。
- 全量后端测试中 `TestApp_StartWebServe_StartupMatrix/设置开启默认启动`（main 包 web_serve_test.go:397）失败，git stash 后干净 master 同样复现——**既有失败，与本任务无关**；改动域 service 包全绿。

## Definition of Done

- 后端 `go test ./...` 绿、前端 `npm test` 绿（覆盖率门禁不降）。
- `docs/功能说明.md` AI 对话章节补充选择题交互说明。
- 需要时沉淀 spec（chat-question 约定格式契约）至 `docs/spec/`。

## Decision (ADR-lite)

**Context**：无头模式 AskUserQuestion 自动拒绝；真原生交互需双向流常驻进程（方案 C），颠覆现有一问一进程架构。
**Decision**：A（禁用+声明）+ B（输出约定 + 前端渲染拦截 chips + resume 回传）；否决后端结构化事件方案（B2：需新增事件/模型字段/wailsjs 三处同步，收益不抵成本——原文落盘即含块，历史重载天然兼容）。
**Consequences**：模型守约依赖 prompt（非硬保证），非法块降级为普通代码块显示；chips 已答状态仅本地不落盘；未来若上方案 C，约定格式可平移。

## Out of Scope

- 方案 C（双向 stream-json control protocol 常驻进程）。
- 已答状态持久化 / 多设备同步。
- 后端解析提问块（不改 ChatService 解析与消息模型）。
- AskUserQuestion 原生 UI 复刻（description 折叠、多 tab 等高级交互）。

## Decision (ADR-lite) 补充

- 实现路径用户已确认 B1（前端渲染拦截）。补充硬约束：chips 非模态内联，任何导航（会话/目录/面板切换）不被阻塞（B7-B9）。

## Technical Notes

- fence 渲染拦截：markdown-it `md.renderer.rules.fence` 按 info string 分流；点击交互用容器级事件委托（同 `onChatMarkdownClick` 先例，v-html 内无法绑 Vue 事件）。
- prompt 声明注入点：`RunChat`（service 层）拼 prompt 处按 `ClaudeSessionID` 判首/续轮。
- 答案回传文本格式建议：`用户通过选项卡片回答：\n问题：<question>\n选择：<label1>、<label2>`。
- 相关文件：service/chat_service.go、service/chat_service_test.go、frontend/src/utils/chatMarkdown.js、frontend/src/components/AiChatPanel.vue、frontend/src/stores/（aiChat store 调 RunChat 处）。
