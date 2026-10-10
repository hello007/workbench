# Research: 进程管理——进程枚举 / 端口→PID 映射 / 优雅停止方案

- **Query**: 为「工具箱·进程管理」选择跨平台（Windows 主力 + Linux 次要）的进程枚举方案，以及「优雅停止」与「强制强杀」的语义实现
- **Scope**: internal（本项目既有能力与范式） + external（Windows/Linux 平台机制、同类工具 UX 约定） + 本机实证
- **Date**: 2026-10-09
- **任务目录**: `.trellis/tasks/10-09-toolbox-process-manager/`

---

## TL;DR（核心结论）

| 问题 | 结论 |
|---|---|
| **进程枚举 + 端口映射** | **推荐混合方案**：Windows 用 `netstat -ano`（一次调用同时拿 PID + 端口 + 状态，实测 0.075s）+ `tasklist /FO CSV /NH`（补进程名/内存，实测 ~1.0s/494 行）；Linux 解析 `/proc/net/tcp{,6}` + `/proc/*/cmdline`。**零新增依赖**。备选：若后续需要命令行/用户/CPU 等更多字段且不愿维护解析器，再评估 `gopsutil/v4`（代价是新依赖树 + 安全扫描面扩大） |
| **优雅停止（Windows）** | **做不到通用优雅**。~~Windows 没有 SIGTERM 等价物：`taskkill`（不带 `/F`）只对**有顶层窗口的 GUI 进程**生效（窗口关闭语义），对无窗口/后台进程直接失败~~ → **此结论已被实测推翻，见第 3 节开头「实测更正」**：请求能否生效取决于**目标进程是否响应**（实测 `ping` 响应退出、`cmd /k` 有窗口仍拒绝），无法按类型预判。工程结论不变：把「停止」定义为**尽力优雅 + 超时降级**（发关闭请求 → 轮询 5s → 仍存活则转强杀），且 **SUCCESS ≠ 已退出**，必须轮询实测 |
| **同类工具 UX** | 普遍列：进程名 / PID / 端口 / 命令行 / 内存 / 用户；默认按单列排序 + 关键字过滤；结束进程**一律二次确认**；**杀进程树是二级选项而非默认**（Windows 任务管理器默认只结束单进程） |
| **自我保护** | **PID 硬禁**（`os.Getpid()` 自身 + 父进程链）+ **系统关键进程软禁**（二次确认 + 警告文案）。进程名 `workbench.exe` 只作提示不匹配保护（E2E 场景 exe 改名 `workbench-e2e.exe`，见 `docs/spec/e2e-testing.md:108`） |

---

## 1. 内部现状：已存在的能力与范式

### 1.1 已存在的进程相关代码

| 文件 | 内容 | 对本次的可用性 |
|---|---|---|
| `util/kill_windows.go:13` | `KillProcessTree(pid int)`：Windows `taskkill /T /F /PID`，`HideCommandWindow` 隐藏窗口 | **强杀可直接复用**；但无优雅语义、无枚举能力 |
| `util/kill_other.go:12` | `KillProcessTree(pid int)`：非 Windows 先 `syscall.Kill(-pid, SIGKILL)`（**进程组**语义），失败再 `syscall.Kill(pid, SIGKILL)` | 非 Windows 侧是 SIGKILL；`-pid` 仅当目标为进程组组长时才成立，对外部进程（如 IDE 启动的 java）通常落到单进程兜底 |
| `service/obsidian_windows.go:58` | `isObsidianRunning()`：`tasklist /FO CSV /NH` 全量枚举 + 字符串过滤 | **进程枚举的既有先例**（同包同范式），且注释已记录「`/FI` 在 cmd 嵌套下易 exit status 1，故全量输出再过滤」 |
| `util/exec_windows.go:10` | `HideCommandWindow(cmd)` 设 `SysProcAttr.HideWindow` | 调 `netstat`/`tasklist` 时须调用，避免闪黑窗 |
| `app_services.go:23` | `AppServices` struct + `NewAppServices`（第 66 行） | 新增 service 须改这 **2 处** |
| `model/app_error.go:50` | 错误码常量表（E_GIT_* / E_SEARCH_* / E_DIFF_* / E_CHAT_* / E_RPC_*） | 新增错误码须同步此处 + `frontend/src/utils/error.js` |

### 1.2 可复用的范式参照

| 范式 | 参照点 |
|---|---|
| 命令输出解析 + 错误识别单点 | `util/git_lock.go:34` `parseGitLockError(stderr)`：把 stderr 形态判定收敛到一个函数，返回 `*model.AppError` |
| 超时收口 + 取消 | `service/content_search.go:196` `searchWithRipgrep` / `mergeTimeoutGroups`（CLAUDE.md「内容搜索超时收口」条目）：**先判 `ctx.Err()` 再判定退出码**，超时不得误判为「无结果」 |
| 平台分叉 | 编译期字段独有走 `_windows.go` / `_other.go` 双文件（CLAUDE.md `linux-platform.md` 条目），测试平台断言拆 `_windows_test.go` / `_linux_test.go` |
| 前端工具箱交互 | `frontend/src/components/ToolboxPanel.vue:189` `tools` 数组 + `el-dialog`（`append-to-body`，480px）；进程列表内容量远大于现有表单，需按 CLAUDE.md「弹窗 `min(像素,视口比例)` 响应式尺寸分档」上调到 900 档 |

### 1.3 本机实证：端口占用现状

```
> netstat -ano | grep 36115
  TCP    127.0.0.1:36115        0.0.0.0:0              LISTENING       44336
```

