import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GitMerge from '../GitMerge.vue'

// 内嵌冲突解决第一期（10-26-conflict-resolve-inapp）：快捷选边 / 内嵌解决入口 / rebase 语义提示
vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetClaudeDefaultModel: vi.fn(() => Promise.resolve('')),
  Merge: vi.fn(),
  Rebase: vi.fn(),
  CherryPick: vi.fn(),
  GetConflictState: vi.fn(() => Promise.resolve({ type: 'none', files: [] })),
  ResolveConflict: vi.fn(() => Promise.resolve()),
  ChooseConflictSide: vi.fn(() => Promise.resolve()),
  GetConflictFileContent: vi.fn(() => Promise.resolve({
    content: '<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> b',
    encoding: 'UTF-8',
    binary: false,
    tooLarge: false
  })),
  SaveConflictResolution: vi.fn(() => Promise.resolve()),
  ContinueMerge: vi.fn(),
  ContinueRebase: vi.fn(),
  ContinueCherryPick: vi.fn(),
  AbortMerge: vi.fn(),
  AbortRebase: vi.fn(),
  AbortCherryPick: vi.fn(),
  SkipRebase: vi.fn(),
  OpenInVSCode: vi.fn(() => Promise.resolve(true)),
  OpenInExternalMerge: vi.fn(() => Promise.resolve()),
  CancelGitOperation: vi.fn(() => Promise.resolve(true))
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' }
}))

const stubs = {
  'el-card': { template: '<div class="el-card"><slot name="header" /><slot /></div>' },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['icon', 'loading', 'size', 'type', 'circle', 'disabled', 'plain', 'text'],
    emits: ['click']
  },
  'el-text': { template: '<span v-bind="$attrs"><slot /></span>', props: ['type', 'size'] },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
  'el-empty': { template: '<div class="el-empty">{{ description }}</div>', props: ['description'] },
  'el-divider': { template: '<hr />' },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div><span>{{ label }}</span><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<textarea v-if="type === \'textarea\'" :value="modelValue" :rows="rows" @input="$emit(\'update:modelValue\', $event.target.value)" /><input v-else :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'type', 'rows', 'disabled'],
    emits: ['update:modelValue']
  },
  'el-radio-group': {
    template: '<div><slot /></div>',
    props: ['modelValue'],
    emits: ['update:modelValue'],
    provide() { return { elRadioGroup: this } }
  },
  'el-radio': {
    template: '<label><input type="radio" :value="value" :checked="isChecked" @change="onChange" /><span><slot /></span></label>',
    props: ['value'],
    inject: { elRadioGroup: { default: null } },
    computed: {
      isChecked() { return this.elRadioGroup && this.elRadioGroup.modelValue === this.value }
    },
    methods: {
      onChange() {
        if (this.elRadioGroup) this.elRadioGroup.$emit('update:modelValue', this.value)
      }
    }
  },
  'el-select': { template: '<select :value="modelValue"><slot /></select>', props: ['modelValue'] },
  'el-option': { template: '<option :value="value">{{ label }}</option>', props: ['label', 'value'] },
  // rebase 选边按钮的语义反转提示
  'el-tooltip': { template: '<span class="el-tooltip" :title="content"><slot /></span>', props: ['content', 'placement'] },
  // ---- ConflictResolverDialog 内嵌挂载所需 stub ----
  'el-dialog': {
    template: '<div v-if="modelValue" class="el-dialog"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'top', 'appendToBody'],
    emits: ['update:modelValue']
  },
  'el-alert': { template: '<div class="el-alert">{{ title }}</div>', props: ['type', 'title', 'closable', 'showIcon'] },
  'el-collapse': { template: '<div class="el-collapse"><slot /></div>' },
  'el-collapse-item': { template: '<div class="el-collapse-item"><slot /></div>', props: ['title', 'name'] }
}

function createWrapper(props = {}) {
  return mount(GitMerge, {
    props: { repoPath: '/repo/A', ...props },
    global: { stubs }
  })
}

const findBtn = (wrapper, text) =>
  wrapper.findAll('button').find(b => b.text() === text)

