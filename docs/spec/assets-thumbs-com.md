# 素材缩略图 COM 队列契约（assets-thumbs）

> 来源：任务 10-14-ppt-com PR1（2026-10-14，PowerPoint COM 缩略图链路基座：串行队列 + STA worker + 平台分叉）。探针实测依据：`.trellis/tasks/10-14-ppt-com/research/com-thumbnail-export.md` 文末「探针定案」节 + `scripts/ppt-com-probe/`（一次性工具已归档结论）。

## 1. Scope / Trigger

- 触发：改动 `service/assets_thumbs*.go`（队列调度 / COM 实现 / 桩）、缩略图事件、打开跳转链路、新增类似「外部进程 COM 自动化」能力时。
- 涉及文件：`service/assets_thumbs.go`（平台无关队列）、`service/assets_thumbs_windows.go`（go-ole）、`service/assets_thumbs_other.go`（桩）、`app_assets.go`（3 绑定方法）、`model/assets.go`（事件载荷）。

## 2. Signatures

| 层 | 签名 | 位置 |
|---|---|---|
| 接口 | `ThumbExporter`：`Available() bool` / `ExportPresentation(ctx, spec, thumbsDir, tiers, pages, preempt) (int, error)`（pages=nil 导全书，非 nil 为页白名单） / `JumpOnly(jr) error` / `Discard()` / `Shutdown()` | `assets_thumbs.go` |
| 队列 | `EnqueueAssetThumbs(libraryID, files, thumbsDir)` / `RequestAssetThumb(libraryID, fileID, relPath, slideNum, tierIndex) error`（详情侧单页单档按需生成）/ `JumpToSlide(absPath, slideNum) error` / `CancelAssetThumbs(libraryID) bool` / `WaitThumbsDone()` / `ThumbsAvailable() bool` / `GetAssetThumb(libraryID, fileID, slideNum, tierIndex) (string, error)` | `AssetLibraryService` |
| App 绑定 | `AssetThumbsAvailable() bool` / `JumpToAssetSlide(absPath, slideNum) error` / `CancelAssetThumbs(libraryID) bool` / `RequestAssetThumb(libraryID, fileID, relPath, slideNum, tierIndex) error` / `GetAssetThumb(libraryID, fileID, slideNum, tierIndex) (string, error)` | `app_assets.go` |
| 事件 | `assets-thumbs:progress`（AssetThumbsProgress）/ `assets-thumbs:done`（AssetThumbsResult，含 `onDemand` 标记）/ `assets-thumbs:error`（{libraryId, message}，当前触发点 = 入队队列满；沿用 assets-scan:error 不弹提示仅复位契约） | EventSink |
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
- **urgent 通道严格优先（2026-10-22，PPT 预览转换插队）**：COM 实现层（`goOleThumbExporter`）双通道——`taskCh`（普通，缩略图批量/跳转提交）+ `urgentCh`（无缓冲同构）；`submit`/`submitUrgent` 共用 `submitTo` 单点，消费决策单点 `receiveNextTask`（先非阻塞探测 urgentCh，未命中降级三路阻塞选择；worker 主循环与 STA 初始化失败排空路径共用）。语义红线：**单 worker 单 STA 契约不破**（仅通道 +1，禁双队列）；优先级只作用于「下一个被消费任务」的选取——正在执行的 COM 任务不可打断（阻塞 Invoke 契约），紧急任务最早在当前任务返回后的下一轮被消费；urgent 与普通任务共用同一 `comWorkerState`（publishOwned/清理路径/Discard/超时杀进程收口全部不动）。消费面：当前仅 `ConvertToPdf`（PPT 预览用户可感知交互）走 urgent，后续新增用户交互类高优任务复用 `submitUrgent`；缩略图批量/跳转链路恒普通通道零行为变化。新增通道时 `receiveNextTask` 两个 select 必须同步扩集（漏 urgentCh = 插队失效退化为 FIFO）。
- 单文件失败（含超时强杀）→ `exporter.Discard()`（置脏，下任务重建会话）→ 继续后续文件（SRS 失败隔离）。**fake 注入测试会掩蔽 COM 实现的回写遗漏**——preempt 路径的 `jr.Result` 写回必须有真链路验证或契约断言。
- **Export 目标路径必须绝对（2026-10-15 真机实锤，相对路径 = 确定性「发生意外」）**：`Slide.Export` 的 pngPath 由 **COM 服务器进程（wpp.exe/powerpnt.exe）在自己的 cwd 下解析**，非调用方进程——WPS 启动后自改 cwd，继承不可靠。dataDir 相对字面量（`"data"`）使 thumbs 目录相对化：进程内消费者（MkdirAll/Stat/ledger/读 API）按 app cwd 解析全部正常，唯独 Export 跨进程解析非法 → DISP_E_EXCEPTION 空 ExcepInfo，逐文件确定性失败且重试无效。修复 = `NewAssetLibraryService` 构造单点 `filepath.Abs` 收口（Abs 失败保底原值 + Warn），db/注册表/thumbs 全消费面同一绝对基准。**凡路径交给 COM 服务器的调用（Export/SaveAs/Open 参数）一律绝对路径**；早前冒烟通过属 fresh-spawn 实例 cwd 恰好继承的侥幸，非契约保证。
- **页级 Export 瞬态重试（2026-10-15 真机实证）**：`Slide.Export` 失败按 `isTransientExportError` 判定重试（同页同参至多 3 次，退避线性 1s/2s，`thumbExportMaxAttempts`/`thumbExportRetryDelay` var 注入；ctx 取消/超时**绝不重试**且判定前置）。瞬态口径：`DISP_E_EXCEPTION(0x80020009)` 且 ExcepInfo 空——go-ole 对空 bstrDescription 渲染字面量 `"<nil>"`，真机形态「发生意外。 (<nil>)」即 hr 的 FormatMessage 中文本地化**非 ExcepInfo 自带描述**（41MB/116 页真实文件冷启动未就绪实测，PowerPoint 探针同参延后成功佐证瞬态）；描述含「发生意外」的任一形态亦判瞬态；带实质业务描述的 0x80020009 与文件损坏类非瞬态立即返回。重试在 Export 调用内部不跨 preempt 边界，`jr.Result` 恰一次写回不受影响。注意：「发生意外」空 ExcepInfo 形态是**多因一果**（冷启动未就绪=瞬态 / 相对路径=确定性），先核路径绝对性再归因瞬态。
- **Discard 重建落定等待**：COM 单实例激活会重连尚未退净的旧进程（Quit 异步）——dirty 重建路径 `releaseApp` 后轮询 ownedPIDs 实际退出（200ms 间隔 / 3s 上界，var 注入，探针 `thumbOwnedExitProbe` 可注入）再 `CreateInstance`；超时未退继续重建走 0x8001010A 重试链兜底。owned 空（复用用户实例）不等待不探针（禁杀语义）。等待落定是重建语义的一部分，非可选优化——缺失则下批 277ms 内「重建」实为连到垂死实例秒败（真机实测）。
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
| 重建 | `rebuildLibrary` 步骤 2/2.1/2.2 | 保留 fileID（占位行显式主键恢复）+ 按内容指纹保留 PNG（详见 §9.2）；消失文件真删除并清图；收尾孤儿清扫 |

