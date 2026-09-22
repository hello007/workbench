import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ElMessage, ElMessageBox } from 'element-plus'
import AiChatPanel from '../AiChatPanel.vue'
import { useAiChatStore } from '../../store'
import {
  ListChatDirectories,
  AddChatDirectory,
  UpdateChatDirectory,
  RemoveChatDirectory,
  ReorderChatDirectories,
  ListChatSessions,
  CreateChatSession,
  GetChatSession,
  DeleteChatSession,
  RunChat,
  CancelChatTask,
  ListChatTemplates,
  AddChatTemplate,
  UpdateChatTemplate,
  RemoveChatTemplate,
  SaveChatSettings
} from '../../../wailsjs/go/main/App'
// 断言监听器注销行为需引用 mock 本体
import { EventsOn, EventsOff, BrowserOpenURL } from '../../../wailsjs/runtime/runtime'

// Wails 事件 handler 注册表（EventsOn mock 捕获 + 返回注销闭包，对齐真实 runtime 行为）
const eventHandlers = {}
const offClosures = []
vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((name, cb) => {
    eventHandlers[name] = cb
    const off = vi.fn(() => { delete eventHandlers[name] })
    offClosures.push(off)
    return off
  }),
  EventsOff: vi.fn(),
  BrowserOpenURL: vi.fn()
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: {
      error: vi.fn(),
      success: vi.fn(),
      warning: vi.fn(),
      info: vi.fn()
    },
    ElMessageBox: {
      confirm: vi.fn()
    }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  ListChatDirectories: vi.fn(() => Promise.resolve([
    { id: 'chatdir-1', path: 'D:\\workspace\\very-long-projects\\demo-projects\\alpha', displayName: '项目管理', sortOrder: 0, createdAt: 1000 },
    { id: 'chatdir-2', path: 'C:\\work\\server', displayName: 'server', sortOrder: 1, createdAt: 2000 }
  ])),
  AddChatDirectory: vi.fn(() => Promise.resolve({
    id: 'chatdir-9', path: 'D:\\new\\dir', displayName: 'dir', sortOrder: 2, createdAt: 3000
  })),
  UpdateChatDirectory: vi.fn(() => Promise.resolve()),
  RemoveChatDirectory: vi.fn(() => Promise.resolve()),
  ReorderChatDirectories: vi.fn(() => Promise.resolve()),
  ListChatSessions: vi.fn(() => Promise.resolve([
    // 后端按最近活跃降序：首个 = 最近更新
    { id: 'chatsession-new', directoryId: 'chatdir-1', title: '最近会话', cwd: 'D:\\p', createdAt: 2, updatedAt: 200 },
    { id: 'chatsession-old', directoryId: 'chatdir-1', title: '旧会话', cwd: 'D:\\p', createdAt: 1, updatedAt: 100 }
  ])),
  CreateChatSession: vi.fn(() => Promise.resolve({
    id: 'chatsession-created', directoryId: 'chatdir-1', title: '新会话', cwd: 'D:\\p', createdAt: 3, updatedAt: 300
  })),
  GetChatSession: vi.fn((id) => Promise.resolve({
    id, directoryId: 'chatdir-1', title: id === 'chatsession-new' ? '最近会话' : '旧会话', cwd: 'D:\\p', createdAt: 1, updatedAt: 100,
    messages: [
      { role: 'user', content: '第一问', timestamp: 1, taskId: 't1' },
      { role: 'assistant', content: '# 结论\n\n**要点一**\n\n```go\nfmt.Println("hi")\n```', timestamp: 2, taskId: 't1' }
    ]
  })),
  DeleteChatSession: vi.fn(() => Promise.resolve()),
  UpdateChatSessionTitle: vi.fn(() => Promise.resolve()),
  RunChat: vi.fn(() => Promise.resolve('chattask-1')),
  CancelChatTask: vi.fn(() => Promise.resolve(true)),
  GetChatTaskState: vi.fn(() => Promise.resolve(null)),
  ListChatTemplates: vi.fn(() => Promise.resolve([
    { id: 'tpl-dir', scope: 'directory', directoryId: 'chatdir-1', name: '目录模板', content: '目录模板内容' },
    { id: 'tpl-global', scope: 'global', directoryId: '', name: '全局模板', content: '全局模板内容' }
  ])),
  AddChatTemplate: vi.fn(() => Promise.resolve({ id: 'tpl-new', scope: 'directory', directoryId: 'chatdir-1', name: 'n', content: 'c' })),
  UpdateChatTemplate: vi.fn(() => Promise.resolve()),
  RemoveChatTemplate: vi.fn(() => Promise.resolve()),
  GetChatSettings: vi.fn(() => Promise.resolve({ permissionMode: 'default', modelName: '' })),
  SaveChatSettings: vi.fn(() => Promise.resolve())
}))

vi.mock('../../../utils/debug', () => ({
  debug: { log: vi.fn(), error: vi.fn(), warn: vi.fn() }
}))

