/**
 * useTerminal composable 主题切换测试
 * 覆盖：getTerminalTheme 选择、initTerminal 按生效主题初始化、resolvedTheme 变化实时更新 term.options.theme
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'

// 用 vi.hoisted 声明捕获变量，确保 vi.mock 工厂可安全引用（hoistable）
const captures = vi.hoisted(() => ({ lastTerminal: null, lastTerminalConfig: null }))

vi.mock('@xterm/xterm', () => ({
  Terminal: vi.fn(function (config) {
    captures.lastTerminalConfig = config
    captures.lastTerminal = {
      cols: 80,
      rows: 24,
      options: { theme: config.theme },
      loadAddon: vi.fn(),
      open: vi.fn(),
      onData: vi.fn(),
      write: vi.fn(),
      writeln: vi.fn(),
      dispose: vi.fn(),
      focus: vi.fn()
    }
    return captures.lastTerminal
  })
}))
vi.mock('@xterm/addon-fit', () => ({
  FitAddon: vi.fn(function () { return { fit: vi.fn() } })
}))
vi.mock('@xterm/addon-web-links', () => ({
  WebLinksAddon: vi.fn(function () { return {} })
}))
vi.mock('../../../wailsjs/go/main/App', () => ({
  CreateTerminal: vi.fn(() => Promise.resolve('sid-1')),
  WriteTerminalInput: vi.fn(() => Promise.resolve()),
  ChangeTerminalDir: vi.fn(() => Promise.resolve()),
  ResizeTerminal: vi.fn(() => Promise.resolve()),
  CloseTerminal: vi.fn(() => Promise.resolve())
}))
vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn()
}))

import { useTerminal, getTerminalTheme, LIGHT_TERMINAL_THEME, DARK_TERMINAL_THEME } from '../useTerminal'
import { useSettingsStore } from '../../store'

describe('useTerminal - 主题切换', () => {
  let settingsStore

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    captures.lastTerminal = null
    captures.lastTerminalConfig = null
    settingsStore = useSettingsStore()
  })

  it('getTerminalTheme: dark → DARK_TERMINAL_THEME，light → LIGHT_TERMINAL_THEME', () => {
    expect(getTerminalTheme('dark')).toBe(DARK_TERMINAL_THEME)
    expect(getTerminalTheme('light')).toBe(LIGHT_TERMINAL_THEME)
  })

  it('DARK_TERMINAL_THEME 含深色背景/浅色文字/主色光标', () => {
    expect(DARK_TERMINAL_THEME.background).toBe('#1d1e1f')
    expect(DARK_TERMINAL_THEME.foreground).toBe('#e4e4e4')
    expect(DARK_TERMINAL_THEME.cursor).toBe('#409eff')
  })

  it('LIGHT_TERMINAL_THEME 含白色背景/深色文字', () => {
    expect(LIGHT_TERMINAL_THEME.background).toBe('#ffffff')
    expect(LIGHT_TERMINAL_THEME.foreground).toBe('#303133')
  })

  it('initTerminal 按 resolvedTheme=dark 初始化 xterm 主题', async () => {
    settingsStore.themeMode = 'dark'
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.lastTerminalConfig.theme).toBe(DARK_TERMINAL_THEME)
  })

  it('initTerminal 按 resolvedTheme=light 初始化 xterm 主题', async () => {
    settingsStore.themeMode = 'light'
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.lastTerminalConfig.theme).toBe(LIGHT_TERMINAL_THEME)
  })

  it('resolvedTheme 变化时实时更新 term.options.theme（终端已初始化）', async () => {
    settingsStore.themeMode = 'light'
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.lastTerminal.options.theme).toBe(LIGHT_TERMINAL_THEME)
    // 切到 dark：watch 触发，更新已存在终端的主题
    settingsStore.themeMode = 'dark'
    await nextTick()
    expect(captures.lastTerminal.options.theme).toBe(DARK_TERMINAL_THEME)
  })

  it('终端未初始化时主题切换不报错（term.value 为 null）', async () => {
    settingsStore.themeMode = 'light'
    const t = useTerminal()
    // 未调用 initTerminal，term.value 仍为 null
    expect(t.term.value).toBeNull()
    settingsStore.themeMode = 'dark'
    await nextTick()
    // watch 回调内 term.value 为 null 时跳过，不抛错
    expect(t.term.value).toBeNull()
  })
})
