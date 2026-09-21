# Research: Wails v2 (Go) 桌面应用增量支持纯浏览器访问的可行路径

- **Query**: WorkBench（Wails v2.16.0 + Vue3 + Go）如何增量支持浏览器访问同一套前端与后端（headless / server 模式、shim 方案、事件桥接、无窗口运行的注意事项）
- **Scope**: mixed（内部代码勘察 + 外部官方文档/仓库/issue 验证）
- **Date**: 2026-09-21

---

## 一、结论速览

| 路径 | 官方支持度 | 改造量 | 风险 | 结论 |
|---|---|---|---|---|
| C1. `wails dev` dev server（:34115）浏览器访问 | 官方提供，但**仅限开发构建** | 零 | 低（语义漂移已由官方承认为隐患） | 短期演示/调试够用，**生产不可用** |
| C2. 生产二进制内加 `--serve` 分支：跳过 `wails.Run`，自建 HTTP(RPC+WS) 服务 + 前端 transport shim | 无官方支持（v2 无此模式），但 Go/HTTP 全是标准能力 | **中-大**（约 4 块改造，见路径 A） | 中（双通道语义需自证一致） | **推荐的增量方案**，同仓双模式 |
| C3. 迁移 Wails v3 用官方实验性 `-tags server` 模式 | **官方已合入 v3-alpha**（2026-01-25，PR #4903），标注 Experimental | 大（v2→v3 API 全量重写 + 前端绑定体系更换） | 中-高（alpha 阶段 API 可能再变） | 方向正确但当下迁移代价过高，**建议观望** |
| C4. `wails.Run` + `StartHidden:true` 等选项变体 | 选项存在（`options.App.StartHidden`） | 小 | — | **否决**：仅隐藏窗口，生产无任何浏览器可达端口，且仍依赖 WebView2 |

---

## 二、逐题查证

### 2.1 Wails v2 官方是否支持浏览器模式 / headless？（问题 1）

**结论：v2 官方不支持生产浏览器模式，也没有 headless 选项。**

已验证的事实链：

1. **生产形态无 HTTP 监听**。v2 生产构建把 `frontend/dist` 通过 `go:embed`（本项目 `main.go:25-26`）注入 asset server，由 WebView2 的自定义 scheme（Windows 上是 `http://wails.localhost/`）在 webview 进程内应答，**不监听任何 TCP 端口**，浏览器无从访问。
2. **:34115 只存在于 `wails dev`**。这是 v2 dev 命令的 devServer 默认端口（本项目 CLAUDE.md「常用命令」里 `netstat -ano | findstr ":34115"` 即指它）。dev 模式下它同时 serve 前端与 wails runtime，浏览器打开 `http://localhost:34115` 可直接调试——**这是官方唯一的浏览器访问形态，且不随 `wails build` 产物存在**。
3. **官方立场明确反对把浏览器模式当一等公民**。Wails 作者 leaanthony 在 issue #3274 评论中承认：浏览器 dev 模式容易让人针对浏览器 API 开发、到 webview 里才炸，「这让我们在 v3 里重新考虑是否还保留浏览器 dev 模式」。
4. **assetserver 不能独立服务**。`v2/pkg/options/assetserver.Options`（Assets / Handler / Middleware 三字段，已从源码核实）只是 `wails.Run` 的配置结构；真正的服务引擎在 `v2/internal/assetserver`（internal 包），脱离 `wails.Run` 生命周期无法复用。但本项目资产本来就是 `embed.FS`，独立 serve 等价物只是 `http.FileServer(http.FS(assets))` + SPA fallback + 复用现有 `server.PreviewHandler()`（该 handler 是纯 `net/http` 实现，`server/preview.go:35`，与 Wails 无耦合，可直接搬）。
5. `options.BindingsAllowedOrigins`（v2 `pkg/options/options.go` 已核实存在）是给 webview 内绑定调用做 origin 白名单的，不构成浏览器方案。

### 2.2 Wails v3 是否原生支持？v2 迁移代价？（问题 2）

**结论：v3 已有官方（实验性）server 模式，但 v2→v3 迁移对本项目是全量重写级别。**

v3 server 模式（PR #4903，2026-01-25 合入 v3-alpha，issue 正文与官方示例均已核实）：

