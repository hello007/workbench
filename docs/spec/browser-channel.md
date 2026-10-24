# 浏览器访问通道契约

> 适用范围：WorkBench 浏览器访问模式（桌面同开 HTTP 与 `--serve` 无头）涉及的全部跨层契约：事件出口（EventSink）、RPC 翻译层（/api/rpc）、WebSocket 事件 hub、token 认证与热轮换、断线重连续传、前端 transport polyfill。改动本通道任何一环前必读。
> 来源：v1.7 浏览器访问模式 epic（任务 `.trellis/tasks/09-21-agent`，2026-09）；审核缺陷修复补强（任务 `.trellis/tasks/09-22-browser-audit-fixes`，2026-09）；断线重连续传（任务 `.trellis/tasks/10-22-ws-reconnect-resume`，2026-10）。

## 1. Scope / Trigger

以下改动触发本文档约束：

* service 层新增/修改向前端推送的事件
* 新增/修改 App 绑定方法（浏览器经 /api/rpc 可达）
* /api/rpc 请求-响应协议、WS 帧格式、token 校验通道变更
* WS 续传机制（序号分配、环形缓冲、resume 握手、控制帧）变更
* settings.json `webServe` 段结构变更
* webServeManager 生命周期（Start/Stop/Wait/令牌轮换）与预览路由认证语义变更

## 2. Signatures

### 2.1 事件出口（service 层，唯一合法通道）

```go
// service/event_sink.go
type EventSink interface { Emit(name string, data ...any) }
func NewWailsEventSink(ctx context.Context) EventSink   // 桌面实现，内部 runtime.EventsEmit
func emitEvent(sink EventSink, name string, data ...any) // 集中 nil/非 Wails ctx 防护

// service/sink.go —— 需运行时切换出口的 service 嵌入此持有器
type sinkHolder struct{ sinkMu sync.RWMutex; sink EventSink }
func (h *sinkHolder) SetEventSink(s EventSink)
func (h *sinkHolder) eventSink() EventSink                  // 仅测试/诊断读
func (h *sinkHolder) emitCurrent(name string, data ...any)  // 发射唯一入口：持 RLock 取值+投递
```

三个实现：`wailsEventSink`（桌面）、`WSHub`（serve 广播，`server/ws.go`）、`multicastSink`（桌面同开 HTTP 时 Wails+hub 双出口，`web_serve.go`）。

### 2.2 RPC 翻译层（server/rpc.go）

```text
POST /api/rpc            （须经 token 中间件）
请求: {"method":"<Go导出方法名>","args":[<位置参数>]}   body 上限 32MB（http.MaxBytesReader）
成功: {"ok":true,"data":<首返回值>}
失败: HTTP 200 {"ok":false,"error":{"code":"...","message":"..."}}
```

路由 = `reflect.ValueOf(target).MethodByName`，target 为 `*App`（暴露面=全部导出方法，与桌面 webview 一致，认证由外层 token 把关）。参数逐位 `json.Unmarshal` 进形参类型。

**调用者身份注入与 per-request target（2026-10-24 连接管控三期）**：`requireToken` 认证成功后将 `tokenID` 注入请求上下文（私有键 `rpcActorKey`，只注归因键不注明文凭据）→ `rpcHandler`（handler 已由闭包改为结构体，`NewRPCHandler(target)` 静态形态保留兼容）经 `WebOptions.RPCTargetForActor func(actorTokenID string) any` 装配回调按请求构造装饰 target——main 包 `rpcAuditTarget{ *App; actorTokenID }` embed 提升其余全部导出方法、仅 override `KickConnection` 委托未导出 `kickConnectionAs(connID, "rpc", tokenID)`（未导出方法不入绑定面/反射面，导出面签名零变更；无身份回落静态 target = desktop 语义；提升语义由 `TestApp_KickConnection_RPCActorAudit` 的 GetAppVersion 提升路径断言钉住）。**HiddenMethods 屏蔽（`WebOptions.RPCHiddenMethods []string`，生产值 `["QueryAuditLog"]`）**：命中返回与「方法不存在」同响应（同错误码 `E_RPC_METHOD_NOT_FOUND`、同消息模板），不暴露被屏蔽方法存在性。这是对「同构暴露面不特判」原则的首个**有据偏离**：该原则指管控动作面（动作语义两端一致），此处为数据隐私面——审计读侧若同构可达，浏览器通道可直调获取全谱 IP/令牌明细，破「他人 IP/token 明细仅走桌面出口」红线，红线优先。桌面 Wails 绑定不经翻译层不受屏蔽影响；后续「纯桌面数据面」新增方法复用该机制（追加 HiddenMethods 列表即可）。

### 2.3 WebSocket 事件 hub（server/ws.go）

```text
GET /ws  —— Sec-WebSocket-Protocol: <t>  /  wb_token cookie  /  ?token=<t>
                                                               （升级前 401 拒绝，勿升级后踢）
帧: {"event":"<事件名>","data":[...]}    零载荷 data 为 [] 非 null
func (h *WSHub) Close()   // 停机关闭：写锁遍历 clients 逐个 conn.Close，send 由 readPump unregister 收尾
```

浏览器 shim 以 `callback(...frame.data)` 还原 Wails `EventsOn` 回调形态。

**连接数事件（`ws-connections:changed`，2026-10 三期 PR3 多设备语义明示）**：每次客户端 register/unregister 成功路径广播一条 `{"event":"ws-connections:changed","data":[<事件后连接总数>],"seq":<N>}`。三个抑制/抑制态：count=0（最后一个连接断开，无接收者）不广播；hub `Close` 收口期（`closing` 标记）不广播；**锁序红线**——register/unregister 持 mu 写锁，`Emit` 内部按 seqMu → mu.RLock 加锁，必须在**释放 mu 写锁后**调用 `notifyConnectionDetails`（明细快照在写锁临界区内构造出局部变量，出锁后发射——浏览器载荷恒为 count 数字，他人 IP/token 明细不进浏览器通道，锁内直调 Emit = 同 goroutine 重入 RLock 死锁）。事件经常规 Emit 进环形缓冲占用 seq，resume 补发面包含该事件（计数为幂等快照，最后到达值即当前态，续传契约不变）；并发连接/断开下事件发送顺序可能与计数变化顺序不一致（各自独立加 seqMu），仅信息提示用途可接受。桌面 Wails 出口不产生该事件，前端监听空安全（`useDeviceConnections` composable 单点消费，多组件共听经 EventsOn 闭包精准注销）。

