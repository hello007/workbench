package model

import (
	"fmt"
	"time"
)

// Directory 工作目录配置
type Directory struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	IsDefault  bool      `json:"isDefault"`
	CreateTime time.Time `json:"createTime"`
}

// NewDirectory 创建新的工作目录
func NewDirectory(name, path string, isDefault bool) *Directory {
	return &Directory{
		ID:         fmt.Sprintf("dir-%d", time.Now().UnixNano()),
		Name:       name,
		Path:       path,
		IsDefault:  isDefault,
		CreateTime: time.Now(),
	}
}

// Validate 验证工作目录配置
func (d *Directory) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("目录名称不能为空")
	}
	if d.Path == "" {
		return fmt.Errorf("目录路径不能为空")
	}
	return nil
}

// FileTreeNode 文件树节点
type FileTreeNode struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Path        string           `json:"path"`
	Type        string           `json:"type"`
	IsGitRepo   bool             `json:"isGitRepo"`
	HasChildren bool             `json:"hasChildren"`
	Children    []*FileTreeNode  `json:"children,omitempty"`
	IsLeaf      bool             `json:"isLeaf"`
}

// NewFileTreeNode 创建文件树节点
func NewFileTreeNode(name, path, fileType string) *FileTreeNode {
	return &FileTreeNode{
		ID:          path,
		Name:        name,
		Path:        path,
		Type:        fileType,
		IsGitRepo:   false,
		HasChildren: fileType == "directory",
		IsLeaf:      fileType == "file",
	}
}

// GitCommit Git提交记录
type GitCommit struct {
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
	Message string    `json:"message"`
}

// ShortHash 返回短哈希（前7位）
func (c *GitCommit) ShortHash() string {
	if len(c.Hash) > 7 {
		return c.Hash[:7]
	}
	return c.Hash
}

// GitRepoInfo Git仓库信息
type GitRepoInfo struct {
	Path      string      `json:"path"`
	Branch    string      `json:"branch"`
	Remote    string      `json:"remote"`
	RemoteURL string      `json:"remoteUrl"`
	Commits   []GitCommit `json:"commits"`
	IsRepo    bool        `json:"isRepo"`
}

// PageResult 分页结果
type PageResult struct {
	Records interface{} `json:"records"`
	Total   int64       `json:"total"`
	Current int         `json:"current"`
	Size    int         `json:"size"`
	Pages   int         `json:"pages"`
}

// NewPageResult 创建分页结果
func NewPageResult(records interface{}, total int64, current, size int) *PageResult {
	pages := int(total) / size
	if int(total)%size != 0 {
		pages++
	}

	return &PageResult{
		Records: records,
		Total:   total,
		Current: current,
		Size:    size,
		Pages:   pages,
	}
}

// FilePreview 文件预览
type FilePreview struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Content  string `json:"content,omitempty"`
	IsBinary bool   `json:"isBinary"`
	TooLarge bool   `json:"tooLarge"`
	Error    string `json:"error,omitempty"`
}
