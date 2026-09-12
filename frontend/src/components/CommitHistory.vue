<template>
  <el-card class="commit-history-card" shadow="hover">
    <template #header>
      <div class="card-header">
        <span>提交历史</span>
        <div class="header-actions">
          <el-input
            v-model="searchKeyword"
            placeholder="搜索提交..."
            prefix-icon="Search"
            size="small"
            class="search-input"
            clearable
            @update:model-value="handleSearch"
          />
          <el-button
            v-if="selectedShas.length === 2"
            type="primary"
            size="small"
            class="compare-btn"
            @click="openRangeDiff"
          >
            对比选中 (2)
          </el-button>
          <el-button
            v-else
            size="small"
            class="compare-btn"
            disabled
            :title="selectedShas.length < 2 ? '选择两个提交进行对比' : ''"
          >
            对比选中 ({{ selectedShas.length }}/2)
          </el-button>
          <el-button
            :icon="Refresh"
            circle
            size="small"
            @click="handleRefresh"
            :loading="loading"
          />
        </div>
      </div>
    </template>

    <div class="filter-bar">
      <el-input
        v-model="filter.author"
        placeholder="作者"
        size="small"
        class="filter-input"
        clearable
        @update:model-value="applyFilter"
      />
      <el-date-picker
        v-model="filter.dateRange"
        type="daterange"
        size="small"
        range-separator="至"
        start-placeholder="开始日期"
        end-placeholder="结束日期"
        value-format="YYYY-MM-DD"
        class="filter-date"
        @change="applyFilter"
      />
      <el-input
        v-model="filter.filePath"
        placeholder="文件路径"
        size="small"
        class="filter-input"
        clearable
        @update:model-value="applyFilter"
      />
    </div>

    <div v-loading="loading" class="timeline-container">
      <div v-if="commits.length > 0" class="commit-list">
        <div
          v-for="commit in commits"
          :key="commit.sha"
          class="commit-card"
          :class="{ 'is-expanded': expandedCommits.has(commit.sha) }"
          @click="toggleCommitDetail(commit.sha)"
        >
          <!-- 头部单行：勾选 · 短 SHA · 文件数 · 作者 · 相对时间 · 展开箭头 -->
          <div class="commit-header">
            <div class="commit-header-main">
              <el-checkbox
                :model-value="selectedShas.includes(commit.sha)"
                class="commit-checkbox"
                @update:model-value="toggleSelectSha(commit.sha, $event)"
                @click.stop
              />
              <el-text
                type="primary"
                class="sha-text"
                @click.stop="copyToClipboard(commit.sha)"
              >
                {{ commit.shortSha }}
              </el-text>
              <el-tag size="small" type="info" class="files-count-tag">
                {{ commit.files?.length || 0 }} 文件
              </el-tag>
              <span class="commit-author">
                <el-icon><User /></el-icon>{{ commit.author }}
              </span>
              <span class="commit-time">{{ formatTime(commit.timestamp) }}</span>
            </div>
            <el-icon class="commit-expand-icon">
              <component :is="expandedCommits.has(commit.sha) ? ArrowUp : ArrowDown" />
            </el-icon>
          </div>

          <div class="commit-message">{{ commit.message }}</div>

          <el-collapse-transition>
            <div v-show="expandedCommits.has(commit.sha)" class="commit-detail">
              <el-descriptions :column="1" size="small" border>
                <el-descriptions-item label="完整 SHA">
                  <div class="sha-full">
                    <el-text class="sha-text">{{ commit.sha }}</el-text>
                    <el-button
                      :icon="DocumentCopy"
                      size="small"
                      text
                      @click.stop="copyToClipboard(commit.sha)"
                    />
                  </div>
                </el-descriptions-item>
                <el-descriptions-item label="作者邮箱">
                  {{ commit.email }}
                </el-descriptions-item>
                <el-descriptions-item label="提交时间">
                  {{ commit.dateTime }}
                </el-descriptions-item>
              </el-descriptions>

              <div class="files-section">
                <el-text size="small" strong>变更文件：</el-text>
                <el-tag
                  v-for="(file, index) in commit.files"
                  :key="index"
                  size="small"
                  class="file-tag"
                  @click.stop="openCommitFileDiff(commit.sha, file)"
                >
                  {{ file }}
                </el-tag>
              </div>
            </div>
          </el-collapse-transition>
        </div>
      </div>

      <el-empty
        v-else-if="!loading && commits.length === 0"
        :description="hasActiveFilter ? '未找到匹配的提交' : '暂无提交记录'"
      />

      <div
        v-if="!loading && commits.length > 0 && hasMore"
        class="load-more"
      >
        <el-button
          type="primary"
          @click="loadMore"
          :loading="loadingMore"
          plain
          style="width: 100%;"
        >
          加载更多 ({{ commits.length }})
        </el-button>
      </div>
    </div>

    <!-- 单提交单文件 diff 弹窗 -->
    <FileDiffDialog
      v-model="commitDiffVisible"
      :repo-path="repoPath"
      :file="commitDiffFile"
      :sha="commitDiffSha"
      mode="commit"
    />

    <!-- 两提交区间 diff 弹窗 -->
    <FileDiffDialog
      v-model="rangeDiffVisible"
      :repo-path="repoPath"
      :base-sha="rangeBaseSHA"
      :head-sha="rangeHeadSHA"
      mode="range"
    />
  </el-card>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Refresh, DocumentCopy, ArrowUp, ArrowDown,
  User, Search
} from '@element-plus/icons-vue'
import { GetCommitHistory } from '../../wailsjs/go/main/App'
import FileDiffDialog from './FileDiffDialog.vue'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const PAGE_SIZE = 20

