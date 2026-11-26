<template>
  <el-dialog
    :model-value="visible"
    title="AI 任务运行历史"
    width="920px"
    class="ai-history-dialog"
    append-to-body
    destroy-on-close
    @update:model-value="$emit('update:visible', $event)"
    @open="onOpen"
  >
    <!-- 顶部筛选：功能下拉 / 时间范围 / 状态多选 + 清理按钮 -->
    <div class="history-filter">
      <el-select
        v-model="filter.functionId"
        placeholder="全部功能"
        size="small"
        clearable
        class="filter-fn"
      >
        <el-option
          v-for="f in functions"
          :key="f.id"
          :label="f.name"
          :value="f.id"
        />
      </el-select>
      <el-date-picker
        v-model="dateRange"
        type="datetimerange"
        size="small"
        range-separator="至"
        start-placeholder="开始时间"
        end-placeholder="结束时间"
        format="YYYY-MM-DD HH:mm"
        value-format="x"
        class="filter-date"
        @change="onDateChange"
      />
      <el-select
        v-model="filter.status"
        placeholder="全部状态"
        size="small"
        clearable
        class="filter-status"
        @change="onStatusChange"
      >
        <el-option label="成功" value="success" />
        <el-option label="失败" value="failed" />
        <el-option label="已取消" value="canceled" />
        <el-option label="超时" value="timeout" />
      </el-select>
      <!-- 概览 M5 下钻带入的多状态筛选（失败+超时），可一键清除 -->
      <el-tag
        v-if="filter.statuses.length"
        size="small"
        type="warning"
        closable
        @close="clearStatuses"
      >
        {{ statusesLabel }}
      </el-tag>
      <el-button size="small" @click="loadList">查询</el-button>
      <el-button size="small" type="warning" plain @click="clearOld">清理 30 天前</el-button>
      <el-button size="small" plain @click="exportCsv">导出 CSV</el-button>
      <el-button size="small" plain @click="exportMarkdown">导出 Markdown</el-button>
      <span class="filter-count">共 {{ list.length }} 条</span>
    </div>

    <!-- 统计卡片区：随筛选查询一并刷新 -->
    <div class="history-stats">
      <div class="stat-card">
        <div
          class="stat-value"
          :title="`成功 ${stats.successCount || 0} / 失败 ${stats.failedCount || 0} / 超时 ${stats.timeoutCount || 0} / 取消 ${stats.canceledCount || 0}`"
        >
          {{ stats.totalCount }} <span class="stat-sub">成功 {{ stats.successCount }}</span>
        </div>
        <div class="stat-label">运行次数</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">${{ stats.totalCostUsd.toFixed(3) }}</div>
        <div class="stat-label">总成本</div>
      </div>
      <div class="stat-card">
        <div class="stat-value" title="入 {{ stats.totalInputTokens }} / 出 {{ stats.totalOutputTokens }} / 缓存读 {{ stats.totalCacheReadTokens }} / 缓存写 {{ stats.totalCacheCreationTokens }}">
          {{ totalTokens() }}
        </div>
        <div class="stat-label">总 token</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ fmtDuration(stats.totalDurationMs) }}</div>
        <div class="stat-label">总耗时</div>
      </div>
      <!-- 功能排行：次数（含取消）/ 成本与占比 / token，可切换排序 -->
      <div v-if="stats.byFunction && stats.byFunction.length" class="stat-card fn-rank">
        <div class="fn-rank-head">
          <span class="fn-rank-title">功能排行（近 90 天 / 最多 2000 条）</span>
          <el-radio-group v-model="rankSort" size="small">
            <el-radio-button value="count">次数</el-radio-button>
            <el-radio-button value="cost">成本</el-radio-button>
            <el-radio-button value="tokens">token</el-radio-button>
          </el-radio-group>
        </div>
        <el-table :data="rankedFunctions" size="small" border max-height="140">
          <el-table-column prop="functionName" label="功能排行" min-width="110" show-overflow-tooltip>
            <template #default="{ row }">{{ row.functionName }}</template>
          </el-table-column>
          <el-table-column label="次数" width="96">
            <template #default="{ row }">
              {{ row.count }}
              <span v-if="row.canceledCount" class="fn-canceled">取消 {{ row.canceledCount }}</span>
            </template>
          </el-table-column>
          <el-table-column label="成本" width="104">
            <template #default="{ row }">
              ${{ row.totalCostUsd.toFixed(3) }}
              <span class="fn-share">{{ costShare(row) }}%</span>
            </template>
          </el-table-column>
          <el-table-column label="入+出 token" width="96">
            <template #default="{ row }">{{ formatTokens(row.totalTokens) }}</template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <!-- 历史列表 -->
    <el-table
      :data="list"
      size="small"
      border
      max-height="460"
      empty-text="暂无历史记录"
      @row-click="onRowClick"
    >
      <el-table-column label="时间" width="150">
        <template #default="{ row }">
          {{ fmtTime(row.finishedAt) }}
        </template>
      </el-table-column>
      <el-table-column prop="name" label="功能" min-width="140" />
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag size="small" :type="statusType(row.status)">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="耗时" width="80">
        <template #default="{ row }">
          {{ row.metrics ? fmtDuration(row.metrics.durationMs) : '—' }}
        </template>
      </el-table-column>
      <el-table-column label="成本" width="80">
        <template #default="{ row }">
          {{ row.metrics && row.metrics.costUsd > 0 ? '$' + row.metrics.costUsd.toFixed(3) : '—' }}
        </template>
      </el-table-column>
      <el-table-column label="输出大小" width="100">
        <template #default="{ row }">
          {{ fmtSize(row.outputSize) }}
        </template>
      </el-table-column>
      <el-table-column label="操作" fixed="right" width="140">
        <template #default="{ row }">
          <el-button size="small" link @click.stop="viewOutput(row)">查看输出</el-button>
          <el-button size="small" link type="danger" @click.stop="removeOne(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 输出详情抽屉：懒加载读取归档输出文件全文 -->
    <el-drawer
      v-model="detailVisible"
      :title="detailTitle"
      size="60%"
      append-to-body
      destroy-on-close
    >
      <div v-if="detailLoading" class="detail-loading">读取输出中…</div>
      <pre v-else class="detail-output">{{ detailOutput || '（无输出内容）' }}</pre>
    </el-drawer>
  </el-dialog>
