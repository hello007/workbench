# 素材管理第七批：facet 多维筛选 + 保存检索条件（SRS §2.5）

## Goal

补齐 SRS §2.5 检索面的两个缺口：多维度筛选（facet：星级/修改时间/文件大小，与关键词+标签可组合，检索与浏览双语境生效）与保存检索条件（命名/调取/删除/覆盖，data/ JSON 全局存储）。附带字段加权方向实测核对（不改切分链路）。

## Requirements

- **facet 三维筛选**：星级（0-5 含未评分）、修改时间（相对档 + 自定义区间）、文件大小（分档），与关键词/标签筛选可组合
- **facet UI**：工具行「筛选」按钮 + 弹层（el-popover），badge 常显生效 facet 数
- **双语境生效**：检索语境并入 SearchAssets；浏览语境 facet 激活时走 facets-only 路径（复用检索分页链路），清除后回底册
- **空结果引导**：facet 激活且零命中 → 空态文案 + 「清除全部筛选」一键复位
- **保存检索条件**：保存当前组合（关键词+标签+facet+flatten+slideTagOnlyOwn）、命名、快速调取、删除/覆盖；调取异库时标签按名重映射（miss 丢弃）
- **字段加权核对**：bm25 现加权（file_name 4.0 > slide_title 3.0 > slide_text 2.0 > 备注 1.0）方向实测，偏差才校准（须回归「标题命中 vs 正文命中」分层断言先例）
- 新增 App 绑定 wails generate 三处同步；mock 改 wails-mock-defaults.js 单一数据源；E2E 从 fixtures.js import、禁 waitForTimeout

## Decision (ADR-lite)

| # | 决策 | 结论 |
|---|---|---|
| D1 | 星级 facet 口径 | 统一文件级——只读 `files.rating`，幻灯片条目继承归属文件评分；页级评分（slide_meta.rating）纯展示不参与筛选；修改时间/文件大小同为文件维度，三 facet 口径一致 |
| D2 | facet UI 形态 | 工具行「筛选」按钮 + 弹层，内嵌三维控件，badge 显生效数；工具行不膨胀，预留后续 facet（文件类型已有/主色 P1）扩展落点 |
| D3 | facet 作用语境 | 双语境生效——检索语境并入 SearchAssets 链路；浏览语境 facet 激活时放开现「空关键词+空标签返回空页」语义，走 facets-only 路径（复用检索分页链路，底册退位），清除后回底册；清空点位矩阵按两语境全部登记 |
| D4 | 保存条件存储 | data/asset_saved_searches.json（util.SaveJSON 原子写，last-writer-wins 沿用 settings 约束）；全局存储带 libraryId；调取异库 tagIds 降级——保存时按标签名冗余存一份，调取按名重查当前库 ID，miss 丢弃该标签；关键词/facet/flatten/slideTagOnlyOwn 异库照回填 |
| D5 | 排序器 | 砍到第八批——本批排序维持相关性单序，保存条件不含排序；第八批带真实使用诉求再做 |
| D6 | 空结果引导 | 空态文案 + 一键清除（facet+标签复位），两语境复用同一空态；分维度命中计数不做 |
| D7 | 保存内容面 | flatten 与 slideTagOnlyOwn 均入快照，调取完整还原检索语境；摘要可在保存时预览 |

**Context**：SRS §2.5 facet/保存条件无实现细节；本批按 MVP 收敛、口径单点、红线不破三原则定夺。
**Consequences**：facets-only 语义重定义、跨库标签降级、facets-only 路径与底册切换联动为本批新契约，随批沉淀 docs/spec/assets-management.md。

## Acceptance Criteria

- [ ] facet 三维与关键词/标签组合检索正确（keyword/tagsOnly/facets-only 三路径后端单测）
- [ ] 浏览语境 facet 激活走 facets-only 路径、清除回底册（前端组件测试）
- [ ] 保存条件保存/调取/删除/覆盖闭环；异库调取标签按名重映射 miss 丢弃（单测）
- [ ] 空结果引导文案 + 清除全部筛选复位（组件测试）
- [ ] 加权方向实测结论落 research（偏差才校准并回归分层断言）
- [ ] 清空点位矩阵登记 facet 两语境全部变更点位；双代次守卫（librarySeq/searchSeq）不破
- [ ] 原子写：saved_searches 走 util.SaveJSON；禁 os.WriteFile 直写
- [ ] service ≥76% / 前端 ≥70% 门禁绿
- [ ] 真机冒烟：facet 组合检索 + 保存条件调取

