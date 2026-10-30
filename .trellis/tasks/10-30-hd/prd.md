# 素材缩略图生成性能优化：hd 按需 + 增量重建

## Goal

素材管理「重建索引」后缩略图生成耗时 4–5 分钟（53 文件 / 1208 页 / 双档 2416 次 COM Export），规模放大 3 倍后预计 12–15 分钟。本任务把重建后的重导出量压到「仅变更文件」，并让高清档退出批量预生成改为按需生成，消除隐性淘汰循环。

附带修复的既有缺陷：hd 档默认配额 1024MB < 全库 hd 实际体积（1208 页 × 1–3MB = 1.2–3.6GB），LRU 淘汰后补缺链路按 PNG 存在性判缺失且为文件级粒度，形成「淘汰 → 整本重导 → 再淘汰」的循环。

## Requirements

### R1 hd 档退出批量补缺

- `collectThumbsBacklog` 只按 small 档判定缺失，hd 档不再进入批量入队。
- hd 档不再因 LRU 淘汰触发整本重导，淘汰循环结构性消除。
- hd 档的记账、配额淘汰、读取 API 语义保持不变。

### R2 hd 按需生成链路

- 新增后端能力：按「库 + 文件 + 页 + 档位」生成单页单档缩略图，走既有串行队列（不新增 COM worker）。
- 批次任务结构扩展为可选档位与可选页码（nil = 全档全页，保持既有批语义不变）。
- 新增 App 绑定方法 `RequestAssetThumb(libraryID, fileID, slideNum, tierIndex) error`，前端详情打开 hd 缺失时调用。
- 事件复用 `assets-thumbs:progress` / `assets-thumbs:done`，载荷增加 `onDemand` 标记；前端对 onDemand 批次不显示批量进度条。

### R3 详情侧 hd 兜底展示

- 详情侧边栏大图区改为「small 底图 + hd 覆盖」：small 立即可见（略糊），hd 就绪后替换覆盖层。
- 打开详情时若 hd 缺失，自动触发 R2 的按需生成；完成后经 `thumbEpoch` 重拉显示。
- hd 生成失败保留既有失败提示与「重试生成」入口（重试链路改为按需单页生成，不再整本重导）。

### R4 重建索引保留 fileID

- 重建前备份 `path → {fileID, hash, size, mtimeNs, pageCount}`。
- 重建清表后按路径命中旧映射的文件显式复用旧 `files.id`（显式插入主键，不依赖 AUTOINCREMENT）。
- 标注重绑（文件标签 / 页级标注）沿用现有 path + hash 两级匹配，不改变语义。

### R5 重建按内容指纹保留缩略图

- hash 相同且页数相同 → 保留该文件全部 PNG，不重导。
- hash 或页数变化 / 新增文件 → 删除该文件全部 PNG 与记账，交补缺链路重导。
- 重建收尾清扫孤儿 PNG：删除 fileID 不在新索引集合中的缩略图文件与记账行。
- 缩略图目录不再整体 `RemoveAll`。

### R6 既有语义保持不变

- small 档批量补缺、失败隔离（`Discard`）、取消（`CancelAssetThumbs`）、关停收口（`WaitThumbsDone`）、记账攒批与 LRU 淘汰语义全部不变。
- 单文件导出超时（300s）与页级瞬态重试不变。
- 全局单 COM worker 串行约束不变（多实例改造不在本任务）。

## Acceptance Criteria

- [ ] 重建索引且文件零变更时，假 exporter 记录的 Export 调用次数为 0（单测断言）。
- [ ] 重建后文件内容变更时，仅该文件的 PNG 被重建，其余文件 Export 次数为 0。
- [ ] hd 档被 LRU 淘汰后再次扫描，不产生任何批量入队（单测断言）。
- [ ] small 档全量补缺场景行为与基线一致（回归用例）。
- [ ] `RequestAssetThumb` 对 hd 缺失页生成成功后，`GetAssetThumb` 可读到该 PNG；已存在时不重复入队。
- [ ] 详情侧 hd 缺失时先显示 small 底图，hd 就绪后替换，且不跳版。
- [ ] 重建后孤儿 PNG 被清除（文件删除、页数减少两类场景）。
- [ ] 后端 service 覆盖率 ≥76%、前端 ≥70% 门禁通过。

## Definition of Done

- 新增判定逻辑均有单测（档位过滤、页粒度任务、重建 ID 复用、PNG 保留/清扫、按需去重）。
- 覆盖率门禁通过；`go test ./...` 与 `npm test` 全绿。
- `docs/spec/assets-thumbs-com.md`、`docs/spec/assets-management.md`（重建语义）同步更新；`README.md` 确认是否需要更新。
- 真机（WPS 环境）冒烟：重建零变更库、变更单文件库、详情 hd 首开三个场景。
- 性能数据记录到 PRD 技术备注（当前规模重建耗时、常规扫描耗时），不设硬门禁。

