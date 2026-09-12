# 调研：Go 桌面应用（Wails v2）结构化日志库选型 + 全局错误处理最佳实践

- **范围**：mixed（内部代码核实 + 外部技术调研）
- **日期**：2026-09-12
- **运行时限制说明**：外部技术对比基于 Go 生态通用知识整理，版本/行为细节标注「待核实」，落地前用官方文档复核。

---

## 一、调研结论与推荐（前置）

| 维度 | 推荐方案 | 一句话理由 |
|---|---|---|
| 日志库 | **slog（标准库）+ lumberjack 轮转** | 零外部依赖、Go 1.24 原生支持、结构化字段齐全；桌面单用户性能非关键 |
| 日志注入 | **package-level logger + `SetLogger` setter** | 不改 14 个 service 构造签名，不触碰 76% 覆盖率基线，AppServices 装配点统一注入 |
| 错误处理 | **自定义 `AppError{Code,Message}` 类型，`Error()` 输出 `[CODE] message`** | 保留 `%w` 链式包装，前端按前缀解析错误码；方法签名不变 → Wails 绑定零 diff |
| 前端拦截 | **扩 `gitError.js` → 通用 `handleError` helper** | 全组件统一，按 code 分流，替代 message 子串匹配 |

**关键发现**：Wails v2 bound method 返回 `error` 时，前端拿到标准 JS `Error`，`.message` = Go 侧 `err.Error()` 字符串，**自定义 error 类型的结构化字段不会跨边界传递**。决定「错误码必须编码进 `Error()` 字符串」或「改方法签名返回结构化值」两条路。

> **2026-09-13 更正（源码核实）**：上述"致命约束"基于未联网推测。核实 Wails v2.12 源码后发现**原生支持结构化 error 传递**：
> - `internal/frontend/dispatcher/calls.go:74` `CallbackMessage.Err` 类型为 `any`（非 string）
> - `calls.go:53-54` 若注册 `d.errfmt`（`options.ErrorFormatter`），调用 `d.errfmt(err)` 返回 `any`，`json.Marshal` 后传前端
> - `pkg/options/options.go:69` `ErrorFormatter func(error) any` 选项，在 `wails.Run` options 注册
> - 即：自定义 `AppError{Code,Message}` 经 formatter 转为 `{code, message}` 对象传前端，前端直接读 `error.code`/`error.message`，**无需正则解析字符串前缀，无需改 133 委托方法签名**
>
> 此发现使错误处理方案升级为「方案 A'：AppError + ErrorFormatter 注册」优于原推荐的「Error() 编码 code 字符串」。详见四 4.2 Approach A'。

---

## 二、内部现状（已核实）

### 2.1 日志现状

| 项 | 数据 |
|---|---|
| `println("Error:", err.Error())` 散点 | 59 处 / 17 文件 |
| `fmt.Errorf` 总量 | 411 处 / 44 文件（service 层占绝大多数） |
| 现有日志库（slog/zerolog/zap） | 0 处 |
| 日志落盘 | 无，全部 stdout |

散点分布（top）：`app_directory.go`(9)、`app_external.go`(8)、`app_filetree.go`(6)、`app_clipboard.go`(4)、`app_repometa.go`(3)。

`console_windows.go` 的 `consolePrint()` 通过 `AttachConsole` 附加父进程控制台，GUI 模式回退 stderr。`println` 在无控制台的 GUI 双击启动时基本不可见——崩溃排查无据可查的根因。

**规范冲突**：`docs/开发规范.md:138-147` 明确写「使用 println 输出调试信息」「禁止使用 log 包」。与新方案直接冲突，属本任务 DoD 必须修订项。

### 2.2 错误处理现状

| 项 | 数据 |
|---|---|
| sentinel error | 4 个（`service/fileoperation.go:23-27` 3 个 obsidian + `service/git.go:22` ErrOperationInProgress） |
| `errors.Is/As` 消费点 | 5 处 |
| 自定义 error 类型（带错误码） | 0 |
| 错误包装 | `fmt.Errorf("中文消息: %w", err)` 普遍正确用 `%w` |

现有「错误分流」原型：`app_external.go` `OpenInObsidian`/`AutoRegisterAndOpen` 用 `errors.Is` 识别 sentinel，返回短状态码字符串（`"not-registered"`/`"not-installed"`/`"running"`/`"failed"`）。已有「后端 sentinel → 前端可分流」原型，但状态码硬编码各方法内、无统一协议、仅 obsidian 域。

两种 error 返回风格并存：
1. `(*model.X, error)`：Wails 序列化成 JS Error，前端 try/catch
2. `string`：如 `app_git.go:44` `return "错误: " + err.Error()`，绕过 Go error 机制

前端 `gitError.js` 的 `isGitOperationInProgress` 按**中文字符串子串匹配**——脆弱（改文案即失效）、仅 git 域 6 组件。

### 2.3 装配点与注入位

`app_services.go:46` `NewAppServices(ctx, dataDir)` 集中构造 14 service。**所有 service 构造器均不接受 logger 参数**。注入策略关键约束。

