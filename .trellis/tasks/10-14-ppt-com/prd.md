# 素材管理第二批——PPT COM 缩略图链路

## Goal

在素材管理 MVP（任务 10-13-dam-ppt，纯 Go 文本索引 + FTS5 检索 + 占位卡画廊）之上落地第二批：PowerPoint COM 缩略图生成链路。覆盖 SRS §2.2.2（缩略图生成：COM 串行队列 + 超时强杀 + 失败隔离 + LRU 缓存）、§2.2.5（打开跳转指定页）、§7（COM 稳定性风险）。画廊占位卡替换为真实缩略图（懒加载 + base64 通道），检索命中可打开原 PPT 跳指定页；无 PowerPoint 环境整体降级（仅文本索引，UI 明示）；Linux 零编译影响。

## What I already know

### 代码库现状（已探明）
- `go-ole v1.3.0` 已在 go.mod（indirect，升 direct 零成本）；平台分叉先例充足（`service/fileoperation_windows.go` / `_other`、`util/clipboard_windows.go` 等）。
- assets service 现有结构：`assets_db/library/parse/scan/schema/fts/search*.go`；slides 表已有（id/file_id/slide_num/title/text/notes）；`AssetSlideEntry` 无缩略图字段。
- 扫描链路：增量判定 mtime+size、单文件失败隔离、cancels 注册表取消、`assets-scan:progress/done/error` 事件、`WaitScansDone` 关停收口。
- 前端：`AssetsPanel.vue` 画廊分页（PAGE_SIZE=50）+ 双代次守卫（librarySeq/searchSeq）+ `decodeBase64Utf8` 规约（`ReadFileBytes` base64 通道）。
- EventSink 契约：`web_serve.go` `applySink` 已含 `assetsSvc.SetEventSink`；事件载荷带 `libraryId` 前端按库过滤。
- AppServices 装配两处 + wailsjs 三处绑定同步。

### SRS 约束（§2.2.2/§2.2.5/§7 摘录）
- COM 调用必须串行、STA 模式，**全局同一时刻最多 1 个 COM 任务**，禁止并行。
- 单 PPT 超时（推荐 300s）强杀 powerpnt，任务标记失败不阻塞队列；失败隔离 = 终止当前 COM 会话、重启实例继续后续。
- 缓存：素材库缓存目录独立；多分辨率（画廊小图 + 高清预览）；LRU 淘汰 + 大小上限可配置。
- 无 PowerPoint：跳过缩略图仅文本索引；打开跳转降级为提示手动打开。
- §7：文件被占用时标记「跳过(文件被占用)」支持重试；残留 powerpnt 进程清理。

## Decision (ADR-lite)

### D1：批量触发时机 = 全自动（2026-10-14 用户确认）
**Context**：SRS §2.2.2 写「后台批量生成」，触发方式待定。
**Decision**：扫描/重建完成后自动入队生成；幂等增量补缺（已有缩略图且源文件未变的页跳过）；设置项「自动生成缩略图」默认开可关。
**Consequences**：首次大库扫描后后台静默跑数分钟（进度事件可见 + 可取消兜底）；无 COM 环境静默跳过。

### D2：缓存配置全暴露（2026-10-14 用户确认）
**Decision**：素材管理设置区可改 5 项——自动生成开关、小图分辨率、高清分辨率、小图缓存上限、高清缓存上限。
**Consequences**：改分辨率须清该档缓存全量重生成（设置变更时重入队）；上限变更触发惰性淘汰检查。

### D3：COM 队列 = 单队列 + 跳转插队（2026-10-14 用户确认）
**Context**：SRS 硬约束全局同一时刻最多 1 个 COM 任务；PowerPoint 单实例应用，双实例不可行。
**Decision**：缩略图批量与打开跳转共享同一串行队列互斥执行；跳转高优先级插队（当前页导完即执行，最多等一页时间）；跳转完成后批量在同实例继续（`WithWindow:=msoFalse` 另开 ReadOnly Presentation 导图）。
**Consequences**：用户窗口与后台导图并存（PowerPoint 成熟能力）；「WithWindow:=msoFalse 是否干扰用户已开文档窗口」列入首日实测清单，实测不行退化为 D3'（跳转后批量挂起等 PowerPoint 关闭）。

