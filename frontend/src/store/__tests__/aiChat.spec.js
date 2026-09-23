import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'
import { useAiChatStore } from '..'
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
  UpdateChatSessionTitle,
  RunChat,
  CancelChatTask,
  GetChatTaskState,
  ListChatTemplates,
  AddChatTemplate,
  UpdateChatTemplate,
  RemoveChatTemplate,
  GetChatSettings,
  SaveChatSettings
} from '../../../wailsjs/go/main/App'

// AI 对话全域绑定 mock（形状对齐 model.ChatDirectory / ChatSession / ChatTemplate
// / ChatSettings 与 App.d.ts 签名）。vi.mock 提升到文件顶部，上方静态 import
// 拿到的即这里的 mock。
vi.mock('../../../wailsjs/go/main/App', () => ({
  ListChatDirectories: vi.fn(() => Promise.resolve([
    { id: 'chatdir-1', path: 'C:\\projects\\app', displayName: '项目管理', sortOrder: 0, createdAt: 1000 },
    { id: 'chatdir-2', path: 'C:\\work\\server', displayName: 'server', sortOrder: 1, createdAt: 2000 }
  ])),
  AddChatDirectory: vi.fn(() => Promise.resolve({
    id: 'chatdir-3', path: 'C:\\new\\dir', displayName: 'dir', sortOrder: 2, createdAt: 3000
  })),
  UpdateChatDirectory: vi.fn(() => Promise.resolve()),
  RemoveChatDirectory: vi.fn(() => Promise.resolve()),
  ReorderChatDirectories: vi.fn(() => Promise.resolve()),
  ListChatSessions: vi.fn(() => Promise.resolve([
    // 后端按最近活跃降序返回：首个 = 最近更新会话
    { id: 'chatsession-new', directoryId: 'chatdir-1', title: '最近会话', cwd: 'C:\\projects\\app', createdAt: 2, updatedAt: 200 },
    { id: 'chatsession-old', directoryId: 'chatdir-1', title: '旧会话', cwd: 'C:\\projects\\app', createdAt: 1, updatedAt: 100 }
  ])),
  CreateChatSession: vi.fn(() => Promise.resolve({
    id: 'chatsession-new2', directoryId: 'chatdir-1', title: '新会话', cwd: 'C:\\projects\\app', createdAt: 3, updatedAt: 300
  })),
  GetChatSession: vi.fn((id) => Promise.resolve({
    id, directoryId: 'chatdir-1', title: '会话', cwd: 'C:\\projects\\app', createdAt: 1, updatedAt: 100,
    messages: [
      { role: 'user', content: '第一问', timestamp: 1, taskId: 't1' },
      { role: 'assistant', content: '第一答', timestamp: 2, taskId: 't1' }
    ]
  })),
  DeleteChatSession: vi.fn(() => Promise.resolve()),
  UpdateChatSessionTitle: vi.fn(() => Promise.resolve()),
  RunChat: vi.fn(() => Promise.resolve('chattask-1')),
  CancelChatTask: vi.fn(() => Promise.resolve(true)),
  GetChatTaskState: vi.fn(() => Promise.resolve(null)),
  ListChatTemplates: vi.fn(() => Promise.resolve([
    { id: 'tpl-dir', scope: 'directory', directoryId: 'chatdir-1', name: '目录模板', content: 'D' },
    { id: 'tpl-global', scope: 'global', directoryId: '', name: '全局模板', content: 'G' }
  ])),
  AddChatTemplate: vi.fn(() => Promise.resolve({
    id: 'tpl-new', scope: 'directory', directoryId: 'chatdir-1', name: 'n', content: 'c'
  })),
  UpdateChatTemplate: vi.fn(() => Promise.resolve()),
  RemoveChatTemplate: vi.fn(() => Promise.resolve()),
  GetChatSettings: vi.fn(() => Promise.resolve({ permissionMode: 'default', modelName: '' })),
  SaveChatSettings: vi.fn(() => Promise.resolve())
}))

vi.mock('../../../utils/debug', () => ({
  debug: { log: vi.fn(), error: vi.fn(), warn: vi.fn() }
}))

