import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import ElementPlus from 'element-plus'
import AiFunctionConfigDialog from '../AiFunctionConfigDialog.vue'

// seed 配置（覆盖 file/form/none + followUps + followUps.input=text 四种形态）
const seedData = [
  {
    id: 'speech-doc',
    name: '文档转 HTML 发言稿',
    icon: 'Microphone',
    command: '/ab-office:agree-slides',
    cwd: 'D:\\proj',
    permissionMode: 'bypassPermissions',
    timeoutMinutes: 15,
    completion: 'preview',
    params: {
      type: 'file',
      label: '选择源文档',
      startDir: 'D:\\工作\\Typora',
      extensions: ['.md', '.docx'],
      textFieldKey: 'file'
    },
    followUps: []
  },
  {
    id: 'weekly-report',
    name: '生成周报',
    icon: 'Calendar',
    command: '/ab-weekly-report',
    cwd: 'D:\\proj',
    permissionMode: 'bypassPermissions',
    timeoutMinutes: 20,
    completion: 'open_dir',
    params: null,
    followUps: [{ id: 'confirm-finalize', label: '确认落盘终稿', promptTemplate: '落盘', input: null }]
  },
  {
    id: 'meeting-book',
    name: '预约腾讯会议',
    icon: 'VideoCamera',
    command: '/tencent-meeting-mcp',
    cwd: 'D:\\proj',
    permissionMode: 'bypassPermissions',
    timeoutMinutes: 5,
    completion: 'copy',
    params: {
      type: 'form',
      label: '会议信息',
      promptTemplate: '主题{{topic}} 时间{{startTime}} 时长{{duration}}',
      fields: [
        { key: 'topic', label: '主题', type: 'text', required: true },
        { key: 'startTime', label: '开始时间', type: 'datetime', required: true },
        { key: 'duration', label: '时长', type: 'number', required: true }
      ]
    },
    followUps: []
  },
  {
    id: 'meeting-list',
    name: '查看/取消腾讯会议',
    icon: 'Clock',
    command: '/tencent-meeting-mcp',
    cwd: 'D:\\proj',
    permissionMode: 'bypassPermissions',
    timeoutMinutes: 5,
    completion: 'none',
    params: { type: 'none' },
    followUps: [
      {
        id: 'cancel-meeting',
        label: '取消会议',
        promptTemplate: '取消 {{meeting}}',
        input: { type: 'text', label: '要取消的会议', textFieldKey: 'meeting' }
      }
    ]
  }
]

const saveAiFunctionsMock = vi.fn(() => Promise.resolve())
vi.mock('../../../wailsjs/go/main/App', () => ({
  GetAiFunctions: vi.fn(() => Promise.resolve(seedData)),
  SaveAiFunctions: (...args) => saveAiFunctionsMock(...args)
}))

// stub 外壳/折叠组件（避免 teleport 与 transition 在 jsdom 下的问题）；
// el-input/el-select/el-form/el-switch 等用真实 Element Plus（plugins 注入）
const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="dlg"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'appendToBody', 'destroyOnClose']
  },
  'el-collapse': { template: '<div class="collapse"><slot /></div>', props: ['modelValue'] },
  'el-collapse-item': {
    template: '<div class="collapse-item" :data-name="name"><slot /></div>',
    props: ['title', 'name']
  }
}

const createWrapper = async () => {
  // visible 初始 false 再 setProps true，触发 watch(props.visible) 调 load 拉取 seed
  const wrapper = mount(AiFunctionConfigDialog, {
    props: { visible: false },
    global: { plugins: [ElementPlus], stubs }
  })
  await flushPromises()
  await wrapper.setProps({ visible: true })
  await flushPromises()
  return wrapper
}

const clickItem = async (wrapper, id) => {
  const items = wrapper.findAll('.config-item')
  const target = items.find((n) => n.text().includes(id))
  await target.trigger('click')
  await flushPromises()
}

const clickSave = async (wrapper) => {
  const btns = wrapper.findAll('button')
  const saveBtn = btns.find((b) => b.text().includes('保存全部'))
  await saveBtn.trigger('click')
  await flushPromises()
}

// 找承载四块字段的原始 JSON textarea（含 "params" 键的那个，区别于 promptTemplate 等）
const findRawJsonArea = (wrapper) =>
  wrapper.findAll('textarea').find((t) => t.element.value.includes('"params"'))

beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(ElMessage, 'warning').mockImplementation(() => {})
  vi.spyOn(ElMessage, 'success').mockImplementation(() => {})
  vi.spyOn(ElMessage, 'error').mockImplementation(() => {})
})

