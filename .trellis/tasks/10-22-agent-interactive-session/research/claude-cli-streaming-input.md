# Research: claude CLI 交互式流式多轮会话协议（stream-json input / control 协议 / --resume）

- **Query**: 核实 claude CLI 交互式流式多轮会话能力边界，为 WorkBench「网页版 Claude Code 体验」（运行中插话、权限审批弹窗、持续会话）升级提供协议依据
- **Scope**: mixed（官方文档抓取 + 本机 claude.exe 二进制字符串取证 + 10 组 Python 驱动的实测探针）
- **Date**: 2026-10-22
- **本机版本**: `claude --version` → `2.1.296 (Claude Code)`（winget 安装，exe 约 249MB）
- **实测环境**: Windows 11 + Git Bash + Python 3.13 驱动 `subprocess.Popen`，子进程 cwd 为临时目录

---

## 结论摘要

1. **流式多轮输入模式存在且可用**：`claude -p --input-format stream-json --output-format stream-json --verbose` 即可长驻进程、多次写 stdin 排队多轮消息，**必须配 `-p`**。【已验证】
2. **权限审批可经 stdout/stdin 全双工闭环**：但裸 CLI 不会下发审批请求，**必须追加 `--permission-prompt-tool=stdio`**（官方 TS SDK 在 canUseTool 场景正是传这个参数）；下发 `control_request/can_use_tool`，以 `control_response` 应答 allow/deny，allow/deny 双路径均实测闭环。【已验证】
3. **运行中插话与中断**：多消息自动排队顺序处理【已验证】；`control_request/interrupt` 中断当前执行并返回含 `still_queued` 的回执【已验证】。
4. **--resume 可与流式输入组合**：恢复后 session_id 不变，上下文实测续传（暗号复述验证）。【已验证】
5. **进程生命周期**：stdin 打开则长驻无空闲超时；stdin 关闭后处理完在途轮次退出；每轮一个 `result` 事件，退出码 0/1 区分成败。【已验证 + 文档佐证】

---

## 1. stream-json input 模式

**结论：必须配 `--print/-p`；stdin 每行一个 JSON 用户消息，支持多次写入排队处理；不带 `-p` 是 TUI 交互模式不走此协议。**【已验证】

- `claude --help` 原文：`--input-format <format>` — *"Input format (only works with --print): 'text' (default), or 'stream-json' (realtime streaming input)"*。二进制内亦有字符串 `input-format=stream-json requires --print`。【已验证（help + 二进制取证）】
- **stdin 消息格式**（实测命令与载荷）：

```json
{"type": "user", "message": {"role": "user", "content": [{"type": "text", "text": "只回复两个字：收到"}]}, "parent_tool_use_id": null}
```

  - `content` 亦可为纯字符串；支持 image 块（base64）等 Anthropic content block 形态【文档佐证：[Streaming Input](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode) 的 TypeScript/Python 示例】
  - `parent_tool_use_id` 主会话恒 `null`（实测带不带均被接受）
