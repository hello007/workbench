import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import ElementPlus from 'element-plus'
import AiFunctionCreateDialog from '../AiFunctionCreateDialog.vue'

// 生成草稿（形状对齐 model.AiFunctionDraftResult.draft）
const draftFixture = {
  id: 'weekly-report',
  name: '生成周报',
  description: '把会议纪要转成周报',
  icon: 'Calendar',
  command: '',
  cwd: '',
  addDirs: [],
  tags: ['周报'],
  pinned: false,
  permissionMode: 'bypassPermissions',
  timeoutMinutes: 5,
  completion: 'none',
  params: { type: 'form', label: '输入', promptTemplate: '请生成周报：{{topic}}', fields: [] },
  followUps: []
}

const generateMock = vi.fn(() => Promise.resolve({ draft: draftFixture, warnings: [], renamed: false }))
const cancelMock = vi.fn(() => Promise.resolve(true))
const openDirMock = vi.fn(() => Promise.resolve(''))
const discoveredGlobal = [
  { name: 'drawio', description: '画图工具', command: '/drawio', cwd: '', source: 'user', sourceDir: '/h/skills/drawio', plugin: '' }
]
const discoveredInDir = [
  { name: 'release', description: '发版流程', command: '/release', cwd: 'D:\\work', source: 'project', sourceDir: 'D:\\work\\.claude\\skills\\release', plugin: '' }
]
const getDiscoveredMock = vi.fn(() => Promise.resolve(discoveredGlobal))
const getDiscoveredInMock = vi.fn(() => Promise.resolve(discoveredInDir))

vi.mock('../../../wailsjs/go/main/App', () => ({
  GenerateAiFunctionDraft: (...args) => generateMock(...args),
  CancelAiFunctionDraft: () => cancelMock(),
  GetDiscoveredSkills: () => getDiscoveredMock(),
  GetDiscoveredSkillsIn: (...args) => getDiscoveredInMock(...args),
  OpenDirectoryDialog: (...args) => openDirMock(...args)
}))

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="dlg"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'appendToBody', 'destroyOnClose', 'closeOnClickModal'],
    // closed 由用例经 stub 实例 $emit 触发，确保组件模板上的 @closed 绑定本身被覆盖
    emits: ['closed', 'update:modelValue']
  },
  // el-select / el-option 走轻量桩：候选内容经 vm.skills 断言，避免 popper 在 jsdom 下的开销
  'el-select': {
    template: '<div class="sel"><slot /></div>',
    props: ['modelValue', 'loading', 'placeholder', 'clearable', 'filterable', 'disabled', 'size']
  },
  'el-option': { template: '<div class="opt"></div>', props: ['label', 'value'] }
}

const createWrapper = async (visible = true) => {
  const wrapper = mount(AiFunctionCreateDialog, {
    props: { visible: false },
    global: { plugins: [ElementPlus], stubs }
  })
  await flushPromises()
  if (visible) {
    await wrapper.setProps({ visible: true })
    await flushPromises()
  }
  return wrapper
}

const clickBtn = async (wrapper, text) => {
  const btn = wrapper.findAll('button').find((b) => b.text() === text)
  expect(btn, `未找到按钮「${text}」`).toBeTruthy()
  await btn.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(ElMessage, 'warning').mockImplementation(() => {})
  vi.spyOn(ElMessage, 'success').mockImplementation(() => {})
  vi.spyOn(ElMessage, 'error').mockImplementation(() => {})
  generateMock.mockImplementation(() => Promise.resolve({ draft: draftFixture, warnings: [], renamed: false }))
  cancelMock.mockImplementation(() => Promise.resolve(true))
  openDirMock.mockImplementation(() => Promise.resolve(''))
  getDiscoveredMock.mockImplementation(() => Promise.resolve(discoveredGlobal))
  getDiscoveredInMock.mockImplementation(() => Promise.resolve(discoveredInDir))
})

