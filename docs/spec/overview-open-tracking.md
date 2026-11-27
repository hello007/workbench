# 概览面板主动打开埋点契约（T4 Go/No-Go 门槛取证）

> 适用范围：个人数据看板 T4「按 Go/No-Go 门槛验证 P0 使用闭环」的 G1 取证链路，以及判 Go 后 T5 复用该日志器的扩展。
> 决策真相源：`.trellis/tasks/11-24-dashboard-t4-go-no-go-gate/prd.md`（D1–D9 / C1–C13 / AC1–AC13）。
> 本文档只描述**可执行的跨层契约**，不含判定结论；判定协议与 `observation.md` 由 T4 任务目录承载。
> 建立日期：2026-11-27。

## 1. Scope / Trigger

本任务同时命中「新增 App 方法签名」「跨层请求契约」「文件存储接入」三项，**按 Code-Spec First 规则必须补齐七节可执行契约**。

| 变更面 | 位置 | 触发理由 |
|---|---|---|
| App 绑定方法 | `app_overview.go` | 新增 `RecordOverviewOpen(source string) error`，Wails 绑定与浏览器 RPC 反射路由同时可达 |
| 生成产物 | `frontend/wailsjs/go/main/App.js`、`App.d.ts` | 新增方法须两处手同步；无新增 `model/` 导出 struct，`models.ts` 不变 |
| 存储接入 | `service/overview_open_log.go`、`data/overview_opens.log`、`.gitignore` | 新增 append-only JSONL 旁路观测面，须明确写入豁免口径与忽略规则 |
| 前端信箱 | `frontend/src/store/ui.js`、`OverviewView.vue`、`ActivityBar.vue`、`Home.vue` | 新增「进入来源」一次性信箱，用于区分主动/被动进入 |
| 判定日读取 | `.trellis/tasks/11-24-dashboard-t4-go-no-go-gate/observation.md` | 判定任务只读日志按 `date` 归 W1/W2 两段，不新增 RPC 出口 |

## 2. Signatures（单点真相源）

| 签名 | 约束 |
|---|---|
| `App.RecordOverviewOpen(source string) error` | **恒返回 nil error**；委托 `overviewOpenLogSvc.Record(source)`，依赖为 nil 时 no-op；非法 `source` 由 service 白名单丢弃；旁路观测面不得阻断概览加载 |
| `service.NewOverviewOpenLog(path string) *OverviewOpenLog` | 路径由调用方注入；生产为 `filepath.Join(dataDir, "overview_opens.log")`；构造纯 `new`，不预建文件、不启动 goroutine |
| `(*OverviewOpenLog).Record(source string)` | O_APPEND 单行原子追加；无返回值；写失败仅 `Logger().Warn` 降级 |
| `(*OverviewOpenLog).ReadAll() ([]OverviewOpenEntry, error)` | 只读解析；文件不存在返回空切片 + nil error；坏行/空行跳过 |
| `validOverviewOpenSource(source string) bool` | **唯一白名单守卫**：仅 `click` / `shortcut` / `alt` 返回 true；禁止在 App 层或前端重复校验 |
| `useUiStore().overviewEntrySource` + `setOverviewEntrySource(source)` | 一次性信箱；写入小写字面量；不持久化、不进会话快照；`OverviewView` watch 消费即清空 |
| `OverviewView` 的 `watch(() => uiStore.activePanel, ..., { immediate: true })` | 命中 `overview` 时读信箱，非空才调 `RecordOverviewOpen`，随后**无条件** `load(false)` |
| `frontend/wailsjs/go/main/App.js` / `App.d.ts` | 两处手同步新方法；**禁止执行 `wails generate module`**（会覆盖 T3 手补的 `models.ts` 别名与本次两处手同步） |

## 3. Contracts（跨层字段）

### 3.1 日志行结构

`data/overview_opens.log` 为 JSONL，每行一个 `OverviewOpenEntry`：

| 字段 | 类型 | 语义与约束 |
|---|---|---|
| `ts` | int64 | Unix 毫秒；与 `date` 必须由**同一次 `time.Now()`** 生成，避免跨自然日边界归段漂移 |
| `date` | string | 本地时区 `YYYY-MM-DD`；判定日按此字段排除适应期 `2026-11-26 ~ 11-29` 并归 W1/W2 |
| `source` | string | `click`（侧边栏图标）/ `shortcut`（自定义 `openOverview` 快捷键）/ `alt`（`Alt+数字` 按 `orderedPanels` 命中概览）；**小写常量，大小写敏感** |

### 3.2 写入与通道契约

