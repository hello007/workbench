import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AssetFilterPopover, {
  FACET_MTIME_PRESET,
  FACET_SIZE_PRESET,
  createDefaultFacetState,
  resolveMtimeRangeNs,
  resolveSizeRangeBytes,
  buildFacetOptions,
  countActiveFacets,
  restoreFacetStateFromSnapshot,
  describeFacetState
} from '../AssetFilterPopover.vue'

// ---- Element Plus 组件 stub（弹层双 slot 全渲染，免触发机制直测内容）----
const stubs = {
  'el-popover': {
    template: '<div class="el-popover-stub"><slot name="reference" /><div class="popover-content"><slot /></div></div>'
  },
  // badge 以 data 属性透出 value/hidden（常显生效 facet 数契约断言点）
  'el-badge': {
    template: '<span class="el-badge" :data-value="String(value)" :data-hidden="hidden ? \'true\' : \'false\'"><slot /></span>',
    props: ['value', 'hidden']
  },
  'el-button': {
    template: '<button class="el-button" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'size', 'text', 'type'],
    emits: ['click']
  },
  'el-select': {
    template: '<select class="el-select"><slot /></select>',
    props: ['modelValue', 'multiple', 'collapseTags', 'clearable', 'placeholder', 'size', 'disabled'],
    emits: ['update:modelValue', 'change']
  },
  'el-option': {
    template: '<option :value="value"><slot /></option>',
    props: ['label', 'value']
  },
  'el-radio-group': {
    template: '<div class="el-radio-group"><slot /></div>',
    props: ['modelValue', 'size'],
    emits: ['update:modelValue', 'change']
  },
  'el-radio': {
    template: '<label class="el-radio"><slot /></label>',
    props: ['value', 'size']
  },
  'el-date-picker': {
    template: '<input class="el-date-picker" />',
    props: ['modelValue', 'type', 'size', 'startPlaceholder', 'endPlaceholder'],
    emits: ['update:modelValue']
  }
}

// 挂载 harness：v-model 双向绑定使 emitted 快照回灌 props（受控组件回环，
// 断言交互后 UI 态与外发快照一致）；无 v-model 直挂时 props 不回灌，弹层内
// 控件显示态不更新，交互链断（受控组件测试须带宿主）
const mountPopover = (initial) => {
  const Harness = {
    components: { AssetFilterPopover },
    template: '<asset-filter-popover v-model="state" />',
    data: () => ({ state: initial || createDefaultFacetState() })
  }
  return mount(Harness, { global: { stubs } })
}

const getPopover = (wrapper) => wrapper.findComponent(AssetFilterPopover)

// 相对档换算的固定「当前时刻」（注入 now，禁真实时钟——test-stability 红线）。
// 本地时间解析（无时区后缀的 ISO date-time 按本地时区解释），期望值同口径以
// Date 构造器表达，断言与 CI 时区无关
const NOW = new Date('2026-10-16T10:30:00').getTime()
const DAY_MS = 24 * 60 * 60 * 1000
const MB = 1024 * 1024
const NS_PER_MS = 1e6

