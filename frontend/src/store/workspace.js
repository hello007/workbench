import { ref, reactive } from 'vue'
import { acceptHMRUpdate, defineStore } from 'pinia'

/**
 * 工作区共享态 store
 *
 * 收敛原 Home.vue 本地 ref：selectedNode / latestCommit / clipboard(reactive) / lastInteractedTree。
 * 批次6 实现：三栏（DirectoryTree/FileTreePanel/ContentPanel）共享态上提，子组件直读 store 消除 prop 透传。
 * 命令式跨组件调用（refreshNode/previewFile/locateNode 等）不走 store，仍走 template ref + defineExpose。
 * selectedNode/latestCommit 由 Home.vue onDirectorySelect/onNodeSelect 写入；ContentPanel 直读 selectedNode，
 * 并在 CommitHistory emit latest-commit 时经 onLatestCommit 直写 latestCommit（不再经 Home emit 中转）。
 * clipboard 仅 Home 内部 handleCopy/handleCut/handlePaste 消费，子组件 clipboard prop 已删除（零消费）。
 */
export const useWorkspaceStore = defineStore('workspace', () => {
  // 当前选中的文件树节点（file / directory / gitRepo directory 节点）
  const selectedNode = ref(null)
  // 最新提交（CommitHistory tab emit，GitInfo 面板消费）
  const latestCommit = ref(null)
  // 最近交互的树面板，用于 F2/Del 快捷键分派（'directory' | 'file'）
  const lastInteractedTree = ref('directory')
  // 剪贴板：mode 'copy'|'cut'|null + 源路径/名/类型
  const clipboard = reactive({
    mode: null,
    sourcePath: '',
    sourceName: '',
    sourceType: ''
  })

  /**
   * 清空剪贴板（切换工作目录 / 剪切粘贴完成后调用）
   */
  function clearClipboard() {
    clipboard.mode = null
    clipboard.sourcePath = ''
    clipboard.sourceName = ''
    clipboard.sourceType = ''
  }

  return { selectedNode, latestCommit, lastInteractedTree, clipboard, clearClipboard }
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useWorkspaceStore, import.meta.hot))
}
