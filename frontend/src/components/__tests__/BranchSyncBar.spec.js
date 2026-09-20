import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import BranchSyncBar from '../BranchSyncBar.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetBranchSyncInfo: vi.fn(),
  HasUpstream: vi.fn(),
  PushRepo: vi.fn(),
  PullRepo: vi.fn(),
  FetchRepo: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' }
}))

const stubs = {
  'el-tooltip': { template: '<span class="el-tooltip"><slot /></span>', props: ['content', 'placement'] },
  'el-tag': { template: '<span class="el-tag" :data-type="type"><slot /></span>', props: ['type', 'size'] },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\', $event)"><slot /><i v-if="$slots.icon"><slot name="icon" /></i></button>',
    props: ['icon', 'loading', 'size', 'type', 'circle', 'plain', 'disabled'],
    emits: ['click']
  },
  // el-dropdown split-button：主按钮 emit click，下拉项经 command（测试用 vm.$emit 触发）
  'el-dropdown': {
    name: 'ElDropdownStub',
    template: '<div class="el-dropdown"><button class="dropdown-main-btn" :disabled="disabled" @click="$emit(\'click\', $event)"><slot /></button><slot name="dropdown" /></div>',
    props: ['splitButton', 'type', 'size', 'disabled'],
    emits: ['click', 'command']
  },
  'el-dropdown-menu': { template: '<div class="el-dropdown-menu"><slot /></div>' },
  'el-dropdown-item': { template: '<div class="el-dropdown-item"><slot /></div>', props: ['command'] },
  PushResultDialog: {
    name: 'PushResultDialog',
    template: '<div class="push-result-stub" />',
    props: ['modelValue', 'output']
  }
}

