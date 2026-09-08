# AI 运行历史归档 + 大输出流式文件（第 3 批）

## Goal

将 AI 任务运行结果归档为可回溯的持久化历史记录，同时将大输出从内存 `strings.Builder` 迁移到流式文件，解决长输出（agree-slides 类）前端卡顿与内存积累。二者经 `os.Rename` 零拷贝衔接：任务完成后输出文件直接移入归档目录，无大对象过 IPC。

合并 P1-2（运行历史归档）与 P0-4(3.3)（大输出流式文件）实施，避免分批返工。

## What I already know

### 设计决策已定稿（来自 4 份必读文档，勿重新发散）

- **表格视图兜底**：方案 A 后端预解析 `TableExtracted`，`GetAiTaskState`/`AiTaskRunResult` 增 `TableExtracted *MeetingTable` 字段（`nil` 表示无表格）。来源：output-memory 4.4 / 总览 7.2
- **存储结构**：方案 B 元数据 JSON + 输出文件分存。`data/ai_task_history.json` 存元数据（轻量快速加载），`data/ai_task_history/<id>.txt` 存完整输出（按需懒加载）。来源：run-history 4.1
- **流式文件**：`aiTaskRuntime.output` 由 `strings.Builder` 改为 `*os.File` + `outputSize int64`；`pumpOutput` 写文件；`GetAiTaskState` 返回尾部预览（末尾 ~4KB）+ `OutputSize` + `OutputFile`。来源：output-memory 4.3
- **零拷贝衔接**：运行期输出文件 `data/ai_task_output/<id>.txt`，归档时 `os.Rename` 移到 `data/ai_task_history/<id>.txt`，元数据只存路径与大小。来源：output-memory 4.3 / 总览 7.2
- **末尾窗口**：前端展示 256KB（第 1 批已做），后端 `GetAiTaskState` 预览末尾 ~4KB。来源：output-memory 4.1
- **copy 全量读取**：3.3 阶段改调 `GetAiTaskOutput`（3.1 阶段的 `t.fullOutput` 兜底废弃）。来源：output-memory 4.1/4.3

### 第 1/2 批已完成衔接点（勿重复改动）

- `AiTaskMetrics` / `AiTaskUsage` 已落地（`model/ai_function.go:86`），历史归档直接复用
- `AiTaskState` 已有 `Metrics`/`Queued` 字段，`GetAiTaskState` 已返回 metrics
- 前端 `AiFunctionPanel.vue` 已有 `truncated`/`fullOutput`/256KB 截断（3.1）；`copyOutput`/`previewOutput`/`meetingTable` 取 `t.fullOutput` 兜底
- `concurrencySem` 并发上限（第 1 批）不动
- `RemoveAiTask` 已存在（`app.go:1333` / `service/ai_function.go:285`），第 1 批只 delete map，本批加输出文件删除
- `AiFunctionConfigDialog` 四块表单 + MCP stdio（第 2 批）不动

### 当前代码落点（codegraph + Read 探查确认）

| 文件 | 现状 | 本批改造 |
| --- | --- | --- |
| `service/ai_function.go:53` | `aiTaskRuntime.output strings.Builder` | 改 `*os.File` + `outputSize int64` |
| `service/ai_function.go:610` | `pumpOutput` 中 `task.output.WriteString(text)` | 改写文件 + `outputSize += len` |
| `service/ai_function.go:635` | `result.Output = task.output.String()` 全量拷贝 | 改尾部预览 + `TableExtracted` 预解析 |
| `service/ai_function.go:256` | `GetAiTaskState` 返回 `task.output.String()` 全量 | 改尾部预览 + `OutputSize`/`OutputFile`/`TableExtracted` |
| `service/ai_function.go:285` | `RemoveAiTask` 只 delete map | 加删输出文件 |
| `model/ai_function.go:112` | `AiTaskState.Output` 全量 | 增 `OutputSize`/`OutputFile`/`TableExtracted`，`Output` 改预览语义 |
| `model/ai_function.go:101` | `AiTaskRunResult.Output` 全量 | 同上增字段，`Output` 改预览 |
| `AiFunctionPanel.vue:514,526,340` | `copyOutput`/`previewOutput`/`meetingTable` 取 `t.fullOutput` | 改调 `GetAiTaskOutput`；`meetingTable` 优先用 `result.tableExtracted` |
| `frontend/wailsjs/go/main/App.{js,d.ts}` | 8 个 AI 方法 | 新增 `GetAiTaskOutput`/`GetAiTaskHistory`/`DeleteAiTaskHistory`/`ClearAiTaskHistory` |
| `frontend/wailsjs/go/models.ts` | `AiTaskState`/`AiTaskRunResult` TS 定义 | 同步新增字段 + `AiTaskHistory`/`MeetingTable` |

