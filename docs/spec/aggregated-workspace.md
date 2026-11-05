# 聚合视图（虚拟工作区）契约

> 版本：v1（2026-11-05）｜关联实现：任务 `11-05-virtual-workspace-aggregated-dirs`

## 1. 定位与边界

**聚合视图**指文件树面板的第二种模式：以虚拟根节点平铺展示全部**目录类型**收藏，用户无需在多个工作目录之间切换即可统一浏览、展开与编辑这些目录下的文件。

**MVP 边界**：

| 纳入 | 排除（二期） |
|---|---|
| 收藏目录平铺（忽略 Group 分组） | 收藏分组折叠展示 |
| 展开懒加载、预览、编辑保存、新建/重命名/删除/复制粘贴 | 聚合树内跨目录拖拽移动 |
| 目录内按名筛选（作用域 = 点击的目录） | 访问频率自动推荐常用目录 |
| 右键「设为工作目录」 | 跨仓库 git 批量操作 |
| 失效收藏标灰 + 右键移除 | — |

## 2. 数据链路

### 2.1 虚拟根与根层来源

- 聚合树根为**前端虚拟节点**，不落盘、不持久化、不进 `GetFileTree` 链路。
- 根层节点由后端 `App.GetAggregatedRootNodes(paths []string) []*model.FileTreeNode` 一次返回：
  - 每个入参路径 `os.Stat`：目录 → 正常节点（复用 `isGitRepoDir`/`hasRemote` 缓存填 git 徽标）；
  - 文件 → **跳过**（收藏夹允许文件收藏，文件类收藏不进聚合树）；
  - Stat 失败 → `Missing=true` 占位节点（`type` 仍为 `directory`，`name` 取路径末段），**禁静默丢弃**。
- 顺序恒与入参一致（收藏列表顺序），不排序、不去重。
- 前端 `loadAggregatedRootNodes` 消费后：
  - **聚合根集合**：祖先已被收藏的项不再单独列为根（`aggregatedFavoriteRoots`，顺序无关、同路径保留首个，路径归一含大小写/分隔符/尾分隔符）。el-tree `node-key="path"` 下同一路径在根层与子层同时注册会互相覆盖 `nodesMap`（展开态/定位/刷新错位）；父收藏展开后仍可浏览到该目录。该集合**同时是 `findAggregatedRootFor` 的匹配面**（二者同源），否则被剔除的子收藏仍会被最长前缀命中，定位/刷新落到树上不存在的一级节点；
  - 收藏 `alias` 非空时覆盖节点显示名（后端只知路径）；
  - `isLeaf = type === 'file' || missing || !hasChildren`（失效占位不可展开，防展开请求打空磁盘）；
  - 统一走 `truncateTreeNodes(nodes, '__aggregated__')` 截断链路（上限 100 收藏，理论不触发，保持行为一致）；
  - 加载三态 `aggregatedRootLoading/aggregatedRootError`：加载中与失败态**不得**显示「暂无目录收藏」，否则误导用户认为收藏丢失。

### 2.2 后续行为复用既有绝对路径链路

收藏目录展开后的子节点、预览、保存与文件操作全部复用 `GetFileTree` / `PreviewFile` / `SaveFile` / `CreateDirectory` / `Rename` / `Delete` 等既有绑定（`FileTreeNode.ID === Path`，懒加载按节点自身绝对路径拉取），**零后端业务改动**。

### 2.3 契约字段

`model.FileTreeNode.Missing bool json:"missing"`：

- 仅 `GetAggregatedRootNodes` 为失效收藏产出占位节点时置 `true`；普通树节点恒为零值，旧数据反序列化兼容。
- 失效占位节点须显式 `HasChildren=false`、`IsLeaf=true`（`NewFileTreeNode` 对 directory 的默认值是与 `Missing` 矛盾的「有子节点、可展开」）。
- 新增字段须同步 `deepCopyNode`（`service/filetree_cache.go`）与 `TestDeepCopyNode_CoversAllFields` 的非零构造，否则反射守护测试失败。
- `frontend/wailsjs/go/models.ts` 的 `FileTreeNode.missing: boolean` 随 `wails generate` 生成；`GetAggregatedRootNodes` 已登记 `main.go` 的 `assembleWebServe` 装配的 `RPCHiddenMethods`（载荷含任意入参路径的存在性信息，浏览器通道不可调用，前端入口按 `isBrowserChannel()` 隐藏）。

## 3. 模式切换与状态隔离