- DeleteLibrary 同构：`CancelAssetThumbs`（**缩小窗口**口径——取消异步不等待，在途单文件仍可能重建空 tier 目录残留少量 PNG，可手动清理）+ RemoveAll + dropLibrary。
- flush 快照与失效清理的竞态窗口残留幽灵行判定无害（同 key 重导出覆盖 / 读取不查 db / 淘汰删文件 NotExist 忽略），由 WHERE EXISTS 兜底落盘面。

### 补缺判定（扫描完成自动入队）

- **只判 small 档**（`batchBacklogTiers` 单点，2026-10-30 hd 退出批量）：hd 单张 1-3MB，全库体积远超默认配额（1024MB），参与补缺会因 LRU 淘汰形成「淘汰 → 整本重导 → 再淘汰」的永久循环。hd 改由详情侧 `RequestAssetThumb` 按需生成。
- **页命中唯一权威口径 = PNG 文件存在（os.Stat）**；记账表**不参与**补缺判定。原因：记账落盘攒批异步（dirty 未 flush 时 db 查不到），参与判定会把已生成页误判缺失，连续扫描重复入队重导出（真机冒烟抓出的幂等回归）——幂等不得依赖 flush 时序；记账 miss 而文件在的页由读取路径 touch 自愈补记账。
- **Stat 命中顺手补记账**（2026-10-30）：重建保留 PNG 时 `thumb_cache` 行被清表级联删除，而补缺收集的 Stat 是必做动作，命中即 `ledger.record`——零额外成本把记账补回，否则保留产物游离于 LRU 配额统计之外。反向（记账命中文件缺失）同样判缺失重导。
- 入队位置：`runLibraryScan` 尾部扫描 goroutine 内（结构性保证 `WaitScansDone` join 时批次已注册）——补缺收集的 Stat 开销（千文件 × 20 页 ≈ 4 万次，秒级）为此保证的接受成本，设计目标千级库，禁为省 Stat 移后台 goroutine 破坏该保证。
- 文件级粒度：任一页缺失 → 整文件重导出（COM 打开粒度 = 文件；已有页覆盖写幂等）。

