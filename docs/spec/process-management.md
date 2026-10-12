# 进程管理契约（工具箱「进程管理」）

进程枚举与终止的跨层契约：Wails 绑定方法签名、`model.ProcessInfo` 字段语义、`E_PROCESS_*` 错误矩阵，以及**Windows 优雅停止的真实语义**（本机实测更正，推翻了「有无窗口决定能否优雅」的常见结论）。

适用范围：改动 `util/process*.go` / `service/process.go` / `app_process.go` / `ProcessManagerDialog.vue`，或新增依赖进程枚举能力的任何功能。

---

## 1. Scope / Trigger

触发代码规范深度的场景：

- 新增或修改 Wails 绑定方法（`ListProcesses` / `StopProcess` / `KillProcess`）
- 改动 `model/` 下 `ProcessInfo` / `ProcessPort` / `ProcessStopResult` 的导出字段（须同步 `frontend/wailsjs/` 三处）
- 新增 `E_PROCESS_*` 错误码（须同步两处）
- 改动进程枚举解析（`netstat` / `tasklist` / `/proc`）或终止链路（taskkill / SIGTERM）

---

## 2. Signatures

### Wails 绑定方法（`app_process.go` 委托 → `service.ProcessService`）

| 方法 | 签名 | 说明 |
|---|---|---|
| `ListProcesses` | `(keyword string, mode string) ([]model.ProcessInfo, error)` | 检索进程 |
| `StopProcess` | `(pid int, killTree bool) (*model.ProcessStopResult, error)` | 全自动两级停止 |
| `KillProcess` | `(pid int, killTree bool) error` | 直接强杀 |

> `mode` **必须用普通 `string`**，不得用 `type ProcessFilterMode string` 具名类型。wails generate 不为具名 string 在 models.ts 生成 `export type X = string` 别名，`npm run build` 会报 MISSING_EXPORT，而 vitest 不报——属易漏构建失败。常量定义在 `service/process.go`：`ProcessFilterModeAuto/Port/Name`（值 `auto`/`port`/`name`）。

### util 层平台分叉（`//go:build` 编译期分叉）

| 函数 | 签名 | 语义 |
|---|---|---|
| `ListProcesses` | `(ctx context.Context) ([]model.ProcessInfo, error)` | 平台枚举，已填 `IsSelf`/`IsSystem` |
| `IsSelfProcess` | `(pid int) bool` | 自我保护判据（自身 PID + 父进程链） |
| `RequestProcessStop` | `(pid int, killTree bool) (bool, error)` | 优雅请求；`ok=false` 表示请求**被目标拒绝** |
| `KillProcess` | `(pid int, killTree bool) error` | 强制终止（不带 `/T`，killTree 时才加） |
| `IsProcessAlive` | `(pid int) bool` | 存活探测；拒绝访问时**保守判存活** |

---

## 3. Contracts

### 数据模型（`model/process.go`）

```go
type ProcessPort struct {
    Port      int    `json:"port"`
    Protocol  string `json:"protocol"`  // TCP / UDP
    State     string `json:"state"`     // LISTENING / ESTABLISHED；UDP 无状态时为空串
    LocalAddr string `json:"localAddr"`
}

type ProcessInfo struct {
    Pid      int           `json:"pid"`
    Name     string        `json:"name"`
    Ports    []ProcessPort `json:"ports"`
    MemoryKB int64         `json:"memoryKB"`
    IsSelf   bool          `json:"isSelf"`   // 后端填充：本应用自身 + 父进程链
    IsSystem bool          `json:"isSystem"` // 后端填充：系统关键进程
}

type ProcessStopResult struct {
    Stopped bool   `json:"stopped"`
    Method  string `json:"method"` // ProcessStopMethodGraceful("graceful") / ProcessStopMethodForced("forced")
}
```

**关键契约**：`IsSelf` / `IsSystem` **由后端填充随列表下发，前端只消费不自判**。前端若自行按进程名判定，会与后端判据漂移；且进程名判据本身不可靠（见第 7 节 Wrong vs Correct）。

### 检索 mode 语义

| mode | 输入 | 行为 |
|---|---|---|
| `auto` | 纯数字 | 按端口精确匹配 |
| `auto` | 含非数字 | 按进程名关键字模糊匹配（大小写不敏感），**无端口进程也能命中** |
| `port` | 任意 | 强制按端口；输入非数字时返回空列表（不回退、不报错） |
| `name` | 任意 | 强制按进程名 |
| 任意 | 空 | 列出全部监听态进程（默认视图） |

