import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import {
  buildSessionState,
  applySessionState,
  restoreSession,
  startSessionAutoSave
} from '../useSessionState'
import { useDirectoryStore, useUiStore } from '../../store'

// 复用 setup.js 全局 mock（GetSessionState→null / SaveSessionState→true 已注入 wails-mock-defaults）
// 用 vi.importMock 拿到 mock 引用并按用例 override

describe('useSessionState - buildSessionState', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('从 store 读取当前状态构建快照', () => {
    const directoryStore = useDirectoryStore()
    const uiStore = useUiStore()
    directoryStore.selectedDirectoryId = 'dir-1'
    uiStore.activePanel = 'ai'
    uiStore.terminalVisible = true
    uiStore.terminalHeight = 240
    uiStore.terminalDir = 'D:/repo'

    const snapshot = buildSessionState()
    expect(snapshot.selectedDirectoryId).toBe('dir-1')
    expect(snapshot.activePanel).toBe('ai')
    expect(snapshot.terminal).toEqual({ visible: true, height: 240, workDir: 'D:/repo' })
  })

  it('空状态构建为缺省值快照', () => {
    const snapshot = buildSessionState()
    expect(snapshot.selectedDirectoryId).toBe('')
    expect(snapshot.activePanel).toBe('directory') // ui store 默认 activePanel
    // terminalHeight 默认 200（ui store 初始值），visible/workDir 缺省
    expect(snapshot.terminal).toEqual({ visible: false, height: 200, workDir: '' })
  })
})

describe('useSessionState - applySessionState', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('恢复合法 activePanel 与终端状态', () => {
    const uiStore = useUiStore()
    const touched = applySessionState({
      activePanel: 'stats',
      terminal: { visible: true, height: 300, workDir: 'D:/work' }
    })
    expect(touched).toBe(true)
    expect(uiStore.activePanel).toBe('stats')
    expect(uiStore.terminalVisible).toBe(true)
    expect(uiStore.terminalHeight).toBe(300)
    expect(uiStore.terminalDir).toBe('D:/work')
  })

  it('非法 activePanel 被忽略（保持默认 directory）', () => {
    const uiStore = useUiStore()
    const touched = applySessionState({ activePanel: 'invalid-panel' })
    expect(touched).toBe(false)
    expect(uiStore.activePanel).toBe('directory')
  })

  it('null 快照不恢复且返回 false（冷启动）', () => {
    const uiStore = useUiStore()
    uiStore.activePanel = 'ai'
    const touched = applySessionState(null)
    expect(touched).toBe(false)
    expect(uiStore.activePanel).toBe('ai')
  })

  it('terminal 子对象缺失时不恢复终端', () => {
    const uiStore = useUiStore()
    const touched = applySessionState({ activePanel: 'toolbox' })
    expect(touched).toBe(true)
    expect(uiStore.terminalVisible).toBe(false)
  })

  it('terminal.visible 为 false 时不标记终端恢复', () => {
    const uiStore = useUiStore()
    const touched = applySessionState({ terminal: { visible: false, height: 0, workDir: '' } })
    expect(touched).toBe(false)
  })
})

describe('useSessionState - restoreSession', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('GetSessionState 返回完整快照时透传', async () => {
    const App = await vi.importMock('../../../wailsjs/go/main/App')
    App.GetSessionState.mockResolvedValueOnce({
      selectedDirectoryId: 'dir-1',
      activePanel: 'ai',
      terminal: { visible: true, height: 200, workDir: 'D:/r' }
    })
    const state = await restoreSession()
    expect(state).not.toBeNull()
    expect(state.selectedDirectoryId).toBe('dir-1')
  })

  it('GetSessionState 返回 null 时走冷启动（返回 null）', async () => {
    const App = await vi.importMock('../../../wailsjs/go/main/App')
    App.GetSessionState.mockResolvedValueOnce(null)
    const state = await restoreSession()
    expect(state).toBeNull()
  })

  it('GetSessionState 返回空快照（全字段缺省）时走冷启动', async () => {
    const App = await vi.importMock('../../../wailsjs/go/main/App')
    App.GetSessionState.mockResolvedValueOnce({})
    const state = await restoreSession()
    expect(state).toBeNull()
  })

  it('GetSessionState 抛异常时静默走冷启动（不阻塞启动）', async () => {
    const App = await vi.importMock('../../../wailsjs/go/main/App')
    App.GetSessionState.mockRejectedValueOnce(new Error('boom'))
    const state = await restoreSession()
    expect(state).toBeNull()
  })
})

