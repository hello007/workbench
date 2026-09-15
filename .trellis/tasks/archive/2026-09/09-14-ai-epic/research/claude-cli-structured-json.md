# Research: Claude CLI 非交互模式结构化 JSON 输出可行性

- **Query**: claude CLI（Claude Code）非 interactive 模式能否直接输出结构化 JSON，避免前端从自由文本解析
- **Scope**: mixed（内部代码链路 + 外部 CLI 实测）
- **Date**: 2026-09-14
- **本机 CLI**: `claude --version` = `2.1.158 (Claude Code)`，经火山引擎代理走 glm-5（见实测 `modelUsage.glm-5`）

---

## 结论速览（TL;DR）

1. **推荐方案 C：`--json-schema` 强制结构化输出**。本机 2.1.158 已实测可用：`--output-format json --json-schema '<schema>'` 会在结果对象新增 `structured_output` 字段，内容严格符合 schema，机制为 tool use（function calling），可靠性远高于 prompt 约束。
2. **`--output-format json` 单独使用只给「JSON 信封 + 自由文本内容」**——`result` 字段仍是模型自由文本，并非结构化内容。单独用解决不了结构化需求，须配 `--json-schema` 或退回 prompt 约束。
3. **流式与结构化不冲突**：`stream-json` 模式下流式 `assistant` 文本增量照推前端展示进度，`structured_output` 只在最终 `result` 事件出现，完成后再渲染结构化面板。
4. **解析位置推荐后端**：在 `parseStreamLine` 提取 `structured_output`，经 `AiTaskRunResult` 新字段透传前端，前端只渲染不解析。

---

## 证据来源

| # | 来源 | 关键结论 |
|---|---|---|
| 1 | `claude --help` 本机输出（2.1.158）| `--output-format` 取值 `text`/`json`/`stream-json`（仅 `--print`）；`--json-schema <schema>` 存在且带示例；`--include-partial-messages` 仅 `--print`+`stream-json` |
| 2 | 实测 `claude -p ... --output-format json`（预算超限错误态）| stdout 单行 `{"type":"result","subtype":"error_max_budget_usd",...}`，确认 result 对象为单 JSON |
| 3 | 实测 `claude -p ... --output-format json --json-schema '{...}'`（成功态）| 结果含 `structured_output` 字段严格符合 schema；`num_turns=2`、经历 `stop_reason:"tool_use"` → 确认 function calling 机制 |
| 4 | 内部代码 `service/ai_function.go` + `model/ai_function.go` | 现有 `buildClaudeArgs` 已用 `--output-format stream-json --verbose`；`parseStreamLine` 解析 assistant/result 事件；`meeting-list` 已用 prompt 约束 + `extractTable` 解析 markdown 表格（即 Option B 既有实践） |
| 5 | Claude Code 官方文档 `docs.anthropic.com/en/docs/claude-code/cli-reference` | `--output-format` / `--json-schema` 为官方支持 flag（本会话未联网抓取页面，据 CLI help + 实测推断；确切最低引入版本未核验，见 Caveats） |

---

## Findings

### 1. claude CLI 原生结构化输出能力（核心）

#### 1.1 `--output-format` 三态（仅 `-p`/`--print` 模式）

CLI help 原文：

```
--output-format <format>   Output format (only works with --print):
                           "text" (default), "json" (single result),
                           or "stream-json" (realtime streaming)
                           (choices: "text", "json", "stream-json")
```

- `text`：纯文本（默认）
- `json`：**单行**最终结果 JSON 对象，进程结束后一次性输出（无流式增量）
- `stream-json`：逐行事件流（`assistant`/`result`/`system` 等），最终以 `result` 事件收尾

#### 1.2 `--json-schema <schema>` 强制结构化输出

CLI help 原文：

```
--json-schema <schema>   JSON Schema for structured output validation.
                         Example: {"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}
```

**实测结果对象（成功态，`--output-format json --json-schema`）**：

