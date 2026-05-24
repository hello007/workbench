# 文件树点击行为优化 — 设计文档

**日期**：2026-05-24
**状态**：已确认

## 需求

在文件树中，当当前选中的是子目录或子节点时，点击其父目录（祖先目录）仅选中该节点，不触发收起操作。

**示例**：当前选中 `D:/workspace/workspace_ai/github/PPT`，点击 `D:/workspace/workspace_ai/github` 时，不应收起 github 目录，仅选中即可。

## 方案

**方案 A：禁用默认点击展开 + 自定义 toggle 逻辑**

## 改动范围

仅 `frontend/src/components/FileTreePanel.vue`

## 详细设计

### 1. el-tree 属性变更

```html
<el-tree
  ...
  :expand-on-click-node="false"
  highlight-current
>
```

- `expand-on-click-node="false"` — 禁用点击节点内容时的默认展开/收起切换
- `highlight-current` — 让 el-tree 自动跟踪当前选中节点

### 2. 跟踪当前选中路径

```javascript
const currentSelectedPath = ref('')
```

### 3. 重写 onNodeClick

```javascript
const onNodeClick = (data, node) => {
  const clickedPath = data.path.replace(/\\/g, '/')
  const prevPath = currentSelectedPath.value.replace(/\\/g, '/')

  currentSelectedPath.value = data.path
  emit('select', data)

  if (data.isLeaf || data.type === 'file') return

  const isAncestor = prevPath.length > clickedPath.length
    && prevPath.startsWith(clickedPath + '/')

  if (isAncestor) return

  if (node.expanded) {
    node.collapse()
  } else {
    node.expand()
  }
}
```

## 行为规则

| 场景 | 行为 |
|---|---|
| 点击当前选中节点的祖先目录 | 仅选中，不收起 |
| 点击非祖先的已展开目录 | 收起 |
| 点击未展开的目录 | 展开 |
| 点击文件 | 仅选中 |
| 点击箭头图标 | 始终可展开/收起 |
