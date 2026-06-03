import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useFavorites } from '../useFavorites'

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetFavorites: vi.fn(() => Promise.resolve([
    { path: 'C:\\projects\\app', alias: 'My App', group: '默认', createdAt: 1000 }
  ])),
  AddFavorite: vi.fn(() => Promise.resolve('')),
  RemoveFavorite: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteAlias: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteGroup: vi.fn(() => Promise.resolve(''))
}))

describe('useFavorites', () => {
  it('loads favorites', async () => {
    const { favorites, loadFavorites } = useFavorites()
    await loadFavorites()
    expect(favorites.value.length).toBe(1)
    expect(favorites.value[0].alias).toBe('My App')
  })

  it('adds a favorite', async () => {
    const { addFavorite } = useFavorites()
    const result = await addFavorite('C:\\new\\path', '', '默认')
    expect(result).toBe('')
  })

  it('searches favorites by alias and path', () => {
    const { favorites, searchFavorites } = useFavorites()
    favorites.value = [
      { path: 'C:\\projects\\app', alias: 'My App', group: '默认', createdAt: 1000 },
      { path: 'C:\\work\\server', alias: '', group: '工作', createdAt: 2000 }
    ]

    const results = searchFavorites('app')
    expect(results.length).toBe(1)
    expect(results[0].path).toBe('C:\\projects\\app')
  })

  it('removes a favorite', async () => {
    const { RemoveFavorite } = await import('../../../wailsjs/go/main/App')
    const { removeFavorite } = useFavorites()
    const result = await removeFavorite('C:\\projects\\app')
    expect(RemoveFavorite).toHaveBeenCalledWith('C:\\projects\\app')
    expect(result).toBe('')
  })

  it('updates group', async () => {
    const { UpdateFavoriteGroup } = await import('../../../wailsjs/go/main/App')
    const { updateGroup } = useFavorites()
    const result = await updateGroup('C:\\projects\\app', '工作')
    expect(UpdateFavoriteGroup).toHaveBeenCalledWith('C:\\projects\\app', '工作')
    expect(result).toBe('')
  })

  it('searchFavorites returns all when query is empty', () => {
    const { favorites, searchFavorites } = useFavorites()
    favorites.value = [
      { path: 'C:\\a', alias: '', group: '默认', createdAt: 1000 },
      { path: 'C:\\b', alias: '', group: '默认', createdAt: 2000 }
    ]

    const results = searchFavorites('')
    expect(results.length).toBe(2)
  })
})
