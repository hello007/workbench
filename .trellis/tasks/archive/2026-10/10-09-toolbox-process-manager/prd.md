# 工具箱新增进程管理（端口/关键字查进程 + 停止/强杀）

## Goal

在侧边栏「工具箱」面板新增「进程管理」工具：用户按**端口号**或**进程名关键字**检索本机进程，结果列表展示进程名 / PID / 端口 / 协议 / 状态 / 内存，并对命中进程执行**停止**（优雅请求 → 5 秒未退自动转强杀）或**强杀**（直接强制终止）。

**为什么**：开发者日常高频痛点是端口被占用（本机实证 36115 被 PID 44336 占用，常为常驻的 workbench.exe 自身），当前必须切到任务管理器或手工 `netstat -ano | findstr :端口` + `taskkill`。本功能把「查端口 → 定位进程 → 结束」三步收进工作台内。

---

## Requirements

### 功能需求

| # | 需求 |
|---|---|
| R1 | 工具箱新增「进程管理」卡片入口，点击打开进程管理弹窗 |
| R2 | 单输入框智能识别：输入**纯数字** → 按端口精确匹配；**含非数字字符** → 按进程名关键字模糊匹配 |
| R3 | 旁附模式切换下拉（自动 / 端口 / 进程名），用于消除歧义（如进程名恰为纯数字） |
| R4 | 空输入或首次打开 → 列出全部处于 LISTENING 的进程（本机实测 72 条） |
| R5 | 结果展示列：进程名 / PID / 端口 / 协议（TCP/UDP）/ 连接状态 / 内存占用 |
| R6 | 每行提供「停止」「强杀」两个按钮 |
| R7 | 标题栏提供「刷新」按钮手动重拉枚举结果 |
| R8 | 弹窗顶部「同时结束子进程（进程树）」勾选框，默认关闭 |

### 已定：终止语义（全自动两级）

* **「停止」**：发优雅停止请求 → 轮询存活上限 5s（间隔 300~500ms）→ 实测已退出提示成功；**超时未退自动转强杀**，中途不询问用户
* **「强杀」**：跳过优雅阶段，直接强制终止
* Windows 优雅请求走 `taskkill` 不带 `/F`（对控制台进程投递控制信号、对窗口进程投递关闭消息）；Linux 走 `SIGTERM`
* **UI 不谎称已优雅停止**：优雅请求能否生效**取决于目标进程是否响应，无法按进程类型事前预判**（实测更正见 `research/process-enumeration.md` 第 3 节：`ping -n 300` 响应并退出；`cmd /k` 为保持交互会话主动忽略、报「只能强制终止」且仍存活）。故不做类型预判，统一「发请求 → 轮询实测存活 → 未退转强杀」，结果提示如实反映最终实际生效的手段（「已正常退出」/「已强制结束」）

### 已定：确认与自我保护（三层）

| 层 | 范围 | 表现 |
|---|---|---|
| **L1 硬禁** | 本应用自身 PID + 父进程链 | 前端按钮禁用并标注「本应用」；后端终止入口**再判一次**（前端可绕过，后端是最终防线），返 warning 级 `E_PROCESS_SELF_PROTECT` |
| **L2 强确认** | 系统关键进程（Windows：`System` / `System Idle Process` / `Secure System` / `csrss` / `smss` / `lsass` / `wininit` / `winlogon` / `services`；Linux：PID 1、内核线程） | 红色警告确认，明示后果（蓝屏 / 系统重启） |
| **L3 通用确认** | 其余所有进程 | 「确定结束 X（PID n）？未保存的数据将丢失。」强杀文案再加强调 |

* 自我保护**只用 PID 判据**，禁止用进程名 `workbench.exe`（E2E 会把 exe 改名 `workbench-e2e.exe`，见 `docs/spec/e2e-testing.md`）
* `IsSelf` / `IsSystem` 标志由**后端填充**随列表返回，前端只消费不自行判定（避免两套判据漂移）

### 已定：进程树范围

* 「停止」「强杀」默认只终止**目标进程本身**（Windows `taskkill` 不带 `/T`；Linux 对单 PID 发信号）
* 勾选「同时结束子进程」时才走进程树路径
* 既有 `util.KillProcessTree` 保留给 AI task 取消等既有调用方；本功能默认路径**新增不带 `/T` 的单进程终止路径**

### 已定：边缘态提示