async function mountWithConflict(type = 'merge') {
  const { GetConflictState } = await import('../../../wailsjs/go/main/App')
  GetConflictState.mockResolvedValue({ type, files: ['src/a.txt'] })
  const wrapper = createWrapper()
  await flushPromises()
  GetConflictState.mockClear()
  return wrapper
}

describe('GitMerge.vue - 内嵌冲突解决（R1/R2）', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
  })

  it('「以当前为准」调用 ChooseConflictSide(path, file, ours) 并刷新冲突态', async () => {
    const { ChooseConflictSide, GetConflictState } = await import('../../../wailsjs/go/main/App')
    wrapper = await mountWithConflict('merge')

    await findBtn(wrapper, '以当前为准').trigger('click')
    await flushPromises()

    expect(ChooseConflictSide).toHaveBeenCalledWith('/repo/A', 'src/a.txt', 'ours')
    expect(GetConflictState).toHaveBeenCalledWith('/repo/A')
  })

  it('「以传入为准」调用 ChooseConflictSide(path, file, theirs)', async () => {
    const { ChooseConflictSide } = await import('../../../wailsjs/go/main/App')
    wrapper = await mountWithConflict('merge')

    await findBtn(wrapper, '以传入为准').trigger('click')
    await flushPromises()

    expect(ChooseConflictSide).toHaveBeenCalledWith('/repo/A', 'src/a.txt', 'theirs')
  })

  it('选边成功后提示已解决', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = await mountWithConflict('merge')

    await findBtn(wrapper, '以当前为准').trigger('click')
    await flushPromises()

    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('已以当前为准解决'))
  })

  it('选边失败走 handleError 分流提示', async () => {
    const { ChooseConflictSide } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    wrapper = await mountWithConflict('merge')
    ChooseConflictSide.mockRejectedValueOnce(new Error('E_GIT_LOCKED: 操作冲突'))

    await findBtn(wrapper, '以当前为准').trigger('click')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('操作冲突'))
  })

  it('「内嵌解决」打开解决器弹窗并加载冲突文件内容', async () => {
    const { GetConflictFileContent } = await import('../../../wailsjs/go/main/App')
    wrapper = await mountWithConflict('merge')

    await findBtn(wrapper, '内嵌解决').trigger('click')
    await flushPromises()

    expect(GetConflictFileContent).toHaveBeenCalledWith('/repo/A', 'src/a.txt')
    // 弹窗渲染冲突块卡片
    expect(wrapper.find('.el-dialog').exists()).toBe(true)
    expect(wrapper.findAll('.conflict-block')).toHaveLength(1)
  })

  it('解决器保存成功后（resolved 事件）刷新冲突态', async () => {
    const { GetConflictState, SaveConflictResolution } = await import('../../../wailsjs/go/main/App')
    wrapper = await mountWithConflict('merge')
    GetConflictState.mockClear()

    await findBtn(wrapper, '内嵌解决').trigger('click')
    await flushPromises()

    // 块决策「用当前」后保存
    const useOurs = wrapper.findAll('button').find(b => b.text() === '用当前')
    await useOurs.trigger('click')
    await wrapper.findAll('button').find(b => b.text().includes('保存并标记已解决')).trigger('click')
    await flushPromises()

    expect(SaveConflictResolution).toHaveBeenCalledWith('/repo/A', 'src/a.txt', 'ours', 'UTF-8')
    // resolved 事件链：GitMerge 收到后刷新冲突态
    expect(GetConflictState).toHaveBeenCalledWith('/repo/A')
  })

  it('既有入口保留（R9）：外部合并 / 标记已解决按钮仍在', async () => {
    wrapper = await mountWithConflict('merge')

    expect(findBtn(wrapper, '外部合并')).toBeDefined()
    expect(findBtn(wrapper, '标记已解决')).toBeDefined()
    expect(findBtn(wrapper, '打开')).toBeDefined()
  })

  it('rebase 冲突：以当前为准带语义反转提示，merge 冲突不带', async () => {
    wrapper = await mountWithConflict('merge')
    expect(wrapper.find('.el-tooltip').exists()).toBe(false)
    wrapper.unmount()

    wrapper = await mountWithConflict('rebase')
    const tip = wrapper.find('.el-tooltip')
    expect(tip.exists()).toBe(true)
    expect(tip.attributes('title')).toContain('变基目标侧')
  })
})
