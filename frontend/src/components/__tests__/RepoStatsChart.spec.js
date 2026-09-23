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
  dirLineStats: [
    { path: 'service', insertions: 33, deletions: 13 },
    { path: 'web', insertions: 5, deletions: 1 }
  ],
  topFileLineStats: [
    { path: 'service/repo_stats.go', insertions: 20, deletions: 5 },
    { path: 'web/StatsView.vue', insertions: 5, deletions: 1 }
  ],
  heatmap: [{ date: '2026-09-10', count: 2 }, { date: '2026-09-11', count: 5 }],
  totalCommits: 7,
  dateRange: '最近 7 天',
  granularity: 'day',
  sampled: false
}

// el-radio-group stub：点击容器在 slot 内按钮 value 序列中循环推进 modelValue
// （commits→lines→commits / dirs→files→dirs），两组维度切换器各自独立驱动
const elRadioGroupStub = {
  name: 'ElRadioGroup',
  props: ['modelValue', 'size'],
  emits: ['update:modelValue'],
  computed: {
    buttonValues() {
      const slot = (this.$slots.default && this.$slots.default()) || []
      return slot.map(v => v.props && v.props.value).filter(v => v !== undefined)
    }
  },
  methods: {
    onClick() {
      const vals = this.buttonValues
      if (!vals.length) return
      const idx = vals.indexOf(this.modelValue)
      this.$emit('update:modelValue', vals[(idx + 1) % vals.length])
    }
  },
  template: '<div class="el-radio-group-stub" @click="onClick"><slot /></div>'
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
  it('stats 传入时应渲染四个图表区域', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    expect(wrapper.findAll('.v-chart-stub')).toHaveLength(4)
    expect(wrapper.find('.chart-trend').exists()).toBe(true)
    expect(wrapper.find('.chart-heatmap').exists()).toBe(true)
    expect(wrapper.find('.chart-contributor').exists()).toBe(true)
    expect(wrapper.find('.chart-path-line').exists()).toBe(true)
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

  it('应渲染四个图表标题', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    const titles = wrapper.findAll('.chart-title')
    expect(titles[0].text()).toBe('提交趋势')
    expect(titles[1].text()).toBe('活跃度热力图')
    expect(titles[2].text()).toBe('贡献者排名')
    expect(titles[3].text()).toBe('行数分布')
  })

  it('两组维度切换器各渲染两个选项（贡献者组 + 行数分布组）', () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    const groups = wrapper.findAll('.el-radio-group-stub')
    expect(groups).toHaveLength(2)
    // 贡献者组按钮
    const contributorButtons = groups[0].findAll('.el-radio-button-stub')
    expect(contributorButtons).toHaveLength(2)
    expect(contributorButtons[0].text()).toBe('提交数')
    expect(contributorButtons[1].text()).toBe('行数')
    // 行数分布组按钮
    const pathButtons = groups[1].findAll('.el-radio-button-stub')
    expect(pathButtons).toHaveLength(2)
    expect(pathButtons[0].text()).toBe('目录')
    expect(pathButtons[1].text()).toBe('文件')
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

    // 其余三图不受贡献者维度切换影响（行数分布图 series 名同为总变更行数但 option 独立）
    const stubs = wrapper.findAll('.v-chart-stub')
    expect(stubs[0].attributes('data-series-name')).toBe('提交数')
  })

  it('行数分布默认目录维度，点击文件维度后切换数据源', async () => {
    const wrapper = mount(RepoStatsChart, { props: { stats: mockStats }, global: { stubs: globalStubs } })
    const pathChart = wrapper.find('.v-chart-stub.chart-path-line')
    expect(pathChart.attributes('data-series-name')).toBe('总变更行数')
    // 默认目录维度：y 轴为目录桶
    expect(pathChart.attributes('data-option')).toBe('set')

    const groups = wrapper.findAll('.el-radio-group-stub')
    await groups[1].trigger('click')
    // 文件维度仍为同一 series 名（同 builder），option 已按 topFileLineStats 重建
    expect(pathChart.attributes('data-series-name')).toBe('总变更行数')
  })
})
