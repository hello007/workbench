import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// Mock ECharts 模块（canvas 在 jsdom 不可用，仅验组件编排与 option 传递）
vi.mock('echarts/core', () => ({ use: vi.fn() }))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: {} }))
vi.mock('echarts/charts', () => ({ LineChart: {}, BarChart: {}, HeatmapChart: {} }))
vi.mock('echarts/components', () => ({
  GridComponent: {},
  TooltipComponent: {},
  VisualMapComponent: {},
  CalendarComponent: {}
}))

// Mock vue-echarts 的 VChart：捕获 option prop 供断言
vi.mock('vue-echarts', () => ({
  default: {
    name: 'VChart',
    props: ['option', 'loading', 'autoresize'],
    template: '<div class="v-chart-stub" :data-option="option && Object.keys(option).length ? \'set\' : \'empty\'" />'
  }
}))

import RepoStatsChart from '../RepoStatsChart.vue'

const mockStats = {
  trend: [{ date: '2026-09-10', count: 2 }, { date: '2026-09-11', count: 5 }],
  contributors: [
    { author: 'Alice', email: 'a@x.com', count: 10 },
    { author: 'Bob', email: 'b@x.com', count: 3 }
  ],
  heatmap: [{ date: '2026-09-10', count: 2 }, { date: '2026-09-11', count: 5 }],
  totalCommits: 7,
  dateRange: '最近 7 天',
  granularity: 'day',
  sampled: false
}

describe('RepoStatsChart', () => {
  it('stats 传入时应渲染三个图表区域', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats } })
    expect(wrapper.findAll('.v-chart-stub')).toHaveLength(3)
    expect(wrapper.find('.chart-trend').exists()).toBe(true)
    expect(wrapper.find('.chart-heatmap').exists()).toBe(true)
    expect(wrapper.find('.chart-contributor').exists()).toBe(true)
  })

  it('stats 传入时各图表 option 应已构造（非空）', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats } })
    const stubs = wrapper.findAll('.v-chart-stub')
    stubs.forEach(s => {
      expect(s.attributes('data-option')).toBe('set')
    })
  })

  it('stats 为 null 时图表 option 应为空（loading 态）', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: null } })
    const stubs = wrapper.findAll('.v-chart-stub')
    stubs.forEach(s => {
      expect(s.attributes('data-option')).toBe('empty')
    })
  })

  it('应渲染三个图表标题', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats } })
    const titles = wrapper.findAll('.chart-title')
    expect(titles[0].text()).toBe('提交趋势')
    expect(titles[1].text()).toBe('活跃度热力图')
    expect(titles[2].text()).toBe('贡献者排名')
  })
})
