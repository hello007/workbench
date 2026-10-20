# 素材管理：文件夹树与拖拽整理

## Goal

素材管理面板（AssetsPanel）左侧新增可收缩「文件夹树」，展示当前素材库根目录下的真实子目录结构；点击文件夹节点递归展示该子树下全部 PPT 素材；支持文件夹节点管理（新增/删除/重命名）与文件夹级标注；支持把 PPT（单/多选）及文件夹拖拽移动到其他目录（真实移动文件并同步索引）。

## Requirements

* R1 可收缩文件夹树栏：树 = 库根真实子目录（D1），含空目录；排除规则（ExcludeDirs）命中目录带标记展示且禁管理操作（h）
* R2 点击节点递归过滤主区素材（含子目录，D3）；节点徽标显示递归 PPT 数/页数（a）；根节点「全部素材」
* R3 目录管理：新增/删除/重命名 = 磁盘目录操作（空目录删除、非空删除需确认）；重名拒绝并明确报错（f，自决）
* R4 文件级拖拽移动：单/多选（Ctrl/Shift）PPT 卡片/行拖到目标目录（c）；幻灯片页不可独立移动（页归属文件）
* R5 目录级拖拽移动（d）：复用前缀替换机制重链其下全部文件索引
* R6 移动健壮性（g）：失败回滚与错误提示；跨盘移动拒绝并明确报错；移动目标重名拒绝
* R7 检索联动（e）：文件夹为独立过滤维度，与 keyword/tags/facets AND 叠加；语境矩阵登记
* R8 文件夹级标签/备注（b）：对齐文件/页级标注体系
* R9 移动后索引同步：专用移动 API——os.Rename + 事务内 UPDATE path + FTS 行重建（禁裸 UPDATE）+ relPath 前缀重算；缩略图缓存键（files.id）不变可复用

## Acceptance Criteria

* [ ] 树栏可收缩隐藏，展开时三列布局内完整渲染（flex 约束链完整）
* [ ] 空目录在树中可见；ExcludeDirs 命中目录带标记、右键菜单无新增/删除/重命名
* [ ] 点击节点主区仅显示该子树 PPT；徽标计数与实际条目数一致；切库/清语境点位正确复位
* [ ] 新增/删除/重命名后树即时刷新，重名操作报明确错误；删除非空目录需二次确认
* [ ] 单选/多选拖拽移动后：磁盘文件位新、索引 path 更新、FTS 检索命中新路径、缩略图不重生成、relPath 前缀正确
* [ ] 目录拖拽移动后其下全部文件索引重链正确（嵌套腾位按 oldPath 降序）
* [ ] 跨盘移动被拒绝并提示；移动失败磁盘与索引状态一致（不留半态）
* [ ] 检索语境下选中文件夹，结果同时满足关键词/标签/facet 与目录前缀
* [ ] 文件夹标签/备注保存后刷新回读一致（双代次守卫防旧代次写回）
* [ ] 后端 service 单测覆盖列目录/移动事务/FTS 重建/排除态；前端 vitest 覆盖树组件/过滤/拖拽编排；覆盖率门禁达标

## Definition of Done

* 后端/前端测试通过（service ≥76%、前端 ≥70% 门禁）
* 真机验证拖拽与目录管理（Windows）
* Wails 绑定新增方法同步 frontend/wailsjs/ 三处（App.js/App.d.ts/models.ts）
* 若新增 service 事件：web_serve.go applySink 补 SetEventSink
* 文档更新：功能说明.md / API参考.md（新增绑定方法）/ README 确认
* 规范沉淀：新契约写入 docs/spec/assets-management.md 相应节

## Technical Approach

* 后端：AssetLibraryService 新增 ListFolders（读 FS 列目录，含排除态标记与统计）、CreateFolder/DeleteFolder/RenameFolder、MoveAssetFile/MoveAssetFolder；目录/移动操作与扫描互斥（复用 tryLockScan 口径评估是否需要）
* 前端：AssetsPanel 内新增 FolderTree 子组件（el-tree，参照 AssetTagTreeSelect 拖拽先例）；文件选中维度叠加 folderPath 前缀过滤；拖拽编排多选集合
* 移动事务：单事务内完成 path 更新 + FTS 行重建；拖拽 done 走返回值汇总（对齐批量拆分口径，不逐文件发事件）

## Decision (ADR-lite)

**D1 文件夹树 = 真实文件系统目录**（用户确认）
Context：库根目录下已有真实子目录，虚拟分组会与之并存造成两套结构。
Decision：树 = 库根真实子目录；管理操作即磁盘目录操作；拖拽即移动真实文件。
Consequences：需处理移动后索引同步（path 更新 + FTS 重建 + 缩略图键复用）与排除规则交互；面板结构与资源管理器一致。

**D2 MVP 范围 = 全量（a–h 全进）**（用户确认）

**D3 点击节点 = 递归含子目录**（用户确认）
Context：素材管理定位「找素材」，递归口径点上级即可纵览，与「库根 = 全部」自洽。
Decision：递归展示整棵子树 PPT，徽标递归计数，根节点 = 全库。
Consequences：过滤实现按 relPath 前缀匹配；徽标需后端统计或前端聚合（前端聚合 fileEntries 即可，零后端成本）。

**自决项（推荐口径，可推翻）**
* 树数据源 = 后端新增列目录 API（直接读文件系统，非索引派生）——空目录可见、与管理操作天然一致
* 拖拽对象 = PPT 文件级（文件卡片/行）+ 目录级（d）
* 移动后索引同步 = 专用移动 API 同步事务，不走 lost+重链（异步扫描兜底不适用显式移动）
* 重名冲突拒绝报错（对齐 os.ErrExist 口径，不自动改名）
* 跨盘移动拒绝报错（MVP 不做 copy+delete 降级）

## Out of Scope

* 触屏长按手势适配（桌面鼠标拖拽）
* 跨盘 copy+delete 降级移动
* 重名自动改名
* 树节点拖拽排序（目录顺序=文件系统字典序）

## Research References

* （待补：若实施中需要 el-tree 拖拽与移动事务的细节勘察，经 trellis-research 落 research/）

## Technical Notes

* 关联文件：frontend/src/components/AssetsPanel.vue、service/assets_library.go、service/fileoperation.go、service/assets_scan*.go（排除 matcher 单点复用，禁两套口径）
* 移动/目录操作与 ExcludeDirs matcher 共用同一 matcher（对齐 §12.1 契约）
* 语境矩阵：folderPath 为新过滤维度，登记进 assets-management.md §8 清空点位矩阵
* FTS 重建参照 excluded/lost 教训（裸 UPDATE 翻 path 会漏检）
* 排除产物目录（isPptxSplitDerivative 拆分产物）不应出现在可操作树节点（派生产物非用户目录）——实施时确认扫描/树口径
* 缩略图缓存键 fileID 稳定，移动后不失效；pptx 预览缓存键含 srcPath，移动后自然失效重建，可接受

## Implementation Plan (small PRs)

* PR1：后端 ListFolders API + 树数据模型 + 前端可收缩 FolderTree 组件（递归过滤 + 徽标 + 排除态标记）+ Wails 绑定三处同步 + service/前端单测
* PR2：目录管理（新增/删除/重命名 + 排除态禁操作 + 空点位矩阵登记）+ 检索联动（folderPath AND 叠加）
* PR3：文件级拖拽移动（单/多选编排 + MoveAssetFile 移动事务 + FTS 重建 + 重名/跨盘拒绝）+ service 单测
* PR4：目录级拖拽移动（前缀替换复用）+ 文件夹级标签/备注 + 文档更新 + 规范沉淀
