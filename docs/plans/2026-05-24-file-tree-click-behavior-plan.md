# 文件树点击行为优化 — 实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 文件树中点击当前选中节点的祖先目录时，仅选中不收起；其他情况保持默认 toggle 行为。

**Architecture:** 禁用 el-tree 默认的点击展开/收起行为（`expand-on-click-node="false"`），在自定义 `onNodeClick` 中根据路径前缀判断是否为祖先节点，决定是否 toggle。

**Tech Stack:** Vue 3 + Element Plus el-tree

---

### Task 1: 禁用 el-tree 默认点击展开并添加路径跟踪

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue:11-24`（el-tree 属性）
- Modify: `frontend/src/components/FileTreePanel.vue:303`（新增 ref）

**Step 1: 给 el-tree 添加 `expand-on-click-node` 和 `highlight-current` 属性**

在 `frontend/src/components/FileTreePanel.vue` 的 `<el-tree>` 标签上添加两个属性：

```html
        :expand-on-click-node="false"
        highlight-current
```

具体位置在第 18 行 `lazy` 之后、`:load="loadTreeNode"` 之前，完整上下文：

```html
      <el-tree
        v-if="selectedDirId"
        :key="treeKey"
        ref="fileTreeRef"
        :props="treeProps"
        node-key="path"
        lazy
        :expand-on-click-node="false"
        highlight-current
        :load="loadTreeNode"
        @node-click="onNodeClick"
        @node-contextmenu="onNodeContextMenu"
        @node-expand="closeContextMenu"
        @node-collapse="closeContextMenu"
        class="file-tree"
      >
```

**Step 2: 新增 `currentSelectedPath` ref**

在 `frontend/src/components/FileTreePanel.vue` 第 303 行（`const fileTreeRef = ref()` 之前），新增：

```javascript
const currentSelectedPath = ref('')
```

**Step 3: 重写 `onNodeClick` 函数**

替换 `frontend/src/components/FileTreePanel.vue` 第 400-402 行的 `onNodeClick` 函数为：

```javascript
// ---- 节点点击 ----
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

**Step 4: 启动开发服务器验证**

Run: `wails dev`

验证以下场景：
1. 展开目录 A → 展开子目录 B → 选中 B 下的文件 → 点击目录 A → 目录 A 应仅被选中，不应收起
2. 选中文件后点击兄弟目录（非祖先）→ 应正常展开
3. 点击已展开的非祖先目录 → 应正常收起
4. 点击箭头图标 → 应始终可展开/收起

**Step 5: 提交**

```bash
git add frontend/src/components/FileTreePanel.vue
git commit -m "fix: 文件树点击祖先目录时仅选中不收起

- 禁用 el-tree 默认的 expand-on-click-node 行为
- 在 onNodeClick 中自定义 toggle 逻辑
- 当点击当前选中节点的祖先目录时只选中不收起"
```
