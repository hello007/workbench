import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import GitTags from '../GitTags.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetTags: vi.fn(),
  CreateTag: vi.fn(),
  DeleteTag: vi.fn(),
  PushTag: vi.fn()
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
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
  'el-empty': { template: '<div class="el-empty" />', props: ['description'] },
  'el-dialog': {
    template: '<div v-if="modelValue"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue']
  },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div class="form-item"><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<template v-if="type === \'textarea\'"><textarea :value="modelValue" :rows="rows" @input="$emit(\'update:modelValue\', $event.target.value)" /></template><template v-else><input :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" /></template>',
    props: ['modelValue', 'placeholder', 'type', 'rows'],
    emits: ['update:modelValue']
  }
}
const directives = { loading: () => {} }

const tag = (over = {}) => ({
  name: 'v1.0',
  type: 'lightweight',
  sha: 'abcdef1234567890abcdef1234567890abcdef12',
  shortSha: 'abcdef12',
  message: '',
  tagger: '',
  date: '',
  ...over
})

function createWrapper(props = {}) {
  return mount(GitTags, {
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

describe('GitTags.vue', () => {
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

  it('加载时调用 GetTags 并展示标签列表', async () => {
    const { GetTags } = await import('../../../wailsjs/go/main/App')
    GetTags.mockResolvedValue([
      tag(),
      tag({ name: 'v1.1', type: 'annotated', message: 'release', tagger: 'test', date: '2 天前' })
    ])

    wrapper = createWrapper()
    await flushPromises()

    expect(GetTags).toHaveBeenCalledWith('/repo/A')
    expect(wrapper.findAll('.tag-row')).toHaveLength(2)
    expect(wrapper.text()).toContain('v1.0')
    expect(wrapper.text()).toContain('v1.1')
    expect(wrapper.text()).toContain('注释')
    expect(wrapper.text()).toContain('轻量')
  })

  it('空标签列表渲染 empty 占位', async () => {
    const { GetTags } = await import('../../../wailsjs/go/main/App')
    GetTags.mockResolvedValue([])

    wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('GetTags 失败时调用 ElMessage.error', async () => {
    const { GetTags } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    GetTags.mockRejectedValue(new Error('boom'))

    wrapper = createWrapper()
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalled()
  })

  it('新建标签填写 name+message 后调用 CreateTag 并刷新', async () => {
    const { GetTags, CreateTag } = await import('../../../wailsjs/go/main/App')
    GetTags.mockResolvedValue([])

    wrapper = createWrapper()
    await flushPromises()
    GetTags.mockClear()

    // 打开对话框
    await findBtn(wrapper, '新建标签').trigger('click')
    await flushPromises()

    // 填写表单：第一个 input 为标签名，textarea 为注释消息
    const inputs = wrapper.findAll('input, textarea')
    expect(inputs.length).toBeGreaterThanOrEqual(2)
    await inputs[0].setValue('v1.0.0')
    await inputs[1].setValue('release note')

    // 点击"创建"
    await findBtn(wrapper, '创建', true).trigger('click')
    await flushPromises()

    expect(CreateTag).toHaveBeenCalledWith('/repo/A', 'v1.0.0', 'release note')
    expect(GetTags).toHaveBeenCalled()
  })

  it('删除标签确认后调用 DeleteTag 并刷新', async () => {
    const { GetTags, DeleteTag } = await import('../../../wailsjs/go/main/App')
    GetTags.mockResolvedValue([tag({ name: 'v1.0' })])

    wrapper = createWrapper()
    await flushPromises()
    GetTags.mockClear()

    await findBtn(wrapper, '删除', true).trigger('click')
    await flushPromises()

    expect(DeleteTag).toHaveBeenCalledWith('/repo/A', 'v1.0')
    expect(GetTags).toHaveBeenCalled()
  })

  it('推送单个标签调用 PushTag', async () => {
    const { GetTags, PushTag } = await import('../../../wailsjs/go/main/App')
    GetTags.mockResolvedValue([tag({ name: 'v1.0' })])
    PushTag.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '推送', true).trigger('click')
    await flushPromises()

    expect(PushTag).toHaveBeenCalledWith('/repo/A', 'v1.0')
  })

  it('推送全部标签对每个标签调用 PushTag', async () => {
    const { GetTags, PushTag } = await import('../../../wailsjs/go/main/App')
    GetTags.mockResolvedValue([tag({ name: 'v1.0' }), tag({ name: 'v1.1' })])
    PushTag.mockResolvedValue('')

    wrapper = createWrapper()
    await flushPromises()

    await findBtn(wrapper, '推送全部').trigger('click')
    await flushPromises()

    expect(PushTag).toHaveBeenCalledTimes(2)
    expect(PushTag).toHaveBeenCalledWith('/repo/A', 'v1.0')
    expect(PushTag).toHaveBeenCalledWith('/repo/A', 'v1.1')
  })

  it('repoPath 变化时重新加载', async () => {
    const { GetTags } = await import('../../../wailsjs/go/main/App')
    GetTags.mockResolvedValue([])

    wrapper = createWrapper()
    await flushPromises()
    GetTags.mockClear()

    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()

    expect(GetTags).toHaveBeenCalledWith('/repo/B')
  })
})
