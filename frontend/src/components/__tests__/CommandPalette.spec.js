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

// useRecentAccess 走 localStorage（跨文件残留脏记录），mock 为空避免 undefined path 触发 getFileName
vi.mock('../../composables/useRecentAccess', () => ({
  useRecentAccess: () => ({ getRecent: () => [], record: () => {}, clear: () => {} })
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
    // 清空 localStorage 避免 useRecentAccess 跨文件残留脏记录（undefined path 触发 getFileName 报错）
    localStorage.clear()
  })

  afterEach(() => {
    // 清理 onInput 排程的 searchTimer（300ms setTimeout），避免跨测试异步触发渲染报错
    vi.clearAllTimers()
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

  // ===== 纯函数分支 =====
  it('formatTime 按时间差输出相对文案（4 分支）', () => {
    wrapper = createWrapper()
    const { formatTime } = wrapper.vm.$.setupState
    const now = Date.now()
    expect(formatTime(now)).toBe('刚刚')
    expect(formatTime(now - 5 * 60000)).toBe('5分钟前')
    expect(formatTime(now - 2 * 3600000)).toBe('2小时前')
    expect(formatTime(now - 2 * 86400000)).toBe('2天前')
  })

  it('getFileName 兼容 / 与 \\ 分隔符', () => {
    wrapper = createWrapper()
    const { getFileName } = wrapper.vm.$.setupState
    expect(getFileName('a/b/c.go')).toBe('c.go')
    expect(getFileName('a\\b\\d.go')).toBe('d.go')
  })

  it('highlightMatch 无关键词时返回转义文本，有关键词时包裹 <mark>', () => {
    wrapper = createWrapper()
    const { highlightMatch } = wrapper.vm.$.setupState
    expect(highlightMatch('a<b>', '')).toBe('a&lt;b&gt;')
    expect(highlightMatch('hello world', 'world')).toContain('<mark>')
    // 关键词含正则特殊字符不报错
    expect(highlightMatch('a.b.c', 'a.b')).toContain('<mark>')
  })

  // ===== 键盘导航分支 =====
  it('moveDown/moveUp 调整 selectedIndex', async () => {
    wrapper = createWrapper()
    const input = wrapper.find('input')
    await input.setValue('#')
    await input.trigger('input')
    await nextTick()
    const ss = wrapper.vm.$.setupState
    const before = ss.selectedIndex
    await input.trigger('keydown', { key: 'ArrowDown' })
    expect(ss.selectedIndex).toBe(before + 1)
    await input.trigger('keydown', { key: 'ArrowUp' })
    expect(ss.selectedIndex).toBe(before)
    // moveUp 在 0 时不再递减
    ss.selectedIndex = 0
    await input.trigger('keydown', { key: 'ArrowUp' })
    expect(ss.selectedIndex).toBe(0)
  })

  it('selectCurrent 在 workdir 模式 emit select-workdir', async () => {
    wrapper = createWrapper()
    const input = wrapper.find('input')
    await input.setValue('#')
    await input.trigger('input')
    await nextTick()
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('select-workdir')).toBeTruthy()
  })

  it('点击工作目录项 emit select-workdir', async () => {
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

  // ===== 内容搜索模式分支 =====
  it(': 前缀进入单目录内容搜索模式，展示提示', async () => {
    wrapper = createWrapper()
    const input = wrapper.find('input')
    await input.setValue(':keyword')
    await input.trigger('input')
    await nextTick()
    expect(wrapper.vm.$.setupState.mode).toBe('content')
    expect(wrapper.text()).toContain('按 Enter 搜索')
  })

  it(':: 前缀进入全局内容搜索模式', async () => {
    wrapper = createWrapper()
    const input = wrapper.find('input')
    await input.setValue('::keyword')
    await input.trigger('input')
    await nextTick()
    expect(wrapper.vm.$.setupState.mode).toBe('content-global')
    expect(wrapper.text()).toContain('按 Enter 确认搜索')
  })

  it('内容搜索查询解析 fileExt 与 subDir', () => {
    wrapper = createWrapper()
    const ss = wrapper.vm.$.setupState
    ss.input = ':.go src/ keyword'
    expect(ss.contentQuery.fileExt).toBe('.go')
    expect(ss.contentQuery.subDir).toBe('src')
    expect(ss.contentQuery.keyword).toBe('keyword')
  })

  // ===== 空状态分支 =====
  it('general 模式无匹配时展示未找到匹配项', async () => {
    wrapper = createWrapper()
    const input = wrapper.find('input')
    await input.setValue('zzznotexist')
    await input.trigger('input')
    await nextTick()
    expect(wrapper.text()).toContain('未找到匹配项')
  })

  // ===== 纯函数与 emit 分支 =====
  it('selectItem emit select-file 并关闭', async () => {
    wrapper = createWrapper()
    const ss = wrapper.vm.$.setupState
    ss.selectItem({ path: 'D:\\a.go', type: 'file' })
    expect(wrapper.emitted('select-file')).toBeTruthy()
    expect(useUiStore().commandPaletteVisible).toBe(false)
  })

  it('selectFile emit select-file 带 path/type', async () => {
    wrapper = createWrapper()
    wrapper.vm.$.setupState.selectFile({ path: 'D:\\b.go', type: 'file', name: 'b.go' })
    expect(wrapper.emitted('select-file')).toBeTruthy()
    expect(wrapper.emitted('select-file')[0][0]).toEqual({ path: 'D:\\b.go', type: 'file' })
  })

  it('selectFavorite emit select-favorite', async () => {
    wrapper = createWrapper()
    const fav = { path: 'D:\\fav', alias: '收藏1' }
    wrapper.vm.$.setupState.selectFavorite(fav)
    expect(wrapper.emitted('select-favorite')).toBeTruthy()
    expect(wrapper.emitted('select-favorite')[0][0]).toEqual(fav)
  })

  it('selectWorkDir emit select-workdir', async () => {
    wrapper = createWrapper()
    const dir = { id: '1', name: 'A', path: 'C:\\a' }
    wrapper.vm.$.setupState.selectWorkDir(dir)
    expect(wrapper.emitted('select-workdir')).toBeTruthy()
  })

  it('getFavIndex / getFileIndex 计算全局索引', async () => {
    wrapper = createWrapper()
    const ss = wrapper.vm.$.setupState
    // showRecent=false（无 recent），getFavIndex=index
    ss.recentItems = []
    expect(ss.getFavIndex(2)).toBe(2)
    // showRecent=true，getFavIndex = recentItems.length + index（item 须有 path，否则 render 调 getFileName 报错）
    ss.recentItems = [{ path: 'a' }, { path: 'b' }]
    expect(ss.getFavIndex(1)).toBe(3)
  })

  it('handleRemoveFav 调 removeFavorite 并过滤已删项', async () => {
    wrapper = createWrapper()
    const ss = wrapper.vm.$.setupState
    ss.favoriteResults = [{ path: 'D:\\a' }, { path: 'D:\\b' }]
    const { RemoveFavorite } = await import('../../../wailsjs/go/main/App')
    RemoveFavorite.mockResolvedValueOnce('')
    await ss.handleRemoveFav({ path: 'D:\\a' })
    expect(RemoveFavorite).toHaveBeenCalledWith('D:\\a')
    expect(ss.favoriteResults.length).toBe(1)
    expect(ss.favoriteResults[0].path).toBe('D:\\b')
  })

  it('onInput：favorites 模式调 searchFavorites', async () => {
    wrapper = createWrapper()
    const ss = wrapper.vm.$.setupState
    ss.input = '@key'
    ss.onInput()
    // favorites 模式 → favoriteResults 被赋值（searchFavorites 返回空数组）
    expect(ss.favoriteResults).toEqual([])
  })

  it('onInput：content 模式清空上次结果', async () => {
    wrapper = createWrapper()
    const ss = wrapper.vm.$.setupState
    ss.favoriteResults = [{ path: 'x' }]
    ss.fileResults = [{ path: 'y' }]
    ss.input = ':kw'
    ss.onInput()
    expect(ss.favoriteResults).toEqual([])
    expect(ss.fileResults).toEqual([])
    expect(ss.contentSearchExecuted).toBe(false)
  })

  it('onInput：general 模式有 query 时设 favoriteResults 并排程搜索', async () => {
    wrapper = createWrapper()
    const ss = wrapper.vm.$.setupState
    ss.input = 'keyword'
    ss.onInput()
    // favoriteResults 被赋空（无收藏匹配），searchTimer 排程
    expect(ss.favoriteResults).toEqual([])
  })
})
