# 内置终端面板实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 WorkBench 添加内置终端面板，支持 xterm.js 渲染 + Go PTY 后端，底部可拖拽可收起面板，目录自动跟随文件树。

**Architecture:** 前端用 xterm.js 渲染终端界面，后端用 Go PTY 库创建伪终端进程，通过 Wails Runtime Events 双向传输数据。Windows 平台使用 ConPTY API，Linux/macOS 使用 creack/pty。Home.vue 改造为上下 flex 布局，终端面板位于底部。

**Tech Stack:** Go PTY (ConPTY/creack/pty) | xterm.js 5.x | xterm-addon-fit | xterm-addon-web-links | Wails Events | Vue 3 Composition API

---

## 文件结构

| 操作 | 文件路径 | 职责 |
|------|---------|------|
| 新建 | `model/terminal.go` | 终端数据模型（TerminalSession） |
| 新建 | `util/pty_windows.go` | Windows 平台 PTY 封装（ConPTY） |
| 新建 | `util/pty_other.go` | Linux/macOS 平台 PTY 封装（creack/pty） |
| 新建 | `service/terminal.go` | 终端服务（创建/写入/调整/关闭/输出泵） |
| 新建 | `service/terminal_test.go` | 终端服务单元测试 |
| 修改 | `app.go` | 新增 terminalSvc 字段和终端相关 Binding 方法 |
| 修改 | `model/settings.go` | AppSettings 新增终端配置字段 |
| 修改 | `frontend/package.json` | 新增 xterm 相关依赖 |
| 新建 | `frontend/src/composables/useTerminal.js` | 终端逻辑复用组合式函数 |
| 新建 | `frontend/src/components/TerminalPanel.vue` | 终端面板组件 |
| 修改 | `frontend/src/views/Home.vue` | 布局改造（上下分区 + 终端面板集成） |
| 修改 | `frontend/src/components/ActivityBar.vue` | 新增终端图标 |
| 修改 | `frontend/src/components/SettingsPanel.vue` | 新增终端设置区域 |

---

### Task 1: 后端模型 — TerminalSession 和 Shell 路径解析

**Files:**
- Create: `model/terminal.go`
- Test: `service/terminal_test.go`（本 Task 只写模型，测试在 Task 3）

- [ ] **Step 1: 创建终端数据模型**

创建 `model/terminal.go`：

```go
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
	Type        string `json:"type"`
	Executable  string `json:"executable"`
	Args        []string `json:"args,omitempty"`
DisplayName  string `json:"displayName"`
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
```

- [ ] **Step 2: 扩展 AppSettings 模型**

修改 `model/settings.go`，在 `AppSettings` 中新增终端配置字段：

```go
package model

// AppSettings 应用设置
type AppSettings struct {
	GpuDisabled  bool   `json:"gpuDisabled"`
	DefaultShell string `json:"defaultShell"`  // 默认 Shell 类型：powershell/cmd/gitbash/wsl
	GitBashPath  string `json:"gitBashPath"`    // Git Bash 自定义路径
	WslDistro    string `json:"wslDistro"`      // WSL 发行版名称
}
```

- [ ] **Step 3: 提交**

```bash
git add model/terminal.go model/settings.go
git commit -m "feat(terminal): 添加终端数据模型和 Shell 配置"
```

---

### Task 2: 后端 PTY 工具层 — 平台适配

**Files:**
- Create: `util/pty_windows.go`
- Create: `util/pty_other.go`

- [ ] **Step 1: 安装 PTY 依赖**

```bash
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\workbench
go get github.com/iamacarpet/go-winpty
go get github.com/creack/pty
```

- [ ] **Step 2: 创建 Windows PTY 封装**

创建 `util/pty_windows.go`：

```go
//go:build windows

package util

import (
	"fmt"
	"os"
	"os/exec"
	"winpty"
)

// PtyProcess PTY 进程封装
type PtyProcess struct {
	winPty *winpty.WinPTY
}

// NewPtyProcess 创建新的 PTY 进程
func NewPtyProcess(cmd string, args []string, dir string, cols, rows uint16) (*PtyProcess, error) {
	wp, err := winpty.Open(cmd, "")
	if err != nil {
		return nil, fmt.Errorf("创建 ConPTY 失败: %w", err)
	}

	// 设置工作目录
	if dir != "" {
		wp.SetDir(dir)
	}

	// 设置窗口大小
	wp.SetSize(int(cols), int(rows))

	// 启动进程
	if err := wp.Spawn(cmd, args...); err != nil {
		wp.Close()
		return nil, fmt.Errorf("启动 Shell 进程失败: %w", err)
	}

	return &PtyProcess{winPty: wp}, nil
}

// Read 从 PTY 读取输出
func (p *PtyProcess) Read(buf []byte) (int, error) {
	return p.winPty.Read(buf)
}

// Write 向 PTY 写入输入
func (p *PtyProcess) Write(data []byte) (int, error) {
	return p.winPty.Write(data)
}

// Resize 调整 PTY 窗口大小
func (p *PtyProcess) Resize(cols, rows uint16) error {
	p.winPty.SetSize(int(cols), int(rows))
	return nil
}

// Close 关闭 PTY 进程
func (p *PtyProcess) Close() error {
	if p.winPty != nil {
		p.winPty.Close()
	}
	return nil
}

// GetCmd 获取底层命令（Windows ConPTY 不直接暴露 exec.Cmd）
func (p *PtyProcess) GetCmd() *exec.Cmd {
	return nil
}

// IsProcessRunning 检查进程是否仍在运行
func (p *PtyProcess) IsProcessRunning() bool {
	if p.winPty == nil {
		return false
	}
	return p.winPty.IsRunning()
}
```

