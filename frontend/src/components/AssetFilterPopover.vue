<!-- frontend/src/components/AssetFilterPopover.vue -->
<!-- 素材多维筛选弹层（第七批 SRS §2.5，PRD D1/D2/D3）：工具行「筛选」按钮 +
     el-popover 弹层三维控件（星级多选 / 修改时间相对档 + 自定义区间 / 文件大小
     分档），badge 常显生效 facet 维度数（0 维时隐藏，无意义 0 徽标是噪音）。
     受控组件契约（对齐 AssetListView 无自有业务状态风格）：v-model 承载 UI 档位
     快照（ratings 数组 + mtimePreset/mtimeRange + sizePreset），宿主持有状态、
     组件只做展示与映射；任何控件变更经 update:modelValue + change 双事件同步
     宿主（change 触发宿主语境变更链，见 AssetsPanel.onFacetChange；对齐
     AssetTagTreeSelect 的 v-model + @change 双通道形态）。
     档位→检索参数换算单点：resolveMtimeRangeNs / resolveSizeRangeBytes /
     buildFacetOptions / countActiveFacets 以具名导出形式挂本组件模块（下方普通
     <script> 块）——宿主构建 SearchAssets opts 时每次检索调用 buildFacetOptions
     现算（相对档边界随检索时刻刷新，防冻结区间漏当日新文件），禁在宿主复制第二
     份换算口径（档位语义单一数据源在前端，PRD Technical Notes）。
     时间换算口径：今天 = 本地时区当日零点起算；近 7/30 天 = now 往前推自然毫秒
     数；自定义区间 = 起日零点至止日次日零点前 1ns（闭区间含止日全天）。后端只认
     绝对纳秒闭区间（model.AssetSearchOptions.MtimeFrom/MtimeTo，0 = 不设该侧
     边界），不认相对档。now 参数为相对档换算的注入点（单测以固定时刻驱动，禁
     依赖真实时钟——test-stability 约定）。
     大小换算口径：相邻档闭区间无缝衔接（<1MB 与 1-10MB 在 1MB 处分界、10-50MB
     与 >50MB 在 50MB 处分界），任意文件大小恰落入一档；多档组合不做（后端单
     区间，选档即单值映射，PRD D2 收敛）。
     保存检索条件双向换算（第七批 PR3，PRD D4/D7）：buildFacetOptions 产出的区间
     原始值即保存快照的 facet 字段口径（AssetSavedSearch.mtimeFrom/mtimeTo/
     sizeFrom/sizeTo），restoreFacetStateFromSnapshot 为调取回填的反推单点——
     大小精确匹配四档闭区间还原档位；修改时间双侧非零还原为自定义区间冻结档
     （相对档保存时已解析为绝对区间，调取按区间还原，保存的是什么区间调取就是
     什么区间），区间反推以「止日末毫秒」为可逆锚点（自定义档上界 = 止日次日零点
     前 1ms——纳秒值超出 JS Number 2^53 安全整数后 1ns 尾差经双精度舍入完全丢失，
     毫秒级尾差可精确还原，见 resolveMtimeRangeNs 注释）。describeFacetState 为
     档位快照的摘要文案单点（保存弹窗预览消费，时间维展示解析后的绝对区间与还原
     口径一致，禁摘要显档位而数据存区间的两套口径）。 -->
