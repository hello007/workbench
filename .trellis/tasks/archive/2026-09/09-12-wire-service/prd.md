# 后端依赖注入(Wire)与 service 治理

## Goal

WorkBench 后端 `App` struct 在 `startup()` 手动 new 13 个 service，构造签名参差（configPath / dataDir 子路径 / ctx / 跨 service 依赖 / 无参五类），新增 service 需手改 startup 且易漏跨依赖。引入依赖注入统一 service 装配与生命周期，并理顺 service 治理边界，为后续全局错误处理 + 日志系统（横切关注点）铺设稳定分层。

## What I already know

* `app.go:13` `type App struct` 持 13 个 service 字段（directorySvc / fileTreeSvc / fileOpSvc / gitSvc / settingsSvc / terminalSvc / searchSvc / favoritesSvc / contentSearchSvc / updateSvc / repoMetaSvc / aiFuncSvc / skillDiscoverySvc）
* `app.go:35` `startup(ctx)` 按序手动 new，构造路径散落：
  - 无参：`NewFileTreeService` / `NewFileOperationService` / `NewSearchService` / `NewContentSearchService` / `NewUpdateService`
  - 单 configPath：`NewDirectoryService` / `NewSettingsService` / `NewFavoritesService`
  - dataDir 子路径：`NewGitServiceWithCache(repo_scan_cache.json)` / `NewRepoMetaService(repo_meta.json)` / `NewAiFunctionService(ctx, ai_functions.json)` / `NewAiTaskHistoryService(dataDir)`
  - 跨 service 依赖：`NewSkillDiscoveryService(a.directorySvc)`（唯一现有跨依赖）
  - ctx 依赖：`NewTerminalService(ctx)` / `NewAiFunctionService(ctx, ...)`
* service 包 50+ 文件，测试覆盖 service ≥76% 基线（[test-coverage-gate.md](docs/spec/test-coverage-gate.md)）
* Wails v2 桌面应用，单进程单用户，无 HTTP server 启动框架
* 不改 Wails 绑定（App.js / App.d.ts / models.ts），仅后端内部重构（见 [cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)）

## Assumptions (temporary)

* "service 治理"范围待确认：纯装配 DI，还是含接口抽取 / 生命周期 / 错误包装标准化
* Wire 代码生成适合本规模（13 service，1 跨依赖）—— 待研究验证
* 重构分批，每批可单独测试，Wails 绑定零变更

## Open Questions

* ~~Wire vs 手写 provider~~ → 已定方案 A（见 Decision）
* "service 治理"具体范围边界（仅装配 / 含接口 / 含生命周期 / 含错误标准）— 待定
* 是否在本次顺带统一 service 构造签名风格（如统一接收 config struct）— 待定

## Decision (ADR-lite)

**Context**: 任务标题原点名 Wire。调研 [research/di-approach.md](research/di-approach.md) 显示本项目依赖图极扁平（13 service、仅 1 条跨依赖边 `SkillDiscovery → Directory`），生命周期重（`os.Exit` / goroutine / CloseAll），133 个 App 委托方法须保稳定。Wire 的图求解价值在此规模闲置，且引入 `wire gen` 与 `wails generate` 双生成器复杂度；fx 运行时反射对桌面二进制风险高。

**Decision**: 采用方案 A — 手写 `NewAppServices` 聚合函数 + `App` 内嵌 `*AppServices`（Go 字段提升）。装配集中到 `NewAppServices(ctx, dataDir)`（定义在 `package main` 新建 `app_services.go`），`App` 内嵌 `*AppServices` 使 133 个委托方法 `a.xxxSvc` 直接可达，零 diff；Wails 绑定三文件零 diff。零新依赖、零新构建步骤。

**Consequences**:
- 装配顺序仍人工维护（图仅 1 边，成本可忽略）
- 当 service 跨依赖边增至 10+ 或引入多实现抽象时，重新评估升级 Wire
- 生命周期（StartHistoryCleanup / SetContext / CheckPendingUpdate+os.Exit / CloseAll）仍保留在 startup/shutdown，本任务不抽离

## Requirements (evolving)

