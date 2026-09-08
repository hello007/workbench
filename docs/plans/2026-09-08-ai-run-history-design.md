# AI 任务运行历史归档设计

**日期**：2026-09-08
**优先级**：P1
**状态**：待评审

## 1. 概述

任务完成后将执行记录（功能、时间、prompt、输出、session_id、计量）归档到持久化历史，独立于主 Tab 的复用策略，可按功能/时间回看。解决「同功能重跑覆盖旧输出」导致的历史丢失，兑现 [[2026-09-08-ai-skill-menu-design]] 痛点「终端关闭即丢失、无历史记录」的原始诉求。

## 2. 背景与痛点

主 Tab 复用策略（`AiFunctionPanel.vue` 第 247-258 行 `doRunMain`）：同功能重跑时从后往前找非运行中任务 Tab 原位替换：

```js
if (reuseIdx >= 0) {
  tasks.value.splice(reuseIdx, 1, task)  // 旧输出被覆盖
}
```

| 痛点 | 说明 |
| --- | --- |
| 输出被覆盖 | 同功能重跑覆盖上一次输出，无法回看上次结果 |
| 重开丢失 | `tasks` 仅内存态，应用重启后全部丢失 |
| 无 session 复用 | `session_id` 随 Tab 丢失，无法事后 `--resume` 续看 |
| 无统计 | 无累计运行次数、成本、耗时汇总（依赖 [[2026-09-08-ai-task-observability-design]] 的 metrics） |

## 3. 需求总结

1. 任务完成（成功/失败/取消）后自动归档到持久化存储
2. 历史面板按功能、时间范围、状态筛选回看
3. 历史详情展示 prompt、输出、session_id、计量、exitCode
4. 支持清理：按时间（如 30 天前）、按功能、手动单条删
5. 主 Tab 复用策略不变，历史独立留存

## 4. 设计

### 4.1 存储方案对比

| 方案 | 结构 | 优点 | 缺点 |
| --- | --- | --- | --- |
| A. 单 JSON | `data/ai_task_history.json` 存全部（含输出） | 实现简单 | 输出大，文件膨胀，全量读写 |
| B. 元数据 + 输出文件 | `data/ai_task_history.json` 存元数据；`data/ai_task_history/<id>.txt` 存完整输出 | 元数据轻量快速加载；输出按需读取 | 多文件管理，清理须同步 |
| C. SQLite | 单库文件，表 + 索引 | 查询/分页/统计高效 | 引入依赖，与现有 JSON 配置体系不一致 |

**推荐方案 B**：元数据 JSON 加载快，输出文件按需懒加载，与现有 `data/` 配置体系一致，清理逻辑清晰。

### 4.2 数据模型

`model/ai_task_history.go` 新增：

```go
// AiTaskHistory 历史记录元数据（完整输出单独存文件）
type AiTaskHistory struct {
    ID         string             `json:"id"`         // 任务 id
    FunctionID string             `json:"functionId"`
    Name       string             `json:"name"`       // 功能名快照
    Prompt     string             `json:"prompt"`     // prompt 预览
    StartedAt  int64              `json:"startedAt"`  // unix 毫秒
    FinishedAt int64              `json:"finishedAt"`
    Status     string             `json:"status"`     // success / failed / canceled / timeout
    ExitCode   int                `json:"exitCode"`
    Error      string             `json:"error"`
    SessionID  string             `json:"sessionId"`  // 供事后 --resume
    Metrics    *AiTaskMetrics     `json:"metrics,omitempty"` // 依赖可观测性设计
    OutputFile string             `json:"outputFile"` // 输出文件相对路径
    OutputSize int64              `json:"outputSize"` // 输出字节数（列表展示用）
}
```

### 4.3 归档时机与流程

`pumpOutput` 结束、`emit("ai-task:done", result)` 前后触发归档（service 层）：

1. 生成输出文件 `data/ai_task_history/<id>.txt`，写入 `task.output.String()`
2. 构造 `AiTaskHistory` 元数据
3. 追加到 `data/ai_task_history.json`（全量重写，量小可接受；超千条改分片）
4. emit `ai-task:archived` 事件通知前端历史面板刷新

> 主 Tab 复用策略不变：归档与 Tab 渲染解耦，Tab 仍可覆盖，历史已先留存。

### 4.4 历史面板

`AiFunctionPanel.vue` 新增「历史」入口（标题栏按钮或独立 Tab）：

- 左侧筛选：功能下拉、时间范围、状态多选
- 右侧列表：时间 | 功能 | 状态 | 耗时 | 成本 | 输出大小
- 点击行展开详情：prompt、计量、exitCode、error，输出懒加载（点「查看输出」读取文件）
- 操作：单条删除、按条件批量清理、复制 session_id

### 4.5 清理策略

| 触发 | 行为 |
| --- | --- |
| 手动单条删 | 删元数据 + 输出文件 |
| 按时间清理 | 如「清理 30 天前」，删符合条件的元数据与输出文件 |
| 容量上限 | 元数据超 N 条（如 2000）时自动清理最旧（可配置阈值） |

清理均同步删输出文件，避免孤儿文件。

### 4.6 session_id 复用

历史详情提供「继续会话」按钮：以 `session_id` 调 `RunAiFollowUp` 机制在原会话 `--resume` 发送新 prompt。用途：上周报草稿事后确认落盘、会议事后取消。

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `model/ai_task_history.go` | 新增 | `AiTaskHistory` |
| `service/ai_task_history.go` | 新增 | 归档、加载、筛选、清理、输出文件读写 |
| `service/ai_function.go` | 修改 | `pumpOutput` 完成后触发归档 |
| `app.go` | 新增绑定 | `GetAiTaskHistory(filter)` / `DeleteAiTaskHistory(id)` / `ClearAiTaskHistory(criteria)` |
| `frontend/src/components/AiTaskHistoryPanel.vue` | 新增 | 历史面板 |
| `frontend/src/components/AiFunctionPanel.vue` | 修改 | 标题栏增「历史」入口；监听 `ai-task:archived` 刷新 |
| `frontend/wailsjs/` | 同步 | 新增绑定 |

## 6. 验收标准

1. 任务完成后自动归档，元数据与输出文件均落盘
2. 历史面板按功能/时间/状态筛选生效
3. 输出懒加载，大输出不阻塞列表
4. 清理同步删输出文件，无孤儿
5. 重开应用后历史完整恢复
6. 「继续会话」以 session_id `--resume` 成功

## 7. 待确认点

1. 输出文件单条大小上限？超长是否截断存储？建议完整存储，仅列表展示 `outputSize`，详情按需读
2. 历史保留策略默认值（条数上限 / 天数）？建议默认 2000 条或 90 天，可配置
3. 是否需要导出历史为报告（如月度运行统计）？本设计仅展示，导出属后续增强

## 8. 关联

- 来源：[[2026-09-08-ai-skill-menu-design]] 痛点「过程不可视、无历史记录」的彻底兑现
- 依赖：[[2026-09-08-ai-task-observability-design]]（metrics 字段）
