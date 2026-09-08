# MCP Server stdio 类型支持设计

**日期**：2026-09-08
**优先级**：P2
**状态**：待评审（要点级，实施前需补充代码探查）

## 1. 概述

扩展 `AiMcpServer` 支持 `stdio` 类型 MCP server，使本地命令行 MCP（如基于 `npx` / `python` 启动的 server）可在功能项配置中注入，解除当前仅 `http` 类型的限制。

## 2. 现状与痛点

`model/ai_function.go` 第 28-32 行，`AiMcpServer` 的 `Type` 注释固定 `"http"`，字段仅 `Type` / `URL` / `Headers`：

```go
type AiMcpServer struct {
    Type    string            `json:"type"`    // 固定 "http"
    URL     string            `json:"url"`
    Headers map[string]string `json:"headers"`
}
```

`buildClaudeArgs`（service/ai_function.go:206）序列化 `--mcp-config` 时按此结构输出，stdio server 无法配置。当前会议功能走 http MCP，若后续接入本地 stdio MCP（如文件系统、数据库类）需改代码。

## 3. 需求总结

1. `AiMcpServer.Type` 支持 `http` 与 `stdio` 两种取值
2. `stdio` 类型携带 `command` / `args` / `env`
3. `buildClaudeArgs` 序列化 `--mcp-config` 时按 type 输出对应 JSON 结构
4. 前端 `McpEditor`（见 [[2026-09-08-ai-config-form-design]] 4.5 节）支持 stdio 分支录入

## 4. 设计要点

### 4.1 model 扩展

```go
type AiMcpServer struct {
    Type    string            `json:"type"`    // http / stdio
    // http
    URL     string            `json:"url,omitempty"`
    Headers map[string]string `json:"headers,omitempty"`
    // stdio
    Command string            `json:"command,omitempty"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
}
```

### 4.2 序列化

`buildClaudeArgs` 中 `json.Marshal(fn.Mcp)` 已整体序列化，model 字段扩展后 JSON 结构自动匹配。需确认 `--mcp-config` 的 stdio server JSON 结构：

> **实施前需确认**：claude `--mcp-config` 内联 JSON 对 stdio server 的字段要求。预期结构（以 MCP 规范为准，待校验）：
> ```json
> {"mcpServers":{"name":{"type":"stdio","command":"...","args":[...],"env":{...}}}}
> ```

### 4.3 校验

保存时按 type 校验必填：http 须有 `url`；stdio 须有 `command`。交叉字段（http 配了 command）忽略不报错。

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `model/ai_function.go` | 修改 | `AiMcpServer` 增 stdio 字段 |
| `service/ai_function.go` | 修改 | `buildClaudeArgs` 序列化兼容（依赖 model 扩展，逻辑基本不变） |
| `frontend/src/components/McpEditor.vue` | 修改 | stdio 分支录入（依赖 [[2026-09-08-ai-config-form-design]]） |
| `service/ai_function_test.go` | 修改 | stdio 序列化测试 |

## 6. 待确认点

1. `--mcp-config` stdio server JSON 结构以 MCP 规范为准，实施前抓样确认
2. stdio server 的 `env` 是否走 `expandEnvRef`（与功能项 env 一致）？建议是

## 7. 关联

- 来源：[[2026-09-08-ai-skill-menu-design]] 后续优化建议
- 配套：[[2026-09-08-ai-config-form-design]]（McpEditor 界面）