`.gitignore`：`data/*.json` 忽略，`data/` 目录与 `*.log` 未忽略。已有 `notify_error.log` 单独忽略行，日志文件落地 gitignore 已有先例。需补 `data/logs/` 规则。

---

## 三、外部技术调研

### 3.1 日志库选型：slog vs zerolog vs zap

| 维度 | log/slog | zerolog | zap |
|---|---|---|---|
| 引入 | Go 1.21+ 标准库零依赖 | rs/zerolog | uber-go/zap |
| 依赖体积 | 0 | 小 | 较大 |
| 结构化字段 | `slog.Info("msg","k",v)` k-v | 链式 `Str().Msg()` | `zap.String` / Sugar `Infow` |
| 性能（零分配） | 一般 | 最快 | 接近 zerolog |
| 轮转支持 | 无，需外接 | 无，需外接 | 无，需外接 |
| 本项目契合 | **最高**：go 1.24.0 原生，零依赖 | 中 | 低：依赖重，性能优势无意义 |

桌面单进程单用户，日志吞吐极低，**零分配性能优势无意义**。选型优先：①依赖体积 ②API 易用 ③与 Go 团队方向对齐。**slog 胜出**。

### 3.2 日志轮转

| 方案 | 机制 | 适配 |
|---|---|---|
| **lumberjack** | 按大小轮转（MaxSize MB，保留 MaxBackups 份，可 Compress） | 最主流，slog Handler 的 io.Writer 即插即用 |
| file-rotatelogs | 按时间轮转 | 需按天切割时选；本项目日志量小，按大小足够 |
| 自研 slog.Handler | 自行实现 | 不推荐重复造轮子 |

**澄清**：lumberjack 按文件大小轮转，非按日期。本项目按大小 5MB + 保留 5 份 + 压缩，上限约 25MB，足够。

### 3.3 桌面应用日志落盘策略

