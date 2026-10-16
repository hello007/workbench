<!-- frontend/src/components/AssetBatchRatingDialog.vue -->
<!-- 素材批量评分弹窗（素材管理第八批，SRS §2.6 P0 / PRD D4/D5）：对选中条目批量
     设置星级。语义 = 每条目一事务逐条目提交，部分失败不中断（成功条目已写入）。
     契约（形态复刻 AssetBatchTagDialog 先例）：
     - 宿主分派（弹窗不感知 viewMode）：本组件只收 targets（[{fileId, slideNum?,
       label}]，label 为宿主按 viewMode 拼装的定位展示文案）与 execute 分派回调
       ((rating) => Promise<AssetBatchRatingResult>)——宿主按打开时构建的 targets
       调 BatchSetAssetRating（单一入口混合文件/页目标，slideNum 0 = 文件条目），
       本层零 viewMode 分支；
     - 星级单选 0-5（el-rate，clearable 支持回落 0），0 = 清除评分（后端将
       files.rating / slide_meta.rating 置 0，与单条编辑「0 = 清除」语义一致）；
     - 结果反馈对齐批量打标签先例：全部成功 toast 后关闭；有失败弹窗内切结果视图
       （成功 X / 失败 Y 汇总 + 失败逐条定位明细 + message）——成功条目已写入
       不回滚，故 succeeded > 0 时照常上报 completed 触发宿主刷新；
     - 选中集合处理留宿主：本组件只上报 completed，宿主刷新链保留选中（数据变更
       点位非语境变更，清空点位矩阵不增行）；
     - 失败明细定位：后端逐条目返回拼装 Label（文件路径 [+ · 第 N 页]）为第一
       数据源；Label 缺省时回退 targets 反查（slideNum||0 双侧归一），再 miss
       合成兜底文案（对齐 AssetBatchTagDialog 反查契约）。
     视觉：弹窗 560 档（中小弹窗分档，对齐 frontend-visual-conventions 弹窗尺寸
     策略），top vh 响应式；失败明细行 = danger tag + 定位文案 + 原因。 -->
<template>
  <el-dialog
    v-model="visible"
    title="批量评分"
    width="min(560px, 86vw)"
    top="15vh"
    :close-on-click-modal="false"
    append-to-body
  >
    <!-- 表单视图：星级单选（0 = 清除评分） -->
    <div v-if="!resultVisible" class="batch-rating-body">
      <p class="batch-rating-scope">
        将对选中的 {{ targets.length }} 项设置星级：文件条目评文件星级，幻灯片页条目评页星级（互不影响）
      </p>
      <div class="batch-rating-section">
        <span class="batch-rating-label">星级（0 = 清除评分）</span>
        <div class="batch-rating-picker">
          <el-rate
            v-model="rating"
            :max="5"
            :allow-half="false"
            clearable
            :disabled="submitting"
          />
          <span class="batch-rating-hint">{{ ratingLabel }}</span>
        </div>
      </div>
    </div>

    <!-- 结果视图（有失败时切换）：汇总行 + 失败逐条定位明细，仅展示可关闭 -->
    <div v-else class="batch-rating-result">
      <div class="batch-rating-summary">
        <span class="summary-ok">成功 {{ resultSummary.succeeded }} 项</span>
        <span class="summary-failed">失败 {{ resultSummary.failed }} 项</span>
      </div>
      <!-- 部分失败不中断语义明示：成功条目已写入，失败项可关闭后重试 -->
      <p class="batch-rating-result-hint">
        成功条目已写入，不受失败条目影响；可关闭后对失败项单独重试。
      </p>
      <div class="batch-rating-failed-list">
        <div v-for="(item, i) in failedItems" :key="i" class="batch-rating-failed-row">
          <el-tag size="small" type="danger">失败</el-tag>
          <span class="failed-target" :title="item.label">{{ item.label }}</span>
          <span v-if="item.message" class="failed-msg" :title="item.message">{{ item.message }}</span>
        </div>
      </div>
    </div>

    <template #footer>
      <template v-if="!resultVisible">
        <el-button :disabled="submitting" @click="visible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">提交</el-button>
      </template>
      <el-button v-else type="primary" @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { handleError } from '../utils/error'

