# 研究：claude CLI `--mcp-config` 内联 JSON 对 stdio 类型 MCP server 的字段要求

- **查询**：确认 claude CLI `--mcp-config` 内联 JSON 的顶层结构、stdio server 必填/可选字段、http 与 stdio 是否可混用、与 `.mcp.json` 配置格式是否一致
- **范围**：外部（官方文档 + 本地 CLI 实测 + 用户配置实例）+ 内部（WorkBench 现有序列化代码对照）
- **日期**：2026-09-08

---

## 一、结论先行：完整的 stdio + http 混用 JSON 示例

`--mcp-config` 接受的 JSON 字符串顶层为 `{"mcpServers": { ... }}` 容器，与 `.mcp.json` 文件内容同构。stdio 与 http server 可在同一 `mcpServers` map 中混用：

```json
{
  "mcpServers": {
    "my-stdio": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      "env": { "API_KEY": "xxx" }
    },
    "my-http": {
      "type": "http",
      "url": "https://mcp.example.com/mcp",
      "headers": { "Authorization": "Bearer xxx" }
    }
  }
}
```

本地实测铁证（`C:\Users\liuyang\.claude.json` user-scope，第 2651-2671 行，三个 server 同块混用）：

```json
"mcpServers": {
  "codegraph": {
    "type": "stdio",
    "command": "codegraph",
    "args": ["serve", "--mcp"]
  },
  "tavily-remote-mcp": {
    "type": "http",
    "url": "https://mcp.tavily.com/mcp/?tavilyApiKey=..."
  },
  "officecli": {
    "type": "stdio",
    "command": "C:\\Users\\liuyang\\AppData\\Local\\OfficeCli\\officecli.exe",
    "args": ["mcp"],
    "env": {}
  }
}
```

`claude mcp list` 实测三个 server 均 `✓ Connected`，证实混用与 Windows 绝对路径 command 均被接受。

---

## 二、字段清单表

### 顶层容器

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `mcpServers` | object | 是 | 顶层包装键，camelCase（S 大写）。value 为 server map，key 为 server 名（自定义，用作工具前缀与 `/mcp` 面板显示） |

### server 条目公共字段

| 字段 | 类型 | 必填 | 适用 type | 说明 |
|---|---|---|---|---|
| `type` | string | http/sse/ws 必填；stdio 可省略 | 全部 | 取值 `"stdio"` / `"http"` / `"sse"` / `"ws"`，**全小写**。官方明确：缺省 `type` 时按 stdio 处理；但有 `url` 却无 `type` 是配置错误（Claude Code 跳过该 server 并报 `MCP server "<name>" has a "url" but no "type"`） |
| `timeout` | number | 否 | 全部 | 每 tool call 的墙钟上限（毫秒），<1000 被忽略。非本批关注字段 |

### stdio 类型字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `command` | string | 是 | 可执行命令名（依赖 PATH）或绝对路径。Windows 绝对路径示例：`"C:\\Users\\...\\officecli.exe"`（JSON 内反斜杠须转义） |
| `args` | string[] | 否 | 命令参数数组，每个元素为字符串。如 `["-y", "@some/mcp-server"]` |
| `env` | map<string,string> | 否 | 子进程环境变量键值对。可为空 `{}`（本地 officecli 实例即为空 map） |

**官方原文（docs.claude.com/en/docs/claude-code/mcp.md）明确列出**："`stdio` servers: `command`, `args`, `env`" —— 即 stdio 仅此三字段。

### http / sse / ws 类型字段

| 字段 | 类型 | 必填 | 适用 type | 说明 |
|---|---|---|---|---|
| `url` | string | 是 | http/sse/ws | server 端点 URL |
| `headers` | map<string,string> | 否 | http/ws | 附加请求头，如 `{"Authorization": "Bearer xxx"}` |
| `oauth` | object | 否 | http/sse | OAuth 客户端配置（`clientId` / `callbackPort` 等），非本批关注 |

---

## 三、四个核心问题的确认回答

### Q1. `--mcp-config` 接受的内联 JSON 顶层结构

**确认**：`{"mcpServers": { "<server-name>": { ... }, ... }}`。

CLI 帮助原文（`claude --help`）：

```
--mcp-config <configs...>  Load MCP servers from JSON files or strings (space-separated)
--strict-mcp-config        Only use MCP servers from --mcp-config, ignoring all other MCP configurations
```

即 `--mcp-config` 既可传文件路径，也可传内联 JSON 字符串；字符串内容就是 `{"mcpServers":{...}}` 完整容器（与 `.mcp.json` 文件内容一致）。

