# 连接管控七期：IP 手动封禁 + 可信网段豁免

## Goal

补连接管控处置面缺口（五/六期 Out of Scope 转正）：

1. **手动封禁 IP**：管理员发现攻击源可主动处置（即时生效，永久/天级/自定义时长），不再只能等自动封禁触发。对齐四期 BanWebToken 形态（expiresAt 0=永久、actorSource 归因分叉 desktop/rpc）。
2. **CIDR 网段白名单豁免**：可信来源（NAT 共享出口 IP 场景）认证失败不计不封，回环之外唯一豁免口径单点。

## Requirements

### R1 手动封禁（service 状态机）

- `IPBanService.Ban(ip, expiresAt, reason, actorSource, actorTokenID)`：手动封禁入口。expiresAt 0=永久，否则为到期 unix 秒。
- `ipBanEntry` 扩 `source` 字段（manual/auto）；手动封禁创建/覆盖条目置 manual。
- 手动封禁覆盖在途自动封禁（BannedAt/bannedUntil/reason/归因整体覆盖，对齐四期「重复封禁=刷新参数」）；不动 strikes（管理员决策非攻击信号，不参与递增）；曾有自动史则 strikes 保留。
- 自动遇手动在途：`RecordFailure` 封禁中早退天然覆盖（失败不累计、不会自动再封），剪枝 cutoff 逻辑通用无分叉。
- 封禁白名单/回环 IP 拒绝（新错误码，warning 引导级）。
- 持久化：手动条目落 `data/ip_bans.json`（util.SaveJSON 原子写、损坏降级空列表 + slog.Warn，对齐 web_bans.json 四期先例）；仅手动封禁/解封触发落盘，自动封禁/失败计数纯内存；启动载入仅期内条目，**已过期手动条目载入时丢弃**（文件有界自洁；「已过期」态仅运行期内可见，懒过期语义不变）。
- 解封：`UnbanIP` 单点不分叉（删条目通吃 manual/auto，含归零 strikes），审计 ip_unban 归因已有。
- 即时生效：写入同一 entries map，两认证点 IPBanCheck 判定零改动自动覆盖。

### R2 白名单豁免（CIDR）

- `IPBanSettings` 扩 `whitelist []string`（CIDR 串；`EnsureIPBanDefaults` 补默认空列表——现四处 Ensure 须同步）。
- 豁免判定单点 `isWhitelistedIP(ip)` = 回环 || CIDR 命中（回环豁免收编同函数，「回环之外唯一豁免口径单点」落地）；裸 IP 自动补 /32//128；IPv4/IPv6 双栈。
- 三处消费：`RecordFailure` 入口早退（白名单失败完全不计数不进队列）；`RejectMessage` 入口先查（命中返空串放行）；`BanIP` 入口校验（封白名单/回环拒绝）。
- 生效中封禁追溯放行（豁免=语义修正；在途记录不删，白名单移除且未过期即恢复拒连）。
- 热更：`SetIPBanConfig` 扩第五参 `whitelist []string`，「保存+生效」显式单点延续（normalize→复制基底换段→Save→`ApplyConfig`）；normalize 剔除非法 CIDR 条目（clamp 收敛哲学），UI 保存后回读归一值刷新表单。
- `ApplyConfig` 锁内预解析存 `[]*net.IPNet` 整体替换（判定热路径零重复解析）。

### R3 绑定与归因（App 层）

- `BanIP(ip string, expiresAt int64, reason string) error` 对齐 `BanWebToken` 三参形态，wailsjs 三处同步。
- 桌面恒 `banIPAs(ip, ..., "desktop", "")`；`rpcAuditTarget` 扩 override 委托未导出 `banIPAs(ip, ..., "rpc", actorTokenID)`。
- `ListIPBans` 继续 `RPCHiddenMethods` 屏蔽（零改动）；`model.IPBanStatus` 扩 `source` 桌面出口。
- 白名单经 SetIPBanConfig 浏览器通道可达延续（不新增暴露面类别）。

### R4 审计

- 复用 `ip_ban` 单事件（actorSource desktop/rpc=手动、system=自动；AuditRecord 零新增字段，不设 ip_ban_manual）。
- reason：手动非空用输入值，为空生成「管理员手动封禁（永久）」/「管理员手动封禁，至 <时刻>」；自动文案六期格式不动。
- 手动封禁 failCount=0 / strike 不携带（omitempty）。
- 「动作生效（写内存+落盘成功）后才写审计」纪律延续：落盘失败回滚内存不写审计。
- 手动封禁不触发 `webserve-ip-banned` toast（onBanned 自动专属，前端 Promise 重拉）。
- 白名单变更不写审计（对齐六期参数变更口径，明确取舍）。

### R5 前端

- `BanIPDialog` 复刻四期 `BanTokenDialog` 三档：永久 0 / 天级 N 天（默认 1）/ 自定义日期时间（el-date-picker 到时分，未选日期前置拦截）+ IP 输入 + 可选 reason + 白名单命中即时校验提示。
- ActivityBar「自动封禁」区块更名「IP 封禁」，头部加「封禁 IP」按钮；条目 source 徽标（自动=含 strike「第 N 次」、手动=「手动」）；浏览器通道入口隐藏（isBrowserChannel 先例）。
- SettingsPanel「自动封禁」小节扩展 CIDR 列表编辑器（动态行增删 + 行级格式校验 + 保存并生效同按钮走 SetIPBanConfig 五参；空列表「无可信网段」占位）。

