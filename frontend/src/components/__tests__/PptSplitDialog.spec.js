import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PptSplitDialog from '../PptSplitDialog.vue'
import { SplitPptxPerPage, CancelPptxSplit, OpenFileDialog } from '../../../wailsjs/go/main/App'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  SplitPptxPerPage: vi.fn(() =>
    Promise.resolve({ outputDir: 'D:/docs/汇报', files: ['D:/docs/汇报/汇报-01.pptx'], pageCount: 1 })
  ),
  CancelPptxSplit: vi.fn(() => Promise.resolve(true)),
  OpenFileDialog: vi.fn(() => Promise.resolve(''))
}))

// 捕获 EventsOn 注册的事件回调：用例内直接调回调模拟后端事件推送；
// off 闭包从 eventHandlers 删除对应项，可断言注销行为（精准注销非 EventsOff 全局移除）
const eventHandlers = {}
let offFns = []
vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((name, cb) => {
    eventHandlers[name] = cb
    const off = vi.fn(() => { delete eventHandlers[name] })
    offFns.push(off)
    return off
  }),
  EventsOff: vi.fn()
}))

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="ps-dialog"><slot name="header" /><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'width'],
    emits: ['update:modelValue']
  },
  'el-input': {
    template: '<input :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" @keyup="$emit(\'keyup\', $event)" />',
    props: ['modelValue', 'placeholder', 'disabled'],
    emits: ['update:modelValue', 'keyup']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'type', 'loading'],
    emits: ['click']
  }
}

const createWrapper = (visible = true) =>
  mount(PptSplitDialog, { props: { modelValue: visible }, global: { stubs } })

const setInput = async (wrapper, value) => {
  await wrapper.find('input').setValue(value)
}

const clickButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text() === text).trigger('click')

