# 素材管理第四批：幻灯片独立标注体系（SRS §2.2.4）

## Goal

幻灯片单页获得三层独立标注——独立标签、独立业务备注、独立星级评分，落地 SRS 标签继承三契约（存储完全独立 / 查询层虚拟继承合并 / 「仅使用幻灯片自身标签」筛选开关），幻灯片详情面板升级为可编辑（对齐文件详情编辑形态）。

## Requirements（定案）

1. **存储形态（Q1=推荐 A）**：独立表 `slide_tags` + `slide_meta`，键 `(file_id, slide_num)`，外键 `files(id) ON DELETE CASCADE`。不用 slides.id 作键（重解析删旧插新全变）。
2. **增量语义（Q2）**：页数不变标注原样保留；页数减少清理失配标注；页内容移位标注跟随页码（位置语义，与缩略图缓存键 file_id+page 同口径）。
3. **重建保留（Q5）**：按 `files.path + slide_num` 备份重绑（对齐 `fileTagBindingsByPath` 先例）；页消失对应标注静默丢弃；slide_tags 与 slide_meta 全部纳入备份。
4. **标签继承三契约**：
   - 存储层：文件标签与页标签互不写入；
   - 筛选层：默认合并【页自身标签 ∪ 归属文件标签】，`SlideTagOnlyOwn=true` 仅匹配页自身标签；
   - UI 层：详情面板「文件标签（继承）」只读弱化 +「幻灯片标签」可编辑。
5. **关键词检索行为不变（Q3）**：标签不入 FTS；幻灯片业务备注不入 FTS（不对称记为已知限制，未来批评估加列）。
6. **开关参数（Q4）**：`AssetSearchOptions.SlideTagOnlyOwn bool`，默认 false=继承合并，向后兼容；仅作用于标签筛选（纯标签浏览与关键词+标签混合检索两条路径都生效），关键词命中不受开关影响。
7. **详情编辑（Q6）**：星级 el-rate / 标签 el-select 多选（自有）/ 备注 textarea 命名「业务备注」（区别于解析自带「演讲者备注」）；保存走新绑定 `UpdateAssetSlideMeta(libraryID, fileID, slideNum, meta)`；评分/备注变更不触发重解析与缩略图失效。
8. **schema v4→v5**：`PRAGMA user_version` 递增，`assetsMigrations[5]` 建两表；无 Go 层回填（存量标注天然为空）；db 过新拒绝打开。
9. **条目数据面**：`AssetSlideEntry` 与检索幻灯片条目增加自有标注字段（自有标签/业务备注/星级），详情面板数据由条目携带（不新增查询绑定）；`Tags` 字段维持继承语义不变。

## Acceptance Criteria

- [ ] 存储层独立性：文件标签增删不写 slide_tags（单测断言）。
- [ ] 默认标签筛选命中【页自身 ∪ 文件标签】；OnlyOwn=true 仅页自身（单测两模式 × 纯标签浏览/混合检索两路径）。
- [ ] 重建后幻灯片标注（标签/备注/星级）按 path+slide_num 重绑保留；页消失标注丢弃（单测）。
- [ ] 增量重解析：页数不变保留、页数减少清理失配（单测）。
- [ ] v4 旧库升级打开正常，标注空默认；v5 建库直建（迁移测试）。
- [ ] 详情面板：继承标签只读弱化 + 业务备注/星级/自有标签编辑保存（vitest）。
- [ ] 保存标注不触发重解析与缩略图失效（断言无副作用）。
- [ ] GOOS=linux 编译 vet 绿；service ≥76%、前端 ≥70% 覆盖率门禁。

## Definition of Done

- `go test ./...` 与 `cd frontend && npm test` 绿；lint/typecheck/build 绿
- wailsjs 三处同步（App.js / App.d.ts / models.ts）
- docs/功能说明.md 14.4 更新 + spec 沉淀（assets-management.md 补标注契约）
- 真机冒烟：独立标注编辑 + 继承筛选开关 + 重建不丢标注

## Decision (ADR-lite)

