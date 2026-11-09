import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
// 静态 import：coverage 插桩下动态 import 首次编译会顶到 5s 超时线
import TempTodoPanel from '../TempTodoPanel.vue'

// mock 句柄用 vi.hoisted 提升：静态 import 的组件先于普通 const 求值，
// 顶层变量直接进 vi.mock 工厂会触发 TDZ（Cannot access before initialization）
const mocks = vi.hoisted(() => ({
  message: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  confirm: vi.fn(() => Promise.resolve()),
  list: vi.fn(),
  add: vi.fn(),
  addFromClipboard: vi.fn(),
  update: vi.fn(),
  setCompleted: vi.fn(),
  remove: vi.fn()
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: mocks.message,
    ElMessageBox: { confirm: (...args) => mocks.confirm(...args) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  ListTempTodos: (...a) => mocks.list(...a),
  AddTempTodo: (...a) => mocks.add(...a),
  AddTempTodoFromClipboard: (...a) => mocks.addFromClipboard(...a),
  UpdateTempTodo: (...a) => mocks.update(...a),
  SetTempTodoCompleted: (...a) => mocks.setCompleted(...a),
  DeleteTempTodo: (...a) => mocks.remove(...a)
}))

vi.mock('@element-plus/icons-vue', () => {
  const stub = (name) => ({ name, template: `<span class="i-${name}" />` })
  return {
    Memo: stub('memo'),
    Plus: stub('plus'),
    CopyDocument: stub('copy'),
    Edit: stub('edit'),
    Delete: stub('delete'),
    ArrowDown: stub('arrow-down'),
    ArrowRight: stub('arrow-right')
  }
})

const stubs = {
  'el-icon': { template: '<i><slot /></i>' },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'loading', 'size', 'type', 'link'],
    emits: ['click']
  },
  // 同时支撑 textarea 与普通 input：type=textarea 时渲染 textarea，便于验证多行正文
  'el-input': {
    template: `<textarea v-if="type === 'textarea'" :value="modelValue" :placeholder="placeholder" @input="$emit('update:modelValue', $event.target.value)" @keydown="$emit('keydown', $event)" />
      <input v-else :value="modelValue" :placeholder="placeholder" @input="$emit('update:modelValue', $event.target.value)" @keydown="$emit('keydown', $event)" />`,
    props: ['modelValue', 'placeholder', 'type', 'rows', 'clearable', 'size', 'resize'],
    emits: ['update:modelValue', 'keydown']
  },
  'el-checkbox': {
    template: '<label class="el-checkbox"><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'change\', $event.target.checked)" /></label>',
    props: ['modelValue', 'disabled', 'size'],
    emits: ['change']
  },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['size', 'type'] },
  'el-tooltip': { template: '<div class="el-tooltip"><slot /></div>', props: ['content', 'placement', 'showAfter'] },
  // 真实 ElTooltip 默认 teleport 到 body，按钮会脱离 wrapper，必须显式 stub 掉
  ElTooltip: { template: '<div class="el-tooltip"><slot /></div>' }
}

const now = Date.now()
const sampleTodos = () => [
  {
    id: 'todo-1',
    text: '第一条待办\n第二行正文',
    tags: ['重构'],
    createdAt: now - 3600 * 1000,
    completedAt: null
  },
  {
    id: 'done-1',
    text: '已完成的事',
    tags: [],
    createdAt: now - 7200 * 1000,
    completedAt: now - 60 * 1000
  }
]

const mountPanel = async () => {
  const wrapper = mount(TempTodoPanel, { global: { stubs } })
  await flushPromises()
  return wrapper
}

