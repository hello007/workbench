import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUiStore } from '..'

describe('ui store', () => {
  beforeEach(() => {
    // 每个 it 独立 pinia 实例，避免 store 状态跨用例污染
    setActivePinia(createPinia())
  })

  describe('初始状态', () => {
    it('活动面板默认 directory', () => {
      const store = useUiStore()
      expect(store.activePanel).toBe('directory')
    })

    it('终端默认隐藏，高度 200，跟随目录为空', () => {
      const store = useUiStore()
      expect(store.terminalVisible).toBe(false)
      expect(store.terminalHeight).toBe(200)
      expect(store.terminalDir).toBe('')
    })

    it('全部弹窗 visible 默认 false', () => {
      const store = useUiStore()
      expect(store.settingsVisible).toBe(false)
      expect(store.updateDialogVisible).toBe(false)
      expect(store.commandPaletteVisible).toBe(false)
      expect(store.repoFilterVisible).toBe(false)
    })

    it('updateInfo 默认空对象，其余派生态默认空', () => {
      const store = useUiStore()
      expect(store.updateInfo).toEqual({})
      expect(store.contentSearchInit).toBe('')
      expect(store.repoFilterInitialDirId).toBe('')
      expect(store.appVersion).toBe('')
    })
  })

  describe('toggleTerminal', () => {
    it('连续切换 false → true → false', () => {
      const store = useUiStore()
      expect(store.terminalVisible).toBe(false)
      store.toggleTerminal()
      expect(store.terminalVisible).toBe(true)
      store.toggleTerminal()
      expect(store.terminalVisible).toBe(false)
    })
  })

  describe('openRepoFilter', () => {
    it('携带 dirId 时锁定到该目录并打开弹窗', () => {
      const store = useUiStore()
      store.openRepoFilter('dir-1')
      expect(store.repoFilterInitialDirId).toBe('dir-1')
      expect(store.repoFilterVisible).toBe(true)
    })

    it('无 dirId 时显式重置 initialDirId 为空（避免残留上次右键锁定目录）', () => {
      const store = useUiStore()
      // 先模拟上次右键锁定一个目录
      store.openRepoFilter('dir-stale')
      expect(store.repoFilterInitialDirId).toBe('dir-stale')
      // 再以无 dirId 入口打开（FileTreePanel 空白右键 / 工具栏按钮）
      store.openRepoFilter()
      expect(store.repoFilterInitialDirId).toBe('')
      expect(store.repoFilterVisible).toBe(true)
    })
  })
})
