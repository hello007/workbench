import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import FollowUpsEditor from '../FollowUpsEditor.vue'

const stubs = {
  'el-form': { template: '<form @submit.prevent><slot /></form>', props: ['labelWidth', 'size'] },
  'el-form-item': { template: '<div class="el-form-item"><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'type', 'rows', 'autosize'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'link', 'disabled'],
    emits: ['click']
  },
  'el-switch': {
    template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
    props: ['modelValue'],
    emits: ['update:modelValue']
  },
  ParamsEditor: { template: '<div class="params-editor-stub" />', props: ['modelValue'] }
}

function createWrapper(model) {
  return mount(FollowUpsEditor, {
    props: { modelValue: model },
    global: { stubs }
  })
}

describe('FollowUpsEditor.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  it('渲染已有 followUps 卡片，空 label 显示(未命名)', async () => {
    const model = reactive([
      { id: 'fu_1', label: '确认', promptTemplate: 'tpl', input: null },
      { id: 'fu_2', label: '', promptTemplate: '', input: null }
    ])
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.findAll('.fu-card').length).toBe(2)
    const titles = wrapper.findAll('.fu-title')
    expect(titles[0].text()).toBe('确认')
    expect(titles[1].text()).toBe('(未命名)')
  })

  it('add：追加新项（含 id/label/promptTemplate/input）', async () => {
    const model = reactive([])
    wrapper = createWrapper(model)
    await nextTick()
    await wrapper.findAll('button').find(b => b.text().includes('新增')).trigger('click')
    expect(model.length).toBe(1)
    expect(model[0].id).toBeTruthy()
    expect(model[0].label).toBe('')
    expect(model[0].input).toBeNull()
  })

  it('add：model 为 null 时初始化数组', async () => {
    wrapper = createWrapper(null)
    await nextTick()
    await wrapper.findAll('button').find(b => b.text().includes('新增')).trigger('click')
    // defineModel 赋值后 emit update:modelValue
    const emits = wrapper.emitted('update:modelValue')
    expect(emits).toBeTruthy()
    const last = emits.at(-1)[0]
    expect(Array.isArray(last)).toBe(true)
    expect(last.length).toBe(1)
    expect(last[0].id).toBeTruthy()
  })

  it('remove：删除指定索引项', async () => {
    const model = reactive([
      { id: 'fu_1', label: 'A', promptTemplate: '', input: null },
      { id: 'fu_2', label: 'B', promptTemplate: '', input: null }
    ])
    wrapper = createWrapper(model)
    await nextTick()
    // 第一张卡片的删除按钮
    await wrapper.findAll('.fu-card')[0].findAll('button').find(b => b.text() === '删除').trigger('click')
    expect(model.length).toBe(1)
    expect(model[0].label).toBe('B')
  })

  it('moveUp：第二项上移与第一项交换', async () => {
    const model = reactive([
      { id: 'fu_1', label: 'A', promptTemplate: '', input: null },
      { id: 'fu_2', label: 'B', promptTemplate: '', input: null }
    ])
    wrapper = createWrapper(model)
    await nextTick()
    // 第二张卡片的"上移"按钮（第一张的 disabled）
    const card2 = wrapper.findAll('.fu-card')[1]
    const upBtn = card2.findAll('button').find(b => b.text() === '上移')
    expect(upBtn.attributes('disabled')).toBeUndefined()
    await upBtn.trigger('click')
    expect(model[0].label).toBe('B')
    expect(model[1].label).toBe('A')
  })

  it('moveUp：首项上移按钮禁用', async () => {
    const model = reactive([
      { id: 'fu_1', label: 'A', promptTemplate: '', input: null },
      { id: 'fu_2', label: 'B', promptTemplate: '', input: null }
    ])
    wrapper = createWrapper(model)
    await nextTick()
    const upBtn = wrapper.findAll('.fu-card')[0].findAll('button').find(b => b.text() === '上移')
    expect(upBtn.attributes('disabled')).toBeDefined()
  })

  it('moveDown：第一项下移与第二项交换', async () => {
    const model = reactive([
      { id: 'fu_1', label: 'A', promptTemplate: '', input: null },
      { id: 'fu_2', label: 'B', promptTemplate: '', input: null }
    ])
    wrapper = createWrapper(model)
    await nextTick()
    const downBtn = wrapper.findAll('.fu-card')[0].findAll('button').find(b => b.text() === '下移')
    await downBtn.trigger('click')
    expect(model[0].label).toBe('B')
    expect(model[1].label).toBe('A')
  })

  it('moveDown：末项下移按钮禁用', async () => {
    const model = reactive([
      { id: 'fu_1', label: 'A', promptTemplate: '', input: null },
      { id: 'fu_2', label: 'B', promptTemplate: '', input: null }
    ])
    wrapper = createWrapper(model)
    await nextTick()
    const downBtn = wrapper.findAll('.fu-card')[1].findAll('button').find(b => b.text() === '下移')
    expect(downBtn.attributes('disabled')).toBeDefined()
  })

  it('onInputToggle：开启时给默认 text 输入规格', async () => {
    const model = reactive([{ id: 'fu_1', label: 'A', promptTemplate: '', input: null }])
    wrapper = createWrapper(model)
    await nextTick()
    const sw = wrapper.find('input[type="checkbox"]')
    await sw.setValue(true)
    await sw.trigger('change')
    await nextTick()
    expect(model[0].input).toEqual({ type: 'text', label: '输入', textFieldKey: 'input' })
    // 开启后渲染 ParamsEditor
    expect(wrapper.find('.params-editor-stub').exists()).toBe(true)
    expect(wrapper.text()).toContain('点击前需录入参数')
  })

  it('onInputToggle：关闭时置 null', async () => {
    const model = reactive([{ id: 'fu_1', label: 'A', promptTemplate: '', input: { type: 'text', label: '输入', textFieldKey: 'input' } }])
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.find('.params-editor-stub').exists()).toBe(true)
    const sw = wrapper.find('input[type="checkbox"]')
    await sw.setValue(false)
    await sw.trigger('change')
    await nextTick()
    expect(model[0].input).toBeNull()
    expect(wrapper.find('.params-editor-stub').exists()).toBe(false)
    expect(wrapper.text()).toContain('点击直接发送')
  })

  it('validationErrors：label 为空时报错', async () => {
    const model = reactive([{ id: 'fu_1', label: '', promptTemplate: '', input: null }])
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.vm.validationErrors).toEqual(expect.arrayContaining([expect.stringContaining('label 不能为空')]))
    expect(wrapper.find('.warn-box').exists()).toBe(true)
  })

  it('validationErrors：form input 缺 promptTemplate 报错', async () => {
    const model = reactive([{ id: 'fu_1', label: 'A', promptTemplate: '', input: { type: 'form', promptTemplate: '', fields: [] } }])
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.vm.validationErrors).toEqual(expect.arrayContaining([expect.stringContaining('缺少 promptTemplate')]))
  })

  it('validationErrors：字段齐全时无告警', async () => {
    const model = reactive([{ id: 'fu_1', label: '确认', promptTemplate: 'tpl', input: null }])
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.vm.validationErrors.length).toBe(0)
    expect(wrapper.find('.warn-box').exists()).toBe(false)
  })
})
