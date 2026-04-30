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
        <div v-if="gitInfo.remoteUrl">
          <el-link
            v-if="isHttpUrl(gitInfo.remoteUrl)"
            :href="gitInfo.remoteUrl"
            target="_blank"
            type="primary"
          >
            {{ gitInfo.remoteUrl }}
          </el-link>
          <div v-else class="url-with-copy">
            <span class="url-text">{{ gitInfo.remoteUrl }}</span>
            <el-button
              :icon="DocumentCopy"
              size="small"
              text
              @click="copyToClipboard(gitInfo.remoteUrl)"
            />
          </div>
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
          <el-text class="sha-text">{{ latestCommit?.shortSha || 'N/A' }}</el-text>
          <el-button
            :icon="DocumentCopy"
            size="small"
            text
            @click="copyToClipboard(latestCommit?.sha || '')"
          />
        </div>
      </el-descriptions-item>

      <el-descriptions-item label="提交时间">
        {{ formatTime(latestCommit?.timestamp) }}
      </el-descriptions-item>

      <el-descriptions-item label="提交消息">
        <el-text class="commit-message" :line-clamp="3">
          {{ latestCommit?.message || 'N/A' }}
        </el-text>
      </el-descriptions-item>
    </el-descriptions>
  </el-card>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, DocumentCopy } from '@element-plus/icons-vue'
import { GetGitRemoteURL } from '../../wailsjs/go/main/App'
import { gitCache, getCacheKey } from '../utils/gitCache'

const props = defineProps({
  repoPath: { type: String, required: true },
  latestCommit: { type: Object, default: null }
})

const gitInfo = ref(null)
const loading = ref(false)

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
    }

    const info = await GetGitRemoteURL(props.repoPath)
    gitInfo.value = info
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
.git-info-card { margin-bottom: 20px; }
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 500;
}
.url-with-copy {
  display: flex;
  align-items: center;
  gap: 8px;
}
.url-text {
  font-family: monospace;
  font-size: 13px;
  color: #606266;
}
.sha-with-copy {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sha-text {
  font-family: monospace;
  font-size: 13px;
  color: #409EFF;
  cursor: pointer;
}
.commit-message {
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
