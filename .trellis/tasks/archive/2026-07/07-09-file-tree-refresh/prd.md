# 文件树自动刷新与拷贝后保持展开

## Goal

解决文件树的两个体验问题：
1. 文件删除/新建后文件树不及时更新（应用内右键菜单 + 操作面板"删除"按钮均存在），需手动点"刷新"。
2. "拷贝到"目标文件夹后，该目标文件夹在文件树中自动收起，需手动再展开。

目标是让文件树在应用内文件操作后**准确、无感**地反映最新状态，并**保留用户的展开位置**；拷贝到完成后**自动展开目标**并刷新内容。

## Requirements

- 应用内删除/新建/重命名/拷贝/粘贴后，文件树及时刷新，**含工作目录根下直接子项的操作**。
- 刷新时**保留被刷新节点子树的展开状态**（不收起其他已展开的文件夹）。
- "拷贝到"完成后，**自动展开目标文件夹**并刷新内容，新拷贝的项立即可见。
- F5 刷新当前节点同样保留子树展开状态。
- 路径分隔符健壮（Windows `\` / `/` 混用不导致刷新失效或退化）。

## Acceptance Criteria

- [ ] 在工作目录根下右键新建文件/文件夹，新项立即出现，其他展开状态不变。
- [ ] 在工作目录根下右键删除文件/文件夹，该项立即消失，其他展开状态不变。
- [ ] 操作面板"删除"按钮在根目录下操作同样生效。
- [ ] 深层目录下新建/删除，目标父节点刷新，且其子树已展开的文件夹保持展开。
- [ ] "拷贝到"完成后，目标文件夹（在文件树中可见时）自动展开并刷新内容，新拷贝项可见，其他展开状态不变。
- [ ] 粘贴 / F5 刷新同样保留子树展开状态。
- [ ] 重命名后父节点刷新，展开状态保留。
- [ ] 现有 `locateNode`、切换工作目录的 `saveCurrentState/restoreTreeState` 行为不退化。

## Definition of Done

- 组件测试补充：`FileTreePanel.spec.js`（refreshNode 行为）、`Home.spec.js`（拷贝到/删除链路）。
- 前端测试 `npm test` 绿。
- 不破坏现有展开状态持久化、`locateNode` 等逻辑。
- README / `docs/功能说明.md` 按需更新（每次功能完成后确认）。

## Technical Approach

核心：将 `FileTreePanel.vue` 的 `refreshNode(nodePath)` 增强为 `refreshNode(nodePath, options)`，实现"保留子树展开状态的局部刷新"，并让所有调用点（删除/新建/重命名/粘贴/F5/拷贝）统一受益。

1. **路径规范化**：入口统一 nodePath 与 el-tree `nodesMap` key 的分隔符（复用 `findExpandedAncestor` 的规范化思路），避免因分隔符不匹配退化到祖先刷新。
2. **目标节点定位**：优先 `store.nodesMap[nodePath]`；若 nodePath 为工作目录根路径，定位 `store.root`（level 0 虚拟节点）；仍未命中再退化到 `findExpandedAncestor` 兜底。
3. **保留子树展开**：刷新前记录目标节点子树的 `expandedPaths`；`node.loaded=false; node.expand()` 触发 `loadData` 重建；`await waitForNodeLoaded(node)` 后，按记录的 paths 逐层恢复展开（复用 `restoreTreeState` 的 depth 分组逻辑）。
4. **拷贝到自动展开**：`Home.handleCopyTo` 成功后调用 `refreshNode(targetPath)`。路径规范化使命中 target 自身（而非退化到祖先），`expand()` 对已展开 target 触发 loadData 重建并保持展开，对未展开但可见的 target（在 nodesMap 中）展开并加载--两种情况均自动展开目标并刷新内容。
5. **统一调用点**：删除/新建/重命名/粘贴/F5/拷贝 全部走增强版 refreshNode，自动继承"保留子树展开"。

### 关键风险（实现时验证）
- el-tree 根节点 `store.root` 的 `loadData/expand` 是否触发 `loadTreeNode(level=0)` 重新加载，需实测；若不触发，根目录操作退化用 `refreshAll + restoreTreeState` 兜底。
- `loadData` 异步恢复展开的时序，依赖 `waitForNodeLoaded` 等待子节点重建完成。
- 后端 `GetFileTree` 返回的 path 分隔符需确认（看 `service` 代码），以对齐规范化逻辑。

## Decision (ADR-lite)

**Context**：`refreshNode` 对工作目录根路径失效（`nodesMap` 无根、`findExpandedAncestor` 对根返回 null），且 `loadData` 重建 `childNodes` 会丢失子树展开状态。两者共同导致问题 1（根目录下操作不刷新）与问题 2（拷贝后目标收起）。用户确认仅应用内场景，无需文件系统监听；拷贝后期望"自动展开目标并刷新"。

**Decision**：重写 `refreshNode` 为"保留子树展开的局部刷新"，支持根节点 + 路径分隔符健壮化；`expand()` 命中目标后自动展开并加载最新子节点，覆盖拷贝到的"自动展开目标"需求。**不引入 fsnotify 文件系统监听，不新增 expand 参数**（命中后 `expand()` 已自动展开，参数冗余）。

**Consequences**：一次性解决所有应用内刷新场景（删除/新建/重命名/粘贴/F5/拷贝）；不增加后端监听复杂度与性能开销；需谨慎处理 el-tree 内部状态时序；若未来需感知外部程序修改，再单独引入 fsnotify（已识别为未来扩展点）。

## Out of Scope

- 外部程序（资源管理器/VSCode 等）修改文件的自动监听（fsnotify）-- 用户确认不需要。
- 拷贝后定位/高亮具体新拷贝项（批量拷贝/非整目录拷贝时定位困难，用户未选）。
- 跨工作目录拷贝时目标树的刷新（targetPath 不在当前工作目录树内时，保持现状不处理）。

## What I already know（调研记录）

### 文件树架构
- 组件：`frontend/src/components/FileTreePanel.vue`，Element Plus `el-tree`（v2.13.7），`lazy` 懒加载，`node-key="path"`，`:load="loadTreeNode"`，未声明 `:default-expanded-keys`（展开靠手动 restore）。
- 刷新两种方式：
  - `refreshAll()`：`refreshCounter++` -> `treeKey` 变 -> el-tree 整树销毁重建，展开状态全部丢失，靠 `restoreTreeState` 从 localStorage 恢复。
  - `refreshNode(nodePath)`：局部刷新，设 `loaded=false` + `expand()`。
- 展开状态持久化：`useTreeState.js`，按工作目录路径存 localStorage；仅切换工作目录时 `saveCurrentState` -> `restoreTreeState`。

### el-tree 源码结论（已确认）
源码：`node_modules/element-plus/lib/components/tree/src/model/node.js`

- `expand()`（215-236 行）对已展开节点**无 early-return**：`loaded=false` 时 `shouldLoadData()` 为 true -> 进入 `loadData`。
- `loadData` 的 resolve 回调（333-343 行）会 `this.childNodes = []`（清空）-> `doCreateChildren` 重建。
- **后果 1**：`refreshNode(X)` 重建 X 的子节点列表，**丢失 X 子树已展开节点的展开状态**（X 自身 `expanded` 保持 true）。
- **后果 2**：X 不在 `nodesMap`（分隔符不匹配/未加载）时，退化到 `findExpandedAncestor` 刷新祖先 -> 祖先 `childNodes` 重建 -> **X 本身被重建为收起**。
- **后果 3**：X 为工作目录根路径时，`nodesMap` 无根、`findExpandedAncestor` 对根返回 null -> **刷新不执行**。

### 操作链路
- 新建：`FileTreePanel.handleCreate` -> `refreshNode(createParentData.path)`。
- 删除（右键）：`FileTreePanel.handleDeleteAt` -> `refreshNode(parentPath)`。
- 删除（操作面板）：`ContentPanel` `$emit('delete')` -> `Home.handleDelete`（340-368 行）-> `refreshNode(parentPath)`。
- 重命名：`FileTreePanel.handleRename` -> `refreshNode(parentPath)`。
- 粘贴：`Home` 粘贴逻辑（600-609 行）-> `refreshNode(targetDir)`。
- 拷贝到：`FileTreePanel.handleCopyTo` `emit('copyTo')` -> `Home.handleCopyTo`（619-635 行）-> `CopyTo(...)` -> `refreshNode(data.targetPath)`。`data.targetPath` 来自对话框手输，分隔符可能与 `nodesMap` key 不一致。
- F5：`Home.handleGlobalKeydown`（446-452 行）-> `refreshNode(selectedNode.path)`。

## Technical Notes

- el-tree 源码：`node_modules/element-plus/lib/components/tree/src/model/node.js`（expand 215-236、loadData 333-347、shouldLoadData 246-248）。
- 关键文件：
  - `frontend/src/components/FileTreePanel.vue`：`refreshNode`（556-575）、`findExpandedAncestor`（533-554）、`getExpandedPaths`（1105-1120）、`restoreTreeState`（1131-1168）、`waitForNodeLoaded`（1185-1198）。
  - `frontend/src/views/Home.vue`：`handleCopyTo`（619-635）、`handleDelete`（340-368）、粘贴（594-616）、F5（446-452）。
  - `frontend/src/composables/useTreeState.js`：展开状态持久化。
- 后端：`service/fileoperation.go`（`CopyTo` 516、`CopyItem` 468、`Delete` 73）；`app.go`（`GetFileTree` 绑定）。