- **多轮排队**（探针 B）：两条消息间隔 6 秒写入，msg2 在 msg1 处理中进入队列，result#1 后自动处理 msg2，两条 `result` 事件均 `subtype=success`，进程退出码 0。【已验证】
- **`--replay-user-messages`**：将 stdin 收到的用户消息回显到 stdout 用于确认（help + [CLI 参考](https://code.claude.com/docs/en/cli-reference)）。【文档佐证】
- **重要观察**：每个新轮次开始前会重复发出一条 `system/init` 事件（同一 session_id，探针 B 中 t=5.7 与 t=67.3 两条）。前端解析勿假设 init 只出现一次。【已验证（观察）】

### 输出事件形态（实测）

| 事件 | 关键字段 | 备注 |
|---|---|---|
| `system/hook_started`、`system/hook_response` | hook_id、session_id | 本机装有 SessionStart hook 故先于 init 出现；无 hook 时 init 是首事件 |
| `system/init` | **session_id**、model、tools、mcp_servers、**capabilities**、claude_code_version、permissionMode | capabilities 数组可做特性检测 |
| `assistant` | message.content[]（text/tool_use 块） | 逐块下发 |
| `user` | message.content 为 tool_result 块 | 工具结果回传 |
| `system/permission_denied` | tool_name、tool_use_id、message | 权限被拒时出现（见 §2） |
| `system/thinking_tokens` | estimated_tokens(_delta) | 思考量进度 |
| `result` | **subtype、is_error、result、num_turns、duration_ms、usage、queued_turn_count** | 每轮一个；流尾 |

### 官方文档地址

- Headless/CLI 编程使用：https://code.claude.com/docs/en/headless
- CLI 参考（含全部 flag）：https://code.claude.com/docs/en/cli-reference
- 流式输入模式（SDK 层）：https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode

---

## 2. 权限审批协议（control_request / can_use_tool）

**结论：审批请求经 stdout 以 `control_request` 事件下发、经 stdin 以 `control_response` 应答，按 `request_id` 匹配；但裸 CLI 默认不开启，必须加 `--permission-prompt-tool=stdio`；`--permission-prompt-tool` 的 MCP 工具形态是另一条等价路由。**【已验证】

### 2.1 关键开关（实测三次排除法）

| 配置 | 行为 | 出处 |
|---|---|---|
| 裸 CLI stream-json + default 权限模式 | 不下发审批，直接 `system/permission_denied` + `tool_result` 告知需批准，模型自行放弃 | 探针 D 实测 |
| 裸 CLI + 先发 `initialize` 控制请求握手 | 仍不下发（握手本身成功，返回 commands 等元数据） | 探针 F 实测 |
| + `--permission-prompt-tool=stdio` | **完整审批闭环** | 探针 I 实测 |

- `--permission-prompt-tool=stdio` 的来历：官方 TypeScript SDK 源码（unpkg `@anthropic-ai/claude-agent-sdk/sdk.mjs`）在用户注册 canUseTool 回调时向 CLI 追加 `W.push("--permission-prompt-tool=stdio")`，与 `permissionPromptToolName`（MCP 工具名）互斥。【文档佐证（SDK 源码取证）+ 已验证（探针 I）】
- `--permission-prompts host|none`（v2.1.259+）：`host`（默认）表示审批交给 SDK 宿主或 `--permission-prompt-tool`；`none` 表示无人应答直接拒绝。【文档佐证：[headless 文档](https://code.claude.com/docs/en/headless)】

### 2.2 下发结构（探针 I 实测原始载荷摘录）

```json
{"type": "control_request",
 "request_id": "cd577ead-4e0a-4174-869a-8c0f1035abd6",
 "request": {
   "subtype": "can_use_tool",
   "tool_name": "Bash",
   "display_name": "Bash",
   "tool_use_id": "toolu_kRkJNMYtKUjaF67V98Lozc",
   "input": {"command": "mkdir permtest_dir9", "description": "创建目录 permtest_dir9"},
   "description": "创建目录 permtest_dir9",
   "blocked_path": "C:\\...\\permtest_dir9",
   "permission_suggestions": [
     {"type": "addRules", "rules": [{"toolName": "Bash", "ruleContent": "mkdir permtest_dir9"}], "behavior": "allow", "destination": "localSettings"},
     {"type": "addDirectories", "directories": ["C:\\...\\cc-probe"], "destination": "session"},
     {"type": "setMode", "mode": "acceptEdits", "destination": "session"}]}}
```

实测 `request` 键集合：`blocked_path / description / display_name / input / permission_suggestions / subtype / tool_name / tool_use_id`。二进制内 zod schema 另有 `mcp_server`（可选）与 `requires_user_interaction`、`decision_reason` 等可选键。【已验证】

### 2.3 应答格式（stdin，实测闭环）

**Allow**（工具随后真实执行，`tool_result` 无输出、assistant 确认目录已创建）：

```json
{"type": "control_response",
 "response": {"subtype": "success",
              "request_id": "cd577ead-4e0a-4174-869a-8c0f1035abd6",
              "response": {"behavior": "allow", "updatedInput": {"command": "mkdir permtest_dir9", "description": "创建目录 permtest_dir9"}}}}
```

**Deny**（探针 J 实测：`tool_result` 为 `is_error: true`，内容 = 拒绝 message 原文，模型据此调整话术）：

```json
{"type": "control_response",
 "response": {"subtype": "success",
              "request_id": "92585e86-d65c-4d05-87c5-603fd9c90d04",
              "response": {"behavior": "deny", "message": "Probe denial: mkdir is not allowed"}}}
```

- `updatedInput` 支持「修改后批准」（改参后执行，模型不知情）；SDK 文档注明 **v2.1.207 之前 allow 缺 `updatedInput` 会被判校验错误拒绝**，故始终回传原 input。【文档佐证】
- allow 可附 `updatedPermissions`（回传 `permission_suggestions` 子集）实现「批准并记住」——`destination: "localSettings"` 的建议写入 `.claude/settings.local.json` 跨会话生效。【文档佐证：[user-input 文档](https://code.claude.com/docs/en/agent-sdk/user-input)】
- 权限评估顺序（hooks → deny 规则 → ask 规则 → 权限模式 → allow 规则 → canUseTool）：被前置步骤放行的调用**不会**产生 can_use_tool 请求。【文档佐证：[permissions 文档](https://code.claude.com/docs/en/agent-sdk/permissions)】
- `AskUserQuestion` 澄清问题也走同一 can_use_tool 通道（`tool_name: "AskUserQuestion"`），应答结构为 `{questions, answers}`。【文档佐证】

### 2.4 initialize 握手与未决请求重派

- 握手：stdin 发 `{"type":"control_request","request_id":"<id>","request":{"subtype":"initialize","hooks":{}}}` → stdout 回 `control_response`（实测返回 commands 技能清单等元数据）。【已验证】
- 成功的 initialize 响应可带 `pending_permission_requests` 数组（v2.1.268+），列出该进程已发出未解决的 can_use_tool，**宿主重连/重派场景**消费；更早版本可能缺省，自行解析线协议时「缺字段 ≠ 无未决」。【文档佐证：TS SDK 参考】

### 2.5 与 `--permission-prompt-tool`（MCP 形态）的关系

`--permission-prompt-tool mcp__<server>__<tool>` 把审批路由到指定 MCP 工具（CLI 会等该 MCP server 连接后再跑首轮，MCP_TIMEOUT 30 秒）；`stdio` 是其中的内建特殊值，把审批路由到 stdin/stdout 控制协议。两者互斥，一次只能一个。【文档佐证 + SDK 源码取证】

---

## 3. 中断（运行中打断当前执行）

**结论：可以通过 stdin 发 `control_request/interrupt` 打断当前轮次，CLI 返回回执。**【已验证】

实测命令时序（探针 C）：发消息「用 Bash 运行 sleep 60」→ 6 秒后发：

```json
{"type": "control_request", "request_id": "probe-interrupt-1", "request": {"subtype": "interrupt"}}
```

CLI 立即回发（stdout）：

```json
{"type": "control_response", "response": {"subtype": "success", "request_id": "probe-interrupt-1", "response": {"still_queued": []}}}
```

随后被中断轮以 `user` 消息 `[Request interrupted by user]` + `result subtype=error_during_execution, is_error=true` 收尾；进程退出码 1。

- `still_queued` 回执 = `interrupt_receipt_v1` 能力（**v2.1.205+**；更早版本回空成功体）。列出中断到达时尚在排队的用户消息 UUID；客户端可用 `cancel_queued: true` 一并取消排队消息（`interrupt_cancel_queued_v1`，**v2.1.219+**），此时这些消息改列于 `cancelled` 且不执行。【已验证（回执实测）+ 文档佐证（语义细节：[TS SDK 参考](https://code.claude.com/docs/en/agent-sdk/typescript)）】
- 本机 `system/init.capabilities` 实测值：`["interrupt_receipt_v1", "interrupt_cancel_queued_v1", "interrupt_send_now_v1", "msg_lifecycle_v1", "request_marker_lists_v1", "sdk_mcp_tools_list_changed", "sdk_mcp_manifests", "mcp_read_resource_v1", "mcp_tool_ui_meta_v1", "ui_surface_v1"]`。【已验证】

---

## 4. --resume 配合

**结论：交互式流式模式可与 `--resume <session-id>` 组合续上上下文；session_id 在 `system/init` 事件下发。**【已验证】

- 探针 G/H 两阶段验证：phase1 新会话写入暗号「GLM-PROBE-7391」→ phase2 以 `claude -p --resume <sid> --input-format stream-json ...` 恢复并追问「暗号是什么」，模型正确复述 `GLM-PROBE-7391`（该暗号只存在于 phase1 的用户消息中，证明 API 请求携带了历史）。【已验证】
- resume 后 `system/init` 的 `session_id` 与原会话一致（复用原 id，除非加 `--fork-session` 换新 id——【文档佐证】help 文本）。【已验证】
- `--resume` 也可传会话 `.jsonl` 落盘文件绝对路径；会话文件位于 `~/.claude/projects/<cwd 编码目录>/<session_id>.jsonl`，user/assistant 消息逐条入档（实测核验过）。【文档佐证 + 已验证（文件核验）】
- **实测怪癖（与协议无关）**：经代理的首轮流式轮次，haiku 与 sonnet 均出现「忽略首轮指令、回泛泛问候」现象，但消息已入档且被后续轮引用（暗号即证）。WorkBench 集成测试勿以「首轮必须服从指令」作断言。【已验证（观察）】
- 注意：`--resume` 的会话查找自 v2.1.223 起才覆盖本机全部项目目录（此前仅当前项目及 worktree）。【文档佐证】

---

## 5. 进程生命周期

**结论：stdin 打开则长驻、无空闲自动退出；stdin 关闭后处理完在途轮次退出；每轮一个 result 事件；退出码 0/1。**【已验证 + 文档佐证】

| 行为 | 结论 | 置信度 |
|---|---|---|
| 空闲超时/自动退出 | 无（stdin 保持打开进程一直等待下一条消息）。唯一的 10 分钟上限针对「后台工作等待收尾」（`CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS`），非交互空闲 | 文档佐证（headless 文档仅述后台工作收尾；实测 5 组探针 stdin 打开期间进程均稳定长驻） |
| stdin 关闭 | 处理完在途轮次后退出（探针 A：stdin 立即关闭，消息仍被处理并回完 result 再退出） | 已验证 |
| 每轮终态 | 一个 `result` 事件：`subtype=success`（正常）、`error_during_execution`（中断/执行出错）等；**`subtype` 与 `is_error` 需组合判断**——实测认证失败轮 `subtype=success` 但 `is_error=true, result="Not logged in · Please run /login"`、退出码 1 | 已验证 |
| 退出码 | 成功 0；result 出错/中断收尾为 1；SIGTERM 杀进程为 143（SIGINT 才是优雅结束轮次） | 已验证（0/1）+ 文档佐证（143） |
| `--max-turns` | 仅 print 模式；限制代理轮数，超限报错退出。**流式输入下**：「a message still queued when the limit ends a turn stays queued and starts a new turn with its own limit」——排队消息会开新轮并重新计数，不会丢消息 | 文档佐证（[CLI 参考](https://code.claude.com/docs/en/cli-reference)） |
| 输出积压 | 消费端读得慢时 CLI 等待排空再退出，上限 30 秒 | 文档佐证 |
| 崩溃/断连 | stdin 读不到（父端断开）时 CLI 向 stderr 打警告并继续；宿主重连可用 initialize 的 `pending_permission_requests` 重派未决审批 | 文档佐证 |

---

## 6. 版本要求

- **本机实测**：`claude --version` → `2.1.296 (Claude Code)`。【已验证】
- 特性-版本对照（均为【文档佐证】，help/--version 逐条可复核）：

| 能力 | 最低版本 | 建议 WorkBench 门槛 |
|---|---|---|
| `--input-format stream-json`（需 `-p`） | 早于本次调研窗口（长期存在） | — |
| `capabilities` 特性检测数组（system/init） | 2.1.205 | **≥ 2.1.205 硬门槛**（interrupt 回执依赖） |
| interrupt 回执 `still_queued` | 2.1.205 | 同上 |
| interrupt `cancel_queued: true` | 2.1.219 | 建议 ≥ 2.1.219 |
| `--permission-prompts host|none` | 2.1.259 | 建议 ≥ 2.1.259 |
| initialize 响应 `pending_permission_requests` | 2.1.268 | 建议 ≥ 2.1.268 |
| allow 缺 `updatedInput` 被拒 | 2.1.207 前的旧行为 | 恒回传 updatedInput 即可不依赖 |
| `--permission-prompt-tool=stdio` 内建值 | 未见官方版本记载（SDK 源码在用） | 推测待验证（2.1.296 实测可用） |

- 启动时建议按 `claude --version` + init 事件 `capabilities` 双重校验，capabilities 里的未知值应忽略（官方声明其为开放集合）。【文档佐证】

---

## 7. 实测记录（命令与原始输出摘录）

实测驱动方式：Python `subprocess.Popen(["claude","-p","--input-format","stream-json","--output-format","stream-json","--verbose","--model","haiku"], stdin=PIPE, stdout=PIPE, ...)`，逐行读写，10 组探针 transcript 留存于 `C:\Users\liuyang\AppData\Local\Temp\cc-probe\`（exp1~exp10）。

**探针 A（单轮 + stdin 立即关闭）**：

```text
t=0.02  stdin > {"type":"user","message":{"role":"user","content":[{"type":"text","text":"只回复两个字：收到"}]},"parent_tool_use_id":null}
t=7.76  stdout < {"type":"system","subtype":"init","session_id":"e76f872a-...","model":"claude-haiku-5-5","claude_code_version":"2.1.296","capabilities":["interrupt_receipt_v1",...],"permissionMode":"auto",...}
t=14.91 stdout < {"type":"assistant","message":{...,"content":[{"type":"text","text":"收到"}]},...}
t=19.68 stdout < {"type":"result","subtype":"success","is_error":false,"result":"收到","num_turns":1,"duration_ms":15658}
t=exit  code=0
```

**探针 B（两消息排队）**：t=6.02 写入 msg2（此时 msg1 未处理完）→ t=65.03 result#1("1") → t=77.00 result#2("2") → exit 0；期间 t=67.28 出现第二条 `system/init`（同一 session_id）。

**探针 C（interrupt）**：见 §3 原始载荷；result `subtype=error_during_execution, is_error=true`，exit 1。

**探针 I（审批 allow 闭环）**：见 §2.2/§2.3 原始载荷；allow 后 `tool_result` 为 `(Bash completed with no output)`、`is_error=false`，assistant 确认目录已创建，exit 0。

**探针 J（审批 deny 闭环）**：deny 后 `tool_result` 为 `{"content":"Probe denial: mkdir is not allowed","is_error":true}`，模型复述拒绝原因并调整方案，exit 0。

**探针 H（resume 暗号）**：phase2 `result: 'GLM-PROBE-7391'`（暗号仅在 phase1 出现过）。

**实测踩坑记录（WorkBench 无关但值得留档）**：从 Claude Code 会话内部再 spawn claude 子进程时，宿主注入的 `CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST`、`CLAUDECODE` 等环境变量会使子 CLI 走「宿主管控认证」而报 `Not logged in · Please run /login`（result `is_error=true`，exit 1）；需剔除 `CLAUDECODE*`/`CLAUDE_CODE_*` 前缀变量并显式注入认证 env（本机为 `~/.claude/settings.json` 的 `env` 段：`ANTHROPIC_AUTH_TOKEN`/`ANTHROPIC_BASE_URL`）。WorkBench 是独立 Go 进程 spawn，正常桌面环境无此污染，但**若 WorkBench 内嵌终端里再嵌套启动 claude -p 需注意**。

---

## 对 WorkBench 实现的含义

1. **单进程长驻替代每轮一进程**：以 `claude -p --input-format stream-json --output-format stream-json --verbose --permission-prompt-tool=stdio` 一个长驻子进程承载整个会话：Go 侧维护「stdin 写入器 + stdout 逐行 JSON 解析器」双 goroutine；运行中插话=继续写 stdin（自动排队），会话上下文由 CLI 进程内维持，`--resume` 退化为崩溃恢复手段（进程死亡后按 init 捕获的 session_id 重启并恢复，两模型可组合）。
2. **权限审批直接映射现有弹窗**：`control_request/can_use_tool` 的 `request_id` 匹配回写 `control_response`；`input` 渲染审批内容、`updatedInput` 支持「修改后批准」、`permission_suggestions`（addRules→localSettings）支持「批准并记住」——与现有前端 `handleError`/确认弹窗体系对齐后即达成网页版审批体验；**不加 `--permission-prompt-tool=stdio` 时审批会静默自动拒绝，极易漏配**。
3. **中断走协议而非杀进程**：`control_request/interrupt` 得到回执（`still_queued` 可展示被取消的排队消息，`cancel_queued:true` 可一并撤队），被中断轮以 `result subtype=error_during_execution` 收尾；进程强杀（SIGKILL/终止树）保留为兜底与「彻底关闭」路径。
4. **解析层防御性要点**：`system/init` 每轮可能重复出现（勿按一次性事件建状态）；`result` 的 `subtype` 与 `is_error` 必须组合判断（存在 `subtype=success` 且 `is_error=true` 的失败形态）；用 `capabilities` 数组做特性检测而非版本字符串比较；流式渐进输出可加 `--include-partial-messages` 消费 `stream_event` 事件。
5. **版本与启动契约**：启动前校验 `claude --version ≥ 2.1.205`（建议 ≥ 2.1.268），init 事件 `capabilities` 含 `interrupt_receipt_v1` 才启用回执语义；认证 env 从 WorkBench 侧显式注入（不依赖 settings.json env 段在嵌套环境下生效）。

## Caveats / Not Found

- `--permission-prompt-tool=stdio` 的引入版本无官方记载（SDK 源码在用 + 2.1.296 实测可用），更低版本可用性【推测待验证】。
- 官方文档目前把 canUseTool 完整语义写在 Agent SDK 文档（Python/TS），CLI 裸线协议（control_request envelope、`--permission-prompt-tool=stdio`）无独立文档页，本报告以 SDK 文档 + 二进制取证 + 实测三方交叉支撑。
- 本机代理链路（GLM 兼容端点）下模型偶发「首轮泛泛回复」，属模型/代理侧行为，不影响协议结论，但集成测试断言需规避。
- deny 路径的 `interrupt: true` 字段（deny 同时打断当前轮）未实测，仅文档佐证。
