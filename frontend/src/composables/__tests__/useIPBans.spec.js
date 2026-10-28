import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, defineComponent } from 'vue'
import { useIPBanList, useIPBanSuggestions, useIPBanTrend, useIPBanToast, formatBanExpiry, WEBSERVE_IP_BANNED_EVENT } from '../useIPBans'

// Wails 事件 handler 注册表（EventsOn mock 捕获）：返回注销闭包记录调用，
// 供卸载精准注销断言（cross-layer-contracts 闭包注销契约）。十期起三个 composable
// 共听 webserve-ip-banned（真实 runtime 支持同事件多监听器），mock 升级为按名
// 收集监听器列表 + 分发函数——eventHandlers[name] 恒为「派发一次触达全部监听器」
// 的可调用形态（对齐 ActivityBar.spec.js 同款升级），注销闭包按引用精准移除本
// 监听器，最后一个移除时清空注册项
const eventHandlers = {}
const eventListeners = {}
const offSpy = vi.fn()

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((name, cb) => {
    if (!eventListeners[name]) {
      eventListeners[name] = []
    }
    eventListeners[name].push(cb)
    eventHandlers[name] = (...args) => {
      for (const fn of eventListeners[name] || []) {
        fn(...args)
      }
    }
    return () => {
      const list = eventListeners[name]
      const idx = list ? list.indexOf(cb) : -1
      if (idx !== -1) {
        list.splice(idx, 1)
      }
      offSpy()
      if (!list || list.length === 0) {
        delete eventListeners[name]
        delete eventHandlers[name]
      }
    }
  })
}))

vi.mock('../../../wailsjs/go/main/App', () => ({
  ListIPBans: vi.fn(),
  ListIPBanSuggestions: vi.fn(),
  QueryIPBanTrend: vi.fn()
}))

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }
}))

import { ListIPBans, ListIPBanSuggestions, QueryIPBanTrend } from '../../../wailsjs/go/main/App'
import { ElMessage } from 'element-plus'

// 封禁条目 fixture（形状对齐 model.IPBanStatus：ip/bannedAt/expiresAt unix 秒/
// failCount/strike/expired bool）
const banEntry = (over = {}) => ({
  ip: '203.0.113.7',
  bannedAt: 1735689600,
  expiresAt: 1735689900,
  failCount: 10,
  strike: 1,
  expired: false,
  ...over
})

// 挂载一个消费 composable 的最小组件（composable 依赖组件生命周期注册/注销监听）
const mountWith = (setupFn) => {
  let exposed
  const Harness = defineComponent({
    setup() {
      exposed = setupFn()
      return () => h('div', { class: 'harness' }, JSON.stringify(exposed.snapshot()))
    }
  })
  const wrapper = mount(Harness)
  return { wrapper, exposed }
}

