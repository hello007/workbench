# 连接管控四期：临时封禁（踢出防重连 + 封禁管理）

## Goal

补齐连接管控的时限性拒连档位。二期落定踢出语义 = 断开当前连接、对端同令牌可重连（非封禁），防重连仅有 RevokeWebToken（永久凭据失效）一档——缺「先踢先禁、观察后解封」的临时拒连手段。三期已补 actor 归因与审计可见性，封禁动作的审计与追溯基建现成。本任务新增：封禁状态机（tokenID 维度临时拒连）+ 明细面板/令牌管理两处封禁入口 + 封禁状态查看与解封。

## Requirements

* 封禁状态机：tokenID 维度，支持永久（手动解封）+ 带过期时间（懒过期判定，禁后台扫描）双形态；可选 reason 备注
* 两认证点（WS 握手 + HTTP requireToken）封禁均拒，专用错误提示（含解封时间；与普通 401「请携带访问令牌」可区分）
* 封禁服务端语义 = 仅拒新连（已建立连接不受影响，对齐撤销热生效语义）；UI 确认弹窗带「同时断开在途连接」勾选（默认勾选），勾选后前端按 tokenID 过滤连接明细逐个 KickConnection
* 存储：`data/web_bans.json` 持久化（util.SaveJSON 原子写），由 WebTokenService 持有；撤销令牌时联动清理其封禁记录；封禁已撤销令牌拒绝
* ban/unban 审计行含操作者归因（actorSource/actorTokenId），复用三期装饰器链路（rpcAuditTarget 扩 override）
* 幂等语义对齐 Revoke 先例：重复封禁 = 刷新封禁参数（改时长/改永久），未封禁解封 = no-op 成功，令牌不存在 = E_TOKEN_NOT_FOUND
* UI：SettingsPanel 令牌条目封禁徽标（含到期时间/reason tooltip）+ 封禁/解封按钮；ActivityBar 明细面板行封禁按钮（与踢出并列）；不建独立封禁列表弹窗
* 同构 RPC 面可达（动作面不屏蔽）；不新增浏览器通道数据泄露面（封禁状态经 ListWebTokens join 透出，名册本就可达）

## Acceptance Criteria

* [ ] 被封禁令牌的新连接（WS 握手 + HTTP 请求）两认证点均拒，401 消息含封禁语义（非「请携带访问令牌」）
* [ ] 已建立 WS 连接封禁后保持至自然断开（数据面不校验）；勾选联动踢出时按 tokenID 全部断开
* [ ] 封禁带过期时间则到期自动恢复可达（懒判定无后台扫描）；永久封禁可手动解封；解封后新连接立即可达
* [ ] 撤销令牌联动清理封禁记录；封禁已撤销令牌被拒（明确提示）
* [ ] 重启后封禁持续生效（持久化）
* [ ] ban/unban 审计行含 actorSource/actorTokenId（桌面 desktop/空、RPC rpc/调用者记录 ID），动作生效后才写（失败不写），审计历史弹窗可过滤查看
* [ ] BanWebToken/UnbanWebToken 浏览器 RPC 同构可达（动作面）；QueryAuditLog 屏蔽不回归
* [ ] go test 全绿（含 -race）+ vitest/E2E 回归 + spec 文档更新（browser-channel.md + logging-and-errors.md）

## Definition of Done

* 后端单测：两认证点生效/绕过、懒过期（nowFn 注入）、幂等、撤销联动、持久化损坏降级、SaveJSON 失败回滚、审计行、并发 -race
* 前端 vitest：条目徽标/封禁弹窗/解封链/联动踢出编排；E2E：封禁链路 spec
* 错误码三处同步（如有新增）；wailsjs 三处手动同步
* docs/spec/browser-channel.md（§2.2/§2.3/§3/§4/§6）+ logging-and-errors.md（§8 审计事件表）更新

## Technical Approach

### 后端

