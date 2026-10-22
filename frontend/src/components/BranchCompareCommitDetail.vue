<!-- frontend/src/components/BranchCompareCommitDetail.vue -->
<!-- 提交详情弹层（R7）：对齐表点击提交行弹出，展示该提交变更文件列表
     （GetCommitChangedFiles）；点文件查看该提交的单文件 diff（GetCommitFileDiff）
     经 BranchCompareDiffViewer 渲染。两步各自 loading，互不阻塞。 -->
<template>
  <el-dialog
    :model-value="modelValue"
    :title="dialogTitle"
    width="min(860px, 85vw)"
    append-to-body
    @update:model-value="$emit('update:modelValue', $event)"
    @closed="resetDetail"
  >
    <div class="commit-detail">
      <div class="detail-meta">
        <span class="meta-sha">{{ commit?.shortHash }}</span>
        <span class="meta-subject" :title="commit?.subject">{{ commit?.subject }}</span>
        <span class="meta-author">{{ commit?.author }} · {{ formatDateTime(commit?.date) }}</span>
      </div>
      <div class="detail-body">
        <!-- 左：变更文件列表 -->
        <div v-loading="filesLoading" class="changed-files">
          <div v-if="filesError" class="files-error">{{ filesError }}</div>
          <div v-else-if="!filesLoading && changedFiles.length === 0" class="files-empty">无变更文件</div>
          <template v-else>
            <div
              v-for="f in changedFiles"
              :key="f.path"
              class="changed-file"
              :class="{ 'is-selected': selectedFile?.path === f.path }"
              @click="selectFile(f)"
            >
              <el-tag size="small" :type="statusTagType(f.status)">{{ f.status }}</el-tag>
              <span class="changed-path" :title="f.path">{{ f.path }}</span>
            </div>
          </template>
        </div>
        <!-- 右：单文件 diff -->
        <div class="file-diff">
          <div v-if="!selectedFile" class="diff-placeholder">点击左侧文件查看该提交的差异</div>
          <BranchCompareDiffViewer
            v-else
            :diff-text="fileDiffText"
            :base-label="'父提交版本'"
            :target-label="commit?.shortHash || '该提交'"
          />
        </div>
      </div>
    </div>
    <template #footer>
      <el-button type="primary" @click="$emit('update:modelValue', false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import dayjs from 'dayjs'
import { GetCommitChangedFiles, GetCommitFileDiff } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import BranchCompareDiffViewer from './BranchCompareDiffViewer.vue'

defineOptions({ name: 'BranchCompareCommitDetail' })

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  // 仓库路径（对齐表所属 repo）
  repoPath: { type: String, default: '' },
  // 被点击的提交对象（row.a 或 row.b）：{hash, shortHash, author, email, date, subject}
  commit: { type: Object, default: null }
})

defineEmits(['update:modelValue'])

// 步骤 1：变更文件列表
const filesLoading = ref(false)
const filesError = ref('')
const changedFiles = ref([])
// 步骤 2：选中文件的单文件 diff
const selectedFile = ref(null)
const diffLoading = ref(false)
const fileDiffText = ref('')

const dialogTitle = computed(() => {
  const sha = props.commit?.shortHash || ''
  return sha ? `提交详情 - ${sha}` : '提交详情'
})

// git 短码状态 → tag 语义色（M/A/D/R/?）
const statusTagType = (status) => {
  const map = { A: 'success', D: 'danger', R: 'warning', M: 'warning', '?': 'info' }
  return map[status] || 'info'
}

const formatDateTime = (sec) => {
  if (!sec) return ''
  return dayjs.unix(sec).format('YYYY-MM-DD HH:mm')
}

// 打开即拉变更文件列表（watch modelValue：destroy-on-close 下每次打开重建数据）
watch(() => props.modelValue, async (visible) => {
  if (!visible) return
  const sha = props.commit?.hash
  if (!sha || !props.repoPath) return
  filesLoading.value = true
  filesError.value = ''
  try {
    const list = await GetCommitChangedFiles(props.repoPath, sha)
    changedFiles.value = Array.isArray(list) ? list : []
  } catch (e) {
    handleError('加载提交变更文件失败: ', e)
    filesError.value = '加载变更文件失败'
    changedFiles.value = []
  } finally {
    filesLoading.value = false
  }
})

// 点文件：拉取该提交的单文件 diff
const selectFile = async (file) => {
  const sha = props.commit?.hash
  if (!file || !sha) return
  selectedFile.value = file
  diffLoading.value = true
  fileDiffText.value = ''
  try {
    const text = await GetCommitFileDiff(props.repoPath, sha, file.path)
    fileDiffText.value = text || ''
  } catch (e) {
    handleError('加载文件差异失败: ', e)
    fileDiffText.value = ''
  } finally {
    diffLoading.value = false
  }
}

// 关闭后释放（el-dialog @closed）
const resetDetail = () => {
  changedFiles.value = []
  filesError.value = ''
  selectedFile.value = null
  fileDiffText.value = ''
}

defineExpose({ selectFile })
</script>

<style scoped>
.commit-detail {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm, 8px);
}

.detail-meta {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm, 8px);
  flex-wrap: wrap;
}

.meta-sha {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  color: var(--primary-color);
}

.meta-subject {
  font-size: 13px;
  color: var(--text-primary);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meta-author {
  font-size: 12px;
  color: var(--text-tertiary);
}

.detail-body {
  display: flex;
  gap: var(--spacing-sm, 8px);
  height: min(52vh, 480px);
}

.changed-files {
  width: 38%;
  flex-shrink: 0;
  overflow: auto;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  background: var(--bg-secondary);
  padding: var(--spacing-xs, 4px) 0;
}

.files-empty,
.files-error,
.diff-placeholder {
  padding: 24px 12px;
  text-align: center;
  color: var(--text-tertiary);
  font-size: 13px;
}

.files-error {
  color: var(--danger-color);
}

.changed-file {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  cursor: pointer;
}

.changed-file:hover {
  background: var(--primary-bg);
}

.changed-file.is-selected {
  background: var(--primary-bg);
}

.changed-path {
  flex: 1;
  min-width: 0;
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-diff {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  overflow: hidden;
}

.diff-placeholder {
  flex: 1;
}
</style>
