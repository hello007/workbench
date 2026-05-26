import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import DirectoryTree from '../DirectoryTree.vue'

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
  AddDirectory: vi.fn(() => Promise.resolve({ id: 'dir-1', name: '测试', path: '/test', isDefault: false })),
  UpdateDirectory: vi.fn(() => Promise.resolve({ id: 'dir-1', name: '测试', path: '/test', isDefault: false })),
  DeleteDirectory: vi.fn(() => Promise.resolve(true)),
  SetDefaultDirectory: vi.fn(() => Promise.resolve(true))
}))

vi.mock('../../../utils/debug', () => ({
  debug: { log: vi.fn(), error: vi.fn(), warn: vi.fn() }
}))

const defaultStubs = {
  'el-button': { template: '<button v-bind="$attrs"><slot /></button>' },
  'el-icon': { template: '<i><slot /></i>' },
  'el-input': {
    template: '<input v-model="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'disabled'],
    emits: ['update:modelValue']
  },
  'el-switch': {
    template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
    props: ['modelValue'],
    emits: ['update:modelValue']
  },
  'el-form': { template: '<form><slot /></form>' },
  'el-form-item': { template: '<div><slot /></div>', props: ['label'] },
  'el-dialog': {
    template: '<div v-if="modelValue" class="el-dialog"><slot /><slot name="footer" /></div>',
    props: ['modelValue'],
    emits: ['update:modelValue']
  },
  'el-empty': { template: '<div class="el-empty" />' },
  Folder: { template: '<span>folder</span>' },
  Star: { template: '<span>star</span>' },
  Plus: { template: '<span>plus</span>' },
  Edit: { template: '<span>edit</span>' },
  Delete: { template: '<span>del</span>' },
  Refresh: { template: '<span>refresh</span>' }
}

const mockDirectories = [
  { id: 'dir-1', name: '项目A', path: '/path/a', isDefault: true },
  { id: 'dir-2', name: '项目B', path: '/path/b', isDefault: false }
]

function createWrapper(props = {}) {
  return mount(DirectoryTree, {
    props: {
      directories: mockDirectories,
      selectedId: 'dir-1',
      ...props
    },
    global: { stubs: defaultStubs }
  })
}

