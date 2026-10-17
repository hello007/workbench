<!-- frontend/src/components/AssetPrefixReplaceDialog.vue -->
<!-- 素材路径前缀替换弹窗（素材管理第九批 PR2，SRS §2.6.5 根目录前缀替换工具，
     工具行「维护」菜单入口）：库内子目录重组后按前缀批量重链索引路径。
     契约（形态对齐 AssetExportDialog 宿主分派先例）：
     - 命中预览与执行分派留宿主：本组件收 preview / execute 回调 props（本层零
       绑定 import，便于 spec 注入断言），preview 防抖驱动实时显示命中数（输入
       变更 300ms 后查询，快速键入不刷库）；命中口径（目录边界语义）在后端单点，
       本层只透传归一化前的原始输入；
     - 二次确认（D7）：提交先 ElMessageBox.confirm 列出命中数与新旧前缀，确认后
       才调 execute（后端再做零命中/越界/冲突校验，双防线）；
     - 执行期间 submitting 态（后端单事务替换 + 逐条探测 + 自动增量扫描，可能
       数秒）；完成（成功或失败）一律 emit completed——替换为单事务原子提交，
       自动扫描失败时替换已提交，宿主两种终态都需要刷新；结果 toast 按三态计数
       分型（复活/重解析/保持丢失）；
     - 错误分流：零命中 / 越界为确认型 warning 码（WARNING_CODES），经 handleError
       自动 warning 分流（弹窗保持打开可改前缀重试）。 -->
<template>
  <el-dialog
    v-model="visible"
    title="路径前缀替换"
    width="min(560px, 86vw)"
    top="15vh"
    :close-on-click-modal="false"
    append-to-body
  >
    <div class="prefix-replace-body">
      <p class="prefix-hint">
        目录重组后索引路径失效时，把命中旧前缀的全部记录批量改指新前缀（库内相对路径，
        如「2023方案」→「2024方案」）。
      </p>
      <div class="prefix-field">
        <span class="prefix-label">旧前缀</span>
        <el-input
          v-model="oldPrefix"
          placeholder="例如 2023方案"
          :disabled="submitting"
          @input="schedulePreview"
        />
      </div>
      <div class="prefix-field">
        <span class="prefix-label">新前缀</span>
        <el-input
          v-model="newPrefix"
          placeholder="例如 2024方案"
          :disabled="submitting"
          @keyup.enter="onSubmit"
          @input="schedulePreview"
        />
      </div>
      <div class="prefix-preview" :class="{ 'is-empty': previewCount === 0 }">
        <template v-if="previewCount === null">命中统计中…</template>
        <template v-else>命中 <span class="prefix-count">{{ previewCount }}</span> 条索引记录</template>
      </div>
      <div class="prefix-notice">
        执行后逐条核对新位置：内容未变直接恢复，已变化重新解析，缺失保持「丢失」标记；
        完成后自动触发一次增量扫描。
      </div>
    </div>
    <template #footer>
      <el-button :disabled="submitting" @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="onSubmit">替换</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { handleError } from '../utils/error'

const props = defineProps({
  // 弹窗显隐（v-model）
  modelValue: { type: Boolean, default: false },
  // 命中数预览回调 (oldPrefix, newPrefix) => Promise<number>：宿主持有
  // PreviewAssetPathPrefixReplace 绑定（本层零绑定 import）
  preview: { type: Function, default: null },
  // 执行回调 (oldPrefix, newPrefix) => Promise<model.AssetPrefixReplaceResult>：
  // 宿主持有 ReplaceAssetPathPrefix 绑定
  execute: { type: Function, default: null }
})

const emit = defineEmits(['update:modelValue', 'completed'])

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const oldPrefix = ref('')
const newPrefix = ref('')
const submitting = ref(false)
// 命中数（null = 统计中占位；0 = 零命中合法终态，非异常但禁提交）
const previewCount = ref(null)

// 防抖预览（300ms）：快速键入不逐键刷库；代次守卫丢弃过期响应（连続输入时仅
// 最后一次查询写回，与面板 searchSeq 同款思路的最小实现）
let previewTimer = null
let previewSeq = 0
const schedulePreview = () => {
  if (previewTimer) clearTimeout(previewTimer)
  // 输入即失效旧命中数（回「统计中」占位）：300ms 防抖窗口与在途请求期间不得
  // 展示旧前缀结果——陈旧 0 会误拦合法提交（onSubmit 零命中拦截），陈旧非 0 会
  // 误导二次确认文案；预览口径由后端执行时兜底校验
  previewCount.value = null
  previewTimer = setTimeout(runPreview, 300)
}

const runPreview = async () => {
  if (typeof props.preview !== 'function') return
  if (!oldPrefix.value.trim() || !newPrefix.value.trim()) {
    previewCount.value = null // 前缀未填全：不查询（空串查询是参数错误非零命中）
    return
  }
  const seq = ++previewSeq
  try {
    const n = await props.preview(oldPrefix.value.trim(), newPrefix.value.trim())
    if (seq !== previewSeq) return
    previewCount.value = n
  } catch (error) {
    if (seq !== previewSeq) return
    previewCount.value = null
    // 非法前缀（空串/逃逸形态）经后端参数错误透出：warning 分流由 handleError
    // 按码承担，此处静默兜底防未分类错误无反馈
    handleError('统计命中失败: ', error)
  }
}

const onSubmit = async () => {
  if (submitting.value || typeof props.execute !== 'function') return
  const oldVal = oldPrefix.value.trim()
  const newVal = newPrefix.value.trim()
  // 前端第一防线（后端同口径校验，双防线）：前缀必填
  if (!oldVal || !newVal) {
    ElMessage.warning('请填写旧前缀与新前缀')
    return
  }
  if (previewCount.value === 0) {
    ElMessage.warning('旧前缀没有命中的索引记录，请核对拼写')
    return
  }
  // 二次确认（D7）：命中数 + 新旧前缀列出（previewCount 为 null 时以「未统计」
  // 文案兜底——用户可直接提交，后端命中校验兜底）
  const hitText = previewCount.value === null ? '（未统计，执行时由后端校验）' : ` ${previewCount.value} 条`
  try {
    await ElMessageBox.confirm(
      `旧前缀「${oldVal}」命中${hitText}记录，将批量改指新前缀「${newVal}」。确定执行？`,
      '确认替换',
      { type: 'warning', confirmButtonText: '替换', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  submitting.value = true
  try {
    const result = await props.execute(oldVal, newVal)
    const parts = [`更新 ${result?.replaced ?? 0} 条路径`]
    if (result?.revived) parts.push(`恢复 ${result.revived}`)
    if (result?.reparsed) parts.push(`重解析 ${result.reparsed}`)
    if (result?.stillLost) parts.push(`${result.stillLost} 条新位置缺失保持丢失`)
    ElMessage.success(`前缀替换完成：${parts.join('，')}，已自动触发增量扫描`)
    visible.value = false
  } catch (error) {
    handleError('前缀替换失败: ', error)
  } finally {
    submitting.value = false
    // 成败均上报 completed：替换为单事务原子提交，自动扫描失败时替换已提交，
    // 宿主两种终态都需要刷新（幂等刷新无副作用）
    emit('completed')
  }
}
</script>

<style scoped>
.prefix-replace-body {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.prefix-hint,
.prefix-notice {
  margin: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

.prefix-field {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.prefix-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.prefix-preview {
  font-size: 13px;
  color: var(--text-primary);
}

.prefix-preview.is-empty {
  color: var(--text-tertiary);
}

.prefix-count {
  font-weight: 600;
  color: var(--primary-color);
}
</style>