describe('AiFunctionCreateDialog', () => {
  it('打开时加载全局 skill 候选（未选目录走 GetDiscoveredSkills）', async () => {
    const wrapper = await createWrapper()
    expect(getDiscoveredMock).toHaveBeenCalled()
    expect(getDiscoveredInMock).not.toHaveBeenCalled()
    expect(wrapper.vm.skills.map((s) => s.command)).toEqual(['/drawio'])
  })

  it('需求为空时「生成」按钮禁用，后端不被调用', async () => {
    const wrapper = await createWrapper()
    const btn = wrapper.findAll('button').find((b) => b.text() === '生成')
    expect(btn.attributes('disabled')).toBeDefined()
    await btn.trigger('click')
    expect(generateMock).not.toHaveBeenCalled()
  })

  it('生成成功展示草稿预览，「填入配置」emit created 并关闭弹窗', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('把会议纪要转成周报')
    await clickBtn(wrapper, '生成')

    expect(generateMock).toHaveBeenCalledTimes(1)
    expect(generateMock).toHaveBeenCalledWith({
      description: '把会议纪要转成周报',
      cwd: '',
      skillCommand: ''
    })

    // 预览展示关键字段
    expect(wrapper.find('.draft-preview').exists()).toBe(true)
    expect(wrapper.text()).toContain('weekly-report')
    expect(wrapper.text()).toContain('生成周报')
    expect(wrapper.text()).toContain('form（表单字段）')

    await clickBtn(wrapper, '填入配置')
    const created = wrapper.emitted('created')
    expect(created).toBeTruthy()
    expect(created[0][0].id).toBe('weekly-report')
    expect(wrapper.emitted('update:visible').at(-1)).toEqual([false])
  })

  it('重命名与警告在预览中展示', async () => {
    generateMock.mockResolvedValueOnce({
      draft: { ...draftFixture, id: 'weekly-report-2' },
      warnings: ['命令 /x 不在 skill 候选清单中', 'ID 已自动重命名'],
      renamed: true,
      originalId: 'weekly-report'
    })
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('写周报')
    await clickBtn(wrapper, '生成')

    expect(wrapper.find('[data-testid="rename-hint"]').text()).toContain('weekly-report-2')
    expect(wrapper.text()).toContain('不在 skill 候选清单中')
    expect(wrapper.text()).toContain('ID 已自动重命名')
  })

  it('选择目录后调 GetDiscoveredSkillsIn 刷新候选并清空失配的已选项', async () => {
    openDirMock.mockResolvedValueOnce('D:\\work')
    const wrapper = await createWrapper()
    // 先选一个全局 skill
    const vm = wrapper.vm
    vm.skillCommand = '/drawio'
    await flushPromises()

    await clickBtn(wrapper, '选择目录')
    expect(openDirMock).toHaveBeenCalledWith('选择工作目录')
    expect(vm.cwd).toBe('D:\\work')
    expect(getDiscoveredInMock).toHaveBeenCalledWith('D:\\work')
    expect(vm.skills.map((s) => s.command)).toEqual(['/release'])
    // /drawio 不在新候选内 → 清空
    expect(vm.skillCommand).toBe('')
  })

  it('用户取消目录选择（返回空串）不刷新候选', async () => {
    const wrapper = await createWrapper()
    getDiscoveredInMock.mockClear()
    await clickBtn(wrapper, '选择目录')
    expect(wrapper.vm.cwd).toBe('')
    expect(getDiscoveredInMock).not.toHaveBeenCalled()
  })

  it('生成请求携带已选 skill 命令与目录', async () => {
    const wrapper = await createWrapper()
    const vm = wrapper.vm
    vm.cwd = 'D:\\work'
    vm.skillCommand = '/release'
    await wrapper.find('textarea').setValue('发版')
    await flushPromises()
    await clickBtn(wrapper, '生成')
    expect(generateMock).toHaveBeenCalledWith({
      description: '发版',
      cwd: 'D:\\work',
      skillCommand: '/release'
    })
  })

  it('生成中显示进度与「中止生成」，点击调 CancelAiFunctionDraft 且丢弃取消错误', async () => {
    let rejectGen
    generateMock.mockImplementationOnce(
      () => new Promise((_, reject) => { rejectGen = reject })
    )
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('写周报')
    await clickBtn(wrapper, '生成')
    expect(wrapper.find('.gen-progress').exists()).toBe(true)

    await clickBtn(wrapper, '中止生成')
    expect(cancelMock).toHaveBeenCalledTimes(1)

    // 取消错误不弹提示（用户主动中止）
    rejectGen({ code: 'E_X', message: '生成已取消' })
    await flushPromises()
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(wrapper.find('.draft-preview').exists()).toBe(false)
  })

  it('关闭弹窗（closed）时若仍在生成则中止，并复位生成态', async () => {
    generateMock.mockImplementationOnce(() => new Promise(() => {})) // 永不 resolve
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('写周报')
    await clickBtn(wrapper, '生成')
    expect(cancelMock).not.toHaveBeenCalled()

    // 经 stub 触发 @closed，覆盖组件模板上的关闭收尾绑定（而非直调内部方法）
    await wrapper.findComponent(stubs['el-dialog']).vm.$emit('closed')
    await flushPromises()
    expect(cancelMock).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.generating).toBe(false)
  })

  it('生成中重复点击「生成」不会发起第二次请求', async () => {
    generateMock.mockImplementationOnce(() => new Promise(() => {})) // 保持进行中
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('写周报')
    await clickBtn(wrapper, '生成')
    await clickBtn(wrapper, '生成')
    expect(generateMock).toHaveBeenCalledTimes(1)
  })

  it('关闭后旧生成结果不回溯，重开可重新发起生成', async () => {
    let resolveGen
    generateMock.mockImplementationOnce(
      () => new Promise((resolve) => { resolveGen = resolve })
    )
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('写周报')
    await clickBtn(wrapper, '生成')

    await wrapper.findComponent(stubs['el-dialog']).vm.$emit('closed')
    await flushPromises()
    expect(cancelMock).toHaveBeenCalledTimes(1)

    // 旧请求晚到：代次已作废，不得回填预览
    resolveGen({ draft: draftFixture, warnings: [], renamed: false })
    await flushPromises()
    expect(wrapper.find('.draft-preview').exists()).toBe(false)

    // 重开后再次生成正常发起
    await wrapper.setProps({ visible: false })
    await wrapper.setProps({ visible: true })
    await flushPromises()
    await wrapper.find('textarea').setValue('再写一份')
    await clickBtn(wrapper, '生成')
    expect(generateMock).toHaveBeenCalledTimes(2)
  })

  it('旧 skill 请求返回不覆盖新候选、不清空用户已选', async () => {
    let resolveGlobal
    getDiscoveredMock.mockImplementationOnce(
      () => new Promise((resolve) => { resolveGlobal = resolve })
    )
    const wrapper = await createWrapper() // 打开时发起的全局请求保持 pending

    // 切到目录候选：新请求先返回
    wrapper.vm.cwd = 'D:\\work'
    await wrapper.vm.refreshSkills()
    await flushPromises()
    expect(wrapper.vm.skills.map((s) => s.command)).toEqual(['/release'])

    // 用户选中目录内 skill 后，旧全局响应晚到
    wrapper.vm.skillCommand = '/release'
    resolveGlobal(discoveredGlobal)
    await flushPromises()
    expect(wrapper.vm.skills.map((s) => s.command)).toEqual(['/release'])
    expect(wrapper.vm.skillCommand).toBe('/release')
  })

  it('同名 command 多来源候选按 command 去重', async () => {
    getDiscoveredMock.mockResolvedValueOnce([
      { ...discoveredGlobal[0], sourceDir: '/h/skills/drawio' },
      { ...discoveredGlobal[0], sourceDir: '/p/skills/drawio', source: 'plugin', plugin: 'p' }
    ])
    const wrapper = await createWrapper()
    expect(wrapper.vm.skills.map((s) => s.command)).toEqual(['/drawio'])
  })

  it('草稿高级字段（env/mcp/addDirs/followUps/outputSchema）在预览中显式列出', async () => {
    generateMock.mockResolvedValueOnce({
      draft: {
        ...draftFixture,
        env: { FOO: '$ENV:FOO' },
        mcp: { mcpServers: { fs: { type: 'stdio', command: 'npx' } } },
        addDirs: ['D:\\extra'],
        followUps: [{ id: 'confirm', label: '确认', promptTemplate: 'ok', input: null }],
        outputSchema: { type: 'object' }
      },
      warnings: [],
      renamed: false
    })
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('写周报')
    await clickBtn(wrapper, '生成')

    const advanced = wrapper.find('[data-testid="advanced-fields"]')
    expect(advanced.exists()).toBe(true)
    const text = advanced.text()
    expect(text).toContain('环境变量')
    expect(text).toContain('MCP server')
    expect(text).toContain('授权目录')
    expect(text).toContain('后续段')
    expect(text).toContain('输出结构')
    expect(text).toContain('FOO')
    expect(text).toContain('npx')
    // JSON 序列化会转义反斜杠，断言路径主干即可
    expect(text).toContain('extra')
  })

  it('草稿无高级字段时不渲染高级字段区块', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('写周报')
    await clickBtn(wrapper, '生成')
    expect(wrapper.find('[data-testid="advanced-fields"]').exists()).toBe(false)
  })

  it('生成失败经 handleError 提示（后端消息不加多余前缀）', async () => {
    generateMock.mockRejectedValueOnce({ message: '生成超时（上限 2 分钟）' })
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('写周报')
    await clickBtn(wrapper, '生成')
    expect(ElMessage.error).toHaveBeenCalled()
    expect(ElMessage.error.mock.calls[0][0]).toBe('生成超时（上限 2 分钟）')
    expect(wrapper.find('.draft-preview').exists()).toBe(false)
  })

  it('每次打开重置上一次的状态', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('textarea').setValue('写周报')
    await clickBtn(wrapper, '生成')
    expect(wrapper.find('.draft-preview').exists()).toBe(true)

    await wrapper.setProps({ visible: false })
    await flushPromises()
    await wrapper.setProps({ visible: true })
    await flushPromises()
    expect(wrapper.vm.description).toBe('')
    expect(wrapper.find('.draft-preview').exists()).toBe(false)
  })
})
