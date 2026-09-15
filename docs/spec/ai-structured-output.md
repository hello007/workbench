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
6. **seed skill 合并白名单**：新增 skill 须在 `mergeMissingSeedSkills`（service/ai_function.go）的白名单数组加入其 ID，否则 PR1 前已建 `data/ai_functions.json` 的老用户配置不含该 skill，`RunAiFunction` 报「AI 功能 <id> 不存在」。详见下方「seed skill 合并白名单」节。

## seed skill 合并白名单

### 背景

`defaultAiFunctions()` 内置 skill 配置（speech-doc / weekly-report / meeting-book / meeting-list / commit-message / code-review）。`LoadAiFunctions` 对已存在配置走 `migrateFunctions`（补字段）+ `validateFunctions`（校验），但**不按 ID 合并新 seed skill**。PR1 前已建 `data/ai_functions.json`（仅含 4 项原 skill）的用户升级后，`commit-message` / `code-review` 缺失，`RunAiFunction` 报「AI 功能 commit-message 不存在」。

### 契约

- 新增 seed skill **必须**加入 `mergeMissingSeedSkills` 的白名单数组，否则老用户配置不合并
- 白名单仅含 epic 交付物：`["commit-message", "code-review"]`
- **旧 seed skill（speech-doc / weekly-report / meeting-book / meeting-list）不进白名单** —— 用户可能故意删除，不加回尊重用户删除决策
- 用户已存在同 ID 项（含自定义 Name/Params）**不覆盖**，尊重用户自定义
- seed 按 ID 索引 `map[string]*model.AiFunction` O(1) 查表，非线性扫 `defaultAiFunctions`
- 合并后 `changed=true`，`LoadAiFunctions` 落盘条件 `migrated || len(invalidIDs) > 0 || seedMerged` 触发落盘，下次加载直读

### 签名

```go
// mergeMissingSeedSkills 按 ID 合并白名单内缺失的 seed skill。
// 白名单 = epic 交付物（commit-message/code-review）。
// 用户已存同 ID 项不覆盖；旧 seed skill 不进白名单尊重删除决策。
// 返回合并后的 funcs 与是否发生合并。
func mergeMissingSeedSkills(funcs []*model.AiFunction) ([]*model.AiFunction, bool)
```

### 验证矩阵

| 条件 | 行为 | changed |
|---|---|---|
| 老配置（4 项原 skill，缺 epic skill） | 追加 commit-message/code-review | true |
| 全量配置（6 项） | 不变 | false |
| 用户删了 meeting-book（5 项含 epic skill） | meeting-book 不加回 | false |
| 用户自定义 commit-message（ID 同字段改） | 不覆盖 | false |
| 部分缺项（有 commit-message 缺 code-review） | 仅补 code-review | true |

### Wrong vs Correct

#### Wrong
```go
// 全量同步 defaultAiFunctions —— 用户故意删过的 skill 被加回
func mergeMissingSeedSkills(funcs []*model.AiFunction) ([]*model.AiFunction, bool) {
    defaults := defaultAiFunctions()
    // 遍历 defaults 全追加缺失项，无视用户删除决策
}
```

#### Correct
```go
// 白名单仅 epic 交付物，尊重用户删除/自定义
var seedMergeWhitelist = []string{"commit-message", "code-review"}

func mergeMissingSeedSkills(funcs []*model.AiFunction) ([]*model.AiFunction, bool) {
    existing := make(map[string]bool, len(funcs))
    for _, fn := range funcs {
        if fn != nil && fn.ID != "" {
            existing[fn.ID] = true
        }
    }
    seedByID := make(map[string]*model.AiFunction)
    for _, fn := range defaultAiFunctions() {
        seedByID[fn.ID] = fn
    }
    changed := false
    for _, id := range seedMergeWhitelist {
        if !existing[id] {
            if seed, ok := seedByID[id]; ok {
                funcs = append(funcs, seed)
                changed = true
            }
        }
    }
    return funcs, changed
}
```

### 测试

- `service/ai_function_seed_merge_test.go`：6 单测覆盖验证矩阵全 5 行 + 端到端集成（老 4 项配置经 `LoadAiFunctions` → 6 项 + 落盘 + 二次加载稳定）
- 断言点：合并后 funcs 含白名单 skill、用户自定义项原字段未变、meeting-book 未加回、`changed` 返回值正确

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
| service/ai_function.go | `mergeMissingSeedSkills`（seed skill 合并白名单） | ~304 |
| service/ai_function.go | `aiTaskRuntime.structuredOutput` / `pumpOutput` 缓存 / `GetAiTaskState` 透传 | ~84/1191/497 |
| frontend/wailsjs/go/models.ts | `AiFunction.outputSchema` / `AiTaskState.structuredOutput` | 字段同步 |
| frontend/src/test/wails-mock-defaults.js | done payload `structuredOutput` | ~263 |

## 相关

- [cross-layer-contracts.md](cross-layer-contracts.md) — wailsjs 三处同步规则
- [logging-and-errors.md](logging-and-errors.md) — AI 功能错误链路
- research/claude-cli-structured-json.md — 三方案对比 + 实测证据
