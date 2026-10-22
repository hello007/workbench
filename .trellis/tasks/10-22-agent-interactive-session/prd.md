# Agent 交互式多轮会话（子任务 3/3，执行顺序第 3）

## Goal

AI 对话从「发一轮答一轮」升级为网页版 Claude Code 体验：运行中会话可持续交互，浏览器端与桌面端体验对齐。来源：docs/浏览器模式-二期扩展.md §1（标「优先级最高」）。

## Requirements

### PR1（本轮交付）

1. **执行模型重构**：每轮一进程 → 每会话持续进程。`claude -p --input-format stream-json --output-format stream-json --verbose --permission-prompt-tool=stdio [--resume <sid>] [--permission-mode <m>] [--model <x>]`；stdin 写入驱动轮次，stdout 流式解析复用 parseStreamLine 思路。
2. **中途插话（排队注入）**：运行中再发消息直接写 stdin（CLI 自动排队逐轮处理），不再报 E_CHAT_IN_PROGRESS；前端输入框运行中不再置灰。
3. **权限审批**：`control_request`/`can_use_tool` 事件 → 后端经 EventSink 下发前端（新增 chat-approval 事件）→ 前端审批弹窗 → 应答经 WS 上行新通道（/api/rpc 先例）回后端 → 写 stdin `control_response`（按 request_id 匹配，allow/deny 双路径）。
4. **运行中断**：取消链路改走 stdin `control_request`/interrupt（替换现杀进程方式或叠加），被中断轮以 result 收尾落盘。
5. **进程生命周期**：空闲（无在途轮次）超 10 分钟自动退出（常量，后续再配置化）；下次发消息 spawn+`--resume` 续上下文；应用关闭 Shutdown 收口全部存活进程；同时活跃进程上限 4（超出提示先结束某会话）。
6. **chat-question 围栏保留**：AskUserQuestion 仍禁用（流式 headless 下被 CLI 自动应答、不进控制协议）；工具授权与模型提问分工——审批弹窗管前者，围栏卡片管后者。

### PR2（第二轮交付，本任务内完成）

7. **多会话并行**：会话注册表扩展（每会话独立进程态）；chat-task:* 事件命名空间扩展会话维度；前端会话列表切换；进程上限与回收策略复用 PR1 生命周期机制。
8. **会话恢复复核**：断线/重开浏览器场景续聊体验复核（--resume 链路已有，重点核 WS 断线续传 2c8e5f8 与多会话并行的叠加场景）。
9. **浏览器端对齐**：桌面/浏览器两端口径一致复核（审批弹窗在浏览器通道的应答链路）。

10. **spec 沉淀**：ai-chat-service.md 重写执行模型章节 + browser-channel.md 补上行通道契约。

## Acceptance Criteria

### PR1

- [x] 运行中会话可插话且影响后续输出（Go 侧 FIFO 排队注入，fake 进程链路验证；真实 CLI 插话待真机冒烟）
- [x] 工具授权请求在前端弹窗确认后继续执行；deny 后模型收到拒绝并可改道（fake 进程 allow/deny 双路径 + E2E 弹窗链路；真实 CLI 待真机冒烟）
- [x] 运行中取消走 interrupt，被中断轮部分回复正常落盘
- [x] 空闲超时自动退出后再次发消息可 resume 续上下文
- [x] 同会话不再出现 E_CHAT_IN_PROGRESS 拒绝（排队注入生效，错误码退役保留兼容）
- [x] chat-question 选择题卡片行为不回归（围栏契约/已答置灰/提交链路零改动，vitest+E2E 回归通过）
- [x] 桌面端与浏览器端审批链路均可用（事件零特判进 WS hub 广播；应答走 /api/rpc 通用通道，E2E mock 链路验证；真机浏览器待冒烟）
- [x] go test + vitest + E2E 全绿（后端全量 + -race；前端 2040 用例；ai-chat E2E 8 用例含新增插话/停止/审批 5 用例）

### PR2

- [x] 多会话并行互不串流（架构原生：sessions 注册表按会话键控；隔离单测 TestChatService_MultiSessionParallel 覆盖交错输出/落盘/session id 三面；前端 currentSessionChatTasks 视图过滤 + vitest 跨会话隔离用例），关停链 CloseAll 收口完整（Kill → pump 收尾 → 槽位归还，-race 通过）
- [x] 断线重连后多会话事件续传正确（seq+resume 基建 2c8e5f8 已收口且 chat 事件零特判覆盖；复核结论：chat 兜底 = 面板切回 ListChatActiveTasks 全量重建，不新增 transport 钩子——spec §10）
- [x] 删除会话终止常驻进程回收槽位（多会话模型新增回收路径，TestChatService_DeleteSessionKillsProcess）
- [x] go test + vitest + E2E 全绿

## Definition of Done

* 测试更新（后端 fake process 注入面扩展 + 前端 vitest + E2E）
* CI 全绿（覆盖率门禁按现行分层基线）
* spec 沉淀完成（ai-chat-service.md / browser-channel.md）
* CLI 版本下限校验：启动前探测（锁 ≥2.1.205，建议 ≥2.1.268；本机 2.1.296 已达标）

## Decision (ADR-lite)

**Context**：权限审批强依赖持续会话进程（headless 每轮一进程无法实现）；CLI 协议经实测验证可行但有关键坑。

**Decision**：
1. 执行模型定为持续会话进程（`-p --input-format stream-json --output-format stream-json --verbose --permission-prompt-tool=stdio`）。
2. 插话走 stdin 排队注入（CLI 原生逐轮处理），退役同会话拒绝语义。
3. 生命周期空闲超时退出（10 分钟常量）+ 活跃上限 4。
4. 工具授权原生审批弹窗 / 模型提问 chat-question 围栏，两者并存分工。
5. 分两阶段：PR1 执行模型+插话+审批，PR2 并行+恢复复核。

**Consequences**：
* `--permission-prompt-tool=stdio` 漏配 = 审批静默自动拒绝（无报错），参数组装必须单点且有测试守护。
* `system/init` 流式模式下每轮重复下发，前端勿按一次性事件建模。
* 终态判定 `result.subtype` 与 `is_error` 必须组合判断（存在 subtype=success 且 is_error=true 失败形态）。
* 审批应答打破 WS「唯一业务上行是 resume 握手帧」契约，须设计上行通道（/api/rpc 终端输入先例）。
* chatProcess 接口须扩展 stdin 写入口（现仅 stdout 单向读），fake 注入面同步扩展。

## Out of Scope

* 单页 OCR 及其相关 schema/队列改动（父任务已决策暂缓）
* 移动端适配、TLS、多 token（二期其他清单项）
* AskUserQuestion 弹窗化（CLI 控制协议不覆盖，不调研）
* 空闲超时配置化（先常量，有需求再开）

## Technical Notes

* 相关 spec：docs/spec/ai-chat-service.md（ChatService 契约/chat-question 块/processFactory 注入点/--resume 回写/同会话串行）、browser-channel.md、logging-and-errors.md（AppError 三处同步）
* 既有基建：chat-task:* 事件流/落盘索引分离/选择题卡片、WS 双向通道+断线续传（2c8e5f8）、processFactory 注入点
* 依赖顺序：本批次子任务 2（断线续传）已完成——多会话并行场景其基建已就绪
* 调研报告：research/claude-cli-streaming-input.md（协议实测，10 组探针载荷）、research/repo-current-state.md（影响面分层）
