# T4 按 Go/No-Go 门槛验证 P0 使用闭环

- 父任务：`11-24-personal-dashboard`，执行顺序第 5 项（序号 4）
- 阶段 / 优先级：阶段 2 验证门槛 / P0
- 前置：T2、T3（均已归档）；**本项解锁或终止阶段 3/4 全部任务**
- 状态：`in_progress`（2026-11-26 头脑风暴收敛完毕并经用户确认）
- 判定日：**2026-12-14（周一）09:07**（一次性 timer 已登记，ID `27550c82`）
- 取证明细与 Q&A 轨迹见 [`research/context-findings.md`](research/context-findings.md)

## 1. 目标

用真实使用数据验证「看板是否真的改变了行为」，对抗个人自用场景最高频失败模式「看板变墙纸」，并据此显式解锁或终止 T5–T11。本轮交付判定协议与取证能力，**结论留到判定日产出**。

## 2. 判定标准（父任务已锁定，不因投入成本放宽）

| 编号 | 门槛 | 原始表述 | 判定口径 |
|---|---|---|---|
| G1 | 每周主动打开次数 ≥2 | 「每周主动打开次数 ≥2」 | W1 与 W2 **各自** ≥2；仅 3 个显式入口计入（D4/D6） |
| G2 | 至少一次因看板改变了当天动作 | 「至少一次因看板改变了当天动作」 | `observation.md` 中至少 1 条当天记录的四要素案例（D5） |

两条同时满足为 **Go**，解锁 T5–T11；否则为 **No-Go**，停止后续投入，退回周报导出脚本或直接砍掉看板。
禁用「页面好看」「数据齐全」等不可验收描述代替判定；证据不足一律 No-Go（D7）。

## 3. 已确认决策（D1–D9）

| # | 决策 | 关键理由 |
|---|---|---|
| D1 | **单任务两段式**：本轮钉死判定协议 + 取证埋点 + 观察记录载体 + 判定日提醒，T4 保持 `in_progress` 挂到判定日；不拆子任务、不做立即预判定 | 协议先于数据锁定，防止两周后自证式放宽门槛；与「阶段 2 硬准入门槛」语义一致 |
| D2 | **极轻量埋点**自动取数 G1（非人工自记、非代理指标事后回看） | 不漏记、判定日可核对；人工自记自证偏差高；审计日志 / `session.json` 已确认无法事后还原 |
| D3 | **Go 后端独立 append-only JSONL + 新增 `App.RecordOverviewOpen(source)`**，前端在显式入口上报；不用纯前端 localStorage、不在 `GetOverviewSnapshot` 隐式追加 | localStorage 在桌面 WebView2 与浏览器模式下不共享会数据分裂、清缓存即丢；隐式追加无法鉴别「进入面板 / 切档 / 手刷 / 卡片重试 / TTL 命中」 |
| D4 | **仅 3 个显式入口算主动**（点击图标 / 快捷键 / `Alt+数字`），会话恢复不计、切档/手刷/TTL 命中不计、下钻切走不计、下钻后再次显式点回计一次；两周判定 = **每周各 ≥2**（W1 ≥2 且 W2 ≥2），不采用平均口径 | 父任务原文为「**每周**主动打开次数 ≥2」；平均口径会放过「第二周 0 次」这一典型变墙纸信号；计入会话恢复会让每次重启自动 +1 |
| D5 | **T4 任务目录新建 `observation.md`**：上半部 G1 两段周汇总表、下半部 G2 案例表；G2 四要素 = 日期时间 + 看到的卡片/数值 + 原本打算做什么 + 实际改成做什么，并关联同日埋点打开时间；**当天记录、判定日禁补写**；判定人 = 用户本人 | G2 天然主观，只能靠「当天记录 + 可交叉核对 + 禁事后补写」压住自证偏差；不改概览 UI，避免测量工具诱导打开污染 G1 |
| D6 | **日历周口径**：适应期 2026-11-26~11-29（**不计入判定**）；观察期起点 **2026-11-30（周一）**；W1 = `11-30 ~ 12-06`、W2 = `12-07 ~ 12-13`；判定日 **2026-12-14 09:07**；按日历日期归段，与 T3 `week` 档同源；不设中途检查提醒 | 周界与仓内既有 weekly 语义一致、按日期归段最省事；中途提醒会诱导打开概览、污染 G1。代价是 11-26~29 四天排除在外，如实接受不补算 |
| D7 | **判定日证据不足一律判 No-Go**：埋点文件缺失/损坏/被清空、日志行数不足以归段、`observation.md` 无当天记录却在判定日补写、次数明显异常且无对应行为案例——均按未达标处理，**不补采、不延期、不放宽** | 由父任务 §3「不因已投入成本而放宽门槛」+ 本文件 §2 推导；举证责任在主张 Go 的一方 |
| D8 | **No-Go = 纯文档级终止**：父 PRD §2 顺序表 5–11 行标「已终止（T4=No-Go，2026-12-14）」；8 个子任务 `prd.md` 顶部加终止横幅（含依据与「不得 start」）；T4 结论文件 + journal 记账 + 父 PRD「当前状态」回写。**不碰 `task.json`、不归档、不改状态机**。判 Go 对称动作 = 父 PRD 标注解锁 + 回写「当前状态」，任务保持 `planning` 不动 | 父任务只要求「停止后续投入」，文档级终止已显式、可核对、可追溯；archive 会把终止写成 `completed` 语义失真；新增 `cancelled` 状态值兼容性未验证 |
| D9 | **判定日直接只读** `data/overview_opens.log` 按日期归两段聚合，**不新增 App 查看/导出方法、不新增 RPC 出口、不加 UI**；证据与结论落在 T4 任务目录的 `observation.md` + `decision.md`，父 PRD「当前状态」回写、journal 记账；运行期日志本身**不入库** | 一年仅用一次的判定，为读 14 天日志造 API 属过度工程，且会把「我打开过几次概览」暴露到浏览器通道；符合全局编码原则「简洁优先」 |