<template>
  <el-popover placement="bottom-start" :width="400" trigger="click" popper-class="asset-filter-popper">
    <template #reference>
      <!-- badge 常显生效 facet 维度数（D2）：countActiveFacets 口径（0-3 维） -->
      <el-badge :value="activeCount" :hidden="activeCount === 0" class="facet-filter-badge">
        <el-button size="small" :disabled="disabled">筛选</el-button>
      </el-badge>
    </template>
    <div class="facet-filter-panel">
      <!-- 维度一：星级多选（0 = 未评分，与后端 files.rating=0 语义对齐） -->
      <div class="facet-field">
        <div class="facet-field-label">星级</div>
        <el-select
          v-model="ratingsModel"
          class="facet-rating-select"
          multiple
          collapse-tags
          clearable
          placeholder="全部星级"
          size="small"
        >
          <el-option v-for="opt in RATING_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
        </el-select>
      </div>
      <!-- 维度二：修改时间相对档（不限/今天/近 7 天/近 30 天/自定义）；自定义档
           追加日期区间选择器，区间完整才参与生效判定（半选不判活，防后端零
           条件空结果页误现「无命中」） -->
      <div class="facet-field">
        <div class="facet-field-label">修改时间</div>
        <el-radio-group v-model="mtimePresetModel" size="small" class="facet-radio-group">
          <el-radio v-for="opt in MTIME_PRESET_OPTIONS" :key="opt.value" :value="opt.value">{{ opt.label }}</el-radio>
        </el-radio-group>
        <el-date-picker
          v-if="mtimePresetModel === MTIME_PRESETS.CUSTOM"
          v-model="mtimeRangeModel"
          class="facet-date-range"
          type="daterange"
          size="small"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
        />
      </div>
      <!-- 维度三：文件大小分档（单选映射 sizeFrom/sizeTo 字节闭区间） -->
      <div class="facet-field">
        <div class="facet-field-label">文件大小</div>
        <el-radio-group v-model="sizePresetModel" size="small" class="facet-radio-group">
          <el-radio v-for="opt in SIZE_PRESET_OPTIONS" :key="opt.value" :value="opt.value">{{ opt.label }}</el-radio>
        </el-radio-group>
      </div>
      <!-- 重置：回默认档并回写（与宿主空态「清除全部筛选」按钮同语义，D6） -->
      <div class="facet-panel-footer">
        <el-button size="small" text type="primary" @click="resetFilters">重置</el-button>
      </div>
    </div>
  </el-popover>
</template>

<script>
// ---- 档位常量与换算（具名导出 = 换算单点，宿主 AssetsPanel 与单测共同消费）----

// 修改时间相对档取值（'custom' 走 mtimeRange 自定义区间，其余档在
// resolveMtimeRangeNs 内换算为绝对纳秒区间）
export const FACET_MTIME_PRESET = {
  ANY: 'any',
  TODAY: 'today',
  DAYS7: '7d',
  DAYS30: '30d',
  CUSTOM: 'custom'
}

// 文件大小分档取值（相邻档闭区间无缝衔接，见文件头换算口径说明）
export const FACET_SIZE_PRESET = {
  ANY: 'any',
  LT_1MB: 'lt1',
  FROM_1_TO_10MB: '1to10',
  FROM_10_TO_50MB: '10to50',
  GT_50MB: 'gt50'
}

// 三维选项常量（弹层控件与 describeFacetState 摘要/restoreFacetStateFromSnapshot
// 反推共用同一份，禁第二份字面量；模板经 setup 别名中转消费）
export const FACET_RATING_OPTIONS = [
  { value: 0, label: '未评分' },
  { value: 1, label: '1 星' },
  { value: 2, label: '2 星' },
  { value: 3, label: '3 星' },
  { value: 4, label: '4 星' },
  { value: 5, label: '5 星' }
]

export const FACET_MTIME_PRESET_OPTIONS = [
  { value: FACET_MTIME_PRESET.ANY, label: '不限' },
  { value: FACET_MTIME_PRESET.TODAY, label: '今天' },
  { value: FACET_MTIME_PRESET.DAYS7, label: '近 7 天' },
  { value: FACET_MTIME_PRESET.DAYS30, label: '近 30 天' },
  { value: FACET_MTIME_PRESET.CUSTOM, label: '自定义' }
]

export const FACET_SIZE_PRESET_OPTIONS = [
  { value: FACET_SIZE_PRESET.ANY, label: '不限' },
  { value: FACET_SIZE_PRESET.LT_1MB, label: '< 1MB' },
  { value: FACET_SIZE_PRESET.FROM_1_TO_10MB, label: '1-10MB' },
  { value: FACET_SIZE_PRESET.FROM_10_TO_50MB, label: '10-50MB' },
  { value: FACET_SIZE_PRESET.GT_50MB, label: '> 50MB' }
]

const NS_PER_MS = 1e6
const DAY_MS = 24 * 60 * 60 * 1000
const MB_BYTES = 1024 * 1024

