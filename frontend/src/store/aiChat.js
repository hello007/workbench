import { ref, computed } from 'vue'
import { acceptHMRUpdate, defineStore } from 'pinia'
import {
  AddChatDirectory,
  ListChatDirectories,
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
} from '../../wailsjs/go/main/App'
import { normalizePath } from '../utils/pathMatch'
import { debug } from '../utils/debug'

/**
 * AI 对话 store
 *
 * 管理 AI 对话工作台（ActivityBar「AI 对话」面板）全域状态：
 * - 目录域：侧栏常用目录列表 + 当前选中目录（PR2）
 * - 会话域：当前目录会话列表 + 选中会话消息（PR3）
 * - 任务域：在途对话任务态（queued/running + 流式回复累积），chat-task:* 事件驱动（PR3）
 * - 模板域：输入框模板（目录 + 全局合并，scope 标记）（PR3）
 * - 配置域：权限模式 + 模型（持久化记忆）（PR3）
 *
 * 错误约定：load 静默降级（debug 日志 + 保留空列表，面板仍可用）；
 * 增删改 action 错误向上冒泡，由调用方（右键菜单 / AiChatPanel）handleError 分流提示。
 */
export const useAiChatStore = defineStore('aiChat', () => {
  // ===== 目录域（PR2）=====
  // 侧栏常用目录列表（后端按 SortOrder 升序返回，形状对齐 model.ChatDirectory）
  const chatDirectories = ref([])
  // 当前选中目录项 id：驱动侧栏高亮，右侧对话区消费
  const selectedChatDirectoryId = ref('')

  // 当前选中目录项对象（未选中或已被移除时为 null）
  const selectedChatDirectory = computed(() =>
    chatDirectories.value.find(d => d.id === selectedChatDirectoryId.value) || null
  )

  /**
   * 加载常用目录列表。
   * 选中项若已不在列表中（被移除/后端清空）则复位选中，避免悬空高亮。
   */
  async function loadChatDirectories() {
    try {
      chatDirectories.value = (await ListChatDirectories()) || []
      if (selectedChatDirectoryId.value &&
          !chatDirectories.value.some(d => d.id === selectedChatDirectoryId.value)) {
        selectedChatDirectoryId.value = ''
      }
    } catch (error) {
      debug.log('加载 AI 对话目录失败:', error)
    }
  }

  /**
   * 添加常用目录（displayName 为空时后端取目录名；重复路径后端幂等返回既有项）。
   * 成功后重载列表。错误向上冒泡由调用方提示。
   * @returns {Promise<object|null>} 后端返回的目录项
   */
  async function addChatDirectory(path, displayName = '') {
    const item = await AddChatDirectory(path, displayName)
    await loadChatDirectories()
    return item
  }

  /** 修改目录项显示名（成功后重载列表）。错误向上冒泡。 */
  async function updateChatDirectory(id, displayName) {
    await UpdateChatDirectory(id, displayName)
    await loadChatDirectories()
  }

  /** 移除目录项（仅移出侧栏，不影响该目录的会话与消息）。错误向上冒泡。 */
  async function removeChatDirectory(id) {
    await RemoveChatDirectory(id)
    await loadChatDirectories()
  }

  /** 按 ids 顺序持久化拖拽重排（成功后重载列表）。错误向上冒泡。 */
  async function reorderChatDirectories(ids) {
    await ReorderChatDirectories(ids)
    await loadChatDirectories()
  }

  /**
   * 列表中是否已含该路径（右键添加前的就地查重）。
   * 规范化比较（\ -> / + 小写），与后端 filepath.Abs 去重键对齐；
   * 后端同路径幂等返回既有项，此处前置拦截给出可读提示。
   */
  function containsPath(path) {
    const norm = normalizePath(path)
    return chatDirectories.value.some(d => normalizePath(d.path) === norm)
  }

  /** 选中目录项（侧栏点击）。会话/消息/模板加载由组件 watch selectedChatDirectoryId 驱动。 */
  function selectChatDirectory(id) {
    selectedChatDirectoryId.value = id
  }

  // ===== 会话域（PR3）=====
  // 当前目录的会话列表（后端按最近活跃降序返回，形状对齐 model.ChatSession，不含消息）
  const chatSessions = ref([])
  // 当前选中会话 id
  const selectedChatSessionId = ref('')
  // 当前选中会话的元数据（不含消息；消息单独存 chatMessages）
  const selectedChatSession = ref(null)
  // 当前选中会话的消息数组（GetChatSession 加载，发送/完成时乐观更新 + 重载定型）
  const chatMessages = ref([])

  // 默认会话标题：与后端 CreateChatSession 空标题默认值一致。
  // 标题自动生成依据：标题仍为本默认值时，首条用户消息发送成功后以前 20 字覆盖。
  const CHAT_DEFAULT_SESSION_TITLE = '新会话'

  /**
   * 加载当前目录的会话列表，并默认选中最近更新的会话（列表已降序，取首个）。
   * 目录未选中或加载为空时复位会话选中并清空消息。
   */
  async function loadChatSessions() {
    const dirId = selectedChatDirectoryId.value
    if (!dirId) {
      chatSessions.value = []
      resetChatSessionSelection()
      return
    }
    let sessions
    try {
      sessions = (await ListChatSessions(dirId)) || []
    } catch (error) {
      debug.log('加载 AI 对话会话列表失败:', error)
      sessions = []
    }
    // await 期间目录可能已被切换（快速连点 A→B）：丢弃迟到响应，
    // 防旧目录的会话列表覆盖新目录视图并误选旧目录会话
    if (selectedChatDirectoryId.value !== dirId) return
    chatSessions.value = sessions
    // 选中失效（切目录/被删）或尚无选中时默认取最近更新会话
    const stillExists = chatSessions.value.some(s => s.id === selectedChatSessionId.value)
    if (!stillExists) {
      const latest = chatSessions.value[0] || null
      if (latest) {
        await selectChatSession(latest.id)
      } else {
        resetChatSessionSelection()
      }
    }
  }

  /** 复位会话选中与消息（目录切换到空态/会话全删时）。 */
  function resetChatSessionSelection() {
    selectedChatSessionId.value = ''
    selectedChatSession.value = null
    chatMessages.value = []
  }

  /**
   * 切换选中会话并加载其消息。失败静默降级（消息置空，面板仍可用）。
   * @param {string} id 会话 id
   */
  async function selectChatSession(id) {
    selectedChatSessionId.value = id
    if (!id) {
      resetChatSessionSelection()
      return
    }
    try {
      const sess = await GetChatSession(id)
      // await 期间选中可能已切走（快速连切）：丢弃迟到响应，防旧会话消息覆盖新选中会话
      if (selectedChatSessionId.value !== id) return
      selectedChatSession.value = sess
      chatMessages.value = (sess && sess.messages) || []
    } catch (error) {
      if (selectedChatSessionId.value !== id) return
      debug.log('加载 AI 对话会话失败:', error)
      selectedChatSession.value = null
      chatMessages.value = []
    }
  }

  /**
   * 在当前目录新建会话并选中（空标题由后端默认「新会话」，cwd 取目录项路径）。
   * @returns {Promise<object>} 新建的会话对象
   */
  async function createChatSession() {
    const dir = selectedChatDirectory.value
    if (!dir) throw new Error('未选择目录')
    const sess = await CreateChatSession(dir.id, '', dir.path)
    await loadChatSessions()
    await selectChatSession(sess.id)
    return sess
  }

  /**
   * 删除会话。删的是当前选中会话时切换到剩余最近会话（无则清空）。
   * @param {string} id 会话 id
   */
  async function deleteChatSession(id) {
    await DeleteChatSession(id)
    if (selectedChatSessionId.value === id) {
      selectedChatSessionId.value = ''
    }
    await loadChatSessions()
  }

  /** 修改会话标题（成功后同步本地列表项与选中元数据，避免整表重载）。 */
  async function renameChatSession(id, title) {
    await UpdateChatSessionTitle(id, title)
    const item = chatSessions.value.find(s => s.id === id)
    if (item) item.title = title
    if (selectedChatSession.value && selectedChatSession.value.id === id) {
      selectedChatSession.value.title = title
    }
  }

  // ===== 任务域（PR3）=====
  // 在途对话任务态：{ taskId, chatSessionId, status: 'queued'|'running', reply, prompt }。
  // null = 无在途任务。RunChat resolve 即置 running（后端返回时进程已启动，
  // queued/started 事件此时早已发过；先到的事件因本地任务尚未建立按 taskId 对不上被忽略）。
  const chatTask = ref(null)

  /** 是否有在途（排队/运行中）对话任务。 */
  const chatInFlight = computed(() => !!chatTask.value)

  /** 在途任务是否属于当前选中会话（流式气泡仅在该会话视图渲染）。 */
  const chatTaskInCurrentSession = computed(
    () => !!chatTask.value && chatTask.value.chatSessionId === selectedChatSessionId.value
  )

  /**
   * 发送一轮对话：RunChat 自由 prompt（权限模式/模型取当前配置）。
   * 成功后任务置 running + 追加本地 user 气泡；首条消息自动生成会话标题。
   * @param {string} prompt 用户输入
   * @returns {Promise<string>} 任务 id
   */
  async function runChat(prompt) {
    const sessionId = selectedChatSessionId.value
    if (!sessionId) throw new Error('未选择会话')
    const isFirstTurn = !chatMessages.value.some(m => m.role === 'user')
    const settings = chatSettings.value
    const taskId = await RunChat(sessionId, prompt, settings.permissionMode, settings.modelName)
    // 乐观态：resolve 即进程已启动，任务直接置 running（queued 态仅由 queued 事件
    // 兜底，且不回退已运行任务）+ user 气泡就地追加
    chatTask.value = {
      taskId,
      chatSessionId: sessionId,
      status: 'running',
      reply: '',
      prompt
    }
    chatMessages.value = [
      ...chatMessages.value,
      { role: 'user', content: prompt, timestamp: Date.now(), taskId }
    ]
    // 首条消息自动生成会话标题（默认标题未被用户改过时），失败不阻断对话。
    // Array.from 按 Unicode 码点截断，避免 UTF-16 码元截半 emoji 产生非法半代理
    if (isFirstTurn && selectedChatSession.value &&
        selectedChatSession.value.title === CHAT_DEFAULT_SESSION_TITLE) {
      const title = Array.from(prompt.replace(/\s+/g, ' ').trim()).slice(0, 20).join('')
      if (title) {
        renameChatSession(sessionId, title).catch((error) => {
          debug.log('会话标题自动生成失败:', error)
        })
      }
    }
    return taskId
  }

  /** 取消在途对话任务（杀 claude 进程树）。无在途任务返回 false。 */
  async function cancelChatTask() {
    if (!chatTask.value) return false
    const taskId = chatTask.value.taskId
    const ok = await CancelChatTask(taskId)
    // 排队取消立即生效（后端直接 emit done）；运行中取消等 done 事件收尾
    if (chatTask.value && chatTask.value.status === 'queued') {
      chatTask.value = null
    }
    return ok
  }

  /**
   * 在途任务状态恢复（事件丢失兜底）：拉 GetChatTaskState 覆盖本地任务态。
   * 任务已不存在（后端重启等）时清空任务态防卡死。
   */
  async function restoreChatTaskState() {
    const task = chatTask.value
    if (!task) return
    try {
      const state = await GetChatTaskState(task.taskId)
      if (!state) {
        chatTask.value = null
        return
      }
      if (state.running) {
        task.status = 'running'
        task.reply = state.reply || task.reply
      } else if (!state.queued) {
        // 已终止但 done 事件丢失：清任务态，消息定型交由 GetChatSession 重载
        chatTask.value = null
        if (task.chatSessionId === selectedChatSessionId.value) {
          await selectChatSession(task.chatSessionId)
        }
      }
    } catch (error) {
      debug.log('恢复对话任务状态失败:', error)
    }
  }

  // ----- chat-task:* 事件 handler（组件 EventsOn 绑定，按 taskId 对号）-----

  /** chat-task:queued：排队兜底确认。仅补排队态，不把已运行的任务回退为排队。 */
  function onChatTaskQueued(ev) {
    const task = chatTask.value
    if (!task || task.taskId !== ev?.taskId) return
    if (task.status === 'running') return
    task.status = 'queued'
  }

  /** chat-task:started：后端取得并发槽位转执行态（幂等）。 */
  function onChatTaskStarted(ev) {
    const task = chatTask.value
    if (!task || task.taskId !== ev?.taskId) return
    task.status = 'running'
  }

  /** chat-task:output：流式增量追加到本轮回复累积。 */
  function onChatTaskOutput(ev) {
    const task = chatTask.value
    if (!task || task.taskId !== ev?.taskId) return
    task.reply += ev.text || ''
  }

  /**
   * chat-task:done：任务定型。清理任务态、重载会话列表（活跃时间排序变化）、
   * done 会话为当前选中会话时重载消息（与后端持久化对齐，覆盖乐观态）。
   * @param {object} result model.ChatTaskRunResult 形状
   * @returns {boolean} 是否为本 store 持有的任务（供调用方决定是否提示）
   */
  async function handleChatTaskDone(result) {
    const task = chatTask.value
    if (!task || task.taskId !== result?.taskId) return false
    const sessionId = task.chatSessionId
    chatTask.value = null
    await loadChatSessions()
    if (sessionId === selectedChatSessionId.value) {
      await selectChatSession(sessionId)
    }
    return true
  }

  // ===== 模板域（PR3）=====
  // 模板列表：后端合并返回「当前目录模板 + 全局模板」（scope 标记，目录在前）
  const chatTemplates = ref([])

  /**
   * 加载模板列表（按当前选中目录过滤目录域模板）。
   * 目录未选中时仅加载全局模板。
   */
  async function loadChatTemplates() {
    try {
      chatTemplates.value = (await ListChatTemplates(selectedChatDirectoryId.value)) || []
    } catch (error) {
      debug.log('加载 AI 对话模板失败:', error)
      chatTemplates.value = []
    }
  }

  /** 新增模板（成功后重载列表）。错误向上冒泡。 */
  async function addChatTemplate(scope, directoryId, name, content) {
    await AddChatTemplate(scope, directoryId, name, content)
    await loadChatTemplates()
  }

  /** 修改模板名与内容（成功后重载列表）。错误向上冒泡。 */
  async function updateChatTemplate(id, name, content) {
    await UpdateChatTemplate(id, name, content)
    await loadChatTemplates()
  }

  /** 删除模板（成功后重载列表）。错误向上冒泡。 */
  async function removeChatTemplate(id) {
    await RemoveChatTemplate(id)
    await loadChatTemplates()
  }

  /** 按 scope 分组（下拉两组展示）：{ directory: [...], global: [...] }。 */
  const chatTemplateGroups = computed(() => ({
    directory: chatTemplates.value.filter(t => t.scope === 'directory'),
    global: chatTemplates.value.filter(t => t.scope === 'global')
  }))

  // ===== 配置域（PR3）=====
  // 执行配置（权限模式 + 模型），持久化于后端 settings.json
  const chatSettings = ref({ permissionMode: 'default', modelName: '' })

  /** 加载执行配置（后端缺失/损坏已回默认，前端不再兜底）。 */
  async function loadChatSettings() {
    try {
      const settings = await GetChatSettings()
      if (settings) {
        chatSettings.value = {
          permissionMode: settings.permissionMode || 'default',
          modelName: settings.modelName || ''
        }
      }
    } catch (error) {
      debug.log('加载 AI 对话设置失败:', error)
    }
  }

  /**
   * 保存执行配置：乐观更新本地 + 持久化。错误向上冒泡（调用方提示，
   * 本地值保留乐观态供本轮对话使用）。
   */
  async function saveChatSettings(permissionMode, modelName) {
    chatSettings.value = { permissionMode, modelName }
    await SaveChatSettings(permissionMode, modelName)
  }

  return {
    // 目录域
    chatDirectories,
    selectedChatDirectoryId,
    selectedChatDirectory,
    loadChatDirectories,
    addChatDirectory,
    updateChatDirectory,
    removeChatDirectory,
    reorderChatDirectories,
    containsPath,
    selectChatDirectory,
    // 会话域
    chatSessions,
    selectedChatSessionId,
    selectedChatSession,
    chatMessages,
    loadChatSessions,
    selectChatSession,
    createChatSession,
    deleteChatSession,
    renameChatSession,
    // 任务域
    chatTask,
    chatInFlight,
    chatTaskInCurrentSession,
    runChat,
    cancelChatTask,
    restoreChatTaskState,
    onChatTaskQueued,
    onChatTaskStarted,
    onChatTaskOutput,
    handleChatTaskDone,
    // 模板域
    chatTemplates,
    chatTemplateGroups,
    loadChatTemplates,
    addChatTemplate,
    updateChatTemplate,
    removeChatTemplate,
    // 配置域
    chatSettings,
    loadChatSettings,
    saveChatSettings
  }
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useAiChatStore, import.meta.hot))
}
