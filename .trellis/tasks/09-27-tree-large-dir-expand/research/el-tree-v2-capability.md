# Research: el-tree-v2（Element Plus 2.13.x 虚拟化树）组件能力调研

- **Query**: el-tree-v2 懒加载支持性、大数据树替代模式、与 el-tree 的 API/DOM 差异清单、性能实测表现
- **Scope**: mixed（本地 node_modules 源码验证 + 官方文档 + GitHub issues 外部检索）
- **Date**: 2026-09-27
- **版本说明**: 项目 `frontend/package.json` 声明 `element-plus: ^2.13.7`，本地实际安装 **2.14.5**；关键结论同时用 **2.13.7 官方发布包**（npmmirror tarball）与本地 2.14.5 双重验证，两者 tree-v2 的 props/事件/方法集完全一致。

---

## 结论速览

| # | 问题 | 结论 |
|---|---|---|
| 1 | el-tree-v2 是否支持懒加载（lazy/load） | **否（No）**。props 无 `lazy` 无 `load`，官方文档 attributes 表也无此二行 |
| 2 | 不支持懒加载时的替代模式 | **「node-expand 事件异步填充 children + 替换 data 引用」可行且可靠**，展开状态按 key 保留（源码级确认） |
| 3 | API 差异 | 事件 4 个同名全存在（签名有差异）；`getNode` 存在，`loadData`/`store.nodesMap` 不存在；`.el-tree-node` class 保留但 DOM 为扁平结构（无嵌套容器） |
| 4 | 性能 | 虚拟滚动仅渲染可视区（默认 26px/行 + cache 2）；万级~十万级数据滚动/展开流畅，社区有生产验证案例；无官方公开 benchmark 数字 |

---

## 1. 决定性问题：是否支持懒加载？—— 否

### 1.1 证据一：2.13.7 官方发布包类型定义

`element-plus@2.13.7` tarball 中 `es/components/tree-v2/src/types.d.ts` 的 `TreeProps` 完整属性列表（共 20 项，原文照录）：

```text
data, emptyText, height, props, highlightCurrent, showCheckbox, defaultCheckedKeys,
checkStrictly, defaultExpandedKeys, indent, itemSize, icon, expandOnClickNode,
checkOnClickNode, checkOnClickLeaf, currentNodeKey, accordion, filterMethod,
perfMode, scrollbarAlwaysOn
```

**无 `lazy`，无 `load`。** 本地 2.14.5 的 `treeProps`（`virtual-tree.d.ts`）逐项一致，也无 lazy/load。

来源：
- 2.13.7：https://github.com/element-plus/element-plus/blob/v2.13.7/packages/components/tree-v2/src/types.ts
- 本地验证：`frontend/node_modules/element-plus/es/components/tree-v2/src/types.d.ts`、`es/components/tree-v2/src/virtual-tree.d.ts`

### 1.2 证据二：官方文档 attributes 表

官方中文文档 tree-v2 页 attributes 表逐行提取结果：`data / empty-text / props / highlight-current / expand-on-click-node / check-on-click-node / check-on-click-leaf(2.9.6) / default-expanded-keys / show-checkbox / check-strictly / default-checked-keys / current-node-key / filter-method / indent(16) / icon / item-size(2.2.33, 默认26) / scrollbar-always-on(2.10.4) / height(200)`。

**表中无 lazy、无 load 行。**（HTML 内出现的 `lazy` 均为页面侧边栏头像的 `loading="lazy"` 属性，与组件无关。）

来源：https://element-plus.org/zh-CN/component/tree-v2.html

### 1.3 对照：el-tree（v1）有 lazy/load

本地 `frontend/node_modules/element-plus/es/components/tree/src/tree.d.ts` 第 69-76 行明确存在：

```text
readonly lazy: BooleanConstructor;
readonly load: { type: PropType<LoadFunction>; ... }
```

即懒加载是 el-tree v1 独有能力，tree-v2 未实现。

---

## 2. 替代模式：「node-expand 异步填充 children 再改 data」hack 的可靠性

### 2.1 模式描述

tree-v2 无内置懒加载，社区通行做法：

