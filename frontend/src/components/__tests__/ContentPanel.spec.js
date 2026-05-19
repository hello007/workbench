import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ContentPanel from '../ContentPanel.vue'

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
    template: '<input :value="modelValue" />',
    props: ['modelValue', 'placeholder', 'disabled', 'type', 'rows', 'readonly']
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
})
