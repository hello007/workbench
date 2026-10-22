import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BranchCompareStatsBar from '../BranchCompareStatsBar.vue'

const stubs = {
  'el-alert': { template: '<div class="el-alert" :data-type="type">{{ title }}<slot /></div>', props: ['type', 'title', 'closable', 'showIcon'] }
}

const mountBar = (props) => mount(BranchCompareStatsBar, { props, global: { stubs } })

const BASE_STATS = { aOnly: 3, bOnly: 5, matched: 12, patchMatched: 2, commonBelow: 40 }

describe('BranchCompareStatsBar.vue', () => {
  it('渲染四段计数：A 独有 / 匹配（含疑似拣选）/ B 独有 / 共同历史', () => {
    const wrapper = mountBar({ stats: BASE_STATS })
    const line = wrapper.find('.stats-line')
    expect(line.exists()).toBe(true)
    expect(line.text()).toContain('A 独有 3')
    expect(line.text()).toContain('匹配 12')
    expect(line.text()).toContain('疑似拣选 2')
    expect(line.text()).toContain('B 独有 5')
    expect(line.text()).toContain('merge-base 之下共同历史 40')
  })

  it('patchMatched 为 0 时不展示疑似拣选括号', () => {
    const wrapper = mountBar({ stats: { ...BASE_STATS, patchMatched: 0 } })
    expect(wrapper.find('.stats-line').text()).not.toContain('疑似拣选')
  })

  it('stats 为 null 时不渲染数字行', () => {
    const wrapper = mountBar({ stats: null })
    expect(wrapper.find('.stats-line').exists()).toBe(false)
  })

  it('truncated=true 时展示仅显示前 2000 条警告', () => {
    const wrapper = mountBar({ stats: BASE_STATS, truncated: true })
    const alerts = wrapper.findAll('.el-alert')
    expect(alerts).toHaveLength(1)
    expect(alerts[0].text()).toContain('仅显示前 2000 条')
  })

  it('unrelated=true（无共同祖先）时展示无共同祖先警告', () => {
    const wrapper = mountBar({ stats: BASE_STATS, unrelated: true })
    const alerts = wrapper.findAll('.el-alert')
    expect(alerts).toHaveLength(1)
    expect(alerts[0].text()).toContain('无共同祖先')
  })

  it('truncated 与 unrelated 同时为 true 时展示两条警告', () => {
    const wrapper = mountBar({ stats: BASE_STATS, truncated: true, unrelated: true })
    expect(wrapper.findAll('.el-alert')).toHaveLength(2)
  })
})
