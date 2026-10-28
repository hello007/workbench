import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import ActivityBar from '../ActivityBar.vue'
import { useUiStore } from '../../store'

// Wails 事件 handler 注册表（EventsOn mock 捕获）：多设备连接数事件驱动断言用。
// 九期起 useIPBanList 与 useIPBanSuggestions 共听 webserve-ip-banned（真实
// runtime 支持同事件多监听器），mock 升级为按名收集监听器列表 + 分发函数——
// eventHandlers[name] 恒为「派发一次触达全部监听器」的可调用形态（既有断言
// 形态兼容），注销闭包按引用精准移除本监听器，最后一个移除时清空注册项
const eventHandlers = {}
const eventListeners = {}

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
      if (!list || list.length === 0) {
        delete eventListeners[name]
        delete eventHandlers[name]
      }
    }
  })
}))

// 踢出链依赖（连接管控任务）+ IP 自动封禁链（连接管控五期）：App 绑定 +
// ElMessageBox/ElMessage（确认弹窗与 handleError 分流断言；文件级工厂，对齐
// confirm-dialog vitest mock 形态）
vi.mock('../../../wailsjs/go/main/App', () => ({
  KickConnection: vi.fn(),
  ListIPBans: vi.fn(),
  ListIPBanSuggestions: vi.fn(),
  QueryIPBanTrend: vi.fn(),
  UnbanIP: vi.fn(),
  UnbanCIDR: vi.fn()
}))

vi.mock('element-plus', () => ({
  ElMessageBox: { confirm: vi.fn() },
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn() }
}))

// 跨用例重置监听器注册表：未显式 unmount 的历史 wrapper 实例不向后续用例泄漏
// 监听器（真实 runtime 随组件卸载注销，测试侧以注册表重置等价兜底——多监听器
// 分发下残留监听器会让事件派发触达陈旧实例，放大 mock 调用计数）
beforeEach(() => {
  for (const k of Object.keys(eventListeners)) delete eventListeners[k]
  for (const k of Object.keys(eventHandlers)) delete eventHandlers[k]
})

// 封禁弹窗桩（连接管控四期，明细行封禁入口）：本文件只断言接线（行内按钮打开
// 弹窗、目标为行 tokenId），弹窗内部表单与提交链由 BanTokenDialog 专属 spec 覆盖
vi.mock('../BanTokenDialog.vue', () => ({
  default: {
    name: 'BanTokenDialog',
    props: ['modelValue', 'token', 'connections'],
    emits: ['update:modelValue', 'banned'],
    template: '<div class="ban-token-dialog-stub" />'
  }
}))

// 手动封禁 IP 弹窗桩（连接管控七期，区块头部入口；九期扩聚合建议行预填入口）：
// 同上只测接线（头部按钮/建议行打开弹窗、initialCidr 预填透传），弹窗内部表单
// 与提交链由 BanIPDialog 专属 spec 覆盖
vi.mock('../BanIPDialog.vue', () => ({
  default: {
    name: 'BanIPDialog',
    props: ['modelValue', 'initialCidr'],
    emits: ['update:modelValue', 'banned'],
    template: '<div class="ban-ip-dialog-stub" />'
  }
}))

describe('ActivityBar', () => {
  // activePanel / terminalActive 已迁 ui store：mount 前在 uiStore 上设初值，
  // 组件内直读 uiStore.activePanel / uiStore.terminalVisible；
  // 点击面板项直写 store（不再 emit update:modelValue），设置/终端仍 emit 由 Home 处理。
  // 图标顺序：directory(0) / ai(1) / ai-chat(2) / toolbox(3) / assets(4) / dashboard(5)
  //           / stats(6) / 设置(7) / 终端(8)
  const createWrapper = (activePanel = 'directory') => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.activePanel = activePanel
    return mount(ActivityBar, {
      global: {
        plugins: [pinia],
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-tooltip': { template: '<div><slot /></div>', props: ['content', 'placement'] }
        }
      }
    })
  }

  it('应该渲染九个活动栏图标按钮（工作目录、AI 功能、AI 对话、工具箱、素材管理、状态看板、仓库统计、设置、终端）', () => {
    const wrapper = createWrapper()
    const items = wrapper.findAll('.activity-bar-item')
    expect(items.length).toBe(9)
  })

  it('默认选中工作目录', () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).toContain('is-active')
    expect(items[1].classes()).not.toContain('is-active')
    expect(items[2].classes()).not.toContain('is-active')
  })

  it('选中状态看板时高亮对应图标', () => {
    const wrapper = createWrapper('dashboard')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).not.toContain('is-active')
    // dashboard 是第6个图标（index=5）
    expect(items[5].classes()).toContain('is-active')
    expect(items[1].classes()).not.toContain('is-active')
  })

  it('选中 AI 功能时高亮对应图标', () => {
    const wrapper = createWrapper('ai')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).not.toContain('is-active')
    expect(items[3].classes()).not.toContain('is-active')
    // ai 是第2个图标（index=1）
    expect(items[1].classes()).toContain('is-active')
  })

  it('选中 AI 对话时高亮对应图标', () => {
    const wrapper = createWrapper('ai-chat')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).not.toContain('is-active')
    expect(items[1].classes()).not.toContain('is-active')
    expect(items[3].classes()).not.toContain('is-active')
    // ai-chat 是第3个图标（index=2）
    expect(items[2].classes()).toContain('is-active')
  })

  it('选中素材管理时高亮对应图标', () => {
    const wrapper = createWrapper('assets')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).not.toContain('is-active')
    // assets 是第5个图标（index=4，工具箱之后）
    expect(items[4].classes()).toContain('is-active')
    expect(items[3].classes()).not.toContain('is-active')
    expect(items[5].classes()).not.toContain('is-active')
  })

  it('选中仓库统计时高亮对应图标', () => {
    const wrapper = createWrapper('stats')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[1].classes()).not.toContain('is-active')
    // stats 是第7个图标（index=6）
    expect(items[6].classes()).toContain('is-active')
    expect(items[2].classes()).not.toContain('is-active')
  })

  it('选中工具箱时高亮对应图标', () => {
    const wrapper = createWrapper('toolbox')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).not.toContain('is-active')
    expect(items[1].classes()).not.toContain('is-active')
    expect(items[4].classes()).not.toContain('is-active')
    expect(items[5].classes()).not.toContain('is-active')
    // toolbox 是第4个图标（index=3）
    expect(items[3].classes()).toContain('is-active')
  })

  it('点击工作目录图标应将 activePanel 置为 directory', async () => {
    const wrapper = createWrapper('toolbox')
    const items = wrapper.findAll('.activity-bar-item')
    await items[0].trigger('click')
    expect(useUiStore().activePanel).toBe('directory')
  })

  it('点击素材管理图标应将 activePanel 置为 assets', async () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    // assets 是第5个图标（index=4）
    await items[4].trigger('click')
    expect(useUiStore().activePanel).toBe('assets')
  })

  it('点击状态看板图标应将 activePanel 置为 dashboard', async () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    // dashboard 是第6个图标（index=5）
    await items[5].trigger('click')
    expect(useUiStore().activePanel).toBe('dashboard')
  })

  it('点击 AI 功能图标应将 activePanel 置为 ai', async () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    // ai 是第2个图标（index=1）
    await items[1].trigger('click')
    expect(useUiStore().activePanel).toBe('ai')
  })

  it('点击 AI 对话图标应将 activePanel 置为 ai-chat', async () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    // ai-chat 是第3个图标（index=2）
    await items[2].trigger('click')
    expect(useUiStore().activePanel).toBe('ai-chat')
  })

  it('点击仓库统计图标应将 activePanel 置为 stats', async () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    // stats 是第7个图标（index=6）
    await items[6].trigger('click')
    expect(useUiStore().activePanel).toBe('stats')
  })

  it('点击工具箱图标应将 activePanel 置为 toolbox', async () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    // toolbox 是第4个图标（index=3）
    await items[3].trigger('click')
    expect(useUiStore().activePanel).toBe('toolbox')
  })

  it('点击设置图标应触发 openSettings 事件', async () => {
    const wrapper = createWrapper()
    const items = wrapper.findAll('.activity-bar-item')
    // settings 是第8个图标（index=7）
    await items[7].trigger('click')
    expect(wrapper.emitted('openSettings')).toBeTruthy()
  })
})

