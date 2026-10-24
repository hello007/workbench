# 连接管控三期（审计可见性：actor 归因 + 审计历史查看）

## Goal

补全连接管控二期遗留的两个审计可见性缺口：
1. **actor 归因**：`App.KickConnection` 为桌面按钮与浏览器 RPC 共用入口，方法体内无调用来源上下文，`conn_kick` 审计行只记被踢目标不记操作者（多令牌场景无法回答「谁踢的」）。将 RPC 调用者身份透传进审计。
2. **审计历史查看**：`data/web_audit.log` 已有 auth_*/token_*/conn_kick 全谱事件落盘，但无任何 UI 消费面。提供桌面端审计历史查看 UI（过滤/分页）。

## Requirements

### R1 actor 透传（方案 A：context 注入 + 每请求装饰器）

- `server/web.go` `requireToken`：认证成功后将 `tokenID` 注入 `r.Context()`（`context.WithValue` 私有 key 类型；只注 tokenID 不注明文凭据）。
- `server/rpc.go` `handleRPC`：每请求从 ctx 取 actor，经装配层注入的构造回调（WebOptions 新字段）构造 per-request 装饰 target。
- **main 包定义 wrapper**（server 包禁反向依赖 main）：`type rpcAuditTarget struct { *App; actor string }` embed 提升全部导出方法，override `KickConnection` 委托 App 未导出方法。
- App 层：导出 `KickConnection(connID)` 签名不变（wailsjs 零变更），委托未导出 `kickConnectionAs(connID, auditActorDesktop)`；未导出方法不入 Wails 绑定面与 RPC 反射面。
- actor 表达：`auditActorDesktop = "desktop"`；RPC 路径 actor=tokenID。

### R2 审计行 actor 字段（两字段格式）

- `AuditRecord` 加 `actorSource`（desktop|rpc，omitempty）+ `actorTokenId`（RPC 调用者令牌 ID，桌面为空，omitempty）。
- 现有 `TokenID` 字段语义不变（conn_kick 的被踢目标令牌；auth_* 的认证令牌）。
- `LogConnKick` 改造为结构化参数（如 `KickAudit{Actor{Source,TokenID}, Target{TokenID,Name,IP,ConnID}}`），单调用点 `auditConnKick` 同步。

### R3 隐私屏蔽（HiddenMethods，对二期原则的有据偏离）

- RPC 翻译层支持 `RPCHiddenMethods`（WebOptions 新字段，装配层传入含 `QueryAuditLog`）；命中返回 `ErrCodeRPCMethodNotFound`「方法不存在」，与真不存在同响应，不暴露存在性。
- 偏离理由：二期「同构暴露不特判」指管控动作面（两端语义一致）；审计历史为数据隐私面（他人 IP/token 明细），红线「仅走桌面出口」优先于同构洁癖。注释写清。

### R4 审计读侧 API（后端过滤 + 分页）

- `service/audit_log.go` `AuditLogger` 加 `QueryAuditLog(opts AuditQueryOptions) (AuditQueryResult, error)`：
  - 过滤维度：Events []string / From, To int64（unix 秒）/ TokenID / Offset, Limit（Limit 上限钳制防一次拉全量）。
  - 合并读 `.1`（旧）+ 主文件（新），坏行跳过（读方容错约定），时间倒序（最新在前），Total 为过滤后总数。
  - 快照语义：打开-读-关闭无锁；与写侧 O_APPEND 并发安全；轮转 rename 窗口竞态容忍（可能重复/丢单行，PRD 明示不强求强一致）。
- App 导出绑定方法 `QueryAuditLog(opts)` 返回结果（Wails 绑定 + HiddenMethods 屏蔽 RPC 面）；`model.AuditQueryOptions`/`AuditQueryResult` 定义于 model 包。

### R5 审计历史 UI（设置页入口 + 独立弹窗）

- `SettingsPanel.vue`「浏览器访问」段（token 管理同区）加「审计日志」按钮，开独立 Dialog（`min(960px, 视口比例)` × `min(620px, vh)` 档，对齐设置弹窗先例）。
- 新组件 `AuditHistoryDialog.vue`：表格列（时间/事件/来源/操作者令牌/目标令牌/目标 IP 等，列取舍实现期定）+ 事件类型过滤 + 时间窗过滤 + 分页。
- **浏览器模式隐藏入口**（红线下不暴露功能面，而非点击报错）；即使误调，RPC 面 HiddenMethods 兜底拒「方法不存在」。
- 操作者/目标令牌 ID → 名称映射消费 `ListWebTokens` 既有数据。

## Acceptance Criteria