**桌面连接明细回调出口（`webserve-connections:changed`，2026-10-23 桌面多设备指示任务，同日明细列表任务升级载荷）**：hub 计数口径 = 浏览器 WS 客户端数，桌面恰是发起共享的一方，视角与浏览器不同——桌面 = 纯浏览器连接列表（指示条件 `len>=1`，且须接收空数组熄指示并清空明细），浏览器含自身连接（`count>1`），故桌面专用事件名与浏览器通道**同义不同名刻意分叉**（同名会造成桌面 1 浏览器连接时指示不显示的 off-by-one + 前端 `n>=1` 校验吞 0）。链路：`WSHub.SetOnConnectionsChange(fn func([]ConnDetail))`（全量明细快照回调，幂等可重放，count 由 `len` 派生——单事件单真相无两事件时序窗口；server 包不知 Wails，分层红线；须在 ServeHTTP 开始服务前注入，启动后热更不做线程安全承诺）→ register/unregister 在 mu 写锁临界区内经 `snapshotDetailsLocked` 构造快照（按连接 ID 升序 = 接入顺序；`ConnDetail{ID, IP, TokenID, TokenName, ConnectedAt}` 元数据在 ServeHTTP 升级成功时一次赋值此后不可变，ID 为 hub 级自增、IP 为 RemoteAddr 剥端口），**出锁后**、`!closing` 时触发（**空快照也透传**，与 WS 广播侧 count=0 抑制态相互独立）→ manager 装配层（`startLocked` 在 `applySink(hub)` 后）注入闭包，闭包外构造一次 `service.NewWailsEventSink(m.app.ctx)` 捕获，载荷经 `App.webTokenNamedDetails` 补全 TokenName（tokenID→名称映射在装配层查 `WebTokenService.List()`，server 不反向依赖 service；miss = 令牌已撤销/webTokenSvc 未装配 → TokenName 空串，前端 fallback「已撤销令牌」）后 emit `{webServeConnectionsEvent, details}`（`--serve` 无头对非 Wails ctx 静默跳过，无新分支）。**隐私红线**：他人 IP/token 明细仅走本桌面事件，浏览器通道 `ws-connections:changed` 载荷恒为 count 数字（`len(details)`）逐字节不变。停机收口：`hub.Close` 后回调受 closing 抑制不再触发，`stopLocked` 在 `applySink(nil)` 后经 Wails 出口**显式补发空数组** `[]server.ConnDetail{}`（非 nil——marshal 为 `[]` 非 null，前端 `Array.isArray` 校验通过；浏览器仍连接时停机桌面须清空明细）。前端 `useDeviceConnections` 双通道消费：桌面事件到达即锁定通道归属（`isDesktopChannel`），`connections` ref 原样落地明细、`deviceCount` 分流派生（桌面 `len(明细)` / 浏览器 count 数字），桌面载荷校验 `Array.isArray`（非数组忽略且不锁定通道），浏览器通道消费逻辑逐字节不变（桌面事件浏览器端永不到达，无需模式探测旗标）。Wails 绑定零新增（纯事件面，`ConnDetail` 不进 models.ts 生成面，前端手写 interface 对齐后端 json tag：id/ip/tokenId/tokenName/connectedAt）。

**连接管控（踢出连接 + 令牌在线数，2026-10-23 连接管控任务；二期补踢出审计与被踢端感知，2026-10-24）**：桌面明细面板每行可踢出（断开该 WS 连接，**非封禁**——对端同令牌可重连，防重连由撤销令牌承担：撤销即时生效于新连接、不影响已建立连接，两手段互补不互替，MVP 无黑名单）。链路：`WSHub.DisconnectClient(connID uint64) (ConnDetail, bool)`（mu 写锁内遍历 clients 定位 `detail.ID`，命中后**不再直接 conn.Close**：经 `enqueueKickFrames` 向该连接 send 队列非阻塞入队（1）`ws-kicked` 单播控制帧（无 seq、不进环形缓冲）与（2）nil 哨兵——writePump 单写者按 FIFO 先写出踢出帧、见哨兵即发 RFC Close 帧后退出（defer conn.Close 收尾），被踢端**先收到被踢提示帧再感知连接关闭**（单写者内保序）；readPump 感知断连走既有 unregister 收尾并触发明细快照双出口，**零新增广播事件**。降级 best-effort 口径：入队失败（队列满=慢消费者形态，或哨兵单独入队失败的极窄窗口）回退直接 `conn.Close()`，踢出帧丢失退化为普通断开，**踢出断开本身恒生效**；踢出帧不经 Emit，resume 补发面不含该帧（防重连幻影回放「被踢」）。返回值：ok=false 为 miss；ok=true 时 detail 为被踢连接元数据快照（审计留痕数据源）。幂等：已注销连接 miss 返 false；哨兵已入队、readPump 尚未收尾的窄窗口内重复踢出命中在册连接，`wsClient.kicked` 标记（mu 保护）挡住二次入队（writePump 自关路径垃圾帧防御），detail 照常返回 ok=true）→ `webServeManager.KickConnection` 透传 detail（mu 内取 hub 引用即释放，nil 返 `E_WEBSERVE_NOT_RUNNING`、miss 返 `E_CONNECTION_NOT_FOUND`，均 warning）→ App 绑定 `KickConnection(connID uint64) error`（对外签名不变，wailsjs 三处零 diff；**命中后写审计 `conn_kick`**——对齐「动作生效后才写审计」纪律，miss/停机不写，桌面按钮与浏览器 RPC 反射面同走本方法单咽喉全覆盖，见 logging-and-errors.md §8 审计事件表）。浏览器 UI 零改动（无踢出入口、WS 帧无管控字段；RPC 反射面同构暴露不特判——与停服务/撤令牌同能力面，认证由外层 token 把关）。**操作者归因（2026-10-24 三期）**：`App.KickConnection` 恒走 `kickConnectionAs(connID, "desktop", "")`（桌面路径），浏览器 RPC 经 per-request 装饰 target（`rpcAuditTarget` override，见 §2.2）捕获调用者令牌 ID 走 `kickConnectionAs(connID, "rpc", actorTokenID)`——审计行 `actorSource`（desktop|rpc）+ `actorTokenId`（RPC 调用者记录 ID，桌面空不落键）与被踢目标字段（`tokenId`/`name` 仍为受害侧）正交，多令牌场景可回答「谁踢的」；goroutine-local hack 无官方 API、签名加参动 wailsjs/RPC 位置参数契约、翻译层双写点破单咽喉，三者均否决，装饰器方案零侵入面胜出。**令牌在线数**：App 层连接明细缓存（`connDetails` + `connDetailsMu`，回调 goroutine 写 + ListWebTokens 读），`startLocked` 既有 `SetOnConnectionsChange` 闭包内 `rememberConnDetails(named)` 写入，`stopLocked` 因 hub 回调被 closing 抑制须**显式写空快照清零**（与空数组补发同处）；`ListWebTokens` 返回 `[]model.WebTokenStatus`（`Record WebTokenRecord` + `OnlineConns int`，tokenId→count 本地聚合，列表+计数单调用原子一致，与明细面板行数同源；已撤销令牌仍有在途连接 = `revoked:true` 且 `OnlineConns>0`，与明细 TokenName 空串同款语义；**不参与落盘**——`data/web_tokens.json` 仍以纯 `WebTokenRecord` 序列化零污染）。前端：ActivityBar 行内踢出按钮（popover 内容区直挂，不在 reference 链上无嵌套约束）走 `ElMessageBox.confirm` 单次确认（文案含可重连与两手段互补警示）+ `handleError` 分流；SettingsPanel 令牌列表条目形状改 `WebTokenStatus`（`item.record.*` 取值）+ 在线徽标（`onlineConns>0` 显示 success tag）；被踢端感知见 §2.4 踢出控制帧与 connBanner 被踢态。

