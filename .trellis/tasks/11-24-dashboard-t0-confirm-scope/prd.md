# T0 确认评估前提并锁定上板指标

- 父任务：`11-24-personal-dashboard`，执行顺序第 1 项（序号 0）
- 阶段 / 优先级：阶段 0 前置确认 / P0
- 前置：无；**本项阻塞全部后续任务**
- 状态：in_progress（PRD 已定稿，文档交付完成，待归档）
- 决策人：liuyang（2026-11-25 确认组合 A）

## 1. 目标与结论摘要

在写任何代码前锁死方向、指标范围与口径，避免做出「建完不看的看板」。

**结论**：两条前提假设成立；主看板上板 **5 个指标（5 张卡片）**，全部落在已有数据源，
零新增采集层、不引入 LLM；「仓库统计」类图表（趋势/贡献者/热力图）降为下钻，
不占主位。

## 2. 前提假设确认

| 编号 | 假设 | 核对方式 | 证据 | 结论 |
|---|---|---|---|---|
| H1 | 「现有工作台」即本仓 WorkBench | 代码核实 | `wails.json`（Wails v2 应用）、`frontend/src/store/sidebarPanels.js`（9 个面板）、`app_services.go`（服务装配）；均为单用户桌面形态，无登录/多租户 | **成立** |
| H2 | 「个人数据看板」仅限工作台内使用/产出概览 | 数据源盘点 | 现有数据源（`RepoStatus`、`RepoStats`、`AiTaskHistory`、`TempTodo`、审计日志、素材 SQLite）全部为工作台内产出；无任何外部健康/财务/睡眠接入 | **成立** |

H2 的边界补充：外部数据（工时、睡眠、财务）一律进 Not Now；若未来要纳入，须另立
采集任务并重新过 Signal Check，不得在本期看板隐性开口。

## 3. 上板指标清单（5 项 = 主看板 5 个模块）

Signal Check 三问：Q1 知道这个数会改变明天的动作？Q2 能自动采集或 30 秒内更新？
Q3 每周至少看一次？

| 编号 | 模块 | 指标定义与口径 | 数据源 | Q1 | Q2 | Q3 | 默认阈值 | 下钻内容 |
|---|---|---|---|---|---|---|---|---|
| M1 | 待收尾仓库 | 已 pin 仓库中 `dirty=true` 或 `stashCount>0` 的仓库数；同时显示 pin 总数 | `RepoStatus.Dirty` / `StashCount`（git status --porcelain、git stash list） | 是 | 是（git 秒级） | 是 | >0 即高亮 | 仓库名 + dirtyCount + stashCount + behind |
| M2 | 久未推进仓库 | 已 pin 仓库中 `LastCommitUnix` 距今天数 > 阈值的仓库数 | `RepoStatus.LastCommitUnix`（committer 时间，Unix 秒） | 是 | 是 | 是 | 默认 7 天，设置项可调（7/14/30） | 仓库名 + 距今天数 |
| M3 | 待办积压 | 未完成待办条数 + 最老未完成条目已滞留天数 | `TempTodo.CompletedAt == nil`；最老天数 = now − min(CreatedAt) | 是 | 是 | 是 | 条数 >0 即显示；最老天数 ≥7 天高亮 | 待办列表，最老置顶标记 |
| M4 | AI 今日/本周成本 | 今日与本周（周一 00:00 起）AI 任务总成本 USD，附 token 四项 | `AiTaskHistoryService.Stats(filter)` → `TotalCostUSD`、token 四分项；时间窗按 `StartedAt` | 是 | 是（内存聚合） | 是 | 无阈值，仅展示 | 按功能项排行（`ByFunction`） |
| M5 | AI 成功率 | 本周任务成功率 = success ÷ (success + failed + timeout)，**canceled 不进分母** | `AiTaskHistoryStats.SuccessCount` / `TotalCount` | 是 | 是 | 是 | 周期内无任务显示「无数据」，不显示 0% | 失败/超时任务清单 |

### 3.1 必须显式标注的口径断层

| 指标 | 断层 | 前端展示要求 |
|---|---|---|
| M1 | `behind` 基于本地已有远程引用，非实时 | 下钻处标注「相对上次 fetch 的近似值」 |
| M1 | `Missing=true`（目录已删）与 `IsRepo=false` 不计入 M1 计数 | 单列灰显「失效仓库 N 个」 |
| M2 | `LastCommitUnix==0`（空仓库/unborn/查询失败）无法算天数 | 单列「无提交 N 个」，不计入久未推进数 |
| M4/M5 | AI 历史保留 90 天 / 2000 条，超出不可见 | 卡片标注「近 90 天 / 最多 2000 条内」 |
| M3 | 已完成待办保留 30 天 / 500 条 | 只统计未完成，本指标不受影响；无需标注 |

### 3.2 M5 的实现前置（T2 须处理）