- [ ] RPC 踢出审计行：`actorSource=rpc` + `actorTokenId=<调用者令牌 ID>`；桌面踢出审计行：`actorSource=desktop`、`actorTokenId` 空
- [ ] 桌面 UI 可查看含 conn_kick 全谱审计历史，事件类型/时间窗过滤 + 分页可用
- [ ] 浏览器通道调 `QueryAuditLog` 被拒「方法不存在」（HTTP RPC 隐私断言测试）；设置页入口浏览器模式隐藏
- [ ] 写侧契约逐字不变（O_APPEND/轮转/旁路降级现有测试零改动通过）
- [ ] 读侧解析：坏行跳过 / `.1`+主文件合并 / 过滤分页 / 时间倒序 单测覆盖
- [ ] go test 全绿（含 `-race`）+ vitest/E2E 回归 + spec 文档更新（browser-channel.md + logging-and-errors.md）

## Definition of Done

- 测试：读侧解析（坏行/轮转合并/过滤分页）、actor 透传（RPC 审计行含调用者 tokenID、桌面含 desktop）、HiddenMethods 隐私断言（HTTP 层实测）、写侧契约回归、`-race`
- 前端：vitest 组件用例（渲染/过滤/分页/浏览器模式入口隐藏）+ wails-mock-defaults.js 补 `QueryAuditLog` + E2E
- wailsjs 三处手动同步（App.js/App.d.ts/models.ts）+ `type X string` 别名检查（本轮参数为 struct 预期无，仍须验证 npm run build 过 MISSING_EXPORT）
- spec 沉淀：browser-channel.md §2.2（HiddenMethods 契约）+ §2.3（actor 归因）+ logging-and-errors.md §8（读侧 API + 快照语义）

## Technical Approach

实施序（单分支三段）：

1. **后端 actor 链**：`service/audit_log.go`（AuditRecord 字段 + LogConnKick 结构化参数）→ `app_webserve.go`（kickConnectionAs 拆分 + auditConnKick 带 actor + wrapper rpcAuditTarget + 装配回调）→ `server/web.go`（ctx 注入）+ `server/rpc.go`（per-request target + RPCHiddenMethods）→ 测试。
2. **读侧 API**：AuditLogger.QueryAuditLog + model 类型 + App 绑定方法 + wailsjs 三处同步 + 测试。
3. **前端 UI**：AuditHistoryDialog + 设置入口 + 浏览器模式判定隐藏 + vitest/E2E + spec 文档。

## Decision (ADR-lite)

**Context**：审计行需区分操作者来源；桌面 Wails 绑定面 = RPC 白名单同构，App 方法无调用来源上下文；goroutine-local 无官方 API；server 包禁反向依赖 main。
**Decision**：方案 A（context 注入 + main 包 per-request wrapper override + App 未导出 `kickConnectionAs`）+ 两字段 actor 格式 + HiddenMethods 屏蔽（呈现「方法不存在」）+ 读侧 AuditLogger 加 QueryAuditLog 后端过滤分页 + UI 走设置页入口独立弹窗。
**Consequences**：导出面/wailsjs/RPC 位置参数契约零变更；HiddenMethods 为二期「同构不特判」原则的首个有据偏离（数据隐私红线优先，后续同类需求复用该机制）；读侧快照语义接受轮转窗口竞态（重复/丢单行）；wrapper 每请求构造一次（低频调用面无性能顾虑）。

## Out of Scope

- 浏览器通道的审计历史任何形态（红线，HiddenMethods 恒拒）
- 审计日志外部归档/导出、多份轮转链（维持单份 .1）
- 审计写侧任何语义变更（O_APPEND/轮转/旁路降级逐字不变）
- 踢出防重连黑名单（二期已定 MVP 无）
- actor 身份强化（如桌面多用户区分——本地单用户模型恒 desktop）

## Technical Notes

### 已探明事实（代码勘察 2026-10-24）

- `server/web.go:173` `requireToken` 持有 tokenID（`authorized` 返回 matched+tokenID），RPC handler 挂载其内；`server/rpc.go:148` `invokeRPC` 反射 `MethodByName`，方法签名即契约。
- `App.KickConnection`（app_webserve.go:236）桌面按钮与浏览器 RPC 共用；审计写点 `auditConnKick`（:253）单咽喉；前端调用点 ActivityBar.vue 直接 import wailsjs 绑定。
- `service/audit_log.go` 纯写侧：`write` 单锁收口（轮转→建目录→O_APPEND），`.1` 覆盖旧份，5MB 上限≈3 万行。
- 明细 popover（ActivityBar.vue）320px 宽——历史表格塞不下，UI 另选容器（已定设置页+弹窗）。
- `NewRPCHandler(target any)` 以 any+reflect 路由避 server→main 反向依赖——wrapper 必须定义在 main 包。

### 前置阅读

- docs/spec/browser-channel.md §2.2（RPC 反射面契约）+ §2.3（连接管控二期段，actor 未记录为已知限制）
- docs/spec/logging-and-errors.md §8（审计事件表 + 「动作生效后才写」+ 禁明文 token/完整 URL）
- docs/spec/cross-layer-contracts.md（wailsjs 三处同步）
- docs/spec/frontend-visual-conventions.md（弹窗响应式分档、flex 约束链）
