# AI 任务大输出内存积累治理设计

**日期**：2026-09-08
**优先级**：P0（3.1 前端截断，与 [[2026-09-08-ai-task-observability-design]] 同批）/ P1（3.3 流式文件，随 [[2026-09-08-ai-run-history-design]]）
**状态**：决策已定，待实施（2026-09-08 评审确认）

## 1. 概述

治理 `claude -p` 子进程输出在「后端内存 → IPC 事件 → 前端累加 → DOM 渲染」全链路的内存积累与渲染卡顿。分三层独立落地：前端末尾窗口截断（3.1，提前到 P0-2 同批）、事件时间窗口合并（3.2，视实测定）、后端流式写文件（3.3，随 P1-2 历史归档）。三层解耦，可分批提交，3.3 与历史归档 `os.Rename` 零拷贝衔接。

## 2. 背景与痛点

输出数据全链路三段，每段均全量驻留：

| 环节 | 位置 | 机制 | 痛点 |
| --- | --- | --- | --- |
| 后端写入 | `pumpOutput`（service/ai_function.go:426） | `task.output.WriteString(text)`，`strings.Builder` 全量驻留 | 单行上限 4MB（`scanner.Buffer`），整段输出无上限 |
| 后端读取 | `GetAiTaskState`（service/ai_function.go:170）/ `AiTaskRunResult.Output` | `task.output.String()` 一次性全量拷贝 | 每次调用全量复制一份 |
| 事件推送 | `emit("ai-task:output", {text})` | 每个文本片段一个 IPC 事件 | 大输出时事件密集，每个 text 经 Wails 序列化 |
| 前端累加 | `onOutput`（AiFunctionPanel.vue:409）`t.output += ev.text` | 字符串拼接，Vue 响应式追踪 | 每次 `+=` 创建新字符串，O(n) 累加 |
| 前端渲染 | `<pre>{{ t.output }}</pre>`（AiFunctionPanel.vue:151） | 纯文本插值，无虚拟滚动 | 每次 `t.output` 变化重渲整个文本节点；`scrollOutput` 强制布局 |
| 前端副本 | `onDone` 后 `result.output` 与 `t.output` 并存 | 完成时全量再塞一份 | 两份大字符串同驻 |

一条 N MB 输出，后端内存一份 + 事件序列化 N 次 + 前端内存两份 + 浏览器对超大 `<pre>` 的 reflow。`agree-slides` 类 skill 生成完整幻灯片源码（实测可达数 MB 到数十 MB）时，单任务即可让前端卡顿。

| 痛点 | 说明 |
| --- | --- |
| 前端渲染卡顿 | `t.output += ev.text` 触发 `<pre>` patch，5MB 文本节点 reflow + `scrollOutput` 强制布局，流式过程持续掉帧 |
| 后端内存峰值 | `strings.Builder` 全量驻留，完成时 `String()` 再拷贝一份瞬态 |
| IPC 事件洪流 | stream-json 逐片段推送，数千事件经 Wails 序列化 |
| 前端双份驻留 | `t.output` 与 `result.output` 在 `onDone` 后并存 |

## 3. 需求总结

1. 前端展示截断到末尾窗口，大输出不卡顿
2. 截断后顶部提示省略量与查看完整输出入口
3. copy/preview/表格视图等完成动作取全量数据，不受截断影响
4. 后端输出流式写文件，`strings.Builder` 不再全量驻留
5. `GetAiTaskState` 返回尾部预览与大小，不全量拷贝
6. 新增全量读取入口，供 copy/preview/表格视图按需拉取
7. 3.3 输出文件与 P1-2 历史归档零拷贝衔接（`os.Rename`）

## 4. 设计

### 4.1 3.1 前端末尾窗口截断（P0，独立可做）

`onOutput` 累加时设上限，超出只保留末尾窗口，顶部提示省略量：

