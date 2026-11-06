import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import FileTreePanel from '../FileTreePanel.vue'
import { useDirectoryStore, useWorkspaceStore, useUiStore, useFavoritesStore } from '../../store'

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
  GetClaudeDefaultModel: vi.fn(() => Promise.resolve('')),
  GetFileTree: vi.fn(() => Promise.resolve([])),
  InvalidateFileTreeCache: vi.fn(() => Promise.resolve()),
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
  // Obsidian 域（未注册顺序两段链守护用例触达；默认 '' = 已注册零分支，用例按需 Once 覆盖）
  OpenInObsidian: vi.fn(() => Promise.resolve('')),
  AutoRegisterAndOpen: vi.fn(() => Promise.resolve(0)),
  OpenObsidianVaultManager: vi.fn(() => Promise.resolve(true)),
  CopyObsidianVaultPath: vi.fn(() => Promise.resolve('')),
  ScanAndPullRepos: vi.fn(() => Promise.resolve({ total: 0 })),
  GetFavorites: vi.fn(() => Promise.resolve([])),
  // 聚合视图根节点（后端一次判定存在性 + git 徽标）：默认空数组 = 无目录收藏空态
  GetAggregatedRootNodes: vi.fn(() => Promise.resolve([])),
  AddFavorite: vi.fn(() => Promise.resolve(true)),
  RemoveFavorite: vi.fn(() => Promise.resolve(true)),
  UpdateFavoriteAlias: vi.fn(() => Promise.resolve(true)),
  UpdateFavoriteGroup: vi.fn(() => Promise.resolve(true)),
  // 状态看板 pin：值对齐 src/test/wails-mock-defaults.js 单一数据源
  IsDashboardPinned: vi.fn(() => Promise.resolve(false)),
  AddDashboardPin: vi.fn(() => Promise.resolve(true)),
  RemoveDashboardPin: vi.fn(() => Promise.resolve(true)),
  // AI 对话目录域（aiChat store 经右键「添加到 AI 对话」触达）
  ListChatDirectories: vi.fn(() => Promise.resolve([])),
  AddChatDirectory: vi.fn(() => Promise.resolve({ id: 'chatdir-1', path: '/path/a/sub', displayName: 'sub', sortOrder: 0, createdAt: 1000 })),
  UpdateChatDirectory: vi.fn(() => Promise.resolve()),
  RemoveChatDirectory: vi.fn(() => Promise.resolve()),
  ReorderChatDirectories: vi.fn(() => Promise.resolve()),
  // PPT 按页拆分（右键 .pptx 文件「按页拆分」入口），形状对齐 model.PptxSplitResult
  SplitPptxPerPage: vi.fn(() =>
    Promise.resolve({ outputDir: '/path/a/演示', files: ['/path/a/演示/演示-01.pptx'], pageCount: 1 })
  ),
  CancelPptxSplit: vi.fn(() => Promise.resolve(false))
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
  // 渲染 description 文本：空态文案（工作目录未选/聚合视图无收藏）须可断言
  'el-empty': { template: '<div class="el-empty">{{ description }}</div>', props: ['description', 'imageSize'] },
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
  DocumentCopy: { template: '<span>dc</span>' },
  Files: { template: '<span>files</span>' }
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
  // stubs 覆盖位：聚合视图用例需给 el-tree 桩补 store.root（getExpandedPaths/
  // waitUntil 依赖 root.childNodes），默认桩保持 root: null 不影响既有用例
  const { directories = mockDirectories, selectedDirId = 'dir-1', stubs } = props
  const pinia = setupDirectoryStore(directories, selectedDirId)
  return mount(FileTreePanel, {
    global: { plugins: [pinia], stubs: stubs ? { ...defaultStubs, ...stubs } : defaultStubs }
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

  describe('单层超限截断（perf-baseline §15.4）', () => {
    // 构造超限节点：默认阈值 2000，用小阈值直调组件内部截断路径不可行
    // （loadTreeNode 用默认 limit），故 mock 大量节点走真实 2000 阈值
    const makeNodes = n => Array.from({ length: n }, (_, i) => ({
      name: `file${i}.txt`,
      path: `/path/a/file${i}.txt`,
      type: 'file',
      hasChildren: false,
      isLeaf: true
    }))

    it('根层超限时 resolve 列表 = limit + 1 个哨兵，哨兵 path 派生自工作目录路径', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce(makeNodes(2003))

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
      await flushPromises()

      const resolvedNodes = resolve.mock.calls[0][0]
      expect(resolvedNodes).toHaveLength(2001)
      const hint = resolvedNodes[resolvedNodes.length - 1]
      expect(hint.type).toBe('truncation-hint')
      expect(hint.path).toBe('/path/a\0truncation-hint')
      expect(hint.isLeaf).toBe(true)
      // 前 2000 项为真实节点，顺序保持
      expect(resolvedNodes[0].name).toBe('file0.txt')
      expect(resolvedNodes[1999].name).toBe('file1999.txt')
    })

    it('子层超限时哨兵 path 派生自 node.data.path（多目录截断不冲突）', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce(makeNodes(2001).map(n => ({
        ...n,
        path: `/path/a/src/${n.name}`
      })))

      wrapper = createWrapper()
      const resolve = vi.fn()
      await wrapper.vm.loadTreeNode({ level: 1, data: { path: '/path/a/src' } }, resolve)
      await flushPromises()

      const resolvedNodes = resolve.mock.calls[0][0]
      expect(resolvedNodes[resolvedNodes.length - 1].path).toBe('/path/a/src\0truncation-hint')
    })

    it('点击哨兵节点不 emit select', async () => {
      wrapper = createWrapper()
      const hintData = {
        name: '已显示前 2000 项',
        path: '/path/a\0truncation-hint',
        type: 'truncation-hint',
        isLeaf: true
      }

      const tree = wrapper.findComponent('.el-tree')
      tree.vm.$attrs.onNodeClick(hintData, { expanded: false })
      await flushPromises()

      expect(wrapper.emitted('select')).toBeFalsy()
    })

    it('右键哨兵节点不打开上下文菜单', async () => {
      wrapper = createWrapper()
      const hintData = {
        name: '已显示前 2000 项',
        path: '/path/a\0truncation-hint',
        type: 'truncation-hint',
        isLeaf: true
      }
      const event = { preventDefault: vi.fn(), stopPropagation: vi.fn(), clientX: 10, clientY: 10 }

      const tree = wrapper.findComponent('.el-tree')
      tree.vm.$attrs.onNodeContextmenu(event, hintData)
      await flushPromises()

      expect(wrapper.vm.contextMenu.visible).toBe(false)
    })

    // 哨兵经 el-tree highlight-current 可能成为 getCurrentNode 结果（组件 onNodeClick
    // 拦不住内部 setCurrentNode），F2/Delete 快捷键须过滤防对合成 path 弹对话框。
    // getCurrentNode 契约：组件层包装返回 currentNode.data（裸 data 对象，非 Node），
    // mock 须返回裸 data；断言副作用（对话框状态/确认弹窗）而非 spy 内部函数——
    // script setup 绑定经 vm 代理写回不落组件内部闭包引用
    it('F2/删除快捷键作用于哨兵节点时不弹对话框', async () => {
      const { ElMessageBox } = await import('element-plus')
      wrapper = createWrapper()
      wrapper.vm.fileTreeRef = {
        getCurrentNode: vi.fn(() => ({
          name: '已显示前 2000 项', path: '/path/a\0truncation-hint', type: 'truncation-hint', isLeaf: true
        }))
      }

      wrapper.vm.triggerRenameCurrent()
      wrapper.vm.triggerDeleteCurrent()
      await flushPromises()

      expect(wrapper.vm.renameDialogVisible).toBe(false)
      expect(ElMessageBox.confirm).not.toHaveBeenCalled()
    })

    it('F2/删除快捷键作用于真实节点时正常触发重命名对话框', async () => {
      wrapper = createWrapper()
      wrapper.vm.fileTreeRef = {
        getCurrentNode: vi.fn(() => ({
          name: 'a.txt', path: '/path/a/a.txt', type: 'file', isLeaf: true
        }))
      }

      wrapper.vm.triggerRenameCurrent()
      await flushPromises()

      expect(wrapper.vm.renameDialogVisible).toBe(true)
      // 预填值来自 getCurrentNode 返回的 data 对象本身（证明真契约贯通）
      expect(wrapper.vm.renameName).toBe('a.txt')
      expect(wrapper.vm.renameNode).toMatchObject({ path: '/path/a/a.txt' })
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

      // 名称输入框在对话框容器内查找（工具栏筛选框在 dialog 外，天然排除）
      const inputs = wrapper.find('form').findAll('input')
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

      const inputs = wrapper.find('form').findAll('input')
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

      const inputs = wrapper.find('form').findAll('input')
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

    it('refreshNode 应先调 InvalidateFileTreeCache 清后端缓存再触发 expand', async () => {
      const { InvalidateFileTreeCache } = await import('../../../wailsjs/go/main/App')
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

      expect(InvalidateFileTreeCache).toHaveBeenCalledWith('/path/a/src/foo')
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

  // ---- 目录内按名筛选（截断优化配套，perf-baseline §15.6 盲区补齐）----
  describe('目录内按名筛选', () => {
    const makeNodes = n => Array.from({ length: n }, (_, i) => ({
      name: `file${i}.txt`,
      path: `/path/a/file${i}.txt`,
      type: 'file',
      hasChildren: false,
      isLeaf: true
    }))

    it('filterModeActive：关键词非空激活，空串未激活', async () => {
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      expect(state.filterModeActive).toBe(false)
      state.treeFilterKeyword = 'abc'
      await flushPromises()
      expect(state.filterModeActive).toBe(true)
    })

    it('runFilter：按作用域路径拉 GetFileTree 全量后过滤，命中写入 filteredNodes', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce([
        ...makeNodes(2),
        { name: 'README.md', path: '/path/a/README.md', type: 'file', isLeaf: true }
      ])
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'README'
      await state.runFilter()
      await flushPromises()

      expect(GetFileTree).toHaveBeenCalledWith('/path/a')
      expect(state.filteredNodes).toHaveLength(1)
      expect(state.filteredNodes[0].name).toBe('README.md')
    })

    it('runFilter：筛选前最后点击目录为作用域（目录节点），文件点击回退根层', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState

      // 点击目录节点记录作用域
      state.onNodeClick({ name: 'sub', path: '/path/a/sub', type: 'directory' }, { expanded: false, expand: vi.fn() })
      state.treeFilterKeyword = 'x'
      await state.runFilter()
      await flushPromises()
      expect(GetFileTree).toHaveBeenLastCalledWith('/path/a/sub')

      // 点击文件节点 → 作用域回退工作目录根层
      state.onNodeClick({ name: 'a.txt', path: '/path/a/a.txt', type: 'file' }, { expanded: false })
      await state.runFilter()
      await flushPromises()
      expect(GetFileTree).toHaveBeenLastCalledWith('/path/a')
    })

    it('runFilter：GetFileTree 失败时 error 提示且 filteredNodes 清空', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockRejectedValueOnce(new Error('读取失败'))
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'x'
      state.filteredNodes = makeNodes(1)
      await state.runFilter()
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalled()
      expect(state.filteredNodes).toEqual([])
    })

    it('exitFilterMode：清空关键词与命中列表', async () => {
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'abc'
      state.filteredNodes = makeNodes(2)
      state.exitFilterMode()

      expect(state.treeFilterKeyword).toBe('')
      expect(state.filteredNodes).toEqual([])
      expect(state.filterModeActive).toBe(false)
    })

    it('onFilterResultClick 文件：emit select 且筛选态保持', async () => {
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'kw'
      const item = { name: 'hit.go', path: '/path/a/hit.go', type: 'file' }
      state.onFilterResultClick(item)
      await flushPromises()

      expect(wrapper.emitted('select')[0][0]).toEqual(item)
      expect(state.treeFilterKeyword).toBe('kw')
    })

    it('onFilterResultClick 目录：退出筛选回树（不 emit select）', async () => {
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'kw'
      state.onFilterResultClick({ name: 'sub', path: '/path/a/sub', type: 'directory' })
      await flushPromises()

      expect(state.treeFilterKeyword).toBe('')
      expect(wrapper.emitted('select')).toBeFalsy()
    })

    it('onFilterResultClick 哨兵：纯提示不可交互（不 emit select 不退筛选）', async () => {
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'kw'
      state.onFilterResultClick({ name: '提示', path: '/path/a\0truncation-hint', type: 'truncation-hint', isLeaf: true })
      await flushPromises()

      expect(wrapper.emitted('select')).toBeFalsy()
      expect(state.treeFilterKeyword).toBe('kw')
    })

    it('refreshNode 前置退出筛选态', async () => {
      wrapper = createWrapperWithStore()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'kw'
      await wrapper.vm.refreshNode('/path/a/src')
      await flushPromises()

      expect(state.treeFilterKeyword).toBe('')
    })

    it('treeKey 变化（整体刷新）重置筛选态', async () => {
      wrapper = createWrapperWithStore()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'kw'
      state.filteredNodes = makeNodes(1)
      await wrapper.vm.$.setupState.refreshAll()
      await flushPromises()

      expect(state.treeFilterKeyword).toBe('')
      expect(state.filteredNodes).toEqual([])
    })

    it('filteredDisplayNodes：命中超限时复用截断（2000 + 哨兵）', async () => {
      const { GetFileTree } = await import('../../../wailsjs/go/main/App')
      GetFileTree.mockResolvedValueOnce(makeNodes(2003).map(n => ({ ...n, name: `x-${n.name}` })))
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'file'
      await state.runFilter()
      await flushPromises()

      expect(state.filteredNodes).toHaveLength(2003)
      expect(state.filteredDisplayNodes).toHaveLength(2001)
      expect(state.filteredDisplayNodes[2000].type).toBe('truncation-hint')
      expect(state.filteredDisplayNodes[2000].path).toBe('/path/a\0truncation-hint')
    })

    it('onNodeClick 哨兵分支聚焦筛选框且不 emit select', async () => {
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.onNodeClick({
        name: '提示', path: '/path/a\0truncation-hint', type: 'truncation-hint', isLeaf: true
      }, { expanded: false })
      await flushPromises()

      expect(wrapper.emitted('select')).toBeFalsy()
      expect(state.currentSelectedPath).toBe('')
    })

    it('onFilterResultClick 文件命中树中节点：同步 setCurrentKey + currentSelectedPath（防快捷键错位）', async () => {
      wrapper = createWrapperWithStore()
      const state = wrapper.vm.$.setupState
      const setCurrentKey = vi.fn()
      const getNode = vi.fn(p => (p === '/path/a/hit.go' ? { data: { path: p } } : null))
      state.fileTreeRef = { setCurrentKey, getNode }

      state.onFilterResultClick({ name: 'hit.go', path: '/path/a/hit.go', type: 'file' })
      await flushPromises()

      expect(setCurrentKey).toHaveBeenCalledWith('/path/a/hit.go')
      expect(state.currentSelectedPath).toBe('/path/a/hit.go')
    })

    it('onFilterResultClick 文件命中树外节点（截断层外）：清当前键防快捷键命中旧节点', async () => {
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      const setCurrentKey = vi.fn()
      state.fileTreeRef = { setCurrentKey, getNode: vi.fn(() => null) }

      state.onFilterResultClick({ name: 'ghost.go', path: '/path/a/ghost.go', type: 'file' })
      await flushPromises()

      expect(setCurrentKey).toHaveBeenCalledWith(null)
      expect(wrapper.emitted('select')[0][0].path).toBe('/path/a/ghost.go')
    })

    // ---- 覆盖层右键菜单（09-27-filetree-filter-overlay-context-menu）----

    it('覆盖层命中行右键：复用 onNodeContextMenu 弹节点菜单（data 指向命中项）', async () => {
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'hit'
      state.filteredNodes = [{ name: 'hit.go', path: '/path/a/hit.go', type: 'file', isLeaf: true }]
      await flushPromises()

      await wrapper.find('.filter-result-node').trigger('contextmenu')
      await flushPromises()

      expect(state.contextMenu.visible).toBe(true)
      expect(state.contextMenu.data.path).toBe('/path/a/hit.go')
      expect(state.contextMenu.isBlankArea).toBe(false)
    })

    it('覆盖层哨兵行右键：不弹菜单（哨兵过滤面不回归）', async () => {
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'kw'
      state.filteredNodes = [{ name: '提示', path: '/path/a\0truncation-hint', type: 'truncation-hint', isLeaf: true }]
      await flushPromises()

      await wrapper.find('.filter-result-node').trigger('contextmenu')
      await flushPromises()

      expect(state.contextMenu.visible).toBe(false)
    })

    it('筛选语境删除命中文件：显式失效父目录缓存 + 重跑筛选保语境（不退筛选）', async () => {
      const { GetFileTree, DeleteFile, InvalidateFileTreeCache } = await import('../../../wailsjs/go/main/App')
      const hitA = { name: 'hit-a.go', path: '/path/a/hit-a.go', type: 'file', isLeaf: true }
      const hitB = { name: 'hit-b.go', path: '/path/a/hit-b.go', type: 'file', isLeaf: true }
      GetFileTree.mockResolvedValueOnce([hitA, hitB])
      DeleteFile.mockResolvedValueOnce(true)
      // 操作后 runFilter 重拉：hit-a 已删
      GetFileTree.mockResolvedValueOnce([hitB])

      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'hit'
      await state.runFilter()
      await flushPromises()
      expect(state.filteredNodes).toHaveLength(2)

      const { ElMessageBox } = await import('element-plus')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')
      await state.handleDeleteAt(hitA)
      await flushPromises()

      expect(DeleteFile).toHaveBeenCalledWith('/path/a/hit-a.go')
      // refreshAfterFilterOp 显式失效父目录缓存（refreshNode target miss 早退不清缓存的缺口补齐）
      expect(InvalidateFileTreeCache).toHaveBeenCalledWith('/path/a')
      // 筛选语境保持：关键词未清，命中列表反映删除后状态
      expect(state.treeFilterKeyword).toBe('hit')
      expect(state.filteredNodes).toHaveLength(1)
      expect(state.filteredNodes[0].path).toBe('/path/a/hit-b.go')
      expect(wrapper.emitted('delete')).toBeTruthy()
    })

    it('筛选语境重命名命中文件：保语境刷新命中列表（新名仍含关键词则保留）', async () => {
      const { GetFileTree, RenameFile, InvalidateFileTreeCache } = await import('../../../wailsjs/go/main/App')
      const hit = { name: 'hit.go', path: '/path/a/hit.go', type: 'file', isLeaf: true }
      GetFileTree.mockResolvedValueOnce([hit])
      RenameFile.mockResolvedValueOnce(true)
      // 操作后 runFilter 重拉：已更名为 renamed-hit.go
      const renamed = { name: 'renamed-hit.go', path: '/path/a/renamed-hit.go', type: 'file', isLeaf: true }
      GetFileTree.mockResolvedValueOnce([renamed])

      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'hit'
      await state.runFilter()
      await flushPromises()

      state.showRenameAt(hit)
      state.renameName = 'renamed-hit.go'
      await state.handleRename()
      await flushPromises()

      expect(RenameFile).toHaveBeenCalledWith('/path/a/hit.go', 'renamed-hit.go')
      expect(InvalidateFileTreeCache).toHaveBeenCalledWith('/path/a')
      expect(state.treeFilterKeyword).toBe('hit')
      expect(state.filteredNodes).toHaveLength(1)
      expect(state.filteredNodes[0].path).toBe('/path/a/renamed-hit.go')
    })

    it('非筛选语境删除命中项：维持退筛选现状（不重跑筛选）', async () => {
      const { GetFileTree, InvalidateFileTreeCache } = await import('../../../wailsjs/go/main/App')
      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      // 未激活筛选：右键入口为树视图，删除后走 refreshNode 现状路径
      expect(state.filterModeActive).toBe(false)

      const { ElMessageBox } = await import('element-plus')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')
      await state.handleDeleteAt({ name: 'a.go', path: '/path/a/a.go', type: 'file' })
      await flushPromises()

      expect(state.treeFilterKeyword).toBe('')
      // 筛选未激活不重跑 runFilter：GetFileTree 仅懒加载 0 次（stub 环境无树渲染）
      expect(GetFileTree).not.toHaveBeenCalled()
      // stub 环境 refreshNode target miss 早退，不产生额外缓存失效调用
      expect(InvalidateFileTreeCache).not.toHaveBeenCalled()
    })

    it('非筛选语境删除：树语境 refreshNode 真实执行（root 兜底命中清缓存 + 触发重载）', async () => {
      // 上一用例的否定断言依赖 stub 环境 refreshNode 早退；本用例以 root 兜底命中的
      // store mock 验证树语境 refreshNode 路径真实执行到缓存失效与 expand 重载：
      // InvalidateFileTreeCache 父目录 1 次（refreshNode 内部，无 refreshAfterFilterOp
      // 的第二次独立显式失效）；GetFileTree 重拉由 el-tree loadData 内部机制驱动，
      // stub 无法真实触发，不断言（E2E 已覆盖该链路）
      const { InvalidateFileTreeCache } = await import('../../../wailsjs/go/main/App')
      const rootExpand = vi.fn(function () { this.loaded = true })
      wrapper = createWrapperWithStore({
        root: { childNodes: [], data: null, expanded: false, loaded: false, expand: rootExpand }
      })
      const state = wrapper.vm.$.setupState

      const { ElMessageBox } = await import('element-plus')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')
      await state.handleDeleteAt({ name: 'a.go', path: '/path/a/a.go', type: 'file' })
      await flushPromises()

      expect(state.treeFilterKeyword).toBe('')
      expect(InvalidateFileTreeCache).toHaveBeenCalledTimes(1)
      expect(InvalidateFileTreeCache).toHaveBeenCalledWith('/path/a')
      expect(rootExpand).toHaveBeenCalledTimes(1)
    })

    it('筛选语境新建文件：保语境刷新命中列表（产物命中关键词时出现）', async () => {
      const { GetFileTree, CreateFile, InvalidateFileTreeCache } = await import('../../../wailsjs/go/main/App')
      const hitA = { name: 'hit-a.go', path: '/path/a/hit-a.go', type: 'file', isLeaf: true }
      GetFileTree.mockResolvedValueOnce([hitA])
      CreateFile.mockResolvedValueOnce(true)
      const newHit = { name: 'new-hit.go', path: '/path/a/new-hit.go', type: 'file', isLeaf: true }
      // 操作后 runFilter 重拉：新建产物命中关键词进入命中列表
      GetFileTree.mockResolvedValueOnce([hitA, newHit])

      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      state.treeFilterKeyword = 'hit'
      await state.runFilter()
      await flushPromises()
      expect(state.filteredNodes).toHaveLength(1)

      state.createParentData = { name: 'a', path: '/path/a', type: 'directory' }
      state.createType = 'file'
      state.createName = 'new-hit.go'
      await state.handleCreate()
      await flushPromises()

      expect(CreateFile).toHaveBeenCalledWith('/path/a', 'new-hit.go', '')
      // refreshAfterFileOp 分流到 refreshAfterFilterOp：显式失效父目录缓存
      expect(InvalidateFileTreeCache).toHaveBeenCalledWith('/path/a')
      // 筛选语境保持：覆盖层不退出，命中列表含新建产物
      expect(state.treeFilterKeyword).toBe('hit')
      expect(state.filteredNodes).toHaveLength(2)
      expect(state.filteredNodes[1].path).toBe('/path/a/new-hit.go')
    })

    it('树语境新建：走 refreshNode 现状（不重跑筛选，stub 环境早退不显式失效）', async () => {
      const { GetFileTree, CreateDirectory, InvalidateFileTreeCache } = await import('../../../wailsjs/go/main/App')
      CreateDirectory.mockResolvedValueOnce(true)

      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      expect(state.filterModeActive).toBe(false)

      state.createParentData = { name: 'a', path: '/path/a', type: 'directory' }
      state.createType = 'directory'
      state.createName = 'new-folder'
      await state.handleCreate()
      await flushPromises()

      expect(CreateDirectory).toHaveBeenCalledWith('/path/a', 'new-folder')
      expect(state.treeFilterKeyword).toBe('')
      // 筛选未激活不重跑 runFilter：GetFileTree 仅懒加载 0 次（stub 环境无树渲染）
      expect(GetFileTree).not.toHaveBeenCalled()
      expect(InvalidateFileTreeCache).not.toHaveBeenCalled()
    })

    it('refreshAfterFileOp 分流：筛选激活走保语境刷新（显式失效 + 重跑筛选），树语境走 refreshNode', async () => {
      const { GetFileTree, InvalidateFileTreeCache } = await import('../../../wailsjs/go/main/App')

      wrapper = createWrapper()
      const state = wrapper.vm.$.setupState
      expect(state.filterModeActive).toBe(false)

      // 树语境：分流到 refreshNode（stub 环境 target miss 早退，无缓存失效调用）
      await state.refreshAfterFileOp('/path/a')
      await flushPromises()
      expect(InvalidateFileTreeCache).not.toHaveBeenCalled()

      // 筛选激活：分流到 refreshAfterFilterOp（显式失效父目录缓存 + 重跑筛选保语境）
      const hit = { name: 'hit.go', path: '/path/a/hit.go', type: 'file', isLeaf: true }
      GetFileTree.mockResolvedValueOnce([hit])
      state.treeFilterKeyword = 'hit'
      await state.runFilter()
      await flushPromises()
      GetFileTree.mockResolvedValueOnce([hit])
      await state.refreshAfterFileOp('/path/a')
      await flushPromises()

      expect(InvalidateFileTreeCache).toHaveBeenCalledWith('/path/a')
      expect(state.treeFilterKeyword).toBe('hit')
      expect(state.filteredNodes).toHaveLength(1)
      expect(state.filteredNodes[0].path).toBe('/path/a/hit.go')
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

    it('jumpStats 设选中节点为 git 目录并切统计面板', () => {
      const workspaceStore = useWorkspaceStore()
      const uiStore = useUiStore()
      const gitDirData = { name: 'repo', path: 'D:\\proj\\repo', type: 'directory', isGitRepo: true }
      setMenuDataAndCommand('jumpStats', gitDirData)
      expect(workspaceStore.selectedNode?.path).toBe('D:\\proj\\repo')
      expect(workspaceStore.selectedNode?.name).toBe('repo')
      expect(workspaceStore.selectedNode?.isGitRepo).toBe(true)
      expect(uiStore.activePanel).toBe('stats')
    })

    it('refresh 命令关闭菜单，且非聚合刷新不清全部树缓存', async () => {
      const { ClearAllFileTreeCache } = await import('../../../wailsjs/go/main/App')
      wrapper.vm.$.setupState.contextMenu.visible = true
      setMenuDataAndCommand('refresh')
      // onMenuCommand 先 closeContextMenu
      expect(wrapper.vm.$.setupState.contextMenu.visible).toBe(false)
      // 回归：普通树刷新维持 refreshNode 定向刷新语义，不触发整树缓存清理
      expect(ClearAllFileTreeCache).not.toHaveBeenCalled()
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

    it('copyRelativePath / copyRefPath 以当前工作目录根求相对并写剪贴板', async () => {
      const writeText = vi.fn(() => Promise.resolve())
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      // 工作目录根 = /path/a（mockDirectories dir-1）
      const relData = { name: 'a.go', path: '/path/a/src/a.go', type: 'file' }
      setMenuDataAndCommand('copyRelativePath', relData)
      await flushPromises()
      expect(writeText).toHaveBeenCalledWith('src/a.go')
      setMenuDataAndCommand('copyRefPath', relData)
      await flushPromises()
      expect(writeText).toHaveBeenCalledWith('@src/a.go')
      vi.unstubAllGlobals()
    })

    it('copyRelativePath：节点不在工作目录根下时复制归一化绝对路径（防御分支）', async () => {
      const writeText = vi.fn(() => Promise.resolve())
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      setMenuDataAndCommand('copyRelativePath')
      await flushPromises()
      // nodeData.path = D:\proj\a.go 不在 /path/a 下 → 归一化绝对路径
      expect(writeText).toHaveBeenCalledWith('D:/proj/a.go')
      vi.unstubAllGlobals()
    })

    it('copyName 目录节点提示文件夹名', async () => {
      const writeText = vi.fn(() => Promise.resolve())
      vi.stubGlobal('navigator', { clipboard: { writeText } })
      setMenuDataAndCommand('copyName', { ...nodeData, type: 'directory' })
      await flushPromises()
      expect(writeText).toHaveBeenCalledWith('a.go')
      expect(ElMessage.success).toHaveBeenCalledWith('文件夹名已复制到剪贴板')
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

    it('addToAiChat 调 AddChatDirectory（displayName 空由后端取目录名）', async () => {
      const { AddChatDirectory } = await import('../../../wailsjs/go/main/App')
      const dirData = { name: 'sub', path: 'D:\\proj\\sub', type: 'directory' }
      setMenuDataAndCommand('addToAiChat', dirData)
      await flushPromises()
      expect(AddChatDirectory).toHaveBeenCalledWith('D:\\proj\\sub', '')
      expect(ElMessage.success).toHaveBeenCalledWith('已添加到 AI 对话「sub」')
    })

    it('addToAiChat 重复路径 info 提示不重复添加', async () => {
      const { AddChatDirectory } = await import('../../../wailsjs/go/main/App')
      const { useAiChatStore } = await import('../../store')
      const aiChatStore = useAiChatStore()
      aiChatStore.chatDirectories = [{ id: 'chatdir-x', path: 'd:/proj/sub', displayName: 'sub', sortOrder: 0, createdAt: 0 }]
      const dirData = { name: 'sub', path: 'D:\\proj\\sub', type: 'directory' }
      setMenuDataAndCommand('addToAiChat', dirData)
      await flushPromises()
      expect(ElMessage.info).toHaveBeenCalledWith('该目录已在 AI 对话列表中')
      expect(AddChatDirectory).not.toHaveBeenCalled()
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

    it('isFavorited：收藏路径带尾分隔符时与节点路径按归一比较命中', async () => {
      await flushPromises()
      // 收藏路径 D:\repo\ 与节点路径 D:\repo 属同一目录：右键须显示「取消收藏」
      useFavoritesStore().favorites = [{ path: 'D:\\repo\\', alias: '', group: '默认' }]
      const state = wrapper.vm.$.setupState

      state.contextMenu.data = { path: 'D:\\repo', name: 'repo', type: 'directory' }
      expect(state.isFavorited).toBe(true)

      // 分隔符与大小写差异同样归一
      state.contextMenu.data = { path: 'd:/repo', name: 'repo', type: 'directory' }
      expect(state.isFavorited).toBe(true)

      state.contextMenu.data = { path: 'D:\\other', name: 'other', type: 'directory' }
      expect(state.isFavorited).toBe(false)
    })
  })

  describe('toggleDashboardPin 状态看板关注', () => {
    const fakeEvent = { preventDefault: () => {}, stopPropagation: () => {}, clientX: 100, clientY: 200 }

    it('git 仓库节点右键触发 IsDashboardPinned 查询（参数为 data.path）', async () => {
      const { IsDashboardPinned } = await import('../../../wailsjs/go/main/App')
      const gitRepoData = { id: 'repo-1', name: 'my-repo', path: 'D:\\proj\\my-repo', type: 'directory', isGitRepo: true }
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, gitRepoData)
      await flushPromises()
      expect(IsDashboardPinned).toHaveBeenCalledWith('D:\\proj\\my-repo')
    })

    it('targetPinned=true 时点「取消关注状态看板」调 RemoveDashboardPin（不调 AddDashboardPin）', async () => {
      const { IsDashboardPinned, RemoveDashboardPin, AddDashboardPin } = await import('../../../wailsjs/go/main/App')
      IsDashboardPinned.mockResolvedValueOnce(true)
      const gitRepoData = { id: 'repo-1', name: 'my-repo', path: 'D:\\proj\\my-repo', type: 'directory', isGitRepo: true }
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, gitRepoData)
      await flushPromises()
      expect(wrapper.vm.$.setupState.contextMenu.targetPinned).toBe(true)
      wrapper.vm.onMenuCommand('toggleDashboardPin')
      await flushPromises()
      expect(RemoveDashboardPin).toHaveBeenCalledWith('D:\\proj\\my-repo')
      expect(AddDashboardPin).not.toHaveBeenCalled()
      expect(ElMessage.success).toHaveBeenCalledWith('已取消关注状态看板')
    })

    it('targetPinned=false 时点「加入状态看板」调 AddDashboardPin（不调 RemoveDashboardPin）', async () => {
      const { IsDashboardPinned, AddDashboardPin, RemoveDashboardPin } = await import('../../../wailsjs/go/main/App')
      // factory 默认 IsDashboardPinned 返回 false，无需 override
      const gitRepoData = { id: 'repo-2', name: 'another-repo', path: 'D:\\proj\\another-repo', type: 'directory', isGitRepo: true }
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, gitRepoData)
      await flushPromises()
      expect(wrapper.vm.$.setupState.contextMenu.targetPinned).toBe(false)
      wrapper.vm.onMenuCommand('toggleDashboardPin')
      await flushPromises()
      expect(AddDashboardPin).toHaveBeenCalledWith('D:\\proj\\another-repo')
      expect(RemoveDashboardPin).not.toHaveBeenCalled()
      expect(ElMessage.success).toHaveBeenCalledWith('已加入状态看板')
    })

    it('path 切换守卫：A 的 IsDashboardPinned 延迟返回时不更新 B 的 targetPinned', async () => {
      const { IsDashboardPinned } = await import('../../../wailsjs/go/main/App')
      let resolveA
      IsDashboardPinned.mockImplementationOnce(() => new Promise(r => { resolveA = r }))
      const repoA = { id: 'a', name: 'repo-a', path: 'D:\\proj\\repo-a', type: 'directory', isGitRepo: true }
      const repoB = { id: 'b', name: 'repo-b', path: 'D:\\proj\\repo-b', type: 'directory', isGitRepo: true }
      // 右键 A：发起 IsDashboardPinned(A)，尚未 resolve
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, repoA)
      // 右键 B：contextMenu.data 切到 B，B 的查询走默认 false 立即 resolve
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, repoB)
      await flushPromises()
      // B 的 targetPinned 为 false（默认），A 的延迟返回尚未 resolve
      expect(wrapper.vm.$.setupState.contextMenu.targetPinned).toBe(false)
      // A 的查询 resolve 为 true，但因 contextMenu.data.path 已是 B，守卫拦截不更新
      resolveA(true)
      await flushPromises()
      expect(wrapper.vm.$.setupState.contextMenu.targetPinned).toBe(false)
      expect(IsDashboardPinned).toHaveBeenCalledWith('D:\\proj\\repo-a')
      expect(IsDashboardPinned).toHaveBeenCalledWith('D:\\proj\\repo-b')
    })
  })

  describe('PPT 按页拆分（右键 .pptx 文件入口）', () => {
    const fakeEvent = { preventDefault: () => {}, stopPropagation: () => {}, clientX: 100, clientY: 200 }

    // 右键打开菜单后按文本查找菜单项
    const findMenuItem = (wrapper, text) =>
      wrapper.findAll('.context-menu .context-menu-item').find(li => li.text().includes(text))

    it('右键 .pptx 文件显示「按页拆分」菜单项', async () => {
      wrapper = createWrapper()
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, { name: '汇报.pptx', path: 'D:\\proj\\汇报.pptx', type: 'file' })
      await flushPromises()

      const item = findMenuItem(wrapper, '按页拆分')
      expect(item).toBeTruthy()
    })

    it('右键 .pptx 判断大小写不敏感（.PPTX 同样显示）', async () => {
      wrapper = createWrapper()
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, { name: 'DECK.PPTX', path: 'D:\\proj\\DECK.PPTX', type: 'file' })
      await flushPromises()

      expect(findMenuItem(wrapper, '按页拆分')).toBeTruthy()
    })

    it('右键非 .pptx 文件不显示「按页拆分」菜单项', async () => {
      wrapper = createWrapper()
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, { name: 'a.go', path: 'D:\\proj\\a.go', type: 'file' })
      await flushPromises()

      expect(findMenuItem(wrapper, '按页拆分')).toBeUndefined()
    })

    it('点击菜单项弹覆盖确认框，确认后调 SplitPptxPerPage 并 success 提示输出目录与页数', async () => {
      const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
      const { ElMessageBox } = await import('element-plus')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')
      wrapper = createWrapper()
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, { name: '汇报.pptx', path: 'D:\\proj\\汇报.pptx', type: 'file' })
      await flushPromises()

      await findMenuItem(wrapper, '按页拆分').trigger('click')
      await flushPromises()

      // 确认框预告覆盖行为与输出位置
      expect(ElMessageBox.confirm).toHaveBeenCalled()
      const [message] = ElMessageBox.confirm.mock.calls[0]
      expect(message).toContain('汇报')
      expect(message).toContain('覆盖')
      expect(SplitPptxPerPage).toHaveBeenCalledWith('D:\\proj\\汇报.pptx')
      expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('/path/a/演示'))
      expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('1 页'))
    })

    it('用户取消确认不调用 SplitPptxPerPage', async () => {
      const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
      const { ElMessageBox } = await import('element-plus')
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')
      wrapper = createWrapper()
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, { name: '汇报.pptx', path: 'D:\\proj\\汇报.pptx', type: 'file' })
      await flushPromises()

      await findMenuItem(wrapper, '按页拆分').trigger('click')
      await flushPromises()

      expect(SplitPptxPerPage).not.toHaveBeenCalled()
    })

    it('拆分被取消（E_GIT_CANCELLED）经 handleError 分流为 warning 中性提示', async () => {
      const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
      const { ElMessageBox } = await import('element-plus')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')
      SplitPptxPerPage.mockRejectedValueOnce({ code: 'E_GIT_CANCELLED', message: '操作已取消' })
      wrapper = createWrapper()
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, { name: '汇报.pptx', path: 'D:\\proj\\汇报.pptx', type: 'file' })
      await flushPromises()

      await findMenuItem(wrapper, '按页拆分').trigger('click')
      await flushPromises()

      expect(ElMessage.warning).toHaveBeenCalledWith('操作已取消')
      expect(ElMessage.error).not.toHaveBeenCalled()
    })

    it('拆分真失败经 handleError 分流为 error（带前缀）', async () => {
      const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
      const { ElMessageBox } = await import('element-plus')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')
      SplitPptxPerPage.mockRejectedValueOnce({ code: 'E_REPO_CONFIG_INVALID_JSON', message: '文件损坏' })
      wrapper = createWrapper()
      wrapper.vm.$.setupState.onNodeContextMenu(fakeEvent, { name: '汇报.pptx', path: 'D:\\proj\\汇报.pptx', type: 'file' })
      await flushPromises()

      await findMenuItem(wrapper, '按页拆分').trigger('click')
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalledWith('PPT 拆分失败: 文件损坏')
      expect(ElMessage.warning).not.toHaveBeenCalled()
    })
  })

  // EP 2.14.5 顺序两段链守护（10-20-ep-2-14-5-e2e Q3）：与 DirectoryTree/ContentPanel
  // 逐字节同构的三处之一，独立守护防一处改坏他处不知（对齐 AiChatPanel 删除在途
  // 会话 times(2) 先例）。
  describe('Obsidian 未注册顺序两段链守护', () => {
    it('两段确认到底：confirm times(2)，首段注册意向/次段信任提示语义，AutoRegisterAndOpen 调用', async () => {
      const { OpenInObsidian, AutoRegisterAndOpen } = await import('../../../wailsjs/go/main/App')
      const { ElMessageBox } = await import('element-plus')
      OpenInObsidian.mockResolvedValueOnce('not-registered')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm') // 第一段：注册意向
      ElMessageBox.confirm.mockResolvedValueOnce('confirm') // 第二段：信任提示
      AutoRegisterAndOpen.mockResolvedValueOnce('') // 组件 '' 分支 = 注册成功

      wrapper = createWrapper()
      await wrapper.vm.$.setupState.handleOpenObsidian('/path/vault')
      await flushPromises()

      expect(OpenInObsidian).toHaveBeenCalledWith('/path/vault')
      expect(ElMessageBox.confirm).toHaveBeenCalledTimes(2)
      // 第一段：未注册提示（distinguishCancelAndClose：cancel = 打开仓库管理器）
      const [msg1, title1, opts1] = ElMessageBox.confirm.mock.calls[0]
      expect(title1).toBe('提示')
      expect(msg1).toContain('未注册为 Obsidian vault')
      expect(opts1.confirmButtonText).toBe('自动注册并打开')
      expect(opts1.distinguishCancelAndClose).toBe(true)
      // 第二段：确认自动注册（信任提示/备份/运行中需关闭预告，HTML 渲染）
      const [msg2, title2, opts2] = ElMessageBox.confirm.mock.calls[1]
      expect(title2).toBe('确认自动注册')
      expect(msg2).toContain('Trust author and enable plugins')
      expect(opts2.dangerouslyUseHTMLString).toBe(true)
      expect(AutoRegisterAndOpen).toHaveBeenCalledWith('/path/vault')
      expect(ElMessage.success).toHaveBeenCalledWith('已注册为 Obsidian vault 并打开，首次会弹信任提示请确认')
    })

    it('第二段取消：两段 confirm 弹出后取消，AutoRegisterAndOpen 不调用（静默收口）', async () => {
      const { OpenInObsidian, AutoRegisterAndOpen, CopyObsidianVaultPath, OpenObsidianVaultManager } = await import('../../../wailsjs/go/main/App')
      const { ElMessageBox } = await import('element-plus')
      OpenInObsidian.mockResolvedValueOnce('not-registered')
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')

      wrapper = createWrapper()
      await wrapper.vm.$.setupState.handleOpenObsidian('/path/vault')
      await flushPromises()

      expect(ElMessageBox.confirm).toHaveBeenCalledTimes(2)
      expect(AutoRegisterAndOpen).not.toHaveBeenCalled()
      // 静默面：第二段「取消」被组件内层 catch 吞掉（异常不冒泡到外层 catch），不弹
      // 成功提示，也不误入外层 cancel 分支的仓库管理器链
      expect(ElMessage.success).not.toHaveBeenCalled()
      expect(CopyObsidianVaultPath).not.toHaveBeenCalled()
      expect(OpenObsidianVaultManager).not.toHaveBeenCalled()
    })
  })
})

