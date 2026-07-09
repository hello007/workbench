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
	// IsGitRepo 表示该工作目录路径本身是否为 git 仓库根。
	// 运行时检测填充，不参与业务持久化语义；旧配置文件无此字段时反序列化零值为 false，天然兼容。
	IsGitRepo bool `json:"isGitRepo"`
	// HasRemote 表示该 git 仓库是否配置了远程仓库（仅 IsGitRepo=true 时有意义）。
	// 用于一键更新跳过无远程仓库及前端灰色图标区分；运行时检测填充，旧配置零值 false 兼容。
	HasRemote bool `json:"hasRemote"`
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
	// HasRemote 表示该 git 仓库节点是否配置了远程仓库（仅 IsGitRepo=true 时有意义），
	// 用于前端灰色图标区分无远程仓库。
	HasRemote   bool             `json:"hasRemote"`
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
	// Kind 预览类型分流，供前端按类型选择渲染器：text/image/pdf/office/unsupported。
	// 与 IsBinary 互补：图片/PDF/Office 虽是二进制，但可预览，需区别于"无法预览的二进制"。
	Kind string `json:"kind,omitempty"`
	// Encoding 文本编码来源（utf-8/gbk），仅 text 类预览填充，供前端保存时按原编码回写。
	// 空串等同于 utf-8；unsupported 降级为 text 时同样填充。
	Encoding string `json:"encoding,omitempty"`
}

// 预览类型常量
const (
	KindText        = "text"        // 文本类（txt/json/sql/md/代码等）
	KindImage       = "image"       // 图片（jpg/png/bmp/gif/webp 等）
	KindPDF         = "pdf"         // PDF
	KindOffice      = "office"      // Office 文档（doc/docx/ppt/pptx/xls/xlsx 等）
	KindUnsupported = "unsupported" // 不支持内嵌预览
)

// FileBytes 文件原始字节（base64），供前端构造 Blob 喂给预览组件（图片/PDF/Office）
type FileBytes struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Kind     string `json:"kind,omitempty"`
	Base64   string `json:"base64,omitempty"` // base64 编码的字节
	TooLarge bool   `json:"tooLarge"`
	Error    string `json:"error,omitempty"`
}

// PullResult 单个仓库的拉取结果
type PullResult struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Success bool   `json:"success"`
	// Skipped 表示因无远程配置被跳过（未执行 pull），与 Success 互斥：
	// 不计入成功也不计入失败，用于一键更新跳过无远程仓库。
	Skipped bool   `json:"skipped"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// PullSummary ScanAndPullRepos 的初始返回值
type PullSummary struct {
	Total int `json:"total"`
}

// Favorite 收藏夹条目
type Favorite struct {
	Path      string `json:"path"`
	Alias     string `json:"alias,omitempty"`
	Group     string `json:"group"`
	CreatedAt int64  `json:"createdAt"`
}

// SearchResult 文件搜索结果
type SearchResult struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}
