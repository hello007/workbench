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
            @input="handleSearch"
          />
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

    <div v-loading="loading" class="timeline-container">
      <div v-if="filteredCommits.length > 0" class="commit-list">
        <div
          v-for="commit in filteredCommits"
          :key="commit.sha"
          class="commit-card"
          :class="{ 'is-expanded': expandedCommits.has(commit.sha) }"
          @click="toggleCommitDetail(commit.sha)"
        >
          <!-- 头部单行：短 SHA · 文件数 · 作者 · 相对时间 · 展开箭头 -->
          <div class="commit-header">
            <div class="commit-header-main">
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
        description="暂无提交记录"
      />

      <el-empty
        v-else-if="!loading && filteredCommits.length === 0"
        description="未找到匹配的提交"
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
  </el-card>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Refresh, DocumentCopy, ArrowUp, ArrowDown,
  User, Search
} from '@element-plus/icons-vue'
import { GetCommitHistory } from '../../wailsjs/go/main/App'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const PAGE_SIZE = 20
const MAX_COMMITS = 500

const commits = ref([])
const expandedCommits = ref(new Set())
const emit = defineEmits(['latest-commit'])
const loading = ref(false)
const loadingMore = ref(false)
const searchKeyword = ref('')
const hasMore = ref(false)

const filteredCommits = computed(() => {
  if (!searchKeyword.value) return commits.value

  const keyword = searchKeyword.value.toLowerCase()
  return commits.value.filter(commit =>
    commit.message.toLowerCase().includes(keyword) ||
    commit.author.toLowerCase().includes(keyword) ||
    commit.sha.toLowerCase().includes(keyword)
  )
})

const loadCommits = async (reset = true) => {
  if (reset) {
    loading.value = true
    commits.value = []
    expandedCommits.value.clear()
  } else {
    if (commits.value.length >= MAX_COMMITS) return
    loadingMore.value = true
  }

  try {
    const offset = reset ? 0 : commits.value.length
    const remaining = MAX_COMMITS - offset
    const pageSize = Math.min(PAGE_SIZE, remaining)
    const newCommits = await GetCommitHistory(props.repoPath, pageSize, offset)

    if (reset) {
      commits.value = newCommits || []
      if (newCommits && newCommits.length > 0) {
        emit('latest-commit', newCommits[0])
      }
    } else {
      commits.value.push(...(newCommits || []))
    }

    hasMore.value = newCommits && newCommits.length === pageSize && commits.value.length < MAX_COMMITS
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
  loadCommits(true)
}

const handleSearch = () => {
  // 搜索由 computed 属性自动处理
}

const toggleCommitDetail = (sha) => {
  if (expandedCommits.value.has(sha)) {
    expandedCommits.value.delete(sha)
  } else {
    expandedCommits.value.add(sha)
  }
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
