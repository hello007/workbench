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
            type="primary"
            class="url-text"
            :underline="false"
            @click="openRemoteUrl(gitInfo.remoteUrl)"
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
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
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
        // 缓存命中：一并恢复 info 与 latestCommit，
        // 避免命中分支直接 return 导致提交信息丢失（偶发 N/A）。
        gitInfo.value = cached.info
        localLatestCommit.value = cached.latestCommit || null
        loading.value = false
        return
      }
    } else {
      gitCache.delete(cacheKey)
      localLatestCommit.value = null
    }

    // 用 allSettled 区分 info / commit 成败：
    //  - info 失败：抛出走外层 catch 报错；
    //  - commit 失败：本次不落缓存，下次进入自动重试，
    //    避免 5 分钟内重进仍 N/A（防御性：失败不污染缓存）。
    const [infoRes, commitsRes] = await Promise.allSettled([
      GetGitRemoteURL(props.repoPath),
      GetCommitHistory(props.repoPath, 1, 0)
    ])

    if (infoRes.status !== 'fulfilled') {
      throw infoRes.reason
    }
    gitInfo.value = infoRes.value

    const commits = commitsRes.status === 'fulfilled' ? commitsRes.value : null
    const latestCommit = commits && commits.length > 0 ? commits[0] : null
    localLatestCommit.value = latestCommit

    // 仅当 commit 成功才落缓存；commit 失败时本次不缓存，下次进入重拉两边。
    if (commitsRes.status === 'fulfilled') {
      gitCache.set(cacheKey, { info: infoRes.value, latestCommit })
    }
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

// http(s) 远程地址点击交由系统默认浏览器打开（BrowserOpenURL），
// 避免在 Wails 内置 webview 内导航/新开窗口（无法复用用户浏览器会话）。
const openRemoteUrl = (url) => {
  if (!url || !isHttpUrl(url)) return
  BrowserOpenURL(url)
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
  localLatestCommit.value = null
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
.git-info-card :deep(.el-descriptions__label) {
  width: 80px;
  min-width: 80px;
  white-space: nowrap;
  font-weight: 600;
  color: var(--text-primary);
  background: var(--bg-tertiary);
  text-align: center;
}
.git-info-card :deep(.el-descriptions__cell.is-bordered-label) {
  text-align: center;
}
:deep(.el-descriptions__content) {
  color: var(--text-secondary);
}
</style>
