/**
 * useTerminal composable 测试
 * 覆盖：主题切换、多实例事件治理、外观热更、addon 生命周期（WebGL 回退 /
 * 搜索开关 / serialize 回放）、Ctrl+F 拦截、OSC 通知开关与节流、多行粘贴确认
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'

// 用 vi.hoisted 声明捕获变量，确保 vi.mock 工厂可安全引用（hoistable）
const captures = vi.hoisted(() => ({
  lastTerminal: null,
  lastTerminalConfig: null,
  lastFit: null,
  offFns: [],
  lastWebgl: null,
  lastSearch: null,
  lastSerialize: null,
  serializedText: '',
  oscHandlers: {},
  linkProvider: null,
  searchShortcutCb: null,
  lastOutputCb: null,
  webglContextLossCb: null
}))

vi.mock('@xterm/xterm', () => ({
  Terminal: vi.fn(function (config) {
    captures.lastTerminalConfig = config
    captures.oscHandlers = {}
    captures.searchShortcutCb = null
    captures.lastTerminal = {
      cols: 80,
      rows: 24,
      // options 承载初始化配置（xterm options 可运行时热更，测试模拟同款语义）
      options: { theme: config.theme, fontSize: config.fontSize, fontFamily: config.fontFamily },
      loadAddon: vi.fn(),
      open: vi.fn(),
      onData: vi.fn(),
      write: vi.fn(),
      writeln: vi.fn(),
      dispose: vi.fn(),
      focus: vi.fn(),
      paste: vi.fn(),
      parser: {
        registerOscHandler: vi.fn((ident, cb) => {
          captures.oscHandlers[ident] = cb
          return true
        })
      },
      attachCustomKeyEventHandler: vi.fn((cb) => { captures.searchShortcutCb = cb }),
      registerLinkProvider: vi.fn((provider) => { captures.linkProvider = provider })
    }
    return captures.lastTerminal
  })
}))
vi.mock('@xterm/addon-fit', () => ({
  FitAddon: vi.fn(function () {
    captures.lastFit = { fit: vi.fn() }
    return captures.lastFit
  })
}))
vi.mock('@xterm/addon-web-links', () => ({
  WebLinksAddon: vi.fn(function () { return {} })
}))
vi.mock('@xterm/addon-webgl', () => ({
  WebglAddon: vi.fn(function () {
    captures.lastWebgl = {
      onContextLoss: vi.fn((cb) => { captures.webglContextLossCb = cb }),
      dispose: vi.fn()
    }
    return captures.lastWebgl
  })
}))
vi.mock('@xterm/addon-search', () => ({
  SearchAddon: vi.fn(function () {
    captures.lastSearch = {
      findNext: vi.fn(),
      findPrevious: vi.fn(),
      dispose: vi.fn(),
      onDidChangeResults: vi.fn()
    }
    return captures.lastSearch
  })
}))
vi.mock('@xterm/addon-serialize', () => ({
  SerializeAddon: vi.fn(function () {
    captures.lastSerialize = {
      serialize: vi.fn(() => captures.serializedText),
      dispose: vi.fn()
    }
    return captures.lastSerialize
  })
}))
vi.mock('@xterm/addon-unicode11', () => ({
  Unicode11Addon: vi.fn(function () { return {} })
}))
// ElMessageBox 供多行粘贴确认消费、ElNotification 供 OSC 通知消费（文件级 mock 屏蔽 EP 运行时）
vi.mock('element-plus', () => ({
  ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) },
  ElNotification: vi.fn()
}))
vi.mock('../../../wailsjs/go/main/App', () => ({
  GetClaudeDefaultModel: vi.fn(() => Promise.resolve('')),
  CreateTerminal: vi.fn(() => Promise.resolve('sid-1')),
  WriteTerminalInput: vi.fn(() => Promise.resolve()),
  ChangeTerminalDir: vi.fn(() => Promise.resolve()),
  ResizeTerminal: vi.fn(() => Promise.resolve()),
  CloseTerminal: vi.fn(() => Promise.resolve()),
  OpenWithDefaultApp: vi.fn(() => Promise.resolve(true))
}))
vi.mock('../../../wailsjs/runtime/runtime', () => ({
  // 与真实 Wails runtime 一致：EventsOn 返回本监听器的注销闭包；
  // terminal-output 回调另存 captures.lastOutputCb（回放顺序用例驱动暂存缓冲）
  EventsOn: vi.fn((name, cb) => {
    const off = vi.fn()
    captures.offFns.push(off)
    if (name === 'terminal-output') captures.lastOutputCb = cb
    return off
  }),
  EventsOff: vi.fn()
}))

import { useTerminal, resetWebglProbeCacheForTests, getTerminalTheme, LIGHT_TERMINAL_THEME, DARK_TERMINAL_THEME } from '../useTerminal'
import { useSettingsStore } from '../../store'

describe('useTerminal - 主题切换', () => {
  let settingsStore

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    captures.lastTerminal = null
    captures.lastTerminalConfig = null
    captures.lastFit = null
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

describe('useTerminal - 多实例事件治理', () => {
  let settingsStore

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    captures.lastTerminal = null
    captures.lastTerminalConfig = null
    captures.lastFit = null
    captures.offFns = []
    settingsStore = useSettingsStore()
  })

  it('destroyTerminal 调用 EventsOn 返回的闭包精准注销，不再调用全局 EventsOff', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    // terminal-output + terminal-exit 两个监听器各有一个注销闭包
    expect(captures.offFns.length).toBe(2)
    captures.offFns.forEach((off) => expect(off).not.toHaveBeenCalled())

    await t.destroyTerminal()
    captures.offFns.forEach((off) => expect(off).toHaveBeenCalledTimes(1))
  })

  it('CreateTerminal 失败时同样精准注销监听器', async () => {
    const { CreateTerminal } = await import('../../../wailsjs/go/main/App')
    CreateTerminal.mockRejectedValueOnce(new Error('pty fail'))
    const { EventsOff } = await import('../../../wailsjs/runtime/runtime')

    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    captures.offFns.forEach((off) => expect(off).toHaveBeenCalledTimes(1))
    // 全局 EventsOff 不得被调用（会误删其他实例监听器）
    expect(EventsOff).not.toHaveBeenCalled()
  })

  it('initTerminal 传递 fontSize/scrollback options 至 xterm 配置', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell', { fontSize: 16, scrollback: 5000 })
    await flushPromises()
    expect(captures.lastTerminalConfig.fontSize).toBe(16)
    expect(captures.lastTerminalConfig.scrollback).toBe(5000)
  })

  it('未传 options 时使用默认外观（fontSize 14 / scrollback 5000，与后端默认常量对齐）', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.lastTerminalConfig.fontSize).toBe(14)
    expect(captures.lastTerminalConfig.scrollback).toBe(5000)
  })

  it('两个实例并存：各自注册独立监听器，销毁其一不影响另一实例', async () => {
    const t1 = useTerminal()
    const t2 = useTerminal()
    await t1.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await t2.initTerminal(document.createElement('div'), 'D:\\', 'powershell')
    await flushPromises()
    expect(captures.offFns.length).toBe(4)

    await t1.destroyTerminal()
    // 仅 t1 的两个闭包被调用，t2 的保持未调用
    const calledCount = captures.offFns.filter((off) => off.mock.calls.length > 0).length
    expect(calledCount).toBe(2)
  })
})

describe('useTerminal - 外观设置热更（PR5/R5）', () => {
  let settingsStore

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    captures.lastTerminal = null
    captures.lastTerminalConfig = null
    captures.lastFit = null
    settingsStore = useSettingsStore()
  })

  it('initTerminal 未传 options 时从 settings store 读取外观三字段', async () => {
    settingsStore.terminalFontSize = 18
    settingsStore.terminalFontFamily = 'Consolas'
    settingsStore.terminalScrollback = 5000
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.lastTerminalConfig.fontSize).toBe(18)
    expect(captures.lastTerminalConfig.fontFamily).toBe('"Consolas", Consolas, "Courier New", monospace')
    expect(captures.lastTerminalConfig.scrollback).toBe(5000)
  })

  it('initTerminal 未配置字体时使用默认 Cascadia Code 栈', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.lastTerminalConfig.fontFamily).toBe('"Cascadia Code", "Fira Code", Consolas, "Courier New", monospace')
  })

  it('options 显式传值优先于 store 外观配置', async () => {
    settingsStore.terminalFontSize = 18
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell', { fontSize: 12 })
    await flushPromises()
    expect(captures.lastTerminalConfig.fontSize).toBe(12)
  })

  it('字号变化热更 term.options.fontSize 并 refit（fit + ResizeTerminal 链路）', async () => {
    const { ResizeTerminal } = await import('../../../wailsjs/go/main/App')
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.lastTerminal.options.fontSize).toBe(14)
    captures.lastFit.fit.mockClear()
    ResizeTerminal.mockClear()

    settingsStore.terminalFontSize = 20
    await nextTick()
    expect(captures.lastTerminal.options.fontSize).toBe(20)
    // 字号变化改变 cols/rows 计算：refit + 通知后端 PTY 同步尺寸
    expect(captures.lastFit.fit).toHaveBeenCalled()
    expect(ResizeTerminal).toHaveBeenCalled()
  })

  it('字体变化热更 term.options.fontFamily', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    settingsStore.terminalFontFamily = 'Courier New'
    await nextTick()
    expect(captures.lastTerminal.options.fontFamily).toBe('"Courier New", Consolas, "Courier New", monospace')
  })

  it('终端未初始化时外观变化不报错（term.value 为 null）', async () => {
    const t = useTerminal()
    settingsStore.terminalFontSize = 20
    await nextTick()
    expect(t.term.value).toBeNull()
  })
})

describe('useTerminal - addon 生命周期（WebGL 回退 / 搜索开关 / serialize 回放）', () => {
  let settingsStore

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    captures.lastTerminal = null
    captures.lastTerminalConfig = null
    captures.lastFit = null
    captures.offFns = []
    captures.lastWebgl = null
    captures.webglContextLossCb = null
    captures.lastSearch = null
    captures.lastSerialize = null
    captures.serializedText = ''
    captures.linkProvider = null
    captures.lastOutputCb = null
    // WebGL 探测结果为模块级单例缓存：复位后本用例组 stub 的探测结果生效
    resetWebglProbeCacheForTests()
    // WebGL 支持性探测门控：stub canvas 返回真值上下文（jsdom 无 canvas 包恒 null）
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({})
    settingsStore = useSettingsStore()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('WebglAddon 构造抛错时终端仍创建成功（回退 DOM 渲染不阻塞）', async () => {
    const { WebglAddon } = await import('@xterm/addon-webgl')
    WebglAddon.mockImplementationOnce(() => { throw new Error('no webgl context') })
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(t.term.value).not.toBeNull()
    expect(t.isActive.value).toBe(true)
  })

  it('canvas 探测不支持 WebGL 时跳过挂载（直接 DOM 渲染，不构建 addon）', async () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
    const { WebglAddon } = await import('@xterm/addon-webgl')
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(WebglAddon).not.toHaveBeenCalled()
    expect(t.term.value).not.toBeNull()
    expect(t.isActive.value).toBe(true)
  })

  it('WebGL 探测结果模块级缓存：二次 init 不再创建探测 canvas（GL 上下文泄漏防御）', async () => {
    const canvasSpy = vi.spyOn(HTMLCanvasElement.prototype, 'getContext')
    const t1 = useTerminal()
    await t1.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    await t1.destroyTerminal()
    const countAfterFirst = canvasSpy.mock.calls.length
    const t2 = useTerminal()
    await t2.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    // 缓存生效：第二次 init 不再建探测 canvas（浏览器 GL 上下文有 ~16 活跃上限）
    expect(canvasSpy.mock.calls.length).toBe(countAfterFirst)
  })

  it('WebGL 挂载成功后注册上下文丢失回调，触发时 dispose addon 回退 DOM 渲染', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.lastWebgl).not.toBeNull()
    expect(captures.lastWebgl.onContextLoss).toHaveBeenCalled()
    captures.webglContextLossCb()
    expect(captures.lastWebgl.dispose).toHaveBeenCalledTimes(1)
    // 终端本体不受影响（term 不重建）
    expect(t.term.value).not.toBeNull()
  })

  it('openSearchAddon 惰性挂载且幂等，disposeSearchAddon 释放后重建为新实例', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    const addon1 = t.openSearchAddon()
    expect(addon1).not.toBeNull()
    expect(captures.lastTerminal.loadAddon).toHaveBeenCalledWith(addon1)
    // 幂等：二次呼出返回同一实例（不重复挂载）
    expect(t.openSearchAddon()).toBe(addon1)
    // 关闭释放：dispose 被调用且实例清空
    t.disposeSearchAddon()
    expect(captures.lastSearch.dispose).toHaveBeenCalledTimes(1)
    // 再次呼出：新实例（旧 addon 已释放）
    const addon2 = t.openSearchAddon()
    expect(addon2).not.toBe(addon1)
  })

  it('searchFindNext/searchFindPrevious 透传关键词与选项至 SearchAddon', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    await t.searchFindNext('foo', { caseSensitive: true })
    expect(captures.lastSearch.findNext).toHaveBeenCalledWith('foo', { caseSensitive: true })
    await t.searchFindPrevious('bar')
    expect(captures.lastSearch.findPrevious).toHaveBeenCalledWith('bar', undefined)
  })

  it('destroyTerminal 释放搜索 addon', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    t.openSearchAddon()
    await t.destroyTerminal()
    expect(captures.lastSearch.dispose).toHaveBeenCalledTimes(1)
    // 销毁后呼出不再挂载（终端已不存在）
    expect(t.openSearchAddon()).toBeNull()
  })

  it('Ctrl+F 拦截：preventDefault + 触发面板回调 + 返回 false 阻止字符写入', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    let called = 0
    t.setSearchShortcutHandler(() => { called++ })
    const ev = { type: 'keydown', ctrlKey: true, metaKey: false, altKey: false, shiftKey: false, key: 'f', preventDefault: vi.fn() }
    const ret = captures.searchShortcutCb(ev)
    expect(ret).toBe(false)
    expect(ev.preventDefault).toHaveBeenCalledTimes(1)
    expect(called).toBe(1)
  })

  it('无面板回调时 Ctrl+F 放行（不劫持浏览器默认查找）', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    const ev = { type: 'keydown', ctrlKey: true, metaKey: false, altKey: false, shiftKey: false, key: 'f', preventDefault: vi.fn() }
    const ret = captures.searchShortcutCb(ev)
    expect(ret).toBe(true)
    expect(ev.preventDefault).not.toHaveBeenCalled()
  })

  it('keyup 与其余按键放行（Ctrl+F 仅在 keydown 拦截一次）', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    let called = 0
    t.setSearchShortcutHandler(() => { called++ })
    expect(captures.searchShortcutCb({ type: 'keyup', ctrlKey: true, key: 'f', preventDefault: vi.fn() })).toBe(true)
    // Ctrl+Shift+F / Alt+F / 普通按键不拦截
    expect(captures.searchShortcutCb({ type: 'keydown', ctrlKey: true, shiftKey: true, altKey: false, key: 'f', preventDefault: vi.fn() })).toBe(true)
    expect(captures.searchShortcutCb({ type: 'keydown', ctrlKey: true, altKey: true, shiftKey: false, key: 'f', preventDefault: vi.fn() })).toBe(true)
    expect(captures.searchShortcutCb({ type: 'keydown', ctrlKey: false, altKey: false, shiftKey: false, key: 'a', preventDefault: vi.fn() })).toBe(true)
    expect(called).toBe(0)
  })

  it('restartTerminal 在旧实例 dispose 之前序列化，新实例收到旧缓冲文本回放', async () => {
    captures.serializedText = 'old-buffer-lines'
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    const oldTerminal = captures.lastTerminal
    await t.restartTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    const newTerminal = captures.lastTerminal
    expect(newTerminal).not.toBe(oldTerminal)
    // 序列化 addon 挂到旧实例（dispose 前取文本——dispose 后缓冲即销毁）
    expect(oldTerminal.loadAddon).toHaveBeenCalledWith(captures.lastSerialize)
    // 时序：serializer 挂载先于旧实例 dispose
    const serializerCallIdx = oldTerminal.loadAddon.mock.calls.findIndex(c => c[0] === captures.lastSerialize)
    const serializerOrder = oldTerminal.loadAddon.mock.invocationCallOrder[serializerCallIdx]
    expect(serializerOrder).toBeLessThan(oldTerminal.dispose.mock.invocationCallOrder[0])
    // 新实例 write 收到旧缓冲文本（scrollback 内存回放）
    expect(newTerminal.write).toHaveBeenCalledWith('old-buffer-lines')
  })

  it('回放文本先于暂存新输出写入（新 shell banner 不得排在旧缓冲之前）', async () => {
    captures.serializedText = 'old-buffer-lines'
    const { CreateTerminal } = await import('../../../wailsjs/go/main/App')
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    // 挂起 restart 内的 CreateTerminal：模拟 PTY 创建窗口期，期间新 shell 输出先到（进暂存缓冲）
    let resolveCreate
    CreateTerminal.mockImplementationOnce(() => new Promise((res) => { resolveCreate = res }))
    const restartPromise = t.restartTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    // 新会话首条输出先于 CreateTerminal 返回到达（sessionID 未设 → 暂存缓冲）
    captures.lastOutputCb('sid-2', 'new-shell-banner')
    resolveCreate('sid-2')
    await restartPromise
    await flushPromises()
    // 回放文本必须先于暂存缓冲刷新（旧缓冲在上、新输出在下，顺序不错乱）
    const writes = captures.lastTerminal.write.mock.calls.map(c => c[0])
    expect(writes.indexOf('old-buffer-lines')).toBeGreaterThanOrEqual(0)
    expect(writes.indexOf('old-buffer-lines')).toBeLessThan(writes.indexOf('new-shell-banner'))
  })

  it('序列化抛错时回放静默跳过，新会话照常创建不阻塞', async () => {
    const { SerializeAddon } = await import('@xterm/addon-serialize')
    SerializeAddon.mockImplementationOnce(() => { throw new Error('serialize fail') })
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    await t.restartTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(t.term.value).not.toBeNull()
    expect(t.isActive.value).toBe(true)
    // 回放跳过：新实例不写入旧文本
    expect(captures.lastTerminal.write).not.toHaveBeenCalledWith('old-buffer-lines')
  })

  it('initTerminal 注册文件路径 link provider（Ctrl+Click 打开路径）', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.linkProvider).not.toBeNull()
    expect(captures.linkProvider.provideLinks).toBeTypeOf('function')
  })
})

describe('useTerminal - OSC 通知（R3）', () => {
  let settingsStore
  let notifyMock

  beforeEach(async () => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    captures.lastTerminal = null
    captures.oscHandlers = {}
    notifyMock = (await import('element-plus')).ElNotification
    settingsStore = useSettingsStore()
  })

  it('开关关闭（默认）时 OSC 9 放行不提示', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.oscHandlers[9]).toBeTypeOf('function')
    const ret = captures.oscHandlers[9]('build done')
    expect(ret).toBe(false)
    expect(notifyMock).not.toHaveBeenCalled()
  })

  it('开关开启后 OSC 9 转应用内提示（title 回退「终端通知」）', async () => {
    settingsStore.terminalOscNotifyEnabled = true
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    const ret = captures.oscHandlers[9]('build done')
    expect(ret).toBe(true)
    expect(notifyMock).toHaveBeenCalledWith({ title: '终端通知', message: 'build done', type: 'info', duration: 5000 })
  })

  it('开关切换对已初始化终端即时生效（handler 实时读取，无需重建终端）', async () => {
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.oscHandlers[9]('msg-a')).toBe(false)
    settingsStore.terminalOscNotifyEnabled = true
    expect(captures.oscHandlers[9]('msg-a')).toBe(true)
    settingsStore.terminalOscNotifyEnabled = false
    expect(captures.oscHandlers[9]('msg-a')).toBe(false)
  })

  it('OSC 9;4 进度条子类型放行（xterm 内部处理，不转通知）', async () => {
    settingsStore.terminalOscNotifyEnabled = true
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.oscHandlers[9]('4;1;50')).toBe(false)
    expect(notifyMock).not.toHaveBeenCalled()
  })

  it('OSC 777 notify;title;body 解析（无 tab 标题时用载荷 title）', async () => {
    settingsStore.terminalOscNotifyEnabled = true
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    const ret = captures.oscHandlers[777]('notify;构建任务;编译完成')
    expect(ret).toBe(true)
    expect(notifyMock).toHaveBeenCalledWith({ title: '构建任务', message: '编译完成', type: 'info', duration: 5000 })
  })

  it('OSC 777 非 notify 子类型放行', async () => {
    settingsStore.terminalOscNotifyEnabled = true
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.oscHandlers[777]('other;foo')).toBe(false)
    expect(notifyMock).not.toHaveBeenCalled()
  })

  it('OSC 99 kitty 双段与含元数据段解析', async () => {
    settingsStore.terminalOscNotifyEnabled = true
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    expect(captures.oscHandlers[99]('部署完成;artifact.zip')).toBe(true)
    expect(notifyMock).toHaveBeenCalledWith(expect.objectContaining({ title: '部署完成', message: 'artifact.zip' }))
    // 首段含 '=' 判为 kitty 元数据（i=xxx:p=1），标题取第二段
    expect(captures.oscHandlers[99]('i=1:p=1;部署完成;artifact.zip')).toBe(true)
    expect(notifyMock).toHaveBeenCalledWith(expect.objectContaining({ title: '部署完成', message: 'artifact.zip' }))
  })

  it('同实例同内容节流：5s 窗口内只提示一次', async () => {
    settingsStore.terminalOscNotifyEnabled = true
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    captures.oscHandlers[9]('same msg')
    captures.oscHandlers[9]('same msg')
    captures.oscHandlers[9]('same msg')
    expect(notifyMock).toHaveBeenCalledTimes(1)
    // 不同内容不受节流影响
    captures.oscHandlers[9]('other msg')
    expect(notifyMock).toHaveBeenCalledTimes(2)
  })

  it('destroyTerminal 置空通知器后缓冲 OSC 触发 handler 放行不抛错（销毁竞态空守卫）', async () => {
    settingsStore.terminalOscNotifyEnabled = true
    const t = useTerminal()
    await t.initTerminal(document.createElement('div'), 'C:\\', 'powershell')
    await flushPromises()
    const handlers = captures.oscHandlers
    // destroyTerminal 不等完成：同步段（置空 oscNotifier）先于首个 await
    // CloseTerminal 执行，此刻缓冲未 flush 的 OSC 到达触发 handler
    const destroyPromise = t.destroyTerminal()
    for (const ident of [9, 777, 99]) {
      // 空守卫放行（返回 false 不消费），不抛 TypeError 打穿 parser 写链
      expect(handlers[ident]('late msg')).toBe(false)
    }
    expect(notifyMock).not.toHaveBeenCalled()
    await destroyPromise
  })
})

describe('useTerminal - 多行粘贴确认（R6，容器捕获阶段拦截 DOM paste）', () => {
  let ElMessageBox

  /** 构造携带 clipboardData 的 paste 事件（jsdom 无 ClipboardEvent 数据构造器） */
  function makePasteEvent(text) {
    const ev = new Event('paste', { bubbles: true, cancelable: true })
    ev.clipboardData = { getData: () => text }
    return ev
  }

  beforeEach(async () => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    captures.lastTerminal = null
    ElMessageBox = (await import('element-plus')).ElMessageBox
    ElMessageBox.confirm.mockResolvedValue(undefined)
  })

  it('单行粘贴直通：不拦截不弹确认框（xterm 默认链路写入）', async () => {
    const container = document.createElement('div')
    const t = useTerminal()
    await t.initTerminal(container, 'C:\\', 'powershell')
    await flushPromises()
    const ev = makePasteEvent('git status')
    container.dispatchEvent(ev)
    // 未拦截：事件保持可默认传播，走 xterm 原生 paste 链
    expect(ev.defaultPrevented).toBe(false)
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
    expect(captures.lastTerminal.paste).not.toHaveBeenCalled()
  })

  it('含换行粘贴拦截：preventDefault + 弹单次确认框（风险文案 + 粘贴按钮）', async () => {
    const container = document.createElement('div')
    const t = useTerminal()
    await t.initTerminal(container, 'C:\\', 'powershell')
    await flushPromises()
    const ev = makePasteEvent('git status\nnpm test')
    container.dispatchEvent(ev)
    expect(ev.defaultPrevented).toBe(true)
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    const [message, title, opts] = ElMessageBox.confirm.mock.calls[0]
    expect(message).toContain('多行')
    expect(title).toBe('粘贴多行内容')
    expect(opts.confirmButtonText).toBe('粘贴')
    expect(opts.cancelButtonText).toBe('取消')
    expect(captures.lastTerminal.paste).not.toHaveBeenCalled()
  })

  it('确认后经 terminal.paste 原生链路写入（保留 \n→\\r 归一与 bracketed paste 语义）', async () => {
    const container = document.createElement('div')
    const t = useTerminal()
    await t.initTerminal(container, 'C:\\', 'powershell')
    await flushPromises()
    container.dispatchEvent(makePasteEvent('echo a\recho b'))
    await flushPromises()
    expect(captures.lastTerminal.paste).toHaveBeenCalledWith('echo a\recho b')
  })

  it('取消后不写入终端', async () => {
    ElMessageBox.confirm.mockRejectedValueOnce('cancel')
    const container = document.createElement('div')
    const t = useTerminal()
    await t.initTerminal(container, 'C:\\', 'powershell')
    await flushPromises()
    container.dispatchEvent(makePasteEvent('rm -rf /tmp/x\nrm -rf /tmp/y'))
    await flushPromises()
    expect(captures.lastTerminal.paste).not.toHaveBeenCalled()
  })

  it('destroyTerminal 摘除容器 paste 监听（销毁后粘贴不再拦截）', async () => {
    const container = document.createElement('div')
    const t = useTerminal()
    await t.initTerminal(container, 'C:\\', 'powershell')
    await flushPromises()
    await t.destroyTerminal()
    const ev = makePasteEvent('a\nb')
    container.dispatchEvent(ev)
    // 监听已摘除：事件未被拦截
    expect(ev.defaultPrevented).toBe(false)
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
  })
})
