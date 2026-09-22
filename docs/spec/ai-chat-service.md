# AI 对话服务契约（ChatService）

> 任务来源：09-22-ai-aichatbox（AI 对话工作台）。本文描述 ChatService 的跨层契约与关键约束，新增对话相关能力前必读。

## 1. 职责边界

| 服务 | 交互范式 | 入口 |
|---|---|---|
| AiFunctionService（AI 功能） | 一次性技能触发（绑定 AiFunction 配置） | AiFunctionPanel |
| ChatService（AI 对话） | 多会话持续式多轮对话（自由 prompt） | AiChatPanel |

两者并行独立，事件前缀隔离：`ai-task:*` 与 `chat-task:*`，互不干扰。

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

## 3. 执行链路（RunChat）

```
RunChat(sessionID, prompt, permissionMode, model)
  → 同会话串行保护（检查-注册同一锁临界区，并发双入队拒绝 E_CHAT_IN_PROGRESS）
  → 任务队列（queued → started → done，经 EventSink emitCurrent）
  → buildChatArgs：-p <prompt> [--resume <sid>] [--permission-mode <m>]（非 default 才追加）[--model <x>]（非空才追加）--output-format stream-json --verbose
  → chatProcess 接口（生产 exec.Cmd / 测试 fake 注入，processFactory 函数注入点）
  → 复用 parseStreamLine（同包）提取 assistant 文本增量与 session_id
  → session_id 回写会话元数据（下次自动 --resume 续上下文）
```

- 消息入档时序：user 消息在进程 Start 成功后写入；assistant 回复（含取消轮的部分回复）在 **done 事件之前**落盘——前端收到 done 重读会话即得完整消息。
- 失败轮无回复不留空消息。
- provider 抽象：MVP 单 claude 实现；多 CLI 扩展点在 processFactory 与参数组装处分派，勿在业务层散落 `if cli == "xxx"`。

## 4. 前端状态与事件纪律（aiChat store）

- 单一 pinia store `aiChat`（目录/会话/消息/任务/模板/配置六域）。
- `chat-task:output` 增量追加当前 assistant 消息；done 后按 `GetChatSession` 重载定型（乐观态被持久化数据覆盖，前后端一致）。
- EventsOn 返回闭包在 onBeforeUnmount 逐一注销，**禁 EventsOff**（多面板共存）。
- 面板 v-show 常驻不卸载 → 事件可能丢失 → `watch(activePanel)` 切回时 `restoreChatTaskState`（GetChatTaskState）兜底恢复，任务不存在则清态防卡死。

## 5. 装配同步点（新增 service 通用教训）

新增带事件的服务（不只 ChatService）时，**必须同步 `web_serve.go` 的 `applySink`**：补 `SetEventSink` 行，否则浏览器通道（/api 或 --serve 模式）收不到该服务全部事件——桌面端正常、浏览器端静默失效，极易漏测。当前 applySink 覆盖五个服务：terminal / update / aiFunc / git / chat。

## 6. 错误码

| 码 | 语义 |
|---|---|
| E_CHAT_SESSION_NOT_FOUND | 会话不存在 |
| E_CHAT_IN_PROGRESS | 同会话任务进行中（串行保护） |
| E_CHAT_EMPTY_PROMPT | 空 prompt |

三处同步：model/app_error.go 常量表 + frontend/src/utils/error.js + docs/API参考.md。

## 7. 权限与模型

- permissionMode：default / acceptEdits / plan / bypassPermissions（claude 原生 `--permission-mode` 取值），默认 default；前端 bypassPermissions 选中即 ElMessage.warning。
- model：空串 = claude 默认模型（不加 --model 参数）；前端 el-select allow-create 支持自定义模型 ID。
