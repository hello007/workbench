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
})