```json
{
  "type": "result",
  "subtype": "success",
  "is_error": false,
  "api_error_status": null,
  "duration_ms": 117046,
  "duration_api_ms": 97963,
  "ttft_ms": 22782,
  "num_turns": 2,
  "result": "你好，我是CavecrewBuilder。已选择数字7。",
  "stop_reason": "end_turn",
  "session_id": "7b7d4d80-b21c-46de-95e1-c615bb5b7352",
  "total_cost_usd": 0.303605,
  "usage": {
    "input_tokens": 60266,
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "output_tokens": 91,
    "server_tool_use": { "web_search_requests": 0, "web_fetch_requests": 0 },
    "service_tier": "standard",
    "cache_creation": { "ephemeral_1h_input_tokens": 0, "ephemeral_5m_input_tokens": 0 }
  },
  "modelUsage": {
    "glm-5": {
      "inputTokens": 60266, "outputTokens": 91,
      "cacheReadInputTokens": 0, "cacheCreationInputTokens": 0,
      "webSearchRequests": 0, "costUSD": 0.303605,
      "contextWindow": 200000, "maxOutputTokens": 32000
    }
  },
  "permission_denials": [],
  "structured_output": { "message": "Hi. Picked 7.", "score": 7 },
  "terminal_reason": "completed",
  "fast_mode_state": "off",
  "uuid": "b1c52055-2ee8-4e61-a94d-8fc4e23ca959"
}
```

**关键观察**：

- `structured_output` 字段 **仅在传了 `--json-schema` 时出现**，内容严格符合所给 schema（`{"message":string,"score":integer}`）。
- `result` 字段仍是模型自由叙述（本次甚至偏离了 prompt 的 "say hi"，但 `structured_output` 仍正确）——**证明结构化输出与自由文本解耦，schema 约束不依赖模型叙述自觉性**。
- `num_turns=2`、`stop_reason` 链路经历 `tool_use` → `end_turn`：**机制是 function calling**（CLI 注入一个参数 schema 匹配的 tool，强制模型以 tool_use 产出结构化参数，CLI 抽取参数填入 `structured_output`）。
- 错误态（预算超限）结果对象结构略有差异：无 `ttft_ms`/`structured_output`/`terminal_reason`，多 `errors:["Reached maximum budget ($0.05)"]`，`subtype:"error_max_budget_usd"`，`is_error:true`。

#### 1.3 `--output-format json`（无 schema）单独使用的局限

无 `--json-schema` 时，`result` 字段是模型自由文本。即「JSON 信封包裹自由文本内容」，**不是结构化内容**。要拿结构化数据仍须 prompt 约束模型把 `result` 输出成 JSON 文本再 parse——可靠性退化为 Option B 水平。故 Option A 单独不成立，须与 B 或 C 组合。

### 2. 现有 RunStage 链路（内部代码）

| 文件:行 | 符号 | 现状 |
|---|---|---|
| `service/ai_function.go:792` | `buildClaudeArgs` | 组装 `-p <prompt> [--resume <sid>] --output-format stream-json --verbose [--permission-mode] [--add-dir]... [--mcp-config] [--settings]` |
| `service/ai_function.go:338` | `RunStage` | 起子进程、StdoutPipe、`pumpOutput` 异步泵 |
| `service/ai_function.go:1162` | `pumpOutput` | 逐行 `bufio.Scanner`（单行上限 4MB），调 `parseStreamLine`，文本增量写文件 + emit `ai-task:output`，result 事件取 metrics，末尾 emit `ai-task:done` |
| `service/ai_function.go:1012` | `parseStreamLine` | 解析 `streamEvent`：`assistant` 事件抽 `message.content[].text` 为文本增量；`result` 事件抽 `duration_ms`/`num_turns`/`total_cost_usd`/`usage` 为 metrics |
| `service/ai_function.go:978` | `streamEvent` struct | 已定义 `Type/Subtype/SessionID/Result/Message/IsError/DurationMs/NumTurns/TotalCostUSD/Usage`——**未定义 `StructuredOutput` 字段**，需新增 |
| `service/ai_function.go:1062` | `extractTable` | 预解析 markdown 表格 → `MeetingTable`，供 `meeting-list` 的 prompt 约束表格输出后端解析（Option B 既有实践） |
| `model/ai_function.go:127` | `AiTaskRunResult` | done 事件 payload，含 `Output`(预览)/`OutputSize`/`OutputFile`/`TableExtracted`/`Metrics`——**无结构化输出字段** |
| `model/ai_function.go:5` | `AiFunction` | 配置项 struct，**无 `OutputSchema`/`OutputFormat` 字段** |

