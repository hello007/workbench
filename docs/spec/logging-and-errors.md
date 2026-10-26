# 日志与错误处理规范

> 后端 slog 结构化日志 + AppError 统一错误处理契约。记录日志库选型、ErrorFormatter 跨层错误码协议、前端 handleError 分流机制、web_audit.log 审计事件表。
> 最后更新：2026-10-26 · 来源任务：09-12-global-error-logging；审计事件表（conn_kick）：10-24-connection-kick-audit-and-kicked-end-notice；读侧 API 与 actor 归因（§8.1）：10-24-actor；token_ban/token_unban 审计行与 E_TOKEN_REVOKED：10-24-connection-ban；ip_ban/ip_unban 审计行（actorSource 扩 system）：10-26-10-26-connection-auto-ban

---

## 1. 适用范围

- 后端日志记录（替换散落 `println`）
- service 层错误返回需前端分流提示的场景
- 前端 catch 块错误提示统一
- 新增错误码 / 新增日志点

## 2. 背景

原后端日志全用 `println("Error:", err.Error())` 散落 44 处，无级别无落盘，GUI 双击启动时 stdout 不可见致崩溃无据可查。service 错误 `fmt.Errorf` 中文直返无统一类型，前端 `gitError.js` 按中文字符串子串匹配脆弱（改文案即失效）。

调研（[research/logging-approach.md](.trellis/tasks/09-12-global-error-logging/research/logging-approach.md)）结论：slog（标准库）+ lumberjack 胜出；自定义 AppError + Wails ErrorFormatter 原生支持结构化 error 传递（源码核实 `CallbackMessage.Err any` + `ErrorFormatter func(error) any`）。

## 3. 日志契约

### 3.1 日志器

- `util.InitLogger(logDir, isDev)` 创建 slog JSONHandler + lumberjack.Writer
- 落盘 `data/logs/app.log`，按 5MB 轮转，保留 5 份，压缩，上限约 25MB
- dev 模式（`version=="dev"`）`io.MultiWriter` 加 stdout，prod 仅文件
- `NewAppServices` 顶部 `service.SetLogger(slog.Default())` 注入

### 3.2 service 包级日志器

- `service/logger.go`：`var appLogger *slog.Logger` + `SetLogger(l)` + `Logger()` helper
- `Logger()` 未注入时回退 `slog.Default()`，保证单测与启动早期不 panic
- service 包内用 `Logger().Warn(...)` / `Logger().Error(...)`，不引 slog 包
- App 层（main 包）用 `slog.Error(...)` / `slog.Info(...)` 直接调默认 logger

### 3.3 日志级别

| 级别 | 场景 |
|---|---|
| `slog.Debug` | 调试细节 |
| `slog.Info` | 启动/关停等关键流程（`workbench started` / `workbench shutting down`） |
| `slog.Warn` | 降级兜底（如 Obsidian 注册表读失败降级直接打开） |
| `slog.Error` | 错误，带 `err` 字段与业务上下文（`path`/`repo` 等） |

### 3.4 结构化字段

- 必带业务上下文字段：`slog.Error("clone failed", "repo", name, "err", err)`
- 禁止 `slog.Error(err.Error())` 裸字符串（丢失结构化字段）
- 禁止 `println`（无级别、无落盘、GUI 不可见）

## 4. 错误处理契约

### 4.1 AppError 类型

`model/app_error.go` 定义：

```go
type AppError struct {
    Code    string  // 机器可读，如 "E_GIT_IN_PROGRESS"
    Message string  // 用户可读中文
    Err     error   // 根因，支持 errors.Is/As 穿透
}
```

- `Error()` 返回 `[CODE] message[: root]` 供日志与兜底
- `Unwrap()` 暴露根因，`errors.Is/As` 穿透包装链
- `NewAppError(code, message)` 无根因（纯业务拒绝）
- `WrapAppError(code, message, err)` 带根因

### 4.2 Wails ErrorFormatter 跨层协议

