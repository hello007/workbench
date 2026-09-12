# 研究报告：仓库统计图表库选型

- **Query**: Vue3 + Element Plus + Vite 8 桌面应用（Wails 桌面端 Git 工具 WorkBench）中实现仓库提交统计图表的图表库选型；需覆盖提交趋势折线/柱状图、贡献者排名柱状图/列表、活跃度热力图（类 GitHub 草地）三种可视化
- **Scope**: mixed（内部现状核查 + 外部库对比）
- **Date**: 2026-09-12

## 一、内部现状（基线）

### 前端依赖现状

| 文件路径 | 关键事实 |
|---|---|
| `frontend/package.json` | 无任何图表库依赖（echarts / chart.js / d3 / antv 均未直接声明）；已有 `mermaid ^11.16.0`、`xlsx ^0.18.5`、`highlight.js ^11.11.1`、`@codemirror/*` 等重型依赖 |
| `frontend/package.json` | 技术栈锁定：`vue ^3.5.33`、`element-plus ^2.13.7`、`vite ^8.0.10`、`vitest ^4.1.5`、`@vue/test-utils ^2.4.9`、`jsdom ^29.1.0` |
| `frontend/src/components/CommitHistory.vue` | 数据获取范式：`<script setup>` Composition API + Wails 绑定 `GetCommitHistory(repoPath, pageSize, offset, filter)` + `v-loading` + `ElMessage.error` 错误兜底；统计页应复用此模式 |
| `frontend/src/components/FilePreviewRenderer.vue` | 重型可视化库集成先例：`import mermaid from 'mermaid'`（静态导入），`mermaid.initialize()` 在模块加载（line 384）与主题切换（line 496）时各调用一次，主题切换时还原节点原文重渲染（line 522-541） |
| `frontend/src/style.css` | 设计令牌系统：`:root` 与 `html.dark` 双套 CSS 变量；主色 `--primary-color: #409eff`（Element Plus 默认蓝），含 `--primary-light/dark`、`--bg-primary/secondary/tertiary`、`--text-primary/secondary/tertiary`、`--border-color`、`--success/warning/danger/info-color` |
| `frontend/src/style.css:82` | 暗色触发器：`<html>` 元素加 `.dark` class，由 `App.vue` watch `resolvedTheme` 切换；选择器 `html.dark` 与 Element Plus `dark/css-vars.css` 一致 |
| `frontend/src/router/index.js` | 单路由 `Home`（懒加载 `() => import('../views/Home.vue')`）；统计页需新增路由或作为 Home 内子面板 |
| `frontend/wailsjs/go/main/App.js`、`App.d.ts` | 跨层绑定；PRD 已警示：改 `app.go` App 方法签名须同步 `App.js / App.d.ts / models.ts` 三处（见 `docs/spec/cross-layer-contracts.md`） |

### 关键内部约束（影响选型）

1. **前端 src 不使用 TypeScript**：`.vue` / `.js` 均为纯 JS，仅 `wailsjs/**/*.d.ts` 为类型声明。故"TypeScript 支持"维度对本项目**非承重因素**，仅作加分项。
2. **暗色主题必须支持**：图表须在 `html.dark` 切换时重渲染。mermaid 已建立"initialize + 主题切换重 init + 重渲染"范式，图表库须能复用此模式。
3. **设计令牌一致性**：图表配色须读取 `--primary-color` 等 CSS 变量，与 Element Plus 风格统一。
4. **包体积非极端敏感**：已存在 mermaid（约 800KB-1MB）、xlsx（约 600KB）等大体积依赖，Wails 桌面端将前端资产嵌入二进制；新增 200-300KB 可接受，但 Vite 8 开发冷启动与构建时长仍需控制。
5. **测试基础设施**：Vitest + `@vue/test-utils` + jsdom；图表组件须可 mock 库实例做单测（前端覆盖率门禁 ≥70%）。

## 二、三方案对比

### 方案 A：ECharts（vue-echarts 封装）—— 全覆盖单库

**依赖组合**：`echarts` + `vue-echarts`（Vue 3 官方推荐封装，提供 `<v-chart>` 组件）