* 新建 `app_services.go`（package main），定义 `AppServices` struct，14 字段与 App 现字段同名同序小写
* `NewAppServices(ctx, dataDir)` 集中装配 14 个 service/cache，处理 configPath / dataDir 子路径 / ctx / 跨依赖（`SkillDiscovery → Directory`）
* `App` struct 改为内嵌 `*AppServices`，移除 14 个 service 字段
* `NewApp()` 改为 `return &App{AppServices: &AppServices{}}`（nil 防护）
* `startup()` 改调 `a.AppServices = NewAppServices(ctx, dataDir)`，保留 4 处生命周期副作用（`StartHistoryCleanup` / `SetContext` / `CheckPendingUpdate+os.Exit` / `CloseAll` 在 shutdown）
* 测试同步改：`app_test.go` 8 处 + `app_repo_filter_test.go` 1 处字面量 + `newAppWithCommitCache` helper + `app_git_stats_test.go:128`
* Wails 绑定零变更，App 方法签名不变
* `go test ./... -race` 全绿，service 覆盖率不降基线

## Acceptance Criteria (evolving)

* [ ] `app_services.go` 落地，`NewAppServices` 集中 14 装配点
* [ ] `App` 内嵌 `*AppServices`，133 委托方法零 diff（`a.xxxSvc` 字段提升可达）
* [ ] `NewApp()` 返回非 nil AppServices，零值 App 调方法不 panic
* [ ] startup 保留 4 处生命周期副作用
* [ ] 测试同步改并全绿
* [ ] Wails 绑定三文件零 diff（App.js / App.d.ts / models.ts）
* [ ] `go test ./... -race` 全绿，service ≥76%
* [ ] `wails build` 通过
* [ ] CLAUDE.md / 路线图技术债勾选同步

## Definition of Done (team quality bar)

* 后端测试全绿（含 -race）
* service 覆盖率不降基线
* Wails 绑定零变更
* CLAUDE.md / 路线图技术债勾选同步
* 跨层契约文档更新（如涉及构造签名变更）

## Out of Scope (explicit)

* 前端任何改动
* Wails 绑定生成
* 全局错误处理 + 日志系统（独立后续任务）
* service 间业务逻辑重写

## Technical Notes

* 装配现状：`app.go:13-80`，14 装配点（13 service + 1 commitHistoryCache）
* 唯一跨依赖：`SkillDiscoveryService → DirectoryService`
* **包归属关键修正**：`AppServices` 须放 `package main`（新建 `app_services.go`），非 `package service`。Go 跨包内嵌未导出字段不提升——若 `AppServices` 在 service 包且字段小写，main 包内 `a.directorySvc` 编译失败；字段大写则 133 委托方法全 diff。放 main 包字段保持小写，内嵌提升同包可见，133 方法零 diff。研究样例 `service/AppServices.go` 疏忽，以本注记为准。
* App struct 14 字段全小写驼峰（`directorySvc` 等），`AppServices` 字段须同名同序
* 生命周期副作用 4 处保留 startup/shutdown：`aiFuncSvc.StartHistoryCleanup()` / `updateSvc.SetContext(ctx)` / `updateSvc.CheckPendingUpdate()+os.Exit(0)` / `terminalSvc.CloseAll()`+`aiFuncSvc.CloseAll()`
* `commitHistoryCache` 非 service 但装配在 startup，一并迁入 `AppServices`
* **测试构造成本（内嵌必然）**：
  - `app_test.go` 8 处 `&App{directorySvc: ...}` 字面量
  - `app_repo_filter_test.go` 1 处 `&App{directorySvc:..., gitSvc:...}` 字面量
  - `newAppWithCommitCache()` helper（`app_git_cache_test.go:45`）：`app.commitHistoryCache = ...`
  - `app_git_stats_test.go:128`：`app.commitHistoryCache = nil`（走全量扫路径）
  - 内嵌后 `App` 无这些字段，须改 `&App{AppServices: &AppServices{...}}` 或加测试 helper
  - **nil 防护关键**：`app_git.go:177,232` 有 `if a.commitHistoryCache == nil`；`app_git_stats_test.go:63` `NewApp()` 零值后调 `GetRepoStats` 须不 panic。解法：`NewApp()` 改为 `return &App{AppServices: &AppServices{}}`，AppServices 非 nil 字段全 nil，字段提升访问不 panic，各 `if xxxSvc == nil` 防护自然生效。生产 startup 后 `NewAppServices` 覆写。
  - 非生产代码 diff，验收"Wails 绑定零 diff"不受影响
* 后续任务依赖：全局错误处理 + 日志需在稳定 service 边界上铺设，故本任务先行
* 相关 spec：[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md) / [test-coverage-gate.md](docs/spec/test-coverage-gate.md)

## Research References

* 待填充：`research/di-approach.md` — Wire vs 手写 provider 在 Go Wails 桌面应用本规模下的取舍
