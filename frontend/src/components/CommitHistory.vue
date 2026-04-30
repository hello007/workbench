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
            style="width: 200px; margin-right: 10px;"
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
      <el-timeline v-if="filteredCommits.length > 0">
        <el-timeline-item
          v-for="commit in filteredCommits"
          :key="commit.sha"
          :timestamp="formatTime(commit.timestamp)"
          placement="top"
          @click="toggleCommitDetail(commit.sha)"
          class="commit-item"
        >
          <el-card class="commit-card" shadow="hover">
            <div class="commit-header">
              <div class="commit-sha">
                <el-text
                  type="primary"
                  class="sha-text"
                  @click.stop="copyToClipboard(commit.sha)"
                >
                  {{ commit.shortSha }}
                </el-text>
                <el-tag size="small" type="info" style="margin-left: 10px;">
                  {{ commit.files.length }} 个文件
                </el-tag>
              </div>
              <el-button
                :icon="expandedCommits.has(commit.sha) ? ArrowUp : ArrowDown"
                size="small"
                text
                @click.stop="toggleCommitDetail(commit.sha)"
              />
            </div>

            <el-text class="commit-message">{{ commit.message }}</el-text>
            <div class="commit-meta">
              <el-icon><User /></el-icon>
              <el-text size="small">{{ commit.author }}</el-text>
              <el-divider direction="vertical" />
              <el-text size="small" type="info">
                {{ formatTime(commit.timestamp) }}
              </el-text>
            </div>

            <el-collapse-transition>
              <div v-show="expandedCommits.has(commit.sha)" class="commit-detail">
                <el-divider />
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
                    style="margin: 5px 5px 0 0;"
                  >
                    {{ file }}
                  </el-tag>
                </div>
              </div>
            </el-collapse-transition>
          </el-card>
        </el-timeline-item>
      </el-timeline>

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
import { ref, computed, onMounted } from 'vue'
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

const commits = ref([])
const expandedCommits = ref(new Set())
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
  } else {
    loadingMore.value = true
  }

  try {
    const offset = reset ? 0 : commits.value.length
    const newCommits = await GetCommitHistory(props.repoPath, PAGE_SIZE, offset)

    if (reset) {
      commits.value = newCommits
    } else {
      commits.value.push(...newCommits)
    }

    hasMore.value = newCommits.length === PAGE_SIZE
  } catch (error) {
    ElMessage.error('加载提交历史失败: ' + error)
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

onMounted(() => {
  loadCommits(true)
})

defineExpose({ loadCommits, handleRefresh })
</script>

<style scoped>
.commit-history-card { height: 100%; }
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 500;
}
.header-actions {
  display: flex;
  align-items: center;
}
.timeline-container {
  max-height: 600px;
  overflow-y: auto;
}
.commit-item { cursor: pointer; }
.commit-card { margin-bottom: 10px; }
.commit-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.commit-sha {
  display: flex;
  align-items: center;
}
.sha-text {
  font-family: monospace;
  font-size: 13px;
  cursor: pointer;
}
.sha-text:hover { text-decoration: underline; }
.sha-full {
  display: flex;
  align-items: center;
  gap: 10px;
}
.commit-message {
  display: block;
  margin: 10px 0;
  font-size: 14px;
  line-height: 1.5;
  color: #303133;
}
.commit-meta {
  display: flex;
  align-items: center;
  gap: 5px;
  color: #909399;
}
.commit-detail { margin-top: 10px; }
.files-section { margin-top: 15px; }
.load-more {
  margin-top: 20px;
  text-align: center;
}
.timeline-container::-webkit-scrollbar { width: 6px; }
.timeline-container::-webkit-scrollbar-thumb {
  background-color: #dcdfe6;
  border-radius: 3px;
}
</style>
