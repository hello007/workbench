# Research: Go HTTP/WebSocket 服务内置 TLS 的实现方案与兼容面

- **Query**: 在 `webServeManager`（自持 `net.Listener` + `http.Server` + gorilla/websocket WSHub）上内置 TLS：正确姿势、自签引导、用户证书、浏览器信任、设置链路兼容面
- **Scope**: mixed（本仓库源码 + 本机 Go 1.26 标准库源码 + 业界公开实践）
- **Date**: 2026-10-22
- **环境基线**: Go 1.26.10（本机 toolchain 源码为证据源）、gorilla/websocket v1.5.3（go.sum 在案）

---

## 0. 结论前置

| # | 问题 | 结论 |
|---|---|---|
| 1 | TLS 接入姿势 | **推荐 `tls.NewListener(ln, tlsConfig)` + `srv.Serve(tlsLn)`**，自建 `tls.Config`；`ServeTLS` 内部本质相同但会强制 HTTP/2 初始化，与 gorilla（仅 HTTP/1.1）有兼容面张力。`listenAddr()` 与 `srv.Close()` 语义在 TLS 下**均不变** |
| 2 | WebSocket over TLS | gorilla upgrader **零额外要求**（Hijack 对 `tls.Conn` 透明）；`?token=` 在 wss 下泄漏面**收窄**（链路加密），本仓库「不落日志」约定保持即可 |
| 3 | 自签引导 | `crypto/x509.CreateCertificate` 程序内生成；**SAN 必填**（CN 自 Chrome 58 起不参与匹配）；首次生成**持久化** `data/`（对齐 token 先例），禁每次启动重生成 |
| 4 | 用户证书 | **PEM cert+key 两文件路径**（`tls.LoadX509KeyPair`）为 MVP 形态；PKCS12 与 passphrase 加密 key **均不建议 MVP 支持**；热加载不需要，重启生效 |
| 5 | 浏览器信任 | 非 HSTS 下 Chrome/Edge 可「继续前往」；**绝不发送 HSTS**（发了即禁止绕过）；进阶项 = 引导导入 Windows 用户根存储 |
| 6 | HSTS / MinVersion / cipher | HSTS 不发；MinVersion **保持默认**（Go 服务端默认 TLS 1.2）；cipher 不配置（Go 默认）；**补 `TLSHandshakeTimeout`**（默认 0 = 不限） |
| 7 | 设置链路 | 证书加载必须发生在 `startLocked` **字段赋值之前**（失败即 return，无半死态）；`Start` 幂等比较键须从 `listen` 扩为「listen + TLS 配置」 |

---

## 1. `ServeTLS` vs `tls.NewListener` + `Serve` 的差异与坑

### 1.1 结论

推荐 **`tls.NewListener(ln, tlsConfig)` + `srv.Serve(tlsLn)`**，理由有三，均有 Go 1.26 标准库源码佐证：

1. **`ServeTLS` 内部本质就是这两步**（无性能/语义差异）：`go1.26 src/net/http/server.go:3502-3524` —— `setupHTTP2_ServeTLS()` → `cloneTLSConfig` → 同步 `tls.LoadX509KeyPair(certFile, keyFile)` → `tls.NewListener(l, config)` → `s.Serve(tlsListener)`。区别仅在于它替你做证书文件加载与 HTTP/2 初始化。
2. **`ServeTLS` 默认开启 HTTP/2 ALPN**：`server.go:3526-3548`（`protocols()`），除非 `TLSNextProto` 为非 nil 无 `h2` 键、或显式 `Server.Protocols` 关闭。gorilla/websocket 只支持 HTTP/1.1 升级（其 `Upgrade` 要求 `r.ProtoMajor == 1`），h2 在 ALPN 中会引入「WebSocket over HTTP/2」行为分叉面（见 1.3）。
3. **自建 `tls.Config` 才能把证书加载提前到 `startLocked` 字段赋值之前**（问题 7 的无半死态要求），并精确控制 `NextProtos` / `MinVersion`。

### 1.2 `listenAddr()` 与 `stopLocked` 语义在 TLS 下均成立

