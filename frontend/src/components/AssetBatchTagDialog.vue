<!-- frontend/src/components/AssetBatchTagDialog.vue -->
<!-- 素材批量打标签弹窗（素材管理第六批 PR3，SRS §2.4 / PRD D3/D4/D5）：对选中
     条目批量追加/移除标签。语义 = 每条目「现标签集 − removeTags ∪ addTags」
     （相对各条目现状追加/移除，非整体替换），只动标签不动星级与备注。
     契约：
     - 宿主分派（弹窗不感知 viewMode）：本组件只收 targets（[{fileId, slideNum?,
       label}]，label 为宿主按 viewMode 拼装的定位展示文案）与 execute 分派回调
       ((op) => Promise<AssetBatchTagResult>)——宿主按 viewMode 决定调
       BatchUpdateAssetFileTags（文件模式，fileIDs 数组）或
       BatchUpdateAssetSlideTags（幻灯片模式，(fileId, slideNum) 目标数组）
       （PRD D3 两独立方法，存储形态分叉收敛宿主单点，本层零分支）；
     - op 为标签名数组（后端 AssetBatchTagOp 契约：不存在的添加名自动创建为根
       标签，沿用默认色板）——树组件统一 ID 口径（PRD D6），ID→名称映射经
       loaded 事件回传的平铺标签清单在宿主弹窗侧完成；
     - 双防线对齐 PRD D4：添加/移除交集非空提交禁用 + 提示（后端
       normalizeBatchTagOp 亦校验拒绝）；两区全空提交禁用（后端亦拒绝）；
     - 结果反馈对齐批量拆分 splitOutcome 明细先例：全部成功 toast 后关闭；
       有失败弹窗内切结果视图（成功 X / 失败 Y 汇总 + 失败逐条定位明细 +
       message）——批量「每条目一事务部分失败不中断」（PRD D3），成功条目已
       写入不回滚，故 succeeded > 0 时照常上报 completed 触发宿主刷新；
     - 选中集合处理留宿主：本组件只上报 completed，宿主刷新链保留选中
       （PRD D5：数据变更点位非语境变更，不清空点位矩阵不增行）。
     视觉：弹窗 560 档（中小弹窗分档，对齐 frontend-visual-conventions 弹窗
     尺寸策略），top vh 响应式；失败明细行 = danger tag + 定位文案 + 原因，
     状态色按语义色（danger = 失效/失败语义）。 -->
<template>
  <el-dialog
    v-model="visible"
    title="批量打标签"
    width="min(560px, 86vw)"
    top="15vh"
    :close-on-click-modal="false"
    append-to-body
  >
    <!-- 表单视图：添加/移除两分区，各挂一个标签树多选（单组件复用，PRD D6） -->
    <div v-if="!resultVisible" class="batch-tag-body">
      <p class="batch-tag-scope">
        将对选中的 {{ targets.length }} 项更新标签：添加/移除相对各条目现状生效，星级与备注不受影响
      </p>
      <div class="batch-tag-section">
        <span class="batch-tag-label">添加标签</span>
        <AssetTagTreeSelect
          v-model="addTagIds"
          class="batch-tag-select"
          :library-id="libraryId"
          placeholder="选择要添加的标签"
          empty-text="暂无标签，可先在「管理标签」中新建"
          :disabled="submitting"
          @loaded="onTagsLoaded"
        />
      </div>
      <div class="batch-tag-section">
        <span class="batch-tag-label">移除标签</span>
        <AssetTagTreeSelect
          v-model="removeTagIds"
          class="batch-tag-select"
          :library-id="libraryId"
          placeholder="选择要移除的标签"
          empty-text="暂无标签，可先在「管理标签」中新建"
          :disabled="submitting"
          @loaded="onTagsLoaded"
        />
      </div>
      <!-- 交集防线（PRD D4 前端侧）：后端 normalizeBatchTagOp 亦拒绝，双防线 -->
      <div v-if="conflictNames.length > 0" class="batch-tag-conflict">
        标签「{{ conflictNames.join('、') }}」不能同时添加与移除
      </div>
    </div>

    <!-- 结果视图（有失败时切换）：汇总行 + 失败逐条定位明细，仅展示可关闭 -->
    <div v-else class="batch-tag-result">
      <div class="batch-tag-summary">
        <span class="summary-ok">成功 {{ resultSummary.succeeded }} 项</span>
        <span class="summary-failed">失败 {{ resultSummary.failed }} 项</span>
      </div>
      <!-- 部分失败不中断语义明示（PRD D3）：成功条目已写入，失败项可关闭后重试 -->
      <p class="batch-tag-result-hint">
        成功条目已写入，不受失败条目影响；可关闭后对失败项单独重试。
      </p>
      <div class="batch-tag-failed-list">
        <div v-for="(item, i) in failedItems" :key="i" class="batch-tag-failed-row">
          <el-tag size="small" type="danger">失败</el-tag>
          <span class="failed-target" :title="item.label">{{ item.label }}</span>
          <span v-if="item.message" class="failed-msg" :title="item.message">{{ item.message }}</span>
        </div>
      </div>
    </div>

    <template #footer>
      <template v-if="!resultVisible">
        <el-button :disabled="submitting" @click="visible = false">取消</el-button>
        <!-- 提交中 loading 防重复提交；交集/全空禁用为前端防线（后端双防线） -->
        <el-button type="primary" :loading="submitting" :disabled="!canSubmit" @click="onSubmit">提交</el-button>
      </template>
      <el-button v-else type="primary" @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import AssetTagTreeSelect from './AssetTagTreeSelect.vue'