### 3.1 D2 强制实施前置（不可跳过，禁止隐式开口）

D2 突破阶段 1「零采集层」硬约束，实施前必须先完成三处回写：

1. 回写 `docs/spec/dashboard-metrics.md` §6：把「面板活跃度」排除理由由「待 T5 落地后再评估」改为「T4 门槛取证专用极轻量埋点，仅记录概览面板进入，不上主看板卡片」；**保留「不上主看板」结论不变**；§11 修订记录补一行。
2. 回写父任务 `.trellis/tasks/11-24-personal-dashboard/prd.md` §5：登记「阶段 2 为门槛取证破例引入最小采集，范围仅限概览面板进入记录，不扩到其他面板/指标」；§7「当前状态」登记 T4 已激活与判定日。
3. 埋点须与 T5 同源设计：判 **Go** 后 T5 直接复用本日志器扩展，**不得另起一套计数定义**。

## 4. 技术约束（C1–C13）

| # | 约束 |
|---|---|
| C1 | 新建 `service/overview_open_log.go` 独立日志器，O_APPEND 单行 JSONL，路径 `data/overview_opens.log`；**不复用 `AuditLogger` / `AuditRecord` 闭集**（会污染 `QueryAuditLog` 读侧） |
| C2 | 新增 `App.RecordOverviewOpen(source string)`：`App.js` / `App.d.ts` 两处手同步；**无新增 `model/` 导出 struct，`models.ts` 不变**（T3 先例） |
| C3 | 新增 service 须 `app_services.go` 两处装配（字段 + 构造行），构造纯 `new` 无生命周期副作用 |
| C4 | `RecordOverviewOpen` **不加入** `main.go` 的 `RPCHiddenMethods`：浏览器模式下用户同样能打开概览面板，不隐藏才可避免双通道数据分裂；方法只写一条无敏感字段记录、不返回明细，不触碰「他人 IP/token 明细只走桌面出口」红线。**实施时复核，发现冲突只报告不擅自改决策** |
| C5 | `uiStore.overviewEntrySource` 由三个显式入口写入（`click` / `shortcut` / `alt`），`OverviewView` watch 命中 overview 时读取并**消费即清空**；恢复路径不写源 → 不上报。与 `overviewDrillRequest` 的「seq 递增 + 消费即清」同族（`store/ui.js:90-100`） |
| C6 | 写码前须读用户全局 `CLAUDE-编码原则.md` 与本仓 `docs/agent-resources/conventions/`；C5 的共享标志是满足「精准修改」的最小改动，不改 `ActivityBar.vue` 通用赋值语义、不改 `markRestored` |
| C7 | 日志行须带本地日期时间戳；判定日按 `11-30~12-06` / `12-07~12-13` 两段按日历日期聚合，分别核对 ≥2；适应期 `11-26~11-29` 记录保留但排除在外 |
| C8 | `observation.md` 落在 T4 任务目录，含 G1 两段周汇总表与 G2 案例表；**观察期开始前创建并写好表头与填写规则**，观察期内只追加行、不改规则；表头写明适应期/观察期/判定日 |
| C9 | 判定日一次性提醒用 `cc-connect timer add --at "2026-12-14T09:07"`，**不用 cron**；已登记，勿重复 |
| C10 | 判定日结论须逐条列出 G1 两段次数、G2 案例（或「无」）、异常项，再给结论；证据不足不补采 |
| C11 | `.gitignore` 增补 `data/overview_opens.log`（与 `data/web_audit.log` 同列），避免运行期日志被误提交 |
| C12 | 不新增埋点日志的 App 查看/导出方法、不新增 RPC 出口、不加 UI；判定任务直接只读本地 JSONL |
| **C13** | **埋点须在 2026-11-30 00:00 前部署并验证通过**；观察期 `11-30~12-13` 内不改动概览面板功能与埋点逻辑（A2）。逾期未部署 → W1 无数据 → 按 D7 直接判 No-Go |

