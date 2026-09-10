import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import SettingsPanel from '../SettingsPanel.vue'
import { useUiStore, useSettingsStore, DEFAULTS } from '../../store'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetSettings: vi.fn(),
  SaveSettings: vi.fn(),
  GetAppVersion: vi.fn(),
  CheckForUpdate: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  WarningFilled: { template: '<i class="i-warn" />' },
  Key: { template: '<i class="i-key" />' }
}))

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="el-dialog"><slot /></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'closeOnPressEscape', 'class', 'append'],
    emits: ['update:modelValue']
  },
  'el-switch': {
    template: '<input type="checkbox" :checked="modelValue" @change="onChange" />',
    props: ['modelValue', 'activeText', 'inactiveText'],
    emits: ['update:modelValue', 'change'],
    methods: {
      onChange(e) {
        const v = e.target.checked
        this.$emit('update:modelValue', v)
        this.$emit('change', v)
      }
    }
  },
  'el-input': {
    template: '<input :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" @change="$emit(\'change\', $event.target.value)" @keyup="$emit(\'keyup\', $event)" />',
    props: ['modelValue', 'placeholder', 'size', 'type'],
    emits: ['update:modelValue', 'change', 'keyup']
  },
  'el-select': {
    template: '<select :value="modelValue" @change="onChange"><slot /></select>',
    props: ['modelValue', 'size'],
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
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'loading', 'disabled', 'text'],
    emits: ['click']
  },
  'el-icon': { template: '<i><slot /></i>', props: ['size'] },
  'el-tag': {
    template: '<span class="el-tag"><slot /><span class="tag-close" @click.stop="$emit(\'close\')" /></span>',
    props: ['closable', 'size'],
    emits: ['close']
  },
  // el-radio-group + el-radio stub：用 provide/inject 传递选中态，
  // 原生 radio change 时由 group 向父组件 emit update:modelValue + change
  'el-radio-group': {
    template: '<div class="el-radio-group" :data-model="modelValue"><slot /></div>',
    props: ['modelValue'],
    emits: ['update:modelValue', 'change'],
    provide() {
      return { elRadioGroup: this }
    }
  },
  'el-radio': {
    template: '<label class="el-radio"><input type="radio" :value="value" :checked="isChecked" @change="onChange" /><span><slot /></span></label>',
    props: ['value'],
    inject: { elRadioGroup: { default: null } },
    computed: {
      isChecked() {
        return this.elRadioGroup && this.elRadioGroup.modelValue === this.value
      }
    },
    methods: {
      onChange() {
        if (this.elRadioGroup) {
          this.elRadioGroup.$emit('update:modelValue', this.value)
          this.elRadioGroup.$emit('change', this.value)
        }
      }
    }
  }
}

const baseSettings = (over = {}) => ({
  gpuDisabled: false,
  defaultShell: 'powershell',
  gitBashPath: 'C:\\Program Files\\Git\\bin\\bash.exe',
  wslDistro: '',
  obsidianPath: '',
  searchExcludeDirs: ['node_modules'],
  searchExcludeFiles: ['.log'],
  shortcutCommandPalette: DEFAULTS.commandPalette,
  shortcutToggleTerminal: DEFAULTS.toggleTerminal,
  shortcutRename: DEFAULTS.rename,
  shortcutDelete: DEFAULTS.delete,
  ...over
})

