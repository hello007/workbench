<template>
  <div class="directory-tree-panel">
    <!-- 工具栏 -->
    <div class="dir-toolbar">
      <span class="dir-toolbar-title">工作目录</span>
      <el-button :icon="Plus" circle size="small" @click="showAddDialog" />
    </div>

    <!-- 目录列表 -->
    <div class="dir-list">
      <VueDraggable
        v-model="localDirectories"
        :animation="200"
        ghost-class="dir-item--ghost"
        :prevent-on-filter="false"
        @end="onDragEnd"
      >
        <div
          v-for="dir in localDirectories"
          :key="dir.id"
          class="dir-item"
          :class="{ 'dir-item--active': dir.id === selectedId }"
          @mousedown="handleSelect(dir.id)"
          @click="handleSelect(dir.id)"
          @contextmenu="onContextMenu($event, dir)"
        >
          <div class="dir-info">
            <div class="dir-row">
              <el-icon class="dir-item-icon" color="#909399">
                <Folder />
              </el-icon>
              <span class="dir-item-name" :title="dir.name">{{ dir.name }}</span>
              <img
                v-if="dir.isGitRepo"
                :src="dir.hasRemote ? gitIcon : gitGrayIcon"
                class="dir-item-git-img"
                :alt="dir.hasRemote ? 'Git 仓库' : 'Git 仓库（无远程）'"
                :title="dir.hasRemote ? 'Git 仓库' : 'Git 仓库（未配置远程）'"
              />
              <el-icon v-if="dir.isDefault" class="dir-item-star" color="#e6a23c">
                <Star />
              </el-icon>
            </div>
            <div class="dir-path" :title="dir.path">{{ shortenPath(dir.path) }}</div>
          </div>
        </div>
      </VueDraggable>
      <el-empty
        v-if="localDirectories.length === 0"
        description="暂无工作目录"
        :image-size="80"
      />
    </div>

    <!-- 版本号 -->
    <div v-if="version" class="dir-version">v{{ version }}</div>

    <!-- 右键菜单 -->
    <ul
      v-if="contextMenu.visible"
      class="context-menu"
      :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }"
      @click.stop
      @mousedown.stop
    >
      <li class="context-menu-item" @click="onMenuCommand('rename')">
        <el-icon><Edit /></el-icon>重命名
        <span class="context-menu-shortcut">{{ shortcutRename }}</span>
      </li>
      <li class="context-menu-item" @click="onMenuCommand('setDefault')">
        <el-icon><Star /></el-icon>设为默认
      </li>
      <li class="context-menu-item" @click="onMenuCommand('copyPath')">
        <el-icon><CopyDocument /></el-icon>复制路径
      </li>
      <li class="context-menu-divider" />
      <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
        <img :src="explorerIcon" class="context-menu-img-icon" alt="资源管理器" />在资源管理器中打开
      </li>
      <li class="context-menu-item" @click="onMenuCommand('openVSCode')">
        <img :src="vscodeIcon" class="context-menu-img-icon" alt="VSCode" />用 VSCode 打开
      </li>
      <li class="context-menu-item" @click="onMenuCommand('openWarp')">
        <img :src="warpIcon" class="context-menu-img-icon" alt="Warp" />用 Warp 打开
      </li>
      <li class="context-menu-item" @click="onMenuCommand('openObsidian')">
        <img :src="obsidianIcon" class="context-menu-img-icon" alt="Obsidian" />用 Obsidian 打开
      </li>
      <li class="context-menu-divider" />
      <li class="context-menu-item" @click="onMenuCommand('openRepoFilter')">
        <el-icon><Filter /></el-icon>仓库筛选器
      </li>
      <li class="context-menu-item" @click="onMenuCommand('pullRepos')">
        <el-icon><Refresh /></el-icon>更新仓库
      </li>
      <li class="context-menu-divider" />
      <li class="context-menu-item" @click="onMenuCommand('delete')">
        <el-icon><Delete /></el-icon>删除
        <span class="context-menu-shortcut">{{ shortcutDelete }}</span>
      </li>
    </ul>

    <!-- 添加目录对话框 -->
    <el-dialog v-model="addDialogVisible" title="添加工作目录" width="500px" append-to-body>
      <el-form :model="addForm" label-width="100px">
        <el-form-item label="目录名称">
          <el-input v-model="addForm.name" placeholder="例如: 我的工作空间" />
        </el-form-item>
        <el-form-item label="目录路径">
          <el-input ref="addPathInputRef" v-model="addForm.path" placeholder="例如: C:\workspace" />
        </el-form-item>
        <el-form-item label="设为默认">
          <el-switch v-model="addForm.isDefault" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleAdd" :loading="addLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重命名目录对话框 -->
    <el-dialog v-model="renameDialogVisible" title="重命名工作目录" width="420px" append-to-body>
      <el-form label-width="80px">
        <el-form-item label="当前名称">
          <el-input :model-value="contextMenu.targetDir?.name" disabled />
        </el-form-item>
        <el-form-item label="新名称">
          <el-input
            ref="renameInputRef"
            v-model="renameName"
            placeholder="请输入新名称"
            :disabled="renameLoading"
            @keyup.enter="handleRename"
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
import { ref, reactive, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Folder, Star, Plus, Edit, Delete, FolderOpened, Refresh, CopyDocument, Filter } from '@element-plus/icons-vue'
import { VueDraggable } from 'vue-draggable-plus'
import {
  AddDirectory,
  UpdateDirectory,
  DeleteDirectory,
  SetDefaultDirectory,
  ReorderDirectories,
  OpenInExplorer,
  OpenInVSCode,
  OpenInWarp,
  OpenInObsidian,
  OpenObsidianVaultManager,
  CopyObsidianVaultPath,
  AutoRegisterAndOpen
} from '../../wailsjs/go/main/App'
import obsidianIcon from '../assets/icons/obsidian.png'
import explorerIcon from '../assets/icons/explorer.png'
import vscodeIcon from '../assets/icons/vscode.ico'
import warpIcon from '../assets/icons/warp.ico'
import gitIcon from '../assets/icons/git.png'
import gitGrayIcon from '../assets/icons/git-gray.png'
import { useShortcuts } from '../composables/useShortcuts'

