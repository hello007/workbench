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
              <el-icon v-if="dir.isDefault" class="dir-item-star" color="#e6a23c">
                <Star />
              </el-icon>
            </div>
            <div class="dir-path" :title="dir.path">{{ dir.path }}</div>
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
      <el-form :model="addForm" label-width="100px">
        <el-form-item label="目录名称">
          <el-input v-model="addForm.name" placeholder="例如: 我的工作空间" />
        </el-form-item>
        <el-form-item label="目录路径">
          <el-input v-model="addForm.path" placeholder="例如: C:\workspace" />
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
    <el-dialog v-model="renameDialogVisible" title="重命名工作目录" width="420px">
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
import { Folder, Star, Plus, Edit, Delete } from '@element-plus/icons-vue'
import { VueDraggable } from 'vue-draggable-plus'
import {
  AddDirectory,
  UpdateDirectory,
  DeleteDirectory,
  SetDefaultDirectory,
  ReorderDirectories
} from '../../wailsjs/go/main/App'

const props = defineProps({
  directories: { type: Array, default: () => [] },
  selectedId: { type: String, default: '' },
  version: { type: String, default: '' }
})

const emit = defineEmits(['select', 'change'])

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

const onContextMenu = (event, dir) => {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.x = event.clientX
  contextMenu.y = event.clientY
  contextMenu.targetDir = dir
  contextMenu.visible = true
}

const closeContextMenu = () => {
  contextMenu.visible = false
}

const onGlobalClick = () => {
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
    case 'delete':
      handleDelete(dir)
      break
  }
}

// --- 添加目录 ---
const addDialogVisible = ref(false)
const addLoading = ref(false)
const addForm = ref({ name: '', path: '', isDefault: false })

const showAddDialog = () => {
  addForm.value = { name: '', path: '', isDefault: false }
  addDialogVisible.value = true
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

const showRenameDialog = (dir) => {
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
  const dir = contextMenu.targetDir
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
  document.addEventListener('click', onGlobalClick)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onGlobalClick)
})
</script>

<style scoped>
.directory-tree-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background-color: #f5f7fa;
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
  font-size: 14px;
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
  padding: 8px 12px;
  cursor: pointer;
  border-left: 3px solid transparent;
  transition: background-color 0.2s ease;
}

.dir-item:hover {
  background-color: #ecf5ff;
}

.dir-item--active {
  background-color: #e6f7ff;
  border-left-color: #409eff;
}

.dir-item-icon {
  flex-shrink: 0;
  margin-right: 8px;
}

.dir-item-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  color: #303133;
}

.dir-item-star {
  flex-shrink: 0;
  margin-left: 6px;
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
  font-size: 11px;
  color: #909399;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
  padding-left: 24px;
}


.dir-version {
  flex-shrink: 0;
  padding: 6px 12px;
  font-size: 12px;
  color: #909399;
  text-align: center;
  border-top: 1px solid #ebeef5;
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

.dir-item--ghost {
  opacity: 0.5;
  background: #c8e6c9;
}
</style>