// 选中目录（chatdir-1）的标准前置：目录加载 + 选中 + 会话列表加载
async function setupWithDirectory() {
  const store = useAiChatStore()
  await store.loadChatDirectories()
  store.selectChatDirectory('chatdir-1')
  await store.loadChatSessions()
  return store
}

describe('aiChat store', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // 每个 it 独立 pinia 实例，避免 store 状态跨用例污染
    setActivePinia(createPinia())
  })

  // ===== 目录域（PR2）=====

  it('loadChatDirectories 加载列表，返回 null 时降级空数组', async () => {
    const store = useAiChatStore()
    await store.loadChatDirectories()
    expect(store.chatDirectories.length).toBe(2)
    expect(store.chatDirectories[0].displayName).toBe('项目管理')

    ListChatDirectories.mockResolvedValueOnce(null)
    await store.loadChatDirectories()
    expect(store.chatDirectories).toEqual([])
  })

  it('loadChatDirectories 加载失败静默降级不抛错', async () => {
    ListChatDirectories.mockRejectedValueOnce(new Error('boom'))
    const store = useAiChatStore()
    await expect(store.loadChatDirectories()).resolves.toBeUndefined()
    expect(store.chatDirectories).toEqual([])
  })

  it('选中项被移除后 loadChatDirectories 复位选中', async () => {
    const store = useAiChatStore()
    await store.loadChatDirectories()
    store.selectChatDirectory('chatdir-2')
    expect(store.selectedChatDirectoryId).toBe('chatdir-2')

    // 列表重载后不再含 chatdir-2（模拟移除后的重载）
    ListChatDirectories.mockResolvedValueOnce([
      { id: 'chatdir-1', path: 'C:\\projects\\app', displayName: '项目管理', sortOrder: 0, createdAt: 1000 }
    ])
    await store.loadChatDirectories()
    expect(store.selectedChatDirectoryId).toBe('')
    expect(store.selectedChatDirectory).toBeNull()
  })

  it('addChatDirectory 调后端并触发重载，返回目录项', async () => {
    const store = useAiChatStore()
    await store.loadChatDirectories()

    const item = await store.addChatDirectory('C:\\new\\dir', '')
    expect(AddChatDirectory).toHaveBeenCalledWith('C:\\new\\dir', '')
    expect(item.id).toBe('chatdir-3')
    // mock 无状态：重载返回基准 2 项，断言「初始加载 + add 后重载」共两次调用
    expect(ListChatDirectories).toHaveBeenCalledTimes(2)
    expect(store.chatDirectories.length).toBe(2)
  })

  it('addChatDirectory 后端报错时向上冒泡', async () => {
    AddChatDirectory.mockRejectedValueOnce({ message: '目录不存在: x' })
    const store = useAiChatStore()
    await expect(store.addChatDirectory('x', '')).rejects.toEqual({ message: '目录不存在: x' })
  })

  it('updateChatDirectory / removeChatDirectory / reorderChatDirectories 调后端并重载', async () => {
    const store = useAiChatStore()
    await store.loadChatDirectories()

    await store.updateChatDirectory('chatdir-1', '新名')
    expect(UpdateChatDirectory).toHaveBeenCalledWith('chatdir-1', '新名')

    await store.removeChatDirectory('chatdir-1')
    expect(RemoveChatDirectory).toHaveBeenCalledWith('chatdir-1')

    await store.reorderChatDirectories(['chatdir-2', 'chatdir-1'])
    expect(ReorderChatDirectories).toHaveBeenCalledWith(['chatdir-2', 'chatdir-1'])
  })

  it('containsPath 规范化路径查重（分隔符与大小写不敏感）', async () => {
    const store = useAiChatStore()
    await store.loadChatDirectories()

    expect(store.containsPath('C:\\projects\\app')).toBe(true)
    expect(store.containsPath('C:/PROJECTS/APP')).toBe(true)
    expect(store.containsPath('C:/other')).toBe(false)
    expect(store.containsPath('')).toBe(false)
  })

  it('selectChatDirectory 更新选中且 selectedChatDirectory 指向对应项', async () => {
    const store = useAiChatStore()
    await store.loadChatDirectories()

    store.selectChatDirectory('chatdir-2')
    expect(store.selectedChatDirectoryId).toBe('chatdir-2')
    expect(store.selectedChatDirectory.displayName).toBe('server')

    store.selectChatDirectory('chatdir-missing')
    expect(store.selectedChatDirectory).toBeNull()
  })

  // ===== 会话域（PR3）=====

  it('loadChatSessions 默认选中最近更新会话并加载其消息', async () => {
    const store = await setupWithDirectory()
    expect(ListChatSessions).toHaveBeenCalledWith('chatdir-1')
    expect(store.selectedChatSessionId).toBe('chatsession-new')
    expect(GetChatSession).toHaveBeenCalledWith('chatsession-new')
    expect(store.chatMessages.length).toBe(2)
    expect(store.chatMessages[0].content).toBe('第一问')
  })

  it('loadChatSessions 目录未选中时复位会话选中', async () => {
    const store = useAiChatStore()
    await store.loadChatSessions()
    expect(store.selectedChatSessionId).toBe('')
    expect(store.chatMessages).toEqual([])
    expect(ListChatSessions).not.toHaveBeenCalled()
  })

  it('loadChatSessions 列表为空时清空选中与消息', async () => {
    ListChatSessions.mockResolvedValueOnce([])
    const store = await setupWithDirectory()
    expect(store.selectedChatSessionId).toBe('')
    expect(store.chatSessions).toEqual([])
    expect(GetChatSession).not.toHaveBeenCalled()
  })

  it('loadChatSessions 加载失败静默降级为空列表', async () => {
    ListChatSessions.mockRejectedValueOnce(new Error('boom'))
    const store = await setupWithDirectory()
    expect(store.chatSessions).toEqual([])
  })

  it('loadChatSessions await 期间目录已切换时丢弃迟到响应', async () => {
    const store = await setupWithDirectory()
    const baseline = store.chatSessions.map(s => s.id)
    let resolveLate
    ListChatSessions.mockImplementationOnce(() => new Promise((r) => { resolveLate = r }))
    const pending = store.loadChatSessions()
    // await 期间快速切到另一目录，随后旧目录的空列表响应迟到
    store.selectChatDirectory('chatdir-2')
    resolveLate([])
    await pending
    // 迟到响应被丢弃：不覆盖新目录视图、不误选旧目录会话
    expect(store.chatSessions.map(s => s.id)).toEqual(baseline)
    expect(store.selectedChatSessionId).toBe('chatsession-new')
  })

  it('selectChatSession 切换会话并加载对应消息', async () => {
    const store = await setupWithDirectory()
    GetChatSession.mockClear()
    await store.selectChatSession('chatsession-old')
    expect(GetChatSession).toHaveBeenCalledWith('chatsession-old')
    expect(store.selectedChatSessionId).toBe('chatsession-old')
    expect(store.selectedChatSession.id).toBe('chatsession-old')
  })

  it('selectChatSession 加载失败静默降级（消息清空面板可用）', async () => {
    const store = await setupWithDirectory()
    GetChatSession.mockRejectedValueOnce({ code: 'E_CHAT_SESSION_NOT_FOUND', message: '会话不存在' })
    await store.selectChatSession('chatsession-missing')
    expect(store.selectedChatSessionId).toBe('chatsession-missing')
    expect(store.chatMessages).toEqual([])
  })

  it('selectChatSession 迟到响应不覆盖新选中会话的消息', async () => {
    const store = await setupWithDirectory()
    let resolveLate
    GetChatSession.mockImplementationOnce(
      (id) => new Promise((r) => {
        resolveLate = () => r({ id, title: '旧会话', messages: [{ role: 'user', content: '迟到旧消息', timestamp: 1 }] })
      })
    )
    const late = store.selectChatSession('chatsession-old')
    // 快速连切：新选中会话先完成加载
    await store.selectChatSession('chatsession-new')
    resolveLate()
    await late
    // 迟到的旧会话响应被丢弃：消息保持新选中会话内容
    expect(store.selectedChatSessionId).toBe('chatsession-new')
    expect(store.chatMessages[0].content).toBe('第一问')
  })

  it('createChatSession 以当前目录建会话并选中', async () => {
    const store = await setupWithDirectory()
    const sess = await store.createChatSession()
    expect(CreateChatSession).toHaveBeenCalledWith('chatdir-1', '', 'C:\\projects\\app')
    expect(sess.id).toBe('chatsession-new2')
    expect(store.selectedChatSessionId).toBe('chatsession-new2')
  })

  it('createChatSession 未选目录时报错', async () => {
    const store = useAiChatStore()
    await expect(store.createChatSession()).rejects.toThrow('未选择目录')
  })

  it('deleteChatSession 删除当前选中会话后回退到剩余最近会话', async () => {
    const store = await setupWithDirectory()
    expect(store.selectedChatSessionId).toBe('chatsession-new')

    // 删除后 mock 重载返回剩余 1 条（后端按 UpdatedAt 降序）
    DeleteChatSession.mockImplementationOnce(async () => {
      ListChatSessions.mockResolvedValueOnce([
        { id: 'chatsession-old', directoryId: 'chatdir-1', title: '旧会话', cwd: 'C:\\p', createdAt: 1, updatedAt: 100 }
      ])
    })
    await store.deleteChatSession('chatsession-new')
    expect(DeleteChatSession).toHaveBeenCalledWith('chatsession-new')
    expect(store.selectedChatSessionId).toBe('chatsession-old')
  })

  it('deleteChatSession 删除非选中会话不影响当前选中', async () => {
    const store = await setupWithDirectory()
    await store.deleteChatSession('chatsession-old')
    expect(store.selectedChatSessionId).toBe('chatsession-new')
  })

  it('renameChatSession 调后端并同步本地列表与选中元数据', async () => {
    const store = await setupWithDirectory()
    await store.renameChatSession('chatsession-new', '改名')
    expect(UpdateChatSessionTitle).toHaveBeenCalledWith('chatsession-new', '改名')
    expect(store.chatSessions.find(s => s.id === 'chatsession-new').title).toBe('改名')
    expect(store.selectedChatSession.title).toBe('改名')
  })

  // ===== 任务域（PR3）=====

  it('runChat 以当前配置发送并乐观置 running 任务态与 user 气泡', async () => {
    const store = await setupWithDirectory()
    await store.saveChatSettings('acceptEdits', 'opus')

    const taskId = await store.runChat('帮我看看这个项目')
    expect(RunChat).toHaveBeenCalledWith('chatsession-new', '帮我看看这个项目', 'acceptEdits', 'opus')
    expect(taskId).toBe('chattask-1')
    expect(store.chatInFlight).toBe(true)
    expect(store.chatTask.taskId).toBe('chattask-1')
    expect(store.chatTask.chatSessionId).toBe('chatsession-new')
    // RunChat resolve 即进程已启动，任务直接为 running（非排队态）
    expect(store.chatTask.status).toBe('running')
    // 乐观 user 气泡追加（GetChatSession 基准 2 条 + 1 条乐观）
    expect(store.chatMessages.length).toBe(3)
    expect(store.chatMessages[2].role).toBe('user')
    expect(store.chatMessages[2].content).toBe('帮我看看这个项目')
  })

  it('runChat 首条消息且默认标题时自动生成会话标题', async () => {
    // 预置空消息会话（首条消息场景）
    GetChatSession.mockResolvedValueOnce({
      id: 'chatsession-new', directoryId: 'chatdir-1', title: '新会话', cwd: 'C:\\p',
      createdAt: 1, updatedAt: 100, messages: []
    })
    const store = await setupWithDirectory()
    await store.runChat('这是一个超过二十个字的需求请帮我生成会话标题吧朋友')
    await flushPromises()
    expect(UpdateChatSessionTitle).toHaveBeenCalledWith(
      'chatsession-new',
      '这是一个超过二十个字的需求请帮我生成会话'
    )
  })

  it('runChat 会话标题按 Unicode 码点截断（emoji 不截半产生非法半代理）', async () => {
    GetChatSession.mockResolvedValueOnce({
      id: 'chatsession-new', directoryId: 'chatdir-1', title: '新会话', cwd: 'C:\\p',
      createdAt: 1, updatedAt: 100, messages: []
    })
    const store = await setupWithDirectory()
    await store.runChat('😀'.repeat(25))
    await flushPromises()
    const calledTitle = UpdateChatSessionTitle.mock.calls[0][1]
    expect(Array.from(calledTitle).length).toBe(20)
    expect(calledTitle).toBe('😀'.repeat(20))
  })

  it('runChat 非首条消息或标题已改时不自动生成标题', async () => {
    const store = await setupWithDirectory()
    // GetChatSession 返回标题已非默认
    GetChatSession.mockResolvedValueOnce({
      id: 'chatsession-new', directoryId: 'chatdir-1', title: '已有标题', cwd: 'C:\\p',
      createdAt: 1, updatedAt: 100,
      messages: [{ role: 'user', content: '旧消息', timestamp: 1, taskId: 't0' }]
    })
    await store.selectChatSession('chatsession-new')
    await store.runChat('第二轮')
    await Promise.resolve()
    await Promise.resolve()
    expect(UpdateChatSessionTitle).not.toHaveBeenCalled()
  })

  it('runChat 未选会话时报错', async () => {
    const store = useAiChatStore()
    await expect(store.runChat('hi')).rejects.toThrow('未选择会话')
  })

  it('queued/started/output 先于 RunChat resolve 到达时被安全忽略，resolve 后为 running', async () => {
    const store = await setupWithDirectory()
    let resolveRun
    RunChat.mockImplementationOnce(() => new Promise((r) => { resolveRun = r }))
    const pending = store.runChat('hi')
    // 真实时序：后端先 emit 事件再返回 bound call，此时本地任务尚未建立
    store.onChatTaskQueued({ taskId: 'chattask-1' })
    store.onChatTaskStarted({ taskId: 'chattask-1', chatSessionId: 'chatsession-new' })
    store.onChatTaskOutput({ taskId: 'chattask-1', text: '早到增量' })
    expect(store.chatTask).toBeNull()
    resolveRun('chattask-1')
    await pending
    // resolve 即 running，早到的增量不被误记
    expect(store.chatTask.status).toBe('running')
    expect(store.chatTask.reply).toBe('')
  })

  it('onChatTaskStarted / onChatTaskOutput 按 taskId 对号更新，queued 不回退运行态', async () => {
    const store = await setupWithDirectory()
    await store.runChat('hi')
    // resolve 即 running；不匹配 taskId 的 started 不改状态
    expect(store.chatTask.status).toBe('running')
    store.onChatTaskStarted({ taskId: 'other' })
    expect(store.chatTask.status).toBe('running')

    // 已 running 时同任务 queued 事件不回退为排队态（F2 回归）
    store.onChatTaskQueued({ taskId: 'chattask-1' })
    expect(store.chatTask.status).toBe('running')

    // started 幂等
    store.onChatTaskStarted({ taskId: 'chattask-1' })
    expect(store.chatTask.status).toBe('running')

    store.onChatTaskOutput({ taskId: 'chattask-1', text: '你' })
    store.onChatTaskOutput({ taskId: 'chattask-1', text: '好' })
    store.onChatTaskOutput({ taskId: 'other', text: '忽略' })
    expect(store.chatTask.reply).toBe('你好')
  })

  it('handleChatTaskDone 定型：清任务态 + 重载当前会话消息 + 重载列表', async () => {
    const store = await setupWithDirectory()
    await store.runChat('hi')
    store.onChatTaskOutput({ taskId: 'chattask-1', text: '回复' })
    GetChatSession.mockClear()

    const handled = await store.handleChatTaskDone({
      taskId: 'chattask-1', chatSessionId: 'chatsession-new', reply: '回复全文', exitCode: 0
    })
    expect(handled).toBe(true)
    expect(store.chatInFlight).toBe(false)
    expect(ListChatSessions).toHaveBeenCalled()
    // 当前会话消息经 GetChatSession 重载定型（乐观态被覆盖）
    expect(GetChatSession).toHaveBeenCalledWith('chatsession-new')
    expect(store.chatMessages[store.chatMessages.length - 1].role).toBe('assistant')
  })

  it('handleChatTaskDone 不匹配 taskId 返回 false 且不清任务态', async () => {
    const store = await setupWithDirectory()
    await store.runChat('hi')
    const handled = await store.handleChatTaskDone({ taskId: 'other' })
    expect(handled).toBe(false)
    expect(store.chatInFlight).toBe(true)
  })

  it('cancelChatTask 运行中等 done 收尾，排队态兜底立即清任务态', async () => {
    const store = await setupWithDirectory()
    // RunChat resolve 即 running：取消后任务态保留，等 done 事件收尾
    await store.runChat('hi')
    await store.cancelChatTask()
    expect(CancelChatTask).toHaveBeenCalledWith('chattask-1')
    expect(store.chatInFlight).toBe(true)

    // done 到达后清理
    await store.handleChatTaskDone({ taskId: 'chattask-1', canceled: true })
    expect(store.chatInFlight).toBe(false)

    // 排队态（兜底路径）：取消立即清任务态
    store.chatTask = {
      taskId: 'chattask-2', chatSessionId: 'chatsession-new', status: 'queued', reply: '', prompt: 'p'
    }
    await store.cancelChatTask()
    expect(CancelChatTask).toHaveBeenCalledWith('chattask-2')
    expect(store.chatInFlight).toBe(false)
  })

  it('cancelChatTask 无在途任务返回 false', async () => {
    const store = await setupWithDirectory()
    expect(await store.cancelChatTask()).toBe(false)
    expect(CancelChatTask).not.toHaveBeenCalled()
  })

  it('restoreChatTaskState 恢复运行中任务的回复累积', async () => {
    const store = await setupWithDirectory()
    await store.runChat('hi')
    GetChatTaskState.mockResolvedValueOnce({
      taskId: 'chattask-1', running: true, queued: false, reply: '服务端累积回复'
    })
    await store.restoreChatTaskState()
    expect(store.chatTask.status).toBe('running')
    expect(store.chatTask.reply).toBe('服务端累积回复')
  })

  it('restoreChatTaskState 任务已不存在时清空任务态', async () => {
    const store = await setupWithDirectory()
    await store.runChat('hi')
    GetChatTaskState.mockResolvedValueOnce(null)
    await store.restoreChatTaskState()
    expect(store.chatInFlight).toBe(false)
  })

  it('restoreChatTaskState 终态且 done 丢失时清任务态并重载消息', async () => {
    const store = await setupWithDirectory()
    await store.runChat('hi')
    GetChatSession.mockClear()
    GetChatTaskState.mockResolvedValueOnce({
      taskId: 'chattask-1', running: false, queued: false, reply: ''
    })
    await store.restoreChatTaskState()
    expect(store.chatInFlight).toBe(false)
    expect(GetChatSession).toHaveBeenCalledWith('chatsession-new')
  })

  it('restoreChatTaskState 无任务时不调后端', async () => {
    const store = await setupWithDirectory()
    await store.restoreChatTaskState()
    expect(GetChatTaskState).not.toHaveBeenCalled()
  })

  // ===== 模板域（PR3）=====

  it('loadChatTemplates 按选中目录加载合并列表', async () => {
    const store = await setupWithDirectory()
    await store.loadChatTemplates()
    expect(ListChatTemplates).toHaveBeenCalledWith('chatdir-1')
    expect(store.chatTemplates.length).toBe(2)
  })

  it('chatTemplateGroups 按 scope 分组（目录域在前）', async () => {
    const store = await setupWithDirectory()
    await store.loadChatTemplates()
    expect(store.chatTemplateGroups.directory.map(t => t.id)).toEqual(['tpl-dir'])
    expect(store.chatTemplateGroups.global.map(t => t.id)).toEqual(['tpl-global'])
  })

  it('addChatTemplate 调后端并重载列表', async () => {
    const store = await setupWithDirectory()
    await store.addChatTemplate('directory', 'chatdir-1', 'n', 'c')
    expect(AddChatTemplate).toHaveBeenCalledWith('directory', 'chatdir-1', 'n', 'c')
    expect(ListChatTemplates).toHaveBeenCalled()
  })

  it('updateChatTemplate / removeChatTemplate 调后端并重载', async () => {
    const store = await setupWithDirectory()
    await store.updateChatTemplate('tpl-dir', '新名', '新内容')
    expect(UpdateChatTemplate).toHaveBeenCalledWith('tpl-dir', '新名', '新内容')

    await store.removeChatTemplate('tpl-dir')
    expect(RemoveChatTemplate).toHaveBeenCalledWith('tpl-dir')
  })

  it('loadChatTemplates 加载失败静默降级空列表', async () => {
    ListChatTemplates.mockRejectedValueOnce(new Error('boom'))
    const store = await setupWithDirectory()
    await store.loadChatTemplates()
    expect(store.chatTemplates).toEqual([])
  })

  // ===== 配置域（PR3）=====

  it('loadChatSettings 读取后端配置', async () => {
    const store = useAiChatStore()
    await store.loadChatSettings()
    expect(store.chatSettings).toEqual({ permissionMode: 'default', modelName: '' })

    GetChatSettings.mockResolvedValueOnce({ permissionMode: 'plan', modelName: 'opus' })
    await store.loadChatSettings()
    expect(store.chatSettings).toEqual({ permissionMode: 'plan', modelName: 'opus' })
  })

  it('loadChatSettings 加载失败保留本地默认', async () => {
    GetChatSettings.mockRejectedValueOnce(new Error('boom'))
    const store = useAiChatStore()
    await store.loadChatSettings()
    expect(store.chatSettings).toEqual({ permissionMode: 'default', modelName: '' })
  })

  it('saveChatSettings 乐观更新本地并持久化', async () => {
    const store = useAiChatStore()
    await store.saveChatSettings('bypassPermissions', 'sonnet')
    expect(store.chatSettings).toEqual({ permissionMode: 'bypassPermissions', modelName: 'sonnet' })
    expect(SaveChatSettings).toHaveBeenCalledWith('bypassPermissions', 'sonnet')
  })

  it('saveChatSettings 后端报错向上冒泡（本地保留乐观态）', async () => {
    SaveChatSettings.mockRejectedValueOnce({ message: '权限模式无效: x' })
    const store = useAiChatStore()
    await expect(store.saveChatSettings('bad', '')).rejects.toEqual({ message: '权限模式无效: x' })
    expect(store.chatSettings.permissionMode).toBe('bad')
  })
})

