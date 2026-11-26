import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AiTaskHistoryPanel from '../AiTaskHistoryPanel.vue'

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn()
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

const mockHistory = [
  {
    id: 'aitask-1',
    functionId: 'weekly-report',
    name: '生成周报',
    prompt: '/ab-weekly-report',
    startedAt: Date.now() - 3600 * 1000,
    finishedAt: Date.now() - 3500 * 1000,
    status: 'success',
    exitCode: 0,
    error: '',
    sessionId: 's1',
    metrics: { durationMs: 60000, costUsd: 0.012, numTurns: 2, usage: { inputTokens: 1000, outputTokens: 500 } },
    outputFile: 'ai_task_history/aitask-1.txt',
    outputSize: 2048
  },
  {
    id: 'aitask-2',
    functionId: 'meeting-book',
    name: '预约腾讯会议',
    prompt: '/tencent-meeting-mcp',
    startedAt: Date.now() - 1800 * 1000,
    finishedAt: Date.now() - 1700 * 1000,
    status: 'failed',
    exitCode: 1,
    error: 'MCP 未配置',
    sessionId: '',
    metrics: null,
    outputFile: '',
    outputSize: 0
  }
]

const SaveFileDialogMock = vi.fn(() => Promise.resolve('D:/tmp/ai-task-history.csv'))
const getAiTaskHistoryMock = vi.fn(() => Promise.resolve(mockHistory))
const getAiTaskHistoryStatsMock = vi.fn(() =>
  Promise.resolve({
    totalCount: 2,
    successCount: 1,
    totalCostUsd: 0.012,
    totalInputTokens: 1000,
    totalOutputTokens: 500,
    totalCacheReadTokens: 200,
    totalCacheCreationTokens: 100,
    totalDurationMs: 60000,
    byFunction: [
      { functionId: 'weekly-report', functionName: '生成周报', count: 1, totalCostUsd: 0.012, totalTokens: 1500 },
      { functionId: 'meeting-book', functionName: '预约腾讯会议', count: 1, totalCostUsd: 0, totalTokens: 0 }
    ]
  })
)
const getAiTaskHistoryOutputMock = vi.fn(() => Promise.resolve('归档输出全文\n第二行'))
const deleteAiTaskHistoryMock = vi.fn(() => Promise.resolve(true))
const clearAiTaskHistoryMock = vi.fn(() => Promise.resolve(1))
const exportCsvMock = vi.fn(() => Promise.resolve('﻿时间,功能\n2026-01-01,周报'))
const exportMarkdownMock = vi.fn(() => Promise.resolve('# AI 任务历史报告'))
const saveFileMock = vi.fn(() => Promise.resolve())
const getAiFunctionsMock = vi.fn(() =>
  Promise.resolve([
    { id: 'weekly-report', name: '生成周报' },
    { id: 'meeting-book', name: '预约腾讯会议' }
  ])
)

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetClaudeDefaultModel: vi.fn(() => Promise.resolve('')),
  GetAiFunctions: (...args) => getAiFunctionsMock(...args),
  GetAiTaskHistory: (...args) => getAiTaskHistoryMock(...args),
  GetAiTaskHistoryStats: (...args) => getAiTaskHistoryStatsMock(...args),
  GetAiTaskHistoryOutput: (...args) => getAiTaskHistoryOutputMock(...args),
  ExportAiTaskHistoryCSV: (...args) => exportCsvMock(...args),
  ExportAiTaskHistoryMarkdown: (...args) => exportMarkdownMock(...args),
  SaveFile: (...args) => saveFileMock(...args),
  SaveFileDialog: (...args) => SaveFileDialogMock(...args),
  DeleteAiTaskHistory: (...args) => deleteAiTaskHistoryMock(...args),
  ClearAiTaskHistory: (...args) => clearAiTaskHistoryMock(...args)
}))

// 排行排序 radio 组：声明 props/emits，测试经 update:modelValue 驱动 v-model，
// 验证组件模板接线本身（只改 vm.rankSort 无法发现模板绑定写错导致的假绿）
const rankGroupStub = {
  template: '<div class="el-radio-group"><slot /></div>',
  props: ['modelValue', 'size'],
  emits: ['update:modelValue', 'change']
}
const rankButtonStub = {
  template: '<label class="el-radio-button"><slot /></label>',
  props: ['value', 'label']
}

