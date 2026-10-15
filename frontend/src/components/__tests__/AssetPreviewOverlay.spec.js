import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import AssetPreviewOverlay from '../AssetPreviewOverlay.vue'

// ---- 测试数据（形状对齐 AssetsPanel displayCards 同构卡片模型，kind 全为 slide）----
const cardOf = (id, fileId, slideNum, title) => ({
  key: `slide-${id}`,
  kind: 'slide',
  id,
  fileId,
  title,
  filePath: `方案A/城商行方案.pptx`,
  slideNum,
  tags: [],
  hitCount: 1,
  status: '',
  snippet: '',
  slide: { id, fileId, slideNum, title }
})

const slides = [
  cardOf(21, 11, 1, '封面页'),
  cardOf(22, 11, 2, '背景页'),
  cardOf(23, 12, 1, '收尾页')
]

// AssetThumb 已有独立 spec 覆盖加载行为，此处 stub 掉排除干扰（仅断言 props 透传）。
// hideOnMiss 必须带 Boolean 类型声明：组件模板裸属性 hide-on-miss 值为空串，无类型
// 声明时 prop 收到 ""（falsy），有 Boolean 声明才按 Vue 规则转型为 true（与真实
// AssetThumb 的 prop 声明一致）
const AssetThumbStub = {
  template:
    '<div class="asset-thumb-stub" :data-file-id="fileId" :data-slide-num="slideNum" :data-tier="tier" :data-hide-on-miss="hideOnMiss ? \'1\' : \'0\'" :data-epoch="String(epoch)"></div>',
  props: {
    libraryId: String,
    fileId: Number,
    slideNum: Number,
    tier: Number,
    epoch: Number,
    hideOnMiss: Boolean,
    alt: String
  }
}

const stubs = { AssetThumb: AssetThumbStub }

const mountOverlay = (props = {}) =>
  mount(AssetPreviewOverlay, {
    props: {
      slides,
      index: 0,
      libraryId: 'lib-1',
      thumbEpoch: 7,
      canLoadMore: false,
      ...props
    },
    global: { stubs }
  })

const dispatchKey = (key, init = {}) => {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, ...init }))
}

let wrapper = null

afterEach(() => {
  // 键盘监听挂 document，残留挂载会跨用例响应按键，逐用例卸载
  wrapper?.unmount()
  wrapper = null
})

