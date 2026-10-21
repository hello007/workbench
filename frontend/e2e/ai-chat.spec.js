/**
 * AI 对话工作台 E2E（mock 后端 chat-task:* 事件流，测「多会话对话 UI 链路正确性」）。
 *
 * 真实链路中 RunChat 起 claude CLI 子进程并经 Wails 事件推送
 * queued/started/output/done；E2E 以 {__value__, __preEvents__, __events__} 描述符
 * mock 该异步事件流——queued/started 先于 resolve 派发（对齐后端「先 emit 再返回」
 * 时序，wails-init.js 派发至 EventsOn 注册的回调），覆盖核心链路：
 * 1. 对话闭环：添加目录 -> 目录出现在侧栏 -> 选中（自动选中最近会话）->
 *    发送消息（RunChat 参数正确）-> 流式输出渲染 -> done 后消息定型 + 会话标题自动生成
 * 2. 会话切换：会话下拉（类 /resume）查看会话列表 -> 切换到历史会话 -> 消息随之加载
 *
 * mock 数据形状对齐 model.ChatDirectory / ChatSession / ChatMessage /
 * ChatTaskRunResult（见 model/chat_session.go json tag）。
 * done 事件 delayMs 延后派发，为「流式中」断言留出确定性观察窗口
 * （模式同 ai-function.spec.js 取消链路的 done 延迟派发）。
 */
import { test, expect, getWailsCalls } from './fixtures'

const PROMPT = '帮我梳理这个项目的目录结构'
const REPLY = '这是 AI 的回复：**加粗要点**。\n'

/** AI 对话侧栏目录项（形状对齐 model.ChatDirectory） */
const CHAT_DIR = {
  id: 'chatdir-1',
  path: 'D:/e2e-demo/demo-repo',
  displayName: '示例项目',
  sortOrder: 0,
  createdAt: 1757400000000
}

/** 初始会话：消息为空、无 claude session id（首通对话前） */
const EMPTY_SESSION = {
  id: 'chatsession-e2e-1',
  directoryId: 'chatdir-1',
  title: '新会话',
  cwd: 'D:/e2e-demo/demo-repo',
  claudeSessionId: '',
  createdAt: 1757400000000,
  updatedAt: 1757400000000,
  messages: []
}

/** done 重载后的定型会话：user + assistant 两条消息，claude session id 已产生 */
const FINAL_SESSION = {
  ...EMPTY_SESSION,
  claudeSessionId: 'claude-e2e-1',
  updatedAt: 1757400060000,
  messages: [
    { role: 'user', content: PROMPT, timestamp: 1757400001000, taskId: 'chattask-e2e-1' },
    { role: 'assistant', content: REPLY, timestamp: 1757400002000 }
  ]
}

/** RunChat 描述符：queued/started 在 resolve 前同步派发（真实时序：后端先 emit 再返回），
 * output 立即派发、done 延迟 2.5s，留出流式态断言窗口 */
const RUN_CHAT_WITH_STREAM_WINDOW = {
  __value__: 'chattask-e2e-1',
  __preEvents__: [
    { event: 'chat-task:queued', payload: { taskId: 'chattask-e2e-1' } },
    { event: 'chat-task:started', payload: { taskId: 'chattask-e2e-1', chatSessionId: 'chatsession-e2e-1' } }
  ],
  __events__: [
    {
      event: 'chat-task:output',
      delayMs: 300,
      payload: { taskId: 'chattask-e2e-1', chatSessionId: 'chatsession-e2e-1', text: REPLY }
    },
    {
      event: 'chat-task:done',
      delayMs: 2500,
      payload: {
        taskId: 'chattask-e2e-1',
        chatSessionId: 'chatsession-e2e-1',
        claudeSessionId: 'claude-e2e-1',
        reply: REPLY,
        exitCode: 0,
        error: '',
        canceled: false
      }
    }
  ]
}

/** AI 对话面板公共前置：打开首页并切到 AI 对话面板（活动栏第 3 项） */
async function openAiChatPage(page) {
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/')
  // 活动画板第 3 项为 AI 对话入口（panels 顺序：工作目录/AI 功能/AI 对话/工具箱/状态看板/仓库统计）
  await page.locator('.activity-bar-item').nth(2).click()
  await expect(page.locator('.ai-chat-panel')).toBeVisible()
}

