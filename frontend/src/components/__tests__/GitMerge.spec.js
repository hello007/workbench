import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GitMerge from '../GitMerge.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  Merge: vi.fn(),
  Rebase: vi.fn(),
  CherryPick: vi.fn(),
  GetConflictState: vi.fn(() => Promise.resolve({ type: 'none', files: [] })),
  ResolveConflict: vi.fn(),
  ContinueMerge: vi.fn(),
  ContinueRebase: vi.fn(),
  ContinueCherryPick: vi.fn(),
  AbortMerge: vi.fn(),
  AbortRebase: vi.fn(),
  AbortCherryPick: vi.fn(),
  SkipRebase: vi.fn(),
  OpenInVSCode: vi.fn(() => Promise.resolve(true))
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' }
}))

// el-radio-group + el-radio stub：provide/inject 传递选中态，change 时 emit update:modelValue
const stubs = {
  'el-card': { template: '<div class="el-card"><slot name="header" /><slot /></div>' },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['icon', 'loading', 'size', 'type', 'circle', 'disabled', 'plain', 'text'],
    emits: ['click']
  },
  'el-text': { template: '<span v-bind="$attrs"><slot /></span>', props: ['type', 'size'] },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
  'el-empty': { template: '<div class="el-empty" />', props: ['description'] },
  'el-divider': { template: '<hr />' },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div class="form-item"><span class="form-item-label">{{ label }}</span><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<input :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'type', 'rows', 'disabled'],
    emits: ['update:modelValue']
  },
  'el-radio-group': {
    template: '<div class="el-radio-group" :data-model="modelValue"><slot /></div>',
    props: ['modelValue'],
    emits: ['update:modelValue', 'change'],
    provide() { return { elRadioGroup: this } }
  },
  'el-radio': {
    template: '<label class="el-radio"><input type="radio" :value="value" :checked="isChecked" @change="onChange" /><span><slot /></span></label>',
    props: ['value'],
    inject: { elRadioGroup: { default: null } },
    computed: {
      isChecked() { return this.elRadioGroup && this.elRadioGroup.modelValue === this.value }
    },
    methods: {
      onChange() {
        if (this.elRadioGroup) {
          this.elRadioGroup.$emit('update:modelValue', this.value)
          this.elRadioGroup.$emit('change', this.value)
        }
      }
    }
  },
  'el-select': {
    template: '<select class="el-select" :value="modelValue" @change="onChange"><slot /></select>',
    props: ['modelValue', 'size', 'disabled'],
    emits: ['update:modelValue', 'change'],
    methods: {
      onChange(e) {
        const v = e.target.value
        this.$emit('update:modelValue', v)
        this.$emit('change', v)
      }
    }
  },
  'el-option': { template: '<option class="el-option" :value="value">{{ label }}</option>', props: ['label', 'value'] }
}
const directives = { loading: () => {} }

