# PRD：文件树筛选覆盖层右键菜单（按名筛选配套，补截断层外文件操作入口缺口）

## 1. 背景与问题

树内按名筛选（`FileTreePanel.vue` 覆盖层 + `utils/treeFilter.js`，09-27-tree-dir-name-filter）落地后存在动线断裂：

1. **动线长**：命中文件仅支持点击预览（`onFilterResultClick` emit select），重命名/删除等操作须退出筛选回树右键，深层目录动线长。
2. **硬缺口**：截断层 2000 名外文件在树中无节点，回树也无从右键——该类文件「可预览、不可操作」。
3. **入口留空**：覆盖层命中行右键 `@contextmenu.prevent.stop`（FileTreePanel.vue:42）仅阻断冒泡（防弹错语义的空白区菜单），未提供任何菜单入口。

## 2. 方案决策

**采用方案 A：复用现有 contextMenu 全量菜单**。

理由：
- 命中项 data 来自 `GetFileTree` 原始节点（path/name/type/isGitRepo 等），与树节点 data 完全同构，`onMenuCommand` 现有命令直接可用；
- 复用 `onNodeContextMenu(event, data)` 主体（位置边界修正、IsDashboardPinned 查询、emit('contextmenu') 关闭兄弟菜单），零新增菜单组件；
- 方案 B（精简四项菜单）与树右键两套语义、双份维护，否决。

否决的替代路径：另起菜单组件（违反约束「禁另起菜单组件」）。

## 3. 现状链路核实结论（代码勘察）

| 事实 | 位置 | 影响 |
|---|---|---|
| `onNodeContextMenu` 已含哨兵拦截（`isTruncationHint` early return）| FileTreePanel.vue:971 | 覆盖层哨兵项右键复用即自动不弹菜单 |
| `refreshNode` 开头无条件 `exitFilterMode()` | FileTreePanel.vue:851 | 筛选语境操作后退筛选，与「保语境」需求冲突，需参数化 |
| `refreshNode` 在 target 未命中时静默 `return`，`InvalidateFileTreeCache` 不执行 | FileTreePanel.vue:870 | 2000 名外文件树中无节点 → target miss → 缓存不清 → 重跑筛选可能命中陈旧缓存 |
| 后端 `RenameFile`/`DeleteFile` 纯文件操作，不自清树缓存 | app_filetree.go:64-81 | 缓存失效完全依赖前端 |
| 缓存三层失效含 mtime 差量（删除/重命名更新父目录 mtime → 自动 miss 重扫）| service/filetree_cache.go:64 | mtime 兜底存在，但 NTFS 同 tick 边界不可靠，前端显式失效仍必须 |
| 删除预览清理链路已存在：emit('delete') → Home.onDeleteFromFileTree → clearPreview | Home.vue:278-286 | 选中态清理零新增 |

## 4. 实施内容（零后端改动，全部在 FileTreePanel.vue + 测试）

### 4.1 覆盖层命中行接入现有右键菜单

模板改为 `@contextmenu.prevent.stop="onNodeContextMenu($event, item)"`：
- `onNodeContextMenu` 第一行 preventDefault/stopPropagation，哨兵分支在其后，命中项与哨兵语义均正确；
- 菜单弹出位置边界修正、pin 状态查询等复用现有逻辑，零新增。

### 4.2 操作后筛选结果刷新（保语境）

1. `refreshNode(nodePath, options = {})` 增加可选参数：`options.keepFilter` 为 true 时跳过开头 `exitFilterMode()`；默认无参调用行为不变（树语境零回归）。
2. 新增辅助 `refreshAfterFilterOp(parentPath)`：
   - 显式 `InvalidateFileTreeCache(parentPath)`（不依赖 refreshNode 内 target 查找，补 2000 名外 target miss 不清缓存缺口；同时兜底 mtime 同 tick 边界）；
   - `refreshNode(parentPath, { keepFilter: true })`（树保持挂载，退筛选后即见新数据）；
   - `await runFilter()`（缓存已清，重拉保证命中列表新鲜，筛选语境连续）。
3. `handleRename` / `handleDeleteAt` 成功分支：`filterModeActive` 为 true 时走 `refreshAfterFilterOp(parentPath)`，否则维持现有 `refreshNode(parentPath)`。

### 4.3 明确不改的菜单项（防范围蔓延）

- **refresh（刷新）**：筛选语境下保持现状（`refreshNode` 内 exitFilterMode 退筛选 + 刷树）——「退出筛选看树真实状态」语义自然合理；
- **paste/copyTo/createFile/createDir**：刷新链路经 refreshNode 或父组件，退筛选为兜底行为，与现状一致，不回归；
- **cut/copy/copyPath/打开方式类/收藏类**：不触树不触筛选，零改动。

### 4.4 选中态清理

- 删除/重命名的文件若曾点击同步过树 currentKey：`refreshNode(keepFilter)` 刷新后节点消失/更名，高亮自动失效；2000 名外文件树中本无节点（`onFilterResultClick` 已 setCurrentKey(null)）；
- 预览清理走现有 emit('delete') → Home 链路，零新增。

### 4.5 哨兵过滤面六处不回归

onNodeClick 不选中 / onNodeContextMenu 不弹菜单（覆盖层复用后仍拦截哨兵）/ triggerRenameCurrent-triggerDeleteCurrent 过滤 / expandAll-collapseAll 天然兼容 / 覆盖层哨兵样式弱化——全部不动，仅命中行 contextmenu 绑定从空操作改为复用 onNodeContextMenu（哨兵在其内 early return，不弹菜单不变式保持）。

## 5. 约束

- 复用现有 contextMenu/onMenuCommand 机制，禁另起菜单组件；
- 哨兵过滤面六处不回归（CLAUDE.md 关键规则 §15.6）；
- 删除/重命名走现有 DeleteFile/RenameFile 绑定，零后端改动（§3 已核实缓存缺口在前端补齐即可）；
- 视觉守 frontend-visual-conventions（本任务纯逻辑接入，菜单复用现有样式，无新视觉元素）。

## 6. 验收标准

1. **功能**：截断层外文件（>2000 项 mock 场景）：筛选 → 右键 → 重命名/删除全链可走通，操作后覆盖层仍激活且命中列表正确刷新；
2. **零回归**：树视图右键行为不变（现有 E2E/组件测试护航）；树语境（未激活筛选）rename/delete 行为与现状逐字节一致；
3. **测试**：
   - 组件测试（FileTreePanel.spec.js）：覆盖层右键弹菜单且 contextMenu.data 指向命中项；筛选语境 rename/delete 走 keepFilter + runFilter 链路；非筛选语境维持现状；哨兵右键不弹菜单；
   - E2E（file-tree.spec.js）：新增覆盖层右键用例（弹菜单/重命名全链/删除全链/2000 名外删除）；现有用例全过；
   - vitest + E2E 全绿，前端覆盖率 ≥70% 门禁；
4. **流程**：trellis-check + 子 agent 审核。

## 7. Out of Scope

- 覆盖层命中目录项的「粘贴/新建/在此目录中搜索」等操作的筛选语境优化（退筛选兜底已可接受）；
- 覆盖层多选批量操作；
- 后端缓存失效策略改造（mtime 同 tick 问题由前端显式失效覆盖，无需后端改动）。
