# 三列布局重构实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将工作目录下拉框改为左侧树型展示，整体布局重构为三列（工作目录树 200px | 文件树 280px | 操作面板自适应）。

**Architecture:** 将 Home.vue 拆分为 DirectoryTree.vue、FileTreePanel.vue、ContentPanel.vue 三个子组件，Home.vue 精简为布局容器+状态中枢。采用 props down / emit up 模式通信，通过 expose 暴露刷新方法。

**Tech Stack:** Vue 3 Composition API + Element Plus + Wails v2

**重要发现：** 后端 `app.go` 已存在 `DeleteDirectory`、`UpdateDirectory`（可用于重命名）、`SetDefaultDirectory` 方法，无需新增后端代码。

---

### Task 1: 创建 DirectoryTree.vue

**Files:**
- Create: `frontend/src/components/DirectoryTree.vue`

**Step 1: 创建组件文件**

```vue
<template>
  <div class="directory-tree-panel">
    <!-- 工具栏 -->
    <div class="dir-toolbar">
      <span class="dir-toolbar-title">工作目录</span>
      <el-button
        :icon="Plus"
        circle
        size="small"
        @click="showAddDialog"
      />
    </div>

    <!-- 目录列表 -->
    <div class="dir-list">
      <div
        v-for="dir in directories"
        :key="dir.id"
        class="dir-item"
        :class="{ 'is-active': dir.id === selectedId }"
        @click="emit('select', dir.id)"
        @contextmenu.prevent="onContextMenu($event, dir)"
      >
        <el-icon :color="dir.id === selectedId ? '#409EFF' : '#909399'" style="margin-right: 8px;">
          <Folder />
        </el-icon>
        <span class="dir-name" :title="dir.name">{{ dir.name }}</span>
        <el-icon v-if="dir.isDefault" color="#E6A23C" style="margin-left: auto;">
          <Star />
        </el-icon>
      </div>
      <el-empty v-if="directories.length === 0" description="暂无工作目录" :image-size="60" />
    </div>

    <!-- 右键菜单 -->
    <ul
      v-if="contextMenu.visible"
      class="context-menu"
      :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }"
      @click.stop
    >
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
    </ul>

    <!-- 添加目录对话框 -->
    <el-dialog v-model="addDialogVisible" title="添加工作目录" width="500px">
      <el-form :model="newDir" label-width="100px">
        <el-form-item label="目录名称">
          <el-input v-model="newDir.name" placeholder="例如: 我的工作空间" />
        </el-form-item>
        <el-form-item label="目录路径">
          <el-input v-model="newDir.path" placeholder="例如: C:\workspace" />
        </el-form-item>
        <el-form-item label="设为默认">
          <el-switch v-model="newDir.isDefault" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleAdd">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重命名对话框 -->
    <el-dialog v-model="renameDialogVisible" title="重命名工作目录" width="420px">
      <el-form label-width="80px">
        <el-form-item label="新名称">
          <el-input
            ref="renameInputRef"
            v-model="renameName"
            placeholder="请输入新名称"
            @keyup.enter="handleRename"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="renameDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleRename">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Folder, Star, Plus, Edit, Delete } from '@element-plus/icons-vue'
import { AddDirectory, UpdateDirectory, DeleteDirectory, SetDefaultDirectory } from '../../wailsjs/go/main/App'

const props = defineProps({
  directories: { type: Array, required: true },
  selectedId: { type: String, default: '' }
})

const emit = defineEmits(['select', 'change'])

// 添加目录
const addDialogVisible = ref(false)
const newDir = ref({ name: '', path: '', isDefault: false })

const showAddDialog = () => {
  newDir.value = { name: '', path: '', isDefault: false }
  addDialogVisible.value = true
}

const handleAdd = async () => {
  if (!newDir.value.name || !newDir.value.path) {
    ElMessage.error('请填写完整信息')
    return
  }
  const result = await AddDirectory(newDir.value.name, newDir.value.path, newDir.value.isDefault)
  if (result) {
    ElMessage.success('添加成功')
    addDialogVisible.value = false
    emit('change')
  } else {
    ElMessage.error('添加失败')
  }
}

// 重命名目录
const renameDialogVisible = ref(false)
const renameName = ref('')
const renameInputRef = ref()
const renameTarget = ref(null)

const handleRename = async () => {
  if (!renameName.value.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  const dir = renameTarget.value
  const result = await UpdateDirectory(dir.id, renameName.value.trim(), dir.path, dir.isDefault)
  if (result) {
    ElMessage.success('重命名成功')
    renameDialogVisible.value = false
    emit('change')
  } else {
    ElMessage.error('重命名失败')
  }
}

// 删除目录
const handleDelete = async (dir) => {
  try {
    await ElMessageBox.confirm(`确定要删除 "${dir.name}" 吗？`, '警告', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch { return }

  const result = await DeleteDirectory(dir.id)
  if (result) {
    ElMessage.success('删除成功')
    emit('change')
  } else {
    ElMessage.error('删除失败')
  }
}

// 设为默认
const handleSetDefault = async (dir) => {
  const result = await SetDefaultDirectory(dir.id)
  if (result) {
    ElMessage.success('已设为默认')
    emit('change')
  } else {
    ElMessage.error('设置失败')
  }
}

// 右键菜单
const contextMenu = reactive({ visible: false, x: 0, y: 0, data: null })

const onContextMenu = (event, dir) => {
  contextMenu.x = event.clientX
  contextMenu.y = event.clientY
  contextMenu.data = dir
  contextMenu.visible = true
}

const closeContextMenu = () => { contextMenu.visible = false }

const onMenuCommand = (command) => {
  const dir = contextMenu.data
  closeContextMenu()
  if (!dir) return

  switch (command) {
    case 'rename':
      renameTarget.value = dir
      renameName.value = dir.name
      renameDialogVisible.value = true
      setTimeout(() => {
        const input = renameInputRef.value?.input
        if (input) { input.focus(); input.select() }
      }, 100)
      break
    case 'delete':
      handleDelete(dir)
      break
    case 'setDefault':
      handleSetDefault(dir)
      break
  }
}

const onGlobalClick = () => closeContextMenu()

onMounted(() => document.addEventListener('click', onGlobalClick))
onBeforeUnmount(() => document.removeEventListener('click', onGlobalClick))
</script>

<style scoped>
.directory-tree-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background-color: #f5f7fa;
  overflow: hidden;
}
.dir-toolbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-bottom: 1px solid #ebeef5;
}
.dir-toolbar-title {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}
.dir-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 4px 0;
}
.dir-item {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  cursor: pointer;
  border-left: 3px solid transparent;
  transition: all 0.2s ease;
}
.dir-item:hover {
  background-color: #ecf5ff;
}
.dir-item.is-active {
  background-color: #e6f7ff;
  border-left-color: #409EFF;
}
.dir-name {
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.is-active .dir-name {
  color: #409EFF;
  font-weight: 500;
}
/* 右键菜单样式 */
.context-menu {
  position: fixed;
  z-index: 2000;
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  padding: 4px 0;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.12);
  min-width: 140px;
  margin: 0;
  list-style: none;
}
.context-menu-item {
  display: flex;
  align-items: center;
  padding: 5px 16px;
  font-size: 14px;
  color: #606266;
  cursor: pointer;
  white-space: nowrap;
}
.context-menu-item:hover {
  background-color: #ecf5ff;
  color: #409eff;
}
.context-menu-item .el-icon { margin-right: 6px; }
.context-menu-divider {
  height: 1px;
  background-color: #e4e7ed;
  margin: 4px 0;
}
</style>
```

