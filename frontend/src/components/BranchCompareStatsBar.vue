<!-- frontend/src/components/BranchCompareStatsBar.vue -->
<!-- 分支对比统计条（纯展示组件，R4）：A 独有 / 匹配（含疑似拣选数）/ B 独有 /
     merge-base 之下共同历史计数；截断与无共同祖先的提示横幅。
     stats/truncated/unrelated 均由父级（BranchCompareView）从对齐结果派生。 -->
<template>
  <div class="bc-stats-bar">
    <div v-if="stats" class="stats-line">
      <span class="stats-item accent-a">A 独有 <b>{{ stats.aOnly }}</b></span>
      <span class="stats-sep">·</span>
      <span class="stats-item accent-matched">匹配 <b>{{ stats.matched }}</b><template v-if="stats.patchMatched > 0">（疑似拣选 <b>{{ stats.patchMatched }}</b>）</template></span>
      <span class="stats-sep">·</span>
      <span class="stats-item accent-b">B 独有 <b>{{ stats.bOnly }}</b></span>
      <span class="stats-sep">·</span>
      <span class="stats-item">merge-base 之下共同历史 <b>{{ stats.commonBelow }}</b></span>
    </div>
    <!-- 无共同祖先（unrelated histories）：merge-base 不存在 -->
    <el-alert
      v-if="unrelated"
      type="warning"
      :closable="false"
      show-icon
      title="两个分支无共同祖先（unrelated histories），仅按提交哈希对齐"
    />
    <!-- 任一侧触及后端 2000 条上限：明示截断，数字不完整 -->
    <el-alert
      v-if="truncated"
      type="warning"
      :closable="false"
      show-icon
      title="仅显示前 2000 条"
    />
  </div>
</template>

<script setup>
defineOptions({ name: 'BranchCompareStatsBar' })

defineProps({
  // 对齐统计 {aOnly, bOnly, matched, patchMatched, commonBelow}；null 不渲染数字行
  stats: { type: Object, default: null },
  // 对齐结果被后端截断（任一侧触及 2000 条上限）
  truncated: { type: Boolean, default: false },
  // 无共同祖先（merge-base 缺失）
  unrelated: { type: Boolean, default: false }
})
</script>

<style scoped>
.bc-stats-bar {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm, 8px);
}

.stats-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--spacing-sm, 8px);
  font-size: 13px;
  color: var(--text-secondary);
}

.stats-item b {
  font-weight: 700;
  color: var(--text-primary);
  font-variant-numeric: tabular-nums;
}

.stats-sep {
  color: var(--text-tertiary);
}

/* 三类计数的语义色点：A 独有 primary（蓝）、匹配 success（绿）、B 独有 warning（橙） */
.accent-a::before,
.accent-matched::before,
.accent-b::before {
  content: '';
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  vertical-align: middle;
}

.accent-a::before {
  background: var(--primary-color);
}

.accent-matched::before {
  background: var(--success-color);
}

.accent-b::before {
  background: var(--warning-color);
}
</style>
