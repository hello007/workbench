package model

import (
	"os"
	"os/exec"
	"sync"
)

// TerminalSession 终端会话
type TerminalSession struct {
	ID        string      `json:"id"`
	Dir       string      `json:"dir"`
	ShellType string      `json:"shellType"`
	Running   bool        `json:"running"`
	mu        sync.Mutex
	ptyFile   *os.File
	cmd       *exec.Cmd
}

// SetRunning 安全设置运行状态
func (s *TerminalSession) SetRunning(running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Running = running
}

// IsRunning 安全获取运行状态
func (s *TerminalSession) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Running
}

// ShellConfig Shell 配置
type ShellConfig struct {
	Type        string   `json:"type"`
	Executable  string   `json:"executable"`
	Args        []string `json:"args,omitempty"`
	DisplayName string   `json:"displayName"`
}

// GetShellConfigs 获取所有可用的 Shell 配置
func GetShellConfigs() []ShellConfig {
	return []ShellConfig{
		{Type: "powershell", Executable: "powershell.exe", DisplayName: "PowerShell"},
		{Type: "cmd", Executable: "cmd.exe", DisplayName: "CMD"},
		{Type: "gitbash", Executable: `C:\Program Files\Git\bin\bash.exe`, Args: []string{"-i"}, DisplayName: "Git Bash"},
		{Type: "wsl", Executable: "wsl.exe", DisplayName: "WSL"},
	}
}

// ResolveShellConfig 根据类型解析 Shell 配置，支持自定义路径覆盖
func ResolveShellConfig(shellType, customPath string) *ShellConfig {
	configs := GetShellConfigs()
	for _, c := range configs {
		if c.Type == shellType {
			config := c
			if customPath != "" {
				config.Executable = customPath
			}
			return &config
		}
	}
	// 默认返回 PowerShell
	return &ShellConfig{Type: "powershell", Executable: "powershell.exe", DisplayName: "PowerShell"}
}
