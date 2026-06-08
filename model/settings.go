package model

// AppSettings 应用设置
type AppSettings struct {
	GpuDisabled        bool     `json:"gpuDisabled"`
	DefaultShell       string   `json:"defaultShell"`       // 默认 Shell 类型：powershell/cmd/gitbash/wsl
	GitBashPath        string   `json:"gitBashPath"`        // Git Bash 自定义路径
	WslDistro          string   `json:"wslDistro"`          // WSL 发行版名称
	SearchExcludeDirs  []string `json:"searchExcludeDirs"`  // 搜索排除目录
	SearchExcludeFiles     []string `json:"searchExcludeFiles"`     // 搜索排除文件模式
	ShortcutCommandPalette string   `json:"shortcutCommandPalette"` // 命令面板快捷键，默认 "Ctrl+P"
	ShortcutToggleTerminal string   `json:"shortcutToggleTerminal"` // 切换终端快捷键，默认 "Ctrl+`"
}