async function createWrapper(settingsOver = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const { GetSettings, GetAppVersion } = await import('../../../wailsjs/go/main/App')
  GetSettings.mockResolvedValue(baseSettings(settingsOver))
  GetAppVersion.mockResolvedValue('1.0.0')
  const uiStore = useUiStore()
  uiStore.settingsVisible = true
  const wrapper = mount(SettingsPanel, { global: { stubs, plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

describe('SettingsPanel.vue', () => {
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

  it('挂载时加载设置 + 版本号，渲染 4 个导航 tab', async () => {
    wrapper = await createWrapper()
    expect(wrapper.findAll('.settings-nav-item').length).toBe(4)
    expect(wrapper.text()).toContain('v1.0.0')
  })

  it('切换到终端 tab', async () => {
    wrapper = await createWrapper()
    const terminalTab = wrapper.findAll('.settings-nav-item')[1]
    await terminalTab.trigger('click')
    expect(terminalTab.classes()).toContain('is-active')
    expect(wrapper.text()).toContain('默认 Shell')
  })

  it('切换到搜索 tab', async () => {
    wrapper = await createWrapper({ searchExcludeDirs: ['node_modules', 'dist'], searchExcludeFiles: ['.log'] })
    const searchTab = wrapper.findAll('.settings-nav-item')[2]
    await searchTab.trigger('click')
    expect(wrapper.findAll('.el-tag').length).toBeGreaterThanOrEqual(2)
  })

  it('切换到快捷键 tab 渲染可自定义 + 固定快捷键', async () => {
    wrapper = await createWrapper()
    const shortcutsTab = wrapper.findAll('.settings-nav-item')[3]
    await shortcutsTab.trigger('click')
    // 4 可自定义 + 4 固定
    expect(wrapper.findAll('.shortcut-item').length).toBe(8)
  })

  it('loadSettings 失败时 gpuEnabled 回退 true', async () => {
    const { GetSettings, GetAppVersion } = await import('../../../wailsjs/go/main/App')
    GetSettings.mockRejectedValue(new Error('fail'))
    GetAppVersion.mockResolvedValue('1.0.0')
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.settingsVisible = true
    wrapper = mount(SettingsPanel, { global: { stubs, plugins: [pinia] } })
    await flushPromises()
    // gpuEnabled 回退 true（catch 分支）
    const sw = wrapper.find('input[type="checkbox"]')
    expect(sw.element.checked).toBe(true)
  })

  it('GetAppVersion 失败时回退 dev', async () => {
    const { GetSettings, GetAppVersion } = await import('../../../wailsjs/go/main/App')
    GetSettings.mockResolvedValue(baseSettings())
    GetAppVersion.mockRejectedValue(new Error('ver fail'))
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.settingsVisible = true
    wrapper = mount(SettingsPanel, { global: { stubs, plugins: [pinia] } })
    await flushPromises()
    expect(wrapper.text()).toContain('vdev')
  })

  it('GPU 开关变更调用 SaveSettings 并提示需重启', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper()
    const sw = wrapper.find('input[type="checkbox"]')
    await sw.setValue(false)
    await sw.trigger('change')
    await flushPromises()
    expect(SaveSettings).toHaveBeenCalled()
    expect(wrapper.find('.settings-restart-hint').exists()).toBe(true)
    expect(wrapper.text()).toContain('需重启应用')
  })

  it('GPU 保存失败时回滚 gpuEnabled', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockRejectedValue(new Error('save fail'))
    wrapper = await createWrapper()
    const sw = wrapper.find('input[type="checkbox"]')
    // VTU setValue 对 checkbox 不改 checked，手动置 false 后触发 change
    sw.element.checked = false
    await sw.trigger('change')
    await flushPromises()
    await nextTick()
    // 回滚：gpuEnabled 恢复 true（!gpuEnabled.value = !false = true）
    expect(sw.element.checked).toBe(true)
    // 失败时不提示需重启
    expect(wrapper.find('.settings-restart-hint').exists()).toBe(false)
  })

  it('检查更新：无新版本时 success 提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { CheckForUpdate } = await import('../../../wailsjs/go/main/App')
    CheckForUpdate.mockResolvedValue({ hasUpdate: false, currentVer: '1.0.0' })
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('检查更新')).trigger('click')
    await flushPromises()
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('最新版本'))
  })

  it('检查更新：有新版本时 emit update-available', async () => {
    const { CheckForUpdate } = await import('../../../wailsjs/go/main/App')
    const info = { hasUpdate: true, latestVer: '1.1.0', currentVer: '1.0.0' }
    CheckForUpdate.mockResolvedValue(info)
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('检查更新')).trigger('click')
    await flushPromises()
    expect(wrapper.emitted('update-available')).toBeTruthy()
    expect(wrapper.emitted('update-available')[0]).toEqual([info])
  })

  it('检查更新：抛错时 error 提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { CheckForUpdate } = await import('../../../wailsjs/go/main/App')
    CheckForUpdate.mockRejectedValue(new Error('net fail'))
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('检查更新')).trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('net fail'))
  })

  it('添加排除目录：回车添加 + 去重', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper({ searchExcludeDirs: ['node_modules'] })
    // 切到搜索 tab
    await wrapper.findAll('.settings-nav-item')[2].trigger('click')
    // 搜索 tab 内的添加输入框（搜索 tab 第二个 settings-tags 的 input）
    const addInput = wrapper.findAll('.settings-tags input')[0]
    await addInput.setValue('dist')
    await addInput.trigger('keyup', { key: 'Enter' })
    await nextTick()
    expect(wrapper.text()).toContain('dist')
    // 重复添加不生效
    const addBtn = wrapper.findAll('.settings-tags button')[0]
    await addInput.setValue('dist')
    await addBtn.trigger('click')
    // 仍只有一个 dist tag
    expect(wrapper.text().match(/dist/g)?.length).toBe(1)
    expect(SaveSettings).toHaveBeenCalled()
  })

  it('删除排除目录 tag', async () => {
    wrapper = await createWrapper({ searchExcludeDirs: ['node_modules', 'dist'] })
    await wrapper.findAll('.settings-nav-item')[2].trigger('click')
    expect(wrapper.text()).toContain('node_modules')
    // 点第一个 tag 的 close
    await wrapper.findAll('.tag-close')[0].trigger('click')
    await nextTick()
    expect(wrapper.text()).not.toContain('node_modules')
  })

  it('添加排除文件 + 删除', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper({ searchExcludeFiles: ['.log'] })
    await wrapper.findAll('.settings-nav-item')[2].trigger('click')
    // 排除文件区的 input（第二个 settings-tags）
    const fileInput = wrapper.findAll('.settings-tags input')[1]
    await fileInput.setValue('.tmp')
    await fileInput.trigger('keyup', { key: 'Enter' })
    await nextTick()
    expect(wrapper.text()).toContain('.tmp')
    // 删除 .log
    const logTag = wrapper.findAll('.el-tag').find(t => t.text().includes('.log'))
    await logTag.find('.tag-close').trigger('click')
    await nextTick()
    expect(wrapper.text()).not.toContain('.log')
  })

  it('终端 tab：选 gitbash 展示 Git Bash 路径输入', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper()
    await wrapper.findAll('.settings-nav-item')[1].trigger('click')
    const select = wrapper.find('select')
    await select.setValue('gitbash')
    await nextTick()
    expect(wrapper.text()).toContain('Git Bash 路径')
  })

  it('终端 tab：选 wsl 展示 WSL 发行版输入', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper()
    await wrapper.findAll('.settings-nav-item')[1].trigger('click')
    await wrapper.find('select').setValue('wsl')
    await nextTick()
    expect(wrapper.text()).toContain('WSL 发行版')
  })

  it('obsidian 路径变更触发 SaveSettings', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper()
    // general tab 的 obsidian 输入框（placeholder 含 Obsidian）
    const obsInput = wrapper.findAll('input').find(i => i.attributes('placeholder')?.includes('Obsidian'))
    await obsInput.setValue('C:\\Obsidian.exe')
    await obsInput.trigger('change')
    await flushPromises()
    expect(SaveSettings).toHaveBeenCalled()
  })

  it('通用 tab 渲染主题单选（跟随系统/浅色/暗色）', async () => {
    wrapper = await createWrapper()
    expect(wrapper.text()).toContain('主题')
    expect(wrapper.text()).toContain('跟随系统')
    expect(wrapper.text()).toContain('浅色')
    expect(wrapper.text()).toContain('暗色')
  })

  it('主题单选绑定 settingsStore.themeMode（store 变化反映到选中态）', async () => {
    wrapper = await createWrapper()
    // createWrapper 内 setActivePinia 重置 pinia，须在其后取 store，确保与组件同实例
    const settingsStore = useSettingsStore()
    settingsStore.themeMode = 'dark'
    await nextTick()
    const group = wrapper.find('.el-radio-group')
    expect(group.exists()).toBe(true)
    // group stub 经 data-model 暴露当前 modelValue（v-model 绑定 store.themeMode）
    expect(group.attributes('data-model')).toBe('dark')
  })

  it('切换主题为 dark 触发 saveTheme（SaveSettings 被调用）', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper()
    const darkRadio = wrapper.find('input[type="radio"][value="dark"]')
    await darkRadio.setValue(true)
    await flushPromises()
    expect(SaveSettings).toHaveBeenCalled()
  })

  it('快捷键：点击可编辑区进入录制态', async () => {
    wrapper = await createWrapper()
    await wrapper.findAll('.settings-nav-item')[3].trigger('click')
    await wrapper.findAll('.shortcut-keys--editable')[0].trigger('click')
    await nextTick()
    expect(wrapper.find('.recording-hint').exists()).toBe(true)
  })

  it('快捷键：录制有效组合后写入 store', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper()
    const settingsStore = useSettingsStore()
    await wrapper.findAll('.settings-nav-item')[3].trigger('click')
    const editable = wrapper.findAll('.shortcut-keys--editable')[0]
    await editable.trigger('click')
    await nextTick()
    // 按 Ctrl+K（不与默认/固定冲突）
    await editable.trigger('keydown', { key: 'k', ctrlKey: true })
    await flushPromises()
    expect(settingsStore.shortcutCommandPalette).toBe('Ctrl+K')
  })

  it('快捷键：Escape 取消录制', async () => {
    wrapper = await createWrapper()
    await wrapper.findAll('.settings-nav-item')[3].trigger('click')
    const editable = wrapper.findAll('.shortcut-keys--editable')[0]
    await editable.trigger('click')
    await nextTick()
    expect(wrapper.find('.recording-hint').exists()).toBe(true)
    await editable.trigger('keydown', { key: 'Escape' })
    await nextTick()
    expect(wrapper.find('.recording-hint').exists()).toBe(false)
  })

  it('快捷键：与其他可自定义快捷键冲突时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = await createWrapper()
    await wrapper.findAll('.settings-nav-item')[3].trigger('click')
    const editable = wrapper.findAll('.shortcut-keys--editable')[0] // commandPalette
    await editable.trigger('click')
    await nextTick()
    // 按 Ctrl+`（toggleTerminal 默认）→ 冲突
    await editable.trigger('keydown', { key: '`', ctrlKey: true })
    await nextTick()
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('冲突'))
  })

  it('快捷键：与固定快捷键冲突时 warning', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = await createWrapper()
    await wrapper.findAll('.settings-nav-item')[3].trigger('click')
    const editable = wrapper.findAll('.shortcut-keys--editable')[0]
    await editable.trigger('click')
    await nextTick()
    // 按 Ctrl+C（固定）→ 冲突
    await editable.trigger('keydown', { key: 'c', ctrlKey: true })
    await nextTick()
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('固定快捷键'))
  })

  it('快捷键：重置单项恢复默认值', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper({ shortcutCommandPalette: 'Ctrl+K' })
    const settingsStore = useSettingsStore()
    await wrapper.findAll('.settings-nav-item')[3].trigger('click')
    // 非默认 → 展示"重置"按钮
    const resetBtn = wrapper.findAll('button').find(b => b.text() === '重置')
    expect(resetBtn).toBeTruthy()
    await resetBtn.trigger('click')
    expect(settingsStore.shortcutCommandPalette).toBe(DEFAULTS.commandPalette)
  })

  it('快捷键：默认快捷键不展示重置按钮', async () => {
    wrapper = await createWrapper()
    await wrapper.findAll('.settings-nav-item')[3].trigger('click')
    // 全默认 → 无"重置"按钮（单项）
    const resetBtns = wrapper.findAll('button').filter(b => b.text() === '重置')
    expect(resetBtns.length).toBe(0)
  })

  it('快捷键：重置所有恢复全部默认', async () => {
    const { SaveSettings } = await import('../../../wailsjs/go/main/App')
    SaveSettings.mockResolvedValue(true)
    wrapper = await createWrapper({
      shortcutCommandPalette: 'Ctrl+K',
      shortcutRename: 'F3'
    })
    const settingsStore = useSettingsStore()
    await wrapper.findAll('.settings-nav-item')[3].trigger('click')
    await wrapper.findAll('button').find(b => b.text().includes('重置所有')).trigger('click')
    expect(settingsStore.shortcutCommandPalette).toBe(DEFAULTS.commandPalette)
    expect(settingsStore.shortcutRename).toBe(DEFAULTS.rename)
  })
})
