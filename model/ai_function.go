package model

// AiFunction AI 功能项：工具箱「AI 功能」页中一个可一键触发的 Claude skill 调用配置。
// 配置存 data/ai_functions.json，新增 skill 只加配置不改代码。
type AiFunction struct {
	ID             string            `json:"id"`             // 唯一标识（主键）
	Name           string            `json:"name"`           // 显示名称
	Description    string            `json:"description"`    // 显示描述
	Icon           string            `json:"icon"`           // Element Plus 图标名（如 "MagicStick"）
	Command        string            `json:"command"`        // 斜杠命令（含插件命名空间，如 "/ab-office:agree-slides"）
	Cwd            string            `json:"cwd"`            // 子进程工作目录（skill 所在项目根，项目级 skill 发现依赖此值）
	AddDirs        []string          `json:"addDirs"`        // 额外授权目录（逐个映射为 --add-dir）
	Env            map[string]string `json:"env"`            // 注入子进程的环境变量（经 --settings {"env":{...}}，MCP token 等）
	Mcp            *AiMcpConfig      `json:"mcp"`            // MCP server 注入（经 --mcp-config 内联 JSON），nil 表示不注入
	PermissionMode string            `json:"permissionMode"` // claude --permission-mode，默认 bypassPermissions（菜单场景无交互）
	TimeoutMinutes int               `json:"timeoutMinutes"` // 超时分钟数，0 表示用默认值 10
	Completion     string            `json:"completion"`     // 完成动作：none/open_dir/preview/copy
	Params         *AiParamSpec      `json:"params"`         // 运行前参数输入，nil 表示无参数直跑
	FollowUps      []AiFollowUp      `json:"followUps"`      // 后续段（多段编排），运行完成后面板显示对应按钮，点击即 --resume 续会话
	Tags           []string          `json:"tags,omitempty"`   // 业务域标签，如 ["周报","ABX5"]，供列表分组筛选与搜索匹配
	Pinned         bool              `json:"pinned,omitempty"` // 置顶，列表排序时始终排在最前
}

// CurrentSchemaVersion ai_functions.json 当前 schema 版本。
// 字段演进（增非兼容字段）时 +1，并在 service.migrateFunctions 新增对应迁移分支。
// v1：顶层裸数组 []*AiFunction（无 schemaVersion）；v2：{schemaVersion, functions} + Tags/Pinned 字段。
const CurrentSchemaVersion = 2

// AiFunctionsConfig ai_functions.json 顶层结构（schema v2+）。
// 加载时旧 v1 数组自动包一层并迁移补全新字段；保存时统一写本结构。
type AiFunctionsConfig struct {
	SchemaVersion int           `json:"schemaVersion"`
	Functions     []*AiFunction `json:"functions"`
}

// ImportPreview 导入预览：解析+迁移+比对后的分类结果，供前端展示后确认，不直接落盘。
// ImportAiFunctions 对外部 JSON 走 migrateFunctions 迁移补全 + validateFunctions 校验后，
// 与本机已加载功能项按 id 比对生成三类：New（本机不存在，将新增）、Conflict（id 已存在，
// 待用户决策覆盖/跳过）、Invalid（校验失败的 id，不导入）。
type ImportPreview struct {
	New      []*AiFunction `json:"new"`      // 本机不存在的，将新增
	Conflict []*AiFunction `json:"conflict"` // id 已存在，待用户决策覆盖本机/跳过
	Invalid  []string      `json:"invalid"`  // 校验失败的 id（不会导入）
}

// AiMcpConfig MCP server 注入配置，整体序列化为 --mcp-config 的内联 JSON。
type AiMcpConfig struct {
	Servers map[string]AiMcpServer `json:"mcpServers"`
}

// AiMcpServer 单个 MCP server 定义，支持 http 与 stdio 两种类型。
// 序列化进 --mcp-config 的 mcpServers map 项，按 type 输出对应字段（交叉字段 omitempty 不输出）。
type AiMcpServer struct {
	Type string `json:"type"` // http / stdio
	// http 类型字段
	URL     string            `json:"url,omitempty"`     // server 地址（http 必填）
	Headers map[string]string `json:"headers,omitempty"` // 附加请求头（如 Authorization: Bearer xxx）
	// stdio 类型字段（本地命令行 MCP，如 npx/python 启动的 server）
	Command string            `json:"command,omitempty"` // 启动命令（stdio 必填，如 npx/-y/@modelcontextprotocol/server-filesystem）
	Args    []string          `json:"args,omitempty"`    // 命令参数
	Env     map[string]string `json:"env,omitempty"`     // 子进程环境变量（经 expandEnvRef 展开，与功能项 env 一致）
}

