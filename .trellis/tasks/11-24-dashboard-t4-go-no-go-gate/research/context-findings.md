# T4 头脑风暴取证明细（上下文下沉）

> 本文件承接 `prd.md` 精简时下沉的取证记录与过程性说明，**不改变任何已确认决策**。
> 决策真相源仍是 `prd.md` 的 D1–D9 / C1–C13 / R1–R17 / AC1–AC13。
> 取证时间：2026-11-26，仓内只读扫描。

## 1. 现状核对 F1–F22

| # | 事实 | 依据 |
|---|---|---|
| F1 | T1/T2/T3 全部在 2026-11-25 ~ 11-26 交付并归档，P0 代码当天才齐 | `git log`（`3b5c1c9f` / `df928ff9` / `7663ffe3`）、`journal-5.md` Session 266–268 |
| F2 | 门槛要求「上线两周后」判定；若以 11-26 为起点最早判定日为 2026-12-10（该推定已被 D6 覆盖，实际判定日 12-14） | 父任务 PRD §3、T4 PRD §2、D6 |
| F3 | 阶段 1 硬约束「零新增采集层、零 LLM 调用」 | 父任务 PRD §5、T1 PRD R2、T3 PRD Goal |
| F4 | `docs/spec/dashboard-metrics.md` §6 把「终端会话数 / 面板活跃度」列入排除清单，理由「需新增采集层（Q2=No），待 T5 落地后再评估」 | `docs/spec/dashboard-metrics.md:111` |
| F5 | T5「轻量活动日志 service」的前置是 4=Go，即采集层排在门槛之后，形成依赖环 | 父任务 PRD §2 表格序号 5 |
| F6 | `useRecentAccess` 只记录仓库路径 + `lastAccess` 时间戳，上限 50 条，存 localStorage；不记录面板打开 | `frontend/src/composables/useRecentAccess.js` |
| F7 | `data/session.json` 只恢复 `activePanel` 单值，无时间序列、无历史打开记录；`startSessionAutoSave` 为覆盖式 debounce 写盘 | `frontend/src/composables/useSessionState.js` |
| F8 | `service/audit_log.go` 只落认证 / token 生命周期 / 连接管控九类事件，`server/rpc.go` 无方法调用日志；RPC 调用次数不可从中还原 | `service/audit_log.go:18-30`、`server/rpc.go` |
| F9 | `OverviewView.vue` 的 `watch(activePanel === 'overview') → load(false)` 是天然的「进入面板」信号点，但当前不落任何记录 | `frontend/src/views/OverviewView.vue:133-139` |
| F10 | 当前没有任何可自动统计 G1 的数据源；G2 属主观行为改变，天然无法自动采集 | F3–F9 推论 |
| F11 | 该 watch 带 `{ immediate: true }`，且 `OverviewView` 在 `Home.vue:104` 为 `v-show` 常驻挂载（启动即 mounted）；会话快照恢复时 `applySessionState` 写回 `activePanel='overview'` 会触发一次 watch，属被动恢复，必须排除 | `OverviewView.vue:138`、`Home.vue:104,825-842`、`useSessionState.js:77-79` |
| F12 | 口径更正：切档走 `el-radio-group` 的 `@change="load(false)"`，不经过该 watch，不会误计；下钻后点回概览会再次把 `activePanel` 置回 `overview` 并触发 watch | `OverviewView.vue:28,117-129`、`Home.vue:604`、`ActivityBar.vue:15` |
| F13 | `AuditLogger.Log` 为通用 O_APPEND 追加写，但 `AuditRecord` 字段是认证/连接管控专用闭集；塞入面板事件会污染 `QueryAuditLog` 读侧语义，应另立独立日志器 | `service/audit_log.go:63-90,131,277`、`data-atomic-write.md` append-only 豁免口径 |
| F14 | 全仓仅 3 个业务写入口把 `activePanel` 置为 `overview`：`ActivityBar.vue:15`（点击，通用赋值）、`Home.vue:604`（`openOverview` 快捷键）、`Home.vue:612-620`（`Alt+数字`）；另有 `useSessionState.js:78`（会话恢复写回，被动）。watch 只能看到最终值，无法区分入口 | 全仓 grep `activePanel.*overview` / `'overview'` |
| F15 | `startSessionAutoSave` 的 `restored` 是 `useSessionState.js` 闭包内的局部标志，不对外暴露；排除恢复期触发须新增共享标志，不改 `markRestored` 语义 | `useSessionState.js:151-220` |
| F16 | `uiStore.activePanel` 默认 `'directory'`，故 `immediate: true` 的首次执行不命中 overview；误计风险只来自恢复写回 | `store/ui.js:15` |
| F17 | 仓内无观察记录 / 回顾类载体先例；`TempTodo` 已完成条目保留 30 天且最多 500 条（`List()` 读取时清理）；`data/` 下 10 个 JSON 均为业务数据，无行为记录 | glob `.trellis/tasks/**/*observation*` / `*review*`、`docs/spec/temp-todo.md:16,21`、`data/*.json` |
| F18 | 本机时间基准：2026-11-26 19:26 +0800（星期四）；`cc-connect timer add --at "YYYY-MM-DDTHH:MM"` 支持一次性绝对时间提醒（本地时区），cron 无一次性语义；当前 `cc-connect timer list` 无待触发任务 | `date`、`cc-connect timer add --help`、`cc-connect timer list` |
| F19 | Trellis 状态机无 terminated/cancelled：全仓 `task.json` 的 `status` 只有 `planning` / `in_progress` / `completed` 三态；`task.py archive --no-commit` 会写成 `completed` 并移入 `archive/`，「终止」与「完成」语义混淆 | `task.py archive --help`、`task.py list --status`、全仓 `task.json` grep |
| F20 | 项目长期记忆：任务归档默认走 `/trellis:finish-work`（含脏路径分类守门 + journal 记账），禁裸跑 `task.py archive` | `docs/memory/trellis-finish-work-entry.md` |
| F21 | `.gitignore` 未忽略 `data/overview_opens.log`：现有 `data/*.json` 与 `data/web_audit.log` 被忽略，但新 `.log` 路径不在忽略清单，有误提交风险 | `git check-ignore -v`、`.gitignore:31,54` |
| F22 | `data-atomic-write.md` §边界：手改 `data/*.json` 前须关闭 `workbench.exe`（整体覆盖 last-writer-wins）；append-only 日志读取侧为「打开-读-关闭」，不持写锁，判定日只读文件无此约束 | `docs/spec/data-atomic-write.md:50-52`、`service/audit_log.go:348` |

