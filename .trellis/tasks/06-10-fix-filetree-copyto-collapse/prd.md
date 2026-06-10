# 修复文件树右键"拷贝到"后展开节点被收起的问题

## Goal

修复文件树右键"拷贝到"对话框点击"确定"成功后，已展开的所有目录节点被一次性收起的体验问题。同时一并修复 `refreshNode` 在传入"陌生路径"时回退到整树重建的共性陷阱，让 create / rename / delete 等其他依赖 `refreshNode` 的操作也不再有同类风险。

## What I already know

- 触发链路：右键"拷贝到" → 弹窗 → `handleCopyTo`（`frontend/src/views/Home.vue:525`）→ 调 `CopyTo` → 成功后调 `fileTreePanelRef.refreshNode(data.targetPath)`（`Home.vue:534`）。
- 根因在 `refreshNode`（`frontend/src/components/FileTreePanel.vue:502-513`）：
  - 命中 `nodesMap[nodePath]` 时调 `treeNode.expand()` 单点刷新，体验正常。
  - 未命中（路径不在已加载/已展开节点中）时执行 `refreshCounter.value++`。
  - `refreshCounter` 被 `treeKey`（`FileTreePanel.vue:356`）依赖：`treeKey = ${selectedDirId}_${refreshCounter}`。
  - `<el-tree :key="treeKey">` 一旦 key 变更，整棵树被销毁重建，**所有展开状态全部丢失**。
- "拷贝到"必中 `else` 分支：用户在弹窗里手动输入的目标路径几乎不会恰好是当前已加载到 `nodesMap` 里的节点。
- 同样的 `refreshNode(parentPath)` 在 `handleCreate`（`FileTreePanel.vue:782`）、`handleRename`（`:825`）、`handleDelete`（`:862`）里也有调用，传的是父目录路径。父目录通常已展开，命中率高，所以表现不明显，但**潜在同类陷阱存在**。
- 已有的展开状态保存/恢复能力：`useTreeState.js`（`saveState` / `restoreState` / `clearState`，localStorage 持久化，cap = 200 条）和 `FileTreePanel.vue` 内部的 `getExpandedPaths` / `restoreTreeState`。
- 已有测试：
  - `frontend/src/composables/__tests__/useTreeState.spec.js`（已覆盖 useTreeState）。
  - `frontend/src/components/__tests__/FileTreePanel.spec.js`（el-tree 是 stub，无 `store`，测不到 `refreshNode` 这类涉及 store 的逻辑）。