## 5. 已确认前提与边界

| # | 结论 |
|---|---|
| P1 | T4 产出是「结论 + 依据记录 + 解锁/终止动作」，不是看板新功能；判定人是用户本人（单用户自用场景） |
| P2 | 父任务与 12 个子任务当前均为 `planning`；应用同时支持桌面 Wails 与浏览器访问模式 |
| P3 | 「P0 上线」指 T1–T3 代码合入 `master`；观察期起点 2026-11-30、判定日 2026-12-14；11-26~29 为适应期不计入 |
| A2 | 观察期 `11-30~12-13` 内不改动概览面板功能与埋点逻辑，避免「边改边观察」污染结论（C13） |
| A3 | 判定证据与结论落在 T4 任务目录（`observation.md` + `decision.md`）、父 PRD「当前状态」回写、journal 记账；运行期日志不入库 |

## 6. Open Questions（全部清零）

Q1 交付形态 → D1｜Q2 G1 取证 → D2｜Q3 埋点落层 → D3｜Q4 计数口径 → D4｜Q5 G2 载体 → D5｜Q6 周界与判定日 → D6｜证据异常 → D7（推导）｜Q7 No-Go 终止动作 → D8｜Q8/Q9 读日志与落盘 → D9。用户 2026-11-26 回复「确认」。

## 7. Requirements（R1–R17）

| # | 要求 |
|---|---|
| R1 | 产出明确的 Go / No-Go 结论及依据记录（打开次数、可复述的行为改变案例） |
| R2 | 据此显式解锁或终止 T5–T11 |
| R3 | 判定标准逐条可验收，禁用主观观感替代；不因已投入成本放宽门槛 |
| R4 | 本轮不得隐式突破「阶段 1 零采集层」；D2 已授权破例，须按 §3.1 三处回写后落地 |
| R5 | 本轮交付「判定协议 + 取证埋点 + 观察记录载体 + 判定日提醒」，结论留到 2026-12-14；不拆子任务、不做立即预判定 |
| R6 | 埋点仅记录「概览面板进入」，不上主看板卡片、不扩到其他面板、不引入 LLM、不写敏感字段 |
| R7 | 埋点须能鉴别被动进入，不得把会话恢复、切档刷新误计为主动打开 |
| R8 | 埋点数据须支持判定日按周聚合核对，且重启不丢 |
| R9 | 埋点在桌面与浏览器双模式下落同一份 `data/overview_opens.log`，口径一致 |
| R10 | G1 判定按 W1/W2 两段日历周分别聚合，每段各 ≥2 才达标；适应期记录排除 |
| R11 | `observation.md` 在观察期开始前建好，含 G2 四要素表头与「当天记录、判定日禁补写」规则；观察期内只追加行 |
| R12 | 判定日一次性提醒登记在 2026-12-14 09:07，无中途提醒；`cc-connect timer`，不用 cron |
| R13 | 证据不足不补采、不延期，直接判 No-Go |
| R14 | No-Go 只做文档级终止，不改 `task.json`、不归档；Go 只做父 PRD 解锁标注 |
| R15 | `.gitignore` 补 `data/overview_opens.log` |
| R16 | 判定日直接只读本地日志；证据结论落 `observation.md` + `decision.md`（T4 目录），父 PRD 回写，journal 记账；不新增 App 查看/导出出口 |
| R17 | 埋点在 2026-11-30 00:00 前部署并验证；观察期内冻结概览面板功能与埋点逻辑 |

