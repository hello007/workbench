import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GitSubmodules from '../GitSubmodules.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetSubmodules: vi.fn(),
  InitSubmodules: vi.fn(),
  UpdateSubmodules: vi.fn(),
  AddSubmodule: vi.fn(),
  RemoveSubmodule: vi.fn(),
  CheckoutSubmoduleBranch: vi.fn()
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
  // el-tag 渲染 data-type 便于断言四色状态标签
  'el-tag': { template: '<span class="el-tag" :data-type="type"><slot /></span>', props: ['type', 'size'] },
  'el-empty': { template: '<div class="el-empty" />', props: ['description'] },
  'el-dialog': {
    template: '<div class="el-dialog" v-if="modelValue"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue']
  },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div class="form-item"><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<input :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'type', 'rows', 'disabled'],
    emits: ['update:modelValue']
  },
  // el-select 渲染原生 select 便于 setValue 触发 update:modelValue
  'el-select': {
    template: '<select class="el-select" :value="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.value)"><slot /></select>',
    props: ['modelValue', 'placeholder', 'size', 'disabled'],
    emits: ['update:modelValue']
  },
  'el-option': { template: '<option :value="value">{{ label }}</option>', props: ['label', 'value'] },
  'el-checkbox': {
    template: '<label class="el-checkbox"><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" /><span><slot /></span></label>',
    props: ['modelValue'],
    emits: ['update:modelValue']
  }
}
const directives = { loading: () => {} }

// 构造 GitSubmodule 测试数据（11 字段默认干净态）
const sub = (over = {}) => ({
  path: 'libs/a',
  sha: 'abcdef1234567890abcdef1234567890abcdef12',
  shortSha: 'abcdef12',
  describe: '',
  branch: '',
  url: 'https://example.com/a.git',
  initialized: true,
  shaMismatch: false,
  dirty: false,
  conflict: false,
  detached: false,
  ...over
})

function createWrapper(props = {}) {
  return mount(GitSubmodules, {
    props: { repoPath: '/repo/A', ...props },
    global: { stubs, directives }
  })
}

// 按文本查找按钮（exact=true 精确匹配，否则 includes）
function findBtn(wrapper, text, exact = false) {
  return wrapper.findAll('button').find(b =>
    exact ? b.text().trim() === text : b.text().includes(text)
  )
}

// 在已打开的弹窗内按文本查找按钮（scope 到 .el-dialog，避免与 header/行按钮同名冲突）
function findDialogBtn(wrapper, text) {
  const dlg = wrapper.find('.el-dialog')
  if (!dlg.exists()) return undefined
  return dlg.findAll('button').find(b => b.text().trim() === text)
}