### GetAssetThumb 读取 API

- `GetAssetThumb(libraryID, fileID, slideNum, tierIndex) (string, error)`：tierIndex 0=small/1=hd（**序号非宽度**——分辨率可配后宽度会漂移，序号稳定）；未生成/已淘汰返回 `("", nil)`（前端回退占位，非错误）；参数校验 slideNum≥1、fileID≥1、tierIndex 越界报错。
- 读路径零 db 访问（注册表锁 + 内存 record），与 flusher 不争单连接；命中即 `record` touch（LRU 新鲜度，攒批零同步写）。
- 前端消费（PR3）：返回串直接拼 `data:image/png;base64,<串>`，禁 `decodeBase64Utf8`（文本专用）禁裸 atob；量级口径 hd 档单张 1-3MB（base64 +33%）仅详情大图单张按需，画廊一律 small 档懒加载禁拉 hd。

### 沉淀来源（PR2）

- 真机冒烟（WPS 环境，`assets_thumbs_smoke_integration_windows_test.go`，`go test -tags=integration -run TestSmokeAssetsThumbsEndToEnd`）：抓出 Slide.Export 目录 P0（探针掩蔽）+ 补缺幂等回归（flush 时序依赖）两个真问题；D3 并存/记账落盘/幂等重扫/读取 API 全断言过（约 7s 端到端）。
- 独立审核（cavecrew-reviewer）抓出：FK 幽灵键吞整批（WHERE EXISTS 修）、重建不取消在途批次、DeleteLibrary 取消异步口径、补缺 Stat 风暴与入队位置约束的取舍登记。

## 9. 设置链路与前端消费（PR3，2026-10-14）

### 设置段 AssetThumbsSettings（model.AppSettings.assets，指针段）

- 五项：`auto`（扫描后自动生成，默认 true）/ `smallWidth`（320，clamp 120-800）/ `hdWidth`（1600，clamp 480-3840）/ `smallQuotaMB`（1024，clamp 64-8192）/ `hdQuotaMB`（1024，clamp 64-8192）。段指针形态区分「老配置缺失（→ 全默认）」与显式取值（WebServe 先例）；`NormalizeAssetThumbsSettings` 单点归一（nil → 默认 + 零值回退 + clamp），高按 16:9 自算（320×9/16=180、1600×9/16=900 与既有默认严格一致）。
- **SettingsService 缓存语义**（PR3 起）：Load 首读后缓存，缩略图读路径（画廊逐卡片）高频消费不能每次读盘。不变式：**Load 返回对象恒已 Ensure**（缓存 miss 分支 Ensure 后才入缓存；Save 落盘成功换缓存前同样 Ensure）；**Load 返回共享指针，消费方只读禁改**（原地改与并发读者竞态——需改先复制，app_webserve.go SetWebServeConfig 先例）；Save 锁跨「落盘+缓存换新」全程持有（两通道并发 Save 磁盘序与缓存序不可相反）；**运行期手改 settings.json 不再即时生效**（须重启）。Load 吞错降级无错误信号——SaveSettings 的 prev 对比在「缓存 miss + 盘损坏」窗口退化为默认值，存量非默认宽度会被误判清档重导（概率极窄、后果仅一次自愈重导，已登记接受）。