### 2.4 断线重连续传（server/ws.go，2026-10 新增）

```text
事件帧: {"event":"<名>","data":[...],"seq":<N>}    seq 为 hub 级单调序号（自 1 起，omitempty）
控制帧: {"event":"ws-gap","data":[<服务端当前序号>]}      断层标记（无 seq）
        {"event":"ws-synced","data":[<补发快照当前序号>]} 补发完成标记（无 seq）
        {"event":"ws-kicked","data":[]}                   被踢单播帧（无 seq、不进环形缓冲，2026-10 二期）
握手帧: {"resume":<客户端已收最大序号>}                    客户端 → 服务端，唯一业务上行
```

**被踢端感知（`ws-kicked` 单播帧，2026-10-24 连接管控二期）**：帧仅送达被踢连接（hub 首个非广播投递面，单播天然隔离——其他连接收不到任何管控帧），**判定窗口 = 帧到达本身零启发式**：收到即被踢（确定性），没收到即网络抖动。前端 transport `events.js` onmessage 分发层拦截（ws-gap/ws-synced 同位，不进业务监听器、不推进序号基准）→ `connBanner.notifyKicked()`：置被踢态横幅「连接已被主机断开，正在自动重连…」（warning 常驻）；紧随的 `notifyDisconnected()` 检测被踢态不改文案（防覆盖）；重连成功 `notifyConnected()` 走既有「连接已恢复」并**消费被踢态一次**（首次重连周期内有效，其后断线恢复普通文案）；踢出后服务紧接停机 → `notifyUnreachable` 覆盖「服务不可达」（可接受不加态）。随后的既有静默自动重连链路不变（踢出非封禁）。多标签页各持独立 WS 连接，帧只达被踢 tab。

```go
const wsResumeBuffer = 2000                                  // 环形缓冲容量（帧数，满后覆写最旧）
func NewWSHub(token string) *WSHub
func newWSHubTuned(token string, writeWait, pongWait time.Duration, resumeBuffer int) *WSHub
// WSHub 内部：seqMu 保护「占号-入环-广播/取补发集」临界区，锁序恒 seqMu → mu
```

> **WS 上行边界刻意保持**：WS 仅 resume 握手一帧业务上行，交互类上行（终端输入、AI 对话工具授权审批应答 `RespondChatApproval` 等）一律走 `/api/rpc` 通用方法通道——不因新交互需求扩 WS 上行协议，readLimit 4096 与「唯一业务上行」契约不被打破。chat 域新增事件（`chat-task:*`、`chat-approval:request/cancelled`）经 applySink 的 chat 行自动进 hub 广播，零特判。

### 2.5 webServe 绑定方法（app_webserve.go，2026-09 新增 4 个）

`GetWebServeConfig` / `SetWebServeConfig(enabled bool, bindAddress string)` / `GetWebServeToken` / `RegenerateWebToken`。

### 2.6 webServeManager 生命周期（web_serve.go）

```go
func (m *webServeManager) Start(listen string) error  // 幂等；改址先 stopLocked 再 startLocked（重建 hub/handler/done）
func (m *webServeManager) Stop() error                // 立即断开语义（见契约表「停机语义」）
func (m *webServeManager) Wait() error                // 代际阻塞：done channel 引用即代际标识
done chan error                                       // 每次 Start 重建、Stop 置 nil；cond 联动 mu 供 Wait 等下一次 Start
```

## 3. Contracts

