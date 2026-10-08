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
  // 正在重试的超时目录 repoPath 列表（行内 loading 态 + 防同目录重复触发；
  // 批量重试启动时预占全部目标目录，排队中目录 warning 行也显「重试中...」且防手动重复）
  const retryingRepoPaths = ref([])
  // 搜索版本号：executeContentSearch/open/close 自增。批量重试多请求在途，
  // 改词重搜或关闭面板后旧请求写回会污染新结果，闭包捕获起始版本号，写回前比对不匹配则丢弃
  const searchVersion = ref(0)
  // 批量重试态：banner 进度展示「重试中 (done/total)...」与按钮禁用
  const batchRetrying = ref(false)
  const batchRetryTotal = ref(0)
  const batchRetryDone = ref(0)
  // 首轮批量重试完成后置 true：banner 文案区分初始「检测到 N 个」与完成态有剩余「仍有 N 个」+ 按钮「重试剩余 N 个」
  const hasBatchRetried = ref(false)

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
    resetBatchRetry()
    searchVersion.value++
  }

  function close() {
    visible.value = false
    input.value = ''
    fileResults.value = []
    contentGroups.value = []
    contentSearchExecuted.value = false
    retryingRepoPaths.value = []
    resetBatchRetry()
    searchVersion.value++
  }

  function openWithContentSearch(subDir) {
    visible.value = true
    input.value = ':' + subDir + '/ '
    selectedIndex.value = 0
    fileResults.value = []
    contentGroups.value = []
    contentSearchExecuted.value = false
    retryingRepoPaths.value = []
    resetBatchRetry()
    searchVersion.value++
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
    // 改词重搜清空旧重试态：旧 retry pending 期间同目录又超时会被 isGroupRetrying 误拒；
    // 自增版本号让在途批量重试请求写回时版本不匹配丢弃，防污染新结果
    retryingRepoPaths.value = []
    resetBatchRetry()
    searchVersion.value++
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

  // 单目录重试核心逻辑：调 ContentSearchDirs + 原位 splice 替换标志组（零命中仅移除，空态兜底），
  // 再超时（单目录零正常组命中 mergeTimeoutGroups 报错路径）保留原标志组仅弹提示。
  // version 守卫：批量重试多请求在途，改词重搜/关闭面板后版本号已变，写回前比对丢弃防污染
  async function retryOneDir(group, version) {
    const { keyword, fileExt, subDir } = lastContentQuery.value
    if (!keyword) return
    try {
      const groups = await ContentSearchDirs(keyword, fileExt, subDir, [group.repoPath], [group.repoName])
      if (version !== searchVersion.value) return  // 版本不匹配：用户已改词重搜/关闭，丢弃旧结果
      const idx = contentGroups.value.findIndex(g => g.repoPath === group.repoPath)
      if (idx >= 0) {
        const next = contentGroups.value.slice()
        next.splice(idx, 1, ...(groups || []))
        contentGroups.value = next
        // 标志组增删后 selectedIndex 数值不变但指向内容已变，复位避免键盘高亮/Enter 错位
        resetSelection()
      }
    } catch (e) {
      if (version !== searchVersion.value) return
      // 再超时等失败：标志组即「此目录超时」的现状表示，保留不动
      handleError('内容搜索失败: ', e)
    }
  }

  // 重试单个超时目录（warning 行点击入口）：预占 repoPath 防重复 + 调核心逻辑 + 释放。
  // version 快照 + finally 守卫与批量路径对齐：版本失效（改词重搜/关闭面板）后不清除预占，
  // 防旧 finally 移除新重试的预占 repoPath 致 isGroupRetrying 短暂 false 容许重复触发
  async function retryTimedOutGroup(group) {
    if (!group?.timedOut || retryingRepoPaths.value.includes(group.repoPath)) return
    const version = searchVersion.value
    retryingRepoPaths.value = [...retryingRepoPaths.value, group.repoPath]
    try {
      await retryOneDir(group, version)
    } finally {
      if (version === searchVersion.value) {
        retryingRepoPaths.value = retryingRepoPaths.value.filter(p => p !== group.repoPath)
      }
    }
  }

  // 批量重试所有超时目录（banner 入口）：预占全部目标目录（排队中 warning 行也显态 + 防手动重复）
  // + 信号量 3 并发（对齐 AI 功能全局信号量，rg 进程数可控 IO 抢占温和）+ 版本号守卫
  async function retryAllTimedOutGroups() {
    const timedOutGroups = contentGroups.value.filter(g => g.timedOut)
    if (timedOutGroups.length < 2) return  // banner 仅 ≥2 显示，防御单目录误调
    if (retryingRepoPaths.value.length > 0) return  // 批量/单目录进行中防重复

    const version = ++searchVersion.value
    retryingRepoPaths.value = [...retryingRepoPaths.value, ...timedOutGroups.map(g => g.repoPath)]
    batchRetrying.value = true
    batchRetryTotal.value = timedOutGroups.length
    batchRetryDone.value = 0

    const CONCURRENCY = 3
    let cursor = 0
    const worker = async () => {
      while (cursor < timedOutGroups.length) {
        if (version !== searchVersion.value) return  // 版本失效（改词重搜/关闭）：终止 worker，不再发请求
        const group = timedOutGroups[cursor++]
        try {
          await retryOneDir(group, version)
        } finally {
          if (version === searchVersion.value) {
            retryingRepoPaths.value = retryingRepoPaths.value.filter(p => p !== group.repoPath)
            batchRetryDone.value++
          }
        }
      }
    }
    const workers = Array.from({ length: Math.min(CONCURRENCY, timedOutGroups.length) }, () => worker())
    await Promise.all(workers)
    if (version === searchVersion.value) {
      batchRetrying.value = false
      // 首轮完成：仍有超时目录置 true 驱动 banner 切「仍有 N 个」+「重试剩余 N 个」文案；全成功置 false（banner 消失）
      hasBatchRetried.value = contentGroups.value.some(g => g.timedOut)
    }
  }

  function resetBatchRetry() {
    batchRetrying.value = false
    batchRetryTotal.value = 0
    batchRetryDone.value = 0
    hasBatchRetried.value = false
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
    batchRetrying,
    batchRetryTotal,
    batchRetryDone,
    hasBatchRetried,
    open,
    close,
    openWithContentSearch,
    searchFiles,
    executeContentSearch,
    retryTimedOutGroup,
    retryAllTimedOutGroups,
    isGroupRetrying,
    moveSelection,
    resetSelection
  }
}