const commits = ref([])
const expandedCommits = ref(new Set())
const emit = defineEmits(['latest-commit'])
const loading = ref(false)
const loadingMore = ref(false)
const searchKeyword = ref('')
const hasMore = ref(false)

// 服务端过滤条件：author/keyword/filePath 子串匹配，dateRange 转换为 since/until 日期串
const filter = ref({ author: '', dateRange: null, filePath: '' })
let filterTimer = null

// 单提交单文件 diff 弹窗状态
const commitDiffVisible = ref(false)
const commitDiffSha = ref('')
const commitDiffFile = ref('')

// range diff 勾选与弹窗状态
const selectedShas = ref([])
const rangeDiffVisible = ref(false)
const rangeBaseSHA = ref('')
const rangeHeadSHA = ref('')

// buildFilter 将前端过滤状态组装为后端 CommitFilter 对象，空值不参与过滤
const buildFilter = () => {
  const f = {
    author: filter.value.author || '',
    keyword: searchKeyword.value || '',
    filePath: filter.value.filePath || ''
  }
  if (filter.value.dateRange && filter.value.dateRange.length === 2) {
    f.since = filter.value.dateRange[0]
    f.until = filter.value.dateRange[1]
  }
  return f
}

// hasActiveFilter 是否存在任意过滤条件，用于区分空列表来源：有过滤则「未找到匹配的提交」，无则「暂无提交记录」
const hasActiveFilter = computed(() => {
  const f = buildFilter()
  return Boolean(f.author || f.keyword || f.filePath || f.since || f.until)
})

// applyFilter 防抖触发服务端过滤重载（300ms 内连续输入合并为一次请求）
const applyFilter = () => {
  clearTimeout(filterTimer)
  filterTimer = setTimeout(() => {
    selectedShas.value = []
    expandedCommits.value.clear()
    loadCommits(true)
  }, 300)
}

// 组件卸载时清未触发的防抖定时器，避免卸载后回调仍触发 loadCommits 写已销毁响应式状态
onUnmounted(() => {
  clearTimeout(filterTimer)
})

