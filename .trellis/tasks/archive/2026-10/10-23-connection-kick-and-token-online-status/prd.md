# 连接管控（踢出连接 + 令牌在线状态标记）

## Goal

补齐桌面端连接处置闭环（来源：10-23-desktop-device-connection-details 明细任务的 out-of-scope 延后项）：桌面明细面板（ActivityBar popover）当前只能看不能管——发现异常接入后无可处置手段，须到令牌管理页撤令牌，且撤令牌不影响已建立连接（要等对端重连才生效）。本任务新增两项能力：

1. **踢出单个连接**：桌面明细面板每行支持踢出（断开该 WS 连接）。
2. **令牌在线状态**：令牌管理列表显示各令牌的在线连接数。

## What I already know

### 代码探索结论（2026-10-23）

* `server/ws.go`：`WSHub.clients map[*wsClient]struct{}`，`wsClient.detail`（`ConnDetail{ID, IP, TokenID, TokenName, ConnectedAt}`，升级成功时一次赋值此后不可变，ID 为 hub 级 `connSeq` 自增）。
* `WSHub.Close()`（ws.go:316）已示范「只关 conn 不 close(send)」模式：`conn.Close` 使 readPump `ReadMessage` 返回错误，走既有 unregister（写锁下 delete + close(send)）收尾，天然防双重 close。**踢出可复用同一模式，零新增事件**——unregister 自动触发明细快照双出口（桌面事件 + 浏览器 count），行即时消失。
* `register/unregister` 锁序：mu 写锁内构造快照，**出锁后** `notifyConnectionDetails`（锁内直调 Emit 同 goroutine 重入 RLock 死锁——红线）。
* 装配：`webServeManager.startLocked`（web_serve.go:163）持 hub 引用，`SetOnConnectionsChange` 闭包经 `webTokenNamedDetails` 补 TokenName 后 emit 桌面事件；`stopLocked` 在 `applySink(nil)` 后显式补发空数组。**hub 引用不出 manager**——App 层踢出需 manager 暴露方法。
* 浏览器 RPC 路由 = reflect 全部 App 导出方法，桌面/浏览器同构暴露面，**无调用来源区分机制**——「浏览器端不可见不可踢」只能达成 UI 面无入口，RPC 层不特判也无法可靠特判（与停服务/撤令牌同能力面：同用户语义，认证由外层 token 把关）。
* `App.ListWebTokens`（app_webserve.go:180）返回 `[]model.WebTokenRecord`（全基本类型字段）；`SettingsPanel.vue:636` 消费。`WebTokenRecord` 直接参与 `data/web_tokens.json` 落盘序列化——在线数不应加在 record 上（运行时字段会污染落盘形态），应包装 struct。
* `useDeviceConnections.js`：`connections` ref 全量快照落地，桌面通道含 `tokenId`（前端可本地聚合 tokenID→count）。
* ActivityBar.vue 明细面板：el-popover `:persistent="false"`，行内 v-for 渲染；tooltip 外层 + popover 直挂原生 div 的嵌套约束已有注释（行内按钮不在 reference 链上，无此问题）。
* 被踢端：浏览器 transport 断线自动重连，令牌仍有效则**数秒内自动回连**（预期行为，UI 文案须说明）。
* Wails 绑定三处同步规则（App.js / App.d.ts / models.ts）适用于新绑定方法与返回类型变化。

## Assumptions (temporary)

* 踢出语义 = 断开当前连接（conn.Close），对端同令牌可重连，**不是封禁**（用户已定）。
* 防重连 = 撤令牌（现成手段，撤销即时生效于新连接）：踢出断存量连接、撤令牌断未来连接，两手段互补不互替（用户已定，MVP 不加黑名单）。
* 令牌在线数数据源 = SetOnConnectionsChange 回调缓存（App 层，用户已定）。

## Decision (ADR-lite)

**Context**：踢出后防重连手段选型、令牌在线数数据源选型。

**Decision**：
1. 不加临时 tokenID 黑名单——撤令牌已是现成防重连手段（撤销即时生效于新连接），黑名单是第三种重叠手段且引入新状态面（TTL/落盘/与撤销关系），无对应场景（个人单用户工具，异常接入直接撤令牌重发）。
2. 令牌在线数走回调缓存——`startLocked` 既有 `SetOnConnectionsChange` 闭包顺带写 App 层缓存（mutex 保护），`stopLocked` 补发空数组自动清零；`ListWebTokens` 返回包装 struct（record + onlineConns），单调用列表+计数原子一致。否决主动拉取（SettingsPanel 须新增事件重拉链 + 两次调用时序窗口破坏口径一致验收）与前端聚合（浏览器通道 connections 恒空致浏览器端令牌列表在线数恒 0，语义错误）。

**Consequences**：App 新增缓存状态面（回调 goroutine 写 + ListWebTokens 读，须 mutex）；包装 struct 变化 wailsjs models.ts 生成面，须三处同步 + wails-mock-defaults / SettingsPanel.spec mock 形状更新；后端聚合下浏览器端令牌列表也显示真实计数（count 级别，与 `ws-connections:changed` 同隐私级别，不违反红线）。

## Requirements (evolving)

