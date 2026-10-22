import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PptSplitPanel from '../PptSplitPanel.vue'
import { SplitPptxPerPage, SplitPptxFolder, CancelPptxSplit, OpenFileDialog, OpenDirectoryDialog, OpenInExplorer } from '../../../wailsjs/go/main/App'

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
  SplitPptxFolder: vi.fn(() => Promise.resolve({ dirPath: 'D:/docs', total: 2, items: [] })),
  CancelPptxSplit: vi.fn(() => Promise.resolve(true)),
  OpenFileDialog: vi.fn(() => Promise.resolve('')),
  OpenDirectoryDialog: vi.fn(() => Promise.resolve('')),
  OpenInExplorer: vi.fn(() => Promise.resolve(true))
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
  // 模式切换 stub：真 EP 组件由 radio-group 统一接管 v-model 与 disabled 透传，
  // slot 内 radio-button 的 emit 不会被 group 模板捕获，走 provide/inject 桥接
  'el-radio-group': {
    template: '<div class="ps-mode-switch" :class="{ \'is-disabled\': disabled }"><slot /></div>',
    props: ['modelValue', 'size', 'disabled'],
    emits: ['update:modelValue'],
    provide() {
      return { radioGroupStub: this }
    },
    methods: {
      select(value) {
        this.$emit('update:modelValue', value)
      }
    }
  },
  'el-radio-button': {
    template: '<label class="ps-mode-option" :data-value="value" :data-disabled="isDisabled ? \'1\' : \'0\'" @click="select"><slot /></label>',
    props: ['value', 'disabled'],
    inject: { radioGroupStub: { default: null } },
    computed: {
      isDisabled() {
        return !!this.disabled || !!(this.radioGroupStub && this.radioGroupStub.disabled)
      }
    },
    methods: {
      select() {
        if (this.isDisabled) return
        if (this.radioGroupStub) this.radioGroupStub.select(this.value)
        else this.$emit('update:modelValue', this.value)
      }
    }
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
  },
  'el-tag': { template: '<span class="tag"><slot /></span>', props: ['type', 'size'] }
}

const createWrapper = () => mount(PptSplitPanel, { global: { stubs } })

// 切到文件夹批量模式
const switchToFolderMode = async (wrapper) => {
  await wrapper.find('.ps-mode-option[data-value="folder"]').trigger('click')
}

const setInput = async (wrapper, value) => {
  await wrapper.find('input').setValue(value)
}

const clickButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text() === text).trigger('click')