// el-select / el-tag 桩声明 emits，测试经 $emit('change') / $emit('close') 驱动，
// 验证模板 @change / @close 接线本身（直接调 vm 方法无法发现模板绑定写错导致的假绿）。
const elSelectStub = {
  template: '<select class="el-select"><slot /></select>',
  props: ['modelValue'],
  emits: ['change', 'update:modelValue']
}
const elTagStub = {
  template: '<span class="tag"><slot /></span>',
  props: ['type', 'size', 'closable'],
  emits: ['close']
}

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="dlg"><slot /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['open', 'update:modelValue'],
    mounted() {
      if (this.modelValue) this.$emit('open')
    },
    watch: {
      modelValue(v) {
        if (v) this.$emit('open')
      }
    }
  },
  'el-drawer': {
    template: '<div v-if="modelValue" class="drawer"><slot /></div>',
    props: ['modelValue', 'title', 'size']
  },
  'el-select': elSelectStub,
  'el-option': { template: '<option class="el-option" :value="value">{{ label }}</option>', props: ['label', 'value'] },
  'el-date-picker': { template: '<input class="el-date-picker" />', props: ['modelValue'] },
  'el-button': {
    emits: ['click'],
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>'
  },
  'el-tag': elTagStub,
  'el-table': {
    template: '<div class="el-table"><slot /></div>',
    props: ['data', 'size', 'border']
  },
  // 行 stub：遍历 data 逐行渲染 scoped slot（供「查看输出/删除」按钮渲染与断言）
  'el-table-column': {
    props: ['prop', 'label', 'width', 'fixed'],
    template: '<div class="el-table-col"><span class="col-label">{{ label }}</span><div v-for="row in $parent.data" :key="row.id"><slot :row="row" /></div></div>'
  },
  'el-radio-group': rankGroupStub,
  'el-radio-button': rankButtonStub
}

const createWrapper = (visible = true) =>
  mount(AiTaskHistoryPanel, {
    props: { visible },
    global: { stubs }
  })

