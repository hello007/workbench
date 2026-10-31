# 日志与错误处理规范

> 后端 slog 结构化日志 + AppError 统一错误处理契约。记录日志库选型、ErrorFormatter 跨层错误码协议、前端 handleError 分流机制、web_audit.log 审计事件表。
> 最后更新：2026-10-27 · 来源任务：09-12-global-error-logging；审计事件表（conn_kick）：10-24-connection-kick-audit-and-kicked-end-notice；读侧 API 与 actor 归因（§8.1）：10-24-actor；token_ban/token_unban 审计行与 E_TOKEN_REVOKED：10-24-connection-ban；ip_ban/ip_unban 审计行（actorSource 扩 system）：10-26-10-26-connection-auto-ban；ip_ban strike 字段与档位 reason（六期）：10-27-connection-ban-tuning；ip_ban 手动封禁归因分叉（七期）：10-27-ip

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
| `ErrCodeAssetsSplitSourceDeleteFailed` | `E_ASSETS_SPLIT_SOURCE_DELETE_FAILED` | 素材管理 | warning（就地替换拆分成功但源文件无法移入回收站：被 PowerPoint/WPS 占用、只读或权限不足。产物、索引行与标注副本均保留、源文件与源行亦保留，用户关闭占用程序后可重试拆分覆盖产物；2026-10-31 素材拆分 B 方案 PR1 新增，`splitOneFileReplace` 回收站移入失败分支返回，单文件入口升格 AppError、批量入口经 `Item.Code` 透出不整体失败） |
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

`service.AuditLogger` 落盘 JSONL（`data/web_audit.log`，O_APPEND 追加，超限轮转入多档链 `.1`…`.N`（连接管控十一期，默认 6 档×5MB，详见 §8.2），豁免原子写口径；**禁落明文 token 与完整 URL**）。全部写入收敛 `write` 单点，两个出口：`Log`（认证事件经 server OnAuth 回调透传、token 生命周期经 WebTokenService）与 `LogConnKick`（连接管控动作，App 层专用）。**时序纪律：动作生效后才写**——hub 命中才落 conn_kick（miss/停机不写）、token 事件在 SaveJSON 成功后发（落盘失败回滚不留已发审计行）。

