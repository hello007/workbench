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

### 2.3 WebSocket 事件 hub（server/ws.go）

```text
GET /ws  —— Sec-WebSocket-Protocol: <t>  /  wb_token cookie  /  ?token=<t>
                                                               （升级前 401 拒绝，勿升级后踢）
帧: {"event":"<事件名>","data":[...]}    零载荷 data 为 [] 非 null
func (h *WSHub) Close()   // 停机关闭：写锁遍历 clients 逐个 conn.Close，send 由 readPump unregister 收尾
```

浏览器 shim 以 `callback(...frame.data)` 还原 Wails `EventsOn` 回调形态。

### 2.4 断线重连续传（server/ws.go，2026-10 新增）

```text
事件帧: {"event":"<名>","data":[...],"seq":<N>}    seq 为 hub 级单调序号（自 1 起，omitempty）
控制帧: {"event":"ws-gap","data":[<服务端当前序号>]}      断层标记（无 seq）
        {"event":"ws-synced","data":[<补发快照当前序号>]} 补发完成标记（无 seq）
握手帧: {"resume":<客户端已收最大序号>}                    客户端 → 服务端，唯一业务上行
```

```go
const wsResumeBuffer = 2000                                  // 环形缓冲容量（帧数，满后覆写最旧）
func NewWSHub(token string) *WSHub
func newWSHubTuned(token string, writeWait, pongWait time.Duration, resumeBuffer int) *WSHub
// WSHub 内部：seqMu 保护「占号-入环-广播/取补发集」临界区，锁序恒 seqMu → mu
```

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
| token 四通道 | HTTP 凭据优先级：`Authorization: Bearer` > `X-Auth-Token` > `wb_token` cookie > `?token=`（仅首访手工验证，勿写进文档推荐）；WS：子协议 > `wb_token` cookie > `?token=`。HTTP 与 WS 均**任一通道命中即通过**（候选有序但不短路——短路提取会让轮换后浏览器自动携带的旧 cookie 遮蔽 `?token=` 里的新令牌，同浏览器重验证恢复路径永远 401；单令牌基准下任一通道命中即须持有当前令牌，通道间无降级面，回归 `TestWebHandler_RotationRecoveryViaQueryToken`）。cookie 会话贯通（`issueSessionCookie`）：header/query 认证成功且 cookie 缺失或不一致时 `Set-Cookie: wb_token=<t>; Path=/; Max-Age=2592000; HttpOnly; SameSite=Strict`（`r.TLS != nil` 加 Secure），cookie 命中不重复 Set-Cookie——首访 `/?token=<t>` 后静态资产级联请求经 cookie 透明通过（查询参数只随单请求发送，无会话时全部 401，应用永远起不来——2026-09-22 用户实测缺陷）；令牌轮换后旧 cookie 失效须重新验证。恒定时间比较（`subtle.ConstantTimeCompare`），fail-closed。落盘 `data/web_token` 0600，已 gitignore |
| token 热轮换 | `RegenerateWebToken`/`WebHandler.SetToken`/`WSHub.SetToken`：HTTP 与新 WS 握手立即生效；在途 WS 连接数据面不再校验 token，保持至断开重连（UI 文案已交代） |
| token 轮换原子性 | `RegenerateWebToken` 生成→`SaveToken` 落盘→`RotateToken` 全程持 `App.webTokenMu` 互斥：并发轮换（浏览器多标签页同时触发）不加锁会交错出磁盘与 handler/hub 内存基准不一致；互斥后「落盘+热轮换」原子完成，最后一个持锁完成的调用决定三处一致的最终态（并发回归：`TestApp_RegenerateWebToken_ConcurrentConsistency`） |
| Wait 代际语义 | `--serve` 主流程阻塞于 `Wait`；done channel 引用即代际标识，收到退出错误后核对代际——代际已更替（浏览器经 `SetWebServeConfig` 触发的重启/停机）则继续等当前代际（停机态经 `cond.Wait` 等下一次 Start），进程存活；仅代际未变的真实 Serve 故障退出才返回错误终止进程。旧实现读到旧 done 的 `ErrServerClosed` 即返回 → `runServe` 返回 → 进程意外退出（🔴 已修复，`TestWebServeManager_WaitGenerationSurvivesRestart`） |
| 停机语义 = 立即断开 | `stopLocked` 用 `srv.Close`（立即断全部 HTTP 连接）而非 `Shutdown`（优雅等待）：浏览器经 /api/rpc 在 HTTP 处理器内触发停机/重启时，当前请求所在连接正是 Shutdown 要等的在途请求——自己等自己空转到 3s 超时且残留僵尸连接。代价：触发停机的 RPC 响应可能随连接断开不可达（操作已生效，前端 transport 有失败提示与重连）；`http.Server.Close` 不跟踪 hijacked 连接，在途 WS 客户端须显式 `hub.Close()`（在 `applySink(nil)` 前调用），客户端读侧感知断开、pump goroutine 经 readPump unregister 收尾 |
| 预览路由同源收敛（serve 模式） | `/preview-pdf`、`/preview-raw/` 经 `requireSameOrigin` 中间件：`Sec-Fetch-Site: same-origin` 放行，`cross-site`/`none`/缺失一律 403。理由：预览端点可读任意本地绝对路径，合法调用方仅同源页面内嵌框架（现代浏览器必带 same-origin，含 WebView2），缺失头即非浏览器客户端探测。桌面模式零变化——桌面预览经 AssetServer.Handler 直挂 `PreviewHandler`，不经此中间件 |
| emitCurrent 持锁投递 | service 发射事件一律 `s.emitCurrent(...)`（取值与投递在同一 RLock 域，与 SetEventSink 写锁互斥），禁止先 `eventSink()` 释放锁再 Emit（切换窗口内单条事件迟到旧出口）。Emit 锁内执行要求下游实现不得回调本持有者的 SetEventSink/eventSink（RWMutex 写锁等待下递归取读锁死锁）——现有三实现均只触自身内部锁 |
| webServe 设置段 | `AppSettings.WebServe *WebServeSettings` 指针字段：段缺失→`EnsureWebServeDefaults` 补 `enabled:true`（默认同开）、`bindAddress:"127.0.0.1:36115"`；`--listen` 命令行 > settings.json > 默认值 |
| polyfill 注入时序 | wailsjs 生成 wrapper 在**函数调用体内**才解引用 `window.go`；`main.js` 在 `createApp` 前 `installBrowserTransport()`（有 Wails runtime 则 no-op）即全链路生效。新增模块顶层代码禁止触碰 `window.go`/`window.runtime` |
| 桌面/serve 装配同构 | 桌面同开与 `--serve` 共用 `assembleWebServe`（main.go），禁复制装配逻辑产生第二份 |
| WS 续传序号单调 | `WSHub.Emit` 在 `seqMu` 临界区内完成「占号-入环-广播」：序号分配与环形缓冲写入同锁防乱序，广播同锁保证任一帧的投递先于后续 resume 快照的补发集计算；marshal 失败回退占号（未对外发布），保持「仅成功广播占号」的连续分配。锁序恒 `seqMu → mu`，`mu` 写侧路径（register/unregister/SetToken）不获取 `seqMu`，无反向嵌套死锁面 |
| WS 重连握手 | 浏览器 transport 记录已收最大 `seq`（内存态，多标签页各记各的，页面刷新归零），重连 `onopen` 且有基准时发送 `{"resume": lastSeq}`；首连不发送——服务端从当前序号起广播，**不重放历史** |
| WS 补发与断层 | resume 处理在 `seqMu` 临界区取补发集：`lastSeq > 当前序号`（hub 重建序号归零，跨代际不可比较）或 `lastSeq+1 < 环形缓冲最旧序号`（断线期间事件量超出 2000 帧缓冲，缺口被覆写）→ 只发 `ws-gap` 标记帧，前端重置基准、经 connBanner 提示「部分更新已跳过」，由各面板既有冷启动快照/状态查询链路或手动刷新补齐；否则补发缓冲内 `seq > lastSeq` 全部帧（复用入环时的序列化字节）+ `ws-synced` 完成标记（空补发的追平形态同样必发） |
| WS 补发写入路径 | 补发帧经既有 `send` 队列（与广播同路）：`send` channel 的 close 仅发生在 unregister（写锁下）且 unregister 只由 readPump 自身 defer 调用，补发投递与 close 无并发窗口；writePump 单写者消费保持单连接写串行化。投递非阻塞 select——队列满按慢消费者同语义关连接终止补发（阻塞式灌入会拖住 readPump 使其无法读 pong 且注销 defer 停滞）；**禁**在 readPump 直接 `conn.WriteMessage`（gorilla 禁止并发写同一连接） |
| WS 事件有序性 | 补发帧与重连注册窗口内的实时广播共用 FIFO 发送队列、交错到达且补发集是窗口广播的超集：前端以 `ws-synced` 为界进入同步模式，序号帧先进缓冲不落地，标记到达后按 `seq` 升序去重（同 seq 必为同一事件——一次 Emit 占一个序号、载荷不可变）一次性落地；`ws-gap` 放弃缓冲。无 `seq` 帧直接分发不推进基准（老服务端兼容形态）；实时模式 `seq <= lastSeq` 的重复帧丢弃 |
| 多设备/多标签页语义 | 维持广播共享语义：多标签页/多设备共享同一后端状态（终端会话共享、输入 last-write-wins）；`lastSeq` 为页面内存态各记各的，hub 重建（webServeManager 重启）使序号归零属预期——重连握手判定为断层走 `ws-gap` 提示，非异常 |

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
| 非回环绑定 | 后端 slog.Warn + 前端 ElMessageBox 风险确认双保险 |
| 运行中服务被浏览器侧停机后再重启 | `Wait` 经 cond 唤醒继续监视新代际，`--serve` 进程不退出 |
| resume 握手 `lastSeq` 可精确补发 | 补发缓冲内 `seq > lastSeq` 全部帧 + `ws-synced` 标记（`TestWSHub_ResumeFillExact`） |
| resume `lastSeq` 追平（等于服务端当前序号） | 无帧可补，仅发 `ws-synced`（`TestWSHub_ResumeCaughtUp`） |
| resume `lastSeq+1` 早于缓冲最旧序号（溢出）/ `lastSeq` 超前（hub 重建） | 只发 `ws-gap` 标记帧（data[0]=服务端当前序号），不补发（`TestWSHub_ResumeGapOnOverflow` / `TestWSHub_ResumeGapOnHubRestart`） |
| 新连接（无 resume） | 从当前序号起广播，不重放历史（`TestWSHub_FreshConnectionNoReplay`） |
| 客户端上行非协议帧（非法 JSON / 缺 resume / 类型不符） | 静默忽略，连接存活（`TestWSHub_ClientFrameGarbageIgnored`） |