// ---- 多设备连接指示（PR3 语义明示）：连接数事件驱动显隐 ----

describe('ActivityBar - 多设备连接指示', () => {
  const createWrapper = (activePanel = 'directory') => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.activePanel = activePanel
    return mount(ActivityBar, {
      global: {
        plugins: [pinia],
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-tooltip': { template: '<div><span class="tooltip-content">{{ content }}</span><slot /></div>', props: ['content', 'placement'] },
          // 明细 popover 桩（参照 AssetsPanel 模式）：双 slot 全渲染（免触发机制直测面板内容）
          'el-popover': {
            template: '<div class="el-popover-stub"><slot name="reference" /><div class="popover-content"><slot /></div></div>',
            props: ['placement', 'width', 'trigger', 'persistent']
          }
        }
      }
    })
  }

  it('连接数 <=1 时指示不显示（桌面模式无事件恒此形态，空安全）', () => {
    const wrapper = createWrapper()
    expect(wrapper.find('.activity-bar-devices').exists()).toBe(false)
    // 图标数仍为 9（指示项不占位）
    expect(wrapper.findAll('.activity-bar-item').length).toBe(9)
    wrapper.unmount()
  })

  it('ws-connections:changed 事件连接数 >1 时显示指示与计数角标', async () => {
    const wrapper = createWrapper()
    eventHandlers['ws-connections:changed'](3)
    await nextTick()
    expect(wrapper.find('.activity-bar-devices').exists()).toBe(true)
    expect(wrapper.text()).toContain('3 个设备已连接')
    // 角标显示连接数
    expect(wrapper.find('.devices-badge').text()).toBe('3')
    wrapper.unmount()
  })

  it('连接数回落 1（其他设备全部断开）时指示隐藏', async () => {
    const wrapper = createWrapper()
    eventHandlers['ws-connections:changed'](2)
    await nextTick()
    expect(wrapper.find('.activity-bar-devices').exists()).toBe(true)
    eventHandlers['ws-connections:changed'](1)
    await nextTick()
    expect(wrapper.find('.activity-bar-devices').exists()).toBe(false)
    wrapper.unmount()
  })

  it('卸载时经 EventsOn 返回闭包精准注销监听器', () => {
    const wrapper = createWrapper()
    expect(eventHandlers['ws-connections:changed']).toBeTypeOf('function')
    wrapper.unmount()
    expect(eventHandlers['ws-connections:changed']).toBeUndefined()
  })

  it('桌面通道明细面板：三列明细（IP/token 名/接入时间），空 tokenName 降级「已撤销令牌」，非法时间显「--」', async () => {
    const wrapper = createWrapper()
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() },
      { id: 2, ip: '10.0.0.8', tokenId: 't2', tokenName: '', connectedAt: new Date().toISOString() },
      { id: 3, ip: '172.16.0.2', tokenId: 't3', tokenName: '平板', connectedAt: 'invalid' }
    ])
    await nextTick()
    expect(wrapper.find('.devices-panel').exists()).toBe(true)
    expect(wrapper.find('.devices-panel-title').text()).toBe('已连接设备（3）')
    const rows = wrapper.findAll('.devices-panel-row')
    expect(rows.length).toBe(3)
    expect(rows[0].find('.devices-panel-ip').text()).toBe('192.168.1.5')
    expect(rows[0].find('.devices-panel-token').text()).toBe('手机')
    // 同日接入时间只显 HH:mm:ss
    expect(rows[0].find('.devices-panel-time').text()).toMatch(/^\d{1,2}:\d{2}:\d{2}$/)
    // tokenName 空串（令牌已撤销）降级展示
    expect(rows[1].find('.devices-panel-token').text()).toBe('已撤销令牌')
    // 非法接入时间降级 '--'
    expect(rows[2].find('.devices-panel-time').text()).toBe('--')
    // 底部固定共享语义提示
    expect(wrapper.find('.devices-panel-hint').text()).toBe('多端共享同一后端状态，后操作覆盖先操作')
    wrapper.unmount()
  })

  it('浏览器通道明细面板显示隐私引导文案（他人 IP/token 明细不进浏览器通道）', async () => {
    const wrapper = createWrapper()
    eventHandlers['ws-connections:changed'](3)
    await nextTick()
    expect(wrapper.find('.devices-panel').exists()).toBe(true)
    expect(wrapper.find('.devices-panel-empty').text()).toBe('明细仅在桌面端显示')
    // 浏览器通道无明细数据，列表不渲染
    expect(wrapper.find('.devices-panel-list').exists()).toBe(false)
    wrapper.unmount()
  })
})

