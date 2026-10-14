# 素材缩略图 COM 队列契约（assets-thumbs）

> 来源：任务 10-14-ppt-com PR1（2026-10-14，PowerPoint COM 缩略图链路基座：串行队列 + STA worker + 平台分叉）。探针实测依据：`.trellis/tasks/10-14-ppt-com/research/com-thumbnail-export.md` 文末「探针定案」节 + `scripts/ppt-com-probe/`（一次性工具已归档结论）。

## 1. Scope / Trigger

- 触发：改动 `service/assets_thumbs*.go`（队列调度 / COM 实现 / 桩）、缩略图事件、打开跳转链路、新增类似「外部进程 COM 自动化」能力时。
- 涉及文件：`service/assets_thumbs.go`（平台无关队列）、`service/assets_thumbs_windows.go`（go-ole）、`service/assets_thumbs_other.go`（桩）、`app_assets.go`（3 绑定方法）、`model/assets.go`（事件载荷）。

## 2. Signatures

| 层 | 签名 | 位置 |
|---|---|---|
| 接口 | `ThumbExporter`：`Available() bool` / `ExportPresentation(ctx, spec, thumbsDir, tiers, preempt) (int, error)` / `JumpOnly(jr) error` / `Discard()` / `Shutdown()` | `assets_thumbs.go` |
| 队列 | `EnqueueAssetThumbs(libraryID, files, thumbsDir)` / `JumpToSlide(absPath, slideNum) error` / `CancelAssetThumbs(libraryID) bool` / `WaitThumbsDone()` / `ThumbsAvailable() bool` / `GetAssetThumb(libraryID, fileID, slideNum, tierIndex) (string, error)` | `AssetLibraryService` |
| App 绑定 | `AssetThumbsAvailable() bool` / `JumpToAssetSlide(absPath, slideNum) error` / `CancelAssetThumbs(libraryID) bool` / `GetAssetThumb(libraryID, fileID, slideNum, tierIndex) (string, error)` | `app_assets.go` |
| 事件 | `assets-thumbs:progress`（AssetThumbsProgress）/ `assets-thumbs:done`（AssetThumbsResult）/ `assets-thumbs:error`（{libraryId, message}，当前触发点 = 入队队列满；沿用 assets-scan:error 不弹提示仅复位契约） | EventSink |
| 缓存命名 | `thumbPNGPath(thumbsDir, tierKey, fileID, slideNum)` = `<thumbsDir>/<tier>/<fileID>-<3位页>.png` | 单点函数 |
| 分叉构造 | `newThumbExporter()`（_windows=go-ole / _other=桩）/ `newThumbQueue(exp, sinkFn, ledger)`（第三参 = 记账 ledger，五处构造点全部注入） | 平台分叉点 |
| 记账 | `thumbCacheLedger`：`record` / `invalidateFile` / `dropLibrary` / `flushLibrary` / `pruneBeyond` / `Shutdown`；`storeForThumbs`（锁内校验+取连接单点） | `assets_thumbs_ledger.go` |

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
- **`Slide.Export` 不自建父目录**：目标 tier 目录缺失时 E_FAIL（「发生意外」无定位信息）——`exportSession` 导出前必须逐档 `os.MkdirAll`（PR2 真机冒烟抓出的 P0；PR1 探针预建导出目录曾掩蔽）。

### 导图与用户文档并存（D3 前提，真机实测成立）

- 复用用户实例（快照 diff 为空）场景下，无窗口 `ww=0` 只读打开 + `Slide.Export` 正常工作（变体矩阵实测：无窗口/带窗口/最小化/复用窗口化文档自身导出全通过）；全新实例无窗口导出（PR1 P5）与并存场景互不推导，两条都须真机验证。

### fixture 构造（冒烟/集成验证）

- **手写 OOXML zip fixture 会被 WPS 拒开**（`Presentations.Open` E_FAIL `0x80020009`）——真机验证一律 **COM 自产 fixture**（新建 → `Slides.Add(i, 12)` 逐页空白版式 → `SaveAs(path, 24)` → Close），合法性与真实文档同级（`service/assets_thumbs_smoke_integration_windows_test.go` `smokeProduceFixtures` 同款）。
- 冒烟前置硬性要求：进程预检发现 `powerpnt.exe/wpp.exe` 残留即 skip 拒跑（冒烟会 Quit 唯一实例，防误关用户文档）；上一轮失败可能残留孤儿 wpp.exe，重跑前 `tasklist` 核查。

## 8. 记账与失效联动（PR2，2026-10-14）

### thumb_cache 表（schema v3）

- 列：`file_id REFERENCES files(id) ON DELETE CASCADE` + `tier` + `page`，主键 `(file_id, tier, page)`；`bytes` + `last_access_ns`（LRU 依据——NTFS atime 落盘延迟 ≤1h 且受系统策略控制，**应用层自记时间戳**，禁依赖文件系统 atime/mtime 做访问序）。索引 `idx_thumb_cache_access(tier, last_access_ns)` 供淘汰游标。
- 双保险口径：外键级联兜底 + 代码显式 DELETE（失效/重建路径），两者并存防 pragma 漂移静默漏清。

### ledger 攒批（assets_thumbs_ledger.go）

