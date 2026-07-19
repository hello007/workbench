package model

import "time"

// RepoMeta 仓库用户元数据，按规范化路径（filepath.Abs）作主键持久化到 data/repo_meta.json。
// 与 DirectoryService 一致采用 filepath.Abs 规范化路径，规避 Windows 下大小写/分隔符歧义。
type RepoMeta struct {
	// Path 规范化后的绝对路径，作为主键。写入时由 RepoMetaService 统一 filepath.Abs。
	Path string `json:"path"`
	// Summary 用户自定义简述（手动录入），与 ReadmeSummary 分离，互不覆盖。
	Summary string `json:"summary,omitempty"`
	// Tags 用户自定义标签列表，纯自由输入，无预设库。
	// "已编辑"Tab 定义：添加过至少一个标签即为已编辑（简述填写不影响分类）。
	Tags []string `json:"tags,omitempty"`
	// ReadmeSummary 自动解析的 README 摘要（缓存，避免重复读盘）。
	// 首次扫描时解析写入，后续扫描沿用缓存；空串表示无 README 或解析为空。
	ReadmeSummary string `json:"readmeSummary,omitempty"`
	// Missing 扫描时路径已失效（不在最新扫描结果中），前端灰显 + "失效"标记。
	// 不自动删除，由 CleanMissingRepoMeta 手动清理。
	Missing bool `json:"missing,omitempty"`
	// UpdatedAt 元数据最后更新时间（Summary/Tags 变更时刷新）。
	UpdatedAt time.Time `json:"updatedAt"`
	// LastScanAt 最后一次扫描命中时间，同时作为 README 摘要是否已解析的标记：
	// 零值表示尚未解析过 README，需重新解析；非零表示 ReadmeSummary 已是权威缓存。
	LastScanAt time.Time `json:"lastScanAt,omitempty"`
}

// RepoFilterItem 仓库筛选器返回前端的列表项，合并扫描结果与用户元数据。
type RepoFilterItem struct {
	// Name 仓库名称（路径末段），用于列表展示。
	Name string `json:"name"`
	// Path 仓库绝对路径（主键，前端据此定位/保存元数据）。
	Path string `json:"path"`
	// Summary 用户自定义简述（手动录入）。
	Summary string `json:"summary,omitempty"`
	// Tags 用户自定义标签列表。
	Tags []string `json:"tags,omitempty"`
	// ReadmeSummary 自动解析的 README 摘要（只读展示），缺失为空串。
	ReadmeSummary string `json:"readmeSummary,omitempty"`
	// Missing 路径已失效，前端灰显 + "失效"标记。
	Missing bool `json:"missing,omitempty"`
	// HasRemote 是否配置了远程仓库，用于前端灰色图标区分无远程仓库。
	HasRemote bool `json:"hasRemote"`
	// IsGitRepo 是否为 Git 仓库（扫描结果均为 true，保留字段供前端统一处理）。
	IsGitRepo bool `json:"isGitRepo"`
}
