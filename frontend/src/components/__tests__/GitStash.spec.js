import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GitStash from '../GitStash.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  ListStashes: vi.fn(),
  StashPush: vi.fn(),
  StashPop: vi.fn(),
  StashApply: vi.fn(),
  StashDrop: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' }
}))

import { ElMessage, ElMessageBox } from 'element-plus'
import { ListStashes, StashPush, StashPop, StashApply, StashDrop } from '../../../wailsjs/go/main/App'

const stubs = {
  'el-card': { template: '<div class="el-card"><slot name="header" /><slot /></div>' },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['icon', 'loading', 'size', 'type', 'circle', 'disabled', 'plain', 'title'],
    emits: ['click']
  },
  'el-text': { template: '<span v-bind="$attrs"><slot /></span>', props: ['type', 'size'] },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
  'el-empty': { template: '<div class="el-empty" />', props: ['description', 'imageSize'] },
  'el-dialog': {
    template: '<div class="el-dialog" v-if="modelValue"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue']
  },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div class="form-item"><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<input :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'type', 'rows', 'disabled'],
    emits: ['update:modelValue']
  },
  'el-checkbox': {
    template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
    props: ['modelValue'],
    emits: ['update:modelValue']
  },
  'el-alert': { template: '<div class="el-alert">{{ title }}<slot /></div>', props: ['type', 'closable', 'showIcon', 'title', 'description'] }
}
const directives = { loading: () => {} }

const stash = (over = {}) => ({
  index: 0,
  description: '中文描述：切换分支前的改动',
  branch: 'master',
  shortSha: 'abc1234',
  dateTime: '2026-11-07 10:30',
  timestamp: 1794021045,
  ...over
})

function createWrapper(props = {}) {
  return mount(GitStash, {
    props: { repoPath: '/repo/A', ...props },
    global: { stubs, directives }
  })
}

// findBtn 按文本查找按钮（exact=true 精确匹配，否则 includes）
function findBtn(wrapper, text, exact = false) {
  return wrapper.findAll('button').find(b =>
    exact ? b.text() === text : b.text().includes(text)
  )
}

