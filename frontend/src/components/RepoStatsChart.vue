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
      <div class="chart-header">
        <div class="chart-title">贡献者排名</div>
        <!-- 维度切换器：提交数 / 行数，复用同一图表容器（行数视图按总变更行数排序，
             tooltip 展示 +新增/−删除 细分）；样式对齐 StatsView 档位切换器 -->
        <el-radio-group v-model="contributorDim" size="small">
          <el-radio-button value="commits">提交数</el-radio-button>
          <el-radio-button value="lines">行数</el-radio-button>
        </el-radio-group>
      </div>
      <v-chart
        class="chart-canvas chart-contributor"
        :option="contributorOption"
        :loading="!stats"
        autoresize
      />
    </div>

    <div class="chart-section">
      <div class="chart-header">
        <div class="chart-title">行数分布</div>
        <!-- 维度切换器：目录一级上卷（模块分布）/ Top 文件（热点明细），同源
             numstat 通道（rename 归一 new 路径、二进制排除）；样式对齐贡献者卡 -->
        <el-radio-group v-model="pathDim" size="small">
          <el-radio-button value="dirs">目录</el-radio-button>
          <el-radio-button value="files">文件</el-radio-button>
        </el-radio-group>
      </div>
      <v-chart
        class="chart-canvas chart-path-line"
        :option="pathLineOption"
        :loading="!stats"
        autoresize
      />
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
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
  buildContributorOption,
  buildContributorLineOption,
  buildPathLineOption
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

// contributorDim 贡献者卡片维度：commits=提交数（默认）/ lines=行数（总变更排序）
const contributorDim = ref('commits')

// contributorOption 贡献者排名 option：横向柱状，提交数维度按 count 降序（后端保证），
// 行数维度按总变更行数（新增+删除）降序（buildContributorLineOption 内重排）
const contributorOption = computed(() => {
  if (!props.stats) return {}
  if (contributorDim.value === 'lines') {
    return buildContributorLineOption(props.stats.contributors)
  }
  return buildContributorOption(props.stats.contributors)
})

// pathDim 行数分布维度：dirs=目录一级上卷（默认，模块分布）/ files=Top 文件（热点明细）
const pathDim = ref('dirs')

// pathLineOption 行数分布 option：目录/文件维度取后端对应聚合字段
// （dirLineStats 全量降序 / topFileLineStats 后端已截 Top 15），后端保证排序
const pathLineOption = computed(() => {
  if (!props.stats) return {}
  const entries = pathDim.value === 'files'
    ? props.stats.topFileLineStats
    : props.stats.dirLineStats
  return buildPathLineOption(entries)
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
/* 贡献者卡片标题行：标题居左 + 维度切换器居右（对齐 StatsView header-actions 布局） */
.chart-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-sm, 8px);
}
.chart-header .chart-title {
  margin-bottom: 0;
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
.chart-path-line {
  height: 280px;
}
</style>
