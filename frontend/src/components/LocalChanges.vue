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
        :data="sortedChanges"
        height="100%"
        size="small"
        :row-class-name="rowClassName"
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
        <el-table-column label="暂存" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="row.staged ? 'success' : 'info'" size="small">{{ row.staged ? '已暂存' : '未暂存' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" align="center">
          <template #default="{ row }">
            <el-button v-if="!row.staged" size="small" text @click="stageSingle(row)">+暂存</el-button>
            <el-button v-else size="small" text type="warning" @click="unstageSingle(row)">-取消暂存</el-button>
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
                  <el-dropdown-item command="stageSelected" :disabled="selectedChanges.length === 0">
                    暂存选中 ({{ selectedChanges.length }})
                  </el-dropdown-item>
                  <el-dropdown-item command="unstageSelected" :disabled="selectedChanges.length === 0">
                    取消暂存选中 ({{ selectedChanges.length }})
                  </el-dropdown-item>
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
  HasUpstream,
  StageFiles,
  UnstageFiles
} from '../../wailsjs/go/main/App'
import FileDiffDialog from './FileDiffDialog.vue'
import { handleGitError } from '../utils/gitError'

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

// 单表分组：按 staged 排序，未暂存组在上、已暂存组在下（稳定排序保留组内原序）。
// Staged 字段驱动分组，不拆双栏表，保持单 el-table。
const sortedChanges = computed(() => {
  return [...changes.value].sort((a, b) => {
    const sa = a.staged ? 1 : 0
    const sb = b.staged ? 1 : 0
    return sa - sb
  })
})

// 行级 class：区分已暂存/未暂存组，提供视觉分组提示。
const rowClassName = ({ row }) => (row.staged ? 'row-staged' : 'row-unstaged')

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
    handleGitError('提交失败: ', error)
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
    handleGitError('推送失败: ', error)
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
    handleGitError('回滚失败: ', error)
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
    handleGitError('回滚失败: ', error)
  }
}

// 暂存单文件：行级 + 按钮，调 StageFiles 后刷新面板。
const stageSingle = async (row) => {
  if (!row || !row.path) return
  try {
    await StageFiles(props.repoPath, [row.path])
    await loadChanges()
  } catch (error) {
    handleGitError('暂存失败: ', error)
  }
}

// 取消暂存单文件：行级 - 按钮，调 UnstageFiles（git restore --staged）后刷新面板。
const unstageSingle = async (row) => {
  if (!row || !row.path) return
  try {
    await UnstageFiles(props.repoPath, [row.path])
    await loadChanges()
  } catch (error) {
    handleGitError('取消暂存失败: ', error)
  }
}

// 批量暂存选中：仅暂存未暂存的选中项（git add 对已暂存项幂等，过滤避免冗余）。
const stageSelected = async () => {
  const paths = selectedChanges.value.filter(c => !c.staged).map(c => c.path)
  if (paths.length === 0) {
    ElMessage.warning('选中文件均已暂存')
    return
  }
  try {
    await StageFiles(props.repoPath, paths)
    await loadChanges()
  } catch (error) {
    handleGitError('暂存失败: ', error)
  }
}

// 批量取消暂存选中：仅对已暂存的选中项执行 git restore --staged，
// 避免对未跟踪/未暂存文件调用导致 git 报错。
const unstageSelected = async () => {
  const paths = selectedChanges.value.filter(c => c.staged).map(c => c.path)
  if (paths.length === 0) {
    ElMessage.warning('选中文件均未暂存')
    return
  }
  try {
    await UnstageFiles(props.repoPath, paths)
    await loadChanges()
  } catch (error) {
    handleGitError('取消暂存失败: ', error)
  }
}

const onMoreCommand = (command) => {
  if (command === 'stageSelected') stageSelected()
  else if (command === 'unstageSelected') unstageSelected()
  else if (command === 'discardSelected') discardSelected()
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

defineExpose({ loadChanges, stageSingle, unstageSingle })
</script>

<style scoped>
.local-changes-card {
  height: 100%;
  border-radius: var(--radius-md);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
/* el-card 内部 body 撑满剩余高度（header 固定） */
.local-changes-card :deep(.el-card__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
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
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
/* 表格区：flex:1 撑满剩余高度并内部滚动（表头自动固定）；footer 固定在底部 */
.changes-container :deep(.el-table) {
  flex: 1;
  min-height: 0;
}
/* 单表分组视觉区分：已暂存行浅绿底，未暂存行默认底（Staged 字段驱动 row-class-name） */
.changes-container :deep(.row-staged) {
  background-color: var(--success-bg, #f0f9eb);
}
.changes-container :deep(.row-unstaged) {
  background-color: var(--bg-secondary);
}
.changes-footer {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border-color);
}
.file-path {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  color: var(--text-secondary);
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