| 边缘态 | 表现 |
|---|---|
| 进程已自行退出（点击时已不存在） | 提示「进程已不存在」并自动刷新列表 |
| 权限不足（拒绝访问 / PPL 受保护进程） | warning 提示「权限不足，需以管理员身份运行」 |
| 枚举超时 | **先判 `ctx.Err()` 再判命令退出码**（对齐 `service/content_search.go` 收口范式），超时不得误判为「无结果」，提示可重试 |

---

## Acceptance Criteria

- [ ] 工具箱出现「进程管理」卡片，点击打开弹窗
- [ ] 输入端口号（如 `36115`）仅返回占用该端口的进程
- [ ] 输入进程名关键字（如 `node`）返回进程名匹配的全部进程
- [ ] 空输入返回全部 LISTENING 进程，含进程名 / PID / 端口 / 协议 / 状态 / 内存
- [ ] 模式切换下拉可强制指定「端口」或「进程名」，覆盖自动识别结果
- [ ] 自身进程行标注「本应用」且「停止」「强杀」按钮禁用；绕过前端直调后端被拒并返回 `E_PROCESS_SELF_PROTECT`
- [ ] 系统关键进程点击后弹红色警告确认
- [ ] 普通进程点击后弹通用确认
- [ ] 「停止」对响应优雅请求的进程（如 `ping`）可正常退出，提示「已正常退出」
- [ ] 「停止」对不响应优雅请求的进程（如 `cmd /k`）立即转强杀，提示「已强制结束」
- [ ] 「停止」对 5 秒内未退出的进程自动转强杀，提示「已强制结束」
- [ ] 「强杀」直接强制终止，提示「已强制结束」
- [ ] 勾选「同时结束子进程」时子进程一并终止；不勾选时子进程存活
- [ ] 权限不足 / 进程已退出 / 枚举超时三类边缘态各有明确提示，无静默失败
- [ ] 「刷新」按钮可重拉最新结果

## Definition of Done

* 后端单测：`util` 解析纯函数（Windows fixture 的 `netstat`/`tasklist` 输出、Linux fixture 的 `/proc` 内容）+ `service` 层用注入的 fake 命令执行器
* 平台测试文件按惯例拆 `_windows_test.go` / `_linux_test.go`
* 前端组件单测（列表渲染 / 自身禁用 / 三级确认 / 模式切换）
* `go test ./...` 与 `npm test` 绿；覆盖率门禁达标（后端 service ≥76%、util ≥40%，前端 ≥70%）
* 错误码同步两处：`model/app_error.go` 常量表 + `frontend/src/utils/error.js` 的 `ErrorCode` 与 `WARNING_CODES`
* 跨层契约同步三处：`frontend/wailsjs/` 的 `App.js` / `App.d.ts` / `models.ts`
* 服务装配：`app_services.go` 的 struct 字段 + `NewAppServices` 构造行（2 处）
* 文档更新：`README.md` 与 `docs/功能说明.md` 工具箱章节
* 手工验证补齐研究缺口（**已完成**）：隐藏窗口 `ping -n 300`（控制台进程）与可见窗口 `cmd /k`（控制台交互进程）各跑一次，实测结论为「能否优雅取决于目标进程是否响应」而非「有无窗口」，详见 `research/process-enumeration.md` 第 3 节实测更正；notepad 用例无效（Win11 包应用启动器 PID 不可作为目标，见同节说明）

---

## Technical Approach

### 技术选型：方案 A —— 零依赖解析（已定）

| 平台 | 实现 |
|---|---|
| Windows | `netstat -ano` 拿端口 + PID + 协议 + 状态（实测 421 行 / **0.075s**）；`tasklist /FO CSV /NH` 补进程名与内存（实测 494 进程 / **~1.0s**） |
| Linux | 解析 `/proc/net/tcp{,6}` + `/proc/*/fd` inode 反查端口→PID；`/proc/*/cmdline`、`/proc/*/status` 拿进程名与内存 |

**不引入 gopsutil**：命令行列未纳入 MVP，方案 A 已覆盖全部所需字段，零新增依赖对 `govulncheck`、二进制体积、CI 供应链面全部零影响。

**Windows 解析三条硬约束（必守）**：

1. `netstat` 输出按 `strings.Fields` 拆分后按**列数**分支解析（TCP 5 列 / UDP 4 列），**不依赖列头文字**（系统 UI 语言本地化会使列头变化）
2. 本地地址端口取**最后一个冒号之后**（兼容 IPv6 `[::]:8080` 与 `fe80::1%12:8080`）
3. `tasklist` 用 `encoding/csv` 解析（内存字段含千分位逗号如 `2,904 K`），跳过非法行

### 分层与文件落点