| 事件 | 产出点 | 关键字段 |
|---|---|---|
| `auth_success` / `auth_failure` | server 认证点（HTTP requireToken / WS 握手） | tokenId（失败为空；**封禁拒连形态非空**——凭据有效但封禁生效中，认证未放行不发 auth_success、不种会话 cookie，连接管控四期） / remoteAddr / userAgent / ts |
| `token_create` / `token_revoke` / `token_migrate` | WebTokenService 生命周期操作 | tokenId / name / ts |
| `token_rotate` | App 轮换编排完成（创建/撤旧明细由服务层各自落盘） | tokenId / name / ts |
| `token_ban` | WebTokenService.Ban 生效（含落盘成功）后（连接管控四期；操作者归因由 App 层 `BanWebToken`/`rpcAuditTarget` 透传） | tokenId / name / **actorSource（`desktop`\|`rpc`）+ actorTokenId（RPC 调用者令牌记录 ID，桌面空不落键）** / reason（`omitempty`，封禁备注未填不落键）/ expiresAt（封禁到期 unix 秒，0=永久）/ ts；落盘失败回滚不生效不写 |
| `token_unban` | WebTokenService.Unban 生效后（解封删除封禁记录；未封禁 no-op 未改变状态不写） | tokenId / name / actorSource + actorTokenId（同上归因口径）/ ts |
| `conn_kick` | App.KickConnection 命中后（连接管控二期；三期补操作者归因） | tokenId / name（名册映射，miss 空串）/ remoteAddr（来源 IP，明细快照剥端口口径）/ connId（`AuditRecord.ConnID`，`json:"connId,omitempty"` 仅本事件携带）/ **actorSource（`desktop`\|`rpc`）+ actorTokenId（RPC 调用者令牌记录 ID，桌面空不落键；与 TokenID 正交——TokenID 恒为被踢目标侧，2026-10-24 三期）** / ts |
| `ip_ban` | `IPBanService.RecordFailure` 达阈值在封禁写入内存后（连接管控五期，2026-10-26；六期扩 strike 与档位 reason，2026-10-27；**七期复用单事件扩手动封禁归因，2026-10-27**——`AuditRecord` 零新增字段，`LogIPBan` 补 `ActorTokenID` 落键） | **ip（来源地址，RemoteAddr 剥端口归一形态，`AuditRecord.IP` `omitempty` 仅 ip_ban/ip_unban 携带——与 remoteAddr 字段互不复用，该字段语义为认证事件原始地址含端口）** / **failCount（触发时滑动窗口失败计数，`omitempty`；手动封禁恒 0 不落键——无自动触发语境）** / **strike（累计触发次数，六期递增封禁，`omitempty` 仅 ip_ban 携带——ip_unban 无计数语义；手动封禁不携带）** / expiresAt（封禁到期 unix 秒，手动永久封禁为 0）/ reason（自动 = 六期档位化「N 分钟内认证失败 M 次，第 K 次触发，自动临时封禁 X 分钟」；**手动（七期）= 用户输入非空用输入值，为空生成缺省文案「管理员手动封禁（永久）」/「管理员手动封禁，至 X」**）/ **actorSource（`system` = 自动触发恒 actorTokenId 空；`desktop`\|`rpc` = 七期手动封禁——桌面恒 desktop/空，浏览器 RPC 经 `rpcAuditTarget` override `BanIP` 委托未导出 `banIPAs` 捕获调用者令牌记录 ID，对齐 banWebTokenAs 四期先例）** / ts；纪律对齐全表——封禁写入内存且（手动封禁）`data/ip_bans.json` 落盘成功后才写，落盘失败内存回滚不写审计；无明文 token 无完整 URL；**八期网段封禁同事件复用**：`BanCIDR` 落盘成功后写 ip_ban，ip 字段存规范化 CIDR 串（`ipnet.String()`），actor 归因经 `rpcAuditTarget` override `BanCIDR` 委托未导出 `banCIDRAs`（对齐 banIPAs 先例） |
| `ip_unban` | `IPBanService.UnbanIP` 实际解除后（连接管控五期，2026-10-26） | ip / **actorSource（`desktop`\|`rpc`）+ actorTokenId（RPC 调用者令牌记录 ID，桌面空不落键）**——桌面明细面板路径 desktop/空，浏览器 RPC 路径经 `rpcAuditTarget` override `UnbanIP` → 未导出 `unbanIPAs` 捕获调用者令牌 ID（对齐 unbanWebTokenAs 四期先例；`UnbanIP` 为动作面同构可达，`ListIPBans` 数据面仍在 RPCHiddenMethods 屏蔽）；幂等 no-op 未改变状态不写——无封禁记录/仅失败计数条目均不构成解封 / ts；`ClearAll` 联动清零不写（预防性动作非处置，对齐撤销联动清理口径）；**八期网段解封同事件复用**：`UnbanCIDR` 实际解除后写 ip_unban（ip 字段存规范化 CIDR 串，rpcAuditTarget override → `unbanCIDRAs`；miss false 不写） |

新增审计事件的步骤：`service/audit_log.go` 加 `AuditEvent*` 常量 → 经 `write` 单点新增专责出口方法（或复用 `Log`）→ 若新增结构化字段则扩 `AuditRecord`（`omitempty` 保证既有事件行形态不变）→ 本表登记。`AuditLogger` 为旁路观测面：未装配（nil）静默跳过，不阻断主链路。

### 8.1 读侧 API（QueryAuditLog，2026-10-24 连接管控三期）