describe('AiFunctionConfigDialog', () => {
  it('加载后列表显示四项 seed', async () => {
    const wrapper = await createWrapper()
    const items = wrapper.findAll('.config-item')
    expect(items.length).toBe(4)
    expect(wrapper.text()).toContain('文档转 HTML 发言稿')
    expect(wrapper.text()).toContain('生成周报')
  })

  it('选中 form 类型功能项（meeting-book）回填 promptTemplate 与 fields', async () => {
    const wrapper = await createWrapper()
    await clickItem(wrapper, 'meeting-book')
    // ParamsEditor 渲染 form 子表单：promptTemplate textarea 含 {{topic}}
    const templateArea = wrapper.findAll('textarea').find((t) =>
      t.element.value.includes('{{topic}}')
    )
    expect(templateArea).toBeTruthy()
    // fields 列表 3 项（key 输入框值为 topic/startTime/duration）
    const inputs = wrapper.findAll('input')
    const keys = inputs.filter((i) => ['topic', 'startTime', 'duration'].includes(i.element.value))
    expect(keys.length).toBe(3)
  })

  it('form 模式 promptTemplate 占位符与 fields 不一致时告警（不阻断保存）', async () => {
    const wrapper = await createWrapper()
    await clickItem(wrapper, 'meeting-book')
    const templateArea = wrapper.findAll('textarea').find((t) =>
      t.element.value.includes('{{topic}}')
    )
    await templateArea.setValue('主题{{topic}} 额外{{undefinedField}}')
    await flushPromises()
    expect(wrapper.text()).toContain('未在字段列表中定义')
    // 占位符告警不阻断保存
    await clickSave(wrapper)
    expect(saveAiFunctionsMock).toHaveBeenCalled()
  })

  it('选中 meeting-list 回填 followUps（cancel-meeting）与 input.text 配置', async () => {
    const wrapper = await createWrapper()
    await clickItem(wrapper, 'meeting-list')
    expect(wrapper.find('.fu-title').text()).toBe('取消会议')
    // input 配置 ParamsEditor 渲染（text 模式，textFieldKey=meeting）
    const meetingKey = wrapper.findAll('input').find((i) => i.element.value === 'meeting')
    expect(meetingKey).toBeTruthy()
  })

  it('followUps 字段级校验：清空 label 后保存失败，提示定位 followUps[0].label', async () => {
    const wrapper = await createWrapper()
    await clickItem(wrapper, 'meeting-list')
    // 清空 label（值为"取消会议"的那个 input）
    const labelInput = wrapper.findAll('input').find((i) => i.element.value === '取消会议')
    await labelInput.setValue('')
    await flushPromises()
    await clickSave(wrapper)
    expect(ElMessage.warning).toHaveBeenCalled()
    expect(ElMessage.warning.mock.calls[0][0]).toContain('followUps[0].label')
    expect(saveAiFunctionsMock).not.toHaveBeenCalled()
  })

  it('mcp http 校验：配 http server 无 url 保存失败提示定位', async () => {
    const wrapper = await createWrapper()
    await clickItem(wrapper, 'speech-doc')
    const rawArea = findRawJsonArea(wrapper)
    await rawArea.setValue(JSON.stringify({
      params: null,
      followUps: [],
      env: null,
      mcp: { mcpServers: { web: { type: 'http', url: '' } } }
    }))
    await rawArea.trigger('change')
    await flushPromises()
    await clickSave(wrapper)
    expect(ElMessage.warning).toHaveBeenCalled()
    expect(ElMessage.warning.mock.calls[0][0]).toContain('web')
    expect(ElMessage.warning.mock.calls[0][0]).toContain('url')
    expect(saveAiFunctionsMock).not.toHaveBeenCalled()
  })

  it('mcp stdio 录入：无 command 校验失败，填 command 后保存成功', async () => {
    const wrapper = await createWrapper()
    await clickItem(wrapper, 'speech-doc')
    const rawArea = findRawJsonArea(wrapper)
    // stdio 无 command → 校验失败
    await rawArea.setValue(JSON.stringify({
      params: null,
      followUps: [],
      env: null,
      mcp: { mcpServers: { fs: { type: 'stdio', command: '' } } }
    }))
    await rawArea.trigger('change')
    await flushPromises()
    await clickSave(wrapper)
    expect(ElMessage.warning).toHaveBeenCalled()
    expect(ElMessage.warning.mock.calls[0][0]).toContain('stdio')
    expect(ElMessage.warning.mock.calls[0][0]).toContain('command')
    expect(saveAiFunctionsMock).not.toHaveBeenCalled()

    // stdio 有 command → 保存成功
    await rawArea.setValue(JSON.stringify({
      params: null,
      followUps: [],
      env: null,
      mcp: { mcpServers: { fs: { type: 'stdio', command: 'npx', args: ['-y', 'server'] } } }
    }))
    await rawArea.trigger('change')
    await flushPromises()
    await clickSave(wrapper)
    expect(saveAiFunctionsMock).toHaveBeenCalled()
  })

  it('原始 JSON 与表单双向同步：表单改 → JSON 更新', async () => {
    const wrapper = await createWrapper()
    await clickItem(wrapper, 'weekly-report')
    // 改 followUps[0].label（值为"确认落盘终稿"的那个 input）
    const labelInput = wrapper.findAll('input').find((i) => i.element.value === '确认落盘终稿')
    await labelInput.setValue('确认并落盘')
    await flushPromises()
    // 原始 JSON 自动同步含新 label
    const rawArea = findRawJsonArea(wrapper)
    expect(rawArea.element.value).toContain('确认并落盘')
  })

  it('原始 JSON 解析失败标红且表单保持原值，保存不崩', async () => {
    const wrapper = await createWrapper()
    await clickItem(wrapper, 'weekly-report')
    const rawArea = findRawJsonArea(wrapper)
    // 输入非法 JSON
    await rawArea.setValue('{ invalid json }}}')
    await rawArea.trigger('change')
    await flushPromises()
    expect(wrapper.find('.raw-error-msg').exists()).toBe(true)
    expect(wrapper.text()).toContain('JSON 解析失败')
    // 保存走 editing.value（未被破坏），不崩
    await clickSave(wrapper)
    expect(saveAiFunctionsMock).toHaveBeenCalled()
  })

  it('保存成功调用 SaveAiFunctions 并 emit saved', async () => {
    const wrapper = await createWrapper()
    await clickItem(wrapper, 'weekly-report')
    await clickSave(wrapper)
    expect(saveAiFunctionsMock).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('saved')).toBeTruthy()
    expect(wrapper.emitted('update:visible')).toBeTruthy()
  })
})
