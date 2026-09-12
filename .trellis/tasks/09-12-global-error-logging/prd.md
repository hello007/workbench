# 全局错误处理与日志系统

## Goal

WorkBench 后端日志全用 `println("Error:", err.Error())` 散落各处（无级别/无结构/无落盘），service 错误 `fmt.Errorf` 中文消息直返无统一类型。前端错误拦截仅 `gitError.js` 覆盖 git 域 6 组件，其他组件各自 catch。引入结构化日志 + 统一错误处理机制，为桌面应用提供可观测性（崩溃排查 / 异常追踪）与一致的用户错误提示。

## What I already know

* 后端日志现状：`println("Error:", err.Error())` 散落 `app.go`（187/32/42/52/62... 等 10+ 处）+ service 层，stdout 打印无级别无落盘
* 后端错误现状：service 层 `fmt.Errorf("中文消息: %w", err)` 直返，无统一错误类型 / 错误码 / 分类
* 前端错误拦截：`frontend/src/utils/gitError.js`（`isGitOperationInProgress` + `handleGitError`）仅 git 域 6 组件用（LocalChanges/GitMerge/GitTags/GitRemotes/GitBranches/ContentPanel），其他组件各自 catch
* 前端 ElMessage 错误提示散落各组件
* Wails v2 桌面应用，单进程单用户，无 HTTP server，日志落盘到本地文件
* 后端 service 边界已稳定（上一个任务 AppServices 装配集中），本任务在其上铺设横切

## Assumptions (temporary)

* 日志方案选型待研究：Go 标准库 `log/slog` vs zerolog vs zap，桌面应用场景取舍
* 错误处理范围待确认：仅日志统一 / 含统一错误类型 / 含前端错误码协议
* 日志落盘位置：data/ 目录下？轮转策略？

## Open Questions

* ~~日志库选型~~ → slog（标准库）+ lumberjack（见 Decision）
* ~~错误处理方案~~ → A' AppError + ErrorFormatter 注册（源码核实 Wails 原生支持）
* ~~迁移范围~~ → 日志全量（59 处 println）+ 错误两域（git/obsidian）
* ~~规范冲突~~ → 修订 docs/开发规范.md「禁用 log 包」为 slog 规范（DoD 必然项）

## Decision (ADR-lite)

**Context**: 后端日志全用 `println` 散落 59 处无级别无落盘，GUI 双击启动时不可见致崩溃无据可查；service 错误 `fmt.Errorf` 中文直返无统一类型；前端 `gitError.js` 中文字串子串匹配脆弱仅 git 域 6 组件。`docs/开发规范.md:138-147` 明确禁用 log 包，与新方案冲突。

**Decision**:
- **日志**：slog（标准库）+ lumberjack 按大小轮转（5MB/5 份/压缩）。package-level logger + `SetLogger` setter 注入，`NewAppServices` 顶部 `service.SetLogger(util.InitLogger(data/logs))`。不改 14 service 构造签名，保 76% 覆盖率基线。落盘 `data/logs/app.log`，dev 模式额外 stdout。
- **错误**：自定义 `AppError{Code,Message,Err}` 类型，`main.go` `wails.Run` options 注册 `options.ErrorFormatter`，将 AppError 转为 `{code,message}` 对象传前端（Wails v2.12 源码核实：`CallbackMessage.Err` 类型 `any`，`ErrorFormatter func(error) any` 原生支持结构化 error 传递）。133 委托方法签名零改，Wails 绑定零 diff。前端直接读 `error.code`/`error.message`，无需正则。
- **迁移范围**：日志全量（59 处 println → slog）+ 错误两域（git/obsidian 已有 4 sentinel 基础转 AppError，其余 fmt.Errorf 经 ErrorFormatter 默认 `{message: err.Error()}` 处理）。
- **前端**：`gitError.js` → 通用 `error.js` `handleError(prefix, error)`，按 code 分流 ElMessage.warning/error，全组件统一。
- **规范**：修订 `docs/开发规范.md:138-147`「禁用 log 包」为 slog 规范。

**Consequences**:
- package-level logger 是全局状态（测试需 SetLogger 重置或 slog.Default 兜底）
- 错误码命名规范需制定（E_<域>_<动作>，如 E_GIT_IN_PROGRESS）
- AppError 渐进迁移，未迁移域经 ErrorFormatter 默认处理，前端 helper 兼容无 code 的纯 message error
- lumberjack 按大小非按天轮转（本项目日志量小，足够）
- 需补 `.gitignore` `data/logs/` 规则

## Requirements (evolving)

