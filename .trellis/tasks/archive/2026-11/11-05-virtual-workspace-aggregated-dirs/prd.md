# 虚拟聚合工作目录:常用文件夹右键收藏与统一浏览

## Goal

在文件树面板提供「聚合视图」模式:用户通过右键将常用文件夹收藏后,可在同一棵树中统一查看、展开、编辑这些目录下的文件,免去在多个工作目录之间反复切换。

## Requirements

1. **模式切换**:FileTreePanel 树工具栏增加切换开关,「当前目录树 ⇄ 聚合树」共用一个面板
2. **收藏入口**:复用现有右键「收藏/取消收藏」链路(后端 `FavoritesService`),不另建配置
3. **聚合树结构**:虚拟根节点下平铺所有**目录类型**收藏(路径存在且为目录的收藏条目;文件类收藏不进聚合树),忽略 Group 分组
4. **浏览与编辑**:收藏目录展开懒加载、点击文件预览、编辑保存、新建/重命名/删除/复制粘贴——全部走既有绝对路径链路(`GetFileTree` / `PreviewFile` / `SaveFile` / `CreateDirectory` / `Rename` / `Delete`),预期零后端改动
5. **目录内按名筛选**:聚合视图下 Ctrl+F 覆盖层筛选作用域 = 点击的目录节点自身(适配 `runFilter` 作用域逻辑)
6. **「设为工作目录」**:聚合树内右键收藏目录可一键提升为当前工作目录(切换回普通树并定位到该目录)
7. **失效收藏兜底**:收藏目录被删除/移动后,聚合树节点标灰 + tooltip 提示,右键可移除该收藏
8. **状态隔离**:普通树/聚合树两模式的展开状态持久化 key 隔离,互不污染

## Acceptance Criteria

- [ ] 收藏 3 个不同盘符/层级的目录后,聚合树均可见、可展开、懒加载正常
- [ ] 聚合树内点击文本文件可预览并编辑保存,内容落盘正确(含中文不乱码)
- [ ] 聚合树内新建/重命名/删除文件操作正确作用于对应绝对路径
- [ ] 聚合视图内 Ctrl+F 筛选命中范围 = 点击目录子树,退出筛选恢复正常
- [ ] 右键收藏目录「设为工作目录」后,普通树切换为该目录且树上可见
- [ ] 删除某收藏目录对应的磁盘目录后,聚合树该节点标灰且 tooltip 可读,可右键移除
- [ ] 取消收藏后目录从聚合树消失;重复收藏、收藏上限 100 提示正常
- [ ] 普通/聚合两模式树展开状态互不污染(切换往返后各自保持)
- [ ] 超过 2000 条目目录在聚合树内正常触发截断哨兵(复用既有截断链路)

## Definition of Done

- 单测覆盖:虚拟根节点构造、模式切换状态隔离、筛选作用域、展开回溯适配、失效判定
- E2E fixture 对齐后端 model 真实形状(`frontend/e2e/fixtures.js`);新增 spec 目录须同步 vitest `test.exclude`
- 覆盖率门禁达标(前端 ≥70% 硬失败)
- 若新增 App 方法/model struct 字段:同步 `frontend/wailsjs/` 三处(App.js/App.d.ts/models.ts);纯桌面数据面方法同步 `RPCHiddenMethods`
- 后端如有改动须 `go vet` + `go test` 通过;前端 `npm run build` + `npm test` 通过
- 确认 README.md 是否需更新

## Technical Approach

**核心思路(方案 A:前端虚拟根聚合)**:后端全链路绝对路径驱动(`FileTreeNode.Path` 恒为绝对路径且 `ID=path`),树为懒加载——根节点用工作目录路径、其余节点一律用节点自身绝对路径。聚合树 = 把 level 0 根解析改为虚拟根,其子节点 = 收藏目录列表(前端构造),后续展开/预览/编辑/文件操作全部复用既有链路。

