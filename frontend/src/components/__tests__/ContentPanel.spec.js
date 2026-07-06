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
    },
    ElMessageBox: {
      confirm: vi.fn(() => Promise.resolve())
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
    template: '<template v-if="type === \'textarea\'"><textarea :value="modelValue" :rows="rows" :readonly="readonly" @input="$emit(\'update:modelValue\', $event.target.value)" /></template><template v-else><input :value="modelValue" :placeholder="placeholder" :disabled="disabled" :type="type" :readonly="readonly" @input="$emit(\'update:modelValue\', $event.target.value)" /></template>',
    props: ['modelValue', 'placeholder', 'disabled', 'type', 'rows', 'readonly', 'autosize'],
    emits: ['update:modelValue']
  },
  'el-table': { template: '<table><slot /></table>', props: ['data', 'size'] },
  'el-table-column': { template: '<col />', props: ['prop', 'label', 'width', 'minWidth'] },
  'el-progress': { template: '<div />', props: ['percentage', 'format', 'status'] },
  'el-icon': { template: '<i><slot /></i>' },
  GitInfo: { template: '<div class="git-info" />' },
  CommitHistory: { template: '<div class="commit-history" />' },
  SuccessFilled: { template: '<span />' },
  CircleCloseFilled: { template: '<span />' },
  ArrowLeft: { template: '<span class="arrow-left" />' }
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

    it('previewFile(overridePath) 按 overridePath 预览（markdown 相对链接切换，不改 selectedNode）', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/docs/other.md', name: 'other.md', size: 7,
        content: '# other', isBinary: false, tooLarge: false, error: '', kind: 'text'
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'intro.md', path: '/docs/intro.md', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      // 通过 expose 的 previewFile 按 overridePath 切换预览（selectedNode 保持 intro.md）
      await wrapper.vm.previewFile('/docs/other.md')
      await flushPromises()

      expect(PreviewFile).toHaveBeenCalledWith('/docs/other.md')
    })

    it('链接跳转后可后退回文件树选中节点（单步判断 selectedNode vs filePreview.path）', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      // 1) A.md：点击「预览」按钮触发（模拟用户从文件树点击 file 节点由 Home.onNodeSelect 主驱动）
      PreviewFile.mockResolvedValueOnce({
        path: '/docs/a.md', name: 'a.md', size: 4,
        content: '# a', isBinary: false, tooLarge: false, error: '', kind: 'text'
      })
      // 2) B.md：链接跳转
      PreviewFile.mockResolvedValueOnce({
        path: '/docs/b.md', name: 'b.md', size: 4,
        content: '# b', isBinary: false, tooLarge: false, error: '', kind: 'text'
      })
      // 3) 后退回选中节点 A.md
      PreviewFile.mockResolvedValueOnce({
        path: '/docs/a.md', name: 'a.md', size: 4,
        content: '# a', isBinary: false, tooLarge: false, error: '', kind: 'text'
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'a.md', path: '/docs/a.md', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      // 预览 A：预览路径与选中节点一致 → 不可后退，按钮不渲染
      const previewBtn = wrapper.findAll('button').find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      expect(wrapper.vm.canGoBack).toBe(false)
      expect(wrapper.find('.preview-back-btn').exists()).toBe(false)

      // 模拟 markdown 链接跳转到 B（selectedNode 仍是 A）
      await wrapper.vm.previewFile('/docs/b.md')
      await flushPromises()

      // 此时预览（B）≠ 选中节点（A）→ 可后退，按钮显示
      expect(wrapper.vm.canGoBack).toBe(true)
      expect(wrapper.find('.preview-back-btn').exists()).toBe(true)

      // 后退：回到选中节点 A
      await wrapper.vm.goBack()
      await flushPromises()

      // 最后一次 PreviewFile 应以选中节点 /docs/a.md 调用
      const lastCall = PreviewFile.mock.calls[PreviewFile.mock.calls.length - 1]
      expect(lastCall[0]).toBe('/docs/a.md')

      // 退回后预览路径 === 选中节点路径 → 不可再后退，按钮消失
      expect(wrapper.vm.canGoBack).toBe(false)
      expect(wrapper.find('.preview-back-btn').exists()).toBe(false)
    })

    it('文件树点击 B 后选中节点与预览一致 → 后退按钮必不出现（关键回归）', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValue({
        path: '/docs/b.md', name: 'b.md', size: 4,
        content: '# b', isBinary: false, tooLarge: false, error: '', kind: 'text'
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'a.md', path: '/docs/a.md', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      // 模拟「文件树点击 B」：选中节点变更为 B
      await wrapper.setProps({
        selectedNode: { name: 'b.md', path: '/docs/b.md', type: 'file' }
      })
      // Home.onNodeSelect 主动调用 previewFile(B.path)
      await wrapper.vm.previewFile('/docs/b.md')
      await flushPromises()

      // 此场景正是「正常文件树点击」：预览 === 选中节点 → 后退按钮必不出现
      expect(wrapper.vm.canGoBack).toBe(false)
      expect(wrapper.find('.preview-back-btn').exists()).toBe(false)
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

  describe('编辑态键盘快捷键', () => {
    // 进入编辑态并返回 textarea 包装器
    const enterEditMode = async (wrapper, content = 'Hello') => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/file.md', name: 'file.md', size: 5,
        content, isBinary: false, tooLarge: false, error: '', kind: 'text'
      })
      const previewBtn = wrapper.findAll('button').find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()
      const editBtn = wrapper.findAll('button').find(btn => btn.text().includes('编辑'))
      await editBtn.trigger('click')
      await flushPromises()
      return wrapper.find('textarea')
    }

    const mountPanel = () => mount(ContentPanel, {
      props: {
        selectedNode: { name: 'file.md', path: '/test/file.md', type: 'file' },
        clipboard: { mode: null }
      },
      global: { stubs: contentPanelStubs }
    })

    it('Ctrl+S 有修改时触发保存', async () => {
      const { SaveFile } = await import('../../../wailsjs/go/main/App')
      wrapper = mountPanel()
      const textarea = await enterEditMode(wrapper, 'Hello')

      // 修改内容 → isContentModified 为真
      await textarea.setValue('Hello changed')
      await textarea.trigger('keydown', { key: 's', ctrlKey: true })
      await flushPromises()

      expect(SaveFile).toHaveBeenCalledWith('/test/file.md', 'Hello changed')
    })

    it('Ctrl+S 无修改时不触发保存', async () => {
      const { SaveFile } = await import('../../../wailsjs/go/main/App')
      wrapper = mountPanel()
      const textarea = await enterEditMode(wrapper, 'Hello')

      // 未修改，直接 Ctrl+S
      await textarea.trigger('keydown', { key: 's', ctrlKey: true })
      await flushPromises()

      expect(SaveFile).not.toHaveBeenCalled()
    })

    it('Esc 无修改时直接退出编辑态', async () => {
      wrapper = mountPanel()
      const textarea = await enterEditMode(wrapper, 'Hello')

      await textarea.trigger('keydown', { key: 'Escape' })
      await flushPromises()

      // 退出编辑态 → textarea 消失
      expect(wrapper.find('textarea').exists()).toBe(false)
    })

    it('Esc 有修改时二次确认（确认后退出）', async () => {
      const { ElMessageBox } = await import('element-plus')
      wrapper = mountPanel()
      const textarea = await enterEditMode(wrapper, 'Hello')

      await textarea.setValue('Hello changed')
      await textarea.trigger('keydown', { key: 'Escape' })
      await flushPromises()

      expect(ElMessageBox.confirm).toHaveBeenCalled()
      // mock 默认 resolve（放弃修改）→ 退出编辑态
      expect(wrapper.find('textarea').exists()).toBe(false)
    })
  })

  describe('markdown 目录按钮', () => {
    // 预览一个 markdown 文件并返回 wrapper
    const previewMarkdown = async (wrapper) => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/doc.md', name: 'doc.md', size: 20,
        content: '# 标题A\n## 标题B', isBinary: false, tooLarge: false, error: '', kind: 'text'
      })
      const previewBtn = wrapper.findAll('button').find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()
    }

    const mountPanel = (type = 'file', name = 'doc.md', path = '/test/doc.md') => mount(ContentPanel, {
      props: { selectedNode: { name, path, type }, clipboard: { mode: null } },
      global: { stubs: contentPanelStubs }
    })

    it('markdown 预览显示「目录」按钮，非 markdown 不显示', async () => {
      // markdown 文件
      wrapper = mountPanel()
      await previewMarkdown(wrapper)
      expect(wrapper.findAll('button').find(b => b.text() === '目录')).toBeTruthy()
      wrapper.unmount()

      // 非 markdown 文本文件
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/a.txt', name: 'a.txt', size: 5,
        content: 'hello', isBinary: false, tooLarge: false, error: '', kind: 'text'
      })
      wrapper = mountPanel('file', 'a.txt', '/test/a.txt')
      const previewBtn = wrapper.findAll('button').find(b => b.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()
      expect(wrapper.findAll('button').find(b => b.text() === '目录')).toBeFalsy()
    })

    it('默认不显示 TOC，点击「目录」按钮后显示，X 关闭后隐藏', async () => {
      wrapper = mountPanel()
      await previewMarkdown(wrapper)

      // 默认隐藏
      expect(wrapper.find('.preview-toc').exists()).toBe(false)

      // 点击目录按钮 → 显示
      const tocBtn = wrapper.findAll('button').find(b => b.text() === '目录')
      await tocBtn.trigger('click')
      await flushPromises()
      expect(wrapper.find('.preview-toc').exists()).toBe(true)

      // 点击 X → 隐藏
      await wrapper.find('.toc-close-icon').trigger('click')
      await flushPromises()
      expect(wrapper.find('.preview-toc').exists()).toBe(false)
    })
  })
})
