import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetClaudeDefaultModel: vi.fn(() => Promise.resolve('')),
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

  describe('loadDirectories 选中保留与回落', () => {
    // 聚合视图下经 DirectoryTree 增删/重命名/设默认工作目录后刷新列表（emit('change')
    // → Home.vue @change → loadDirectories），刷新不得隐式改写仍有效的选中：
    // 隐式跳变会造成选中态与聚合模式错配（高亮跳变、clipboard 被清、快照污染），
    // 普通树则触发 treeKey 变化重建 el-tree。重命名不改 id，与「仍存在」同构覆盖。
    it('当前选中仍存在时保留选中（不被默认目录抢占）', async () => {
      GetDirectories.mockResolvedValue([
        { id: '1', isDefault: true, path: 'C:\\a' },
        { id: '2', path: 'C:\\b' }
      ])
      const store = useDirectoryStore()
      store.selectedDirectoryId = '2'
      await store.loadDirectories()
      expect(store.selectedDirectoryId).toBe('2')
    })

    it('列表重排后保留选中（拖拽排序保存失败重试的刷新）', async () => {
      GetDirectories.mockResolvedValue([
        { id: 'y', path: 'C:\\y' },
        { id: 'x', path: 'C:\\x' }
      ])
      const store = useDirectoryStore()
      store.selectedDirectoryId = 'x'
      await store.loadDirectories()
      expect(store.selectedDirectoryId).toBe('x')
    })

    it('当前选中不存在时回落默认目录（如选中目录被删除）', async () => {
      GetDirectories.mockResolvedValue([
        { id: '1', isDefault: true, path: 'C:\\a' },
        { id: '2', path: 'C:\\b' }
      ])
      const store = useDirectoryStore()
      store.selectedDirectoryId = 'gone'
      await store.loadDirectories()
      expect(store.selectedDirectoryId).toBe('1')
    })

    it('当前选中不存在且无默认时回落首个', async () => {
      GetDirectories.mockResolvedValue([
        { id: 'x', path: 'C:\\x' },
        { id: 'y', path: 'C:\\y' }
      ])
      const store = useDirectoryStore()
      store.selectedDirectoryId = 'gone'
      await store.loadDirectories()
      expect(store.selectedDirectoryId).toBe('x')
    })

    it('目录删光后清空脏选中（现状残留指向已删除目录的脏值）', async () => {
      GetDirectories.mockResolvedValue([])
      const store = useDirectoryStore()
      store.selectedDirectoryId = 'gone'
      await store.loadDirectories()
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
