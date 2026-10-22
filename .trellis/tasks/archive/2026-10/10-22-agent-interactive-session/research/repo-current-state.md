# Research: AI 对话链路现状梳理（为 Agent 交互式多轮会话改造做准备）

- **Query**: 梳理 WorkBench 中 AI 对话链路现状（ChatService / 前端消费面 / WS 事件分发 / App 绑定），为「持续会话进程 + stdin 流式输入 + 权限审批事件」改造做影响面初判
- **Scope**: internal（先读 docs/spec/ai-chat-service.md 与 docs/spec/browser-channel.md，再对照代码核实）
- **Date**: 2026-10-22
- **对应 PRD**: `../prd.md`（Agent 交互式多轮会话，子任务 3/3）

> 说明：实际代码根为仓库根 `D:\workspace\workspace_ai\demo_OpenSpec\git_tools\git-manager\`（下文 file:line 均相对仓库根）。

---

## 1. 总览

| 层 | 文件 | 职责 |
|---|---|---|
| service | `service/chat_service.go`（1050 行） | 会话/消息/目录 CRUD + RunChat 执行器 + 事件推送 |
| service | `service/chat_config.go`（288 行） | 模板 CRUD + 执行配置（权限模式/模型） |
| service | `service/ai_function.go:1082` | `parseStreamLine`（chat 复用的 stream-json 解析单点） |
| model | `model/chat_session.go`（112 行） | ChatMessage/ChatSession/ChatTaskState/ChatTaskRunResult/ChatDirectory/ChatTemplate/ChatSettings |
| App 绑定 | `app_chat.go`（186 行，22 个方法） | Wails/RPC 暴露面 |
| 前端 store | `frontend/src/store/aiChat.js`（500 行） | 六域状态（目录/会话/消息/任务/模板/配置 + 选择题卡片域） |
| 前端面板 | `frontend/src/components/AiChatPanel.vue`（1677 行） | UI + chat-task:* 事件监听 + chat-question 卡片交互 |
| WS 通道 | `server/ws.go`（481 行）+ `web_serve.go` applySink | 服务事件经 EventSink 广播给浏览器 |
| 前端 transport | `frontend/src/transport/events.js`（392 行） | 浏览器事件桥（重连/seq 续传/同步模式） |

两份 spec 与代码核实结论一致（`docs/spec/ai-chat-service.md`、`docs/spec/browser-channel.md`），差异点：spec §5 写「applySink 覆盖五个服务」已过时，代码现为**八个**（`web_serve.go:198-221`，chat 在第 217 行）。

---

## 2. service/chat_service.go 全貌

### 2.1 ChatService struct 字段清单（`service/chat_service.go:164-173`）

```go
type ChatService struct {
    ctx        context.Context
    sinkHolder        // 事件出口持有器（embed），构造注入 + serve 模式经 SetEventSink 切换
    dataDir    string // 存储根目录 data/ai_chat/
    mu         sync.Mutex
    tasks      map[string]*chatTaskRuntime
    concurrencySem chan struct{}      // 全局并发信号量，缓冲 = chatMaxConcurrent(3)
    processFactory chatProcessFactory // 子进程工厂（测试注入 fake）
}
```

- `chatMaxConcurrent = 3`（chat_service.go:30）——**全局**并发上限，与会话数无关；改造为多会话并行时这是资源上限的现成挂点。
- `chatDefaultTimeoutMinutes = 10`（chat_service.go:26）——单轮超时，`context.WithTimeout` 起算于**取得槽位后**（chat_service.go:769-770，排队不侵蚀预算）。
- 任务 id：`newChatTaskID()` = `chattask-<unixnano>-<seq>`（chat_service.go:38-43，atomic 序号防同 tick 覆盖）。

### 2.2 chatTaskRuntime（chat_service.go:128-146）

```go
type chatTaskRuntime struct {
    id              string
    chatSessionID   string          // 所属 WorkBench 会话 id
    prompt          string
    claudeSessionID string          // stream 事件提取
    reply           strings.Builder // 本轮回复累积（持锁写）
    proc            chatProcess
    ctx             context.Context
    cancel          context.CancelFunc
    running / canceled / queued bool
    queuedAt / startedAt / finishedAt time.Time
    timeoutMin      int
    errText         string
    queueCancel     chan struct{}   // 排队取消信号：close 唤醒 RunChat 的 select
}
```

### 2.3 chatProcess 接口（fake 注入面，chat_service.go:61-76）

```go
type chatProcess interface {
    StdoutPipe() (io.ReadCloser, error)  // stderr 已并入该流
    Start() error
    Wait() error
    Kill()                               // 整进程树杀（killProcessTree）
    ExitCode() int                       // 未退出返回 -1
}
type chatProcessFactory func(ctx context.Context, name string, args []string, dir string) chatProcess
```

- 生产实现 `execChatProcess` 包装 `exec.Cmd`（chat_service.go:79-121）：`cmd.Cancel` 定制为 `killProcessTree`（chat_service.go:90-93），`util.HideCommandWindow` 隐藏窗口。
- 注入点 `setProcessFactory`（chat_service.go:189-191，小写未导出，仅包内测试用）。
- **改造要点**：接口只有 stdout 单向读——无 stdin 写入口、无 stderr 独立流；持续会话进程 + stdin 注入需扩接口（StdinPipe/其等价物）。

### 2.4 RunChat 完整流程（chat_service.go:700-832）

```
RunChat(chatSessionID, prompt, permissionMode, modelName) → (taskID, error)
 1. 空 prompt 拒绝 E_CHAT_EMPTY_PROMPT（:701-703）
 2. GetChatSession 校验会话存在（:704-707）
 3. 同会话串行保护：检查-注册同一 s.mu 临界区，已存在该会话 running||queued 任务
    → E_CHAT_IN_PROGRESS（:721-729）；任务以 queued=true 入 s.tasks
 4. emit chat-task:queued {taskId}（:730）
 5. 三路 select 排队（:736-748）：concurrencySem 槽位 / queueCancel close / s.ctx.Done
 6. 取槽位后二次查 canceled（:751-757）；queued=false→running=true 同临界区翻转
    （:762-765，防双 false 窗口绕过串行保护）
 7. emit chat-task:started {taskId, chatSessionId}（:766）
 8. 创建执行 ctx（10 分钟超时，:769-770）
 9. 首轮（sess.ClaudeSessionID == ""）prompt 前拼 chatQuestionPreamble（:775-778）；
    声明只进 claude prompt，落盘 user 消息保持用户原文
