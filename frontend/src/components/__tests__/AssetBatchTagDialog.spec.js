import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetBatchTagDialog from '../AssetBatchTagDialog.vue'

// 形状对齐 model.AssetTagListItem：两级样例（根 1 含子 2、根 3），供 ID→名称
// 映射断言（op 提交标签名数组，后端 AssetBatchTagOp 契约）
const mockTagList = [
  { id: 1, name: '重点客户', color: '#2563eb', parentId: 0, count: 3 },
  { id: 2, name: '城商行', color: '#0891b2', parentId: 1, count: 2 },
  { id: 3, name: '方案', color: '#16a34a', parentId: 0, count: 1 }
]

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

// 标签树组件桩：受控 emit 形态对齐真实组件（update:modelValue ID 口径 + mounted
// 模拟首拉完成回传 loaded 平铺清单），两个分区实例经 findAllComponents 按序定位
//（模板序 = 添加区、移除区）
const AssetTagTreeSelectStub = {
  name: 'AssetTagTreeSelectStub',
  template: '<div class="tree-select-stub"></div>',
  props: ['modelValue', 'libraryId', 'placeholder', 'disabled', 'emptyText'],
  emits: ['update:modelValue', 'change', 'loaded'],
  mounted() {
    this.$emit('loaded', mockTagList)
  }
}

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
  AssetTagTreeSelect: AssetTagTreeSelectStub
}

