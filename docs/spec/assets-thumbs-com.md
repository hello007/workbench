# 素材缩略图 COM 队列契约（assets-thumbs）

> 来源：任务 10-14-ppt-com PR1（2026-10-14，PowerPoint COM 缩略图链路基座：串行队列 + STA worker + 平台分叉）。探针实测依据：`.trellis/tasks/10-14-ppt-com/research/com-thumbnail-export.md` 文末「探针定案」节 + `scripts/ppt-com-probe/`（一次性工具已归档结论）。

## 1. Scope / Trigger

- 触发：改动 `service/assets_thumbs*.go`（队列调度 / COM 实现 / 桩）、缩略图事件、打开跳转链路、新增类似「外部进程 COM 自动化」能力时。
- 涉及文件：`service/assets_thumbs.go`（平台无关队列）、`service/assets_thumbs_windows.go`（go-ole）、`service/assets_thumbs_other.go`（桩）、`app_assets.go`（3 绑定方法）、`model/assets.go`（事件载荷）。

## 2. Signatures

| 层 | 签名 | 位置 |
|---|---|---|
| 接口 | `ThumbExporter`：`Available() bool` / `ExportPresentation(ctx, spec, thumbsDir, tiers, preempt) (int, error)` / `JumpOnly(jr) error` / `Discard()` / `Shutdown()` | `assets_thumbs.go` |
| 队列 | `EnqueueAssetThumbs(libraryID, files, thumbsDir)` / `JumpToSlide(absPath, slideNum) error` / `CancelAssetThumbs(libraryID) bool` / `WaitThumbsDone()` / `ThumbsAvailable() bool` | `AssetLibraryService` |
| App 绑定 | `AssetThumbsAvailable() bool` / `JumpToAssetSlide(absPath, slideNum) error` / `CancelAssetThumbs(libraryID) bool` | `app_assets.go` |
| 事件 | `assets-thumbs:progress`（AssetThumbsProgress）/ `assets-thumbs:done`（AssetThumbsResult）/ `assets-thumbs:error`（PR2 触发点出现时补） | EventSink |
| 缓存命名 | `thumbPNGPath(thumbsDir, tierKey, fileID, slideNum)` = `<thumbsDir>/<tier>/<fileID>-<3位页>.png` | 单点函数 |
| 分叉构造 | `newThumbExporter()`（_windows=go-ole / _other=桩）/ `newThumbQueue(exp, sinkFn)` | 平台分叉点 |

## 3. Contracts

### STA worker 线程模型（SRS 硬约束「全局同一时刻最多 1 个 COM 任务」）

- **全部 COM 调用锁定在专用常驻 STA goroutine**：`runtime.LockOSThread()` + `CoInitializeEx(COINIT_APARTMENTTHREADED)`，**S_FALSE(0x1) 是「已初始化」非错误须放行**（go-ole 视 hr≠0 为错误），放行后仍须配对一次 `CoUninitialize`。
- 主侧（ExportPresentation/JumpOnly）只做任务提交与结果等待；主侧**不可跨线程直读 worker 状态**（ownedPIDs 等）——用互斥镜像（`ownedMu + ownedMirror`，worker 线程整体替换发布）。
- **阻塞 Invoke 不可中断**（Go 无法中断 syscall）：超时/取消唯一解 = 杀自启 COM 服务器进程，让在途 Invoke 以 `0x800706BA` 返回，worker 走清理路径（10s 宽限兜底 + slog 告警）。

### 进程归属判定（禁误杀用户实例）

- `CreateInstance` 前后进程快照 diff，**过滤集合 = `{powerpnt.exe, wpp.exe}`**——本机 `PowerPoint.Application` ProgID 可能由 WPS 接管（标准 MS CLSID 键存在但 LocalServer32 缺失，实际启动 `wpp.exe`），硬编码 powerpnt 会「枚举恒空 → 误判复用 → 禁杀」，超时强杀能力整体失效。
- 新增 PID = 自启实例（可 Quit 可强杀）；diff 为空 = 复用用户实例（**禁 Quit 禁杀**，静默只读消费）。**禁 `taskkill /IM` 按映像名全杀**。

### `jr.Result` 单写契约（P0 死锁教训）

