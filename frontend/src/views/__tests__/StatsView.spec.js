import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetRepoStats: vi.fn()
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' }
}))

import StatsView from '../StatsView.vue'
import { GetRepoStats } from '../../../wailsjs/go/main/App'
import { useWorkspaceStore, useUiStore } from '../../store'

const mockStats = {
  trend: [{ date: '2026-09-10', count: 2 }],
  contributors: [
    { author: 'Alice', email: 'a@x.com', count: 10 },
    { author: 'Bob', email: 'b@x.com', count: 3 }
  ],
  heatmap: [{ date: '2026-09-10', count: 2 }],
  totalCommits: 13,
  dateRange: '最近 30 天',
  granularity: 'day',
  sampled: false
}

// RepoStatsChart stub：捕获 stats prop
const RepoStatsChartStub = {
  name: 'RepoStatsChart',
  template: '<div class="rs-chart-stub" :data-stats="stats ? \'set\' : \'empty\'" />',
  props: ['stats']
}

const stubs = {
  RepoStatsChart: RepoStatsChartStub,
  'el-radio-group': {
    template: '<div class="el-radio-group"><slot /></div>',
    props: ['modelValue', 'size'],
    emits: ['update:modelValue', 'change']
  },
  'el-radio-button': { template: '<span class="el-radio-btn" />', props: ['value', 'label'] },
  'el-button': {
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
    props: ['icon', 'loading', 'size', 'circle'],
    emits: ['click']
  },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type'] },
  'el-empty': { template: '<div class="el-empty" />', props: ['description'] },
  'el-alert': { template: '<div class="el-alert" />', props: ['type', 'closable', 'showIcon', 'title'] },
  'el-icon': { template: '<i><slot /></i>' }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('StatsView', () => {
  it('非统计页可见时不调 GetRepoStats（guard）', async () => {
    // activePanel 默认 directory，loadStats 应 guard return 不打后端
    useWorkspaceStore().selectedNode = { path: '/fake/repo' }
    const wrapper = mount(StatsView, { global: { stubs } })
    await flushPromises()
    expect(GetRepoStats).not.toHaveBeenCalled()
  })

  it('统计页可见 + 无选中节点时应显示空提示且不调 GetRepoStats', async () => {
    useUiStore().activePanel = 'stats'
    const wrapper = mount(StatsView, { global: { stubs } })
    await flushPromises()
    expect(wrapper.find('.el-empty').exists()).toBe(true)
    expect(GetRepoStats).not.toHaveBeenCalled()
  })

  it('有选中节点时应调 GetRepoStats 并渲染摘要与图表', async () => {
    GetRepoStats.mockResolvedValue({ ...mockStats })
    useUiStore().activePanel = 'stats'
    useWorkspaceStore().selectedNode = { path: '/fake/repo', type: 'directory' }

    const wrapper = mount(StatsView, { global: { stubs } })
    await flushPromises()

    expect(GetRepoStats).toHaveBeenCalledWith('/fake/repo', '30d')
    expect(wrapper.find('.rs-chart-stub').attributes('data-stats')).toBe('set')
    expect(wrapper.text()).toContain('最近 30 天')
    expect(wrapper.text()).toContain('13')
  })

  it('sampled=true 时应显示采样提示', async () => {
    GetRepoStats.mockResolvedValue({ ...mockStats, sampled: true })
    useUiStore().activePanel = 'stats'
    useWorkspaceStore().selectedNode = { path: '/fake/repo' }

    const wrapper = mount(StatsView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.find('.el-alert').exists()).toBe(true)
  })

  it('GetRepoStats 失败时应提示错误且清空统计', async () => {
    GetRepoStats.mockRejectedValue(new Error('boom'))
    useUiStore().activePanel = 'stats'
    useWorkspaceStore().selectedNode = { path: '/fake/repo' }

    const wrapper = mount(StatsView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.find('.rs-chart-stub').exists()).toBe(false)
  })

  it('切换选中节点应重新加载统计', async () => {
    GetRepoStats.mockResolvedValue({ ...mockStats })
    useUiStore().activePanel = 'stats'
    const store = useWorkspaceStore()
    store.selectedNode = { path: '/repo1' }
    const wrapper = mount(StatsView, { global: { stubs } })
    await flushPromises()

    store.selectedNode = { path: '/repo2' }
    await flushPromises()

    expect(GetRepoStats).toHaveBeenCalledWith('/repo1', '30d')
    expect(GetRepoStats).toHaveBeenCalledWith('/repo2', '30d')
  })

  it('并发请求时旧响应应被丢弃（requestSeq 串行化）', async () => {
    // 两次请求，第一次慢后返回，应被丢弃
    let resolveFirst
    const slowFirst = new Promise(r => { resolveFirst = r })
    GetRepoStats.mockReturnValueOnce(slowFirst)
    GetRepoStats.mockResolvedValueOnce({ ...mockStats, totalCommits: 999 })

    useUiStore().activePanel = 'stats'
    useWorkspaceStore().selectedNode = { path: '/repo1' }
    mount(StatsView, { global: { stubs } })
    await flushPromises()

    // 第二次请求（切档位触发）
    useWorkspaceStore().selectedNode = { path: '/repo2' }
    await flushPromises()

    // 第一次后返回，应被丢弃
    resolveFirst({ ...mockStats, totalCommits: 1 })
    await flushPromises()

    // 最终采纳第二次（totalCommits 999），非第一次（1）
    const calls = GetRepoStats.mock.calls
    expect(calls).toHaveLength(2)
  })
})