* server 包新增按 connID 断开方法（mu 写锁内定位 clients、conn.Close 幂等、readPump unregister 自然收尾并触发明细事件，零新增事件）。
* Wails 绑定新增踢出方法（wailsjs 三处同步）；webServe 停止后踢出不可达不崩（hub nil 返回错误）。
* 浏览器 UI 零改动（无踢出入口、WS 帧无管控字段）；RPC 反射面不特判（同构暴露面，无来源区分机制）。
* 明细面板每行加踢出按钮（ElMessageBox 单次确认，含「对端可用同令牌重连」与「撤令牌不影响已建立连接，两手段互补」警示）。
* 令牌管理列表加在线连接数徽标；与明细面板行数口径一致。
* 被踢端走既有 transport 重连链路，无新增事件。

## Technical Approach

### 后端

1. **server 包踢出**：`WSHub.DisconnectClient(connID uint64) bool`——mu 写锁内遍历 clients 定位 `detail.ID == connID`，命中 `conn.Close()`（net 层非阻塞，锁内安全，对齐 `Close()` 既有模式）返 true；miss 返 false。readPump `ReadMessage` 感知断连，defer unregister（写锁下 delete + close(send)）自然收尾并触发明细快照双出口——**零新增事件**。重复踢出幂等（已注销连接不在 map，天然 miss）。
2. **manager 暴露**：`webServeManager.KickConnection(connID uint64) error`——mu 内取当前 hub 引用，nil（停机/未启用）返 AppError（错误码实现时定，或复用现有 webServe 态码），hub 活则委托 `DisconnectClient`，miss 返可读错误（连接已断开）。
3. **App 绑定**：`KickConnection(connID uint64) error` 委托 manager；wailsjs 三处同步（App.js / App.d.ts / models.ts——uint64 传 number）。
4. **在线数缓存**：App 层新增连接明细缓存（mutex 保护），`startLocked` 既有 `SetOnConnectionsChange` 闭包内写入（`webTokenNamedDetails` 旁路），`stopLocked` 补发空数组自然清零；`ListWebTokens` 返回 `[]model.WebTokenStatus`（包装 struct：`Record WebTokenRecord` + `OnlineConns int`，tokenId→count 本地聚合），record 落盘形态零污染。

### 前端

5. **ActivityBar 明细行踢出按钮**：行内小按钮（el-popover `:persistent="false"` 内容区，不在 reference 链上无嵌套约束问题）→ `ElMessageBox.confirm` 单次确认，警示文案含「对端可用同令牌自动重连（非封禁）」与「防重连请配合撤销令牌（撤销不影响已建立连接，两手段互补）」；确认调 `KickConnection`，错误走 `handleError` 分流。行消失靠明细事件自动更新（connections ref 全量落地），无手动删。
6. **SettingsPanel 令牌列表在线徽标**：`webTokens` 渲染处加在线连接数徽标（onlineConns > 0 显示），数据随既有 `ListWebTokens` 拉取链自动到位。

### 测试

* server：`DisconnectClient` 命中（连接断开 + 明细事件自动更新断言）/ miss / 幂等（重复踢）/ 踢出与并发断开竞态（-race）。
* App：`ListWebTokens` 在线数 join（含撤销令牌仍有在途连接形态——TokenName 空串同款）；manager 停机后 `KickConnection` 返回错误不崩。
* 前端：ActivityBar 确认交互与错误分流、SettingsPanel 徽标渲染；`wails-mock-defaults.js` 补 `KickConnection` 默认值与 `ListWebTokens` 新形状；E2E `settings-token.spec.js` 回归。

## Acceptance Criteria

* [ ] 桌面明细面板踢出指定连接后该行即时消失（明细事件自动更新），对端断开重连可回来。
* [ ] webServe 停止后踢出不可达不崩（返回可读错误）。
* [ ] 令牌管理列表在线连接数与明细面板行数口径一致（同一缓存快照 join）。
* [ ] 浏览器端零改动（无踢出入口、WS 帧无管控字段；RPC 反射面同构暴露不特判）。
* [ ] go test 全绿（含 -race：踢出与并发断开竞态）+ wailsjs 绑定三处同步 + 现有 vitest/E2E 回归。

## Implementation Plan

* PR1（后端闭环）：server `DisconnectClient` + manager/App 绑定 + 在线数缓存 + `WebTokenStatus` 包装 + 全部后端测试。
* PR2（前端）：ActivityBar 踢出按钮 + 确认交互、SettingsPanel 在线徽标、wailsjs 三处同步 + mock 更新 + vitest/E2E 回归。
* 收尾：docs/spec/browser-channel.md §2.3 契约更新（踢出方法 + 在线数缓存数据源）。

## Definition of Done

* 单测：hub 按 ID 踢出（命中/miss/幂等/踢出后明细事件自动更新）、踢出与并发断开竞态（-race）；App 层令牌在线数 join；前端确认交互与徽标渲染。
* Lint / typecheck / CI 绿。
* docs/spec/browser-channel.md 契约更新（踢出方法 + 在线数数据源）。

## Out of Scope (evolving)

* 临时 tokenID 黑名单（防重连用撤令牌，两手段互补不互替）。
* 踢出全部/批量踢出。
* 浏览器端踢出入口。

## Technical Notes

* 关键文件：`server/ws.go`、`web_serve.go`、`app_webserve.go`、`frontend/src/components/ActivityBar.vue`、`frontend/src/composables/useDeviceConnections.js`、`frontend/src/components/SettingsPanel.vue`。
* 前置阅读：docs/spec/browser-channel.md §2.3（双通道契约、隐私红线、锁序红线）。
* 前一任务：`.trellis/tasks/10-23-desktop-device-connection-details`（明细面板形态与本任务的直接基底）。
