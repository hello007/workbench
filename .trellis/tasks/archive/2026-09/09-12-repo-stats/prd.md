# 仓库统计功能

## Goal

把仓库提交历史聚合成可视化数据，给用户看「谁在贡献、什么时候活跃、改了多少」，落地路线图「团队协作 → 统计功能」。数据复用 `service/commit_history_cache.go` 纯内存缓存，不重复扫 git log。

## What I already know

### 数据基础（已就位）

* `service/commit_history_cache.go`：全量快照含 `Files` 字段，HEAD SHA 增量更新，TTL 5min，单仓上限 5000
* `App.GetCommitHistory(path, limit, offset, filter)` (`app_git.go:148`)：已支持 `model.CommitFilter`（Author/Keyword/Since/Until/FilePath）过滤 + 分页
* 缓存命中/增量/全量三态解析在 `resolveCommitHistory`，超限仓库（>5000）回退原 go-git 路径不缓存

### model.Commit 结构（model/commit.go:4）

```go
type Commit struct {
    SHA       string   // 40 位
    ShortSHA  string   // 前 8 位
    Message   string
    Author    string   // 名称
    Email     string   // 邮箱
    Timestamp int64    // Unix
    DateTime  string   // 格式化
    Files     []string // 变更文件路径列表
}
```

**关键缺口**：无 Additions/Deletions 行数字段。`getCommitFiles` (`app_git.go:461`) 用 `currentTree.Patch(parentTree)` 遍历 `FilePatches()` 只取 `Path()`，未统计行数。

### 前端现状

* 技术栈 Vue3 + Element Plus + Vite，package.json 无 echarts/chart.js 依赖
* `frontend/src` 无任何统计/图表组件
* 已有 `CommitHistory.vue` 组件（提交历史列表），统计页可参考其数据获取模式
* store/composables/router 分层清晰

### 后端测试

* `app_git_cache_test.go` 有完整注入缓存测试范式（`newAppWithCommitCache` + `setupRepoWithCommits`）
* 覆盖率门禁：service ≥76% / 前端 ≥70%

## Assumptions (temporary)

* 统计计算放后端 service 层聚合，返结构化数据给前端（避免前端拿全量 commit 自算）
* 单仓统计为主，跨仓汇总非 MVP
* 图表库新引入，倾向 ECharts（功能全，热力图/折线/柱状都覆盖）

## Open Questions

（无 — 待 Step 8 用户最终确认）

### 时间窗口：固定档位 + 粒度自动适配

**Context**：趋势图需聚合粒度，热力图需显示范围。用户是否需自定义日期区间。

**Decision**：MVP 用固定档位（最近 7 天/30 天/90 天/1 年/全部），聚合粒度随档位自动适配（7 天按日、30 天按日、90 天按周、1 年按周、全部按月）。热力图始终显示最近 1 年。自定义区间留 Phase 2。

**Consequences**：MVP 简洁出图快；用户无法选自定义区间（Phase 2 补）；档位与粒度映射在后端聚合方法中固化，可配置化。

## Research References

* [`research/chart-lib-selection.md`](research/chart-lib-selection.md) — 推荐 ECharts（vue-echarts 封装），单库覆盖三种可视化，原生 calendar heatmap 是决定性差异
* [`research/heatmap-implementation.md`](research/heatmap-implementation.md) — ECharts calendar heatmap 首选；纯 CSS/现成组件库均需双轨，无优势

## Decision (ADR-lite)

### 图表库选型：ECharts（vue-echarts 封装）

**Context**：MVP 三种可视化（趋势折线/柱状、贡献者排名柱状、活跃度热力图）需选图表库。前端无任何图表依赖。

**Decision**：引入 ECharts，按需注册（echarts/core + HeatmapChart/LineChart/BarChart + CalendarComponent/TooltipComponent/VisualMapComponent/GridComponent + CanvasRenderer）+ vue-echarts 声明式绑定。

**Consequences**：
- 优势：单库单范式覆盖三图；原生 calendar+visualMap 兜底热力图边界（周对齐/跨年/月标签/空日）；暗色主题可复用 mermaid 已建立的 initialize+重渲染模式（FilePreviewRenderer.vue:384/496）
- 代价：bundle 增量约 70–250KB gzip（桌面端可接受）；需显式装 dayjs（当前仅 mermaid 传递依赖）
- 风险：版本/体积数据来自训练知识，implement 阶段须 `npm view echarts version` + `vite build` 实测；ECharts 按需引入在 Vite8+rolldown 下 tree-shake 效果未验证

## Requirements (evolving)

### MVP 范围（已定：A）