即 PRD 描述的场景真实存在：36115 被某个 PID 占用（本机实测为 44336）。这正是「进程管理」要解决的痛点；也意味着**枚举结果里一定会包含本应用自身**，自我保护必须前置设计。

---

## 2. 问题一：进程枚举 + 端口→PID 映射的方案对比

### 2.1 方案 A：纯 Go 标准库 + 平台命令/文件系统解析（零新增依赖）

#### Windows 侧

**A1. `netstat -ano`（端口→PID 主通道）——本机实测**

```
> netstat -ano | head -6

Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       1912
  TCP    0.0.0.0:445            0.0.0.0:0              LISTENING       4

> netstat -ano | wc -l        → 421 行
> time (netstat -ano >/dev/null) → real 0m0.075s
> netstat -ano | grep -c LISTENING → 72
```

| 维度 | 实测/结论 |
|---|---|
| 耗时 | **0.075s**（421 行），无需管理员，普通用户进程即可执行 |
| 输出字段 | 协议 / 本地地址 / 外部地址 / 状态 / PID，一次调用同时给出端口与 PID |
| 解析脆弱性 | ① 列头随系统 UI 语言本地化（本机为英文 `Proto/Local Address/...`；中文系统为「协议/本地地址/外部地址/状态」）→ **建议跳过前若干行/按正则匹配而非固定列宽**；② IPv6 行本地地址形如 `[::]:135`、`fe80::...%12:xxx`，端口在最后一个冒号后；③ 列间为多空格对齐，须 `strings.Fields` 而非 `Split(" ")`；④ 存在 UDP 行（无 State 列 → 字段数 4 而非 5）→ **按协议分支解析**，不能统一取第 5 列 |
| 覆盖 | TCPv4/TCPv6/UDPv4/UDPv6 全覆盖；`-p tcp` 可只看 TCP（但 `-ano -p tcp` 组合在部分版本行为差异，建议全量后自行过滤） |

**A2. `tasklist /FO CSV /NH`（进程名/内存补通道）——本机实证**

```
> tasklist /FO CSV /NH | head -3
"System Idle Process","0","Services","0","8 K"
"System","4","Services","0","2,904 K"
"Secure System","236","Services","0","105,464 K"

> tasklist /FO CSV /NH | wc -l → 494 行
> time (tasklist /FO CSV /NH >/dev/null) → real 0m1.044s
```

| 维度 | 实测/结论 |
|---|---|
| 耗时 | **~1.0s**（494 进程）。比 `netstat` 慢一个数量级，是枚举链路的主要成本项 |
| 字段 | 映像名 / PID / 会话名 / 会话号 / 内存使用（**无命令行、无用户、无可执行路径**） |
| 解析脆弱性 | CSV 带引号且内存字段含千分位逗号（`2,904 K`）→ **必须用 CSV 解析器**（`encoding/csv`）而非 `Split(",")`；`/NH` 已去表头，但不同版本仍可能输出 `INFO:` 行 → 须跳过非法行 |
| 环境变量陷阱（实证） | 在 Git Bash / MSYS 下直接执行 `tasklist /FO CSV /NH` 会被路径转换破坏：`ERROR: Invalid argument/option - 'C:/Program Files/Git/FO'`。须 `MSYS_NO_PATHCONV=1`。**Go 的 `exec.Command` 不经 shell，不受影响**，但在 bash 里手工复现/写脚本验证时需加该变量 |
| 补充字段 | 需要命令行/可执行路径：`tasklist /V` 仍无命令行；可行解是 `wmic PROCESS WHERE ProcessId=... GET CommandLine`（**wmic 已在 Win10 21H1+ 弃用、Win11 24H2 默认移除**）或 PowerShell `Get-CimInstance Win32_Process`（启动成本 ~1s+）→ **不建议为命令行字段引入 PowerShell** |

**A3. PowerShell `Get-NetTCPConnection`**
- 输出为对象（`OwningProcess` 直接给 PID），无文本解析脆弱性
- 代价：`powershell -NoProfile -Command ...` 冷启动 **1s 量级**，且依赖 PowerShell 5.1+/7 存在（可被公司策略禁用/约束语言模式）
- 本机**未完成实测**（环境策略拦截了本次 PowerShell 调用），耗时为经验值

#### Linux 侧（无可实测环境，依据平台机制）

| 手段 | 说明 |
|---|---|
| `ss -tunlp` | 一行给出端口 + `pid=<n>`，最省事；但 ① 依赖 `iproute2` 安装（极简容器/最小化发行版可能无）；② `-p` 需有权限读 `/proc/<pid>/fd`，**非 root 看不到其他用户进程的 PID**（字段留空） |
| `/proc/net/tcp` + `/proc/net/tcp6` | 纯文件读取、零依赖；每行含 local_address（hex 端口）、st（状态）、inode。**需要第二步把 socket inode 映射到 PID**：遍历 `/proc/[0-9]*/fd/*` 找 `socket:[inode]`（O(进程数 × fd 数)） |
| `/proc/*/cmdline` | 遍历 `/proc` 目录读 `cmdline`（NUL 分隔）、`comm`（进程名）、`status`（Uid/内存）→ 拿进程名与命令行 |
| 优化 | 先按端口从 `/proc/net/tcp` 拿到 inode 集合，再只遍历 `/proc/*/fd` 做 inode 反查 → 端口检索场景只付出一次 `/proc` 遍历；关键字检索则直接遍历 `/proc/*/cmdline` |
| 权限 | 非 root 只能读自己的进程（其他用户目录 `cmdline` 读不到）→ 与 Windows 一致，需降级处理 |

