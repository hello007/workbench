<template>
  <el-card class="git-stash-card" shadow="hover">
    <template #header>
      <div class="card-header">
        <span>Stash 暂存</span>
        <div class="header-actions">
          <el-button size="small" type="primary" @click="openPushDialog">暂存现场</el-button>
          <el-button
            :icon="Refresh"
            circle
            size="small"
            @click="loadStashes"
            :loading="loading"
          />
        </div>
      </div>
    </template>

    <div v-loading="loading" class="stash-container">
      <div v-if="stashes.length > 0" class="stash-list">
        <div v-for="stash in stashes" :key="stash.index" class="stash-row">
          <div class="stash-main">
            <el-text class="stash-description" :title="stash.description">{{ stash.description }}</el-text>
            <div class="stash-meta">
              <el-tag v-if="stash.branch" size="small" type="info">{{ stash.branch }}</el-tag>
              <el-text v-if="stash.shortSha" size="small" class="stash-sha">{{ stash.shortSha }}</el-text>
              <el-text v-if="stash.dateTime" size="small" type="info">{{ stash.dateTime }}</el-text>
            </div>
          </div>
          <div class="stash-actions">
            <el-button
              size="small"
              text
              type="primary"
              @click="restoreStash(stash)"
              :loading="operatingKey === `pop-${stash.index}`"
            >恢复</el-button>
            <el-button
              size="small"
              text
              title="恢复变更但保留本条目在列表中"
              @click="applyStash(stash)"
              :loading="operatingKey === `apply-${stash.index}`"
            >应用</el-button>
            <el-button
              size="small"
              text
              type="danger"
              @click="dropStash(stash)"
              :loading="operatingKey === `drop-${stash.index}`"
            >删除</el-button>
          </div>
        </div>
      </div>

      <el-empty v-else-if="!loading" description="暂无 stash 条目" :image-size="64" />
    </div>

    <!-- 暂存现场对话框：描述可空（git 默认 WIP 文案），untracked 显式勾选 -->
    <el-dialog
      v-model="pushVisible"
      title="暂存现场"
      width="420px"
      :close-on-click-modal="false"
    >
      <el-form label-position="top">
        <el-form-item label="描述（可选，留空使用默认文案）">
          <el-input
            v-model="pushMessage"
            placeholder="例如：切换分支前的进行中改动"
            @keyup.enter="submitPush"
          />
        </el-form-item>
        <el-checkbox v-model="pushIncludeUntracked">包含未跟踪文件（新文件一并暂存）</el-checkbox>
      </el-form>
      <template #footer>
        <el-button @click="pushVisible = false">取消</el-button>
        <el-button type="primary" @click="submitPush" :loading="pushing">暂存</el-button>
      </template>
    </el-dialog>

    <!-- 冲突结果对话框：完整输出 + 冲突文件清单（超长输出可滚动） -->
    <el-dialog
      v-model="conflictVisible"
      title="恢复遇到冲突"
      width="560px"
      :close-on-click-modal="false"
    >
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        title="冲突未解决，stash 条目已保留"
        description="请解决冲突后提交变更；冲突解决可使用「合并/变基」页。"
      />
      <div class="conflict-files">
        <span class="conflict-files-label">冲突文件：</span>
        <el-tag v-for="file in conflictResult?.conflictFiles" :key="file" size="small" type="danger" class="conflict-file-tag">
          {{ file }}
        </el-tag>
      </div>
      <pre v-if="conflictResult?.output" class="conflict-output">{{ conflictResult.output }}</pre>
      <template #footer>
        <el-button type="primary" @click="conflictVisible = false">知道了</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import {
  ListStashes, StashPush, StashPop, StashApply, StashDrop
} from '../../wailsjs/go/main/App'
import { handleGitError } from '../utils/gitError'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const stashes = ref([])
const loading = ref(false)

// 暂存现场对话框状态
const pushVisible = ref(false)
const pushMessage = ref('')
const pushIncludeUntracked = ref(false)
const pushing = ref(false)

// 行级操作进行中标记（pop/apply/drop 按「动作-序号」区分，互不串 loading）
const operatingKey = ref('')

// 冲突结果对话框状态（pop/apply 冲突时展示完整输出与冲突文件）
const conflictVisible = ref(false)
const conflictResult = ref(null)

