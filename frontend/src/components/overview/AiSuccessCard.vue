<template>
  <OverviewCardShell
    title="AI 成功率"
    :value="displayValue"
    :hint="hint"
    :error="ai.error"
    @drill="$emit('drill', drillPayload)"
    @retry="$emit('retry')"
  >
    <div class="card-extras">
      <!-- 四状态计数来自后端 Stats，用户可据此自行验算成功率 -->
      <div v-if="hasData" class="card-extra">
        成功 {{ ai.successCount || 0 }} · 失败 {{ ai.failedCount || 0 }} · 超时 {{ ai.timeoutCount || 0 }} · 取消 {{ ai.canceledCount || 0 }}
      </div>
      <div v-else-if="ai.canceledCount" class="card-extra">本周 {{ ai.canceledCount }} 个任务均被取消</div>
      <div class="card-extra">本周 success ÷（success+failed+timeout），已取消不进分母</div>
      <div class="card-extra">基于近 90 天 / 最多 2000 条历史</div>
    </div>
  </OverviewCardShell>
</template>

<script setup>
import { computed } from 'vue'
import OverviewCardShell from './OverviewCardShell.vue'

/**
 * M5 AI 成功率卡片：本周 success ÷（success+failed+timeout）。
 * canceled 不进分母；周期内无有效任务显示「无数据」而不是 0%。
 */
const props = defineProps({
  ai: { type: Object, required: true }
})

defineEmits(['drill', 'retry'])

const hasData = computed(() => !!props.ai.hasData)
const successRate = computed(() => Number(props.ai.successRate) || 0)
const displayValue = computed(() => (hasData.value ? `${(successRate.value * 100).toFixed(1)}%` : '无数据'))
const hint = computed(() => (hasData.value ? '本周 AI 任务' : '本周暂无有效任务'))

/**
 * 下钻载荷按卡片所处状态分支，保证抽屉里的列表与卡片说明一致：
 * - 有有效任务：预选失败+超时，查看「拉低成功率」的任务；
 * - 无有效任务但存在取消：卡片显示「均被取消」，此时预选取消，避免点开得到 0 条；
 * - 其余（无数据且无取消）：回落失败+超时，与卡片默认下钻语义一致。
 */
const drillPayload = computed(() => {
  if (hasData.value) return { range: 'week', statuses: ['failed', 'timeout'] }
  if (props.ai.canceledCount) return { range: 'week', statuses: ['canceled'] }
  return { range: 'week', statuses: ['failed', 'timeout'] }
})
</script>

<style scoped>
.card-extras {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  margin-top: auto;
}

.card-extra {
  font-size: 11px;
  color: var(--text-tertiary);
  line-height: 1.5;
}
</style>