// UI 档位快照默认值（宿主初始化与「清除全部筛选」复位共用同一形态，禁第二份字面量）
export function createDefaultFacetState() {
  return {
    ratings: [],
    mtimePreset: FACET_MTIME_PRESET.ANY,
    mtimeRange: null,
    sizePreset: FACET_SIZE_PRESET.ANY
  }
}

// 本地时区当日零点（毫秒）：setHours 归零按本地时区与夏令时规则解析，禁用
// 「当日零点 = now - 当日已过毫秒」式手算（DST 日期会偏移）
const startOfLocalDayMs = (ms) => {
  const d = new Date(ms)
  d.setHours(0, 0, 0, 0)
  return d.getTime()
}

// 本地时区次日零点（毫秒）：先走日历日 +1 再归零零点（禁 +24h 手算，DST 会偏 1 小时）
const startOfNextLocalDayMs = (ms) => {
  const d = new Date(ms)
  d.setDate(d.getDate() + 1)
  d.setHours(0, 0, 0, 0)
  return d.getTime()
}

// 自定义区间完整性：双端均非空才视为有效。半选状态不参与 facet 生效判定与
// 区间换算——防「选了自定义还没挑日期」时前端判活而后端 facet 条件为空
//（assetsFacetFilter.active 假），检索落入「空查询返回空页」误现「无命中」空态
const hasCompleteRange = (range) =>
  Array.isArray(range) && range.length === 2 && range[0] != null && range[1] != null

// 修改时间档 → 绝对纳秒闭区间（now 注入 = 相对档确定性换算点）。相对档上界取
// now：检索时刻之后再无更晚的 mtime，语义等价「截至检索时刻」；宿主每次检索
// 调用本函数现算，当天内重复检索的零点边界稳定、上界随检索时刻前进（防冻结
// 区间漏当日新文件）。自定义区间闭区间含止日全天（上界 = 止日次日零点前 1ms，
// 可逆性取舍见下方分支注释）。
export function resolveMtimeRangeNs(state, now = Date.now()) {
  const preset = state?.mtimePreset || FACET_MTIME_PRESET.ANY
  if (preset === FACET_MTIME_PRESET.TODAY) {
    return { mtimeFrom: startOfLocalDayMs(now) * NS_PER_MS, mtimeTo: now * NS_PER_MS }
  }
  if (preset === FACET_MTIME_PRESET.DAYS7) {
    return { mtimeFrom: (now - 7 * DAY_MS) * NS_PER_MS, mtimeTo: now * NS_PER_MS }
  }
  if (preset === FACET_MTIME_PRESET.DAYS30) {
    return { mtimeFrom: (now - 30 * DAY_MS) * NS_PER_MS, mtimeTo: now * NS_PER_MS }
  }
  if (preset === FACET_MTIME_PRESET.CUSTOM && hasCompleteRange(state?.mtimeRange)) {
    // 归一化 Date/毫秒数/可解析字符串三种入参形态（el-date-picker 无 value-format
    // 时下发 Date 对象，防御性兼容），再按本地零点口径取整
    const fromMs = new Date(state.mtimeRange[0]).getTime()
    const toMs = new Date(state.mtimeRange[1]).getTime()
    if (Number.isNaN(fromMs) || Number.isNaN(toMs)) {
      return { mtimeFrom: 0, mtimeTo: 0 }
    }
    return {
      mtimeFrom: startOfLocalDayMs(fromMs) * NS_PER_MS,
      // 上界 = 止日次日零点前 1ms（止日末毫秒）：闭区间含止日全天的语义在文件
      // 系统 mtime 实际粒度（NTFS 100ns / ext4 ns）下与「前 1ns」等价，取毫秒级
      // 尾差是为可逆性——纳秒值量级 ~1.8e18 超出 JS Number 2^53 安全整数，1ns
      // 尾差经双精度舍入（该量级 ULP≈256ns）完全丢失，保存的区间快照无法反推
      // 止日（第七批 PR3 调取回填需要 round-trip 可逆）；毫秒级尾差可精确还原，
      // 见 restoreFacetStateFromSnapshot / mtimeRangeFromNs
      mtimeTo: (startOfNextLocalDayMs(toMs) - 1) * NS_PER_MS
    }
  }
  // 'any' 与无效自定义（半选/非法日期）均不设界
  return { mtimeFrom: 0, mtimeTo: 0 }
}