function createWrapper(props = {}) {
  return mount(GitMerge, {
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

describe('GitMerge.vue', () => {
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

  it('挂载时调用 GetConflictState 检测冲突态，无冲突不渲染面板', async () => {
    const { GetConflictState } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'none', files: [] })

    wrapper = createWrapper()
    await flushPromises()

    expect(GetConflictState).toHaveBeenCalledWith('/repo/A')
    expect(wrapper.find('.conflict-panel').exists()).toBe(false)
  })

  it('GetConflictState 失败时调用 ElMessage.error', async () => {
    const { GetConflictState } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetConflictState.mockRejectedValue(new Error('boom'))

    wrapper = createWrapper()
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
  })

  it('存在 merge 冲突时渲染冲突面板 + 文件列表 + 类型标签', async () => {
    const { GetConflictState } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({
      type: 'merge',
      files: ['src/a.txt', 'src/b.txt']
    })

    wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.conflict-panel').exists()).toBe(true)
    expect(wrapper.text()).toContain('合并冲突')
    expect(wrapper.findAll('.conflict-row')).toHaveLength(2)
    expect(wrapper.text()).toContain('src/a.txt')
  })

  it('冲突文件为空时渲染 empty 占位', async () => {
    const { GetConflictState } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'rebase', files: [] })

    wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('默认 merge 操作展示策略选择，切换到 rebase 后隐藏策略', async () => {
    const { GetConflictState } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'none', files: [] })

    wrapper = createWrapper()
    await flushPromises()

    // 默认 merge：策略 select 存在
    expect(wrapper.find('.el-select').exists()).toBe(true)
    // 目标 label 为"目标分支"
    expect(wrapper.text()).toContain('目标分支')

    // 切换到 rebase：点击 value=rebase 的 radio
    await wrapper.find('input[type="radio"][value="rebase"]').trigger('change')
    await flushPromises()

    expect(wrapper.find('.el-select').exists()).toBe(false)
    expect(wrapper.text()).toContain('变基')
  })

  it('切换到 cherry-pick 后目标 label 变为"提交 SHA"', async () => {
    const { GetConflictState } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'none', files: [] })

    wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('input[type="radio"][value="cherry-pick"]').trigger('change')
    await flushPromises()

    expect(wrapper.text()).toContain('提交 SHA')
    // cherry-pick 不显示策略 select
    expect(wrapper.find('.el-select').exists()).toBe(false)
  })

  it('执行 merge 调用 Merge(path, branch, mode) 且无冲突时 success', async () => {
    const { Merge, GetConflictState } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    Merge.mockResolvedValue('Updating abc..def')
    GetConflictState.mockResolvedValue({ type: 'none', files: [] })

    wrapper = createWrapper()
    await flushPromises()

    // 输入目标分支
    await wrapper.find('input[placeholder]').setValue('feature/x')
    // 策略切到 no-ff
    await wrapper.find('select').setValue('no-ff')

    await findBtn(wrapper, '执行', true).trigger('click')
    await flushPromises()

    expect(Merge).toHaveBeenCalledWith('/repo/A', 'feature/x', 'no-ff')
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('操作完成'))
  })

  it('执行 merge 产生冲突时不弹 success，展示冲突面板', async () => {
    const { Merge, GetConflictState } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    Merge.mockResolvedValue('CONFLICT (content): Merge conflict in src/a.txt')
    // 第一次（挂载）none，第二次（执行后）merge 冲突
    GetConflictState.mockResolvedValueOnce({ type: 'none', files: [] })
    GetConflictState.mockResolvedValueOnce({ type: 'merge', files: ['src/a.txt'] })

    wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('input[placeholder]').setValue('feature/x')
    await findBtn(wrapper, '执行', true).trigger('click')
    await flushPromises()

    expect(Merge).toHaveBeenCalledWith('/repo/A', 'feature/x', 'ff')
    expect(ElMessage.success).not.toHaveBeenCalled()
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('冲突'))
    expect(wrapper.find('.conflict-panel').exists()).toBe(true)
  })

  it('执行 rebase 调用 Rebase(path, branch)', async () => {
    const { Rebase, GetConflictState } = await import('../../../wailsjs/go/main/App')
    Rebase.mockResolvedValue('')
    GetConflictState.mockResolvedValue({ type: 'none', files: [] })

    wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('input[type="radio"][value="rebase"]').trigger('change')
    await flushPromises()
    await wrapper.find('input[placeholder]').setValue('main')
    await findBtn(wrapper, '执行', true).trigger('click')
    await flushPromises()

    expect(Rebase).toHaveBeenCalledWith('/repo/A', 'main')
  })

  it('执行 cherry-pick 调用 CherryPick(path, sha)', async () => {
    const { CherryPick, GetConflictState } = await import('../../../wailsjs/go/main/App')
    CherryPick.mockResolvedValue('')
    GetConflictState.mockResolvedValue({ type: 'none', files: [] })

    wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('input[type="radio"][value="cherry-pick"]').trigger('change')
    await flushPromises()
    await wrapper.find('input[placeholder]').setValue('a1b2c3d')
    await findBtn(wrapper, '执行', true).trigger('click')
    await flushPromises()

    expect(CherryPick).toHaveBeenCalledWith('/repo/A', 'a1b2c3d')
  })

  it('执行时空目标弹警告且不调用操作', async () => {
    const { Merge } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '执行', true).trigger('click')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('目标分支不能为空'))
    expect(Merge).not.toHaveBeenCalled()
  })

  it('执行抛错时调用 ElMessage.error', async () => {
    const { Merge, GetConflictState } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    Merge.mockRejectedValue(new Error('dirty worktree'))
    GetConflictState.mockResolvedValue({ type: 'none', files: [] })

    wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('input[placeholder]').setValue('feature/x')
    await findBtn(wrapper, '执行', true).trigger('click')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('dirty worktree'))
  })

  it('点击"打开"调用 OpenInVSCode 拼接仓库根路径', async () => {
    const { GetConflictState, OpenInVSCode } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'merge', files: ['src/a.txt'] })

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '打开', true).trigger('click')
    await flushPromises()

    expect(OpenInVSCode).toHaveBeenCalledWith('/repo/A/src/a.txt')
  })

  it('OpenInVSCode 返回 false 时弹错误', async () => {
    const { GetConflictState, OpenInVSCode } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetConflictState.mockResolvedValue({ type: 'merge', files: ['src/a.txt'] })
    OpenInVSCode.mockResolvedValueOnce(false)

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '打开', true).trigger('click')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('VSCode'))
  })

  it('点击"标记已解决"调用 ResolveConflict 并刷新冲突态', async () => {
    const { GetConflictState, ResolveConflict } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'merge', files: ['src/a.txt'] })
    ResolveConflict.mockResolvedValue(undefined)

    wrapper = createWrapper()
    await flushPromises()
    GetConflictState.mockClear()

    await findBtn(wrapper, '标记已解决', true).trigger('click')
    await flushPromises()

    expect(ResolveConflict).toHaveBeenCalledWith('/repo/A', 'src/a.txt')
    expect(GetConflictState).toHaveBeenCalled()
  })

  it('点击"继续"按冲突类型分发到 ContinueMerge', async () => {
    const { GetConflictState, ContinueMerge } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'merge', files: ['src/a.txt'] })
    ContinueMerge.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '继续', true).trigger('click')
    await flushPromises()

    expect(ContinueMerge).toHaveBeenCalledWith('/repo/A')
  })

  it('继续 rebase 冲突后冲突态清空时弹 success', async () => {
    const { GetConflictState, ContinueRebase } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    // 挂载时 rebase 冲突，继续后 none
    GetConflictState.mockResolvedValueOnce({ type: 'rebase', files: ['src/a.txt'] })
    GetConflictState.mockResolvedValueOnce({ type: 'none', files: [] })
    ContinueRebase.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '继续', true).trigger('click')
    await flushPromises()

    expect(ContinueRebase).toHaveBeenCalledWith('/repo/A')
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('冲突已解决'))
  })

  it('点击"中止"按冲突类型分发到 AbortCherryPick', async () => {
    const { GetConflictState, AbortCherryPick } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'cherry-pick', files: ['src/a.txt'] })
    AbortCherryPick.mockResolvedValue(undefined)

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '中止', true).trigger('click')
    await flushPromises()

    expect(AbortCherryPick).toHaveBeenCalledWith('/repo/A')
  })

  it('rebase 冲突态显示"跳过"按钮且调用 SkipRebase', async () => {
    const { GetConflictState, SkipRebase } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'rebase', files: ['src/a.txt'] })
    SkipRebase.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    const skipBtn = findBtn(wrapper, '跳过', true)
    expect(skipBtn).toBeDefined()
    await skipBtn.trigger('click')
    await flushPromises()

    expect(SkipRebase).toHaveBeenCalledWith('/repo/A')
  })

  it('merge 冲突态不显示"跳过"按钮', async () => {
    const { GetConflictState } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'merge', files: ['src/a.txt'] })

    wrapper = createWrapper()
    await flushPromises()

    expect(findBtn(wrapper, '跳过', true)).toBeUndefined()
  })

  it('repoPath 变化时重置并重新检测冲突态', async () => {
    const { GetConflictState } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'none', files: [] })

    wrapper = createWrapper()
    await flushPromises()
    GetConflictState.mockClear()

    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()

    expect(GetConflictState).toHaveBeenCalledWith('/repo/B')
  })

  it('defineExpose 暴露 refreshConflictState', async () => {
    const { GetConflictState } = await import('../../../wailsjs/go/main/App')
    GetConflictState.mockResolvedValue({ type: 'none', files: [] })

    wrapper = createWrapper()
    await flushPromises()

    expect(typeof wrapper.vm.refreshConflictState).toBe('function')
  })
})