`model.AiTaskHistoryStats` 当前只有 `TotalCount` / `SuccessCount`，**没有 canceled 计数**。
按上表口径「canceled 不进分母」，T2 须二选一：

1. 给 `AiTaskHistoryStats` 增加 `CanceledCount` 字段（跨层契约，须同步 `frontend/wailsjs/` 三处）；
2. 改用现有 `List(filter)` 在前端按 `Status` 过滤计数（数据量 ≤2000，可接受，但多一次 IPC）。

**决策**：优先方案 1（后端算好、前端只渲染，与仓内「统计在后端」惯例一致）；若 T2 评估
改契约成本过高，可降级方案 2，但须在 T2 的 PRD 中登记。

## 4. 排除清单（明确不上主看板）

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
| 终端会话数 / 面板活跃度 | 需新增采集层（Q2=No），且价值弱；待 T5 落地后再评估 |
| 最近访问 Top N（`useRecentAccess`） | Q1 弱、Q3 否；仅作入口导航辅助，不占上板卡片（T1 数据源中保留此定位） |

## 5. 与下游任务的口径映射

| 下游任务 | 本清单的约束 |
|---|---|
| T1 概览入口 | 原表述「一屏可见 4 路数据」修正为：**3 个数据域、5 张卡片**（仓库状态 → M1/M2；AI 任务 → M4/M5；待办 → M3）。「仓库统计」域不再占主位，趋势/热力图仅作下钻。主看板模块数 ≤5。 |
| T2 AI 成本上浮 | M4 + M5 即 T2 的主视野范围；须处理 §3.2 的 canceled 计数前置。 |
| T3 时间范围 | 范围控件**只覆盖 M4/M5**（今日/本周切换）。M1/M2/M3 是当前快照或全量未完成，**不加时间窗**，避免语义混乱。 |
| T7 待办节奏 | M3 是起点；T7 的完成率/积压趋势属扩展，须保持「只统计未完成基线」口径不冲突。 |
| T8 规则提醒 | 告警阈值直接复用 M1/M2/M3 的默认阈值，不另立一套。 |

## 6. 决策记录（ADR-lite）

**Context**：候选指标共 13 项，需在零新增采集层约束下收敛到 ≤5 项，且要能驱动「今天先
做什么」的动作。

**Decision**：采用组合 A —— M1 待收尾仓库、M2 久未推进仓库、M3 待办积压、M4 AI 成本、
M5 AI 成功率。

**Alternatives**：
- 组合 B（偏全景）：纳入素材待处理，但素材域仅对重度用户有效（Q3=否），且挤掉 AI 成功率；
- 组合 C（最小 3 项）：信息量不足以覆盖父任务核心 Job 的三段（仓库/成本/待办）。

**Consequences**：
- 收益：5 项全部有现成数据源，T1 可零采集层落地；指标与核心 Job 逐字对齐；
- 代价：仓库统计类图表（趋势、贡献者、热力图）不在主视野，仅下钻可见；
- 风险：M5 依赖 `AiTaskHistoryStats` 扩字段，T2 需同步跨层契约（见 §3.2）。

## 7. 验收标准

- [x] 两条假设各有一条可核对的证据记录（§2）
- [x] 上板指标 ≤5 项，每项写明定义、数据源、Signal Check 三问结论、阈值、下钻（§3）
- [x] 排除清单 ≥10 项且逐项给原因（§4）
- [x] 主看板模块数 = 5（M1–M5），无第 6 个模块
- [x] 口径断层逐项标注（§3.1）
- [x] 本文档经用户确认后归档，作为 T1–T11 的范围基准（2026-11-25 用户确认组合 A + 两条假设成立）
- [x] 指标口径已沉淀 `docs/spec/dashboard-metrics.md`，并登记 `docs/spec/README.md` 索引与 `AGENTS.md` 关键规则钩子

## 8. 不做的事

不做任何代码实现，不新增数据采集层，不改 `model/`、`app_*.go`、`frontend/` 任何文件。
本任务的交付物只有本文档（以及可选的口径契约文档，见技术注记）。

## 9. 技术注记

数据源位置（供 T1–T3 直接查阅）：

| 数据 | 位置 |
|---|---|
| 仓库状态 | `model/dashboard.go`（`RepoStatus`）、`service/dashboard.go`、`app_dashboard.go` |
| AI 历史统计 | `service/ai_task_history.go`（`Stats` / `List` / `UsageCounts`）、`model/ai_task_history.go`（`AiTaskHistoryStats`） |
| 待办 | `service/temp_todo.go`（`List`）、`model/temp_todo.go`（`TempTodo`） |
| 仓库统计（下钻用） | `service/repo_stats.go`（`AggregateRepoStats`）、`model/repo_stats.go` |

待决事项：是否把 §3 指标口径与 §3.1 断层标注沉淀为 `docs/spec/dashboard-metrics.md`，
作为 T1–T7 的共同契约（T0 归档后 PRD 进入 archive，后续任务不一定回读）。
