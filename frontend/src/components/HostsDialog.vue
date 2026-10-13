<template>
  <el-dialog
    :model-value="modelValue"
    width="min(720px, 85vw)"
    class="hosts-dialog"
    append-to-body
    destroy-on-close
    @update:model-value="$emit('update:modelValue', $event)"
    @open="onOpen"
  >
    <template #header>
      <div class="hosts-header">
        <span class="hosts-title">hosts 文件</span>
        <el-button
          size="small"
          :loading="loading"
          @click="loadHosts"
        >
          <el-icon class="hosts-refresh-icon"><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </template>

    <!-- 总览：实际读取路径 + 展示口径说明（路径随平台不同，排障时确认读的是哪个文件） -->
    <div class="hosts-meta">
      <span class="hosts-path" :title="view?.path">{{ view?.path || '—' }}</span>
      <span class="hosts-meta-hint">映射行展示 IP / 主机名 / 行内注释，注释与空行默认隐藏；复制单行复制原始行文本</span>
    </div>

    <!-- 组级错误：读取失败不整体报错，warning 行就地提示（单来源失败形态，对齐 EnvVarDialog） -->
    <div v-if="view?.error" class="hosts-error">
      <el-icon><WarningFilled /></el-icon>
      <span>{{ view.error }}</span>
    </div>
    <template v-else>
      <div class="hosts-toolbar">
        <el-input
          v-model="keyword"
          size="small"
          clearable
          placeholder="按 IP 或主机名过滤，大小写不敏感"
          class="hosts-filter-input"
        />
        <el-checkbox v-model="showComments" size="small">显示注释</el-checkbox>
        <span class="hosts-count">{{ filteredEntries.length }} / {{ visibleEntries.length }} 条</span>
      </div>
      <el-table
        :data="filteredEntries"
        size="small"
        border
        max-height="420"
        :empty-text="tableEmptyText"
        class="hosts-table"
      >
        <el-table-column label="IP" width="160">
          <template #default="{ row }">
            <span v-if="row.kind === 'entry'" class="hosts-ip">{{ row.ip }}</span>
          </template>
        </el-table-column>
        <el-table-column label="主机名" min-width="180">
          <template #default="{ row }">
            <!-- 映射行展示主机名列表；未识别行灰色展示原文 + 标注（不静默丢弃），空行占位说明 -->
            <template v-if="row.kind === 'entry'">
              <span class="hosts-hostnames">{{ (row.hostnames || []).join(' ') }}</span>
            </template>
            <template v-else-if="row.kind === 'unknown'">
              <el-tag size="small" type="info" class="hosts-unknown-tag">未识别</el-tag>
              <span class="hosts-line-raw">{{ row.raw }}</span>
            </template>
            <template v-else-if="row.kind === 'blank'">
              <span class="hosts-line-raw">（空行）</span>
            </template>
          </template>
        </el-table-column>
        <el-table-column label="注释" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <!-- entry 的行内注释；纯注释行原文整行落在注释列（含 # 前缀，与原文一致） -->
            <span v-if="row.kind === 'entry' && row.comment" class="hosts-comment">{{ row.comment }}</span>
            <span v-else-if="row.kind === 'comment'" class="hosts-line-raw">{{ row.raw }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" fixed="right" width="150">
          <template #default="{ row }">
            <el-button v-if="row.kind !== 'blank'" size="small" link @click="copyRaw(row)">复制单行</el-button>
            <el-button v-if="row.kind === 'entry'" size="small" link @click="copyIp(row)">复制 IP</el-button>
          </template>
        </el-table-column>
      </el-table>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, WarningFilled } from '@element-plus/icons-vue'
import { GetHosts } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'

defineProps({
  modelValue: { type: Boolean, default: false }
})
defineEmits(['update:modelValue'])

const loading = ref(false)
const view = ref(null)
const keyword = ref('')
// 注释/空行默认隐藏，「显示注释」开关切回展示（未识别行恒展示——非空非注释行不因开关消失）
const showComments = ref(false)

