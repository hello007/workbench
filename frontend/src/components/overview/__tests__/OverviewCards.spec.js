import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import RepoPendingCard from '../RepoPendingCard.vue'
import RepoStaleCard from '../RepoStaleCard.vue'
import TodoBacklogCard from '../TodoBacklogCard.vue'
import AiCostCard from '../AiCostCard.vue'
import AiSuccessCard from '../AiSuccessCard.vue'
import { overviewRangeLabel, overviewRangeText } from '../overviewRange'

const stubs = { 'el-icon': true, 'el-button': true }

const baseRepo = {
  pinnedTotal: 3,
  pendingCount: 0,
  staleCount: 0,
  staleDays: 7,
  missingCount: 0,
  noCommitCount: 0
}

const emptyCost = {
  costUsd: 0,
  taskCount: 0,
  inputTokens: 0,
  outputTokens: 0,
  cacheReadTokens: 0,
  cacheCreationTokens: 0
}

const baseSelected = {
  cost: emptyCost,
  successRate: 0,
  hasData: false,
  successCount: 0,
  failedCount: 0,
  timeoutCount: 0,
  canceledCount: 0
}

// 顶层 today/week 是恒定对比基准；selected 是所选档位的完整统计。
const baseAI = {
  today: emptyCost,
  week: emptyCost,
  rangeKey: 'week',
  selected: baseSelected,
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

  it('M4 AI 成本主数值与 token 读所选档位，week 档 hint 显示今日对比', () => {
    const wrapper = mount(AiCostCard, {
      props: {
        ai: {
          ...baseAI,
          today: { ...emptyCost, costUsd: 0.1234 },
          week: { ...emptyCost, costUsd: 1.5 },
          selected: {
            ...baseSelected,
            cost: { costUsd: 1.5, taskCount: 3, inputTokens: 2000, outputTokens: 800, cacheReadTokens: 400, cacheCreationTokens: 200 }
          }
        }
      },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('$1.50')
    expect(wrapper.text()).toContain('今日 $0.12')
    expect(wrapper.text()).toContain('入 2.0k')
    expect(wrapper.text()).toContain('缓存写 200')
    expect(wrapper.text()).toContain('范围：本周')
    expect(wrapper.text()).toContain('历史保留上限：近 90 天 / 最多 2000 条')
  })

  it('M4 today 档 hint 显示本周对比', () => {
    const wrapper = mount(AiCostCard, {
      props: {
        ai: {
          ...baseAI,
          rangeKey: 'today',
          today: { ...emptyCost, costUsd: 0.2 },
          week: { ...emptyCost, costUsd: 1.5 },
          selected: { ...baseSelected, cost: { ...emptyCost, costUsd: 0.2 } }
        }
      },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('$0.20')
    expect(wrapper.text()).toContain('本周 $1.50')
    expect(wrapper.text()).toContain('范围：今日')
  })

  it('M4 近 N 天档 hint 同时显示今日与本周对比，下钻带当前档位', async () => {
    const wrapper = mount(AiCostCard, {
      props: {
        ai: {
          ...baseAI,
          rangeKey: '7d',
          today: { ...emptyCost, costUsd: 0.2 },
          week: { ...emptyCost, costUsd: 1.5 },
          selected: { ...baseSelected, cost: { ...emptyCost, costUsd: 3 } }
        }
      },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('$3.00')
    expect(wrapper.text()).toContain('今日 $0.20 · 本周 $1.50')
    expect(wrapper.text()).toContain('范围：近 7 天')
    await wrapper.find('.overview-card').trigger('click')
    expect(wrapper.emitted('drill')[0][0]).toEqual({ range: '7d' })
  })

  it('M5 无有效任务显示「无数据」而不是 0%', () => {
    const wrapper = mount(AiSuccessCard, {
      props: { ai: { ...baseAI, selected: { ...baseSelected, hasData: false, successRate: 0 } } },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('无数据')
  })

  it('M5 有数据时展示百分比', () => {
    const wrapper = mount(AiSuccessCard, {
      props: { ai: { ...baseAI, selected: { ...baseSelected, hasData: true, successRate: 0.75 } } },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('75.0%')
  })

  it('M5 有数据时展示四状态计数，成功率可自行验算', () => {
    const wrapper = mount(AiSuccessCard, {
      props: {
        ai: {
          ...baseAI,
          selected: { ...baseSelected, hasData: true, successRate: 0.75, successCount: 3, failedCount: 1, timeoutCount: 0, canceledCount: 2 }
        }
      },
      global: { stubs }
    })
    const text = wrapper.text()
    expect(text).toContain('成功 3')
    expect(text).toContain('失败 1')
    expect(text).toContain('超时 0')
    expect(text).toContain('取消 2')
  })

  it('M5 无有效任务但存在取消任务时说明原因', () => {
    const wrapper = mount(AiSuccessCard, {
      props: { ai: { ...baseAI, selected: { ...baseSelected, hasData: false, successRate: 0, canceledCount: 4 } } },
      global: { stubs }
    })
    expect(wrapper.find('.value-num').text()).toBe('无数据')
    expect(wrapper.text()).toContain('4 个任务均被取消')
  })

  it('M5 全部任务取消时下钻载荷改为取消状态，避免点开为空', async () => {
    const wrapper = mount(AiSuccessCard, {
      props: { ai: { ...baseAI, selected: { ...baseSelected, hasData: false, successRate: 0, canceledCount: 4 } } },
      global: { stubs }
    })
    await wrapper.find('.overview-card').trigger('click')
    expect(wrapper.emitted('drill')[0][0]).toEqual({ range: 'week', statuses: ['canceled'] })
  })

  it('M5 无数据且无取消时下钻载荷回落失败+超时', async () => {
    const wrapper = mount(AiSuccessCard, {
      props: { ai: { ...baseAI, selected: { ...baseSelected, hasData: false, successRate: 0 } } },
      global: { stubs }
    })
    await wrapper.find('.overview-card').trigger('click')
    expect(wrapper.emitted('drill')[0][0]).toEqual({ range: 'week', statuses: ['failed', 'timeout'] })
  })

  it('M5 下钻载荷带入失败+超时状态集合', async () => {
    const wrapper = mount(AiSuccessCard, {
      props: {
        ai: { ...baseAI, selected: { ...baseSelected, hasData: true, successRate: 0.5, successCount: 1, failedCount: 1 } }
      },
      global: { stubs }
    })
    await wrapper.find('.overview-card').trigger('click')
    expect(wrapper.emitted('drill')[0][0]).toEqual({ range: 'week', statuses: ['failed', 'timeout'] })
  })

  it('M5 口径句与下钻档位随所选档位变化', async () => {
    const wrapper = mount(AiSuccessCard, {
      props: {
        ai: {
          ...baseAI,
          rangeKey: '30d',
          selected: { ...baseSelected, hasData: true, successRate: 0.5, successCount: 1, failedCount: 1 }
        }
      },
      global: { stubs }
    })
    expect(wrapper.text()).toContain('近 30 天 success ÷（success+failed+timeout）')
    expect(wrapper.text()).toContain('范围：近 30 天')
    await wrapper.find('.overview-card').trigger('click')
    expect(wrapper.emitted('drill')[0][0]).toEqual({ range: '30d', statuses: ['failed', 'timeout'] })
  })

  it('AI 卡片 ai.error 降级：显示加载失败与重试，不渲染 0 主数值', async () => {
    for (const Card of [AiCostCard, AiSuccessCard]) {
      const wrapper = mount(Card, {
        props: { ai: { ...baseAI, error: 'AI 统计加载失败' } },
        global: { stubs }
      })
      expect(wrapper.find('.card-error').exists()).toBe(true)
      expect(wrapper.find('.error-text').text()).toContain('加载失败')
      // el-button 被 stub 为 true 时 slot 文本不渲染，按真实 DOM 断言重试按钮存在且可发出 retry。
      const retryBtn = wrapper.find('.card-error').find('el-button-stub')
      expect(retryBtn.exists()).toBe(true)
      await retryBtn.trigger('click')
      expect(wrapper.emitted('retry')).toBeTruthy()
      // 降级态由 shell 的 v-if="error" 分支接管，主数值与口径句均不渲染，
      // 失败不得伪装成 $0.00 / 0.0%（对齐 R8）。
      expect(wrapper.find('.value-num').exists()).toBe(false)
      expect(wrapper.text()).not.toContain('$0.00')
      expect(wrapper.text()).not.toContain('0.0%')
    }
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

// overviewRange 纯函数直测：覆盖 AC「空串/非法 range 回落 week」与口径句档位联动。
describe('overviewRange 档位文案', () => {
  it('非法档位 365d 与空串均回落默认档 week', () => {
    expect(overviewRangeLabel('365d')).toBe('本周')
    expect(overviewRangeLabel('')).toBe('本周')
    expect(overviewRangeLabel(undefined)).toBe('本周')
  })

  it('五档标签与 spec §4 一致', () => {
    expect(overviewRangeLabel('today')).toBe('今日')
    expect(overviewRangeLabel('week')).toBe('本周')
    expect(overviewRangeLabel('7d')).toBe('近 7 天')
    expect(overviewRangeLabel('30d')).toBe('近 30 天')
    expect(overviewRangeLabel('90d')).toBe('近 90 天')
  })

  it('overviewRangeText 非法档位回落 week，保留上限恒定', () => {
    expect(overviewRangeText('365d')).toBe('范围：本周 · 历史保留上限：近 90 天 / 最多 2000 条')
    expect(overviewRangeText('')).toContain('范围：本周')
    expect(overviewRangeText('30d')).toContain('范围：近 30 天')
  })
})
