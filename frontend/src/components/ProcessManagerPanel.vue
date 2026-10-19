<template>
  <div class="process-manager-panel">
    <div class="tool-panel-titlebar">
      <span class="tool-panel-title">进程管理</span>
      <el-button
        size="small"
        :loading="loading"
        @click="loadList"
      >
        <el-icon class="pm-refresh-icon"><Refresh /></el-icon>
        刷新
      </el-button>
    </div>

    <!-- 检索区：单输入框（纯数字按端口 / 含非数字按进程名）+ 模式下拉消除歧义 -->
    <div class="pm-search">
      <el-input
        v-model="keyword"
        size="small"
        clearable
        placeholder="输入端口号或进程名关键字，回车检索"
        class="pm-search-input"
        @keyup.enter="loadList"
      />
      <el-select v-model="mode" size="small" class="pm-mode-select">
        <el-option label="自动" :value="MODE_AUTO" />
        <el-option label="端口" :value="MODE_PORT" />
        <el-option label="进程名" :value="MODE_NAME" />
      </el-select>
      <el-button size="small" type="primary" :loading="loading" @click="loadList">
        检索
      </el-button>
      <el-checkbox v-model="killTree" size="small" class="pm-tree-checkbox">
        同时结束子进程（进程树）
      </el-checkbox>
      <span class="pm-count">共 {{ processes.length }} 个进程</span>
    </div>

    <el-table
      :data="processes"
      size="small"
      border
      max-height="440"
      :empty-text="loading ? '检索中…' : '未找到匹配的进程'"
      class="pm-table"
    >
      <el-table-column label="进程名" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="pm-name">{{ row.name }}</span>
          <el-tag v-if="row.isSelf" size="small" type="info" class="pm-name-tag">本应用</el-tag>
          <el-tag v-else-if="row.isSystem" size="small" type="danger" class="pm-name-tag">系统进程</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="pid" label="PID" width="84" />
      <el-table-column label="端口" width="120" show-overflow-tooltip>
        <template #default="{ row }">{{ portText(row) }}</template>
      </el-table-column>
      <el-table-column label="协议" width="80">
        <template #default="{ row }">{{ protocolText(row) }}</template>
      </el-table-column>
      <el-table-column label="状态" width="116">
        <template #default="{ row }">{{ stateText(row) }}</template>
      </el-table-column>
      <el-table-column label="内存" width="100">
        <template #default="{ row }">{{ memoryText(row) }}</template>
      </el-table-column>
      <el-table-column label="操作" fixed="right" width="128">
        <template #default="{ row }">
          <el-button
            size="small"
            link
            :disabled="row.isSelf || isRowPending(row)"
            :loading="isRowPending(row)"
            @click="terminate(row, 'stop')"
          >停止</el-button>
          <el-button
            size="small"
            link
            type="danger"
            :disabled="row.isSelf || isRowPending(row)"
            :loading="isRowPending(row)"
            @click="terminate(row, 'kill')"
          >强杀</el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { ListProcesses, StopProcess, KillProcess } from '../../wailsjs/go/main/App'
import { handleError, ErrorCode } from '../utils/error'

// 检索模式：auto / port / name，取值对齐 service/process.go 的 ProcessFilterMode* 常量
const MODE_AUTO = 'auto'
const MODE_PORT = 'port'
const MODE_NAME = 'name'

// 停止生效手段：取值对齐 model/process.go 的 ProcessStopMethod* 常量。
// Method 由后端按轮询实测结果如实填充——优雅请求能否生效取决于目标进程是否响应、无法事前
// 预判，故后端统一「发请求 → 5s 轮询实测存活 → 未退转强杀」，graceful 表示实测确已退出，
// forced 表示请求被拒或超时后强杀。前端只按值选提示文案，不自行推断。
const STOP_METHOD_GRACEFUL = 'graceful'
const STOP_METHOD_FORCED = 'forced'

const keyword = ref('')
const mode = ref(MODE_AUTO)
// 进程树为显式二级选项，默认关闭（与 Windows 任务管理器一致：默认只结束单进程）
const killTree = ref(false)
const loading = ref(false)
const processes = ref([])

// 行级等待态：终止操作最坏约 11s（3s 优雅命令超时 + 5s 轮询 + 3s 强杀），期间须
// 禁用该行两个按钮防重复点击，并给出可见 loading 反馈。用数组而非 Set，增删时
// 整体重新赋值以稳定触发响应更新。
const pendingPids = ref([])

// 该行是否有终止操作在途（模板据此 disabled + loading）
const isRowPending = (row) => pendingPids.value.includes(row.pid)

// 检索序号：快速重复检索时旧响应后到会覆盖新列表，过期响应整体丢弃
let loadSeq = 0

const loadList = async () => {
  const seq = ++loadSeq
  loading.value = true
  try {
    const list = await ListProcesses(keyword.value.trim(), mode.value)
    if (seq !== loadSeq) return
    processes.value = list || []
  } catch (e) {
    if (seq !== loadSeq) return
    // 按 code 分流：枚举超时（E_PROCESS_ENUM_TIMEOUT）为 warning 引导重试，真失败弹 error
    handleError('进程检索失败: ', e)
  } finally {
    if (seq === loadSeq) {
      loading.value = false
    }
  }
}