**三维可视化覆盖**：
- 提交趋势：`line` / `bar` series + `grid` + `tooltip` + `dataZoom`（日/周/月切换仅需重组数据，无需换库）
- 贡献者排名：`bar` series（横向柱状）+ `tooltip`；排名列表可用 Element Plus `el-table` 原生实现，不必走图表
- 活跃度热力图：**原生 `calendar` 组件 + `heatmap` series + `visualMap`**——开箱即得 GitHub 草地式年度日历（周列 × 7 行 × 月份标签 × 颜色深浅映射），无需手算日历布局

**体积**：全量约 1MB（min）/ 330KB（gzip）；按需注册可压至约 200-280KB（min）/ 70-90KB（gzip）。Tree-shaking 路径：从 `echarts/core` 导入 `use` 注册器，仅注册 `LineChart`、`BarChart`、`HeatmapChart`、`CalendarComponent`、`TooltipComponent`、`GridComponent`、`VisualMapComponent`。

**Vue3 适配**：vue-echarts 7.x 基于 Composition API，`<v-chart :option="...">` 声明式，自动 resize，与 `<script setup>` 契合。

**Element Plus 风格一致性**：支持自定义 theme 对象，可在 `initialize` 时读取 CSS 变量注入配色；ECharts 内置 `dark` 主题。**热力图原生支持度最高**。

**暗色主题**：仿 mermaid 范式——主题切换时重新 `setOption` 或重 init 主题；可在 theme 对象里引用 Element Plus 变量值。

**维护活跃度**：Apache 基金会顶级项目，约 60k+ star，版本迭代规律（当前 5.x，已用 TypeScript 重写，类型完备）。

**TypeScript 支持**：原生 TS 编写，类型定义完备（本项目非承重）。

**风险**：体积为三方案最大；需手写 tree-shaking 注册代码（约 10 行）。

### 方案 B：Chart.js（vue-chartjs）+ chartjs-chart-matrix —— 轻量但热力图需手算

**依赖组合**：`chart.js` + `vue-chartjs`（Vue 3 支持，5.x）+ `chartjs-chart-matrix`（社区矩阵插件，提供 cell 渲染）

**三维可视化覆盖**：
- 提交趋势：`line` / `bar` 类型，原生支持
- 贡献者排名：`bar`（横向）；列表同样可走 `el-table`
- 活跃度热力图：**无原生 calendar 组件**。`chartjs-chart-matrix` 仅提供矩阵单元格渲染，**日历布局（周列对齐、月份分隔、星期标签）须自行计算 x/y 坐标**——相当于手写 GitHub 草地排版逻辑

**体积**：core 约 200KB（min）/ 65KB（gzip），tree-shaking 后更小；加 matrix 插件约 +30KB；vue-chartjs 约 +5KB。合计约 235KB（min）/ 75KB（gzip），略小于方案 A 按需后体积。

**Vue3 适配**：vue-chartjs 5.x 支持 Composition API，但 API 偏命令式（需自己包 `<canvas>` + `new Chart()`），声明式程度低于 vue-echarts。

**Element Plus 风格一致性**：无主题系统，颜色逐 dataset 手填；暗色需逐图换色，无法像 ECharts 那样整体切主题。**风格统一成本最高**。

**暗色主题**：无内置机制，须监听 `html.dark` 手动重设所有 dataset 颜色后 `chart.update()`。

**维护活跃度**：活跃，约 65k+ star；`chartjs-chart-matrix` 为社区插件，迭代节奏慢于核心。

**TypeScript 支持**：Chart.js v3+ 内置类型；matrix 插件类型不完整。

**风险**：热力图日历布局手算成本高、易错；暗色多图换色代码膨胀；matrix 插件为社区维护。

### 方案 C：混合方案 —— ECharts（趋势 + 排名）+ 纯 CSS Grid（热力图）

**依赖组合**：`echarts` + `vue-echarts`（仅用于折线/柱状）+ 自写 CSS Grid 热力图组件（无依赖）

**三维可视化覆盖**：
- 提交趋势、贡献者排名：同方案 A
- 活跃度热力图：纯 CSS Grid（N 周 × 7 行），颜色用 `style="background: var(--heatmap-level-3)"` 或内联色阶；GitHub 草地本质即 CSS Grid，可 1:1 还原

**体积**：ECharts 按需后约 150-200KB（min）/ 50-65KB（gzip）（去掉 `HeatmapChart` + `CalendarComponent`）；CSS Grid 0 KB。

**Vue3 适配**：CSS Grid 即 Vue 模板 `<div v-for>`，最贴合 Composition API；ECharts 部分同方案 A。

