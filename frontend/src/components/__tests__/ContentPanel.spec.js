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
  PullRepo: vi.fn(() => Promise.resolve('')),
  CloneRepo: vi.fn(() => Promise.resolve('克隆成功')),
  OpenWithDefaultApp: vi.fn(() => Promise.resolve(true))
}))

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn()
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
    it('previewFile 成功应显示内容', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      const testContent = 'Hello, world!'
      PreviewFile.mockResolvedValueOnce({
        path: '/test/file.txt',
        name: 'file.txt',
        size: 13,
        content: testContent,
        isBinary: false,
        tooLarge: false,
        error: ''
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'file.txt', path: '/test/file.txt', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      // 先检查初始状态
      expect(wrapper.find('textarea').exists()).toBe(false)

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      expect(previewBtn.exists()).toBe(true)
      await previewBtn.trigger('click')
      await flushPromises()

      expect(PreviewFile).toHaveBeenCalledWith('/test/file.txt')

      // 检查组件实例的 filePreview 值
      console.log('filePreview:', wrapper.vm.filePreview)

      console.log('Wrapper HTML after preview:', wrapper.html())

      // 尝试通过其他方式查找文本框
      const textarea = wrapper.find('textarea')
      console.log('Textarea found:', textarea.exists())

      if (textarea.exists()) {
        console.log('Textarea value:', textarea.element.value)
      }

      expect(wrapper.find('textarea').exists()).toBe(true)
      expect(wrapper.find('textarea').element.value).toBe(testContent)
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

    it('previewFile 二进制文件应显示警告', async () => {
      const { PreviewFile } = await import('../../../wailsjs/go/main/App')
      PreviewFile.mockResolvedValueOnce({
        path: '/test/image.png',
        name: 'image.png',
        size: 1000,
        content: '',
        isBinary: true,
        tooLarge: false,
        error: ''
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'image.png', path: '/test/image.png', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()

      expect(ElMessage.warning).toHaveBeenCalledWith('二进制文件，无法预览')
      expect(wrapper.find('textarea').exists()).toBe(false)
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
        error: ''
      })

      wrapper = mount(ContentPanel, {
        props: {
          selectedNode: { name: 'file.txt', path: '/test/file.txt', type: 'file' },
          clipboard: { mode: null }
        },
        global: { stubs: contentPanelStubs }
      })

      // 先调用 previewFile 显示内容
      const buttons = wrapper.findAll('button')
      const previewBtn = buttons.find(btn => btn.text().includes('预览'))
      await previewBtn.trigger('click')
      await flushPromises()
      expect(wrapper.find('textarea').exists()).toBe(true)

      // 调用 clearPreview 清空
      await wrapper.vm.clearPreview()
      expect(wrapper.find('textarea').exists()).toBe(false)
    })
  })
})