| 层 | 文件 | 职责 |
|---|---|---|
| model | `model/process.go` | `ProcessInfo` / `ProcessPort` / `ProcessStopResult` 结构体 |
| model | `model/app_error.go` | 新增错误码常量 |
| util | `util/process_windows.go` / `util/process_other.go` | 平台枚举与终止；编译期分叉 |
| util | `util/process_parse_windows.go` / `_other.go` | **纯解析函数**（输入 output string，输出结构体）——可单测的核心 |
| service | `service/process.go` | 编排枚举 / 过滤 / 自我保护 / 终止 / 超时 |
| main | `app_process.go` | App 委托方法 |
| main | `app_services.go` | 装配（2 处） |
| 前端 | `frontend/src/components/ProcessManagerDialog.vue` | 新建，进程管理弹窗 |
| 前端 | `frontend/src/components/ToolboxPanel.vue` | `tools` 数组新增卡片 |

### Wails 绑定方法

| 方法 | 签名 |
|---|---|
| `ListProcesses` | `(keyword string, mode string) ([]model.ProcessInfo, error)` |
| `StopProcess` | `(pid int, killTree bool) (*model.ProcessStopResult, error)` |
| `KillProcess` | `(pid int, killTree bool) error` |

> `mode` 用**普通 `string`** 而非 `type ProcessFilterMode string` 具名类型——CLAUDE.md 记录 wails generate 不为具名 string 生成 models.ts 别名，会导致 `npm run build` 报 MISSING_EXPORT（vitest 不报、易漏）。

### 数据模型

```go
type ProcessPort struct {
    Port      int
    Protocol  string // TCP / UDP
    State     string // LISTENING / ESTABLISHED
    LocalAddr string
}

type ProcessInfo struct {
    Pid      int
    Name     string
    Ports    []ProcessPort
    MemoryKB int64
    IsSelf   bool // 本应用自身 + 父进程链，前端据此禁用按钮
    IsSystem bool // 系统关键进程，前端据此红色确认
}

type ProcessStopResult struct {
    Stopped bool   // 最终是否已终止
    Method  string // "graceful" / "forced"，前端据此选择提示文案
}
```

### 错误码（新增）

| 码 | 级别 | 含义 |
|---|---|---|
| `E_PROCESS_ENUM_FAILED` | error | 枚举命令执行失败 |
| `E_PROCESS_ENUM_TIMEOUT` | warning | 枚举超时（先判 `ctx.Err()`），可重试 |
| `E_PROCESS_NOT_FOUND` | warning | 目标进程已不存在（列表已过期） |
| `E_PROCESS_ACCESS_DENIED` | warning | 权限不足 / 受保护进程，引导以管理员身份运行 |
| `E_PROCESS_SELF_PROTECT` | warning | 拒绝终止本应用自身或父进程链 |
| `E_PROCESS_KILL_FAILED` | error | 强杀失败（含 stderr 形态识别） |

* 停止超时**不产生面向用户的错误码**（全自动转强杀），仅在后端 `slog.Warn` 记录
* stderr 形态识别（「拒绝访问」「只能强行终止」）收敛到**单一函数**，对齐 `util/git_lock.go` 的 `parseGitLockError` 范式

### 超时与并发

| 参数 | 值 |
|---|---|
| 枚举 `netstat` / `tasklist` | 整体 ctx 10s |
| 停止优雅等待轮询 | 上限 5s，间隔 300~500ms |
| 命令窗口 | 调外部命令须 `util.HideCommandWindow(cmd)` 避免闪黑窗 |

---

## Decision (ADR-lite)

**Context**：Windows 上不存在 SIGTERM 等价物。`taskkill`（不带 `/F`）对控制台进程投递控制信号、对窗口进程投递关闭消息，**能否生效取决于目标进程是否响应**，无法按进程类型事前预判。本机实测（2026-10-09，见 `research/process-enumeration.md` 第 3 节）：

| 用例 | 进程 | 输出 | 1 秒后是否存活 |
|---|---|---|---|
| A | `ping -n 300 127.0.0.1`（`-WindowStyle Hidden`） | `SUCCESS: Sent termination signal...` | **已退出** |
| B | `cmd /k echo STOP-TEST`（有可见窗口） | `ERROR: ... can only be terminated forcefully (with /F option).` | **仍存活** |

即：不是「有没有窗口」的问题，是「进程买不买账」的问题。`GenerateConsoleCtrlEvent` 仅适用于应用自身 spawn 的控制台子进程，对外部既有进程不适用。

**Decision**：

