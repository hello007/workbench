import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetBatchRatingDialog from '../AssetBatchRatingDialog.vue'

// 批量目标 fixture（宿主构建传入）：文件模式两条（slideNum 缺省）+ 幻灯片模式
// 一条（label 为宿主按 viewMode 拼装的定位文案，弹窗只消费不拼装）
const mockTargets = [
  { fileId: 11, label: '方案A/城商行方案.pptx' },
  { fileId: 12, label: '方案B/获客方案.pptx' },
  { fileId: 11, slideNum: 2, label: '方案A/城商行方案.pptx · 第 2 页' }
]

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="stub-dialog"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'appendToBody'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'type', 'size', 'loading', 'text', 'plain'],
    // emits 必须声明：缺省时父级 onClick 经 attrs 透传为原生监听器双重执行
    emits: ['click']
  },
  'el-tag': { template: '<span class="el-tag" :data-type="type"><slot /></span>', props: ['type', 'size'] },
  // 星级控件桩：点击第 n 颗星 emit 数字值（对齐真实 el-rate update:modelValue
  // 数字语义，clearable 透传供断言 0 = 清除评分契约）
  'el-rate': {
    template: '<div class="el-rate"><button v-for="n in 5" :key="n" class="rate-star" @click="$emit(\'update:modelValue\', n)">{{ n }}</button></div>',
    props: ['modelValue', 'max', 'allowHalf', 'clearable', 'disabled'],
    emits: ['update:modelValue']
  }
}

