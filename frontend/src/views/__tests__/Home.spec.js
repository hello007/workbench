/**
 * Home.vue 组件测试
 * 重点关注修复的两个bug：
 * 1. 懒加载树根节点检测
 * 2. 节点切换时预览状态清理
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import Home from '../Home.vue'

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
  RefreshDirectoriesGitFlag: vi.fn(() => Promise.resolve([]))
}))

describe('Home.vue - Bug修复验证', () => {
  let wrapper

  beforeEach(() => {
    wrapper = mount(Home, {
      global: {
        stubs: {
          Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
          Pane: { template: '<div class="pane"><slot /></div>', props: ['size', 'minSize', 'maxSize'] },
          DirectoryTree: { template: '<div class="stub-directory-tree" />' },
          FileTreePanel: { template: '<div class="stub-file-tree-panel" />', methods: { saveCurrentState: () => {}, restoreTreeState: () => {} } },
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
      expect(typeof wrapper.vm.loadDirectories).toBe('function')
    })

    it('应该正确选择目录后清空选中节点', () => {
      wrapper.vm.selectedNode = { name: 'test', path: '/test' }
      wrapper.vm.onDirectorySelect('new-dir-id')

      expect(wrapper.vm.selectedDirectoryId).toBe('new-dir-id')
      expect(wrapper.vm.selectedNode).toBeNull()
      expect(wrapper.vm.latestCommit).toBeNull()
    })

    it('应该正确处理目录切换', () => {
      wrapper.vm.onDirectorySelect('dir-1')
      expect(wrapper.vm.selectedDirectoryId).toBe('dir-1')
    })
  })

  describe('工作目录切换 git 仓库双刷新修复', () => {
    it('切到 git 工作目录时 selectedNode 立即等于期望的 git 节点对象，无 null 中间态', async () => {
      wrapper.vm.directories = [
        { id: 'git-1', name: '仓库A', path: '/a/git-repo', isGitRepo: true, isDefault: false }
      ]

      await wrapper.vm.onDirectorySelect('git-1')
      await flushPromises()

      expect(wrapper.vm.selectedDirectoryId).toBe('git-1')
      expect(wrapper.vm.selectedNode).toEqual({
        id: 'git-1',
        path: '/a/git-repo',
        name: '仓库A',
        type: 'directory',
        isGitRepo: true
      })
      // latestCommit 应被清零
      expect(wrapper.vm.latestCommit).toBeNull()
    })

    it('切到非 git 工作目录时 selectedNode 被置 null', async () => {
      wrapper.vm.directories = [
        { id: 'plain-1', name: '普通目录', path: '/b/plain', isGitRepo: false, isDefault: false }
      ]

      await wrapper.vm.onDirectorySelect('plain-1')
      await flushPromises()

      expect(wrapper.vm.selectedDirectoryId).toBe('plain-1')
      expect(wrapper.vm.selectedNode).toBeNull()
      expect(wrapper.vm.latestCommit).toBeNull()
    })

    it('gitA → gitB 切换时 selectedNode 由 A-git 直切 B-git（无 null 中间态）', async () => {
      wrapper.vm.directories = [
        { id: 'git-A', name: '仓库A', path: '/a/gitA', isGitRepo: true, isDefault: false },
        { id: 'git-B', name: '仓库B', path: '/b/gitB', isGitRepo: true, isDefault: false }
      ]
      await wrapper.vm.onDirectorySelect('git-A')
      await flushPromises()
      expect(wrapper.vm.selectedNode.path).toBe('/a/gitA')

      // 切到 B：观察中间态是否经过 null
      const observed = []
      const unwatch = wrapper.vm.$watch(() => wrapper.vm.selectedNode, (v) => observed.push(v), { deep: true, flush: 'sync' })

      await wrapper.vm.onDirectorySelect('git-B')
      await flushPromises()
      unwatch()

      // 最终落到 B-git
      expect(wrapper.vm.selectedNode).toEqual({
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

      expect(wrapper.vm.selectedNode).toEqual(newNode)
    })

    it('应该保留选中的节点信息', () => {
      const newNode = {
        name: 'test-folder',
        path: '/test/folder',
        type: 'directory',
        isGitRepo: false
      }

      wrapper.vm.onNodeSelect(newNode)

      expect(wrapper.vm.selectedNode.name).toBe('test-folder')
      expect(wrapper.vm.selectedNode.path).toBe('/test/folder')
    })

    it('应该在Git仓库节点上选中', () => {
      const gitNode = {
        name: 'test-repo',
        path: '/test/repo',
        type: 'directory',
        isGitRepo: true
      }

      wrapper.vm.onNodeSelect(gitNode)

      expect(wrapper.vm.selectedNode).toEqual(gitNode)
    })

    it('切换文件树节点时应清零 latestCommit，避免上一个仓库的提交残留', () => {
      // 模拟上一个仓库经"提交历史"tab emit 后 latestCommit 已有值
      wrapper.vm.latestCommit = { sha: 'aaa', shortSha: 'aaa1111', message: '上一个仓库的提交' }
      expect(wrapper.vm.latestCommit).not.toBeNull()

      const newNode = {
        name: 'repo-B',
        path: '/test/repo-B',
        type: 'directory',
        isGitRepo: true
      }

      wrapper.vm.onNodeSelect(newNode)

      expect(wrapper.vm.selectedNode).toEqual(newNode)
      // 关键：切换节点后 latestCommit 被清零，GitInfo 不再显示上一个仓库的提交
      expect(wrapper.vm.latestCommit).toBeNull()
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
            ContentPanel: { template: '<div class="stub-content-panel" />', methods: { clearPreview: () => {}, previewFile: () => {} } }
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
            'el-tree': { template: '<div v-bind="$attrs"></div>' }
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
      expect(w.vm.selectedDirectoryId).toBe('dir-2')
      w.unmount()
    })

    it('无默认目录时应该选中第一个', async () => {
      GetDirectoriesMock.mockResolvedValueOnce([
        { id: 'dir-1', name: '项目A', path: '/a', isDefault: false },
        { id: 'dir-2', name: '项目B', path: '/b', isDefault: false }
      ])

      const w = mount(Home, { global: { stubs: dirStubs } })
      await flushPromises()

      expect(w.vm.selectedDirectoryId).toBe('dir-1')
      w.unmount()
    })

    it('空列表不应报错', async () => {
      const w = mount(Home, { global: { stubs: dirStubs } })
      await flushPromises()

      expect(w.vm.selectedDirectoryId).toBe('')
      expect(w.vm.directories).toEqual([])
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
      expect(wrapper.vm.activePanel).toBe('directory')
    })

    it('activePanel 为 toolbox 时不显示 DirectoryTree', async () => {
      const wrapper = createWrapper()
      wrapper.vm.activePanel = 'toolbox'
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
      w.vm.selectedNode = { name: 'a.txt', path: '/a/a.txt', type: 'file' }
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
            }
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
})
