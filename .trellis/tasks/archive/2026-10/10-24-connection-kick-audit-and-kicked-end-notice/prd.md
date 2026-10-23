# 连接管控二期：踢出审计 + 被踢端感知提示

## Goal

连接管控一期（10-23）已落地踢出与令牌在线数，但留有两个感知面缺口：

1. **踢出动作无审计留痕**——`data/web_audit.log` 仅有认证与 token 生命周期事件（`auth_*`/`token_*`），管控操作无迹可查；
2. **被踢端无「被主机断开」感知**——踢出走 transport 既有静默自动重连链路，被踢端横幅显示通用「与后端连接已断开，正在自动重连…」，桌面操作者踢出后该行消失又因对端秒级重连立即出现，无法确认踢出生效。

本任务补全：踢出动作进审计日志 + 被踢端展示断开原因提示。

## Requirements

1. **踢出审计**：`App.KickConnection` 成功（hub 返回命中）后写 `data/web_audit.log`，事件名 `conn_kick`，行含被踢目标 `tokenId`/`name`/`remoteAddr`/`connId`/`ts`；无明文 token 无完整 URL
2. **被踢端提示**：单播控制帧 `{"event":"ws-kicked","data":[...]}`（无 seq，不进环形缓冲）送达被踢连接；前端 connBanner 显示被踢态文案「连接已被主机断开，正在自动重连…」，随后既有自动重连链路不变
3. **单播隔离**：未被踢的其他连接收不到任何管控帧；`ws-connections:changed` 广播载荷恒 count 数字（隐私红线不破）

## Acceptance Criteria

- [ ] 审计行断言：字段齐（event=conn_kick / tokenId / name / remoteAddr / connId / ts）、无敏感信息；踢出生效才写（miss 不写）
- [ ] 踢出帧到达时序：帧先于连接关闭送达（writePump 单写者 flush 后自关）
- [ ] 帧丢失降级：队列满/写失败静默退化为普通断开（踢出断开本身恒生效）
- [ ] 单播不误伤其他连接
- [ ] 踢出帧不进环形缓冲（resume 补发不含，防重连幻影「被踢」）
- [ ] 前端：events.js 拦截 `ws-kicked`、connBanner 被踢态置位/消费一次/重连清除、`notifyDisconnected` 不覆盖被踢文案
- [ ] go test 全绿（含 -race）+ vitest/E2E 回归 + spec 文档更新

## Definition of Done

- 后端单测/集成测试覆盖上述断言面；前端 vitest（events.spec.js / connBanner.spec.js）
- spec 更新：browser-channel.md §2.3 连接管控段（踢出帧契约）+ §3 契约表（单播帧/审计行）+ logging-and-errors.md 审计事件表（conn_kick）
- wailsjs 绑定零变更（App.KickConnection 对外签名不动）

## Technical Approach

### 审计（方案 A：App/manager 绑定层）

- `WSHub.DisconnectClient` 返回值扩为 `(ConnDetail, bool)`（内部已定位 target，零额外遍历）；manager.KickConnection 透传 detail；App 层命中后 `AuditLogger.Log` 写 `conn_kick`
- `AuditRecord` 加 `ConnID uint64 json:"connId,omitempty"`（既有事件不受影响）；`Log` 签名扩参或新增专用方法，实现时按最小扰动定
- 时序纪律对齐 `auditEventLocked` 先例：**动作生效后才写审计**（hub 返回 true 才写；miss/停机 nil 不写）
- server 包零新增回调注入面；桌面按钮与浏览器 RPC 反射面同走 App.KickConnection，单咽喉全覆盖

### 被踢端感知（方案 A：单播帧 + flush 后自关）

- `DisconnectClient` 锁内定位 target 后不再直接 `conn.Close()`：非阻塞入队（1）marshal 好的踢出帧字节（2）nil 哨兵；writePump 新增 nil 分支——写出哨兵前全部帧后发 RFC Close 帧 return（defer conn.Close() 收尾）→ readPump 走既有 unregister。**单写者内保序：帧必先于关闭送达**
- 降级：队列满（缓冲 64，慢消费者形态）非阻塞入队失败 → 回退直接 `conn.Close()`；**best-effort 口径：踢出断开恒生效，提示帧丢失退化为既有断连横幅**（spec 注明）
- 踢出帧无 seq 不进环形缓冲（对齐 ws-gap/ws-synced 控制帧先例；禁 resume 幻影回放）
- 重复踢出：kicked 标记防二次入队（writePump 自关路径垃圾帧防御），幂等语义保留

### 前端（方案 1：connBanner 被踢感知态）

- events.js onmessage 分发层拦截 `ws-kicked`（控制帧先例位置）→ connBanner 新 `notifyKicked()`：置被踢态横幅「连接已被主机断开，正在自动重连…」
- 紧随的 `notifyDisconnected()` 检测被踢态不改文案（防覆盖）；重连成功 `notifyConnected()` 走既有「连接已恢复」+ 清除被踢态；**被踢态仅消费一次**（首次重连周期内有效，其后断线恢复普通文案）
- **判定窗口 = 帧到达本身零启发式**：收到帧即被踢（确定性），没收到即网络抖动
- 边界：踢出后服务紧接停机 → 被踢态先替换文案、随后 `notifyUnreachable` 覆盖「服务不可达」——可接受不加态；多标签页各持独立 WS 连接，帧只达被踢 tab（单播天然隔离）

## Decision (ADR-lite)

**Context**：一期踢出无留痕、被踢端无感知；审计基建（AuditLogger JSONL）与控制帧先例（ws-gap/ws-synced）均现成。
**Decision**：审计走 App/manager 绑定层（Q1=A）；感知走单播帧 + writePump flush 后自关（Q2=A）；提示走 connBanner 被踢态（Q3=1）；可见性仅落盘、不记 actor（Q4=推荐）。
**Consequences**：server 零新增注入面、wailsjs 零变更；best-effort 容忍度明确（断开恒生效、提示可丢）；actor 归因为已知限制，未来经 RPC 上下文透传补齐（涉 §2.2 翻译层契约，独立任务）；单播是 hub 首个非广播投递面，后续同类需求（定向提示）可复用该通道形态。

## Out of Scope

- 黑名单/防重连（一期已定：撤销令牌承担）
- 审计历史桌面 UI 页签
- actor 归因（RPC 调用者身份透传）
- `hub.Close()` 停机断全连的审计（生命周期动作非管控动作）

## Technical Notes

- 前置阅读已核：docs/spec/browser-channel.md §2.3/§2.4、server/ws.go（DisconnectClient/writePump/send 容量 64/wsSendBuffer）、service/audit_log.go 全文、frontend/src/transport/events.js + connBanner.js、useDeviceConnections.js
- writePump 现有「send channel 关闭→发 Close 帧收尾」为 nil 哨兵方案的结构基础（frame==nil 分支与其并列）
- 发散收口：慢消费者被踢 = 队列满 = 自动降级路径，与 best-effort 自洽无特判；重复踢出 kicked 标记；停机不审计
