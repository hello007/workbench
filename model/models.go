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
