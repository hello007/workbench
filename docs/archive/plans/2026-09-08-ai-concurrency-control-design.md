# AI 任务全局并发执行上限设计

**日期**：2026-09-08
**优先级**：P0（头脑风暴补充，与 [[2026-09-08-ai-task-observability-design]] 同批）
**状态**：决策已定，待实施（2026-09-08 评审确认）

## 1. 概述

为 `AiFunctionService` 引入全局并发信号量，限制同时运行的 `claude -p` 子进程数量（默认 3），超限任务排队等待并向前端暴露排队态。解决多 Tab 并发起进程导致的本地资源抢占、API 侧限流与超时误判，保护单任务执行预算。

## 2. 背景与痛点

`RunStage`（service/ai_function.go:89）是唯一执行入口，`RunAiFunction`（app.go:1253）与 `RunAiFollowUp`（app.go:1277）最终都调它。当前流程直接 `cmd.Start()` 后异步 `go s.pumpOutput`，无全局并发控制：

```go
if err := cmd.Start(); err != nil { ... }
s.mu.Lock()
s.tasks[taskID] = task
s.mu.Unlock()
go s.pumpOutput(task, stdout)
return taskID, nil
```

`AiFunctionService` struct（service/ai_function.go:25）仅 `mu sync.Mutex` + `tasks map`，`s.mu` 只保护单 task 字段与 map 写入，**不阻止并发起进程**。前端 `doRunMain`（AiFunctionPanel.vue:230）每次点击即调 `RunAiFunction`，用户连续触发 N 个功能项即同时起 N 个 claude 进程。

| 痛点 | 说明 |
| --- | --- |
| 进程数无上限 | 单个 claude 进程常驻 ~150-300MB，5 并发即 ~1-1.5GB 本地内存 |
| API 侧限流 | 多会话并发触发 Anthropic 速率限制，集体超时 |
| 超时误判 | `ctx` 从 `cmd.Start()` 起算（RunStage:102），本地资源抢占致 claude 响应变慢，10 分钟超时可能在仅 3 分钟有效工作时触发，用户感知为「功能不稳定」 |
| 事件洪流 | 多流并发 `emit("ai-task:output")`，前端 `onOutput` 遍历 `tasks` 查找，大输出时渲染卡顿 |
| tasks map 无限增长 | 全代码无 `delete(s.tasks,...)` / `RemoveAiTask`，已完成任务对象永久驻留 map，长期使用内存持续增长 |

## 3. 需求总结

1. 全局并发上限，默认 3，超限任务排队等待
2. 排队态对前端可见，Tab 区分「运行中」与「排队中」
3. 排队任务可取消，取消后立即让出排队位
4. 超时计时从获取执行槽位后起算，排队等待不侵蚀执行预算
5. 信号量释放与进程生命周期绑定，进程退出即释放槽位
6. 标题栏展示并发占用情况（如「并发 2/3」）

## 4. 设计

### 4.1 信号量方案

`AiFunctionService` 增带缓冲 channel 作为全局信号量，缓冲长度即并发上限：

```go
const aiTaskMaxConcurrent = 3 // 默认并发上限，先硬编码常量，后续随 schema v2 入配置

type AiFunctionService struct {
    ctx            context.Context
    configPath     string
    mu             sync.Mutex
    tasks          map[string]*aiTaskRuntime
    concurrencySem chan struct{} // 新增：全局并发信号量，缓冲 = aiTaskMaxConcurrent
}

func NewAiFunctionService(ctx context.Context, configPath string) *AiFunctionService {
    return &AiFunctionService{
        ctx:            ctx,
        configPath:     configPath,
        tasks:          make(map[string]*aiTaskRuntime),
        concurrencySem: make(chan struct{}, aiTaskMaxConcurrent),
    }
}
```

### 4.2 排队等待与槽位获取

`RunStage` 在 `loadFunction` 后、`cmd.Start` 前获取信号量。排队期间用 `s.ctx`（app 级）响应取消，获取信号量后再 `WithTimeout` 创建执行 ctx——这是超时起算点后移的关键：

