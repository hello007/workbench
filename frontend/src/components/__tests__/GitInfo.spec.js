import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GitInfo from '../GitInfo.vue'
import { gitCache, getCacheKey } from '../../utils/gitCache'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetGitRemoteURL: vi.fn(),
  GetCommitHistory: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' },
  DocumentCopy: { template: '<i class="i-copy" />' }
}))

// stub 掉 element-plus 组件，保留 slot 链路以便断言文本；props 显式吸收避免 attrs fallthrough 警告
const stubs = {
  'el-card': { template: '<div class="el-card"><slot /></div>' },
  'el-descriptions': { template: '<div class="el-descriptions"><slot /></div>', props: ['column', 'border', 'size'] },
  'el-descriptions-item': { template: '<div class="el-descriptions-item"><slot /></div>', props: ['label'] },
  'el-text': { template: '<span><slot /></span>' },
  'el-tag': { template: '<span class="el-tag"><slot /></span>' },
  'el-button': { template: '<button v-bind="$attrs"><slot /></button>', props: ['loading', 'size', 'type', 'icon', 'circle', 'disabled'] },
  'el-link': { template: '<a><slot /></a>', props: ['href', 'type', 'underline'] }
}

describe('GitInfo.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    gitCache.clear()
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  const remoteInfo = (over = {}) => ({
    remoteUrl: 'https://example.com/repo.git',
    branch: 'main',
    isDetached: false,
    ...over
  })

  const commit = (over = {}) => ({
    sha: 'abcdef1234567890',
    shortSha: 'abcdef12',
    message: 'fix: 修复某个问题',
    timestamp: 1700000000,
    ...over
  })

  it('缓存命中时应一并恢复 info 与 latestCommit，且不再调用后端', async () => {
    const { GetGitRemoteURL, GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const repoPath = '/repo/A'

    GetGitRemoteURL.mockResolvedValueOnce(remoteInfo())
    GetCommitHistory.mockResolvedValueOnce([commit()])

    // 首次加载：未命中缓存，拉取并写缓存
    wrapper = mount(GitInfo, { props: { repoPath }, global: { stubs } })
    await flushPromises()

    expect(GetGitRemoteURL).toHaveBeenCalledTimes(1)
    expect(GetCommitHistory).toHaveBeenCalledWith(repoPath, 1, 0)
    expect(wrapper.find('.sha-text').text()).toBe('abcdef12')
    expect(wrapper.text()).toContain('fix: 修复某个问题')
    wrapper.unmount()
    wrapper = null

    // 第二次加载：缓存命中，恢复 info + latestCommit，不再调后端
    GetGitRemoteURL.mockClear()
    GetCommitHistory.mockClear()
    wrapper = mount(GitInfo, { props: { repoPath }, global: { stubs } })
    await flushPromises()

    expect(GetGitRemoteURL).not.toHaveBeenCalled()
    expect(GetCommitHistory).not.toHaveBeenCalled()
    // 关键：缓存命中恢复 latestCommit，shortSha 正常显示而非 N/A
    expect(wrapper.find('.sha-text').text()).toBe('abcdef12')
    expect(wrapper.text()).toContain('fix: 修复某个问题')
  })

  it('切换 repoPath 后缓存命中恢复新仓库的 latestCommit，不残留旧仓库提交', async () => {
    const { GetGitRemoteURL, GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const repoA = '/repo/A'
    const repoB = '/repo/B'

    // 预填充 A、B 缓存（模拟两个仓库此前均已访问过）
    gitCache.set(getCacheKey('git-info', repoA), {
      info: remoteInfo({ branch: 'main' }),
      latestCommit: commit({ shortSha: 'aaa11111', message: 'A 的提交' })
    })
    gitCache.set(getCacheKey('git-info', repoB), {
      info: remoteInfo({ branch: 'dev' }),
      latestCommit: commit({ shortSha: 'bbb22222', message: 'B 的提交' })
    })

    wrapper = mount(GitInfo, { props: { repoPath: repoA }, global: { stubs } })
    await flushPromises()
    expect(wrapper.find('.sha-text').text()).toBe('aaa11111')

    // 切换到 B：缓存命中应恢复 B 的 latestCommit，不残留 A 的
    await wrapper.setProps({ repoPath: repoB })
    await flushPromises()
    expect(wrapper.find('.sha-text').text()).toBe('bbb22222')
    expect(wrapper.text()).toContain('B 的提交')
    expect(wrapper.text()).not.toContain('A 的提交')

    // 缓存命中路径不应调后端
    expect(GetGitRemoteURL).not.toHaveBeenCalled()
    expect(GetCommitHistory).not.toHaveBeenCalled()
  })

  it('GetCommitHistory 失败时不写入缓存，下次进入可重试恢复', async () => {
    const { GetGitRemoteURL, GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const repoPath = '/repo/C'

    GetGitRemoteURL.mockResolvedValue(remoteInfo())
    GetCommitHistory.mockRejectedValueOnce(new Error('git command failed'))

    // 首次：commit 失败，本次显示 N/A，且不写缓存
    wrapper = mount(GitInfo, { props: { repoPath }, global: { stubs } })
    await flushPromises()

    // info 成功 → 卡片渲染；commit 失败 → latestCommit 为 null → N/A
    expect(wrapper.find('.sha-text').text()).toBe('N/A')
    expect(gitCache.get(getCacheKey('git-info', repoPath))).toBeNull()
    wrapper.unmount()
    wrapper = null

    // 下次进入：未缓存 → 重新调后端，commit 成功 → 正常显示（重试生效）
    GetCommitHistory.mockResolvedValueOnce([commit({ shortSha: 'ccc33333', message: 'recovered' })])
    wrapper = mount(GitInfo, { props: { repoPath }, global: { stubs } })
    await flushPromises()

    expect(GetCommitHistory).toHaveBeenCalledWith(repoPath, 1, 0)
    expect(wrapper.find('.sha-text').text()).toBe('ccc33333')
  })
})
