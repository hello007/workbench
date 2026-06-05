import { ref, computed } from 'vue'
import { SearchFiles, ContentSearch } from '../../wailsjs/go/main/App'

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
  }

  function close() {
    visible.value = false
    input.value = ''
    fileResults.value = []
    contentGroups.value = []
    contentSearchExecuted.value = false
  }

  function openWithContentSearch(subDir) {
    visible.value = true
    input.value = ':' + subDir + '/ '
    selectedIndex.value = 0
    fileResults.value = []
    contentGroups.value = []
    contentSearchExecuted.value = false
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

    try {
      const groups = await ContentSearch(keyword, fileExt, subDir, isGlobal)
      contentGroups.value = groups || []
    } catch (e) {
      contentGroups.value = []
    } finally {
      contentSearching.value = false
      contentSearchExecuted.value = true
    }
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
    open,
    close,
    openWithContentSearch,
    searchFiles,
    executeContentSearch,
    moveSelection,
    resetSelection
  }
}