**Context**：slides.id 每轮重解析全变，files.id 稳定；标注生命周期须独立于解析。
**Decision**：独立表键 `(file_id, slide_num)`（方案 A）；重建按 path+slide_num 重绑；业务备注不入 FTS；开关走 AssetSearchOptions 新布尔字段。
**Consequences**：增量重解析零迁移逻辑天然保留（收益）；读侧 JOIN 成本（可接受，条目批量装配）；页内容移位标注位置语义（与缩略图键口径一致，用户已确认）；幻灯片备注不可关键词检索（已知限制，PRD 记录）。

## Out of Scope

- 第五批及以后：列表视图/全屏预览/选中模式/批量打标签/多级标签树/facet 筛选
- 幻灯片业务备注入 FTS
- 多级标签树 UI（tags.parent_id 仍预留）

## Technical Approach

### 后端（PR1）

- `service/assets_schema.go`：`assetsSchemaVersion = 5` + `assetsSchemaV5SQL`（slide_tags + slide_meta）+ `assetsMigrations[5]`。
- `service/assets_db.go`：
  - `listSlideTagGroups`（slide_tags 全库分组）/ `listSlideMetaMap`（slide_meta 批量）——单连接池纪律：rows 显式 Close 后再发下一查询；
  - `listSlideEntries` / 检索幻灯片条目装配补自有标注字段；
  - `fileIDsForTagFilter` 家族扩展：slideIDsForTagFilter(own)、合并语义实现单点（默认 OR 两表，OnlyOwn 走单表）；
  - `updateSlideMeta(libraryID, fileID, slideNum, ...)`：整体替换语义对齐 updateFileMeta；文件不存在/页不存在报可读错误；
  - 重建链：`slideAnnotationsByPath`（两表 JOIN files 备份）+ `rebindSlideAnnotationsAfterRebuild`（path→新 file_id + slides 现存页号匹配，INSERT OR IGNORE）；
  - 重解析收口：slides 删旧插新后 `DELETE FROM slide_tags/slide_meta WHERE file_id=? AND slide_num NOT IN (SELECT slide_num FROM slides WHERE file_id=?)` 清失配。
- `service/assets_search.go`：标签筛选两条路径接 OnlyOwn。
- `service/assets_scan.go`：rebuildLibrary 接备份/重绑；processScanFile 重解析路径接失配清理。
- `model/assets.go`：`AssetSlideEntry`/检索条目加 `OwnTags/Note/Rating`；`AssetSearchOptions` 加 `SlideTagOnlyOwn`；`AssetSlideMetaUpdate` 新 struct。**普通字段名，无具名 string 类型**（models.ts 别名坑）。
- `app_assets.go`：`UpdateAssetSlideMeta` 委托 + wailsjs 三处同步。

### 前端（PR2）

- `AssetsPanel.vue` 幻灯片详情：业务备注/星级/自有标签编辑区（对齐文件详情形态与间距）+「文件标签（继承）」只读弱化（现有继承标记样式沿用，标题改名）+ 保存/重置；保存成功刷新当前条目。
- 检索视图标签筛选行加开关「仅使用幻灯片自身标签」（el-switch/checkbox，仅幻灯片视图语义相关，文件视图隐藏）。
- `wails-mock-defaults.js` 补 `UpdateAssetSlideMeta` 与新字段默认值（单一数据源）。
- vitest：编辑保存链路 + 开关联动 + 继承标签只读渲染；E2E 从 fixtures.js import（如适用）。

### PR 切分

- PR1：后端 schema v5 + 标注读写 + 继承筛选 + 重建/重解析保留链 + 绑定 + 单测。
- PR2：前端详情编辑 + 开关 + mock + vitest + 文档。

## Technical Notes

- 关键文件：`service/assets_schema.go`、`assets_db.go`、`assets_search.go`、`assets_scan.go`、`model/assets.go`、`app_assets.go`、`frontend/src/components/AssetsPanel.vue`、`frontend/src/test/wails-mock-defaults.js`
- 硬约束：SQLite 单连接池（rows Close 禁再查）；继承合并单点后端；GOOS=linux vet；禁具名 string 类型过 Wails；fixture 程序化构造；测试禁 sleep/mtime 时序
- 先例：`fileTagBindingsByPath`/`rebindFileTagsAfterRebuild`（assets_db.go:812/875）、`updateFileMeta` 整体替换、v2 迁移回填门禁形态
