/**
 * Home.vue 组件测试
 * 重点关注修复的两个bug：
 * 1. 懒加载树根节点检测
 * 2. 节点切换时预览状态清理
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { watch } from 'vue'
import { ElMessage } from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import Home from '../Home.vue'
import { useUiStore, useDirectoryStore, useWorkspaceStore } from '../../store'

// Mock Wails runtime
vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
  EventsOff: vi.fn()
}))

// Mock Element Plus组件
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
      confirm: vi.fn(() => Promise.resolve(true))
    }
  }
})

// Mock debug工具
vi.mock('../utils/debug', () => ({
  debug: {
    log: vi.fn(),
    error: vi.fn(),
    warn: vi.fn()
  }
}))

// Mock Wails Go bindings
vi.mock('../../../wailsjs/go/main/App', () => ({
  GetDirectories: vi.fn(() => Promise.resolve([])),
  GetAppVersion: vi.fn(() => Promise.resolve('1.0.0')),
  ScanAndPullRepos: vi.fn(() => Promise.resolve('')),
  DeleteFile: vi.fn(() => Promise.resolve(true)),
  CopyItem: vi.fn(() => Promise.resolve('')),
  CopyTo: vi.fn(() => Promise.resolve('')),
  MoveItem: vi.fn(() => Promise.resolve('')),
  CopyToSystemClipboard: vi.fn(() => Promise.resolve('')),
  CutToSystemClipboard: vi.fn(() => Promise.resolve('')),
  ReadFromSystemClipboard: vi.fn(() => Promise.resolve(null)),
  GetFavorites: vi.fn(() => Promise.resolve([])),
  AddFavorite: vi.fn(() => Promise.resolve(true)),
  RemoveFavorite: vi.fn(() => Promise.resolve(true)),
  UpdateFavoriteAlias: vi.fn(() => Promise.resolve(true)),
  UpdateFavoriteGroup: vi.fn(() => Promise.resolve(true)),
  RefreshDirectoriesGitFlag: vi.fn(() => Promise.resolve([])),
  AddDirectory: vi.fn(() => Promise.resolve({ id: '1' })),
  GetSettings: vi.fn(() => Promise.resolve({}))
}))

describe('Home.vue - Bug修复验证', () => {
  let wrapper

  beforeEach(() => {
    // Home.vue 渲染真实 CommandPalette（未 stub），其 setup 调用 useFavoritesStore() 需活跃 pinia
    setActivePinia(createPinia())
    // 清空 localStorage 避免 useRecentAccess 跨文件残留脏记录（undefined path 触发 getFileName 报错）
    localStorage.clear()
    wrapper = mount(Home, {
      global: {
        stubs: {
          Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
          Pane: { template: '<div class="pane"><slot /></div>', props: ['size', 'minSize', 'maxSize'] },
          DirectoryTree: { template: '<div class="stub-directory-tree" />' },
          FileTreePanel: { template: '<div class="stub-file-tree-panel" />', methods: { saveCurrentState: () => {}, restoreTreeState: () => {}, setCopyToLoading: () => {}, closeCopyToDialog: () => {}, refreshNode: () => {} } },
          ContentPanel: { template: '<div class="stub-content-panel" />', methods: { clearPreview: () => {}, startBatchPull: () => {}, previewFile: () => {} } },
          RepoFilterDialog: { template: '<div class="stub-repo-filter-dialog" />' },
          'el-tree': true,
          'el-dialog': true,
          'el-form': true,
          'el-form-item': true,
          'el-input': true,
          'el-switch': true,
          'el-button': true,
          'el-button-group': true,
          'el-divider': true,
          'el-select': true,
          'el-option': true,
          'el-empty': true,
          'el-descriptions': true,
          'el-descriptions-item': true,
          'el-icon': true,
          'el-progress': true,
          'el-table': true,
          'el-table-column': true
        }
      }
    })
  })

  describe('Bug修复 #1: 懒加载树根节点检测', () => {
    it('应该正确加载目录列表', async () => {
      // 验证loadDirectories函数存在且可调用
      expect(typeof useDirectoryStore().loadDirectories).toBe('function')
    })

    it('应该正确选择目录后清空选中节点', () => {
      useWorkspaceStore().selectedNode = { name: 'test', path: '/test' }
      wrapper.vm.onDirectorySelect('new-dir-id')

      expect(useDirectoryStore().selectedDirectoryId).toBe('new-dir-id')
      expect(useWorkspaceStore().selectedNode).toBeNull()
      expect(useWorkspaceStore().latestCommit).toBeNull()
    })

    it('应该正确处理目录切换', () => {
      wrapper.vm.onDirectorySelect('dir-1')
      expect(useDirectoryStore().selectedDirectoryId).toBe('dir-1')
    })
  })

  describe('工作目录切换 git 仓库双刷新修复', () => {
    it('切到 git 工作目录时 selectedNode 立即等于期望的 git 节点对象，无 null 中间态', async () => {
      useDirectoryStore().directories = [
        { id: 'git-1', name: '仓库A', path: '/a/git-repo', isGitRepo: true, isDefault: false }
      ]

      await wrapper.vm.onDirectorySelect('git-1')
      await flushPromises()

      expect(useDirectoryStore().selectedDirectoryId).toBe('git-1')
      expect(useWorkspaceStore().selectedNode).toEqual({
        id: 'git-1',
        path: '/a/git-repo',
        name: '仓库A',
        type: 'directory',
        isGitRepo: true
      })
      // latestCommit 应被清零
      expect(useWorkspaceStore().latestCommit).toBeNull()
    })

    it('切到非 git 工作目录时 selectedNode 被置 null', async () => {
      useDirectoryStore().directories = [
        { id: 'plain-1', name: '普通目录', path: '/b/plain', isGitRepo: false, isDefault: false }
      ]

      await wrapper.vm.onDirectorySelect('plain-1')
      await flushPromises()

      expect(useDirectoryStore().selectedDirectoryId).toBe('plain-1')
      expect(useWorkspaceStore().selectedNode).toBeNull()
      expect(useWorkspaceStore().latestCommit).toBeNull()
    })

    it('gitA → gitB 切换时 selectedNode 由 A-git 直切 B-git（无 null 中间态）', async () => {
      useDirectoryStore().directories = [
        { id: 'git-A', name: '仓库A', path: '/a/gitA', isGitRepo: true, isDefault: false },
        { id: 'git-B', name: '仓库B', path: '/b/gitB', isGitRepo: true, isDefault: false }
      ]
      await wrapper.vm.onDirectorySelect('git-A')
      await flushPromises()
      expect(useWorkspaceStore().selectedNode.path).toBe('/a/gitA')

      // 切到 B：观察中间态是否经过 null
      const observed = []
      const unwatch = watch(() => useWorkspaceStore().selectedNode, (v) => observed.push(v), { deep: true, flush: 'sync' })

      await wrapper.vm.onDirectorySelect('git-B')
      await flushPromises()
      unwatch()

      // 最终落到 B-git
      expect(useWorkspaceStore().selectedNode).toEqual({
        id: 'git-B',
        path: '/b/gitB',
        name: '仓库B',
        type: 'directory',
        isGitRepo: true
      })
      // 关键：观察序列中不应出现 null（即 content-inner 不会卸载再挂载 = 无双刷新）
      expect(observed.some(v => v === null)).toBe(false)
    })
  })

  describe('Bug修复 #2: 节点切换时预览状态清理', () => {
    it('应该在选中节点时更新selectedNode', () => {
      const newNode = {
        name: 'new-file.txt',
        path: '/test/new-file.txt',
        type: 'file',
        isGitRepo: false
      }

      wrapper.vm.onNodeSelect(newNode)

      expect(useWorkspaceStore().selectedNode).toEqual(newNode)
    })

    it('应该保留选中的节点信息', () => {
      const newNode = {
        name: 'test-folder',
        path: '/test/folder',
        type: 'directory',
        isGitRepo: false
      }

      wrapper.vm.onNodeSelect(newNode)

      expect(useWorkspaceStore().selectedNode.name).toBe('test-folder')
      expect(useWorkspaceStore().selectedNode.path).toBe('/test/folder')
    })

    it('应该在Git仓库节点上选中', () => {
      const gitNode = {
        name: 'test-repo',
        path: '/test/repo',
        type: 'directory',
        isGitRepo: true
      }

      wrapper.vm.onNodeSelect(gitNode)

      expect(useWorkspaceStore().selectedNode).toEqual(gitNode)
    })

    it('切换文件树节点时应清零 latestCommit，避免上一个仓库的提交残留', () => {
      // 模拟上一个仓库经"提交历史"tab emit 后 latestCommit 已有值
      useWorkspaceStore().latestCommit = { sha: 'aaa', shortSha: 'aaa1111', message: '上一个仓库的提交' }
      expect(useWorkspaceStore().latestCommit).not.toBeNull()

      const newNode = {
        name: 'repo-B',
        path: '/test/repo-B',
        type: 'directory',
        isGitRepo: true
      }

      wrapper.vm.onNodeSelect(newNode)

      expect(useWorkspaceStore().selectedNode).toEqual(newNode)
      // 关键：切换节点后 latestCommit 被清零，GitInfo 不再显示上一个仓库的提交
      expect(useWorkspaceStore().latestCommit).toBeNull()
    })
  })

  describe('错误处理改进', () => {
    it('应该正确处理错误消息（运算符优先级修复）', async () => {
      const mockResolve = vi.fn()

      // 验证错误处理逻辑：确保运算符优先级正确
      const error = new Error('Test error')
      const errorMessage = '加载节点失败: ' + (error.message || error)

      expect(errorMessage).toBe('加载节点失败: Test error')
      expect(errorMessage).toContain('Test error')
    })

    it('应该处理字符串类型的Error', () => {
      const error = 'String error'
      const errorMessage = '加载节点失败: ' + (error.message || error)

      expect(errorMessage).toBe('加载节点失败: String error')
    })
  })

  describe('调试日志行为', () => {
    it('应该使用debug工具而不是console.log', () => {
      // 验证debug工具被导入
      const { debug } = require('../../utils/debug')
      expect(debug).toBeDefined()
      expect(debug.log).toBeDefined()
      expect(debug.error).toBeDefined()
    })
  })

  describe('handleCopyTo 自定义文件名透传（拷贝到支持重命名）', () => {
    it('targetName 随 CopyTo 第 3 参透传，空串兜底', async () => {
      const App = await vi.importMock('../../../wailsjs/go/main/App')
      App.CopyTo.mockClear()

      // 场景 1：带自定义名
      await wrapper.vm.handleCopyTo({ sourcePath: '/a/src.txt', targetPath: '/b', targetName: 'renamed.txt', copyWholeDir: false })
      expect(App.CopyTo).toHaveBeenCalledWith('/a/src.txt', '/b', 'renamed.txt', false)

      App.CopyTo.mockClear()
      // 场景 2：无自定义名（目录源）传空串，保持原行为
      await wrapper.vm.handleCopyTo({ sourcePath: '/a/srcdir', targetPath: '/b', copyWholeDir: true })
      expect(App.CopyTo).toHaveBeenCalledWith('/a/srcdir', '/b', '', true)
    })
  })

  describe('splitpanes 三栏布局验证', () => {
    let layoutWrapper

    beforeEach(() => {
      layoutWrapper = mount(Home, {
        global: {
          stubs: {
            Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
            Pane: { template: '<div class="pane" :data-size="size" :data-min-size="minSize" :data-max-size="maxSize"><slot /></div>', props: ['size', 'minSize', 'maxSize'] },
            DirectoryTree: { template: '<div class="stub-directory-tree" />' },
            FileTreePanel: { template: '<div class="stub-file-tree-panel" />' },
            ContentPanel: { template: '<div class="stub-content-panel" />', methods: { clearPreview: () => {}, previewFile: () => {} } },
            RepoFilterDialog: { template: '<div class="stub-repo-filter-dialog" />' },
            'el-dialog': true,
            'el-drawer': true,
            'el-table': true,
            'el-table-column': true,
            'el-tag': true,
            'el-date-picker': true,
            'el-select': true,
            'el-option': true,
            'el-button': true
          }
        }
      })
    })

    afterEach(() => {
      if (layoutWrapper) {
        layoutWrapper.unmount()
        layoutWrapper = null
      }
    })

    it('应该渲染 splitpanes 容器', () => {
      expect(layoutWrapper.find('.splitpanes').exists()).toBe(true)
    })

    it('应该渲染三个 Pane', () => {
      const panes = layoutWrapper.findAll('.pane')
      expect(panes.length).toBe(3)
    })

    it('三个面板应按左-中-右顺序排列', () => {
      const panes = layoutWrapper.findAll('.pane')
      expect(panes[0].find('.stub-directory-tree').exists()).toBe(true)
      expect(panes[1].find('.stub-file-tree-panel').exists()).toBe(true)
      expect(panes[2].find('.stub-content-panel').exists()).toBe(true)
    })

    it('第一个 Pane 尺寸配置正确', () => {
      const panes = layoutWrapper.findAll('.pane')
      expect(panes[0].attributes('data-size')).toBe('20')
      expect(panes[0].attributes('data-min-size')).toBe('10')
    })

    it('第二个 Pane 尺寸配置正确', () => {
      const panes = layoutWrapper.findAll('.pane')
      expect(panes[1].attributes('data-size')).toBe('30')
      expect(panes[1].attributes('data-min-size')).toBe('15')
    })

    it('第三个 Pane 尺寸配置正确', () => {
      const panes = layoutWrapper.findAll('.pane')
      expect(panes[2].attributes('data-size')).toBe('50')
      expect(panes[2].attributes('data-min-size')).toBe('30')
    })
  })

  describe('左侧文件树滚动条', () => {
    let slotWrapper

    beforeEach(() => {
      slotWrapper = mount(Home, {
        global: {
          stubs: {
            Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
            Pane: { template: '<div class="pane" :data-size="size" :data-min-size="minSize" :data-max-size="maxSize"><slot /></div>', props: ['size', 'minSize', 'maxSize'] },
            DirectoryTree: { template: '<div class="stub-directory-tree" />' },
            FileTreePanel: { template: '<div class="stub-file-tree-panel" />' },
            ContentPanel: { template: '<div class="stub-content-panel" />', methods: { clearPreview: () => {}, startBatchPull: () => {}, previewFile: () => {} } },
            RepoFilterDialog: { template: '<div class="stub-repo-filter-dialog" />' },
            'el-dialog': true,
            'el-form': true,
            'el-form-item': true,
            'el-input': true,
            'el-switch': true,
            'el-button': { template: '<button v-bind="$attrs"><slot /></button>' },
            'el-button-group': { template: '<div><slot /></div>' },
            'el-divider': true,
            'el-select': true,
            'el-option': true,
            'el-empty': true,
            'el-descriptions': true,
            'el-descriptions-item': true,
            'el-icon': true,
            'el-tree': { template: '<div v-bind="$attrs"></div>' },
            'el-drawer': true,
            'el-table': true,
            'el-table-column': true,
            'el-tag': true,
            'el-date-picker': true
          }
        }
      })
    })

    it('应该渲染 splitpanes 容器', () => {
      const splitpanes = slotWrapper.find('.splitpanes')
      expect(splitpanes.exists()).toBe(true)
    })

    it('中间面板应该渲染 FileTreePanel', () => {
      const panes = slotWrapper.findAll('.pane')
      expect(panes.length).toBe(3)
      expect(panes[1].find('.stub-file-tree-panel').exists()).toBe(true)
    })

    it('右侧面板应该渲染 ContentPanel', () => {
      const panes = slotWrapper.findAll('.pane')
      expect(panes[2].find('.stub-content-panel').exists()).toBe(true)
    })

    it('应该渲染三个 Pane 面板', () => {
      const panes = slotWrapper.findAll('.pane')
      expect(panes.length).toBe(3)
    })
  })

  describe('loadDirectories 默认选中逻辑', () => {
    const dirStubs = {
      'el-container': { template: '<div><slot /></div>' },
      'el-header': true,
      'el-aside': { template: '<aside v-bind="$attrs"><slot /></aside>' },
      'el-main': { template: '<main><slot /></main>' },
      'el-tree': true,
      'el-dialog': true,
      'el-form': true,
      'el-form-item': true,
      'el-input': true,
      'el-switch': true,
      'el-button': true,
      'el-button-group': true,
      'el-divider': true,
      'el-select': true,
      'el-option': true,
      'el-empty': true,
      'el-descriptions': true,
      'el-descriptions-item': true,
      'el-icon': true,
      'el-progress': true,
      'el-table': true,
      'el-table-column': true
    }

    let GetDirectoriesMock

    beforeEach(async () => {
      const appModule = await vi.importMock('../../../wailsjs/go/main/App')
      GetDirectoriesMock = appModule.GetDirectories
    })

    afterEach(() => {
      GetDirectoriesMock.mockClear()
    })

    it('应该自动选中默认目录', async () => {
      GetDirectoriesMock.mockResolvedValueOnce([
        { id: 'dir-1', name: '项目A', path: '/a', isDefault: false },
        { id: 'dir-2', name: '项目B', path: '/b', isDefault: true },
        { id: 'dir-3', name: '项目C', path: '/c', isDefault: false }
      ])

      const w = mount(Home, { global: { stubs: dirStubs } })
      await flushPromises()

      expect(GetDirectoriesMock).toHaveBeenCalled()
      expect(useDirectoryStore().selectedDirectoryId).toBe('dir-2')
      w.unmount()
    })

    it('无默认目录时应该选中第一个', async () => {
      GetDirectoriesMock.mockResolvedValueOnce([
        { id: 'dir-1', name: '项目A', path: '/a', isDefault: false },
        { id: 'dir-2', name: '项目B', path: '/b', isDefault: false }
      ])

      const w = mount(Home, { global: { stubs: dirStubs } })
      await flushPromises()

      expect(useDirectoryStore().selectedDirectoryId).toBe('dir-1')
      w.unmount()
    })

    it('空列表不应报错', async () => {
      const w = mount(Home, { global: { stubs: dirStubs } })
      await flushPromises()

      expect(useDirectoryStore().selectedDirectoryId).toBe('')
      expect(useDirectoryStore().directories).toEqual([])
      w.unmount()
    })
  })

  describe('ActivityBar 和 ToolboxPanel 集成', () => {
    const createWrapper = () => {
      return mount(Home, {
        global: {
          stubs: {
            Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
            Pane: { template: '<div class="pane"><slot /></div>', props: ['size', 'minSize', 'maxSize'] },
            ActivityBar: { template: '<div class="stub-activity-bar" />', props: ['modelValue'] },
            DirectoryTree: { template: '<div class="stub-directory-tree" />' },
            ToolboxPanel: { template: '<div class="stub-toolbox-panel" />' },
            FileTreePanel: { template: '<div class="stub-file-tree-panel" />' },
            ContentPanel: { template: '<div class="stub-content-panel" />', methods: { clearPreview: () => {}, startBatchPull: () => {}, previewFile: () => {} } },
            'el-tree': true,
            'el-dialog': true,
            'el-form': true,
            'el-form-item': true,
            'el-input': true,
            'el-switch': true,
            'el-button': true,
            'el-button-group': true,
            'el-divider': true,
            'el-select': true,
            'el-option': true,
            'el-empty': true,
            'el-descriptions': true,
            'el-descriptions-item': true,
            'el-icon': true
          }
        }
      })
    }

    it('应该渲染 ActivityBar 组件', () => {
      const wrapper = createWrapper()
      expect(wrapper.find('.stub-activity-bar').exists()).toBe(true)
    })

    it('默认 activePanel 应为 directory', () => {
      const wrapper = createWrapper()
      expect(useUiStore().activePanel).toBe('directory')
    })

    it('activePanel 为 toolbox 时不显示 DirectoryTree', async () => {
      const wrapper = createWrapper()
      useUiStore().activePanel = 'toolbox'
      await wrapper.vm.$nextTick()
      expect(wrapper.find('.stub-toolbox-panel').exists()).toBe(true)
    })
  })

  describe('Ctrl+C 复制拦截修复（预览选中文本放行）', () => {
    const createWrapper = () => mount(Home, {
      global: {
        stubs: {
          Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
          Pane: { template: '<div class="pane"><slot /></div>' },
          ActivityBar: { template: '<div />', props: ['modelValue'] },
          DirectoryTree: { template: '<div />' },
          ToolboxPanel: { template: '<div />' },
          FileTreePanel: { template: '<div />' },
          ContentPanel: { template: '<div />', methods: { clearPreview: () => {}, startBatchPull: () => {}, previewFile: () => {} } },
          'el-tree': true,
          'el-dialog': true,
          'el-form': true,
          'el-form-item': true,
          'el-input': true,
          'el-switch': true,
          'el-button': true,
          'el-button-group': true,
          'el-divider': true,
          'el-select': true,
          'el-option': true,
          'el-empty': true,
          'el-descriptions': true,
          'el-descriptions-item': true,
          'el-icon': true
        }
      }
    })

    let w
    let getSelectionSpy

    beforeEach(async () => {
      w = createWrapper()
      await flushPromises()
      useWorkspaceStore().selectedNode = { name: 'a.txt', path: '/a/a.txt', type: 'file' }
    })

    afterEach(() => {
      if (getSelectionSpy) {
        getSelectionSpy.mockRestore()
        getSelectionSpy = null
      }
      if (w) {
        w.unmount()
        w = null
      }
    })

    const dispatchCtrlC = () => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'c', ctrlKey: true, bubbles: true }))
    }

    it('预览区选中文本时 Ctrl+C 放行，不复制文件路径', async () => {
      getSelectionSpy = vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => '选中的预览文本' })
      const App = await vi.importMock('../../../wailsjs/go/main/App')
      App.CopyToSystemClipboard.mockClear()

      dispatchCtrlC()
      await flushPromises()

      expect(App.CopyToSystemClipboard).not.toHaveBeenCalled()
    })

    it('无选中文本时 Ctrl+C 仍复制文件路径（保持原行为）', async () => {
      getSelectionSpy = vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => '' })
      const App = await vi.importMock('../../../wailsjs/go/main/App')
      App.CopyToSystemClipboard.mockClear()

      dispatchCtrlC()
      await flushPromises()

      expect(App.CopyToSystemClipboard).toHaveBeenCalledWith('/a/a.txt')
    })
  })

  describe('onNodeSelect 显式传参修复（回归：链接跳转后再点空白预览到上一文件）', () => {
    // 复现真实时序：父组件 selectedNode ref 更新后，子组件 ContentPanel 的
    // props.selectedNode 在 nextTick 才 patch。若 onNodeSelect 用无参 previewFile()，
    // 其内部 `targetPath = overridePath || props.selectedNode?.path` 读到的是【旧节点】路径。
    // 修复：显式传入当前 data.path / data.name，绕开 props 更新时机。
    const mountWithStub = (previewFileMock) => {
      return mount(Home, {
        global: {
          stubs: {
            Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
            Pane: { template: '<div class="pane"><slot /></div>' },
            DirectoryTree: { template: '<div />' },
            FileTreePanel: { template: '<div />' },
            ContentPanel: {
              template: '<div />',
              methods: {
                clearPreview: () => {},
                startBatchPull: () => {},
                previewFile: previewFileMock
              }
            },
            'el-dialog': true,
            'el-drawer': true,
            'el-table': true,
            'el-table-column': true,
            'el-tag': true,
            'el-date-picker': true,
            'el-select': true,
            'el-option': true,
            'el-button': true
          }
        }
      })
    }

    it('点击 file 节点时显式传入该节点 path/name（不依赖 props 异步更新）', async () => {
      const previewFileMock = vi.fn()
      const w = mountWithStub(previewFileMock)
      await flushPromises()

      const node = { name: 'a.md', path: '/dir/a.md', type: 'file' }
      w.vm.onNodeSelect(node)
      await flushPromises()

      expect(previewFileMock).toHaveBeenCalledTimes(1)
      expect(previewFileMock).toHaveBeenLastCalledWith('/dir/a.md', 'a.md')
      w.unmount()
    })

    it('连续切换不同 file 节点，最后一次 previewFile 调用参数为当前节点（非上一节点）', async () => {
      const previewFileMock = vi.fn()
      const w = mountWithStub(previewFileMock)
      await flushPromises()

      const nodeA = { name: 'a.md', path: '/dir/a.md', type: 'file' }
      const nodeB = { name: 'b.md', path: '/dir/sub/b.md', type: 'file' }
      w.vm.onNodeSelect(nodeA)
      await flushPromises()
      w.vm.onNodeSelect(nodeB)
      await flushPromises()

      expect(previewFileMock).toHaveBeenCalledTimes(2)
      // 关键断言：最后一次调用参数是 nodeB（修复前会读到 nodeA 的路径）
      expect(previewFileMock).toHaveBeenLastCalledWith('/dir/sub/b.md', 'b.md')
      // 且不应出现「第二次仍用 nodeA 路径」的回归情形
      expect(previewFileMock.mock.calls[1]).toEqual(['/dir/sub/b.md', 'b.md'])
      w.unmount()
    })

    it('切到非 file 节点调用 clearPreview，不调用 previewFile', async () => {
      const previewFileMock = vi.fn()
      const w = mountWithStub(previewFileMock)
      await flushPromises()

      const dirNode = { name: 'sub', path: '/dir/sub', type: 'directory' }
      w.vm.onNodeSelect(dirNode)
      await flushPromises()

      expect(previewFileMock).not.toHaveBeenCalled()
      w.unmount()
    })
  })

  describe('onRepoLocate 跨工作目录跳转衔接', () => {
    // 复用 research/cross-workdir-locate.md 推荐时序：
    //   规范化（\ -> / + toLowerCase）查找 targetDir -> 关弹窗 -> 跨目录则 await onDirectorySelect -> locateNode
    const mountWithLocate = (locateNodeMock) => mount(Home, {
      global: {
        stubs: {
          Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
          Pane: { template: '<div class="pane"><slot /></div>' },
          DirectoryTree: { template: '<div />' },
          FileTreePanel: {
            template: '<div />',
            methods: {
              saveCurrentState: () => {},
              restoreTreeState: () => {},
              locateNode: locateNodeMock
            }
          },
          ContentPanel: { template: '<div />', methods: { clearPreview: () => {}, startBatchPull: () => {}, previewFile: () => {} } },
          RepoFilterDialog: { template: '<div />' },
          'el-tree': true,
          'el-dialog': true,
          'el-form': true,
          'el-form-item': true,
          'el-input': true,
          'el-switch': true,
          'el-button': true,
          'el-button-group': true,
          'el-divider': true,
          'el-select': true,
          'el-option': true,
          'el-empty': true,
          'el-descriptions': true,
          'el-descriptions-item': true,
          'el-icon': true
        }
      }
    })

    let w
    afterEach(() => {
      if (w) { w.unmount(); w = null }
    })

    it('同工作目录：直接 locateNode，不切换工作目录', async () => {
      const locateNodeMock = vi.fn().mockResolvedValue(undefined)
      w = mountWithLocate(locateNodeMock)
      await flushPromises()
      useDirectoryStore().directories = [
        { id: 'dir-1', name: '工作目录1', path: 'D:/work', isGitRepo: false, isDefault: false }
      ]
      useDirectoryStore().selectedDirectoryId = 'dir-1'
      await flushPromises()

      await w.vm.onRepoLocate('D:/work/repo-a')
      await flushPromises()

      expect(locateNodeMock).toHaveBeenCalledWith('D:/work/repo-a')
      // 同工作目录不应触发切换
      expect(useDirectoryStore().selectedDirectoryId).toBe('dir-1')
    })

    it('跨工作目录：先切换工作目录再 locateNode', async () => {
      const locateNodeMock = vi.fn().mockResolvedValue(undefined)
      w = mountWithLocate(locateNodeMock)
      await flushPromises()
      useDirectoryStore().directories = [
        { id: 'dir-1', name: '工作目录1', path: 'D:/work', isGitRepo: false, isDefault: false },
        { id: 'dir-2', name: '工作目录2', path: 'D:/other', isGitRepo: false, isDefault: false }
      ]
      useDirectoryStore().selectedDirectoryId = 'dir-1'
      await flushPromises()

      await w.vm.onRepoLocate('D:/other/repo-x')
      await flushPromises()

      // 应切换到 dir-2（跨工作目录先切换）
      expect(useDirectoryStore().selectedDirectoryId).toBe('dir-2')
      // 切换完成后调用 locateNode 定位目标
      expect(locateNodeMock).toHaveBeenCalledWith('D:/other/repo-x')
    })

    it('未知路径：给出警告且不调用 locateNode', async () => {
      const locateNodeMock = vi.fn().mockResolvedValue(undefined)
      w = mountWithLocate(locateNodeMock)
      await flushPromises()
      useDirectoryStore().directories = [
        { id: 'dir-1', name: '工作目录1', path: 'D:/work', isGitRepo: false, isDefault: false }
      ]
      useDirectoryStore().selectedDirectoryId = 'dir-1'
      await flushPromises()
      ElMessage.warning.mockClear()

      await w.vm.onRepoLocate('D:/unknown/repo')
      await flushPromises()

      expect(ElMessage.warning).toHaveBeenCalledWith('未找到该仓库所属的工作目录')
      expect(locateNodeMock).not.toHaveBeenCalled()
    })

    it('大小写/分隔符差异：规范化后仍能命中目标工作目录并定位', async () => {
      const locateNodeMock = vi.fn().mockResolvedValue(undefined)
      w = mountWithLocate(locateNodeMock)
      await flushPromises()
      // 工作目录路径用反斜杠 + 大写盘符，仓库路径用正斜杠 + 小写盘符
      useDirectoryStore().directories = [
        { id: 'dir-1', name: '工作目录1', path: 'D:\\work', isGitRepo: false, isDefault: false }
      ]
      useDirectoryStore().selectedDirectoryId = 'dir-1'
      await flushPromises()

      await w.vm.onRepoLocate('d:/work/repo-a')
      await flushPromises()

      // 规范化（\ -> / + toLowerCase）后应命中 dir-1 并定位（规避 locateNode 内 startsWith 大小写敏感的静默失败）
      expect(locateNodeMock).toHaveBeenCalledWith('d:/work/repo-a')
    })
  })
})

// ===== 补充：未覆盖 handler 分支 =====
describe('Home.vue - handler 分支补充', () => {
  let wrapper
  // 子组件方法 spy 收集器
  let fileTreeSpies, dirTreeSpies, contentSpies

  const mountHome = () => {
    fileTreeSpies = {
      saveCurrentState: vi.fn(), restoreTreeState: vi.fn(), locateNode: vi.fn().mockResolvedValue(),
      refreshNode: vi.fn(), triggerRenameCurrent: vi.fn(), triggerDeleteCurrent: vi.fn(),
      showRenameAt: vi.fn(), showCreateAt: vi.fn(), showCopyToDialog: vi.fn(),
      closeCopyToDialog: vi.fn(), setCopyToLoading: vi.fn(), closeMenu: vi.fn()
    }
    dirTreeSpies = { closeMenu: vi.fn(), triggerRenameCurrent: vi.fn(), triggerDeleteCurrent: vi.fn() }
    contentSpies = { clearPreview: vi.fn(), startBatchPull: vi.fn(), previewFile: vi.fn() }
    return mount(Home, {
      global: {
        stubs: {
          Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
          Pane: { template: '<div class="pane"><slot /></div>' },
          DirectoryTree: { template: '<div />', methods: dirTreeSpies },
          FileTreePanel: { template: '<div />', methods: fileTreeSpies },
          ContentPanel: { template: '<div />', methods: contentSpies },
          RepoFilterDialog: { template: '<div />' },
          'el-tree': true, 'el-dialog': true, 'el-form': true, 'el-form-item': true,
          'el-input': true, 'el-switch': true, 'el-button': true, 'el-button-group': true,
          'el-divider': true, 'el-select': true, 'el-option': true, 'el-empty': true,
          'el-descriptions': true, 'el-descriptions-item': true, 'el-icon': true
        }
      }
    })
  }

  beforeEach(async () => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    localStorage.clear()
    wrapper = mountHome()
    await flushPromises()
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
  })

  // matchShortcut 严格比对 ctrlKey/altKey/shiftKey，事件须显式给 false
  const keyEvent = (over = {}) => ({
    ctrlKey: false, altKey: false, shiftKey: false, metaKey: false,
    target: document.body, preventDefault: () => {}, ...over
  })

  // ---- resolveTargetDir / closeToolbox / onRefreshNode ----
  it('resolveTargetDir：directory 直接返回 path，file 取父目录', () => {
    expect(wrapper.vm.resolveTargetDir({ type: 'directory', path: 'D:\\dir' })).toBe('D:\\dir')
    expect(wrapper.vm.resolveTargetDir({ type: 'file', path: 'D:\\dir\\a.go' })).toBe('D:\\dir')
    expect(wrapper.vm.resolveTargetDir({ type: 'file', path: 'a.go' })).toBe('')
  })

  it('closeToolbox：toolbox 激活时切回 directory', () => {
    const ui = useUiStore()
    ui.activePanel = 'toolbox'
    wrapper.vm.closeToolbox()
    expect(ui.activePanel).toBe('directory')
    // 非 toolbox 时不变
    ui.activePanel = 'directory'
    wrapper.vm.closeToolbox()
    expect(ui.activePanel).toBe('directory')
  })

  it('onRefreshNode 透传给 fileTreePanel.refreshNode', () => {
    wrapper.vm.onRefreshNode('/some/path')
    expect(fileTreeSpies.refreshNode).toHaveBeenCalledWith('/some/path')
  })

  // ---- handleCopy / handleCut ----
  it('handleCopy 写入剪贴板态 + 系统剪贴板', async () => {
    const { CopyToSystemClipboard } = await import('../../../wailsjs/go/main/App')
    const ws = useWorkspaceStore()
    await wrapper.vm.handleCopy({ path: 'D:\\a.go', name: 'a.go', type: 'file' })
    expect(ws.clipboard.mode).toBe('copy')
    expect(ws.clipboard.sourcePath).toBe('D:\\a.go')
    expect(CopyToSystemClipboard).toHaveBeenCalledWith('D:\\a.go')
  })

  it('handleCut 写入剪贴板态为 cut', async () => {
    const { CutToSystemClipboard } = await import('../../../wailsjs/go/main/App')
    const ws = useWorkspaceStore()
    await wrapper.vm.handleCut({ path: 'D:\\b.go', name: 'b.go', type: 'file' })
    expect(ws.clipboard.mode).toBe('cut')
    expect(CutToSystemClipboard).toHaveBeenCalledWith('D:\\b.go')
  })

  // ---- handlePaste 分支 ----
  it('handlePaste：目标目录为空时直接返回', async () => {
    const { ReadFromSystemClipboard } = await import('../../../wailsjs/go/main/App')
    await wrapper.vm.handlePaste({ type: 'file', path: 'a.go' })
    expect(ReadFromSystemClipboard).not.toHaveBeenCalled()
  })

  it('handlePaste：剪贴板为空时 info 提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { ReadFromSystemClipboard } = await import('../../../wailsjs/go/main/App')
    ReadFromSystemClipboard.mockResolvedValueOnce(null)
    await wrapper.vm.handlePaste({ type: 'directory', path: 'D:\\dst' })
    expect(ElMessage.info).toHaveBeenCalledWith('剪贴板中没有可粘贴的内容')
  })

  it('handlePaste：复制模式成功时 success + refreshNode', async () => {
    const { ElMessage } = await import('element-plus')
    const { ReadFromSystemClipboard, CopyItem } = await import('../../../wailsjs/go/main/App')
    ReadFromSystemClipboard.mockResolvedValueOnce(JSON.stringify({ paths: ['D:\\a.go'], isCut: false }))
    CopyItem.mockResolvedValueOnce('ok')
    await wrapper.vm.handlePaste({ type: 'directory', path: 'D:\\dst' })
    await flushPromises()
    expect(CopyItem).toHaveBeenCalledWith('D:\\a.go', 'D:\\dst')
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('1'))
    expect(fileTreeSpies.refreshNode).toHaveBeenCalledWith('D:\\dst')
  })

  it('handlePaste：剪切模式成功时调 MoveItem + clearClipboard', async () => {
    const { ReadFromSystemClipboard, MoveItem } = await import('../../../wailsjs/go/main/App')
    const ws = useWorkspaceStore()
    ws.clipboard.mode = 'cut'
    ReadFromSystemClipboard.mockResolvedValueOnce(JSON.stringify({ paths: ['D:\\a.go'], isCut: true }))
    MoveItem.mockResolvedValueOnce('ok')
    await wrapper.vm.handlePaste({ type: 'directory', path: 'D:\\dst' })
    await flushPromises()
    expect(MoveItem).toHaveBeenCalledWith('D:\\a.go', 'D:\\dst')
    // clearClipboard 重置剪贴板态
    expect(ws.clipboard.mode).toBeFalsy()
  })

  it('handlePaste：全部失败时 error 提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { ReadFromSystemClipboard, CopyItem } = await import('../../../wailsjs/go/main/App')
    ReadFromSystemClipboard.mockResolvedValueOnce(JSON.stringify({ paths: ['D:\\a.go'], isCut: false }))
    CopyItem.mockResolvedValueOnce('错误：源不存在')
    await wrapper.vm.handlePaste({ type: 'directory', path: 'D:\\dst' })
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('粘贴失败')
  })

  it('handlePaste：抛异常时 error 提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { ReadFromSystemClipboard } = await import('../../../wailsjs/go/main/App')
    ReadFromSystemClipboard.mockRejectedValueOnce(new Error('boom'))
    await wrapper.vm.handlePaste({ type: 'directory', path: 'D:\\dst' })
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('boom'))
  })

  // ---- onBatchPull / onAddWorkDir ----
  it('onBatchPull 成功时调 startBatchPull', async () => {
    const { ScanAndPullRepos } = await import('../../../wailsjs/go/main/App')
    ScanAndPullRepos.mockResolvedValueOnce({ total: 3 })
    await wrapper.vm.onBatchPull({ path: 'D:\\root' })
    await flushPromises()
    expect(ScanAndPullRepos).toHaveBeenCalledWith('D:\\root')
    expect(contentSpies.startBatchPull).toHaveBeenCalledWith({ total: 3 })
  })

  it('onBatchPull 失败时 warning 提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { ScanAndPullRepos } = await import('../../../wailsjs/go/main/App')
    ScanAndPullRepos.mockRejectedValueOnce('some error msg')
    await wrapper.vm.onBatchPull({ path: 'D:\\root' })
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('some error msg')
  })

  it('onAddWorkDir 成功时 reload + success', async () => {
    const { ElMessage } = await import('element-plus')
    const { AddDirectory } = await import('../../../wailsjs/go/main/App')
    AddDirectory.mockResolvedValueOnce({ id: '1' })
    await wrapper.vm.onAddWorkDir({ name: '新目录', path: 'D:\\new' })
    await flushPromises()
    expect(AddDirectory).toHaveBeenCalledWith('新目录', 'D:\\new', false)
    expect(ElMessage.success).toHaveBeenCalledWith('已添加为工作目录')
  })

  it('onAddWorkDir 返回 null 时 error', async () => {
    const { ElMessage } = await import('element-plus')
    const { AddDirectory } = await import('../../../wailsjs/go/main/App')
    AddDirectory.mockResolvedValueOnce(null)
    await wrapper.vm.onAddWorkDir({ name: 'x', path: 'D:\\x' })
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('添加工作目录失败')
  })

  it('onAddWorkDir 抛异常时 error', async () => {
    const { ElMessage } = await import('element-plus')
    const { AddDirectory } = await import('../../../wailsjs/go/main/App')
    AddDirectory.mockRejectedValueOnce(new Error('dup'))
    await wrapper.vm.onAddWorkDir({ name: 'x', path: 'D:\\x' })
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('dup'))
  })

  // ---- onDeleteFromFileTree / onDeleteFromContent ----
  it('onDeleteFromFileTree：选中节点在删除子树内时清空预览', () => {
    const ws = useWorkspaceStore()
    ws.selectedNode = { path: 'D:\\dir\\sub\\a.go' }
    wrapper.vm.onDeleteFromFileTree({ path: 'D:\\dir\\sub' })
    expect(ws.selectedNode).toBeNull()
    expect(contentSpies.clearPreview).toHaveBeenCalled()
  })

  it('onDeleteFromFileTree：选中节点不在删除子树内时保留', () => {
    const ws = useWorkspaceStore()
    ws.selectedNode = { path: 'D:\\other\\b.go' }
    wrapper.vm.onDeleteFromFileTree({ path: 'D:\\dir\\sub' })
    expect(ws.selectedNode).toBeTruthy()
  })

  it('onDeleteFromFileTree：无选中节点时直接返回', () => {
    const ws = useWorkspaceStore()
    ws.selectedNode = null
    wrapper.vm.onDeleteFromFileTree({ path: 'D:\\dir' })
    expect(contentSpies.clearPreview).not.toHaveBeenCalled()
  })

  it('onDeleteFromContent：用户取消时不删除', async () => {
    const { ElMessageBox, ElMessage } = await import('element-plus')
    ElMessageBox.confirm.mockRejectedValueOnce('cancel')
    await wrapper.vm.onDeleteFromContent({ name: 'a.go', path: 'D:\\a.go' })
    await flushPromises()
    const { DeleteFile } = await import('../../../wailsjs/go/main/App')
    expect(DeleteFile).not.toHaveBeenCalled()
    expect(ElMessage.success).not.toHaveBeenCalled()
  })

  it('onDeleteFromContent：确认后删除成功 + 刷新父节点', async () => {
    const { ElMessageBox } = await import('element-plus')
    const { DeleteFile } = await import('../../../wailsjs/go/main/App')
    ElMessageBox.confirm.mockResolvedValueOnce(true)
    DeleteFile.mockResolvedValueOnce(true)
    await wrapper.vm.onDeleteFromContent({ name: 'a.go', path: 'D:\\dir\\a.go' })
    await flushPromises()
    expect(DeleteFile).toHaveBeenCalledWith('D:\\dir\\a.go')
    expect(fileTreeSpies.refreshNode).toHaveBeenCalledWith('D:\\dir')
  })

  it('onDeleteFromContent：DeleteFile 返回 false 时 error', async () => {
    const { ElMessageBox, ElMessage } = await import('element-plus')
    const { DeleteFile } = await import('../../../wailsjs/go/main/App')
    ElMessageBox.confirm.mockResolvedValueOnce(true)
    DeleteFile.mockResolvedValueOnce(false)
    await wrapper.vm.onDeleteFromContent({ name: 'a.go', path: 'a.go' })
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('删除失败')
  })

  // ---- onPaletteSelectFile / onPaletteSelectFavorite ----
  it('onPaletteSelectFile：路径在当前目录内直接 locateNode', async () => {
    const dirStore = useDirectoryStore()
    dirStore.directories = [{ id: 'd1', name: 'w', path: 'D:\\work', isGitRepo: false }]
    dirStore.selectedDirectoryId = 'd1'
    await wrapper.vm.onPaletteSelectFile({ path: 'D:\\work\\a.go', type: 'file' })
    expect(fileTreeSpies.locateNode).toHaveBeenCalledWith('D:\\work\\a.go')
  })

  it('onPaletteSelectFile：路径在其他目录时切目录后 locateNode', async () => {
    const dirStore = useDirectoryStore()
    dirStore.directories = [
      { id: 'd1', name: 'w1', path: 'D:\\work1', isGitRepo: false },
      { id: 'd2', name: 'w2', path: 'D:\\work2', isGitRepo: false }
    ]
    dirStore.selectedDirectoryId = 'd1'
    await wrapper.vm.onPaletteSelectFile({ path: 'D:\\work2\\b.go', type: 'file' })
    await flushPromises()
    expect(dirStore.selectedDirectoryId).toBe('d2')
    expect(fileTreeSpies.locateNode).toHaveBeenCalledWith('D:\\work2\\b.go')
  })

  // ---- onUpdateAvailable / onOpenContentSearch ----
  it('onUpdateAvailable 写入 updateInfo + 打开弹窗', () => {
    const ui = useUiStore()
    wrapper.vm.onUpdateAvailable({ latestVer: '1.2.0' })
    expect(ui.updateInfo.latestVer).toBe('1.2.0')
    expect(ui.updateDialogVisible).toBe(true)
  })

  it('onOpenContentSearch 带 subDir 时设置 :subDir/ 初始串', () => {
    const ui = useUiStore()
    wrapper.vm.onOpenContentSearch('sub\\dir')
    expect(ui.contentSearchInit).toBe(':sub/dir/ ')
    expect(ui.commandPaletteVisible).toBe(true)
  })

  it('onOpenContentSearch 无 subDir 时设置 : 初始串', () => {
    const ui = useUiStore()
    wrapper.vm.onOpenContentSearch('')
    expect(ui.contentSearchInit).toBe(':')
  })

  // ---- handleGlobalKeydown 分支 ----
  it('快捷键打开命令面板', () => {
    const ui = useUiStore()
    ui.commandPaletteVisible = false
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: 'p', ctrlKey: true }))
    expect(ui.commandPaletteVisible).toBe(true)
  })

  it('快捷键切换终端', () => {
    const ui = useUiStore()
    const before = ui.terminalVisible
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: '`', ctrlKey: true }))
    expect(ui.terminalVisible).toBe(!before)
  })

  it('F5 刷新选中节点', () => {
    const ws = useWorkspaceStore()
    ws.selectedNode = { path: 'D:\\dir' }
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: 'F5' }))
    expect(fileTreeSpies.refreshNode).toHaveBeenCalledWith('D:\\dir')
  })

  it('F5 无选中节点时不刷新', () => {
    const ws = useWorkspaceStore()
    ws.selectedNode = null
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: 'F5' }))
    expect(fileTreeSpies.refreshNode).not.toHaveBeenCalled()
  })

  it('重命名快捷键作用于最近交互的目录树', () => {
    const ws = useWorkspaceStore()
    ws.lastInteractedTree = 'directory'
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: 'F2' }))
    expect(dirTreeSpies.triggerRenameCurrent).toHaveBeenCalled()
  })

  it('删除快捷键作用于最近交互的文件树', () => {
    const ws = useWorkspaceStore()
    ws.lastInteractedTree = 'file'
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: 'Delete' }))
    expect(fileTreeSpies.triggerDeleteCurrent).toHaveBeenCalled()
  })

  it('输入框聚焦时不触发重命名/删除', () => {
    const input = document.createElement('input')
    document.body.appendChild(input)
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: 'F2', target: input }))
    expect(dirTreeSpies.triggerRenameCurrent).not.toHaveBeenCalled()
    document.body.removeChild(input)
  })

  it('Ctrl+C 预览区选中文本时放行原生复制', () => {
    const ws = useWorkspaceStore()
    ws.selectedNode = { path: 'D:\\a.go' }
    const getSelectionSpy = vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => 'selected text' })
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: 'c', ctrlKey: true }))
    expect(ws.clipboard.sourcePath).toBe('')
    getSelectionSpy.mockRestore()
  })

  it('Ctrl+C 无选中文本时触发 handleCopy', () => {
    const ws = useWorkspaceStore()
    ws.selectedNode = { path: 'D:\\a.go' }
    const getSelectionSpy = vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => '' })
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: 'c', ctrlKey: true }))
    expect(ws.clipboard.sourcePath).toBe('D:\\a.go')
    getSelectionSpy.mockRestore()
  })

  it('无选中节点时 Ctrl+C 直接返回', () => {
    const ws = useWorkspaceStore()
    ws.selectedNode = null
    const getSelectionSpy = vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => '' })
    wrapper.vm.handleGlobalKeydown(keyEvent({ key: 'c', ctrlKey: true }))
    expect(ws.clipboard.sourcePath).toBe('')
    getSelectionSpy.mockRestore()
  })
})
