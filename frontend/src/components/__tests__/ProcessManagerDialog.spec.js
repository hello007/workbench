import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ProcessManagerDialog from '../ProcessManagerDialog.vue'

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
const killProcessMock = vi.fn()

vi.mock('../../../wailsjs/go/main/App', () => ({
  ListProcesses: (...args) => listProcessesMock(...args),
  StopProcess: (...args) => stopProcessMock(...args),
  KillProcess: (...args) => killProcessMock(...args)
}))

// 形状对齐 model.ProcessInfo（字段随后端下发，isSelf / isSystem 由后端填充）
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
      { port: 3000, protocol: 'TCP', state: 'LISTENING', localAddr: '0.0.0.0' },
      { port: 3001, protocol: 'TCP', state: 'LISTENING', localAddr: '0.0.0.0' }
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
    template: '<div v-if="modelValue" class="pm-dialog"><slot name="header" /><slot /></div>',
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
  'el-input': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keyup="$emit(\'keyup\', $event)" />',
    props: ['modelValue', 'placeholder', 'size', 'clearable'],
    emits: ['update:modelValue', 'keyup']
  },
  'el-select': {
    template: '<select class="pm-mode-select" :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><slot /></select>',
    props: ['modelValue', 'size'],
    emits: ['update:modelValue']
  },
  'el-option': { template: '<option :value="value">{{ label }}</option>', props: ['label', 'value'] },
  'el-checkbox': {
    template: '<label class="pm-checkbox"><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" /><slot /></label>',
    props: ['modelValue', 'size'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'size', 'type', 'link', 'loading'],
    emits: ['click']
  },
  'el-tag': { template: '<span class="tag"><slot /></span>', props: ['type', 'size'] },
  'el-icon': { template: '<i><slot /></i>' },
  Refresh: { name: 'Refresh', template: '<span class="refresh-icon" />' },
  'el-table': { template: '<div class="el-table"><slot /></div>', props: ['data', 'size', 'border', 'emptyText'] },
  // 行 stub：遍历 el-table 的 data 逐行渲染 scoped slot，供「停止 / 强杀」按钮渲染与断言
  'el-table-column': {
    props: ['prop', 'label', 'width', 'minWidth', 'fixed', 'showOverflowTooltip'],
    template: '<div class="el-table-col"><span class="col-label">{{ label }}</span><div v-for="row in $parent.data" :key="row.pid" class="pm-row"><slot :row="row" /></div></div>'
  }
}

const createWrapper = (visible = true) =>
  mount(ProcessManagerDialog, {
    props: { modelValue: visible },
    global: { stubs }
  })

// 操作列（最后一列）按行下标取「停止」「强杀」按钮
const rowButtons = (wrapper, rowIndex) => {
  const cols = wrapper.findAll('.el-table-col')
  const opCol = cols[cols.length - 1]
  return opCol.findAll('.pm-row')[rowIndex].findAll('button')
}

// 按可见文本取按钮（搜索区 / 表头刷新）
const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text() === text)