**源码核实**（Wails v2.12）：
- `internal/frontend/dispatcher/calls.go:74` `CallbackMessage.Err` 类型 `any`（非 string）
- `calls.go:53-54` 注册 `d.errfmt` 时调 `d.errfmt(err)` 返 `any`，`json.Marshal` 后传前端
- `pkg/options/options.go:69` `ErrorFormatter func(error) any` 选项

`main.go` 注册 `ErrorFormatter: formatAppError`：
- `AppError` → `{"code": "E_GIT_IN_PROGRESS", "message": "..."}`
- 普通 error → `{"message": "..."}`

前端 catch 拿到**对象**（非 Error 实例），`error.code` / `error.message` 直接可读。

### 4.3 错误码常量表

`model/app_error.go` 维护错误码常量，命名规范 `E_<域>_<动作/状态>`：

| 常量 | 值 | 域 | 提示级别 |
|---|---|---|---|
| `ErrCodeGitInProgress` | `E_GIT_IN_PROGRESS` | Git | warning（预期拒绝，用户重试） |
| `ErrCodeGitCancelled` | `E_GIT_CANCELLED` | Git | warning（用户取消操作，中性提示非 error；2026-09-28 取消链路新增，`ErrOperationCancelled` sentinel 承载，`mapCancelError` 统一映射） |
| `ErrCodeGitLocked` | `E_GIT_LOCKED` | Git | warning（`.git/index.lock` 被占用或残留，写操作被拒；2026-09-29 锁残留收口新增，`util.parseGitLockError` 在 `GitCommand.run`/`clone` 失败分支识别 git stderr `Unable to create '<path>': File exists` 特征构造，message 含锁路径与人工删除引导；禁自动删——残留不能断定 stale，同仓可能有用户手动 git 进程合法持锁） |
| `ErrCodeGitTimeout` | `E_GIT_TIMEOUT` | Git | warning（Git 操作超时被终止，用户可重试；2026-09-29 超时收口新增，`GitCommand.run`/`clone` 的 `DeadlineExceeded` 分支在锁残留告警后**立即返回** `model.WrapAppError(E_GIT_TIMEOUT, "Git 操作超时已终止，请重试", ctx.Err())`，不再落入退出码白名单与锁 parse——超时被杀 exit 1 曾被 `conflictExitCodes={1}` 吞为「冲突正常返回」致冲突面板假态，且被杀子进程 stderr 半截不可信不参与锁识别；2026-09-30 覆盖范围扩至主包 `streamCommitLog`（Start/读循环/Wait 三处错误出口同构判定，读循环分支超时 err 在 `finish()` 先 cancel 后 Wait 之前捕获，不依赖 cancel 时序）） |
| `ErrCodeSearchTimeout` | `E_SEARCH_TIMEOUT` | 内容搜索 | warning（内容搜索超时被终止，可缩小范围后重试；2026-09-30 超时收口新增，`ContentSearchService.searchWithRipgrep` 失败分支**先判 `ctx.Err()`** 再走 exit 1「无匹配」——rg 超时被杀 Windows TerminateProcess exit 1 恰命中无匹配分支、Unix signal kill exit -1 落入失败分支，两平台均须先判 ctx 才能区分超时与真空结果；`ContentSearch` 多目录聚合：rg 超时禁回退 `searchWithGo`（已超时 ctx 下回退立即空返二次掩盖），Go 回退路径经调用后 `ctx.Err()` 判定对称收口（不改其签名，超时部分结果丢弃），全部目录超时且结果为空返本码，已有非空结果返结果 + slog.Warn（Wails error 返回与结果互斥，报错会丢已有结果）） |
| `ErrCodeDiffToolNotConfigured` | `E_DIFF_TOOL_NOT_CONFIGURED` | 外部 diff 工具 | warning（引导去设置） |
| `ErrCodeDiffToolLaunchFailed` | `E_DIFF_TOOL_LAUNCH_FAILED` | 外部 diff 工具 | error |
| `ErrCodeRepoConfigInvalidJSON` | `E_REPO_CONFIG_INVALID_JSON` | 仓库列表配置导入导出 | error（导入文件非法 JSON/顶层结构缺失） |
| `ErrCodeRepoConfigUnsupportedVersion` | `E_REPO_CONFIG_UNSUPPORTED_VERSION` | 仓库列表配置导入导出 | error（manifestVersion 缺失或高于支持） |
| `ErrCodeAssetsCancelled` | `E_ASSETS_CANCELLED` | 素材管理 | warning（素材扫描被用户取消，中性提示非 error；2026-10-13 素材管理 MVP 新增，`ErrAssetsCancelled` sentinel 承载，`mapAssetsCancelError` 统一映射，对齐 `E_GIT_CANCELLED` 模式） |
| `ErrCodeAssetsLibNotFound` | `E_ASSETS_LIB_NOT_FOUND` | 素材管理 | error（素材库不存在或已删除、前端列表过期，引导刷新库列表） |
| `ErrCodeAssetsScanInProgress` | `E_ASSETS_SCAN_IN_PROGRESS` | 素材管理 | warning（该素材库已有扫描进行中，本次拒绝，可稍后重试；`tryLockScan` TryLock 互斥拒绝时返回，`DeleteLibrary` 复用同锁与在途扫描互斥） |
| `ErrCodeTokenRevoked` | `E_TOKEN_REVOKED` | 浏览器访问通道连接管控 | warning（封禁已撤销令牌被拒：撤销即凭据失效封禁无拒连对象，引导刷新列表；2026-10-24 连接管控四期新增，`WebTokenService.Ban` 校验分支返回） |
| `ErrCodeRPCBadRequest` | `E_RPC_BAD_REQUEST` | RPC 协议层（serve 浏览器模式） | error（请求体非法/方法不允许） |
| `ErrCodeRPCMethodNotFound` | `E_RPC_METHOD_NOT_FOUND` | RPC 协议层（serve 浏览器模式） | error（方法不存在或未导出） |
| `ErrCodeRPCMethodUnsupported` | `E_RPC_METHOD_UNSUPPORTED` | RPC 协议层（serve 浏览器模式） | error（签名形态不受支持，如变参） |
| `ErrCodeRPCArgCountMismatch` | `E_RPC_ARG_COUNT_MISMATCH` | RPC 协议层（serve 浏览器模式） | error（参数个数不匹配） |
| `ErrCodeRPCArgTypeMismatch` | `E_RPC_ARG_TYPE_MISMATCH` | RPC 协议层（serve 浏览器模式） | error（参数类型不匹配） |
| `ErrCodeRPCInternal` | `E_RPC_INTERNAL` | RPC 协议层（serve 浏览器模式） | error（非 AppError 通用内部错误） |

