<template>
  <div class="content-panel">
    <div v-if="selectedNode" style="padding: 16px;">
      <h2>{{ selectedNode.name }}</h2>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="路径">{{ selectedNode.path }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ selectedNode.type === 'directory' ? '文件夹' : '文件' }}</el-descriptions-item>
      </el-descriptions>

      <!-- Git 拉取更新按钮 -->
      <div v-if="selectedNode.isGitRepo" style="margin-top: 10px;">
        <el-button type="primary" @click="pullRepo" :loading="gitLoading">
          拉取更新
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
      </el-tabs>

      <div v-else-if="selectedNode.type === 'directory'" style="margin-top: 12px;">
        <h3>文件夹操作</h3>
        <div style="display: flex; flex-direction: column; gap: 10px;">
          <!-- 基本操作 -->
          <div>
            <span style="font-size: 12px; color: #909399; margin-bottom: 3px; display: block;">基本操作</span>
            <el-button-group>
              <el-button @click="$emit('cut', selectedNode)">剪切</el-button>
              <el-button @click="$emit('copy', selectedNode)">复制</el-button>
              <el-button :disabled="!clipboard.mode" @click="$emit('paste', selectedNode)">粘贴</el-button>
              <el-button @click="$emit('copyTo', selectedNode)">拷贝到</el-button>
            </el-button-group>
          </div>
          <!-- 编辑操作 -->
          <div>
            <span style="font-size: 12px; color: #909399; margin-bottom: 3px; display: block;">编辑操作</span>
            <el-button-group>
              <el-button @click="$emit('createDirectory', selectedNode)">新建文件夹</el-button>
              <el-button @click="$emit('createFile', selectedNode)">新建文件</el-button>
              <el-button @click="$emit('rename', selectedNode)">重命名</el-button>
              <el-button type="danger" @click="$emit('delete', selectedNode)">删除</el-button>
            </el-button-group>
          </div>
          <!-- 查看操作 -->
          <div>
            <span style="font-size: 12px; color: #909399; margin-bottom: 3px; display: block;">查看操作</span>
            <el-button-group>
              <el-button @click="handleCopyPath">复制路径</el-button>
              <el-button @click="handleOpenInExplorer">打开资源管理器</el-button>
              <el-button @click="handleOpenInVSCode">用 VSCode 打开</el-button>
              <el-button @click="handleOpenInWarp">用 Warp 打开</el-button>
            </el-button-group>
          </div>
          <!-- 高级操作 -->
          <div>
            <span style="font-size: 12px; color: #909399; margin-bottom: 3px; display: block;">高级操作</span>
            <el-button-group>
              <el-button type="success" @click="showCloneDialog">克隆仓库</el-button>
              <el-button @click="handleUpdateRepos">更新仓库</el-button>
              <el-button @click="handleRefresh">刷新</el-button>
            </el-button-group>
          </div>
        </div>
      </div>

      <div v-else-if="selectedNode.type === 'file'" style="margin-top: 12px; display: flex; flex-direction: column; flex: 1;">
        <h3>文件操作</h3>
        <div style="display: flex; flex-direction: column; gap: 10px;">
          <!-- 基本操作 -->
          <div>
            <span style="font-size: 12px; color: #909399; margin-bottom: 3px; display: block;">基本操作</span>
            <el-button-group>
              <el-button @click="$emit('cut', selectedNode)">剪切</el-button>
              <el-button @click="$emit('copy', selectedNode)">复制</el-button>
              <el-button :disabled="!clipboard.mode" @click="$emit('paste', selectedNode)">粘贴</el-button>
              <el-button @click="$emit('copyTo', selectedNode)">拷贝到</el-button>
            </el-button-group>
          </div>
          <!-- 编辑操作 -->
          <div>
            <span style="font-size: 12px; color: #909399; margin-bottom: 3px; display: block;">编辑操作</span>
            <el-button-group>
              <el-button type="primary" @click="handleOpenWithDefaultApp">打开</el-button>
              <el-button @click="previewFile">预览</el-button>
              <el-button @click="$emit('rename', selectedNode)">重命名</el-button>
              <el-button type="danger" @click="$emit('delete', selectedNode)">删除</el-button>
            </el-button-group>
          </div>
          <!-- 查看操作 -->
          <div>
            <span style="font-size: 12px; color: #909399; margin-bottom: 3px; display: block;">查看操作</span>
            <el-button-group>
              <el-button @click="handleCopyPath">复制路径</el-button>
              <el-button @click="handleCopyName">复制文件名</el-button>
              <el-button @click="handleOpenInExplorer">打开资源管理器</el-button>
              <el-button @click="handleOpenInVSCode">用 VSCode 打开</el-button>
              <el-button @click="handleOpenInWarp">用 Warp 打开</el-button>
            </el-button-group>
          </div>
        </div>

        <div v-if="filePreview.content" style="margin-top: 12px; display: flex; flex-direction: column; flex: 1;">
          <h4 style="margin-bottom: 6px;">文件内容</h4>
          <el-input
            v-model="filePreview.content"
            type="textarea"
            :rows="15"
            readonly
            :style="{
              fontFamily: 'monospace',
              height: '100%',
              minHeight: '200px',
              resize: 'vertical'
            }"
            class="preview-textarea"
          />
        </div>
      </div>
    </div>
    <el-empty v-else description="请从左侧选择文件或文件夹" />

    <!-- 克隆仓库对话框 -->
    <el-dialog
      v-model="cloneDialogVisible"
      title="克隆仓库"
      width="500px"
    >
      <el-form label-width="100px">
        <el-form-item label="目标文件夹">
          <el-input :model-value="selectedNode?.path" disabled />
        </el-form-item>
        <el-form-item label="Git 地址">
          <el-input
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

    <!-- 更新仓库进度弹窗 -->
    <el-dialog
      v-model="pullDialogVisible"
      :title="pullCompleted ? '更新完成' : '更新仓库'"
      width="700px"
      :close-on-click-modal="false"
      :close-on-press-escape="!pullCompleted"
      :show-close="pullCompleted"
    >
      <div style="margin-bottom: 16px;">
        <el-progress
          :percentage="pullProgress.total > 0 ? Math.round(pullProgress.current / pullProgress.total * 100) : 0"
          :format="() => `${pullProgress.current} / ${pullProgress.total}`"
          :status="pullCompleted ? (pullSummary.failed > 0 ? 'warning' : 'success') : undefined"
        />
        <div v-if="pullCompleted" style="margin-top: 8px; color: #909399; font-size: 13px;">
          成功: {{ pullSummary.success }}，失败: {{ pullSummary.failed }}
        </div>
      </div>

      <el-table :data="pullResults" style="width: 100%" max-height="400" size="small">
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
            <span v-if="row.success" style="color: #67C23A;">{{ row.output || '已是最新' }}</span>
            <span v-else style="color: #F56C6C;">{{ row.error }}</span>
          </template>
        </el-table-column>
      </el-table>

      <template #footer>
        <el-button
          type="primary"
          @click="pullDialogVisible = false"
          :disabled="!pullCompleted"
        >
          {{ pullCompleted ? '关闭' : '更新中...' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onBeforeUnmount, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { SuccessFilled, CircleCloseFilled } from '@element-plus/icons-vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import GitInfo from './GitInfo.vue'
import CommitHistory from './CommitHistory.vue'
import {
  PreviewFile, PullRepo, CloneRepo, OpenWithDefaultApp,
  OpenInExplorer, OpenInVSCode, OpenInWarp
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

const pullDialogVisible = ref(false)
const pullProgress = reactive({ current: 0, total: 0 })
const pullResults = ref([])
const pullCompleted = ref(false)
const pullSummary = reactive({ success: 0, failed: 0 })

const isWailsRuntime = () => !!window.runtime

const onLatestCommit = (commit) => {
  emit('latestCommit', commit)
}

const pullRepo = async () => {
  if (!props.selectedNode) return

  gitLoading.value = true
  try {
    const result = await PullRepo(props.selectedNode.path)
    ElMessage.success(result || '拉取完成')
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

// 监听选中节点变化，自动预览文件内容
watch(() => props.selectedNode, async (newNode) => {
  if (newNode && newNode.type === 'file') {
    await previewFile()
  }
})

const showCloneDialog = () => {
  cloneUrl.value = ''
  cloneDialogVisible.value = true
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
  pullDialogVisible.value = true
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
}

/* 内容区域容器 */
.content-panel > div:first-child {
  padding: var(--spacing-lg);
  animation: fadeIn var(--transition-normal);
}

/* 标题样式 */
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

/* 按钮组样式 */
.el-button-group {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
}

/* 操作按钮容器 */
.content-panel > div:first-child > div:not(.el-tabs):not(.el-descriptions):not(.el-divider) {
  background: var(--bg-tertiary);
  padding: 12px;
  border-radius: var(--radius-md);
  margin-top: 10px;
  border: 1px solid var(--border-color);
  transition: all var(--transition-normal);
}

.content-panel > div:first-child > div:not(.el-tabs):not(.el-descriptions):not(.el-divider):hover {
  box-shadow: var(--shadow-sm);
  border-color: var(--primary-light);
}

/* 文件预览区域 */
.preview-textarea {
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-color);
  transition: all var(--transition-normal);
}

.preview-textarea:hover {
  border-color: var(--primary-light);
  box-shadow: var(--shadow-sm);
}

/* 标签页样式 */
.el-tabs {
  margin-top: 10px;
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

/* 空状态样式 */
:deep(.el-empty) {
  margin-top: 50px;
}

:deep(.el-empty__description) {
  color: var(--text-tertiary);
  font-size: 14px;
}
</style>