### 设置生效链路（SaveSettings → ApplyThumbsSettingsChange）

- diff（宽度变 → cleared 档；MB 变 → requota 档）→ 异步逐库：`resetThumbTier` = CancelAssetThumbs → `deleteThumbCacheTier`（db 单语句）→ `dropTier`（dirty）→ `os.RemoveAll` 档目录（**重试一次**——协作式取消不等待在途页导出，Windows 句柄可致首删部分失败）→ 全部 cleared 档清完后 `Auto && Available` 短路重入队补缺（一次一批覆盖双档）；requota 逐库 `checkQuota`（复用 evictTier 后台语义）。错误全部仅告警，不回滚保存。
- **关停竞态口径（结构性接受）**：应用 goroutine 不感知 shutdown——closed 置位后重入队被拒；窗口内批次滞留 batchCh、CloseAll 后 storeForThumbs 惰性重开做一次清档，均仅发生在退出收尾期，无 COM 调用无可观察危害。
- **清档残留口径**：RemoveAll 重试仍失败的旧分辨率 PNG 无链路再清（记账已删、Auto=false 不重导、补缺 PNG 存在性权威放行旧图）——照常展示直至重建索引/手动清理，注释登记非「自愈」。
- 测试直调 `applyThumbsSettingsChangeSync`（公开方法异步包装），chan 通知驱动禁 sleep。

### 前端消费契约（AssetThumb.vue + AssetsPanel.vue）

- **GetAssetThumb 返回串直接拼 `data:image/png;base64,<串>`**（禁 decodeBase64Utf8 文本专用 / 禁裸 atob）；画廊一律 tier=0，hd（tier=1）仅详情侧边栏单张 `hide-on-miss`。
- 懒加载：IntersectionObserver（threshold 0.05 触发一次即断）+ 无 IO 环境挂载即加载兜底（jsdom 单测免 stub）；组件内请求序号守卫（props 变更/卸载自增，在途响应写回前比对丢弃）；读取失败静默回退占位（逐卡弹错噪音大，后台 slog 已有日志——登记口径）。
- 补图代次：`thumbEpoch` 于 assets-thumbs:done 自增，AssetThumb 仅 miss 态响应 epoch 重拉（ready 卡不重拉）。
- 失败收集：progress 事件 `failed` 为批次内累计数，`failed > lastThumbsFailedCount` 时把 `currentFile` 记入失败集合；done 清集合 + 复位计数；**切库 watch 复位运行态**（thumbsRunning/thumbsProgress/失败集合——终态事件按 libraryId 过滤，不复位则进度条在新库页面永挂、取消打到新库恒 false 自相矛盾）。内存态刷新即清可接受（重扫/重试可恢复，PRD 认可）。
- 降级 UI：`AssetThumbsAvailable()` 探测（mount 时一次，探测失败按 false 处理不弹错）——false 时画廊顶部常驻信息条（非 error 红）+ 跳转入口隐藏 + 占位卡保留。
- 打开跳转：详情侧边栏按钮 → `JumpToAssetSlide(joinAssetPath(root, rel), slideNum)`（根去尾斜杠拼接，相对路径保持正斜杠）；错误分流 handleError 既有三码自动覆盖（BUSY warning 可重试 / UNAVAILABLE、JUMP_FAILED error）。
- 设置弹窗：GetSettings 整体拉取 → assets 段归一化回显（缺失/零值逐字段回退默认）→ 保存回填 assets 段整体提交；**基底 null 禁止保存**（GetSettings 失败时空壳整体提交会清掉其他全部设置段，拦截提示重开）。

### RetryAssetThumb 越界遏制

- relPath 来自前端（浏览器通道可达），`..`/绝对路径可逃逸库根让 PowerPoint 渲染库外文件、经 GetAssetThumb 回读内容——`FromSlash` 后先拒 `IsAbs`，`Join` 后 `filepath.Rel` 校验无 `..` 前缀才放行。