RPC 协议层错误码不构成 AppError（传输协议错误而非业务域错误），取值唯一来源为 `model/app_error.go` RPC 协议段常量表；`server/rpc.go` 以别名引用，前端 `src/transport/rpc.js` 在浏览器模式下按 code 透传给 `handleError` 分流（401 不透传，由 transport 层 token 输入门消化后重试）。

**新增错误码须同步**：本表 + `frontend/src/utils/error.js` `ErrorCode` + `WARNING_CODES` + 前端 `codeMap`（如有）。

**Git 超时错误格式变更（2026-09-29 收口，有意推翻）**：`GitCommand.run`/`clone` 超时（`DeadlineExceeded`）错误从此前的裸错误 `git [args] failed: <stderr>`（杀进程后 stderr 常为空，呈现为无引导的 `git [pull --rebase] failed:` 空摘要）改为 `E_GIT_TIMEOUT` 引导级 AppError。非超时失败路径（含 `parseGitLockError` 锁识别）格式逐字不变。变更动机：超时被杀子进程在 Windows 下 exit code 1 恰命中 `conflictExitCodes={1}`，白名单分支曾返回 `(stdout, nil)` 把超时误判为「冲突正常返回」，service 冲突类方法进入冲突流程后 `GetConflictState` 探不到冲突，前端冲突面板卡空白假态且超时静默丢失。

