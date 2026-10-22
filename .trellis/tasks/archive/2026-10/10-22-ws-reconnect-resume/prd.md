# 浏览器断线重连续传（子任务 2/3，执行顺序第 2）

## Goal

浏览器通道断网重连后，终端滚动缓冲与 AI 任务/对话输出从断点续传，替代一期整屏重放。来源：docs/浏览器模式-二期扩展.md §2。

## Requirements

1. **事件序号增量同步**：WS 连接级单调序号，重连握手携带已收序号，服务端补发缺口；缓冲上限与溢出策略（丢弃最旧+标记断层提示）brainstorm 定
2. **终端续传**：滚动缓冲镜像方案（服务端增量缓存 vs xterm scrollback 服务端镜像）brainstorm 定，与 terminal-multi-session 既有快照机制（v2 归一化）对齐
3. **AI 输出续传**：ai-task:*/chat-task:* 事件流断点续传语义；流式文本块补发边界
4. **多设备同时连接语义**：会话归属（终端会话按连接隔离 or 共享）、光标/输入冲突策略明确化
5. spec 沉淀 browser-channel.md（三通道与热轮换契约基础上扩展）

## Acceptance Criteria

- [ ] 断网重连后终端与 AI 输出无缺失无重复
- [ ] 重连期间事件不丢（有界缓冲），溢出有断层提示
- [ ] 多连接语义有明确实现与文档
- [ ] serve_rpc_smoke / serve_cookie_smoke / E2E 全绿

## Technical Notes

* 相关 spec：docs/spec/browser-channel.md、terminal-multi-session.md、ai-chat-service.md
* 一期已留口子：前端 transport 适配层（wails/browser 双通道）、WS 按会话分发、RPC 与 service 解耦

## Decisions（2026-10-22 brainstorm 收口，全按推荐）

1. **序号机制**：WSHub 全局单调 seq + 环形缓冲（2000 帧）。Emit 统一编 seq，帧加 seq 字段；客户端记 lastSeq，重连首帧发 `{"resume": <lastSeq>}`，服务端补发 buffer 内 `seq > lastSeq` 缺口；`lastSeq` 落于 buffer 之外（缺口）发断层标记帧，前端提示后走全量刷新。多标签页各记各的 lastSeq。
2. **终端续传**：通用 ring buffer 补发为主，短断线无感；溢出断层时终端面板调既有快照/重拉接口整屏重建，不动 terminal service。
3. **AI 输出续传**：同终端——ring buffer 覆盖短断线；断层时前端调既有 RPC（AI 任务状态/会话消息查询）拉当前态对齐，不重放历史流式块。
4. **多设备语义**：维持广播共享语义（多标签页/多设备共享同一后端状态），终端会话共享、输入 last-write-wins，文档明确化。

> 即：一个通用机制（seq + ring buffer + resume 握手）覆盖需求 1/2/3，需求 4 靠文档明确。改动面集中在 `server/ws.go` + `frontend/src/transport/events.js` + 断层 UI 提示。

## Implementation Notes

* hub 重启（webServeManager Start 重建）seq 归零；客户端 lastSeq > 服务端当前 seq 视为断层（不补发）。
* 新连接（无 resume）从当前 seq 起，不重放历史。
* 慢消费者踢出逻辑保留不变。
* 客户端消息处理：readPump 现只读不处理客户端帧，需增加 resume 帧解析。
