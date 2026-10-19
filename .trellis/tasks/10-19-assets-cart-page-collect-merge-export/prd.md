# 素材管理-购物栏：页级收集与合并导出

## Goal

素材管理面板新增「购物栏」：用户浏览素材库时把任意 PPT 的任意页加入购物栏，购物栏内拖拽调整顺序，点击「导出」把购物栏内全部页按顺序合并为一个新 PPT 文件（导出时选择保存位置并命名）。

## What I already know

* 用户需求原文：某 PPT 某页加入购物栏；购物栏内拖拽排序；导出合并为新 PPT，导出时需选位置与命名。
* 素材管理已有：slide 条目选中键 `'slide:<fileId>:<slideNum>'`（禁 slides.id，重解析全变）、缩略图 COM 队列、标注体系 (file_id, slide_num) 键、FTS 检索、页级评分/标签。
* PPT 拆分已有纯 Go OOXML 实现（禁 COM 依赖）：最小编辑集、页序三跳映射（sldIdLst→rId→rels→part 名）、GC 保留页真实链路（页→版式→母版→主题可达性闭包剪除，含 sldMasterIdLst 剪枝 + presentation.xml.rels 悬挂清理 + 保留母版 rels 被剪版式关系删除 + 母版本体 sldLayoutIdLst 条目删除四处联动）。
* data/ 持久化必须走 `util.SaveJSON` 原子写。
* 素材库多库并存，slide 条目跨库 fileId 不唯一——购物栏条目须带 libraryId。
* 工具箱已全窗口面板化，新工具禁 el-dialog；但购物栏属于素材面板内功能，形态待定（抽屉/侧栏/覆盖层）。

## Decisions

* 购物栏**跨库收集**（用户确认 2026-10-19）：条目键 (libraryId, fileId, slideNum)，来源行显示「库名/文件名/页码」。
* 购物栏**持久化**（按既有先例自定）：`data/asset_cart.json`，`util.SaveJSON` 原子写，含顺序，重启不丢。

* 购物栏 **UI = 素材面板内嵌抽屉覆盖层**（用户确认 2026-10-19）：工具行购物车入口（数量徽标）+ 抽屉列表（缩略图+库名/文件名/页码）+ 拖拽排序 + 底部导出按钮；加号入口三处：页卡片 hover 按钮区、详情侧栏、预览层（均有既有按钮先例）。

* 导出位置选择（按先例自定）：`SaveFileDialog('默认名.pptx', [{Pattern:'*.pptx'}])` 一步完成选位置+命名；serve 模式走既有 `wailsRuntimeUnavailable` 守卫。默认文件名自动预填（`合并导出-yyyyMMdd-HHmm.pptx`）。
* 导出成功后**保留购物栏**（用户确认 2026-10-19）：不清空，成功 toast 提供可选「清空购物栏」动作。
* 购物栏内**同页去重**（按条目键），重复加号操作幂等（已收集中提示已在购物栏）。
* 条目鲁棒性：购物栏条目存 (libraryId, fileId, slideNum) + 来源文件路径快照；fileId 跨重解析稳定（files.id），源文件 lost/改名时按路径重探，探不到标记失效条目（置灰+可手动移除），导出跳过失效条目并在结果中报告。
* 导出为**可取消长操作**：独立 cancels 注册表（复用素材域取消模式），导出中禁二次导出，禁修改购物栏（或修改时提示）。

* **MVP 含跳转溯源**（用户确认 2026-10-19）：购物栏条目可点击定位回素材库该页（切库+定位语境，复用 locateNode 类先例；目标库非当前库时先切库）。
* **MVP 含批量加页**（用户确认 2026-10-19）：页级多选模式（复用/扩展现有选中体系），筛选与检索结果内勾选多页一次入栏。选中键体系沿用，注意批量操作清聚合展开缓存的既有矩阵约束、不清选中。
* 合并技术路线 = **路线 A：基底注入式合并**（用户确认 2026-10-19；研究文件 research/ooxml-merge-approaches.md）。纯 Go、CI 可测、复用拆分基建，MVP 边界见 Out of Scope。COM 双通道列为后续增强。

