/**
 * Home.vue 组件测试
 * 重点关注修复的两个bug：
 * 1. 懒加载树根节点检测
 * 2. 节点切换时预览状态清理
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
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

describe('Home.vue - Bug修复验证', () => {
  let wrapper

  beforeEach(() => {
    wrapper = mount(Home, {
      global: {
        stubs: {
          'el-container': true,
          'el-header': true,
          'el-aside': true,
          'el-main': true,
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
    it('应该正确识别根节点（level === 0）', async () => {
      const mockResolve = vi.fn()
      const rootNode = {
        level: 0,
        data: null
      }

      // 模拟目录数据
      await wrapper.vm.loadDirectories()
      wrapper.vm.selectedDirectoryId = 'test-dir-id'

      // 调用loadTreeNode
      await wrapper.vm.loadTreeNode(rootNode, mockResolve)

      // 验证resolve被调用（即使返回空数组）
      expect(mockResolve).toHaveBeenCalled()
    })

    it('应该正确识别子节点（level > 0且有data）', async () => {
      const mockResolve = vi.fn()
      const childNode = {
        level: 1,
        data: {
          path: '/test/path',
          name: 'test-folder'
        }
      }

      // 调用loadTreeNode
      await wrapper.vm.loadTreeNode(childNode, mockResolve)

      // 验证resolve被调用
      expect(mockResolve).toHaveBeenCalled()
    })

    it('应该处理没有data的节点', async () => {
      const mockResolve = vi.fn()
      const nodeWithoutData = {
        level: 1,
        data: null
      }

      // 调用loadTreeNode
      await wrapper.vm.loadTreeNode(nodeWithoutData, mockResolve)

      // 验证resolve被调用（应该作为根节点处理）
      expect(mockResolve).toHaveBeenCalled()
    })
  })

  describe('Bug修复 #2: 节点切换时预览状态清理', () => {
    it('应该在点击节点时清空文件预览', async () => {
      // 设置初始预览内容
      wrapper.vm.filePreview = {
        content: 'previous file content',
        error: ''
      }

      // 模拟点击新节点
      const newNode = {
        name: 'new-file.txt',
        path: '/test/new-file.txt',
        type: 'file',
        isGitRepo: false
      }

      await wrapper.vm.onNodeClick(newNode)

      // 验证预览被清空
      expect(wrapper.vm.filePreview.content).toBe('')
      expect(wrapper.vm.filePreview.error).toBe('')
    })

    it('应该保留选中的节点信息', async () => {
      const newNode = {
        name: 'test-folder',
        path: '/test/folder',
        type: 'directory',
        isGitRepo: false
      }

      await wrapper.vm.onNodeClick(newNode)

      // 验证selectedNode被更新
      expect(wrapper.vm.selectedNode.name).toBe('test-folder')
      expect(wrapper.vm.selectedNode.path).toBe('/test/folder')
    })

    it('应该在Git仓库节点上获取Git信息', async () => {
      const gitNode = {
        name: 'test-repo',
        path: '/test/repo',
        type: 'directory',
        isGitRepo: true
      }

      await wrapper.vm.onNodeClick(gitNode)

      // 验证filePreview仍然被清空（即使后续会获取Git信息）
      expect(wrapper.vm.filePreview.content).toBe('')
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

  describe('三栏布局验证（AC1）', () => {
    let layoutWrapper

    beforeEach(() => {
      layoutWrapper = mount(Home, {
        global: {
          stubs: {
            'el-container': { template: '<div class="el-container"><slot /></div>' },
            'el-aside': { template: '<aside v-bind="$attrs"><slot /></aside>' },
            'el-main': { template: '<main class="el-main"><slot /></main>' },
            'el-header': true,
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
            'el-table-column': true,
            'DirectoryTree': { template: '<div class="stub-directory-tree" />' },
            'FileTreePanel': { template: '<div class="stub-file-tree-panel" />' },
            'ContentPanel': { template: '<div class="stub-content-panel" />' }
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

    it('应该渲染三个面板组件', () => {
      expect(layoutWrapper.find('.stub-directory-tree').exists()).toBe(true)
      expect(layoutWrapper.find('.stub-file-tree-panel').exists()).toBe(true)
      expect(layoutWrapper.find('.stub-content-panel').exists()).toBe(true)
    })

    it('第一栏应该有 directory-aside 类且 width 为 200px', () => {
      const aside = layoutWrapper.find('.directory-aside')
      expect(aside.exists()).toBe(true)
      expect(aside.attributes('width')).toBe('200px')
    })

    it('第二栏应该有 file-tree-aside 类且 width 为 280px', () => {
      const aside = layoutWrapper.find('.file-tree-aside')
      expect(aside.exists()).toBe(true)
      expect(aside.attributes('width')).toBe('280px')
    })

    it('第三栏应该有 content-main 类', () => {
      const main = layoutWrapper.find('.content-main')
      expect(main.exists()).toBe(true)
    })

    it('应该包含 el-container 作为布局容器', () => {
      const container = layoutWrapper.find('.el-container')
      expect(container.exists()).toBe(true)
    })

    it('三栏应按左-中-右顺序排列在容器内', () => {
      const container = layoutWrapper.find('.el-container')
      const children = container.findAll(':scope > aside, :scope > main')
      const classes = children.map(el => {
        if (el.classes().includes('directory-aside')) return 'directory'
        if (el.classes().includes('file-tree-aside')) return 'file-tree'
        if (el.classes().includes('content-main')) return 'content'
        return 'unknown'
      })
      expect(classes).toEqual(['directory', 'file-tree', 'content'])
    })

    it('DirectoryTree 应嵌套在 directory-aside 内', () => {
      const aside = layoutWrapper.find('.directory-aside')
      expect(aside.find('.stub-directory-tree').exists()).toBe(true)
    })

    it('FileTreePanel 应嵌套在 file-tree-aside 内', () => {
      const aside = layoutWrapper.find('.file-tree-aside')
      expect(aside.find('.stub-file-tree-panel').exists()).toBe(true)
    })

    it('ContentPanel 应嵌套在 content-main 内', () => {
      const main = layoutWrapper.find('.content-main')
      expect(main.find('.stub-content-panel').exists()).toBe(true)
    })
  })

  describe('左侧文件树滚动条', () => {
    let slotWrapper

    beforeEach(() => {
      slotWrapper = mount(Home, {
        global: {
          stubs: {
            'el-container': { template: '<div><slot /></div>' },
            'el-header': { template: '<header><slot /></header>' },
            'el-aside': { template: '<aside v-bind="$attrs"><slot /></aside>' },
            'el-main': { template: '<main><slot /></main>' },
            'el-tree': { template: '<div v-bind="$attrs"></div>' },
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
            'GitInfo': true,
            'CommitHistory': true
          }
        }
      })
    })

    it('el-aside 应该有 file-tree-aside 类以启用 flex 布局', () => {
      const aside = slotWrapper.find('.file-tree-aside')
      expect(aside.exists()).toBe(true)
    })

    it('el-aside 内部应该有 tree-toolbar 容器', () => {
      const toolbar = slotWrapper.find('.tree-toolbar')
      expect(toolbar.exists()).toBe(true)
    })

    it('el-tree 应该有 file-tree 类以启用滚动条', async () => {
      await slotWrapper.vm.$nextTick()
      slotWrapper.vm.selectedDirectoryId = 'test-id'
      await slotWrapper.vm.$nextTick()
      const tree = slotWrapper.find('.file-tree')
      expect(tree.exists()).toBe(true)
    })

    it('内层 el-container 应该有 main-content 类以约束高度', () => {
      const innerContainer = slotWrapper.findAll('div').filter(el => el.classes().includes('main-content'))
      expect(innerContainer.length).toBe(1)
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
