# 连接管控五期:认证失败自动封禁(阈值联动 + 自动黑名单)

## Goal

四期落定手动封禁状态机,但封禁触发纯依赖管理员在场。本任务补自动化:认证失败频次达标即自动临时封禁来源 IP(短时限),补「管理员不在线时的被动安全」档位。四期 Out of Scope「黑名单自动封禁」留白转正。

## Requirements

1. **计数**:HTTP `requireToken` 与 WS 握手两认证点认证失败按请求计 1(多凭据候选天然单计);`auth_success` 即清该 IP 计数;WS 与 HTTP 合并计数;回环地址(127.0.0.1/::1)豁免
2. **封禁判定**:滑动窗口 10 分钟内 ≥10 次失败,自动封禁来源 IP 5 分钟;拒绝点在两认证点 verify **之前**(新注入面);封禁期间失败不再累计,期满重新计数
3. **到期恢复**:懒判定(禁后台扫描,对齐四期),过期记录不删(可见性保留到重启)
4. **IP 提取**:只认 RemoteAddr 剥端口(net.SplitHostPort),忽略 X-Forwarded-For(可伪造)
5. **手动解封**:`UnbanIP(ip)` 桌面绑定,语义复用四期(生效才写审计/幂等 no-op/解封审计行),方法签名独立(IP 无 tokenID)
6. **审计**:`ip_ban`/`ip_unban` 审计行,actorSource 扩 `system` 枚举(自动触发),携带来源 IP + 触发原因(窗口内失败计数);手动解封行为 actorSource=desktop;审计历史弹窗可过滤可见
7. **UI**:桌面 ActivityBar 明细面板加「自动封禁」区块(IP/触发时间/到期时间/窗口内失败计数 + 解封按钮),仅桌面出口(他人 IP 明细不进浏览器通道,三期隐私红线);到期懒消失(渲染时过滤)
8. **桌面 toast 通知**:自动封禁触发时桌面专用事件,前端提示管理员
9. **令牌轮换/撤销联动清零**:`RegenerateWebToken`/`Revoke`/`RevokeAllExcept` 后清空全部 IP 计数(轮换产生的旧 cookie 失败潮防误伤叠加)

## Acceptance Criteria

* [ ] 10 分钟内第 10 次失败触发自动封禁,此后新连接两认证点 verify 前即拒(401 专用消息含解封时间)
* [ ] 5 分钟到期懒恢复;封禁期间失败不累计;期满重新计数
* [ ] auth_success 清零该 IP 计数;回环地址不触发;单次失败不触发
* [ ] 单请求多凭据候选全失败仅计 1;令牌轮换后旧 cookie 单请求不误计 N
* [ ] 轮换/撤销令牌清空全部 IP 计数
* [ ] ip_ban 审计行含 system 归因 + 来源 IP + 触发原因(窗口计数),审计历史可过滤
* [ ] 桌面明细面板自动封禁区块展示/解封可用;UnbanIP 幂等 no-op;解封生效(新连接可达)
* [ ] 桌面 toast 触发提示;浏览器通道无 IP 明细
* [ ] go test 全绿(含 -race)+ vitest/E2E 回归 + spec 更新(browser-channel.md + logging-and-errors.md)

## Definition of Done

* 单测:阈值触发/窗口滑动剪枝/到期恢复(nowFn 注入禁 sleep)/回环豁免/多候选单计/成功清零/轮换清零/解封幂等/审计行形态/并发 -race
* server 两认证点注入面测试(漏一即绕过——对齐四期测试面)
* CI 绿;spec 文档更新;README 若涉及

## Technical Approach

**分层**(对齐四期 server 不知 service 红线):

