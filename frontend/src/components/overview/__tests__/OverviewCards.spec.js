import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import RepoPendingCard from '../RepoPendingCard.vue'
import RepoStaleCard from '../RepoStaleCard.vue'
import TodoBacklogCard from '../TodoBacklogCard.vue'
import AiCostCard from '../AiCostCard.vue'
import AiSuccessCard from '../AiSuccessCard.vue'

const stubs = { 'el-icon': true, 'el-button': true }

const baseRepo = {
  pinnedTotal: 3,
  pendingCount: 0,
  staleCount: 0,
  staleDays: 7,
  missingCount: 0,
  noCommitCount: 0
}

const baseAI = {
  today: { costUsd: 0, taskCount: 0, inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheCreationTokens: 0 },
  week: { costUsd: 0, taskCount: 0, inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheCreationTokens: 0 },
  successRate: 0,
  hasData: false
}

describe('概览卡片', () => {
  it('M1 待收尾仓库展示计数、pin 总数与失效单列', () => {
    const wrapper = mount(RepoPendingCard, {
      props: { repos: { ...baseRepo, pendingCount: 2, missingCount: 1 } },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('2')
    expect(wrapper.text()).toContain('共关注 3 个仓库')
    expect(wrapper.text()).toContain('失效仓库 1 个')
    expect(wrapper.find('.overview-card').classes()).toContain('highlighted')
  })

  it('M2 久未推进仓库展示阈值与无提交单列', () => {
    const wrapper = mount(RepoStaleCard, {
      props: { repos: { ...baseRepo, staleCount: 1, staleDays: 14, noCommitCount: 2 } },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('1')
    expect(wrapper.text()).toContain('超过 14 天未提交')
    expect(wrapper.text()).toContain('无提交 2 个')
  })

  it('M1/M2 无 pin 仓库时显示「未关注仓库」空态', () => {
    const empty = { pinnedTotal: 0, pendingCount: 0, staleCount: 0, staleDays: 7, missingCount: 0, noCommitCount: 0 }
    const pending = mount(RepoPendingCard, { props: { repos: empty }, global: { stubs } })
    expect(pending.text()).toContain('未关注仓库')
    const stale = mount(RepoStaleCard, { props: { repos: empty }, global: { stubs } })
    expect(stale.text()).toContain('未关注仓库')
  })

  it('M3 待办积压展示条数与最老滞留天数，≥7 天高亮', () => {
    const wrapper = mount(TodoBacklogCard, {
      props: {
        todos: { pendingCount: 3, oldestDays: 10, oldestId: 't1', oldestText: '十天前的待办' }
      },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('3')
    expect(wrapper.text()).toContain('最老已滞留 10 天')
    expect(wrapper.text()).toContain('十天前的待办')
    expect(wrapper.find('.overview-card').classes()).toContain('highlighted')
  })

  it('M3 下钻事件带 oldestId 载荷', async () => {
    const wrapper = mount(TodoBacklogCard, {
      props: { todos: { pendingCount: 1, oldestDays: 2, oldestId: 't9', oldestText: 'x' } },
      global: { stubs }
    })
    await wrapper.find('.overview-card').trigger('click')
    expect(wrapper.emitted('drill')[0][0]).toEqual({ todoId: 't9' })
  })

  it('M4 AI 成本展示今日主数值、本周副值与 token 四项', () => {
    const wrapper = mount(AiCostCard, {
      props: {
        ai: {
          ...baseAI,
          today: { costUsd: 0.1234, taskCount: 1, inputTokens: 1000, outputTokens: 500, cacheReadTokens: 200, cacheCreationTokens: 100 },
          week: { costUsd: 1.5, taskCount: 3, inputTokens: 2000, outputTokens: 800, cacheReadTokens: 400, cacheCreationTokens: 200 }
        }
      },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('$0.12')
    expect(wrapper.text()).toContain('本周 $1.50')
    expect(wrapper.text()).toContain('入 2.0k')
    expect(wrapper.text()).toContain('缓存写 200')
    expect(wrapper.text()).toContain('近 90 天')
  })

  it('M5 无有效任务显示「无数据」而不是 0%', () => {
    const wrapper = mount(AiSuccessCard, {
      props: { ai: { ...baseAI, hasData: false, successRate: 0 } },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('无数据')
  })

  it('M5 有数据时展示百分比', () => {
    const wrapper = mount(AiSuccessCard, {
      props: { ai: { ...baseAI, hasData: true, successRate: 0.75 } },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('75.0%')
  })

  it('分区错误只降级当前卡片并展示重试', async () => {
    const wrapper = mount(RepoPendingCard, {
      props: { repos: { ...baseRepo, error: '读取仓库状态失败' } },
      global: { stubs }
    })
    expect(wrapper.text()).toContain('读取仓库状态失败')
    expect(wrapper.find('.value-num').exists()).toBe(false)
    // 错误态下点击卡片仍可下钻（切面板），重试由 shell 的 retry 按钮发出
    await wrapper.find('.overview-card').trigger('click')
    expect(wrapper.emitted('drill')).toBeTruthy()
  })
})
