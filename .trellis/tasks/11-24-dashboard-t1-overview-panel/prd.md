# T1 新增只读「今日/本周概览」入口

- 父任务：`11-24-personal-dashboard`，执行顺序第 2 项（序号 1）
- 阶段 / 优先级：阶段 1 P0 纯聚合 / P0
- 前置：T0（`docs/spec/dashboard-metrics.md` 已锁定口径）
- 状态：planning（brainstorm 已完成，待 `task.py start`）

## Goal

新增第 11 个侧边栏面板「概览」（id `overview`），把分散在多个入口的数据收敛到一屏：
打开工作台 10 秒内回答「今天先动哪个仓库、AI 花了多少、哪些事没清」。卡片只读聚合、
可下钻回原面板，**不改写任何事实源**，零新增采集层、零 LLM 调用。

上板指标固定为 `docs/spec/dashboard-metrics.md` 的 M1–M5（3 个数据域、5 张卡片），
模块数不超过 5。

## Requirements

### R1 面板与入口

- `SIDEBAR_PANELS` 末尾追加 `{ id: 'overview', label: '概览' }`，成为第 11 项。
- `Alt+1..9` 固定映射语义不变；本面板无固定数字键，靠可自定义快捷键
  `openOverview`（默认空串，用户按需录制）补位，与「提示词」面板同构。
- 冷启动默认面板仍为 `directory`；有会话快照时按快照恢复。
- 前端注册四处：`sidebarPanels.js`、`ActivityBar.vue` 的 `panelIconMap`、
  `Home.vue` 的 `v-show` 挂载、`SettingsPanel.vue` + `store/settings.js` 快捷键项。
- 面板按 `v-show` 常驻挂载，切走不丢快照与滚动位置。

### R2 数据契约

- 新增 `GetOverviewSnapshot(range string, force bool) (*model.OverviewSnapshot, error)`：
  **单次 IPC** 返回 5 卡聚合结果；`force=true` 跳过 TTL 强制重算。
- `range` 从 T1 就进入签名（`today` / `week`），T1 内部固定返回「今日 + 本周」双窗口块，
  T3 只加切换控件、不改契约。
- 后端按 `docs/spec/dashboard-metrics.md` 计算全部口径，前端只渲染不重算。
- 快照带 `GeneratedAt`（Unix 毫秒），前端据此展示「统计于 HH:mm」。

### R3 五卡口径

| 卡片 | 展示 | 计算口径 |
|---|---|---|
| M1 待收尾仓库 | 待收尾仓库数 / pin 总数 | `Dirty==true` 或 `StashCount>0`；`Missing` 与 `IsRepo==false` 不计入，单列「失效仓库 N 个」 |
| M2 久未推进仓库 | 数量 + 当前阈值 | `LastCommitUnix>0` 且当前时间 − `LastCommitUnix` > 阈值 × 24h（按实际时长比较，7 天 1 小时计入）；`LastCommitUnix==0` 单列「无提交 N 个」 |
| M3 待办积压 | 未完成条数 + 最老滞留天数 | `CompletedAt==nil`；最老天数 = now − min(CreatedAt)，≥7 天高亮 |
| M4 AI 成本 | 今日/本周成本 + token 四项 | 窗口按 `StartedAt`；`Metrics==nil` 计入任务数但不计成本 |
| M5 AI 成功率 | 本周成功率 | `success ÷ (success+failed+timeout)`，**canceled 不进分母**；无有效任务显示「无数据」 |

### R4 下钻

- M1/M2 → 切 `dashboard` 面板，零载荷。
- M3 → 经 `uiStore.overviewDrillRequest` 信箱切 `todo` 并定位最老未完成条目。
- M4/M5 → 经同一信箱切 `ai` 并打开 AI 历史抽屉、传入时间窗；**不预设状态筛选**。
- 信箱遵循 `pushTraceBlameRequest` 的 seq 单调递增 + 消费即清空语义，不持久化。

### R5 刷新

- 进入面板即拉取；后端 5 分钟 TTL 内复用缓存，超过 TTL 或 `force=true` 才重算。
- 面板提供手动刷新按钮；TTL 判定只在后端，前端不自行计时。
- 前端用 `requestSeq` 丢弃并发旧响应，对齐 `StatsView.vue` 范式。

### R6 错误与空态

- **分区降级**：`RepoOverview` / `AIOverview` / `TodoOverview` 各自带 `error` 字段；
  单源失败只让对应卡片显示「加载失败 + 重试」，不影响其他卡片。