* 新增 `util/logger.go`：`InitLogger(logDir)` 创建 lumberjack.Writer + slog.JSONHandler，dev 模式 `io.MultiWriter` 加 stdout
* 新增 `service/logger.go`：package-level `var log *slog.Logger` + `SetLogger(l)` + `Logger()` helper（未注入返 `slog.Default()` 兜底）
* `app_services.go:NewAppServices` 顶部注入 `service.SetLogger(util.InitLogger(filepath.Join(dataDir,"logs")))`
* 59 处 `println("Error:", err.Error())` / `println(...)` 替换为 `slog.Error(...)` / `slog.Info(...)`，带结构化字段（component/repo/path/err）
* 新增 `model/app_error.go`：`AppError{Code,Message,Err}` 类型 + `Error()`/`Unwrap()` + 错误码常量表（E_GIT_IN_PROGRESS 等）
* `main.go` `wails.Run` options 注册 `options.ErrorFormatter`：AppError 转 `{code,message}`，普通 error 转 `{message: err.Error()}`
* git 域（service/git.go ErrOperationInProgress + app_git.go 相关）错误转 AppError
* ~~obsidian 域~~ → 降级保留现状：obsidian 4 方法返回 string 状态码（"not-registered" 等）非 error 路径，转 AppError 须改返回签名触前端，收益低风险高，本任务不动
* 前端 `frontend/src/utils/gitError.js` → 扩展为 `error.js` 通用 `handleError(prefix, error)`，按 code 分流；git 域 6 组件迁移，其余组件接入
* `.gitignore` 补 `data/logs/`
* `docs/开发规范.md:138-147` 修订为 slog 规范
* 新增 `docs/spec/logging-and-errors.md` 日志规范 + 错误处理契约

## Acceptance Criteria (evolving)

* [ ] `util/logger.go` + `service/logger.go` 落地，日志落盘 `data/logs/app.log`
* [ ] 59 处 println 替换为 slog，带结构化字段
* [ ] `model/app_error.go` AppError 类型 + 错误码常量表
* [ ] `main.go` 注册 ErrorFormatter，AppError 结构化传前端
* [ ] git/obsidian 两域错误转 AppError
* [ ] 前端 `error.js` 通用 helper，全组件覆盖
* [ ] `go test ./... -race` 全绿，service ≥76%，前端 ≥70%
* [ ] `wails build` 通过，Wails 绑定零 diff（App.js/App.d.ts/models.ts）
* [ ] `.gitignore` 补 `data/logs/`
* [ ] `docs/开发规范.md` 修订 + `docs/spec/logging-and-errors.md` 落地
* [ ] CLAUDE.md 关键规则 + 路线图技术债勾选

## Definition of Done (team quality bar)

* 后端测试全绿（含 -race）
* 覆盖率不降基线
* 前端测试全绿
* CLAUDE.md / 路线图技术债勾选同步
* spec 文档更新（日志规范 + 错误处理契约）

## Out of Scope (explicit)

* 远程日志上报 / 遥测（桌面单用户无此需求）
* 后端依赖注入（上一任务已完成）
* E2E 测试（第三个任务）
* service 业务逻辑重写

## Technical Notes

* 日志现状：`println` 散落 app.go 10+ 处 + service 层
* 错误现状：service `fmt.Errorf` 中文直返，前端各自 catch
* 前端 helper：`gitError.js` 仅 git 域 6 组件
* 相关 spec：[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md) / [test-coverage-gate.md](docs/spec/test-coverage-gate.md)
* 上一任务成果：AppServices 装配集中（[app-services-assembly.md](docs/spec/app-services-assembly.md)），logger 注入位可在此扩展

## Research References

* [`research/logging-approach.md`](research/logging-approach.md) — slog+ lumberjack 胜出；AppError + ErrorFormatter（Wails v2.12 源码核实 `CallbackMessage.Err any` + `ErrorFormatter func(error) any` 原生支持结构化 error 传递，133 签名零改）

## Technical Notes

* 日志现状：59 处 `println` 散落 17 文件（app_directory.go 9/app_external.go 8/app_filetree.go 6 等），无落盘，GUI 双击启动不可见
* 错误现状：411 处 `fmt.Errorf`（service 层主），4 个 sentinel（git ErrOperationInProgress + obsidian 3 个），0 自定义 error 类型
* 前端现状：`gitError.js` 中文字串子串匹配，仅 git 域 6 组件
* **Wails ErrorFormatter 源码核实**：`internal/frontend/dispatcher/calls.go:74` `CallbackMessage.Err any`；`calls.go:53-54` `d.errfmt(err)` 返 any；`pkg/options/options.go:69` `ErrorFormatter func(error) any` 选项。注册后 AppError 结构化传前端，签名零改
* 装配注入点：`app_services.go:NewAppServices`（上一任务成果），logger 在此注入
* 规范冲突：`docs/开发规范.md:138-147`「禁用 log 包用 println」需修订为 slog 规范
* `.gitignore` 已有 `notify_error.log` 先例，需补 `data/logs/`
* 相关 spec：[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md) / [test-coverage-gate.md](docs/spec/test-coverage-gate.md) / [app-services-assembly.md](docs/spec/app-services-assembly.md)
