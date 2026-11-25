<template>
  <OverviewCardShell
    title="待收尾仓库"
    :value="pendingCount"
    unit="个"
    :hint="hint"
    :error="repos.error"
    :highlighted="pendingCount > 0"
    @drill="$emit('drill')"
    @retry="$emit('retry')"
  >
    <div v-if="repos.missingCount > 0" class="card-extra">
      失效仓库 {{ repos.missingCount }} 个（不计入）
    </div>
  </OverviewCardShell>
</template>

<script setup>
import { computed } from 'vue'
import OverviewCardShell from './OverviewCardShell.vue'

/**
 * M1 待收尾仓库卡片：Dirty 或 StashCount>0 的 pin 仓库数。
 * 失效仓库（目录已删/非 Git 仓库）单列展示，不计入主数值。
 */
const props = defineProps({
  repos: { type: Object, required: true }
})

defineEmits(['drill', 'retry'])

const pendingCount = computed(() => props.repos.pendingCount || 0)
// 无 pin 仓库时按 R6 空态显示「未关注仓库」，点击卡片可下钻到状态看板添加。
const hint = computed(() =>
  (props.repos.pinnedTotal || 0) === 0 ? '未关注仓库，点击前往状态看板' : `共关注 ${props.repos.pinnedTotal} 个仓库`
)
</script>

<style scoped>
.card-extra {
  margin-top: auto;
  font-size: 12px;
  color: var(--text-tertiary);
}
</style>