test.describe('AI 对话工作台', () => {
  test.describe('对话闭环', () => {
    test.use({
      wailsOverrides: {
        // ListChatDirectories 序列：面板挂载加载为空 -> 添加目录后重载返回新目录项
        ListChatDirectories: { __sequence__: [[], [CHAT_DIR]] },
        AddChatDirectory: CHAT_DIR,
        // GetChatSession 序列：选中目录自动选中会话（空消息）-> done 后重载定型消息
        GetChatSession: { __sequence__: [EMPTY_SESSION, FINAL_SESSION] },
        RunChat: RUN_CHAT_WITH_STREAM_WINDOW
      }
    })

    test('添加目录 -> 发送消息 -> 流式输出 -> done 后消息定型', async ({ page }) => {
      await openAiChatPage(page)

      // 添加目录：弹窗填路径与显示名（面板「添加目录」按钮入口）
      await page.locator('.panel-actions').getByRole('button', { name: '添加目录' }).click()
      const dialog = page.locator('.el-dialog:visible')
      await dialog.getByPlaceholder('例如: D:\\workspace\\demo').fill('D:/e2e-demo/demo-repo')
      await dialog.getByPlaceholder('留空则取目录名，例如: 项目管理').fill('示例项目')
      await dialog.getByRole('button', { name: '确定' }).click()

      // 目录出现在侧栏
      await expect(page.locator('.chat-dir-item', { hasText: '示例项目' })).toBeVisible()

      // 选中目录：加载会话列表并自动选中最近会话（按钮标题由「选择会话」变「新会话」）
      await page.locator('.chat-dir-item', { hasText: '示例项目' }).click()
      await expect(page.locator('.session-btn-label')).toHaveText('新会话')

      // 输入需求发送
      await page.getByPlaceholder('输入需求').fill(PROMPT)
      await page.locator('.chat-send-btn').click()

      // RunChat 参数正确（会话 id + prompt + 权限模式 + 模型）
      const runCalls = await getWailsCalls(page, 'RunChat')
      expect(runCalls).toEqual([
        { method: 'RunChat', args: ['chatsession-e2e-1', PROMPT, 'default', ''] }
      ])

      // F2 回归（生产时序）：queued/started 事件先于 RunChat resolve 派发后，
      // resolve 即运行态——状态行不得停留在「排队等待中」
      await expect(page.locator('.chat-input-status')).toHaveText('回复生成中…')

      // 流式输出：assistant 气泡流式渲染（is-streaming 光标 + markdown 加粗）
      const streaming = page.locator('.chat-msg-bubble.is-streaming')
      await expect(streaming).toBeVisible()
      await expect(streaming.locator('strong')).toHaveText('加粗要点')

      // done 后消息定型：流式光标消失，消息区定型渲染 user + assistant 内容
      await expect(page.locator('.chat-msg-bubble.is-streaming')).toHaveCount(0)
      await expect(page.locator('.chat-msg-row.user .chat-msg-text')).toHaveText(PROMPT)
      await expect(page.locator('.chat-msg-md strong')).toHaveText('加粗要点')

      // 首条消息自动生成会话标题（默认标题未被用户改过时，以前 20 字覆盖）
      await expect
        .poll(() => getWailsCalls(page, 'UpdateChatSessionTitle'))
        .toEqual([{ method: 'UpdateChatSessionTitle', args: ['chatsession-e2e-1', PROMPT] }])
    })
  })

  test.describe('会话切换', () => {
    // 两条历史会话（列表按最近活跃降序：B 在前默认选中，A 为更早会话）
    const SESSION_B = {
      id: 'chatsession-b',
      directoryId: 'chatdir-1',
      title: '重构方案讨论',
      cwd: 'D:/e2e-demo/demo-repo',
      claudeSessionId: 'claude-b',
      createdAt: 1757400000000,
      updatedAt: 1757400900000
    }
    const SESSION_A = {
      id: 'chatsession-a',
      directoryId: 'chatdir-1',
      title: '初始调研',
      cwd: 'D:/e2e-demo/demo-repo',
      claudeSessionId: 'claude-a',
      createdAt: 1757300000000,
      updatedAt: 1757310000000
    }
    const SESSION_B_FULL = {
      ...SESSION_B,
      messages: [
        { role: 'user', content: '梳理重构方案', timestamp: 1757400001000 },
        { role: 'assistant', content: 'B 的回复内容', timestamp: 1757400002000 }
      ]
    }
    const SESSION_A_FULL = {
      ...SESSION_A,
      messages: [
        { role: 'user', content: '调研现有模块划分', timestamp: 1757300001000 }
      ]
    }

    test.use({
      wailsOverrides: {
        ListChatDirectories: [CHAT_DIR],
        ListChatSessions: [SESSION_B, SESSION_A],
        // GetChatSession 序列：默认选中 B -> 切换到 A
        GetChatSession: { __sequence__: [SESSION_B_FULL, SESSION_A_FULL] }
      }
    })

    test('会话下拉查看列表并切换：进入历史会话加载其消息', async ({ page }) => {
      await openAiChatPage(page)

      // 选中目录后默认选中最近更新会话（重构方案讨论）
      await page.locator('.chat-dir-item', { hasText: '示例项目' }).click()
      await expect(page.locator('.session-btn-label')).toHaveText('重构方案讨论')
      await expect(page.locator('.chat-msg-row.user .chat-msg-text')).toHaveText('梳理重构方案')

      // 打开会话下拉（类 /resume），切到更早的「初始调研」
      await page.locator('.session-btn').click()
      await page.locator('.session-menu .session-menu-title', { hasText: '初始调研' }).click()

      // 会话按钮标题与消息区切换到目标会话
      await expect(page.locator('.session-btn-label')).toHaveText('初始调研')
      await expect(page.locator('.chat-msg-row.user .chat-msg-text')).toHaveText('调研现有模块划分')

      // GetChatSession 按切换顺序加载两份消息
      const calls = await getWailsCalls(page, 'GetChatSession')
      expect(calls.map((c) => c.args[0])).toEqual(['chatsession-b', 'chatsession-a'])
    })
  })

  test.describe('选择题卡片交互', () => {
    // 围栏 JSON 契约（唯一契约源 frontend/src/utils/chatMarkdown.js parseChatQuestion）：
    // { questions: [{ question, options: [{ label, description? }], multiSelect? }] }
    // 渲染为 .chat-question 卡片（题目 + 选项按钮 + 初始禁用的「发送选择」提交钮）；
    // 点选/提交交互由 AiChatPanel 在气泡容器上事件委托处理（v-html 内无法绑 Vue 事件）
    const QUESTION_JSON = {
      questions: [
        {
          question: '部署环境选哪种？',
          options: [{ label: '物理机' }, { label: '容器化', description: 'K8s 集群部署' }]
        },
        {
          question: '是否启用灰度发布？',
          options: [{ label: '启用' }, { label: '不启用' }]
        }
      ]
    }
    // assistant 消息携带 ```chat-question JSON``` 围栏（真实链路：RunChat output
    // 事件流式累积后随 done 持久化；本用例直接经 GetChatSession 会话 fixture 注入，
    // 事件流渲染已由「对话闭环」用例覆盖，此处聚焦卡片渲染与交互链）
    const QUESTION_CONTENT =
      '为确认部署方案，请回答以下问题：\n\n' +
      '```chat-question\n' +
      JSON.stringify(QUESTION_JSON, null, 2) +
      '\n```'
    const QUESTION_SESSION = {
      id: 'chatsession-e2e-1',
      directoryId: 'chatdir-1',
      // 非默认标题：提交答案不触发首条消息自动改名（UpdateChatSessionTitle 不出现）
      title: '部署方案确认',
      cwd: 'D:/e2e-demo/demo-repo',
      claudeSessionId: 'claude-e2e-1',
      createdAt: 1757400000000,
      updatedAt: 1757400000000
    }
    const QUESTION_SESSION_FULL = {
      ...QUESTION_SESSION,
      messages: [
        { role: 'user', content: '帮我确认部署方案', timestamp: 1757400001000, taskId: 'chattask-q1' },
        // taskId 必带（model.ChatMessage.TaskID）：卡片提交经 data-task-id 对号
        // 标记已答（renderMsgMarkdown 据此在重渲染时兜底追加置灰类）
        { role: 'assistant', content: QUESTION_CONTENT, timestamp: 1757400002000, taskId: 'chattask-q1' }
      ]
    }

    test.use({
      wailsOverrides: {
        ListChatDirectories: [CHAT_DIR],
        ListChatSessions: [QUESTION_SESSION],
        GetChatSession: QUESTION_SESSION_FULL,
        // 提交答案走 runChat 常规链路：RunChat 第二轮返回新任务 id（答案文本本轮
        // 无 output/done 事件不影响被断言面——置灰在 runChat resolve 后即写入）
        RunChat: 'chattask-answer-2'
      }
    })

    test('卡片渲染 → 点选启用提交 → 提交置灰并按题序回传答案消息', async ({ page }) => {
      await openAiChatPage(page)
      await page.locator('.chat-dir-item', { hasText: '示例项目' }).click()

      // 卡片渲染：两道题 + 选项 + 提交钮初始禁用；description 进选项 title 提示
      const card = page.locator('.chat-question')
      await expect(card).toBeVisible()
      const items = card.locator('.chat-question-item')
      await expect(items).toHaveCount(2)
      await expect(items.nth(0).locator('.chat-question-title')).toHaveText('部署环境选哪种？')
      await expect(items.nth(1).locator('.chat-question-title')).toHaveText('是否启用灰度发布？')
      await expect(items.nth(0).locator('.chat-question-option')).toHaveCount(2)
      const submit = card.locator('.chat-question-submit')
      await expect(submit).toBeDisabled()
      await expect(submit).toHaveText('发送选择')
      await expect(
        items.nth(0).locator('.chat-question-option', { hasText: '容器化' })
      ).toHaveAttribute('title', 'K8s 集群部署')

      // 点选选项：selected 态 + 单选 radio 语义（同题重选清除前选）
      const q1 = items.nth(0)
      const q2 = items.nth(1)
      await q1.locator('.chat-question-option', { hasText: '物理机' }).click()
      await expect(q1.locator('.chat-question-option', { hasText: '物理机' })).toHaveClass(/selected/)
      await q1.locator('.chat-question-option', { hasText: '容器化' }).click()
      await expect(q1.locator('.chat-question-option', { hasText: '容器化' })).toHaveClass(/selected/)
      await expect(q1.locator('.chat-question-option', { hasText: '物理机' })).not.toHaveClass(/selected/)

      // 每题至少选中一项才启用提交（第 2 题未答仍禁用）
      await expect(submit).toBeDisabled()
      await q2.locator('.chat-question-option', { hasText: '不启用' }).click()
      await expect(submit).toBeEnabled()

      // 提交：答案按题序拼文本作为新消息发出（用户气泡就地追加 + RunChat 回传）
      await submit.click()
      const answerText =
        '用户通过选项卡片回答：\n1. 部署环境选哪种？：容器化\n2. 是否启用灰度发布？：不启用'
      const answerBubble = page
        .locator('.chat-msg-row.user .chat-msg-text')
        .filter({ hasText: '用户通过选项卡片回答' })
      await expect(answerBubble).toBeVisible()
      await expect(answerBubble).toContainText('1. 部署环境选哪种？：容器化')
      await expect(answerBubble).toContainText('2. 是否启用灰度发布？：不启用')
      await expect(page.locator('.chat-input-status')).toHaveText('回复生成中…')
      await expect.poll(async () => getWailsCalls(page, 'RunChat')).toEqual([
        { method: 'RunChat', args: ['chatsession-e2e-1', answerText, 'default', ''] }
      ])

      // 卡片置灰（chat-question-answered：store 标记 + renderMsgMarkdown 兜底重渲染）
      await expect(page.locator('.chat-question')).toHaveClass(/chat-question-answered/)
    })
  })
})
