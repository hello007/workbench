import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { ElMessage } from 'element-plus'
import OverviewView from '../OverviewView.vue'
import { useUiStore } from '../../store'

// 每个用例结束自动卸载 wrapper：watch(immediate) 常驻监听 activePanel，
// 不卸载会把陈旧实例泄漏到后续用例（消费端埋点断言会串扰）。
enableAutoUnmount(afterEach)
// console.warn spy 等局部 spy 用例间不留残留（restoreAllMocks 只作用于 spyOn，不动 vi.fn）
afterEach(() => { vi.restoreAllMocks() })

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

const { wailsMock } = vi.hoisted(() => ({
  wailsMock: {
    GetOverviewSnapshot: vi.fn(),
    // T4 门槛取证埋点：无返回值，默认 resolve；失败路径由用例局部 mockRejectedValue
    RecordOverviewOpen: vi.fn(() => Promise.resolve())
  }
}))

vi.mock('../../../wailsjs/go/main/App', () => wailsMock)

const emptyCost = {
  costUsd: 0,
  taskCount: 0,
  inputTokens: 0,
  outputTokens: 0,
  cacheReadTokens: 0,
  cacheCreationTokens: 0
}

const emptySelected = {
  cost: emptyCost,
  successRate: 0,
  hasData: false,
  successCount: 0,
  failedCount: 0,
  timeoutCount: 0,
  canceledCount: 0
}

function makeSnapshot({ repos = {}, ai = {}, todos = {}, range = 'week' } = {}) {
  return {
    generatedAt: Date.now(),
    range,
    repos: { pinnedTotal: 2, pendingCount: 0, staleCount: 0, staleDays: 7, missingCount: 0, noCommitCount: 0, ...repos },
    ai: {
      today: emptyCost,
      week: emptyCost,
      rangeKey: range,
      selected: emptySelected,
      successRate: 0,
      hasData: false,
      successCount: 0,
      failedCount: 0,
      timeoutCount: 0,
      canceledCount: 0,
      ...ai
    },
    todos: { pendingCount: 0, oldestDays: 0, oldestId: '', oldestText: '', ...todos }
  }
}

// el-radio-group 桩声明 v-model 与 change 事件，测试经事件驱动，验证模板接线本身。
const radioGroupStub = {
  template: '<div class="el-radio-group"><slot /></div>',
  props: ['modelValue', 'size'],
  emits: ['update:modelValue', 'change']
}
const radioButtonStub = {
  template: '<label class="el-radio-button"><slot /></label>',
  props: ['value']
}

const stubs = {
  'el-icon': true,
  'el-empty': true,
  'el-button': true,
  'el-radio-group': radioGroupStub,
  'el-radio-button': radioButtonStub
}

function mountView() {
  return mount(OverviewView, {
    global: {
      stubs,
      directives: { loading: {} }
    }
  })
}

/** 经 stub 切换档位：v-model 更新 rangeKey 后触发生效的 change。 */
async function switchRange(wrapper, key) {
  const group = wrapper.findComponent(radioGroupStub)
  await group.vm.$emit('update:modelValue', key)
  await group.vm.$emit('change', key)
  await flushPromises()
}

