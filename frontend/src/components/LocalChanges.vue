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
        <!-- commit message 输入区 + AI 生成按钮 -->
        <div class="commit-input-row">
          <el-input
            v-model="commitMessage"
            type="textarea"
            :rows="2"
            placeholder="请输入提交信息（必填）"
            class="commit-input"
            resize="vertical"
          />
          <el-tooltip
            :content="hasStagedChanges ? '基于暂存区 diff 生成 Conventional Commits 提交信息候选' : '无暂存文件，请先 git add 要提交的变更'"
            placement="top"
          >
            <el-button
              :icon="MagicStick"
              size="small"
              type="primary"
              plain
              :loading="aiGenerating"
              :disabled="!hasStagedChanges || aiGenerating"
              @click="generateCommitMessage"
              class="ai-gen-btn"
            >AI 生成</el-button>
          </el-tooltip>
        </div>

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

    <!-- AI 提交信息候选弹窗 -->
    <el-dialog
      v-model="candidateDialogVisible"
      title="AI 提交信息候选"
      width="520px"
      :close-on-click-modal="false"
      append-to-body
    >
      <div v-if="aiGenerating" class="candidate-loading">
        <el-icon class="is-loading"><Loading /></el-icon>
        <span>生成中...</span>
      </div>
      <div v-else-if="candidates.length > 0">
        <div
          v-for="(c, idx) in candidates"
          :key="idx"
          class="candidate-item"
          @click="selectCandidate(c)"
        >
          <div class="candidate-head">
            <el-tag size="small" :type="getCandidateTagType(c.type)">{{ c.type }}</el-tag>
            <span v-if="c.scope" class="candidate-scope">({{ c.scope }})</span>
          </div>
          <div class="candidate-desc">{{ c.description }}</div>
        </div>
      </div>
      <el-empty v-else description="AI 输出格式异常，请手输提交信息" :image-size="60" />
      <template #footer>
        <el-button size="small" @click="candidateDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, ArrowDown, MagicStick, Loading } from '@element-plus/icons-vue'
import {
  GetLocalChanges,
  DiscardChanges,
  CommitFiles,
  PushRepo,
  HasUpstream,
  StageFiles,
  UnstageFiles,
  GetStagedDiffText,
  GetRecentCommitSubjects,
  RunAiFunction
} from '../../wailsjs/go/main/App'
// 仅引 EventsOn：用其返回的「注销本监听器」闭包（offAiTaskDone）在 onBeforeUnmount 调用，
// 精准移除本组件监听器。禁用 EventsOff('ai-task:done')——Wails v2 EventsOff 按 eventName 删全部监听器，
// 会误删 AiFunctionPanel（v-show 常驻）的 onDone，导致本组件卸载后 AI 功能面板 done 事件失效。
import { EventsOn } from '../../wailsjs/runtime/runtime'
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

// AI 生成提交信息状态
const aiGenerating = ref(false)
const candidates = ref([])
const candidateDialogVisible = ref(false)
let currentAiTaskId = null
// EventsOn 返回的注销闭包：onBeforeUnmount 调用，精准移除本组件的 ai-task:done 监听器（不波及 AiFunctionPanel）
let offAiTaskDone = null

// 有无暂存文件：changes 列表筛 staged 非空（驱动 AI 生成按钮启用态）
const hasStagedChanges = computed(() => changes.value.some(c => c.staged))

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

// AI 生成提交信息：取暂存区聚合 diff + 历史 few-shot → RunAiFunction('commit-message')
// done 事件经 onAiTaskDone 取 result.structuredOutput.candidates 渲染候选列表。
// 空暂存返回 AppError{E_GIT_NO_STAGED_CHANGES}，handleGitError 按 code warning 提示。
const generateCommitMessage = async () => {
  if (!hasStagedChanges.value || aiGenerating.value) return
  aiGenerating.value = true
  candidates.value = []
  candidateDialogVisible.value = true
  try {
    const diffText = await GetStagedDiffText(props.repoPath)
    const subjects = await GetRecentCommitSubjects(props.repoPath, 3)
    const history = (subjects || []).join('\n')
    currentAiTaskId = await RunAiFunction('commit-message', { diff: diffText, history })
  } catch (error) {
    aiGenerating.value = false
    candidateDialogVisible.value = false
    currentAiTaskId = null
    handleGitError('AI 生成失败: ', error)
  }
}

// done 事件处理：taskId 匹配后取 structuredOutput.candidates 渲染候选列表。
// 全局常驻监听（onMounted 注册），多组件共存各自 taskId 过滤互不干扰。
const onAiTaskDone = (result) => {
  if (!currentAiTaskId || result.taskId !== currentAiTaskId) return
  aiGenerating.value = false
  currentAiTaskId = null
  if (result.error || result.canceled) {
    candidateDialogVisible.value = false
    ElMessage.error('AI 生成失败: ' + (result.error || '已取消'))
    return
  }
  const structured = result.structuredOutput
  if (structured && Array.isArray(structured.candidates) && structured.candidates.length > 0) {
    candidates.value = structured.candidates
  } else {
    // structuredOutput 为空（解析失败或未配 OutputSchema）→ 降级空列表，el-empty 提示手输
    candidates.value = []
  }
}

// 候选点击填入提交框：拼 type(scope): description，关闭弹窗，用户可编辑后走现有 CommitFiles。
const selectCandidate = (c) => {
  const scope = c.scope ? '(' + c.scope + ')' : ''
  commitMessage.value = `${c.type}${scope}: ${c.description}`
  candidateDialogVisible.value = false
}

// 候选 type 标签颜色：feat/fix/perf 主色区分，其余 primary
const getCandidateTagType = (type) => {
  if (type === 'feat') return 'success'
  if (type === 'fix') return 'danger'
  if (type === 'perf') return 'warning'
  return 'primary'
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
  offAiTaskDone = EventsOn('ai-task:done', onAiTaskDone)
})

onBeforeUnmount(() => {
  // 调 EventsOn 返回的注销闭包，仅移除本组件监听器（Wails EventsOff 会清同名全部监听器，误伤 AiFunctionPanel）
  if (offAiTaskDone) {
    offAiTaskDone()
    offAiTaskDone = null
  }
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

.commit-input-row {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.commit-input {
  flex: 1;
  min-width: 0;
}

.commit-input :deep(.el-textarea__inner) {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
}

.ai-gen-btn {
  flex-shrink: 0;
}

.candidate-loading {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 0;
  color: var(--text-secondary);
}

.candidate-item {
  padding: 10px 12px;
  border: 1px solid var(--border-color);
  border-radius: 4px;
  margin-bottom: 8px;
  cursor: pointer;
  transition: border-color 0.2s, background-color 0.2s;
}

.candidate-item:hover {
  border-color: var(--el-color-primary);
  background-color: var(--el-color-primary-light-9, #ecf5ff);
}

.candidate-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}

.candidate-scope {
  font-size: 13px;
  color: var(--text-secondary);
}

.candidate-desc {
  font-size: 14px;
  color: var(--text-primary);
  word-break: break-all;
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
