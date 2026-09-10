import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetSettings: vi.fn(() => Promise.resolve({})),
  SaveSettings: vi.fn(() => Promise.resolve(true))
}))

import {
  useSettingsStore,
  isValidShortcut,
  matchShortcut,
  formatDisplay,
  shortcutFromEvent,
  DEFAULTS
} from '..'
import { GetSettings, SaveSettings } from '../../../wailsjs/go/main/App'

describe('settings store - 默认值', () => {
  it('DEFAULTS 应包含 rename=F2 与 delete=Delete', () => {
    expect(DEFAULTS.rename).toBe('F2')
    expect(DEFAULTS.delete).toBe('Delete')
    expect(DEFAULTS.commandPalette).toBe('Ctrl+P')
    expect(DEFAULTS.toggleTerminal).toBe('Ctrl+`')
  })
})

describe('settings store - isValidShortcut（单键白名单）', () => {
  it('功能键单键合法', () => {
    expect(isValidShortcut('F2')).toBe(true)
    expect(isValidShortcut('Delete')).toBe(true)
    expect(isValidShortcut('F5')).toBe(true)
    expect(isValidShortcut('Insert')).toBe(true)
    expect(isValidShortcut('ArrowUp')).toBe(true)
  })
  it('字母/数字单键非法（避免与文本输入冲突）', () => {
    expect(isValidShortcut('A')).toBe(false)
    expect(isValidShortcut('1')).toBe(false)
  })
  it('含修饰键的组合合法', () => {
    expect(isValidShortcut('Ctrl+P')).toBe(true)
    expect(isValidShortcut('Ctrl+Shift+A')).toBe(true)
    expect(isValidShortcut('Alt+F4')).toBe(true)
  })
  it('空值非法', () => {
    expect(isValidShortcut('')).toBe(false)
    expect(isValidShortcut(null)).toBe(false)
  })
})

describe('settings store - matchShortcut（单键匹配，大小写归一）', () => {
  const ev = (key, mods = {}) => ({
    key,
    ctrlKey: !!mods.ctrl,
    altKey: !!mods.alt,
    shiftKey: !!mods.shift
  })
  it('F2 单键匹配', () => {
    expect(matchShortcut(ev('F2'), 'F2')).toBe(true)
  })
  it('Delete 单键匹配', () => {
    expect(matchShortcut(ev('Delete'), 'Delete')).toBe(true)
  })
  it('Shift+Delete 不匹配 Delete（修饰键差异，避免误触）', () => {
    expect(matchShortcut(ev('Delete', { shift: true }), 'Delete')).toBe(false)
  })
  it('Ctrl+P 匹配（既有命令面板行为不破坏）', () => {
    expect(matchShortcut(ev('p', { ctrl: true }), 'Ctrl+P')).toBe(true)
  })
  it('纯字母无修饰键不匹配 Ctrl+P', () => {
    expect(matchShortcut(ev('p'), 'Ctrl+P')).toBe(false)
  })
})

describe('settings store - shortcutFromEvent', () => {
  const base = { ctrlKey: false, altKey: false, shiftKey: false }
  it('F2 事件 → "F2"', () => {
    expect(shortcutFromEvent({ key: 'F2', ...base })).toBe('F2')
  })
  it('Delete 事件 → "Delete"', () => {
    expect(shortcutFromEvent({ key: 'Delete', ...base })).toBe('Delete')
  })
  it('Ctrl+p 事件 → "Ctrl+P"（字母大写）', () => {
    expect(shortcutFromEvent({ key: 'p', ctrlKey: true, altKey: false, shiftKey: false })).toBe('Ctrl+P')
  })
})

describe('settings store - formatDisplay', () => {
  it('组合键拆分数组', () => {
    expect(formatDisplay('Ctrl+P')).toEqual(['Ctrl', 'P'])
  })
  it('单键单元素数组', () => {
    expect(formatDisplay('F2')).toEqual(['F2'])
  })
})