```js
const MAX_DISPLAY = 256 * 1024 // 展示末 256KB

const onOutput = (ev) => {
  const t = tasks.value.find((x) => x.taskId === ev.taskId)
  if (!t) return
  t.output += ev.text || ''
  if (t.output.length > MAX_DISPLAY) {
    const omittedKB = Math.floor((t.output.length - MAX_DISPLAY) / 1024)
    t.output = '…（已省略前 ' + omittedKB + ' KB，完整内容可复制或查看历史）\n' + t.output.slice(-MAX_DISPLAY)
    t.truncated = true
  }
  scrollOutput()
}
```

`<pre>` 渲染 `t.output`，超出时顶部提示省略 KB 数。`task` 对象增 `truncated: false` 初值。

**截断后完成动作的取数改造**：`t.output` 已被截断，`copy`/`preview`/`meetingTable` 不再取它，改调后端全量读取（3.3 的 `GetAiTaskOutput`；3.3 落地前，3.1 阶段临时保留 `result.output` 全量供完成动作使用，即 `onDone` 时 `t.fullOutput = result.output`，完成动作取 `t.fullOutput`，展示取 `t.output`）。

| 项 | 说明 |
| --- | --- |
| 末尾窗口大小 | 256KB（可配置，默认值，实测 agree-slides 输出后调） |
| 截断提示 | 顶部「已省略前 N KB，完整内容可复制或查看历史」 |
| `truncated` 标记 | 供 UI 判断是否展示「查看完整输出」按钮 |
| 3.1 阶段临时方案 | `onDone` 存 `t.fullOutput = result.output`，完成动作取它；3.3 落地后改为调 `GetAiTaskOutput` |

### 4.2 3.2 事件时间窗口合并（视实测定）

后端 `pumpOutput` 不再每片段即 emit，按时间窗口合并批量推送：

| 项 | 说明 |
| --- | --- |
| 缓冲字段 | `aiTaskRuntime` 增 `bufferedText strings.Builder` + `lastFlush time.Time` |
| 合并窗口 | 30ms（兼顾流畅与 IPC 压力，过长致打字效果变「批次出现」） |
| flush 机制 | 启动定时 flush goroutine，每 30ms 若 `bufferedText` 非空则持锁取快照、锁外 emit 并清空 |
| 最终 flush | 进程结束（`cmd.Wait()` 返回）后保证最终 flush，不丢尾部数据 |
| 持锁安全 | flush 须持 `s.mu` 取 `bufferedText` 快照再 emit（锁外），避免与写入竞态 |

**实施前提**：3.1 落地后实测 IPC 压力。若 256KB 截断后前端渲染已流畅、IPC 次数可接受，则 3.2 可不做；若仍卡，再做。

### 4.3 3.3 后端流式写文件（P1，随 P1-2）

`aiTaskRuntime.output` 从 `strings.Builder` 改为 `*os.File` + `outputSize int64`：

```go
type aiTaskRuntime struct {
    // ... 现有字段
    outputFile *os.File // 新增：写入 data/ai_task_output/<taskID>.txt
    outputSize int64    // 新增：累计字节数（持锁更新，供展示/上限判断）
}
```

`RunStage` 起进程前创建输出文件，`pumpOutput` 中 `text` 直接 `task.outputFile.WriteString(text)` + `task.outputSize += int64(len(text))`（持 `s.mu`）。

`GetAiTaskState` 不再返回全量 `Output`，改为返回尾部预览：

```go
type AiTaskState struct {
    // ... 现有字段
    Output     string `json:"output"`     // 语义改为末尾 ~4KB 预览（readTail）
    OutputSize int64  `json:"outputSize"` // 新增：完整字节数
    OutputFile string `json:"outputFile"` // 新增：文件相对路径（供前端拉全量）
}
```

新增 `GetAiTaskOutput(taskID) (string, error)` 绑定，流式读文件返回全量（供 copy/preview/表格视图）。copy 完成动作优先走后端直接写系统剪贴板（避免大字符串过 IPC），3.3 阶段评估是否引入 `golang.design/x/clipboard`。

