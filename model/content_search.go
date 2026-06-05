// model/content_search.go
package model

// ContentSearchResult 内容搜索单条结果
type ContentSearchResult struct {
	RepoName string `json:"repoName"` // 仓库名
	RepoPath string `json:"repoPath"` // 仓库绝对路径
	FilePath string `json:"filePath"` // 相对路径
	LineNum  int    `json:"lineNum"`  // 行号
	LineText string `json:"lineText"` // 匹配行内容
}

// ContentSearchGroup 按仓库分组的搜索结果
type ContentSearchGroup struct {
	RepoName string                 `json:"repoName"`
	RepoPath string                 `json:"repoPath"`
	Items    []*ContentSearchResult `json:"items"`
}

// ContentSearchQuery 解析后的搜索查询参数
type ContentSearchQuery struct {
	Keyword   string // 搜索关键词
	FileExt   string // 文件类型过滤（如 ".java"），为空则不过滤
	SubDir    string // 子目录路径（相对于工作目录），为空则搜索整个目录
	SearchAll bool   // 是否搜索所有工作目录
}