// 文件大小档 → 字节闭区间（后端 SizeFrom/SizeTo，0 = 不设该侧边界）
export function resolveSizeRangeBytes(state) {
  const preset = state?.sizePreset || FACET_SIZE_PRESET.ANY
  if (preset === FACET_SIZE_PRESET.LT_1MB) return { sizeFrom: 0, sizeTo: MB_BYTES - 1 }
  if (preset === FACET_SIZE_PRESET.FROM_1_TO_10MB) return { sizeFrom: MB_BYTES, sizeTo: 10 * MB_BYTES - 1 }
  if (preset === FACET_SIZE_PRESET.FROM_10_TO_50MB) return { sizeFrom: 10 * MB_BYTES, sizeTo: 50 * MB_BYTES - 1 }
  if (preset === FACET_SIZE_PRESET.GT_50MB) return { sizeFrom: 50 * MB_BYTES, sizeTo: 0 }
  return { sizeFrom: 0, sizeTo: 0 }
}

// UI 档位快照 → SearchAssets opts 的 facet 字段片段（ratings/mtimeFrom/mtimeTo/
// sizeFrom/sizeTo，与 model.AssetSearchOptions 的 JSON tag 一一对应；ratings 空
// 数组与后端「不筛选」同义）。宿主每次检索调用现算（now 缺省当刻），禁缓存换算
// 结果——相对档边界须随检索时刻刷新。
export function buildFacetOptions(state, now = Date.now()) {
  const mtime = resolveMtimeRangeNs(state, now)
  const size = resolveSizeRangeBytes(state)
  return {
    ratings: Array.from(state?.ratings || []),
    mtimeFrom: mtime.mtimeFrom,
    mtimeTo: mtime.mtimeTo,
    sizeFrom: size.sizeFrom,
    sizeTo: size.sizeTo
  }
}

// 生效 facet 维度数（badge 口径，0-3）：星级非空 / 时间档非不限（自定义须区间
// 完整）/ 大小档非不限，各计 1 维。该计数同时是宿主 searchMode 的 facet 判定源
// （>0 即激活 facets-only 分页链路），与后端 newAssetsFacetFilter 的 active 判定
// 口径对齐（时间自定义半选不判活、ratings 空数组不筛选）
export function countActiveFacets(state) {
  if (!state) return 0
  let count = 0
  if (Array.isArray(state.ratings) && state.ratings.length > 0) count++
  if (state.mtimePreset === FACET_MTIME_PRESET.CUSTOM) {
    if (hasCompleteRange(state.mtimeRange)) count++
  } else if (state.mtimePreset && state.mtimePreset !== FACET_MTIME_PRESET.ANY) {
    count++
  }
  if (state.sizePreset && state.sizePreset !== FACET_SIZE_PRESET.ANY) count++
  return count
}

// ---- 保存检索条件快照反推与摘要（第七批 PR3，PRD D4/D7；与上方换算同源单点）----

// 纳秒 → 毫秒（Math.round 而非 floor：区间边界经 JSON 序列化传输存在 ±128ns 的
// 双精度舍入，floor 在舍入向下时会跨过整毫秒边界落错一日，round 恒落回原毫秒）
const nsToMs = (ns) => Math.round(ns / NS_PER_MS)

// 纳秒区间 → UI 自定义区间 [起日, 止日]（Date 形态，el-date-picker daterange 无
// value-format 时收发 Date 对象）。起日 = 存储下界所在日零点（换算口径恒零点对齐，
// 舍入后按日归零即原起日）；止日 = 存储上界所在日零点——自定义档上界 = 止日末毫秒，
// 归零即止日本身（round-trip 精确）；相对档（今天/近 N 天）冻结的上界 = 保存时刻，
// 归零为其所在日，两种形态统一按「上界所在日」还原
const mtimeRangeFromNs = (mtimeFromNs, mtimeToNs) => {
  const fromDate = new Date(nsToMs(mtimeFromNs))
  fromDate.setHours(0, 0, 0, 0)
  const toDate = new Date(nsToMs(mtimeToNs))
  toDate.setHours(0, 0, 0, 0)
  return [fromDate, toDate]
}

