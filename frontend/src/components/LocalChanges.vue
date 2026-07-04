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
        @row-dblclick="openDiff"
      >
        <el-table-column type="selection" width="40" />
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">{{ getStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="文件路径（双击查看差异）" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="file-path">{{ row.path }}</span>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-else-if="!loading" description="没有本地变动" :image-size="60" />

      <div v-if="changes.length > 0" class="changes-footer">
        <!-- commit message 输入区 -->
        <el-input
          v-model="commitMessage"
          type="textarea"
          :rows="2"
          placeholder="请输入提交信息（必填）"
          class="commit-input"
          resize="vertical"
        />

        <!-- 操作按钮组 -->
        <div class="action-bar">
          <div class="action-left">
            <el-button
              size="small"
              type="primary"
              :loading="committing"
              :disabled="!canCommit"
              @click="commitSelected"
            >
              提交
            </el-button>
            <el-button
              size="small"
              type="success"
              :loading="committing"
              :disabled="!canCommit"
              @click="commitAndPush"
            >
              提交并推送
            </el-button>
            <el-button
              size="small"
              type="warning"
              :loading="pushing"
              :disabled="selectedChanges.length === 0 && changes.length === 0"
              @click="pushOnly"
            >
              推送
            </el-button>
          </div>

          <div class="action-right">
            <el-dropdown trigger="click" @command="onMoreCommand">
              <el-button size="small">
                更多<el-icon class="el-icon--right"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="discardSelected" :disabled="selectedChanges.length === 0">
                    回滚选中 ({{ selectedChanges.length }})
                  </el-dropdown-item>
                  <el-dropdown-item command="discardAll">全部回滚</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
      </div>
    </div>

    <!-- 双栏 diff 弹窗 -->
    <FileDiffDialog v-model="diffVisible" :repo-path="repoPath" :file="diffFile" />
  </el-card>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, ArrowDown } from '@element-plus/icons-vue'
import {
  GetLocalChanges,
  DiscardChanges,
  CommitFiles,
  PushRepo,
  HasUpstream
} from '../../wailsjs/go/main/App'
import FileDiffDialog from './FileDiffDialog.vue'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const emit = defineEmits(['committed'])

const changes = ref([])
const selectedChanges = ref([])
const loading = ref(false)
const tableRef = ref()

// commit / push 状态
const commitMessage = ref('')
const committing = ref(false)
const pushing = ref(false)

// diff 弹窗状态
const diffVisible = ref(false)
const diffFile = ref('')

const canCommit = computed(() => {
  return selectedChanges.value.length > 0 && commitMessage.value.trim().length > 0
})

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

const openDiff = (row) => {
  if (!row || !row.path) return
  diffFile.value = row.path
  diffVisible.value = true
}

/**
 * 调用 CommitFiles 提交勾选文件。
 * @param withPush 是否在提交成功后接着推送
 */
const doCommit = async (withPush) => {
  if (!canCommit.value) return
  const paths = selectedChanges.value.map(c => c.path)
  if (paths.length === 0) {
    ElMessage.warning('请先勾选要提交的文件')
    return
  }
  const message = commitMessage.value.trim()
  committing.value = true
  try {
    await CommitFiles(props.repoPath, message, paths)
    ElMessage.success(withPush ? '提交成功，准备推送...' : '提交成功')
    commitMessage.value = ''
    // 清空表格勾选状态
    tableRef.value?.clearSelection?.()
    await loadChanges()
    emit('committed')

    if (withPush) {
      committing.value = false
      await doPush()
    }
  } catch (error) {
    ElMessage.error('提交失败: ' + (error?.message || String(error)))
  } finally {
    committing.value = false
  }
}

const commitSelected = () => doCommit(false)
const commitAndPush = () => doCommit(true)

/**
 * 推送：先 HasUpstream 判断；无上游弹确认是否 set-upstream。
 */
const doPush = async () => {
  pushing.value = true
  try {
    let setUpstream = false
    try {
      const has = await HasUpstream(props.repoPath)
      if (!has) {
        try {
          await ElMessageBox.confirm(
            '当前分支无上游，是否设置上游（git push --set-upstream origin <当前分支>）并推送？',
            '无上游分支',
            { confirmButtonText: '设置并推送', cancelButtonText: '取消', type: 'warning' }
          )
          setUpstream = true
        } catch {
          // 用户取消
          ElMessage.info('已取消推送')
          return
        }
      }
    } catch (e) {
      // HasUpstream 探测失败：按常规推送（不 set-upstream），让 git 报错透传
      ElMessage.warning('无法判断上游分支，将尝试常规推送')
    }

    const output = await PushRepo(props.repoPath, setUpstream)
    const text = (output || '').trim()
    if (text.length > 200) {
      // 超长输出截断展示
      ElMessage.success(text.slice(0, 200) + '...')
    } else {
      ElMessage.success(text || '推送完成')
    }
    await loadChanges()
    emit('committed')
  } catch (error) {
    ElMessage.error('推送失败: ' + (error?.message || String(error)))
  } finally {
    pushing.value = false
  }
}

const pushOnly = () => doPush()

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

const onMoreCommand = (command) => {
  if (command === 'discardSelected') discardSelected()
  else if (command === 'discardAll') discardAll()
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
  commitMessage.value = ''
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
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border-color);
}

.commit-input {
  width: 100%;
}

.commit-input :deep(.el-textarea__inner) {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
}

.action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.action-left {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.action-right {
  display: flex;
  align-items: center;
}
</style>