**Element Plus 风格一致性**：CSS Grid 直接读 `--primary-color` 等变量，**风格一致性最佳**，暗色随变量自动切换零代码。ECharts 部分同方案 A。

**暗色主题**：CSS Grid 部分零成本（变量驱动）；ECharts 部分同方案 A。

**维护活跃度**：自写代码需自维护；ECharts 部分同方案 A。

**风险**：**双渲染范式并存**（SVG/Canvas 图表 + CSS Grid）增加心智负担与测试成本；热力图交互（tooltip 悬停某日显示提交数）须自实现；省下的体积（约 30-50KB）相对方案 A 收益边际。

## 三、方案对比矩阵

| 维度 | 方案 A（ECharts 全覆盖） | 方案 B（Chart.js + matrix） | 方案 C（ECharts + 纯 CSS 热力图） |
|---|---|---|---|
| 热力图原生支持 | **原生 calendar + heatmap** | 无 calendar，须手算布局 | CSS Grid 自写 |
| 趋势/排名覆盖 | 原生 line/bar | 原生 line/bar | 原生 line/bar |
| 体积（min / gzip，约） | 200-280KB / 70-90KB | 235KB / 75KB | 150-200KB / 50-65KB |
| Vue3 Composition API 适配 | 高（`<v-chart>` 声明式） | 中（偏命令式） | 高（模板 + 声明式） |
| Element Plus 风格一致性 | 高（theme 对象注入变量） | 低（逐 dataset 手填色） | **最高**（CSS 变量直读） |
| 暗色主题成本 | 中（重 init 主题，仿 mermaid） | 高（逐图手换色） | 低（变量驱动） |
| 维护活跃度 | 高（Apache 顶级项目） | 中（核心高，matrix 社区） | 自维护 |
| TypeScript 支持 | 原生 TS | 类型基本完整 | N/A（纯 JS 项目） |
| 测试可 mock 性 | 高（mock `setOption`） | 中 | 高（纯 DOM） |
| 实现复杂度 | 低（单库单范式） | 高（日历手算 + 多图换色） | 中（双范式并存） |

> 体积数值为基于训练知识的估算（minified / gzip），实际以 `npm install` 后 `vite build` 产物 `dist/assets/*.js` 大小为准，建议落选后用 bundlephobia 或本地 dry-run 校验。

## 四、推荐方案

**推荐方案 A：ECharts（vue-echarts 封装），单库覆盖三种可视化。**

### 推荐理由

1. **热力图是决定性差异点**：PRD 明确要求"类 GitHub 草地，时间轴 + 颜色深浅表提交密度"。ECharts 是三方案中**唯一原生提供 calendar 组件**的库，开箱即得年度日历排版、月份标签、星期行、visualMap 色阶映射。方案 B 需手写日历布局算法，方案 C 需自实现 tooltip 交互，均背离"复用现成能力"原则。
2. **单库单范式降低实现与测试成本**：三种可视化共用一套 `setOption` + `<v-chart>` 模式，组件抽象与 Vitest mock 路径统一；方案 C 双范式（Canvas + CSS Grid）会分裂组件设计与测试策略。
3. **暗色主题有现成范式可循**：项目已用 mermaid 建立"initialize + 主题切换重 init + 重渲染"模式（`FilePreviewRenderer.vue:384/496/522-541`），ECharts 的 theme + `setOption` 机制可无缝复用此范式，暗色切换成本可控。
4. **体积可接受**：按需注册后约 70-90KB（gzip），项目已有 mermaid/xlsx 等更大依赖，相对增量不构成瓶颈；省下的方案 C 仅 30-50KB，不值得引入双范式复杂度。
5. **Element Plus 风格一致性可达**：通过 ECharts theme 对象在 init 时读取 `--primary-color` / `--text-primary` / `--border-color` 等变量注入，配合 `visualMap` 色阶用 `--primary-light` → `--primary-dark` 渐变，可做到与 Element Plus 视觉统一。

### 落地建议（供 implement 阶段参考，非本研究的实施指令）