describe('settings store - checkConflict（含 rename/delete）', () => {
  beforeEach(() => {
    // 每个 it 独立 pinia 实例，store 默认值为 DEFAULTS
    setActivePinia(createPinia())
  })

  it('与 rename 默认值 F2 冲突', () => {
    const store = useSettingsStore()
    const c = store.checkConflict('F2', 'delete')
    expect(c).toBeTruthy()
    expect(c.key).toBe('rename')
  })
  it('与 delete 默认值 Delete 冲突', () => {
    const store = useSettingsStore()
    const c = store.checkConflict('Delete', 'rename')
    expect(c).toBeTruthy()
    expect(c.key).toBe('delete')
  })
  it('excludeKey 排除自身', () => {
    const store = useSettingsStore()
    expect(store.checkConflict('F2', 'rename')).toBeNull()
  })
  it('与命令面板默认值 Ctrl+P 冲突', () => {
    const store = useSettingsStore()
    const c = store.checkConflict('Ctrl+P', 'rename')
    expect(c).toBeTruthy()
    expect(c.key).toBe('commandPalette')
  })
})

describe('settings store - loadShortcuts/saveShortcuts', () => {
  beforeEach(() => {
    // 每个 it 独立 pinia 实例（store 重置为 DEFAULTS）+ 清理 mock 历史 + 重置默认实现
    setActivePinia(createPinia())
    vi.clearAllMocks()
    GetSettings.mockResolvedValue({})
    SaveSettings.mockResolvedValue(true)
  })

  it('loadShortcuts 后端值覆盖默认，未提供字段填默认', async () => {
    GetSettings.mockResolvedValue({ shortcutRename: 'F5', shortcutCommandPalette: 'Ctrl+Shift+P' })
    const store = useSettingsStore()
    await store.loadShortcuts()
    expect(store.shortcutRename).toBe('F5')
    expect(store.shortcutCommandPalette).toBe('Ctrl+Shift+P')
    // 未提供的字段回填默认（toggleTerminal/delete）
    expect(store.shortcutToggleTerminal).toBe(DEFAULTS.toggleTerminal)
    expect(store.shortcutDelete).toBe(DEFAULTS.delete)
  })

  it('loadShortcuts 空对象全填默认', async () => {
    GetSettings.mockResolvedValue({})
    const store = useSettingsStore()
    await store.loadShortcuts()
    expect(store.shortcutCommandPalette).toBe(DEFAULTS.commandPalette)
    expect(store.shortcutRename).toBe(DEFAULTS.rename)
    expect(store.shortcutToggleTerminal).toBe(DEFAULTS.toggleTerminal)
    expect(store.shortcutDelete).toBe(DEFAULTS.delete)
  })

  it('loadShortcuts 失败回退默认（覆盖用户改过的非默认值）', async () => {
    GetSettings.mockRejectedValue(new Error('fail'))
    const store = useSettingsStore()
    // 先改非默认，验证 catch 分支确实重置
    store.shortcutRename = 'F5'
    expect(store.shortcutRename).toBe('F5')
    await store.loadShortcuts()
    expect(store.shortcutRename).toBe(DEFAULTS.rename)
    expect(store.shortcutCommandPalette).toBe(DEFAULTS.commandPalette)
  })

  it('saveShortcuts 将 4 个 shortcut 字段写入 settings 对象并保留其他字段', async () => {
    const existing = { shortcutCommandPalette: 'Ctrl+P', otherField: 'keep' }
    GetSettings.mockResolvedValue(existing)
    const store = useSettingsStore()
    store.shortcutRename = 'F5'
    await store.saveShortcuts()
    expect(SaveSettings).toHaveBeenCalledTimes(1)
    const saved = SaveSettings.mock.calls[0][0]
    // 改后值写入
    expect(saved.shortcutRename).toBe('F5')
    // store 当前值写入（commandPalette 默认 'Ctrl+P'）
    expect(saved.shortcutCommandPalette).toBe('Ctrl+P')
    // 默认值写入
    expect(saved.shortcutToggleTerminal).toBe(DEFAULTS.toggleTerminal)
    expect(saved.shortcutDelete).toBe(DEFAULTS.delete)
    // 非快捷键字段保留
    expect(saved.otherField).toBe('keep')
  })
})

