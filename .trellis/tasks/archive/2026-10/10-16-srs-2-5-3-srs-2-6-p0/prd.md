# 素材管理第八批：检索结果排序器（SRS §2.5.3）+ 批量修改评分（SRS §2.6 P0）

## Goal

补齐检索结果排序可配置（SRS §2.5.3「相关性、修改时间、评分等可配置」），并落地批量修改评分 P0 操作面（SRS §2.6）。两面独立，拆三 PR 交付。

## Requirements

### 排序器

* 排序档位：相关性（默认）/ 修改时间 / 评分；修改时间与评分支持升降切换（默认降序），相关性无向
* UI 挂点：检索工具行独立下拉（不入 AssetFilterPopover）
* 排序键后端实现，禁前端内存排序（分页懒加载下前端排序必错段）
* 三路径全覆盖：关键词（`searchByKeyword`）、纯标签（`searchByTagsOnly`）、facets-only（`searchByFacetsOnly`）
* 评分排序统一文件级口径：排序键恒 `files.rating`，幻灯片条目/聚合条目继承归属文件 rating（与星级 facet 口径同源，第七批 D1）；页级 `slide_meta.rating` 纯展示与详情编辑，不参与排序
* mtime 排序同理文件维度继承（幻灯片条目按归属文件 mtime_ns）
* 相关性排序现状不动（FTS bm25 序；聚合组首=组内最高分页，SRS「聚合按最高单页相关性」现状已满足）
* mtime/rating 排序在分页前对全量 entries 排序：关键词路径批量取文件属性（复用 fileMetaByIDs 字段面：mtime_ns/rating 一次 IN 查询）后 Go 层稳定排序；标签/facet 路径 SQL 查询层扩展（SELECT 加列 + ORDER BY 或 Go 层同构排序）
* 保存检索条件快照补 `sortBy`/`sortDir` 字段，调取时还原；JSON 容错反序列化零值默认 = 相关性（旧快照向后兼容，无版本号）
* 排序变更 = 检索条件变更点位：走 `applySearch` 同路径（searchSeq 代次守卫 + 清详情/预览/选中/聚合展开缓存），矩阵 §8.2 登记

### 批量修改评分

* 入口：AssetSelectionBar 加「评分」按钮，弹 `AssetBatchRatingDialog`
* 弹窗形态复刻 AssetBatchTagDialog：收 targets+execute props、不感知 viewMode、失败明细反查 `slideNum||0`
* 星级单选 0-5，0 = 清除评分
* 作用对象：文件模式选中 → `files.rating`；幻灯片模式选中 → `slide_meta.rating`；聚合父卡 = 评其代表页（与批量打标签同语义）
* 后端每条目一事务逐条目明细（splitOutcome 先例，部分失败不中断）
* 执行成功后不清选中（数据变更点位，对齐批量打标签矩阵定性）；星级 facet 命中联动属正常数据变更，不清选中
* 新 App 绑定 wails generate 三处同步（App.js / App.d.ts / models.ts）

## Acceptance Criteria

* [ ] 检索结果可切换排序档位（相关性/修改时间/评分 ± 升降），分页跨页顺序正确
* [ ] 聚合条目 mtime/rating 排序继承文件维度，测试覆盖
* [ ] 排序确定性测试：注入固定 mtime_ns/rating 构造分层（禁 sleep/mtime 时序）
* [ ] 保存检索条件含排序状态；旧快照（无排序字段）调取不报错且默认相关性
* [ ] 批量评分：文件条目/幻灯片条目/聚合父卡作用对象正确；部分失败有明细且不中断
* [ ] 批量评分后 facet 星级命中联动、选中保留
* [ ] service ≥76% / 前端 ≥70% 覆盖率门禁通过
* [ ] E2E 从 fixtures.js import、禁 waitForTimeout；mock 改 wails-mock-defaults.js

## Definition of Done

* 测试补齐（后端三路径排序单测 + 批量评分逐条目明细单测 + 前端组件用例）
* lint / 覆盖率门禁绿
* spec 沉淀（排序契约 + 批量评分先例扩展）+ README 确认
* 真机冒烟：排序档位切换 + 批量评分后 facet/排序联动

## Decision (ADR-lite)