1. **server 层**：`TokenVerifier` 契约不动；新增封禁判定注入面——`BanCheck func(tokenID string) string`（空串=未封禁，非空=拒绝消息由 service 组装，含解封时间），`WebOptions.BanCheck` + `WebHandler`、`WSHub` 同款字段（构造注入，对齐 verify/onAuth 先例，server 不知 service）。两认证点在 authorized 命中后判：HTTP `requireToken` 401 + 专用消息；WS `ServeHTTP` 升级前 401 + 专用消息。封禁拒连的审计形态：复用 `AuthEventFailure`（TokenID 填命中值，文档注明封禁形态非空——与普通失败恒空区分）
2. **service 层**（WebTokenService 扩展，同 mu 锁）：
   - 存储 `data/web_bans.json`（`WebBanList{Version, Bans}`，损坏降级空列表 + slog.Warn 对齐 web_tokens.json 模式）
   - `WebBanRecord{TokenID, TokenName, BannedAt, ExpiresAt(0=永久), Reason, ActorSource, ActorTokenID}`
   - `Ban(id, expiresAt, reason, actorSource, actorTokenID)`：校验令牌存在且未撤销（已撤销拒，候选新码 `E_TOKEN_REVOKED` warning）；已封禁刷新参数；SaveJSON 失败回滚内存；生效后审计
   - `Unban(id, actorSource, actorTokenID)`：未封禁 no-op 成功；令牌不存在 E_TOKEN_NOT_FOUND
   - `BanRejectMessage(tokenID) string`：懒过期判定（ExpiresAt!=0 && <=now 视为未封禁，记录不删留列表显示「已过期」态）——verify 闭包旁 banCheck 注入的生产实现
   - `Revoke`/`RevokeAllExcept` 联动清理对应 ban 记录
   - 审计：`AuditEventTokenBan`/`AuditEventTokenUnban` 常量 + `LogBan`/`LogUnban` 结构化出口（对齐 KickAudit 形态，reason omitempty）
3. **model**：`WebTokenStatus` 扩封禁字段（Banned/BannedUntil/BanReason，join 形态对齐 OnlineConns；落盘零污染——web_tokens.json 仍纯 WebTokenRecord）；错误码候选 `E_TOKEN_REVOKED`（三处同步）
4. **App 层**：`BanWebToken(id, expiresAt, reason) error` + `UnbanWebToken(id) error` 绑定；actor 归因复用三期链路——`rpcAuditTarget` 扩 override 两方法 → 未导出 `banWebTokenAs`/`unbanWebTokenAs`；`ListWebTokens` join 封禁状态；装配层 verify 闭包旁注入 banCheck（web_serve.go assembleWebServe 单点）

### 前端

5. **SettingsPanel 令牌条目**：已封禁徽标（warning tag：永久 /「至 <时间>」；tooltip 显 reason）；「封禁」按钮弹窗（形态选择：永久 / 自定义时长 + reason 输入 + 「同时断开在途连接」勾选默认勾）+「解封」按钮（确认链）；条目上封禁/解封与撤销并列
6. **ActivityBar 明细面板行**：封禁按钮与踢出并列（按行 tokenId 封禁，确认弹窗同款含联动勾选）
7. **联动踢出编排**：`useDeviceConnections` connections ref 按 tokenID 过滤逐个 KickConnection（桌面专用路径，浏览器通道无明细无此面）
8. wailsjs 三处手动同步（App.js/App.d.ts/models.ts）；wails-mock-defaults.js 补 BanWebToken/UnbanWebToken 默认值；error.js 同步新码

## Decision (ADR-lite)

### D1: 在途连接语义 = 基线拒新连 + 可选联动踢出（2026-10-24 brainstorm 定稿）

**Context**：验收初稿写「已建立连接不受影响」（仅拒新连），但异常设备「先踢先禁」场景不踢在途连接则封禁即时防护价值大减——当前连接仍可持续操作到主动断开。

**Decision**：封禁服务端语义 = 仅拒新连（基线，与撤销热生效语义一致，验收条款成立）；UI 确认弹窗带「同时断开在途连接」勾选（默认勾选），勾选后**前端编排**按 tokenID 过滤连接明细缓存逐个调 KickConnection（复用既有 connDetails，后端 BanWebToken 零额外参数）。不勾 = 严格「已建立连接不受影响」。

**Consequences**：撤销/封禁两管控动作服务端语义保持一致（热生效面均为新连接）；联动踢出为前端编排非原子——封禁与踢出之间新连接已被拒、旧连接短暂存活，无竞态危害仅时间窗；多 tab 同令牌须逐连接踢（明细缓存按 tokenID 过滤天然覆盖）。

