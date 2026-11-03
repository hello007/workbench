import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { nextTick } from 'vue'

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetClaudeDefaultModel: vi.fn(() => Promise.resolve('')),
  GetGitInfo: vi.fn(),
  GetBranches: vi.fn(),
  OpenDirectoryDialog: vi.fn(),
  GetBranchCompareAlignment: vi.fn(),
  GetBranchCompareFileTree: vi.fn(),
  GetBranchCompareFileDiff: vi.fn(),
  GetCommitChangedFiles: vi.fn(),
  GetCommitFileDiff: vi.fn()
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

import BranchCompareView from '../BranchCompareView.vue'
import {
  GetGitInfo,
  GetBranches,
  OpenDirectoryDialog,
  GetBranchCompareAlignment,
  GetBranchCompareFileTree,
  GetBranchCompareFileDiff
} from '../../../wailsjs/go/main/App'
import { useUiStore, useDirectoryStore } from '../../store'
import { gitCache } from '../../utils/gitCache'

const REPO = '/fake/repo'

const BRANCHES = {
  branches: [
    { name: 'main', isRemote: false, isCurrent: true },
    { name: 'feature/x', isRemote: false, isCurrent: false },
    { name: 'origin/main', isRemote: true, isCurrent: false }
  ]
}

const ALIGNMENT = {
  branchA: 'main',
  branchB: 'origin/main',
  mergeBaseHash: '001122',
  commonHistoryCount: 3,
  rows: [
    {
      key: 'pair-1',
      a: { hash: 'aaa111', shortHash: 'aaa111', author: 'e2e', email: 'e2e@x.com', date: 1757400000, subject: 'feat: 登录' },
      b: { hash: 'bbb111', shortHash: 'bbb111', author: 'e2e', email: 'e2e@x.com', date: 1757486400, subject: 'feat: 登录' },
      side: 'matched',
      matchedBy: 'hash'
    },
    { key: 'a-only-1', a: { hash: 'aaa333', shortHash: 'aaa333', author: 'e2e', email: 'e2e@x.com', date: 1757400000, subject: 'feat: 仅 A' }, b: null, side: 'a-only', matchedBy: 'none' }
  ],
  stats: { aOnly: 1, bOnly: 0, matched: 1, patchMatched: 0, commonBelow: 3 },
  truncated: false
}

const FILE_TREE = [
  { path: 'src/a.js', status: 'modified', additions: 2, deletions: 1, binary: false },
  { path: 'src/b.js', status: 'added', additions: 5, deletions: 0, binary: false }
]

// 子组件 stub：捕获 props（视图编排层测试，子组件有各自 spec）
const stubs = {
  BranchCompareStatsBar: {
    template: '<div class="stats-stub" :data-has-stats="stats ? \'yes\' : \'no\'" :data-truncated="String(truncated)" :data-unrelated="String(unrelated)" />',
    props: ['stats', 'truncated', 'unrelated']
  },
  BranchCompareCommitTable: {
    template: '<div class="commit-table-stub" :data-rows="rows.length" />',
    props: ['rows', 'branchA', 'branchB'],
    emits: ['selectCommit']
  },
  BranchCompareFileTree: {
    template: '<div class="file-tree-stub" :data-files="files.length" @click="$emit(\'selectFile\', { path: \'src/a.js\' })" />',
    props: ['files', 'selectedPath'],
    emits: ['selectFile']
  },
  BranchCompareDiffViewer: {
    template: '<div class="diff-viewer-stub" :data-diff="diffText" />',
    props: ['diffText', 'baseLabel', 'targetLabel']
  },
  BranchCompareCommitDetail: {
    template: '<div class="commit-detail-stub" :data-visible="String(modelValue)" />',
    props: ['modelValue', 'repoPath', 'commit'],
    emits: ['update:modelValue']
  },
  'el-empty': { template: '<div class="el-empty"><slot name="description" />{{ description }}</div>', props: ['description'] },
  'el-alert': { template: '<div class="el-alert" :data-type="type"><slot /></div>', props: ['type', 'title', 'closable', 'showIcon'] },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'loading', 'disabled'],
    emits: ['click']
  },
  'el-select': { template: '<div class="el-select"><slot /></div>', props: ['modelValue', 'filterable', 'placeholder', 'size', 'loading'] },
  'el-option': { template: '<div class="el-option" />', props: ['label', 'value'] },
  'el-tag': { template: '<span class="el-tag" :data-type="type"><slot /></span>', props: ['type', 'size'] }
}