- **`listenAddr()`**：管理器 `m.ln` 继续保存**原始 TCP listener**（`Addr()` 即内核分配地址）；传给 `Serve` 的是 `tls.NewListener` 包装体（其 `Addr()` 直接委托内层，两者一致）。`web_serve.go:189-196` 零改动。
- **`stopLocked` 的 `srv.Close()`**：`server.go:3111-3116` 明确「立即关闭全部 listener 与 StateNew/StateActive/StateIdle 连接；不关 hijacked 连接（如 WebSocket）」。TLS 下接受连接为 `*tls.Conn`（`tls.listener.Accept` → `tls.Server(c, config)`），作为普通 `net.Conn` 被跟踪与关闭，语义与纯 TCP **完全一致**；hijacked 的 WS 仍由 `hub.Close()` 显式关闭（`web_serve.go:117-134` 注释已覆盖此分工，TLS 不改变它）。
- **`r.TLS` 自动生效**：`server.go:1934` —— 服务端检测到 `c.rwc` 为 `*tls.Conn` 即走 `HandshakeContext` 并填充 `r.TLS`。因此 `server/web.go:218` 的 `Secure: r.TLS != nil`（wb_token cookie）在 TLS 下**自动切到 Secure cookie，零改动**。附带收益：当前经外部反代时 `r.TLS` 恒为 nil 的盲区被消除（内置 TLS 后该字段重新可信）。
- **`Serve` 退出错误链不变**：`srv.Close()` → `Serve` 返回 `ErrServerClosed`（`server.go:3158-3159`），`done` channel 代际语义（`web_serve.go:146-165`）零改动。

### 1.3 HTTP/2 的坑（为何必须钉死 http/1.1）

- Go 1.26 bundled h2 **默认禁用 extended CONNECT**（WebSocket over HTTP/2，RFC 8441）：`src/net/http/h2_bundle.go:3555-3568`，注释原文「Enabling extended CONNECT by causes browsers to attempt to use WebSockets-over-HTTP/2... Disable extended CONNECT by default for now」（issue #71128）。默认禁用下浏览器应回退到新建 HTTP/1.1 连接完成 wss 握手。
- 但该回退行为属浏览器实现细节（无 RFC 级契约），且若未来 Go 翻转默认值（GODEBUG `http2xconnect=1` 可开启），gorilla 将在 h2 连接上收到升级请求直接失败。**推荐显式在 `tls.Config.NextProtos` 只放 `["http/1.1"]`**，把行为从「依赖浏览器回退」变为「ALPN 层确定性协商 http/1.1」。注意：`tls.NewListener` 路径下 ALPN 完全由我们传入的 config 决定，`adjustNextProtos`（`server.go:3553`）只在 `ServeTLS` 内部对它自建的 config 生效——即 `ServeTLS` 即便传了自建 config 也会被追加协议，这正是选 `tls.NewListener` 的直接原因。
- 若未来想复用 `ServeTLS`，Go 1.24+ 可用 `Server.Protocols`（`src/net/http/http.go:30,44,50`，`SetHTTP1(true)`/`SetHTTP2(false)`）关闭 h2；但本仓库 `startLocked` 手持 ln 的形态下 `tls.NewListener` 更贴合（见落地清单）。

### 1.4 自签证书的内存形态

`ServeTLS` 的 `configHasCert` 分支（`server.go:3512-3520`）表明：`tls.Config.Certificates` 已填充时可不传文件路径。程序内自签生成的证书（内存 `tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}`）两条路径均可承载；走 `tls.NewListener` 时同样直接放 `Certificates`。

---

## 2. WebSocket over TLS：gorilla 侧无额外要求

### 2.1 升级透明性

gorilla `Upgrader.Upgrade` 走 `http.Hijacker` 接口；`http.response.Hijack()` 把底层 `net.Conn`（此处为 `*tls.Conn`）交出，后续 WS 帧读写经 TLS 层透明加解密。upgrader 无任何 TLS 特有配置项；`server/ws.go:129-137` 的 `Subprotocols`（token 回显）与 CheckOrigin 默认同源校验在 wss 下语义不变。`server/ws.go` **预计零改动**。

一个前序条件由 1.3 保障：升级请求必须落在 HTTP/1.1 连接上（ALPN 钉死 http/1.1），否则 gorilla 在 h2 请求上必然失败。

### 2.2 `?token=` 查询参数在 wss 下的泄漏面

对比纯 HTTP，wss 下泄漏面**收窄**：

