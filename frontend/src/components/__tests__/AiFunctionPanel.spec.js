import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessageBox } from 'element-plus'
import AiFunctionPanel from '../AiFunctionPanel.vue'
import AiFunctionRunner from '../AiFunctionRunner.vue'

// Wails 事件 handler 注册表（EventsOn mock 捕获）
const eventHandlers = {}

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((name, cb) => {
    eventHandlers[name] = cb
  }),
  EventsOff: vi.fn()
}))

const runAiFunctionMock = vi.fn(() => Promise.resolve('task-1'))
const runAiFollowUpMock = vi.fn(() => Promise.resolve('task-2'))
const openWithDefaultAppMock = vi.fn(() => Promise.resolve())
const openInExplorerMock = vi.fn(() => Promise.resolve())

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetAiFunctions: vi.fn(() =>
    Promise.resolve([
      {
        id: 'weekly-report',
        name: '生成周报',
        description: '周报描述',
        icon: 'Calendar',
        command: '/ab-weekly-report',
        cwd: 'D:\\proj',
        params: null,
        followUps: [{ id: 'confirm', label: '确认落盘', promptTemplate: '落盘' }],
        completion: 'open_dir'
      },
      {
        id: 'speech-doc',
        name: '发言稿',
        description: '发言稿描述',
        icon: 'Microphone',
        command: '/ab-office:agree-slides',
        cwd: 'D:\\ppt',
        params: { type: 'file', label: '源文档', textFieldKey: 'file' },
        followUps: [],
        completion: 'preview'
      },
      {
        id: 'meeting-list',
        name: '查看/取消腾讯会议',
        description: '会议列表描述',
        icon: 'Clock',
        command: '/tencent-meeting-mcp',
        cwd: 'D:\\meeting',
        params: { type: 'none' },
        followUps: [
          {
            id: 'cancel-meeting',
            label: '取消会议',
            promptTemplate: '/tencent-meeting-mcp 取消会议 {{meeting}}'
          }
        ],
        completion: 'none'
      }
    ])
  ),
  RunAiFunction: (...args) => runAiFunctionMock(...args),
  RunAiFollowUp: (...args) => runAiFollowUpMock(...args),
  CancelAiTask: vi.fn(),
  OpenInExplorer: (...args) => openInExplorerMock(...args),
  OpenWithDefaultApp: (...args) => openWithDefaultAppMock(...args),
  GetFileTree: vi.fn(() => Promise.resolve([]))
}))

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="dlg"><slot /></div>',
    props: ['modelValue', 'title', 'width', 'top', 'appendToBody', 'destroyOnClose']
  },
  'el-empty': { template: '<div class="empty" />' },
  'el-tabs': { template: '<div class="tabs"><slot /></div>', props: ['modelValue'] },
  'el-tab-pane': { template: '<div class="tab-pane"><slot /></div>', props: ['label', 'name'] },
  'el-tag': { template: '<span class="tag"><slot /></span>', props: ['size', 'type'] },
  // 声明 emits 后 click 监听器不进 $attrs，避免透传 onClick 与 $emit('click') 双触发
  'el-button': {
    emits: ['click'],
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>'
  },
  'el-icon': { template: '<span><slot /></span>' },
  'el-table': {
    template: '<div class="el-table"><slot /></div>',
    props: ['data', 'size', 'border']
  },
  // 行 stub：固定行数据供「操作」列 scoped slot 渲染详情/取消按钮（与 mock 会议表格数据一致，
  // 含隐藏列字段——列不渲染但数据在行内，供「详情」复制）
  'el-table-column': {
    props: ['prop', 'label', 'width', 'fixed'],
    data() {
      return {
        stubRow: {
          会议主题: '评审会',
          会议号: '123-456',
          开始时间: '2026-09-09 10:00',
          结束时间: '2026-09-09 11:00',
          时长: '60分钟',
          入会链接: 'https://meeting.tencent.com/dm/r/abc123',
          入会密码: '无',
          状态: '未开始'
        }
      }
    },
    template: '<div class="el-table-col"><span class="col-label">{{ label }}</span><slot :row="stubRow" /></div>'
  },
  // 功能 Tab 内容 stub：声明 props/emits，测试用 $emit('run', params) 模拟点运行
  AiFunctionRunner: {
    props: ['fn'],
    emits: ['run'],
    template: '<div class="ai-runner-stub" />'
  },
  AiFunctionConfigDialog: true
}

const createWrapper = () =>
  mount(AiFunctionPanel, {
    // 组件经 v-show 常驻挂载（Home 主区互斥展示），无 visible prop，挂载即加载并渲染
    global: { stubs }
  })