- 构建：`go build -tags server`；运行时纯 HTTP 服务，无窗口、免 CGO；
- 配置：`application.Options{ Server: application.ServerOptions{Host, Port}, Services: [...], Assets: ... }`，支持 `WAILS_SERVER_HOST` / `WAILS_SERVER_PORT` 环境变量覆盖；
- 事件双向：浏览器 → HTTP → `app.Event.On(...)`；Go → `app.Event.Emit(...)` → **WebSocket 广播给所有已连接浏览器**；
- 健康检查 `/health`；每个浏览器标签页映射为一个 `browser-N` 虚拟窗口，窗口 API 全部 no-op；
- 配套 `Dockerfile.server` 模板与 `wails3 task build:server` / `build:docker`；
- 官方示例 `v3/examples/server/main.go` 前端通过 `/wails/runtime.js`（HTTP 原生）加载运行时，bindings 用法与桌面模式同构。

迁移代价（WorkBench 具体面）：

- 后端：`options.App{Bind: []interface{}}` → `application.New` + `application.NewService`；`OnStartup(ctx)` 生命周期与 `runtime.*` 调用全部换 API；
- 前端：v3 不再生成 `window.go.main.App.*` 全局，改用 `wails3 generate bindings` 产出的 bindings 包 + `@wailsio/runtime`。本项目 138 个绑定方法、所有组件的 `wailsjs` import、事件注册模式都要动（可用「把 `wailsjs/*.js` wrapper 内部换成 v3 bindings import」的缓冲层压缩业务组件改动，但 `wailsjs/` 目前不入库的约定要同步改）；
- v3 仍在 v3-alpha，server 模式自身标注 Experimental 且 2026-01 才合入，API 冻结无承诺。

### 2.3 社区实践先例（问题 3）

| 来源 | 内容 | 状态 |
|---|---|---|
| PR #1652「Feature/add support for server and hybrid」（2022） | 社区最早先例：`wails build -outputType server|hybrid`，server 不开窗口只开 dev server 式 HTTP（默认 :3112），hybrid 开窗 + 远程访问 | **未合入，已关闭**。maintainer stffabi 在 #2513 中把它引为「hybrid 模式可满足你的需求」的参考实现 |
| Issue #2513「Browser only mode」 | 请求 `--webonly` 构建/运行模式；stffabi 回复指向 #1652 | 已关闭（未计划） |
| Issue #3274「Browser-Only Development Mode」 | 请求 `wails dev --browser-only`；leaanthony 长评说明顾虑 | 已关闭 |
| PR #4903 + `v3/examples/server` + 官方指南 server-build | v3 官方 server 模式（见 2.2） | **已合入 v3-alpha** |

**没有检索到「Wails v2 生产浏览器模式」的成熟开源项目先例**；社区做法一致收敛为：要么等 v3，要么自建 HTTP 层。

### 2.4 前端 shim 方案可行性（问题 4）

**结论：可行，且本项目结构对此特别友好。**

本项目前端所有后端调用都经过两层间接：

1. `frontend/wailsjs/go/main/App.js`（生成物，已核实）：每个方法形如 `return window['go']['main']['App']['Xxx'](arg1, ...)` —— 动态查表；
2. 事件统一走 `wailsjs/runtime/runtime` → `window.runtime.EventsOn/EventsEmit`（如 `useTerminal.js:13`）。

因此只要在应用模块加载之前 polyfill `window.go` 与 `window.runtime` 两个全局对象，全部 138 个调用点与所有事件注册点**零业务改动**即可切到浏览器通道：

- `window.go.main.App.X = (...args) => fetch('/api/rpc/X', {method:'POST', body: JSON.stringify(args)})`；
- `window.runtime.EventsOn/Off/Emit` → WebSocket 消息映射，必须复刻「`EventsOn` 返回注销闭包」语义（WorkBench 已将其固化为规范，见 CommitHistory.spec.js:28-29 注释）。

项目内已有同型先例：vitest/E2E 的 `window.go`/`window.runtime` mock（`frontend/src/test/wails-mock-defaults.js` 单一数据源）本质上就是「无 Wails 环境下的 transport 替身」，浏览器 shim 是它的网络版。

业界同型模式成熟：IPC 抽象层（Electron preload 桥 vs 浏览器 HTTP adapter 双通道）是常见工程实践；v3 server 模式内部（HTTP + WS 承载同一套 message processor）证明了「同一后端、双 transport」架构成立。

