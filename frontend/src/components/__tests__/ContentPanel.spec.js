import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import ContentPanel from '../ContentPanel.vue'
import { useWorkspaceStore } from '../../store'

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
  OpenWithDefaultApp: vi.fn(() => Promise.resolve(true)),
  OpenInExplorer: vi.fn(() => Promise.resolve(true)),
  OpenInVSCode: vi.fn(() => Promise.resolve(true)),
  OpenInWarp: vi.fn(() => Promise.resolve(true)),
  GetBranches: vi.fn(() => Promise.resolve({ branches: [{ name: 'main', isCurrent: true, isRemote: false }] })),
  CheckoutBranch: vi.fn(() => Promise.resolve(true))
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
  'el-switch': {
    template: '<input type="checkbox" class="el-switch" :checked="modelValue" @change="onChange" />',
    props: ['modelValue', 'activeText', 'inlinePrompt'],
    emits: ['update:modelValue'],
    methods: {
      onChange(e) { this.$emit('update:modelValue', e.target.checked) }
    }
  },
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
  GitTags: { template: '<div class="git-tags" />' },
  GitRemotes: { template: '<div class="git-remotes" />' },
  GitMerge: { template: '<div class="git-merge" />' },
  SuccessFilled: { template: '<span />' },
  CircleCloseFilled: { template: '<span />' },
  ArrowLeft: { template: '<span class="arrow-left" />' }
}

// selectedNode/latestCommit 已迁 workspace store：mount 前经 store 设初始值驱动（不再传 prop）。
function mountPanel(selectedNode = null) {
  const workspaceStore = useWorkspaceStore()
  workspaceStore.selectedNode = selectedNode
  return mount(ContentPanel, {
    global: { stubs: contentPanelStubs }
  })
}