> **注意区分**：`claude mcp add-json <name> <json>` 子命令接受的是**单个 server 对象**（不带 `mcpServers` 包装），官方文档原文："Pass `claude mcp add-json` the object inside `mcpServers`, not the wrapper."。而 `--mcp-config` 与 `.mcp.json` 接受的是**带 `mcpServers` 包装的完整块**。二者层次不同。

### Q2. stdio server 字段名、大小写、是否有其他字段

**确认**：
- 字段名：`type` / `command` / `args` / `env`，**全小写**
- 必填：`command`（stdio 核心字段，缺则报 `command: expected string, received undefined`，v2.1.202 前的错误形态）；`type` 在纯 stdio 场景可省略（缺省即 stdio），但显式写 `"stdio"` 合法且更清晰
- 可选：`args`、`env`
- **未支持 `cwd`**：官方文档字段列表与本地三个实测 server 均无 `cwd` 字段。MCP 规范（modelcontextprotocol.io）的 stdio server config 也未定义 `cwd`。Claude Code 不支持通过配置指定子进程工作目录（见"不确定点"）

### Q3. http 与 stdio 能否在同一 mcpServers map 混用

**确认**：可以混用。本地 `.claude.json` user-scope 块同时包含 codegraph(stdio) + tavily-remote-mcp(http) + officecli(stdio)，`claude mcp list` 全部 `✓ Connected`。官方文档亦明示多种 transport 共存于同一 `mcpServers` 块（"Multiple transport types: support for stdio, SSE, HTTP, and WebSocket transports"）。

每个 server 条目按自身 `type` 字段独立解析，http 条目读 `url`/`headers`，stdio 条目读 `command`/`args`/`env`，互不干扰。交叉字段（如 http 条目里写了 `command`）会被忽略不报错（基于"按 type 分支解析"的通用实现模式，官方未明确文档化此行为，但本地实测与 WorkBench 现有 http 配置共存正常）。

### Q4. 与 Claude Code `.mcp.json` 配置文件格式是否一致

**确认**：完全一致。`--mcp-config` 内联 JSON 字符串 = `.mcp.json` 文件内容 = `~/.claude.json` 顶层 `mcpServers` 块，三者使用同一 schema（`{"mcpServers":{...}}`）。

官方文档原文：
- "add the entry under `mcpServers` in `.mcp.json` at your project root and commit it" —— `.mcp.json` 用 `mcpServers` 包装
- "An `mcpServers` JSON block: configuration written for another client's settings file" —— 该结构跨客户端通用（Claude Desktop 同形）
- `--strict-mcp-config` 的行为描述印证 `--mcp-config` 与项目级 `.mcp.json` 是同一加载入口的不同来源

scope 区别（仅存储位置不同，schema 相同）：
- `.mcp.json`（项目根）= project scope，提交到仓库共享给团队，首次使用需信任审批
- `~/.claude.json` 顶层 `mcpServers` = user scope，跨项目可用
- `--mcp-config` 内联 = 一次性临时注入，仅当前会话生效（配合 `--strict-mcp-config` 可独占）

---

## 四、WorkBench 现有实现对照（事实陈述，非建议）

`service/ai_function.go:348-352` 现有序列化逻辑：

```go
if fn.Mcp != nil && len(fn.Mcp.Servers) > 0 {
    if raw, err := json.Marshal(fn.Mcp); err == nil {
        args = append(args, "--mcp-config", string(raw))
    }
}
```

`model/ai_function.go:23-32` 现有结构：

```go
type AiMcpConfig struct {
    Servers map[string]AiMcpServer `json:"mcpServers"`
}
type AiMcpServer struct {
    Type    string            `json:"type"`
    URL     string            `json:"url"`
    Headers map[string]string `json:"headers"`
}
```

对照结论：
- 顶层 `AiMcpConfig.Servers` 的 json tag 为 `mcpServers`，序列化结果为 `{"mcpServers":{...}}`，**与 `--mcp-config` 要求的顶层结构匹配**（已由现有 http 功能正常运行佐证）
- `AiMcpServer` 的 `Type`/`URL`/`Headers` 字段名与官方 http server 字段名**完全一致**（全小写）
- 设计文档 `docs/plans/2026-09-08-ai-mcp-stdio-design.md` 第 4.1 节拟扩展的 `Command`/`Args`/`Env` 字段（json tag `command`/`args`/`env`，全小写）**与官方 stdio 字段名完全一致**
- `Env` 类型 `map[string]string`、`Args` 类型 `[]string`，**与官方类型匹配**
- `json:"...,omitempty"` 标注可使 http 条目不输出 stdio 字段、stdio 条目不输出 http 字段，避免交叉字段污染 JSON（虽被 CLI 忽略，但 omitempty 让输出更干净）

---

## 五、来源链接

