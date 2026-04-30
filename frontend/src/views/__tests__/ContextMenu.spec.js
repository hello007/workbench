/**
 * 右键菜单功能测试
 * 覆盖：onNodeContextMenu、onMenuCommand 分发、重命名、删除、复制、资源管理器打开
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  RenameFile, DeleteFile, OpenInExplorer
} from '../../../wailsjs/go/main/App'
import Home from '../Home.vue'

// Mock Wails 绑定
vi.mock('../../../wailsjs/go/main/App', () => ({
  GetDirectories: vi.fn().mockResolvedValue([]),
  GetFileTree: vi.fn().mockResolvedValue([]),
  CreateDirectory: vi.fn().mockResolvedValue(true),
  CreateFile: vi.fn().mockResolvedValue(true),
  RenameFile: vi.fn().mockResolvedValue(true),
  DeleteFile: vi.fn().mockResolvedValue(true),
  PreviewFile: vi.fn().mockResolvedValue({ content: '' }),
  PullRepo: vi.fn().mockResolvedValue('拉取完成'),
  CloneRepo: vi.fn().mockResolvedValue('克隆成功'),
  OpenInExplorer: vi.fn().mockResolvedValue(true)
}))

// Mock Element Plus
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
      confirm: vi.fn().mockResolvedValue(true)
    }
  }
})

// Mock debug工具
vi.mock('../../utils/debug', () => ({
  debug: {
    log: vi.fn(),
    error: vi.fn(),
    warn: vi.fn()
  }
}))

// Mock clipboard API
Object.assign(navigator, {
  clipboard: {
    writeText: vi.fn().mockResolvedValue(undefined)
  }
})

const mountHome = () => mount(Home, {
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
      'GitInfo': true,
      'CommitHistory': true,
      'el-tabs': true,
      'el-tab-pane': true
    }
  }
})

// 模拟 MouseEvent
const createMouseEvent = (x = 100, y = 200) => ({
  clientX: x,
  clientY: y,
  preventDefault: vi.fn(),
  stopPropagation: vi.fn()
})

describe('右键菜单 - onNodeContextMenu 显示菜单', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    wrapper = mountHome()
  })

  afterEach(() => {
    wrapper.unmount()
  })

  it('右键节点应显示菜单并记录坐标和数据', () => {
    const event = createMouseEvent(150, 250)
    const data = { name: 'src', path: '/project/src', type: 'directory' }

    wrapper.vm.onNodeContextMenu(event, data)

    expect(wrapper.vm.contextMenu.visible).toBe(true)
    expect(wrapper.vm.contextMenu.x).toBe(150)
    expect(wrapper.vm.contextMenu.y).toBe(250)
    expect(wrapper.vm.contextMenu.data).toEqual(data)
  })

  it('应阻止默认行为和冒泡', () => {
    const event = createMouseEvent()
    const data = { name: 'test.txt', path: '/test.txt', type: 'file' }

    wrapper.vm.onNodeContextMenu(event, data)

    expect(event.preventDefault).toHaveBeenCalled()
    expect(event.stopPropagation).toHaveBeenCalled()
  })
})

describe('右键菜单 - closeContextMenu 关闭菜单', () => {
  it('应隐藏菜单', () => {
    const wrapper = mountHome()
    wrapper.vm.contextMenu.visible = true
    wrapper.vm.contextMenu.data = { name: 'test' }

    wrapper.vm.closeContextMenu()

    expect(wrapper.vm.contextMenu.visible).toBe(false)
  })
})

describe('右键菜单 - onMenuCommand 分发', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    wrapper = mountHome()
  })

  afterEach(() => {
    wrapper.unmount()
  })

  it('createFile 命令应设置 selectedNode 并打开创建文件对话框', () => {
    const dirData = { name: 'src', path: '/project/src', type: 'directory' }
    wrapper.vm.contextMenu.data = dirData

    wrapper.vm.onMenuCommand('createFile')

    expect(wrapper.vm.selectedNode).toEqual(dirData)
    expect(wrapper.vm.createType).toBe('file')
    expect(wrapper.vm.createName).toBe('')
    expect(wrapper.vm.createDialogVisible).toBe(true)
    expect(wrapper.vm.contextMenu.visible).toBe(false)
  })

  it('createDir 命令应设置 selectedNode 并打开创建文件夹对话框', () => {
    const dirData = { name: 'src', path: '/project/src', type: 'directory' }
    wrapper.vm.contextMenu.data = dirData

    wrapper.vm.onMenuCommand('createDir')

    expect(wrapper.vm.selectedNode).toEqual(dirData)
    expect(wrapper.vm.createType).toBe('directory')
    expect(wrapper.vm.createDialogVisible).toBe(true)
  })

  it('rename 命令应打开重命名对话框并预填名称', () => {
    const fileData = { name: 'old.txt', path: '/project/old.txt', type: 'file' }
    wrapper.vm.contextMenu.data = fileData

    wrapper.vm.onMenuCommand('rename')

    expect(wrapper.vm.selectedNode).toEqual(fileData)
    expect(wrapper.vm.renameName).toBe('old.txt')
    expect(wrapper.vm.renameDialogVisible).toBe(true)
  })

  it('delete 命令应触发删除确认', async () => {
    const fileData = { name: 'test.txt', path: '/project/test.txt', type: 'file' }
    wrapper.vm.contextMenu.data = fileData

    wrapper.vm.onMenuCommand('delete')
    await vi.waitFor(() => expect(ElMessageBox.confirm).toHaveBeenCalled())

    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      '确定要删除 "test.txt" 吗？此操作不可撤销。',
      '警告',
      expect.objectContaining({ type: 'warning' })
    )
  })

  it('copyPath 命令应复制完整路径', async () => {
    const fileData = { name: 'test.txt', path: '/project/test.txt', type: 'file' }
    wrapper.vm.contextMenu.data = fileData

    wrapper.vm.onMenuCommand('copyPath')
    await vi.waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith('/project/test.txt'))
  })

  it('copyName 命令应仅复制文件名', async () => {
    const fileData = { name: 'test.txt', path: '/project/test.txt', type: 'file' }
    wrapper.vm.contextMenu.data = fileData

    wrapper.vm.onMenuCommand('copyName')
    await vi.waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith('test.txt'))
  })

  it('openExplorer 命令应调用 OpenInExplorer', async () => {
    const dirData = { name: 'project', path: '/project', type: 'directory' }
    wrapper.vm.contextMenu.data = dirData

    wrapper.vm.onMenuCommand('openExplorer')
    await vi.waitFor(() => expect(OpenInExplorer).toHaveBeenCalledWith('/project'))
  })

  it('data 为 null 时不应执行任何操作', () => {
    wrapper.vm.contextMenu.data = null

    wrapper.vm.onMenuCommand('delete')

    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
  })
})

describe('右键菜单 - copyToClipboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('成功复制应显示成功提示', async () => {
    const wrapper = mountHome()

    await wrapper.vm.copyToClipboard('/some/path', '路径')

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('/some/path')
    expect(ElMessage.success).toHaveBeenCalledWith('路径已复制到剪贴板')
  })

  it('复制失败应显示错误提示', async () => {
    navigator.clipboard.writeText = vi.fn().mockRejectedValue(new Error('denied'))
    const wrapper = mountHome()

    await wrapper.vm.copyToClipboard('/some/path', '路径')

    expect(ElMessage.error).toHaveBeenCalledWith('复制失败')
  })
})

describe('右键菜单 - handleOpenExplorer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('成功打开不显示错误', async () => {
    const wrapper = mountHome()

    await wrapper.vm.handleOpenExplorer('/project')

    expect(OpenInExplorer).toHaveBeenCalledWith('/project')
    expect(ElMessage.error).not.toHaveBeenCalled()
  })

  it('后端返回 false 应显示错误', async () => {
    OpenInExplorer.mockResolvedValue(false)
    const wrapper = mountHome()

    await wrapper.vm.handleOpenExplorer('/project')

    expect(ElMessage.error).toHaveBeenCalledWith('打开资源管理器失败')
  })

  it('后端抛出异常应显示错误', async () => {
    OpenInExplorer.mockRejectedValue(new Error('exec error'))
    const wrapper = mountHome()

    await wrapper.vm.handleOpenExplorer('/project')

    expect(ElMessage.error).toHaveBeenCalledWith('打开资源管理器失败: exec error')
  })
})

describe('右键菜单 - handleDeleteAt', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('用户取消确认不应执行删除', async () => {
    ElMessageBox.confirm.mockRejectedValue('cancel')
    const wrapper = mountHome()

    const data = { name: 'test.txt', path: '/project/test.txt' }
    await wrapper.vm.handleDeleteAt(data)

    expect(DeleteFile).not.toHaveBeenCalled()
  })

  it('确认删除应调用 DeleteFile', async () => {
    ElMessageBox.confirm.mockResolvedValue(true)
    const wrapper = mountHome()

    const data = { name: 'test.txt', path: '/project/test.txt' }
    await wrapper.vm.handleDeleteAt(data)

    expect(DeleteFile).toHaveBeenCalledWith('/project/test.txt')
  })

  it('删除成功应清空 selectedNode 并显示成功', async () => {
    ElMessageBox.confirm.mockResolvedValue(true)
    const wrapper = mountHome()

    const data = { name: 'test.txt', path: '/project/test.txt' }
    wrapper.vm.selectedNode = data
    await wrapper.vm.handleDeleteAt(data)

    expect(wrapper.vm.selectedNode).toBeNull()
    expect(ElMessage.success).toHaveBeenCalledWith('删除成功')
  })

  it('DeleteFile 抛出异常应显示错误', async () => {
    ElMessageBox.confirm.mockResolvedValue(true)
    DeleteFile.mockRejectedValue(new Error('permission denied'))
    const wrapper = mountHome()

    const data = { name: 'test.txt', path: '/project/test.txt' }
    await wrapper.vm.handleDeleteAt(data)

    expect(ElMessage.error).toHaveBeenCalledWith('删除失败: permission denied')
  })
})

describe('右键菜单 - handleRename', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('空名称应显示警告且不调用 RenameFile', async () => {
    const wrapper = mountHome()
    wrapper.vm.renameName = ''
    wrapper.vm.selectedNode = { name: 'old.txt', path: '/old.txt' }

    await wrapper.vm.handleRename()

    expect(ElMessage.warning).toHaveBeenCalledWith('请输入名称')
    expect(RenameFile).not.toHaveBeenCalled()
  })

  it('无 selectedNode 应直接返回', async () => {
    const wrapper = mountHome()
    wrapper.vm.selectedNode = null
    wrapper.vm.renameName = 'new.txt'

    await wrapper.vm.handleRename()

    expect(RenameFile).not.toHaveBeenCalled()
  })

  it('重命名成功应关闭对话框并显示成功', async () => {
    const wrapper = mountHome()
    wrapper.vm.selectedNode = { name: 'old.txt', path: '/project/old.txt' }
    wrapper.vm.renameName = 'new.txt'
    wrapper.vm.renameDialogVisible = true
    wrapper.vm.refreshNode = vi.fn()

    await wrapper.vm.handleRename()

    expect(RenameFile).toHaveBeenCalledWith('/project/old.txt', 'new.txt')
    expect(wrapper.vm.renameDialogVisible).toBe(false)
    expect(ElMessage.success).toHaveBeenCalledWith('重命名成功')
  })

  it('重命名失败应显示错误', async () => {
    RenameFile.mockResolvedValue(false)
    const wrapper = mountHome()
    wrapper.vm.selectedNode = { name: 'old.txt', path: '/old.txt' }
    wrapper.vm.renameName = 'new.txt'

    await wrapper.vm.handleRename()

    expect(ElMessage.error).toHaveBeenCalledWith('重命名失败')
  })

  it('RenameFile 抛出异常应显示错误', async () => {
    RenameFile.mockRejectedValue(new Error('rename error'))
    const wrapper = mountHome()
    wrapper.vm.selectedNode = { name: 'old.txt', path: '/old.txt' }
    wrapper.vm.renameName = 'new.txt'

    await wrapper.vm.handleRename()

    expect(ElMessage.error).toHaveBeenCalledWith('重命名失败: rename error')
  })
})

describe('右键菜单 - showRenameDialogAt', () => {
  it('应设置 renameName 为当前节点名称并打开对话框', () => {
    const wrapper = mountHome()
    const data = { name: 'my-file.go', path: '/project/my-file.go' }

    wrapper.vm.showRenameDialogAt(data)

    expect(wrapper.vm.renameName).toBe('my-file.go')
    expect(wrapper.vm.renameDialogVisible).toBe(true)
    expect(wrapper.vm.selectedNode).toEqual(data)
  })
})

describe('右键菜单 - deleteFile 复用 handleDeleteAt', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('deleteFile 应委托给 handleDeleteAt', async () => {
    ElMessageBox.confirm.mockResolvedValue(true)
    const wrapper = mountHome()
    wrapper.vm.selectedNode = { name: 'test.txt', path: '/project/test.txt' }

    await wrapper.vm.deleteFile()

    expect(DeleteFile).toHaveBeenCalledWith('/project/test.txt')
  })

  it('无 selectedNode 时 deleteFile 不执行', async () => {
    const wrapper = mountHome()
    wrapper.vm.selectedNode = null

    await wrapper.vm.deleteFile()

    expect(DeleteFile).not.toHaveBeenCalled()
  })
})