import { handleError } from '../utils/error'

const props = defineProps({
  // 弹窗显隐（v-model）
  modelValue: { type: Boolean, default: false },
  // 所属素材库 ID（透传标签树组件拉取 ListAssetTags）
  libraryId: { type: String, default: '' },
  // 批量目标清单 [{ fileId, slideNum?, label }]：label 为宿主按 viewMode 拼装的
  // 定位展示文案（文件模式 = 文件路径；幻灯片模式 = 「文件路径 · 第 N 页」），
  // 供失败明细行定位展示与结果条目反查
  targets: { type: Array, default: () => [] },
  // 分派执行回调 (op) => Promise<model.AssetBatchTagResult>：宿主按 viewMode
  // 决定调 BatchUpdateAssetFileTags 或 BatchUpdateAssetSlideTags（见文件头契约）
  execute: { type: Function, default: null }
})

const emit = defineEmits(['update:modelValue', 'completed'])

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

// ---- 表单态 ----
const addTagIds = ref([])
const removeTagIds = ref([])
const submitting = ref(false)

// 标签平铺清单（ID→名称映射数据源）：两树组件同数据源，loaded 事件任一回传
// 即整体替换（Vue 数组整体替换响应式纪律，禁原地变异）
const flatTags = ref([])
const onTagsLoaded = (items) => {
  flatTags.value = items || []
}
const tagNameById = computed(() => new Map(flatTags.value.map(t => [t.id, t.name])))

// 交集防线：两区勾选的同一标签（ID 口径，标签库内唯一一名一 ID）映射为名称提示
const conflictNames = computed(() => {
  const removeSet = new Set(removeTagIds.value)
  return addTagIds.value
    .filter(id => removeSet.has(id))
    .map(id => tagNameById.value.get(id) || `#${id}`)
})

// 提交可用性：两区全空禁用（PRD D4 后端亦校验）+ 交集非空禁用 + 提交中禁用
const canSubmit = computed(() =>
  !submitting.value &&
  (addTagIds.value.length > 0 || removeTagIds.value.length > 0) &&
  conflictNames.value.length === 0
)

// ---- 结果态 ----
const resultVisible = ref(false)
const resultData = ref(null)
const resultSummary = computed(() => ({
  succeeded: resultData.value?.succeeded ?? 0,
  failed: resultData.value?.failed ?? 0
}))

// 失败明细（定位信息 = targets 反查 label）：结果条目 (fileId, slideNum) 与提交
// 目标一一对应（顺序同提交序），slideNum 缺省按 0 归一（文件条目后端省略该字段）
const failedItems = computed(() => {
  const items = resultData.value?.items || []
  return items
    .filter(item => item.status !== 'ok')
    .map(item => {
      const target = props.targets.find(t =>
        t.fileId === item.fileId && (t.slideNum || 0) === (item.slideNum || 0)
      )
      const fallback = `文件 #${item.fileId}${item.slideNum ? ` · 第 ${item.slideNum} 页` : ''}`
      return { ...item, label: target?.label || fallback }
    })
})

// 打开即复位表单/结果态（防上次会话残留：结果视图、勾选、提交中态）
watch(visible, (val) => {
  if (!val) return
  addTagIds.value = []
  removeTagIds.value = []
  resultVisible.value = false
  resultData.value = null
  submitting.value = false
})

const onSubmit = async () => {
  if (!canSubmit.value || typeof props.execute !== 'function') return
  submitting.value = true
  // ID→名称映射（后端 AssetBatchTagOp 契约为标签名数组，不存在的添加名自动创建）
  const op = {
    addTags: addTagIds.value.map(id => tagNameById.value.get(id)).filter(Boolean),
    removeTags: removeTagIds.value.map(id => tagNameById.value.get(id)).filter(Boolean)
  }
  try {
    const result = await props.execute(op)
    // 部分失败不中断（PRD D3）：succeeded > 0 即有数据写入，completed 照常上报
    // 宿主刷新链（选中保留 D5，宿主不清选中）
    if ((result?.succeeded ?? 0) > 0) emit('completed')
    if ((result?.failed ?? 0) === 0) {
      ElMessage.success(`已为 ${result?.succeeded ?? 0} 项更新标签`)
      visible.value = false
    } else {
      // 有失败：弹窗内切结果视图（汇总 + 失败逐条定位明细），不自动关闭
      resultData.value = result
      resultVisible.value = true
    }
  } catch (error) {
    // 整体失败（参数校验拒绝/库不存在等）经 handleError 统一分流
    handleError('批量打标签失败: ', error)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.batch-tag-body {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.batch-tag-scope {
  margin: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

.batch-tag-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.batch-tag-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

/* 交集防线提示（warning 语义色派生，对齐 frontend-visual-conventions 语义色规则） */
.batch-tag-conflict {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: 12px;
  color: var(--warning-color);
  background: color-mix(in srgb, var(--warning-color) 12%, transparent);
  border-radius: var(--radius-md);
}

/* ---- 结果视图 ---- */
.batch-tag-result {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.batch-tag-summary {
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

.batch-tag-result-hint {
  margin: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

/* 失败明细列表：高度上限内滚（内容多大滚动，对齐弹窗尺寸策略） */
.batch-tag-failed-list {
  max-height: min(280px, 40vh);
  overflow-y: auto;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-xs);
}

.batch-tag-failed-row {
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