## 9.1 跳转零可见修复：owned 镜像持久化 + 残留回收 + WPS 可见性（D7，2026-10-15）

### 根因（真机两轮定性）

「打开并跳到此页」零可见效果（COM 链路日志全成功、无报错、无窗口）两个叠加根因：
1. **幽灵残留误判用户实例**：上会话缩略图批次自启的 wpp.exe 因会话崩溃（Shutdown 未跑）残留；下会话 `CreateObject` 命中幽灵（进程 diff 无新增）→ `ownedPIDs` 空 → 误判「复用用户已开实例（禁 Quit 禁杀）」→ 在后台幽灵进程内 Open+GotoSlide「成功」零可见。归属判定纯内存态，崩溃即失忆。
2. **WPS 应用级不可见**：COM `CreateInstance` 激活的 wpp.exe 应用级默认不可见；PowerPoint 的 `Open(WithWindow:=msoTrue)` 顺带显示窗口，**WPS 兼容层无此行为**——全新 owned 实例下 Open/GotoSlide/Activate 全成功仍零可见（research §2 按 PowerPoint 语义，WPS 差异真机实测补出）。

### owned 镜像持久化契约

- 文件 `data/assets/com_owned_pids.json`（`comOwnedPIDsFileName` 单点常量），内容 `{"pids":[...]}`，`util.SaveJSON` 原子写（禁 os.WriteFile）。
- **写**：`publishOwned`（worker 发布镜像时）锁外同步落盘；清空发布同样落盘 `{"pids":[]}`（本会话已无 owned 时旧残留集立即失效）。落盘失败仅 Warn（内存镜像仍有效，持久化只是跨会话辅助）。
- **读**：ensureApp **先于首个 CreateInstance** load-once（时序要害：首个 CreateInstance 可能直接新建进程跳过 diff 空分支，懒加载会读到本会话内容而非上会话残留，归属自证失效）；过滤文件 mtime 24h 窗口；损坏/无文件/超窗统一降级空集合。
- **清**：正常 Shutdown 收口后删文件（自有实例已 Quit，识别依据失效——**崩溃路径不执行该步，文件恰为下会话识别依据，这是修复成立前提**）。

### ensureApp 残留回收链

- diff 空（判「复用」）时 `classifyReusedInstance(aliveAfter, staleOwned)` 求交集：非空 = **自有残留自证**（非用户实例）→ `killPids` 精确强杀（禁 taskkill /IM 全杀）→ `waitForOwnedProcessesExit`（3s 上界，返回仍存活子集）→ 重走 0x8001010A 重试链重建，**单次回收禁循环**。
- **仍存活 PID 并回 `w.ownedPIDs`**（楔死实例归属仍自证，禁杀语义内；并回在 `diffPIDs` 之后——垂死 PID 在 before 快照已在集，先并回会被覆盖）——缺此步则 Quit 失败的楔死 owned 实例被当未知实例放行，D7 症状会话内复现。
- 重建仍 diff 空（WPS 单实例激活到另一已开实例等罕见态）→ 按原「复用用户实例」兜底放行（保守安全侧）。
- reclaim 覆盖写 `app` 前必须 `Release()` 垂死代理（指向已 kill 进程，对齐 releaseApp 先例）。
- 双保险防 PID 复用误杀：进程名白名单（officeServerPIDSet 枚举侧天然过滤）+ 24h mtime 窗口；残余风险 PRD 已登记接受。

### 跳转可见性兜底与日志

- `jumpInSession` 在 findOrOpen 后显式 `PutProperty(app, "Visible", msoTrue)`——WPS 兼容层不随 WithWindow 显示窗口；PowerPoint 已可见时幂等无害；失败仅 Warn。exportSession 的 ReadOnly 无窗口导出路径不经此函数不受影响；preempt 插队跳转共用本函数同获兜底。
- 成功路径日志 `"缩略图跳转完成"` 带 `file`/`slide`/`reused` 三字段（reused 区分「落在用户已开实例」vs「自启新窗口」——排障要害）。

## 9.2 hd 按需生成与增量重建（2026-10-30）

### hd 档按需生成

