package model

import "time"

// RepoStatus 单个仓库的状态快照，供全局状态看板展示。
// 字段语义：
//   - Dirty：工作区有未提交改动（含未暂存/已暂存/未跟踪，git status --porcelain 非空）
//   - Ahead：本地领先上游的提交数（git rev-list --left-right --count @{u}...HEAD 右值）
//   - Behind：本地落后上游的提交数（同命令左值）
//   - HasUpstream：当前分支是否配置跟踪上游；false 时 Ahead/Behind 无意义置 0
//   - Detached：HEAD 处于 detached 状态（非分支），ahead/behind 无上游可比
//
// ahead/behind 基于本地已有远程引用（上次 fetch/clone 快照），看板不主动 fetch，
// behind 为「相对上次 fetch」的近似值，前端标注时效。
type RepoStatus struct {
	// Path 仓库绝对路径（规范化主键，与 PinnedRepos.Paths 一致）。
	Path string `json:"path"`
	// Name 仓库名称（路径末段），用于列表展示。
	Name string `json:"name"`
	// Branch 当前分支名；detached HEAD 时为短 SHA 或空。
	Branch string `json:"branch"`
	// Dirty 工作区是否有未提交改动。
	Dirty bool `json:"dirty"`
	// Ahead 本地领先上游的提交数。
	Ahead int `json:"ahead"`
	// Behind 本地落后上游的提交数。
	Behind int `json:"behind"`
	// HasUpstream 当前分支是否配置跟踪上游。
	HasUpstream bool `json:"hasUpstream"`
	// Detached HEAD 是否处于 detached 状态。
	Detached bool `json:"detached"`
	// IsRepo 路径是否为有效 Git 仓库（false 时其余字段无意义，用于失效仓库降级展示）。
	IsRepo bool `json:"isRepo"`
	// Missing 路径已失效（pin 后目录被删除），前端灰显 + 失效标记，不阻塞看板。
	Missing bool `json:"missing"`
	// Error 状态计算错误信息（非致命，前端展示警告而非阻塞）。
	Error string `json:"error,omitempty"`
}

// PinnedRepos 全局状态看板的 pin 仓库列表，持久化到 data/dashboard_pinned.json。
// 路径经 filepath.Abs 规范化作主键（与 RepoMeta/DirectoryService 一致）。
// 与 RepoMeta 职责分离：RepoMeta 存用户元数据（简述/标签），PinnedRepos 仅存关注路径列表。
type PinnedRepos struct {
	// Paths pin 的仓库绝对路径列表（规范化）。
	Paths []string `json:"paths"`
	// UpdatedAt 最后一次增删 pin 的时间。
	UpdatedAt time.Time `json:"updatedAt"`
}
