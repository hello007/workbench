import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useCommandPalette } from '../useCommandPalette'

// wailsjs 在 frontend/wailsjs，从 src/composables/__tests__/ 解析为 ../../../wailsjs
vi.mock('../../../wailsjs/go/main/App', () => ({
  SearchFiles: vi.fn(() => Promise.resolve([{ name: 'a.go', path: 'src/a.go', type: 'file' }])),
  ContentSearch: vi.fn(() => Promise.resolve([{ repo: 'r', results: [] }])),
  ContentSearchDirs: vi.fn(() => Promise.resolve([]))
}))

// element-plus 仅 mock ElMessage，断言 executeContentSearch catch 的 handleError 分流
vi.mock('element-plus', () => ({
  ElMessage: Object.assign(vi.fn(), { warning: vi.fn(), error: vi.fn(), success: vi.fn() })
}))

import { SearchFiles, ContentSearch, ContentSearchDirs } from '../../../wailsjs/go/main/App'
import { ElMessage } from 'element-plus'

describe('useCommandPalette', () => {
  let palette

  beforeEach(() => {
    vi.clearAllMocks()
    SearchFiles.mockResolvedValue([{ name: 'a.go', path: 'src/a.go', type: 'file' }])
    ContentSearch.mockResolvedValue([{ repo: 'r', results: [] }])
    palette = useCommandPalette()
  })

  describe('mode 计算', () => {
    it(':: 前缀为 content-global', () => {
      palette.input.value = '::kw'
      expect(palette.mode.value).toBe('content-global')
    })
    it(': 前缀为 content', () => {
      palette.input.value = ':kw'
      expect(palette.mode.value).toBe('content')
    })
    it('# 前缀为 workdir', () => {
      palette.input.value = '#'
      expect(palette.mode.value).toBe('workdir')
    })
    it('@ 前缀为 favorites', () => {
      palette.input.value = '@'
      expect(palette.mode.value).toBe('favorites')
    })
    it('> 前缀为 command', () => {
      palette.input.value = '>cmd'
      expect(palette.mode.value).toBe('command')
    })
    it('无前缀为 general', () => {
      palette.input.value = 'kw'
      expect(palette.mode.value).toBe('general')
    })
  })

  describe('query 计算', () => {
    it('general 返回 trim 输入', () => {
      palette.input.value = '  kw  '
      expect(palette.query.value).toBe('kw')
    })
    it('workdir 去掉 # 前缀', () => {
      palette.input.value = '#proj'
      expect(palette.query.value).toBe('proj')
    })
    it('content 去掉 : 前缀', () => {
      palette.input.value = ':kw'
      expect(palette.query.value).toBe('kw')
    })
    it('content-global 去掉 :: 前缀', () => {
      palette.input.value = '::kw'
      expect(palette.query.value).toBe('kw')
    })
  })

  describe('contentQuery 解析', () => {
    it('空查询返回空字段', () => {
      palette.input.value = ':'
      expect(palette.contentQuery.value).toEqual({ keyword: '', fileExt: '', subDir: '' })
    })
    it('提取文件扩展名', () => {
      palette.input.value = ':.go keyword'
      expect(palette.contentQuery.value.fileExt).toBe('.go')
      expect(palette.contentQuery.value.keyword).toBe('keyword')
    })
    it('提取子目录', () => {
      palette.input.value = ':src/ keyword'
      expect(palette.contentQuery.value.subDir).toBe('src')
      expect(palette.contentQuery.value.keyword).toBe('keyword')
    })
    it('仅 keyword', () => {
      palette.input.value = ':keyword'
      expect(palette.contentQuery.value.keyword).toBe('keyword')
    })
  })

  describe('open/close', () => {
    it('open 重置状态并显示', () => {
      palette.input.value = 'old'
      palette.fileResults.value = [{ name: 'x' }]
      palette.open()
      expect(palette.visible.value).toBe(true)
      expect(palette.input.value).toBe('')
      expect(palette.fileResults.value).toEqual([])
      expect(palette.contentSearchExecuted.value).toBe(false)
    })
    it('close 重置状态并隐藏', () => {
      palette.visible.value = true
      palette.input.value = 'old'
      palette.close()
      expect(palette.visible.value).toBe(false)
      expect(palette.input.value).toBe('')
      expect(palette.fileResults.value).toEqual([])
    })
    it('openWithContentSearch 设置 : 前缀', () => {
      palette.openWithContentSearch('src')
      expect(palette.visible.value).toBe(true)
      expect(palette.input.value).toBe(':src/ ')
    })
  })

  describe('searchFiles', () => {
    it('general 模式查询返回结果', async () => {
      palette.input.value = 'kw'
      await palette.searchFiles('/root')
      expect(palette.searchLoading.value).toBe(false)
      expect(palette.fileResults.value.length).toBe(1)
    })
    it('非 general 模式不查询', async () => {
      palette.input.value = '#prefix'
      await palette.searchFiles('/root')
      expect(palette.fileResults.value).toEqual([])
    })
    it('空 query 不查询', async () => {
      palette.input.value = ''
      await palette.searchFiles('/root')
      expect(palette.fileResults.value).toEqual([])
    })
    it('查询异常时置空结果', async () => {
      SearchFiles.mockRejectedValueOnce(new Error('fail'))
      palette.input.value = 'kw'
      await palette.searchFiles('/root')
      expect(palette.fileResults.value).toEqual([])
      expect(palette.searchLoading.value).toBe(false)
    })
  })

  describe('executeContentSearch', () => {
    it('空 keyword 不执行', async () => {
      palette.input.value = ':'
      await palette.executeContentSearch()
      expect(palette.contentSearching.value).toBe(false)
      expect(palette.contentSearchExecuted.value).toBe(false)
    })
    it('有 keyword 执行并返回分组', async () => {
      palette.input.value = ':kw'
      await palette.executeContentSearch()
      expect(palette.contentSearchExecuted.value).toBe(true)
      expect(palette.contentGroups.value.length).toBe(1)
    })
    it('异常时 contentGroups 置空但 executed 为 true', async () => {
      ContentSearch.mockRejectedValueOnce(new Error('fail'))
      palette.input.value = ':kw'
      await palette.executeContentSearch()
      expect(palette.contentGroups.value).toEqual([])
      expect(palette.contentSearchExecuted.value).toBe(true)
    })
    it('真失败（非 warning 码）弹 error 提示', async () => {
      ContentSearch.mockRejectedValueOnce(new Error('fail'))
      palette.input.value = ':kw'
      await palette.executeContentSearch()
      expect(ElMessage.error).toHaveBeenCalledWith('内容搜索失败: fail')
      expect(ElMessage.warning).not.toHaveBeenCalled()
    })
    it('E_SEARCH_TIMEOUT 命中 WARNING_CODES 弹 warning 引导', async () => {
      ContentSearch.mockRejectedValueOnce({
        code: 'E_SEARCH_TIMEOUT',
        message: '内容搜索超时已终止，可缩小范围后重试'
      })
      palette.input.value = ':kw'
      await palette.executeContentSearch()
      expect(ElMessage.warning).toHaveBeenCalledWith('内容搜索超时已终止，可缩小范围后重试')
      expect(ElMessage.error).not.toHaveBeenCalled()
    })
  })

  describe('retryTimedOutGroup 超时目录单目录重试', () => {
    const timeoutGroup = { repoName: 'repo-timeout', repoPath: 'C:/r2', items: [], timedOut: true }

    beforeEach(async () => {
      // 先执行一次完整搜索建立 lastContentQuery 快照与标志组现状
      palette.input.value = '::kw'
      await palette.executeContentSearch()
      palette.contentGroups.value = [
        { repoName: 'repo-ok', repoPath: 'C:/r1', items: [{ filePath: 'a.txt', lineNum: 1, lineText: 'kw' }] },
        { ...timeoutGroup }
      ]
      ElMessage.warning.mockClear()
      ElMessage.error.mockClear()
    })

    it('重试成功移除标志组并原位插入结果组', async () => {
      ContentSearchDirs.mockResolvedValueOnce([
        { repoName: 'repo-timeout', repoPath: 'C:/r2', items: [{ filePath: 'b.txt', lineNum: 2, lineText: 'kw' }] }
      ])
      await palette.retryTimedOutGroup(timeoutGroup)
      expect(ContentSearchDirs).toHaveBeenCalledWith('kw', '', '', ['C:/r2'], ['repo-timeout'])
      expect(palette.contentGroups.value.length).toBe(2)
      expect(palette.contentGroups.value.some(g => g.timedOut)).toBe(false)
      expect(palette.contentGroups.value[1].items[0].filePath).toBe('b.txt')
      expect(palette.isGroupRetrying(timeoutGroup)).toBe(false)
    })

    it('重试零命中仅移除标志组', async () => {
      ContentSearchDirs.mockResolvedValueOnce([])
      await palette.retryTimedOutGroup(timeoutGroup)
      expect(palette.contentGroups.value.length).toBe(1)
      expect(palette.contentGroups.value[0].repoPath).toBe('C:/r1')
    })

    it('重试再超时保留标志组并弹 warning 引导', async () => {
      ContentSearchDirs.mockRejectedValueOnce({
        code: 'E_SEARCH_TIMEOUT',
        message: '内容搜索超时已终止，可缩小范围后重试'
      })
      await palette.retryTimedOutGroup(timeoutGroup)
      expect(palette.contentGroups.value.length).toBe(2)
      expect(palette.contentGroups.value.some(g => g.timedOut)).toBe(true)
      expect(ElMessage.warning).toHaveBeenCalledWith('内容搜索超时已终止，可缩小范围后重试')
      expect(palette.isGroupRetrying(timeoutGroup)).toBe(false)
    })

    it('重试中同目录重复触发被拒，loading 态期间置位', async () => {
      let resolveRetry
      ContentSearchDirs.mockReturnValueOnce(new Promise(r => { resolveRetry = r }))
      const pending = palette.retryTimedOutGroup(timeoutGroup)
      expect(palette.isGroupRetrying(timeoutGroup)).toBe(true)
      // 重试中再次点击（组件层经 isGroupRetrying 守卫，此处验证 composable 层兜底）
      await palette.retryTimedOutGroup({ ...timeoutGroup })
      expect(ContentSearchDirs).toHaveBeenCalledTimes(1)
      resolveRetry([])
      await pending
      expect(palette.isGroupRetrying(timeoutGroup)).toBe(false)
    })

    it('非 timedOut 组不触发重试', async () => {
      await palette.retryTimedOutGroup({ repoName: 'repo-ok', repoPath: 'C:/r1', items: [] })
      expect(ContentSearchDirs).not.toHaveBeenCalled()
    })

    it('重试使用发起搜索时的参数快照，不取实时 contentQuery', async () => {
      // 用户改词后旧结果组仍在（onInput 不清 contentGroups 既有行为），重试须用快照参数
      palette.input.value = '::changed'
      await palette.retryTimedOutGroup(timeoutGroup)
      expect(ContentSearchDirs).toHaveBeenCalledWith('kw', '', '', ['C:/r2'], ['repo-timeout'])
    })

    it('close 清空重试中状态', async () => {
      let resolveRetry
      ContentSearchDirs.mockReturnValueOnce(new Promise(r => { resolveRetry = r }))
      const pending = palette.retryTimedOutGroup(timeoutGroup)
      expect(palette.retryingRepoPaths.value.length).toBe(1)
      palette.close()
      expect(palette.retryingRepoPaths.value).toEqual([])
      resolveRetry([])
      await pending
    })

    it('改词重搜清空旧重试态，pending 期间同目录可重新触发重试', async () => {
      // 旧 retry pending 中用户改词重搜，retryingRepoPaths 须清空否则同目录又超时被误拒
      let resolveOldRetry
      ContentSearchDirs.mockReturnValueOnce(new Promise(r => { resolveOldRetry = r }))
      const oldPending = palette.retryTimedOutGroup(timeoutGroup)
      expect(palette.retryingRepoPaths.value.length).toBe(1)

      palette.input.value = '::kw2'
      await palette.executeContentSearch()
      expect(palette.retryingRepoPaths.value).toEqual([])

      // 旧 retry 仍未 resolve，新重试应能立即触发（不被旧态阻塞）
      ContentSearchDirs.mockResolvedValueOnce([])
      await palette.retryTimedOutGroup(timeoutGroup)
      expect(ContentSearchDirs).toHaveBeenLastCalledWith('kw2', '', '', ['C:/r2'], ['repo-timeout'])

      resolveOldRetry([])
      await oldPending
    })

    it('重试成功后 selectedIndex 复位避免键盘高亮错位', async () => {
      // 用户键盘选了第二项后点 warning 行重试，splice 替换标志组后 selectedIndex 须归 0
      palette.selectedIndex.value = 1
      ContentSearchDirs.mockResolvedValueOnce([
        { repoName: 'repo-timeout', repoPath: 'C:/r2', items: [{ filePath: 'b.txt', lineNum: 2, lineText: 'kw' }] }
      ])
      await palette.retryTimedOutGroup(timeoutGroup)
      expect(palette.selectedIndex.value).toBe(0)
    })
  })

  describe('retryAllTimedOutGroups 批量重试', () => {
    const timeoutGroups = [
      { repoName: 'repo-t1', repoPath: 'C:/t1', items: [], timedOut: true },
      { repoName: 'repo-t2', repoPath: 'C:/t2', items: [], timedOut: true },
      { repoName: 'repo-t3', repoPath: 'C:/t3', items: [], timedOut: true },
      { repoName: 'repo-t4', repoPath: 'C:/t4', items: [], timedOut: true },
      { repoName: 'repo-t5', repoPath: 'C:/t5', items: [], timedOut: true }
    ]

    beforeEach(async () => {
      palette.input.value = '::kw'
      await palette.executeContentSearch()
      palette.contentGroups.value = [...timeoutGroups]
      ElMessage.warning.mockClear()
      ElMessage.error.mockClear()
    })

    it('批量重试全部成功，标志组逐个原位替换为结果组', async () => {
      timeoutGroups.forEach((g, i) => {
        ContentSearchDirs.mockResolvedValueOnce([
          { repoName: g.repoName, repoPath: g.repoPath, items: [{ filePath: `f${i}.txt`, lineNum: i + 1, lineText: 'kw' }] }
        ])
      })
      await palette.retryAllTimedOutGroups()
      expect(palette.contentGroups.value.length).toBe(5)
      expect(palette.contentGroups.value.some(g => g.timedOut)).toBe(false)
      expect(palette.contentGroups.value[0].items[0].filePath).toBe('f0.txt')
      expect(palette.contentGroups.value[4].items[0].filePath).toBe('f4.txt')
      expect(palette.batchRetrying.value).toBe(false)
      expect(palette.batchRetryDone.value).toBe(5)
      expect(palette.retryingRepoPaths.value).toEqual([])
    })

    it('批量重试部分成功部分再超时，成功替换结果组再超时保留标志组', async () => {
      ContentSearchDirs
        .mockResolvedValueOnce([{ repoName: 'repo-t1', repoPath: 'C:/t1', items: [{ filePath: 'a.txt', lineNum: 1, lineText: 'kw' }] }])
        .mockRejectedValueOnce({ code: 'E_SEARCH_TIMEOUT', message: '内容搜索超时已终止，可缩小范围后重试' })
        .mockResolvedValueOnce([{ repoName: 'repo-t3', repoPath: 'C:/t3', items: [{ filePath: 'c.txt', lineNum: 3, lineText: 'kw' }] }])
        .mockRejectedValueOnce({ code: 'E_SEARCH_TIMEOUT', message: '内容搜索超时已终止，可缩小范围后重试' })
        .mockResolvedValueOnce([{ repoName: 'repo-t5', repoPath: 'C:/t5', items: [{ filePath: 'e.txt', lineNum: 5, lineText: 'kw' }] }])
      await palette.retryAllTimedOutGroups()
      const timedOutRemaining = palette.contentGroups.value.filter(g => g.timedOut)
      expect(timedOutRemaining.length).toBe(2)
      expect(timedOutRemaining.map(g => g.repoPath).sort()).toEqual(['C:/t2', 'C:/t4'])
      expect(palette.batchRetrying.value).toBe(false)
      expect(palette.batchRetryDone.value).toBe(5)
    })

    it('批量重试启动预先占用全部目标目录 retryingRepoPaths', async () => {
      const resolvers = []
      let callCount = 0
      ContentSearchDirs.mockImplementation(() => {
        callCount++
        if (callCount <= 3) return new Promise(r => { resolvers.push(r) })
        return Promise.resolve([])
      })
      const pending = palette.retryAllTimedOutGroups()
      // 并发 3，5 目录 → 3 in-flight + 2 排队，retryingRepoPaths 预占全部 5
      expect(palette.retryingRepoPaths.value.length).toBe(5)
      expect(palette.batchRetrying.value).toBe(true)
      expect(palette.batchRetryTotal.value).toBe(5)
      expect(ContentSearchDirs).toHaveBeenCalledTimes(3)
      // 手动点单目录重试被预占挡住
      await palette.retryTimedOutGroup(timeoutGroups[0])
      expect(ContentSearchDirs).toHaveBeenCalledTimes(3)
      // resolve 第一批，第二批立即 resolve，批量完成
      resolvers.forEach(r => r([]))
      await pending
      expect(palette.batchRetrying.value).toBe(false)
    })

    it('批量重试并发上限 3，最多 3 个同时 in-flight', async () => {
      let inFlight = 0, maxInFlight = 0
      ContentSearchDirs.mockImplementation(() => {
        inFlight++
        maxInFlight = Math.max(maxInFlight, inFlight)
        return new Promise(r => { setTimeout(() => { inFlight--; r([]) }, 0) })
      })
      await palette.retryAllTimedOutGroups()
      expect(maxInFlight).toBe(3)
      expect(ContentSearchDirs).toHaveBeenCalledTimes(5)
    })

    it('批量进行中改词重搜，旧请求写回被版本号守卫丢弃不污染新结果', async () => {
      const resolvers = []
      let callCount = 0
      ContentSearchDirs.mockImplementation(() => {
        callCount++
        if (callCount <= 3) return new Promise(r => { resolvers.push(r) })
        return Promise.resolve([])
      })
      const oldPending = palette.retryAllTimedOutGroups()
      expect(palette.batchRetrying.value).toBe(true)

      // 改词重搜触发版本号自增，batchRetrying 复位
      palette.input.value = '::kw2'
      ContentSearch.mockResolvedValueOnce([{ repoName: 'new-repo', repoPath: 'C:/new', items: [{ filePath: 'n.txt', lineNum: 1, lineText: 'kw2' }] }])
      await palette.executeContentSearch()
      expect(palette.batchRetrying.value).toBe(false)
      expect(palette.retryingRepoPaths.value).toEqual([])
      expect(palette.contentGroups.value.length).toBe(1)
      expect(palette.contentGroups.value[0].repoPath).toBe('C:/new')

      // 旧批量请求 resolve：版本不匹配，写回被丢弃，contentGroups 保持新结果
      resolvers.forEach(r => r([{ repoName: 'repo-t1', repoPath: 'C:/t1', items: [{ filePath: 'stale.txt', lineNum: 99, lineText: 'kw' }] }]))
      await oldPending
      expect(palette.contentGroups.value.length).toBe(1)
      expect(palette.contentGroups.value[0].repoPath).toBe('C:/new')
    })

    it('批量进行中关闭面板，旧请求写回被版本号守卫丢弃', async () => {
      const resolvers = []
      let callCount = 0
      ContentSearchDirs.mockImplementation(() => {
        callCount++
        if (callCount <= 3) return new Promise(r => { resolvers.push(r) })
        return Promise.resolve([])
      })
      const oldPending = palette.retryAllTimedOutGroups()
      palette.close()
      expect(palette.batchRetrying.value).toBe(false)
      resolvers.forEach(r => r([{ repoName: 'repo-t1', repoPath: 'C:/t1', items: [{ filePath: 'stale.txt', lineNum: 99, lineText: 'kw' }] }]))
      await oldPending
      expect(palette.contentGroups.value).toEqual([])
    })

    it('关闭面板自增版本号终止 worker，在途 reject 被版本守卫挡住不弹 stale 提示', async () => {
      // M1 回归守护：close 须自增 searchVersion，worker while 头部版本失效 return 不再发请求；
      // 在途 ContentSearchDirs reject 时 retryOneDir catch 版本不匹配 return，不调 handleError 弹 stale 提示
      const rejectors = []
      let callCount = 0
      ContentSearchDirs.mockImplementation(() => {
        callCount++
        if (callCount <= 3) return new Promise((_, reject) => { rejectors.push(reject) })
        return Promise.resolve([])
      })
      const oldPending = palette.retryAllTimedOutGroups()
      expect(callCount).toBe(3) // 3 并发 in-flight
      palette.close()
      // close 自增版本号：worker 终止不再发第 4+ 请求
      rejectors.forEach(rej => rej({ code: 'E_SEARCH_TIMEOUT', message: '内容搜索超时已终止，可缩小范围后重试' }))
      await oldPending
      expect(callCount).toBe(3) // worker 终止，未取第 4/5 批
      expect(ElMessage.warning).not.toHaveBeenCalled()
      expect(ElMessage.error).not.toHaveBeenCalled()
    })

    it('批量进行中重复触发被拒', async () => {
      const resolvers = []
      let callCount = 0
      ContentSearchDirs.mockImplementation(() => {
        callCount++
        if (callCount <= 3) return new Promise(r => { resolvers.push(r) })
        return Promise.resolve([])
      })
      const pending = palette.retryAllTimedOutGroups()
      expect(palette.retryingRepoPaths.value.length).toBe(5)
      await palette.retryAllTimedOutGroups()
      expect(ContentSearchDirs).toHaveBeenCalledTimes(3) // 第二次被 retryingRepoPaths 防重复挡住
      // resolve 让批量完成
      resolvers.forEach(r => r([]))
      await pending
      expect(palette.batchRetrying.value).toBe(false)
    })

    it('timedOut 组 <2 防御性不触发批量', async () => {
      palette.contentGroups.value = [timeoutGroups[0]]
      await palette.retryAllTimedOutGroups()
      expect(ContentSearchDirs).not.toHaveBeenCalled()
      expect(palette.batchRetrying.value).toBe(false)
    })

    it('全部成功后可基于剩余超时目录发起多轮重试', async () => {
      // 第一轮：3 成功 2 再超时
      ContentSearchDirs
        .mockResolvedValueOnce([{ repoName: 'repo-t1', repoPath: 'C:/t1', items: [{ filePath: 'a.txt', lineNum: 1, lineText: 'kw' }] }])
        .mockResolvedValueOnce([{ repoName: 'repo-t2', repoPath: 'C:/t2', items: [{ filePath: 'b.txt', lineNum: 2, lineText: 'kw' }] }])
        .mockResolvedValueOnce([{ repoName: 'repo-t3', repoPath: 'C:/t3', items: [{ filePath: 'c.txt', lineNum: 3, lineText: 'kw' }] }])
        .mockRejectedValueOnce({ code: 'E_SEARCH_TIMEOUT', message: '内容搜索超时已终止，可缩小范围后重试' })
        .mockRejectedValueOnce({ code: 'E_SEARCH_TIMEOUT', message: '内容搜索超时已终止，可缩小范围后重试' })
      await palette.retryAllTimedOutGroups()
      expect(palette.contentGroups.value.filter(g => g.timedOut).length).toBe(2)
      expect(palette.batchRetrying.value).toBe(false)
      expect(palette.retryingRepoPaths.value).toEqual([])
      expect(palette.hasBatchRetried.value).toBe(true) // 首轮完成有剩余 → 完成态文案
      // 第二轮：剩余 2 目录全成功
      ContentSearchDirs
        .mockResolvedValueOnce([{ repoName: 'repo-t4', repoPath: 'C:/t4', items: [{ filePath: 'd.txt', lineNum: 4, lineText: 'kw' }] }])
        .mockResolvedValueOnce([{ repoName: 'repo-t5', repoPath: 'C:/t5', items: [{ filePath: 'e.txt', lineNum: 5, lineText: 'kw' }] }])
      await palette.retryAllTimedOutGroups()
      expect(palette.contentGroups.value.filter(g => g.timedOut).length).toBe(0)
      expect(palette.contentGroups.value.length).toBe(5)
      expect(palette.hasBatchRetried.value).toBe(false) // 全成功 → banner 消失，文案复位
    })
  })

  describe('moveSelection / resetSelection', () => {
    it('下移选中索引', () => {
      palette.fileResults.value = [{}, {}, {}]
      palette.moveSelection(1)
      expect(palette.selectedIndex.value).toBe(1)
    })
    it('下移越界保持最大索引', () => {
      palette.fileResults.value = [{}, {}, {}]
      palette.selectedIndex.value = 2
      palette.moveSelection(1)
      expect(palette.selectedIndex.value).toBe(2)
    })
    it('上移下界为 0', () => {
      palette.fileResults.value = [{}, {}, {}]
      palette.selectedIndex.value = 1
      palette.moveSelection(-5)
      expect(palette.selectedIndex.value).toBe(0)
    })
    it('resetSelection 重置为 0', () => {
      palette.selectedIndex.value = 5
      palette.resetSelection()
      expect(palette.selectedIndex.value).toBe(0)
    })
  })
})
