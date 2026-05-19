import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import FileTreePanel from '../FileTreePanel.vue'

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
      confirm: vi.fn()
    }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetFileTree: vi.fn(() => Promise.resolve([])),
  GetGitInfo: vi.fn(() => Promise.resolve({})),
  CreateDirectory: vi.fn(() => Promise.resolve(true)),
  CreateFile: vi.fn(() => Promise.resolve(true)),
  RenameFile: vi.fn(() => Promise.resolve(true)),
  DeleteFile: vi.fn(() => Promise.resolve(true)),
  OpenInExplorer: vi.fn(() => Promise.resolve(true)),
  OpenInVSCode: vi.fn(() => Promise.resolve(true)),
  OpenInWarp: vi.fn(() => Promise.resolve(true)),
  OpenWithDefaultApp: vi.fn(() => Promise.resolve(true)),
  ScanAndPullRepos: vi.fn(() => Promise.resolve({ total: 0 }))
}))

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn()
}))

vi.mock('../../../utils/debug', () => ({
  debug: { log: vi.fn(), error: vi.fn(), warn: vi.fn() }
}))

const defaultStubs = {
  'el-button-group': { template: '<div><slot /></div>' },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['loading', 'size'],
    emits: ['click']
  },
  'el-tree': {
    template: '<div class="el-tree"></div>',
    props: ['props', 'lazy', 'load', 'nodeKey', 'data']
  },
  'el-empty': { template: '<div class="el-empty" />', props: ['description', 'imageSize'] },
  'el-icon': { template: '<i><slot /></i>' },
  'el-dialog': {
    template: '<div v-if="modelValue"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue']
  },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'disabled']
  },
  Folder: { template: '<span>folder</span>' },
  FolderOpened: { template: '<span>folder-opened</span>' },
  Document: { template: '<span>doc</span>' },
  SuccessFilled: { template: '<span>git</span>' },
  FolderAdd: { template: '<span>fa</span>' },
  DocumentAdd: { template: '<span>da</span>' },
  Edit: { template: '<span>edit</span>' },
  Delete: { template: '<span>del</span>' },
  CopyDocument: { template: '<span>cp</span>' },
  Monitor: { template: '<span>mon</span>' },
  Refresh: { template: '<span>ref</span>' },
  EditPen: { template: '<span>ep</span>' },
  Open: { template: '<span>open</span>' },
  Promotion: { template: '<span>prom</span>' },
  Scissor: { template: '<span>sci</span>' },
  DocumentCopy: { template: '<span>dc</span>' }
}

const mockDirectories = [
  { id: 'dir-1', name: '项目A', path: '/path/a', isDefault: true },
  { id: 'dir-2', name: '项目B', path: '/path/b', isDefault: false }
]

function createWrapper(props = {}) {
  return mount(FileTreePanel, {
    props: {
      directories: mockDirectories,
      selectedDirId: 'dir-1',
      clipboard: { mode: null },
      ...props
    },
    global: { stubs: defaultStubs }
  })
}