### 默认视图（空输入）的防御式过滤

`filterListeningProcesses` 的保留规则——**核心是「识别不了就不过滤」**：

| 情形 | 处理 |
|---|---|
| 无端口 | 排除 |
| 存在监听态端口 | 保留，Ports 裁剪为仅监听端口 |
| 状态**全部无法识别**（含空串） | **保留且不裁剪**，避免整体空列表 |
| 状态可识别但都不是监听态 | 排除 |

`netstat` 状态列随系统 UI 语言本地化（中文系统为「监听」等）。若把无法识别的状态一律当「非监听」排除，**默认视图在非英文系统上会退化为空列表**——属严重降级，故以「保留」为安全方向。`isKnownPortState` 维护已知状态集合，落在集合外即视为无法识别。

### 自我保护（三层）

| 层 | 范围 | 表现 |
|---|---|---|
| L1 硬禁 | 本应用自身 PID + 父进程链 | 前端按钮禁用 + 标注「本应用」；**后端终止入口再判一次**（前端可绕过，后端是最终防线）→ `E_PROCESS_SELF_PROTECT` |
| L2 强确认 | 系统关键进程（Windows：System / System Idle Process / Secure System / csrss / smss / lsass / wininit / winlogon / services；Linux：PID 1、内核线程） | 前端红色警告确认（`type: 'error'`），明示蓝屏/重启后果 |
| L3 通用确认 | 其余 | `type: 'warning'`，「确定结束 X（PID n）？未保存的数据将丢失。」强杀文案再加强调 |

### 进程树范围

「停止」「强杀」默认**只终止目标进程本身**（`taskkill` 不带 `/T`）。进程树是弹窗顶部的显式勾选框，默认关闭——与 Windows 任务管理器 / macOS 活动监视器 / htop 一致（杀树是二级选项而非默认）。

既有 `util.KillProcessTree`（带 `/T`）保留给 AI task 取消等原调用方，**未被本功能复用**。

---

## 4. Validation & Error Matrix

| 条件 | 错误码 | 级别 | 前端表现 |
|---|---|---|---|
| 枚举命令执行失败 | `E_PROCESS_ENUM_FAILED` | error | 报错提示 |
| 枚举超时（**先判 `ctx.Err()`** 再判命令退出码） | `E_PROCESS_ENUM_TIMEOUT` | warning | 引导重试 |
| 目标进程已不存在 | `E_PROCESS_NOT_FOUND` | warning | 提示后**自动刷新列表** |
| 权限不足 / 受保护进程 | `E_PROCESS_ACCESS_DENIED` | warning | 「权限不足，需以管理员身份运行」 |
| 拒绝终止自身或父进程链 | `E_PROCESS_SELF_PROTECT` | warning | 中性提示 |
| 强杀失败 | `E_PROCESS_KILL_FAILED` | error | 报错提示 |

**错误码须同步两处**：`model/app_error.go` 常量表 + `frontend/src/utils/error.js` 的 `ErrorCode` 与 `WARNING_CODES`。

**超时收口（必守）**：枚举失败出口先判 `ctx.Err()` 再判命令退出码，对齐 `service/content_search.go` 范式——**超时不得误判为「无结果」**。Windows 上被杀的子进程 exit 1 恰好命中「无匹配」分支，会静默返空。

**stderr 形态识别单点**：`classifyTaskkillResult`（Windows）/ `classifySignalError`（Linux）是唯一出口，对齐 `util/git_lock.go` 的 `parseGitLockError` 范式。识别「not found」「Access is denied」「only be terminated forcefully」的中英双形态。

### 停止编排（`StopProcess` 全自动两级）

1. 判 `IsSelfProcess` → 拒绝（L1）
2. 发优雅请求
3. **请求被目标拒绝**（`ok=false`）→ **直接转强杀，不轮询**，`Method=forced`
4. 请求送达（`ok=true`）→ 轮询 `IsProcessAlive` 上限 **5s**（间隔 300~500ms）：已退出 → `Method=graceful`；超时 → 强杀 → `Method=forced`
5. 停止超时**不产生面向用户错误码**（全自动转强杀），仅后端 `slog.Warn` 记录

---

## 5. Good / Base / Bad Cases

