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
| M4 | AI 成本 | 所选范围（今日 / 本周 / 近 7 / 30 / 90 天，默认本周）AI 任务总成本 USD，附 token 四项；今日与本周值同时作对比基准展示 | `AiTaskHistoryService.Stats(filter)` → `TotalCostUSD` + token 四分项；时间窗按 `StartedAt`，范围起点见 §4 | 无阈值，仅展示 | 按功能项排行（`ByFunction`） |
| M5 | AI 成功率 | 所选范围成功率 = success ÷ (success + failed + timeout)，**canceled 不进分母**；默认本周 | `SuccessCount` + 按 `Status` 计数（`TotalCount` 含 canceled，不可直接作分母；见 §5） | 周期内无任务显示「无数据」，不显示 0% | 失败/超时任务清单 |

主看板模块数上限 = 5。新增第 6 个模块须先回写本文档并说明淘汰哪个。

## 3. 口径断层与标注要求（必须显式标注）

| 指标 | 断层 | 前端展示要求 |
|---|---|---|
| M1 | `behind` 基于本地已有远程引用，非实时 | 下钻处标注「相对上次 fetch 的近似值」 |
| M1 | `Missing=true`（目录已删）与 `IsRepo=false` 不计入 M1 计数 | 单列灰显「失效仓库 N 个」 |
| M2 | `LastCommitUnix==0`（空仓库/unborn/查询失败）无法算天数 | 单列「无提交 N 个」，不计入久未推进数 |
| M4/M5 | AI 历史保留 90 天 / 2000 条，超出不可见；「近 90 天」档即保留上限，无更长档位 | 卡片常显随档位联动的句子「范围：<档位> · 历史保留上限：近 90 天 / 最多 2000 条」 |
| M4/M5 | 「近 N 天」按自然日边界（今天 00:00 往前 N−1 天，含今天共 N 个自然日），与「今日/本周」同族；与仓库统计面板的滚动窗口口径不同，不作统一 | 档位标签写「近 N 天」，卡片范围文案同义 |
| M4/M5 | 保留裁剪按 `FinishedAt`，查询时间窗按 `StartedAt` | 跨天边界任务可能不计入，属已知微小差异，T2 无需消除 |
| M3 | 已完成待办保留 30 天 / 500 条 | 只统计未完成，本指标不受影响，无需标注 |

## 4. 快照类与窗口类的边界（约束 T1 / T3）

- **快照或全量类**：M1、M2（当前 `RepoStatus` 快照）、M3（全量未完成待办）。
  **不加时间范围控件**——套时间窗会产生「7 天前的仓库状态」这种无意义语义。
- **窗口类**：M4、M5。时间范围控件只覆盖这两张卡片，档位固定五档：

  | 档位 key | 标签 | 窗口起点 | 说明 |
  |---|---|---|---|
  | `today` | 今日 | 本地时区当天 00:00 | |
  | `week` | 本周 | 本地时区本周一 00:00 | **默认档** |
  | `7d` | 近 7 天 | 今天 00:00 往前 6 天 | 含今天共 7 个自然日 |
  | `30d` | 近 30 天 | 今天 00:00 往前 29 天 | |
  | `90d` | 近 90 天 | 今天 00:00 往前 89 天 | 等于历史保留上限 |

  不设 365 天档：AI 历史保留上限即 90 天，更长档位恒等于 90 天的结果，属伪档位。
  控件位于「AI 任务」域标题行右侧，位置即语义，M1–M3 不加任何范围说明文字。
  空值/非法档位回落 `week`（默认档）。
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
**该「两次 Stats」为 T2 时点实现，T3 已扩展为五档，现行契约以 §10 为准。**

T1 曾在 `service/overview.go` 内手写全量读取与状态计数，该实现已于 T2 移除，禁止回退。

**T3 扩展（2026-11-26 确认）**：`buildAI` 由「今日/本周两次 Stats」扩展为**一次 build 计算
全部五档**（今日/本周/近 7/30/90 天）并按档位缓存，`AIOverview` 新增 `RangeKey` 与
`Selected`（成本块 + 成功率 + 四状态计数）；既有 `Today`/`Week` 与顶层本周语义字段
**原样保留**，属纯增量契约：后端顶层本周语义字段与 T2 一致；前端默认档主数值按所选范围取
`selected`（week 档为 `ai.selected.cost` 本周值），与 T2 读 `ai.today`（今日）主数值的差异见 §2。

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
| 终端会话数 / 面板活跃度 | T4 门槛取证专用极轻量埋点，仅记录概览面板进入，不上主看板卡片 |
| 最近访问 Top N（`useRecentAccess`） | Q1 弱、Q3 否；仅作入口导航辅助，不占上板卡片 |
| 365 天时间档 | AI 历史保留上限 90 天，更长档位恒等于 90 天结果（T3 排除） |

## 7. 下游任务约束映射

