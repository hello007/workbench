# 收藏目录跳转自动展开修复设计

## 1. 问题描述

从收藏目录跳转时，文件树不会自动展开路径也不会选中目标目录。两种场景均失败：
- 同工作目录内跳转
- 跨工作目录跳转

## 2. 根因分析

| # | 问题 | 位置 |
|---|------|------|
| 1 | lazy tree 根节点子节点未加载时 `tree.getNode()` 返回 null，循环立即 break | FileTreePanel.vue:1024 |
| 2 | 跨目录切换使用 `setTimeout(500ms)` 猜测时机，树可能尚未就绪 | Home.vue:324 |
| 3 | `locateNode` 仅展开中间节点，目标目录本身不展开 | FileTreePanel.vue:1027 条件 `i < segments.length - 1` |

## 3. 设计方案

### 3.1 树就绪信号机制（FileTreePanel.vue）

新增 `treeReadyPromise` 管理树根加载状态：

- 维护 `treeReadyPromise` 和对应的 `treeReadyResolve`
- `treeKey` 变化时重置 Promise（`resetTreeReady()`）
- `loadTreeNode` 中根节点（level 0）加载成功后，`nextTick` 触发 resolve
- `locateNode` 入口先 `await treeReadyPromise`

### 3.2 locateNode 改造（FileTreePanel.vue）

```js
async function locateNode(targetPath) {
  await treeReadyPromise

  const tree = fileTreeRef.value
  if (!tree) return

  // ... 路径归一化逻辑不变 ...

  for (let i = 0; i < segments.length; i++) {
    currentPath += sep + segments[i]
    const node = tree.getNode(currentPath)
    if (!node) break

    // 移除 i < segments.length - 1 限制，目标目录也展开
    if (!node.expanded) {
      node.expand()
      if (!node.loaded) {
        await waitForNodeLoaded(node, 3000)
      }
      await nextTick()
    }
  }

  // 选中 + 滚动逻辑不变
}
```

变更点：
- 入口 await treeReadyPromise，保证根子节点已注册
- 目标目录也执行 expand 展示内容
- 超时从 2000ms 放宽至 3000ms

### 3.3 跨目录跳转改造（Home.vue）

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

去掉 `setTimeout(500)`，改为 `nextTick` 后直接调用。`locateNode` 内部等待树就绪。

### 3.4 treeKey watch 重置

```js
watch(() => treeKey.value, () => {
  resetTreeReady()
})
```

## 4. 影响范围

| 文件 | 改动内容 |
|------|----------|
| `frontend/src/components/FileTreePanel.vue` | 新增 treeReady 机制、改造 locateNode、watch treeKey |
| `frontend/src/views/Home.vue` | `onPaletteSelectFavorite` 去掉 setTimeout |

## 5. 测试要点

- 同目录内：从 Command Palette 选择收藏目录 → 树展开至目标并选中
- 跨目录：选择另一工作目录下的收藏 → 切换目录后树展开至目标并选中
- 深层路径（4+ 层嵌套）→ 逐级展开正常
- 快速连续跳转 → 不产生竞态或残留状态
