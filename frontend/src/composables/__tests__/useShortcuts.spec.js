import { describe, it, expect, vi } from 'vitest'

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetSettings: vi.fn(() => Promise.resolve({})),
  SaveSettings: vi.fn(() => Promise.resolve(true))
}))

import { useShortcuts } from '../useShortcuts'

const {
  isValidShortcut,
  matchShortcut,
  formatDisplay,
  shortcutFromEvent,
  checkConflict,
  DEFAULTS
} = useShortcuts()

describe('useShortcuts - 默认值', () => {
  it('DEFAULTS 应包含 rename=F2 与 delete=Delete', () => {
    expect(DEFAULTS.rename).toBe('F2')
    expect(DEFAULTS.delete).toBe('Delete')
    expect(DEFAULTS.commandPalette).toBe('Ctrl+P')
    expect(DEFAULTS.toggleTerminal).toBe('Ctrl+`')
  })
})

describe('useShortcuts - isValidShortcut（单键白名单）', () => {
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

describe('useShortcuts - matchShortcut（单键匹配，大小写归一）', () => {
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

describe('useShortcuts - shortcutFromEvent', () => {
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

describe('useShortcuts - formatDisplay', () => {
  it('组合键拆分数组', () => {
    expect(formatDisplay('Ctrl+P')).toEqual(['Ctrl', 'P'])
  })
  it('单键单元素数组', () => {
    expect(formatDisplay('F2')).toEqual(['F2'])
  })
})

describe('useShortcuts - checkConflict（含 rename/delete）', () => {
  it('与 rename 默认值 F2 冲突', () => {
    const c = checkConflict('F2', 'delete')
    expect(c).toBeTruthy()
    expect(c.key).toBe('rename')
  })
  it('与 delete 默认值 Delete 冲突', () => {
    const c = checkConflict('Delete', 'rename')
    expect(c).toBeTruthy()
    expect(c.key).toBe('delete')
  })
  it('excludeKey 排除自身', () => {
    expect(checkConflict('F2', 'rename')).toBeNull()
  })
  it('与命令面板默认值 Ctrl+P 冲突', () => {
    const c = checkConflict('Ctrl+P', 'rename')
    expect(c).toBeTruthy()
    expect(c.key).toBe('commandPalette')
  })
})
