# 浏览器访问模式：远程管理本地文件与 Agent

## Goal

将 WorkBench 页面暴露为纯浏览器可访问形式：进程启动后可在浏览器打开同一套 UI，用于从远程（或本机其他浏览器）交互管理本地文件、Git、终端，并运行/管理本地 agent（如 Claude Code），查看其输入与输出，提高个人效率。

## What I already know

* 现架构为 Wails v2 桌面应用：前端 Vue3 经 `window.go.main.App.*`（138 个委托方法）调用后端；后端经 `runtime.EventsEmit` 推事件（如 `terminal-output`/`terminal-exit`/`ai-task:*`）。
* 终端为 ConPTY（Windows）/ creack-pty（Unix），`service/terminal.go` startOutputPump 直接 EventsEmit，浏览器模式需替换为 WebSocket 通道。
* AI agent 能力已存在：`service/ai_function.go` 以 `claude --print --output-format stream-json --verbose` 跑任务，parseStreamLine 解析输出/计量/structured output，历史归档 `ai_task_history.go`。「运行本地 agent 并查看输入输出」与现有 AI 任务体系高度重合。
* `server/` 包已有内嵌 HTTP 服务先例（preview.go，文件预览）。
* 前端 `frontend/wailsjs/` 不入库，构建前由 wails generate 生成；前端另有 `src/test/wails-mock-defaults.js` 单一数据源 mock，说明前端对 Wails 绑定的调用点集中、可加 transport 适配层。
* 端口 34115 为 wails dev 前端 dev server；生产模式资源由 Wails assetserver 内嵌服务，无对外 HTTP。

## Assumptions (temporary)

* 单用户个人工具，无多租户/账号体系需求。
* 桌面窗口模式必须保留，浏览器模式为增量能力，两者共存于同一进程（待确认）。
* 远程访问场景为「自己的设备访问自己的机器」，可接受 token 认证 + 用户自建网络通道（Tailscale/frp/SSH 隧道），不做公网多用户服务。

## Open Questions

（已全部收敛）

## Requirements (evolving)

* 启动 WorkBench 后，浏览器打开 URL 即可使用与桌面一致的 UI。
* **MVP 范围＝全功能对齐**（已确认）：149 个绑定方法经通用 RPC 翻译层全部可用（文件/Git/AI 任务），终端走 PTY over WebSocket，业务前端零改动（polyfill `window.go`/`window.runtime`）。
* **部署形态**（已确认）：桌面模式默认同开 HTTP 服务（设置可关）；另支持 `--serve` 无头纯服务模式（跳过 `wails.Run`），用于远程常驻/开机自启。
* **安全边界**（已确认）：token 必开（首次启动生成随机 token，UI 可查/可重置），HTTP 与 WS 握手均校验；默认绑定 127.0.0.1，改绑 `0.0.0.0` 须设置确认并弹风险提示；TLS 不内置，远程走 Tailscale/frp/反代隧道。
* 浏览器端可发起/查看 AI agent 任务，实时看到流式输入输出。一期沿用现有单轮任务式（`claude --print`）；交互式多轮会话留二期（已归档 `docs/浏览器模式-二期扩展.md`），RPC/WS 通道保持双向扩展能力。

## Decision (ADR-lite)

**Context**：Wails v2 无官方浏览器模式；需要远程/浏览器访问本地文件与 agent。
**Decision**：同二进制自建浏览器通道——桌面默认同开 HTTP（设置可关）+ `--serve` 无头模式；静态资产自服务 + `/api/rpc` 通用翻译 149 绑定方法 + WS hub 推事件；前端 polyfill `window.go`/`window.runtime`，业务零改动；token 必开 + 默认 127.0.0.1，无内置 TLS；agent 交互一期沿用单轮任务式。
**Consequences**：不迁 Wails v3（避免全量重写）；需收敛 EventsEmit 触点为 EventSink 并修 `service/terminal.go:193,213` nil-ctx 隐患；远程安全依赖用户自建隧道；二期交互式会话须另立任务。

## Acceptance Criteria

* [ ] 浏览器访问 `http://127.0.0.1:<port>`，输入 token 后可使用与桌面一致的全功能 UI
* [ ] 无 token / token 错误的 HTTP 与 WS 请求均被拒绝
* [ ] 桌面模式行为不回归（现有 go/vitest/E2E 测试全绿）
* [ ] 终端在浏览器中可交互（xterm 双向、resize、断连提示）
* [ ] AI 任务流式输出在浏览器实时可见，历史可查
* [ ] `--serve` 无头模式可启动并对外服务，不创建窗口
* [ ] 默认仅绑定 127.0.0.1；改绑 0.0.0.0 需设置确认

## Definition of Done

* 单元/集成测试覆盖 RPC 翻译层、认证中间件、WS hub
* `go test ./...` + 前端 vitest + E2E 绿
* docs（README/功能说明/架构设计/部署说明）同步更新
* 二期扩展项已归档至 `docs/浏览器模式-二期扩展.md`

## Out of Scope (explicit)

* 浏览器模式 E2E（真实 serve 进程的浏览器端到端用例）：transport 适配层（PR4）以 vitest 单测覆盖（mock fetch/WebSocket），真实 serve 进程的浏览器 E2E 留后续任务补建
* Agent 交互式多轮会话（二期，见 `docs/浏览器模式-二期扩展.md`）
* 内置 TLS、多 token/审计（二期）
* 多用户/账号权限体系
* 公网直接暴露服务（用户自建隧道/内网穿透）
* 移动端专用 UI 适配（响应式尽力即可）
* 断线增量续传（一期整屏重放兜底，二期序号增量）

## Technical Notes

* 已勘察：`app.go`（App 内嵌 AppServices，138 委托零 diff 模式）、`service/terminal.go`（EventsEmit 输出泵）、`util/pty_windows.go`（ConPTY）、`service/ai_function.go`（claude CLI 流式）、`server/preview.go`（HTTP 先例）。
* 关键改造点猜想（待研究确认）：后端加 HTTP+WS 网关层翻译「绑定方法调用」与「Events 事件」；前端加 transport 适配层（wails / browser 双通道，运行时探测）；终端输出泵按连接分发。
## Research References

* [`research/web-exposure-patterns.md`](research/web-exposure-patterns.md) — code-server/ttyd/gotty/Jupyter/FileBrowser 共性：默认回环 + 单凭据 token + WS 多路复用 + TLS 内置或交隧道；`main.go:45` AssetServer.Handler 可挂自定义路由；绑定导出实为 149 个。
* [`research/wails-browser-bridge.md`](research/wails-browser-bridge.md) — v2 无官方浏览器模式（作者反对），v3 `-tags server` 实验性但迁移=全量重写；推荐同二进制 `--serve` 分支跳过 `wails.Run` 自建 HTTP + `/api/rpc` + WS hub（melody/gorilla）；前端 polyfill `window.go`/`window.runtime` 业务零改动；需修 `service/terminal.go:193,213` nil-ctx fatal 隐患。
