package model

// Commit 表示一个 Git 提交记录
type Commit struct {
	SHA       string   `json:"sha"`       // 完整的 40 位 SHA
	ShortSHA  string   `json:"shortSha"`  // 前 8 位 SHA，用于显示
	Message   string   `json:"message"`   // 提交消息
	Author    string   `json:"author"`    // 作者名称
	Email     string   `json:"email"`     // 作者邮箱
	Timestamp int64    `json:"timestamp"` // Unix 时间戳
	DateTime  string   `json:"dateTime"`  // 格式化的时间字符串
	Files     []string `json:"files"`     // 变更的文件路径列表
}

// GitRemoteInfo 表示 Git 远程仓库信息
type GitRemoteInfo struct {
	RemoteURL  string `json:"remoteUrl"`  // 远程仓库地址
	Branch     string `json:"branch"`     // 当前分支名称
	IsDetached bool   `json:"isDetached"` // 是否处于分离头指针状态
}
