# Research: 多客户端连接协作语义 + 多 token/审计日志业界模式（浏览器 serve 模式三期改造前置调研）

- **Query**: 终端共享语义业界对照、Web IDE 多光标冲突、会话归属粒度、多 token 管理与有效期、审计日志粒度、断线续传兼容性
- **Scope**: mixed（外部行业模式为主 + 本仓库 serve 通道现状定位）
- **Date**: 2026-10-22
- **核验环境说明**: 本次调研期间 HTTPS 出网受限（沙箱拦截），仅 HTTP 可达源完成核验（OpenBSD man page、GNU screen manual）。GitHub / Apache / Microsoft / Tailscale 等站点内容**凭既有知识 + 官方文档 URL 引用，均标注「未联网核验」**，采纳前建议人工点开链接复核。

## 结论前置

1. **终端多端语义业界三大流派**：① 共享终端（tmux attach / ttyd / Guacamole share link）——多端同屏、输入字节流交织，无冲突消解；② 按连接隔离（wetty / SSH exec / VS Code Remote 终端）——每连接独立会话；③ 持久工作区 + 各自会话（tmux 私有 session / Gitpod 共享 workspace 各自终端）。**本仓库现状（全局注册表 + 全局广播 + WriteInput 无归属校验）在语义上等价于流派①的共享终端**，与 tmux 多客户端 attach 行为一致，并非异常形态；冲突表现 = 字节流交织（业界共享终端完全相同），无需引入 OT/CRDT。
2. **最小合理语义组合（推荐）**：维持全局共享广播（现状不动）+ 前端显式提示「N 个连接已接入」（ Guacamole 共享会话的在线成员可见性）+ 按连接做**只读观察模式**作为可选增强（对齐 tmux `attach -r` / Guacamole sharing profile 的读写分档）；私有会话/独占接管后置。
3. **token 模型（推荐）**：多 token 表（id / 名称 / SHA-256 哈希 / createdAt / lastUsedAt / expiresAt 可空 / revoked），对齐 GitHub PAT + Tailscale auth key 的数据模型；生成时一次性回显明文，落盘只存哈希；存量单 token `data/web_token` 迁移为首条记录；`RotateToken` 语义收窄为「新建 + 撤销旧 token」的编排而非独立原语。撤销粒度收益 = 精准吊销单台设备 + 审计日志按 token 归因。
4. **有效期（推荐）**：惰性失效（认证时判 `expiresAt`/`revoked`，不做后台清理任务）——GitHub / Tailscale 均为此模式，过期条目保留展示由用户手动清理；默认永久 + 创建时可选短命档位（对齐 Tailscale「给临时设备发短命 key」场景）。
5. **审计日志（推荐）**：JSONL append 落盘，记**认证事件（成功/失败/IP/UA/token id/时间）+ token 生命周期操作（创建/撤销/轮换）**，普通 RPC 读操作不记（单用户桌面应用操作级审计噪音大于价值，业界共识是认证 + 敏感管理操作两级）；X-Forwarded-For 仅在显式配置可信反代时启用，直连形态恒以 `RemoteAddr` 为准。
6. **断线续传兼容性：成立，无需改动**。resume 是连接级 `lastSeq` 补发（`server/ws.go` `resumeClient`），认证在 WS 升级前完成、续传游标与身份解耦，与 SSE `Last-Event-ID` / Kafka offset 等业界通用「连接级游标」模式一致，未发现反面模式。唯一结构性牵连：三期若做按连接/token 事件过滤，全局 seq 与过滤后视图出现空洞，resume 补发面需按过滤面重算（当前 MVP 不做过滤则不涉及）。

---

## 1. 终端共享语义业界对照