describe('FileTreePanel.vue', () => {
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

  describe('loadTreeNode 懒加载', () => {
    it('根节点(level=0)应从 directories 查找路径并调用 GetFileTree', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce([
        { name: 'src', path: '/path/a/src', type: 'directory', isGitRepo: false, hasChildren: true, isLeaf: false },
        { name: 'readme.md', path: '/path/a/readme.md', type: 'file', isGitRepo: false, hasChildren: false, isLeaf: true }
      ])

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
      await flushPromises()

      expect(GetFileTree).toHaveBeenCalledWith('/path/a')
      expect(resolve).toHaveBeenCalled()
      const resolvedNodes = resolve.mock.calls[0][0]
      expect(resolvedNodes.length).toBe(2)
    })

    it('子节点(level>0)应使用 node.data.path 调用 GetFileTree', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce([
        { name: 'main.go', path: '/path/a/src/main.go', type: 'file', isGitRepo: false, hasChildren: false, isLeaf: true }
      ])

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 1, data: { path: '/path/a/src' } }, resolve)
      await flushPromises()

      expect(GetFileTree).toHaveBeenCalledWith('/path/a/src')
      expect(resolve).toHaveBeenCalled()
    })

    it('无选中目录时 resolve 空数组', async () => {
      wrapper = createWrapper({ selectedDirId: '' })
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)

      expect(resolve).toHaveBeenCalledWith([])
    })
  })

  describe('isLeaf 判断逻辑', () => {
    it('文件节点 isLeaf=true', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce([
        { name: 'file.txt', path: '/test/file.txt', type: 'file', hasChildren: false, isLeaf: true }
      ])

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
      await flushPromises()

      const resolvedNodes = resolve.mock.calls[0][0]
      const fileNode = resolvedNodes.find(n => n.name === 'file.txt')
      expect(fileNode.isLeaf).toBe(true)
    })

    it('目录节点 hasChildren=true 时 isLeaf=false', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce([
        { name: 'src', path: '/test/src', type: 'directory', hasChildren: true, isLeaf: false }
      ])

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
      await flushPromises()

      const resolvedNodes = resolve.mock.calls[0][0]
      const dirNode = resolvedNodes.find(n => n.name === 'src')
      expect(dirNode.isLeaf).toBe(false)
    })

    it('目录节点 hasChildren=false 时前端二次判断 isLeaf=true', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce([
        { name: 'empty-dir', path: '/test/empty-dir', type: 'directory', hasChildren: false, isLeaf: false }
      ])

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
      await flushPromises()

      const resolvedNodes = resolve.mock.calls[0][0]
      const dirNode = resolvedNodes.find(n => n.name === 'empty-dir')
      // 前端逻辑: n.type === 'file' || !n.hasChildren → true
      expect(dirNode.isLeaf).toBe(true)
    })
  })

  describe('加载失败处理', () => {
    it('GetFileTree 失败时应 resolve 空数组', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockRejectedValueOnce(new Error('读取失败'))

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
      await flushPromises()

      expect(resolve).toHaveBeenCalledWith([])
      expect(ElMessage.error).toHaveBeenCalled()
    })
  })

  describe('事件监听清理', () => {
    it('unmount 时应移除 click 和 contextmenu 监听器', () => {
      const removeSpy = vi.spyOn(document, 'removeEventListener')
      wrapper = createWrapper()
      wrapper.unmount()
      expect(removeSpy).toHaveBeenCalledWith('click', expect.any(Function))
      expect(removeSpy).toHaveBeenCalledWith('contextmenu', expect.any(Function))
      removeSpy.mockRestore()
      wrapper = null
    })
  })

  describe('isGitRepo 字段透传', () => {
    it('isGitRepo=true 的目录节点应保留该字段', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce([
        { name: 'my-repo', path: '/path/a/my-repo', type: 'directory', isGitRepo: true, hasChildren: true, isLeaf: false },
        { name: 'plain-dir', path: '/path/a/plain-dir', type: 'directory', isGitRepo: false, hasChildren: true, isLeaf: false }
      ])

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
      await flushPromises()

      const resolvedNodes = resolve.mock.calls[0][0]
      const repoNode = resolvedNodes.find(n => n.name === 'my-repo')
      const plainNode = resolvedNodes.find(n => n.name === 'plain-dir')
      expect(repoNode.isGitRepo).toBe(true)
      expect(plainNode.isGitRepo).toBe(false)
    })

    it('isGitRepo=true 的文件节点不应出现（文件不检测 Git）', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce([
        { name: 'file.txt', path: '/test/file.txt', type: 'file', isGitRepo: false, hasChildren: false, isLeaf: true }
      ])

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
      await flushPromises()

      const resolvedNodes = resolve.mock.calls[0][0]
      const fileNode = resolvedNodes.find(n => n.name === 'file.txt')
      expect(fileNode.isGitRepo).toBe(false)
    })
  })

  describe('隐藏项数据透传', () => {
    it('隐藏目录和隐藏文件应从后端正确透传到前端', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce([
        { name: '.claude', path: '/path/a/.claude', type: 'directory', isGitRepo: false, hasChildren: true, isLeaf: false },
        { name: '.env', path: '/path/a/.env', type: 'file', isGitRepo: false, hasChildren: false, isLeaf: true },
        { name: '.gitignore', path: '/path/a/.gitignore', type: 'file', isGitRepo: false, hasChildren: false, isLeaf: true },
        { name: 'src', path: '/path/a/src', type: 'directory', isGitRepo: false, hasChildren: true, isLeaf: false }
      ])

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
      await flushPromises()

      const resolvedNodes = resolve.mock.calls[0][0]
      expect(resolvedNodes.length).toBe(4)
      const claudeNode = resolvedNodes.find(n => n.name === '.claude')
      expect(claudeNode.type).toBe('directory')
      expect(claudeNode.isLeaf).toBe(false)
      const envNode = resolvedNodes.find(n => n.name === '.env')
      expect(envNode.type).toBe('file')
      expect(envNode.isLeaf).toBe(true)
      const gitignoreNode = resolvedNodes.find(n => n.name === '.gitignore')
      expect(gitignoreNode.type).toBe('file')
      expect(gitignoreNode.isLeaf).toBe(true)
    })
  })

  // ---- Story 2-4: 全部展开/收起与节点选中 ----

  function createWrapperWithStore(storeMock = {}) {
    const mergedStore = {
      root: { childNodes: [] },
      nodesMap: {},
      ...storeMock
    }
    const stubs = {
      ...defaultStubs,
      'el-tree': {
        template: '<div class="el-tree"></div>',
        props: ['props', 'lazy', 'load', 'nodeKey', 'data'],
        data() {
          return { store: mergedStore }
        }
      }
    }
    return mount(FileTreePanel, {
      props: {
        directories: mockDirectories,
        selectedDirId: 'dir-1',
        clipboard: { mode: null }
      },
      global: { stubs }
    })
  }

  describe('全部展开 expandAll', () => {
    it('应递归展开非叶节点并显示成功提示', async () => {
      const mockExpand = vi.fn((callback) => callback())
      const childDir = {
        isLeaf: false,
        expanded: false,
        childNodes: [{ isLeaf: true, childNodes: [] }],
        expand: mockExpand
      }
      const childFile = { isLeaf: true, childNodes: [] }

      wrapper = createWrapperWithStore({
        root: { childNodes: [childDir, childFile] }
      })

      await wrapper.vm.expandAll()
      await flushPromises()

      expect(mockExpand).toHaveBeenCalledTimes(1)
      expect(ElMessage.success).toHaveBeenCalledWith('已全部展开')
    })

    it('失败时应显示错误提示', async () => {
      wrapper = createWrapper()
      await wrapper.vm.expandAll()
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalled()
    })
  })

  describe('全部收起 collapseAll', () => {
    it('应收起所有展开节点并显示成功提示', () => {
      const expandedNode = { expanded: true, childNodes: [] }
      const collapsedNode = { expanded: false, childNodes: [] }

      wrapper = createWrapperWithStore({
        nodesMap: {
          '/path/a/src': expandedNode,
          '/path/a/readme.md': collapsedNode
        }
      })

      wrapper.vm.collapseAll()

      expect(expandedNode.expanded).toBe(false)
      expect(ElMessage.success).toHaveBeenCalledWith('已全部收起')
    })
  })

  describe('节点选中 onNodeClick', () => {
    it('点击节点应 emit select 事件携带节点数据', async () => {
      wrapper = createWrapper()
      const testData = { name: 'test.txt', path: '/test/test.txt', type: 'file' }

      const tree = wrapper.findComponent('.el-tree')
      const handler = tree.vm.$attrs.onNodeClick
      handler(testData)
      await flushPromises()

      expect(wrapper.emitted('select')).toBeTruthy()
      expect(wrapper.emitted('select')[0][0]).toEqual(testData)
    })
  })

  // ---- Story 3-1: 创建文件和文件夹 ----

  describe('showCreateAt 创建对话框', () => {
    it('调用 showCreateAt(file) 应打开对话框并显示父路径', async () => {
      wrapper = createWrapperWithStore()
      const parentData = { name: 'src', path: '/path/a/src', type: 'directory' }

      wrapper.vm.showCreateAt(parentData, 'file')
      await flushPromises()

      // 对话框打开后应能找到父路径输入框和确定按钮
      const inputs = wrapper.findAll('input')
      const parentInput = inputs.find(i => i.element.value === '/path/a/src')
      expect(parentInput).toBeTruthy()
      const buttons = wrapper.findAll('button')
      const confirmBtn = buttons.find(b => b.text() === '确定')
      expect(confirmBtn).toBeTruthy()
    })

    it('调用 showCreateAt(directory) 也应打开对话框', async () => {
      wrapper = createWrapperWithStore()
      const parentData = { name: 'src', path: '/path/a/src', type: 'directory' }

      wrapper.vm.showCreateAt(parentData, 'directory')
      await flushPromises()

      const inputs = wrapper.findAll('input')
      const parentInput = inputs.find(i => i.element.value === '/path/a/src')
      expect(parentInput).toBeTruthy()
    })
  })

  describe('handleCreate 创建文件', () => {
    it('创建文件夹成功应调用 CreateDirectory 并显示成功提示', async () => {
      const { CreateDirectory } = await import('../../../wailsjs/go/main/App')
      const mockExpand = vi.fn((callback) => callback())
      const childDir = {
        data: { path: '/path/a' },
        isLeaf: false,
        expanded: true,
        childNodes: [],
        expand: mockExpand,
        loaded: true,
        loading: false
      }

      wrapper = createWrapperWithStore({
        nodesMap: { '/path/a': childDir },
        root: { childNodes: [childDir] }
      })

      wrapper.vm.showCreateAt({ name: 'a', path: '/path/a', type: 'directory' }, 'directory')
      await flushPromises()

      // 找到名称输入框（value 为空的 input）并输入名称
      const inputs = wrapper.findAll('input')
      const nameInput = inputs.find(i => i.element.value === '')
      await nameInput.setValue('new-folder')

      // 点击确定按钮
      const buttons = wrapper.findAll('button')
      const confirmBtn = buttons.find(b => b.text() === '确定')
      await confirmBtn.trigger('click')
      await flushPromises()

      expect(CreateDirectory).toHaveBeenCalledWith('/path/a', 'new-folder')
      expect(ElMessage.success).toHaveBeenCalledWith('文件夹创建成功')
    })

    it('创建文件成功应调用 CreateFile 并显示成功提示', async () => {
      const { CreateFile } = await import('../../../wailsjs/go/main/App')
      const mockExpand = vi.fn((callback) => callback())
      const childDir = {
        data: { path: '/path/a' },
        isLeaf: false,
        expanded: true,
        childNodes: [],
        expand: mockExpand,
        loaded: true,
        loading: false
      }

      wrapper = createWrapperWithStore({
        nodesMap: { '/path/a': childDir },
        root: { childNodes: [childDir] }
      })

      wrapper.vm.showCreateAt({ name: 'a', path: '/path/a', type: 'directory' }, 'file')
      await flushPromises()

      const inputs = wrapper.findAll('input')
      const nameInput = inputs.find(i => i.element.value === '')
      await nameInput.setValue('new-file.go')

      const buttons = wrapper.findAll('button')
      const confirmBtn = buttons.find(b => b.text() === '确定')
      await confirmBtn.trigger('click')
      await flushPromises()

      expect(CreateFile).toHaveBeenCalledWith('/path/a', 'new-file.go', '')
      expect(ElMessage.success).toHaveBeenCalledWith('文件创建成功')
    })

    it('空名称应显示警告提示', async () => {
      wrapper = createWrapperWithStore()
      wrapper.vm.showCreateAt({ name: 'a', path: '/path/a', type: 'directory' }, 'file')
      await flushPromises()

      const buttons = wrapper.findAll('button')
      const confirmBtn = buttons.find(b => b.text() === '确定')
      await confirmBtn.trigger('click')
      await flushPromises()

      expect(ElMessage.warning).toHaveBeenCalledWith('请输入文件名称')
    })

    it('创建失败应显示错误提示', async () => {
      const { CreateDirectory } = await import('../../../wailsjs/go/main/App')
      CreateDirectory.mockResolvedValueOnce(false)

      const childDir = {
        data: { path: '/path/a' },
        isLeaf: false,
        expanded: true,
        childNodes: [],
        expand: vi.fn(),
        loaded: true,
        loading: false
      }

      wrapper = createWrapperWithStore({
        nodesMap: { '/path/a': childDir },
        root: { childNodes: [childDir] }
      })

      wrapper.vm.showCreateAt({ name: 'a', path: '/path/a', type: 'directory' }, 'directory')
      await flushPromises()

      const inputs = wrapper.findAll('input')
      const nameInput = inputs.find(i => i.element.value === '')
      await nameInput.setValue('existing-dir')

      const buttons = wrapper.findAll('button')
      const confirmBtn = buttons.find(b => b.text() === '确定')
      await confirmBtn.trigger('click')
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalledWith('创建失败')
    })
  })
})