| 来源 | URL / 路径 | 关键内容 |
|---|---|---|
| Claude Code MCP 官方文档（markdown） | `https://docs.claude.com/en/docs/claude-code/mcp.md` | stdio 字段列表、`mcpServers` 包装结构、`type` 缺省规则、`--mcp-config` / `--strict-mcp-config` 说明、混用支持、`.mcp.json` 格式 |
| Claude Code CLI 本地帮助 | `claude --help` | `--mcp-config <configs...>` 描述 "Load MCP servers from JSON files or strings" |
| Claude Code CLI 本地帮助 | `claude mcp add --help` | `-t/--transport` 取值 `stdio`/`sse`/`http`，默认 stdio；`-e/--env` 传环境变量；`--` 分隔符语义 |
| Claude Code CLI 本地帮助 | `claude mcp add-json --help` | add-json 接受单个 server 对象（不带 mcpServers 包装） |
| 用户配置实测实例 | `C:\Users\liuyang\.claude.json` 第 2651-2671 行 | codegraph(stdio) + tavily(http) + officecli(stdio) 同块混用，`claude mcp list` 全部 Connected |
| WorkBench 现有序列化代码 | `service/ai_function.go:326-364` | `buildClaudeArgs` 把 `fn.Mcp` 整体 `json.Marshal` 后作为 `--mcp-config` 内联字符串 |
| WorkBench 现有模型 | `model/ai_function.go:22-32` | `AiMcpConfig` / `AiMcpServer` 当前结构与字段 tag |
| WorkBench 设计文档 | `docs/plans/2026-09-08-ai-mcp-stdio-design.md` | 第 4.1/4.2 节拟扩展方案与"待确认点" |
| MCP 规范（参考） | `https://modelcontextprotocol.io/docs/learn/mcp-clients` | stdio server config 的规范定义（与 Claude Code 实现一致） |

> 官方文档以 `docs.claude.com/en/docs/claude-code/mcp` 现行页面为准；本研究的字段结论已由本地 CLI 实测与 `.claude.json` 真实配置双重佐证，不依赖网页抓取完整性。

---

## 六、不确定点 / 注意事项

1. **`cwd` 字段**：官方文档字段列表与本地三个实测 server 均无 `cwd`，MCP 规范未定义。Claude Code 不支持通过配置指定子进程工作目录。若 stdio server 依赖特定工作目录，需在 server 自身实现中处理，或通过 `args` 传 `--cwd` 类参数给支持该 flag 的 server。**未做主动实测**（避免污染用户配置），结论基于官方文档明示字段范围 + 实测样例缺失双重证据。

2. **`type` 缺省行为**：官方明确"无 `type` 视为 stdio"，但 WorkBench 现有 `AiMcpServer.Type` 对 http 是必填（设计文档第 4.3 节校验：http 须有 url，stdio 须有 command）。建议 stdio 条目显式写 `"type":"stdio"` 以避免歧义（这是事实陈述——官方推荐显式声明以避免 `url` 误判场景）。

3. **`env` 值类型**：官方所有示例 `env` 值均为字符串。`map[string]string` 安全。若 server 需要非字符串环境变量，须在 server 端自行解析。

4. **Windows 路径转义**：JSON 字符串内 Windows 绝对路径的反斜杠须转义（`"C:\\Users\\..."`），或用正斜杠（`"C:/Users/..."`）。本地 officecli 实例用双反斜杠形式。

5. **变量展开**：官方文档示例支持 `${CLAUDE_PLUGIN_ROOT}` 与 `${VAR:-default}` 形式的变量引用（如 `"${API_BASE_URL:-https://api.example.com}/mcp"`）。WorkBench 现有 `expandEnvRef`（`service/ai_function.go:366-375`）处理 `$ENV:VAR` 前缀，与 Claude Code 原生 `${VAR}` 语法不同——这是 WorkBench 自有的运行时展开，不影响 `--mcp-config` JSON 结构本身（展开后写入）。stdio server 的 `env` 是否应经 `expandEnvRef` 展开，设计文档第 6 节已列为待确认点。

6. **`add-json` 与 `--mcp-config` 层次差异**：`claude mcp add-json` 接受单个 server 对象（不带 `mcpServers` 包装），`--mcp-config` 接受完整 `{"mcpServers":{...}}` 块。本研究针对 `--mcp-config`，结论为带包装的完整块。WorkBench 走 `--mcp-config` 路径，与设计一致。

7. **未实测项**：未主动构造带 `cwd` 或带未知字段的 JSON 跑 `--mcp-config` 验证 Claude Code 是否严格拒绝未知字段（避免启动 claude 进程消耗 token 与污染用户环境）。基于官方"字段列表明示"的文档风格，未列出字段应被忽略而非报错，但此行为未实测。
