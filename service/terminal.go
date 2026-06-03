package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"git-manager/model"
	"git-manager/util"
)

// TerminalService 终端服务，管理终端会话的创建、输入、切换目录、调整大小和关闭
type TerminalService struct {
	ctx      context.Context
	sessions map[string]*model.TerminalSession
	mu       sync.Mutex
}

// NewTerminalService 创建终端服务实例
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

	storePtyProcess(session, ptyProc)

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	go s.startOutputPump(sessionID, ptyProc)
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
	cdCmd := s.buildCdCommand(dir, session.ShellType)
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

// buildCdCommand 根据 Shell 类型构建 cd 命令
// CMD: cd /d "path"（/d 标志切换驱动器+目录）
// PowerShell: cd "path"（自动处理驱动器切换）
// Git Bash: cd "path"（反斜杠转正斜杠）
// WSL: cd "/mnt/x/path"（Windows 路径转 WSL 挂载路径）
func (s *TerminalService) buildCdCommand(dir string, shellType string) string {
	normalizedDir := filepath.Clean(dir)

	switch shellType {
	case "cmd":
		// CMD: /d 标志用于同时切换驱动器和目录
		return fmt.Sprintf(`cd /d "%s"`, normalizedDir) + "\n"
	case "gitbash":
		// Git Bash: 无 /d 标志，反斜杠转正斜杠
		unixDir := strings.ReplaceAll(normalizedDir, `\`, `/`)
		return fmt.Sprintf(`cd "%s"`, unixDir) + "\n"
	case "wsl":
		// WSL: D:\path → /mnt/d/path
		wslDir := toWslPath(normalizedDir)
		return fmt.Sprintf(`cd "%s"`, wslDir) + "\n"
	default:
		// PowerShell 及其他: 无需 /d 标志
		return fmt.Sprintf(`cd "%s"`, normalizedDir) + "\n"
	}
}

// toWslPath 将 Windows 路径转换为 WSL 挂载路径
// D:\work → /mnt/d/work，C:\ → /mnt/c/
func toWslPath(path string) string {
	if len(path) >= 2 && path[1] == ':' {
		drive := strings.ToLower(string(path[0]))
		rest := strings.ReplaceAll(path[3:], `\`, `/`)
		return fmt.Sprintf("/mnt/%s/%s", drive, rest)
	}
	return strings.ReplaceAll(path, `\`, `/`)
}

// startOutputPump 输出泵，持续读取 PTY 输出并向前端发送事件
func (s *TerminalService) startOutputPump(sessionID string, ptyProc *util.PtyProcess) {
	buf := make([]byte, 4096)
	for {
		n, err := ptyProc.Read(buf)
		if err != nil {
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