需要注意的语义坑（shim 层必须显式处理）：

- **错误通道**：桌面端错误经 `ErrorFormatter`（`main.go:53`）转 `{code,message}`；RPC 层要以相同结构返回失败（建议非 2xx + JSON body）；
- **参数序列化**：138 个方法签名各异，注意 `[]byte`（如 `ReadFileBytes` 仍走 base64 语义）、时间、nil 语义与 JSON 的映射；前端已强制 `decodeBase64Utf8` 处理中文（规范在案），RPC 层保持返回 base64 即可复用；
- **多标签页并发**：桌面是单窗口单会话，浏览器天然多客户端——选中工作目录、终端会话、AI 在途任务/取消都是全局单例语义，需明确「共享同一后端状态」还是「每标签页会话」并写进设计；
- **加载时序**：shim 必须先于任何业务模块执行（入口 main.js 首行同步注入，或由服务端在 index.html 注入 `window.go = ...` 脚本）。

### 2.5 runtime.EventsEmit 的事件如何桥接到 HTTP 场景（问题 5）

**结论：自建 WS hub 是标准做法；推荐 melody（最少代码）或 gorilla/websocket（最大生态）。**

现状盘点（已核实 `EventsEmit` 全部点位）：`terminal-output` / `terminal-exit`（`service/terminal.go:193,198,213`）、`update:download-progress`（`service/update.go:189,213`）、AI 事件（`service/ai_function.go:1321`）、git 事件（`service/git.go:576` safeEmit）。

桥接模式：把「发事件」从对 `runtime.EventsEmit(ctx, ...)` 的直接依赖收敛为一个 `EventSink` 接口（wails 通道 = 现有实现；serve 通道 = WS hub `Broadcast(名, 数据)`），service 层只依赖接口。WS hub 负责连接管理 + JSON 帧广播。

Go WS 库选型（活跃度已核实）：

| 库 | Stars | 最近 push | 特点 |
|---|---|---|---|
| gorilla/websocket | 24.9k | 2025-03 | 事实标准；官方 chat example 就是 hub 广播范式 |
| coder/websocket（原 nhooyr.io/websocket） | 5.5k | 2026-06 | 活跃维护、context 风格、API 现代 |
| olahol/melody | 4.1k | 2025-10 | 自带 Broadcast/广播 hub，代码量最少 |

（另可对照：v3 官方 server 模式内部就是「WebSocket event broadcasting to connected browsers」的同型实现。）

### 2.6 同一二进制 `--serve` 跳过窗口直接跑 HTTP 的注意点（问题 6）

**结论：可行（`wails.Run` 不是必经之路），但有三个已核实的具体坑。**

1. **`wails.Run` 必然创建窗口**（Windows 下拉起 WebView2），v2 无 headless 选项；`StartHidden:true` 只是隐藏窗口，不是服务模式。所以 `--serve` 分支必须**不调用 `wails.Run`**，直接自建 `net/http`，这对 WorkBench 是成立的：service 层是纯 Go，唯一的 Wails 依赖就是事件发射。
2. **nil ctx 会让 `runtime.EventsEmit` fatal**。项目内测试注释已明确（`service/ai_function_delegates_test.go:14`：「ctx 传 nil……避免触发 runtime.EventsEmit fatal」）。现有防护不齐：
   - 有防护：`safeEmit`（`service/git.go:576` 路径）、`ai_function.go:1321`（ctx==nil 检查）、`terminal.go:197`（output 有 `s.ctx != nil` 判断）；
   - **无防护：`terminal.go:193` 与 `terminal.go:213` 的 `terminal-exit` 发射**——serve 模式下 PTY 退出会直接 fatal，收敛 EventSink 时必须一并覆盖。
   - `app.startup`（`main.go:47` 注入 ctx + Logger/目录初始化）在 serve 模式不会被调用，需为 serve 分支提供等价的初始化入口。
3. **PreviewHandler 与静态资产可直接复用**：`AssetServer.Handler` 仅在 Assets 未命中时被调用（`server/preview.go:33-34` 注释），独立 http.ServeMux 上按「静态优先、`/preview-pdf`、`/preview-raw`、`/api/rpc`、`/ws` 次序」挂载即可等价复刻。