// ===== 选择题卡片域 =====
// chat-question 已答标记：提交后置灰防重复作答；key 按会话+任务隔离；
// 仅内存态（应用重启恢复为未答，PRD B8 决策不落盘）
describe('aiChatStore 选择题卡片域', () => {
  it('默认未答；markChatQuestionAnswered 后同会话同任务判已答', () => {
    const store = useAiChatStore()
    expect(store.isChatQuestionAnswered('s1', 't1')).toBe(false)
    store.markChatQuestionAnswered('s1', 't1')
    expect(store.isChatQuestionAnswered('s1', 't1')).toBe(true)
  })

  it('已答标记按会话/任务隔离：他任务、他会话不受影响', () => {
    const store = useAiChatStore()
    store.markChatQuestionAnswered('s1', 't1')
    expect(store.isChatQuestionAnswered('s1', 't2')).toBe(false)
    expect(store.isChatQuestionAnswered('s2', 't1')).toBe(false)
  })

  it('空 sessionId/taskId 入参与查询均安全（历史消息 taskId 可能缺失）', () => {
    const store = useAiChatStore()
    store.markChatQuestionAnswered('', 't1')
    store.markChatQuestionAnswered('s1', '')
    expect(store.isChatQuestionAnswered('', 't1')).toBe(false)
    expect(store.isChatQuestionAnswered('s1', '')).toBe(false)
    expect(store.isChatQuestionAnswered(undefined, undefined)).toBe(false)
  })
})