| 任务 | 约束 |
|---|---|
| T1 概览入口 | 按 §4 的「3 域 5 卡」实施；模块数 ≤5 |
| T2 AI 成本上浮 | 实现 M4 + M5；§5 前置已落地（四状态计数 + `Statuses` 多状态下钻），概览改走 `Stats` 同源聚合，成功率分母取 success+failed+timeout 三态之和 |
| T3 时间范围 | 范围控件只覆盖 M4/M5，五档 今日/本周/近 7/30/90 天（默认本周、自然日边界、无 365 档）；M1–M3 不加时间窗；契约纯增量（`AIOverview` 新增 `RangeKey` + `Selected`，本周语义字段原样保留）；下钻带当前档位 |
| T4 门槛取证 | 仅记录概览面板主动打开，跨层契约见 [overview-open-tracking.md](overview-open-tracking.md)；判 Go 后 T5 复用同一日志器，不上主看板卡片 |
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
| 仓库统计（下钻用） | `service/repo_stats.go`（`AggregateRepoStats` / `StatsRangeWindow`）、`model/repo_stats.go` |

## 10. T3 跨层实现契约（code-spec）

> **触发**：`GetOverviewSnapshot` 档位语义扩展、`model.AIOverview` 导出字段新增、
> `frontend/wailsjs/go/models.ts` 同步、缓存失败态语义变更，属跨层契约与缓存行为变更，
> 按「Code-Spec First」补齐可执行契约。§5 的「两次 Stats」为 T2 时点描述，现行实现以本节为准。

### 10.1 Scope / Trigger

| 变更面 | 位置 | 触发理由 |
|---|---|---|
| App 绑定入参语义 | `app_overview.go` | `rangeKey` 由回显变为五档生效，签名不变 |
| 导出 struct 字段 | `model/overview.go` | `AIOverview` 新增 2 字段 + 新 `AIRangeStats` |
| 生成产物 | `frontend/wailsjs/go/models.ts` | `model/` 导出 struct 变更须手动补类型 |
| 缓存行为 | `service/overview.go` | 失败快照是否参与 TTL 命中属非显而易见行为 |

### 10.2 Signatures（单点真相源）

| 签名 | 约束 |
|---|---|
| `App.GetOverviewSnapshot(rangeKey string, force bool) (*model.OverviewSnapshot, error)` | 签名不变，恒返回 `nil error`（三域自带分区降级） |
| `OverviewService.GetSnapshot(rangeKey string, force bool) *model.OverviewSnapshot` | 五档归一化 + 缓存命中回填 |
| `normalizeOverviewRange(v string) string` | 五档白名单，空值/非法值恒回落 `week` |
| `overviewRangeStart(rangeKey string, now time.Time) time.Time` | **唯一窗口起点换算点**，build 与单测共用，禁在别处重算日期 |
| `overviewRangeKeys []string` | 五档固定顺序，一次 build 内五档共用同一 `now` |
| `aiRangeStatsFromStats(stats *model.AiTaskHistoryStats) model.AIRangeStats` | 成功率分母单点（success+failed+timeout） |
| `overviewRangeLabel` / `overviewRangeText`（`frontend/src/components/overview/overviewRange.js`） | 前端档位文案单点，非法档回落「本周」 |

### 10.3 Contracts（跨层字段）

| 字段 | 类型 | 语义与约束 |
|---|---|---|
| `OverviewSnapshot.Range` | string | 回显当前档位，与 `AI.RangeKey` 一致 |
| `AIOverview.Today` / `Week` | `AICostBlock` | **恒为今日 / 本周对比基准**，不随所选档位变化 |
| `AIOverview.RangeKey` | string | 当前档位回显 |
| `AIOverview.Selected` | `AIRangeStats` | 所选档完整统计；`week` 档须与顶层本周语义字段逐项相等 |
| `AIOverview.SuccessRate` / `HasData` / 四状态计数 | — | **恒为本周语义**，保持 T2 契约不变 |
| `AIRangeStats` | struct | `Cost` / `SuccessRate` / `HasData` / `SuccessCount` / `FailedCount` / `TimeoutCount` / `CanceledCount` |
| `models.ts` | 生成产物 | 须含 `AIRangeStats` class 与 `AIOverview.rangeKey` / `selected`；`App.js` / `App.d.ts` 两参签名不变故不改 |

**前端消费约束**：两张 AI 卡片主数值、四状态计数、`hasData` 一律读 `ai.selected`；
`ai.today` / `ai.week` 仅作 hint 对比基准，**禁混读顶层本周字段**。
下钻 payload：M4 成本卡只带 `{ range }`（全状态成本排行），M5 成功率卡带 `{ range, statuses }`。
`AiTaskHistoryPanel.applyInitialRange` 的 `7d/30d/90d` 换算与后端 `overviewRangeStart` 同口径。