### D4：降级 UI + 失败重试（2026-10-14 用户确认）
**Decision**：无 COM 环境——画廊顶部常驻提示条「未检测到 PowerPoint，仅文本索引，缩略图与跳转不可用」，跳转入口隐藏，卡片保持占位样式。生成失败页（超时/占用/损坏）——回退占位样式，详情侧边栏显示失败原因 + 「重试」按钮（单文件入队，复用队列链路）。
**Consequences**：覆盖 SRS §7「文件占用支持重试」；用户关闭占用程序后可就地恢复，无需全量重建。

### 已采纳 research 推荐的技术决策（待实施验证）
- COM 链路：专用 goroutine（LockOSThread + CoInitializeEx STA，放行 S_FALSE）→ `oleutil.CreateObject("PowerPoint.Application")` IDispatch late binding；导出 `Open(ReadOnly, WithWindow:=msoFalse)` + `Slide.Export(path,"PNG",w,h)`；跳页 `WithWindow:=msoTrue` + `View.GotoSlide(n)` + `Activate`。PowerPoint Application 无 Visible 属性（Excel 才有），窗口控制全靠 WithWindow。
- 超时强杀：进程快照 diff 定位自启 powerpnt PID，杀之让阻塞 Invoke 以 0x800706BA 返回；**禁 `taskkill /IM` 全杀**（误杀用户自开的 PPT）。
- 无 COM 检测：注册表预检 + CreateInstance 兜底 + 包级缓存（避免重复探测）。
- 缓存键：`thumbs/{small,hd}/<file-id>-<3位页>.png`；**slides.id 每轮重解析全变（AUTOINCREMENT 删旧插新）禁作缓存键，files.id 稳定**。
- 记账：SQLite 库内新表（schema v3 迁移）+ 内存 LRU 攒批落盘（200 条或 30-60s 单事务），atime 不可用（NTFS 延迟落盘）。
- 淘汰：后台异步 + 惰性触发，禁写入同步淘汰。

## Requirements

1. **COM 串行队列**（service 层，`_windows`/`_other` 双文件分叉）：全局同一时刻最多 1 个 COM 任务、STA 模式；缩略图批量与打开跳转共享同一队列互斥执行，跳转高优先级插队（D3）；单 PPT 超时（~300s，可配置）进程快照 diff 强杀自启 powerpnt；失败隔离（终止当前 COM 会话重启实例继续后续）；禁 `taskkill /IM` 全杀。
2. **缩略图批量生成**：扫描/重建完成后自动入队（D1，幂等增量补缺）；双档分辨率（默认小图 320px / 高清 1600px，可配）；失效联动——源 PPT 变更重扫描后按 file_id 清缓存重入队、文件删除同步清缓存、页数缩减靠记账表对账清理越界页。
3. **缓存管理**：目录 `data/assets/<lib-id>/thumbs/{small,hd}/<file-id>-<NNN>.png`（slides.id 禁作缓存键）；记账走 SQLite 库内新表（schema v3 迁移）+ 内存 LRU 攒批落盘（200 条或 30-60s 单事务）；后台异步惰性淘汰（禁写入同步淘汰）；上限默认小图/高清各 1GB。
4. **画廊与详情**：占位卡替换为真实缩略图（懒加载、`ReadFileBytes` base64 通道、`decodeBase64Utf8` 规约）；详情侧边栏高清大图；失败页回退占位 + 失败原因 + 「重试」按钮（D4）。
5. **打开跳转**：检索命中幻灯片点击打开原 PPT 跳指定页（`View.GotoSlide(n)` + `Activate`）；文件被占用走 ReadOnly 打开规避。
6. **降级 UI**：无 COM 探测（注册表预检 + CreateInstance 兜底 + 包级缓存结果）→ 提示条 + 入口隐藏（D4）。
7. **设置区**（素材管理面板内 5 项，D2）：自动生成开关、小图分辨率、高清分辨率、小图缓存上限、高清缓存上限；分辨率变更清该档缓存重入队；上限变更触发惰性淘汰。
8. **Linux 零编译影响**：go-ole 仅 `_windows` 文件引用，`_other` 桩实现。
9. **进度/取消/关停**：`assets-thumbs:*` 事件（progress 带 libraryId / done 仅复位 / error 不弹提示，沿用 assets-scan 契约）；cancels 注册表可取消；App 关停收口——取消队列 + 等 COM 会话退出 + 清理自启 powerpnt 残留（对齐 WaitScansDone 模式）。