**与 P1-2 零拷贝衔接**：输出文件已在 `data/ai_task_output/<id>.txt`，P1-2 归档时 `os.Rename` 移到 `data/ai_task_history/<id>.txt`，元数据只存 `OutputFile` 路径与 `OutputSize`，无大对象拷贝。

### 4.4 表格视图兜底（隐藏难点）

`meetingTable`（AiFunctionPanel.vue:280 解析 markdown 表格）依赖从 `t.output` 解析表格，前端截断后表格可能丢失在省略区。两种兜底方案：

| 方案 | 说明 | 取舍 |
| --- | --- | --- |
| A. 后端预解析 | `GetAiTaskState` 额外返回 `TableExtracted` 预解析结果（表格行/表头），前端表格视图直接用 | 后端解析一次，省前端每次渲染解析；但 `GetAiTaskState` 职责膨胀 |
| B. 表格视图触发全量读 | 表格视图渲染时调 `GetAiTaskOutput` 拉全量再解析 | `GetAiTaskState` 职责单纯；但表格视图首次渲染有全量读取延迟 |

**推荐 A**：后端预解析。表格解析逻辑本在后端测试覆盖更稳，且避免表格视图每次渲染重复解析。`AiTaskState` 增 `TableExtracted *MeetingTable` 字段（`nil` 表示无表格）。

### 4.5 输出文件清理

3.3 阶段 `data/ai_task_output/` 会堆积文件，清理策略：

| 触发 | 行为 |
| --- | --- |
| Tab 关闭 | 调 `RemoveAiTask`（与 [[2026-09-08-ai-concurrency-control-design]] 4.6 同入口），删后端 runtime + 输出文件 |
| P1-2 归档接管 | 任务完成后归档流程 `os.Rename` 移走，原目录无残留 |
| P1-2 落地前兜底 | 定时清理超 1 天的 `ai_task_output` 文件（防孤儿） |

## 5. 改动范围

按三层拆分，分批提交：

| 层 | 文件 | 改动 | 时机 |
| --- | --- | --- | --- |
| 3.1 前端截断 | `frontend/src/components/AiFunctionPanel.vue` | `onOutput` 上限截断 + `truncated` 标记 + `<pre>` 顶部提示；`onDone` 存 `t.fullOutput`；`copyOutput`/`previewOutput`/`meetingTable` 改取 `t.fullOutput` | 第 1 批（P0-2 同批） |
| 3.1 测试 | `frontend/src/components/__tests__/AiFunctionPanel.spec.js` | 截断触发、提示文案、完成动作取全量测试 | 第 1 批 |
| 3.2 事件合并 | `service/ai_function.go` | `aiTaskRuntime` 增缓冲字段；`pumpOutput` 改批量 flush；新增 flush goroutine | 视实测 |
| 3.2 测试 | `service/ai_function_test.go` | 合并窗口、最终 flush、并发安全测试 | 视实测 |
| 3.3 流式文件 | `model/ai_function.go` | `AiTaskState`/`AiTaskRunResult` 增 `OutputSize`/`OutputFile`，`Output` 语义改预览；增 `TableExtracted` | 第 3 批（P1-2 同批） |
| 3.3 流式文件 | `service/ai_function.go` | `outputFile` 替换 `strings.Builder`；`pumpOutput` 写文件；`GetAiTaskState` 读尾部 + 预解析表格；新增 `GetAiTaskOutput`/`RemoveAiTask`（与缺口 1 共用） | 第 3 批 |
| 3.3 流式文件 | `app.go` | 新增 `GetAiTaskOutput(taskID)` 绑定 | 第 3 批 |
| 3.3 流式文件 | `frontend/src/components/AiFunctionPanel.vue` | `copyOutput`/`previewOutput`/`meetingTable` 改调 `GetAiTaskOutput` 全量读取 | 第 3 批 |
| 3.3 流式文件 | `frontend/wailsjs/` | 同步 `GetAiTaskOutput` 绑定 | 第 3 批 |
| 3.3 测试 | `service/ai_function_test.go` | 输出文件写入、尾部读取、大小统计、表格预解析测试 | 第 3 批 |

