# T3 统一时间范围控件与数据口径标注

- 父任务：`11-24-personal-dashboard`，执行顺序第 4 项（序号 3）
- 阶段 / 优先级：阶段 1 P0 纯聚合 / P0
- 前置：T1 面板骨架（已交付）；与 T2 可并行（已完成）
- 状态：planning → 头脑风暴已收敛（2026-11-26），口径已回写 `docs/spec/dashboard-metrics.md`

## Goal

给「概览」面板的 AI 任务域加时间范围控件，并让每路数据的口径断层在界面上显式可见，
杜绝把近似值 / 采样值 / 截断值误读为精确事实。零新增采集层、零 LLM 调用，只扩展既有聚合。

## 现状核对（2026-11-26 只读确认）

| 事实 | 位置 |
|---|---|
| `GetOverviewSnapshot(range string, force bool)` 已预留 range 入参，仅回显、不参与计算 | `app_overview.go`、`service/overview.go:62` |
| `AIOverview` 固定返回「今日 + 本周」两块 + 本周四状态计数与成功率 | `model/overview.go:49` |
| 前端每次固定传空串 range，无控件 | `OverviewView.vue:86` |
| 范围控件先例：7/30/90 天 + 1 年 + 全部 | `StatsView.vue:7` |
| 已有口径标注：AI 卡片「基于近 90 天 / 最多 2000 条」、成功率分母公式、behind tooltip、5000 条采样 alert | `AiCostCard.vue`、`AiSuccessCard.vue`、`DashboardView.vue:7`、`StatsView.vue:18` |
| 保留策略在写入时清理（超 90 天 / 2000 条直接删旧），磁盘无法判断「是否被截断」 | `service/ai_task_history.go:118` |
| 下钻时间窗仅识别 `today`/`week` | `AiTaskHistoryPanel.vue:351` |
| `GetOverviewSnapshot` 不在 `RPCHiddenMethods`；overview 无审计字段 | `main.go:262` |

## 已确认决策（2026-11-26 头脑风暴，ADR-lite）

| # | 决策 | 关键理由 |
|---|---|---|
| D1 | 档位 = 今日 / 本周 / 近 7 / 30 / 90 天，**无 365 档**；默认本周 | AI 历史保留上限 90 天，365 档恒等于 90 天，属伪档位 |
| D2 | 控件只作用于 AI 域（M4/M5），放「AI 任务」域标题行右侧 | 位置即语义，M1–M3 无需解释文字；符合 spec §4「快照类不加时间窗」 |
| D3 | 主数值 = 所选范围的成本 / 成功率；hint 保留今日/本周作对比；token 与四状态计数随范围 | 卡片名与控件同窗口，避免成本数字与范围脱节 |
| D4 | 口径标注沿用 11px 小字常显，文案随档位联动；不引入 ⓘ popover | 关键断层须在「10 秒读屏」内可见，折叠进悬浮层等于隐藏 |
| D5 | 「近 N 天」= 自然日边界，含今天共 N 个自然日 | 与同控件内「今日 00:00 / 本周一 00:00」同族，内部一致优先于跨面板一致 |
| D6 | 契约纯增量：`AIOverview` 新增 `RangeKey` + `Selected`，`Today`/`Week` 与顶层本周语义字段原样保留 | 零语义变更、零向后兼容风险；后端顶层本周语义字段与 T2 一致，前端默认档主数值按所选范围取 `selected`（week 档为 `ai.selected.cost`），与 T2 读 `ai.today` 今日主数值的差异见 `docs/spec/dashboard-metrics.md` §2 |
| D7 | 一次 build 算全部五档并按档位缓存，切档不重算仓库域 | 仓库域跑 git 子进程是主要成本，切档必须命中缓存 |
| D8 | 不做「已截断」动态感知 | 写入时即删旧，磁盘上无从判断，任何动态提示都是猜测 |

## Requirements

### R1 范围控件（前端）