## 5. Good/Base/Bad Cases

* **Good**：新 service 需要推事件 → 嵌 `sinkHolder`，构造注入 wails sink，发事件处 `s.emitCurrent("xxx", data)`；`AppServices` 装配行 + `assembleWebServe` 切换点各一处接入。
* **Base**：只跑桌面不开 HTTP → `webServe.enabled=false`；`--serve` 无头 → 跳过 `wails.Run`，`multicastSink` 对 Background ctx 的 Wails 出口静默跳过，等效 hub only。
* **Bad**：service 直调 `runtime.EventsEmit`（见 §7）；先 `eventSink()` 取值释放锁再 Emit（顺序缺口，B5 已消除）；`stopLocked` 改回 `Shutdown` 优雅等待（RPC 处理器内触发停机自等 3s 死锁复现）；`WSHub.Close` 直接 `close(c.send)`（与 readPump unregister 双重 close）；`RegenerateWebToken` 去掉 `webTokenMu`（并发轮换三处不一致）；测试用 `127.0.0.1:0` 构造「改址重启」（同串命中 Start 幂等 no-op，触发不了真实重启路径，须用 freePort 具体端口）；resume 补发在 readPump 直写 `conn`（与 writePump 并发写同一连接，gorilla 禁止）或阻塞式灌 send 队列（readPump 停滞 + 注销 defer 无法执行）；服务端在补发路径做与广播的排序协调（补发集/注册窗口广播经同一 FIFO 队列交叠属预期，有序性由前端 `ws-synced` 同步模式保证，服务端侧二次协调是过度设计）；前端实时模式直接丢弃 `seq <= lastSeq` 的补发窗口帧外的乱序帧（同步模式缓冲按 seq 升序落地前，先到的注册窗口广播会推进 lastSeq——去重只能在同步缓冲内做）。