| 通道 | 纯 HTTP | wss（TLS） |
|---|---|---|
| 链路窃听 | 明文可见 | 加密，不可见 |
| 中间代理/隧道日志 | 可见 | 摆脱外部反代后该面消失 |
| 服务端日志 | 本仓库已规避（`ws.go:299-301` 只记 `RemoteAddr` 不记 URL） | 同左，约定保持 |
| 浏览器 Referrer | WS 握手请求的 Referer 头携带的是**发起页面** URL 而非 ws URL，token 不经此泄漏 | 同左 |

结论：`?token=` 作为「非浏览器客户端与手工验证便利」通道（`ws.go:290-296`）在 TLS 下可保留，风险定性从「中等（明文链路）」降为「低（仅服务端日志面，已规避）」。无需改造。

---

## 3. 自签证书引导（self-signed bootstrap）

### 3.1 生成方式

`crypto/x509.CreateCertificate`（`src/crypto/x509/x509.go:1679`）+ 自签（template == parent）。要点：

- **SerialNumber 可省略**：Go 1.26 起 nil 时按 RFC 5280 §4.1.2.2 自动生成 20 字节正序号（`x509.go:1686-1700`）。
- **签名算法**：ECDSA P-256（`ecdsa.GenerateKey`，`x509.ECDSAWithSHA256`）为兼容面最稳的默认（全部现代浏览器支持）；Ed25519 亦可（`x509.PureEd25519`，Go 原生、更小更快）。**建议 ECDSA P-256**——对潜在的非浏览器消费端（curl 老版本、各种 GUI 客户端）兼容面更宽。
- **SAN 必填**：`template.DNSNames` + `template.IPAddresses`。现代浏览器对服务器证书的域名匹配**只看 SAN**：
  - Chrome 自 58（2017-03）起移除 CommonName 匹配（chromestatus feature 4981023330648064）；
  - Windows SChannel/Edge 同样要求 SAN（微软 KB 文档「Certification Authority Guidance」系列，自 Server 2008 起强烈要求，Vista+ 客户端对无 SAN 证书拒绝）。
  - Firefox 对无 SAN 证书仍有 CN 回退的传闻**未验证**，不可依赖，统一按「SAN 必填」处理。
- **SAN 取值建议**：`127.0.0.1`、`::1`、`localhost` + 本机全部非回环 IPv4（`web_serve.go:297-323` 的 `localIPv4Addresses()` 现成复用）+ 主机名（`os.Hostname()`）。一次生成覆盖全部候选访问地址，绑定地址变化无需重生成。
- **BasicConstraints/KeyUsage**：`ExtKeyUsageServerAuth`、`KeyUsageDigitalSignature|KeyUsageKeyEncipherment`、`template.IsCA = true`（自签叶证书既是根也是叶，`IsCA=true` 便于用户后续手动导入信任——部分导入流程要求 CA 位；mkcert 生成的本地 CA 即如此）。此点为实践惯例，非 RFC 强制。

### 3.2 有效期惯例

- **mkcert**：叶子证书 825 天（mkcert 源码 `cert.go` 常量；825 天是 Apple 对公开信任 TLS 证书的历史上限，被广泛用作自签惯例试金石）。GitHub：https://github.com/FilloSottile/mkcert（本机网络未能拉取源码核验行号，天数置信度高、属广为人知的实现常量）。
- **Caddy** 内部 CA 签发短效证书（12 小时级）+ 自动续期——需要常驻证书管理器，不适合本工具的「生成一次用到底」形态。
- **Traefik** 默认自签证书有效期 1 年（源码常量，未核验行号，置信度中）。
- **建议**：825 天 ~ 3 年均可接受；个人工具取长（如 825 天）减少「证书过期复发警告」的运维噪声。**标注**：自签证书不受公开信任 CA 的 398 天上限约束（该规则仅约束公共 CA 链），但取 825 天无害且对齐业界先例。

### 3.3 落盘位置与权限

对齐本仓库两个先例：

- **`server/token.go`（`web_token`）**：独立文件、`0600`、`LoadOrCreateToken` 首次生成持久化。私钥文件应完全复刻此形态（`0600`；Windows 上由用户目录 ACL 兜底）。
- **`util.SaveJSON` 原子写**（CLAUDE.md 关键规则）：PEM 文件是纯文本非 JSON，不走 `SaveJSON`，但**建议**采用同构的「同目录 temp 写 + rename 替换」防半截文件（私钥半截 = 下次启动解析失败）；若嫌过度，普通 `os.WriteFile(…, 0o600)`（对齐 `SaveToken`）即可接受——文件只在首次生成时写一次，后续只读。

