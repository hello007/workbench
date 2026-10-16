# 素材管理第六批：批量打标签 + 多级标签树（SRS §2.4）

## Goal

消费第五批预留的批量操作按钮位：AssetSelectionBar 挂「批量打标签」，对选中条目批量追加/移除标签（文件模式落文件标签，幻灯片模式落 slide_tags 页自有标签）。同时把平铺标签体系升级为多级标签树（AssetTag.ParentID 已预留），三处消费点（详情编辑、标签筛选、批量弹窗）树形化，补标签管理入口（增删改）。

## What I already know

- SRS §2.4：多级标签树 + 自定义颜色；多标签绑定；左侧标签面板点击筛选；标签管理新增/重命名/删除（删除标签不删素材仅解除关联）；批量打标签（多选素材/幻灯片，添加/移除）
- SRS §2.3：选中模式由 viewMode 决定（文件模式选整个 PPT / 幻灯片模式选单页），批量操作对象跟随视图模式防误操作
- `AssetTag.ParentID` 字段已预留（model/assets.go:25，恒 0），tags 表 schema 需确认是否已有 parent_id 列
- `tagOptions` 从 fileEntries 底册聚合（AssetsPanel.vue:968），注释预留「接口补齐后可替换为独立数据源」——无独立标签列表 API 是既有明账
- `UpdateAssetFileMeta`（文件整体替换）/`UpdateAssetSlideMeta`（页标注整体替换）均单条目语义，无批量 API；标签自动创建分配默认色
- `AssetSelectionBar` 受控组件（count + select-all/invert/clear 三事件），批量按钮位预留（第六批扩展）
- `assetSelectionKey` 契约：`file:<id>` / `slide:<fileId>:<slideNum>`，禁 slides.id；反查经宿主底册
- 标签筛选现状：`selectedTagIds` 多选 + `slideTagOnlyOwn` 继承开关；FTS 检索与纯标签浏览两路径同源
- 批量明细先例：assets_split.go `splitOutcome` + `AssetSplitBatchResult` 逐文件明细（total/split/skipped/failed）
- 清空点位矩阵（§8.2）：切库/检索语境/viewMode/扫描刷新等六处 + 溯源单独补清

## Assumptions (temporary)

- tags 表已有 parent_id 列（随 ParentID 字段预留）——待确认
- 单连接池下批量写入单事务逐条目处理可行（条目量级 ≤ 几百）

## Open Questions

1. ~~标签树数据源与入库形态~~ → 已决：方案 A，启用 tags.parent_id 现列
2. ~~父子级联命中语义~~ → 已决：级联（选父含全部后代）
3. ~~批量 API 形态~~ → 已决：两独立方法 + 每条目一事务
4. ~~批量操作语义~~ → 已决：addTags/removeTags 双数组
5. ~~批量完成后选中集合处理~~ → 已决：保留选中
6. ~~树形控件选型~~ → 已决：el-tree-select 封装单组件 + 标签管理弹窗

## Open Questions

（已全部收敛）

## Decision (ADR-lite)

### D1：标签树数据源 = 启用 tags.parent_id 现列（方案 A）

**Context**：AssetTag.ParentID 已预留恒 0，需多级树消费；候选为启用现列 vs 独立标签字典表。
**Decision**：单一 tags 表，parent_id 启用（0=根），父标签同为 tags 表一行。新增 `ListAssetTags(libraryID)` 独立标签列表 API（含未使用标签），前端 tagOptions 聚合数据源替换为该 API。标签自动创建（遇新名）落为根标签。
**Consequences**：schema 零迁移/AssetTag JSON 形态不变（前端绑定零改动）；树操作（删父级联、防环）逻辑集中 service 层。放弃字典/绑定分离的职责纯洁性——数据量级（个人 DAM）不值得两表两份 ID 体系。

### D2：父子级联命中 = 选父 ∪ 全部后代（方案 A）

