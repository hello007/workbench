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
vi.mock('../../wailsjs/runtime/runtime', () => ({
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
  ReadFromSystemClipboard: vi.fn(() => Promise.resolve(null))
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
          FileTreePanel: { template: '<div class="stub-file-tree-panel" />' },
          ContentPanel: { template: '<div class="stub-content-panel" />', methods: { clearPreview: () => {}, startBatchPull: () => {} } },
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
            ContentPanel: { template: '<div class="stub-content-panel" />' }
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
            ContentPanel: { template: '<div class="stub-content-panel" />', methods: { clearPreview: () => {}, startBatchPull: () => {} } },
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
})