function shortenPath(path) {
  if (!path || path.length <= 40) return path
  const parts = path.replace(/\\/g, '/').split('/')
  if (parts.length <= 3) return path
  return `.../${parts[parts.length - 2]}/${parts[parts.length - 1]}`
}

const props = defineProps({
  directories: { type: Array, default: () => [] },
  selectedId: { type: String, default: '' },
  version: { type: String, default: '' }
})

const emit = defineEmits(['select', 'change', 'contextmenu', 'batchPull', 'openRepoFilter'])

const { shortcutRename, shortcutDelete } = useShortcuts()

// --- 本地目录列表（可变，用于拖拽） ---
const localDirectories = ref([...props.directories])
watch(() => props.directories, (val) => {
  localDirectories.value = [...val]
})

// --- 选中 ---
const handleSelect = (dirId) => {
  emit('select', dirId)
}

// --- 右键菜单 ---
const contextMenu = reactive({
  visible: false,
  x: 0,
  y: 0,
  targetDir: null
})

// 暴露关闭菜单的方法
const closeMenu = () => {
  contextMenu.visible = false
}

// 键盘快捷键入口：作用于当前选中的工作目录（F2 重命名 / Del 删除）
// showRenameDialog / handleDelete 在下方定义，函数调用时才查找，无 TDZ 问题
const triggerRenameCurrent = () => {
  const dir = localDirectories.value.find(d => d.id === props.selectedId)
  if (dir) showRenameDialog(dir)
}

const triggerDeleteCurrent = () => {
  const dir = localDirectories.value.find(d => d.id === props.selectedId)
  if (dir) handleDelete(dir)
}

defineExpose({
  closeMenu,
  triggerRenameCurrent,
  triggerDeleteCurrent
})

const onContextMenu = (event, dir) => {
  event.preventDefault()
  event.stopPropagation() // 恢复 stopPropagation()，防止事件冒泡

  // 通知父组件关闭另一个组件的菜单
  emit('contextmenu')

  // 先设置菜单位置
  let x = event.clientX
  let y = event.clientY

  contextMenu.x = x
  contextMenu.y = y
  contextMenu.targetDir = dir
  contextMenu.visible = true

  // 等菜单渲染完成后测量实际高度并调整位置
  nextTick(() => {
    const menuElement = document.querySelector('.context-menu')
    if (menuElement) {
      const rect = menuElement.getBoundingClientRect()
      const menuWidth = rect.width
      const menuHeight = rect.height

      // 重新检查并调整边界
      let adjustedX = x
      let adjustedY = y

      // 检查右侧边界
      if (adjustedX + menuWidth > window.innerWidth) {
        adjustedX = window.innerWidth - menuWidth - 5
      }

      // 检查底部边界
      if (adjustedY + menuHeight > window.innerHeight) {
        adjustedY = window.innerHeight - menuHeight - 5
      }

      // 检查左侧边界
      if (adjustedX < 5) {
        adjustedX = 5
      }

      // 检查顶部边界
      if (adjustedY < 5) {
        adjustedY = 5
      }

      // 如果位置有变化，更新菜单位置
      if (adjustedX !== x || adjustedY !== y) {
        contextMenu.x = adjustedX
        contextMenu.y = adjustedY
      }
    }
  })
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
    case 'copyPath':
      copyToClipboard(dir.path.replaceAll('\\', '/'), '路径')
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
    case 'openObsidian':
      handleOpenObsidian(dir.path)
      break
    case 'openRepoFilter':
      emit('openRepoFilter', dir.id)
      break
    case 'pullRepos':
      emit('batchPull', { path: dir.path })
      break
    case 'delete':
      handleDelete(dir)
      break
  }
}