#### 方案 A 小结
- **优点**：零新增依赖（对 govulncheck / 依赖体积零影响）、实现与调试直观、与既有 `obsidian_windows.go` 范式一致
- **缺点**：需要自己维护两套解析器（Windows 文本 + Linux /proc）；字段受限（Windows 无命令行/用户）；`netstat` 列头本地化与 IPv6 格式是长期脆弱点

### 2.2 方案 B：引入 `github.com/shirou/gopsutil/v4`

| 维度 | 评估 |
|---|---|
| 跨平台一致性 | **最好**。`process.Processes()` / `net.Connections("tcp")` 一套 API 覆盖 Windows/Linux/macOS，返回统一结构体（PID、Name、Cmdline、Username、MemoryInfo、Connections 含 Laddr.Port / Status） |
| 内部实现（Windows） | `net.Connections` 走 IP Helper API `GetExtendedTcpTable`（`iphlpapi.dll`），与方案 C 同源；进程信息走 `OpenProcess` + `NtQueryInformationProcess`，部分字段（如某些场景的 cmdline）走 WMI |
| 内部实现（Linux） | 解析 `/proc/net/tcp` + `/proc/[pid]/fd` inode 反查，与方案 A 的 Linux 思路一致（即引入它省的是**自己写这份代码**，不是换机制） |
| 解析脆弱性 | 由上游承担（`netstat` 本地化、IPv6、`/proc` 格式差异均已处理并持续维护）→ **这是引入它的最大实际收益** |
| 是否需管理员 | 否（与方案 A 相同，读取系统表/`/proc` 均不需提权；仅「终止他人/系统进程」需权限） |
| 枚举耗时 | 千级进程量级下与方案 A 同数量级（百毫秒~秒级，取决于是否取 Cmdline/Memory 等需额外系统调用的字段） |
| **新增依赖体积** | `go.mod` 当前依赖树里**没有** gopsutil。v4 会引入：`golang.org/x/sys`（**项目已有 v0.48.0，可复用**）、`github.com/go-ole/go-ole`（**项目已有 v1.3.0，间接依赖**）、`github.com/yusufpapurcu/wmi`（Windows WMI）、`github.com/ebitengine/purego`（v4 用于免 cgo 的动态库调用）、`github.com/tklauser/go-sysconf`、`github.com/power-devops/perfstat`（AIX，不进 Windows 构建但会进 go.sum）。**净新增 3~4 个模块**；二进制体积增量约为数百 KB 量级（wmi/go-ole 为主） |
| **govulncheck 影响** | 项目用 `govulncheck ./...`（**调用链分析，非全依赖树**，见 CLAUDE.md `security-scan.md` 条目）→ 只扫实际用到的 `process`/`net` 子包链路，未被调用的平台分支不产生告警；但依赖树变宽意味着**未来上游任一被引入的子包出现 CVE 时可能进告警面**，需每次升级后重跑 |
| 风险 | ① Windows 上部分字段走 WMI → **WMI 查询慢且可能挂起**（数百毫秒到数秒，WMI 服务异常时更久），需超时保护；② v4 与项目 Go 1.26 的兼容性需 `go get` 后实际编译验证；③ 引入后 CLAUDE.md 的覆盖率门禁/依赖规则无需改，但 `go.sum` 变更需要 PR 说明 |

> **未核实声明**：本报告**未在本机下载 gopsutil**（网络抓取与环境策略受限），上述依赖清单为依据 v4 公开实现的陈述，**实施前须以 `go get github.com/shirou/gopsutil/v4` 后的 `go.mod` / `go mod graph` 实际结果为准**，并随即跑 `govulncheck ./...` 确认清零。

### 2.3 方案 C：Windows IP Helper API `GetExtendedTcpTable` via `golang.org/x/sys/windows`（零新增模块）

- 用 `x/sys/windows.NewLazySystemDLL("iphlpapi.dll")` + `SyscallN` 调 `GetExtendedTcpTable`，取 `TCP_TABLE_OWNER_PID_ALL` 级别即可直接拿到 `dwLocalPort` / `dwOwningPid` / `dwState`
- **项目已有 `golang.org/x/sys` v0.48.0** → 无新增模块，无 govulncheck 影响
- 代价：需自行定义 `MIB_TCPTABLE_OWNER_PID` / `MIB_TCPROW_OWNER_PID` 结构体、处理 `ERROR_INSUFFICIENT_BUFFER` 重试循环、UDP 侧是另一组结构体（`GetExtendedUdpTable`）；**端口字节序需注意**（网络序，需 `ntohs`）
- Linux 侧仍需另一套 `/proc` 实现 → **跨平台代码量不会比方案 A 少**，只是 Windows 侧从「解析文本」变成「调 syscall」
- 适用判断：若**确定不引入 gopsutil 但要求 Windows 枚举不受 `netstat` 本地化影响**，这是最稳的零依赖方案

### 2.4 三方案对比表

| 维度 | A：标准库 + 命令解析 | B：gopsutil v4 | C：IP Helper API（x/sys） |
|---|---|---|---|
| 跨平台一致性 | 差（两套自研实现） | **好**（一套 API） | 差（Windows syscall + Linux /proc） |
| 解析脆弱性 | **高**（netstat 本地化、IPv6、/proc 格式） | 低（上游维护） | 中（结构体对齐/版本差异） |
| 是否需要管理员 | 否 | 否 | 否 |
| 千级进程枚举耗时 | ~1.0s（tasklist 主导） | ~同量级 | 最快（Windows 单次 syscall） |
| 新增依赖体积 | **零** | 净增 3~4 模块 | **零**（复用 x/sys） |
| govulncheck 影响 | **零** | 依赖面扩大（调用链分析下当前无告警，未来风险上升） | **零** |
| Windows netstat 本地化风险 | **存在** | 不存在 | 不存在 |
| 可拿到的字段 | 端口/PID/进程名/内存（Windows 无命令行） | **端口/PID/进程名/命令行/用户/内存/CPU** | 端口/PID（进程名需另查） |
| 实现工作量 | 中 | **低** | 高 |
| 与既有范式一致性 | **高**（同 `obsidian_windows.go`） | 低（新依赖需 PR 说明 + 文档更新） | 中 |

