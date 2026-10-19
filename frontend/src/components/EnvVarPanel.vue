<template>
  <div class="env-var-panel">
    <div class="tool-panel-titlebar">
      <span class="tool-panel-title">环境变量</span>
      <el-button
        size="small"
        :loading="loading"
        @click="loadList"
      >
        <el-icon class="ev-refresh-icon"><Refresh /></el-icon>
        刷新
      </el-button>
    </div>

    <!-- 总览：来源数与变量数 + 脱敏交互说明 -->
    <div class="ev-meta">
      <span>共 {{ groups.length }} 个来源、{{ totalVarCount }} 个变量</span>
      <span class="ev-meta-hint">敏感变量默认打码，点击眼睛图标临时显示；复制始终复制真实值</span>
    </div>

    <!-- 来源分组：组数随返回动态（Windows 三组 system/user/process，Linux 仅 process 一组） -->
    <el-tabs v-if="groups.length > 0" v-model="activeTab">
      <el-tab-pane
        v-for="group in groups"
        :key="group.source"
        :name="group.source"
        :label="group.name"
      >
        <!-- 组级错误：该来源枚举失败不拖垮整体，warning 行就地提示、其余组正常（不打 ElMessage） -->
        <div v-if="group.error" class="ev-group-error">
          <el-icon><WarningFilled /></el-icon>
          <span>{{ group.error }}</span>
        </div>
        <template v-else>
          <div class="ev-group-toolbar">
            <el-input
              v-model="filters[group.source]"
              size="small"
              clearable
              placeholder="按名称或值过滤，大小写不敏感"
              class="ev-filter-input"
            />
            <span class="ev-group-count">{{ filteredVars(group).length }} / {{ (group.vars || []).length }} 个变量</span>
          </div>
          <el-table
            :data="filteredVars(group)"
            size="small"
            border
            max-height="420"
            :empty-text="loading ? '加载中…' : '未找到匹配的变量'"
            class="ev-table"
          >
            <el-table-column prop="name" label="名称" min-width="200" show-overflow-tooltip />
            <el-table-column label="值" min-width="300">
              <template #default="{ row }">
                <div class="ev-value-cell">
                  <!-- 敏感项默认打码，点击眼睛临时显值（仅本行本会话）；非敏感项直接展示 -->
                  <template v-if="row.sensitive && !isRevealed(group, row)">
                    <span class="ev-value-masked">{{ maskValue(row.value) }}</span>
                    <el-button
                      class="ev-eye-btn"
                      size="small"
                      link
                      title="显示值"
                      @click="toggleReveal(group, row)"
                    >
                      <el-icon><View /></el-icon>
                    </el-button>
                  </template>
                  <template v-else>
                    <span class="ev-value">{{ plainValue(row) }}</span>
                    <el-button
                      v-if="row.sensitive"
                      class="ev-eye-btn"
                      size="small"
                      link
                      title="隐藏值"
                      @click="toggleReveal(group, row)"
                    >
                      <el-icon><Hide /></el-icon>
                    </el-button>
                  </template>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="操作" fixed="right" width="80">
              <template #default="{ row }">
                <el-button size="small" link @click="copyValue(row)">复制</el-button>
              </template>
            </el-table-column>
          </el-table>
        </template>
      </el-tab-pane>
    </el-tabs>
    <div v-else-if="!loading" class="ev-empty">暂无可展示的环境变量</div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Hide, Refresh, View, WarningFilled } from '@element-plus/icons-vue'
import { ListEnvVars } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'

const loading = ref(false)
const groups = ref([])
const activeTab = ref('')

// 各来源独立的过滤关键字（键为分组 source），前端过滤：名称/值大小写不敏感子串包含
const filters = reactive({})

// 临时显值态：键 = source + NUL + name（NUL 非环境变量名合法字符，保证键零冲突）。
// 仅本行本会话生效，不做持久开关；刷新后重置——重枚举后值可能已变，避免旧显值误解新值
const revealedKeys = ref([])

// 枚举序号：快速重复刷新时旧响应后到会覆盖新列表，过期响应整体丢弃
let loadSeq = 0