// 点卡片打开功能 Tab，并在功能 Tab 内触发 run（模拟 Runner 运行按钮）
const runFromCard = async (wrapper, idx, params = {}) => {
  await wrapper.findAll('.ai-card')[idx].trigger('click')
  await flushPromises()
  const runners = wrapper.findAllComponents(AiFunctionRunner)
  runners[runners.length - 1].vm.$emit('run', params)
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  for (const k of Object.keys(eventHandlers)) delete eventHandlers[k]
})

describe('AiFunctionPanel', () => {
  it('渲染功能卡片列表', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    const cards = wrapper.findAll('.ai-card')
    expect(cards.length).toBe(3)
    expect(cards[0].text()).toContain('生成周报')
    expect(cards[1].text()).toContain('发言稿')
    expect(cards[2].text()).toContain('查看/取消腾讯会议')
  })

  it('点击无参功能卡片打开功能 Tab 而非直接运行，Runner 收到正确 fn', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await wrapper.findAll('.ai-card')[0].trigger('click')
    await flushPromises()
    // 点卡片只打开功能 Tab，不直接调 RunAiFunction
    expect(runAiFunctionMock).not.toHaveBeenCalled()
    const runner = wrapper.findComponent(AiFunctionRunner)
    expect(runner.exists()).toBe(true)
    expect(runner.props('fn')).toMatchObject({ id: 'weekly-report', name: '生成周报' })
  })

  it('有参功能点击卡片同样打开功能 Tab（参数录入内嵌 Runner）', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await wrapper.findAll('.ai-card')[1].trigger('click')
    await flushPromises()
    expect(runAiFunctionMock).not.toHaveBeenCalled()
    const runner = wrapper.findComponent(AiFunctionRunner)
    expect(runner.exists()).toBe(true)
    expect(runner.props('fn')).toMatchObject({ id: 'speech-doc' })
  })

  it('功能 Tab 触发 run：RunAiFunction 以正确参数调用 + 新任务 Tab 出现 + 自动切换，功能 Tab 保留', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await runFromCard(wrapper, 1, { file: 'D:\\doc\\a.md' })
    expect(runAiFunctionMock).toHaveBeenCalledWith('speech-doc', { file: 'D:\\doc\\a.md' })
    // 新任务 Tab 出现（输出区渲染）
    expect(wrapper.find('.task-output').exists()).toBe(true)
    // 自动切到新任务 Tab
    expect(wrapper.vm.activeTabId).toBe('task-1')
    // 功能 Tab 保留（可反复运行）
    expect(wrapper.findAllComponents(AiFunctionRunner).length).toBe(1)
  })

  it('主段复用同功能空闲任务 Tab：命中最新非运行中条目原位替换不新增，全部运行中则新增', async () => {
    runAiFunctionMock
      .mockReturnValueOnce(Promise.resolve('task-1'))
      .mockReturnValueOnce(Promise.resolve('task-2'))
      .mockReturnValueOnce(Promise.resolve('task-3'))
    const wrapper = createWrapper()
    await flushPromises()
    // 第一次运行：无空闲同功能 Tab，新增
    await runFromCard(wrapper, 1, { file: 'D:\\doc\\a.md' })
    expect(wrapper.vm.tasks.length).toBe(1)
    // 任务完成 → 空闲
    eventHandlers['ai-task:done']({ taskId: 'task-1', error: '', canceled: false })
    await flushPromises()
    // 第二次运行同功能：复用该空闲 Tab，数组长度不变、内容替换为新任务
    await runFromCard(wrapper, 1, { file: 'D:\\doc\\b.md' })
    expect(wrapper.vm.tasks.length).toBe(1)
    expect(wrapper.vm.tasks[0]).toMatchObject({
      taskId: 'task-2',
      functionId: 'speech-doc',
      running: true,
      output: '',
      error: '',
      canceled: false
    })
    // 旧输出/会话等历史一并清空
    expect(wrapper.vm.tasks[0].sessionId).toBe('')
    expect(wrapper.vm.activeTabId).toBe('task-2')
    // 第三次运行：同功能 Tab 全部运行中，不拦截，新增 Tab
    await runFromCard(wrapper, 1, { file: 'D:\\doc\\c.md' })
    expect(wrapper.vm.tasks.length).toBe(2)
    expect(wrapper.vm.activeTabId).toBe('task-3')
  })

  it('同功能功能 Tab 已存在时再点卡片仅切回不重复开', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await runFromCard(wrapper, 0, {})
    expect(wrapper.vm.activeTabId).toBe('task-1')
    // 再点同功能卡片：切回已有功能 Tab，不新开
    await wrapper.findAll('.ai-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.findAllComponents(AiFunctionRunner).length).toBe(1)
    expect(wrapper.vm.activeTabId).toBe('func:weekly-report')
  })

  it('ai-task:output 事件追加输出，done 后触发完成动作', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await runFromCard(wrapper, 0, {})
    expect(runAiFunctionMock).toHaveBeenCalledWith('weekly-report', {})

    // 流式输出
    eventHandlers['ai-task:output']({ taskId: 'task-1', text: '第一段输出' })
    await flushPromises()
    expect(wrapper.find('.task-output').text()).toContain('第一段输出')

    // 完成（completion=open_dir → 打开 cwd）
    eventHandlers['ai-task:done']({
      taskId: 'task-1',
      sessionId: 's1',
      error: '',
      canceled: false,
      output: '第一段输出'
    })
    await flushPromises()
    expect(wrapper.find('.tag').text()).toBe('已完成')
    expect(openInExplorerMock).toHaveBeenCalledWith('D:\\proj')
  })

  it('done 带错误时展示失败状态且不触发完成动作', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    await runFromCard(wrapper, 0, {})

    eventHandlers['ai-task:done']({
      taskId: 'task-1',
      error: 'boom',
      canceled: false
    })
    await flushPromises()
    expect(wrapper.find('.tag').text()).toBe('失败')
    expect(openInExplorerMock).not.toHaveBeenCalled()
  })

  it('会议列表输出 markdown 表格时渲染表格视图（隐藏列不渲染），行内取消走确认后续段', async () => {
    const confirmSpy = vi.spyOn(ElMessageBox, 'confirm').mockResolvedValue()
    const wrapper = createWrapper()
    await flushPromises()
    // meeting-list（第三张卡，无参）
    await runFromCard(wrapper, 2, {})
    expect(runAiFunctionMock).toHaveBeenCalledWith('meeting-list', {})

    eventHandlers['ai-task:output']({
      taskId: 'task-1',
      text: '| 会议主题 | 会议号 | 开始时间 | 结束时间 | 时长 | 入会链接 | 入会密码 | 状态 |\n' +
            '| --- | --- | --- | --- | --- | --- | --- | --- |\n' +
            '| 评审会 | 123-456 | 2026-09-09 10:00 | 2026-09-09 11:00 | 60分钟 | https://meeting.tencent.com/dm/r/abc123 | 无 | 未开始 |'
    })
    eventHandlers['ai-task:done']({ taskId: 'task-1', error: '', canceled: false })
    await flushPromises()

    // 表格视图生效：表格容器 + 动态列头，文本视图与 followup-bar 双隐藏
    expect(wrapper.find('.task-table').exists()).toBe(true)
    expect(wrapper.find('.task-output').exists()).toBe(false)
    expect(wrapper.find('.followup-bar').exists()).toBe(false)
    const labels = wrapper.findAll('.col-label').map((n) => n.text())
    // 可见列 + 操作列正常渲染
    expect(labels).toEqual(
      expect.arrayContaining(['会议主题', '会议号', '开始时间', '结束时间', '状态', '操作'])
    )
    // 隐藏列不渲染（数据仍在行内，经「详情」按钮复制补全）
    expect(labels).toEqual(
      expect.not.arrayContaining(['时长', '入会链接', '入会密码'])
    )

    // 行内取消：确认弹窗 → RunAiFollowUp 携带该行会议标识
    const cancelBtn = wrapper.findAll('.task-table button').find((b) => b.text().includes('取消'))
    expect(cancelBtn).toBeTruthy()
    await cancelBtn.trigger('click')
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalled()
    expect(runAiFollowUpMock).toHaveBeenCalledWith('task-1', 'cancel-meeting', {
      meeting: '评审会（会议号 123-456）'
    })
    confirmSpy.mockRestore()
  })

  it('行内「详情」按钮组装完整会议信息（含隐藏列）复制到剪贴板', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const wrapper = createWrapper()
    await flushPromises()
    // meeting-list（第三张卡，无参）
    await runFromCard(wrapper, 2, {})

    eventHandlers['ai-task:output']({
      taskId: 'task-1',
      text: '| 会议主题 | 会议号 | 开始时间 | 结束时间 | 时长 | 入会链接 | 入会密码 | 状态 |\n' +
            '| --- | --- | --- | --- | --- | --- | --- | --- |\n' +
            '| 评审会 | 123-456 | 2026-09-09 10:00 | 2026-09-09 11:00 | 60分钟 | https://meeting.tencent.com/dm/r/abc123 | 无 | 未开始 |'
    })
    eventHandlers['ai-task:done']({ taskId: 'task-1', error: '', canceled: false })
    await flushPromises()

    const detailBtn = wrapper.findAll('.task-table button').find((b) => b.text().includes('详情'))
    expect(detailBtn).toBeTruthy()
    await detailBtn.trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledTimes(1)
    expect(writeText).toHaveBeenCalledWith(
      '会议主题：评审会\n' +
      '会议时间：2026-09-09 10:00-2026-09-09 11:00\n' +
      '时长：60分钟\n' +
      '入会链接：https://meeting.tencent.com/dm/r/abc123\n' +
      '#腾讯会议：123-456\n' +
      '入会密码：无'
    )
  })
})