- `thumbJumpRequest.Result`（缓冲 1）**只允许恰一次写回**：JumpOnly 契约内写回（成败均写）；preempt 页间插队路径由 COM 实现写回；`handleJump` **不得二次写**——Result 仅 `JumpToSlide` 单读者，双写 = worker 永久阻塞 = 整个队列停摆（check 实测抓出的 P0）。
- `JumpToSlide` 提交等待带 `thumbJumpEnqueueTimeout`（60s）超时：与单页导出最坏耗时对齐（HD 档复杂页可超 5s，批量在途时页边界 preempt 是唯一消费点——提交窗过短会在发送方退队后被 preempt 摘到 stale 请求静默执行）；结果等待带 90s 兜底（`thumbJumpResultTimeout` ≥ 实现内部 60s jumpTimeout + 10s 收尾宽限），防 worker 永挂传染到前端 RPC。

### 双引擎兼容（WPS / MS Office）

| 行为 | MS PowerPoint | WPS（实测） | 生产策略 |
|---|---|---|---|
| `Presentations` 访问 | 属性（GetProperty） | 同 | **禁 `CallMethod`**（只走 DISPATCH_METHOD 报「找不到成员」） |
| ww=0 无窗口导出 | 可用（社区共识） | **可用且渲染正常** | 恒 `Open(ro=-1, untitled=0, ww=0)` |
| 同名文件二次 Open | 报「已打开」错 | **允许双开** | 先按 FullName 尽力遍历复用（不依赖 Count 完整性——Count=0 时遍历空集自然落 Open），未命中直接 Open；WPS 同名双开允许下未命中 Open 直接成功 |
| Presentations.Count | 可靠 | **不可靠**（开着 1 个报 0） | 复用检查不能依赖集合完整枚举 |
| 显式尺寸导出 | 精确生效 | 精确生效（2560x1440 默认口径被绕开） | **纵横比调用方自算**（Export 传什么出什么，不 fit-inside——从 presentation.xml sldSz 读幻灯片比例算高度） |
| CreateInstance 忙态 | `0x8001010A` 瞬态 | **可持续数秒**（强杀后激活管线卡死） | 5 次 × 1s 重试，非瞬态失败才算实例不可用 |

### 防弹窗三板斧（先于一切 Open）

`DisplayAlerts=1`（ppAlertsNone）+ `AutomationSecurity=3`（禁宏）+ 恒 ReadOnly。残余卡死由 300s 文件级超时兜底。

### 队列与失败隔离

- 单 worker 串行消费 `batchCh`（缓冲 8）+ `jumpCh`（**无缓冲**：交接即消费，提交阻塞等消费方，消除「页边界错过提交」竞态）；跳转优先 = worker 每轮非阻塞摘 jumpCh + 批量在途时 preempt 页间摘取（D3「最多等一页」；导出快于提交时兜底走批次后主循环消费，语义自洽）。**停泊 select（第二 select）必须与 first select 同集收 jumpCh**（check P0 教训：停泊分支漏 jumpCh = 批次完成后的空闲期跳转无人接收，JumpToSlide 撞入队超时报 BUSY 假态，stale 请求被后续批量唤醒误执行）。
- 单文件失败（含超时强杀）→ `exporter.Discard()`（置脏，下任务重建会话）→ 继续后续文件（SRS 失败隔离）。**fake 注入测试会掩蔽 COM 实现的回写遗漏**——preempt 路径的 `jr.Result` 写回必须有真链路验证或契约断言。
- 关停顺序：`WaitScansDone()` → `WaitThumbsDone()` → `CloseAll()`（app.go shutdown）；WaitThumbsDone = 取消全部批次 → worker 退出 → exporter.Shutdown（Quit 自启实例 + 镜像 PID 强杀残留兜底）→ `closed` 置位拒绝新任务。

### 缓存键契约

- `<thumbsDir>/<tier>/<fileID>-<3位页>.png`；**slides.id 禁作缓存键**（每轮重解析 AUTOINCREMENT 删旧插新全变），files.id 稳定。`thumbPNGPath` 单点函数，PR2 记账表以同规则反查。

