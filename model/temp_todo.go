package model

// TempTodo 临时待办条目（侧边栏「临时待办」面板，data/temp_todos.json）。
//
// CompletedAt 用指针而非 int64：nil 表示未完成，非 nil 为完成时间（Unix 毫秒）。
// 若用值类型 + omitempty，0 值既可能是「未完成」也可能是「1970-01-01」，
// 前端无法区分归档态（对齐 session.TerminalSnapshot 的同类决策）。
type TempTodo struct {
	ID   string   `json:"id"`   // 16 位小写 hex 随机 ID；熵源失败退化为 t<纳秒>
	Text string   `json:"text"` // 正文，保留换行，不做富文本
	Tags []string `json:"tags"` // 自由文本标签（输入即建，不去重归一之外的语义）
	// CreatedAt 创建时间（Unix 毫秒）。编辑正文/标签不改变该字段。
	CreatedAt int64 `json:"createdAt"`
	// CompletedAt 完成时间（Unix 毫秒）；nil = 仍在待办区。
	CompletedAt *int64 `json:"completedAt"`
}

// TempTodoDraft 批量导入时的单条草稿（Markdown 导入用）。
//
// 与 TempTodo 的区别：无 ID / 完成态，ID 由 service 生成，导入条目恒为待办态。
// CreatedAt 为 0 表示「取导入时刻」——前端对没有行内时间的待办行传 0，
// 由后端统一填充，避免前后端时钟口径分叉。
type TempTodoDraft struct {
	Text      string   `json:"text"`
	Tags      []string `json:"tags"`
	CreatedAt int64    `json:"createdAt"`
}
