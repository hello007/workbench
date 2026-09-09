import { ref, computed } from 'vue'
import { acceptHMRUpdate, defineStore } from 'pinia'
import { GetDirectories, RefreshDirectoriesGitFlag } from '../../wailsjs/go/main/App'
import { debug } from '../utils/debug'

/**
 * 工作目录 store
 *
 * 收敛原 Home.vue 本地 ref：directories / selectedDirectoryId / currentDirPath computed。
 * 批次5 实现：迁入纯 directory 域 action（loadDirectories / refreshGitFlags）。
 * 跨域 action（onDirectorySelect / onRepoLocate / onPaletteSelect* / onAddWorkDir）涉 ref 链
 * （fileTreePanelRef / contentPanelRef）与 workspace 域依赖（selectedNode / latestCommit / clipboard），
 * 仍留 Home.vue 编排，内部读写改为 directoryStore.xxx。
 */
export const useDirectoryStore = defineStore('directory', () => {
  const directories = ref([])
  const selectedDirectoryId = ref('')

  // 当前选中工作目录路径：由 directories + selectedDirectoryId 派生，消费方直接读 string
  const currentDirPath = computed(() => {
    const dir = directories.value.find(d => d.id === selectedDirectoryId.value)
    return dir ? dir.path : ''
  })

  /**
   * 加载目录列表并自动选中默认目录（或首个）
   * GetDirectories 启动时直接返回 directories.json 持久化值（秒回）。
   */
  async function loadDirectories() {
    try {
      const dirs = await GetDirectories()
      directories.value = dirs || []

      // 自动选择默认目录
      const defaultDir = dirs.find(d => d.isDefault)
      if (defaultDir) {
        selectedDirectoryId.value = defaultDir.id
      } else if (dirs.length > 0) {
        selectedDirectoryId.value = dirs[0].id
      }
    } catch (error) {
      debug.log('加载目录失败:', error)
    }
  }

  /**
   * 异步刷新工作目录的 git 标识。
   * GetDirectories 启动时返回持久化的 IsGitRepo（秒回），这里再调一次后端刷新覆盖
   * "目录后来才纳管为 git"等变化。仅替换 directories 列表（左栏 git 标记随 dir.isGitRepo
   * 更新自动刷新），不动 selectedDirectoryId，避免打断用户已选中的目录。
   */
  async function refreshGitFlags() {
    try {
      const fresh = await RefreshDirectoriesGitFlag()
      if (fresh && fresh.length) {
        directories.value = fresh
      }
    } catch (error) {
      // 刷新失败不影响主流程，缓存值仍可用
      debug.log('刷新工作目录 git 标识失败:', error)
    }
  }

  return { directories, selectedDirectoryId, currentDirPath, loadDirectories, refreshGitFlags }
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useDirectoryStore, import.meta.hot))
}