**Step 2: 验证**

Run: `cd workbench && wails dev`
Expected: 编译通过，但 Home.vue 尚未引用此组件，不影响现有功能。

**Step 3: Commit**

```bash
cd workbench
git add frontend/src/components/DirectoryTree.vue
git commit -m "feat: 新增 DirectoryTree 组件（工作目录树面板）"
```

---

### Task 2: 创建 FileTreePanel.vue

**Files:**
- Create: `frontend/src/components/FileTreePanel.vue`

**Step 1: 创建组件文件**

从 Home.vue 中提取文件树相关逻辑，包括：el-tree 懒加载、自定义节点渲染、右键菜单、全部收起、新建/重命名文件对话框。

```vue
<template>
  <div class="file-tree-panel">
    <!-- 工具栏 -->
    <div class="tree-toolbar">
      <el-button-group>
        <el-button size="small" @click="collapseAll">全部收起</el-button>
      </el-button-group>
    </div>

    <!-- 文件树 -->
    <el-tree
      v-if="selectedDirId"
      :key="selectedDirId"
      ref="fileTreeRef"
      :props="treeProps"
      node-key="path"
      lazy
      :load="loadTreeNode"
      @node-click="onNodeClick"
      @node-contextmenu="onNodeContextMenu"
      class="file-tree"
    >
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
    </el-tree>
    <el-empty v-else description="请先选择工作目录" :image-size="100" />

    <!-- 右键菜单 -->
    <ul
      v-if="contextMenu.visible"
      class="context-menu"
      :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }"
      @click.stop
    >
      <template v-if="contextMenu.data?.type === 'directory'">
        <li class="context-menu-item" @click="onMenuCommand('createFile')">
          <el-icon><DocumentAdd /></el-icon>新建文件
        </li>
        <li class="context-menu-item" @click="onMenuCommand('createDir')">
          <el-icon><FolderAdd /></el-icon>新建文件夹
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('rename')">
          <el-icon><Edit /></el-icon>重命名
        </li>
        <li class="context-menu-item" @click="onMenuCommand('delete')">
          <el-icon><Delete /></el-icon>删除
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('copyPath')">
          <el-icon><CopyDocument /></el-icon>复制路径
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
          <el-icon><Monitor /></el-icon>在资源管理器中打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
          <el-icon><EditPen /></el-icon>用 VSCode 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInWarp')">
          <el-icon><Promotion /></el-icon>用 Warp 打开
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('pullRepos')">
          <el-icon><Refresh /></el-icon>更新仓库
        </li>
      </template>
      <template v-else>
        <li class="context-menu-item" @click="onMenuCommand('rename')">
          <el-icon><Edit /></el-icon>重命名
        </li>
        <li class="context-menu-item" @click="onMenuCommand('delete')">
          <el-icon><Delete /></el-icon>删除
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('copyPath')">
          <el-icon><CopyDocument /></el-icon>复制路径
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyName')">
          <el-icon><CopyDocument /></el-icon>复制文件名
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
          <el-icon><Monitor /></el-icon>在资源管理器中打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
          <el-icon><EditPen /></el-icon>用 VSCode 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInWarp')">
          <el-icon><Promotion /></el-icon>用 Warp 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openWithDefaultApp')">
          <el-icon><Open /></el-icon>用默认程序打开
        </li>
      </template>
    </ul>

    <!-- 新建文件/文件夹对话框 -->
    <el-dialog
      v-model="createDialogVisible"
      :title="createType === 'directory' ? '新建文件夹' : '新建文件'"
      width="420px"
    >
      <el-form label-width="80px">
        <el-form-item label="父文件夹">
          <el-input :model-value="createParentPath" disabled />
        </el-form-item>
        <el-form-item :label="createType === 'directory' ? '文件夹名' : '文件名'">
          <el-input
            v-model="createName"
            :placeholder="createType === 'directory' ? '例如: src' : '例如: main.go'"
            :disabled="createLoading"
            @keyup.enter="handleCreate"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialogVisible = false" :disabled="createLoading">取消</el-button>
        <el-button type="primary" @click="handleCreate" :loading="createLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重命名文件对话框 -->
    <el-dialog
      v-model="renameDialogVisible"
      title="重命名"
      width="420px"
    >
      <el-form label-width="80px">
        <el-form-item label="当前名称">
          <el-input :model-value="renameTarget?.name" disabled />
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
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Folder, FolderOpened, Document, SuccessFilled,
  FolderAdd, DocumentAdd, Edit, Delete, CopyDocument,
  Monitor, Refresh, EditPen, Open, Promotion
} from '@element-plus/icons-vue'
import { debug } from '../utils/debug'
import {
  GetFileTree,
  CreateDirectory, CreateFile, RenameFile, DeleteFile,
  OpenInExplorer, OpenInVSCode, OpenInWarp, OpenWithDefaultApp,
  ScanAndPullRepos
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const props = defineProps({
  directories: { type: Array, required: true },
  selectedDirId: { type: String, default: '' }
})

const emit = defineEmits(['select', 'batchPull'])

const fileTreeRef = ref()

const treeProps = {
  label: 'name',
  children: 'children',
  isLeaf: 'isLeaf'
}

// ---- 文件树加载 ----
const loadTreeNode = async (node, resolve) => {
  let path
  if (!node || node.level === 0 || !node.data) {
    const dir = props.directories.find(d => d.id === props.selectedDirId)
    if (!dir) { resolve([]); return }
    path = dir.path
  } else {
    path = node.data.path
  }

  try {
    const nodes = await GetFileTree(path)
    const processedNodes = (nodes || []).map(n => ({
      ...n,
      isLeaf: n.type === 'file' || !n.hasChildren
    }))
    resolve(processedNodes)
  } catch (error) {
    console.error('Error in loadTreeNode:', error)
    ElMessage.error('加载节点失败: ' + (error.message || error))
    resolve([])
  }
}

const onNodeClick = async (data) => {
  emit('select', data)
}

const collapseAll = () => {
  if (fileTreeRef.value) {
    const allNodes = fileTreeRef.value.store.nodesMap
    Object.keys(allNodes).forEach(key => {
      if (allNodes[key].expanded) allNodes[key].expanded = false
    })
  }
}

// ---- 节点刷新（expose 给父组件调用） ----
const refreshNode = (nodePath) => {
  if (!fileTreeRef.value || !nodePath) return
  const treeNode = fileTreeRef.value.store.nodesMap[nodePath]
  if (treeNode) {
    treeNode.loaded = false
    treeNode.loading = false
    treeNode.expand()
  }
}

// ---- 右键菜单 ----
const contextMenu = reactive({ visible: false, x: 0, y: 0, data: null })

const onNodeContextMenu = (event, data) => {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.x = event.clientX
  contextMenu.y = event.clientY
  contextMenu.data = data
  contextMenu.visible = true
}

const closeContextMenu = () => { contextMenu.visible = false }

// ---- 新建文件/文件夹 ----
const createDialogVisible = ref(false)
const createType = ref('directory')
const createName = ref('')
const createLoading = ref(false)
const createParentPath = ref('')

const showCreateAt = (data, type) => {
  contextMenu.data = data
  createParentPath.value = data.path
  createType.value = type
  createName.value = ''
  createDialogVisible.value = true
}

const handleCreate = async () => {
  if (!createName.value.trim()) {
    ElMessage.warning(createType.value === 'directory' ? '请输入文件夹名称' : '请输入文件名称')
    return
  }
  createLoading.value = true
  try {
    const parentPath = createParentPath.value
    let result
    if (createType.value === 'directory') {
      result = await CreateDirectory(parentPath, createName.value.trim())
    } else {
      result = await CreateFile(parentPath, createName.value.trim(), '')
    }
    if (result) {
      ElMessage.success(createType.value === 'directory' ? '文件夹创建成功' : '文件创建成功')
      createDialogVisible.value = false
      refreshNode(parentPath)
    } else {
      ElMessage.error('创建失败')
    }
  } catch (error) {
    ElMessage.error('创建失败: ' + (error.message || String(error)))
  } finally {
    createLoading.value = false
  }
}

// ---- 重命名文件 ----
const renameDialogVisible = ref(false)
const renameName = ref('')
const renameLoading = ref(false)
const renameInputRef = ref()
const renameTarget = ref(null)

const showRenameAt = (data) => {
  renameTarget.value = data
  renameName.value = data.name
  renameDialogVisible.value = true
  setTimeout(() => {
    const input = renameInputRef.value?.input
    if (input) { input.focus(); input.select() }
  }, 100)
}

const handleRename = async () => {
  if (!renameName.value.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  if (!renameTarget.value) return
  renameLoading.value = true
  try {
    const result = await RenameFile(renameTarget.value.path, renameName.value.trim())
    if (result) {
      ElMessage.success('重命名成功')
      renameDialogVisible.value = false
      const targetPath = renameTarget.value.path
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      refreshNode(parentPath)
      emit('select', null)
    } else {
      ElMessage.error('重命名失败')
    }
  } catch (error) {
    ElMessage.error('重命名失败: ' + (error.message || String(error)))
  } finally {
    renameLoading.value = false
  }
}

// ---- 删除文件 ----
const handleDeleteAt = async (data) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除 "${data.name}" 吗？此操作不可撤销。`,
      '警告',
      { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning' }
    )
  } catch { return }

  try {
    const result = await DeleteFile(data.path)
    if (result) {
      ElMessage.success('删除成功')
      const targetPath = data.path
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      refreshNode(parentPath)
      emit('select', null)
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || String(error)))
  }
}