describe('AssetFilterPopover 档位换算（纯函数单点）', () => {
  it('createDefaultFacetState 默认快照：三维全不限、无评分选中、无自定义区间', () => {
    expect(createDefaultFacetState()).toEqual({
      ratings: [],
      mtimePreset: 'any',
      mtimeRange: null,
      sizePreset: 'any'
    })
    // 每次调用返回全新对象（复位共用同一工厂，禁共享引用串态）
    const a = createDefaultFacetState()
    const b = createDefaultFacetState()
    expect(a).not.toBe(b)
    expect(a.ratings).not.toBe(b.ratings)
  })

  it('resolveMtimeRangeNs：不限档不设界（0 = 后端不设该侧边界）', () => {
    expect(resolveMtimeRangeNs({ mtimePreset: 'any' }, NOW)).toEqual({ mtimeFrom: 0, mtimeTo: 0 })
    expect(resolveMtimeRangeNs(undefined, NOW)).toEqual({ mtimeFrom: 0, mtimeTo: 0 })
  })

  it('resolveMtimeRangeNs：今天 = 本地时区当日零点起算，上界为注入时刻', () => {
    const range = resolveMtimeRangeNs({ mtimePreset: FACET_MTIME_PRESET.TODAY }, NOW)
    expect(range.mtimeFrom).toBe(new Date(2026, 9, 16).getTime() * NS_PER_MS)
    expect(range.mtimeTo).toBe(NOW * NS_PER_MS)
  })

  it('resolveMtimeRangeNs：近 7 天 / 近 30 天 = now 往前推自然毫秒数', () => {
    expect(resolveMtimeRangeNs({ mtimePreset: FACET_MTIME_PRESET.DAYS7 }, NOW)).toEqual({
      mtimeFrom: (NOW - 7 * DAY_MS) * NS_PER_MS,
      mtimeTo: NOW * NS_PER_MS
    })
    expect(resolveMtimeRangeNs({ mtimePreset: FACET_MTIME_PRESET.DAYS30 }, NOW)).toEqual({
      mtimeFrom: (NOW - 30 * DAY_MS) * NS_PER_MS,
      mtimeTo: NOW * NS_PER_MS
    })
  })

  it('resolveMtimeRangeNs：自定义区间 = 起日零点至止日末毫秒（闭区间含止日全天，毫秒尾差保 round-trip 可逆）', () => {
    const state = {
      mtimePreset: FACET_MTIME_PRESET.CUSTOM,
      mtimeRange: [new Date('2026-10-01T15:20:00'), new Date('2026-10-05T08:00:00')]
    }
    const range = resolveMtimeRangeNs(state, NOW)
    expect(range.mtimeFrom).toBe(new Date(2026, 9, 1).getTime() * NS_PER_MS)
    expect(range.mtimeTo).toBe(new Date(2026, 9, 6).getTime() * NS_PER_MS - NS_PER_MS)
  })

  it('resolveMtimeRangeNs：自定义区间半选（单端/空）不设界，防半选判活后空结果页误现', () => {
    expect(resolveMtimeRangeNs({ mtimePreset: 'custom', mtimeRange: null }, NOW)).toEqual({ mtimeFrom: 0, mtimeTo: 0 })
    expect(resolveMtimeRangeNs({ mtimePreset: 'custom', mtimeRange: [new Date()] }, NOW)).toEqual({ mtimeFrom: 0, mtimeTo: 0 })
    expect(resolveMtimeRangeNs({ mtimePreset: 'custom', mtimeRange: [new Date(), null] }, NOW)).toEqual({ mtimeFrom: 0, mtimeTo: 0 })
  })

  it('resolveMtimeRangeNs：自定义区间含不可解析日期时防御性不设界（不产出 NaN 边界脏参）', () => {
    expect(
      resolveMtimeRangeNs({ mtimePreset: 'custom', mtimeRange: ['not-a-date', '2026-10-05'] }, NOW)
    ).toEqual({ mtimeFrom: 0, mtimeTo: 0 })
  })

  it('resolveSizeRangeBytes：四档闭区间无缝衔接（相邻档在 1MB/10MB/50MB 处分界）', () => {
    expect(resolveSizeRangeBytes({ sizePreset: FACET_SIZE_PRESET.ANY })).toEqual({ sizeFrom: 0, sizeTo: 0 })
    expect(resolveSizeRangeBytes({ sizePreset: FACET_SIZE_PRESET.LT_1MB })).toEqual({ sizeFrom: 0, sizeTo: MB - 1 })
    expect(resolveSizeRangeBytes({ sizePreset: FACET_SIZE_PRESET.FROM_1_TO_10MB })).toEqual({ sizeFrom: MB, sizeTo: 10 * MB - 1 })
    expect(resolveSizeRangeBytes({ sizePreset: FACET_SIZE_PRESET.FROM_10_TO_50MB })).toEqual({ sizeFrom: 10 * MB, sizeTo: 50 * MB - 1 })
    expect(resolveSizeRangeBytes({ sizePreset: FACET_SIZE_PRESET.GT_50MB })).toEqual({ sizeFrom: 50 * MB, sizeTo: 0 })
  })

  it('buildFacetOptions：五字段与 model.AssetSearchOptions facet JSON tag 一一对应，ratings 为防御性拷贝', () => {
    const state = {
      ratings: [0, 5],
      mtimePreset: FACET_MTIME_PRESET.TODAY,
      mtimeRange: null,
      sizePreset: FACET_SIZE_PRESET.FROM_1_TO_10MB
    }
    const opts = buildFacetOptions(state, NOW)
    expect(opts).toEqual({
      ratings: [0, 5],
      mtimeFrom: new Date(2026, 9, 16).getTime() * NS_PER_MS,
      mtimeTo: NOW * NS_PER_MS,
      sizeFrom: MB,
      sizeTo: 10 * MB - 1
    })
    // 拷贝语义：调用方（宿主 runSearch spread）不得与 UI 快照共享数组引用
    expect(opts.ratings).not.toBe(state.ratings)
  })

  it('countActiveFacets：0-3 维计数，时间自定义半选不判活（与后端 active 口径对齐）', () => {
    expect(countActiveFacets(createDefaultFacetState())).toBe(0)
    expect(countActiveFacets({ ...createDefaultFacetState(), ratings: [3] })).toBe(1)
    expect(countActiveFacets({ ...createDefaultFacetState(), mtimePreset: 'today' })).toBe(1)
    // 自定义半选（区间未挑）不判活——防「选了自定义还没挑日期」时 facets-only
    // 激活而后端 facet 条件为空，检索落入空查询空结果页误现「无命中」
    expect(countActiveFacets({ ...createDefaultFacetState(), mtimePreset: 'custom', mtimeRange: null })).toBe(0)
    expect(countActiveFacets({ ...createDefaultFacetState(), mtimePreset: 'custom', mtimeRange: [new Date(), new Date()] })).toBe(1)
    expect(countActiveFacets({ ...createDefaultFacetState(), sizePreset: 'gt50' })).toBe(1)
    expect(
      countActiveFacets({ ratings: [1], mtimePreset: '7d', mtimeRange: null, sizePreset: 'lt1' })
    ).toBe(3)
    expect(countActiveFacets(null)).toBe(0)
  })
})

