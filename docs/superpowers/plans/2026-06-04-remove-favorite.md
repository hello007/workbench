# 取消收藏功能 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在文件树右键菜单和 Command Palette 收藏列表中提供"取消收藏"操作入口

**Architecture:** 文件树右键菜单通过 computed 判断当前节点是否已收藏，动态显示"添加到收藏"或"取消收藏"；Command Palette 收藏项增加行内删除按钮。两处共享 `useFavorites` composable 的 `removeFavorite` 方法。

**Tech Stack:** Vue 3 Composition API, Element Plus

---

## 文件结构

| 文件 | 职责 | 操作 |
|------|------|------|
| `frontend/src/components/FileTreePanel.vue` | 右键菜单智能切换 + removeFavorite handler | 修改 |
| `frontend/src/components/CommandPalette.vue` | 收藏项行内删除按钮 | 修改 |

---

## Task 1: 文件树右键菜单 — 智能切换

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue`

- [ ] **Step 1: 解构 removeFavorite 和 favorites**

将第 320 行：

```js
const { addFavorite } = useFavorites()
```

改为：

```js
const { addFavorite, removeFavorite, favorites, loadFavorites } = useFavorites()
```

- [ ] **Step 2: 加载收藏列表**

在 `onMounted` 中（约第 1085 行）添加加载收藏：

```js
onMounted(() => {
  document.addEventListener('mousedown', onGlobalClick)
  document.addEventListener('contextmenu', onGlobalContextMenu)
  loadFavorites()
})
```

- [ ] **Step 3: 新增 isFavorited computed**

在 `const treeProps` 之后（约第 343 行后）添加：

```js
const isFavorited = computed(() => {
  const path = contextMenu.data?.path
  if (!path) return false
  return favorites.value.some(f => f.path === path)
})
```

需确认 `computed` 已在 import 中（第 275 行已有）。

- [ ] **Step 4: 导入 StarFilled 图标**

将第 277-295 行的图标 import 中添加 `StarFilled`：

```js
import {
  Folder,
  FolderOpened,
  Document,
  SuccessFilled,
  FolderAdd,
  DocumentAdd,
  Edit,
  Delete,
  CopyDocument,
  Monitor,
  Refresh,
  EditPen,
  Open,
  Promotion,
  Scissor,
  DocumentCopy,
  Star,
  StarFilled
} from '@element-plus/icons-vue'
```

- [ ] **Step 5: 修改模板 — 目录节点右键菜单（第一处，约第 219 行）**

将第 218-221 行：

```html
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('addFavorite')">
          <el-icon><Star /></el-icon>添加到收藏
        </li>
```

改为：

```html
        <li class="context-menu-divider" />
        <li v-if="isFavorited" class="context-menu-item" @click="onMenuCommand('removeFavorite')">
          <el-icon><StarFilled /></el-icon>取消收藏
        </li>
        <li v-else class="context-menu-item" @click="onMenuCommand('addFavorite')">
          <el-icon><Star /></el-icon>添加到收藏
        </li>
```

- [ ] **Step 6: 修改模板 — 文件节点右键菜单（第二处，约第 266 行）**

将第 265-268 行：

```html
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('addFavorite')">
          <el-icon><Star /></el-icon>添加到收藏
        </li>
```

改为：

```html
        <li class="context-menu-divider" />
        <li v-if="isFavorited" class="context-menu-item" @click="onMenuCommand('removeFavorite')">
          <el-icon><StarFilled /></el-icon>取消收藏
        </li>
        <li v-else class="context-menu-item" @click="onMenuCommand('addFavorite')">
          <el-icon><Star /></el-icon>添加到收藏
        </li>
```

- [ ] **Step 7: onMenuCommand 添加 case**

在 `case 'addFavorite'` 之后（约第 686 行后）添加：

```js
    case 'removeFavorite':
      handleRemoveFavorite(data)
      break