- [ ] **Step 3: 创建 Linux/macOS PTY 封装**

创建 `util/pty_other.go`：

```go
//go:build !windows

package util

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
)

// PtyProcess PTY 进程封装
type PtyProcess struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

// NewPtyProcess 创建新的 PTY 进程
func NewPtyProcess(cmdStr string, args []string, dir string, cols, rows uint16) (*PtyProcess, error) {
	cmd := exec.Command(cmdStr, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 PTY 失败: %w", err)
	}

	return &PtyProcess{ptmx: ptmx, cmd: cmd}, nil
}

// Read 从 PTY 读取输出
func (p *PtyProcess) Read(buf []byte) (int, error) {
	return p.ptmx.Read(buf)
}

// Write 向 PTY 写入输入
func (p *PtyProcess) Write(data []byte) (int, error) {
	return p.ptmx.Write(data)
}

// Resize 调整 PTY 窗口大小
func (p *PtyProcess) Resize(cols, rows uint16) error {
	return pty.Setsize(p.ptmx, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
}

// Close 关闭 PTY 进程
func (p *PtyProcess) Close() error {
	if p.ptmx != nil {
		p.ptmx.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Signal(syscall.SIGTERM)
	}
	return nil
}

// GetCmd 获取底层 exec.Cmd
func (p *PtyProcess) GetCmd() *exec.Cmd {
	return p.cmd
}

// IsProcessRunning 检查进程是否仍在运行
func (p *PtyProcess) IsProcessRunning() bool {
	if p.cmd == nil || p.cmd.Process == nil {
		return false
	}
	// 发送 signal 0 检查进程是否存活
	err := p.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}
```

- [ ] **Step 4: 验证编译**

```bash
go build ./util/...
```

Expected: 编译成功（在 Windows 上只编译 `pty_windows.go`）

- [ ] **Step 5: 提交**

```bash
git add util/pty_windows.go util/pty_other.go go.mod go.sum
git commit -m "feat(terminal): 添加 PTY 平台适配层（Windows ConPTY / Unix PTY）"
```

---

### Task 3: 后端终端服务 — TerminalService

**Files:**
- Create: `service/terminal.go`
- Create: `service/terminal_test.go`

- [ ] **Step 1: 编写终端服务单元测试**

创建 `service/terminal_test.go`：