- 禁止把失败静默降级为 0。
- 空态：无 pin 仓库时 M1/M2 显示「未关注仓库」并可下钻到状态看板添加流程；
  无待办显示 0；无 AI 任务时 M4 显示 `$0.00`、M5 显示「无数据」；不做整面板空态插画。

### R7 设置项

- `AppSettings.Dashboard.StaleRepoDays`（默认 7，可选 7/14/30），经
  `EnsureAllDefaults` + `PrivateCopy` 两单点落地；设置页新增下拉项。

### R8 浏览器访问模式

- `GetOverviewSnapshot` **不加入** `RPCHiddenMethods`：三个数据源现有面板均已对浏览器
  开放，本面板不新增敏感字段；浏览器模式下侧边栏入口可用。

## Acceptance Criteria

- [ ] 侧边栏出现第 11 项「概览」，点击/快捷键可进入，`Alt+1..9` 行为不变。
- [ ] 一屏可见 M1–M5 五张卡片，数值与 `GetDashboardStatuses` / `ListTempTodos` /
      `GetAiTaskHistory` 原始数据逐项可核对。
- [ ] 口径断层标注齐全：behind 近似值（下钻处）、失效仓库单列、无提交单列、
      AI「近 90 天 / 最多 2000 条」。
- [ ] M1/M2 下钻切到状态看板；M3 下钻切到待办并定位最老条目；M4/M5 下钻切到 AI 面板
      并打开历史抽屉、时间窗正确。
- [ ] M5 在周期内无有效任务时显示「无数据」而非 0%。
- [ ] 5 分钟 TTL：切走再切回不重复计算；点刷新后数据更新。
- [ ] 单数据源失败时仅该卡片降级，其余卡片正常。
- [ ] 设置页可切换 7/14/30 天，M2 计数随之变化。
- [ ] `wails generate module` 后 `go test ./...`、`npm run build` 通过，覆盖率门禁不降。
- [ ] 未新增采集层、未新增 LLM 调用。

## Definition of Done

- 后端 `service/overview_test.go` 覆盖：五卡计数、阈值边界、空态、分区错误、TTL 命中/过期。
- 前端卡片组件单测 + `OverviewView` 单测（并发旧响应丢弃、下钻信箱 seq）。
- E2E：面板可达、五卡渲染、三类下钻目标正确。
- 跨层契约同步 `frontend/wailsjs/` 三处（`App.js` / `App.d.ts` / `models.ts`）。
- 回写父任务第 2 节 T2 完成标志、T2 的 `prd.md`；必要时更新 `README.md`。

## Technical Approach

### 后端

| 层 | 文件 | 内容 |
|---|---|---|
| model | `model/overview.go` | `OverviewSnapshot` / `RepoOverview` / `AIOverview` / `AICostBlock` / `TodoOverview`；各域带 `Error string` |
| service | `service/overview.go` | `OverviewService` 依赖 `*DashboardService`、`*TempTodoService`、`*AiFunctionService`；注入 `now func() time.Time` 便于测试；内建 5 分钟缓存（`mu` + `cached` + `cachedAt`） |
| service | `service/overview.go` | AI 数据一次 `GetAiTaskHistory(nil)` 全量读取，内存按 `StartedAt` 切今日/本周，避免两次 loadAll |
| app | `app_overview.go` | `GetOverviewSnapshot(range, force)` 委托 service；整体仅在后端不可恢复错误时返回 error |
| 装配 | `app_services.go` | `AppServices` 字段 + `NewAppServices` 构造行（构造在三个依赖 service 之后，纯 `new`） |
| 设置 | `model/settings.go` | `DashboardSettings{ StaleRepoDays int }`；`EnsureAllDefaults` 与 `PrivateCopy` 各加一行 |

M2 阈值从 `SettingsService` 读取；读取失败回落 7 天并记 Warn，不阻断快照。

### 前端

| 层 | 文件 | 内容 |
|---|---|---|
| 面板 | `views/OverviewView.vue` | 拉取、TTL 展示、`requestSeq`、手刷、三域布局 |
| 卡片 | `components/overview/M1TodoReposCard.vue` 等 5 个 | 每卡独立 props + `@drill` 事件，便于未来合并面板时整体迁移 |
| 信箱 | `store/ui.js` | `overviewDrillRequest` + `pushOverviewDrillRequest(target, payload)`，seq 单调递增 |
| 消费 | `Home.vue` / `TempTodoPanel.vue` / `AiFunctionPanel.vue` | 切面板；定位最老条目；打开历史抽屉并设时间窗 |
| 设置 | `store/settings.js` + `SettingsPanel.vue` | `staleRepoDays`、`openOverview` 读写与设置项 |