const createWrapper = async (props = {}) => {
  const wrapper = mount(AssetBatchRatingDialog, {
    props: {
      modelValue: true,
      targets: mockTargets,
      execute: vi.fn(() => Promise.resolve({ total: 0, succeeded: 0, failed: 0, items: [] })),
      ...props
    },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}

const selectRating = async (wrapper, value) => {
  await wrapper.findAll('.rate-star')[value - 1].trigger('click')
  await flushPromises()
}

const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text().trim() === text)

describe('AssetBatchRatingDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('打开即渲染星级单选（默认 0 = 清除语义），目标计数与作用对象说明可见', async () => {
    const wrapper = await createWrapper()
    expect(wrapper.find('.batch-rating-scope').text()).toContain('选中的 3 项')
    expect(wrapper.find('.batch-rating-scope').text()).toContain('文件条目评文件星级')
    expect(wrapper.find('.batch-rating-label').text()).toContain('0 = 清除评分')
    expect(wrapper.find('.batch-rating-hint').text()).toContain('清除评分')
  })

  it('提交契约：execute 收到星级数字值（0-5，后端 BatchSetAssetRating rating 形态）', async () => {
    const execute = vi.fn(() => Promise.resolve({ total: 1, succeeded: 1, failed: 0, items: [] }))
    const wrapper = await createWrapper({ execute })
    await selectRating(wrapper, 4)
    expect(wrapper.find('.batch-rating-hint').text()).toContain('4 星')
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()
    expect(execute).toHaveBeenCalledTimes(1)
    expect(execute).toHaveBeenCalledWith(4)
  })

  it('0 = 清除评分：不选星级直接提交，toast 为清除语义', async () => {
    const execute = vi.fn(() => Promise.resolve({ total: 2, succeeded: 2, failed: 0, items: [] }))
    const wrapper = await createWrapper({ execute })
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()
    expect(execute).toHaveBeenCalledWith(0)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已清除 2 项评分')
  })

  it('全部成功：toast「已为 N 项设置 N 星」+ 上报 completed + 弹窗关闭，不进结果视图', async () => {
    const execute = vi.fn(() => Promise.resolve({
      total: 2,
      succeeded: 2,
      failed: 0,
      items: [
        { fileId: 11, status: 'ok', label: '方案A/城商行方案.pptx' },
        { fileId: 12, status: 'ok', label: '方案B/获客方案.pptx' }
      ]
    }))
    const wrapper = await createWrapper({ execute })
    await selectRating(wrapper, 3)
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已为 2 项设置 3 星')
    expect(wrapper.emitted('completed')).toHaveLength(1)
    expect(wrapper.emitted('update:modelValue').at(-1)).toEqual([false])
    expect(wrapper.find('.batch-rating-result').exists()).toBe(false)
  })

  it('部分失败：弹窗内切结果视图（汇总 + 失败逐条定位明细），成功条目照常上报 completed', async () => {
    const execute = vi.fn(() => Promise.resolve({
      total: 3,
      succeeded: 1,
      failed: 2,
      items: [
        { fileId: 11, status: 'ok', label: '方案A/城商行方案.pptx' },
        { fileId: 12, status: 'failed', message: '文件不存在或已被删除（fileId=12）', label: '方案B/获客方案.pptx' },
        { fileId: 11, slideNum: 2, status: 'failed', message: '页不存在', label: '方案A/城商行方案.pptx · 第 2 页' }
      ]
    }))
    const wrapper = await createWrapper({ execute })
    await selectRating(wrapper, 5)
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()

    // 成功条目已写入（每条目一事务部分失败不中断）：completed 照常上报宿主刷新
    expect(wrapper.emitted('completed')).toHaveLength(1)
    // 弹窗不自动关闭，切结果视图：汇总 + 失败明细（定位文案 = 后端 Label）
    expect(wrapper.emitted('update:modelValue')).toBeFalsy()
    expect(wrapper.find('.batch-rating-result').exists()).toBe(true)
    expect(wrapper.find('.summary-ok').text()).toBe('成功 1 项')
    expect(wrapper.find('.summary-failed').text()).toBe('失败 2 项')
    const rows = wrapper.findAll('.batch-rating-failed-row')
    expect(rows.length).toBe(2)
    expect(rows[0].text()).toContain('方案B/获客方案.pptx')
    expect(rows[0].text()).toContain('文件不存在')
    expect(rows[1].text()).toContain('方案A/城商行方案.pptx · 第 2 页')
    expect(rows[1].text()).toContain('页不存在')
    // 结果视图页脚只有「关闭」
    expect(findButton(wrapper, '关闭')).toBeTruthy()
    expect(findButton(wrapper, '提交')).toBeFalsy()
  })

  it('后端 Label 缺省回退 targets 反查（slideNum||0 双侧归一），再 miss 合成兜底文案', async () => {
    const execute = vi.fn(() => Promise.resolve({
      total: 2,
      succeeded: 0,
      failed: 2,
      items: [
        { fileId: 11, slideNum: 2, status: 'failed', message: '目标已失效' }, // 无 label → targets 反查命中
        { fileId: 99, slideNum: 4, status: 'failed', message: '目标已失效' } // 反查 miss → 合成兜底
      ]
    }))
    const wrapper = await createWrapper({ execute })
    await selectRating(wrapper, 2)
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()
    const rows = wrapper.findAll('.batch-rating-failed-row')
    expect(rows.length).toBe(2)
    expect(rows[0].text()).toContain('方案A/城商行方案.pptx · 第 2 页')
    expect(rows[1].text()).toContain('文件 #99 · 第 4 页')
  })

  it('提交中 loading 防重复提交（在途 Promise 期间重复点击只执行一次）', async () => {
    let resolveExecute
    const execute = vi.fn(() => new Promise((resolve) => { resolveExecute = resolve }))
    const wrapper = await createWrapper({ execute })
    await selectRating(wrapper, 1)
    await findButton(wrapper, '提交').trigger('click')
    expect(findButton(wrapper, '提交').attributes('disabled') !== undefined).toBe(true)

    // 在途窗口内重复点击：提交被 loading/disabled 拦截，execute 不重复调用
    await findButton(wrapper, '提交').trigger('click')
    expect(execute).toHaveBeenCalledTimes(1)

    resolveExecute({ total: 1, succeeded: 1, failed: 0, items: [] })
    await flushPromises()
    expect(wrapper.emitted('completed')).toHaveLength(1)
  })

  it('执行失败（rating 越界/库不存在等整体拒绝）经 handleError 分流，不切结果视图不上报 completed', async () => {
    const execute = vi.fn(() => Promise.reject(new Error('素材库不存在或已删除')))
    const wrapper = await createWrapper({ execute })
    await selectRating(wrapper, 3)
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('批量评分失败: 素材库不存在或已删除')
    expect(wrapper.find('.batch-rating-result').exists()).toBe(false)
    expect(wrapper.emitted('completed')).toBeFalsy()
  })

  it('重新打开复位表单/结果态：上次会话的结果视图与星级选择不残留', async () => {
    const execute = vi.fn(() => Promise.resolve({
      total: 1, succeeded: 0, failed: 1,
      items: [{ fileId: 12, status: 'failed', message: '文件已被占用', label: '方案B/获客方案.pptx' }]
    }))
    const wrapper = await createWrapper({ execute })
    await selectRating(wrapper, 3)
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()
    expect(wrapper.find('.batch-rating-result').exists()).toBe(true)

    // 关闭后重新打开：复位为表单视图 + 星级清零（0 = 清除语义提示）
    await wrapper.setProps({ modelValue: false })
    await wrapper.setProps({ modelValue: true })
    await flushPromises()
    expect(wrapper.find('.batch-rating-result').exists()).toBe(false)
    expect(wrapper.find('.batch-rating-hint').text()).toContain('清除评分')
  })

  it('取消按钮关闭弹窗且不上报 completed（无数据变更）', async () => {
    const wrapper = await createWrapper()
    await findButton(wrapper, '取消').trigger('click')
    expect(wrapper.emitted('update:modelValue').at(-1)).toEqual([false])
    expect(wrapper.emitted('completed')).toBeFalsy()
  })
})