describe('useIPBanList', () => {
  beforeEach(() => {
    for (const k of Object.keys(eventHandlers)) delete eventHandlers[k]
    for (const k of Object.keys(eventListeners)) delete eventListeners[k]
    offSpy.mockClear()
    vi.clearAllMocks()
    ListIPBans.mockResolvedValue([])
  })

  it('挂载即拉取列表并注册事件监听，数据落地 ipBans', async () => {
    ListIPBans.mockResolvedValue([banEntry({ strike: 3 })])
    const { wrapper, exposed } = mountWith(() => {
      const { ipBans, reload } = useIPBanList()
      return { snapshot: () => ipBans.value, ipBans, reload }
    })
    await flush()
    expect(ListIPBans).toHaveBeenCalledTimes(1)
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeTypeOf('function')
    expect(exposed.ipBans.value).toHaveLength(1)
    expect(exposed.ipBans.value[0].ip).toBe('203.0.113.7')
    // 载荷整条透传（连接管控六期 strike 字段透传落地，徽标渲染由 ActivityBar spec 消费断言）
    expect(exposed.ipBans.value[0].strike).toBe(3)
    wrapper.unmount()
  })

  it('加载失败静默（保持空列表，不崩不弹错）', async () => {
    ListIPBans.mockRejectedValue(new Error('service not ready'))
    const { wrapper, exposed } = mountWith(() => {
      const { ipBans } = useIPBanList()
      return { snapshot: () => ipBans.value, ipBans }
    })
    await flush()
    expect(exposed.ipBans.value).toEqual([])
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(ElMessage.warning).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('非数组载荷归一空数组（后端异常形态防御）', async () => {
    ListIPBans.mockResolvedValue(null)
    const { wrapper, exposed } = mountWith(() => {
      const { ipBans } = useIPBanList()
      return { snapshot: () => ipBans.value, ipBans }
    })
    await flush()
    expect(exposed.ipBans.value).toEqual([])
    wrapper.unmount()
  })

  it('webserve-ip-banned 事件到达重拉列表（面板未开也刷新）', async () => {
    const { wrapper, exposed } = mountWith(() => {
      const { ipBans } = useIPBanList()
      return { snapshot: () => ipBans.value, ipBans }
    })
    await flush()
    expect(ListIPBans).toHaveBeenCalledTimes(1)
    ListIPBans.mockResolvedValue([banEntry({ ip: '198.51.100.9' })])
    eventHandlers[WEBSERVE_IP_BANNED_EVENT]({ ip: '198.51.100.9', failCount: 10 })
    await flush()
    expect(ListIPBans).toHaveBeenCalledTimes(2)
    expect(exposed.ipBans.value.map((b) => b.ip)).toEqual(['198.51.100.9'])
    wrapper.unmount()
  })

  it('卸载时经 EventsOn 返回闭包精准注销监听器', async () => {
    const { wrapper } = mountWith(() => {
      const { ipBans } = useIPBanList()
      return { snapshot: () => ipBans.value, ipBans }
    })
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeTypeOf('function')
    wrapper.unmount()
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeUndefined()
    expect(offSpy).toHaveBeenCalledTimes(1)
  })
})

describe('useIPBanSuggestions（连接管控九期网段聚合建议）', () => {
  beforeEach(() => {
    for (const k of Object.keys(eventHandlers)) delete eventHandlers[k]
    for (const k of Object.keys(eventListeners)) delete eventListeners[k]
    offSpy.mockClear()
    vi.clearAllMocks()
    ListIPBanSuggestions.mockResolvedValue([])
  })

  it('挂载即拉取建议并注册事件监听，数据落地 suggestions', async () => {
    ListIPBanSuggestions.mockResolvedValue([{ cidr: '203.0.113.0/24', count: 3 }])
    const { wrapper, exposed } = mountWith(() => {
      const { suggestions, reload } = useIPBanSuggestions()
      return { snapshot: () => suggestions.value, suggestions, reload }
    })
    await flush()
    expect(ListIPBanSuggestions).toHaveBeenCalledTimes(1)
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeTypeOf('function')
    expect(exposed.suggestions.value).toEqual([{ cidr: '203.0.113.0/24', count: 3 }])
    wrapper.unmount()
  })

  it('加载失败静默（保持空列表，不崩不弹错）', async () => {
    ListIPBanSuggestions.mockRejectedValue(new Error('service not ready'))
    const { wrapper, exposed } = mountWith(() => {
      const { suggestions } = useIPBanSuggestions()
      return { snapshot: () => suggestions.value, suggestions }
    })
    await flush()
    expect(exposed.suggestions.value).toEqual([])
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(ElMessage.warning).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('非数组载荷归一空数组（后端异常形态防御）', async () => {
    ListIPBanSuggestions.mockResolvedValue(null)
    const { wrapper, exposed } = mountWith(() => {
      const { suggestions } = useIPBanSuggestions()
      return { snapshot: () => suggestions.value, suggestions }
    })
    await flush()
    expect(exposed.suggestions.value).toEqual([])
    wrapper.unmount()
  })

  it('webserve-ip-banned 事件到达重拉建议（新自动封禁可能产出/改变建议）', async () => {
    const { wrapper, exposed } = mountWith(() => {
      const { suggestions } = useIPBanSuggestions()
      return { snapshot: () => suggestions.value, suggestions }
    })
    await flush()
    expect(ListIPBanSuggestions).toHaveBeenCalledTimes(1)
    ListIPBanSuggestions.mockResolvedValue([{ cidr: '198.51.100.0/24', count: 4 }])
    eventHandlers[WEBSERVE_IP_BANNED_EVENT]({ ip: '198.51.100.9', failCount: 10 })
    await flush()
    expect(ListIPBanSuggestions).toHaveBeenCalledTimes(2)
    expect(exposed.suggestions.value).toEqual([{ cidr: '198.51.100.0/24', count: 4 }])
    wrapper.unmount()
  })

  it('卸载时经 EventsOn 返回闭包精准注销监听器', async () => {
    const { wrapper } = mountWith(() => {
      const { suggestions } = useIPBanSuggestions()
      return { snapshot: () => suggestions.value, suggestions }
    })
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeTypeOf('function')
    wrapper.unmount()
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeUndefined()
    expect(offSpy).toHaveBeenCalledTimes(1)
  })
})

describe('useIPBanTrend（连接管控十期攻击趋势）', () => {
  // 趋势结果 fixture（形状对齐 model.IPBanTrendResult / IPBanTrendBucket）
  const trendResult = (over = {}) => ({
    windowDays: 7,
    earliestTs: 0,
    buckets: [
      { key: '203.0.113.0/24', days: [0, 1, 0, 3, 0, 2, 5], autoTotal: 11, manualCount: 2, rebounded: true, lastSeen: 1735689900 }
    ],
    ...over
  })

  beforeEach(() => {
    for (const k of Object.keys(eventHandlers)) delete eventHandlers[k]
    for (const k of Object.keys(eventListeners)) delete eventListeners[k]
    offSpy.mockClear()
    vi.clearAllMocks()
    QueryIPBanTrend.mockResolvedValue(trendResult({ buckets: [] }))
  })

  it('挂载即拉取趋势并注册事件监听，结果透传落地 trend', async () => {
    QueryIPBanTrend.mockResolvedValue(trendResult())
    const { wrapper, exposed } = mountWith(() => {
      const { trend, reload } = useIPBanTrend()
      return { snapshot: () => trend.value, trend, reload }
    })
    await flush()
    expect(QueryIPBanTrend).toHaveBeenCalledTimes(1)
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeTypeOf('function')
    expect(exposed.trend.value.windowDays).toBe(7)
    expect(exposed.trend.value.earliestTs).toBe(0)
    expect(exposed.trend.value.buckets).toHaveLength(1)
    // 桶字段整条透传（days 序列 / manualCount / rebounded 供 ActivityBar 徽标消费）
    expect(exposed.trend.value.buckets[0].key).toBe('203.0.113.0/24')
    expect(exposed.trend.value.buckets[0].days).toEqual([0, 1, 0, 3, 0, 2, 5])
    expect(exposed.trend.value.buckets[0].rebounded).toBe(true)
    wrapper.unmount()
  })

  it('加载失败静默（保持空形态，不崩不弹错）——审计旁路面非正确性依赖', async () => {
    QueryIPBanTrend.mockRejectedValue(new Error('audit read failed'))
    const { wrapper, exposed } = mountWith(() => {
      const { trend } = useIPBanTrend()
      return { snapshot: () => trend.value, trend }
    })
    await flush()
    expect(exposed.trend.value.buckets).toEqual([])
    expect(exposed.trend.value.windowDays).toBe(0)
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(ElMessage.warning).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('非法载荷防御归一（buckets 非数组归空、windowDays/earliestTs 非数字归 0）', async () => {
    QueryIPBanTrend.mockResolvedValue({ windowDays: '7', earliestTs: null, buckets: 'oops' })
    const { wrapper, exposed } = mountWith(() => {
      const { trend } = useIPBanTrend()
      return { snapshot: () => trend.value, trend }
    })
    await flush()
    expect(exposed.trend.value.windowDays).toBe(0)
    expect(exposed.trend.value.earliestTs).toBe(0)
    expect(exposed.trend.value.buckets).toEqual([])
    wrapper.unmount()
  })

  it('非对象载荷（null）保持上次值不覆盖', async () => {
    const { wrapper, exposed } = mountWith(() => {
      const { trend, reload } = useIPBanTrend()
      return { snapshot: () => trend.value, trend, reload }
    })
    await flush()
    QueryIPBanTrend.mockResolvedValue(null)
    await exposed.reload()
    await flush()
    // null 不覆盖已加载形态（防御后端异常形态静默清空展示面）
    expect(exposed.trend.value.windowDays).toBe(7)
    expect(exposed.trend.value.buckets).toEqual([])
    wrapper.unmount()
  })

  it('webserve-ip-banned 事件到达重拉趋势（新自动封禁改变计数与 rebounded）', async () => {
    const { wrapper, exposed } = mountWith(() => {
      const { trend } = useIPBanTrend()
      return { snapshot: () => trend.value, trend }
    })
    await flush()
    expect(QueryIPBanTrend).toHaveBeenCalledTimes(1)
    QueryIPBanTrend.mockResolvedValue(trendResult({ earliestTs: 1735689600 }))
    eventHandlers[WEBSERVE_IP_BANNED_EVENT]({ ip: '198.51.100.9', failCount: 10 })
    await flush()
    expect(QueryIPBanTrend).toHaveBeenCalledTimes(2)
    expect(exposed.trend.value.earliestTs).toBe(1735689600)
    wrapper.unmount()
  })

  it('卸载时经 EventsOn 返回闭包精准注销监听器', async () => {
    const { wrapper } = mountWith(() => {
      const { trend } = useIPBanTrend()
      return { snapshot: () => trend.value, trend }
    })
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeTypeOf('function')
    wrapper.unmount()
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeUndefined()
    expect(offSpy).toHaveBeenCalledTimes(1)
  })

  it('浏览器通道（isBrowserChannel）跳过加载（隐私红线——RPCHiddenMethods 屏蔽面调用必败，隐藏入口而非点击报错）', async () => {
    // 组件在模块求值期捕获通道归属，须重置模块注册表后以 true 形态重新求值；
    // doMock 不提升、仅作用于本次动态导入（SettingsPanel.spec 审计入口先例同款）
    vi.resetModules()
    vi.doMock('../../transport', () => ({ isBrowserChannel: () => true }))
    try {
      const { useIPBanTrend: BrowserTrend } = await import('../useIPBans')
      const { QueryIPBanTrend: FreshQuery } = await import('../../../wailsjs/go/main/App')
      FreshQuery.mockResolvedValue(trendResult())
      const { wrapper, exposed } = mountWith(() => {
        const { trend } = BrowserTrend()
        return { snapshot: () => trend.value, trend }
      })
      await flush()
      expect(FreshQuery).not.toHaveBeenCalled()
      expect(exposed.trend.value.buckets).toEqual([])
      wrapper.unmount()
    } finally {
      vi.doUnmock('../../transport')
      vi.resetModules()
    }
  })
})

describe('useIPBanToast', () => {
  beforeEach(() => {
    for (const k of Object.keys(eventHandlers)) delete eventHandlers[k]
    for (const k of Object.keys(eventListeners)) delete eventListeners[k]
    offSpy.mockClear()
    vi.clearAllMocks()
  })

  it('事件到达弹 warning 提示，文案含来源 IP 与到期时间', async () => {
    const { wrapper } = mountWith(() => {
      useIPBanToast()
      return { snapshot: () => '' }
    })
    eventHandlers[WEBSERVE_IP_BANNED_EVENT](banEntry())
    await flush()
    expect(ElMessage.warning).toHaveBeenCalledTimes(1)
    const msg = ElMessage.warning.mock.calls[0][0]
    expect(msg).toContain('203.0.113.7')
    expect(msg).toContain('已临时封禁至')
    // 到期时间为 M-D HH:mm 紧凑形态（非 '--' 降级）
    expect(msg).not.toContain('--')
    wrapper.unmount()
  })

  it('重复事件逐条弹提示（连续多 IP 封禁不吞并）', async () => {
    const { wrapper } = mountWith(() => {
      useIPBanToast()
      return { snapshot: () => '' }
    })
    eventHandlers[WEBSERVE_IP_BANNED_EVENT](banEntry())
    eventHandlers[WEBSERVE_IP_BANNED_EVENT](banEntry({ ip: '198.51.100.9' }))
    await flush()
    expect(ElMessage.warning).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('非法载荷忽略（ip 非字符串/空串/expiresAt 非有限数字）', async () => {
    const { wrapper } = mountWith(() => {
      useIPBanToast()
      return { snapshot: () => '' }
    })
    eventHandlers[WEBSERVE_IP_BANNED_EVENT](null)
    eventHandlers[WEBSERVE_IP_BANNED_EVENT]({ ip: '', expiresAt: 1 })
    eventHandlers[WEBSERVE_IP_BANNED_EVENT]({ ip: '1.2.3.4', expiresAt: 'soon' })
    await flush()
    expect(ElMessage.warning).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('卸载时经 EventsOn 返回闭包精准注销监听器', async () => {
    const { wrapper } = mountWith(() => {
      useIPBanToast()
      return { snapshot: () => '' }
    })
    wrapper.unmount()
    expect(eventHandlers[WEBSERVE_IP_BANNED_EVENT]).toBeUndefined()
  })
})

describe('formatBanExpiry', () => {
  it('unix 秒格式化为 M-D HH:mm', () => {
    // 固定时间戳按本地时区格式化，只断言形态（月-日 时:分），不做时区耦合断言
    expect(formatBanExpiry(1735689900)).toMatch(/^\d{1,2}-\d{1,2} \d{2}:\d{2}$/)
  })

  it('非法值（0/undefined/NaN）降级 --', () => {
    expect(formatBanExpiry(0)).toBe('--')
    expect(formatBanExpiry(undefined)).toBe('--')
    expect(formatBanExpiry(NaN)).toBe('--')
  })
})

/** 微任务收敛（onMounted 异步加载与事件回调 flush） */
async function flush() {
  await Promise.resolve()
  await Promise.resolve()
  await new Promise((r) => setTimeout(r, 0))
}