// ---- 复制到剪贴板 ----
const copyToClipboard = async (text, label) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch { ElMessage.error('复制失败') }
}

// ---- 打开外部工具 ----
const handleOpenExplorer = async (path) => {
  try {
    const result = await OpenInExplorer(path)
    if (!result) ElMessage.error('打开资源管理器失败')
  } catch (error) { ElMessage.error('打开资源管理器失败: ' + (error.message || String(error))) }
}

const handleOpenInVSCode = async (path) => {
  try {
    const result = await OpenInVSCode(path)
    if (!result) ElMessage.error('打开 VSCode 失败')
  } catch (error) { ElMessage.error('打开 VSCode 失败: ' + (error.message || String(error))) }
}

const handleOpenInWarp = async (path) => {
  try {
    const result = await OpenInWarp(path)
    if (!result) ElMessage.error('打开 Warp 失败')
  } catch (error) { ElMessage.error('打开 Warp 失败: ' + (error.message || String(error))) }
}

const handleOpenWithDefaultApp = async (path) => {
  try {
    const result = await OpenWithDefaultApp(path)
    if (!result) ElMessage.error('打开文件失败')
  } catch (error) { ElMessage.error('打开文件失败: ' + (error.message || String(error))) }
}

// ---- 批量拉取 ----
const handleBatchPull = (data) => {
  emit('batchPull', data)
}

