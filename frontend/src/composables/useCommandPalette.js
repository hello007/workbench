import { ref, computed } from 'vue'
import { SearchFiles, ContentSearch, ContentSearchDirs } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'

export function useCommandPalette() {
  const visible = ref(false)
  const input = ref('')
  const selectedIndex = ref(0)
  const fileResults = ref([])
  const searchLoading = ref(false)

  // 内容搜索相关状态
  const contentGroups = ref([])
  const contentSearching = ref(false)
  const contentSearchExecuted = ref(false)
  // 发起搜索时的查询参数快照：超时目录重试须用与标志组同源的参数，
  // 禁取实时 contentQuery（用户改词后旧结果组仍在，参数错位）
  const lastContentQuery = ref({ keyword: '', fileExt: '', subDir: '' })
  // 正在重试的超时目录 repoPath 列表（行内 loading 态 + 防同目录重复触发）
  const retryingRepoPaths = ref([])

  const mode = computed(() => {
    if (input.value.startsWith('::')) return 'content-global'
    if (input.value.startsWith(':')) return 'content'
    if (input.value.startsWith('#')) return 'workdir'
    if (input.value.startsWith('@')) return 'favorites'
    if (input.value.startsWith('>')) return 'command'
    return 'general'
  })

  const query = computed(() => {
    if (mode.value === 'content' || mode.value === 'content-global') {
      return input.value.replace(/^::?/, '').trim()
    }
    if (mode.value !== 'general') {
      return input.value.slice(1).trim()
    }
    return input.value.trim()
  })

  // 解析内容搜索查询参数
  const contentQuery = computed(() => {
    const raw = query.value
    if (!raw) return { keyword: '', fileExt: '', subDir: '' }

    let remaining = raw
    let fileExt = ''
    let subDir = ''

    // 提取文件类型（以 . 开头的第一个词）
    const extRegex = /^\.(\w+)\s+/
    const extMatch = remaining.match(extRegex)
    if (extMatch) {
      fileExt = '.' + extMatch[1]
      remaining = remaining.slice(extMatch[0].length)
    }

    // 提取子目录路径（以 / 或 \ 结尾的部分）
    const pathRegex = /^(.+?)[/\\]\s+/
    const pathMatch = remaining.match(pathRegex)
    if (pathMatch) {
      subDir = pathMatch[1].replace(/[/\\]$/, '')
      remaining = remaining.slice(pathMatch[0].length)
    }

    return { keyword: remaining.trim(), fileExt, subDir }
  })

  function open() {
    visible.value = true
    input.value = ''
    selectedIndex.value = 0
    fileResults.value = []
    contentGroups.value = []
    contentSearchExecuted.value = false
    retryingRepoPaths.value = []
  }

  function close() {
    visible.value = false
    input.value = ''
    fileResults.value = []
    contentGroups.value = []
    contentSearchExecuted.value = false
    retryingRepoPaths.value = []
  }

  function openWithContentSearch(subDir) {
    visible.value = true
    input.value = ':' + subDir + '/ '
    selectedIndex.value = 0
    fileResults.value = []
    contentGroups.value = []
    contentSearchExecuted.value = false
    retryingRepoPaths.value = []
  }

  async function searchFiles(rootDir) {
    if (!query.value || mode.value !== 'general') {
      fileResults.value = []
      return
    }
    searchLoading.value = true
    try {
      fileResults.value = await SearchFiles(rootDir, query.value, 20)
    } catch (e) {
      fileResults.value = []
    } finally {
      searchLoading.value = false
    }
  }

  async function executeContentSearch() {
    const { keyword, fileExt, subDir } = contentQuery.value
    if (!keyword) return

    const isGlobal = mode.value === 'content-global'
    contentSearching.value = true
    contentGroups.value = []
    contentSearchExecuted.value = false
    // 改词重搜清空旧重试态：旧 retry pending 期间同目录又超时会被 isGroupRetrying 误拒
    retryingRepoPaths.value = []
    lastContentQuery.value = { keyword, fileExt, subDir }

    try {
      const groups = await ContentSearch(keyword, fileExt, subDir, isGlobal)
      contentGroups.value = groups || []
    } catch (e) {
      contentGroups.value = []
      // E_SEARCH_TIMEOUT 等 warning 级错误弹引导提示，真失败弹 error（此前 catch 静默吞错）
      handleError('内容搜索失败: ', e)
    } finally {
      contentSearching.value = false
      contentSearchExecuted.value = true
    }
  }

  // 重试单个超时目录：成功移除标志组并原位插入返回组（零命中仅移除，空态兜底），
  // 再超时（单目录零正常组命中 mergeTimeoutGroups 报错路径）保留原标志组仅弹提示
  async function retryTimedOutGroup(group) {
    if (!group?.timedOut || retryingRepoPaths.value.includes(group.repoPath)) return
    const { keyword, fileExt, subDir } = lastContentQuery.value
    if (!keyword) return

    retryingRepoPaths.value = [...retryingRepoPaths.value, group.repoPath]
    try {
      const groups = await ContentSearchDirs(keyword, fileExt, subDir, [group.repoPath], [group.repoName])
      const idx = contentGroups.value.findIndex(g => g.repoPath === group.repoPath)
      if (idx >= 0) {
        const next = contentGroups.value.slice()
        next.splice(idx, 1, ...(groups || []))
        contentGroups.value = next
        // 标志组增删后 selectedIndex 数值不变但指向内容已变，复位避免键盘高亮/Enter 错位
        resetSelection()
      }
    } catch (e) {
      // 再超时等失败：标志组即「此目录超时」的现状表示，保留不动
      handleError('内容搜索失败: ', e)
    } finally {
      retryingRepoPaths.value = retryingRepoPaths.value.filter(p => p !== group.repoPath)
    }
  }

  function isGroupRetrying(group) {
    return !!group && retryingRepoPaths.value.includes(group.repoPath)
  }

  function moveSelection(delta) {
    const maxIndex = fileResults.value.length - 1
    selectedIndex.value = Math.max(0, Math.min(maxIndex, selectedIndex.value + delta))
  }

  function resetSelection() {
    selectedIndex.value = 0
  }

  return {
    visible,
    input,
    mode,
    query,
    selectedIndex,
    fileResults,
    searchLoading,
    contentQuery,
    contentGroups,
    contentSearching,
    contentSearchExecuted,
    lastContentQuery,
    retryingRepoPaths,
    open,
    close,
    openWithContentSearch,
    searchFiles,
    executeContentSearch,
    retryTimedOutGroup,
    isGroupRetrying,
    moveSelection,
    resetSelection
  }
}
