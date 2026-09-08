# AI 功能配置高级字段表单化 + MCP stdio 支持（第 2 批）

## Goal

将 `AiFunctionConfigDialog.vue` 中仍以裸 JSON textarea 编辑的 `params`/`followUps`/`env`/`mcp` 四块高级字段改为结构化表单（4 个子组件 + 折叠面板 + 原始 JSON 兜底视图），并扩展 `AiMcpServer` 支持 `stdio` 类型，解除当前仅 `http` 类型的限制。合并实施 P0-1（配置表单化）与 P2-1（MCP stdio），二者共用 McpEditor 组件，合并避免返工。

## What I already know

### 第 1 批已完成的衔接点（勿重复改动）

- `AiFunction`/`AiTaskState` 等 model 已扩展 `metrics`/`queued` 等字段，本批不动这些
- `AiFunctionPanel.vue` 已有 `queued`/`metrics`/`truncated`/`fullOutput`，本批只改 `AiFunctionConfigDialog.vue` 及新增子组件
- `parseStreamLine`/`concurrencySem` 等执行链路本批不碰

### 当前实现现状（代码探查）

- `AiFunctionConfigDialog.vue` 第 69-77 行：`params`/`followUps`/`env`/`mcp` 四块合并塞进单个 `el-input type="textarea"`，由 `advancedJSON` 字符串承载
- `save`（第 205-239 行）仅做 `JSON.parse` 语法校验（第 224 行），无字段级校验
- `select`（第 144-162 行）加载时把四块序列化为 `advancedJSON` 字符串
- `model/ai_function.go` 第 28-32 行：`AiMcpServer` 仅 `Type`/`URL`/`Headers`，`Type` 注释固定 `"http"`
- `buildClaudeArgs`（service/ai_function.go:329）第 348-352 行：`json.Marshal(fn.Mcp)` 整体序列化进 `--mcp-config`，model 字段扩展后 JSON 结构自动适配
- `expandEnvRef`（service/ai_function.go:368）：`$ENV:VAR` 前缀展开为进程环境变量，env 走 `--settings {"env":{...}}`
- `BuildStagePrompt`（service/ai_function.go:419）form 模式：只校验 `required` 字段为空，**不校验** `promptTemplate` 的 `{{key}}` 与 `fields.key` 一致性
- `renderPrompt`（service/ai_function.go:378）：`strings.ReplaceAll` 替换 `{{key}}`

### seed 配置（build/bin/data/ai_functions.json，4 项）

| id | params.type | followUps | 覆盖点 |
| --- | --- | --- | --- |
| speech-doc | file | 无 | file 模式（startDir/extensions/textFieldKey） |
| weekly-report | 无 | 1 项（input=null） | followUps 直接发送 |
| meeting-book | form | 无 | form 模式（promptTemplate + 3 fields） |
| meeting-list | none | 1 项（input=text） | none + followUps.input 嵌套 ParamsEditor |

四项 seed 正好覆盖全部 params.type 与 followUps 嵌套场景，表单化后须正确回填。

### 跨层契约（docs/spec/cross-layer-contracts.md）

- model 导出 struct 字段变更须同步 `frontend/wailsjs/go/models.ts`（字段声明 + 构造函数赋值）
- `omitempty` json tag → TS 字段用 `?:` 可选
- `frontend/wailsjs/` 整目录已 gitignore，`wails generate module` 自动生成，不提交
- 本批改 `AiMcpServer`（加 command/args/env），须同步 `models.ts` 的 `AiMcpServer` class

## Research References

- [`research/mcp-stdio-config-format.md`](research/mcp-stdio-config-format.md) — claude CLI `--mcp-config` stdio server 字段结构三重佐证确认

### Research 结论（待确认点 1 已闭环）

