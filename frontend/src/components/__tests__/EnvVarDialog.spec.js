import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import EnvVarDialog from '../EnvVarDialog.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

const listEnvVarsMock = vi.fn()

vi.mock('../../../wailsjs/go/main/App', () => ({
  ListEnvVars: (...args) => listEnvVarsMock(...args)
}))

// 图标按需 mock：EnvVarDialog 用 Refresh / WarningFilled / View / Hide
vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { name: 'Refresh', template: '<span class="i-refresh" />' },
  WarningFilled: { name: 'WarningFilled', template: '<span class="i-warning" />' },
  View: { name: 'View', template: '<span class="i-view" />' },
  Hide: { name: 'Hide', template: '<span class="i-hide" />' }
}))

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="ev-dialog"><slot name="header" /><slot /></div>',
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
    // emits 声明必须存在：@click 属声明事件才不落入 $attrs，否则 v-bind="$attrs" 把
    // onClick 直接挂到 button 上与 $emit 双路径各触发一次（toggle 显隐抵消、刷新计数翻倍）
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'size', 'type', 'link', 'loading'],
    emits: ['click']
  },
  'el-icon': { template: '<i><slot /></i>' },
  'el-tabs': { template: '<div class="el-tabs"><slot /></div>', props: ['modelValue'] },
  // 组 stub：全部渲染（不做激活态裁剪），data-name 供按分组圈定断言范围
  'el-tab-pane': {
    template: '<div class="el-tab-pane" :data-name="name"><span class="tab-label">{{ label }}</span><slot /></div>',
    props: ['label', 'name']
  },
  'el-input': {
    template: '<input type="text" :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'clearable', 'size']
  },
  'el-table': {
    template: '<div class="el-table"><slot /><span v-if="emptyText && data && data.length === 0" class="empty-text">{{ emptyText }}</span></div>',
    props: ['data', 'size', 'border', 'maxHeight', 'emptyText']
  },
  // 行 stub：遍历 el-table 的 data 逐行渲染 scoped slot
  'el-table-column': {
    props: ['prop', 'label', 'width', 'minWidth', 'fixed', 'showOverflowTooltip'],
    template: '<div class="el-table-col"><span class="col-label">{{ label }}</span><div v-for="(row, i) in $parent.data" :key="i" class="ev-row"><template v-if="prop">{{ row[prop] }}</template><slot :row="row" /></div></div>'
  }
}

// 形状对齐 model.EnvVarGroup / EnvVarInfo：组内按名称大小写不敏感排序（对齐后端
// buildEnvVarInfos），敏感项 value 恒为真实值（打码是前端展示层职责）
const mockGroups = [
  {
    source: 'system',
    name: '系统环境变量',
    vars: [
      { name: 'JAVA_HOME', value: 'C:\\Program Files\\Java\\jdk-17', sensitive: false },
      { name: 'PATH', value: 'C:\\Windows\\system32;C:\\Windows', sensitive: false }
    ],
    error: ''
  },
  {
    source: 'user',
    name: '用户环境变量',
    vars: [
      { name: 'ANTHROPIC_API_KEY', value: 'sk-ant-fake-123456', sensitive: true },
      { name: 'TEMP', value: 'C:\\Users\\demo\\AppData\\Local\\Temp', sensitive: false }
    ],
    error: ''
  },
  {
    source: 'process',
    name: '进程环境变量',
    vars: [
      { name: 'FAKE_TOKEN', value: 'tok-fake-abcdef', sensitive: true },
      { name: 'NUMBER_OF_PROCESSORS', value: '8', sensitive: false }
    ],
    error: ''
  }
]

const createWrapper = (visible = true) =>
  mount(EnvVarDialog, {
    props: { modelValue: visible },
    global: { stubs }
  })

const pane = (wrapper, name) => wrapper.find(`.el-tab-pane[data-name="${name}"]`)

// 指定分组的列行集（列序：名称 0 / 值 1 / 操作 2）
const colRows = (wrapper, source, colIndex) =>
  pane(wrapper, source).findAll('.el-table-col')[colIndex].findAll('.ev-row')

const valueRows = (wrapper, source) => colRows(wrapper, source, 1)
const opRows = (wrapper, source) => colRows(wrapper, source, 2)

const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text() === text)