**Context**：标签筛选选父标签时，子级标签绑定的素材是否命中。候选为级联 vs 仅直接绑定。
**Decision**：筛选/检索命中口径 = 所选标签 ID ∪ 其全部后代 ID（FTS 检索与纯标签浏览两路径同源展开）。附带决议：**批量打标签勾父标签只绑定被勾标签行本身，不做级联绑定**（绑定是精确动作，筛选才是聚合语义）。
**Consequences**：命中数聚合按子树算，检索 SQL 需子树展开（后端一次闭包展开）；平铺老数据（全 parent_id=0）行为与现状完全一致；前端树面板可按子树聚合命中数。

### D3：批量 API = 两独立方法 + 每条目一事务（方案 A）

**Context**：现有单条目 UpdateAssetFileMeta/UpdateAssetSlideMeta 均整体替换语义；批量需覆盖文件标签与页自有标签两形态。
**Decision**：
- `BatchUpdateAssetFileTags(libraryID string, fileIDs []int64, op model.AssetBatchTagOp) (model.AssetBatchTagResult, error)` —— op 内含 addTags/removeTags 标签名列表（见 D4）
- `BatchUpdateAssetSlideTags(libraryID string, targets []model.AssetSlideTarget, op model.AssetBatchTagOp) (model.AssetBatchTagResult, error)` —— `AssetSlideTarget{FileID int64, SlideNum int}`（键契约 (file_id, slide_num) 禁 slides.id）
- 事务边界：**每条目一个事务**，逐条目 commit；失败记明细继续下一条，部分失败不中断；返回 `AssetBatchTagResult{Total, Succeeded, Failed int, Items []AssetBatchTagItem}`，Item{Target, Status, Message} 对齐 splitOutcome 明细先例
- 批量方法为独立新增，不破坏既有单条目整体替换语义；标签自动创建沿用默认色板
**Consequences**：App 绑定 +2 方法（wails generate 三处同步 + 具名 string 类型别名坑检查）；两套存储分叉在两个方法签名内显式化，无 kind 隐形分支器；每条目事务失败面小、明细准确；批量内 FTS 同步随每条目事务落定（slide_tags 变更影响检索命中口径）。

### D4：批量操作语义 = addTags/removeTags 双数组（方案 A）

**Context**：SRS §2.4「批量添加/移除标签」；候选为双数组同提交 vs 模式参数（add/replace）。
**Decision**：`AssetBatchTagOp{AddTags []string, RemoveTags []string}`（标签名，不存在的名自动创建沿用默认色板）。单次批量可同时加/移；add 与 remove 交集非空为参数错误（E_ASSETS_BATCH_TAG_CONFLICT 或入错误码表）。语义 = 每条目「现标签集 − remove ∪ add」，相对条目现状追加/移除，非整体替换；只动标签不动星级/备注。
**Consequences**：一次交互完成混合调整；replace 语义不提供（批量下危险且 SRS 无此需求）；后端校验交集 + 前端弹窗亦不提供同名双选（两处防线）。

### D5：批量完成后选中集合 = 保留（不清空）

**Context**：批量打标签提交成功后 selectedKeys 处理；需记入清空点位矩阵（§8.2）。
**Decision**：批量打标签完成（无论全部成功/部分失败）**不清选中**。清空点位矩阵新增一行：批量打标签完成 = 不清（数据变更点位，非语境变更——与切库/切 viewMode/检索语境等强制清空点位性质不同）。用户手动「清空」按钮照旧。
**Consequences**：支持「打完标签继续对这批操作」（后续批量评分等第七批面）与失败后手动重试；选中条目标签已变，宿主底册刷新时选中键（file:<id>/slide:<fileId>:<slideNum>）稳定不受重解析影响（files.id 稳定、slide_num 稳定）——保留无脏键风险。

### D6：树形控件 = el-tree-select 封装单组件 + 标签管理弹窗（方案 A）

