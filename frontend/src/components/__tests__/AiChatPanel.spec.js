import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ElMessage, ElMessageBox, ElRadioGroup, ElRadioButton } from 'element-plus'
import AiChatPanel from '../AiChatPanel.vue'
import { useAiChatStore, useUiStore } from '../../store'
import { CMD_RUN_BTN_TITLE_NO_CWD } from '../../utils/chatMarkdown'
import { CLAUDE_BUILTIN_COMMANDS } from '../../constants/claudeCommands'
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
  ListChatActiveTasks,
  RespondChatApproval,
  ListChatTemplates,
  AddChatTemplate,
  UpdateChatTemplate,
  RemoveChatTemplate,
  SaveChatSettings,
  DiscoverChatClis,
  GetDiscoveredSkills,
  RefreshDiscoveredSkills,
  OpenFileDialog
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
  GetClaudeDefaultModel: vi.fn(() => Promise.resolve('')),
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
    contextTokens: 46000,
    messages: [
      { role: 'user', content: '第一问', timestamp: 1, taskId: 't1' },
      { role: 'assistant', content: '# 结论\n\n**要点一**\n\n```bash\nfmt.Println("hi")\n```', timestamp: 2, taskId: 't1' }
    ]
  })),
  // 单仓库状态快照（GetRepoStatus）：当前无前端消费方（对话状态栏精简版已移除
  // 分支段），保留 mock 防组件回归引入时缺 mock
  GetRepoStatus: vi.fn(() => Promise.resolve({
    path: 'D:\\p', name: 'p', branch: 'feature/ctx', dirty: true, ahead: 1, behind: 0,
    hasUpstream: true, detached: false, isRepo: true
  })),
  DeleteChatSession: vi.fn(() => Promise.resolve()),
  UpdateChatSessionTitle: vi.fn(() => Promise.resolve()),
  RunChat: vi.fn(() => Promise.resolve('chattask-1')),
  CancelChatTask: vi.fn(() => Promise.resolve(true)),
  ListChatActiveTasks: vi.fn(() => Promise.resolve([])),
  RespondChatApproval: vi.fn(() => Promise.resolve()),
  GetChatTaskState: vi.fn(() => Promise.resolve(null)),
  ListChatTemplates: vi.fn(() => Promise.resolve([
    { id: 'tpl-dir', scope: 'directory', directoryId: 'chatdir-1', name: '目录模板', content: '目录模板内容' },
    { id: 'tpl-global', scope: 'global', directoryId: '', name: '全局模板', content: '全局模板内容' }
  ])),
  AddChatTemplate: vi.fn(() => Promise.resolve({ id: 'tpl-new', scope: 'directory', directoryId: 'chatdir-1', name: 'n', content: 'c' })),
  UpdateChatTemplate: vi.fn(() => Promise.resolve()),
  RemoveChatTemplate: vi.fn(() => Promise.resolve()),
  GetChatSettings: vi.fn(() => Promise.resolve({ permissionMode: 'default', modelName: '', launcher: '' })),
  SaveChatSettings: vi.fn(() => Promise.resolve()),
  DiscoverChatClis: vi.fn(() => Promise.resolve([
    { name: 'claude', path: 'C:\\tools\\claude.cmd' },
    { name: 'myai', path: 'C:\\tools\\myai.cmd' }
  ])),
  // 已装 skill 发现（/ 斜杠补全弹层数据源）：五条混合 fixture——user 恒可用、
  // project 匹配会话 cwd（D:\p）/不匹配各一条、plugin 空 cwd（用户级安装）恒可用 /
  // 绑定会话 cwd 各一条，覆盖 availableDiscoveredSkills 的会话级过滤
  GetDiscoveredSkills: vi.fn(() => Promise.resolve([
    { name: 'drawio', description: '画图工具', command: '/drawio', cwd: '', source: 'user', sourceDir: 'C:/h/.claude/skills/drawio', plugin: '' },
    { name: 'release', description: '发版流程', command: '/release', cwd: 'D:\\p', source: 'project', sourceDir: 'D:/p/.claude/skills/release', plugin: '' },
    { name: 'other', description: '别处项目级', command: '/other', cwd: 'C:\\other', source: 'project', sourceDir: 'C:/other/.claude/skills/other', plugin: '' },
    { name: 'agree-slides', description: '幻灯片', command: '/ab-office:agree-slides', cwd: '', source: 'plugin', sourceDir: 'C:/cache/ab-office/skills/agree-slides', plugin: 'ab-office' },
    { name: 'bound', description: '绑定目录插件', command: '/pplug:bound', cwd: 'D:\\p', source: 'plugin', sourceDir: 'C:/cache/pplug/skills/bound', plugin: 'pplug' }
  ])),
  RefreshDiscoveredSkills: vi.fn(() => Promise.resolve([
    { name: 'drawio2', description: '画图工具2', command: '/drawio2', cwd: '', source: 'user', sourceDir: 'C:/h/.claude/skills/drawio2', plugin: '' }
  ])),
  OpenFileDialog: vi.fn(() => Promise.resolve(''))
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
    // data-width 透出 width attribute（stub 不声明该 prop，落入 $attrs），供弹窗尺寸分档断言
    template: '<div v-if="modelValue" class="el-dialog" :data-width="$attrs.width"><slot /><slot name="footer" /></div>',
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
  // el-radio-group 不 stub：权限模式分段组走真 EP 组件（createWrapper 已全局注册
  // ElRadioGroup/ElRadioButton，group 的 v-model 事件链路 stub 无法模拟；stub 会
  // 截断 provide 上下文致真 radio-button 改值失效）。el-radio（模板弹窗 scope 选择）
  // 无交互断言，保留 stub 简化渲染
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
    // 权限模式分段组用真 EP 组件（radio-button 的 group v-model 链路 stub 难以模拟，
    // 真组件点击即改 modelValue）；其余组件仍走 defaultStubs 简化渲染
    global: {
      plugins: [pinia],
      stubs: defaultStubs,
      components: { ElRadioGroup, ElRadioButton }
    }
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
    it('输入框默认 6 行且已去除 autosize（固定 rows + 原生拖拽调高）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const ta = wrapper.find("textarea.chat-input")
      expect(ta.element.rows).toBe(6)
      // autosize 若未移除会经 stub $attrs 透传为 DOM attribute（其 JS 接管高度与原生 resize 冲突）
      expect(ta.element.getAttribute('autosize')).toBe(null)
      // 拖拽调高是 CSS resize:vertical 的 DOM 临时态（仅会话内有效），不持久化、不入会话快照，无断言支撑
    })

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
      await store.saveChatSettings('acceptEdits', 'opus', '')
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
      // 建会话触发的草稿 watch（null→created）会把已捕获的原文存入空串键，
      // 发送成功须一并清除，防切回无会话态时已发送内容作为草稿复活
      expect(wrapper.vm.$.setupState.chatDrafts.get('')).toBeUndefined()
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

    it('输入草稿按会话隔离：切走保存、切入恢复', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      // 会话 a（chatsession-new）输入草稿
      wrapper.vm.$.setupState.inputText = '会话A的草稿'
      // 切到会话 b（chatsession-old）：输入框清空
      await wrapper.vm.$.setupState.handleSelectSession('chatsession-old')
      await flushPromises()
      expect(wrapper.vm.$.setupState.inputText).toBe('')

      // 切回会话 a：草稿恢复
      await wrapper.vm.$.setupState.handleSelectSession('chatsession-new')
      await flushPromises()
      expect(wrapper.vm.$.setupState.inputText).toBe('会话A的草稿')
    })

    it('发送成功后清当前会话草稿（切走再切回不复活）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = '待发送内容'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()
      expect(wrapper.vm.$.setupState.inputText).toBe('')
      // 强断言：发送成功立即清草稿 Map（不依赖切换时 inputText='' 的覆盖写回，
      // 后者会掩盖 delete 缺失使本用例恒真）
      expect(wrapper.vm.$.setupState.chatDrafts.get('chatsession-new')).toBeUndefined()

      // 切走再切回：草稿不应复活（发送成功已清）
      await wrapper.vm.$.setupState.handleSelectSession('chatsession-old')
      await flushPromises()
      await wrapper.vm.$.setupState.handleSelectSession('chatsession-new')
      await flushPromises()
      expect(wrapper.vm.$.setupState.inputText).toBe('')
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

    it('在途任务时切换目录：不打断在途对话直接切换（持续会话模型）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.findAll('.chat-dir-item')[1].trigger('click')
      await flushPromises()

      // 不确认、不取消：后台会话继续运行
      expect(ElMessageBox.confirm).not.toHaveBeenCalled()
      expect(CancelChatTask).not.toHaveBeenCalled()
      expect(useAiChatStore().selectedChatDirectoryId).toBe('chatdir-2')
    })

    it('在途任务时切换会话：不打断在途对话直接切换', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.vm.$.setupState.handleSelectSession('chatsession-old')
      await flushPromises()

      expect(ElMessageBox.confirm).not.toHaveBeenCalled()
      expect(CancelChatTask).not.toHaveBeenCalled()
      expect(useAiChatStore().selectedChatSessionId).toBe('chatsession-old')
    })

    it('删除在途会话：确认后先取消该会话在途轮次再删除（防孤儿消息复活）', async () => {
      ElMessageBox.confirm.mockResolvedValue()
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = 'hi'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      await wrapper.vm.$.setupState.handleDeleteSession()
      await flushPromises()

      // 一次确认（删除会话）；在途轮次取消由 store.deleteChatSession 统一处理
      expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
      expect(CancelChatTask).toHaveBeenCalledWith('chattask-1')
      expect(DeleteChatSession).toHaveBeenCalledWith('chatsession-new')
    })

    it('删除会话确认被拒时不删除不取消', async () => {
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

    it('chat-approval:request 弹窗允许：RespondChatApproval(requestId, true)', async () => {
      wrapper = createWrapper()
      await flushPromises()

      eventHandlers['chat-approval:request']({
        requestId: 'req-1', chatSessionId: 'chatsession-new', taskId: 'chattask-1',
        toolName: 'Bash', input: { command: 'mkdir d1' }, description: '创建目录 d1'
      })
      await flushPromises()

      expect(ElMessageBox.confirm).toHaveBeenCalled()
      expect(RespondChatApproval).toHaveBeenCalledWith('req-1', true)
    })

    it('chat-approval:request 弹窗拒绝：RespondChatApproval(requestId, false)', async () => {
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')
      wrapper = createWrapper()
      await flushPromises()

      eventHandlers['chat-approval:request']({
        requestId: 'req-2', chatSessionId: 'chatsession-new', taskId: 'chattask-1',
        toolName: 'Write', input: { path: 'x' }, description: ''
      })
      await flushPromises()

      expect(RespondChatApproval).toHaveBeenCalledWith('req-2', false)
    })

    it('chat-approval:cancelled 撤销队列中的待弹审批', async () => {
      ElMessageBox.confirm.mockImplementation(() => new Promise(() => {})) // 挂起不决
      wrapper = createWrapper()
      await flushPromises()

      eventHandlers['chat-approval:request']({
        requestId: 'req-3', chatSessionId: 'chatsession-new', taskId: 'chattask-1',
        toolName: 'Bash', input: {}, description: ''
      })
      await flushPromises()
      // 弹窗展示中收到 cancelled：用户应答时后端报已失效（静默降级）
      eventHandlers['chat-approval:cancelled']({ requestId: 'req-3', chatSessionId: 'chatsession-new' })
      expect(RespondChatApproval).not.toHaveBeenCalled()
      wrapper.unmount()
      wrapper = null
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

    it('管理弹窗对齐设置弹窗分档：宽 min(960px, 86vw)，正文 textarea 默认 12 行', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      await wrapper.find('.tpl-manage-item').trigger('click')
      const dialog = wrapper.find('.el-dialog')
      // 宽度分档与 SettingsPanel 的 settings-dialog 一致
      expect(dialog.attributes('data-width')).toBe('min(960px, 86vw)')
      expect(dialog.find('.tpl-manage-body').exists()).toBe(true)
      expect(dialog.find('.tpl-form textarea').element.rows).toBe(12)
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

  describe('代码块「在终端执行」（终端联动一期）', () => {
    it('assistant 代码块渲染 cmd-run 操作条按钮，命令原文可经 pre>code textContent 还原', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const btn = wrapper.find('.chat-msg-md .cmd-run-btn')
      expect(btn.exists()).toBe(true)
      expect(btn.attributes('data-cmd-run')).toBe('true')
      // hljs 高亮 span 不破坏 textContent（点击委托按此取命令原文）；
      // 围栏固定带结尾换行，handler 侧剥除（防 PTY 自动回车）
      const code = wrapper.find('.chat-msg-md .cmd-run-wrap pre code')
      expect(code.element.textContent).toBe('fmt.Println("hi")\n')
    })

    it('点击按钮：打开底部终端、terminalDir 定位会话 cwd、pending 信箱写入命令原文', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const uiStore = useUiStore()
      expect(uiStore.terminalVisible).toBe(false)
      await wrapper.find('.chat-msg-md .cmd-run-btn').trigger('click')

      expect(uiStore.terminalVisible).toBe(true)
      expect(uiStore.terminalDir).toBe('D:\\p')
      expect(uiStore.terminalPendingCmd).toMatchObject({ dir: 'D:\\p', cmd: 'fmt.Println("hi")' })
      expect(uiStore.terminalPendingCmd.seq).toBeTypeOf('number')
    })

    it('会话无 cwd：按钮置灰（is-disabled）且点击不写通道', async () => {
      GetChatSession.mockResolvedValueOnce({
        id: 'chatsession-new', directoryId: 'chatdir-1', title: '无 cwd 会话', cwd: '',
        createdAt: 1, updatedAt: 100,
        messages: [
          { role: 'assistant', content: '```bash\ngit status\n```', timestamp: 2 }
        ]
      })
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const btn = wrapper.find('.chat-msg-md .cmd-run-btn')
      expect(btn.exists()).toBe(true)
      expect(btn.classes()).toContain('is-disabled')
      // 置灰态 tooltip 同步替换为「当前会话无工作目录」说明（PRD R3）
      expect(btn.attributes('title')).toBe(CMD_RUN_BTN_TITLE_NO_CWD)

      const uiStore = useUiStore()
      await btn.trigger('click')
      expect(uiStore.terminalPendingCmd).toBeNull()
      expect(uiStore.terminalVisible).toBe(false)
    })

    it('chat-question 卡片消息不注入 cmd-run 按钮（卡片渲染不回归）', async () => {
      GetChatSession.mockResolvedValueOnce({
        id: 'chatsession-new', directoryId: 'chatdir-1', title: '最近会话', cwd: 'D:\\p',
        createdAt: 1, updatedAt: 100,
        messages: [
          { role: 'assistant', content: '```chat-question\n' + JSON.stringify({
            questions: [{ question: '选择环境', options: [{ label: '测试' }, { label: '生产' }] }]
          }) + '\n```', timestamp: 2 }
        ]
      })
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(wrapper.find('.chat-question').exists()).toBe(true)
      expect(wrapper.find('.cmd-run-btn').exists()).toBe(false)
    })
  })

  describe('权限与模型配置', () => {
    // 分段式按钮组交互辅助：点第 idx 个档位（PERMISSION_MODES 顺序：默认/接受编辑/规划/跳过确认）。
    // jsdom 对 label 合成点击不做 input 激活转发，须直接对内部 radio input 触发
    const clickPermSeg = async (wrapper, idx) => {
      const inputs = wrapper.findAll('.chat-perm-seg input[type="radio"]')
      await inputs[idx].setValue(true)
      await flushPromises()
    }

    it('权限模式分段组常驻输入盒底栏：切换持久化并调 SaveChatSettings（launcher 一并透传）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      // DOM 序：工具条只剩模型下拉（select 仅 1 个），权限模式为输入盒内分段组
      const selects = wrapper.findAll('select')
      expect(selects.length).toBe(1)
      const seg = wrapper.find('.chat-perm-seg')
      expect(seg.exists()).toBe(true)
      expect(seg.findAll('.el-radio-button').length).toBe(6)
      await clickPermSeg(wrapper, 2) // 规划

      expect(SaveChatSettings).toHaveBeenCalledWith('plan', '', '')
      expect(useAiChatStore().chatSettings.permissionMode).toBe('plan')
    })

    it('选择 bypassPermissions 时警告提示', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      await clickPermSeg(wrapper, 5) // 跳过确认（六档末位）

      expect(ElMessage.warning).toHaveBeenCalledWith(
        expect.stringContaining('跳过确认')
      )
      expect(SaveChatSettings).toHaveBeenCalledWith('bypassPermissions', '', '')
    })

    it('权限模式非默认档分段组带警示强调类（is-elevated），默认档弱化', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(wrapper.find('.chat-perm-seg').classes()).not.toContain('is-elevated')
      await clickPermSeg(wrapper, 1) // 接受编辑
      expect(wrapper.find('.chat-perm-seg').classes()).toContain('is-elevated')
    })

    it('模型下拉切换持久化（allow-create 自定义值同样生效）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const selects = wrapper.findAll('select')
      await selects[0].setValue('sonnet')
      await flushPromises()

      expect(SaveChatSettings).toHaveBeenCalledWith('default', 'sonnet', '')
    })

    it('模型下拉首选项回显 claude 设置解析的默认模型，未解析到回退通用文案', async () => {
      const { GetClaudeDefaultModel } = await import('../../../wailsjs/go/main/App')
      GetClaudeDefaultModel.mockResolvedValue('glm-5.3-flash')
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      // el-select stub 渲染原生 select，首 option 文本 = modelOptions[0].label
      const modelSelect = wrapper.findAll('select')[0]
      expect(modelSelect.element.options[0].text).toBe('默认（claude 设置：glm-5.3-flash）')

      // 空串（设置文件缺失/无配置）回退「默认模型」
      GetClaudeDefaultModel.mockResolvedValue('')
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)
      expect(wrapper.findAll('select')[0].element.options[0].text).toBe('默认模型')
    })
  })

  describe('启动命令下拉（AI 启动配置）', () => {
    it('挂载即扫描 PATH 候选并渲染下拉（默认项 + 候选 name/path + 指定路径）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(DiscoverChatClis).toHaveBeenCalled()
      const menu = wrapper.find('.launcher-menu')
      expect(menu.exists()).toBe(true)
      expect(menu.text()).toContain('claude·自动')
      expect(menu.text()).toContain('myai')
      expect(menu.text()).toContain('指定路径…')
      // 按钮默认显示 claude·自动
      expect(wrapper.find('.launcher-btn-label').text()).toBe('claude·自动')
    })

    it('点击候选切换 launcher 并持久化（透传候选 path）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      const item = wrapper.findAll('.launcher-menu .el-dropdown-item')
        .find(w => w.text().includes('myai'))
      await item.trigger('click')
      await flushPromises()

      expect(SaveChatSettings).toHaveBeenCalledWith('default', '', 'C:\\tools\\myai.cmd')
      expect(useAiChatStore().chatSettings.launcher).toBe('C:\\tools\\myai.cmd')
    })

    it('默认项清空 launcher 回默认 claude（按钮标签还原）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)
      // 预置非默认 launcher（须用面板挂载的同实例 store：createWrapper 内部
      // 另建 pinia，mount 前取到的 store 不驱动组件视图）
      const store = useAiChatStore()
      await store.saveChatSettings('default', '', 'C:\\tools\\myai.cmd')
      await flushPromises()
      expect(wrapper.find('.launcher-btn-label').text()).toContain('myai')

      await wrapper.vm.$.setupState.onLauncherCommand('__launcher_default__')
      await flushPromises()
      expect(SaveChatSettings).toHaveBeenCalledWith('default', '', '')
      expect(wrapper.find('.launcher-btn-label').text()).toBe('claude·自动')
    })

    it('指定路径：OpenFileDialog 选中 exe 回填绝对路径，按钮显示自定义文件名', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      OpenFileDialog.mockResolvedValueOnce('D:\\apps\\qwen-cli.exe')
      await wrapper.vm.$.setupState.onLauncherCommand('__launcher_pick__')
      await flushPromises()

      expect(OpenFileDialog).toHaveBeenCalledWith('选择 AI 启动程序', [
        { DisplayName: '可执行文件', Pattern: '*.exe;*.cmd;*.bat' }
      ])
      expect(SaveChatSettings).toHaveBeenCalledWith('default', '', 'D:\\apps\\qwen-cli.exe')
      expect(wrapper.find('.launcher-btn-label').text()).toBe('自定义：qwen-cli.exe')
    })

    it('指定路径：对话框取消（空串）不改动配置', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      OpenFileDialog.mockResolvedValueOnce('')
      await wrapper.vm.$.setupState.onLauncherCommand('__launcher_pick__')
      await flushPromises()

      expect(SaveChatSettings).not.toHaveBeenCalled()
    })

    it('扫描失败静默降级：下拉仅剩默认项可用', async () => {
      DiscoverChatClis.mockRejectedValueOnce(new Error('boom'))
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      expect(useAiChatStore().chatCliCandidates).toEqual([])
      const menu = wrapper.find('.launcher-menu')
      expect(menu.text()).toContain('claude·自动')
      expect(menu.text()).not.toContain('myai')
    })
  })

  describe('思考过程流式展示（仅流式，零持久化）', () => {
    it('chat-task:output thinking 增量渲染灰色思考块（正文上方），不进正文', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = '难题'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()

      eventHandlers['chat-task:output']({ taskId: 'chattask-1', text: '让我想想', thinking: true })
      await flushPromises()

      const bubble = wrapper.find('.chat-msg-bubble.is-streaming')
      const thinking = bubble.find('.chat-thinking')
      expect(thinking.exists()).toBe(true)
      expect(thinking.find('.chat-thinking-title').text()).toBe('思考中…')
      // 默认展开，思考文本在内容区
      expect(thinking.find('.chat-thinking-body').text()).toBe('让我想想')
      // 思考不进正文 markdown 区
      expect(bubble.find('.chat-msg-md').text()).not.toContain('让我想想')

      // 正文增量正常进正文区
      eventHandlers['chat-task:output']({ taskId: 'chattask-1', text: '最终答案' })
      await flushPromises()
      expect(wrapper.find('.chat-msg-bubble.is-streaming .chat-msg-md').text()).toContain('最终答案')
    })

    it('思考块点标题可收起/展开（组件本地态），done 后整体丢弃不留空块', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = '难题'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()
      eventHandlers['chat-task:output']({ taskId: 'chattask-1', text: '思考内容', thinking: true })
      await flushPromises()

      // 默认展开 → 点标题收起 → 再点展开
      // jsdom 计算样式对未挂载树不可靠：用内联 style（v-show 直写）与箭头
      // 折叠类断言展开/收起态
      const body = () => wrapper.find('.chat-thinking-body')
      const arrow = () => wrapper.find('.chat-thinking-arrow')
      expect(arrow().classes()).not.toContain('is-collapsed')
      expect(body().attributes('style') || '').not.toContain('display: none')
      await wrapper.find('.chat-thinking-head').trigger('click')
      expect(arrow().classes()).toContain('is-collapsed')
      expect(body().attributes('style')).toContain('display: none')
      await wrapper.find('.chat-thinking-head').trigger('click')
      expect(arrow().classes()).not.toContain('is-collapsed')
      expect(body().attributes('style') || '').not.toContain('display: none')

      // done：任务摘除即丢弃，气泡定型后无思考块（历史消息无此块）
      await eventHandlers['chat-task:done']({
        taskId: 'chattask-1', chatSessionId: 'chatsession-new', reply: '全文回复'
      })
      await flushPromises()
      expect(wrapper.find('.chat-thinking').exists()).toBe(false)
      expect(wrapper.find('.chat-msg-bubble.is-streaming').exists()).toBe(false)
    })

    it('无思考输出的模型气泡形态与现状一致（不留空块）', async () => {
      wrapper = createWrapper()
      await flushPromises()
      await selectFirstDirectory(wrapper)

      wrapper.vm.$.setupState.inputText = '普通问题'
      await wrapper.vm.$.setupState.handleSend()
      await flushPromises()
      eventHandlers['chat-task:output']({ taskId: 'chattask-1', text: '普通回复' })
      await flushPromises()

      expect(wrapper.find('.chat-msg-bubble.is-streaming .chat-thinking').exists()).toBe(false)
    })
  })
})


