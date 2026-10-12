import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import JsonYamlToolDialog from '../JsonYamlToolDialog.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="jt-dialog"><slot name="header" /><slot /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue']
  },
  'el-select': {
    // 数值选项（缩进 2/4）转 Number 回传——字符串缩进会被 JSON.stringify 逐层重复
    template: '<select :class="$attrs.class" :value="modelValue" @change="$emit(\'update:modelValue\', coerce($event.target.value))"><slot /></select>',
    props: ['modelValue', 'size'],
    emits: ['update:modelValue'],
    methods: {
      coerce(v) {
        const n = Number(v)
        return Number.isNaN(n) ? v : n
      }
    }
  },
  'el-option': { template: '<option :value="value">{{ label }}</option>', props: ['label', 'value'] },
  'el-input': {
    template: '<textarea :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'type', 'rows', 'placeholder'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'size', 'type', 'link'],
    emits: ['click']
  }
}

const createWrapper = (visible = true) =>
  mount(JsonYamlToolDialog, {
    props: { modelValue: visible },
    global: { stubs }
  })

const getTextarea = (wrapper) => wrapper.find('textarea')
const getInput = (wrapper) => getTextarea(wrapper).element.value
const setText = async (wrapper, text) => {
  await getTextarea(wrapper).setValue(text)
}
const clickButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text() === text).trigger('click')

describe('JsonYamlToolDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('默认 JSON 类型渲染缩进选择与「转 YAML」按钮', () => {
    const wrapper = createWrapper()
    expect(wrapper.find('.jt-indent-select').exists()).toBe(true)
    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons).toContain('校验')
    expect(buttons).toContain('格式化')
    expect(buttons).toContain('压缩')
    expect(buttons).toContain('转 YAML')
    expect(buttons).not.toContain('转 JSON')
  })

  it('切换 YAML 类型后隐藏缩进选择并换为「转 JSON」按钮', async () => {
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    expect(wrapper.find('.jt-indent-select').exists()).toBe(false)
    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons).toContain('转 JSON')
    expect(buttons).not.toContain('转 YAML')
  })

  it('JSON 校验通过展示成功结论', async () => {
    const wrapper = createWrapper()
    await setText(wrapper, '{"a":1}')
    await clickButton(wrapper, '校验')
    expect(wrapper.find('.jt-result-ok').text()).toContain('校验通过')
  })

  it('JSON 校验失败展示错误与行号（position 换算）', async () => {
    const wrapper = createWrapper()
    // 尾逗号非法：JSON.parse 报错 position 落在第 2 行的 `}`
    await setText(wrapper, '{\n"a": 1,\n}')
    await clickButton(wrapper, '校验')
    const err = wrapper.find('.jt-result-err')
    expect(err.exists()).toBe(true)
    expect(err.text()).toMatch(/第 \d+ 行/)
  })

  it('YAML 校验失败展示 js-yaml 错误行号', async () => {
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    await setText(wrapper, 'a: 1\n b: 2')
    await clickButton(wrapper, '校验')
    const err = wrapper.find('.jt-result-err')
    expect(err.exists()).toBe(true)
    expect(err.text()).toMatch(/第 2 行/)
  })

  it('空内容校验提示先粘贴文本', async () => {
    const wrapper = createWrapper()
    await setText(wrapper, '   ')
    await clickButton(wrapper, '校验')
    expect(wrapper.find('.jt-result-err').text()).toContain('内容为空')
  })

  it('JSON 格式化按选定缩进输出', async () => {
    const wrapper = createWrapper()
    await setText(wrapper, '{"a":[1,2]}')
    await wrapper.find('.jt-indent-select').setValue('4')
    await clickButton(wrapper, '格式化')
    expect(getInput(wrapper)).toBe('{\n    "a": [\n        1,\n        2\n    ]\n}')
  })

  it('YAML 格式化输出标准 dump', async () => {
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    await setText(wrapper, 'a:   1\nb: "x"')
    await clickButton(wrapper, '格式化')
    expect(getInput(wrapper)).toBe('a: 1\nb: x\n')
  })

  it('压缩输出无空白 JSON（YAML 类型同样按 JSON 序列化）', async () => {
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    await setText(wrapper, 'a: 1\nb: 2')
    await clickButton(wrapper, '压缩')
    expect(getInput(wrapper)).toBe('{"a":1,"b":2}')
  })

  it('JSON 转 YAML：内容转换且类型切到 YAML', async () => {
    const wrapper = createWrapper()
    await setText(wrapper, '{"a":1,"b":"x"}')
    await clickButton(wrapper, '转 YAML')
    expect(getInput(wrapper)).toBe('a: 1\nb: x\n')
    // 类型已切换：出现「转 JSON」按钮
    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons).toContain('转 JSON')
  })

  it('YAML 转 JSON：内容转换且类型切到 JSON', async () => {
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    await setText(wrapper, 'a: 1')
    await clickButton(wrapper, '转 JSON')
    expect(getInput(wrapper)).toBe('{\n  "a": 1\n}')
    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons).toContain('转 YAML')
  })

  it('YAML 多文档拒绝转换并说明原因（不静默取首文档）', async () => {
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    await setText(wrapper, 'a: 1\n---\nb: 2')
    await clickButton(wrapper, '转 JSON')
    expect(wrapper.find('.jt-result-err').text()).toContain('2 个文档')
  })

  it('YAML 含注释时转 JSON 前弹出有损确认', async () => {
    const { ElMessageBox } = await import('element-plus')
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    await setText(wrapper, '# 顶部注释\na: 1')
    await clickButton(wrapper, '转 JSON')
    await flushPromises()
    expect(ElMessageBox.confirm).toHaveBeenCalled()
    const [message] = ElMessageBox.confirm.mock.calls[0]
    expect(message).toContain('注释')
    expect(message).toContain('丢失')
  })

  it('YAML 含锚点别名时转 JSON 前弹出有损确认（展开语义）', async () => {
    const { ElMessageBox } = await import('element-plus')
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    await setText(wrapper, 'base: &b\n  x: 1\nuse: *b')
    await clickButton(wrapper, '转 JSON')
    await flushPromises()
    const [message] = ElMessageBox.confirm.mock.calls[0]
    expect(message).toContain('锚点')
  })

  it('有损确认后继续完成转换', async () => {
    const { ElMessageBox } = await import('element-plus')
    ElMessageBox.confirm.mockResolvedValueOnce()
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    await setText(wrapper, '# 注释\na: 1')
    await clickButton(wrapper, '转 JSON')
    await flushPromises()
    expect(getInput(wrapper)).toBe('{\n  "a": 1\n}')
  })

  it('无注释无锚点的 YAML 转 JSON 不弹确认', async () => {
    const { ElMessageBox } = await import('element-plus')
    const wrapper = createWrapper()
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    await setText(wrapper, 'a: 1')
    await clickButton(wrapper, '转 JSON')
    await flushPromises()
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
  })

  it('切换格式类型清空上一结论', async () => {
    const wrapper = createWrapper()
    await setText(wrapper, '{"a":1}')
    await clickButton(wrapper, '校验')
    expect(wrapper.find('.jt-result').exists()).toBe(true)
    await wrapper.findAll('select')[0].setValue('yaml')
    await flushPromises()
    expect(wrapper.find('.jt-result').exists()).toBe(false)
  })
})