describe('AssetFilterPopover 保存快照反推与摘要（第七批 PR3，换算单点）', () => {
  it('restoreFacetStateFromSnapshot：自定义区间快照 round-trip 精确还原（星级/区间日界/大小档位）', () => {
    const state = {
      ratings: [0, 4],
      mtimePreset: FACET_MTIME_PRESET.CUSTOM,
      mtimeRange: [new Date(2026, 9, 1), new Date(2026, 9, 15)],
      sizePreset: FACET_SIZE_PRESET.FROM_1_TO_10MB
    }
    const snapshot = buildFacetOptions(state, NOW)
    const restored = restoreFacetStateFromSnapshot(snapshot)
    expect(restored.ratings).toEqual([0, 4])
    expect(restored.mtimePreset).toBe(FACET_MTIME_PRESET.CUSTOM)
    expect(restored.mtimeRange[0].getTime()).toBe(new Date(2026, 9, 1).getTime())
    expect(restored.mtimeRange[1].getTime()).toBe(new Date(2026, 9, 15).getTime())
    expect(restored.sizePreset).toBe(FACET_SIZE_PRESET.FROM_1_TO_10MB)
    // 反推结果再换算回区间与快照一致（调取后检索参数 = 保存时检索参数）
    expect(resolveMtimeRangeNs(restored, NOW)).toEqual({ mtimeFrom: snapshot.mtimeFrom, mtimeTo: snapshot.mtimeTo })
    expect(resolveSizeRangeBytes(restored)).toEqual({ sizeFrom: snapshot.sizeFrom, sizeTo: snapshot.sizeTo })
  })

  it('restoreFacetStateFromSnapshot：相对档快照冻结为自定义区间（按上界所在日还原，日级覆盖面一致）', () => {
    const snapshot = buildFacetOptions({ ...createDefaultFacetState(), mtimePreset: FACET_MTIME_PRESET.DAYS7 }, NOW)
    const restored = restoreFacetStateFromSnapshot(snapshot)
    expect(restored.mtimePreset).toBe(FACET_MTIME_PRESET.CUSTOM)
    // 近 7 天（保存于 NOW）冻结区间 [10-09, 10-16]，按日还原两端均落在覆盖日
    expect(restored.mtimeRange[0].getTime()).toBe(new Date(2026, 9, 9).getTime())
    expect(restored.mtimeRange[1].getTime()).toBe(new Date(2026, 9, 16).getTime())
  })

  it('restoreFacetStateFromSnapshot：全零区间回不限档；gt50 单侧上界（sizeTo=0）正确反推档位', () => {
    expect(restoreFacetStateFromSnapshot({ ratings: [], mtimeFrom: 0, mtimeTo: 0, sizeFrom: 0, sizeTo: 0 }))
      .toEqual(createDefaultFacetState())
    expect(
      restoreFacetStateFromSnapshot({ ratings: [], mtimeFrom: 0, mtimeTo: 0, sizeFrom: 50 * MB, sizeTo: 0 }).sizePreset
    ).toBe(FACET_SIZE_PRESET.GT_50MB)
  })

  it('restoreFacetStateFromSnapshot：异常数据防御——星级越界丢弃、大小未匹配回退不限、mtime 单侧非零不判活', () => {
    const restored = restoreFacetStateFromSnapshot({
      ratings: [2, 9, -1, 2],
      mtimeFrom: 123,
      mtimeTo: 0,
      sizeFrom: 777,
      sizeTo: 888
    })
    expect(restored.ratings).toEqual([2])
    expect(restored.mtimePreset).toBe(FACET_MTIME_PRESET.ANY)
    expect(restored.mtimeRange).toBeNull()
    expect(restored.sizePreset).toBe(FACET_SIZE_PRESET.ANY)
  })

  it('describeFacetState：三维摘要文案（星级排序拼接/时间绝对区间/大小档位），不限档显「不限」', () => {
    expect(describeFacetState(createDefaultFacetState(), NOW)).toEqual({
      ratingText: '不限',
      mtimeText: '不限',
      sizeText: '不限'
    })
    const text = describeFacetState(
      {
        ratings: [5, 0],
        mtimePreset: FACET_MTIME_PRESET.CUSTOM,
        mtimeRange: [new Date(2026, 9, 1), new Date(2026, 9, 15)],
        sizePreset: FACET_SIZE_PRESET.LT_1MB
      },
      NOW
    )
    expect(text.ratingText).toBe('未评分、5 星')
    expect(text.mtimeText).toBe('2026-10-01 ~ 2026-10-15')
    expect(text.sizeText).toBe('< 1MB')
  })

  it('describeFacetState：相对档展示解析后的绝对区间（与保存快照/调取还原口径一致）', () => {
    const text = describeFacetState({ ...createDefaultFacetState(), mtimePreset: FACET_MTIME_PRESET.TODAY }, NOW)
    expect(text.mtimeText).toBe('2026-10-16 ~ 2026-10-16')
  })
})