describe('settings store - 主题 (themeMode/resolvedTheme/loadTheme/saveTheme)', () => {
  let matchMediaSpy

  beforeEach(() => {
    // 每个 it 独立 pinia 实例（store 重置为默认 themeMode='system'）+ 清理 mock
    setActivePinia(createPinia())
    vi.clearAllMocks()
    GetSettings.mockResolvedValue({})
    SaveSettings.mockResolvedValue(true)
  })

  afterEach(() => {
    if (matchMediaSpy) {
      matchMediaSpy.mockRestore()
      matchMediaSpy = null
    }
  })

  it('resolvedTheme：light/dark 模式直接映射', () => {
    const store = useSettingsStore()
    store.themeMode = 'light'
    expect(store.resolvedTheme).toBe('light')
    store.themeMode = 'dark'
    expect(store.resolvedTheme).toBe('dark')
  })

  it('resolvedTheme：system 模式默认跟随系统浅色（matchMedia matches:false）', () => {
    // test/setup.js 全局 matchMedia stub 返回 matches:false
    const store = useSettingsStore()
    store.themeMode = 'system'
    expect(store.resolvedTheme).toBe('light')
  })

  it('resolvedTheme：system 模式 + 系统偏好暗色 → dark', () => {
    matchMediaSpy = vi.spyOn(window, 'matchMedia').mockReturnValue({
      matches: true,
      media: '',
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {}
    })
    const store = useSettingsStore()
    store.themeMode = 'system'
    expect(store.resolvedTheme).toBe('dark')
  })

  it('matchMedia change 监听器：system 模式下系统切换暗色 → resolvedTheme 变 dark', () => {
    let changeCb = null
    matchMediaSpy = vi.spyOn(window, 'matchMedia').mockImplementation((q) => ({
      matches: false,
      media: q,
      addEventListener: (ev, cb) => { changeCb = cb },
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {}
    }))
    const store = useSettingsStore()
    store.themeMode = 'system'
    expect(store.resolvedTheme).toBe('light')
    expect(changeCb).not.toBeNull()
    // 模拟系统主题变化为暗色
    changeCb({ matches: true })
    expect(store.resolvedTheme).toBe('dark')
  })

  it('loadTheme：后端 themeMode=dark → themeMode=dark', async () => {
    GetSettings.mockResolvedValue({ themeMode: 'dark' })
    const store = useSettingsStore()
    await store.loadTheme()
    expect(store.themeMode).toBe('dark')
    expect(store.resolvedTheme).toBe('dark')
  })

  it('loadTheme：themeMode 缺失 → 默认 system', async () => {
    GetSettings.mockResolvedValue({})
    const store = useSettingsStore()
    store.themeMode = 'dark'
    await store.loadTheme()
    expect(store.themeMode).toBe('system')
  })

  it('loadTheme：非法值（pink）→ 默认 system', async () => {
    GetSettings.mockResolvedValue({ themeMode: 'pink' })
    const store = useSettingsStore()
    await store.loadTheme()
    expect(store.themeMode).toBe('system')
  })

  it('loadTheme：GetSettings 失败 → 默认 system', async () => {
    GetSettings.mockRejectedValue(new Error('fail'))
    const store = useSettingsStore()
    store.themeMode = 'dark'
    await store.loadTheme()
    expect(store.themeMode).toBe('system')
  })

  it('saveTheme：合并写（GetSettings → 覆盖 themeMode → SaveSettings），保留其他字段', async () => {
    GetSettings.mockResolvedValue({ otherField: 'keep', themeMode: 'light' })
    const store = useSettingsStore()
    store.themeMode = 'dark'
    await store.saveTheme()
    expect(SaveSettings).toHaveBeenCalledTimes(1)
    const saved = SaveSettings.mock.calls[0][0]
    expect(saved.themeMode).toBe('dark')
    // 非主题字段保留
    expect(saved.otherField).toBe('keep')
  })
})