**核心矛盾**：判定日未到（F1/F2），且门槛 ① 所需证据在「零采集层」下不存在（F3–F10）；而采集层任务 T5 又被本门槛阻塞（F5）。今天既不能出结论，也不能自动取数。

## 2. 发散考虑（已全部收敛）

| 类别 | 考虑点 | 收敛结果 |
|---|---|---|
| 未来演化 | 判 Go 后 T5 直接复用本轮埋点；判 No-Go 时协议须支撑「退回周报导出脚本」的取舍说明 | PRD §4.1 前置 3、D8 |
| 相邻场景 | 与 T1 ADR 决策 1「T4 门槛需要『主动打开』可测」呼应；与父任务 §4「Not Now」清单、「阶段 2 是硬准入门槛」一致 | D4/D8 |
| 失败与边界 | 会话恢复误计、下钻返回定性、为凑次数刻意打开、观察期重装/清缓存丢数据、中途改看板功能、判定日无数据、No-Go 终止动作 | D4/D7/A2/D8 |
| 取证能力 | 桌面 WebView2 与浏览器模式 localStorage 不共享；`GetOverviewSnapshot` 调用场景多于「进入面板」 | D3 排除 |
| 安全面 | 浏览器通道可写 `data/overview_opens.log`；持有 token 者可伪造一条记录，单用户自用威胁低但判定日须核对 | C4/D7 |
| 载体缺口 | 仓内无观察记录先例；`TempTodo` 有 30 天/500 条裁剪、`data/` 无行为记录 | D5 解决 |
| 时间基准 | 周界定义直接决定两段聚合边界与判定日 | D6 日历周 |
| 框架缺口 | Trellis 无 terminated 状态，archive 会写成 completed | D8 绕开 |
| 版本控制 | 新 `.log` 未被 `.gitignore` 忽略；判定日只读不写不触发 `data/*.json` 手改约束 | C11/F22 |