const openCompleted = async (wrapper) => {
  await wrapper.find('.todo-section-title--toggle').trigger('click')
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  mocks.list.mockResolvedValue(sampleTodos())
  mocks.add.mockResolvedValue({ id: 'new', text: '新建', tags: [], createdAt: now, completedAt: null })
  mocks.addFromClipboard.mockResolvedValue({
    id: 'clip', text: '剪贴板里的一段提示词内容超过了二十个字符的截断阈值', tags: [], createdAt: now, completedAt: null
  })
  mocks.update.mockResolvedValue()
  mocks.setCompleted.mockResolvedValue()
  mocks.remove.mockResolvedValue()
  mocks.confirm.mockResolvedValue()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('TempTodoPanel - 列表渲染与分区', () => {
  it('挂载时加载列表，待办区只显示未完成条目', async () => {
    const wrapper = await mountPanel()

    expect(mocks.list).toHaveBeenCalledTimes(1)
    const pendingItems = wrapper.findAll('.todo-item:not(.todo-item--done)')
    expect(pendingItems).toHaveLength(1)
    expect(pendingItems[0].text()).toContain('第一条待办')
    expect(pendingItems[0].find('.el-tag').text()).toBe('重构')
    expect(wrapper.find('.todo-item--done').exists()).toBe(false)
  })

  it('已完成区默认折叠，点击标题后显示并标注完成时间', async () => {
    const wrapper = await mountPanel()
    expect(wrapper.find('.todo-item--done').exists()).toBe(false)

    await openCompleted(wrapper)
    const done = wrapper.find('.todo-item--done')
    expect(done.exists()).toBe(true)
    expect(done.text()).toContain('已完成的事')
    expect(done.text()).toContain('完成于')
  })

  it('搜索正文或标签命中，未命中显示空状态', async () => {
    const wrapper = await mountPanel()
    const inputs = wrapper.findAll('input')
    const search = inputs[0]

    await search.setValue('第二行')
    expect(wrapper.findAll('.todo-item')).toHaveLength(1)

    await search.setValue('重构')
    expect(wrapper.findAll('.todo-item')).toHaveLength(1)

    await search.setValue('不存在的关键字')
    expect(wrapper.findAll('.todo-item')).toHaveLength(0)
    expect(wrapper.text()).toContain('没有匹配')
  })

  it('条目长文默认折叠，点击正文切换展开态 class', async () => {
    const wrapper = await mountPanel()
    const text = wrapper.find('.todo-item .todo-text')
    expect(text.classes()).not.toContain('is-expanded')

    await text.trigger('click')
    expect(wrapper.find('.todo-item .todo-text').classes()).toContain('is-expanded')
  })
})

describe('TempTodoPanel - 完成流转与删除', () => {
  it('勾选待办调用 SetTempTodoCompleted(id, true) 并刷新', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('.todo-item .el-checkbox input').trigger('change')

    expect(mocks.setCompleted).toHaveBeenCalledWith('todo-1', true)
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it('已完成条目取消勾选回到待办区', async () => {
    const wrapper = await mountPanel()
    await openCompleted(wrapper)
    await wrapper.find('.todo-item--done .el-checkbox input').trigger('change')

    expect(mocks.setCompleted).toHaveBeenCalledWith('done-1', false)
  })

  it('删除待办需二次确认，确认后调用 DeleteTempTodo', async () => {
    const wrapper = await mountPanel()
    const deleteBtn = wrapper.find('.todo-item [data-action="delete"]')
    await deleteBtn.trigger('click')
    await flushPromises()

    expect(mocks.confirm).toHaveBeenCalledTimes(1)
    expect(mocks.remove).toHaveBeenCalledWith('todo-1')
  })

  it('删除待办时取消确认不发起删除', async () => {
    mocks.confirm.mockRejectedValueOnce(new Error('cancel'))
    const wrapper = await mountPanel()
    const deleteBtn = wrapper.find('.todo-item [data-action="delete"]')
    await deleteBtn.trigger('click')
    await flushPromises()

    expect(mocks.remove).not.toHaveBeenCalled()
  })

  it('已完成条目的删除不进二次确认，直接删除', async () => {
    const wrapper = await mountPanel()
    await openCompleted(wrapper)
    const deleteBtn = wrapper.find('.todo-item--done [data-action="delete"]')
    await deleteBtn.trigger('click')
    await flushPromises()

    expect(mocks.confirm).not.toHaveBeenCalled()
    expect(mocks.remove).toHaveBeenCalledWith('done-1')
  })

  it('复制正文写入系统剪贴板并提示', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    const origin = navigator.clipboard
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    const wrapper = await mountPanel()
    await wrapper.find('.todo-item [data-action="copy"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('第一条待办\n第二行正文')
    expect(mocks.message.success).toHaveBeenCalledWith('已复制到剪贴板')

    Object.defineProperty(navigator, 'clipboard', { value: origin, configurable: true })
  })
})

describe('TempTodoPanel - 新建 / 编辑 / 快捷键记录', () => {
  it('新建保存调用 AddTempTodo 并带上解析后的标签', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('.panel-actions [data-action="create"]').trigger('click')

    const editor = wrapper.find('.todo-editor')
    const textarea = editor.find('textarea')
    await textarea.setValue('新的一条待办')
    const tagInput = editor.find('input')
    await tagInput.setValue('甲, 乙，丙')

    const saveBtn = editor.findAll('button').find(b => b.text().includes('保存'))
    await saveBtn.trigger('click')
    await flushPromises()

    expect(mocks.add).toHaveBeenCalledWith('新的一条待办', ['甲', '乙', '丙'])
  })

  it('新建空白正文拒绝保存并警告', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('.panel-actions [data-action="create"]').trigger('click')
    const editor = wrapper.find('.todo-editor')
    const saveBtn = editor.findAll('button').find(b => b.text().includes('保存'))
    await saveBtn.trigger('click')
    await flushPromises()

    expect(mocks.add).not.toHaveBeenCalled()
    expect(mocks.message.warning).toHaveBeenCalledWith('待办内容不能为空')
  })

  it('编辑已有条目调用 UpdateTempTodo 并保留原标签输入', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('.todo-item [data-action="edit"]').trigger('click')

    const textarea = wrapper.find('.todo-item textarea')
    expect(textarea.element.value).toContain('第一条待办')
    await textarea.setValue('改后的正文')
    const saveBtn = wrapper.findAll('.todo-item button').find(b => b.text().includes('保存'))
    await saveBtn.trigger('click')
    await flushPromises()

    expect(mocks.update).toHaveBeenCalledWith('todo-1', '改后的正文', ['重构'])
  })

  it('快捷键入口记录成功并提示正文预览（截断 20 字）', async () => {
    const wrapper = await mountPanel()
    await wrapper.vm.quickAddFromClipboard()
    await flushPromises()

    expect(mocks.addFromClipboard).toHaveBeenCalledTimes(1)
    expect(mocks.list).toHaveBeenCalledTimes(2)
    const tip = mocks.message.success.mock.calls[0][0]
    expect(tip).toContain('已记录：')
    expect(tip).toContain('…')
  })

  it('快捷键入口失败时按错误码分流提示而非抛出', async () => {
    mocks.addFromClipboard.mockRejectedValueOnce({ code: 'E_TEMP_TODO_CLIPBOARD_EMPTY', message: '剪贴板为空，未记录' })
    const wrapper = await mountPanel()
    await wrapper.vm.quickAddFromClipboard()
    await flushPromises()

    expect(mocks.message.warning).toHaveBeenCalledWith('剪贴板为空，未记录')
  })
})