### 10.4 Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| `rangeKey` 为空 / 非五档（含 `365d`） | 归一为 `week`，卡片展示与默认档一致 |
| AI 依赖缺失或五档中任一 `Stats` 失败 | `AI.Error` 非空，整域降级；仓库/待办域照常聚合；`Selected` 保持零值，**不伪装成 0 成本 / 0%** |
| `AI.Error != ""` 的快照 | **不参与 TTL 命中**，后续 `force=false` 请求每次重算，数据恢复后切档即自愈，无需手动点「重试」 |
| `force=true` | 跳过 5 分钟 TTL 强制重算三域（仅来自刷新按钮与卡片重试） |
| 缓存命中且 `AI.Error == ""` | 浅拷贝快照 + 按入参回填 `Range` / `RangeKey` / `Selected`，**不重算任何域** |
| `GetOverviewSnapshot` 在 `RPCHiddenMethods` | 保持不在清单（浏览器通道可用）；快照顶层字段闭集为 `generatedAt` / `range` / `repos` / `ai` / `todos`，无审计字段 |

### 10.5 Good / Base / Bad Cases

| 档 | 场景 |
|---|---|
| Good | 五档各 `Selected` 与直接 `Stats(From=该档起点)` 一致；`week` 档 7 项与顶层逐项相等；切档命中缓存且仓库/待办域不变 |
| Base | 历史数据不足所选范围正常展示，不标「数据不足」；用户对照常显保留上限文案自行判断 |
| Bad | 失败快照被缓存 5 分钟（切档一直拿零值 `Selected`）；前端仍读 `ai.today` 作主数值导致数字与档位脱节；新增 365 天伪档位 |

### 10.6 Tests Required（断言点）

| 层 | 断言点 |
|---|---|
| 后端单测 | `overviewRangeStart` 五档边界（跨月 / 跨年 / 周日 / 未知回落）；五档 `Selected` ↔ `Stats` 一致；`week` 档 7 项相等；缓存命中用**状态变更探针**（首次 build 后新增 pin 仓库与待办，切档后 `PinnedTotal` / `PendingCount` 不变）；失败态 `force=false` 自愈；失败真实路径下其余两域不受影响 |
| 前端单测 | 非法档位 `overviewRangeLabel` / `overviewRangeText` 回落「本周」；`ai.error` 降级态渲染错误与重试、不渲染 `$0.00` / `0.0%`；采样 alert 正向文案断言 |
| E2E | 切档后 AI 卡片数值与口径句变化、仓库 / 待办卡片文本不变；`behind` tooltip 回归断言 |
| 浏览器通道 | `GetOverviewSnapshot` 未被隐藏、RPC `ok=true`、快照顶层字段闭集 |

### 10.7 Wrong vs Correct

**缓存失败态**

```go
// Wrong：失败快照进入 5 分钟 TTL，TTL 内切档永远拿零值 Selected
if !force && s.cached != nil && now.Sub(s.cachedAt) < overviewSnapshotTTL { ... }

// Correct：失败态每次重算，数据恢复后切档即自愈
if !force && s.cached != nil && s.cached.AI.Error == "" &&
    now.Sub(s.cachedAt) < overviewSnapshotTTL { ... }
```

**前端档位读法**

```javascript
// Wrong：切到「近 90 天」仍显示今日成本，数字与控件脱节
:value="`$${formatUSD(ai.today?.costUsd)}`"

// Correct：主数值只读所选档位；today/week 仅作 hint 对比基准
:value="`$${formatUSD(selected.cost?.costUsd)}`"
```

## 11. 修订记录

| 日期 | 修订 | 依据 |
|---|---|---|
| 2026-11-25 | 初版：锁定组合 A，5 项指标与排除清单 | T0 阶段 0 决策 |
| 2026-11-26 | §5 回写 T2 四状态计数与同源聚合实现状态 | T2 落地 |
| 2026-11-26 | §2/§3/§4/§6/§7 扩展 M4/M5 为五档时间范围，明确自然日边界、默认本周与无 365 档；§2 M5 改为「所选范围成功率」 | T3 头脑风暴确认（档位方案 A、控件置于 AI 域标题行） |
| 2026-11-26 | §5 标明「两次 Stats」为 T2 时点实现；新增 §10 T3 跨层实现契约（签名 / 字段 / 错误矩阵 / 测试断言点 / Wrong vs Correct），沉淀失败态缓存不复用机理 | T3 收口评审 |
| 2026-11-26 | §6「终端会话数 / 面板活跃度」排除理由改为「T4 门槛取证专用极轻量埋点，仅记录概览面板进入，不上主看板卡片」；**不上主看板结论不变** | T4 头脑风暴（D2 破例授权） |
| 2026-11-27 | §7 增补 T4 行指向 `overview-open-tracking.md`；新增 T4 跨层埋点契约文档 | T4 收口评审 |
