# 调研：Wails v2 桌面应用后端依赖注入方案选型（Wire vs 手写聚合 vs fx）

- **查询**：Go 桌面应用（Wails v2）后端依赖注入方案选型 — google/wire 编译时代码生成 vs 手写 provider 聚合函数；含 fx 适用性、同类工具装配参考
- **范围**：mixed（内部代码核查 + 外部库现状核查）
- **日期**：2026-09-12

## 一、关键事实核查（已验证，非推测）

### 1.1 现状装配（内部，`app.go`）

| 位置 | 事实 |
|---|---|
| `app.go:13-29` | `App struct` 持 13 个 service 字段（含 `commitHistoryCache` 实为 14 个构造点） |
| `app.go:35-80` | `startup(ctx)` 手动 `new` 全部 service，构造签名五类混杂 |
| `app.go:65` | 唯一跨 service 依赖：`NewSkillDiscoveryService(a.directorySvc)` — 全图仅 1 条边 |
| `app.go:62` | 构造后副作用：`aiFuncSvc.StartHistoryCleanup()`（启 goroutine） |
| `app.go:69` | 构造后 setter：`updateSvc.SetContext(ctx)` |
| `app.go:74-77` | 启动期副作用：`CheckPendingUpdate()` 命中则 `os.Exit(0)` — 强生命周期语义 |
| `app.go:83-89` | `shutdown()` 调 `terminalSvc.CloseAll()` + `aiFuncSvc.CloseAll()` |

**构造签名分类**（14 个构造点）：无参 6 / 单 configPath 4 / dataDir 子路径 4 / ctx 依赖 2 / 跨依赖 1（部分重叠）。

### 1.2 项目硬约束（内部）

| 事实 | 来源 | 影响 |
|---|---|---|
| App 导出方法 133 个，全部 `a.xxxSvc.` 委托 | `grep app*.go` | 改 App 字段访问方式 → 133 处 diff，高风险 |
| service 包 51 个 `.go` 文件，仅 1 个 interface（`pipeReader`，内部） | `grep service/*.go` | 无抽象层，`wire.Bind`/fx interface provider 无用武之地 |
| 测试直接 `service.NewXxxService(...)` 构造（`NewDirectoryService` 在测试中出现 12 次） | `grep *_test.go` | 测试绕过 App 装配，DI 框架不带来可测性增益 |
| `go.mod` 无任何 DI 依赖，仅 `samber/lo`（indirect） | `go.mod` | 引入 Wire/fx = 新增直接依赖 + 新构建步骤 |
| 工具链 go1.26.2，`go.mod` 声明 `go 1.24.0` | `go version` / `go.mod` | 无版本阻碍 |
| App 字段不暴露给 Wails（Wails 只绑定导出方法） | `cross-layer-contracts.md` | 重构 App struct 字段 / startup 内部对 Wails 绑定零影响 |

### 1.3 外部库现状（网络核查）

| 库 | 最新版本 | 发布时间 | 维护状态 |
|---|---|---|---|
| `github.com/google/wire` | v0.7.0 | 2025-08-22 | 活跃（约一年内有 release） |
| `go.uber.org/fx` | v1.24.0 | 2025-05-13 | 活跃 |

### 1.4 同类工具装配参考

- **lazygit**（TUI，规模相近）：`pkg/gui/gui.go` 定义 `Gui struct` + `func NewGui(...)` 手写装配，无 DI 框架。
- **gh-cli**：`internal/cmd/factory` 采用 `Factory` struct + 惰性 `func() (T, error)` 闭包字段，手写工厂模式，无 DI 框架。
- **Wails 官方模板**：`startup(ctx)` 内直接构造，主流 Wails 应用未见 Wire/fx 集成范例。

**共性**：Go 桌面/CLI 工具普遍手写 struct 装配或工厂闭包，DI 框架在此场景属少数派。

## 二、核心判断（决定性依据）

1. **依赖图极扁平**：13 个根 service，仅 1 条跨依赖边（`SkillDiscovery → Directory`）。Wire/fx 的核心价值是图求解与拓扑排序，在 1 条边的图上几乎不发挥。
2. **真正复杂的是生命周期与副作用，不是构造图**：`StartHistoryCleanup` / `SetContext` / `CheckPendingUpdate + os.Exit(0)` / `CloseAll`。Wire 只解决构造，不解决生命周期；fx 解决生命周期但用运行时反射。构造本身仅 ~14 行 `new`，并非瓶颈。
3. **签名多样性不会被任何 DI 方案消除**：每个 service 仍需一个 provider/构造行处理 configPath/dataDir/ctx 差异，代码体积等价。
4. **133 个委托方法的稳定性是硬约束**：App 字段访问方式若改（如 `a.directorySvc` → `a.services.DirectorySvc`），133 处 diff。但 Go embedded struct 字段提升可让 `App` 内嵌 `*AppServices` 后，`a.directorySvc` 仍直接可达 → 133 处零 diff。

## 三、三个候选方案

### 方案 A：手写 provider 聚合函数（`NewAppServices`）— 推荐

**工作原理**

```go
// service/AppServices.go（新增）
type AppServices struct {
    directorySvc       *DirectoryService
    fileTreeSvc        *FileTreeService
    // ... 13 字段，与现 App 字段同名同序
    skillDiscoverySvc  *SkillDiscoveryService
}

func NewAppServices(ctx context.Context, dataDir string) *AppServices {
    s := &AppServices{}
    s.directorySvc = NewDirectoryService(filepath.Join(dataDir, "directories.json"))
    s.fileTreeSvc = NewFileTreeService()
    // ... 集中处理 configPath / dataDir / ctx / 跨依赖
    s.skillDiscoverySvc = NewSkillDiscoveryService(s.directorySvc)
    return s
}
```