describe('EnvVarDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listEnvVarsMock.mockResolvedValue(mockGroups)
  })

  it('打开时枚举环境变量并按来源渲染三个 tab 与总览计数', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    expect(listEnvVarsMock).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.el-tab-pane').length).toBe(3)
    expect(wrapper.find('.tab-label').text()).toBe('系统环境变量')
    expect(pane(wrapper, 'system').text()).toContain('系统环境变量')
    expect(pane(wrapper, 'user').text()).toContain('用户环境变量')
    expect(pane(wrapper, 'process').text()).toContain('进程环境变量')
    // 总览：来源数与变量数
    expect(wrapper.find('.ev-meta').text()).toContain('共 3 个来源、6 个变量')
  })

  it('未打开时不枚举环境变量', async () => {
    createWrapper(false)
    await flushPromises()
    expect(listEnvVarsMock).not.toHaveBeenCalled()
  })

  it('仅单来源分组时只渲染一个 tab（Linux 形态）', async () => {
    listEnvVarsMock.mockResolvedValue([
      { source: 'process', name: '进程环境变量', vars: [{ name: 'HOME', value: '/root', sensitive: false }], error: '' }
    ])
    const wrapper = createWrapper(true)
    await flushPromises()

    expect(wrapper.findAll('.el-tab-pane').length).toBe(1)
    expect(pane(wrapper, 'process').exists()).toBe(true)
    expect(valueRows(wrapper, 'process').length).toBe(1)
  })

  it('敏感项默认打码（首字符保留 + 打码位），非敏感项直接展示', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    const maskedRow = valueRows(wrapper, 'process')[0]
    expect(maskedRow.text()).toContain('t••••••')
    expect(maskedRow.text()).not.toContain('tok-fake-abcdef')
    const plainRow = valueRows(wrapper, 'process')[1]
    expect(plainRow.text()).toContain('8')
  })

  it('点击眼睛图标临时显示真实值，再次点击恢复打码', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await valueRows(wrapper, 'process')[0].find('button').trigger('click')
    await flushPromises()
    expect(valueRows(wrapper, 'process')[0].text()).toContain('tok-fake-abcdef')

    await valueRows(wrapper, 'process')[0].find('button').trigger('click')
    await flushPromises()
    expect(valueRows(wrapper, 'process')[0].text()).not.toContain('tok-fake-abcdef')
    expect(valueRows(wrapper, 'process')[0].text()).toContain('••••••')
  })

  it('刷新后重置临时显值态', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await valueRows(wrapper, 'process')[0].find('button').trigger('click')
    await flushPromises()
    expect(valueRows(wrapper, 'process')[0].text()).toContain('tok-fake-abcdef')

    await findButton(wrapper, '刷新').trigger('click')
    await flushPromises()
    expect(valueRows(wrapper, 'process')[0].text()).not.toContain('tok-fake-abcdef')
  })

  it('复制单值始终复制真实值（打码状态下也复制真实值）并成功提示', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const wrapper = createWrapper(true)
    await flushPromises()

    // 打码行（未点击显值）复制：取真实值
    await opRows(wrapper, 'process')[0].find('button').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('tok-fake-abcdef')
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已复制 FAKE_TOKEN')
  })

  it('clipboard.writeText 失败时提示复制失败', async () => {
    const writeText = vi.fn(() => Promise.reject(new Error('denied')))
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const wrapper = createWrapper(true)
    await flushPromises()

    await opRows(wrapper, 'process')[0].find('button').trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('复制失败: denied')
  })

  it('过滤关键字命中名称，大小写不敏感，计数随之变化', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    const input = pane(wrapper, 'system').find('input')
    await input.setValue('path')
    await flushPromises()

    const nameRows = colRows(wrapper, 'system', 0)
    expect(nameRows.length).toBe(1)
    expect(nameRows[0].text()).toContain('PATH')
    expect(pane(wrapper, 'system').text()).toContain('1 / 2 个变量')
  })

  it('过滤关键字命中值', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await pane(wrapper, 'system').find('input').setValue('jdk-17')
    await flushPromises()

    const nameRows = colRows(wrapper, 'system', 0)
    expect(nameRows.length).toBe(1)
    expect(nameRows[0].text()).toContain('JAVA_HOME')
  })

  it('过滤无命中时展示空态文案', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()

    await pane(wrapper, 'system').find('input').setValue('zzz-not-exist')
    await flushPromises()

    expect(pane(wrapper, 'system').find('.empty-text').text()).toBe('未找到匹配的变量')
  })

  it('组级 Error 显示 warning 行且该组不渲染表格，其余组正常', async () => {
    listEnvVarsMock.mockResolvedValue([
      { source: 'system', name: '系统环境变量', vars: [], error: '注册表读取失败：拒绝访问' },
      { source: 'process', name: '进程环境变量', vars: [{ name: 'PATH', value: '/usr/bin', sensitive: false }], error: '' }
    ])
    const wrapper = createWrapper(true)
    await flushPromises()

    const errPane = pane(wrapper, 'system')
    expect(errPane.find('.ev-group-error').exists()).toBe(true)
    expect(errPane.find('.ev-group-error').text()).toContain('注册表读取失败：拒绝访问')
    expect(errPane.find('.el-table').exists()).toBe(false)

    // 其余组正常渲染（单来源失败不拖垮整体）
    expect(pane(wrapper, 'process').find('.el-table').exists()).toBe(true)
    expect(valueRows(wrapper, 'process').length).toBe(1)
  })

  it('整体枚举失败（全部来源失败）走 handleError 提示并展示空态', async () => {
    listEnvVarsMock.mockRejectedValue({ code: 'E_ENV_ENUM_FAILED', message: '环境变量枚举失败，请重试' })
    const wrapper = createWrapper(true)
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('加载失败: 环境变量枚举失败，请重试')
    expect(ElMessage.warning).not.toHaveBeenCalled()
    // 空态兜底：无分组可展示
    expect(wrapper.find('.ev-empty').exists()).toBe(true)
  })

  it('刷新按钮重新枚举', async () => {
    const wrapper = createWrapper(true)
    await flushPromises()
    expect(listEnvVarsMock).toHaveBeenCalledTimes(1)

    await findButton(wrapper, '刷新').trigger('click')
    await flushPromises()
    expect(listEnvVarsMock).toHaveBeenCalledTimes(2)
  })

  it('空值变量展示占位（空）', async () => {
    listEnvVarsMock.mockResolvedValue([
      { source: 'process', name: '进程环境变量', vars: [{ name: 'EMPTY_VAR', value: '', sensitive: false }], error: '' }
    ])
    const wrapper = createWrapper(true)
    await flushPromises()

    expect(valueRows(wrapper, 'process')[0].text()).toContain('（空）')
  })
})
