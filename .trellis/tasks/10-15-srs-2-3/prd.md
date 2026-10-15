# 素材管理第五批：素材视图与选中体系（SRS §2.3）

## Goal

补齐素材管理三块视图/交互能力：① 列表视图（表格元数据，与画廊网格切换并存）；② 幻灯片全屏预览 + 翻页浏览（hd 档）；③ 文件/幻灯片两种选中模式（多选 + 全选/反选 + 计数，为第六批批量操作预留数据结构）。现有交互（溯源/打开跳转/拆分入口/标注编辑）在列表视图与全屏预览语境保持可用。

## What I already know（代码盘点结论）

- **现有面板形态**（AssetsPanel.vue 2576 行）：`viewMode`（file/slide）× 画廊网格单一呈现；详情侧栏 tier=1 单张 hide-on-miss；无列表视图、无全屏预览、无多选。
- **模型字段盘点**（决定列表视图列集）：
  - `AssetFileEntry`：id/path/size/modTime/status/title/note/rating/tags——浏览文件视图列集齐
  - `AssetSlideEntry`：id/fileId/filePath/slideNum/title/text/speakerNotes/tags(继承)/ownTags/note/rating——**无 size/modTime**（文件属性）
  - `AssetSearchHit`：refType/refId/fileId/filePath/title/slideNum/snippet/hitCount/tags/ownTags/note/rating——**无 size/modTime/status**
- **API 现状**：`ListAssetSlides(libraryId, limit, offset)` 无按 fileId 过滤参数——「全屏翻归属文件全部页」需新后端绑定；「翻当前视图条目序列」纯前端可行。
- **fileEntries 底册**：面板已预载全量文件清单（标签聚合/溯源共用）——幻灯片列表视图 size/modTime 列可前端 join，零后端改动。
- **缩略图消费纪律**（硬约束）：画廊 tier=0 可视区懒加载 / hd tier=1 单张按需 hide-on-miss / 返回串直拼 `data:image/png;base64,` / 失败静默回退占位 / epoch 代次自动补图。
- **z-index 惯例**：终端全屏 1500 < 右键菜单 2000 ≤ EP 弹窗 2001+；ESC 过滤 `defaultPrevented` 与 `.el-overlay/.xterm-helper-textarea`。
- **异步守卫惯例**：librarySeq/searchSeq 双代序号，await 写回前比对丢弃旧响应。
- **已知 UX 缺口**（登记项）：失败缩略图跨会话无自动补（失败后不重扫恒无预览，仅单文件 RetryAssetThumb 手动重试）。本批是否顺带收口待定。

## Assumptions (temporary)

- 纯前端批候选：列表视图 + 全屏预览（视图序列方案）+ 选中体系均可零后端落地；待 Q1 定夺。
- 选中模式跟随 viewMode（文件视图选文件 / 幻灯片视图选页），不设独立选中维度开关。

## Open Questions

- Q5 冲突矩阵：聚合折叠卡 × 选中、语境切换 × 选中集合、预览态 × 详情编辑

## Requirements

- 列表视图：表格展示条目元数据（列集对齐 D3），与画廊网格切换并存，滚动懒加载对齐现有契约（ListAssetSlides limit/offset + SearchAssets 分页）
- 全屏预览：幻灯片全屏查看 + 翻页（键盘左右/按钮），翻页序列 = 当前视图幻灯片条目子序列（D1），hd 档（tier=1 hide-on-miss），预览态详情侧栏保持可编辑（D5#6）
- 两种选中模式：文件模式（选整个 PPT）/ 幻灯片模式（选单页），常驻复选框 + 底部浮动操作条（D2），多选 + 全选/反选 + 选中计数；selectedKeys 键形态为第六批批量预留
- 缩放滑块：画廊缩略图尺寸调节，会话态不持久化（D4）
- 失败缩略图自动补：thumbs done 后失败集自动重试一轮（D4）
- 现有交互兼容：溯源/打开跳转/拆分入口/标注编辑在列表视图与全屏预览语境可用；冲突矩阵对齐 D5