### 2.5 推荐

**首选方案 A（零依赖），字段收敛到「进程名 / PID / 端口 / 内存」**，理由：
1. PRD 验收项只要求「进程名、PID、端口」，方案 A 全覆盖；
2. 零新增依赖 → 对 govulncheck、二进制体积、CI 供应链面全部零影响，符合项目一贯的克制取向；
3. 与既有 `service/obsidian_windows.go` 的 `tasklist` 先例同源，团队可维护。

**触发升级到方案 B 的条件**：若后续明确要展示**命令行 / 用户 / CPU%**（开发者定位「哪个 java 是哪个服务」的强需求，命令行几乎是刚需），此时自研 Windows 命令行获取要么用已弃用的 `wmic`，要么启 PowerShell（1s+ 冷启动），**gopsutil 的性价比反超** → 建议那时直接引入 v4 并做超时保护。

**Windows 解析的三条硬约束（若选方案 A 必守）**：
1. `netstat` 输出**只按最后一列取 PID、倒数第二列取状态**，并按协议（TCP 5 列 / UDP 4 列）分支解析；
2. 本地地址端口取**最后一个冒号之后**（兼容 IPv6 `[::]:8080` 与 `fe80::1%12:8080`）；
3. 不依赖列头文字（本地化风险），用 `strings.Fields` 后按列数判定；`tasklist` 用 `encoding/csv` 解析（内存字段含千分位逗号）。

---

## 3. 问题二：Windows「优雅停止」的真实可行手段

> ## ⚠️ 实测更正（2026-10-09，覆盖本节原始结论）
>
> 本节 3.1、3.2、3.3、3.5 原结论为**机制性推理**（当时杀进程实测被环境策略拦截，见第 7 节 Caveats 第 1 条），
> 核心表述为「有顶层窗口的 GUI 进程能优雅关闭（WM_CLOSE）；无窗口后台进程 / 服务没有优雅通道，只能 `/F`」。
> **该结论已被本机实测推翻，两个方向都不成立**，保留原文只为留存推理过程，请勿再引用。
>
> **实测数据**（本机 Windows，真实执行）：
>
> | 用例 | 进程 | 命令 | 输出 | 1 秒后是否存活 |
> |---|---|---|---|---|
> | A | `ping -n 300 127.0.0.1`（`-WindowStyle Hidden`，控制台进程） | `taskkill /PID <id>` | `SUCCESS: Sent termination signal to the process with PID 38888.` | **已退出** |
> | B | `cmd /k echo STOP-TEST`（有可见窗口） | `taskkill /PID <id>` | `ERROR: The process with PID 68996 could not be terminated. Reason: This process can only be terminated forcefully (with /F option).` | **仍存活** |
>
> **无效用例（勿引用）**：`notepad` 的 `Start-Process -PassThru` 返回的是 Win11 AppX 包应用**启动器** PID，
> `taskkill` 报 `not found`，属包应用启动器特性，**不能读作「GUI 进程无法优雅关闭」**。
>
> **正确结论**：`taskkill`（不带 `/F`）对控制台进程投递控制信号、对窗口进程投递关闭消息，
> **成败取决于目标进程是否选择响应**，与「有无窗口」无对应关系——
> `ping` 默认响应并终止（成功且真退出），`cmd /k` 为保持交互会话主动忽略（报「只能强制终止」且仍存活）。
> 因此**无法按进程类型事前预判**，只能事后实测存活。
>
> **轮询仍然必需**：`SUCCESS` 只代表信号送达，不代表进程已退出；收到信号后延迟退出的进程
> （刷盘、写日志）在信号送达时尚未结束，若只看 `SUCCESS` 就报「已正常退出」即为撒谎。
> 本功能因此采用「发请求 → 5s 轮询实测存活 → 未退转强杀」而非「按类型预判有无优雅通道」：
> 轮询对 A（实测已退出 → `graceful`）与 B（首步即被拒 → 转强杀报 `forced`）两种情形都正确。
>
> 落地表述见 `prd.md` 的 Decision 章节；代码注释见 `util/process_windows.go`
> `RequestProcessStop` / `classifyTaskkillResult` 与 `service/process.go` `StopProcess`。

### 3.1 `taskkill /PID <n>`（不带 `/F`）

**官方文档口径**（已抓取 `learn.microsoft.com` taskkill 页，最后更新 2024-11-01 + 本机 `taskkill /?`）：
- `/f`：「Specifies that processes be forcefully ended. This parameter is ignored for remote processes; all remote processes are forcefully ended.」
- `/t`：「Ends the specified process and any child processes started by it.」
- **官方文档未说明不带 `/F` 时实际发送的机制**——这是本节所有不确定性的根源。

**社区/实证共识**（标注：本节行为分类依据 Windows 终止进程的通用机制与广泛社区经验；**本次未在本机实际 kill 进程做验证**，见第 6 节 Caveats。**2026-10-09 补充：下表按进程类型的分类已被本节开头「实测更正」推翻，仅作推理过程留存，禁止引用**）：

