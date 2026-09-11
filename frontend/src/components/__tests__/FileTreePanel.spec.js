import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import FileTreePanel from '../FileTreePanel.vue'
import { useDirectoryStore } from '../../store'

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
  RefreshFileTree: vi.fn(() => Promise.resolve([])),
  ClearAllFileTreeCache: vi.fn(() => Promise.resolve()),
  GetGitInfo: vi.fn(() => Promise.resolve({})),
  CreateDirectory: vi.fn(() => Promise.resolve(true)),
  CreateFile: vi.fn(() => Promise.resolve(true)),
  RenameFile: vi.fn(() => Promise.resolve(true)),
  DeleteFile: vi.fn(() => Promise.resolve(true)),
  OpenInExplorer: vi.fn(() => Promise.resolve(true)),
  OpenInVSCode: vi.fn(() => Promise.resolve(true)),
  OpenInWarp: vi.fn(() => Promise.resolve(true)),
  OpenWithDefaultApp: vi.fn(() => Promise.resolve(true)),
  ScanAndPullRepos: vi.fn(() => Promise.resolve({ total: 0 })),
  GetFavorites: vi.fn(() => Promise.resolve([])),
  AddFavorite: vi.fn(() => Promise.resolve(true)),
  RemoveFavorite: vi.fn(() => Promise.resolve(true)),
  UpdateFavoriteAlias: vi.fn(() => Promise.resolve(true)),
  UpdateFavoriteGroup: vi.fn(() => Promise.resolve(true))
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
    props: ['props', 'lazy', 'load', 'nodeKey', 'data'],
    // 提供 store mock：refreshNode 命中 nodesMap/root 兜底时 target=null 提前返回，不抛 nodesMap
    data: () => ({ store: { nodesMap: {}, root: null } })
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

// directories / selectedDirId 已迁 directory store：经 store 设置初始值驱动（不再传 prop）。
// clipboard prop 已删（workspace 域批次6 迁 store，FileTreePanel 零消费）。
function setupDirectoryStore(directories = mockDirectories, selectedDirId = 'dir-1') {
  const pinia = createPinia()
  setActivePinia(pinia)
  const directoryStore = useDirectoryStore()
  directoryStore.directories = directories
  directoryStore.selectedDirectoryId = selectedDirId
  return pinia
}

