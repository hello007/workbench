# 浏览器首访资产 401 修复（cookie 会话）

## Goal

修复首访 `http://127.0.0.1:36115/?token=<t>` 后 `/assets/*.js` 全部 401、应用无法启动、token 门无法出现的缺陷。根因：`?token=` 查询参数只随单个请求发送，浏览器后续静态资产请求不带任何凭据。

## 方案（已定）

cookie 会话贯通：

1. **server/web.go token 中间件**：凭据优先级扩为 `Authorization: Bearer` > `X-Auth-Token` > `wb_token` cookie > `?token=`；经 header/query 认证成功且 cookie 未带/不一致时 `Set-Cookie: wb_token=<t>; Path=/; Max-Age=2592000; HttpOnly; SameSite=Strict`（`r.TLS != nil` 时加 Secure）。cookie 校验同样恒定时间比较。带 cookie 的请求不再重复 Set-Cookie。
2. **transport/events.js connect()**：localStorage 有合法 hex token → 现状子协议通道；无 token → **裸 WebSocket 连接**（不传 protocols），服务端经 cookie 认证。裸连 401 失败才走 token 门（复用 everOpened/连败逻辑）。
3. 顺带：静态资产与 index.html 同受 token 保护不变（cookie 使其透明通过）；/healthz 豁免不变；令牌轮换后旧 cookie 失效 → 走门，语义与现有一致。

## Acceptance Criteria

* [ ] 全新浏览器（无 localStorage）打开 `/?token=<t>`：index.html 200 + Set-Cookie，`/assets/*.js` 200，应用启动可用，无需再次输 token
* [ ] RPC 经 cookie 认证 200；WS 裸连经 cookie 升级成功收帧
* [ ] 错 cookie 401；无任何凭据 401；token 轮换后旧 cookie 401
* [ ] 子协议通道（localStorage 有 token）行为不回归（vitest 存量用例）
* [ ] `go build/test/vet`、`-race` 抽查、前端 build/test、覆盖率门禁全绿

## Out of Scope

* index.html 内联 bootstrap 注入 localStorage（cookie 方案后无需）
* 浏览器真实进程 E2E

## Technical Notes

* 报告来源：用户实测 2026-09-22，`/assets/index-BjOTlykT.js` 401
* PR1 遗留备注「PR3 正式链路走 header 通道不受影响」只覆盖了 index.html 自身，漏了资产级联请求——冒烟盲区
* 文档同步：功能说明/部署说明 token 通道表、browser-channel.md token 三通道契约改四通道（+cookie）