文件名建议：`data/web_tls_cert.pem`（0644）与 `data/web_tls_key.pem`（0600），与 `data/web_token` 命名风格一致。

### 3.4 首次生成 vs 每次启动重生成

**必须首次生成后持久化**。每次启动重生成意味着：

1. 用户手动信任/导入的证书在下次启动后失效，信任动作反复做（致命摩擦）；
2. 浏览器「继续前往」例外按证书指纹记忆，重生成后例外失效。

`LoadOrCreateToken`（`token.go:32-51`）是现成模板：存在即读、空/坏重建、非 NotExist 错误 fail-fast。证书对可做同构 `LoadOrCreateTLSCertPair(path)`；**证书与私钥损坏（解析失败）时的策略需决策**：token 对「空文件走重生成」，证书对建议**报错而非静默重生成**——静默重生成会无声作废用户已建立的信任，错误应上抛由调用方提示（与 3.1 token 注释的「不静默重建避免掩盖部署问题」语义对齐）。

---

## 4. 用户自带证书形态

### 4.1 PEM cert+key 两文件路径（推荐 MVP）

`tls.LoadX509KeyPair(certFile, keyFile)` 是 Go 生态标准形态（`ServeTLS`/`ListenAndServeTLS` 内部同款），配置面就是两个路径字符串。CA 签发链证书拼接进 cert 文件（certFile 应为「服务器证书 + 中间证书」串接，`server.go:3496-3498` 文档原文）。

### 4.2 PKCS12（不建议）

- Go 标准库**不支持** PKCS12 解析；`golang.org/x/crypto/pkcs12` 处于维护模式（其文档自述 legacy），现代替代为第三方模块 `software.ssl.golang.org/pkcs12`。
- 引入新依赖违背本仓库最小依赖面原则（CLAUDE.md 规约记忆）；目标用户（开发者本机/内网）持有 PEM 形态证书无障碍。**明确标注**：如未来有用户反馈强需求再评估。

### 4.3 加密私钥（passphrase）

`tls.LoadX509KeyPair` **不支持**带 passphrase 的加密 PEM：`x509.DecryptPEMBlock` 已标记 deprecated（legacy RC2/3DES 形态），现代 PKCS#8 加密私钥（`EncryptedPrivateKeyInfo`）标准库同样无解析支持，需第三方库。**建议不支持**：私钥文件已有 `0600` + 用户目录 ACL 保护，威胁模型（单用户个人工具、本机盘）下加密私钥收益边际；文档写明「key 须为未加密 PEM，否则启动报错」（`tls.X509KeyPair` 对加密块会返回明确错误，fail-fast 天然成立）。

### 4.4 证书变更热加载（不需要）

业界多数非证书管理器形态的应用均为**重启生效**（nginx reload、Traefik 需配 dynamic config、常规 Go 应用重启）。本仓库已有现成的平滑重启链：设置页改路径 → `SetWebServeConfig` → `Start`（地址变了即停旧起新）→ 新 `tls.Config` 重新加载文件。**MVP 不做文件 watcher 热加载**；语义上「改证书路径 = 重启服务」与「改绑定地址 = 重启服务」对称，用户心智一致。

---

## 5. 浏览器信任自签证书的现实体验

### 5.1 警告形态与绕过

- Chrome/Edge：`NET::ERR_CERT_AUTHORITY_INVALID` 全页拦截，**非 HSTS** 域名可「高级 → 继续前往（不安全）」。该例外按 host（+端口）记忆，此后同源页面与 **wss 握手沿用例外**（业界通行实测：OctoPrint、VS Code server 等自署 + WS 应用均依赖此行为）。**标注**：wss 无法在 WebSocket API 内单独绕过证书错误（浏览器不给入口），依赖的是 https 页面例外的外溢——该行为无正式契约，**列入真机冒烟验证项**（Chrome + Edge 各验一次）。
- **HSTS 与绕过互斥（关键）**：站点发送 HSTS 后 Chrome/Edge **禁止**「继续前往」（MDN Strict-Transport-Security 文档明确「警告不可绕过」；https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Strict-Transport-Security）。本仓库自控响应头 → **不发送 HSTS 即可保留用户绕过能力**（见 §6）。