// ---- 明细面板踢出交互（连接管控：断开单个连接，非封禁）----

describe('ActivityBar - 明细面板踢出', () => {
  const createWrapper = () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.activePanel = 'directory'
    return mount(ActivityBar, {
      global: {
        plugins: [pinia],
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-tooltip': { template: '<div><span class="tooltip-content">{{ content }}</span><slot /></div>', props: ['content', 'placement'] },
          'el-popover': {
            template: '<div class="el-popover-stub"><slot name="reference" /><div class="popover-content"><slot /></div></div>',
            props: ['placement', 'width', 'trigger', 'persistent']
          }
        }
      }
    })
  }

  /** 注入桌面通道两条连接明细并返回挂载后的 wrapper（行内各带踢出按钮） */
  const mountWithConnections = async () => {
    const wrapper = createWrapper()
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() },
      { id: 2, ip: '10.0.0.8', tokenId: 't2', tokenName: '', connectedAt: new Date().toISOString() }
    ])
    await nextTick()
    return wrapper
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('每行渲染踢出按钮；点击弹单次确认（含可重连/撤令牌互补警示），确认后调 KickConnection(连接 ID)', async () => {
    const { KickConnection } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox } = await import('element-plus')
    ElMessageBox.confirm.mockResolvedValue()
    KickConnection.mockResolvedValue()
    const wrapper = await mountWithConnections()
    const kicks = wrapper.findAll('.devices-panel-kick')
    expect(kicks.length).toBe(2)
    await kicks[0].trigger('click')
    await flushConfirm()
    // 单次确认（confirm-dialog 单次确认形态），警示文案含两手段互补语义
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    const [message, title] = ElMessageBox.confirm.mock.calls[0]
    expect(title).toBe('踢出连接')
    expect(message).toContain('192.168.1.5')
    expect(message).toContain('自动重连')
    expect(message).toContain('撤销')
    expect(KickConnection).toHaveBeenCalledTimes(1)
    expect(KickConnection).toHaveBeenCalledWith(1)
    // 行消失靠明细事件自动更新，不手动删（此处仅断言调用面）
    wrapper.unmount()
  })

  it('确认取消则不调用 KickConnection', async () => {
    const { KickConnection } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox } = await import('element-plus')
    ElMessageBox.confirm.mockRejectedValue('cancel')
    const wrapper = await mountWithConnections()
    await wrapper.findAll('.devices-panel-kick')[0].trigger('click')
    await flushConfirm()
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    expect(KickConnection).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('踢出失败走 handleError 分流（E_CONNECTION_NOT_FOUND 预期拒绝 warning，不带前缀）', async () => {
    const { KickConnection } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox, ElMessage } = await import('element-plus')
    ElMessageBox.confirm.mockResolvedValue()
    KickConnection.mockRejectedValue({ code: 'E_CONNECTION_NOT_FOUND', message: '连接不存在或已断开（列表已过期，请刷新后重试）' })
    const wrapper = await mountWithConnections()
    await wrapper.findAll('.devices-panel-kick')[1].trigger('click')
    await flushConfirm()
    expect(KickConnection).toHaveBeenCalledWith(2)
    expect(ElMessage.warning).toHaveBeenCalledWith('连接不存在或已断开（列表已过期，请刷新后重试）')
    expect(ElMessage.error).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('踢出真失败（非预期拒绝码）走 error 提示并拼前缀', async () => {
    const { KickConnection } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox, ElMessage } = await import('element-plus')
    ElMessageBox.confirm.mockResolvedValue()
    // 非预期拒绝码（E_RPC_INTERNAL 不在 WARNING_CODES）：error 提示拼前缀
    KickConnection.mockRejectedValue({ code: 'E_RPC_INTERNAL', message: '内部错误' })
    const wrapper = await mountWithConnections()
    await wrapper.findAll('.devices-panel-kick')[0].trigger('click')
    await flushConfirm()
    expect(ElMessage.error).toHaveBeenCalledWith('踢出失败: 内部错误')
    expect(ElMessage.warning).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  /** 等待确认链微任务收敛（confirm resolve/reject 与后续调用的 flush） */
  async function flushConfirm() {
    await nextTick()
    await Promise.resolve()
    await nextTick()
  }
})

// ---- 明细面板封禁入口（连接管控四期：行内封禁按钮按 tokenID 触发共享弹窗）----

describe('ActivityBar - 明细面板封禁入口', () => {
  const createWrapper = () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.activePanel = 'directory'
    return mount(ActivityBar, {
      global: {
        plugins: [pinia],
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-tooltip': { template: '<div><span class="tooltip-content">{{ content }}</span><slot /></div>', props: ['content', 'placement'] },
          'el-popover': {
            template: '<div class="el-popover-stub"><slot name="reference" /><div class="popover-content"><slot /></div></div>',
            props: ['placement', 'width', 'trigger', 'persistent']
          }
        }
      }
    })
  }

  /** 注入桌面通道两条连接明细并返回挂载后的 wrapper（行内各带封禁按钮） */
  const mountWithConnections = async () => {
    const wrapper = createWrapper()
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() },
      { id: 2, ip: '10.0.0.8', tokenId: 't2', tokenName: '', connectedAt: new Date().toISOString() }
    ])
    await nextTick()
    return wrapper
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('每行渲染封禁按钮（与踢出并列）；点击打开共享弹窗，目标为行 tokenId（非单连接）', async () => {
    const wrapper = await mountWithConnections()
    const bans = wrapper.findAll('.devices-panel-ban')
    expect(bans.length).toBe(2)
    await bans[0].trigger('click')
    await nextTick()
    const dialog = wrapper.findComponent({ name: 'BanTokenDialog' })
    expect(dialog.props('modelValue')).toBe(true)
    // 封禁为 tokenID 维度：目标取行 tokenId 与令牌名（非连接 ID）
    expect(dialog.props('token')).toEqual({ id: 't1', name: '手机' })
    // 连接明细快照透传（弹窗内勾选联动踢出时按 tokenID 过滤消费）
    expect(dialog.props('connections')).toHaveLength(2)
    wrapper.unmount()
  })

  it('令牌名空串（已撤销令牌连接）降级以空名打开弹窗（弹窗内按 ID 展示）', async () => {
    const wrapper = await mountWithConnections()
    await wrapper.findAll('.devices-panel-ban')[1].trigger('click')
    await nextTick()
    const dialog = wrapper.findComponent({ name: 'BanTokenDialog' })
    expect(dialog.props('token')).toEqual({ id: 't2', name: '' })
    wrapper.unmount()
  })
})