事件流：`ai-task:queued` → `ai-task:started` → `ai-task:output`（流式文本增量）→ `ai-task:archived` → `ai-task:done`（终态 result）。

`meeting-list` 默认功能项（`ai_function.go:1348`）的 `PromptTemplate` 已用 prompt 约束："以 markdown 表格输出，固定列顺序：会议主题|会议号|...表格之外不要输出其他说明文字"——**这是 Option B 的现网实践**，后端 `extractTable` 兜底解析。可作为 Option B 可行但需兜底的佐证。

### 3. 三方案对比

| 维度 | A: `--output-format json` 单独 | B: prompt 约束 JSON | C: `--json-schema` 强制 schema |
|---|---|---|---|
| **可行性** | 可（但只给信封） | 可 | 可，已实测 |
| **内容结构化** | 否（`result` 仍自由文本） | 靠模型自觉 | 是（`structured_output` 严格符合 schema） |
| **可靠性** | 低（等同 B） | 中（约 80-90%） | 高（CLI 校验 + tool_use 强制） |
| **典型失败模式** | `result` 非 JSON | ```json 代码块包裹、前后赘述、字段缺失、多余解释 | schema 非法时 CLI 启动报错；模型极少数 tool_use 失败（实测未遇） |
| **失败兜底** | 同 B | 须前端/后端容错 JSON parse（剥 markdown 包裹、截取首个 `{` 到末个 `}`） | 基本无须兜底；`structured_output` 缺失则降级文本视图 |
| **改造成本** | 低（换 flag） | 零 flag（prompt 工程 + 解析兜底） | 中（详见下方适配清单） |
| **流式 UX** | 无（单 result，无增量） | 可流式（自由文本增量） | 可流式（`assistant` 文本增量展示进度）+ 完成后结构化面板 |
| **版本依赖** | 2.x 起 | 无 | 需 `--json-schema` 支持（2.1.158 确认） |
| **适配新功能** | 不够 | 够但脆，问题清单缺字段即渲染崩 | 最佳 |

### 4. 推荐方案与理由

**推荐 C（`--json-schema`）用于 PR2 提交信息生成 + PR3 代码审查两功能。**

理由：

1. **可靠性是结构化渲染的硬约束**。代码审查问题清单含 `file/line/level/category/description/suggestion` 六字段，前端按级别分组 + 行号跳转——任一字段缺失即渲染异常。prompt 约束（B）的 ```json 包裹/字段缺失风险不可接受。实测 C 的 `structured_output` 严格符合 schema，即便模型自由叙述偏离 prompt 也不影响结构化结果。
2. **CLI 已原生支持**，无须前端从自由文本解析，符合任务诉求"避免前端从自由文本解析"。
3. **`structured_output` 与自由文本 `result` 解耦**，流式期间仍可推 `assistant` 文本增量展示进度，完成后渲染结构化面板，UX 不退化。
4. **机制是 function calling**，是模型强项，glm-5 实测可用（`stop_reason: tool_use` 正常触发）。
5. **与现有架构契合**：`buildClaudeArgs` 已组装 flag 列表，加一个 `--json-schema` 是最小增量；`parseStreamLine` 已处理 `result` 事件，加 `structured_output` 提取是同点扩展。

