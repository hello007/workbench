# 收藏目录跳转自动展开 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复从收藏目录跳转时文件树不自动展开、不选中目标的问题

**Architecture:** 在 FileTreePanel 中新增树就绪 Promise 信号，locateNode 入口 await 该信号后再逐级展开；Home.vue 中去掉不可靠的 setTimeout 改为依赖 locateNode 内部等待。

**Tech Stack:** Vue 3 Composition API, Element Plus el-tree (lazy mode)

---

## 文件结构

| 文件 | 职责 | 操作 |
|------|------|------|
| `frontend/src/components/FileTreePanel.vue` | 新增 treeReady 机制、改造 locateNode、watch treeKey | 修改 |
| `frontend/src/views/Home.vue` | 修改 onPaletteSelectFavorite 去掉 setTimeout | 修改 |

---

## Task 1: 新增树就绪信号机制

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue:322-332`

- [ ] **Step 1: 在 Refs 区域后新增 treeReady 状态管理**

在 `const treeKey = computed(...)` 之后（第 326 行后）插入：

```js
// ---- 树就绪信号 ----
let treeReadyResolve = null
let treeReadyPromise = new Promise(r => { treeReadyResolve = r })

function resetTreeReady() {
  treeReadyPromise = new Promise(r => { treeReadyResolve = r })
}
```

- [ ] **Step 2: 在 loadTreeNode 根加载成功后触发 resolve**

在 `frontend/src/components/FileTreePanel.vue:412`，`resolve(processedNodes)` 之后添加：

```js
    resolve(processedNodes)
    // 根节点加载完成，标记树就绪
    if (!node || node.level === 0 || !node.data) {
      nextTick(() => treeReadyResolve?.())
    }
```

注意：需要确保 `nextTick` 已从 vue 中导入（检查现有 import）。

- [ ] **Step 3: 添加 treeKey watcher 重置信号**

在 treeReady 声明之后添加：

```js
watch(treeKey, () => {
  resetTreeReady()
})
```

- [ ] **Step 4: 验证构建**

Run: `cd frontend && npm run build`
Expected: 无编译错误

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/FileTreePanel.vue
git commit -m "feat(file-tree): add treeReady promise signal for root load completion"
```

---

## Task 2: 改造 locateNode 方法

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue:1002-1049`

- [ ] **Step 1: locateNode 入口添加 await treeReadyPromise**

将第 1002-1004 行：

```js
async function locateNode(targetPath) {
  const tree = fileTreeRef.value
  if (!tree) return
```

改为：

```js
async function locateNode(targetPath) {
  await treeReadyPromise

  const tree = fileTreeRef.value
  if (!tree) return
```

- [ ] **Step 2: 移除中间节点限制，目标目录也展开**

将第 1027 行：

```js
    if (i < segments.length - 1 && !node.expanded) {
```

改为：

```js
    if (!node.expanded && !node.isLeaf) {
```

- [ ] **Step 3: 放宽 waitForNodeLoaded 超时**

将第 1030 行：

```js
        await waitForNodeLoaded(node, 2000)
```

改为：

```js
        await waitForNodeLoaded(node, 3000)
```

- [ ] **Step 4: 验证构建**

Run: `cd frontend && npm run build`
Expected: 无编译错误

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/FileTreePanel.vue
git commit -m "fix(file-tree): locateNode awaits tree ready and expands target directory"
```

---

## Task 3: 改造跨目录跳转逻辑

**Files:**
- Modify: `frontend/src/views/Home.vue:316-327`

- [ ] **Step 1: 修改 onPaletteSelectFavorite**

将第 316-327 行：

```js
function onPaletteSelectFavorite(fav) {
  recordAccess({ path: fav.path, type: 'dir', workDir: currentDirPath.value })
  if (fav.path.startsWith(currentDirPath.value)) {
    fileTreePanelRef.value?.locateNode(fav.path)
  } else {
    const targetDir = directories.value.find(d => fav.path.startsWith(d.path))
    if (targetDir) {
      onDirectorySelect(targetDir.id)
      setTimeout(() => fileTreePanelRef.value?.locateNode(fav.path), 500)
    }
  }
}
```

改为：

```js
function onPaletteSelectFavorite(fav) {
  recordAccess({ path: fav.path, type: 'dir', workDir: currentDirPath.value })
  if (fav.path.startsWith(currentDirPath.value)) {
    fileTreePanelRef.value?.locateNode(fav.path)
  } else {
    const targetDir = directories.value.find(d => fav.path.startsWith(d.path))
    if (targetDir) {
      onDirectorySelect(targetDir.id)
      nextTick(() => fileTreePanelRef.value?.locateNode(fav.path))
    }
  }
}
```

- [ ] **Step 2: 确认 nextTick 已导入**

检查 Home.vue 的 import 中是否已有 `nextTick`。如果没有，添加到 vue 的 import 中：

```js
import { nextTick } from 'vue'
```

- [ ] **Step 3: 验证构建**

Run: `cd frontend && npm run build`
Expected: 无编译错误

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/Home.vue
git commit -m "fix(navigation): replace setTimeout with nextTick for cross-directory favorite locate"
```

---

## Task 4: 端到端验证

- [ ] **Step 1: 启动开发服务器**

Run: `wails dev`

- [ ] **Step 2: 同目录内收藏跳转测试**

1. 添加一个深层目录到收藏
2. 收起文件树
3. Ctrl+P 打开 Command Palette → 选择该收藏
4. 验证：树逐级展开至目标目录，目标目录选中高亮且展开显示内容

- [ ] **Step 3: 跨目录收藏跳转测试**

1. 切换到另一个工作目录
2. 添加该目录下深层目录到收藏
3. 切回原工作目录
4. Ctrl+P → 选择刚添加的跨目录收藏
5. 验证：自动切换到目标工作目录，树展开至目标，目标选中高亮

- [ ] **Step 4: 快速连续跳转测试**

1. 快速连续选择两个不同收藏
2. 验证：最终停留在第二个目标，无报错、无卡死

- [ ] **Step 5: 最终提交（如有调整）**

```bash
git add -A
git commit -m "fix(file-tree): finalize favorite directory auto-expand on navigation"
```