| 工具 | 多客户端行为 | 读/写区分 | 冲突表现 | 来源状态 |
|---|---|---|---|---|
| tmux attach | **共享**：多客户端 attach 同一 session，同屏同输入 | `attach-session -r` 只读客户端 | 输入字节流直接交织进同一 PTY，shell 逐字节解释，无仲裁 | 未联网核验（tmux 官方 wiki / man page） |
| GNU screen | **共享 + 多用户 ACL**：`multiuser on` + `aclchg` 按用户授读写位 | ACL 粒度（`aclchg user +x/-w`） | 同 tmux，字节流交织 | **已核验**：manual 含 multiuser/aclchg/aclumask（http://www.gnu.org/software/screen/manual/screen.html） |
| ttyd | **共享同一命令进程**：一个 `ttyd <cmd>` 实例的全部浏览器客户端看到同一 PTY；有 `--max-clients` 限流。早期版本默认只读需 `--writable` 开输入，后改为默认可写（版本节点未核实） | 历史 `--writable` / `--read-only` 开关 | 同上，字节流交织 | 未联网核验（https://github.com/tsl0922/ttyd） |
| wetty | **按连接隔离**：每个 WebSocket 连接各自 spawn 独立 PTY/shell | 无 | 各会话互不可见 | 未联网核验（https://github.com/butlerx/wetty） |
| Apache Guacamole | **默认独占 + 显式共享**：connection 默认单活跃用户，活跃会话经 share link + sharing profile 分享给他人；sharing profile 定义只读/可写 | sharing profile 分档 | 共享会话内输入合并到远端桌面，RDP/VNC 层无仲裁 | 未联网核验（https://guacamole.apache.org/doc/gug/） |
| VS Code Remote-Tunnel | **每客户端独立 UI/终端**：共享同一远程主机与扩展宿主，终端面板各窗口独立 | 无（窗口内终端即各自进程） | 文件编辑无协同，保存 LWW（见 §2） | 未联网核验（https://code.visualstudio.com/docs/remote/tunnels） |

**多端同敲键盘的冲突表现**：共享终端流派（tmux/ttyd/Guacamole）均为**字节流交织**——各端输入按到达顺序逐字节写入同一 PTY，无锁定、无合并、无提示；表现为命令字符交错、行编辑混乱，PTY 层不做任何仲裁。这正是本仓库 `WriteInput`（`service/terminal.go:83`）多端同写的现状语义，与业界共享终端行为一致，属「如实复刻 tmux 共享 attach」而非缺陷形态。

**对本仓库的定位**：WorkBench 终端是长任务载体（跑构建/服务），共享语义符合个人多设备场景；业界「混合模式」（默认共享 + 可建私有会话）对应 Guacamole 的 connection + share link 结构与 tmux 的多 session 布局，实现成本显著高于「共享 + 只读观察」，建议后置。

---

## 2. Web IDE 类协作语义与「纯 RPC 转发」架构的最小实现

- **VS Code web / code-server**：不做并发协同（无 OT/CRDT）。code-server 明确面向单用户（FAQ：不支持多用户），多窗口共享同一工作区，同一文件并发编辑 = **last-writer-wins**（后保存覆盖，最多文件过期提示）。来源：https://github.com/coder/code-server/blob/main/docs/FAQ.md （未联网核验）。
- **Gitpod shared workspaces**：共享 workspace URL 后多人可见同一文件系统，但官方文档明确**看不到彼此光标、不支持协同编辑**，各用户有独立终端会话。时效性标注：Gitpod Classic（gitpod.io SaaS）已于 2024 年后转型（品牌/产品线变动），原文档可能下线，此结论凭历史文档记忆（未联网核验）。
- **真协同（多光标/字符级合并）需要 OT/CRDT**：VS Code Live Share（OT，未联网核验，https://learn.microsoft.com/en-us/visualstudio/liveshare/）等，需要文档级实时通道与服务端合并逻辑，属独立协作层产品。

**最小合理实现（针对本仓库「后端无文档状态、纯 RPC 转发」）**：