// 大小区间 → 档位反推：精确匹配四档闭区间（快照区间恒由本组件 resolveSizeRangeBytes
// 换算产出，精确匹配即完整还原）；未匹配（异常快照数据，如手改 data 文件）防御性
// 回退不限档并在此登记——无自定义大小档 UI，MVP 不为异常数据造档
const sizePresetFromRange = (sizeFrom, sizeTo) => {
  if (sizeFrom === 0 && sizeTo === MB_BYTES - 1) return FACET_SIZE_PRESET.LT_1MB
  if (sizeFrom === MB_BYTES && sizeTo === 10 * MB_BYTES - 1) return FACET_SIZE_PRESET.FROM_1_TO_10MB
  if (sizeFrom === 10 * MB_BYTES && sizeTo === 50 * MB_BYTES - 1) return FACET_SIZE_PRESET.FROM_10_TO_50MB
  if (sizeFrom === 50 * MB_BYTES && sizeTo === 0) return FACET_SIZE_PRESET.GT_50MB
  return FACET_SIZE_PRESET.ANY
}

// 保存的检索条件快照（model.AssetSavedSearch facet 字段）→ UI 档位快照（调取回填
// 单点，反推口径与 buildFacetOptions 同源）。星级按选项域归一（0-5 整数去重，异常
// 值丢弃防 el-select 渲染未知选项）；修改时间双侧非零还原为自定义区间冻结档（双侧
// 均 0 = 未设界回不限档；单侧非 0 为异常数据，回不限档登记——本组件换算恒双侧成对
// 产出，不产单侧值）；大小精确匹配四档。相对档不反推回原档位：快照只存区间原始值，
// 相对档保存时已解析为绝对区间冻结（保存的是什么区间调取就是什么区间，D7 完整还原）。
export function restoreFacetStateFromSnapshot(snapshot) {
  const state = createDefaultFacetState()
  const ratingValues = Array.isArray(snapshot?.ratings) ? snapshot.ratings : []
  state.ratings = Array.from(new Set(
    ratingValues.map(Number).filter(v => Number.isInteger(v) && v >= 0 && v <= 5)
  ))
  const mtimeFrom = Number(snapshot?.mtimeFrom) || 0
  const mtimeTo = Number(snapshot?.mtimeTo) || 0
  if (mtimeFrom > 0 && mtimeTo > 0) {
    state.mtimePreset = FACET_MTIME_PRESET.CUSTOM
    state.mtimeRange = mtimeRangeFromNs(mtimeFrom, mtimeTo)
  }
  state.sizePreset = sizePresetFromRange(Number(snapshot?.sizeFrom) || 0, Number(snapshot?.sizeTo) || 0)
  return state
}

// 纳秒 → 当地日 YYYY-MM-DD（摘要文案展示口径；按本地时区取日分量，与区间换算
// 的本地零点口径同源）
const formatNsDay = (ns) => {
  const d = new Date(nsToMs(ns))
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

// UI 档位快照 → 摘要文案（保存弹窗条件预览消费）。时间维统一展示解析后的绝对区间
//——相对档保存即冻结为区间，摘要与调取还原口径一致（禁摘要显档位而数据存区间的
// 两套口径）；now 注入 = 相对档区间换算的确定性驱动点（对齐 resolveMtimeRangeNs）
export function describeFacetState(state, now = Date.now()) {
  const ratingValues = Array.from(state?.ratings || [])
  const ratingText = ratingValues.length === 0
    ? '不限'
    : ratingValues
      .slice()
      .sort((a, b) => a - b)
      .map(v => FACET_RATING_OPTIONS.find(opt => opt.value === v)?.label || `${v} 星`)
      .join('、')
  const range = resolveMtimeRangeNs(state, now)
  const mtimeText = range.mtimeFrom === 0 && range.mtimeTo === 0
    ? '不限'
    : `${formatNsDay(range.mtimeFrom)} ~ ${formatNsDay(range.mtimeTo)}`
  const sizeText = FACET_SIZE_PRESET_OPTIONS.find(opt => opt.value === state?.sizePreset)?.label || '不限'
  return { ratingText, mtimeText, sizeText }
}
</script>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  // UI 档位快照（受控）：形状见 createDefaultFacetState；宿主持有，组件不落自持状态
  modelValue: { type: Object, default: null },
  disabled: { type: Boolean, default: false }
})
const emit = defineEmits(['update:modelValue', 'change'])

