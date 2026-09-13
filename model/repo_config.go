package model

import "time"

// RepoConfigManifestVersion 仓库列表配置导出文件当前 manifest 版本。
// 结构演进（增删字段）时 +1，并在 service.RepoConfigService 解析侧新增兼容分支。
// v1：{manifestVersion, exportedAt, directories, favorites}，工作目录仅业务字段
// （name/path/isDefault），收藏夹全字段（path/alias/group/createdAt）。
const RepoConfigManifestVersion = 1

// RepoConfigManifest 仓库列表配置导出文件顶层结构。
// 导出侧由 service.RepoConfigService 组装；导入侧按 ManifestVersion 判定兼容性，
// 高于 RepoConfigManifestVersion 的版本整体拒绝（结构未知，宁拒不误并）。
type RepoConfigManifest struct {
	ManifestVersion int                    `json:"manifestVersion"`
	ExportedAt      time.Time              `json:"exportedAt"`
	Directories     []*RepoConfigDirectory `json:"directories"`
	Favorites       []*RepoConfigFavorite  `json:"favorites"`
}

// RepoConfigDirectory 导出文件中的工作目录条目（仅业务字段）。
// 不含运行时检测字段（IsGitRepo/HasRemote，导入时按本机重算）与本机标识（ID/CreateTime）。
type RepoConfigDirectory struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDefault bool   `json:"isDefault"`
}

// RepoConfigFavorite 导出文件中的收藏夹条目（全字段，收藏夹无本机运行时态）。
type RepoConfigFavorite struct {
	Path      string `json:"path"`
	Alias     string `json:"alias"`
	Group     string `json:"group"`
	CreatedAt int64  `json:"createdAt"`
}

// RepoConfigDirectoryPreview 导入预览中的工作目录条目（manifest 字段直传，供前端展示）。
type RepoConfigDirectoryPreview struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDefault bool   `json:"isDefault"`
}

// RepoConfigFavoritePreview 导入预览中的收藏夹条目。
type RepoConfigFavoritePreview struct {
	Path      string `json:"path"`
	Alias     string `json:"alias"`
	Group     string `json:"group"`
	CreatedAt int64  `json:"createdAt"`
}

// RepoConfigInvalidItem 导入预览中的非法项：不导入，带原因供前端逐条展示。
// Kind 区分来源列表，Name 取路径（空路径项取名称兜底）。
type RepoConfigInvalidItem struct {
	Kind   string `json:"kind"`   // directory / favorite
	Name   string `json:"name"`   // 展示用，一般取 path
	Reason string `json:"reason"` // 非法原因（中文，可直接展示）
}

// RepoConfigImportPreview 导入预览：解析+校验+与本机比对后的分类结果，不落盘。
// 冲突判定键为 path（工作目录取规范化绝对路径，收藏夹取原 path）。
type RepoConfigImportPreview struct {
	NewDirectories      []*RepoConfigDirectoryPreview `json:"newDirectories"`      // 本机不存在，将新增
	ConflictDirectories []*RepoConfigDirectoryPreview `json:"conflictDirectories"` // path 已存在，待决策
	NewFavorites        []*RepoConfigFavoritePreview  `json:"newFavorites"`        // 本机不存在，将新增
	ConflictFavorites   []*RepoConfigFavoritePreview  `json:"conflictFavorites"`   // path 已存在，待决策
	Invalid             []*RepoConfigInvalidItem      `json:"invalid"`             // 非法项，不导入
}

// 冲突决策取值常量（Decisions map 的 value）。
const (
	ImportDecisionSkip      = "skip"      // 跳过，保留本机
	ImportDecisionOverwrite = "overwrite" // 覆盖本机同 path 项（保留本机 ID）
	ImportDecisionSaveAsNew = "saveAsNew" // 保留本机 + 导入项以新身份并存
)

// RepoConfigImportDecisions 用户对冲突项的逐项决策，key 为冲突项 path。
// 仅冲突项需要决策；新增项无决策直接导入，非法项不导入。
type RepoConfigImportDecisions struct {
	Directories map[string]string `json:"directories"` // path -> skip / overwrite / saveAsNew
	Favorites   map[string]string `json:"favorites"`   // path -> skip / overwrite / saveAsNew
}

// RepoConfigImportResult 导入执行结果汇总，供前端展示计数。
// 另存为新项计入 Added；FailedReasons 收集落盘失败与收藏夹超限等逐项原因。
type RepoConfigImportResult struct {
	Added         int      `json:"added"`                   // 新增条数（含另存为新项）
	Overwritten   int      `json:"overwritten"`             // 覆盖条数
	Skipped       int      `json:"skipped"`                 // 跳过条数
	Failed        int      `json:"failed"`                  // 失败条数
	FailedReasons []string `json:"failedReasons,omitempty"` // 失败明细（中文原因）
}