## Acceptance Criteria

- [ ] 网格/列表双向切换，文件/幻灯片两种 viewMode 下均可用；切换呈现选中集合保留（D5#5）
- [ ] 列表视图滚动到底增量加载（检索模式 + 浏览幻灯片模式）；幻灯片列表归属文件大小/修改时间 join 正确，底册 miss 显 `—`
- [ ] 全屏预览键盘 ← → 与按钮翻页；hd miss 时整块优雅降级不占位；预览翻页详情侧栏跟随
- [ ] 多选勾选不触发详情（@click.stop）；全选/反选作用于当前渲染卡片键集；浮动条计数正确
- [ ] D5#2/#3/#4 各清空点位生效（切库/检索语境/viewMode 切换/扫描刷新清空选中）
- [ ] 聚合折叠卡无复选框；子项可勾选
- [ ] thumbs done 后失败集自动重试恰好一轮；下轮 done 不再自动触发
- [ ] 前端测试 ≥70% 覆盖率；E2E 从 fixtures.js import；禁 waitForTimeout；mock 改 wails-mock-defaults.js 单一数据源

## Definition of Done

- 单元测试覆盖新增状态机/组件；lint/typecheck 通过；覆盖率门禁通过
- spec 沉淀（预览层级选型、选中数据结构契约）
- README/docs 更新确认

## Out of Scope (explicit)

- 批量操作本体（打标签/删除等——第六批消费选中数据结构）
- facet 多维筛选/保存检索条件（第七批）
- 多级标签树（第六批）

## Decision (ADR-lite)

### D1 全屏预览翻页数据源 = 当前视图条目序列（2026-10-15）

**Context**：`ListAssetSlides` 无按 fileId 过滤参数，「翻归属文件全部页」需新增后端绑定 + wails generate 三处同步，破坏纯前端批前提。
**Decision**：翻页范围 = 画廊当前可见幻灯片条目序列（浏览模式 = 幻灯片清单已加载段，滚动到底续拉；检索模式 = 命中结果序列）。零 service 改动。
**Consequences**：本批坐实纯前端批；「单文件内翻全部页」不支持（低频路径，后续批可加只读绑定独立增量）；翻页语境与用户所见一致（检索命中里翻 = 翻命中）。

### D2 选中交互 = 常驻复选框 + 底部浮动操作条（2026-10-15）

**Context**：挑素材高频「边看边选」，选择模式开关每次勾选先切模式动线多一步；点击进详情与勾选需并行不互斥。
**Decision**：网格卡/列表行常驻复选框（点击主体仍进详情，勾选独立操作）；全选/反选 + 选中计数在底部浮动操作条（选中数 > 0 时浮出，第六批批量按钮挂此条）。选中维度跟随 viewMode，不设独立开关。
**Decision（数据结构）**：`selectedKeys = Set<string>`，键形态文件 `'file:<fileId>'`、幻灯片 `'slide:<fileId>:<slideNum>'`。**禁用 slides.id 作选中键**（每轮重解析全变，与缩略图缓存键用 files.id 同理由）；类型前缀防第六批批量消费时类型歧义，键反查条目经底册/清单。
**Consequences**：浏览与勾选两套交互同屏（网格卡视觉密度略增）；批量操作第六批直接消费 selectedKeys，跨批契约以此键形态为准。

### D3 列表视图列集与排序（2026-10-15）