const props = defineProps({
  // 弹窗显隐（v-model）
  modelValue: { type: Boolean, default: false },
  // 批量目标清单 [{ fileId, slideNum?, label }]：label 为宿主按 viewMode 拼装的
  // 定位展示文案，供失败明细行 targets 反查兜底（后端 Label 为第一数据源）
  targets: { type: Array, default: () => [] },
  // 分派执行回调 (rating) => Promise<model.AssetBatchRatingResult>：宿主以打开时
  // 构建的 targets 调 BatchSetAssetRating（本层零 viewMode 分支，契约见文件头）
  execute: { type: Function, default: null }
})

const emit = defineEmits(['update:modelValue', 'completed'])

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

// ---- 表单态 ----
const rating = ref(0)
const submitting = ref(false)

// 当前选择的语义说明（0 = 清除评分，与后端「0 = 清除」契约一致）
const ratingLabel = computed(() =>
  rating.value === 0 ? '清除评分（移除所选条目的星级）' : `${rating.value} 星`
)

// ---- 结果态 ----
const resultVisible = ref(false)
const resultData = ref(null)
const resultSummary = computed(() => ({
  succeeded: resultData.value?.succeeded ?? 0,
  failed: resultData.value?.failed ?? 0
}))

// 失败明细（定位信息 = 后端 Label 优先，targets 反查兜底，再 miss 合成兜底文案）：
// 结果条目 (fileId, slideNum) 与提交目标一一对应（顺序同提交序），slideNum 缺省
// 按 0 归一（文件条目后端省略该字段，slideNum||0 双侧归一契约）
const failedItems = computed(() => {
  const items = resultData.value?.items || []
  return items
    .filter(item => item.status !== 'ok')
    .map(item => {
      const target = props.targets.find(t =>
        t.fileId === item.fileId && (t.slideNum || 0) === (item.slideNum || 0)
      )
      const fallback = `文件 #${item.fileId}${item.slideNum ? ` · 第 ${item.slideNum} 页` : ''}`
      return { ...item, label: item.label || target?.label || fallback }
    })
})

// 打开即复位表单/结果态（防上次会话残留：结果视图、星级选择、提交中态）
watch(visible, (val) => {
  if (!val) return
  rating.value = 0
  resultVisible.value = false
  resultData.value = null
  submitting.value = false
})

const onSubmit = async () => {
  if (submitting.value || typeof props.execute !== 'function') return
  submitting.value = true
  try {
    const result = await props.execute(rating.value)
    // 部分失败不中断：succeeded > 0 即有数据写入，completed 照常上报宿主刷新链
    //（选中保留，宿主不清选中）
    if ((result?.succeeded ?? 0) > 0) emit('completed')
    if ((result?.failed ?? 0) === 0) {
      ElMessage.success(rating.value === 0
        ? `已清除 ${result?.succeeded ?? 0} 项评分`
        : `已为 ${result?.succeeded ?? 0} 项设置 ${rating.value} 星`)
      visible.value = false
    } else {
      // 有失败：弹窗内切结果视图（汇总 + 失败逐条定位明细），不自动关闭
      resultData.value = result
      resultVisible.value = true
    }
  } catch (error) {
    // 整体失败（rating 越界/空目标/库不存在等参数拒绝）经 handleError 统一分流
    handleError('批量评分失败: ', error)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.batch-rating-body {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.batch-rating-scope {
  margin: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

.batch-rating-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.batch-rating-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.batch-rating-picker {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.batch-rating-hint {
  font-size: 12px;
  color: var(--text-tertiary);
}

/* ---- 结果视图（形态对齐 AssetBatchTagDialog 结果视图） ---- */
.batch-rating-result {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.batch-rating-summary {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  font-size: 13px;
  font-weight: 500;
}

.summary-ok {
  color: var(--success-color);
}

.summary-failed {
  color: var(--danger-color);
}

.batch-rating-result-hint {
  margin: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

/* 失败明细列表：高度上限内滚（内容多大滚动，对齐弹窗尺寸策略） */
.batch-rating-failed-list {
  max-height: min(280px, 40vh);
  overflow-y: auto;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-xs);
}

.batch-rating-failed-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
  min-width: 0;
}

.failed-target {
  flex-shrink: 0;
  max-width: 55%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--text-primary);
}

.failed-msg {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--text-tertiary);
  min-width: 0;
}
</style>
