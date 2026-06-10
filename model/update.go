package model

// UpdateInfo 版本检查结果
type UpdateInfo struct {
	HasUpdate    bool   `json:"hasUpdate"`    // 是否有新版本
	CurrentVer   string `json:"currentVer"`   // 当前版本
	LatestVer    string `json:"latestVer"`    // 最新版本
	DownloadURL  string `json:"downloadUrl"`  // 下载地址
	ReleaseNotes string `json:"releaseNotes"` // 更新日志
	PublishedAt  string `json:"publishedAt"`  // 发布时间
	FileSize     int64  `json:"fileSize"`     // 文件大小（字节）
}

// DownloadProgress 下载进度
type DownloadProgress struct {
	TotalBytes    int64   `json:"totalBytes"`    // 总字节数
	Downloaded    int64   `json:"downloaded"`    // 已下载字节数
	Percent       float64 `json:"percent"`       // 下载百分比 0-100
	Speed         string  `json:"speed"`          // 下载速度（格式化字符串）
	Completed     bool    `json:"completed"`     // 是否完成
	Error         string  `json:"error"`         // 错误信息
}
