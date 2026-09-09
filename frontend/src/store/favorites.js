import { acceptHMRUpdate, defineStore } from 'pinia'

/**
 * 收藏夹 store
 *
 * 收敛原 composables/useFavorites.js 的模块级单例 ref（favorites）。
 * 批次2 实现：迁入 GetFavorites/AddFavorite/RemoveFavorite 等调用与 loadFavorites 逻辑。
 */
export const useFavoritesStore = defineStore('favorites', () => {
  // 批次2 填充：favorites ref + loadFavorites/addFavorite/removeFavorite/updateAlias/updateGroup/searchFavorites
  return {}
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useFavoritesStore, import.meta.hot))
}