#### 4.1 建议的 JSON Schema

**PR2 提交信息生成（2-3 候选）**：

```json
{
  "type": "object",
  "properties": {
    "candidates": {
      "type": "array",
      "minItems": 2,
      "maxItems": 3,
      "items": {
        "type": "object",
        "properties": {
          "type": { "type": "string", "enum": ["feat","fix","docs","style","refactor","perf","test","chore","build","ci"] },
          "scope": { "type": "string" },
          "description": { "type": "string" }
        },
        "required": ["type", "description"]
      }
    }
  },
  "required": ["candidates"]
}
```

**PR3 代码审查（问题清单）**：

```json
{
  "type": "object",
  "properties": {
    "issues": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "file": { "type": "string" },
          "line": { "type": "integer" },
          "level": { "type": "string", "enum": ["critical","warning","info"] },
          "category": { "type": "string", "enum": ["bug","convention","security","performance","improvement"] },
          "description": { "type": "string" },
          "suggestion": { "type": "string" }
        },
        "required": ["file", "level", "category", "description"]
      }
    }
  },
  "required": ["issues"]
}
```

> schema 设计原则：`required` 精简到渲染必需字段（`suggestion`/`scope`/`line` 可选），`enum` 收敛分类，避免复杂嵌套致模型偶发违反。

### 5. 现有 RunStage 链路适配清单（点 5）

推荐**后端提取 `structured_output` 透传前端，前端只渲染不解析**。

| 步骤 | 文件 | 改动 |
|---|---|---|
| 1 | `model/ai_function.go` `AiFunction` | 加字段 `OutputSchema json.RawMessage json:"outputSchema,omitempty"`（存 JSON Schema 文本，nil 表示不约束）。须同步 `frontend/wailsjs/` 三处（`App.js`/`App.d.ts`/`models.ts`，见 cross-layer-contracts.md） |
| 2 | `model/ai_function.go` `AiTaskRunResult` + `AiTaskState` | 加字段 `StructuredOutput json.RawMessage json:"structuredOutput,omitempty"`（透传给前端） |
| 3 | `service/ai_function.go` `streamEvent` struct | 加字段 `StructuredOutput json.RawMessage json:"structured_output"`（result 事件携带） |
| 4 | `service/ai_function.go` `parseStreamLine` | result 事件分支：当 `ev.StructuredOutput` 非空时返回给调用方（扩展返回值或挂到 metrics 同级的独立出参） |
| 5 | `service/ai_function.go` `aiTaskRuntime` | 加字段 `structuredOutput json.RawMessage` 缓存，pumpOutput 末尾填入 `AiTaskRunResult.StructuredOutput` |
| 6 | `service/ai_function.go` `buildClaudeArgs` | 当 `fn.OutputSchema` 非空时追加 `--json-schema <string(fn.OutputSchema)>`；保持 `--output-format stream-json`（流式 UX 保留） |
| 7 | `frontend/src/components/AiFunctionPanel.vue` | `onDone` 时若 `result.structuredOutput` 非空，按功能 `functionId`/`outputSchema` 渲染结构化面板（候选列表 / 问题清单）；否则退回现有文本/表格视图 |

**为何后端解析而非前端**：

- 后端已在 `parseStreamLine` 解析 stream-json，同点提取 `structured_output` 是最小扩展。
- 经 Wails 绑定传 pre-parsed raw JSON（`json.RawMessage` 透传为前端 JSON 对象），前端无须容错 JSON parse、无须处理 markdown 包裹。
- schema 校验可选在后端做（CLI 已校验，后端可不再重复）。
- 前端按功能 contract 渲染（commit 候选 vs review 问题），逻辑清晰。

### 6. 流式输出与结构化冲突评估（点 4）