10. buildChatArgs 组参 → processFactory(ctx, "claude", args, sess.Cwd)（:779-780）
11. StdoutPipe/Start 失败：复位标志、摘除任务、归还槽位、同步报错（:782-802）
12. Start 成功 → user 消息即刻入档 appendMessages（:805-814，落盘失败仅 log 不阻断）
13. 补挂 proc/ctx/cancel + lateCanceled 补杀（启动窗口内的取消，:816-828）
14. go pumpChatOutput(task, proc, stdout)（:830）
```

事件序列：`chat-task:queued` → `chat-task:started` → `chat-task:output`（增量）→ `chat-task:done`。

### 2.5 buildChatArgs 参数组装（chat_service.go:675-689，纯函数）

```go
func buildChatArgs(prompt, resumeSessionID, permissionMode, modelName string) []string
// -p <prompt> [--resume <sid>] --output-format stream-json --verbose
// [--permission-mode <mode>]（仅非 default 非空）[--model <model>]（仅非空）
// --disallowedTools AskUserQuestion（恒定追加）
```

- 每轮一进程模型：`-p`（print/headless）单轮执行完退出。改造为持续会话进程时这里是参数形态的主要变更点（spec ai-chat-service.md §3 注明 provider 分派点在 processFactory 与参数组装处）。

### 2.6 pumpChatOutput 流式读取→落盘→done（chat_service.go:918-1049）

- 逐行 Scan（单行上限 4MB，:924），复用同包 `parseStreamLine`（ai_function.go:1082）：
  `func parseStreamLine(line string) (text string, isResult bool, sessionID string, metrics *model.AiTaskMetrics, structuredOutput json.RawMessage, resultError string)`——chat 只消费 text/isResult/sessionID/resultError 四个返回值（chat_service.go:930）。
- 状态写回持 `s.mu`，emit 放锁外（:934-949）；`chat-task:output` 载荷 `{taskId, chatSessionId, text}`。
- 退出后（:961-1002）：超时判定先读 `ctx.Err()` 再 cancel；错误优先级 canceled > timedOut > claudeErr > waitErr/scanErr；附 ExitCode。
- **落盘时序（先落盘后 done）**：
  1. assistant 回复非空才入档 `appendMessages`（:1012-1023；取消轮保留部分回复，失败轮无回复不留空消息）；
  2. `claudeSessionID` 非空经 `updateSessionMeta` 回写会话元数据（:1026-1039）——**session_id 回写点**，会话在途被删则 Warn 降级（仅影响下轮 resume）；
  3. 最后 `emit chat-task:done`（ChatTaskRunResult，:1041）——保证前端收到 done 时 `GetChatSession` 已能读到完整消息；
  4. 终态任务从 `s.tasks` 摘除（:1046-1048，防内存无界；前端对「查无此任务」按终态处理）。

### 2.7 CancelChatTask（chat_service.go:841-874）

- 两态：排队中 → `close(task.queueCancel)` 唤醒 RunChat select 自清 + 立即 emit `chat-task:done{canceled:true}`；运行中 → 标记 canceled + `proc.Kill()`（pumpChatOutput 末尾统一构造 done）。
- 幂等：`task.canceled` 已置位直接返回 true（防 double-close panic 与重复 done）。
- 启动窗口（queued 翻转后、proc 赋值前）取消由 RunChat 的 `lateCanceled` 检查补杀（:822-828）。

### 2.8 其他

- `GetChatTaskState`（:878-895）：锁内快照转 `model.ChatTaskState`，任务不存在返回 nil（前端按终态处理）。
- `CloseAll`（:898-913）：遍历 tasks 逐个 cancel + Kill（app 关停链调用）。
- 持久化：全部经 `saveChatJSON`（:49-55，.bak 保险 + `util.SaveJSON` 原子写）；索引损坏降级空索引不阻塞（loadIndex :220-244）；消息文件损坏降级空数组（readMessages :615-636）；`appendMessages` 读-合并-整体写回（:639-651）。
- 同会话串行保护的不变式：`running || queued` 双标志无「双 false 窗口」（:758-764 注释）。

---

## 3. 前端消费面

### 3.1 aiChat store（`frontend/src/store/aiChat.js`）

**chat-task:* 事件 handler 清单**（store 内定义，由面板 EventsOn 绑定，全部按 taskId 对号）：

| handler | 行 | 行为 |
|---|---|---|
| `onChatTaskQueued(ev)` | :322-327 | 仅补排队态，不把已 running 任务回退 |
| `onChatTaskStarted(ev)` | :330-334 | 置 running（幂等） |
| `onChatTaskOutput(ev)` | :337-341 | `task.reply += ev.text`（流式累积） |
| `handleChatTaskDone(result)` | :349-359 | 清 chatTask → `loadChatSessions()` → done 会话为当前选中时 `selectChatSession` 重载定型 |

**关键状态**：

- `chatTask` ref（:230）：单任务槽 `{ taskId, chatSessionId, status: 'queued'|'running', reply, prompt }`；`chatInFlight` computed（:233）= `!!chatTask`。**单在途任务模型**——多会话并行改造需改为按会话维度的 map/列表。
- `runChat(prompt)`（:246-277）：RunChat resolve 即置 running（乐观态，queued 事件仅兜底不回退）+ 就地追加 user 气泡 + 首条消息自动生成标题（前 20 字）。
- `cancelChatTask()`（:280-289）：排队取消立即清任务态；运行中等 done 事件收尾。
- `restoreChatTaskState()`（:295-317）：事件丢失兜底，`GetChatTaskState` 拉取覆盖；任务不存在（后端重启/终态摘除）清任务态防卡死。
- **会话列表切换现状**：`selectChatSession`（:171-189）切 id → `GetChatSession` 加载消息，迟到响应按 `selectedChatSessionId !== id` 丢弃；在途任务不属于当前会话时流式气泡不渲染（`chatTaskInCurrentSession` :236-238）。
- 选择题卡片域：`answeredChatQuestions` 内存 map（key `${sessionId}:${taskId}`，:405-417），切会话往返保留、重启恢复未答可再点，不落盘。

### 3.2 AiChatPanel.vue（`frontend/src/components/AiChatPanel.vue`）

**chat-task:* 事件监听清单**（onMounted 同步注册，:945-951）：

```js
offQueued  = EventsOn('chat-task:queued',  aiChatStore.onChatTaskQueued)
offStarted = EventsOn('chat-task:started', aiChatStore.onChatTaskStarted)
offOutput  = EventsOn('chat-task:output',  aiChatStore.onChatTaskOutput)
offDone    = EventsOn('chat-task:done',    onTaskDone)   // :935-943，done 后按 canceled/error 分流 ElMessage
```

- onBeforeUnmount 逐一调 off 闭包注销（:963-972），禁 EventsOff（多面板共存）。
- 面板 v-show 常驻不卸载 → 切回 `activePanel === 'ai-chat'` 时 watch 触发 `restoreChatTaskState()` 兜底（:975-979）。

**chatInFlight 守卫分布**：

- 发送：`handleSend` 进行中禁发（:798，按钮已切停止，双保护）+ `sending` 重入保护（:788-799）。
- 选择题提交：`submitChatQuestionCard` 在途静默忽略（:719），CSS `.chat-in-flight` 置灰承担视觉态。
- 切目录/切会话/新建/删除：`confirmDiscardInFlight`（:423-441）——ElMessageBox 确认「取消任务并继续」，取消失败则中止动作。

**chat-question 卡片提交链路**（:681-741）：

1. v-html 气泡容器事件委托（`onChatMarkdownClick` :648-659 先分流卡片再处理链接）；
2. 选项点选：单选 radio 语义 / 多选 toggle，`syncCardSubmitState` 每题至少一项才启用提交；
3. 提交：拼「用户通过选项卡片回答：」文本 → `aiChatStore.runChat()` 常规链路（--resume 回传模型）→ `markChatQuestionAnswered` + DOM 置灰类；
4. 已答渲染：`renderMsgMarkdown`（:665-674）对 `class="chat-question"` 字符串替换追加 `chat-question-answered`（依赖 renderer 输出形态固定，spec 明示勿改）。

---

## 4. WS 事件分发（浏览器通道）

### 4.1 事件出口链路

```
ChatService.emitCurrent("chat-task:*", ...)         （sinkHolder 持 RLock 投递）
  → 桌面：wailsEventSink → Wails runtime 事件
  → serve/同开：multicastSink { wailsSink, WSHub }   （web_serve.go:203-221 applySink）
      → WSHub.Emit：seqMu 临界区内「占号-入环-广播」（server/ws.go:153-183）
          → 帧 {"event":"chat-task:output","data":[...],"seq":N}
          → 各 wsClient.send 队列 → writePump 单写者落 conn