- `OverviewView.vue` 的「AI 任务」`section` 标题行右侧新增 `el-radio-group`（`size="small"`）：
  今日 / 本周 / 近 7 天 / 近 30 天 / 近 90 天，`value` 分别为 `today`/`week`/`7d`/`30d`/`90d`。
- 默认 `week`；不持久化（不写 settings / localStorage），切面板（`v-show`）保持当前档位。
- 切档调用 `load(false)`：重置 `requestSeq` 守卫，后到的旧响应不得覆盖新档位结果。**不传 force**
  ——后端缓存按档位存好，切档只换返回块。
- 控件与 M1–M3 无任何耦合：切档后仓库/待办卡片数值不变。

### R2 契约扩展（后端）

- `model.AIOverview` 新增：
  - `RangeKey string`：当前档位回显（`today`/`week`/`7d`/`30d`/`90d`）。
  - `Selected AIRangeStats`：所选档位的完整统计。
- 新增 `model.AIRangeStats`：`Cost AICostBlock` + `SuccessRate float64` + `HasData bool` +
  `SuccessCount` / `FailedCount` / `TimeoutCount` / `CanceledCount int`。
- 既有 `Today` / `Week`（`AICostBlock`）与顶层 `SuccessRate` / `HasData` / 四状态计数
  **字段名与「本周」语义均不变**。顶层字段继续由本周统计填充，`Selected` 在 `week` 档必须与之逐项相等。
- `OverviewSnapshot.Range` 继续回显同一档位值（与 `AIOverview.RangeKey` 一致）。
- `normalizeOverviewRange` 扩展为五档白名单，**空值/非法值回落 `week`**（现状回落 `today`，本任务修正）。

### R3 卡片语义（前端）

- `AiCostCard`：主数值 = `ai.selected.cost.costUsd`；`hint` 显示其余基准（选 `week` 显示
  「今日 $x」，选 `today` 显示「本周 $y」，选 7/30/90 天显示「今日 $x · 本周 $y」）；
  token 四项读 `ai.selected.cost`。
- `AiSuccessCard`：成功率、`hasData`、四状态计数全部读 `ai.selected`；「无数据」判定沿用。
- M4 成本卡下钻只带 `{ range: rangeKey }`（全状态成本 / 功能项排行）；M5 成功率卡下钻带
  `{ range: rangeKey, statuses: [...] }`（失败 + 超时或取消清单）。`AiFunctionPanel` 已对非数组
  `statuses` 做防御，无需改动。

### R4 口径标注（前端）

- AI 两张卡片底部常显动态句：「范围：<档位标签> · 历史保留上限：近 90 天 / 最多 2000 条」。
- 成功率卡片保留「<档位> success ÷（success+failed+timeout），已取消不进分母」，
  `<档位>` 随档位变化（今日 / 本周 / 近 N 天）。
- M1–M3 不加任何时间范围说明（控件位置已表意）。
- `behind` 近似值 tooltip（`DashboardView`）与 5000 条采样 alert（`StatsView`）现状已满足 spec §3，
  本任务**只补回归断言，不改 UI**。

### R5 缓存（后端）

- `OverviewService` 缓存结构改为持有：一次快照 + 五档 `map[string]model.AIRangeStats` + `cachedAt`。
- 命中缓存时浅拷贝快照，按入参档位填 `RangeKey` / `Selected` 后返回；不重算任何域。
- 缓存过期或 `force=true` 时重算：仓库/待办域各一次，AI 域五次 `GetAiTaskHistoryStats`（五档各一次）。

### R6 下钻扩展（前端）

- `AiTaskHistoryPanel.applyInitialRange` 支持 `7d`/`30d`/`90d`：
  `from = 今天 00:00 − (N−1) 天`，`to = 0`；`today`/`week` 行为不变。
- `AiFunctionPanel` 的 payload 透传逻辑不变（`range` 原样交给子组件）。

### R7 隐私红线

- `GetOverviewSnapshot` **保持不在** `RPCHiddenMethods`：三个数据源现有面板均已对浏览器开放，
  本任务不新增敏感字段；浏览器模式下 AI 域范围控件照常可用。
