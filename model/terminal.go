package model

import (
	"os"
	"os/exec"
	"runtime"
	"sync"
)

// TerminalSession 终端会话
type TerminalSession struct {
	ID        string `json:"id"`
	Dir       string `json:"dir"`
	ShellType string `json:"shellType"`
	Running   bool   `json:"running"`
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

// MarkExited 标记会话已退出，返回本次调用是否完成了「运行中 → 已退出」的状态跃迁。
//
// 输出泵（Read 结束）与进程等待（事件通道/轮询）双通道都可能判定退出，且存在
// 同时判定的竞态窗口。调用方仅在返回 true 时发送 terminal-exit，保证同一会话
// 的退出事件只发一次：先到者跃迁成功并发事件，后到者看到 Running 已为 false
// 返回 false 静默退出。
func (s *TerminalSession) MarkExited() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Running {
		return false
	}
	s.Running = false
	return true
}

// ShellConfig Shell 配置
type ShellConfig struct {
	Type        string   `json:"type"`
	Executable  string   `json:"executable"`
	Args        []string `json:"args,omitempty"`
	DisplayName string   `json:"displayName"`
}

// GetShellConfigs 获取当前平台可用的 Shell 配置
func GetShellConfigs() []ShellConfig {
	if runtime.GOOS == "windows" {
		return []ShellConfig{
			{Type: "powershell", Executable: "powershell.exe", DisplayName: "PowerShell"},
			{Type: "cmd", Executable: "cmd.exe", DisplayName: "CMD"},
			{Type: "gitbash", Executable: `C:\Program Files\Git\bin\bash.exe`, Args: []string{"-i"}, DisplayName: "Git Bash"},
			{Type: "wsl", Executable: "wsl.exe", DisplayName: "WSL"},
		}
	}
	// Linux/Unix: 常见登录 shell，默认 bash；路径取各发行版通用安装位置
	return []ShellConfig{
		{Type: "bash", Executable: "/bin/bash", DisplayName: "Bash"},
		{Type: "zsh", Executable: "/bin/zsh", DisplayName: "Zsh"},
		{Type: "fish", Executable: "/usr/bin/fish", DisplayName: "Fish"},
		{Type: "sh", Executable: "/bin/sh", DisplayName: "Sh"},
	}
}

// defaultShellConfig 返回当前平台的默认 Shell 配置（Windows: PowerShell；Linux: Bash）
func defaultShellConfig() *ShellConfig {
	if runtime.GOOS == "windows" {
		return &ShellConfig{Type: "powershell", Executable: "powershell.exe", DisplayName: "PowerShell"}
	}
	return &ShellConfig{Type: "bash", Executable: "/bin/bash", DisplayName: "Bash"}
}

// ResolveShellConfig 根据类型解析 Shell 配置，支持自定义路径覆盖；
// 类型不在当前平台列表中时回退到平台默认 Shell
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
	return defaultShellConfig()
}