## Acceptance Criteria

- [ ] 手动封禁后新连接（HTTP+WS 两认证点）即拒，401 消息含解封时刻；永久封禁消息含「永久」；对 verify 前判定语义无回归。
- [ ] 手动封禁覆盖在途自动封禁（bannedUntil 顺延、source 置 manual、strikes 不变）；解封删条目归零 strikes；重启后手动条目仍生效、已过期条目不载入。
- [ ] 白名单网段命中 IP 认证失败不计数不触发封禁（failures 队列无痕）；已封禁 IP 加入白名单即放行，移除且未过期恢复拒连；封白名单/回环 IP 被拒（新错误码）。
- [ ] 白名单热更即时生效；非法 CIDR 条目被剔除且回读刷新；IPv4/IPv6/掩码边界用例绿。
- [ ] 审计 ip_ban 行手动封禁 actorSource=desktop/rpc + actorTokenId 归因，reason 区分 manual/auto；落盘失败不写审计不生效。
- [ ] SetIPBanConfig 五参保存+生效联动（含白名单），越界/非法输入 clamp/剔除，其他段保留。
- [ ] go test 全绿（含 -race）+ vitest/E2E 回归 + spec 更新（browser-channel.md §2.2/§2.3/§3/§6 + logging-and-errors.md §8 + CLAUDE.md 规则行 v2.34→v2.35）。

## Definition of Done

- 单测/E2E 按 §6 测试节清单补齐（nowFn 注入禁 sleep；持久化用例走 temp dataDir）。
- Lint / CI 全绿；wailsjs 绑定三处手动同步核验。
- spec 文档更新 + CLAUDE.md 关键规则行追加。

## Technical Approach

分层推进，server 包零改动（IPBanCheck 判定闭包读同一 entries map，手动封禁写入即生效）：

- **model**：`IPBanSettings` 扩 `whitelist []string` + `NormalizeIPBanSettings` 扩条目剔除；`IPBanStatus` 扩 `source`；新错误码。
- **service**：`IPBanService` 扩 Ban 方法 + `ipBanEntry.source` + 持久化（`data/ip_bans.json` Load 合并/Save）+ `isWhitelistedIP` 单点（收编回环豁免）+ `ApplyConfig` 扩 whitelist 预解析。
- **App**：`BanIP`/`banIPAs` + `rpcAuditTarget` override + `SetIPBanConfig` 扩五参。
- **前端**：BanIPDialog 新组件 + ActivityBar 区块改造 + SettingsPanel 白名单编辑器 + composable 扩展。

## Decision (ADR-lite)

**Context**：手动封禁与自动封禁共处一 map，持久化与否、strikes 交互、白名单形态、审计口径均有多解。
**Decision**：8 组决策全定稿（见 Requirements 内定稿标注）——手动条目持久化（A）；同 map 共存 source 字段/手动不动 strikes/覆盖刷新/UnbanIP 单点；三档时长复刻四期弹窗；CIDR 白名单三处消费单点判定 + 追溯放行；SetIPBanConfig 扩五参热更 + normalize 剔除；ip_ban 单事件复用；RPC 动作面可达数据面屏蔽；UI 宿主 ActivityBar+SettingsPanel。
**Consequences**：手动条目持久化引入服务层落盘面（仅手动动作触发，写放大可控）；过期条目跨重启不留历史（列表语义微变，文档记录）；白名单经浏览器通道可达（明确取舍，令牌有效时 IP 封禁非主防线）。

## Out of Scope

- X-Forwarded-For 解析（可伪造，五期已否决，延续）。
- 自动封禁持久化（重启清零语义不变）。
- 白名单命中时的审计/日志（豁免为静默放行）。
- 封禁黑名单跨机器同步、IP 段手动批量封禁（CIDR 封禁——白名单豁免网段 ≠ 封禁网段，封禁仍按单 IP 精确匹配）。

## Technical Notes

- 前置阅读已核：docs/spec/browser-channel.md §2.2（IPBanCheck verify 前判定/HiddenMethods）+ §2.3（五期状态机/六期参数化段）+ §3 契约表 + §6 测试节；docs/spec/logging-and-errors.md §8（审计事件表）；service/ip_ban_service.go 全文；app_webserve.go 四期 banWebTokenAs/五期 unbanIPAs 先例。
- 两认证点（HTTP requireToken / WS 握手）IPBanCheck 注入已接，手动封禁零 server 改动。
- `EnsureIPBanDefaults` 现四处（Load/Save 双链路两处 + 其他两处），扩 whitelist 默认值须同步核验。
- 持久化文件名 `data/ip_bans.json`（与 data/web_bans.json 命名对齐）。

## Implementation Plan (small PRs)

- **PR1 service+model**：ipBanEntry.source / Ban 方法 / 持久化载入 / isWhitelistedIP 单点 / IPBanSettings.whitelist+normalize / IPBanStatus.source / 错误码 / 全量单测（service+model）。
- **PR2 App 层**：BanIP/banIPAs / rpcAuditTarget override / SetIPBanConfig 五参 / wailsjs 三处同步 / App 层单测。
- **PR3 前端**：BanIPDialog / ActivityBar 区块改造 / SettingsPanel 白名单编辑器 / composable / vitest。
- **PR4 收尾**：E2E 用例 / spec 更新三文档 / 全量回归（go test -race + vitest + E2E）。