// ---- 菜单命令分发 ----
const onMenuCommand = (command) => {
  const data = contextMenu.data
  closeContextMenu()
  if (!data) return

  switch (command) {
    case 'createFile': showCreateAt(data, 'file'); break
    case 'createDir': showCreateAt(data, 'directory'); break
    case 'rename': showRenameAt(data); break
    case 'delete': handleDeleteAt(data); break
    case 'copyPath': copyToClipboard(data.path.replaceAll('\\', '/'), '路径'); break
    case 'copyName': copyToClipboard(data.name, '文件名'); break
    case 'openExplorer': handleOpenExplorer(data.path); break
    case 'openInVSCode': handleOpenInVSCode(data.path); break
    case 'openInWarp': handleOpenInWarp(data.path); break
    case 'openWithDefaultApp': handleOpenWithDefaultApp(data.path); break
    case 'pullRepos': handleBatchPull(data); break
  }
}

// ---- 全局事件 ----
const onGlobalClick = () => closeContextMenu()
onMounted(() => document.addEventListener('click', onGlobalClick))
onBeforeUnmount(() => document.removeEventListener('click', onGlobalClick))

defineExpose({ refreshNode, collapseAll })
</script>

<style scoped>
.file-tree-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background-color: #f5f7fa;
  overflow: hidden;
}
.tree-toolbar {
  flex-shrink: 0;
  padding: 10px;
  border-bottom: 1px solid #ebeef5;
}
.file-tree {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  background: transparent;
}
.el-tree-node__content {
  transition: background-color 0.2s ease;
  border-radius: 4px;
  margin: 2px 0;
}
.el-tree-node__content:hover {
  background-color: #F5F7FA !important;
}
.is-current > .el-tree-node__content {
  background-color: #E6F7FF !important;
  font-weight: 500;
}
.custom-tree-node {
  flex: 1;
  display: flex;
  align-items: center;
  font-size: 14px;
  cursor: default;
  user-select: none;
}
.el-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
/* 右键菜单样式 */
.context-menu {
  position: fixed;
  z-index: 2000;
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  padding: 4px 0;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.12);
  min-width: 160px;
  margin: 0;
  list-style: none;
}
.context-menu-item {
  display: flex;
  align-items: center;
  padding: 5px 16px;
  font-size: 14px;
  color: #606266;
  cursor: pointer;
  white-space: nowrap;
}
.context-menu-item:hover {
  background-color: #ecf5ff;
  color: #409eff;
}
.context-menu-item .el-icon { margin-right: 6px; }
.context-menu-divider {
  height: 1px;
  background-color: #e4e7ed;
  margin: 4px 0;
}
</style>
```

**Step 2: 验证**

Run: `cd workbench && wails dev`
Expected: 编译通过，不影响现有功能。

**Step 3: Commit**

```bash
cd workbench
git add frontend/src/components/FileTreePanel.vue
git commit -m "feat: 新增 FileTreePanel 组件（文件树面板）"
```

---

### Task 3: 创建 ContentPanel.vue

**Files:**
- Create: `frontend/src/components/ContentPanel.vue`

**Step 1: 创建组件文件**

从 Home.vue 中提取右侧操作面板，包括节点详情、Git 信息、文件操作、克隆对话框、更新进度对话框。

```vue
<template>
  <div class="content-panel">
    <div v-if="selectedNode" style="padding: 20px;">
      <h2>{{ selectedNode.name }}</h2>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="路径">{{ selectedNode.path }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ selectedNode.type === 'directory' ? '文件夹' : '文件' }}</el-descriptions-item>
      </el-descriptions>

      <!-- Git 拉取更新按钮 -->
      <div v-if="selectedNode.isGitRepo" style="margin-top: 10px;">
        <el-button type="primary" @click="pullRepo" :loading="gitLoading">
          拉取更新
        </el-button>
      </div>

      <el-divider />

      <!-- Git 信息签页 -->
      <el-tabs v-if="selectedNode.isGitRepo" v-model="activeGitTab">
        <el-tab-pane label="仓库信息" name="repo">
          <GitInfo
            ref="gitInfoRef"
            :repo-path="selectedNode.path"
            :latest-commit="latestCommit"
          />
        </el-tab-pane>
        <el-tab-pane label="提交历史" name="commits" lazy>
          <CommitHistory
            ref="commitHistoryRef"
            :repo-path="selectedNode.path"
            @latest-commit="commit => emit('latestCommit', commit)"
          />
        </el-tab-pane>
      </el-tabs>

      <div v-else-if="selectedNode.type === 'directory'" style="margin-top: 20px;">
        <h3>文件夹操作</h3>
        <el-button-group>
          <el-button @click="emit('createDirectory', selectedNode)">新建文件夹</el-button>
          <el-button @click="emit('createFile', selectedNode)">新建文件</el-button>
          <el-button type="success" @click="showCloneDialog">克隆仓库</el-button>
        </el-button-group>
      </div>

      <div v-else-if="selectedNode.type === 'file'" style="margin-top: 20px;">
        <h3>文件操作</h3>
        <el-button-group>
          <el-button type="primary" @click="handleOpenWithDefaultApp">打开</el-button>
          <el-button @click="previewFile">预览</el-button>
          <el-button @click="emit('rename', selectedNode)">重命名</el-button>
          <el-button type="danger" @click="emit('delete', selectedNode)">删除</el-button>
        </el-button-group>

        <div v-if="filePreview.content" style="margin-top: 20px;">
          <h4>文件内容</h4>
          <el-input
            v-model="filePreview.content"
            type="textarea"
            :rows="10"
            readonly
            style="font-family: monospace;"
          />
        </div>
      </div>
    </div>
    <el-empty v-else description="请从左侧选择文件或文件夹" />

    <!-- 克隆仓库对话框 -->
    <el-dialog v-model="cloneDialogVisible" title="克隆仓库" width="500px">
      <el-form label-width="100px">
        <el-form-item label="目标文件夹">
          <el-input :model-value="selectedNode?.path" disabled />
        </el-form-item>
        <el-form-item label="Git 地址">
          <el-input
            v-model="cloneUrl"
            placeholder="例如: https://github.com/user/repo.git"
            :disabled="cloneLoading"
            @keyup.enter="cloneRepo"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="cloneDialogVisible = false" :disabled="cloneLoading">取消</el-button>
        <el-button type="primary" @click="cloneRepo" :loading="cloneLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 更新仓库进度弹窗 -->
    <el-dialog
      v-model="pullDialogVisible"
      :title="pullCompleted ? '更新完成' : '更新仓库'"
      width="700px"
      :close-on-click-modal="false"
      :close-on-press-escape="!pullCompleted"
      :show-close="pullCompleted"
    >
      <div style="margin-bottom: 16px;">
        <el-progress
          :percentage="pullProgress.total > 0 ? Math.round(pullProgress.current / pullProgress.total * 100) : 0"
          :format="() => `${pullProgress.current} / ${pullProgress.total}`"
          :status="pullCompleted ? (pullSummary.failed > 0 ? 'warning' : 'success') : undefined"
        />
        <div v-if="pullCompleted" style="margin-top: 8px; color: #909399; font-size: 13px;">
          成功: {{ pullSummary.success }}，失败: {{ pullSummary.failed }}
        </div>
      </div>

      <el-table :data="pullResults" style="width: 100%" max-height="400" size="small">
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-icon v-if="row.success" color="#67C23A"><SuccessFilled /></el-icon>
            <el-icon v-else color="#F56C6C"><CircleCloseFilled /></el-icon>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="仓库名称" min-width="150" show-overflow-tooltip />
        <el-table-column prop="path" label="路径" min-width="250" show-overflow-tooltip />
        <el-table-column label="结果" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.success" style="color: #67C23A;">{{ row.output || '已是最新' }}</span>
            <span v-else style="color: #F56C6C;">{{ row.error }}</span>
          </template>
        </el-table-column>
      </el-table>

      <template #footer>
        <el-button type="primary" @click="pullDialogVisible = false" :disabled="!pullCompleted">
          {{ pullCompleted ? '关闭' : '更新中...' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { SuccessFilled, CircleCloseFilled } from '@element-plus/icons-vue'
import {
  PreviewFile, PullRepo, CloneRepo, OpenWithDefaultApp as OpenWithDefaultAppAPI
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import GitInfo from './GitInfo.vue'
import CommitHistory from './CommitHistory.vue'

const props = defineProps({
  selectedNode: { type: Object, default: null },
  latestCommit: { type: Object, default: null }
})

const emit = defineEmits(['latestCommit', 'createDirectory', 'createFile', 'rename', 'delete', 'refreshNode'])

const gitLoading = ref(false)
const gitInfoRef = ref()
const commitHistoryRef = ref()
const activeGitTab = ref('repo')

// ---- 文件预览 ----
const filePreview = ref({ content: '', error: '' })

const previewFile = async () => {
  if (!props.selectedNode) return
  const preview = await PreviewFile(props.selectedNode.path)
  filePreview.value = preview
  if (preview.error) ElMessage.error('预览失败: ' + preview.error)
  else if (preview.tooLarge) ElMessage.warning('文件过大，无法预览')
  else if (preview.isBinary) ElMessage.warning('二进制文件，无法预览')
}

// ---- 打开文件 ----
const handleOpenWithDefaultApp = async () => {
  if (!props.selectedNode || props.selectedNode.type !== 'file') return
  try {
    const result = await OpenWithDefaultAppAPI(props.selectedNode.path)
    if (!result) ElMessage.error('打开文件失败')
  } catch (error) {
    ElMessage.error('打开文件失败: ' + (error.message || String(error)))
  }
}

// ---- 拉取 ----
const pullRepo = async () => {
  if (!props.selectedNode) return
  gitLoading.value = true
  try {
    const result = await PullRepo(props.selectedNode.path)
    ElMessage.success(result || '拉取完成')
    gitInfoRef.value?.handleRefresh()
    commitHistoryRef.value?.handleRefresh()
  } catch (error) {
    ElMessage.error('拉取失败: ' + (error.message || String(error)))
  } finally {
    gitLoading.value = false
  }
}

// ---- 克隆 ----
const cloneDialogVisible = ref(false)
const cloneUrl = ref('')
const cloneLoading = ref(false)

const showCloneDialog = () => {
  cloneUrl.value = ''
  cloneDialogVisible.value = true
}

const cloneRepo = async () => {
  if (!cloneUrl.value.trim()) {
    ElMessage.warning('请输入 Git 仓库地址')
    return
  }
  if (!props.selectedNode) return
  cloneLoading.value = true
  try {
    const result = await CloneRepo(cloneUrl.value.trim(), props.selectedNode.path)
    if (result.includes('成功')) {
      ElMessage.success(result)
      cloneDialogVisible.value = false
      emit('refreshNode', props.selectedNode.path)
    } else {
      ElMessage.error(result)
    }
  } catch (error) {
    ElMessage.error('克隆失败: ' + (error.message || String(error)))
  } finally {
    cloneLoading.value = false
  }
}

// ---- 批量拉取进度 ----
const pullDialogVisible = ref(false)
const pullProgress = reactive({ current: 0, total: 0 })
const pullResults = ref([])
const pullCompleted = ref(false)
const pullSummary = reactive({ success: 0, failed: 0 })

const isWailsRuntime = () => !!window.runtime

const cleanupPullEvents = () => {
  if (!isWailsRuntime()) return
  EventsOff("pull-progress")
  EventsOff("pull-complete")
}

const setupPullEvents = () => {
  if (!isWailsRuntime()) return
  cleanupPullEvents()
  EventsOn("pull-progress", (result) => {
    pullResults.value = [...pullResults.value, result]
    pullProgress.current++
  })
  EventsOn("pull-complete", (summary) => {
    pullCompleted.value = true
    pullSummary.success = summary.success || 0
    pullSummary.failed = summary.failed || 0
  })
}

const startBatchPull = (summary) => {
  pullResults.value = []
  pullProgress.current = 0
  pullProgress.total = summary.total
  pullCompleted.value = false
  pullSummary.success = 0
  pullSummary.failed = 0
  pullDialogVisible.value = true
}

onMounted(() => setupPullEvents())
onBeforeUnmount(() => cleanupPullEvents())

defineExpose({ startBatchPull, clearPreview: () => { filePreview.value = { content: '', error: '' } } })
</script>

<style scoped>
.content-panel {
  height: 100%;
  background-color: #fff;
  overflow-y: auto;
}
</style>
```

**Step 2: 验证**

Run: `cd workbench && wails dev`
Expected: 编译通过，不影响现有功能。

**Step 3: Commit**

```bash
cd workbench
git add frontend/src/components/ContentPanel.vue
git commit -m "feat: 新增 ContentPanel 组件（操作面板）"
```

---

### Task 4: 重构 Home.vue 为布局容器

**Files:**
- Modify: `frontend/src/views/Home.vue` (完全重写)

**Step 1: 重写 Home.vue**

```vue
<template>
  <div class="home">
    <el-container style="height: 100vh;">
      <!-- 左侧：工作目录树 -->
      <el-aside width="200px" class="directory-aside">
        <DirectoryTree
          :directories="directories"
          :selected-id="selectedDirectoryId"
          @select="onDirectorySelect"
          @change="loadDirectories"
        />
      </el-aside>

      <!-- 中间：文件树 -->
      <el-aside width="280px" class="file-tree-aside">
        <FileTreePanel
          ref="fileTreePanelRef"
          :directories="directories"
          :selected-dir-id="selectedDirectoryId"
          @select="onNodeSelect"
          @batch-pull="onBatchPull"
        />
      </el-aside>

      <!-- 右侧：操作面板 -->
      <el-main class="content-main">
        <ContentPanel
          ref="contentPanelRef"
          :selected-node="selectedNode"
          :latest-commit="latestCommit"
          @latest-commit="commit => latestCommit = commit"
          @refresh-node="onRefreshNode"
          @create-directory="onCreateFromContent"
          @create-file="onCreateFromContent"
          @rename="onRenameFromContent"
          @delete="onDeleteFromContent"
        />
      </el-main>
    </el-container>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { GetDirectories, ScanAndPullRepos, DeleteFile, RenameFile } from '../../wailsjs/go/main/App'
import { debug } from '../utils/debug'
import DirectoryTree from '../components/DirectoryTree.vue'
import FileTreePanel from '../components/FileTreePanel.vue'
import ContentPanel from '../components/ContentPanel.vue'

// ---- 核心状态 ----
const directories = ref([])
const selectedDirectoryId = ref('')
const selectedNode = ref(null)
const latestCommit = ref(null)

// ---- 子组件 ref ----
const fileTreePanelRef = ref()
const contentPanelRef = ref()

// ---- 加载工作目录 ----
const loadDirectories = async () => {
  const dirs = await GetDirectories()
  directories.value = dirs || []

  const defaultDir = dirs?.find(d => d.isDefault)
  if (defaultDir) {
    selectedDirectoryId.value = defaultDir.id
  } else if (dirs?.length > 0) {
    selectedDirectoryId.value = dirs[0].id
  }
}

// ---- 目录切换 ----
const onDirectorySelect = (dirId) => {
  selectedDirectoryId.value = dirId
  selectedNode.value = null
  latestCommit.value = null
  contentPanelRef.value?.clearPreview()
}

// ---- 文件树节点选中 ----
const onNodeSelect = (data) => {
  selectedNode.value = data
  contentPanelRef.value?.clearPreview()
}

// ---- 刷新节点 ----
const onRefreshNode = (path) => {
  fileTreePanelRef.value?.refreshNode(path)
}

// ---- 从 ContentPanel 转发到 FileTreePanel 的操作 ----
const onCreateFromContent = (node) => {
  // 选中该节点后，右键菜单中的新建功能在 FileTreePanel 中
  // 这里直接调用 FileTreePanel 的 expose 方法不太合适
  // 改为：点击按钮后触发 FileTreePanel 中对应操作
  // 通过 selectedNode 已经是正确的节点，FileTreePanel 可以直接操作
  fileTreePanelRef.value?.refreshNode(node.path)
}

const onRenameFromContent = async (node) => {
  // ContentPanel 点击重命名时，直接调用后端
  // 因为重命名对话框已经在 FileTreePanel 中
  // 这里使用 FileTreePanel 的 showRenameAt（需要额外 expose）
  // 简化处理：直接在这里弹对话框
  // 更好的方案：将重命名对话框也移到 ContentPanel
  // 但这会重复代码，所以让 FileTreePanel 暴露 showRenameAt
}

const onDeleteFromContent = async (node) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除 "${node.name}" 吗？此操作不可撤销。`,
      '警告',
      { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning' }
    )
  } catch { return }

  try {
    const result = await DeleteFile(node.path)
    if (result) {
      ElMessage.success('删除成功')
      let parentPath = node.path.substring(0, node.path.lastIndexOf('\\'))
      if (!parentPath) parentPath = node.path.substring(0, node.path.lastIndexOf('/'))
      fileTreePanelRef.value?.refreshNode(parentPath)
      selectedNode.value = null
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || String(error)))
  }
}

// ---- 批量拉取 ----
const onBatchPull = async (data) => {
  try {
    const summary = await ScanAndPullRepos(data.path)
    contentPanelRef.value?.startBatchPull(summary)
  } catch (error) {
    ElMessage.warning(error || '未找到任何 Git 仓库')
  }
}

// ---- 初始化 ----
onMounted(async () => {
  await loadDirectories()
  debug.log('Directories loaded:', directories.value)
})
</script>

<style scoped>
.home {
  font-family: 'Microsoft YaHei', Arial, sans-serif;
}
.directory-aside {
  border-right: 1px solid #e6e6e6;
  overflow: hidden;
}
.file-tree-aside {
  border-right: 1px solid #e6e6e6;
  overflow: hidden;
}
.content-main {
  padding: 0;
  overflow-y: auto;
}
</style>
```

**注意：** 上面的 `onRenameFromContent` 需要进一步处理。有两种方案：

**方案 A（推荐）：** 将 FileTreePanel 的 `showRenameAt` 通过 `defineExpose` 暴露，Home.vue 通过 ref 调用。

**方案 B：** ContentPanel 中的「重命名」和「删除」按钮直接通过 emit 通知 Home.vue，Home.vue 弹对话框并调用后端。

推荐方案 A，在 FileTreePanel 的 `defineExpose` 中增加 `showRenameAt` 和 `showCreateAt`。

**Step 2: 补充 FileTreePanel 的 expose**

在 `FileTreePanel.vue` 的 `defineExpose` 中增加：

```js
defineExpose({ refreshNode, collapseAll, showRenameAt, showCreateAt })
```

然后在 Home.vue 中：

```js
const onRenameFromContent = (node) => {
  fileTreePanelRef.value?.showRenameAt(node)
}
```

**Step 3: 验证**

Run: `cd workbench && wails dev`
Expected: 三列布局正确显示，工作目录可点击切换，文件树加载正常。

**Step 4: Commit**

```bash
cd workbench
git add frontend/src/views/Home.vue
git commit -m "feat: 重构 Home.vue 为三列布局容器"
```

---

### Task 5: 功能验证与修复

**Step 1: 启动应用，逐项验证**

Run: `cd workbench && wails dev`

按以下清单验证：

| 序号 | 验证项 | 操作 |
|------|--------|------|
| 1 | 三列布局 | 确认左侧 200px、中间 280px、右侧自适应 |
| 2 | 工作目录选中 | 点击目录项，文件树切换内容 |
| 3 | 工作目录右键 | 重命名、删除、设为默认 |
| 4 | 添加目录 | 点击「+」按钮，弹窗添加 |
| 5 | 文件树展开 | 点击目录节点，子节点懒加载 |
| 6 | 文件树右键 | 新建、重命名、删除、复制路径 |
| 7 | 节点详情 | 点击文件/文件夹，右侧展示详情 |
| 8 | Git 操作 | 选中 Git 仓库，拉取、查看信息/历史 |
| 9 | 文件预览 | 选中文件，点击预览 |
| 10 | 窗口缩放 | 缩放窗口，布局不错乱 |

**Step 2: 修复发现的问题**

记录并修复验证中发现的问题。

**Step 3: Commit**

```bash
cd workbench
git add -A
git commit -m "fix: 修复三列布局重构后的功能问题"
```

---

### Task 6: 清理旧代码

**Files:**
- Modify: `frontend/src/views/Home.vue` (删除残留的旧代码)
- Verify: 无引用旧组件的残留

**Step 1: 确认 Home.vue 干净**

确保 Home.vue 中不再包含：
- 旧的 `el-header` 和 `el-select` 下拉框
- 旧的文件树 `el-tree` 代码
- 旧的右键菜单模板
- 旧的对话框模板
- 不再使用的 import 和变量

**Step 2: 确认 App.vue 无需修改**

Read `frontend/src/App.vue`，确认路由配置正确。

**Step 3: 运行后端测试**

Run: `cd workbench && go test ./...`
Expected: 所有测试通过（后端未改动，应保持绿色）。

**Step 4: Commit**

```bash
cd workbench
git add -A
git commit -m "chore: 清理三列布局重构的残留代码"
```
