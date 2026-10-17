import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetPrefixReplaceDialog from '../AssetPrefixReplaceDialog.vue'

// 覆盖面：前缀输入与防抖预览（preview 回调消费 + 代次守卫 + 未填全不查询）、
// 零命中/未填全提交拦截（前端第一防线）、二次确认（命中数 + 新旧前缀列出）、
// execute 分派契约（归一化前的原始输入透传）、结果 toast 三态计数分型、completed
// 成败均上报（自动扫描失败时替换已提交，宿主都要刷新）、错误 handleError 分流
// （确认型 warning 码弹窗保持）。
// 宿主职责（PreviewAssetPathPrefixReplace / ReplaceAssetPathPrefix 绑定与刷新链）
// 不经本 spec——宿主接线由 AssetsPanel.spec 打桩消费。

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
    template: '<div v-if="modelValue" class="stub-dialog"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'appendToBody'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'type', 'size', 'loading', 'text', 'plain'],
    emits: ['click']
  },
  'el-input': {
    template: '<input class="el-input" :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="onInput" @keyup.enter="$emit(\'keyup.enter\')" />',
    props: ['modelValue', 'placeholder', 'disabled'],
    emits: ['update:modelValue', 'input', 'keyup.enter'],
    methods: {
      // 双事件通道：update:modelValue（v-model）+ input（组件 @input 防抖钩子）
      onInput(e) {
        this.$emit('update:modelValue', e.target.value)
        this.$emit('input', e)
      }
    }
  }
}

const okResult = {
  libraryId: 'lib-1', oldPrefix: '旧目录', newPrefix: '新目录',
  replaced: 3, revived: 2, reparsed: 1, stillLost: 0,
  scan: { total: 3, new: 0, updated: 0, unchanged: 3, failed: 0, encrypted: 0, denied: 0, lost: 0, revived: 0, removed: 0 }
}

