<template>
  <div class="overview-view">
    <div class="overview-header">
      <span class="overview-title">概览</span>
      <div class="header-actions">
        <span v-if="generatedText" class="fresh-tip">统计于 {{ generatedText }}</span>
        <el-button size="small" :loading="loading" @click="load(true)">
          <el-icon><Refresh /></el-icon>
          <span>刷新</span>
        </el-button>
      </div>
    </div>

    <div v-loading="loading && !snapshot" class="overview-body">
      <template v-if="snapshot">
        <section class="domain">
          <h3 class="domain-title">仓库状态</h3>
          <div class="card-grid">
            <RepoPendingCard :repos="snapshot.repos" @drill="goDashboard" @retry="load(true)" />
            <RepoStaleCard :repos="snapshot.repos" @drill="goDashboard" @retry="load(true)" />
          </div>
        </section>

        <section class="domain">
          <!-- 范围控件只覆盖 AI 域（M4/M5）：位置即语义，仓库/待办不加任何范围说明 -->
          <div class="domain-head">
            <h3 class="domain-title">AI 任务</h3>
            <el-radio-group v-model="rangeKey" size="small" @change="load(false)">
              <el-radio-button value="today">今日</el-radio-button>
              <el-radio-button value="week">本周</el-radio-button>
              <el-radio-button value="7d">近 7 天</el-radio-button>
              <el-radio-button value="30d">近 30 天</el-radio-button>
              <el-radio-button value="90d">近 90 天</el-radio-button>
            </el-radio-group>
          </div>
          <div class="card-grid">
            <AiCostCard :ai="snapshot.ai" @drill="goAi" @retry="load(true)" />
            <AiSuccessCard :ai="snapshot.ai" @drill="goAi" @retry="load(true)" />
          </div>
        </section>

        <section class="domain">
          <h3 class="domain-title">待办</h3>
          <div class="card-grid">
            <TodoBacklogCard :todos="snapshot.todos" @drill="goTodo" @retry="load(true)" />
          </div>
        </section>
      </template>

      <el-empty v-else-if="!loading" description="概览加载失败，请点击刷新重试" />
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { GetOverviewSnapshot } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import { useUiStore } from '../store'
import RepoPendingCard from '../components/overview/RepoPendingCard.vue'
import RepoStaleCard from '../components/overview/RepoStaleCard.vue'
import AiCostCard from '../components/overview/AiCostCard.vue'
import AiSuccessCard from '../components/overview/AiSuccessCard.vue'
import TodoBacklogCard from '../components/overview/TodoBacklogCard.vue'

/**
 * 个人看板「概览」面板：3 个数据域、5 张卡片（M1–M5）。
 *
 * 只读聚合，数值全部由后端 GetOverviewSnapshot 按 docs/spec/dashboard-metrics.md
 * 计算，本视图只渲染。刷新策略：进入即拉 + 后端 5 分钟 TTL + 手动刷新（force）。
 * requestSeq 丢弃并发旧响应（对齐 StatsView 范式）。
 */
const uiStore = useUiStore()

const snapshot = ref(null)
const loading = ref(false)

// 时间范围档位（只作用于 AI 域 M4/M5），默认本周；不持久化，切面板保当前档位。
// 后端按五档缓存，切档不传 force，仅换返回块，仓库/待办域数值不变。
const rangeKey = ref('week')

// requestSeq 请求序号：连点刷新/快速切档/切回面板时，后到的旧响应不覆盖新快照。
// 每次 load 先自增，切档调 load(false) 即重置守卫。
let requestSeq = 0

const generatedText = computed(() => {
  const ts = snapshot.value?.generatedAt
  if (!ts) return ''
  return new Date(ts).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
})

/**
 * 拉取概览快照。
 * @param {boolean} force true = 跳过后端 TTL 强制重算
 */
async function load(force = false) {
  const seq = ++requestSeq
  loading.value = true
  try {
    const result = await GetOverviewSnapshot(rangeKey.value, force)
    if (seq !== requestSeq) return
    snapshot.value = result
  } catch (error) {
    if (seq !== requestSeq) return
    // 统一错误分流：结构化 {code,message} 走 handleError，禁裸 ElMessage.error。
    handleError('加载概览失败: ', error)
  } finally {
    if (seq === requestSeq) {
      loading.value = false
    }
  }
}

/** M1/M2 下钻：切到「状态看板」，仓库明细与 ahead/behind 在该面板查看。 */
function goDashboard() {
  uiStore.activePanel = 'dashboard'
}

/** M3 下钻：切到「临时待办」并让面板定位最老未完成条目。 */
function goTodo(payload) {
  uiStore.pushOverviewDrillRequest('todo', payload)
  uiStore.activePanel = 'todo'
}

/** M4/M5 下钻：切到「AI 功能」并打开历史抽屉、带入时间窗。 */
function goAi(payload) {
  uiStore.pushOverviewDrillRequest('ai', payload)
  uiStore.activePanel = 'ai'
}

// 面板 v-show 常驻挂载：仅在本面板可见时拉取，避免其他面板操作触发无谓聚合。
watch(
  () => uiStore.activePanel,
  (panel) => {
    if (panel === 'overview') load(false)
  },
  { immediate: true }
)
</script>

<style scoped>
.overview-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: var(--spacing-md) var(--spacing-lg);
  overflow: hidden;
  background: var(--bg-primary);
}

.overview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
  margin-bottom: var(--spacing-md);
  padding-bottom: var(--spacing-sm);
  border-bottom: 1px solid var(--border-color);
}

.overview-title {
  font-size: 18px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.fresh-tip {
  font-size: 12px;
  color: var(--text-tertiary);
}

.overview-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--spacing-lg);
  min-height: 0;
  overflow: auto;
}

.domain {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.domain-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
}

.domain-title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: var(--spacing-md);
}
</style>