### 5.2 降低摩擦的可选方案（按成本排序）

1. **文档引导「继续前往」**（零开发）：MVP 之外兜底说明。
2. **生成时引导导入信任**：生成证书后，设置页提供「导出 .cer / 打开所在目录」，用户双击安装到「受信任的根证书颁发机构 → 当前用户」。程序化导入（`certutil -user -addstore Root cert.cer` 或 CryptoAPI `CertAddCertificateContextToStore`）对 CurrentUser Root 无需管理员权限，但**会触发 Windows 系统确认弹窗**（mkcert -install 在 Windows 上的既有先例：https://github.com/FilloSottile/mkcert）。**标注**：certutil -user 是否静默未核验，按「有确认弹窗」设计交互。
3. **mkcert 式本地 CA**（生成 CA + 签叶证书，用户只信任 CA 一次，后续证书轮换免导入）：适合证书会轮换的形态；本工具「一证到底」下相对方案 2 无增益，不建议 MVP。
- Chrome 对 Windows 证书存储的信任：Chrome Root Store 上线后**继续信任** OS 存储中用户/企业管理员添加的本地 CA（Chrome Root Store 文档「locally-managed certificates」；https://chromium.googlesource.com/chromium/src/+/main/net/data/ssl/chrome_root_store/faq.md）。**标注**：置信度高，属公开承诺的企业兼容条款。

### 5.3 HTTP→HTTPS 跳转与 mixed content

- **单端口无法同时服务 http 与 https**（TLS 握手与明文 HTTP 判别需不同 listener）。可选：额外开一个 http 端口做 301 跳转，或干脆不做跳转（访问 `http://` 只会收到 Go 的 400 「Client sent an HTTP request to an HTTPS server.」，`server.go:1944-1949`——该兜底响应内置且可读，个人工具场景可接受）。**建议 MVP 不做跳转 listener**，健康检查/提示走文档。
- **mixed content**：前端已满足安全前提——`frontend/src/transport/index.js:45` 按 `location.protocol === 'https:' ? 'wss:' : 'ws:'` 拼 WS URL（install.spec.js:67 有断言），其余资源同源相对路径。**https 页面下前端零改动自动 wss**，证实 PRD Assumption「前端 transport 无需感知 TLS」。
- `webAccessUrls`（`web_serve.go:279-293`）与 `GetWebServeConfig.AccessUrls` 硬拼 `http://` 前缀，TLS 开启时须改为 `https://`（落地清单第 8 条）。

---

## 6. HSTS / MinVersion / cipher

| 项 | 建议 | 证据 |
|---|---|---|
| HSTS | **不发送**（加不发 `Strict-Transport-Security` 头即可） | §5.2：发了即禁止证书警告绕过，与自签体验直接冲突；且个人工具无降级攻击威胁模型 |
| MinVersion | **不设置（保持默认）** | `crypto/tls/common.go:784-791`：「By default, TLS 1.2 is currently used as the minimum」（服务端，Go 1.18 起的默认；`common.go:1229` 实现佐证）。显式写 `tls.VersionTLS12` 仅具文档价值，可选 |
| TLS 1.3-only | **否** | MaxVersion 默认即最新（1.3，`common.go:793-795`）；1.3 与 1.2 并存对客户端零成本，1.3-only 只会排除老客户端，无收益 |
| CipherSuites | **不配置** | Go 默认套件随版本更新维护浏览器兼容面（`tls` 包文档立场：默认即合理；CipherSuites 字段仅作用于 TLS 1.0-1.2，1.3 套件固定不可配） |
| TLSHandshakeTimeout | **显式设置（建议 10s）** | `server.go:1935-1940`：握手 deadline 只取 `Server.TLSHandshakeTimeout`，**默认 0 = 不限**——慢握手（slowloris）连接可无限占位。现 `http.Server{ReadHeaderTimeout: 10s}`（`web_serve.go:81-84`）不覆盖握手期 |

---

## 7. 设置链路与平滑重启（`SetWebServeConfig` → Start/Stop）的 TLS 特有坑

### 7.1 证书加载失败不得把服务打成半死态