```go
package service

import (
	"testing"
)

func TestResolveShellConfig_Default(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("powershell", "")
	if config.Type != "powershell" {
		t.Errorf("期望 shellType=powershell, 实际=%s", config.Type)
	}
	if config.Executable != "powershell.exe" {
		t.Errorf("期望 executable=powershell.exe, 实际=%s", config.Executable)
	}
}

func TestResolveShellConfig_CustomPath(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("gitbash", "D:\\custom\\bash.exe")
	if config.Executable != "D:\\custom\\bash.exe" {
		t.Errorf("期望自定义路径, 实际=%s", config.Executable)
	}
}

func TestResolveShellConfig_UnknownType(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("unknown", "")
	if config.Type != "powershell" {
		t.Errorf("未知类型应回退到 powershell, 实际=%s", config.Type)
	}
}

func TestBuildCdCommand_Windows(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\workspace\test`)
	expected := `cd /d "D:\workspace\test"` + "\n"
	if cmd != expected {
		t.Errorf("期望 cd 命令=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\my project\test folder`)
	expected := `cd /d "D:\my project\test folder"` + "\n"
	if cmd != expected {
		t.Errorf("期望 cd 命令(含空格)=%q, 实际=%q", expected, cmd)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

```bash
go test ./service/ -run "TestResolveShellConfig|TestBuildCdCommand" -v
```

Expected: 编译失败，`NewTerminalService` 和相关方法未定义

- [ ] **Step 3: 实现终端服务**

创建 `service/terminal.go`：

```go
package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"workbench/model"
	"workbench/util"
)

type TerminalService struct {
	ctx      context.Context
	sessions map[string]*model.TerminalSession
	mu       sync.Mutex
}

func NewTerminalService(ctx context.Context) *TerminalService {
	return &TerminalService{
		ctx:      ctx,
		sessions: make(map[string]*model.TerminalSession),
	}
}

// CreateTerminal 创建终端会话
func (s *TerminalService) CreateTerminal(dir, shellType, customPath string, cols, rows uint16) (string, error) {
	config := s.resolveShellConfig(shellType, customPath)

	ptyProc, err := util.NewPtyProcess(config.Executable, config.Args, dir, cols, rows)
	if err != nil {
		return "", fmt.Errorf("创建终端失败: %w", err)
	}

	sessionID := fmt.Sprintf("term-%d", time.Now().UnixNano())
	session := &model.TerminalSession{
		ID:        sessionID,
		Dir:       dir,
		ShellType: shellType,
		Running:   true,
	}
	session.SetRunning(true)

	// 保存 PTY 进程引用（通过扩展存储）
	storePtyProcess(session, ptyProc)

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	// 启动输出泵
	go s.startOutputPump(sessionID, ptyProc)

	// 监控进程退出
	go s.watchProcess(sessionID, ptyProc)

	return sessionID, nil
}

// WriteInput 向终端写入用户输入
func (s *TerminalService) WriteInput(sessionID, input string) error {
	session, ptyProc, err := s.getSessionAndPty(sessionID)
	if err != nil {
		return err
	}
	if !session.IsRunning() {
		return fmt.Errorf("终端会话 %s 已停止", sessionID)
	}
	_, err = ptyProc.Write([]byte(input))
	return err
}

// ChangeDir 切换终端工作目录
func (s *TerminalService) ChangeDir(sessionID, dir string) error {
	session, ptyProc, err := s.getSessionAndPty(sessionID)
	if err != nil {
		return err
	}
	if !session.IsRunning() {
		return fmt.Errorf("终端会话 %s 已停止", sessionID)
	}
	cdCmd := s.buildCdCommand(dir)
	_, err = ptyProc.Write([]byte(cdCmd))
	if err != nil {
		return err
	}
	session.Dir = dir
	return nil
}

// Resize 调整终端窗口大小
func (s *TerminalService) Resize(sessionID string, cols, rows uint16) error {
	_, ptyProc, err := s.getSessionAndPty(sessionID)
	if err != nil {
		return err
	}
	return ptyProc.Resize(cols, rows)
}

// CloseTerminal 关闭终端会话
func (s *TerminalService) CloseTerminal(sessionID string) error {
	s.mu.Lock()
	session, exists := s.sessions[sessionID]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("终端会话 %s 不存在", sessionID)
	}
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	session.SetRunning(false)

	ptyProc := getPtyProcess(session)
	if ptyProc != nil {
		ptyProc.Close()
	}
	return nil
}

// CloseAll 关闭所有终端会话
func (s *TerminalService) CloseAll() {
	s.mu.Lock()
	sessions := make([]*model.TerminalSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.sessions = make(map[string]*model.TerminalSession)
	s.mu.Unlock()

	for _, session := range sessions {
		session.SetRunning(false)
		ptyProc := getPtyProcess(session)
		if ptyProc != nil {
			ptyProc.Close()
		}
	}
}

// resolveShellConfig 解析 Shell 配置
func (s *TerminalService) resolveShellConfig(shellType, customPath string) *model.ShellConfig {
	return model.ResolveShellConfig(shellType, customPath)
}

// buildCdCommand 构建 cd 命令（Windows 使用 cd /d 支持跨盘符切换）
func (s *TerminalService) buildCdCommand(dir string) string {
	// 统一使用正斜杠转义，并用引号包裹路径
	normalizedDir := filepath.Clean(dir)
	return fmt.Sprintf(`cd /d "%s"`, normalizedDir) + "\n"
}

// startOutputPump 输出泵：持续读取 PTY 输出并通过 Wails Events 推送到前端
func (s *TerminalService) startOutputPump(sessionID string, ptyProc *util.PtyProcess) {
	buf := make([]byte, 4096)
	for {
		n, err := ptyProc.Read(buf)
		if err != nil {
			// 读取失败（进程退出或管道关闭）
			s.mu.Lock()
			session, exists := s.sessions[sessionID]
			s.mu.Unlock()
			if exists {
				session.SetRunning(false)
				runtime.EventsEmit(s.ctx, "terminal-exit", sessionID)
			}
			return
		}
		if n > 0 && s.ctx != nil {
			runtime.EventsEmit(s.ctx, "terminal-output", sessionID, string(buf[:n]))
		}
	}
}

// watchProcess 监控进程退出
func (s *TerminalService) watchProcess(sessionID string, ptyProc *util.PtyProcess) {
	for {
		time.Sleep(500 * time.Millisecond)
		if !ptyProc.IsProcessRunning() {
			s.mu.Lock()
			session, exists := s.sessions[sessionID]
			s.mu.Unlock()
			if exists {
				session.SetRunning(false)
				runtime.EventsEmit(s.ctx, "terminal-exit", sessionID)
			}
			return
		}
	}
}

// getSessionAndPty 获取会话和 PTY 进程
func (s *TerminalService) getSessionAndPty(sessionID string) (*model.TerminalSession, *util.PtyProcess, error) {
	s.mu.Lock()
	session, exists := s.sessions[sessionID]
	s.mu.Unlock()
	if !exists {
		return nil, nil, fmt.Errorf("终端会话 %s 不存在", sessionID)
	}
	ptyProc := getPtyProcess(session)
	if ptyProc == nil {
		return nil, nil, fmt.Errorf("终端会话 %s PTY 进程不可用", sessionID)
	}
	return session, ptyProc, nil
}

// --- PTY 进程存储辅助 ---
// 使用 sync.Map 将 PtyProcess 与 TerminalSession 关联

var ptyStore sync.Map

func storePtyProcess(session *model.TerminalSession, proc *util.PtyProcess) {
	ptyStore.Store(session.ID, proc)
}

func getPtyProcess(session *model.TerminalSession) *util.PtyProcess {
	val, ok := ptyStore.Load(session.ID)
	if !ok {
		return nil
	}
	proc, ok := val.(*util.PtyProcess)
	if !ok {
		return nil
	}
	return proc
}
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./service/ -run "TestResolveShellConfig|TestBuildCdCommand" -v
```

Expected: 4 个测试全部 PASS

- [ ] **Step 5: 提交**

```bash
git add service/terminal.go service/terminal_test.go
git commit -m "feat(terminal): 实现终端服务（创建/写入/切换目录/调整/关闭/输出泵）"
```

---

### Task 4: 后端 App 层集成 — Binding 方法

**Files:**
- Modify: `app.go:16-23`（新增 terminalSvc 字段）
- Modify: `app.go:29-42`（startup/shutdown 集成）
- Modify: `app.go`（新增终端 Binding 方法）

- [ ] **Step 1: 修改 App 结构体和生命周期**

在 `app.go` 的 `App` 结构体中新增 `terminalSvc` 字段：

```go
type App struct {
	ctx            context.Context
	directorySvc   *service.DirectoryService
	fileTreeSvc    *service.FileTreeService
	fileOpSvc      *service.FileOperationService
	gitSvc         *service.GitService
	settingsSvc    *service.SettingsService
	terminalSvc    *service.TerminalService
}
```

在 `startup` 方法中初始化 `terminalSvc`：

```go
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dataDir := "data"
	configPath := filepath.Join(dataDir, "directories.json")
	settingsPath := filepath.Join(dataDir, "settings.json")

	a.directorySvc = service.NewDirectoryService(configPath)
	a.fileTreeSvc = service.NewFileTreeService()
	a.fileOpSvc = service.NewFileOperationService()
	a.gitSvc = service.NewGitService()
	a.settingsSvc = service.NewSettingsService(settingsPath)
	a.terminalSvc = service.NewTerminalService(ctx)

	println("WorkBench started")
}
```

在 `shutdown` 方法中清理终端：

```go
func (a *App) shutdown(context.Context) {
	if a.terminalSvc != nil {
		a.terminalSvc.CloseAll()
	}
	println("WorkBench shutting down...")
}
```

- [ ] **Step 2: 新增终端 Binding 方法**

在 `app.go` 末尾添加：

```go
// ===== 终端相关 =====

// CreateTerminal 创建终端会话
func (a *App) CreateTerminal(dir, shellType string, cols, rows uint16) (string, error) {
	// 获取自定义 Shell 路径
	var customPath string
	settings, err := a.settingsSvc.Load()
	if err == nil {
		switch shellType {
		case "gitbash":
			customPath = settings.GitBashPath
		}
	}
	return a.terminalSvc.CreateTerminal(dir, shellType, customPath, cols, rows)
}

// WriteTerminalInput 向终端写入用户输入
func (a *App) WriteTerminalInput(sessionID, input string) error {
	return a.terminalSvc.WriteInput(sessionID, input)
}

// ChangeTerminalDir 切换终端工作目录
func (a *App) ChangeTerminalDir(sessionID, dir string) error {
	return a.terminalSvc.ChangeDir(sessionID, dir)
}

// ResizeTerminal 调整终端窗口大小
func (a *App) ResizeTerminal(sessionID string, cols, rows uint16) error {
	return a.terminalSvc.Resize(sessionID, cols, rows)
}

// CloseTerminal 关闭终端会话
func (a *App) CloseTerminal(sessionID string) error {
	return a.terminalSvc.CloseTerminal(sessionID)
}

// GetShellConfigs 获取可用的 Shell 配置列表
func (a *App) GetShellConfigs() []model.ShellConfig {
	return model.GetShellConfigs()
}
```

- [ ] **Step 3: 验证编译**

```bash
go build ./...
```

Expected: 编译成功

- [ ] **Step 4: 提交**

```bash
git add app.go
git commit -m "feat(terminal): App 层集成终端服务（Binding 方法 + 生命周期管理）"
```

---

### Task 5: 前端依赖 — 安装 xterm.js

**Files:**
- Modify: `frontend/package.json`

- [ ] **Step 1: 安装 xterm 依赖**

```bash
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\workbench\frontend
npm install xterm@^5 xterm-addon-fit@^0.8 xterm-addon-web-links@^0.9
```

- [ ] **Step 2: 验证安装**

```bash
npm ls xterm
```

Expected: `xterm@5.x.x` 显示成功

- [ ] **Step 3: 生成 Wails JS 绑定**

```bash
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\workbench
wails generate module
```

如果 `wails generate module` 不可用，在 `wails dev` 时会自动生成。

- [ ] **Step 4: 提交**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "feat(terminal): 安装 xterm.js 及插件依赖"
```

---

### Task 6: 前端组合式函数 — useTerminal

**Files:**
- Create: `frontend/src/composables/useTerminal.js`

- [ ] **Step 1: 创建 composables 目录和 useTerminal.js**

创建 `frontend/src/composables/useTerminal.js`：

```javascript
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import { WebLinksAddon } from 'xterm-addon-web-links'
import 'xterm/css/xterm.css'
import {
  CreateTerminal,
  WriteTerminalInput,
  ChangeTerminalDir,
  ResizeTerminal,
  CloseTerminal
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

export function useTerminal() {
  const term = ref(null)
  const fitAddon = ref(null)
  const sessionID = ref('')
  const isActive = ref(false)
  const currentDir = ref('')
  const currentShellType = ref('powershell')
  const isExited = ref(false)

  // 初始化终端
  async function initTerminal(container, dir, shellType) {
    if (isActive.value && sessionID.value) {
      return
    }

    // 创建 xterm 实例
    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Consolas, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        selectionBackground: '#264f78'
      },
      allowProposedApi: true
    })

    const fit = new FitAddon()
    terminal.loadAddon(fit)
    terminal.loadAddon(new WebLinksAddon())

    terminal.open(container)
    fit.fit()

    term.value = terminal
    fitAddon.value = fit
    currentDir.value = dir
    currentShellType.value = shellType || 'powershell'

    // 计算 PTY 初始大小
    const cols = terminal.cols
    const rows = terminal.rows

    // 创建后端 PTY 会话
    try {
      const sid = await CreateTerminal(dir, currentShellType.value, cols, rows)
      sessionID.value = sid
      isActive.value = true
      isExited.value = false
    } catch (err) {
      terminal.writeln(`\x1b[31m创建终端失败: ${err}\x1b[0m`)
      return
    }

    // 监听用户输入，转发到后端
    terminal.onData((data) => {
      if (sessionID.value) {
        WriteTerminalInput(sessionID.value, data).catch(() => {})
      }
    })

    // 监听后端输出
    EventsOn('terminal-output', (sid, output) => {
      if (sid === sessionID.value && term.value) {
        term.value.write(output)
      }
    })

    // 监听终端退出
    EventsOn('terminal-exit', (sid) => {
      if (sid === sessionID.value) {
        isActive.value = false
        isExited.value = true
        if (term.value) {
          term.value.writeln('\r\n\x1b[33m终端进程已退出。点击「重新启动」恢复。\x1b[0m')
        }
      }
    })
  }

  // 切换工作目录
  async function changeDir(dir) {
    if (!sessionID.value || !isActive.value) return
    if (dir === currentDir.value) return
    try {
      await ChangeTerminalDir(sessionID.value, dir)
      currentDir.value = dir
    } catch (err) {
      console.error('切换终端目录失败:', err)
    }
  }

  // 调整大小
  async function resize() {
    if (fitAddon.value && term.value) {
      fitAddon.value.fit()
      if (sessionID.value && isActive.value) {
        try {
          await ResizeTerminal(sessionID.value, term.value.cols, term.value.rows)
        } catch (err) {
          console.error('调整终端大小失败:', err)
        }
      }
    }
  }

  // 销毁终端
  async function destroyTerminal() {
    // 移除事件监听
    EventsOff('terminal-output')
    EventsOff('terminal-exit')

    if (sessionID.value) {
      try {
        await CloseTerminal(sessionID.value)
      } catch (err) {
        console.error('关闭终端失败:', err)
      }
      sessionID.value = ''
    }

    if (term.value) {
      term.value.dispose()
      term.value = null
    }

    fitAddon.value = null
    isActive.value = false
    isExited.value = false
  }

  // 重新启动终端
  async function restartTerminal(container, dir, shellType) {
    await destroyTerminal()
    await initTerminal(container, dir, shellType)
  }

  return {
    term,
    sessionID,
    isActive,
    isExited,
    currentDir,
    currentShellType,
    initTerminal,
    changeDir,
    resize,
    destroyTerminal,
    restartTerminal
  }
}
```

- [ ] **Step 2: 提交**

```bash
git add frontend/src/composables/useTerminal.js
git commit -m "feat(terminal): 添加 useTerminal 组合式函数"
```

---

### Task 7: 前端组件 — TerminalPanel

**Files:**
- Create: `frontend/src/components/TerminalPanel.vue`

- [ ] **Step 1: 创建 TerminalPanel.vue**

创建 `frontend/src/components/TerminalPanel.vue`：

```vue
<template>
  <div v-if="visible" class="terminal-panel">
    <!-- 工具栏 -->
    <div class="terminal-toolbar">
      <div class="terminal-toolbar-left">
        <el-select
          v-model="shellType"
          size="small"
          class="shell-select"
          @change="onShellChange"
        >
          <el-option
            v-for="config in shellConfigs"
            :key="config.type"
            :label="config.displayName"
            :value="config.type"
          />
        </el-select>
        <span class="terminal-dir" :title="currentDir">{{ currentDir }}</span>
      </div>
      <div class="terminal-toolbar-right">
        <el-button
          v-if="isExited"
          size="small"
          type="primary"
          text
          @click="onRestart"
        >
          重新启动
        </el-button>
        <span class="toolbar-btn" @click="$emit('toggle')">─</span>
      </div>
    </div>
    <!-- 终端区域 -->
    <div ref="terminalContainer" class="terminal-container"></div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useTerminal } from '../composables/useTerminal'
