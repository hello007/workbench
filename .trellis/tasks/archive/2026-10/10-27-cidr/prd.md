# 连接管控八期：CIDR 网段手动封禁

## Goal

七期已落地 IP 手动封禁（单 IP 精确匹配）+ 可信网段白名单豁免，但封禁仍按单 IP——攻击源切换同网段 IP 即绕过。八期将七期 Out of Scope「CIDR 封禁网段」转正：管理员可封禁整个网段（如 203.0.113.0/24），与白名单豁免（放行网段）形成对称处置面。

## Requirements

1. **条目形态（独立列表）**：`IPBanService` 加 `cidrBans []*ipBanCidrEntry` 独立字段（预解析 `*net.IPNet` + 元数据：cidr 原串/bannedAt/bannedUntil/reason/actorSource/actorTokenID）。判定链：`isWhitelistedIP` 恒最先（七期定稿不动）→ `entries` 精确查 → `cidrBans` 线性扫 `Contains`，任一命中即拒，消息文案取更具体（精确条目优先于网段条目）。
2. **封禁入口**：新 `BanCIDR(cidr, expiresAt, reason, actorSource, actorTokenID)` 绑定方法。解析复用 `parseIPBanWhitelistEntry` 单点（CIDR 串或裸 IP 自动补 /32 //128 双栈，禁第二套口径）；白名单相交**不拒绝**（豁免优先级已保证语义正确）；白名单完全覆盖（整个网段都在豁免内）拒绝封禁返回错误。
3. **解封入口**：新 `UnbanCIDR(cidr, actorSource, actorTokenID) bool`，精确匹配单条目；审计复用 `ip_unban`，IP 字段存 CIDR 串。
4. **自动封禁交互（照常计数）**：`RecordFailure` 零改动——网段内 IP 认证失败照常进窗口、照常自动封禁（单 IP 条目与网段条目并存），strikes 照常递增。白名单是「信任」完全不计数，网段封禁是「拒斥」累积攻击证据，语义刻意分叉。
5. **持久化（复用 ip_bans.json）**：`IPBanRecord` 加 `kind` 字段（`"cidr"`；`omitempty` 缺省 `ip` 向后兼容 v1），`IPBanListVersion` 升 2，载入兼容 v1/v2 双版。CIDR 条目 `IP` 字段存 CIDR 串。manualVersion 快照-IO-复核、过期载入丢弃、util.SaveJSON 原子写、落盘失败回滚全机制沿用。
6. **List 覆盖可视化（后端算透传）**：`model.IPBanStatus` 加 `CoveredBy string`（`omitempty`），List 构造时后端计算（单 IP 命中网段条目填网段串；网段被更宽网段覆盖同理），口径与 RejectMessage 同源。排序：封禁中在前已过期在后，组内网段条目置前。前端只渲染徽标 + tooltip，禁前端复刻 Contains 判定。
7. **拒绝消息**：网段命中「该来源 IP 已被网段封禁（203.0.113.0/24，永久），请联系管理员解封」（临时档带自动恢复时刻，对齐七期格式）。
8. **前端**：`BanIPDialog` 扩单 IP/网段档位切换（网段输入同口径校验）；白名单交集前端预检提示「与白名单 N 条有交集，交集内 IP 不受此封禁影响」（utils/ipWhitelist.js 先例，仅提示面非正确性依赖）；ActivityBar 网段条目 kind 徽标 + 解封确认链（ElMessageBox 单次确认，文案带网段与影响面；全网段封禁确认文案升级「将拒绝除白名单与本机外的全部来源」）。
9. **RPC 面**：`BanCIDR`/`UnbanCIDR` 是动作面，不进 `RPCHiddenMethods`（三期定稿：纯桌面数据面才隐藏）；actor 归因复用 `rpcAuditTarget` override 先例（`banCIDRAs`/`unbanCIDRAs`）。
10. **审计**：复用 `ip_ban`/`ip_unban` 事件，IP 字段存 CIDR 串；审计在落盘成功后写（落盘失败回滚不留已发审计行，既有纪律）。

## Acceptance Criteria

