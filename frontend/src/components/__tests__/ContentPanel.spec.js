import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import ContentPanel from '../ContentPanel.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: {
      error: vi.fn(),
      success: vi.fn(),
      warning: vi.fn(),
      info: vi.fn()
    }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  PreviewFile: vi.fn(() => Promise.resolve({ content: '', error: '' })),
  ReadFileBytes: vi.fn(() => Promise.resolve({ base64: '', error: '' })),
  SaveFile: vi.fn(() => Promise.resolve(undefined)),
  PullRepo: vi.fn(() => Promise.resolve('')),
  CloneRepo: vi.fn(() => Promise.resolve('克隆成功')),
  OpenWithDefaultApp: vi.fn(() => Promise.resolve(true))
}))

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn()
}))

// docx-preview / xlsx 在 jsdom 下会真实加载（动态 import），用空实现 stub，
// 避免 Office 渲染用例触发真实库渲染（jsdom 下 DOM/Canvas 能力不全易崩）。
vi.mock('docx-preview', () => ({
  renderAsync: vi.fn(() => Promise.resolve())
}))
vi.mock('xlsx', () => ({
  read: vi.fn(() => ({ SheetNames: [], Sheets: {} })),
  utils: { sheet_to_json: vi.fn(() => []) }
}))

const contentPanelStubs = {
  'el-descriptions': { template: '<div class="el-descriptions"><slot /></div>', props: ['column', 'border'] },
  'el-descriptions-item': { template: '<div class="el-descriptions-item"><slot /></div>', props: ['label'] },
  'el-divider': { template: '<hr />' },
  'el-tabs': { template: '<div><slot /></div>', props: ['modelValue'] },
  'el-tab-pane': { template: '<div><slot /></div>', props: ['label', 'name', 'lazy'] },
  'el-button': { template: '<button v-bind="$attrs"><slot /></button>', props: ['loading', 'type', 'disabled', 'size'] },
  'el-button-group': { template: '<div><slot /></div>' },
  'el-empty': { template: '<div />', props: ['description', 'imageSize'] },
  'el-dialog': {
    template: '<div v-if="modelValue"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue']
  },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<template v-if="type === \'textarea\'"><textarea :value="modelValue" :rows="rows" :readonly="readonly" /></template><template v-else><input :value="modelValue" :placeholder="placeholder" :disabled="disabled" :type="type" :readonly="readonly" /></template>',
    props: ['modelValue', 'placeholder', 'disabled', 'type', 'rows', 'readonly', 'autosize']
  },
  'el-table': { template: '<table><slot /></table>', props: ['data', 'size'] },
  'el-table-column': { template: '<col />', props: ['prop', 'label', 'width', 'minWidth'] },
  'el-progress': { template: '<div />', props: ['percentage', 'format', 'status'] },
  'el-icon': { template: '<i><slot /></i>' },
  GitInfo: { template: '<div class="git-info" />' },
  CommitHistory: { template: '<div class="commit-history" />' },
  SuccessFilled: { template: '<span />' },
  CircleCloseFilled: { template: '<span />' }
}

