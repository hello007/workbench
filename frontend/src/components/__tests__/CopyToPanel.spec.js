import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import CopyToPanel from '../CopyToPanel.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  CopyTo: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  Sort: { template: '<i class="i-sort" />' }
}))

const stubs = {
  'el-form': { template: '<form><slot /></form>', props: ['labelWidth'] },
  'el-form-item': { template: '<div class="el-form-item"><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<input type="text" :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" @keyup="$emit(\'keyup\', $event)" />',
    props: ['modelValue', 'placeholder', 'disabled', 'clearable'],
    emits: ['update:modelValue', 'keyup']
  },
  'el-checkbox': {
    template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
    props: ['modelValue', 'disabled'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /><slot name="icon" /></button>',
    props: ['type', 'size', 'text', 'loading', 'disabled'],
    emits: ['click']
  },
  'el-icon': { template: '<i><slot /></i>' }
}

const createWrapper = () => mount(CopyToPanel, { global: { stubs } })

// 填入源/目标/文件名（name 未传时跳过，让 watch 自动同步默认名）
async function fillInputs(wrapper, { source = 'D:\\src\\file.go', target = 'D:\\dst', name } = {}) {
  const inputs = wrapper.findAll('input[type="text"]')
  // 输入框顺序：原地址(0) 目标地址(1) 文件名(2)
  if (source !== undefined) await inputs[0].setValue(source)
  if (target !== undefined) await inputs[1].setValue(target)
  if (name !== undefined) await inputs[2].setValue(name)
  // 等待 sourcePath watch 触发默认名同步 + 重渲
  await flushPromises()
  return inputs
}

const clickButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text() === text).trigger('click')

describe('CopyToPanel', () => {
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

  it('渲染标题与三个输入框', () => {
    wrapper = createWrapper()
    expect(wrapper.find('.tool-panel-title').text()).toBe('拷贝到')
    expect(wrapper.findAll('input[type="text"]').length).toBe(3)
  })

  it('源路径变化时自动同步默认文件名', async () => {
    wrapper = createWrapper()
    const inputs = await fillInputs(wrapper, { source: 'D:\\proj\\main.go', target: 'D:\\dst' })
    expect(inputs[2].element.value).toBe('main.go')
  })

  it('用户自定义文件名不被源路径变化覆盖', async () => {
    wrapper = createWrapper()
    const inputs = await fillInputs(wrapper, { source: 'D:\\a.go', target: 'D:\\dst', name: 'custom.go' })
    // 改源路径，文件名应保持 custom.go（不等于旧默认 a.go）
    await inputs[0].setValue('D:\\b.go')
    await flushPromises()
    expect(wrapper.findAll('input[type="text"]')[2].element.value).toBe('custom.go')
  })

  it('copyToPreview：整目录拷贝展示 from → dst/name', async () => {
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\src', target: 'D:\\dst' })
    // 默认 wholeDir=true
    const preview = wrapper.find('.ct-preview')
    expect(preview.exists()).toBe(true)
    expect(preview.text()).toContain('D:/src')
    expect(preview.text()).toContain('D:/dst/src')
  })

  it('copyToPreview：非整目录展示 from/* → dst/*', async () => {
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\src', target: 'D:\\dst' })
    // 取消勾选包含文件夹本身
    await wrapper.find('input[type="checkbox"]').setValue(false)
    await flushPromises()
    const preview = wrapper.find('.ct-preview')
    expect(preview.text()).toContain('D:/src/*')
    expect(preview.text()).toContain('D:/dst/*')
  })

  it('互换按钮交换原地址与目标地址', async () => {
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\a.go', target: 'D:\\b.go' })
    await clickButton(wrapper, '互换')
    await flushPromises()
    const inputs = wrapper.findAll('input[type="text"]')
    expect(inputs[0].element.value).toBe('D:\\b.go')
    expect(inputs[1].element.value).toBe('D:\\a.go')
  })

  it('原地址为空时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: '', target: 'D:\\dst' })
    await clickButton(wrapper, '开始拷贝')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入原地址')
  })

  it('目标地址为空时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\a.go', target: '' })
    await clickButton(wrapper, '开始拷贝')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入目标地址')
  })

  it('文件名含非法字符时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\a.go', target: 'D:\\dst', name: 'a:b' })
    await clickButton(wrapper, '开始拷贝')
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('非法字符'))
  })

  it('文件名为 . 或 .. 时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\a.go', target: 'D:\\dst', name: '..' })
    await clickButton(wrapper, '开始拷贝')
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('非法字符'))
  })

  it('成功时 success 提示并重置表单', async () => {
    const { ElMessage } = await import('element-plus')
    const { CopyTo } = await import('../../../wailsjs/go/main/App')
    CopyTo.mockResolvedValue('')
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\a.go', target: 'D:\\dst' })
    await clickButton(wrapper, '开始拷贝')
    await flushPromises()
    expect(CopyTo).toHaveBeenCalled()
    expect(ElMessage.success).toHaveBeenCalledWith('拷贝成功')
    // 面板常驻无关闭语义：成功后重置表单（等价原弹窗「成功后关闭」的下一轮起点）
    expect(wrapper.findAll('input[type="text"]')[0].element.value).toBe('')
  })

  it('结果含附加提示时完整展示', async () => {
    const { ElMessage } = await import('element-plus')
    const { CopyTo } = await import('../../../wailsjs/go/main/App')
    CopyTo.mockResolvedValue('已忽略自定义名')
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\a.go', target: 'D:\\dst' })
    await clickButton(wrapper, '开始拷贝')
    await flushPromises()
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('已忽略自定义名'))
  })

  it('后端返回错误串时 error 提示且保留输入', async () => {
    const { ElMessage } = await import('element-plus')
    const { CopyTo } = await import('../../../wailsjs/go/main/App')
    CopyTo.mockResolvedValue('错误：源不存在')
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\a.go', target: 'D:\\dst' })
    await clickButton(wrapper, '开始拷贝')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('错误：源不存在')
    // 错误时不重置表单（用户可修正后重试）
    expect(wrapper.findAll('input[type="text"]')[0].element.value).toBe('D:\\a.go')
  })

  it('抛异常时 error 提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { CopyTo } = await import('../../../wailsjs/go/main/App')
    CopyTo.mockRejectedValue(new Error('boom'))
    wrapper = createWrapper()
    await fillInputs(wrapper, { source: 'D:\\a.go', target: 'D:\\dst' })
    await clickButton(wrapper, '开始拷贝')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
  })
})