## 3. Q&A 收敛轨迹

| 轮次 | 问题 | 用户答复 | 决策 |
|---|---|---|---|
| Q1 | 本次「开工 T4」交付哪种形态？ | A | D1 单任务两段式 |
| Q2 | G1 如何取证？ | 2 | D2 极轻量埋点 |
| Q3 | 埋点落在哪一层？ | A | D3 Go 后端 append-only JSONL |
| Q4 | G1 计数口径？ | 1 | D4 仅显式入口 + 每周各 ≥2 |
| Q5 | G2 记录载体与要素？ | A | D5 `observation.md` 四要素 |
| Q6 | 观察期起点与周界？ | 2 | D6 日历周 / 12-14 判定 |
| — | 证据异常处置（推导） | — | D7 一律 No-Go |
| Q7 | No-Go 如何终止后续任务？ | 1 | D8 纯文档级终止 |
| Q8/Q9 | 判定日如何读日志、证据落哪？ | A | D9 直接只读 + T4 目录 |

最终用户确认：2026-11-26「确认」。

## 4. Technical Notes（实现参考）

- 关键依据：`docs/spec/dashboard-metrics.md`（§6 排除清单、§8 变更规则）、父任务 PRD（§2 顺序表、§3 门槛、§5 硬约束）、T1/T3 归档 PRD（零采集层约束）、`docs/memory/trellis-finish-work-entry.md`、`docs/spec/data-atomic-write.md`。
- 现有可复用机制：`service/audit_log.go:277` 的 O_APPEND 单行原子追加 + 轮转链、`util.SaveJSON`（整体覆盖型快照）、`store/ui.js:90-100` 的 `overviewDrillRequest` 信箱范式、`useSessionState.js`（`activePanel` 恢复）、`useRecentAccess.js`（仅导航辅助）。
- 三个显式入口：`ActivityBar.vue:15`（点击）、`Home.vue:604`（`openOverview` 快捷键）、`Home.vue:612-620`（`Alt+数字`）；被动入口 `useSessionState.js:78`（恢复）。
- 工程硬约束：`data/` 写入走 `util.SaveJSON`（append-only 日志走既有豁免）、日志用 `log/slog`、`RPCHiddenMethods` 隐私面、覆盖率 service ≥76% / 前端 ≥70%、新增 service 须 `app_services.go` 两处装配、新增 App 方法须 `wailsjs` 两处同步。
- 写码前须读：`docs/agent-resources/conventions/`（`index-convention.md` 入口）、`coding-error-handling.md`、`coding-vue-style.md`、`project-boundaries.md`、用户全局 `CLAUDE-编码原则.md`。
- 排期机制：`cc-connect timer add --at "2026-12-14T09:07"`，已登记 ID `27550c82`；一次性用 timer，不用 cron。
- **实施子代理失败教训（2026-11-26）**：首轮 dispatch 因 12 条 jsonl 注入约 280KB spec 全文（含 `browser-channel.md` 116KB）+ 31KB PRD 触发 API 400「input length too long」，业务代码零落盘。收敛为 5 条小体积 spec（约 35KB）+ PRD 精简 + 分两批 dispatch。
- **子代理中断前有效发现**：`OverviewView.spec.js` 的文件级 `vi.mock` 仅声明 `GetOverviewSnapshot`，新增 `RecordOverviewOpen` import 必须同步扩展该 mock，否则既有用例挂。

## 5. 全局编码原则摘要

`C:\Users\liuyang\.claude\self\CLAUDE-编码原则.md` 四原则：

1. **编码前思考**：不要假设、不要隐藏困惑、呈现权衡；有歧义时询问而非默默选择。
2. **简洁优先**：用最少代码解决问题，不添加要求之外的功能、不为一次性代码创建抽象、不为不可能发生的场景做错误处理。
3. **精准修改**：只碰必须碰的，不「改进」相邻代码，匹配现有风格；删除因自身改动产生的孤儿导入/变量，不删预先存在的死代码。
4. **目标驱动执行**：定义成功标准，循环验证直到达成；多步骤任务列简短计划并逐步验证。