## 8. Acceptance Criteria（AC1–AC13）

| # | 验收项 |
|---|---|
| AC1 | 判定协议明确 G1/G2 取证方式、观察期起止（含适应期）、判定人、判定日 |
| AC2 | 埋点仅由 3 个显式入口触发；会话恢复、切档、手刷、TTL 命中、下钻切走均不写记录（单测 + 手工核对） |
| AC3 | 埋点重启后记录仍在；桌面与浏览器模式汇入同一文件；适应期记录可被排除 |
| AC4 | 判定日可按 W1/W2 两段各聚合一次核对每周次数 |
| AC5 | G2 案例四要素齐全、当天记录、可与同日埋点时间对应 |
| AC6 | 判定日产出 Go / No-Go 结论，结论与证据逐条对应；证据不足时结论为 No-Go |
| AC7 | 结论落地后 T5–T11 被显式解锁或显式终止，父 PRD「当前状态」同步回写 |
| AC8 | 未出现「页面好看 / 数据齐全」类不可验收表述，未放宽门槛 |
| AC9 | 本轮协议与 `observation.md` 表头在 2026-11-26 落盘；12-14 09:07 一次性提醒已登记；T4 保持未完成状态直到结论产出 |
| AC10 | §3.1 三处回写完成，且 `docs/spec/dashboard-metrics.md` §6「不上主看板」结论未被推翻 |
| AC11 | `data/overview_opens.log` 已被 `.gitignore` 忽略，运行期不出现在 `git status` |
| AC12 | 代码门禁：`go vet ./...`、`go test ./...`、`npm test`、`npm run build` 通过，覆盖率 service ≥76% / 前端 ≥70% 不下降 |
| AC13 | 埋点在 2026-11-30 00:00 前部署完成并自测通过（C13） |

## 9. Definition of Done

| # | 项目 |
|---|---|
| 1 | 代码门禁与覆盖率不下降（AC12）；跨层契约同步 `frontend/wailsjs/` 三处（本次 `models.ts` 不变） |
| 2 | 文档：判定协议、`observation.md` 表头、三处 spec/父 PRD 回写；功能完成后确认是否需要更新 `README.md`（AGENTS.md 文档语言约定） |
| 3 | 回滚/终止路径已考虑：No-Go 时后续任务处置明确（D8） |
| 4 | 本轮为协议交付，不改 M1–M5 口径、不做看板 UI 改动（A2） |
| 5 | 不主动 commit / push；提交前 `git fetch origin` + `git rebase origin/master`；禁 merge 提交 |

## 10. Out of Scope

1. 不新增看板指标、不改 M1–M5 口径、不新增时间档位
2. 不提前实现 T5 的完整活动日志 service（仅做门槛取证所需最小埋点）
3. 不引入 LLM 主链路、不做外部数据聚合
4. 不在观察期内新增「提醒/告警/自然语言问数」等影响使用行为的功能
5. 不做立即预判定（D1）
6. 不把面板活跃度做成上板卡片（§3.1 前置 1）
7. 不设观察期中途检查提醒（D6，避免诱导打开污染 G1）
8. 不新增埋点日志的 App 查看/导出方法、不新增 RPC 出口、不加 UI（D9/C12）
9. 不物理删除/归档/改状态机来「终止」后续任务（D8）
10. 运行期日志不提交入库（C11/D9）

## 11. Technical Approach

| 层 | 文件 | 改动 |
|---|---|---|
| service | `service/overview_open_log.go`（新增） | 独立 O_APPEND 单行 JSONL 日志器；`Record(source)` 写 `{ts, date, source}`；失败仅 `slog.Warn` 降级不阻断 UI；`ReadAll()` 供判定任务只读 |
| app | `app_overview.go` | 新增 `RecordOverviewOpen(source string)`，委托 service；`source` 白名单校验，非法值不落盘；恒返回 nil error |
| 装配 | `app_services.go` | `AppServices` 字段 + `NewAppServices` 构造行（两处，构造纯 `new`） |
| 生成产物 | `frontend/wailsjs/go/main/App.js` / `App.d.ts` | 手同步新方法两处；`models.ts` 不变 |
| store | `frontend/src/store/ui.js` | `overviewEntrySource` ref + setter，消费即清（对齐 `overviewDrillRequest` 范式） |
| 入口 | `ActivityBar.vue:15`、`Home.vue:604`、`Home.vue:612-620` | 三处写入 `overviewEntrySource`（`click` / `shortcut` / `alt`）；不改通用赋值语义 |
| 消费 | `OverviewView.vue:133-139` | watch 命中 overview 时读 `overviewEntrySource`，非空则调 `RecordOverviewOpen` 后清空；恢复路径无源故不上报；失败静默降级不阻断 `load(false)` |
| 忽略 | `.gitignore` | 补 `data/overview_opens.log` |
| 文档 | `docs/spec/dashboard-metrics.md` §6、父 PRD §5、T4 `observation.md` | §3.1 三处回写 + 观察记录载体 |