describe('AssetPreviewOverlay', () => {
  it('渲染大图（AssetThumb tier=1 + hide-on-miss + epoch 透传）与页码指示、信息行', () => {
    wrapper = mountOverlay()
    const thumb = wrapper.find('.asset-thumb-stub')
    expect(thumb.attributes('data-file-id')).toBe('11')
    expect(thumb.attributes('data-slide-num')).toBe('1')
    expect(thumb.attributes('data-tier')).toBe('1')
    expect(thumb.attributes('data-hide-on-miss')).toBe('1')
    expect(thumb.attributes('data-epoch')).toBe('7')
    expect(wrapper.find('.preview-counter').text()).toBe('1 / 3')
    expect(wrapper.find('.preview-title').text()).toBe('封面页')
    expect(wrapper.find('.preview-path').text()).toBe('方案A/城商行方案.pptx')
  })

  it('miss 弱化占位常驻底层（AssetThumb hide-on-miss 不渲染时透出，非空白）', () => {
    wrapper = mountOverlay()
    // 占位元素恒在 DOM（被上层大图覆盖不可见），文案为「暂无高清图」
    expect(wrapper.find('.preview-miss-hint').text()).toBe('暂无高清图')
  })

  it('按钮翻页：→/← 上报 update:index（受控，本层不改自身状态）', async () => {
    wrapper = mountOverlay({ index: 1 })
    await wrapper.find('.preview-nav-next').trigger('click')
    expect(wrapper.emitted('update:index')).toEqual([[2]])
    await wrapper.find('.preview-nav-prev').trigger('click')
    expect(wrapper.emitted('update:index')).toEqual([[2], [0]])
  })

  it('边界禁用：首页 ← 禁用、末页 → 禁用（canLoadMore=false）', () => {
    wrapper = mountOverlay({ index: 0 })
    expect(wrapper.find('.preview-nav-prev').attributes('disabled')).toBeDefined()
    expect(wrapper.find('.preview-nav-next').attributes('disabled')).toBeUndefined()

    wrapper.unmount()
    wrapper = mountOverlay({ index: 2 })
    expect(wrapper.find('.preview-nav-prev').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('.preview-nav-next').attributes('disabled')).toBeDefined()
  })

  it('懒加载边界：末页 canLoadMore=true 时 → 可用，点击上报 load-more 不翻页；非末页照常翻页', async () => {
    wrapper = mountOverlay({ index: 2, canLoadMore: true })
    expect(wrapper.find('.preview-nav-next').attributes('disabled')).toBeUndefined()
    await wrapper.find('.preview-nav-next').trigger('click')
    expect(wrapper.emitted('load-more')).toHaveLength(1)
    expect(wrapper.emitted('update:index')).toBeUndefined()

    await wrapper.find('.preview-nav-prev').trigger('click')
    expect(wrapper.emitted('update:index')).toEqual([[1]])
  })

  it('键盘翻页与 ESC 关闭：←/→ 上报翻页，Escape 上报 close', () => {
    wrapper = mountOverlay({ index: 1 })
    dispatchKey('ArrowRight')
    expect(wrapper.emitted('update:index')).toEqual([[2]])
    dispatchKey('ArrowLeft')
    expect(wrapper.emitted('update:index')).toEqual([[2], [0]])
    dispatchKey('Escape')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('已被消费的按键（defaultPrevented）被过滤，不翻页不关闭', () => {
    wrapper = mountOverlay({ index: 1 })
    for (const key of ['ArrowRight', 'ArrowLeft', 'Escape']) {
      const ev = new KeyboardEvent('keydown', { key, cancelable: true })
      ev.preventDefault()
      document.dispatchEvent(ev)
    }
    expect(wrapper.emitted('update:index')).toBeUndefined()
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('EP 弹层（.el-overlay）与 xterm 辅助 textarea 内的按键被过滤（对齐终端全屏守卫）', () => {
    wrapper = mountOverlay({ index: 1 })
    const overlay = document.createElement('div')
    overlay.className = 'el-overlay'
    document.body.appendChild(overlay)
    overlay.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
    overlay.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))

    const textarea = document.createElement('textarea')
    textarea.className = 'xterm-helper-textarea'
    document.body.appendChild(textarea)
    textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true }))
    textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))

    overlay.remove()
    textarea.remove()
    expect(wrapper.emitted('update:index')).toBeUndefined()
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('可编辑元素聚焦时方向键不翻页（详情侧栏备注编辑与预览并存），ESC 仍关闭', () => {
    wrapper = mountOverlay({ index: 1 })
    const input = document.createElement('input')
    document.body.appendChild(input)
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true }))
    expect(wrapper.emitted('update:index')).toBeUndefined()
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    input.remove()
  })

  it('序列空或下标越界不渲染（宿主侧另有越界关闭，此处组件级防御）', () => {
    const empty = mountOverlay({ slides: [] })
    expect(empty.find('.asset-preview-overlay').exists()).toBe(false)
    empty.unmount()

    wrapper = mountOverlay({ index: 9 })
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(false)
  })

  it('卸载注销 document 键盘监听：卸载后按键不再产生事件', () => {
    // unmount 后实例 emitted 记录被销毁无法断言，改经 attrs 监听 spy 计数
    const onUpdateIndex = vi.fn()
    const onClose = vi.fn()
    wrapper = mount(
      AssetPreviewOverlay,
      {
        props: { slides, index: 1, libraryId: 'lib-1', thumbEpoch: 0, canLoadMore: false },
        global: { stubs },
        attrs: { 'onUpdate:index': onUpdateIndex, onClose }
      }
    )
    dispatchKey('ArrowRight')
    expect(onUpdateIndex).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    wrapper = null
    dispatchKey('ArrowRight')
    dispatchKey('Escape')
    expect(onUpdateIndex).toHaveBeenCalledTimes(1)
    expect(onClose).not.toHaveBeenCalled()
  })
})
