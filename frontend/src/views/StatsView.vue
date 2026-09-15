<!-- frontend/src/views/StatsView.vue -->
<template>
  <div class="stats-view">
    <div class="stats-header">
      <span class="stats-title">仓库统计</span>
      <div class="header-actions">
        <el-radio-group v-model="rangeKey" size="small" @change="loadStats">
          <el-radio-button value="7d">7 天</el-radio-button>
          <el-radio-button value="30d">30 天</el-radio-button>
          <el-radio-button value="90d">90 天</el-radio-button>
          <el-radio-button value="1y">1 年</el-radio-button>
          <el-radio-button value="all">全部</el-radio-button>
        </el-radio-group>
        <el-button :icon="Refresh" circle size="small" :loading="loading" @click="loadStats" />
      </div>
    </div>

    <div v-if="sampled" class="sampled-tip">
      <el-alert type="warning" :closable="false" show-icon title="采样提示">
        数据基于最近 5000 条提交采样（仓库超限未全量缓存，TotalCommits/贡献者为采样值）
      </el-alert>
    </div>

    <div v-loading="loading" class="stats-body">
      <div v-if="repoPath" class="stats-repo-info">
        <span class="repo-name">{{ repoName }}</span>
        <span class="repo-path" :title="repoPath">{{ repoPath }}</span>
        <el-tag v-if="repoInfo?.branch && !repoInfo.isDetached" size="small">{{ repoInfo.branch }}</el-tag>
        <el-tag v-else-if="repoInfo?.isDetached" size="small" type="danger">分离头指针</el-tag>
        <span v-if="repoInfo?.remoteUrl" class="repo-remote" :title="repoInfo.remoteUrl">{{ repoInfo.remoteUrl }}</span>
      </div>
      <div v-if="!repoPath" class="stats-empty">
        <el-empty description="请先在文件树选择仓库或目录" />
      </div>
      <template v-else-if="stats">
        <div class="stats-summary">
          <el-tag>时间窗口：{{ stats.dateRange }}</el-tag>
          <el-tag type="info">提交数：{{ stats.totalCommits }}</el-tag>
          <el-tag type="success">贡献者：{{ stats.contributors.length }}</el-tag>
          <el-tag type="warning">粒度：{{ granularityLabel }}</el-tag>
        </div>
        <RepoStatsChart :stats="stats" />
      </template>
      <el-empty v-else-if="!loading" description="暂无统计数据" />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { GetRepoStats, GetGitRemoteURL } from '../../wailsjs/go/main/App'
import { useWorkspaceStore, useUiStore } from '../store'
import { gitCache, getCacheKey } from '../utils/gitCache'
import RepoStatsChart from '../components/RepoStatsChart.vue'

const workspaceStore = useWorkspaceStore()
const uiStore = useUiStore()

const rangeKey = ref('30d')
const stats = ref(null)
const loading = ref(false)
const sampled = ref(false)

// requestSeq 请求序号：仅采纳最新一次 loadStats 的结果，丢弃并发旧请求返回
// （连点档位/快速切仓库时后到旧响应不覆盖新选中态）。
let requestSeq = 0
// repoInfoSeq 仓库信息请求序号：与 loadStats 同理，快速切仓库时丢弃后到的旧仓库信息响应。
let repoInfoSeq = 0

// repoPath 当前选中节点的路径（文件/目录均可，后端 FindGitRoot 定位 git 根）
const repoPath = computed(() => workspaceStore.selectedNode?.path || '')

// repoName 当前仓库名（取选中节点 name，文件树节点已带）
const repoName = computed(() => workspaceStore.selectedNode?.name || '')

// repoInfo 仓库分支/远程地址（复用 GitInfo 的 git-info 缓存键，命中则零调用）
const repoInfo = ref(null)

// granularityLabel 粒度中文映射，供摘要标签展示
const granularityLabel = computed(() => {
  const map = { day: '按日', week: '按周', month: '按月' }
  return map[stats.value?.granularity] || stats.value?.granularity || ''
})