| 目标进程类型 | 不带 `/F` 的 `taskkill` 行为 | 对应机制 |
|---|---|---|
| 有顶层可见窗口的 GUI 进程 | **通常能结束**。应用收到关闭请求，可拦截（如弹「是否保存」对话框）→ 用户不响应就一直不退出 | 窗口关闭消息（`WM_CLOSE`）语义 |
| 有窗口但无响应（挂起/死锁） | **失败**，报「此进程只能强行终止（带 `/F` 选项）」→ 必须 `/F` | 消息循环不泵消息 |
| 控制台进程（有 conhost 窗口） | **行为不确定/常失败**：不是 `CTRL+C`（`CTRL+C` 只能经 `GenerateConsoleCtrlEvent` 投递），有窗口时走窗口关闭路径，无窗口时直接失败 | 见下节 |
| 无窗口的后台进程 / 服务 / 托盘程序 | **失败**。`ERROR: The process with PID xxxx could not be terminated. Reason: This process can only be terminated forcefully (with /F option).` | Windows 对无窗口进程没有标准的「请求退出」通道 |

**工程含义**：`taskkill`（不带 `/F`）**不是** Linux `SIGTERM` 的等价物。它只对「窗口型 GUI 进程」这一个子集有优雅语义，覆盖率取决于目标进程类型，无法保证。

### 3.2 `GenerateConsoleCtrlEvent`（控制台控制信号）

| 约束 | 说明 |
|---|---|
| 只作用于控制台 | 只能发给**附加到同一控制台**的进程组；目标进程必须是控制台进程，对 GUI/无控制台进程无效 |
| 调用方须先脱离自己的控制台 | 调用进程若已有控制台（GUI 进程通常没有，控制台进程有），须先 `FreeConsole()` 再 `AttachConsole(pid)`；一个进程同时只能附加一个控制台 |
| 须保护自身 | 发 `CTRL_C_EVENT` 前调用方要 `SetConsoleCtrlHandler(NULL, TRUE)` 否则**自己也会被杀** |
| 范围难以精控 | 事件发给共享该控制台的进程组（`dwProcessGroupId=0` 即全部）→ 可能波及同控制台的其他进程（如从同一 cmd 启动的兄弟进程） |
| 目标可忽略 | 目标进程若自行 `SetConsoleCtrlHandler` 忽略 `CTRL_C_EVENT`，信号无效 |
| 结论 | **只适用于「本应用自己 `CREATE_NEW_PROCESS_GROUP` 启动的控制台子进程」**（本项目终端服务经 conpty 管理的进程属这一类）；对**外部既有进程**（用户从资源管理器/IDE 启动的 node/java）**不适用**——进程组与控制台不可控 |

### 3.3 `EnumWindows` + `SendMessage(WM_CLOSE)`（窗口枚举硬关）

- 遍历顶层窗口，`GetWindowThreadProcessId` 匹配目标 PID，逐个发 `WM_CLOSE`（可先发 `WM_QUERYENDSESSION` 探询）
- 覆盖 GUI 进程（含隐藏窗口/托盘应用，只要窗口存在）→ **比 `taskkill` 覆盖面更广**，且不依赖外部命令
- 限制：
  - 无窗口进程（后台服务、无窗口的 java/node）完全无效；
  - **UIPI（用户界面特权隔离）**：低完整性级别进程无法向更高完整性级别（管理员/提权）进程的窗口发消息 → 非提权时杀不掉提权进程；
  - 跨会话不可见（Session 0 服务进程的窗口枚举不到）；
  - 应用弹「是否保存」对话框会阻塞退出 → **必须有超时降级**；
  - 实现需 `user32.dll` 三个函数 + 回调（`x/sys/windows` 或 `syscall.NewLazyDLL` 均可，零新增模块）
- 结论：**可作为 `taskkill`（不带 `/F`）的等价自研替代**，同样只覆盖窗口型进程，但省掉外部命令并可控超时

### 3.4 Linux 侧对应：SIGTERM

- `syscall.Kill(pid, syscall.SIGTERM)`：标准优雅终止，进程可自行清理（关闭监听、刷盘、退出子进程）
- 进程可以**忽略或阻塞** SIGTERM（如数据库进程优雅关闭可能耗时数十秒）→ 同样需要超时降级
- 现状 `util/kill_other.go:12` 的 `KillProcessTree` 用 `syscall.Kill(-pid, SIGKILL)`：`-pid` 表示**进程组**，仅当目标 PID 是进程组组长（如 shell 启动的进程）时才顺带杀子进程，否则会失败并落到单进程兜底——**这是现状语义，实施「停止」时应另建 `SIGTERM` 路径而非复用它**

### 3.5 结论：Windows 上能否真正优雅？

**不能通用优雅，只能「尽力优雅 + 显式降级」。** 三层现实（**已被本节开头「实测更正」推翻，禁止引用**，正确结论：成败取决于目标进程是否响应，与进程类型无关）：

| 进程类型 | 能否优雅 | 手段 |
|---|---|---|
| 有窗口的 GUI 应用 | **能**（可被拦截） | `taskkill` 不带 `/F`，或 `EnumWindows` + `WM_CLOSE` |
| 本应用自 spawn 的控制台子进程 | **能** | `GenerateConsoleCtrlEvent(CTRL_C_EVENT)` |
| 无窗口后台进程 / 外部控制台进程 / 服务 | **不能** | 无标准请求退出通道，只能 `TerminateProcess`（`/F`） |