import { GetShellConfigs } from '../../wailsjs/go/main/App'

const props = defineProps({
  visible: { type: Boolean, default: false },
  currentDir: { type: String, default: '' }
})

defineEmits(['toggle'])

const {
  isActive,
  isExited,
  currentDir: terminalDir,
  initTerminal,
  changeDir,
  resize,
  destroyTerminal,
  restartTerminal
} = useTerminal()

const terminalContainer = ref(null)
const shellType = ref('powershell')
const shellConfigs = ref([])
const hasInitialized = ref(false)

// 加载 Shell 配置
onMounted(async () => {
  try {
    shellConfigs.value = await GetShellConfigs()
  } catch {
    shellConfigs.value = [
      { type: 'powershell', displayName: 'PowerShell' },
      { type: 'cmd', displayName: 'CMD' },
      { type: 'gitbash', displayName: 'Git Bash' },
      { type: 'wsl', displayName: 'WSL' }
    ]
  }
})

// 监听 visible 变化，首次打开时初始化终端
watch(() => props.visible, async (val) => {
  if (val && !hasInitialized.value && terminalContainer.value) {
    await nextTick()
    const dir = props.currentDir || 'C:\\'
    await initTerminal(terminalContainer.value, dir, shellType.value)
    hasInitialized.value = true
  }
  // 展开时重新调整大小
  if (val && isActive.value) {
    await nextTick()
    resize()
  }
})