## 6. 验收标准

### 3.1 前端截断

1. 输出超 256KB 时前端只保留末尾窗口，顶部提示省略 KB 数
2. 截断后 `<pre>` 渲染流畅，流式过程无持续掉帧
3. `copy`/`preview`/`meetingTable` 取 `t.fullOutput` 全量，不受截断影响
4. `truncated` 标记正确，UI 据此展示「查看完整输出」入口

### 3.3 流式文件

5. 后端输出流式写 `data/ai_task_output/<id>.txt`，`strings.Builder` 不再全量驻留
6. `GetAiTaskState` 返回尾部预览 + `OutputSize` + `OutputFile`，不全量拷贝
7. `GetAiTaskOutput` 全量读取正确，供 copy/preview/表格视图按需拉取
8. 表格视图经后端预解析 `TableExtracted` 渲染，不受截断影响
9. 输出文件 `os.Rename` 零拷贝衔接 P1-2 归档，无大对象拷贝
10. Tab 关闭 / 归档接管 / 定时清理三路径同步删文件，无孤儿

## 7. 决策结论

2026-09-08 评审确认，以下决策已定稿：

| 决策项 | 结论 | 依据 |
| --- | --- | --- |
| 三层分批 | 3.1 第 1 批（P0-2 同批）；3.2 视 3.1 后实测；3.3 第 3 批（P1-2 同批） | 3.1 零后端改动立即治标；3.3 与归档零拷贝衔接须随 P1-2 |
| 末尾窗口大小 | 256KB，可配置 | 太小表格易丢，太大仍卡；默认 256KB，实测 agree-slides 输出后调 |
| 事件合并窗口 | 30ms | 兼顾流畅与 IPC 压力，过长致打字效果变批次出现；3.1 后实测决定是否做 |
| copy 全量读取 | 3.1 阶段 `t.fullOutput`；3.3 阶段 `GetAiTaskOutput` | 3.1 不依赖后端改动；3.3 后端流式读文件，评估引入 `golang.design/x/clipboard` 后端直接写剪贴板省 IPC |
| 表格视图兜底 | 后端预解析 `TableExtracted`（方案 A） | 后端解析一次比前端每次渲染解析省，且测试覆盖更稳 |
| 输出文件清理 | Tab 关闭调 `RemoveAiTask` + P1-2 归档 `os.Rename` 接管 + 定时清理兜底 | 三路径同步删文件，避免孤儿；`RemoveAiTask` 与 [[2026-09-08-ai-concurrency-control-design]] 共用入口 |

## 8. 关联

- 来源：[[2026-09-08-ai-optimization-overview]] 第 7.2 节头脑风暴补充缺口
- 同批：[[2026-09-08-ai-task-observability-design]]（3.1 第 1 批同处 `AiFunctionPanel.vue`）；[[2026-09-08-ai-run-history-design]]（3.3 第 3 批零拷贝衔接归档）
- 共用：[[2026-09-08-ai-concurrency-control-design]]（`RemoveAiTask` 入口共用，tasks map 清理同批）
- 探查依据：`service/ai_function.go` 的 `pumpOutput`（:426）/ `GetAiTaskState`（:157）/ `scanner.Buffer`（4MB 单行上限）、`model/ai_function.go` 的 `AiTaskState`（:80）/ `AiTaskRunResult`（:70）、`AiFunctionPanel.vue` 的 `onOutput`（:409）/ `onDone`（:413）/ `<pre>` 渲染（:151）/ `meetingTable`（:280）/ `copyOutput`（:449）/ `previewOutput`（:460）
