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

// Mock vue-echarts 的 VChart：捕获 option prop 供断言（data-series-name 暴露 series 名，
// 供贡献者维度切换断言：提交数 / 总变更行数）
vi.mock('vue-echarts', () => ({
  default: {
    name: 'VChart',
    props: ['option', 'loading', 'autoresize'],
    template: `<div class="v-chart-stub"
      :data-option="option && Object.keys(option).length ? 'set' : 'empty'"
      :data-series-name="option && option.series ? option.series[0].name : ''" />`
  }
}))

import RepoStatsChart from '../RepoStatsChart.vue'

const mockStats = {
  trend: [{ date: '2026-09-10', count: 2 }, { date: '2026-09-11', count: 5 }],
  contributors: [
    { author: 'Alice', email: 'a@x.com', count: 10, insertions: 100, deletions: 20 },
    { author: 'Bob', email: 'b@x.com', count: 3, insertions: 15, deletions: 45 }
  ],
  heatmap: [{ date: '2026-09-10', count: 2 }, { date: '2026-09-11', count: 5 }],
  totalCommits: 7,
  dateRange: '最近 7 天',
  granularity: 'day',
  sampled: false
}

// el-radio-group stub：点击容器即 emit update:modelValue('lines')，驱动 v-model 切换维度
const elRadioGroupStub = {
  name: 'ElRadioGroup',
  template: '<div class="el-radio-group-stub" @click="$emit(\'update:modelValue\', \'lines\')"><slot /></div>',
  props: ['modelValue', 'size'],
  emits: ['update:modelValue']
}
const elRadioButtonStub = {
  name: 'ElRadioButton',
  template: '<span class="el-radio-button-stub"><slot /></span>',
  props: ['value', 'label']
}

const globalStubs = {
  'el-radio-group': elRadioGroupStub,
  'el-radio-button': elRadioButtonStub
}

describe('RepoStatsChart', () => {
  it('stats 传入时应渲染三个图表区域', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    expect(wrapper.findAll('.v-chart-stub')).toHaveLength(3)
    expect(wrapper.find('.chart-trend').exists()).toBe(true)
    expect(wrapper.find('.chart-heatmap').exists()).toBe(true)
    expect(wrapper.find('.chart-contributor').exists()).toBe(true)
  })

  it('stats 传入时各图表 option 应已构造（非空）', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    const stubs = wrapper.findAll('.v-chart-stub')
    stubs.forEach(s => {
      expect(s.attributes('data-option')).toBe('set')
    })
  })

  it('stats 为 null 时图表 option 应为空（loading 态）', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: null }, global: { stubs: globalStubs } })
    const stubs = wrapper.findAll('.v-chart-stub')
    stubs.forEach(s => {
      expect(s.attributes('data-option')).toBe('empty')
    })
  })

  it('应渲染三个图表标题', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    const titles = wrapper.findAll('.chart-title')
    expect(titles[0].text()).toBe('提交趋势')
    expect(titles[1].text()).toBe('活跃度热力图')
    expect(titles[2].text()).toBe('贡献者排名')
  })

  it('贡献者卡片应渲染维度切换器（提交数/行数两个选项）', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    const group = wrapper.find('.el-radio-group-stub')
    expect(group.exists()).toBe(true)
    const buttons = wrapper.findAll('.el-radio-button-stub')
    expect(buttons).toHaveLength(2)
    expect(buttons[0].text()).toBe('提交数')
    expect(buttons[1].text()).toBe('行数')
  })

  it('默认维度为提交数，贡献者图 series 为提交数排名', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    const contributorChart = wrapper.find('.v-chart-stub.chart-contributor')
    expect(contributorChart.attributes('data-series-name')).toBe('提交数')
  })

  it('点击切换到行数维度后，贡献者图 series 应变为总变更行数排名', async () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    const contributorChart = wrapper.find('.v-chart-stub.chart-contributor')
    expect(contributorChart.attributes('data-series-name')).toBe('提交数')

    await wrapper.find('.el-radio-group-stub').trigger('click')
    expect(contributorChart.attributes('data-series-name')).toBe('总变更行数')

    // 其余两图不受维度切换影响（trend series 名固定提交数，heatmap 无 series 名 → 属性缺省）
    const stubs = wrapper.findAll('.v-chart-stub')
    expect(stubs[0].attributes('data-series-name')).toBe('提交数')
    expect(stubs[1].attributes('data-series-name')).toBeUndefined()
  })
})