**Context**：`AssetSearchHit`/`AssetSlideEntry` 无 size/modTime；前端排序与滚动懒加载叠加语义破碎（排序只作用已加载段，续拉打乱序）。
**Decision**：列集分语境——文件列表（浏览）：复选框·缩略图·名称·**路径**（实现补充，2026-10-15 审核确认：对齐画廊卡信息面 asset-card-sub，同名文件区分依赖路径）·大小·修改时间·星级·标签·状态；幻灯片列表（浏览）：复选框·缩略图·页标题·归属文件·页码·归属文件大小†·归属文件修改时间†·星级·自有标签；检索模式同上经 fileEntries 底册 join（聚合条目 hitCount>1 不显大小，底册 miss 显 `—`），幻灯片检索条目追加命中摘要 snippet 列。† join 归属文件属性，表头标注口径。
**Decision（排序）**：本批不做列排序交互，保持后端序（浏览 = 路径+页序、检索 = 相关性降序）；第六批 facet 批引入后端排序参数时一并闭环。
**Consequences**：列表视图零后端改动；幻灯片列表 join 依赖 fileEntries 底册已预载（面板既有行为）。

### D4 缩放滑块与失败缩略图自动补纳入本批（2026-10-15）

**Decision（缩放滑块）**：画廊缩略图尺寸调节纳入——CSS 变量驱动卡宽 + el-slider，纯前端；**会话态不持久化**（不进 SaveSettings 链路，切库/重开恢复默认档）。
**Decision（失败缩略图自动补）**：纳入，轻量收口——`assets-thumbs:done` 事件后 failedThumbPaths 非空则逐个 `RetryAssetThumb` 自动重入队，**只自动重试一轮**（下轮 done 不再自动重试，需手动）；覆盖瞬态失败（COM 队列占用等），损坏类必然再失败但一轮即止不放大。
**Consequences**：已登记 UX 缺口顺带收口；自动重试与手动重试共用链路，epoch 代次驱动 miss 卡补图不变。

### D5 交互冲突矩阵（2026-10-15）

**Decision**：
| # | 冲突点 | 方案 |
|---|---|---|
| 1 | 聚合折叠卡 × 选中 | 聚合卡（hitCount>1）不渲染复选框（勾选语义歧义），展开后勾子项；「全选」只作用当前渲染的独立卡片 |
| 2 | 切库 / 检索语境变更（改词/改标签/平铺/仅自身开关） | 清空选中（键属库+检索语境，驻留即幽灵选中） |
| 3 | viewMode 切换（文件 ↔ 幻灯片） | 清空（选中维度互换不通约） |
| 4 | 扫描刷新（resetAndReloadView） | 清空（条目重解析底册 id 变） |
| 5 | 呈现切换（网格 ↔ 列表） | 保留（同数据语境纯呈现切换） |
| 6 | 进全屏预览 / 翻页 | 保留；翻页时详情侧栏同步跟随当前页（预览即详情导航），侧栏标注编辑照常可用 |
| 7 | 复选框 × 卡片主体点击 | checkbox `@click.stop`——勾选不进详情，主体点击不勾选 |
| 8 | 全选范围 | 当前已渲染卡片（含检索已加载段；懒加载未加载段不进全选范围，浮动条计数口径一致） |

**Consequences**：清空策略保守但无幽灵选中；选中集合生命周期与既有 librarySeq/searchSeq 守卫语境变更点对齐（同一批 watch/函数内清空，不新增守卫机制）。

## Technical Approach

### 组件拆分

- **AssetsPanel.vue**：宿主，持有视图状态机（viewMode × displayMode 网格/列表）+ 选中状态 selectedKeys + 全屏预览开关；列表/网格/预览/浮动条子组件按需拆出（实现时定，倾向列表视图拆 `AssetListView.vue`、全屏预览拆 `AssetPreviewOverlay.vue`、浮动条拆 `AssetSelectionBar.vue`，均无状态或受控组件，状态面留宿主防跨组件同步复杂度）
- **AssetThumb.vue**：复用不动（列表行小图同样 tier=0；预览层 tier=1 hide-on-miss）

### 视图状态机

```
viewMode: 'file' | 'slide'          // 数据维度（既有）
displayMode: 'grid' | 'list'        // 呈现维度（新增，会话态）
previewState: null | { index }      // 全屏预览：displayCards 幻灯片子序列索引（新增）
```