1. 初始 data 中目录节点 children 置空（或省略）；
2. 监听 `@node-expand`，异步请求该节点子级；
3. 将子级写入业务数据后**替换整个 data 引用**（`data.value = newData`），或对实例调 `setData(newData)`；
4. tree-v2 内部重建扁平化列表，新 children 出现，展开状态保留。

### 2.2 源码级响应式链路分析（2.14.5 `composables/useTree.mjs`，2.13.7 同逻辑）

| 环节 | 源码事实 | 行号 |
|---|---|---|
| data 监听 | `watch(() => props.data, (data) => { setData(data) })` —— **引用级监听（非 deep）**，替换 `props.data` 引用即触发 | 200-201 |
| 全量重建 | `setData(data)` → `tree.value = createTree(data)` → traverse 深度遍历重建 `treeNodeMap/levelTreeNodeMap/treeNodes`（新 TreeNode 对象） | 180-181, 44-79 |
| **展开状态保留** | `node.expanded = expandedKeySet.value.has(value)` —— `expandedKeySet` 是独立 ref，不随重建重置；**只要 key 不变，重建后仍保持展开** | 62 |
| isLeaf 重算 | `node.isLeaf = !children || children.length === 0` —— 填充 children 后自动从叶子变为目录，展开箭头出现 | 61 |
| 扁平化重算 | `flattenTree` 是 computed，依赖 `tree.value.treeNodes` + `expandedKeySet` + `hiddenNodeKeySet`，重建后自动失效重算 | 26-40 |
| 触发重渲染 | `flattenTree` 直接绑定给内部 `FixedSizeList` 的 `data`/`total`，computed 更新即重新渲染可视区 | tree.vue 编译产物 61-62 行 |

**结论：可靠。** data 引用替换 → 重建 → 扁平化重算 → 虚拟列表重渲染，链路全部响应式自动完成；展开状态按 key 保留无需手动恢复。另有一个官方佐证：官方文档为 `setData` 方法给出的说明是「当数据量庞大时，直接使用响应式数据监听可能不太优化……提供这个显式调用的方法」——官方为大数据场景显式支持整体重设数据这一用法。

### 2.3 三个可靠性注意点

1. **必须替换顶层引用**：`watch` 非 deep，直接 push 到原 data 的 children 数组（浅 mutation）**不会触发**重建。正确做法是构造新数组赋值或调 `setData(newData)`。
2. **异步期间叶子无展开箭头**：children 未填充时 `isLeaf=true`，`el-tree-node__expand-icon` 带 `is-leaf`/`is-hidden` class 被隐藏；用户看到的是无箭头条目。如需 loading 提示，用 `default` 插槽（scope 为 `{ node }`）按 `node.isLeaf` 自定义渲染，或先插入占位子节点。
3. **filter 状态与 data 重建并存**：`hiddenNodeKeySet`（filter 隐藏集）同样是独立 ref，重建后保留；若异步填充发生在 filter 之后，需自行评估过滤口径是否需要重算。

### 2.4 社区佐证