describe('useSessionState - startSessionAutoSave', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('markRestored 前状态变化不触发保存（避免恢复阶段覆盖）', async () => {
    const App = await vi.importMock('../../../wailsjs/go/main/App')
    App.SaveSessionState.mockClear()
    const directoryStore = useDirectoryStore()
    const handle = startSessionAutoSave()

    // 恢复未完成时改状态
    directoryStore.selectedDirectoryId = 'dir-1'
    await vi.advanceTimersByTimeAsync(3000)
    expect(App.SaveSessionState).not.toHaveBeenCalled()

    handle.dispose()
  })

  it('markRestored 后状态变化经 debounce 触发一次保存', async () => {
    const App = await vi.importMock('../../../wailsjs/go/main/App')
    App.SaveSessionState.mockClear()
    const directoryStore = useDirectoryStore()
    const handle = startSessionAutoSave()
    handle.markRestored()

    directoryStore.selectedDirectoryId = 'dir-2'
    // debounce 间隔内多次变化合并为一次（Vue watch 批处理同步变更）
    directoryStore.selectedDirectoryId = 'dir-3'
    await vi.advanceTimersByTimeAsync(1999)
    expect(App.SaveSessionState).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(2)
    expect(App.SaveSessionState).toHaveBeenCalledTimes(1)
    // 透传 buildSessionState 的快照
    const arg = App.SaveSessionState.mock.calls[0][0]
    expect(arg.selectedDirectoryId).toBe('dir-3')

    handle.dispose()
  })

  it('dispose 后状态变化不再触发保存', async () => {
    const App = await vi.importMock('../../../wailsjs/go/main/App')
    App.SaveSessionState.mockClear()
    const directoryStore = useDirectoryStore()
    const handle = startSessionAutoSave()
    handle.markRestored()
    handle.dispose()

    directoryStore.selectedDirectoryId = 'dir-x'
    await vi.advanceTimersByTimeAsync(3000)
    expect(App.SaveSessionState).not.toHaveBeenCalled()
  })

  it('terminal 状态变化触发保存', async () => {
    const App = await vi.importMock('../../../wailsjs/go/main/App')
    App.SaveSessionState.mockClear()
    const uiStore = useUiStore()
    const handle = startSessionAutoSave()
    handle.markRestored()

    uiStore.terminalVisible = true
    await vi.advanceTimersByTimeAsync(2000)
    expect(App.SaveSessionState).toHaveBeenCalledTimes(1)
    const arg = App.SaveSessionState.mock.calls[0][0]
    expect(arg.terminal.visible).toBe(true)

    handle.dispose()
  })

  it('beforeunload 触发最终保存（best-effort flush）', async () => {
    const App = await vi.importMock('../../../wailsjs/go/main/App')
    App.SaveSessionState.mockClear()
    const directoryStore = useDirectoryStore()
    const handle = startSessionAutoSave()
    handle.markRestored()
    directoryStore.selectedDirectoryId = 'dir-1'
    // 未到 debounce 间隔即触发 beforeunload → flush 立即保存
    await vi.advanceTimersByTimeAsync(500)
    expect(App.SaveSessionState).not.toHaveBeenCalled()
    window.dispatchEvent(new Event('beforeunload'))
    expect(App.SaveSessionState).toHaveBeenCalledTimes(1)

    handle.dispose()
  })
})
