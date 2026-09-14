# AI 功能配置 — 高级字段表单化设计

**日期**：2026-09-08
**优先级**：P0
**状态**：待评审

## 1. 概述

将 `AiFunctionConfigDialog` 中仍以裸 JSON 编辑的 `params`、`followUps`、`env`、`mcp` 四块高级字段改为结构化表单，消除「可视化集成」的体验断点。顶部基础字段（id/name/command/cwd 等）已表单化，本设计仅覆盖高级字段区。

## 2. 背景与痛点

`AiFunctionConfigDialog.vue` 顶部 10 个基础字段已用 `el-form-item` 表单化，但高级字段（第 69-77 行）合并塞进单个 `el-input type="textarea"`，由 `advancedJSON` 字符串承载：

```vue
<el-input v-model="advancedJSON" type="textarea"
  placeholder="params（参数输入）与 followUps（多段编排按钮）的 JSON 定义" />
```

| 痛点 | 说明 |
| --- | --- |
| 无字段级校验 | 仅做 `JSON.parse` 语法校验（第 225 行），`promptTemplate` 占位符与 `fields` 的 `key` 是否匹配、`type` 取值是否合法均不校验 |
| 易写错 | `params.type` 取值 `none/file/text/form` 靠记忆，`followUps` 数组结构靠手写，普通用户难正确填写 |
| 可视化未闭环 | 顶部表单 + 底部 JSON 割裂，新功能项仍需懂 JSON 才能配齐参数与后续段 |
| MCP 仅 http | `mcp.servers` 的 `type` 固定 `http`，stdio 类 server 无法在界面配置（见 [[2026-09-08-ai-mcp-stdio-design]]） |

## 3. 需求总结

1. `params` 按 `type` 切换子表单，字段级录入与校验
2. `followUps` 动态列表，每项展开 `label` / `promptTemplate` / 嵌套 `input`（可选 `AiParamSpec`）
3. `env` 键值对编辑器，增删改行
4. `mcp` server 列表编辑，支持 `http` 与 `stdio` 两种类型
5. 保留「原始 JSON」折叠视图作为兜底与高级逃生口，与表单双向同步
6. 保存时字段级校验失败须给出具体定位（如「followUps[1].label 不能为空」），而非整体语法错误

## 4. 设计

### 4.1 子组件拆分

高级字段区从 `AiFunctionConfigDialog.vue` 拆出，按块独立组件，降低主对话框复杂度：

| 组件 | 职责 | props / v-model |
| --- | --- | --- |
| `ParamsEditor.vue` | 编辑 `AiParamSpec` | `v-model: params` |
| `FollowUpsEditor.vue` | 编辑 `AiFollowUp[]`，每项内嵌 `ParamsEditor` | `v-model: followUps` |
| `EnvEditor.vue` | 键值对编辑 `map[string]string` | `v-model: env` |
| `McpEditor.vue` | 编辑 `AiMcpConfig`，server 列表 | `v-model: mcp` |

主对话框高级字段区改为四块折叠面板（`el-collapse`），默认展开 `params`，其余收起。

### 4.2 ParamsEditor — 按 type 切换

`type` 用 `el-select`（none/file/text/form 四选），下方按取值条件渲染：

| type | 渲染字段 |
| --- | --- |
| `none` | 无附加字段 |
| `file` | `label`、`textFieldKey`（默认 `file`）、`startDir`、`extensions`（逗号分隔转数组） |
| `text` | `label`、`textFieldKey`（默认 `text`） |
| `form` | `label`、`promptTemplate`（多行文本）、`fields` 列表（每行 `key`/`label`/`type`/`placeholder`/`required`） |

`form` 模式下 `promptTemplate` 中 `{{key}}` 占位符须与 `fields[].key` 集合一致，保存时校验：

- 模板出现的占位符有对应 field 定义（告警：未定义字段）
- field 的 `key` 未在模板出现（告警：未使用字段）

### 4.3 FollowUpsEditor — 动态列表

每项一个可折叠卡片，标题显示 `label` 或 `(未命名)`，内容：

- `label`：按钮文案
- `promptTemplate`：多行文本
- `input`：可选，点击「配置输入」嵌入 `ParamsEditor`（`v-model: input`，null 表示直接发送）

列表底部「+ 新增后续段」按钮。每项可删除、可上下排序。

### 4.4 EnvEditor — 键值对

两列表格：键 | 值 | 操作（删除）。底部「+ 新增」追加空行。值支持 `${VAR}` 环境变量引用提示（后端 `expandEnvRef` 已支持，见 service/ai_function.go `buildClaudeArgs`）。

### 4.5 McpEditor — server 列表

server 列表，每项：

- `name`：server 标识（`mcpServers` map 的 key）
- `type`：`http` / `stdio` 切换
- `http`：`url` + `headers`（键值对，复用 EnvEditor 样式）
- `stdio`：`command` + `args`（数组，逗号分隔） + `env`（键值对）

> stdio 类型依赖 [[2026-09-08-ai-mcp-stdio-design]] 的 model 扩展，可先行界面占位，model 落地后打通。

### 4.6 原始 JSON 兜底视图

折叠面板末项「原始 JSON（高级）」，展开后显示当前四块合并的 JSON（与表单双向绑定，只读优先；编辑后 `JSON.parse` 同步回各表单，解析失败标红提示）。用途：批量粘贴已有配置、调试、表单未覆盖的边界字段。

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `frontend/src/components/AiFunctionConfigDialog.vue` | 修改 | 移除 `advancedJSON` textarea，改为四块折叠面板 + 子组件 |
| `frontend/src/components/ParamsEditor.vue` | 新增 | params 按 type 切换子表单 |
| `frontend/src/components/FollowUpsEditor.vue` | 新增 | followUps 动态列表 |
| `frontend/src/components/EnvEditor.vue` | 新增 | 键值对编辑器 |
| `frontend/src/components/McpEditor.vue` | 新增 | mcp server 列表 |
| `frontend/src/components/__tests__/AiFunctionConfigDialog.spec.js` | 新增/修改 | 子组件交互、字段级校验、JSON 双向同步测试 |

## 6. 验收标准

1. 四块高级字段全部表单录入，无需手写 JSON 即可配出含 `form` 参数 + 多 `followUps` + `env` + `mcp` 的功能项
2. 保存时字段级校验，失败定位到具体字段
3. `form` 模式 `promptTemplate` 与 `fields.key` 一致性校验生效
4. 原始 JSON 视图与表单双向同步，解析失败有明确提示
5. 既有 `data/ai_functions.json` 配置加载后表单正确回填（含四项 seed 功能）

## 7. 待确认点

1. `form` 模式 `promptTemplate` 占位符一致性校验是「阻断保存」还是「仅告警」？建议告警不阻断，允许用户先存后调
2. 原始 JSON 视图是否允许编辑（双向），还是只读展示？建议允许编辑但解析失败时不回写表单

## 8. 关联

- 来源：[[2026-09-08-ai-skill-menu-design]] 后续优化建议
- 依赖：[[2026-09-08-ai-mcp-stdio-design]]（McpEditor 的 stdio 分支）