- 依赖安装：`echarts` + `vue-echarts`（具体版本以 `npm install` 时最新稳定 5.x / 7.x 为准，锁定到 `package.json`）
- Tree-shaking：新建 `frontend/src/utils/echarts.js` 集中 `use` 注册所需图表/组件，组件统一从此模块导入，避免全量引入
- 主题适配：新建 `frontend/src/composables/useEchartsTheme.js`，监听 `html.dark` 切换，仿 mermaid 范式重 init theme + 重 `setOption`
- 配色：ECharts theme 读取 `getComputedStyle(document.documentElement).getPropertyValue('--primary-color')` 等变量，热力图 visualMap 色阶映射到 `--primary-light`/`--primary-dark`
- 跨层契约：后端 service 层聚合统计返结构化数据（趋势序列、排名列表、热力图日期→提交数映射），新增 Wails 绑定方法须同步 `frontend/wailsjs/go/main/App.js` / `App.d.ts` / `models.ts` 三处（见 `docs/spec/cross-layer-contracts.md`）
- 测试：mock `vue-echarts` 的 `<v-chart>` 或 `echarts.use`，验证组件传入了正确的 `option` prop，满足前端 ≥70% 覆盖率门禁

## 五、外部参考

- [Apache ECharts 官方文档](https://echarts.apache.org/handbook/) — 配置项、tree-shaking 用法、calendar + heatmap 示例
- [vue-echarts GitHub](https://github.com/ecomfe/vue-echarts) — Vue 3 封装，`<v-chart>` 组件 API
- [ECharts calendar heatmap 示例](https://echarts.apache.org/examples/zh/editor.html?c=calendar-heatmap) — GitHub 草地式日历热力图官方示例
- [Chart.js 官方文档](https://www.chartjs.org/docs/latest/) — line/bar 类型
- [chartjs-chart-matrix GitHub](https://github.com/kurkle/chartjs-chart-matrix) — 矩阵/热力图社区插件
- [vue-chartjs GitHub](https://github.com/apertureless/vue-chartjs) — Vue 3 封装

> 本环境未挂载 `mcp__exa__web_search_exa` 工具，上述链接与版本/体积数据来自训练知识，链接可达性与具体版本号请在 implement 阶段用 `npm view echarts version` / bundlephobia 复核。

## 六、相关规范文档

| 文档路径 | 关联点 |
|---|---|
| `.trellis/tasks/09-12-repo-stats/prd.md` | 任务 PRD，已列图表库选型为 Open Question 1，倾向 ECharts |
| `docs/spec/cross-layer-contracts.md` | 新增 Wails 绑定方法须同步 `App.js / App.d.ts / models.ts` 三处 |
| `docs/spec/test-coverage-gate.md` | 前端 ≥70% 覆盖率门禁（exclude wailsjs），图表组件须可 mock |
| `docs/测试策略.md` | Vitest + @vue/test-utils + jsdom 测试范式 |
| `frontend/src/style.css` | 设计令牌（CSS 变量）定义，图表配色须对齐 |
| `frontend/src/components/FilePreviewRenderer.vue` | mermaid 主题切换范式（initialize + 重渲染），图表库可复用 |

## 七、Caveats / 未确认项

1. **版本号未锁定**：echarts / vue-echarts / chart.js / vue-chartjs 的具体最新稳定版本号未在本环境实查（无 npm 网络访问），implement 阶段须 `npm view <pkg> version` 确认后锁定。
2. **体积数值为估算**：三方案 minified / gzip 体积基于训练知识近似，实际以 `vite build` 产物为准；建议选定后跑一次 `npm install` + `vite build` 用 `dist/assets/*.js` 实测校验。
3. **ECharts 按需注册的精确组件清单**：趋势图（line/bar）+ 热力图（heatmap + calendar）+ 通用（tooltip / grid / visualMap / legend）所需 `use` 的组件/图表器最终清单，须在 implement 时按实际 option 反推，本研究的清单为最小预估。
4. **方案 C 的 tooltip 交互自实现成本**未深入评估：若热力图需悬停某日显示"X 提交"，纯 CSS 方案须自写定位与内容渲染，复杂度高于 ECharts 的 `tooltip` formatter。
5. **未调研** unplugin-vue-components 自动按需导入 vue-echarts 的可行性（可降低手动 `use` 注册负担，但增加配置复杂度），implement 阶段可评估。
6. **mermaid 与 ECharts 共存**的体积叠加效应未实测：两者皆为重型可视化库，若统计页与 markdown 预览页不同时渲染，可考虑路由级懒加载隔离，避免首屏加载两者。
