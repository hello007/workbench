import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ElMessage } from 'element-plus'
import OverviewView from '../OverviewView.vue'
import { useUiStore } from '../../store'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

const { wailsMock } = vi.hoisted(() => ({
  wailsMock: { GetOverviewSnapshot: vi.fn() }
}))

vi.mock('../../../wailsjs/go/main/App', () => wailsMock)

const emptyBlock = {
  costUsd: 0,
  taskCount: 0,
  inputTokens: 0,
  outputTokens: 0,
  cacheReadTokens: 0,
  cacheCreationTokens: 0
}

function makeSnapshot(overrides = {}) {
  return {
    generatedAt: Date.now(),
    range: 'today',
    repos: { pinnedTotal: 2, pendingCount: 0, staleCount: 0, staleDays: 7, missingCount: 0, noCommitCount: 0, ...(overrides.repos || {}) },
    ai: {
      today: emptyBlock,
      week: emptyBlock,
      successRate: 0,
      hasData: false,
      ...(overrides.ai || {})
    },
    todos: { pendingCount: 0, oldestDays: 0, oldestId: '', oldestText: '', ...(overrides.todos || {}) }
  }
}

const stubs = {
  'el-icon': true,
  'el-empty': true,
  'el-button': true
}

function mountView() {
  return mount(OverviewView, {
    global: {
      stubs,
      directives: { loading: {} }
    }
  })
}

describe('OverviewView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    wailsMock.GetOverviewSnapshot.mockReset()
  })

  it('进入面板即拉取快照并渲染 5 张卡片', async () => {
    wailsMock.GetOverviewSnapshot.mockResolvedValue(makeSnapshot())
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    expect(wailsMock.GetOverviewSnapshot).toHaveBeenCalledWith('', false)
    expect(wrapper.findAll('.overview-card')).toHaveLength(5)
    expect(wrapper.text()).toContain('待收尾仓库')
    expect(wrapper.text()).toContain('久未推进仓库')
    expect(wrapper.text()).toContain('待办积压')
    expect(wrapper.text()).toContain('AI 成本')
    expect(wrapper.text()).toContain('AI 成功率')
  })

  it('非激活面板不触发拉取', async () => {
    const ui = useUiStore()
    ui.activePanel = 'directory'
    mountView()
    await flushPromises()
    expect(wailsMock.GetOverviewSnapshot).not.toHaveBeenCalled()
  })

  it('点击刷新以 force=true 重新拉取', async () => {
    wailsMock.GetOverviewSnapshot.mockResolvedValue(makeSnapshot())
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('el-button-stub').trigger('click')
    await flushPromises()
    expect(wailsMock.GetOverviewSnapshot).toHaveBeenLastCalledWith('', true)
  })

  it('M1/M2 下钻切到状态看板', async () => {
    wailsMock.GetOverviewSnapshot.mockResolvedValue(makeSnapshot())
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    await wrapper.findAll('.overview-card')[0].trigger('click')
    expect(ui.activePanel).toBe('dashboard')
  })

  it('M3 下钻写入信箱并切到待办面板', async () => {
    wailsMock.GetOverviewSnapshot.mockResolvedValue(
      makeSnapshot({ todos: { pendingCount: 1, oldestDays: 9, oldestId: 'todo-1', oldestText: '旧待办' } })
    )
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    // 卡片 DOM 顺序按数据域：M1、M2、M4、M5、M3（待办域在最后）
    await wrapper.findAll('.overview-card')[4].trigger('click')
    expect(ui.activePanel).toBe('todo')
    expect(ui.overviewDrillRequest).toMatchObject({ target: 'todo', payload: { todoId: 'todo-1' } })
  })

  it('M4 下钻写入 ai 信箱并切到 AI 面板', async () => {
    wailsMock.GetOverviewSnapshot.mockResolvedValue(makeSnapshot())
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    await wrapper.findAll('.overview-card')[2].trigger('click')
    expect(ui.activePanel).toBe('ai')
    expect(ui.overviewDrillRequest).toMatchObject({ target: 'ai', payload: { range: 'week' } })
  })

  it('并发旧响应不覆盖新快照（requestSeq 守卫）', async () => {
    let resolveFirst
    let resolveSecond
    wailsMock.GetOverviewSnapshot
      .mockReturnValueOnce(new Promise((resolve) => { resolveFirst = resolve }))
      .mockReturnValueOnce(new Promise((resolve) => { resolveSecond = resolve }))

    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await Promise.resolve()

    await wrapper.find('el-button-stub').trigger('click')
    expect(wailsMock.GetOverviewSnapshot).toHaveBeenCalledTimes(2)
    resolveSecond(makeSnapshot({ repos: { pendingCount: 2 } }))
    await flushPromises()

    resolveFirst(makeSnapshot({ repos: { pendingCount: 9 } }))
    await flushPromises()

    expect(wrapper.findAll('.value-num')[0].text()).toBe('2')
  })

  it('拉取失败提示错误且不渲染快照', async () => {
    wailsMock.GetOverviewSnapshot.mockRejectedValue(new Error('boom'))
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalled()
    expect(wrapper.findAll('.overview-card')).toHaveLength(0)
  })
})
