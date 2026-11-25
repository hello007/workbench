<template>
  <OverviewCardShell
    title="待办积压"
    :value="pendingCount"
    unit="条"
    :hint="hint"
    :error="todos.error"
    :highlighted="oldestDays >= 7"
    @drill="$emit('drill', { todoId: todos.oldestId || '' })"
    @retry="$emit('retry')"
  >
    <div v-if="todos.oldestText" class="card-extra" :title="todos.oldestText">
      最老：{{ todos.oldestText }}
    </div>
  </OverviewCardShell>
</template>

<script setup>
import { computed } from 'vue'
import OverviewCardShell from './OverviewCardShell.vue'

/**
 * M3 待办积压卡片：未完成条数 + 最老条目滞留天数。
 * 最老条目滞留 ≥7 天高亮；下钻载荷带 oldestId，待办面板据此定位。
 */
const props = defineProps({
  todos: { type: Object, required: true }
})

defineEmits(['drill', 'retry'])

const pendingCount = computed(() => props.todos.pendingCount || 0)
const oldestDays = computed(() => props.todos.oldestDays || 0)

const hint = computed(() => {
  if (pendingCount.value === 0) return '暂无未完成待办'
  if (oldestDays.value > 0) return `最老已滞留 ${oldestDays.value} 天`
  return '均为今天创建'
})
</script>

<style scoped>
.card-extra {
  margin-top: auto;
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
