<template>
  <OverviewCardShell
    title="AI 成功率"
    :value="displayValue"
    :hint="hint"
    :error="ai.error"
    @drill="$emit('drill', { range: 'week' })"
    @retry="$emit('retry')"
  >
    <div class="card-extras">
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
