import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import EnvEditor from '../EnvEditor.vue'

const stubs = {
  'el-input': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @change="$emit(\'change\')" />',
    props: ['modelValue', 'placeholder', 'type', 'rows', 'size'],
    emits: ['update:modelValue', 'change']
  },
  'el-button': {
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'link'],
    emits: ['click']
  }
}

function createWrapper(modelValue) {
  return mount(EnvEditor, {
    props: { modelValue },
    global: { stubs }
  })
}

describe('EnvEditor.vue', () => {
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

  it('model → rows 同步：渲染已有键值对', async () => {
    const model = reactive({ FOO: 'bar', BAZ: 'qux' })
    wrapper = createWrapper(model)
    await nextTick()
    const inputs = wrapper.findAll('input')
    // 每行 2 个 input（key + value），2 行 = 4
    expect(inputs.length).toBe(4)
    expect(inputs[0].element.value).toBe('FOO')
    expect(inputs[1].element.value).toBe('bar')
  })

  it('model 为 null 时不报错，渲染空列表', async () => {
    wrapper = createWrapper(null)
    await nextTick()
    expect(wrapper.findAll('input').length).toBe(0)
  })

  it('新增变量：点击按钮追加空行', async () => {
    const model = reactive({ FOO: 'bar' })
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.findAll('input').length).toBe(2)
    await wrapper.findAll('button').find(b => b.text().includes('新增')).trigger('click')
    expect(wrapper.findAll('input').length).toBe(4) // +1 行 = 2 input
  })

  it('删除行：splice 后 syncToModel 回写 map', async () => {
    const model = reactive({ FOO: 'bar', BAZ: 'qux' })
    wrapper = createWrapper(model)
    await nextTick()
    // 点第一行的删除按钮
    await wrapper.findAll('button').find(b => b.text() === '删除').trigger('click')
    await nextTick()
    // FOO 被删除，只剩 BAZ
    expect(model.FOO).toBeUndefined()
    expect(model.BAZ).toBe('qux')
  })

  it('改键后 syncToModel 重建 map（删旧键加新键）', async () => {
    const model = reactive({ OLD: 'val' })
    wrapper = createWrapper(model)
    await nextTick()
    // 改第一行的 key
    const keyInput = wrapper.findAll('input')[0]
    await keyInput.setValue('NEW')
    await keyInput.trigger('change')
    await nextTick()
    expect(model.OLD).toBeUndefined()
    expect(model.NEW).toBe('val')
  })

  it('改值后 syncToModel 回写新值', async () => {
    const model = reactive({ FOO: 'bar' })
    wrapper = createWrapper(model)
    await nextTick()
    const valInput = wrapper.findAll('input')[1]
    await valInput.setValue('updated')
    await valInput.trigger('change')
    await nextTick()
    expect(model.FOO).toBe('updated')
  })

  it('空键名的行不写入 model', async () => {
    const model = reactive({ FOO: 'bar' })
    wrapper = createWrapper(model)
    await nextTick()
    // 新增一行（空键），改值，syncToModel 应忽略空键行
    await wrapper.findAll('button').find(b => b.text().includes('新增')).trigger('click')
    const inputs = wrapper.findAll('input')
    // 新行 value input（index 3）
    await inputs[3].setValue('orphan')
    await inputs[3].trigger('change')
    await nextTick()
    // model 仍只有 FOO
    expect(Object.keys(model).length).toBe(1)
    expect(model.FOO).toBe('bar')
  })
})