// 监听目录变化，自动跟随
watch(() => props.currentDir, (newDir) => {
  if (newDir && isActive.value) {
    changeDir(newDir)
  }
})

// Shell 类型切换
async function onShellChange(newType) {
  if (!terminalContainer.value) return
  const dir = terminalDir.value || props.currentDir || 'C:\\'
  await restartTerminal(terminalContainer.value, dir, newType)
}

// 重新启动
async function onRestart() {
  if (!terminalContainer.value) return
  const dir = terminalDir.value || props.currentDir || 'C:\\'
  await restartTerminal(terminalContainer.value, dir, shellType.value)
}

// 窗口 resize 监听
let resizeObserver = null

onMounted(() => {
  resizeObserver = new ResizeObserver(() => {
    if (props.visible && isActive.value) {
      resize()
    }
  })
})

onBeforeUnmount(async () => {
  if (resizeObserver) {
    resizeObserver.disconnect()
  }
  await destroyTerminal()
})
</script>

<style scoped>
.terminal-panel {
  display: flex;
  flex-direction: column;
  background-color: #1e1e1e;
  border-top: 1px solid var(--border-color, #3c3c3c);
  height: 100%;
}

.terminal-toolbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  padding: 0 8px;
  background: #252526;
  border-bottom: 1px solid #3c3c3c;
}

