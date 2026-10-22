# AI 对话服务契约（ChatService）

> 任务来源：09-22-ai-aichatbox（AI 对话工作台）；2026-10-22 经 10-22-agent-interactive-session 升级为持续会话进程模型（网页版 Claude Code 体验：运行中插话/权限审批/多轮排队）。本文描述 ChatService 的跨层契约与关键约束，新增对话相关能力前必读。

## 1. 职责边界

| 服务 | 交互范式 | 入口 |
|---|---|---|
| AiFunctionService（AI 功能） | 一次性技能触发（绑定 AiFunction 配置） | AiFunctionPanel |
| ChatService（AI 对话） | 持续会话进程式多轮对话（自由 prompt + 运行中插话 + 工具审批） | AiChatPanel |

两者并行独立，事件前缀隔离：`ai-task:*` 与 `chat-task:*`/`chat-approval:*`，互不干扰。

## 2. 存储布局（data/ai_chat/）

```
data/ai_chat/
├── sessions.json      # 会话索引（含 schemaVersion；损坏备份降级为空，不阻塞启动）
├── directories.json   # 常用目录项（id/path/displayName/sortOrder）
├── templates.json     # 对话模板（scope: global | directory）
├── settings.json      # 执行配置（permissionMode / model）
└── messages/<sessionId>.json   # 每会话消息文件（索引不落 Messages，omitempty）
```

- 全部 JSON 读写遵循既有模式：损坏 → 备份原文件 + slog.Warn + 降级空值，**不阻塞启动**。
- 索引与消息分离：列表页只读索引，消息按需加载。

## 3. 执行链路（持续会话进程模型）

**每会话一个常驻 claude 进程**（不再是每轮一进程）：

```
claude -p --input-format stream-json --output-format stream-json --verbose
       [--resume <sid>] [--permission-mode <m>] [--model <x>]
       --disallowedTools AskUserQuestion --permission-prompt-tool stdio
```

- stdin 写入驱动轮次（stream-json input 协议，**必须配 `-p`**）：`{"type":"user","message":{"role":"user","content":[{"type":"text","text":...}]}}`；stdout 逐行 stream-json 解析（chat 域独立解析器 `parseChatStreamLine`，与 ai_function.go 的 `parseStreamLine` 分域，AI 功能零改动）。
- **运行中插话 = Go 侧 FIFO 排队注入**：RunChat 把轮次追加进会话队列（不拒绝），前序轮 result 后依序写入 stdin（CLI 侧逐轮串行，--resume 上下文与消息文件写入不交错）；排队轮可取消（直接摘除），执行轮取消走 interrupt 协议。
- **权限审批走控制协议**：`--permission-prompt-tool stdio` 开启（**漏配 = 审批静默自动拒绝**，无报错，参数组装单点 `buildChatArgs` 守护、单测恒断言该开关在）。`control_request/can_use_tool` 经 stdout 下发 → 注册审批 + 推 `chat-approval:request` 事件 → 前端弹窗 → `RespondChatApproval(requestID, allow)` 写 stdin `control_response`（allow 恒回传原 input 作 `updatedInput`——旧版 CLI 缺失判校验错误；deny 带拒绝原因）→ 模型继续。轮结束/进程退出时未决审批自动 deny + 推 `chat-approval:cancelled`（前端撤弹窗）。
- **中断走协议不杀进程**：`control_request/interrupt`，被中断轮以 `result subtype=error_during_execution` 收尾（done 标记 canceled，部分回复保留入档），进程存活可继续对话；进程 Kill 仅用于空闲回收/参数重启/关停链。
- **进程生命周期**：spawn 时非阻塞占进程槽位（上限 `chatMaxConcurrent=4`，满报 `E_CHAT_PROCESS_LIMIT` 不排队）；watchdog 扫描——执行轮超 `chatTurnTimeout`（10 分钟）发 interrupt、再超一倍强杀；空闲（无执行轮且队列空）超 `chatIdleTimeout`（10 分钟）强杀回收；权限模式/模型变更且空闲时优雅重启（stdin 关闭→等待退出→超时强杀），忙时沿用旧参数。重启/回收后再次发消息自动 respawn 并 `--resume` 续上下文（session id 持久化在会话元数据）。
- **终态判定**：result 的 `subtype` 与 `is_error` 必须组合判断（存在 `subtype=success` 且 `is_error=true` 的失败形态）；`system/init` 每轮重复出现（勿按一次性事件建模），session_id 从 init/result 事件刷新（rt 层 + 轮次层双写）。
- 消息入档时序：user 消息在 stdin 写入成功后落盘（排队取消/启动失败不留痕）；assistant 回复（含取消轮的部分回复）在 **done 事件之前**落盘——前端收到 done 重读会话即得完整消息。失败轮无回复不留空消息。
- 版本门槛：CLI ≥ 2.1.205（capabilities/interrupt 回执），建议 ≥ 2.1.268。协议细节与实测佐证见任务研究 `.trellis/tasks/10-22-agent-interactive-session/research/claude-cli-streaming-input.md`。
- provider 抽象：MVP 单 claude 实现；多 CLI 扩展点在 processFactory 与参数组装处分派，勿在业务层散落 `if cli == "xxx"`。

