import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetThumb from '../AssetThumb.vue'

// vi.mock 工厂被 hoist，不能引用外部变量；用 vi.hoisted 提升可变 mock 函数
const { wailsMock } = vi.hoisted(() => ({
  wailsMock: { GetAssetThumb: vi.fn() }
}))

vi.mock('../../../wailsjs/go/main/App', () => wailsMock)

// IntersectionObserver stub：捕获回调供用例手动触发 intersect（模拟卡片进入可视区）；
// trigger 遵循真实 IO 语义——disconnect 后不再派发回调
let ioCallback = null
let ioInstances = []
class IntersectionObserverStub {
  constructor(cb) {
    ioCallback = cb
    ioInstances.push(this)
    this.disconnected = false
    this.observe = vi.fn()
    this.unobserve = vi.fn()
    this.disconnect = vi.fn(() => { this.disconnected = true })
  }
  trigger() {
    if (this.disconnected) return
    ioCallback([{ isIntersecting: true }], this)
  }
}

const mountThumb = (props = {}) =>
  mount(AssetThumb, { props: { libraryId: 'lib-1', fileId: 11, slideNum: 1, ...props } })

describe('AssetThumb', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ioInstances = []
    ioCallback = null
    globalThis.IntersectionObserver = IntersectionObserverStub
    wailsMock.GetAssetThumb.mockReset()
    wailsMock.GetAssetThumb.mockResolvedValue('')
  })

  it('懒加载：IntersectionObserver 触发前不发请求，进入可视区才加载并渲染 data URL 图片', async () => {
    wailsMock.GetAssetThumb.mockResolvedValue('QUFB')
    const wrapper = mountThumb()
    expect(ioInstances.length).toBe(1)
    expect(wailsMock.GetAssetThumb).not.toHaveBeenCalled()

    // 未进入可视区：不请求、渲染占位
    await flushPromises()
    expect(wrapper.find('.asset-thumb-placeholder').exists()).toBe(true)

    // 进入可视区（触发一次即停止观察）：请求加载并渲染 data URL
    ioInstances[0].trigger()
    await flushPromises()
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledTimes(1)
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledWith('lib-1', 11, 1, 0)
    expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,QUFB')

    // 再次触发（已 unobserve 语义）：不重复请求
    ioInstances[0].trigger()
    await flushPromises()
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledTimes(1)
  })

  it('无 IntersectionObserver 环境（jsdom/旧内核）：挂载即加载兜底', async () => {
    delete globalThis.IntersectionObserver
    const wrapper = mountThumb()
    await flushPromises()
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledTimes(1)
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledWith('lib-1', 11, 1, 0)
    // mock 默认空串 = 未生成：miss 占位
    expect(wrapper.find('.asset-thumb-placeholder').text()).toBe('无预览')
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('空串返回：miss 占位（后端契约：未生成/已淘汰返回空串非错误）', async () => {
    const wrapper = mountThumb()
    ioInstances[0].trigger()
    await flushPromises()
    expect(wrapper.find('.asset-thumb-placeholder').exists()).toBe(true)
    expect(wrapper.find('.asset-thumb-placeholder').text()).toBe('无预览')
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('hideOnMiss：loading/miss 均不渲染组件（详情两层大图由 small 底图透出），ready 才渲染', async () => {
    const wrapper = mountThumb({ hideOnMiss: true })
    await flushPromises()
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledTimes(1) // 详情面板打开即可见：挂载即加载
    expect(wrapper.find('.asset-thumb').exists()).toBe(false)

    // 后台生成完成后（epoch 变化）：miss 重试；加载期仍不渲染（底图保持可见），
    // ready 后直接渲染大图替换底图
    wailsMock.GetAssetThumb.mockResolvedValue('SEHD')
    await wrapper.setProps({ epoch: 1 })
    await flushPromises()
    expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,SEHD')
  })

  it('epoch 变化：miss 卡自动重试补图（后台批次完成后的补图入口），ready 卡不重拉', async () => {
    delete globalThis.IntersectionObserver
    const wrapper = mountThumb()
    await flushPromises()
    expect(wrapper.find('.asset-thumb-placeholder').exists()).toBe(true)
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledTimes(1)

    // ready 后 epoch 变化：不重拉（避免无谓大载荷传输）
    wailsMock.GetAssetThumb.mockResolvedValue('BBB')
    await wrapper.setProps({ epoch: 1 })
    await flushPromises()
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledTimes(2)
    expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,BBB')

    // 已 ready 再变 epoch：不重拉
    await wrapper.setProps({ epoch: 2 })
    await flushPromises()
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledTimes(2)
  })

  it('props 变更请求序号守卫：切卡后旧卡在途慢响应写回被丢弃（不串卡）', async () => {
    delete globalThis.IntersectionObserver
    let resolveOld
    wailsMock.GetAssetThumb.mockImplementation((_libraryId, fileId) => {
      if (fileId === 11) return new Promise((resolve) => { resolveOld = () => resolve('OLDCARD') })
      return Promise.resolve('NEWCARD')
    })
    const wrapper = mountThumb({ fileId: 11 })
    await flushPromises()

    // 切卡（fileId 12 请求立即落定）
    await wrapper.setProps({ fileId: 12 })
    await flushPromises()
    expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,NEWCARD')

    // 旧卡慢响应后到：序号守卫丢弃，不覆写新卡图片
    resolveOld()
    await flushPromises()
    expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,NEWCARD')
  })

  it('读取异常：静默回退 miss 占位不抛错（画廊批量懒加载逐卡弹错噪音大，后台 slog 已记录）', async () => {
    delete globalThis.IntersectionObserver
    wailsMock.GetAssetThumb.mockRejectedValue(new Error('读取失败'))
    const wrapper = mountThumb()
    await flushPromises()
    expect(wrapper.find('.asset-thumb-placeholder').text()).toBe('无预览')
  })

  it('参数不完整（fileId 无底册记录为 0）：不发请求直接 miss', async () => {
    delete globalThis.IntersectionObserver
    const wrapper = mountThumb({ fileId: 0 })
    await flushPromises()
    expect(wailsMock.GetAssetThumb).not.toHaveBeenCalled()
    expect(wrapper.find('.asset-thumb-placeholder').exists()).toBe(true)
  })
})
