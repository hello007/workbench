# 刷新文件节点时自动刷新其所在目录

## 目标

在文件树中，对**文件节点**触发刷新（右键菜单「刷新」或 F5 快捷键）时，自动刷新该文件**所在的目录**，使同级文件的变更（新增 / 删除 / 重命名 / 外部改动）能立即反映出来。

## 背景与现状（查代码得出）

* el-tree 配置：`lazy` + `node-key="path"` + `:load="loadTreeNode"`，文件与目录同树，靠 `data.type`（`'file'` / `'directory'`）区分；文件 `isLeaf = true`。
* `refreshNode(nodePath)`（`FileTreePanel.vue:563`）已具备成熟的目录刷新能力：`nodesMap` 命中 → `target.expand()` 重载 → `restoreExpandedPaths` 恢复子树展开；工作目录根用 `store.root` 兜底；未命中路径用 `findExpandedAncestor` 回溯。
* **问题根因**：文件节点作为叶子也存在于 `store.nodesMap`，`refreshNode(文件路径)` 命中文件节点后对叶子 `expand()` 无效 → **刷新无反应**。
* **入口1（右键菜单）**：`onMenuCommand('refresh')` → `refreshNode(data.path)`（`:811`）。但**文件右键菜单（template `:264` 的 `v-else` 块）当前没有「刷新」项**，只有目录菜单（`:194` 块）有。
* **入口2（F5）**：`Home.vue:446` → `refreshNode(selectedNode.value.path)`，选中文件时同样无效。
* **现成可复用模式**：`Home.vue:356` 已用 `lastIndexOf('\\' | '/')` 计算父路径（删除后刷新父目录）。

## 方案候选

### 方案 A（推荐）：在 `refreshNode` 内部统一转换

命中 `target` 后判断是否为文件（`target.isLeaf` 或 `target.data.type === 'file'`），是则 `target = target.parent`（el-tree 父节点，文件位于根时即 `store.root`）。

* 一处改动；右键 + F5 两个入口自动生效；语义统一（「刷新文件 = 刷新它所在目录」）。
* 其他 `refreshNode` 调用（新建 / 删除 / 拷贝后）传入的已是目录路径，不受影响。

### 方案 B：在两个调用点分别转换

在 `case 'refresh'` 与 F5 处分别判断 `type === 'file'` 并计算父路径。

* 显式；但两处重复「计算父路径」逻辑，且未来新增调用点需记得同样处理。

## 决策（ADR-lite）

* **上下文**：文件节点刷新当前无效（叶子 `expand()` 无反应），且文件右键菜单无「刷新」项；右键与 F5 两个入口都直接把文件路径传给 `refreshNode`。
* **决策**：采用方案 A —— 在 `refreshNode` 内部命中目标后判断是否为文件，是则改为刷新其 `parent`（父目录节点，文件位于根下时即 `store.root`）；同时在文件右键菜单补「刷新 F5」项（位置参照目录菜单，置于「用默认程序打开」之后、收藏项之前）。
* **影响**：一处改动覆盖全部入口；`refreshNode` 契约变为「传入任意节点路径均刷新其代表的可见层级（文件→所在目录）」；既有传入目录路径的调用（新建 / 删除 / 拷贝后）不受影响。

## 需求（演进中）

1. 文件右键菜单新增「刷新」项（带 `F5` 快捷键提示，与目录菜单一致）。
2. 文件节点触发刷新时，实际刷新其**所在目录**。
3. F5 对当前选中的文件节点同样刷新其所在目录。

## 验收标准（演进中）

* [ ] 文件节点右键菜单出现「刷新 F5」项。
* [ ] 文件节点右键「刷新」→ 其所在目录重载，同级文件增删能即时反映，且子树展开状态保留。
* [ ] 选中文件按 F5 → 同上效果。
* [ ] 刷新父目录后，原文件若仍存在则保持选中高亮。
* [ ] 目录节点刷新行为不变（回归）。
* [ ] 新建 / 删除 / 拷贝后刷新父目录的既有逻辑不受影响（回归）。
* [ ] 单元测试覆盖「文件 → 父目录」分支。

## 定义完成

* 单测通过（`FileTreePanel.spec.js` / `Home.spec.js`）。
* `npm test` 全绿。
* 若 `docs/功能说明.md` 涉及右键菜单清单，同步更新（确认后定）。
* 每次功能完成后确认是否更新 `README.md`（项目规范）。

## 范围外

* F5 在未选中任何节点时的行为（保持现状 no-op）。
* 左侧 `DirectoryTree` 的刷新（其菜单本无刷新项，不在本次范围）。
* 刷新后强制滚动 / 重新定位（依赖 el-tree 现有 `highlight-current`，不做额外处理）。

## 技术备注

* `frontend/src/components/FileTreePanel.vue`：`refreshNode` `:563`、`onMenuCommand` `:760`、菜单 template `:178-319`。
* `frontend/src/views/Home.vue`：F5 `:446`、`onRefreshNode` `:290`、父路径计算 `:356`。
* `frontend/src/components/__tests__/FileTreePanel.spec.js`：`refreshNode` 祖先回溯用例 `:718` 起（可参考其测试搭建方式）。