```go
// RunStage 内，prompt 校验后、cmd 组装前

// 1. 注册 task 为排队态（先入 map，前端立即可见 Tab）
taskID := fmt.Sprintf("aitask-%d", time.Now().UnixNano())
task := &aiTaskRuntime{
    id:         taskID,
    functionID: functionID,
    prompt:     prompt,
    queued:     true, // 新增字段：排队中
    queuedAt:   time.Now(),
}
s.mu.Lock()
s.tasks[taskID] = task
s.mu.Unlock()
s.emit("ai-task:queued", map[string]any{"taskId": taskID})

// 2. 排队等待槽位；用 s.ctx 响应 app 关闭，用 task.canceled 响应用户取消
select {
case s.concurrencySem <- struct{}{}:
    // 获取到，继续
case <-s.ctx.Done():
    s.mu.Lock()
    delete(s.tasks, taskID)
    s.mu.Unlock()
    return "", fmt.Errorf("应用关闭，任务未启动")
}

// 3. 用户在排队期间取消：不启动进程，释放排队位
s.mu.Lock()
if task.canceled {
    delete(s.tasks, taskID)
    s.mu.Unlock()
    <-s.concurrencySem // 归还刚获取的槽位
    return "", fmt.Errorf("任务已取消")
}
task.queued = false
s.mu.Unlock()
s.emit("ai-task:started", map[string]any{"taskId": taskID})

// 4. 获取槽位后才创建执行 ctx（超时起算点后移）
timeout := fn.TimeoutMinutes
if timeout <= 0 {
    timeout = aiTaskDefaultTimeoutMinutes
}
ctx, cancel := context.WithTimeout(s.ctx, time.Duration(timeout)*time.Minute)
task.ctx = ctx
task.cancel = cancel
// ... 后续 cmd 组装、Start、pumpOutput（pumpOutput defer 释放信号量）
```

### 4.3 槽位释放

`pumpOutput` 末尾 defer 释放信号量，与进程退出绑定（`cmd.Wait()` 返回即释放）：

```go
func (s *AiFunctionService) pumpOutput(task *aiTaskRuntime, stdout pipeReader) {
    defer func() { <-s.concurrencySem }() // 进程结束释放槽位
    // ... 现有 scanner 循环、Wait、result 构造不变
}
```

**关键**：释放与 `tasks` map 清理无关——map 残留的 `AiTaskState` 数据对象不占并发槽，槽位随进程退出归还。map 清理是独立的内存增长问题（见 4.6）。

### 4.4 排队期间取消

`CancelAiTask`（service/ai_function.go:142）现有逻辑仅杀进程。排队中的任务无进程，须区分两态：

```go
func (s *AiFunctionService) CancelAiTask(taskID string) bool {
    s.mu.Lock()
    task, ok := s.tasks[taskID]
    if !ok {
        s.mu.Unlock()
        return false
    }
    task.canceled = true
    queued := task.queued
    s.mu.Unlock()

    if queued {
        // 排队中：不杀进程（无进程），等 RunStage 的 select 醒来后自行清理
        // 或主动从 map 删除并 emit done（canceled=true）
        s.mu.Lock()
        delete(s.tasks, taskID)
        s.mu.Unlock()
        s.emit("ai-task:done", model.AiTaskRunResult{
            TaskID:   taskID,
            Canceled: true,
            Error:    "已取消（排队中）",
        })
        return true
    }
    // 运行中：杀进程树（现有逻辑）
    killProcessTree(task.cmd)
    return true
}
```

### 4.5 数据模型与前端展示

`aiTaskRuntime` 增排队字段；`AiTaskState` 增 `Queued` 供前端恢复展示：

```go
type aiTaskRuntime struct {
    // ... 现有字段
    queued   bool      // 新增：排队中（未获取信号量）
    queuedAt time.Time // 新增：入队时间（供排队时长展示）
}
```

```go
type AiTaskState struct {
    // ... 现有字段
    Queued bool `json:"queued"` // 新增：排队中
}
```

`GetAiTaskState` 返回时填 `Queued: task.queued`。

前端 `AiFunctionPanel.vue`：

| 改动点 | 说明 |
| --- | --- |
| `task` 对象 | 增 `queued: false` 初值 |
| `onQueued` 事件 | 新增，监听 `ai-task:queued`，设 `t.queued = true` |
| `onStarted` 事件 | 新增，监听 `ai-task:started`，设 `t.queued = false` |
| `statusText` | 增排队态：`t.queued ? '排队中' : ...` |
| `statusTagType` | 排队态返回 `info`（灰色） |
| `onDone` | 排队中取消的 done 事件走同一 done 处理 |
| 标题栏 | 新增并发占用展示 `并发 {running}/{max}`（调 `GetAiConcurrencyStatus`） |
| `onMounted` | 增 `EventsOn('ai-task:queued', onQueued)` 与 `'ai-task:started', onStarted)` |

### 4.6 tasks map 清理（独立项，建议同批）

`tasks` map 当前无清理入口，已完成任务对象永久驻留。此问题与并发上限无耦合（不占信号量槽位），但同属 `AiFunctionService` 的内存治理，建议同批补：

- `app.go` 新增 `RemoveAiTask(taskID)` 绑定，前端 Tab 关闭时调用
- `service` 层 `RemoveAiTask`：确认 `!task.running && !task.queued` 后 `delete(s.tasks, taskID)`
- 前端 `AiFunctionPanel.vue` Tab 关闭（`removeTab`）调 `RemoveAiTask`

> 运行中的任务 Tab 禁止关闭（关闭按钮置灰或点击提示「请先取消或等待完成」），避免误杀进行中的 claude 进程；已完成/已取消 Tab 可关闭，调 `RemoveAiTask` 清理。

### 4.7 并发状态查询

`app.go` 新增 `GetAiConcurrencyStatus()` 绑定，供前端标题栏展示：

