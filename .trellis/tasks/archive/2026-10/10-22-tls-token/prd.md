# 浏览器模式三期批次：多设备连接语义 + 内置 TLS + token 管理细化

## Goal

交付 docs/浏览器模式-二期扩展.md 剩余清单 §2「多设备同时连接语义」与 §3「安全增强（内置 TLS + token 管理细化）」三项能力，使浏览器模式可安全暴露到非回环网络（多设备访问），摆脱外部反代/隧道依赖。

## What I already know（repo 现状）

* token：单 token，`data/web_token` 持久化（`server.SaveToken/GenerateToken`），`RegenerateWebToken` 热轮换（`RotateToken` 更新 HTTP 认证与 WS 握手基准，已建 WS 连接保持至断开）
* 认证三源：HTTP header/cookie、WS `?token=` 查询参数、子协议；`WebHandler.requireToken` 单点
* WSHub：全局广播 + 全局单调 seq（2000 帧环形缓冲）断线续传，锁序 seqMu→mu 已收口；**无按连接过滤**——多端收到全量事件
* 终端会话：`TerminalService.sessions` 全局注册表，`terminal-output` 事件带 sessionID 广播，任何连接可对任何会话 WriteInput——多端同写即「共享终端、后写覆盖」，无归属/隔离概念
* HTTP 入口：`webServeManager.startLocked` 自持 `net.Listen` + `http.Server`，不经 Wails AssetServer，TLS 插入点清晰（`tls.NewListener` 或 `srv.ServeTLS`）
* 设置段：`AppSettings.WebServe`（BindAddress 等，`EnsureWebServeDefaults`），SettingsService 有内存缓存（Load 返回共享指针只读禁改）
* 前置收口（10-22）：WSHub 断线续传与上行边界刚完成（docs/spec/browser-channel.md）

## Assumptions (temporary)

* TLS 仅服务浏览器通道（`--serve` 与桌面同开的 web 服务），不涉桌面窗口
* 前端 transport 适配层（wails/browser 双通道）无需感知 TLS（浏览器侧 wss/https 由 URL scheme 决定）

## Research References

* [`research/web-tls-compat.md`](research/web-tls-compat.md) — TLS 接入推荐 `tls.NewListener` + 自建 tls.Config 钉 ALPN http/1.1（ServeTLS 默认开 h2 ALPN，gorilla 仅 HTTP/1.1 升级）；`listenAddr`/`srv.Close` 语义 TLS 下不变；`Start` 幂等键须扩「listen+TLS 配置」；自签 ECDSA P-256 + SAN 必填、落盘 data/ 0600、损坏 fail-fast；不发 HSTS；前端 transport 已按 location.protocol 自适配 wss 零改动
* [`research/multi-device-token-semantics.md`](research/multi-device-token-semantics.md) — 现状多端广播等价 tmux 共享 attach（业界流派非缺陷）；Web IDE（code-server/Gitpod）均不做协同合并，最小合理实现 = 维持广播 + 连接数提示 + 明示「后操作覆盖」；终端归属持久型（长任务载体）；多 token = GitHub PAT 式数据模型（id/name/SHA-256 哈希/lastUsedAt/expiresAt/revoked），存量 web_token 迁移为首条；有效期惰性失效；审计 = JSONL 认证事件级，普通 RPC 不记；续传按连接与 token 解耦兼容成立

注意：multi-device 研究部分来源未联网核验（沙箱出网受限），标注「未联网核验」的表在方案定稿前人工复核。

## Research Notes（收敛中的方案草案）

**方案 A：多设备语义——维持共享广播 + 明示语义（推荐 MVP）**
* How：不改广播架构；前端加「N 个设备已连接」提示 + 多端写终端时明示后写覆盖；终端会话不绑连接（长任务载体持久型）
* Pros：零架构改动、与断线续传续传面完全兼容、对齐 code-server/tmux 语义
* Cons：无隔离，多端误操作风险靠提示缓解

**方案 B：共享 + 终端会话独占接管**
* How：方案 A 基础上加会话级「接管」态（独占写入权，其他连接只读镜像）
* Pros：防多端输入交织；保留共享查看
* Cons：接管态状态机 + 前后端协议扩展，复杂度中

**方案 C：按连接隔离终端归属**
* How：WSHub 按连接过滤事件 + 终端会话归属绑定连接/设备
* Pros：完全隔离
* Cons：事件过滤须重算 resume 补发面（研究明确标注为前置约束）、与「长任务载体」定位冲突（断线即失会话或需 orphan 回收状态机）

**token 管理草案**：多 token GitHub PAT 式（生成一次性回显、SHA-256 哈希落盘、可命名/撤销/可选过期、惰性失效）；存量 data/web_token 迁移为首条记录；RotateToken 收窄为「新建+撤旧」编排；审计 JSONL 认证事件级。

**TLS 草案**：`tls.NewListener` + 钉 ALPN；配置支持「自签引导（默认）+ 用户证书路径」；证书加载失败在 startLocked 字段赋值前 fail-fast（无半死态）；Start 幂等键扩 listen+TLS。

## Requirements

**R1 多 token 管理（D2/D4）**
* GitHub PAT 式数据模型：id / name（设备名）/ SHA-256 哈希 / createdAt / lastUsedAt / expiresAt（可空=永久）/ revoked
* 存量 `data/web_token` 首次读取迁移为首条记录（名称「默认」）
* 生成一次性回显明文，落盘只存哈希；认证命中更新 lastUsedAt
* 有效期：永久（默认）/ 7 / 30 天 / 自定义日期；惰性失效；撤销立即生效
* `RegenerateWebToken`（RotateToken）收窄为「新建 + 撤旧」编排，前端行为不变
* 认证链改造：`WebHandler.requireToken` / WSHub `SetToken` 单 token 比较 → 哈希集合匹配（恒定时间比较语义保留）；WS 子协议回显逻辑同步
* 前端 token 管理 UI（列表/新建/撤销/过期标记）

