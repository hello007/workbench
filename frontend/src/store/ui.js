import { acceptHMRUpdate, defineStore } from 'pinia'

/**
 * UI 临时态 store
 *
 * 收敛原 Home.vue 本地 ref：activePanel + 终端3(terminalVisible/Height/Dir)
 * + 弹窗 visible×6(settingsVisible/updateDialogVisible/commandPaletteVisible/repoFilterVisible
 *   + contentSearchInit + repoFilterInitialDirId) + appVersion。
 * 批次4 实现：纯 UI 态剥离 Home.vue，减负。
 */
export const useUiStore = defineStore('ui', () => {
  // 批次4 填充：activePanel + 终端3 + 弹窗 visible×6 + appVersion
  return {}
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useUiStore, import.meta.hot))
}
