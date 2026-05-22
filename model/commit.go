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

// FileChange 表示本地变动文件
type FileChange struct {
	Path   string `json:"path"`   // 文件相对路径
	Status string `json:"status"` // 变更状态: M/A/D/R/?
	Staged bool   `json:"staged"` // 是否已暂存
}

// BranchInfo 分支信息
type BranchInfo struct {
	Name      string `json:"name"`
	IsRemote  bool   `json:"isRemote"`
	IsCurrent bool   `json:"isCurrent"`
}

// BranchList 分支列表
type BranchList struct {
	Branches []BranchInfo `json:"branches"`
}

// StatusLabel 返回状态的可读标签
func (f *FileChange) StatusLabel() string {
	switch f.Status {
	case "M":
		return "已修改"
	case "A":
		return "已添加"
	case "D":
		return "已删除"
	case "R":
		return "已重命名"
	case "?":
		return "未跟踪"
	default:
		return f.Status
	}
}