// AiParamSpec 运行前参数输入规格。
// Type 取值：
//   - "none"：无参数直跑（Params 为 nil 等价）
//   - "file"：选一个文件作为输入源（追加到命令尾部）
//   - "form"：字段化表单（渲染 PromptTemplate 模板）
//   - "text"：单行文本（追加到命令尾部）
type AiParamSpec struct {
	Type           string        `json:"type"`
	Label          string        `json:"label"`          // 参数标题（如 "选择源文档"）
	StartDir       string        `json:"startDir"`       // file：文件选择起始目录
	Extensions     []string      `json:"extensions"`     // file：文件扩展名过滤（如 [".md", ".docx"]）
	PromptTemplate string        `json:"promptTemplate"` // form：完整 prompt 模板，{{key}} 占位替换
	Fields         []AiFormField `json:"fields"`         // form：字段定义
	TextFieldKey   string        `json:"textFieldKey"`   // text/file：单值参数在 params map 中的 key（如 "file"、"meeting"）
}

// AiFormField 表单字段定义（form 类型参数用）。
type AiFormField struct {
	Key         string `json:"key"`   // 模板占位符名
	Label       string `json:"label"` // 表单标签
	Type        string `json:"type"`  // text/datetime/number
	Placeholder string `json:"placeholder"`
	Required    bool   `json:"required"`
}

// AiFollowUp 后续段定义（多段编排）。
// 主段运行完成后，若功能项定义了 FollowUps，输出面板展示这些按钮；
// 点击后以 --resume <session-id> 在同一会话继续发送渲染后的 Prompt。
type AiFollowUp struct {
	ID             string       `json:"id"`
	Label          string       `json:"label"`          // 按钮文案（如 "确认落盘"）
	PromptTemplate string       `json:"promptTemplate"` // prompt 模板，{{key}} 占位替换
	Input          *AiParamSpec `json:"input"`          // 点击前需要的输入，nil 表示直接发送
}

// AiTaskUsage 单次执行的 token 用量（来自 result 事件 usage 字段）。
// 源字段映射：input_tokens / output_tokens / cache_creation_input_tokens / cache_read_input_tokens。
type AiTaskUsage struct {
	InputTokens              int `json:"inputTokens"`
	OutputTokens             int `json:"outputTokens"`
	CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
	CacheReadInputTokens     int `json:"cacheReadInputTokens"`
}

// AiTaskMetrics 任务计量摘要（来自 result 事件 + 进程退出状态）。
// 源字段映射：duration_ms / total_cost_usd（顶层，非 cost_usd）/ num_turns。
type AiTaskMetrics struct {
	Usage      *AiTaskUsage `json:"usage,omitempty"`
	DurationMs int64        `json:"durationMs"`
	CostUSD    float64      `json:"costUsd"`
	NumTurns   int          `json:"numTurns"`
}

// AiConcurrencyStatus 全局并发占用情况，供前端标题栏展示「N/M」。
type AiConcurrencyStatus struct {
	Running int `json:"running"` // 运行中数量（已获取信号量槽位且进程未退出）
	Queued  int `json:"queued"`  // 排队中数量（等待槽位）
	Max     int `json:"max"`     // 并发上限（信号量缓冲长度）
}

// AiTaskRunResult 单段执行的结果快照，经 Wails 事件 ai-task:done 推送。
// 3.3 流式文件改造后 Output 仅含末尾预览（~4KB），全量输出已落输出文件，
// 前端 copy/preview/表格视图经 GetAiTaskOutput 全量读取，不再从本结构取全量。
type AiTaskRunResult struct {
	TaskID         string         `json:"taskId"`
	SessionID      string         `json:"sessionId"` // claude 会话 id，供后续段 --resume
	ExitCode       int            `json:"exitCode"`
	Error          string         `json:"error"`
	Output         string         `json:"output"`                   // 末尾预览（~4KB），非全量
	OutputSize     int64          `json:"outputSize"`               // 完整输出字节数
	OutputFile     string         `json:"outputFile"`               // 输出文件相对路径（归档后指向 data/ai_task_history/<id>.txt）
	TableExtracted *MeetingTable  `json:"tableExtracted,omitempty"` // 预解析 markdown 表格，nil 表示无表格
	Canceled       bool           `json:"canceled"`
	Metrics        *AiTaskMetrics `json:"metrics,omitempty"` // P0-2：result 事件计量，nil 表示无计量数据
}

// AiTaskState 任务当前状态（前端恢复/展示用），GetAiTaskState 拉取。
// 3.3 流式文件改造后 Output 仅含末尾预览（~4KB），不再全量拷贝 strings.Builder。
type AiTaskState struct {
	TaskID         string         `json:"taskId"`
	FunctionID     string         `json:"functionId"`
	Running        bool           `json:"running"`
	Queued         bool           `json:"queued"` // P0-3：排队中（等待并发槽位，未起进程）
	SessionID      string         `json:"sessionId"`
	Prompt         string         `json:"prompt"`
	Output         string         `json:"output"`                   // 末尾预览（~4KB），非全量
	OutputSize     int64          `json:"outputSize"`               // 完整输出字节数
	OutputFile     string         `json:"outputFile"`               // 输出文件相对路径（供前端拉全量）
	TableExtracted *MeetingTable  `json:"tableExtracted,omitempty"` // 预解析 markdown 表格，nil 表示无表格
	Error          string         `json:"error"`
	StartedAt      int64          `json:"startedAt"`         // unix 毫秒
	Metrics        *AiTaskMetrics `json:"metrics,omitempty"` // P0-2：计量摘要，恢复展示用
}
