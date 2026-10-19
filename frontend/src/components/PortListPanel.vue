<template>
  <div class="port-list-panel">
    <div class="tool-panel-titlebar">
      <span class="tool-panel-title">端口占用</span>
      <el-button
        size="small"
        :loading="loading"
        @click="loadList"
      >
        <el-icon class="pl-refresh-icon"><Refresh /></el-icon>
        刷新
      </el-button>
    </div>

    <!-- 视图说明：与进程管理共用同一枚举底座，这里只展开监听态端口，一行一个端口 -->
    <div class="pl-meta">
      <span>共 {{ rows.length }} 个监听端口</span>
      <span class="pl-meta-hint">按端口号排序，仅展示监听（LISTENING）状态</span>
    </div>

    <el-table
      :data="rows"
      size="small"
      border
      max-height="440"
      :empty-text="loading ? '加载中…' : '当前没有监听端口'"
      class="pl-table"
    >
      <el-table-column prop="port" label="端口" width="90" sortable />
      <el-table-column prop="protocol" label="协议" width="72" />
      <el-table-column prop="localAddr" label="本地地址" min-width="150" show-overflow-tooltip />
      <el-table-column prop="state" label="状态" width="110">
        <template #default="{ row }">{{ row.state || '—' }}</template>
      </el-table-column>
      <el-table-column label="进程名" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="pl-name">{{ row.name }}</span>
          <el-tag v-if="row.isSelf" size="small" type="info" class="pl-name-tag">本应用</el-tag>
          <el-tag v-else-if="row.isSystem" size="small" type="danger" class="pl-name-tag">系统进程</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="pid" label="PID" width="84" />
      <el-table-column label="操作" fixed="right" width="90">
        <template #default="{ row }">
          <el-button
            size="small"
            link
            type="danger"
            :disabled="row.isSelf || isRowPending(row)"
            :loading="isRowPending(row)"
            @click="terminate(row)"
          >终止</el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { ListProcesses, StopProcess } from '../../wailsjs/go/main/App'
import { handleError, ErrorCode } from '../utils/error'

// 停止生效手段：取值对齐 model/process.go 的 ProcessStopMethod* 常量（与进程管理同语义：
// 后端统一「发请求 → 5s 轮询实测存活 → 未退转强杀」，graceful 表示实测确已退出）
const STOP_METHOD_GRACEFUL = 'graceful'

const loading = ref(false)
const rows = ref([])

// 行级等待态：终止操作最坏约 11s，期间禁用该行按钮防重复点击。
// 用数组而非 Set，增删时整体重新赋值以稳定触发响应更新。
const pendingPids = ref([])

const isRowPending = (row) => pendingPids.value.includes(row.pid)

// 检索序号：快速重复刷新时旧响应后到会覆盖新列表，过期响应整体丢弃
let loadSeq = 0

/**
 * 枚举监听态进程并展开为一行一个端口。
 *
 * 复用进程管理枚举底座：ListProcesses 空关键字默认视图即「仅监听态」（保留判定见
 * service/process.go filterListeningProcesses——本地化状态名保守保留），这里只做
 * 前端扁平化投影 + 按端口号排序，后端零新增方法、wailsjs 绑定零变更。
 */
const loadList = async () => {
  const seq = ++loadSeq
  loading.value = true
  try {
    const processes = await ListProcesses('', '')
    if (seq !== loadSeq) return
    rows.value = flattenPortRows(processes || [])
  } catch (e) {
    if (seq !== loadSeq) return
    // 按 code 分流：枚举超时为 warning 引导重试，真失败弹 error
    handleError('端口枚举失败: ', e)
  } finally {
    if (seq === loadSeq) {
      loading.value = false
    }
  }
}

// 面板挂载即枚举（工具切换 = 卸载重挂，等价原弹窗 destroy-on-close + @open 语义；
// 端口占用实时变化，不做缓存）
onMounted(() => {
  loadList()
})

// 扁平化：进程 × 端口 → 行。端口行保留进程级自我保护标志（前端禁用依据，后端终止入口仍会再拒一次）
const flattenPortRows = (processes) => {
  const list = []
  for (const proc of processes) {
    const ports = proc.ports || []
    for (const p of ports) {
      list.push({
        port: p.port,
        protocol: p.protocol,
        state: p.state,
        localAddr: p.localAddr,
        pid: proc.pid,
        name: proc.name,
        isSelf: proc.isSelf,
        isSystem: proc.isSystem,
        // 同进程端口数：确认文案提示「终止进程会连带其他端口」用
        portCount: ports.length
      })
    }
  }
  return list.sort((a, b) => a.port - b.port)
}

/**
 * 终止确认。判据全部取后端下发的 isSelf / isSystem 标志，前端不自判
 * （自我保护只能用 PID，禁止按进程名，与进程管理一致）。
 *
 * @param {object} row 端口行
 * @returns {Promise<boolean>} 用户是否确认
 */
const requestConfirm = async (row) => {
  const extra = row.portCount > 1
    ? `该进程共占用 ${row.portCount} 个端口，终止将一并释放其余端口。`
    : ''
  if (row.isSystem) {
    // 强确认：系统关键进程用红色错误级确认框，明示后果
    try {
      await ElMessageBox.confirm(
        `「${row.name}」（PID ${row.pid}）是系统关键进程，结束它可能导致系统蓝屏或立即重启，所有未保存的数据将丢失。${extra}确定继续？`,
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
  try {
    await ElMessageBox.confirm(
      `确定结束进程「${row.name}」（PID ${row.pid}）？未保存的数据将丢失。${extra}`,
      '结束进程',
      {
        type: 'warning',
        confirmButtonText: '确定结束',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger'
      }
    )
  } catch {
    return false
  }
  return true
}

/**
 * 终止端口占用进程（全自动两级：发优雅停止请求 → 5s 内未退出自动转强杀）。
 * 提示文案按后端轮询实测返回的 Method 如实反映生效手段，不谎称已优雅退出。
 */
const terminate = async (row) => {
  // 硬禁：本应用自身与父进程链（后端终止入口还会再判一次，前端可绕过）
  if (row.isSelf) return
  // 防重：该行已有在途终止操作
  if (isRowPending(row)) return
  // 确认框先于等待态——用户取消时不进入 loading
  if (!(await requestConfirm(row))) return
  pendingPids.value = [...pendingPids.value, row.pid]
  try {
    const result = await StopProcess(row.pid, false)
    if (result?.method === STOP_METHOD_GRACEFUL) {
      ElMessage.success('进程已正常退出')
    } else {
      ElMessage.success('进程已强制结束')
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
</script>

<style scoped>
.pl-refresh-icon {
  margin-right: 4px;
}

.pl-meta {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
  font-size: 12px;
  color: var(--text-tertiary);
}

.pl-name {
  font-weight: 500;
  color: var(--text-primary);
}

.pl-name-tag {
  margin-left: var(--spacing-xs);
}

/* 列表区限高滚动：监听端口可达数百行，el-table max-height 内部滚动（表头吸顶），
   面板整体滚动由 ToolboxPanel .toolbox-main 容器兜底 */
</style>