**streamCommitLog 超时收口（2026-09-30 补齐）**：主包 `commit_history_cli.go` 的 `streamCommitLog`（提交历史流式采集，独立构造 git log 子进程，30s 超时与 util 缺省对齐）同步收口：Start/读循环 readErr/Wait 三处错误出口均判 `ctx.Err() == context.DeadlineExceeded`，命中返回同一 `E_GIT_TIMEOUT` AppError——此前超时杀进程后管道断，呈现为裸错误「读取 git log 输出失败: read |0: file already closed」或「git log failed: exit status ...」。与 util 分叉点：git log 只读不写 index，无 `index.lock` 残留风险，不做锁残留探测。非超时错误格式（「启动 git log 失败」「读取 git log 输出失败」「git log failed: %w, stderr: %s」）与解析错误路径逐字不变；提前终止（handle 返 false）属正常控制流返 nil 不变。留笔：service 内容搜索超时（app_search.go 60s/10s）为非 git 域不复用本码，已于 2026-09-30 独立收口为 `E_SEARCH_TIMEOUT`（见下节）。

**内容搜索超时收口（2026-09-30）**：`ContentSearchService` 内容搜索超时此前静默吞错——`searchWithRipgrep` 的 `cmd.Run()` 失败分支判 `ExitCode() == 1` 返 `nil, nil`（无匹配），rg 超时被杀 Windows TerminateProcess exit 1 恰命中该分支，超时表现为「搜索无结果」假态。收口三点：(1) `searchWithRipgrep` 失败分支最前判 `ctx.Err() != nil`（先于 exit code 判定，同时覆盖 Windows exit 1 与 Unix signal kill exit -1 两平台）返 `E_SEARCH_TIMEOUT` 引导级 AppError（`WrapAppError` %w 包装保留 `errors.Is(ctx.Err())` 穿透）；ctx 未超时的 exit 1 无匹配语义逐字不变，exit 2 及其他错误 `rg failed` 格式不变。(2) `ContentSearch` 循环分流：rg 超时**禁回退** `searchWithGo`（共享 ctx 已超时，回退立即空返二次掩盖超时）；`searchWithGo` 无错误返回（Walk 内 `ctx.Done() → SkipAll` 消费 ctx 但超时返部分结果），经调用后 `ctx.Err()` 判定对称收口，不改其签名（7 处既有用例零波及），超时部分结果丢弃计超时。(3) 多目录聚合：共享 ctx 一旦超时所有在途目录同时超时（无「部分超时其余继续搜」形态）；全部目录超时且结果为空返 `E_SEARCH_TIMEOUT`，已有非空结果返结果 + slog.Warn（Wails error 返回与结果互斥，报错会丢已有结果）。前端 `useCommandPalette.executeContentSearch` catch 原静默吞错，改走 `handleError` 分流（E_SEARCH_TIMEOUT 命中 WARNING_CODES 弹 warning 引导重试，真失败弹 error——附带行为增强：非超时真失败此前也无任何提示）。测试注入：预取消 ctx 驱动（`cmd.Run` 在 Start 前即返 ctx 错误，rg 二进制是否在 PATH 不影响覆盖），无匹配回归用例依赖真实 rg 缺失则 t.Skip。

