<template>
  <div class="content-panel">
    <div v-if="selectedNode" class="content-inner">
      <h2>{{ selectedNode.name }}</h2>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="路径">{{ selectedNode.path }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ selectedNode.type === 'directory' ? '文件夹' : '文件' }}</el-descriptions-item>
      </el-descriptions>

      <!-- Git 操作按钮 -->
      <div v-if="selectedNode.isGitRepo" class="git-actions">
        <el-button type="primary" @click="pullRepo" :loading="gitLoading">
          拉取更新
        </el-button>
        <el-button @click="showBranchDialog" :loading="branchLoading">
          切换分支
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
            @latest-commit="onLatestCommit"
          />
        </el-tab-pane>
        <el-tab-pane label="本地变动" name="changes" lazy>
          <LocalChanges
            ref="localChangesRef"
            :repo-path="selectedNode.path"
          />
        </el-tab-pane>
      </el-tabs>

      <div v-else-if="selectedNode.type === 'directory'" class="node-actions">
        <h3>文件夹操作</h3>
        <div class="action-groups">
          <div>
            <span class="action-label">基本操作</span>
            <el-button-group>
              <el-button @click="$emit('cut', selectedNode)">剪切</el-button>
              <el-button @click="$emit('copy', selectedNode)">复制</el-button>
              <el-button :disabled="!clipboard.mode" @click="$emit('paste', selectedNode)">粘贴</el-button>
              <el-button @click="$emit('copyTo', selectedNode)">拷贝到</el-button>
            </el-button-group>
          </div>
          <div>
            <span class="action-label">编辑操作</span>
            <el-button-group>
              <el-button @click="$emit('createDirectory', selectedNode)">新建文件夹</el-button>
              <el-button @click="$emit('createFile', selectedNode)">新建文件</el-button>
              <el-button @click="$emit('rename', selectedNode)">重命名</el-button>
              <el-button type="danger" @click="$emit('delete', selectedNode)">删除</el-button>
            </el-button-group>
          </div>
          <div>
            <span class="action-label">查看操作</span>
            <el-button-group>
              <el-button @click="handleCopyPath">复制路径</el-button>
              <el-button @click="handleOpenInExplorer">打开资源管理器</el-button>
              <el-button @click="handleOpenInVSCode">用 VSCode 打开</el-button>
              <el-button @click="handleOpenInWarp">用 Warp 打开</el-button>
            </el-button-group>
          </div>
          <div>
            <span class="action-label">高级操作</span>
            <el-button-group>
              <el-button type="success" @click="showCloneDialog">克隆仓库</el-button>
              <el-button @click="handleUpdateRepos">更新仓库</el-button>
              <el-button @click="handleRefresh">刷新</el-button>
            </el-button-group>
          </div>
        </div>
      </div>

      <div v-else-if="selectedNode.type === 'file'" class="node-actions node-actions--file">
        <h3>文件操作</h3>
        <div class="action-groups">
          <div>
            <span class="action-label">基本操作</span>
            <el-button-group>
              <el-button @click="$emit('cut', selectedNode)">剪切</el-button>
              <el-button @click="$emit('copy', selectedNode)">复制</el-button>
              <el-button :disabled="!clipboard.mode" @click="$emit('paste', selectedNode)">粘贴</el-button>
              <el-button @click="$emit('copyTo', selectedNode)">拷贝到</el-button>
            </el-button-group>
          </div>
          <div>
            <span class="action-label">编辑操作</span>
            <el-button-group>
              <el-button type="primary" @click="handleOpenWithDefaultApp">打开</el-button>
              <el-button @click="previewFile">预览</el-button>
              <el-button @click="$emit('rename', selectedNode)">重命名</el-button>
              <el-button type="danger" @click="$emit('delete', selectedNode)">删除</el-button>
            </el-button-group>
          </div>
          <div>
            <span class="action-label">查看操作</span>
            <el-button-group>
              <el-button @click="handleCopyPath">复制路径</el-button>
              <el-button @click="handleCopyName">复制文件名</el-button>
              <el-button @click="handleOpenInExplorer">打开资源管理器</el-button>
              <el-button @click="handleOpenInVSCode">用 VSCode 打开</el-button>
              <el-button @click="handleOpenInWarp">用 Warp 打开</el-button>
            </el-button-group>
          </div>
        </div>

        <div v-if="filePreview.content" class="file-preview">
          <h4>文件内容</h4>
          <el-input
            v-model="filePreview.content"
            type="textarea"
            :rows="15"
            readonly
            class="preview-textarea"
          />
        </div>
      </div>
    </div>
    <div v-else class="empty-state">
      <div class="empty-state-icon">
        <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
          <line x1="12" y1="11" x2="12" y2="17"/>
          <line x1="9" y1="14" x2="15" y2="14"/>
        </svg>
      </div>
      <p class="empty-state-title">请从左侧选择文件或文件夹</p>
      <p class="empty-state-hint">在文件树中点击项目查看详情和操作</p>
    </div>

    <!-- 切换分支对话框 -->
    <el-dialog
      v-model="branchDialogVisible"
      title="切换分支"
      width="480px"
      append-to-body
    >
      <div class="branch-info">
        当前分支：<span class="branch-name">{{ currentBranchName }}</span>
      </div>
      <el-select
        ref="branchSelectRef"
        v-model="selectedBranch"
        placeholder="搜索并选择分支"
        filterable
        class="branch-select"
        :disabled="switchingBranch"
      >
        <el-option-group label="本地分支">
          <el-option
            v-for="b in localBranches"
            :key="b.name"
            :label="b.name"
            :value="b.name"
            :disabled="b.isCurrent"
          />
        </el-option-group>
        <el-option-group v-if="remoteBranches.length > 0" label="远程分支">
          <el-option
            v-for="b in remoteBranches"
            :key="b.name"
            :label="b.name"
            :value="b.name"
            :disabled="b.isCurrent"
          />
        </el-option-group>
      </el-select>
      <template #footer>
        <el-button @click="branchDialogVisible = false" :disabled="switchingBranch">取消</el-button>
        <el-button
          type="primary"
          @click="doCheckout"
          :loading="switchingBranch"
          :disabled="!selectedBranch || selectedBranch === currentBranchName"
        >
          切换
        </el-button>
      </template>
    </el-dialog>

    <!-- 克隆仓库对话框 -->
    <el-dialog
      v-model="cloneDialogVisible"
      title="克隆仓库"
      width="500px"
      append-to-body
    >
      <el-form label-width="100px">
        <el-form-item label="目标文件夹">
          <el-input :model-value="selectedNode?.path" disabled />
        </el-form-item>
        <el-form-item label="Git 地址">
          <el-input
            ref="cloneInputRef"
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

    <!-- 单仓库拉取结果弹窗 -->
    <el-dialog
      v-model="singlePullVisible"
      title="拉取结果"
      width="600px"
      append-to-body
    >
      <div class="pull-result-output">{{ singlePullResult }}</div>
      <template #footer>
        <el-button type="primary" @click="singlePullVisible = false">关闭</el-button>
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
      append-to-body
    >
      <div class="pull-progress-section">
        <el-progress
          :percentage="pullProgress.total > 0 ? Math.round(pullProgress.current / pullProgress.total * 100) : 0"
          :format="() => `${pullProgress.current} / ${pullProgress.total}`"
          :status="pullCompleted ? (pullSummary.failed > 0 ? 'warning' : 'success') : undefined"
        />
        <div v-if="pullCompleted" class="pull-summary">
          成功: {{ pullSummary.success }}，失败: {{ pullSummary.failed }}
        </div>
      </div>

      <el-table :data="pullResults" class="pull-table" max-height="400" size="small">
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
            <span :class="row.success ? 'text-success' : 'text-danger'">{{ row.success ? (row.output || '已是最新') : row.error }}</span>
          </template>
        </el-table-column>
      </el-table>

      <template #footer>
        <el-button
          v-if="!pullCompleted"
          @click="pullRunningInBackground = true; pullDialogVisible = false"
        >
          后台运行
        </el-button>
        <el-button
          type="primary"
          @click="pullDialogVisible = false"
          :disabled="!pullCompleted"
        >
          {{ pullCompleted ? '关闭' : '更新中...' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 底部后台运行状态栏 -->
    <transition name="slide-up">
      <div v-if="pullRunningInBackground" class="pull-status-bar" @click="onStatusBarClick">
        <div class="pull-status-left">
          <span v-if="!pullCompleted" class="pull-status-pulse" />
          <el-icon v-else :size="14" color="#67C23A"><SuccessFilled /></el-icon>
          <span class="pull-status-text">
            <template v-if="!pullCompleted">正在更新 {{ pullProgress.current }}/{{ pullProgress.total }}</template>
            <template v-else>更新完成（{{ pullSummary.success }} 成功<template v-if="pullSummary.failed > 0">，{{ pullSummary.failed }} 失败</template>）</template>
          </span>
        </div>
        <el-progress
          v-if="!pullCompleted"
          :percentage="pullProgress.total > 0 ? Math.round(pullProgress.current / pullProgress.total * 100) : 0"
          :stroke-width="4"
          :show-text="false"
          class="pull-status-progress"
        />
        <span v-else class="pull-status-view">查看详情</span>
      </div>
    </transition>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onBeforeUnmount, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { SuccessFilled, CircleCloseFilled } from '@element-plus/icons-vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import GitInfo from './GitInfo.vue'
import CommitHistory from './CommitHistory.vue'
import LocalChanges from './LocalChanges.vue'
import {
  PreviewFile, PullRepo, CloneRepo, OpenWithDefaultApp,
  OpenInExplorer, OpenInVSCode, OpenInWarp,
  GetBranches, CheckoutBranch
} from '../../wailsjs/go/main/App'

const props = defineProps({
  selectedNode: {
    type: Object,
    default: null
  },
  latestCommit: {
    type: Object,
    default: null
  },
  clipboard: { type: Object, default: () => ({ mode: null }) }
})

const emit = defineEmits([
  'latestCommit',
  'refreshNode',
  'createDirectory',
  'createFile',
  'rename',
  'delete',
  'copy',
  'cut',
  'paste',
  'copyTo',
  'batchPull'
])

const gitLoading = ref(false)
const activeGitTab = ref('repo')
const gitInfoRef = ref()
const commitHistoryRef = ref()

const filePreview = ref({
  content: '',
  error: ''
})

const cloneDialogVisible = ref(false)
const cloneUrl = ref('')
const cloneLoading = ref(false)
const cloneInputRef = ref()

const pullDialogVisible = ref(false)
const pullProgress = reactive({ current: 0, total: 0 })
const pullResults = ref([])
const pullCompleted = ref(false)
const pullSummary = reactive({ success: 0, failed: 0 })
const pullRunningInBackground = ref(false)

const singlePullVisible = ref(false)
const singlePullResult = ref('')

const branchDialogVisible = ref(false)
const branchLoading = ref(false)
const switchingBranch = ref(false)
const branchList = ref([])
const selectedBranch = ref('')
const currentBranchName = ref('')
const localBranches = computed(() => branchList.value.filter(b => !b.isRemote))
const remoteBranches = computed(() => branchList.value.filter(b => b.isRemote))
const branchSelectRef = ref()

const isWailsRuntime = () => !!window.runtime

const onLatestCommit = (commit) => {
  emit('latestCommit', commit)
}

const showBranchDialog = async () => {
  if (!props.selectedNode) return

  branchLoading.value = true
  branchDialogVisible.value = true
  selectedBranch.value = ''

  try {
    const result = await GetBranches(props.selectedNode.path)
    branchList.value = result.branches || []
    const current = branchList.value.find(b => b.isCurrent)
    currentBranchName.value = current ? current.name : ''
    nextTick(() => {
      branchSelectRef.value?.focus()
    })
  } catch (error) {
    ElMessage.error('获取分支列表失败: ' + (error.message || String(error)))
  } finally {
    branchLoading.value = false
  }
}

const doCheckout = async () => {
  if (!props.selectedNode || !selectedBranch.value) return

  const branch = branchList.value.find(b => b.name === selectedBranch.value)
  if (!branch) return

  switchingBranch.value = true
  try {
    await CheckoutBranch(props.selectedNode.path, selectedBranch.value, branch.isRemote)
    ElMessage.success('已切换到分支: ' + selectedBranch.value)
    branchDialogVisible.value = false
    gitInfoRef.value?.handleRefresh()
    commitHistoryRef.value?.handleRefresh()
  } catch (error) {
    ElMessage.error('切换分支失败: ' + (error.message || String(error)))
  } finally {
    switchingBranch.value = false
  }
}

const pullRepo = async () => {
  if (!props.selectedNode) return

  gitLoading.value = true
  try {
    const result = await PullRepo(props.selectedNode.path)
    if (result && result.length > 200) {
      singlePullResult.value = result
      singlePullVisible.value = true
    } else {
      ElMessage.success(result || '拉取完成')
    }
    gitInfoRef.value?.handleRefresh()
    commitHistoryRef.value?.handleRefresh()
  } catch (error) {
    ElMessage.error('拉取失败: ' + (error.message || String(error)))
  } finally {
    gitLoading.value = false
  }
}

const handleOpenWithDefaultApp = async () => {
  if (!props.selectedNode || props.selectedNode.type !== 'file') return
  try {
    const result = await OpenWithDefaultApp(props.selectedNode.path)
    if (!result) {
      ElMessage.error('打开文件失败')
    }
  } catch (error) {
    ElMessage.error('打开文件失败: ' + (error.message || String(error)))
  }
}

const handleOpenInExplorer = async () => {
  if (!props.selectedNode) return
  try {
    const result = await OpenInExplorer(props.selectedNode.path)
    if (!result) {
      ElMessage.error('打开资源管理器失败')
    }
  } catch (error) {
    ElMessage.error('打开资源管理器失败: ' + (error.message || String(error)))
  }
}

const handleOpenInVSCode = async () => {
  if (!props.selectedNode) return
  try {
    const result = await OpenInVSCode(props.selectedNode.path)
    if (!result) {
      ElMessage.error('打开 VSCode 失败，请确认已安装 VSCode 并将 code 命令加入 PATH')
    }
  } catch (error) {
    ElMessage.error('打开 VSCode 失败: ' + (error.message || String(error)))
  }
}

const handleOpenInWarp = async () => {
  if (!props.selectedNode) return
  try {
    const result = await OpenInWarp(props.selectedNode.path)
    if (!result) {
      ElMessage.error('打开 Warp 失败，请确认已安装 Warp 终端')
    }
  } catch (error) {
    ElMessage.error('打开 Warp 失败: ' + (error.message || String(error)))
  }
}

const handleCopyPath = async () => {
  if (!props.selectedNode) return
  try {
    await navigator.clipboard.writeText(props.selectedNode.path.replaceAll('\\', '/'))
    ElMessage.success('路径已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

const handleCopyName = async () => {
  if (!props.selectedNode || props.selectedNode.type !== 'file') return
  try {
    await navigator.clipboard.writeText(props.selectedNode.name)
    ElMessage.success('文件名已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

const handleRefresh = () => {
  emit('refreshNode', props.selectedNode.path)
}

const handleUpdateRepos = () => {
  emit('batchPull', props.selectedNode)
}

const previewFile = async () => {
  if (!props.selectedNode) return

  const preview = await PreviewFile(props.selectedNode.path)
  filePreview.value = preview

  if (preview.error) {
    ElMessage.error('预览失败: ' + preview.error)
  } else if (preview.tooLarge) {
    ElMessage.warning('文件过大，无法预览')
  } else if (preview.isBinary) {
    ElMessage.warning('二进制文件，无法预览')
  }
}

watch(() => props.selectedNode, async (newNode) => {
  if (newNode && newNode.type === 'file') {
    await previewFile()
  }
})

const showCloneDialog = () => {
  cloneUrl.value = ''
  cloneDialogVisible.value = true
  setTimeout(() => {
    const input = cloneInputRef.value?.input
    if (input) {
      input.focus()
    }
  }, 100)
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

const startBatchPull = (summary) => {
  pullResults.value = []
  pullProgress.current = 0
  pullProgress.total = summary.total
  pullCompleted.value = false
  pullSummary.success = 0
  pullSummary.failed = 0
  pullRunningInBackground.value = false
  pullDialogVisible.value = true
}

const onStatusBarClick = () => {
  if (pullCompleted.value) {
    pullRunningInBackground.value = false
    pullDialogVisible.value = true
  }
}

const clearPreview = () => {
  filePreview.value = {
    content: '',
    error: ''
  }
}

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
    if (pullRunningInBackground.value) {
      pullRunningInBackground.value = false
      pullDialogVisible.value = true
    }
  })
}

setupPullEvents()

onBeforeUnmount(() => {
  cleanupPullEvents()
})

defineExpose({
  startBatchPull,
  clearPreview
})
</script>

<style scoped>
.content-panel {
  background-color: var(--bg-secondary);
  height: 100%;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  position: relative;
}

.content-inner {
  padding: var(--spacing-lg);
  animation: fadeIn var(--transition-normal);
}

.content-panel h2 {
  color: var(--text-primary);
  font-size: 20px;
  font-weight: 700;
  margin-bottom: 10px;
  letter-spacing: 0.5px;
}

.content-panel h3 {
  color: var(--text-primary);
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 10px;
  padding-bottom: 6px;
  border-bottom: 2px solid var(--border-color);
}

.content-panel h4 {
  color: var(--text-primary);
  font-size: 16px;
  font-weight: 500;
  margin-bottom: var(--spacing-sm);
}

/* Git 操作区 */
.git-actions {
  margin-top: var(--spacing-sm);
}

/* 操作区域 */
.node-actions {
  margin-top: var(--spacing-sm);
  background: var(--bg-tertiary);
  padding: var(--spacing-md);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-color);
  transition: all var(--transition-normal);
}

.node-actions:hover {
  box-shadow: var(--shadow-sm);
  border-color: var(--primary-light);
}

.node-actions--file {
  display: flex;
  flex-direction: column;
  flex: 1;
}

.action-groups {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.action-label {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-bottom: 3px;
  display: block;
  letter-spacing: 0.3px;
  text-transform: uppercase;
  font-weight: 500;
}

/* 按钮组样式 */
.el-button-group {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
}

/* 文件预览区域 */
.file-preview {
  margin-top: var(--spacing-sm);
  display: flex;
  flex-direction: column;
  flex: 1;
}

.preview-textarea {
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-color);
  transition: all var(--transition-normal);
  font-family: Consolas, 'Courier New', monospace;
  min-height: 200px;
  height: 100%;
  resize: vertical;
}

.preview-textarea:hover {
  border-color: var(--primary-light);
  box-shadow: var(--shadow-sm);
}

/* 分支对话框 */
.branch-info {
  margin-bottom: var(--spacing-md);
  font-size: 13px;
  color: var(--text-tertiary);
}

.branch-name {
  color: var(--text-primary);
  font-weight: 600;
}

.branch-select {
  width: 100%;
}

/* 拉取结果 */
.pull-result-output {
  max-height: 400px;
  overflow-y: auto;
  padding: var(--spacing-md);
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
}

.pull-progress-section {
  margin-bottom: var(--spacing-md);
}

.pull-summary {
  margin-top: var(--spacing-sm);
  color: var(--text-tertiary);
  font-size: 13px;
}

.pull-table {
  width: 100%;
}

.text-success {
  color: var(--success-color);
}

.text-danger {
  color: var(--danger-color);
}

/* 标签页样式 */
.el-tabs {
  margin-top: var(--spacing-sm);
}

.el-tabs__header {
  margin-bottom: var(--spacing-md);
}

.el-tabs__nav-wrap {
  border-bottom: 2px solid var(--border-color);
}

.el-tabs__item {
  color: var(--text-secondary);
  font-weight: 500;
  border-bottom: 2px solid transparent;
  transition: all var(--transition-normal);
}

.el-tabs__item:hover {
  color: var(--primary-color);
}

.el-tabs__item.is-active {
  color: var(--primary-color);
  font-weight: 600;
}

/* 表格样式 */
.el-table {
  background: var(--bg-secondary);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.el-table :deep(.el-table__cell) {
  padding: var(--spacing-sm) var(--spacing-md);
}

.el-table :deep(.el-table__row:hover) {
  background-color: var(--bg-tertiary);
}

/* 描述列表样式 */
.el-descriptions {
  margin-bottom: var(--spacing-md);
}

.el-descriptions__table {
  width: 100%;
}

.el-descriptions__label {
  font-weight: 600;
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

.el-descriptions__content {
  color: var(--text-secondary);
}

/* 进度条样式 */
.el-progress {
  margin-bottom: var(--spacing-md);
}

.el-progress__text {
  font-size: 14px;
  color: var(--text-tertiary);
}

/* 对话框样式覆盖 */
:deep(.el-dialog) {
  border-radius: var(--radius-lg);
  overflow: hidden;
}

:deep(.el-dialog__header) {
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--border-color);
}

:deep(.el-dialog__title) {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

:deep(.el-dialog__body) {
  padding: var(--spacing-lg);
  background: var(--bg-secondary);
}

:deep(.el-dialog__footer) {
  padding: var(--spacing-md) var(--spacing-lg);
  background: var(--bg-tertiary);
  border-top: 1px solid var(--border-color);
}

/* 自定义空状态 */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: var(--spacing-xl);
  animation: fadeIn var(--transition-normal);
}

.empty-state-icon {
  color: var(--border-light);
  margin-bottom: var(--spacing-lg);
  transition: color var(--transition-normal);
}

.empty-state:hover .empty-state-icon {
  color: var(--primary-light);
}

.empty-state-title {
  font-size: 15px;
  color: var(--text-secondary);
  margin: 0 0 var(--spacing-xs) 0;
  font-weight: 500;
}

.empty-state-hint {
  font-size: 13px;
  color: var(--text-tertiary);
  margin: 0;
}

/* 底部后台运行状态栏 */
.pull-status-bar {
  position: sticky;
  bottom: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 16px;
  background: var(--primary-bg);
  border-top: 1px solid var(--border-color);
  cursor: pointer;
  transition: all var(--transition-fast);
}
.pull-status-bar:hover {
  background: linear-gradient(135deg, var(--primary-bg) 0%, #d6ecfa 100%);
  box-shadow: 0 -2px 8px rgba(64, 158, 255, 0.1);
}
.pull-status-left {
  display: flex;
  align-items: center;
  gap: 8px;
}
.pull-status-pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--primary-color);
  animation: status-pulse 1.5s ease-in-out infinite;
}
@keyframes status-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.8); }
}
.pull-status-text {
  font-size: 13px;
  color: var(--text-secondary);
  white-space: nowrap;
}
.pull-status-progress {
  width: 100px;
  flex-shrink: 0;
}
.pull-status-view {
  font-size: 12px;
  color: var(--primary-color);
  font-weight: 500;
  white-space: nowrap;
}
.slide-up-enter-active,
.slide-up-leave-active {
  transition: all 0.3s ease;
}
.slide-up-enter-from,
.slide-up-leave-to {
  transform: translateY(100%);
  opacity: 0;
}
</style>
