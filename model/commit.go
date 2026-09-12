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

// CommitFilter 提交历史服务端过滤条件，各字段组合语义为 AND，空字段表示该维度不参与过滤。
type CommitFilter struct {
	Author   string `json:"author,omitempty"`   // 作者 Name+Email 子串匹配（大小写不敏感）
	Keyword  string `json:"keyword,omitempty"`  // 提交消息子串匹配（大小写不敏感）
	Since    string `json:"since,omitempty"`    // 起始日期 YYYY-MM-DD（含当天 00:00:00）
	Until    string `json:"until,omitempty"`    // 截止日期 YYYY-MM-DD（含当天 23:59:59）
	FilePath string `json:"filePath,omitempty"` // 文件路径子串匹配（目录级，大小写不敏感）
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

// GitTag 表示一个 Git 标签
type GitTag struct {
	Name     string `json:"name"`     // 标签名
	Type     string `json:"type"`     // 类型: lightweight|annotated
	Sha      string `json:"sha"`      // 指向的提交 SHA
	ShortSha string `json:"shortSha"` // 前 8 位 SHA，用于显示
	Message  string `json:"message"`  // 注释消息（轻量标签为空）
	Tagger   string `json:"tagger"`   // 标注者（轻量标签为空）
	Date     string `json:"date"`     // 标注时间（轻量标签为空）
}

// GitRemote 表示一个 Git 远程仓库
type GitRemote struct {
	Name string `json:"name"` // 远程仓库名
	URL  string `json:"url"`  // 远程仓库地址（取首个 URL）
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

// MergeMode 合并策略，对应 git merge 的 --ff/--no-ff/--squash 选项。
type MergeMode string

const (
	// MergeModeFF 快进合并：当前分支为目标分支祖先时直接前移指针，不产生合并提交。
	MergeModeFF MergeMode = "ff"
	// MergeModeNoFF 强制产生合并提交：即使可快进也创建合并节点，保留分支拓扑。
	MergeModeNoFF MergeMode = "no-ff"
	// MergeModeSquash 压缩合并：将目标分支多个提交压缩为单个暂存变更，不产生合并节点。
	MergeModeSquash MergeMode = "squash"
)

// ConflictType 冲突来源操作类型，标识当前进行中的 merge/rebase/cherry-pick。
type ConflictType string

const (
	ConflictTypeNone       ConflictType = "none"        // 无进行中的冲突态
	ConflictTypeMerge      ConflictType = "merge"       // git merge 产生的冲突
	ConflictTypeRebase     ConflictType = "rebase"      // git rebase 产生的冲突
	ConflictTypeCherryPick ConflictType = "cherry-pick" // git cherry-pick 产生的冲突
)

// ConflictState 冲突态快照：当前进行中的操作类型 + 未解决冲突文件列表。
// 前端据此渲染冲突解决面板，Type=none 时隐藏面板。
type ConflictState struct {
	Type  ConflictType `json:"type"`  // 冲突来源操作
	Files []string     `json:"files"` // 冲突文件相对仓库根路径
}
