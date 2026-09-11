import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GitBranches from '../GitBranches.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetBranches: vi.fn(),
  CreateBranch: vi.fn(),
  DeleteBranch: vi.fn(),
  RenameBranch: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' }
}))

const stubs = {
  'el-card': { template: '<div class="el-card"><slot name="header" /><slot /></div>' },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['icon', 'loading', 'size', 'type', 'circle', 'disabled', 'plain'],
    emits: ['click']
  },
  'el-text': { template: '<span v-bind="$attrs"><slot /></span>', props: ['type', 'size'] },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
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
    props: ['modelValue', 'placeholder', 'type', 'rows', 'disabled'],
    emits: ['update:modelValue']
  }
}
const directives = { loading: () => {} }

const branch = (over = {}) => ({
  name: 'main',
  isRemote: false,
  isCurrent: true,
  ...over
})

function createWrapper(props = {}) {
  return mount(GitBranches, {
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

describe('GitBranches.vue', () => {
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

  it('加载时调用 GetBranches 并展示本地分支（过滤远程）+ 标记当前', async () => {
    const { GetBranches } = await import('../../../wailsjs/go/main/App')
    GetBranches.mockResolvedValue({
      branches: [
        branch(),
        branch({ name: 'feature/x', isCurrent: false }),
        branch({ name: 'remotes/origin/main', isRemote: true })
      ]
    })

    wrapper = createWrapper()
    await flushPromises()

    expect(GetBranches).toHaveBeenCalledWith('/repo/A')
    // 远程分支被过滤，仅 2 个本地分支行
    expect(wrapper.findAll('.branch-row')).toHaveLength(2)
    expect(wrapper.text()).toContain('main')
    expect(wrapper.text()).toContain('feature/x')
    expect(wrapper.text()).toContain('当前')
    // 远程分支不在面板展示
    expect(wrapper.text()).not.toContain('remotes/origin/main')
  })

  it('空分支列表渲染 empty 占位', async () => {
    const { GetBranches } = await import('../../../wailsjs/go/main/App')
    GetBranches.mockResolvedValue({ branches: [] })

    wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('GetBranches 返回 null 时不崩溃且渲染 empty', async () => {
    const { GetBranches } = await import('../../../wailsjs/go/main/App')
    GetBranches.mockResolvedValue(null)

    wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('GetBranches 失败时调用 ElMessage.error', async () => {
    const { GetBranches } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetBranches.mockRejectedValue(new Error('boom'))

    wrapper = createWrapper()
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
  })

  it('新建分支填写 name 后调用 CreateBranch 并刷新', async () => {
    const { GetBranches, CreateBranch } = await import('../../../wailsjs/go/main/App')
    GetBranches.mockResolvedValue({ branches: [] })
    CreateBranch.mockResolvedValue(undefined)

    wrapper = createWrapper()
    await flushPromises()
    GetBranches.mockClear()

    await findBtn(wrapper, '新建分支').trigger('click')
    await flushPromises()

    const input = wrapper.find('.el-dialog input')
    await input.setValue('feature/new')

    await findBtn(wrapper, '创建', true).trigger('click')
    await flushPromises()

    expect(CreateBranch).toHaveBeenCalledWith('/repo/A', 'feature/new')
    expect(GetBranches).toHaveBeenCalled()
  })

  it('新建分支名为空时弹警告且不调用 CreateBranch', async () => {
    const { CreateBranch } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    CreateBranch.mockResolvedValue(undefined)

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '新建分支').trigger('click')
    await flushPromises()

    await findBtn(wrapper, '创建', true).trigger('click')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('分支名不能为空'))
    expect(CreateBranch).not.toHaveBeenCalled()
  })

  it('删除分支 -d 成功：调用 DeleteBranch(force=false) 并刷新', async () => {
    const { GetBranches, DeleteBranch } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox } = await import('element-plus')
    GetBranches.mockResolvedValue({
      branches: [
        branch(),
        branch({ name: 'feature/x', isCurrent: false })
      ]
    })
    DeleteBranch.mockResolvedValue(undefined)

    wrapper = createWrapper()
    await flushPromises()
    GetBranches.mockClear()

    const rows = wrapper.findAll('.branch-row')
    // 第二行为非当前分支 feature/x
    const deleteBtn = rows[1].findAll('button').find(b => b.text() === '删除')
    await deleteBtn.trigger('click')
    await flushPromises()

    expect(DeleteBranch).toHaveBeenCalledWith('/repo/A', 'feature/x', false)
    // -d 成功不应弹二次确认
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
    expect(GetBranches).toHaveBeenCalled()
  })

  it('删除分支 -d 失败 + 确认：二次确认后调用 DeleteBranch(force=true)', async () => {
    const { GetBranches, DeleteBranch } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox } = await import('element-plus')
    GetBranches.mockResolvedValue({
      branches: [branch(), branch({ name: 'feature/x', isCurrent: false })]
    })
    // 第一次 -d 失败（未合并），第二次 -D 成功
    DeleteBranch.mockRejectedValueOnce(new Error('not fully merged'))
    DeleteBranch.mockResolvedValueOnce(undefined)
    ElMessageBox.confirm.mockResolvedValue('confirm')

    wrapper = createWrapper()
    await flushPromises()

    const rows = wrapper.findAll('.branch-row')
    const deleteBtn = rows[1].findAll('button').find(b => b.text() === '删除')
    await deleteBtn.trigger('click')
    await flushPromises()

    expect(DeleteBranch).toHaveBeenNthCalledWith(1, '/repo/A', 'feature/x', false)
    expect(DeleteBranch).toHaveBeenNthCalledWith(2, '/repo/A', 'feature/x', true)
    expect(ElMessageBox.confirm).toHaveBeenCalled()
  })

  it('删除分支 -d 失败 + 取消：不调用 DeleteBranch(force=true)', async () => {
    const { GetBranches, DeleteBranch } = await import('../../../wailsjs/go/main/App')
    GetBranches.mockResolvedValue({
      branches: [branch(), branch({ name: 'feature/x', isCurrent: false })]
    })
    DeleteBranch.mockRejectedValueOnce(new Error('not fully merged'))
    // ElMessageBox.confirm 拒绝 = 用户取消
    const { ElMessageBox } = await import('element-plus')
    ElMessageBox.confirm.mockRejectedValueOnce(new Error('cancel'))

    wrapper = createWrapper()
    await flushPromises()

    const rows = wrapper.findAll('.branch-row')
    const deleteBtn = rows[1].findAll('button').find(b => b.text() === '删除')
    await deleteBtn.trigger('click')
    await flushPromises()

    // 仅调用一次（-d），未调用 -D
    expect(DeleteBranch).toHaveBeenCalledTimes(1)
    expect(DeleteBranch).toHaveBeenCalledWith('/repo/A', 'feature/x', false)
    expect(DeleteBranch).not.toHaveBeenCalledWith('/repo/A', 'feature/x', true)
  })

  it('当前分支删除按钮禁用', async () => {
    const { GetBranches } = await import('../../../wailsjs/go/main/App')
    GetBranches.mockResolvedValue({
      branches: [branch(), branch({ name: 'feature/x', isCurrent: false })]
    })

    wrapper = createWrapper()
    await flushPromises()

    const rows = wrapper.findAll('.branch-row')
    // 当前分支（第一行）删除按钮禁用
    const currentDeleteBtn = rows[0].findAll('button').find(b => b.text() === '删除')
    expect(currentDeleteBtn.attributes('disabled')).toBeDefined()
    // 非当前分支（第二行）删除按钮可用
    const otherDeleteBtn = rows[1].findAll('button').find(b => b.text() === '删除')
    expect(otherDeleteBtn.attributes('disabled')).toBeUndefined()
  })

  it('重命名分支填写新名后调用 RenameBranch(oldName, newName) 并刷新', async () => {
    const { GetBranches, RenameBranch } = await import('../../../wailsjs/go/main/App')
    GetBranches.mockResolvedValue({
      branches: [branch(), branch({ name: 'feature/x', isCurrent: false })]
    })
    RenameBranch.mockResolvedValue(undefined)

    wrapper = createWrapper()
    await flushPromises()
    GetBranches.mockClear()

    const rows = wrapper.findAll('.branch-row')
    const renameBtn = rows[1].findAll('button').find(b => b.text() === '重命名')
    await renameBtn.trigger('click')
    await flushPromises()

    // 重命名对话框：新分支名 input 按 placeholder 定位
    const newNameInput = wrapper.findAll('input').find(i => i.attributes('placeholder') === '输入新分支名')
    await newNameInput.setValue('feature/renamed')

    // 对话框内提交按钮（非行级重命名按钮）
    const submitBtn = wrapper.find('.el-dialog').findAll('button').find(b => b.text() === '重命名')
    await submitBtn.trigger('click')
    await flushPromises()

    expect(RenameBranch).toHaveBeenCalledWith('/repo/A', 'feature/x', 'feature/renamed')
    expect(GetBranches).toHaveBeenCalled()
  })

  it('重命名新名为空时弹警告且不调用 RenameBranch', async () => {
    const { RenameBranch } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    RenameBranch.mockResolvedValue(undefined)

    wrapper = createWrapper()
    await flushPromises()

    const rows = wrapper.findAll('.branch-row')
    const renameBtn = rows[1].findAll('button').find(b => b.text() === '重命名')
    await renameBtn.trigger('click')
    await flushPromises()

    const submitBtn = wrapper.find('.el-dialog').findAll('button').find(b => b.text() === '重命名')
    await submitBtn.trigger('click')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('新分支名不能为空'))
    expect(RenameBranch).not.toHaveBeenCalled()
  })

  it('repoPath 变化时重新加载', async () => {
    const { GetBranches } = await import('../../../wailsjs/go/main/App')
    GetBranches.mockResolvedValue({ branches: [] })

    wrapper = createWrapper()
    await flushPromises()
    GetBranches.mockClear()

    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()

    expect(GetBranches).toHaveBeenCalledWith('/repo/B')
  })

  it('defineExpose 暴露 loadBranches', async () => {
    const { GetBranches } = await import('../../../wailsjs/go/main/App')
    GetBranches.mockResolvedValue({ branches: [] })

    wrapper = createWrapper()
    await flushPromises()

    expect(typeof wrapper.vm.loadBranches).toBe('function')
  })
})
