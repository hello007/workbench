<template>
  <div class="ai-function-panel">
    <!-- 顶部标题栏：风格对齐 ToolboxPanel 的 .toolbox-header（渐变背景 + 16px/700 标题） -->
    <div class="panel-header">
      <div class="panel-heading">
        <span class="panel-title">
          <el-icon :size="18" class="panel-title-icon"><MagicStick /></el-icon>
          AI 功能
        </span>
        <span class="panel-subtitle"><span class="subtitle-sign">&gt;</span>Claude Skills 聚合触发</span>
      </div>
      <span class="panel-actions">
        <span v-if="concurrency.running > 0 || concurrency.queued > 0" class="concurrency-badge">
          并发 {{ concurrency.running }}/{{ concurrency.max }}
          <span v-if="concurrency.queued > 0" class="concurrency-queued">（排队 {{ concurrency.queued }}）</span>
        </span>
        <el-button size="small" @click="historyVisible = true">历史</el-button>
        <el-button size="small" @click="configVisible = true">配置管理</el-button>
        <el-button size="small" @click="loadFunctions">刷新</el-button>
      </span>
    </div>
    <div class="ai-layout">
      <!-- 左：功能卡片列表（管理按钮已上移标题栏，侧栏纯列表） -->
      <div class="ai-sidebar">
        <div class="ai-cards">
          <div
            v-for="f in functions"
            :key="f.id"
            class="ai-card"
            @click="openFunctionTab(f)"
          >
            <div class="ai-card-head">
              <el-icon :size="18" class="ai-card-icon"><component :is="iconComp(f.icon)" /></el-icon>
              <span class="ai-card-name">{{ f.name }}</span>
            </div>
            <div class="ai-card-desc">{{ f.description }}</div>
          </div>
        </div>
      </div>

      <!-- 右：功能 Tab + 任务输出 Tab -->
      <div class="ai-main">
        <el-empty
          v-if="tasks.length === 0 && openFuncs.length === 0"
          class="ai-empty"
          description="从左侧选择功能开始 · 输出与任务将在此展示"
        />
        <template v-else>
          <el-tabs v-model="activeTabId" type="card" closable @tab-remove="closeTab">
            <!-- 功能 Tab：功能详情 + 内嵌参数录入 + 运行按钮，常驻可反复运行 -->
            <el-tab-pane
              v-for="f in openFuncs"
              :key="f.id"
              :name="funcTabName(f.id)"
              :label="f.name"
            >
              <AiFunctionRunner :fn="f" @run="(params) => onRunnerRun(f, params)" />
            </el-tab-pane>
            <!-- 任务 Tab：运行输出 -->
            <el-tab-pane
              v-for="t in tasks"
              :key="t.taskId"
              :name="t.taskId"
              :label="tabLabel(t)"
            >
              <div class="task-toolbar">
                <!-- 运行中态附加条件 class 驱动呼吸脉搏动画（仅装饰，不参与逻辑） -->
                <el-tag size="small" :type="statusTagType(t)" :class="{ 'status-running': t.running }">{{ statusText(t) }}</el-tag>
                <span class="task-prompt" :title="t.prompt">{{ t.prompt }}</span>
                <div class="task-actions">
                  <el-button
                    v-if="t.running || t.queued"
                    size="small"
                    type="danger"
                    plain
                    @click="cancelTask(t)"
                  >
                    取消
                  </el-button>
                  <el-button
                    v-if="!t.running && t.completion === 'copy'"
                    size="small"
                    @click="copyOutput(t)"
                  >
                    复制输出
                  </el-button>
                  <el-button
                    v-if="!t.running && t.completion === 'open_dir'"
                    size="small"
                    @click="openDir(t)"
                  >
                    打开目录
                  </el-button>
                  <el-button
                    v-if="!t.running && t.completion === 'preview'"
                    size="small"
                    @click="previewOutput(t)"
                  >
                    预览产物
                  </el-button>
                </div>
              </div>
              <!-- 底栏：完成态展示计量摘要（耗时/token/成本），失败态展示 exitCode 与错误信息 -->
              <div v-if="!t.running && !t.queued && (metricsText(t) || t.error)" class="task-footer">
                <span v-if="metricsText(t)" class="metrics-text">{{ metricsText(t) }}</span>
                <span v-if="t.error" class="error-text">✗ {{ t.canceled ? '已取消' : '失败' }} · {{ t.error }}</span>
              </div>
              <!-- 后续段按钮（多段编排）；会议表格视图时由行内取消按钮替代 -->
              <div
                v-if="!t.running && t.followUps?.length && !t.error && !meetingTable(t)"
                class="followup-bar"
              >
                <el-button
                  v-for="fu in t.followUps"
                  :key="fu.id"
                  size="small"
                  type="primary"
                  plain
                  @click="doRunFollowUp(t, fu, {})"
                >
                  {{ fu.label }}
                </el-button>
              </div>
              <!-- 会议列表表格视图：输出含合法 markdown 表格 + 表头含「会议号」+ 配有 cancel-meeting 后续段。
                   时长/入会链接/入会密码列不渲染（过长冗余），数据仍在行内，由「详情」按钮复制补全 -->
              <div v-if="meetingTable(t)" class="task-table">
                <el-table :data="meetingTable(t).rows" size="small" border>
                  <el-table-column
                    v-for="h in meetingTable(t).visibleHeaders"
                    :key="h"
                    :prop="h"
                    :label="h"
                    min-width="120"
                  />
                  <el-table-column label="操作" fixed="right" width="130">
                    <template #default="{ row }">
                      <el-button size="small" @click="onDetailMeetingRow(row)">
                        详情
                      </el-button>
                      <el-button
                        size="small"
                        type="danger"
                        plain
                        :loading="cancelLoading"
                        @click="onCancelMeetingRow(t, row)"
                      >
                        取消
                      </el-button>
                    </template>
                  </el-table-column>
                </el-table>
              </div>
              <div v-else class="task-output">
                <!-- 终端窗口装饰行：macOS 三圆点 + claude -p 提示（纯装饰，sticky 常驻输出区顶部） -->
                <div class="term-bar" aria-hidden="true">
                  <span class="term-dot term-dot-r"></span>
                  <span class="term-dot term-dot-y"></span>
                  <span class="term-dot term-dot-g"></span>
                  <span class="term-cmd">claude -p</span>
                </div>
                <pre>{{ t.output || '（等待输出…）' }}</pre>
              </div>
            </el-tab-pane>
          </el-tabs>
        </template>
      </div>
    </div>

    <!-- 配置管理 -->
    <AiFunctionConfigDialog
      :visible="configVisible"
      @update:visible="configVisible = $event"
      @saved="loadFunctions"
    />

    <!-- 运行历史（P1-2：任务完成归档后查看历史记录，复用 metrics 展示样式） -->
    <AiTaskHistoryPanel v-model:visible="historyVisible" />
  </div>
