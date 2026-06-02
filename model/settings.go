package model

// AppSettings 应用设置
type AppSettings struct {
	GpuDisabled  bool   `json:"gpuDisabled"`
	DefaultShell string `json:"defaultShell"` // 默认 Shell 类型：powershell/cmd/gitbash/wsl
	GitBashPath  string `json:"gitBashPath"`  // Git Bash 自定义路径
	WslDistro    string `json:"wslDistro"`    // WSL 发行版名称
}