// loadStats 拉取统计：仅统计页可见时触发（避免非统计页选中节点白打后端）；
// 无选中路径清空；requestSeq 串行化丢弃并发旧响应。
const loadStats = async () => {
  // 非统计页不聚合：StatsView v-show 常驻挂载，watch(repoPath) 在任意面板触发，
  // 仅统计页可见才调后端，避免工作目录/工具箱面板浏览文件树时无谓统计
  if (uiStore.activePanel !== 'stats') return
  if (!repoPath.value) {
    stats.value = null
    return
  }
  const seq = ++requestSeq
  loading.value = true
  try {
    const result = await GetRepoStats(repoPath.value, rangeKey.value)
    // 旧请求返回丢弃：切档位/切仓库后已有更新的请求在途
    if (seq !== requestSeq) return
    stats.value = result
    sampled.value = result?.sampled || false
  } catch (error) {
    if (seq !== requestSeq) return
    ElMessage.error('加载统计数据失败: ' + (error.message || String(error)))
    stats.value = null
    sampled.value = false
  } finally {
    if (seq === requestSeq) {
      loading.value = false
    }
  }
}

// loadRepoInfo 加载仓库分支/远程地址：仅统计页可见时触发（与 loadStats 同 guard）；
// 复用 GitInfo 的 git-info 缓存键，用户看过「仓库信息」tab 即命中零调用；
// repoInfoSeq 丢弃并发旧响应（快速切仓库时后到旧仓库信息不覆盖新选中态）。
const loadRepoInfo = async () => {
  if (uiStore.activePanel !== 'stats') return
  if (!repoPath.value) {
    repoInfo.value = null
    return
  }
  const cacheKey = getCacheKey('git-info', repoPath.value)
  const cached = gitCache.get(cacheKey)
  if (cached?.info) {
    repoInfo.value = cached.info
    return
  }
  const seq = ++repoInfoSeq
  try {
    const info = await GetGitRemoteURL(repoPath.value)
    if (seq !== repoInfoSeq) return
    repoInfo.value = info
  } catch {
    if (seq !== repoInfoSeq) return
    repoInfo.value = null
  }
}

// 选中节点变化时重载统计与仓库信息（切仓库/切目录），loadStats 内部判面板可见性
watch(repoPath, () => {
  loadStats()
  loadRepoInfo()
})

// 切到统计页时若数据未加载则触发（从其他面板切回补载）
watch(() => uiStore.activePanel, (panel) => {
  if (panel === 'stats' && repoPath.value) {
    if (!stats.value) loadStats()
    if (!repoInfo.value) loadRepoInfo()
  }
})

onMounted(() => {
  loadStats()
  loadRepoInfo()
})
</script>

<style scoped>
.stats-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary, #fff);
  overflow: hidden;
}
.stats-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-md, 16px);
  border-bottom: 1px solid var(--border-color, #ebeef5);
}
.stats-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary, #303133);
}
.header-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm, 8px);
}
.sampled-tip {
  padding: var(--spacing-sm, 8px) var(--spacing-md, 16px) 0;
}
.stats-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-md, 16px);
}
.stats-repo-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm, 8px);
  flex-wrap: wrap;
  padding: var(--spacing-sm, 8px) var(--spacing-md, 16px);
  margin-bottom: var(--spacing-md, 16px);
  background: var(--bg-secondary, #f5f7fa);
  border: 1px solid var(--border-color, #ebeef5);
  border-radius: var(--radius-md, 8px);
}
.stats-repo-info .repo-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary, #303133);
}
.stats-repo-info .repo-path {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-tertiary, #909399);
  word-break: break-all;
}
.stats-repo-info .repo-remote {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-secondary, #606266);
  word-break: break-all;
}
.stats-summary {
  display: flex;
  gap: var(--spacing-sm, 8px);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-md, 16px);
}
.stats-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}
</style>
