import { ref } from 'vue'
import { acceptHMRUpdate, defineStore } from 'pinia'
import { GetFavorites, AddFavorite, RemoveFavorite, UpdateFavoriteAlias, UpdateFavoriteGroup } from '../../wailsjs/go/main/App'

/**
 * 收藏夹 store
 *
 * 收敛原 composables/useFavorites.js 的模块级单例 ref（favorites）与全部收藏夹操作。
 * 维护 favorites 列表，封装加载/增删/改别名/改分组/搜索逻辑，调用后端 wailsjs 绑定。
 */
export const useFavoritesStore = defineStore('favorites', () => {
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
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useFavoritesStore, import.meta.hot))
}
