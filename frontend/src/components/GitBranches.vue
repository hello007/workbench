<template>
  <el-card class="git-branches-card" shadow="hover">
    <template #header>
      <div class="card-header">
        <span>Git 分支</span>
        <div class="header-actions">
          <el-button size="small" type="primary" @click="openCreateDialog">新建分支</el-button>
          <el-button
            :icon="Refresh"
            circle
            size="small"
            @click="loadBranches"
            :loading="loading"
          />
        </div>
      </div>
    </template>

    <div v-loading="loading" class="branches-container">
      <div v-if="localBranches.length > 0" class="branch-list">
        <div v-for="branch in localBranches" :key="branch.name" class="branch-row">
          <div class="branch-main">
            <el-text class="branch-name" :title="branch.name">{{ branch.name }}</el-text>
            <el-tag v-if="branch.isCurrent" size="small" type="success">当前</el-tag>
          </div>
          <div class="branch-actions">
            <el-button
              size="small"
              text
              @click="openRenameDialog(branch)"
              :loading="renamingName === branch.name"
            >重命名</el-button>
            <el-button
              size="small"
              text
              type="danger"
              :disabled="branch.isCurrent"
              @click="deleteBranch(branch)"
              :loading="deletingName === branch.name"
            >删除</el-button>
          </div>
        </div>
      </div>

      <el-empty v-else-if="!loading" description="暂无本地分支" />
    </div>

    <!-- 新建分支对话框 -->
    <el-dialog
      v-model="createVisible"
      title="新建分支"
      width="420px"
      :close-on-click-modal="false"
    >
      <el-form label-position="top">
        <el-form-item label="分支名">
          <el-input
            v-model="createName"
            placeholder="从当前 HEAD 创建，如 feature/x"
            @keyup.enter="submitCreate"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" @click="submitCreate" :loading="creating">创建</el-button>
      </template>
    </el-dialog>

    <!-- 重命名分支对话框 -->
    <el-dialog
      v-model="renameVisible"
      title="重命名分支"
      width="420px"
      :close-on-click-modal="false"
    >
      <el-form label-position="top">
        <el-form-item label="原分支名">
          <el-input :model-value="renameTarget?.name" disabled />
        </el-form-item>
        <el-form-item label="新分支名">
          <el-input
            v-model="renameNewName"
            placeholder="输入新分支名"
            @keyup.enter="submitRename"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="renameVisible = false">取消</el-button>
        <el-button type="primary" @click="submitRename" :loading="renaming">重命名</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import {
  GetBranches, CreateBranch, DeleteBranch, RenameBranch
} from '../../wailsjs/go/main/App'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const branches = ref([])
const loading = ref(false)

// 创建分支对话框状态
const createVisible = ref(false)
const createName = ref('')
const creating = ref(false)

// 重命名分支对话框状态
const renameVisible = ref(false)
const renameTarget = ref(null)
const renameNewName = ref('')
const renaming = ref(false)

// 删除分支进行中标记（行级 loading）
const deletingName = ref('')
// 重命名分支进行中标记（行级 loading，与 submitting 区分）
const renamingName = ref('')

// 仅展示本地分支：远程分支切换仍走 ContentPanel 原弹窗，不在本面板展示。
const localBranches = computed(() =>
  (branches.value || []).filter(b => !b.isRemote)
)

const loadBranches = async () => {
  loading.value = true
  try {
    const result = await GetBranches(props.repoPath)
    branches.value = (result && result.branches) || []
  } catch (error) {
    ElMessage.error('加载分支列表失败: ' + (error.message || String(error)))
  } finally {
    loading.value = false
  }
}

const openCreateDialog = () => {
  createName.value = ''
  createVisible.value = true
}

const submitCreate = async () => {
  const name = createName.value.trim()
  if (!name) {
    ElMessage.warning('分支名不能为空')
    return
  }
  creating.value = true
  try {
    await CreateBranch(props.repoPath, name)
    ElMessage.success('分支创建成功')
    createVisible.value = false
    await loadBranches()
  } catch (error) {
    ElMessage.error('创建分支失败: ' + (error.message || String(error)))
  } finally {
    creating.value = false
  }
}

const openRenameDialog = (branch) => {
  renameTarget.value = branch
  renameNewName.value = ''
  renameVisible.value = true
}

const submitRename = async () => {
  if (!renameTarget.value) return
  const newName = renameNewName.value.trim()
  if (!newName) {
    ElMessage.warning('新分支名不能为空')
    return
  }
  const oldName = renameTarget.value.name
  renaming.value = true
  renamingName.value = oldName
  try {
    // 始终显式传入原分支名：git branch -m <old> <new> 对当前/非当前分支均适用，
    // 后端 RenameBranch 校验 oldName 非空（不支持省略 old 的当前分支简写）。
    await RenameBranch(props.repoPath, oldName, newName)
    ElMessage.success('分支已重命名')
    renameVisible.value = false
    await loadBranches()
  } catch (error) {
    ElMessage.error('重命名分支失败: ' + (error.message || String(error)))
  } finally {
    renaming.value = false
    renamingName.value = ''
  }
}

// 删除分支：先走 git branch -d（安全删除）；失败（含 "not fully merged"）弹二次确认，
// 确认后走 git branch -D 强删。当前分支禁用删除按钮（模板 disabled），不触达此方法。
const deleteBranch = async (branch) => {
  if (branch.isCurrent) return
  deletingName.value = branch.name
  try {
    try {
      await DeleteBranch(props.repoPath, branch.name, false)
      ElMessage.success('分支已删除')
      await loadBranches()
      return
    } catch (err) {
      // -d 失败：可能是未完全合并，弹二次确认是否强删
      const confirmed = await ElMessageBox.confirm(
        `分支「${branch.name}」可能未完全合并。是否强制删除（git branch -D）？此操作不可撤销。`,
        '强制删除确认',
        { confirmButtonText: '强制删除', cancelButtonText: '取消', type: 'warning' }
      ).then(() => true).catch(() => false)
      if (!confirmed) return
      await DeleteBranch(props.repoPath, branch.name, true)
      ElMessage.success('分支已强制删除')
      await loadBranches()
    }
  } catch (err) {
    ElMessage.error('删除分支失败: ' + (err.message || String(err)))
  } finally {
    deletingName.value = ''
  }
}

watch(() => props.repoPath, () => {
  branches.value = []
  loadBranches()
})

loadBranches()

defineExpose({ loadBranches })
</script>

<style scoped>
.git-branches-card {
  border-radius: var(--radius-md);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 600;
  font-size: 16px;
  color: var(--text-primary);
}
.header-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
.branches-container {
  min-height: 60px;
}
.branch-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}
.branch-row {
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-left: 3px solid var(--border-light);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  transition: all var(--transition-fast);
}
.branch-row:hover {
  box-shadow: var(--shadow-md);
  border-left-color: var(--primary-color);
}
.branch-main {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}
.branch-name {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  font-weight: 600;
  color: var(--primary-color);
}
.branch-actions {
  margin-top: 6px;
  display: flex;
  gap: var(--spacing-xs);
  justify-content: flex-end;
}
</style>
