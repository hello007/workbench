# 浏览器通道审核缺陷修复

## Goal

修复浏览器访问模式三维度代码审核（安全/并发与生命周期/跨层契约）发现的 1 🔴 + 8 🟡 缺陷。方案已由审核结论给出，无需重新设计。

## Requirements

### 后端组（B 组）

* **B1（🔴）`main.go:144`**：`--serve` 模式 SetWebServeConfig 在 HTTP 处理器内 Stop/重启 manager，旧 `done` 收 `ErrServerClosed` 致 `Wait()` 误判正常停机返回 → 进程退出。修法：`Wait` 改按代际（generation）循环重读 `m.done`，或等效方案保证重启/关闭不误杀 `--serve` 主流程；同时消除 `stopLocked` 自等在途请求空转 3s 的问题。
* **B2 `web_serve.go:110`**：`WSHub` 增加 `Close()`（写锁遍历 clients 逐个 `conn.Close()`，readPump 走既有 unregister 收尾）；`stopLocked` 在 `applySink(nil)` 前调用，杜绝 Shutdown 不关 hijacked WS 连接导致的僵尸客户端与 goroutine 泄漏。
* **B3 `app_webserve.go:89`**：`RegenerateWebToken` 的生成→落盘→热轮换全程加互斥，防并发交错致磁盘/handler token 不一致。
* **B4 `server/web.go:72`**：serve 模式预览路由免认证收敛——优先方案：校验 `Sec-Fetch-Site: same-origin`（缺失头视为放行以兼容非浏览器客户端？否——预览仅浏览器 iframe 用，缺失/跨站一律拒绝）；或预览路由套 token（`?token=` 通道）。二选一，注释说明取舍，桌面模式路径不受影响。
* **B5 `service/sink.go:39`**：`Emit` 移入 `sinkHolder` RLock 域内执行，消除切换窗口单事件旧 sink 投递的顺序缺口。

### 前端组（F 组）

* **F1 `SettingsPanel.vue:515`**：浏览器模式 `RegenerateWebToken` 成功后调 `transport/token.js` 的 `setToken(newToken)` 持久化，避免本页自锁 401。
* **F2 `events.js:96`**：WS 连续重连失败达阈值（建议 3 次）后转 `requestToken()` 门或清本地令牌强制重认证，终结无限静默重连。
* **F3 `events.js:70`**：connect 前校验 token 为 hex（服务端签发即 64 位 hex），非法字符拒绝并走 token 门重建，杜绝 `new WebSocket` SyntaxError 未捕获致重连链死亡。
* **F4 `events.js:91`**：后端不可达场景给出反馈——连续失败经 connBanner 提示「服务不可达」，token 门增加关闭/取消出口，终结无解释循环弹门。

## Acceptance Criteria

* [ ] `--serve` 下浏览器改绑定地址/关开关：进程存活、服务在新地址可用（集成测试覆盖代际 Wait）
* [ ] 改址重启/关开关后旧 WS 连接被关闭（无僵尸客户端，Hub Close 有测试）
* [ ] 并发 RegenerateWebToken 后磁盘 token == handler/hub token（并发测试）
* [ ] serve 模式跨站预览请求被拒（Sec-Fetch-Site 或等效测试），桌面预览回归不变
* [ ] 前端：重新生成 token 后本页继续可用（vitest）；连败转 token 门；非法 token 不再抛未捕获 SyntaxError；后端不可达有横幅反馈且门可取消
* [ ] `go build/test/vet`、`-race` 抽查、前端 build/test、覆盖率门禁全绿

## Definition of Done

* 每组修复带回归测试（断言点见 AC）
* spec 沉淀：browser-channel.md 补 WSHub.Close、Wait 代际、token 轮换原子性三条契约
* 审核结论与本 PRD 对应关系可追溯

## Out of Scope

* 预览路由的深度防御体系改造（只做单一收敛方案）
* 浏览器真实进程 E2E（仍留后续）

## Technical Notes

* 审核来源：三维度 cavecrew-reviewer（2026-09-22），范围 `ac2089c..2cfd0e6`
* B1/B2 同属 manager 停机面，须同一代理一次改；F1-F4 同属 token 轮换闭环
* 后端组与前端组文件零交集，可并行实现