**Context**：详情编辑/标签筛选/批量弹窗三处树形消费；候选 EP el-tree-select 封装 vs popover 自绘 vs 部分树形。
**Decision**：封装 `AssetTagTreeSelect` 单组件（基于 EP el-tree-select，checkbox 多选 + filterable），三处复用；新建标签统一收进标签管理弹窗（筛选区「管理标签」入口：树形展示 + 新增根/子标签、重命名、删除、改色）；详情编辑去掉 allow-create，加「+ 新建标签」快捷按钮预填管理弹窗。
**Consequences**：EP 原生组件守 frontend-visual-conventions；单组件三处复用无口径漂移；详情编辑失去 inline 输入即建（多一步弹窗，接受）；「左侧标签面板」仍 out of scope，筛选处树形下拉已覆盖 SRS §2.4「点击标签筛选」主路径。

### D7：扩展扫描收口（用户确认「按推荐来」）

（以下 D8 为冒烟反馈增量，2026-10-16）

- **删父标签 = 级联删除全部后代**（各自解除与素材关联，SRS「删除标签不删素材」逐级适用），删除前确认弹窗列子树标签数 + 受影响条目数
- **防环**：新增/改父时后端校验 parent 非自身/非后代
- **批量遇新标签名**：自动创建为根标签（沿用 D1）；批量内同名词单点去重（事务内 ensure 一次）
- **空操作防护**：add/remove 全空禁用提交（前端）+ 后端参数校验
- **ListAssetTags 返回带直接绑定计数**（树节点显数量；父节点子树合计前端由树结构自算）
- **批量后刷新链**：重拉底册 + 检索态重搜（对齐 onSaveMeta 先例）；选中保留（D5）
- 本批不预留批量评分字段/接口（第七批另建方法）

### D8：真机冒烟反馈增量（2026-10-16，用户四条 + 拖拽语义确认方案 A）

1. 标签管理弹窗视图样式优化 + 拖拽调整标签顺序：el-tree 原生 draggable，**同级排序 + 跨父拖拽二合一**（拖到节点上下=同级排序、拖到节点上=变其子级）；tags 表加 `sort_order` 列（user_version 6 迁移），后端 `MoveAssetTag(libraryID, tagID, newParentID, index)` 单 API 事务内写 parent + 同父兄弟重排；防环沿用 updateTagFull；ListAssetTags 返回按 sort_order 排序
2. 「平铺」检索结果开关默认改为打开（flatten 默认 true）
3. 搜索输入框与标签筛选下拉高度统一（样式对齐）
4. 非最大化窗口宽度下 viewMode radio 与 displayMode radio 换行成上下堆叠——flex 布局收缩修复（不换行或合理收缩）

## Requirements

- AssetSelectionBar 新增「批量打标签」入口（选中数 > 0 时可用），操作对象跟随 viewMode
- 批量 API：BatchUpdateAssetFileTags / BatchUpdateAssetSlideTags（D3 签名），addTags/removeTags 双数组（D4），每条目一事务逐条目明细，部分失败不中断
- 批量弹窗：AssetTagTreeSelect 选添加/移除标签集，提交后明细结果反馈（全成功 toast + 有失败弹明细）
- 多级标签树：tags.parent_id 启用（D1），ListAssetTags 独立标签列表 API（带直接绑定计数）
- 级联命中：筛选/检索选父 = 父 ∪ 全部后代（D2，FTS 与纯标签浏览两路径同源）；批量绑定不级联
- 标签管理弹窗：树形展示 + 新增根/子标签、重命名、删除（级联删子 + 确认弹窗列影响面）、改色；防环校验（D7）
- AssetTagTreeSelect 单组件三处复用：详情编辑（去 allow-create + 「+ 新建标签」按钮）、标签筛选下拉、批量弹窗（D6）
- 批量完成保留选中（D5），重拉底册 + 检索态重搜
- 新 App 绑定 wails generate 三处同步（App.js/App.d.ts/models.ts）；mock 改 wails-mock-defaults.js 单一数据源

## Acceptance Criteria

