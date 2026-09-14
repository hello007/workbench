# AppServices 装配范式

> App struct service 装配集中化契约。记录 `AppServices` 聚合 + Go 字段提升内嵌机制，供后续新增 service、重构 App struct、或评估 DI 框架升级时参照。
> 最后更新：2026-09-14 · 来源任务：09-12-wire-service（09-14 委托方法/字段计数校正）

---

## 1. 适用范围

- 新增 service 到 App（加字段 + 装配）
- 重构 App struct 持有方式
- 评估是否引入 DI 框架（Wire / fx 等）
- App 相关测试构造（字面量 / helper）

## 2. 背景

原 `App` struct 在 `app.go` 持 16 个 service/cache 字段，`startup()` 手动 `new`，构造签名五类混杂（无参 / configPath / dataDir 子路径 / ctx / 跨依赖）。新增 service 须手改 startup 且易漏跨依赖。

调研（[research/di-approach.md](.trellis/tasks/09-12-wire-service/research/di-approach.md)）结论：本项目依赖图极扁平（16 个 service/cache、仅 1 条跨依赖边 `SkillDiscovery → Directory`），生命周期重（`os.Exit` / goroutine / `CloseAll`），Wire 图求解价值闲置且引入双生成器复杂度，fx 运行时反射对桌面二进制风险高。采用方案 A 手写聚合。

## 3. 契约

### 3.1 装配集中

- 全部 service/cache 装配集中在 `NewAppServices(ctx, dataDir)`（`app_services.go`，package main）
- `App.startup` 仅调 `NewAppServices` + 执行启动期副作用，不直接 `new` service
- 新增 service 改动点 = 2 处：`AppServices` struct 加字段 + `NewAppServices` 加构造行

### 3.2 内嵌字段提升零 diff

- `App` 内嵌 `*AppServices`（非持字段），借 Go 字段提升使 `a.directorySvc` 等直接可达
- 138 个 `app_*.go` 委托方法 `a.xxxSvc.Method()` 零改动
- Wails 绑定（App.js / App.d.ts）由 `wails generate module` 重生成，因导出方法签名零变更，绑定零 diff（已验证）

### 3.3 包归属关键

- `AppServices` 须定义在 `package main`，**不可放 `package service`**
- 原因：Go 跨包内嵌时未导出字段不提升。若 `AppServices` 在 service 包且字段小写（`directorySvc`），main 包内 `a.directorySvc` 编译失败；字段大写（`DirectorySvc`）则 138 委托方法全改 `a.DirectorySvc`
- 放 main 包字段保持小写，内嵌提升同包可见，零 diff 成立

### 3.4 构造与生命周期分离

- `NewAppServices` 仅做纯构造（`new` 16 个 service/cache），不执行副作用
- 启动期副作用保留 `App.startup`：
  - `aiFuncSvc.StartHistoryCleanup()`：定时清理兜底 goroutine
  - `updateSvc.SetContext(ctx)`：ctx setter 注入
  - `updateSvc.CheckPendingUpdate()`：命中待更新则 `os.Exit(0)`
- 关停副作用保留 `App.shutdown`：
  - `terminalSvc.CloseAll()` / `aiFuncSvc.CloseAll()`

### 3.5 NewApp nil 防护

- `NewApp()` 返回 `&App{AppServices: &AppServices{}}`，AppServices 非 nil、字段全 nil
- 保证 startup 前或测试零值构造时，`a.xxxSvc` 字段提升访问不 panic
- 各 service 方法内已有的 `if xxxSvc == nil` 防护自然生效
- 生产 startup 后 `NewAppServices` 覆写空 AppServices

## 4. 新增 service 步骤

1. `app_services.go` `AppServices` struct 加字段（小写驼峰，与现命名一致）
2. `NewAppServices` 加构造行（注意跨依赖顺序：被依赖者先构造）
3. `app_*.go` 加委托方法（`a.xxxSvc` 字段提升可达，无需改 App struct）
4. 若 service 构造需新 configPath/dataDir 子路径，在 `NewAppServices` 内 `filepath.Join` 处理
5. 跑 `wails generate module` 重生成绑定，确认 App.js / App.d.ts 仅新增方法（无现有方法 diff）

## 5. 测试构造范式

内嵌后 `App` struct literal 仅接受 `ctx` 与 `AppServices` 两个字段，service 字段须经 `AppServices` 注入：

```go
// 单 service 注入
app := &App{AppServices: &AppServices{directorySvc: service.NewDirectoryService(configPath)}}

// 多 service 注入
app := &App{
    AppServices: &AppServices{
        directorySvc: service.NewDirectoryService(...),
        gitSvc:       service.NewGitServiceWithCache(...),
        repoMetaSvc:  service.NewRepoMetaService(...),
    },
}

// 零值构造（AppServices 非 nil，字段全 nil，方法内 nil 防护处理）
app := NewApp()

// 字段提升赋值（NewApp 后注入单个 service，AppServices 已非 nil）
app := NewApp()
app.commitHistoryCache = service.NewCommitHistoryCache()
```

## 6. 何时重新评估 DI 框架

出现以下任一条件时，可重新评估升级到 Wire：
- service 跨依赖边增至 10+ 条，人工排序开始易错
- 引入需多实现切换的抽象（如日志/遥测 interface + 多 backend），需 `wire.Bind` 接口绑定
- service 数量增至 30+ 且 provider 来自多个子包，需 `wire.NewSet` 分组治理

fx 在桌面单进程场景无明确升级触发条件（运行时反射对分发二进制风险高）。

## 7. 相关文档

- [cross-layer-contracts.md](cross-layer-contracts.md) — Wails 绑定契约，本范式保证方法签名零变更从而绑定零 diff
- [test-coverage-gate.md](test-coverage-gate.md) — service ≥76% 基线，重构不可降
- [research/di-approach.md](.trellis/tasks/09-12-wire-service/research/di-approach.md) — Wire vs 手写聚合 vs fx 选型调研