// --- 添加目录 ---
const addDialogVisible = ref(false)
const addLoading = ref(false)
const addForm = ref({ name: '', path: '', isDefault: false })
const addNameManuallySet = ref(false)
const addPathInputRef = ref()

watch(() => addForm.value.path, (newPath) => {
  if (addNameManuallySet.value) return
  const trimmed = newPath.replace(/[\\/]+$/, '')
  const lastSep = Math.max(trimmed.lastIndexOf('/'), trimmed.lastIndexOf('\\'))
  addForm.value.name = lastSep >= 0 ? trimmed.substring(lastSep + 1) : trimmed
})

watch(() => addForm.value.name, (newVal, oldVal) => {
  if (oldVal === '' && newVal !== '') {
    addNameManuallySet.value = true
  }
})

const showAddDialog = () => {
  addForm.value = { name: '', path: '', isDefault: false }
  addNameManuallySet.value = false
  addDialogVisible.value = true
  setTimeout(() => {
    const input = addPathInputRef.value?.input
    if (input) {
      input.focus()
    }
  }, 100)
}

const handleAdd = async () => {
  if (!addForm.value.name.trim()) {
    ElMessage.warning('请输入目录名称')
    return
  }
  if (!addForm.value.path.trim()) {
    ElMessage.warning('请输入目录路径')
    return
  }

  addLoading.value = true
  try {
    const result = await AddDirectory(
      addForm.value.name.trim(),
      addForm.value.path.trim(),
      addForm.value.isDefault
    )
    if (result) {
      ElMessage.success('添加成功')
      addDialogVisible.value = false
      emit('change')
    } else {
      ElMessage.error('添加失败')
    }
  } catch (error) {
    ElMessage.error('添加失败: ' + (error.message || String(error)))
  } finally {
    addLoading.value = false
  }
}

// --- 重命名目录 ---
const renameDialogVisible = ref(false)
const renameLoading = ref(false)
const renameName = ref('')
const renameInputRef = ref()
const renameTargetDir = ref(null)

const showRenameDialog = (dir) => {
  renameTargetDir.value = dir
  renameName.value = dir.name
  renameDialogVisible.value = true
  nextTick(() => {
    const input = renameInputRef.value?.input
    if (input) {
      input.focus()
      input.select()
    }
  })
}

const handleRename = async () => {
  const dir = renameTargetDir.value
  if (!renameName.value.trim()) {
    ElMessage.warning('请输入新名称')
    return
  }
  if (!dir) return

  renameLoading.value = true
  try {
    const result = await UpdateDirectory(
      dir.id,
      renameName.value.trim(),
      dir.path,
      dir.isDefault
    )
    if (result) {
      ElMessage.success('重命名成功')
      renameDialogVisible.value = false
      emit('change')
    } else {
      ElMessage.error('重命名失败')
    }
  } catch (error) {
    ElMessage.error('重命名失败: ' + (error.message || String(error)))
  } finally {
    renameLoading.value = false
  }
}

// --- 设为默认 ---
const handleSetDefault = async (dir) => {
  try {
    const result = await SetDefaultDirectory(dir.id)
    if (result) {
      ElMessage.success('已设为默认目录')
      emit('change')
    } else {
      ElMessage.error('设置失败')
    }
  } catch (error) {
    ElMessage.error('设置失败: ' + (error.message || String(error)))
  }
}

