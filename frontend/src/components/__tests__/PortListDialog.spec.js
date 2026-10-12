import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PortListDialog from '../PortListDialog.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

const listProcessesMock = vi.fn()
const stopProcessMock = vi.fn()

vi.mock('../../../wailsjs/go/main/App', () => ({
  ListProcesses: (...args) => listProcessesMock(...args),
  StopProcess: (...args) => stopProcessMock(...args)
}))

// 形状对齐 model.ProcessInfo：端口一览复用进程枚举底座（空关键字 = 仅监听态默认视图）
const mockProcesses = [
  {
    pid: 44336,
    name: 'workbench.exe',
    ports: [{ port: 36115, protocol: 'TCP', state: 'LISTENING', localAddr: '127.0.0.1' }],
    memoryKB: 102400,
    isSelf: true,
    isSystem: false
  },
  {
    pid: 8001,
    name: 'node.exe',
    ports: [
      { port: 3001, protocol: 'TCP', state: 'LISTENING', localAddr: '0.0.0.0' },
      { port: 3000, protocol: 'TCP', state: 'LISTENING', localAddr: '0.0.0.0' }
    ],
    memoryKB: 2048,
    isSelf: false,
    isSystem: false
  },
  {
    pid: 4,
    name: 'System',
    ports: [{ port: 445, protocol: 'TCP', state: 'LISTENING', localAddr: '0.0.0.0' }],
    memoryKB: 2904,
    isSelf: false,
    isSystem: true
  }
]

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="pl-dialog"><slot name="header" /><slot /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue', 'open'],
    mounted() {
      if (this.modelValue) this.$emit('open')
    },
    watch: {
      modelValue(v) {
        if (v) this.$emit('open')
      }
    }
  },
  'el-button': {
    template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'size', 'type', 'link', 'loading'],
    emits: ['click']
  },
  'el-tag': { template: '<span class="tag"><slot /></span>', props: ['type', 'size'] },
  'el-icon': { template: '<i><slot /></i>' },
  Refresh: { name: 'Refresh', template: '<span class="refresh-icon" />' },
  'el-table': {
    template: '<div class="el-table"><slot /><span v-if="emptyText && (!data || data.length === 0)" class="empty-text">{{ emptyText }}</span></div>',
    props: ['data', 'size', 'border', 'emptyText']
  },
  // 行 stub：遍历 el-table 的 data 逐行渲染 scoped slot（key 带端口号——扁平化后同 pid 多行）
  'el-table-column': {
    props: ['prop', 'label', 'width', 'minWidth', 'fixed', 'sortable', 'showOverflowTooltip'],
    template: '<div class="el-table-col"><span class="col-label">{{ label }}</span><div v-for="(row, i) in $parent.data" :key="row.pid + \'-\' + row.port + \'-\' + i" class="pl-row"><template v-if="prop">{{ row[prop] }}</template><slot :row="row" /></div></div>'
  }
}

const createWrapper = (visible = true) =>
  mount(PortListDialog, {
    props: { modelValue: visible },
    global: { stubs }
  })

// 操作列（最后一列）按行下标取「终止」按钮；行序 = 端口升序扁平化结果
const rowButtons = (wrapper, rowIndex) => {
  const cols = wrapper.findAll('.el-table-col')
  const opCol = cols[cols.length - 1]
  return opCol.findAll('.pl-row')[rowIndex].findAll('button')
}

const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text() === text)