`startLocked` 现序（`web_serve.go:69-97`）：`assembleWebServe` → `net.Listen` → `applySink` → 赋值五字段 → 起 goroutine。TLS 插入必须保证**任何失败发生在字段赋值之前**：

- 证书加载（`tls.LoadX509KeyPair` 或读自签文件）是**纯文件读 + 内存解析**，应放在 `net.Listen` 之前（甚至 `assembleWebServe` 之前）——失败直接 `return err`，管理器五字段未动、旧服务未被触碰，调用方按既有分流（桌面 `slog.Warn` 降级 / `--serve` fatal / 浏览器 RPC 报错回传）。
- **顺序红线**：先停旧再加载证书 = 改证书路径填错时旧服务已停新服务起不来（半死态）。正确序 = 加载成功后才允许停旧。
- TLS 配置作为 `Start` 入参的一部分（见 7.2）后，`Start` 内部「先停旧再起新」的窗口里新起失败（端口占用/证书坏）都会留下「服务全停」态——这是**既有行为**（改地址填错端口同样如此），TLS 只是新增一种失败源；失败错误同步上抛给前端 toast 即可，不需额外机制。

### 7.2 `Start` 幂等键须扩容

`Start(listen)` 的 no-op 判定是 `m.listen == listen`（`web_serve.go:58-64`）。TLS 配置（证书路径/开关/自签）不属于 listen，**同地址下开/关 TLS 或换证书路径不会触发重启**。须把幂等键扩为 `(listen, tlsFingerprint)`——最简单是 `m.listen` 存拼接键（如 `listen + "\x00" + certPath + "\x00" + keyPath + "\x00" + mode`）或引入小型 config struct 作参数。**这是必改点，漏改则 TLS 开关在设置页表现为「保存了但不生效」。**

### 7.3 配置面与默认值

`model/settings.go` 的 `WebServeSettings`（指针形态区分段缺失/显式取值，`settings.go:8-12`）追加 TLS 字段；`EnsureWebServeDefaults`（`settings.go:158-166`）补默认。字段建议：

```go
type WebServeSettings struct {
    Enabled     bool   `json:"enabled"`
    BindAddress string `json:"bindAddress"`
    TLS         string `json:"tls"`         // "", "auto", "custom"（空 = 关闭，向后兼容）
    CertFile    string `json:"certFile"`    // tls == "custom" 时必填
    KeyFile     string `json:"keyFile"`     // tls == "custom" 时必填
}
```

- `"auto"`（自签引导）：首次启用时生成并持久化 `data/web_tls_cert.pem` / `data/web_tls_key.pem`（§3）。
- `"custom"`（用户证书）：两路径必填，缺失/解析失败在 `Start` 前置校验报错。
- 向后兼容：老配置 `tls` 为空串 = 行为不变（纯 HTTP），`EnsureWebServeDefaults` 不补 TLS 默认值（TLS 是显式决策项，不默认开启）。

### 7.4 `--serve` 与桌面共用

两者共用 `webServeManager`（`web_serve.go:16-26`），TLS 逻辑收在 `startLocked` 内则两条链路自动同构；`--listen` 命令行覆盖只影响地址（`main.go:48-55`），TLS 目前无命令行开关需求（settings 单一来源即可，避免 `--listen` 与 settings 优先级问题在 TLS 上重演）。

---

## 8. 对本仓库的落地建议清单

1. **接入形态**（`web_serve.go` `startLocked`）：
   ```go
   // 顺序：证书解析 → net.Listen → applySink → 赋字段 → goroutine
   tlsCfg, err := buildWebServeTLSConfig(tlsCfgIn) // nil = 纯 HTTP；自签/用户证书均在内部收敛
   if err != nil {
       return fmt.Errorf("加载 TLS 证书失败: %w", err) // 字段未动，无半死态
   }
   tcpLn, err := net.Listen("tcp", listen)
   if err != nil { /* 既有错误路径 */ }
   serveLn := net.Listener(tcpLn)
   if tlsCfg != nil {
       tlsCfg.NextProtos = []string{"http/1.1"}          // 钉死 http/1.1（§1.3）
       serveLn = tls.NewListener(tcpLn, tlsCfg)
   }
   m.srv, m.ln, m.hub, m.handler, m.listen = srv, tcpLn, hub, handler, listen // m.ln 恒存原始 TCP listener
   go func() { done <- srv.Serve(serveLn) }()            // Serve 收包装体
   ```
