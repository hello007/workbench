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

// FileCommit 文件级提交记录（git log --follow 单文件演进史）。与仓库级 Commit
// 独立成契约：文件级历史无 Files 列表（恒为同一文件），新增重命名检出与该提交
// 时刻路径两个字段，供前端 rename 标记与 diff 查看定位。
type FileCommit struct {
	SHA       string `json:"sha"`       // 完整的 40 位 SHA
	ShortSHA  string `json:"shortSha"`  // 前 8 位 SHA，用于显示
	Message   string `json:"message"`   // 提交消息
	Author    string `json:"author"`    // 作者名称
	Email     string `json:"email"`     // 作者邮箱
	Timestamp int64  `json:"timestamp"` // Unix 时间戳
	DateTime  string `json:"dateTime"`  // 格式化的时间字符串
	// PathAtCommit 该提交之后文件的仓库相对路径（--follow 历史中随重命名逐段变化；
	// rename 提交取新路径）。供 diff 查看的 pathspec（GetCommitFileDiff 以相对
	// gitRoot 路径过滤，绝对路径或旧路径会取不到 rename 提交与旧段提交的变更）。
	PathAtCommit string `json:"pathAtCommit"`
	// RenameFrom 非空表示该提交重命名/复制了目标文件（--name-status R/C 行），
	// 值为变更前路径；RenameTo 为变更后路径。
	RenameFrom string `json:"renameFrom,omitempty"`
	RenameTo   string `json:"renameTo,omitempty"`
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

// BranchSyncInfo 当前分支与上游跟踪分支的同步状态摘要 + 关键 refs 位置映射，
// 供提交历史视图渲染「本地 vs 远程」同步摘要条与提交行 refs badge。
type BranchSyncInfo struct {
	// Branch 当前分支名；detached HEAD 时为短 SHA。
	Branch string `json:"branch"`
	// Ahead 本地领先上游的提交数（未推送）。
	Ahead int `json:"ahead"`
	// Behind 本地落后上游的提交数（未拉取，基于上次 fetch 的本地远程引用快照）。
	Behind int `json:"behind"`
	// HasUpstream 当前分支是否配置跟踪上游；false 时 Ahead/Behind 无意义置 0。
	HasUpstream bool `json:"hasUpstream"`
	// Detached HEAD 是否处于分离头指针状态。
	Detached bool `json:"detached"`
	// HeadSha 当前 HEAD 指向的完整 SHA。
	HeadSha string `json:"headSha"`
	// Refs 关键引用位置映射（HEAD/当前分支/上游），提交行按 sha 匹配渲染 badge。
	Refs []CommitRef `json:"refs"`
}

// CommitRef 分支同步摘要的引用位置条目：某个引用当前指向的 commit。
// Kind 用普通 string 字段 + 常量表达（不用具名 string 类型，规避 wails 不为
// 具名 string 生成 models.ts 别名导致 npm run build MISSING_EXPORT，
// 见 docs/spec/cross-layer-contracts.md）。
type CommitRef struct {
	// Sha 引用指向的完整 SHA。
	Sha string `json:"sha"`
	// Kind 引用类别，取值见 CommitRefKind 系列常量。
	Kind string `json:"kind"`
	// Name 引用展示名：HEAD / 分支名（如 master）/ 上游名（如 origin/master）。
	Name string `json:"name"`
}

// CommitRef Kind 取值常量。
const (
	// CommitRefKindHead 分离头指针状态下的 HEAD 位置标记。
	CommitRefKindHead = "head"
	// CommitRefKindLocal 当前本地分支位置标记。
	CommitRefKindLocal = "local"
	// CommitRefKindRemote 当前分支上游远程分支位置标记（如 origin/master）。
	CommitRefKindRemote = "remote"
)

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

// ConflictFileContent 冲突文件工作区内容快照（内嵌冲突解决器加载用）。
// Binary/TooLarge 任一为 true 时 Content 为空，前端拦截进解决器（提示走选边或外部工具）。
type ConflictFileContent struct {
	Content  string `json:"content"`  // 解码为 UTF-8 后的工作区全文（含冲突标记）
	Encoding string `json:"encoding"` // 原文件编码（utf-8/gbk），空串等同于 utf-8
	Binary   bool   `json:"binary"`   // 二进制文件（含 NUL 或非 UTF-8 且 GBK 解码失败）
	TooLarge bool   `json:"tooLarge"` // 超过 1MB 上限，未读内容
}

// GitSubmodule 表示 superproject 下的一个 Git submodule。
// 数据由 `git submodule status`（前导码/SHA/path/describe/init 态）与
// `git status --porcelain=2`（dirty 标记）双命令融合产出，按 path 关联。
type GitSubmodule struct {
	Path        string `json:"path"`        // submodule 在 superproject 中的相对路径
	Sha         string `json:"sha"`         // 当前 checkout 的提交 SHA（40 位）
	ShortSha    string `json:"shortSha"`    // 前 8 位 SHA，用于显示
	Describe    string `json:"describe"`    // git describe 结果，未初始化或无 tag 时为空
	Branch      string `json:"branch"`      // .gitmodules 中配置的跟踪分支，未配置时为空
	Url         string `json:"url"`         // .gitmodules 中配置的仓库地址
	Initialized bool   `json:"initialized"` // 是否已初始化（前导码 - 表示未初始化）
	ShaMismatch bool   `json:"shaMismatch"` // checkout SHA 与 index 记录不一致（前导码 +）
	Dirty       bool   `json:"dirty"`       // submodule 工作区有未提交改动（来自 porcelain=2）
	Conflict    bool   `json:"conflict"`    // superproject 合并冲突（前导码 U）
	Detached    bool   `json:"detached"`    // submodule HEAD 处于分离头指针状态
}

// SubmoduleUpdateMode submodule 更新策略，对应 git submodule update 的 --merge/--rebase/--remote 选项。
// checkout 为默认（git 原生行为，检出 index 记录 SHA，结果 detached）。
type SubmoduleUpdateMode string

const (
	// SubmoduleUpdateCheckout 默认检出：按 superproject index 记录的 SHA 检出，结果为 detached HEAD。
	SubmoduleUpdateCheckout SubmoduleUpdateMode = "checkout"
	// SubmoduleUpdateMerge 合并更新：把远程 tip 合并进 submodule 当前分支，保持在分支上。
	SubmoduleUpdateMerge SubmoduleUpdateMode = "merge"
	// SubmoduleUpdateRebase 变基更新：把 submodule 当前分支变基到远程 tip。
	SubmoduleUpdateRebase SubmoduleUpdateMode = "rebase"
	// SubmoduleUpdateRemote 远程升级：忽略 index SHA，拉 .gitmodules 的 branch 远程最新（后须 superproject commit 指针）。
	SubmoduleUpdateRemote SubmoduleUpdateMode = "remote"
)

// StashEntry 表示一个 stash 暂存条目（git stash list 单行解析产物）。
type StashEntry struct {
	// Index stash 序号（stash@{n} 的 n，0 = 最新）。
	Index int `json:"index"`
	// Description 条目描述：自定义 -m 描述或 git 默认文案（"WIP on ..."）。
	Description string `json:"description"`
	// Branch 创建条目时所在分支（自 reflog 主题解析，解析失败为空）。
	Branch string `json:"branch"`
	// ShortSha 基点提交短 SHA（reflog 主题冒号后段，解析失败为空）。
	ShortSha string `json:"shortSha"`
	// DateTime 创建时间展示串（本地时区 "2006-01-02 15:04"，解析失败为空）。
	DateTime string `json:"dateTime"`
	// Timestamp 创建时间 Unix 秒（解析失败为 0）。
	Timestamp int64 `json:"timestamp"`
}

// StashApplyResult stash pop/apply 操作结果。
// ConflictFiles 非空 = 冲突发生：git 默认保留 stash 条目（pop 亦不 drop），
// 工作区留下未合并文件，冲突解决走「合并/变基」tab 既有链路；
// ConflictFiles 为空 = 正常完成。
type StashApplyResult struct {
	// Output git 命令完整输出（stdout + stderr 合并），供前端超长展示。
	Output string `json:"output"`
	// ConflictFiles 冲突文件相对路径列表（porcelain 未合并态，权威判定）。
	ConflictFiles []string `json:"conflictFiles"`
}