const loadList = async () => {
  const seq = ++loadSeq
  loading.value = true
  try {
    const list = await ListEnvVars()
    if (seq !== loadSeq) return
    groups.value = list || []
    revealedKeys.value = []
    // 保持当前选中 tab（刷新后仍在分组内），不在分组内（首次打开）时回落首个分组
    const sources = groups.value.map((g) => g.source)
    if (!sources.includes(activeTab.value)) {
      activeTab.value = sources[0] || ''
    }
  } catch (e) {
    if (seq !== loadSeq) return
    // 全部来源失败时后端才整体报错（E_ENV_ENUM_FAILED），message 已自解释；
    // 单来源失败走分组 Error 字段就地展示，不进此分支
    handleError('加载失败: ', e)
  } finally {
    if (seq === loadSeq) {
      loading.value = false
    }
  }
}

// 面板挂载即枚举（工具切换 = 卸载重挂，等价原弹窗 destroy-on-close + @open 语义；
// 注册表来源可能被外部修改，配合手动刷新兜底）
onMounted(() => {
  loadList()
})

const totalVarCount = computed(() =>
  groups.value.reduce((sum, g) => sum + (g.vars ? g.vars.length : 0), 0)
)

// 前端过滤：名称/值大小写不敏感子串包含。组内顺序沿用后端按名称升序，不再重排
const filteredVars = (group) => {
  const vars = group.vars || []
  const kw = (filters[group.source] || '').trim().toLowerCase()
  if (!kw) return vars
  return vars.filter(
    (v) =>
      String(v.name || '').toLowerCase().includes(kw) ||
      String(v.value || '').toLowerCase().includes(kw)
  )
}

// 脱敏展示：保留首字符 + 固定打码位；空值仅打码位
const MASK_CHARS = '••••••'
const maskValue = (value) => {
  const v = value || ''
  return (v ? v.slice(0, 1) : '') + MASK_CHARS
}

// 空值占位（FOO= 形态的空值变量合法存在，与有值行区分）
const plainValue = (row) => (row.value === '' || row.value == null ? '（空）' : row.value)

const revealKey = (group, row) => `${group.source}\u0000${row.name}`

const isRevealed = (group, row) => revealedKeys.value.includes(revealKey(group, row))

// 用数组整体重赋值而非 Set：增删时稳定触发响应更新（与 PortListPanel pendingPids 同款）
const toggleReveal = (group, row) => {
  const key = revealKey(group, row)
  revealedKeys.value = isRevealed(group, row)
    ? revealedKeys.value.filter((k) => k !== key)
    : [...revealedKeys.value, key]
}

// 复制单值：始终复制真实 value（脱敏仅是展示层行为，与打码状态无关）
const copyValue = async (row) => {
  try {
    await navigator.clipboard.writeText(row.value || '')
    ElMessage.success(`已复制 ${row.name}`)
  } catch (e) {
    ElMessage.error('复制失败: ' + (e?.message || String(e)))
  }
}
</script>

<style scoped>
.ev-refresh-icon {
  margin-right: 4px;
}

.ev-meta {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-md);
  font-size: 12px;
  color: var(--text-tertiary);
}

.ev-group-toolbar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-sm);
}

.ev-filter-input {
  width: 240px;
}

.ev-group-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-tertiary);
}

.ev-value-cell {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.ev-value {
  color: var(--text-primary);
  word-break: break-all;
  white-space: pre-wrap;
}

.ev-value-masked {
  color: var(--text-tertiary);
  letter-spacing: 1px;
  user-select: none;
}

.ev-eye-btn {
  flex-shrink: 0;
}

/* 组级错误提示：warning 语义色经 color-mix 从变量派生，随主题切换不脱钩 */
.ev-group-error {
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

.ev-empty {
  padding: var(--spacing-xl) 0;
  text-align: center;
  font-size: 13px;
  color: var(--text-tertiary);
}

/* 列表区限高滚动：进程环境变量可达数百行，el-table max-height 内部滚动（表头吸顶），
   面板整体滚动由 ToolboxPanel .toolbox-main 容器兜底 */
</style>
