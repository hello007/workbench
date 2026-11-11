package model

// Prompt 提示词条目（侧边栏「提示词」面板，data/prompts.json）。
//
// 与 TempTodo 的区别：没有完成态，定位是可长期复用的文本资产；标题必填，
// 用于列表识别与关键字检索。正文允许包含 {{名称}} 占位符，解析与替换在前端
// 完成（后端只负责存取原文，不关心模板语义）。
type Prompt struct {
	ID      string `json:"id"`      // 16 位小写 hex 随机 ID；熵源失败退化为 p<纳秒>
	Title   string `json:"title"`   // 标题，去首尾空白，不超过 PromptTitleMaxRunes
	Content string `json:"content"` // 正文，保留换行，不做富文本
	// CreatedAt 创建时间（Unix 毫秒）。编辑不改变该字段。
	CreatedAt int64 `json:"createdAt"`
	// UpdatedAt 最近编辑时间（Unix 毫秒）；创建时等于 CreatedAt。
	UpdatedAt int64 `json:"updatedAt"`
}