| 契约 | 内容 |
|---|---|
| 错误形态三端一致 | main.go `formatAppError` `{code,message}` ↔ rpc.go `error:{code,message}` ↔ transport/rpc.js reject `{code,message}`；`E_RPC_*` 六码取值唯一源 `model/app_error.go`，前端 `utils/error.js` ErrorCode 同步（新增码三处同步，同 logging-and-errors.md） |
| token 四通道 | HTTP 凭据优先级：`Authorization: Bearer` > `X-Auth-Token` > `wb_token` cookie > `?token=`（仅首访手工验证，勿写进文档推荐）；WS：子协议 > `wb_token` cookie > `?token=`。HTTP 与 WS 均**任一通道命中即通过**（候选有序但不短路——短路提取会让轮换后浏览器自动携带的旧 cookie 遮蔽 `?token=` 里的新令牌，同浏览器重验证恢复路径永远 401；单令牌基准下任一通道命中即须持有当前令牌，通道间无降级面，回归 `TestWebHandler_RotationRecoveryViaQueryToken`）。cookie 会话贯通（`issueSessionCookie`）：header/query 认证成功且 cookie 缺失或不一致时 `Set-Cookie: wb_token=<t>; Path=/; Max-Age=2592000; HttpOnly; SameSite=Strict`（`r.TLS != nil` 加 Secure），cookie 命中不重复 Set-Cookie——首访 `/?token=<t>` 后静态资产级联请求经 cookie 透明通过（查询参数只随单请求发送，无会话时全部 401，应用永远起不来——2026-09-22 用户实测缺陷）；令牌轮换后旧 cookie 失效须重新验证。恒定时间比较（`subtle.ConstantTimeCompare`），fail-closed。多 token 记录落盘 `data/web_tokens.json`（SHA-256 哈希，明文不落盘，util.SaveJSON 原子写）；存量单 token `data/web_token` 首载迁移为首条记录（保留不删，回滚安全；初始引导/轮换把当前「默认」令牌明文写该文件作「查看令牌」兼容面，0600，已 gitignore） |
| token 校验（多 token，2026-10 三期 PR1） | `WebOptions.Verify` / `WSHub` 持 `TokenVerifier func(candidate) (tokenID, ok)`，生产实现直调 `service.WebTokenService.Verify`（闭包持服务引用，无状态快照）——新建/撤销/过期在服务层即时生效，**无热轮换写路径**（单 token 时代 `SetToken`/`RotateToken` 已删）；命中凭据候选（matched candidate）即会话 cookie 种植值与 WS 子协议回显值（WS 经 `Upgrade` 的 responseHeader 传 `Sec-Websocket-Protocol`，`Subprotocols` 恒 nil）；verify 为 nil（未装配）时一切拒绝（fail closed） |
| token 轮换编排 | `RegenerateWebToken` = `WebTokenService.Create("默认")` 新建 → `RevokeAllExcept` 撤旧 → 新明文同步写存量兼容文件 `data/web_token`（0600，保「查看令牌」链路），全程持 `App.webTokenMu` 互斥：并发轮换（浏览器多标签页同时触发）不加锁会交错出中间存活集合；互斥后编排串行，最后一个持锁完成的调用决定最终态（唯一存活记录=其新令牌，并发回归：`TestApp_RegenerateWebToken_ConcurrentConsistency`） |
| Wait 代际语义 | `--serve` 主流程阻塞于 `Wait`；done channel 引用即代际标识，收到退出错误后核对代际——代际已更替（浏览器经 `SetWebServeConfig` 触发的重启/停机）则继续等当前代际（停机态经 `cond.Wait` 等下一次 Start），进程存活；仅代际未变的真实 Serve 故障退出才返回错误终止进程。旧实现读到旧 done 的 `ErrServerClosed` 即返回 → `runServe` 返回 → 进程意外退出（🔴 已修复，`TestWebServeManager_WaitGenerationSurvivesRestart`） |
| 停机语义 = 立即断开 | `stopLocked` 用 `srv.Close`（立即断全部 HTTP 连接）而非 `Shutdown`（优雅等待）：浏览器经 /api/rpc 在 HTTP 处理器内触发停机/重启时，当前请求所在连接正是 Shutdown 要等的在途请求——自己等自己空转到 3s 超时且残留僵尸连接。代价：触发停机的 RPC 响应可能随连接断开不可达（操作已生效，前端 transport 有失败提示与重连）；`http.Server.Close` 不跟踪 hijacked 连接，在途 WS 客户端须显式 `hub.Close()`（在 `applySink(nil)` 前调用），客户端读侧感知断开、pump goroutine 经 readPump unregister 收尾 |
| 预览路由同源收敛（serve 模式） | `/preview-pdf`、`/preview-raw/` 经 `requireSameOrigin` 中间件：`Sec-Fetch-Site: same-origin` 放行，`cross-site`/`none`/缺失一律 403。理由：预览端点可读任意本地绝对路径，合法调用方仅同源页面内嵌框架（现代浏览器必带 same-origin，含 WebView2），缺失头即非浏览器客户端探测。桌面模式零变化——桌面预览经 AssetServer.Handler 直挂 `PreviewHandler`，不经此中间件 |
| emitCurrent 持锁投递 | service 发射事件一律 `s.emitCurrent(...)`（取值与投递在同一 RLock 域，与 SetEventSink 写锁互斥），禁止先 `eventSink()` 释放锁再 Emit（切换窗口内单条事件迟到旧出口）。Emit 锁内执行要求下游实现不得回调本持有者的 SetEventSink/eventSink（RWMutex 写锁等待下递归取读锁死锁）——现有三实现均只触自身内部锁 |
| webServe 设置段 | `AppSettings.WebServe *WebServeSettings` 指针字段：段缺失→`EnsureWebServeDefaults` 补 `enabled:true`（默认同开）、`bindAddress:"127.0.0.1:36115"`；`--listen` 命令行 > settings.json > 默认值；TLS 三字段（`tls`/`tlsCertPath`/`tlsKeyPath`）显式决策项不补默认（零值=纯 HTTP 向后兼容） |
| 内置 TLS（2026-10 三期 PR2） | `Start(listen string, tlsCfg *WebServeTLSConfig)`（nil=纯 HTTP）：接入形态 `tls.NewListener(ln, tlsConfig)` + `srv.Serve`（**禁 ServeTLS**——其默认开 h2 ALPN，gorilla 仅 HTTP/1.1 升级；自建 tls.Config 须钉 `NextProtos:["http/1.1"]` + `MinVersion TLS1.2`）。`WebServeTLSConfig{Enabled, CertPath, KeyPath}` 为可比较 struct 作幂等键组成部分——**幂等键 = listen + TLS 配置联合比较**（同地址开/关 TLS 必须真实重启，仅比较 listen 会「保存了但不生效」）。**证书加载/生成在 `Start` 内幂等检查后、`stopLocked` 与任何字段赋值之前**（失败旧实例原样存活，无半死态；加载放 startLocked 内无法满足此约束——改配置路径下停旧先于 startLocked）。`m.ln` 恒存原始 TCP listener（`listenAddr()` 语义不变）。双供给形态：`tlsCertPath`+`tlsKeyPath` 非空走用户 PEM（加载失败 fail-fast），双空走自签引导 |
| 自签证书引导（`server/selfsigned.go`） | `EnsureSelfSignedCert(dataDir)` 产出 `data/tls_cert.pem`（0644）+ `data/tls_key.pem`（0600）；ECDSA P-256 自签，有效期 10 年（决策：引导用途越长过期噪声越少，非 research 草案 825 天）、NotBefore 提前 24h 防时钟偏移，SAN 必填覆盖回环 + `util.LocalIPv4Addresses()` 本机枚举 + localhost（Chrome 58 起 CN 不参与匹配），KeyUsage/ExtKeyUsageServerAuth 齐备。**复用判定 = `tls.LoadX509KeyPair` 成功（解析+配对一次校验）且 NotAfter 未过期；损坏/过期/单边残留一律 fail-fast 返 `E_TLS_CERT_INVALID`，禁静默重生成**（重生成会使浏览器已信任的证书例外失效且难排查）。不发 HSTS（发了 Chrome 禁「继续前往」自签例外路径）。前端 transport 按 `location.protocol` 自适配 wss 零改动。Go 1.26 `http.Server` 无 TLSHandshakeTimeout 字段，握手超时取 `min(ReadHeaderTimeout, ReadTimeout, WriteTimeout)`——既有 `ReadHeaderTimeout:10s` 天然覆盖 slowloris |
| polyfill 注入时序 | wailsjs 生成 wrapper 在**函数调用体内**才解引用 `window.go`；`main.js` 在 `createApp` 前 `installBrowserTransport()`（有 Wails runtime 则 no-op）即全链路生效。新增模块顶层代码禁止触碰 `window.go`/`window.runtime` |
| 桌面/serve 装配同构 | 桌面同开与 `--serve` 共用 `assembleWebServe`（main.go），禁复制装配逻辑产生第二份 |
| WS 续传序号单调 | `WSHub.Emit` 在 `seqMu` 临界区内完成「占号-入环-广播」：序号分配与环形缓冲写入同锁防乱序，广播同锁保证任一帧的投递先于后续 resume 快照的补发集计算；marshal 失败回退占号（未对外发布），保持「仅成功广播占号」的连续分配。锁序恒 `seqMu → mu`，`mu` 写侧路径（register/unregister）不获取 `seqMu`，无反向嵌套死锁面 |
| WS 重连握手 | 浏览器 transport 记录已收最大 `seq`（内存态，多标签页各记各的，页面刷新归零），重连 `onopen` 且有基准时发送 `{"resume": lastSeq}`；首连不发送——服务端从当前序号起广播，**不重放历史** |
| WS 补发与断层 | resume 处理在 `seqMu` 临界区取补发集：`lastSeq > 当前序号`（hub 重建序号归零，跨代际不可比较）或 `lastSeq+1 < 环形缓冲最旧序号`（断线期间事件量超出 2000 帧缓冲，缺口被覆写）→ 只发 `ws-gap` 标记帧，前端重置基准、经 connBanner 提示「部分更新已跳过」，由各面板既有冷启动快照/状态查询链路或手动刷新补齐；否则补发缓冲内 `seq > lastSeq` 全部帧（复用入环时的序列化字节）+ `ws-synced` 完成标记（空补发的追平形态同样必发） |
| WS 补发写入路径 | 补发帧经既有 `send` 队列（与广播同路）：`send` channel 的 close 仅发生在 unregister（写锁下）且 unregister 只由 readPump 自身 defer 调用，补发投递与 close 无并发窗口；writePump 单写者消费保持单连接写串行化。投递非阻塞 select——队列满按慢消费者同语义关连接终止补发（阻塞式灌入会拖住 readPump 使其无法读 pong 且注销 defer 停滞）；**禁**在 readPump 直接 `conn.WriteMessage`（gorilla 禁止并发写同一连接） |
| WS 事件有序性 | 补发帧与重连注册窗口内的实时广播共用 FIFO 发送队列、交错到达且补发集是窗口广播的超集：前端以 `ws-synced` 为界进入同步模式，序号帧先进缓冲不落地，标记到达后按 `seq` 升序去重（同 seq 必为同一事件——一次 Emit 占一个序号、载荷不可变）一次性落地；`ws-gap` 放弃缓冲。无 `seq` 帧直接分发不推进基准（老服务端兼容形态）；实时模式 `seq <= lastSeq` 的重复帧丢弃 |
| 多设备/多标签页语义 | 维持广播共享语义：多标签页/多设备共享同一后端状态（终端会话共享、输入 last-write-wins）；`lastSeq` 为页面内存态各记各的，hub 重建（webServeManager 重启）使序号归零属预期——重连握手判定为断层走 `ws-gap` 提示，非异常 |
| WS 被踢单播帧（2026-10 二期） | `ws-kicked` 仅经 `DisconnectClient` 的 send 队列送达被踢连接，**绝不走 `Emit` 广播路径**（无 seq、不进环形缓冲——resume 补发面不含，防重连幻影回放）；时序 = 踢出帧先于连接关闭送达（writePump 单写者 FIFO：帧写出后见 nil 哨兵才发 Close 帧退出）；投递 best-effort——队列满/写失败静默退化为普通断开，踢出断开本身恒生效（fallback 直接 `conn.Close()`）；`wsClient.kicked` 标记（mu 保护）防窗口内重复踢出二次入队，幂等语义保留（窄窗口内重复命中返 true 不再入队，注销后 miss 返 false 不变） |
| 踢出审计行（conn_kick，2026-10 二期） | `App.KickConnection` 在 hub 命中（踢出生效）后经 `AuditLogger.LogConnKick` 写 `conn_kick` 行：tokenId/name（名册映射 miss 空串）/remoteAddr（来源 IP，明细快照剥端口口径）/connId/ts；**miss（`E_CONNECTION_NOT_FOUND`）与停机（`E_WEBSERVE_NOT_RUNNING`）不写**；无明文 token 无完整 URL；`AuditRecord.ConnID` 字段 `omitempty` 仅本事件携带（既有事件行形态不变）；桌面按钮与浏览器 RPC 反射面同走 App.KickConnection 单咽喉全覆盖，server 包零新增回调注入面；审计事件全表见 logging-and-errors.md §8 |

