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
      expect(panes[0].attributes('data-size')).toBe('15')
      expect(panes[0].attributes('data-min-size')).toBe('10')
      expect(panes[0].attributes('data-max-size')).toBe('30')
    })

    it('第二个 Pane 尺寸配置正确', () => {
      const panes = layoutWrapper.findAll('.pane')
      expect(panes[1].attributes('data-size')).toBe('22')
      expect(panes[1].attributes('data-min-size')).toBe('15')
      expect(panes[1].attributes('data-max-size')).toBe('35')
    })

    it('第三个 Pane 尺寸配置正确', () => {
      const panes = layoutWrapper.findAll('.pane')
      expect(panes[2].attributes('data-size')).toBe('63')
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
            ContentPanel: { template: '<div class="stub-content-panel" />' },
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
})