**R2 审计日志（D2）**
* JSONL append：认证成功/失败（时间、来源 IP、UA、token id，不记明文 token）+ token 生命周期操作（创建/撤销/轮换/过期）
* 落盘 data/，append 语义独立实现（util.SaveJSON 原子写不适用）；按大小上限轮转
* 来源 IP 默认取 RemoteAddr，不做 XFF 信任（无可信反代判定面）

**R3 内置 TLS（D3）**
* 接入：`tls.NewListener(ln, tlsConfig)` + `srv.Serve`，tls.Config 钉 `NextProtos: ["http/1.1"]`；`m.ln` 保留原始 TCP listener（`listenAddr()` 语义不变）
* 默认自签引导：ECDSA P-256，SAN 覆盖本机候选 IP（复用 `localIPv4Addresses`），落盘 data/（0600），损坏 fail-fast 不静默重生成
* 用户证书：settings 配 cert/key PEM 路径，非空时跳过自签，加载失败 fail-fast
* `Start` 幂等键扩为「listen + TLS 配置」（同地址开/关 TLS 须重启）
* 证书加载失败在 `startLocked` 字段赋值前返回（无半死态），且在停旧实例之前
* 不发 HSTS；前端 transport 按 location.protocol 自适配 wss，前端零改动
* 前端设置 UI：TLS 开关 + 证书路径

**R4 多设备语义明示（D1）**
* 架构零改动：广播制、终端会话不绑连接
* 前端「N 个设备已连接」提示（WSHub 连接数变化事件）
* 终端多端写入提示「多设备同时操作，后输入覆盖先输入」

## Acceptance Criteria

* [ ] 多 token：新建（明文仅一次回显）/撤销/过期后认证全部拒绝；存量单 token 无感迁移；RotateToken 后旧 token 失效、新 token 可用（热轮换不断连接语义保持）
* [ ] 认证命中更新 lastUsedAt；审计 JSONL 含登录成功/失败与 token 生命周期事件，无明文 token
* [ ] TLS 开启后 https + wss 真机浏览器可用；同地址开/关 TLS 正确重启；证书加载失败服务保持原态不半死
* [ ] 自签证书首次生成、损坏 fail-fast；用户证书路径生效
* [ ] 前端设备连接数提示准确；终端多端写入提示可见
* [ ] 断线续传回归不受影响（resume 契约不变）
* [ ] 覆盖率门禁达标；docs/spec/browser-channel.md 等规范同步更新

## Decision (ADR-lite)

**D1 多设备语义 = 方案 A：维持共享广播 + 明示语义（2026-10-22 用户选定）**
* Context：多端同连现状等价 tmux 共享 attach；隔离方案与长任务载体定位冲突且牵动 resume 补发面
* Decision：架构零改动；前端「N 个设备已连接」提示 + 多端写终端明示「后操作覆盖」；终端会话不绑连接
* Consequences：无隔离，误操作靠提示缓解；独占接管（方案 B）留作后续增强口子

**D2 审计粒度 = 仅认证事件级（2026-10-22 用户选定）**
* Context：单用户桌面工具，操作级审计噪音大于价值；多 token 落地后可按 token id 定位来源设备
* Decision：JSONL append——认证成功/失败（时间、来源 IP、UA、token id，不记明文）+ token 生命周期操作（创建/撤销/轮换/过期）；普通 RPC 不记
* Consequences：操作级回溯如未来出现需求，JSONL 形态可追加字段扩展

**D3 TLS 证书供给 = 自签引导 + 用户证书双形态（2026-10-22 用户选定）**
* Context：两类场景——临时手机访问（自签够用）与有正式域名证书（用户自带）
* Decision：默认自签（ECDSA P-256，SAN 覆盖本机候选 IP，落盘 data/ 0600，损坏 fail-fast 不静默重生成）；高级设置 cert/key PEM 路径覆盖（非自签时跳过生成，加载失败 fail-fast）；不发 HSTS
* Consequences：自签证书浏览器有警告（保留「继续前往」路径），真机冒烟验证 https 例外与同源 wss 外溢行为

**D4 token 有效期 = 默认永久 + 可选短命档（2026-10-22 用户选定）**
* Context：个人自用默认永久省心；给外部设备发凭据时可加期限（GitHub PAT 模式）
* Decision：创建时可选 永久（默认）/ 7 天 / 30 天 / 自定义日期；惰性失效（认证时判定，无后台清理任务）；撤销立即生效；`RotateToken` 收窄为「新建 + 撤旧」编排兼容存量热轮换入口
* Consequences：到期 token 不自动删记录（保留审计与列表可见性），界面标记「已过期」

## Open Questions

（已全部收敛，见 Decision ADR-lite）

## Acceptance Criteria

* [ ] 待 brainstorm 收敛

## Definition of Done (team quality bar)

* 单元/集成测试齐（对齐 coverage 门禁）
* Lint / typecheck / CI green
* docs/spec/browser-channel.md 等规范沉淀更新
* 真机冒烟（连同 chat-interactive-smoke-pending 记忆欠账一起）

## Out of Scope (explicit)

* 移动端适配（§2 剩余项，另行任务）
* 文件大流量优化 / 无头服务化（§4）

## Technical Notes

* 前置阅读：docs/spec/browser-channel.md（WSHub 续传契约 §2.4、/api/rpc 三通道）
* web_serve.go 装配链：`startLocked` → `assembleWebServe` → `applySink`（八服务 sink 切换）
* 研究产物：research/ 目录（TLS 兼容面、多设备语义业界模式）
