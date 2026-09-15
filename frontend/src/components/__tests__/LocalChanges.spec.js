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

// ai-task:done 事件总线：EventsOn 记录 handler 供测试手动触发（vi.hoisted 避免工厂函数 TDZ）
const aiEventBus = vi.hoisted(() => ({ handlers: {} }))

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetLocalChanges: vi.fn(),
  DiscardChanges: vi.fn(),
  CommitFiles: vi.fn(),
  PushRepo: vi.fn(),
  HasUpstream: vi.fn(),
  StageFiles: vi.fn(),
  UnstageFiles: vi.fn(),
  GetStagedDiffText: vi.fn(),
  GetRecentCommitSubjects: vi.fn(),
  GetUncommittedDiffText: vi.fn(),
  RunAiFunction: vi.fn(),
  CancelAiTask: vi.fn()
}))

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  // 对齐 Wails v2：EventsOn 返回「注销本监听器」闭包，onBeforeUnmount 调它精准移除（不清同名全部）
  EventsOn: vi.fn((event, handler) => {
    aiEventBus.handlers[event] = handler
    return () => {
      if (aiEventBus.handlers[event] === handler) delete aiEventBus.handlers[event]
    }
  }),
  EventsOff: vi.fn((event) => {
    delete aiEventBus.handlers[event]
  })
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' },
  ArrowDown: { template: '<i class="i-down" />' },
  MagicStick: { template: '<i class="i-magic" />' },
  Loading: { template: '<i class="i-loading" />' },
  View: { template: '<i class="i-view" />' }
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
  'el-tooltip': { template: '<span class="el-tooltip"><slot /></span>', props: ['content', 'placement'] },
  'el-dialog': {
    template: '<div class="el-dialog" v-if="modelValue"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width']
  },
  FileDiffDialog: { name: 'FileDiffDialog', template: '<div class="file-diff-dialog-stub" :data-file="file" />', props: ['modelValue', 'repoPath', 'file'] },
  CodeReviewResult: {
    name: 'CodeReviewResult',
    template: '<div class="code-review-stub" v-if="modelValue" :data-loading="loading"><span v-for="(i, idx) in issues" :key="idx" class="stub-issue" @click="$emit(\'locate-file\', i.file)">{{ i.file }}</span></div>',
    props: ['modelValue', 'issues', 'summary', 'loading'],
    emits: ['update:modelValue', 'locate-file']
  }
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
    Object.keys(aiEventBus.handlers).forEach(k => delete aiEventBus.handlers[k])
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

  it('切仓库时取消在途 AI 任务并重置态（防旧仓库候选串入新仓库 + loading 卡死）', async () => {
    const { GetLocalChanges, GetStagedDiffText, GetRecentCommitSubjects, RunAiFunction, CancelAiTask } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetStagedDiffText.mockResolvedValue('diff')
    GetRecentCommitSubjects.mockResolvedValue([])
    RunAiFunction.mockResolvedValue('task-ai-stale')
    CancelAiTask.mockResolvedValue(true)
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 生成')).trigger('click')
    await flushPromises()
    expect(CancelAiTask).not.toHaveBeenCalled()
    // 候选弹窗已开（生成中）
    expect(wrapper.find('.el-dialog').exists()).toBe(true)
    // 切仓库：触发 resetAiState
    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()
    // 在途任务被取消（释放并发槽位与 claude 子进程）
    expect(CancelAiTask).toHaveBeenCalledWith('task-ai-stale')
    // 候选弹窗关闭、loading 复位
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
    // 旧 taskId 的 done 事件不再匹配本组件 → 不渲染候选（防旧仓库结果串入新仓库）
    const doneHandler = aiEventBus.handlers['ai-task:done']
    doneHandler({ taskId: 'task-ai-stale', structuredOutput: { candidates: [{ type: 'feat', description: 'x' }] }, error: '', canceled: false })
    await flushPromises()
    expect(wrapper.findAll('.candidate-item').length).toBe(0)
  })

  it('卸载时取消在途 AI 审查任务并注销监听器', async () => {
    const { GetLocalChanges, GetUncommittedDiffText, RunAiFunction, CancelAiTask } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetUncommittedDiffText.mockResolvedValue('diff')
    RunAiFunction.mockResolvedValue('task-review-unmount')
    CancelAiTask.mockResolvedValue(true)
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 审查')).trigger('click')
    await flushPromises()
    expect(CancelAiTask).not.toHaveBeenCalled()
    wrapper.unmount()
    await flushPromises()
    // 卸载时在途审查任务被取消
    expect(CancelAiTask).toHaveBeenCalledWith('task-review-unmount')
    wrapper = null
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

  // ===== AI 生成提交信息 =====

  it('AI 生成按钮：无暂存文件时禁用', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    // changes 无 staged 字段 → 均视为未暂存 → 按钮禁用
    GetLocalChanges.mockResolvedValue(changes)
    wrapper = await createWrapper()
    const aiBtn = wrapper.findAll('button').find(b => b.text().includes('AI 生成'))
    expect(aiBtn).toBeTruthy()
    expect(aiBtn.attributes('disabled')).toBeDefined()
  })

  it('AI 生成按钮：有暂存文件时可用', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges) // 含 staged: true
    wrapper = await createWrapper()
    const aiBtn = wrapper.findAll('button').find(b => b.text().includes('AI 生成'))
    expect(aiBtn.attributes('disabled')).toBeUndefined()
  })

  it('点击 AI 生成：调 GetStagedDiffText + GetRecentCommitSubjects + RunAiFunction 注入 diff/history', async () => {
    const { GetLocalChanges, GetStagedDiffText, GetRecentCommitSubjects, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetStagedDiffText.mockResolvedValue('=== src/b.go ===\n+const x = 1\n')
    GetRecentCommitSubjects.mockResolvedValue(['feat: a', 'fix: b'])
    RunAiFunction.mockResolvedValue('task-ai-1')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 生成')).trigger('click')
    await flushPromises()
    expect(GetStagedDiffText).toHaveBeenCalledWith('/repo/A')
    expect(GetRecentCommitSubjects).toHaveBeenCalledWith('/repo/A', 3)
    expect(RunAiFunction).toHaveBeenCalledWith('commit-message', {
      diff: '=== src/b.go ===\n+const x = 1\n',
      history: 'feat: a\nfix: b'
    })
  })

  it('done 事件 structuredOutput.candidates 渲染候选 + 点击填入提交框', async () => {
    const { GetLocalChanges, GetStagedDiffText, GetRecentCommitSubjects, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetStagedDiffText.mockResolvedValue('diff')
    GetRecentCommitSubjects.mockResolvedValue([])
    RunAiFunction.mockResolvedValue('task-ai-2')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 生成')).trigger('click')
    await flushPromises()
    // 手动触发 done 事件（taskId 匹配 currentAiTaskId）
    const doneHandler = aiEventBus.handlers['ai-task:done']
    expect(doneHandler).toBeTruthy()
    doneHandler({
      taskId: 'task-ai-2',
      structuredOutput: {
        candidates: [
          { type: 'feat', scope: 'auth', description: '新增登录' },
          { type: 'fix', scope: '', description: '修复空指针' }
        ]
      },
      error: '',
      canceled: false
    })
    await flushPromises()
    const items = wrapper.findAll('.candidate-item')
    expect(items.length).toBe(2)
    // 点击第一个候选填入提交框：feat(auth): 新增登录
    await items[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('textarea').element.value).toBe('feat(auth): 新增登录')
  })

  it('done 事件 structuredOutput 为空时降级提示手输', async () => {
    const { GetLocalChanges, GetStagedDiffText, GetRecentCommitSubjects, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetStagedDiffText.mockResolvedValue('diff')
    GetRecentCommitSubjects.mockResolvedValue([])
    RunAiFunction.mockResolvedValue('task-ai-3')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 生成')).trigger('click')
    await flushPromises()
    const doneHandler = aiEventBus.handlers['ai-task:done']
    doneHandler({ taskId: 'task-ai-3', structuredOutput: null, error: '', canceled: false })
    await flushPromises()
    expect(wrapper.findAll('.candidate-item').length).toBe(0)
    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('AI 生成失败（空暂存 AppError）时 handleGitError 走 warning 并关闭弹窗', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetLocalChanges, GetStagedDiffText } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetStagedDiffText.mockRejectedValue({ code: 'E_GIT_NO_STAGED_CHANGES', message: '无暂存文件，请先 git add 要提交的变更' })
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 生成')).trigger('click')
    await flushPromises()
    // E_GIT_NO_STAGED_CHANGES 在 WARNING_CODES → handleGitError 走 warning
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('无暂存文件'))
    // 弹窗关闭
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
  })

  it('卸载时用 EventsOn 返回闭包注销本组件监听器（禁 EventsOff 清同名全部，避误伤 AiFunctionPanel）', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    const { EventsOn, EventsOff } = await import('../../../wailsjs/runtime/runtime')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    wrapper = await createWrapper()
    // 挂载后已注册 ai-task:done 监听器
    expect(EventsOn).toHaveBeenCalledWith('ai-task:done', expect.any(Function))
    expect(aiEventBus.handlers['ai-task:done']).toBeTruthy()
    EventsOff.mockClear()
    wrapper.unmount()
    // 卸载后本组件监听器被精准移除
    expect(aiEventBus.handlers['ai-task:done']).toBeUndefined()
    // 关键断言：未调 EventsOff（Wails EventsOff 按 eventName 清同名全部监听器，会误删 AiFunctionPanel 的 onDone）
    expect(EventsOff).not.toHaveBeenCalled()
    // 标记已卸载，避免 afterEach 重复 unmount
    wrapper = null
  })

  // ===== AI 代码审查 =====

  it('AI 审查按钮：无本地变动时 footer 不渲染（按钮随 action-bar 隐藏）', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue([])
    wrapper = await createWrapper()
    // changes-footer 在 changes.length===0 时不渲染，AI 审查按钮随之不出现
    expect(wrapper.find('.changes-footer').exists()).toBe(false)
    const btn = wrapper.findAll('button').find(b => b.text().includes('AI 审查'))
    expect(btn).toBeUndefined()
  })

  it('AI 审查按钮：有本地变动时可用', async () => {
    const { GetLocalChanges } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    wrapper = await createWrapper()
    const btn = wrapper.findAll('button').find(b => b.text().includes('AI 审查'))
    expect(btn.attributes('disabled')).toBeUndefined()
  })

  it('点击 AI 审查：调 GetUncommittedDiffText + RunAiFunction 注入 diff', async () => {
    const { GetLocalChanges, GetUncommittedDiffText, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetUncommittedDiffText.mockResolvedValue('=== src/a.go ===\n+const x = 1\n')
    RunAiFunction.mockResolvedValue('task-review-1')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 审查')).trigger('click')
    await flushPromises()
    expect(GetUncommittedDiffText).toHaveBeenCalledWith('/repo/A')
    expect(RunAiFunction).toHaveBeenCalledWith('code-review', { diff: '=== src/a.go ===\n+const x = 1\n' })
  })

  it('done 事件 structuredOutput.issues 渲染问题清单到 CodeReviewResult', async () => {
    const { GetLocalChanges, GetUncommittedDiffText, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetUncommittedDiffText.mockResolvedValue('diff')
    RunAiFunction.mockResolvedValue('task-review-2')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 审查')).trigger('click')
    await flushPromises()
    const doneHandler = aiEventBus.handlers['ai-task:done']
    expect(doneHandler).toBeTruthy()
    doneHandler({
      taskId: 'task-review-2',
      structuredOutput: {
        issues: [
          { file: 'src/a.go', line: 1, severity: 'critical', category: 'bug', description: '空指针', suggestion: '判空' },
          { file: 'src/b.go', line: 2, severity: 'warning', category: 'style', description: '命名', suggestion: '改名' }
        ],
        summary: '2 个问题'
      },
      error: '',
      canceled: false
    })
    await flushPromises()
    const stub = wrapper.findComponent({ name: 'CodeReviewResult' })
    expect(stub.props('modelValue')).toBe(true)
    expect(stub.props('issues').length).toBe(2)
    expect(stub.props('summary')).toBe('2 个问题')
    // loading 关闭
    expect(stub.props('loading')).toBe(false)
  })

  it('done 事件 code-review 与 commit-message taskId 共存不串扰', async () => {
    // 同时发起 commit-message 与 code-review，done 事件按 taskId 路由互不干扰
    const { GetLocalChanges, GetStagedDiffText, GetRecentCommitSubjects, GetUncommittedDiffText, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetStagedDiffText.mockResolvedValue('staged-diff')
    GetRecentCommitSubjects.mockResolvedValue([])
    GetUncommittedDiffText.mockResolvedValue('uncommitted-diff')
    // commit-message 返回 task-cm，code-review 返回 task-cr
    RunAiFunction.mockImplementation((id) => Promise.resolve(id === 'commit-message' ? 'task-cm' : 'task-cr'))
    wrapper = await createWrapper()
    // 先点 AI 生成（commit-message）
    await wrapper.findAll('button').find(b => b.text().includes('AI 生成')).trigger('click')
    await flushPromises()
    // 再点 AI 审查（code-review）
    await wrapper.findAll('button').find(b => b.text().includes('AI 审查')).trigger('click')
    await flushPromises()
    const doneHandler = aiEventBus.handlers['ai-task:done']
    // done commit-message → 候选弹窗渲染候选
    doneHandler({ taskId: 'task-cm', structuredOutput: { candidates: [{ type: 'fix', scope: '', description: '修' }] }, error: '', canceled: false })
    await flushPromises()
    expect(wrapper.findAll('.candidate-item').length).toBe(1)
    // done code-review → 问题清单渲染
    doneHandler({ taskId: 'task-cr', structuredOutput: { issues: [{ file: 'a.go', severity: 'info', category: 'style', description: 'x' }] }, error: '', canceled: false })
    await flushPromises()
    const stub = wrapper.findComponent({ name: 'CodeReviewResult' })
    expect(stub.props('issues').length).toBe(1)
  })

  it('AI 审查失败（无变更 AppError）时 handleGitError 走 warning 并关闭弹窗', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetLocalChanges, GetUncommittedDiffText } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetUncommittedDiffText.mockRejectedValue({ code: 'E_GIT_NO_STAGED_CHANGES', message: '无本地变更可审查' })
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 审查')).trigger('click')
    await flushPromises()
    // E_GIT_NO_STAGED_CHANGES 在 WARNING_CODES → handleGitError 走 warning
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('无本地变更'))
    // CodeReviewResult 弹窗关闭
    const stub = wrapper.findComponent({ name: 'CodeReviewResult' })
    expect(stub.props('modelValue')).toBe(false)
  })

  it('done 事件 error 时弹错误并关闭审查弹窗', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetLocalChanges, GetUncommittedDiffText, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetUncommittedDiffText.mockResolvedValue('diff')
    RunAiFunction.mockResolvedValue('task-review-err')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 审查')).trigger('click')
    await flushPromises()
    const doneHandler = aiEventBus.handlers['ai-task:done']
    doneHandler({ taskId: 'task-review-err', error: '超时', canceled: false, structuredOutput: null })
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('超时'))
    const stub = wrapper.findComponent({ name: 'CodeReviewResult' })
    expect(stub.props('modelValue')).toBe(false)
  })

  it('CodeReviewResult locate-file 打开工作区 FileDiffDialog', async () => {
    const { GetLocalChanges, GetUncommittedDiffText, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    GetLocalChanges.mockResolvedValue(mixedChanges)
    GetUncommittedDiffText.mockResolvedValue('diff')
    RunAiFunction.mockResolvedValue('task-review-loc')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('AI 审查')).trigger('click')
    await flushPromises()
    const doneHandler = aiEventBus.handlers['ai-task:done']
    doneHandler({ taskId: 'task-review-loc', structuredOutput: { issues: [{ file: 'src/a.go', severity: 'info', category: 'style', description: 'x' }] }, error: '', canceled: false })
    await flushPromises()
    const stub = wrapper.findComponent({ name: 'CodeReviewResult' })
    await stub.vm.$emit('locate-file', 'src/a.go')
    await flushPromises()
    // 工作区 FileDiffDialog 打开，file 定位到 src/a.go
    const diffStub = wrapper.findComponent({ name: 'FileDiffDialog' })
    expect(diffStub.exists()).toBe(true)
    expect(diffStub.props('file')).toBe('src/a.go')
  })
})