| 候选 | 路径 | 优劣 |
|---|---|---|
| 应用 data 目录 | `data/logs/app.log` | 与现有 `data/*.json` 同域，用户可在应用内「打开 data 目录」直查；data 目录随 exe 移动 |
| Windows %APPDATA% | `%APPDATA%\WorkBench\logs\` | 跨实例稳定，符合 Windows 规范；路径不直观 |
| 两者结合 | 默认 data/logs/，设置面板提供「打开日志目录」按钮 | **推荐** |

**推荐** `data/logs/app.log`：与现有配置同域便于定位；单用户便携式工具 data 随 exe 走；复用 `OpenInExplorer` 打开目录零 UI 成本。

### 3.4 Go 错误处理最佳实践

| 机制 | 适用 |
|---|---|
| sentinel error | 稳定、少量、需跨层身份识别 |
| 自定义 error 类型 | 需携带结构化信息（错误码、分类、上下文） |
| %w 链式包装 | 每层附上下文同时保留根因 |
| errors.Is | 判等（含包装链） |
| errors.As | 提取类型化字段 |

要点：每层包装附加业务上下文用 `%w` 保留根因；service 返回错误时在「错误最终消费点」（通常 App 层委托方法）记录日志，避免每层重复记录膨胀。

### 3.5 Wails v2 错误向前端传递机制（关键）

**核心机制**：Wails v2 bound method 返回 `(T, error)` 时，runtime 调 `err.Error()` 取字符串，JS 侧 reject promise 抛 `new Error(message)`。前端拿到标准 `Error`，`.message` 即 Go 错误串。

**致命约束**：自定义 error 类型的结构化字段（Code、Category）**不会跨边界传递**，只有 `.Error()` 字符串存活。`errors.As` 在 JS 侧无对应物。

**待核实**：Wails v2.12 是否对实现了 `json.Marshaler` 的 error 调用 `MarshalJSON` 传递结构化信息——落地前查 Wails v2 `pkg/runtime` 源码确认。若不支持（当前判断），错误码只能以下面方式过界。

**错误码过界方案**：

| 方案 | 机制 | Wails 绑定影响 | 优劣 |
|---|---|---|---|
| **A. 编码进 Error() 字符串** | `Error()` 返回 `"[E_GIT_IN_PROGRESS] 该仓库有 Git 操作进行中"` | **零**（签名不变） | **推荐**：最小侵入 |
| B. 纯 message 子串匹配 | 沿用 gitError.js 扩展 | 零 | 脆弱，改文案即失效 |
| C. 返回结构化 error 值 | 签名改返回 `model.AppResult` | **需同步 App.d.ts** | 最干净但最重，违反「不碰绑定」 |

---

## 四、方案设计

### 4.1 日志库方案

#### Approach A（推荐）：slog + lumberjack，package-level logger + SetLogger 注入

**工作原理**：
1. `util/logger.go` 定义 `InitLogger(logDir)`：lumberjack.Writer（5MB/5 份/压缩）→ `slog.JSONHandler` → `slog.SetDefault(logger)`
2. `service/logger.go` 定义 package-level `var log *slog.Logger` + `SetLogger(l)` setter + `logger()` helper（未注入返 `slog.Default()` 兜底）
3. `app_services.go:NewAppServices` 顶部 `service.SetLogger(util.InitLogger(filepath.Join(dataDir,"logs")))`
4. service 用 `service.Logger().Error("clone failed","repo",name,"err",err)`；App 层用 `slog.Error`
5. dev 模式额外加 stdout handler（`io.MultiWriter`），prod 仅文件

**优势**：零外部核心依赖增量；不改 14 个 service 构造签名保覆盖率；AppServices 装配点统一注入；结构化字段齐全。
**劣势**：package-level logger 是全局状态（测试需 SetLogger 重置）；lumberjack 按大小非按天。
**适配度**：高。

#### Approach B：zerolog + lumberjack
API 链式直观，零分配性能（桌面无意义）。新增外部依赖，与标准库方向不一致。适配度中。

#### Approach C：slog + 自研轮转 Handler
零外部依赖但需自测轮转，重复造轮子。适配度低。

### 4.2 错误处理方案

#### Approach A（推荐）：自定义 AppError 类型 + Error() 编码错误码

```go
type AppError struct {
    Code    string // "E_GIT_IN_PROGRESS"
    Message string // 用户可读中文
    Err     error  // 根因，支持 %w 链
}
func (e *AppError) Error() string { return "[" + e.Code + "] " + e.Message }
func (e *AppError) Unwrap() error { return e.Err }
```

service 关键错误点用 `&AppError{...}` 替换裸 `fmt.Errorf`；普通错误保留 `fmt.Errorf("ctx: %w", err)`。App 层委托方法记 `slog.Error` 后原样返回 error。前端 `error.js`（扩展自 gitError.js）正则 `^\[(E_\w+)\]` 提取 code，按 code 映射 ElMessage.warning/error。

**优势**：Wails 绑定零 diff；错误码过界；保留 %w 链；前端从中文匹配升级为 code 匹配；渐进迁移。
**劣势**：`Error()` 字符串格式变更需前端 helper 剥离 code 前缀展示；错误码命名规范需制定；需统一 App 层「记录日志 + 返回 error」模板。
**适配度**：最高。

#### Approach B：纯 sentinel + errors.Is，前端 message 子串匹配（现状扩展）
改动最小，无错误码规范成本。中文字串脆弱，无错误码聚合。适配度中低。

#### Approach C：返回结构化 error 值，改方法签名 + Wails 绑定同步
前端最干净强类型。**违反「不碰 Wails 绑定」约束**——改 133 委托方法签名 + 同步三文件 + 重测。适配度低。

---

## 五、推荐组合与落地要点

**日志**：Approach A（slog + lumberjack + package-level SetLogger）。
**错误**：Approach A（AppError + Error() 编码 code + 前端 helper 解析前缀）。

落地要点：
1. 新增 `util/logger.go`（InitLogger）+ `service/logger.go`（package-level logger + SetLogger）
2. `app_services.go:NewAppServices` 顶部注入 logger
3. 新增 `model/app_error.go`（AppError 类型）+ 错误码常量表
4. 渐进迁移：先 git 域 + obsidian 域，其余保留现状
5. App 层委托方法统一「slog.Error 记录 + 返回 error」模板
6. 前端 `gitError.js` → `error.js` 通用 `handleError`，按 code 分流；git 域 6 组件迁移，其余接入
7. `.gitignore` 补 `data/logs/`
8. `docs/开发规范.md:138-147` 修订「禁用 log 包」为 slog 规范
9. `docs/spec/` 新增日志规范 + 错误处理契约文档

**覆盖率风险控制**：不重写 service 方法体，仅新增 logger 调用与 AppError 类型；新增代码配套单测。既有 76% 基线因不改动既有测试保持。

---

## 六、Caveats / 待核实

1. **Wails v2.12 是否对 `json.Marshaler` error 调 MarshalJSON**：未联网核实。当前判断「否，仅取 `.Error()`」。落地前查 Wails v2 源码确认——若支持，错误处理 Approach C 可不动签名实现错误码过界，会改变推荐结论。
2. lumberjack/slog 版本与 go 1.24 兼容性：`go get` 时确认。
3. lazygit/gh-cli 日志实现细节：基于通用知识，未读源码，仅参考方向。
4. 项目内无任何已有日志库使用、无日志落盘先例（除 `notify_error.log`），确认 greenfield 引入。
5. `data/` 目录在 GUI 双击启动时相对路径基准：`main.go` 用 `filepath.Join("data",...)` 相对路径依赖 exe 工作目录。日志落盘 `data/logs/` 沿用此基准。
6. 错误码命名规范：仅给样例（E_GIT_IN_PROGRESS），完整错误码表 implement 阶段按域制定。

---

**一句话结论**：推荐 **slog（标准库）+ lumberjack 按大小轮转 + package-level SetLogger 注入** 做日志，**自定义 AppError 类型将错误码编码进 `Error()` 字符串** 做错误处理——两者均不改 service 构造签名与 Wails 绑定，保住 76% 覆盖率基线与 App.d.ts 零 diff；唯一待核实项是 Wails v2.12 是否对 `json.Marshaler` error 做结构化序列化。