## 4. Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| 无/错 token 访问 RPC、静态页（除 /healthz） | 401 JSON |
| 带有效 `wb_token` cookie 访问 RPC/静态资产/WS | 200/升级成功（会话贯通）；cookie 命中不重复 Set-Cookie |
| 错 cookie / 令牌轮换后旧 cookie | 401 JSON（header/query 重验证后重种新会话） |
| WS 握手无/错 token（三通道均未命中） | HTTP 401 拒绝升级 |
| serve 模式预览请求 `Sec-Fetch-Site` 非 same-origin（含缺失） | 403 JSON（`requireSameOrigin`；桌面模式不经此路径） |
| serve 模式调窗口对话框方法（Save/OpenFileDialog 等） | `wailsRuntimeUnavailable` 守卫返回可读错误（不守卫则 wails `log.Fatalf` 杀整个服务进程——真实事故，PR2 修复） |
| RPC 未知方法 / 未导出方法 / 变参方法 | `E_RPC_METHOD_NOT_FOUND` / 拒绝 |
| 参数个数/类型不匹配、浮点转整型 | `E_RPC_ARG_COUNT_MISMATCH` / `E_RPC_ARG_TYPE_MISMATCH` |
| body 超限 / 非法 JSON / 非 POST | 32MB `MaxBytesError` 专错 / `E_RPC_BAD_REQUEST` / 405+Allow |
| 桌面启动端口被占 | `slog.Warn` 降级，Running=false，桌面不崩；`SetWebServeConfig` 可重试 |
| TLS 证书加载失败（用户路径无效/自签损坏过期单边） | `E_TLS_CERT_INVALID` error 级 AppError；三链路形态：桌面启动 slog.Warn 降级不崩 / `SetWebServeConfig` 结构化错误透传前端弹 error / `--serve` fatal 退出；失败时旧实例原样存活（半死态防护，`TestWebServeManager_TLSCertInvalidOldInstanceSurvives`） |
| 踢出连接时服务停机/未启用（hub nil） | `E_WEBSERVE_NOT_RUNNING` warning 级 AppError（`TestApp_KickConnection_NotRunning`：零值 nil 管理器与停机后同码不崩） |
| 踢出连接 ID 不存在/已断开 | `E_CONNECTION_NOT_FOUND` warning 级 AppError（列表过期引导刷新；重复踢出幂等同此路径，`TestApp_KickConnection_EndToEnd`）；**miss 不写 conn_kick 审计**（`TestApp_KickConnection_AuditTrail`） |
| 踢出生效（hub 命中） | 被踢连接先收 `ws-kicked` 帧（无 seq、data 空数组）再感知断开（`TestWSHub_DisconnectClientKickFrameBeforeClose`）；其余连接零管控帧（`TestWSHub_DisconnectClientUnicastIsolation`）；重连 resume 补发不含踢出帧（`TestWSHub_KickFrameNotInResumeReplay`）；写一条 conn_kick 审计行（字段齐/无明文 token，`TestApp_KickConnection_AuditTrail`） |
| 踢出时 send 队列满（慢消费者）/ 帧写失败 | 静默降级为直接断开（踢出帧丢失，断开恒生效）；`kicked` 标记挡住窗口内重复踢出二次入队（`TestWSHub_DisconnectClientQueueFullFallback`） |
| webAuditSvc 未装配（旁路构造） | 踢出主链路不崩、不写审计（`TestApp_KickConnection_AuditTrail`） |
| 非回环绑定 | 后端 slog.Warn + 前端 ElMessageBox 风险确认双保险 |
| 运行中服务被浏览器侧停机后再重启 | `Wait` 经 cond 唤醒继续监视新代际，`--serve` 进程不退出 |
| resume 握手 `lastSeq` 可精确补发 | 补发缓冲内 `seq > lastSeq` 全部帧 + `ws-synced` 标记（`TestWSHub_ResumeFillExact`） |
| resume `lastSeq` 追平（等于服务端当前序号） | 无帧可补，仅发 `ws-synced`（`TestWSHub_ResumeCaughtUp`） |
| resume `lastSeq+1` 早于缓冲最旧序号（溢出）/ `lastSeq` 超前（hub 重建） | 只发 `ws-gap` 标记帧（data[0]=服务端当前序号），不补发（`TestWSHub_ResumeGapOnOverflow` / `TestWSHub_ResumeGapOnHubRestart`） |
| 新连接（无 resume） | 从当前序号起广播，不重放历史（`TestWSHub_FreshConnectionNoReplay`） |
| 客户端 register/unregister 成功 | 广播 `ws-connections:changed`（data[0]=事件后连接总数；count=0 与 hub Close 收口期不广播；mu 写锁外发射）（`TestWSHub_ConnectionCountEvents` / `TestWSHub_CloseSuppressesConnectionEvents`） |
| 客户端上行非协议帧（非法 JSON / 缺 resume / 类型不符） | 静默忽略，连接存活（`TestWSHub_ClientFrameGarbageIgnored`） |