## Assumptions (temporary)

- 输出文件运行期目录 `data/ai_task_output/`，归档目录 `data/ai_task_history/`，应用启动时自动创建
- `MeetingTable` 结构：`Headers []string` + `Rows []map[string]string`，后端从输出全文解析 markdown 表格（复用前端 `parseMarkdownTable` 逻辑移植到 Go）
- `AiTaskRunResult.Output` 3.3 后改为尾部预览（与 `GetAiTaskState` 一致），前端 `onDone` 不再写 `t.fullOutput`，`fullOutput` 字段废弃
- 归档在 `pumpOutput` 末尾 `emit("ai-task:done")` 之前完成，done 事件 payload 不含全量输出（避免大对象过 IPC），前端按需调 `GetAiTaskOutput`

## Decision (ADR-lite)

> 4 个产品/UX 参数按 design 推荐值定稿（2026-09-09，用户授权按已定稿决策实施）。

| 决策项 | 结论 | 依据 |
| --- | --- | --- |
| 历史保留策略 | 2000 条 + 90 天双上限，先到先清理最旧任务，归档时自动触发 | run-history 4.5 推荐 |
| 输出文件定时清理周期 | 1 天，兜底清理未归档的残留文件（归档接管已 `os.Rename` 移走不留残） | output-memory 4.5 推荐 |
| 历史面板本批范围 | 基础筛选（功能下拉/时间范围/状态多选）+ 列表（时间·功能·状态·耗时·成本·输出大小）+ 点击详情查看输出（懒加载） | run-history 4.4 |
| 继续会话 `--resume` | 本批不做，session_id 存入历史元数据，后续单独补入口（零成本） | run-history 4.6 属增强 |

**Context**：4 份 design 文档决策已定稿，仅剩 4 个产品参数默认值待确认。用户授权按 design 推荐值实施，不再发散。
**Decision**：采用各 design 文档第 4/7 节明确推荐的默认值（见上表）。
**Consequences**：保留策略与清理周期均为硬编码常量（与 `aiTaskMaxConcurrent` 一致，后续随 schema v2 入配置可调）；继续会话延后，但 session_id 已落盘不丢。

## Requirements (evolving)

### P0-4(3.3) 流式文件

- `aiTaskRuntime.output` 改 `*os.File` + `outputSize int64`，`RunStage` 起进程前创建 `data/ai_task_output/<taskID>.txt`
- `pumpOutput` 写文件而非 Builder，持锁更新 `outputSize`
- `GetAiTaskState` 返回尾部预览（末尾 ~4KB）+ `OutputSize` + `OutputFile` + `TableExtracted`（预解析表格），不再全量 `String()` 拷贝
- 新增 `GetAiTaskOutput(taskID) (string, error)` 全量读取输出文件（供 copy/preview/表格视图触发时读取）
- `AiTaskRunResult.Output` 改预览语义，增 `OutputSize`/`OutputFile`/`TableExtracted`
- 前端 `copyOutput`/`previewOutput` 改调 `GetAiTaskOutput`；`meetingTable` 优先用后端预解析 `TableExtracted`
- 输出文件清理三路径：Tab 关闭（`RemoveAiTask` 删文件）/ 归档接管（`os.Rename` 移走）/ 定时清理（1 天周期，兜底未归档残留）

### P1-2 历史归档