1. **不做协同合并**——后端没有文档状态可挂 OT/CRDT，且单用户个人工具的并发编辑冲突面极小（自己两台设备同时改同一文件的窗口期极短）。
2. 语义上明确「**多端镜像同一后端状态**」：现状 WSHub 全局广播天然达成（`server/ws.go:82-85` 注释明示该设计意图：「一条事件推给全部已连接客户端……单用户工具不做订阅过滤/单播」）。
3. 前端补**连接可见性**：顶栏/状态栏显示「当前 N 个连接已接入」（WSHub 已有 `ClientCount()`，`server/ws.go:251`，仅暴露即可）；对齐 Guacamole 共享会话的成员可见性。
4. 输入冲突策略 = **LWW/字节交织，如实告知**（UI 文案「多端同时操作时后操作覆盖先操作」），不引入锁定。

---

## 3. 「按连接隔离终端会话归属」的合理粒度

三个候选粒度对照：

| 粒度 | 行为 | 业界先例 | 评估 |
|---|---|---|---|
| 绑定到 WS 连接 | 连接断即回收该端创建的会话 | SSH exec session、Kubernetes `exec`（会话生命周期 = 连接生命周期） | **过激**：浏览器刷新/网络抖动即丢终端，与「长任务载体」定位冲突 |
| 绑定到 token/设备 | 会话归属 token，token 撤销/过期才回收；同 token 多连接均可操作 | Jupyter：kernel 与浏览器会话解耦，空闲回收可配（`cull_idle_timeout`）；VS Code Remote 的 `enablePersistentSessions`（断线重连恢复视图） | 中庸，但引入「会话 → token」归属面与回收任务 |
| 全局共享 + 操作者标记 | 现状语义 + 事件/RPC 带 token id 标记操作者 | tmux 共享 attach（无归属概念）；Guacamole 共享会话内也不区分输入者 | **最小改动，MVP 推荐** |

**orphan PTY 回收的业界模式**：

| 系统 | 断线后会话命运 | 依据状态 |
|---|---|---|
| tmux / screen | **永不回收**（持久会话是核心卖点，直至 detach/kill/重启） | 未联网核验（官方 wiki / manual） |
| ttyd | 最后一个客户端断开后终止命令进程（凭记忆，未核验） | 未联网核验 |
| SSH / kubectl exec | 连接断 → PTY 立即终止 | 常识级（未联网核验） |
| VS Code Remote | 连接断 → 终端进程终止；重连只回放 scrollback，不复活进程 | 未联网核验 |
| Jupyter | kernel 跨浏览器断线存活；空闲剔除超时可配（如 `cull_idle_timeout` 默认关闭） | 未联网核验（https://jupyter-server.readthedocs.io） |
| Guacamole | Web 会话空闲超时（`api-session-timeout`，凭记忆默认 60 分钟） | 未联网核验 |

**归类**：业界两型——**易失工具型**（随连接终止：SSH exec / ttyd / VS Code Remote 终端）与**持久工作区型**（不随连接终止 + 可选空闲回收：tmux/screen/Jupyter）。WorkBench 终端挂载的是用户 shell 长任务，属后者语义。

**对本仓库的建议**：会话**不随 WS 连接断开回收**（保持现状：会话生命周期由 `CloseTerminal`/进程退出驱动，`service/terminal.go` watchDones 机制），归属暂不做；可选后置档位 = 「最后连接断开后 N 分钟闲置回收」（对齐 Jupyter idle culling 形态，且本项目已有惰性定时思想先例）。

---

## 4. 多 token 管理模式与数据模型

### 4.1 业界参照

| 系统 | 多 token | 命名 | 过期 | 撤销 | 备注 | 来源状态 |
|---|---|---|---|---|---|---|
| GitHub PAT（classic / fine-grained） | 是 | note 字段 | classic 可选（7/30/60/90/365/custom/永不）；fine-grained 强制过期 | 是（可单撤） | 列表展示 last used；创建时一次性显示明文；fine-grained 有细粒度权限 | 未联网核验（https://docs.github.com/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens） |
| Tailscale auth keys | 是 | 可选描述 | 创建时选 1–90 天 | 是（提前失效） | reusable / ephemeral / preauthorized / tags 能力位；一次性回显 | 未联网核验（https://tailscale.com/kb/1085/auth-keys） |
| SSH authorized_keys | 每行一钥 | **尾随 comment 字段**（纯人工标签，协议不读） | `expiry-time="timespec"` 选项 | 删行即撤 | options 还有 `from=`（来源限制）、`no-pty`、`command=`；无内建 last-used（审计走 sshd 日志） | **已核验**：http://man.openbsd.org/sshd （AUTHORIZED_KEYS FILE FORMAT 节含 expiry-time） |