**推荐的降级策略（供 PRD 决策）**：
1. 用户点「停止」→ 后端按平台发优雅请求（Windows：窗口关闭请求；Linux：`SIGTERM`）；
2. 后端轮询目标进程存活（建议 5s 上限，间隔 300~500ms）；
3. 三种出口：
   - **已退出** → 成功提示；
   - **超时仍存活** → 返回引导级错误（如 `E_PROCESS_STOP_TIMEOUT`），前端弹确认：「进程 N 秒未响应关闭请求，是否强制结束？未保存的数据可能丢失。」→ 用户确认后再发强杀；
   - **目标无优雅通道（前置判定为无窗口进程）** → 直接提示「该进程无窗口，无法优雅关闭，是否强制结束？」，**不假装优雅**（避免用户以为已优雅停止）。
4. 强杀统一走既有 `util.KillProcessTree`（`/T /F`）——若产品决定「默认杀树」则直接复用；若决定「默认只杀单进程、杀树作二级选项」（与 Windows 任务管理器一致），则需新增不带 `/T` 的路径。

> 说明：轮询等待不能阻塞前端主线程——与项目既有长操作一致，应放在后端 goroutine + ctx 超时（参照 `service/content_search.go` 的超时收口范式），或前端先返回「停止中」再由结果事件/二次调用确认。

---

## 4. 问题三：同类工具的 UX 约定

> 说明：以下为依据各工具长期使用经验的归纳，**未逐条抓取官方页面二次核实**（本次外部检索受环境限制，见 Caveats）。用于对齐交互惯例。

### 4.1 各工具的列与操作

| 工具 | 展示列 | 排序 / 筛选 | 结束进程的确认 | 是否默认杀树 |
|---|---|---|---|---|
| **Windows 任务管理器·详细信息** | 名称、PID、状态、用户名、CPU、内存、（可选列：命令行、体系结构、会话 ID） | 点列头排序；无内置关键字筛选（靠「搜索」跳转到进程） | 「结束任务」**无确认对话框**（直接结束）；未响应进程表现为直接终止 | **否**。「结束进程树」是**独立右键菜单项**，需用户显式选择 |
| **Sysinternals Process Explorer** | **树形**进程（父子关系）、PID、CPU、私有字节、工作集、描述、公司名；进程属性页含 **TCP/IP 标签页**（列出该进程所有连接的本地/远程地址与状态） | 树形 + 列排序；`Ctrl+F` 查找句柄/DLL；`Find → Find Handle or DLL` 可按端口/DLL 反查进程 | 「Kill Process」有确认；「Kill Process Tree」单独项并明确提示会杀子进程 | **否**（树为独立操作） |
| **macOS 活动监视器** | 进程名、PID、用户、%CPU、内存、能耗、磁盘、网络、端口/位置（「打开的文件和端口」面板可查监听端口） | 列排序 + 右上搜索框（可按名称/PID/端口过滤） | 「退出」：无响应进程提示「是否强制退出」；「强制退出」弹确认「确定要强制退出 X 吗？未保存的更改将丢失」 | **否**（默认单进程；「退出」不递归杀子进程） |
| **htop** | PID、USER、PRI、NI、VIRT、RES、SHR、S、CPU%、MEM%、TIME+、Command（**完整命令行**） | F6 选择排序列；F3 搜索；F4 过滤；F5 树形 | F9 弹出**信号选择列表**，默认选中 `SIGTERM(15)` → 需回车确认发送 | 否（对单进程发信号；进程组需自行处理） |
| **`lsof -i :PORT`** | COMMAND、PID、USER、FD、TYPE、DEVICE、SIZE/OFF、NODE、NAME（含 `:PORT`） | 无交互；常与 `grep` 组合 | 不涉及（只列出，杀进程由用户自行 `kill`） | 不涉及 |
| **VS Code 端口面板** | 端口、本地地址、进程名/CWD（转发标签）、来源（自动检测/手动）；悬停显示详情 | 端口列表按端口号排序；顶部有端口筛选输入框 | 面板内「Stop」**只停止端口转发**，**不结束进程**；结束进程需另用终端/系统工具 | 不涉及 |
| **JetBrains IDE** | 无通用进程管理器；Run/Debug 工具窗口列运行配置与进程，进程详情页可看 JVM 参数 | 按启动顺序/名称 | 「Stop」（`Ctrl+F2`）先尝试优雅关闭（相当于 shutdown hook），**长时间不退出时按钮变为「Terminate」（杀进程）** | 否（结束的是进程组/子进程由 IDE 管理） |

### 4.2 可提炼的 UX 约定

1. **列集合**：进程名 / PID / 端口（或连接）/ 命令行 / 内存 / 用户 是六项最高频列；**命令行**是开发者区分同名进程（多个 `java.exe`、多个 `node.exe`）的决定性字段，价值高于内存/CPU。
2. **检索**：**关键字过滤**（名称/命令行子串）是标配；端口检索有两种形态——「输入框直接填端口号」（类 `lsof -i :PORT`）与「大列表 + 筛选框」（类活动监视器/VS Code）。前者更贴合本 PRD 的痛点（「36115 被谁占了」）。
3. **排序**：默认按**端口号**或**PID**升序；不自动按内存排序（避免列表跳动）。
4. **结束确认**：**除 Windows 任务管理器外，主流工具均有二次确认**；且确认文案会区分「正常退出」与「强制退出」的后果。macOS 与 htop 的做法（默认选中 SIGTERM，强制作二级选择）与本报告推荐的「停止/强杀双按钮」最一致。
5. **杀树不是默认**：Windows/macOS/htop 均把「连带子进程」作为**独立二级操作**。JetBrains 的「先优雅→按钮变红为 Terminate」是**渐进升级**的优秀范式，可作为「停止 → 超时 → 强杀」的交互借鉴。
6. **进程树展示**：Process Explorer 与 htop 都提供树形视图 → 若产品决定支持杀树，用树形/层级缩进表达父子关系是用户可预期的方式。