1. 「停止」定义为**全自动两级**：优雅请求 → 5s 轮询实测存活 → 未退自动强杀，中途不询问用户；**不做进程类型预判**（无法预判），轮询对「已退出」与「被拒绝」两种情形都正确
2. 结果提示**如实反映实际生效手段**（`graceful` = 实测已退出，`forced` = 请求被拒或超时后强杀）；`SUCCESS` 只代表信号送达不代表已退出，故轮询不可省，否则对延迟退出的进程会撒谎
3. 进程枚举采用**零依赖方案 A**（不引入 gopsutil）
4. 进程树**默认关闭**，作为显式二级选项
5. 自我保护**三层**（自身 PID 硬禁 / 系统进程强确认 / 普通进程通用确认），判据只用 PID

**Consequences**：

* 优点：零新增依赖；交互按键次数最少；语义诚实不误导
* 代价：优雅请求是否被响应不可控，「停止」在相当一部分进程上最终落为强杀（用户可能感知为「点了停止却像强杀」），靠文案如实说明消除歧义；自维护两套解析器，`netstat` 本地化与 IPv6 格式是长期脆弱点
* 未来触发升级条件：若需命令行 / 用户 / CPU% 列（区分同名 `java.exe` 的刚需），Windows 侧自研成本过高（wmic 已弃用、PowerShell 冷启动 ~1s），届时直接引入 `gopsutil/v4` 并做超时保护

---

## Out of Scope

* **命令行列**（本次明确不纳入，是触发引入 gopsutil 的升级条件）
* 进程树层级视图（父子缩进展示）
* CPU% / 用户名 / 可执行路径列
* 自动轮询刷新（仅手动刷新按钮）
* 按端口批量结束该端口上的全部进程
* Linux 侧实测验证（当前开发环境为 Windows，仅保证代码路径与单测 fixture 覆盖）
* 从其他面板（如 web serve 端口冲突、设置面板）跳转到进程管理的联动入口

---

## Research References

* [`research/process-enumeration.md`](research/process-enumeration.md) — 零依赖枚举可行；Windows 无 SIGTERM 等价物；自我保护须用 PID 而非进程名

## Technical Notes

**已读文件**

* `frontend/src/components/ToolboxPanel.vue`（工具箱卡片范式 + `tools` 数组 + el-dialog 480px）
* `util/kill_windows.go` / `util/kill_other.go`（既有 `KillProcessTree`）
* `app_services.go`（服务装配 2 处）
* `go.mod`（无 gopsutil；已有 `golang.org/x/sys`）
* `model/app_error.go`（错误码常量表）

**项目规范约束（来自 CLAUDE.md）**

| 约束 | 要点 |
|---|---|
| `app-services-assembly.md` | 新增 service 改 `app_services.go` 的 struct 字段 + `NewAppServices` 构造行（2 处） |
| `cross-layer-contracts.md` | 新增 App 方法与 `model/` 导出结构须手同步 `frontend/wailsjs/` 三处；具名 string 类型需手补 models.ts 别名（本方案改用普通 string 规避） |
| `logging-and-errors.md` | 错误走 `model.AppError` + ErrorFormatter；新增错误码同步 `model/app_error.go` + `frontend/src/utils/error.js`；超时先判 `ctx.Err()` |
| `linux-platform.md` | 编译期平台分叉走 `_windows.go` / `_other.go`；测试平台断言拆 `_windows_test.go` / `_linux_test.go` |
| `frontend-visual-conventions.md` | 弹窗按内容量分档：列表类走 900+ 档（`min(900px, 92vw)` 类），**不适用现有 480px** |
| `e2e-testing.md` | E2E 把 exe 改名 `workbench-e2e.exe` → 按进程名自我保护不可靠 |
| `security-scan.md` | 依赖升级后须重跑 `govulncheck ./...`（本方案零新增依赖，无影响） |

**研究缺口实证结果（2026-10-09 已填补）**

* `taskkill` 不带 `/F` 按「GUI / 控制台 / 无窗口」三类进程分类的机制性结论**已被实测推翻**：真实判据是「目标进程是否响应」而非「有无窗口」（A：`ping -n 300` 响应并退出；B：`cmd /k` 有可见窗口但主动忽略）。实测数据与正确语义见 `research/process-enumeration.md` 第 3 节「实测更正」，原机制性结论保留在同节并标注为「已被实测推翻」
* notepad 用例**无效**：Win11 下 `Start-Process -PassThru` 返回的是 AppX 包应用启动器 PID，`taskkill` 报 `not found`，属包应用启动器特性，**不能读作「GUI 进程无法优雅关闭」**
* `netstat` 中文系统列头形态仍未实测（解析不依赖列头文字，风险可控）