- 新增回归断言：概览快照结构不含审计/攻击趋势字段；`main.go` 的 `RPCHiddenMethods` 清单不变。

### R8 错误与边界

- AI 域读取失败时维持 T2 分区降级：卡片显示「加载失败 + 重试」，范围控件保持可点
  （恢复后重试即可），**禁止把失败降级为 0 或 0%**。
- 历史数据不足所选范围时正常展示（不标注「数据不足」），用户可自行对照保留上限文案。
- 非法档位入参（含空串）由后端归一化为 `week`，前端无需防御性分支。

## Acceptance Criteria（可测试）

- [ ] AI 域标题行出现五档控件，默认「本周」；切档后两张 AI 卡片的数值随之变化，仓库/待办卡片不变。
- [ ] 五档窗口起点正确：`today`=当天 00:00、`week`=本周一 00:00、`7d`=今天 00:00−6 天、
      `30d`=−29 天、`90d`=−89 天（单测覆盖，含跨月/跨年与周日边界）。
- [ ] `selected` 在 `week` 档与 T2 既有顶层字段逐项相等；在任意档位与直接调用
      `GetAiTaskHistoryStats(From=该档起点)` 的结果一致。
- [ ] 空串/非法 `range` 回落 `week`，卡片展示与默认档一致。
- [ ] 切档命中缓存：缓存有效期内连续切档不触发仓库域重算。**验收替代方案**：`OverviewService`
      依赖均为具体类型指针（`*DashboardService` / `*TempTodoService`），无接口注入缝，
      在不改生产代码签名、不新增接口的前提下无法注入计数 stub；改用等价强度的「状态变更探针」——
      首次 build 后新增一个 pin 仓库或新增一条待办，切档命中缓存时 `PinnedTotal` / `PendingCount`
      保持首次值即证明未重算（若重算会立刻读到新数据）。断言强度不降低：仍同时保留
      `GeneratedAt` 不变与仓库/待办域值不变两项断言。
- [ ] AI 卡片常显口径句，且「范围：」部分随档位变化；成功率公式文案随档位变化。
- [ ] 概览在浏览器通道可用（`GetOverviewSnapshot` 未被隐藏），快照无审计字段。
- [ ] `behind` tooltip 与采样 alert 的回归断言存在（不改 UI）。
- [ ] `go vet ./...`、`go test ./...`、`npm test`、`npm run build` 通过；
      覆盖率 service ≥76%、前端 ≥70% 不下降。

## Definition of Done

- 后端：`model/overview.go` 契约扩展；`service/overview.go` 五档计算 + 缓存改造 + 单测
  （五档边界、Selected 一致性、缓存命中不重算、非法档回落、分区降级）。
- 前端：`OverviewView.vue` 控件 + 传参；`AiCostCard` / `AiSuccessCard` 改读 `selected` + 动态文案；
  `AiTaskHistoryPanel` 档位换算；对应 spec（单测）。
- E2E：切档数值变化、口径标注可见、M1–M3 不随档位变化。
- 契约同步：`frontend/wailsjs/go/models.ts` 手补 `AIRangeStats` 与 `AIOverview` 新字段
  （`App.js` / `App.d.ts` 签名不变，不改）。
- 文档：`docs/spec/dashboard-metrics.md` 已回写（§2/§3/§4/§6/§7/§10）；父任务 PRD 第 2 节 T3 完成标志同步。
- 未新增采集层、未新增 LLM 调用、无新错误码、`data/` 无新增直写。

## Technical Approach

### 后端

| 层 | 文件 | 改动 |
|---|---|---|
| model | `model/overview.go` | 新增 `AIRangeStats`；`AIOverview` 加 `RangeKey` / `Selected`（其余字段不动） |
| service | `service/overview.go` | `normalizeOverviewRange` 五档白名单；新增 `overviewRangeStart(key, now) time.Time` 单点；`buildAI` 计算五档；缓存结构加 `cachedRanges map[string]model.AIRangeStats`，命中时浅拷贝 + 填 `RangeKey`/`Selected` |
| app | `app_overview.go` | 签名不变 |

