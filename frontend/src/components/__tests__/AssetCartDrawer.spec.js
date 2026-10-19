import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AssetCartDrawer from '../AssetCartDrawer.vue'

// ElMessageBox（清空确认）mock：默认 resolve = 用户确认（部分用例 reject 模拟取消）
vi.mock('element-plus', () => ({
  ElMessageBox: { confirm: vi.fn(() => Promise.resolve(true)) }
}))

// ---- 测试数据（形状对齐 model.AssetCartItem 经 Wails 绑定的 JSON 字段，
// stale 为 List 探测态非持久化字段）----
const cartItem = (overrides = {}) => ({
  libraryId: 'lib-1',
  fileId: 11,
  slideNum: 2,
  srcPath: 'D:/assets/产品素材库/方案A/城商行方案.pptx',
  fileName: '城商行方案.pptx',
  slideTitle: '背景页',
  libraryName: '产品素材库',
  addedAt: '2026-10-19T10:00:00+08:00',
  ...overrides
})

// AssetThumb 已有独立 spec 覆盖加载行为，此处 stub 排除干扰（仅断言 props 透传）
const AssetThumbStub = {
  template: '<div class="asset-thumb-stub" :data-library-id="libraryId" :data-file-id="fileId" :data-slide-num="slideNum" :data-tier="tier"></div>',
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

const ElButtonStub = {
  template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
  props: ['disabled', 'type', 'size', 'loading', 'text', 'plain'],
  emits: ['click']
}

const stubs = {
  AssetThumb: AssetThumbStub,
  'el-button': ElButtonStub,
  'el-empty': { template: '<div class="el-empty">{{ description }}</div>', props: ['description', 'imageSize'] },
  'el-progress': { template: '<div class="el-progress" :data-percentage="percentage" />', props: ['percentage', 'strokeWidth', 'showText'] },
  'el-icon': { template: '<i @click="$emit(\'click\', $event)"><slot /></i>', props: ['title'], emits: ['click'] }
}

// VueDraggable 用直通 stub（保留默认插槽渲染 + 透传 modelValue 与 end 事件），
// 拖拽行为本身由 vue-draggable-plus 库保证，本 spec 断言宿主接线
const VueDraggableStub = {
  name: 'VueDraggableStub',
  template: '<div class="vue-draggable-stub"><slot /></div>',
  props: ['modelValue', 'animation', 'ghostClass', 'handle'],
  emits: ['end']
}

const mountDrawer = (props = {}) =>
  mount(AssetCartDrawer, {
    props: {
      visible: true,
      items: [cartItem(), cartItem({ fileId: 22, slideNum: 1, slideTitle: '', fileName: '收尾方案.pptx', libraryName: '另一个库', libraryId: 'lib-2' })],
      exporting: false,
      exportProgress: null,
      ...props
    },
    global: {
      stubs: { ...stubs, VueDraggable: VueDraggableStub }
    }
  })

describe('AssetCartDrawer', () => {
  it('显隐受控：visible=false 不渲染，visible=true 渲染标题与条目计数', () => {
    const hidden = mountDrawer({ visible: false })
    expect(hidden.find('.cart-drawer').exists()).toBe(false)
    hidden.unmount()

    const wrapper = mountDrawer()
    expect(wrapper.find('.cart-drawer').exists()).toBe(true)
    expect(wrapper.find('.cart-title').text()).toContain('购物栏')
    expect(wrapper.find('.cart-count').text()).toBe('2')
    wrapper.unmount()
  })

  it('条目渲染（PRD 来源行契约）：库名/文件名/页码 + 缩略图 tier=0 懒加载透传', () => {
    const wrapper = mountDrawer()
    const rows = wrapper.findAll('.cart-item')
    expect(rows).toHaveLength(2)
    const thumbs = wrapper.findAll('.asset-thumb-stub')
    expect(thumbs[0].attributes('data-library-id')).toBe('lib-1')
    expect(thumbs[0].attributes('data-file-id')).toBe('11')
    expect(thumbs[0].attributes('data-slide-num')).toBe('2')
    expect(thumbs[0].attributes('data-tier')).toBe('0')
    // 跨库条目来源行：库名展示（快照 libraryName，无则回退 libraryId）
    expect(rows[0].text()).toContain('产品素材库')
    expect(rows[0].text()).toContain('城商行方案.pptx')
    expect(rows[0].text()).toContain('第 2 页')
    expect(rows[1].text()).toContain('另一个库')
    wrapper.unmount()
  })

  it('页标题优先展示（slideTitle 快照），空标题回退文件名', () => {
    const wrapper = mountDrawer()
    const titles = wrapper.findAll('.cart-item-title')
    expect(titles[0].text()).toBe('背景页')
    expect(titles[1].text()).toBe('收尾方案.pptx')
    wrapper.unmount()
  })

  it('失效条目（stale）：置灰 is-stale class + 「源文件不存在」标记，仍可移除', () => {
    const wrapper = mountDrawer({ items: [cartItem({ stale: true })] })
    const row = wrapper.find('.cart-item')
    expect(row.classes()).toContain('is-stale')
    expect(wrapper.find('.cart-item-stale-mark').text()).toContain('源文件不存在')
    wrapper.unmount()
  })

  it('空购物栏：空态引导渲染', () => {
    const wrapper = mountDrawer({ items: [] })
    expect(wrapper.find('.el-empty').exists()).toBe(true)
    wrapper.unmount()
  })

  it('行点击上报 trace（溯源定位分派留宿主）、移除图标上报 remove（载荷为条目对象）', async () => {
    const wrapper = mountDrawer()
    await wrapper.find('.cart-item').trigger('click')
    expect(wrapper.emitted('trace')).toEqual([[wrapper.props('items')[0]]])
    await wrapper.find('.cart-item-remove').trigger('click')
    expect(wrapper.emitted('remove')).toEqual([[wrapper.props('items')[0]]])
    // 移除不冒泡进 trace（@click.stop）
    expect(wrapper.emitted('trace')).toHaveLength(1)
    wrapper.unmount()
  })

  it('拖拽结束上报 reorder：载荷为新顺序的按键列表（与后端 Key 同构）', async () => {
    const wrapper = mountDrawer()
    // 模拟拖拽后副本重排（交换两条）再触发 end
    const draggable = wrapper.findComponent({ name: 'VueDraggableStub' })
    const reordered = [...wrapper.props('items')].reverse()
    await draggable.vm.$emit('update:modelValue', reordered)
    await draggable.vm.$emit('end')
    expect(wrapper.emitted('reorder')).toEqual([[['lib-2|22|1', 'lib-1|11|2']]])
    wrapper.unmount()
  })

  it('导出按钮：空购物栏禁用；有条目点击上报 export（分派留宿主）', async () => {
    const empty = mountDrawer({ items: [] })
    const emptyExportBtn = empty.findAll('.cart-footer button').find((b) => b.text().includes('导出'))
    expect(emptyExportBtn.attributes('disabled')).toBeDefined()
    empty.unmount()

    const wrapper = mountDrawer()
    const exportBtn = wrapper.findAll('.cart-footer button').find((b) => b.text().includes('导出'))
    expect(exportBtn.text()).toContain('2 页')
    await exportBtn.trigger('click')
    expect(wrapper.emitted('export')).toHaveLength(1)
    wrapper.unmount()
  })

  it('导出运行态：导出按钮 loading、进度条渲染、清空/移除/导出禁用、取消入口出现', async () => {
    const wrapper = mountDrawer({ exporting: true, exportProgress: { outPath: 'D:/out.pptx', done: 1, total: 2 } })
    expect(wrapper.find('.cart-export-progress').exists()).toBe(true)
    const footerBtns = wrapper.findAll('.cart-footer button')
    const clearBtn = footerBtns.find((b) => b.text().trim() === '清空')
    expect(clearBtn.attributes('disabled')).toBeDefined()
    expect(footerBtns.find((b) => b.text().includes('取消导出'))).toBeTruthy()
    const removeIcon = wrapper.find('.cart-item-remove')
    expect(removeIcon.attributes('style') || '').not.toContain('display: none')
    // 移除按钮仍可点（运行态禁改由宿主 addToCart/Remove 入口 warning 拦截，
    // 抽屉层不做 disabled 断言——图标非禁用控件）
    await removeIcon.trigger('click')
    expect(wrapper.emitted('remove')).toHaveLength(1)
    wrapper.unmount()
  })

  it('取消导出按钮上报 cancel-export', async () => {
    const wrapper = mountDrawer({ exporting: true })
    await wrapper.findAll('.cart-footer button').find((b) => b.text().includes('取消导出')).trigger('click')
    expect(wrapper.emitted('cancel-export')).toHaveLength(1)
    wrapper.unmount()
  })

  it('关闭按钮上报 close', async () => {
    const wrapper = mountDrawer()
    await wrapper.find('.cart-close').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('清空按钮需确认：确认后上报 clear（确认弹窗内闭环）', async () => {
    const wrapper = mountDrawer()
    // ElMessageBox.confirm 已在全局 mock（src/test/setup.js），默认 resolve
    await wrapper.findAll('.cart-footer button').find((b) => b.text().trim() === '清空').trigger('click')
    await vi.waitFor(() => {
      expect(wrapper.emitted('clear')).toHaveLength(1)
    })
    wrapper.unmount()
  })
})
