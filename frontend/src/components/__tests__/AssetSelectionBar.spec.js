import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AssetSelectionBar from '../AssetSelectionBar.vue'

// el-button stub：click 经 $emit 派发（emits 声明防 attrs 透传双触发）
const ElButtonStub = {
  template: '<button class="el-button" :data-type="type" @click="$emit(\'click\')"><slot /></button>',
  props: ['type', 'size', 'text'],
  emits: ['click']
}

const mountBar = (count = 0) =>
  mount(AssetSelectionBar, {
    props: { count },
    global: { stubs: { 'el-button': ElButtonStub } }
  })

describe('AssetSelectionBar', () => {
  it('显示条件：count > 0 浮出并展示「已选 N 项」，count = 0 不渲染（PRD D2）', () => {
    expect(mountBar(0).find('.asset-selection-bar').exists()).toBe(false)
    const wrapper = mountBar(3)
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)
    expect(wrapper.find('.selection-count').text()).toBe('已选 3 项')
  })

  it('三个操作按钮：全选/反选/清空分别上报 select-all / invert / clear（分派留宿主）', async () => {
    const wrapper = mountBar(2)
    const byText = (text) => wrapper.findAll('button').find((b) => b.text().trim() === text)
    await byText('全选').trigger('click')
    await byText('反选').trigger('click')
    await byText('清空').trigger('click')
    expect(wrapper.emitted('select-all')).toHaveLength(1)
    expect(wrapper.emitted('invert')).toHaveLength(1)
    expect(wrapper.emitted('clear')).toHaveLength(1)
  })

  it('批量打标签按钮（第六批 PR3）：上报 batch-tag 事件（targets 构建与 API 分派留宿主）', async () => {
    const wrapper = mountBar(2)
    const byText = (text) => wrapper.findAll('button').find((b) => b.text().trim() === text)
    expect(byText('批量打标签')).toBeTruthy()
    await byText('批量打标签').trigger('click')
    expect(wrapper.emitted('batch-tag')).toHaveLength(1)
  })
})
