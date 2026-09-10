import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import UpdateDialog from '../UpdateDialog.vue'
import { useUiStore } from '../../store'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  DownloadUpdate: vi.fn(),
  CancelDownload: vi.fn(),
  ApplyUpdate: vi.fn()
}))

// EventsOn 记录回调以便模拟下载进度事件
const progressHandlers = {}
vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((event, cb) => { progressHandlers[event] = cb }),
  EventsOff: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  CircleCheckFilled: { template: '<i class="i-circle-check" />' }
}))

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="update-dialog"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'closeOnPressEscape', 'append', 'class'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'loading', 'disabled', 'size']
  },
  'el-icon': { template: '<i><slot /></i>', props: ['size', 'color'] },
  'el-progress': { template: '<div class="el-progress" />', props: ['percentage', 'strokeWidth', 'format'] }
}

const updateInfo = (over = {}) => ({
  latestVer: '1.2.0',
  currentVer: '1.1.0',
  releaseNotes: '修复若干问题',
  fileSize: 2048,
  downloadUrl: 'https://example.com/dl.zip',
  ...over
})

function createWrapper() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const uiStore = useUiStore()
  uiStore.updateDialogVisible = true
  uiStore.updateInfo = updateInfo()
  return mount(UpdateDialog, { global: { stubs } })
}