---

## 5. 问题四：权限与自我保护边界

### 5.1 枚举阶段会被拒绝/不可见的进程

| 平台 | 现象 | 机制 |
|---|---|---|
| Windows | `System Idle Process`（PID 0）、`System`（PID 4）、`Secure System`（本机实测 PID 236，VBS 安全内核）**可枚举但不可终止** | 非真实用户态进程；`OpenProcess(PROCESS_TERMINATE)` 返回 `ACCESS_DENIED` |
| Windows | `csrss.exe`、`smss.exe`、`lsass.exe`、`MsMpEng.exe` 等 **PPL（Protected Process Light）** | 受保护进程，即使管理员 `taskkill /F` 也报「拒绝访问」 |
| Windows | 其他用户会话 / 更高完整性级别（UAC 提权）进程 | 非提权进程无法 `OpenProcess`；`taskkill` 报「拒绝访问」 |
| Linux | 其他用户的进程 | 非 root 无权限发信号（`EPERM`）、读不到其 `/proc/<pid>/cmdline` |
| Linux | 内核线程（`/proc/<pid>/cmdline` 为空，PPid=2） | 无用户态实体，不可杀 |

### 5.2 强杀系统关键进程的后果

| 进程 | 后果 |
|---|---|
| `csrss.exe` / `smss.exe` / `wininit.exe` | **立即 BSOD**（Windows 关键子系统被终止即蓝屏） |
| `lsass.exe` | 安全子系统终止 → 系统触发**强制重启/蓝屏** |
| `winlogon.exe` / `services.exe` | 会话/服务控制管理器终止 → 系统不可用，需重启 |
| `svchost.exe`（任一） | 承载多项系统服务 → 网络/更新/音频等功能异常，严重时需重启 |
| `explorer.exe` | 桌面与任务栏消失（**相对温和**，可从任务管理器重新运行恢复）——常被当作「可接受」的强杀目标 |
| Linux PID 1（systemd/init） | SIGTERM 由 init 特殊处理（多数被忽略）；强杀不可行，且会导致系统失去 init |

### 5.3 如何避免误杀本应用自身（workbench.exe，监听 36115）

**硬禁（PID 级，必须做）**：
- `os.Getpid()` 得到自身 PID → 结果列表中标记「本应用」并**禁用停止/强杀按钮**；后端在终止入口再判一次（前端可绕过，后端必须有最终防线）
- **父进程链**：把自身 PID 的父链（或至少直接父进程）纳入保护，避免杀掉守护/启动器导致自身连带退出（尤其在 `wails dev` 场景下父进程可能是终端）

**软提示（名称/端口级）**：
- 进程名 `workbench.exe`：**只能用于标注，不能作为保护判据**——`docs/spec/e2e-testing.md:108` 记录 E2E 会把 exe 复制改名为 `workbench-e2e.exe` 实现实例隔离，按名称保护会漏；反之按名称保护也可能误伤用户自建同名 exe
- 监听 36115 的进程：在 UI 上标注「（本应用监听端口）」，停止按钮仍允许但**二次确认文案升级**为「该端口由 WorkBench 自身监听，结束将导致本应用退出，确定继续？」

### 5.4 保护策略建议（三层）

| 层 | 内容 | 表现 |
|---|---|---|
| **L1 硬禁** | 自身 PID + 父进程链 | 按钮禁用 + 后端拒绝（错误码 `E_PROCESS_SELF_PROTECT`，warning 级） |
| **L2 系统关键进程警告** | Windows 黑名单：`System` / `System Idle Process` / `Secure System` / `csrss` / `smss` / `lsass` / `wininit` / `winlogon` / `services`；Linux：PID 1、内核线程（cmdline 为空） | 允许尝试但**二次确认必须显式警告后果**（多数情况 `taskkill` 会直接返回拒绝访问，此时提示「权限不足，需以管理员身份运行」） |
| **L3 通用确认** | 其他所有进程 | 「确定结束进程 X（PID n）？未保存的数据将丢失。」强杀文案再加强调 |

同类工具的做法：**Windows 任务管理器不做白名单**（点了就结束，靠系统自身的 ACCESS_DENIED 兜底）；**macOS 活动监视器对系统进程显示但强制退出时警告**；**Process Explorer 允许尝试并由系统拒绝**。→ 本项目采用「L1 硬禁 + L2/L3 确认」比「完全禁止」更符合用户预期（用户可能确实需要结束 svchost 排查问题）。

---

## 6. 落地到本项目的参照点

