import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import HostsPanel from '../HostsPanel.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

const getHostsMock = vi.fn()

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetHosts: (...args) => getHostsMock(...args)
}))

// 图标按需 mock：HostsPanel 用 Refresh / WarningFilled
vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { name: 'Refresh', template: '<span class="i-refresh" />' },
  WarningFilled: { name: 'WarningFilled', template: '<span class="i-warning" />' }
}))

const stubs = {
  'el-button': {
    // emits 声明必须存在：@click 属声明事件才不落入 $attrs，否则与 $emit 双路径各触发一次
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'size', 'type', 'link', 'loading'],
    emits: ['click']
  },
  'el-icon': { template: '<i><slot /></i>' },
  'el-input': {
    template: '<input type="text" :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'clearable', 'size']
  },
  'el-checkbox': {
    template: '<label class="el-checkbox"><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" /><slot /></label>',
    props: ['modelValue', 'size'],
    emits: ['update:modelValue']
  },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['size', 'type'] },
  'el-table': {
    template: '<div class="el-table"><slot /><span v-if="emptyText && data && data.length === 0" class="empty-text">{{ emptyText }}</span></div>',
    props: ['data', 'size', 'border', 'maxHeight', 'emptyText']
  },
  // 行 stub：遍历 el-table 的 data 逐行渲染 scoped slot
  'el-table-column': {
    props: ['prop', 'label', 'width', 'minWidth', 'fixed', 'showOverflowTooltip'],
    template: '<div class="el-table-col"><span class="col-label">{{ label }}</span><div v-for="(row, i) in $parent.data" :key="i" class="hosts-row"><template v-if="prop">{{ row[prop] }}</template><slot :row="row" /></div></div>'
  }
}

// 形状对齐 model.HostsView / HostsEntry：覆盖 entry/comment/unknown/blank 四种行类别
const mockView = {
  path: 'C:\\Windows\\System32\\drivers\\etc\\hosts',
  entries: [
    { kind: 'entry', ip: '127.0.0.1', hostnames: ['localhost'], comment: '', raw: '127.0.0.1 localhost' },
    { kind: 'entry', ip: '::1', hostnames: ['ip6-local'], comment: 'IPv6 回环', raw: '::1 ip6-local # IPv6 回环' },
    { kind: 'entry', ip: '10.20.30.40', hostnames: ['dev.internal.example.com'], comment: '', raw: '10.20.30.40 dev.internal.example.com' },
    { kind: 'comment', ip: '', hostnames: [], comment: '', raw: '# 屏蔽广告域名示例' },
    { kind: 'unknown', ip: '', hostnames: [], comment: '', raw: 'broken-line-without-ip' },
    { kind: 'blank', ip: '', hostnames: [], comment: '', raw: '' }
  ],
  error: ''
}

const createWrapper = () =>
  mount(HostsPanel, {
    global: { stubs }
  })

// 列序：IP 0 / 主机名 1 / 注释 2 / 操作 3
const colRows = (wrapper, colIndex) => wrapper.findAll('.el-table-col')[colIndex].findAll('.hosts-row')

const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text() === text)