// 面板挂载即枚举（工具切换 = 卸载重挂，等价原弹窗 destroy-on-close + @open 语义；
// 进程状态实时变化，不做缓存）
onMounted(() => {
  loadList()
})

/**
 * 三级确认。判据全部取后端下发的 isSelf / isSystem 标志，前端不自判
 * （自我保护只能用 PID，禁止按进程名：E2E 会把 exe 改名 workbench-e2e.exe）。
 *
 * @param {object} row 进程行
 * @param {'stop'|'kill'} action 操作类型（强杀文案更强硬）
 * @returns {Promise<boolean>} 用户是否确认
 */
const requestConfirm = async (row, action) => {
  const isForce = action === 'kill'
  if (row.isSystem) {
    // L2 强确认：系统关键进程用红色错误级确认框，明示蓝屏 / 系统重启后果
    try {
      await ElMessageBox.confirm(
        `「${row.name}」（PID ${row.pid}）是系统关键进程，结束它可能导致系统蓝屏或立即重启，所有未保存的数据将丢失。确定继续？`,
        '危险操作',
        {
          type: 'error',
          confirmButtonText: '仍然结束',
          cancelButtonText: '取消',
          confirmButtonClass: 'el-button--danger'
        }
      )
    } catch {
      return false // 用户放弃
    }
    return true
  }
  // L3 通用确认：强杀额外强调不等待进程保存数据
  const message = isForce
    ? `确定强制结束进程「${row.name}」（PID ${row.pid}）？强制结束不会等待进程保存数据，未保存的数据将立即丢失。`
    : `确定结束进程「${row.name}」（PID ${row.pid}）？未保存的数据将丢失。`
  try {
    await ElMessageBox.confirm(message, isForce ? '强制结束进程' : '结束进程', {
      type: 'warning',
      confirmButtonText: '确定结束',
      cancelButtonText: '取消',
      confirmButtonClass: 'el-button--danger'
    })
  } catch {
    return false
  }
  return true
}

/**
 * 结束进程（停止 / 强杀）。
 *
 * 「停止」为全自动两级（发优雅停止请求 → 5s 内未退出自动转强杀）。优雅请求能否生效取决于
 * 目标进程是否响应，不做进程类型预判；提示文案按后端轮询实测返回的 Method 如实反映最终
 * 生效手段（graceful = 确已退出，forced = 请求被拒或超时后强杀），不谎称已优雅退出。
 */
const terminate = async (row, action) => {
  // L1 硬禁：本应用自身与父进程链（后端终止入口还会再判一次，前端可绕过）
  if (row.isSelf) return
  // 防重：该行已有在途终止操作
  if (isRowPending(row)) return
  // 确认框先于等待态——用户取消时不进入 loading，按钮立即恢复可用
  if (!(await requestConfirm(row, action))) return
  pendingPids.value = [...pendingPids.value, row.pid]
  try {
    if (action === 'kill') {
      await KillProcess(row.pid, killTree.value)
      ElMessage.success('进程已强制结束')
    } else {
      const result = await StopProcess(row.pid, killTree.value)
      if (result?.method === STOP_METHOD_GRACEFUL) {
        ElMessage.success('进程已正常退出')
      } else {
        // 轮询超时未退出转强杀，或优雅请求被目标拒绝后直落强杀
        ElMessage.success('进程已强制结束')
      }
    }
    loadList()
  } catch (e) {
    handleError('结束进程失败: ', e)
    // 进程已自行退出（列表过期）：提示后自动刷新，避免陈旧行留在列表里被反复点击
    if (e?.code === ErrorCode.ProcessNotFound) {
      loadList()
    }
  } finally {
    // 无论成功失败都解除该行等待态，避免按钮永久禁用
    pendingPids.value = pendingPids.value.filter((pid) => pid !== row.pid)
  }
}

// 一个进程可占用多个端口：同列按去重后的值逗号连接展示，无端口时占位
const joinUnique = (values) => {
  const list = (values || []).filter((v) => v !== undefined && v !== null && v !== '')
  return [...new Set(list)].join(', ')
}

const portText = (row) => joinUnique((row.ports || []).map((p) => p.port)) || '—'
const protocolText = (row) => joinUnique((row.ports || []).map((p) => p.protocol)) || '—'
const stateText = (row) => joinUnique((row.ports || []).map((p) => p.state)) || '—'

// 后端下发单位为 KB，按量级升档展示；不可读时为 0 展示占位
const memoryText = (row) => {
  const kb = row.memoryKB || 0
  if (!kb) return '—'
  if (kb < 1024) return `${kb} KB`
  if (kb < 1024 * 1024) return `${(kb / 1024).toFixed(1)} MB`
  return `${(kb / 1024 / 1024).toFixed(2)} GB`
}
</script>

<style scoped>
.pm-refresh-icon {
  margin-right: 4px;
}

.pm-search {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-md);
}

.pm-search-input {
  flex: 1;
  min-width: 220px;
}

.pm-mode-select {
  width: 110px;
}

.pm-tree-checkbox {
  margin-right: 0;
}

.pm-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-tertiary);
}

.pm-name {
  font-weight: 500;
  color: var(--text-primary);
}

.pm-name-tag {
  margin-left: var(--spacing-xs);
}

/* 列表区限高滚动：进程条目可达数百行，el-table max-height 内部滚动（表头吸顶），
   面板整体滚动由 ToolboxPanel .toolbox-main 容器兜底 */
</style>
