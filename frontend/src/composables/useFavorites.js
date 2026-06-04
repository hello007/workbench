import { ref } from 'vue'
import { GetFavorites, AddFavorite, RemoveFavorite, UpdateFavoriteAlias, UpdateFavoriteGroup } from '../../wailsjs/go/main/App'

export function useFavorites() {
  const favorites = ref([])

  async function loadFavorites() {
    favorites.value = (await GetFavorites()) || []
  }

  async function addFavorite(path, alias, group) {
    const err = await AddFavorite(path, alias, group || '默认')
    if (!err) {
      await loadFavorites()
    }
    return err
  }

  async function removeFavorite(path) {
    const err = await RemoveFavorite(path)
    if (!err) {
      await loadFavorites()
    }
    return err
  }

  async function updateAlias(path, alias) {
    return await UpdateFavoriteAlias(path, alias)
  }

  async function updateGroup(path, group) {
    return await UpdateFavoriteGroup(path, group)
  }

  function searchFavorites(query) {
    if (!query) return favorites.value
    const q = query.toLowerCase()
    return favorites.value.filter(f => {
      const name = (f.alias || f.path).toLowerCase()
      return name.includes(q) || f.path.toLowerCase().includes(q)
    })
  }

  return { favorites, loadFavorites, addFavorite, removeFavorite, updateAlias, updateGroup, searchFavorites }
}