// ---- 自动封禁区块（连接管控五期：认证失败达阈值自动封禁来源 IP，桌面专用
// 数据源 ListIPBans + webserve-ip-banned 事件驱动刷新 + 解封单次确认链）----

describe('ActivityBar - 自动封禁区块', () => {
  const createWrapper = () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.activePanel = 'directory'
    return mount(ActivityBar, {
      global: {
        plugins: [pinia],
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-tooltip': { template: '<div><span class="tooltip-content">{{ content }}</span><slot /></div>', props: ['content', 'placement'] },
          'el-popover': {
            template: '<div class="el-popover-stub"><slot name="reference" /><div class="popover-content"><slot /></div></div>',
            props: ['placement', 'width', 'trigger', 'persistent']
          }
        }
      }
    })
  }

  /** 封禁条目 fixture（形状对齐 model.IPBanStatus，expiresAt 默认 5 分钟后） */
  const banEntry = (over = {}) => ({
    ip: '203.0.113.7',
    bannedAt: Math.floor(Date.now() / 1000) - 60,
    expiresAt: Math.floor(Date.now() / 1000) + 300,
    failCount: 10,
    expired: false,
    source: 'auto',
    ...over
  })

  /** 挂载并 flush 挂载期 ListIPBans 异步加载 */
  const mountFlushed = async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await nextTick()
    return wrapper
  }

  /** 等待确认链微任务收敛（confirm resolve/reject 与后续调用的 flush） */
  const flushConfirm = async () => {
    await nextTick()
    await Promise.resolve()
    await nextTick()
  }

  beforeEach(async () => {
    vi.clearAllMocks()
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([])
  })

  it('挂载即拉取列表，区块渲染行（IP/失败计数/到期时间/解封按钮）', async () => {
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([banEntry()])
    const wrapper = await mountFlushed()
    expect(ListIPBans).toHaveBeenCalled()
    expect(wrapper.find('.devices-ban-section').exists()).toBe(true)
    const rows = wrapper.findAll('.devices-ban-row')
    expect(rows.length).toBe(1)
    expect(rows[0].find('.devices-panel-ip').text()).toBe('203.0.113.7')
    expect(rows[0].text()).toContain('10 次失败')
    expect(rows[0].text()).toMatch(/至 \d{1,2}-\d{1,2} \d{2}:\d{2}/)
    expect(rows[0].find('.devices-ban-unban').exists()).toBe(true)
    wrapper.unmount()
  })

  it('面板因连接可见而封禁列表为空时，空态显「暂无封禁记录」', async () => {
    const wrapper = await mountFlushed()
    // 面板可见条件 = 连接存在或封禁存在；注入一条桌面通道连接使面板可见
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() }
    ])
    await nextTick()
    expect(wrapper.find('.devices-ban-section').exists()).toBe(true)
    expect(wrapper.findAll('.devices-ban-row').length).toBe(0)
    expect(wrapper.find('.devices-ban-empty').text()).toBe('暂无封禁记录')
    wrapper.unmount()
  })

  it('来源徽标分流（七期）：手动条目显「手动」徽标隐藏失败计数，永久条目显「永久」；自动条目 strike>1 显递增档位', async () => {
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([
      banEntry({ source: 'manual', failCount: 0, expiresAt: 0 }),
      banEntry({ ip: '198.51.100.8', source: 'auto', strike: 3 }),
      banEntry({ ip: '198.51.100.9', source: 'auto', strike: 1 })
    ])
    const wrapper = await mountFlushed()
    const rows = wrapper.findAll('.devices-ban-row')
    // 手动：「手动」徽标 + 无失败计数列 + 永久文案
    expect(rows[0].find('.devices-ban-source').exists()).toBe(true)
    expect(rows[0].find('.devices-ban-source').text()).toBe('手动')
    expect(rows[0].text()).not.toContain('次失败')
    expect(rows[0].find('.devices-ban-expiry').text()).toBe('永久')
    // 自动 strike>1：递增档位徽标，无来源徽标
    expect(rows[1].find('.devices-ban-source').exists()).toBe(false)
    expect(rows[1].find('.devices-ban-strike').exists()).toBe(true)
    expect(rows[1].find('.devices-ban-strike').text()).toContain('第 3 次')
    // 自动首触（strike=1）：无任何徽标（六期向后兼容）
    expect(rows[2].find('.devices-ban-strike').exists()).toBe(false)
    wrapper.unmount()
  })

  it('头部「封禁 IP」按钮打开手动封禁弹窗（七期接线）：桌面事件锁通道后渲染，点击置 v-model', async () => {
    const wrapper = await mountFlushed()
    // isDesktopChannel 由桌面事件锁定（初始 false = 浏览器通道形态）：浏览器通道
    // 不渲染入口（ListIPBans 屏蔽面同源）
    expect(wrapper.find('.devices-ban-add').exists(), '浏览器通道不应渲染入口').toBe(false)
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() }
    ])
    await nextTick()
    expect(wrapper.find('.devices-ban-add').exists(), '桌面通道应渲染入口').toBe(true)
    await wrapper.find('.devices-ban-add').trigger('click')
    await nextTick()
    const dialog = wrapper.findComponent({ name: 'BanIPDialog' })
    expect(dialog.exists(), 'BanIPDialog 桩应挂载').toBe(true)
    expect(dialog.props('modelValue'), '点击后 v-model 置 true').toBe(true)
    wrapper.unmount()
  })

  it('过期态标注「已过期」（info 灰样式），生效中行走 warning 样式', async () => {
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([
      banEntry(),
      banEntry({ ip: '198.51.100.9', expired: true })
    ])
    const wrapper = await mountFlushed()
    const rows = wrapper.findAll('.devices-ban-row')
    expect(rows[0].find('.devices-ban-expiry').classes()).not.toContain('is-expired')
    expect(rows[0].text()).not.toContain('已过期')
    expect(rows[1].find('.devices-ban-expiry').classes()).toContain('is-expired')
    expect(rows[1].text()).toContain('已过期')
    // 过期行解封按钮保留（UnbanIP 幂等 no-op，且可顺带清理残留记录）
    expect(rows[1].find('.devices-ban-unban').exists()).toBe(true)
    wrapper.unmount()
  })

  it('解封确认链：单次确认（文案含 IP 与再封禁警示）→ UnbanIP(ip) → 成功 toast + 重拉列表', async () => {
    const { UnbanIP, ListIPBans } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox, ElMessage } = await import('element-plus')
    ElMessageBox.confirm.mockResolvedValue()
    UnbanIP.mockResolvedValue()
    ListIPBans.mockResolvedValue([banEntry()])
    const wrapper = await mountFlushed()
    await wrapper.find('.devices-ban-unban').trigger('click')
    await flushConfirm()
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    const [message, title] = ElMessageBox.confirm.mock.calls[0]
    expect(title).toBe('解封 IP')
    expect(message).toContain('203.0.113.7')
    expect(message).toContain('再次被自动封禁')
    expect(UnbanIP).toHaveBeenCalledTimes(1)
    expect(UnbanIP).toHaveBeenCalledWith('203.0.113.7')
    expect(ElMessage.success).toHaveBeenCalledWith('已解封 203.0.113.7')
    // 解封成功后重拉列表（ListIPBans 二次调用 = 挂载 1 次 + reload 1 次）
    expect(ListIPBans).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('确认取消则不调用 UnbanIP', async () => {
    const { UnbanIP } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox } = await import('element-plus')
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([banEntry()])
    ElMessageBox.confirm.mockRejectedValue('cancel')
    const wrapper = await mountFlushed()
    await wrapper.find('.devices-ban-unban').trigger('click')
    await flushConfirm()
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    expect(UnbanIP).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('解封失败走 handleError 分流（非预期拒绝码 error 提示拼前缀）', async () => {
    const { UnbanIP } = await import('../../../wailsjs/go/main/App')
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox, ElMessage } = await import('element-plus')
    ListIPBans.mockResolvedValue([banEntry()])
    ElMessageBox.confirm.mockResolvedValue()
    UnbanIP.mockRejectedValue({ message: 'IP 封禁服务未装配' })
    const wrapper = await mountFlushed()
    await wrapper.find('.devices-ban-unban').trigger('click')
    await flushConfirm()
    expect(ElMessage.error).toHaveBeenCalledWith('解封失败: IP 封禁服务未装配')
    expect(ElMessage.success).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('webserve-ip-banned 事件到达重拉列表（事件驱动刷新）', async () => {
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    const wrapper = await mountFlushed()
    expect(ListIPBans).toHaveBeenCalledTimes(1)
    ListIPBans.mockResolvedValue([banEntry({ ip: '192.0.2.66' })])
    eventHandlers['webserve-ip-banned']({ ip: '192.0.2.66', failCount: 10 })
    await flushPromises()
    await nextTick()
    expect(ListIPBans).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('192.0.2.66')
    wrapper.unmount()
  })

  it('卸载时经 EventsOn 返回闭包精准注销监听器', async () => {
    const wrapper = await mountFlushed()
    expect(eventHandlers['webserve-ip-banned']).toBeTypeOf('function')
    wrapper.unmount()
    expect(eventHandlers['webserve-ip-banned']).toBeUndefined()
  })

  it('无连接但有封禁记录时指示显示：角标显封禁数（warning 色）+ 提示文案分流', async () => {
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([banEntry(), banEntry({ ip: '198.51.100.9' })])
    const wrapper = await mountFlushed()
    expect(wrapper.find('.activity-bar-devices').exists()).toBe(true)
    expect(wrapper.find('.devices-badge').text()).toBe('2')
    expect(wrapper.find('.devices-badge').classes()).toContain('devices-badge-warn')
    // tooltip stub 对每个 item 均渲染 .tooltip-content，取 devices 项（some 断言）
    const hints = wrapper.findAll('.tooltip-content').map((t) => t.text())
    expect(hints.some((t) => t.includes('2 个来源 IP 已被自动封禁'))).toBe(true)
    wrapper.unmount()
  })

  it('递增档位徽标（六期）：strike>1 渲染「第 N 次」，首触与旧载荷不渲染', async () => {
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([
      banEntry({ ip: '203.0.113.8', strike: 3 }),
      banEntry({ ip: '203.0.113.9', strike: 1 }),
      banEntry({ ip: '203.0.113.10' }) // 旧载荷形态（无 strike 字段）向后兼容
    ])
    const wrapper = await mountFlushed()
    const rows = wrapper.findAll('.devices-ban-row')
    expect(rows[0].find('.devices-ban-strike').exists()).toBe(true)
    expect(rows[0].find('.devices-ban-strike').text()).toBe('第 3 次')
    expect(rows[1].find('.devices-ban-strike').exists()).toBe(false)
    expect(rows[2].find('.devices-ban-strike').exists()).toBe(false)
    wrapper.unmount()
  })

  it('网段条目类型徽标（八期）：kind=cidr 显「网段」徽标，kind 缺省（旧载荷）不渲染', async () => {
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([
      banEntry({ ip: '203.0.113.0/24', kind: 'cidr', source: 'manual', failCount: 0 }),
      banEntry({ ip: '198.51.100.9' }) // 旧载荷形态（无 kind 字段）
    ])
    const wrapper = await mountFlushed()
    const rows = wrapper.findAll('.devices-ban-row')
    expect(rows[0].find('.devices-ban-kind').exists()).toBe(true)
    expect(rows[0].find('.devices-ban-kind').text()).toBe('网段')
    // 网段条目行仍渲染解封按钮（复用同一交互面）
    expect(rows[0].find('.devices-ban-unban').exists()).toBe(true)
    expect(rows[1].find('.devices-ban-kind').exists()).toBe(false)
    wrapper.unmount()
  })

  it('覆盖提示徽标（八期）：coveredBy 非空渲染「被覆盖」徽标，tooltip 载明覆盖方网段', async () => {
    const { ListIPBans } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([
      banEntry({ ip: '203.0.113.7', coveredBy: '203.0.113.0/24' }),
      banEntry({ ip: '198.51.100.9' })
    ])
    const wrapper = await mountFlushed()
    const rows = wrapper.findAll('.devices-ban-row')
    expect(rows[0].find('.devices-ban-covered').exists()).toBe(true)
    expect(rows[0].find('.devices-ban-covered').text()).toBe('被覆盖')
    // el-tooltip stub 渲染 .tooltip-content：载荷为覆盖方 CIDR 串
    expect(rows[0].find('.tooltip-content').text()).toContain('203.0.113.0/24')
    expect(rows[1].find('.devices-ban-covered').exists()).toBe(false)
    wrapper.unmount()
  })

  it('网段条目解封确认链（八期）：单次确认（文案带网段与影响面）→ UnbanCIDR(cidr)，UnbanIP 不调用', async () => {
    const { UnbanIP, UnbanCIDR, ListIPBans } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox, ElMessage } = await import('element-plus')
    ElMessageBox.confirm.mockResolvedValue()
    UnbanCIDR.mockResolvedValue()
    ListIPBans.mockResolvedValue([
      banEntry({ ip: '203.0.113.0/24', kind: 'cidr', source: 'manual', failCount: 0 })
    ])
    const wrapper = await mountFlushed()
    await wrapper.find('.devices-ban-unban').trigger('click')
    await flushConfirm()
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    const [message, title] = ElMessageBox.confirm.mock.calls[0]
    expect(title).toBe('解封网段')
    expect(message).toContain('203.0.113.0/24')
    expect(message).toContain('全部来源 IP')
    expect(UnbanCIDR).toHaveBeenCalledTimes(1)
    expect(UnbanCIDR).toHaveBeenCalledWith('203.0.113.0/24')
    expect(UnbanIP).not.toHaveBeenCalled()
    expect(ElMessage.success).toHaveBeenCalledWith('已解封 203.0.113.0/24')
    // 解封成功后重拉列表（挂载 1 次 + reload 1 次）
    expect(ListIPBans).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})

// ---- 聚合建议小节（连接管控九期：同网段自动封禁达阈值产出网段封禁建议，点击
// 行预填 BanIPDialog 网段档；数据源 ListIPBanSuggestions 桌面专用 + 刷新时机
// 对齐封禁列表）----

describe('ActivityBar - 聚合建议小节', () => {
  const createWrapper = () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.activePanel = 'directory'
    return mount(ActivityBar, {
      global: {
        plugins: [pinia],
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-tooltip': { template: '<div><span class="tooltip-content">{{ content }}</span><slot /></div>', props: ['content', 'placement'] },
          'el-popover': {
            template: '<div class="el-popover-stub"><slot name="reference" /><div class="popover-content"><slot /></div></div>',
            props: ['placement', 'width', 'trigger', 'persistent']
          }
        }
      }
    })
  }

  /** 挂载并 flush 挂载期两数据源（ListIPBans + ListIPBanSuggestions）异步加载 */
  const mountFlushed = async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await nextTick()
    return wrapper
  }

  beforeEach(async () => {
    vi.clearAllMocks()
    const { ListIPBans, ListIPBanSuggestions } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([])
    ListIPBanSuggestions.mockResolvedValue([])
  })

  it('空建议小节不渲染（v-if 整体隐藏，IP 封禁区块仅显封禁列表空态）', async () => {
    const wrapper = await mountFlushed()
    // 注入一条连接使面板可见（可见条件 = 连接存在或封禁存在，九期不扩条件）
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() }
    ])
    await nextTick()
    expect(wrapper.find('.devices-ban-section').exists()).toBe(true)
    expect(wrapper.find('.devices-suggest-block').exists()).toBe(false)
    wrapper.unmount()
  })

  it('有建议渲染小节标题与建议行（网段串 + N 个来源计数 + 建议文案）', async () => {
    const { ListIPBans, ListIPBanSuggestions } = await import('../../../wailsjs/go/main/App')
    ListIPBanSuggestions.mockResolvedValue([
      { cidr: '198.51.100.0/24', count: 4 },
      { cidr: '203.0.113.0/24', count: 3 }
    ])
    ListIPBans.mockResolvedValue([
      { ip: '203.0.113.7', bannedAt: Math.floor(Date.now() / 1000) - 60, expiresAt: Math.floor(Date.now() / 1000) + 300, failCount: 10, expired: false, source: 'auto' }
    ])
    const wrapper = await mountFlushed()
    const block = wrapper.find('.devices-suggest-block')
    expect(block.exists()).toBe(true)
    expect(block.find('.devices-suggest-title').text()).toBe('聚合建议')
    const rows = wrapper.findAll('.devices-suggest-row')
    expect(rows.length).toBe(2)
    // 后端已排序（计数降序），前端零排序原样渲染
    expect(rows[0].text()).toContain('198.51.100.0/24')
    expect(rows[0].text()).toContain('该网段 4 个来源已被封禁，建议网段封禁')
    expect(rows[1].text()).toContain('203.0.113.0/24')
    expect(rows[1].text()).toContain('该网段 3 个来源已被封禁，建议网段封禁')
    wrapper.unmount()
  })

  it('点击建议行打开 BanIPDialog 并透传 initialCidr（预填链路穿透断言）', async () => {
    const { ListIPBans, ListIPBanSuggestions } = await import('../../../wailsjs/go/main/App')
    ListIPBanSuggestions.mockResolvedValue([{ cidr: '203.0.113.0/24', count: 3 }])
    ListIPBans.mockResolvedValue([
      { ip: '203.0.113.7', bannedAt: Math.floor(Date.now() / 1000) - 60, expiresAt: Math.floor(Date.now() / 1000) + 300, failCount: 10, expired: false, source: 'auto' }
    ])
    const wrapper = await mountFlushed()
    await wrapper.find('.devices-suggest-row').trigger('click')
    await nextTick()
    const dialog = wrapper.findComponent({ name: 'BanIPDialog' })
    expect(dialog.exists(), 'BanIPDialog 桩应挂载').toBe(true)
    expect(dialog.props('modelValue'), '点击建议行后 v-model 置 true').toBe(true)
    expect(dialog.props('initialCidr'), '建议网段串经 initialCidr 透传预填').toBe('203.0.113.0/24')
    wrapper.unmount()
  })

  it('头部「封禁 IP」按钮路径 initialCidr 为空串（与建议行入口互斥，默认单 IP 档）', async () => {
    const wrapper = await mountFlushed()
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() }
    ])
    await nextTick()
    await wrapper.find('.devices-ban-add').trigger('click')
    await nextTick()
    const dialog = wrapper.findComponent({ name: 'BanIPDialog' })
    expect(dialog.props('modelValue')).toBe(true)
    expect(dialog.props('initialCidr')).toBe('')
    wrapper.unmount()
  })

  it('webserve-ip-banned 事件到达重拉建议（与封禁列表同款事件驱动刷新）', async () => {
    const { ListIPBans, ListIPBanSuggestions } = await import('../../../wailsjs/go/main/App')
    // 注入一条封禁使面板可见（可见条件 = 连接存在或封禁存在，九期不扩条件）
    ListIPBans.mockResolvedValue([
      { ip: '203.0.113.7', bannedAt: Math.floor(Date.now() / 1000) - 60, expiresAt: Math.floor(Date.now() / 1000) + 300, failCount: 10, expired: false, source: 'auto' }
    ])
    const wrapper = await mountFlushed()
    expect(ListIPBanSuggestions).toHaveBeenCalledTimes(1)
    ListIPBanSuggestions.mockResolvedValue([{ cidr: '203.0.113.0/24', count: 3 }])
    eventHandlers['webserve-ip-banned']({ ip: '203.0.113.9', failCount: 10 })
    await flushPromises()
    await nextTick()
    expect(ListIPBanSuggestions).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.devices-suggest-row').text()).toContain('203.0.113.0/24')
    wrapper.unmount()
  })

  it('封禁动作后 @banned 重拉建议（面板打开与动作后列表、建议口径同步）', async () => {
    const { ListIPBans, ListIPBanSuggestions } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([
      { ip: '203.0.113.7', bannedAt: Math.floor(Date.now() / 1000) - 60, expiresAt: Math.floor(Date.now() / 1000) + 300, failCount: 10, expired: false, source: 'auto' }
    ])
    const wrapper = await mountFlushed()
    expect(ListIPBanSuggestions).toHaveBeenCalledTimes(1)
    await wrapper.findComponent({ name: 'BanIPDialog' }).vm.$emit('banned')
    await flushPromises()
    expect(ListIPBanSuggestions).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})

