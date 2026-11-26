# 个人数据看板指标口径契约

> 适用范围：侧边栏「个人看板」相关任务（T1 概览入口、T2 AI 成本上浮、T3 时间范围、
> T7 待办节奏、T8 规则提醒）的指标定义、数据源、阈值与口径标注。
> 真相源于任务 `11-24-dashboard-t0-confirm-scope`（阶段 0 决策，2026-11-25 锁定组合 A）。
> **本契约优先于旧任务描述**：若 T1/T3 的 PRD 与本文冲突，以本文为准并回写该 PRD。

## 1. 前提边界

| 编号 | 假设 | 结论 |
|---|---|---|
| H1 | 「现有工作台」即本仓 WorkBench（Wails + Go + Vue3 单用户桌面应用） | 成立（`wails.json`、`SIDEBAR_PANELS`、`app_services.go` 取证） |
| H2 | 「个人数据看板」仅限工作台内使用/产出概览，不含外部健康/财务/睡眠数据 | 成立（现有数据源全部为工作台内产出） |

外部数据属 Not Now：若要纳入，须另立采集任务并重新过 Signal Check，不得在本期看板隐性开口。

## 2. 上板指标（5 项 = 主看板 5 个模块）

Signal Check 三问（Q1 改变明天动作 / Q2 自动采集或 30 秒内更新 / Q3 每周至少看一次）全部为
Yes 才可上主看板。

| 编号 | 模块 | 定义与口径 | 数据源 | 默认阈值 | 下钻 |
|---|---|---|---|---|---|
| M1 | 待收尾仓库 | 已 pin 仓库中 `dirty=true` 或 `stashCount>0` 的仓库数；同时显示 pin 总数 | `RepoStatus.Dirty` / `StashCount` | >0 即高亮 | 仓库名 + dirtyCount + stashCount + behind |
| M2 | 久未推进仓库 | 已 pin 仓库中 `LastCommitUnix` 距今天数 > 阈值的仓库数 | `RepoStatus.LastCommitUnix`（committer 时间，Unix 秒） | 7 天，设置项可调（7/14/30） | 仓库名 + 距今天数 |
| M3 | 待办积压 | 未完成待办条数 + 最老未完成条目已滞留天数 | `TempTodo.CompletedAt == nil`；最老天数 = now − min(CreatedAt) | 条数 >0 即显示；最老天数 ≥7 天高亮 | 待办列表，最老置顶标记 |
| M4 | AI 今日/本周成本 | 今日与本周（周一 00:00 起）AI 任务总成本 USD，附 token 四项 | `AiTaskHistoryService.Stats(filter)` → `TotalCostUSD` + token 四分项；时间窗按 `StartedAt` | 无阈值，仅展示 | 按功能项排行（`ByFunction`） |
| M5 | AI 成功率 | 本周成功率 = success ÷ (success + failed + timeout)，**canceled 不进分母** | `SuccessCount` + 按 `Status` 计数（`TotalCount` 含 canceled，不可直接作分母；见 §5） | 周期内无任务显示「无数据」，不显示 0% | 失败/超时任务清单 |

主看板模块数上限 = 5。新增第 6 个模块须先回写本文档并说明淘汰哪个。

## 3. 口径断层与标注要求（必须显式标注）

| 指标 | 断层 | 前端展示要求 |
|---|---|---|
| M1 | `behind` 基于本地已有远程引用，非实时 | 下钻处标注「相对上次 fetch 的近似值」 |
| M1 | `Missing=true`（目录已删）与 `IsRepo=false` 不计入 M1 计数 | 单列灰显「失效仓库 N 个」 |
| M2 | `LastCommitUnix==0`（空仓库/unborn/查询失败）无法算天数 | 单列「无提交 N 个」，不计入久未推进数 |
| M4/M5 | AI 历史保留 90 天 / 2000 条，超出不可见 | 卡片标注「近 90 天 / 最多 2000 条内」 |
| M4/M5 | 保留裁剪按 `FinishedAt`，查询时间窗按 `StartedAt` | 跨天边界任务可能不计入，属已知微小差异，T2 无需消除 |
| M3 | 已完成待办保留 30 天 / 500 条 | 只统计未完成，本指标不受影响，无需标注 |

## 4. 快照类与窗口类的边界（约束 T1 / T3）

- **快照或全量类**：M1、M2（当前 `RepoStatus` 快照）、M3（全量未完成待办）。
  **不加时间范围控件**——套时间窗会产生「7 天前的仓库状态」这种无意义语义。
- **窗口类**：M4、M5。时间范围控件只覆盖这两张卡片，今日/本周切换。
- **T1 口径修正**：原「一屏可见 4 路数据」修正为 **3 个数据域、5 张卡片**
  （仓库状态 → M1/M2；AI 任务 → M4/M5；待办 → M3）。「仓库统计」域不再占主位，
  趋势/贡献者/热力图仅作下钻，不新增主看板模块。

## 5. M5 的实现前置（T2 须处理）

