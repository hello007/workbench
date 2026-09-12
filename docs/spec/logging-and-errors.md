# 日志与错误处理规范

> 后端 slog 结构化日志 + AppError 统一错误处理契约。记录日志库选型、ErrorFormatter 跨层错误码协议、前端 handleError 分流机制。
> 最后更新：2026-09-13 · 来源任务：09-12-global-error-logging

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

**新增错误码须同步**：本表 + `frontend/src/utils/error.js` `ErrorCode` + `WARNING_CODES` + 前端 `codeMap`（如有）。

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

## 8. 相关文档

- [cross-layer-contracts.md](cross-layer-contracts.md) — ErrorFormatter 属 options 非签名变更，Wails 绑定零 diff
- [app-services-assembly.md](app-services-assembly.md) — logger 在 NewAppServices 注入
- [test-coverage-gate.md](test-coverage-gate.md) — service ≥76% + 前端 ≥70% 门禁
- [research/logging-approach.md](.trellis/tasks/09-12-global-error-logging/research/logging-approach.md) — 选型调研