// ---- 攻击趋势区块（连接管控十期：审计历史 ip_ban/ip_unban 天桶聚合的时序视图，
// 数据源 QueryIPBanTrend 桌面专用 + 刷新时机对齐封禁列表/建议 + 行点击复用九期
// 封禁弹窗预填链）----

describe('ActivityBar - 攻击趋势区块', () => {
  const createWrapper = () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.activePanel = 'directory'
    return mount(ActivityBar, {
      global: {
        plugins: [pinia],
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-tooltip': { template: '<div><span class="tooltip-content">{{ content }}</span><slot /></div>', props: ['content', 'placement'] },
          'el-popover': {
            template: '<div class="el-popover-stub"><slot name="reference" /><div class="popover-content"><slot /></div></div>',
            props: ['placement', 'width', 'trigger', 'persistent']
          }
        }
      }
    })
  }

  /** 趋势结果 fixture（形状对齐 model.IPBanTrendResult / IPBanTrendBucket） */
  const trendResult = (over = {}) => ({
    windowDays: 7,
    earliestTs: 0,
    buckets: [],
    ...over
  })

  /** 挂载并 flush 挂载期三数据源（ListIPBans + ListIPBanSuggestions + QueryIPBanTrend）异步加载 */
  const mountFlushed = async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await nextTick()
    return wrapper
  }

  beforeEach(async () => {
    vi.clearAllMocks()
    const { ListIPBans, QueryIPBanTrend } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([])
    QueryIPBanTrend.mockResolvedValue(trendResult())
  })

  it('挂载即拉取趋势，空桶显低调空态文案（标题带窗口天数后缀）', async () => {
    const { QueryIPBanTrend } = await import('../../../wailsjs/go/main/App')
    const wrapper = await mountFlushed()
    // 注入一条连接使面板可见（可见条件 = 连接存在或封禁存在）
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() }
    ])
    await nextTick()
    expect(QueryIPBanTrend).toHaveBeenCalled()
    const block = wrapper.find('.devices-trend-block')
    expect(block.exists()).toBe(true)
    // 默认 mock windowDays=7：标题带窗口天数后缀
    expect(block.find('.devices-suggest-title').text()).toBe('攻击趋势（近 7 天）')
    expect(block.find('.devices-trend-empty').text()).toBe('暂无攻击记录')
    expect(block.find('.devices-trend-list').exists()).toBe(false)
    // 空桶且 earliestTs=0：earliest 提示行不渲染
    expect(block.find('.devices-trend-earliest').exists()).toBe(false)
    wrapper.unmount()
  })

  it('桶行渲染：key + 窗口总数文案 + 天级条形（含零值基线）+ 处置/再犯徽标 + 标题带窗口天数', async () => {
    const { QueryIPBanTrend } = await import('../../../wailsjs/go/main/App')
    QueryIPBanTrend.mockResolvedValue(trendResult({
      buckets: [
        { key: '203.0.113.0/24', days: [0, 1, 0, 3, 0, 2, 5], autoTotal: 11, manualCount: 2, rebounded: true, lastSeen: 1735689900 },
        { key: '198.51.100.0/24', days: [1], autoTotal: 1, manualCount: 0, rebounded: false, lastSeen: 1735689000 }
      ]
    }))
    const wrapper = await mountFlushed()
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() }
    ])
    await nextTick()
    const block = wrapper.find('.devices-trend-block')
    expect(block.find('.devices-suggest-title').text()).toBe('攻击趋势（近 7 天）')
    const rows = wrapper.findAll('.devices-trend-row')
    expect(rows.length).toBe(2)
    // 后端已排序（autoTotal 降序），前端零排序原样渲染
    expect(rows[0].find('.devices-panel-ip').text()).toBe('203.0.113.0/24')
    expect(rows[0].find('.devices-trend-count').text()).toBe('11 次自动封禁')
    // 天级条形逐日渲染，零值日带 is-empty 基线类
    const bars = rows[0].findAll('.devices-trend-bar')
    expect(bars.length).toBe(7)
    expect(bars.filter((b) => b.classes().includes('is-empty')).length).toBe(3)
    // 处置徽标（manualCount>0）与再犯徽标（rebounded）
    expect(rows[0].find('.devices-trend-manual').text()).toBe('处置 2')
    expect(rows[0].find('.devices-trend-rebounded').text()).toBe('再犯')
    // manualCount=0 与 rebounded=false 不渲染徽标
    expect(rows[1].find('.devices-trend-manual').exists()).toBe(false)
    expect(rows[1].find('.devices-trend-rebounded').exists()).toBe(false)
    wrapper.unmount()
  })

  it('行点击打开 BanIPDialog 并透传桶 key（复用九期网段预填链，桶 key 即 CIDR 串）', async () => {
    const { QueryIPBanTrend } = await import('../../../wailsjs/go/main/App')
    QueryIPBanTrend.mockResolvedValue(trendResult({
      buckets: [
        { key: '203.0.113.0/24', days: [0, 1, 0, 3, 0, 2, 5], autoTotal: 11, manualCount: 0, rebounded: false, lastSeen: 1735689900 }
      ]
    }))
    const wrapper = await mountFlushed()
    // 注入一条连接使面板可见（可见条件 = 连接存在或封禁存在）
    eventHandlers['webserve-connections:changed']([
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() }
    ])
    await nextTick()
    await wrapper.find('.devices-trend-row').trigger('click')
    await nextTick()
    const dialog = wrapper.findComponent({ name: 'BanIPDialog' })
    expect(dialog.exists(), 'BanIPDialog 桩应挂载').toBe(true)
    expect(dialog.props('modelValue'), '点击趋势行后 v-model 置 true').toBe(true)
    expect(dialog.props('initialCidr'), '桶 key 经 initialCidr 透传预填').toBe('203.0.113.0/24')
    wrapper.unmount()
  })

  it('earliestTs 提示行三分支：窗口起点之前不提示 / 窗口内起点显「数据自 M-D 起」/ 无记录不提示', async () => {
    const { QueryIPBanTrend } = await import('../../../wailsjs/go/main/App')
    const nowSec = Math.floor(Date.now() / 1000)
    // 注入一条连接使面板可见（可见条件 = 连接存在或封禁存在）
    const injectPanelVisible = async () => {
      eventHandlers['webserve-connections:changed']([
        { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() }
      ])
      await nextTick()
    }
    // 留存覆盖整个窗口（earliestTs 早于窗口起点 = 今天本地 0 点往回 6 天）→ 不提示
    QueryIPBanTrend.mockResolvedValue(trendResult({ earliestTs: nowSec - 9 * 86400 }))
    let wrapper = await mountFlushed()
    await injectPanelVisible()
    expect(wrapper.find('.devices-trend-earliest').exists()).toBe(false)
    wrapper.unmount()
    // 留存未覆盖窗口（earliestTs 在窗口内）→ 提示「数据自 M-D 起（更早历史已随审计日志轮转）」
    QueryIPBanTrend.mockResolvedValue(trendResult({ earliestTs: nowSec - 2 * 86400 }))
    wrapper = await mountFlushed()
    await injectPanelVisible()
    const hint = wrapper.find('.devices-trend-earliest')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toMatch(/^数据自 \d{1,2}-\d{1,2} 起（更早历史已随审计日志轮转）$/)
    wrapper.unmount()
    // 快照无 ip_ban 行（earliestTs=0）→ 不提示
    QueryIPBanTrend.mockResolvedValue(trendResult({ earliestTs: 0 }))
    wrapper = await mountFlushed()
    await injectPanelVisible()
    expect(wrapper.find('.devices-trend-earliest').exists()).toBe(false)
    wrapper.unmount()
  })

  it('webserve-ip-banned 事件到达重拉趋势（与列表/建议同款事件驱动刷新）', async () => {
    const { ListIPBans, QueryIPBanTrend } = await import('../../../wailsjs/go/main/App')
    ListIPBans.mockResolvedValue([
      { ip: '203.0.113.7', bannedAt: Math.floor(Date.now() / 1000) - 60, expiresAt: Math.floor(Date.now() / 1000) + 300, failCount: 10, expired: false, source: 'auto' }
    ])
    const wrapper = await mountFlushed()
    expect(QueryIPBanTrend).toHaveBeenCalledTimes(1)
    QueryIPBanTrend.mockResolvedValue(trendResult({
      buckets: [
        { key: '192.0.2.0/24', days: [2], autoTotal: 2, manualCount: 0, rebounded: false, lastSeen: 1735689900 }
      ]
    }))
    eventHandlers['webserve-ip-banned']({ ip: '192.0.2.66', failCount: 10 })
    await flushPromises()
    await nextTick()
    expect(QueryIPBanTrend).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.devices-trend-row').text()).toContain('192.0.2.0/24')
    wrapper.unmount()
  })

  it('封禁动作后 @banned 重拉趋势（reloadBanPanel 三数据源同步——新网段封禁改变处置计数与再犯观测）', async () => {
    const { QueryIPBanTrend } = await import('../../../wailsjs/go/main/App')
    const wrapper = await mountFlushed()
    expect(QueryIPBanTrend).toHaveBeenCalledTimes(1)
    await wrapper.findComponent({ name: 'BanIPDialog' }).vm.$emit('banned')
    await flushPromises()
    expect(QueryIPBanTrend).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
