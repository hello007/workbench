package model

// 行级归属追溯（git blame）契约模型。
//
// BlameResult.Lines 与工作树文件行号 1:1（下标 = 行号-1），值为该行归属提交在
// Commits 中的下标。前端以此把 CodeMirror 文档行映射到提交元数据；行数须与
// 预览文档行数一致，不一致说明预览读取后文件被外部修改（前端提示刷新）。
//
// 未提交行：工作树 blame（不传 rev）中尚未提交的行归属 SHA 为全零
// （000…0，git 契约），前端按 ^0+$ 判定并禁跳转，后端不加冗余标志字段。

// BlameCommit 行归属提交元数据（Commits 去重列表条目，首次出现序）。
type BlameCommit struct {
	SHA       string `json:"sha"`       // 完整的 40 位 SHA；未提交行为全零
	ShortSHA  string `json:"shortSha"`  // 前 8 位 SHA，用于显示
	Message   string `json:"message"`   // 提交消息（summary 首段）
	Author    string `json:"author"`    // 作者名称
	Email     string `json:"email"`     // 作者邮箱
	Timestamp int64  `json:"timestamp"` // 作者时间 Unix 时间戳（前端相对时间格式化）
	DateTime  string `json:"dateTime"`  // 作者时区墙钟时间串（口径对齐 FileCommit.DateTime）
}

// BlameResult 行归属结果。
type BlameResult struct {
	Commits []BlameCommit `json:"commits"` // 去重提交列表（首次出现序）
	Lines   []int         `json:"lines"`   // 每行 → Commits 下标（下标 = 文件行号-1）
}