| 机制 | 契约 |
|---|---|
| 模式状态 | `FileTreePanel` 内 `aggregatedMode` ref，工具栏按钮 `data-action="toggle-aggregated-view"` 切换 |
| 浏览器通道 | `GetAggregatedRootNodes` 已登记 `RPCHiddenMethods`，浏览器（serve）模式**启动期隐藏切换入口**（`isBrowserChannel()`，与 SettingsPanel/useIPBans 隐私入口先例一致），禁点击报错 |
| 树重建 | `treeKey` = `${聚合 ? '__agg__' : selectedDirectoryId}_${refreshCounter}`；聚合树与工作目录无关，**不绑定 selectedDirectoryId**，切目录不重建 |
| 就绪等待 | `waitTreeReady()`：树已有根节点直接通过，否则由 `treeReadyPromise`（loadTreeNode 完成信号）驱动，3000ms 超时兜底；空收藏不空等 |
| 切换竞态 | `modeSwitchSeq` 代次守卫；在途切换（`modeSwitchInFlight`）期间跳过保存，恢复 key 显式固化目标模式，防旧树状态写入新模式 key；`refreshAggregatedRoot` 共用同一代次，且恢复前复查 `aggregatedMode`，防聚合展开态落到普通树 |
| 状态持久化 | 聚合模式固定 key `treeState:__aggregated__`，普通树沿用 `treeState:<dirPath>`；分流收口在 `currentStateKey()`，`useTreeState` 本体零改动 |
| 切换时序 | `switchTreeMode`：保存旧模式状态 → 翻转 `aggregatedMode` → `exitFilterMode()` + 清空 `lastClickedNodeData` → 等新树根就绪 → 恢复新模式状态 |
| 「设为工作目录」退出 | `exitAggregatedMode({ restoreState: false })` 跳过新树状态恢复（`onDirectorySelect` 即将保存/恢复目标目录状态） |

## 4. 筛选作用域与展开回溯

- **筛选作用域**（`runFilter` / `filteredDisplayNodes` / `treeFilterPlaceholder`）：
  - 普通树：点击的目录节点 → 回退当前工作目录根；
  - 聚合视图：**恒为点击的目录节点，无工作目录回退**——收藏目录间无公共祖先，未点击即无作用域，直接不拉取（placeholder 与覆盖层 summary 均引导先点击目录，禁显示「工作目录内 0 命中」误导文案）。**无作用域时树保持可见可交互**（`v-show` 放行），否则引导文案指向的目录被覆盖层挡住，筛选成为死路。
- **展开回溯**（`findExpandedAncestor`）：
  - 普通树按工作目录根回溯，回溯到根返回 `null`（根不在 `nodesMap`，由 `store.root` 兜底）；
  - 聚合视图按**最长前缀收藏根**回溯（`findAggregatedRootFor`，兼容 `\`/`/` 与大小写），收藏根在 `nodesMap` 中可本身作为刷新目标（未展开也可，`expand` 会加载最新子节点）；目标不在任何收藏范围内返回 `null`。
  - 路径前缀判定必须为 `target === root || target.startsWith(root + sep)`，排除仅共享前缀串的兄弟目录（`/a/b` 与 `/a/bc`）。
- 刷新分隔符基准：聚合视图取目标所属收藏根（无命中取目标自身），普通树取工作目录根。

## 5. 失效兜底与「设为工作目录」

- 失效节点：树行灰色 + 删除线 + `title` 提示（「目录不存在或已移动（收藏失效），可右键移除收藏」）；点击仅提示、不选中、不 emit `select`；`locateNode` 不选中失效根；快捷键 `triggerRenameCurrent`/`triggerDeleteCurrent` 经 `isUnusableNode`（截断哨兵或 `data.missing`）拦截，防对不存在路径发起重命名/删除。
- 失效节点右键菜单：**仅**「移除收藏」「复制路径」（其余文件操作对不存在路径无意义）。
- 收藏增删后（`handleAddFavorite` / `handleRemoveFavorite` 成功分支）调 `refreshAggregatedRoot()`：保存展开态 → `refreshCounter++` 重建根层 → 恢复展开态（根层是收藏列表快照，`refreshNode` 只刷新子层，无法感知根层增删）。
- 收藏根自身的重命名/删除：`refreshAfterFileOp(parentPath)` 判定父目录不属于任何收藏根时改调 `refreshAggregatedRoot()`（`refreshNode` 对收藏范围外路径会静默早退）；收藏记录仍指向旧路径，重建后按 `Missing` 占位兜底提示用户移除。
- 「设为工作目录」（`handleSetAsWorkDir` → `Home.vue` `onSetAsWorkDir`）：
  1. 工作目录列表已有同路径（`isSameDirPath`：分隔符归一 + 去尾分隔符 + 小写比较，Windows 语义）则复用，否则 `AddDirectory` + `loadDirectories`；后端判重返回 nil 时回查列表再判定（防「目录已存在却报添加失败」）；
  2. 退出聚合模式（跳过状态恢复）；
  3. `onDirectorySelect` 切换（既有保存/恢复链）；
  4. `locateNode` 定位目标。
  - 聚合树下 `locateNode` 支持目标即收藏根（树一级节点可直接选中并滚动居中）；普通树工作目录根不在树中，维持静默返回。

## 6. 测试要求

- 后端单测：`service/filetree_aggregated_test.go` 覆盖目录/文件/失效三类入参分流、git 徽标、顺序保持、空入参与空串防御。
- 前端单测：`FileTreePanel.spec.js`「聚合视图（虚拟工作区）」覆盖模式切换与 `treeKey`、虚拟根构造与别名/`isLeaf`、聚合绑定失败降级、空态引导、失效节点点击与右键菜单、状态 key 隔离、筛选作用域、展开回溯、`set-as-work-dir` emit。展开/收起行为断言须用真实 EP `mount`（禁自造 el-tree 桩假绿）；本组状态隔离用例仅断言持久化 key 与回溯函数，可用桩。
- E2E：`e2e/file-tree.spec.js` 顶层 `聚合视图（虚拟工作区）` 覆盖切换、别名与失效标灰、失效右键菜单、切回目录树；mock 数据经 `wailsOverrides` 注入，默认值登记于 `src/test/wails-mock-defaults.js`（`GetAggregatedRootNodes: []`）。