### 4.2 推荐数据模型

```jsonc
// data/web_tokens.json（util.SaveJSON 原子写，对齐 data-atomic-write 规则）
[
  {
    "id": "tok-xxxx",        // 短 ID，非 secret，日志/审计引用键
    "name": "macbook-safari", // 设备/用途标签（对齐 GitHub note / SSH comment）
    "tokenHash": "sha256-hex", // SHA-256(token)，校验时恒定时间比较
    "createdAt": "...",
    "lastUsedAt": "...",      // 认证成功时惰性更新（写放大可按分钟节流）
    "expiresAt": "..." | null, // null = 永不过期
    "revoked": false,
    "revokedAt": null
  }
]
```

### 4.3 哈希存储必要性（本地单用户文件场景）

- **泄露面**：`data/` 目录被其他进程/备份/云同步读取即明文裸奔（现状 `server/token.go:55-63` 为明文落盘 + 0600）；哈希后备份/快照泄露不再直接等于凭据泄露。
- **成本**：生成时一次性回显明文（GitHub/Tailscale 均为此交互），对个人工具可接受；校验端把三通道（子协议/cookie/查询参数，`server/ws.go:325-346`）与 HTTP 通道的明文输入做 SHA-256 后比对哈希表即可，恒定时间比较语义保留（对哈希值比较）。
- **标注**：「GitHub 服务端只存 token 哈希」这一说法凭记忆，未核验原文，采纳建议时不必依赖该句（以上成本/收益论证已独立成立）。

### 4.4 「不同设备不同 token」的撤销粒度收益

1. **精准吊销**：某设备遗失/借出后撤销其 token，其余设备不受影响（现状单 token 的 `RotateToken` 热轮换 = 全设备登出，换锁不换门）。
2. **审计归因**：审计日志以 token id 为稳定归因键——比 IP 可靠（家庭内网 DHCP 漂移），比 UA 可靠（同浏览器多设备难分）。这是 §6 审计价值的前提。
3. **分档凭据**：给临时访客/新设备发短命 token（对齐 Tailscale 场景），无需动主凭据。

---

## 5. token 有效期设计

| 维度 | 业界模式 | 采纳建议 |
|---|---|---|
| 永久 vs 固定 vs 可配置 | GitHub classic 可选永不；fine-grained 强制过期；Tailscale 1–90 天固定档 | **默认永久 + 创建时可选档位**（如 24h / 7d / 30d / 永久），覆盖「给临时设备发短命凭据」场景 |
| 到期自动清理 vs 惰性失效 | **惰性失效为主流**：认证时比较 `expiresAt`/`revoked` 即拒；过期条目在列表中保留展示（GitHub 过期 PAT 标 expired，用户手动删） | 惰性失效，不做后台清理任务（个人工具少一个常驻定时器；列表展示时前端标「已过期」即可） |
| 轮转与多 token 关系 | GitHub 无「轮换」原语（新建 + 删旧手工编排）；Tailscale key 到期重新生成 | `RotateToken` 语义收窄为**编排动作**：「新建 token + 撤销旧 token」；存量 `data/web_token` 迁移为首条记录（明文已知可直接算哈希），后续 `RotateToken` 保留兼容调用方 |

**已建连接的既有契约不变**：`SetToken` 热轮换后已建立连接保持有效至断开重连（`server/ws.go:273-284`）；多 token 下天然延续——已建连接不二次校验，撤销只影响新握手。

---

## 6. 审计日志粒度与落盘形态

### 6.1 业界粒度分层