## 4. Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| 无 PowerPoint（探测不可用） | 入队静默跳过（无事件）；跳转返回 `E_ASSETS_THUMBS_UNAVAILABLE`（error，前端提示条常驻 + 入口隐藏） |
| 跳转排队超时 / 队列忙 | `E_ASSETS_THUMBS_BUSY`（warning 可重试；**禁复用 UNAVAILABLE**——语义为环境缺失，会误触降级 UI） |
| 跳转执行失败（文件不存在/损坏/COM 调用失败） | `E_ASSETS_THUMBS_JUMP_FAILED`（error，JumpToSlide 出口包装——nil 与既有 AppError 原样返回，COM 原始错误 `WrapAppError` 包裹保留根因 `errors.Is` 穿透） |
| 单 PPT 超时（300s） | 杀自启实例（精确 PID + 镜像快照）→ 文件记失败 → 继续后续 |
| 批次取消 | ctx 传播 → `done.Canceled=true` → 中断剩余文件 |
| 复用用户实例被模态弹窗卡死 | 结构性不可杀：90s 结果兜底 + slog 告警，后续任务逐个烧超时（接受，观测面已有） |
| 批次执行中 PowerPoint 变不可用 | 整批 Skipped 终态 done 事件 |

## 5. Good/Base/Bad Cases

- **Good**：新增 COM 能力时先跑探针定案（`scripts/ppt-com-probe` 模式：逐项验证 + 结论回填 research），接口按实测校准后再写实现。
- **Base**：新增分辨率档 = 改 `thumbTiers`（缓存子目录自动隔离）；新增跳转类操作 = 复用 jumpCh 优先通道。
- **Bad**：在主线程（非 STA worker）调 COM；硬编码 powerpnt 进程名；`jr.Result` 双写；用 `CallMethod` 取属性；依赖 `Presentations.Count` 做存在性判定。

## 6. Tests Required

- fake 注入（`fakeThumbExporter`）：串行事件序列（progress 1→N + done 汇总）、失败隔离（Discard + 继续）、取消（ctx 传播 + Canceled 终态）、同库批次覆盖、关停收口（worker 退出 + Shutdown 恰一次 + closed 拒绝）、跳转双路径（preempt 快路径 / 主循环兜底——断言用户可感知语义：跳转必达且恰执行一次，不断言路径）、不可用降级（无事件 + errThumbsUnavailable）。
- **chan 交接替代 sleep**：无缓冲 release chan 放行 = 「批次已进入第 N 页」确定性同步；done 事件经通知 sink channel 等待（禁 sleep 等待竞态）。
- **测试须跑 worker 存活性守护**：跳转失败/双路径用例后断言批次仍能执行（P0 二次写死锁的回归形态——fake 不写 Result 会永挂，超时守护兜底暴露）。
- COM 实现文件按 test-coverage-gate「系统调用核心不强行补测」接受 0% 覆盖，真机冒烟归集成验证。

## 7. Wrong vs Correct

### Wrong（jr.Result 双写——P0 队列停摆）

```go
func (q *thumbQueue) handleJump(jr *thumbJumpRequest) {
    if err := q.exporter.JumpOnly(jr); err != nil {
        jr.Result <- err // JumpOnly 契约内已写回——双写 = worker 永久阻塞在此
    }
    jr.Result <- nil
}
```

### Correct（契约单点写回，handleJump 只记日志）

```go
func (q *thumbQueue) handleJump(jr *thumbJumpRequest) {
    if !q.exporter.Available() {
        jr.Result <- errThumbsUnavailable // 此路径 JumpOnly 未执行，本层写回
        return
    }
    if err := q.exporter.JumpOnly(jr); err != nil {
        Logger().Warn("缩略图跳转失败", "path", jr.AbsPath, "err", err) // 结果已由 JumpOnly 写回
    }
}
```

### Wrong（CallMethod 取属性——「找不到成员」）

```go
pres, err := oleutil.CallMethod(app, "Presentations") // DISPATCH_METHOD → DISP_E_MEMBERNOTFOUND
```

### Correct（属性走 GetProperty）

```go
pres, err := oleutil.GetProperty(app, "Presentations") // DISPATCH_PROPERTYGET
```

---

**沉淀来源**：任务 10-14-ppt-com PR1（2026-10-14）。trellis-check 抓出 4 P0（handleJump 二次写死锁 / preempt 路径不写回 / 强杀快照 submitSafe 死锁 / defer Release 先于 Close）——「fake 掩蔽生产缺陷」形态：fake 自写 Result 掩蔽 COM 实现回写遗漏，新增强回归守护须断言 worker 存活性。
