# DirectoryTree 右键菜单新增打开操作 - 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在左侧工作目录树（DirectoryTree.vue）右键菜单中新增"在资源管理器中打开"、"用 VSCode 打开"、"用 Warp 终端打开"三个操作。

**Architecture:** 仅修改前端组件 DirectoryTree.vue，复用已有的后端方法（OpenInExplorer/OpenInVSCode/OpenInWarp），在现有右键菜单模板中插入新菜单项，调用模式与 FileTreePanel.vue 保持一致。

**Tech Stack:** Vue 3 Composition API、Element Plus Icons、Wails Go Binding

---

### Task 1: 添加 import 声明

**Files:**
- Modify: `frontend/src/components/DirectoryTree.vue:116` (图标 import)
- Modify: `frontend/src/components/DirectoryTree.vue:118-124` (后端方法 import)

**Step 1: 添加 Element Plus 图标 import**

在 `DirectoryTree.vue` 第 116 行，将：

```javascript
import { Folder, Star, Plus, Edit, Delete } from '@element-plus/icons-vue'
```

替换为：

```javascript
import { Folder, Star, Plus, Edit, Delete, FolderOpened, Monitor, EditPen, Promotion } from '@element-plus/icons-vue'
```

**Step 2: 添加后端方法 import**

在第 118-124 行，将：

```javascript
import {
  AddDirectory,
  UpdateDirectory,
  DeleteDirectory,
  SetDefaultDirectory,
  ReorderDirectories
} from '../../wailsjs/go/main/App'
```

替换为：

```javascript
import {
  AddDirectory,
  UpdateDirectory,
  DeleteDirectory,
  SetDefaultDirectory,
  ReorderDirectories,
  OpenInExplorer,
  OpenInVSCode,
  OpenInWarp
} from '../../wailsjs/go/main/App'
```

**Step 3: 验证修改**

运行: `cd frontend && npm run build`
预期: 构建成功（新 import 暂未使用，Vite 不报错）

---

### Task 2: 添加菜单项模板

**Files:**
- Modify: `frontend/src/components/DirectoryTree.vue:52-68` (右键菜单 `<ul>` 模板)

**Step 1: 在"设为默认"和"删除"之间插入打开操作菜单项**

将第 57-68 行：

```html
      <li class="context-menu-item" @click="onMenuCommand('rename')">
        <el-icon><Edit /></el-icon>重命名
      </li>
      <li class="context-menu-item" @click="onMenuCommand('setDefault')">
        <el-icon><Star /></el-icon>设为默认
      </li>
      <li class="context-menu-divider" />
      <li class="context-menu-item" @click="onMenuCommand('delete')">
        <el-icon><Delete /></el-icon>删除
      </li>
```

替换为：

```html
      <li class="context-menu-item" @click="onMenuCommand('rename')">
        <el-icon><Edit /></el-icon>重命名
      </li>
      <li class="context-menu-item" @click="onMenuCommand('setDefault')">
        <el-icon><Star /></el-icon>设为默认
      </li>
      <li class="context-menu-divider" />
      <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
        <el-icon><Monitor /></el-icon>在资源管理器中打开
      </li>
      <li class="context-menu-item" @click="onMenuCommand('openVSCode')">
        <el-icon><EditPen /></el-icon>用 VSCode 打开
      </li>
      <li class="context-menu-item" @click="onMenuCommand('openWarp')">
        <el-icon><Promotion /></el-icon>用 Warp 打开
      </li>
      <li class="context-menu-divider" />
      <li class="context-menu-item" @click="onMenuCommand('delete')">
        <el-icon><Delete /></el-icon>删除
      </li>
```

图标选择说明（与 FileTreePanel.vue 保持一致）：
- Monitor → 资源管理器
- EditPen → VSCode
- Promotion → Warp

**Step 2: 验证构建**

运行: `cd frontend && npm run build`
预期: 构建成功

---

### Task 3: 添加菜单命令处理逻辑

**Files:**
- Modify: `frontend/src/components/DirectoryTree.vue:231-247` (`onMenuCommand` 函数)

**Step 1: 扩展 onMenuCommand 的 switch case**

在 `onMenuCommand` 函数中，将：

```javascript
const onMenuCommand = (command) => {
  const dir = contextMenu.targetDir
  closeContextMenu()
  if (!dir) return

  switch (command) {
    case 'rename':
      showRenameDialog(dir)
      break
    case 'setDefault':
      handleSetDefault(dir)
      break
    case 'delete':
      handleDelete(dir)
      break
  }
}
```

替换为：

```javascript
const onMenuCommand = (command) => {
  const dir = contextMenu.targetDir
  closeContextMenu()
  if (!dir) return

  switch (command) {
    case 'rename':
      showRenameDialog(dir)
      break
    case 'setDefault':
      handleSetDefault(dir)
      break
    case 'openExplorer':
      handleOpenExplorer(dir.path)
      break
    case 'openVSCode':
      handleOpenVSCode(dir.path)
      break
    case 'openWarp':
      handleOpenWarp(dir.path)
      break
    case 'delete':
      handleDelete(dir)
      break
  }
}
```

**Step 2: 在 `onDragEnd` 函数之前（约第 404 行）添加三个处理函数**

在 `handleDelete` 函数（第 376-402 行）之后、`onDragEnd` 函数（第 405 行）之前，插入：

```javascript
// --- 打开操作 ---
const handleOpenExplorer = async (path) => {
  try {
    const result = await OpenInExplorer(path)
    if (!result) {
      ElMessage.error('打开资源管理器失败')
    }
  } catch (error) {
    ElMessage.error('打开资源管理器失败: ' + (error.message || String(error)))
  }
}

const handleOpenVSCode = async (path) => {
  try {
    const result = await OpenInVSCode(path)
    if (!result) {
      ElMessage.error('打开 VSCode 失败，请确认已安装 VSCode 并将 code 命令加入 PATH')
    }
  } catch (error) {
    ElMessage.error('打开 VSCode 失败: ' + (error.message || String(error)))
  }
}

const handleOpenWarp = async (path) => {
  try {
    const result = await OpenInWarp(path)
    if (!result) {
      ElMessage.error('打开 Warp 失败，请确认已安装 Warp 终端')
    }
  } catch (error) {
    ElMessage.error('打开 Warp 失败: ' + (error.message || String(error)))
  }
}
```

这些函数的错误提示文案与 FileTreePanel.vue 中保持完全一致。

**Step 3: 验证构建**

运行: `cd frontend && npm run build`
预期: 构建成功

---

### Task 4: 手动测试验证

**Step 1: 启动开发服务器**

运行: `wails dev`

**Step 2: 测试右键菜单功能**

测试用例：

| # | 操作 | 预期结果 |
|---|------|---------|
| 1 | 右键点击工作目录项 | 弹出菜单包含 7 个菜单项和 2 条分隔线 |
| 2 | 菜单结构 | 顺序为：重命名、设为默认、---、资源管理器、VSCode、Warp、---、删除 |
| 3 | 点击"在资源管理器中打开" | Windows 资源管理器打开对应目录路径 |
| 4 | 点击"用 VSCode 打开" | VSCode 打开对应目录作为工作区 |
| 5 | 点击"用 Warp 打开" | Warp 终端定位到对应目录 |
| 6 | 原有功能（重命名、设为默认、删除） | 功能不受影响 |

**Step 3: 提交**

```bash
git add frontend/src/components/DirectoryTree.vue
git commit -m "feat: 工作目录树右键菜单新增资源管理器/VSCode/Warp打开操作"
```
