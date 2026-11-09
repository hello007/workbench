import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetClaudeDefaultModel: vi.fn(() => Promise.resolve('')),
  GetSettings: vi.fn(() => Promise.resolve({})),
  SaveSettings: vi.fn(() => Promise.resolve(true)),
  GetShellConfigs: vi.fn(() => Promise.resolve([]))
}))

import {
  useSettingsStore,
  isValidShortcut,
  matchShortcut,
  formatDisplay,
  shortcutFromEvent,
  normalizeSidebarOrder,
  sidebarShortcutIndex,
  isReservedSidebarShortcut,
  SIDEBAR_PANELS,
  DEFAULTS,
  DIFF_TOOL_PRESETS,
  DIFF_TOOL_PRESETS_LINUX,
  diffToolPresetsFor,
  TERMINAL_APPEARANCE_DEFAULTS,
  TERMINAL_FONT_SIZE_MIN,
  TERMINAL_FONT_SIZE_MAX,
  TERMINAL_SCROLLBACK_MIN,
  TERMINAL_SCROLLBACK_MAX,
  TERMINAL_FONT_OPTIONS,
  FALLBACK_SHELL,
  resolveDefaultShell
} from '..'
import { GetSettings, SaveSettings, GetShellConfigs } from '../../../wailsjs/go/main/App'

describe('settings store - 默认值', () => {
  it('DEFAULTS 应包含 rename=F2 与 delete=Delete', () => {
    expect(DEFAULTS.rename).toBe('F2')
    expect(DEFAULTS.delete).toBe('Delete')
    expect(DEFAULTS.commandPalette).toBe('Ctrl+P')
    expect(DEFAULTS.toggleTerminal).toBe('Ctrl+`')
  })
})