const createWrapper = async (props = {}) => {
  const wrapper = mount(AssetBatchTagDialog, {
    props: {
      modelValue: true,
      libraryId: 'lib-1',
      targets: mockTargets,
      execute: vi.fn(() => Promise.resolve({ total: 0, succeeded: 0, failed: 0, items: [] })),
      ...props
    },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}

// 定位两分区树组件（模板序：0 = 添加区、1 = 移除区），emit 勾选 ID 数组
const selectIn = async (wrapper, section, ids) => {
  const selects = wrapper.findAllComponents(AssetTagTreeSelectStub)
  await selects[section].vm.$emit('update:modelValue', ids)
  await flushPromises()
}

const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text().trim() === text)

// disabled 断言辅助：桩按钮 truthy disabled 渲染为 disabled 属性（空串值）
const isDisabled = (btn) => btn.attributes('disabled') !== undefined

describe('AssetBatchTagDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('打开即渲染两分区树组件（添加/移除），目标计数与相对语义说明可见', async () => {
    const wrapper = await createWrapper()
    const selects = wrapper.findAllComponents(AssetTagTreeSelectStub)
    expect(selects.length).toBe(2)
    expect(selects[0].props('placeholder')).toBe('选择要添加的标签')
    expect(selects[1].props('placeholder')).toBe('选择要移除的标签')
    expect(selects[0].props('libraryId')).toBe('lib-1')
    expect(wrapper.find('.batch-tag-scope').text()).toContain('选中的 3 项')
    expect(wrapper.find('.batch-tag-scope').text()).toContain('相对各条目现状生效')
  })

  it('提交契约：op 为标签名数组（ID 经 loaded 平铺清单映射，后端 AssetBatchTagOp 形态）', async () => {
    const execute = vi.fn(() => Promise.resolve({ total: 1, succeeded: 1, failed: 0, items: [] }))
    const wrapper = await createWrapper({ execute })
    await selectIn(wrapper, 0, [1])
    await selectIn(wrapper, 1, [3])
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()
    expect(execute).toHaveBeenCalledTimes(1)
    expect(execute).toHaveBeenCalledWith({ addTags: ['重点客户'], removeTags: ['方案'] })
  })

  it('两区全空提交禁用（前端防线，后端 normalizeBatchTagOp 双防线）', async () => {
    const wrapper = await createWrapper()
    const submit = findButton(wrapper, '提交')
    expect(isDisabled(submit)).toBe(true)
    await selectIn(wrapper, 0, [1])
    expect(isDisabled(findButton(wrapper, '提交'))).toBe(false)
  })

  it('交集防线：同一标签同时出现在添加与移除区时提交禁用 + 提示（PRD D4 双防线前端侧）', async () => {
    const wrapper = await createWrapper()
    await selectIn(wrapper, 0, [1])
    await selectIn(wrapper, 1, [1])
    const conflict = wrapper.find('.batch-tag-conflict')
    expect(conflict.exists()).toBe(true)
    expect(conflict.text()).toContain('「重点客户」不能同时添加与移除')
    expect(isDisabled(findButton(wrapper, '提交'))).toBe(true)
    // 非交集标签可正常启用（仅交集项被拦截）
    await selectIn(wrapper, 1, [3])
    expect(wrapper.find('.batch-tag-conflict').exists()).toBe(false)
    expect(isDisabled(findButton(wrapper, '提交'))).toBe(false)
  })

  it('全部成功：toast「已为 N 项更新标签」+ 上报 completed + 弹窗关闭，不进结果视图', async () => {
    const execute = vi.fn(() => Promise.resolve({
      total: 2,
      succeeded: 2,
      failed: 0,
      items: [
        { fileId: 11, status: 'ok', message: '' },
        { fileId: 12, status: 'ok', message: '' }
      ]
    }))
    const wrapper = await createWrapper({ execute })
    await selectIn(wrapper, 0, [1])
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已为 2 项更新标签')
    expect(wrapper.emitted('completed')).toHaveLength(1)
    expect(wrapper.emitted('update:modelValue').at(-1)).toEqual([false])
    expect(wrapper.find('.batch-tag-result').exists()).toBe(false)
  })

  it('部分失败：弹窗内切结果视图（成功 X / 失败 Y 汇总 + 失败逐条定位明细），成功条目照常上报 completed', async () => {
    const execute = vi.fn(() => Promise.resolve({
      total: 3,
      succeeded: 2,
      failed: 1,
      items: [
        { fileId: 11, status: 'ok', message: '' },
        { fileId: 12, status: 'failed', message: '文件已被占用' },
        { fileId: 11, slideNum: 2, status: 'failed', message: '页不存在' }
      ]
    }))
    const wrapper = await createWrapper({ execute })
    await selectIn(wrapper, 0, [1])
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()

    // 成功条目已写入（PRD D3 部分失败不中断）：completed 照常上报宿主刷新
    expect(wrapper.emitted('completed')).toHaveLength(1)
    // 弹窗不自动关闭，切结果视图：汇总 + 失败明细（定位文案来自 targets 反查）
    expect(wrapper.emitted('update:modelValue')).toBeFalsy()
    expect(wrapper.find('.batch-tag-result').exists()).toBe(true)
    expect(wrapper.find('.summary-ok').text()).toBe('成功 2 项')
    expect(wrapper.find('.summary-failed').text()).toBe('失败 1 项')
    const rows = wrapper.findAll('.batch-tag-failed-row')
    expect(rows.length).toBe(2)
    expect(rows[0].text()).toContain('方案B/获客方案.pptx')
    expect(rows[0].text()).toContain('文件已被占用')
    expect(rows[1].text()).toContain('方案A/城商行方案.pptx · 第 2 页')
    expect(rows[1].text()).toContain('页不存在')
    // 结果视图页脚只有「关闭」
    expect(findButton(wrapper, '关闭')).toBeTruthy()
    expect(findButton(wrapper, '提交')).toBeFalsy()
  })

  it('结果条目反查 miss（targets 无对应项）回退合成定位文案，不丢明细行', async () => {
    const execute = vi.fn(() => Promise.resolve({
      total: 1,
      succeeded: 0,
      failed: 1,
      items: [{ fileId: 99, slideNum: 4, status: 'failed', message: '目标已失效' }]
    }))
    const wrapper = await createWrapper({ execute })
    await selectIn(wrapper, 0, [1])
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()
    const rows = wrapper.findAll('.batch-tag-failed-row')
    expect(rows.length).toBe(1)
    expect(rows[0].text()).toContain('文件 #99 · 第 4 页')
  })

  it('提交中 loading 防重复提交（在途 Promise 期间重复点击只执行一次）', async () => {
    let resolveExecute
    const execute = vi.fn(() => new Promise((resolve) => { resolveExecute = resolve }))
    const wrapper = await createWrapper({ execute })
    await selectIn(wrapper, 0, [1])
    await findButton(wrapper, '提交').trigger('click')
    expect(isDisabled(findButton(wrapper, '提交'))).toBe(true)

    // 在途窗口内重复点击：提交被 loading/disabled 拦截，execute 不重复调用
    await findButton(wrapper, '提交').trigger('click')
    expect(execute).toHaveBeenCalledTimes(1)

    resolveExecute({ total: 1, succeeded: 1, failed: 0, items: [] })
    await flushPromises()
    expect(wrapper.emitted('completed')).toHaveLength(1)
    // 落定后提交中态复位（下次打开可正常提交）
    expect(isDisabled(findButton(wrapper, '提交'))).toBe(false)
  })

  it('执行失败（参数校验拒绝/库不存在等整体失败）经 handleError 分流，不切结果视图不上报 completed', async () => {
    const execute = vi.fn(() => Promise.reject(new Error('素材库不存在或已删除')))
    const wrapper = await createWrapper({ execute })
    await selectIn(wrapper, 0, [1])
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('批量打标签失败: 素材库不存在或已删除')
    expect(wrapper.find('.batch-tag-result').exists()).toBe(false)
    expect(wrapper.emitted('completed')).toBeFalsy()
  })

  it('重新打开复位表单/结果态：上次会话的结果视图与勾选不残留', async () => {
    const execute = vi.fn(() => Promise.resolve({
      total: 1, succeeded: 0, failed: 1,
      items: [{ fileId: 12, status: 'failed', message: '文件已被占用' }]
    }))
    const wrapper = await createWrapper({ execute })
    await selectIn(wrapper, 0, [1])
    await findButton(wrapper, '提交').trigger('click')
    await flushPromises()
    expect(wrapper.find('.batch-tag-result').exists()).toBe(true)

    // 关闭后重新打开：复位为表单视图 + 勾选清空
    await wrapper.setProps({ modelValue: false })
    await wrapper.setProps({ modelValue: true })
    await flushPromises()
    expect(wrapper.find('.batch-tag-result').exists()).toBe(false)
    expect(isDisabled(findButton(wrapper, '提交'))).toBe(true)
  })

  it('取消按钮关闭弹窗且不上报 completed（无数据变更）', async () => {
    const wrapper = await createWrapper()
    await findButton(wrapper, '取消').trigger('click')
    expect(wrapper.emitted('update:modelValue').at(-1)).toEqual([false])
    expect(wrapper.emitted('completed')).toBeFalsy()
  })
})