- 批量补缺只负责 small（`batchBacklogTiers`）；hd 由详情侧 `RequestAssetThumb(libraryID, fileID, relPath, slideNum, tierIndex)` 单页单档生成：PNG 已存在直接返回（幂等），同页在途经 `demandPending` 去重，异步入队后前端等该库 `assets-thumbs:done`（`onDemand=true`）重拉。
- **按需走独立通道 `demandCh`（容量 16）**：不进 `batches` 注册表，因此**不与批量批次互相取消**（`EnqueueAssetThumbs` 同库覆盖式取消只作用于批量）——共用注册表会让「打开详情」取消正在跑的批量、或反之让详情图永不出现。worker 消费顺序：jump → demand → batch（demand 只做非阻塞探测，阻塞 select 中与 batch 同集随机，按需最多等一个批量文件）。
- 按需批次用任务自带 `Tiers`/`Pages`（单档单页），**不发 progress 事件**（批量进度条不闪），done 载荷带 `onDemand` + 目标 `fileId`/`slideNum` 供前端把失败/取消落到当前详情卡（显示「重试生成」，否则 hd 永久占位无反馈）。
### hd 单页提效：会话缓存与邻页预取（2026-10-30）

- **文件会话缓存（LRU=1）**：`comWorkerState.cached` 挂单个已打开 Presentation（worker 线程独占，无锁）。命中条件 = `absPath` 相同 **且** 磁盘 size/mtime 未变（免 `findOrOpen`/`Open`、免重查 `Slides.Count`）；同文件连续翻页因此不再重复付出大文件 Open 成本。未命中先 `closeCachedPresentation()` 再开新会话。
- **清理单点**：非 reused 先 `Close` 后 `Release`（顺序颠倒会对已释放指针调 Close），reused 只 `Release`（**禁 Close 用户文档**）。接线点 = `releaseApp`（含 Discard 脏重建）、`cleanup`（quit/Shutdown）、**`jumpInSession` 开头**（跳转要 `WithWindow=msoTrue`，缓存的无窗口 Presentation 会被 `findOrOpen` 复用导致 GotoSlide 失败）、空闲超时 `thumbSessionIdle`（var 注入，默认 60s，仅在取到任务后执行前判定，urgent 优先级语义不变）。
- **邻页预取**：`thumbPrefetchPages(slideNum)` 返回 `[目标页, 目标页-1, 目标页+1]`（钳制 ≥1、去重、≤3 页）。**`Pages[0]` 恒为主目标页**——该约定同时是 `done.SlideNum` 回填、去重键、失败判定的基准，接口签名不变。预取页 `Item`/`Export` 失败只 Warn 继续，**仅主目标页失败返回错误**（邻页损坏不得把当前页标失败）。翻到相邻页时 PNG 已在，`GetAssetThumb` 直接命中，前端无需再请求。
- 排障日志：`缩略图会话缓存命中` / `缩略图会话已缓存` / `缩略图会话缓存失效（文件已变更或不可读，重开会话）` / `缩略图会话缓存切换（异文件）` / `缩略图会话缓存空闲超时关闭`。

- **批量在途的按需插队**：`handleBatch` 每个文件边界调 `drainDemand()` 非阻塞排空按需通道（对齐跳转的页间 preempt）——只在批间消费会让千页级批量（数分钟）挡住按需请求直到排队超时作废。嵌套按需任务不再插队（无递归放大）。
- **按需任务纳入取消面**：`demandTasks` 登记（不进 `batches`），`CancelAssetThumbs` 取消同库按需任务、`WaitThumbsDone` 取消全部——否则删库/清档的 `RemoveAll` 之后按需任务仍会重建档目录并写入旧分辨率 PNG，关停还要等满按需超时。`handleBatch` 统一 `defer b.Cancel()` 回收超时定时器。
- **页白名单批次禁走 `pruneBeyond`**：按「本次导出页数」裁剪会把第 1 页之后的既有产物误删（单页导出返回 n=1 时会清空整本后续页）。记账只按白名单页 Stat 成功者写入。
- 前端：详情大图区与全屏预览层均为 small 底图（tier=0 常显）+ hd 覆盖层（tier=1 `hide-on-miss` 绝对定位）；预览层按需请求由详情侧 watch 驱动，本层禁另发。`hideOnMiss` 语义为**仅 ready 渲染**（loading 期不渲染，否则灰底占位会盖住 small 底图）。详情打开/切页触发按需请求；onDemand done 带 `failed/canceled` 且 `fileId` 命中当前详情时，把该条目计入失败集显示「重试生成」。

