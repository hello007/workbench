<template>
  <div class="file-tree-aside">
    <div class="tree-toolbar">
      <el-button-group>
        <el-button size="small" @click="refreshAll">刷新</el-button>
        <el-button size="small" @click="expandAll" :loading="expanding">全部展开</el-button>
        <el-button size="small" @click="collapseAll">全部收起</el-button>
      </el-button-group>
    </div>
    <el-tree
      v-if="selectedDirId"
      :key="treeKey"
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

    <!-- 新建文件夹/文件对话框 -->
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

    <!-- 重命名对话框 -->
    <el-dialog
      v-model="renameDialogVisible"
      title="重命名"
      width="420px"
    >
      <el-form label-width="80px">
        <el-form-item label="当前名称">
          <el-input :model-value="renameNode?.name" disabled />
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

    <!-- 拷贝到对话框 -->
    <el-dialog
      v-model="copyToDialogVisible"
      title="拷贝到"
      width="480px"
    >
      <el-form label-width="100px">
        <el-form-item label="原地址">
          <el-input
            v-model="copyToSourcePath"
            placeholder="请输入原文件或文件夹路径"
            :disabled="copyToLoading"
          />
        </el-form-item>
        <el-form-item label="目标地址">
          <el-input
            v-model="copyToTargetPath"
            placeholder="请输入目标文件夹路径"
            :disabled="copyToLoading"
            @keyup.enter="handleCopyTo"
          />
        </el-form-item>
        <el-form-item>
          <el-checkbox
            v-model="copyToWholeDir"
            :disabled="copyToLoading"
          >
            对原地址目录整体操作
          </el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="copyToDialogVisible = false" :disabled="copyToLoading">取消</el-button>
        <el-button type="primary" @click="handleCopyTo" :loading="copyToLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 自定义右键菜单 -->
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
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
        </li>
        <li class="context-menu-item" :class="{ 'is-disabled': !clipboard.mode }" @click="clipboard.mode && onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyTo')">
          <el-icon><FolderAdd /></el-icon>拷贝到...
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
        <li class="context-menu-item" @click="onMenuCommand('refresh')">
          <el-icon><Refresh /></el-icon>刷新
        </li>
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
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
        </li>
        <li class="context-menu-item" :class="{ 'is-disabled': !clipboard.mode }" @click="clipboard.mode && onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyTo')">
          <el-icon><FolderAdd /></el-icon>拷贝到...
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
  </div>
</template>

<script setup>
import { ref, computed, reactive, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
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
  DocumentCopy
} from '@element-plus/icons-vue'
import { debug } from '../utils/debug'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  GetFileTree,
  CreateDirectory, CreateFile, RenameFile, DeleteFile,
  OpenInExplorer,
  OpenInVSCode,
  OpenInWarp,
  OpenWithDefaultApp,
  ScanAndPullRepos
} from '../../wailsjs/go/main/App'

// ---- Props & Emits ----
const props = defineProps({
  directories: { type: Array, default: () => [] },
  selectedDirId: { type: String, default: '' },
  clipboard: { type: Object, default: () => ({ mode: null }) }
})

const emit = defineEmits(['select', 'batchPull', 'copy', 'cut', 'paste', 'copyTo'])

// ---- Refs ----
const fileTreeRef = ref()
const refreshCounter = ref(0)
const treeKey = computed(() => `${props.selectedDirId}_${refreshCounter.value}`)

const treeProps = {
  label: 'name',
  children: 'children',
  isLeaf: 'isLeaf'
}

// ---- 右键菜单状态 ----
const contextMenu = reactive({
  visible: false,
  x: 0,
  y: 0,
  data: null
})

// ---- 新建对话框状态 ----
const createDialogVisible = ref(false)
const createType = ref('directory')
const createName = ref('')
const createLoading = ref(false)
const createParentData = ref(null)
const createParentPath = computed(() => createParentData.value?.path || '')

// ---- 重命名对话框状态 ----
const renameDialogVisible = ref(false)
const renameName = ref('')
const renameLoading = ref(false)
const renameInputRef = ref()
const renameNode = ref(null)

// ---- 拷贝到对话框状态 ----
const copyToDialogVisible = ref(false)
const copyToSourcePath = ref('')
const copyToTargetPath = ref('')
const copyToWholeDir = ref(true)
const copyToLoading = ref(false)