// ===== skill 斜杠补全弹层 =====
// 行首 / 触发（对齐 claude CLI）：列表项 = 命令/名称/描述/来源标签，
// ↑↓ 移动高亮、Enter/Tab 补全（不发送）、Esc 关闭；数据源懒加载（首次 / 才拉）。
// 输入区仅在选中目录后渲染，用例统一 selectFirstDirectory 前置
describe('AiChatPanel.vue skill 斜杠补全弹层', () => {
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

  // 模拟输入到指定文本（setValue 触发 input 事件，caret = 文本末尾，
  // 经 el-input stub 的 $attrs 落入 onChatInput）
  const typeText = async (text) => {
    const ta = wrapper.find('textarea.chat-input')
    await ta.setValue(text)
    return ta
  }

  // 原生派发 keydown（可断言 defaultPrevented，规避 VTU 修饰符映射差异）
  const nativeKeydown = (ta, key, extra = {}) => {
    const evt = new Event('keydown', { bubbles: true, cancelable: true })
    evt.key = key
    Object.assign(evt, extra)
    ta.element.dispatchEvent(evt)
    return evt
  }

  // 复位会话选中（目录仍在，模拟「无选中会话」空态）：pinia setup store
  // 直接写 ref，触发 availableDiscoveredSkills 的 cwd 过滤回退分支
  const deselectSession = async () => {
    const store = useAiChatStore()
    store.selectedChatSessionId = ''
    store.selectedChatSession = null
    await flushPromises()
  }

  it('行首 / 弹层出现且懒加载只调一次：列表项命令/名称/描述/来源齐备', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)

    await typeText('/')
    await flushPromises()

    const popup = wrapper.find('.skill-popup')
    expect(popup.exists()).toBe(true)
    // 已装 skill（cwd 过滤后）在前，claude 内置命令追加在后
    const cmds = wrapper.findAll('.skill-popup-cmd').map(n => n.text())
    expect(cmds).toEqual([
      '/drawio', '/release', '/ab-office:agree-slides', '/pplug:bound',
      ...CLAUDE_BUILTIN_COMMANDS.map(c => c.command)
    ])
    // 首项命令与名称、来源标签渲染
    expect(wrapper.find('.skill-popup-name').text()).toBe('drawio')
    expect(wrapper.find('.skill-popup-source').text()).toBe('用户级')
    // 懒加载：首次触发拉取一次；清空再敲不重复调用（skillsLoaded 标记）
    expect(GetDiscoveredSkills).toHaveBeenCalledTimes(1)
    await typeText('')
    await flushPromises()
    await typeText('/')
    await flushPromises()
    expect(GetDiscoveredSkills).toHaveBeenCalledTimes(1)
  })

  it('会话 cwd 过滤：project/plugin 绑定目录项按 cwd 放行；无会话仅 user + 空 cwd 插件；内置命令恒可用', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    const cmds = () => wrapper.findAll('.skill-popup-cmd').map(n => n.text())
    const builtinCmds = CLAUDE_BUILTIN_COMMANDS.map(c => c.command)

    await typeText('/')
    await flushPromises()
    // release（project cwd=D:\p）与 pplug:bound（plugin cwd=D:\p）放行；
    // other（project cwd=C:\other）滤掉；user 与空 cwd 插件恒可用；内置命令恒可用
    expect(cmds()).toEqual([
      '/drawio', '/release', '/ab-office:agree-slides', '/pplug:bound', ...builtinCmds
    ])

    await deselectSession()
    await typeText('/')
    await flushPromises()
    // 无会话 cwd：project 与绑定目录 plugin 全部滤掉，内置命令不受影响
    expect(cmds()).toEqual(['/drawio', '/ab-office:agree-slides', ...builtinCmds])
  })

  it('继续输入过滤：/dr 只剩 drawio，/zzz 无匹配弹层消失', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)

    await typeText('/dr')
    await flushPromises()
    expect(wrapper.findAll('.skill-popup-cmd').map(n => n.text())).toEqual(['/drawio'])

    await typeText('/zzz')
    await flushPromises()
    expect(wrapper.find('.skill-popup').exists()).toBe(false)
  })

  it('↑↓ 移动高亮且循环不越界', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    const ta = await typeText('/')
    await flushPromises()
    const items = () => wrapper.findAll('.skill-popup-list li')
    // 4 条已装 skill + 全部内置命令
    const total = 4 + CLAUDE_BUILTIN_COMMANDS.length
    expect(items().length).toBe(total)
    expect(items()[0].classes()).toContain('is-active')

    await ta.trigger('keydown', { key: 'ArrowDown' })
    expect(items()[1].classes()).toContain('is-active')
    for (let i = 0; i < total - 2; i++) {
      await ta.trigger('keydown', { key: 'ArrowDown' }) // 走到末项（高亮下标 total-1）
    }
    await ta.trigger('keydown', { key: 'ArrowDown' }) // 末项再下 = 回绕首项
    expect(items()[0].classes()).toContain('is-active')
    await ta.trigger('keydown', { key: 'ArrowUp' }) // 首项再上 = 回绕末项
    expect(items()[total - 1].classes()).toContain('is-active')
  })

  it('弹层打开时 Enter 补全不发送：值替换为命令+尾随空格，弹层关闭', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    const ta = await typeText('/dr')
    await flushPromises()
    expect(wrapper.find('.skill-popup').exists()).toBe(true)

    await ta.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    // 补全而非发送
    expect(RunChat).not.toHaveBeenCalled()
    expect(wrapper.vm.$.setupState.inputText).toBe('/drawio ')
    // token 因尾随空格结束，弹层关闭
    expect(wrapper.find('.skill-popup').exists()).toBe(false)
  })

  it('Tab 补全：拦默认焦点跳转 + 值替换', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    const ta = await typeText('/dr')
    await flushPromises()

    const evt = nativeKeydown(ta, 'Tab')
    expect(evt.defaultPrevented).toBe(true)
    expect(wrapper.vm.$.setupState.inputText).toBe('/drawio ')
    // 原生派发不经过 VTU 的 await 链路，等 DOM 刷新后再断言弹层关闭
    await flushPromises()
    expect(wrapper.find('.skill-popup').exists()).toBe(false)
  })

  it('Esc 关闭：拦默认、弹层消失，其后 Enter 恢复发送', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    const ta = await typeText('/')
    await flushPromises()
    expect(wrapper.find('.skill-popup').exists()).toBe(true)

    const evt = nativeKeydown(ta, 'Escape')
    expect(evt.defaultPrevented).toBe(true)
    await flushPromises()
    expect(wrapper.find('.skill-popup').exists()).toBe(false)

    // Esc 后 Enter 走发送链路（prompt = '/'）
    await ta.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(RunChat).toHaveBeenCalledTimes(1)
    expect(RunChat.mock.calls[0][1]).toBe('/')
  })

  it('IME 组合中弹层打开的 Enter：不补全不发送', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    const ta = await typeText('/')
    await flushPromises()
    expect(wrapper.find('.skill-popup').exists()).toBe(true)

    const evt = nativeKeydown(ta, 'Enter', { isComposing: true })
    expect(evt.defaultPrevented).toBe(false)
    expect(RunChat).not.toHaveBeenCalled()
    expect(wrapper.vm.$.setupState.inputText).toBe('/')
    // 弹层保持打开（未消费）
    expect(wrapper.find('.skill-popup').exists()).toBe(true)
  })

  it('补全整段替换：token 后正文保留', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    // 光标须落在 token 内（首个空白后 token 已失活）：手设 value + selectionStart
    // 再派发 input，模拟「/dr 后补字回到 token 中部」
    const ta = wrapper.find('textarea.chat-input')
    ta.element.value = '/dr 画一张图'
    ta.element.setSelectionRange(2, 2)
    await ta.trigger('input')
    await flushPromises()
    expect(wrapper.find('.skill-popup').exists()).toBe(true)

    await ta.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    // applyCompletion 吸收 token 终点空格，正文保留
    expect(wrapper.vm.$.setupState.inputText).toBe('/drawio 画一张图')
  })

  it('无已装 skill 时弹层只显示内置命令', async () => {
    GetDiscoveredSkills.mockResolvedValueOnce([])
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)

    await typeText('/')
    await flushPromises()
    // 内置命令恒可用，弹层不空
    expect(wrapper.find('.skill-popup').exists()).toBe(true)
    expect(wrapper.findAll('.skill-popup-cmd').map(n => n.text()))
      .toEqual(CLAUDE_BUILTIN_COMMANDS.map(c => c.command))
  })

  it('刷新按钮调 RefreshDiscoveredSkills 且列表更新', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    await typeText('/')
    await flushPromises()
    expect(wrapper.find('.skill-popup').exists()).toBe(true)

    await wrapper.find('.skill-popup-refresh').trigger('click')
    await flushPromises()
    expect(RefreshDiscoveredSkills).toHaveBeenCalledTimes(1)
    const cmds = wrapper.findAll('.skill-popup-cmd').map(n => n.text())
    expect(cmds).toContain('/drawio2')
    expect(cmds).not.toContain('/drawio')
  })
})