- **流式过程**：`assistant` 事件文本增量照推 `ai-task:output`，展示模型自由叙述（进度感）。`structured_output` **不在流式增量中**，只在最终 `result` 事件出现。
- **完成后**：`ai-task:done` 携带 `structured_output`，前端渲染结构化面板。
- **取舍**：流式展示原始叙述 vs 完成后结构化面板——两者互补不冲突。对 commit-message/review，用户核心价值在结构化结果，流式叙述仅作进度指示。实测 `--json-schema` 模式下模型自由叙述很少（`result` 仅一句、`output_tokens=91`），流式期间面板可显示"生成中…"骨架，完成后切换结构化视图。
- **不建议切 `--output-format json`（单 result）**：会丢失流式 UX，且单 result 须等进程结束才输出，长任务（代码审查多文件）用户无进度反馈。保持 `stream-json` + 末尾提取 `structured_output` 最优。

### 7. 版本依赖（点 6）

| 项 | 状态 |
|---|---|
| 本机 `claude --version` | `2.1.158` |
| `--output-format json/stream-json` | help 列出，实测可用 |
| `--json-schema` | help 列出，实测可用（`structured_output` + `tool_use` 确认） |
| 确切最低引入版本 | **未联网核验**（本会话未抓取 docs.anthropic.com CLI reference 页面）。据 CLI help 与实测，2.1.158 确定支持。建议实现期在启动检测或 skill 配置加 `claude --version` 兜底校验，低于阈值提示用户升级 |
| WorkBench 兼容性 | 当前开发机已兼容；分发用户机若 CLI 版本过旧可能不支持 `--json-schema`，须 README/部署说明提示 |

---

## Related Specs

- `docs/spec/cross-layer-contracts.md` — `AiFunction` 加字段须同步 `frontend/wailsjs/` 三处（App.js/App.d.ts/models.ts）
- `docs/spec/app-services-assembly.md` — service 层改动遵循 AppServices 装配规则
- `docs/spec/logging-and-errors.md` — schema 非法/CLI 报错的错误码分流
- `.trellis/tasks/09-14-ai-epic/prd.md` — epic 需求，PR2 候选提交信息 / PR3 代码审查问题清单

---

## Caveats / Not Found

1. **`--output-format stream-json --json-schema` 组合未单独实测**。本研究只实测了 `--output-format json --json-schema`（单 result）。stream-json 的最终 `result` 事件结构应与单 result 一致（含 `structured_output`），但实现期建议补一次实测确认 `structured_output` 在 stream-json result 事件中确实存在。
2. **官方文档未联网核验**。本会话无 web search 工具，未抓取 `docs.anthropic.com/en/docs/claude-code/cli-reference` 原文。`--json-schema` 确切最低引入版本、`structured_output` 字段是否为官方稳定契约（vs 随版本变更），须实现期联网核验或查 GitHub anthropics/claude-code 仓库 release notes。
3. **模型 provider 差异**。本机 claude 经火山引擎代理走 glm-5（实测 `modelUsage.glm-5`），非官方 Anthropic 模型。`--json-schema` 的 tool_use 机制在 glm-5 上已实测可用，但用户机若换 provider（官方 Anthropic / Bedrock / Vertex）tool_use 行为应一致（function calling 为通用能力），仍建议多 provider 冒烟。
4. **实测单次成本 ~$0.30**（60K input tokens，系统提示词占比大；diff 注入后 token 会进一步增加）。生产链路须配合 PR1 的 diff 截断保护控成本。
5. **`--json-schema` schema 合法性**。schema 须为合法 JSON Schema；复杂 schema（深嵌套/oneOf/多 enum）模型偶有违反，建议 schema 简洁、`required` 精简、`enum` 收敛（见 4.1 设计原则）。
6. **`--include-partial-messages`**（流式 partial 文本块）与 `structured_output` 关系未实测；当前架构用完整 `assistant` 事件文本增量，无须 partial，不建议引入。