`model.AiTaskHistoryStats` 当前只有 `TotalCount` / `SuccessCount`，**没有 canceled 计数**。
按 §2 口径「canceled 不进分母」，T2 二选一：

1. 给 `AiTaskHistoryStats` 增加 `CanceledCount` 字段——跨层契约，须同步
   `frontend/wailsjs/` 三处（`App.js` / `App.d.ts` / `models.ts`，见 `cross-layer-contracts.md`）；
2. 用现有 `List(filter)` 在前端按 `Status` 过滤计数——不超过 2000 条，可接受，但多一次 IPC。

**默认决策**：优先方案 1（统计在后端算、前端只渲染，与仓内惯例一致）。若 T2 评估改契约
成本过高，可降级方案 2，但须在 T2 的 PRD 登记理由。

**实现状态（2026-11-26，T2 已落地）**：采用方案 1 并扩展为四状态计数——
`AiTaskHistoryStats` 新增 `CanceledCount` / `FailedCount` / `TimeoutCount`，
`FunctionStat` 新增 `CanceledCount`（`Count` 语义不变，仍含 canceled）；
`AiTaskHistoryFilter` 新增 `Statuses []string`（非空优先于 `Status`，供「失败/超时清单」
多状态下钻）。`OverviewService.buildAI` 改为两次调用 `GetAiTaskHistoryStats`
（今日、本周），概览与 AI 历史面板**同源聚合**，成功率分母 =
`SuccessCount + FailedCount + TimeoutCount`（与卡片文案「success ÷（success+failed+timeout）」
同式，不依赖「归档状态为闭集」假设；历史文件若出现未知状态只计入 `TotalCount`，不污染分母）。
四状态计数之和 = `TotalCount` 保留为当前 `Stats` 实现的验证点，**不作为分母推导前提**。
今日/本周两次 `Stats` 读取非原子，属秒级窗口，在 5 分钟 TTL 下可接受。

T1 曾在 `service/overview.go` 内手写全量读取与状态计数，该实现已于 T2 移除，禁止回退。

## 6. 排除清单（明确不上主看板）

| 排除项 | 原因 |
|---|---|
| 总提交数、总面板数等累计计数 | 虚荣指标，不改变明天动作（Q1=No） |
| 饼图占比 | 无法支撑决策，只增加认知负担 |
| 3D 仪表盘 | 装饰性，与「10 秒读屏」目标冲突 |
| 伪精确置信度 | 无统计依据的假精确 |
| 无下钻的数字墙 | 看到数却无法行动 |
| 外部健康/财务/睡眠数据 | H2 边界外，无数据源且需手工录入 |
| 贡献者排行 | 单用户桌面场景基本只有自己（Q1=No） |
| 提交热力图 | 装饰性（Q1=No） |
| 今日/本周提交数 | 与 M2 重复且更易沦为虚荣指标 |
| 审计失败 RPC 数 | 用户不以此做决策（Q1=No） |
| 终端会话数 / 面板活跃度 | 需新增采集层（Q2=No），待 T5 落地后再评估 |
| 最近访问 Top N（`useRecentAccess`） | Q1 弱、Q3 否；仅作入口导航辅助，不占上板卡片 |

## 7. 下游任务约束映射

| 任务 | 约束 |
|---|---|
| T1 概览入口 | 按 §4 的「3 域 5 卡」实施；模块数 ≤5 |
| T2 AI 成本上浮 | 实现 M4 + M5；§5 前置已落地（四状态计数 + `Statuses` 多状态下钻），概览改走 `Stats` 同源聚合，成功率分母取 success+failed+timeout 三态之和 |
| T3 时间范围 | 范围控件只覆盖 M4/M5；M1–M3 不加时间窗 |
| T7 待办节奏 | M3 为起点；扩展的完成率/积压趋势不得改变「只统计未完成」的基线口径 |
| T8 规则提醒 | 告警阈值复用 M1–M3 的默认阈值，不另立一套 |

## 8. 变更规则

- 增删指标、修改口径或阈值 → 先改本文档，再改对应任务 PRD；**禁只改代码不回写**。
- 新增数据源（如为 M2 增加「距上次拉取」）须先过 Signal Check 三问，并确认不违反
  「阶段 1 零采集层」约束。
- 涉及 `model/` 导出 struct 字段的变更，同步 `frontend/wailsjs/` 三处。

## 9. 数据源位置速查

| 数据 | 位置 |
|---|---|
| 概览聚合（M1–M5 单次快照） | `model/overview.go`、`service/overview.go`、`app_overview.go` |
| 仓库状态 | `model/dashboard.go`（`RepoStatus`）、`service/dashboard.go`、`app_dashboard.go` |
| AI 历史统计 | `service/ai_task_history.go`（`Stats` / `List` / `UsageCounts`）、`model/ai_task_history.go`；`Stats` 返回四状态计数，`List` 支持 `Statuses` 多状态匹配 |
| 待办 | `service/temp_todo.go`（`List`）、`model/temp_todo.go` |
| 仓库统计（下钻用） | `service/repo_stats.go`（`AggregateRepoStats`）、`model/repo_stats.go` |
