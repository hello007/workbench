# 文件树点击展开文件夹时保持展开状态

## Goal

优化文件树交互体验：当已展开的文件夹处于未选中状态时，点击它应只执行选中操作，不应同时收起文件夹。

## What I already know

**用户描述：**
- 当前文件夹已展开但未选中
- 点击选中时，当前行为是"选中 + 收起"
- 期望行为是"仅选中，保持展开"

**代码分析（FileTreePanel.vue 第 515-534 行）：**

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
    node.collapse()  // ← 问题：不区分选中/未选中，已展开就收起
  } else {
    node.expand()
  }
}
```

**问题根因：**
- 点击时先更新 `currentSelectedPath`，再判断展开/收起
- 但判断逻辑不检查「之前是否已选中」
- 导致已展开未选中节点被点击时，先选中再收起

## Requirements

1. 已展开未选中的文件夹，点击后仅选中，保持展开状态
2. 已展开已选中的文件夹，点击后收起（保持现有行为）
3. 未展开的文件夹，点击后展开并选中（保持现有行为）

## Acceptance Criteria

- [ ] 已展开未选中的文件夹 → 点击后仅选中，保持展开
- [ ] 已展开已选中的文件夹 → 点击后收起
- [ ] 未展开的文件夹 → 点击后展开并选中
- [ ] 文件节点点击行为不变

## Definition of Done

* 代码修改完成
* 手动测试验证上述场景

## Technical Approach

在 `onNodeClick` 中，收起/展开判断前增加「是否已选中」检查：

```javascript
const wasSelected = prevPath === clickedPath  // 点击前是否已选中

if (isAncestor) return

// 新逻辑：未选中时优先选中，不收起
if (node.expanded) {
  if (wasSelected) {
    node.collapse()  // 已选中时才收起
  }
  // 未选中时只选中不收起
} else {
  node.expand()
}
```

## Technical Notes

* 文件：`frontend/src/components/FileTreePanel.vue`
* 函数：`onNodeClick`
* el-tree 配置了 `highlight-current`，可通过 `currentSelectedPath` 判断选中状态
