# 修复终端输出 UTF-8 分片截断

## Goal

消除终端输出中多字节字符（中文、emoji、Nerd Font 图标）跨 PTY 读取边界时产生的永久乱码，保证 `terminal-output` 事件链路字节级正确。

## What I already know

- 后端输出泵 `service/terminal.go:293-311` 每次 `Read` 最多取 4096 字节，随后直接 `string(buf[:n])` 经 `s.emitCurrent("terminal-output", ...)` 发送。
- PTY 读取边界与 UTF-8 字符边界无关；切片可能落在多字节字符中间。
- Go 字符串允许非法 UTF-8，但 Wails 事件经 JSON 序列化时非法字节被替换为 `U+FFFD`，损坏不可恢复。
- 前端 `useTerminal.js:163-170` 直接 `term.write(output)`，接收字符串；浏览器通道复用同一事件载荷。
- 现有 `terminal-output` 事件契约被桌面端、浏览器通道、前端单测 mock（`wails-mock-defaults.js`）共同消费。
- xterm.js 6.0 `write()` 同时接受 `string` 与 `Uint8Array`，内部 UTF-8 解码器可跨多次 write 保留残片。

## Assumptions (temporary)

- ConPTY 与 Unix PTY 在正常 locale 下均输出 UTF-8；非 UTF-8 字节按替换字符处理可接受。
- 本任务不改变事件名称与参数顺序，仅改变字符串内容正确性。

## Open Questions

- 无。传输方案已定为后端增量 UTF-8 解码（见 Decision）。

## Requirements (evolving)

- 输出泵不得发送被截断的多字节字符。
- 残片按会话隔离，最多保留 3 字节，不阻塞已完整的输出。
- 读取结束（EOF/关闭）时对残留不完整序列做确定处理（替换或丢弃），不得静默吞掉整段。
- 非法字节序列按 `U+FFFD` 替换，不得让输出泵停滞。
- 保持 `terminal-output` 事件名与 `(sessionID, payload)` 参数顺序不变（方案 A 前提下）。

## Acceptance Criteria (evolving)

- [x] 将同一中文/emoji 字符串按任意 1 字节切分逐片喂入解码器，拼接结果与原文完全一致。
- [x] 4096 字节边界恰好落在 3 字节汉字中间时，输出无 `U+FFFD`、无乱码。
- [x] 末尾残缺序列在 Flush 后不产生半个字符，且不影响此前已输出内容。
- [x] 非法 UTF-8 字节输入不 panic、不阻塞，输出含替换字符。
- [ ] 现有终端相关单测、前端单测与 E2E 全部通过（后端单测、前端 vitest 已通过；E2E 未运行）。
- [ ] 桌面模式与浏览器通道终端输出均无回归（事件契约未变；浏览器通道未单独实测）。

## Definition of Done (team quality bar)

- 单元测试覆盖跨片、连续多字节、非法字节、EOF 残片四类场景。
- `go test ./service/... ./util/...` 通过；前端 `npm test` 通过。
- 不新增依赖，不改变事件契约（方案 A）。
- 若行为有变，更新相关注释；无需更新 README。

## Decision (ADR-lite)

**Context**：事件载荷当前为字符串，已被桌面端、浏览器通道、前端 mock 与 E2E 共同消费；本次只需修复字节截断，不需要引入二进制协议。

**Decision**：采用方案 A——后端输出泵内做增量 UTF-8 解码，最多保留 3 字节残片，仅发送完整字符；事件名、参数顺序、字符串载荷均不变。

**Consequences**：前端零改动、回归面最小、可纯函数单测；代价是字符串载荷无法表达任意二进制，未来若引入图片/附件协议需另立任务改造传输层。

## Out of Scope (explicit)

- 不改用 Base64 二进制事件（除非最终选择方案 B）。
- 不做输出聚合/背压优化（另立任务）。
- 不改 ConPTY 生命周期与退出检测（另有两个任务）。
- 不处理非 UTF-8 locale 的转码（如 GBK 输出）。

## Technical Notes

- 关键文件：`service/terminal.go`（`startOutputPump`）、`frontend/src/composables/useTerminal.js`、`frontend/src/e2e/wails-mock-defaults.js`。
- 推荐实现：提取纯函数/小类型 `utf8StreamDecoder`，`Write([]byte) string` 返回可安全发送的完整前缀，内部保留最多 3 字节残片；`Flush() string` 处理 EOF。
- 测试可用表驱动直接喂字节块，无需真实 PTY；输出泵只需把 `string(buf[:n])` 替换为 `decoder.Write(buf[:n])`。
- 参考：`docs/research/2026-11-04-终端内核开源方案调研与借鉴清单.md` 第 3.2 节。

## Research References

- [`docs/research/2026-11-04-终端内核开源方案调研与借鉴清单.md`](../../../docs/research/2026-11-04-终端内核开源方案调研与借鉴清单.md) — 现状基线与 UTF-8 分片风险分析
