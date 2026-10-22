# Agent 交互式多轮会话（子任务 3/3，执行顺序第 3）

## Goal

AI 对话从「发一轮答一轮」升级为网页版 Claude Code 体验：运行中会话可持续交互，浏览器端与桌面端体验对齐。来源：docs/浏览器模式-二期扩展.md §1（标「优先级最高」）。

## Requirements

1. **中途插话**：任务运行中追加指令/纠正方向——ChatService 进程 stdin 持续写入（stream-json input 模式）；同会话串行保护（E_CHAT_IN_PROGRESS）语义复核是否放宽为「排队注入」
2. **权限审批**：CLI permission 请求事件拦截 → chat-question 块契约复用/扩展 → 前端审批 UI 弹窗 → 应答回写 stdin；headless 自动应答禁用逻辑（--disallowedTools）与新审批链路关系须理清
3. **多会话并行**：会话注册表 + 每会话事件流隔离（chat-task:* 命名空间扩展会话维度）；前端会话列表切换；进程数上限与资源回收
4. **会话恢复**：--resume 链路已有（chat-task 落盘），复核断线/重开浏览器场景续聊体验
5. **浏览器端对齐**：WS 双通道事件分发复核，桌面/浏览器两端口径一致
6. spec 沉淀 ai-chat-service.md + browser-channel.md

## Acceptance Criteria

- [ ] 运行中会话可插话并影响后续输出
- [ ] 工具授权请求在前端弹窗确认后继续执行
- [ ] 多会话并行互不串流，关停链 WaitXxxDone 收口完整
- [ ] go test + vitest + E2E 全绿

## Technical Notes

* 相关 spec：docs/spec/ai-chat-service.md（ChatService 契约/chat-question 块/processFactory 注入点/--resume 回写/同会话串行）、browser-channel.md
* 既有基建：AI 对话工作台（chat-task:* 流/落盘索引分离/选择题卡片）、WS 双向通道、chat-question 块契约
* 依赖顺序：建议在本批次子任务 2（断线续传）之后做——多会话并行会放大断线场景