**部分超时前端感知（2026-09-30 补齐，TimedOut 标志组）**：收口留笔「已有非空结果返结果 + slog.Warn 前端零感知」落地——`model.ContentSearchGroup` 加 `TimedOut bool`（`json:"timedOut,omitempty"`，true 时 Items 恒空）；goroutine 超时分支 append 进**独立 `timedOutGroups` slice**（禁直接混入 `groups`，否则超时且无正常结果组态 `len(groups)==0` 报错判定失效误走返结果路径）；聚合抽 `mergeTimeoutGroups` 纯函数（三态：无超时透传 / 存在超时目录且正常结果组为空时报错——判定条件与收口前逐字一致，含部分目录正常完成但零命中不产组的混合形态 / 部分超时标志组尾部合并，合并走新切片拷贝不原地扩展调用方底层数组），slog.Warn 改带超时目录名数组（原仅数量难对号）。前端零签名变更：Wails 方法签名不动，仅 `models.ts` `ContentSearchGroup` class 补 `timedOut` 字段（App.js/App.d.ts 零改动）；`CommandPalette.vue` 对标志组渲染组级 warning 行「此目录搜索超时，结果可能不完整」（持久可见、与超时目录对号入座，弃 ElMessage 3 秒消失方案）；`group.timedOut` undefined falsy 向后兼容旧数据形态。测试：部分超时端到端因共享 ctx 结构性不可确定性构造（ctx 一超时所有在途目录同超时），由 `mergeTimeoutGroups` 三态单测 + 组件级标志组渲染用例覆盖；`wails-mock-defaults.js` 补 `ContentSearch: []` 默认值。

**多超时目录批量重试前端编排（2026-10-08，承接 fc5c733 单目录重试 MVP）**：单目录重试 MVP 仅 warning 行逐个点重试，多目录同时超时（全局搜索 5 大目录 3 超时）体验差。批量重试走**路径 B：前端编排 N 个单目录 `ContentSearchDirs` 调用并发 + 前端信号量 3 限流**，非一次调 `ContentSearchDirs(allDirs, allRepoNames)`——后者经 service 层 `ContentSearch` 共享 60s ctx，一目录超时全超时，不满足「每目录独立 60s ctx 互不阻塞」；路径 B 每目录独立一次后端调用 = 独立 60s ctx，service 层零改动（`mergeTimeoutGroups`/`searchWithRipgrep`/`searchWithGo` 不动），只加前端编排层。抽 `retryOneDir(group, version)` 核心函数（调 `ContentSearchDirs` + 原位 splice 替换标志组 + 版本号守卫），单目录入口 `retryTimedOutGroup` 与批量入口 `retryAllTimedOutGroups` 共用。

**搜索版本号守卫**：`searchVersion` ref，`executeContentSearch`/`open`/`close`/`openWithContentSearch` 自增（与 `retryingRepoPaths` 清空 + `resetBatchRetry` 同步）。批量编排闭包捕获起始版本号 `version`，`retryOneDir` 在 `await ContentSearchDirs` 写回前比对 `version !== searchVersion.value` 丢弃结果（防改词重搜/关闭面板后旧请求污染新 `contentGroups`）；worker `while` 循环开头版本失效提前 `return` 终止（防版本失效后继续 `cursor++` 取下一批调 `ContentSearchDirs` 永挂 pending）；`finally` 内版本失效时不更新 `retryingRepoPaths`/`batchRetryDone`（保留 `batchRetrying` 复位由 `executeContentSearch` 的 `resetBatchRetry` 兜底）。单目录重试 `retryTimedOutGroup` 内部改调 `retryOneDir` 顺带获版本守卫（既有改词重搜污染漏洞顺带修，行为增强非破坏）。

**批量启动预占 `retryingRepoPaths`**：`retryAllTimedOutGroups` 启动时一次性加入全部目标目录 `repoPath`——排队中目录 warning 行也显「重试中...」（`isGroupRetrying` 天然覆盖批量态），且防用户手动点单目录重试与批量编排竞态（`retryTimedOutGroup` 的 `retryingRepoPaths.includes` 检查挡住）。`batchRetrying`/`batchRetryTotal`/`batchRetryDone` ref 驱动 banner 进度态（进行中「重试中 (done/total)...」+ 按钮 disabled + Loading 图标；静态「检测到 N 个目录搜索超时」+「重试全部」；完成后全成功 banner 消失，有剩余转「重试剩余」可再轮重试）。banner 显示条件 `batchRetrying || timedOutCount >= 2`（<2 走单目录 warning 行避免冗余）。