## 5. Good/Base/Bad Cases

* **Good**：新 service 需要推事件 → 嵌 `sinkHolder`，构造注入 wails sink，发事件处 `s.emitCurrent("xxx", data)`；`AppServices` 装配行 + `assembleWebServe` 切换点各一处接入。
* **Base**：只跑桌面不开 HTTP → `webServe.enabled=false`；`--serve` 无头 → 跳过 `wails.Run`，`multicastSink` 对 Background ctx 的 Wails 出口静默跳过，等效 hub only。
* **Bad**：service 直调 `runtime.EventsEmit`（见 §7）；先 `eventSink()` 取值释放锁再 Emit（顺序缺口，B5 已消除）；`stopLocked` 改回 `Shutdown` 优雅等待（RPC 处理器内触发停机自等 3s 死锁复现）；`WSHub.Close` 直接 `close(c.send)`（与 readPump unregister 双重 close）；`RegenerateWebToken` 去掉 `webTokenMu`（并发轮换交错出中间存活集合）；verify 闭包内做服务状态快照复制（多 token 即时生效依赖直读，快照化会复活热轮换同步问题）；审计日志落 token 明文或完整 URL（`?token=` 泄露面）；测试用 `127.0.0.1:0` 构造「改址重启」（同串命中 Start 幂等 no-op，触发不了真实重启路径，须用 freePort 具体端口）；resume 补发在 readPump 直写 `conn`（与 writePump 并发写同一连接，gorilla 禁止）或阻塞式灌 send 队列（readPump 停滞 + 注销 defer 无法执行）；服务端在补发路径做与广播的排序协调（补发集/注册窗口广播经同一 FIFO 队列交叠属预期，有序性由前端 `ws-synced` 同步模式保证，服务端侧二次协调是过度设计）；前端实时模式直接丢弃 `seq <= lastSeq` 的补发窗口帧外的乱序帧（同步模式缓冲按 seq 升序落地前，先到的注册窗口广播会推进 lastSeq——去重只能在同步缓冲内做）；踢出帧改走 `Emit` 广播或进环形缓冲（全员收到「被踢」误报 + 重连 resume 幻影回放，二期单播契约崩坏）；`DisconnectClient` 命中后在锁内直调 notify/Emit 或把审计写进 server 包（锁序红线 / server 不知 service 分层红线——审计归 App 层 hub 命中后落盘）；connBanner 被踢态缺「仅消费一次」收口（其后每次普通断线恒显被踢文案，误报常态化）。

## 6. Tests Required