const createWrapper = async (props = {}) => {
  const wrapper = mount(AssetPrefixReplaceDialog, {
    props: {
      modelValue: true,
      preview: vi.fn(() => Promise.resolve(3)),
      execute: vi.fn(() => Promise.resolve(okResult)),
      ...props
    },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}

const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text().trim() === text)

const setInput = async (wrapper, index, value) => {
  const input = wrapper.findAll('.el-input').at(index)
  input.element.value = value
  await input.trigger('input') // 桩双通道：update:modelValue + input（防抖钩子）
  await flushPromises()
}

describe('AssetPrefixReplaceDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('前缀未填全不查询预览（空串查询是参数错误非零命中），填全后防抖触发 preview', async () => {
    const preview = vi.fn(() => Promise.resolve(3))
    const wrapper = await createWrapper({ preview })
    // 仅填旧前缀：不查询
    await setInput(wrapper, 0, '旧目录')
    vi.advanceTimersByTime(400)
    await flushPromises()
    expect(preview).not.toHaveBeenCalled()
    // 填全新前缀：防抖 300ms 后查询一次
    await setInput(wrapper, 1, '新目录')
    vi.advanceTimersByTime(400)
    await flushPromises()
    expect(preview).toHaveBeenCalledTimes(1)
    expect(preview).toHaveBeenCalledWith('旧目录', '新目录')
    expect(wrapper.find('.prefix-count').text()).toBe('3')
    // 快速连续输入：仅最后一次查询生效（代次守卫丢弃过期响应）
    await setInput(wrapper, 0, '旧前缀B')
    await setInput(wrapper, 0, '旧前缀C')
    vi.advanceTimersByTime(400)
    await flushPromises()
    expect(preview).toHaveBeenLastCalledWith('旧前缀C', '新目录')
  })

  it('输入变化立即失效旧命中数回「统计中」占位（防抖窗口与在途请求不展示旧结果）', async () => {
    const wrapper = await createWrapper({})
    await setInput(wrapper, 0, '旧目录')
    await setInput(wrapper, 1, '新目录')
    vi.advanceTimersByTime(400)
    await flushPromises()
    expect(wrapper.find('.prefix-count').text()).toBe('3')
    // 再次输入：300ms 防抖未到即应回占位——陈旧 0 会误拦合法提交（零命中拦截），
    // 陈旧非 0 会误导二次确认文案
    await setInput(wrapper, 0, '旧目录2')
    expect(wrapper.find('.prefix-preview').text()).toContain('命中统计中')
    expect(wrapper.find('.prefix-count').exists()).toBe(false)
  })

  it('前缀未填全或零命中提交被拦（前端第一防线，确认框与 execute 不调）', async () => {
    const preview = vi.fn(() => Promise.resolve(0))
    const execute = vi.fn(() => Promise.resolve(okResult))
    const wrapper = await createWrapper({ preview, execute })
    await setInput(wrapper, 0, '旧目录')
    await setInput(wrapper, 1, '新目录')
    vi.advanceTimersByTime(400)
    await flushPromises()
    expect(wrapper.find('.prefix-preview').text()).toContain('命中 0 条')
    await findButton(wrapper, '替换').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('旧前缀没有命中的索引记录，请核对拼写')
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
    expect(execute).not.toHaveBeenCalled()

    // 未填全直接提交：必填拦截
    const wrapper2 = await createWrapper({ preview, execute })
    await findButton(wrapper2, '替换').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenLastCalledWith('请填写旧前缀与新前缀')
    expect(execute).not.toHaveBeenCalled()
  })

  it('二次确认列出命中数与新旧前缀，确认后 execute 收归一化前原始输入并三态 toast', async () => {
    const execute = vi.fn(() => Promise.resolve(okResult))
    const wrapper = await createWrapper({ execute })
    await setInput(wrapper, 0, '旧目录')
    await setInput(wrapper, 1, '新目录 ')
    vi.advanceTimersByTime(400)
    await flushPromises()
    await findButton(wrapper, '替换').trigger('click')
    await flushPromises()
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('旧前缀「旧目录」命中 3 条记录'),
      '确认替换',
      expect.objectContaining({ type: 'warning' })
    )
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('新前缀「新目录」'),
      '确认替换',
      expect.anything()
    )
    // execute 收归一化前的原始输入（归一化在后端单点，前端只 trim）
    expect(execute).toHaveBeenCalledWith('旧目录', '新目录')
    // 三态计数分型 toast（replaced=3 revived=2 reparsed=1 stillLost=0）
    expect(ElMessage.success).toHaveBeenCalledWith(
      '前缀替换完成：更新 3 条路径，恢复 2，重解析 1，已自动触发增量扫描'
    )
    // 成功关闭弹窗 + completed 上报
    expect(wrapper.emitted('completed')).toHaveLength(1)
    expect(wrapper.emitted('update:modelValue')).toContainEqual([false])
  })

  it('确认框取消不执行；执行失败仍上报 completed 且弹窗保持（确认型 warning 分流可改前缀重试）', async () => {
    // 确认框取消
    ElMessageBox.confirm.mockRejectedValueOnce('cancel')
    const execute = vi.fn(() => Promise.resolve(okResult))
    const wrapper = await createWrapper({ execute })
    await setInput(wrapper, 0, '旧目录')
    await setInput(wrapper, 1, '新目录')
    vi.advanceTimersByTime(400)
    await flushPromises()
    await findButton(wrapper, '替换').trigger('click')
    await flushPromises()
    expect(execute).not.toHaveBeenCalled()
    expect(wrapper.emitted('completed')).toBeUndefined()

    // 执行失败（零命中确认型 warning 码）：handleError 分流 warning、弹窗保持、
    // completed 仍上报（宿主刷新幂等）
    const failExecute = vi.fn(() => Promise.reject({ code: 'E_ASSETS_PREFIX_REPLACE_NO_MATCH', message: '旧前缀「旧」没有命中的索引记录' }))
    const wrapper2 = await createWrapper({ execute: failExecute })
    await setInput(wrapper2, 0, '旧')
    await setInput(wrapper2, 1, '新')
    vi.advanceTimersByTime(400)
    await flushPromises()
    await findButton(wrapper2, '替换').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('旧前缀「旧」没有命中的索引记录')
    expect(wrapper2.emitted('completed')).toHaveLength(1)
    expect(wrapper2.emitted('update:modelValue') ?? []).not.toContainEqual([false])
  })
})
