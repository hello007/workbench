import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetExportDialog from '../AssetExportDialog.vue'
import { ErrorCode } from '../../utils/error'

// 覆盖面：范围预览渲染、格式/目标前端校验防线、execute 分派契约（partial 形状：
// csvPath/mdDir/formats/pathMode/overwrite）、目标已存在确认型链路
//（E_ASSETS_EXPORT_TARGET_EXISTS → ElMessageBox.confirm → overwrite=true 重调 →
// 取消不再调）、其他错误 handleError 分流、目标浏览回填、中止通道（提交中关闭 =
// 放弃本次导出：cancel 触发 / 放弃态终态静默或中性收口 / 覆盖确认期间放弃不重调）。
// 宿主职责（范围解析/检索快照/目标浏览对话框绑定）不经本 spec——宿主接线由
// AssetsPanel.spec 打桩消费。

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve(true)) }
  }
})

import { ElMessage, ElMessageBox } from 'element-plus'

const stubs = {
  'el-dialog': {
    name: 'ElDialogStub',
    template: '<div v-if="modelValue" class="stub-dialog"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>',
    // beforeClose 声明为 prop 供守卫用例取用（真实 EP 中 before-close 拦截 ESC/X/
    // 遮罩三类用户关闭入口；桩不复刻其内部调用时机，守卫函数逻辑直测）
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'appendToBody', 'beforeClose'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'type', 'size', 'loading', 'text', 'plain'],
    emits: ['click']
  },
  'el-checkbox': {
    template: '<label class="el-checkbox"><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.checked)" /><slot /></label>',
    props: ['modelValue', 'disabled'],
    emits: ['update:modelValue']
  },
  'el-input': {
    template: '<input class="el-input" :value="modelValue" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" @keyup.enter="$emit(\'keyup.enter\')" />',
    props: ['modelValue', 'placeholder', 'disabled'],
    emits: ['update:modelValue', 'keyup.enter']
  },
  'el-radio-group': {
    template: '<div class="el-radio-group"><slot /></div>',
    props: ['modelValue', 'disabled'],
    emits: ['update:modelValue'],
    // provide 中继：radio 点选经组回传 update:modelValue（对齐 el-dropdown 桩先例
    // ——slot 内容与组的 v-model 无 props 通路，须显式中继）
    provide() {
      return { __radioGroupModel: (v) => this.$emit('update:modelValue', v) }
    }
  },
  'el-radio': {
    template: `<label class="el-radio"><input type="radio" :checked="modelValue === value" :disabled="disabled" @change="onPick" /><slot /></label>`,
    props: ['modelValue', 'value', 'disabled'],
    emits: ['update:modelValue'],
    inject: { __radioGroupModel: { default: null } },
    methods: {
      onPick() {
        if (this.__radioGroupModel) this.__radioGroupModel(this.value)
        this.$emit('update:modelValue', this.value)
      }
    }
  }
}

const okResult = { libraryId: 'lib-1', csvPath: 'D:/out.csv', mdDir: '', rows: 3, mdFiles: 0 }