* [ ] 两认证点（HTTP requireToken + WS ServeHTTP）网段命中即拒，401 消息含网段串与永久/到期标识
* [ ] 白名单豁免优先于网段封禁：豁免网段/回环内具体 IP 不受网段封禁影响（`isWhitelistedIP` 单点优先级不变）
* [ ] 精确条目与网段条目并存时判定与消息文案符合「更具体优先」
* [ ] 自动封禁零回归：网段内 IP 认证失败照常计数、照常触发单 IP 自动封禁
* [ ] 既有单 IP 封禁/七期测试零回归
* [ ] 持久化：v1 文件载入兼容；CIDR 条目重启后封禁保真；落盘失败内存回滚；过期条目载入丢弃
* [ ] `CoveredBy` 计算正确（单 IP 被覆盖/网段被更宽网段覆盖/无覆盖三态）
* [ ] BanCIDR 校验：非法网段拒绝（新错误码）、裸 IP 补 /32 //128、白名单完全覆盖拒绝、到期时刻早于当前拒绝
* [ ] 前端：档位切换、输入校验、交集提示、确认链（含全网段升级文案）、徽标渲染
* [ ] wailsjs 三处同步（App.js/App.d.ts/models.ts）+ 错误码三处同步（app_error.go + error.js ErrorCode/WARNING_CODES）+ models.ts 新字段
* [ ] spec 三文档更新（browser-channel.md §2.2/§2.3/§3/§4 + logging-and-errors.md §8 + CLAUDE.md 关键规则八期行，文档版本 v2.36）

## Definition of Done

* 单元测试覆盖：网段判定/判定链优先级/持久化双版兼容/审计 actor 归因/CoveredBy 三态/校验拒绝面
* `go test ./...` + 前端 `npm test` + E2E 绿
* spec 文档更新 + CLAUDE.md v2.36

## Decision (ADR-lite)

**Context**：七期 Out of Scope「CIDR 封禁网段」转正，六决策点需定案（条目形态/持久化/自动封禁交互/List 可视化/前端/解封）。

**Decision**（2026-10-27 用户确认）：
1. 独立列表 `cidrBans`，不混 `entries` map（failures/strikes 是单 IP 自动封禁语义，混存污染遍历与快照）
2. 复用 `data/ip_bans.json` 扩 `kind` 字段升 v2（同域同机制，独立文件纯增双源）
3. `RecordFailure` 照常计数（拒斥面累积证据 vs 白名单信任面不计数，刻意分叉；零特判零耦合）
4. `CoveredBy` 后端算透传（判定单点在后端，前端复刻违反「同口径仅提示面」纪律）
5. `0.0.0.0/0` 应急锁允许（回环恒豁免 + 白名单恒优先，本机不死；拒绝防不住 0.0.0.0/1+128.0.0.0/1 绕过，特判形同虚设），前端确认文案升级
6. `BanCIDR`/`UnbanCIDR` 独立方法（Wails 绑定签名清晰；不进 RPCHiddenMethods）

**Consequences**：
* `cidrBans` 线性扫在认证失败热路径（RecordFailure 不扫——零改动；RejectMessage 扫）——管理员手动条目量级小，无压力
* 多网段条目可重叠（任一命中即拒），不做包含/重叠合并——管理员心智模型简单，Out of Scope
* `kind` 字段使 v1/v2 双版载入分支永存——升级面小（文件有界自洁，过期条目载入丢弃加速收敛）

## Out of Scope

* 自动网段封禁（自动封禁仍恒单 IP）
* 封禁网段间包含/重叠合并与冲突消解（任一命中即拒已语义完备）
* 浏览器通道管理员 UI 变更（复用既有 RPC 动作面；桌面 ActivityBar 为唯一管理入口——对齐七期「封禁 IP」按钮 isDesktopChannel 锁定先例）
* 封禁条目导入导出

## Technical Notes

* 七期 spec：CLAUDE.md 关键规则「连接管控七期」行 + docs/spec/browser-channel.md §2.2/§2.3 七期段
* 网段解析复用 `parseIPBanWhitelistEntry`（service/ip_ban_whitelist.go，含裸 IP 补 /32 //128），禁第二套口径
* 判定链插入点：`RejectMessage` 白名单判定之后、`entries` 精确查之后补 `cidrBans` 扫描（精确优先语义）
* `Ban` 白名单拒绝语义对比：单 IP 封禁命中白名单直接拒绝（七期）；网段封禁与白名单**相交**合法（部分豁免由优先级保证），仅**完全覆盖**（网段 ⊆ 白名单并集）拒绝——封了也零效果
* 错误码候选：`E_IP_BAN_CIDR_INVALID`（网段格式非法）、`E_IP_BAN_CIDR_WHITELISTED`（网段被白名单完全覆盖）——命名对齐 `E_IP_BAN_WHITELISTED`/`E_IP_BAN_IP_REQUIRED` 先例
* wailsjs 三处同步清单：BanCIDR/UnbanCIDR 绑定（App.js/App.d.ts）+ IPBanStatus.CoveredBy/kind（models.ts）
* 测试平台无关（纯 net 包判定，无 Windows 特有面）
