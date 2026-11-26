<template>
  <OverviewCardShell
    title="AI 成本"
    :value="`$${formatUSD(selected.cost?.costUsd)}`"
    :hint="hint"
    :error="ai.error"
    @drill="$emit('drill', { range: rangeKey })"
    @retry="$emit('retry')"
  >
    <div class="token-row">
      <span class="token-item">入 {{ formatTokens(selected.cost?.inputTokens) }}</span>
      <span class="token-item">出 {{ formatTokens(selected.cost?.outputTokens) }}</span>
      <span class="token-item">缓存读 {{ formatTokens(selected.cost?.cacheReadTokens) }}</span>
      <span class="token-item">缓存写 {{ formatTokens(selected.cost?.cacheCreationTokens) }}</span>
    </div>
    <div class="card-extra">{{ rangeText }}</div>
  </OverviewCardShell>
</template>

<script setup>
import { computed } from 'vue'
import OverviewCardShell from './OverviewCardShell.vue'
import { overviewRangeText } from './overviewRange'

/**
 * M4 AI 成本卡片：所选档位成本为主数值，token 四项随档位；
 * hint 展示未入选的对比基准（今日 / 本周），保证数字与范围不脱节。
 */
const props = defineProps({
  ai: { type: Object, required: true }
})

defineEmits(['drill', 'retry'])

const rangeKey = computed(() => props.ai.rangeKey || 'week')
const rangeText = computed(() => overviewRangeText(rangeKey.value))
// 顶层 today/week 是无条件返回的对比基准（不随所选档位变化）。
const selected = computed(() => props.ai.selected || { cost: {} })
const todayCost = computed(() => props.ai.today?.costUsd)
const weekCost = computed(() => props.ai.week?.costUsd)

const hint = computed(() => {
  if (rangeKey.value === 'week') return `今日 $${formatUSD(todayCost.value)}`
  if (rangeKey.value === 'today') return `本周 $${formatUSD(weekCost.value)}`
  return `今日 $${formatUSD(todayCost.value)} · 本周 $${formatUSD(weekCost.value)}`
})

function formatUSD(value) {
  const num = Number(value) || 0
  if (num === 0) return '0.00'
  return num < 0.01 ? num.toFixed(4) : num.toFixed(2)
}

function formatTokens(value) {
  const num = Number(value) || 0
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`
  if (num >= 1000) return `${(num / 1000).toFixed(1)}k`
  return String(num)
}
</script>

<style scoped>
.token-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs) var(--spacing-sm);
  font-size: 12px;
  color: var(--text-secondary);
}

.token-item {
  white-space: nowrap;
}

.card-extra {
  margin-top: auto;
  font-size: 11px;
  color: var(--text-tertiary);
  line-height: 1.5;
}
</style>