## Assumptions (temporary)

* 购物栏跨库收集（同一购物栏可装不同素材库的页）——待确认。
* 导出走纯 Go OOXML 合并，不走 COM（与拆分同策略，CI 可测）。
* 购物栏内容需持久化（重启不丢）——待确认。

## Open Questions

1. MVP 范围：购物栏是否跨素材库？是否持久化？
2. 购物栏 UI 形态与入口（页卡片上加号、预览层加号、购物栏入口位置）。
3. 合并导出技术方案：多源文件的母版/版式/主题如何处理（研究后提选项）。
4. 导出文件位置选择方式（目录输入框+浏览按钮 vs 系统对话框；serve 模式禁系统对话框）。
5. 失败/边界：源文件丢失（lost/重链）、同文件多页、导出重名、大文件性能。

## Requirements

* 页级加入购物栏：页卡片 hover 加号、详情侧栏、预览层三处入口；同页幂等去重。
* 批量加页：页级多选模式，筛选/检索结果勾选多页一次入栏。
* 购物栏抽屉：工具行入口（数量徽标），列表（缩略图+库名/文件名/页码），拖拽排序（vue-draggable-plus），条目移除。
* 跳转溯源：条目点击定位回素材库该页（必要时先切库）。
* 持久化：data/asset_cart.json，util.SaveJSON 原子写，含顺序，跨库条目 (libraryId, fileId, slideNum) + 路径快照。
* 导出：SaveFileDialog 选位置+命名（默认名预填），按购物栏顺序合并为单一 .pptx，可取消长操作，进度反馈。
* 失效条目：源文件探不到时置灰可移除，导出跳过并在结果中报告。
* 导出成功保留购物栏，toast 提供可选清空动作。

## Acceptance Criteria

* [ ] 三处入口可加入/移出购物栏，图标态反馈（已收集态可见），重复添加幂等。
* [ ] 批量加页一次入栏且顺序符合追加语义。
* [ ] 购物栏顺序可拖拽调整，顺序持久（重启保留）。
* [ ] 跨库条目混装正常，来源行显示库名/文件名/页码。
* [ ] 条目点击可溯源定位到素材库该页。
* [ ] 导出产物在 PowerPoint/WPS 正常打开无修复弹窗。
* [ ] 页序与购物栏顺序一致（三跳映射，slide 文件名 ≠ 页序）。
* [ ] 跨源文件页合并成功（母版冲突处理按选定方案）。
* [ ] 导出可取消，取消后产物不落盘（或明确降级语义）。
* [ ] 失效条目置灰、导出跳过且结果报告。

## Definition of Done (team quality bar)

* 后端/前端单测 + 关键链路集成测试（程序化构造 pptx fixture，禁依赖本机 Office）。
* Lint / 测试全绿，覆盖率门禁达标。
* README.md / docs 相关文档更新。
* spec 沉淀（新契约入 docs/spec/）。

## Technical Approach

**后端核心（路线 A：基底注入式合并）**

1. **GC 集合化泛化**（0.5-1 人日）：`deleteSetForPage` 单页保留 → 页集合保留（roots 含全部保留页链路，闭包自动多母版化），可独立交付「多页子集导出」小能力、独立可测。
2. **子图提取**（复用）：非基底源文件跑集合化 GC 得保留闭包 + 三跳页序映射。
3. **注入层**（全新核心，2-3 人日）：
   - part 名映射：注入闭包内全部 part 全局计数器重命名（slide101.xml 起），`_rels` 随 part 同步改名；
   - 逐 part 字节拷贝 + rels Target 按映射重写（未命中且不在基底 = 指向未购物页，走 danglingRelsDropPredicate 同款删除）；
   - `presentation.xml.rels` 追加 slide/master 关系（rId 基底 max 递增）；
   - `presentation.xml`：sldIdLst 按购物栏顺序追加（sldId ≥256 基底 max+1）、sldMasterIdLst 追加注入母版（id ≥2147483648 基底 max+1）；
   - `[Content_Types].xml` Override 按 PartName 映射追加 + 缺失媒体扩展补 Default。