const createWrapper = async (props = {}) => {
  const wrapper = mount(AssetExportDialog, {
    props: {
      modelValue: true,
      previewCount: 3,
      previewScopeLabel: '全库',
      execute: vi.fn(() => Promise.resolve(okResult)),
      pickCsvPath: vi.fn(() => Promise.resolve('')),
      pickMdDir: vi.fn(() => Promise.resolve('')),
      ...props
    },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}

const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text().trim() === text)

const setInput = async (wrapper, cls, value) => {
  const input = wrapper.find(cls)
  input.element.value = value
  await input.trigger('input')
}

describe('AssetExportDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('打开即渲染范围预览（将导出 N 条 + 范围文案；-1 = 统计中占位，0 = 零命中终态）', async () => {
    const wrapper = await createWrapper()
    expect(wrapper.find('.export-scope').text()).toContain('将导出 3 条')
    expect(wrapper.find('.export-scope').text()).toContain('全库')

    const loading = await createWrapper({ previewCount: -1, previewScopeLabel: '当前检索命中' })
    expect(loading.find('.export-count').text()).toBe('…')
    expect(loading.find('.export-scope-label').text()).toBe('（当前检索命中）')

    // 0 命中/空库是合法终态：显示 0 而非统计中省略号
    const zero = await createWrapper({ previewCount: 0, previewScopeLabel: '当前检索命中' })
    expect(zero.find('.export-count').text()).toBe('0')
  })

  it('未勾选任何格式提交被拦（前端第一防线，execute 不调）', async () => {
    const execute = vi.fn(() => Promise.resolve(okResult))
    const wrapper = await createWrapper({ execute })
    await wrapper.findAll('.el-checkbox input')[0].setValue(false)
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('请至少选择一种导出格式（CSV / Markdown）')
    expect(execute).not.toHaveBeenCalled()
  })

  it('选中格式但目标路径空缺提交被拦（CSV / MD 分别提示）', async () => {
    const execute = vi.fn(() => Promise.resolve(okResult))
    const wrapper = await createWrapper({ execute })
    // CSV 勾选 + 路径空
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('请填写 CSV 导出文件路径')
    expect(execute).not.toHaveBeenCalled()
    // 切 MD：勾上 MD、去掉 CSV，MD 目录空
    await wrapper.findAll('.el-checkbox input')[0].setValue(false)
    await wrapper.findAll('.el-checkbox input')[1].setValue(true)
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('请填写 Markdown 导出目录')
    expect(execute).not.toHaveBeenCalled()
  })

  it('提交契约：execute 收到 partial {csvPath, mdDir, formats, pathMode, overwrite:false}，成功 toast + 关闭 + exported', async () => {
    const execute = vi.fn(() => Promise.resolve({ libraryId: 'lib-1', csvPath: 'x', mdDir: 'd', rows: 5, mdFiles: 5 }))
    const wrapper = await createWrapper({ execute })
    await wrapper.findAll('.el-checkbox input')[1].setValue(true) // md 同选
    await setInput(wrapper, '.export-section .el-input', 'D:\\导出\\out.csv')
    // MD 目录输入（勾选后为必填项）：第二个 .el-input 即 MD 目录
    const inputs = wrapper.findAll('.el-input')
    inputs[1].element.value = 'D:\\导出\\cards'
    await inputs[1].trigger('input')
    // 路径口径切绝对（radio 经组中继回传 v-model）
    await wrapper.findAll('.el-radio input')[1].setValue()
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    expect(execute).toHaveBeenCalledTimes(1)
    expect(execute).toHaveBeenCalledWith({
      csvPath: 'D:\\导出\\out.csv',
      mdDir: 'D:\\导出\\cards',
      formats: ['csv', 'md'],
      pathMode: 'absolute',
      overwrite: false
    })
    expect(ElMessage.success).toHaveBeenCalledWith('已导出 5 条元数据（5 张卡片）')
    expect(wrapper.emitted('exported')).toBeTruthy()
    expect(wrapper.emitted('update:modelValue').at(-1)[0]).toBe(false)
  })

  it('目标已存在确认链路：确认型错误 → ElMessageBox.confirm（含后端消息）→ overwrite=true 重调成功', async () => {
    const existsError = { code: ErrorCode.AssetsExportTargetExists, message: '导出目标已存在：D:\\out.csv。确认覆盖将重写同名内容' }
    const execute = vi.fn()
      .mockImplementationOnce(() => Promise.reject(existsError))
      .mockImplementationOnce(() => Promise.resolve(okResult))
    const wrapper = await createWrapper({ execute })
    await setInput(wrapper, '.export-section .el-input', 'D:\\out.csv')
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    // 确认框弹出且消息含后端目标路径
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    const [msg] = ElMessageBox.confirm.mock.calls[0]
    expect(msg).toContain('D:\\out.csv')
    // 确认后 overwrite=true 重调
    expect(execute).toHaveBeenCalledTimes(2)
    expect(execute.mock.calls[1][0]).toMatchObject({ csvPath: 'D:\\out.csv', overwrite: true })
    expect(ElMessage.success).toHaveBeenCalledWith('已导出 3 条元数据')
    expect(wrapper.emitted('update:modelValue').at(-1)[0]).toBe(false)
  })

  it('覆盖确认取消：不再重调、弹窗保持打开', async () => {
    ElMessageBox.confirm.mockImplementationOnce(() => Promise.reject('cancel'))
    const existsError = { code: ErrorCode.AssetsExportTargetExists, message: '导出目标已存在' }
    const execute = vi.fn(() => Promise.reject(existsError))
    const wrapper = await createWrapper({ execute })
    await setInput(wrapper, '.export-section .el-input', 'D:\\out.csv')
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    expect(execute).toHaveBeenCalledTimes(1)
    expect(ElMessage.success).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('其他确认型错误（warning 码）走 warning 分流，非确认型错误走 error 分流', async () => {
    const warningError = { code: ErrorCode.AssetsScanInProgress, message: '该素材库正在扫描中' }
    const execute = vi.fn(() => Promise.reject(warningError))
    const wrapper = await createWrapper({ execute })
    await setInput(wrapper, '.export-section .el-input', 'D:\\out.csv')
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
    expect(ElMessage.warning).toHaveBeenCalledWith('该素材库正在扫描中')

    ElMessage.warning.mockClear()
    const execute2 = vi.fn(() => Promise.reject({ code: 'E_OTHER', message: '磁盘已满' }))
    const wrapper2 = await createWrapper({ execute: execute2 })
    await setInput(wrapper2, '.export-section .el-input', 'D:\\out.csv')
    await findButton(wrapper2, '导出').trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('导出失败: 磁盘已满')
  })

  it('提交中用户关闭（ESC/X/遮罩 → before-close）= 放弃本次导出：触发 cancel 放行关闭，非提交期不触发 cancel', async () => {
    let resolveExecute
    const execute = vi.fn(() => new Promise((resolve) => { resolveExecute = resolve }))
    const cancel = vi.fn(() => true)
    const wrapper = await createWrapper({ execute, cancel })
    const dialog = wrapper.findComponent({ name: 'ElDialogStub' })
    await setInput(wrapper, '.export-section .el-input', 'D:\\out.csv')
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    expect(execute).toHaveBeenCalledTimes(1)
    // 提交在途关闭：守卫放行（done 被调）+ 宿主 cancel 回调触发（后端中止通道）
    let closed = false
    dialog.vm.beforeClose(() => { closed = true })
    await flushPromises()
    expect(closed).toBe(true)
    expect(cancel).toHaveBeenCalledTimes(1)
    // 在途 execute 落定（放弃语义收口，成功走「后台完成」info 非成功 toast）
    resolveExecute(okResult)
    await flushPromises()
    expect(ElMessage.success).not.toHaveBeenCalled()
    expect(ElMessage.info).toHaveBeenCalledWith('已中止等待，导出已在后台完成')
    // 非提交期关闭：直接放行，cancel 不再触发
    dialog.vm.beforeClose(() => { closed = true })
    expect(closed).toBe(true)
    expect(cancel).toHaveBeenCalledTimes(1)
    // 重新提交：abandoned 标记已复位，提交中关闭再次触发 cancel
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    expect(execute).toHaveBeenCalledTimes(2)
    dialog.vm.beforeClose(() => {})
    await flushPromises()
    expect(cancel).toHaveBeenCalledTimes(2)
  })

  it('提交中 footer「中止导出」按钮：触发 cancel 并关闭弹窗，非提交期为普通取消', async () => {
    let resolveExecute
    const execute = vi.fn(() => new Promise((resolve) => { resolveExecute = resolve }))
    const cancel = vi.fn(() => true)
    const wrapper = await createWrapper({ execute, cancel })
    await setInput(wrapper, '.export-section .el-input', 'D:\\out.csv')
    // 非提交期按钮文案为「取消」
    expect(findButton(wrapper, '取消')).toBeTruthy()
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    // 提交在途：按钮切「中止导出」且启用（原 disabled 双保险随硬阻断守卫一并移除）
    const abortBtn = findButton(wrapper, '中止导出')
    expect(abortBtn).toBeTruthy()
    expect(abortBtn.attributes('disabled')).toBeUndefined()
    await abortBtn.trigger('click')
    await flushPromises()
    expect(cancel).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('update:modelValue').at(-1)[0]).toBe(false)
    // 落定后放弃语义收口（拒绝静默），不再弹「已导出」成功 toast
    resolveExecute(okResult)
    await flushPromises()
    expect(ElMessage.success).not.toHaveBeenCalled()
    expect(ElMessage.error).not.toHaveBeenCalled()
  })

  it('放弃后在途 execute 先成功（后端先于取消完成的竞态）：info「后台完成」中性提示 + exported 上报，不弹成功 toast 不走 finishExport', async () => {
    let resolveExecute
    const execute = vi.fn(() => new Promise((resolve) => { resolveExecute = resolve }))
    const cancel = vi.fn(() => true)
    const wrapper = await createWrapper({ execute, cancel })
    const dialog = wrapper.findComponent({ name: 'ElDialogStub' })
    await setInput(wrapper, '.export-section .el-input', 'D:\\out.csv')
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    // 提交在途关闭（真实 EP 中 done 即关闭；桩层 done 只回执被调）
    let closed = false
    dialog.vm.beforeClose(() => { closed = true })
    await flushPromises()
    expect(closed).toBe(true)
    resolveExecute({ libraryId: 'lib-1', csvPath: 'x', rows: 9, mdFiles: 0 })
    await flushPromises()
    expect(ElMessage.info).toHaveBeenCalledWith('已中止等待，导出已在后台完成')
    expect(ElMessage.success).not.toHaveBeenCalled()
    expect(wrapper.emitted('exported')).toHaveLength(1)
    // 放弃分支不触碰 visible（真实 EP 已由 done 关闭）：组件层零 update:modelValue
    //（finishExport 才会 emit 关闭）——以此断言未走 finishExport
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('放弃后在途 execute 拒绝（含后端取消确认错误）：静默无任何 toast', async () => {
    let rejectExecute
    const execute = vi.fn(() => new Promise((_, reject) => { rejectExecute = reject }))
    const cancel = vi.fn(() => true)
    const wrapper = await createWrapper({ execute, cancel })
    const dialog = wrapper.findComponent({ name: 'ElDialogStub' })
    await setInput(wrapper, '.export-section .el-input', 'D:\\out.csv')
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    dialog.vm.beforeClose(() => {})
    await flushPromises()
    rejectExecute({ code: ErrorCode.AssetsCancelled, message: '导出已取消' })
    await flushPromises()
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(ElMessage.warning).not.toHaveBeenCalled()
    expect(ElMessage.success).not.toHaveBeenCalled()
  })

  it('覆盖确认期间放弃：确认后不再以 overwrite=true 重调', async () => {
    let resolveConfirm
    ElMessageBox.confirm.mockImplementationOnce(
      () => new Promise((resolve) => { resolveConfirm = resolve })
    )
    const existsError = { code: ErrorCode.AssetsExportTargetExists, message: '导出目标已存在' }
    const execute = vi.fn(() => Promise.reject(existsError))
    const cancel = vi.fn(() => true)
    const wrapper = await createWrapper({ execute, cancel })
    const dialog = wrapper.findComponent({ name: 'ElDialogStub' })
    await setInput(wrapper, '.export-section .el-input', 'D:\\out.csv')
    await findButton(wrapper, '导出').trigger('click')
    await flushPromises()
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    // 确认框悬停期间关闭弹窗（放弃本次导出）
    dialog.vm.beforeClose(() => {})
    await flushPromises()
    expect(cancel).toHaveBeenCalledTimes(1)
    // 用户随后点「覆盖写入」：放弃态拦截，不重调
    resolveConfirm(true)
    await flushPromises()
    expect(execute).toHaveBeenCalledTimes(1)
  })

  it('目标浏览：pickCsvPath/pickMdDir 返回路径回填输入框，空串（用户取消）不回填', async () => {
    const pickCsvPath = vi.fn(() => Promise.resolve('D:\\选\\a.csv'))
    const pickMdDir = vi.fn(() => Promise.resolve('D:\\选\\cards'))
    const wrapper = await createWrapper({ pickCsvPath, pickMdDir })
    // 浏览按钮随格式勾选启停（未勾选时 disabled）：先勾 MD 再点其浏览
    await wrapper.findAll('.el-checkbox input')[1].setValue(true)
    const browseButtons = wrapper.findAll('button').filter((b) => b.text().trim() === '浏览…')
    await browseButtons[0].trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.el-input')[0].element.value).toBe('D:\\选\\a.csv')
    await browseButtons[1].trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.el-input')[1].element.value).toBe('D:\\选\\cards')
    // 取消（空串）不覆盖已填值
    const pickCancel = vi.fn(() => Promise.resolve(''))
    await wrapper.setProps({ pickCsvPath: pickCancel })
    await browseButtons[0].trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.el-input')[0].element.value).toBe('D:\\选\\a.csv')
  })
})
