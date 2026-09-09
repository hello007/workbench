import { acceptHMRUpdate, defineStore } from 'pinia'

/**
 * 工作目录 store
 *
 * 收敛原 Home.vue 本地 ref：directories / selectedDirectoryId / currentDirPath computed。
 * 批次5 实现：迁入 loadDirectories / refreshGitFlags / onDirectorySelect / onRepoLocate 逻辑，
 * 内部调用 useTreeState 工具保存/恢复树状态。
 */
export const useDirectoryStore = defineStore('directory', () => {
  // 批次5 填充：directories ref + selectedDirectoryId ref + currentDirPath computed + load/refresh/select/locate action
  return {}
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useDirectoryStore, import.meta.hot))
}