```

- `applySink`（web_serve.go:198-221）覆盖八服务（terminal/update/ai_function/git/**chat**/pptxSplit/assets/pptxMerge，chat 在 :217）；启动时 `applySink(hub)`（:80），停机 `hub.Close()` 后 `applySink(nil)`（:124-129）。
- WSHub 断线续传（2c8e5f8 刚收口）：环形缓冲 2000 帧（ws.go:43）、resume 握手 `{"resume":lastSeq}` 唯一业务上行（ws.go:391-399，readLimit 4096）、补发经同一 send 队列 FIFO、断层 ws-gap / 追平 ws-synced 控制帧（ws.go:48-57）。

### 4.2 chat-task:* 在浏览器通道的覆盖方式

- **零特判**：chat 事件与其他服务事件同构——emitCurrent → hub 广播 → 前端 transport 按事件名分发。chat-task:* 事件载荷均为可 JSON 序列化的 map/struct，无 func/channel，不会触发 Emit 的 marshal 失败回退分支。
- 前端 `events.js`（`frontend/src/transport/events.js`）：
  - 带 seq 帧实时模式落地推进 lastSeq（:264-269）；同步模式（resume 后）先进 syncBuffer，ws-synced 后按 seq 升序去重落地（:245-289）——**流式 output 增量的有序性由此保证**；
  - ws-gap 断层：重置基准 + connBanner 提示，靠「各面板既有冷启动/状态查询链路」补齐（:306-312）——chat 的对应兜底正是 `restoreChatTaskState()`（GetChatTaskState 轮询恢复）；
  - 桌面模式回调收变参 `...data`，与 `emitCurrent(name, map)` 单载荷形态一致（chat 的四个事件载荷都是单 map 参数）。
- 多会话并行会放大断线场景（PRD 已注明）：2000 帧缓冲溢出 → ws-gap 概率上升；chat 在途任务恢复依赖 `GetChatTaskState`，而**终态任务已从 s.tasks 摘除**（chat_service.go:1046-1048）——ws-gap 后若 done 事件丢失，前端只能靠「任务查无 → 清态 → 重载会话」恢复（store :300-313 已覆盖该形态）。

---

## 5. App 绑定（app_chat.go，22 个方法）

| 方法 | 行 | 签名要点 |
|---|---|---|
| CreateChatSession | :13 | (directoryID, title, cwd) → *ChatSession |
| ListChatSessions | :22 | (directoryID) → []*ChatSession（不含消息） |
| GetChatSession | :35 | (sessionID) → *ChatSession（含消息） |
| DeleteChatSession | :43 | (sessionID) → error |
| UpdateChatSessionTitle | :51 | (sessionID, title) → error |
| **RunChat** | :63 | (chatSessionID, prompt, permissionMode, modelName) → (string, error) |
| **CancelChatTask** | :71 | (taskID) → bool |
| **GetChatTaskState** | :80 | (taskID) → *ChatTaskState（可 nil） |
| AddChatDirectory | :89 | (path, displayName) → *ChatDirectory |
| ListChatDirectories | :97 | () → []*ChatDirectory |
| UpdateChatDirectory | :109 | (id, displayName) → error |
| RemoveChatDirectory | :117 | (id) → error |
| ReorderChatDirectories | :125 | (ids []string) → error |
| AddChatTemplate | :134 | (scope, directoryID, name, content) → *ChatTemplate |
| ListChatTemplates | :143 | (directoryID) → []*ChatTemplate |
| UpdateChatTemplate | :155 | (id, name, content) → error |
| RemoveChatTemplate | :164 | (id) → error |
| GetChatSettings | :172 | () → *ChatSettings |
| SaveChatSettings | :180 | (permissionMode, modelName) → error |

全部为 chatSvc 的 nil 守卫直委托。装配：`app_services.go:53` 字段 + `:175` 构造 `service.NewChatService(ctx, filepath.Join(dataDir, "ai_chat"))`。这些方法同时是浏览器 /api/rpc 的可达面（reflect MethodByName，browser-channel.md §2.2）。

---

## 6. 改造影响面初判（持续会话进程 + stdin 流式输入 + 权限审批事件）

> 仅列结构判断，不做设计。三层的「要动 / 可复用」按现状代码事实归类。

### 6.1 后端 service 层

| 项 | 判断 | 依据 |
|---|---|---|
| chatProcess 接口 | **要动** | 现接口仅 StdoutPipe/Start/Wait/Kill/ExitCode（chat_service.go:61-72），无 stdin 写入口；持续会话进程需 StdinPipe（或等价写通道）+ 更长生命周期语义 |
| processFactory 注入点 | **可复用** | 签名 `(ctx, name, args, dir) → chatProcess`（:76），fake 注入面已立；stdin 扩展后 fake 需同步扩展 |
| buildChatArgs | **要动** | 现为每轮一进程的 headless `-p` 形态（:675-689）；持续会话为流式 input 模式，参数形态不同（且 `--disallowedTools AskUserQuestion` 与权限审批链路的关系需理清——PRD Requirements 2） |
| tasks map + chatTaskRuntime | **要动** | 现为「每轮任务」粒度、终态即摘除（:1046-1048）；持续会话需要「会话进程」粒度的长生命周期注册表，摘除语义改为会话关闭/超时回收 |
| concurrencySem（上限 3） | **可复用（语义位移）** | 现按「每轮进程」计数，持续会话下自然变为「并行会话进程」计数，chatMaxConcurrent 常量含义重定义即可 |
| 同会话串行保护（E_CHAT_IN_PROGRESS） | **要动/放宽** | 现为检查-注册同临界区拒绝（:721-729）+ queued/running 双标志无窗口不变式（:758-764）；「运行中插话」要求该保护放宽为排队注入，注入排队结构需新建 |
| RunChat 流程 | **要动** | 排队→启动→流读→落盘→done 的单轮闭环（:700-832）改为：进程常驻 + 每轮消息写入 stdin + 响应流多轮多路解析 |
| pumpChatOutput | **要动（解析层可复用）** | Scan + parseStreamLine 复用（ai_function.go:1082 是同包单点）；但「单轮 result 即退出落盘」的收尾语义、done 事件粒度需按多轮重排 |
| 消息落盘时序（先落盘后 done） | **可复用（契约不变）** | appendMessages + updateSessionMeta(ClaudeSessionID) 两步（:1012-1039）；持续会话下每轮仍须维持「前端收到 done 时 GetChatSession 可读完整消息」 |
| session_id 回写点 | **可复用** | updateSessionMeta 回写 ClaudeSessionID（:1026-1039），--resume 链路现成（PRD Requirement 4） |
| CancelChatTask | **要动** | 两态取消（:841-874）需扩展为「杀会话进程」语义；若加排队注入取消则再添一态 |
| CloseAll | **可复用（需覆盖新注册表）** | 遍历 cancel+Kill 的形态（:898-913）不变，遍历对象随注册表结构变化 |
| chat-question 块契约 | **可复用（扩展点）** | 前导声明 chatQuestionPreamble（:660-664）+ 前端围栏渲染/提交链路现成；权限审批事件若复用该围栏需扩块类型 |
| 落盘/降级/原子写基建 | **可复用** | saveChatJSON、索引损坏降级、appendMessages 读-合并-写回均与执行模型解耦 |

### 6.2 前端 store + panel

| 项 | 判断 | 依据 |
|---|---|---|
| chatTask 单任务槽 + chatInFlight | **要动** | `ref(null)` 单槽（aiChat.js:230-233）是全链路守卫的根基（handleSend :798、submitChatQuestionCard :719、confirmDiscardInFlight :424、restoreChatTaskState :296）；多会话并行需按会话维度改 map，chatInFlight 语义拆分 |
| 四个事件 handler 的 taskId 对号 | **要动（模式可复用）** | 现按 `task.taskId !== ev?.taskId` 单任务对号（:322-341）；多会话并行 + 事件命名空间扩展会话维度后，对号键与载荷形状随之变（PRD Requirement 3） |
| runChat 乐观态 + user 气泡就地追加 | **可复用（需去串行耦合）** | 乐观置 running、首条消息自动标题（:246-277）与单任务模型耦合，剥离后逻辑可平移 |
| restoreChatTaskState 兜底链 | **可复用（需扩展）** | watch(activePanel) 切回恢复（AiChatPanel.vue:975-979）+ GetChatTaskState 查询形态现成；多会话需遍历恢复 |
| selectChatSession 迟到响应丢弃 | **可复用** | 代次守卫形态（aiChat.js:179-181, 146）与多会话兼容 |
| chat-question 卡片交互链 | **可复用** | 事件委托/单多选/提交回传/已答置灰（AiChatPanel.vue:681-741）独立于执行模型；权限审批 UI 可复用其弹窗/置灰/提交形态 |
| confirmDiscardInFlight（切走前确认取消） | **要动（语义复核）** | 现在切会话必须取消在途任务（:423-441）；持续会话并行后「切走不取消」成为预期，该确认链可能整体撤销 |
| store 事件监听注销纪律 | **可复用** | EventsOn 闭包精准注销（AiChatPanel.vue:945-972）、禁 EventsOff，不变 |

### 6.3 WS 通道

| 项 | 判断 | 依据 |
|---|---|---|
| applySink chat 行 | **可复用（零改动）** | chat 已在八服务切换清单（web_serve.go:217）；新增事件名自动覆盖 |
| WSHub.Emit 广播 + seq 续传 | **可复用（零改动）** | 事件名无关的通用广播；chat-task:* 新子事件无需改 ws.go |
| seq 单调 + 同步模式有序落地 | **可复用** | events.js 对流式增量的按 seq 升序落地（:277-289）天然支撑多轮流式；多会话并行只放大帧量（2000 帧缓冲溢出 → ws-gap 概率上升），恢复链已有 |
| 权限审批的「应答回写 stdin」 | **要动（新上行通道）** | 现浏览器上行仅 resume 握手一帧且 readLimit 4096（ws.go:39, 391-399）；审批应答若走 WS 上行则打破「唯一业务上行」假设（终端输入走 /api/rpc 的先例可参照） |
| ws-gap 后 chat 状态恢复 | **可复用（粒度复核）** | 依赖 GetChatTaskState + 查无即终态清态；持续会话下「任务查无 ≠ 会话终止」，恢复语义需重新定义 |
| 桌面/浏览器双端口径 | **可复用** | 事件载荷单 map 变参形态两端一致（events.js dispatch :224-238）；新增事件保持该形态即零分叉 |

---

## 7. Caveats / 未覆盖项

- `AiChatPanel.vue` 模板段（:1-380）与样式段未逐行核对（与本任务相关的脚本逻辑已全覆盖）；chat-question 卡片的 DOM 结构（`data-task-id`/`data-question`/`data-multi` 属性）由 `frontend/src/utils/chatMarkdown.js` 渲染产生，本次未读该文件（spec 已描述其 fence 分流契约）。
- `parseStreamLine` 仅核对签名与调用点（ai_function.go:1082、chat_service.go:930），其内部对 stream-json 各事件类型的字段提取未展开——改造若需新增事件类型（如 permission 请求）的消费，需另行核读该函数体。
- spec `ai-chat-service.md` §5「五个服务」与代码「八个服务」存在文档滞后（代码为准，web_serve.go:198 注释已更新为八服务）。
- CLI 交互协议（stream-json input 模式、permission 请求事件格式）属外部调研，在 `research/claude-cli-streaming-input.md`（PRD 注明调研中），本文不涉及。