4. **编排**：基底=购物栏第一项源文件（GC 后）；复用取消注册表（registerCancel/unregisterCancel defer LIFO）+ EventSink 事件（新 `assets-cart:*` 前缀）+ 原子写三先例；新增 service 须按 app-services-assembly 两处装配 + web_serve.go applySink 补 SetEventSink。

**前端**：素材面板内嵌抽屉（vue-draggable-plus 拖拽）、三处加号入口、页级多选模式、SaveFileDialog 导出、assets-cart 事件消费。

**测试**：程序化构造多源 pptx fixture（复用 pptx_split_test.go 基建），断言含 part→rels 方向 r:id 可解析校验；真机 PowerPoint/WPS 打开无修复弹窗为验收线。

## Decision (ADR-lite)

**Context**：购物栏导出需跨源文件页合并；纯 Go 生态无开源 deck 合并库（unioffice 商业 EULA 已核实），拆分基建已解决单文件内保留页子集正确性。
**Decision**：路线 A 基底注入式合并——GC 集合化泛化 + 跨包子图注入（part 名/rId/母版 ID 三重冲突消解），母版/版式/主题整链注入不合并 theme（PowerPoint「保留源格式」语义）。B（重绑基底母版）保真不可控否决；C（现成库）前提不成立否决；D（COM）CI 不可测，列后续 Windows 增强通道。
**Consequences**：保真高、CI 可测、与拆分同技术栈；16:9/4:3 混排不自动缩放（warning 提示）、备注页剔除、嵌入字体剥离为已知取舍。

## Out of Scope

* COM 双通道高保真导出（Windows 桌面增强，后续任务）
* notesMaster 搬运（备注页完整保留；MVP 剔除备注页）
* 嵌入字体搬运（MVP 剥离 embeddedFontLst + fntdata）
* 混排页面尺寸自动缩放（MVP 仅 warning）
* 多购物栏/命名集、导出 PDF/图片格式
* 跨页超链接保留（指向未购物页的关系删除，拆分同语义）

## Research References

* [`research/ooxml-merge-approaches.md`](research/ooxml-merge-approaches.md) — 四路线对比：A 推荐两步交付约 3-5 人日核心；B/C 否决（保真/商业 EULA）；D 列后续增强。

## Technical Notes

* 拆分 GC 已实现「单文件内保留页子集产出合法 pptx」；购物栏导出=多文件页集合并，难点在跨包注入层三重冲突消解。
* 选中键 (fileId, slideNum) 重解析全变，购物栏条目键同样禁 slides.id（fileId=files.id 稳定）。
* 相关 spec：docs/spec/pptx-split.md、docs/spec/assets-management.md、docs/spec/assets-thumbs-com.md。
* ID 冲突三命名空间独立：sldMasterId@id ≥2147483648、sldId@id ≥256、presentation.xml.rels rId。
* 媒体 Default（png/jpeg）基底缺失扩展名须补 Default；OPC Target 相对关系源 part 目录解析（既有踩坑先例）。

## Implementation Plan (small PRs)

* PR1（后端核心）：GC 集合化泛化 + 多页子集导出能力 + fixture 测试（独立可交付）。
* PR2（后端核心）：跨包子图注入层（part 改名/rels 重写/Content_Types/presentation.xml 合并）+ 多源合并编排（取消/事件/原子写）+ 合并正确性 fixture 测试。
* PR3（前端+装配）：素材服务装配（购物栏 service/存储 data/asset_cart.json）+ 购物栏抽屉 UI（三处加号入口、拖拽排序、溯源跳转、批量加页、失效条目）+ 导出对话框 + 事件消费 + 文档/README/spec 沉淀。
