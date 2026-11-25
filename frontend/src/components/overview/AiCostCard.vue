<template>
  <OverviewCardShell
    title="AI 成本"
    :value="`$${formatUSD(ai.today?.costUsd)}`"
    :hint="`本周 $${formatUSD(ai.week?.costUsd)}`"
    :error="ai.error"
    @drill="$emit('drill', { range: 'week' })"
    @retry="$emit('retry')"
  >
    <div class="token-row">
      <span class="token-item">入 {{ formatTokens(ai.week?.inputTokens) }}</span>
      <span class="token-item">出 {{ formatTokens(ai.week?.outputTokens) }}</span>
      <span class="token-item">缓存读 {{ formatTokens(ai.week?.cacheReadTokens) }}</span>
      <span class="token-item">缓存写 {{ formatTokens(ai.week?.cacheCreationTokens) }}</span>
    </div>
    <div class="card-extra">基于近 90 天 / 最多 2000 条历史</div>
  </OverviewCardShell>
</template>

<script setup>
import OverviewCardShell from './OverviewCardShell.vue'

/**
 * M4 AI 成本卡片：今日成本为主数值，本周成本 + 本周 token 四项为辅。
 * 下钻载荷固定 range='week'（成本排行以周为口径更可读）。
 */
defineProps({
  ai: { type: Object, required: true }
})

defineEmits(['drill', 'retry'])

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
}
</style>