**判定日读取链路**：判定任务直接只读 `data/overview_opens.log` → 按 `date` 排除适应期、归 W1/W2 两段 → 读 `observation.md` 核对 G2 → 写 `decision.md`（结论 + 依据 + 异常项）→ 执行 D8 解锁/终止 → 回写父 PRD → journal 记账。全程无新 RPC 出口。

## 12. 实施计划

| 批次 | 内容 | 验证 |
|---|---|---|
| 批 1 文档 + 后端 | `observation.md` 表头；`docs/spec/dashboard-metrics.md` §6 回写；父 PRD §5/§7 回写；`.gitignore`；`service/overview_open_log.go` + 单测；`app_overview.go` 新方法；`app_services.go` 两处装配；`wailsjs` 两处同步 | 文档一致性、`go vet ./...`、`go test ./...`、service 覆盖率 ≥76% |
| 批 2 前端 + 全量门禁 | `uiStore.overviewEntrySource` + 三入口写入 + `OverviewView` watch 消费 + 前端单测/E2E + `wails-mock-defaults.js` 补 mock | `npm test`、`npm run build`、前端覆盖率 ≥70%、AC2/AC3/AC13 |

**构建前置链**（AGENTS.md §2）：`wails generate module` 会覆盖手改的 `App.js`/`App.d.ts`，须**先 generate 再手同步**，或 generate 后复核两处新方法仍在；`frontend/dist/assets/app.js` 需 placeholder，否则 `//go:embed all:frontend/dist` 报错。

**已知测试坑**：`OverviewView.spec.js` 是文件级 `vi.mock`，仅声明 `GetOverviewSnapshot`；新增 `RecordOverviewOpen` import 必须同步扩展该 mock，否则既有用例挂。

## 13. Decision (ADR-lite)

| # | 决策 | 关键理由 |
|---|---|---|
| 1 | 单任务两段式 | 协议先于数据锁定，防止两周后自证放宽 |
| 2 | G1 走极轻量埋点 | 自动不漏记、判定日可核对；审计日志与会话快照无法事后还原 |
| 3 | Go 后端独立 append-only JSONL + `App.RecordOverviewOpen(source)` | 双通道统一落盘、重启不丢、口径可鉴别 |
| 4 | 仅 3 个显式入口算主动；两周判定 = 每周各 ≥2 | 贴合「每周」原文；平均口径放过第二周 0 次 |
| 5 | G2 走 `observation.md`，四要素 + 当天记录 + 判定日禁补写 | 主观判断用可交叉核对与禁补写压住自证偏差；不改概览 UI |
| 6 | 日历周口径：适应期不计、观察期 11-30~12-13、判定日 12-14 09:07 | 与 T3 `week` 档同源；中途提醒会污染 G1 |
| 7 | 判定日证据不足一律 No-Go，不补采不延期 | 举证责任在主张 Go 方 |
| 8 | No-Go = 纯文档级终止，不碰 task.json / 不归档 | archive 会把终止写成 completed 语义失真 |
| 9 | 判定日直接只读本地日志；证据结论落 T4 目录 + 父 PRD + journal | 一年一次的判定不为读 14 天日志造 API |

## 14. 修订记录

| 日期 | 修订 | 依据 |
|---|---|---|
| 2026-11-24 | 初版：登记父任务顺序表与两条判定标准 | 父任务创建 |
| 2026-11-26 | 头脑风暴 8 轮收敛：D1–D9、C1–C13、R1–R17、AC1–AC13；用户确认 | Q1–Q9 逐问确认 |
| 2026-11-26 | 取证明细 F1–F22、发散考虑、Technical Notes 下沉 `research/context-findings.md`；本文件精简为决策真相源 | 首轮实施子代理因注入 spec 全文超长触发 API 400，收敛上下文 |