// 受控回写出口：update:modelValue 供 v-model 同步快照，change 供宿主触发语境
// 变更链（onFacetChange）。整体快照替换（不可变更新），防控件直接变异 props 对象
const emitState = (next) => {
  emit('update:modelValue', next)
  emit('change', next)
}

// 档位快照读取兜底：宿主未初始化（modelValue null）时按默认档渲染，不写回
const currentState = () => props.modelValue || createDefaultFacetState()

// 三维控件的受控代理（computed get 读 props / set 组装新快照整体回写）
const ratingsModel = computed({
  get: () => currentState().ratings || [],
  set: (values) => emitState({ ...currentState(), ratings: Array.from(values || []) })
})
const mtimePresetModel = computed({
  get: () => currentState().mtimePreset || FACET_MTIME_PRESET.ANY,
  set: (value) => emitState({ ...currentState(), mtimePreset: value })
})
// 切走自定义档保留已选区间（重开弹层可恢复，重置/清除才清）——区间不参与
// 非 custom 档的生效判定与换算，驻留无副作用
const mtimeRangeModel = computed({
  get: () => currentState().mtimeRange,
  set: (value) => emitState({ ...currentState(), mtimeRange: hasCompleteRange(value) ? [value[0], value[1]] : null })
})
const sizePresetModel = computed({
  get: () => currentState().sizePreset || FACET_SIZE_PRESET.ANY,
  set: (value) => emitState({ ...currentState(), sizePreset: value })
})

// badge 生效维度数（0 隐藏）
const activeCount = computed(() => countActiveFacets(props.modelValue))

// 重置：回默认档并回写（与宿主空态「清除全部筛选」按钮同语义，D6 一键复位）
const resetFilters = () => emitState(createDefaultFacetState())

// 模板消费的常量别名（普通 <script> 块的模块级绑定不保证进模板作用域，经 setup
// 绑定中转保模板可解析；别名不得与原常量同名——同名 const 声明会进入暂时性
// 死区自引用崩溃）
const MTIME_PRESETS = FACET_MTIME_PRESET
const RATING_OPTIONS = FACET_RATING_OPTIONS
const MTIME_PRESET_OPTIONS = FACET_MTIME_PRESET_OPTIONS
const SIZE_PRESET_OPTIONS = FACET_SIZE_PRESET_OPTIONS
</script>

<style scoped>
/* badge 与按钮基线对齐（el-badge 默认 inline-block 上标偏移会影响工具行垂直居中） */
.facet-filter-badge {
  display: inline-flex;
  align-items: center;
}

/* 弹层内容排版节奏（EP popper 自带白底/圆角/阴影，内容层只管字段分组） */
.facet-filter-panel {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  padding: var(--spacing-xs);
}

.facet-field {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.facet-field-label {
  font-size: 12px;
  color: var(--text-tertiary);
}

/* 星级多选占满弹层宽度 */
.facet-rating-select {
  width: 100%;
}

/* 单选组换行排布（五档选项在弹层宽度内一行放不下时整组折行，禁组内收缩挤压） */
.facet-radio-group {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs) var(--spacing-sm);
}

/* 日期区间占满弹层宽度（EP range editor 默认固定宽，deep 覆盖） */
.facet-field :deep(.el-date-editor) {
  width: 100%;
}

/* 底部操作行：右对齐重置入口，上缘分隔线与三维字段区隔 */
.facet-panel-footer {
  display: flex;
  justify-content: flex-end;
  border-top: 1px solid var(--border-light);
  padding-top: var(--spacing-sm);
}
</style>
