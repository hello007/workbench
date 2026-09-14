# AI 结构化输出契约（--json-schema 方案）

> 跨层契约：AI 功能项结构化输出链路。新增需结构化输出的 AI skill（候选数组/问题清单/表单数据等）必知。

## 背景

WorkBench AI 功能（`AiFunctionService.RunStage`）起 claude 子进程走 `--output-format stream-json --verbose`，输出经 `ai-task:output` 流式推送 + `ai-task:done` 全量落盘。PR1 引入结构化输出需求（AI 提交信息生成要候选数组、AI 代码审查要问题清单 JSON），claude CLI 的 `--json-schema` flag 提供 tool use 强制结构化能力。

## 实测结论（2026-09-15，claude 2.1.158）

- `stream-json --verbose --json-schema '<schema>'` 组合通过：result 事件含 `structured_output` 字段，严格符合 schema
- `structured_output` 与自由文本 `result` **解耦**：自由文本带 markdown 包裹（如 `**7**`）不影响结构化输出正确性 —— 方案 A（prompt 约束 JSON + 后端容错解析）会被 markdown 包裹破坏，方案 C（`--json-schema`）不会
- stop_reason=`end_turn` 非 tool_use，structured_output 不依赖特定 stop_reason
- `--print` + `stream-json` 硬性要求 `--verbose`（`buildClaudeArgs` 已加）

## 跨层字段链路

```
AiFunction.OutputSchema (json.RawMessage, 配置层)
  ↓ buildClaudeArgs：OutputSchema 非 nil 追加 --json-schema <schema>
claude 子进程
  ↓ result 事件 structured_output 字段
streamEvent.StructuredOutput (json.RawMessage, service/ai_function.go)
  ↓ parseStreamLine 第 5 返回值
aiTaskRuntime.structuredOutput (service 缓存)
  ↓ pumpOutput 末尾构造 + GetAiTaskState 透传
AiTaskRunResult.StructuredOutput (done 事件 payload)
AiTaskState.StructuredOutput (GetAiTaskState 返回)
  ↓ Wails 事件/方法 IPC
frontend/wailsjs/go/models.ts: AiTaskState.structuredOutput (any)
wails-mock-defaults.js: done payload structuredOutput (测试单一数据源)
```

## 新增结构化 AI skill 步骤

1. **model**：`AiFunction.OutputSchema` 字段已存在（json.RawMessage，`omitempty`），配 JSON schema 字面量
2. **service**：`buildClaudeArgs` 自动追加 `--json-schema`（OutputSchema 非 nil），`parseStreamLine` 自动提取 `structured_output` —— **零新增 service 代码**
3. **wailsjs**：`AiFunction.outputSchema` / `AiTaskState.structuredOutput` 字段已同步（models.ts）；若新增 struct 字段须同步 `frontend/wailsjs/{App.js,App.d.ts,models.ts}` 三处（见 [cross-layer-contracts.md](cross-layer-contracts.md)）
4. **前端消费**：`AiFunctionPanel.vue` 的 `onDone` 读 `result.structuredOutput` 渲染结构化面板（按功能定制：候选列表 / 问题清单 / 表格）；**PR1 只搭链路不消费，消费在 PR2/PR3**
5. **测试**：`wails-mock-defaults.js` 的 done payload 加 `structuredOutput` 字段（单一数据源，vitest/E2E 共用）

## 约束

- `OutputSchema` 可选（nil 不加 `--json-schema` flag），兼容现有非结构化 skill 零回归
- `structured_output` 仅在 result 事件出现（流式过程无法增量渲染 JSON）—— 流式 `assistant` 文本增量照推前端展示进度，完成后渲染结构化面板
- `structured_output` 解析在后端（`parseStreamLine` 提取为 `json.RawMessage` 透传），前端只渲染不解析
- `structured_output` 为空（未配 OutputSchema 或 result 无该字段）时 `json.RawMessage` 为 nil，前端判空降级展示原始文本

## 关键文件

| 文件 | 符号 | 行 |
|---|---|---|
| model/ai_function.go | `AiFunction.OutputSchema` | struct 字段 |
| model/ai_function.go | `AiTaskRunResult.StructuredOutput` / `AiTaskState.StructuredOutput` | struct 字段 |
| service/ai_function.go | `buildClaudeArgs`（追加 `--json-schema`） | ~797 |
| service/ai_function.go | `streamEvent.StructuredOutput` / `parseStreamLine` 第 5 返回值 | ~989/1023 |
| service/ai_function.go | `aiTaskRuntime.structuredOutput` / `pumpOutput` 缓存 / `GetAiTaskState` 透传 | ~84/1191/497 |
| frontend/wailsjs/go/models.ts | `AiFunction.outputSchema` / `AiTaskState.structuredOutput` | 字段同步 |
| frontend/src/test/wails-mock-defaults.js | done payload `structuredOutput` | ~263 |

## 相关

- [cross-layer-contracts.md](cross-layer-contracts.md) — wailsjs 三处同步规则
- [logging-and-errors.md](logging-and-errors.md) — AI 功能错误链路
- research/claude-cli-structured-json.md — 三方案对比 + 实测证据
