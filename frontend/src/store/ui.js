import { ref } from 'vue'
import { acceptHMRUpdate, defineStore } from 'pinia'

/**
 * UI 临时态 store
 *
 * 收敛原 Home.vue 本地 ref：activePanel + 终端3(terminalVisible/Height/Dir)
 * + 弹窗 visible×6(settingsVisible/updateDialogVisible/commandPaletteVisible/repoFilterVisible
 *   + contentSearchInit + repoFilterInitialDirId) + appVersion。
 * 批次4 实现：纯 UI 态剥离 Home.vue，子组件直读直写 store，消除 visible/activePanel prop 透传。
 * 数据态（directories/selectedDirectoryId/selectedNode 等）仍留 Home.vue，批次5/6 处理。
 */
export const useUiStore = defineStore('ui', () => {
  // 活动栏当前面板：'directory' | 'ai' | 'toolbox'
  const activePanel = ref('directory')

  // 终端3：可见性 / 高度 / 跟随目录
  const terminalVisible = ref(false)
  const terminalHeight = ref(200)
  const terminalDir = ref('')

  // 弹窗 visible×6：设置 / 更新提示 / 命令面板 / 仓库筛选 + 内容搜索初始串 + 仓库筛选初始目录 id
  const settingsVisible = ref(false)
  const updateDialogVisible = ref(false)
  const updateInfo = ref({})
  const commandPaletteVisible = ref(false)
  const contentSearchInit = ref('')
  const repoFilterVisible = ref(false)
  // 仓库筛选器打开时初始锁定的工作目录 id（由 DirectoryTree 右键"仓库筛选器"触发，
  // 优先于 currentDirId）。每次打开后由 RepoFilterDialog 内 watch 消费，无需在此重置。
  const repoFilterInitialDirId = ref('')

  // 应用版本号（DirectoryTree :version 与 SettingsPanel "关于" 区均消费）
  const appVersion = ref('')

  /**
   * 切换终端可见性
   */
  function toggleTerminal() {
    terminalVisible.value = !terminalVisible.value
  }

  /**
   * 统一打开仓库筛选器入口：
   *   - 携带 dirId（DirectoryTree 右键触发）-> 锁定到右键所选项
   *   - 无 dirId（FileTreePanel 空白右键 / 工具栏按钮）-> 回退当前选中目录（由 RepoFilterDialog 取 currentDirId）
   * 关键：无 dirId 入口必须显式重置 initialDirId 为空，避免残留上次右键锁定的目录。
   */
  function openRepoFilter(dirId = '') {
    repoFilterInitialDirId.value = dirId || ''
    repoFilterVisible.value = true
  }

  return {
    activePanel,
    terminalVisible,
    terminalHeight,
    terminalDir,
    settingsVisible,
    updateDialogVisible,
    updateInfo,
    commandPaletteVisible,
    contentSearchInit,
    repoFilterVisible,
    repoFilterInitialDirId,
    appVersion,
    toggleTerminal,
    openRepoFilter
  }
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useUiStore, import.meta.hot))
}