describe('settings store - loadDefaultShell', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    GetSettings.mockResolvedValue({})
    GetShellConfigs.mockResolvedValue([])
  })

  it('用户已设置 defaultShell 且在平台列表内时优先取用户设置', async () => {
    GetSettings.mockResolvedValue({ defaultShell: 'zsh' })
    GetShellConfigs.mockResolvedValue([
      { type: 'bash', displayName: 'Bash' },
      { type: 'zsh', displayName: 'Zsh' }
    ])
    const store = useSettingsStore()
    await store.loadDefaultShell()
    expect(store.defaultShell).toBe('zsh')
  })

  it('用户未设置时取 GetShellConfigs 首项（平台默认：Windows=powershell、Linux=bash）', async () => {
    GetSettings.mockResolvedValue({})
    GetShellConfigs.mockResolvedValue([
      { type: 'bash', displayName: 'Bash' },
      { type: 'zsh', displayName: 'Zsh' }
    ])
    const store = useSettingsStore()
    await store.loadDefaultShell()
    expect(store.defaultShell).toBe('bash')
  })

  it('存量跨平台残留设置收敛到平台默认（settings 存 gitbash + Linux 列表 → bash）', async () => {
    // Windows 时期设置的 gitbash 在 Linux 主机：列表内无匹配项，收敛到首项 bash，
    // 避免下拉裸值 / gitbash 条件渲染误触发 / PTY shellType 无效
    GetSettings.mockResolvedValue({ defaultShell: 'gitbash' })
    GetShellConfigs.mockResolvedValue([
      { type: 'bash', displayName: 'Bash' },
      { type: 'zsh', displayName: 'Zsh' },
      { type: 'fish', displayName: 'Fish' }
    ])
    const store = useSettingsStore()
    await store.loadDefaultShell()
    expect(store.defaultShell).toBe('bash')
  })

  it('列表不可用但用户设置存在时保留用户设置（Windows 感知零变化）', async () => {
    GetSettings.mockResolvedValue({ defaultShell: 'cmd' })
    GetShellConfigs.mockRejectedValue(new Error('fail'))
    const store = useSettingsStore()
    await store.loadDefaultShell()
    expect(store.defaultShell).toBe('cmd')
  })

  it('后端均不可用时回退 FALLBACK_SHELL（powershell，Windows 感知不变）', async () => {
    GetSettings.mockRejectedValue(new Error('fail'))
    GetShellConfigs.mockRejectedValue(new Error('fail'))
    const store = useSettingsStore()
    await store.loadDefaultShell()
    // 断言「列表不可用时的最终解析值」为 FALLBACK_SHELL（G12：不再对导入常量与字面量
    // 做恒真比较，行为语义由上一行解析值断言承载）
    expect(store.defaultShell).toBe(FALLBACK_SHELL)
    expect(store.defaultShell).toBe('powershell')
  })

  it('GetShellConfigs 返回空列表时同样回退 FALLBACK_SHELL', async () => {
    GetSettings.mockResolvedValue({})
    GetShellConfigs.mockResolvedValue([])
    const store = useSettingsStore()
    await store.loadDefaultShell()
    expect(store.defaultShell).toBe(FALLBACK_SHELL)
  })

  it('loadDefaultShell 拉到列表后写入 shellConfigsCache（G8-G10 平台信号单一数据源）', async () => {
    GetSettings.mockResolvedValue({})
    GetShellConfigs.mockResolvedValue([
      { type: 'bash', displayName: 'Bash' },
      { type: 'zsh', displayName: 'Zsh' }
    ])
    const store = useSettingsStore()
    await store.loadDefaultShell()
    expect(store.shellConfigsCache.map(c => c.type)).toEqual(['bash', 'zsh'])
  })

  it('isWindowsPlatform：列表首项 powershell → true，bash → false（G8-G10 平台信号，与 R7 同源）', async () => {
    GetSettings.mockResolvedValue({})
    const store = useSettingsStore()
    GetShellConfigs.mockResolvedValue([
      { type: 'powershell', displayName: 'PowerShell' },
      { type: 'cmd', displayName: 'CMD' }
    ])
    await store.loadDefaultShell()
    expect(store.isWindowsPlatform).toBe(true)

    GetShellConfigs.mockResolvedValue([
      { type: 'bash', displayName: 'Bash' },
      { type: 'zsh', displayName: 'Zsh' }
    ])
    await store.loadDefaultShell()
    expect(store.isWindowsPlatform).toBe(false)
  })

  it('isWindowsPlatform：列表未加载（空）时保守按 Windows 兜底（Windows 用户可见行为零变化）', () => {
    // 全新 store 未执行任何加载，shellConfigsCache 为空 → 兜底 true
    const store = useSettingsStore()
    expect(store.isWindowsPlatform).toBe(true)
  })
})

