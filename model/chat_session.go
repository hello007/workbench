package model

// ChatRole 消息角色常量（普通 string 常量而非具名 string 类型：
// 具名 string 类型出现在 Wails 方法签名时 wails 不为 models.ts 生成
// export type 别名，须手动补——用基础类型规避该同步负担，
// 见 docs/spec/cross-layer-contracts.md）。
const (
	ChatRoleUser      = "user"      // 用户发送的消息
	ChatRoleAssistant = "assistant" // AI 回复的消息
)

// 对话模板归属域常量（普通 string 常量，规避具名 string 类型的
// models.ts 别名同步负担，理由同上）。
const (
	ChatTemplateScopeGlobal    = "global"    // 全局模板：跨目录共享
	ChatTemplateScopeDirectory = "directory" // 目录模板：归属 AI 对话侧栏某目录项
)

// claude 权限模式取值（对话执行配置下拉可选集）。
const (
	ChatPermissionModeDefault          = "default"           // claude 默认行为（不传 --permission-mode）
	ChatPermissionModeAcceptEdits      = "acceptEdits"       // 自动接受文件编辑
	ChatPermissionModePlan             = "plan"              // 仅规划不执行变更
	ChatPermissionModeBypassPermission = "bypassPermissions" // 跳过全部权限确认（高危）
)

// ChatMessage 对话会话中的单条消息。
// 随所属 ChatSession 持久化于 data/ai_chat/messages/<sessionId>.json，
// 每轮对话追加 user 消息与 assistant 回复各一条。
type ChatMessage struct {
	Role      string `json:"role"`             // user / assistant（ChatRole* 常量）
	Content   string `json:"content"`          // 消息文本（markdown）
	Timestamp int64  `json:"timestamp"`        // unix 毫秒
	TaskID    string `json:"taskId,omitempty"` // 产生本条消息的对话任务 id（chat-task:done 对号用，历史消息无任务关联时为空）
}

// ChatSession AI 对话会话：AI 对话工作台中某工作目录下的一个持续式多轮会话。
// 元数据持久化于 data/ai_chat/sessions.json 索引（Messages 不落索引，omitempty）；
// 消息逐会话存 data/ai_chat/messages/<sessionId>.json，GetChatSession 加载时填充。
// ClaudeSessionID 为 claude CLI 侧会话 id，下一轮 RunChat 经 --resume 续上下文。
type ChatSession struct {
	ID              string        `json:"id"`                        // 唯一标识（主键，chatsession-<unixnano>）
	DirectoryID     string        `json:"directoryId"`               // 所属工作目录 id（AI 对话侧栏目录项）
	Title           string        `json:"title"`                     // 会话标题（用户可改，默认「新会话」）
	Cwd             string        `json:"cwd"`                       // claude 子进程工作目录（侧栏目录项对应路径）
	ClaudeSessionID string        `json:"claudeSessionId,omitempty"` // claude CLI 会话 id，空表示尚未产生（首轮对话）
	CreatedAt       int64         `json:"createdAt"`                 // 创建时间 unix 毫秒
	UpdatedAt       int64         `json:"updatedAt"`                 // 最近活跃时间 unix 毫秒（新消息/改标题时刷新，列表按此降序）
	Messages        []ChatMessage `json:"messages,omitempty"`        // 会话消息（索引文件不落此字段，GetChatSession 单独加载填充）
}

// ChatTaskState 对话任务当前状态（GetChatTaskState 拉取，前端恢复展示用）。
// Reply 为本轮 assistant 回复的累积文本（流式增量经 chat-task:output 事件推送，
// 本结构用于切页恢复/轮询兜底）。
type ChatTaskState struct {
	TaskID          string `json:"taskId"`
	ChatSessionID   string `json:"chatSessionId"` // 所属 WorkBench 会话 id
	Running         bool   `json:"running"`
	Queued          bool   `json:"queued"`          // 排队中（等待并发槽位，未起进程）
	ClaudeSessionID string `json:"claudeSessionId"` // claude 会话 id（result 事件提取，供下一轮 --resume）
	Reply           string `json:"reply"`           // 本轮 assistant 回复累积文本
	Error           string `json:"error"`
	StartedAt       int64  `json:"startedAt"` // unix 毫秒
}

// ChatTaskRunResult 单轮对话执行结果，经 chat-task:done 事件推送。
// Reply 为 assistant 回复全文（markdown）；失败/取消时可能为空或含部分回复。
type ChatTaskRunResult struct {
	TaskID          string `json:"taskId"`
	ChatSessionID   string `json:"chatSessionId"`   // 所属 WorkBench 会话 id
	ClaudeSessionID string `json:"claudeSessionId"` // claude 会话 id，供下一轮 --resume
	Reply           string `json:"reply"`           // assistant 回复全文（markdown）
	ExitCode        int    `json:"exitCode"`
	Error           string `json:"error"`
	Canceled        bool   `json:"canceled"`
}

// ChatDirectory AI 对话侧栏常用目录项。
// 与工作目录（model.Directory）/收藏夹（favorites）语义隔离：本列表专属
// AI 对话入口（自定义显示名 + 排序 + 独立增删），供 DirectoryTree/FileTreePanel
// 右键「添加到 AI 对话」沉淀。持久化于 data/ai_chat/directories.json，
// ChatSession.DirectoryID 引用本结构 ID。
type ChatDirectory struct {
	ID          string `json:"id"`          // 唯一标识（chatdir-<unixnano>）
	Path        string `json:"path"`        // 目录绝对路径（filepath.Abs 规范化，claude 子进程 cwd）
	DisplayName string `json:"displayName"` // 侧栏显示名（添加时为空则取目录名，可改）
	SortOrder   int    `json:"sortOrder"`   // 侧栏排序（拖拽重排后按列表序重写）
	CreatedAt   int64  `json:"createdAt"`   // 创建时间 unix 毫秒
}

// ChatTemplate 对话输入框纯文本模板片段（点击整段填入输入框可再修改，
// 无占位符注入）。持久化于 data/ai_chat/templates.json。
// Scope 取 ChatTemplateScope* 常量：global 跨目录共享，directory 归属
// ChatDirectory.ID（该目录项被移除后其目录模板不再返回，数据留档不清理）。
type ChatTemplate struct {
	ID          string `json:"id"`                    // 唯一标识（chattpl-<unixnano>）
	Scope       string `json:"scope"`                 // global / directory（ChatTemplateScope* 常量）
	DirectoryID string `json:"directoryId,omitempty"` // scope=directory 时的归属目录项 id，global 恒空
	Name        string `json:"name"`                  // 模板名（下拉展示）
	Content     string `json:"content"`               // 模板正文（纯文本，点击整段填入输入框）
	CreatedAt   int64  `json:"createdAt"`             // 创建时间 unix 毫秒
	UpdatedAt   int64  `json:"updatedAt"`             // 最近修改时间 unix 毫秒
}

// ChatSettings AI 对话执行配置（权限模式 + 模型），对话页顶部下拉选择后
// 持久化记忆，重启恢复。持久化于 data/ai_chat/settings.json。
// ModelName 为空表示使用 claude CLI 默认模型。
type ChatSettings struct {
	PermissionMode string `json:"permissionMode"` // default/acceptEdits/plan/bypassPermissions（ChatPermissionMode* 常量）
	ModelName      string `json:"modelName"`      // claude 模型名（如 sonnet/opus/具体版本号），空 = claude 默认
}