| 层级 | 内容 | 业界实践 |
|---|---|---|
| 认证事件（基线，所有系统都有） | 成功/失败、来源 IP、UA、时间、所用凭据 | GitHub security log、Tailscale audit log（登录/key 创建/轮换等）均为标配（均未联网核验） |
| 敏感管理操作 | token 创建/撤销/轮换、设置变更、TLS 证书更换 | 多用户 SaaS 记入 audit log；单用户工具通常仅日志（slog）不落结构化审计 |
| 全量操作级（每个 RPC） | 每次 API 调用 | 仅多用户合规场景（等保/审计要求）；**单用户桌面应用噪音远大于价值，业界无此先例主流** |

**本仓库建议**：认证事件 + token 生命周期操作两级。普通 RPC（文件树/状态查询/终端输出）不记。理由：单用户场景下全量 RPC 审计的消费方不存在（审计给自己看没有意义），且 serve 通道 RPC 频次高（WS 事件 + 轮询），落盘写放大不可忽视。

### 6.2 落盘形态与轮转

- **JSONL append（推荐）**：`data/audit/audit-YYYYMMDD.jsonl` 追加写，一行一事件；append-only 语义与审计不可篡改诉求天然契合，按日期自然轮转（旧文件可手工归档/删除），无需引入锁与查询层。对比 SQLite 表（项目 assets 域已有 modernc.org/sqlite）：可查询但为审计单开库/表引入写路径耦合，且单用户查询需求弱，**不推荐 MVP 采用**。
- **写入路径**：追加失败（磁盘满等）仅 slog.Warn 降级，不阻塞主链路（审计是旁路，非正确性依赖）。

### 6.3 来源 IP 与 X-Forwarded-For 注意点

1. **XFF 是可伪造头**：直连形态（三期内置 TLS 后的常态）必须以 `RemoteAddr`（TCP 对端）为准，忽略 XFF；仅当显式配置「可信反代地址」时才从 XFF 取值（取「从右往左第一个非可信跳」）。
2. RFC 7239 `Forwarded` 是标准化替代头但普及度低于 XFF（https://www.rfc-editor.org/rfc/rfc7239，未联网核验——本次 301 未取到正文，属常识级标准）。
3. 家庭内网 IP 归因能力弱（DHCP 漂移），**审计归因主键用 token id，IP 仅为辅助字段**。
4. 现状日志已遵循「不落完整 URL 防 `?token=` 泄露」（`server/ws.go:299-301`），审计事件同样禁止落 token 明文。

---

## 7. 断线续传在多 token/多设备下的兼容性

**结论：成立，无需改动。**

- 本仓库 resume 是**连接级**机制：重连后客户端上行 `{"resume":<lastSeq>}`，服务端按全局环形缓冲补发缺口（`server/ws.go:417-436` `resumeClient`）；令牌校验发生在 WS 升级前（`authorized`，`server/ws.go:325`），续传游标与认证身份完全解耦——认证决定「能否连」，lastSeq 决定「从哪补」。多 token 不改变两者中任何一个。
- 业界对照：SSE `Last-Event-ID`、Kafka offset、RabbitMQ stream 等通用「连接级游标 + 服务端有界缓冲」模式中，**游标与认证身份解耦是常态**，未发现「resume 必须绑定 token」的反面模式（未联网核验，属协议设计常识级）。
- **唯一结构性牵连（三期注意）**：若未来做按连接/token 的事件过滤（私有小窗事件不下发给他端），全局 seq 与过滤后 per-connection 视图将出现**有序号空洞**——此时 `resumePlanLocked` 的连续性判定与补发集计算需按「该连接的过滤面」重算，全局环形缓冲需改为按过滤面投影。**当前 MVP 维持全局广播则完全不涉及**；此点应作为「若做隔离」的前置约束记入 spec。

---

## 对本仓库的落地建议清单（推荐语义组合）