## 6. Tests Required

* 事件出口：sink nil-ctx/非 Wails ctx 安全、fake sink 透传断言、SetEventSink 切换 -race（并发 Emit+切换）、emitCurrent 持锁投递顺序（`TestSinkHolder_EmitCurrentUnderLockNoStaleDelivery`：新出口首事件不得早于旧出口在途 Emit 结束）
* RPC：成功/未知方法/参数矩阵/AppError 透传/未认证 401/32MB 上限
* cookie 会话：认证矩阵（有效 cookie 200/错 cookie 唯一凭据 401/错 cookie + 对 query 重验证重种）、首访 Set-Cookie 属性（HttpOnly/SameSite=Strict/Max-Age/Path、TLS 加 Secure）、cookie 命中不重复 Set-Cookie、旧 cookie 轮换后 401、header 认证重种、轮换后同浏览器 `?token=` 重验证恢复（`TestWebHandler_RotationRecoveryViaQueryToken`）、首访资产级联（`TestWebHandler_CookieSessionCascade`）与真实 embed 资产全链路（`serve_cookie_smoke_test.go`：?token= 拿 Set-Cookie → 带 cookie 资产 200 → 无凭据 401 → cookie RPC 200）
* WS：认证矩阵（三 token 通道）、帧格式 `data:[]`、断连清理无 goroutine 泄漏、慢消费者踢出、SetToken 热轮换、cookie 通道裸连升级（`TestWSHub_CookieAuthChannel`）、`Close` 断开全部在途客户端 + 幂等（`TestWSHub_CloseDisconnectsAllClients`）+ manager Stop 集成（`TestWebServeManager_StopClosesWSClients`）
* WS 续传：序号单调与多客户端一致（`TestWSHub_EmitSeqMonotonicShared`）、resume 精确补发/追平/溢出断层/hub 重建断层/新连接不重放（上表四用例）、非协议上行帧忽略（`TestWSHub_ClientFrameGarbageIgnored`）、广播与 resume 补发并发 `-race` + 收帧序号不回退（`TestWSHub_ResumeConcurrentWithEmitRace`）
* 前端续传（`frontend/src/transport/__tests__/events.spec.js`）：lastSeq 记录与 resume 握手发送（首连/无基准不发送）、同步模式缓冲按 seq 升序去重落地、ws-gap 断层处理（提示+基准重置）、同步缓冲超限兜底、无 seq 帧向后兼容、实时模式重复帧丢弃
* webServe：manager 幂等/改址重启/端口占用三态、RegenerateWebToken 真实 HTTP 链路旧 401 新 200、并发轮换三处一致（磁盘==handler==hub）、Wait 代际（重启/停机/重开不返回 + 绕过管理器关 listener 真实退出返回错误）
* 预览同源：`Sec-Fetch-Site` 矩阵（same-origin 过关入业务校验 / cross-site、none、缺失 403）
* serve 端到端：`serve_rpc_smoke_test.go`（真实 *App + httptest：401/Bearer/query/WS 广播/对话框守卫不杀进程）
* 前端事件桥：无本地令牌裸连（`new WebSocket(url)` 不传 protocols、不弹门）、裸连失败转 token 门后转子协议、裸连成功后断线静默重连、有令牌走子协议零回归（events.spec.js）

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

**创建：** 2026-09-22（任务 `.trellis/tasks/09-21-agent`）
**更新：** 2026-09-22（任务 `.trellis/tasks/09-22-browser-audit-fixes`：补 Wait 代际、停机立即断开语义、token 轮换原子性、WSHub.Close、预览同源收敛、emitCurrent 持锁投递契约）
**更新：** 2026-09-22（任务 `.trellis/tasks/09-22-first-visit-asset-401`：token 三通道扩为四通道——新增 wb_token cookie 会话贯通（首访 `?token=` 自动种 30 天 HttpOnly/SameSite=Strict 会话，静态资产级联/RPC/WS 裸连透明通过），前端无本地令牌时 WS 裸连）