.terminal-toolbar-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.shell-select {
  width: 130px;
}

.shell-select :deep(.el-input__wrapper) {
  background: #3c3c3c;
  box-shadow: none;
  border: 1px solid #4c4c4c;
}

.shell-select :deep(.el-input__inner) {
  color: #d4d4d4;
  font-size: 12px;
}

.terminal-dir {
  font-size: 12px;
  color: #888;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.terminal-toolbar-right {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.toolbar-btn {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #888;
  cursor: pointer;
  border-radius: 4px;
  font-size: 14px;
  transition: all 0.15s;
}

.toolbar-btn:hover {
  background: #3c3c3c;
  color: #d4d4d4;
}

.terminal-container {
  flex: 1;
  min-height: 0;
  padding: 4px 8px;
}

.terminal-container :deep(.xterm) {
  height: 100%;
}

.terminal-container :deep(.xterm-viewport) {
  overflow-y: auto !important;
}
</style>
```

- [ ] **Step 2: 提交**

```bash
git add frontend/src/components/TerminalPanel.vue
git commit -m "feat(terminal): 添加 TerminalPanel 终端面板组件"
```

---

### Task 8: 前端布局改造 — Home.vue

**Files:**
- Modify: `frontend/src/views/Home.vue`

- [ ] **Step 1: 改造 Home.vue 布局**

在 `<template>` 中，将现有 `.home-layout` 内容包裹在上半区，新增终端面板下半区和拖拽分隔条。完整替换 `<template>` 部分：

```vue
<template>
  <div class="home">
    <div class="home-layout">
      <ActivityBar v-model="activePanel" />
      <div class="main-area">
        <!-- 上半区：原有 Splitpanes 三栏 -->
        <div class="main-panes" ref="mainPanesRef">
          <Splitpanes class="default-theme splitpanes-container" :push-other-panes="false" :maximize-panes="false">
            <Pane :size="20" :min-size="10">
              <div class="pane-content" style="position:relative;">
                <DirectoryTree
                  v-show="activePanel === 'directory'"
                  ref="directoryTreeRef"
                  :directories="directories"
                  :selected-id="selectedDirectoryId"
                  :version="appVersion"
                  @select="onDirectorySelect"
                  @change="loadDirectories"
                  @contextmenu="onDirectoryContextMenu"
                  @batch-pull="onBatchPull"
                />
                <ToolboxPanel
                  v-show="activePanel === 'toolbox'"
                  @close="activePanel = 'directory'"
                />
                <SettingsPanel
                  v-show="activePanel === 'settings'"
                  @close="activePanel = 'directory'"
                />
              </div>
            </Pane>
            <Pane :size="30" :min-size="15">
              <div class="pane-content" @mousedown="closeToolbox" @contextmenu="closeToolbox">
                <FileTreePanel
                  ref="fileTreePanelRef"
                  :directories="directories"
                  :selected-dir-id="selectedDirectoryId"
                  :clipboard="clipboard"
                  @select="onNodeSelect"
                  @batch-pull="onBatchPull"
                  @copy="handleCopy"
                  @cut="handleCut"
                  @paste="handlePaste"
                  @copy-to="handleCopyTo"
                  @contextmenu="onFileTreeContextMenu"
                  @delete="onDeleteFromFileTree"
                />
              </div>
            </Pane>
            <Pane :size="50" :min-size="30">
              <div class="pane-content" @mousedown="closeToolbox" @contextmenu="closeToolbox">
                <ContentPanel
                  ref="contentPanelRef"
                  :selected-node="selectedNode"
                  :latest-commit="latestCommit"
                  :clipboard="clipboard"
                  @latest-commit="commit => latestCommit = commit"
                  @refresh-node="onRefreshNode"
                  @create-directory="node => fileTreePanelRef.showCreateAt(node, 'directory')"
                  @create-file="node => fileTreePanelRef.showCreateAt(node, 'file')"
                  @rename="onRenameFromContent"
                  @delete="onDeleteFromContent"
                  @copy="handleCopy"
                  @cut="handleCut"
                  @paste="handlePaste"
                  @copy-to="node => fileTreePanelRef.showCopyToDialog(node)"
                  @batch-pull="onBatchPull"
                />
              </div>
            </Pane>
          </Splitpanes>
        </div>
        <!-- 拖拽分隔条 -->
        <div
          v-if="terminalVisible"
          class="resize-bar"
          @mousedown="onResizeBarMouseDown"
        ></div>
        <!-- 下半区：终端面板 -->
        <TerminalPanel
          :visible="terminalVisible"
          :current-dir="terminalDir"
          :style="{ height: terminalVisible ? terminalHeight + 'px' : '0px' }"
          @toggle="toggleTerminal"
        />
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 2: 更新 script 部分**

