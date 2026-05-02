# 文件树右键菜单 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为文件树节点添加右键操作菜单，支持新建文件/文件夹、重命名、删除、复制路径、在资源管理器中打开等功能。

**Architecture:** 前端使用 Element Plus `el-dropdown`（`trigger="contextmenu"`）包裹现有节点内容，按节点类型（文件/文件夹）条件渲染不同菜单项。后端新增 `OpenInExplorer` 方法调用 Windows 资源管理器。新建/重命名共用一个 `el-dialog` 对话框。

**Tech Stack:** Vue 3 (Composition API), Element Plus (el-dropdown, el-dialog, ElMessageBox), Go (os/exec), Wails

---

### Task 1: 后端 — 新增 OpenInExplorer 方法

**Files:**
- Modify: `git-manager/service/fileoperation.go` (末尾追加方法)
- Modify: `git-manager/app.go` (末尾追加绑定方法)

**Step 1: 在 service/fileoperation.go 末尾新增 OpenInExplorer 方法**

在文件末尾（`PreviewFile` 方法之后）添加：

```go
// OpenInExplorer 在资源管理器中打开
func (s *FileOperationService) OpenInExplorer(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return exec.Command("explorer", path).Start()
	}
	return exec.Command("explorer", "/select,", path).Start()
}
```

同时需要在文件顶部 import 块中添加 `"os/exec"`。

**Step 2: 在 app.go 末尾新增 Wails 绑定方法**

```go
// OpenInExplorer 在资源管理器中打开
func (a *App) OpenInExplorer(path string) bool {
	err := a.fileOpSvc.OpenInExplorer(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}
```

**Step 3: 运行后端测试确认无破坏**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && go test ./...`
Expected: PASS（无编译错误，现有测试全部通过）

**Step 4: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager
git add service/fileoperation.go app.go
git commit -m "feat: add OpenInExplorer backend method"
```

---

### Task 2: 前端 — 导入图标和 OpenInExplorer 绑定

**Files:**
- Modify: `git-manager/frontend/src/views/Home.vue` (script setup 部分)

**Step 1: 在图标 import 中添加右键菜单需要的图标**

将现有图标导入（约第225-230行）：

```javascript
import {
  Folder,
  FolderOpened,
  Document,
  SuccessFilled
} from '@element-plus/icons-vue'
```

替换为：

```javascript
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
  Monitor
} from '@element-plus/icons-vue'
```

**Step 2: 在 Wails 绑定 import 中添加 OpenInExplorer**

将现有 import（约第234-239行）：

```javascript
import {
  GetDirectories, AddDirectory,
  GetFileTree,
  CreateDirectory, CreateFile, RenameFile, DeleteFile, PreviewFile,
  PullRepo, CloneRepo
} from '../../wailsjs/go/main/App'
```

替换为：

```javascript
import {
  GetDirectories, AddDirectory,
  GetFileTree,
  CreateDirectory, CreateFile, RenameFile, DeleteFile, PreviewFile,
  PullRepo, CloneRepo,
  OpenInExplorer
} from '../../wailsjs/go/main/App'
```

**Step 3: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager
git add frontend/src/views/Home.vue
git commit -m "feat: import context menu icons and OpenInExplorer binding"
```

---

### Task 3: 前端 — 将 el-tree 节点内容包裹在 el-dropdown 中

**Files:**
- Modify: `git-manager/frontend/src/views/Home.vue` (template 部分，第49-72行)

**Step 1: 替换 el-tree 的 #default 插槽内容**

将现有的 `#default` 插槽（第49-72行）：

```html
<template #default="{ node, data }">
  <span class="custom-tree-node">
    <el-icon
      v-if="data.type === 'directory'"
      :color="node.expanded ? '#409EFF' : '#909399'"
      style="margin-right: 5px;"
    >
      <component :is="node.expanded ? FolderOpened : Folder" />
    </el-icon>
    <el-icon v-else color="#606266" style="margin-right: 5px;">
      <Document />
    </el-icon>
    <span :style="{
      color: data.type === 'directory'
        ? (node.expanded ? '#409EFF' : '#909399')
        : '#606266'
    }">
      {{ node.label }}
    </span>
    <el-icon v-if="data.isGitRepo" color="#67C23A" style="margin-left: 5px;">
      <SuccessFilled />
    </el-icon>
  </span>
</template>
```