**测试**：composable 9 用例（全部成功原位替换 / 部分成功部分再超时分流 / 预占 retryingRepoPaths 防手动重复 / 并发上限 3 maxInFlight / 改词重搜版本守卫丢弃 / 关闭面板版本守卫丢弃 / 重复触发被拒 / <2 防御 / 多轮重试）+ 组件 3 用例（banner ≥2 显示与 <2 不显示 / 点击触发全部成功 banner 消失 / 进行中进度态按钮禁用）。mock 构造：前 3 次 `ContentSearchDirs` 返回 pending resolver 捕获中间态，第 4+ 次返 `Promise.resolve([])` 防第二批 worker 永挂。

### 4.4 迁移范围

- **已迁移**：git 域 `ErrOperationInProgress`（`*model.AppError`，`errors.As` 识别 + 文案兜底）
- **保留现状**：obsidian 域（4 方法返回 string 状态码非 error 路径，转 AppError 须改签名触前端，收益低风险高）
- **普通错误**：`fmt.Errorf("ctx: %w", err)` 保留，经 ErrorFormatter 默认 `{message}` 处理

## 5. 前端契约

### 5.1 handleError helper

`frontend/src/utils/error.js`：

```javascript
import { handleError } from '@/utils/error'
catch (error) {
  handleError('提交失败: ', error) // code 命中 WARNING_CODES → warning，否则 error
}
```

- `extractError(error)`：优先读对象 `code`/`message`，兼容裸 Error（无 code 走 message）
- `isWarningError(code, message)`：code 命中 `WARNING_CODES` 或 GitInProgress 文案兜底
- `handleGitError` 是 `handleError` 的 git 域别名（兼容旧调用点）

### 5.2 兼容性

- `gitError.js` 重导出 `error.js` 的 `handleGitError`/`isGitOperationInProgress`/`handleError`/`ErrorCode`
- 7 组件（LocalChanges/GitMerge/GitTags/GitRemotes/GitBranches/GitSubmodules/ContentPanel）import 路径不变
- 新代码直接 `import from '@/utils/error'`

### 5.3 Wails 调用错误桥（v2.16 runtime 回归适配，任务 10-14-srs-2-2-4）

**wails 升级须回归验证 ErrorFormatter reject 链路**。v2.16 runtime（`internal/frontend/runtime/desktop/calls.js`）对失败应答统一 `new Error(message.error)` 后 reject——`message.error` 经 JSON.parse 必为普通对象，永不满足 `instanceof Error`，`new Error(对象).message = String(对象) = "[object Object]"`，`{code, message}` 结构化错误被压扁：全应用 code 分流（WARNING_CODES warning/error）与可读消息同时失效。v2.12 行为是 `reject(message.error)` 原对象透传，本项目 error.js 契约基于该语义。上游自带测试只钉字符串错误形态（issue #4379 面）。

适配层 `frontend/src/utils/wails-call-error-bridge.js`（`main.js` 安装，早于任何绑定调用）：包装 `window.wails.Callback`，应答 error 为结构化对象时按 runtime 同款收口协议（`clearTimeout` + `delete callbacks[id]`）直接以原对象 reject pending 调用（恢复 v2.12 语义）；字符串错误/成功应答/非法 JSON/未注册回调原样委托 runtime。单点覆盖全部绑定方法，禁在各调用点单补。浏览器模式仅 polyfill `window.go`/`window.runtime` 不触碰 `window.wails`，桥 no-op。`extractError` 另有防御兜底：message 缺失/非字符串的对象形态走 `JSON.stringify`（循环引用退 `String()`），任何路径不再产出 `[object Object]`。

**wails 未来版本升级检查项**：核对 runtime desktop calls.js 的 reject 形态是否仍为结构化对象透传（桥若失效应静默 no-op 不叠加破坏）；升级后真机跑一次触发后端 error 的路径（如不存在的库 ID 调用）验证 toast 文案非 `[object Object]` 且 code 分流正确。

## 6. 新增错误码步骤