describe('AssetFilterPopover 组件（受控契约）', () => {
  it('默认快照挂载：badge 隐藏（0 维无意义徽标是噪音），不外发任何事件', () => {
    const wrapper = mountPopover()
    const badge = wrapper.find('.el-badge')
    expect(badge.attributes('data-hidden')).toBe('true')
    expect(getPopover(wrapper).emitted('update:modelValue')).toBeUndefined()
    expect(getPopover(wrapper).emitted('change')).toBeUndefined()
  })

  it('badge 常显生效维度数：外部快照 2 维生效时展示 2', () => {
    const wrapper = mountPopover({ ratings: [1, 4], mtimePreset: 'today', mtimeRange: null, sizePreset: 'any' })
    const badge = wrapper.find('.el-badge')
    expect(badge.attributes('data-hidden')).toBe('false')
    expect(badge.attributes('data-value')).toBe('2')
  })

  it('星级多选：映射 ratings 数组（含 0=未评分），经 update:modelValue + change 双事件同步宿主', async () => {
    const wrapper = mountPopover()
    const select = wrapper.findComponent(stubs['el-select'])
    await select.vm.$emit('update:modelValue', [0, 3])
    const emitted = getPopover(wrapper).emitted()
    expect(emitted['update:modelValue']).toHaveLength(1)
    expect(emitted['change']).toHaveLength(1)
    expect(emitted['update:modelValue'][0][0].ratings).toEqual([0, 3])
    // 受控回环：harness v-model 回灌后 badge 计数升 1
    await wrapper.vm.$nextTick()
    expect(wrapper.vm.state.ratings).toEqual([0, 3])
    expect(wrapper.find('.el-badge').attributes('data-value')).toBe('1')
  })

  it('修改时间相对档：单选映射 mtimePreset，选「自定义」时日期区间选择器出现', async () => {
    const wrapper = mountPopover()
    const [mtimeGroup] = wrapper.findAllComponents(stubs['el-radio-group'])
    expect(wrapper.find('.el-date-picker').exists()).toBe(false)
    await mtimeGroup.vm.$emit('update:modelValue', FACET_MTIME_PRESET.CUSTOM)
    await wrapper.vm.$nextTick()
    expect(wrapper.vm.state.mtimePreset).toBe('custom')
    expect(wrapper.find('.el-date-picker').exists()).toBe(true)
  })

  it('自定义区间：双端日期写入 mtimeRange，半选（单端）归一为 null 不落脏快照', async () => {
    const d1 = new Date('2026-10-01T00:00:00')
    const d2 = new Date('2026-10-05T00:00:00')
    const wrapper = mountPopover({ ...createDefaultFacetState(), mtimePreset: 'custom' })
    const picker = wrapper.findComponent(stubs['el-date-picker'])
    await picker.vm.$emit('update:modelValue', [d1, d2])
    expect(wrapper.vm.state.mtimeRange).toEqual([d1, d2])
    await picker.vm.$emit('update:modelValue', [d1])
    expect(wrapper.vm.state.mtimeRange).toBeNull()
  })

  it('文件大小分档：单选映射 sizePreset（多档组合不做，选档即单值映射）', async () => {
    const wrapper = mountPopover()
    const [, sizeGroup] = wrapper.findAllComponents(stubs['el-radio-group'])
    await sizeGroup.vm.$emit('update:modelValue', FACET_SIZE_PRESET.GT_50MB)
    expect(wrapper.vm.state.sizePreset).toBe('gt50')
  })

  it('重置：任意生效快照回默认档并回写宿主', async () => {
    const wrapper = mountPopover({ ratings: [2], mtimePreset: '30d', mtimeRange: null, sizePreset: '1to10' })
    const resetBtn = wrapper.findAll('button').find((b) => b.text().trim() === '重置')
    await resetBtn.trigger('click')
    expect(wrapper.vm.state).toEqual(createDefaultFacetState())
    expect(getPopover(wrapper).emitted('change')).toHaveLength(1)
  })
})