替换为：

```html
<template #default="{ node, data }">
  <el-dropdown
    trigger="contextmenu"
    @command="handleContextMenu($event, data, node)"
    @visible-change="(visible) => !visible || onNodeClick(data)"
  >
    <span class="custom-tree-node">
      <el-icon
        v-if="data.type === 'directory'"
        :color="node.expanded ? '#409EFF' : '#909399'"
        style="margin-right: 5px;"
      >
        <component :is="node.expanded ? FolderOpened : Folder" />
      </el-icon>
      <el-icon v-else color="#606266" style="margin-right: 5px;">
        <Document />
      </el-icon>
      <span :style="{
        color: data.type === 'directory'
          ? (node.expanded ? '#409EFF' : '#909399')
          : '#606266'
      }">
        {{ node.label }}
      </span>
      <el-icon v-if="data.isGitRepo" color="#67C23A" style="margin-left: 5px;">
        <SuccessFilled />
      </el-icon>
    </span>
    <template #dropdown>
      <el-dropdown-menu>
        <!-- 文件夹专属菜单 -->
        <template v-if="data.type === 'directory'">
          <el-dropdown-item command="createFile">
            <el-icon><DocumentAdd /></el-icon>新建文件
          </el-dropdown-item>
          <el-dropdown-item command="createDir">
            <el-icon><FolderAdd /></el-icon>新建文件夹
          </el-dropdown-item>
          <el-dropdown-item divided command="rename">
            <el-icon><Edit /></el-icon>重命名
          </el-dropdown-item>
          <el-dropdown-item command="delete">
            <el-icon><Delete /></el-icon>删除
          </el-dropdown-item>
          <el-dropdown-item divided command="copyPath">
            <el-icon><CopyDocument /></el-icon>复制路径
          </el-dropdown-item>
          <el-dropdown-item command="openExplorer">
            <el-icon><Monitor /></el-icon>在资源管理器中打开
          </el-dropdown-item>
        </template>
        <!-- 文件专属菜单 -->
        <template v-else>
          <el-dropdown-item command="rename">
            <el-icon><Edit /></el-icon>重命名
          </el-dropdown-item>
          <el-dropdown-item command="delete">
            <el-icon><Delete /></el-icon>删除
          </el-dropdown-item>
          <el-dropdown-item divided command="copyPath">
            <el-icon><CopyDocument /></el-icon>复制路径
          </el-dropdown-item>
          <el-dropdown-item command="copyName">
            <el-icon><CopyDocument /></el-icon>复制文件名
          </el-dropdown-item>
          <el-dropdown-item command="openExplorer">
            <el-icon><Monitor /></el-icon>在资源管理器中打开
          </el-dropdown-item>
        </template>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>
```

**Step 2: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager
git add frontend/src/views/Home.vue
git commit -m "feat: add context menu dropdown to file tree nodes"
```

---

### Task 4: 前端 — 实现右键菜单操作的处理函数

**Files:**
- Modify: `git-manager/frontend/src/views/Home.vue` (script setup 部分)

**Step 1: 添加重命名对话框相关的响应式变量**

在 `createLoading` 变量（约第264行）之后添加：

```javascript
const renameDialogVisible = ref(false)
const renameName = ref('')
const renameLoading = ref(false)
const contextMenuTarget = ref(null)
```

**Step 2: 添加 handleContextMenu 分发函数**

在 `onLatestCommit` 函数（约第375行）之后添加：

```javascript
const handleContextMenu = (command, data, node) => {
  contextMenuTarget.value = { data, node }

  switch (command) {
    case 'createFile':
      showCreateFileDialogAt(data)
      break
    case 'createDir':
      showCreateDirectoryDialogAt(data)
      break
    case 'rename':
      showRenameDialogAt(data)
      break
    case 'delete':
      handleDeleteAt(data)
      break
    case 'copyPath':
      copyToClipboard(data.path, '路径')
      break
    case 'copyName':
      copyToClipboard(data.name, '文件名')
      break
    case 'openExplorer':
      handleOpenExplorer(data.path)
      break
  }
}
```

**Step 3: 添加各操作的具体实现函数**

在 `handleContextMenu` 之后添加：

```javascript
// ---- 右键菜单操作实现 ----