1. `model/app_error.go` 加常量 `ErrCodeXxx = "E_域_动作"`
2. service 层 sentinel 改为 `model.NewAppError(ErrCodeXxx, "中文消息")` 或用 `WrapAppError` 包装根因
3. `frontend/src/utils/error.js` `ErrorCode` 加映射 + `WARNING_CODES` 加 code（若预期拒绝类）
4. 前端调用点用 `handleError(prefix, error)` 替代裸 `ElMessage.error`
5. 跑 `wails generate module` 确认绑定零 diff（ErrorFormatter 是 options 非签名变更）
6. 跑 `npm test` + `go test ./... -race`

## 7. 反例

### 错误日志

```go
// ❌ println 无级别无落盘，GUI 不可见
println("Error:", err.Error())

// ❌ 裸字符串丢失结构化字段
slog.Error(err.Error())

// ✅ 带级别与字段
slog.Error("clone failed", "repo", name, "err", err)
```

### 前端错误处理

```javascript
// ❌ 裸 ElMessage 无法区分预期拒绝与真失败
catch (error) {
  ElMessage.error('提交失败: ' + (error.message || error))
}

// ❌ 中文字符串子串匹配，改文案即失效
if (error.message.includes('该仓库有 Git 操作进行中')) { ... }

// ✅ handleError 按 code 分流
catch (error) {
  handleError('提交失败: ', error)
}
```

## 8. 审计事件表（web_audit.log）

`service.AuditLogger` 落盘 JSONL（`data/web_audit.log`，O_APPEND 5MB 轮转 `.1`，豁免原子写口径；**禁落明文 token 与完整 URL**）。全部写入收敛 `write` 单点，两个出口：`Log`（认证事件经 server OnAuth 回调透传、token 生命周期经 WebTokenService）与 `LogConnKick`（连接管控动作，App 层专用）。**时序纪律：动作生效后才写**——hub 命中才落 conn_kick（miss/停机不写）、token 事件在 SaveJSON 成功后发（落盘失败回滚不留已发审计行）。

| 事件 | 产出点 | 关键字段 |
|---|---|---|
| `auth_success` / `auth_failure` | server 认证点（HTTP requireToken / WS 握手） | tokenId（失败为空；**封禁拒连形态非空**——凭据有效但封禁生效中，认证未放行不发 auth_success、不种会话 cookie，连接管控四期） / remoteAddr / userAgent / ts |
| `token_create` / `token_revoke` / `token_migrate` | WebTokenService 生命周期操作 | tokenId / name / ts |
| `token_rotate` | App 轮换编排完成（创建/撤旧明细由服务层各自落盘） | tokenId / name / ts |
| `token_ban` | WebTokenService.Ban 生效（含落盘成功）后（连接管控四期；操作者归因由 App 层 `BanWebToken`/`rpcAuditTarget` 透传） | tokenId / name / **actorSource（`desktop`\|`rpc`）+ actorTokenId（RPC 调用者令牌记录 ID，桌面空不落键）** / reason（`omitempty`，封禁备注未填不落键）/ expiresAt（封禁到期 unix 秒，0=永久）/ ts；落盘失败回滚不生效不写 |
| `token_unban` | WebTokenService.Unban 生效后（解封删除封禁记录；未封禁 no-op 未改变状态不写） | tokenId / name / actorSource + actorTokenId（同上归因口径）/ ts |
| `conn_kick` | App.KickConnection 命中后（连接管控二期；三期补操作者归因） | tokenId / name（名册映射，miss 空串）/ remoteAddr（来源 IP，明细快照剥端口口径）/ connId（`AuditRecord.ConnID`，`json:"connId,omitempty"` 仅本事件携带）/ **actorSource（`desktop`\|`rpc`）+ actorTokenId（RPC 调用者令牌记录 ID，桌面空不落键；与 TokenID 正交——TokenID 恒为被踢目标侧，2026-10-24 三期）** / ts |
| `ip_ban` | `IPBanService.RecordFailure` 达阈值在封禁写入内存后（连接管控五期，2026-10-26） | **ip（来源地址，RemoteAddr 剥端口归一形态，`AuditRecord.IP` `omitempty` 仅 ip_ban/ip_unban 携带——与 remoteAddr 字段互不复用，该字段语义为认证事件原始地址含端口）** / **failCount（触发时滑动窗口失败计数，`omitempty`）** / expiresAt（封禁到期 unix 秒）/ reason（「N 分钟内认证失败 M 次，自动临时封禁」）/ **actorSource 恒 `system`（服务端自动触发无人类操作者，actorTokenId 恒空）** / ts；纪律对齐全表——封禁写入内存后才写，无明文 token 无完整 URL |
| `ip_unban` | `IPBanService.UnbanIP` 实际解除后（连接管控五期，2026-10-26） | ip / **actorSource（`desktop`\|`rpc`）+ actorTokenId（RPC 调用者令牌记录 ID，桌面空不落键）**——桌面明细面板路径 desktop/空，浏览器 RPC 路径经 `rpcAuditTarget` override `UnbanIP` → 未导出 `unbanIPAs` 捕获调用者令牌 ID（对齐 unbanWebTokenAs 四期先例；`UnbanIP` 为动作面同构可达，`ListIPBans` 数据面仍在 RPCHiddenMethods 屏蔽）；幂等 no-op 未改变状态不写——无封禁记录/仅失败计数条目均不构成解封 / ts；`ClearAll` 联动清零不写（预防性动作非处置，对齐撤销联动清理口径） |