- `service/ip_ban_service.go` 新建 `IPBanService`:内存态(mu 锁 + nowFn 注入 + 审计注入,对齐 WebTokenService 模式);API = `RecordFailure(ip)` / `RecordSuccess(ip)` / `RejectMessage(ip)` / `UnbanIP(ip, actorSource)` / `List()` / `ClearAll()`;滑动窗口 = 每 IP 时间戳环形队列,记录时剪枝;时间计算走单调时钟(防系统时间回拨);常量:阈值 10 / 窗口 10min / 时长 5min(硬编码,二期设置项)
- server 注入面:`WebOptions.IPBanCheck func(ip string) string`(对齐 BanCheck 先例),`requireToken` 与 WS `ServeHTTP` 开头(verify 前)判定,命中 401 专用消息「来源 IP 已被临时封禁,至 X 自动恢复」;**两认证点必须同接**(漏一即绕过,对齐四期)
- 计数驱动:复用 OnAuth 事件流(AuthEvent 已含 remoteAddr),装配层闭包消费 auth_failure/auth_success 驱动 RecordFailure/RecordSuccess;IPBanCheck 拒绝形态不计数(封禁中不累计语义)
- 审计:`AuditLogger.LogIPBan/LogIPUnban`(对齐 LogBan 形态,BanAudit 结构化参数防位置参数错位);actorSource 扩 `system`
- 桌面事件:`webserve-ip-banned` 桌面专用(对齐 webserve-connections:changed 同义不同名先例),载荷含 ip/bannedAt/expiresAt/failCount;浏览器通道不见(IP 隐私红线)
- 轮换联动:装配层在 RegenerateWebToken/Revoke/RevokeAllExcept 成功后调 `ClearAll()`
- 前端:ActivityBar 明细面板「自动封禁」区块 + toast 消费;错误码如需新错误码同步 model/app_error.go + frontend/src/utils/error.js 三处

## Decision (ADR-lite)

1. **封禁对象粒度 = IP 短时限自动封**(用户选定 A):auth_failure 哈希不命中拿不到 tokenID,IP 是唯一自动化键;四期反对的是 IP 手动长时封(管理员不在场无人解封),短时限自动到期消除该痛点。拒绝点 verify 之前,与四期 BanCheck(tokenID 维度,verify 后)并存两道独立判定。
2. **计数口径**(用户确认推荐包):计数点 = 认证点 ok=false 单一分支每请求计 1;auth_success 清零;滑动窗口 10min/10 次;封禁 5min 固定,期间不累计,期满重计数(持续攻击 = 5 分钟封禁循环,等效递增)。
3. **误伤防护**(代码/验收定稿):回环豁免;WS/HTTP 合并计数(单例计数器,防「HTTP 9+WS 9」绕过);IP 只认 RemoteAddr 剥端口,忽略 XFF。
4. **存储/UI/解封/参数化**(用户确认推荐包):内存态不落盘(重启清零,攻击间隙 ≤5min 接受);UI 宿主 = 桌面明细面板自动封禁区块(仅桌面出口);独立 UnbanIP 语义复用四期;参数一期硬编码。
5. **MVP 边界**(用户选定 1+2+3):主线 + 令牌轮换/撤销联动清零 + 桌面 toast 通知。

## Out of Scope

* 递增封禁(反复触发封越久)
* 设置项参数化(阈值/窗口/时长)
* IPv6 /64 前缀聚合(防 /128 漫游绕过——一期接受该绕过面)
* 公网/私网 IP 差异化阈值
* 封禁记录持久化(重启清零已接受)
* 全局速率限制(全员连坐,已否决)

## Technical Notes

* 前置阅读:docs/spec/browser-channel.md §2.2(BanCheck 注入契约)/§2.3(四期状态机)/§3 契约表/§4 错误矩阵;docs/spec/logging-and-errors.md §8(auth_failure 行形态 + token_ban 产出点「动作生效后才写审计」纪律)
* 关键文件:server/web.go(requireToken:209,authorized:243)/server/ws.go(ServeHTTP:440)/service/web_token.go(Ban:314/Unban:367/BanRejectMessage:415)/model/web_token.go(WebBanRecord:75)/model/app_error.go(E_TOKEN_* 先例)/service/audit_log.go(LogBan:144)/main.go(BanCheck 装配:187-205)/app_webserve.go(rpcAuditTarget 先例:317)
* 装配注意:web_serve.go applySink 七服务先例;新桌面事件须在 startLocked 注入、stopLocked 收口(对齐连接明细回调先例)
* 前端依赖:handleError 双参签名;wailsjs 三处同步(新增绑定方法时)