* 事件出口：sink nil-ctx/非 Wails ctx 安全、fake sink 透传断言、SetEventSink 切换 -race（并发 Emit+切换）、emitCurrent 持锁投递顺序（`TestSinkHolder_EmitCurrentUnderLockNoStaleDelivery`：新出口首事件不得早于旧出口在途 Emit 结束）
* RPC：成功/未知方法/参数矩阵/AppError 透传/未认证 401/32MB 上限
* cookie 会话：认证矩阵（有效 cookie 200/错 cookie 唯一凭据 401/错 cookie + 对 query 重验证重种）、首访 Set-Cookie 属性（HttpOnly/SameSite=Strict/Max-Age/Path、TLS 加 Secure）、cookie 命中不重复 Set-Cookie、旧 cookie 轮换后 401、header 认证重种、轮换后同浏览器 `?token=` 重验证恢复（`TestWebHandler_RotationRecoveryViaQueryToken`）、首访资产级联（`TestWebHandler_CookieSessionCascade`）与真实 embed 资产全链路（`serve_cookie_smoke_test.go`：?token= 拿 Set-Cookie → 带 cookie 资产 200 → 无凭据 401 → cookie RPC 200）
* WS：认证矩阵（三 token 通道）、帧格式 `data:[]`、断连清理无 goroutine 泄漏、慢消费者踢出、verify 热更替轮换（`TestWSHub_VerifierSwapRotation`）、握手 OnAuth 认证事件（`TestWSHub_OnAuthEvents`）、cookie 通道裸连升级（`TestWSHub_CookieAuthChannel`）、`Close` 断开全部在途客户端 + 幂等（`TestWSHub_CloseDisconnectsAllClients`）+ manager Stop 集成（`TestWebServeManager_StopClosesWSClients`）
* WS 续传：序号单调与多客户端一致（`TestWSHub_EmitSeqMonotonicShared`）、resume 精确补发/追平/溢出断层/hub 重建断层/新连接不重放（上表四用例）、非协议上行帧忽略（`TestWSHub_ClientFrameGarbageIgnored`）、广播与 resume 补发并发 `-race` + 收帧序号不回退（`TestWSHub_ResumeConcurrentWithEmitRace`）
* WS 连接数事件：连接/断开计数事件断言、Close 收口期抑制（`TestWSHub_ConnectionCountEvents` / `TestWSHub_CloseSuppressesConnectionEvents`）；桌面回调出口：明细快照全量透传（字段断言 ID 单调/IP 剥端口/TokenID/接入时间非零/server 侧 TokenName 恒空、快照按 ID 升序、空快照非 nil 透传、浏览器载荷恒 count 数字隐私断言）、closing 抑制、nil 安全（`TestWSHub_OnConnectionsChangeCallback` / `TestWSHub_CloseSuppressesOnConnectionsChangeCallback` / `TestWSHub_OnConnectionsChangeNilSafe`，manager 级 Stop 收口空数组 emit 静默形态由 `TestWebServeManager_StopClosesWSClients` 覆盖）；前端消费 `useDeviceConnections` composable（事件驱动显隐/非法载荷忽略/卸载闭包注销/桌面通道锁定与空数组透传/deviceCount=len(明细) 分流派生）+ ActivityBar 指示与明细 popover 面板组件用例 + TerminalPanel 提示组件用例 + E2E `settings-token.spec.js` 事件驱动链
* 前端续传（`frontend/src/transport/__tests__/events.spec.js`）：lastSeq 记录与 resume 握手发送（首连/无基准不发送）、同步模式缓冲按 seq 升序去重落地、ws-gap 断层处理（提示+基准重置）、同步缓冲超限兜底、无 seq 帧向后兼容、实时模式重复帧丢弃
* webServe：manager 幂等/改址重启/端口占用三态、RegenerateWebToken 真实 HTTP 链路旧 401 新 200、并发轮换最终态一致（唯一存活令牌==磁盘兼容文件==校验基准）、Wait 代际（重启/停机/重开不返回 + 绕过管理器关 listener 真实退出返回错误）
* 预览同源：`Sec-Fetch-Site` 矩阵（same-origin 过关入业务校验 / cross-site、none、缺失 403）
* serve 端到端：`serve_rpc_smoke_test.go`（真实 *App + httptest：401/Bearer/query/WS 广播/对话框守卫不杀进程）
* 前端事件桥：无本地令牌裸连（`new WebSocket(url)` 不传 protocols、不弹门）、裸连失败转 token 门后转子协议、裸连成功后断线静默重连、有令牌走子协议零回归（events.spec.js）
* 连接管控二期：踢出帧时序（帧先于 close 送达、无 seq、data 空数组，`TestWSHub_DisconnectClientKickFrameBeforeClose`）、单播隔离（`TestWSHub_DisconnectClientUnicastIsolation`）、resume 补发不含踢出帧（`TestWSHub_KickFrameNotInResumeReplay`）、队列满降级 + kicked 幂等（手工在册客户端确定性构造，`TestWSHub_DisconnectClientQueueFullFallback`）、踢出审计行（字段齐/miss 不写/无明文 token/webAuditSvc nil 不崩，`TestApp_KickConnection_AuditTrail` + service `TestAuditLogger_LogConnKick`）、前端 `ws-kicked` 分发层拦截与 connBanner 被踢态置位/消费一次/重连清除/不可达覆盖（events.spec.js + connBanner.spec.js）
* 连接管控三期：RPC 踢出操作者归因端到端（真实 WS 接入 + 操作者令牌 Bearer 调 `/api/rpc` KickConnection，审计行 actorSource=rpc + actorTokenId=调用者记录 ID + 目标字段仍为受害侧 + 无明文泄露，`TestApp_KickConnection_RPCActorAudit`）、桌面行 desktop/空（`TestApp_KickConnection_AuditTrail` 扩展断言）、审计读侧对浏览器通道屏蔽（HiddenMethods 命中与真不存在同响应 + 非屏蔽方法不受影响，`TestServeRPC_QueryAuditLogHiddenForBrowser`）、读侧绑定委托与空值收敛（`TestApp_QueryAuditLog`）、审计历史弹窗与设置页入口（vitest `AuditHistoryDialog.spec.js` + `SettingsPanel.spec.js` 审计入口两用例 + E2E `audit-history.spec.js`）

## 7. Wrong vs Correct

### Wrong

```go
// service 层直调 wails runtime：非 Wails ctx 时 log.Fatalf 杀进程；
// serve 浏览器通道永远收不到该事件
runtime.EventsEmit(s.ctx, "task-progress", progress)

// 先取值释放锁再 Emit：切换窗口内事件迟到旧出口（B5 已消除）
emitEvent(s.eventSink(), "task-progress", progress)
```

### Correct

