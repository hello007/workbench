<!-- frontend/src/components/BranchCompareCommitTable.vue -->
<!-- 分支对齐提交表（R3 + R8）：左列 A 提交、右列 B 提交，配对提交同行显示；
     匹配徽标区分三级匹配方式（hash=info / patch-id=warning 疑似拣选 / subject=info）；
     搜索框按 hash / subject / author 子串过滤（任一侧命中即整行显示，纯前端）。
     行样式：匹配行灰显、A 独有蓝 accent、B 独有橙 accent（走设计令牌，暗色安全）。 -->
<template>
  <div class="bc-commit-table">
    <div class="table-toolbar">
      <el-input
        v-model="keyword"
        class="input-w-md"
        size="small"
        clearable
        placeholder="搜索 hash / 提交说明 / 作者"
        :prefix-icon="Search"
      />
      <span class="row-count">{{ visibleRows.length }}/{{ rows.length }}</span>
    </div>
    <el-table
      :data="visibleRows"
      :row-class-name="rowClassName"
      height="100%"
      @row-click="onRowClick"
    >
      <el-table-column :label="'分支 A：' + branchA" min-width="220">
        <template #default="{ row }">
          <div v-if="row.a" class="commit-cell">
            <span class="commit-sha">{{ row.a.shortHash }}</span>
            <span class="commit-subject" :title="row.a.subject">{{ row.a.subject }}</span>
            <span class="commit-meta">{{ row.a.author }} · {{ formatDate(row.a.date) }}</span>
          </div>
          <span v-else class="cell-empty">—</span>
        </template>
      </el-table-column>
      <el-table-column label="对齐" width="130">
        <template #default="{ row }">
          <div class="align-cell">
            <el-tag size="small" :type="sideTagType(row.side)">{{ sideLabel(row.side) }}</el-tag>
            <el-tag
              v-if="badgeOf(row)"
              size="small"
              :type="badgeOf(row).type"
              :class="'badge-' + row.matchedBy"
            >{{ badgeOf(row).label }}</el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="'分支 B：' + branchB" min-width="220">
        <template #default="{ row }">
          <div v-if="row.b" class="commit-cell">
            <span class="commit-sha">{{ row.b.shortHash }}</span>
            <span class="commit-subject" :title="row.b.subject">{{ row.b.subject }}</span>
            <span class="commit-meta">{{ row.b.author }} · {{ formatDate(row.b.date) }}</span>
          </div>
          <span v-else class="cell-empty">—</span>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import dayjs from 'dayjs'
import { Search } from '@element-plus/icons-vue'

defineOptions({ name: 'BranchCompareCommitTable' })

const props = defineProps({
  // 对齐行数组：[{key, a, b, side: 'matched'|'a-only'|'b-only', matchedBy: 'hash'|'patch-id'|'subject'|'none'}]
  rows: { type: Array, default: () => [] },
  branchA: { type: String, default: '' },
  branchB: { type: String, default: '' }
})

const emit = defineEmits(['selectCommit'])

// 搜索关键词（R8）：按 hash / subject / author 子串过滤，任一侧命中即整行保留
const keyword = ref('')

// 匹配行包含行内搜索可见性判定：commit 对象任一字段子串命中（大小写不敏感）
const rowHit = (commit, kw) => {
  if (!commit) return false
  const fields = [commit.hash, commit.shortHash, commit.subject, commit.author, commit.email]
  return fields.some(f => typeof f === 'string' && f.toLowerCase().includes(kw))
}

const visibleRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return props.rows
  return props.rows.filter(row => rowHit(row.a, kw) || rowHit(row.b, kw))
})

// side 中文标签与 tag 类型
const sideLabel = (side) => {
  const map = { matched: '匹配', 'a-only': 'A 独有', 'b-only': 'B 独有' }
  return map[side] || side
}

const sideTagType = (side) => {
  const map = { matched: 'info', 'a-only': 'primary', 'b-only': 'warning' }
  return map[side] || 'info'
}

/**
 * 匹配方式徽标（R3）：patch-id → warning「疑似拣选」（同补丁不同提交，cherry-pick 特征）；
 * hash → info「同哈希」；subject → info「同说明」；none → 不显示。
 */
const badgeOf = (row) => {
  if (row.side !== 'matched') return null
  const map = {
    'patch-id': { type: 'warning', label: '疑似拣选' },
    hash: { type: 'info', label: '同哈希' },
    subject: { type: 'info', label: '同说明' }
  }
  return map[row.matchedBy] || null
}

// 行样式：匹配行灰显（次要视觉），a-only / b-only 以左 accent 强调（CSS 变量派生，暗色安全）
const rowClassName = ({ row }) => {
  if (row.side === 'matched') return 'row-matched'
  if (row.side === 'a-only') return 'row-a-only'
  if (row.side === 'b-only') return 'row-b-only'
  return ''
}

const onRowClick = (row) => {
  emit('selectCommit', row)
}

// date 为 unix 秒
const formatDate = (sec) => {
  if (!sec) return ''
  return dayjs.unix(sec).format('YYYY-MM-DD HH:mm')
}

defineExpose({ rowClassName, sideLabel, sideTagType, badgeOf })
</script>

<style scoped>
.bc-commit-table {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm, 8px);
  min-height: 0;
  height: 100%;
}

.table-toolbar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm, 8px);
}

.input-w-md {
  width: 220px;
}

.row-count {
  font-size: 12px;
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.commit-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  cursor: pointer;
}

.commit-sha {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--primary-color);
}

.commit-subject {
  font-size: 13px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.commit-meta {
  font-size: 12px;
  color: var(--text-tertiary);
}

.cell-empty {
  color: var(--text-tertiary);
}

.align-cell {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}
</style>

<style>
/* el-table 行样式（rowClassName 产出，需全局作用域命中 el-table 内部行） */
.el-table .row-matched {
  --el-table-tr-bg-color: var(--bg-tertiary);
  color: var(--text-secondary);
}

.el-table .row-a-only td:first-child {
  box-shadow: inset 3px 0 0 var(--primary-color);
}

.el-table .row-b-only td:last-child {
  box-shadow: inset -3px 0 0 var(--warning-color);
}
</style>