| 事项 | 参照 |
|---|---|
| 平台分叉文件 | `util/process_windows.go` + `util/process_other.go`（编译期分叉，符合 `docs/spec/linux-platform.md`）；测试拆 `_windows_test.go` / `_linux_test.go` |
| Service 装配 | 新增 `service/process.go` → `app_services.go` 的 `AppServices` struct 加字段 + `NewAppServices` 加构造行（**2 处**，见 `docs/spec/app-services-assembly.md`） |
| 错误码 | 建议新增 `E_PROCESS_ENUM_FAILED` / `E_PROCESS_NOT_FOUND` / `E_PROCESS_ACCESS_DENIED` / `E_PROCESS_STOP_TIMEOUT` / `E_PROCESS_KILL_FAILED` / `E_PROCESS_SELF_PROTECT`；同步 `model/app_error.go:50` 常量表 + `frontend/src/utils/error.js` 的 `ErrorCode` 与 `WARNING_CODES`（超时/拒绝/自我保护为 warning 级） |
| 超时收口 | 参照 `service/content_search.go`：**先判 `ctx.Err()` 再判命令退出码**，超时不得误判为「无结果/已成功」 |
| 解析单点 | 参照 `util/git_lock.go:34` `parseGitLockError`：把「拒绝访问」「只能强制终止」等 stderr 形态判定收敛到单一函数，返回 `*model.AppError` |
| 命令执行 | `util.HideCommandWindow(cmd)`（`util/exec_windows.go:10`）避免闪窗；枚举命令自身加 `context` 超时（复用 `util.ExecuteWithContext` 系列，见 CLAUDE.md「Git 操作取消链路」条目） |
| 前端入口 | `frontend/src/components/ToolboxPanel.vue:189` `tools` 数组新增「进程管理」卡片；弹窗内容量大 → 按 CLAUDE.md「内容多大弹窗 900+」分档，尺寸 `min(900px, 92vw)` 类 |
| 跨层契约 | 新增 App 方法与 `model/` 结构体 → 手动同步 `frontend/wailsjs/` 的 `App.js` / `App.d.ts` / `models.ts` **三处**（`docs/spec/cross-layer-contracts.md`） |

---

## 7. Caveats / 未核实项

1. **杀进程类实测已完成（2026-10-09）——原机制性结论被推翻**：原记录为「实测被环境安全策略拦截，待实施阶段补验」。实施阶段实测结果：**A. `ping -n 300 127.0.0.1`（`-WindowStyle Hidden`，控制台进程）→ `SUCCESS: Sent termination signal...`，1 秒后已退出；B. `cmd /k echo STOP-TEST`（有可见窗口）→ `ERROR: ... can only be terminated forcefully (with /F option).`，1 秒后仍存活**。即成败取决于**目标进程是否响应**，与「有无窗口」无对应关系，第 3.1 / 3.2 / 3.3 / 3.5 节按进程类型的分类**已被推翻、禁止引用**（原文保留作推理留存）。另注：原建议的 notepad 用例**无效**——Win11 下 `Start-Process -PassThru` 返回 AppX 包应用启动器 PID，`taskkill` 报 `not found`，属启动器特性，不能读作「GUI 进程无法优雅关闭」。完整更正见第 3 节开头「实测更正」框。
2. **gopsutil v4 依赖树未下载核实**：网络抓取受限，依赖清单为依据公开实现的陈述，非本机 `go get` 结果。若采纳方案 B，须以实际 `go.mod` 为准并重跑 `govulncheck ./...`。
3. **Linux 侧无实测环境**：`/proc` 解析与 `ss` 可用性为机制层面陈述，未在本机（Windows）验证。
4. **`netstat` 本地化未实测中文形态**：本机系统 UI 语言为英文，输出 `Proto / Local Address / Foreign Address / State / PID`；中文系统的列头文字未实测（但列序一致、解析应跳过列头故风险可控）。
5. **PowerShell `Get-NetTCPConnection` 未实测**：冷启动耗时（经验值 ~1s）与 `-OwningProcess` 可见性未在本机验证。
6. **`GenerateConsoleCtrlEvent` 未做代码级验证**：本节为其 API 约束的机制性描述，未编写/运行验证程序。
7. **MSYS 路径转换陷阱**：`tasklist /FO CSV /NH` 在 Git Bash 下会被转换成 `C:/Program Files/Git/FO` 而报错（已实证），须 `MSYS_NO_PATHCONV=1`；Go `exec.Command` 不受影响。
8. **现有 `util/kill_other.go` 的 `-pid` 语义**：`syscall.Kill(-pid, SIGKILL)` 仅当目标是进程组组长时才连带子进程，对外部进程通常落到单进程兜底——实施「杀树」时须明确这一现状语义，不要假定它总能杀树。

---

## 8. 外部参考

| # | 来源 | 关键点 | 类型 | 抓取时间 |
|---|---|---|---|---|
| 1 | Microsoft Learn — `taskkill` | 语法与 `/f` `/t` `/fi` 语义；**未说明不带 `/F` 时的机制**；远程进程一律强制结束 | 官方 | 2026-10-09 |
| 2 | 本机 `taskkill /?` | 与官方文档一致的参数说明，无机制描述 | 本机实证 | 2026-10-09 |
| 3 | 本机 `netstat -ano` | 421 行 / 0.075s / 英文列头 / 72 条 LISTENING / 36115 被 PID 44336 占用 | 本机实证 | 2026-10-09 |
| 4 | 本机 `tasklist /FO CSV /NH` | 494 行 / ~1.0s / 前 3 行为 System Idle Process、System、Secure System / 内存字段含千分位逗号 | 本机实证 | 2026-10-09 |
| 5 | 项目内 `service/obsidian_windows.go:58` | `tasklist` 全量枚举先例；`/FI` 在 cmd 嵌套下不可靠 | 内部代码 | 2026-10-09 |
| 6 | `.trellis/tasks/archive/2026-07/07-08-obsidian-vault-not-found/research/obsidian-auto-register.md` | tasklist 枚举的 Go 实测记录与「检测失败时保守降级」策略 | 内部研究 | 2026-10-09 |
| 7 | 项目 `docs/spec/e2e-testing.md:108` | E2E 把 exe 改名 `workbench-e2e.exe`，证明**按进程名做自我保护不可靠** | 内部规范 | 2026-10-09 |
| 8 | 同类工具 UX（任务管理器 / Process Explorer / 活动监视器 / htop / lsof / VS Code 端口面板 / JetBrains） | 第 4 节归纳；**未逐条抓取官方页面核实** | 产品经验（未外链核实） | 2026-10-09 |
