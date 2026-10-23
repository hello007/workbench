# 多设备连接明细（连接数升级为设备明细列表）

## Goal

上一任务（10-23-desktop-device-connection-indicator）把浏览器连接数补到桌面 UI，但只有数量没有明细——多设备共享时无法识别「是谁连着」（IP / 用了哪个 token / 何时接入），异常接入不可发现。本任务把连接数升级为设备明细列表：桌面 ActivityBar 指示点击展开 popover 面板，逐连接展示 IP、token 名、接入时间，连接/断开即时增减。

## Requirements

* **明细粒度 = 逐连接快照（D1）**：每行一个 WS 连接，同 IP 多标签页多行（接入时间区分）；与 count 口径严格一致（`count = len(明细)`），无聚合层；重连即新连接条目（新 ID/新接入时间），不跨会话去重
* hub 记录每个在册连接的元数据：连接 ID（hub 级自增）、IP（`RemoteAddr` 剥端口）、tokenID、接入时间；采集点在 `ServeHTTP` 升级成功构造 `wsClient` 处（`authorized` 已返回 tokenID，零新增探测）；元数据构造后不可变
* 桌面回调出口 `SetOnCountChange` 签名升级为 `func([]server.ConnDetail)` 全量快照（D3：幂等可重放，count 由 `len` 派生）；快照在 register/unregister 的 mu 写锁临界区内构造出局部变量，出锁后触发（锁序红线不变）
* **桌面事件 `webserve-connections:changed` 载荷直接升级为明细数组（D3）**：`data[0]` 从 count 改为 `ConnDetail[]`（json tag 小驼峰）；弃双事件形态（无时序窗口）；`stopLocked` 归零补发同点位改为空数组（桌面列表清空）
* tokenID→token 名称映射在装配层做（startLocked 闭包持 `WebTokenService` 引用查名册，server 包不反向依赖 service）；映射 miss（token 已撤销）降级展示（TokenName 空 → 前端 fallback 文案「已撤销令牌」）
* 浏览器通道 `ws-connections:changed` 逐字节不变（仅 count 数字，无明细）——**隐私红线**：他人 IP/token 明细仅走桌面 Wails 出口
* hub 代际更替（SetWebServeConfig 重启）明细随新 hub 重建——天然成立（新 hub 空 clients，旧 hub 回调随 closing 抑制）
* 前端 `useDeviceConnections` 返回明细列表 ref；`deviceCount` 分流派生（桌面 = `len(明细)`，浏览器仍走 count 数字）；浏览器通道消费逻辑零行为变化
* ActivityBar：**el-popover 点击面板（D2）**——三列（IP / token 名 / 接入时间）排布，placement right 贴 48px 窄栏，IP 可复制，行数多可滚动，空态「无设备连接」；现状 el-tooltip 提示文案保留为 hover 层；浏览器端组件零改动
* Wails 绑定零新增（纯事件面；事件载荷类型不进 models.ts 生成面，前端手写 interface 对齐后端真实形状）

## Acceptance Criteria

* [ ] 桌面开启浏览器访问 → 多设备连接后 ActivityBar 点击展开明细面板（IP / token 名 / 接入时间），连接/断开即时增减
* [ ] 同 IP 多标签页显示多行（接入时间区分），count 徽标 = 明细行数
* [ ] webServe 停止后桌面明细清空（stopLocked 补发空数组）
* [ ] token 撤销后已连接会话保持，重连后明细名称降级展示不崩
* [ ] 浏览器端行为回归不变（WS 帧不含明细字段，现有 vitest/E2E 全绿）
* [ ] go test 全绿（明细快照回调/元数据采集/count=0 空数组透传/closing 抑制/hub 代际/manager Stop 收口用例）
* [ ] wailsjs 绑定零变更（如实施中被迫变更须三处同步）

## Definition of Done

* 后端单测：明细快照回调（字段断言）、元数据采集（IP 剥端口/tokenID/接入时间）、空明细补发、closing 抑制、hub 代际重建
* 前端 vitest：useDeviceConnections 明细消费（非数组载荷忽略/桌面通道锁定）、ActivityBar popover 渲染与空态
* E2E：settings-token.spec.js 事件驱动链回归（mock 载荷同步改数组，走 wails-mock-defaults 单一数据源）
* docs/spec/browser-channel.md §2.3 更新（桌面出口段扩明细契约 + 锁序红线段落复核）
* CLAUDE.md 关键规则行同步（browser-channel 行更新载荷形态描述）

## Decision (ADR-lite)