`App` 改为内嵌 `*AppServices`：

```go
type App struct {
    *AppServices          // 内嵌 → a.directorySvc 等字段提升，133 委托方法零 diff
    ctx        context.Context
}

func (a *App) startup(ctx context.Context) {
    a.ctx = ctx
    a.AppServices = service.NewAppServices(ctx, "data")
    a.aiFuncSvc.StartHistoryCleanup()
    a.updateSvc.SetContext(ctx)
    if hasPending, _ := a.updateSvc.CheckPendingUpdate(); hasPending { os.Exit(0) }
}
```

**优势**：零新依赖、零新构建步骤、零新 DSL；编译期错误（无反射）；内嵌 `*AppServices` → 133 委托方法 + Wails 绑定三文件零 diff；生命周期与 `os.Exit(0)` 副作用自然保留在 `startup`；测试零影响；新增 service 改动点 = 2 处。

**劣势**：装配顺序仍人工维护（图仅 1 边，成本可忽略）；未来跨依赖激增至 10+ 条边时才开始吃力（当前不成立）。

**项目适配度：高**。

### 方案 B：google/wire 编译时代码生成

**工作原理**：`wire.go`（`//go:build wireinject`）声明 provider set + `wire.Build`，`wire gen` 生成 `wire_gen.go`，`startup` 调 `InitializeApp(ctx, dataDir)` 拿 `*AppServices`，再跑生命周期。

**优势**：编译期图校验，缺依赖在 `wire gen` / `go build` 即报错；声明式 provider 单一职责。

**劣势**：
- 新增构建步骤：`wire gen` 须与 `wails generate module` 共存（输出目录不冲突，但流程多一步）。
- `wire_gen.go` 提交策略需决策（社区主流提交入库，但增 review 噪音与 merge 冲突面）。
- 生命周期不覆盖：`StartHistoryCleanup` / `SetContext` / `CheckPendingUpdate+os.Exit` / `CloseAll` 仍须手写 — Wire 只搬走 ~14 行 `new`。
- 图求解价值不发挥：1 条边的图，拓扑排序能力闲置。
- 签名多样性仍需每 service 一个 provider，代码体积与手写等价。
- 团队须学 Wire DSL。

**项目适配度：中低**。Wails+Wire 无主流范例，属自踩路径。

### 方案 C：uber/fx 运行时 DI — 不推荐

**优势**：原生生命周期（`fx.Hook`）— 唯一能优雅承载 Start/Stop 的方案；模块化 provider 分组。

**劣势**：
- 运行时反射：依赖图错误在启动期才暴露，对分发到用户桌面的二进制是硬伤。
- 双事件循环协调：`fx.App.Run()` 阻塞，Wails `app.Run()` 也阻塞；须改用 `fx.App.Start(ctx)` 手动驱动，绕开 fx 主循环语义。
- 为 HTTP server 长驻服务设计，桌面单进程单用户过度设计。
- 同方案 B，签名多样性不消除。

**项目适配度：低**。

## 四、明确推荐

**采用方案 A（手写 `NewAppServices` 聚合函数 + `App` 内嵌 `*AppServices`）。**

理由闭环：
1. 依赖图扁平（1 条边）→ Wire/fx 图求解价值闲置。
2. 真实痛点是生命周期与签名多样性 → Wire 不解生命周期，fx 解生命周期但用反射；两者都不消除签名多样性。
3. 内嵌 `*AppServices` 使 133 委托方法与 Wails 绑定零 diff（决定性技术优势）。
4. 测试已直接构造 service，DI 框架无可测性增益。
5. 零新依赖、零新构建步骤、零新 DSL。
6. 满足全部验收项。

## 五、何时重新考虑 Wire/fx

- service 跨依赖边增至 10+ 条，人工排序开始易错；
- 引入需多实现切换的抽象（日志/遥测 interface + 多 backend），需要 `wire.Bind` 接口绑定；
- service 数量增至 30+ 且 provider 来自多个子包。

当前均不成立。fx 在桌面场景无明确升级触发条件。

## 六、相关 spec 与文件

- `app.go`（现状装配，`:13-80`）
- `docs/spec/cross-layer-contracts.md`（Wails 绑定契约）
- `docs/spec/test-coverage-gate.md`（service ≥76% 基线）
- `.trellis/tasks/09-12-wire-service/prd.md`

## 七、未决/待确认

- "service 治理"范围：本次聚焦装配 DI；接口抽取 / 生命周期标准化 / 错误包装是独立子议题。建议装配先行，接口抽取待全局错误处理任务再评估。
- 是否顺带统一 service 构造签名风格：方案 A 不要求，但 `NewAppServices` 是天然收口点，可同期收敛 — 可选项，不阻塞主路径。

**一句话结论**：本规模（13 service、仅 1 条跨依赖、生命周期重、133 委托方法须保稳定）下，**方案 A 手写 `NewAppServices` 聚合 + App 内嵌 `*AppServices`** 为推荐项；Wire 图求解价值闲置且引入双生成器复杂度，fx 运行时反射对桌面二进制过度且风险高，均不推荐。