**D1 排序 UI 挂工具行独立下拉**：排序高频切换与 facet 低频筛选不同频；Billfish/ES 类工具排序在工具栏直出。同档再点切升降（mtime/评分），相关性无向。

**D2 评分排序统一文件级口径**：与星级 facet 口径（第七批 D1 幻灯片继承归属文件）同源单一，排序/筛选联动直觉一致；页级评分纯展示。实现最简（entries 批量取文件属性一次 IN 查询）。

**D3 快照补 sortBy/sortDir 零值默认**：检索语境完整还原应有之义；JSON 容错反序列化旧快照天然兼容，无版本号。

**D4 聚合父卡批量评分 = 代表页**：与批量打标签先例完全一致（失败明细反查 `slideNum||0` 原样复用）；文件级评分走文件视图或详情单条入口。先例一致性优先于「评整本」直觉。

**D5 评分入口/交互**：AssetSelectionBar「评分」按钮 + AssetBatchRatingDialog（targets+execute props、不感知 viewMode）；0=清除；成功不清选中。

**D6 三 PR**：PR1 后端排序 + PR2 前端排序档位 + PR3 批量评分。同面拆前后端 review 面小，评分独立域单独成 PR。

## Out of Scope

* 改切分链路与 bm25 权重（第七批刚收敛）
* 图片主色检索、以图搜图（SRS P1/P2）
* 快捷键评分、批量导出幻灯片截图（P1）
* 排序键持久化为用户偏好（仅快照随检索条件）

## Technical Approach

### PR1 后端排序

* `model.AssetSearchOptions` 加 `SortBy string`（"" = relevance，"mtime"/"rating"）+ `SortDir string`（"desc" 默认 / "asc"）；`model.AssetSavedSearch` 加同名字段
* `service/assets_search.go` 三路径分流：
  * 关键词路径 relevance：现状 FTS 序不动
  * 关键词路径 mtime/rating：FTS 命中 fileID 全集批量查 files 属性（mtime_ns/rating）→ entries 填 sortMtimeNs/sortRating → Go 层稳定排序（次级键 score→path 保证确定性）→ 聚合（组首=排序后首现，文件维度单值聚合天然正确）
  * 标签/facet 路径：SQL SELECT 扩展 mtime_ns/rating 列，Go 层同构排序（与关键词路径共用排序函数单点）
* `pageAndFillEntries` 不动（补全层与排序层解耦）

### PR2 前端排序档位

* 工具行排序下拉（相关性/修改时间/评分 + 升降切换），变更走 `applySearch`（矩阵登记：清详情/预览/选中/aggChildren）
* `buildSavedSearchSnapshot` 补 sortBy/sortDir；`applySavedSearch`/`restoreFacetStateFromSnapshot` 还原
* mock：`wails-mock-defaults.js` 兼容新参数（排序不改变 mock 返回集合时可为透传）

### PR3 批量评分

* 后端：新绑定 `BatchSetAssetRating(targets []model.AssetRatingTarget, rating int64)`（target = {fileId, slideNum}，slideNum 0 = 文件），service 每条目一事务：文件 UPDATE files.rating / 页 upsert slide_meta.rating；返回逐条目明细（ok/error/label）
* 前端：AssetSelectionBar「评分」按钮 + AssetBatchRatingDialog（复刻 AssetBatchTagDialog 形态）；星级 0-5 单选；成功不清选中
* wailsjs 三处同步 + mock 补 `BatchSetAssetRating`

## Technical Notes

* 关键文件：`service/assets_search.go`（三路径 + 聚合）、`service/assets_db.go`（fileMetaByIDs/fileMeta 已含 mtime_ns/rating）、`model/assets.go`（AssetSearchOptions/AssetSavedSearch）、`frontend/src/components/AssetsPanel.vue`（searchCards/buildSavedSearchSnapshot/applySavedSearch/批量目标反查）、AssetSelectionBar、AssetBatchTagDialog（先例）
* 无 schema 迁移（rating 列已有）；slide_meta rating upsert 链路已有（onSaveSlideMeta 后端侧）
* 排序次级键须确定性（同值条目跨分页稳定）：score → path → fileID
* 约束继承：FTS 五契约不破；每条目一事务；测试禁 sleep/固定端口；frontend-visual-conventions 工具行/操作条既有约束
