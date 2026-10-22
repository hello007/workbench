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