const showCreateFileDialogAt = (data) => {
  selectedNode.value = data
  createType.value = 'file'
  createName.value = ''
  createDialogVisible.value = true
}

const showCreateDirectoryDialogAt = (data) => {
  selectedNode.value = data
  createType.value = 'directory'
  createName.value = ''
  createDialogVisible.value = true
}

const showRenameDialogAt = (data) => {
  selectedNode.value = data
  renameName.value = data.name
  renameDialogVisible.value = true
}

const handleRename = async () => {
  if (!renameName.value.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  if (!selectedNode.value) return

  renameLoading.value = true
  try {
    const result = await RenameFile(selectedNode.value.path, renameName.value.trim())
    if (result) {
      ElMessage.success('重命名成功')
      renameDialogVisible.value = false
      // 刷新父节点
      const targetPath = selectedNode.value.path
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) {
        parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      }
      refreshNode(parentPath)
      selectedNode.value = null
    } else {
      ElMessage.error('重命名失败')
    }
  } catch (error) {
    ElMessage.error('重命名失败: ' + (error.message || String(error)))
  } finally {
    renameLoading.value = false
  }
}

const handleDeleteAt = async (data) => {
  selectedNode.value = data

  try {
    await ElMessageBox.confirm(
      `确定要删除 "${data.name}" 吗？此操作不可撤销。`,
      '警告',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
  } catch {
    return
  }

  const targetPath = data.path
  const result = await DeleteFile(targetPath)
  if (result) {
    ElMessage.success('删除成功')
    selectedNode.value = null
    let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
    if (!parentPath) {
      parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
    }
    refreshNode(parentPath)
  } else {
    ElMessage.error('删除失败')
  }
}

const copyToClipboard = async (text, label) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch {
    ElMessage.error('复制失败')
  }
}

const handleOpenExplorer = async (path) => {
  const result = await OpenInExplorer(path)
  if (!result) {
    ElMessage.error('打开资源管理器失败')
  }
}
```

**Step 4: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager
git add frontend/src/views/Home.vue
git commit -m "feat: implement context menu action handlers"
```

---

### Task 5: 前端 — 添加重命名对话框模板

**Files:**
- Modify: `git-manager/frontend/src/views/Home.vue` (template 部分)

**Step 1: 在现有对话框之后添加重命名对话框**

在 `</el-dialog>` （新建文件夹/文件对话框的闭合标签，约第218行）之后，`</div>` （home div 闭合标签，约第219行）之前，添加：

```html
<!-- 重命名对话框 -->
<el-dialog
  v-model="renameDialogVisible"
  title="重命名"
  width="420px"
>
  <el-form label-width="80px">
    <el-form-item label="当前名称">
      <el-input :model-value="selectedNode?.name" disabled />
    </el-form-item>
    <el-form-item label="新名称">
      <el-input
        ref="renameInputRef"
        v-model="renameName"
        placeholder="请输入新名称"
        :disabled="renameLoading"
        @keyup.enter="handleRename"
        @keydown.esc="renameDialogVisible = false"
      />
    </el-form-item>
  </el-form>
  <template #footer>
    <el-button @click="renameDialogVisible = false" :disabled="renameLoading">取消</el-button>
    <el-button type="primary" @click="handleRename" :loading="renameLoading">确定</el-button>
  </template>
</el-dialog>
```

**Step 2: 在 script 中添加 renameInputRef 并在对话框打开时自动聚焦**

在变量声明区域（约第246行附近）添加：

```javascript
const renameInputRef = ref()
```

然后在 `showRenameDialogAt` 函数中，设置 `renameDialogVisible.value = true` 之后，添加自动聚焦逻辑。将 `showRenameDialogAt` 改为：