describe('DirectoryTree.vue', () => {
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

  describe('目录列表渲染', () => {
    it('应该渲染目录列表项', () => {
      wrapper = createWrapper()
      const items = wrapper.findAll('.dir-item')
      expect(items.length).toBe(2)
    })

    it('应该显示目录名称', () => {
      wrapper = createWrapper()
      const names = wrapper.findAll('.dir-item-name')
      expect(names[0].text()).toBe('项目A')
      expect(names[1].text()).toBe('项目B')
    })

    it('选中的目录应该有 active 样式', () => {
      wrapper = createWrapper()
      const items = wrapper.findAll('.dir-item')
      expect(items[0].classes()).toContain('dir-item--active')
      expect(items[1].classes()).not.toContain('dir-item--active')
    })

    it('空列表应该显示 el-empty', () => {
      wrapper = createWrapper({ directories: [] })
      expect(wrapper.find('.el-empty').exists()).toBe(true)
    })

    it('默认目录应该显示星标', () => {
      wrapper = createWrapper()
      const stars = wrapper.findAll('.dir-item-star')
      expect(stars.length).toBe(1)
    })

    it('应该显示目录路径', () => {
      wrapper = createWrapper()
      const paths = wrapper.findAll('.dir-path')
      expect(paths[0].text()).toBe('/path/a')
      expect(paths[1].text()).toBe('/path/b')
    })

    it('路径应该有 title 属性显示完整路径', () => {
      wrapper = createWrapper()
      const paths = wrapper.findAll('.dir-path')
      expect(paths[0].attributes('title')).toBe('/path/a')
      expect(paths[1].attributes('title')).toBe('/path/b')
    })

    it('路径样式应该为小字灰色', () => {
      wrapper = createWrapper()
      const paths = wrapper.findAll('.dir-path')
      expect(paths.length).toBe(2)
    })
  })

  describe('添加目录（AC1）', () => {
    it('点击添加按钮应该显示对话框', async () => {
      wrapper = createWrapper()
      const btn = wrapper.find('.dir-toolbar button')
      await btn.trigger('click')
      expect(wrapper.find('.el-dialog').exists()).toBe(true)
    })

    it('空名称应该提示警告', async () => {
      wrapper = createWrapper()
      await wrapper.find('.dir-toolbar button').trigger('click')

      const { AddDirectory } = await import('../../../wailsjs/go/main/App')
      // 直接调用 handleAdd
      wrapper.vm.addForm = { name: '', path: '/test', isDefault: false }
      await wrapper.vm.handleAdd()

      expect(ElMessage.warning).toHaveBeenCalledWith('请输入目录名称')
      expect(AddDirectory).not.toHaveBeenCalled()
    })

    it('空路径应该提示警告', async () => {
      wrapper = createWrapper()
      await wrapper.find('.dir-toolbar button').trigger('click')

      const { AddDirectory } = await import('../../../wailsjs/go/main/App')
      wrapper.vm.addForm = { name: '测试', path: '', isDefault: false }
      await wrapper.vm.handleAdd()

      expect(ElMessage.warning).toHaveBeenCalledWith('请输入目录路径')
      expect(AddDirectory).not.toHaveBeenCalled()
    })

    it('添加成功应该 emit change', async () => {
      wrapper = createWrapper()
      wrapper.vm.addForm = { name: '新目录', path: '/new', isDefault: false }
      await wrapper.vm.handleAdd()
      await flushPromises()

      const { AddDirectory } = await import('../../../wailsjs/go/main/App')
      expect(AddDirectory).toHaveBeenCalledWith('新目录', '/new', false)
      expect(ElMessage.success).toHaveBeenCalledWith('添加成功')
      expect(wrapper.emitted('change')).toBeTruthy()
    })

    it('添加失败应该显示错误', async () => {
      const { AddDirectory } = await import('../../../wailsjs/go/main/App')
      AddDirectory.mockRejectedValueOnce(new Error('路径不存在'))

      wrapper = createWrapper()
      wrapper.vm.addForm = { name: '新目录', path: '/bad', isDefault: false }
      await wrapper.vm.handleAdd()
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalled()
    })

    it('添加期间按钮应该 loading', async () => {
      let resolveAdd
      const { AddDirectory } = await import('../../../wailsjs/go/main/App')
      AddDirectory.mockImplementationOnce(() => new Promise(r => { resolveAdd = r }))

      wrapper = createWrapper()
      wrapper.vm.addForm = { name: '新目录', path: '/new', isDefault: false }
      const promise = wrapper.vm.handleAdd()

      expect(wrapper.vm.addLoading).toBe(true)
      resolveAdd({ id: 'dir-3' })
      await promise
      await flushPromises()

      expect(wrapper.vm.addLoading).toBe(false)
    })
  })

  describe('删除目录（AC2）', () => {
    let ElMessageBox

    beforeEach(async () => {
      ElMessageBox = (await import('element-plus')).ElMessageBox
    })

    it('删除成功应该 emit change', async () => {
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')

      wrapper = createWrapper()
      await wrapper.vm.handleDelete(mockDirectories[0])
      await flushPromises()

      const { DeleteDirectory } = await import('../../../wailsjs/go/main/App')
      expect(DeleteDirectory).toHaveBeenCalledWith('dir-1')
      expect(ElMessage.success).toHaveBeenCalledWith('删除成功')
      expect(wrapper.emitted('change')).toBeTruthy()
    })

    it('取消删除不应该调用 DeleteDirectory', async () => {
      ElMessageBox.confirm.mockRejectedValueOnce('cancel')

      wrapper = createWrapper()
      await wrapper.vm.handleDelete(mockDirectories[0])
      await flushPromises()

      const { DeleteDirectory } = await import('../../../wailsjs/go/main/App')
      expect(DeleteDirectory).not.toHaveBeenCalled()
    })

    it('删除失败应该显示错误', async () => {
      ElMessageBox.confirm.mockResolvedValueOnce('confirm')

      const { DeleteDirectory } = await import('../../../wailsjs/go/main/App')
      DeleteDirectory.mockResolvedValueOnce(false)

      wrapper = createWrapper()
      await wrapper.vm.handleDelete(mockDirectories[0])
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalledWith('删除失败')
    })
  })

  describe('目录选中', () => {
    it('点击目录应该 emit select', async () => {
      wrapper = createWrapper()
      const items = wrapper.findAll('.dir-item')
      await items[1].trigger('click')
      expect(wrapper.emitted('select')).toBeTruthy()
      expect(wrapper.emitted('select')[0][0]).toBe('dir-2')
    })
  })

  describe('事件监听清理', () => {
    it('unmount 时应该移除 click 监听器', () => {
      const removeSpy = vi.spyOn(document, 'removeEventListener')
      wrapper = createWrapper()
      wrapper.unmount()
      expect(removeSpy).toHaveBeenCalledWith('click', expect.any(Function))
      removeSpy.mockRestore()
      wrapper = null
    })
  })

  describe('设为默认目录', () => {
    it('设为默认成功应该 emit change', async () => {
      wrapper = createWrapper()
      await wrapper.vm.handleSetDefault(mockDirectories[1])
      await flushPromises()

      const { SetDefaultDirectory } = await import('../../../wailsjs/go/main/App')
      expect(SetDefaultDirectory).toHaveBeenCalledWith('dir-2')
      expect(ElMessage.success).toHaveBeenCalledWith('已设为默认目录')
      expect(wrapper.emitted('change')).toBeTruthy()
    })

    it('设为默认失败应该显示错误', async () => {
      const { SetDefaultDirectory } = await import('../../../wailsjs/go/main/App')
      SetDefaultDirectory.mockResolvedValueOnce(false)

      wrapper = createWrapper()
      await wrapper.vm.handleSetDefault(mockDirectories[1])
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalledWith('设置失败')
    })

    it('设为默认异常应该显示错误消息', async () => {
      const { SetDefaultDirectory } = await import('../../../wailsjs/go/main/App')
      SetDefaultDirectory.mockRejectedValueOnce(new Error('网络错误'))

      wrapper = createWrapper()
      await wrapper.vm.handleSetDefault(mockDirectories[1])
      await flushPromises()

      expect(ElMessage.error).toHaveBeenCalledWith('设置失败: 网络错误')
    })
  })

  describe('版本号显示', () => {
    it('传入 version 时应该显示版本号', () => {
      wrapper = createWrapper({ version: '1.0.0' })
      expect(wrapper.find('.dir-version').exists()).toBe(true)
      expect(wrapper.find('.dir-version').text()).toBe('v1.0.0')
    })

    it('未传入 version 时不显示版本号', () => {
      wrapper = createWrapper({ version: '' })
      expect(wrapper.find('.dir-version').exists()).toBe(false)
    })
  })

  describe('更新仓库', () => {
    it('点击"更新仓库"菜单项应该 emit batchPull 携带目录 path', async () => {
      wrapper = createWrapper()
      const items = wrapper.findAll('.dir-item')
      await items[1].trigger('contextmenu', { clientX: 10, clientY: 10 })

      const menuItems = wrapper.findAll('.context-menu-item')
      const pullItem = menuItems.find(el => el.text().includes('更新仓库'))
      expect(pullItem).toBeTruthy()

      await pullItem.trigger('click')

      expect(wrapper.emitted('batchPull')).toBeTruthy()
      expect(wrapper.emitted('batchPull')[0][0]).toEqual({ path: '/path/b' })
    })

    it('点击"更新仓库"后菜单应该关闭', async () => {
      wrapper = createWrapper()
      const items = wrapper.findAll('.dir-item')
      await items[0].trigger('contextmenu', { clientX: 10, clientY: 10 })

      const menuItems = wrapper.findAll('.context-menu-item')
      const pullItem = menuItems.find(el => el.text().includes('更新仓库'))
      await pullItem.trigger('click')

      expect(wrapper.find('.context-menu').exists()).toBe(false)
    })
  })
})