describe('PptSplitPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // 恢复默认实现：个别用例的 mockRejectedValue/mockReturnValue（不带 Once）
    // 会永久覆盖 implementation，须逐用例重置防污染后续用例
    SplitPptxPerPage.mockImplementation(() =>
      Promise.resolve({ outputDir: 'D:/docs/汇报', files: ['D:/docs/汇报/汇报-01.pptx'], pageCount: 1 })
    )
    SplitPptxFolder.mockImplementation(() => Promise.resolve({ dirPath: 'D:/docs', total: 2, items: [] }))
    CancelPptxSplit.mockImplementation(() => Promise.resolve(true))
    OpenFileDialog.mockImplementation(() => Promise.resolve(''))
    OpenDirectoryDialog.mockImplementation(() => Promise.resolve(''))
    OpenInExplorer.mockImplementation(() => Promise.resolve(true))
    for (const key of Object.keys(eventHandlers)) delete eventHandlers[key]
    offFns = []
  })

  // ==================== 事件监听生命周期 ====================

  it('挂载注册 pptx-split 四个事件监听（progress/done/error/batch:progress）', () => {
    createWrapper()
    expect(Object.keys(eventHandlers)).toEqual(
      expect.arrayContaining([
        'pptx-split:progress',
        'pptx-split:done',
        'pptx-split:error',
        'pptx-split:batch:progress'
      ])
    )
  })

  it('组件卸载时闭包精准注销全部监听器（不调 EventsOff）', async () => {
    const { EventsOff } = await import('../../../wailsjs/runtime/runtime')
    const wrapper = createWrapper()
    expect(offFns.length).toBe(4)

    wrapper.unmount()

    expect(offFns.filter((off) => off.mock.calls.length > 0).length).toBe(4)
    expect(Object.keys(eventHandlers)).toHaveLength(0)
    expect(EventsOff).not.toHaveBeenCalled()
  })

  // ==================== 单文件模式（原弹窗用例迁移） ====================

  it('默认单文件模式；路径为空点拆分 warning 且不发请求', async () => {
    const { ElMessage } = await import('element-plus')
    const wrapper = createWrapper()
    expect(wrapper.text()).toContain('PPT 路径')
    expect(wrapper.findAll('button').some((b) => b.text() === '开始拆分')).toBe(true)
    await clickButton(wrapper, '开始拆分')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入 PPT 文件路径')
    expect(SplitPptxPerPage).not.toHaveBeenCalled()
  })

  it('非 .pptx 后缀 warning 且不发请求（.ppt 老格式前端拒绝）', async () => {
    const { ElMessage } = await import('element-plus')
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.ppt')
    await clickButton(wrapper, '开始拆分')
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('仅支持 .pptx'))
    expect(SplitPptxPerPage).not.toHaveBeenCalled()
  })

  it('单文件拆分成功：以输入路径调用后端并展示输出目录与文件列表', async () => {
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
    // 拆分结束后进度区复位
    expect(wrapper.find('.ps-progress').exists()).toBe(false)
  })

  it('拆分成功后「打开文件夹」按钮调 OpenInExplorer 传入输出目录', async () => {
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    await clickButton(wrapper, '打开文件夹')
    await flushPromises()
    expect(OpenInExplorer).toHaveBeenCalledWith('D:/docs/汇报')
  })

  it('打开输出目录失败（返回 false）warning 中性提示（不 error）', async () => {
    const { ElMessage } = await import('element-plus')
    OpenInExplorer.mockResolvedValue(false)
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    await clickButton(wrapper, '打开文件夹')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('打开输出文件夹失败')
    expect(ElMessage.error).not.toHaveBeenCalled()
  })

  it('单文件拆分中展示页级进度（progress 事件按 srcPath 过滤更新）', async () => {
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

  it('单文件取消错误经 handleError 分流为 warning 中性提示', async () => {
    const { ElMessage } = await import('element-plus')
    SplitPptxPerPage.mockRejectedValue({ code: 'E_GIT_CANCELLED', message: '操作已取消' })
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('操作已取消')
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(wrapper.find('.ps-progress').exists()).toBe(false)
  })

  it('单文件真失败经 handleError 分流为 error（带前缀）', async () => {
    const { ElMessage } = await import('element-plus')
    SplitPptxPerPage.mockRejectedValue({ code: 'E_REPO_CONFIG_INVALID_JSON', message: '文件损坏' })
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith('PPT 拆分失败: 文件损坏')
    expect(ElMessage.warning).not.toHaveBeenCalled()
  })

  it('取消拆分按钮调用 CancelPptxSplit', async () => {
    SplitPptxPerPage.mockReturnValue(new Promise(() => {}))
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/汇报.pptx')
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    await clickButton(wrapper, '取消拆分')
    expect(CancelPptxSplit).toHaveBeenCalled()
  })

  it('浏览按钮选中文件回填路径（返回数组取首个）', async () => {
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
    OpenFileDialog.mockResolvedValue('')
    const wrapper = createWrapper()
    await setInput(wrapper, 'D:/docs/已有.pptx')
    await clickButton(wrapper, '浏览')
    await flushPromises()
    expect(wrapper.find('input').element.value).toBe('D:/docs/已有.pptx')
  })

  // ==================== 文件夹批量模式（PR3） ====================

  it('切换到批量模式渲染目录输入与批量按钮', async () => {
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    expect(wrapper.text()).toContain('文件夹批量拆分')
    expect(wrapper.find('input').exists()).toBe(true)
    expect(wrapper.findAll('button').some((b) => b.text() === '开始批量拆分')).toBe(true)
  })

  it('批量目录为空 warning 不发请求', async () => {
    const { ElMessage } = await import('element-plus')
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await clickButton(wrapper, '开始批量拆分')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入要拆分的文件夹路径')
    expect(SplitPptxFolder).not.toHaveBeenCalled()
  })

  it('批量拆分成功：Promise 返回值渲染汇总计数与逐文件明细', async () => {
    SplitPptxFolder.mockResolvedValue({
      dirPath: 'D:/docs',
      total: 2,
      items: [
        { srcPath: 'D:/docs/汇报.pptx', outputDir: 'D:/docs/汇报', pageCount: 12, error: '' },
        { srcPath: 'D:/docs/年报.pptx', outputDir: 'D:/docs/年报', pageCount: 3, error: '' }
      ]
    })
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await setInput(wrapper, 'D:/docs')
    await clickButton(wrapper, '开始批量拆分')
    await flushPromises()

    expect(SplitPptxFolder).toHaveBeenCalledWith('D:/docs')
    const result = wrapper.find('.ps-result')
    expect(result.text()).toContain('共 2 / 2 个文件处理完成')
    expect(result.text()).toContain('成功 2 个、失败 0 个')
    // 明细行：文件名 + 页数 + 输出目录
    const rows = wrapper.findAll('.ps-batch-item')
    expect(rows.length).toBe(2)
    expect(rows[0].text()).toContain('汇报.pptx')
    expect(rows[0].text()).toContain('12 页')
    expect(rows[0].text()).toContain('D:/docs/汇报')
    // 全部成功 success 提示
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('批量拆分完成：2 个文件全部成功')
  })

  it('批量明细行仅成功项有「打开文件夹」入口且传入各自 outputDir', async () => {
    SplitPptxFolder.mockResolvedValue({
      dirPath: 'D:/docs',
      total: 2,
      items: [
        { srcPath: 'D:/docs/汇报.pptx', outputDir: 'D:/docs/汇报', pageCount: 12, error: '' },
        { srcPath: 'D:/docs/损坏.pptx', outputDir: '', pageCount: 0, error: 'zip 损坏' }
      ]
    })
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await setInput(wrapper, 'D:/docs')
    await clickButton(wrapper, '开始批量拆分')
    await flushPromises()

    const rows = wrapper.findAll('.ps-batch-item')
    // 成功项有入口，失败项（outputDir 空）无入口
    expect(rows[0].findAll('button').some((b) => b.text() === '打开文件夹')).toBe(true)
    expect(rows[1].findAll('button').some((b) => b.text() === '打开文件夹')).toBe(false)
    await rows[0].findAll('button').find((b) => b.text() === '打开文件夹').trigger('click')
    await flushPromises()
    expect(OpenInExplorer).toHaveBeenCalledWith('D:/docs/汇报')
  })

  it('批量文件级进度事件按 dirPath 过滤更新（第 N/M 个 + 当前文件名）', async () => {
    SplitPptxFolder.mockReturnValue(new Promise(() => {}))
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await setInput(wrapper, 'D:/docs')
    await clickButton(wrapper, '开始批量拆分')
    await flushPromises()

    expect(wrapper.find('.ps-progress').text()).toContain('正在扫描')
    eventHandlers['pptx-split:batch:progress']({ dirPath: 'D:/docs', index: 2, total: 5, currentFile: 'D:/docs/年报.pptx' })
    await flushPromises()
    const progress = wrapper.find('.ps-progress')
    expect(progress.text()).toContain('第 2 / 5 个文件')
    expect(progress.text()).toContain('D:/docs/年报.pptx')

    // 非本批次 dirPath 的事件不过滤写入（多入口共听过滤）
    eventHandlers['pptx-split:batch:progress']({ dirPath: 'D:/other', index: 9, total: 9, currentFile: 'D:/other/x.pptx' })
    await flushPromises()
    expect(wrapper.find('.ps-progress').text()).toContain('第 2 / 5 个文件')
  })

  it('批量页级进度：pptx-split:progress 按当前文件过滤写入页级行', async () => {
    SplitPptxFolder.mockReturnValue(new Promise(() => {}))
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await setInput(wrapper, 'D:/docs')
    await clickButton(wrapper, '开始批量拆分')
    await flushPromises()

    eventHandlers['pptx-split:batch:progress']({ dirPath: 'D:/docs', index: 1, total: 3, currentFile: 'D:/docs/汇报.pptx' })
    await flushPromises()
    // 当前文件的页级进度写入
    eventHandlers['pptx-split:progress']({ srcPath: 'D:/docs/汇报.pptx', done: 4, total: 12 })
    await flushPromises()
    expect(wrapper.find('.ps-batch-progress-page').text()).toContain('4 / 12 页')

    // 非当前文件（其他入口发起的单文件拆分）不写入批量页级行
    eventHandlers['pptx-split:progress']({ srcPath: 'D:/docs/其他.pptx', done: 9, total: 9 })
    await flushPromises()
    expect(wrapper.find('.ps-batch-progress-page').text()).toContain('4 / 12 页')

    // 下一个文件开始：页级进度清零（batch:progress 重置）
    eventHandlers['pptx-split:batch:progress']({ dirPath: 'D:/docs', index: 2, total: 3, currentFile: 'D:/docs/年报.pptx' })
    await flushPromises()
    expect(wrapper.find('.ps-batch-progress-page').exists()).toBe(false)
  })

  it('批量部分失败：汇总行转警告语义并展示失败原因', async () => {
    SplitPptxFolder.mockResolvedValue({
      dirPath: 'D:/docs',
      total: 2,
      items: [
        { srcPath: 'D:/docs/汇报.pptx', outputDir: 'D:/docs/汇报', pageCount: 12, error: '' },
        { srcPath: 'D:/docs/损坏.pptx', outputDir: '', pageCount: 0, error: 'zip 损坏' }
      ]
    })
    const { ElMessage } = await import('element-plus')
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await setInput(wrapper, 'D:/docs')
    await clickButton(wrapper, '开始批量拆分')
    await flushPromises()

    const result = wrapper.find('.ps-result')
    expect(result.text()).toContain('成功 1 个、失败 1 个')
    expect(result.find('.ps-result-summary-partial').exists()).toBe(true)
    const failedRow = wrapper.findAll('.ps-batch-item')[1]
    expect(failedRow.text()).toContain('损坏.pptx')
    expect(failedRow.text()).toContain('失败原因：zip 损坏')
    // 部分失败 warning 提示（非 success）
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('1 个失败'))
    expect(ElMessage.success).not.toHaveBeenCalled()
  })

  it('批量取消：调 CancelPptxSplit 且 Promise reject 走 warning 中性提示', async () => {
    const { ElMessage } = await import('element-plus')
    SplitPptxFolder.mockReturnValue(new Promise(() => {}))
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await setInput(wrapper, 'D:/docs')
    await clickButton(wrapper, '开始批量拆分')
    await flushPromises()
    expect(wrapper.find('.ps-progress').exists()).toBe(true)

    await clickButton(wrapper, '取消批量拆分')
    expect(CancelPptxSplit).toHaveBeenCalled()

    // 取消后 Promise reject（E_GIT_CANCELLED），handleError 分流 warning
    SplitPptxFolder.mockRejectedValue({ code: 'E_GIT_CANCELLED', message: '操作已取消' })
    const wrapper2 = createWrapper()
    await switchToFolderMode(wrapper2)
    await setInput(wrapper2, 'D:/docs')
    await clickButton(wrapper2, '开始批量拆分')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('操作已取消')
    expect(ElMessage.error).not.toHaveBeenCalled()
    // 在途态复位
    expect(wrapper2.find('.ps-progress').exists()).toBe(false)
    expect(wrapper2.findAll('button').some((b) => b.text() === '开始批量拆分')).toBe(true)
  })

  it('批量空扫描：后端引导级错误经 handleError 分流为 error 提示', async () => {
    const { ElMessage } = await import('element-plus')
    SplitPptxFolder.mockRejectedValue({ message: '目录下未找到 PPT 文件（D:/docs）' })
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await setInput(wrapper, 'D:/docs')
    await clickButton(wrapper, '开始批量拆分')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith('批量拆分失败: 目录下未找到 PPT 文件（D:/docs）')
    expect(wrapper.find('.ps-progress').exists()).toBe(false)
  })

  it('批量进行中模式切换禁用（防在途状态跨模式串扰）', async () => {
    SplitPptxFolder.mockReturnValue(new Promise(() => {}))
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await setInput(wrapper, 'D:/docs')
    await clickButton(wrapper, '开始批量拆分')
    await flushPromises()

    expect(wrapper.find('.ps-mode-switch').classes()).toContain('is-disabled')
    // 选项 disabled 态透传（radio 选项点击不再切换）
    const fileOption = wrapper.find('.ps-mode-option[data-value="file"]')
    expect(fileOption.attributes('data-disabled')).toBe('1')
  })

  it('目录浏览按钮经 OpenDirectoryDialog 回填路径', async () => {
    OpenDirectoryDialog.mockResolvedValue('D:/chosen')
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await clickButton(wrapper, '浏览')
    await flushPromises()
    expect(OpenDirectoryDialog).toHaveBeenCalledWith('选择要拆分的文件夹')
    expect(wrapper.find('input').element.value).toBe('D:/chosen')
  })

  it('目录浏览取消（返回空串）不覆盖已有输入', async () => {
    OpenDirectoryDialog.mockResolvedValue('')
    const wrapper = createWrapper()
    await switchToFolderMode(wrapper)
    await setInput(wrapper, 'D:/docs')
    await clickButton(wrapper, '浏览')
    await flushPromises()
    expect(wrapper.find('input').element.value).toBe('D:/docs')
  })
})