| 项 | 约束 |
|---|---|
| 写入方式 | `os.OpenFile(path, O_APPEND|O_CREATE|O_WRONLY, 0o600)` 单行 `Write`；`sync.Mutex` 保护；**不复用 `AuditLogger` / `AuditRecord` 闭集**（否则污染 `QueryAuditLog` 读侧） |
| 写入口径 | 属 `docs/spec/data-atomic-write.md` 的「非 JSON 写入」append-only 豁免，**不走 `util.SaveJSON`**；整体覆盖型 JSON 仍须走原子写 |
| 版本控制 | `.gitignore` 必须含 `data/overview_opens.log`；运行期日志**不入库**，判定证据经 `observation.md` / `decision.md` 落盘 |
| 浏览器通道 | `main.go` 的 `RPCHiddenMethods` 为黑名单制且**不含** `RecordOverviewOpen`；桌面与浏览器经同一 App 方法落同一文件，避免双通道数据分裂 |
| 隐私边界 | 方法只写一条无敏感字段记录、不返回明细；不触碰「他人 IP / token 明细只走桌面出口」红线 |
| 扩展点 | 判 **Go** 后 T5 必须直接复用 `OverviewOpenLog` 扩展，不得另起一套计数定义，保证 T4 证据与 T5 数据可比 |

### 3.3 主动 / 被动入口矩阵

| 入口 | 是否写 `overviewEntrySource` | 是否落盘 |
|---|---|---|
| `ActivityBar.vue` 点击概览图标 | `click` | 是 |
| `Home.vue` `openOverview` 快捷键 | `shortcut` | 是 |
| `Home.vue` `Alt+数字` 命中 `panels[sidebarIdx].id === 'overview'` | `alt` | 是 |
| 会话快照恢复写回 `activePanel` | 否 | **否** |
| `OverviewView` watch 的 `immediate: true` 首次执行 | 否 | 否 |
| 切档 `el-radio-group @change="load(false)"` | 否 | 否 |
| 手动刷新 `load(true)` / 卡片重试 | 否 | 否 |
| 后端 5 分钟 TTL 缓存命中 | 否 | 否 |
| 下钻切走（`goDashboard` / `goTodo` / `goAi`） | 否 | 否 |

## 4. Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| `source` 非三档白名单（含空串、大写） | `validOverviewOpenSource` 直接 return，**不创建文件、不写行** |
| `MkdirAll` / `OpenFile` / `Write` 失败 | 仅 `Logger().Warn` 降级；`Record` 无返回值；App 恒 nil error；前端 `load(false)` 照常执行 |
| `json.Marshal` 失败 | 防御性 Warn 降级（全基本类型字段，生产不可达） |
| `App.overviewOpenLogSvc == nil` | no-op，不 panic |
| `ReadAll` 文件不存在 | 返回空切片 + nil error |
| `ReadAll` 遇坏行 / 空行 / 缺失 `date` 或 `source` | 跳过该行，继续解析 |
| `RecordOverviewOpen` 在前端 reject 或同步抛错 | `Promise.resolve().then(...).catch(console.warn)` 兜住；不弹 `ElMessage`、不引入新错误码、不阻断 `load(false)` |
| 停留于 `overview` 时再次点击同一入口 | `activePanel` 同值赋值不触发 watch，信箱残留；但生产置回 overview 的路径仅三个显式入口（均覆盖 source）与启动期恢复（信箱初始为空），**无真实误报路径**；该次点击按 D4 不计入 G1 |
| 浏览器通道持 token 调用 | 可写 `data/overview_opens.log`；属 C4 已授权决策，伪造风险由 D7「次数明显异常且无对应行为案例 → No-Go」兜底 |

## 5. Good / Base / Bad Cases

| 档 | 场景 |
|---|---|
| Good | 点击图标 → 信箱写 `click` → watch 消费即清 → 日志新增一行 `{ts,date,source:"click"}` → `load(false)` 正常；重启后 `ReadAll` 仍能读到该行；桌面与浏览器分别打开汇入同一文件 |
| Good | 会话恢复把 `activePanel` 写回 `overview` → watch 触发但信箱为空 → **不落盘**；切档 / 手刷 / TTL 命中同样不落盘 |
| Base | 日志目录不可写 → Warn 降级，概览面板照常加载；判定日按 D7 证据不足处理 |
| Base | `ReadAll` 读到一行坏 JSON → 跳过该行，其余行正常聚合 |
| Bad | 在 `GetOverviewSnapshot` 内隐式追加记录 → 进入面板 / 切档 / 手刷 / 重试 / TTL 命中全部混入，无法鉴别「主动打开」，门槛失效 |
| Bad | 复用 `AuditLogger.Log` 与 `AuditRecord` → `QueryAuditLog` 读侧出现非认证事件，语义污染 |
| Bad | 把 `overviewEntrySource` 写进 `session.json` → 会话恢复误计一次，每次重启自动 +1 |

## 6. Tests Required（断言点）