// 同步态基线数据（形状对齐 model.BranchSyncInfo）
const syncInfo = (over = {}) => ({
  branch: 'main',
  ahead: 0,
  behind: 0,
  hasUpstream: true,
  detached: false,
  headSha: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0',
  refs: [
    { sha: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0', kind: 'local', name: 'main' },
    { sha: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0', kind: 'remote', name: 'origin/main' }
  ],
  ...over
})

function createWrapper(props = {}) {
  return mount(BranchSyncBar, {
    props: { repoPath: '/repo/A', ...props },
    global: { stubs }
  })
}

describe('BranchSyncBar.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  it('挂载时拉取摘要并渲染分支名，emit update:info', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    const info = syncInfo()
    GetBranchSyncInfo.mockResolvedValue(info)
    wrapper = createWrapper()
    await flushPromises()
    expect(GetBranchSyncInfo).toHaveBeenCalledWith('/repo/A')
    expect(wrapper.find('.branch-name').text()).toBe('main')
    expect(wrapper.emitted('update:info')).toBeTruthy()
    expect(wrapper.emitted('update:info')[0]).toEqual([info])
  })

  it('同步态不显示计数 tag 与操作按钮，仅 fetch', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo())
    wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.findAll('.count-tag').length).toBe(0)
    expect(wrapper.text()).not.toContain('推送')
    expect(wrapper.find('.pull-dropdown').exists()).toBe(false)
    expect(wrapper.find('.fetch-btn').exists()).toBe(true)
  })

  it('ahead>0 显示 danger 计数与推送按钮', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ ahead: 2 }))
    wrapper = createWrapper()
    await flushPromises()
    const tags = wrapper.findAll('.count-tag')
    expect(tags.length).toBe(1)
    expect(tags[0].attributes('data-type')).toBe('danger')
    expect(wrapper.text()).toContain('↑2')
    expect(wrapper.text()).toContain('推送')
  })

  it('behind>0 显示 warning 计数与 split 拉取按钮', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ behind: 3 }))
    wrapper = createWrapper()
    await flushPromises()
    const tags = wrapper.findAll('.count-tag')
    expect(tags.length).toBe(1)
    expect(tags[0].attributes('data-type')).toBe('warning')
    expect(wrapper.text()).toContain('↓3')
    expect(wrapper.find('.pull-dropdown').exists()).toBe(true)
    expect(wrapper.find('.dropdown-main-btn').text()).toContain('拉取(rebase)')
  })

  it('点击推送按钮走 HasUpstream(true) → PushRepo(path, false)，成功后 emit synced 并刷新', async () => {
    const { GetBranchSyncInfo, HasUpstream, PushRepo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ ahead: 1 }))
    HasUpstream.mockResolvedValue(true)
    PushRepo.mockResolvedValue('To origin\n   main -> main')
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text().includes('推送')).trigger('click')
    await flushPromises()
    expect(HasUpstream).toHaveBeenCalledWith('/repo/A')
    expect(PushRepo).toHaveBeenCalledWith('/repo/A', false)
    expect(wrapper.emitted('synced')).toEqual([['push']])
  })

  it('无上游时确认后走 PushRepo(path, true) set-upstream', async () => {
    const { ElMessageBox } = await import('element-plus')
    const { GetBranchSyncInfo, HasUpstream, PushRepo } = await import('../../../wailsjs/go/main/App')
    ElMessageBox.confirm.mockResolvedValue()
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ ahead: 1, hasUpstream: false, refs: [syncInfo().refs[0]] }))
    HasUpstream.mockResolvedValue(false)
    PushRepo.mockResolvedValue('pushed')
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text().includes('推送')).trigger('click')
    await flushPromises()
    expect(ElMessageBox.confirm).toHaveBeenCalled()
    expect(PushRepo).toHaveBeenCalledWith('/repo/A', true)
    expect(wrapper.emitted('synced')).toEqual([['push']])
  })

  it('无上游用户取消确认时不推送', async () => {
    const { ElMessageBox } = await import('element-plus')
    const { GetBranchSyncInfo, HasUpstream, PushRepo } = await import('../../../wailsjs/go/main/App')
    ElMessageBox.confirm.mockRejectedValue(new Error('cancel'))
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ ahead: 1, hasUpstream: false }))
    HasUpstream.mockResolvedValue(false)
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text().includes('推送')).trigger('click')
    await flushPromises()
    expect(PushRepo).not.toHaveBeenCalled()
    expect(wrapper.emitted('synced')).toBeFalsy()
  })

  it('无上游不显示计数 tag，但保留推送按钮（set-upstream 引导）', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ hasUpstream: false, refs: [syncInfo().refs[0]] }))
    wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.findAll('.count-tag').length).toBe(0)
    expect(wrapper.text()).toContain('推送')
    expect(wrapper.find('.pull-dropdown').exists()).toBe(false)
  })

  it('拉取主按钮走 PullRepo(path, true) rebase', async () => {
    const { GetBranchSyncInfo, PullRepo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ behind: 1 }))
    PullRepo.mockResolvedValue('Successfully rebased')
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.dropdown-main-btn').trigger('click')
    await flushPromises()
    expect(PullRepo).toHaveBeenCalledWith('/repo/A', true)
    expect(wrapper.emitted('synced')).toEqual([['pull']])
  })

  it('下拉项 command=merge 走 PullRepo(path, false)', async () => {
    const { GetBranchSyncInfo, PullRepo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ behind: 1 }))
    PullRepo.mockResolvedValue('Merge made by recursive')
    wrapper = createWrapper()
    await flushPromises()
    // 从 el-dropdown stub 组件实例 emit command（split-button 下拉项语义）
    const comp = wrapper.findComponent({ name: 'ElDropdownStub' })
    expect(comp.exists()).toBe(true)
    comp.vm.$emit('command', 'merge')
    await flushPromises()
    expect(PullRepo).toHaveBeenCalledWith('/repo/A', false)
    expect(wrapper.emitted('synced')).toEqual([['pull']])
  })

  it('fetch 按钮调 FetchRepo(path, origin, false)，成功后 emit synced(source=fetch) 并重算摘要', async () => {
    const { GetBranchSyncInfo, FetchRepo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo())
    FetchRepo.mockResolvedValue('Fetching origin')
    wrapper = createWrapper()
    await flushPromises()
    GetBranchSyncInfo.mockClear()
    // fetch 后远程引用更新：第二次摘要返回新 refs（验证 refresh 自刷新链路驱动父组件 badge）
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ behind: 1, refs: [syncInfo().refs[0]] }))
    await wrapper.find('.fetch-btn').trigger('click')
    await flushPromises()
    expect(FetchRepo).toHaveBeenCalledWith('/repo/A', 'origin', false)
    expect(wrapper.emitted('synced')).toEqual([['fetch']])
    // fetch 成功后内部自刷新摘要：update:info 携带新数据再次上抛（refs badge 更新链路）
    expect(GetBranchSyncInfo).toHaveBeenCalledWith('/repo/A')
    const updates = wrapper.emitted('update:info')
    expect(updates[updates.length - 1][0].behind).toBe(1)
  })

  it('fetch 失败仅提示不阻塞（不抛错、摘要条保留）', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetBranchSyncInfo, FetchRepo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo())
    FetchRepo.mockRejectedValue({ code: 'E_GIT_IN_PROGRESS', message: '该仓库有 Git 操作进行中，请稍后重试' })
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.fetch-btn').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('该仓库有 Git 操作进行中，请稍后重试')
    // 降级不阻塞：摘要条仍渲染
    expect(wrapper.find('.branch-name').exists()).toBe(true)
  })

  it('detached HEAD 仅提示分离头指针，无计数与操作按钮', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo({
      detached: true,
      branch: 'a1b2c3d4',
      hasUpstream: false,
      refs: [{ sha: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0', kind: 'head', name: 'HEAD' }]
    }))
    wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.text()).toContain('分离头指针')
    expect(wrapper.text()).not.toContain('推送')
    expect(wrapper.find('.pull-dropdown').exists()).toBe(false)
    expect(wrapper.find('.fetch-btn').exists()).toBe(false)
  })

  it('摘要拉取失败静默降级隐藏摘要条，不报错', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockRejectedValue(new Error('not a repo'))
    wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.find('.branch-sync-bar').exists()).toBe(false)
    expect(wrapper.emitted('synced')).toBeFalsy()
  })

  it('切换 repoPath 清旧摘要并重新拉取', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    GetBranchSyncInfo.mockResolvedValue(syncInfo())
    wrapper = createWrapper()
    await flushPromises()
    GetBranchSyncInfo.mockClear()
    GetBranchSyncInfo.mockResolvedValue(syncInfo({ branch: 'dev' }))
    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()
    expect(GetBranchSyncInfo).toHaveBeenCalledWith('/repo/B')
    expect(wrapper.find('.branch-name').text()).toBe('dev')
  })

  it('切仓库后旧仓库在途响应晚返回被丢弃（不写状态不 emit，防旧 refs 污染新仓库）', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    // A 的摘要请求挂起（可控 resolve），B 的摘要请求立即返回
    let resolveA
    const pendingA = new Promise((resolve) => { resolveA = resolve })
    GetBranchSyncInfo.mockImplementation((path) => {
      if (path === '/repo/A') return pendingA
      return Promise.resolve(syncInfo({ branch: 'dev' }))
    })
    wrapper = createWrapper() // repoPath=/repo/A，挂载即发起 A 摘要请求（挂起中）
    await flushPromises()
    // 切到 B：watch 清旧摘要并立即拉取 B（已 resolve）
    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()
    expect(wrapper.find('.branch-name').text()).toBe('dev')
    // A 的在途响应此刻才晚到
    resolveA(syncInfo({ branch: 'stale-A', refs: [{ sha: 'f'.repeat(40), kind: 'local', name: 'stale-branch' }] }))
    await flushPromises()
    // 状态仍为 B 数据：A 的过期响应被丢弃，不覆盖状态、不再 emit 旧数据
    expect(wrapper.find('.branch-name').text()).toBe('dev')
    const updates = wrapper.emitted('update:info')
    expect(updates[updates.length - 1][0].branch).toBe('dev')
    expect(wrapper.text()).not.toContain('stale-branch')
  })

  it('切仓库后旧仓库摘要请求失败晚返回被丢弃（不清掉新仓库已渲染的摘要）', async () => {
    const { GetBranchSyncInfo } = await import('../../../wailsjs/go/main/App')
    let rejectA
    const failingA = new Promise((_, reject) => { rejectA = reject })
    GetBranchSyncInfo.mockImplementation((path) => {
      if (path === '/repo/A') return failingA
      return Promise.resolve(syncInfo({ branch: 'dev' }))
    })
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()
    expect(wrapper.find('.branch-name').text()).toBe('dev')
    // A 的失败响应此刻才晚到：不得把 B 已渲染的摘要降级清空
    rejectA(new Error('stale failure'))
    await flushPromises()
    expect(wrapper.find('.branch-sync-bar').exists()).toBe(true)
    expect(wrapper.find('.branch-name').text()).toBe('dev')
  })
})