`AuditLogger.QueryAuditLog(AuditQueryOptions) (AuditQueryResult, error)`：过滤（`Events []string` 白名单 / `From`/`To` unix 秒含边界 0=不限 / `TokenID` 命中 `TokenID` **或** `ActorTokenID` 任一即相关——同一令牌可为事件主体、认证者或踢出操作者）→ 合并读轮转链全部档位（旧，`<path>.N`…`<path>.1`，按当前 depth 构造）+ 主文件（新）→ 时间倒序分页（`Offset`/`Limit`，缺省 100、上限钳 500，`Records` 恒非 nil，`Total` 为过滤后总数）。**快照语义**：打开-读-关闭不持写锁（与 O_APPEND 单行原子追加并发安全）；读写竞速平台分叉（十一期实测，见 §8.2）：Windows 下读打开的档使写侧 rename 失败（写侧停链降级自愈，读侧拿轮转前一致快照），Unix 下 rename 不受阻、读期间轮转窗口可能重复/漏读边界单行——均属可接受偏差（观测面非正确性依赖）；坏行（非法 JSON / 缺 event 字段）跳过——读方容错，写侧不保证行完整性。**消费面与隐私红线**：唯一合法消费面为桌面 App 绑定 `QueryAuditLog`（前端设置页「审计日志」弹窗）；浏览器通道经 `server.WebOptions.RPCHiddenMethods` 屏蔽（呈现「方法不存在」，见 browser-channel.md §2.2）——新增纯桌面数据面方法须同步追加该屏蔽列表，漏加 = 全谱 IP/令牌明细经同构 RPC 面外泄。

操作者归因链路（写侧）：桌面路径 `KickConnection` 恒 `kickConnectionAs(connID, "desktop", "")`；浏览器 RPC 路径经 `requireToken` ctx 注入 + `rpcAuditTarget` per-request 装饰 override 捕获调用者令牌 ID（见 browser-channel.md §2.2/§2.3）。

### 8.2 审计留存多档轮转链（2026-10-28 连接管控十一期）

#### Scope / Trigger

十期趋势面数据源为审计行懒扫，但审计留存 5MB×2（约 6 万行）在认证失败潮下数小时即轮转一轮，`TrendWindowDays` 默认 7 天回看窗口名存实亡。十一期扩轮转链深度：**审计 JSONL 单一数据源不变**，读侧 `readSnapshot` 多档合并，`QueryIPBanTrend` / `QueryAuditLog` 两消费面零改动透明受益。十期 Out of Scope「不做多份轮转链」由本期转正（`webAuditMaxBytes` doc comment 落转正理由）。

#### Signatures

- 轮转（写侧单点）：`rotateLocked(incoming int64)`——持 `a.mu`，超限时链式下移 `for i := depth-1; i >= 1; i--`（源档 Stat 不存在跳过，`os.Rename` 失败即 Warn **停链 return**）后 `rename(path, path+".1")`。Windows 走 MoveFileEx REPLACE_EXISTING 覆盖已存在目标档。
- 读侧（单点）：`readSnapshot() ([]AuditRecord, error)`——**锁内快照 depth 后**构造档位列表 `path.N … path.1 + path`（时间正序拼接，最旧档在前）；depth 经热更缩小后残留超深度旧档**不入读也不删除**（静默滞留磁盘，容量口径以新配置为准）。
- 热更生效点：`(a *AuditLogger) ApplyConfig(maxBytes int64, depth int)`——mu 锁内整体替换两字段；**仅面向未来**（已超限主文件不立即轮转，下次写入达限触发；depth 缩小不追溯清理）。
- 设置面：`model.AuditSettings{MaxBytesMB int, Depth int}`（`AppSettings.Audit *AuditSettings`，`json:"audit,omitempty"` 段缺失全默认）+ `EnsureAuditDefaults` 收编 `AppSettings.EnsureAllDefaults` 串联单点（2026-10-28 单点化：Load miss / Save / GetSettings 降级三处消费一处加行，对齐「现五处 Ensure 手抄」的漏配收口）+ `App.SetAuditConfig(maxBytesMB, depth int) error`（normalize → GetSettings 基底经 `AppSettings.PrivateCopy` 全指针段私有拷贝合并写 → Save → `webAuditSvc.ApplyConfig(MB→字节换算)`，对齐六期 SetIPBanConfig「保存+生效」显式单点，**禁走通用 SaveSettings 隐式联动**）。
- 装配：`NewAppServices` 构造 `webAuditSvc` 后紧随 `settingsSvc.Load()` 归一值 `ApplyConfig` 注入启动初值（对齐 ipBanSvc 六期形态）。

#### Contracts（参数与 clamp）

| 参数 | clamp | 默认 | 语义 |
|---|---|---|---|
| `maxBytesMB` | 1–20 | 5 | 单档字节上限（生产常量 `webAuditMaxBytes` = 5MB 仍为构造缺省） |
| `depth` | 2–10 | 6 | 链档位深度（生产常量 `webAuditMaxRotationDepth`）；总容量 = (depth+1)×maxBytes，默认 35MB、上限 220MB |