```

- [ ] **Step 8: 添加 handleRemoveFavorite 函数**

在 `handleAddFavorite` 函数之后（约第 914 行后）添加：

```js
const handleRemoveFavorite = async (node) => {
  const err = await removeFavorite(node.path)
  if (err) {
    ElMessage.warning(err)
  } else {
    ElMessage.success('已取消收藏')
  }
}
```

- [ ] **Step 9: 验证构建**

Run: `cd frontend && npm run build`
Expected: 无编译错误

- [ ] **Step 10: Commit**

```bash
git add frontend/src/components/FileTreePanel.vue
git commit -m "feat(favorites): add remove-favorite option in file tree context menu"
```

---

## Task 2: Command Palette 收藏列表 — 行内删除按钮

**Files:**
- Modify: `frontend/src/components/CommandPalette.vue`

- [ ] **Step 1: 导入 Close 图标和 RemoveFavorite**

将第 131 行：

```js
import { Search, Document, Folder, Star, Loading } from '@element-plus/icons-vue'
```

改为：

```js
import { Search, Document, Folder, Star, Loading, Close } from '@element-plus/icons-vue'
```

将第 147 行：

```js
const { favorites, loadFavorites, searchFavorites } = useFavorites()
```

改为：

```js
const { favorites, loadFavorites, searchFavorites, removeFavorite } = useFavorites()
```

- [ ] **Step 2: 添加 handleRemoveFav 函数**

在 `selectFavorite` 函数之后添加：

```js
async function handleRemoveFav(item) {
  await removeFavorite(item.path)
  favoriteResults.value = favoriteResults.value.filter(f => f.path !== item.path)
}
```

- [ ] **Step 3: 修改收藏列表模板 — 添加删除按钮**

将第 57-72 行的收藏项 div：

```html
        <div
          v-for="(item, index) in favoriteResults"
          :key="'fav-' + index"
          class="result-item"
          :class="{ 'result-item--active': getFavIndex(index) === selectedIndex }"
          @click="selectFavorite(item)"
          @mouseenter="selectedIndex = getFavIndex(index)"
        >
          <el-icon class="result-icon" color="#f59e0b">
            <Star />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ item.alias || getFileName(item.path) }}</div>
            <div class="result-path">{{ item.path }}</div>
          </div>
        </div>
```

改为：

```html
        <div
          v-for="(item, index) in favoriteResults"
          :key="'fav-' + index"
          class="result-item"
          :class="{ 'result-item--active': getFavIndex(index) === selectedIndex }"
          @click="selectFavorite(item)"
          @mouseenter="selectedIndex = getFavIndex(index)"
        >
          <el-icon class="result-icon" color="#f59e0b">
            <Star />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ item.alias || getFileName(item.path) }}</div>
            <div class="result-path">{{ item.path }}</div>
          </div>
          <el-icon class="remove-fav-btn" @click.stop="handleRemoveFav(item)">
            <Close />
          </el-icon>
        </div>
```

- [ ] **Step 4: 添加样式**

在 CommandPalette.vue 的 `<style scoped>` 部分末尾添加：

```css
.remove-fav-btn {
  opacity: 0;
  cursor: pointer;
  color: #909399;
  font-size: 14px;
  margin-left: auto;
  padding: 4px;
  border-radius: 4px;
  transition: all 0.2s;
}

.remove-fav-btn:hover {
  color: #f56c6c;
  background: rgba(245, 108, 108, 0.1);
}

.result-item:hover .remove-fav-btn {
  opacity: 1;
}
```

- [ ] **Step 5: 验证构建**

Run: `cd frontend && npm run build`
Expected: 无编译错误

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/CommandPalette.vue
git commit -m "feat(favorites): add inline remove button in command palette favorites list"
```

---

## Task 3: 端到端验证

- [ ] **Step 1: 启动开发服务器**

Run: `wails dev`

- [ ] **Step 2: 文件树右键菜单测试**

1. 右键一个未收藏目录 → 应显示"添加到收藏"
2. 点击"添加到收藏" → 成功提示
3. 再次右键同一目录 → 应显示"取消收藏"（StarFilled 图标）
4. 点击"取消收藏" → 成功提示
5. 再次右键 → 恢复显示"添加到收藏"

- [ ] **Step 3: Command Palette 删除测试**

1. 确保有收藏项存在
2. Ctrl+P 打开 Command Palette，输入 `@` 进入收藏模式
3. hover 收藏项 → 右侧出现关闭按钮
4. 点击关闭按钮 → 该项从列表中消失，不触发跳转
5. 再次打开 Command Palette → 确认该项已被永久移除
