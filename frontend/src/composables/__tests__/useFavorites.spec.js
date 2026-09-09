import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useFavoritesStore } from '../../store'

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetFavorites: vi.fn(() => Promise.resolve([
    { path: 'C:\\projects\\app', alias: 'My App', group: '默认', createdAt: 1000 }
  ])),
  AddFavorite: vi.fn(() => Promise.resolve('')),
  RemoveFavorite: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteAlias: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteGroup: vi.fn(() => Promise.resolve(''))
}))

describe('favorites store', () => {
  beforeEach(() => {
    // 每个 it 独立 pinia 实例，避免 store 状态跨用例污染
    setActivePinia(createPinia())
  })

  it('loads favorites', async () => {
    const store = useFavoritesStore()
    await store.loadFavorites()
    expect(store.favorites.length).toBe(1)
    expect(store.favorites[0].alias).toBe('My App')
  })

  it('adds a favorite', async () => {
    const store = useFavoritesStore()
    const result = await store.addFavorite('C:\\new\\path', '', '默认')
    expect(result).toBe('')
  })

  it('searches favorites by alias and path', () => {
    const store = useFavoritesStore()
    store.favorites = [
      { path: 'C:\\projects\\app', alias: 'My App', group: '默认', createdAt: 1000 },
      { path: 'C:\\work\\server', alias: '', group: '工作', createdAt: 2000 }
    ]

    const results = store.searchFavorites('app')
    expect(results.length).toBe(1)
    expect(results[0].path).toBe('C:\\projects\\app')
  })

  it('removes a favorite', async () => {
    const { RemoveFavorite } = await import('../../../wailsjs/go/main/App')
    const store = useFavoritesStore()
    const result = await store.removeFavorite('C:\\projects\\app')
    expect(RemoveFavorite).toHaveBeenCalledWith('C:\\projects\\app')
    expect(result).toBe('')
  })

  it('updates group', async () => {
    const { UpdateFavoriteGroup } = await import('../../../wailsjs/go/main/App')
    const store = useFavoritesStore()
    const result = await store.updateGroup('C:\\projects\\app', '工作')
    expect(UpdateFavoriteGroup).toHaveBeenCalledWith('C:\\projects\\app', '工作')
    expect(result).toBe('')
  })

  it('searchFavorites returns all when query is empty', () => {
    const store = useFavoritesStore()
    store.favorites = [
      { path: 'C:\\a', alias: '', group: '默认', createdAt: 1000 },
      { path: 'C:\\b', alias: '', group: '默认', createdAt: 2000 }
    ]

    const results = store.searchFavorites('')
    expect(results.length).toBe(2)
  })
})
