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
      >
        <el-option label="成功" value="success" />
        <el-option label="失败" value="failed" />
        <el-option label="已取消" value="canceled" />
        <el-option label="超时" value="timeout" />
      </el-select>
      <el-button size="small" @click="loadList">查询</el-button>
      <el-button size="small" type="warning" plain @click="clearOld">清理 30 天前</el-button>
      <el-button size="small" plain @click="exportCsv">导出 CSV</el-button>
      <el-button size="small" plain @click="exportMarkdown">导出 Markdown</el-button>
      <span class="filter-count">共 {{ list.length }} 条</span>
    </div>

    <!-- 统计卡片区：随筛选查询一并刷新 -->
    <div class="history-stats">
      <div class="stat-card">
        <div class="stat-value">{{ stats.totalCount }} <span class="stat-sub">成功 {{ stats.successCount }}</span></div>
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
      <!-- 功能排行：全部展示 -->
      <div v-if="stats.byFunction && stats.byFunction.length" class="stat-card fn-rank">
        <el-table :data="stats.byFunction" size="small" border max-height="120">
          <el-table-column prop="functionName" label="功能排行" min-width="120" show-overflow-tooltip>
            <template #default="{ row }">{{ row.functionName }}</template>
          </el-table-column>
          <el-table-column prop="count" label="次数" width="60" />
          <el-table-column label="成本" width="80">
            <template #default="{ row }">${{ row.totalCostUsd.toFixed(3) }}</template>
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
import { ref, onMounted, onBeforeUnmount } from 'vue'
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

const props = defineProps({ visible: Boolean })
const emit = defineEmits(['update:visible'])

const functions = ref([])
const list = ref([])
const filter = ref({ functionId: '', status: '', from: 0, to: 0 })
const dateRange = ref(null)
// 统计随筛选查询一并拉取（loadList 时并行）
const stats = ref({
  totalCount: 0,
  successCount: 0,
  totalCostUsd: 0,
  totalInputTokens: 0,
  totalOutputTokens: 0,
  totalCacheReadTokens: 0,
  totalCacheCreationTokens: 0,
  totalDurationMs: 0,
  byFunction: []
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
  try {
    // 列表与统计并行拉取，保证统计与列表同筛选范围
    const [hist, stat] = await Promise.all([
      GetAiTaskHistory(filter.value),
      GetAiTaskHistoryStats(filter.value)
    ])
    list.value = hist || []
    stats.value = stat || stats.value
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
  let text
  try {
    text = await fetchText(filter.value)
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

// 打开时加载功能列表与历史（destroy-on-close 下每次打开重新加载，含最新归档）
const onOpen = () => {
  loadFunctions()
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