| 级别 | 场景 |
|---|---|
| **Good** | 用户点「停止」→ 进程响应并退出 → 提示「已正常退出」（`graceful`）；列表自动刷新 |
| **Base** | 用户点「停止」→ 进程忽略请求（如 `cmd /k`）→ 立即转强杀 → 提示「已强制结束」（`forced`）。**UI 如实反映，不谎称已优雅退出** |
| **Bad** | 只看 `taskkill` 返回 `SUCCESS` 就报「已正常退出」——`SUCCESS` 仅代表信号送达，延迟退出的进程（刷盘、写日志）此时并未结束，属撒谎 |
| **Bad** | 默认视图按状态过滤时把无法识别的状态一律排除 → 中文系统上列表全空 |
| **Bad** | 前端按进程名 `workbench.exe` 判定自我保护 → E2E 改名 `workbench-e2e.exe` 后保护失效 |

---

## 6. Tests Required

| 层 | 文件 | 断言点 |
|---|---|---|
| util | `util/process_parse_windows_test.go` | `netstat` 按列数分支（TCP 5 列 / UDP 4 列）、IPv6 `[::]:8080` 与 `fe80::1%12:8080` 取末冒号端口、`tasklist` CSV 千分位逗号内存（`2,904 K`）、跳过 `INFO:` 非法行与表头、中文列表头用例 |
| util | `util/process_parse_other_test.go` | `/proc/net/tcp{,6}` 小端地址反转、`/proc/net/udp{,6}` 协议标记 UDP 且状态留空、`status` VmRSS、`stat` PPid、socket inode 解析 |
| service | `service/process_test.go` | 检索三态（端口/进程名/空输入 LISTENING）、模式切换、自我保护拒绝、优雅成功、**超时转强杀**、请求被拒直转强杀、错误码映射、默认视图的未知状态保留 |
| 前端 | `__tests__/ProcessManagerDialog.spec.js` | 列表六列渲染、`isSelf` 行按钮禁用且标注「本应用」、`isSystem` 红色确认（`type: 'error'`）、普通进程通用确认、`graceful`/`forced` 双文案、进程树勾选默认关闭且传参、模式切换、行级 loading 与防重 |

**稳定性约束**（`test-stability.md`）：全部用注入的 fake runner / fixture 目录，**禁依赖真实进程、固定端口（本机常驻 workbench.exe 监听 36115）、mtime、sleep 等待**。时间注入点为 `ProcessService` 的 `now` / `sleep` 字段，测试注入假时钟。

---

## 7. Wrong vs Correct

### 7.1 优雅停止：不要按进程类型预判

**Windows 实测（2026-10-09，本机 Win11）**

| 用例 | 进程 | `taskkill /PID <id>` 输出 | 1 秒后存活 |
|---|---|---|---|
| A | `ping -n 300 127.0.0.1`（`-WindowStyle Hidden`，控制台） | `SUCCESS: Sent termination signal...` | **已退出** |
| B | `cmd /k echo STOP-TEST`（有可见窗口） | `ERROR: ... can only be terminated forcefully (with /F option).` | **仍存活** |

**正确语义**：`taskkill` 不带 `/F` 对控制台进程投递控制信号、对窗口进程投递关闭消息，**能否生效取决于目标进程是否响应**——`ping` 默认终止，`cmd /k` 为保持交互会话主动忽略。不是「有没有窗口」的问题，是「进程买不买账」的问题，**无法事前预判**。

> 常见资料（含本项目调研报告初版）称「有窗口 GUI 进程能优雅关闭、无窗口后台进程没有优雅通道」，**该结论已被上述实测推翻，禁止引用**。

**Wrong**（按类型预判、或以 `SUCCESS` 当成功）：

```go
// 错：以 taskkill 的 SUCCESS 判定已退出
if err := taskkill(pid); err == nil {
    return &model.ProcessStopResult{Stopped: true, Method: "graceful"} // 撒谎
}
```

**Correct**（发请求后轮询实测存活）：

```go
delivered, err := util.RequestProcessStop(pid, killTree)
if !delivered {
    // 请求被目标拒绝：无优雅通道，直接转强杀
    return s.kill(pid, killTree) // Method = forced
}
// 请求送达 ≠ 已退出：轮询实测
for deadline := s.now().Add(5 * time.Second); s.now().Before(deadline); {
    if !util.IsProcessAlive(pid) {
        return &model.ProcessStopResult{Stopped: true, Method: model.ProcessStopMethodGraceful}, nil
    }
    s.sleep(400 * time.Millisecond)
}
return s.kill(pid, killTree) // 超时转强杀，Method = forced
```

### 7.2 自我保护：只能用 PID，禁用进程名

**Wrong**：