// 聚合视图（虚拟工作区）：收藏目录平铺浏览——虚拟根构造、模式切换、状态隔离、
// 筛选作用域、展开回溯与失效兜底
describe('聚合视图（虚拟工作区）', () => {
  let wrapper

  // el-tree 桩补 store.root：saveCurrentState 遍历 root 收集展开路径、
  // waitTreeReady 判定 root.childNodes 非空（默认桩 root: null 会在遍历时抛错）
  const aggregatedTreeStub = {
    ...defaultStubs['el-tree'],
    data: () => ({ store: { nodesMap: {}, root: { expanded: false, childNodes: [{}] } } })
  }

  // 收藏 store 在 onMounted loadFavorites 落地后再注入，避免被空列表覆盖
  async function createAggWrapper(favorites = []) {
    // 同一用例内重复创建时先卸载，防 document 事件监听与实例泄漏
    if (wrapper) { wrapper.unmount(); wrapper = null }
    wrapper = createWrapper({ stubs: { 'el-tree': aggregatedTreeStub } })
    await flushPromises()
    useFavoritesStore().favorites = favorites
    return wrapper.vm.$.setupState
  }

  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
    // 防抖用例切过假定时器：统一恢复，避免影响后续 describe
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('工具栏切换按钮：点击进入聚合视图，treeKey 带模式后缀且文案切为激活态', async () => {
    const state = await createAggWrapper()
    const btn = wrapper.find('[data-action="toggle-aggregated-view"]')
    expect(btn.text()).toBe('目录树')

    await btn.trigger('click')
    await flushPromises()

    expect(state.aggregatedMode).toBe(true)
    expect(state.treeKey).toContain('_agg')
    expect(wrapper.find('[data-action="toggle-aggregated-view"]').text()).toBe('聚合视图')
  })

  it('再次切换回目录树：treeKey 恢复工作目录模式', async () => {
    const state = await createAggWrapper()
    await state.switchTreeMode(true, { restoreState: false })
    expect(state.treeKey).toContain('_agg')

    await state.switchTreeMode(false, { restoreState: false })
    expect(state.aggregatedMode).toBe(false)
    expect(state.treeKey).not.toContain('_agg')
  })

  it('isAggregatedMode / exitAggregatedMode 暴露：返回值随模式切换', async () => {
    const state = await createAggWrapper()
    // Home.vue 统一入口读取该返回值判定「选工作目录 = 退出聚合」
    expect(wrapper.vm.isAggregatedMode()).toBe(false)

    await state.switchTreeMode(true, { restoreState: false })
    expect(wrapper.vm.isAggregatedMode()).toBe(true)

    await wrapper.vm.exitAggregatedMode({ restoreState: false })
    expect(wrapper.vm.isAggregatedMode()).toBe(false)
  })

  it('聚合模式 level 0 调 GetAggregatedRootNodes（不调 GetFileTree），别名覆盖且失效节点按叶子', async () => {
    const { GetAggregatedRootNodes, GetFileTree } = await import('../../../wailsjs/go/main/App')
    GetAggregatedRootNodes.mockResolvedValueOnce([
      { id: '/repo/a', name: 'a', path: '/repo/a', type: 'directory', isGitRepo: true, hasRemote: true, hasChildren: true, isLeaf: false, missing: false },
      { id: '/repo/gone', name: 'gone', path: '/repo/gone', type: 'directory', hasChildren: false, isLeaf: false, missing: true }
    ])
    const state = await createAggWrapper([
      { path: '/repo/a', alias: '常用项目', group: '默认' },
      { path: '/repo/gone', alias: '', group: '默认' }
    ])
    state.aggregatedMode = true
    const resolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
    await flushPromises()

    expect(GetAggregatedRootNodes).toHaveBeenCalledWith(['/repo/a', '/repo/gone'])
    expect(GetFileTree).not.toHaveBeenCalled()
    const nodes = resolve.mock.calls[0][0]
    expect(nodes).toHaveLength(2)
    // 收藏 alias 覆盖后端路径末段显示名；空 alias 回落后端 name
    expect(nodes[0].name).toBe('常用项目')
    expect(nodes[1].name).toBe('gone')
    // 失效占位不可展开：isLeaf=true 防展开请求打空磁盘
    expect(nodes[0].isLeaf).toBe(false)
    expect(nodes[1].isLeaf).toBe(true)
    expect(nodes[1].missing).toBe(true)
    expect(state.aggregatedRootCount).toBe(2)
  })

  it('聚合绑定失败：提示错误并 resolve 空数组（不阻断树初始化）', async () => {
    const { GetAggregatedRootNodes } = await import('../../../wailsjs/go/main/App')
    GetAggregatedRootNodes.mockRejectedValueOnce(new Error('IPC 异常'))
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    const resolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('加载收藏目录失败'))
    expect(resolve).toHaveBeenCalledWith([])
    expect(state.aggregatedRootCount).toBe(0)
    // 失败态与空收藏区分：不得显示「暂无目录收藏」误导用户以为收藏丢失
    await wrapper.vm.$nextTick()
    expect(state.aggregatedRootError).toContain('加载收藏目录失败')
    expect(wrapper.find('.el-empty').text()).toContain('加载收藏目录失败')
    expect(wrapper.find('.el-empty').text()).not.toContain('暂无目录收藏')
  })

  it('无目录收藏：聚合根层返回空数组后显示空态引导（走真实加载链路）', async () => {
    const { GetAggregatedRootNodes } = await import('../../../wailsjs/go/main/App')
    GetAggregatedRootNodes.mockResolvedValueOnce([])
    const state = await createAggWrapper([])
    state.aggregatedMode = true
    const resolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(resolve).toHaveBeenCalledWith([])
    expect(state.aggregatedRootLoading).toBe(false)
    expect(state.aggregatedRootError).toBe('')
    expect(state.aggregatedRootCount).toBe(0)
    expect(wrapper.find('.el-empty').text()).toContain('暂无目录收藏')
  })

  it('失效收藏节点点击：提示失效且不选中/不 emit select', async () => {
    const state = await createAggWrapper()
    state.onNodeClick(
      { path: '/repo/gone', name: 'gone', type: 'directory', missing: true },
      { expanded: false }
    )

    expect(ElMessage.info).toHaveBeenCalledWith(expect.stringContaining('收藏项已失效或不可访问，可右键移除收藏'))
    expect(wrapper.emitted('select')).toBeUndefined()
    expect(state.currentSelectedPath).toBe('')
  })

  it('失效收藏节点 tooltip：统一为「收藏项已失效或不可访问」，不断言已移动也不暗示目录', async () => {
    // 用渲染 default slot 的 el-tree 桩直接验证节点模板绑定（不涉及展开行为）
    const missingNodeTreeStub = {
      template: `<div class="el-tree">
        <slot :node="{ expanded: false }"
              :data="{ path: '/repo/gone', name: 'gone', type: 'directory', missing: true }" />
      </div>`,
      data: () => ({ store: { nodesMap: {}, root: { expanded: false, childNodes: [{}] } } })
    }
    wrapper = createWrapper({ stubs: { 'el-tree': missingNodeTreeStub } })
    await flushPromises()

    const title = wrapper.find('.custom-tree-node').attributes('title')
    expect(title).toBe('收藏项已失效或不可访问，可右键移除收藏')
    expect(title).not.toContain('已移动')
    expect(title).not.toContain('目录')
  })

  it('失效收藏节点右键菜单：仅保留移除收藏与复制路径（其余文件操作无意义）', async () => {
    const state = await createAggWrapper()
    state.aggregatedMode = true
    Object.assign(state.contextMenu, {
      visible: true,
      x: 0,
      y: 0,
      isBlankArea: false,
      data: { path: '/repo/gone', name: 'gone', type: 'directory', missing: true }
    })
    await wrapper.vm.$nextTick()

    const menuText = wrapper.find('.context-menu').text()
    expect(menuText).toContain('移除收藏')
    expect(menuText).toContain('复制路径')
    expect(menuText).not.toContain('新建文件')
    expect(menuText).not.toContain('设为工作目录')
  })

  it('目录右键菜单按模式分流：聚合视图「设为工作目录」，普通树「添加为工作目录」', async () => {
    const state = await createAggWrapper()
    Object.assign(state.contextMenu, {
      visible: true,
      x: 0,
      y: 0,
      isBlankArea: false,
      data: { path: '/repo/a', name: '常用项目', type: 'directory' }
    })
    state.aggregatedMode = true
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.context-menu').text()).toContain('设为工作目录')
    expect(wrapper.find('.context-menu').text()).not.toContain('添加为工作目录')

    state.aggregatedMode = false
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.context-menu').text()).toContain('添加为工作目录')
    expect(wrapper.find('.context-menu').text()).not.toContain('设为工作目录')
  })

  it('树状态持久化 key 隔离：聚合模式写虚拟 key，普通模式写目录路径 key', async () => {
    const state = await createAggWrapper()
    state.aggregatedMode = true
    state.saveCurrentState('/path/a')
    expect(localStorage.getItem('treeState:__aggregated__')).toBeTruthy()
    expect(localStorage.getItem('treeState:/path/a')).toBeNull()

    state.aggregatedMode = false
    state.saveCurrentState('/path/a')
    expect(localStorage.getItem('treeState:/path/a')).toBeTruthy()
    // 普通树写入不覆盖聚合视图已保存状态（两模式互不污染）
    expect(localStorage.getItem('treeState:__aggregated__')).toBeTruthy()
  })

  it('聚合筛选作用域：未点击目录不拉取，点击目录后作用于该目录', async () => {
    const { GetFileTree } = await import('../../../wailsjs/go/main/App')
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    // 模式切换触发 treeKey watch（exitFilterMode 清空关键词），先等 watch 落地再输入
    await wrapper.vm.$nextTick()
    state.treeFilterKeyword = 'index'
    await wrapper.vm.$nextTick()
    // 无作用域时覆盖层给出引导，不显示「工作目录内 0 命中」误导文案
    expect(wrapper.find('.tree-filter-summary').text()).toContain('请先点击一个收藏目录')

    // 收藏目录间无公共祖先：未点击就无作用域可回退，直接不拉取
    await state.runFilter()
    expect(GetFileTree).not.toHaveBeenCalled()

    GetFileTree.mockResolvedValueOnce([
      { name: 'index.js', path: '/repo/a/index.js', type: 'file' }
    ])
    state.lastClickedNodeData = { path: '/repo/a', name: 'a', type: 'directory' }
    state.treeFilterKeyword = 'index'
    await state.runFilter()
    expect(GetFileTree).toHaveBeenCalledWith('/repo/a')
    expect(state.filteredNodes).toHaveLength(1)
  })

  it('findExpandedAncestor：聚合模式回溯到收藏根，收藏范围外返回 null', async () => {
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    const rootNode = { expanded: false }
    const store = { nodesMap: { '/repo/a': rootNode } }

    // 收藏根在 nodesMap 中：即使未展开也作为刷新目标（expand 会加载最新子节点）
    expect(state.findExpandedAncestor('/repo/a/src/deep', store)).toBe(rootNode)
    // 目标不在任何收藏范围内：无法确定回溯根
    expect(state.findExpandedAncestor('/other/x', store)).toBeNull()
  })

  it('聚合视图目录右键「设为工作目录」：emit set-as-work-dir 携路径与显示名', async () => {
    const state = await createAggWrapper()
    state.aggregatedMode = true
    state.contextMenu.data = { path: '/repo/a', name: '常用项目', type: 'directory' }

    state.onMenuCommand('setAsWorkDir')

    expect(wrapper.emitted('set-as-work-dir')).toEqual([[{ path: '/repo/a', name: '常用项目' }]])
    expect(wrapper.emitted('add-work-dir')).toBeUndefined()
  })

  it('嵌套收藏去重（父先子后）：祖先已是收藏时不重复列为根', async () => {
    const { GetAggregatedRootNodes } = await import('../../../wailsjs/go/main/App')
    GetAggregatedRootNodes.mockResolvedValueOnce([
      { id: '/repo/a', name: 'a', path: '/repo/a', type: 'directory', hasChildren: true, isLeaf: false, missing: false }
    ])
    const state = await createAggWrapper([
      { path: '/repo/a', alias: '', group: '默认' },
      { path: '/repo/a/sub', alias: '', group: '默认' },
      // 分隔符/大小写差异也应识别为同一祖先子树
      { path: '/REPO/A/other', alias: '', group: '默认' }
    ])
    state.aggregatedMode = true
    const resolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
    await flushPromises()

    expect(GetAggregatedRootNodes).toHaveBeenCalledWith(['/repo/a'])
  })

  it('嵌套收藏去重（子先父后）：去重与收藏顺序无关', async () => {
    const { GetAggregatedRootNodes } = await import('../../../wailsjs/go/main/App')
    GetAggregatedRootNodes.mockResolvedValueOnce([
      { id: '/repo/a', name: 'a', path: '/repo/a', type: 'directory', hasChildren: true, isLeaf: false, missing: false }
    ])
    const state = await createAggWrapper([
      { path: '/repo/a/sub', alias: '', group: '默认' },
      { path: '/repo/a', alias: '', group: '默认' }
    ])
    state.aggregatedMode = true
    const resolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
    await flushPromises()

    // 单遍扫描会漏掉「先子后父」，此处锁定顺序无关语义
    expect(GetAggregatedRootNodes).toHaveBeenCalledWith(['/repo/a'])
  })

  it('refreshAfterFileOp：聚合视图根级收藏重命名/删除后改为整层重建', async () => {
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    await wrapper.vm.$nextTick()
    const before = state.refreshCounter

    // 父目录 /repo 不属于任何收藏根 = 被操作节点是收藏根自身
    await state.refreshAfterFileOp('/repo')
    await flushPromises()

    expect(state.refreshCounter).toBe(before + 1)
  })

  it('locateNode 命中失效收藏根：提示失效且不选中（防快捷键误操作）', async () => {
    const state = await createAggWrapper([{ path: '/repo/gone', alias: '', group: '默认' }])
    state.aggregatedMode = true
    // 等 treeKey watch 重置就绪信号后再加载，保证 locateNode 等的是本次加载的 promise
    await wrapper.vm.$nextTick()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, vi.fn())
    await flushPromises()

    const setCurrentKey = vi.fn()
    state.fileTreeRef = {
      getNode: vi.fn(() => ({ data: { path: '/repo/gone', missing: true } })),
      setCurrentKey
    }
    await state.locateNode('/repo/gone')

    expect(ElMessage.info).toHaveBeenCalledWith(expect.stringContaining('收藏项已失效或不可访问，可右键移除收藏'))
    expect(setCurrentKey).not.toHaveBeenCalled()
  })

  it('快捷键重命名/删除：当前节点为失效收藏时不弹对话框', async () => {
    const { ElMessageBox } = await import('element-plus')
    const state = await createAggWrapper()
    state.fileTreeRef = {
      getCurrentNode: () => ({ data: { path: '/repo/gone', name: 'gone', type: 'directory', missing: true } })
    }

    state.triggerRenameCurrent()
    state.triggerDeleteCurrent()

    expect(state.renameDialogVisible).toBe(false)
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
  })

  it('聚合视图无作用域时树保持可见：引导可执行，筛选不成死路', async () => {
    const { GetAggregatedRootNodes } = await import('../../../wailsjs/go/main/App')
    GetAggregatedRootNodes.mockResolvedValueOnce([
      { id: '/repo/a', name: 'a', path: '/repo/a', type: 'directory', hasChildren: true, isLeaf: false, missing: false }
    ])
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    await wrapper.vm.$nextTick()
    // 先让根层加载完成（count=1），否则树因「无收藏空态」隐藏，与本用例验证点无关
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, vi.fn())
    await flushPromises()
    state.treeFilterKeyword = 'index'
    await wrapper.vm.$nextTick()

    expect(wrapper.find('.tree-filter-summary').text()).toContain('请先点击一个收藏目录')
    expect(wrapper.find('.el-tree').attributes('style') || '').not.toContain('display: none')
  })

  it('聚合模式「复制相对路径」/「文件引用路径」以所属收藏根为基准（普通树仍以工作目录为基准）', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true

    // 收藏根子节点：相对路径相对收藏根（非工作目录）
    state.contextMenu.data = { path: '/repo/a/src/index.js', name: 'index.js', type: 'file' }
    state.onMenuCommand('copyRelativePath')
    await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('src/index.js')

    // 文件引用路径与复制相对路径同基准，避免两者口径分裂
    state.contextMenu.data = { path: '/repo/a/src/index.js', name: 'index.js', type: 'file' }
    state.onMenuCommand('copyRefPath')
    await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('@src/index.js')

    // 收藏根本身：相对路径为 '.'
    state.contextMenu.data = { path: '/repo/a', name: 'a', type: 'directory' }
    state.onMenuCommand('copyRelativePath')
    await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('.')

    // 回归：普通树保持原语义（相对当前工作目录 /path/a）
    state.aggregatedMode = false
    state.contextMenu.data = { path: '/path/a/src/index.js', name: 'index.js', type: 'file' }
    state.onMenuCommand('copyRelativePath')
    await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('src/index.js')
  })

  it('聚合模式空白区右键：隐藏新建/仓库筛选等项，保留刷新；粘贴随最后点击目录出现', async () => {
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    // 聚合空白区不依赖工作目录：清空选中目录后仍应弹出菜单
    useDirectoryStore().selectedDirectoryId = null
    state.lastClickedNodeData = null
    await wrapper.vm.$nextTick()

    const event = { stopPropagation: vi.fn(), clientX: 10, clientY: 10 }
    state.onBlankAreaContextMenu(event)
    await wrapper.vm.$nextTick()

    const menuText = wrapper.find('.context-menu').text()
    expect(menuText).toContain('刷新')
    expect(menuText).not.toContain('新建文件')
    expect(menuText).not.toContain('新建文件夹')
    expect(menuText).not.toContain('仓库筛选器')
    expect(menuText).not.toContain('跳转仓库统计')
    // 无最后点击目录 = 无粘贴落点，不显示粘贴项
    expect(menuText).not.toContain('粘贴')
    expect(wrapper.find('[data-action="aggregated-blank-refresh"]').exists()).toBe(true)

    // 最后点击目录节点后粘贴项出现，且菜单 data 指向该目录（非工作目录伪装的节点）
    state.lastClickedNodeData = { path: '/repo/a', name: 'a', type: 'directory' }
    state.onBlankAreaContextMenu(event)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.context-menu').text()).toContain('粘贴')
    expect(state.contextMenu.data).toEqual({ path: '/repo/a', name: 'a', type: 'directory' })

    // 点击刷新 = 重建根层（虚拟根无节点可定位，refreshNode 会早退）
    const before = state.refreshCounter
    await wrapper.find('[data-action="aggregated-blank-refresh"]').trigger('click')
    await flushPromises()
    expect(state.refreshCounter).toBe(before + 1)
  })

  it('聚合空白区「刷新」：先清全部树/徽标缓存，再整层重建并重新查询根层', async () => {
    const { ClearAllFileTreeCache, GetAggregatedRootNodes } = await import('../../../wailsjs/go/main/App')
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    await wrapper.vm.$nextTick()

    // 先完成一次根层加载，确立基线调用次数（桩不自动懒加载，需手动模拟）
    const initialResolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, initialResolve)
    await flushPromises()
    const rootsBefore = GetAggregatedRootNodes.mock.calls.length
    const clearBefore = ClearAllFileTreeCache.mock.calls.length
    const counterBefore = state.refreshCounter

    state.lastClickedNodeData = null
    state.onBlankAreaContextMenu({ stopPropagation: vi.fn(), clientX: 10, clientY: 10 })
    await wrapper.vm.$nextTick()
    await wrapper.find('[data-action="aggregated-blank-refresh"]').trigger('click')
    await flushPromises()

    // 空白区刷新 = 整树刷新：清全部缓存（含根行 git 徽标缓存），再推进代次重建
    expect(ClearAllFileTreeCache.mock.calls.length).toBe(clearBefore + 1)
    expect(state.refreshCounter).toBe(counterBefore + 1)

    // 重建后的 el-tree 重新懒加载根层：后端重新探测 isGitRepo/hasRemote
    const freshResolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, freshResolve)
    await flushPromises()
    expect(GetAggregatedRootNodes.mock.calls.length).toBe(rootsBefore + 1)
    expect(freshResolve).toHaveBeenCalled()
  })

  it('收藏变化（外部入口）去抖后重建聚合根层；非聚合模式不触发', async () => {
    vi.useFakeTimers()
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    await wrapper.vm.$nextTick()

    // 非聚合模式：收藏变化直接忽略
    state.aggregatedMode = false
    const beforeAgg = state.refreshCounter
    useFavoritesStore().favorites = [{ path: '/repo/a', alias: '别名-A', group: '默认' }]
    await wrapper.vm.$nextTick()
    vi.advanceTimersByTime(200)
    await flushPromises()
    expect(state.refreshCounter).toBe(beforeAgg)

    // 聚合模式：100ms 去抖后重建（配置导入/命令面板取消收藏等外部入口）
    state.aggregatedMode = true
    const before = state.refreshCounter
    useFavoritesStore().favorites = [
      { path: '/repo/a', alias: '别名-A', group: '默认' },
      { path: '/repo/b', alias: '', group: '默认' }
    ]
    await wrapper.vm.$nextTick()
    vi.advanceTimersByTime(99)
    expect(state.refreshCounter).toBe(before)
    vi.advanceTimersByTime(1)
    await flushPromises()
    expect(state.refreshCounter).toBe(before + 1)
  })

  it('收藏变化去抖期间的代次守卫：调度后切换模式则丢弃本次重建', async () => {
    vi.useFakeTimers()
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    await wrapper.vm.$nextTick()
    const before = state.refreshCounter

    useFavoritesStore().favorites = [{ path: '/repo/a', alias: '新别名', group: '默认' }]
    await wrapper.vm.$nextTick()
    // 模式切换推进 modeSwitchSeq：此前调度的重建代次过期
    await state.switchTreeMode(false, { restoreState: false })
    vi.advanceTimersByTime(200)
    await flushPromises()

    expect(state.refreshCounter).toBe(before)
  })

  it('刷新收藏根本身：先清徽标缓存再整层重建，根行徽标随重新探测更新；子目录刷新不重建', async () => {
    const { GetAggregatedRootNodes, InvalidateFileTreeCache } = await import('../../../wailsjs/go/main/App')
    // 两次根层查询：刷新前旧快照（非仓库）、刷新后重新探测（模拟 git init + git remote add）
    const rootNode = (isGitRepo, hasRemote) => ({
      id: '/repo/a',
      name: 'a',
      path: '/repo/a',
      type: 'directory',
      isGitRepo,
      hasRemote,
      hasChildren: true,
      isLeaf: false,
      missing: false
    })
    GetAggregatedRootNodes
      .mockResolvedValueOnce([rootNode(false, false)])
      .mockResolvedValueOnce([rootNode(true, true)])
    const state = await createAggWrapper([{ path: '/repo/a', alias: '', group: '默认' }])
    state.aggregatedMode = true
    await wrapper.vm.$nextTick()
    // 刷新前根层快照：徽标为旧结论
    const staleResolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, staleResolve)
    await flushPromises()
    expect(staleResolve.mock.calls[0][0][0]).toMatchObject({ isGitRepo: false, hasRemote: false })

    const before = state.refreshCounter
    const invalidateBefore = InvalidateFileTreeCache.mock.calls.length

    await state.refreshNode('/repo/a')
    await flushPromises()
    // 必须清后端 git 徽标缓存，否则整层重建仍命中原有的 isGitRepo/hasRemote 旧结论
    expect(InvalidateFileTreeCache.mock.calls.length).toBe(invalidateBefore + 1)
    expect(InvalidateFileTreeCache).toHaveBeenLastCalledWith('/repo/a')
    // 收藏根走整层重建：树 key 变化触发根层重新加载
    expect(state.refreshCounter).toBe(before + 1)

    // 重建后的根层查询返回新结论：根行徽标即时更新（⑬ 验收）
    const freshResolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, freshResolve)
    await flushPromises()
    expect(freshResolve.mock.calls[0][0][0]).toMatchObject({ isGitRepo: true, hasRemote: true })

    const callsAfterRoot = GetAggregatedRootNodes.mock.calls.length
    const invalidateAfterRoot = InvalidateFileTreeCache.mock.calls.length
    await state.refreshNode('/repo/a/src')
    await flushPromises()
    // 子目录刷新走既有子层链路（桩下 nodesMap 未命中静默早退），不触发整层重建
    expect(state.refreshCounter).toBe(before + 1)
    expect(GetAggregatedRootNodes.mock.calls.length).toBe(callsAfterRoot)
    expect(InvalidateFileTreeCache.mock.calls.length).toBe(invalidateAfterRoot)
  })

  it('收藏别名修改后重建根层：显示名取新别名', async () => {
    vi.useFakeTimers()
    const { GetAggregatedRootNodes } = await import('../../../wailsjs/go/main/App')
    // Once 而非 mockResolvedValue：本用例只手动触发一次根层加载，回退后保留模块 mock 的
    // 默认实现（[]），避免 mockReset 把后续用例的根层查询打成 undefined
    GetAggregatedRootNodes.mockResolvedValueOnce([
      { id: '/repo/a', name: 'a', path: '/repo/a', type: 'directory', hasChildren: true, isLeaf: false, missing: false }
    ])
    const state = await createAggWrapper([{ path: '/repo/a', alias: '旧别名', group: '默认' }])
    state.aggregatedMode = true
    await wrapper.vm.$nextTick()

    // store.updateAlias 成功后会 loadFavorites，列表别名变化触发去抖重建
    const before = state.refreshCounter
    useFavoritesStore().favorites = [{ path: '/repo/a', alias: '新别名', group: '默认' }]
    await wrapper.vm.$nextTick()
    vi.advanceTimersByTime(100)
    await flushPromises()
    // toBe +1 而非仅断言显示名：否则手工 loadTreeNode 也能拿到新别名，watch 失效时用例假绿
    expect(state.refreshCounter).toBe(before + 1)

    const resolve = vi.fn()
    await wrapper.vm.loadTreeNode({ level: 0, data: null }, resolve)
    await flushPromises()
    expect(resolve.mock.calls[0][0][0].name).toBe('新别名')
  })

  it('聚合模式 treeKey 不绑定工作目录：切换目录不重建聚合树', async () => {
    const state = await createAggWrapper()
    state.aggregatedMode = true
    await wrapper.vm.$nextTick()
    const keyBefore = state.treeKey

    useDirectoryStore().selectedDirectoryId = 'dir-2'
    await wrapper.vm.$nextTick()

    expect(state.treeKey).toBe(keyBefore)
  })
})

// 浏览器通道入口隐藏：聚合根查询在 RPC 面被 RPCHiddenMethods 屏蔽，
// 入口按 SettingsPanel/useIPBans 隐私先例「启动期隐藏而非点击报错」
describe('FileTreePanel.vue - 浏览器通道入口隐藏', () => {
  it('isBrowserChannel() 为 true 时不渲染聚合视图切换按钮', async () => {
    vi.resetModules()
    vi.doMock('../../transport', () => ({ isBrowserChannel: () => true }))
    let wrapperLocal = null
    try {
      const { default: BrowserFileTreePanel } = await import('../FileTreePanel.vue')
      const pinia = createPinia()
      setActivePinia(pinia)
      wrapperLocal = mount(BrowserFileTreePanel, {
        global: { plugins: [pinia], stubs: defaultStubs }
      })
      await flushPromises()

      expect(wrapperLocal.find('[data-action="toggle-aggregated-view"]').exists()).toBe(false)
      expect(wrapperLocal.find('.tree-toolbar').exists()).toBe(true)
    } finally {
      // 断言失败时同样卸载，避免实例与监听残留
      if (wrapperLocal) wrapperLocal.unmount()
      vi.doUnmock('../../transport')
      vi.resetModules()
    }
  })
})
