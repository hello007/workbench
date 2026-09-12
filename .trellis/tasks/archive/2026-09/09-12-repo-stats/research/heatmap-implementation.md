# 研究：GitHub 贡献草地热力图实现方式

> 日期：2026-09-12 | 范围：Vue3 + Element Plus + Vite 8（Wails 桌面端）

## 内部现状（已核实）

| 项 | 现状 | 影响 |
|---|---|---|
| 图表库依赖 | package.json 无 echarts/chart.js/d3（d3 仅 mermaid 传递依赖，不可复用） | 需新引入 |
| 现有统计组件 | frontend/src 无 heatmap/calendar/stats 组件 | 全新组件，参考 CommitHistory.vue 数据获取+loading 模式 |
| Element Plus | 有 el-calendar（月视图），非贡献热力图 | 不适用 |
| 后端数据契约 | 约定返回 `[{date,count}, ...]` | 三方案都能适配 |
| 运行形态 | Wails 桌面，打包进二进制无网络下载 | bundle 非硬约束 |

## 三方案对比矩阵

| 维度 | A. ECharts calendar heatmap | B. 纯 CSS grid | C1. vue3-calendar-heatmap | C2. cal-heatmap v4 |
|---|---|---|---|---|
| 日历布局（周对齐/跨年/月标签） | 内置 calendar 坐标系 | 手写 grid+前置空格 | 内置 | 内置 subdomain |
| 空日补零 | visualMap 0 档自动 | 手动补空 div | 自动 | 自动 |
| 颜色分级 | visualMap.pieces 配置化 | JS 函数+CSS class 最灵活 | props.range 较固定 | options.scale 较灵活 |
| tooltip | 内置富文本 | 手写浮动/el-tooltip 虚拟触发 | 内置 | 内置 |
| 性能（1年/多年） | canvas 优 | DOM div 优 | SVG 良 | SVG 良 |
| bundle（gzip 近似） | 150–250KB（未实测） | 0 | 30–50KB（未实测） | ~250KB（未实测） |
| 是否同时覆盖趋势/排名图 | **是**（折线/柱状同库） | 否 | 否 | 否 |
| 样式可控度 | 中（theme/option） | 全 | 低 | 中 |
| 中文文档 | 完善 | 无 | 弱 | 弱 |

## 推荐：方案 A（ECharts calendar heatmap）

**决定性理由：**

1. **一个依赖覆盖 MVP 三种图**（热力图+趋势折线/柱状+贡献者柱状），统一主题与生命周期；选 B/C 仍需再引趋势图库，净复杂度更高
2. `visualMap` piecewise 天然适配 GitHub 5 档色阶，`calendar` 内置周对齐/月标签/跨年，边界由库兜底
3. Wails 桌面 bundle 打包进二进制，150–250KB gzip 可接受，方案 B 零字节优势价值低
4. Apache 中文文档完善，匹配团队中文规范

**GitHub 5 档色阶参考：**
`#ebedf0`(0) / `#9be9a8`(1-3) / `#40c463`(4-6) / `#30a14e`(7-9) / `#216e39`(10+)

## ECharts 实施要点

- **按需引入**：`echarts/core` + `HeatmapChart`/`LineChart`/`BarChart` + `CalendarComponent`/`TooltipComponent`/`VisualMapComponent`/`GridComponent` + `CanvasRenderer`；勿全量 `import * as echarts`
- **Vue3 集成**：`vue-echarts` 的 `<v-chart :option>` 声明式绑定；或 `onMounted` 手动 init + resize 监听 + `onBeforeUnmount` dispose（Wails 窗口可 resize，须接 ResizeObserver）
- **数据转换**：后端 `[{date,count}]` → `list.map(x => [x.date, x.count])` 喂 heatmap series.data；空日前端 dayjs 补全年日期序列 `{count:0}` 或交 visualMap 0 档
- **跨年**：单 `calendar.range` 跨年；多年用 `calendar` 数组多实例 + 多 series
- **dayjs**：当前仅 mermaid 传递依赖，需显式 `npm i dayjs` 加入直接依赖

## 未验证项（implement 阶段核实）

- 各方案 bundle gzip 数值未在本仓库 `vite build` 实测，落地前跑 build 核 dist/assets 体积
- vue3-calendar-heatmap（C1）npm 最近发布与 GitHub 最近 commit 未核，若倾向 C1 须先验维护活跃度
- cal-heatmap v4 与 Vue3 wrapper `cal-heatmap-vue` 兼容性未核
- ECharts 按需引入在 Vite 8 + rolldown 下 tree-shake 实际效果未验证