在 `<script setup>` 中新增终端相关逻辑。在现有 import 后添加：

```javascript
import TerminalPanel from '../components/TerminalPanel.vue'
```

在 `// ---- 核心状态 ----` 区域后新增终端状态：

```javascript
// ---- 终端状态 ----
const terminalVisible = ref(false)
const terminalHeight = ref(200)
const terminalDir = ref('')
const mainPanesRef = ref(null)
```

在 `// ---- 键盘快捷键 ----` 的 `handleGlobalKeydown` 函数中新增终端快捷键逻辑。在 `if (e.key === 'F5')` 块之前添加：

```javascript
  // Ctrl+` 切换终端
  if (e.key === '`' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault()
    toggleTerminal()
    return
  }
```

在 `handleGlobalKeydown` 函数之后新增终端相关函数：

```javascript
// ---- 终端 ----
const toggleTerminal = () => {
  terminalVisible.value = !terminalVisible.value
}

// 更新终端跟随目录
watch(() => selectedNode.value, (node) => {
  if (node && node.type === 'directory') {
    terminalDir.value = node.path
  } else if (node && node.type === 'file') {
    // 文件节点取父目录
    const lastSep = Math.max(node.path.lastIndexOf('\\'), node.path.lastIndexOf('/'))
    terminalDir.value = lastSep > 0 ? node.path.substring(0, lastSep) : node.path
  }
})

// 也可以跟随选中的工作目录
watch(() => selectedDirectoryId.value, () => {
  const dir = directories.value.find(d => d.id === selectedDirectoryId.value)
  if (dir && !selectedNode.value) {
    terminalDir.value = dir.path
  }
})

// 拖拽分隔条
const onResizeBarMouseDown = (e) => {
  e.preventDefault()
  const startY = e.clientY
  const startHeight = terminalHeight.value

  const onMouseMove = (moveEvent) => {
    const delta = startY - moveEvent.clientY
    const newHeight = Math.max(100, Math.min(startHeight + delta, window.innerHeight - 200))
    terminalHeight.value = newHeight
  }

  const onMouseUp = () => {
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}
```

- [ ] **Step 3: 更新 style 部分**

在 `<style scoped>` 中修改 `.home-layout` 和新增终端相关样式：

```css
.home-layout {
  display: flex;
  height: 100%;
  width: 100%;
}

.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
}

.main-panes {
  flex: 1;
  min-height: 0;
  overflow: hidden !important;
}