- `<el-tree>` 暴露的 store API：`store.nodesMap[path]` 拿节点，`store.root` 拿根，`tree.getNode(path)` 也可用。
- 路径分隔符：Windows 下后端返回 `\`，前端有的地方做了 `replaceAll('\\', '/')`，但 `nodesMap` 的 key 取决于 `<el-tree>` 内部存的 `data.path`——以后端返回为准，**回溯祖先时不能简单按 `/` 切分**。已有 `locateNode`（`FileTreePanel.vue:1084-1109`）演示了基于"根目录路径分隔符"的归一化方法，可参考。

## Assumptions (temporary)

- 修复后"拷贝到"成功的预期行为是：**已展开的祖先目录里、若目标父目录已经展开过，则刷新该父目录显示新拷贝结果；否则不动文件树**（用户下次主动展开自然会按需加载）。不需要做"自动定位并展开到目标位置"。
- create / rename / delete 的现有行为本就是刷新父目录，父目录通常已展开，**修改 `refreshNode` 兜底分支不影响它们的可见行为**，只是把"父目录恰好不在 nodesMap"这个极端情况从"整树重建"改成"静默放弃"。
- `refreshAll`（手动按"刷新"按钮）和切换工作目录这些"用户主动整树刷新"语义保留，不动。

## Requirements (evolving)

- R1 拷贝到对话框点确定并拷贝成功后，文件树**已展开的节点保持展开**，不出现整树折叠。
- R2 拷贝到成功后，若目标父目录在文件树中已经展开（即在 `nodesMap` 中），则刷新该父目录使其能看到新拷贝出的子项；若未展开，则不做任何刷新动作。
- R3 修改 `refreshNode` 的兜底逻辑：传入路径在 `nodesMap` 命中时按现有逻辑刷新；未命中时**沿父路径向上回溯**，找到第一个已展开的祖先节点刷新它；若一路回溯到根都找不到已展开祖先，则静默放弃，**不再触发 `refreshCounter++`**。
- R4 `refreshAll`（"刷新"按钮）保留 `refreshCounter++` 的整树重建语义，不变。
- R5 兼容 Windows 反斜杠和 Unix 正斜杠路径，回溯时使用与 `locateNode` 一致的分隔符策略（以选中工作目录的根路径分隔符为准）。

## Acceptance Criteria

- [ ] AC1 在 Windows 上：随便展开 3 层及以上目录 → 右键任一文件/目录 → "拷贝到..." → 输入一个**未展开过**的目标父目录 → 确定 → 拷贝成功提示出现，**展开的所有节点保持原样**。
- [ ] AC2 同上场景，但**目标父目录已经展开**：拷贝成功后该父目录下出现新拷贝的子项，其它已展开节点保持原样。
- [ ] AC3 在 create / rename / delete 父目录已展开时，行为与改动前一致（父目录刷新、其他节点保持展开）。
- [ ] AC4 在 create / rename / delete 时若父目录恰好不在 `nodesMap`（极端场景），不再触发整树重建，其他已展开节点保持。
- [ ] AC5 点击工具栏"刷新"按钮（`refreshAll`）行为不变，整树会重建（这是用户主动刷新的预期）。
- [ ] AC6 单测：新增针对 `refreshNode` 兜底回溯逻辑的单测（命中 / 未命中但有已展开祖先 / 一路无已展开祖先三种分支）。

## Definition of Done

- 通过 `frontend && npm run test` 全量前端单测；新增/修改的单测覆盖上面三条分支。
- 手测覆盖 AC1–AC5（Windows 平台）。
- `wails dev` 下走一遍：拷贝文件、拷贝目录、create / rename / delete 各自跑一次，无回归。
- 不修改 `useTreeState.js` 公共 API（不引入持久化层级的兼容包袱）。
- README / 功能说明.md 不需要更新（这是 bug 修复，不是行为变更）。

## Out of Scope

- 不实现"拷贝成功后自动定位并展开到目标位置"。
- 不改造 `<el-tree>` 的 lazy load 模型 / 不切换 tree 实现。
- 不修改 `useTreeState` 的持久化结构。
- 不动 `refreshAll`、`expandAll`、`collapseAll`、切换工作目录的整树语义。
- 不顺手处理路径分隔符在其他面板的归一化问题（如果发现也只在本任务相关代码内修，不外溢）。

## Technical Approach

**核心：把 `refreshNode` 的"兜底重建"换成"祖先回溯刷新 / 否则静默放弃"。**

`frontend/src/components/FileTreePanel.vue` 中 `refreshNode` 改造：

```js
const refreshNode = (nodePath) => {
  if (!fileTreeRef.value || !nodePath) return
  const store = fileTreeRef.value.store
  const nodesMap = store.nodesMap

  // 1) 命中：原行为
  const direct = nodesMap[nodePath]
  if (direct) {
    direct.loaded = false
    direct.loading = false
    direct.expand()
    return
  }

  // 2) 未命中：沿父路径向上回溯，找第一个已展开的祖先刷新它
  const ancestor = findExpandedAncestor(nodePath, store)
  if (ancestor) {
    ancestor.loaded = false
    ancestor.loading = false
    ancestor.expand()
    return
  }

  // 3) 一路无已展开祖先：静默放弃，不再 refreshCounter++
}
```

`findExpandedAncestor` 辅助函数：
- 复用 `locateNode` 里的分隔符归一化策略（基于当前 `selectedDirId` 对应工作目录根路径的分隔符）。
- 自下向上逐级裁剪 `nodePath` 末段，每级查 `nodesMap[parentPath]`，命中且 `node.expanded === true` 即返回。
- 裁剪到根目录之上仍未命中，返回 `null`。

**handleCopyTo 不需要改动**：上层只调 `refreshNode(targetPath)`，由 `refreshNode` 自己处理"找不到就静默放弃"。这样 create / rename / delete 也自动受益于修复。

## Decision (ADR-lite)

- **Context**：`refreshNode` 的兜底分支用 `refreshCounter++` 触发 `<el-tree>` key 变更重建整棵树，是导致"拷贝到"后展开状态丢失的根因。该兜底语义对 create / rename / delete 也有潜在风险。
- **Decision**：方案 B —— 修改 `refreshNode` 兜底分支为"沿父路径回溯到首个已展开祖先并刷新；找不到就静默放弃"，不再让"刷新陌生路径"摧毁整树状态。`refreshAll` 等用户主动整树刷新的入口保留 `refreshCounter++`。
- **Consequences**：
  - 优点：根因层修复；create / rename / delete 受益；改动集中在一处；测试可控。
  - 代价：`refreshNode` 的语义从"必定刷新"变成"找不到合适刷新位置就放弃"，调用方不能再把它当成"100% 触发某种 UI 反馈"用。当前所有调用点（拷贝到、create、rename、delete、右键 refresh）都不依赖这种保证，安全。
  - 风险：路径分隔符归一化处理有疏漏会让回溯永远命中不到。靠测试和复用 `locateNode` 的策略对冲。

## Research References

- 本次根因可在仓库内直接定位，未触发 research-first 模式，未生成外部研究文档。

## Technical Notes

- 关键文件：
  - `frontend/src/components/FileTreePanel.vue`：`refreshNode`（:502）、`treeKey`（:356）、`refreshCounter`（:355）、`getExpandedPaths`（:990）、`locateNode`（:1084）。
  - `frontend/src/views/Home.vue`：`handleCopyTo`（:525）。
  - `frontend/src/composables/useTreeState.js`：保存/恢复展开路径，**本次不修改**。
- 关键测试文件：
  - `frontend/src/components/__tests__/FileTreePanel.spec.js`：当前 `el-tree` 是 stub，缺乏 `store`，需要为 `refreshNode` 单测构造一个轻量 fake store（`{ nodesMap, root }`），并暴露 `refreshNode` 或通过 `defineExpose` 已暴露的接口调用。
  - `FileTreePanel.vue:1136-1146` 已有 `defineExpose({ refreshNode, ... })`，可直接在测试中通过组件实例 `vm.refreshNode(...)` 调用。
- 命名约定：参考 `locateNode` 的"基于工作目录根分隔符"归一化套路写 `findExpandedAncestor`。
