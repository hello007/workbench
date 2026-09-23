// frontend/src/utils/repoStatsOptions.js
//
// ECharts option 构造纯函数：将后端 RepoStats 结构化数据转为 ECharts option 对象。
// 抽离为纯函数便于单元测试（不依赖 Vue/echarts 运行时），组件层仅负责挂载 v-chart。
// 色阶对齐 GitHub 贡献草地 5 档，阈值可配置化便于后续按仓库活跃度动态分档。

// GitHub 贡献草地 5 档色阶（0 无提交 → 4 高活跃）
const HEATMAP_COLORS = ['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39']

// HEATMAP_PIECES 热力图 visualMap 分段阈值，与 HEATMAP_COLORS 一一对应。
export const HEATMAP_PIECES = [
  { min: 0, max: 0, color: HEATMAP_COLORS[0] },
  { min: 1, max: 3, color: HEATMAP_COLORS[1] },
  { min: 4, max: 6, color: HEATMAP_COLORS[2] },
  { min: 7, max: 9, color: HEATMAP_COLORS[3] },
  { min: 10, color: HEATMAP_COLORS[4] }
]

// buildTrendOption 构造提交趋势图 option（折线/柱状）。
// trend: [{date, count}]，granularity: day/week/month 供 X 轴格式化提示。
export function buildTrendOption(trend, granularity) {
  const data = trend || []
  const xData = data.map(b => b.date)
  const yData = data.map(b => b.count)
  return {
    tooltip: {
      trigger: 'axis',
      formatter: (params) => {
        const p = params[0]
        return `${p.axisValue}<br/>提交数: <b>${p.value}</b>`
      }
    },
    grid: { left: 40, right: 20, top: 20, bottom: 40 },
    xAxis: {
      type: 'category',
      data: xData,
      axisLabel: { rotate: xData.length > 12 ? 45 : 0 }
    },
    yAxis: { type: 'value', minInterval: 1 },
    series: [{
      name: '提交数',
      type: data.length > 30 ? 'bar' : 'line',
      data: yData,
      smooth: true,
      areaStyle: { opacity: 0.15 },
      itemStyle: { color: '#409eff' }
    }]
  }
}

// buildHeatmapOption 构造活跃度热力图 option（calendar + heatmap + visualMap）。
// heatmap: [{date, count}] 最近一年按日序列。rangeStart/rangeEnd 控制日历范围。
export function buildHeatmapOption(heatmap) {
  const data = heatmap || []
  const seriesData = data.map(d => [d.date, d.count])
  // 范围取数据首末日；空数据回退最近一年
  const rangeEnd = data.length > 0 ? data[data.length - 1].date : new Date().toISOString().slice(0, 10)
  const rangeStart = data.length > 0 ? data[0].date : rangeEnd
  return {
    tooltip: {
      formatter: (params) => `${params.value[0]}<br/>提交数: <b>${params.value[1]}</b>`
    },
    visualMap: {
      type: 'piecewise',
      pieces: HEATMAP_PIECES,
      orient: 'horizontal',
      left: 'center',
      top: 0,
      itemWidth: 12,
      itemHeight: 12
    },
    calendar: {
      top: 60,
      range: [rangeStart, rangeEnd],
      cellSize: ['auto', 13],
      left: 30,
      right: 30,
      dayLabel: { firstDay: 1 },
      monthLabel: { nameMap: 'zh-CN' },
      yearLabel: { show: false },
      splitLine: { show: false }
    },
    series: [{
      type: 'heatmap',
      coordinateSystem: 'calendar',
      data: seriesData
    }]
  }
}

// buildContributorOption 构造贡献者排名图 option（横向柱状，按 count 降序已由后端保证）。
// contributors: [{author, email, count}]，limit 控制展示数量避免过长。
// yAxis inverse:true 使 data[0]（最高 count）渲染在顶部，无需 reverse 数组——
// 避免反转数组与 tooltip 索引逆向映射的三处耦合（改其一忘其二致柱条与浮窗错位）。
export function buildContributorOption(contributors, limit = 15) {
  const data = (contributors || []).slice(0, limit)
  const authors = data.map(c => c.author)
  const counts = data.map(c => c.count)
  return {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params) => {
        const p = params[0]
        const c = data[p.dataIndex]
        return `${c.author}<br/>${c.email}<br/>提交数: <b>${c.count}</b>`
      }
    },
    grid: { left: 100, right: 30, top: 20, bottom: 30 },
    xAxis: { type: 'value', minInterval: 1 },
    yAxis: {
      type: 'category',
      data: authors,
      inverse: true
    },
    series: [{
      name: '提交数',
      type: 'bar',
      data: counts,
      itemStyle: { color: '#40c463' },
      label: { show: true, position: 'right', formatter: '{c}' }
    }]
  }
}