## Technical Approach

### 队列层：批次任务增加可选档位与页

`thumbBatchTask` 增加 `Tiers []thumbTier`、`Pages []int`，nil 表示全档全页。`handleBatch` 以 `b.Tiers` 优先（nil 回退 `q.tiers()`），`ExportPresentation` 增加页白名单参数（nil = 全页）。既有入队调用点不变，新增按需入队构造单页单档任务。

### 按需 API 与去重

`RequestAssetThumb` 校验顺序：库存在 → COM 可用 → 参数非法（tierIndex 越界、slideNum < 1、fileID < 1）→ 源文件存在性。已存在 PNG 直接返回 nil（幂等，不重复入队）。入队走 `EnqueueAssetThumbs` 同构路径，事件载荷带 `onDemand=true`。

### 重建链路改造

1. `rebuildLibrary` 步骤 1 增加 `path → fileMeta` 备份（含旧页数）。
2. 清表 + 清记账，**不再** `RemoveAll` 缩略图目录。
3. 扫描新增路径改为「命中旧映射则显式插入旧 id」；插入函数新增显式主键变体。
4. 每个文件扫描完成后比对：hash 与页数均与旧记录一致 → 保留 PNG；否则 `invalidateFileThumbs`（既有单点，删记账 + 删 PNG 前缀）。
5. 重建收尾：枚举 thumbs 各档目录，删除 fileID 不在新索引集合中的 PNG；`thumb_cache` 中对应用户已删文件的行随外键级联清理。
6. 标注重绑 defer 保持不变。

### 前端

详情块改为两层：small 层常显（复用 `AssetThumb` tier=0）、hd 层绝对定位覆盖（tier=1，ready 才显示）。打开详情时若 hd 层 miss，调用 `RequestAssetThumb`；收到该库 `assets-thumbs:done`（含 onDemand）后 `thumbEpoch++` 触发 hd 层重拉。批量进度条过滤 onDemand 批次。

### 跨层同步点

新增 App 方法须同步 `frontend/wailsjs/go/main/App.js`、`App.d.ts`、`models.ts` 三处；新增错误码（若有）三处同步（`app_error.go` + `error.js`）。

## Decision (ADR-lite)

**Context**：重建索引后全量重导的根因是「清表换 ID + 整目录删除」；重建耗时另有双档导出与 hd 淘汰循环叠加。

**Decision**：本轮只做「hd 按需 + 增量重建」，不动 COM 单实例约束；多实例并发改造作为独立探针子任务，结论确定后再立项。

**Consequences**：改动集中在扫描/补缺/队列/详情前端，不触及 COM 实现层，风险可控；收益依赖「重建时文件未变更」的比例，首次建库与全量变更场景仍为全量导出，需依靠后续多实例方案解决。

## Out of Scope

- COM 多实例池改造：仅在子任务中做可行性探针，不在本任务实现。
- 单次导出 + 本地派生（hd → small 降采样）、BMP/WebP 输出格式改造。
- 整本一次 `Presentation.Export` 替代逐页导出。
- Shell 缩略图（`IShellItemImageFactory`）快速占位层。
- 补缺判定的 `os.Stat` → `ReadDir` 提速（当前规模秒级，收益边缘）。

## Technical Notes

- 关键文件：`service/assets_scan.go`（扫描/重建/补缺）、`service/assets_thumbs.go`（队列/API）、`service/assets_thumbs_windows.go`（COM 导出）、`service/assets_thumbs_ledger.go`（记账/淘汰）、`frontend/src/components/AssetThumb.vue`、`AssetsPanel.vue`。
- 关键约束：`Slide.Export` 不自建父目录；缓存键 `<fileID>-<3位页>.png` 禁改；补缺判定唯一权威为 PNG 存在性；重建前必须先 `CancelAssetThumbs`。
- 基线数据：53 文件 / 1208 页 / 双档 2416 次 Export / 240–300s；单次 Export 约 70–90ms（估算，详见探针）。
- 相关规范：`docs/spec/assets-thumbs-com.md`、`docs/spec/assets-management.md`、`docs/spec/perf-baseline.md`。

## Implementation Plan（小 PR）

- PR1：队列/批次档位与页粒度 + `RequestAssetThumb` + hd 退出批量补缺 + wailsjs 同步 + 单测。
- PR2：重建 fileID 复用 + PNG 保留/清扫 + 失效联动 + 单测。
- PR3：前端 small 兜底 + 按需触发 + onDemand 事件过滤 + 真机冒烟 + 规范沉淀。
