<template>
  <el-card
    v-if="gitInfo"
    class="git-info-card"
    shadow="hover"
  >
    <template #header>
      <div class="card-header">
        <span>Git 仓库信息</span>
        <el-button
          :icon="Refresh"
          circle
          size="small"
          @click="handleRefresh"
          :loading="loading"
        />
      </div>
    </template>

    <el-descriptions
      :column="1"
      border
      size="small"
      v-loading="loading"
    >
      <el-descriptions-item label="远程地址">
        <div v-if="gitInfo.remoteUrl" class="url-with-copy">
          <el-link
            v-if="isHttpUrl(gitInfo.remoteUrl)"
            :href="gitInfo.remoteUrl"
            target="_blank"
            type="primary"
            class="url-text"
            :underline="false"
          >
            {{ gitInfo.remoteUrl }}
          </el-link>
          <span v-else class="url-text">{{ gitInfo.remoteUrl }}</span>
          <el-button
            :icon="DocumentCopy"
            size="small"
            text
            @click="copyToClipboard(gitInfo.remoteUrl)"
          />
        </div>
        <el-text v-else type="info">未配置远程地址</el-text>
      </el-descriptions-item>

      <el-descriptions-item label="当前分支">
        <el-tag
          v-if="!gitInfo.isDetached"
          :type="getBranchTagType(gitInfo.branch)"
        >
          {{ gitInfo.branch }}
        </el-tag>
        <el-tag v-else type="danger">分离头指针</el-tag>
      </el-descriptions-item>

      <el-descriptions-item label="最新提交">
        <div class="sha-with-copy">
          <el-text class="sha-text">{{ effectiveLatestCommit?.shortSha || 'N/A' }}</el-text>
          <el-button
            :icon="DocumentCopy"
            size="small"
            text
            @click="copyToClipboard(effectiveLatestCommit?.sha || '')"
          />
        </div>
      </el-descriptions-item>

      <el-descriptions-item label="提交时间">
        {{ formatTime(effectiveLatestCommit?.timestamp) }}
      </el-descriptions-item>

      <el-descriptions-item label="提交消息">
        <el-text class="commit-message" :line-clamp="3">
          {{ effectiveLatestCommit?.message || 'N/A' }}
        </el-text>
      </el-descriptions-item>
    </el-descriptions>
  </el-card>
</template>

<script setup>
import { ref, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, DocumentCopy } from '@element-plus/icons-vue'
import { GetGitRemoteURL, GetCommitHistory } from '../../wailsjs/go/main/App'
import { gitCache, getCacheKey } from '../utils/gitCache'

const props = defineProps({
  repoPath: { type: String, required: true },
  latestCommit: { type: Object, default: null }
})

const gitInfo = ref(null)
const loading = ref(false)
const localLatestCommit = ref(null)

const effectiveLatestCommit = computed(() => props.latestCommit || localLatestCommit.value)

const loadGitInfo = async (forceRefresh = false) => {
  loading.value = true
  try {
    const cacheKey = getCacheKey('git-info', props.repoPath)

    if (!forceRefresh) {
      const cached = gitCache.get(cacheKey)
      if (cached) {
        gitInfo.value = cached
        loading.value = false
        return
      }
    } else {
      gitCache.delete(cacheKey)
      localLatestCommit.value = null
    }

    const [info, commits] = await Promise.all([
      GetGitRemoteURL(props.repoPath),
      GetCommitHistory(props.repoPath, 1, 0).catch(() => [])
    ])
    gitInfo.value = info
    if (commits && commits.length > 0) {
      localLatestCommit.value = commits[0]
    }
    gitCache.set(cacheKey, info)
  } catch (error) {
    ElMessage.error('加载 Git 信息失败: ' + (error.message || String(error)))
  } finally {
    loading.value = false
  }
}

const handleRefresh = () => {
  loadGitInfo(true)
}

const isHttpUrl = (url) => {
  return url && (url.startsWith('http://') || url.startsWith('https://'))
}

const getBranchTagType = (branch) => {
  if (branch === 'main' || branch === 'master') return 'primary'
  return 'success'
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
  const days = Math.floor(diff / (1000 * 60 * 60 * 24))
  if (days === 0) return '今天'
  if (days === 1) return '昨天'
  if (days < 7) return `${days} 天前`
  if (days < 30) return `${Math.floor(days / 7)} 周前`
  const date = new Date(timestamp * 1000)
  return date.toLocaleDateString('zh-CN')
}

watch(() => props.repoPath, () => {
  gitInfo.value = null
  loadGitInfo()
})

loadGitInfo()

defineExpose({ loadGitInfo, handleRefresh })
</script>

<style scoped>
.git-info-card {
  margin-bottom: var(--spacing-lg);
  border-radius: var(--radius-md);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
  transition: all var(--transition-normal);
}
.git-info-card:hover {
  box-shadow: var(--shadow-md);
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 600;
  font-size: 16px;
  color: var(--text-primary);
}
.url-with-copy {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
.url-text {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  color: var(--text-secondary);
}
/* el-link 复用 .url-text：仅统一字体族与字号，不覆盖 el-link 主题色 */
:deep(.el-link.url-text) {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  /* 颜色由 el-link 自身 type=primary 控制，保持主题色 */
}
.sha-with-copy {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
.sha-text {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  color: var(--primary-color);
  cursor: pointer;
  font-weight: 500;
}
.commit-message {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
}
/* 优化标签样式 */
:deep(.el-tag) {
  border-radius: var(--radius-sm);
  font-weight: 500;
  padding: 4px 8px;
}
/* 优化链接样式 */
:deep(.el-link) {
  color: var(--primary-color);
  font-weight: 500;
}
:deep(.el-link:hover) {
  color: var(--primary-dark);
  text-decoration: underline;
}
/* 优化描述列表样式 */
:deep(.el-descriptions__table) {
  width: 100%;
}
:deep(.el-descriptions__label) {
  width: 80px;
  min-width: 80px;
  white-space: nowrap;
  font-weight: 600;
  color: var(--text-primary);
  background: var(--bg-tertiary);
  text-align: center;
}
:deep(.el-descriptions__content) {
  color: var(--text-secondary);
}
</style>
