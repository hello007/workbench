<template>
  <el-dialog
    v-model="visible"
    title="审计日志"
    width="min(960px, 92vw)"
    top="6vh"
    :close-on-click-modal="true"
    :close-on-press-escape="true"
    class="audit-history-dialog"
    append-to-body
    @open="onOpen"
  >
    <div class="audit-history-body">
      <!-- 过滤行：事件类型多选 + 时间窗 + 刷新。条件变更即重查并回到第 1 页 -->
      <div class="audit-history-filters">
        <el-select
          v-model="eventFilter"
          multiple
          collapse-tags
          collapse-tags-tooltip
          clearable
          placeholder="全部事件"
          size="small"
          class="audit-filter-events"
          @change="reloadFromFirstPage"
        >
          <el-option v-for="meta in EVENT_METAS" :key="meta.value" :label="meta.label" :value="meta.value" />
        </el-select>
        <el-select
          v-model="timeRange"
          size="small"
          class="audit-filter-time"
          @change="reloadFromFirstPage"
        >
          <el-option label="全部时间" :value="TIME_ALL" />
          <el-option label="近 24 小时" :value="TIME_24H" />
          <el-option label="近 7 天" :value="TIME_7D" />
          <el-option label="近 30 天" :value="TIME_30D" />
        </el-select>
        <el-button size="small" :loading="loading" @click="load">刷新</el-button>
      </div>

      <!-- 审计行表：时间倒序（最新在前）。操作者/目标令牌名经名册 join 补全，
           已撤销令牌降级展示；actorSource 为空（认证/令牌生命周期事件无操作者
           归因语义）显示「—」 -->
      <el-table :data="records" size="small" v-loading="loading" class="audit-history-table" empty-text="暂无审计记录">
        <el-table-column label="时间" :width="TIME_COL_WIDTH">
          <template #default="{ row }">{{ formatUnixTime(row.ts) }}</template>
        </el-table-column>
        <el-table-column label="事件" :width="EVENT_COL_WIDTH">
          <template #default="{ row }">
            <el-tag size="small" :type="eventMeta(row.event).tagType">{{ eventMeta(row.event).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="来源" :width="SOURCE_COL_WIDTH">
          <template #default="{ row }">{{ sourceLabel(row.actorSource) }}</template>
        </el-table-column>
        <el-table-column label="操作者" :width="TOKEN_COL_WIDTH">
          <template #default="{ row }">{{ actorLabel(row) }}</template>
        </el-table-column>
        <el-table-column label="目标令牌" :width="TOKEN_COL_WIDTH">
          <template #default="{ row }">{{ targetLabel(row) }}</template>
        </el-table-column>
        <el-table-column label="来源 IP" min-width="120">
          <template #default="{ row }">
            <span class="audit-cell-ellipsis" :title="row.ip || row.remoteAddr">{{ sourceIPLabel(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="连接" :width="CONN_COL_WIDTH">
          <template #default="{ row }">{{ row.connId || '—' }}</template>
        </el-table-column>
      </el-table>

      <!-- 分页：Offset/Limit 透传后端（后端钳制上限），Total 为过滤后总数 -->
      <div class="audit-history-footer">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          small
          @current-change="load"
          @size-change="reloadFromFirstPage"
        />
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { ref } from 'vue'
import { QueryAuditLog, ListWebTokens } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'

// 时间窗预设（相对档，查询时按当前时间换算 From unix 秒；TIME_ALL = 不限）
const TIME_ALL = 'all'
const TIME_24H = 24 * 3600
const TIME_7D = 7 * 24 * 3600
const TIME_30D = 30 * 24 * 3600

// 事件元信息表：值与 service/audit_log.go AuditEvent* 常量严格一致（新增审计
// 事件须同步此处），tagType 走 EP 语义色
const EVENT_METAS = [
  { value: 'auth_success', label: '认证成功', tagType: 'success' },
  { value: 'auth_failure', label: '认证失败', tagType: 'danger' },
  { value: 'token_create', label: '新建令牌', tagType: 'primary' },
  { value: 'token_revoke', label: '撤销令牌', tagType: 'warning' },
  { value: 'token_rotate', label: '轮换令牌', tagType: 'warning' },
  { value: 'token_migrate', label: '令牌迁移', tagType: 'info' },
  { value: 'conn_kick', label: '踢出连接', tagType: 'danger' },
  { value: 'ip_ban', label: '自动封禁IP', tagType: 'danger' },
  { value: 'ip_unban', label: '解封IP', tagType: 'success' }
]

// 列宽常量（模板引用；数值列/标签列定宽防抖动，IP 列 min-width 弹性）
const TIME_COL_WIDTH = 160
const EVENT_COL_WIDTH = 96
const SOURCE_COL_WIDTH = 76
const TOKEN_COL_WIDTH = 110
const CONN_COL_WIDTH = 70

const visible = defineModel({ type: Boolean, default: false })

const loading = ref(false)
const records = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const eventFilter = ref([])
const timeRange = ref(TIME_ALL)
// tokenID → 名称映射（打开时经 ListWebTokens 拉一次；含已撤销记录——操作者/
// 目标令牌名补全与「已撤销令牌」降级展示的数据源）
const tokenNames = ref({})

function eventMeta(event) {
  return EVENT_METAS.find((m) => m.value === event) || { label: event || '未知', tagType: 'info' }
}

function sourceLabel(source) {
  if (source === 'desktop') return '桌面'
  if (source === 'rpc') return '浏览器'
  if (source === 'system') return '系统'
  return '—'
}

// 操作者：actorTokenId 仅 conn_kick 携带（desktop 行为空 → 「桌面用户」；
// rpc 行经名册补全令牌名，miss（已撤销）降级；system 为服务端自动触发
// （ip_ban 自动封禁，无人类操作者）；其余事件无操作者语义 → 「—」）
function actorLabel(row) {
  if (!row.actorSource) return '—'
  if (row.actorSource === 'desktop') return '桌面用户'
  if (row.actorSource === 'system') return '系统'
  if (!row.actorTokenId) return '浏览器'
  return tokenNames.value[row.actorTokenId] || '已撤销令牌'
}

// 来源 IP 列：认证事件走 remoteAddr（原始地址含端口），ip_ban/ip_unban 事件
// 走 ip 字段（剥端口归一后形态，AuditRecord 五期新增字段）；ip_ban 行尾追加
// 触发时窗口内失败计数（与审计行 reason 文案对号入座）
function sourceIPLabel(row) {
  const ip = row.ip || row.remoteAddr
  if (!ip) return '—'
  if (row.event === 'ip_ban' && row.failCount) {
    return `${ip}（失败 ${row.failCount} 次）`
  }
  return ip
}

// 目标令牌：tokenId 经名册补全名，miss 降级「已撤销令牌」；name 后端已补全
// （conn_kick 行），优先直用；两者皆空（认证失败行 tokenId 空）→ 「—」
function targetLabel(row) {
  if (row.name) return row.name
  if (row.tokenId) return tokenNames.value[row.tokenId] || '已撤销令牌'
  return '—'
}

function formatUnixTime(sec) {
  if (!sec) return '—'
  const d = new Date(sec * 1000)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

// 时间窗换算：相对档每次查询按当前时间现算 From（防弹窗驻留期间窗口漂移）
function fromUnix() {
  if (timeRange.value === TIME_ALL) return 0
  return Math.floor(Date.now() / 1000) - timeRange.value
}

async function loadTokenNames() {
  try {
    const items = await ListWebTokens()
    const map = {}
    for (const item of items || []) {
      if (item?.record?.id) map[item.record.id] = item.record.name
    }
    tokenNames.value = map
  } catch (err) {
    // 名称补全为展示增强，失败降级 ID 直显（「已撤销令牌」形态），不阻断主查询
    tokenNames.value = {}
  }
}

async function load() {
  loading.value = true
  try {
    const res = await QueryAuditLog({
      Events: eventFilter.value,
      From: fromUnix(),
      To: 0,
      TokenID: '',
      Offset: (page.value - 1) * pageSize.value,
      Limit: pageSize.value
    })
    total.value = res?.Total ?? 0
    // 越界页（过滤变严后 Total 收缩）回退末页重查一次
    if (res?.Total > 0 && !res?.Records?.length && page.value > 1) {
      page.value = Math.ceil(res.Total / pageSize.value)
      loading.value = false
      return load()
    }
    records.value = res?.Records ?? []
  } catch (err) {
    // handleError 双参签名（prefix, error）——单参调用会把错误对象当 prefix，
    // error 形参 undefined 走空消息分支产出 "[object Object]"（error.js 明令禁止）
    handleError('查询审计日志失败: ', err)
  } finally {
    loading.value = false
  }
}

// 条件变更：回第 1 页再查（分页位置随过滤语义失效）
function reloadFromFirstPage() {
  page.value = 1
  load()
}

async function onOpen() {
  await loadTokenNames()
  await load()
}
</script>

<style scoped>
.audit-history-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.audit-history-filters {
  display: flex;
  align-items: center;
  gap: 8px;
}

.audit-filter-events {
  flex: 1;
  min-width: 0;
}

.audit-filter-time {
  width: 130px;
  flex-shrink: 0;
}

.audit-history-table {
  width: 100%;
}

.audit-cell-ellipsis {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.audit-history-footer {
  display: flex;
  justify-content: flex-end;
}
</style>