// ---- 懒加载 ----
const loadTreeNode = async (node, resolve) => {
  debug.log('loadTreeNode called, node:', node)
  debug.log('node.level:', node?.level)
  debug.log('node.data:', node?.data)

  let path
  if (!node || node.level === 0 || !node.data) {
    const dir = props.directories.find(d => d.id === props.selectedDirId)
    if (!dir) {
      debug.log('No directory found for ID:', props.selectedDirId)
      resolve([])
      return
    }
    path = dir.path
    debug.log('Loading root node for path:', path)
  } else {
    path = node.data.path
    debug.log('Loading child nodes for path:', path)
  }

  try {
    const nodes = await GetFileTree(path)
    debug.log('Got nodes for path', path, ':', nodes)

    const processedNodes = (nodes || []).map(n => ({
      ...n,
      isLeaf: n.type === 'file' || !n.hasChildren
    }))

    debug.log('Processed nodes:', processedNodes)
    resolve(processedNodes)
  } catch (error) {
    console.error('Error in loadTreeNode:', error)
    ElMessage.error('加载节点失败: ' + (error.message || error))
    resolve([])
  }
}

// ---- 节点点击 ----
const onNodeClick = (data) => {
  emit('select', data)
}

// ---- 刷新节点 ----
const refreshNode = (nodePath) => {
  if (!fileTreeRef.value || !nodePath) return

  const treeNode = fileTreeRef.value.store.nodesMap[nodePath]
  if (treeNode) {
    treeNode.loaded = false
    treeNode.loading = false
    treeNode.expand()
  }
}

// ---- 全部刷新 ----
const refreshAll = () => {
  refreshCounter.value++
  ElMessage.success('文件树已刷新')
}

// ---- 全部展开 ----
const expanding = ref(false)

const expandAll = async () => {
  if (!fileTreeRef.value) return

  expanding.value = true
  try {
    const expandNode = (node) => {
      return new Promise(resolve => {
        if (node.isLeaf) {
          resolve()
          return
        }
        node.expand(() => {
          Promise.all(node.childNodes.map(child => expandNode(child))).then(resolve)
        })
      })
    }

    const root = fileTreeRef.value.store.root
    await Promise.all(root.childNodes.map(child => expandNode(child)))
    ElMessage.success('已全部展开')
  } catch (error) {
    ElMessage.error('展开失败: ' + (error.message || String(error)))
  } finally {
    expanding.value = false
  }
}

// ---- 全部收起 ----
const collapseAll = () => {
  if (fileTreeRef.value) {
    const allNodes = fileTreeRef.value.store.nodesMap
    Object.keys(allNodes).forEach(key => {
      const node = allNodes[key]
      if (node.expanded) {
        node.expanded = false
      }
    })
    ElMessage.success('已全部收起')
  }
}

// ---- 右键菜单 ----
const onNodeContextMenu = (event, data) => {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.x = event.clientX
  contextMenu.y = event.clientY
  contextMenu.data = data
  contextMenu.visible = true
}

const closeContextMenu = () => {
  contextMenu.visible = false
}

const onGlobalClick = () => {
  closeContextMenu()
}

const onGlobalContextMenu = () => {
  closeContextMenu()
}

// ---- 菜单命令分发 ----
const onMenuCommand = (command) => {
  const data = contextMenu.data
  closeContextMenu()
  if (!data) return

  switch (command) {
    case 'createFile':
      showCreateAt(data, 'file')
      break
    case 'createDir':
      showCreateAt(data, 'directory')
      break
    case 'rename':
      showRenameAt(data)
      break
    case 'delete':
      handleDeleteAt(data)
      break
    case 'cut':
      emit('cut', data)
      break
    case 'copy':
      emit('copy', data)
      break
    case 'paste':
      emit('paste', data)
      break
    case 'copyTo':
      showCopyToDialog(data)
      break
    case 'copyPath':
      copyToClipboard(data.path.replaceAll('\\', '/'), '路径')
      break
    case 'copyName':
      copyToClipboard(data.name, '文件名')
      break
    case 'openExplorer':
      handleOpenExplorer(data.path)
      break
    case 'openInVSCode':
      handleOpenInVSCode(data.path)
      break
    case 'openInWarp':
      handleOpenInWarp(data.path)
      break
    case 'openWithDefaultApp':
      handleOpenWithDefaultApp(data.path)
      break
    case 'refresh':
      refreshNode(data.path)
      break
    case 'pullRepos':
      handleBatchPull(data)
      break
  }
}

// ---- 新建文件/文件夹 ----
const showCreateAt = (data, type) => {
  createParentData.value = data
  createType.value = type
  createName.value = ''
  createDialogVisible.value = true
}

const handleCreate = async () => {
  if (!createName.value.trim()) {
    ElMessage.warning(createType.value === 'directory' ? '请输入文件夹名称' : '请输入文件名称')
    return
  }
  if (!createParentData.value) return

  createLoading.value = true
  try {
    let result
    if (createType.value === 'directory') {
      result = await CreateDirectory(createParentData.value.path, createName.value.trim())
    } else {
      result = await CreateFile(createParentData.value.path, createName.value.trim(), '')
    }
    if (result) {
      ElMessage.success(createType.value === 'directory' ? '文件夹创建成功' : '文件创建成功')
      createDialogVisible.value = false
      refreshNode(createParentData.value.path)
    } else {
      ElMessage.error('创建失败')
    }
  } catch (error) {
    ElMessage.error('创建失败: ' + (error.message || String(error)))
  } finally {
    createLoading.value = false
  }
}