**D1 明细粒度 = 逐连接快照**
Context：现状 count 口径 = WS 客户端数（含同设备多标签页），明细需定粒度。
Decision：逐连接一行，不做 IP 聚合。
Consequences：与 count 严格一致（`count = len(明细)`）零换算；「设备」语义弱化（同机多标签页多行）可接受——异常接入识别看 IP 列即可；UI 后续可加分组不作承诺。

**D2 展开形态 = el-popover 点击面板**
Context：明细三列信息（IP/token 名/接入时间）tooltip 塞不下且无法复制。
Decision：点击徽标弹 popover 面板，tooltip 保留为 hover 提示层。
Consequences：可复制、稳定驻留、可滚动；ActivityBar 交互多一层（点击），follow frontend-visual-conventions 面板规范。

**D3 桌面事件载荷直接升级为明细数组**
Context：`webserve-connections:changed` 载荷 count → ConnDetail[] 为破坏性变更，备选是新增 details 事件双轨。
Decision：直接改载荷，事件名不变；count 派生 len。
Consequences：单事件单真相无时序窗口，stopLocked 单点补发；消费单点（useDeviceConnections/ActivityBar/测试）全在本仓，前后端同版本分发无兼容收益，破坏面可控。

**ConnDetail 放 server 包**：采集点与回调出口均在 server，model 包无绑定方法引用则 models.ts 不涉及；前端手写 interface（fixture 对齐真实形状）。

## Out of Scope

* 浏览器端查看明细（须单独 brainstorm 定界：浏览器可看自身连接信息，但他人 IP/token 属隐私面）
* 连接历史持久化 / 审计查询 UI（认证审计 JSONL 已在 data/web_audit.log，不建查询面）
* 按 IP 去重的跨会话「设备」身份识别（重连即新连接条目，不做会话粘性）
* 明细手动踢出连接（断开某设备）能力

## Technical Notes

### 关键实现点

* `wsClient` 扩字段：`connID uint64`（hub 级自增，register 写锁内分配）、`ip string`、`tokenID string`、`connectedAt time.Time`——ServeHTTP 构造时赋值，不可变
* `notifyConnectionCount` 演进：写锁临界区内遍历 clients 构造 `[]ConnDetail` 快照出局部变量，出锁后回调（Emit 浏览器侧仍发 count int，不变）
* `stopLocked`：`Emit(webServeConnectionsEvent, 0)` → `Emit(webServeConnectionsEvent, []server.ConnDetail{})`（空数组 marshal 为 `[]` 非 null，前端 `Array.isArray` 校验）
* startLocked 闭包：收 `[]ConnDetail` → 查名册填 TokenName → emit；名册查询用 `WebTokenService.List()` 建 ID→Name map（每次回调重建，量小无碍）
* `RemoteAddr` 剥端口：`net.SplitHostPort`，失败原样保留（Unix socket 形态兜底）
* 接入时间序列化：`time.Time` → RFC3339 字符串（json tag 小驼峰 `connectedAt`），前端 `new Date()` 解析格式化
* 前端 interface：`{ id, ip, tokenId, tokenName, connectedAt }`；`tokenName` 空串 fallback「已撤销令牌」
* 前端初始态：明细 `[]`，桌面通道未锁定时浏览器口径不变（deviceCount 初始 1 语义保留）
* 既有窗口接受态：ActivityBar 重挂载（HMR）期间已建立连接的事件丢失致明细空——与现状 deviceCount 同窗口，不在本任务收口

### 勘察结论（已完成）

* `server/ws.go`：wsClient 三字段、notifyConnectionCount 双出口、锁序红线（mu 写锁外触发回调）
* `web_serve.go`：startLocked 闭包注入点（182-185 行）、stopLocked 归零补发点（243-245 行）
* `service/web_token.go`：`List() []model.WebTokenRecord` 名册已有，tokenID 即记录 ID
* `useDeviceConnections.js`：双通道消费单点、isDesktopChannel 锁定、非法载荷防御模式
* `ActivityBar.vue`：el-tooltip + 徽标现状（39-46 行）、useDeviceConnections 消费（76 行）
* models.ts 生成规则：生成源仅绑定方法签名引用的类型，纯事件载荷不自动生成

### 测试参考（既有基线）

* `TestWSHub_OnCountChangeCallback` / `TestWSHub_CloseSuppressesOnCountChangeCallback` / `TestWSHub_OnCountChangeNilSafe`（server/ws_test.go，随签名升级改造）
* `TestWebServeManager_StopClosesWSClients`（manager 级收口归零形态）
* 前端 `useDeviceConnections` composable 用例 + ActivityBar 组件用例 + E2E settings-token.spec.js