</template>

<script setup>
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  GetAiFunctions,
  GetAiTaskHistory,
  GetAiTaskHistoryStats,
  GetAiTaskHistoryOutput,
  ExportAiTaskHistoryCSV,
  ExportAiTaskHistoryMarkdown,
  DeleteAiTaskHistory,
  ClearAiTaskHistory,
  SaveFile,
  SaveFileDialog
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const props = defineProps({
  visible: Boolean,
  // 概览卡片下钻带入的时间窗（'today'/'week'/'7d'/'30d'/'90d'，空 = 不套用）。
  // 打开时套用一次，避免上次下钻的范围残留影响下一次手动打开。
  initialRange: { type: String, default: '' },
  // 概览 M5 下钻带入的状态集合（如 ['failed','timeout']），空 = 不套用。
  initialStatuses: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:visible'])

const functions = ref([])
const list = ref([])
const filter = ref({ functionId: '', status: '', statuses: [], from: 0, to: 0 })
// appliedFilter 记录「上次查询生效」的筛选快照：loadList 发起前浅拷贝 filter 并
// 复制 statuses 数组，成功返回后写入。用户改了控件但未点查询时，导出仍按上次生效
// 条件生成，避免导出内容与屏幕列表悄悄不一致（M5 下钻改状态后尤其明显）。
const appliedFilter = ref(null)
const dateRange = ref(null)
// 统计随筛选查询一并拉取（loadList 时并行）
const stats = ref({
  totalCount: 0,
  successCount: 0,
  failedCount: 0,
  canceledCount: 0,
  timeoutCount: 0,
  totalCostUsd: 0,
  totalInputTokens: 0,
  totalOutputTokens: 0,
  totalCacheReadTokens: 0,
  totalCacheCreationTokens: 0,
  totalDurationMs: 0,
  byFunction: []
})

// 排行排序维度：次数（默认）/ 成本 / token，仅前端排序，不改后端返回顺序
const rankSort = ref('count')

// 排序数值归一：缺失字段按 0 参与比较，避免 undefined 相减产生 NaN 打乱排序
const num = (v) => Number(v) || 0

// 排行表排序：稳定排序保留同值时的原有相对顺序
const rankedFunctions = computed(() => {
  const rows = [...(stats.value.byFunction || [])]
  const key = rankSort.value
  return rows.sort((a, b) => {
    if (key === 'cost') return num(b.totalCostUsd) - num(a.totalCostUsd)
    if (key === 'tokens') return num(b.totalTokens) - num(a.totalTokens)
    return num(b.count) - num(a.count)
  })
})

// 单个功能项成本占当前筛选总成本的百分比（总成本为 0 时返回 0，避免 NaN）
const costShare = (row) => {
  const total = num(stats.value.totalCostUsd)
  if (!total) return '0.0'
  return ((num(row.totalCostUsd) / total) * 100).toFixed(1)
}

// token 缩写与概览成本卡一致，便于跨面板对照
const formatTokens = (value) => {
  const n = num(value)
  if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}

// 多状态筛选的中文标签（用于筛选行上的可关闭标签）
const statusesLabel = computed(() => {
  const map = { success: '成功', failed: '失败', canceled: '已取消', timeout: '超时' }
  return filter.value.statuses.map((s) => map[s] || s).join(' + ')
})

// 详情抽屉状态
const detailVisible = ref(false)
const detailTitle = ref('')
const detailOutput = ref('')
const detailLoading = ref(false)

const loadFunctions = async () => {
  try {
    functions.value = (await GetAiFunctions()) || []
  } catch {
    // 功能列表加载失败不阻断历史查询（功能名直接用历史记录快照）
  }
}

const loadList = async () => {
  // 发起前先取筛选快照：浅拷贝当前控件值 + 复制 statuses 数组，成功返回后才写入
  // appliedFilter，失败不污染「上次生效」条件。
  const snapshot = {
    ...filter.value,
    statuses: [...(filter.value.statuses || [])]
  }
  try {
    // 列表与统计并行拉取，保证统计与列表同筛选范围
    const [hist, stat] = await Promise.all([
      GetAiTaskHistory(snapshot),
      GetAiTaskHistoryStats(snapshot)
    ])
    list.value = hist || []
    stats.value = stat || stats.value
    appliedFilter.value = snapshot
  } catch (e) {
    ElMessage.error('加载历史失败: ' + (e?.message || String(e)))
  }
}

// 总 token = 四分项相加（不合并口径，tooltip 展示分项）
const totalTokens = () =>
  (stats.value.totalInputTokens || 0) +
  (stats.value.totalOutputTokens || 0) +
  (stats.value.totalCacheReadTokens || 0) +
  (stats.value.totalCacheCreationTokens || 0)

// 导出通用流程：后端生成文本 -> SaveFileDialog 选路径 -> SaveFile 落盘
const exportReport = async (fetchText, defaultFilename, filters, label) => {
  // 导出与可见列表同源：优先用「上次查询生效」的快照，首次无快照才回落当前控件。
  const exportFilter = appliedFilter.value || filter.value
  let text
  try {
    text = await fetchText(exportFilter)
  } catch (e) {
    ElMessage.error('生成' + label + '失败: ' + (e?.message || String(e)))
    return
  }
  let path
  try {
    path = await SaveFileDialog(defaultFilename, filters)
  } catch {
    // runtime 不可用时静默（保存对话框被取消不算错误）
    return
  }
  if (!path) return // 用户取消
  try {
    await SaveFile(path, text, 'utf-8')
    ElMessage.success(label + '已保存: ' + path)
  } catch (e) {
    ElMessage.error('保存' + label + '失败: ' + (e?.message || String(e)))
  }
}

const exportCsv = () =>
  exportReport(ExportAiTaskHistoryCSV, 'ai-task-history.csv', [{ DisplayName: 'CSV 文件', Pattern: '*.csv' }], 'CSV')

const exportMarkdown = () =>
  exportReport(ExportAiTaskHistoryMarkdown, 'ai-task-history-report.md', [{ DisplayName: 'Markdown 文件', Pattern: '*.md' }], 'Markdown 报告')

// 概览卡片下钻带入的时间窗：换算成本地「今日 00:00」或「本周一 00:00」起，
// 与后端 Stats/List 的 StartedAt 窗口口径一致；结束时间不设上限（to=0）。
const applyInitialRange = () => {
  // 每次打开先复位下钻相关筛选：单值状态清空、多状态按下钻入参套用，
  // 避免上次下钻残留（M5 带入 failed+timeout 后手动重开不应仍被限制）。
  filter.value.status = ''
  filter.value.statuses = Array.isArray(props.initialStatuses) ? [...props.initialStatuses] : []
  if (!props.initialRange) {
    filter.value.from = 0
    filter.value.to = 0
    dateRange.value = null
    return
  }
  const now = new Date()
  const dayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  let from = dayStart.getTime()
  if (props.initialRange === 'week') {
    const offset = (dayStart.getDay() + 6) % 7
    dayStart.setDate(dayStart.getDate() - offset)
    from = dayStart.getTime()
  } else {
    // 「近 N 天」= 今天 00:00 往前 N−1 天（含今天共 N 个自然日），档位 key 由概览卡片下钻带入。
    // 边界：下钻只传合法五档（today/week/7d/30d/90d）；未知档位不套用偏移、前端按今日兜底，
    // 与后端 normalizeOverviewRange 回落 week 的归一化不同源，不做同源假设。
    const offsets = { '7d': 6, '30d': 29, '90d': 89 }
    const offset = offsets[props.initialRange]
    if (offset !== undefined) {
      const start = new Date(dayStart)
      start.setDate(start.getDate() - offset)
      from = start.getTime()
    }
  }
  filter.value.from = from
  filter.value.to = 0
  // 日期选择器 value-format="x"，v-model 是毫秒字符串数组
  dateRange.value = [String(from), String(Date.now())]
}

// 打开时加载功能列表与历史（destroy-on-close 下每次打开重新加载，含最新归档）
const onOpen = () => {
  applyInitialRange()
  loadFunctions()
  loadList()
}

// 单值状态选择会覆盖多状态筛选：后端 Statuses 非空时优先于 Status，
// 若不清除，用户选了「成功」仍会按「失败+超时」查询。
const onStatusChange = () => {
  filter.value.statuses = []
}

// 清除下钻带入的多状态筛选并立即刷新列表
const clearStatuses = () => {
  filter.value.statuses = []
  loadList()
}

// 时间范围变更：datetimerange 的 value-format="x" 返回毫秒字符串数组
const onDateChange = (val) => {
  if (val && val.length === 2) {
    filter.value.from = Number(val[0])
    filter.value.to = Number(val[1])
  } else {
    filter.value.from = 0
    filter.value.to = 0
  }
}

// 行点击展开输出详情（懒加载全量读取归档输出文件）
const onRowClick = (row) => {
  viewOutput(row)
}

const viewOutput = async (row) => {
  detailTitle.value = `${row.name} · ${fmtTime(row.finishedAt)}`
  detailVisible.value = true
  detailLoading.value = true
  detailOutput.value = ''
  try {
    detailOutput.value = await GetAiTaskHistoryOutput(row.id)
  } catch (e) {
    detailOutput.value = '读取输出失败: ' + (e?.message || String(e))
  } finally {
    detailLoading.value = false
  }
}

const removeOne = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确认删除历史记录「${row.name}」（${fmtTime(row.finishedAt)}）？输出文件一并删除，不可恢复。`,
      '删除历史',
      { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '再想想' }
    )
  } catch {
    return // 用户放弃
  }
  try {
    if (await DeleteAiTaskHistory(row.id)) {
      ElMessage.success('已删除')
      loadList()
    } else {
      ElMessage.warning('记录不存在或删除失败')
    }
  } catch (e) {
    ElMessage.error('删除失败: ' + (e?.message || String(e)))
  }
}

const clearOld = async () => {
  try {
    await ElMessageBox.confirm(
      '确认清理 30 天前的全部历史记录（元数据与输出文件一并删除）？',
      '批量清理',
      { type: 'warning', confirmButtonText: '确认清理', cancelButtonText: '再想想' }
    )
  } catch {
    return
  }
  try {
    const n = await ClearAiTaskHistory({ olderThanDays: 30 })
    ElMessage.success(`已清理 ${n} 条`)
    loadList()
  } catch (e) {
    ElMessage.error('清理失败: ' + (e?.message || String(e)))
  }
}

// 归档事件：面板打开时收到 ai-task:archived 刷新列表（未打开不主动拉取，避免每次任务完成都拉）
const onArchived = () => {
  if (props.visible) {
    loadList()
  }
}

const statusLabel = (s) => ({
  success: '成功',
  failed: '失败',
  canceled: '已取消',
  timeout: '超时'
}[s] || s)
const statusType = (s) => ({
  success: 'success',
  failed: 'danger',
  canceled: 'info',
  timeout: 'warning'
}[s] || 'info')

const fmtTime = (ms) => {
  if (!ms) return '—'
  const d = new Date(ms)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
const fmtDuration = (ms) => {
  if (!ms) return '—'
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}
const fmtSize = (bytes) => {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

onMounted(() => {
  EventsOn('ai-task:archived', onArchived)
})
onBeforeUnmount(() => {
  EventsOff('ai-task:archived')
})
</script>

<style scoped>
.ai-history-dialog :deep(.el-dialog__body) {
  padding: 12px 20px 20px;
}
.history-filter {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.filter-fn {
  width: 180px;
}
.filter-date {
  width: 360px !important;
}
.filter-status {
  width: 120px;
}
.filter-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-tertiary, #909399);
}
/* 统计卡片区：4 个数字卡 + 功能排行，随筛选刷新 */
.history-stats {
  display: flex;
  align-items: stretch;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.stat-card {
  min-width: 110px;
  padding: 8px 12px;
  border: 1px solid var(--border-color, #dcdfe6);
  border-radius: var(--radius-md, 6px);
  background: var(--bg-tertiary, #f5f7fa);
}
.stat-value {
  font-size: 16px;
  font-weight: 600;
  line-height: 1.4;
}
.stat-sub {
  font-size: 12px;
  font-weight: 400;
  color: var(--text-tertiary, #909399);
}
.stat-label {
  font-size: 12px;
  color: var(--text-tertiary, #909399);
}
.fn-rank {
  flex: 1;
  min-width: 300px;
  padding: 4px;
}
.fn-rank-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 2px 4px;
}
.fn-rank-title {
  font-size: 12px;
  color: var(--text-secondary, #606266);
}
.fn-canceled {
  margin-left: 2px;
  font-size: 11px;
  color: var(--text-tertiary, #909399);
}
.fn-share {
  margin-left: 4px;
  font-size: 11px;
  color: var(--text-tertiary, #909399);
}
.detail-loading {
  padding: 24px;
  text-align: center;
  color: var(--text-tertiary, #909399);
}
.detail-output {
  margin: 0;
  padding: 12px;
  font-size: 12px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: 'Cascadia Code', Consolas, monospace;
  background: var(--bg-tertiary, #f5f7fa);
  border-radius: var(--radius-md, 6px);
  min-height: 200px;
}
</style>