describe('GitSubmodules.vue', () => {
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

  it('加载时调用 GetSubmodules 并展示多态子模块列表与状态标签颜色', async () => {
    const { GetSubmodules } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([
      sub({ path: 'libs/clean', describe: 'v1.0.0' }),
      sub({ path: 'libs/dirty', dirty: true }),
      sub({ path: 'libs/uninit', initialized: false }),
      sub({ path: 'libs/conflict', conflict: true }),
      sub({ path: 'libs/mismatch', shaMismatch: true }),
      sub({ path: 'libs/detached', detached: true, branch: 'main', describe: 'v2.3.1' })
    ])

    wrapper = createWrapper()
    await flushPromises()

    expect(GetSubmodules).toHaveBeenCalledWith('/repo/A')
    const rows = wrapper.findAll('.submodule-row')
    expect(rows).toHaveLength(6)

    // 状态标签四色判定（每行首个 el-tag 为状态标签）
    expect(rows[0].findAll('.el-tag')[0].attributes('data-type')).toBe('success') // 干净
    expect(rows[1].findAll('.el-tag')[0].attributes('data-type')).toBe('warning') // 已修改
    expect(rows[2].findAll('.el-tag')[0].attributes('data-type')).toBe('info')    // 未初始化
    expect(rows[3].findAll('.el-tag')[0].attributes('data-type')).toBe('danger')  // 冲突
    expect(rows[4].findAll('.el-tag')[0].attributes('data-type')).toBe('warning') // 未同步

    // detached 行：状态标签 success + 额外 detached tag（warning）
    const detachedTags = rows[5].findAll('.el-tag')
    expect(detachedTags).toHaveLength(2)
    expect(detachedTags[0].attributes('data-type')).toBe('success')
    expect(detachedTags[1].text()).toBe('detached')
    expect(detachedTags[1].attributes('data-type')).toBe('warning')

    // describe 字段渲染
    expect(rows[0].text()).toContain('v1.0.0')
    expect(rows[5].text()).toContain('v2.3.1')
  })

  it('空子模块列表渲染 empty 占位', async () => {
    const { GetSubmodules } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([])

    wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('GetSubmodules 失败时调用 ElMessage.error', async () => {
    const { GetSubmodules } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetSubmodules.mockRejectedValue(new Error('boom'))

    wrapper = createWrapper()
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
  })

  it('初始化按钮调用 InitSubmodules 并刷新列表', async () => {
    const { GetSubmodules, InitSubmodules } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/a' })])
    InitSubmodules.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()
    GetSubmodules.mockClear()

    await findBtn(wrapper, '初始化', true).trigger('click')
    await flushPromises()

    expect(InitSubmodules).toHaveBeenCalledWith('/repo/A')
    expect(GetSubmodules).toHaveBeenCalled()
  })

  it('更新弹窗默认参数（checkout + recursive + init）调用 UpdateSubmodules 全量更新', async () => {
    const { GetSubmodules, UpdateSubmodules } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/a' })])
    UpdateSubmodules.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()
    GetSubmodules.mockClear()

    // 点击 header "更新" 打开弹窗
    await findBtn(wrapper, '更新', true).trigger('click')
    await flushPromises()

    // 默认 mode=checkout / recursive=true / init=true / subPath=''，提交
    await findDialogBtn(wrapper, '更新').trigger('click')
    await flushPromises()

    expect(UpdateSubmodules).toHaveBeenCalledWith('/repo/A', 'checkout', true, true, '')
    expect(GetSubmodules).toHaveBeenCalled()
  })

  it('更新弹窗改 mode 为 rebase 并关 init 后调用 UpdateSubmodules 参数正确', async () => {
    const { GetSubmodules, UpdateSubmodules } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/a' })])
    UpdateSubmodules.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '更新', true).trigger('click')
    await flushPromises()

    // 改 mode 为 rebase
    await wrapper.find('.el-select').setValue('rebase')
    // 关掉 init 复选框（第二个 checkbox：recursive, init）
    const checkboxes = wrapper.findAll('input[type=checkbox]')
    expect(checkboxes).toHaveLength(2)
    await checkboxes[1].setChecked(false)

    await findDialogBtn(wrapper, '更新').trigger('click')
    await flushPromises()

    // rebase + recursive=true（未动）+ init=false + subPath=''
    expect(UpdateSubmodules).toHaveBeenCalledWith('/repo/A', 'rebase', true, false, '')
  })

  it('行内更新按钮打开弹窗带 subPath 并调用 UpdateSubmodules 单行更新', async () => {
    const { GetSubmodules, UpdateSubmodules } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/a' })])
    UpdateSubmodules.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    // 点击行内 "更新" 按钮（scope 到行，区别于 header 全量更新）
    const row = wrapper.findAll('.submodule-row')[0]
    const rowUpdateBtn = row.findAll('button').find(b => b.text().trim() === '更新')
    await rowUpdateBtn.trigger('click')
    await flushPromises()

    await findDialogBtn(wrapper, '更新').trigger('click')
    await flushPromises()

    expect(UpdateSubmodules).toHaveBeenCalledWith('/repo/A', 'checkout', true, true, 'libs/a')
  })

  it('添加弹窗填写 url/path/branch 后调用 AddSubmodule 并刷新', async () => {
    const { GetSubmodules, AddSubmodule } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([])
    AddSubmodule.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()
    GetSubmodules.mockClear()

    await findBtn(wrapper, '添加子模块').trigger('click')
    await flushPromises()

    const inputs = wrapper.findAll('.el-dialog input')
    expect(inputs.length).toBeGreaterThanOrEqual(3)
    await inputs[0].setValue('https://example.com/sub.git')
    await inputs[1].setValue('libs/sub')
    await inputs[2].setValue('main')

    await findBtn(wrapper, '添加', true).trigger('click')
    await flushPromises()

    expect(AddSubmodule).toHaveBeenCalledWith('/repo/A', 'https://example.com/sub.git', 'libs/sub', 'main')
    expect(GetSubmodules).toHaveBeenCalled()
  })

  it('添加弹窗 branch 留空时传空串（走默认分支）', async () => {
    const { GetSubmodules, AddSubmodule } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([])
    AddSubmodule.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '添加子模块').trigger('click')
    await flushPromises()

    const inputs = wrapper.findAll('.el-dialog input')
    await inputs[0].setValue('https://example.com/sub.git')
    await inputs[1].setValue('libs/sub')
    // branch 留空

    await findBtn(wrapper, '添加', true).trigger('click')
    await flushPromises()

    expect(AddSubmodule).toHaveBeenCalledWith('/repo/A', 'https://example.com/sub.git', 'libs/sub', '')
  })

  it('添加弹窗 url 为空时 warning 不调用 AddSubmodule', async () => {
    const { GetSubmodules, AddSubmodule } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetSubmodules.mockResolvedValue([])
    AddSubmodule.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '添加子模块').trigger('click')
    await flushPromises()

    await findBtn(wrapper, '添加', true).trigger('click')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('仓库地址不能为空')
    expect(AddSubmodule).not.toHaveBeenCalled()
  })

  it('删除子模块二次确认后调用 RemoveSubmodule 并刷新', async () => {
    const { GetSubmodules, RemoveSubmodule } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/a' })])
    RemoveSubmodule.mockResolvedValue(undefined)

    wrapper = createWrapper()
    await flushPromises()
    GetSubmodules.mockClear()

    await findBtn(wrapper, '删除', true).trigger('click')
    await flushPromises()

    expect(RemoveSubmodule).toHaveBeenCalledWith('/repo/A', 'libs/a')
    expect(GetSubmodules).toHaveBeenCalled()
  })

  it('detached 子模块点击切换跟踪分支调用 CheckoutSubmoduleBranch', async () => {
    const { GetSubmodules, CheckoutSubmoduleBranch } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/d', detached: true, branch: 'main' })])
    CheckoutSubmoduleBranch.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()
    GetSubmodules.mockClear()

    await findBtn(wrapper, '切换跟踪分支').trigger('click')
    await flushPromises()

    expect(CheckoutSubmoduleBranch).toHaveBeenCalledWith('/repo/A', 'libs/d', 'main')
    expect(GetSubmodules).toHaveBeenCalled()
  })

  it('detached 子模块 branch 为空时切换按钮禁用并提示未配置跟踪分支', async () => {
    const { GetSubmodules, CheckoutSubmoduleBranch } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/d', detached: true, branch: '' })])
    CheckoutSubmoduleBranch.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    const btn = findBtn(wrapper, '切换跟踪分支')
    expect(btn.exists()).toBe(true)
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.attributes('title')).toBe('未配置跟踪分支')
    expect(CheckoutSubmoduleBranch).not.toHaveBeenCalled()
  })

  it('点击 SHA 复制完整提交号到剪贴板', async () => {
    const { GetSubmodules } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/a' })])

    const writeText = vi.fn(() => Promise.resolve())
    vi.stubGlobal('navigator', { clipboard: { writeText } })

    wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('.sm-sha').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('abcdef1234567890abcdef1234567890abcdef12')
    expect(ElMessage.success).toHaveBeenCalledWith('已复制到剪贴板')
    vi.unstubAllGlobals()
  })

  it('变更操作被并发拒绝时 handleGitError 走 warning 提示', async () => {
    const { GetSubmodules, RemoveSubmodule } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/a' })])
    RemoveSubmodule.mockRejectedValue(new Error('该仓库有 Git 操作进行中，请稍后重试'))

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '删除', true).trigger('click')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('该仓库有 Git 操作进行中，请稍后重试')
  })

  it('变更操作普通失败时 handleGitError 走 error 提示', async () => {
    const { GetSubmodules, RemoveSubmodule } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetSubmodules.mockResolvedValue([sub({ path: 'libs/a' })])
    RemoveSubmodule.mockRejectedValue(new Error('git rm fail'))

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '删除', true).trigger('click')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('git rm fail'))
  })

  it('repoPath 变化时重新加载', async () => {
    const { GetSubmodules } = await import('../../../wailsjs/go/main/App')
    GetSubmodules.mockResolvedValue([])

    wrapper = createWrapper()
    await flushPromises()
    GetSubmodules.mockClear()

    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()

    expect(GetSubmodules).toHaveBeenCalledWith('/repo/B')
  })
})
