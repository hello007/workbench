import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, defineComponent } from 'vue'
import { useIPBanList, useIPBanToast, formatBanExpiry, WEBSERVE_IP_BANNED_EVENT } from '../useIPBans'

// Wails 事件 handler 注册表（EventsOn mock 捕获）：返回注销闭包记录调用，
// 供卸载精准注销断言（cross-layer-contracts 闭包注销契约）。
const eventHandlers = {}
const offSpy = vi.fn()

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((name, cb) => {
    eventHandlers[name] = cb
    return () => {
      delete eventHandlers[name]
      offSpy()
    }
  })
}))

vi.mock('../../../wailsjs/go/main/App', () => ({
  ListIPBans: vi.fn()
}))

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }
}))

import { ListIPBans } from '../../../wailsjs/go/main/App'
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

describe('useIPBanToast', () => {
  beforeEach(() => {
    for (const k of Object.keys(eventHandlers)) delete eventHandlers[k]
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