**已识别的适配点**:

| 位置 | 现状 | 适配 |
|---|---|---|
| `loadTreeNode`(FileTreePanel.vue:744) | level 0 取当前工作目录 | 聚合模式 level 0 返回前端构造的收藏目录节点数组 |
| `runFilter`(:681) | 作用域 `treeFilterScope.path \|\| currentWorkDir.path` | 聚合模式作用域=点击的目录节点 |
| `findExpandedAncestor`(:833) | 按工作目录前缀匹配回溯 | 聚合模式按各收藏根前缀匹配(或直接定位收藏根) |
| `useTreeState` | 单一树状态 | 聚合模式独立持久化 key |
| 虚拟根子节点 | 后端填充 `IsGitRepo`/`HasRemote` | 前端构造节点需补 git 状态(实现时确认 `GetGitInfo` 绑定复用) |
| 失效判定 | — | 实现时确认轻量存在性检查途径(展开失败兜底标灰或轻量 Stat 绑定) |

## Decision (ADR-lite)

**1. 呈现形态(2026-11-05)**:
- Context: 备选 FileTreePanel 双模式切换 / 独立面板 / 侧边栏固定分区
- Decision: FileTreePanel 内双模式切换
- Consequences: 改动最小、交互一致;代价是组件内模式状态增多,需隔离展开状态与筛选作用域防互串

**2. 收藏分组展示(2026-11-05)**:
- Context: 收藏条目自带 Group 字段
- Decision: MVP 平铺,忽略分组
- Consequences: 常用目录少时折叠层是多余交互;分组折叠留二期纯展示增强

**3. MVP 边界(2026-11-05)**:
- Context: 增强项取舍
- Decision: 纳入目录内筛选、「设为工作目录」、失效兜底;排除拖拽移动
- Consequences: 核心闭环 + 高频增强 + 稳健性兜底齐备;跨目录拖拽编排复杂留二期

## Out of Scope

- 访问频率自动推荐「常用目录」(二期,`RecentAccess` 基建可支撑)
- 聚合树内跨目录拖拽移动文件(二期)
- 聚合视图内跨仓库 git 批量操作
- 收藏分组折叠展示(二期)

## Technical Notes

- 关键文件:`frontend/src/components/FileTreePanel.vue`(树+右键菜单+筛选+状态持久化)、`frontend/src/store/favorites.js`、`frontend/src/store/directory.js`、`service/favorites.go`、`service/filetree.go`、`app_favorites.go`、`model/models.go`(Favorite/FileTreeNode)
- 被否方案:B 后端虚拟目录服务(绝对路径已天然可用,映射层过度设计);C 复用多工作目录切换(仍是切换式非聚合)
- **实现前置调研已收口**(详见 [research/implementation-precheck.md](research/implementation-precheck.md)):useTreeState 聚合 key 用 `treeState:__aggregated__`;workspaceStore 天然兼容零适配;收藏可含文件须过滤;推荐新增后端聚合绑定 `GetAggregatedRootNodes(paths)` 一次收口失效判定+git 徽标(新增 App 方法的 wailsjs 三处 + RPCHiddenMethods 同步义务见 research §5)
- 已确认基建:右键收藏入口(`isFavorited` computed)、收藏上限 100/去重提示、Ctrl+P 收藏搜索(`selectFavorite`)、截断哨兵按目录 path 截断(兼容)

## Implementation Plan (small PRs)

- **PR1 聚合模式骨架**:模式状态 + 工具栏切换开关 + 虚拟根节点构造(消费 favoritesStore 过滤目录类收藏)+ 失效判定与标灰 + 单测
- **PR2 行为适配**:runFilter 作用域适配 + findExpandedAncestor 回溯兜底 + 树状态持久化 key 隔离 + 「设为工作目录」右键项 + 单测
- **PR3 收尾**:E2E spec + fixture 对齐 + 文档更新(README + 必要的 spec 沉淀)