describe('GitStash.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    // 重设 confirm 默认 resolve 实现（「取消」用例改写的 reject 实现不得泄漏到后续用例）
    ElMessageBox.confirm.mockImplementation(() => Promise.resolve())
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  it('加载时调用 ListStashes 并展示条目（描述/分支/时间）', async () => {
    ListStashes.mockResolvedValue([stash()])
    wrapper = createWrapper()
    await flushPromises()

    expect(ListStashes).toHaveBeenCalledWith('/repo/A')
    expect(wrapper.text()).toContain('中文描述：切换分支前的改动')
    expect(wrapper.text()).toContain('master')
    expect(wrapper.text()).toContain('2026-11-07 10:30')
    expect(wrapper.text()).toContain('abc1234')
  })

  it('空 stash 列表渲染 empty 占位', async () => {
    ListStashes.mockResolvedValue([])
    wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('ListStashes 返回 null 时不崩溃且渲染 empty', async () => {
    ListStashes.mockResolvedValue(null)
    wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('ListStashes 失败时调用 ElMessage.error', async () => {
    ListStashes.mockRejectedValue(new Error('boom'))
    wrapper = createWrapper()
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalled()
  })

  it('暂存现场：描述与 untracked 勾选透传 StashPush 并刷新', async () => {
    ListStashes.mockResolvedValue([])
    wrapper = createWrapper()
    await flushPromises()
    vi.mocked(ListStashes).mockClear()

    await findBtn(wrapper, '暂存现场').trigger('click')
    await flushPromises()

    const input = wrapper.find('input[placeholder]')
    await input.setValue('临时现场')
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await findBtn(wrapper, '暂存', true).trigger('click')
    await flushPromises()

    expect(StashPush).toHaveBeenCalledWith('/repo/A', '临时现场', true)
    expect(ListStashes).toHaveBeenCalledTimes(1)
    expect(ElMessage.success).toHaveBeenCalled()
  })

  it('暂存现场：描述留空传空串（git 默认文案）', async () => {
    ListStashes.mockResolvedValue([])
    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '暂存现场').trigger('click')
    await flushPromises()
    await findBtn(wrapper, '暂存', true).trigger('click')
    await flushPromises()

    expect(StashPush).toHaveBeenCalledWith('/repo/A', '', false)
  })

  it('恢复（pop）：confirm 后调用 StashPop 并刷新', async () => {
    ListStashes.mockResolvedValue([stash()])
    StashPop.mockResolvedValue({ output: 'Dropped', conflictFiles: [] })
    wrapper = createWrapper()
    await flushPromises()
    vi.mocked(ListStashes).mockClear()

    await findBtn(wrapper, '恢复').trigger('click')
    await flushPromises()

    expect(ElMessageBox.confirm).toHaveBeenCalled()
    expect(StashPop).toHaveBeenCalledWith('/repo/A', 0)
    expect(ListStashes).toHaveBeenCalledTimes(1)
  })

  it('恢复（pop）冲突：弹冲突对话框且不弹 success', async () => {
    ListStashes.mockResolvedValue([stash()])
    StashPop.mockResolvedValue({ output: 'CONFLICT (content): Merge conflict in a.txt', conflictFiles: ['a.txt'] })
    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '恢复').trigger('click')
    await flushPromises()

    expect(ElMessage.success).not.toHaveBeenCalled()
    const dialogs = wrapper.findAll('.el-dialog')
    const conflictDialog = dialogs.find(d => d.text().includes('冲突未解决，stash 条目已保留'))
    expect(conflictDialog).toBeTruthy()
    expect(conflictDialog.text()).toContain('a.txt')
    expect(conflictDialog.text()).toContain('CONFLICT (content)')
  })

  it('恢复（pop）取消 confirm：不调用 StashPop', async () => {
    ElMessageBox.confirm.mockImplementation(() => Promise.reject(new Error('cancelled')))
    ListStashes.mockResolvedValue([stash()])
    wrapper = createWrapper()
    await flushPromises()
    vi.mocked(ListStashes).mockClear()

    await findBtn(wrapper, '恢复').trigger('click')
    await flushPromises()

    expect(StashPop).not.toHaveBeenCalled()
    expect(ListStashes).not.toHaveBeenCalled()
  })

  it('应用（apply）：不 confirm 直接调用 StashApply 并保留条目提示', async () => {
    ListStashes.mockResolvedValue([stash()])
    StashApply.mockResolvedValue({ output: '', conflictFiles: [] })
    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '应用').trigger('click')
    await flushPromises()

    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
    expect(StashApply).toHaveBeenCalledWith('/repo/A', 0)
    expect(ElMessage.success).toHaveBeenCalled()
  })

  it('应用（apply）冲突：弹冲突对话框', async () => {
    ListStashes.mockResolvedValue([stash()])
    StashApply.mockResolvedValue({ output: 'CONFLICT', conflictFiles: ['b.txt'] })
    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '应用').trigger('click')
    await flushPromises()

    const conflictDialog = wrapper.findAll('.el-dialog').find(d => d.text().includes('冲突未解决，stash 条目已保留'))
    expect(conflictDialog).toBeTruthy()
    expect(conflictDialog.text()).toContain('b.txt')
  })

  it('删除（drop）：confirm 后调用 StashDrop 并刷新', async () => {
    ListStashes.mockResolvedValue([stash()])
    StashDrop.mockResolvedValue()
    wrapper = createWrapper()
    await flushPromises()
    vi.mocked(ListStashes).mockClear()

    await findBtn(wrapper, '删除').trigger('click')
    await flushPromises()

    expect(ElMessageBox.confirm).toHaveBeenCalled()
    expect(StashDrop).toHaveBeenCalledWith('/repo/A', 0)
    expect(ListStashes).toHaveBeenCalledTimes(1)
  })

  it('删除（drop）取消 confirm：不调用 StashDrop', async () => {
    ElMessageBox.confirm.mockImplementation(() => Promise.reject(new Error('cancelled')))
    ListStashes.mockResolvedValue([stash()])
    wrapper = createWrapper()
    await flushPromises()
    vi.mocked(ListStashes).mockClear()

    await findBtn(wrapper, '删除').trigger('click')
    await flushPromises()

    expect(StashDrop).not.toHaveBeenCalled()
  })

  it('repoPath 变化时重新加载', async () => {
    ListStashes.mockResolvedValue([])
    wrapper = createWrapper()
    await flushPromises()
    vi.mocked(ListStashes).mockClear()

    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()

    expect(ListStashes).toHaveBeenCalledWith('/repo/B')
  })

  it('defineExpose 暴露 loadStashes', async () => {
    ListStashes.mockResolvedValue([])
    wrapper = createWrapper()
    await flushPromises()
    vi.mocked(ListStashes).mockClear()

    wrapper.vm.loadStashes()
    await flushPromises()

    expect(ListStashes).toHaveBeenCalledTimes(1)
  })
})
