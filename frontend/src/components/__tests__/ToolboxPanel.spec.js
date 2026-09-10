import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import ToolboxPanel from '../ToolboxPanel.vue'

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
  CopyDocument: { template: '<i class="i-copy" />' },
  SetUp: { template: '<i class="i-setup" />' },
  Sort: { template: '<i class="i-sort" />' }
}))

const stubs = {
  'el-icon': { template: '<i><slot /></i>', props: ['size'] },
  'el-dialog': {
    template: '<div v-if="modelValue" class="el-dialog"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'append'],
    emits: ['update:modelValue']
  },
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
  }
}

function createWrapper() {
  return mount(ToolboxPanel, { global: { stubs } })
}

// 打开拷贝到对话框并填入源/目标/文件名（name 未传时跳过，让 watch 自动同步默认名）
async function openCopyToDialog(wrapper, { source = 'D:\\src\\file.go', target = 'D:\\dst', name } = {}) {
  await wrapper.findAll('.toolbox-item')[0].trigger('click')
  await nextTick()
  const inputs = wrapper.find('.el-dialog').findAll('input')
  // 输入框顺序：原地址(0) 目标地址(1) 文件名(2)；checkbox 为 (3)
  const textInputs = inputs.filter(i => i.element.type !== 'checkbox')
  if (source !== undefined) await inputs[0].setValue(source)
  if (target !== undefined) await inputs[1].setValue(target)
  if (name !== undefined) await inputs[2].setValue(name)
  // 等待 copyToSourcePath watch 触发默认名同步 + 重渲
  await flushPromises()
  return inputs
}

describe('ToolboxPanel', () => {
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

  it('渲染标题栏与工具项', () => {
    wrapper = createWrapper()
    expect(wrapper.find('.toolbox-header').text()).toContain('工具箱')
    expect(wrapper.findAll('.toolbox-item').length).toBe(1)
    expect(wrapper.text()).toContain('拷贝到')
  })

  it('点击关闭按钮 emit close', async () => {
    wrapper = createWrapper()
    await wrapper.find('.toolbox-close').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('点击拷贝到工具项打开对话框', async () => {
    wrapper = createWrapper()
    await wrapper.findAll('.toolbox-item')[0].trigger('click')
    await nextTick()
    expect(wrapper.find('.el-dialog').exists()).toBe(true)
  })

  it('源路径变化时自动同步默认文件名', async () => {
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\proj\\main.go', target: 'D:\\dst' })
    const nameInput = wrapper.find('.el-dialog').findAll('input[type="text"]')[2]
    expect(nameInput.element.value).toBe('main.go')
  })

  it('用户自定义文件名不被源路径变化覆盖', async () => {
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\a.go', target: 'D:\\dst', name: 'custom.go' })
    const inputs = wrapper.find('.el-dialog').findAll('input[type="text"]')
    // 改源路径，文件名应保持 custom.go（不等于旧默认 a.go）
    await inputs[0].setValue('D:\\b.go')
    await nextTick()
    expect(inputs[2].element.value).toBe('custom.go')
  })

  it('copyToPreview：整目录拷贝展示 from → dst/name', async () => {
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\src', target: 'D:\\dst' })
    // 默认 copyToWholeDir=true
    const preview = wrapper.find('.copy-to-preview')
    expect(preview.exists()).toBe(true)
    expect(preview.text()).toContain('D:/src')
    expect(preview.text()).toContain('D:/dst/src')
  })

  it('copyToPreview：非整目录展示 from/* → dst/*', async () => {
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\src', target: 'D:\\dst' })
    // 取消勾选包含文件夹本身
    const checkbox = wrapper.find('.el-dialog input[type="checkbox"]')
    await checkbox.setValue(false)
    await nextTick()
    const preview = wrapper.find('.copy-to-preview')
    expect(preview.text()).toContain('D:/src/*')
    expect(preview.text()).toContain('D:/dst/*')
  })

  it('互换按钮交换原地址与目标地址', async () => {
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\a.go', target: 'D:\\b.go' })
    const swapBtn = wrapper.findAll('button').find(b => b.text().includes('互换'))
    await swapBtn.trigger('click')
    await nextTick()
    const inputs = wrapper.find('.el-dialog').findAll('input[type="text"]')
    expect(inputs[0].element.value).toBe('D:\\b.go')
    expect(inputs[1].element.value).toBe('D:\\a.go')
  })

  it('handleCopyTo：原地址为空时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: '', target: 'D:\\dst' })
    await wrapper.findAll('button').find(b => b.text() === '确定').trigger('click')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入原地址')
  })

  it('handleCopyTo：目标地址为空时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\a.go', target: '' })
    await wrapper.findAll('button').find(b => b.text() === '确定').trigger('click')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入目标地址')
  })

  it('handleCopyTo：文件名含非法字符时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\a.go', target: 'D:\\dst', name: 'a:b' })
    await wrapper.findAll('button').find(b => b.text() === '确定').trigger('click')
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('非法字符'))
  })

  it('handleCopyTo：文件名为 . 或 .. 时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\a.go', target: 'D:\\dst', name: '..' })
    await wrapper.findAll('button').find(b => b.text() === '确定').trigger('click')
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('非法字符'))
  })

  it('handleCopyTo：成功时 success 提示并关闭对话框', async () => {
    const { ElMessage } = await import('element-plus')
    const { CopyTo } = await import('../../../wailsjs/go/main/App')
    CopyTo.mockResolvedValue('')
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\a.go', target: 'D:\\dst' })
    await wrapper.findAll('button').find(b => b.text() === '确定').trigger('click')
    await flushPromises()
    expect(CopyTo).toHaveBeenCalled()
    expect(ElMessage.success).toHaveBeenCalledWith('拷贝成功')
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
  })

  it('handleCopyTo：结果含附加提示时完整展示', async () => {
    const { ElMessage } = await import('element-plus')
    const { CopyTo } = await import('../../../wailsjs/go/main/App')
    CopyTo.mockResolvedValue('已忽略自定义名')
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\a.go', target: 'D:\\dst' })
    await wrapper.findAll('button').find(b => b.text() === '确定').trigger('click')
    await flushPromises()
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('已忽略自定义名'))
  })

  it('handleCopyTo：后端返回错误串时 error 提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { CopyTo } = await import('../../../wailsjs/go/main/App')
    CopyTo.mockResolvedValue('错误：源不存在')
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\a.go', target: 'D:\\dst' })
    await wrapper.findAll('button').find(b => b.text() === '确定').trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('错误：源不存在')
    // 错误时不关闭对话框
    expect(wrapper.find('.el-dialog').exists()).toBe(true)
  })

  it('handleCopyTo：抛异常时 error 提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { CopyTo } = await import('../../../wailsjs/go/main/App')
    CopyTo.mockRejectedValue(new Error('boom'))
    wrapper = createWrapper()
    await openCopyToDialog(wrapper, { source: 'D:\\a.go', target: 'D:\\dst' })
    await wrapper.findAll('button').find(b => b.text() === '确定').trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
  })

  it('取消按钮关闭对话框', async () => {
    wrapper = createWrapper()
    await openCopyToDialog(wrapper)
    await wrapper.findAll('button').find(b => b.text() === '取消').trigger('click')
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
  })
})