```go
type AiConcurrencyStatus struct {
    Running int `json:"running"` // 运行中数量（已获取槽位）
    Queued  int `json:"queued"`  // 排队中数量
    Max     int `json:"max"`     // 并发上限
}

func (a *App) GetAiConcurrencyStatus() AiConcurrencyStatus {
    return a.aiFuncSvc.GetConcurrencyStatus()
}
```

`GetConcurrencyStatus` 持 `s.mu` 遍历 `tasks` 统计 `running && !queued` 与 `queued` 计数，`Max` 取 `cap(s.concurrencySem)`。

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `service/ai_function.go` | 修改 | struct 增 `concurrencySem`；`NewAiFunctionService` 初始化；`RunStage` 拆排队等待/执行两段，超时 ctx 起算点后移；`pumpOutput` defer 释放信号量；`CancelAiTask` 区分排队/运行两态；新增 `GetConcurrencyStatus` / `RemoveAiTask` |
| `model/ai_function.go` | 修改 | `AiTaskState` 增 `Queued`；新增 `AiConcurrencyStatus` |
| `app.go` | 新增绑定 | `GetAiConcurrencyStatus()` / `RemoveAiTask(taskID)` |
| `frontend/src/components/AiFunctionPanel.vue` | 修改 | `task` 增 `queued` 字段；`onQueued`/`onStarted` 事件监听；`statusText`/`statusTagType` 排队态；标题栏并发占用展示；Tab 关闭调 `RemoveAiTask` |
| `frontend/src/components/__tests__/AiFunctionPanel.spec.js` | 修改 | 排队态渲染、排队取消、并发占用展示测试 |
| `service/ai_function_test.go` | 修改 | 并发上限触发排队、排队取消释放位、超时起算后移、槽位释放测试 |
| `frontend/wailsjs/` | 同步 | `App.js` / `App.d.ts` 增 `GetAiConcurrencyStatus` / `RemoveAiTask` 绑定 |

## 6. 验收标准

1. 并发运行任务数不超过上限（默认 3），超限任务进入排队态，Tab 显示「排队中」
2. 排队任务在前序任务完成后自动获取槽位启动，Tab 转为「运行中」
3. 排队任务可取消，取消后立即从队列移除并让出排队位，Tab 显示「已取消」
4. 超时计时从获取槽位后起算，排队等待时间不计入执行预算
5. 进程退出即释放槽位，不依赖 `tasks` map 清理
6. 标题栏展示并发占用「N/M」，随任务起止实时更新
7. Tab 关闭调 `RemoveAiTask` 清理后端 runtime，`tasks` map 不无限增长
8. 应用关闭时排队任务清理，不残留僵尸条目

## 7. 决策结论

2026-09-08 评审确认，以下决策已定稿：

| 决策项 | 结论 | 依据 |
| --- | --- | --- |
| 默认并发上限值 | 3 | 兼顾本地资源（单进程 ~150-300MB，3 并发约 ~1GB）与 API 限流。先硬编码常量 `aiTaskMaxConcurrent = 3`，后续随 [[2026-09-08-ai-config-schema-design]] 的 schema v2 入配置文件可调 |
| 超限策略 | C 排队 + 状态可见 | 超限任务排队，emit `ai-task:queued`，Tab 灰色「排队中」，前序完成自动启动。A（阻塞等待，前端误导为运行中）与 B（直接拒绝，需手动重试）体验均劣于 C |
| 超时起算点 | 获取信号量后 | 排队等待不计入执行预算，排队用 `s.ctx` 响应 app 关闭与取消 |
| tasks map 清理 | 同批补 | 与并发上限无耦合（信号量随进程退出释放），但同属 `AiFunctionService` 内存治理，分批做会二次改动同一文件 |
| 运行中 Tab 关闭策略 | 禁止关闭运行中 Tab | 关闭按钮置灰或点击提示「请先取消或等待完成」，避免误杀进行中的 claude 进程。已完成/已取消 Tab 可关闭，调 `RemoveAiTask` 清理 |
| 多段编排处理 | 不特殊处理 | `RunAiFollowUp` 走 `RunStage` 同样占槽位，用户快速连点后续段会排队，此行为符合预期，无需特殊处理 |

## 8. 关联

- 来源：[[2026-09-08-ai-optimization-overview]] 第 7.1 节头脑风暴补充缺口
- 同批：[[2026-09-08-ai-task-observability-design]]（同处 `service/ai_function.go` 执行链路，第 1 批合并提交）
- 配套：[[2026-09-08-ai-config-schema-design]]（并发上限后续入 schema v2 可配置）
- 探查依据：`service/ai_function.go` 的 `RunStage`（:89）/ `pumpOutput`（:426）/ `CancelAiTask`（:142）、`AiFunctionService` struct（:25）、`AiFunctionPanel.vue` 的 `doRunMain`（:230）/ `onOutput`（:406）/ 事件注册（:515）