| 层 | 断言点 |
|---|---|
| 后端单测 | 写入字段一致性（`ts`/`date`/`source` 同源）；重启不丢（重新构造实例再 `ReadAll`）；非法 source 不落盘且不建文件；64 并发追加后物理行完整；坏行/空行跳过；文件不存在返回空；写失败仅降级不 panic |
| 前端单测 | 三入口各写一次且为小写字面量；点击非概览面板不写源；会话恢复写回无源不调用；`immediate` 首帧不调用；切档 `@change` 不调用；手刷 `load(true)` 不调用；消费即清（切走再切回不重复上报）；`RecordOverviewOpen` reject 时 `load(false)` 仍执行且不弹 ElMessage；spy 须 `afterEach` restore + `enableAutoUnmount` |
| E2E | 从 `frontend/e2e/fixtures.js` import `{ test, expect }`；真实浏览器点击概览图标 → `getWailsCalls` 断言恰好一次 `click`；切档不新增调用；禁 `waitForTimeout`，用 `expect.poll` |
| Mock 单一数据源 | `frontend/src/test/wails-mock-defaults.js` 登记 `RecordOverviewOpen: null`（**值表**，须可结构化克隆供 Playwright `addInitScript` 序列化；不可写成函数）；两个 spec 的文件级 `vi.mock` 须各自补该方法 |
| 回归 | `wailsjs` 两处含新方法、`models.ts` 未改；`RPCHiddenMethods` 不含新方法；`.gitignore` 命中；前端覆盖率 ≥70%、service ≥76% |

## 7. Wrong vs Correct

### 7.1 埋点挂点

**Wrong**：在聚合方法内隐式记录，无法区分主动/被动。

```go
// Wrong：进入面板、切档、手刷、TTL 命中都调 GetOverviewSnapshot
func (a *App) GetOverviewSnapshot(rangeKey string, force bool) (*model.OverviewSnapshot, error) {
    a.overviewOpenLogSvc.Record("open") // 口径不可鉴别
    return a.overviewSvc.GetSnapshot(rangeKey, force), nil
}
```

**Correct**：只在显式入口写一次性信箱，watch 消费即清。

```javascript
// Correct：ActivityBar.vue 仅在命中概览时登记来源
@click="if (item.id === 'overview') uiStore.setOverviewEntrySource('click'); uiStore.activePanel = item.id"

// Correct：OverviewView.vue 命中 overview 才消费
const source = uiStore.overviewEntrySource
uiStore.overviewEntrySource = ''
if (source) {
  Promise.resolve()
    .then(() => RecordOverviewOpen(source))
    .catch((error) => console.warn('[overview] 记录面板打开失败（埋点降级，不影响概览加载）:', error))
}
load(false)
```

### 7.2 日志器归属

**Wrong**：复用认证审计闭集。

```go
// Wrong：AuditRecord 字段是认证/连接管控专用，QueryAuditLog 读侧会被污染
auditLogger.Log("overview_open", "", "", "", "")
```

**Correct**：独立 O_APPEND JSONL，失败旁路降级。

```go
// Correct：service/overview_open_log.go 独立日志器
f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
if err != nil {
    Logger().Warn("open overview open log failed", "path", l.path, "err", err)
    return
}
_, _ = f.Write(line)
_ = f.Close()
```

## 8. 设计决策（ADR-lite）

| # | 决策 | 关键理由 |
|---|---|---|
| 1 | 白名单收口在 `validOverviewOpenSource` 单点，App 层只委托 | 避免双份校验漂移；单测可直接覆盖「非法 source 不落盘」 |
| 2 | 前端用一次性邮箱而非「当前入口」持久化状态 | 会话恢复、切档、手刷天然无源，零额外判定逻辑即可排除被动进入 |
| 3 | 不新增 App 查看/导出方法，判定日直接只读本地 JSONL | 一年仅用一次的判定；避免把「打开过几次概览」暴露到浏览器通道；符合简洁优先 |
| 4 | `RecordOverviewOpen` 不进 `RPCHiddenMethods` | 双通道须落同一文件，隐藏会造成数据分裂；隐私面无明细字段 |
| 5 | 失败采用 `console.warn` 而非完全静默 | 对齐 `coding-error-handling.md`「禁空 catch」，同时不弹提示、不引入新错误码、不阻断加载 |
| 6 | `ActivityBar.vue` 保留模板内联多语句 | 实测 `@vue/compiler-sfc` 3.5.44 编译稳定、E2E 真实浏览器通过；按「精准修改」不在本轮重写，建议观察期后等价迁到 `<script setup>` handler |

## 9. 已知边界（非缺陷）

| 边界 | 处置 |
|---|---|
| 同值赋值不触发 watch 导致信箱残留 | 生产无真实误报路径；列为观察期后的口径澄清项，本轮不改 |
| 持有浏览器 token 者可伪造记录 | C4 已授权，D7 异常次数兜底；判定日须核对 |
| TTL 命中未单独构造单测 | 以「无新来源重复进入」覆盖同等语义；部署后手工核对 |
| 运行期桌面/浏览器双通道落盘对照 | 子代理未重启 `workbench.exe` 实证；须用户部署后自测（AC13） |

## 10. 修订记录

| 日期 | 修订 | 依据 |
|---|---|---|
| 2026-11-27 | 初版：T4 收口评审，补齐七节跨层 code-spec | 任务 `11-24-dashboard-t4-go-no-go-gate` 收口 |
