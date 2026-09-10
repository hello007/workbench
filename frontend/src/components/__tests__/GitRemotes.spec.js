import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GitRemotes from '../GitRemotes.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetRemotes: vi.fn(),
  AddRemote: vi.fn(),
  RemoveRemote: vi.fn(),
  FetchRepo: vi.fn(),
  SetBranchUpstream: vi.fn(),
  GetGitRemoteURL: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' }
}))

const stubs = {
  'el-card': { template: '<div class="el-card"><slot name="header" /><slot /></div>' },
  'el-button': {
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
    props: ['icon', 'loading', 'size', 'type', 'circle', 'disabled', 'plain'],
    emits: ['click']
  },
  'el-text': { template: '<span v-bind="$attrs"><slot /></span>', props: ['type', 'size'] },
  'el-empty': { template: '<div class="el-empty" />', props: ['description'] },
  'el-dialog': {
    template: '<div class="el-dialog" v-if="modelValue"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue']
  },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div class="form-item"><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<input :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'type', 'rows'],
    emits: ['update:modelValue']
  },
  'el-select': { template: '<div class="el-select"><slot /></div>', props: ['modelValue', 'placeholder', 'size', 'disabled'] },
  'el-option': { template: '<div class="el-option" />', props: ['label', 'value'] },
  'el-checkbox': {
    template: '<label class="el-checkbox"><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" /><span><slot /></span></label>',
    props: ['modelValue'],
    emits: ['update:modelValue']
  }
}
const directives = { loading: () => {} }

const remote = (over = {}) => ({
  name: 'origin',
  url: 'https://example.com/origin.git',
  ...over
})

const remoteInfo = (over = {}) => ({
  remoteUrl: '',
  branch: 'main',
  isDetached: false,
  ...over
})

function createWrapper(props = {}) {
  return mount(GitRemotes, {
    props: { repoPath: '/repo/A', ...props },
    global: { stubs, directives }
  })
}

function findBtn(wrapper, text, exact = false) {
  return wrapper.findAll('button').find(b =>
    exact ? b.text() === text : b.text().includes(text)
  )
}

describe('GitRemotes.vue', () => {
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

  it('加载时调用 GetRemotes 与 GetGitRemoteURL，展示远程列表与当前分支', async () => {
    const { GetRemotes, GetGitRemoteURL } = await import('../../../wailsjs/go/main/App')
    GetRemotes.mockResolvedValue([remote(), remote({ name: 'upstream', url: 'https://example.com/up.git' })])
    GetGitRemoteURL.mockResolvedValue(remoteInfo())

    wrapper = createWrapper()
    await flushPromises()

    expect(GetRemotes).toHaveBeenCalledWith('/repo/A')
    expect(GetGitRemoteURL).toHaveBeenCalledWith('/repo/A')
    expect(wrapper.findAll('.remote-row')).toHaveLength(2)
    expect(wrapper.text()).toContain('origin')
    expect(wrapper.text()).toContain('upstream')
    // 当前分支展示
    expect(wrapper.text()).toContain('main')
  })

  it('GetRemotes 失败时调用 ElMessage.error', async () => {
    const { GetRemotes, GetGitRemoteURL } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetRemotes.mockRejectedValue(new Error('boom'))
    GetGitRemoteURL.mockResolvedValue(remoteInfo())

    wrapper = createWrapper()
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalled()
  })

  it('新增远程填写 name+url 后调用 AddRemote 并刷新', async () => {
    const { GetRemotes, AddRemote, GetGitRemoteURL } = await import('../../../wailsjs/go/main/App')
    GetRemotes.mockResolvedValue([])
    GetGitRemoteURL.mockResolvedValue(remoteInfo())

    wrapper = createWrapper()
    await flushPromises()
    GetRemotes.mockClear()

    await findBtn(wrapper, '新增远程').trigger('click')
    await flushPromises()

    // 对话框内两个 input：远程名称 + 远程地址
    const dialogInputs = wrapper.findAll('.el-dialog input')
    expect(dialogInputs.length).toBeGreaterThanOrEqual(2)
    await dialogInputs[0].setValue('origin')
    await dialogInputs[1].setValue('https://example.com/origin.git')

    await findBtn(wrapper, '添加', true).trigger('click')
    await flushPromises()

    expect(AddRemote).toHaveBeenCalledWith('/repo/A', 'origin', 'https://example.com/origin.git')
    expect(GetRemotes).toHaveBeenCalled()
  })

  it('删除远程确认后调用 RemoveRemote 并刷新', async () => {
    const { GetRemotes, RemoveRemote, GetGitRemoteURL } = await import('../../../wailsjs/go/main/App')
    GetRemotes.mockResolvedValue([remote()])
    GetGitRemoteURL.mockResolvedValue(remoteInfo())

    wrapper = createWrapper()
    await flushPromises()
    GetRemotes.mockClear()

    await findBtn(wrapper, '删除', true).trigger('click')
    await flushPromises()

    expect(RemoveRemote).toHaveBeenCalledWith('/repo/A', 'origin')
    expect(GetRemotes).toHaveBeenCalled()
  })

  it('行级拉取调用 FetchRepo 指定 remote', async () => {
    const { GetRemotes, FetchRepo, GetGitRemoteURL } = await import('../../../wailsjs/go/main/App')
    GetRemotes.mockResolvedValue([remote()])
    GetGitRemoteURL.mockResolvedValue(remoteInfo())
    FetchRepo.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '拉取', true).trigger('click')
    await flushPromises()

    expect(FetchRepo).toHaveBeenCalledWith('/repo/A', 'origin', false)
  })

  it('拉取全部调用 FetchRepo 空远程', async () => {
    const { GetRemotes, FetchRepo, GetGitRemoteURL } = await import('../../../wailsjs/go/main/App')
    GetRemotes.mockResolvedValue([remote()])
    GetGitRemoteURL.mockResolvedValue(remoteInfo())
    FetchRepo.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '拉取全部').trigger('click')
    await flushPromises()

    expect(FetchRepo).toHaveBeenCalledWith('/repo/A', '', false)
  })

  it('设跟踪分支调用 SetBranchUpstream（当前分支 + 默认首个远程）', async () => {
    const { GetRemotes, SetBranchUpstream, GetGitRemoteURL } = await import('../../../wailsjs/go/main/App')
    GetRemotes.mockResolvedValue([remote()])
    GetGitRemoteURL.mockResolvedValue(remoteInfo({ branch: 'main' }))
    SetBranchUpstream.mockResolvedValue(undefined)

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '设置', true).trigger('click')
    await flushPromises()

    expect(SetBranchUpstream).toHaveBeenCalledWith('/repo/A', 'main', 'origin')
  })

  it('repoPath 变化时重新加载', async () => {
    const { GetRemotes, GetGitRemoteURL } = await import('../../../wailsjs/go/main/App')
    GetRemotes.mockResolvedValue([])
    GetGitRemoteURL.mockResolvedValue(remoteInfo())

    wrapper = createWrapper()
    await flushPromises()
    GetRemotes.mockClear()

    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()

    expect(GetRemotes).toHaveBeenCalledWith('/repo/B')
  })
})