const loadStashes = async () => {
  loading.value = true
  try {
    const result = await ListStashes(props.repoPath)
    stashes.value = result || []
  } catch (error) {
    ElMessage.error('加载 stash 列表失败: ' + (error.message || String(error)))
  } finally {
    loading.value = false
  }
}

const openPushDialog = () => {
  pushMessage.value = ''
  pushIncludeUntracked.value = false
  pushVisible.value = true
}

const submitPush = async () => {
  pushing.value = true
  try {
    await StashPush(props.repoPath, pushMessage.value.trim(), pushIncludeUntracked.value)
    ElMessage.success('已暂存工作现场')
    pushVisible.value = false
    await loadStashes()
  } catch (error) {
    handleGitError('暂存现场失败: ', error)
  } finally {
    pushing.value = false
  }
}

// 恢复（pop）：恢复变更并移除条目；pop/drop 为破坏性操作先 confirm（git 冲突时
// 默认保留条目，冲突结果经对话框如实呈现「条目已保留」）
const restoreStash = async (stash) => {
  const confirmed = await ElMessageBox.confirm(
    `恢复「${stash.description}」并从列表移除该条目？若发生冲突，条目会保留。`,
    '恢复确认',
    { confirmButtonText: '恢复', cancelButtonText: '取消', type: 'warning' }
  ).then(() => true).catch(() => false)
  if (!confirmed) return

  operatingKey.value = `pop-${stash.index}`
  try {
    const result = await StashPop(props.repoPath, stash.index)
    if (result && result.conflictFiles && result.conflictFiles.length > 0) {
      showConflictResult(result)
    } else {
      ElMessage.success('已恢复并移除 stash 条目')
    }
    await loadStashes()
  } catch (error) {
    handleGitError('恢复 stash 失败: ', error)
  } finally {
    operatingKey.value = ''
  }
}

// 应用（apply）：恢复变更但保留条目（非破坏性，不 confirm；冲突语义同恢复）
const applyStash = async (stash) => {
  operatingKey.value = `apply-${stash.index}`
  try {
    const result = await StashApply(props.repoPath, stash.index)
    if (result && result.conflictFiles && result.conflictFiles.length > 0) {
      showConflictResult(result)
    } else {
      ElMessage.success('已应用 stash 变更（条目保留）')
    }
    await loadStashes()
  } catch (error) {
    handleGitError('应用 stash 失败: ', error)
  } finally {
    operatingKey.value = ''
  }
}

// 删除（drop）：直接丢弃条目，不可恢复，必须 confirm
const dropStash = async (stash) => {
  const confirmed = await ElMessageBox.confirm(
    `删除「${stash.description}」？此操作不可恢复。`,
    '删除确认',
    { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
  ).then(() => true).catch(() => false)
  if (!confirmed) return

  operatingKey.value = `drop-${stash.index}`
  try {
    await StashDrop(props.repoPath, stash.index)
    ElMessage.success('stash 条目已删除')
    await loadStashes()
  } catch (error) {
    handleGitError('删除 stash 失败: ', error)
  } finally {
    operatingKey.value = ''
  }
}

const showConflictResult = (result) => {
  conflictResult.value = result
  conflictVisible.value = true
}

watch(() => props.repoPath, () => {
  stashes.value = []
  loadStashes()
})

loadStashes()

defineExpose({ loadStashes })
</script>

<style scoped>
.git-stash-card {
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
.stash-container {
  min-height: 60px;
  max-height: 360px;
  overflow-y: auto;
}
.stash-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}
.stash-row {
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-left: 3px solid var(--border-light);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  transition: all var(--transition-fast);
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
.stash-row:hover {
  box-shadow: var(--shadow-md);
  border-left-color: var(--primary-color);
}
.stash-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}
.stash-description {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.stash-meta {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-width: 0;
}
.stash-sha {
  font-family: Consolas, 'Courier New', monospace;
  color: var(--text-secondary);
}
.stash-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  flex-shrink: 0;
}
.conflict-files {
  margin: var(--spacing-md) 0;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}
.conflict-files-label {
  font-weight: 600;
  color: var(--text-primary);
}
.conflict-file-tag {
  max-width: 100%;
}
.conflict-output {
  margin: 0;
  padding: var(--spacing-sm) var(--spacing-md);
  max-height: 220px;
  overflow-y: auto;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
