import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import TerminalPanel from '../TerminalPanel.vue'
import { useUiStore } from '../../store'

// Mock useTerminal（pty 终端本质依赖后端运行时，mock 为可控函数）
const terminalMock = {
  initTerminal: vi.fn(() => Promise.resolve()),
  changeDir: vi.fn(),
  resize: vi.fn(),
  focus: vi.fn(),
  destroyTerminal: vi.fn(() => Promise.resolve()),
  restartTerminal: vi.fn(() => Promise.resolve())
}
vi.mock('../../composables/useTerminal', async () => {
  const { ref } = await import('vue')
  return {
    useTerminal: () => ({
      isActive: ref(false),
      isExited: ref(false),
      currentDir: ref(''),
      ...terminalMock
    })
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetShellConfigs: vi.fn(),
  GetSettings: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  Folder: { template: '<i class="i-folder" />' },
  RefreshRight: { template: '<i class="i-refresh" />' }
}))

const stubs = {
  'el-select': {
    template: '<select :value="modelValue" @change="onChange"><slot /></select>',
    props: ['modelValue', 'size', 'class', 'popperClass'],
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
  'el-icon': { template: '<i><slot /></i>', props: ['size'] },
  'el-button': {
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /><slot name="icon" /></button>',
    props: ['type', 'size', 'text', 'loading'],
    emits: ['click']
  }
}

async function createWrapper(shellConfigsOver = null, settingsOver = null) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const { GetShellConfigs, GetSettings } = await import('../../../wailsjs/go/main/App')
  if (shellConfigsOver === null) {
    GetShellConfigs.mockResolvedValue([{ type: 'powershell', displayName: 'PowerShell' }, { type: 'cmd', displayName: 'CMD' }])
  } else if (shellConfigsOver === 'fail') {
    GetShellConfigs.mockRejectedValue(new Error('fail'))
  } else {
    GetShellConfigs.mockResolvedValue(shellConfigsOver)
  }
  if (settingsOver === null) {
    GetSettings.mockResolvedValue({ defaultShell: 'powershell' })
  } else if (settingsOver === 'fail') {
    GetSettings.mockRejectedValue(new Error('settings fail'))
  } else {
    GetSettings.mockResolvedValue(settingsOver)
  }
  const uiStore = useUiStore()
  uiStore.terminalVisible = false
  const wrapper = mount(TerminalPanel, { global: { stubs, plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

describe('TerminalPanel.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    Object.values(terminalMock).forEach(m => m.mockClear && m.mockClear())
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  it('挂载时加载 Shell 配置并渲染选项', async () => {
    wrapper = await createWrapper()
    expect(wrapper.findAll('option').length).toBe(2)
    expect(wrapper.text()).toContain('PowerShell')
  })

  it('GetShellConfigs 失败时回退默认 4 种 Shell', async () => {
    wrapper = await createWrapper('fail')
    expect(wrapper.findAll('option').length).toBe(4)
  })

  it('GetSettings 返回 defaultShell 时同步 shellType', async () => {
    wrapper = await createWrapper(null, { defaultShell: 'gitbash' })
    expect(wrapper.vm.$.setupState.shellType).toBe('gitbash')
  })

  it('GetSettings 失败时保持默认 powershell', async () => {
    wrapper = await createWrapper(null, 'fail')
    expect(wrapper.vm.$.setupState.shellType).toBe('powershell')
  })

  it('切换 Shell 类型调用 restartTerminal', async () => {
    wrapper = await createWrapper()
    const select = wrapper.find('select')
    await select.setValue('cmd')
    await flushPromises()
    expect(terminalMock.restartTerminal).toHaveBeenCalled()
  })

  it('isExited 时展示重新启动按钮，点击调用 restartTerminal', async () => {
    wrapper = await createWrapper()
    wrapper.vm.$.setupState.isExited = true
    await nextTick()
    const restartBtn = wrapper.findAll('button').find(b => b.text().includes('重新启动'))
    expect(restartBtn).toBeTruthy()
    await restartBtn.trigger('click')
    await flushPromises()
    expect(terminalMock.restartTerminal).toHaveBeenCalled()
  })

  it('点击收起按钮 emit toggle', async () => {
    wrapper = await createWrapper()
    await wrapper.find('.minimize-btn').trigger('click')
    expect(wrapper.emitted('toggle')).toBeTruthy()
  })

  it('terminalVisible 首次为 true 时初始化终端', async () => {
    wrapper = await createWrapper()
    const uiStore = useUiStore()
    uiStore.terminalDir = 'D:\\proj'
    uiStore.terminalVisible = true
    await nextTick()
    await flushPromises()
    expect(terminalMock.initTerminal).toHaveBeenCalledWith(
      expect.any(Object),
      'D:\\proj',
      'powershell'
    )
    expect(terminalMock.focus).toHaveBeenCalled()
  })

  it('terminalDir 变化且终端激活时调用 changeDir', async () => {
    wrapper = await createWrapper()
    const uiStore = useUiStore()
    // 先激活终端
    uiStore.terminalVisible = true
    await nextTick()
    await flushPromises()
    wrapper.vm.$.setupState.isActive = true
    uiStore.terminalDir = 'D:\\new'
    await nextTick()
    await flushPromises()
    expect(terminalMock.changeDir).toHaveBeenCalledWith('D:\\new')
  })

  it('卸载时调用 destroyTerminal', async () => {
    wrapper = await createWrapper()
    wrapper.unmount()
    wrapper = null
    await flushPromises()
    expect(terminalMock.destroyTerminal).toHaveBeenCalled()
  })
})
