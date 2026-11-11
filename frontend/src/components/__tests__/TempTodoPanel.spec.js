import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
// 静态 import：coverage 插桩下动态 import 首次编译会顶到 5s 超时线
import TempTodoPanel from '../TempTodoPanel.vue'
import { useUiStore } from '../../store/ui'

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
  remove: vi.fn(),
  exportMd: vi.fn(),
  openDir: vi.fn(),
  addMany: vi.fn(),
  openFile: vi.fn(),
  readFileBytes: vi.fn(),
  browserChannel: false
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
  DeleteTempTodo: (...a) => mocks.remove(...a),
  ExportTempTodosMarkdown: (...a) => mocks.exportMd(...a),
  OpenDirectoryDialog: (...a) => mocks.openDir(...a),
  AddTempTodos: (...a) => mocks.addMany(...a),
  OpenFileDialog: (...a) => mocks.openFile(...a),
  ReadFileBytes: (...a) => mocks.readFileBytes(...a)
}))

// 浏览器/桌面模式判定：用例内翻转 mocks.browserChannel 驱动降级分支
vi.mock('../../transport', () => ({
  isBrowserChannel: () => mocks.browserChannel
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
    ArrowRight: stub('arrow-right'),
    Download: stub('download'),
    Upload: stub('upload'),
    ChatDotRound: stub('chat-dot-round'),
    FullScreen: stub('full-screen')
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
  // 导入预览对话框：真实 EP dialog teleport 到 body 且带过渡，stub 成受 modelValue 控制的块
  'el-dialog': {
    template: `<div v-if="modelValue" class="el-dialog">
      <div class="dialog-title">{{ title }}</div>
      <slot />
      <slot name="footer" />
    </div>`,
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue', 'closed'],
    watch: {
      // 真实 EP 在关闭动画结束后触发 closed；stub 以 modelValue 置 false 模拟
      modelValue(value) {
        if (!value) this.$emit('closed')
      }
    }
  },
  'el-switch': {
    template: '<label class="el-switch"><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" /></label>',
    props: ['modelValue', 'size'],
    emits: ['update:modelValue']
  },
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
  mocks.exportMd.mockResolvedValue('')
  mocks.openDir.mockResolvedValue('')
  mocks.addMany.mockResolvedValue(1)
  mocks.openFile.mockResolvedValue('')
  mocks.readFileBytes.mockResolvedValue({ path: '', name: '', size: 0, tooLarge: false })
  mocks.browserChannel = false
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

describe('TempTodoPanel - 二期入口（导出 / 转提示词 / 外部变更刷新）', () => {
  it('导出：选目录后调用后端并提示写入路径', async () => {
    mocks.openDir.mockResolvedValue('D:/notes')
    mocks.exportMd.mockResolvedValue('D:/notes/2026-11-10.md')
    const wrapper = await mountPanel()
    await wrapper.find('[data-action="export"]').trigger('click')
    await flushPromises()

    expect(mocks.openDir).toHaveBeenCalledWith('选择待办导出目录')
    expect(mocks.exportMd).toHaveBeenCalledWith('D:/notes')
    expect(mocks.message.success).toHaveBeenCalledWith('已导出到 D:/notes/2026-11-10.md')
  })

  it('导出：取消目录选择不调用后端', async () => {
    mocks.openDir.mockResolvedValue('')
    const wrapper = await mountPanel()
    await wrapper.find('[data-action="export"]').trigger('click')
    await flushPromises()

    expect(mocks.exportMd).not.toHaveBeenCalled()
    expect(mocks.message.success).not.toHaveBeenCalled()
  })

  it('导出：浏览器模式提示仅桌面可用且不弹目录对话框', async () => {
    mocks.browserChannel = true
    const wrapper = await mountPanel()
    await wrapper.find('[data-action="export"]').trigger('click')
    await flushPromises()

    expect(mocks.message.warning).toHaveBeenCalledWith('该功能仅桌面模式可用')
    expect(mocks.openDir).not.toHaveBeenCalled()
  })

  it('导出：后端返回空路径提示暂无可导出条目', async () => {
    mocks.openDir.mockResolvedValue('D:/notes')
    mocks.exportMd.mockResolvedValue('')
    const wrapper = await mountPanel()
    await wrapper.find('[data-action="export"]').trigger('click')
    await flushPromises()

    expect(mocks.message.info).toHaveBeenCalledWith('暂无可导出的待办')
  })

  it('无条目时导出按钮禁用', async () => {
    mocks.list.mockResolvedValue([])
    const wrapper = await mountPanel()

    expect(wrapper.find('[data-action="export"]').attributes('disabled')).toBeDefined()
  })

  it('转提示词：写入 uiStore 一次性信箱（正文原样）', async () => {
    const wrapper = await mountPanel()
    const uiStore = useUiStore()

    await wrapper.find('.todo-item [data-action="to-prompt"]').trigger('click')

    expect(uiStore.todoPromptRequest).toMatchObject({ text: '第一条待办\n第二行正文' })
    expect(uiStore.todoPromptRequest.seq).toBeGreaterThan(0)
  })

  it('外部变更通知触发重拉列表', async () => {
    await mountPanel()
    expect(mocks.list).toHaveBeenCalledTimes(1)

    useUiStore().notifyTempTodoChanged()
    await flushPromises()

    expect(mocks.list).toHaveBeenCalledTimes(2)
  })
})

describe('TempTodoPanel - 三期导入 Markdown', () => {
  const mdBase64 = (text) => Buffer.from(text, 'utf-8').toString('base64')

  const openPreview = async (wrapper, md) => {
    mocks.openFile.mockResolvedValue('D:/notes/2026-01-05.md')
    mocks.readFileBytes.mockResolvedValue({ base64: mdBase64(md), tooLarge: false })
    await wrapper.find('[data-action="import"]').trigger('click')
    await flushPromises()
  }

  it('选择文件后解析并打开预览，汇总将导入与重复条数', async () => {
    const wrapper = await mountPanel()
    await openPreview(wrapper, [
      '- [ ] 已完成的事',
      '- 09:30 新条目  #标签',
      '- 普通段落'
    ].join('\n'))

    expect(mocks.openFile).toHaveBeenCalledTimes(1)
    expect(mocks.readFileBytes).toHaveBeenCalledWith('D:/notes/2026-01-05.md')
    const dialog = wrapper.find('.el-dialog')
    expect(dialog.exists()).toBe(true)
    expect(dialog.text()).toContain('将导入 1 条')
    expect(dialog.text()).toContain('跳过重复 1 条')
    expect(dialog.text()).toContain('新条目')
    expect(dialog.text()).toContain('已存在')
  })

  it('确认导入调用 AddTempTodos，默认过滤重复项并刷新列表', async () => {
    const wrapper = await mountPanel()
    await openPreview(wrapper, '- [ ] 已完成的事\n- 09:30 新条目  #标签')

    await wrapper.find('[data-action="import-confirm"]').trigger('click')
    await flushPromises()

    expect(mocks.addMany).toHaveBeenCalledWith([
      {
        text: '新条目',
        tags: ['标签'],
        createdAt: new Date(2026, 0, 5, 9, 30).getTime()
      }
    ])
    expect(mocks.message.success).toHaveBeenCalledWith('已导入 1 条待办')
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it('打开「重复项也导入」后连同重复项一起写入', async () => {
    const wrapper = await mountPanel()
    await openPreview(wrapper, '- [ ] 已完成的事\n- 09:30 新条目  #标签')

    await wrapper.find('.el-switch input').setValue(true)
    await flushPromises()
    expect(wrapper.find('.el-dialog').text()).toContain('将导入 2 条')

    await wrapper.find('[data-action="import-confirm"]').trigger('click')
    await flushPromises()

    const items = mocks.addMany.mock.calls[0][0]
    expect(items.map(item => item.text)).toEqual(['已完成的事', '新条目'])
  })

  it('浏览器模式提示仅桌面可用且不弹文件对话框', async () => {
    mocks.browserChannel = true
    const wrapper = await mountPanel()
    await wrapper.find('[data-action="import"]').trigger('click')
    await flushPromises()

    expect(mocks.message.warning).toHaveBeenCalledWith('该功能仅桌面模式可用')
    expect(mocks.openFile).not.toHaveBeenCalled()
  })

  it('文件中没有待办行时提示且不打开预览', async () => {
    const wrapper = await mountPanel()
    await openPreview(wrapper, '# 只有标题\n\n普通段落\n- [x] 已完成')

    expect(mocks.message.info).toHaveBeenCalledWith('未找到可导入的待办行')
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
  })

  it('全部重复时确认按钮禁用', async () => {
    const wrapper = await mountPanel()
    await openPreview(wrapper, '- [ ] 已完成的事')

    expect(wrapper.find('[data-action="import-confirm"]').attributes('disabled')).toBeDefined()
  })

  it('取消文件选择不读取文件', async () => {
    mocks.openFile.mockResolvedValue('')
    const wrapper = await mountPanel()
    await wrapper.find('[data-action="import"]').trigger('click')
    await flushPromises()

    expect(mocks.readFileBytes).not.toHaveBeenCalled()
  })
})

describe('TempTodoPanel - 全屏编辑弹窗', () => {
  const openInlineDialog = async (wrapper) => {
    await wrapper.find('.todo-item [data-action="edit"]').trigger('click')
    await wrapper.find('.todo-item [data-action="expand-editor"]').trigger('click')
  }

  it('行内编辑打开全屏弹窗，正文与标签沿用当前草稿', async () => {
    const wrapper = await mountPanel()
    await openInlineDialog(wrapper)

    const dialog = wrapper.find('.el-dialog')
    expect(dialog.exists()).toBe(true)
    expect(dialog.text()).toContain('编辑待办')
    expect(dialog.find('textarea').element.value).toContain('第一条待办')
    expect(dialog.find('input').element.value).toBe('重构')
  })

  it('弹窗内保存调用 UpdateTempTodo，并关闭弹窗、退出编辑态', async () => {
    const wrapper = await mountPanel()
    await openInlineDialog(wrapper)

    const dialog = wrapper.find('.el-dialog')
    await dialog.find('textarea').setValue('改后的正文')
    await dialog.find('input').setValue('甲, 乙')
    await wrapper.find('[data-action="editor-save"]').trigger('click')
    await flushPromises()

    expect(mocks.update).toHaveBeenCalledWith('todo-1', '改后的正文', ['甲', '乙'])
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
    expect(wrapper.find('.todo-item textarea').exists()).toBe(false)
  })

  it('弹窗内 Ctrl+Enter 保存同一份草稿', async () => {
    const wrapper = await mountPanel()
    await openInlineDialog(wrapper)

    const textarea = wrapper.find('.el-dialog textarea')
    await textarea.setValue('快捷键保存的正文')
    await textarea.trigger('keydown', { key: 'Enter', ctrlKey: true })
    await flushPromises()

    expect(mocks.update).toHaveBeenCalledWith('todo-1', '快捷键保存的正文', ['重构'])
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
    expect(wrapper.find('.todo-item textarea').exists()).toBe(false)
  })

  it('新建卡片可打开全屏弹窗，保存调用 AddTempTodo', async () => {
    const wrapper = await mountPanel()
    await wrapper.find('.panel-actions [data-action="create"]').trigger('click')
    await wrapper.find('.todo-editor [data-action="expand-editor"]').trigger('click')

    const dialog = wrapper.find('.el-dialog')
    expect(dialog.text()).toContain('新建待办')
    await dialog.find('textarea').setValue('新待办')
    await wrapper.find('[data-action="editor-save"]').trigger('click')
    await flushPromises()

    expect(mocks.add).toHaveBeenCalledWith('新待办', [])
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
  })

  it('关闭弹窗只回到原编辑态，未保存的草稿保留', async () => {
    const wrapper = await mountPanel()
    await openInlineDialog(wrapper)

    const dialog = wrapper.find('.el-dialog')
    await dialog.find('textarea').setValue('暂未保存的修改')
    await wrapper.find('[data-action="editor-back"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('.el-dialog').exists()).toBe(false)
    expect(wrapper.find('.todo-item textarea').element.value).toBe('暂未保存的修改')
    expect(mocks.update).not.toHaveBeenCalled()
  })

  it('正文为空时提示且不关闭弹窗', async () => {
    const wrapper = await mountPanel()
    await openInlineDialog(wrapper)

    const dialog = wrapper.find('.el-dialog')
    await dialog.find('textarea').setValue('   ')
    await wrapper.find('[data-action="editor-save"]').trigger('click')
    await flushPromises()

    expect(mocks.message.warning).toHaveBeenCalledWith('待办内容不能为空')
    expect(wrapper.find('.el-dialog').exists()).toBe(true)
    expect(mocks.update).not.toHaveBeenCalled()
  })

  it('保存失败时弹窗保持打开且内容不丢', async () => {
    mocks.update.mockRejectedValueOnce(new Error('boom'))
    const wrapper = await mountPanel()
    await openInlineDialog(wrapper)

    const dialog = wrapper.find('.el-dialog')
    await dialog.find('textarea').setValue('会失败的内容')
    await wrapper.find('[data-action="editor-save"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('.el-dialog').exists()).toBe(true)
    expect(wrapper.find('.el-dialog textarea').element.value).toBe('会失败的内容')
    expect(wrapper.find('.todo-item textarea').element.value).toBe('会失败的内容')
  })

  it('保存请求在途时关闭弹窗，完成后仍退出编辑态不残留草稿', async () => {
    let resolveUpdate
    mocks.update.mockImplementationOnce(() => new Promise((resolve) => { resolveUpdate = resolve }))
    const wrapper = await mountPanel()
    await openInlineDialog(wrapper)

    await wrapper.find('.el-dialog textarea').setValue('慢保存的正文')
    await wrapper.find('[data-action="editor-save"]').trigger('click')
    // 保存未完成时用户按 ESC / X 关闭弹窗
    await wrapper.find('[data-action="editor-back"]').trigger('click')
    expect(wrapper.find('.el-dialog').exists()).toBe(false)

    resolveUpdate()
    await flushPromises()

    expect(mocks.update).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.todo-item textarea').exists()).toBe(false)
  })
})