1. **design 拟扩展字段名/类型完全正确**：`Command string` / `Args []string` / `Env map[string]string`，json tag `command`/`args`/`env`（全小写），与官方 stdio 字段一致，可直接落地
2. **不要加 `cwd` 字段**：官方文档明示 stdio 仅 `command`/`args`/`env` 三字段，MCP 规范未定义 `cwd`，Claude Code 不支持配置子进程工作目录
3. **`type` 在 stdio 场景可省略但建议显式写**：官方规则"无 type 视为 stdio，但有 url 无 type 报错"；stdio 条目显式写 `"type":"stdio"` 最稳妥
4. **顶层 `{"mcpServers":{...}}` 结构无需改动**：`AiMcpConfig.Servers` 的 `json:"mcpServers"` tag 已正确
5. **http+stdio 可在同块混用**：本地 `C:\Users\liuyang\.claude.json` 实例（codegraph stdio + tavily http + officecli stdio 同块且全 Connected）佐证
6. **Windows 路径转义**：stdio 的 `command` 若用 Windows 绝对路径，JSON 内反斜杠须双写或用正斜杠

## Requirements

### P0-1 配置表单化

1. 拆出 4 个子组件到 `frontend/src/components/`：
   - `ParamsEditor.vue` — params 按 type(none/file/text/form) 切换子表单；form 模式校验 promptTemplate 的 `{{key}}` 与 fields.key 一致性
   - `FollowUpsEditor.vue` — followUps 动态列表，每项内嵌 ParamsEditor，可排序删除
   - `EnvEditor.vue` — 键值对编辑器，值支持 `${VAR}` 引用提示
   - `McpEditor.vue` — server 列表，type 切 http/stdio 分支
2. `AiFunctionConfigDialog.vue`：移除 `advancedJSON` textarea，改四块 `el-collapse` 折叠面板（默认展开 params）+ 末项「原始 JSON」兜底视图（与表单双向同步，解析失败标红）
3. 保存时字段级校验，失败定位到具体字段（如 `followUps[1].label 不能为空`）

### P2-1 MCP stdio

1. `model/ai_function.go` 的 `AiMcpServer` 增 stdio 字段：`Command string` / `Args []string` / `Env map[string]string`（均 omitempty）；**不加 cwd**
2. `buildClaudeArgs` 序列化 `--mcp-config` 时按 type 输出对应结构（model 扩展后 `json.Marshal` 自动适配）
3. `McpEditor.vue` 的 stdio 分支录入 command/args/env
4. 保存校验：http 须有 url，stdio 须有 command；交叉字段忽略不报错
5. stdio 的 env 走 `expandEnvRef`（与功能项 env 一致）

### 跨层同步

1. model 导出字段变更同步 `frontend/wailsjs/go/models.ts`（AiMcpServer class 增 command/args/env 字段声明 + 构造函数赋值）
2. 本批预计无需新增 app.go 绑定（无新 App 方法）

## Acceptance Criteria

- [ ] 四块高级字段全部表单录入，无需手写 JSON 即可配出含 form 参数 + 多 followUps + env + mcp 的功能项
- [ ] 保存时字段级校验，失败定位到具体字段
- [ ] form 模式 promptTemplate 与 fields.key 一致性校验生效（告警）
- [ ] 原始 JSON 视图与表单双向同步，解析失败有明确提示
- [ ] 既有 `build/bin/data/ai_functions.json` 配置加载后表单正确回填（含四项 seed）
- [ ] stdio 类型 MCP server 可配置且 `--mcp-config` 序列化结构正确
- [ ] 前后端双绿（`go test ./...` && `cd frontend && npm test`），wailsjs 同步完成

## Definition of Done

- 后端单测覆盖 stdio 序列化（http+stdio 混用、字段 omitempty、交叉字段忽略）
- 前端单测覆盖子组件交互、字段级校验、JSON 双向同步、seed 回填
- `models.ts` 同步且 `git status` 确认 `frontend/wailsjs/` 无未提交残留（gitignore 已忽略）
- commit 前双绿
- `/trellis:finish-work` 归档

## Technical Approach

### 子组件拆分（design 4.1 节）