关键实现约束：

- `overviewRangeStart` 为唯一窗口起点换算点，`buildAI` 与单测都走它；
  禁止在卡片或前端重算日期。
- 五档 `Stats` 调用共用同一 `now`（一次 build 内不重复取时钟），避免跨秒漂移。
- 缓存写回仍在锁内，与现有 `mu` / `cachedAt` 语义一致；不得持锁做 git 子进程。

### 前端

| 层 | 文件 | 改动 |
|---|---|---|
| 视图 | `views/OverviewView.vue` | `rangeKey` ref（默认 `'week'`）；AI 域标题行 `el-radio-group`；`load()` 传 `rangeKey`；切档清 `requestSeq` 语义沿用 |
| 卡片 | `components/overview/AiCostCard.vue` | 读 `ai.selected`；hint 对比基准；动态范围文案 |
| 卡片 | `components/overview/AiSuccessCard.vue` | 读 `ai.selected`；公式文案随档位；下钻带 `rangeKey` |
| 抽屉 | `components/AiTaskHistoryPanel.vue` | `applyInitialRange` 支持 `7d`/`30d`/`90d` |
| 契约 | `frontend/wailsjs/go/models.ts` | 手补新字段与新 struct |

## 实施计划（小步）

- **PR1 后端契约与聚合**：`model/overview.go` + `normalizeOverviewRange` / `overviewRangeStart` +
  `buildAI` 五档 + 缓存改造 + 单测 + `wailsjs/models.ts` 同步。
- **PR2 前端控件与卡片**：`OverviewView` 控件、两张 AI 卡片改读 `selected` + 动态文案 + 单测。
- **PR3 下钻与收尾**：`AiTaskHistoryPanel` 档位换算 + E2E + 文档回写与覆盖率核对。

## 风险

| 风险 | 缓解 |
|---|---|
| 五档 Stats 使每次重算的 `loadAll` 从 2 次增到 5 次 | 单次 ≤2000 条内存聚合，5 分钟 TTL；PR 中记录实测耗时，超预期则改为按档懒算 + 同 TTL 缓存 |
| `Selected` 与顶层字段在 `week` 档出现漂移 | 单测硬断言逐项相等；前端只读 `selected`，不混用两套 |
| 前端漏改、仍读顶层本周字段 | 卡片全部改为 `ai.selected`，`OverviewView.spec` 断言渲染值随档位变化 |
| 档位文案与 spec 不一致 | 文案常量集中在卡片内，spec §4 表格为真相源，PR 对照检查 |

## Out of Scope

- 365 天档（乃至更长）与自定义日期区间。
- 把时间范围扩到 M1–M3（快照/全量语义不变）。
- 「数据被截断」的动态感知与提示。
- 数据补全、回填、跨档位对比图。
- 范围选择的持久化与跨会话记忆。

## Technical Notes

- 现有关键文件：`service/overview.go`（TTL 常量、`buildAI`、`normalizeOverviewRange`）、
  `model/overview.go`、`OverviewView.vue`、`AiCostCard.vue`、`AiSuccessCard.vue`、
  `AiTaskHistoryPanel.vue:351`（`applyInitialRange`）。
- 跨层契约：`frontend/wailsjs/go/models.ts` 必须手补，`npm run build` 缺 `MISSING_EXPORT` 会报错。
- 缓存语义：切档不传 `force`，靠后端五档缓存；`force=true` 仅来自「刷新」按钮与卡片重试。
- 与 T2 的边界：T2 已把 `buildAI` 收敛到 `Stats` 同源聚合，T3 不改聚合公式，只扩档位与返回结构。
- E2E 断言方式：切档后断言 AI 卡片文本变化 + 仓库卡片文本不变；口径句用 `data-testid` 或文本匹配。