* **提交趋势统计**：按日/周/月聚合 commit 数量，折线/柱状图
* **活跃度热力图**：类 GitHub 草地，时间轴 + 颜色深浅表提交密度
* **贡献者提交数排名**：按作者分组，提交数排名（不含行数）
* 复用 `commit_history_cache` 数据（Author/Timestamp/Files 现成字段），不重复扫 git log
* 后端 service 层聚合统计，返结构化数据
* 前端新增统计页 + 图表组件
* Wails 绑定新增统计方法（改 App 签名须同步 `frontend/wailsjs/` 三处）

## Acceptance Criteria (evolving)

* [ ] 统计方法复用缓存，不触发全量 git log 重扫
* [ ] 提交趋势图支持日/周/月聚合切换
* [ ] 活跃度热力图按日渲染提交密度（类 GitHub 草地）
* [ ] 贡献者排名展示作者 + 提交数
* [ ] 后端 service 覆盖率 ≥76%，前端 ≥70%
* [ ] 跨层契约同步（wailsjs App.js/App.d.ts/models.ts）

## Definition of Done (team quality bar)

* Tests added/updated（service 单测 + 前端组件测）
* Lint / typecheck / CI green
* README.md / docs 更新（功能说明.md 补统计功能）
* 跨层契约同步

## Out of Scope (explicit)

* **贡献者新增/删除行数排名**（Phase 2）：缓存 `model.Commit` 无 Additions/Deletions 字段，需补算 patch.Stats，留后续迭代
* **跨仓汇总**：MVP 单仓统计，跨仓汇总留后续
* **自定义日期区间**：MVP 用固定档位，自定义区间留后续
* PR 级统计（merge/review 数据）
* 实时推送（统计为按需拉取）

## Technical Approach

### 后端（Go）

* 新增 `service/repo_stats.go`：纯函数聚合 `AggregateCommitStats(commits []model.Commit, granularity string) RepoStats`，接收 commit 列表做时序/贡献者/热力图聚合，不依赖 git 仓库对象，便于单测注入
* 新增 `App.GetRepoStats(path, range string) (RepoStats, error)`：复用 `resolveCommitHistory` 拿全量快照（命中缓存三态逻辑），调 service 聚合返结构化数据
* 数据契约（`model/repo_stats.go`）：

```go
type RepoStats struct {
    Trend        []TimeBucket  `json:"trend"`        // [{date, count}] 按粒度聚合
    Contributors []Contributor `json:"contributors"` // [{author, email, count}] 按 count 降序
    Heatmap      []DayCount    `json:"heatmap"`      // [{date, count}] 最近1年按日
    TotalCommits int           `json:"totalCommits"`
    DateRange    string        `json:"dateRange"`
}
```

### 前端（Vue3）

* 新增 `frontend/src/views/StatsView.vue`：统计页容器，档位切换 + loading 态（参考 `CommitHistory.vue`）
* 新增 `frontend/src/components/RepoStatsChart.vue`：ECharts 图表组件（vue-echarts 声明式绑定），含趋势折线/柱状、贡献者柱状、热力图 calendar heatmap
* 按需引入 `echarts/core` + 各 Chart/Component + `CanvasRenderer`，`npm i echarts vue-echarts dayjs`
* 路由 + 导航中心新增统计入口

### 跨层契约

* `App.GetRepoStats` 新增方法须同步 `frontend/wailsjs/go/main/App.js` / `App.d.ts` / `models.ts` 三处（见 `docs/spec/cross-layer-contracts.md`）

### 实现计划（小 PR）

* **PR1**：后端 `service/repo_stats.go` 聚合方法 + `model/repo_stats.go` 数据契约 + `App.GetRepoStats` 绑定 + service 单测（覆盖率 ≥76%）
* **PR2**：前端装依赖 + `RepoStatsChart.vue` 图表组件 + `StatsView.vue` 页面 + 路由/导航入口 + 组件测（覆盖率 ≥70%）
* **PR3**：跨层契约同步校验 + README/功能说明.md 更新 + 边界场景（空仓库/单 commit/超限仓库回退路径）

## Technical Notes

* 缓存复用入口：`App.GetCommitHistory` → `resolveCommitHistory` → `filterCommits`；统计方法复用前两步，跳过 `filterCommits` 分页
* 超限仓库（>5000 commit）回退非缓存路径，统计走 go-git 全量扫，性能较慢需 loading 态兜底
* 图表组件参考 `CommitHistory.vue` 数据获取 + loading 态模式
* 暗色主题复用 mermaid 已建立的 initialize + 主题切换重渲染模式（`FilePreviewRenderer.vue:384/496`）
* 跨层契约文档：`docs/spec/cross-layer-contracts.md`
* 未验证项：ECharts 版本/体积须 implement 阶段 `npm view echarts version` + `vite build` 实测