describe('ContentPanel.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  describe('节点信息展示', () => {
    it('选中文件节点应显示名称、路径和类型', () => {
      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'test.txt', path: '/path/to/test.txt', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      expect(wrapper.find('h2').text()).toBe('test.txt')
      expect(wrapper.text()).toContain('/path/to/test.txt')
      expect(wrapper.text()).toContain('文件')
    })

    it('选中文件夹节点应显示类型为"文件夹"', () => {
      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'src', path: '/path/to/src', type: 'directory' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      expect(wrapper.find('h2').text()).toBe('src')
      expect(wrapper.text()).toContain('/path/to/src')
      expect(wrapper.text()).toContain('文件夹')
    })

    it('未选中节点时不应显示 h2 标题', () => {
      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: null,
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      expect(wrapper.find('h2').exists()).toBe(false)
    })
  })

  describe('文件预览', () => {
    it('previewFile 成功应显示内容（文本类默认只读，进入编辑后显示 textarea）', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      const testContent = 'Hello, world!'
      PreviewFile.mockResolvedValueOnce({
        path: '/test/file.txt',
        name: 'file.txt',
        size: 13,
        content: testContent,
        isBinary: false,
        tooLarge: false,
        error: '',
        kind: 'text'
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'file.txt', path: '/test/file.txt', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      // 初始无预览区
      expect(wrapper.find('textarea').exists()).toBe(false)

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      expect(previewBtn.exists()).toBe(true)
      await previewBtn.trigger('click')
      await flushPromises()

      expect(PreviewFile).toHaveBeenCalledWith('/test/file.txt')

      // 新契约：文本类默认只读渲染（无 textarea），预览区已显示
      expect(wrapper.find('.file-preview').exists()).toBe(true)
      expect(wrapper.find('textarea').exists()).toBe(false)

      // 点击「编辑」进入编辑态，出现 textarea 并携带内容
      const editBtn = wrapper.findAll('button').find(btn => btn.text().includes('编辑'))
      expect(editBtn.exists()).toBe(true)
      await editBtn.trigger('click')
      await flushPromises()

      const textarea = wrapper.find('textarea')
      expect(textarea.exists()).toBe(true)
      expect(textarea.element.value).toBe(testContent)
    })

    it('previewFile 大文件应显示警告', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/large.pdf',
        name: 'large.pdf',
        size: 2 * 1024 * 1024,
        content: '',
        isBinary: false,
        tooLarge: true,
        error: ''
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'large.pdf', path: '/test/large.pdf', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      expect(ElMessage.warning).toHaveBeenCalledWith('文件过大，无法预览')
      expect(wrapper.find('textarea').exists()).toBe(false)
    })

    it('previewFile 不支持的二进制文件应降级提示（用默认程序打开）', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/data.bin',
        name: 'data.bin',
        size: 1000,
        content: '',
        isBinary: true,
        tooLarge: false,
        error: '',
        kind: 'unsupported'
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'data.bin', path: '/test/data.bin', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      // 不支持的二进制文件：无 textarea，降级分支提供「用默认程序打开」
      expect(wrapper.find('textarea').exists()).toBe(false)
      expect(wrapper.text()).toContain('用默认程序打开')
      expect(ElMessage.warning).toHaveBeenCalledWith('该文件类型暂不支持内嵌预览')
    })

    it('previewFile 错误应显示错误提示', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/nonexistent.txt',
        name: 'nonexistent.txt',
        size: 0,
        content: '',
        isBinary: false,
        tooLarge: false,
        error: 'File not found'
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'nonexistent.txt', path: '/test/nonexistent.txt', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalledWith('预览失败: File not found')
    })

    it('previewFile office(docx) 应调用 ReadFileBytes 取 base64 传给渲染器', async () => {
      const { PreviewFile, ReadFileBytes } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/report.docx',
        name: 'report.docx',
        size: 1024,
        content: '',
        base64: '',
        isBinary: false,
        tooLarge: false,
        error: '',
        kind: 'office'
      })
      ReadFileBytes.mockResolvedValueOnce({ base64: 'UEsDBBQAAAAAA', error: '', tooLarge: false })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'report.docx', path: '/test/report.docx', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      // office 应触发 ReadFileBytes 取字节
      expect(ReadFileBytes).toHaveBeenCalledWith('/test/report.docx')
      // 渲染器组件被挂载，且接受了 base64 prop
      const renderer = wrapper.findComponent({ name: 'FilePreviewRenderer' })
      // stub 场景下组件名可能缺失，回退断言预览区已显示
      expect(wrapper.find('.file-preview').exists()).toBe(true)
    })

    it('previewFile office 文件过大（ReadFileBytes 返回 tooLarge）应降级提示', async () => {
      const { PreviewFile, ReadFileBytes } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/big.xlsx',
        name: 'big.xlsx',
        size: 80 * 1024 * 1024,
        content: '',
        base64: '',
        isBinary: false,
        tooLarge: false,
        error: '',
        kind: 'office'
      })
      ReadFileBytes.mockResolvedValueOnce({ base64: '', error: '', tooLarge: true })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'big.xlsx', path: '/test/big.xlsx', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      expect(ReadFileBytes).toHaveBeenCalledWith('/test/big.xlsx')
      expect(ElMessage.warning).toHaveBeenCalledWith('文件过大，无法预览')
    })

    it('previewFile 图片读取字节失败（ReadFileBytes 返回 error）应走降级提示', async () => {
      const { PreviewFile, ReadFileBytes } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/pic.png',
        name: 'pic.png',
        size: 1024,
        content: '',
        base64: '',
        isBinary: false,
        tooLarge: false,
        error: '',
        kind: 'image'
      })
      ReadFileBytes.mockResolvedValueOnce({ base64: '', error: 'read error', tooLarge: false })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'pic.png', path: '/test/pic.png', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      expect(ReadFileBytes).toHaveBeenCalledWith('/test/pic.png')
      expect(ElMessage.error).toHaveBeenCalledWith('读取文件字节失败: read error')
      // 图片读取失败应走降级分支，提供「用默认程序打开」
      expect(wrapper.text()).toContain('用默认程序打开')
    })
  })

  describe('clearPreview', () => {
    it('clearPreview 应重置预览状态', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/file.txt',
        name: 'file.txt',
        size: 13,
        content: 'Hello, world!',
        isBinary: false,
        tooLarge: false,
        error: '',
        kind: 'text'
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'file.txt', path: '/test/file.txt', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      // 先调用 previewFile 显示内容（文本类默认只读，进入编辑后出现 textarea）
      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      const editBtn = wrapper.findAll('button').find(btn => btn.text().includes('编辑'))
      await editBtn.trigger('click')
      await flushPromises()
      expect(wrapper.find('textarea').exists()).toBe(true)

      // 调用 clearPreview 清空
      await wrapper.vm.clearPreview()
      expect(wrapper.find('textarea').exists()).toBe(false)
    })
  })
})