const loadCommits = async (reset = true) => {
  if (reset) {
    loading.value = true
    commits.value = []
    expandedCommits.value.clear()
  } else {
    loadingMore.value = true
  }

  try {
    const offset = reset ? 0 : commits.value.length
    const pageSize = PAGE_SIZE
    const newCommits = await GetCommitHistory(props.repoPath, pageSize, offset, buildFilter())

    if (reset) {
      commits.value = newCommits || []
      if (newCommits && newCommits.length > 0) {
        emit('latest-commit', newCommits[0])
      }
    } else {
      commits.value.push(...(newCommits || []))
    }

    hasMore.value = newCommits && newCommits.length === pageSize
  } catch (error) {
    ElMessage.error('加载提交历史失败: ' + (error.message || String(error)))
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

const loadMore = () => {
  loadCommits(false)
}

const handleRefresh = () => {
  expandedCommits.value.clear()
  selectedShas.value = []
  loadCommits(true)
}

const handleSearch = () => {
  applyFilter()
}

const toggleCommitDetail = (sha) => {
  if (expandedCommits.value.has(sha)) {
    expandedCommits.value.delete(sha)
  } else {
    expandedCommits.value.add(sha)
  }
}

// 打开指定提交中某文件相对父提交的 diff 弹窗
const openCommitFileDiff = (sha, file) => {
  commitDiffSha.value = sha
  commitDiffFile.value = file
  commitDiffVisible.value = true
}

// 勾选提交参与区间对比，限选 2 个：第三个替换最早选中的（FIFO）
const toggleSelectSha = (sha, checked) => {
  if (checked) {
    if (!selectedShas.value.includes(sha)) {
      if (selectedShas.value.length >= 2) {
        // 满额时弹出最早的，保持选择顺序为勾选先后
        selectedShas.value.shift()
      }
      selectedShas.value.push(sha)
    }
  } else {
    selectedShas.value = selectedShas.value.filter(s => s !== sha)
  }
}

// 打开两提交区间 diff 弹窗，base 为先选、head 为后选
const openRangeDiff = () => {
  if (selectedShas.value.length !== 2) return
  rangeBaseSHA.value = selectedShas.value[0]
  rangeHeadSHA.value = selectedShas.value[1]
  rangeDiffVisible.value = true
}

const copyToClipboard = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch (error) {
    ElMessage.error('复制失败')
  }
}

const formatTime = (timestamp) => {
  if (!timestamp) return 'N/A'
  const now = Date.now()
  const diff = now - timestamp * 1000
  const minutes = Math.floor(diff / (1000 * 60))
  const hours = Math.floor(diff / (1000 * 60 * 60))
  const days = Math.floor(diff / (1000 * 60 * 60 * 24))

  if (minutes < 60) return `${minutes} 分钟前`
  if (hours < 24) return `${hours} 小时前`
  if (days < 30) return `${days} 天前`
  const date = new Date(timestamp * 1000)
  return date.toLocaleDateString('zh-CN')
}

watch(() => props.repoPath, () => {
  searchKeyword.value = ''
  filter.value = { author: '', dateRange: null, filePath: '' }
  selectedShas.value = []
  loadCommits(true)
})

onMounted(() => {
  loadCommits(true)
})

defineExpose({ loadCommits, handleRefresh })
</script>

<style scoped>
.commit-history-card {
  height: 100%;
  border-radius: var(--radius-md);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
/* el-card 内部 body 撑满剩余高度（header 固定） */
.commit-history-card :deep(.el-card__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: var(--spacing-md);
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
}
.filter-bar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) 0;
  border-bottom: 1px solid var(--border-color);
  flex-wrap: wrap;
}
.filter-input {
  width: 140px;
}
.filter-date {
  width: 240px !important;
}
.timeline-container {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden; /* 兜底：禁止 hover 等场景产生横向滚动条 */
  /* 右侧留白，让卡片右边缘与 webkit 滚动条之间有清晰间距，避免视觉重叠 */
  padding-right: var(--spacing-sm);
}

/* 卡片列表：纵向排列，无时间轴占位 */
.commit-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}
.commit-card {
  cursor: pointer;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-left: 3px solid var(--border-light);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  box-shadow: var(--shadow-sm);
  transition: all var(--transition-fast);
}
.commit-card:hover {
  box-shadow: var(--shadow-md);
  border-color: var(--primary-light);
  border-left-color: var(--primary-color);
}
.commit-card.is-expanded {
  border-left-color: var(--primary-color);
}

/* 头部单行：主信息 + 展开箭头 */
.commit-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--spacing-sm);
}
.commit-header-main {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-width: 0;
  flex: 1;
}
.commit-checkbox {
  flex-shrink: 0;
  /* el-checkbox 默认 margin-right 偏大，收紧贴合列表行 */
  margin-right: 0;
}
.compare-btn {
  margin-right: 10px;
}
.sha-text {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  cursor: pointer;
  color: var(--primary-color);
  font-weight: 500;
  flex-shrink: 0;
}
.sha-text:hover {
  text-decoration: underline;
  color: var(--primary-dark);
}
.files-count-tag {
  flex-shrink: 0;
}
.commit-author {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 120px;
}
.commit-time {
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
  margin-left: auto;
  flex-shrink: 0;
}
.commit-expand-icon {
  color: var(--text-tertiary);
  flex-shrink: 0;
  transition: color var(--transition-fast);
}
.commit-card:hover .commit-expand-icon {
  color: var(--primary-color);
}
.sha-full {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
.commit-message {
  display: block;
  margin-top: 6px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--text-primary);
  word-break: break-word;
}
.commit-detail {
  margin-top: var(--spacing-sm);
  padding-top: var(--spacing-sm);
  border-top: 1px solid var(--border-color);
  animation: fadeIn var(--transition-fast);
}
.files-section {
  margin-top: var(--spacing-md);
}
.files-section .el-tag {
  margin-right: var(--spacing-xs);
  margin-bottom: var(--spacing-xs);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  border: 1px solid var(--border-color);
}
.load-more {
  margin-top: var(--spacing-lg);
  text-align: center;
}
.timeline-container::-webkit-scrollbar {
  width: 6px;
}
.timeline-container::-webkit-scrollbar-thumb {
  background-color: var(--text-tertiary);
  border-radius: 3px;
  transition: background var(--transition-fast);
}
.timeline-container::-webkit-scrollbar-thumb:hover {
  background-color: var(--text-secondary);
}
.search-input {
  width: 200px;
  margin-right: 10px;
}
.file-tag {
  margin: 5px 5px 0 0;
}
</style>