---

## 三、推荐路径（按落地顺序）

### 路径一（零改造，立即可用）：`wails dev` 的 :34115 浏览器访问

适用：本机调试、远程桌面/投屏演示。不解决生产浏览器访问。注意官方已承认 dev 期浏览器语义可能与 webview 有差异（2.1 第 3 条），交付验收以桌面窗口为准。

### 路径二（推荐主方案）：`--serve` 双模式二进制 + 前端 transport shim（上述 C2）

改造要点清单：

1. **Go 侧启动分支**：`main.go` 增 `--serve`（含 host/port 参数），走 `server.Serve()`：`http.FileServer(http.FS(assets))` + SPA fallback + 复用 `PreviewHandler()` + `/api/rpc/` + `/ws`；不调用 `wails.Run`。
2. **RPC 端点**：138 个方法注册。`Bind` 的反射调用逻辑在 wails internal 不外露，务实做法是写一个小的注册 helper（方法名 → `func(args []json.RawMessage) (any, error)`）手工挂接，或以泛型包装逐个注册；错误统一走 `formatAppError`（`main.go:74`）保证 `{code,message}` 通道一致。
3. **EventSink 收敛**：定义 `Emit(name string, data ...any)` 接口；wails 模式注入现实现，serve 模式注入 melody/gorilla WS hub；同步修复 `terminal.go:193,213` 的 nil-ctx 隐患（该修复独立于浏览器功能也成立）。
4. **前端 shim**：构建期用 Vite 环境变量或运行时探测注入 `src/transport/browser-shim.js`，polyfill `window.go`/`window.runtime`（复刻 `EventsOn` 返回闭包语义），业务代码零改动；`wails-mock-defaults.js` 的语义可作为 shim 行为的第三份参照。
5. **测试与门禁**：RPC handler 与 WS hub 是纯 Go，可进常规单测；跨通道一致性用集成测试（`-tags=integration`）对同一方法在两通道各断言一次。

### 路径三（中期观望）：v3 `-tags server` 官方模式（上述 C3）

触发条件建议：v3 进入稳定/beta、或产品明确需要容器化部署（官方已给 `Dockerfile.server`）时重估。届时可先用官方 `examples/server` 对 WorkBench 做迁移 spike，重点评估前端 wailsjs→bindings 适配层成本。

---

## 四、参考链接

- v2 无浏览器生产模式 / 官方立场：
  - Issue #3274（leaanthony 评论）：https://github.com/wailsapp/wails/issues/3274
  - Issue #2513（stffabi 指向 #1652）：https://github.com/wailsapp/wails/issues/2513
  - PR #1652（server/hybrid 先例，未合入）：https://github.com/wailsapp/wails/pull/1652
- v3 官方 server 模式：
  - PR #4903（已合入 v3-alpha，2026-01-25）：https://github.com/wailsapp/wails/pull/4903
  - 官方示例：https://github.com/wailsapp/wails/blob/master/v3/examples/server/main.go
  - 示例 README（server mode 说明）：https://github.com/wailsapp/wails/blob/master/v3/examples/server/README.md
- v2 源码佐证：
  - options.App（StartHidden 等，无 headless）：https://github.com/wailsapp/wails/blob/master/v2/pkg/options/options.go
  - assetserver.Options（Assets/Handler/Middleware）：https://github.com/wailsapp/wails/blob/master/v2/pkg/options/assetserver/options.go
- WebSocket 库：
  - https://github.com/olahol/melody
  - https://github.com/gorilla/websocket （chat example 即 hub 范式）
  - https://github.com/coder/websocket

## Caveats / Not Found

- 未检索到「Wails v2 生产浏览器模式」的成熟开源项目先例（社区收敛为等 v3 或自建 HTTP 层）；`wails dev` devServer 默认端口 34115 的官方文档页（wails.io CLI reference）因站点防护未能直接抓取原文，结论依据 v2 项目实践与本项目 CLAUDE.md 在案记录，置信度高但未引原文。
- v3 迁移代价一节基于 v3 官方示例与生成物体系的公开事实推断前端改面，未实际做迁移 spike 验证工时。
- 本项目内部「多标签页并发语义」（2.4 第 3 条）仅盘点现状，未做设计决策，需在方案阶段拍板。
