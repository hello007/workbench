# 文件树筛选覆盖层「粘贴/新建/拷贝到」保语境刷新（rename/delete 保语境链路收尾）

## Goal

覆盖层命中行右键触发的全部文件操作（rename/delete/paste/createFile/createDir/copyTo）操作成功后统一保筛选语境：覆盖层仍激活、命中列表正确刷新，用户不被踢出筛选回树视图。收尾 09-27-filetree-filter-overlay-context-menu 审核发现 #2 与 PRD Out of Scope 留白。

## What I already know（代码勘察结论）

* 三条退筛选链路均已核实：
  * 粘贴：`FileTreePanel.vue` onMenuCommand `'paste'` emit → `Home.vue` handlePaste（:564）成功分支 :599 直调 `fileTreePanelRef.refreshNode(targetDir)`（无 keepFilter → refreshNode 内 exitFilterMode）
  * 拷贝到：FileTreePanel copyTo 对话框 emit `'copy-to'` → `Home.vue` handleCopyTo（:609）成功分支 :619 `await refreshNode(data.targetPath)`；失败路径（`startsWith('错误')` 与异常）均不刷新，无需处理
  * 新建：`FileTreePanel.vue` handleCreate（:1259）成功分支 :1277 `refreshNode(createParentData.value.path)`
* 已有保语境先例（rename/delete 组件内分流）：`handleRename` :1321、`handleDeleteAt` :1363 均为 `if (filterModeActive.value) await refreshAfterFilterOp(parentPath) else refreshNode(parentPath)`
* `refreshAfterFilterOp(parentPath)`（:923）三步链路：显式 `InvalidateFileTreeCache(parentPath)`（补 refreshNode target miss 不清缓存缺口 + mtime 同 tick 兜底）→ `refreshNode(parentPath, { keepFilter: true })`（不 await，并行）→ `await runFilter()`（重拉命中列表，GetFileTree 缓存已失效保证新鲜，产物命中关键词时出现在列表）
* `refreshNode` 内 `!options.keepFilter` 才 exitFilterMode（:854）
* defineExpose（:1784）现含 refreshNode，无分流出口方法
* 覆盖层「刷新」菜单项（onMenuCommand `'refresh'` :1184）刻意维持退筛选语义，本任务不动
* ContentPanel 也 emit paste（Home.vue :64）与 copy-to（:65 走 FileTreePanel 对话框），与 FileTreePanel paste 汇入同两个 Home.vue 处理器；筛选覆盖层属 FileTreePanel，分流以 filterModeActive 判定天然正确
* 树语境零回归护航：现有 E2E file-tree.spec.js 新建/删除用例 + refreshNode 真实路径用例（FileTreePanel.spec.js :1467）

## Requirements

* FileTreePanel 新增 `refreshAfterFileOp(parentPath)` 并 defineExpose：内部按 `filterModeActive` 分流（激活走 `refreshAfterFilterOp`，否则 `refreshNode` 现状）；父组件零语境判断，分流逻辑单点收敛
* `handleRename`/`handleDeleteAt` 既有内联分流归并为调用 `refreshAfterFileOp`（消除三处重复分流，行为等价）
* `handleCreate` 成功分支改调 `refreshAfterFileOp(createParentData.value.path)`
* `Home.vue` handlePaste 成功分支 :599、handleCopyTo 成功分支 :619 改调 `refreshAfterFileOp`（替代直调 refreshNode）；失败路径不触发刷新维持现状
* 覆盖层「刷新」项维持退筛选语义不动
* 零后端改动；复用现有刷新机制，禁另起刷新路径

## Acceptance Criteria

* [ ] 筛选态下覆盖层右键粘贴/新建文件/新建文件夹/拷贝到：成功后覆盖层仍激活，命中列表正确刷新（产物命中关键词时出现），不被踢出筛选
* [ ] 树视图同操作行为零回归（E2E file-tree.spec.js 现有用例全绿）
* [ ] 新增 vitest 用例：覆盖层（filterModeActive）语境 paste/create/copyTo 保语境刷新 + 树语境走 refreshNode 现状
* [ ] 哨兵过滤面六处不回归
* [ ] vitest 全绿 + 前端覆盖率 ≥70% 门禁
* [ ] trellis-check 通过 + 子 agent 审核
* [ ] 文档同步：功能说明.md 筛选条目 + docs/spec/perf-baseline.md §15.6

## Definition of Done

* 测试新增/更新，lint + vitest 全绿
* 文档（功能说明.md、perf-baseline §15.6）更新
* CLAUDE.md 关键规则表核对（perf-baseline §15.6 行需补 paste/create/copyTo 收敛描述）

## Technical Approach

方案 A（收敛组件内分流出口）：

```
FileTreePanel
  refreshAfterFileOp(parentPath)   // 新增 defineExpose
    ├─ filterModeActive → refreshAfterFilterOp(parentPath)   // 现有三步链路复用
    └─ 否则           → refreshNode(parentPath)               // 现状

调用方（零语境判断）
  handleRename/handleDeleteAt（组件内，归并既有内联分流）
  handleCreate（组件内）
  Home.vue handlePaste 成功分支
  Home.vue handleCopyTo 成功分支
```

方案 B（否决）：emit payload 携带语境标记，父组件按标记分流——语境判断泄漏到父组件，emit 变更面更大，两处 Home.vue 处理器均需感知筛选态。

## Decision (ADR-lite)

**Context**: rename/delete 已有组件内分流先例，paste/create/copyTo 三链路需同语义；分流逻辑放组件内还是父组件需决策。
**Decision**: 方案 A。分流出口收敛为 defineExpose 单方法，组件内三处 + 父组件两处共五个调用点统一。
**Consequences**: Home.vue 不感知筛选态；后续新增触树文件操作入口只调 refreshAfterFileOp 即得正确分流；方法名语义为「文件操作后刷新」需注释说明与「刷新」菜单项退筛选语义的区别。

## Out of Scope

* 覆盖层「刷新」菜单项语义变更（刻意保留退筛选）
* 后端改动（GetFileTree / InvalidateFileTreeCache 签名与行为）
* ContentPanel 侧筛选能力（覆盖层仅属 FileTreePanel）
* 剪切/复制（不触树刷新，无语境问题）

## Technical Notes

* paste 目标为 2000+ 大目录时产物多落截断层外，树中不可见——保语境后 runFilter 重拉命中列表可见，正是本任务核心价值
* refreshAfterFilterOp 内 refreshNode(keepFilter) 不 await 的时序对 paste/copyTo 无影响：runFilter 已 await，命中列表刷新完成即语义完整
* resolveTargetDir 返回 '' 时 handlePaste 早退，refreshAfterFileOp 同样有 !parentPath 守卫，双保险
* 相关 spec：docs/spec/perf-baseline.md §15.6；关键规则表「文件树单层超 2000 节点截断」行
