import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import LocalChanges from '../LocalChanges.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetLocalChanges: vi.fn(),
  DiscardChanges: vi.fn(),
  CommitFiles: vi.fn(),
  PushRepo: vi.fn(),
  HasUpstream: vi.fn(),
  StageFiles: vi.fn(),
  UnstageFiles: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' },
  ArrowDown: { template: '<i class="i-down" />' }
}))

// el-table stub：渲染行 + 挂载即 emit selection-change（模拟全选）+ 双击行 emit row-dblclick
const ElTableC = {
  name: 'ElTableC',
  template: '<div class="el-table"><div v-for="(row, i) in data" :key="i" class="table-row" @dblclick="$emit(\'row-dblclick\', row)"><span class="row-path">{{ row.path }}</span></div></div>',
  props: ['data', 'height', 'size'],
  emits: ['selection-change', 'row-dblclick'],
  mounted() {
    if (this.data && this.data.length) this.$emit('selection-change', this.data)
  }
}

// el-dropdown stub：保留 slot，供 findComponent 后 $emit('command') 触发 onMoreCommand
const ElDropdownC = {
  name: 'ElDropdownC',
  template: '<div class="el-dropdown"><slot /><slot name="dropdown" /></div>',
  props: ['trigger'],
  emits: ['command']
}

const stubs = {
  'el-card': { template: '<div class="el-card"><slot name="header" /><slot /></div>' },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'loading', 'disabled', 'size', 'icon', 'circle', 'plain'],
    emits: ['click']
  },
  'el-icon': { template: '<i><slot /></i>' },
  'el-input': {
    template: '<textarea :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'type', 'rows', 'placeholder', 'resize'],
    emits: ['update:modelValue', 'change']
  },
  'el-table': ElTableC,
  'el-table-column': { template: '<div class="el-table-col"><slot /></div>', props: ['type', 'label', 'prop', 'width', 'align'] },
  'el-empty': { template: '<div class="el-empty" />', props: ['description', 'imageSize'] },
  'el-dropdown': ElDropdownC,
  'el-dropdown-menu': { template: '<div class="el-dropdown-menu"><slot /></div>' },
  'el-dropdown-item': { template: '<div class="el-dropdown-item" :data-cmd="command"><slot /></div>', props: ['command', 'disabled'] },
  FileDiffDialog: { template: '<div class="file-diff-dialog-stub" />', props: ['modelValue', 'repoPath', 'file'] }
}
const directives = { loading: () => {} }

const changes = [
  { path: 'src/a.go', status: 'M' },
  { path: 'src/b.go', status: 'A' },
  { path: 'src/c.go', status: '?' }
]

// 含 staged 字段的混合数据：未暂存(a,c) + 已暂存(b,d)
const mixedChanges = [
  { path: 'src/a.go', status: 'M', staged: false },
  { path: 'src/b.go', status: 'A', staged: true },
  { path: 'src/c.go', status: '?', staged: false },
  { path: 'src/d.go', status: 'M', staged: true }
]

// 全部已暂存：用于 stageSelected 无可暂存项的警告分支
const stagedChanges = [
  { path: 'src/a.go', status: 'M', staged: true },
  { path: 'src/b.go', status: 'A', staged: true }
]

function mockBindings(over = {}) {
  return {
    getLocalChanges: vi.fn(() => Promise.resolve(changes)),
    discardChanges: vi.fn(() => Promise.resolve(true)),
    commitFiles: vi.fn(() => Promise.resolve(true)),
    pushRepo: vi.fn(() => Promise.resolve('Everything up-to-date')),
    hasUpstream: vi.fn(() => Promise.resolve(true)),
    ...over
  }
}

async function createWrapper(props = {}, bindings = {}) {
  const App = await import('../../../wailsjs/go/main/App')
  Object.keys(bindings).forEach(k => {
    const name = k.charAt(0).toUpperCase() + k.slice(1)
    if (App[name] && App[name].mockImplementation) {
      App[name].mockImplementation(bindings[k])
    }
  })
  const wrapper = mount(LocalChanges, {
    props: { repoPath: '/repo/A', ...props },
    global: { stubs, directives }
  })
  await flushPromises()
  return wrapper
}