- 列表视图数据源 = displayCards 同源（浏览清单/检索结果统一卡片模型已就绪），仅呈现不同；滚动懒加载复用 onGalleryScroll 同款判定挂列表容器
- 全屏预览序列 = displayCards 过滤 kind==='slide'（含检索命中的幻灯片条目；聚合子项展开后同样可进预览）；进入点 = 幻灯片卡「预览」入口（详情侧栏大图点击 / 卡片 hover 按钮，实现时定）
- 翻页 = 序列 index ± 1（键盘 ← → / 按钮）；滚动懒加载边界处（浏览模式幻灯片清单）翻到末尾自动触发续拉后继续
- 预览层：面板内覆盖画廊区（absolute，非 fixed 全窗），详情侧栏保持可见可编辑；z-index 沉淀到 spec
- 预览态选中/语境变更（检索刷新/切库）时 previewState 复位关闭

### 选中体系

```
selectedKeys: Set<string>
键形态：'file:<fileId>' / 'slide:<fileId>:<slideNum>'   // 禁 slides.id（重解析全变）
```

- 勾选写 selectedKeys；全选 = 当前渲染卡片键集并入；反选 = 当前渲染键集对称差
- 浮动操作条：选中数 > 0 浮出，显示计数 + 全选/反选 + 清空（第六批批量按钮挂此条，本批只留清空与计数）
- 清空点位（对齐 D5）：切库 watch、applySearch、onViewModeChange、onFlattenChange、onSlideTagOwnChange、resetAndReloadView——全部既有语境变更函数内补一行，不新增机制
- 键反查条目：fileEntries 底册（文件）/ slideEntries + searchItems（幻灯片），第六批消费入口

### 失败缩略图自动补

- onThumbsDone 内：failedThumbPaths 非空且本轮未自动重试过 → 逐个 RetryAssetThumb（libraryId + fileId + path；fileId 从 fileEntries 按 path 反查）→ 标记已自动重试；下轮 done 不再触发（手动重试按钮不受限）
- 自动重试完成后的 done 事件 epoch 自增照常驱动 miss 卡补图

## Implementation Plan (small PRs)

- **PR1 视图状态机 + 列表视图**：displayMode 切换控件 + AssetListView（列集对齐 D3、滚动懒加载、底册 join）+ 测试
- **PR2 全屏预览**：AssetPreviewOverlay（翻页/键盘/hd hide-on-miss/详情联动）+ z-index 选型 + 测试
- **PR3 选中体系 + 浮动条 + 失败自动补**：selectedKeys + 冲突矩阵清空点位 + AssetSelectionBar + 自动重试一轮 + 测试 + spec 沉淀

### D6 真机冒烟反馈增量（2026-10-15 第一轮）

**Context**：真机冒烟暴露 2 缺陷 + 1 漏实现 + 3 新交互诉求。
**Decision**：
1. **卡片尺寸突变修复**：详情栏 300px flex 兄弟挤压画廊 + `auto-fill minmax(220px,1fr)` 重排列数（4→3）致每列 1fr 变宽（实测 459→576）。修复 = **固定卡宽网格** `repeat(auto-fill, var(--asset-card-w))`：详情开关只减列数不改卡宽，突变消除；与缩放滑块同一变量落点。
2. **缩放滑块补实现**（D4 纳入项，PR3 漏做）：el-slider 驱动 `--asset-card-w`（范围约 220-420px，默认 280，会话态不持久化）。
3. **详情按钮遮挡**：`.detail-actions` 补 `flex-wrap: wrap`（300px 栏三按钮溢出裁切）。
4. **幻灯片卡双击进全屏预览**：@dblclick 直接 openPreview（单击已装配详情，双击以当前详情定位）。
5. **幻灯片详情页文本下沉**：页文本/演讲者备注/命中摘要三区移到操作按钮区之后（展示优先级低于标注编辑）。
6. **文件卡双击 = 文件内过滤浏览**：新状态 `fileFilterPath`——双击文件卡切幻灯片视图并仅显示该文件页；过滤视图滚动懒加载照旧（全局续拉过滤收敛），过滤空时自动续拉直到命中或拉完；过滤条展示「N 页 + 清除」；切库/检索变更/扫描刷新点位同步清过滤。
7. **跳转无效果**：前端链路自查完好（本批仅重构装配函数形态等价），待用户补充现象（loading/报错/时长）后定性，本批不盲改。