- 写路径只进内存 dirty buffer（`record` 微秒级持锁），攒批 **200 条**或 **45s**（`thumbLedgerFlushInterval` var 注入点）触发单 goroutine flusher；落盘 **单事务** `INSERT OR REPLACE ... SELECT ... WHERE EXISTS(SELECT 1 FROM files WHERE id = ?)`——**幽灵键（file_id 已不在 files 表）必须 WHERE EXISTS 过滤**：外键已启用，裸 INSERT 会因单个毒键（stale touch / 快照竞态 / 重建在途旧 ID）令**整批事务失败丢弃**；落盘行数与批大小差额告警登记。硬约束「禁逐条 UPDATE」指正常路径；flush 快照（持 mu 取出 dirty 后放锁）→ 锁外 db/文件操作。
- `Shutdown()`：同步 flush 全部有 dirty 的库 → close(quit) → wg.Wait；由 `WaitThumbsDone` 尾部调用（**连接关闭前**），置位 stopped 后 record 丢弃（幂等可重入）。

### LRU 淘汰

- 只允许后台异步路径：`flushLibrary` 落盘成功后对涉及档位 `evictTier`（SUM 超配额 → `last_access_ns` 最旧优先游标收集至 90% 水位 → rows Close 后单事务批量 DELETE + 逐个删 PNG，NotExist 忽略）。**禁在 record/读取路径同步淘汰**。配额 small/hd 各 1GB 常量（`thumbTierQuotaBytes` var，PR3 设置界面接）。
- 页数缩减对账 `pruneBeyond`（导出成功后按返回页数清越界记账+PNG）属对账语义非配额淘汰，仅成功路径调用（失败文件旧图仍有效）。

### 失效联动三路径（记账 + PNG + dirty 三处状态同清）

| 路径 | 触发点 | 动作 |
|---|---|---|
| 重解析 | `processScanFile` exists 分支（markProcessing 后、parse 前——成败两分支都覆盖） | `invalidateFileThumbs`：显式 DELETE 记账 + `ledger.invalidateFile` + 前缀删 PNG（`fileID + "-"` 完整形态防 `102`/`1023` 误伤） |
| 文件删除 | `scanLibrary` 尾部 gone 清理 | `deleteFilesByPaths` 事务内显式 DELETE 记账（返回 fileIDs）→ 事务提交后删 PNG + `invalidateFile` |
| 重建 | `rebuildLibrary` 步骤 2/2.1/2.2 | 清表（事务内）+ `RemoveAll` 整目录 + **`dropLibrary` 清 dirty**（残留 dirty flush 时 FK 失败整批丢弃）+ 开头 `CancelAssetThumbs`（防在途批次向已清目录写旧 ID PNG——旧 ID 永不复用，残留超配额永久孤儿） |

- DeleteLibrary 同构：`CancelAssetThumbs`（**缩小窗口**口径——取消异步不等待，在途单文件仍可能重建空 tier 目录残留少量 PNG，可手动清理）+ RemoveAll + dropLibrary。
- flush 快照与失效清理的竞态窗口残留幽灵行判定无害（同 key 重导出覆盖 / 读取不查 db / 淘汰删文件 NotExist 忽略），由 WHERE EXISTS 兜底落盘面。

### 补缺判定（扫描完成自动入队）

- **页命中唯一权威口径 = PNG 文件存在（os.Stat）**；记账表**不参与**补缺判定。原因：记账落盘攒批异步（dirty 未 flush 时 db 查不到），参与判定会把已生成页误判缺失，连续扫描重复入队重导出（真机冒烟抓出的幂等回归）——幂等不得依赖 flush 时序；记账 miss 而文件在的页由读取路径 touch 自愈补记账。反向（记账命中文件缺失）同样判缺失重导。
- 入队位置：`runLibraryScan` 尾部扫描 goroutine 内（结构性保证 `WaitScansDone` join 时批次已注册）——补缺收集的 Stat 开销（千文件 × 20 页 ≈ 4 万次，秒级）为此保证的接受成本，设计目标千级库，禁为省 Stat 移后台 goroutine 破坏该保证。
- 文件级粒度：任一页缺失 → 整文件重导出（COM 打开粒度 = 文件；已有页覆盖写幂等）。

### GetAssetThumb 读取 API

- `GetAssetThumb(libraryID, fileID, slideNum, tierIndex) (string, error)`：tierIndex 0=small/1=hd（**序号非宽度**——分辨率可配后宽度会漂移，序号稳定）；未生成/已淘汰返回 `("", nil)`（前端回退占位，非错误）；参数校验 slideNum≥1、fileID≥1、tierIndex 越界报错。
- 读路径零 db 访问（注册表锁 + 内存 record），与 flusher 不争单连接；命中即 `record` touch（LRU 新鲜度，攒批零同步写）。
- 前端消费（PR3）：返回串直接拼 `data:image/png;base64,<串>`，禁 `decodeBase64Utf8`（文本专用）禁裸 atob；量级口径 hd 档单张 1-3MB（base64 +33%）仅详情大图单张按需，画廊一律 small 档懒加载禁拉 hd。

### 沉淀来源（PR2）

- 真机冒烟（WPS 环境，`assets_thumbs_smoke_integration_windows_test.go`，`go test -tags=integration -run TestSmokeAssetsThumbsEndToEnd`）：抓出 Slide.Export 目录 P0（探针掩蔽）+ 补缺幂等回归（flush 时序依赖）两个真问题；D3 并存/记账落盘/幂等重扫/读取 API 全断言过（约 7s 端到端）。
- 独立审核（cavecrew-reviewer）抓出：FK 幽灵键吞整批（WHERE EXISTS 修）、重建不取消在途批次、DeleteLibrary 取消异步口径、补缺 Stat 风暴与入队位置约束的取舍登记。

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