新增审计事件的步骤：`service/audit_log.go` 加 `AuditEvent*` 常量 → 经 `write` 单点新增专责出口方法（或复用 `Log`）→ 若新增结构化字段则扩 `AuditRecord`（`omitempty` 保证既有事件行形态不变）→ 本表登记。`AuditLogger` 为旁路观测面：未装配（nil）静默跳过，不阻断主链路。

### 8.1 读侧 API（QueryAuditLog，2026-10-24 连接管控三期）

`AuditLogger.QueryAuditLog(AuditQueryOptions) (AuditQueryResult, error)`：过滤（`Events []string` 白名单 / `From`/`To` unix 秒含边界 0=不限 / `TokenID` 命中 `TokenID` **或** `ActorTokenID` 任一即相关——同一令牌可为事件主体、认证者或踢出操作者）→ 合并读 `<path>.1`（旧）+ 主文件（新）→ 时间倒序分页（`Offset`/`Limit`，缺省 100、上限钳 500，`Records` 恒非 nil，`Total` 为过滤后总数）。**快照语义**：打开-读-关闭不持写锁（与 O_APPEND 单行原子追加并发安全）；读期间轮转 rename 窗口可能重复/漏读边界单行，可接受偏差（观测面非正确性依赖）；坏行（非法 JSON / 缺 event 字段）跳过——读方容错，写侧不保证行完整性。**消费面与隐私红线**：唯一合法消费面为桌面 App 绑定 `QueryAuditLog`（前端设置页「审计日志」弹窗）；浏览器通道经 `server.WebOptions.RPCHiddenMethods` 屏蔽（呈现「方法不存在」，见 browser-channel.md §2.2）——新增纯桌面数据面方法须同步追加该屏蔽列表，漏加 = 全谱 IP/令牌明细经同构 RPC 面外泄。

操作者归因链路（写侧）：桌面路径 `KickConnection` 恒 `kickConnectionAs(connID, "desktop", "")`；浏览器 RPC 路径经 `requireToken` ctx 注入 + `rpcAuditTarget` per-request 装饰 override 捕获调用者令牌 ID（见 browser-channel.md §2.2/§2.3）。

## 9. 相关文档

- [cross-layer-contracts.md](cross-layer-contracts.md) — ErrorFormatter 属 options 非签名变更，Wails 绑定零 diff
- [app-services-assembly.md](app-services-assembly.md) — logger 在 NewAppServices 注入
- [test-coverage-gate.md](test-coverage-gate.md) — service ≥76% + 前端 ≥70% 门禁
- [research/logging-approach.md](.trellis/tasks/09-12-global-error-logging/research/logging-approach.md) — 选型调研