function createWrapper(props = {}) {
  const { directories = mockDirectories, selectedDirId = 'dir-1' } = props
  const pinia = setupDirectoryStore(directories, selectedDirId)
  return mount(FileTreePanel, {
    global: { plugins: [pinia], stubs: defaultStubs }
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
    it('unmount 时应移除 mousedown 和 contextmenu 监听器', () => {
      const removeSpy = vi.spyOn(document, 'removeEventListener')
      wrapper = createWrapper()
      wrapper.unmount()
      expect(removeSpy).toHaveBeenCalledWith('mousedown', expect.any(Function))
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
    const pinia = setupDirectoryStore(mockDirectories, 'dir-1')
    return mount(FileTreePanel, {
      global: { plugins: [pinia], stubs }
    })
  }

  describe('全部展开 expandAll', () => {
    it('应递归展开非叶节点并显示成功提示', async () => {
      const mockExpand = vi.fn(function (callback) { this.loaded = true; if (typeof callback === 'function') callback() })
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
      const mockExpand = vi.fn(function (callback) { this.loaded = true; if (typeof callback === 'function') callback() })
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
      const mockExpand = vi.fn(function (callback) { this.loaded = true; if (typeof callback === 'function') callback() })
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

  // ---- Story 3-2: 重命名和删除 ----

  describe('showRenameAt 重命名对话框', () => {
    it('调用 showRenameAt 应打开对话框并预填当前名称', async () => {
      wrapper = createWrapperWithStore()
      const nodeData = { name: 'old-name.txt', path: '/path/a/old-name.txt', type: 'file' }

      wrapper.vm.showRenameAt(nodeData)
      await flushPromises()

      // 对话框打开后应能找到包含当前名称的输入框和确定按钮
      const inputs = wrapper.findAll('input')
      const nameInput = inputs.find(i => i.element.value === 'old-name.txt')
      expect(nameInput).toBeTruthy()
      const buttons = wrapper.findAll('button')
      const confirmBtn = buttons.find(b => b.text() === '确定')
      expect(confirmBtn).toBeTruthy()
    })
  })

  describe('handleRename 重命名', () => {
    it('重命名成功应调用 RenameFile 并显示成功提示', async () => {
      const { RenameFile } = await import('../../../wailsjs/go/main/App')
      const mockExpand = vi.fn(function (callback) { this.loaded = true; if (typeof callback === 'function') callback() })
      const childNode = {
        data: { path: '/path/a' },
        isLeaf: false,
        expanded: true,
        childNodes: [],
        expand: mockExpand,
        loaded: true,
        loading: false
      }

      wrapper = createWrapperWithStore({
        nodesMap: { '/path/a': childNode },
        root: { childNodes: [childNode] }
      })

      wrapper.vm.showRenameAt({ name: 'old.txt', path: '/path/a/old.txt', type: 'file' })
      await flushPromises()

      // 找到新名称输入框（对话框中有两个 value='old.txt' 的 input，第二个是 v-model 绑定的）
      const inputs = wrapper.findAll('input')
      const matchingInputs = inputs.filter(i => i.element.value === 'old.txt')
      const nameInput = matchingInputs[matchingInputs.length - 1]
      await nameInput.setValue('new.txt')

      const buttons = wrapper.findAll('button')
      const confirmBtn = buttons.find(b => b.text() === '确定')
      await confirmBtn.trigger('click')
      await flushPromises()

      expect(RenameFile).toHaveBeenCalledWith('/path/a/old.txt', 'new.txt')
      expect(ElMessage.success).toHaveBeenCalledWith('重命名成功')
      expect(mockExpand).toHaveBeenCalled()
    })

    it('空名称应显示警告提示', async () => {
      wrapper = createWrapperWithStore()
      wrapper.vm.showRenameAt({ name: 'file.txt', path: '/path/a/file.txt', type: 'file' })
      await flushPromises()

      // 清空新名称输入框（取最后一个匹配的 input，即 v-model 绑定的）
      const inputs = wrapper.findAll('input')
      const matchingInputs = inputs.filter(i => i.element.value === 'file.txt')
      const nameInput = matchingInputs[matchingInputs.length - 1]
      await nameInput.setValue('')

      const buttons = wrapper.findAll('button')
      const confirmBtn = buttons.find(b => b.text() === '确定')
      await confirmBtn.trigger('click')
      await flushPromises()

      expect(ElMessage.warning).toHaveBeenCalledWith('请输入名称')
    })

    it('重命名失败应显示错误提示', async () => {
      const { RenameFile } = await import('../../../wailsjs/go/main/App')
      RenameFile.mockResolvedValueOnce(false)

      wrapper = createWrapperWithStore()
      wrapper.vm.showRenameAt({ name: 'old.txt', path: '/path/a/old.txt', type: 'file' })
      await flushPromises()

      const inputs = wrapper.findAll('input')
      const matchingInputs = inputs.filter(i => i.element.value === 'old.txt')
      const nameInput = matchingInputs[matchingInputs.length - 1]
      await nameInput.setValue('new.txt')

      const buttons = wrapper.findAll('button')
      const confirmBtn = buttons.find(b => b.text() === '确定')
      await confirmBtn.trigger('click')
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalledWith('重命名失败')
    })
  })

  describe('handleDeleteAt 删除', () => {
    it('确认删除应调用 DeleteFile 并显示成功提示', async () => {
      const { ElMessageBox } = await import('element-plus')
      const { DeleteFile } = await import('../../../wailsjs/go/main/App')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')

      const mockExpand = vi.fn(function (callback) { this.loaded = true; if (typeof callback === 'function') callback() })
      const childNode = {
        data: { path: '/path/a' },
        isLeaf: false,
        expanded: true,
        childNodes: [],
        expand: mockExpand,
        loaded: true,
        loading: false
      }

      wrapper = createWrapperWithStore({
        nodesMap: { '/path/a': childNode },
        root: { childNodes: [childNode] }
      })

      await wrapper.vm.handleDeleteAt({ name: 'to-delete.txt', path: '/path/a/to-delete.txt', type: 'file' })
      await flushPromises()

      expect(ElMessageBox.confirm).toHaveBeenCalledWith(
        expect.stringContaining('to-delete.txt'),
        '警告',
        expect.any(Object)
      )
      expect(DeleteFile).toHaveBeenCalledWith('/path/a/to-delete.txt')
      expect(ElMessage.success).toHaveBeenCalledWith('删除成功')
      expect(mockExpand).toHaveBeenCalled()
    })

    it('用户取消确认不应调用 DeleteFile', async () => {
      const { ElMessageBox } = await import('element-plus')
      const { DeleteFile } = await import('../../../wailsjs/go/main/App')
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')

      wrapper = createWrapperWithStore()

      await wrapper.vm.handleDeleteAt({ name: 'file.txt', path: '/path/a/file.txt', type: 'file' })
      await flushPromises()

      expect(ElMessageBox.confirm).toHaveBeenCalled()
      expect(DeleteFile).not.toHaveBeenCalled()
    })

    it('删除失败应显示错误提示', async () => {
      const { ElMessageBox } = await import('element-plus')
      const { DeleteFile } = await import('../../../wailsjs/go/main/App')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')
      DeleteFile.mockResolvedValueOnce(false)

      wrapper = createWrapperWithStore()

      await wrapper.vm.handleDeleteAt({ name: 'file.txt', path: '/path/a/file.txt', type: 'file' })
      await flushPromises()

      expect(DeleteFile).toHaveBeenCalledWith('/path/a/file.txt')
      expect(ElMessage.error).toHaveBeenCalledWith('删除失败')
    })
  })

  describe('refreshNode 祖先回溯', () => {
    it('命中分支：nodesMap 中存在目标路径时，应直接刷新该节点', async () => {
      const targetExpand = vi.fn(function () { this.loaded = true })
      const ancestorExpand = vi.fn(function () { this.loaded = true })
      const targetNode = {
        data: { path: '/path/a/src/foo' },
        loaded: true,
        loading: false,
        expanded: true,
        isLeaf: false,
        childNodes: [],
        expand: targetExpand
      }
      const ancestorNode = {
        data: { path: '/path/a/src' },
        loaded: true,
        loading: false,
        expanded: true,
        isLeaf: false,
        childNodes: [targetNode],
        expand: ancestorExpand
      }

      wrapper = createWrapperWithStore({
        nodesMap: {
          '/path/a/src': ancestorNode,
          '/path/a/src/foo': targetNode
        }
      })
      await flushPromises()

      await wrapper.vm.refreshNode('/path/a/src/foo')

      expect(targetExpand).toHaveBeenCalledTimes(1)
      expect(ancestorExpand).not.toHaveBeenCalled()
    })

    it('refreshNode 应先调 RefreshFileTree 清后端缓存再触发 expand', async () => {
      const { RefreshFileTree } = await import('../../../wailsjs/go/main/App')
      const targetExpand = vi.fn(function () { this.loaded = true })
      const targetNode = {
        data: { path: '/path/a/src/foo' },
        loaded: true,
        loading: false,
        expanded: true,
        isLeaf: false,
        childNodes: [],
        expand: targetExpand
      }

      wrapper = createWrapperWithStore({
        nodesMap: { '/path/a/src/foo': targetNode }
      })
      await flushPromises()

      await wrapper.vm.refreshNode('/path/a/src/foo')

      expect(RefreshFileTree).toHaveBeenCalledWith('/path/a/src/foo')
      expect(targetExpand).toHaveBeenCalledTimes(1)
    })

    it('回溯命中分支：目标缺失但存在已展开祖先时，应刷新最近的已展开祖先', async () => {
      const grandExpand = vi.fn(function () { this.loaded = true })
      const parentExpand = vi.fn(function () { this.loaded = true })
      const grandNode = {
        data: { path: '/path/a/src' },
        loaded: true,
        loading: false,
        expanded: true,
        isLeaf: false,
        childNodes: [],
        expand: grandExpand
      }
      const parentNode = {
        data: { path: '/path/a/src/foo' },
        loaded: true,
        loading: false,
        expanded: true,
        isLeaf: false,
        childNodes: [],
        expand: parentExpand
      }

      wrapper = createWrapperWithStore({
        nodesMap: {
          '/path/a/src': grandNode,
          '/path/a/src/foo': parentNode
        }
      })
      await flushPromises()

      await wrapper.vm.refreshNode('/path/a/src/foo/bar/baz.txt')

      expect(parentExpand).toHaveBeenCalledTimes(1)
      expect(grandExpand).not.toHaveBeenCalled()
    })

    it('静默放弃分支：目标和所有祖先均不可命中时，不应触发任何 expand', async () => {
      const parentExpand = vi.fn()
      const parentNode = {
        data: { path: '/path/a/src/foo' },
        loaded: true,
        loading: false,
        expanded: false,
        isLeaf: false,
        childNodes: [],
        expand: parentExpand
      }

      wrapper = createWrapperWithStore({
        nodesMap: {
          '/path/a/src/foo': parentNode
        }
      })
      await flushPromises()

      await wrapper.vm.refreshNode('/path/a/other/dir/file.txt')

      expect(parentExpand).not.toHaveBeenCalled()
      expect(parentNode.loaded).toBe(true)
    })

    it('根节点分支：nodePath 为工作目录根时，应刷新 store.root', async () => {
      const rootExpand = vi.fn(function () { this.loaded = true })
      const root = {
        loaded: true,
        loading: false,
        expanded: true,
        isLeaf: false,
        childNodes: [],
        expand: rootExpand
      }

      wrapper = createWrapperWithStore({
        root,
        nodesMap: {}
      })
      await flushPromises()

      await wrapper.vm.refreshNode('/path/a')

      expect(rootExpand).toHaveBeenCalledTimes(1)
    })

    it('路径分隔符规范化：nodePath 用 / 而 nodesMap key 用 \\ 时仍能命中', async () => {
      const targetExpand = vi.fn(function () { this.loaded = true })
      const targetNode = {
        data: { path: 'D:\\proj\\src' },
        loaded: true,
        loading: false,
        expanded: true,
        isLeaf: false,
        childNodes: [],
        expand: targetExpand
      }
      const winDirs = [{ id: 'dir-win', name: 'win', path: 'D:\\proj', isDefault: true }]
      const winStore = {
        root: { childNodes: [] },
        nodesMap: { 'D:\\proj\\src': targetNode }
      }
      const stubs = {
        ...defaultStubs,
        'el-tree': {
          template: '<div class="el-tree"></div>',
          props: ['props', 'lazy', 'load', 'nodeKey', 'data'],
          data() { return { store: winStore } }
        }
      }
      const pinia = setupDirectoryStore(winDirs, 'dir-win')
      wrapper = mount(FileTreePanel, {
        global: { plugins: [pinia], stubs }
      })
      await flushPromises()

      // nodePath 用正斜杠，应规范化为反斜杠命中 nodesMap
      await wrapper.vm.refreshNode('D:/proj/src')

      expect(targetExpand).toHaveBeenCalledTimes(1)
    })

    it('子树展开保留：刷新已展开节点后应恢复其子节点的展开状态', async () => {
      const subExpand = vi.fn(function () { this.loaded = true; this.expanded = true })
      const subNode = {
        data: { path: '/path/a/src/sub' },
        loaded: true,
        loading: false,
        expanded: true,
        isLeaf: false,
        childNodes: [],
        expand: subExpand
      }
      // target.expand 模拟 loadData 重建：子节点被重置为未展开的新节点
      const targetExpand = vi.fn(function () {
        this.loaded = true
        subNode.expanded = false
        this.childNodes = [subNode]
      })
      const targetNode = {
        data: { path: '/path/a/src' },
        loaded: true,
        loading: false,
        expanded: true,
        isLeaf: false,
        childNodes: [subNode],
        expand: targetExpand
      }
      const subStore = {
        root: { childNodes: [targetNode] },
        nodesMap: { '/path/a/src': targetNode, '/path/a/src/sub': subNode }
      }
      const stubs = {
        ...defaultStubs,
        'el-tree': {
          template: '<div class="el-tree"></div>',
          props: ['props', 'lazy', 'load', 'nodeKey', 'data'],
          data() { return { store: subStore } },
          methods: {
            getNode(path) { return subStore.nodesMap[path] }
          }
        }
      }
      const pinia = setupDirectoryStore(mockDirectories, 'dir-1')
      wrapper = mount(FileTreePanel, {
        global: { plugins: [pinia], stubs }
      })
      await flushPromises()

      await wrapper.vm.refreshNode('/path/a/src')

      expect(targetExpand).toHaveBeenCalledTimes(1)
      // 重建后 subNode 被重置为未展开，restoreExpandedPaths 应重新展开它
      expect(subExpand).toHaveBeenCalledTimes(1)
    })
  })

  describe('refreshNode 文件节点刷新所在目录', () => {
    it('传入文件路径时，应刷新其父目录而非文件本身', async () => {
      const dirExpand = vi.fn(function () { this.loaded = true })
      const fileExpand = vi.fn(function () { this.loaded = true })
      const dirNode = {
        data: { path: '/path/a/src', type: 'directory' },
        loaded: true, loading: false, expanded: true, isLeaf: false,
        childNodes: [], expand: dirExpand
      }
      const fileNode = {
        data: { path: '/path/a/src/foo.txt', type: 'file' },
        loaded: true, loading: false, expanded: false, isLeaf: true,
        childNodes: [], parent: dirNode, expand: fileExpand
      }
      dirNode.childNodes = [fileNode]
      const store = {
        root: { childNodes: [dirNode] },
        nodesMap: { '/path/a/src': dirNode, '/path/a/src/foo.txt': fileNode }
      }
      const stubs = {
        ...defaultStubs,
        'el-tree': {
          template: '<div class="el-tree"></div>',
          props: ['props', 'lazy', 'load', 'nodeKey', 'data'],
          data() { return { store } },
          methods: { getNode(p) { return store.nodesMap[p] } }
        }
      }
      const pinia = setupDirectoryStore(mockDirectories, 'dir-1')
      wrapper = mount(FileTreePanel, {
        global: { plugins: [pinia], stubs }
      })
      await flushPromises()

      await wrapper.vm.refreshNode('/path/a/src/foo.txt')

      expect(dirExpand).toHaveBeenCalledTimes(1)
      expect(fileExpand).not.toHaveBeenCalled()
    })

    it('文件位于工作目录根下时，应刷新 store.root', async () => {
      const rootExpand = vi.fn(function () { this.loaded = true })
      const root = {
        loaded: true, loading: false, expanded: true, isLeaf: false,
        childNodes: [], expand: rootExpand
      }
      const fileNode = {
        data: { path: '/path/a/root.txt', type: 'file' },
        loaded: true, loading: false, expanded: false, isLeaf: true,
        childNodes: [], parent: root, expand: vi.fn(function () { this.loaded = true })
      }
      root.childNodes = [fileNode]
      const store = { root, nodesMap: { '/path/a/root.txt': fileNode } }
      const stubs = {
        ...defaultStubs,
        'el-tree': {
          template: '<div class="el-tree"></div>',
          props: ['props', 'lazy', 'load', 'nodeKey', 'data'],
          data() { return { store } },
          methods: { getNode(p) { return store.nodesMap[p] } }
        }
      }
      const pinia = setupDirectoryStore(mockDirectories, 'dir-1')
      wrapper = mount(FileTreePanel, {
        global: { plugins: [pinia], stubs }
      })
      await flushPromises()

      await wrapper.vm.refreshNode('/path/a/root.txt')

      expect(rootExpand).toHaveBeenCalledTimes(1)
    })
  })
})

// ===== 补充：菜单分发与外部工具 handler 分支 =====
describe('FileTreePanel.vue - 菜单分发与 handler 补充', () => {
  let wrapper

  const nodeData = { id: 'n1', name: 'a.go', path: 'D:\\proj\\a.go', type: 'file' }

  beforeEach(() => {
    vi.clearAllMocks()
    wrapper = createWrapper()
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
  })

  // 设置右键菜单数据后触发 onMenuCommand
  const setMenuDataAndCommand = (command, data = nodeData) => {
    wrapper.vm.$.setupState.contextMenu.data = data
    wrapper.vm.onMenuCommand(command)
  }

  describe('onMenuCommand 分发', () => {
    it('createFile / createDir 打开创建对话框', () => {
      setMenuDataAndCommand('createFile')
      expect(wrapper.vm.$.setupState.createDialogVisible).toBe(true)
      expect(wrapper.vm.$.setupState.createType).toBe('file')
      setMenuDataAndCommand('createDir')
      expect(wrapper.vm.$.setupState.createType).toBe('directory')
    })

    it('rename 打开重命名对话框', () => {
      setMenuDataAndCommand('rename')
      expect(wrapper.vm.$.setupState.renameDialogVisible).toBe(true)
      expect(wrapper.vm.$.setupState.renameName).toBe('a.go')
    })

    it('copyTo 打开拷贝到对话框（文件模式）', () => {
      setMenuDataAndCommand('copyTo')
      expect(wrapper.vm.$.setupState.copyToDialogVisible).toBe(true)
      expect(wrapper.vm.$.setupState.copyToFileMode).toBe(true)
      expect(wrapper.vm.$.setupState.copyToWholeDir).toBe(false)
    })

    it('copyTo 目录模式设置 wholeDir=true', () => {
      setMenuDataAndCommand('copyTo', { ...nodeData, type: 'directory' })
      expect(wrapper.vm.$.setupState.copyToWholeDir).toBe(true)
      expect(wrapper.vm.$.setupState.copyToFileMode).toBe(false)
    })

    it('cut/copy/paste emit 对应事件', () => {
      setMenuDataAndCommand('cut')
      expect(wrapper.emitted('cut')).toBeTruthy()
      setMenuDataAndCommand('copy')
      expect(wrapper.emitted('copy')).toBeTruthy()
      setMenuDataAndCommand('paste')
      expect(wrapper.emitted('paste')).toBeTruthy()
    })

    it('openRepoFilter emit open-repo-filter', () => {
      setMenuDataAndCommand('openRepoFilter')
      expect(wrapper.emitted('open-repo-filter')).toBeTruthy()
    })

    it('pullRepos emit batchPull', () => {
      setMenuDataAndCommand('pullRepos')
      expect(wrapper.emitted('batchPull')).toBeTruthy()
    })

    it('refresh 命令关闭菜单', () => {
      wrapper.vm.$.setupState.contextMenu.visible = true
      setMenuDataAndCommand('refresh')
      // onMenuCommand 先 closeContextMenu
      expect(wrapper.vm.$.setupState.contextMenu.visible).toBe(false)
    })

    it('contentSearch：路径在当前工作目录内 emit 相对子目录', () => {
      // nodeData.path = D:\proj\a.go，currentDir path = /path/a（mockDirectories）
      // 不同前缀 → emit 空串
      setMenuDataAndCommand('contentSearch')
      const emits = wrapper.emitted('open-content-search')
      expect(emits).toBeTruthy()
    })

    it('copyPath / copyName 调 navigator.clipboard', async () => {
      const writeText = vi.fn(() => Promise.resolve())
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      setMenuDataAndCommand('copyPath')
      await flushPromises()
      expect(writeText).toHaveBeenCalledWith('D:/proj/a.go')
      setMenuDataAndCommand('copyName')
      await flushPromises()
      expect(writeText).toHaveBeenCalledWith('a.go')
      vi.unstubAllGlobals()
    })

    it('openExplorer / openInVSCode / openWithDefaultApp 调后端绑定', async () => {
      const { OpenInExplorer, OpenInVSCode, OpenWithDefaultApp } = await import('../../../wailsjs/go/main/App')
      setMenuDataAndCommand('openExplorer')
      await flushPromises()
      expect(OpenInExplorer).toHaveBeenCalledWith('D:\\proj\\a.go')
      setMenuDataAndCommand('openInVSCode')
      await flushPromises()
      expect(OpenInVSCode).toHaveBeenCalledWith('D:\\proj\\a.go')
      setMenuDataAndCommand('openWithDefaultApp')
      await flushPromises()
      expect(OpenWithDefaultApp).toHaveBeenCalledWith('D:\\proj\\a.go')
    })

    it('addFavorite / removeFavorite 调收藏 store', async () => {
      const { AddFavorite, RemoveFavorite } = await import('../../../wailsjs/go/main/App')
      setMenuDataAndCommand('addFavorite')
      await flushPromises()
      expect(AddFavorite).toHaveBeenCalled()
      setMenuDataAndCommand('removeFavorite')
      await flushPromises()
      expect(RemoveFavorite).toHaveBeenCalled()
    })

    it('无 contextMenu.data 时直接返回', () => {
      wrapper.vm.$.setupState.contextMenu.data = null
      wrapper.vm.onMenuCommand('rename')
      // 不抛错即通过
      expect(wrapper.vm.$.setupState.renameDialogVisible).toBe(false)
    })
  })

  describe('handleCreate', () => {
    it('空名时 warning', () => {
      wrapper.vm.$.setupState.createType = 'directory'
      wrapper.vm.$.setupState.createParentData = { path: 'D:\\dir' }
      wrapper.vm.$.setupState.createName = ''
      wrapper.vm.handleCreate()
      expect(ElMessage.warning).toHaveBeenCalledWith('请输入文件夹名称')
    })

    it('目录创建成功', async () => {
      const { CreateDirectory } = await import('../../../wailsjs/go/main/App')
      wrapper.vm.$.setupState.createType = 'directory'
      wrapper.vm.$.setupState.createParentData = { path: 'D:\\dir' }
      wrapper.vm.$.setupState.createName = 'newdir'
      await wrapper.vm.handleCreate()
      await flushPromises()
      expect(CreateDirectory).toHaveBeenCalledWith('D:\\dir', 'newdir')
      expect(ElMessage.success).toHaveBeenCalledWith('文件夹创建成功')
    })

    it('文件创建成功', async () => {
      const { CreateFile } = await import('../../../wailsjs/go/main/App')
      wrapper.vm.$.setupState.createType = 'file'
      wrapper.vm.$.setupState.createParentData = { path: 'D:\\dir' }
      wrapper.vm.$.setupState.createName = 'new.txt'
      await wrapper.vm.handleCreate()
      await flushPromises()
      expect(CreateFile).toHaveBeenCalledWith('D:\\dir', 'new.txt', '')
      expect(ElMessage.success).toHaveBeenCalledWith('文件创建成功')
    })

    it('创建返回 false 时 error', async () => {
      const { CreateFile } = await import('../../../wailsjs/go/main/App')
      CreateFile.mockResolvedValueOnce(false)
      wrapper.vm.$.setupState.createType = 'file'
      wrapper.vm.$.setupState.createParentData = { path: 'D:\\dir' }
      wrapper.vm.$.setupState.createName = 'x.txt'
      await wrapper.vm.handleCreate()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith('创建失败')
    })

    it('创建抛异常时 error', async () => {
      const { CreateFile } = await import('../../../wailsjs/go/main/App')
      CreateFile.mockRejectedValueOnce(new Error('boom'))
      wrapper.vm.$.setupState.createType = 'file'
      wrapper.vm.$.setupState.createParentData = { path: 'D:\\dir' }
      wrapper.vm.$.setupState.createName = 'x.txt'
      await wrapper.vm.handleCreate()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
    })
  })

  describe('handleRename', () => {
    it('空名时 warning', () => {
      wrapper.vm.$.setupState.renameNode = { path: 'D:\\a.go', name: 'a.go' }
      wrapper.vm.$.setupState.renameName = ''
      wrapper.vm.handleRename()
      expect(ElMessage.warning).toHaveBeenCalledWith('请输入名称')
    })

    it('成功时 refreshNode 父目录（\\ 分隔）', async () => {
      const { RenameFile } = await import('../../../wailsjs/go/main/App')
      wrapper.vm.$.setupState.renameNode = { path: 'D:\\dir\\a.go', name: 'a.go' }
      wrapper.vm.$.setupState.renameName = 'b.go'
      await wrapper.vm.handleRename()
      await flushPromises()
      expect(RenameFile).toHaveBeenCalledWith('D:\\dir\\a.go', 'b.go')
      expect(ElMessage.success).toHaveBeenCalledWith('重命名成功')
    })

    it('成功时 refreshNode 父目录（/ 分隔）', async () => {
      const { RenameFile } = await import('../../../wailsjs/go/main/App')
      wrapper.vm.$.setupState.renameNode = { path: 'dir/a.go', name: 'a.go' }
      wrapper.vm.$.setupState.renameName = 'b.go'
      await wrapper.vm.handleRename()
      await flushPromises()
      expect(RenameFile).toHaveBeenCalledWith('dir/a.go', 'b.go')
      expect(ElMessage.success).toHaveBeenCalledWith('重命名成功')
    })

    it('返回 false 时 error', async () => {
      const { RenameFile } = await import('../../../wailsjs/go/main/App')
      RenameFile.mockResolvedValueOnce(false)
      wrapper.vm.$.setupState.renameNode = { path: 'D:\\a.go', name: 'a.go' }
      wrapper.vm.$.setupState.renameName = 'b.go'
      await wrapper.vm.handleRename()
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith('重命名失败')
    })
  })

  describe('handleCopyTo 校验', () => {
    it('原地址为空时 warning', () => {
      wrapper.vm.$.setupState.copyToSourcePath = ''
      wrapper.vm.$.setupState.copyToTargetPath = 'D:\\dst'
      wrapper.vm.handleCopyTo()
      expect(ElMessage.warning).toHaveBeenCalledWith('请输入原地址')
    })

    it('目标地址为空时 warning', () => {
      wrapper.vm.$.setupState.copyToSourcePath = 'D:\\src'
      wrapper.vm.$.setupState.copyToTargetPath = ''
      wrapper.vm.handleCopyTo()
      expect(ElMessage.warning).toHaveBeenCalledWith('请输入目标地址')
    })

    it('文件名含非法字符时 warning', () => {
      wrapper.vm.$.setupState.copyToSourcePath = 'D:\\src'
      wrapper.vm.$.setupState.copyToTargetPath = 'D:\\dst'
      wrapper.vm.$.setupState.copyToTargetName = 'a:b'
      wrapper.vm.handleCopyTo()
      expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('非法字符'))
    })

    it('校验通过时 emit copyTo', () => {
      wrapper.vm.$.setupState.copyToSourcePath = 'D:/src'
      wrapper.vm.$.setupState.copyToTargetPath = 'D:/dst'
      wrapper.vm.$.setupState.copyToTargetName = 'newname'
      wrapper.vm.$.setupState.copyToWholeDir = true
      wrapper.vm.handleCopyTo()
      expect(wrapper.emitted('copyTo')).toBeTruthy()
      const payload = wrapper.emitted('copyTo')[0][0]
      expect(payload.targetName).toBe('newname')
      expect(payload.copyWholeDir).toBe(true)
    })
  })

  describe('外部工具 handler', () => {
    it('handleOpenExplorer 失败时 error', async () => {
      const { OpenInExplorer } = await import('../../../wailsjs/go/main/App')
      OpenInExplorer.mockResolvedValueOnce(false)
      await wrapper.vm.$.setupState.handleOpenExplorer('D:\\dir')
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith('打开资源管理器失败')
    })

    it('handleOpenExplorer 抛异常时 error', async () => {
      const { OpenInExplorer } = await import('../../../wailsjs/go/main/App')
      OpenInExplorer.mockRejectedValueOnce(new Error('x'))
      await wrapper.vm.$.setupState.handleOpenExplorer('D:\\dir')
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('x'))
    })

    it('handleOpenInVSCode 失败时 error', async () => {
      const { OpenInVSCode } = await import('../../../wailsjs/go/main/App')
      OpenInVSCode.mockResolvedValueOnce(false)
      await wrapper.vm.$.setupState.handleOpenInVSCode('D:\\a.go')
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('VSCode'))
    })

    it('handleOpenWithDefaultApp 失败时 error', async () => {
      const { OpenWithDefaultApp } = await import('../../../wailsjs/go/main/App')
      OpenWithDefaultApp.mockResolvedValueOnce(false)
      await wrapper.vm.$.setupState.handleOpenWithDefaultApp('D:\\a.go')
      await flushPromises()
      expect(ElMessage.error).toHaveBeenCalledWith('打开文件失败')
    })
  })

  describe('copyToClipboard', () => {
    it('成功时 success 提示', async () => {
      const writeText = vi.fn(() => Promise.resolve())
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      await wrapper.vm.$.setupState.copyToClipboard('text', '路径')
      expect(ElMessage.success).toHaveBeenCalledWith('路径已复制到剪贴板')
      vi.unstubAllGlobals()
    })

    it('失败时 error 提示', async () => {
      const writeText = vi.fn(() => Promise.reject(new Error('denied')))
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      await wrapper.vm.$.setupState.copyToClipboard('text', '路径')
      expect(ElMessage.error).toHaveBeenCalledWith('复制失败')
      vi.unstubAllGlobals()
    })
  })

  describe('节点点击与右键菜单', () => {
    it('onNodeClick：叶子/文件节点 emit select 后直接返回', () => {
      const data = { path: 'D:\\a.go', type: 'file', isLeaf: true }
      wrapper.vm.$.setupState.onNodeClick(data, { expanded: false })
      expect(wrapper.emitted('select')).toBeTruthy()
    })

    it('onNodeClick：已展开已选中目录收起', () => {
      wrapper.vm.$.setupState.currentSelectedPath = 'D:\\dir'
      const data = { path: 'D:\\dir', type: 'directory' }
      const node = { expanded: true, collapse: vi.fn(), expand: vi.fn() }
      wrapper.vm.$.setupState.onNodeClick(data, node)
      expect(node.collapse).toHaveBeenCalled()
    })

    it('onNodeClick：未展开目录展开', () => {
      const data = { path: 'D:\\dir', type: 'directory' }
      const node = { expanded: false, collapse: vi.fn(), expand: vi.fn() }
      wrapper.vm.$.setupState.onNodeClick(data, node)
      expect(node.expand).toHaveBeenCalled()
    })

    it('onNodeContextMenu：设置菜单位置 + emit contextmenu', () => {
      const event = { preventDefault: () => {}, stopPropagation: () => {}, clientX: 100, clientY: 200 }
      wrapper.vm.$.setupState.onNodeContextMenu(event, { path: 'D:\\a.go', name: 'a.go' })
      expect(wrapper.emitted('contextmenu')).toBeTruthy()
      expect(wrapper.vm.$.setupState.contextMenu.visible).toBe(true)
      expect(wrapper.vm.$.setupState.contextMenu.isBlankArea).toBe(false)
      expect(wrapper.vm.$.setupState.contextMenu.data.path).toBe('D:\\a.go')
    })

    it('closeContextMenu 关闭菜单', () => {
      wrapper.vm.$.setupState.contextMenu.visible = true
      wrapper.vm.$.setupState.contextMenu.isBlankArea = true
      wrapper.vm.$.setupState.closeContextMenu()
      expect(wrapper.vm.$.setupState.contextMenu.visible).toBe(false)
      expect(wrapper.vm.$.setupState.contextMenu.isBlankArea).toBe(false)
    })

    it('onGlobalClick 关闭菜单', () => {
      wrapper.vm.$.setupState.contextMenu.visible = true
      wrapper.vm.$.setupState.onGlobalClick()
      expect(wrapper.vm.$.setupState.contextMenu.visible).toBe(false)
    })

    it('onGlobalContextMenu 关闭菜单', () => {
      wrapper.vm.$.setupState.contextMenu.visible = true
      wrapper.vm.$.setupState.onGlobalContextMenu()
      expect(wrapper.vm.$.setupState.contextMenu.visible).toBe(false)
    })

    it('onBlankAreaContextMenu：设菜单数据为当前工作目录', () => {
      const event = { stopPropagation: () => {}, clientX: 50, clientY: 50 }
      wrapper.vm.$.setupState.onBlankAreaContextMenu(event)
      expect(wrapper.emitted('contextmenu')).toBeTruthy()
      expect(wrapper.vm.$.setupState.contextMenu.visible).toBe(true)
      expect(wrapper.vm.$.setupState.contextMenu.isBlankArea).toBe(true)
      // mockDirectories[0].path = /path/a
      expect(wrapper.vm.$.setupState.contextMenu.data.type).toBe('directory')
    })
  })

  describe('refreshAll / 拷贝到纯函数', () => {
    it('refreshAll 先清全部缓存再自增 counter + success 提示', async () => {
      const { ClearAllFileTreeCache } = await import('../../../wailsjs/go/main/App')
      const before = wrapper.vm.$.setupState.refreshCounter
      await wrapper.vm.$.setupState.refreshAll()
      expect(ClearAllFileTreeCache).toHaveBeenCalled()
      expect(wrapper.vm.$.setupState.refreshCounter).toBe(before + 1)
      expect(ElMessage.success).toHaveBeenCalledWith('文件树已刷新')
    })

    it('defaultCopyToName：路径末段', () => {
      wrapper.vm.$.setupState.copyToSourcePath = 'D:\\src\\dir'
      expect(wrapper.vm.$.setupState.defaultCopyToName()).toBe('dir')
    })

    it('copyToPreview：整目录模式 from → dst/name', () => {
      wrapper.vm.$.setupState.copyToSourcePath = 'D:/src'
      wrapper.vm.$.setupState.copyToTargetPath = 'D:/dst'
      wrapper.vm.$.setupState.copyToWholeDir = true
      wrapper.vm.$.setupState.copyToTargetName = 'newname'
      const p = wrapper.vm.$.setupState.copyToPreview
      expect(p.from).toBe('D:/src')
      expect(p.to).toBe('D:/dst/newname')
    })

    it('copyToPreview：非整目录模式 from/* → dst/*', () => {
      wrapper.vm.$.setupState.copyToSourcePath = 'D:/src'
      wrapper.vm.$.setupState.copyToTargetPath = 'D:/dst'
      wrapper.vm.$.setupState.copyToWholeDir = false
      const p = wrapper.vm.$.setupState.copyToPreview
      expect(p.from).toBe('D:/src/*')
      expect(p.to).toBe('D:/dst/*')
    })

    it('copyToPreview：source/target 为空时返回 null', () => {
      wrapper.vm.$.setupState.copyToSourcePath = ''
      wrapper.vm.$.setupState.copyToTargetPath = 'D:/dst'
      expect(wrapper.vm.$.setupState.copyToPreview).toBeNull()
    })

    it('swapCopyToPaths 交换原地址与目标地址', () => {
      wrapper.vm.$.setupState.copyToSourcePath = 'D:/a'
      wrapper.vm.$.setupState.copyToTargetPath = 'D:/b'
      wrapper.vm.$.setupState.swapCopyToPaths()
      expect(wrapper.vm.$.setupState.copyToSourcePath).toBe('D:/b')
      expect(wrapper.vm.$.setupState.copyToTargetPath).toBe('D:/a')
    })
  })

  describe('收藏 handler', () => {
    it('handleAddFavorite 成功时 success', async () => {
      const { AddFavorite } = await import('../../../wailsjs/go/main/App')
      AddFavorite.mockResolvedValueOnce('')
      await wrapper.vm.$.setupState.handleAddFavorite({ path: 'D:\\a.go' })
      await flushPromises()
      expect(AddFavorite).toHaveBeenCalled()
      expect(ElMessage.success).toHaveBeenCalledWith('已添加到收藏')
    })

    it('handleAddFavorite 返回错误时 warning', async () => {
      const { AddFavorite } = await import('../../../wailsjs/go/main/App')
      AddFavorite.mockResolvedValueOnce('已存在')
      await wrapper.vm.$.setupState.handleAddFavorite({ path: 'D:\\a.go' })
      await flushPromises()
      expect(ElMessage.warning).toHaveBeenCalledWith('已存在')
    })

    it('handleRemoveFavorite 成功时 success', async () => {
      const { RemoveFavorite } = await import('../../../wailsjs/go/main/App')
      RemoveFavorite.mockResolvedValueOnce('')
      await wrapper.vm.$.setupState.handleRemoveFavorite({ path: 'D:\\a.go' })
      await flushPromises()
      expect(ElMessage.success).toHaveBeenCalledWith('已取消收藏')
    })
  })
})