### D2: 状态存储 = data/ 持久化（2026-10-24 brainstorm 定稿）

**Context**：封禁核心场景 = 管理员不在线时持续拒连可疑设备；内存态重启清零，服务重启/升级后可疑设备直接恢复可达，自毁封禁价值（个人工具迭代期重启频繁）。

**Decision**：`data/web_bans.json` 独立文件持久化（`util.SaveJSON` 原子写，对齐 web_tokens.json 模式），由 WebTokenService 持有（同 mu 锁、复用 audit 注入先例）。

**Consequences**：须做生命周期联动——撤销令牌时清理其封禁记录；持久化失败回滚内存（对齐 Revoke 先例）；重启后封禁持续生效。

### D3: 封禁列表宿主 = 令牌管理条目内嵌（2026-10-24 brainstorm 定稿）

**Context**：封禁列表查看/解封需要宿主；三期明细 popover 320px 空间教训。

**Decision**：SettingsPanel 令牌条目内嵌封禁徽标 + 封禁/解封按钮，封禁的令牌即列表可见子集，不建独立封禁列表弹窗；封禁弹窗含可选 reason（审计行与徽标 tooltip 展示）。

**Consequences**：零新弹窗、零两视图联动成本；封禁列表无独立过滤视图（列表项少，徽标扫读即可）。

### D4: 封禁判定接入 = verify 返回后组合判定，非 verify 内聚合（2026-10-24 定稿，Auto-Context 推定）

**Context**：两认证点均经 `authorized` 逐候选调 verify 返回 `(tokenID, ok)`。封禁判定两种接入形态。

**Decision**：verify 契约不动（凭据有效性单一语义），新增 `BanCheck func(tokenID) string` 独立注入面，两认证点在 authorized 命中后组合判定。

**Consequences**：封禁命中可返回含解封时间的专用提示（verify 内聚合会把封禁吞为普通认证失败、无法区分）；`data/web_tokens.json` 结构零变更（version 迁移省却）；server 层新增一个注入字段（对齐 verify/onAuth 既有形态）。

## Out of Scope

* IP 维度封禁（NAT 共享出口误伤面大，留待真实需求）
* 后台过期扫描（懒判定已覆盖）；过期封禁记录自动清理（留列表显示「已过期」态）
* 黑名单自动封禁（失败次数阈值联动等）
* 封禁独立过滤视图弹窗（D3）

## Technical Notes

* 已勘察：docs/spec/browser-channel.md §2.2/§2.3/§2.4、logging-and-errors.md §8/§8.1、server/web.go（requireToken/authorized/issueSessionCookie）、server/ws.go（authorized/ServeHTTP/DisconnectClient/ConnDetail）、service/web_token.go（Verify/Revoke/RevokeAllExcept/auditEventLocked）、app_webserve.go（kickConnectionAs/auditConnKick/rpcAuditTarget/webTokenNamedDetails/ListWebTokens）
* 复用面：KickAudit 结构化参数形态、rpcAuditTarget 装饰器（须扩 override）、WebTokenStatus join 形态、connDetails 缓存、nowFn 测试注入先例、SaveJSON 失败回滚先例
* 封禁命中不发 auth_success（认证未放行）；是否单独 auth 审计形态实现期定（最小面：auth_failure + tokenID 非空即封禁形态）
* reason 经 ListWebTokens 透出浏览器 RPC 可达——可接受（名册本就同构可达，与「已撤销」可见同理），非新增泄露面
* 隐私红线不回归：QueryAuditLog 恒 HiddenMethods；明细 IP/token 恒桌面出口

## Implementation Plan (small PRs)

* **PR1 后端**：model（WebBanRecord/WebTokenStatus 扩展/错误码）→ service（web_bans.json 状态机 + Ban/Unban/BanRejectMessage + Revoke 联动 + 审计出口）→ server（BanCheck 注入 + 两认证点判定 + 审计形态）→ App（BanWebToken/UnbanWebToken + rpcAuditTarget 扩 override + ListWebTokens join + 装配注入）+ 后端全量测试（-race）
* **PR2 前端**：wailsjs 三处同步 → SettingsPanel 条目（徽标/封禁弹窗/解封链）→ ActivityBar 明细行封禁按钮 → 联动踢出编排 → mock 默认值/error.js → vitest + E2E + spec 文档更新
