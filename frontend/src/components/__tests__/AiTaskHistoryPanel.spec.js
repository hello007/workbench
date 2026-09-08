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

const getAiTaskHistoryMock = vi.fn(() => Promise.resolve(mockHistory))
const getAiTaskHistoryOutputMock = vi.fn(() => Promise.resolve('归档输出全文\n第二行'))
const deleteAiTaskHistoryMock = vi.fn(() => Promise.resolve(true))
const clearAiTaskHistoryMock = vi.fn(() => Promise.resolve(1))
const getAiFunctionsMock = vi.fn(() =>
  Promise.resolve([
    { id: 'weekly-report', name: '生成周报' },
    { id: 'meeting-book', name: '预约腾讯会议' }
  ])
)

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetAiFunctions: (...args) => getAiFunctionsMock(...args),
  GetAiTaskHistory: (...args) => getAiTaskHistoryMock(...args),
  GetAiTaskHistoryOutput: (...args) => getAiTaskHistoryOutputMock(...args),
  DeleteAiTaskHistory: (...args) => deleteAiTaskHistoryMock(...args),
  ClearAiTaskHistory: (...args) => clearAiTaskHistoryMock(...args)
}))

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
  'el-select': { template: '<select class="el-select"><slot /></select>', props: ['modelValue'] },
  'el-option': { template: '<option class="el-option" :value="value">{{ label }}</option>', props: ['label', 'value'] },
  'el-date-picker': { template: '<input class="el-date-picker" />', props: ['modelValue'] },
  'el-button': {
    emits: ['click'],
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>'
  },
  'el-tag': { template: '<span class="tag"><slot /></span>', props: ['type'] },
  'el-table': {
    template: '<div class="el-table"><slot /></div>',
    props: ['data', 'size', 'border']
  },
  // 行 stub：遍历 data 逐行渲染 scoped slot（供「查看输出/删除」按钮渲染与断言）
  'el-table-column': {
    props: ['prop', 'label', 'width', 'fixed'],
    template: '<div class="el-table-col"><span class="col-label">{{ label }}</span><div v-for="row in $parent.data" :key="row.id"><slot :row="row" /></div></div>'
  }
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
})