describe('LocalChanges.vue', () => {
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

  it('挂载时加载本地变动并渲染表格行', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    wrapper = await createWrapper()
    expect(GetLocalChanges).toHaveBeenCalledWith('/repo/A')
    expect(wrapper.findAll('.table-row').length).toBe(3)
    // 文件数 tag
    expect(wrapper.text()).toContain('3 个文件')
  })

  it('状态标签按 status 映射类型与文案（经 setupState 直测纯函数分支）', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    wrapper = await createWrapper()
    const { getStatusType, getStatusLabel } = wrapper.vm.$.setupState
    expect(getStatusType('M')).toBe('warning')
    expect(getStatusType('A')).toBe('success')
    expect(getStatusType('D')).toBe('danger')
    expect(getStatusType('?')).toBe('info')
    expect(getStatusType('X')).toBe('info') // default
    expect(getStatusLabel('M')).toBe('已修改')
    expect(getStatusLabel('A')).toBe('已添加')
    expect(getStatusLabel('D')).toBe('已删除')
    expect(getStatusLabel('R')).toBe('已重命名')
    expect(getStatusLabel('?')).toBe('未跟踪')
    expect(getStatusLabel('Z')).toBe('Z') // default 回退原值
  })

  it('空变动展示 el-empty 且无 footer', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue([])
    wrapper = await createWrapper()
    expect(wrapper.find('.el-empty').exists()).toBe(true)
    expect(wrapper.find('.changes-footer').exists()).toBe(false)
  })

  it('加载失败时弹错误提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockRejectedValue(new Error('boom'))
    wrapper = await createWrapper()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
  })

  it('canCommit：有勾选 + 有提交信息时按钮可用', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    wrapper = await createWrapper()
    // el-table stub 挂载即 emit selection-change → selectedChanges = 3
    const commitBtn = wrapper.findAll('button').find(b => b.text().includes('提交') && !b.text().includes('推送'))
    // 未填提交信息 → 禁用
    expect(commitBtn.attributes('disabled')).toBeDefined()
    // 填提交信息
    const textarea = wrapper.find('textarea')
    await textarea.setValue('fix: 修复')
    await nextTick()
    expect(commitBtn.attributes('disabled')).toBeUndefined()
  })

  it('点击提交调用 CommitFiles 并成功提示 + 重新加载', async () => {
    const { ElMessage } = await import('element-plus')
    const { CommitFiles, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    CommitFiles.mockResolvedValue(true)
    wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('fix: 修复')
    await wrapper.findAll('button').find(b => b.text().includes('提交') && !b.text().includes('推送')).trigger('click')
    await flushPromises()
    expect(CommitFiles).toHaveBeenCalledWith('/repo/A', 'fix: 修复', ['src/a.go', 'src/b.go', 'src/c.go'])
    expect(ElMessage.success).toHaveBeenCalledWith('提交成功')
    // 提交后重新加载
    expect(GetLocalChanges.mock.calls.length).toBeGreaterThanOrEqual(2)
    // 提交信息清空
    expect(wrapper.find('textarea').element.value).toBe('')
  })

  it('提交并推送：CommitFiles 成功后接 doPush（有上游）', async () => {
    const { CommitFiles, PushRepo, HasUpstream, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    CommitFiles.mockResolvedValue(true)
    HasUpstream.mockResolvedValue(true)
    PushRepo.mockResolvedValue('Pushed')
    wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('feat: 新功能')
    await wrapper.findAll('button').find(b => b.text().includes('提交并推送')).trigger('click')
    await flushPromises()
    expect(CommitFiles).toHaveBeenCalled()
    expect(HasUpstream).toHaveBeenCalledWith('/repo/A')
    expect(PushRepo).toHaveBeenCalledWith('/repo/A', false)
  })

  it('CommitFiles 失败时弹错误提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { CommitFiles, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    CommitFiles.mockRejectedValue(new Error('commit fail'))
    wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('msg')
    await wrapper.findAll('button').find(b => b.text().includes('提交') && !b.text().includes('推送')).trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('commit fail'))
  })

  it('推送：无上游且确认设置上游 → PushRepo(setUpstream=true)', async () => {
    const { ElMessageBox, ElMessage } = await import('element-plus')
    const { PushRepo, HasUpstream, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    HasUpstream.mockResolvedValue(false)
    ElMessageBox.confirm.mockResolvedValueOnce('confirm')
    PushRepo.mockResolvedValue('Done')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text() === '推送').trigger('click')
    await flushPromises()
    expect(ElMessageBox.confirm).toHaveBeenCalled()
    expect(PushRepo).toHaveBeenCalledWith('/repo/A', true)
  })

  it('推送：无上游且用户取消 → ElMessage.info 且不推送', async () => {
    const { ElMessageBox, ElMessage } = await import('element-plus')
    const { PushRepo, HasUpstream, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    HasUpstream.mockResolvedValue(false)
    ElMessageBox.confirm.mockRejectedValueOnce(new Error('cancel'))
    PushRepo.mockResolvedValue('Done')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text() === '推送').trigger('click')
    await flushPromises()
    expect(ElMessage.info).toHaveBeenCalledWith('已取消推送')
    expect(PushRepo).not.toHaveBeenCalled()
  })

  it('推送：HasUpstream 探测失败时按常规推送', async () => {
    const { ElMessage } = await import('element-plus')
    const { PushRepo, HasUpstream, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    HasUpstream.mockRejectedValue(new Error('probe fail'))
    PushRepo.mockResolvedValue('ok')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text() === '推送').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('无法判断上游'))
    expect(PushRepo).toHaveBeenCalledWith('/repo/A', false)
  })

  it('推送超长输出截断展示', async () => {
    const { ElMessage } = await import('element-plus')
    const { PushRepo, HasUpstream, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    HasUpstream.mockResolvedValue(true)
    const long = 'x'.repeat(300)
    PushRepo.mockResolvedValue(long)
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text() === '推送').trigger('click')
    await flushPromises()
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('...'))
  })

  it('推送失败时弹错误提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { PushRepo, HasUpstream, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    HasUpstream.mockResolvedValue(true)
    PushRepo.mockRejectedValue(new Error('push fail'))
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text() === '推送').trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('push fail'))
  })

  it('回滚选中：确认后调用 DiscardChanges 传选中路径', async () => {
    const { ElMessageBox } = await import('element-plus')
    const { DiscardChanges, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    DiscardChanges.mockResolvedValue(true)
    wrapper = await createWrapper()
    const dropdown = wrapper.findComponent(ElDropdownC)
    dropdown.vm.$emit('command', 'discardSelected')
    await flushPromises()
    expect(ElMessageBox.confirm).toHaveBeenCalled()
    expect(DiscardChanges).toHaveBeenCalledWith('/repo/A', ['src/a.go', 'src/b.go', 'src/c.go'])
  })

  it('回滚选中：用户取消时不调用 DiscardChanges', async () => {
    const { ElMessageBox } = await import('element-plus')
    const { DiscardChanges, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    ElMessageBox.confirm.mockRejectedValueOnce(new Error('cancel'))
    wrapper = await createWrapper()
    const dropdown = wrapper.findComponent(ElDropdownC)
    dropdown.vm.$emit('command', 'discardSelected')
    await flushPromises()
    expect(DiscardChanges).not.toHaveBeenCalled()
  })

  it('全部回滚：确认后调用 DiscardChanges 传空数组', async () => {
    const { DiscardChanges, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    DiscardChanges.mockResolvedValue(true)
    wrapper = await createWrapper()
    const dropdown = wrapper.findComponent(ElDropdownC)
    dropdown.vm.$emit('command', 'discardAll')
    await flushPromises()
    expect(DiscardChanges).toHaveBeenCalledWith('/repo/A', [])
  })

  it('双击行打开 diff 弹窗', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    wrapper = await createWrapper()
    await wrapper.find('.table-row').trigger('dblclick')
    expect(wrapper.find('.file-diff-dialog-stub').exists()).toBe(true)
  })

  it('切换 repoPath 清空变动并重新加载', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    wrapper = await createWrapper()
    GetLocalChanges.mockClear()
    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()
    expect(GetLocalChanges).toHaveBeenCalledWith('/repo/B')
  })

  it('defineExpose 暴露 loadChanges', async () => {
    wrapper = await createWrapper()
    expect(typeof wrapper.vm.loadChanges).toBe('function')
  })

  // ===== 暂存 / 取消暂存交互 =====

  it('sortedChanges：未暂存组在上、已暂存组在下（Staged 字段驱动）', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    wrapper = await createWrapper()
    const sorted = wrapper.vm.$.setupState.sortedChanges
    expect(sorted.map(c => c.path)).toEqual(['src/a.go', 'src/c.go', 'src/b.go', 'src/d.go'])
  })

  it('rowClassName：按 staged 返回 row-staged / row-unstaged', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    wrapper = await createWrapper()
    const { rowClassName } = wrapper.vm.$.setupState
    expect(rowClassName({ row: { staged: true } })).toBe('row-staged')
    expect(rowClassName({ row: { staged: false } })).toBe('row-unstaged')
  })

  it('stageSingle：行级暂存调用 StageFiles(path, [row.path]) 并刷新', async () => {
    const { StageFiles, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    StageFiles.mockResolvedValue(undefined)
    wrapper = await createWrapper()
    GetLocalChanges.mockClear()
    await wrapper.vm.stageSingle({ path: 'src/a.go', staged: false })
    await flushPromises()
    expect(StageFiles).toHaveBeenCalledWith('/repo/A', ['src/a.go'])
    expect(GetLocalChanges).toHaveBeenCalled()
  })

  it('unstageSingle：行级取消暂存调用 UnstageFiles(path, [row.path]) 并刷新', async () => {
    const { UnstageFiles, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    UnstageFiles.mockResolvedValue(undefined)
    wrapper = await createWrapper()
    GetLocalChanges.mockClear()
    await wrapper.vm.unstageSingle({ path: 'src/b.go', staged: true })
    await flushPromises()
    expect(UnstageFiles).toHaveBeenCalledWith('/repo/A', ['src/b.go'])
    expect(GetLocalChanges).toHaveBeenCalled()
  })

  it('stageSingle 失败时弹错误提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { StageFiles, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    StageFiles.mockRejectedValue(new Error('stage fail'))
    wrapper = await createWrapper()
    await wrapper.vm.stageSingle({ path: 'src/a.go', staged: false })
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('stage fail'))
  })

  it('stageSelected：批量暂存仅传未暂存的选中路径', async () => {
    const { StageFiles, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    StageFiles.mockResolvedValue(undefined)
    wrapper = await createWrapper()
    // el-table stub 挂载即 emit selection-change → selectedChanges = 全部 4 行（按 sorted 顺序）
    const dropdown = wrapper.findComponent(ElDropdownC)
    dropdown.vm.$emit('command', 'stageSelected')
    await flushPromises()
    expect(StageFiles).toHaveBeenCalledWith('/repo/A', ['src/a.go', 'src/c.go'])
  })

  it('unstageSelected：批量取消暂存仅传已暂存的选中路径', async () => {
    const { UnstageFiles, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    UnstageFiles.mockResolvedValue(undefined)
    wrapper = await createWrapper()
    const dropdown = wrapper.findComponent(ElDropdownC)
    dropdown.vm.$emit('command', 'unstageSelected')
    await flushPromises()
    expect(UnstageFiles).toHaveBeenCalledWith('/repo/A', ['src/b.go', 'src/d.go'])
  })

  it('stageSelected：选中均已暂存时弹警告且不调用 StageFiles', async () => {
    const { ElMessage } = await import('element-plus')
    const { StageFiles, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(stagedChanges)
    StageFiles.mockResolvedValue(undefined)
    wrapper = await createWrapper()
    const dropdown = wrapper.findComponent(ElDropdownC)
    dropdown.vm.$emit('command', 'stageSelected')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('均已暂存'))
    expect(StageFiles).not.toHaveBeenCalled()
  })

  it('unstageSelected：选中均未暂存时弹警告且不调用 UnstageFiles', async () => {
    const { ElMessage } = await import('element-plus')
    const { UnstageFiles, GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes) // changes 无 staged 字段 → 均视为未暂存
    UnstageFiles.mockResolvedValue(undefined)
    wrapper = await createWrapper()
    const dropdown = wrapper.findComponent(ElDropdownC)
    dropdown.vm.$emit('command', 'unstageSelected')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('均未暂存'))
    expect(UnstageFiles).not.toHaveBeenCalled()
  })

  it('更多下拉渲染 stageSelected / unstageSelected 命令项', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(changes)
    wrapper = await createWrapper()
    expect(wrapper.find('[data-cmd="stageSelected"]').exists()).toBe(true)
    expect(wrapper.find('[data-cmd="unstageSelected"]').exists()).toBe(true)
  })
})