const defaultStubs = {
  'el-button': { template: '<button v-bind="$attrs"><slot /></button>' },
  'el-icon': { template: '<i><slot /></i>' },
  // el-input stub：单根动态组件区分 textarea/input，透传 $attrs（keydown 监听
  // 经 $attrs 落在原生元素上），暴露 focus()/select()/input 与真实组件对齐
  'el-input': {
    template: `<component
      :is="type === 'textarea' ? 'textarea' : 'input'"
      ref="el"
      :value="modelValue"
      v-bind="$attrs"
      @input="$emit('update:modelValue', $event.target.value)"
    />`,
    props: ['modelValue', 'placeholder', 'disabled', 'type'],
    emits: ['update:modelValue'],
    computed: {
      input() { return this.$refs.el }
    },
    methods: {
      focus() { this.$refs.el && this.$refs.el.focus && this.$refs.el.focus() },
      select() { this.$refs.el && this.$refs.el.select && this.$refs.el.select() }
    }
  },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div><slot /></div>', props: ['label'] },
  'el-dialog': {
    template: '<div v-if="modelValue" class="el-dialog"><slot /><slot name="footer" /></div>',
    props: ['modelValue'],
    emits: ['update:modelValue']
  },
  'el-empty': { template: '<div class="el-empty">{{ description }}</div>', props: ['description', 'imageSize'] },
  // el-dropdown stub：经 provide 向后代 el-dropdown-item 下发命令派发函数，
  // 使下拉项可 DOM 点击触发 @command（对齐真实 EP 组件的 item 点击行为）
  'el-dropdown': {
    template: '<div class="el-dropdown"><slot /><slot name="dropdown" /></div>',
    emits: ['command'],
    provide() {
      return { __dropdownCommand: (cmd) => this.$emit('command', cmd) }
    }
  },
  'el-dropdown-menu': { template: '<div class="el-dropdown-menu"><slot /></div>' },
  'el-dropdown-item': {
    template: '<div class="el-dropdown-item" :class="{ \'is-disabled\': disabled }" @click="onClick"><slot /></div>',
    props: ['command', 'disabled', 'divided'],
    inject: { __dropdownCommand: { default: null } },
    methods: {
      onClick() {
        if (this.disabled) return
        if (this.__dropdownCommand) this.__dropdownCommand(this.command)
      }
    }
  },
  'el-select': {
    template: '<select :value="modelValue" @change="onChange"><slot /></select>',
    props: ['modelValue', 'placeholder', 'size', 'filterable', 'allowCreate', 'defaultFirstOption'],
    emits: ['update:modelValue', 'change'],
    methods: {
      onChange(e) {
        this.$emit('update:modelValue', e.target.value)
        this.$emit('change', e.target.value)
      }
    }
  },
  'el-option': { template: '<option :value="value">{{ label }}</option>', props: ['value', 'label'] },
  'el-tooltip': { template: '<div><slot /></div>', props: ['content', 'placement'] },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['size', 'type'] },
  'el-radio-group': { template: '<div><slot /></div>', props: ['modelValue', 'disabled'] },
  'el-radio': { template: '<label><slot /></label>', props: ['value', 'disabled'] },
  Folder: { template: '<span>folder</span>' },
  Delete: { template: '<span>del</span>' },
  Plus: { template: '<span>plus</span>' },
  Edit: { template: '<span>edit</span>' },
  ChatDotRound: { template: '<span>chat</span>' },
  ArrowDown: { template: '<span>arrow-down</span>' },
  ChatLineRound: { template: '<span>chat-line</span>' },
  Document: { template: '<span>doc</span>' },
  Setting: { template: '<span>setting</span>' },
  Position: { template: '<span>position</span>' },
  VideoPause: { template: '<span>pause</span>' }
}

// 列表数据经 aiChat store（onMounted loadChatDirectories 驱动），mount 后统一 flushPromises。
function createWrapper() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const wrapper = mount(AiChatPanel, {
    global: { plugins: [pinia], stubs: defaultStubs }
  })
  return wrapper
}

// 选中第一个目录的标准前置（走真实点击路径，触发 watch 加载会话与模板）
async function selectFirstDirectory(wrapper) {
  await wrapper.findAll('.chat-dir-item')[0].trigger('click')
  await flushPromises()
}