- [ ] 文件模式批量打标签后 ListAssetFiles 底册标签正确（追加/移除混合）
- [ ] 幻灯片模式批量打页标签后 slide_tags 独立存储正确 + FTS 检索命中口径同步
- [ ] 选父标签筛选命中含全部后代绑定条目；平铺老数据（parent_id=0）行为与现状一致
- [ ] 批量部分失败：明细逐条目状态，成功条目已写入不回滚
- [ ] add 与 remove 交集非空被后端拒绝
- [ ] 删父标签级联删子树并全部解除关联；确认弹窗列影响面；防环校验生效
- [ ] 批量完成后选中保留
- [ ] AssetTagTreeSelect 三处消费形态一致（单组件）
- [ ] 后端 service 覆盖率 ≥76%；前端 ≥70%

## Definition of Done

- 单测/E2E 覆盖；lint/CI 绿
- spec 沉淀（assets-management.md）
- 真机冒烟：批量打标签双模式 + 标签树筛选级联

## Out of Scope

- 第七批：§2.5 facet 多维筛选（星级/时间/大小）+ 保存检索条件 + 字段加权核对
- 左侧标签面板（SRS §2.4「左侧标签面板」——本批只做三处消费点树形化 + 管理入口，独立面板布局待评估；prd 迭代中定夺是否并入）

## Technical Approach

后端（service/assets_db.go + assets_service + app_assets.go）：
- model 新增：AssetBatchTagOp{AddTags, RemoveTags []string}、AssetSlideTarget{FileID, SlideNum}、AssetBatchTagResult{Total, Succeeded, Failed, Items}、AssetBatchTagItem{Target, Status, Message}、AssetTagListItem（AssetTag + Count）
- App 新增 6 方法：ListAssetTags / CreateAssetTag / RenameAssetTag / UpdateAssetTagColor（或合并 UpdateAssetTag）/ DeleteAssetTag / BatchUpdateAssetFileTags / BatchUpdateAssetSlideTags；wails generate 三处同步，具名 string 类型不引入（普通类型零别名坑）
- service：BatchUpdateFileTags/BatchUpdateSlideTags（每条目一事务 + 明细）；ListTags（含计数）；CreateTag/RenameTag/DeleteTag（级联删子 + 防环）；检索两路径子树展开（tagIDs → tagIDs ∪ descendants）
- 事务契约：批量内 slide_tags 变更 FTS 同步随每条目事务落定；rows Close 契约（单连接池）

前端（frontend/src/components/ + AssetsPanel.vue）：
- AssetTagTreeSelect.vue 单组件（el-tree-select 封装，树数据由 ListAssetTags 构建）
- AssetTagManagerDialog.vue 标签管理弹窗（树形 + CRUD + 删除确认影响面）
- AssetSelectionBar 加「批量打标签」按钮 + AssetBatchTagDialog.vue（TreeSelect 添加区/移除区 + 明细结果）
- AssetsPanel：tagOptions 数据源切 ListAssetTags；筛选下拉树形化；详情编辑接入；批量入口按 viewMode 分派两 API
- mock：wails-mock-defaults.js 补 6 方法返回值

## Implementation Plan (small PRs)

- PR1：后端批量 API + 标签 CRUD + 子树展开检索（model + service + app + 测试 + wails generate）
- PR2：前端 AssetTagTreeSelect + 标签管理弹窗 + 筛选/详情接入
- PR3：批量打标签弹窗 + SelectionBar 接入 + 明细反馈 + mock + E2E + spec 沉淀

## Technical Notes

- 摸底文件：model/assets.go、app_assets.go、AssetsPanel.vue（tagOptions/selectedTagIds/onSaveMeta）、AssetSelectionBar.vue、utils/assetSelection.js、service/assets_db.go（标签筛选 SQL）
- 硬约束：assetSelectionKey 反查禁第二处键拼装；(file_id, slide_num) 禁 slides.id；批量写入事务；单连接池 rows Close；FTS 中文切分同源 splitFtsText