### 增量重建（保留 fileID 与缩略图）

- 缓存键 `<fileID>-<3位页>.png` 依赖 `files.id` 稳定——重建若换 ID 则旧图全失效必然全库重导。故 `rebuildLibrary` 清表前快照全部文件行（`listRebuildPlaceholders`：id/path/size/mtime/hash/title），清表后 `restoreRebuildPlaceholders` **显式主键恢复占位行**（status=processing 强制重解析，重建语义不变）。
- **主键复用与 AUTOINCREMENT**：序列不回退，旧 ID 均 ≤ 历史最大值，显式插入不与新分配冲突；新文件仍从历史最大值继续递增。单事务写入，任一失败整体回滚。
- **缩略图目录不再整目录 `RemoveAll`**：`finalizeScanFile` 的重解析分支改为「内容指纹未变则跳过失效联动」——判据 = `scanContentUnchanged`（sha256 相同且解析成功）。字节相同 ⇒ 同版本解析器页数与版式必然一致。保守侧（hash 空 / 解析失败）一律判变化清图。
- 页数漂移兜底（解析器升级等）：新页数变多 → 补缺缺页判定触发整本重导；变少 → 越界旧页残留不参与展示，配额由 LRU 收口。
- 重建收尾四步：① `reconcileRebuildPageCounts` 页数漂移对账（旧页数取占位快照，与新解析页数不等即清该文件全部 PNG 与记账——覆盖「字节未变但解析器版本升级致页数变化」的盲区）；② `recordRetainedThumbLedger` 按实际存在的 PNG 给 small+hd 两档补记账（passive 变体只补 bytes 不刷 LRU；与 Auto 开关 / COM 可用性**解耦**，否则 Auto=false 或未装 PowerPoint 时保留产物游离于配额统计）；③ 占位行中磁盘已消失者真删除（`clearLostFiles`，不落 lost——否则旧路径行与改名后新行并存，哈希兜底误判）+ `pruneOrphanThumbFiles` 删除 fileID 不在索引中的 PNG（fail-safe：索引集合读取失败整体跳过）；④ 扫描中断时 `deleteRebuildPlaceholders` 删除仍 processing 的占位行，恢复「取消后索引零残留」。
- **清表与占位恢复同事务**（`resetIndexForRebuild`）：分属两事务时恢复失败会留下空索引，级联删除的标注随磁盘 VACUUM 备份（best-effort）之外无恢复手段——单事务保证失败整体回滚到重建前。
- **批量批次默认只导 small**：`handleBatch` 对 `Tiers == nil` 的任务取 `batchBacklogTiers`（不是全档），`RetryAssetThumb` / 设置变更重入队同获此口径；显式 `Tiers`（按需）不受影响。
- `CancelAssetThumbs` 仍在重建开头调用（防在途批次与清表竞态）。

## 4. Validation & Error Matrix


| 条件 | 行为 |
|---|---|
| 无 PowerPoint（探测不可用） | 入队静默跳过（无事件）；跳转返回 `E_ASSETS_THUMBS_UNAVAILABLE`（error，前端提示条常驻 + 入口隐藏） |
| 跳转排队超时 / 队列忙 / **按需生成队列满（demandCh 容量 16）** | `E_ASSETS_THUMBS_BUSY`（warning 可重试；**禁复用 UNAVAILABLE**——语义为环境缺失，会误触降级 UI） |
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
- urgent 优先级（`TestGoOleThumbExporter_UrgentJumpsQueuedNormalTask`，Windows 原生零 COM）：普通任务执行中（body 阻塞）+ 普通提交方挂起于 taskCh（FIFO 存量），urgent 提交后经 guReady 无缓冲会合 + 放行断言先于存量被消费；收尾断言 worker 存活（urgent 消费后普通通道仍消费）。
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