```go
// 经 EventSink 出口 + 持读锁投递：桌面走 wailsEventSink，
// 同开/serve 自动切换到 WS hub 广播，且切换与投递互斥
s.emitCurrent("task-progress", progress)
```

> **Warning**：`runtime.EventsEmit` 的 ctx 缺 `frontend` 键（含 `context.Background()`）时 wails v2.16 直接 `log.Fatalf` 退出**整个进程**——serve 模式下任何一个持 token 请求即可杀死服务。一切事件必须走 EventSink；一切依赖 Wails 窗口运行时的调用（对话框等）必须过 `wailsRuntimeUnavailable` 守卫。

---

**更新：** 2026-10-24（任务 `.trellis/tasks/10-24-actor`：连接管控三期审计可见性——actor 归因：`requireToken` 注入 tokenID 进 ctx + `WebOptions.RPCTargetForActor` per-request 装饰 target（main 包 `rpcAuditTarget` embed override KickConnection → 未导出 `kickConnectionAs`）+ `AuditRecord` 加 `actorSource`/`actorTokenId`（桌面 desktop/空、RPC rpc/调用者记录 ID）+ `LogConnKick` 收编为 `KickAudit` 结构化参数；审计历史读侧：`AuditLogger.QueryAuditLog`（Events/时间窗/TokenID 过滤 + 倒序分页 + `.1`+主文件合并 + 坏行跳过 + 快照语义）+ App 绑定 `QueryAuditLog` + 前端设置页「审计日志」入口（浏览器通道隐藏）+ `AuditHistoryDialog`（960 档弹窗/过滤/分页/名册映射）；隐私红线：`WebOptions.RPCHiddenMethods` 屏蔽（对浏览器呈现「方法不存在」）——「同构暴露不特判」原则的首个有据偏离（数据隐私面优先于动作面洁癖）；见 §2.2 身份注入与 HiddenMethods、§2.3 连接管控段、§6 测试节、logging-and-errors.md §8）
**更新：** 2026-10-24（任务 `.trellis/tasks/10-24-connection-kick-audit-and-kicked-end-notice`：连接管控二期——`DisconnectClient` 返回值扩 `(ConnDetail, bool)` 且命中改「ws-kicked 单播帧 + nil 哨兵入队、writePump flush 后发 Close 帧自关」（帧先于关闭送达；队列满回退直关 best-effort；`kicked` 标记幂等；不进环形缓冲防重连幻影）+ `KickConnection` 透传 detail（App 对外签名零变更）+ App 命中后写 `conn_kick` 审计行（`AuditLogger.LogConnKick` + `AuditRecord.ConnID`，miss/停机不写）+ 前端 events.js 拦截 `ws-kicked` → connBanner 被踢态（消费一次/重连清除/不可达覆盖）；见 §2.3 连接管控段、§2.4 被踢端感知、§3 契约表两行、logging-and-errors.md §8）
**更新：** 2026-10-23（任务 `.trellis/tasks/10-23-connection-kick-and-token-online-status`：连接管控——`WSHub.DisconnectClient(connID)` 按 ID 踢出（对齐 Close 只关 conn 模式、零新增事件、幂等）+ `webServeManager.KickConnection`/App 绑定（`E_WEBSERVE_NOT_RUNNING`/`E_CONNECTION_NOT_FOUND` 两新码三处同步）+ App 层连接明细缓存（回调写入/停机显式清零）+ `ListWebTokens` 返回 `[]model.WebTokenStatus`（record+onlineConns join，落盘零污染）+ 前端 ActivityBar 行内踢出确认链与 SettingsPanel 在线徽标；见 §2.3 连接管控段）
**更新：** 2026-10-23（任务 `.trellis/tasks/10-23-desktop-device-connection-details`：连接数升级为设备明细列表——`server.ConnDetail`（ID/IP/TokenID/TokenName/ConnectedAt，ServeHTTP 升级成功时一次赋值此后不可变）+ `SetOnCountChange` 更名 `SetOnConnectionsChange`（`func([]ConnDetail)` 全量快照）+ `notifyConnectionCount`→`notifyConnectionDetails`（快照 `snapshotDetailsLocked` 写锁内构造 ID 升序、出锁后触发）+ stopLocked 补发空数组 + 装配层 `webTokenNamedDetails` 查名册补 TokenName（miss 空串前端 fallback「已撤销令牌」）+ 前端 `connections` ref/ActivityBar el-popover 明细面板（**隐私红线**：浏览器通道载荷恒 count 数字）；见 §2.3 桌面出口段）
**更新：** 2026-10-23（任务 `.trellis/tasks/10-23-desktop-device-connection-indicator`：桌面连接数回调出口——`WSHub.SetOnCountChange` 纯 `func(int)` 回调 + manager 装配经 Wails 出口发桌面事件 `webserve-connections:changed`（count=0 透传/重连/熄指示语义与浏览器通道刻意分叉，见 §2.3 桌面出口段）、`stopLocked` 显式归零、前端 `useDeviceConnections` 双通道消费；任务 `.trellis/tasks/10-22-tls-token` PR1：多 token 管理——`TokenVerifier` 校验函数注入（SetToken/RotateToken 删除）、SHA-256 哈希落盘 `data/web_tokens.json` + 存量迁移、轮换编排撤旧建新、认证审计 JSONL `data/web_audit.log`、`OnAuth` 认证事件回调；PR2：内置 TLS——`tls.NewListener` 接入 + ALPN 钉 http/1.1、幂等键扩 listen+TLS、证书加载前置无半死态、自签引导 `server/selfsigned.go`（ECDSA P-256/SAN 必填/fail-fast 禁静默重生成）+ 用户证书双形态、`E_TLS_CERT_INVALID`；PR3：WSHub 连接数事件 `ws-connections:changed` + 前端 `useDeviceConnections` 多设备语义明示）

**创建：** 2026-09-22（任务 `.trellis/tasks/09-21-agent`）
**更新：** 2026-09-22（任务 `.trellis/tasks/09-22-browser-audit-fixes`：补 Wait 代际、停机立即断开语义、token 轮换原子性、WSHub.Close、预览同源收敛、emitCurrent 持锁投递契约）
**更新：** 2026-09-22（任务 `.trellis/tasks/09-22-first-visit-asset-401`：token 三通道扩为四通道——新增 wb_token cookie 会话贯通（首访 `?token=` 自动种 30 天 HttpOnly/SameSite=Strict 会话，静态资产级联/RPC/WS 裸连透明通过），前端无本地令牌时 WS 裸连）