无新错误码：clamp 走 `NormalizeAuditSettings` 静默收敛（零值回退默认、越界收敛边界），无失败分支。

#### Validation & Error Matrix（写侧降级口径）

| 形态 | 行为 |
|---|---|
| rename 链任一环失败（含 Windows 读句柄占用 sharing violation） | slog.Warn **停链 return**：后续档保持现状、主→.1 不执行、本行照常追加主文件（超限容忍，下条再试）——链中断只影响该轮可达深度，不丢行不重排 |
| 读侧持档句柄期间轮转（Windows） | rename 源失败停链（Go os.Open 句柄不透出 DELETE 共享语义）；读侧句柄释放后下一行写入重试成功（**自愈闭环**）；`TestAuditLogger_RotateWhileOpenForRead` 在 `audit_log_windows_test.go`（`//go:build windows`）守护 |
| 同形态（Unix） | rename 不受阻，轮转成功；已打开句柄为轮转前一致快照；`TestAuditLogger_RotateWhileOpenForRead_UnixRenameUnblocked` 在 `audit_log_linux_test.go` 对偶守护（**平台耦合断言禁共享一份**，linux-platform 纪律——共享文件断言 Windows 语义在 ubuntu CI 必挂） |
| 链中段缺档（如 `.2` 缺失） | 源档 Stat 不存在跳过该环，其余档各移一位（`.3`→`.4`、`.1`→`.2`），轮转不因空洞中断 |
| 现网存量 `.1`（十期形态） | 首次轮转 `.1`→`.2` 顺移，零迁移脚本 |
| depth≤1（ApplyConfig 直调防御形态，设置面 clamp 已保证 ≥2） | 循环不执行，直接单档 rename（等价旧单档形态），不 panic |

#### Good/Base/Bad Cases

- Good：默认配置（5MB×6+主）下正常认证频度撑满 7 天趋势窗口；失败潮下「数据自 X 起（更早历史已轮转）」提示行按十期既有语义诚实前移（十期前端零改动）。
- Base：段缺失老配置升级 → `EnsureAuditDefaults` 补默认，`SetAuditConfig` 后热更即时生效。
- Bad：手改 settings.json 越界值 → clamp 收敛（运行期手改本就须重启，见 SettingsService 缓存语义）；`SetAuditConfig` 漏拷其他指针段 → 缓存共享指针竞态（四段全部私有拷贝是硬纪律）。

#### Tests Required

- 轮转链：`TestAuditLogger_RotationChain`（depth=2 淘汰最旧/无超深度档/链内时间正序/读侧仅链内行可见）、`_UpgradeFromDot1`（存量顺移）、`_MiddleGap`（缺档不断链）、`TestAuditLogger_ReadSnapshot_DepthTiers`（多档合并时序）、`TestAuditLogger_DepthShrink_LegacyTierIgnored`（残留档不入读不删除）、竞速双侧（见上表）。
- 设置面：`TestAppSettings_EnsureAuditDefaults` / `TestNormalizeAuditSettings`（model）、`TestApp_SetAuditConfig_SaveAndHotApply` / `_ClampsInvalidInput` / `_PreservesOtherSections`（App 层，热更行为断言用注入小值驱动真实轮转——AuditLogger 字段未导出，跨包不可锁内直读）、`TestAuditLogger_ApplyConfig`（service 包内锁内断言）。
- 透明受益：`QueryAuditLog`/`QueryIPBanTrend` 函数体零改动，六~十期测试零回归即过。

#### Wrong vs Correct

**Wrong**：给轮转链加读侧缓存「优化」懒扫——缓存失效必然依赖审计写事件（每行都变文件），与「缓存失效由手动入口驱动不依赖 mtime 时序」test-stability 纪律冲突，且 35MB 毫秒级扫描无必要。

**Correct**：维持懒扫（面板打开时全档合并），容量/性能由 depth×maxBytes clamp 上限兜底（最坏 220MB 毫秒~几十毫秒）。

## 9. Git 操作取消链路——util 层 ctx 变体、注册表模式与覆盖清单

