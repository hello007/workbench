import { acceptHMRUpdate, defineStore } from 'pinia'

/**
 * 工作区共享态 store
 *
 * 收敛原 Home.vue 本地 ref：selectedNode / latestCommit / clipboard(reactive) / lastInteractedTree。
 * 批次6 实现：三栏（DirectoryTree/FileTreePanel/ContentPanel）共享态上提。
 * 命令式跨组件调用（refreshNode/previewFile/locateNode 等）不走 store，仍走 template ref + defineExpose。
 */
export const useWorkspaceStore = defineStore('workspace', () => {
  // 批次6 填充：selectedNode ref + latestCommit ref + clipboard reactive + lastInteractedTree ref
  return {}
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useWorkspaceStore, import.meta.hot))
}