const directives = { loading: {} }

// 挂载工厂：activePanel 默认分支对比页（触发自动对比链路）
async function createWrapper({ panel = 'branchcompare' } = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const directoryStore = useDirectoryStore()
  directoryStore.directories = [{ id: 'dir-1', name: 'repo', path: REPO, isGitRepo: true }]
  directoryStore.selectedDirectoryId = 'dir-1'
  const uiStore = useUiStore()
  uiStore.activePanel = panel
  const wrapper = mount(BranchCompareView, {
    global: { stubs, directives }
  })
  await flushPromises()
  return { wrapper, uiStore, directoryStore }
}

beforeEach(() => {
  vi.clearAllMocks()
  gitCache.clear()
  GetGitInfo.mockResolvedValue({ isRepo: true, branch: 'main', path: REPO })
  GetBranches.mockResolvedValue(BRANCHES)
  GetBranchCompareAlignment.mockResolvedValue(ALIGNMENT)
  GetBranchCompareFileTree.mockResolvedValue(FILE_TREE)
  GetBranchCompareFileDiff.mockResolvedValue('@@ -1,1 +1,1 @@\n ctx\n')
  OpenDirectoryDialog.mockResolvedValue('')
})

describe('BranchCompareView.vue', () => {
  it('挂载即用当前工作目录做种子并加载仓库元信息 + 分支列表', async () => {
    await createWrapper()
    expect(GetGitInfo).toHaveBeenCalledWith(REPO)
    expect(GetBranches).toHaveBeenCalledWith(REPO)
  })

  it('非 git 仓库时展示引导空态且不拉分支', async () => {
    GetGitInfo.mockResolvedValue({ isRepo: false })
    const { wrapper } = await createWrapper()
    expect(GetBranches).not.toHaveBeenCalled()
    expect(wrapper.find('.el-empty').text()).toContain('不是 Git 仓库')
  })

  it('未选仓库（无工作目录）时展示选择仓库空态', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const directoryStore = useDirectoryStore()
    directoryStore.directories = []
    directoryStore.selectedDirectoryId = ''
    const wrapper = mount(BranchCompareView, { global: { stubs, directives } })
    await flushPromises()
    expect(wrapper.find('.el-empty').text()).toContain('请先选择一个 Git 仓库')
    expect(GetGitInfo).not.toHaveBeenCalled()
  })

  it('面板可见且 A/B 可解析时自动对比（对齐 + 文件树并发拉取）并渲染统计与表格', async () => {
    const { wrapper } = await createWrapper()
    expect(GetBranchCompareAlignment).toHaveBeenCalledWith(REPO, 'main', 'origin/main')
    expect(GetBranchCompareFileTree).toHaveBeenCalledWith(REPO, 'main', 'origin/main')
    expect(wrapper.find('.stats-stub').attributes('data-has-stats')).toBe('yes')
    expect(wrapper.find('.commit-table-stub').attributes('data-rows')).toBe('2')
    expect(wrapper.find('.file-tree-stub').attributes('data-files')).toBe('2')
  })

  it('面板不可见时不自动对比，切到分支对比页后补载', async () => {
    const { wrapper, uiStore } = await createWrapper({ panel: 'directory' })
    expect(GetBranchCompareAlignment).not.toHaveBeenCalled()
    uiStore.activePanel = 'branchcompare'
    await flushPromises()
    expect(GetBranchCompareAlignment).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.commit-table-stub').attributes('data-rows')).toBe('2')
  })

  it('交换 A/B 后以交换后的分支重新拉取', async () => {
    const { wrapper } = await createWrapper()
    expect(GetBranchCompareAlignment).toHaveBeenCalledTimes(1)
    await wrapper.vm.swapBranches()
    await flushPromises()
    expect(GetBranchCompareAlignment).toHaveBeenCalledTimes(2)
    expect(GetBranchCompareAlignment).toHaveBeenLastCalledWith(REPO, 'origin/main', 'main')
    expect(GetBranchCompareFileTree).toHaveBeenLastCalledWith(REPO, 'origin/main', 'main')
  })

  it('requestSeq 丢弃过期响应：快速重对比时旧结果不覆盖新状态', async () => {
    // 第一次对比挂起，期间触发第二次对比（立即 resolve）
    let resolveFirst
    GetBranchCompareAlignment.mockImplementationOnce(() => new Promise(r => { resolveFirst = r }))
    const { wrapper, uiStore } = await createWrapper({ panel: 'directory' })
    uiStore.activePanel = 'branchcompare' // 第一次对比（挂起）
    await flushPromises()

    GetBranchCompareAlignment.mockResolvedValueOnce({ ...ALIGNMENT, branchB: 'feature/x' })
    await wrapper.vm.runCompare() // 第二次对比（立即完成）
    await flushPromises()
    expect(wrapper.vm.alignment.branchB).toBe('feature/x')

    // 旧响应后到：不覆盖
    resolveFirst({ ...ALIGNMENT, branchB: 'stale' })
    await flushPromises()
    expect(wrapper.vm.alignment.branchB).toBe('feature/x')
  })

  it('点文件懒加载分支 diff 并写入 gitCache（命中缓存不再请求）', async () => {
    const { wrapper } = await createWrapper()
    expect(GetBranchCompareFileDiff).not.toHaveBeenCalled()
    await wrapper.find('.file-tree-stub').trigger('click')
    await flushPromises()
    expect(GetBranchCompareFileDiff).toHaveBeenCalledWith(REPO, 'main', 'origin/main', 'src/a.js')
    expect(wrapper.vm.diffText).toContain('ctx')

    // 再次点同一文件：缓存命中，不发起第二次请求
    GetBranchCompareFileDiff.mockClear()
    await wrapper.find('.file-tree-stub').trigger('click')
    await flushPromises()
    expect(GetBranchCompareFileDiff).not.toHaveBeenCalled()
  })

  it('刷新清空本仓库分支对比缓存后重新拉取（文件 diff 重新请求）', async () => {
    const { wrapper } = await createWrapper()
    await wrapper.find('.file-tree-stub').trigger('click')
    await flushPromises()
    expect(GetBranchCompareFileDiff).toHaveBeenCalledTimes(1)

    await wrapper.vm.refreshCompare()
    await flushPromises()
    // 对齐 + 文件树重拉
    expect(GetBranchCompareAlignment).toHaveBeenCalledTimes(2)
    // 新一轮对比清空选中态，缓存也已清空：重新点文件应再发起请求
    await wrapper.find('.file-tree-stub').trigger('click')
    await flushPromises()
    expect(GetBranchCompareFileDiff).toHaveBeenCalledTimes(2)
  })

  it('点提交行打开提交详情弹层', async () => {
    const { wrapper } = await createWrapper()
    await wrapper.vm.onSelectCommit({ a: { hash: 'aaa111', shortHash: 'aaa111' }, b: null, side: 'matched' })
    await nextTick()
    expect(wrapper.find('.commit-detail-stub').attributes('data-visible')).toBe('true')
    expect(wrapper.vm.detailCommit.shortHash).toBe('aaa111')
  })

  it('浏览取消（空串返回）不触发仓库校验', async () => {
    const { wrapper } = await createWrapper()
    GetGitInfo.mockClear()
    OpenDirectoryDialog.mockResolvedValueOnce('') // 用户取消
    await wrapper.vm.browseRepo()
    await flushPromises()
    expect(OpenDirectoryDialog).toHaveBeenCalledWith('选择 Git 仓库目录')
    expect(GetGitInfo).not.toHaveBeenCalled()
  })

  it('浏览选择非 git 仓库时 warning 引导且不落地', async () => {
    const { wrapper } = await createWrapper()
    const { ElMessage } = await import('element-plus')
    GetGitInfo.mockResolvedValueOnce({ isRepo: false })
    OpenDirectoryDialog.mockResolvedValueOnce('/not/a/repo')
    await wrapper.vm.browseRepo()
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('所选目录不是 Git 仓库，请重新选择')
    // 仓库路径未变（仍是种子目录）
    expect(wrapper.vm.repoPath).toBe(REPO)
  })

  it('对比失败经 handleError 分流并清空结果', async () => {
    const { wrapper } = await createWrapper()
    const { ElMessage } = await import('element-plus')
    // 结构化 AppError（超时，warning 类）
    GetBranchCompareAlignment.mockRejectedValueOnce({ code: 'E_GIT_TIMEOUT', message: 'Git 操作超时' })
    await wrapper.vm.runCompare()
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('Git 操作超时')
    expect(wrapper.vm.alignment).toBeNull()
  })
})