```go
if info.Name == "workbench.exe" { /* 保护 */ }  // E2E 会改名 workbench-e2e.exe，保护失效
```

**Correct**：

```go
if util.IsSelfProcess(pid) { // 自身 PID + 父进程链
    return nil, model.NewAppError(model.ErrCodeProcessSelfProtect, ...)
}
```

依据：`docs/spec/e2e-testing.md` —— E2E 把 exe 复制改名 `workbench-e2e.exe` 实现实例隔离。Windows 父进程链覆盖两层（自身 + `os.Getppid()`），Linux 经 `/proc/<pid>/stat` 完整上溯（上限 16 层）。

---

## 8. Windows 解析三条硬约束

1. `netstat -ano` 输出用 `strings.Fields` 拆分后按**列数**分支（TCP 5 列 / UDP 4 列），**绝不依赖列头文字**（系统 UI 语言会本地化）
2. 本地地址端口取**最后一个冒号之后**（兼容 IPv6 `[::]:8080` 与 `fe80::1%12:8080`）
3. `tasklist /FO CSV /NH` 必须用 `encoding/csv` 解析（内存字段含千分位逗号），跳过非法行

**实测耗时**（本机，供性能预期）：`netstat -ano` 421 行 / **0.075s**；`tasklist /FO CSV /NH` 494 进程 / **~1.0s**（枚举链路成本主导项）。

**外部命令必守**：调 `netstat` / `tasklist` / `taskkill` 必须 `util.HideCommandWindow(cmd)` 避免闪黑窗；终止命令必须有超时（当前 3s）——无超时时 `taskkill` 撞上「是否保存」对话框可能永不返回，Wails 绑定调用不返回即前端卡死。

**失败时须拼接 stderr**：用 `cmd.Output()` 会丢弃 stderr，而 taskkill 的失败原因（`not found` / `Access is denied`）很可能只落在 stderr，导致形态识别抓不到特征、边缘态提示退化为笼统失败。须取 `ExitError.Stderr` 拼接后再交给分类函数。

---

## 9. 技术选型：零依赖（不引入 gopsutil）

Windows `netstat -ano` + `tasklist /FO CSV /NH`；Linux 解析 `/proc/net/tcp{,6}`、`/proc/net/udp{,6}`、`/proc/*/fd`、`/proc/*/cmdline`、`/proc/*/status`。

**升级触发条件**：若需展示**命令行 / 用户名 / CPU%**（区分多个同名 `java.exe` 的刚需），Windows 侧自研成本过高（`wmic` 已弃用、PowerShell 冷启动 ~1s），届时直接引入 `github.com/shirou/gopsutil/v4` 并做超时保护（其部分字段走 WMI，慢且可能挂起）。引入后须重跑 `govulncheck ./...`。

---

## 9.1 复用面：工具箱「端口占用」一览（2026-10-12）

端口占用一览（`PortListDialog.vue`）**零后端新增**：直接调 `ListProcesses('', '')`（空关键字默认视图即「仅监听态」，含本地化状态名保守保留语义，见 `filterListeningProcesses`），前端扁平化 `进程 × 端口 → 一行一个端口` 并按端口号升序排序。终止复用 `StopProcess`（两级停止链路）与同一套确认交互（isSelf 禁用 / isSystem 红色强确认 / 通用 warning 确认）。

**后续扩展约束**：同进程多端口行共享 PID——行级防重须按 PID 而非端口（`pendingPids` 语义），否则同进程另一端口行可重复发起终止；确认文案须按端口数提示「终止将连带释放其余端口」（终止单位是进程）。

---

## 10. 相关文档

| 文档 | 关联点 |
|---|---|
| [cross-layer-contracts.md](cross-layer-contracts.md) | wailsjs 三处同步、具名 string 的 MISSING_EXPORT 坑 |
| [logging-and-errors.md](logging-and-errors.md) | 错误码两处同步、超时先判 `ctx.Err()`、形态识别单点范式 |
| [linux-platform.md](linux-platform.md) | `_windows.go` / `_other.go` 编译期分叉、测试按平台拆分 |
| [test-stability.md](test-stability.md) | 禁依赖真实进程 / 固定端口 / sleep |
| [e2e-testing.md](e2e-testing.md) | exe 改名 `workbench-e2e.exe` → 进程名判据不可靠 |
| [frontend-visual-conventions.md](frontend-visual-conventions.md) | 列表类弹窗走 900+ 档（本功能 `min(900px, 85vw)`） |