describe('HostsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getHostsMock.mockResolvedValue(mockView)
  })

  it('打开时读取 hosts 并渲染路径与默认视图（注释/空行隐藏，未识别行恒展示）', async () => {
    const wrapper = createWrapper()
    await flushPromises()

    expect(getHostsMock).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.hosts-path').text()).toBe('C:\\Windows\\System32\\drivers\\etc\\hosts')
    // 默认视图：3 entry + 1 unknown = 4 条（comment/blank 隐藏）
    expect(colRows(wrapper, 0).length).toBe(4)
    expect(wrapper.find('.hosts-count').text()).toContain('4 / 4 条')
  })

  it('挂载即读取 hosts（面板化：onMounted 替代原弹窗 @open 语义）', async () => {
    createWrapper()
    await flushPromises()
    expect(getHostsMock).toHaveBeenCalledTimes(1)
  })

  it('默认视图隐藏纯注释与空行，显示注释开关切回展示', async () => {
    const wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.text()).not.toContain('# 屏蔽广告域名示例')
    expect(wrapper.text()).not.toContain('（空行）')

    await wrapper.find('.el-checkbox input').setValue(true)
    await flushPromises()

    expect(colRows(wrapper, 0).length).toBe(6)
    expect(wrapper.text()).toContain('# 屏蔽广告域名示例')
    expect(wrapper.text()).toContain('（空行）')
  })

  it('未识别行灰色展示原文并带未识别标注，提供复制单行', async () => {
    const wrapper = createWrapper()
    await flushPromises()

    const unknownRow = colRows(wrapper, 1)[3]
    expect(unknownRow.find('.el-tag').text()).toBe('未识别')
    expect(unknownRow.text()).toContain('broken-line-without-ip')
  })

  it('过滤关键字命中 IP', async () => {
    const wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('input[type="text"]').setValue('10.20.30')
    await flushPromises()

    const ipRows = colRows(wrapper, 0)
    expect(ipRows.length).toBe(1)
    expect(ipRows[0].text()).toContain('10.20.30.40')
  })

  it('过滤关键字命中主机名，大小写不敏感', async () => {
    const wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('input[type="text"]').setValue('LOCALHOST')
    await flushPromises()

    expect(colRows(wrapper, 0).length).toBe(1)
    expect(colRows(wrapper, 0)[0].text()).toContain('127.0.0.1')
  })

  it('显示注释开启时纯注释行参与关键字过滤（按原文匹配）', async () => {
    const wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('.el-checkbox input').setValue(true)
    await wrapper.find('input[type="text"]').setValue('屏蔽')
    await flushPromises()

    // 命中的注释行：IP 列空、原文渲染在注释列（列序 2）
    expect(colRows(wrapper, 0).length).toBe(1)
    expect(colRows(wrapper, 2)[0].text()).toContain('# 屏蔽广告域名示例')
  })

  it('过滤无命中时展示空态文案', async () => {
    const wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('input[type="text"]').setValue('zzz-not-exist')
    await flushPromises()

    expect(wrapper.find('.empty-text').text()).toBe('未找到匹配的条目')
  })

  it('复制单行复制原始行文本并成功提示', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const wrapper = createWrapper()
    await flushPromises()

    // 操作列第 2 行（::1 entry）的「复制单行」
    const opButtons = colRows(wrapper, 3)[1].findAll('button')
    await opButtons[0].trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('::1 ip6-local # IPv6 回环')
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已复制单行')
  })

  it('复制 IP 仅 entry 行提供且复制地址 token', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const wrapper = createWrapper()
    await flushPromises()

    // entry 行两个按钮（复制单行 + 复制 IP）；未识别行仅复制单行
    const entryOp = colRows(wrapper, 3)[0].findAll('button')
    expect(entryOp.map((b) => b.text())).toEqual(['复制单行', '复制 IP'])
    const unknownOp = colRows(wrapper, 3)[3].findAll('button')
    expect(unknownOp.map((b) => b.text())).toEqual(['复制单行'])

    await entryOp[1].trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('127.0.0.1')
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已复制 127.0.0.1')
  })

  it('clipboard.writeText 失败时提示复制失败', async () => {
    const writeText = vi.fn(() => Promise.reject(new Error('denied')))
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const wrapper = createWrapper()
    await flushPromises()

    await colRows(wrapper, 3)[0].findAll('button')[0].trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('复制失败: denied')
  })

  it('组级 error 显示 warning 行且不渲染表格', async () => {
    getHostsMock.mockResolvedValue({
      path: 'C:\\Windows\\System32\\drivers\\etc\\hosts',
      entries: [],
      error: 'hosts 文件不存在'
    })
    const wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.hosts-error').exists()).toBe(true)
    expect(wrapper.find('.hosts-error').text()).toContain('hosts 文件不存在')
    expect(wrapper.find('.el-table').exists()).toBe(false)
  })

  it('刷新按钮重新读取', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    expect(getHostsMock).toHaveBeenCalledTimes(1)

    await findButton(wrapper, '刷新').trigger('click')
    await flushPromises()
    expect(getHostsMock).toHaveBeenCalledTimes(2)
  })
})