- `model/ai_task_history.go` 新增 `AiTaskHistory`，复用 `AiTaskMetrics`（耗时/token/成本/轮次）
- 归档时机：任务完成后（`ai-task:done` 前），输出文件 `os.Rename` 零拷贝移入 `data/ai_task_history/<id>.txt`
- 归档存储：`data/ai_task_history.json` 元数据 + `data/ai_task_history/<id>.txt` 输出文件（方案 B）
- `GetAiTaskHistory(filter)` / `DeleteAiTaskHistory(id)` / `ClearAiTaskHistory(criteria)` App 方法
- 前端历史列表视图（复用 metrics 展示样式），点击查看输出（读归档文件，懒加载）
- emit `ai-task:archived` 事件通知前端刷新历史列表

### 跨层同步

- 新增 App 方法（`GetAiTaskOutput`/`GetAiTaskHistory`/`DeleteAiTaskHistory`/`ClearAiTaskHistory`）同步 `frontend/wailsjs/go/main/App.js` + `App.d.ts`
- model 新增 struct（`AiTaskHistory`/`MeetingTable`）与字段同步 `frontend/wailsjs/go/models.ts`

## Acceptance Criteria (evolving)

- [ ] 大输出任务（agree-slides 类）运行后前端不卡顿，后端内存不随输出增长积累（`strings.Builder` 不再全量驻留）
- [ ] copy/preview 完成动作取全量输出（经 `GetAiTaskOutput`），不依赖已截断的 `t.output` 或 `t.fullOutput`
- [ ] 表格视图在输出超 256KB 截断后仍能正确渲染（经后端预解析 `TableExtracted`）
- [ ] 任务完成后自动归档，输出文件经 `os.Rename` 零拷贝移入归档目录（无大对象过 IPC）
- [ ] 历史列表展示耗时/token/成本/轮次（复用 `AiTaskMetrics`），点击可查看输出（懒加载）
- [ ] 输出文件三路径清理生效，无残留临时文件
- [ ] `go test ./...` 与 `cd frontend && npm test` 双绿
- [ ] wailsjs 同步完成（`App.js`/`App.d.ts`/`models.ts` 三处），`git status` 无 wailsjs 残留

## Definition of Done

- 后端单测：输出文件写入、尾部读取、大小统计、表格预解析、归档元数据落盘、`os.Rename` 衔接、清理三路径
- 前端单测：copy/preview 改调 `GetAiTaskOutput`、`meetingTable` 用 `TableExtracted`、历史列表渲染、归档事件刷新
- `go test ./...` && `cd frontend && npm test` 双绿
- wailsjs 三处同步（`App.js`/`App.d.ts`/`models.ts`），`git status` 确认无 wailsjs 残留
- README.md / docs 按需更新（CLAUDE.md 要求功能完成后确认）

## Out of Scope (explicit)

- 3.2 事件时间窗口合并（视 3.1 后实测，本批不做）
- 月度统计导出报告（总览第 8 节明确属后续增强）
- 敏感信息脱敏（缺口 3，降为可选，本批不做）
- 历史搜索框关键词搜索（本批只做基础筛选：功能/时间/状态，关键词搜索属后续增强）
- 「继续会话」`--resume` 入口（session_id 已存历史元数据，后续单独补，本批不做）
- `golang.design/x/clipboard` 后端直写剪贴板（3.3 评估项，本批仍走前端 `navigator.clipboard` + `GetAiTaskOutput`）

## Technical Notes

- 必读文档：`docs/plans/2026-09-08-ai-optimization-overview.md`（总览）、`docs/plans/2026-09-08-ai-run-history-design.md`（P1-2）、`docs/plans/2026-09-08-ai-output-memory-design.md`（P0-4）、`docs/spec/cross-layer-contracts.md`（跨层契约）
- 内存约束：写产物 JSON 前须确认 workbench.exe 未运行（避免被 RepoMetaService 覆盖）；本批运行时 JSON 由 service 写，非手动产物
- 跨层契约：新增 App 方法签名须同步 `App.js`/`App.d.ts`；model struct 字段须同步 `models.ts`（字段声明 + 构造函数赋值，`omitempty` → `?:`）
- 前端 `parseMarkdownTable` 逻辑（`AiFunctionPanel.vue:296`）移植到 Go 后端做 `TableExtracted` 预解析，复用同一解析规则保证前后端一致
