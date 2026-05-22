<template>
  <el-card class="local-changes-card" shadow="hover">
    <template #header>
      <div class="card-header">
        <span>本地变动</span>
        <div class="header-actions">
          <el-tag v-if="changes.length > 0" size="small" type="warning">{{ changes.length }} 个文件</el-tag>
          <el-button
            :icon="Refresh"
            circle
            size="small"
            @click="loadChanges"
            :loading="loading"
          />
        </div>
      </div>
    </template>

    <div v-loading="loading" class="changes-container">
      <el-table
        v-if="changes.length > 0"
        ref="tableRef"
        :data="changes"
        max-height="500"
        size="small"
        @selection-change="onSelectionChange"
      >
        <el-table-column type="selection" width="40" />
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">{{ getStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="文件路径" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="file-path">{{ row.path }}</span>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-else-if="!loading" description="没有本地变动" :image-size="60" />

      <div v-if="changes.length > 0" class="changes-footer">
        <el-button size="small" type="danger" @click="discardSelected" :disabled="selectedChanges.length === 0">
          回滚选中 ({{ selectedChanges.length }})
        </el-button>
        <el-button size="small" type="warning" @click="discardAll">
          全部回滚
        </el-button>
      </div>
    </div>
  </el-card>
</template>

<script setup>
import { ref, watch, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { GetLocalChanges, DiscardChanges } from '../../wailsjs/go/main/App'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const changes = ref([])
const selectedChanges = ref([])
const loading = ref(false)
const tableRef = ref()

const loadChanges = async () => {
  loading.value = true
  try {
    const result = await GetLocalChanges(props.repoPath)
    changes.value = result || []
  } catch (error) {
    ElMessage.error('加载本地变动失败: ' + (error.message || String(error)))
  } finally {
    loading.value = false
  }
}

const onSelectionChange = (selection) => {
  selectedChanges.value = selection
}

const discardSelected = async () => {
  if (selectedChanges.value.length === 0) return
  try {
    await ElMessageBox.confirm(
      `确定回滚选中的 ${selectedChanges.value.length} 个文件吗？此操作不可撤销。`,
      '警告',
      { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }

  try {
    const paths = selectedChanges.value.map(c => c.path)
    await DiscardChanges(props.repoPath, paths)
    ElMessage.success('回滚成功')
    loadChanges()
  } catch (error) {
    ElMessage.error('回滚失败: ' + (error.message || String(error)))
  }
}

const discardAll = async () => {
  try {
    await ElMessageBox.confirm(
      '确定回滚所有本地变动吗？此操作不可撤销。',
      '警告',
      { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }

  try {
    await DiscardChanges(props.repoPath, [])
    ElMessage.success('全部回滚成功')
    loadChanges()
  } catch (error) {
    ElMessage.error('回滚失败: ' + (error.message || String(error)))
  }
}

const getStatusType = (status) => {
  switch (status) {
    case 'M': return 'warning'
    case 'A': return 'success'
    case 'D': return 'danger'
    case '?': return 'info'
    default: return 'info'
  }
}

const getStatusLabel = (status) => {
  switch (status) {
    case 'M': return '已修改'
    case 'A': return '已添加'
    case 'D': return '已删除'
    case 'R': return '已重命名'
    case '?': return '未跟踪'
    default: return status
  }
}

watch(() => props.repoPath, () => {
  changes.value = []
  loadChanges()
})

onMounted(() => {
  loadChanges()
})

defineExpose({ loadChanges })
</script>

<style scoped>
.local-changes-card {
  height: 100%;
  border-radius: var(--radius-md);
  overflow: hidden;
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
  gap: 8px;
}
.changes-container {
  min-height: 100px;
}
.file-path {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  color: var(--text-secondary);
}
.changes-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border-color);
}
</style>
