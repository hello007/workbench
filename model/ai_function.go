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
}

// AiMcpConfig MCP server 注入配置，整体序列化为 --mcp-config 的内联 JSON。
type AiMcpConfig struct {
	Servers map[string]AiMcpServer `json:"mcpServers"`
}

// AiMcpServer 单个 MCP server 定义（当前仅支持 http 类型）。
type AiMcpServer struct {
	Type    string            `json:"type"`    // 固定 "http"
	URL     string            `json:"url"`     // server 地址
	Headers map[string]string `json:"headers"` // 附加请求头（如 Authorization: Bearer xxx）
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

// AiTaskRunResult 单段执行的结果快照，经 Wails 事件 ai-task:done 推送。
type AiTaskRunResult struct {
	TaskID    string `json:"taskId"`
	SessionID string `json:"sessionId"` // claude 会话 id，供后续段 --resume
	ExitCode  int    `json:"exitCode"`
	Error     string `json:"error"`
	Output    string `json:"output"` // 全量文本输出（copy 完成动作取此值）
	Canceled  bool   `json:"canceled"`
}

// AiTaskState 任务当前状态（前端恢复/展示用），GetAiTask 拉取。
type AiTaskState struct {
	TaskID     string `json:"taskId"`
	FunctionID string `json:"functionId"`
	Running    bool   `json:"running"`
	SessionID  string `json:"sessionId"`
	Prompt     string `json:"prompt"`
	Output     string `json:"output"`
	Error      string `json:"error"`
	StartedAt  int64  `json:"startedAt"` // unix 毫秒
}