describe('AiTaskHistoryPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getAiTaskHistoryMock.mockResolvedValue(mockHistory)
    getAiTaskHistoryOutputMock.mockResolvedValue('归档输出全文\n第二行')
    deleteAiTaskHistoryMock.mockResolvedValue(true)
    clearAiTaskHistoryMock.mockResolvedValue(1)
    getAiTaskHistoryStatsMock.mockResolvedValue({
      totalCount: 2,
      successCount: 1,
      failedCount: 0,
      canceledCount: 0,
      timeoutCount: 0,
      totalCostUsd: 0.012,
      totalInputTokens: 1000,
      totalOutputTokens: 500,
      totalCacheReadTokens: 200,
      totalCacheCreationTokens: 100,
      totalDurationMs: 60000,
      byFunction: [
        { functionId: 'weekly-report', functionName: '生成周报', count: 1, totalCostUsd: 0.012, totalTokens: 1500 },
        { functionId: 'meeting-book', functionName: '预约腾讯会议', count: 1, totalCostUsd: 0, totalTokens: 0 }
      ]
    })
    exportCsvMock.mockResolvedValue('﻿时间,功能\n2026-01-01,周报')
    exportMarkdownMock.mockResolvedValue('# AI 任务历史报告')
    saveFileMock.mockResolvedValue()
    SaveFileDialogMock.mockResolvedValue('D:/tmp/ai-task-history.csv')
  })

  it('打开时加载功能列表与历史，按列表渲染', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    expect(getAiFunctionsMock).toHaveBeenCalled()
    expect(getAiTaskHistoryMock).toHaveBeenCalled()
    // 两条历史的功能名渲染
    const text = wrapper.find('.dlg').text()
    expect(text).toContain('生成周报')
    expect(text).toContain('预约腾讯会议')
    // 条数展示
    expect(wrapper.find('.filter-count').text()).toContain('共 2 条')
  })

  it('查看输出调 GetAiTaskHistoryOutput 懒加载归档文件全文', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    // 直接调 viewOutput（绕过 el-table-column stub 的 row 传递，验证方法行为）
    await wrapper.vm.viewOutput(mockHistory[0])
    await flushPromises()
    expect(getAiTaskHistoryOutputMock).toHaveBeenCalledWith('aitask-1')
    // 抽屉展示归档输出全文
    expect(wrapper.find('.drawer').text()).toContain('归档输出全文')
  })

  it('删除单条调 DeleteAiTaskHistory 后刷新列表', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    getAiTaskHistoryMock.mockClear()
    await wrapper.vm.removeOne(mockHistory[0])
    await flushPromises()
    expect(deleteAiTaskHistoryMock).toHaveBeenCalledWith('aitask-1')
    // 删除后重新拉取列表
    expect(getAiTaskHistoryMock).toHaveBeenCalled()
  })

  it('清理 30 天前调 ClearAiTaskHistory 后刷新列表', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    const clearBtn = wrapper.findAll('button').find((b) => b.text().includes('清理 30 天前'))
    expect(clearBtn).toBeTruthy()
    await clearBtn.trigger('click')
    await flushPromises()
    expect(clearAiTaskHistoryMock).toHaveBeenCalledWith({ olderThanDays: 30 })
  })

  it('查询按筛选条件调 GetAiTaskHistory', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    getAiTaskHistoryMock.mockClear()
    // 直接设筛选条件后点查询
    wrapper.vm.filter.status = 'failed'
    await flushPromises()
    const queryBtn = wrapper.findAll('button').find((b) => b.text().includes('查询'))
    await queryBtn.trigger('click')
    await flushPromises()
    expect(getAiTaskHistoryMock).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'failed' })
    )
  })

  it('耗时与输出大小格式化展示', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    const text = wrapper.find('.dlg').text()
    // aitask-1 耗时 60000ms → 1m 0s
    expect(text).toContain('1m 0s')
    // outputSize 2048 → 2.0 KB
    expect(text).toContain('2.0 KB')
    // 成本 0.012 → $0.012
    expect(text).toContain('$0.012')
  })

  it('统计区随查询渲染：数字卡与功能排行', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    expect(getAiTaskHistoryStatsMock).toHaveBeenCalled()
    const text = wrapper.find('.history-stats').text()
    // 运行次数与成功数
    expect(text).toContain('成功 1')
    // 总成本 0.012 → $0.012
    expect(text).toContain('$0.012')
    // 总 token = 1000+500+200+100 = 1800
    expect(wrapper.vm.totalTokens()).toBe(1800)
    // 功能排行含功能名
    expect(text).toContain('生成周报')
    // 排行 token 列头显式标注「入+出 token」，与统计卡片四分项口径区分
    expect(text).toContain('入+出 token')
  })

  it('导出 CSV：调 ExportAiTaskHistoryCSV 后经 SaveFileDialog 选路径调 SaveFile 落盘', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    const btn = wrapper.findAll('button').find((b) => b.text().includes('导出 CSV'))
    expect(btn).toBeTruthy()
    await btn.trigger('click')
    await flushPromises()
    expect(exportCsvMock).toHaveBeenCalled()
    expect(SaveFileDialogMock).toHaveBeenCalled()
    // 落盘调用：路径 + 文本 + 编码
    expect(saveFileMock).toHaveBeenCalledWith('D:/tmp/ai-task-history.csv', expect.stringContaining('时间'), 'utf-8')
  })

  it('功能排行可切换排序：次数 / 成本 / token，成本占比按筛选总成本计算', async () => {
    getAiTaskHistoryStatsMock.mockResolvedValue({
      // 四状态计数之和 = totalCount = byFunction 次数之和（6），对齐后端 Stats 不变式
      totalCount: 6,
      successCount: 2,
      failedCount: 1,
      canceledCount: 2,
      timeoutCount: 1,
      totalCostUsd: 1.0,
      totalInputTokens: 100,
      totalOutputTokens: 50,
      totalCacheReadTokens: 0,
      totalCacheCreationTokens: 0,
      totalDurationMs: 1000,
      byFunction: [
        { functionId: 'a', functionName: '高频低耗', count: 5, canceledCount: 2, totalCostUsd: 0.2, totalTokens: 100 },
        { functionId: 'b', functionName: '低频高耗', count: 1, canceledCount: 0, totalCostUsd: 0.8, totalTokens: 5000 }
      ]
    })
    const wrapper = createWrapper(true)
    await flushPromises()

    // 默认按次数降序
    expect(wrapper.vm.rankSort).toBe('count')
    expect(wrapper.vm.rankedFunctions[0].functionId).toBe('a')

    // 切成本：经 radio 组 v-model 事件切换（验证模板接线），低频高耗升到首位
    const rankGroup = wrapper.findComponent(rankGroupStub)
    expect(rankGroup.exists()).toBe(true)
    rankGroup.vm.$emit('update:modelValue', 'cost')
    await flushPromises()
    expect(wrapper.vm.rankSort).toBe('cost')
    expect(wrapper.vm.rankedFunctions[0].functionId).toBe('b')
    expect(wrapper.vm.costShare(wrapper.vm.rankedFunctions[0])).toBe('80.0')
    // 各功能项占比之和 ≈ 100%（允许一位小数的舍入误差）
    const shareSum = wrapper.vm.rankedFunctions
      .reduce((sum, row) => sum + Number(wrapper.vm.costShare(row)), 0)
    expect(Math.abs(shareSum - 100)).toBeLessThan(0.2)
    // 表格 DOM 顺序同步翻转（防只在 vm 上假绿）；按表头文本定位列，避免下标漂移
    const fnColumn = wrapper
      .findAll('.el-table-col')
      .find((c) => c.find('.col-label').text() === '功能排行')
    expect(fnColumn).toBeTruthy()
    const fnColumnText = fnColumn.text()
    expect(fnColumnText.indexOf('低频高耗')).toBeLessThan(fnColumnText.indexOf('高频低耗'))

    // 切 token：同样低频高耗在前
    rankGroup.vm.$emit('update:modelValue', 'tokens')
    await flushPromises()
    expect(wrapper.vm.rankSort).toBe('tokens')
    expect(wrapper.vm.rankedFunctions[0].functionId).toBe('b')

    // 排序切换按钮文案齐全，用户可发现切换入口
    expect(wrapper.find('.fn-rank-head').text()).toContain('次数')
    expect(wrapper.find('.fn-rank-head').text()).toContain('成本')
    expect(wrapper.find('.fn-rank-head').text()).toContain('token')

    // 取消数在排行表中显式展示，不清零 Count
    const text = wrapper.find('.history-stats').text()
    expect(text).toContain('取消 2')
    expect(text).toContain('5')
    // 四状态计数在运行次数卡片 title 中可见
    const statTitle = wrapper.find('.stat-value').attributes('title')
    expect(statTitle).toContain('失败 1')
    expect(statTitle).toContain('取消 2')
  })

  it('initialStatuses 下钻：列表与统计同带多状态筛选，事件驱动清除', async () => {
    const wrapper = mount(AiTaskHistoryPanel, {
      props: { visible: true, initialStatuses: ['failed', 'timeout'] },
      global: { stubs }
    })
    await flushPromises()
    // 列表与统计同一筛选范围：两者都应收到 statuses
    expect(getAiTaskHistoryMock).toHaveBeenCalledWith(
      expect.objectContaining({ statuses: ['failed', 'timeout'] })
    )
    expect(getAiTaskHistoryStatsMock).toHaveBeenCalledWith(
      expect.objectContaining({ statuses: ['failed', 'timeout'] })
    )
    // 筛选行显示可关闭的多状态标签
    expect(wrapper.find('.dlg').text()).toContain('失败 + 超时')

    // 用户改选单值状态：多状态必须清除，否则后端 Statuses 优先导致查询条件失效。
    // 经 el-select 桩的 change 事件驱动，验证模板 @change 接线本身（非直调方法）。
    wrapper.vm.filter.status = 'success'
    const statusSelect = wrapper.findAllComponents(elSelectStub)[1]
    statusSelect.vm.$emit('change', 'success')
    await flushPromises()
    expect(wrapper.vm.filter.statuses).toEqual([])

    // 清除标签立即刷新列表：经 el-tag 桩的 close 事件驱动，验证 @close 接线本身。
    getAiTaskHistoryMock.mockClear()
    wrapper.vm.filter.statuses = ['failed', 'timeout']
    await flushPromises()
    const filterTag = wrapper.findAllComponents(elTagStub)[0]
    filterTag.vm.$emit('close')
    await flushPromises()
    expect(wrapper.vm.filter.statuses).toEqual([])
    expect(getAiTaskHistoryMock).toHaveBeenCalled()
  })

  it('改筛选不点查询直接导出时，导出沿用上次查询生效条件', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    // 上次查询条件：状态=成功
    wrapper.vm.filter.status = 'success'
    const queryBtn = wrapper.findAll('button').find((b) => b.text().includes('查询'))
    await queryBtn.trigger('click')
    await flushPromises()
    expect(getAiTaskHistoryMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ status: 'success' })
    )

    // 用户改了筛选但未点查询（模拟下拉改选未生效）
    wrapper.vm.filter.status = 'failed'
    exportCsvMock.mockClear()
    exportMarkdownMock.mockClear()

    const csvBtn = wrapper.findAll('button').find((b) => b.text().includes('导出 CSV'))
    await csvBtn.trigger('click')
    await flushPromises()
    // 导出必须使用上次查询的 success，而非屏幕上未生效的 failed
    expect(exportCsvMock).toHaveBeenCalledWith(expect.objectContaining({ status: 'success' }))

    const mdBtn = wrapper.findAll('button').find((b) => b.text().includes('导出 Markdown'))
    await mdBtn.trigger('click')
    await flushPromises()
    expect(exportMarkdownMock).toHaveBeenCalledWith(expect.objectContaining({ status: 'success' }))
  })

  it('导出 Markdown：调 ExportAiTaskHistoryMarkdown 落盘', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    const btn = wrapper.findAll('button').find((b) => b.text().includes('导出 Markdown'))
    expect(btn).toBeTruthy()
    await btn.trigger('click')
    await flushPromises()
    expect(exportMarkdownMock).toHaveBeenCalled()
    expect(saveFileMock).toHaveBeenCalledWith('D:/tmp/ai-task-history.csv', '# AI 任务历史报告', 'utf-8')
  })
})