describe('AiChatPanel.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    for (const k of Object.keys(eventHandlers)) delete eventHandlers[k]
    offClosures.length = 0
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  describe('目录列表渲染', () => {
    it('挂载即加载目录并渲染列表项（显示名 + 短路径）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      expect(ListChatDirectories).toHaveBeenCalled()

      const items = wrapper.findAll('.chat-dir-item')
      expect(items.length).toBe(2)
      const names = wrapper.findAll('.chat-dir-name')
      expect(names[0].text()).toBe('项目管理')
      expect(names[1].text()).toBe('server')
    })

    it('长路径折叠为短路径展示（title 保留全路径）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      const paths = wrapper.findAll('.chat-dir-path')
      expect(paths[0].text()).toBe('.../demo-projects/alpha')
      expect(paths[1].text()).toBe('C:\\work\\server')
    })

    it('空列表显示空态与右键添加提示', async () => {
      ListChatDirectories.mockResolvedValueOnce([])
      wrapper = createWrapper()
      await flushPromises()
      expect(wrapper.find('.chat-dirs-empty').exists()).toBe(true)
      expect(wrapper.find('.chat-dirs-hint').exists()).toBe(true)
      expect(wrapper.findAll('.chat-dir-item').length).toBe(0)
    })
  })

  describe('目录项交互', () => {
    it('点击目录项置选中态（is-active）并渲染右侧对话区工具条', async () => {
      wrapper = createWrapper()
      await flushPromises()

      const items = wrapper.findAll('.chat-dir-item')
      expect(items[0].classes()).not.toContain('is-active')
      await items[0].trigger('click')
      await flushPromises()

      const store = useAiChatStore()
      expect(store.selectedChatDirectoryId).toBe('chatdir-1')
      expect(wrapper.findAll('.chat-dir-item')[0].classes()).toContain('is-active')
      // 对话区渲染：会话下拉默认选中最近更新会话
      expect(wrapper.find('.chat-toolbar').exists()).toBe(true)
      expect(wrapper.find('.session-btn-label').text()).toBe('最近会话')
    })

    it('未选中时右侧占位文案为引导语', async () => {
      ListChatDirectories.mockResolvedValueOnce([])
      wrapper = createWrapper()
      await flushPromises()
      expect(wrapper.find('.chat-main-empty').text()).toContain('从左侧选择目录开始')
    })

    it('确认后移除目录项并提示成功', async () => {
      ElMessageBox.confirm.mockResolvedValueOnce()
      wrapper = createWrapper()
      await flushPromises()

      await wrapper.find('.chat-dir-remove').trigger('click')
      await flushPromises()

      expect(ElMessageBox.confirm).toHaveBeenCalled()
      expect(RemoveChatDirectory).toHaveBeenCalledWith('chatdir-1')
      expect(ElMessage.success).toHaveBeenCalledWith('已移除')
    })

    it('取消确认不移除目录项', async () => {
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')
      wrapper = createWrapper()
      await flushPromises()

      await wrapper.find('.chat-dir-remove').trigger('click')
      await flushPromises()

      expect(RemoveChatDirectory).not.toHaveBeenCalled()
    })

    it('移除目录确认文案如实说明历史会话不可达', async () => {
      ElMessageBox.confirm.mockResolvedValueOnce()
      wrapper = createWrapper()
      await flushPromises()

      await wrapper.find('.chat-dir-remove').trigger('click')

      expect(ElMessageBox.confirm).toHaveBeenCalledWith(
        expect.stringContaining('历史会话'),
        '移除目录',
        expect.objectContaining({ type: 'warning' })
      )
    })

    it('移除失败走 handleError 报错提示', async () => {
      ElMessageBox.confirm.mockResolvedValueOnce()
      RemoveChatDirectory.mockRejectedValueOnce({ message: '目录项不存在' })
      wrapper = createWrapper()
      await flushPromises()

      await wrapper.find('.chat-dir-remove').trigger('click')
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalledWith('移除失败: 目录项不存在')
    })

    it('拖拽结束按新序持久化排序', async () => {
      wrapper = createWrapper()
      await flushPromises()

      await wrapper.vm.$.setupState.onDragEnd()
      expect(ReorderChatDirectories).toHaveBeenCalledWith(['chatdir-1', 'chatdir-2'])
    })

    it('排序保存失败回滚本地顺序并报错', async () => {
      ReorderChatDirectories.mockRejectedValueOnce({ message: '磁盘已满' })
      wrapper = createWrapper()
      await flushPromises()

      const state = wrapper.vm.$.setupState
      // 模拟拖拽后的本地新序（chatdir-2 拖到最前）
      state.localDirs = [
        { id: 'chatdir-2', path: 'C:\\work\\server', displayName: 'server', sortOrder: 1, createdAt: 2000 },
        { id: 'chatdir-1', path: 'D:\\workspace\\very-long-projects\\demo-projects\\alpha', displayName: '项目管理', sortOrder: 0, createdAt: 1000 }
      ]
      await state.onDragEnd()
      await flushPromises()

      // 持久化失败：本地顺序回滚为 store 真实顺序，不停留在未保存的拖拽结果
      expect(state.localDirs.map(d => d.id)).toEqual(['chatdir-1', 'chatdir-2'])
      expect(ElMessage.error).toHaveBeenCalledWith('排序保存失败: 磁盘已满')
    })

    it('重命名：DOM 路径点击编辑图标，对话框预填当前名，提交后调 UpdateChatDirectory 并关闭', async () => {
      wrapper = createWrapper()
      await flushPromises()

      await wrapper.findAll('.chat-dir-edit')[0].trigger('click')
      const dialog = wrapper.find('.el-dialog')
      expect(dialog.exists()).toBe(true)
      // 显示名输入框预填当前名（第 2 个 input，第 1 个为禁用的当前路径）
      const inputs = dialog.findAll('input')
      expect(inputs[1].element.value).toBe('项目管理')
      await inputs[1].setValue('新名')

      const okBtn = dialog.findAll('button').find(b => b.text() === '确定')
      await okBtn.trigger('click')
      await flushPromises()

      expect(UpdateChatDirectory).toHaveBeenCalledWith('chatdir-1', '新名')
      expect(ElMessage.success).toHaveBeenCalledWith('重命名成功')
      expect(wrapper.find('.el-dialog').exists()).toBe(false)
    })

    it('重命名：空名 warning 不调后端', async () => {
      wrapper = createWrapper()
      await flushPromises()

      wrapper.vm.$.setupState.renameTarget = { id: 'chatdir-1', path: 'D:\\a', displayName: '项目管理' }
      wrapper.vm.$.setupState.renameName = '   '
      await wrapper.vm.$.setupState.handleRename()

      expect(ElMessage.warning).toHaveBeenCalledWith('请输入显示名')
      expect(UpdateChatDirectory).not.toHaveBeenCalled()
    })
  })

  describe('添加目录对话框', () => {
    it('点击「添加目录」打开对话框，填表提交成功后关闭并刷新列表（DOM 路径）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      expect(wrapper.find('.el-dialog').exists()).toBe(false)

      await wrapper.find('.panel-actions button').trigger('click')
      expect(wrapper.find('.el-dialog').exists()).toBe(true)

      const inputs = wrapper.findAll('.el-dialog input')
      await inputs[0].setValue('D:\\new\\dir')
      await inputs[1].setValue('项目管理')
      const okBtn = wrapper.findAll('.el-dialog button').find(b => b.text() === '确定')
      await okBtn.trigger('click')
      await flushPromises()

      expect(AddChatDirectory).toHaveBeenCalledWith('D:\\new\\dir', '项目管理')
      expect(ElMessage.success).toHaveBeenCalledWith('已添加「dir」')
      expect(wrapper.find('.el-dialog').exists()).toBe(false)
    })

    it('路径为空时 warning 提示且不调后端', async () => {
      wrapper = createWrapper()
      await flushPromises()

      wrapper.vm.$.setupState.addForm.path = '   '
      await wrapper.vm.$.setupState.handleAdd()

      expect(ElMessage.warning).toHaveBeenCalledWith('请输入目录路径')
      expect(AddChatDirectory).not.toHaveBeenCalled()
    })

    it('重复路径 info 提示且不调后端', async () => {
      wrapper = createWrapper()
      await flushPromises()

      wrapper.vm.$.setupState.addForm.path = 'd:/WORKSPACE/VERY-LONG-PROJECTS/DEMO-PROJECTS/ALPHA'
      await wrapper.vm.$.setupState.handleAdd()

      expect(ElMessage.info).toHaveBeenCalledWith('该目录已在常用目录列表中')
      expect(AddChatDirectory).not.toHaveBeenCalled()
    })

    it('后端报错走 handleError 提示且对话框保持打开', async () => {
      AddChatDirectory.mockRejectedValueOnce({ message: '目录不存在: x' })
      wrapper = createWrapper()
      await flushPromises()

      // 先打开对话框再提交，报错后应保持打开不误关
      wrapper.vm.$.setupState.addDialogVisible = true
      wrapper.vm.$.setupState.addForm.path = 'D:\\bad'
      await wrapper.vm.$.setupState.handleAdd()
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalledWith('添加失败: 目录不存在: x')
      expect(wrapper.vm.$.setupState.addDialogVisible).toBe(true)
    })
  })

  describe('会话管理', () => {
    it('选中目录后加载会话列表并默认选中最近更新会话，消息气泡渲染', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(ListChatSessions).toHaveBeenCalledWith('chatdir-1')
      // 气泡：1 条 user + 1 条 assistant
      expect(wrapper.findAll('.chat-msg-row.user').length).toBe(1)
      const assistantRows = wrapper.findAll('.chat-msg-row.assistant')
      expect(assistantRows.length).toBe(1)
      // assistant markdown 渲染：标题与代码块
      expect(assistantRows[0].find('.chat-msg-md h1').exists()).toBe(true)
      expect(assistantRows[0].find('.chat-msg-md strong').text()).toBe('要点一')
      expect(assistantRows[0].find('.chat-msg-md pre.hljs').exists()).toBe(true)
    })

    it('空消息会话显示输入引导空态', async () => {
      GetChatSession.mockResolvedValueOnce({
        id: 'chatsession-new', directoryId: 'chatdir-1', title: '空会话', cwd: 'D:\\p',
        createdAt: 1, updatedAt: 100, messages: []
      })
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(wrapper.find('.chat-messages-empty').exists()).toBe(true)
      expect(wrapper.find('.chat-messages-empty').text()).toContain('输入需求开始对话')
    })

    it('目录无会话时空态提示新建会话', async () => {
      ListChatSessions.mockResolvedValueOnce([])
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(wrapper.find('.chat-messages-empty').text()).toContain('新建会话')
    })

    it('会话下拉 DOM 点击切换会话并加载对应消息', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)
      GetChatSession.mockClear()

      const item = wrapper.findAll('.session-menu .el-dropdown-item')
        .find(w => w.text().includes('旧会话'))
      await item.trigger('click')
      await flushPromises()

      expect(GetChatSession).toHaveBeenCalledWith('chatsession-old')
      expect(useAiChatStore().selectedChatSessionId).toBe('chatsession-old')
    })

    it('新建会话调 CreateChatSession 并选中新会话', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      // 建会话后重载列表：mock 固定返回 2 条 + created 不在列表，
      // 手动注入重载返回让选中链路完整
      CreateChatSession.mockImplementationOnce(async () => {
        ListChatSessions.mockResolvedValueOnce([
          { id: 'chatsession-created', directoryId: 'chatdir-1', title: '新会话', cwd: 'D:\\p', createdAt: 3, updatedAt: 300 },
          { id: 'chatsession-new', directoryId: 'chatdir-1', title: '最近会话', cwd: 'D:\\p', createdAt: 2, updatedAt: 200 }
        ])
        return { id: 'chatsession-created', directoryId: 'chatdir-1', title: '新会话', cwd: 'D:\\p', createdAt: 3, updatedAt: 300 }
      })
      await wrapper.vm.$.setupState.handleNewSession()
      await flushPromises()

      expect(CreateChatSession).toHaveBeenCalledWith(
        'chatdir-1', '', 'D:\\workspace\\very-long-projects\\demo-projects\\alpha'
      )
      expect(useAiChatStore().selectedChatSessionId).toBe('chatsession-created')
    })

    it('删除当前会话：确认后调 DeleteChatSession', async () => {
      ElMessageBox.confirm.mockResolvedValueOnce()
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      await wrapper.vm.$.setupState.handleDeleteSession()
      await flushPromises()

      expect(ElMessageBox.confirm).toHaveBeenCalled()
      expect(DeleteChatSession).toHaveBeenCalledWith('chatsession-new')
      expect(ElMessage.success).toHaveBeenCalledWith('会话已删除')
    })

    it('删除会话取消确认不调后端', async () => {
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      await wrapper.vm.$.setupState.handleDeleteSession()
      await flushPromises()

      expect(DeleteChatSession).not.toHaveBeenCalled()
    })

    it('会话下拉项渲染标题与更新时间', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const titles = wrapper.findAll('.session-menu .session-menu-title')
      expect(titles.length).toBe(2)
      expect(titles[0].text()).toBe('最近会话')
      expect(wrapper.findAll('.session-menu .session-menu-time')[0].text()).not.toBe('')
    })
  })

  describe('输入与发送', () => {
    it('Enter 发送：RunChat 以当前配置调用，输入框清空，乐观 user 气泡出现', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const ta = wrapper.find("textarea.chat-input")
      await ta.setValue('帮我看看这个项目')
      await ta.trigger('keydown.enter')
      await flushPromises()

      expect(RunChat).toHaveBeenCalledWith('chatsession-new', '帮我看看这个项目', 'default', '')
      expect(wrapper.vm.$.setupState.inputText).toBe('')
      // RunChat resolve 即运行态：状态行显示生成中而非排队等待（F2 回归）
      expect(wrapper.find('.chat-input-status').text()).toContain('回复生成中')
      // 基准 1 条 user + 乐观 1 条
      expect(wrapper.findAll('.chat-msg-row.user').length).toBe(2)
      // 流式 assistant 气泡出现
      expect(wrapper.find('.chat-msg-bubble.is-streaming').exists()).toBe(true)
      // 停止按钮出现（发送按钮切换）
      expect(wrapper.find('.chat-stop-btn').exists()).toBe(true)
    })

    it('Shift+Enter 换行不发送', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const ta = wrapper.find("textarea.chat-input")
      await ta.setValue('第一行')
      await ta.trigger('keydown', { key: 'Enter', shiftKey: true })

      expect(RunChat).not.toHaveBeenCalled()
    })

    it('中文 IME 组合输入的 Enter（isComposing）不发送', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const ta = wrapper.find("textarea.chat-input")
      await ta.setValue('nihao')
      // 构造 isComposing=true 的 keydown 模拟 IME 选词确认（普通 Event 实例可直赋属性）
      const evt = new Event('keydown', { bubbles: true, cancelable: true })
      evt.key = 'Enter'
      evt.isComposing = true
      ta.element.dispatchEvent(evt)
      await flushPromises()

      expect(RunChat).not.toHaveBeenCalled()
      // 输入保留，未被当作消息发送清空
      expect(ta.element.value).toBe('nihao')
    })

    it('空文本点击发送按钮 warning 且不调后端', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      await wrapper.find('.chat-send-btn').trigger('click')
      expect(ElMessage.warning).toHaveBeenCalledWith('请输入对话内容')
      expect(RunChat).not.toHaveBeenCalled()
    })

    it('权限与模型配置随发送参数传递', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const store = useAiChatStore()
      await store.saveChatSettings('acceptEdits', 'opus')
      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      expect(RunChat).toHaveBeenCalledWith('chatsession-new', 'hi', 'acceptEdits', 'opus')
    })

    it('无会话时直接输入自动新建会话再发送', async () => {
      ListChatSessions.mockResolvedValueOnce([])
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      CreateChatSession.mockImplementationOnce(async () => {
        ListChatSessions.mockResolvedValueOnce([
          { id: 'chatsession-created', directoryId: 'chatdir-1', title: '新会话', cwd: 'D:\\p', createdAt: 3, updatedAt: 300 }
        ])
        return { id: 'chatsession-created', directoryId: 'chatdir-1', title: '新会话', cwd: 'D:\\p', createdAt: 3, updatedAt: 300 }
      })
      wrapper.vm.$.setupState.inputText = '第一句'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      expect(CreateChatSession).toHaveBeenCalled()
      expect(RunChat).toHaveBeenCalledWith('chatsession-created', '第一句', 'default', '')
    })

    it('首发建会话挂起期间重复发送被忽略（防双击双会话双发）', async () => {
      ListChatSessions.mockResolvedValueOnce([])
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      // 建会话请求挂起：模拟后端延迟
      let resolveCreate
      CreateChatSession.mockImplementationOnce(
        () => new Promise((r) => { resolveCreate = r })
      )
      wrapper.vm.$.setupState.inputText = '第一句'
      const first = wrapper.vm.$.setupState.handleSend()
      const second = wrapper.vm.$.setupState.handleSend() // 挂起期间重入
      resolveCreate({ id: 'chatsession-created', directoryId: 'chatdir-1', title: '新会话', cwd: 'D:\\p', createdAt: 3, updatedAt: 300 })
      await Promise.all([first, second])
      await flushPromises()

      expect(CreateChatSession).toHaveBeenCalledTimes(1)
      expect(RunChat).toHaveBeenCalledTimes(1)
    })

    it('发送失败走 handleError 且输入保留', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      RunChat.mockRejectedValueOnce({ code: 'E_CHAT_IN_PROGRESS', message: '该会话有对话进行中' })
      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      // 预期拒绝类走 warning（handleError 分流）
      expect(ElMessage.warning).toHaveBeenCalledWith('该会话有对话进行中')
      expect(wrapper.vm.$.setupState.inputText).toBe('hi')
    })

    it('停止按钮调 CancelChatTask', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.find('.chat-stop-btn').trigger('click')
      await flushPromises()
      expect(CancelChatTask).toHaveBeenCalledWith('chattask-1')
    })
  })

  describe('流式事件与在途恢复', () => {
    it('chat-task:output 增量追加流式气泡，done 后定型重载', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = '测试流式'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      eventHandlers['chat-task:output']({ taskId: 'chattask-1', text: '部分回复' })
      await flushPromises()
      expect(wrapper.find('.chat-msg-bubble.is-streaming').text()).toContain('部分回复')

      await eventHandlers['chat-task:done']({
        taskId: 'chattask-1', chatSessionId: 'chatsession-new', reply: '全文回复', exitCode: 0
      })
      await flushPromises()

      expect(wrapper.find('.chat-msg-bubble.is-streaming').exists()).toBe(false)
      // 定型：GetChatSession 重载（mock 返回基准 2 条消息）
      expect(wrapper.findAll('.chat-msg-row').length).toBe(2)
      // 发送按钮恢复
      expect(wrapper.find('.chat-send-btn').exists()).toBe(true)
    })

    it('markdown 链接点击被拦截：外链经 BrowserOpenURL 打开且不触发 webview 导航', async () => {
      GetChatSession.mockResolvedValueOnce({
        id: 'chatsession-new', directoryId: 'chatdir-1', title: '最近会话', cwd: 'D:\\p',
        createdAt: 1, updatedAt: 100,
        messages: [
          { role: 'assistant', content: '看 [文档](https://example.com/doc) 与 [相对](./notes.md)', timestamp: 2 }
        ]
      })
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      // 拦截处理器在执行后会 stopPropagation，故在同容器（.chat-msg-md）上
      // 后置注册监听（同元素监听按注册序执行，晚于 Vue 处理器）读取 defaultPrevented
      let clickDefaultPrevented = null
      wrapper.find('.chat-msg-md').element.addEventListener('click', (e) => {
        clickDefaultPrevented = e.defaultPrevented
      })

      const links = wrapper.findAll('.chat-msg-md a')
      expect(links.length).toBe(2)

      // 外部链接：阻止默认导航 + 交系统浏览器打开
      await links[0].trigger('click')
      expect(BrowserOpenURL).toHaveBeenCalledWith('https://example.com/doc')
      expect(clickDefaultPrevented).toBe(true)

      // 相对链接：仅阻止导航（对话场景无应用内跳转语义），不交系统浏览器
      await links[1].trigger('click')
      expect(BrowserOpenURL).toHaveBeenCalledTimes(1)
      expect(clickDefaultPrevented).toBe(true)
    })

    it('用户上翻离开底部时暂停自动跟随，滚回底部附近恢复', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const el = wrapper.find('.chat-messages').element
      // jsdom 无布局：注入滚动度量（视口 400，内容 1000）
      Object.defineProperty(el, 'scrollHeight', { value: 1000, configurable: true })
      Object.defineProperty(el, 'clientHeight', { value: 400, configurable: true })

      // 用户上翻至距底 500px（> 40 阈值）→ 暂停跟随
      el.scrollTop = 100
      await wrapper.find('.chat-messages').trigger('scroll')

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()
      // 新消息到达但用户已上翻：不强制拽回底部
      expect(el.scrollTop).toBe(100)

      // 滚回底部附近（距底 10px ≤ 阈值）→ 恢复跟随
      el.scrollTop = 590
      await wrapper.find('.chat-messages').trigger('scroll')

      eventHandlers['chat-task:output']({ taskId: 'chattask-1', text: '增量' })
      await flushPromises()
      await flushPromises()
      expect(el.scrollTop).toBe(1000)
    })

    it('done 失败提示与取消提示', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await eventHandlers['chat-task:done']({ taskId: 'chattask-1', error: '执行超时（上限 10 分钟）' })
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith('对话失败: 执行超时（上限 10 分钟）')
    })

    it('不匹配 taskId 的 chat-task 事件被忽略', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      eventHandlers['chat-task:output']({ taskId: 'other-task', text: '不该出现' })
      await flushPromises()
      expect(wrapper.find('.chat-msg-bubble.is-streaming').exists()).toBe(false)
      expect(wrapper.findAll('.chat-msg-row.assistant').length).toBe(1)
    })

    it('在途任务时切换目录：确认后取消任务并切换', async () => {
      ElMessageBox.confirm.mockResolvedValueOnce()
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.findAll('.chat-dir-item')[1].trigger('click')
      await flushPromises()

      expect(ElMessageBox.confirm).toHaveBeenCalled()
      expect(CancelChatTask).toHaveBeenCalledWith('chattask-1')
      expect(useAiChatStore().selectedChatDirectoryId).toBe('chatdir-2')
    })

    it('在途任务时切换目录：留在当前不切换不取消', async () => {
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.findAll('.chat-dir-item')[1].trigger('click')
      await flushPromises()

      expect(CancelChatTask).not.toHaveBeenCalled()
      expect(useAiChatStore().selectedChatDirectoryId).toBe('chatdir-1')
    })

    it('在途任务时切换会话同样确认拦截', async () => {
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.vm.$.setupState.handleSelectSession('chatsession-old')
      await flushPromises()

      expect(ElMessageBox.confirm).toHaveBeenCalled()
      expect(useAiChatStore().selectedChatSessionId).toBe('chatsession-new')
    })

    it('取消任务失败时中止切换（不带着在途任务切走）', async () => {
      ElMessageBox.confirm.mockResolvedValueOnce()
      CancelChatTask.mockRejectedValueOnce({ message: '进程句柄失效' })
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.findAll('.chat-dir-item')[1].trigger('click')
      await flushPromises()

      expect(CancelChatTask).toHaveBeenCalledWith('chattask-1')
      expect(ElMessage.error).toHaveBeenCalledWith('取消任务失败: 进程句柄失效')
      // 取消失败：不切换，留在当前目录
      expect(useAiChatStore().selectedChatDirectoryId).toBe('chatdir-1')
    })

    it('删除在途会话：先确认取消任务再删除（防孤儿消息复活）', async () => {
      ElMessageBox.confirm.mockResolvedValue()
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.vm.$.setupState.handleDeleteSession()
      await flushPromises()

      // 两次确认：先「取消任务并删除」再「删除会话」
      expect(ElMessageBox.confirm).toHaveBeenCalledTimes(2)
      expect(CancelChatTask).toHaveBeenCalledWith('chattask-1')
      expect(DeleteChatSession).toHaveBeenCalledWith('chatsession-new')
    })

    it('删除在途会话：取消确认被拒时不删除不取消', async () => {
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.vm.$.setupState.handleDeleteSession()
      await flushPromises()

      expect(CancelChatTask).not.toHaveBeenCalled()
      expect(DeleteChatSession).not.toHaveBeenCalled()
    })

    it('数据加载完成前卸载也能注销全部监听器（先注册后加载）', async () => {
      let resolveLoad
      ListChatDirectories.mockImplementationOnce(() => new Promise((r) => { resolveLoad = r }))
      wrapper = createWrapper()
      // 不 flushPromises：onMounted 内数据加载仍挂起时立即卸载
      wrapper.unmount()
      wrapper = null
      resolveLoad([
        { id: 'chatdir-1', path: 'D:\\p', displayName: '项目管理', sortOrder: 0, createdAt: 1000 }
      ])
      await flushPromises()

      const chatOffs = offClosures.slice(0, 4)
      expect(chatOffs.length).toBe(4)
      chatOffs.forEach((off) => expect(off).toHaveBeenCalled())
      ;['chat-task:queued', 'chat-task:started', 'chat-task:output', 'chat-task:done']
        .forEach((e) => expect(eventHandlers[e]).toBeUndefined())
    })

    it('卸载时经 EventsOn 返回闭包精准注销监听器（禁 EventsOff）', async () => {
      wrapper = createWrapper()
      await flushPromises()

      const events = ['chat-task:queued', 'chat-task:started', 'chat-task:output', 'chat-task:done']
      events.forEach((e) => expect(eventHandlers[e]).toBeTypeOf('function'))

      wrapper.unmount()
      wrapper = null
      await flushPromises()

      // 4 个注销闭包均被调用；EventsOff 全局移除未被使用
      const chatOffs = offClosures.slice(0, 4)
      expect(chatOffs.length).toBe(4)
      chatOffs.forEach((off) => expect(off).toHaveBeenCalled())
      expect(EventsOff).not.toHaveBeenCalled()
      events.forEach((e) => expect(eventHandlers[e]).toBeUndefined())
    })
  })

  describe('模板系统', () => {
    it('选中目录后加载合并模板列表（目录 + 全局两组渲染）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(ListChatTemplates).toHaveBeenCalledWith('chatdir-1')
      const groups = wrapper.findAll('.tpl-group-title')
      expect(groups.length).toBe(2)
      expect(groups[0].text()).toBe('本目录模板')
      expect(groups[1].text()).toBe('全局模板')
    })

    it('模板点击整段替换填入输入框', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      await wrapper.vm.$.setupState.applyTemplate('tpl-dir')
      expect(wrapper.vm.$.setupState.inputText).toBe('目录模板内容')
    })

    it('模板下拉「管理模板」打开管理弹窗', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(wrapper.vm.$.setupState.tplDialogVisible).toBe(false)
      await wrapper.vm.$.setupState.applyTemplate('__manage_templates__')
      expect(wrapper.vm.$.setupState.tplDialogVisible).toBe(true)
    })

    it('新增目录模板：模板下拉 -> 管理弹窗 DOM 路径调 AddChatTemplate 并重置表单', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      // 模板下拉点击「管理模板」打开弹窗
      await wrapper.find('.tpl-manage-item').trigger('click')
      expect(wrapper.find('.tpl-manage-body').exists()).toBe(true)

      const form = wrapper.find('.tpl-form')
      await form.find('input').setValue('新模板')
      await form.find('textarea').setValue('内容A')
      const addBtn = wrapper.findAll('.el-dialog button').find(b => b.text() === '新增')
      await addBtn.trigger('click')
      await flushPromises()

      expect(AddChatTemplate).toHaveBeenCalledWith('directory', 'chatdir-1', '新模板', '内容A')
      expect(ElMessage.success).toHaveBeenCalledWith('模板已新增')
      expect(wrapper.vm.$.setupState.tplForm.id).toBe('')
    })

    it('编辑模板：调 UpdateChatTemplate', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const state = wrapper.vm.$.setupState
      state.tplForm = { id: 'tpl-dir', scope: 'directory', name: '改名', content: '改内容' }
      await state.handleSaveTemplate()
      await flushPromises()

      expect(UpdateChatTemplate).toHaveBeenCalledWith('tpl-dir', '改名', '改内容')
      expect(ElMessage.success).toHaveBeenCalledWith('模板已更新')
    })

    it('模板表单校验：空名/空内容 warning 不调后端', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const state = wrapper.vm.$.setupState
      state.tplForm = { id: '', scope: 'global', name: '  ', content: 'c' }
      await state.handleSaveTemplate()
      expect(ElMessage.warning).toHaveBeenCalledWith('请输入模板名称')

      state.tplForm = { id: '', scope: 'global', name: 'n', content: ' ' }
      await state.handleSaveTemplate()
      expect(ElMessage.warning).toHaveBeenCalledWith('请输入模板内容')
      expect(AddChatTemplate).not.toHaveBeenCalled()
    })

    it('删除模板：确认后调 RemoveChatTemplate', async () => {
      ElMessageBox.confirm.mockResolvedValueOnce()
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      await wrapper.vm.$.setupState.handleRemoveTemplate({ id: 'tpl-dir', name: '目录模板' })
      await flushPromises()

      expect(RemoveChatTemplate).toHaveBeenCalledWith('tpl-dir')
      expect(ElMessage.success).toHaveBeenCalledWith('模板已删除')
    })

    it('模板无数据时下拉显示占位项', async () => {
      ListChatTemplates.mockResolvedValueOnce([])
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(wrapper.find('.tpl-menu').text()).toContain('暂无模板')
    })
  })

  describe('权限与模型配置', () => {
    it('权限模式下拉切换持久化并调 SaveChatSettings', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const selects = wrapper.findAll('select')
      expect(selects.length).toBe(2)
      await selects[0].setValue('plan')
      await flushPromises()

      expect(SaveChatSettings).toHaveBeenCalledWith('plan', '')
      expect(useAiChatStore().chatSettings.permissionMode).toBe('plan')
    })

    it('选择 bypassPermissions 时警告提示', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const selects = wrapper.findAll('select')
      await selects[0].setValue('bypassPermissions')
      await flushPromises()

      expect(ElMessage.warning).toHaveBeenCalledWith(
        expect.stringContaining('跳过确认')
      )
      expect(SaveChatSettings).toHaveBeenCalledWith('bypassPermissions', '')
    })

    it('模型下拉切换持久化（allow-create 自定义值同样生效）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const selects = wrapper.findAll('select')
      await selects[1].setValue('sonnet')
      await flushPromises()

      expect(SaveChatSettings).toHaveBeenCalledWith('default', 'sonnet')
    })
  })
})