1. **多设备语义 MVP**：全局共享广播不动（现状即 tmux 共享 attach 语义）；前端新增连接数展示（消费 `ClientCount` 经 RPC 暴露或事件推送）+ UI 文案明示「多端同时操作，后操作覆盖」；可选增强 = 只读观察连接（对齐 tmux `-r` / Guacamole sharing profile，可作为独立 token 权限位 `readonly`，先例为 SSH `no-pty` 类 option）。
2. **终端会话归属**：不做归属绑定；会话生命周期维持现状（显式关闭/进程退出驱动，不随 WS 连接断开回收）。「最后连接断开后 N 分钟闲置回收」为后置可选项。
3. **token 数据模型**：`data/web_tokens.json`（SaveJSON 原子写）多 token 表，字段见 §4.2；SHA-256 哈希落盘 + 生成时一次性回显；存量 `data/web_token` 首启迁移为首条记录（明文已在本地，直接算哈希）。
4. **认证比对改造**：HTTP 三通道 + WS 三通道的明文 token 输入 → SHA-256 → 遍历未撤销未过期 token 记录比对哈希；`lastUsedAt` 惰性更新（分钟级节流防写放大）；`SetToken` 热轮换改造为「全 token 哈希集合」替换（子协议回显列表同步改为集合）。
5. **有效期**：默认 `expiresAt=null`；创建弹窗提供 24h/7d/30d/永久档；认证时惰性判定，过期/撤销条目列表保留展示（前端标灰 + 手动删除）。
6. **`RotateToken` 兼容**：保留方法签名（前端 wailsjs 三处绑定不动），内部实现改为「新建 token + 撤销旧」编排；已建连接不二次校验的既有契约不变。
7. **审计日志**：`data/audit/audit-YYYYMMDD.jsonl` append-only；记认证成功/失败（IP/UA/token id/结果/时间）+ token 生命周期事件（创建/撤销/轮换/迁移）+ 关键设置变更；普通 RPC 不记；追加失败 slog.Warn 降级；禁落 token 明文与完整 URL。
8. **XFF 策略**：默认恒以 `RemoteAddr` 记录；`AppSettings.WebServe` 增加可选「可信反代网段」配置后才启用 XFF 解析（三期内置 TLS 后直连为常态，默认关闭最稳）。
9. **续传**：零改动；「若未来做事件过滤须重算 resume 补发面」写入 `docs/spec/browser-channel.md` 前置约束。
10. **来源时效提醒**：本文件大量来源未联网核验（见各表标注），brainstorm/实现前建议人工复核 GitHub PAT、Tailscale、Guacamole、code-server 四处官方文档链接。

## 相关内部代码与规范

| 文件 | 关联点 |
|---|---|
| `server/ws.go:82-106` | WSHub 广播语义（注释明示「单用户工具不做订阅过滤/单播」设计意图） |
| `server/ws.go:273-284` | `SetToken` 热轮换（单 token → 多 token 需改哈希集合） |
| `server/ws.go:417-436` | `resumeClient` 连接级续传（与 token 解耦） |
| `server/token.go:14-63` | 现状单 token 明文落盘（0600） |
| `service/terminal.go:19,46-93` | 终端全局注册表 + `WriteInput` 无归属校验 |
| `web_serve.go:169` | `RotateToken` 热轮换入口 |
| `docs/spec/browser-channel.md` §2.4 | WS 续传契约（改动须同步） |
| `docs/spec/data-atomic-write.md` | token 表落盘必须走 `util.SaveJSON` |

## Caveats / Not Found

- 本次调研期间 HTTPS 出网被沙箱拦截（curl exit 35/28），GitHub/Apache/Microsoft/Tailscale/coder 文档**均未联网核验**，仅凭既有知识 + 官方 URL 引用；已核验来源仅两处（OpenSSH sshd(8) `expiry-time`、GNU screen multiuser/aclchg manual）。
- ttyd「最后客户端断开即杀命令进程」「`--writable` 默认态版本节点」、Guacamole `api-session-timeout` 默认 60 分钟、GitHub「服务端只存哈希」等细节凭记忆，实现不依赖这些句子的请直接以上表标注为准。
- Gitpod 产品线 2024 年后已转型，shared workspaces 原文档可能已下线；其行为结论仅作历史参照。
- 未调研移动端浏览器适配（PRD Out of Scope 明确排除）。
