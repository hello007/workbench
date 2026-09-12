<!-- frontend/src/components/RepoStatsChart.vue -->
<template>
  <div class="repo-stats-chart">
    <div class="chart-section">
      <div class="chart-title">提交趋势</div>
      <v-chart
        class="chart-canvas chart-trend"
        :option="trendOption"
        :loading="!stats"
        autoresize
      />
    </div>

    <div class="chart-section">
      <div class="chart-title">活跃度热力图</div>
      <v-chart
        class="chart-canvas chart-heatmap"
        :option="heatmapOption"
        :loading="!stats"
        autoresize
      />
    </div>

    <div class="chart-section">
      <div class="chart-title">贡献者排名</div>
      <v-chart
        class="chart-canvas chart-contributor"
        :option="contributorOption"
        :loading="!stats"
        autoresize
      />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart, HeatmapChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  VisualMapComponent,
  CalendarComponent
} from 'echarts/components'
import VChart from 'vue-echarts'
import {
  buildTrendOption,
  buildHeatmapOption,
  buildContributorOption
} from '../utils/repoStatsOptions'

// 按需注册 ECharts 模块（tree-shake，避免全量引入）
use([
  CanvasRenderer,
  LineChart,
  BarChart,
  HeatmapChart,
  GridComponent,
  TooltipComponent,
  VisualMapComponent,
  CalendarComponent
])

const props = defineProps({
  // stats 后端 RepoStats 聚合结果，null 表示未加载
  stats: { type: Object, default: null }
})

// trendOption 趋势图 option：粒度随档位自动适配（day/week/month）
const trendOption = computed(() => {
  if (!props.stats) return {}
  return buildTrendOption(props.stats.trend, props.stats.granularity)
})

// heatmapOption 热力图 option：calendar + visualMap 5 档色阶
const heatmapOption = computed(() => {
  if (!props.stats) return {}
  return buildHeatmapOption(props.stats.heatmap)
})

// contributorOption 贡献者排名 option：横向柱状，按 count 降序
const contributorOption = computed(() => {
  if (!props.stats) return {}
  return buildContributorOption(props.stats.contributors)
})
</script>

<style scoped>
.repo-stats-chart {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md, 16px);
  height: 100%;
  overflow-y: auto;
  padding: var(--spacing-md, 16px);
}
.chart-section {
  background: var(--bg-secondary, #fff);
  border: 1px solid var(--border-color, #ebeef5);
  border-radius: var(--radius-md, 8px);
  padding: var(--spacing-sm, 12px);
}
.chart-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary, #303133);
  margin-bottom: var(--spacing-sm, 12px);
}
.chart-canvas {
  width: 100%;
}
.chart-trend {
  height: 240px;
}
.chart-heatmap {
  height: 200px;
}
.chart-contributor {
  height: 320px;
}
</style>