**Consequences**：固定卡宽放弃 1fr 满铺（右侧留白，Eagle 式画廊常态）；过滤浏览纯前端（slideEntries 全局分页流 + 客户端过滤）。

### D7 跳转「无效果」根因修复——上会话自启残留实例误判用户实例（2026-10-15 真机定性）

**Context**：真机「打开并跳到此页」零可见效果。定性链：上会话缩略图批次自启 wpp.exe → 会话崩溃（`crash flag detected`，Shutdown 未跑）→ 幽灵 wpp.exe 残留；本会话跳转 `CreateObject` 命中残留实例（进程 diff 无新增）→ `ownedPIDs` 空 → 误判「复用用户已开实例（禁 Quit 禁杀）」→ Open+GotoSlide 在后台幽灵进程内「成功」（成功路径无日志无 toast）→ 窗口不可见。二次点击走已建会话毫秒级落定（loading 一闪即逝）同零可见。
**Decision**（后端 COM 域，本批从纯前端批扩为含后端修复）：
1. **owned 镜像持久化**：自启 PID 落盘 `data/assets/com_owned_pids.json`（util.SaveJSON 原子写）；publishOwned 时同步落盘；正常 Shutdown 收口后删文件；崩溃残留 = 下会话识别依据。
2. **ensureApp 残留回收**：CreateInstance 判「复用」（diff 空）时，当前存活进程 ∩ 盘上残留 PID（校验：进程名 ∈ {powerpnt.exe, wpp.exe} + 文件 mtime 24h 窗口，双保险防 PID 复用误杀用户实例）非空 → 识别为**自有残留**：精确强杀后重建新实例（归属清晰、窗口可见），日志口径改「回收上会话自启残留」；识别失败按原「复用用户实例」兜底。
3. **跳转成功路径补日志**：目标文件 + 页码 + 复用与否（本次排障若有此条秒定位）。
**Consequences**：纯前端批破例扩后端（真机 bug 修复优先）；PID 复用残余风险经进程名白名单 + 24h 窗口压至可接受；跳转「复用真用户实例但窗口被前台锁压制不置前」场景契约不变（Warn 可忽略）。

### D7b 第二轮定性：WPS 应用级不可见（2026-10-15 真机复测）

**Context**：残留回收修复后真机复测——幽灵已清、全新 owned wpp.exe、日志「跳转完成 slide=2」全成功，仍零可见窗口。根因二：COM `CreateInstance` 激活的 WPS 实例**应用级默认不可见**，PowerPoint 的 `Open(WithWindow:=msoTrue)` 顺带显示窗口而 WPS 兼容层无此行为。
**Decision**：`jumpInSession` 在 findOrOpen 后显式 `PutProperty(app, "Visible", msoTrue)`（PowerPoint 幂等无害，失败仅 Warn）；独立跳转与 preempt 插队跳转共用该函数同获兜底，ReadOnly 无窗口导出路径不经此函数不受影响。
**Consequences**：真机验证通过（WPS 窗口弹出跳页）；强杀残留回收闭环项单测兜底、真机未复测（用户裁可不测）；详细契约沉淀 docs/spec/assets-thumbs-com.md §9.1。

## Technical Notes

- 硬约束：缩略图消费纪律、z-index 惯例、面板根 flex 约束链、frontend-visual-conventions、mock 单一数据源 wails-mock-defaults.js、新 App 绑定须 wails generate 三处同步
- 已读文件：AssetsPanel.vue、AssetThumb.vue、model/assets.go（条目模型）、docs/spec/assets-management.md（既有契约）
