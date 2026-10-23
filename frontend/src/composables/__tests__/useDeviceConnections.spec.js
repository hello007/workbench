import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, defineComponent } from 'vue'
import { useDeviceConnections, WS_CONNECTIONS_EVENT } from '../useDeviceConnections'

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

  it('初始 deviceCount=1（桌面模式无事件恒此值，提示不显示）', () => {
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
    // 回落 1：提示消失
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

  it('卸载时经 EventsOn 返回闭包精准注销（不再接收后续事件）', async () => {
    const wrapper = mount(Harness)
    expect(eventHandlers[WS_CONNECTIONS_EVENT]).toBeTypeOf('function')
    wrapper.unmount()
    expect(eventHandlers[WS_CONNECTIONS_EVENT]).toBeUndefined()
    expect(offSpy).toHaveBeenCalledTimes(1)
  })
})