## Decision Record (ADR-lite)

| # | 决策 | 关键理由 |
|---|---|---|
| 1 | 面板追加为第 11 项，冷启动仍 `directory` | 不破坏 `Alt+1..9` 语义与既有 E2E；T4 门槛需要「主动打开」可测 |
| 2 | 与「状态看板」「仓库统计」并存，合并另立任务 | T1 是骨架，替换会让范围膨胀且删除 T4 未验证的可用面板 |
| 3 | 卡片按独立可复用组件设计 | 为未来合并预留扩展点，避免重写 |
| 4 | 后端聚合 service，单次 IPC | 口径单点；T8 阈值提醒可直接复用；避免前端重复计算 |
| 5 | M5 在 service 内用 `List` 内存计数，不改 `Stats` 契约 | T1 即可出正确成功率；`CanceledCount` 留 T2 决策 |
| 6 | 下钻折中：M1/M2 零载荷；M3/M4/M5 走信箱 | 保住 spec 的「最老置顶」；不把 T1 拖进改 AI 抽屉筛选模型 |
| 7 | T1 完整出 M4/M5 卡片，T2 收缩为口径统一 + 排行视图 | 避免 T1/T2 验收重叠 |
| 8 | 刷新 = 进入即拉 + 5 分钟 TTL + 手刷 | 避免每次切回重跑全部仓库 git 子进程 |
| 9 | T1 实现 M2 阈值设置项 | spec 已锁定「可调」，两单点机制成熟，返工成本高于实现成本 |
| 10 | 浏览器模式不隐藏 | 三源已开放，不新增暴露面；隐藏会造成同数据「单看可见、汇总不可见」矛盾 |
| 11 | 分区错误降级 | 一个失效仓库不应拖垮整屏；禁止失败伪装成 0 |

## Out of Scope

- 不删除、不改写「状态看板」与「仓库统计」的内部实现；合并另立任务并先回写 spec。
- 不做自定义拖拽布局、不下拉时间范围控件（T3）、不做排行图与失败清单双状态（T2）。
- 不纳入 `useRecentAccess` 上卡，仅可作入口导航辅助。
- 不做外部健康/财务/睡眠数据、云同步、多端、多租户。

## Known Gaps

- M5「失败 + 超时一次看全」：`AiTaskHistoryFilter.Status` 为单值，T1 不预设状态筛选，
  交 T2 决定是否扩展抽屉筛选模型。
- M2 的「无提交」与「失效仓库」只单列不计数，符合 spec §3 口径。
- AI 历史保留 90 天 / 2000 条，超出范围不可见，卡片必须标注。

## Implementation Plan

- **PR1 后端**：`model/overview.go` + `service/overview.go` + `app_overview.go` +
  `app_services.go` 装配 + `settings.go` 新段 + 单测 + `wailsjs` 三处同步。
- **PR2 前端骨架**：`OverviewView.vue` + 5 个卡片组件 + 面板四处注册 + 快捷键项 +
  空态/错误态 + 单测。
- **PR3 下钻与收尾**：`overviewDrillRequest` 信箱 + 三处消费改造 + E2E + 父任务/T2 文档回写。

## Research References

本任务无外部技术选型需求，未启用 `trellis-research`；全部结论来自仓内取证与 T0 已锁定的
`docs/spec/dashboard-metrics.md`。

## Technical Notes

- 面板注册四处：`sidebarPanels.js`、`ActivityBar.vue:449` `panelIconMap`、`Home.vue:69-101`、
  `SettingsPanel.vue` 快捷键列表。
- `useSessionState` 的合法面板集合自 `SIDEBAR_PANELS` 派生，新增面板无需二次登记。
- 现有数据源：`GetDashboardStatuses()`、`ListTempTodos()`、
  `AiFunctionService.GetAiTaskHistory(filter)`（`historySvc` 为私有字段，**不要直接访问**）。
- `AiTaskHistoryFilter.From/To` 按 `StartedAt` 过滤；保留裁剪按 `FinishedAt`（已知微小差异）。
- 竞态：`requestSeq`（前端）+ service `mu`（后端缓存）。
- 设置段新增只改两单点，禁止在消费点手抄默认值。
