# 素材管理文件夹树支持收起展开（全展/全收按钮 + 双击切换）

## Goal

素材面板文件夹树当前节点级切换仅依赖左侧 caret 小三角（点击节点名=选中过滤，刻意决策），三角小且灰难发现；目录层级深时逐一点开/收起成本高。本任务为 FolderTree 组件补两类交互：树头部「全部展开/全部收起」按钮（对齐主面板 FileTreePanel 先例）+ 双击节点名切换展开/收起，单击选中过滤语义保持不变。

## What I already know

* 节点级展开/收起已实现（PR2 展开态保留）：`frontend/src/components/FolderTree.vue`
  * el-tree `:default-expanded-keys="expandedKeys"`（line 24），`@node-expand`/`@node-collapse` 维护 `expandedKeys`（line 209-216）
  * 首次加载全展开（`collectAllPaths`，line 194-207），库切换复位
  * `expand-on-click-node="false"`（line 23）——点击节点名仅选中，不切换展开
* 面板级整栏收起已实现：`AssetsPanel.vue:451-490` `folderTreeExpanded`（v-show 保 DOM），本任务不涉及
* 主面板先例：`FileTreePanel.vue:978-1020` `expandAll`（递归 `node.expand()`）与 `collapseAll`（遍历 `store.nodesMap` 置 `expanded=false`），组件自带头部按钮
* el-tree 关键行为：`default-expanded-keys` 仅对新建 Node 生效，已注册 Node 不响应其变化——直接改 `expandedKeys` 数组对当前树无效，且致树数据刷新（store 重建）后回放旧展开面（漂移 bug）
* 树数据全量内存加载（`ListAssetFolders` 一次返回多级树），无懒加载——全展可比 FileTreePanel 简化

## Requirements

* **R1 全部展开/全部收起按钮（B）**：FolderTree 组件内自带头部操作区（宿主 AssetsPanel 零改动），两按钮：
  * 全部展开：经 `treeRef.store.root.childNodes` 递归 `node.expand()`（数据全量内存无懒加载）；完成后同步 `expandedKeys = [''].concat(collectAllPaths(props.nodes))`
  * 全部收起：遍历 `store.nodesMap` 置 `expanded = false`（含根节点，收后仅剩根行可见）；同步 `expandedKeys = []`
  * **单一数据源纪律**：store API 直赋 `expanded` 不触发 node-collapse/expand 事件，须显式同步 `expandedKeys`，防刷新后展开面漂移
  * `loading`/`busy` 态禁用按钮
* **R2 双击节点名切换展开/收起（A'）**：`onNodeClick` 中经 `event.detail === 2` 判定双击，对该节点 toggle 展开（经 store Node 实例取反 `expanded`，展开态变化经既有事件链同步 `expandedKeys`）；单击选中过滤行为不变，无双击延迟惩罚（单击立即生效，双击第二次点击额外 toggle）
* **R3 视觉对齐**：头部按钮样式对齐 FileTreePanel 头部先例（el-button size=small）与 frontend-visual-conventions（蓝灰色阶、无新设计令牌）

## Acceptance Criteria

* [ ] 树头部提供「全部展开」「全部收起」按钮；全展后所有分支节点（含深层）展开，全收后仅根行「全部素材」可见
* [ ] 全展/全收后触发一次树数据刷新（局部 reload），展开面保持操作后状态，不漂移回旧展开面（expandedKeys 同步正确性）
* [ ] 双击有子节点的目录名：展开⇄收起切换；单击节点名：仅选中过滤（现状不变）
* [ ] `loading`/`busy` 态按钮禁用
* [ ] 全收后当前选中节点若在收起子树内：选中态与主区过滤保留，无报错
* [ ] FolderTree.spec.js 覆盖 R1/R2 新交互（对齐既有 el-tree 桩模式，props 已桩 expandOnClickNode/defaultExpandAll 先例 line 35）

## Definition of Done

* FolderTree.spec.js 单测覆盖新交互；`cd frontend && npm test` 全绿（前端 ≥70% 门禁）
* Lint / vitest / go test 全绿
* docs/功能说明.md（素材管理文件夹树节）与 docs/spec/assets-management.md（PR2 展开态章节）同步
* README.md 若有功能清单须确认更新

## Technical Approach

FolderTree 受控组件自治扩展，宿主零改动：

1. 模板：`.folder-tree` 内顶部加头部操作区（两 text 按钮），loading/busy 禁用
2. R1 全展/全收：经 `defineExpose` 已暴露的 `treeRef` 取 store 操作 Node；两条路径都显式回写 `expandedKeys`（单一数据源）
3. R2 双击：`onNodeClick(data)` 签名扩为 `(data, event)`（el-tree node-click 回调第二参为 node、第三参为 event——按 EP 2.14 签名取 `event.detail`；具体以 EP 源码为准，实现时核验）；双击时经 node 实例 toggle
4. 头部样式新增 scoped CSS，复用既有令牌

## Decision (ADR-lite)

**Context**：感知缺口=caret 难发现 + 层级深无批量操作。候选：A 单击展开（与选中过滤冲突，弃）、C bug 排查（代码面无证据，弃）、D 状态持久化（低价值，弃）。
**Decision**：B（全展/全收按钮）+ A'（双击切换），组件内自治，用户 2026-10-29 选定。
**Consequences**：宿主零改动；store 直赋与 expandedKeys 双写需纪律同步（漂移风险点，AC 第 2 条钉死）；双击与单击并存无延迟惩罚但双击第一次点击仍触发选中（可接受——选中即过滤是幂等操作）。

## Out of Scope

* 展开/收起状态 localStorage 持久化（跨重启记忆）
* 主面板 FileTreePanel 改动
* 后端/service 任何改动
* A 单击节点名即展开（与选中过滤语义冲突）

## Technical Notes

* 已检查：FolderTree.vue 全文、AssetsPanel.vue:445-504、FileTreePanel.vue:976-1020、FolderTree.spec.js 桩模式（line 35）
* el-tree store API 先例：`FileTreePanel.collapseAll` 遍历 `store.nodesMap`、`expandAll` 递归 `store.root.childNodes`
* expandedKeys 漂移机制：EP tree-store setData 复用同 node-key 的 Node 实例（展开态保留在实例），store 重建时 default-expanded-keys 仅作用于新建 Node——PR2「刷新复原展开面」注释依赖此语义，R1 同步纪律即为此服务
* 全收含根节点：根行保留可见（收起的是其子层级），对齐 FileTreePanel 全收语义
