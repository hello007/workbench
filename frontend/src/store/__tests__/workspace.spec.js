import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useWorkspaceStore } from '..'

describe('workspace store', () => {
  beforeEach(() => {
    // 每个 it 独立 pinia 实例，避免 store 状态跨用例污染
    setActivePinia(createPinia())
  })

  describe('初始状态', () => {
    it('selectedNode 与 latestCommit 默认 null', () => {
      const store = useWorkspaceStore()
      expect(store.selectedNode).toBeNull()
      expect(store.latestCommit).toBeNull()
    })

    it('lastInteractedTree 默认 directory', () => {
      const store = useWorkspaceStore()
      expect(store.lastInteractedTree).toBe('directory')
    })

    it('clipboard 默认 mode null 且源字段全空', () => {
      const store = useWorkspaceStore()
      expect(store.clipboard.mode).toBeNull()
      expect(store.clipboard.sourcePath).toBe('')
      expect(store.clipboard.sourceName).toBe('')
      expect(store.clipboard.sourceType).toBe('')
    })
  })

  describe('clearClipboard', () => {
    it('剪切态调用后 4 字段全重置', () => {
      const store = useWorkspaceStore()
      store.clipboard.mode = 'cut'
      store.clipboard.sourcePath = 'C:\\src'
      store.clipboard.sourceName = 'src'
      store.clipboard.sourceType = 'directory'
      store.clearClipboard()
      expect(store.clipboard.mode).toBeNull()
      expect(store.clipboard.sourcePath).toBe('')
      expect(store.clipboard.sourceName).toBe('')
      expect(store.clipboard.sourceType).toBe('')
    })

    it('复制态调用后 mode 归 null', () => {
      const store = useWorkspaceStore()
      store.clipboard.mode = 'copy'
      store.clipboard.sourcePath = 'C:\\file.js'
      store.clearClipboard()
      expect(store.clipboard.mode).toBeNull()
    })
  })

  describe('clipboard reactive 直读直写', () => {
    it('直写 clipboard.mode 后直读得到新值', () => {
      const store = useWorkspaceStore()
      store.clipboard.mode = 'copy'
      expect(store.clipboard.mode).toBe('copy')
      // 再次改写验证可重复直写
      store.clipboard.mode = 'cut'
      expect(store.clipboard.mode).toBe('cut')
    })
  })
})
