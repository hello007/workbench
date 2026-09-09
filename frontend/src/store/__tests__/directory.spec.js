import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetDirectories: vi.fn(() => Promise.resolve([])),
  RefreshDirectoriesGitFlag: vi.fn(() => Promise.resolve([]))
}))

import { useDirectoryStore } from '..'
import { GetDirectories, RefreshDirectoriesGitFlag } from '../../../wailsjs/go/main/App'

describe('directory store', () => {
  beforeEach(() => {
    // 每个 it 独立 pinia 实例 + 清理 mock 历史 + 重置默认实现
    setActivePinia(createPinia())
    vi.clearAllMocks()
    GetDirectories.mockResolvedValue([])
    RefreshDirectoriesGitFlag.mockResolvedValue([])
  })

  describe('初始状态', () => {
    it('directories 空数组，selectedDirectoryId 空串', () => {
      const store = useDirectoryStore()
      expect(store.directories).toEqual([])
      expect(store.selectedDirectoryId).toBe('')
    })

    it('未选中目录时 currentDirPath 返回空串', () => {
      const store = useDirectoryStore()
      expect(store.currentDirPath).toBe('')
    })
  })

  describe('currentDirPath computed', () => {
    it('selectedDirectoryId 命中时返回对应目录 path', () => {
      const store = useDirectoryStore()
      store.directories = [
        { id: '1', path: 'C:\\a' },
        { id: '2', path: 'C:\\b' }
      ]
      store.selectedDirectoryId = '2'
      expect(store.currentDirPath).toBe('C:\\b')
    })

    it('selectedDirectoryId 未命中任何目录时返回空串', () => {
      const store = useDirectoryStore()
      store.directories = [{ id: '1', path: 'C:\\a' }]
      store.selectedDirectoryId = 'not-exist'
      expect(store.currentDirPath).toBe('')
    })
  })

  describe('loadDirectories', () => {
    it('加载目录列表并自动选中默认目录', async () => {
      GetDirectories.mockResolvedValue([
        { id: '1', isDefault: true, path: 'C:\\a' },
        { id: '2', path: 'C:\\b' }
      ])
      const store = useDirectoryStore()
      await store.loadDirectories()
      expect(store.directories.length).toBe(2)
      expect(store.selectedDirectoryId).toBe('1')
    })

    it('无默认目录时选中首个', async () => {
      GetDirectories.mockResolvedValue([
        { id: 'x', path: 'C:\\x' },
        { id: 'y', path: 'C:\\y' }
      ])
      const store = useDirectoryStore()
      await store.loadDirectories()
      expect(store.selectedDirectoryId).toBe('x')
    })

    it('返回空列表时不选中任何目录', async () => {
      GetDirectories.mockResolvedValue([])
      const store = useDirectoryStore()
      await store.loadDirectories()
      expect(store.directories).toEqual([])
      expect(store.selectedDirectoryId).toBe('')
    })
  })

  describe('refreshGitFlags', () => {
    it('返回非空新列表时替换 directories', async () => {
      const store = useDirectoryStore()
      store.directories = [{ id: '1', path: 'C:\\a', isGitRepo: false }]
      RefreshDirectoriesGitFlag.mockResolvedValue([
        { id: '1', path: 'C:\\a', isGitRepo: true }
      ])
      await store.refreshGitFlags()
      expect(store.directories.length).toBe(1)
      expect(store.directories[0].isGitRepo).toBe(true)
    })

    it('返回空列表时不替换（保留缓存值）', async () => {
      const store = useDirectoryStore()
      store.directories = [{ id: '1', path: 'C:\\a' }]
      RefreshDirectoriesGitFlag.mockResolvedValue([])
      await store.refreshGitFlags()
      expect(store.directories.length).toBe(1)
    })

    it('返回 null 时不替换', async () => {
      const store = useDirectoryStore()
      store.directories = [{ id: '1', path: 'C:\\a' }]
      RefreshDirectoriesGitFlag.mockResolvedValue(null)
      await store.refreshGitFlags()
      expect(store.directories.length).toBe(1)
    })

    it('刷新抛错时保留缓存值不影响主流程', async () => {
      const store = useDirectoryStore()
      store.directories = [{ id: '1', path: 'C:\\a' }]
      RefreshDirectoriesGitFlag.mockRejectedValue(new Error('fail'))
      await store.refreshGitFlags()
      expect(store.directories.length).toBe(1)
    })
  })
})