describe('PptSplitDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // 恢复默认实现：个别用例的 mockRejectedValue/mockReturnValue（不带 Once）
    // 会永久覆盖 implementation，须逐用例重置防污染后续用例
    SplitPptxPerPage.mockImplementation(() =>
      Promise.resolve({ outputDir: 'D:/docs/汇报', files: ['D:/docs/汇报/汇报-01.pptx'], pageCount: 1 })
    )
    CancelPptxSplit.mockImplementation(() => Promise.resolve(true))
    OpenFileDialog.mockImplementation(() => Promise.resolve(''))
    for (const key of Object.keys(eventHandlers)) delete eventHandlers[key]
    offFns = []
  })

  it('打开对话框注册 pptx-split 三个事件监听（progress/done/error）', () => {
    createWrapper()
    expect(Object.keys(eventHandlers)).toEqual(
      expect.arrayContaining(['pptx-split:progress', 'pptx-split:done', 'pptx-split:error'])
    )
  })

  it('关闭对话框经闭包精准注销监听器（调用 off 闭包而非 EventsOff）', async () => {
    const { EventsOff } = await import('../../../wailsjs/runtime/runtime')
    const wrapper = createWrapper(true)
    const offCount = offFns.length
    expect(offCount).toBe(3)

    await wrapper.setProps({ modelValue: false })
    await flushPromises()

    // 三监听器各自注销且事件表清空；未走 EventsOff 全局移除
    expect(offFns.filter((off) => off.mock.calls.length > 0).length).toBe(3)
    expect(Object.keys(eventHandlers)).toHaveLength(0)
    expect(EventsOff).not.toHaveBeenCalled()
  })

  it('组件卸载时注销全部监听器', () => {
    const wrapper = createWrapper(true)
    wrapper.unmount()
    expect(offFns.filter((off) => off.mock.calls.length > 0).length).toBe(3)
    expect(Object.keys(eventHandlers)).toHaveLength(0)
  })

  it('路径为空点拆分 warning 且不发请求', async () => {
    const { ElMessage } = await import('element-plus')
    const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
    const wrapper = createWrapper()
    await clickButton(wrapper, '开始拆分')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入 PPT 文件路径')
    expect(SplitPptxPerPage).not.toHaveBeenCalled()
  })

  it('非 .pptx 后缀 warning 且不发请求（.ppt 老格式前端拒绝）', async () => {
    const { ElMessage } = await import('element-plus')
    const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.ppt')
    await clickButton(wrapper, '开始拆分')
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('仅支持 .pptx'))
    expect(SplitPptxPerPage).not.toHaveBeenCalled()
  })

  it('拆分成功：以输入路径调用后端并展示输出目录与文件列表', async () => {
    const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    expect(SplitPptxPerPage).toHaveBeenCalledWith('D:/docs/汇报.pptx')
    const result = wrapper.find('.ps-result')
    expect(result.exists()).toBe(true)
    expect(result.text()).toContain('已拆分 1 页')
    expect(result.text()).toContain('D:/docs/汇报')
    expect(wrapper.find('.ps-file-item').text()).toBe('D:/docs/汇报/汇报-01.pptx')
    // 拆分结束后按钮 loading 复位
    expect(wrapper.find('.ps-progress').exists()).toBe(false)
  })

  it('拆分中展示进度（progress 事件按 srcPath 过滤更新）', async () => {
    const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
    // Promise 永挂模拟在途拆分，事件先到
    SplitPptxPerPage.mockReturnValue(new Promise(() => {}))
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    expect(wrapper.find('.ps-progress').text()).toContain('准备中')
    eventHandlers['pptx-split:progress']({ srcPath: 'D:/docs/汇报.pptx', done: 2, total: 5 })
    await flushPromises()
    expect(wrapper.find('.ps-progress').text()).toContain('2 / 5 页')

    // 非本实例 srcPath 的事件不更新进度（多入口共听过滤）
    eventHandlers['pptx-split:progress']({ srcPath: 'D:/docs/其他.pptx', done: 9, total: 9 })
    await flushPromises()
    expect(wrapper.find('.ps-progress').text()).toContain('2 / 5 页')
  })

  it('done 事件先于 Promise 到达时写入结果并复位在途态', async () => {
    const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
    SplitPptxPerPage.mockReturnValue(new Promise(() => {}))
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    eventHandlers['pptx-split:done']({
      srcPath: 'D:/docs/汇报.pptx',
      result: { outputDir: 'D:/docs/汇报', files: ['D:/docs/汇报/汇报-01.pptx', 'D:/docs/汇报/汇报-02.pptx'], pageCount: 2 }
    })
    await flushPromises()

    expect(wrapper.find('.ps-result').text()).toContain('已拆分 2 页')
    expect(wrapper.findAll('.ps-file-item').length).toBe(2)
    expect(wrapper.find('.ps-progress').exists()).toBe(false)
  })

  it('error 事件仅复位在途态不弹提示（提示由 Promise reject 的 handleError 统一分流）', async () => {
    const { ElMessage } = await import('element-plus')
    const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
    SplitPptxPerPage.mockReturnValue(new Promise(() => {}))
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()
    expect(wrapper.find('.ps-progress').exists()).toBe(true)

    eventHandlers['pptx-split:error']({ srcPath: 'D:/docs/汇报.pptx', message: '操作已取消' })
    await flushPromises()

    expect(wrapper.find('.ps-progress').exists()).toBe(false)
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(ElMessage.warning).not.toHaveBeenCalled()
  })

  it('取消错误经 handleError 分流为 warning 中性提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
    SplitPptxPerPage.mockRejectedValue({ code: 'E_GIT_CANCELLED', message: '操作已取消' })
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('操作已取消')
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(wrapper.find('.ps-progress').exists()).toBe(false)
  })

  it('真失败经 handleError 分流为 error（带前缀）', async () => {
    const { ElMessage } = await import('element-plus')
    const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
    SplitPptxPerPage.mockRejectedValue({ code: 'E_REPO_CONFIG_INVALID_JSON', message: '文件损坏' })
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith('PPT 拆分失败: 文件损坏')
    expect(ElMessage.warning).not.toHaveBeenCalled()
  })

  it('取消拆分按钮调用 CancelPptxSplit', async () => {
    const { CancelPptxSplit } = await import('../../../wailsjs/go/main/App')
    const { SplitPptxPerPage } = await import('../../../wailsjs/go/main/App')
    SplitPptxPerPage.mockReturnValue(new Promise(() => {}))
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    await clickButton(wrapper, '取消拆分')
    expect(CancelPptxSplit).toHaveBeenCalled()
  })

  it('关闭对话框重置输入与结果（防旧仓库结果串入下次打开）', async () => {
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()
    expect(wrapper.find('.ps-result').exists()).toBe(true)

    await wrapper.setProps({ modelValue: false })
    await flushPromises()
    await wrapper.setProps({ modelValue: true })
    await flushPromises()

    expect(wrapper.find('input').element.value).toBe('')
    expect(wrapper.find('.ps-result').exists()).toBe(false)
  })

  it('浏览按钮选中文件回填路径（返回数组取首个）', async () => {
    const { OpenFileDialog } = await import('../../../wailsjs/go/main/App')
    OpenFileDialog.mockResolvedValue(['D:/docs/选择.pptx'])
    const wrapper = createWrapper()
    await clickButton(wrapper, '浏览')
    await flushPromises()
    expect(OpenFileDialog).toHaveBeenCalledWith('选择 PPT 文件', [
      { DisplayName: 'PowerPoint 演示文稿', Pattern: '*.pptx' }
    ])
    expect(wrapper.find('input').element.value).toBe('D:/docs/选择.pptx')
  })

  it('浏览按钮取消选择（返回空串）不覆盖已有输入', async () => {
    const { OpenFileDialog } = await import('../../../wailsjs/go/main/App')
    OpenFileDialog.mockResolvedValue('')
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/已有.pptx')
    await clickButton(wrapper, '浏览')
    await flushPromises()
    expect(wrapper.find('input').element.value).toBe('D:/docs/已有.pptx')
  })
})