// ===== 对话状态栏 =====
// 精简版两段：模型 │ 上下文渐变进度条（分支/目录/时间段已按需求移除）。
// 数据源：chatSettings/claudeDefaultModel、会话元数据 contextTokens（后端每轮
// result 采集回写）
describe('AiChatPanel.vue 对话状态栏', () => {
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

  it('选中会话后渲染两段：模型 + 上下文条（无分支/目录/时间）', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    await flushPromises()

    const bar = wrapper.find('.chat-statusline')
    expect(bar.exists()).toBe(true)
    // 模型段：无配置时回退 claude 设置默认模型（mock 空串 → 'claude'）
    expect(wrapper.find('.chat-statusline-model').text()).toBe('claude')
    // 上下文段：46000/200k = 23%，bar 填充宽度与 token 文本
    expect(wrapper.find('.chat-statusline-ctx-text').text()).toBe('46.0k/200k')
    const fill = wrapper.find('.chat-statusline-bar-fill')
    expect(fill.attributes('style')).toContain('width: 23%')
    // 23% 处于渐变前半段（青→黄），颜色含 rgb()
    expect(fill.attributes('style')).toContain('rgb(')
    // 精简版不再渲染分支/目录/时间段
    expect(wrapper.find('.chat-statusline-branch').exists()).toBe(false)
    expect(wrapper.find('.chat-statusline-dir').exists()).toBe(false)
    expect(wrapper.find('.chat-statusline-time').exists()).toBe(false)
  })

  it('glm-5 系模型上下文窗口按 1M 计', async () => {
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    // createWrapper 内部新建 pinia，须挂载后再写执行配置（乐观更新不走后端）
    const store = useAiChatStore()
    await store.saveChatSettings('default', 'glm-5.3-flash', '')
    await flushPromises()

    // 46000 / 1M = 4.6% → 文本 46.0k/1M
    expect(wrapper.find('.chat-statusline-ctx-text').text()).toBe('46.0k/1M')
  })

  it('无 contextTokens 时进度条空态（0%）', async () => {
    GetChatSession.mockImplementation((id) => Promise.resolve({
      id, directoryId: 'chatdir-1', title: 't', cwd: 'D:\\p', createdAt: 1, updatedAt: 100,
      messages: []
    }))
    wrapper = createWrapper()
    await flushPromises()
    await selectFirstDirectory(wrapper)
    await flushPromises()

    expect(wrapper.find('.chat-statusline-ctx-text').text()).toBe('0/200k')
    expect(wrapper.find('.chat-statusline-bar-fill').attributes('style')).toContain('width: 0%')
  })

  it('未选会话不渲染状态栏', async () => {
    wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.find('.chat-statusline').exists()).toBe(false)
  })
})