- [issue #20724](https://github.com/element-plus/element-plus/issues/20724)（open）：tree-select（基于真实 DOM 的 el-tree v1）大数据卡顿；社区成员回复明确「tree-select 用的是 tree 不是 tree-v2，真实 DOM 多了肯定卡」，并给出把 tree-select 内部替换为 tree-v2 的方案，称「生产环境可用」。

---

## 3. el-tree-v2 与 el-tree（v1）API 差异清单

以下基于本地 2.14.5 类型定义（`tree-v2/src/virtual-tree.d.ts`、`tree-v2/src/tree.vue.d.ts`、`tree/src/tree.d.ts`、`tree/src/tree.vue.d.ts`），并与 2.13.7 发布包核对。

### 3.1 事件（问题点：node-click / node-contextmenu / node-expand / node-collapse 是否存在）

**四个事件在 tree-v2 中全部存在**，但签名与 v1 不同：

| 事件 | el-tree-v2 签名 | el-tree v1 签名 | 差异要点 |
|---|---|---|---|
| `node-click` | `(data, node: TreeNode, e: MouseEvent)` | `(data, node: Node, nodeInstance, evt)` | v2 **无第 4 参 nodeInstance**（组件实例），第 3 参直接是 MouseEvent |
| `node-contextmenu` | `(e: Event, data, node: TreeNode)` | `(e, data, node: Node, nodeInstance)` | v2 少 nodeInstance |
| `node-expand` | `(data, node: TreeNode)` | `(data, node: Node, nodeInstance)` | v2 少 nodeInstance |
| `node-collapse` | `(data, node: TreeNode)` | `(data, node: Node, nodeInstance)` | v2 少 nodeInstance |
| `node-drop` | `(data, node: TreeNode, e: DragEvent)`（2.8.3） | `(draggingNode, dropNode, dropType, evt)` | **同名不同义（大坑）**：v1 是拖拽放置完成事件；v2 是单节点拖放原生事件 |
| `check-change` | `(data, checked)` | `(data, checked, indeterminate)` | v2 无 indeterminate 参数 |
| `check` | `(data, checkedInfo: CheckedInfo)` | `(data, checkedInfo: CheckedInfo)` | 一致 |
| `current-change` | `(data, node: TreeNode)` | `(data, node: Node \| null)` | v2 的 node 非 null 类型 |
| `node-drag-start/enter/leave/over/end` | **不存在** | 存在（6 个） | v2 无拖拽排序事件系列；拖拽能力基本缺失 |

tree-v2 全部 8 个事件：`node-click / node-drop / node-contextmenu / check-change / check / current-change / node-expand / node-collapse`。

注意：v1 的 node 参数是 `Node` 类实例（含 `.parent/.childNodes/.level/.data/.expanded` 等富方法）；v2 的是纯数据接口 `TreeNode`（`{ key, level, parent, children, data, disabled, label, isLeaf, expanded }`），无方法。

### 3.2 方法

| 分类 | 明细 |
|---|---|
| **两者共有** | `filter` `getCheckedKeys` `getCheckedNodes` `getHalfCheckedKeys` `getHalfCheckedNodes` `setChecked` `setCheckedKeys` `getCurrentKey` `getCurrentNode` `setCurrentKey` `getNode` |
| **v2 独有** | `setExpandedKeys` `expandNode(node)` `collapseNode(node)` `setData(data)` `scrollTo(offset)`(2.8.0) `scrollToNode(key, strategy?)`(2.8.0) `toggleCheckbox` |
| **v1 独有** | `append` `remove` `insertBefore` `insertAfter` `updateKeyChildren` `store`（TreeStore 实例，含 `nodesMap`） |
| **loadData** | **v1 / v2 公开实例方法均无**。v1 的懒加载由 `lazy`+`load` props 驱动、内部 `Node` 模型私有加载；v2 无任何等价机制 |

要点（对应问题 3）：
- **`getNode` 存在于 v2**（2.13.7 已有），签名 `getNode(data: TreeKey | TreeNodeData) => TreeNode | undefined`，内部查 `treeNodeMap`（`useTree.mjs`）。
- **`store.nodesMap` 不存在于 v2**：v2 无 store 概念，内部对应物是 `tree.value.treeNodeMap`（Map<TreeKey, TreeNode>），**未作为实例属性暴露**，外部只能通过 `getNode` 逐 key 查询。
- v2 的 `getNode` 返回的 TreeNode 是重建后的快照对象，data 引用替换重建后旧引用失效。

### 3.3 Attributes

| 分类 | 明细 |
|---|---|
| 两者共有 | `data` `empty-text` `props` `highlight-current` `expand-on-click-node` `check-on-click-node` `check-on-click-leaf` `show-checkbox` `check-strictly` `default-checked-keys` `default-expanded-keys` `current-node-key` `indent` `icon` `accordion` |
| v1 有、v2 无 | **`lazy` `load`** `node-key`（v2 用 `props.value` 指定 key 字段替代） `default-expand-all` `render-content`（v2 用 default 插槽替代） `draggable` `allow-drag` `allow-drop` `check-descendants` `auto-expand-parent` `render-after-expand` `filter-node-method`（v2 改名 `filter-method`） |
| v2 有、v1 无 | `height`(默认 200) `item-size`(默认 26, 2.2.33) `perf-mode`(默认 true) `scrollbar-always-on`(2.10.4) `filter-method` `props.class`（自定义节点 class, 2.9.0） |

v2 的 `props`（TreeOptionProps）：`value`(key 字段, 默认 `id`) / `label` / `children` / `disabled` / `class`(2.9.0)。v1 无 `value`/`class`，key 由 `node-key` prop 指定字段名。

插槽：v2 仅 `default`（scope `{ node: TreeNode }`，注意非 v1 的 `{ node, data }` 双参）+ `empty`(2.9.0)；v1 另有 `render-content` prop。

### 3.4 DOM 结构（问题点：.el-tree-node class 是否保留、有无嵌套层级）

**`.el-tree-node` class 保留，但结构从嵌套递归改为扁平虚拟列表**（2.14.5 编译产物 `tree-v2/src/tree-node.vue_vue_type_script_setup_true_lang.mjs` 逐 class 提取）：

```text
div.el-tree                                  ← 根（useNamespace("tree").b()）
└─ div.el-tree-virtual-list                  ← FixedSizeList 容器（ns.b("virtual-list")）
   └─ [虚拟 item 容器（absolute 定位，仅可视区 + cache 2 个）]
      └─ div.el-tree-node  [role=treeitem] [data-key] [aria-expanded] [aria-disabled] [aria-checked]
         │  is 类：is-expanded / is-current / is-focusable / is-checked
         └─ div.el-tree-node__content        ← 内联 style：padding-left=(level-1)*indent；height=itemSize px
            ├─ .el-tree-node__expand-icon    ← ElIcon（is-leaf / is-hidden / expanded 状态）
            ├─ el-checkbox（show-checkbox 时）
            └─ 节点 label 内容（default 插槽或文本）
空状态：div.el-tree__empty-block > div.el-tree__empty-text（或 empty 插槽）
```

与 v1 的对照：

| 维度 | el-tree v1 | el-tree-v2 |
|---|---|---|
| 层级表达 | 嵌套 DOM：`div.el-tree-node > .el-tree-node__content + .el-tree-node__children > 子 el-tree-node…` 递归嵌套 | **扁平**：全部节点是虚拟列表的兄弟 item，**无 `.el-tree-node__children` 容器**，层级仅靠 `padding-left` 内联样式表达 |
| 展开/收起动画 | `ElCollapseTransition` 包裹 children（源码 `tree/src/tree-node.vue` 引用 CollapseTransition + `childNodeRendered` 懒渲染） | 无展开动画（虚拟列表直接增删行） |
| 存在的 DOM 数量 | 全部展开节点都真实存在 | 仅可视区 + cache（默认 2）个 |
| 可用的稳定选择器 | `.el-tree-node`、`.el-tree-node__content`、`.el-tree-node__children` | `.el-tree-node`、`.el-tree-node__content`、`.el-tree-node__expand-icon`（children 容器不存在，按 children 写的选择器必挂） |
| 自定义节点 class | `props.class` 不支持（v1 用 node class 走 render-content） | `props.class`（string / `(data, node) => class`，2.9.0） |

对依赖 DOM 嵌套层级的代码（如 `querySelector('.el-tree-node .el-tree-node')` 取子孙）或 CSS 后代选择器，迁移 v2 时必须改写。

---

## 4. 每节点渲染开销与虚拟滚动表现

### 4.1 结构性开销分析（源码依据）

- **固定行高虚拟化**：内部用 element-plus 自研 `FixedSizeList`（`virtual-list/src/components/fixed-size-list.mjs`），`getItemOffset = index * itemSize`，窗口内节点数 ≈ `height / itemSize + cache`（cache 默认 2，`useTree.d.ts`）。height 200 / itemSize 26 时**同屏仅约 8-10 个节点 DOM**。
- **每节点 DOM 开销**：`el-tree-node` div + `el-tree-node__content` div + expand-icon svg + label ≈ **4-6 个元素/节点**（有 checkbox 时 +1）。
- **一次性构建成本**：`createTree` traverse 是 O(n) 深度遍历 + 两张 Map（treeNodeMap/levelTreeNodeMap）；`TreeNode.data` 保留原始 data 引用不做深拷贝。10 万级节点首次 setData 一次性遍历成本可观（毫秒~十毫秒级，取决于节点数），之后展开/收起/滚动只重算 flattenTree 的可见窗口。
- **已知性能注意点（社区）**：
  - [PR #19101](https://github.com/element-plus/element-plus/pull/19101)：`setCheckedKeys` 大量 keys 时性能优化（官方已合并），万级勾选场景需用新版；
  - [PR #23547](https://github.com/element-plus/element-plus/pull/23547)：修复 `defaultExpandedKeys` 变更时内部 expanded 状态不同步（v2 后期修复，展开态外部驱动时注意版本）；
  - [issue #18019](https://github.com/element-plus/element-plus/issues/18019)：`updateCheckedKeys` 调用位置不当导致性能问题（勾选链路 O(子树) 更新）。
  - 注：[issue #23691](https://github.com/element-plus/element-plus/issues/23691)（filter 深层 watcher 性能回归）实为 **el-tree v1**（`store.nodesMap`）的 issue，不适用于 tree-v2。

### 4.2 社区实测口径（无官方公开 benchmark，以下为社区经验数据）

| 场景 | 社区结论 | 来源 |
|---|---|---|
| el-tree v1 大数据（select 下拉内树） | 真实 DOM 全量渲染，数据多必卡 | [issue #20724](https://github.com/element-plus/element-plus/issues/20724) |
| tree-select 内部换 tree-v2 | 自建 tree-select-v2，生产环境可用 | issue #20724 评论区（zhixiaotong） |
| tree-v2 十万级节点滚动/展开 | 官方定位即大数据量树（「当节点数量巨大时…虚拟化」），社区未见渲染卡顿报告；抱怨集中在勾选/过滤链路（已有专项优化 PR） | 官方文档 + 上述 PR/issue |

**明确标注**：未检索到 element-plus 官方或社区的精确 per-node 渲染耗时 benchmark 数字（如 ms/节点）。上表为定性结论 + 源码结构分析。若本任务需要硬数字，需在项目内自建 benchmark（虚拟树 10 万节点首帧 setData + 滚动帧率）。

---

## 5. 与本项目的关联

- 本项目 `frontend/package.json`：`"element-plus": "^2.13.7"`（实际安装 2.14.5，位于 ^2.13.7 范围内，本文结论直接适用）。
- 当前使用 el-tree（v1）的组件：`frontend/src/components/FileTreePanel.vue`（文件树主面板）、`frontend/src/views/Home.vue`、`frontend/src/components/AiFunctionRunner.vue`。
- 若文件树大目录场景迁移 tree-v2：懒加载必须按第 2 节 hack 自行实现；依赖 `node-key`、`default-expand-all`、`node-drop`（拖拽语义）、`check-change` 第三参、DOM 嵌套选择器的现有用法需按第 3 节清单逐项核对。

## Caveats / Not Found

- 未找到 GitHub 上官方对「tree-v2 加 lazy 支持」feature request 的直接表态 issue（`tree-v2 lazy` 全仓搜索 16 条均不直接相关）；「不支持」结论以 2.13.7/2.14.5 两版类型定义 + 官方文档 attributes 表为准，属源码级铁证。
- 无官方/社区公开的精确渲染性能 benchmark 数字（ms 级），第 4 节为定性结论。
- `GitHub raw` 直连失败（网络），2.13.7 源码经 npmmirror 官方 tarball 验证，等效。
- `expandedKeySet`/`hiddenNodeKeySet` 为内部实现细节（未在文档承诺），跨版本升级需留意行为变化。

## 参考来源

- 官方文档（zh-CN）：https://element-plus.org/zh-CN/component/tree-v2.html
- 官方文档（en-US）：https://element-plus.org/en-US/component/tree-v2.html
- el-tree v1 文档：https://element-plus.org/zh-CN/component/tree.html
- 2.13.7 源码（tree-v2 types）：https://github.com/element-plus/element-plus/blob/v2.13.7/packages/components/tree-v2/src/types.ts
- 本地源码验证：`frontend/node_modules/element-plus/es/components/tree-v2/`（types.d.ts、virtual-tree.d.ts、composables/useTree.mjs、tree-node 编译产物）、`es/components/tree/src/tree.d.ts`
- GitHub issues/PRs：#20724、#19101、#23547、#18019、#23691（仅 v1 对照）