</template>

<script setup>
import { ref, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import * as Icons from '@element-plus/icons-vue'
import { MagicStick } from '@element-plus/icons-vue'
import {
  GetAiFunctions,
  RunAiFunction,
  RunAiFollowUp,
  CancelAiTask,
  GetAiConcurrencyStatus,
  GetAiTaskOutput,
  RemoveAiTask,
  OpenInExplorer,
  OpenWithDefaultApp
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import AiFunctionRunner from './AiFunctionRunner.vue'
import AiFunctionConfigDialog from './AiFunctionConfigDialog.vue'
import AiTaskHistoryPanel from './AiTaskHistoryPanel.vue'

// 组件经 v-show 常驻挂载（Home 主区互斥展示），无 visible prop；
// 功能列表挂载时加载一次，配置管理 saved 回调与标题栏「刷新」按钮负责后续重载
const functions = ref([])
const tasks = ref([]) // { taskId, functionId, name, icon, prompt, output, running, error, canceled, followUps, completion, cwd }
// 已打开的功能 Tab（存 AiFunction 快照），name 用 func:<id> 前缀与任务 Tab 区分
const openFuncs = ref([])
const activeTabId = ref('')
const configVisible = ref(false)
const historyVisible = ref(false)

const iconComp = (name) => (name && Icons[name]) || Icons.MagicStick

const loadFunctions = async () => {
  try {
    functions.value = (await GetAiFunctions()) || []
  } catch (e) {
    ElMessage.error('加载 AI 功能失败: ' + (e?.message || String(e)))
  }
}

// ===== 功能 Tab =====
const funcTabName = (id) => `func:${id}`

// 点击卡片打开功能 Tab（详情 + 参数录入 + 运行按钮）；
// 同功能已打开则仅切换并同步最新配置快照（Runner 内已录参数不丢），不重复开 Tab
const openFunctionTab = (f) => {
  const idx = openFuncs.value.findIndex((x) => x.id === f.id)
  if (idx >= 0) {
    openFuncs.value[idx] = f
  } else {
    openFuncs.value.push(f)
  }
  activeTabId.value = funcTabName(f.id)
}

// 功能 Tab 内点运行：主段复用同功能空闲任务 Tab（见 doRunMain），功能 Tab 保留可反复运行
const onRunnerRun = (f, params) => {
  doRunMain(f, params)
}

// ===== 运行 =====
// 主段任务 Tab 复用策略：从后往前找 functionId 匹配且非运行中的最新一条，
// 命中则原位替换（关旧开新到相同位置——v-for :key 为 taskId，旧 pane 卸载、
// 新 pane 同位挂载，taskId 即 el-tabs 的 pane name 随之切换，activeTabId
// 在同一同步块内指向新值，渲染时无中间态残留）；未命中或全部运行中才新增 Tab。
// doRunFollowUp 不参与复用：后续段始终新开 Tab，保留草稿/浏览等深度历史。
const doRunMain = async (f, params) => {
  try {
    const taskId = await RunAiFunction(f.id, params)
    const task = {
      taskId,
      functionId: f.id,
      name: f.name,
      prompt: buildPromptPreview(f, params),
      output: '',
      outputSize: 0,          // 3.3：完整输出字节数（后端 done 写入），列表/详情展示用
      outputFile: '',         // 3.3：输出文件相对路径（后端 done 写入）
      tableExtracted: null,   // 3.3：后端预解析表格（done 写入），表格视图优先用此值
      truncated: false,       // 输出超 256KB 截断标记（仅展示层）
      running: false,         // 后端先排队后执行，初始 queued=true
      queued: true,
      error: '',
      canceled: false,
      sessionId: '',
      metrics: null,          // P0-2 计量，onDone 写入
      followUps: f.followUps || [],
      completion: f.completion || 'none',
      cwd: f.cwd
    }
    let reuseIdx = -1
    for (let i = tasks.value.length - 1; i >= 0; i--) {
      const t = tasks.value[i]
      if (t.functionId === f.id && !t.running && !t.queued) {
        reuseIdx = i
        break
      }
    }
    if (reuseIdx >= 0) {
      tasks.value.splice(reuseIdx, 1, task)
    } else {
      tasks.value.push(task)
    }
    activeTabId.value = taskId
    refreshConcurrency()
  } catch (e) {
    ElMessage.error('启动失败: ' + (e?.message || String(e)))
  }
}

const buildPromptPreview = (f, params) => {
  const vals = Object.values(params || {}).filter(Boolean).join(' ')
  return vals ? `${f.command} ${vals}` : f.command
}

// ===== 会议列表表格视图 =====

// 表格视图中不渲染的列：信息冗余/URL 过长，完整信息由行内「详情」按钮复制补全
const HIDDEN_COLS = ['时长', '入会链接', '入会密码']

// 解析文本中的 markdown 表格：取最后一个连续 |...| 行构成的块，
// 跳过 --- 分隔行后首行为表头、其余为数据行；数据行按表头名映射为
// { 表头: 值 } 对象（列序无关，隐藏列与详情复制均按表头名取值）。
// 无合法表格返回 null。
const parseMarkdownTable = (text) => {
  const lines = String(text || '').split(/\r?\n/)
  const isTableRow = (l) => {
    const s = l.trim()
    return s.length > 1 && s.startsWith('|') && s.endsWith('|')
  }
  const splitCells = (l) =>
    l
      .trim()
      .slice(1, -1)
      .split('|')
      .map((c) => c.trim())
  const isSeparatorRow = (cells) =>
    cells.length > 0 && cells.every((c) => /^:?:?-{2,}:?$/.test(c))

  const blocks = []
  let cur = []
  for (const line of lines) {
    if (isTableRow(line)) {
      cur.push(line)
    } else if (cur.length) {
      blocks.push(cur)
      cur = []
    }
  }
  if (cur.length) blocks.push(cur)

  const block = blocks[blocks.length - 1]
  if (!block || block.length < 2) return null
  const parsedRows = block.map(splitCells).filter((cells) => !isSeparatorRow(cells))
  if (parsedRows.length === 0) return null
  const headers = parsedRows[0]
  const rows = parsedRows.slice(1).map((cells) => {
    const obj = {}
    headers.forEach((h, i) => {
      obj[h] = cells[i] ?? ''
    })
    return obj
  })
  return { headers, rows }
}

// 任务是否走会议表格视图：followUps 含 cancel-meeting、输出含合法表格
// 且表头含「会议号」三个条件同时满足；返回表格（含可见列）或 null。
// 3.3：优先用后端预解析的 tableExtracted（避免末尾窗口截断后表格丢失）；
// 预解析缺失时退化前端从展示文本解析（兜底，可能因截断丢表格）。
const meetingTable = (t) => {
  if (!(t.followUps || []).some((fu) => fu.id === 'cancel-meeting')) return null
  let tbl = t.tableExtracted
  if (!tbl) {
    tbl = parseMarkdownTable(t.output)
  }
  if (!tbl || !tbl.headers.includes('会议号')) return null
  tbl.visibleHeaders = tbl.headers.filter((h) => !HIDDEN_COLS.includes(h))
  return tbl
}

// rowValue 按表头名取行字段值；缺失或空白时返回「无」（详情复制的兜底文案）
const rowValue = (row, header) => {
  const v = row ? row[header] : ''
  return v === undefined || v === null || String(v).trim() === '' ? '无' : String(v)
}

// 行内「详情」按钮：组装该行完整会议信息复制到剪贴板（含表格中隐藏的列）
const onDetailMeetingRow = async (row) => {
  const text =
    `会议主题：${rowValue(row, '会议主题')}\n` +
    `会议时间：${rowValue(row, '开始时间')}-${rowValue(row, '结束时间')}\n` +
    `时长：${rowValue(row, '时长')}\n` +
    `入会链接：${rowValue(row, '入会链接')}\n` +
    `#腾讯会议：${rowValue(row, '会议号')}\n` +
    `入会密码：${rowValue(row, '入会密码')}`
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('会议信息已复制')
  } catch {
    ElMessage.error('复制失败')
  }
}

// 行内取消会议：确认弹窗（展示该行会议信息）→ 确认后走 cancel-meeting 后续段
const cancelLoading = ref(false)
const onCancelMeetingRow = async (t, row) => {
  if (cancelLoading.value) return
  const fu = (t.followUps || []).find((x) => x.id === 'cancel-meeting')
  if (!fu) return
  cancelLoading.value = true
  try {
    try {
      await ElMessageBox.confirm(
        `确认取消会议：${row['会议主题'] || ''}（会议号 ${row['会议号'] || ''}），` +
          `${row['开始时间'] || ''} ~ ${row['结束时间'] || ''}。取消后不可恢复，确认取消该会议？`,
        '取消会议',
        { type: 'warning', confirmButtonText: '确认取消', cancelButtonText: '再想想' }
      )
    } catch {
      return // 用户放弃确认
    }
    await doRunFollowUp(t, fu, {
      meeting: `${row['会议主题'] || ''}（会议号 ${row['会议号'] || ''}）`
    })
  } finally {
    cancelLoading.value = false
  }
}

const doRunFollowUp = async (task, followUp, params) => {
  try {
    const taskId = await RunAiFollowUp(task.taskId, followUp.id, params)
    // 后续段起新 Tab，保留原任务上下文
    const fn = functions.value.find((f) => f.id === task.functionId)
    tasks.value.push({
      taskId,
      functionId: task.functionId,
      name: `${task.name} · ${followUp.label}`,
      prompt: followUp.promptTemplate,
      output: '',
      outputSize: 0,
      outputFile: '',
      tableExtracted: null,
      truncated: false,
      running: false,
      queued: true,
      error: '',
      canceled: false,
      metrics: null,
      followUps: fn?.followUps || [],
      completion: 'none',
      cwd: task.cwd
    })
    activeTabId.value = taskId
    refreshConcurrency()
  } catch (e) {
    ElMessage.error('启动后续段失败: ' + (e?.message || String(e)))
  }
}

// ===== 事件流 =====
// P0-4(3.1) 末尾窗口截断：输出超 MAX_DISPLAY 只保留末尾窗口，顶部提示省略量。
// 截断仅影响展示（t.output）；3.3 后 copy/preview/表格视图改调 GetAiTaskOutput 全量读取，不依赖展示文本。
const MAX_DISPLAY = 256 * 1024

const onOutput = (ev) => {
  const t = tasks.value.find((x) => x.taskId === ev.taskId)
  if (!t) return
  t.output += ev.text || ''
  if (t.output.length > MAX_DISPLAY) {
    const omittedKB = Math.floor((t.output.length - MAX_DISPLAY) / 1024)
    t.output =
      '…（已省略前 ' + omittedKB + ' KB，完整内容可复制或查看历史）\n' +
      t.output.slice(-MAX_DISPLAY)
    t.truncated = true
  }
  scrollOutput()
}

// onQueued 后端注册排队态（RunStage 先入 map 再 select 等槽位）
const onQueued = (ev) => {
  const t = tasks.value.find((x) => x.taskId === ev.taskId)
  if (!t) return
  t.queued = true
  t.running = false
  refreshConcurrency()
}

// onStarted 后端获取槽位转执行态（超时从此起算）
const onStarted = (ev) => {
  const t = tasks.value.find((x) => x.taskId === ev.taskId)
  if (!t) return
  t.queued = false
  t.running = true
  refreshConcurrency()
}

const onDone = (result) => {
  const t = tasks.value.find((x) => x.taskId === result.taskId)
  if (!t) return
  t.running = false
  t.queued = false
  t.error = result.error || ''
  t.canceled = !!result.canceled
  t.metrics = result.metrics || null
  // 3.3：done 事件 payload 仅含末尾预览，全量输出在文件；记录大小/路径/预解析表格供完成动作与表格视图
  t.outputSize = result.outputSize || 0
  t.outputFile = result.outputFile || ''
  t.tableExtracted = result.tableExtracted || null
  if (result.sessionId) t.sessionId = result.sessionId
  refreshConcurrency()
  if (!t.error && !t.canceled) {
    handleCompletion(t, result)
  }
}

// 并发占用展示（标题栏「N/M」），任务起止时刷新
const concurrency = ref({ running: 0, queued: 0, max: 3 })
const refreshConcurrency = async () => {
  try {
    concurrency.value = await GetAiConcurrencyStatus()
  } catch {
    // 查询失败不影响主流程
  }
}

const scrollOutput = () => {
  nextTick(() => {
    const el = document.querySelector('.ai-function-panel .task-output')
    if (el) el.scrollTop = el.scrollHeight
  })
}

// ===== 完成动作 =====
const handleCompletion = async (t, result) => {
  try {
    if (t.completion === 'copy') {
      // 3.3：全量输出经 GetAiTaskOutput 读文件，不依赖已截断的展示文本
      const full = await GetAiTaskOutput(t.taskId)
      if (full) {
        await navigator.clipboard.writeText(full)
        ElMessage.success('输出已复制到剪贴板')
      }
    } else if (t.completion === 'open_dir' && t.cwd) {
      await OpenInExplorer(t.cwd)
    } else if (t.completion === 'preview') {
      previewOutput(t)
    }
  } catch (e) {
    // 完成动作失败不阻断结果展示
    console.warn('完成动作失败:', e)
  }
}

const copyOutput = async (t) => {
  try {
    // 3.3：全量输出经 GetAiTaskOutput 读文件，不受 3.1 末尾窗口截断影响
    const full = await GetAiTaskOutput(t.taskId)
    await navigator.clipboard.writeText(full || '')
    ElMessage.success('已复制')
  } catch (e) {
    ElMessage.error('复制失败: ' + (e?.message || String(e)))
  }
}

const openDir = (t) => OpenInExplorer(t.cwd)

const previewOutput = async (t) => {
  // 从输出中提取第一个 .html 文件路径（发言稿产物），系统默认程序打开即预览。
  // 3.3：全量输出经 GetAiTaskOutput 读文件，避免截断后路径丢失
  try {
    const full = await GetAiTaskOutput(t.taskId)
    const m = (full || '').match(/[A-Za-z]:\\[^\s"'<>|]+\.html/i)
    if (m) {
      await OpenWithDefaultApp(m[0])
    } else {
      ElMessage.info('输出中未找到 .html 产物路径，请手动打开目录')
      openDir(t)
    }
  } catch (e) {
    ElMessage.error('读取输出失败: ' + (e?.message || String(e)))
    openDir(t)
  }
}

// ===== 任务管理 =====
const cancelTask = async (t) => {
  await CancelAiTask(t.taskId)
  // 排队取消立即生效，运行中取消等 done 事件；两种都刷新并发展示
  t.queued = false
  refreshConcurrency()
}

// 关闭 Tab：功能 Tab（func: 前缀）直接关闭；任务 Tab 运行中/排队中拦截
const closeTab = (name) => {
  if (name.startsWith('func:')) {
    const id = name.slice('func:'.length)
    openFuncs.value = openFuncs.value.filter((x) => x.id !== id)
    if (activeTabId.value === name) switchToFirstTab()
    return
  }
  closeTask(name)
}

const closeTask = (taskId) => {
  const t = tasks.value.find((x) => x.taskId === taskId)
  if (t?.running || t?.queued) {
    ElMessage.warning('任务运行中或排队中，请先取消再关闭')
    return
  }
  tasks.value = tasks.value.filter((x) => x.taskId !== taskId)
  // 清理后端 runtime（已完成/已取消才允许走到此处）
  RemoveAiTask(taskId).catch(() => {
    // 清理失败不阻断前端 Tab 关闭
  })
  if (activeTabId.value === taskId) {
    switchToFirstTab()
  }
}

// 关闭当前 Tab 后切换到剩余首个 Tab（任务 Tab 优先，其次功能 Tab）
const switchToFirstTab = () => {
  activeTabId.value =
    tasks.value[0]?.taskId ||
    (openFuncs.value[0] ? funcTabName(openFuncs.value[0].id) : '')
}

const tabLabel = (t) =>
  t.queued ? `${t.name} ⌛` : t.running ? `${t.name} ⏳` : t.error ? `${t.name} ✕` : t.name
const statusText = (t) =>
  t.queued ? '排队中' : t.running ? '运行中' : t.canceled ? '已取消' : t.error ? '失败' : '已完成'
const statusTagType = (t) =>
  t.queued ? 'info' : t.running ? 'primary' : t.canceled ? 'info' : t.error ? 'danger' : 'success'

// 计量摘要（P0-2）：耗时 / token 入出 / 缓存读（非零）/ 成本 / 轮次（>1），无 metrics 返回空
const metricsText = (t) => {
  const m = t.metrics
  if (!m) return ''
  const parts = []
  if (m.durationMs > 0) parts.push(fmtDuration(m.durationMs))
  const u = m.usage
  if (u) {
    parts.push(`入 ${fmtK(u.inputTokens)} / 出 ${fmtK(u.outputTokens)}`)
    if (u.cacheReadInputTokens > 0) parts.push(`缓存读 ${fmtK(u.cacheReadInputTokens)}`)
  }
  if (m.costUsd > 0) parts.push(`$${m.costUsd.toFixed(3)}`)
  if (m.numTurns > 1) parts.push(`${m.numTurns} 轮`)
  return parts.join(' · ')
}
const fmtDuration = (ms) => {
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}
const fmtK = (n) => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n))

// ===== 生命周期 =====
onMounted(() => {
  loadFunctions()
  refreshConcurrency()
  EventsOn('ai-task:queued', onQueued)
  EventsOn('ai-task:started', onStarted)
  EventsOn('ai-task:output', onOutput)
  EventsOn('ai-task:done', onDone)
})
onBeforeUnmount(() => {
  EventsOff('ai-task:queued')
  EventsOff('ai-task:started')
  EventsOff('ai-task:output')
  EventsOff('ai-task:done')
})
</script>

<style scoped>
/* 根元素：占满 Home 主区上半区（与 .main-panes 互斥），高度约束链从此起 */
.ai-function-panel {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
  overflow: hidden;
}
/* 顶部标题栏：风格对齐 ToolboxPanel 的 .toolbox-header */
.panel-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}
.panel-heading {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}
.panel-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 0.5px;
  white-space: nowrap;
}
.panel-subtitle {
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
/* 终端提示符前缀：等宽 + 主色，呼应「指令触发」语义 */
.subtitle-sign {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-weight: 600;
  color: var(--primary-color);
  margin-right: 4px;
}
.panel-title-icon {
  color: var(--primary-color, #409eff);
}
/* 按钮组与标题间加分隔感：左缘细线 + 间距（hover 反馈由 el-button 自带，保持克制） */
.panel-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-left: 16px;
  margin-left: 12px;
  border-left: 1px solid var(--border-color);
}
/* 并发占用徽标：标题栏展示「N/M（排队 K）」，仅运行中或排队中有值时显示 */
.concurrency-badge {
  font-size: 12px;
  color: var(--text-secondary, #909399);
  white-space: nowrap;
}
.concurrency-queued {
  color: var(--el-color-info, #909399);
}
.ai-layout {
  display: flex;
  gap: 12px;
  /* 高度自适应：吃掉标题栏以外剩余高度（原弹窗时代的 calc(88vh-120px) 废弃） */
  flex: 1;
  min-height: 0;
  padding: 12px 14px;
}
/* 侧栏保持面板底色（导航区），与主区白色内容画布形成层次；右缘细线收边 */
.ai-sidebar {
  width: 292px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding-right: 12px;
  border-right: 1px solid var(--border-color);
}
.ai-cards {
  flex: 1;
  overflow: auto;
}
/* 启动器质感卡片：hover 上浮 + 左侧主色指示条，active 按压回弹，入场 stagger 递进入场 */
.ai-card {
  position: relative;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  padding: 10px 12px;
  margin-bottom: 8px;
  cursor: pointer;
  transition: transform 0.12s ease, border-color 0.2s ease, box-shadow 0.2s ease;
  animation: aiCardEnter 0.3s ease-out backwards;
}
/* 入场 stagger：nth-child 递增 60ms（v-for 的 index 不进 scoped style，用 CSS 写前 8 张 delay） */
.ai-card:nth-child(2) { animation-delay: 0.06s; }
.ai-card:nth-child(3) { animation-delay: 0.12s; }
.ai-card:nth-child(4) { animation-delay: 0.18s; }
.ai-card:nth-child(5) { animation-delay: 0.24s; }
.ai-card:nth-child(6) { animation-delay: 0.3s; }
.ai-card:nth-child(7) { animation-delay: 0.36s; }
.ai-card:nth-child(8) { animation-delay: 0.42s; }
@keyframes aiCardEnter {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
/* 左侧 3px 主色指示条：hover 显现（伪元素实现，不占模板） */
.ai-card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 10px;
  bottom: 10px;
  width: 3px;
  border-radius: 0 2px 2px 0;
  background: var(--primary-color);
  opacity: 0;
  transition: opacity 0.2s ease;
}
.ai-card:hover {
  transform: translateY(-1px);
  border-color: var(--primary-light);
  box-shadow: var(--shadow-md);
}
.ai-card:hover::before {
  opacity: 1;
}
/* 点击瞬间：轻微按压回弹（transform 用更短过渡） */
.ai-card:active {
  transform: translateY(0) scale(0.98);
}
.ai-card-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
/* 「应用图标」块：34px 圆角方块，主色淡底 + 图标主色（el-icon 为 inline-flex，直接设宽高即居中） */
.ai-card-icon {
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  border-radius: var(--radius-md);
  background: var(--primary-bg);
  color: var(--primary-color, #409eff);
}
.ai-card-name {
  font-weight: 600;
  font-size: 13px;
  color: var(--text-primary);
}
.ai-card-desc {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-top: 4px;
  line-height: 1.6;
  /* 与图标块右侧文字区对齐（34px 图标 + 10px 间距） */
  padding-left: 44px;
}
.ai-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  /* 高度约束链起点：min-height:0 允许被 .ai-layout 的剩余高度压缩，
     否则 min-height:auto 默认值让长流式输出把面板撑破 */
  min-height: 0;
  overflow: hidden;
  /* 内容画布：白底圆角卡片浮于面板底色上，与侧栏导航区分层 */
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 10px 12px 12px;
}
/* el-tabs 撑满 .ai-main 剩余高度，并把高度约束传导给内容区 */
.ai-main :deep(.el-tabs) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.ai-main :deep(.el-tabs__content) {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.ai-main :deep(.el-tab-pane) {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.task-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.task-prompt {
  flex: 1;
  /* prompt 为指令语义：等宽小字呈现，单行省略保留 */
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-size: 11px;
  color: var(--text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.task-actions {
  display: flex;
  gap: 6px;
}
.followup-bar {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}
/* 底栏计量/失败信息：完成态展示耗时/token/成本，失败态展示 exitCode 与错误分类 */
.task-footer {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 8px;
  padding: 4px 0;
  font-size: 12px;
}
.metrics-text {
  color: var(--text-secondary, #909399);
}
.error-text {
  color: var(--el-color-danger, #f56c6c);
}
/* 运行中状态标签呼吸脉搏：主色 20% 透明度 box-shadow 扩散，仅运行中态 */
.status-running {
  animation: statusPulse 1.6s ease-in-out infinite;
}
@keyframes statusPulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(64, 158, 255, 0.2);
  }
  50% {
    box-shadow: 0 0 0 6px rgba(64, 158, 255, 0);
  }
}
/* 输出区终端化：浅灰底 + macOS 窗口语义装饰行（三圆点 + claude -p），sticky 常驻顶部 */
.task-output {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-tertiary);
  /* tab-pane 为纵向 flex 容器：输出区吃掉剩余高度并内部滚动，
     toolbar/followup-bar 保持自然高度，总高不超面板 */
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0 0 12px;
}
.term-bar {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 12px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-color);
  border-radius: var(--radius-md) var(--radius-md) 0 0;
}
.term-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  opacity: 0.7;
  flex-shrink: 0;
}
.term-dot-r {
  background: var(--danger-color);
}
.term-dot-y {
  background: var(--warning-color);
}
.term-dot-g {
  background: var(--success-color);
}
.term-cmd {
  margin-left: auto;
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-size: 11px;
  color: var(--text-tertiary);
}
.task-table {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  height: calc(100% - 80px);
  overflow: hidden;
  padding: 12px;
}
.task-output pre {
  margin: 0;
  padding: 10px 12px 0;
  font-size: 12px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: 'Cascadia Code', Consolas, monospace;
}
/* 空状态留白充足（配合 el-empty 默认内边距再放宽） */
.ai-main :deep(.el-empty) {
  padding: 48px 0;
}

/* 任务 Tab 关闭按钮常显：Element Plus card tabs 的 .is-icon-close 默认 width:0、
   hover/active 才展开，本面板任务 Tab 需直观可关闭。宽度取组件自身展开值 14px。
   选择器带 .el-tabs--card 以抬高特异性压过默认 width:0 规则 */
.ai-main :deep(.el-tabs--card .el-tabs__item .is-icon-close) {
  width: 14px;
}

/* Tab 宽度锁定：关闭按钮常显 14px 后，Element Plus card tabs 原生
   「hover 时收窄 item padding 至 13px 补偿按钮展开」（tabs.scss 的
   .el-tabs--card>.el-tabs__header .el-tabs__item.is-closable:hover 规则）
   反而造成 hover 时 Tab 净变窄 14px、后续 Tab 横移。此处把 hover/active
   状态 padding 锁定为与默认态一致（源码默认 padding: 0 20px，
   active.is-closable 本就是 20px，此处显式锁定防版本差异），消除宽度变化。
   选择器带 .ai-main + scoped 属性，特异性压过组件默认规则 */
.ai-main :deep(.el-tabs--card .el-tabs__item.is-closable:hover),
.ai-main :deep(.el-tabs--card .el-tabs__item.is-active.is-closable) {
  padding-left: 20px;
  padding-right: 20px;
}

/* card tabs 选中态视觉协调：active tab 文字用项目主色，与卡片 hover 主色边框呼应；
   底色/边框保持 Element 默认（与面板浅色背景已协调），不做多余覆盖 */
.ai-main :deep(.el-tabs--card .el-tabs__item.is-active) {
  color: var(--primary-color, #409eff);
}

/* 动画可访问性：用户系统偏好减少动效时，禁用本面板全部装饰动画与过渡 */
@media (prefers-reduced-motion: reduce) {
  .ai-card,
  .ai-card::before,
  .status-running {
    animation: none !important;
    transition: none !important;
  }
}
</style>