// ---- 重命名 ----
const showRenameAt = (data) => {
  renameNode.value = data
  renameName.value = data.name
  renameDialogVisible.value = true
  setTimeout(() => {
    const input = renameInputRef.value?.input
    if (input) {
      input.focus()
      input.select()
    }
  }, 100)
}

const handleRename = async () => {
  if (!renameName.value.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  if (!renameNode.value) return

  renameLoading.value = true
  try {
    const result = await RenameFile(renameNode.value.path, renameName.value.trim())
    if (result) {
      ElMessage.success('重命名成功')
      renameDialogVisible.value = false
      const targetPath = renameNode.value.path
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) {
        parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      }
      refreshNode(parentPath)
    } else {
      ElMessage.error('重命名失败')
    }
  } catch (error) {
    ElMessage.error('重命名失败: ' + (error.message || String(error)))
  } finally {
    renameLoading.value = false
  }
}

// ---- 删除 ----
const handleDeleteAt = async (data) => {
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

  try {
    const targetPath = data.path
    const result = await DeleteFile(targetPath)
    if (result) {
      ElMessage.success('删除成功')
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) {
        parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      }
      refreshNode(parentPath)
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || String(error)))
  }
}

// ---- 拷贝到对话框 ----
const showCopyToDialog = (data) => {
  copyToSourcePath.value = data.path.replaceAll('\\', '/')
  copyToTargetPath.value = ''
  copyToWholeDir.value = data.type === 'directory'
  copyToLoading.value = false
  copyToDialogVisible.value = true
}

const handleCopyTo = () => {
  if (!copyToSourcePath.value.trim()) {
    ElMessage.warning('请输入原地址')
    return
  }
  if (!copyToTargetPath.value.trim()) {
    ElMessage.warning('请输入目标地址')
    return
  }

  emit('copyTo', {
    sourcePath: copyToSourcePath.value,
    targetPath: copyToTargetPath.value,
    copyWholeDir: copyToWholeDir.value
  })
}

// ---- 复制到剪贴板 ----
const copyToClipboard = async (text, label) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch {
    ElMessage.error('复制失败')
  }
}

// ---- 外部工具 ----
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

const handleOpenInVSCode = async (path) => {
  try {
    const result = await OpenInVSCode(path)
    if (!result) {
      ElMessage.error('打开 VSCode 失败，请确认已安装 VSCode 并将 code 命令加入 PATH')
    }
  } catch (error) {
    ElMessage.error('打开 VSCode 失败: ' + (error.message || String(error)))
  }
}

const handleOpenInWarp = async (path) => {
  try {
    const result = await OpenInWarp(path)
    if (!result) {
      ElMessage.error('打开 Warp 失败，请确认已安装 Warp 终端')
    }
  } catch (error) {
    ElMessage.error('打开 Warp 失败: ' + (error.message || String(error)))
  }
}

const handleOpenWithDefaultApp = async (path) => {
  try {
    const result = await OpenWithDefaultApp(path)
    if (!result) {
      ElMessage.error('打开文件失败')
    }
  } catch (error) {
    ElMessage.error('打开文件失败: ' + (error.message || String(error)))
  }
}

// ---- 批量拉取 ----
const handleBatchPull = (data) => {
  emit('batchPull', data)
}

// ---- 暴露方法 ----
defineExpose({
  refreshNode,
  expandAll,
  collapseAll,
  showRenameAt,
  showCreateAt,
  showCopyToDialog,
  setCopyToLoading: (val) => { copyToLoading.value = val },
  closeCopyToDialog: () => { copyToDialogVisible.value = false }
})

// ---- 生命周期 ----
onMounted(() => {
  document.addEventListener('click', onGlobalClick)
  document.addEventListener('contextmenu', onGlobalContextMenu)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onGlobalClick)
  document.removeEventListener('contextmenu', onGlobalContextMenu)
})
</script>

<style scoped>
.file-tree-aside {
  display: flex;
  flex-direction: column;
  height: 100%;
  border-right: 1px solid #e6e6e6;
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
.el-tree-node__children {
  transition: all 0.3s ease;
  overflow: hidden;
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

.context-menu-item .el-icon {
  margin-right: 6px;
}

.context-menu-divider {
  height: 1px;
  background-color: #e4e7ed;
  margin: 4px 0;
}

.context-menu-item.is-disabled {
  color: #c0c4cc;
  cursor: not-allowed;
}
.context-menu-item.is-disabled:hover {
  background-color: transparent;
  color: #c0c4cc;
}
</style>