## Definition of Done

- 单测/组件测试覆盖新增面；lint/typecheck/CI 绿
- docs/spec/assets-management.md 沉淀 facets-only 语义/跨库降级/弹层契约；CLAUDE.md 关键规则按需追加
- README/功能说明按需更新

## Out of Scope

- OCR 文本检索（P1）；图片主色检索（P1）；以图搜图（P2）
- 标签入 FTS（既定红线）
- 批量评分（§2.6 P0 另批）
- 排序器可配置（D5，第八批）
- 文件类型 facet（已做）；分维度命中计数（D6）

## Technical Approach

### 后端（PR1）
- `model.AssetSearchOptions` 扩 facet 字段（星级档/时间区间/大小档，命名与 JSON tag 定型时对齐前端快照结构）
- `searchAssetsInStore` 空查询语义重定义：无关键词且无标签**但有 facet** → 新 `searchByFacetsOnly` 路径（复用 pageAndFillEntries 分页）；全空仍返空页
- keyword/tagsOnly 两路径行级 facet 判定（文件维度，单点函数，与 slideTagRowMatches 同层）
- facet 查询基于 files.size/mtime_ns/rating；时间相对档由前端转绝对区间传入（后端只认区间，档位语义单一数据源在前端）
- 保存检索条件 service：data/asset_saved_searches.json 读写（util.SaveJSON），CRUD 4 绑定方法（List/Save/Delete/覆盖=Save 同名更新）

### 前端（PR2 facet UI + PR3 保存条件）
- AssetFilterPopover 组件：三维控件（星级 el-rate/select 含未评分、时间相对档+区间、大小分档）+ 重置；工具行按钮 badge
- AssetsPanel 双语境接入：检索语境 facet 并入 SearchAssets 参数；浏览语境 facet 激活切 facets-only 分页链路
- 空态引导：facet 激活零命中文案 + 清除全部筛选
- 保存条件：工具行「保存搜索」入口 + 命名弹窗 + 条件列表调取/删除/覆盖；快照含 flatten/slideTagOnlyOwn
- 清空点位矩阵登记（§8.2）：facet 变更两语境点位 + 保存条件调取点位

### 字段加权核对（PR1 内）
- 实测「文件名命中 > 幻灯片标题命中 > 正文命中 > 备注命中」分层断言；方向符合 SRS 即记录结论，偏差才改 bm25 权重并回归既有排序断言

## Implementation Plan (small PRs)

- PR1（后端）：AssetSearchOptions 扩字段 + searchByFacetsOnly + 三路径 facet 判定 + 空查询语义重定义 + 保存条件 service（JSON CRUD 4 绑定）+ facet/加权单测 + wails generate 同步
- PR2（前端 facet）：AssetFilterPopover + 工具行按钮 + 双语境接入 + 空态引导 + 矩阵登记 + 组件测试 + mock
- PR3（前端保存条件）：保存/调取/删除/覆盖 UI + 快照结构 + 跨库降级 + 组件测试 + mock
- 收尾：spec 沉淀 + 真机冒烟（facet 组合检索 + 保存条件调取）

## Technical Notes

- service/assets_search.go（两路径 + pageAndFillEntries）；service/assets_schema.go（files.size/mtime_ns/rating 列已有，无迁移）
- frontend/src/components/AssetsPanel.vue（检索工具行宿主）、AssetListView.vue（列表视图）
- 跨库标签重映射需后端按名查标签方法（或复用 ensureTagsByNames 读向等价查询，实现时核对）
- 保存条件整体覆盖 last-writer-wins：手改 data 文件前关 workbench.exe（既有约束沿用）
- 时间相对档（今天/近7天/近30天/自定义）语义单一数据源在前端；文件大小分档默认 <1MB/1-10MB/10-50MB/>50MB（弹层内可调边界则档位即输入，实现时定型）
- bm25 权重变更须回归「标题命中 vs 正文命中」确定性分层断言先例
- 双代次守卫（librarySeq/searchSeq）+ 清空点位矩阵（docs/spec/assets-management.md §8.2）
