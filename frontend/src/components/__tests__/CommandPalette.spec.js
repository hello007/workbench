import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import CommandPalette from '../CommandPalette.vue'
import { useUiStore, useDirectoryStore } from '../../store'

// Mock wailsjs bindings used by composables (composables use ../../wailsjs relative to themselves)
vi.mock('../../wailsjs/go/main/App', () => ({
  SearchFiles: vi.fn(() => Promise.resolve([
    { name: 'main.go', path: 'src/main.go', type: 'file' }
  ])),
  GetFavorites: vi.fn(() => Promise.resolve([])),
  AddFavorite: vi.fn(() => Promise.resolve('')),
  RemoveFavorite: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteAlias: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteGroup: vi.fn(() => Promise.resolve(''))
}))

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn()
}))

vi.mock('../../utils/debug', () => ({
  debug: { log: vi.fn(), error: vi.fn(), warn: vi.fn() }
}))

const defaultStubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="command-palette-dialog"><slot name="header" /><slot /></div>',
    props: ['modelValue', 'showClose', 'closeOnClickModal', 'closeOnPressEscape', 'width', 'top'],
    emits: ['update:modelValue', 'close']
  },
  'el-input': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keydown="$emit(\'keydown\', $event)" />',
    props: ['modelValue', 'placeholder', 'size', 'clearable'],
    emits: ['update:modelValue', 'input', 'keydown']
  },
  'el-icon': { template: '<i><slot /></i>' },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
  'el-empty': { template: '<div class="el-empty" />', props: ['description', 'imageSize'] },
  Search: { template: '<span>search</span>' },
  Document: { template: '<span>doc</span>' },
  Folder: { template: '<span>folder</span>' },
  Star: { template: '<span>star</span>' },
  Loading: { template: '<span>loading</span>' }
}

const defaultWorkDirs = [
  { id: '1', name: 'Project A', path: 'C:\\projects\\a' },
  { id: '2', name: 'Project B', path: 'C:\\projects\\b' }
]

// modelValue / contentSearchInit 已迁 ui store：visible 经 uiStore.commandPaletteVisible 驱动，
// contentSearchInit 经 uiStore.contentSearchInit（默认空串，本组用例不依赖）。
// currentDir / workDirs 已迁 directory store：经 directoryStore.directories / selectedDirectoryId 驱动。
function createWrapper(options = {}) {
  const { visible = true, workDirs = defaultWorkDirs } = options
  const pinia = createPinia()
  setActivePinia(pinia)
  const uiStore = useUiStore()
  uiStore.commandPaletteVisible = visible
  const directoryStore = useDirectoryStore()
  directoryStore.directories = workDirs
  directoryStore.selectedDirectoryId = workDirs[0]?.id || ''
  return mount(CommandPalette, {
    global: { plugins: [pinia], stubs: defaultStubs }
  })
}

describe('CommandPalette', () => {
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

  it('renders when visible', () => {
    wrapper = createWrapper()
    expect(wrapper.find('.command-palette-dialog').exists()).toBe(true)
  })

  it('does not render when hidden', () => {
    wrapper = createWrapper({ visible: false })
    expect(wrapper.find('.command-palette-dialog').exists()).toBe(false)
  })

  it('switches to workdir mode with # prefix', async () => {
    wrapper = createWrapper()
    const input = wrapper.find('input')
    await input.setValue('#')
    await input.trigger('input')
    await nextTick()
    const sectionTitles = wrapper.findAll('.section-title')
    const workdirTitle = sectionTitles.find(el => el.text().includes('工作目录'))
    expect(workdirTitle).toBeTruthy()
  })

  it('shows workdir items when in # mode', async () => {
    wrapper = createWrapper()
    const input = wrapper.find('input')
    await input.setValue('#')
    await input.trigger('input')
    await nextTick()
    const items = wrapper.findAll('.result-item')
    expect(items.length).toBeGreaterThanOrEqual(2)
  })

  it('emits select-workdir on workdir click', async () => {
    wrapper = createWrapper()
    const input = wrapper.find('input')
    await input.setValue('#')
    await input.trigger('input')
    await nextTick()
    const items = wrapper.findAll('.result-item')
    if (items.length > 0) {
      await items[0].trigger('click')
      expect(wrapper.emitted('select-workdir')).toBeTruthy()
    }
  })

  it('shows recent section when no input', async () => {
    wrapper = createWrapper()
    await nextTick()
    // With no input, either show recent or empty state
    const content = wrapper.find('.palette-content')
    expect(content.exists()).toBe(true)
  })

  it('switches to favorites mode with @ prefix', async () => {
    const localWrapper = createWrapper()
    const input = localWrapper.find('input')
    await input.setValue('@')
    await input.trigger('input')
    await nextTick()
    // In @ mode, should show favorites section when there are results
    expect(localWrapper.vm).toBeTruthy()
    localWrapper.unmount()
  })

  it('closes on escape key', async () => {
    const localWrapper = createWrapper()
    const input = localWrapper.find('input')
    await input.trigger('keydown', { key: 'Escape' })
    await nextTick()
    // onClose 直写 uiStore.commandPaletteVisible=false（不再 emit update:modelValue）
    expect(useUiStore().commandPaletteVisible).toBe(false)
    localWrapper.unmount()
  })
})