// buildContributorLineOption 构造贡献者行数排名图 option（横向柱状）。
// contributors: [{author, email, count, insertions, deletions}]。后端 Contributors
// 按提交数降序，行数维度在本函数内重排：按总变更行数（新增+删除）降序、同值按作者名
// 升序，与后端 AggregateLineStats 排序规则一致。排序在副本上做，不修改入参数组
// （props 传入的 stats.contributors 不允许原地变更）。
// 柱值 = 总变更行数；tooltip 展示 +新增/−删除 细分（字段分开存储，细分展示不改后端）。
// limit 控制展示数量，复用 buildContributorOption 的 inverse:true 渲染约定。
export function buildContributorLineOption(contributors, limit = 15) {
  const data = [...(contributors || [])]
    .sort((a, b) =>
      ((b.insertions || 0) + (b.deletions || 0)) - ((a.insertions || 0) + (a.deletions || 0)) ||
      String(a.author).localeCompare(String(b.author))
    )
    .slice(0, limit)
  const authors = data.map(c => c.author)
  const totals = data.map(c => (c.insertions || 0) + (c.deletions || 0))
  return {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params) => {
        const c = data[params[0].dataIndex]
        const ins = c.insertions || 0
        const del = c.deletions || 0
        return `${c.author}<br/>${c.email}<br/>总变更: <b>${ins + del}</b><br/>` +
          `<span style="color:#40c463">+${ins} 新增</span> / <span style="color:#d73a49">−${del} 删除</span>`
      }
    },
    grid: { left: 100, right: 30, top: 20, bottom: 30 },
    xAxis: { type: 'value', minInterval: 1 },
    yAxis: {
      type: 'category',
      data: authors,
      inverse: true
    },
    series: [{
      name: '总变更行数',
      type: 'bar',
      data: totals,
      itemStyle: { color: '#8250df' },
      label: { show: true, position: 'right', formatter: '{c}' }
    }]
  }
}

// buildPathLineOption 构造路径维度行数分布图 option（横向柱状）。
// entries: [{path, insertions, deletions}]——目录维度为一级目录上卷（后端全量降序），
// 文件维度为 Top 15（后端 TopFileLineStatsLimit 截断，前端不二次截断）。后端已按
// 总变更行数降序、同值按路径升序排序，本函数不再重排（与 buildContributorLineOption
// 的前端重排不同：路径排序键在后端已含中文名 locale 语义，避免双端规则漂移）。
// 柱值 = 总变更行数；tooltip 展示 +新增/−删除 细分。label 宽路径防溢出：
// formatter 截断超长路径中段（保留首尾段），完整路径由 tooltip 承载。
export function buildPathLineOption(entries) {
  const data = entries || []
  const paths = data.map(e => e.path)
  const totals = data.map(e => (e.insertions || 0) + (e.deletions || 0))
  return {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params) => {
        const e = data[params[0].dataIndex]
        const ins = e.insertions || 0
        const del = e.deletions || 0
        return `${escapeHtml(e.path)}<br/>总变更: <b>${ins + del}</b><br/>` +
          `<span style="color:#40c463">+${ins} 新增</span> / <span style="color:#d73a49">−${del} 删除</span>`
      }
    },
    grid: { left: 160, right: 40, top: 20, bottom: 30 },
    xAxis: { type: 'value', minInterval: 1 },
    yAxis: {
      type: 'category',
      data: paths,
      inverse: true,
      axisLabel: {
        // 路径标签超宽截断（中段省略），完整路径见 tooltip
        formatter: (value) => value.length > 24 ? value.slice(0, 12) + '…' + value.slice(-11) : value
      }
    },
    series: [{
      name: '总变更行数',
      type: 'bar',
      data: totals,
      itemStyle: { color: '#0969da' },
      label: { show: true, position: 'right', formatter: '{c}' }
    }]
  }
}

// escapeHtml tooltip 内路径转义（文件名可含 <> 等字符，防注入破坏 tooltip 渲染）。
function escapeHtml(s) {
  return String(s)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
}