| 组件 | 职责 | v-model |
| --- | --- | --- |
| `ParamsEditor.vue` | 编辑 `AiParamSpec`，按 type 切换子表单 | `params` |
| `FollowUpsEditor.vue` | 编辑 `AiFollowUp[]`，每项内嵌 ParamsEditor | `followUps` |
| `EnvEditor.vue` | 键值对编辑 `map[string]string` | `env` |
| `McpEditor.vue` | 编辑 `AiMcpConfig`，server 列表 type 切 http/stdio | `mcp` |

### ParamsEditor 按 type 切换（design 4.2 节）

| type | 渲染字段 |
| --- | --- |
| none | 无附加字段 |
| file | label、textFieldKey（默认 file）、startDir、extensions（逗号分隔转数组） |
| text | label、textFieldKey（默认 text） |
| form | label、promptTemplate（多行）、fields 列表（key/label/type/placeholder/required） |

form 模式 `promptTemplate` 中 `{{key}}` 占位符与 `fields[].key` 集合一致性校验：
- 模板出现的占位符无对应 field 定义 → 告警（未定义字段）
- field 的 key 未在模板出现 → 告警（未使用字段）

### model 扩展（design 4.1 节 + research 确认）

```go
type AiMcpServer struct {
	Type    string            `json:"type"`              // http / stdio
	// http
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	// stdio
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}
```

### 实施顺序（B 路径）

1. model 扩展（AiMcpServer 加 stdio 字段，不加 cwd）+ wailsjs models.ts 同步
2. 4 子组件（ParamsEditor → EnvEditor → McpEditor → FollowUpsEditor，后者依赖 ParamsEditor）
3. 主对话框集成（移除 advancedJSON textarea，改折叠面板 + 原始 JSON 兜底）
4. stdio 联调（buildClaudeArgs 序列化 + expandEnvRef）
5. 测试（后端 stdio 序列化 + 前端子组件/校验/回填）
6. 双绿后提交 + 归档

## Decision (ADR-lite)

### Context

design 第 6/7 节遗留 3 个待确认点，本批合并实施需先闭环。

### Decision

1. **MCP stdio JSON 结构**（待确认点 1）：以 research 抓样三重佐证为准 — `command`/`args`/`env` 全小写，不加 cwd，顶层 `mcpServers` 不变。**已闭环**
2. **promptTemplate 一致性校验**（待确认点 2）：告警不阻断 — 模板出现的占位符无对应 field 定义、field 的 key 未在模板出现，均仅展示告警提示，不阻止保存。**已定稿（采纳 design 建议）**
3. **原始 JSON 视图编辑性**（待确认点 3）：允许编辑但解析失败不回写表单 — 解析成功时双向同步回各子表单，解析失败时标红提示且表单保持原值不被脏数据污染。**已定稿（采纳 design 建议）**

### Consequences

- 告警不阻断：用户可先存后调，不因模板笔误卡死保存流程
- 原始 JSON 双向：保留批量粘贴/调试逃生口，解析失败时表单不被脏数据污染

## Out of Scope

- AiFormField 的 `Sensitive` 标记（总览 7.3 节降为可选，随 P0-1 顺带但本批不做）
- MCP server 的 `cwd` 字段（research 确认官方不支持）
- skill 自动发现（P1-1，第 4 批）
- 历史归档（P1-2，第 3 批）
- tags/pinned/schema 版本（P2-2/P2-3，第 5 批）

## Technical Notes

- 测试风格参考 `frontend/src/components/__tests__/AiFunctionPanel.spec.js`：vitest + @vue/test-utils，mock wailsjs，stubs Element Plus 组件
- Edit 工具规避：大段中文 markdown > 2000 字符或表格 > 15 行用 Write 重写整个文件
- 子组件 v-model 用 Vue 3 `defineModel` 或 `props + emit` 模式（与项目 Composition API 风格一致）
- `buildClaudeArgs` 中 `fn.Mcp` 序列化逻辑无需改动（model 扩展后 json.Marshal 自动输出 stdio 字段），但需补单测验证 http+stdio 混用场景