describe('OverviewView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    wailsMock.GetOverviewSnapshot.mockReset()
    wailsMock.RecordOverviewOpen.mockReset()
    wailsMock.RecordOverviewOpen.mockResolvedValue(undefined)
    vi.clearAllMocks()
  })

  // ---- 概览面板进入埋点（T4 Go/No-Go 门槛 G1 取证）----

  describe('进入来源消费（仅三个显式入口上报）', () => {
    beforeEach(() => {
      wailsMock.GetOverviewSnapshot.mockResolvedValue(makeSnapshot())
    })

    // 三个入口在 ActivityBar / Home 写入信箱，本组验证消费端对每种来源
    // 都只上报一次，且按小写字面量透传（对齐 service 白名单口径）。
    it.each(['click', 'shortcut', 'alt'])('显式入口 %s：命中 overview 上报一次且透传来源', async (source) => {
      const ui = useUiStore()
      ui.setOverviewEntrySource(source)
      ui.activePanel = 'overview'
      mountView()
      await flushPromises()

      expect(wailsMock.RecordOverviewOpen).toHaveBeenCalledTimes(1)
      expect(wailsMock.RecordOverviewOpen).toHaveBeenCalledWith(source)
      expect(ui.overviewEntrySource).toBe('')
    })

    it('会话恢复写回 activePanel（无来源）不上报', async () => {
      const ui = useUiStore()
      // 模拟 useSessionState.applySessionState 直接写回，不经过三个显式入口
      ui.activePanel = 'overview'
      mountView()
      await flushPromises()

      expect(wailsMock.RecordOverviewOpen).not.toHaveBeenCalled()
      expect(wailsMock.GetOverviewSnapshot).toHaveBeenCalledWith('week', false)
    })

    it('消费即清：切走再切回不重复上报', async () => {
      const ui = useUiStore()
      ui.setOverviewEntrySource('click')
      ui.activePanel = 'overview'
      mountView()
      await flushPromises()
      expect(wailsMock.RecordOverviewOpen).toHaveBeenCalledTimes(1)

      // 切走（watch 不命中）再切回（watch 命中但信箱已清空），均无新来源
      ui.activePanel = 'directory'
      await nextTick()
      ui.activePanel = 'overview'
      await flushPromises()
      expect(wailsMock.RecordOverviewOpen).toHaveBeenCalledTimes(1)
    })

    it('切档 @change 只重拉快照，不产生新埋点', async () => {
      const ui = useUiStore()
      ui.activePanel = 'overview'
      const wrapper = mountView()
      await flushPromises()

      await switchRange(wrapper, '7d')
      expect(wailsMock.RecordOverviewOpen).not.toHaveBeenCalled()
      expect(wailsMock.GetOverviewSnapshot).toHaveBeenLastCalledWith('7d', false)
    })

    it('手刷 load(true) 不产生新埋点', async () => {
      const ui = useUiStore()
      ui.activePanel = 'overview'
      const wrapper = mountView()
      await flushPromises()

      await wrapper.find('el-button-stub').trigger('click')
      await flushPromises()
      expect(wailsMock.RecordOverviewOpen).not.toHaveBeenCalled()
      expect(wailsMock.GetOverviewSnapshot).toHaveBeenLastCalledWith('week', true)
    })

    it('埋点调用失败静默降级，load(false) 仍执行且不弹错误提示', async () => {
      const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
      wailsMock.RecordOverviewOpen.mockRejectedValue(new Error('boom'))

      const ui = useUiStore()
      ui.setOverviewEntrySource('click')
      ui.activePanel = 'overview'
      const wrapper = mountView()
      await flushPromises()

      // 埋点是旁路观测面：失败不引入新错误码、不弹 ElMessage，快照照常拉取与渲染
      expect(wailsMock.GetOverviewSnapshot).toHaveBeenCalledWith('week', false)
      expect(wrapper.findAll('.overview-card')).toHaveLength(5)
      expect(ElMessage.error).not.toHaveBeenCalled()
      expect(warnSpy).toHaveBeenCalled()
    })
  })

  it('进入面板即拉取快照并渲染 5 张卡片', async () => {
    wailsMock.GetOverviewSnapshot.mockResolvedValue(makeSnapshot())
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    // 默认档 week 入参
    expect(wailsMock.GetOverviewSnapshot).toHaveBeenCalledWith('week', false)
    expect(wrapper.findAll('.overview-card')).toHaveLength(5)
    expect(wrapper.text()).toContain('待收尾仓库')
    expect(wrapper.text()).toContain('久未推进仓库')
    expect(wrapper.text()).toContain('待办积压')
    expect(wrapper.text()).toContain('AI 成本')
    expect(wrapper.text()).toContain('AI 成功率')
  })

  it('AI 域标题行渲染五档范围控件，默认本周', async () => {
    wailsMock.GetOverviewSnapshot.mockResolvedValue(makeSnapshot())
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    const group = wrapper.findComponent(radioGroupStub)
    expect(group.exists()).toBe(true)
    expect(group.props('modelValue')).toBe('week')
    const labels = wrapper.findAllComponents(radioButtonStub).map((b) => b.text())
    expect(labels).toEqual(['今日', '本周', '近 7 天', '近 30 天', '近 90 天'])
    // 控件仅出现在 AI 域标题行（1 个），仓库/待办域不加范围说明
    expect(wrapper.findAllComponents(radioGroupStub)).toHaveLength(1)
  })

  it('非激活面板不触发拉取', async () => {
    const ui = useUiStore()
    ui.activePanel = 'directory'
    mountView()
    await flushPromises()
    expect(wailsMock.GetOverviewSnapshot).not.toHaveBeenCalled()
  })

  it('点击刷新以 force=true 重拉', async () => {
    wailsMock.GetOverviewSnapshot.mockResolvedValue(makeSnapshot())
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('el-button-stub').trigger('click')
    await flushPromises()
    expect(wailsMock.GetOverviewSnapshot).toHaveBeenLastCalledWith('week', true)
  })

  it('切档以新档位、force=false 重拉，AI 卡片与口径句随档位变化', async () => {
    wailsMock.GetOverviewSnapshot
      .mockResolvedValueOnce(makeSnapshot())
      .mockResolvedValueOnce(
        makeSnapshot({
          range: '7d',
          ai: {
            rangeKey: '7d',
            selected: { ...emptySelected, cost: { ...emptyCost, costUsd: 3 }, hasData: true, successRate: 0.8, successCount: 4, failedCount: 1 }
          }
        })
      )

    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    await switchRange(wrapper, '7d')

    expect(wailsMock.GetOverviewSnapshot).toHaveBeenLastCalledWith('7d', false)
    const costCard = wrapper.findAll('.overview-card')[2]
    expect(costCard.find('.value-num').text()).toBe('$3.00')
    expect(costCard.text()).toContain('范围：近 7 天')
    const successCard = wrapper.findAll('.overview-card')[3]
    expect(successCard.find('.value-num').text()).toBe('80.0%')
  })

  it('切档不改变仓库/待办卡片数值（快照类不加时间窗）', async () => {
    const repoSnapshot = { repos: { pendingCount: 2 }, todos: { pendingCount: 3, oldestDays: 5, oldestId: 't1', oldestText: '旧待办' } }
    wailsMock.GetOverviewSnapshot
      .mockResolvedValueOnce(makeSnapshot(repoSnapshot))
      .mockResolvedValueOnce(makeSnapshot({ ...repoSnapshot, range: '90d' }))

    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    const repoValueBefore = wrapper.findAll('.overview-card')[0].find('.value-num').text()
    const todoValueBefore = wrapper.findAll('.overview-card')[4].find('.value-num').text()

    await switchRange(wrapper, '90d')

    expect(wrapper.findAll('.overview-card')[0].find('.value-num').text()).toBe(repoValueBefore)
    expect(wrapper.findAll('.overview-card')[4].find('.value-num').text()).toBe(todoValueBefore)
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

  it('M5 下钻写入 ai 信箱并预选失败 + 超时状态', async () => {
    wailsMock.GetOverviewSnapshot.mockResolvedValue(
      makeSnapshot({
        ai: { selected: { ...emptySelected, successRate: 0.5, hasData: true, successCount: 1, failedCount: 1 } }
      })
    )
    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    // 卡片顺序 M1、M2、M4、M5、M3：索引 3 为 AI 成功率卡片
    await wrapper.findAll('.overview-card')[3].trigger('click')
    expect(ui.activePanel).toBe('ai')
    expect(ui.overviewDrillRequest).toMatchObject({
      target: 'ai',
      payload: { range: 'week', statuses: ['failed', 'timeout'] }
    })
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

  it('切档后旧响应迟到不覆盖新档位结果（requestSeq 守卫）', async () => {
    let resolveRefresh
    let resolveSevenDay
    wailsMock.GetOverviewSnapshot
      .mockResolvedValueOnce(makeSnapshot()) // 首次进入（week）即时返回，先渲染出控件
      .mockReturnValueOnce(new Promise((resolve) => { resolveRefresh = resolve })) // 刷新在途
      .mockReturnValueOnce(new Promise((resolve) => { resolveSevenDay = resolve })) // 切档在途

    const ui = useUiStore()
    ui.activePanel = 'overview'
    const wrapper = mountView()
    await flushPromises()

    // 先发起一次刷新（seq 2），再切到 7d（seq 3）：两次都在途。
    await wrapper.find('el-button-stub').trigger('click')
    await switchRange(wrapper, '7d')

    // 先返回 7d 档，再迟到返回刷新（week）的旧响应
    resolveSevenDay(
      makeSnapshot({
        range: '7d',
        ai: { rangeKey: '7d', selected: { ...emptySelected, cost: { ...emptyCost, costUsd: 3 } } }
      })
    )
    await flushPromises()
    resolveRefresh(makeSnapshot({ repos: { pendingCount: 9 } }))
    await flushPromises()

    const costCard = wrapper.findAll('.overview-card')[2]
    expect(costCard.find('.value-num').text()).toBe('$3.00')
    expect(costCard.text()).toContain('范围：近 7 天')
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
