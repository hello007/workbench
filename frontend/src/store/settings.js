import { acceptHMRUpdate, defineStore } from 'pinia'

/**
 * 设置 store
 *
 * 收敛原 composables/useShortcuts.js 的模块级单例 ref：
 * shortcutCommandPalette / shortcutToggleTerminal / shortcutRename / shortcutDelete。
 * 批次3 实现：迁入 loadShortcuts / saveShortcuts / checkConflict / matchShortcut / parseShortcut 等逻辑。
 */
export const useSettingsStore = defineStore('settings', () => {
  // 批次3 填充：4 个 shortcut ref + load/save/checkConflict/matchShortcut
  return {}
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useSettingsStore, import.meta.hot))
}