> 本节承接 CLAUDE.md 关键规则「Git 操作取消链路」行的 util 层 API 面与覆盖清单（超时收口两段已见 §4.3 错误码表与上文 2026-09-29/09-30 收口段，不重复）。

### util 层 ctx 变体 API

- `ExecuteWithContext` / `ExecuteWithCodesWithContext` / `CloneWithContext`——内部 `run` 统一实现，取消错误 `%w` wrap `context.Canceled`；超时语义见 §4.3 `E_GIT_TIMEOUT` 行与上文 2026-09-29 超时收口段。

### service 层取消注册表模式

- 取消注册表 `cancels` 与 `opLocks` 同 `opMu` 保护，键经 `normalizeRepoKey` 与锁一致。
- 新增长操作接入取消 = 自建 `context.WithCancel` + `registerCancel`/`unregisterCancel` defer LIFO（先注销后释放锁）+ `mapCancelError` 统一映射 `ErrOperationCancelled`（`E_GIT_CANCELLED`，前端 warning 中性提示）。

### 现有覆盖清单

- Pull / Clone / BatchPull（BatchPull 的 ctx 注册在 sem 排队前，覆盖「排队中未启动仓库」取消）。
- 冲突类 forward 七方法 Merge / Rebase / CherryPick / ContinueMerge / ContinueRebase / ContinueCherryPick / SkipRebase（仅执行步贯穿，precheckMutation/requireInProgress 快命令不注册；util 层对应 7 个 ctx 变体经 conflictExitCodes 保冲突 exit 1 接受语义）。
- **Abort 三类刻意无 ctx 变体**——abort 是回滚终止手段，中途取消可能留半态。

### 取消单测稳定构造

- ctx 预先 cancel（`exec.CommandContext` 对已取消 ctx 在 Start 前返回，无慢命令依赖）。

### 锁残留探测与锁占用识别（2026-09-29 收口）

- `run`/`clone` 取消与超时分支杀子进程后探测 `<gitroot>/index.lock` 残留仅 slog.Warn **禁自动删**（残留不能断定 stale，同仓可能有用户手动 git 进程合法持锁）。
- `findIndexLockResidue` 覆盖 .git 目录与 worktree gitdir 文件两形态；clone 目标 resolveUpward=false 禁向上防误报父仓锁；探测失败静默降级。
- 锁被占用 stderr（`Unable to create '<path>': File exists`）在失败分支识别为 `E_GIT_LOCKED` AppError（`util.parseGitLockError` 单点，桌面 ErrorFormatter 与 serve `rpcFailureFromCall` 双通道零改动自动透传），前端 warning 引导人工确认后删除，非锁残留错误行为逐字不变。

### 素材拆分取消与删源失败提示口径（2026-10-31，B 方案 PR1）

- **取消**：replace 落点三检查点（核心返回取消错误 / 核心返回后 / 删源前）先回滚已产出产物、索引行与缩略图，再返回 `E_ASSETS_CANCELLED`（复用扫描取消码，消息「拆分已取消」）；前端 warning 中性提示，取消按钮自身只提示「已请求取消拆分」，任务终态不弹 error toast。`cache/source` 保持取消保留已产出。
- **删源失败**：`E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`（warning，见 §4.3 表）。单文件入口升格 `AppError` 经 ErrorFormatter 透传 code；批量入口只填 `Item.Code`，不整体失败；文案固定说明「产物已保留，可关闭占用程序后重新拆分」。
- **批量三分类禁按文本匹配**：已删除源 / 已回滚 / 源保留由 `Item.Status + Target + SourceRemoved + RolledBack + Code` 组合判定（前端 `splitOutcomeKind` 单点），message 仅展示。
- **日志级别**：单文件失败 `Logger().Error`；回滚与删源失败 `Warn`；批量内单文件失败 `Warn`（异常隔离继续）。

## 10. 相关文档

- [cross-layer-contracts.md](cross-layer-contracts.md) — ErrorFormatter 属 options 非签名变更，Wails 绑定零 diff
- [app-services-assembly.md](app-services-assembly.md) — logger 在 NewAppServices 注入
- [test-coverage-gate.md](test-coverage-gate.md) — service ≥76% + 前端 ≥70% 门禁
- [research/logging-approach.md](.trellis/tasks/09-12-global-error-logging/research/logging-approach.md) — 选型调研