.resize-bar {
  flex-shrink: 0;
  height: 3px;
  background: var(--border-color, #3c3c3c);
  cursor: ns-resize;
  transition: background 0.15s;
}

.resize-bar:hover {
  background: var(--primary-color, #409eff);
}
```

移除原有的 `.splitpanes-wrapper` 样式，替换为 `.main-panes` 样式。原有 `.splitpanes-container`、`.pane-content` 及全局 Splitpanes 样式保持不变。

- [ ] **Step 4: 验证编译**

```bash
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\workbench\frontend
npm run build
```

Expected: 构建成功

- [ ] **Step 5: 提交**

```bash
git add frontend/src/views/Home.vue
git commit -m "feat(terminal): Home.vue 布局改造（上下分区 + 终端面板集成）"
```

---

### Task 9: 前端 ActivityBar — 新增终端图标

**Files:**
- Modify: `frontend/src/components/ActivityBar.vue`

- [ ] **Step 1: 添加终端图标**

修改 `ActivityBar.vue` 的 `<script setup>` 部分，在 import 中新增图标：

```javascript
import { Folder, SetUp, Setting, Monitor } from '@element-plus/icons-vue'
```

修改 `panels` 数组，新增终端项：

```javascript
const panels = [
  { id: 'directory', icon: Folder, label: '工作目录' },
  { id: 'toolbox', icon: SetUp, label: '工具箱' },
  { id: 'settings', icon: Setting, label: '设置' }
]
```

终端图标不走 `activePanel` 切换逻辑（它不替换左侧面板），而是通过 emit 通知 Home 切换终端面板显隐。修改模板：

```vue
<template>
  <div class="activity-bar">
    <div
      v-for="item in panels"
      :key="item.id"
      class="activity-bar-item"
      :class="{ 'is-active': modelValue === item.id }"
      @click="$emit('update:modelValue', item.id)"
    >
      <el-icon :size="20">
        <component :is="item.icon" />
      </el-icon>
    </div>
    <!-- 终端图标（底部） -->
    <div class="activity-bar-spacer"></div>
    <div
      class="activity-bar-item"
      :class="{ 'is-active': terminalActive }"
      @click="$emit('toggleTerminal')"
    >
      <el-icon :size="20">
        <Monitor />
      </el-icon>
    </div>
  </div>
</template>
```

更新 props 和 emits：

```javascript
defineProps({
  modelValue: { type: String, default: 'directory' },
  terminalActive: { type: Boolean, default: false }
})

defineEmits(['update:modelValue', 'toggleTerminal'])
```

新增样式：

```css
.activity-bar-spacer {
  flex: 1;
}
```

- [ ] **Step 2: 更新 Home.vue 中 ActivityBar 的使用**

修改 `Home.vue` 中 `<ActivityBar>` 标签：

```vue
<ActivityBar v-model="activePanel" :terminal-active="terminalVisible" @toggle-terminal="toggleTerminal" />
```

- [ ] **Step 3: 提交**

```bash
git add frontend/src/components/ActivityBar.vue frontend/src/views/Home.vue
git commit -m "feat(terminal): ActivityBar 新增终端图标，支持切换终端面板"
```

---

### Task 10: 前端设置面板 — 终端配置

**Files:**
- Modify: `frontend/src/components/SettingsPanel.vue`

- [ ] **Step 1: 在设置面板新增终端配置区域**

修改 `SettingsPanel.vue`，在 `settings-content` 的 `settings-section` 后新增终端设置 section：

在 `<template>` 的 `settings-content` div 内，在现有 `settings-section` 后面添加：

```html
<div class="settings-section">
  <div class="settings-section-title">终端</div>
  <div class="settings-item">
    <div class="settings-item-info">
      <div class="settings-item-label">默认 Shell</div>
      <div class="settings-item-desc">终端面板使用的 Shell 类型</div>
    </div>
    <el-select v-model="defaultShell" size="small" style="width: 140px;" @change="onSettingsChange">
      <el-option label="PowerShell" value="powershell" />
      <el-option label="CMD" value="cmd" />
      <el-option label="Git Bash" value="gitbash" />
      <el-option label="WSL" value="wsl" />
    </el-select>
  </div>
  <div v-if="defaultShell === 'gitbash'" class="settings-item" style="margin-top:8px;">
    <div class="settings-item-info">
      <div class="settings-item-label">Git Bash 路径</div>
      <div class="settings-item-desc">自定义 Git Bash 可执行文件路径</div>
    </div>
    <el-input v-model="gitBashPath" size="small" style="width: 240px;" @change="onSettingsChange" />
  </div>
  <div v-if="defaultShell === 'wsl'" class="settings-item" style="margin-top:8px;">
    <div class="settings-item-info">
      <div class="settings-item-label">WSL 发行版</div>
      <div class="settings-item-desc">指定 WSL 发行版名称（留空使用默认）</div>
    </div>
    <el-input v-model="wslDistro" size="small" style="width: 240px;" @change="onSettingsChange" />
  </div>
</div>
```

在 `<script setup>` 中新增状态和方法：

```javascript
const defaultShell = ref('powershell')
const gitBashPath = ref('C:\\Program Files\\Git\\bin\\bash.exe')
const wslDistro = ref('')

// 修改 onMounted，加载终端设置
onMounted(async () => {
  try {
    const settings = await GetSettings()
    gpuEnabled.value = !settings.gpuDisabled
    defaultShell.value = settings.defaultShell || 'powershell'
    gitBashPath.value = settings.gitBashPath || 'C:\\Program Files\\Git\\bin\\bash.exe'
    wslDistro.value = settings.wslDistro || ''
  } catch {
    gpuEnabled.value = true
  }
})

const onSettingsChange = async () => {
  try {
    await SaveSettings({
      gpuDisabled: !gpuEnabled.value,
      defaultShell: defaultShell.value,
      gitBashPath: gitBashPath.value,
      wslDistro: wslDistro.value
    })
  } catch {
    // 回滚
  }
}
```

同时修改 `onGpuChange` 方法，保存时包含终端设置：

```javascript
const onGpuChange = async (val) => {
  try {
    await SaveSettings({
      gpuDisabled: !val,
      defaultShell: defaultShell.value,
      gitBashPath: gitBashPath.value,
      wslDistro: wslDistro.value
    })
    needsRestart.value = true
  } catch {
    gpuEnabled.value = !gpuEnabled.value
  }
}
```

- [ ] **Step 2: 提交**

```bash
git add frontend/src/components/SettingsPanel.vue
git commit -m "feat(terminal): 设置面板新增终端配置（Shell类型/自定义路径）"
```

---

### Task 11: 集成验证与修复

**Files:**
- 可能修改上述任意文件

- [ ] **Step 1: 运行后端测试**

```bash
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\workbench
go test ./... -v
```

Expected: 所有测试通过

- [ ] **Step 2: 运行前端构建**

```bash
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\workbench\frontend
npm run build
```

Expected: 构建成功，无错误

- [ ] **Step 3: 启动开发模式**

```bash
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\workbench
wails dev
```

验证清单：
1. 应用启动正常
2. 点击 ActivityBar 终端图标，底部面板展开
3. 终端显示 PowerShell 提示符
4. 可在终端内输入命令并看到输出
5. 点击文件树目录节点，终端自动 cd 到该目录
6. 拖拽分隔条可调节终端高度
7. 按 Ctrl+` 可切换终端显隐
8. 收起终端后再展开，终端仍可用
9. 设置面板可切换默认 Shell 类型
10. 关闭应用时终端进程正常退出

- [ ] **Step 4: 修复发现的问题**

根据验证结果修复任何问题，然后提交。

- [ ] **Step 5: 最终提交**

```bash
git add -A
git commit -m "fix(terminal): 集成验证修复"
```

---

### Task 12: 更新 README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 在 README 中添加终端功能说明**

在功能列表中新增终端面板功能的描述，包含：
- 内置终端面板（底部，可拖拽可收起）
- 支持 PowerShell / CMD / Git Bash / WSL
- 目录自动跟随文件树
- 快捷键 Ctrl+` 切换

- [ ] **Step 2: 提交**

```bash
git add README.md
git commit -m "docs: README 新增终端面板功能说明"
```