// 加载序号：快速重复刷新时旧响应后到会覆盖新列表，过期响应整体丢弃（对齐 EnvVarDialog）
let loadSeq = 0

const loadHosts = async () => {
  const seq = ++loadSeq
  loading.value = true
  try {
    const data = await GetHosts()
    if (seq !== loadSeq) return
    view.value = data || null
  } catch (e) {
    if (seq !== loadSeq) return
    // 后端读取失败走组级 Error 字段不整体报错；此分支兜底意外异常（如绑定缺失）
    handleError('加载失败: ', e)
  } finally {
    if (seq === loadSeq) {
      loading.value = false
    }
  }
}

// destroy-on-close 下每次打开重新读取（hosts 可能被外部修改，配合手动刷新兜底）
const onOpen = () => {
  loadHosts()
}

// 注释/空行随开关显隐；entry 与未识别行恒展示
const visibleEntries = computed(() => {
  const entries = view.value?.entries || []
  if (showComments.value) return entries
  return entries.filter((e) => e.kind === 'entry' || e.kind === 'unknown')
})

// 关键字过滤：entry 匹配 IP 或主机名（大小写不敏感子串包含）；注释/空行/未识别行
// 按原文匹配（注释行原文含 # 前缀，按原文口径一致）
const filteredEntries = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return visibleEntries.value
  return visibleEntries.value.filter((e) => {
    if (e.kind === 'entry') {
      return (
        String(e.ip || '').toLowerCase().includes(kw) ||
        (e.hostnames || []).some((h) => String(h || '').toLowerCase().includes(kw))
      )
    }
    return String(e.raw || '').toLowerCase().includes(kw)
  })
})

const tableEmptyText = computed(() => {
  if (loading.value) return '加载中…'
  return (view.value?.entries || []).length === 0 ? 'hosts 文件为空' : '未找到匹配的条目'
})

// 复制单行：原始行文本（含注释与原样空白语义，非展示拼装值）
const copyRaw = async (row) => {
  try {
    await navigator.clipboard.writeText(row.raw || '')
    ElMessage.success('已复制单行')
  } catch (e) {
    ElMessage.error('复制失败: ' + (e?.message || String(e)))
  }
}

// 复制 IP：仅 entry 行提供（文件中的原样地址 token）
const copyIp = async (row) => {
  try {
    await navigator.clipboard.writeText(row.ip || '')
    ElMessage.success(`已复制 ${row.ip}`)
  } catch (e) {
    ElMessage.error('复制失败: ' + (e?.message || String(e)))
  }
}
</script>

<style scoped>
.hosts-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding-right: var(--spacing-lg);
}

.hosts-title {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.hosts-refresh-icon {
  margin-right: 4px;
}

.hosts-meta {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-md);
  font-size: 12px;
  color: var(--text-tertiary);
}

.hosts-path {
  color: var(--text-secondary);
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  word-break: break-all;
}

.hosts-toolbar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-sm);
}

.hosts-filter-input {
  width: 240px;
}

.hosts-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
}

.hosts-ip,
.hosts-hostnames {
  color: var(--text-primary);
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  word-break: break-all;
}

.hosts-comment {
  color: var(--text-tertiary);
  word-break: break-all;
}

.hosts-line-raw {
  color: var(--text-tertiary);
  word-break: break-all;
}

.hosts-unknown-tag {
  margin-right: var(--spacing-xs);
}

/* 组级错误提示：warning 语义色经 color-mix 从变量派生，随主题切换不脱钩 */
.hosts-error {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  margin-bottom: var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: 13px;
  color: var(--warning-color);
  background: color-mix(in srgb, var(--warning-color) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--warning-color) 35%, transparent);
}

/* 列表区限高滚动：el-table max-height 内部滚动（表头吸顶），超出部分由弹窗 body 滚动兜底 */
.hosts-dialog :deep(.el-dialog__body) {
  padding: var(--spacing-md) var(--spacing-lg) var(--spacing-lg);
  max-height: 72vh;
  overflow-y: auto;
}
</style>
