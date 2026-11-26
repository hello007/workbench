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
      <!-- 四状态计数随所选档位，用户可据此自行验算成功率 -->
      <div v-if="hasData" class="card-extra">
        成功 {{ selected.successCount || 0 }} · 失败 {{ selected.failedCount || 0 }} · 超时 {{ selected.timeoutCount || 0 }} · 取消 {{ selected.canceledCount || 0 }}
      </div>
      <div v-else-if="selected.canceledCount" class="card-extra">{{ rangeLabel }} {{ selected.canceledCount }} 个任务均被取消</div>
      <div class="card-extra">{{ rangeLabel }} success ÷（success+failed+timeout），已取消不进分母</div>
      <div class="card-extra">{{ rangeText }}</div>
    </div>
  </OverviewCardShell>
</template>

<script setup>
import { computed } from 'vue'
import OverviewCardShell from './OverviewCardShell.vue'
import { overviewRangeLabel, overviewRangeText } from './overviewRange'

/**
 * M5 AI 成功率卡片：所选档位 success ÷（success+failed+timeout）。
 * canceled 不进分母；档位内无有效任务显示「无数据」而不是 0%。
 */
const props = defineProps({
  ai: { type: Object, required: true }
})

defineEmits(['drill', 'retry'])

const rangeKey = computed(() => props.ai.rangeKey || 'week')
const rangeLabel = computed(() => overviewRangeLabel(rangeKey.value))
const rangeText = computed(() => overviewRangeText(rangeKey.value))
const selected = computed(() => props.ai.selected || {})

const hasData = computed(() => !!selected.value.hasData)
const successRate = computed(() => Number(selected.value.successRate) || 0)
const displayValue = computed(() => (hasData.value ? `${(successRate.value * 100).toFixed(1)}%` : '无数据'))
const hint = computed(() => (hasData.value ? `${rangeLabel.value} AI 任务` : `${rangeLabel.value}暂无有效任务`))

/**
 * 下钻载荷按卡片所处状态分支，保证抽屉里的列表与卡片说明一致：
 * - 有有效任务：预选失败+超时，查看「拉低成功率」的任务；
 * - 无有效任务但存在取消：卡片显示「均被取消」，此时预选取消，避免点开得到 0 条；
 * - 其余（无数据且无取消）：回落失败+超时，与卡片默认下钻语义一致。
 * range 带当前档位，抽屉按同一窗口过滤。
 */
const drillPayload = computed(() => {
  if (hasData.value) return { range: rangeKey.value, statuses: ['failed', 'timeout'] }
  if (selected.value.canceledCount) return { range: rangeKey.value, statuses: ['canceled'] }
  return { range: rangeKey.value, statuses: ['failed', 'timeout'] }
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