describe('PortListDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listProcessesMock.mockResolvedValue(mockProcesses)
    stopProcessMock.mockResolvedValue({ stopped: true, method: 'graceful' })
  })

  it('打开时枚举监听进程并扁平化为一行一个端口、按端口号升序', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    // 空关键字默认视图：后端返回全部监听态进程
    expect(listProcessesMock).toHaveBeenCalledWith('', '')
    // 4 个端口行（node 占 2 端口展开 2 行），升序：445 / 3000 / 3001 / 36115
    expect(wrapper.findAll('.el-table-col')[0].findAll('.pl-row').length).toBe(4)
    const portCol = wrapper.findAll('.el-table-col')[0]
    const ports = portCol.findAll('.pl-row').map((r) => Number(r.text()))
    expect(ports).toEqual([445, 3000, 3001, 36115])
    const text = wrapper.text()
    expect(text).toContain('node.exe')
    expect(text).toContain('LISTENING')
    expect(text).toContain('127.0.0.1')
  })

  it('未打开时不枚举端口', async () => {
    createWrapper(false)
    await flushPromises()
    expect(listProcessesMock).not.toHaveBeenCalled()
  })

  it('空列表展示空态文案', async () => {
    listProcessesMock.mockResolvedValue([])
    const wrapper = createWrapper(true)
    await flushPromises()
    expect(wrapper.find('.empty-text').text()).toBe('当前没有监听端口')
  })

  it('本应用行标注「本应用」且终止按钮禁用', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    // isSelf 行 = 端口 36115（升序末行）
    const nameCol = wrapper.findAll('.el-table-col')[4]
    expect(nameCol.findAll('.pl-row')[3].text()).toContain('本应用')
    const [termBtn] = rowButtons(wrapper, 3)
    expect(termBtn.attributes('disabled')).toBeDefined()
    // 普通进程行按钮可用
    const [normalBtn] = rowButtons(wrapper, 1)
    expect(normalBtn.attributes('disabled')).toBeUndefined()
  })

  it('系统关键进程触发红色强确认', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    // 系统进程行 = 端口 445（升序首行）
    await rowButtons(wrapper, 0)[0].trigger('click')
    await flushPromises()

    const { ElMessageBox } = await import('element-plus')
    expect(ElMessageBox.confirm).toHaveBeenCalled()
    const [message, title, options] = ElMessageBox.confirm.mock.calls[0]
    expect(message).toContain('系统关键进程')
    expect(message).toContain('蓝屏')
    expect(title).toBe('危险操作')
    expect(options.type).toBe('error')
    expect(stopProcessMock).toHaveBeenCalledWith(4, false)
  })

  it('普通进程确认文案含数据丢失提示；多端口进程提示连带释放', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    // node 占 2 端口：确认文案须提示终止会连带其余端口（升序第 2 行 = 端口 3000）
    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    const { ElMessageBox } = await import('element-plus')
    const [message, title, options] = ElMessageBox.confirm.mock.calls[0]
    expect(message).toContain('node.exe')
    expect(message).toContain('未保存的数据将丢失')
    expect(message).toContain('共占用 2 个端口')
    expect(title).toBe('结束进程')
    expect(options.type).toBe('warning')
  })

  it('单端口进程确认文案不含连带提示', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    // System（445）只占 1 端口（升序首行）
    await rowButtons(wrapper, 0)[0].trigger('click')
    await flushPromises()

    const { ElMessageBox } = await import('element-plus')
    expect(ElMessageBox.confirm.mock.calls[0][0]).not.toContain('共占用')
  })

  it('用户取消确认时不调用终止方法', async () => {
    const { ElMessageBox } = await import('element-plus')
    ElMessageBox.confirm.mockRejectedValueOnce(new Error('cancel'))
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    expect(stopProcessMock).not.toHaveBeenCalled()
  })

  it('终止返回 graceful 提示「进程已正常退出」并刷新列表', async () => {
    stopProcessMock.mockResolvedValue({ stopped: true, method: 'graceful' })
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('进程已正常退出')
    expect(listProcessesMock).toHaveBeenCalledTimes(2)
  })

  it('终止返回 forced 提示「进程已强制结束」（如实反映生效手段）', async () => {
    stopProcessMock.mockResolvedValue({ stopped: true, method: 'forced' })
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('进程已强制结束')
    expect(ElMessage.success).not.toHaveBeenCalledWith('进程已正常退出')
  })

  it('进程已不存在时提示并自动刷新列表', async () => {
    stopProcessMock.mockRejectedValue({ code: 'E_PROCESS_NOT_FOUND', message: '进程已不存在' })
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalledWith('进程已不存在')
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(listProcessesMock).toHaveBeenCalledTimes(2)
  })

  it('枚举失败按 error 级提示', async () => {
    listProcessesMock.mockRejectedValue({ code: 'E_PROCESS_ENUM_FAILED', message: 'netstat 执行失败' })
    const wrapper = createWrapper(true)
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('端口枚举失败: netstat 执行失败')
  })

  it('枚举超时按 warning 级提示可重试', async () => {
    listProcessesMock.mockRejectedValue({ code: 'E_PROCESS_ENUM_TIMEOUT', message: '进程枚举超时，请重试' })
    const wrapper = createWrapper(true)
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalledWith('进程枚举超时，请重试')
  })

  it('终止操作在途时该行按钮禁用防重，返回后恢复可用', async () => {
    // 挂起 StopProcess 模拟停止耗时（最坏约 11s：3s 优雅超时 + 5s 轮询 + 3s 强杀）
    let resolveStop
    stopProcessMock.mockReturnValue(new Promise((resolve) => { resolveStop = resolve }))
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    expect(rowButtons(wrapper, 1)[0].attributes('disabled')).toBeDefined()
    expect(stopProcessMock).toHaveBeenCalledTimes(1)

    // 同进程另一端口行同样禁用（同一 pid 在途，防重复终止）
    expect(rowButtons(wrapper, 2)[0].attributes('disabled')).toBeDefined()

    // 重复点击不重复发起（jsdom 对 disabled 按钮不派发 click，另有 isRowPending 兜底）
    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()
    expect(stopProcessMock).toHaveBeenCalledTimes(1)

    resolveStop({ stopped: true, method: 'forced' })
    await flushPromises()

    expect(rowButtons(wrapper, 1)[0].attributes('disabled')).toBeUndefined()
  })

  it('确认框取消时不进入等待态，按钮保持可用', async () => {
    const { ElMessageBox } = await import('element-plus')
    ElMessageBox.confirm.mockRejectedValueOnce(new Error('cancel'))
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    expect(stopProcessMock).not.toHaveBeenCalled()
    expect(rowButtons(wrapper, 1)[0].attributes('disabled')).toBeUndefined()
  })

  it('刷新按钮重新枚举', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    expect(listProcessesMock).toHaveBeenCalledTimes(1)

    await findButton(wrapper, '刷新').trigger('click')
    await flushPromises()
    expect(listProcessesMock).toHaveBeenCalledTimes(2)
  })
})