// --- 删除目录 ---
const handleDelete = async (dir) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除工作目录 "${dir.name}" 吗？此操作不会删除实际文件。`,
      '确认删除',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch {
    return
  }

  try {
    const result = await DeleteDirectory(dir.id)
    if (result) {
      ElMessage.success('删除成功')
      emit('change')
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || String(error)))
  }
}

// --- 复制到剪贴板 ---
const copyToClipboard = async (text, label) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch {
    ElMessage.error('复制失败')
  }
}

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

const handleOpenObsidian = async (path) => {
  try {
    const status = await OpenInObsidian(path)
    if (status === 'not-installed') {
      ElMessage.error('未检测到 Obsidian，请在【设置 → 通用 → 外部应用】中配置 Obsidian 程序路径，或安装 Obsidian 并至少运行一次')
    } else if (status === 'not-registered') {
      try {
        await ElMessageBox.confirm('该目录未注册为 Obsidian vault。', '提示', {
          confirmButtonText: '自动注册并打开',
          cancelButtonText: '打开仓库管理器',
          distinguishCancelAndClose: true,
          type: 'info'
        })
        // 用户点「自动注册并打开」-> 二次确认（预告信任提示 + 备份 + 运行中需关闭）
        try {
          await ElMessageBox.confirm(
            '即将把该目录注册为 Obsidian vault 并打开。<br>• 首次打开会弹出信任提示，请选择「Trust author and enable plugins」<br>• Obsidian 配置已自动备份<br>• 若 Obsidian 正在运行，需先关闭后重试',
            '确认自动注册',
            { confirmButtonText: '继续', cancelButtonText: '取消', type: 'warning', dangerouslyUseHTMLString: true }
          )
          const regStatus = await AutoRegisterAndOpen(path)
          if (regStatus === '') {
            ElMessage.success('已注册为 Obsidian vault 并打开，首次会弹信任提示请确认')
          } else if (regStatus === 'running') {
            ElMessage.warning('Obsidian 正在运行，请关闭所有 Obsidian 窗口后重试')
          } else if (regStatus === 'not-installed') {
            ElMessage.error('未检测到 Obsidian，请在【设置 -> 通用 -> 外部应用】中配置')
          } else {
            ElMessage.error('自动注册失败，请重试或手动添加')
          }
        } catch {
          // 二次确认取消，不处理
        }
      } catch (action) {
        if (action === 'cancel') {
          // 「打开仓库管理器」：复制路径 + 提示 + 跳转 choose-vault
          const copied = await CopyObsidianVaultPath(path)
          if (copied) ElMessage.success('已复制目录路径到剪贴板')
          else ElMessage.warning('复制路径失败')
          const opened = await OpenObsidianVaultManager()
          if (opened) ElMessage.info('已打开 Obsidian 仓库管理器，请将该目录添加为 vault')
          else ElMessage.error('打开 Obsidian 仓库管理器失败')
        }
        // action === 'close' -> 关闭（X），不处理
      }
    }
  } catch (error) {
    ElMessage.error('打开 Obsidian 失败: ' + (error.message || String(error)))
  }
}

// --- 拖拽排序 ---
const onDragEnd = async () => {
  const ids = localDirectories.value.map(d => d.id)
  try {
    const result = await ReorderDirectories(ids)
    if (!result) {
      ElMessage.error('排序保存失败')
      emit('change')
    }
  } catch (error) {
    ElMessage.error('排序保存失败')
    emit('change')
  }
}

// --- 生命周期 ---
onMounted(() => {
  document.addEventListener('mousedown', onGlobalClick)
  document.addEventListener('contextmenu', onGlobalContextMenu)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onGlobalClick)
  document.removeEventListener('contextmenu', onGlobalContextMenu)
})
</script>

<style scoped>
.directory-tree-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background-color: var(--bg-primary);
}

.dir-toolbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}

.dir-toolbar-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 0.5px;
}

.dir-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-sm) 0;
}

.dir-item {
  padding: 8px 16px;
  cursor: pointer;
  border-left: 3px solid transparent;
  transition: all var(--transition-normal);
  border-radius: var(--radius-sm);
  margin: 2px 8px;
  font-size: 13px;
}

.dir-item:hover {
  background-color: var(--bg-tertiary);
}

.dir-item--active {
  background-color: rgba(64, 158, 255, 0.1);
  border-left-color: var(--primary-color);
}

.dir-item--active:hover {
  background-color: rgba(64, 158, 255, 0.15);
}

.dir-item-icon {
  flex-shrink: 0;
  margin-right: var(--spacing-sm);
}

.dir-item-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.dir-item-star {
  flex-shrink: 0;
  margin-left: var(--spacing-xs);
  animation: pulse 1.5s infinite;
}

.dir-item-git {
  flex-shrink: 0;
  margin-left: 5px;
}

/* git 仓库标记 img（替代原 SuccessFilled 绿对勾，尺寸/对齐与原 el-icon 一致） */
.dir-item-git-img {
  flex-shrink: 0;
  width: 14px;
  height: 14px;
  margin-left: 5px;
  vertical-align: middle;
  object-fit: contain;
}

@keyframes pulse {
  0%, 100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.1);
  }
}

.dir-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}

.dir-row {
  display: flex;
  align-items: center;
}

.dir-path {
  font-size: 13px;
  color: var(--text-tertiary);
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 4px;
  padding-left: 4px;
  font-family: Consolas, 'Courier New', monospace;
}

.dir-version {
  flex-shrink: 0;
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: 12px;
  color: var(--text-tertiary);
  text-align: center;
  border-top: 1px solid var(--border-color);
  background: var(--bg-tertiary);
  font-weight: 500;
}

.dir-item--ghost {
  opacity: 0.6;
  background: rgba(103, 194, 58, 0.2);
  border-radius: var(--radius-sm);
  border: 1px dashed var(--success-color);
}
</style>