describe('UpdateDialog.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    Object.keys(progressHandlers).forEach(k => delete progressHandlers[k])
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  it('打开时展示新版本信息与文件大小', () => {
    wrapper = createWrapper()
    expect(wrapper.find('.update-info').exists()).toBe(true)
    expect(wrapper.text()).toContain('v1.2.0')
    expect(wrapper.text()).toContain('v1.1.0')
    expect(wrapper.text()).toContain('2.0 KB')
  })

  it('文件大小为空时不渲染 meta 尺寸', async () => {
    wrapper = createWrapper()
    const uiStore = useUiStore()
    uiStore.updateInfo = updateInfo({ fileSize: 0 })
    await nextTick()
    expect(wrapper.find('.update-meta').exists()).toBe(true)
    // fileSize 为 0 → span 不渲染（v-if）
    expect(wrapper.find('.update-meta').text()).not.toContain('文件大小')
  })

  it('点击立即更新调用 DownloadUpdate 并切换下载中状态', async () => {
    const { DownloadUpdate } = await import('../../../wailsjs/go/main/App')
    DownloadUpdate.mockResolvedValue(true)
    wrapper = createWrapper()
    const buttons = wrapper.findAll('button')
    const updateBtn = buttons.find(b => b.text().includes('立即更新'))
    await updateBtn.trigger('click')
    await flushPromises()
    expect(DownloadUpdate).toHaveBeenCalledWith('https://example.com/dl.zip')
    expect(wrapper.find('.update-downloading').exists()).toBe(true)
  })

  it('下载地址无效时弹错误且不进入下载态', async () => {
    const { ElMessage } = await import('element-plus')
    const { DownloadUpdate } = await import('../../../wailsjs/go/main/App')
    wrapper = createWrapper()
    const uiStore = useUiStore()
    uiStore.updateInfo = updateInfo({ downloadUrl: '' })
    await nextTick()
    const updateBtn = wrapper.findAll('button').find(b => b.text().includes('立即更新'))
    await updateBtn.trigger('click')
    expect(ElMessage.error).toHaveBeenCalledWith('下载地址无效')
    expect(DownloadUpdate).not.toHaveBeenCalled()
  })

  it('DownloadUpdate 抛错时回退下载态并报错', async () => {
    const { ElMessage } = await import('element-plus')
    const { DownloadUpdate } = await import('../../../wailsjs/go/main/App')
    DownloadUpdate.mockRejectedValue(new Error('network down'))
    wrapper = createWrapper()
    const updateBtn = wrapper.findAll('button').find(b => b.text().includes('立即更新'))
    await updateBtn.trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('network down'))
    expect(wrapper.find('.update-downloading').exists()).toBe(false)
  })

  it('下载进度事件 completed 后切到下载完成态', async () => {
    const { DownloadUpdate } = await import('../../../wailsjs/go/main/App')
    DownloadUpdate.mockResolvedValue(true)
    wrapper = createWrapper()
    const updateBtn = wrapper.findAll('button').find(b => b.text().includes('立即更新'))
    await updateBtn.trigger('click')
    await flushPromises()
    // 模拟后端推送进度完成事件
    progressHandlers['update:download-progress']({
      totalBytes: 2048, downloaded: 2048, percent: 100, speed: '1 MB/s', completed: true
    })
    await nextTick()
    expect(wrapper.find('.update-done').exists()).toBe(true)
    expect(wrapper.text()).toContain('更新已下载完成')
    // 完成态按钮：稍后重启 / 立即重启
    const buttons = wrapper.findAll('button')
    expect(buttons.some(b => b.text().includes('立即重启'))).toBe(true)
  })

  it('点击取消下载调用 CancelDownload 并关闭弹窗', async () => {
    const { CancelDownload } = await import('../../../wailsjs/go/main/App')
    const { DownloadUpdate } = await import('../../../wailsjs/go/main/App')
    DownloadUpdate.mockResolvedValue(new Promise(() => {})) // 挂起，保持下载中
    wrapper = createWrapper()
    const updateBtn = wrapper.findAll('button').find(b => b.text().includes('立即更新'))
    await updateBtn.trigger('click')
    await flushPromises()
    const cancelBtn = wrapper.findAll('button').find(b => b.text().includes('取消下载'))
    await cancelBtn.trigger('click')
    expect(CancelDownload).toHaveBeenCalled()
    expect(useUiStore().updateDialogVisible).toBe(false)
  })

  it('立即重启调用 ApplyUpdate', async () => {
    const { ApplyUpdate } = await import('../../../wailsjs/go/main/App')
    const { DownloadUpdate } = await import('../../../wailsjs/go/main/App')
    DownloadUpdate.mockResolvedValue(true)
    wrapper = createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('立即更新')).trigger('click')
    await flushPromises()
    progressHandlers['update:download-progress']({ totalBytes: 1, downloaded: 1, percent: 100, speed: '', completed: true })
    await nextTick()
    await wrapper.findAll('button').find(b => b.text().includes('立即重启')).trigger('click')
    await flushPromises()
    expect(ApplyUpdate).toHaveBeenCalled()
  })

  it('ApplyUpdate 抛错时弹错误提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { ApplyUpdate, DownloadUpdate } = await import('../../../wailsjs/go/main/App')
    DownloadUpdate.mockResolvedValue(true)
    ApplyUpdate.mockRejectedValue(new Error('apply fail'))
    wrapper = createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('立即更新')).trigger('click')
    await flushPromises()
    progressHandlers['update:download-progress']({ totalBytes: 1, downloaded: 1, percent: 100, speed: '', completed: true })
    await nextTick()
    await wrapper.findAll('button').find(b => b.text().includes('立即重启')).trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('apply fail'))
  })

  it('稍后重启关闭弹窗', async () => {
    const { DownloadUpdate } = await import('../../../wailsjs/go/main/App')
    DownloadUpdate.mockResolvedValue(true)
    wrapper = createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('立即更新')).trigger('click')
    await flushPromises()
    progressHandlers['update:download-progress']({ totalBytes: 1, downloaded: 1, percent: 100, speed: '', completed: true })
    await nextTick()
    await wrapper.findAll('button').find(b => b.text().includes('稍后重启')).trigger('click')
    expect(useUiStore().updateDialogVisible).toBe(false)
  })

  it('稍后再说关闭弹窗', async () => {
    wrapper = createWrapper()
    await wrapper.findAll('button').find(b => b.text().includes('稍后再说')).trigger('click')
    expect(useUiStore().updateDialogVisible).toBe(false)
  })

  it('卸载时注销下载进度事件监听', async () => {
    const { EventsOff } = await import('../../../wailsjs/runtime/runtime')
    wrapper = createWrapper()
    wrapper.unmount()
    wrapper = null
    expect(EventsOff).toHaveBeenCalledWith('update:download-progress')
  })

  it('formatSize 跨单位换算（KB/MB/GB）', async () => {
    wrapper = createWrapper()
    // 2048 -> 2.0 KB（已在文案断言），用更大值覆盖 MB 分支
    const uiStore = useUiStore()
    uiStore.updateInfo = updateInfo({ fileSize: 1048576 }) // 1 MB
    await nextTick()
    expect(wrapper.text()).toContain('1.0 MB')
  })
})
