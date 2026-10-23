import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import ActivityBar from '../ActivityBar.vue'
import { useUiStore } from '../../store'

// Wails 事件 handler 注册表（EventsOn mock 捕获）：多设备连接数事件驱动断言用
const eventHandlers = {}

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((name, cb) => {
    eventHandlers[name] = cb
    return () => {
      delete eventHandlers[name]
    }
  })
}))

// 踢出链依赖（连接管控任务）：App 绑定 + ElMessageBox/ElMessage（确认弹窗与
// handleError 分流断言；文件级工厂，对齐 confirm-dialog vitest mock 形态）
vi.mock('../../../wailsjs/go/main/App', () => ({
  KickConnection: vi.fn()
}))

vi.mock('element-plus', () => ({
  ElMessageBox: { confirm: vi.fn() },
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn() }
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