describe('ProcessManagerDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listProcessesMock.mockResolvedValue(mockProcesses)
    stopProcessMock.mockResolvedValue({ stopped: true, method: 'graceful' })
    killProcessMock.mockResolvedValue(undefined)
  })

  it('打开时枚举全部监听进程并渲染端口/协议/状态/内存列', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    // 空输入 + auto 模式：后端返回全部监听进程
    expect(listProcessesMock).toHaveBeenCalledWith('', 'auto')
    const text = wrapper.text()
    expect(text).toContain('workbench.exe')
    expect(text).toContain('36115')
    expect(text).toContain('LISTENING')
    expect(text).toContain('TCP')
    // 内存按量级升档：102400 KB → 100.0 MB
    expect(text).toContain('100.0 MB')
    // 多端口进程逗号连接
    expect(text).toContain('3000, 3001')
  })

  it('未打开时不枚举进程', async () => {
    createWrapper(false)
    await flushPromises()
    expect(listProcessesMock).not.toHaveBeenCalled()
  })

  it('本应用行标注「本应用」且停止/强杀按钮禁用', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    const nameCol = wrapper.findAll('.el-table-col')[0]
    expect(nameCol.findAll('.pm-row')[0].text()).toContain('本应用')

    const [stopBtn, killBtn] = rowButtons(wrapper, 0)
    expect(stopBtn.attributes('disabled')).toBeDefined()
    expect(killBtn.attributes('disabled')).toBeDefined()
    // 普通进程行按钮可用
    const [normalStop] = rowButtons(wrapper, 1)
    expect(normalStop.attributes('disabled')).toBeUndefined()
  })

  it('系统关键进程触发红色强确认', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 2)[0].trigger('click')
    await flushPromises()

    const { ElMessageBox } = await import('element-plus')
    expect(ElMessageBox.confirm).toHaveBeenCalled()
    const [message, title, options] = ElMessageBox.confirm.mock.calls[0]
    expect(message).toContain('系统关键进程')
    expect(message).toContain('蓝屏')
    expect(title).toBe('危险操作')
    // 红色错误级确认（区别于普通进程的 warning 级）
    expect(options.type).toBe('error')
    expect(stopProcessMock).toHaveBeenCalledWith(4, false)
  })

  it('普通进程触发通用确认，文案含数据丢失提示', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    const { ElMessageBox } = await import('element-plus')
    const [message, title, options] = ElMessageBox.confirm.mock.calls[0]
    expect(message).toContain('node.exe')
    expect(message).toContain('未保存的数据将丢失')
    expect(title).toBe('结束进程')
    expect(options.type).toBe('warning')
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

  it('停止返回 graceful 提示「进程已正常退出」', async () => {
    stopProcessMock.mockResolvedValue({ stopped: true, method: 'graceful' })
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('进程已正常退出')
    // 操作成功后自动刷新列表
    expect(listProcessesMock).toHaveBeenCalledTimes(2)
  })

  it('停止返回 forced 提示「进程已强制结束」（如实反映生效手段）', async () => {
    stopProcessMock.mockResolvedValue({ stopped: true, method: 'forced' })
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('进程已强制结束')
    // 谎称优雅的文案不得出现
    expect(ElMessage.success).not.toHaveBeenCalledWith('进程已正常退出')
  })

  it('强杀直接调用 KillProcess 并提示已强制结束', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[1].trigger('click')
    await flushPromises()

    const { ElMessage, ElMessageBox } = await import('element-plus')
    // 强杀跳过优雅阶段，不调 StopProcess
    expect(stopProcessMock).not.toHaveBeenCalled()
    expect(killProcessMock).toHaveBeenCalledWith(8001, false)
    expect(ElMessage.success).toHaveBeenCalledWith('进程已强制结束')
    // 强杀文案更强硬
    expect(ElMessageBox.confirm.mock.calls[0][0]).toContain('强制结束不会等待进程保存数据')
  })

  it('进程树勾选默认关闭，勾选后终止参数透传 true', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[1].trigger('click')
    await flushPromises()
    expect(killProcessMock).toHaveBeenCalledWith(8001, false)

    await wrapper.find('.pm-checkbox input').setValue(true)
    await rowButtons(wrapper, 1)[1].trigger('click')
    await flushPromises()
    expect(killProcessMock).toHaveBeenLastCalledWith(8001, true)
  })

  it('模式切换下拉强制按端口检索', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await wrapper.find('input').setValue('36115')
    await wrapper.find('select').setValue('port')
    await findButton(wrapper, '检索').trigger('click')
    await flushPromises()

    expect(listProcessesMock).toHaveBeenLastCalledWith('36115', 'port')
  })

  it('输入框回车触发检索（默认 auto 模式）', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await wrapper.find('input').setValue('node')
    await wrapper.find('input').trigger('keyup.enter')
    await flushPromises()

    expect(listProcessesMock).toHaveBeenLastCalledWith('node', 'auto')
  })

  it('刷新按钮重新枚举进程', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    expect(listProcessesMock).toHaveBeenCalledTimes(1)

    await findButton(wrapper, '刷新').trigger('click')
    await flushPromises()
    expect(listProcessesMock).toHaveBeenCalledTimes(2)
  })

  it('进程已不存在时提示并自动刷新列表', async () => {
    stopProcessMock.mockRejectedValue({ code: 'E_PROCESS_NOT_FOUND', message: '进程已不存在' })
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    // NOT_FOUND 属 warning 级，不报 error
    expect(ElMessage.warning).toHaveBeenCalledWith('进程已不存在')
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(listProcessesMock).toHaveBeenCalledTimes(2)
  })

  it('枚举失败按 error 级提示', async () => {
    listProcessesMock.mockRejectedValue({ code: 'E_PROCESS_ENUM_FAILED', message: 'netstat 执行失败' })
    const wrapper = createWrapper(true)
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('进程检索失败: netstat 执行失败')
  })

  it('终止操作在途时该行按钮禁用防重，返回后恢复可用', async () => {
    // 挂起 StopProcess 模拟停止耗时（最坏约 11s：3s 优雅超时 + 5s 轮询 + 3s 强杀）
    let resolveStop
    stopProcessMock.mockReturnValue(new Promise((resolve) => { resolveStop = resolve }))
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    // 在途：该行「停止」「强杀」均禁用，给出可见等待态
    expect(rowButtons(wrapper, 1)[0].attributes('disabled')).toBeDefined()
    expect(rowButtons(wrapper, 1)[1].attributes('disabled')).toBeDefined()
    expect(stopProcessMock).toHaveBeenCalledTimes(1)

    // 重复点击不重复发起（jsdom 对 disabled 按钮不派发 click，另有 isRowPending 兜底）
    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()
    expect(stopProcessMock).toHaveBeenCalledTimes(1)

    resolveStop({ stopped: true, method: 'forced' })
    await flushPromises()

    // 返回后解除等待态
    expect(rowButtons(wrapper, 1)[0].attributes('disabled')).toBeUndefined()
    expect(rowButtons(wrapper, 1)[1].attributes('disabled')).toBeUndefined()
  })

  it('确认框取消时不进入等待态，按钮保持可用', async () => {
    const { ElMessageBox } = await import('element-plus')
    ElMessageBox.confirm.mockRejectedValueOnce(new Error('cancel'))
    const wrapper = createWrapper(true)
    await flushPromises()

    await rowButtons(wrapper, 1)[0].trigger('click')
    await flushPromises()

    expect(stopProcessMock).not.toHaveBeenCalled()
    // 取消不应把按钮锁死
    expect(rowButtons(wrapper, 1)[0].attributes('disabled')).toBeUndefined()
  })

  it('枚举超时按 warning 级提示可重试', async () => {
    listProcessesMock.mockRejectedValue({ code: 'E_PROCESS_ENUM_TIMEOUT', message: '进程枚举超时，请重试' })
    const wrapper = createWrapper(true)
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalledWith('进程枚举超时，请重试')
  })
})
