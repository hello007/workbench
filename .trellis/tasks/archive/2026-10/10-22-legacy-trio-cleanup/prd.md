# 素材与预览遗留收口（子任务 1/3，执行顺序第 1）

## Goal

journal 遗留项收口（原三件，其一已被他案修复，实际两项）：素材聚合展开子页不全修复 + PPT 预览 STA 队列插队。

## Requirements

### R1 素材聚合展开子页不全修复（分批懒加载）

根因：`AssetsPanel.vue` `toggleAgg` 调 `SearchAssets(flatten:true, limit:200, offset:0)` 取**全局**命中前 200，前端再按 `filePath` 过滤——多文件命中时目标文件子页被挤出窗口且无提示。

方案（已确认，推荐项）：

1. `model.AssetSearchOptions` 加 `FilePath string` 字段（空=不过滤，零行为变化）；`SearchAssets` 汇聚层按该字段过滤命中条目（`refType=='slide' && filePath 匹配`），三条路径（keyword/facets-only/tagsOnly）行为一致。
2. `frontend/wailsjs/go/models.ts` `AssetSearchOptions` class 手动补 `filePath` 字段（App.js/App.d.ts 不动——方法签名未变）。
3. 前端 `toggleAgg` 带 `filePath` + 分页懒加载：首屏取前 N（沿用 200 上限语义，此时 200 即单文件子页窗口），子页超出时尾部「加载更多」按 offset 续拉；总命中数经 page.total 判定是否续拉。

契约守卫：

* aggChildren 写回双代次守卫（searchSeq/librarySeq 快照比对）+ contextChanged 比对面不变；
* 「加载更多」续拉同走 toggleAgg 的语境快照模式（发起时快照，写回前比对，失效丢弃）；
* aggChildren 缓存键仍为 filePath，续拉结果是**追加**非覆盖（`{...aggChildren.value, [filePath]: children.concat(more)}` 形态）；
* 检索态变更清空 aggChildren 的既有矩阵（§8.2）不变——追加态同被清。

### R2 PPT 预览 STA 队列优先级插队

现状：`assets_thumbs_windows.go` 单 FIFO 无缓冲 `taskCh`，缩略图批量导图期间用户点预览（ConvertToPdf 走同一 `submit`）排尾，首开延迟不可感知。

方案（已确认）：加 urgent 通道严格优先。

1. `goOleThumbExporter` 加 `urgentCh chan func(*comWorkerState)`（无缓冲，与 taskCh 同构）；`submit` 加 urgent 变体（如 `submitUrgent`），worker loop `select` 先非阻塞收 urgentCh（嵌套 select + default 降级再收 taskCh），保严格优先且单 worker 契约不破。
2. PPT 预览转换提交点（`pptx_preview_windows.go` → `ConvertToPdf` 经 `e.submit()`）改走 urgent。
3. 缩略图批量链路仍走普通 `taskCh`，零行为变化。

契约守卫（assets-thumbs-com.md 全部不变）：

* 禁双队列——仍是单 worker 单 STA goroutine，仅通道数 +1，进程快照 diff 互杀逻辑不动；
* Discard 置脏、jr.Result 恰一次写回、在途防抖 chan 缓冲 1、超时杀进程收口全部不动；
* worker 消费 urgent 与普通任务共用同一 `comWorkerState`，publishOwned/清理路径不变。

## Acceptance Criteria

- [ ] 多文件命中（全局命中 > 200）时展开任一文件，子页完整展示或经「加载更多」补全
- [ ] 聚合展开「加载更多」在检索条件变更/切库后在途响应被丢弃（双代次守卫对追加路径生效）
- [ ] 缩略图大批量导图期间点击预览，转换任务先于队列中存量缩略图任务执行（可感知插队）
- [ ] `go vet ./...` 零告警（维持现状，原第 3 项已被 0a2040d 修复）
- [ ] go test + vitest + E2E 全绿，coverage 门禁不回退

## Definition of Done

* 后端：`AssetSearchOptions.FilePath` 过滤单测（三路径一致性 + 空值零行为）+ urgent 通道优先级单测（urgent 先于 FIFO 存量消费）
* 前端：toggleAgg 分页追加 vitest（语境变更丢弃、追加不覆盖、total 判定续拉）
* spec 同步：assets-management.md（FilePath 过滤）、assets-thumbs-com.md（urgent 通道）

## Decision (ADR-lite)

**Context**: 聚合子页不全需选补全方案；vet 告警项经查已被修复。
**Decision**: R1 取后端 FilePath 过滤 + 前端分页懒加载（用户 2026-10-22 确认「按推荐来」）；R2 取 urgent 通道严格优先；原第 3 项（clipboard vet 7 处）移除——`d7d5b51` + `0a2040d` 已修，当前全仓 vet 零告警。
**Consequences**: R1 动 AssetSearchOptions 跨层结构（须手动同步 models.ts）；R2 队列通道 +1 但单 worker 契约保持，后续新增高优任务可复用 submitUrgent。

## Out of Scope

* 单页 OCR（父任务决策暂缓）
* 聚合展开的其他交互改造（如虚拟滚动）
* 缩略图任务自身的优先级细分

## Technical Notes

* 相关 spec：docs/spec/assets-management.md（§8 聚合/选中、§8.2 矩阵、§8.7 刷新入口）、assets-thumbs-com.md（STA 队列全契约）、pptx-preview.md、cross-layer-contracts.md（models.ts 手动同步）
* 遗留出处：journal-3.md 293 行段（vet 已修）、素材二批 013032e 段（聚合截断）
* 关键代码：frontend/src/components/AssetsPanel.vue:3544 toggleAgg、service/assets_search.go:50 SearchAssets、service/assets_thumbs_windows.go:198 submit、model/ AssetSearchOptions struct
