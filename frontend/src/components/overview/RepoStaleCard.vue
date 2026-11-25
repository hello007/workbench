<template>
  <OverviewCardShell
    title="久未推进仓库"
    :value="staleCount"
    unit="个"
    :hint="hint"
    :error="repos.error"
    :highlighted="staleCount > 0"
    @drill="$emit('drill')"
    @retry="$emit('retry')"
  >
    <div v-if="repos.noCommitCount > 0" class="card-extra">
      无提交 {{ repos.noCommitCount }} 个（不计入）
    </div>
  </OverviewCardShell>
</template>

<script setup>
import { computed } from 'vue'
import OverviewCardShell from './OverviewCardShell.vue'

/**
 * M2 久未推进仓库卡片：LastCommitUnix 距今天数超过阈值的 pin 仓库数。
 * 无提交（unborn/查询失败）单列展示，不计入主数值。
 * 阈值来自后端快照（设置项 7/14/30），卡片只展示不判定。
 */
const props = defineProps({
  repos: { type: Object, required: true }
})

defineEmits(['drill', 'retry'])

const staleCount = computed(() => props.repos.staleCount || 0)
// 无 pin 仓库时按 R6 空态显示「未关注仓库」；否则展示当前生效阈值。
const hint = computed(() => {
  if ((props.repos.pinnedTotal || 0) === 0) return '未关注仓库，点击前往状态看板'
  return `超过 ${props.repos.staleDays || 7} 天未提交`
})
</script>

<style scoped>
.card-extra {
  margin-top: auto;
  font-size: 12px;
  color: var(--text-tertiary);
}
</style>