### 3.1 选择题交互契约（chat-question）

headless 流式模式下 claude 的 AskUserQuestion 工具不渲染 UI 且被 CLI **自动应答**（模型基于「用户未作答」的错误前提继续输出），故 `--disallowedTools AskUserQuestion` 恒定禁用，改用输出约定承载选择题；工具授权走原生审批弹窗（§3），模型提问走本围栏卡片，**两者并存分工**：

| 环节 | 契约 |
| --- | --- |
| 声明注入 | 仅 claude 会话第一条消息（运行时 `claudeSessionID` 为空，fresh spawn 未收到 init）在 prompt 前拼 `chatQuestionPreamble`；后续轮（含 resume 重启、插话排队轮）不重复注入。**声明只进 claude stdin，落盘 user 消息保持用户原文** |
| 块格式 | 回复中 ` ```chat-question ` 围栏内 JSON：`{"questions":[{"question","options":[{"label","description"?}],"multiSelect"?}]}`，支持一次多问题 |
| 渲染 | `chatMarkdown.js` fence 渲染分流：合法 JSON 渲染交互卡片；非法/缺失结构降级普通代码块（未闭合流式围栏同样容错） |
| 交互 | AiChatPanel 在气泡容器事件委托（v-html 内无法绑 Vue 事件）：单选 radio 语义、多选 toggle，每题至少一项启用提交；提交拼「用户通过选项卡片回答」文本走 `runChat` 常规链路（运行中提交合法——排队注入，答案轮次前序轮 result 后执行） |
| 已答态 | aiChat store 内存 map（key `sessionId:taskId`），切会话往返保留置灰，**不落盘**（重启恢复未答可再点）；消息模型零变更 |
| 守卫 | 已答卡片置灰禁点；运行中提交不再拦截（原 chatInFlight 提交守卫随排队注入退役） |

改动注意：`chatQuestionPreamble` 文案变更会影响模型守约行为，改后须实测首轮提问；卡片 class 属性被 `renderMsgMarkdown` 字符串替换精准追加已答类，勿改 `class="chat-question"` 的输出形态。

## 4. 前端状态与事件纪律（aiChat store）

- 单一 pinia store `aiChat`（目录/会话/消息/任务/模板/配置六域）。
- **任务域为轮次注册表 `chatTasks`（taskId → 轮态）**：持续会话模型下同会话可有多条在途轮次（插话排队），事件按 taskId 对号；`currentSessionChatTasks` 派生当前会话在途列表（多条流式气泡逐条渲染，插入序 = 提交先后）。`chatInFlight` 语义收窄为「当前会话有在途轮次」，仅驱动停止按钮/状态条展示，**不再禁发消息**（发送 = 排队注入）。
- 乐观态判定：会话无在途轮次 → 置 running（空闲路径 started 事件先于 RunChat resolve 到达被忽略）；已有在途轮次 → 置 queued（started 事件推进时正常对号）。
- `chat-task:output` 增量追加对应轮次回复；done 后摘除该轮 + 按 `GetChatSession` 重载定型（乐观态被持久化数据覆盖，前后端一致）。
- 面板切走再切回：`restoreChatTaskState` 经 `ListChatActiveTasks('')` 全量重建注册表；本地有而后端查无的轮次（done 丢失）摘除并重载其会话。
- **切换目录/会话不再确认取消**（持续会话模型：后台会话继续运行）；删除会话前 store 自动取消该会话全部在途轮次（防删除后进程写孤儿消息文件）。
- **审批弹窗**：`chat-approval:request` → FIFO 队列串行弹窗（ElMessageBox 单实例）→ 允许/拒绝 → `RespondChatApproval`；`chat-approval:cancelled` 撤队列待弹项（展示中的弹窗保留，应答时后端报「已失效」静默降级）。
- EventsOn 返回闭包在 onBeforeUnmount 逐一注销，**禁 EventsOff**（多面板共存）。
- 面板 v-show 常驻不卸载 → 事件可能丢失 → `watch(activePanel)` 切回时 `restoreChatTaskState` 兜底恢复。

## 5. 装配同步点（新增 service 通用教训）

新增带事件的服务（不只 ChatService）时，**必须同步 `web_serve.go` 的 `applySink`**：补 `SetEventSink` 行，否则浏览器通道（/api 或 --serve 模式）收不到该服务全部事件——桌面端正常、浏览器端静默失效，极易漏测。当前 applySink 覆盖八服务：terminal / update / aiFunc / git / chat / pptxSplit / assets / pptxMerge。

## 6. 错误码

| 码 | 语义 |
|---|---|
| E_CHAT_SESSION_NOT_FOUND | 会话不存在 |
| E_CHAT_EMPTY_PROMPT | 空 prompt |
| E_CHAT_PROCESS_LIMIT | 同时活跃的对话会话进程已达上限（4），新会话无法启动（warning） |
| E_CHAT_IN_PROGRESS | （已退役）原同会话串行拒绝；持续会话排队注入模型下 RunChat 不再产生，常量与前端映射保留兼容 |

三处同步：model/app_error.go 常量表 + frontend/src/utils/error.js + docs/API参考.md。

## 7. 权限与模型

- permissionMode：default / acceptEdits / plan / bypassPermissions（claude 原生 `--permission-mode` 取值），默认 default；前端 bypassPermissions 选中即 ElMessage.warning。**spawn 快照语义**：变更且进程空闲时优雅重启生效；进程忙（有执行/排队轮）时本批轮次沿用旧参数，下一批 spawn 生效。
- model：空串 = claude 默认模型（不加 --model 参数）；前端 el-select allow-create 支持自定义模型 ID；快照语义同 permissionMode。
- 审批链路与 permissionMode 正交：bypassPermissions 下 CLI 不下发 can_use_tool（审批弹窗自然不出现）；default/acceptEdits/plan 下未命中放行规则的敏感工具均走审批弹窗。

## 8. App 绑定（对话执行域）

| 方法 | 签名 | 说明 |
|---|---|---|
| RunChat | `(chatSessionID, prompt, permissionMode, modelName) => (taskID, error)` | 提交一轮对话（运行中再发 = 排队注入）；返回轮次 id |
| CancelChatTask | `(taskID) => bool` | 取消轮次（执行中 interrupt / 排队中摘除） |
| ListChatActiveTasks | `(chatSessionID) => ChatTaskState[]` | 列出会话在途轮次（空 id = 全部会话），面板恢复用 |
| GetChatTaskState | `(taskID) => ChatTaskState?` | 查询单轮次状态（兼容保留） |
| RespondChatApproval | `(requestID, allow) => error` | 应答工具授权审批（allow 回传原 input 作 updatedInput） |

## 9. 事件契约

| 事件 | 载荷 | 时机 |
|---|---|---|
| chat-task:queued | `{taskId, chatSessionId}` | 轮次接受入队 |
| chat-task:started | `{taskId, chatSessionId}` | stdin 写入成功开始执行 |
| chat-task:output | `{taskId, chatSessionId, text}` | assistant 文本增量 |
| chat-task:done | `ChatTaskRunResult` | 轮次终态（先落盘后发） |
| chat-approval:request | `ChatApprovalRequest` | can_use_tool 下发 |
| chat-approval:cancelled | `{requestId, chatSessionId, reason}` | 审批自动失效（轮结束/进程退出） |

## 10. 多会话并行（PR2）

- **并行能力为架构原生**：sessions 注册表按 chatSessionID 键控，两会话各一常驻进程独立推进；事件按 taskId 对号 + chatSessionId 维度标注，前端注册表 `chatTasks` 跨会话共享、视图按 `currentSessionChatTasks` 过滤（后台会话流式气泡不入当前视图）。隔离单测：TestChatService_MultiSessionParallel（交错输出/落盘/claude session id 回写三面互不串流）。
- **资源上限与回收**：进程槽位上限 chatMaxConcurrent=4（新会话首条消息超限报 E_CHAT_PROCESS_LIMIT）；空闲超时/参数重启/关停链三条回收路径见 §3。
- **删除会话 = 进程回收**：DeleteChatSession 先摘索引再 killSession（在途轮次失败收尾、未决审批清理、槽位归还由 pump 收尾统一处理）；进程不存在时无操作。前端删除前另取消该会话全部在途轮次（双保险，防孤儿消息文件复活）。
- **断线恢复复核结论**：浏览器 ws-gap（缓冲溢出/服务端重启）→ connBanner 提示 + 各面板既有冷启动链路补齐（既有约定）；chat 的对应兜底 = 面板切回时 `restoreChatTaskState`（ListChatActiveTasks 全量重建跨会话轮次注册表），不新增 transport 级钩子。多会话并行放大帧量仅提高 ws-gap 概率，恢复链不变。