describe('resolveDefaultShell（默认 Shell 解析纯函数）', () => {
  const linuxConfigs = [
    { type: 'bash', displayName: 'Bash' },
    { type: 'zsh', displayName: 'Zsh' }
  ]
  const winConfigs = [
    { type: 'powershell', displayName: 'PowerShell' },
    { type: 'cmd', displayName: 'CMD' }
  ]

  it('用户设置在列表内 → 原样采用', () => {
    expect(resolveDefaultShell('zsh', linuxConfigs)).toBe('zsh')
  })

  it('跨平台残留值（gitbash + Linux 列表）→ 收敛首项', () => {
    expect(resolveDefaultShell('gitbash', linuxConfigs)).toBe('bash')
    expect(resolveDefaultShell('wsl', linuxConfigs)).toBe('bash')
  })

  it('未设置 → 平台首项（Windows=powershell / Linux=bash）', () => {
    expect(resolveDefaultShell('', linuxConfigs)).toBe('bash')
    expect(resolveDefaultShell('', winConfigs)).toBe('powershell')
  })

  it('列表不可用 → 保留用户设置，未设置回退 FALLBACK_SHELL', () => {
    expect(resolveDefaultShell('cmd', null)).toBe('cmd')
    expect(resolveDefaultShell('cmd', [])).toBe('cmd')
    expect(resolveDefaultShell('', null)).toBe(FALLBACK_SHELL)
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

describe('settings store - isReservedSidebarShortcut（Alt+数字为侧边栏固定快捷键）', () => {
  it('Alt+1..9 命中保留集合', () => {
    for (let n = 1; n <= 9; n++) {
      expect(isReservedSidebarShortcut(`Alt+${n}`)).toBe(true)
    }
  })
  it('其他组合与空值不命中', () => {
    expect(isReservedSidebarShortcut('Alt+0')).toBe(false)
    expect(isReservedSidebarShortcut('Alt+A')).toBe(false)
    expect(isReservedSidebarShortcut('Ctrl+Alt+1')).toBe(false)
    expect(isReservedSidebarShortcut('Alt+10')).toBe(false)
    expect(isReservedSidebarShortcut('')).toBe(false)
    expect(isReservedSidebarShortcut(null)).toBe(false)
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
  it('与侧边栏固定快捷键 Alt+1..9 冲突（不可被自定义项占用）', () => {
    const store = useSettingsStore()
    const c = store.checkConflict('Alt+1', 'delete')
    expect(c).toBeTruthy()
    expect(c.key).toBe('sidebarShortcut')
    // 与 quickTodo 默认值 Ctrl+Alt+N 比较：Alt+N 不是 Alt+数字，不误判
    expect(store.checkConflict('Ctrl+Alt+N', 'delete').key).toBe('quickTodo')
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
    GetSettings.mockResolvedValue({
      shortcutRename: 'F5',
      shortcutCommandPalette: 'Ctrl+Shift+P',
      shortcutQuickTodo: 'Ctrl+Alt+T'
    })
    const store = useSettingsStore()
    await store.loadShortcuts()
    expect(store.shortcutRename).toBe('F5')
    expect(store.shortcutCommandPalette).toBe('Ctrl+Shift+P')
    expect(store.shortcutQuickTodo).toBe('Ctrl+Alt+T')
    // 未提供的字段回填默认（toggleTerminal/delete）
    expect(store.shortcutToggleTerminal).toBe(DEFAULTS.toggleTerminal)
    expect(store.shortcutDelete).toBe(DEFAULTS.delete)
  })

  it('loadShortcuts 空对象全填默认（含 quickTodo 缺省回退）', async () => {
    GetSettings.mockResolvedValue({})
    const store = useSettingsStore()
    await store.loadShortcuts()
    expect(store.shortcutCommandPalette).toBe(DEFAULTS.commandPalette)
    expect(store.shortcutRename).toBe(DEFAULTS.rename)
    expect(store.shortcutToggleTerminal).toBe(DEFAULTS.toggleTerminal)
    expect(store.shortcutDelete).toBe(DEFAULTS.delete)
    expect(store.shortcutQuickTodo).toBe(DEFAULTS.quickTodo)
    expect(DEFAULTS.quickTodo).toBe('Ctrl+Alt+N')
  })

  it('loadShortcuts 失败回退默认（覆盖用户改过的非默认值）', async () => {
    GetSettings.mockRejectedValue(new Error('fail'))
    const store = useSettingsStore()
    // 先改非默认，验证 catch 分支确实重置
    store.shortcutRename = 'F5'
    store.shortcutQuickTodo = 'Ctrl+Alt+T'
    expect(store.shortcutRename).toBe('F5')
    await store.loadShortcuts()
    expect(store.shortcutRename).toBe(DEFAULTS.rename)
    expect(store.shortcutCommandPalette).toBe(DEFAULTS.commandPalette)
    expect(store.shortcutQuickTodo).toBe(DEFAULTS.quickTodo)
  })

  it('saveShortcuts 将 5 个 shortcut 字段写入 settings 对象并保留其他字段', async () => {
    const existing = { shortcutCommandPalette: 'Ctrl+P', otherField: 'keep' }
    GetSettings.mockResolvedValue(existing)
    const store = useSettingsStore()
    store.shortcutRename = 'F5'
    store.shortcutQuickTodo = 'Ctrl+Alt+T'
    await store.saveShortcuts()
    expect(SaveSettings).toHaveBeenCalledTimes(1)
    const saved = SaveSettings.mock.calls[0][0]
    // 改后值写入
    expect(saved.shortcutRename).toBe('F5')
    expect(saved.shortcutQuickTodo).toBe('Ctrl+Alt+T')
    // store 当前值写入（commandPalette 默认 'Ctrl+P'）
    expect(saved.shortcutCommandPalette).toBe('Ctrl+P')
    // 默认值写入
    expect(saved.shortcutToggleTerminal).toBe(DEFAULTS.toggleTerminal)
    expect(saved.shortcutDelete).toBe(DEFAULTS.delete)
    // 非快捷键字段保留
    expect(saved.otherField).toBe('keep')
  })
})

describe('settings store - 侧边栏顺序 (normalizeSidebarOrder/sidebarShortcutIndex/orderedPanels/saveSidebarOrder)', () => {
  const DEFAULT_IDS = SIDEBAR_PANELS.map(p => p.id)

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    GetSettings.mockResolvedValue({})
    SaveSettings.mockResolvedValue(true)
  })

  describe('normalizeSidebarOrder 纯函数', () => {
    it('空/非数组输入回退默认顺序（老配置无 sidebarOrder 字段）', () => {
      expect(normalizeSidebarOrder(undefined)).toEqual(DEFAULT_IDS)
      expect(normalizeSidebarOrder(null)).toEqual(DEFAULT_IDS)
      expect(normalizeSidebarOrder([])).toEqual(DEFAULT_IDS)
      expect(normalizeSidebarOrder('stats,directory')).toEqual(DEFAULT_IDS)
      expect(normalizeSidebarOrder(42)).toEqual(DEFAULT_IDS)
    })

    it('完整合法顺序原样保留且返回新数组（不改入参）', () => {
      const input = [...DEFAULT_IDS].reverse()
      const result = normalizeSidebarOrder(input)
      expect(result).toEqual(input)
      expect(result).not.toBe(input)
    })

    it('未知 id 剔除、缺失面板按默认顺序追加末尾', () => {
      // stats 提前 + unknown 历史残留剔除 + directory 在尾部补齐其余
      const result = normalizeSidebarOrder(['stats', 'unknown-panel', 'directory'])
      expect(result[0]).toBe('stats')
      expect(result[1]).toBe('directory')
      expect(result).not.toContain('unknown-panel') // 未知 id 已剔除
      expect(result).toHaveLength(DEFAULT_IDS.length)
      // 剩余位置为默认序中未出现的面板，相对顺序保持默认序
      const rest = result.slice(2)
      const expectedRest = DEFAULT_IDS.filter(id => id !== 'stats' && id !== 'directory')
      expect(rest).toEqual(expectedRest)
    })

    it('重复 id 保留首次出现', () => {
      const result = normalizeSidebarOrder(['ai', 'directory', 'ai'])
      expect(result.filter(id => id === 'ai')).toHaveLength(1)
      expect(result[0]).toBe('ai')
      expect(result).toHaveLength(DEFAULT_IDS.length)
    })
  })

  describe('sidebarShortcutIndex 纯函数', () => {
    const ev = (key, { alt = true, ctrl = false, shift = false, meta = false } = {}) =>
      ({ key, altKey: alt, ctrlKey: ctrl, shiftKey: shift, metaKey: meta })

    it('Alt+数字返回 0-based 序号', () => {
      expect(sidebarShortcutIndex(ev('1'))).toBe(0)
      expect(sidebarShortcutIndex(ev('8'))).toBe(7)
      expect(sidebarShortcutIndex(ev('9'))).toBe(8)
    })

    it('缺 Alt 或带 Ctrl/Shift/Meta 修饰时不匹配', () => {
      expect(sidebarShortcutIndex(ev('1', { alt: false }))).toBe(-1)
      expect(sidebarShortcutIndex(ev('1', { ctrl: true }))).toBe(-1)
      expect(sidebarShortcutIndex(ev('1', { shift: true }))).toBe(-1)
      expect(sidebarShortcutIndex(ev('1', { meta: true }))).toBe(-1)
    })

    it('非单数字 key 不匹配（字母/多字符功能键/符号）', () => {
      expect(sidebarShortcutIndex(ev('a'))).toBe(-1)
      expect(sidebarShortcutIndex(ev('F1'))).toBe(-1)
      expect(sidebarShortcutIndex(ev('!'))).toBe(-1)
      expect(sidebarShortcutIndex(ev(''))).toBe(-1)
    })
  })

  describe('orderedPanels 加载与保存', () => {
    it('loadShortcuts 归一化加载：配置顺序在前、缺失面板按默认序补齐', async () => {
      GetSettings.mockResolvedValue({ sidebarOrder: ['stats', 'directory'] })
      const store = useSettingsStore()
      await store.loadShortcuts()
      expect(store.orderedPanels[0].id).toBe('stats')
      expect(store.orderedPanels[1].id).toBe('directory')
      expect(store.orderedPanels).toHaveLength(DEFAULT_IDS.length)
      // 元素为 SIDEBAR_PANELS 成员（带 label），ActivityBar 直接消费
      expect(store.orderedPanels[0].label).toBe(SIDEBAR_PANELS.find(p => p.id === 'stats').label)
    })

    it('loadShortcuts 空配置回默认顺序，失败回退默认顺序', async () => {
      const store = useSettingsStore()
      await store.loadShortcuts()
      expect(store.orderedPanels.map(p => p.id)).toEqual(DEFAULT_IDS)

      GetSettings.mockRejectedValue(new Error('fail'))
      store.sidebarOrder = ['stats', ...DEFAULT_IDS.filter(id => id !== 'stats')]
      await store.loadShortcuts()
      expect(store.orderedPanels.map(p => p.id)).toEqual(DEFAULT_IDS)
    })

    it('saveSidebarOrder 将顺序写入 settings 对象并保留其他字段', async () => {
      const existing = { sidebarOrder: DEFAULT_IDS, themeMode: 'dark', otherField: 'keep' }
      GetSettings.mockResolvedValue(existing)
      const store = useSettingsStore()
      store.sidebarOrder = ['stats', 'directory', ...DEFAULT_IDS.filter(id => id !== 'stats' && id !== 'directory')]
      await store.saveSidebarOrder()
      expect(SaveSettings).toHaveBeenCalledTimes(1)
      const saved = SaveSettings.mock.calls[0][0]
      expect(saved.sidebarOrder[0]).toBe('stats')
      expect(saved.sidebarOrder).toHaveLength(DEFAULT_IDS.length)
      expect(saved.themeMode).toBe('dark')
      expect(saved.otherField).toBe('keep')
    })
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

describe('settings store - 外部 diff 工具 (diffTool/loadDiffTool/saveDiffTool)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    GetSettings.mockResolvedValue({})
    SaveSettings.mockResolvedValue(true)
  })

  it('默认值：预设 beyondcompare、路径与参数为空、未配置', () => {
    const store = useSettingsStore()
    expect(store.diffToolName).toBe('beyondcompare')
    expect(store.diffToolPath).toBe('')
    expect(store.diffToolArgs).toBe('')
    expect(store.diffToolConfigured).toBe(false)
  })

  it('loadDiffTool 后端值覆盖，空值回退默认', async () => {
    GetSettings.mockResolvedValue({ diffToolName: 'winmerge', diffToolPath: 'C:\\tools\\WinMergeU.exe', diffToolArgs: '{left} {right}' })
    const store = useSettingsStore()
    await store.loadDiffTool()
    expect(store.diffToolName).toBe('winmerge')
    expect(store.diffToolPath).toBe('C:\\tools\\WinMergeU.exe')
    expect(store.diffToolArgs).toBe('{left} {right}')
    expect(store.diffToolConfigured).toBe(true)

    GetSettings.mockResolvedValue({})
    await store.loadDiffTool()
    expect(store.diffToolName).toBe('beyondcompare')
    expect(store.diffToolPath).toBe('')
    expect(store.diffToolConfigured).toBe(false)
  })

  it('loadDiffTool 失败回退默认', async () => {
    GetSettings.mockRejectedValue(new Error('fail'))
    const store = useSettingsStore()
    store.diffToolName = 'vscode'
    await store.loadDiffTool()
    expect(store.diffToolName).toBe('beyondcompare')
    expect(store.diffToolPath).toBe('')
  })

  it('saveDiffTool 合并写：写入三个字段并保留其他字段', async () => {
    GetSettings.mockResolvedValue({ otherField: 'keep' })
    const store = useSettingsStore()
    store.diffToolName = 'vscode'
    store.diffToolPath = 'code'
    store.diffToolArgs = '--diff --wait {left} {right}'
    await store.saveDiffTool()
    expect(SaveSettings).toHaveBeenCalledTimes(1)
    const saved = SaveSettings.mock.calls[0][0]
    expect(saved.diffToolName).toBe('vscode')
    expect(saved.diffToolPath).toBe('code')
    expect(saved.diffToolArgs).toBe('--diff --wait {left} {right}')
    expect(saved.otherField).toBe('keep')
  })

  it('diffToolConfigured：路径仅空白视为未配置', () => {
    const store = useSettingsStore()
    store.diffToolPath = '   '
    store.diffToolArgs = '{left} {right}'
    expect(store.diffToolConfigured).toBe(false)
    store.diffToolPath = ' code '
    expect(store.diffToolConfigured).toBe(true)
  })

  it('diffToolConfigured：args 缺占位符视为未配置（与后端校验对称）', () => {
    const store = useSettingsStore()
    store.diffToolPath = 'code'
    store.diffToolArgs = '--diff'
    expect(store.diffToolConfigured).toBe(false)
    store.diffToolArgs = '{left}'
    expect(store.diffToolConfigured).toBe(false)
    store.diffToolArgs = '{left} {right}'
    expect(store.diffToolConfigured).toBe(true)
  })

  it('DIFF_TOOL_PRESETS：四个预设均含 label/path/args，args 含占位符', () => {
    const keys = Object.keys(DIFF_TOOL_PRESETS)
    expect(keys).toEqual(expect.arrayContaining(['beyondcompare', 'winmerge', 'vscode', 'custom']))
    for (const key of keys) {
      const preset = DIFF_TOOL_PRESETS[key]
      expect(preset.label).toBeTruthy()
      expect(typeof preset.path).toBe('string')
      expect(preset.args).toContain('{left}')
      expect(preset.args).toContain('{right}')
    }
    expect(DIFF_TOOL_PRESETS.vscode.args).toBe('--diff --wait {left} {right}')
  })

  it('diffToolPresetsFor：Windows 返回既有表，非 Windows 返回 Linux 表（G10 平台化）', () => {
    expect(diffToolPresetsFor(true)).toBe(DIFF_TOOL_PRESETS)
    expect(diffToolPresetsFor(false)).toBe(DIFF_TOOL_PRESETS_LINUX)
  })

  it('DIFF_TOOL_PRESETS_LINUX：Linux 常见 diff 工具预设（G10）', () => {
    const keys = Object.keys(DIFF_TOOL_PRESETS_LINUX)
    // beyondcompare/custom 两平台共用 key：loadDiffTool 空值回退名与存量 settings.json
    // 在 Linux 下拉中均有对应项不裸值
    expect(keys).toEqual(expect.arrayContaining(['beyondcompare', 'meld', 'kdiff3', 'vscode', 'custom']))
    for (const key of keys) {
      const preset = DIFF_TOOL_PRESETS_LINUX[key]
      expect(preset.label).toBeTruthy()
      expect(typeof preset.path).toBe('string')
      expect(preset.args).toContain('{left}')
      expect(preset.args).toContain('{right}')
    }
    // Linux 预设用 `command -v` 可命中的裸命令名，不写 Windows 专有路径
    expect(DIFF_TOOL_PRESETS_LINUX.meld.path).toBe('meld')
    expect(DIFF_TOOL_PRESETS_LINUX.kdiff3.path).toBe('kdiff3')
    expect(DIFF_TOOL_PRESETS_LINUX.beyondcompare.path).toBe('bcompare')
    expect(DIFF_TOOL_PRESETS_LINUX.winmerge).toBeUndefined()
  })
})

describe('settings store - 终端外观 (terminalFontSize/fontFamily/scrollback)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    GetSettings.mockResolvedValue({})
    SaveSettings.mockResolvedValue(true)
  })

  it('默认值：字号 14 / 字体空串（Cascadia Code 栈）/ 回滚 5000，与后端常量对齐', () => {
    const store = useSettingsStore()
    expect(store.terminalFontSize).toBe(TERMINAL_APPEARANCE_DEFAULTS.fontSize)
    expect(store.terminalFontSize).toBe(14)
    expect(store.terminalFontFamily).toBe('')
    expect(store.terminalScrollback).toBe(TERMINAL_APPEARANCE_DEFAULTS.scrollback)
    expect(store.terminalScrollback).toBe(5000)
    expect(store.terminalOscNotifyEnabled).toBe(false)
    expect(TERMINAL_FONT_SIZE_MIN).toBe(10)
    expect(TERMINAL_FONT_SIZE_MAX).toBe(24)
    expect(TERMINAL_SCROLLBACK_MIN).toBe(1000)
    expect(TERMINAL_SCROLLBACK_MAX).toBe(10000)
  })

  it('TERMINAL_FONT_OPTIONS 含默认 Cascadia Code 栈（空串 value）与 Consolas/Courier New', () => {
    expect(TERMINAL_FONT_OPTIONS.map(o => o.value)).toEqual(['', 'Consolas', 'Courier New'])
    for (const opt of TERMINAL_FONT_OPTIONS) {
      expect(opt.label).toBeTruthy()
    }
  })

  it('loadTerminalAppearance 后端值覆盖默认，零值/缺失回退默认', async () => {
    GetSettings.mockResolvedValue({ terminalFontSize: 18, terminalFontFamily: 'Consolas', terminalScrollback: 5000 })
    const store = useSettingsStore()
    await store.loadTerminalAppearance()
    expect(store.terminalFontSize).toBe(18)
    expect(store.terminalFontFamily).toBe('Consolas')
    expect(store.terminalScrollback).toBe(5000)

    // 后端零值（老配置兜底缺失场景）回退默认（回滚默认 5000）
    GetSettings.mockResolvedValue({ terminalFontSize: 0, terminalFontFamily: '', terminalScrollback: 0 })
    await store.loadTerminalAppearance()
    expect(store.terminalFontSize).toBe(14)
    expect(store.terminalFontFamily).toBe('')
    expect(store.terminalScrollback).toBe(5000)
  })

  it('已持久化 1000 不被默认值迁移覆盖（仅零值补默认路径生效 5000）', async () => {
    // 存量用户显式配置的合法值保持原样，不做静默改写
    GetSettings.mockResolvedValue({ terminalFontSize: 14, terminalFontFamily: '', terminalScrollback: 1000 })
    const store = useSettingsStore()
    await store.loadTerminalAppearance()
    expect(store.terminalScrollback).toBe(1000)
    // 新装/缺失字段（undefined → NaN → 回退默认）才落 5000
    GetSettings.mockResolvedValue({})
    await store.loadTerminalAppearance()
    expect(store.terminalScrollback).toBe(5000)
  })

  it('loadTerminalAppearance 越界值 clamp 到 [10,24] / [1000,10000]', async () => {
    GetSettings.mockResolvedValue({ terminalFontSize: 99, terminalScrollback: 50 })
    const store = useSettingsStore()
    await store.loadTerminalAppearance()
    expect(store.terminalFontSize).toBe(TERMINAL_FONT_SIZE_MAX)
    expect(store.terminalScrollback).toBe(TERMINAL_SCROLLBACK_MIN)

    GetSettings.mockResolvedValue({ terminalFontSize: 5, terminalScrollback: 99999 })
    await store.loadTerminalAppearance()
    expect(store.terminalFontSize).toBe(TERMINAL_FONT_SIZE_MIN)
    expect(store.terminalScrollback).toBe(TERMINAL_SCROLLBACK_MAX)
  })

  it('loadTerminalAppearance 失败保持当前值不抛错', async () => {
    GetSettings.mockRejectedValue(new Error('fail'))
    const store = useSettingsStore()
    store.terminalFontSize = 20
    await store.loadTerminalAppearance()
    expect(store.terminalFontSize).toBe(20)
  })

  it('saveTerminalAppearance 合并写外观三字段与 OSC 通知开关并保留其他字段', async () => {
    GetSettings.mockResolvedValue({ otherField: 'keep' })
    const store = useSettingsStore()
    store.terminalFontSize = 20
    store.terminalFontFamily = 'Courier New'
    store.terminalScrollback = 8000
    store.terminalOscNotifyEnabled = true
    await store.saveTerminalAppearance()
    expect(SaveSettings).toHaveBeenCalledTimes(1)
    const saved = SaveSettings.mock.calls[0][0]
    expect(saved.terminalFontSize).toBe(20)
    expect(saved.terminalFontFamily).toBe('Courier New')
    expect(saved.terminalScrollback).toBe(8000)
    expect(saved.terminalOscNotifyEnabled).toBe(true)
    expect(saved.otherField).toBe('keep')
  })

  it('loadTerminalAppearance 加载 OSC 通知开关（缺省 false，显式 true 生效）', async () => {
    GetSettings.mockResolvedValue({ terminalOscNotifyEnabled: true })
    const store = useSettingsStore()
    await store.loadTerminalAppearance()
    expect(store.terminalOscNotifyEnabled).toBe(true)
    // 缺省字段回退 false（老配置无该字段）
    GetSettings.mockResolvedValue({})
    await store.loadTerminalAppearance()
    expect(store.terminalOscNotifyEnabled).toBe(false)
    // 非布尔值（脏数据）收敛 false
    GetSettings.mockResolvedValue({ terminalOscNotifyEnabled: 'yes' })
    await store.loadTerminalAppearance()
    expect(store.terminalOscNotifyEnabled).toBe(false)
  })

  it('stepTerminalFontSize 步进 ±1 并持久化', async () => {
    GetSettings.mockResolvedValue({})
    const store = useSettingsStore()
    expect(store.stepTerminalFontSize(1)).toBe(true)
    expect(store.terminalFontSize).toBe(15)
    expect(store.stepTerminalFontSize(-1)).toBe(true)
    expect(store.terminalFontSize).toBe(14)
    await flushPromises()
    expect(SaveSettings).toHaveBeenCalled()
  })

  it('stepTerminalFontSize 边界不越界：10 减 / 24 加返回 false 且不变不存', async () => {
    GetSettings.mockResolvedValue({})
    const store = useSettingsStore()
    store.terminalFontSize = TERMINAL_FONT_SIZE_MIN
    expect(store.stepTerminalFontSize(-1)).toBe(false)
    expect(store.terminalFontSize).toBe(TERMINAL_FONT_SIZE_MIN)

    store.terminalFontSize = TERMINAL_FONT_SIZE_MAX
    expect(store.stepTerminalFontSize(1)).toBe(false)
    expect(store.terminalFontSize).toBe(TERMINAL_FONT_SIZE_MAX)

    // 边界内仍可步进
    expect(store.stepTerminalFontSize(-1)).toBe(true)
    expect(store.terminalFontSize).toBe(TERMINAL_FONT_SIZE_MAX - 1)
  })
})
