# T1 新增只读「今日/本周概览」入口

- 父任务：`11-24-personal-dashboard`，执行顺序第 2 项（序号 1）
- 阶段 / 优先级：阶段 1 P0 纯聚合 / P0
- 前置：T0；本项是 T2、T3 的骨架
- 状态：planning（仅登记，未 start）

## 目标

新增第 11 个侧边栏面板，把分散在四个入口的数据收敛到一屏，卡片可点击下钻回原面板。只读聚合，**不改写事实源**。

## 已有数据（零采集成本）

`GetDashboardStatuses`（pin 仓库 dirty/ahead/behind/stash）、`useRecentAccess`（最近访问 50 条）、`temp_todos.json`、`AiTaskHistoryStats`。ECharts 基建见 `RepoStatsChart.vue`；`v-show` + `v-loading` + `requestSeq` 竞态守卫见 `StatsView.vue`。

## 契约同步清单

新增 `service/` + `model/`；`app_services.go` 两处（`AppServices` 字段 + `NewAppServices` 构造行）；`app_*.go` 委托；`wailsjs` 三处（`App.js` / `App.d.ts` / `models.ts`）；前端四处（`sidebarPanels.js` 的 `SIDEBAR_PANELS`、`ActivityBar.vue` 的 `panelIconMap`、`Home.vue` 的 `v-show`、`SettingsPanel.vue` + `settings.js`）。第 11 项无 `Alt+1..9` 映射，须配自定义快捷键；`router/index.js` 无需改动。

## 验收标准（可测试）

一屏可见 4 路数据且下钻可用；`wails generate module` 后 `go test ./...`、`npm run build`、覆盖率门禁全部通过；未新增任何采集层或 LLM 调用。

## 不做的事

不做跨端/云同步，不改动现有 dashboard 与 stats 面板。