```javascript
const showRenameDialogAt = (data) => {
  selectedNode.value = data
  renameName.value = data.name
  renameDialogVisible.value = true
  // 下一帧聚焦输入框并全选
  setTimeout(() => {
    const input = renameInputRef.value?.input
    if (input) {
      input.focus()
      input.select()
    }
  }, 100)
}
```

**Step 3: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git_tools/git-manager
git add frontend/src/views/Home.vue
git commit -m "feat: add rename dialog with auto-focus"
```

---

### Task 6: 前端 — 移除右侧面板中已由右键菜单覆盖的重复操作按钮

**Files:**
- Modify: `git-manager/frontend/src/views/Home.vue` (template 部分)

**Step 1: 保留右侧面板的重命名功能（之前是占位符），移除重复代码**

当前 `showRenameDialog` 函数（约第479-481行）只是显示"功能开发中"。由于右键菜单已实现重命名，右侧面板的重命名按钮应复用 `showRenameDialogAt`。

将 `showRenameDialog` 函数替换为：

```javascript
const showRenameDialog = () => {
  if (!selectedNode.value) return
  showRenameDialogAt(selectedNode.value)
}
```

**Step 2: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager
git add frontend/src/views/Home.vue
git commit -m "fix: wire up rename button in right panel to context menu handler"
```

---

### Task 7: 前端 — 添加右键菜单的样式微调

**Files:**
- Modify: `git-manager/frontend/src/views/Home.vue` (style 部分)

**Step 1: 在 `<style scoped>` 中添加右键菜单相关样式**

在文件末尾 `</style>` 之前添加：

```css
/* 右键菜单样式 */
.custom-tree-node {
  cursor: default;
  user-select: none;
}

:deep(.el-dropdown-menu__item) {
  padding: 5px 16px;
}

:deep(.el-dropdown-menu__item .el-icon) {
  margin-right: 6px;
}
```

**Step 2: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager
git add frontend/src/views/Home.vue
git commit -m "style: add context menu dropdown styles"
```

---

### Task 8: 前端测试 — 更新测试 stubs

**Files:**
- Modify: `git-manager/frontend/src/views/__tests__/Home.spec.js`

**Step 1: 在测试的 stubs 中添加新组件**

在两个 `beforeEach` 的 stubs 对象中添加：

```javascript
'el-dropdown': true,
'el-dropdown-menu': true,
'el-dropdown-item': true,
```

第一个 `beforeEach`（约第41行）的 stubs 对象末尾添加这三行。

第二个 `beforeEach`（约第206行）的 stubs 对象中也添加这三行。

**Step 2: 运行前端测试**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager/frontend && npm test`
Expected: 所有测试 PASS

**Step 3: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager
git add frontend/src/views/__tests__/Home.spec.js
git commit -m "test: add el-dropdown stubs to Home.vue tests"
```

---

### Task 9: 端到端验证

**Step 1: 启动开发服务器**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && wails dev`

**Step 2: 验证功能清单**

手动测试以下场景：

1. 右键点击文件夹节点 → 应显示：新建文件、新建文件夹、(分隔线)、重命名、删除、(分隔线)、复制路径、在资源管理器中打开
2. 右键点击文件节点 → 应显示：重命名、删除、(分隔线)、复制路径、复制文件名、在资源管理器中打开
3. 新建文件 → 弹出对话框 → 输入名称 → 回车确认 → 文件树刷新
4. 新建文件夹 → 同上
5. 重命名 → 弹出对话框 → 输入框预填当前名称且全选 → 输入新名称 → 回车确认
6. 删除 → 弹出二次确认框 → 确认后删除 → 文件树刷新
7. 复制路径 → 无弹窗 → 显示"路径已复制到剪贴板"提示
8. 复制文件名（仅文件节点） → 无弹窗 → 显示"文件名已复制到剪贴板"
9. 在资源管理器中打开 → Windows 资源管理器弹出

**Step 3: 最终提交（如有修复）**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager
git add -A
git commit -m "feat: complete file tree context menu feature"
```
