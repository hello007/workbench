import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PushResultDialog from '../PushResultDialog.vue'

// Mock element-plus 的 ElMessage（复制成功/失败分支断言）
vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

// stub element-plus 组件：el-dialog 按 modelValue 条件渲染并透传 slot，el-button 透传 click
const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="el-dialog"><slot name="header" /><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'append', 'destroyOnClose'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'icon', 'loading', 'disabled', 'text'],
    emits: ['click']
  },
  'el-icon': { template: '<i class="el-icon"><slot /></i>' }
}

function createWrapper(props = {}) {
  return mount(PushResultDialog, {
    props: { modelValue: true, output: '', ...props },
    global: { stubs }
  })
}

describe('PushResultDialog.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('modelValue=true 时渲染 Dialog 与完整 output 文本', () => {
    const output = 'To github.com/demo/demo-repo.git\n   main -> main'
    const wrapper = createWrapper({ output })
    expect(wrapper.find('.el-dialog').exists()).toBe(true)
    expect(wrapper.find('.push-result-output').text()).toBe(output)
  })

  it('modelValue=false 时不渲染 Dialog', () => {
    const wrapper = createWrapper({ modelValue: false })
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
  })

  it('点击复制按钮调 clipboard.writeText 传入完整 output 并提示成功', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const { ElMessage } = await import('element-plus')
    const output = 'To github.com/demo/demo-repo.git\n   main -> main\n   feature -> feature'
    const wrapper = createWrapper({ output })
    const buttons = wrapper.findAll('button')
    const copyBtn = buttons.find(b => b.text().includes('复制'))
    await copyBtn.trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledTimes(1)
    expect(writeText).toHaveBeenCalledWith(output)
    expect(ElMessage.success).toHaveBeenCalledWith('已复制到剪贴板')
  })

  it('clipboard.writeText 失败时提示复制失败', async () => {
    const writeText = vi.fn(() => Promise.reject(new Error('denied')))
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const { ElMessage } = await import('element-plus')
    const wrapper = createWrapper({ output: 'any output' })
    const copyBtn = wrapper.findAll('button').find(b => b.text().includes('复制'))
    await copyBtn.trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('复制失败')
  })

  it('点击关闭按钮 emit update:modelValue=false', async () => {
    const wrapper = createWrapper({ output: 'x' })
    const closeBtn = wrapper.findAll('button').find(b => b.text().includes('关闭'))
    await closeBtn.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeTruthy()
    expect(wrapper.emitted('update:modelValue')[0]).toEqual([false])
  })

  it('output 为空时仍渲染 Dialog（空 pre）', () => {
    const wrapper = createWrapper({ output: '' })
    expect(wrapper.find('.el-dialog').exists()).toBe(true)
    expect(wrapper.find('.push-result-output').text()).toBe('')
  })
})