2. **`http.Server` 补 `TLSHandshakeTimeout: 10 * time.Second`**（§6；ReadHeaderTimeout 不覆盖握手期）。
3. **`Start` 幂等键扩为「listen + TLS 配置」**（§7.2，必改）。
4. **自签生成**：`server/tls_cert.go`（新文件）`LoadOrCreateTLSCertPair`（对齐 `token.go` 形态）+ `generateSelfSignedCert`（ECDSA P-256、SAN = 127.0.0.1/::1/localhost + `localIPv4Addresses()` + hostname、825 天、SerialNumber 省略走自动）；证书 0644 / 私钥 0600；损坏 fail-fast 不静默重生成（§3.3/3.4）。
5. **用户证书**：`tls.LoadX509KeyPair`，路径校验 + IsAbs/存在性前置检查；PKCS12 与 passphrase 明确不支持并在设置页文案说明（§4）。
6. **不发 HSTS**，不配置 MinVersion/CipherSuites（§6）。
7. **设置面**：`WebServeSettings` 加 `tls/certFile/keyFile`（§7.3）+ `EnsureWebServeDefaults` 不补 TLS 默认 + `model/` 具名类型若作 Wails 绑定参数需手动补 models.ts 别名（CLAUDE.md 跨层契约规则——建议字段用 string 基本类型规避）；`wailsjs/` 三处同步。
8. **`webAccessUrls` scheme**：TLS 开启时输出 `https://`（入参需感知 TLS 配置，`GetWebServeConfig` 一并透出 tls 状态供设置页展示）。
9. **前端零改动确认**：`transport/index.js:45` 已按 `location.protocol` 自适配 wss（§5.3）；`?token=` 通道不改（§2.2）。
10. **测试**：`httptest.NewTLSServer` 不适用（自持 listener 形态），建议 `tls.NewListener` + `net.Listen("tcp","127.0.0.1:0")` 起真 TLS 服务、客户端用 `InsecureSkipVerify` 测 `/healthz` + WS 升级 + `r.TLS`→Secure cookie 断言；幂等键扩容回归（同址开/关 TLS 触发重启）；证书损坏 fail-fast 用例；`--serve` Wait 代际在 TLS 下的既有回归不破坏（`web_serve_test.go` 既有形态平移）。
11. **真机冒烟项**：Chrome/Edge 自签警告「继续前往」后同源 wss 是否连通（§5.1 无正式契约）；Windows 双击 .cer 导入用户根存储后警告消除；`http://` 访问 https 端口收到 400 提示文案的可读性。

## Caveats / Not Found

- §5.1「https 例外外溢到同源 wss」为业界通行实测结论，无 RFC/浏览器正式契约，已列入真机冒烟项。
- mkcert 825 天 / Traefik 1 年等第三方实现常量来自公开源码的既有知识，本机网络未能直连 GitHub 核验行号（chromestatus 为 SPA 无法抓取正文；raw.githubusercontent 不通），置信度高但未逐行复核。
- Firefox 对无 SAN 证书是否仍有 CN 回退未验证（结论不受影响：按 SAN 必填设计）。
- Go 未来若把 extended CONNECT 默认改为开启（GODEBUG `http2xconnect` 现为关闭向），ALPN 钉死 http/1.1 的建议依然成立且免疫该翻转。

## 相关内部文件

| 文件 | 说明 |
|---|---|
| `web_serve.go` | `startLocked`/`stopLocked`/`Start`/`Wait`/`listenAddr`/`webAccessUrls`/`localIPv4Addresses` |
| `server/ws.go` | WSHub upgrader（Subprotocols token、CheckOrigin 同源）、`?token=` 通道 |
| `server/web.go` | `requireToken`/`issueSessionCookie`（`Secure: r.TLS != nil` 先例，web.go:218） |
| `server/token.go` | `LoadOrCreateToken` 0600 持久化先例（自签证书落盘模板） |
| `app_webserve.go` | `SetWebServeConfig` 平滑重启链（§7.1 顺序红线） |
| `model/settings.go` | `WebServeSettings`/`EnsureWebServeDefaults`（§7.3 扩展点） |
| `frontend/src/transport/index.js` | WS URL scheme 自适配（index.js:45，前端零改动依据） |
