import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { reactive } from 'vue'
import ParamsEditor from '../ParamsEditor.vue'

const stubs = {
  'el-form': { template: '<form @submit.prevent><slot /></form>', props: ['labelWidth', 'size'] },
  // el-form-item 渲染 label 文本，便于按 label 断言分支
  'el-form-item': { template: '<div class="el-form-item"><span class="form-label">{{ label }}</span><slot /></div>', props: ['label'] },
  'el-select': {
    template: '<select :value="modelValue" @change="onChange"><slot /></select>',
    props: ['modelValue', 'class'],
    emits: ['update:modelValue', 'change'],
    methods: {
      onChange(e) {
        const v = e.target.value
        this.$emit('update:modelValue', v)
        this.$emit('change', v)
      }
    }
  },
  'el-option': { template: '<option :value="value">{{ label }}</option>', props: ['label', 'value'] },
  'el-input': {
    template: '<input :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'type', 'autosize'],
    emits: ['update:modelValue']
  },
  'el-checkbox': {
    template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
    props: ['modelValue'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'link'],
    emits: ['click']
  }
}

function createWrapper(model) {
  return mount(ParamsEditor, {
    props: { modelValue: model },
    global: { stubs }
  })
}

// defineModel 写操作（model.value = x）经 emit update:modelValue 回传，读 setupState.model 仍是旧 prop。
// 故写操作断言走 emitted('update:modelValue')，读操作断言走 setupState（auto-unwrapped）。
const lastEmittedModel = (wrapper) => {
  const emits = wrapper.emitted('update:modelValue')
  return emits ? emits[emits.length - 1][0] : null
}

describe('ParamsEditor.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
  })

  it('model 为 null 时 type 默认 none + 渲染无参数提示', () => {
    wrapper = createWrapper(null)
    expect(wrapper.vm.$.setupState.type).toBe('none')
    expect(wrapper.find('.none-hint').exists()).toBe(true)
  })

  it('file 类型渲染标题/起始目录/扩展名 label', () => {
    wrapper = createWrapper(reactive({ type: 'file', label: '', textFieldKey: '', startDir: '', extensions: ['.md'] }))
    expect(wrapper.vm.$.setupState.type).toBe('file')
    expect(wrapper.findAll('.form-label').some(el => el.text().includes('起始目录'))).toBe(true)
    expect(wrapper.findAll('.form-label').some(el => el.text().includes('扩展名'))).toBe(true)
  })

  it('text 类型渲染参数键名 label', () => {
    wrapper = createWrapper(reactive({ type: 'text', label: '参数' }))
    expect(wrapper.vm.$.setupState.type).toBe('text')
    expect(wrapper.findAll('.form-label').some(el => el.text().includes('参数键名'))).toBe(true)
  })

  it('form 类型渲染模板/字段列表 label', () => {
    wrapper = createWrapper(reactive({ type: 'form', promptTemplate: '', fields: [] }))
    expect(wrapper.vm.$.setupState.type).toBe('form')
    expect(wrapper.findAll('.form-label').some(el => el.text().includes('模板'))).toBe(true)
    expect(wrapper.findAll('.form-label').some(el => el.text().includes('字段列表'))).toBe(true)
  })

  it('onTypeChange：null 时 emit 新建对象', () => {
    wrapper = createWrapper(null)
    wrapper.vm.$.setupState.onTypeChange('form')
    expect(lastEmittedModel(wrapper)).toEqual({ type: 'form' })
  })

  it('onTypeChange：已有 model 时 emit spread 保留其他字段', () => {
    const model = reactive({ type: 'text', label: '保留', textFieldKey: 'k' })
    wrapper = createWrapper(model)
    wrapper.vm.$.setupState.onTypeChange('form')
    const emitted = lastEmittedModel(wrapper)
    expect(emitted.type).toBe('form')
    expect(emitted.label).toBe('保留')
    expect(emitted.textFieldKey).toBe('k')
  })

  it('extensionsText get/set 逗号互转', () => {
    const model = reactive({ type: 'file', extensions: ['.md', '.docx'] })
    wrapper = createWrapper(model)
    const ss = wrapper.vm.$.setupState
    expect(ss.extensionsText).toBe('.md, .docx')
    ss.extensionsText = '.md, .txt'
    expect(lastEmittedModel(wrapper).extensions).toEqual(['.md', '.txt'])
  })

  it('extensionsText：model 为 null 时 set 不报错', () => {
    wrapper = createWrapper(null)
    wrapper.vm.$.setupState.extensionsText = '.md'
    // 不 emit（model.value 为 null，setter return）
    expect(lastEmittedModel(wrapper)).toBeNull()
  })

  it('addField emit 追加空字段', () => {
    const model = reactive({ type: 'form', fields: [] })
    wrapper = createWrapper(model)
    wrapper.vm.$.setupState.addField()
    const emitted = lastEmittedModel(wrapper)
    expect(emitted.fields.length).toBe(1)
    expect(emitted.fields[0].type).toBe('text')
  })

  it('removeField emit 删除指定索引', () => {
    const model = reactive({ type: 'form', fields: [{ key: 'a' }, { key: 'b' }] })
    wrapper = createWrapper(model)
    wrapper.vm.$.setupState.removeField(0)
    const emitted = lastEmittedModel(wrapper)
    expect(emitted.fields.length).toBe(1)
    expect(emitted.fields[0].key).toBe('b')
  })

  it('placeholderWarnings：模板占位符无对应字段时告警', () => {
    const model = reactive({ type: 'form', promptTemplate: 'hello {{name}}', fields: [{ key: 'other' }] })
    wrapper = createWrapper(model)
    expect(wrapper.vm.placeholderWarnings).toEqual(expect.arrayContaining([
      expect.stringContaining('name'),
      expect.stringContaining('other')
    ]))
  })

  it('placeholderWarnings：字段与模板一致时无告警', () => {
    const model = reactive({ type: 'form', promptTemplate: '{{name}}', fields: [{ key: 'name' }] })
    wrapper = createWrapper(model)
    expect(wrapper.vm.placeholderWarnings.length).toBe(0)
  })

  it('placeholderWarnings：非 form 类型无告警', () => {
    const model = reactive({ type: 'text', promptTemplate: '{{x}}' })
    wrapper = createWrapper(model)
    expect(wrapper.vm.placeholderWarnings.length).toBe(0)
  })

  it('UI：点新增字段按钮追加', async () => {
    const model = reactive({ type: 'form', fields: [] })
    wrapper = createWrapper(model)
    await wrapper.findAll('button').find(b => b.text().includes('新增')).trigger('click')
    expect(lastEmittedModel(wrapper).fields.length).toBe(1)
  })

  it('UI：点字段删除按钮移除', async () => {
    const model = reactive({ type: 'form', fields: [{ key: 'a' }, { key: 'b' }] })
    wrapper = createWrapper(model)
    const delBtn = wrapper.findAll('button').find(b => b.text() === '删除')
    await delBtn.trigger('click')
    expect(lastEmittedModel(wrapper).fields.length).toBe(1)
  })
})