## Acceptance Criteria

- [ ] 扫描完成后自动批量生成缩略图，画廊占位卡替换为真实缩略图（懒加载，滚动追加不卡顿）
- [ ] 详情侧边栏展示高清大图
- [ ] 重复扫描/重建不重复生成（缓存命中跳过）；源文件变更后缩略图失效重生成；删除文件缓存同步清理
- [ ] COM 队列串行：任一时刻最多 1 个自启 powerpnt 实例；批量进行中点击跳转最多等一页导出时间
- [ ] 单 PPT 超时强杀自启进程（不误杀用户自开 powerpnt），失败隔离继续后续队列；失败页回退占位 + 侧边栏可重试
- [ ] 检索命中幻灯片点击打开原 PPT 并跳到指定页
- [ ] 无 PowerPoint 环境：提示条常驻、跳转隐藏、占位卡保留、仅文本索引正常
- [ ] LRU 淘汰 + 缓存上限生效（超限后台淘汰最久未访问条目）
- [ ] 设置区 5 项可改并即时生效（分辨率变更触发该档重生成）
- [ ] `assets-thumbs:progress/done/error` 事件前端可见，批量可取消
- [ ] App 关停时队列收口：无自启 powerpnt 残留进程、无半写缓存文件
- [ ] Linux（GOOS=linux）编译通过，COM 代码不参与编译
- [ ] 服务层接口 fake 注入：队列调度/记账/淘汰/失效联动单测覆盖，不依赖本机 PowerPoint

## Definition of Done

- 新增代码单测覆盖达门禁（service ≥76%）；COM 层接口 fake 注入可测
- `wails generate module` 三处绑定同步；`npm run build` 通过
- Linux 构建通过（`_other` 桩）
- README.md / docs/功能说明.md 更新；新错误码同步 `model/app_error.go` + `frontend/src/utils/error.js`
- spec 沉淀：assets-management.md 增补或新主题文档（COM 队列 + 缓存契约）

## Out of Scope (explicit)

- 拆分 UI（SRS §2.2.3，另一批）
- 幻灯片单页独立标签/评分/备注（P1）
- OCR、颜色检索、以图搜图
- 复制模式导入
- .ppt 老格式（仅文件级索引，不做单页缩略图——SRS §1.4.3）
- 文件丢失检测与路径重定位

## Technical Notes

- SRS：docs/原始需求/202610-素材管理系统需求说明书-V1.0.md
- 上一批 PRD：.trellis/tasks/archive/2026-10/10-13-dam-ppt/prd.md
- 契约必读：docs/spec/assets-management.md、docs/spec/pptx-split.md、docs/spec/linux-platform.md
- 首日实测清单（research 文末 7 项，一次跑定）：Slide.Export 纵横比行为（决定前端卡片占位宽高比）、ppAlertsNone 数值、显式尺寸是否绕开 ExportBitmapResolution、WithWindow:=msoFalse 与用户已开文档并存（D3 前提）、占用文件 ReadOnly 打开、0x800706BA 阻塞返回时序、进程快照 diff 归属判定

## Research References

- [research/com-thumbnail-export.md](research/com-thumbnail-export.md) — go-ole COM 导出缩略图链路（七问全解 + 落地建议 10 条 + 待实测 7 项）
- [research/lru-thumbnail-cache.md](research/lru-thumbnail-cache.md) — 磁盘缩略图缓存 LRU 实践（记账/淘汰/命名/失效联动 + 落地建议 10 条）
