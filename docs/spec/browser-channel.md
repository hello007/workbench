# 浏览器访问通道契约

> 适用范围：WorkBench 浏览器访问模式（桌面同开 HTTP 与 `--serve` 无头）涉及的全部跨层契约：事件出口（EventSink）、RPC 翻译层（/api/rpc）、WebSocket 事件 hub、token 认证与热轮换、前端 transport polyfill。改动本通道任何一环前必读。
> 来源：v1.7 浏览器访问模式 epic（任务 `.trellis/tasks/09-21-agent`，2026-09）。

## 1. Scope / Trigger

以下改动触发本文档约束：

* service 层新增/修改向前端推送的事件
* 新增/修改 App 绑定方法（浏览器经 /api/rpc 可达）
* /api/rpc 请求-响应协议、WS 帧格式、token 校验通道变更
* settings.json `webServe` 段结构变更

## 2. Signatures

### 2.1 事件出口（service 层，唯一合法通道）

```go
// service/event_sink.go
type EventSink interface { Emit(name string, data ...any) }
func NewWailsEventSink(ctx context.Context) EventSink   // 桌面实现，内部 runtime.EventsEmit
func emitEvent(sink EventSink, name string, data ...any) // 集中 nil/非 Wails ctx 防护

// service/sink.go —— 需运行时切换出口的 service 嵌入此持有器
type sinkHolder struct{ mu sync.RWMutex; sink EventSink }
func (h *sinkHolder) SetEventSink(s EventSink)
func (h *sinkHolder) eventSink() EventSink
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
GET /ws?token=<t>  或  Sec-WebSocket-Protocol: <t>     （升级前 401 拒绝，勿升级后踢）
帧: {"event":"<事件名>","data":[...]}    零载荷 data 为 [] 非 null
```

浏览器 shim 以 `callback(...frame.data)` 还原 Wails `EventsOn` 回调形态。

### 2.4 webServe 绑定方法（app_webserve.go，2026-09 新增 4 个）

`GetWebServeConfig` / `SetWebServeConfig(enabled bool, bindAddress string)` / `GetWebServeToken` / `RegenerateWebToken`。

## 3. Contracts

| 契约 | 内容 |
|---|---|
| 错误形态三端一致 | main.go `formatAppError` `{code,message}` ↔ rpc.go `error:{code,message}` ↔ transport/rpc.js reject `{code,message}`；`E_RPC_*` 六码取值唯一源 `model/app_error.go`，前端 `utils/error.js` ErrorCode 同步（新增码三处同步，同 logging-and-errors.md） |
| token 三通道 | HTTP：`Authorization: Bearer` > `X-Auth-Token` > `?token=`（仅首访手工验证，勿写进文档推荐）；WS：`?token=` 或子协议。恒定时间比较（`subtle.ConstantTimeCompare`），fail-closed。落盘 `data/web_token` 0600，已 gitignore |
| token 热轮换 | `RegenerateWebToken`/`WebHandler.SetToken`/`WSHub.SetToken`：HTTP 与新 WS 握手立即生效；在途 WS 连接数据面不再校验 token，保持至断开重连（UI 文案已交代） |
| webServe 设置段 | `AppSettings.WebServe *WebServeSettings` 指针字段：段缺失→`EnsureWebServeDefaults` 补 `enabled:true`（默认同开）、`bindAddress:"127.0.0.1:36115"`；`--listen` 命令行 > settings.json > 默认值 |
| polyfill 注入时序 | wailsjs 生成 wrapper 在**函数调用体内**才解引用 `window.go`；`main.js` 在 `createApp` 前 `installBrowserTransport()`（有 Wails runtime 则 no-op）即全链路生效。新增模块顶层代码禁止触碰 `window.go`/`window.runtime` |
| 桌面/serve 装配同构 | 桌面同开与 `--serve` 共用 `assembleWebServe`（main.go），禁复制装配逻辑产生第二份 |

## 4. Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| 无/错 token 访问 RPC、静态页（除 /healthz） | 401 JSON |
| WS 握手无/错 token | HTTP 401 拒绝升级 |
| serve 模式调窗口对话框方法（Save/OpenFileDialog 等） | `wailsRuntimeUnavailable` 守卫返回可读错误（不守卫则 wails `log.Fatalf` 杀整个服务进程——真实事故，PR2 修复） |
| RPC 未知方法 / 未导出方法 / 变参方法 | `E_RPC_METHOD_NOT_FOUND` / 拒绝 |
| 参数个数/类型不匹配、浮点转整型 | `E_RPC_ARG_COUNT_MISMATCH` / `E_RPC_ARG_TYPE_MISMATCH` |
| body 超限 / 非法 JSON / 非 POST | 32MB `MaxBytesError` 专错 / `E_RPC_BAD_REQUEST` / 405+Allow |
| 桌面启动端口被占 | `slog.Warn` 降级，Running=false，桌面不崩；`SetWebServeConfig` 可重试 |
| 非回环绑定 | 后端 slog.Warn + 前端 ElMessageBox 风险确认双保险 |

## 5. Good/Base/Bad Cases

* **Good**：新 service 需要推事件 → 嵌 `sinkHolder`，构造注入 wails sink，发事件处 `emitEvent(s.eventSink(), "xxx", data)`；`AppServices` 装配行 + `assembleWebServe` 切换点各一处接入。
* **Base**：只跑桌面不开 HTTP → `webServe.enabled=false`；`--serve` 无头 → 跳过 `wails.Run`，`multicastSink` 对 Background ctx 的 Wails 出口静默跳过，等效 hub only。
* **Bad**：service 直调 `runtime.EventsEmit`（见 §7）；测试里包级 var 供调参（-race 实锤竞态，PR3 已改实例字段）；`EventsOff('name')` 全局清监听（见 cross-layer-contracts.md）。

## 6. Tests Required

* 事件出口：sink nil-ctx/非 Wails ctx 安全、fake sink 透传断言、SetEventSink 切换 -race（并发 Emit+切换）
* RPC：成功/未知方法/参数矩阵/AppError 透传/未认证 401/32MB 上限
* WS：认证矩阵（两 token 通道）、帧格式 `data:[]`、断连清理无 goroutine 泄漏、慢消费者踢出、SetToken 热轮换双通道
* webServe：manager 幂等/改址重启/端口占用三态、RegenerateWebToken 真实 HTTP 链路旧 401 新 200
* serve 端到端：`serve_rpc_smoke_test.go`（真实 *App + httptest：401/Bearer/query/WS 广播/对话框守卫不杀进程）

## 7. Wrong vs Correct

### Wrong

```go
// service 层直调 wails runtime：非 Wails ctx 时 log.Fatalf 杀进程；
// serve 浏览器通道永远收不到该事件
runtime.EventsEmit(s.ctx, "task-progress", progress)
```

### Correct

```go
// 经 EventSink 出口：桌面走 wailsEventSink，同开/serve 自动切换到 WS hub 广播
emitEvent(s.eventSink(), "task-progress", progress)
```

> **Warning**：`runtime.EventsEmit` 的 ctx 缺 `frontend` 键（含 `context.Background()`）时 wails v2.16 直接 `log.Fatalf` 退出**整个进程**——serve 模式下任何一个持 token 请求即可杀死服务。一切事件必须走 EventSink；一切依赖 Wails 窗口运行时的调用（对话框等）必须过 `wailsRuntimeUnavailable` 守卫。

---

**创建：** 2026-09-22（任务 `.trellis/tasks/09-21-agent`）
