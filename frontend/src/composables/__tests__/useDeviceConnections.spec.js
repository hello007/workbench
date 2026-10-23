import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, defineComponent } from 'vue'
import { useDeviceConnections, WS_CONNECTIONS_EVENT, WEBSERVE_CONNECTIONS_EVENT } from '../useDeviceConnections'

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

// 挂载一个消费 composable 的最小组件（composable 依赖组件生命周期注册/注销监听）
const Harness = defineComponent({
  setup() {
    const { deviceCount, multiDevice } = useDeviceConnections()
    return () => h('div', { class: 'harness' }, [String(deviceCount.value), String(multiDevice.value)])
  }
})

describe('useDeviceConnections', () => {
  beforeEach(() => {
    for (const k of Object.keys(eventHandlers)) delete eventHandlers[k]
    offSpy.mockClear()
  })

  it('初始 deviceCount=1（两通道首事件到达前按「无其他设备」处理，提示不显示）', () => {
    const wrapper = mount(Harness)
    expect(wrapper.text()).toContain('1')
    expect(wrapper.text()).toContain('false')
    wrapper.unmount()
  })

  it('ws-connections:changed 事件驱动 deviceCount 与 multiDevice', async () => {
    const wrapper = mount(Harness)
    const handler = eventHandlers[WS_CONNECTIONS_EVENT]
    expect(handler).toBeTypeOf('function')
    handler(3)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('true')
    // 回落 1：提示消失（浏览器通道计数含本连接，count>1 才显示）
    handler(1)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('false')
    wrapper.unmount()
  })

  it('非法载荷（非正数/非数值）忽略，保持上次值', async () => {
    const wrapper = mount(Harness)
    const handler = eventHandlers[WS_CONNECTIONS_EVENT]
    handler(2)
    await wrapper.vm.$nextTick()
    handler(0)
    handler(-1)
    handler('NaN')
    handler(undefined)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('2')
    wrapper.unmount()
  })

  it('桌面事件锁定通道：count=1 即显示（桌面视角计数为纯浏览器数）', async () => {
    const wrapper = mount(Harness)
    const desktop = eventHandlers[WEBSERVE_CONNECTIONS_EVENT]
    expect(desktop).toBeTypeOf('function')
    desktop(1)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('1')
    expect(wrapper.text()).toContain('true')
    wrapper.unmount()
  })

  it('桌面事件接受 count=0（最后一个浏览器断开/webServe 停机熄指示）', async () => {
    const wrapper = mount(Harness)
    const desktop = eventHandlers[WEBSERVE_CONNECTIONS_EVENT]
    desktop(2)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('true')
    desktop(0)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('0')
    expect(wrapper.text()).toContain('false')
    wrapper.unmount()
  })

  it('桌面非法载荷忽略且不锁定通道归属（后续仍走浏览器口径）', async () => {
    const wrapper = mount(Harness)
    const desktop = eventHandlers[WEBSERVE_CONNECTIONS_EVENT]
    desktop(-1)
    desktop('NaN')
    desktop(undefined)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('false')
    // 浏览器通道事件仍按浏览器口径判定（isDesktopChannel 未被非法载荷置位）
    const browser = eventHandlers[WS_CONNECTIONS_EVENT]
    browser(2)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('true')
    browser(1)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('false')
    wrapper.unmount()
  })

  it('浏览器通道 1 连接不显示（与桌面通道口径分流回归）', async () => {
    const wrapper = mount(Harness)
    eventHandlers[WS_CONNECTIONS_EVENT](1)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('false')
    wrapper.unmount()
  })

  it('卸载时经 EventsOn 返回闭包精准注销双监听器（不再接收后续事件）', async () => {
    const wrapper = mount(Harness)
    expect(eventHandlers[WS_CONNECTIONS_EVENT]).toBeTypeOf('function')
    expect(eventHandlers[WEBSERVE_CONNECTIONS_EVENT]).toBeTypeOf('function')
    wrapper.unmount()
    expect(eventHandlers[WS_CONNECTIONS_EVENT]).toBeUndefined()
    expect(eventHandlers[WEBSERVE_CONNECTIONS_EVENT]).toBeUndefined()
    expect(offSpy).toHaveBeenCalledTimes(2)
  })
})