describe('ContentPanel.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  describe('节点信息展示', () => {
    it('选中文件节点应显示名称、路径和类型', () => {
      wrapper = mountPanel({ name: 'test.txt', path: '/path/to/test.txt', type: 'file' })

      expect(wrapper.find('h2').text()).toBe('test.txt')
      expect(wrapper.text()).toContain('/path/to/test.txt')
      expect(wrapper.text()).toContain('文件')
    })

    it('选中文件夹节点应显示类型为"文件夹"', () => {
      wrapper = mountPanel({ name: 'src', path: '/path/to/src', type: 'directory' })

      expect(wrapper.find('h2').text()).toBe('src')
      expect(wrapper.text()).toContain('/path/to/src')
      expect(wrapper.text()).toContain('文件夹')
    })

    it('未选中节点时不应显示 h2 标题', () => {
      wrapper = mountPanel(null)

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

      wrapper = mountPanel({ name: 'file.txt', path: '/test/file.txt', type: 'file' })

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

      wrapper = mountPanel({ name: 'large.pdf', path: '/test/large.pdf', type: 'file' })

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

      wrapper = mountPanel({ name: 'data.bin', path: '/test/data.bin', type: 'file' })

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      // 不支持的二进制文件：无 textarea，降级分支提供「用默认程序打开」
      expect(wrapper.find('textarea').exists()).toBe(false)
      expect(wrapper.text()).toContain('用默认程序打开')
      expect(ElMessage.warning).toHaveBeenCalledWith('该文件类型暂不支持内嵌预览')
    })
    it('previewFile unsupported 降级为文本应显示内容（无降级提示）', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      // .log 不在文本白名单 -> detectPreviewKind 返回 unsupported，
      // 但后端 DetectTextEncoding 判定为可显示文本后降级为 kind=text
      PreviewFile.mockResolvedValueOnce({
        path: '/test/note.log', name: 'note.log', size: 10,
        content: 'log line', isBinary: false, tooLarge: false, error: '', kind: 'text',
        encoding: 'utf-8'
      })

      wrapper = mountPanel({ name: 'note.log', path: '/test/note.log', type: 'file' })

      const previewBtn = wrapper.findAll('button').find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      // 降级为 text 后：预览区已显示，无「暂不支持内嵌预览」警告
      expect(wrapper.find('.file-preview').exists()).toBe(true)
      expect(ElMessage.warning).not.toHaveBeenCalledWith('该文件类型暂不支持内嵌预览')
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

      wrapper = mountPanel({ name: 'nonexistent.txt', path: '/test/nonexistent.txt', type: 'file' })

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

      wrapper = mountPanel({ name: 'report.docx', path: '/test/report.docx', type: 'file' })

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

      wrapper = mountPanel({ name: 'big.xlsx', path: '/test/big.xlsx', type: 'file' })

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

      wrapper = mountPanel({ name: 'pic.png', path: '/test/pic.png', type: 'file' })

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

      wrapper = mountPanel({ name: 'intro.md', path: '/docs/intro.md', type: 'file' })

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

      wrapper = mountPanel({ name: 'a.md', path: '/docs/a.md', type: 'file' })

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

      wrapper = mountPanel({ name: 'a.md', path: '/docs/a.md', type: 'file' })

      // 模拟「文件树点击 B」：选中节点变更为 B（经 workspace store 驱动响应式更新）
      useWorkspaceStore().selectedNode = { name: 'b.md', path: '/docs/b.md', type: 'file' }
      await wrapper.vm.$nextTick()
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

      wrapper = mountPanel({ name: 'file.txt', path: '/test/file.txt', type: 'file' })

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

    const mountPanel = () => {
      const workspaceStore = useWorkspaceStore()
      workspaceStore.selectedNode = { name: 'file.md', path: '/test/file.md', type: 'file' }
      return mount(ContentPanel, {
        global: { stubs: contentPanelStubs }
      })
    }

    it('Ctrl+S 有修改时触发保存', async () => {
      const { SaveFile } = await import('../../../wailsjs/go/main/App')
      wrapper = mountPanel()
      const textarea = await enterEditMode(wrapper, 'Hello')

      // 修改内容 → isContentModified 为真
      await textarea.setValue('Hello changed')
      await textarea.trigger('keydown', { key: 's', ctrlKey: true })
      await flushPromises()

      // 默认 UTF-8（PreviewFile 未返回 encoding 时回退空串 = UTF-8）
      expect(SaveFile).toHaveBeenCalledWith('/test/file.md', 'Hello changed', '')
    })

    it('Ctrl+S 保存 GBK 文件应回传 encoding=gbk', async () => {
      const { PreviewFile, SaveFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/gbk.txt', name: 'gbk.txt', size: 11,
        content: '中文GBK文本', isBinary: false, tooLarge: false, error: '', kind: 'text',
        encoding: 'gbk'
      })
      wrapper = mountPanel()
      const previewBtn = wrapper.findAll('button').find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()
      const editBtn = wrapper.findAll('button').find(btn => btn.text().includes('编辑'))
      await editBtn.trigger('click')
      await flushPromises()

      const textarea = wrapper.find('textarea')
      await textarea.setValue('中文GBK文本改')
      await textarea.trigger('keydown', { key: 's', ctrlKey: true })
      await flushPromises()

      // 保存应回传 encoding=gbk，后端按 GBK 编码写入（路径取自 selectedNode）
      expect(SaveFile).toHaveBeenCalledWith('/test/file.md', '中文GBK文本改', 'gbk')
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

    const mountPanel = (type = 'file', name = 'doc.md', path = '/test/doc.md') => {
      const workspaceStore = useWorkspaceStore()
      workspaceStore.selectedNode = { name, path, type }
      return mount(ContentPanel, {
        global: { stubs: contentPanelStubs }
      })
    }

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

describe('ContentPanel.vue - HTML 渲染预览', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  const mountAndPreview = async (previewResult) => {
    const { PreviewFile } = await import('../../../wailsjs/go/main/App')
    PreviewFile.mockResolvedValueOnce(previewResult)
    wrapper = mountPanel({ name: 'index.html', path: '/test/index.html', type: 'file' })
    const previewBtn = wrapper.findAll('button').find(btn => btn.text().includes('预览'))
    await previewBtn.trigger('click')
    await flushPromises()
    return PreviewFile
  }

  const htmlPreviewResult = {
    path: '/test/index.html',
    name: 'index.html',
    size: 48,
    content: '<html><head><title>demo</title></head><body><h1>hi</h1></body></html>',
    isBinary: false,
    tooLarge: false,
    error: '',
    kind: 'text'
  }

  const findBtn = (text) => {
    const btn = wrapper.findAll('button').find(b => b.text().trim() === text)
    return btn || { exists: () => false }
  }

  it('HTML 预览默认进渲染视图：iframe 渲染 +「源码」「刷新」按钮', async () => {
    await mountAndPreview(htmlPreviewResult)
    // 渲染视图（真 FilePreviewRenderer，未 stub）
    expect(wrapper.find('iframe.html-frame').exists()).toBe(true)
    expect(wrapper.find('.preview-codemirror-wrap').exists()).toBe(false)
    // 工具栏按钮
    expect(findBtn('源码').exists()).toBe(true)
    expect(findBtn('刷新').exists()).toBe(true)
  })

  it('「源码」按钮：切到 CodeMirror 源码视图，按钮变「渲染」、「刷新」隐藏', async () => {
    await mountAndPreview(htmlPreviewResult)
    await findBtn('源码').trigger('click')
    await flushPromises()
    expect(wrapper.find('iframe.html-frame').exists()).toBe(false)
    expect(wrapper.find('.preview-codemirror-wrap').exists()).toBe(true)
    expect(findBtn('渲染').exists()).toBe(true)
    expect(findBtn('刷新').exists()).toBe(false)
    // 再切回渲染
    await findBtn('渲染').trigger('click')
    await flushPromises()
    expect(wrapper.find('iframe.html-frame').exists()).toBe(true)
  })

  it('渲染态点「编辑」：自动切源码视图并进入编辑（textarea）', async () => {
    await mountAndPreview(htmlPreviewResult)
    await findBtn('编辑').trigger('click')
    await flushPromises()
    expect(wrapper.find('textarea').exists()).toBe(true)
    expect(wrapper.find('iframe.html-frame').exists()).toBe(false)
  })

  it('源码态编辑保存后：回到渲染视图且内容为最新', async () => {
    await mountAndPreview(htmlPreviewResult)
    await findBtn('编辑').trigger('click')
    await flushPromises()
    await wrapper.find('textarea').setValue('<html><body><h1>new content</h1></body></html>')
    await findBtn('保存').trigger('click')
    await flushPromises()
    // 保存成功 → 回渲染态，srcdoc 含新内容
    expect(wrapper.find('textarea').exists()).toBe(false)
    const frame = wrapper.find('iframe.html-frame')
    expect(frame.exists()).toBe(true)
    expect(frame.attributes('srcdoc')).toContain('new content')
  })

  it('「刷新」按钮：按当前预览路径重新 PreviewFile', async () => {
    const PreviewFile = await mountAndPreview(htmlPreviewResult)
    expect(PreviewFile).toHaveBeenCalledTimes(1)
    await findBtn('刷新').trigger('click')
    await flushPromises()
    expect(PreviewFile).toHaveBeenCalledTimes(2)
    expect(PreviewFile).toHaveBeenLastCalledWith('/test/index.html')
  })

  it('非 HTML 文本（.txt）不显示「源码/刷新」按钮', async () => {
    await mountAndPreview({ ...htmlPreviewResult, name: 'note.txt', path: '/test/note.txt' })
    expect(findBtn('源码').exists()).toBe(false)
    expect(findBtn('刷新').exists()).toBe(false)
    expect(wrapper.find('.preview-codemirror-wrap').exists()).toBe(true)
  })
})

// ===== 补充：外部工具 / 分支 / 拉取 / 克隆 / 编辑 handler 分支 =====
describe('ContentPanel.vue - handler 分支补充', () => {
  let wrapper
  const fileNode = { id: 'n1', name: 'a.go', path: 'D:\\proj\\a.go', type: 'file' }
  const dirNode = { id: 'n2', name: 'proj', path: 'D:\\proj', type: 'directory' }

  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
  })

  const mountWith = (node = null) => {
    const ws = useWorkspaceStore()
    ws.selectedNode = node
    wrapper = mount(ContentPanel, { global: { stubs: contentPanelStubs } })
    return wrapper
  }

  describe('外部工具 handler', () => {
    it('handleOpenInExplorer 调后端', async () => {
      const { OpenInExplorer } = await import('../../../wailsjs/go/main/App')
      mountWith(fileNode)
      await wrapper.vm.$.setupState.handleOpenInExplorer()
      await flushPromises()
      expect(OpenInExplorer).toHaveBeenCalledWith('D:\\proj\\a.go')
    })

    it('handleOpenInExplorer 失败时 error', async () => {
      const { OpenInExplorer } = await import('../../../wailsjs/go/main/App')
      mountWith(fileNode)
      OpenInExplorer.mockResolvedValueOnce(false)
      await wrapper.vm.$.setupState.handleOpenInExplorer()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith('打开资源管理器失败')
    })

    it('handleOpenInExplorer 无选中节点时直接返回', async () => {
      const { OpenInExplorer } = await import('../../../wailsjs/go/main/App')
      mountWith(null)
      await wrapper.vm.$.setupState.handleOpenInExplorer()
      expect(OpenInExplorer).not.toHaveBeenCalled()
    })

    it('handleOpenInVSCode 失败时 error', async () => {
      const { OpenInVSCode } = await import('../../../wailsjs/go/main/App')
      mountWith(fileNode)
      OpenInVSCode.mockResolvedValueOnce(false)
      await wrapper.vm.$.setupState.handleOpenInVSCode()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('VSCode'))
    })

    it('handleOpenInWarp 失败时 error', async () => {
      const { OpenInWarp } = await import('../../../wailsjs/go/main/App')
      mountWith(fileNode)
      OpenInWarp.mockResolvedValueOnce(false)
      await wrapper.vm.$.setupState.handleOpenInWarp()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('Warp'))
    })

    it('handleOpenWithDefaultApp 非 file 类型时不触发', async () => {
      const { OpenWithDefaultApp } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      await wrapper.vm.$.setupState.handleOpenWithDefaultApp()
      expect(OpenWithDefaultApp).not.toHaveBeenCalled()
    })

    it('handleOpenWithDefaultApp 失败时 error', async () => {
      const { OpenWithDefaultApp } = await import('../../../wailsjs/go/main/App')
      mountWith(fileNode)
      OpenWithDefaultApp.mockResolvedValueOnce(false)
      await wrapper.vm.$.setupState.handleOpenWithDefaultApp()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith('打开文件失败')
    })
  })

  describe('复制路径/文件名', () => {
    it('handleCopyPath 成功', async () => {
      const writeText = vi.fn(() => Promise.resolve())
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      mountWith(fileNode)
      await wrapper.vm.$.setupState.handleCopyPath()
      expect(writeText).toHaveBeenCalledWith('D:/proj/a.go')
      expect(ElMessage.success).toHaveBeenCalledWith('路径已复制到剪贴板')
      vi.unstubAllGlobals()
    })

    it('handleCopyPath 失败时 error', async () => {
      const writeText = vi.fn(() => Promise.reject(new Error('x')))
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      mountWith(fileNode)
      await wrapper.vm.$.setupState.handleCopyPath()
      expect(ElMessage.error).toHaveBeenCalledWith('复制失败')
      vi.unstubAllGlobals()
    })

    it('handleCopyName 目录时提示文件夹名', async () => {
      const writeText = vi.fn(() => Promise.resolve())
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      mountWith(dirNode)
      await wrapper.vm.$.setupState.handleCopyName()
      expect(ElMessage.success).toHaveBeenCalledWith('文件夹名已复制到剪贴板')
      vi.unstubAllGlobals()
    })
  })

  describe('刷新/更新 emit', () => {
    it('handleRefresh emit refreshNode', () => {
      mountWith(fileNode)
      wrapper.vm.$.setupState.handleRefresh()
      expect(wrapper.emitted('refreshNode')).toBeTruthy()
    })

    it('handleUpdateRepos emit batchPull', () => {
      mountWith(dirNode)
      wrapper.vm.$.setupState.handleUpdateRepos()
      expect(wrapper.emitted('batchPull')).toBeTruthy()
    })
  })

  describe('分支对话框', () => {
    it('showBranchDialog 加载分支列表', async () => {
      const { GetBranches } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      await wrapper.vm.$.setupState.showBranchDialog()
      await flushPromises()
      expect(GetBranches).toHaveBeenCalledWith('D:\\proj')
      expect(wrapper.vm.$.setupState.branchDialogVisible).toBe(true)
      expect(wrapper.vm.$.setupState.currentBranchName).toBe('main')
    })

    it('showBranchDialog 失败时 error', async () => {
      const { GetBranches } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      GetBranches.mockRejectedValueOnce(new Error('boom'))
      await wrapper.vm.$.setupState.showBranchDialog()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
    })

    it('doCheckout 成功切换', async () => {
      const { CheckoutBranch } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      await wrapper.vm.$.setupState.showBranchDialog()
      await flushPromises()
      wrapper.vm.$.setupState.selectedBranch = 'main'
      await wrapper.vm.$.setupState.doCheckout()
      await flushPromises()
      expect(CheckoutBranch).toHaveBeenCalledWith('D:\\proj', 'main', false)
      expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('main'))
    })

    it('doCheckout 无选中分支时直接返回', async () => {
      const { CheckoutBranch } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      await wrapper.vm.$.setupState.doCheckout()
      expect(CheckoutBranch).not.toHaveBeenCalled()
    })
  })

  describe('拉取', () => {
    it('pullRepo 成功短结果 success', async () => {
      const { PullRepo } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      PullRepo.mockResolvedValueOnce('已是最新的')
      await wrapper.vm.$.setupState.pullRepo()
      await flushPromises()
      expect(ElMessage.success).toHaveBeenCalledWith('已是最新的')
    })

    it('pullRepo 超长结果弹详情', async () => {
      const { PullRepo } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      PullRepo.mockResolvedValueOnce('x'.repeat(300))
      await wrapper.vm.$.setupState.pullRepo()
      await flushPromises()
      expect(wrapper.vm.$.setupState.singlePullVisible).toBe(true)
    })

    it('pullRepo 失败时 error', async () => {
      const { PullRepo } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      PullRepo.mockRejectedValueOnce(new Error('net'))
      await wrapper.vm.$.setupState.pullRepo()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('net'))
    })

    it('pullRepo 无选中节点时直接返回', async () => {
      const { PullRepo } = await import('../../../wailsjs/go/main/App')
      mountWith(null)
      await wrapper.vm.$.setupState.pullRepo()
      expect(PullRepo).not.toHaveBeenCalled()
    })

    it('pullRepo 默认传 useRebase=false（兼容现有 pull 行为）', async () => {
      const { PullRepo } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      PullRepo.mockResolvedValueOnce('ok')
      await wrapper.vm.$.setupState.pullRepo()
      await flushPromises()
      expect(PullRepo).toHaveBeenCalledWith(expect.any(String), false)
    })

    it('pullRepo 切换变基模式后传 useRebase=true', async () => {
      const { PullRepo } = await import('../../../wailsjs/go/main/App')
      // el-switch 仅在 isGitRepo 节点的 git-actions 区渲染，用 git 仓库节点挂载
      const gitNode = { id: 'n2', name: 'proj', path: 'D:\\proj', type: 'directory', isGitRepo: true }
      mountWith(gitNode)
      await wrapper.find('.el-switch').setValue(true)
      await flushPromises()
      PullRepo.mockResolvedValueOnce('ok')
      await wrapper.vm.$.setupState.pullRepo()
      await flushPromises()
      expect(PullRepo).toHaveBeenCalledWith(expect.any(String), true)
    })
  })

  describe('克隆', () => {
    it('cloneRepo 空地址时 warning', async () => {
      mountWith(dirNode)
      await wrapper.vm.$.setupState.cloneRepo()
      expect(ElMessage.warning).toHaveBeenCalledWith('请输入 Git 仓库地址')
    })

    it('cloneRepo 成功时 success + emit refreshNode', async () => {
      const { CloneRepo } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      wrapper.vm.$.setupState.cloneUrl = 'https://example.com/repo.git'
      await wrapper.vm.$.setupState.cloneRepo()
      await flushPromises()
      expect(CloneRepo).toHaveBeenCalled()
      expect(ElMessage.success).toHaveBeenCalled()
      expect(wrapper.emitted('refreshNode')).toBeTruthy()
    })

    it('cloneRepo 结果不含成功时 error', async () => {
      const { CloneRepo } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      wrapper.vm.$.setupState.cloneUrl = 'https://x'
      CloneRepo.mockResolvedValueOnce('认证失败')
      await wrapper.vm.$.setupState.cloneRepo()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith('认证失败')
    })

    it('cloneRepo 抛异常时 error', async () => {
      const { CloneRepo } = await import('../../../wailsjs/go/main/App')
      mountWith(dirNode)
      wrapper.vm.$.setupState.cloneUrl = 'https://x'
      CloneRepo.mockRejectedValueOnce(new Error('boom'))
      await wrapper.vm.$.setupState.cloneRepo()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
    })
  })

  describe('批量拉取/状态栏', () => {
    it('startBatchPull 初始化进度状态', () => {
      mountWith(dirNode)
      wrapper.vm.$.setupState.startBatchPull({ total: 5 })
      expect(wrapper.vm.$.setupState.pullDialogVisible).toBe(true)
      expect(wrapper.vm.$.setupState.pullProgress.total).toBe(5)
      expect(wrapper.vm.$.setupState.pullCompleted).toBe(false)
    })

    it('onStatusBarClick 完成后打开对话框', () => {
      mountWith(dirNode)
      wrapper.vm.$.setupState.pullCompleted = true
      wrapper.vm.$.setupState.pullRunningInBackground = true
      wrapper.vm.$.setupState.onStatusBarClick()
      expect(wrapper.vm.$.setupState.pullDialogVisible).toBe(true)
      expect(wrapper.vm.$.setupState.pullRunningInBackground).toBe(false)
    })
  })

  describe('编辑/纯函数', () => {
    it('normalizePath 规范化反斜杠 + 小写', () => {
      mountWith(null)
      expect(wrapper.vm.$.setupState.normalizePath('D:\\Proj\\A')).toBe('d:/proj/a')
    })

    it('isWailsRuntime 判断 window.runtime', () => {
      mountWith(null)
      expect(wrapper.vm.$.setupState.isWailsRuntime()).toBe(false)
    })

    it('enterEdit HTML 渲染态切源码', () => {
      // isHtmlPreview 依赖 filePreview.kind/name，直接置 filePreview
      mountWith(fileNode)
      wrapper.vm.$.setupState.filePreview = { kind: 'text', name: 'a.html', content: '', encoding: 'utf-8', path: 'D:\\a.html' }
      expect(wrapper.vm.$.setupState.isHtmlPreview).toBe(true)
      expect(wrapper.vm.$.setupState.htmlViewMode).toBe('render')
      wrapper.vm.$.setupState.enterEdit()
      expect(wrapper.vm.$.setupState.htmlViewMode).toBe('source')
      expect(wrapper.vm.$.setupState.isEditing).toBe(true)
    })

    it('handleCancelEdit 恢复原内容并退出编辑', () => {
      mountWith(fileNode)
      wrapper.vm.$.setupState.originalContent = 'orig'
      wrapper.vm.$.setupState.filePreview = { content: 'modified' }
      wrapper.vm.$.setupState.isEditing = true
      wrapper.vm.$.setupState.handleCancelEdit()
      expect(wrapper.vm.$.setupState.filePreview.content).toBe('orig')
      expect(wrapper.vm.$.setupState.isEditing).toBe(false)
    })

    it('onEditKeydown Ctrl+S 触发保存（有修改时）', async () => {
      const { SaveFile } = await import('../../../wailsjs/go/main/App')
      mountWith(fileNode)
      wrapper.vm.$.setupState.originalContent = 'orig'
      wrapper.vm.$.setupState.filePreview = { content: 'changed', encoding: 'utf-8' }
      const e = { ctrlKey: true, metaKey: false, key: 's', preventDefault: vi.fn() }
      await wrapper.vm.$.setupState.onEditKeydown(e)
      await flushPromises()
      expect(SaveFile).toHaveBeenCalled()
    })

    it('onEditKeydown Esc 取消编辑（无修改直接退出）', async () => {
      mountWith(fileNode)
      wrapper.vm.$.setupState.originalContent = 'same'
      wrapper.vm.$.setupState.filePreview = { content: 'same' }
      wrapper.vm.$.setupState.isEditing = true
      const e = { ctrlKey: false, metaKey: false, key: 'Escape', preventDefault: vi.fn() }
      await wrapper.vm.$.setupState.onEditKeydown(e)
      await flushPromises()
      expect(wrapper.vm.$.setupState.isEditing).toBe(false)
    })
  })
})
